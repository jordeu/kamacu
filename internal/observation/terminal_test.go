package observation

import (
	"os"
	"strings"
	"testing"
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
		v.Feed([]byte(strings.Repeat("x", 70) + "\r\n"))
	}
	s := v.Snapshot()
	n := len(s.Screen)
	for _, h := range s.History {
		n += len(h)
	}
	if n > 24000 || len(s.History) > 8 {
		t.Fatal("unbounded evidence", n)
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
