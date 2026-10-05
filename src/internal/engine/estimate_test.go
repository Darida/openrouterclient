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
