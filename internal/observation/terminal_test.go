package observation

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRenderingAndInput(t *testing.T) {
	v := New(80, 24)
	v.Feed([]byte("Working\r\x1b[2KReady?\n"))
	v.Feed([]byte{0xe2, 0x82})
	v.Feed([]byte{0xac})
	s := v.Snapshot()
	if strings.Contains(s.Screen, "Working") || !strings.Contains(s.Screen, "Ready?") || !strings.Contains(s.Screen, "€") {
		t.Fatalf("bad screen %q", s.Screen)
	}
	v.Input()
	if v.Snapshot().Input != 1 {
		t.Fatal("input generation")
	}
	v.Feed([]byte("\x1b[2J\x1b[HNew"))
	if v.Snapshot().Revision <= s.Revision {
		t.Fatal("missing revision")
	}
	if len(v.Snapshot().History) == 0 {
		t.Fatal("missing rendered history")
	}
}
func TestBoundedHistory(t *testing.T) {
	v := New(80, 8)
	for i := 0; i < 1000; i++ {
		v.Feed([]byte(fmt.Sprintf("%04d %s\r\n", i, strings.Repeat("x", 70))))
	}
	s := v.Snapshot()
	n := len(s.Screen)
	for _, h := range s.History {
		n += len(h)
	}
	if n > 24000 || len(s.History) > 256 {
		t.Fatal("unbounded evidence", n)
	}
}

func TestScrollbackSurvivesRepaints(t *testing.T) {
	v := New(100, 6)
	const explanation = "Problem: native memory exhausted the pod.\nSolution: reduce the embedded database cap."
	v.Feed([]byte(strings.ReplaceAll(explanation, "\n", "\r\n")))
	rendered := v.Snapshot().Screen
	original := v.Snapshot().EvidenceAt(rendered)
	v.Feed([]byte("\x1b[2J\x1b[HChoose review effort\r\n1. Low\r\n2. High\r\nspinner 0"))
	for i := 1; i <= 100; i++ {
		v.Feed([]byte(fmt.Sprintf("\r\x1b[2Kspinner %d", i)))
	}
	s := v.Snapshot()
	if !s.ContainsEvidence(rendered) || !s.EvidenceAt(rendered).Equal(original) {
		t.Fatal("repaints displaced the explanation or changed its time", s.History)
	}
	if !strings.Contains(s.Screen, "Choose review effort") || strings.Contains(s.Screen, "Problem:") {
		t.Fatal("history changed the current screen", s.Screen)
	}
}

func TestLongResponseRetainsOpening(t *testing.T) {
	v := New(80, 6)
	var response strings.Builder
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&response, "Explanation line %02d\r\n", i)
	}
	v.Feed([]byte(response.String()))
	s := v.Snapshot()
	for i := 0; i < 60; i++ {
		if !s.ContainsEvidence(fmt.Sprintf("Explanation line %02d", i)) {
			t.Fatalf("lost response line %d", i)
		}
	}
}

func TestHistoryPreservesContextAcrossComplexRepaints(t *testing.T) {
	for _, tc := range []struct{ name, original, repaint string }{
		{"headings", "Before\r\nfailed\r\nAfter\r\npassed", "\x1b[2;1Hpassed\x1b[K\x1b[4;1Hfailed\x1b[K"},
		{"code", "func first() {\r\nreturn 1\r\n}\r\nfunc other() {\r\nreturn 2\r\n}", "\x1b[1;1Hfunc other() {\x1b[4;1Hfunc first() {"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := New(80, 8)
			v.Feed([]byte(tc.original))
			before := v.Snapshot()
			// Reorder existing lines without clearing the screen. A bag of matching
			// lines must not erase the earlier association between headings and values.
			v.Feed([]byte(tc.repaint))
			s := v.Snapshot()
			if !s.ContainsEvidence(before.Screen) {
				t.Fatal("lost the original ordering and heading context", s.History)
			}
			if !s.EvidenceAt(before.Screen).Equal(before.EvidenceAt(before.Screen)) {
				t.Fatal("changed the original context's timestamp")
			}
		})
	}
}

func TestDistinctOutputStillExpiresOldHistory(t *testing.T) {
	v := New(80, 4)
	v.Feed([]byte("Old question?"))
	for i := 0; i < 600; i++ {
		v.Feed([]byte(fmt.Sprintf("\x1b[2J\x1b[HNew output %04d %s", i, strings.Repeat("x", 50))))
	}
	s := v.Snapshot()
	if s.ContainsEvidence("Old question?") {
		t.Fatal("retained old evidence beyond the history budget")
	}
	n := len(s.Screen)
	for _, h := range s.History {
		n += len(h)
	}
	if n > 24000 || len(s.History) > 256 || len(s.History) != len(s.HistoryAt) || len(s.History) != len(s.HistoryLineTimes) {
		t.Fatal("history budget or timestamp alignment broken", n)
	}
}

func TestEvidenceTimestampSurvivesUnrelatedRepaint(t *testing.T) {
	v := New(80, 24)
	v.Feed([]byte("All done\r\nspinner |"))
	before := v.Snapshot().EvidenceAt("All done")
	v.Feed([]byte("\r\x1b[2Kspinner /"))
	after := v.Snapshot()
	if !after.EvidenceAt("All done").Equal(before) {
		t.Fatal("completion timestamp changed on spinner repaint")
	}
	if !after.EvidenceAt("spinner /").After(before) {
		t.Fatal("new evidence has old timestamp")
	}
}

func TestRecordedCodexApproval(t *testing.T) {
	data, err := os.ReadFile("../session/testdata/codex/0.160.1-approval.ansi")
	if err != nil {
		t.Fatal(err)
	}
	v := New(120, 30)
	for start := 0; start < 15001; start += 7 {
		end := min(start+7, 15001)
		v.Feed(data[start:end])
	}
	if !strings.Contains(v.Snapshot().Screen, "Would you like to run") {
		t.Fatalf("approval screen missing: %q", v.Snapshot().Screen)
	}
	for start := 15001; start < len(data); start += 7 {
		end := min(start+7, len(data))
		v.Feed(data[start:end])
	}
	if strings.Contains(v.Snapshot().Screen, "Would you like to run") {
		t.Fatal("approval remained visible after completion")
	}
}

func TestSmallGeometryAndResizeInvalidation(t *testing.T) {
	v := New(8, 2)
	v.Feed([]byte("abcdefghij"))
	s := v.Snapshot()
	if s.Screen != "abcdefgh\nij" {
		t.Fatalf("rendered at wrong geometry: %q", s.Screen)
	}
	v = New(80, 24)
	v.Feed([]byte("Continue?"))
	before := v.Snapshot()
	v.Resize(81, 24)
	if v.Snapshot().Revision == before.Revision {
		t.Fatal("resize failed to invalidate replies")
	}
	v.Resize(500, 24)
	if !v.Snapshot().UnsupportedSize {
		t.Fatal("clipped geometry trusted")
	}
	v.Resize(80, 24)
	if v.Snapshot().UnsupportedSize || v.Snapshot().Screen != "" {
		t.Fatal("unsupported output reused after resize")
	}
	v.Feed([]byte("Redrawn question?"))
	if v.Snapshot().Screen != "Redrawn question?" {
		t.Fatal("resize did not recover")
	}
}

func TestRepeatedEvidenceGetsFreshTimestamp(t *testing.T) {
	v := New(80, 24)
	v.Feed([]byte("All done\r\n"))
	first := v.Snapshot().EvidenceAt("All done")
	v.Input()
	input := v.Snapshot().InputAt
	v.Feed([]byte("All done\r\n"))
	second := v.Snapshot().EvidenceAt("All done")
	if !second.After(first) || !second.After(input) {
		t.Fatal("new completion inherited old completion time")
	}
}

func TestActivityIgnoresResizeAndIdenticalRepaints(t *testing.T) {
	v := New(80, 24)
	v.Feed([]byte("Ready>"))
	before := v.Snapshot().OutputAt
	if before.IsZero() {
		t.Fatal("output activity missing")
	}
	v.Feed([]byte("\rReady>\x1b[?25h"))
	if !v.Snapshot().OutputAt.Equal(before) {
		t.Fatal("identical repaint counted as activity")
	}
	v.Resize(100, 30)
	if !v.Snapshot().OutputAt.Equal(before) {
		t.Fatal("resize counted as activity")
	}
	v.Feed([]byte("Working"))
	if !v.Snapshot().OutputAt.After(before) {
		t.Fatal("new output missing activity")
	}
}

func TestHistoricalEvidenceRetainsOriginalTime(t *testing.T) {
	terminal := New(100, 3)
	evidence := "Created https://github.com/acme/repo/pull/7"
	terminal.Feed([]byte(evidence))
	original := terminal.Snapshot().EvidenceAt(evidence)
	terminal.Feed([]byte("\x1b[2J\x1b[HRunning tests\r\n"))
	snap := terminal.Snapshot()
	if strings.Contains(snap.Screen, evidence) || !snap.ContainsEvidence(evidence) {
		t.Fatal("expected scrolled evidence in history", snap)
	}
	if !snap.EvidenceAt(evidence).Equal(original) {
		t.Fatalf("historical evidence became new: %s vs %s", original, snap.EvidenceAt(evidence))
	}
	if snap.ContainsEvidence("tests\nCreated") {
		t.Fatal("combined unrelated snapshots into evidence")
	}
	// Returned history metadata must not alias the observer's internal buffers.
	for i := range snap.HistoryLineTimes {
		for j := range snap.HistoryLineTimes[i] {
			snap.HistoryLineTimes[i][j] = time.Time{}
		}
	}
	if !terminal.Snapshot().EvidenceAt(evidence).Equal(original) {
		t.Fatal("snapshot mutation changed retained timing")
	}
	for i := 0; i < 20; i++ {
		terminal.Feed([]byte(fmt.Sprintf("line %d\r\n", i)))
	}
	snap = terminal.Snapshot()
	if len(snap.History) != len(snap.HistoryAt) || len(snap.History) != len(snap.HistoryLineTimes) {
		t.Fatal("history timing lost alignment after trimming")
	}
}
