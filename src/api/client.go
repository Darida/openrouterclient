package api

import (
	"context"
	"log/slog"

	"github.com/Darida/openrouterclient/src/model"
)

// Client picks its OpenRouter model internally. Callers never pass or
// receive a model choice, only learn which model produced a result.
type Client interface {
	// GenerateText returns only content that passed an automatic review
	// against requirements.ReviewRulesPrompt. The only error it returns is
	// *model.AttemptsExhaustedError. It panics on any OpenRouter response or local
	// state it does not expect.
	GenerateText(ctx context.Context, requirements model.TextGenerationRequirements) (model.GeneratedText, error)
	// Rate records a caller's manual review of a GeneratedText.GenerationID
	// this client's history already holds. It returns an error for an unknown
	// id or a generation that was already rated.
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
