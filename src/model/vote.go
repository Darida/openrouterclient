package model

// Vote is a caller's manual review of a generation it already received.
type Vote string

const (
	VoteUp Vote = "up"
	// Recorded against the generation's model as a manual rejection. It counts
	// toward excluding that model, the same as a failure.
	VoteDown Vote = "down"
)
