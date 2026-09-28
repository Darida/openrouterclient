package openrouterclient

import "fmt"

// AttemptsExhaustedError is GenerateText's only returned error. It means
// every allowed attempt ended in a failure, a timeout, or a rejection.
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
	// The raw HTTP error body, transport error, or rejecting review notes.
	Reason string
}

type AttemptOutcome string

const (
	OutcomeFailed       AttemptOutcome = "failed"
	OutcomeTimeout      AttemptOutcome = "timeout"
	OutcomeAutoRejected AttemptOutcome = "auto_rejected"
)

func (e *AttemptsExhaustedError) Error() string {
	return fmt.Sprintf("openrouter: all %d attempts failed, timed out, or were rejected: %+v", len(e.Attempts), e.Attempts)
}
