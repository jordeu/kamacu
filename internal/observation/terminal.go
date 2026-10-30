// Package observation renders terminal output without interpreting agent state.
package observation

import (
	"crypto/sha256"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/hinshun/vt10x"
)

type Snapshot struct {
	OutputAt        time.Time   `json:"output_at,omitempty"`
	InputAt         time.Time   `json:"input_at,omitempty"`
	UnsupportedSize bool        `json:"unsupported_size,omitempty"`
	Revision        uint64      `json:"revision"`
	Input           uint64      `json:"input_generation"`
	Screen          string      `json:"screen"`
	History         []string    `json:"history"`
	At              time.Time   `json:"at"`
	LineTimes       []time.Time `json:"-"`
	Digest          string      `json:"digest"`
}

type Terminal struct {
	cols, rows int
	mu         sync.Mutex
	vt         vt10x.Terminal
	tail       []byte
	snap       Snapshot
}

func New(cols, rows int) *Terminal {
	c, r := size(cols, rows)
	return &Terminal{cols: cols, rows: rows, snap: Snapshot{UnsupportedSize: c != cols || r != rows}, vt: vt10x.New(vt10x.WithSize(c, r), vt10x.WithWriter(io.Discard))}
}
func size(cols, rows int) (int, int) { return max(1, min(cols, 300)), max(1, min(rows, 100)) }
func (t *Terminal) Feed(p []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	screen := t.snap.Screen
	data := append(t.tail, p...)
	end := 0
	for end < len(data) {
		if !utf8.FullRune(data[end:]) {
			break
		}
		_, n := utf8.DecodeRune(data[end:])
		end += n
	}
	// Capture intermediate screens at newlines, so a large output burst cannot
	// scroll all its evidence away before the final snapshot is taken.
	start := 0
	for i := 0; i < end; i++ {
		if data[i] == '\n' {
			_, _ = t.vt.Write(data[start : i+1])
			t.capture()
			start = i + 1
		}
	}
	if start < end {
		_, _ = t.vt.Write(data[start:end])
		t.capture()
	}
	if t.snap.Screen != screen {
		t.snap.OutputAt = t.snap.At
	}
	t.tail = append([]byte(nil), data[end:]...)
}
func (t *Terminal) capture() {
	screen := strings.TrimSpace(t.vt.String())
	if screen == t.snap.Screen {
		return
	}
	if t.snap.Screen != "" {
		t.snap.History = append(t.snap.History, t.snap.Screen)
	}
	// Bound evidence by bytes as well as snapshots. History is rendered text,
	// not raw escape codes, and no terminal query is ever answered here.
	total := len(screen)
	for i := len(t.snap.History) - 1; i >= 0; i-- {
		total += len(t.snap.History[i])
		if total > 24000 || len(t.snap.History)-i > 8 {
			t.snap.History = append([]string(nil), t.snap.History[i+1:]...)
			break
		}
	}

	// Preserve the time each visible line first appeared across unrelated TUI
	// repaints. A spinner must not turn a completion seen live into a new event.
	oldTimes := map[string][]time.Time{}
	for i, line := range strings.Split(t.snap.Screen, "\n") {
		line = strings.TrimRight(line, " \t")
		if i < len(t.snap.LineTimes) {
			oldTimes[line] = append(oldTimes[line], t.snap.LineTimes[i])
		}
	}
	now := time.Now()
	t.snap.LineTimes = nil
	for _, line := range strings.Split(screen, "\n") {
		line = strings.TrimRight(line, " \t")
		at := now
		if previous := oldTimes[line]; len(previous) > 0 {
			at = previous[0]
			oldTimes[line] = previous[1:]
		}
		t.snap.LineTimes = append(t.snap.LineTimes, at)
	}
	t.snap.Screen = screen
	t.snap.Revision++
	t.snap.At = time.Now()
	t.snap.Digest = fmt.Sprintf("%x", sha256.Sum256([]byte(screen)))
}
func (t *Terminal) Resize(cols, rows int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cols == cols && t.rows == rows {
		return
	}
	t.cols, t.rows = cols, rows
	c, r := size(cols, rows)
	wasUnsupported := t.snap.UnsupportedSize
	t.snap.UnsupportedSize = c != cols || r != rows
	if wasUnsupported && !t.snap.UnsupportedSize {
		// Bytes rendered at the wrong geometry cannot safely be replayed. Await
		// fresh output after SIGWINCH instead of trusting the clipped screen.
		t.vt = vt10x.New(vt10x.WithSize(c, r), vt10x.WithWriter(io.Discard))
		t.snap.History = nil
	} else {
		t.vt.Resize(c, r)
	}
	revision := t.snap.Revision
	t.capture()
	if t.snap.Revision == revision {
		t.snap.Revision++
		t.snap.At = time.Now()
	}
}
func (t *Terminal) Input() { t.mu.Lock(); t.snap.Input++; t.snap.InputAt = time.Now(); t.mu.Unlock() }
func (t *Terminal) Snapshot() Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.snap
	s.History = append([]string(nil), s.History...)
	s.LineTimes = append([]time.Time(nil), s.LineTimes...)
	return s
}

// EvidenceAt is when all lines of a cited excerpt were present. Fall back to
// the capture timestamp for synthetic or older snapshots without line timing.
func (s Snapshot) EvidenceAt(excerpt string) time.Time {
	start := strings.LastIndex(s.Screen, excerpt)
	if start < 0 || excerpt == "" {
		return s.At
	}
	first := strings.Count(s.Screen[:start], "\n")
	last := first + strings.Count(excerpt, "\n")
	if last >= len(s.LineTimes) {
		return s.At
	}
	at := s.LineTimes[first]
	for _, v := range s.LineTimes[first : last+1] {
		if v.After(at) {
			at = v
		}
	}
	return at
}
