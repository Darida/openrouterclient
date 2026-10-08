package selectionflag

import (
	"fmt"
	"strings"
)

// ParseExclude splits an --exclude value into model IDs, rejecting empty entries.
func ParseExclude(list string) ([]string, error) {
	ids := strings.Split(list, ",")
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("--exclude %q has an empty model ID", list)
		}
	}
	return ids, nil
}
