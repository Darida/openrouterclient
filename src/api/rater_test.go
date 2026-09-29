package api

import "testing"

func TestNewRater_whenHistoryPathEmpty_thenErrors(t *testing.T) {
	// Act
	_, err := NewRater("")

	// Assert
	if err == nil {
		t.Fatal("expected an error for an empty history path")
	}
}
