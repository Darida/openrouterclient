package hedge

// Verdict is how one attempt settled.
type Verdict int

const (
	Lost Verdict = iota
	Won
	// Aborted stops the race: no further attempt launches, and those in
	// flight are canceled.
	Aborted
)
