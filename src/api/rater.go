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

// NewRater errors if h is unset or disabled, since there is nowhere to record a rating.
func NewRater(h History) (Rater, error) {
	if h.store == nil {
		return nil, errors.New("openrouterclient: NewRater needs a History")
	}
	if h.isDisabled() {
		return nil, errors.New("openrouterclient: NewRater needs history, which is disabled")
	}
	return &rater{store: h.store}, nil
}

type rater struct {
	store history.History
}

func (r *rater) Rate(ctx context.Context, generationID string, q model.Quality, reason string) error {
	return r.store.RecordManual(generationID, q, reason, time.Now())
}
