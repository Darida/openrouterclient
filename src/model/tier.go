package model

// ModelTier chooses which OpenRouter models a request may use.
type ModelTier string

const (
	// Free models only, picked at random.
	ModelTierFree ModelTier = "free"
	// Paid models only, picked at random among the cheapest for this request.
	ModelTierPaid ModelTier = "paid"
)
