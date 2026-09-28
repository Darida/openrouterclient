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

func (e *Engine) runAttempt(ctx context.Context, payload []byte, validate func(json.RawMessage) error) (attempt, bool) {
	start := time.Now()
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
		return e.interrupted(ctx, "", start, err.Error()), false
	}
	generationID := resp.Header.Get("X-Generation-Id")
	e.logger.Info("openrouter: response headers", "status", resp.StatusCode, "generationId", generationID, "latency", time.Since(start))
	body, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	killConnection()
	if readErr != nil {
		return e.interrupted(ctx, generationID, start, readErr.Error()), false
	}

	if resp.StatusCode != http.StatusOK {
		providerErr := chat.ParseErrorBody(body)
		e.logger.Warn("openrouter: non-200 response", "status", resp.StatusCode, "generationId", generationID, "error", providerErr)
		if !retryableStatuses[resp.StatusCode] {
			panic(fmt.Sprintf("engine: chat request returned HTTP %d: %s", resp.StatusCode, body))
		}
		return e.interrupted(ctx, generationID, start, fmt.Sprintf("HTTP %d: %v", resp.StatusCode, providerErr)), false
	}
	if generationID == "" {
		panic(fmt.Sprintf("engine: 200 chat response has no X-Generation-Id header — body: %s", body))
	}

	parsed, err := chat.ParseResponse(body)
	var providerErr *chat.ProviderError
	if errors.As(err, &providerErr) {
		e.logger.Warn("openrouter: provider error in 200 response", "generationId", generationID, "error", providerErr)
		return e.interrupted(ctx, generationID, start, providerErr.Error()), false
	}
	if err == nil {
		err = validate(parsed.Content)
	}
	result := attempt{model: parsed.Model, generationID: generationID, content: parsed.Content, latency: time.Since(start)}
	if err != nil {
		result.outcome, result.content, result.reason = history.OutcomeInvalidOutput, nil, err.Error()
		return result, false
	}
	result.outcome = history.OutcomeSuccess
	return result, true
}

// X-Generation-Id arrives with the headers, so a request that failed after
// that still resolves to a real model through the generation log.
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
	result := attempt{generationID: generationID, outcome: outcome, latency: latency, reason: reason}
	if generationID != "" {
		e.logger.Warn("openrouter: resolving model from generation log", "generationId", generationID, "outcome", outcome, "reason", reason)
		result.model = e.catalog.ModelID(e.log.ResolveModel(generationID))
	}
	return result
}
