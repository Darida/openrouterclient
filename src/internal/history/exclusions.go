package history

import (
	"cmp"
	"slices"
	"sort"
	"time"

	"github.com/Darida/openrouterclient/src/internal/quality"
	"github.com/Darida/openrouterclient/src/model"
)

// A failure recorded under another request's tag is weaker evidence about
// how a model handles this one.
const (
	sameTagWeight  = 1.0
	otherTagWeight = 0.5
)

// A model is excluded once its weighted failures exceed any one of these.
const (
	maxFailuresToday    = 3
	maxFailuresWeek     = 6
	maxFailuresMonth    = 12
	maxFailuresLifetime = 24
)

func computeExclusions(entries []Entry, now time.Time, tag string) (Exclusions, error) {
	todayStart := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	weekStart := now.Add(-7 * 24 * time.Hour)
	monthStart := now.Add(-30 * 24 * time.Hour)

	counts := map[string]*FailureCounts{}
	for _, e := range entries {
		f := e.Fields()
		failed, err := countsAsFailure(f)
		if err != nil {
			return Exclusions{}, err
		}
		if !failed {
			continue
		}
		c, ok := counts[f.Model]
		if !ok {
			c = &FailureCounts{Model: f.Model}
			counts[f.Model] = c
		}
		weight := otherTagWeight
		if f.Tag == tag {
			weight = sameTagWeight
		}
		if !f.Timestamp.Before(todayStart) {
			c.Today += weight
		}
		if !f.Timestamp.Before(weekStart) {
			c.Week += weight
		}
		if !f.Timestamp.Before(monthStart) {
			c.Month += weight
		}
		c.Lifetime += weight
	}

	models := make([]string, 0, len(counts))
	for m := range counts {
		models = append(models, m)
	}
	sort.Strings(models)

	var result Exclusions
	for _, m := range models {
		c := counts[m]
		if c.Today > maxFailuresToday || c.Week > maxFailuresWeek || c.Month > maxFailuresMonth || c.Lifetime > maxFailuresLifetime {
			result.Excluded = append(result.Excluded, m)
		} else {
			result.BelowCap = append(result.BelowCap, *c)
		}
	}
	// Stable, so models tied on today stay in name order.
	slices.SortStableFunc(result.BelowCap, func(a, b FailureCounts) int { return cmp.Compare(b.Today, a.Today) })
	return result, nil
}

func countsAsFailure(f EntryFields) (bool, error) {
	if f.Outcome != OutcomeSuccess || f.LatencySeconds >= f.TimeoutSeconds {
		return true, nil
	}
	if f.Role == RoleReviewer {
		return f.Quality == model.QualityLow, nil
	}
	if f.Quality == "" || f.TargetQuality == "" {
		return false, nil
	}
	return quality.Below(f.Quality, f.TargetQuality)
}
