package catalog

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const catalogBody = `{"data":[
  {"id":"liquid/lfm-2.5-2.6b:free","canonical_slug":"liquid/lfm-2.5-2.6b-20260811"},
  {"id":"liquid/lfm-2.5-2.6b","canonical_slug":"liquid/lfm-2.5-2.6b-20260811"}
]}`

func newCatalog(t *testing.T) *Catalog {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(catalogBody))
	}))
	t.Cleanup(server.Close)
	return &Catalog{URL: server.URL, HTTP: server.Client()}
}

func TestCatalogModelID_whenLogNameHasFreeVariant_thenReturnsFreeID(t *testing.T) {
	// Arrange
	c := newCatalog(t)

	// Act
	got := c.ModelID("liquid/lfm-2.5-2.6b-20260811:free")

	// Assert
	if got != "liquid/lfm-2.5-2.6b:free" {
		t.Fatalf("got %q", got)
	}
}

func TestCatalogModelID_whenLogNameHasNoVariant_thenReturnsPaidID(t *testing.T) {
	// Arrange
	c := newCatalog(t)

	// Act
	got := c.ModelID("liquid/lfm-2.5-2.6b-20260811")

	// Assert
	if got != "liquid/lfm-2.5-2.6b" {
		t.Fatalf("got %q", got)
	}
}

func TestCatalogModelID_whenLogNameUnknown_thenPanics(t *testing.T) {
	// Arrange
	c := newCatalog(t)

	// Assert
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()

	// Act
	c.ModelID("vendor/unknown-20260101:free")
}
