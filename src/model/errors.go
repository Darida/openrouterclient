package model

import "fmt"

// AttemptsExhaustedError is GenerateText's only returned error. It means
// every allowed attempt failed, timed out, or was rated below TargetQuality.
// Anything unexpected panics instead.
type AttemptsExhaustedError struct {
	Attempts []FailedAttempt
}

type FailedAttempt struct {
	Outcome AttemptOutcome
	// Empty only for OutcomeFailed when OpenRouter never accepted the request,
	// so there is no generation to attribute it to.
	Model        string
	GenerationID string
	// Unusable for every outcome except OutcomeBelowTarget.
	Quality Quality
	// The raw HTTP error body, transport error, or the review's notes.
	Reason string
}

type AttemptOutcome string

const (
	OutcomeFailed      AttemptOutcome = "failed"
	OutcomeTimeout     AttemptOutcome = "timeout"
	OutcomeBelowTarget AttemptOutcome = "below_target"
)

func (e *AttemptsExhaustedError) Error() string {
	return fmt.Sprintf("openrouter: all %d attempts failed, timed out, or fell below target quality: %+v", len(e.Attempts), e.Attempts)
}
