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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.settings.ChatURL, bytes.NewReader(payload))
	if err != nil {
		panic(fmt.Sprintf("engine: build chat request: %v", err))
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.http.Do(req)
	if err != nil {
		return e.interrupted(ctx, "", start, err.Error()), false
	}
	defer resp.Body.Close()
	generationID := resp.Header.Get("X-Generation-Id")
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return e.interrupted(ctx, generationID, start, err.Error()), false
	}

	if resp.StatusCode != http.StatusOK {
		if !retryableStatuses[resp.StatusCode] {
			panic(fmt.Sprintf("engine: chat request returned HTTP %d: %s", resp.StatusCode, body))
		}
		return e.interrupted(ctx, generationID, start, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, body)), false
	}
	if generationID == "" {
		panic(fmt.Sprintf("engine: 200 chat response has no X-Generation-Id header — body: %s", body))
	}

	parsed, err := chat.ParseResponse(body)
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

// X-Generation-Id arrives with the headers, long before a slow body, so a cut-short request still resolves to a real model.
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
		result.model = e.log.ResolveModel(generationID)
	}
	return result
}
