package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
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
	exclude := flag.String("exclude", "", "comma-separated model IDs never asked (optional)")
	minIndex := flag.Float64("min-intelligence-index", 0, "lowest Artificial Analysis intelligence index asked (optional; 0 means no minimum)")
	costPercentile := flag.Float64("cost-percentile", 0, "cost percentile setting the cheapest pool's ceiling (optional; 0 means the 10th)")
	inputTokens := flag.Int("input-tokens", 0, "prompt tokens to price instead of the sample request (optional; requires --output-tokens)")
	outputTokens := flag.Int("output-tokens", 0, "output tokens to price instead of the sample request (optional; requires --input-tokens)")
	flag.Parse()
	if (*inputTokens == 0) != (*outputTokens == 0) {
		fail(errors.New("--input-tokens and --output-tokens must be given together"))
	}
	var denied []string
	if *exclude != "" {
		var err error
		if denied, err = selectionflag.ParseExclude(*exclude); err != nil {
			fail(err)
		}
	}
	sel := model.ModelSelection{Denied: denied, MinIntelligenceIndex: *minIndex, CostPercentile: *costPercentile}
	// Listing reads only the public model catalog, so no API key is sent.
	e := engine.New(engine.Production, "", tag, timeout, history.Disabled{}, replyfile.Local(), slog.New(slog.NewTextHandler(os.Stderr, nil)))
	var candidates []string
	var err error
	if *inputTokens == 0 {
		request := sampleRequest
		request.Models = sel
		candidates, err = e.CandidateModels(request)
	} else {
		candidates, err = e.EstimateCandidateModels(model.EstimateRequest{Models: sel, InputTokens: *inputTokens, OutputTokens: *outputTokens})
	}
	if err != nil {
		fail(err)
	}
	for _, id := range candidates {
		fmt.Println(id)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "models:", err)
	os.Exit(1)
}
