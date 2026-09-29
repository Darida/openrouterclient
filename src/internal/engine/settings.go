package engine

import (
	"time"
)

type Settings struct {
	ChatURL    string
	CatalogURL string
	// Hedged attempts per race; their timing comes from each request's Timeout.
	MaxAttempts int
	// How far past the request's Timeout an attempt runs before it's cut off,
	// so a success at exactly Timeout is recorded as slow, not as a timeout.
	AttemptGrace time.Duration
	// Generation rounds per call, counting the first; each later one is a correction.
	MaxRounds int
	// Pause before resending a request the provider rejected before generating.
	RejectionRetryDelay time.Duration
}

var Production = Settings{
	ChatURL:             "https://openrouter.ai/api/v1/chat/completions",
	CatalogURL:          "https://openrouter.ai/api/v1/models",
	MaxAttempts:         3,
	AttemptGrace:        time.Second,
	MaxRounds:           3,
	RejectionRetryDelay: time.Second,
}
