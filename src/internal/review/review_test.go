package review

import (
	"encoding/json"
	"testing"
)

func TestReviewValidate_whenViolationsMatchPositiveTotal_thenAccepts(t *testing.T) {
	// Act
	err := Validate(json.RawMessage(`{"violations":[{"rule":"1","evidence":"apple","explanation":"x","recommendedAction":"Use a yellow fruit."}],"totalBadScore":2}`))

	// Assert
	if err != nil {
		t.Fatalf("got %v, want nil", err)
	}
}

func TestReviewValidate_whenTotalMissing_thenRejects(t *testing.T) {
	// Act
	err := Validate(json.RawMessage(`{"violations":[]}`))

	// Assert
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestReviewValidate_whenTotalNegative_thenRejects(t *testing.T) {
	// Act
	err := Validate(json.RawMessage(`{"violations":[{"rule":"1","evidence":"apple","explanation":"x","recommendedAction":"Use a yellow fruit."}],"totalBadScore":-1}`))

	// Assert
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestReviewValidate_whenTotalPositiveWithoutViolations_thenRejects(t *testing.T) {
	// Act
	err := Validate(json.RawMessage(`{"violations":[],"totalBadScore":1}`))

	// Assert
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestReviewValidate_whenTotalZeroWithViolations_thenRejects(t *testing.T) {
	// Act
	err := Validate(json.RawMessage(`{"violations":[{"rule":"1","evidence":"apple","explanation":"x","recommendedAction":"Use a yellow fruit."}],"totalBadScore":0}`))

	// Assert
	if err == nil {
		t.Fatal("expected an error")
	}
}
