package engine

import (
	"time"

	"github.com/Darida/openrouterclient/src/internal/generationlog"
	"github.com/Darida/openrouterclient/src/internal/hedge"
)

type Settings struct {
	ChatURL          string
	GenerationLogURL string
	Hedge            hedge.Timing
	GenerationLog    generationlog.Timing
	// Generation rounds per call, counting the first; each later one is a correction.
	MaxRounds int
}

var Production = Settings{
	ChatURL:          "https://openrouter.ai/api/v1/chat/completions",
	GenerationLogURL: "https://openrouter.ai/api/v1/generation",
	Hedge: hedge.Timing{
		MaxAttempts:    3,
		Stagger:        60 * time.Second,
		AbortGrace:     30 * time.Second,
		AttemptTimeout: 180 * time.Second,
	},
	GenerationLog: generationlog.Timing{
		Window:       30 * time.Second,
		PollInterval: 5 * time.Second,
	},
	MaxRounds: 3,
}
