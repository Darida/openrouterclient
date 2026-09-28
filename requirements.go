package openrouterclient

import "encoding/json"

// JSONSchema is a JSON Schema document plus the name OpenRouter's strict
// json_schema response_format registers it under.
type JSONSchema struct {
	Name   string
	Schema json.RawMessage
}

type TextGenerationRequirements struct {
	Prompt string
	// Appended to the library's fixed review instructions for the automatic
	// follow-up review. The reviewer judges the generated content against
	// these rules only.
	ReviewRulesPrompt string
	OutputSchema      JSONSchema
}
