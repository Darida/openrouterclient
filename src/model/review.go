package model

// ReviewVerdict is the fixed output schema of every automatic review, whatever the caller's rules.
type ReviewVerdict struct {
	// One entry per violated rule.
	Notes []ReviewNote `json:"notes"`
	// The reviewer's sum of the violated rules' bad scores. A rule in
	// OutputValidationRules that states no bad score counts 1.
	TotalBadScore int `json:"totalBadScore"`
}

type ReviewNote struct {
	// The rule violated, quoted or named as it appears in OutputValidationRules.
	Rule string `json:"rule"`
	// An actionable description of the violation.
	Text string `json:"text"`
}

// Review is one completed automatic review. The reviewer is a separate
// OpenRouter generation, so it may be a different model than the one it
// judged.
type Review struct {
	Verdict ReviewVerdict
	// The quality this review assigned to the generation it judged.
	Quality      Quality
	Model string
	// The review's own generation. Rating it low clears Quality from history,
	// so the judged generation no longer counts for or against its model.
	GenerationID string
}
