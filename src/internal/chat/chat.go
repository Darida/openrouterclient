package chat

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Darida/openrouterclient/src/model"
)

// Routes to OpenRouter's free models only; the exclusion list narrows which.
const routerModel = "openrouter/free"

// A ranking hint that weights provider choice by historical p50, not an
// enforced cutoff. The hedge's timeouts are what actually bound a request.
const preferredMaxLatencySeconds = 30

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Response struct {
	Model   string
	Content json.RawMessage
}

func UserMessage(content string) Message      { return Message{Role: "user", Content: content} }
func AssistantMessage(content string) Message { return Message{Role: "assistant", Content: content} }

func BuildPayload(messages []Message, schema model.JSONSchema, excludedModels []string) []byte {
	plugins := []any{}
	if len(excludedModels) > 0 {
		allowed := []string{"*"}
		for _, m := range excludedModels {
			allowed = append(allowed, "!"+m)
		}
		plugins = append(plugins, map[string]any{"id": "auto-router", "allowed_models": allowed})
	}
	payload := map[string]any{
		"model": routerModel,
		"response_format": map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": schema.Name, "strict": true, "schema": schema.Schema},
		},
		"provider":  map[string]any{"require_parameters": true, "preferred_max_latency": preferredMaxLatencySeconds},
		"reasoning": map[string]any{"exclude": true, "effort": "low"},
		"plugins":   plugins,
		"messages":  messages,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("chat: marshal payload: %v", err))
	}
	return body
}

// ParseResponse panics if a 200 body isn't JSON or names no model, since then
// nothing can be attributed. A missing or non-JSON message content is the
// model's fault, so it comes back as an error alongside the known model.
func ParseResponse(body []byte) (Response, error) {
	var parsed struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		panic(fmt.Sprintf("chat: 200 response body is not JSON: %v — body: %s", err, body))
	}
	if parsed.Model == "" {
		panic(fmt.Sprintf("chat: 200 response has no model — body: %s", body))
	}
	response := Response{Model: parsed.Model}
	if len(parsed.Choices) == 0 || parsed.Choices[0].Message.Content == "" {
		return response, errors.New("response has no message content")
	}
	raw := parsed.Choices[0].Message.Content
	if !json.Valid([]byte(raw)) {
		return response, fmt.Errorf("message content is not valid JSON: %s", raw)
	}
	response.Content = json.RawMessage(raw)
	return response, nil
}
