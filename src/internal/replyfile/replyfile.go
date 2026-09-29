// Package replyfile keeps raw OpenRouter replies out of logs, errors, and
// panic messages by saving each one to its own file and naming that file.
package replyfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// Save writes body to a new prefix-named file under the system temp directory and returns its path.
func Save(prefix string, body []byte) string {
	dir := filepath.Join(os.TempDir(), "openrouterclient")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(fmt.Sprintf("replyfile: create %s: %v", dir, err))
	}
	file, err := os.CreateTemp(dir, prefix+"-*.txt")
	if err != nil {
		panic(fmt.Sprintf("replyfile: create reply file in %s: %v", dir, err))
	}
	defer file.Close()
	if _, err := file.Write(body); err != nil {
		panic(fmt.Sprintf("replyfile: write %s: %v", file.Name(), err))
	}
	return file.Name()
}
