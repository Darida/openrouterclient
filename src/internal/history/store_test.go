package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Darida/openrouterclient/src/model"
)

func generatorFields(generationID string, q, target model.Quality, at time.Time) EntryFields {
	return EntryFields{Timestamp: at, Model: "m", GenerationID: generationID, Role: RoleGenerator, Source: SourceAuto, Outcome: OutcomeSuccess, Quality: q, TargetQuality: target, Tag: "t"}
}

func TestStoreRecordManual_whenGenerationUnknown_thenErrors(t *testing.T) {
	// Arrange
	store := Open(filepath.Join(t.TempDir(), "history.json"))

	// Act
	err := store.RecordManual("gen-missing", model.QualityHigh, "good", time.Now())

	// Assert
	if err == nil {
		t.Fatal("expected an error for an unknown generation")
	}
}

func TestStoreRecordManual_whenAlreadyRated_thenErrors(t *testing.T) {
	// Arrange
	store := Open(filepath.Join(t.TempDir(), "history.json"))
	store.Append(NewEntry(generatorFields("gen-m", model.QualityHigh, model.QualityHigh, time.Now())))
	if err := store.RecordManual("gen-m", model.QualityLow, "wrong facts", time.Now()); err != nil {
		t.Fatalf("setup: first rating failed: %v", err)
	}

	// Act
	err := store.RecordManual("gen-m", model.QualityHigh, "changed my mind", time.Now())

	// Assert
	if err == nil {
		t.Fatal("expected an error for a second rating")
	}
}

func TestStoreRecordManual_whenQualityUnusable_thenErrors(t *testing.T) {
	// Arrange
	store := Open(filepath.Join(t.TempDir(), "history.json"))
	store.Append(NewEntry(generatorFields("gen-m", model.QualityHigh, model.QualityHigh, time.Now())))

	// Act
	err := store.RecordManual("gen-m", model.QualityUnusable, "broken", time.Now())

	// Assert
	if err == nil {
		t.Fatal("expected an error for an unusable manual rating")
	}
}

func TestStoreRecordManual_whenBelowTarget_thenCountsTowardExclusion(t *testing.T) {
	// Arrange
	store := Open(filepath.Join(t.TempDir(), "history.json"))
	now := time.Now()
	for i := 0; i < 4; i++ {
		generationID := "gen-" + string(rune('a'+i))
		store.Append(NewEntry(generatorFields(generationID, model.QualityHigh, model.QualityHigh, now)))
		if err := store.RecordManual(generationID, model.QualityLow, "wrong", now); err != nil {
			t.Fatalf("setup: rating failed: %v", err)
		}
	}

	// Act
	got := store.Exclusions(now, "t")

	// Assert
	if len(got.Excluded) != 1 || got.Excluded[0] != "m" {
		t.Fatalf("excluded = %v, want [m]", got.Excluded)
	}
}

func TestStoreOpen_whenFileMalformed_thenPanics(t *testing.T) {
	// Arrange
	path := filepath.Join(t.TempDir(), "history.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Assert
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()

	// Act
	Open(path)
}

func TestNewEntry_whenFailedOutcomeHasRatedQuality_thenPanics(t *testing.T) {
	// Arrange
	fields := generatorFields("gen-m", model.QualityHigh, model.QualityHigh, time.Now())
	fields.Outcome = OutcomeTimeout

	// Assert
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()

	// Act
	NewEntry(fields)
}

func TestStoreOpen_whenEntryInvalid_thenPanics(t *testing.T) {
	// Arrange
	path := filepath.Join(t.TempDir(), "history.json")
	if err := os.WriteFile(path, []byte(`[{"timestamp":"2026-09-28T00:00:00Z","model":"m","generationId":"g","role":"generator","source":"auto","outcome":"success"}]`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Assert
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic for an entry without targetQuality")
		}
	}()

	// Act
	Open(path)
}
