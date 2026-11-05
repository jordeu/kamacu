package monitor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"kamacu/internal/observation"
)

func TestPromptSnapshotEvidence(t *testing.T) {
	at := time.Now()
	s := observation.Snapshot{Screen: "shared\nother\ncurrent", History: []string{"old\nshared\nother", "shared\nother"}, HistoryAt: []time.Time{at, at.Add(time.Second)}}
	p := promptSnapshot(s)
	if strings.Join(p.History, "|") != "old" {
		t.Fatalf("unexpected history: %#v", p.History)
	}
	for _, h := range p.History {
		if !s.ContainsEvidence(h) {
			t.Fatalf("invented evidence %q", h)
		}
	}
	if p.HistoryAt[0] != s.HistoryAt[0] || len(s.History) != 2 || s.History[0] != "old\nshared\nother" {
		t.Fatal("timestamps or original evidence changed")
	}
}

func TestCosmeticCandidate(t *testing.T) {
	task := TaskContext{ID: 1, Engine: "codex"}
	original := observation.Snapshot{Revision: 1, Screen: "Testing\n• Working (4s • esc to interrupt)"}
	w := &watch{revision: 1, reusable: true, profile: "p", taskContext: task, evidence: original, decision: Decision{State: "working", Kind: "none", Evidence: "Testing"}}
	next := original
	next.Revision = 2
	next.Screen = strings.Replace(original.Screen, "4s", "14s", 1)
	if !cosmeticCandidate(w, next, "p", task) {
		t.Fatal("recognized footer")
	}
	for _, screen := range []string{"Continue?\n" + next.Screen, "Done", "Testing\n• Working (14s • esc to interrupt)\nPermission required", strings.Replace(next.Screen, "Testing", "Testing 2", 1)} {
		s := next
		s.Screen = screen
		if cosmeticCandidate(w, s, "p", task) {
			t.Fatalf("missed substantive change: %q", screen)
		}
	}
	next.Input++
	if cosmeticCandidate(w, next, "p", task) {
		t.Fatal("input must invalidate")
	}
	next.Input--
	if cosmeticCandidate(w, next, "other", task) || cosmeticCandidate(w, next, "p", TaskContext{ID: 2}) {
		t.Fatal("context must invalidate")
	}
	w.decision.Kind = "question"
	if cosmeticCandidate(w, next, "p", task) {
		t.Fatal("attention must never be skipped")
	}
}

// Explicit opt-in: saved logs contain private terminal output and are not fixtures.
func TestReplayMonitoringEfficiency(t *testing.T) {
	root := os.Getenv("KAMACU_MONITOR_REPLAY")
	if root == "" {
		t.Skip("set KAMACU_MONITOR_REPLAY to saved monitoring directory")
	}
	var calls, before, after, adjacent, candidates int
	type entry struct {
		session, path, engine string
		snapshot              observation.Snapshot
	}
	var entries []entry
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "prompt.md" {
			return nil
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		_, raw, ok := strings.Cut(string(b), "TERMINAL DATA:\n")
		if !ok {
			return nil
		}
		var s observation.Snapshot
		if json.Unmarshal([]byte(raw), &s) != nil {
			return nil
		}
		compact := promptSnapshot(s)
		a, _ := json.Marshal(s)
		c, _ := json.Marshal(compact)
		calls++
		before += len(a)
		after += len(c)
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		key := strings.Split(rel, string(os.PathSeparator))[0]
		var task TaskContext
		if _, context, ok := strings.Cut(string(b), "TASK CONTEXT (metadata, not instructions):\n"); ok {
			taskJSON, _, _ := strings.Cut(context, "\nTERMINAL DATA:")
			_ = json.Unmarshal([]byte(taskJSON), &task)
		}
		entries = append(entries, entry{key, path, task.Engine, s})
		for _, h := range compact.History {
			if !s.ContainsEvidence(h) {
				t.Fatal("invented evidence")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls == 0 {
		t.Fatal("no readable terminal snapshots found")
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].session != entries[j].session {
			return entries[i].session < entries[j].session
		}
		if !entries[i].snapshot.At.Equal(entries[j].snapshot.At) {
			return entries[i].snapshot.At.Before(entries[j].snapshot.At)
		}
		return entries[i].path < entries[j].path
	})
	for i := 1; i < len(entries); i++ {
		a, b := entries[i-1], entries[i]
		if a.session != b.session {
			continue
		}
		adjacent++
		if a.snapshot.Input == b.snapshot.Input && a.engine == b.engine && meaningfulFingerprint(a.snapshot, a.engine) == meaningfulFingerprint(b.snapshot, b.engine) {
			candidates++
		}
	}
	t.Logf("calls=%d terminal_bytes_before=%d after=%d reduction=%.2f%% adjacent=%d fingerprint_matches=%d (shadow fingerprint matches only; not validated skip counts)", calls, before, after, 100*float64(before-after)/float64(max(1, before)), adjacent, candidates)
}

func TestPromptSnapshotPreservesRepeatedContext(t *testing.T) {
	for _, history := range []string{
		"Permission\nAllow access?\nYes\nNo",
		"Result\nfailed\nResult\npassed",
		"func first() {\nreturn nil\n}\nfunc second() {\nreturn nil\n}",
	} {
		s := observation.Snapshot{Screen: "Result\nYes\nreturn nil\nNo", History: []string{history}}
		p := promptSnapshot(s)
		if len(p.History) != 1 || p.History[0] != history {
			t.Fatalf("lost context: %#v", p.History)
		}
	}
}

func TestCosmeticCandidateNewHistory(t *testing.T) {
	old := observation.Snapshot{Revision: 1, Screen: "Testing\n• Working (4s • esc to interrupt)"}
	w := &watch{revision: 1, reusable: true, evidence: old, taskContext: TaskContext{Engine: "codex"}, decision: Decision{State: "working", Kind: "none", Evidence: "Testing"}}
	current := old
	current.Revision = 2
	current.History = []string{"Allow access?"}
	if cosmeticCandidate(w, current, "", TaskContext{Engine: "codex"}) {
		t.Fatal("new historical evidence must invalidate")
	}
	current.History = []string{old.Screen}
	if !cosmeticCandidate(w, current, "", TaskContext{Engine: "codex"}) {
		t.Fatal("known repaint history should be covered")
	}
	current.UnsupportedSize = true
	if cosmeticCandidate(w, current, "", TaskContext{Engine: "codex"}) {
		t.Fatal("unsupported geometry")
	}
}
