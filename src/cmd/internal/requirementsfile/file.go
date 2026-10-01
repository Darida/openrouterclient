package requirementsfile

import (
	"encoding/json"

	"github.com/Darida/openrouterclient/src/model"
)

type requirementsFile struct {
	Prompt       string `json:"prompt"`
	OutputSchema struct {
		Name   string          `json:"name"`
		Schema json.RawMessage `json:"schema"`
	} `json:"outputSchema"`
	TargetQuality model.Quality `json:"targetQuality"`
	// Optional; 0 or absent means model.DefaultMaxOutputTokens.
	MaxOutputTokens int `json:"maxOutputTokens"`
}
