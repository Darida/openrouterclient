package quality

import (
	"fmt"

	"github.com/Darida/openrouterclient/src/model"
)

// Notes above this count drop an automatic review from medium to low.
const maxMediumNotes = 3

// FromNoteCount rates a generation by how many rule violations its review found.
func FromNoteCount(notes int) model.Quality {
	switch {
	case notes == 0:
		return model.QualityHigh
	case notes <= maxMediumNotes:
		return model.QualityMedium
	default:
		return model.QualityLow
	}
}

func Below(q, target model.Quality) bool {
	return rank(q) < rank(target)
}

// IsRating reports whether q is a quality a review or caller can assign,
// which excludes unusable.
func IsRating(q model.Quality) bool {
	return q == model.QualityHigh || q == model.QualityMedium || q == model.QualityLow
}

func rank(q model.Quality) int {
	switch q {
	case model.QualityUnusable:
		return 0
	case model.QualityLow:
		return 1
	case model.QualityMedium:
		return 2
	case model.QualityHigh:
		return 3
	}
	panic(fmt.Sprintf("quality: unknown quality %q", q))
}
