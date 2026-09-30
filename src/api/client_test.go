package api

import (
	"io"
	"log/slog"
	"testing"
)

func TestAPINew_whenRepliesUnset_thenErrors(t *testing.T) {
	// Arrange
	cfg := Config{APIKey: "key", History: DisabledHistory(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	// Act
	_, err := New(cfg)

	// Assert
	if err == nil {
		t.Fatal("expected an error for unset Replies")
	}
}

func TestAPINew_whenHistoryUnset_thenErrors(t *testing.T) {
	// Arrange
	cfg := Config{APIKey: "key", Replies: DisabledReplies(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	// Act
	_, err := New(cfg)

	// Assert
	if err == nil {
		t.Fatal("expected an error for unset History")
	}
}

func TestAPINew_whenHistoryAndRepliesDisabled_thenBuildsClient(t *testing.T) {
	// Arrange
	cfg := Config{APIKey: "key", History: DisabledHistory(), Replies: DisabledReplies(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	// Act
	_, err := New(cfg)

	// Assert
	if err != nil {
		t.Fatalf("New: %v", err)
	}
}
