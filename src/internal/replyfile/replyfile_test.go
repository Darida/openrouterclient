package replyfile

import (
	"os"
	"strings"
	"testing"
)

func TestSaverDescribe_whenLocal_thenNamedFileHoldsBody(t *testing.T) {
	// Arrange
	t.Setenv("TMPDIR", t.TempDir())

	// Act
	described := Local().Describe("gen-1", "reply", []byte(`{"status": "LGTM"`))

	// Assert
	path := strings.TrimPrefix(described, "body saved to ")
	got, err := os.ReadFile(path)
	if err != nil || string(got) != `{"status": "LGTM"` {
		t.Fatalf("ReadFile(%s) = %q, %v; want the saved body", path, got, err)
	}
}

func TestSaverDescribe_whenDisabled_thenNamesGenerationID(t *testing.T) {
	// Act
	described := Disabled().Describe("gen-1", "reply", []byte(`{"status": "LGTM"`))

	// Assert
	if !strings.Contains(described, "gen-1") {
		t.Fatalf("described = %q, want it to name gen-1", described)
	}
}

func TestSaverDescribe_whenDisabled_thenWritesNoFile(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)

	// Act
	Disabled().Describe("gen-1", "reply", []byte(`{"status": "LGTM"`))

	// Assert
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("temp dir entries = %v, %v; want none", entries, err)
	}
}

func TestSaverDescribe_whenUnset_thenPanics(t *testing.T) {
	// Assert
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()

	// Act
	Saver{}.Describe("gen-1", "reply", nil)
}
