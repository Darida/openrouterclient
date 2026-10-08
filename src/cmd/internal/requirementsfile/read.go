package requirementsfile

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Darida/openrouterclient/src/model"
)

// Read decodes the requirements file at path, rejecting unknown fields.
func Read(path string) (model.GenerateRequest, error) {
	file, err := os.Open(path)
	if err != nil {
		return model.GenerateRequest{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var input requirementsFile
	if err := decoder.Decode(&input); err != nil {
		return model.GenerateRequest{}, fmt.Errorf("%s: %w", path, err)
	}
	return model.GenerateRequest{
		SystemPrompt:    input.SystemPrompt,
		Prompt:          input.Prompt,
		OutputSchema:    model.JSONSchema{Name: input.OutputSchema.Name, Schema: input.OutputSchema.Schema},
		Models:          model.ModelSelection{Denied: input.ExcludedModels, MinIntelligenceIndex: input.MinIntelligenceIndex, CostPercentile: input.CostPercentile},
		TargetQuality:   input.TargetQuality,
		MaxOutputTokens: input.MaxOutputTokens,
	}, nil
}

// Timeout is the commands' fixed Config.Timeout.
const Timeout = 60 * time.Second
