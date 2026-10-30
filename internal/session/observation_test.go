package session

import (
	"os"
	"testing"
	"time"

	"kamacu/internal/observation"
)

func TestTerminalActivityAndMonitorStatus(t *testing.T) {
	s := spawnAgentForTest(t, NewManager())
	deadline := time.Now().Add(5 * time.Second)
	for s.Observation().Screen == "" {
		if time.Now().After(deadline) {
			t.Fatal("initial output missing")
		}
		time.Sleep(time.Millisecond)
	}
	if s.Info().AgentStatus != "working" {
		t.Fatal("initial output must mark activity")
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
	if s.Info().AgentStatus != "working" {
		t.Fatal("input did not immediately mark activity")
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

func TestTerminalAgentStatus(t *testing.T) {
	now := time.Now()
	recent := now.Add(-time.Second)
	old := now.Add(-time.Minute)
	for _, tc := range []struct {
		name          string
		output, input time.Time
		state         string
		observed      time.Time
		want          string
	}{
		{name: "quiet new terminal", want: "idle"},
		{name: "output before any inference", output: recent, want: "working"},
		{name: "input before any inference", input: recent, want: "working"},
		{name: "quiet is idle, not completion", output: old, want: "idle"},
		{name: "failed inference with output", output: recent, state: "unknown", want: "working"},
		{name: "failed inference without output", output: old, state: "unknown", want: "idle"},
		{name: "fresh question", output: recent, state: "waiting", observed: now, want: "waiting"},
		{name: "question stays waiting when quiet", output: old, state: "waiting", observed: old, want: "waiting"},
		{name: "input invalidates question", output: old, input: recent, state: "waiting", observed: old, want: "working"},
		{name: "progress invalidates question", output: recent, state: "waiting", observed: old, want: "working"},
		{name: "fresh completion", output: recent, state: "idle", observed: now, want: "idle"},
		{name: "progress invalidates completion", output: recent, state: "idle", observed: old, want: "working"},
		{name: "fresh error", output: recent, state: "error", observed: now, want: "error"},
		{name: "model working cannot keep silent terminal busy", output: old, state: "working", observed: now, want: "idle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := terminalAgentStatus(observation.Snapshot{OutputAt: tc.output, InputAt: tc.input}, MonitorStatus{State: tc.state, ObservedAt: tc.observed}, now)
			if got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}
