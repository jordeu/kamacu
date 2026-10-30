package session

import (
	"strings"
	"testing"
	"time"
)

func TestAgentExitSource(t *testing.T) {
	for _, source := range []string{"keyboard", "message", "observed", "terminal stop", "external stop"} {
		t.Run(source, func(t *testing.T) {
			s := spawnForTestOpts(t, NewManager(), SpawnOpts{
				Kind: KindAgent, AgentEngine: "custom", Cwd: t.TempDir(), TaskID: 1,
				AgentArgs: []string{"sh", "-c", "printf 'Ready\\n'; read answer"},
			})
			eventually(t, 5*time.Second, "agent ready", func() bool {
				return strings.Contains(s.Observation().Screen, "Ready")
			})
			want := "external"
			var err error
			switch source {
			case "keyboard":
				want = "terminal"
				err = s.WriteInput([]byte("exit\n"))
			case "message":
				err = s.WriteMessage([]byte("exit"), []byte("\n"))
			case "observed":
				obs := s.Observation()
				err = s.WriteObserved(obs.Revision, obs.Input, []byte("exit"), []byte("\n"))
			case "terminal stop":
				want = "terminal"
				s.StopFromTerminal()
			case "external stop":
				// An external stop overrides earlier keyboard activity.
				err = s.WriteInput([]byte("unfinished input"))
				s.Stop()
			}
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-s.Done():
			case <-time.After(5 * time.Second):
				t.Fatal("agent did not exit")
			}
			if got := s.Info().ExitSource; got != want {
				t.Fatalf("exit source = %q, want %q", got, want)
			}
			// Repeated stop requests cannot rewrite the original exit source.
			s.Stop()
			s.StopFromTerminal()
			if got := s.Info().ExitSource; got != want {
				t.Fatalf("exit source changed to %q", got)
			}
		})
	}
}

func TestSpontaneousAgentExitIsExternal(t *testing.T) {
	s := spawnForTestOpts(t, NewManager(), SpawnOpts{
		Kind: KindAgent, AgentEngine: "custom", Cwd: t.TempDir(), TaskID: 1,
		AgentArgs: []string{"true"},
	})
	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("agent did not exit")
	}
	if got := s.Info().ExitSource; got != "external" {
		t.Fatalf("exit source = %q, want external", got)
	}
}
