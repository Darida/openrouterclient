package history

import (
	"time"

	"github.com/Darida/openrouterclient/src/model"
)

// History records generation outcomes and derives model exclusions from them.
type History interface {
	Append(entry Entry)
	Exclusions(now time.Time, tag string) Exclusions
	RecordManual(generationID string, q model.Quality, reason string, now time.Time) error
}
