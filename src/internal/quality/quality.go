package quality

import (
	"fmt"

	"github.com/Darida/openrouterclient/src/model"
)

// FromBadScore rates a generation by its review's total bad score; a total
// above threshold drops it from medium to low.
func FromBadScore(total, threshold int) model.Quality {
	switch {
	case total == 0:
		return model.QualityHigh
	case total <= threshold:
		return model.QualityMedium
	default:
		return model.QualityLow
	}
}

func Below(q, target model.Quality) (bool, error) {
	qRank, err := rank(q)
	if err != nil {
		return false, err
	}
	targetRank, err := rank(target)
	if err != nil {
		return false, err
	}
	return qRank < targetRank, nil
}

// IsRating reports whether q is a quality a review or caller can assign,
// which excludes unusable.
func IsRating(q model.Quality) bool {
	return q == model.QualityHigh || q == model.QualityMedium || q == model.QualityLow
}

func rank(q model.Quality) (int, error) {
	switch q {
	case model.QualityUnusable:
		return 0, nil
	case model.QualityLow:
		return 1, nil
	case model.QualityMedium:
		return 2, nil
	case model.QualityHigh:
		return 3, nil
	}
	return 0, fmt.Errorf("quality: unknown quality %q", q)
}
