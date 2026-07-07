---
estimated_steps: 20
estimated_files: 3
skills_used: []
---

# T02: opencode spawn env injection (D014) + agentStatusLocked heuristic gate

WHY: opencode runs in a PTY like claude (D013) but its status hooks come from an on-disk plugin, not a --settings overlay. That plugin needs the Kamacu session id + hook token + base URL via env (D014) to curl back. And agentStatusLocked must treat opencode as a full-heuristics engine (working/waiting/idle), not collapse it to 'running' like custom. These are the ONLY two Go runtime changes (D013: "only agentStatusLocked() gains opencode").

DO:
1. internal/session/manager.go — the engine switch is inside the `if kind == KindAgent` block (~L156-215). The existing branch `if opts.AgentEngine != "" && opts.AgentEngine != "claude" { ...custom... } else { ...claude... }` already routes opencode into the custom arm (sessions.go calls renderAgentCommand for any non-claude engine, yielding AgentArgs=["opencode"]). opencode therefore REUSES the LookPath + exec.Command + cmd.Dir path. The ONLY delta: inject the three KAMACU_* env vars when engine=="opencode". In the custom arm where cmd.Env is set (~L196), make the env list conditional:
       envExtra := []string{"TERM=xterm-256color", "COLORTERM=truecolor"}
       if opts.AgentEngine == "opencode" {
           envExtra = append(envExtra,
               "KAMACU_SESSION_ID="+id,         // Kamacu session uuid, in scope since id:=uuid.NewString() ~L153
               "KAMACU_HOOK_TOKEN="+cfg.Token,   // cfg:=m.agentCfg read under mu at top of KindAgent block ~L165
               "KAMACU_HOOK_BASE="+cfg.BaseURL)  // D014 env contract
       }
       cmd.Env = append(os.Environ(), envExtra...)
   Confirm cfg.Token/cfg.BaseURL are the AgentConfig fields (internal/session/agent.go). Update the arm's comment to note opencode reuses the custom command-render arm plus the D014 env contract. The CLAUDE arm cmd.Env is byte-for-byte UNCHANGED.
2. internal/session/session.go agentStatusLocked() (~L312-330) — widen the custom-collapse gate (~L322) from `if s.engine != "" && s.engine != "claude" {` to `if s.engine != "" && s.engine != "claude" && s.engine != "opencode" {` so opencode reaches the working/waiting/idle heuristic path. Update the adjacent comment (D013). Do NOT touch SetWaiting/SetIdle/MarkHooksAlive/noteAgentOutputLocked — the status machine is unchanged.
3. New file internal/session/opencode_engine_test.go:
   a. TestOpencodeEngineKeepsHeuristicStates — Spawn{AgentEngine:"opencode", AgentArgs:[]string{"sleep","10"}, Cwd:t.TempDir(), TaskID:1}; assert info.Engine=="opencode" and info.AgentStatus is one of working/idle/waiting, NEVER "running". Mirror TestClaudeEngineKeepsHeuristicStates.
   b. TestOpencodeSpawnInjectsHookEnv — write a bash stub (mirror writeFakeClaude in agent_test.go) that runs `env > <envfile>` then `sleep 300 & wait`; SetAgentConfig(AgentConfig{BaseURL:"http://127.0.0.1:7999", Token:"TOK-OC"}); Spawn{AgentEngine:"opencode", AgentArgs:[]string{stub}}; read envfile; assert it contains KAMACU_SESSION_ID=<nonempty>, KAMACU_HOOK_TOKEN=TOK-OC, KAMACU_HOOK_BASE=http://127.0.0.1:7999; and assert the spawned sess.Info().ID == that KAMACU_SESSION_ID value.
   c. TestCustomEngineDoesNotGetHookEnv — same stub with AgentEngine:"custom"; assert envfile does NOT contain KAMACU_SESSION_ID (proves the injection is opencode-gated, custom unchanged).

CONSTRAINTS: claude arm unchanged; hooks.go untouched; custom arm keeps exact LookPath/exec.Command behavior (only the env list gains conditional vars); NO --settings overlay for opencode (plugin is on disk).

DONE WHEN: opencode sessions report a heuristic status (not 'running'), the opencode process env carries all three KAMACU_* vars with the session id matching Info().ID, custom processes get none of them, and the existing fake-claude + custom-engine tests pass unchanged (R016/R017 regression). Note: a sleep-based stub keeps the process alive for status assertions; never depend on the real opencode binary in CI.

Skills: go, pty, testing.

## Inputs

- `internal/session/manager.go`
- `internal/session/session.go`
- `internal/session/agent.go`
- `internal/session/agent_engine_test.go`
- `internal/session/agent_test.go`

## Expected Output

- `internal/session/opencode_engine_test.go`

## Verification

go test ./internal/session/ -run Opencode -count=1 && go test ./internal/session/ -run TestCustomEngine -count=1 && go test ./internal/session/ -run TestClaudeEngine -count=1
