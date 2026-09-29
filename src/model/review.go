package model

// ReviewVerdict is the fixed output schema of every automatic review, whatever the caller's rules.
type ReviewVerdict struct {
	// One entry per offending instance; a rule broken in several places
	// appears once per place.
	Violations []ReviewViolation `json:"violations"`
	// The reviewer's sum of BadScore over all violations. A verdict whose
	// total differs from that sum is rejected as off-schema.
	TotalBadScore int `json:"totalBadScore"`
}

type ReviewViolation struct {
	// The rule violated, quoted or named as it appears in OutputValidationRules.
	Rule string `json:"rule"`
	// A short excerpt, about five words, that demonstrates the violation: from
	// the output, or from the task for something the output leaves out.
	Evidence string `json:"evidence"`
	// Why the evidence breaks the rule.
	Explanation string `json:"explanation"`
	// The specific change that would fix this violation.
	RecommendedAction string `json:"recommendedAction"`
	// The bad score Rule states in OutputValidationRules, or 1 if it states none.
	BadScore int `json:"badScore"`
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
