package quality

import (
	"testing"

	"github.com/Darida/openrouterclient/src/model"
)

func TestQualityFromBadScore_whenZero_thenHigh(t *testing.T) {
	// Act
	got := FromBadScore(0, 3)

	// Assert
	if got != model.QualityHigh {
		t.Fatalf("got %q, want high", got)
	}
}

func TestQualityFromBadScore_whenAtThreshold_thenMedium(t *testing.T) {
	// Act
	got := FromBadScore(5, 5)

	// Assert
	if got != model.QualityMedium {
		t.Fatalf("got %q, want medium", got)
	}
}

func TestQualityFromBadScore_whenAboveThreshold_thenLow(t *testing.T) {
	// Act
	got := FromBadScore(6, 5)

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
