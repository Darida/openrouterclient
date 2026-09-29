package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Darida/openrouterclient/src/model"
)

func generatorFields(generationID string, q, target model.Quality, at time.Time) EntryFields {
	return EntryFields{Timestamp: at, Model: "m", GenerationID: generationID, Role: RoleGenerator, Source: SourceAuto, Outcome: OutcomeSuccess, Quality: q, TargetQuality: target, TimeoutSeconds: 60, Tag: "t"}
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

func TestEntryNew_whenFailedOutcomeHasRatedQuality_thenPanics(t *testing.T) {
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

func refusedFields(at time.Time) EntryFields {
	return EntryFields{Timestamp: at, Model: "m", Role: RoleGenerator, Source: SourceAuto, Outcome: OutcomeRefused, Quality: model.QualityUnusable, TargetQuality: model.QualityHigh, TimeoutSeconds: 60, Tag: "t"}
}

func TestEntryNew_whenRefusedOutcomeLacksGenerationID_thenBuildsEntry(t *testing.T) {
	// Arrange
	fields := refusedFields(time.Now())

	// Act
	entry := NewEntry(fields)

	// Assert
	if entry.Fields().Outcome != OutcomeRefused {
		t.Fatalf("entry = %+v, want a refused entry", entry.Fields())
	}
}

func TestEntryNew_whenFailedOutcomeLacksGenerationID_thenPanics(t *testing.T) {
	// Arrange
	fields := refusedFields(time.Now())
	fields.Outcome = OutcomeFailed

	// Assert
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()

	// Act
	NewEntry(fields)
}

func TestStoreRecordManual_whenGenerationIDEmpty_thenErrors(t *testing.T) {
	// Arrange
	store := Open(filepath.Join(t.TempDir(), "history.json"))
	store.Append(NewEntry(refusedFields(time.Now())))

	// Act
	err := store.RecordManual("", model.QualityHigh, "good", time.Now())

	// Assert
	if err == nil {
		t.Fatal("expected an error for an empty generation id")
	}
}

func TestEntryNew_whenTimeoutMissing_thenPanics(t *testing.T) {
	// Arrange
	fields := generatorFields("g", model.QualityHigh, model.QualityHigh, time.Now())
	fields.TimeoutSeconds = 0

	// Assert
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()

	// Act
	NewEntry(fields)
}

func reviewerFields(generationID, reviewedID string, at time.Time) EntryFields {
	return EntryFields{Timestamp: at, Model: "r", GenerationID: generationID, Role: RoleReviewer, Source: SourceAuto, Outcome: OutcomeSuccess, TimeoutSeconds: 60, Tag: "t", ReviewedGenerationID: reviewedID}
}

func fieldsOf(t *testing.T, store *Store, generationID string, source Source) EntryFields {
	for _, e := range store.load() {
		if f := e.Fields(); f.GenerationID == generationID && f.Source == source {
			return f
		}
	}
	t.Fatalf("no %s entry for %q", source, generationID)
	return EntryFields{}
}

func TestStoreRecordManual_whenReviewerRatedLow_thenReviewedGenerationUnrated(t *testing.T) {
	// Arrange
	store := Open(filepath.Join(t.TempDir(), "history.json"))
	now := time.Now()
	store.Append(NewEntry(generatorFields("gen-g", model.QualityLow, model.QualityHigh, now)))
	store.Append(NewEntry(reviewerFields("gen-r", "gen-g", now)))

	// Act
	if err := store.RecordManual("gen-r", model.QualityLow, "misread rule 1", now); err != nil {
		t.Fatalf("RecordManual: %v", err)
	}

	// Assert
	if got := fieldsOf(t, store, "gen-g", SourceAuto).Quality; got != "" {
		t.Fatalf("reviewed generation quality = %q, want empty", got)
	}
}

func TestStoreRecordManual_whenReviewerRatedHigh_thenReviewedGenerationKeepsRating(t *testing.T) {
	// Arrange
	store := Open(filepath.Join(t.TempDir(), "history.json"))
	now := time.Now()
	store.Append(NewEntry(generatorFields("gen-g", model.QualityLow, model.QualityHigh, now)))
	store.Append(NewEntry(reviewerFields("gen-r", "gen-g", now)))

	// Act
	if err := store.RecordManual("gen-r", model.QualityHigh, "spot on", now); err != nil {
		t.Fatalf("RecordManual: %v", err)
	}

	// Assert
	if got := fieldsOf(t, store, "gen-g", SourceAuto).Quality; got != model.QualityLow {
		t.Fatalf("reviewed generation quality = %q, want low", got)
	}
}

func TestStoreRecordManual_whenReviewerRatedLow_thenReviewedGenerationManualRatingStays(t *testing.T) {
	// Arrange
	store := Open(filepath.Join(t.TempDir(), "history.json"))
	now := time.Now()
	store.Append(NewEntry(generatorFields("gen-g", model.QualityLow, model.QualityHigh, now)))
	store.Append(NewEntry(reviewerFields("gen-r", "gen-g", now)))
	if err := store.RecordManual("gen-g", model.QualityMedium, "fine by me", now); err != nil {
		t.Fatalf("setup: rating generation failed: %v", err)
	}

	// Act
	if err := store.RecordManual("gen-r", model.QualityLow, "misread rule 1", now); err != nil {
		t.Fatalf("RecordManual: %v", err)
	}

	// Assert
	if got := fieldsOf(t, store, "gen-g", SourceManual).Quality; got != model.QualityMedium {
		t.Fatalf("manual generation quality = %q, want medium", got)
	}
}

func TestStoreRecordManual_whenReviewerWithoutLinkRatedLow_thenGenerationKeepsRating(t *testing.T) {
	// Arrange
	store := Open(filepath.Join(t.TempDir(), "history.json"))
	now := time.Now()
	store.Append(NewEntry(generatorFields("gen-g", model.QualityLow, model.QualityHigh, now)))
	store.Append(NewEntry(reviewerFields("gen-r", "", now)))

	// Act
	if err := store.RecordManual("gen-r", model.QualityLow, "straggler was wrong", now); err != nil {
		t.Fatalf("RecordManual: %v", err)
	}

	// Assert
	if got := fieldsOf(t, store, "gen-g", SourceAuto).Quality; got != model.QualityLow {
		t.Fatalf("generation quality = %q, want low", got)
	}
}

func TestStoreRecordManual_whenReviewersRatedLow_thenReviewerModelExcluded(t *testing.T) {
	// Arrange
	store := Open(filepath.Join(t.TempDir(), "history.json"))
	now := time.Now()
	for i := 0; i < 4; i++ {
		suffix := string(rune('a' + i))
		store.Append(NewEntry(generatorFields("gen-g"+suffix, model.QualityHigh, model.QualityHigh, now)))
		store.Append(NewEntry(reviewerFields("gen-r"+suffix, "gen-g"+suffix, now)))
		if err := store.RecordManual("gen-r"+suffix, model.QualityLow, "wrong", now); err != nil {
			t.Fatalf("setup: rating failed: %v", err)
		}
	}

	// Act
	got := store.Exclusions(now, "t")

	// Assert
	if len(got.Excluded) != 1 || got.Excluded[0] != "r" {
		t.Fatalf("excluded = %v, want [r]", got.Excluded)
	}
}

func TestEntryNew_whenGeneratorNamesReviewedGeneration_thenPanics(t *testing.T) {
	// Arrange
	fields := generatorFields("gen-g", model.QualityHigh, model.QualityHigh, time.Now())
	fields.ReviewedGenerationID = "gen-other"

	// Assert
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()

	// Act
	NewEntry(fields)
}
