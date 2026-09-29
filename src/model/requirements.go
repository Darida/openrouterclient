package model

import (
	"encoding/json"
	"time"
)

// DefaultMaxOutputTokens applies when MaxOutputTokens is 0.
const DefaultMaxOutputTokens = 10_000

// JSONSchema is a JSON Schema document plus the name OpenRouter's strict
// json_schema response_format registers it under.
type JSONSchema struct {
	Name   string
	Schema json.RawMessage
}

type TextGenerationRequirements struct {
	Prompt string
	// The shape Prompt's output must match. Output that doesn't is a failed
	// attempt and never reaches validation.
	OutputSchema JSONSchema
	// Rules an automatic review checks the output against, appended to the
	// library's fixed review instructions. Each rule may state its bad score;
	// one that doesn't counts 1. Empty skips the review.
	OutputValidationRules string
	// The highest total bad score a review may give and still rate medium;
	// above it rates low, and 0 rates high. Required, at least 1, when
	// OutputValidationRules is set; must be 0 when it is empty.
	ReviewToleranceThreshold int
	// How many corrections may follow a review that rates the output below
	// TargetQuality, each one regenerated with the review's notes and reviewed
	// again. Required when OutputValidationRules is set: 0 reviews the first
	// output without correcting it; negative panics. Must be 0 when
	// OutputValidationRules is empty.
	MaxCorrections int
	// The lowest acceptable quality: high, medium, or low. A reviewed result
	// rated below it counts as a failure against its model and gets corrected.
	TargetQuality Quality
	// Groups requests of one kind in history. When picking models, a past
	// failure under the same tag counts in full; under another tag, half.
	Tag       string
	ModelTier ModelTier
	// Exact model IDs never asked, for generation or review. An ID that isn't
	// a structured-output ModelTier model in OpenRouter's catalog panics.
	ExcludedModels []string
	// How long an attempt may stay pending before the next one starts, and
	// how long a success may take before it counts against its model. Each
	// attempt is cut off shortly after. Applies to generation and review
	// calls alike. 0 means api.DefaultTimeout.
	Timeout time.Duration
	// Sent as the response token limit, and priced as the output when ranking
	// paid models by cost. 0 means DefaultMaxOutputTokens.
	MaxOutputTokens int
}
