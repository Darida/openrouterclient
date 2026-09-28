package catalog

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
)

const catalogBody = `{"data":[
  {"id":"liquid/lfm-2.5-2.6b:free","canonical_slug":"liquid/lfm-2.5-2.6b-20260811","supported_parameters":["structured_outputs"],"architecture":{"output_modalities":["text"]}},
  {"id":"liquid/lfm-2.5-2.6b","canonical_slug":"liquid/lfm-2.5-2.6b-20260811","supported_parameters":["structured_outputs"],"architecture":{"output_modalities":["text"]}},
  {"id":"nvidia/nemotron-3.5-lightning:free","canonical_slug":"nvidia/nemotron-3.5-lightning-20260807","supported_parameters":["tools"],"architecture":{"output_modalities":["text"]}}
]}`

func newCatalog(t *testing.T) (*Catalog, *atomic.Int32) {
	var fetches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		w.Write([]byte(catalogBody))
	}))
	t.Cleanup(server.Close)
	return &Catalog{URL: server.URL, HTTP: server.Client()}, &fetches
}

func TestCatalogModelID_whenLogNameHasFreeVariant_thenReturnsFreeID(t *testing.T) {
	// Arrange
	c, _ := newCatalog(t)

	// Act
	got := c.ModelID("liquid/lfm-2.5-2.6b-20260811:free")

	// Assert
	if got != "liquid/lfm-2.5-2.6b:free" {
		t.Fatalf("got %q", got)
	}
}

func TestCatalogModelID_whenLogNameHasNoVariant_thenReturnsPaidID(t *testing.T) {
	// Arrange
	c, _ := newCatalog(t)

	// Act
	got := c.ModelID("liquid/lfm-2.5-2.6b-20260811")

	// Assert
	if got != "liquid/lfm-2.5-2.6b" {
		t.Fatalf("got %q", got)
	}
}

func TestCatalogModelID_whenLogNameUnknown_thenPanics(t *testing.T) {
	// Arrange
	c, _ := newCatalog(t)

	// Assert
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()

	// Act
	c.ModelID("vendor/unknown-20260101:free")
}

func TestCatalogFreeStructuredModels_whenListed_thenOnlyFreeWithStructuredOutputs(t *testing.T) {
	// Arrange
	c, _ := newCatalog(t)

	// Act
	got := c.FreeStructuredModels()

	// Assert
	if !slices.Equal(got, []string{"liquid/lfm-2.5-2.6b:free"}) {
		t.Fatalf("got %v", got)
	}
}

func TestCatalogFreeStructuredModels_whenCalledTwiceWithinTTL_thenFetchesOnce(t *testing.T) {
	// Arrange
	c, fetches := newCatalog(t)
	c.FreeStructuredModels()

	// Act
	c.FreeStructuredModels()

	// Assert
	if fetches.Load() != 1 {
		t.Fatalf("fetched %d times, want 1", fetches.Load())
	}
}
