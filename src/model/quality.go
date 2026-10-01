package model

// Quality rates one generation. An automatic review gives high for a total
// bad score of 0, medium for 1 up to the request's ReviewCriteria.ToleranceThreshold,
// and low above it. A failure or output that doesn't match the schema is
// unusable.
type Quality string

const (
	QualityHigh     Quality = "high"
	QualityMedium   Quality = "medium"
	QualityLow      Quality = "low"
	QualityUnusable Quality = "unusable"
)
