package history

import (
	"encoding/json"
	"fmt"
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

// EntryFields is an Entry's shape; NewEntry is the only way to turn one into an Entry.
type EntryFields struct {
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

// Entry is a validated history record. Its zero value is never valid.
type Entry struct {
	fields EntryFields
}

// NewEntry panics on invalid fields, since only this package's callers build them.
func NewEntry(fields EntryFields) Entry {
	if err := validate(fields); err != nil {
		panic(err.Error())
	}
	return Entry{fields: fields}
}

func (e Entry) Fields() EntryFields { return e.fields }

func (e Entry) MarshalJSON() ([]byte, error) { return json.Marshal(e.fields) }

func (e *Entry) UnmarshalJSON(raw []byte) error {
	var fields EntryFields
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	if err := validate(fields); err != nil {
		return fmt.Errorf("history: %w", err)
	}
	e.fields = fields
	return nil
}

type Exclusions struct {
	Excluded []string
	// Models with failures still under every cap, formatted with their counts for logging.
	BelowCap []string
}
