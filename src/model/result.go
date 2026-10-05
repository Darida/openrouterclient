package model

import "encoding/json"

type GeneratedText struct {
	// Raw JSON that OutputSchema was requested for. Providers comply with the
	// schema on a best-effort basis, so only the caller, who knows the shape,
	// can validate it.
	Content json.RawMessage
	// The model that actually produced Content. Exclusion routing means it can
	// differ from call to call.
	Model string
	// OpenRouter's id for this generation, and the handle Client.Rate takes.
	GenerationID string
}

type ReviewedText struct {
	GeneratedText
	// The automatic review of Content, whose Quality always meets the
	// request's TargetQuality.
	Review Review
}

// Estimate is the model a Generate attempt could ask and what it would cost.
type Estimate struct {
	Model string
	// InputTokens × PromptUSDPerToken + OutputTokens × CompletionUSDPerToken,
	// as if the reply used every output token. 0 for a free model.
	CostUSD               float64
	PromptUSDPerToken     float64
	CompletionUSDPerToken float64
}
