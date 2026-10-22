package cost

import "github.com/Darida/openrouterclient/src/internal/catalog"

// EndpointPrice is ep's price for a request whose prompt holds promptTokens:
// its base price with every matching override applied in order.
func EndpointPrice(ep catalog.Endpoint, promptTokens int) catalog.Price {
	price := ep.Price
	for _, override := range ep.Overrides {
		if promptTokens < override.MinPromptTokens {
			continue
		}
		if override.Prompt != nil {
			price.Prompt = *override.Prompt
		}
		if override.Completion != nil {
			price.Completion = *override.Completion
		}
		if override.Request != nil {
			price.Request = *override.Request
		}
	}
	return price
}

// EndpointEstimate prices a request at price as if it used every one of
// maxOutputTokens.
func EndpointEstimate(price catalog.Price, promptTokens, maxOutputTokens int) float64 {
	return price.Prompt*float64(promptTokens) + price.Completion*float64(maxOutputTokens) + price.Request
}
