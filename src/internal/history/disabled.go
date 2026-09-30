package history

import (
	"errors"
	"time"

	"github.com/Darida/openrouterclient/src/model"
)

// Disabled records nothing and excludes no model, so every call starts with
// no knowledge of past failures.
type Disabled struct{}

func (Disabled) Append(Entry) {}

func (Disabled) Exclusions(time.Time, string) Exclusions { return Exclusions{} }

func (Disabled) RecordManual(string, model.Quality, string, time.Time) error {
	return errors.New("history: rating needs history, which is disabled")
}
