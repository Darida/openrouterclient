package chat

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Darida/openrouterclient/src/internal/replyfile"
	"github.com/Darida/openrouterclient/src/model"
)

func TestChatParseResponse_whenContentNotJSON_thenErrors(t *testing.T) {
	// Arrange
	body := []byte(`{"model":"m/free","choices":[{"message":{"content":"not json"}}]}`)

	// Act
	_, err := ParseResponse(body, replyfile.Disabled(), "gen-1")

	// Assert
	if err == nil {
		t.Fatal("expected an error for non-JSON content")
	}
}

func TestChatParseResponse_whenContentIsJSON_thenReturnsIt(t *testing.T) {
	// Arrange
	body := []byte(`{"model":"m/free","choices":[{"message":{"content":"{\"a\":1}"}}]}`)

	// Act
	got, err := ParseResponse(body, replyfile.Disabled(), "gen-1")

	// Assert
	if err != nil || string(got) != `{"a":1}` {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestChatBuildPayload_whenBuilt_thenRequestsTheGivenModel(t *testing.T) {
	// Arrange
	schema := model.JSONSchema{Name: "s", Schema: json.RawMessage(`{"type":"object"}`)}

	// Act
	payload := string(BuildPayload([]Message{UserMessage("hi")}, schema, "liquid/lfm-2.5-2.6b:free", 100))

	// Assert
	if !strings.Contains(payload, `"model":"liquid/lfm-2.5-2.6b:free"`) {
		t.Fatalf("payload does not request the given model: %s", payload)
	}
}

func TestChatBuildPayload_whenBuilt_thenDisallowsProviderFallbacks(t *testing.T) {
	// Arrange
	schema := model.JSONSchema{Name: "s", Schema: json.RawMessage(`{"type":"object"}`)}

	// Act
	payload := string(BuildPayload([]Message{UserMessage("hi")}, schema, "m:free", 100))

	// Assert
	if !strings.Contains(payload, `"allow_fallbacks":false`) {
		t.Fatalf("payload allows fallbacks: %s", payload)
	}
}

func TestChatParseResponse_whenBodyCarriesProviderError_thenReturnsProviderError(t *testing.T) {
	// Arrange
	body := []byte(`{"id":"gen-1","error":{"message":"Upstream error from Nvidia: Service temporarily overloaded","code":503,"metadata":{"error_type":"provider_overloaded"}}}`)

	// Act
	_, err := ParseResponse(body, replyfile.Disabled(), "gen-1")

	// Assert
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != 503 {
		t.Fatalf("err = %v; want a 503 ProviderError", err)
	}
}

func TestChatParseErrorBody_whenNotAnErrorObject_thenPanics(t *testing.T) {
	// Assert
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()

	// Act
	ParseErrorBody([]byte("<html>502 Bad Gateway</html>"), replyfile.Disabled(), "gen-1")
}

func TestProviderErrorError_whenMetadataHasRaw_thenUsesItsFirstSentence(t *testing.T) {
	// Arrange
	providerErr := ParseErrorBody([]byte(`{"error":{"message":"Provider returned error","code":429,"metadata":{"raw":"qwen/qwen3.8-27b:free is temporarily rate-limited upstream. Please retry shortly.","provider_name":"ModelRun"}}}`), replyfile.Disabled(), "gen-1")

	// Act
	got := providerErr.Error()

	// Assert
	if got != "provider error 429: qwen/qwen3.8-27b:free is temporarily rate-limited upstream" {
		t.Fatalf("got %q", got)
	}
}

func TestChatBuildPayload_whenBuilt_thenSendsMaxTokens(t *testing.T) {
	// Arrange
	schema := model.JSONSchema{Name: "s", Schema: json.RawMessage(`{"type":"object"}`)}

	// Act
	payload := string(BuildPayload([]Message{UserMessage("hi")}, schema, "m:free", 1234))

	// Assert
	if !strings.Contains(payload, `"max_tokens":1234`) {
		t.Fatalf("payload has no max_tokens: %s", payload)
	}
}

func TestChatBuildPayload_whenBuilt_thenSendsNoReasoningParameter(t *testing.T) {
	// Arrange
	schema := model.JSONSchema{Name: "s", Schema: json.RawMessage(`{"type":"object"}`)}

	// Act
	payload := string(BuildPayload([]Message{UserMessage("hi")}, schema, "m", 100))

	// Assert
	if strings.Contains(payload, `"reasoning"`) {
		t.Fatalf("payload sends reasoning, which require_parameters would demand of every endpoint: %s", payload)
	}
}
