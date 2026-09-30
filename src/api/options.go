package api

import (
	"github.com/Darida/openrouterclient/src/internal/history"
	"github.com/Darida/openrouterclient/src/internal/replyfile"
)

// History says where per-generation outcomes are kept. Build one with
// LocalHistory or DisabledHistory; the zero value is invalid.
type History struct {
	store history.History
	// Lets NewRater refuse up front instead of on the first Rate.
	disabled bool
}

// Replies says where raw OpenRouter replies go, so logs, errors, and panics
// can point to one without quoting it. Build one with LocalReplies or
// DisabledReplies; the zero value is invalid.
type Replies struct {
	saver replyfile.Saver
}
