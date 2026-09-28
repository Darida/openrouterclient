package history

import (
	"testing"
	"time"

	"github.com/Darida/openrouterclient/src/model"
)

func failedEntries(n int, at time.Time, tag string) []Entry {
	entries := make([]Entry, n)
	for i := range entries {
		entries[i] = NewEntry(EntryFields{Timestamp: at, Model: "m", GenerationID: "g", Role: RoleGenerator, Source: SourceAuto, Outcome: OutcomeFailed, Quality: model.QualityUnusable, TargetQuality: model.QualityHigh, Tag: tag})
	}
	return entries
}

func TestComputeExclusions_whenFourFailuresToday_thenExcluded(t *testing.T) {
	// Arrange
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

	// Act
	got := computeExclusions(failedEntries(4, now, "t"), now, "t")

	// Assert
	if len(got.Excluded) != 1 {
		t.Fatalf("excluded = %v, want [m]", got.Excluded)
	}
}

func TestComputeExclusions_whenThreeFailuresToday_thenBelowCap(t *testing.T) {
	// Arrange
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

	// Act
	got := computeExclusions(failedEntries(3, now, "t"), now, "t")

	// Assert
	if len(got.Excluded) != 0 || len(got.BelowCap) != 1 {
		t.Fatalf("got %+v, want m below cap", got)
	}
}

func TestCountsAsFailure_whenQualityBelowTarget_thenTrue(t *testing.T) {
	// Arrange
	e := generatorFields("g", model.QualityMedium, model.QualityHigh, time.Now())

	// Act
	got := countsAsFailure(e)

	// Assert
	if !got {
		t.Fatal("medium against a high target should count as a failure")
	}
}

func TestCountsAsFailure_whenSuccessTookSixtySeconds_thenTrue(t *testing.T) {
	// Arrange
	e := generatorFields("g", model.QualityHigh, model.QualityHigh, time.Now())
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
	e := generatorFields("g", "", model.QualityHigh, time.Now())

	// Act
	got := countsAsFailure(e)

	// Assert
	if got {
		t.Fatal("an unrated success should not count as a failure")
	}
}

func TestComputeExclusions_whenFourFailuresTodayUnderOtherTag_thenBelowCap(t *testing.T) {
	// Arrange
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

	// Act
	got := computeExclusions(failedEntries(4, now, "other"), now, "t")

	// Assert
	if len(got.Excluded) != 0 {
		t.Fatalf("excluded = %v; 4 other-tag failures weigh 2, under the cap of 3", got.Excluded)
	}
}

func TestComputeExclusions_whenEightFailuresTodayUnderOtherTag_thenExcluded(t *testing.T) {
	// Arrange
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

	// Act
	got := computeExclusions(failedEntries(8, now, "other"), now, "t")

	// Assert
	if len(got.Excluded) != 1 {
		t.Fatalf("excluded = %v; 8 other-tag failures weigh 4, over the cap of 3", got.Excluded)
	}
}
