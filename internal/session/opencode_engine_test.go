package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeEnvDumpStub writes an executable bash stub that dumps its full process
// environment to envFile (followed by a STUB_READY sentinel) and then idles
// until killed. Spawned as the AgentArgs command (binary at [0], envFile path
// at [1]) it lets a test inspect the exact env the session layer handed the
// child — the proof surface for the D014 KAMACU_* injection. Mirrors
// writeFakeClaude's shape (sleep 300 & wait keeps the process alive for
// status assertions; STUB_READY removes the read/write race).
func writeEnvDumpStub(t *testing.T, envFile string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "env-dump-stub")
	script := fmt.Sprintf(`#!/usr/bin/env bash
env > %q
echo "STUB_READY" >> %q
sleep 300 & wait
`, envFile, envFile)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write env-dump stub: %v", err)
	}
	return path
}

// envValue reads a dumped env file (KEY=VALUE lines) and returns the value
// for key, plus whether the key was present. Used to assert both presence
// (opencode gets the var) and absence (custom does not).
func envValue(t *testing.T, envFile, key string) (string, bool) {
	t.Helper()
	b, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read env dump %s: %v", envFile, err)
	}
	prefix := key + "="
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix), true
		}
	}
	return "", false
}

// waitForStubReady polls envFile until the stub has finished writing it
// (STUB_READY sentinel) so a subsequent absence assertion is race-free.
func waitForStubReady(t *testing.T, envFile string) {
	t.Helper()
	eventually(t, 5*time.Second, "env-dump stub to finish writing env", func() bool {
		b, err := os.ReadFile(envFile)
		return err == nil && strings.Contains(string(b), "STUB_READY")
	})
}

// TestOpencodeEngineKeepsHeuristicStates proves an opencode-engine agent
// session reports one of the claude-style heuristic states
// (working/idle/waiting), NEVER the custom-engine "running" collapse (D013).
// Mirrors TestClaudeEngineKeepsHeuristicStates. A sleep stub keeps the
// process alive for the assertion; spawn sets lastActivity -> working.
func TestOpencodeEngineKeepsHeuristicStates(t *testing.T) {
	m := NewManager()
	s, err := m.Spawn(SpawnOpts{
		Kind:        KindAgent,
		Cwd:         t.TempDir(),
		TaskID:      1,
		AgentEngine: "opencode",
		AgentArgs:   []string{"sleep", "10"},
	})
	if err != nil {
		t.Fatalf("Spawn opencode agent: %v", err)
	}
	defer s.Stop()

	info := s.Info()
	if info.Engine != "opencode" {
		t.Errorf("Engine = %q, want \"opencode\"", info.Engine)
	}
	switch info.AgentStatus {
	case "working", "idle", "waiting":
		// ok — heuristic path intact, NOT collapsed to "running" (D013)
	default:
		t.Errorf("opencode AgentStatus = %q, want one of working/idle/waiting (heuristics intact, not collapsed to running)", info.AgentStatus)
	}
}

// TestOpencodeSpawnInjectsHookEnv proves the D014 env contract: an opencode
// spawn injects KAMACU_SESSION_ID / KAMACU_HOOK_TOKEN / KAMACU_HOOK_BASE into
// the child process env, with KAMACU_SESSION_ID equal to the session's
// Info().ID (the uuid the on-disk plugin echoes back in its hook POST URL).
func TestOpencodeSpawnInjectsHookEnv(t *testing.T) {
	m := NewManager()
	m.SetAgentConfig(AgentConfig{
		BaseURL: "http://127.0.0.1:7999",
		Token:   "TOK-OC",
	})
	envFile := filepath.Join(t.TempDir(), "env")
	stub := writeEnvDumpStub(t, envFile)

	s := spawnForTestOpts(t, m, SpawnOpts{
		Kind:        KindAgent,
		Cwd:         t.TempDir(),
		TaskID:      2,
		AgentEngine: "opencode",
		AgentArgs:   []string{stub, envFile},
	})

	waitForStubReady(t, envFile)

	if v, ok := envValue(t, envFile, "KAMACU_SESSION_ID"); !ok {
		t.Error("KAMACU_SESSION_ID absent from opencode child env (D014 injection missing)")
	} else if v == "" {
		t.Error("KAMACU_SESSION_ID is empty")
	} else if v != s.Info().ID {
		t.Errorf("KAMACU_SESSION_ID = %q, want the session id %q", v, s.Info().ID)
	}
	if v, ok := envValue(t, envFile, "KAMACU_HOOK_TOKEN"); !ok {
		t.Error("KAMACU_HOOK_TOKEN absent from opencode child env")
	} else if v != "TOK-OC" {
		t.Errorf("KAMACU_HOOK_TOKEN = %q, want \"TOK-OC\"", v)
	}
	if v, ok := envValue(t, envFile, "KAMACU_HOOK_BASE"); !ok {
		t.Error("KAMACU_HOOK_BASE absent from opencode child env")
	} else if v != "http://127.0.0.1:7999" {
		t.Errorf("KAMACU_HOOK_BASE = %q, want \"http://127.0.0.1:7999\"", v)
	}
}

// TestCustomEngineDoesNotGetHookEnv proves the D014 injection is opencode-
// gated: a custom-engine spawn (same stub, same AgentConfig) receives NONE
// of the KAMACU_* vars. This is the R017 regression guard — the custom arm
// is byte-for-byte unchanged.
func TestCustomEngineDoesNotGetHookEnv(t *testing.T) {
	m := NewManager()
	m.SetAgentConfig(AgentConfig{
		BaseURL: "http://127.0.0.1:7999",
		Token:   "TOK-OC",
	})
	envFile := filepath.Join(t.TempDir(), "env")
	stub := writeEnvDumpStub(t, envFile)

	spawnForTestOpts(t, m, SpawnOpts{
		Kind:        KindAgent,
		Cwd:         t.TempDir(),
		TaskID:      3,
		AgentEngine: "custom",
		AgentArgs:   []string{stub, envFile},
	})

	waitForStubReady(t, envFile)

	for _, key := range []string{"KAMACU_SESSION_ID", "KAMACU_HOOK_TOKEN", "KAMACU_HOOK_BASE"} {
		if _, ok := envValue(t, envFile, key); ok {
			t.Errorf("%s present in custom child env — D014 injection must be opencode-gated (custom path unchanged)", key)
		}
	}
}
