package model

// Quality rates one generation. An automatic review gives high for 0 notes,
// medium for 1–3, and low for 4 or more. A failure or output that doesn't
// match the schema is unusable.
type Quality string

const (
	QualityHigh     Quality = "high"
	QualityMedium   Quality = "medium"
	QualityLow      Quality = "low"
	QualityUnusable Quality = "unusable"
)
