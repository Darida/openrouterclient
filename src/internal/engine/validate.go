package engine

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Darida/openrouterclient/src/internal/quality"
	"github.com/Darida/openrouterclient/src/model"
)

func validateGenerateReviewed(req model.GenerateReviewedRequest) error {
	if err := validatePrompt(req.Prompt, req.OutputSchema); err != nil {
		return err
	}
	if err := validateSelection("GenerationModels", req.GenerationModels); err != nil {
		return err
	}
	if err := validateSelection("ReviewModels", req.ReviewModels); err != nil {
		return err
	}
	if err := validateCriteria(req.Criteria); err != nil {
		return err
	}
	if err := validateTarget(req.TargetQuality); err != nil {
		return err
	}
	if req.MaxCorrections < 0 {
		return fmt.Errorf("engine: MaxCorrections must not be negative, got %d", req.MaxCorrections)
	}
	return validateMaxOutputTokens(req.MaxOutputTokens)
}

func validateGenerate(req model.GenerateRequest) error {
	if err := validatePrompt(req.Prompt, req.OutputSchema); err != nil {
		return err
	}
	if err := validateSelection("Models", req.Models); err != nil {
		return err
	}
	if err := validateTarget(req.TargetQuality); err != nil {
		return err
	}
	return validateMaxOutputTokens(req.MaxOutputTokens)
}

func validateReview(req model.ReviewRequest) error {
	if req.Task == "" {
		return errors.New("engine: Task is required")
	}
	if !json.Valid(req.Content) {
		return errors.New("engine: Content must be valid JSON")
	}
	if err := validateCriteria(req.Criteria); err != nil {
		return err
	}
	if err := validateSelection("Models", req.Models); err != nil {
		return err
	}
	return validateMaxOutputTokens(req.MaxOutputTokens)
}

// validateSelection checks the shape only; whether named models exist is
// known once the catalog is fetched.
func validateSelection(field string, sel model.ModelSelection) error {
	set := 0
	for _, isSet := range []bool{sel.Tier != "", len(sel.Allowed) > 0, len(sel.Denied) > 0} {
		if isSet {
			set++
		}
	}
	if set > 1 {
		return fmt.Errorf("engine: %s must set at most one of Tier, Allowed, and Denied, got %+v", field, sel)
	}
	if !(sel.MinIntelligenceIndex >= 0 && sel.MinIntelligenceIndex <= 100) {
		return fmt.Errorf("engine: %s.MinIntelligenceIndex must be within 0–100, got %v", field, sel.MinIntelligenceIndex)
	}
	if sel.Tier != "" && sel.Tier != model.ModelTierFree && sel.Tier != model.ModelTierPaid {
		return fmt.Errorf("engine: %s.Tier must be free or paid, got %q", field, sel.Tier)
	}
	return nil
}

func validatePrompt(prompt string, outputSchema model.JSONSchema) error {
	if prompt == "" || outputSchema.Name == "" || len(outputSchema.Schema) == 0 {
		return errors.New("engine: Prompt, OutputSchema.Name, and OutputSchema.Schema are all required")
	}
	return nil
}

func validateCriteria(criteria model.ReviewCriteria) error {
	if criteria.Rules == "" {
		return errors.New("engine: Criteria.Rules is required")
	}
	if criteria.ToleranceThreshold < 1 {
		return fmt.Errorf("engine: Criteria.ToleranceThreshold must be at least 1, got %d", criteria.ToleranceThreshold)
	}
	return nil
}

func validateTarget(target model.Quality) error {
	if !quality.IsRating(target) {
		return fmt.Errorf("engine: TargetQuality must be high, medium, or low, got %q", target)
	}
	return nil
}

func validateMaxOutputTokens(maxTokens int) error {
	if maxTokens < 0 {
		return fmt.Errorf("engine: MaxOutputTokens must not be negative, got %d", maxTokens)
	}
	return nil
}

func validateEstimate(req model.EstimateRequest) error {
	if err := validateSelection("Models", req.Models); err != nil {
		return err
	}
	if req.InputTokens <= 0 || req.OutputTokens <= 0 {
		return fmt.Errorf("engine: InputTokens and OutputTokens must be positive, got %d and %d", req.InputTokens, req.OutputTokens)
	}
	return nil
}
