package session

import (
	"testing"
	"time"
)

// Custom agents use the same terminal-activity status as built-in engines.
// A quiet sleep process is idle but still has a live terminal.
func TestCustomEngineStatusRunningExited(t *testing.T) {
	m := NewManager()
	s, err := m.Spawn(SpawnOpts{
		Kind:        KindAgent,
		Cwd:         t.TempDir(),
		TaskID:      1,
		AgentEngine: "custom",
		AgentArgs:   []string{"sleep", "10"},
	})
	if err != nil {
		t.Fatalf("Spawn custom agent: %v", err)
	}
	defer s.Stop()

	// Lifetime and terminal activity are independent.
	info := s.Info()
	if info.Engine != "custom" {
		t.Errorf("Engine = %q, want \"custom\"", info.Engine)
	}
	if info.AgentStatus != "idle" && info.AgentStatus != "working" {
		t.Errorf("alive AgentStatus = %q, want idle or working", info.AgentStatus)
	}
	if info.Status != StatusRunning {
		t.Errorf("alive Status = %q, want %q", info.Status, StatusRunning)
	}
}

// TestCustomEngineStatusExited proves the custom-agent session reports "exited"
// once the underlying process ends.
func TestCustomEngineStatusExited(t *testing.T) {
	m := NewManager()
	s, err := m.Spawn(SpawnOpts{
		Kind:        KindAgent,
		Cwd:         t.TempDir(),
		TaskID:      2,
		AgentEngine: "custom",
		AgentArgs:   []string{"true"}, // exits 0 immediately
	})
	if err != nil {
		t.Fatalf("Spawn custom agent: %v", err)
	}

	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("custom agent did not exit within 5s")
	}
	info := s.Info()
	if info.AgentStatus != "exited" {
		t.Errorf("exited AgentStatus = %q, want \"exited\"", info.AgentStatus)
	}
	if info.Status != StatusExited {
		t.Errorf("exited Status = %q, want %q", info.Status, StatusExited)
	}
}

// TestClaudeEngineKeepsHeuristicStates proves the claude engine (and "" back-
// compat) still uses the full agentStatusLocked heuristic states, NOT the
// custom "running" collapse. Regression guard for the engine branch. Which
// heuristic state (working/idle/waiting) shows depends on activity timing, so
// assert membership in the set rather than a single value.
func TestClaudeEngineKeepsHeuristicStates(t *testing.T) {
	m := NewManager()
	s, err := m.Spawn(SpawnOpts{
		Kind:        KindAgent,
		Cwd:         t.TempDir(),
		TaskID:      3,
		AgentEngine: "claude",
	})
	if err != nil {
		t.Skipf("claude binary not available, skipping: %v", err)
	}
	defer s.Stop()

	info := s.Info()
	if info.Engine != "claude" {
		t.Errorf("Engine = %q, want \"claude\"", info.Engine)
	}
	switch info.AgentStatus {
	case "idle", "working":
		// ok -- heuristic path intact, NOT collapsed to "running"
	default:
		t.Errorf("claude AgentStatus = %q, want idle or working from terminal activity", info.AgentStatus)
	}
}
