package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// writeFakeClaude writes an executable stub mimicking the verified v2.1.170
// behaviors the engine relies on (bracketed paste enable at startup, TERM
// trap -> exit 143), records its argv one-per-line to argsFile, then idles
// until stopped. Spawned via AgentConfig.ClaudeBin so tests never touch the
// real claude binary.
func writeFakeClaude(t *testing.T, argsFile string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-claude")
	script := fmt.Sprintf(`#!/usr/bin/env bash
printf '\x1b[?2004h'
echo "fake claude ready"
printf '%%s\n' "$@" > %q
trap 'exit 143' TERM
sleep 300 & wait
`, argsFile)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude stub: %v", err)
	}
	return path
}

// testAgentConfig returns the AgentConfig used across agent tests, pointing
// ClaudeBin at the given stub.
func testAgentConfig(stub string) AgentConfig {
	return AgentConfig{
		BaseURL:   "http://127.0.0.1:7333",
		Token:     "TOK",
		ClaudeBin: stub,
	}
}

// TestOverlayJSONShape pins buildOverlayJSON to the empirically verified
// v2.1.170 settings-overlay shape (04-RESEARCH.md Pattern 2 + SessionStart
// canary).
func TestOverlayJSONShape(t *testing.T) {
	raw := buildOverlayJSON("http://127.0.0.1:7333", "TOK", "SESSID")

	var overlay struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
				Timeout int    `json:"timeout"`
				Async   bool   `json:"async"`
			} `json:"hooks"`
		} `json:"hooks"`
		PreferredNotifChannel string `json:"preferredNotifChannel"`
	}
	if err := json.Unmarshal([]byte(raw), &overlay); err != nil {
		t.Fatalf("overlay does not unmarshal as JSON: %v\nraw: %s", err, raw)
	}

	if overlay.PreferredNotifChannel != "terminal_bell" {
		t.Errorf("preferredNotifChannel = %q, want %q", overlay.PreferredNotifChannel, "terminal_bell")
	}

	for _, event := range []string{"Notification", "Stop", "SessionStart"} {
		entries := overlay.Hooks[event]
		if len(entries) != 1 {
			t.Fatalf("hooks[%s] has %d entries, want 1", event, len(entries))
		}
		hooks := entries[0].Hooks
		if len(hooks) != 1 {
			t.Fatalf("hooks[%s][0].hooks has %d entries, want 1", event, len(hooks))
		}
		h := hooks[0]
		if h.Type != "command" {
			t.Errorf("%s hook type = %q, want %q", event, h.Type, "command")
		}
		if h.Timeout != 5 {
			t.Errorf("%s hook timeout = %d, want 5", event, h.Timeout)
		}
		if !h.Async {
			t.Errorf("%s hook async = false, want true", event)
		}
		if !strings.Contains(h.Command, "http://127.0.0.1:7333/api/hooks/sessions/SESSID") {
			t.Errorf("%s hook command missing receiver URL: %q", event, h.Command)
		}
		if !strings.Contains(h.Command, "X-Kangent-Token: TOK") {
			t.Errorf("%s hook command missing token header: %q", event, h.Command)
		}
	}

	if got := overlay.Hooks["Notification"][0].Matcher; got != "permission_prompt|elicitation_dialog" {
		t.Errorf("Notification matcher = %q, want %q", got, "permission_prompt|elicitation_dialog")
	}

	// Stop and SessionStart must have NO matcher key at all (Stop has no
	// matchers in v2.1.170 — research finding 15), not an empty one.
	var generic map[string]any
	if err := json.Unmarshal([]byte(raw), &generic); err != nil {
		t.Fatalf("generic unmarshal: %v", err)
	}
	hooksAny := generic["hooks"].(map[string]any)
	for _, event := range []string{"Stop", "SessionStart"} {
		entry := hooksAny[event].([]any)[0].(map[string]any)
		if _, ok := entry["matcher"]; ok {
			t.Errorf("%s entry carries a matcher field; must be absent", event)
		}
	}

	// idle_prompt is D-46's idle, not waiting — it must never appear (D-46).
	if strings.Contains(raw, "idle_prompt") {
		t.Errorf("overlay contains idle_prompt; matcher must exclude it:\n%s", raw)
	}
}

// TestSpawnAgentRequiresCwd: agents always run in a task worktree — an empty
// Cwd must fail cleanly before any PTY allocation or session registration.
func TestSpawnAgentRequiresCwd(t *testing.T) {
	m := NewManager()
	m.SetAgentConfig(testAgentConfig("/bin/true"))

	before := len(m.List())
	_, err := m.Spawn(SpawnOpts{Kind: KindAgent, TaskID: 1})
	if err == nil {
		t.Fatal("Spawn(Kind=agent, Cwd=\"\") = nil error, want failure")
	}
	if after := len(m.List()); after != before {
		t.Errorf("List len changed %d -> %d: failed agent spawn must not register a session", before, after)
	}
}

// TestSpawnAgentArgvAndIdentity drives a full agent spawn against the
// fake-claude stub and verifies the exact argv contract:
// --session-id <uuid> --settings <inline overlay JSON>.
func TestSpawnAgentArgvAndIdentity(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	stub := writeFakeClaude(t, argsFile)

	m := NewManager()
	m.SetAgentConfig(testAgentConfig(stub))
	s := spawnForTestOpts(t, m, SpawnOpts{Kind: KindAgent, Cwd: t.TempDir(), TaskID: 7})

	info := s.Info()
	if info.Label != "Agent" {
		t.Errorf("Label = %q, want %q", info.Label, "Agent")
	}
	if info.Kind != KindAgent {
		t.Errorf("Kind = %q, want %q", info.Kind, KindAgent)
	}
	if info.TaskID != 7 {
		t.Errorf("TaskID = %d, want 7", info.TaskID)
	}

	eventually(t, 5*time.Second, "stub to record argv", func() bool {
		b, err := os.ReadFile(argsFile)
		return err == nil && len(b) > 0
	})
	b, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read argv file: %v", err)
	}
	args := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(args) != 4 {
		t.Fatalf("argv = %q, want exactly 4 args (--session-id <uuid> --settings <json>)", args)
	}
	if args[0] != "--session-id" {
		t.Errorf("argv[0] = %q, want --session-id", args[0])
	}
	if _, err := uuid.Parse(args[1]); err != nil {
		t.Errorf("argv[1] = %q does not parse as a uuid: %v", args[1], err)
	}
	if args[2] != "--settings" {
		t.Errorf("argv[2] = %q, want --settings", args[2])
	}
	var overlayAny map[string]any
	if err := json.Unmarshal([]byte(args[3]), &overlayAny); err != nil {
		t.Errorf("argv[3] is not valid JSON: %v\nraw: %s", err, args[3])
	}

	// The claude session id is captured for Phase 5 --resume.
	if got := s.ClaudeSessionID(); got != args[1] {
		t.Errorf("ClaudeSessionID() = %q, want the --session-id value %q", got, args[1])
	}
	// The overlay's hook URL embeds the KANGENT session id, not claude's.
	if !strings.Contains(args[3], "/api/hooks/sessions/"+info.ID) {
		t.Errorf("overlay does not embed the kangent session id %s:\n%s", info.ID, args[3])
	}
}

// agentArgv spawns an agent against the argv-recording stub and returns the
// recorded argv (split one-per-line). It polls for the args file (the stub
// writes it after startup output) up to 2s, then leaves the session running —
// the caller stops it.
func agentArgv(t *testing.T, opts SpawnOpts) (*Session, []string) {
	t.Helper()
	argsFile := filepath.Join(t.TempDir(), "args")
	stub := writeFakeClaude(t, argsFile)

	m := NewManager()
	m.SetAgentConfig(testAgentConfig(stub))
	s := spawnForTestOpts(t, m, opts)

	eventually(t, 2*time.Second, "stub to record argv", func() bool {
		b, err := os.ReadFile(argsFile)
		return err == nil && len(b) > 0
	})
	b, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read argv file: %v", err)
	}
	return s, strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

// TestSpawnAgentResumeArgv: with a non-empty ResumeSessionID the engine spawns
// `claude --resume <that uuid> --settings <overlay>` — never --session-id,
// never --fork-session — and reports the resume uuid as the claude session id.
func TestSpawnAgentResumeArgv(t *testing.T) {
	const resumeID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeee0001"
	s, args := agentArgv(t, SpawnOpts{Kind: KindAgent, Cwd: t.TempDir(), TaskID: 1, ResumeSessionID: resumeID})

	if len(args) != 4 {
		t.Fatalf("argv = %q, want exactly 4 args (--resume <uuid> --settings <json>)", args)
	}
	if args[0] != "--resume" {
		t.Errorf("argv[0] = %q, want --resume", args[0])
	}
	if args[1] != resumeID {
		t.Errorf("argv[1] = %q, want the resume uuid %q", args[1], resumeID)
	}
	if args[2] != "--settings" {
		t.Errorf("argv[2] = %q, want --settings", args[2])
	}
	for _, a := range args {
		if a == "--session-id" {
			t.Errorf("argv contains --session-id on a resume spawn: %q", args)
		}
		if a == "--fork-session" {
			t.Errorf("argv contains --fork-session (forks a new id, orphaning the task): %q", args)
		}
	}
	if got := s.ClaudeSessionID(); got != resumeID {
		t.Errorf("ClaudeSessionID() = %q, want the resume uuid %q", got, resumeID)
	}
}

// TestSpawnAgentFreshArgv: with an empty ResumeSessionID the engine keeps the
// existing fresh-spawn behavior — `--session-id <fresh uuid> --settings ...`.
func TestSpawnAgentFreshArgv(t *testing.T) {
	_, args := agentArgv(t, SpawnOpts{Kind: KindAgent, Cwd: t.TempDir(), TaskID: 1})

	if len(args) != 4 {
		t.Fatalf("argv = %q, want exactly 4 args (--session-id <uuid> --settings <json>)", args)
	}
	if args[0] != "--session-id" {
		t.Errorf("argv[0] = %q, want --session-id", args[0])
	}
	if _, err := uuid.Parse(args[1]); err != nil {
		t.Errorf("argv[1] = %q does not parse as a uuid: %v", args[1], err)
	}
	if args[2] != "--settings" {
		t.Errorf("argv[2] = %q, want --settings", args[2])
	}
	for _, a := range args {
		if a == "--resume" {
			t.Errorf("fresh spawn must not carry --resume: %q", args)
		}
	}
}

// TestSpawnResumeRequiresAgent: a ResumeSessionID on a bash session is an
// error — resume is agent-only — and registers no session.
func TestSpawnResumeRequiresAgent(t *testing.T) {
	m := NewManager()

	before := len(m.List())
	_, err := m.Spawn(SpawnOpts{Kind: KindBash, Cwd: t.TempDir(), TaskID: 1, ResumeSessionID: "x"})
	if err == nil {
		t.Fatal("Spawn(Kind=bash, ResumeSessionID set) = nil error, want failure")
	}
	if after := len(m.List()); after != before {
		t.Errorf("List len changed %d -> %d: rejected resume must not register a session", before, after)
	}
}

// TestSpawnKindDefaultsToBash: a zero-value Kind is a bash session,
// byte-for-byte the Phase 2/3 behavior (label family, no claude identity).
func TestSpawnKindDefaultsToBash(t *testing.T) {
	m := NewManager()
	s := spawnForTestOpts(t, m, SpawnOpts{TaskID: 3})

	info := s.Info()
	if info.Label != "Bash 1" {
		t.Errorf("Label = %q, want %q", info.Label, "Bash 1")
	}
	if info.Kind != KindBash {
		t.Errorf("Kind = %q, want %q", info.Kind, KindBash)
	}
	if got := s.ClaudeSessionID(); got != "" {
		t.Errorf("ClaudeSessionID() = %q for a bash session, want empty", got)
	}
}
