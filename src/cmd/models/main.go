package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
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
	// Listing reads only the public model catalog, so no API key is sent.
	e := engine.New(engine.Production, "", tag, timeout, history.Disabled{}, replyfile.Local(), slog.New(slog.NewTextHandler(os.Stderr, nil)))
	for _, tier := range []model.ModelTier{model.ModelTierFree, model.ModelTierPaid} {
		request := sampleRequest
		request.Models = model.ModelSelection{Tier: tier}
		candidates, err := e.CandidateModels(request)
		if err != nil {
			fmt.Fprintln(os.Stderr, "models:", err)
			os.Exit(1)
		}
		fmt.Printf("%s:\n", tier)
		for _, id := range candidates {
			fmt.Printf("  %s\n", id)
		}
	}
}
