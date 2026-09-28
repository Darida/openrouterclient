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
	OutputValidationRules string        `json:"outputValidationRules"`
	TargetQuality         model.Quality `json:"targetQuality"`
}
