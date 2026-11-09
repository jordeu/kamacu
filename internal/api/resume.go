package api

import (
	"os"
	"path/filepath"
	"strings"

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

// Status polling checks many tasks. Scan transcripts once, outside DB cursors,
// instead of globbing the entire projects tree separately for every task.
func transcriptLookup(root string) func(string) bool {
	var index map[string]bool
	return func(id string) bool {
		if root == "" || id == "" {
			return false
		}
		if _, err := uuid.Parse(id); err != nil {
			return false
		}
		if index == nil {
			index = map[string]bool{}
			matches, _ := filepath.Glob(filepath.Join(root, "*", "*.jsonl"))
			for _, path := range matches {
				index[strings.TrimSuffix(filepath.Base(path), ".jsonl")] = true
			}
		}
		return index[id]
	}
}
