package history

import (
	"time"

	"github.com/Darida/openrouterclient/src/model"
)

type Role string

const (
	RoleGenerator Role = "generator"
	RoleReviewer  Role = "reviewer"
)

type Source string

const (
	SourceAuto   Source = "auto"
	SourceManual Source = "manual"
)

type Outcome string

const (
	OutcomeSuccess       Outcome = "success"
	OutcomeFailed        Outcome = "failed"
	OutcomeTimeout       Outcome = "timeout"
	OutcomeAborted       Outcome = "aborted"
	OutcomeInvalidOutput Outcome = "invalid_output"
)

type Entry struct {
	Timestamp    time.Time `json:"timestamp"`
	Model        string    `json:"model"`
	GenerationID string    `json:"generationId"`
	Role         Role      `json:"role"`
	Source       Source    `json:"source"`
	Outcome      Outcome   `json:"outcome"`
	// Unusable for every failed outcome. Empty for a successful generation
	// nobody rated: a reviewer's own output, or a generation that lost the
	// hedge or never got a review.
	Quality model.Quality `json:"quality,omitempty"`
	// The request's TargetQuality. Set for generator entries only.
	TargetQuality  model.Quality `json:"targetQuality,omitempty"`
	LatencySeconds float64       `json:"latencySeconds"`
	Reason         string        `json:"reason,omitempty"`
}

type Exclusions struct {
	Excluded []string
	// Models with failures still under every cap, formatted with their counts for logging.
	BelowCap []string
}
