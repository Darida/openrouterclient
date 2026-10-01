// Package replyfile keeps raw OpenRouter replies out of logs and errors by
// saving each one to its own file and naming that file.
package replyfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Local saves each reply under the system temp directory.
func Local() Saver { return Saver{mode: modeLocal} }

// Disabled saves nothing, so messages can point only to OpenRouter's own
// record of the generation.
func Disabled() Saver { return Saver{mode: modeDisabled} }

func (s Saver) IsZero() bool { return s.mode == modeUnset }

// Describe returns the phrase a message uses in place of body: where it was
// saved or, when saving is disabled, the generation id to fetch it by, if any.
func (s Saver) Describe(generationID, prefix string, body []byte) (string, error) {
	switch s.mode {
	case modeLocal:
		path, err := save(prefix, body)
		if err != nil {
			return "", err
		}
		return "body saved to " + path, nil
	case modeDisabled:
		if generationID == "" {
			return "body not saved (reply files disabled; no generation id)", nil
		}
		return fmt.Sprintf("body not saved (reply files disabled); OpenRouter generation id %s", generationID), nil
	}
	return "", errors.New("replyfile: Saver is unset")
}

func save(prefix string, body []byte) (string, error) {
	dir := filepath.Join(os.TempDir(), "openrouterclient")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("replyfile: create %s: %w", dir, err)
	}
	file, err := os.CreateTemp(dir, prefix+"-*.txt")
	if err != nil {
		return "", fmt.Errorf("replyfile: create reply file in %s: %w", dir, err)
	}
	defer file.Close()
	if _, err := file.Write(body); err != nil {
		return "", fmt.Errorf("replyfile: write %s: %w", file.Name(), err)
	}
	return file.Name(), nil
}
