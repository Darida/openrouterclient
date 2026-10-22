package engine

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/Darida/openrouterclient/src/model"
)

// Only e/cheapest can serve providerRequest at the lowest cost: a lacks
// structured outputs, b caps output below 1,000 tokens, c is above the prompt
// price cap, d costs more, and f's per-request fee outweighs its token prices.
const cheapModelEndpoints = `{"data":{"id":"cheap/model","endpoints":[
	{"tag":"a/no-schema","provider_name":"A","context_length":100000,"supported_parameters":["response_format","max_tokens"],"pricing":{"prompt":"0.00000001","completion":"0.00000001"}},
	{"tag":"b/short-output","provider_name":"B","context_length":100000,"max_completion_tokens":100,"supported_parameters":["structured_outputs","response_format","max_tokens"],"pricing":{"prompt":"0.00000002","completion":"0.00000002"}},
	{"tag":"c/over-cap","provider_name":"C","context_length":100000,"supported_parameters":["structured_outputs","response_format","max_tokens"],"pricing":{"prompt":"0.00001","completion":"0.00000001"}},
	{"tag":"d/mid","provider_name":"D","context_length":100000,"supported_parameters":["structured_outputs","response_format","max_tokens"],"pricing":{"prompt":"0.0000005","completion":"0.0000005"}},
	{"tag":"e/cheapest","provider_name":"E","context_length":100000,"max_completion_tokens":null,"supported_parameters":["structured_outputs","response_format","max_tokens"],"pricing":{"prompt":"0.0000003","completion":"0.0000003","discount":0}},
	{"tag":"f/request-fee","provider_name":"F","context_length":100000,"supported_parameters":["structured_outputs","response_format","max_tokens"],"pricing":{"prompt":"0.0000001","completion":"0.0000001","request":"0.01"}}
]}}`

var providerRequest = model.ProviderRequest{Model: "cheap/model", InputTokens: 1_000, OutputTokens: 1_000}

func newProviderEngine(t *testing.T, endpoints map[string]string) *Engine {
	fake := &fakeOpenRouter{endpoints: endpoints}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	return engine
}

func TestEnginePickProvider_whenSeveralServable_thenPicksTheCheapest(t *testing.T) {
	// Arrange
	engine := newProviderEngine(t, map[string]string{"cheap/model": cheapModelEndpoints})

	// Act
	choice, err := engine.PickProvider(context.Background(), providerRequest)

	// Assert
	if err != nil || choice.Provider != "e/cheapest" {
		t.Fatalf("choice=%+v, %v; want e/cheapest", choice, err)
	}
}

func TestEnginePickProvider_whenPicked_thenCostIsTokensTimesPrices(t *testing.T) {
	// Arrange
	engine := newProviderEngine(t, map[string]string{"cheap/model": cheapModelEndpoints})
	const want = 1_000*0.0000003 + 1_000*0.0000003

	// Act
	choice, err := engine.PickProvider(context.Background(), providerRequest)

	// Assert
	if err != nil || math.Abs(choice.CostUSD-want) > 1e-12 {
		t.Fatalf("choice=%+v, %v; want CostUSD %v", choice, err, want)
	}
}

func TestEnginePickProvider_whenRequestFeeOutweighsTokens_thenCostIncludesIt(t *testing.T) {
	// Arrange
	onlyFee := `{"data":{"endpoints":[{"tag":"f/request-fee","context_length":100000,"supported_parameters":["structured_outputs","response_format","max_tokens"],"pricing":{"prompt":"0.0000001","completion":"0.0000001","request":"0.01"}}]}}`
	engine := newProviderEngine(t, map[string]string{"cheap/model": onlyFee})
	const want = 1_000*0.0000001 + 1_000*0.0000001 + 0.01

	// Act
	choice, err := engine.PickProvider(context.Background(), providerRequest)

	// Assert
	if err != nil || math.Abs(choice.CostUSD-want) > 1e-12 {
		t.Fatalf("choice=%+v, %v; want CostUSD %v", choice, err, want)
	}
}

func TestEnginePickProvider_whenCostsTie_thenPicksTheLowerTag(t *testing.T) {
	// Arrange
	tied := `{"data":{"endpoints":[
		{"tag":"z/same","context_length":100000,"supported_parameters":["structured_outputs","response_format","max_tokens"],"pricing":{"prompt":"0.0000001","completion":"0.0000001"}},
		{"tag":"m/same","context_length":100000,"supported_parameters":["structured_outputs","response_format","max_tokens"],"pricing":{"prompt":"0.0000001","completion":"0.0000001"}}
	]}}`
	engine := newProviderEngine(t, map[string]string{"cheap/model": tied})

	// Act
	choice, err := engine.PickProvider(context.Background(), providerRequest)

	// Assert
	if err != nil || choice.Provider != "m/same" {
		t.Fatalf("choice=%+v, %v; want m/same", choice, err)
	}
}

func TestEnginePickProvider_whenModelNotInCatalog_thenReturnsUnexpectedError(t *testing.T) {
	// Arrange
	engine := newProviderEngine(t, map[string]string{"cheap/model": cheapModelEndpoints})
	unknown := providerRequest
	unknown.Model = "nobody/model"

	// Act
	_, err := engine.PickProvider(context.Background(), unknown)

	// Assert
	var unexpected *model.UnexpectedError
	if !errors.As(err, &unexpected) {
		t.Fatalf("err=%v; want *model.UnexpectedError", err)
	}
}

func TestEnginePickProvider_whenNoEndpointListed_thenReturnsUnexpectedError(t *testing.T) {
	// Arrange
	engine := newProviderEngine(t, map[string]string{"cheap/model": `{"data":{"endpoints":[]}}`})

	// Act
	_, err := engine.PickProvider(context.Background(), providerRequest)

	// Assert
	var unexpected *model.UnexpectedError
	if !errors.As(err, &unexpected) {
		t.Fatalf("err=%v; want *model.UnexpectedError", err)
	}
}

func TestEnginePickProvider_whenNoEndpointFitsOutput_thenReturnsUnexpectedError(t *testing.T) {
	// Arrange
	engine := newProviderEngine(t, map[string]string{"cheap/model": cheapModelEndpoints})
	huge := providerRequest
	huge.OutputTokens = 200_000

	// Act
	_, err := engine.PickProvider(context.Background(), huge)

	// Assert
	var unexpected *model.UnexpectedError
	if !errors.As(err, &unexpected) {
		t.Fatalf("err=%v; want *model.UnexpectedError", err)
	}
}

func TestEnginePickProvider_whenInputTokensZero_thenReturnsUnexpectedError(t *testing.T) {
	// Arrange
	engine := newProviderEngine(t, map[string]string{"cheap/model": cheapModelEndpoints})
	empty := providerRequest
	empty.InputTokens = 0

	// Act
	_, err := engine.PickProvider(context.Background(), empty)

	// Assert
	var unexpected *model.UnexpectedError
	if !errors.As(err, &unexpected) {
		t.Fatalf("err=%v; want *model.UnexpectedError", err)
	}
}
