package review

import (
	"encoding/json"
	"testing"
)

func TestReviewValidate_whenNotesMatchPositiveTotal_thenAccepts(t *testing.T) {
	// Act
	err := Validate(json.RawMessage(`{"notes":[{"rule":"1","text":"x"}],"totalBadScore":2}`))

	// Assert
	if err != nil {
		t.Fatalf("got %v, want nil", err)
	}
}

func TestReviewValidate_whenTotalMissing_thenRejects(t *testing.T) {
	// Act
	err := Validate(json.RawMessage(`{"notes":[]}`))

	// Assert
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestReviewValidate_whenTotalNegative_thenRejects(t *testing.T) {
	// Act
	err := Validate(json.RawMessage(`{"notes":[{"rule":"1","text":"x"}],"totalBadScore":-1}`))

	// Assert
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestReviewValidate_whenTotalPositiveWithoutNotes_thenRejects(t *testing.T) {
	// Act
	err := Validate(json.RawMessage(`{"notes":[],"totalBadScore":1}`))

	// Assert
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestReviewValidate_whenTotalZeroWithNotes_thenRejects(t *testing.T) {
	// Act
	err := Validate(json.RawMessage(`{"notes":[{"rule":"1","text":"x"}],"totalBadScore":0}`))

	// Assert
	if err == nil {
		t.Fatal("expected an error")
	}
}
