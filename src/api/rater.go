package api

import (
	"context"
	"errors"
	"time"

	"github.com/Darida/openrouterclient/src/internal/history"
	"github.com/Darida/openrouterclient/src/model"
)

// Rater records manual ratings in history without contacting OpenRouter, so
// it needs no API key.
type Rater interface {
	// Rate behaves exactly like Client.Rate.
	Rate(ctx context.Context, generationID string, quality model.Quality, reason string) error
}

// NewRater panics if an existing historyPath file is unreadable or malformed.
func NewRater(historyPath string) (Rater, error) {
	if historyPath == "" {
		return nil, errors.New("openrouterclient: historyPath is required")
	}
	return &rater{store: history.Open(historyPath)}, nil
}

type rater struct {
	store *history.Store
}

func (r *rater) Rate(ctx context.Context, generationID string, q model.Quality, reason string) error {
	return r.store.RecordManual(generationID, q, reason, time.Now())
}
