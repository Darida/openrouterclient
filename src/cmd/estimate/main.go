package main

import (
	"context"
	"encoding/json"
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
	tag     = "estimate"
	timeout = 60 * time.Second
)

func main() {
	inputTokens := flag.Int("input-tokens", 0, "prompt tokens to price (required, positive)")
	outputTokens := flag.Int("output-tokens", 0, "output tokens to price (required, positive)")
	exclude := flag.String("exclude", "", "comma-separated model IDs never picked (optional)")
	minIndex := flag.Float64("min-intelligence-index", 0, "lowest Artificial Analysis intelligence index picked (optional; 0 means no minimum)")
	costPercentile := flag.Float64("cost-percentile", 0, "cost percentile setting the cheapest pool's ceiling (optional; 0 means the 10th)")
	flag.Parse()
	var denied []string
	if *exclude != "" {
		var err error
		if denied, err = selectionflag.ParseExclude(*exclude); err != nil {
			fail(err)
		}
	}
	// Estimating reads only the public model catalog, so no API key is sent.
	e := engine.New(engine.Production, "", tag, timeout, history.Disabled{}, replyfile.Local(), slog.New(slog.NewTextHandler(os.Stderr, nil)))
	estimate, err := e.Estimate(context.Background(), model.EstimateRequest{
		Models:       model.ModelSelection{Denied: denied, MinIntelligenceIndex: *minIndex, CostPercentile: *costPercentile},
		InputTokens:  *inputTokens,
		OutputTokens: *outputTokens,
	})
	if err != nil {
		fail(err)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(estimate); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "estimate:", err)
	os.Exit(1)
}
