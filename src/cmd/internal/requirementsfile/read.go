package requirementsfile

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Darida/openrouterclient/src/model"
)

// Read decodes the requirements file at path, rejecting unknown fields.
func Read(path, tag string, tier model.ModelTier) (model.TextGenerationRequirements, error) {
	file, err := os.Open(path)
	if err != nil {
		return model.TextGenerationRequirements{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var input requirementsFile
	if err := decoder.Decode(&input); err != nil {
		return model.TextGenerationRequirements{}, fmt.Errorf("%s: %w", path, err)
	}
	var maxCorrections int
	switch {
	case input.OutputValidationRules != "" && input.MaxCorrections == nil:
		return model.TextGenerationRequirements{}, fmt.Errorf("%s: maxCorrections is required when outputValidationRules is set", path)
	case input.OutputValidationRules == "" && input.MaxCorrections != nil:
		return model.TextGenerationRequirements{}, fmt.Errorf("%s: maxCorrections must be absent when outputValidationRules is empty", path)
	case input.MaxCorrections != nil:
		maxCorrections = *input.MaxCorrections
	}
	return model.TextGenerationRequirements{
		Prompt:                   input.Prompt,
		OutputSchema:             model.JSONSchema{Name: input.OutputSchema.Name, Schema: input.OutputSchema.Schema},
		OutputValidationRules:    input.OutputValidationRules,
		ReviewToleranceThreshold: input.ReviewToleranceThreshold,
		MaxCorrections:           maxCorrections,
		TargetQuality:            input.TargetQuality,
		Tag:                      tag,
		ModelTier:                tier,
		Timeout:                  time.Duration(input.TimeoutSeconds) * time.Second,
		MaxOutputTokens:          input.MaxOutputTokens,
		ExcludedModels:           input.ExcludedModels,
	}, nil
}

// Tier maps the --paid flag to a model tier.
func Tier(paid bool) model.ModelTier {
	if paid {
		return model.ModelTierPaid
	}
	return model.ModelTierFree
}
