package engine

import (
	"time"
)

type Settings struct {
	ChatURL    string
	CatalogURL string
	// Hedged attempts per race; their timing comes from the engine's timeout.
	MaxAttempts int
	// Lets a success at exactly Timeout be recorded as slow, not as a timeout.
	GraceAfterTimeout time.Duration
	// Pause before resending a request the provider rejected before generating.
	RejectionRetryDelay time.Duration
}

var Production = Settings{
	ChatURL:             "https://openrouter.ai/api/v1/chat/completions",
	CatalogURL:          "https://openrouter.ai/api/v1/models",
	MaxAttempts:         3,
	GraceAfterTimeout:   time.Second,
	RejectionRetryDelay: time.Second,
}
