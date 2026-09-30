package api

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Darida/openrouterclient/src/internal/engine"
	"github.com/Darida/openrouterclient/src/model"
)

// Client picks its OpenRouter model internally. Callers never pass or
// receive a model choice, only learn which model produced a result.
type Client interface {
	// GenerateText returns only reviewed content. Its errors are ctx.Err() and
	// *model.AttemptsExhaustedError; anything unexpected panics.
	GenerateText(ctx context.Context, requirements model.TextGenerationRequirements) (model.GeneratedText, error)
	// Rate records a manual high/medium/low rating of a generation's or a
	// review's GenerationID in history. Rating a review low also clears the
	// automatic rating that review gave. It errors for any other quality, an
	// unknown or already-rated id, or when history is disabled.
	Rate(ctx context.Context, generationID string, quality model.Quality, reason string) error
	// Close blocks until attempts still settling after GenerateText returned
	// are recorded in history. Call it once, before the process exits.
	Close()
}

// DefaultTimeout is the timeout for a request whose Timeout is 0.
const DefaultTimeout = 60 * time.Second

// Every field is required. New returns an error if any is unset.
type Config struct {
	APIKey  string
	History History
	Replies Replies
	Logger  *slog.Logger
}

func New(cfg Config) (Client, error) {
	if cfg.APIKey == "" || cfg.History.store == nil || cfg.Replies.saver.IsZero() || cfg.Logger == nil {
		return nil, errors.New("openrouterclient: Config.APIKey, Config.History, Config.Replies, and Config.Logger are all required")
	}
	return &client{Engine: engine.New(engine.Production, cfg.APIKey, cfg.History.store, cfg.Replies.saver, cfg.Logger)}, nil
}

type client struct {
	*engine.Engine
}

func (c *client) GenerateText(ctx context.Context, requirements model.TextGenerationRequirements) (model.GeneratedText, error) {
	if requirements.Timeout == 0 {
		requirements.Timeout = DefaultTimeout
	}
	return c.Engine.GenerateText(ctx, requirements)
}
