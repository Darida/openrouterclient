package cost

import (
	"testing"

	"github.com/Darida/openrouterclient/src/internal/catalog"
)

func ptr(v float64) *float64 { return &v }

// Gemini-style tiering: prompts of 200k tokens or more pay double.
var tieredEndpoint = catalog.Endpoint{
	Tag:       "google-ai-studio",
	Price:     catalog.Price{Prompt: 1, Completion: 10},
	Overrides: []catalog.PriceOverride{{MinPromptTokens: 200_000, Prompt: ptr(2), Completion: ptr(20)}},
}

func TestEndpointPrice_whenPromptBelowOverrideMinimum_thenKeepsBasePrice(t *testing.T) {
	// Act
	got := EndpointPrice(tieredEndpoint, 199_999)

	// Assert
	if got != (catalog.Price{Prompt: 1, Completion: 10}) {
		t.Fatalf("got %+v; want the base price", got)
	}
}

func TestEndpointPrice_whenPromptReachesOverrideMinimum_thenAppliesOverride(t *testing.T) {
	// Act
	got := EndpointPrice(tieredEndpoint, 200_000)

	// Assert
	if got != (catalog.Price{Prompt: 2, Completion: 20}) {
		t.Fatalf("got %+v; want the override price", got)
	}
}

func TestEndpointPrice_whenOverrideOmitsAPrice_thenKeepsThatBasePrice(t *testing.T) {
	// Arrange
	promptOnly := catalog.Endpoint{
		Price:     catalog.Price{Prompt: 1, Completion: 10},
		Overrides: []catalog.PriceOverride{{MinPromptTokens: 0, Prompt: ptr(3)}},
	}

	// Act
	got := EndpointPrice(promptOnly, 5)

	// Assert
	if got.Completion != 10 {
		t.Fatalf("got %+v; want completion 10 kept", got)
	}
}

func TestEndpointPrice_whenTwoOverridesMatch_thenTheLaterWins(t *testing.T) {
	// Arrange
	twoTiers := catalog.Endpoint{
		Price: catalog.Price{Prompt: 1},
		Overrides: []catalog.PriceOverride{
			{MinPromptTokens: 100, Prompt: ptr(2)},
			{MinPromptTokens: 1_000, Prompt: ptr(3)},
		},
	}

	// Act
	got := EndpointPrice(twoTiers, 5_000)

	// Assert
	if got.Prompt != 3 {
		t.Fatalf("got %+v; want prompt 3", got)
	}
}

func TestEndpointEstimate_whenRequestFeeSet_thenAddsItOnce(t *testing.T) {
	// Act
	got := EndpointEstimate(catalog.Price{Prompt: 0.5, Completion: 1, Request: 7}, 2, 3)

	// Assert
	if got != 0.5*2+1*3+7 {
		t.Fatalf("got %v; want 11", got)
	}
}
