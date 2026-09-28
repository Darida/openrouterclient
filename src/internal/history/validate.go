package history

import (
	"fmt"

	"github.com/Darida/openrouterclient/src/internal/quality"
	"github.com/Darida/openrouterclient/src/model"
)

func validate(e Entry) {
	fail := func(detail string) {
		panic(fmt.Sprintf("history: invalid entry %+v: %s", e, detail))
	}
	if e.Timestamp.IsZero() || e.Model == "" || e.GenerationID == "" {
		fail("timestamp, model, and generationId are required")
	}
	switch e.Role {
	case RoleGenerator:
		if !quality.IsRating(e.TargetQuality) {
			fail("a generator entry needs a high, medium, or low targetQuality")
		}
	case RoleReviewer:
		if e.TargetQuality != "" || e.Source != SourceAuto {
			fail("a reviewer entry is automatic and has no targetQuality")
		}
	default:
		fail("unknown role")
	}
	switch e.Outcome {
	case OutcomeSuccess:
		if e.Quality != "" && !quality.IsRating(e.Quality) {
			fail("a successful entry's quality must be empty, high, medium, or low")
		}
		if e.Role == RoleReviewer && e.Quality != "" {
			fail("a reviewer's own output is never rated")
		}
	case OutcomeFailed, OutcomeTimeout, OutcomeAborted, OutcomeInvalidOutput:
		if e.Quality != model.QualityUnusable {
			fail("a failed entry's quality must be unusable")
		}
	default:
		fail("unknown outcome")
	}
	switch e.Source {
	case SourceAuto:
	case SourceManual:
		if e.Outcome != OutcomeSuccess || !quality.IsRating(e.Quality) || e.Reason == "" {
			fail("a manual entry needs a high, medium, or low quality and a reason")
		}
	default:
		fail("unknown source")
	}
}
