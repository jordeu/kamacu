package api

import (
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// transcriptExists reports whether claude has a transcript for the given
// session id under globRoot (~/.claude/projects in production). Verified
// on v2.1.173: transcript existence == resumability — a never-prompted
// session writes no .jsonl and `--resume` of its id exits 1 (Pitfall 4),
// and resume lookup is global by id, so the glob matches claude's own
// behavior. The uuid.Parse guard means raw input never reaches the glob.
func transcriptExists(globRoot, claudeSessionID string) bool {
	if globRoot == "" || claudeSessionID == "" {
		return false
	}
	if _, err := uuid.Parse(claudeSessionID); err != nil {
		return false
	}
	matches, _ := filepath.Glob(filepath.Join(globRoot, "*", claudeSessionID+".jsonl"))
	return len(matches) > 0
}

// defaultTranscriptGlobRoot returns ~/.claude/projects ("" if home is
// unresolvable — every transcriptExists call then returns false, which
// degrades to Reset-only offers, never an error).
func defaultTranscriptGlobRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "projects")
}
