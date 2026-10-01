package history

import (
	"testing"
	"time"

	"github.com/Darida/openrouterclient/src/model"
)

func TestDisabledRecordManual_whenCalled_thenErrors(t *testing.T) {
	// Act
	err := Disabled{}.RecordManual("gen-1", model.QualityHigh, "good", time.Now())

	// Assert
	if err == nil {
		t.Fatal("expected an error while history is disabled")
	}
}

func TestDisabledExclusions_whenFailuresAppended_thenExcludesNothing(t *testing.T) {
	// Arrange
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	history := Disabled{}
	for _, e := range failedEntries(t, 5, now, "t") {
		if err := history.Append(e); err != nil {
			t.Fatal(err)
		}
	}

	// Act
	got := mustExclusions(t, history, now, "t")

	// Assert
	if len(got.Excluded) != 0 {
		t.Fatalf("excluded = %v, want none", got.Excluded)
	}
}
