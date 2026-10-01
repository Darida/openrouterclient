package chat

import "encoding/json"

// Reply is a 200 body sorted by whose fault it is: exactly one field is set.
type Reply struct {
	Content json.RawMessage
	// An upstream failure OpenRouter relayed after already sending a 200.
	ProviderError *ProviderError
	// The model's message is missing or isn't JSON.
	Invalid error
}
