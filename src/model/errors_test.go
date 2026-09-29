package model

import "testing"

func TestAttemptsExhaustedErrorError_whenFailuresRepeat_thenGroupsThem(t *testing.T) {
	// Arrange
	failure := FailedAttempt{Outcome: OutcomeFailed, Model: "qwen/qwen3.8-27b:free", Quality: QualityUnusable, Reason: "HTTP 429: provider error 429: rate-limited upstream"}
	err := &AttemptsExhaustedError{Attempts: []FailedAttempt{failure, failure, failure}}

	// Act
	got := err.Error()

	// Assert
	want := "openrouter: all 3 attempts failed: qwen/qwen3.8-27b:free failed ×3 (HTTP 429: provider error 429: rate-limited upstream)"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestAttemptsExhaustedErrorError_whenAnyAttemptBelowTarget_thenSaysReviewRejected(t *testing.T) {
	// Arrange
	rejected := FailedAttempt{Outcome: OutcomeBelowTarget, Model: "qwen/qwen3.8-27b:free", Quality: QualityLow, Reason: "missing price"}
	timeout := FailedAttempt{Outcome: OutcomeTimeout, Model: "qwen/qwen3.8-27b:free", Quality: QualityUnusable, Reason: "no response"}
	err := &AttemptsExhaustedError{Attempts: []FailedAttempt{rejected, timeout}}

	// Act
	got := err.Error()

	// Assert
	want := "openrouter: review rejected the output: qwen/qwen3.8-27b:free below_target ×1 (missing price); qwen/qwen3.8-27b:free timeout ×1 (no response)"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}
