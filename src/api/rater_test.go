package api

import "testing"

func TestAPINewRater_whenHistoryUnset_thenErrors(t *testing.T) {
	// Act
	_, err := NewRater(History{})

	// Assert
	if err == nil {
		t.Fatal("expected an error for an unset history")
	}
}

func TestAPINewRater_whenHistoryDisabled_thenErrors(t *testing.T) {
	// Act
	_, err := NewRater(DisabledHistory())

	// Assert
	if err == nil {
		t.Fatal("expected an error for disabled history")
	}
}
