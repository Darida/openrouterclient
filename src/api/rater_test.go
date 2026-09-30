package api

import "testing"

func TestNewRater_whenHistoryUnset_thenErrors(t *testing.T) {
	// Act
	_, err := NewRater(History{})

	// Assert
	if err == nil {
		t.Fatal("expected an error for an unset history")
	}
}

func TestNewRater_whenHistoryDisabled_thenErrors(t *testing.T) {
	// Act
	_, err := NewRater(DisabledHistory())

	// Assert
	if err == nil {
		t.Fatal("expected an error for disabled history")
	}
}
