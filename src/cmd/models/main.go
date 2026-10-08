package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

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
	flag.Parse()
	// Listing reads only the public model catalog, so no API key is sent.
	e := engine.New(engine.Production, "", tag, timeout, history.Disabled{}, replyfile.Local(), slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if *exclude == "" {
		printTiers(e, *minIndex)
		return
	}
	denied, err := parseExclude(*exclude)
	if err != nil {
		fail(err)
	}
	printDeniedPool(e, model.ModelSelection{Denied: denied, MinIntelligenceIndex: *minIndex})
}

// printDeniedPool runs one selection over both tiers, as Generate does, since
// the cheapest-pool cut over the whole set differs from one cut per tier.
func printDeniedPool(e *engine.Engine, sel model.ModelSelection) {
	request := sampleRequest
	request.Models = sel
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

func parseExclude(list string) ([]string, error) {
	ids := strings.Split(list, ",")
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("--exclude %q has an empty model ID", list)
		}
	}
	return ids, nil
}

func printTiers(e *engine.Engine, minIndex float64) {
	for _, tier := range []model.ModelTier{model.ModelTierFree, model.ModelTierPaid} {
		request := sampleRequest
		request.Models = model.ModelSelection{Tier: tier, MinIntelligenceIndex: minIndex}
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
