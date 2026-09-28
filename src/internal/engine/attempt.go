package engine

import (
	"encoding/json"
	"time"

	"github.com/Darida/openrouterclient/src/internal/history"
)

// attempt is one settled chat request.
type attempt struct {
	// Empty only when OpenRouter never accepted the request.
	model        string
	generationID string
	outcome      history.Outcome
	// The caller's context ended it; it's no evidence about any model.
	canceled bool
	content  json.RawMessage
	latency  time.Duration
	reason   string
}
