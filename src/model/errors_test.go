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
