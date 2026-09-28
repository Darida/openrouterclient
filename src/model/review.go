package model

// ReviewVerdict is the fixed output schema of every automatic review, whatever the caller's rules.
type ReviewVerdict struct {
	Status ReviewStatus `json:"status"`
	// One entry per violated rule. It is empty when Status is ReviewApproved.
	Notes []ReviewNote `json:"notes"`
}

type ReviewStatus string

const (
	ReviewApproved ReviewStatus = "APPROVED"
	ReviewRejected ReviewStatus = "REJECTED"
)

type ReviewNote struct {
	// The rule violated, quoted or named as it appears in ReviewRulesPrompt.
	Rule string `json:"rule"`
	// An actionable description of the violation.
	Text string `json:"text"`
}

// Review is one completed automatic review. The reviewer is a separate
// OpenRouter generation, so it may be a different model than the one it
// judged.
type Review struct {
	Verdict      ReviewVerdict
	Model        string
	GenerationID string
}
