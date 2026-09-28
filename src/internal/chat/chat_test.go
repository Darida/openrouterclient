package chat

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Darida/openrouterclient/src/model"
)

func TestChatParseResponse_whenModelMissing_thenPanics(t *testing.T) {
	// Arrange
	body := []byte(`{"choices":[{"message":{"content":"{}"}}]}`)

	// Assert
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()

	// Act
	ParseResponse(body)
}

func TestChatParseResponse_whenContentNotJSON_thenErrorsWithModel(t *testing.T) {
	// Arrange
	body := []byte(`{"model":"m/free","choices":[{"message":{"content":"not json"}}]}`)

	// Act
	got, err := ParseResponse(body)

	// Assert
	if err == nil || got.Model != "m/free" {
		t.Fatalf("got %+v, %v; want an error attributed to m/free", got, err)
	}
}

func TestChatParseResponse_whenContentIsJSON_thenReturnsIt(t *testing.T) {
	// Arrange
	body := []byte(`{"model":"m/free","choices":[{"message":{"content":"{\"a\":1}"}}]}`)

	// Act
	got, err := ParseResponse(body)

	// Assert
	if err != nil || string(got.Content) != `{"a":1}` {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestChatBuildPayload_whenBuilt_thenRequestsTheGivenModel(t *testing.T) {
	// Arrange
	schema := model.JSONSchema{Name: "s", Schema: json.RawMessage(`{"type":"object"}`)}

	// Act
	payload := string(BuildPayload([]Message{UserMessage("hi")}, schema, "liquid/lfm-2.5-2.6b:free"))

	// Assert
	if !strings.Contains(payload, `"model":"liquid/lfm-2.5-2.6b:free"`) {
		t.Fatalf("payload does not request the given model: %s", payload)
	}
}

func TestChatBuildPayload_whenBuilt_thenDisallowsProviderFallbacks(t *testing.T) {
	// Arrange
	schema := model.JSONSchema{Name: "s", Schema: json.RawMessage(`{"type":"object"}`)}

	// Act
	payload := string(BuildPayload([]Message{UserMessage("hi")}, schema, "m:free"))

	// Assert
	if !strings.Contains(payload, `"allow_fallbacks":false`) {
		t.Fatalf("payload allows fallbacks: %s", payload)
	}
}

func TestChatParseResponse_whenBodyCarriesProviderError_thenReturnsProviderError(t *testing.T) {
	// Arrange
	body := []byte(`{"id":"gen-1","error":{"message":"Upstream error from Nvidia: Service temporarily overloaded","code":503,"metadata":{"error_type":"provider_overloaded"}}}`)

	// Act
	_, err := ParseResponse(body)

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
	ParseErrorBody([]byte("<html>502 Bad Gateway</html>"))
}

func TestProviderErrorRateLimitedModel_whenRawNamesModel_thenReturnsIt(t *testing.T) {
	// Arrange
	providerErr := ParseErrorBody([]byte(`{"error":{"message":"Provider returned error","code":429,"metadata":{"raw":"qwen/qwen3.8-27b:free is temporarily rate-limited upstream. Please retry shortly.","provider_name":"ModelRun"}}}`))

	// Act
	got := providerErr.RateLimitedModel()

	// Assert
	if got != "qwen/qwen3.8-27b:free" {
		t.Fatalf("got %q", got)
	}
}

func TestProviderErrorRateLimitedModel_whenRawWordingUnknown_thenPanics(t *testing.T) {
	// Arrange
	providerErr := ParseErrorBody([]byte(`{"error":{"message":"Provider returned error","code":429,"metadata":{"raw":"Too many requests"}}}`))

	// Assert
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()

	// Act
	providerErr.RateLimitedModel()
}
