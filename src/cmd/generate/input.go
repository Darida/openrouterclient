package main

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
	OutputValidationRules string `json:"outputValidationRules"`
	// Required when outputValidationRules is set; absent otherwise.
	ReviewToleranceThreshold int `json:"reviewToleranceThreshold"`
	// Required when outputValidationRules is set, 0 or more; absent otherwise.
	MaxCorrections *int          `json:"maxCorrections"`
	TargetQuality  model.Quality `json:"targetQuality"`
	// Optional; 0 or absent means api.DefaultTimeout.
	TimeoutSeconds int `json:"timeoutSeconds"`
	// Optional; 0 or absent means model.DefaultMaxOutputTokens.
	MaxOutputTokens int `json:"maxOutputTokens"`
}
