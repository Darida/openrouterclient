package replyfile

// Saver's zero value is invalid; build one with Local or Disabled.
type Saver struct {
	mode mode
}

type mode int

const (
	modeUnset mode = iota
	modeLocal
	modeDisabled
)
