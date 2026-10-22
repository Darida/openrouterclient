package catalog

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"
)

// Override keys that make a price depend on the time of day, which pricing a
// request ahead of time can't honor.
var timeWindowOverrideKeys = []string{"utc_start", "utc_end", "utc_days"}

// endpointsCache holds each model's endpoints, fetched at most once per
// cacheTTL; it is keyed by model ID.
type endpointsCache struct {
	mu      sync.Mutex
	entries map[string]cachedEndpoints
}

type cachedEndpoints struct {
	fetchedAt time.Time
	endpoints []Endpoint
}

// Endpoints lists every provider endpoint OpenRouter offers for modelID, which
// may be none. It errors on an endpoint without a tag or context length, with
// an invalid price, or with a price override it can't apply.
func (c *Catalog) Endpoints(modelID string) ([]Endpoint, error) {
	c.endpoints.mu.Lock()
	defer c.endpoints.mu.Unlock()
	if cached, ok := c.endpoints.entries[modelID]; ok && time.Since(cached.fetchedAt) < cacheTTL {
		return cached.endpoints, nil
	}
	body, err := c.fetch(c.URL + "/" + modelID + "/endpoints")
	if err != nil {
		return nil, err
	}
	endpoints, err := c.parseEndpoints(modelID, body)
	if err != nil {
		return nil, err
	}
	if c.endpoints.entries == nil {
		c.endpoints.entries = map[string]cachedEndpoints{}
	}
	c.endpoints.entries[modelID] = cachedEndpoints{fetchedAt: time.Now(), endpoints: endpoints}
	return endpoints, nil
}

func (c *Catalog) parseEndpoints(modelID string, body []byte) ([]Endpoint, error) {
	var parsed struct {
		Data *struct {
			Endpoints []endpointEntry `json:"endpoints"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.Data == nil {
		return nil, c.describedError(body, fmt.Sprintf("endpoints of %q are not an endpoint list: %v", modelID, err))
	}
	endpoints := make([]Endpoint, 0, len(parsed.Data.Endpoints))
	for _, raw := range parsed.Data.Endpoints {
		endpoint, problem := endpointFrom(raw)
		if problem != "" {
			return nil, c.describedError(body, fmt.Sprintf("endpoint %q of %q %s", raw.Tag, modelID, problem))
		}
		endpoints = append(endpoints, endpoint)
	}
	return endpoints, nil
}

// endpointFrom returns a non-empty problem describing why raw is unusable.
func endpointFrom(raw endpointEntry) (Endpoint, string) {
	if raw.Tag == "" {
		return Endpoint{}, "has no tag"
	}
	if raw.ContextLength <= 0 {
		return Endpoint{}, "has no context length"
	}
	price, problem := basePrice(raw)
	if problem != "" {
		return Endpoint{}, problem
	}
	overrides := make([]PriceOverride, 0, len(raw.Pricing.Overrides))
	for _, rawOverride := range raw.Pricing.Overrides {
		override, problem := priceOverride(rawOverride)
		if problem != "" {
			return Endpoint{}, problem
		}
		overrides = append(overrides, override)
	}
	return Endpoint{
		Tag:                 raw.Tag,
		ProviderName:        raw.ProviderName,
		ContextTokens:       raw.ContextLength,
		MaxPromptTokens:     raw.MaxPromptTokens,
		MaxCompletionTokens: raw.MaxCompletionTokens,
		SupportedParameters: raw.SupportedParameters,
		Price:               price,
		Overrides:           overrides,
	}, ""
}

func basePrice(raw endpointEntry) (Price, string) {
	prompt, ok := parsePrice(raw.Pricing.Prompt)
	if !ok {
		return Price{}, fmt.Sprintf("has an invalid prompt price %q", raw.Pricing.Prompt)
	}
	completion, ok := parsePrice(raw.Pricing.Completion)
	if !ok {
		return Price{}, fmt.Sprintf("has an invalid completion price %q", raw.Pricing.Completion)
	}
	price := Price{Prompt: prompt, Completion: completion}
	if raw.Pricing.Request != nil {
		if price.Request, ok = parsePrice(*raw.Pricing.Request); !ok {
			return Price{}, fmt.Sprintf("has an invalid request price %q", *raw.Pricing.Request)
		}
	}
	return price, ""
}

// priceOverride reads min_prompt_tokens as the only condition; every other key
// is a price, of which only prompt, completion, and request are kept.
func priceOverride(raw map[string]json.RawMessage) (PriceOverride, string) {
	for _, key := range timeWindowOverrideKeys {
		if _, ok := raw[key]; ok {
			return PriceOverride{}, fmt.Sprintf("has a price override limited by %s, which can't be priced ahead of time", key)
		}
	}
	minRaw, ok := raw["min_prompt_tokens"]
	if !ok {
		return PriceOverride{}, "has a price override without min_prompt_tokens"
	}
	var override PriceOverride
	if err := json.Unmarshal(minRaw, &override.MinPromptTokens); err != nil || override.MinPromptTokens < 0 {
		return PriceOverride{}, fmt.Sprintf("has a price override with invalid min_prompt_tokens %s", minRaw)
	}
	for key, value := range raw {
		if key == "min_prompt_tokens" {
			continue
		}
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			return PriceOverride{}, fmt.Sprintf("has a price override with unknown non-price key %q", key)
		}
		usd, ok := parsePrice(text)
		if !ok {
			return PriceOverride{}, fmt.Sprintf("has a price override with an invalid %s price %q", key, text)
		}
		switch key {
		case "prompt":
			override.Prompt = &usd
		case "completion":
			override.Completion = &usd
		case "request":
			override.Request = &usd
		}
	}
	return override, ""
}

func parsePrice(raw string) (float64, bool) {
	usd, err := strconv.ParseFloat(raw, 64)
	return usd, err == nil && usd >= 0
}
