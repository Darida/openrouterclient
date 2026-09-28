package main

import (
	"encoding/json"

	"github.com/Darida/openrouterclient/src/model"
)

type requirementsFile struct {
	Prompt            string `json:"prompt"`
	ReviewRulesPrompt string `json:"reviewRulesPrompt"`
	OutputSchema      struct {
		Name   string          `json:"name"`
		Schema json.RawMessage `json:"schema"`
	} `json:"outputSchema"`
	TargetQuality model.Quality `json:"targetQuality"`
}
