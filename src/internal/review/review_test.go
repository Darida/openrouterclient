package review

import (
	"encoding/json"
	"testing"
)

func TestReviewValidate_whenViolationsMatchPositiveTotal_thenAccepts(t *testing.T) {
	// Act
	invalid, err := Validate(json.RawMessage(`{"violations":[{"rule":"1","evidence":"apple","explanation":"x","recommendedAction":"Use a yellow fruit.","badScore":2}],"totalBadScore":2}`))

	// Assert
	if invalid != nil || err != nil {
		t.Fatalf("got %v, %v; want nil", invalid, err)
	}
}

func TestReviewValidate_whenTotalMissing_thenRejects(t *testing.T) {
	// Act
	invalid, err := Validate(json.RawMessage(`{"violations":[]}`))

	// Assert
	if invalid == nil || err != nil {
		t.Fatalf("got %v, %v; want the verdict rejected", invalid, err)
	}
}

func TestReviewValidate_whenTotalNegative_thenRejects(t *testing.T) {
	// Act
	invalid, err := Validate(json.RawMessage(`{"violations":[{"rule":"1","evidence":"apple","explanation":"x","recommendedAction":"Use a yellow fruit.","badScore":1}],"totalBadScore":-1}`))

	// Assert
	if invalid == nil || err != nil {
		t.Fatalf("got %v, %v; want the verdict rejected", invalid, err)
	}
}

func TestReviewValidate_whenTotalPositiveWithoutViolations_thenRejects(t *testing.T) {
	// Act
	invalid, err := Validate(json.RawMessage(`{"violations":[],"totalBadScore":1}`))

	// Assert
	if invalid == nil || err != nil {
		t.Fatalf("got %v, %v; want the verdict rejected", invalid, err)
	}
}

func TestReviewValidate_whenTotalZeroWithViolations_thenRejects(t *testing.T) {
	// Act
	invalid, err := Validate(json.RawMessage(`{"violations":[{"rule":"1","evidence":"apple","explanation":"x","recommendedAction":"Use a yellow fruit.","badScore":0}],"totalBadScore":0}`))

	// Assert
	if invalid == nil || err != nil {
		t.Fatalf("got %v, %v; want the verdict rejected", invalid, err)
	}
}

func TestReviewValidate_whenTotalDiffersFromBadScoreSum_thenRejects(t *testing.T) {
	// Act
	invalid, err := Validate(json.RawMessage(`{"violations":[{"rule":"1","evidence":"apple","explanation":"x","recommendedAction":"Use a yellow fruit.","badScore":5},{"rule":"2","evidence":"pear","explanation":"y","recommendedAction":"Use a lemon.","badScore":3}],"totalBadScore":5}`))

	// Assert
	if invalid == nil || err != nil {
		t.Fatalf("got %v, %v; want the verdict rejected", invalid, err)
	}
}

func TestReviewValidate_whenRuleRepeatsAndTotalSumsEveryInstance_thenAccepts(t *testing.T) {
	// Act
	invalid, err := Validate(json.RawMessage(`{"violations":[{"rule":"1","evidence":"apple","explanation":"x","recommendedAction":"Use a yellow fruit.","badScore":5},{"rule":"1","evidence":"pear","explanation":"y","recommendedAction":"Use a lemon.","badScore":5}],"totalBadScore":10}`))

	// Assert
	if invalid != nil || err != nil {
		t.Fatalf("got %v, %v; want nil", invalid, err)
	}
}
