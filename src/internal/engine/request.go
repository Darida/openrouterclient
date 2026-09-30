package engine

import (
	"bytes"
	"context"
	"encoding/json"
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
func (e *Engine) runAttempt(ctx context.Context, modelID string, payload []byte, validate func(json.RawMessage) error) (attempt, bool) {
	start := time.Now()
	for resends := 0; ; resends++ {
		result, ok, rejected := e.sendOnce(ctx, modelID, start, payload, validate)
		result.resends = resends
		if !rejected {
			return result, ok
		}
		select {
		case <-ctx.Done():
			result = e.failed(ctx, modelID, "", start, "provider kept rejecting the request before generating")
			result.resends = resends + 1
			return result, false
		case <-time.After(e.settings.RejectionRetryDelay):
		}
	}
}

// rejected reports a 200 whose body is a provider error other than a rate
// limit. It arrives within a second, before any generation exists, so it's
// resent instead of blamed on the model.
func (e *Engine) sendOnce(ctx context.Context, modelID string, start time.Time, payload []byte, validate func(json.RawMessage) error) (result attempt, ok, rejected bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.settings.ChatURL, bytes.NewReader(payload))
	if err != nil {
		panic(fmt.Sprintf("engine: build chat request: %v", err))
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
		providerErr := chat.ParseErrorBody(body, e.replies, generationID)
		if refusalStatuses[resp.StatusCode] {
			return e.refused(ctx, modelID, generationID, start, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, e.replies.Describe(generationID, "refused", body))), false, false
		}
		if !retryableStatuses[resp.StatusCode] {
			panic(fmt.Sprintf("engine: chat request returned HTTP %d: %s", resp.StatusCode, e.replies.Describe(generationID, "unexpected-status", body)))
		}
		return e.failed(ctx, modelID, generationID, start, fmt.Sprintf("HTTP %d: %v", resp.StatusCode, providerErr)), false, false
	}
	if generationID == "" {
		panic(fmt.Sprintf("engine: 200 chat response has no X-Generation-Id header — %s", e.replies.Describe("", "no-generation-id", body)))
	}

	return e.classifyOK(ctx, modelID, start, generationID, body, validate)
}

// classifyOK sorts a 200 body into success, invalid output, a rate limit, or
// a rejection to resend (see sendOnce).
func (e *Engine) classifyOK(ctx context.Context, modelID string, start time.Time, generationID string, body []byte, validate func(json.RawMessage) error) (result attempt, ok, rejected bool) {
	content, err := chat.ParseResponse(body, e.replies, generationID)
	var providerErr *chat.ProviderError
	if errors.As(err, &providerErr) {
		if providerErr.Code == http.StatusTooManyRequests {
			return e.failed(ctx, modelID, generationID, start, providerErr.Error()), false, false
		}
		return attempt{}, false, true
	}
	if err == nil {
		err = validate(content)
	}
	result = attempt{model: modelID, generationID: generationID, content: content, latency: time.Since(start)}
	if err != nil {
		result.outcome, result.content, result.reason = history.OutcomeInvalidOutput, nil, fmt.Sprintf("%v — %s", err, e.replies.Describe(generationID, generationID, body))
		return result, false, false
	}
	result.outcome = history.OutcomeSuccess
	return result, true, false
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
