package model

import "encoding/json"

// JSONSchema is a JSON Schema document plus the name OpenRouter's strict
// json_schema response_format registers it under.
type JSONSchema struct {
	Name   string
	Schema json.RawMessage
}

type TextGenerationRequirements struct {
	Prompt string
	// The shape Prompt's output must match. Output that doesn't is a failed
	// attempt and never reaches validation.
	OutputSchema JSONSchema
	// Rules an automatic review checks the output against, appended to the
	// library's fixed review instructions. Empty skips the review.
	OutputValidationRules string
	// The lowest acceptable quality: high, medium, or low. A reviewed result
	// rated below it counts as a failure against its model and gets corrected.
	TargetQuality Quality
	// Groups requests of one kind in history. When picking models, a past
	// failure under the same tag counts in full; under another tag, half.
	Tag string
}
