package history

import (
	"testing"
	"time"

	"github.com/Darida/openrouterclient/src/model"
)

func failedEntries(n int, at time.Time) []Entry {
	entries := make([]Entry, n)
	for i := range entries {
		entries[i] = Entry{Timestamp: at, Model: "m", GenerationID: "g", Role: RoleGenerator, Source: SourceAuto, Outcome: OutcomeFailed, Quality: model.QualityUnusable, TargetQuality: model.QualityHigh}
	}
	return entries
}

func TestComputeExclusions_whenFourFailuresToday_thenExcluded(t *testing.T) {
	// Arrange
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

	// Act
	got := computeExclusions(failedEntries(4, now), now)

	// Assert
	if len(got.Excluded) != 1 {
		t.Fatalf("excluded = %v, want [m]", got.Excluded)
	}
}

func TestComputeExclusions_whenThreeFailuresToday_thenBelowCap(t *testing.T) {
	// Arrange
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

	// Act
	got := computeExclusions(failedEntries(3, now), now)

	// Assert
	if len(got.Excluded) != 0 || len(got.BelowCap) != 1 {
		t.Fatalf("got %+v, want m below cap", got)
	}
}

func TestCountsAsFailure_whenQualityBelowTarget_thenTrue(t *testing.T) {
	// Arrange
	e := generatorEntry("m", model.QualityMedium, model.QualityHigh, time.Now())

	// Act
	got := countsAsFailure(e)

	// Assert
	if !got {
		t.Fatal("medium against a high target should count as a failure")
	}
}

func TestCountsAsFailure_whenSuccessTookSixtySeconds_thenTrue(t *testing.T) {
	// Arrange
	e := generatorEntry("m", model.QualityHigh, model.QualityHigh, time.Now())
	e.LatencySeconds = 60

	// Act
	got := countsAsFailure(e)

	// Assert
	if !got {
		t.Fatal("a 60s success should count as a failure")
	}
}

func TestCountsAsFailure_whenUnratedSuccess_thenFalse(t *testing.T) {
	// Arrange
	e := generatorEntry("m", "", model.QualityHigh, time.Now())

	// Act
	got := countsAsFailure(e)

	// Assert
	if got {
		t.Fatal("an unrated success should not count as a failure")
	}
}
