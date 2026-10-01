package model

import "encoding/json"

// DefaultMaxOutputTokens applies when MaxOutputTokens is 0.
const DefaultMaxOutputTokens = 10_000

// JSONSchema is a JSON Schema document plus the name OpenRouter's strict
// json_schema response_format registers it under.
type JSONSchema struct {
	Name   string
	Schema json.RawMessage
}

// GenerateRequest asks for one generation with no review.
type GenerateRequest struct {
	Prompt string
	// The shape Prompt's output must match. Output that doesn't is a failed
	// attempt.
	OutputSchema JSONSchema
	Models       ModelSelection
	// The quality the generation is held to in history: with no review it is
	// recorded as high, so only a later manual rating below this target
	// counts against its model. High, medium, or low.
	TargetQuality Quality
	// Sent as the response token limit, and priced as the output when ranking
	// paid models by cost. 0 means DefaultMaxOutputTokens.
	MaxOutputTokens int
}

// ReviewRequest asks for one review of content the caller supplies.
type ReviewRequest struct {
	// What the content was meant to do, shown to the reviewer as the task
	// someone else was given.
	Task string
	// The work under review. It must be valid JSON.
	Content  json.RawMessage
	Criteria ReviewCriteria
	Models   ModelSelection
	// Sent as the reviewer's response token limit. 0 means
	// DefaultMaxOutputTokens.
	MaxOutputTokens int
}

// GenerateReviewedRequest asks for a generation, reviews it, and corrects it
// until it meets TargetQuality.
type GenerateReviewedRequest struct {
	Prompt string
	// The shape Prompt's output must match. Output that doesn't is a failed
	// attempt and never reaches review.
	OutputSchema     JSONSchema
	GenerationModels ModelSelection
	ReviewModels     ModelSelection
	Criteria         ReviewCriteria
	// The lowest acceptable quality: high, medium, or low. A reviewed result
	// rated below it counts as a failure against its model and gets corrected.
	TargetQuality Quality
	// How many corrections may follow a review that rates the output below
	// TargetQuality, each one regenerated with the review's violations and
	// reviewed again. 0 reviews the first output without correcting it;
	// negative is an error.
	MaxCorrections int
	// Sent as the response token limit of generation and review calls alike,
	// and priced as the output when ranking paid models by cost. 0 means
	// DefaultMaxOutputTokens.
	MaxOutputTokens int
}

// ReviewCriteria is what an automatic review checks output against.
type ReviewCriteria struct {
	// Appended to the library's fixed review instructions. Each rule may state
	// its bad score; one that doesn't counts 1. Required.
	Rules string
	// The highest total bad score a review may give and still rate medium;
	// above it rates low, and 0 rates high. At least 1.
	ToleranceThreshold int
}
