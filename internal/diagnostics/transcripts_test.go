package diagnostics

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func transcriptFields() map[string]any {
	return map[string]any{"purpose": "notification", "prompt": "Full prompt\nwith context", "response": `{"message":"hello","ok":true}`, "provider": map[string]any{"output": "raw output", "stderr": "diagnostic"}, "schema": map[string]string{"type": "object"}, "model": "fixture", "duration_ms": 12, "error": ""}
}
func groupDirs(t *testing.T, r *Recorder) []string {
	t.Helper()
	var dirs []string
	err := filepath.WalkDir(r.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == "group.json" {
			dirs = append(dirs, filepath.Dir(path))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return dirs
}
func TestGroupedConcurrentTranscripts(t *testing.T) {
	root := t.TempDir()
	r, err := NewRecorder(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = WithGroup(ctx, Group{Kind: "batch", ID: "batch-123", StartedAt: time.Date(2026, 11, 4, 15, 30, 0, 0, time.UTC)})
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Go(func() { r.Record(ctx, transcriptFields()) })
	}
	wg.Wait()
	dirs := groupDirs(t, r)
	if len(dirs) != 1 {
		t.Fatalf("groups: %v", dirs)
	}
	if !strings.Contains(dirs[0], filepath.Join("notifications", "2026-11-04", "153000.000000000_batch-batch-123")) {
		t.Fatal(dirs)
	}
	entries, err := os.ReadDir(dirs[0])
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		count++
		dir := filepath.Join(dirs[0], entry.Name())
		for _, file := range []string{"prompt.md", "response.md", "metadata.json", "provider.json", "schema.json"} {
			data, e := os.ReadFile(filepath.Join(dir, file))
			if e != nil {
				t.Fatal(e)
			}
			switch file {
			case "prompt.md":
				if string(data) != "Full prompt\nwith context" {
					t.Fatal(string(data))
				}
			case "response.md":
				if !strings.Contains(string(data), "```json\n{\n  ") {
					t.Fatal(string(data))
				}
			default:
				if !json.Valid(data) {
					t.Fatalf("%s: %s", file, data)
				}
			}
		}
	}
	if count != 30 {
		t.Fatalf("calls: %d", count)
	}
	summary, err := os.ReadFile(filepath.Join(dirs[0], "summary.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(summary), "prompt.md") != 30 {
		t.Fatal(string(summary))
	}
	// Reopening the recorder preserves the sequence and existing calls.
	reopened, err := NewRecorder(root)
	if err != nil {
		t.Fatal(err)
	}
	reopened.Record(ctx, transcriptFields())
	entries, err = os.ReadDir(dirs[0])
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "031_") {
			found = true
		}
	}
	if !found {
		t.Fatal("sequence did not survive restart")
	}
	err = filepath.WalkDir(r.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		want := os.FileMode(0600)
		if d.IsDir() {
			want = 0700
		}
		if info.Mode().Perm() != want {
			t.Errorf("%s permissions %v", path, info.Mode().Perm())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
func TestMonitoringLayout(t *testing.T) {
	r, err := NewRecorder(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithGroup(context.Background(), Group{Kind: "session", ID: "abc", SessionID: "abc", TaskID: 42, TaskTitle: "Improve performance", ProjectID: 12, ProjectName: "kamacu"})
	fields := transcriptFields()
	fields["purpose"] = "monitoring"
	r.Record(ctx, fields)
	r.Record(ctx, fields)
	dirs := groupDirs(t, r)
	want := filepath.Join(r.root, "monitoring", "session-abc", time.Now().UTC().Format("2006-01-02"))
	if len(dirs) != 1 || dirs[0] != want {
		t.Fatalf("groups: %v", dirs)
	}
	summary, err := os.ReadFile(filepath.Join(filepath.Dir(want), "summary.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(summary), "Improve performance") || !strings.Contains(string(summary), "kamacu") {
		t.Fatal(string(summary))
	}
}
func ageGroup(t *testing.T, path string, at time.Time) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(path, "group.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest groupManifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.UpdatedAt = at
	if err = writeJSON(filepath.Join(path, "group.json"), manifest); err != nil {
		t.Fatal(err)
	}
}
func TestRetentionProtectsActiveWorkflow(t *testing.T) {
	r, err := NewRecorder(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = WithGroup(ctx, Group{Kind: "batch", ID: "active"})
	r.Record(ctx, transcriptFields())
	r.Record(ctx, transcriptFields())
	dirs := groupDirs(t, r)
	if len(dirs) != 1 {
		t.Fatal(dirs)
	}
	now := time.Now().UTC()
	ageGroup(t, dirs[0], now.Add(-8*24*time.Hour))
	r.maxBytes = 1
	if err = r.cleanup(now, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(dirs[0]); err != nil {
		t.Fatal("active group deleted", err)
	}
	cancel()
	if err = r.cleanup(now, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(dirs[0]); !os.IsNotExist(err) {
		t.Fatal("completed group retained", err)
	}
}
func TestRetentionMonitoringDayAndSize(t *testing.T) {
	r, err := NewRecorder(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithGroup(context.Background(), Group{Kind: "session", ID: "session", SessionID: "session"})
	fields := transcriptFields()
	fields["purpose"] = "monitoring"
	r.Record(ctx, fields)
	dirs := groupDirs(t, r)
	if len(dirs) != 1 {
		t.Fatal(dirs)
	}
	now := time.Now().UTC()
	r.maxBytes = 1
	if err = r.cleanup(now, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(dirs[0]); err != nil {
		t.Fatal("current day deleted", err)
	}
	old := filepath.Join(filepath.Dir(dirs[0]), now.Add(-24*time.Hour).Format("2006-01-02"))
	if err = os.Rename(dirs[0], old); err != nil {
		t.Fatal(err)
	}
	if err = r.cleanup(now, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("old group retained despite size budget", err)
	}
	if _, err = os.Stat(filepath.Dir(old)); !os.IsNotExist(err) {
		t.Fatal("empty session retained", err)
	}
}
func TestUnsafeGroupIDsAndErrorResponse(t *testing.T) {
	r, err := NewRecorder(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithGroup(context.Background(), Group{Kind: "../../outside", ID: "../../../escape"})
	fields := transcriptFields()
	fields["response"] = "invalid response"
	fields["error"] = "provider failed"
	r.Record(ctx, fields)
	dirs := groupDirs(t, r)
	if len(dirs) != 1 {
		t.Fatal(dirs)
	}
	entries, err := os.ReadDir(dirs[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			data, err := os.ReadFile(filepath.Join(dirs[0], e.Name(), "response.md"))
			if err != nil || string(data) != "invalid response" {
				t.Fatalf("%q %v", data, err)
			}
		}
	}
	if _, err = os.Stat(filepath.Join(filepath.Dir(r.root), "escape")); !os.IsNotExist(err) {
		t.Fatal("escaped root")
	}
}

func TestRetentionAgeAndOldestFirst(t *testing.T) {
	r, err := NewRecorder(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, id := range []string{"old", "recent", "newest"} {
		ctx := WithGroup(context.Background(), Group{Kind: "batch", ID: id})
		r.Record(ctx, transcriptFields())
	}
	dirs := groupDirs(t, r)
	if len(dirs) != 3 {
		t.Fatal(dirs)
	}
	var oldest, recent, newest string
	for _, dir := range dirs {
		switch {
		case strings.HasSuffix(dir, "_batch-old"):
			oldest = dir
			ageGroup(t, dir, now.Add(-8*24*time.Hour))
		case strings.HasSuffix(dir, "_batch-recent"):
			recent = dir
			ageGroup(t, dir, now.Add(-2*time.Hour))
		case strings.HasSuffix(dir, "_batch-newest"):
			newest = dir
			ageGroup(t, dir, now.Add(-time.Hour))
		}
	}
	if oldest == "" || recent == "" || newest == "" {
		t.Fatal(dirs)
	}
	if err = r.cleanup(now, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(oldest); !os.IsNotExist(err) {
		t.Fatal("age retention failed", err)
	}
	if _, err = os.Stat(recent); err != nil {
		t.Fatal("age retention removed recent group", err)
	}
	r.maxBytes = 1
	if err = r.cleanup(now, newest); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(recent); !os.IsNotExist(err) {
		t.Fatal("size retention failed", err)
	}
	if _, err = os.Stat(newest); err != nil {
		t.Fatal("current group removed", err)
	}
}

func TestRetentionRefreshesSessionIndex(t *testing.T) {
	r, err := NewRecorder(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithGroup(context.Background(), Group{Kind: "session", ID: "index", SessionID: "index"})
	fields := transcriptFields()
	fields["purpose"] = "monitoring"
	r.Record(ctx, fields)
	dirs := groupDirs(t, r)
	if len(dirs) != 1 {
		t.Fatal(dirs)
	}
	parent := filepath.Dir(dirs[0])
	now := time.Now().UTC()
	oldDate := now.Add(-8 * 24 * time.Hour).Format("2006-01-02")
	old := filepath.Join(parent, oldDate)
	if err = os.Mkdir(old, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dirs[0], "group.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(old, "group.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	ageGroup(t, old, now.Add(-8*24*time.Hour))
	if err = writeSessionSummary(parent); err != nil {
		t.Fatal(err)
	}
	if err = r.cleanup(now, ""); err != nil {
		t.Fatal(err)
	}
	summary, err := os.ReadFile(filepath.Join(parent, "summary.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(summary), oldDate) || !strings.Contains(string(summary), now.Format("2006-01-02")) {
		t.Fatalf("stale index: %s", summary)
	}
}
