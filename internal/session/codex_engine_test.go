package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubIdleWait bounds the argv-stub readiness poll.
const stubIdleWait = 5 * time.Second

// writeArgvDumpStub writes an executable bash stub that records its argv (one
// token per line) to argvFile and then idles until killed. Spawned as the
// AgentArgs command (binary at [0]) it lets a test assert the EXACT argv the
// session layer handed the child — the proof surface for the codex `-p
// kamacu` profile-flag prepend. Mirrors testdata/fake-opencode's recorder.
func writeArgvDumpStub(t *testing.T, argvFile string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "argv-dump-stub")
	script := fmt.Sprintf(`#!/usr/bin/env bash
printf '%%s\n' "$@" > %q
sleep 300 & wait
`, argvFile)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write argv-dump stub: %v", err)
	}
	return path
}

// readArgvLines reads an argv dump (one token per line, trailing newline).
func readArgvLines(t *testing.T, argvFile string) []string {
	t.Helper()
	b, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatalf("read argv dump %s: %v", argvFile, err)
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// TestCodexEngineKeepsHeuristicStates proves a codex-engine agent session
// reports one of the claude-style heuristic states (working/idle/waiting),
// NEVER the custom-engine "running" collapse — codex's lifecycle hooks drive
// the same SessionStart/Stop/PermissionRequest receiver transitions. Mirrors
// TestOpencodeEngineKeepsHeuristicStates.
func TestCodexEngineKeepsHeuristicStates(t *testing.T) {
	m := NewManager()
	stub := writeArgvDumpStub(t, filepath.Join(t.TempDir(), "args"))
	s, err := m.Spawn(SpawnOpts{
		Kind:        KindAgent,
		Cwd:         t.TempDir(),
		TaskID:      1,
		AgentEngine: "codex",
		AgentArgs:   []string{stub},
	})
	if err != nil {
		t.Fatalf("Spawn codex agent: %v", err)
	}
	defer s.Stop()

	info := s.Info()
	if info.Engine != "codex" {
		t.Errorf("Engine = %q, want \"codex\"", info.Engine)
	}
	switch info.AgentStatus {
	case "working", "idle", "waiting":
		// ok — heuristic path intact, NOT collapsed to "running"
	default:
		t.Errorf("codex AgentStatus = %q, want one of working/idle/waiting (heuristics intact, not collapsed to running)", info.AgentStatus)
	}
}

// TestCodexSpawnInjectsHookEnv proves the D014 env contract extends to codex:
// a codex spawn injects KAMACU_SESSION_ID / KAMACU_HOOK_TOKEN / KAMACU_HOOK_BASE
// into the child env (the env-gated ~/.codex/kamacu.config.toml hooks reference
// them), with KAMACU_SESSION_ID equal to the session's Info().ID.
func TestCodexSpawnInjectsHookEnv(t *testing.T) {
	m := NewManager()
	m.SetAgentConfig(AgentConfig{
		BaseURL: "http://127.0.0.1:7998",
		Token:   "TOK-CX",
	})
	envFile := filepath.Join(t.TempDir(), "env")
	stub := writeEnvDumpStub(t, envFile)

	s := spawnForTestOpts(t, m, SpawnOpts{
		Kind:        KindAgent,
		Cwd:         t.TempDir(),
		TaskID:      2,
		AgentEngine: "codex",
		AgentArgs:   []string{stub, envFile},
	})

	waitForStubReady(t, envFile)

	if v, ok := envValue(t, envFile, "KAMACU_SESSION_ID"); !ok {
		t.Error("KAMACU_SESSION_ID absent from codex child env (D014 injection missing)")
	} else if v != s.Info().ID {
		t.Errorf("KAMACU_SESSION_ID = %q, want the session id %q", v, s.Info().ID)
	}
	if v, ok := envValue(t, envFile, "KAMACU_HOOK_TOKEN"); !ok {
		t.Error("KAMACU_HOOK_TOKEN absent from codex child env")
	} else if v != "TOK-CX" {
		t.Errorf("KAMACU_HOOK_TOKEN = %q, want \"TOK-CX\"", v)
	}
	if v, ok := envValue(t, envFile, "KAMACU_HOOK_BASE"); !ok {
		t.Error("KAMACU_HOOK_BASE absent from codex child env")
	} else if v != "http://127.0.0.1:7998" {
		t.Errorf("KAMACU_HOOK_BASE = %q, want \"http://127.0.0.1:7998\"", v)
	}
}

// TestCodexSpawnPrependsProfileFlag proves a codex spawn prepends the profile
// selector argv (`-p kamacu`) after the binary and before every handler-provided
// token (the resume append included) — kamacu's hook overlay rides the
// exclusive ~/.codex/kamacu.config.toml profile layer, so the flag must ride
// Spawn itself, engine-gated (custom/opencode argv unchanged).
func TestCodexSpawnPrependsProfileFlag(t *testing.T) {
	m := NewManager()
	argvFile := filepath.Join(t.TempDir(), "argv")
	stub := writeArgvDumpStub(t, argvFile)

	spawnForTestOpts(t, m, SpawnOpts{
		Kind:        KindAgent,
		Cwd:         t.TempDir(),
		TaskID:      3,
		AgentEngine: "codex",
		AgentArgs:   []string{stub, "resume", "0d1a2b3c-1111-2222-3333-444455556666"},
	})

	eventually(t, stubIdleWait, "codex argv stub to record argv", func() bool {
		_, err := os.Stat(argvFile)
		return err == nil
	})

	got := readArgvLines(t, argvFile)
	want := []string{"-p", "kamacu", "--no-alt-screen", "resume", "0d1a2b3c-1111-2222-3333-444455556666"}
	if len(got) != len(want) {
		t.Fatalf("codex spawn argv = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("codex spawn argv[%d] = %q, want %q (full argv %q)", i, got[i], want[i], got)
		}
	}
}

// TestCodexSessionIDAccessors proves the codex session-id capture surface: the
// hook receiver records codex's own uuid (payload session_id) onto the session
// post-spawn, and the api layer's capture poller reads it back. Empty until a
// hook fires.
func TestCodexSessionIDAccessors(t *testing.T) {
	m := NewManager()
	s, err := m.Spawn(SpawnOpts{
		Kind:        KindAgent,
		Cwd:         t.TempDir(),
		TaskID:      4,
		AgentEngine: "codex",
		AgentArgs:   []string{"sleep", "10"},
	})
	if err != nil {
		t.Fatalf("Spawn codex agent: %v", err)
	}
	defer s.Stop()

	if got := s.CodexSessionID(); got != "" {
		t.Errorf("CodexSessionID before any hook = %q, want \"\"", got)
	}
	const id = "0d1a2b3c-aaaa-bbbb-cccc-ddddeeeeffff"
	s.SetCodexSessionID(id)
	if got := s.CodexSessionID(); got != id {
		t.Errorf("CodexSessionID after SetCodexSessionID = %q, want %q", got, id)
	}
	// Idempotent overwrite posture: setting again with the same value is a
	// no-op; a DIFFERENT value must not clobber the first capture (first hook
	// wins — the SessionStart of the kamacu-spawned conversation).
	s.SetCodexSessionID("0d1a2b3c-zzzz-zzzz-zzzz-zzzzzzzzzzzz")
	if got := s.CodexSessionID(); got != id {
		t.Errorf("CodexSessionID after second SetCodexSessionID = %q, want %q (first capture wins)", got, id)
	}
}
