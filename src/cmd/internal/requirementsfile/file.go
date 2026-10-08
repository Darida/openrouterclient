package requirementsfile

import (
	"encoding/json"

	"github.com/Darida/openrouterclient/src/model"
)

type requirementsFile struct {
	// Optional; absent sends no system message.
	SystemPrompt string `json:"systemPrompt"`
	Prompt       string `json:"prompt"`
	OutputSchema struct {
		Name   string          `json:"name"`
		Schema json.RawMessage `json:"schema"`
	} `json:"outputSchema"`
	TargetQuality model.Quality `json:"targetQuality"`
	// Optional; 0 or absent means model.DefaultMaxOutputTokens.
	MaxOutputTokens int `json:"maxOutputTokens"`
	// Optional; exact model IDs never asked. Absent means none.
	ExcludedModels []string `json:"excludedModels"`
	// Optional; 0 or absent means no minimum.
	MinIntelligenceIndex float64 `json:"minIntelligenceIndex"`
}
