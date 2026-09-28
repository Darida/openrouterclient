package history

import (
	"errors"
	"fmt"

	"github.com/Darida/openrouterclient/src/internal/quality"
	"github.com/Darida/openrouterclient/src/model"
)

func validate(f EntryFields) error {
	if err := validateRole(f); err != nil {
		return fmt.Errorf("invalid entry %+v: %w", f, err)
	}
	if err := validateOutcome(f); err != nil {
		return fmt.Errorf("invalid entry %+v: %w", f, err)
	}
	if err := validateSource(f); err != nil {
		return fmt.Errorf("invalid entry %+v: %w", f, err)
	}
	if f.Timestamp.IsZero() || f.Model == "" || f.GenerationID == "" {
		return fmt.Errorf("invalid entry %+v: timestamp, model, and generationId are required", f)
	}
	return nil
}

func validateRole(f EntryFields) error {
	switch f.Role {
	case RoleGenerator:
		if !quality.IsRating(f.TargetQuality) {
			return errors.New("a generator entry needs a high, medium, or low targetQuality")
		}
	case RoleReviewer:
		if f.TargetQuality != "" || f.Source != SourceAuto {
			return errors.New("a reviewer entry is automatic and has no targetQuality")
		}
	default:
		return errors.New("unknown role")
	}
	return nil
}

func validateOutcome(f EntryFields) error {
	switch f.Outcome {
	case OutcomeSuccess:
		if f.Quality != "" && !quality.IsRating(f.Quality) {
			return errors.New("a successful entry's quality must be empty, high, medium, or low")
		}
		if f.Role == RoleReviewer && f.Quality != "" {
			return errors.New("a reviewer's own output is never rated")
		}
	case OutcomeFailed, OutcomeTimeout, OutcomeAborted, OutcomeInvalidOutput:
		if f.Quality != model.QualityUnusable {
			return errors.New("a failed entry's quality must be unusable")
		}
	default:
		return errors.New("unknown outcome")
	}
	return nil
}

func validateSource(f EntryFields) error {
	switch f.Source {
	case SourceAuto:
	case SourceManual:
		if f.Outcome != OutcomeSuccess || !quality.IsRating(f.Quality) || f.Reason == "" {
			return errors.New("a manual entry needs a high, medium, or low quality and a reason")
		}
	default:
		return errors.New("unknown source")
	}
	return nil
}
