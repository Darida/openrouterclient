package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Darida/openrouterclient/src/internal/engine"
	"github.com/Darida/openrouterclient/src/internal/history"
	"github.com/Darida/openrouterclient/src/internal/replyfile"
	"github.com/Darida/openrouterclient/src/model"
)

// Only public listings are read, so the tag and timeout never reach OpenRouter or history.
const (
	tag     = "providers"
	timeout = 60 * time.Second
)

func main() {
	modelID := flag.String("model", "", "exact structured-output model ID (required)")
	inputTokens := flag.Int("input-tokens", 0, "prompt tokens to price (required, positive)")
	outputTokens := flag.Int("output-tokens", 0, "output tokens to price (required, positive)")
	flag.Parse()
	// Picking reads only public listings, so no API key is sent.
	e := engine.New(engine.Production, "", tag, timeout, history.Disabled{}, replyfile.Local(), slog.New(slog.NewTextHandler(os.Stderr, nil)))
	choice, err := e.PickProvider(context.Background(), model.ProviderRequest{Model: *modelID, InputTokens: *inputTokens, OutputTokens: *outputTokens})
	if err != nil {
		fail(err)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(choice); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "providers:", err)
	os.Exit(1)
}
