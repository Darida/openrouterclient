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
	described, err := Local().Describe("gen-1", "reply", []byte(`{"status": "LGTM"`))

	// Assert
	if err != nil {
		t.Fatal(err)
	}
	path := strings.TrimPrefix(described, "body saved to ")
	got, err := os.ReadFile(path)
	if err != nil || string(got) != `{"status": "LGTM"` {
		t.Fatalf("ReadFile(%s) = %q, %v; want the saved body", path, got, err)
	}
}

func TestSaverDescribe_whenDisabled_thenNamesGenerationID(t *testing.T) {
	// Act
	described, err := Disabled().Describe("gen-1", "reply", []byte(`{"status": "LGTM"`))

	// Assert
	if err != nil || !strings.Contains(described, "gen-1") {
		t.Fatalf("described = %q, want it to name gen-1", described)
	}
}

func TestSaverDescribe_whenDisabled_thenWritesNoFile(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)

	// Act
	if _, err := Disabled().Describe("gen-1", "reply", []byte(`{"status": "LGTM"`)); err != nil {
		t.Fatal(err)
	}

	// Assert
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("temp dir entries = %v, %v; want none", entries, err)
	}
}

func TestSaverDescribe_whenUnset_thenErrors(t *testing.T) {
	// Act
	_, err := Saver{}.Describe("gen-1", "reply", nil)

	// Assert
	if err == nil {
		t.Fatal("expected an error")
	}
}
