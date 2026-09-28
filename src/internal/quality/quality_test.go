package quality

import (
	"testing"

	"github.com/Darida/openrouterclient/src/model"
)

func TestQualityFromNoteCount_whenNoNotes_thenHigh(t *testing.T) {
	// Act
	got := FromNoteCount(0)

	// Assert
	if got != model.QualityHigh {
		t.Fatalf("got %q, want high", got)
	}
}

func TestQualityFromNoteCount_whenThreeNotes_thenMedium(t *testing.T) {
	// Act
	got := FromNoteCount(3)

	// Assert
	if got != model.QualityMedium {
		t.Fatalf("got %q, want medium", got)
	}
}

func TestQualityFromNoteCount_whenFourNotes_thenLow(t *testing.T) {
	// Act
	got := FromNoteCount(4)

	// Assert
	if got != model.QualityLow {
		t.Fatalf("got %q, want low", got)
	}
}

func TestQualityBelow_whenMediumAgainstHighTarget_thenTrue(t *testing.T) {
	// Act
	got := Below(model.QualityMedium, model.QualityHigh)

	// Assert
	if !got {
		t.Fatal("medium should be below a high target")
	}
}

func TestQualityBelow_whenUnknownQuality_thenPanics(t *testing.T) {
	// Assert
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()

	// Act
	Below("excellent", model.QualityHigh)
}
