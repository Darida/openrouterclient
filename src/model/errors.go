package model

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AttemptsExhaustedError means every allowed attempt failed, timed out, or
// was rated below TargetQuality.
type AttemptsExhaustedError struct {
	Attempts []FailedAttempt
}

// UnexpectedError means the call stopped on something that is neither a
// model's failure nor the caller's context ending: an invalid request, a
// reply OpenRouter isn't known to send, or a history file that can't be read
// or written. Nothing is retried after it. Its message names the file holding
// any raw reply rather than quoting it.
type UnexpectedError struct {
	Err error
}

func (e *UnexpectedError) Error() string { return "openrouter: unexpected: " + e.Err.Error() }

func (e *UnexpectedError) Unwrap() error { return e.Err }

type FailedAttempt struct {
	Outcome AttemptOutcome
	// Empty only when OpenRouter never accepted the request, so there is no
	// generation to attribute it to.
	Model        string
	GenerationID string
	// Unusable for every outcome except OutcomeBelowTarget.
	Quality Quality
	// What went wrong, naming the file that holds any raw reply rather than
	// quoting it.
	Reason string
	// Set only for OutcomeBelowTarget: the schema-valid output the review
	// rejected.
	Content json.RawMessage
	// Set only for OutcomeBelowTarget: the review that rejected Content. Rating
	// its GenerationID low clears the rating it gave.
	Review *Review
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
	details := strings.Join(parts, "; ")
	if e.reviewRejected() {
		return fmt.Sprintf("openrouter: review rejected the output: %s", details)
	}
	return fmt.Sprintf("openrouter: all %d attempts failed: %s", len(e.Attempts), details)
}

func (e *AttemptsExhaustedError) reviewRejected() bool {
	for _, a := range e.Attempts {
		if a.Outcome == OutcomeBelowTarget {
			return true
		}
	}
	return false
}
