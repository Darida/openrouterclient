package api

import (
	"context"
	"log/slog"

	"github.com/Darida/openrouterclient/src/model"
)

// Client picks its OpenRouter model internally. Callers never pass or
// receive a model choice, only learn which model produced a result.
type Client interface {
	// GenerateText returns only reviewed content. Its sole error is
	// *model.AttemptsExhaustedError; anything unexpected panics.
	GenerateText(ctx context.Context, requirements model.TextGenerationRequirements) (model.GeneratedText, error)
	// Rate records a manual review of a GenerationID in this client's history;
	// it errors for an unknown or already-rated id.
	Rate(ctx context.Context, generationID string, vote model.Vote, reason string) error
}

// Every field is required. New returns an error if any is unset.
type Config struct {
	APIKey string
	// A JSON file of per-generation outcomes that drives model exclusion. It
	// is created if absent. It must persist across runs, or exclusion never
	// learns anything.
	HistoryPath string
	Logger      *slog.Logger
}

func New(cfg Config) (Client, error) {
	panic("openrouterclient.New: not implemented")
}
