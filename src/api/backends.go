package api

import (
	"errors"

	"github.com/Darida/openrouterclient/src/internal/history"
	"github.com/Darida/openrouterclient/src/internal/replyfile"
)

// LocalHistory keeps outcomes in a JSON file at path, created if absent, and
// uses them to exclude models that fail too often. The file must persist
// across runs, or exclusion never learns anything. It errors if path is
// empty, or if an existing file there is unreadable or malformed.
func LocalHistory(path string) (History, error) {
	if path == "" {
		return History{}, errors.New("openrouterclient: LocalHistory needs a path")
	}
	store, err := history.Open(path)
	if err != nil {
		return History{}, err
	}
	return History{store: store}, nil
}

// DisabledHistory records nothing and excludes no model, so every call picks
// from all candidates regardless of past failures, and rating is impossible.
func DisabledHistory() History {
	return History{store: history.Disabled{}, disabled: true}
}

// LocalReplies saves each raw reply to its own file under
// openrouterclient/ in the system temp directory, and messages name that file.
func LocalReplies() Replies {
	return Replies{saver: replyfile.Local()}
}

// DisabledReplies saves nothing. Messages name the reply's OpenRouter
// generation id instead, when it has one, so the reply can be looked up on
// OpenRouter; otherwise they say only that it was not saved.
func DisabledReplies() Replies {
	return Replies{saver: replyfile.Disabled()}
}
