package history

import (
	"time"

	"github.com/Darida/openrouterclient/src/model"
)

// History is where generation outcomes go and exclusions come from: a
// file-backed *Store, or Disabled.
type History interface {
	Append(entry Entry)
	Exclusions(now time.Time, tag string) Exclusions
	RecordManual(generationID string, q model.Quality, reason string, now time.Time) error
}
