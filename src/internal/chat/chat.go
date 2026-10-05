package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Darida/openrouterclient/src/internal/catalog"
	"github.com/Darida/openrouterclient/src/internal/replyfile"
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

func SystemMessage(content string) Message    { return Message{Role: "system", Content: content} }
func UserMessage(content string) Message      { return Message{Role: "user", Content: content} }
func AssistantMessage(content string) Message { return Message{Role: "assistant", Content: content} }

func BuildPayload(messages []Message, schema model.JSONSchema, modelID string, maxTokens int) ([]byte, error) {
	payload := map[string]any{
		"model":      modelID,
		"max_tokens": maxTokens,
		"response_format": map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": schema.Name, "strict": true, "schema": schema.Schema},
		},
		"provider": map[string]any{
			"require_parameters": true, "preferred_max_latency": latencyRankingHintSeconds, "allow_fallbacks": false,
			"max_price": map[string]any{"prompt": catalog.MaxPromptUSDPerMillion, "completion": catalog.MaxCompletionUSDPerMillion},
		},
		"messages": messages,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("chat: marshal payload: %w", err)
	}
	return body, nil
}

// ParseErrorBody errors on a non-200 body that isn't an OpenRouter error
// object, describing body with replies.
func ParseErrorBody(body []byte, replies replyfile.Saver, generationID string) (*ProviderError, error) {
	var parsed struct {
		Error *ProviderError `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.Error == nil || parsed.Error.Message == "" {
		described, err := replies.Describe(generationID, "unexpected-error", body)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("chat: error response is not an OpenRouter error object — %s", described)
	}
	return parsed.Error, nil
}

// ParseResponse errors only if the body isn't JSON, which no model causes.
func ParseResponse(body []byte, replies replyfile.Saver, generationID string) (Reply, error) {
	var parsed struct {
		Error   *ProviderError `json:"error"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if unmarshalErr := json.Unmarshal(body, &parsed); unmarshalErr != nil {
		described, err := replies.Describe(generationID, "unexpected-ok", body)
		if err != nil {
			return Reply{}, err
		}
		return Reply{}, fmt.Errorf("chat: 200 response body is not JSON: %v — %s", unmarshalErr, described)
	}
	if parsed.Error != nil {
		return Reply{ProviderError: parsed.Error}, nil
	}
	if len(parsed.Choices) == 0 || parsed.Choices[0].Message.Content == "" {
		return Reply{Invalid: errors.New("response has no message content")}, nil
	}
	raw := parsed.Choices[0].Message.Content
	if !json.Valid([]byte(raw)) {
		return Reply{Invalid: errors.New("message content is not valid JSON")}, nil
	}
	return Reply{Content: json.RawMessage(raw)}, nil
}
