package engine

import (
	"time"

	"github.com/Darida/openrouterclient/src/internal/generationlog"
	"github.com/Darida/openrouterclient/src/internal/hedge"
)

type Settings struct {
	ChatURL          string
	GenerationLogURL string
	CatalogURL       string
	Hedge            hedge.Timing
	GenerationLog    generationlog.Timing
	// Generation rounds per call, counting the first; each later one is a correction.
	MaxRounds int
	// Pause before resending a request the provider rejected before generating.
	RejectionRetryDelay time.Duration
}

var Production = Settings{
	ChatURL:          "https://openrouter.ai/api/v1/chat/completions",
	GenerationLogURL: "https://openrouter.ai/api/v1/generation",
	CatalogURL:       "https://openrouter.ai/api/v1/models",
	Hedge: hedge.Timing{
		MaxAttempts:    3,
		Stagger:        60 * time.Second,
		AbortGrace:     30 * time.Second,
		AttemptTimeout: 180 * time.Second,
	},
	GenerationLog: generationlog.Timing{
		// Measured at 73–122s after a killed request.
		Window:       3 * time.Minute,
		PollInterval: 5 * time.Second,
	},
	MaxRounds:           3,
	RejectionRetryDelay: time.Second,
}
