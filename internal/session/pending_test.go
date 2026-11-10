package session

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"kamacu/internal/observation"
)

func TestPendingPromptProtectsDraftsAndDialogs(t *testing.T) {
	for _, tc := range []struct {
		engine, screen, state string
		resumed, allowed      bool
	}{
		{"claude", "❯ ", "idle", false, true},
		{"claude", "❯ ", "waiting", false, true},
		{"claude", "❯ unfinished user draft", "idle", false, false},
		{"claude", choiceScreen, "waiting", false, false},
		{"claude", "❯ \nDo you trust this workspace?\nEnter to confirm", "idle", false, false},
		{"codex", "› user draft", "idle", false, false},
		{"codex", "› Ask Codex to do anything", "idle", false, true},
		{"codex", "◦ Working (2s • esc to interrupt)\n› Ask Codex to do anything", "idle", false, false},
		{"custom", "Working (esc to stop)\n❯ ", "idle", false, false},
		{"custom", "❯ ", "working", false, false},
		{"custom", "❯ ", "", true, true},
		{"custom", "❯ ", "working", true, false},
	} {
		t.Run(tc.engine+tc.screen+tc.state, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "input")
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			obs := observation.New(100, 30)
			obs.Feed([]byte(strings.ReplaceAll(tc.screen, "\n", "\r\n")))
			s := &Session{status: StatusRunning, kind: KindAgent, engine: tc.engine, ptmx: f, observer: obs}
			if tc.state != "" {
				s.SetMonitorStatus(MonitorStatus{State: tc.state, ObservedAt: time.Now()})
			}
			before := s.Observation()
			err = s.WritePendingPrompt(before.Revision, before.Input, []byte("prompt"), []byte("ENTER"), tc.resumed)
			data, _ := os.ReadFile(f.Name())
			if tc.allowed {
				if err != nil || string(data) != "promptENTER" {
					t.Fatal(string(data), err)
				}
			} else {
				if !errors.Is(err, ErrObservedChanged) || len(data) != 0 {
					t.Fatal("unsafe input was submitted", string(data), err)
				}
			}
		})
	}
}
func TestPendingCompletionDoesNotTrustQuietWorkingTerminal(t *testing.T) {
	now := time.Now()
	obs := observation.Snapshot{Screen: "Running tests; waiting for a tool", InputAt: now.Add(-40 * time.Second), OutputAt: now.Add(-30 * time.Second)}
	if terminalAgentStatus(obs, MonitorStatus{}, now) != "idle" {
		t.Fatal("fixture must reproduce the quiet-idle fallback")
	}
	if pendingCheckFinished(obs, MonitorStatus{}, "custom", now) {
		t.Fatal("silence counted as completion")
	}
	monitor := MonitorStatus{State: "working", ObservedAt: now}
	if pendingCheckFinished(obs, monitor, "custom", now) {
		t.Fatal("working decision counted as completion")
	}
	monitor.State = "idle"
	if !pendingCheckFinished(obs, monitor, "custom", now) {
		t.Fatal("fresh idle decision not recognized")
	}
	obs.InputAt = now.Add(time.Second)
	if pendingCheckFinished(obs, monitor, "custom", now.Add(time.Second)) {
		t.Fatal("stale completion survived new input")
	}
	monitor.State = "waiting"
	monitor.ObservedAt = now.Add(time.Second)
	if !pendingCheckFinished(obs, monitor, "custom", now.Add(time.Second)) {
		t.Fatal("fresh question did not release the slot")
	}
}

func TestPendingCompletionRequiresOutputAfterSubmission(t *testing.T) {
	now := time.Now()
	obs := observation.Snapshot{Screen: "❯", OutputAt: now.Add(-30 * time.Second), InputAt: now.Add(-20 * time.Second)}
	if pendingCheckFinished(obs, MonitorStatus{}, "claude", now) {
		t.Fatal("unchanged prompt counted as a response")
	}
	obs.OutputAt = now.Add(-2 * time.Second)
	if !pendingCheckFinished(obs, MonitorStatus{}, "claude", now) {
		t.Fatal("returned empty prompt not recognized")
	}
	obs.Screen = "Working (esc to interrupt)\n❯"
	if pendingCheckFinished(obs, MonitorStatus{}, "claude", now) {
		t.Fatal("quiet busy screen counted as completion")
	}
	obs.Screen = "❯"
	obs.OutputAt = now
	if pendingCheckFinished(obs, MonitorStatus{}, "claude", now) {
		t.Fatal("transient repaint counted as completion")
	}
}
