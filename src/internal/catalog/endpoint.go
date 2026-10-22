package catalog

import "encoding/json"

// endpointEntry is the part of one /api/v1/models/{id}/endpoints item this
// package reads.
type endpointEntry struct {
	Tag                 string   `json:"tag"`
	ProviderName        string   `json:"provider_name"`
	ContextLength       int      `json:"context_length"`
	MaxPromptTokens     *int     `json:"max_prompt_tokens"`
	MaxCompletionTokens *int     `json:"max_completion_tokens"`
	SupportedParameters []string `json:"supported_parameters"`
	// USD per token (request: per request), as decimal strings.
	Pricing struct {
		Prompt     string  `json:"prompt"`
		Completion string  `json:"completion"`
		Request    *string `json:"request"`
		// Each item holds its conditions plus the prices it replaces.
		Overrides []map[string]json.RawMessage `json:"overrides"`
	} `json:"pricing"`
}

// Endpoint is one provider's offering of a model.
type Endpoint struct {
	// The routing slug provider.order and provider.only accept.
	Tag          string
	ProviderName string
	// ContextTokens bounds prompt plus output tokens in one request.
	ContextTokens int
	// nil when the provider sets no limit beyond ContextTokens.
	MaxPromptTokens     *int
	MaxCompletionTokens *int
	SupportedParameters []string
	Price               Price
	// Applied in order on top of Price; a later match wins.
	Overrides []PriceOverride
}

// Price is in USD per token, except Request, which is per request.
type Price struct {
	Prompt     float64
	Completion float64
	Request    float64
}

// PriceOverride replaces some of an endpoint's prices for requests whose
// prompt holds at least MinPromptTokens tokens.
type PriceOverride struct {
	MinPromptTokens int
	// nil keeps the price it would otherwise have.
	Prompt     *float64
	Completion *float64
	Request    *float64
}
