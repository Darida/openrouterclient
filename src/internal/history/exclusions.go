package history

import (
	"fmt"
	"sort"
	"time"

	"github.com/Darida/openrouterclient/src/internal/quality"
)

// A success this slow counts against the model as much as a failure.
const slowLatencySeconds = 60

// A model is excluded once its failures exceed any one of these.
const (
	maxFailuresToday    = 3
	maxFailuresWeek     = 6
	maxFailuresMonth    = 12
	maxFailuresLifetime = 24
)

type failureCounts struct {
	today, week, month, lifetime int
}

func computeExclusions(entries []Entry, now time.Time) Exclusions {
	todayStart := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	weekStart := now.Add(-7 * 24 * time.Hour)
	monthStart := now.Add(-30 * 24 * time.Hour)

	counts := map[string]*failureCounts{}
	for _, e := range entries {
		f := e.Fields()
		if !countsAsFailure(f) {
			continue
		}
		c, ok := counts[f.Model]
		if !ok {
			c = &failureCounts{}
			counts[f.Model] = c
		}
		if !f.Timestamp.Before(todayStart) {
			c.today++
		}
		if !f.Timestamp.Before(weekStart) {
			c.week++
		}
		if !f.Timestamp.Before(monthStart) {
			c.month++
		}
		c.lifetime++
	}

	models := make([]string, 0, len(counts))
	for m := range counts {
		models = append(models, m)
	}
	sort.Strings(models)

	var result Exclusions
	for _, m := range models {
		c := counts[m]
		if c.today > maxFailuresToday || c.week > maxFailuresWeek || c.month > maxFailuresMonth || c.lifetime > maxFailuresLifetime {
			result.Excluded = append(result.Excluded, m)
		} else {
			result.BelowCap = append(result.BelowCap, fmt.Sprintf("%s (today=%d week=%d month=%d lifetime=%d)", m, c.today, c.week, c.month, c.lifetime))
		}
	}
	return result
}

func countsAsFailure(f EntryFields) bool {
	if f.Outcome != OutcomeSuccess || f.LatencySeconds >= slowLatencySeconds {
		return true
	}
	return f.Quality != "" && f.TargetQuality != "" && quality.Below(f.Quality, f.TargetQuality)
}
