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

// Only these are the model's fault; anything else is a bug in our request or key.
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
func (e *Engine) runAttempt(ctx context.Context, payload []byte, validate func(json.RawMessage) error) (attempt, bool) {
	start := time.Now()
	for {
		result, ok, rejected := e.sendOnce(ctx, start, payload, validate)
		if !rejected {
			return result, ok
		}
		select {
		case <-ctx.Done():
			return e.interrupted(ctx, "", start, "provider kept rejecting the request before generating"), false
		case <-time.After(e.settings.RejectionRetryDelay):
		}
	}
}

// rejected reports a 200 whose body is a provider error other than a rate
// limit. It arrives within a second, before any generation exists, so no
// model can be blamed and the log never has it.
func (e *Engine) sendOnce(ctx context.Context, start time.Time, payload []byte, validate func(json.RawMessage) error) (result attempt, ok, rejected bool) {
	// OpenRouter only logs a generation once its connection is gone, so every
	// path kills the connection before interrupted polls the log.
	requestCtx, killConnection := context.WithCancel(ctx)
	defer killConnection()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, e.settings.ChatURL, bytes.NewReader(payload))
	if err != nil {
		panic(fmt.Sprintf("engine: build chat request: %v", err))
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.http.Do(req)
	if err != nil {
		killConnection()
		return e.interrupted(ctx, "", start, err.Error()), false, false
	}
	generationID := resp.Header.Get("X-Generation-Id")
	e.logger.Info("openrouter: response headers", "status", resp.StatusCode, "generationId", generationID, "latency", time.Since(start))
	body, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	killConnection()
	if readErr != nil {
		return e.interrupted(ctx, generationID, start, readErr.Error()), false, false
	}

	if resp.StatusCode != http.StatusOK {
		providerErr := chat.ParseErrorBody(body)
		e.logger.Warn("openrouter: non-200 response", "status", resp.StatusCode, "generationId", generationID, "error", providerErr)
		if !retryableStatuses[resp.StatusCode] {
			panic(fmt.Sprintf("engine: chat request returned HTTP %d: %s", resp.StatusCode, body))
		}
		return e.providerFailure(ctx, generationID, start, providerErr, fmt.Sprintf("HTTP %d: %v", resp.StatusCode, providerErr)), false, false
	}
	if generationID == "" {
		panic(fmt.Sprintf("engine: 200 chat response has no X-Generation-Id header — body: %s", body))
	}

	parsed, err := chat.ParseResponse(body)
	var providerErr *chat.ProviderError
	if errors.As(err, &providerErr) {
		if providerErr.Code == http.StatusTooManyRequests {
			e.logger.Warn("openrouter: rate limited in 200 response", "generationId", generationID, "error", providerErr)
			return e.providerFailure(ctx, generationID, start, providerErr, providerErr.Error()), false, false
		}
		e.logger.Warn("openrouter: provider rejected before generating; resending", "generationId", generationID, "error", providerErr)
		return attempt{}, false, true
	}
	if err == nil {
		err = validate(parsed.Content)
	}
	result = attempt{model: parsed.Model, generationID: generationID, content: parsed.Content, latency: time.Since(start)}
	if err != nil {
		result.outcome, result.content, result.reason = history.OutcomeInvalidOutput, nil, err.Error()
		return result, false, false
	}
	result.outcome = history.OutcomeSuccess
	return result, true, false
}

// A rate-limited request never ran, so its model comes from the error text
// instead of the generation log.
func (e *Engine) providerFailure(ctx context.Context, generationID string, start time.Time, providerErr *chat.ProviderError, reason string) attempt {
	if providerErr.Code != http.StatusTooManyRequests {
		return e.interrupted(ctx, generationID, start, reason)
	}
	return attempt{model: providerErr.RateLimitedModel(), generationID: generationID, outcome: history.OutcomeFailed, latency: time.Since(start), reason: reason}
}

// interrupted leaves model empty for recordAttempts to resolve from the
// generation log, so a slow log never holds up the hedge's next attempt.
func (e *Engine) interrupted(ctx context.Context, generationID string, start time.Time, reason string) attempt {
	latency := time.Since(start)
	outcome := history.OutcomeFailed
	switch cause := context.Cause(ctx); {
	case errors.Is(cause, hedge.ErrAborted):
		outcome, reason = history.OutcomeAborted, hedge.ErrAborted.Error()
	case errors.Is(cause, hedge.ErrTimeout):
		outcome, reason = history.OutcomeTimeout, hedge.ErrTimeout.Error()
	case ctx.Err() != nil:
		return attempt{canceled: true, generationID: generationID, latency: latency, reason: reason}
	}
	return attempt{generationID: generationID, outcome: outcome, latency: latency, reason: reason}
}
