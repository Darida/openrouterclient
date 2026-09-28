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
		if !countsAsFailure(e) {
			continue
		}
		c, ok := counts[e.Model]
		if !ok {
			c = &failureCounts{}
			counts[e.Model] = c
		}
		if !e.Timestamp.Before(todayStart) {
			c.today++
		}
		if !e.Timestamp.Before(weekStart) {
			c.week++
		}
		if !e.Timestamp.Before(monthStart) {
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

func countsAsFailure(e Entry) bool {
	if e.Outcome != OutcomeSuccess || e.LatencySeconds >= slowLatencySeconds {
		return true
	}
	return e.Quality != "" && e.TargetQuality != "" && quality.Below(e.Quality, e.TargetQuality)
}
