package catalog

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Darida/openrouterclient/src/internal/replyfile"
)

const endpointsBody = `{"data":{"id":"meta-llama/llama-3.3-70b-instruct","endpoints":[
  {"tag":"deepinfra/turbo","provider_name":"DeepInfra","context_length":131072,"max_completion_tokens":16384,"max_prompt_tokens":null,"supported_parameters":["structured_outputs"],"pricing":{"prompt":"0.0000001","completion":"0.00000032","discount":0}},
  {"tag":"google-ai-studio","provider_name":"Google AI Studio","context_length":1048576,"supported_parameters":["structured_outputs"],"pricing":{"prompt":"0.00000125","completion":"0.00001","web_search":"0.014","overrides":[{"min_prompt_tokens":200000,"prompt":"0.0000025","completion":"0.000015","audio":"0.0000025"}]}}
]}}`

// newEndpointsCatalog serves body for every endpoints request and counts them.
func newEndpointsCatalog(t *testing.T, body string) (*Catalog, *atomic.Int32) {
	var fetches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return &Catalog{URL: server.URL, HTTP: server.Client(), Replies: replyfile.Disabled()}, &fetches
}

func TestCatalogEndpoints_whenListed_thenParsesTagsAndPrices(t *testing.T) {
	// Arrange
	c, _ := newEndpointsCatalog(t, endpointsBody)

	// Act
	endpoints, err := c.Endpoints("meta-llama/llama-3.3-70b-instruct")

	// Assert
	if err != nil || len(endpoints) != 2 || endpoints[0].Tag != "deepinfra/turbo" || endpoints[0].Price != (Price{Prompt: 0.0000001, Completion: 0.00000032}) {
		t.Fatalf("endpoints=%+v, %v", endpoints, err)
	}
}

func TestCatalogEndpoints_whenOverrideListed_thenKeepsItsMinimumAndPrices(t *testing.T) {
	// Arrange
	c, _ := newEndpointsCatalog(t, endpointsBody)

	// Act
	endpoints, err := c.Endpoints("meta-llama/llama-3.3-70b-instruct")

	// Assert
	if err != nil || len(endpoints[1].Overrides) != 1 || endpoints[1].Overrides[0].MinPromptTokens != 200_000 || *endpoints[1].Overrides[0].Completion != 0.000015 {
		t.Fatalf("endpoints=%+v, %v", endpoints, err)
	}
}

func TestCatalogEndpoints_whenAskedTwice_thenFetchesOnce(t *testing.T) {
	// Arrange
	c, fetches := newEndpointsCatalog(t, endpointsBody)

	// Act
	for range 2 {
		if _, err := c.Endpoints("meta-llama/llama-3.3-70b-instruct"); err != nil {
			t.Fatal(err)
		}
	}

	// Assert
	if fetches.Load() != 1 {
		t.Fatalf("fetches=%d; want 1", fetches.Load())
	}
}

func TestCatalogEndpoints_whenTagMissing_thenErrors(t *testing.T) {
	// Arrange
	c, _ := newEndpointsCatalog(t, `{"data":{"endpoints":[{"context_length":1000,"pricing":{"prompt":"0","completion":"0"}}]}}`)

	// Act
	_, err := c.Endpoints("a/model")

	// Assert
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestCatalogEndpoints_whenOverrideHasTimeWindow_thenErrors(t *testing.T) {
	// Arrange
	c, _ := newEndpointsCatalog(t, `{"data":{"endpoints":[{"tag":"x","context_length":1000,"pricing":{"prompt":"0.1","completion":"0.1","overrides":[{"utc_start":"0100","utc_end":"0500","prompt":"0.05"}]}}]}}`)

	// Act
	_, err := c.Endpoints("a/model")

	// Assert
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestCatalogEndpoints_whenPriceNegative_thenErrors(t *testing.T) {
	// Arrange
	c, _ := newEndpointsCatalog(t, `{"data":{"endpoints":[{"tag":"x","context_length":1000,"pricing":{"prompt":"-1","completion":"0"}}]}}`)

	// Act
	_, err := c.Endpoints("a/model")

	// Assert
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestCatalogEndpoints_whenBodyHasNoData_thenErrors(t *testing.T) {
	// Arrange
	c, _ := newEndpointsCatalog(t, `{"error":{"code":404,"message":"no such model"}}`)

	// Act
	_, err := c.Endpoints("a/model")

	// Assert
	if err == nil {
		t.Fatal("expected an error")
	}
}
