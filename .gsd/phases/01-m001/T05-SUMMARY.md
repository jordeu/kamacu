---
id: T05
parent: S01
milestone: M001
key_files:
  - internal/session/manager.go
  - internal/api/sessions.go
  - internal/api/agents_crud.go
  - internal/api/agents_crud_test.go
key_decisions:
  - Engine branch keyed on SpawnOpts.AgentEngine: 'claude' or '' (back-compat) -> existing argv unchanged byte-for-byte; any other value -> custom path (LookPath + exec.Command on AgentArgs). Fake-claude regression suite is the gate and passes unchanged
  - Session stays a leaf: the API handler renders the template (renderAgentCommand) and passes already-tokenized AgentArgs; session does only LookPath + exec.Command. No session->settings import (avoids layering inversion)
  - Tokenize-then-substitute-per-token order: a {{worktree}} value with spaces (e.g. /home/user/my projects/wt) stays a single token rather than re-splitting. The naive replace-then-tokenize broke this; the unit test caught it
  - {{session_id}} is honored as an informational stable uuid for users who wire resume-style flags, even though custom agents don't resume (D-M001-2). Unknown placeholders ({{foo}}) are left literal — degrade-don't-break at the config layer
duration: 
verification_result: passed
completed_at: 2026-07-06T16:02:49.008Z
blocker_discovered: false
---

# T05: Spawn engine now branches on agent.engine: claude path byte-for-byte unchanged (fake-claude tests green = the milestone's key risk retired); custom path renders the command template (tokenize-then-substitute per token, fixing a spaces-in-path bug) in the worktree PTY.

**Spawn engine now branches on agent.engine: claude path byte-for-byte unchanged (fake-claude tests green = the milestone's key risk retired); custom path renders the command template (tokenize-then-substitute per token, fixing a spaces-in-path bug) in the worktree PTY.**

## What Happened

Refactored the KindAgent spawn block in internal/session/manager.go to branch on a new SpawnOpts.AgentEngine field. The CLAUDE path (engine=="claude" or "" back-compat) is byte-for-byte unchanged: same session-id/--resume fork, same --settings hook overlay, same ExtraArgs append, same env. The CUSTOM path (any other engine value) does exec.LookPath(opts.AgentArgs[0]) + exec.Command(bin, opts.AgentArgs[1:]...), cmd.Dir=worktree, same TERM/COLORTERM env posture. No hook overlay, no BEL scanning, no resume. Added SpawnOpts.AgentEngine + SpawnOpts.AgentArgs fields.

The API handler (internal/api/sessions.go) resolves the project's agent alongside the existing worktree_path read (a JOIN tasks->projects->agents), hoists agentEngine/agentCommand to the spawn-handler scope, and for a custom agent calls renderAgentCommand to produce the split argv. renderAgentCommand lives in agents_crud.go (api package, so session stays a leaf with no settings import) and reuses settings.Tokenize for shell-splitting (no new deps, D007).

Hit one real bug caught by the unit test: the initial renderAgentCommand did string-replace-then-tokenize, which broke a {{worktree}} value containing spaces (the path re-split into multiple tokens). Rewrote to tokenize-first-then-substitute-per-token so a substituted value with spaces stays a single token -- both --cwd={{worktree}} and a bare {{worktree}} now survive intact. 7 unit cases cover bare command, flags, quoted args, worktree placeholder (with spaces), session_id placeholder, unknown placeholder (left literal), and empty template.

The fake-claude regression suite (TestAgentLifecycle -- full spawn + hooks + resume lifecycle) passes UNCHANGED, proving the claude argv is byte-for-byte preserved. This retires the milestone's key risk.

## Verification

go test ./internal/api/... -run TestAgentLifecycle -> PASS (claude path regression gate, 5.191s). go test ./internal/api/... ./internal/session/... -> both ok (89s/5.4s). TestRenderAgentCommand 7/7 cases (bare, flags, quoted, worktree-with-spaces, session_id, unknown-literal, empty). go vet ./internal/api/... ./internal/session/... ./cmd/kamacu/... clean. go build ./cmd/kamacu/... ok.

## Verification Evidence

| # | Command | Exit Code | Verdict | Duration |
|---|---------|-----------|---------|----------|
| 1 | `go test ./internal/api/... -run TestAgentLifecycle` | 0 | ✅ pass (claude regression gate) | 5191ms |
| 2 | `go test ./internal/api/... ./internal/session/...` | 0 | ✅ pass | 89142ms |
| 3 | `go test ./internal/api/... -run TestRenderAgentCommand -v` | 0 | ✅ pass | 5000ms |
| 4 | `go vet ./internal/api/... ./internal/session/... ./cmd/kamacu/...` | 0 | ✅ pass | 3000ms |
| 5 | `go build ./cmd/kamacu/...` | 0 | ✅ pass | 3000ms |

## Deviations

Two design refinements from the plan, both improving correctness: (1) The custom command is rendered (template substitute + tokenize) at the API layer, not in session — keeps session a pure leaf with no settings import; SpawnOpts carries already-split AgentArgs []string. (2) renderAgentCommand was initially string-replace-then-tokenize, which broke worktree paths containing spaces (they'd re-split into multiple tokens); rewritten to tokenize-first-then-substitute-per-token so a substituted value with spaces stays one token. Caught by the unit test, not at a live gate.

## Known Issues

None.

## Files Created/Modified

- `internal/session/manager.go`
- `internal/api/sessions.go`
- `internal/api/agents_crud.go`
- `internal/api/agents_crud_test.go`
