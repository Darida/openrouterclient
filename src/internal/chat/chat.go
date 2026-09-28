package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Darida/openrouterclient/src/model"
)

const latencyRankingHintSeconds = 30

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ProviderError is an upstream failure OpenRouter relays as an "error" object,
// either in a non-200 body or in a 200 body whose headers were already sent.
type ProviderError struct {
	Code     int             `json:"code"`
	Message  string          `json:"message"`
	Metadata json.RawMessage `json:"metadata"`
}

// Error keeps one line: the upstream's own first sentence when it sends one
// (metadata.raw), since OpenRouter's message is often a generic wrapper.
func (p *ProviderError) Error() string {
	var metadata struct {
		Raw string `json:"raw"`
	}
	json.Unmarshal(p.Metadata, &metadata)
	detail := p.Message
	if metadata.Raw != "" {
		detail, _, _ = strings.Cut(metadata.Raw, ". ")
	}
	return fmt.Sprintf("provider error %d: %s", p.Code, detail)
}

func UserMessage(content string) Message      { return Message{Role: "user", Content: content} }
func AssistantMessage(content string) Message { return Message{Role: "assistant", Content: content} }

func BuildPayload(messages []Message, schema model.JSONSchema, modelID string, maxTokens int) []byte {
	payload := map[string]any{
		"model":      modelID,
		"max_tokens": maxTokens,
		"response_format": map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": schema.Name, "strict": true, "schema": schema.Schema},
		},
		"provider": map[string]any{"require_parameters": true, "preferred_max_latency": latencyRankingHintSeconds, "allow_fallbacks": false},
		"messages": messages,
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

// ParseResponse returns a *ProviderError for a relayed upstream failure.
// Otherwise it panics if the body isn't JSON; a missing or non-JSON message
// content is the model's fault, so it comes back as an error.
func ParseResponse(body []byte) (json.RawMessage, error) {
	var parsed struct {
		Error   *ProviderError `json:"error"`
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
		return nil, parsed.Error
	}
	if len(parsed.Choices) == 0 || parsed.Choices[0].Message.Content == "" {
		return nil, errors.New("response has no message content")
	}
	raw := parsed.Choices[0].Message.Content
	if !json.Valid([]byte(raw)) {
		return nil, fmt.Errorf("message content is not valid JSON: %s", raw)
	}
	return json.RawMessage(raw), nil
}
