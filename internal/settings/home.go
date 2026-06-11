package settings

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandHome resolves a leading "~" or "~/" to the current user's home directory.
func ExpandHome(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, strings.TrimPrefix(p, "~")), nil
	}
	return p, nil
}
