package catalog

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/Darida/openrouterclient/src/internal/replyfile"
	"github.com/Darida/openrouterclient/src/model"
)

const catalogBody = `{"data":[
  {"id":"liquid/lfm-2.5-2.6b:free","context_length":32768,"supported_parameters":["structured_outputs"],"architecture":{"output_modalities":["text"]},"pricing":{"prompt":"0","completion":"0"}},
  {"id":"liquid/lfm-2.5-2.6b","context_length":32768,"supported_parameters":["structured_outputs"],"architecture":{"output_modalities":["text"]},"pricing":{"prompt":"0.0000001","completion":"0.0000002"}},
  {"id":"openai/gpt-6-luna-pro:batch","context_length":32768,"supported_parameters":["structured_outputs"],"architecture":{"output_modalities":["text"]},"pricing":{"prompt":"0.00000005","completion":"0.0000001"}},
  {"id":"openrouter/auto","context_length":32768,"supported_parameters":["structured_outputs"],"architecture":{"output_modalities":["text"]},"pricing":{"prompt":"-1","completion":"-1"}},
  {"id":"nvidia/nemotron-3.5-lightning:free","context_length":32768,"supported_parameters":["tools"],"architecture":{"output_modalities":["text"]},"pricing":{"prompt":"0","completion":"0"}}
]}`

func newCatalog(t *testing.T) (*Catalog, *atomic.Int32) {
	var fetches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		w.Write([]byte(catalogBody))
	}))
	t.Cleanup(server.Close)
	return &Catalog{URL: server.URL, HTTP: server.Client(), Replies: replyfile.Disabled()}, &fetches
}

func candidateIDs(models []Model) []string {
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	return ids
}

func TestCatalogCandidates_whenFreeTier_thenOnlyFreeWithStructuredOutputs(t *testing.T) {
	// Arrange
	c, _ := newCatalog(t)

	// Act
	got := candidateIDs(c.Candidates(model.ModelTierFree))

	// Assert
	if !slices.Equal(got, []string{"liquid/lfm-2.5-2.6b:free"}) {
		t.Fatalf("got %v", got)
	}
}

func TestCatalogCandidates_whenPaidTier_thenSkipsFreeRoutersAndBatchVariants(t *testing.T) {
	// Arrange
	c, _ := newCatalog(t)

	// Act
	got := candidateIDs(c.Candidates(model.ModelTierPaid))

	// Assert
	if !slices.Equal(got, []string{"liquid/lfm-2.5-2.6b"}) {
		t.Fatalf("got %v", got)
	}
}

func TestCatalogFreeStructuredModels_whenCalledTwiceWithinTTL_thenFetchesOnce(t *testing.T) {
	// Arrange
	c, fetches := newCatalog(t)
	c.Candidates(model.ModelTierFree)

	// Act
	c.Candidates(model.ModelTierFree)

	// Assert
	if fetches.Load() != 1 {
		t.Fatalf("fetched %d times, want 1", fetches.Load())
	}
}

func TestCatalogCandidates_whenContextLengthMissing_thenPanics(t *testing.T) {
	// Arrange
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"id":"liquid/lfm-2.5-2.6b:free","supported_parameters":["structured_outputs"],"architecture":{"output_modalities":["text"]},"pricing":{"prompt":"0","completion":"0"}}]}`))
	}))
	t.Cleanup(server.Close)
	c := &Catalog{URL: server.URL, HTTP: server.Client(), Replies: replyfile.Disabled()}
	defer func() {
		// Assert
		if recover() == nil {
			t.Fatal("want panic")
		}
	}()

	// Act
	c.Candidates(model.ModelTierFree)
}
