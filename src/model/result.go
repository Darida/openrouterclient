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
	// The automatic review of Content, whose Quality always meets
	// TargetQuality. Nil when OutputValidationRules is empty.
	Review *Review
}
