package session

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"
)

func TestTerminalReportsPreserveAgentStatus(t *testing.T) {
	reports := []string{
		"", "\x1b[I", "\x1b[O", "\x1b[?1;2c", "\x1b[>0;276;0c",
		"\x1b[24;80R", "\x1b[?24;80R", "\x1b[0n", "\x1b[?2026;2$y",
		"\x1b[8;24;80t", "\x1b[4;600;800t", "\x1b[6;16;8t",
		"\x1b]10;rgb:ffff/ffff/ffff\x1b\\", "\x1b]11;rgb:0000/0000/0000\x07",
		"\x1b]4;2;rgb:0000/ffff/0000\x1b\\", "\x1bP1$r0m\x1b\\",
		"\x1b[I\x1b[24;80R\x1b[?1;2c",
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	for _, waiting := range []bool{false, true} {
		for _, report := range reports {
			s := &Session{kind: KindAgent, engine: "codex", status: StatusRunning, ptmx: writer, waiting: waiting}
			if err := s.WriteInput([]byte(report)); err != nil {
				t.Fatal(err)
			}
			// Even ignored reports must reach the process unchanged so terminal
			// capability negotiation and cursor queries continue to work.
			got := make([]byte, len(report))
			if _, err := io.ReadFull(reader, got); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, []byte(report)) {
				t.Fatalf("forwarded %q, want %q", got, report)
			}
			if s.waiting != waiting || !s.lastActivity.IsZero() {
				t.Fatalf("report %q changed waiting/activity (waiting=%v)", report, waiting)
			}
		}
	}
}

func TestTerminalUserInputClearsWaiting(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	for _, input := range []string{"y", "\r", "\x1b", "\x03", "\x1b[A", "\x1b[B", "\x1b[200~yes\x1b[201~", "\x1b[24;80Ry"} {
		s := &Session{kind: KindAgent, engine: "codex", status: StatusRunning, ptmx: writer, waiting: true}
		if err := s.WriteInput([]byte(input)); err != nil {
			t.Fatal(err)
		}
		if _, err := io.ReadFull(reader, make([]byte, len(input))); err != nil {
			t.Fatal(err)
		}
		if got := s.Info().AgentStatus; got != "working" {
			t.Fatalf("input %q: status = %q, want working", input, got)
		}
	}
}

func TestCodexWaitingSurvivesAttachAndRedraw(t *testing.T) {
	s := &Session{kind: KindAgent, engine: "codex", status: StatusRunning}
	s.SetWaiting()
	s.ClearWaitingOnAttach()
	// Quiet-time expiration and redraws must not clear a pending permission.
	s.lastActivity = time.Now().Add(-2 * agentQuietThreshold)
	injectOutputForTest(s, []byte("permission prompt redraw"))
	if got := s.Info().AgentStatus; got != "waiting" {
		t.Fatalf("after attach/redraw: status = %q, want waiting", got)
	}
	s.SetIdle()
	if got := s.Info().AgentStatus; got != "idle" {
		t.Fatalf("after turn end: status = %q, want idle", got)
	}
}
