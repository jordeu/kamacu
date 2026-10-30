package session

import (
	"os"
	"testing"
	"time"
)

func TestMonitorOwnsAgentStatus(t *testing.T) {
	s := spawnAgentForTest(t, NewManager())
	if s.Info().AgentStatus != "unknown" {
		t.Fatal("initial state must be uncertain")
	}
	s.SetMonitorStatus(MonitorStatus{State: "waiting", Health: "ready", ObservedAt: time.Now()})
	s.SetIdle()
	s.ClearWaitingOnAttach()
	s.MarkHooksAlive()
	if s.Info().AgentStatus != "waiting" {
		t.Fatal("legacy hook or attach changed observed status")
	}
	before := s.Observation()
	if err := s.WriteInput([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if s.Observation().Input <= before.Input {
		t.Fatal("input did not invalidate observation")
	}
	if s.Info().AgentStatus != "waiting" {
		t.Fatal("keypress inferred work")
	}
	if err := s.WriteObserved(before.Revision, before.Input, []byte("yes"), []byte("\r")); err == nil {
		t.Fatal("stale reply written")
	}
}

func TestMessageWriteDoesNotInterleaveWithKeyboard(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "terminal")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s := &Session{status: StatusRunning, kind: KindBash, ptmx: f}
	written := make(chan error, 1)
	go func() { written <- s.WriteMessage([]byte("paste"), []byte("ENTER")) }()
	deadline := time.Now().Add(time.Second)
	for {
		b, _ := os.ReadFile(f.Name())
		if len(b) >= len("paste") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("paste never started")
		}
		time.Sleep(time.Millisecond)
	}
	if err = s.WriteInput([]byte("typed")); err != nil {
		t.Fatal(err)
	}
	if err = <-written; err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(f.Name())
	if string(b) != "pasteENTERtyped" {
		t.Fatalf("interleaved input: %q", b)
	}
}
