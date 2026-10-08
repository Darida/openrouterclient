package model

// ModelSelection chooses which OpenRouter models a call may ask. At most one
// of Tier, Allowed, and Denied may be set; none, like an empty Denied, means
// every model. MinIntelligenceIndex combines with any of them.
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
	// Only models whose Artificial Analysis intelligence index, as listed in
	// OpenRouter's catalog, is at least this; a model listed without one is
	// never asked. 0 means no minimum; it must be within 0–100.
	MinIntelligenceIndex float64
	// Which percentile of the candidates' estimated costs sets the price
	// ceiling: only candidates estimated at most 10% above it are asked. So
	// when at least this percent of the candidates are free, only free ones
	// are asked. 0 means the 10th percentile; it must be within 0–100.
	CostPercentile float64
}
