package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/Darida/openrouterclient/src/cmd/internal/requirementsfile"
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
	inputPath := flag.String("input", "", "requirements JSON file (optional; absent lists each tier for a sample request)")
	flag.Parse()
	// Listing reads only the public model catalog, so no API key is sent.
	e := engine.New(engine.Production, "", tag, timeout, history.Disabled{}, replyfile.Local(), slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if *inputPath == "" {
		printTiers(e)
		return
	}
	printRequestPool(e, *inputPath)
}

// printRequestPool runs one selection over both tiers, as Generate does, since
// the cheapest-pool cut over the whole set differs from one cut per tier.
func printRequestPool(e *engine.Engine, inputPath string) {
	request, err := requirementsfile.Read(inputPath)
	if err != nil {
		fail(err)
	}
	candidates, err := e.CandidateModels(request)
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

func printTiers(e *engine.Engine) {
	for _, tier := range []model.ModelTier{model.ModelTierFree, model.ModelTierPaid} {
		request := sampleRequest
		request.Models = model.ModelSelection{Tier: tier}
		candidates, err := e.CandidateModels(request)
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
