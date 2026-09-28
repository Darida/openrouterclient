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

	"github.com/Darida/openrouterclient/src/internal/generationlog"
	"github.com/Darida/openrouterclient/src/internal/hedge"
	"github.com/Darida/openrouterclient/src/internal/history"
	"github.com/Darida/openrouterclient/src/model"
)

var requirements = model.TextGenerationRequirements{
	Prompt:            "Name a fruit.",
	ReviewRulesPrompt: "1. The fruit must be yellow.",
	OutputSchema: model.JSONSchema{
		Name:   "fruit",
		Schema: json.RawMessage(`{"type":"object","properties":{"fruit":{"type":"string"}},"required":["fruit"],"additionalProperties":false}`),
	},
	TargetQuality: model.QualityHigh,
}

// chatRequest is the part of a chat payload the fake server routes on.
type chatRequest struct {
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
	logLookups  int
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
	mux.HandleFunc("/generation", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.logLookups++
		f.mu.Unlock()
		fmt.Fprint(w, `{"data":{"model":"slow/model-20260101:free"}}`)
	})
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"slow/model:free","canonical_slug":"slow/model-20260101"}]}`)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	settings := Settings{
		ChatURL:          server.URL + "/chat",
		GenerationLogURL: server.URL + "/generation",
		CatalogURL:       server.URL + "/models",
		Hedge:            hedge.Timing{MaxAttempts: 2, Stagger: 5 * time.Second, AbortGrace: 50 * time.Millisecond, AttemptTimeout: 200 * time.Millisecond},
		GenerationLog:    generationlog.Timing{Window: 500 * time.Millisecond, PollInterval: 10 * time.Millisecond},
		MaxRounds:        3,
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
	if err != nil || got.Review.Quality != model.QualityHigh || got.Model != "writer/free" {
		t.Fatalf("got %+v, %v; want high quality from writer/free", got, err)
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

func TestEngineGenerateText_whenAttemptTimesOut_thenRecordsModelFromGenerationLog(t *testing.T) {
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

func TestEngineGenerateText_whenProviderErrorIn200Body_thenRecordsFailureAgainstLogModel(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{
		generate: func(w http.ResponseWriter, r *http.Request, n int) {
			if n == 1 {
				w.Header().Set("X-Generation-Id", "gen-overloaded")
				fmt.Fprint(w, `{"id":"gen-overloaded","error":{"message":"Upstream error: overloaded","code":503,"metadata":{"error_type":"provider_overloaded"}}}`)
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
		if f := e.Fields(); f.GenerationID == "gen-overloaded" && f.Model == "slow/model:free" && f.Outcome == history.OutcomeFailed {
			return
		}
	}
	t.Fatalf("history %+v has no failed entry for gen-overloaded against slow/model:free", entries)
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

func TestEngineGenerateText_whenRateLimited_thenRecordsFailureAgainstModelInMessage(t *testing.T) {
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
		if f := e.Fields(); f.GenerationID == "gen-limited" && f.Model == "qwen/qwen3.8-27b:free" && f.Outcome == history.OutcomeFailed {
			return
		}
	}
	t.Fatalf("history %+v has no failed entry for gen-limited against qwen/qwen3.8-27b:free", entries)
}

func TestEngineGenerateText_whenRateLimited_thenNeverPollsGenerationLog(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{generate: rateLimitedFirst, review: noNotes}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	if _, err := engine.GenerateText(context.Background(), requirements); err != nil {
		t.Fatalf("GenerateText failed: %v", err)
	}

	// Assert
	if fake.logLookups != 0 {
		t.Fatalf("generation log polled %d times; want 0", fake.logLookups)
	}
}
