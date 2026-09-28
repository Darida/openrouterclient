package chat

import (
	"encoding/json"
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

func TestChatBuildPayload_whenModelsExcluded_thenAutoRouterDeniesThem(t *testing.T) {
	// Arrange
	schema := model.JSONSchema{Name: "s", Schema: json.RawMessage(`{"type":"object"}`)}

	// Act
	payload := string(BuildPayload([]Message{UserMessage("hi")}, schema, []string{"bad/model"}))

	// Assert
	if !strings.Contains(payload, `"allowed_models":["*","!bad/model"]`) {
		t.Fatalf("payload has no exclusion: %s", payload)
	}
}

func TestChatBuildPayload_whenBuilt_thenDisallowsProviderFallbacks(t *testing.T) {
	// Arrange
	schema := model.JSONSchema{Name: "s", Schema: json.RawMessage(`{"type":"object"}`)}

	// Act
	payload := string(BuildPayload([]Message{UserMessage("hi")}, schema, nil))

	// Assert
	if !strings.Contains(payload, `"allow_fallbacks":false`) {
		t.Fatalf("payload allows fallbacks: %s", payload)
	}
}
