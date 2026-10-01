package model

// ModelSelection chooses which OpenRouter models a call may ask. Exactly one
// field must be set. Allowed and Denied draw from every structured-output
// model in OpenRouter's catalog, free and paid alike, and name exact model
// IDs; an ID that isn't such a model is an error.
type ModelSelection struct {
	Tier ModelTier
	// Only these models.
	Allowed []string
	// Every model except these.
	Denied []string
}
