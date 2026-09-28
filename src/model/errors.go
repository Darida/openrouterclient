package model

import "fmt"

// AttemptsExhaustedError means every allowed attempt failed, timed out, or
// was rated below TargetQuality.
type AttemptsExhaustedError struct {
	Attempts []FailedAttempt
}

type FailedAttempt struct {
	Outcome AttemptOutcome
	// Empty only when OpenRouter never accepted the request, so there is no
	// generation to attribute it to.
	Model        string
	GenerationID string
	// Unusable for every outcome except OutcomeBelowTarget.
	Quality Quality
	// The raw HTTP error body, transport error, or the review's notes.
	Reason string
}

type AttemptOutcome string

const (
	OutcomeFailed  AttemptOutcome = "failed"
	OutcomeTimeout AttemptOutcome = "timeout"
	// Cut short because a parallel attempt already succeeded.
	OutcomeAborted AttemptOutcome = "aborted"
	// The reply was missing, not JSON, or did not match the requested schema.
	OutcomeInvalidOutput AttemptOutcome = "invalid_output"
	OutcomeBelowTarget   AttemptOutcome = "below_target"
)

func (e *AttemptsExhaustedError) Error() string {
	return fmt.Sprintf("openrouter: all %d attempts failed, timed out, or fell below target quality: %+v", len(e.Attempts), e.Attempts)
}
