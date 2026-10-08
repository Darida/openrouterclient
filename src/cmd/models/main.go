package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/Darida/openrouterclient/src/cmd/internal/selectionflag"
	"github.com/Darida/openrouterclient/src/internal/engine"
	"github.com/Darida/openrouterclient/src/internal/history"
	"github.com/Darida/openrouterclient/src/internal/replyfile"
	"github.com/Darida/openrouterclient/src/model"
)

// Only the catalog is read, so the tag and timeout never reach OpenRouter or history.
const (
	tag     = "models"
	timeout = 60 * time.Second
)

// sampleRequest leaves MaxOutputTokens at its default, so a model whose
// context can't hold that many output tokens is left out, as in a real call.
var sampleRequest = model.GenerateRequest{
	Prompt: "Name a fruit.",
	OutputSchema: model.JSONSchema{
		Name:   "fruit",
		Schema: json.RawMessage(`{"type":"object","properties":{"fruit":{"type":"string"}},"required":["fruit"],"additionalProperties":false}`),
	},
	TargetQuality: model.QualityHigh,
}

func main() {
	exclude := flag.String("exclude", "", "comma-separated model IDs never asked (optional; absent lists each tier)")
	minIndex := flag.Float64("min-intelligence-index", 0, "lowest Artificial Analysis intelligence index asked (optional; 0 means no minimum)")
	costPercentile := flag.Float64("cost-percentile", 0, "cost percentile setting the cheapest pool's ceiling (optional; 0 means the 10th)")
	inputTokens := flag.Int("input-tokens", 0, "prompt tokens to price instead of the sample request (optional; requires --output-tokens)")
	outputTokens := flag.Int("output-tokens", 0, "output tokens to price instead of the sample request (optional; requires --input-tokens)")
	flag.Parse()
	if (*inputTokens == 0) != (*outputTokens == 0) {
		fail(errors.New("--input-tokens and --output-tokens must be given together"))
	}
	// Listing reads only the public model catalog, so no API key is sent.
	e := engine.New(engine.Production, "", tag, timeout, history.Disabled{}, replyfile.Local(), slog.New(slog.NewTextHandler(os.Stderr, nil)))
	// A selection nothing matches is an answer here, not a failure: it lists empty.
	list := func(sel model.ModelSelection) ([]string, error) {
		var candidates []string
		var err error
		if *inputTokens == 0 {
			request := sampleRequest
			request.Models = sel
			candidates, err = e.CandidateModels(request)
		} else {
			candidates, err = e.EstimateCandidateModels(model.EstimateRequest{Models: sel, InputTokens: *inputTokens, OutputTokens: *outputTokens})
		}
		if errors.Is(err, engine.ErrNoCandidates) {
			return nil, nil
		}
		return candidates, err
	}
	if *exclude == "" {
		printTiers(list, *minIndex, *costPercentile)
		return
	}
	denied, err := selectionflag.ParseExclude(*exclude)
	if err != nil {
		fail(err)
	}
	printDeniedPool(list, model.ModelSelection{Denied: denied, MinIntelligenceIndex: *minIndex, CostPercentile: *costPercentile})
}

// candidateLister lists the models a selection would pick from.
type candidateLister func(model.ModelSelection) ([]string, error)

// printDeniedPool runs one selection over both tiers, as Generate does, since
// the cheapest-pool cut over the whole set differs from one cut per tier.
func printDeniedPool(list candidateLister, sel model.ModelSelection) {
	candidates, err := list(sel)
	if err != nil {
		fail(err)
	}
	var free, paid []string
	for _, id := range candidates {
		if strings.HasSuffix(id, ":free") {
			free = append(free, id)
		} else {
			paid = append(paid, id)
		}
	}
	printList(model.ModelTierFree, free)
	printList(model.ModelTierPaid, paid)
}

func printTiers(list candidateLister, minIndex, costPercentile float64) {
	for _, tier := range []model.ModelTier{model.ModelTierFree, model.ModelTierPaid} {
		candidates, err := list(model.ModelSelection{Tier: tier, MinIntelligenceIndex: minIndex, CostPercentile: costPercentile})
		if err != nil {
			fail(err)
		}
		printList(tier, candidates)
	}
}

func printList(tier model.ModelTier, ids []string) {
	fmt.Printf("%s:\n", tier)
	for _, id := range ids {
		fmt.Printf("  %s\n", id)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "models:", err)
	os.Exit(1)
}
