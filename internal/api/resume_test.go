package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

// TestTranscriptExists pins the single resumability rule (Pattern 1): a hit
// only when root/<anydir>/<id>.jsonl exists, with an empty-id / non-uuid /
// missing-file miss. The uuid.Parse guard means raw input never reaches the
// glob.
func TestTranscriptExists(t *testing.T) {
	root := t.TempDir()
	id := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeee0001"

	// No fixture yet → miss.
	if transcriptExists(root, id) {
		t.Errorf("transcriptExists with no fixture = true, want false")
	}

	// Create root/-home-x-wt/<id>.jsonl → hit.
	projDir := filepath.Join(root, "-home-x-wt")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projDir, id+".jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write transcript fixture: %v", err)
	}
	if !transcriptExists(root, id) {
		t.Errorf("transcriptExists with fixture = false, want true")
	}

	// Empty id → false (no glob).
	if transcriptExists(root, "") {
		t.Errorf("transcriptExists(empty id) = true, want false")
	}
	// Empty root → false.
	if transcriptExists("", id) {
		t.Errorf("transcriptExists(empty root) = true, want false")
	}
	// Non-uuid id → false (glob-injection guard); even if a matching file existed.
	bad := "../../etc/passwd*"
	if _, err := uuid.Parse(bad); err == nil {
		t.Fatalf("test invariant: %q unexpectedly parses as a uuid", bad)
	}
	if transcriptExists(root, bad) {
		t.Errorf("transcriptExists(non-uuid id) = true, want false")
	}
	// A different (well-formed) uuid with no file → miss.
	if transcriptExists(root, "ffffffff-0000-0000-0000-000000000000") {
		t.Errorf("transcriptExists(unknown uuid) = true, want false")
	}
}
