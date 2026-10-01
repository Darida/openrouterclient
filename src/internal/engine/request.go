package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Darida/openrouterclient/src/internal/chat"
	"github.com/Darida/openrouterclient/src/internal/hedge"
	"github.com/Darida/openrouterclient/src/internal/history"
)

// A model or provider that can't serve this particular request answers with
// one of these; whether that's the model's fault is judged once the race ends.
var refusalStatuses = map[int]bool{
	http.StatusBadRequest:          true,
	http.StatusNotFound:            true,
	http.StatusUnprocessableEntity: true,
}

// Only these, and refusals, are the model's fault; anything else is a bug in our request or key.
var retryableStatuses = map[int]bool{
	http.StatusRequestTimeout:      true,
	http.StatusTooManyRequests:     true,
	http.StatusInternalServerError: true,
	http.StatusBadGateway:          true,
	http.StatusServiceUnavailable:  true,
	http.StatusGatewayTimeout:      true,
}

// runAttempt resends a request the provider rejected before generating,
// until something else happens or the attempt's own context ends.
func (e *Engine) runAttempt(ctx context.Context, modelID string, payload []byte, validate validator) (attempt, hedge.Verdict) {
	start := time.Now()
	for resends := 0; ; resends++ {
		result, ok, rejected := e.sendOnce(ctx, modelID, start, payload, validate)
		result.resends = resends
		switch {
		case result.fatal != nil:
			return result, hedge.Aborted
		case ok:
			return result, hedge.Won
		case !rejected:
			return result, hedge.Lost
		}
		select {
		case <-ctx.Done():
			result = e.failed(ctx, modelID, "", start, "provider kept rejecting the request before generating")
			result.resends = resends + 1
			return result, hedge.Lost
		case <-time.After(e.settings.RejectionRetryDelay):
		}
	}
}

// rejected reports a 200 whose body is a provider error other than a rate
// limit. It arrives within a second, before any generation exists, so it's
// resent instead of blamed on the model.
func (e *Engine) sendOnce(ctx context.Context, modelID string, start time.Time, payload []byte, validate validator) (result attempt, ok, rejected bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.settings.ChatURL, bytes.NewReader(payload))
	if err != nil {
		return fatal(modelID, "", fmt.Errorf("engine: build chat request: %w", err)), false, false
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.http.Do(req)
	if err != nil {
		return e.failed(ctx, modelID, "", start, err.Error()), false, false
	}
	defer resp.Body.Close()
	generationID := resp.Header.Get("X-Generation-Id")
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return e.failed(ctx, modelID, generationID, start, err.Error()), false, false
	}

	if resp.StatusCode != http.StatusOK {
		return e.classifyNotOK(ctx, modelID, start, generationID, resp.StatusCode, body), false, false
	}
	if generationID == "" {
		described, err := e.replies.Describe("", "no-generation-id", body)
		if err != nil {
			return fatal(modelID, "", err), false, false
		}
		return fatal(modelID, "", fmt.Errorf("engine: 200 chat response has no X-Generation-Id header — %s", described)), false, false
	}

	return e.classifyOK(ctx, modelID, start, generationID, body, validate)
}

// classifyNotOK sorts a non-200 into a refusal, a model failure, or, for a
// status no model causes, a fatal attempt.
func (e *Engine) classifyNotOK(ctx context.Context, modelID string, start time.Time, generationID string, status int, body []byte) attempt {
	providerErr, err := chat.ParseErrorBody(body, e.replies, generationID)
	if err != nil {
		return fatal(modelID, generationID, err)
	}
	if refusalStatuses[status] {
		described, err := e.replies.Describe(generationID, "refused", body)
		if err != nil {
			return fatal(modelID, generationID, err)
		}
		return e.refused(ctx, modelID, generationID, start, fmt.Sprintf("HTTP %d: %s", status, described))
	}
	if !retryableStatuses[status] {
		described, err := e.replies.Describe(generationID, "unexpected-status", body)
		if err != nil {
			return fatal(modelID, generationID, err)
		}
		return fatal(modelID, generationID, fmt.Errorf("engine: chat request returned HTTP %d: %s", status, described))
	}
	return e.failed(ctx, modelID, generationID, start, fmt.Sprintf("HTTP %d: %v", status, providerErr))
}

// classifyOK sorts a 200 body into success, invalid output, a rate limit, or
// a rejection to resend (see sendOnce).
func (e *Engine) classifyOK(ctx context.Context, modelID string, start time.Time, generationID string, body []byte, validate validator) (result attempt, ok, rejected bool) {
	reply, err := chat.ParseResponse(body, e.replies, generationID)
	if err != nil {
		return fatal(modelID, generationID, err), false, false
	}
	if reply.ProviderError != nil {
		if reply.ProviderError.Code == http.StatusTooManyRequests {
			return e.failed(ctx, modelID, generationID, start, reply.ProviderError.Error()), false, false
		}
		return attempt{}, false, true
	}
	invalid := reply.Invalid
	if invalid == nil {
		if invalid, err = validate(reply.Content); err != nil {
			return fatal(modelID, generationID, err), false, false
		}
	}
	result = attempt{model: modelID, generationID: generationID, content: reply.Content, latency: time.Since(start)}
	if invalid != nil {
		described, err := e.replies.Describe(generationID, generationID, body)
		if err != nil {
			return fatal(modelID, generationID, err), false, false
		}
		result.outcome, result.content, result.reason = history.OutcomeInvalidOutput, nil, fmt.Sprintf("%v — %s", invalid, described)
		return result, false, false
	}
	result.outcome = history.OutcomeSuccess
	return result, true, false
}

func fatal(modelID, generationID string, err error) attempt {
	return attempt{model: modelID, generationID: generationID, fatal: err}
}

// refused names the saved body in the reason, since an all-refused race is
// returned to the caller as its only evidence of what went wrong.
func (e *Engine) refused(ctx context.Context, modelID, generationID string, start time.Time, reason string) attempt {
	result := e.failed(ctx, modelID, generationID, start, reason)
	if result.outcome == history.OutcomeFailed {
		result.outcome = history.OutcomeRefused
	}
	return result
}

func (e *Engine) failed(ctx context.Context, modelID, generationID string, start time.Time, reason string) attempt {
	latency := time.Since(start)
	outcome := history.OutcomeFailed
	switch cause := context.Cause(ctx); {
	case errors.Is(cause, hedge.ErrTimeout):
		outcome, reason = history.OutcomeTimeout, hedge.ErrTimeout.Error()
	case ctx.Err() != nil:
		return attempt{canceled: true, model: modelID, generationID: generationID, latency: latency, reason: reason}
	}
	return attempt{model: modelID, generationID: generationID, outcome: outcome, latency: latency, reason: reason}
}
