package session

import (
	"testing"
	"time"
)

// TestCustomEngineStatusRunningExited proves a custom-engine agent session
// reports ONLY "running" (while alive) and "exited" (when dead), never the
// claude-hook-driven working/waiting/idle heuristics (D-M001-2). Uses `sleep`
// as the custom agent command so the process is reliably alive for the
// running-state assertion.
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

	// While alive: a custom agent reports "running", never working/idle/waiting.
	info := s.Info()
	if info.Engine != "custom" {
		t.Errorf("Engine = %q, want \"custom\"", info.Engine)
	}
	if info.AgentStatus != "running" {
		t.Errorf("alive AgentStatus = %q, want \"running\" (custom = running/exited only)", info.AgentStatus)
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
	case "working", "idle", "waiting":
		// ok -- heuristic path intact, NOT collapsed to "running"
	default:
		t.Errorf("claude AgentStatus = %q, want one of working/idle/waiting (heuristics intact, not collapsed to running)", info.AgentStatus)
	}
}
