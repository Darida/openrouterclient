package model

import (
	"fmt"
	"strings"
)

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
	// The reply was missing, not JSON, or did not match the requested schema.
	OutcomeInvalidOutput AttemptOutcome = "invalid_output"
	OutcomeBelowTarget   AttemptOutcome = "below_target"
	// The model or its provider refused the request (HTTP 400, 404, or 422),
	// for example over a schema keyword it doesn't support or an account
	// data policy that excludes its only endpoint.
	OutcomeRefused AttemptOutcome = "refused"
)

// Error groups identical failures into one "model outcome ×N (reason)" entry each.
func (e *AttemptsExhaustedError) Error() string {
	type key struct {
		model   string
		outcome AttemptOutcome
		reason  string
	}
	var order []key
	counts := map[key]int{}
	for _, a := range e.Attempts {
		k := key{a.Model, a.Outcome, a.Reason}
		if counts[k] == 0 {
			order = append(order, k)
		}
		counts[k]++
	}
	parts := make([]string, len(order))
	for i, k := range order {
		parts[i] = fmt.Sprintf("%s %s ×%d (%s)", k.model, k.outcome, counts[k], k.reason)
	}
	return fmt.Sprintf("openrouter: all %d attempts failed: %s", len(e.Attempts), strings.Join(parts, "; "))
}
