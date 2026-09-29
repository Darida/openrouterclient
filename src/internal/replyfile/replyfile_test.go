package replyfile

import (
	"os"
	"testing"
)

func TestReplyfileSave_whenCalled_thenFileHoldsBody(t *testing.T) {
	// Arrange
	t.Setenv("TMPDIR", t.TempDir())

	// Act
	path := Save("reply", []byte(`{"status": "LGTM"`))

	// Assert
	got, err := os.ReadFile(path)
	if err != nil || string(got) != `{"status": "LGTM"` {
		t.Fatalf("ReadFile(%s) = %q, %v; want the saved body", path, got, err)
	}
}
