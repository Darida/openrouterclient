package model

// ModelSelection chooses which OpenRouter models a call may ask. At most one
// field may be set; the zero value, like an empty Denied, means every model.
// Allowed and Denied draw from every structured-output model in OpenRouter's
// catalog, free and paid alike, and name exact model IDs; an ID that isn't
// such a model is an error. Whatever the selection, only the candidates
// priced near the cheapest for the request are asked; free models always are.
type ModelSelection struct {
	Tier ModelTier
	// Only these models.
	Allowed []string
	// Every model except these.
	Denied []string
}
