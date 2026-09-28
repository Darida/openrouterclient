package engine

import (
	"time"

	"github.com/Darida/openrouterclient/src/internal/hedge"
)

type Settings struct {
	ChatURL          string
	CatalogURL       string
	Hedge            hedge.Timing
	// Generation rounds per call, counting the first; each later one is a correction.
	MaxRounds int
	// Pause before resending a request the provider rejected before generating.
	RejectionRetryDelay time.Duration
}

var Production = Settings{
	ChatURL:          "https://openrouter.ai/api/v1/chat/completions",
	CatalogURL:       "https://openrouter.ai/api/v1/models",
	Hedge: hedge.Timing{
		MaxAttempts:    3,
		Stagger:        60 * time.Second,
		// Just past the 60s at which a success already counts as a failure.
		AttemptTimeout: 61 * time.Second,
	},
	MaxRounds:           3,
	RejectionRetryDelay: time.Second,
}
