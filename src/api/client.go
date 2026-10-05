package api

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Darida/openrouterclient/src/internal/engine"
	"github.com/Darida/openrouterclient/src/model"
)

// Client picks its OpenRouter models internally, within each request's
// ModelSelection, and only reports which model produced a result. Every call
// that contacts OpenRouter returns ctx.Err(), *model.AttemptsExhaustedError,
// or *model.UnexpectedError, and never panics.
type Client interface {
	// Generate returns the first schema-valid output, with no review. It is
	// recorded in history as high, so only a later manual rating can count
	// it against its model.
	Generate(ctx context.Context, req model.GenerateRequest) (model.GeneratedText, error)
	// Review rates content the caller supplies against req.Criteria and
	// returns the verdict whatever its quality. Only the reviewer's own
	// outcome is recorded; rating the review's GenerationID low clears no
	// other rating.
	Review(ctx context.Context, req model.ReviewRequest) (model.Review, error)
	// GenerateReviewed generates, reviews, and corrects until the output meets
	// req.TargetQuality, and returns only output that does.
	GenerateReviewed(ctx context.Context, req model.GenerateReviewedRequest) (model.ReviewedText, error)
	// Estimate picks a model from req.Models as a first Generate attempt with
	// req's token counts would, history exclusions included, and prices it.
	// It reads only OpenRouter's model catalog. Each call draws again at
	// random from the cheapest models.
	Estimate(ctx context.Context, req model.EstimateRequest) (model.Estimate, error)
	// Rate records a manual high/medium/low rating of a generation's or a
	// review's GenerationID in history. Rating a review low also clears the
	// automatic rating that review gave. It errors for any other quality, an
	// unknown or already-rated id, or when history is disabled.
	Rate(ctx context.Context, generationID string, quality model.Quality, reason string) error
	// Close blocks until attempts still settling after a call returned are
	// recorded in history, and returns a *model.UnexpectedError if recording
	// any of them failed. Call it once, before the process exits.
	Close() error
}

// Every field is required. New returns an error if any is unset.
type Config struct {
	APIKey  string
	History History
	Replies Replies
	Logger  *slog.Logger
	// Groups this client's calls in history: generations are recorded under
	// Tag + "-generate" and reviews under Tag + "-review". When picking
	// models, a past failure under the same tag counts in full; under another
	// tag, half.
	Tag string
	// How long an attempt may stay pending before the next one starts, and
	// how long a success may take before it counts against its model. Each
	// attempt is cut off shortly after. Applies to generation and review
	// calls alike. Must be positive.
	Timeout time.Duration
}

func New(cfg Config) (Client, error) {
	if cfg.APIKey == "" || cfg.History.store == nil || cfg.Replies.saver.IsZero() || cfg.Logger == nil || cfg.Tag == "" {
		return nil, errors.New("openrouterclient: Config.APIKey, Config.History, Config.Replies, Config.Logger, and Config.Tag are all required")
	}
	if cfg.Timeout <= 0 {
		return nil, errors.New("openrouterclient: Config.Timeout must be positive")
	}
	return &client{Engine: engine.New(engine.Production, cfg.APIKey, cfg.Tag, cfg.Timeout, cfg.History.store, cfg.Replies.saver, cfg.Logger)}, nil
}

type client struct {
	*engine.Engine
}

func (c *client) Close() error {
	if err := c.Engine.Close(); err != nil {
		return &model.UnexpectedError{Err: err}
	}
	return nil
}
