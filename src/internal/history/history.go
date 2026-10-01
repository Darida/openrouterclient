package history

import (
	"time"

	"github.com/Darida/openrouterclient/src/model"
)

// History records generation outcomes and derives model exclusions from them.
type History interface {
	Append(entry Entry) error
	Exclusions(now time.Time, tag string) (Exclusions, error)
	RecordManual(generationID string, q model.Quality, reason string, now time.Time) error
}
