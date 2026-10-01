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
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Darida/openrouterclient/src/internal/history"
	"github.com/Darida/openrouterclient/src/internal/replyfile"
	"github.com/Darida/openrouterclient/src/model"
)

const (
	testTag     = "fruit-test"
	testTimeout = 200 * time.Millisecond
)

var freeTier = model.ModelSelection{Tier: model.ModelTierFree}

var fruitSchema = model.JSONSchema{
	Name:   "fruit",
	Schema: json.RawMessage(`{"type":"object","properties":{"fruit":{"type":"string"}},"required":["fruit"],"additionalProperties":false}`),
}

var reviewed = model.GenerateReviewedRequest{
	Prompt:           "Name a fruit.",
	OutputSchema:     fruitSchema,
	GenerationModels: freeTier,
	ReviewModels:     freeTier,
	Criteria:         model.ReviewCriteria{Rules: "1. The fruit must be yellow.", ToleranceThreshold: 3},
	MaxCorrections:   2,
	TargetQuality:    model.QualityHigh,
}

var unreviewed = model.GenerateRequest{
	Prompt:        "Name a fruit.",
	OutputSchema:  fruitSchema,
	Models:        freeTier,
	TargetQuality: model.QualityHigh,
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
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

// fakeOpenRouter answers generation requests with generate and review
// requests with review, keyed by the 1-based count of that kind so far.
type fakeOpenRouter struct {
	mu          sync.Mutex
	generations []chatRequest
	reviews     []chatRequest
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
			f.reviews = append(f.reviews, req)
			n := len(f.reviews)
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
			{"id":"slow/model:free","context_length":100000,"supported_parameters":["structured_outputs"],"architecture":{"output_modalities":["text"]},"pricing":{"prompt":"0","completion":"0"}},
			{"id":"tiny/model","context_length":10000,"supported_parameters":["structured_outputs"],"architecture":{"output_modalities":["text"]},"pricing":{"prompt":"0.0000001","completion":"0.0000001"}},
			{"id":"cheap/model","context_length":100000,"supported_parameters":["structured_outputs"],"architecture":{"output_modalities":["text"]},"pricing":{"prompt":"0.000001","completion":"0.000001"}},
			{"id":"pricey/model","context_length":100000,"supported_parameters":["structured_outputs"],"architecture":{"output_modalities":["text"]},"pricing":{"prompt":"0.001","completion":"0.001"}}
		]}`)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	settings := Settings{
		ChatURL:             server.URL + "/chat",
		CatalogURL:          server.URL + "/models",
		MaxAttempts:         2,
		GraceAfterTimeout:   10 * time.Millisecond,
		RejectionRetryDelay: 10 * time.Millisecond,
	}
	return server, settings
}

func reply(w http.ResponseWriter, generationID, modelID, content string) {
	w.Header().Set("X-Generation-Id", generationID)
	body, _ := json.Marshal(map[string]any{"model": modelID, "choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
	w.Write(body)
}

func noViolations(int) string { return `{"violations":[],"totalBadScore":0}` }

func newEngine(t *testing.T, settings Settings) (*Engine, string) {
	// Keeps saved replies out of the real system temp directory.
	t.Setenv("TMPDIR", t.TempDir())
	path := filepath.Join(t.TempDir(), "history.json")
	store, err := history.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(settings, "key", testTag, testTimeout, store, replyfile.Local(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() {
		if err := engine.Close(); err != nil {
			t.Error(err)
		}
	})
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

func TestEngineGenerateReviewed_whenReviewHasNoViolations_thenReturnsHighQuality(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) {
			reply(w, "gen-1", "writer/free", `{"fruit":"banana"}`)
		},
		review: noViolations,
	}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	got, err := engine.GenerateReviewed(context.Background(), reviewed)

	// Assert
	if err != nil || got.Review.Quality != model.QualityHigh || got.Model != pickedModel {
		t.Fatalf("got %+v, %v; want high quality from %s", got, err, pickedModel)
	}
}

func TestEngineGenerateReviewed_whenReviewBelowTarget_thenCorrectionCarriesViolations(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) {
			reply(w, fmt.Sprintf("gen-%d", n), "writer/free", `{"fruit":"apple"}`)
		},
		review: func(n int) string {
			if n == 1 {
				return `{"violations":[{"rule":"1","evidence":"apple","explanation":"Apples are not yellow.","recommendedAction":"Use a yellow fruit.","badScore":1}],"totalBadScore":1}`
			}
			return `{"violations":[],"totalBadScore":0}`
		},
	}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	_, err := engine.GenerateReviewed(context.Background(), reviewed)

	// Assert
	if err != nil || len(fake.generations) != 2 || !strings.Contains(fake.generations[1].Messages[2].Content, "Apples are not yellow.") {
		t.Fatalf("err=%v generations=%+v; want a second generation carrying the review violation", err, fake.generations)
	}
}

func alwaysBelowTarget() *fakeOpenRouter {
	return &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) {
			reply(w, fmt.Sprintf("gen-%d", n), "writer/free", `{"fruit":"apple"}`)
		},
		review: func(int) string { return `{"violations":[{"rule":"1","evidence":"apple","explanation":"Not yellow.","recommendedAction":"Use a yellow fruit.","badScore":1}],"totalBadScore":1}` },
	}
}

func TestEngineGenerateReviewed_whenEveryRoundBelowTarget_thenAttemptsExhausted(t *testing.T) {
	// Arrange
	fake := alwaysBelowTarget()
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	oneCorrection := reviewed
	oneCorrection.MaxCorrections = 1

	// Act
	_, err := engine.GenerateReviewed(context.Background(), oneCorrection)

	// Assert
	var exhausted *model.AttemptsExhaustedError
	if !errors.As(err, &exhausted) || len(exhausted.Attempts) != 2 {
		t.Fatalf("err = %v; want AttemptsExhaustedError with 2 below-target attempts", err)
	}
}

func TestEngineGenerateReviewed_whenMaxCorrectionsSet_thenGeneratesOncePlusCorrections(t *testing.T) {
	// Arrange
	fake := alwaysBelowTarget()
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	threeCorrections := reviewed
	threeCorrections.MaxCorrections = 3

	// Act
	engine.GenerateReviewed(context.Background(), threeCorrections)

	// Assert
	if len(fake.generations) != 4 {
		t.Fatalf("generations = %d; want 4", len(fake.generations))
	}
}

func TestEngineGenerateReviewed_whenMaxCorrectionsZero_thenGeneratesOnce(t *testing.T) {
	// Arrange
	fake := alwaysBelowTarget()
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	noCorrections := reviewed
	noCorrections.MaxCorrections = 0

	// Act
	engine.GenerateReviewed(context.Background(), noCorrections)

	// Assert
	if len(fake.generations) != 1 {
		t.Fatalf("generations = %d; want 1", len(fake.generations))
	}
}

func TestEngineValidateGenerateReviewed_whenMaxCorrectionsNegative_thenErrors(t *testing.T) {
	// Arrange
	negative := reviewed
	negative.MaxCorrections = -1

	// Act
	err := validateGenerateReviewed(negative)

	// Assert
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestEngineValidateGenerateReviewed_whenRulesEmpty_thenErrors(t *testing.T) {
	// Arrange
	ruleless := reviewed
	ruleless.Criteria.Rules = ""

	// Act
	err := validateGenerateReviewed(ruleless)

	// Assert
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestEngineGenerateReviewed_whenAttemptTimesOut_thenRecordsTimeoutAgainstPickedModel(t *testing.T) {
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
		review: noViolations,
	}
	_, settings := fake.serve(t)
	engine, path := newEngine(t, settings)
	if _, err := engine.GenerateReviewed(context.Background(), reviewed); err != nil {
		t.Fatalf("setup: GenerateReviewed failed: %v", err)
	}

	// Act
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	entries := readHistory(t, path)

	// Assert
	for _, e := range entries {
		if f := e.Fields(); f.GenerationID == "gen-slow" && f.Model == "slow/model:free" && f.Outcome == history.OutcomeTimeout {
			return
		}
	}
	t.Fatalf("history %+v has no timeout entry for slow/model:free", entries)
}

func TestEngineGenerateReviewed_whenOutputViolatesSchema_thenRecordsInvalidOutput(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) {
			if n == 1 {
				reply(w, "gen-bad", "sloppy/free", `{"vegetable":"carrot"}`)
				return
			}
			reply(w, "gen-2", "writer/free", `{"fruit":"banana"}`)
		},
		review: noViolations,
	}
	_, settings := fake.serve(t)
	engine, path := newEngine(t, settings)
	if _, err := engine.GenerateReviewed(context.Background(), reviewed); err != nil {
		t.Fatalf("setup: GenerateReviewed failed: %v", err)
	}

	// Act
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
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
		generate: func(w http.ResponseWriter, r *http.Request, n int) {
			reply(w, "gen-1", "writer/free", `{"fruit":"banana"}`)
		},
		review: noViolations,
	}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	got, err := engine.GenerateReviewed(context.Background(), reviewed)
	if err != nil {
		t.Fatalf("setup: GenerateReviewed failed: %v", err)
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

func TestEngineGenerateReviewed_whenProviderRejectsIn200Body_thenFirstAttemptResendsAndWins(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: overloadedFirst, review: noViolations}
	_, settings := fake.serve(t)
	settings.MaxAttempts = 1
	engine, _ := newEngine(t, settings)

	// Act
	got, err := engine.GenerateReviewed(context.Background(), reviewed)

	// Assert
	if err != nil || got.GenerationID != "gen-2" {
		t.Fatalf("got %+v, %v; want the single attempt to resend and return gen-2", got, err)
	}
}

func TestEngineGenerateReviewed_whenProviderRejectsIn200Body_thenRecordsNothingForIt(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: overloadedFirst, review: noViolations}
	_, settings := fake.serve(t)
	engine, path := newEngine(t, settings)
	if _, err := engine.GenerateReviewed(context.Background(), reviewed); err != nil {
		t.Fatalf("setup: GenerateReviewed failed: %v", err)
	}

	// Act
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
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

func TestEngineGenerateReviewed_whenRateLimited_thenRecordsFailureAgainstPickedModel(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: rateLimitedFirst, review: noViolations}
	_, settings := fake.serve(t)
	engine, path := newEngine(t, settings)
	if _, err := engine.GenerateReviewed(context.Background(), reviewed); err != nil {
		t.Fatalf("setup: GenerateReviewed failed: %v", err)
	}

	// Act
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
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

func TestEngineGenerateReviewed_whenModelRefusesAndAnotherAttemptWins_thenRecordsRefusalAgainstPickedModel(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: refusedFirst, review: noViolations}
	_, settings := fake.serve(t)
	engine, path := newEngine(t, settings)
	if _, err := engine.GenerateReviewed(context.Background(), reviewed); err != nil {
		t.Fatalf("setup: GenerateReviewed failed: %v", err)
	}

	// Act
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	entries := readHistory(t, path)

	// Assert
	for _, e := range entries {
		if f := e.Fields(); f.Model == pickedModel && f.Outcome == history.OutcomeRefused && f.Quality == model.QualityUnusable {
			return
		}
	}
	t.Fatalf("history %+v has no unusable refused entry for %s", entries, pickedModel)
}

func TestEngineGenerateReviewed_whenEveryAttemptRefused_thenReturnsRefusedAttempts(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: alwaysRefused, review: noViolations}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	_, err := engine.GenerateReviewed(context.Background(), reviewed)

	// Assert
	var exhausted *model.AttemptsExhaustedError
	if !errors.As(err, &exhausted) || len(exhausted.Attempts) != settings.MaxAttempts {
		t.Fatalf("err = %v, want AttemptsExhaustedError with %d attempts", err, settings.MaxAttempts)
	}
	for _, a := range exhausted.Attempts {
		if a.Outcome != model.OutcomeRefused {
			t.Fatalf("attempt %+v, want outcome refused", a)
		}
	}
}

func TestEngineGenerateReviewed_whenEveryAttemptRefused_thenRecordsNoRefusal(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: alwaysRefused, review: noViolations}
	_, settings := fake.serve(t)
	engine, path := newEngine(t, settings)
	if _, err := engine.GenerateReviewed(context.Background(), reviewed); err == nil {
		t.Fatal("setup: GenerateReviewed succeeded, want every attempt refused")
	}

	// Act
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	_, statErr := os.Stat(path)

	// Assert
	if !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("history %+v was written, want nothing recorded", readHistory(t, path))
	}
}

func TestEngineGenerate_whenCalled_thenNeverSendsReview(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) {
			reply(w, "gen-1", "writer/free", `{"fruit":"banana"}`)
		},
		review: func(int) string { t.Error("review requested"); return `{"violations":[],"totalBadScore":0}` },
	}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	got, err := engine.Generate(context.Background(), unreviewed)

	// Assert
	if err != nil || got.GenerationID != "gen-1" || len(fake.reviews) != 0 {
		t.Fatalf("got %+v, %v, %d reviews; want unreviewed content", got, err, len(fake.reviews))
	}
}

func TestEngineGenerateReviewed_whenEveryOutputViolatesSchema_thenNeverSendsReview(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) {
			reply(w, fmt.Sprintf("gen-%d", n), "sloppy/free", `{"vegetable":"carrot"}`)
		},
		review: func(int) string { t.Error("review requested"); return `{"violations":[],"totalBadScore":0}` },
	}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	_, err := engine.GenerateReviewed(context.Background(), reviewed)

	// Assert
	var exhausted *model.AttemptsExhaustedError
	if !errors.As(err, &exhausted) || len(fake.reviews) != 0 {
		t.Fatalf("err=%v reviews=%d; want AttemptsExhaustedError and no review", err, len(fake.reviews))
	}
}

func TestEngineGenerate_whenWon_thenRecordsGenerationAsHigh(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) {
			reply(w, "gen-1", "writer/free", `{"fruit":"banana"}`)
		},
		review: noViolations,
	}
	_, settings := fake.serve(t)
	engine, path := newEngine(t, settings)
	if _, err := engine.Generate(context.Background(), unreviewed); err != nil {
		t.Fatalf("setup: Generate failed: %v", err)
	}

	// Act
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	entries := readHistory(t, path)

	// Assert
	for _, e := range entries {
		if f := e.Fields(); f.GenerationID == "gen-1" && f.Quality == model.QualityHigh {
			return
		}
	}
	t.Fatalf("history %+v has no high entry for gen-1", entries)
}

func TestEngineGenerateReviewed_whenPaidTier_thenAsksOnlyTheCheapPaidModel(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) {
			reply(w, "gen-1", "cheap/model", `{"fruit":"banana"}`)
		},
		review: noViolations,
	}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	paid := reviewed
	paid.GenerationModels = model.ModelSelection{Tier: model.ModelTierPaid}

	// Act
	_, err := engine.GenerateReviewed(context.Background(), paid)

	// Assert
	if err != nil || len(fake.generations) != 1 || fake.generations[0].Model != "cheap/model" {
		t.Fatalf("err=%v generations=%+v; want one request to cheap/model", err, fake.generations)
	}
}

func TestEngineGenerateReviewed_whenCheapestModelContextTooSmall_thenNeverAsksIt(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) {
			reply(w, "gen-1", "cheap/model", `{"fruit":"banana"}`)
		},
		review: noViolations,
	}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	paid := reviewed
	paid.GenerationModels = model.ModelSelection{Tier: model.ModelTierPaid}

	// Act
	_, err := engine.GenerateReviewed(context.Background(), paid)

	// Assert
	if err != nil || len(fake.generations) != 1 || fake.generations[0].Model == "tiny/model" {
		t.Fatalf("err=%v generations=%+v; want one request, not to tiny/model", err, fake.generations)
	}
}

func bananaFrom(modelID string) func(w http.ResponseWriter, r *http.Request, n int) {
	return func(w http.ResponseWriter, r *http.Request, n int) {
		reply(w, fmt.Sprintf("gen-%d", n), modelID, `{"fruit":"banana"}`)
	}
}

func TestEngineGenerateReviewed_whenGenerationModelsDenied_thenAsksOnlyTheRest(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: bananaFrom("pricey/model"), review: noViolations}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	denying := reviewed
	denying.GenerationModels = model.ModelSelection{Denied: []string{pickedModel, "cheap/model"}}

	// Act
	_, err := engine.GenerateReviewed(context.Background(), denying)

	// Assert
	if err != nil || len(fake.generations) != 1 || fake.generations[0].Model != "pricey/model" {
		t.Fatalf("err=%v generations=%+v; want one request to pricey/model, the only undenied model with enough context", err, fake.generations)
	}
}

func TestEngineGenerateReviewed_whenGenerationModelsAllowed_thenAsksOnlyThem(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: bananaFrom("cheap/model"), review: noViolations}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	allowing := reviewed
	allowing.GenerationModels = model.ModelSelection{Allowed: []string{"cheap/model"}}

	// Act
	_, err := engine.GenerateReviewed(context.Background(), allowing)

	// Assert
	if err != nil || len(fake.generations) != 1 || fake.generations[0].Model != "cheap/model" {
		t.Fatalf("err=%v generations=%+v; want one request to cheap/model", err, fake.generations)
	}
}

func TestEngineGenerateReviewed_whenReviewModelsAllowed_thenReviewerAsksOnlyThem(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: bananaFrom(pickedModel), review: noViolations}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	allowing := reviewed
	allowing.ReviewModels = model.ModelSelection{Allowed: []string{"pricey/model"}}

	// Act
	_, err := engine.GenerateReviewed(context.Background(), allowing)

	// Assert
	if err != nil || len(fake.reviews) != 1 || fake.reviews[0].Model != "pricey/model" {
		t.Fatalf("err=%v reviews=%+v; want one review request to pricey/model", err, fake.reviews)
	}
}

func TestEngineGenerateReviewed_whenOnlyGenerationModelsAllowed_thenReviewerKeepsItsOwnSelection(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: bananaFrom("cheap/model"), review: noViolations}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	allowing := reviewed
	allowing.GenerationModels = model.ModelSelection{Allowed: []string{"cheap/model"}}

	// Act
	_, err := engine.GenerateReviewed(context.Background(), allowing)

	// Assert
	if err != nil || len(fake.reviews) != 1 || fake.reviews[0].Model != pickedModel {
		t.Fatalf("err=%v reviews=%+v; want one review request to the free tier's %s", err, fake.reviews, pickedModel)
	}
}

func TestEngineCandidateModels_whenDenied_thenListsOnlyTheCheapestOfTheRest(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	denying := unreviewed
	denying.Models = model.ModelSelection{Denied: []string{pickedModel}}

	// Act
	candidates, err := engine.CandidateModels(denying)

	// Assert
	if err != nil || !reflect.DeepEqual(candidates, []string{"cheap/model"}) {
		t.Fatalf("candidates=%v, %v; want [cheap/model]", candidates, err)
	}
}

func TestEngineCandidateModels_whenAllowed_thenListsOnlyTheCheapestOfThem(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	allowing := unreviewed
	allowing.Models = model.ModelSelection{Allowed: []string{"cheap/model", "pricey/model"}}

	// Act
	candidates, err := engine.CandidateModels(allowing)

	// Assert
	if err != nil || !reflect.DeepEqual(candidates, []string{"cheap/model"}) {
		t.Fatalf("candidates=%v, %v; want [cheap/model]", candidates, err)
	}
}

func TestEngineCandidateModels_whenPaidTier_thenListsOnlyTheCheapest(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	paid := unreviewed
	paid.Models = model.ModelSelection{Tier: model.ModelTierPaid}

	// Act
	candidates, err := engine.CandidateModels(paid)

	// Assert
	if err != nil || !reflect.DeepEqual(candidates, []string{"cheap/model"}) {
		t.Fatalf("candidates=%v, %v; want [cheap/model]", candidates, err)
	}
}

func TestEngineCandidateModels_whenListing_thenSendsNoChatRequest(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	if _, err := engine.CandidateModels(unreviewed); err != nil {
		t.Fatal(err)
	}

	// Assert
	if len(fake.generations)+len(fake.reviews) != 0 {
		t.Fatalf("generations=%+v reviews=%+v; want no chat requests", fake.generations, fake.reviews)
	}
}

func TestEngineGenerateReviewed_whenAllowedModelNotInCatalog_thenReturnsUnexpectedError(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: bananaFrom(pickedModel), review: noViolations}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	unknown := reviewed
	unknown.GenerationModels = model.ModelSelection{Allowed: []string{"missing/model:free"}}

	// Act
	_, err := engine.GenerateReviewed(context.Background(), unknown)

	// Assert
	var unexpected *model.UnexpectedError
	if !errors.As(err, &unexpected) {
		t.Fatalf("err = %v; want *model.UnexpectedError", err)
	}
}

func TestEngineGenerate_whenDeniedModelNotInCatalog_thenReturnsUnexpectedError(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: bananaFrom(pickedModel), review: noViolations}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	unknown := unreviewed
	unknown.Models = model.ModelSelection{Denied: []string{"missing/model:free"}}

	// Act
	_, err := engine.Generate(context.Background(), unknown)

	// Assert
	var unexpected *model.UnexpectedError
	if !errors.As(err, &unexpected) {
		t.Fatalf("err = %v; want *model.UnexpectedError", err)
	}
}

func TestEngineGenerate_whenEveryModelDenied_thenReturnsUnexpectedError(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: bananaFrom(pickedModel), review: noViolations}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	allDenied := unreviewed
	allDenied.Models = model.ModelSelection{Denied: []string{pickedModel, "tiny/model", "cheap/model", "pricey/model"}}

	// Act
	_, err := engine.Generate(context.Background(), allDenied)

	// Assert
	var unexpected *model.UnexpectedError
	if !errors.As(err, &unexpected) {
		t.Fatalf("err = %v; want *model.UnexpectedError", err)
	}
}

func TestEngineValidateSelection_whenTierAndDeniedBothSet_thenErrors(t *testing.T) {
	// Arrange
	both := model.ModelSelection{Tier: model.ModelTierPaid, Denied: []string{"cheap/model"}}

	// Act
	err := validateSelection("Models", both)

	// Assert
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestEngineCandidateModels_whenSelectionEmpty_thenListsOnlyTheCheapestOfEveryModel(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	everyModel := unreviewed
	everyModel.Models = model.ModelSelection{}

	// Act
	candidates, err := engine.CandidateModels(everyModel)

	// Assert
	if err != nil || !reflect.DeepEqual(candidates, []string{pickedModel}) {
		t.Fatalf("candidates=%v, %v; want [%s], the only one priced near the cheapest", candidates, err, pickedModel)
	}
}

func TestEngineValidateGenerateReviewed_whenThresholdZero_thenErrors(t *testing.T) {
	// Arrange
	unthresholded := reviewed
	unthresholded.Criteria.ToleranceThreshold = 0

	// Act
	err := validateGenerateReviewed(unthresholded)

	// Assert
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestEngineValidateGenerate_whenTargetQualityMissing_thenErrors(t *testing.T) {
	// Arrange
	untargeted := unreviewed
	untargeted.TargetQuality = ""

	// Act
	err := validateGenerate(untargeted)

	// Assert
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestEngineGenerateReviewed_whenBadScoreWithinThreshold_thenRatesMedium(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) {
			reply(w, "gen-1", "writer/free", `{"fruit":"lemon"}`)
		},
		review: func(int) string {
			return `{"violations":[{"rule":"1","evidence":"apple","explanation":"Lemons are pale.","recommendedAction":"Use a yellow fruit.","badScore":3}],"totalBadScore":3}`
		},
	}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	mediumTarget := reviewed
	mediumTarget.TargetQuality = model.QualityMedium

	// Act
	got, err := engine.GenerateReviewed(context.Background(), mediumTarget)

	// Assert
	if err != nil || got.Review.Quality != model.QualityMedium {
		t.Fatalf("got %+v, %v; want medium quality", got, err)
	}
}

// correctedOnce flags the first output, so the run holds a correction and a
// second review.
func correctedOnce(t *testing.T) *fakeOpenRouter {
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) {
			reply(w, fmt.Sprintf("gen-%d", n), "writer/free", fmt.Sprintf(`{"fruit":"apple-%d"}`, n))
		},
		review: func(n int) string {
			if n == 1 {
				return `{"violations":[{"rule":"1","evidence":"apple","explanation":"Apples are not yellow.","recommendedAction":"Use a yellow fruit.","badScore":1}],"totalBadScore":1}`
			}
			return `{"violations":[],"totalBadScore":0}`
		},
	}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	if _, err := engine.GenerateReviewed(context.Background(), reviewed); err != nil {
		t.Fatalf("setup: GenerateReviewed failed: %v", err)
	}
	return fake
}

func roles(req chatRequest) []string {
	var got []string
	for _, m := range req.Messages {
		got = append(got, m.Role)
	}
	return got
}

func TestEngineGenerateReviewed_whenReviewing_thenSendsSystemUserUserMessages(t *testing.T) {
	// Arrange
	fake := correctedOnce(t)

	// Act
	got := roles(fake.reviews[0])

	// Assert
	if strings.Join(got, ",") != "system,user,user" {
		t.Fatalf("review roles = %v; want system, user, user", got)
	}
}

func TestEngineGenerateReviewed_whenReviewing_thenNeverSendsAssistantMessage(t *testing.T) {
	// Arrange
	fake := correctedOnce(t)

	// Act
	var assistantTurns int
	for _, req := range fake.reviews {
		for _, role := range roles(req) {
			if role == "assistant" {
				assistantTurns++
			}
		}
	}

	// Assert
	if len(fake.reviews) != 2 || assistantTurns != 0 {
		t.Fatalf("%d reviews carry %d assistant messages; want 2 reviews with none", len(fake.reviews), assistantTurns)
	}
}

func TestEngineGenerateReviewed_whenReviewing_thenSystemMessageCarriesValidationRules(t *testing.T) {
	// Arrange
	fake := correctedOnce(t)

	// Act
	system := fake.reviews[0].Messages[0].Content

	// Assert
	if !strings.Contains(system, reviewed.Criteria.Rules) {
		t.Fatalf("system message %q lacks the validation rules", system)
	}
}

func TestEngineGenerateReviewed_whenReviewing_thenUserMessagesCarryPromptThenOutput(t *testing.T) {
	// Arrange
	fake := correctedOnce(t)

	// Act
	messages := fake.reviews[1].Messages

	// Assert
	if !strings.Contains(messages[1].Content, reviewed.Prompt) || !strings.Contains(messages[2].Content, `{"fruit":"apple-2"}`) {
		t.Fatalf("review messages %+v; want the prompt, then the second generation's output", messages)
	}
}

func TestEngineGenerateReviewed_whenCorrecting_thenGeneratorContinuesItsOwnConversation(t *testing.T) {
	// Arrange
	fake := correctedOnce(t)

	// Act
	correction := fake.generations[1]

	// Assert
	if strings.Join(roles(correction), ",") != "user,assistant,user" || correction.Messages[0].Content != reviewed.Prompt || correction.Messages[1].Content != `{"fruit":"apple-1"}` {
		t.Fatalf("correction messages %+v; want the prompt, the first output as assistant, then the violations", correction.Messages)
	}
}

func TestEngineGenerateReviewed_whenReviewWins_thenReviewerEntryLinksReviewedGeneration(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) {
			reply(w, "gen-1", "writer/free", `{"fruit":"banana"}`)
		},
		review: noViolations,
	}
	_, settings := fake.serve(t)
	engine, path := newEngine(t, settings)
	if _, err := engine.GenerateReviewed(context.Background(), reviewed); err != nil {
		t.Fatalf("setup: GenerateReviewed failed: %v", err)
	}

	// Act
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	entries := readHistory(t, path)

	// Assert
	for _, e := range entries {
		if f := e.Fields(); f.GenerationID == "gen-review-1" && f.ReviewedGenerationID == "gen-1" {
			return
		}
	}
	t.Fatalf("history = %+v; want the reviewer entry linked to gen-1", entries)
}

func TestEngineGenerateReviewed_whenRoundBelowTarget_thenFailedAttemptCarriesReviewGenerationID(t *testing.T) {
	// Arrange
	fake := alwaysBelowTarget()
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	oneCorrection := reviewed
	oneCorrection.MaxCorrections = 1

	// Act
	_, err := engine.GenerateReviewed(context.Background(), oneCorrection)

	// Assert
	var exhausted *model.AttemptsExhaustedError
	if !errors.As(err, &exhausted) || exhausted.Attempts[0].Review.GenerationID != "gen-review-1" {
		t.Fatalf("err = %v; want the first below-target attempt to name gen-review-1", err)
	}
}

func TestEngineGenerateReviewed_whenRoundBelowTarget_thenFailedAttemptCarriesRejectedContent(t *testing.T) {
	// Arrange
	fake := alwaysBelowTarget()
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	oneCorrection := reviewed
	oneCorrection.MaxCorrections = 1

	// Act
	_, err := engine.GenerateReviewed(context.Background(), oneCorrection)

	// Assert
	var exhausted *model.AttemptsExhaustedError
	if !errors.As(err, &exhausted) || string(exhausted.Attempts[0].Content) != `{"fruit":"apple"}` {
		t.Fatalf("err = %v; want the first below-target attempt to carry the rejected output", err)
	}
}

func TestEngineGenerateReviewed_whenRoundBelowTarget_thenFailedAttemptCarriesReviewVerdict(t *testing.T) {
	// Arrange
	fake := alwaysBelowTarget()
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	oneCorrection := reviewed
	oneCorrection.MaxCorrections = 1

	// Act
	_, err := engine.GenerateReviewed(context.Background(), oneCorrection)

	// Assert
	var exhausted *model.AttemptsExhaustedError
	want := model.ReviewVerdict{Violations: []model.ReviewViolation{{Rule: "1", Evidence: "apple", Explanation: "Not yellow.", RecommendedAction: "Use a yellow fruit.", BadScore: 1}}, TotalBadScore: 1}
	if !errors.As(err, &exhausted) || !reflect.DeepEqual(exhausted.Attempts[0].Review.Verdict, want) {
		t.Fatalf("err = %v; want the first below-target attempt to carry the rejecting verdict", err)
	}
}

func TestEngineGenerateReviewed_whenEveryOutputIsNotJSON_thenReasonOmitsReply(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) {
			reply(w, fmt.Sprintf("gen-%d", n), "sloppy/free", `{"status": "LGTM"`)
		},
		review: noViolations,
	}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	_, err := engine.GenerateReviewed(context.Background(), reviewed)

	// Assert
	var exhausted *model.AttemptsExhaustedError
	if !errors.As(err, &exhausted) || strings.Contains(exhausted.Attempts[0].Reason, "LGTM") {
		t.Fatalf("err = %v; want a reason that names the saved reply instead of quoting it", err)
	}
}

func TestEngineGenerateReviewed_whenEveryOutputIsNotJSON_thenReasonNamesFileHoldingReply(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) {
			reply(w, fmt.Sprintf("gen-%d", n), "sloppy/free", `{"status": "LGTM"`)
		},
		review: noViolations,
	}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	_, err := engine.GenerateReviewed(context.Background(), reviewed)

	// Assert
	var exhausted *model.AttemptsExhaustedError
	if !errors.As(err, &exhausted) {
		t.Fatalf("err = %v; want AttemptsExhaustedError", err)
	}
	_, path, _ := strings.Cut(exhausted.Attempts[0].Reason, "body saved to ")
	saved, readErr := os.ReadFile(path)
	if readErr != nil || !strings.Contains(string(saved), "LGTM") {
		t.Fatalf("reason %q names %q, which holds %q (%v); want the raw reply", exhausted.Attempts[0].Reason, path, saved, readErr)
	}
}

func TestEngineGenerateReviewed_whenRoundBelowTarget_thenReasonNamesFileHoldingRejectedOutput(t *testing.T) {
	// Arrange
	fake := alwaysBelowTarget()
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	_, err := engine.GenerateReviewed(context.Background(), reviewed)

	// Assert
	var exhausted *model.AttemptsExhaustedError
	if !errors.As(err, &exhausted) {
		t.Fatalf("err = %v; want AttemptsExhaustedError", err)
	}
	_, rest, _ := strings.Cut(exhausted.Attempts[0].Reason, "rejected output body saved to ")
	path, _, _ := strings.Cut(rest, ";")
	saved, readErr := os.ReadFile(path)
	if readErr != nil || string(saved) != `{"fruit":"apple"}` {
		t.Fatalf("reason %q names %q, which holds %q (%v); want the rejected output", exhausted.Attempts[0].Reason, path, saved, readErr)
	}
}

func TestEngineGenerateReviewed_whenRoundBelowTarget_thenReasonNamesFileHoldingReview(t *testing.T) {
	// Arrange
	fake := alwaysBelowTarget()
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	_, err := engine.GenerateReviewed(context.Background(), reviewed)

	// Assert
	var exhausted *model.AttemptsExhaustedError
	if !errors.As(err, &exhausted) {
		t.Fatalf("err = %v; want AttemptsExhaustedError", err)
	}
	_, path, _ := strings.Cut(exhausted.Attempts[0].Reason, "review body saved to ")
	saved, readErr := os.ReadFile(path)
	if readErr != nil || !strings.Contains(string(saved), "Not yellow.") {
		t.Fatalf("reason %q names %q, which holds %q (%v); want the review", exhausted.Attempts[0].Reason, path, saved, readErr)
	}
}

func TestEngineGenerateReviewed_whenRepliesDisabledAndRoundBelowTarget_thenReasonNamesGenerationID(t *testing.T) {
	// Arrange
	fake := alwaysBelowTarget()
	_, settings := fake.serve(t)
	engine := New(settings, "key", testTag, testTimeout, history.Disabled{}, replyfile.Disabled(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() {
		if err := engine.Close(); err != nil {
			t.Error(err)
		}
	})

	// Act
	_, err := engine.GenerateReviewed(context.Background(), reviewed)

	// Assert
	var exhausted *model.AttemptsExhaustedError
	if !errors.As(err, &exhausted) {
		t.Fatalf("err = %v; want AttemptsExhaustedError", err)
	}
	if reason := exhausted.Attempts[0].Reason; !strings.Contains(reason, "generation id "+exhausted.Attempts[0].GenerationID) {
		t.Fatalf("reason = %q, want it to name generation id %s", reason, exhausted.Attempts[0].GenerationID)
	}
}

func unauthorized(w http.ResponseWriter, r *http.Request, n int) {
	w.WriteHeader(http.StatusUnauthorized)
	fmt.Fprint(w, `{"error":{"message":"No auth credentials found","code":401}}`)
}

func TestEngineGenerateReviewed_whenChatReturnsUnexpectedStatus_thenReturnsUnexpectedError(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: unauthorized, review: noViolations}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	_, err := engine.GenerateReviewed(context.Background(), reviewed)

	// Assert
	var unexpected *model.UnexpectedError
	if !errors.As(err, &unexpected) {
		t.Fatalf("err = %v; want *model.UnexpectedError", err)
	}
}

func TestEngineGenerateReviewed_whenChatReturnsUnexpectedStatus_thenSendsNoFurtherAttempt(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: unauthorized, review: noViolations}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	engine.GenerateReviewed(context.Background(), reviewed)

	// Assert
	if len(fake.generations) != 1 {
		t.Fatalf("generations = %d; want the race aborted after the first", len(fake.generations))
	}
}

func entryTagged(t *testing.T, path, generationID string) string {
	t.Helper()
	for _, e := range readHistory(t, path) {
		if f := e.Fields(); f.GenerationID == generationID {
			return f.Tag
		}
	}
	t.Fatalf("history has no entry for %s", generationID)
	return ""
}

func TestEngineGenerateReviewed_whenRecorded_thenGeneratorEntryTaggedGenerate(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: bananaFrom(pickedModel), review: noViolations}
	_, settings := fake.serve(t)
	engine, path := newEngine(t, settings)
	if _, err := engine.GenerateReviewed(context.Background(), reviewed); err != nil {
		t.Fatalf("setup: GenerateReviewed failed: %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}

	// Act
	tag := entryTagged(t, path, "gen-1")

	// Assert
	if tag != testTag+"-generate" {
		t.Fatalf("tag = %q, want %q", tag, testTag+"-generate")
	}
}

func TestEngineGenerateReviewed_whenRecorded_thenReviewerEntryTaggedReview(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: bananaFrom(pickedModel), review: noViolations}
	_, settings := fake.serve(t)
	engine, path := newEngine(t, settings)
	if _, err := engine.GenerateReviewed(context.Background(), reviewed); err != nil {
		t.Fatalf("setup: GenerateReviewed failed: %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}

	// Act
	tag := entryTagged(t, path, "gen-review-1")

	// Assert
	if tag != testTag+"-review" {
		t.Fatalf("tag = %q, want %q", tag, testTag+"-review")
	}
}

var reviewRequest = model.ReviewRequest{
	Task:     "Name a fruit.",
	Content:  json.RawMessage(`{"fruit":"banana"}`),
	Criteria: model.ReviewCriteria{Rules: "1. The fruit must be yellow.", ToleranceThreshold: 3},
	Models:   freeTier,
}

func neverGenerates(t *testing.T) func(w http.ResponseWriter, r *http.Request, n int) {
	return func(w http.ResponseWriter, r *http.Request, n int) {
		t.Error("generation requested")
		reply(w, "gen-unexpected", pickedModel, `{"fruit":"banana"}`)
	}
}

func TestEngineReview_whenNoViolations_thenReturnsHighQuality(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: neverGenerates(t), review: noViolations}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	got, err := engine.Review(context.Background(), reviewRequest)

	// Assert
	if err != nil || got.Quality != model.QualityHigh || got.GenerationID != "gen-review-1" {
		t.Fatalf("got %+v, %v; want a high review gen-review-1", got, err)
	}
}

func TestEngineReview_whenBadScoreAboveThreshold_thenReturnsLowQualityWithoutError(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: neverGenerates(t),
		review: func(int) string {
			return `{"violations":[{"rule":"1","evidence":"banana","explanation":"x","recommendedAction":"y","badScore":4}],"totalBadScore":4}`
		},
	}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	got, err := engine.Review(context.Background(), reviewRequest)

	// Assert
	if err != nil || got.Quality != model.QualityLow {
		t.Fatalf("got %+v, %v; want a low review and no error", got, err)
	}
}

func TestEngineReview_whenReviewing_thenUserMessagesCarryTaskThenContent(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: neverGenerates(t), review: noViolations}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	if _, err := engine.Review(context.Background(), reviewRequest); err != nil {
		t.Fatalf("setup: Review failed: %v", err)
	}

	// Act
	messages := fake.reviews[0].Messages

	// Assert
	if !strings.Contains(messages[1].Content, reviewRequest.Task) || !strings.Contains(messages[2].Content, string(reviewRequest.Content)) {
		t.Fatalf("review messages %+v; want the task, then the content", messages)
	}
}

func TestEngineReview_whenRecorded_thenReviewerEntryLinksNoGeneration(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: neverGenerates(t), review: noViolations}
	_, settings := fake.serve(t)
	engine, path := newEngine(t, settings)
	if _, err := engine.Review(context.Background(), reviewRequest); err != nil {
		t.Fatalf("setup: Review failed: %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}

	// Act
	entries := readHistory(t, path)

	// Assert
	if len(entries) != 1 || entries[0].Fields().Role != history.RoleReviewer || entries[0].Fields().ReviewedGenerationID != "" {
		t.Fatalf("history = %+v; want one reviewer entry linked to no generation", entries)
	}
}

func TestEngineReview_whenContentNotJSON_thenReturnsUnexpectedError(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: neverGenerates(t), review: noViolations}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	notJSON := reviewRequest
	notJSON.Content = json.RawMessage(`banana`)

	// Act
	_, err := engine.Review(context.Background(), notJSON)

	// Assert
	var unexpected *model.UnexpectedError
	if !errors.As(err, &unexpected) {
		t.Fatalf("err = %v; want *model.UnexpectedError", err)
	}
}
