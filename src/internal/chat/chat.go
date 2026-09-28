package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/Darida/openrouterclient/src/model"
)

const latencyRankingHintSeconds = 30

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Response struct {
	Model   string
	Content json.RawMessage
}

// ProviderError is an upstream failure OpenRouter relays as an "error" object,
// either in a non-200 body or in a 200 body whose headers were already sent.
type ProviderError struct {
	Code     int             `json:"code"`
	Message  string          `json:"message"`
	Metadata json.RawMessage `json:"metadata"`
}

// A rate limit is rejected before any generation exists, so the log never
// has it; the model appears only in the free-text metadata.raw message.
var rateLimitedModelPattern = regexp.MustCompile(`^(\S+/\S+) is temporarily rate-limited upstream`)

// RateLimitedModel panics unless metadata.raw names the model in the known wording.
func (p *ProviderError) RateLimitedModel() string {
	var metadata struct {
		Raw string `json:"raw"`
	}
	if err := json.Unmarshal(p.Metadata, &metadata); err != nil {
		panic(fmt.Sprintf("chat: rate-limit metadata is not JSON: %v — %s", err, p.Metadata))
	}
	match := rateLimitedModelPattern.FindStringSubmatch(metadata.Raw)
	if match == nil {
		panic(fmt.Sprintf("chat: rate-limit message names no model in the known wording: %q", metadata.Raw))
	}
	return match[1]
}

func (p *ProviderError) Error() string {
	return fmt.Sprintf("provider error %d: %s (metadata: %s)", p.Code, p.Message, p.Metadata)
}

func UserMessage(content string) Message      { return Message{Role: "user", Content: content} }
func AssistantMessage(content string) Message { return Message{Role: "assistant", Content: content} }

func BuildPayload(messages []Message, schema model.JSONSchema, modelID string) []byte {
	payload := map[string]any{
		"model": modelID,
		"response_format": map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": schema.Name, "strict": true, "schema": schema.Schema},
		},
		"provider":  map[string]any{"require_parameters": true, "preferred_max_latency": latencyRankingHintSeconds, "allow_fallbacks": false},
		"reasoning": map[string]any{"exclude": true, "effort": "low"},
		"messages":  messages,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("chat: marshal payload: %v", err))
	}
	return body
}

// ParseErrorBody panics on a non-200 body that isn't an OpenRouter error object.
func ParseErrorBody(body []byte) *ProviderError {
	var parsed struct {
		Error *ProviderError `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.Error == nil || parsed.Error.Message == "" {
		panic(fmt.Sprintf("chat: error response is not an OpenRouter error object — body: %s", body))
	}
	return parsed.Error
}

// ParseResponse returns a *ProviderError for a relayed upstream failure, which
// names no model. Otherwise it panics if the body isn't JSON or names no model.
// A missing or non-JSON message content is the model's fault, so it comes back
// as an error alongside the known model.
func ParseResponse(body []byte) (Response, error) {
	var parsed struct {
		Error   *ProviderError `json:"error"`
		Model   string         `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		panic(fmt.Sprintf("chat: 200 response body is not JSON: %v — body: %s", err, body))
	}
	if parsed.Error != nil {
		return Response{}, parsed.Error
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
