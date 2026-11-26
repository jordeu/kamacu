package pi

import (
	"path/filepath"
	"strings"
)

// AgentDir resolves Pi's configuration directory relative to the server's
// directory so the installer and worktree processes use the same profile.
func AgentDir(home, override string) (string, error) {
	switch {
	case override == "":
		return filepath.Join(home, ".pi", "agent"), nil
	case override == "~":
		return home, nil
	case strings.HasPrefix(override, "~/"):
		return filepath.Join(home, strings.TrimPrefix(override, "~/")), nil
	default:
		return filepath.Abs(override)
	}
}
