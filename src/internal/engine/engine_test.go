package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Darida/openrouterclient/src/internal/hedge"
	"github.com/Darida/openrouterclient/src/internal/history"
	"github.com/Darida/openrouterclient/src/model"
)

var requirements = model.TextGenerationRequirements{
	Prompt: "Name a fruit.",
	OutputSchema: model.JSONSchema{
		Name:   "fruit",
		Schema: json.RawMessage(`{"type":"object","properties":{"fruit":{"type":"string"}},"required":["fruit"],"additionalProperties":false}`),
	},
	OutputValidationRules: "1. The fruit must be yellow.",
	TargetQuality:         model.QualityHigh,
	Tag:                   "fruit-test",
	ModelTier:             model.ModelTierFree,
}

// The fake catalog's only candidate, so every attempt picks it.
const pickedModel = "slow/model:free"

// chatRequest is the part of a chat payload the fake server routes on.
type chatRequest struct {
	Model          string `json:"model"`
	ResponseFormat struct {
		JSONSchema struct {
			Name string `json:"name"`
		} `json:"json_schema"`
	} `json:"response_format"`
	Messages []struct {
		Content string `json:"content"`
	} `json:"messages"`
}

// fakeOpenRouter answers generation requests with generate and review
// requests with review, keyed by the 1-based count of that kind so far.
type fakeOpenRouter struct {
	mu          sync.Mutex
	generations []chatRequest
	reviews     int
	generate    func(w http.ResponseWriter, r *http.Request, n int)
	review      func(n int) string
}

func (f *fakeOpenRouter) serve(t *testing.T) (*httptest.Server, Settings) {
	mux := http.NewServeMux()
	mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req chatRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("fake: bad payload: %v", err)
		}
		f.mu.Lock()
		if req.ResponseFormat.JSONSchema.Name == "review_verdict" {
			f.reviews++
			n := f.reviews
			f.mu.Unlock()
			reply(w, "gen-review-"+fmt.Sprint(n), "reviewer/free", f.review(n))
			return
		}
		f.generations = append(f.generations, req)
		n := len(f.generations)
		f.mu.Unlock()
		f.generate(w, r, n)
	})
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[
			{"id":"slow/model:free","supported_parameters":["structured_outputs"],"architecture":{"output_modalities":["text"]},"pricing":{"prompt":"0","completion":"0"}},
			{"id":"cheap/model","supported_parameters":["structured_outputs"],"architecture":{"output_modalities":["text"]},"pricing":{"prompt":"0.000001","completion":"0.000001"}},
			{"id":"pricey/model","supported_parameters":["structured_outputs"],"architecture":{"output_modalities":["text"]},"pricing":{"prompt":"0.001","completion":"0.001"}}
		]}`)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	settings := Settings{
		ChatURL:          server.URL + "/chat",
		CatalogURL:       server.URL + "/models",
		Hedge:            hedge.Timing{MaxAttempts: 2, Stagger: 5 * time.Second, AttemptTimeout: 200 * time.Millisecond},
		MaxRounds:           3,
		RejectionRetryDelay: 10 * time.Millisecond,
	}
	return server, settings
}

func reply(w http.ResponseWriter, generationID, modelID, content string) {
	w.Header().Set("X-Generation-Id", generationID)
	body, _ := json.Marshal(map[string]any{"model": modelID, "choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
	w.Write(body)
}

func noNotes(int) string { return `{"notes":[]}` }

func newEngine(t *testing.T, settings Settings) (*Engine, string) {
	path := filepath.Join(t.TempDir(), "history.json")
	engine := New(settings, "key", history.Open(path), slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(engine.Close)
	return engine, path
}

func readHistory(t *testing.T, path string) []history.Entry {
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var entries []history.Entry
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestEngineGenerateText_whenReviewHasNoNotes_thenReturnsHighQuality(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) { reply(w, "gen-1", "writer/free", `{"fruit":"banana"}`) },
		review:   noNotes,
	}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	got, err := engine.GenerateText(context.Background(), requirements)

	// Assert
	if err != nil || got.Review == nil || got.Review.Quality != model.QualityHigh || got.Model != pickedModel {
		t.Fatalf("got %+v, %v; want high quality from %s", got, err, pickedModel)
	}
}

func TestEngineGenerateText_whenReviewBelowTarget_thenCorrectionCarriesNotes(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) {
			reply(w, fmt.Sprintf("gen-%d", n), "writer/free", `{"fruit":"apple"}`)
		},
		review: func(n int) string {
			if n == 1 {
				return `{"notes":[{"rule":"1","text":"Apples are not yellow."}]}`
			}
			return `{"notes":[]}`
		},
	}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	_, err := engine.GenerateText(context.Background(), requirements)

	// Assert
	if err != nil || len(fake.generations) != 2 || !strings.Contains(fake.generations[1].Messages[2].Content, "Apples are not yellow.") {
		t.Fatalf("err=%v generations=%+v; want a second generation carrying the review note", err, fake.generations)
	}
}

func TestEngineGenerateText_whenEveryRoundBelowTarget_thenAttemptsExhausted(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) {
			reply(w, fmt.Sprintf("gen-%d", n), "writer/free", `{"fruit":"apple"}`)
		},
		review: func(int) string { return `{"notes":[{"rule":"1","text":"Not yellow."}]}` },
	}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	_, err := engine.GenerateText(context.Background(), requirements)

	// Assert
	var exhausted *model.AttemptsExhaustedError
	if !errors.As(err, &exhausted) || len(exhausted.Attempts) != settings.MaxRounds {
		t.Fatalf("err = %v; want AttemptsExhaustedError with %d below-target attempts", err, settings.MaxRounds)
	}
}

func TestEngineGenerateText_whenAttemptTimesOut_thenRecordsTimeoutAgainstPickedModel(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) {
			if n == 1 {
				w.Header().Set("X-Generation-Id", "gen-slow")
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				return
			}
			reply(w, "gen-2", "writer/free", `{"fruit":"banana"}`)
		},
		review: noNotes,
	}
	_, settings := fake.serve(t)
	engine, path := newEngine(t, settings)
	if _, err := engine.GenerateText(context.Background(), requirements); err != nil {
		t.Fatalf("setup: GenerateText failed: %v", err)
	}

	// Act
	engine.Close()
	entries := readHistory(t, path)

	// Assert
	for _, e := range entries {
		if f := e.Fields(); f.GenerationID == "gen-slow" && f.Model == "slow/model:free" && f.Outcome == history.OutcomeTimeout {
			return
		}
	}
	t.Fatalf("history %+v has no timeout entry for slow/model:free", entries)
}

func TestEngineGenerateText_whenOutputViolatesSchema_thenRecordsInvalidOutput(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) {
			if n == 1 {
				reply(w, "gen-bad", "sloppy/free", `{"vegetable":"carrot"}`)
				return
			}
			reply(w, "gen-2", "writer/free", `{"fruit":"banana"}`)
		},
		review: noNotes,
	}
	_, settings := fake.serve(t)
	engine, path := newEngine(t, settings)
	if _, err := engine.GenerateText(context.Background(), requirements); err != nil {
		t.Fatalf("setup: GenerateText failed: %v", err)
	}

	// Act
	engine.Close()
	entries := readHistory(t, path)

	// Assert
	for _, e := range entries {
		if f := e.Fields(); f.GenerationID == "gen-bad" && f.Outcome == history.OutcomeInvalidOutput && f.Quality == model.QualityUnusable {
			return
		}
	}
	t.Fatalf("history %+v has no unusable invalid_output entry for gen-bad", entries)
}

func TestEngineRate_whenGenerationReturned_thenRecordsManualRating(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) { reply(w, "gen-1", "writer/free", `{"fruit":"banana"}`) },
		review:   noNotes,
	}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	got, err := engine.GenerateText(context.Background(), requirements)
	if err != nil {
		t.Fatalf("setup: GenerateText failed: %v", err)
	}

	// Act
	err = engine.Rate(context.Background(), got.GenerationID, model.QualityLow, "bananas are boring")

	// Assert
	if err != nil {
		t.Fatalf("Rate: %v", err)
	}
}

func overloadedFirst(w http.ResponseWriter, r *http.Request, n int) {
	if n == 1 {
		w.Header().Set("X-Generation-Id", "gen-overloaded")
		fmt.Fprint(w, `{"id":"gen-overloaded","error":{"message":"Upstream error from Nvidia: Service temporarily overloaded","code":503,"metadata":{"error_type":"provider_overloaded"}}}`)
		return
	}
	reply(w, "gen-2", "writer/free", `{"fruit":"banana"}`)
}

func TestEngineGenerateText_whenProviderRejectsIn200Body_thenFirstAttemptResendsAndWins(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: overloadedFirst, review: noNotes}
	_, settings := fake.serve(t)
	settings.Hedge.MaxAttempts = 1
	engine, _ := newEngine(t, settings)

	// Act
	got, err := engine.GenerateText(context.Background(), requirements)

	// Assert
	if err != nil || got.GenerationID != "gen-2" {
		t.Fatalf("got %+v, %v; want the single attempt to resend and return gen-2", got, err)
	}
}

func TestEngineGenerateText_whenProviderRejectsIn200Body_thenRecordsNothingForIt(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: overloadedFirst, review: noNotes}
	_, settings := fake.serve(t)
	engine, path := newEngine(t, settings)
	if _, err := engine.GenerateText(context.Background(), requirements); err != nil {
		t.Fatalf("setup: GenerateText failed: %v", err)
	}

	// Act
	engine.Close()
	entries := readHistory(t, path)

	// Assert
	for _, e := range entries {
		if e.Fields().GenerationID == "gen-overloaded" {
			t.Fatalf("history %+v records the rejected request", entries)
		}
	}
}

func rateLimitedFirst(w http.ResponseWriter, r *http.Request, n int) {
	if n == 1 {
		w.Header().Set("X-Generation-Id", "gen-limited")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"message":"Provider returned error","code":429,"metadata":{"raw":"qwen/qwen3.8-27b:free is temporarily rate-limited upstream. Please retry shortly.","provider_name":"ModelRun"}}}`)
		return
	}
	reply(w, "gen-2", "writer/free", `{"fruit":"banana"}`)
}

func TestEngineGenerateText_whenRateLimited_thenRecordsFailureAgainstPickedModel(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: rateLimitedFirst, review: noNotes}
	_, settings := fake.serve(t)
	engine, path := newEngine(t, settings)
	if _, err := engine.GenerateText(context.Background(), requirements); err != nil {
		t.Fatalf("setup: GenerateText failed: %v", err)
	}

	// Act
	engine.Close()
	entries := readHistory(t, path)

	// Assert
	for _, e := range entries {
		if f := e.Fields(); f.GenerationID == "gen-limited" && f.Model == pickedModel && f.Outcome == history.OutcomeFailed {
			return
		}
	}
	t.Fatalf("history %+v has no failed entry for gen-limited against %s", entries, pickedModel)
}

const cohereRefusal = `{"error":{"message":"Provider returned error","code":400,"metadata":{"raw":"{\"message\":\"invalid request: received non-supported constraint for type: 'string'. constraint: 'minLength'\"}","provider_name":"Cohere","is_byok":false}}}`

func refuse(w http.ResponseWriter) {
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprint(w, cohereRefusal)
}

func refusedFirst(w http.ResponseWriter, r *http.Request, n int) {
	if n == 1 {
		refuse(w)
		return
	}
	reply(w, "gen-2", "writer/free", `{"fruit":"banana"}`)
}

func alwaysRefused(w http.ResponseWriter, r *http.Request, n int) { refuse(w) }

func TestEngineGenerateText_whenModelRefusesAndAnotherAttemptWins_thenRecordsRefusalAgainstPickedModel(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: refusedFirst, review: noNotes}
	_, settings := fake.serve(t)
	engine, path := newEngine(t, settings)
	if _, err := engine.GenerateText(context.Background(), requirements); err != nil {
		t.Fatalf("setup: GenerateText failed: %v", err)
	}

	// Act
	engine.Close()
	entries := readHistory(t, path)

	// Assert
	for _, e := range entries {
		if f := e.Fields(); f.Model == pickedModel && f.Outcome == history.OutcomeRefused && f.Quality == model.QualityUnusable {
			return
		}
	}
	t.Fatalf("history %+v has no unusable refused entry for %s", entries, pickedModel)
}

func TestEngineGenerateText_whenEveryAttemptRefused_thenReturnsRefusedAttempts(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: alwaysRefused, review: noNotes}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	_, err := engine.GenerateText(context.Background(), requirements)

	// Assert
	var exhausted *model.AttemptsExhaustedError
	if !errors.As(err, &exhausted) || len(exhausted.Attempts) != settings.Hedge.MaxAttempts {
		t.Fatalf("err = %v, want AttemptsExhaustedError with %d attempts", err, settings.Hedge.MaxAttempts)
	}
	for _, a := range exhausted.Attempts {
		if a.Outcome != model.OutcomeRefused {
			t.Fatalf("attempt %+v, want outcome refused", a)
		}
	}
}

func TestEngineGenerateText_whenEveryAttemptRefused_thenRecordsNoRefusal(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: alwaysRefused, review: noNotes}
	_, settings := fake.serve(t)
	engine, path := newEngine(t, settings)
	if _, err := engine.GenerateText(context.Background(), requirements); err == nil {
		t.Fatal("setup: GenerateText succeeded, want every attempt refused")
	}

	// Act
	engine.Close()
	_, statErr := os.Stat(path)

	// Assert
	if !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("history %+v was written, want nothing recorded", readHistory(t, path))
	}
}

func TestEngineGenerateText_whenValidationRulesEmpty_thenNeverSendsReview(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) { reply(w, "gen-1", "writer/free", `{"fruit":"banana"}`) },
		review:   func(int) string { t.Error("review requested"); return `{"notes":[]}` },
	}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	unreviewed := requirements
	unreviewed.OutputValidationRules = ""

	// Act
	got, err := engine.GenerateText(context.Background(), unreviewed)

	// Assert
	if err != nil || got.Review != nil || fake.reviews != 0 {
		t.Fatalf("got %+v, %v, %d reviews; want unreviewed content", got, err, fake.reviews)
	}
}

func TestEngineGenerateText_whenEveryOutputViolatesSchema_thenNeverSendsReview(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) {
			reply(w, fmt.Sprintf("gen-%d", n), "sloppy/free", `{"vegetable":"carrot"}`)
		},
		review: func(int) string { t.Error("review requested"); return `{"notes":[]}` },
	}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	_, err := engine.GenerateText(context.Background(), requirements)

	// Assert
	var exhausted *model.AttemptsExhaustedError
	if !errors.As(err, &exhausted) || fake.reviews != 0 {
		t.Fatalf("err=%v reviews=%d; want AttemptsExhaustedError and no review", err, fake.reviews)
	}
}

func TestEngineGenerateText_whenValidationRulesEmpty_thenRecordsGenerationAsHigh(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) { reply(w, "gen-1", "writer/free", `{"fruit":"banana"}`) },
		review:   noNotes,
	}
	_, settings := fake.serve(t)
	engine, path := newEngine(t, settings)
	unreviewed := requirements
	unreviewed.OutputValidationRules = ""
	if _, err := engine.GenerateText(context.Background(), unreviewed); err != nil {
		t.Fatalf("setup: GenerateText failed: %v", err)
	}

	// Act
	engine.Close()
	entries := readHistory(t, path)

	// Assert
	for _, e := range entries {
		if f := e.Fields(); f.GenerationID == "gen-1" && f.Quality == model.QualityHigh {
			return
		}
	}
	t.Fatalf("history %+v has no high entry for gen-1", entries)
}

func TestEngineGenerateText_whenPaidTier_thenAsksOnlyTheCheapPaidModel(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) { reply(w, "gen-1", "cheap/model", `{"fruit":"banana"}`) },
		review:   noNotes,
	}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	paid := requirements
	paid.ModelTier = model.ModelTierPaid

	// Act
	_, err := engine.GenerateText(context.Background(), paid)

	// Assert
	if err != nil || len(fake.generations) != 1 || fake.generations[0].Model != "cheap/model" {
		t.Fatalf("err=%v generations=%+v; want one request to cheap/model", err, fake.generations)
	}
}
