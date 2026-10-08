package catalog

import (
	"fmt"
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

func newCatalogServing(t *testing.T, body string) *Catalog {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return &Catalog{URL: server.URL, HTTP: server.Client(), Replies: replyfile.Disabled()}
}

func mustCandidates(t *testing.T, c *Catalog, tier model.ModelTier) []Model {
	t.Helper()
	models, err := c.Candidates(tier)
	if err != nil {
		t.Fatal(err)
	}
	return models
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
	got := candidateIDs(mustCandidates(t, c, model.ModelTierFree))

	// Assert
	if !slices.Equal(got, []string{"liquid/lfm-2.5-2.6b:free"}) {
		t.Fatalf("got %v", got)
	}
}

func TestCatalogCandidates_whenPaidTier_thenSkipsFreeRoutersAndBatchVariants(t *testing.T) {
	// Arrange
	c, _ := newCatalog(t)

	// Act
	got := candidateIDs(mustCandidates(t, c, model.ModelTierPaid))

	// Assert
	if !slices.Equal(got, []string{"liquid/lfm-2.5-2.6b"}) {
		t.Fatalf("got %v", got)
	}
}

func TestCatalogCandidates_whenThirdPartyRouterListed_thenSkipsIt(t *testing.T) {
	// Arrange
	c := newCatalogServing(t, `{"data":[
	  {"id":"liquid/lfm-2.5-2.6b","context_length":32768,"supported_parameters":["structured_outputs"],"architecture":{"output_modalities":["text"]},"pricing":{"prompt":"0.0000001","completion":"0.0000002"}},
	  {"id":"typesafe/jev-router","context_length":32768,"supported_parameters":["structured_outputs"],"architecture":{"output_modalities":["text"]},"pricing":{"prompt":"-1","completion":"-1"}}
	]}`)

	// Act
	got := candidateIDs(mustCandidates(t, c, model.ModelTierPaid))

	// Assert
	if !slices.Equal(got, []string{"liquid/lfm-2.5-2.6b"}) {
		t.Fatalf("got %v", got)
	}
}

func TestCatalogCandidates_whenPriceNegativeButNotRouterPlaceholder_thenErrors(t *testing.T) {
	// Arrange
	c := newCatalogServing(t, `{"data":[{"id":"liquid/lfm-2.5-2.6b","context_length":32768,"supported_parameters":["structured_outputs"],"architecture":{"output_modalities":["text"]},"pricing":{"prompt":"-0.5","completion":"0.0000002"}}]}`)

	// Act
	_, err := c.Candidates(model.ModelTierPaid)

	// Assert
	if err == nil {
		t.Fatal("want an error")
	}
}

func TestCatalogFreeStructuredModels_whenCalledTwiceWithinTTL_thenFetchesOnce(t *testing.T) {
	// Arrange
	c, fetches := newCatalog(t)
	mustCandidates(t, c, model.ModelTierFree)

	// Act
	mustCandidates(t, c, model.ModelTierFree)

	// Assert
	if fetches.Load() != 1 {
		t.Fatalf("fetched %d times, want 1", fetches.Load())
	}
}

func TestCatalogCandidates_whenContextLengthMissing_thenErrors(t *testing.T) {
	// Arrange
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"id":"liquid/lfm-2.5-2.6b:free","supported_parameters":["structured_outputs"],"architecture":{"output_modalities":["text"]},"pricing":{"prompt":"0","completion":"0"}}]}`))
	}))
	t.Cleanup(server.Close)
	c := &Catalog{URL: server.URL, HTTP: server.Client(), Replies: replyfile.Disabled()}

	// Act
	_, err := c.Candidates(model.ModelTierFree)

	// Assert
	if err == nil {
		t.Fatal("want an error")
	}
}

func TestCatalogAllCandidates_whenListed_thenHoldsFreeAndPaidButNoRouterOrBatch(t *testing.T) {
	// Arrange
	c, _ := newCatalog(t)

	// Act
	models, err := c.AllCandidates()

	// Assert
	if got := candidateIDs(models); err != nil || !slices.Equal(got, []string{"liquid/lfm-2.5-2.6b:free", "liquid/lfm-2.5-2.6b"}) {
		t.Fatalf("got %v, %v", got, err)
	}
}

const scoredModel = `{"id":"liquid/lfm-2.5-2.6b","context_length":32768,"supported_parameters":["structured_outputs"],"architecture":{"output_modalities":["text"]},"pricing":{"prompt":"0.0000001","completion":"0.0000002"},"benchmarks":{"design_arena":[],"artificial_analysis":{"intelligence_index":%s,"coding_index":null,"agentic_index":null}}}`

func TestCatalogCandidates_whenIntelligenceIndexListed_thenCarriesIt(t *testing.T) {
	// Arrange
	c := newCatalogServing(t, `{"data":[`+fmt.Sprintf(scoredModel, "43.4")+`]}`)

	// Act
	models := mustCandidates(t, c, model.ModelTierPaid)

	// Assert
	if len(models) != 1 || models[0].IntelligenceIndex == nil || *models[0].IntelligenceIndex != 43.4 {
		t.Fatalf("got %+v; want one model with intelligence index 43.4", models)
	}
}

func TestCatalogCandidates_whenIntelligenceIndexNull_thenLeavesItNil(t *testing.T) {
	// Arrange
	c := newCatalogServing(t, `{"data":[`+fmt.Sprintf(scoredModel, "null")+`]}`)

	// Act
	models := mustCandidates(t, c, model.ModelTierPaid)

	// Assert
	if len(models) != 1 || models[0].IntelligenceIndex != nil {
		t.Fatalf("got %+v; want one model without intelligence index", models)
	}
}

func TestCatalogCandidates_whenBenchmarksAbsent_thenLeavesIntelligenceIndexNil(t *testing.T) {
	// Arrange
	c, _ := newCatalog(t)

	// Act
	models := mustCandidates(t, c, model.ModelTierPaid)

	// Assert
	if len(models) != 1 || models[0].IntelligenceIndex != nil {
		t.Fatalf("got %+v; want one model without intelligence index", models)
	}
}

func TestCatalogCandidates_whenIntelligenceIndexAbove100_thenErrors(t *testing.T) {
	// Arrange
	c := newCatalogServing(t, `{"data":[`+fmt.Sprintf(scoredModel, "100.5")+`]}`)

	// Act
	_, err := c.Candidates(model.ModelTierPaid)

	// Assert
	if err == nil {
		t.Fatal("expected an error")
	}
}
