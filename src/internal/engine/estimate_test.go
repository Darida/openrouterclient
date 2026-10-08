package engine

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/Darida/openrouterclient/src/model"
)

// Too many output tokens for tiny/model's context, so the cheapest paid pool
// is cheap/model alone.
var estimateRequest = model.EstimateRequest{
	Models:       model.ModelSelection{Allowed: []string{"cheap/model", "pricey/model"}},
	InputTokens:  1_000,
	OutputTokens: 20_000,
}

func TestEngineEstimate_whenAllowed_thenPicksTheCheapestOfThem(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	estimate, err := engine.Estimate(context.Background(), estimateRequest)

	// Assert
	if err != nil || estimate.Model != "cheap/model" {
		t.Fatalf("estimate=%+v, %v; want cheap/model", estimate, err)
	}
}

func TestEngineEstimate_whenDenied_thenPicksTheCheapestOfTheRest(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	denying := estimateRequest
	denying.Models = model.ModelSelection{Denied: []string{pickedModel}}

	// Act
	estimate, err := engine.Estimate(context.Background(), denying)

	// Assert
	if err != nil || estimate.Model != "cheap/model" {
		t.Fatalf("estimate=%+v, %v; want cheap/model", estimate, err)
	}
}

func TestEngineEstimate_whenPaidModelPicked_thenCostIsTokensTimesPrices(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	const want = 1_000*0.000001 + 20_000*0.000001

	// Act
	estimate, err := engine.Estimate(context.Background(), estimateRequest)

	// Assert
	if err != nil || math.Abs(estimate.CostUSD-want) > 1e-12 {
		t.Fatalf("estimate=%+v, %v; want CostUSD %v", estimate, err, want)
	}
}

func TestEngineEstimate_whenFreeModelPicked_thenCostIsZero(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	free := estimateRequest
	free.Models = freeTier

	// Act
	estimate, err := engine.Estimate(context.Background(), free)

	// Assert
	if err != nil || estimate != (model.Estimate{Model: pickedModel}) {
		t.Fatalf("estimate=%+v, %v; want %s at $0", estimate, err, pickedModel)
	}
}

func TestEngineEstimate_whenEstimating_thenSendsNoChatRequest(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	if _, err := engine.Estimate(context.Background(), estimateRequest); err != nil {
		t.Fatal(err)
	}

	// Assert
	if len(fake.generations)+len(fake.reviews) != 0 {
		t.Fatalf("generations=%+v reviews=%+v; want no chat requests", fake.generations, fake.reviews)
	}
}

func TestEngineEstimate_whenInputTokensZero_thenReturnsUnexpectedError(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	empty := estimateRequest
	empty.InputTokens = 0

	// Act
	_, err := engine.Estimate(context.Background(), empty)

	// Assert
	var unexpected *model.UnexpectedError
	if !errors.As(err, &unexpected) {
		t.Fatalf("err=%v; want *model.UnexpectedError", err)
	}
}

func TestEngineEstimate_whenOutputTokensZero_thenReturnsUnexpectedError(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	empty := estimateRequest
	empty.OutputTokens = 0

	// Act
	_, err := engine.Estimate(context.Background(), empty)

	// Assert
	var unexpected *model.UnexpectedError
	if !errors.As(err, &unexpected) {
		t.Fatalf("err=%v; want *model.UnexpectedError", err)
	}
}

func TestEngineEstimateCandidateModels_whenDefaultPercentile_thenListsOnlyTheCheapest(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)

	// Act
	candidates, err := engine.EstimateCandidateModels(estimateRequest)

	// Assert
	if err != nil || len(candidates) != 1 || candidates[0] != "cheap/model" {
		t.Fatalf("candidates=%v, %v; want [cheap/model]", candidates, err)
	}
}

func TestEngineEstimateCandidateModels_whenPercentile100_thenListsEveryAllowedModel(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	everyPrice := estimateRequest
	everyPrice.Models.CostPercentile = 100

	// Act
	candidates, err := engine.EstimateCandidateModels(everyPrice)

	// Assert
	if err != nil || len(candidates) != 2 {
		t.Fatalf("candidates=%v, %v; want cheap/model and pricey/model", candidates, err)
	}
}

func TestEngineEstimateCandidateModels_whenInputTokensZero_thenErrors(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	noInput := estimateRequest
	noInput.InputTokens = 0

	// Act
	_, err := engine.EstimateCandidateModels(noInput)

	// Assert
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestEngineEstimateCandidateModels_whenSelectionMatchesNothing_thenErrorIsErrNoCandidates(t *testing.T) {
	// Arrange
	fake := &fakeOpenRouter{}
	_, settings := fake.serve(t)
	engine, _ := newEngine(t, settings)
	unscored := estimateRequest
	unscored.Models = model.ModelSelection{Allowed: []string{"pricey/model"}, MinIntelligenceIndex: 100}

	// Act
	_, err := engine.EstimateCandidateModels(unscored)

	// Assert
	if !errors.Is(err, ErrNoCandidates) {
		t.Fatalf("err = %v, want ErrNoCandidates", err)
	}
}
