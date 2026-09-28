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
	// Appended to the library's fixed review instructions. The reviewer judges
	// content against these rules only.
	ReviewRulesPrompt string
	OutputSchema      JSONSchema
	// The lowest acceptable quality: high, medium, or low. A result rated
	// below it counts as a failure against its model and gets corrected.
	TargetQuality Quality
}
