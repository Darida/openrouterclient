package api

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Darida/openrouterclient/src/internal/engine"
	"github.com/Darida/openrouterclient/src/internal/history"
	"github.com/Darida/openrouterclient/src/model"
)

// Client picks its OpenRouter model internally. Callers never pass or
// receive a model choice, only learn which model produced a result.
type Client interface {
	// GenerateText returns only reviewed content. Its errors are ctx.Err() and
	// *model.AttemptsExhaustedError; anything unexpected panics.
	GenerateText(ctx context.Context, requirements model.TextGenerationRequirements) (model.GeneratedText, error)
	// Rate records a manual high/medium/low rating of a GenerationID in history;
	// it errors for any other quality or an unknown or already-rated id.
	Rate(ctx context.Context, generationID string, quality model.Quality, reason string) error
	// Close blocks until attempts still settling after GenerateText returned
	// are recorded in history. Call it once, before the process exits.
	Close()
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

// New panics if an existing HistoryPath file is unreadable or malformed.
func New(cfg Config) (Client, error) {
	if cfg.APIKey == "" || cfg.HistoryPath == "" || cfg.Logger == nil {
		return nil, errors.New("openrouterclient: Config.APIKey, Config.HistoryPath, and Config.Logger are all required")
	}
	return engine.New(engine.Production, cfg.APIKey, history.Open(cfg.HistoryPath), cfg.Logger), nil
}
