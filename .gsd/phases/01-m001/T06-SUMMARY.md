---
id: T06
parent: S01
milestone: M001
key_files:
  - internal/session/session.go
  - internal/session/manager.go
  - internal/session/agent_engine_test.go
  - web/src/api/types.ts
  - web/src/api/agents.ts
  - web/src/components/StatusDot.tsx
  - web/src/components/board/PRCard.tsx
key_decisions:
  - Custom-engine agents report only running/exited via an engine branch in agentStatusLocked -- the working/waiting/idle heuristics are claude-hook-driven and explicitly rejected for custom TUIs we don't understand (D-M001-2)
  - The engine field lives on the Session struct (set from SpawnOpts.AgentEngine) and is exposed on Info -- the status handler and UI can distinguish claude vs custom, and the branch is localized to the session layer
  - agent_id added to the frontend Project type and "running" added to the AgentStatusEntry status union (custom agents emit it); StatusDot and PRCard gained a running case (blue) so exhaustiveness holds. Full agents CRUD client + management UI is S02
duration: 
verification_result: passed
completed_at: 2026-07-06T16:09:54.795Z
blocker_discovered: false
---

# T06: Custom agents report running/exited only (engine branch in agentStatusLocked); full Go suite + frontend gate green; claude heuristics + fake-claude regression intact.

**Custom agents report running/exited only (engine branch in agentStatusLocked); full Go suite + frontend gate green; claude heuristics + fake-claude regression intact.**

## What Happened

Custom-engine status + full-suite verify. Added an `engine` field to the Session struct (set from SpawnOpts.AgentEngine in Spawn) and to Info. Branched agentStatusLocked: a custom engine (not "claude"/"") returns "running" while alive and "exited" when dead, skipping the working/waiting/idle heuristics (those are claude-hook-driven and explicitly rejected for custom agents per D-M001-2). The claude path keeps the full heuristic states. Exposed Engine on Info so the status handler/UI can distinguish.

Added 3 session tests: custom running (alive = "running"), custom exited (process ends = "exited"), claude-keeps-heuristics (regression guard: claude stays in the working/idle/waiting set, not collapsed to "running").

Frontend type additions to keep the gate green and represent the new states: agent_id on the Project interface (mirrors workspace_id); "running" added to the AgentStatusEntry status union (custom agents emit it); a "running" case (blue) added to StatusDot.dotMeta and PRCard.agentRail so the exhaustive switches compile. The full Agents CRUD client + Settings/Project-selector UI is S02, not this task.

FULL SUITE VERIFICATION (the slice's verify-close): go test ./... -> all 13 packages green (api 89.5s, ws 61s, session 8.5s, store, etc.); go vet ./... clean; cd web && tsc -b && vite build both exit 0. The fake-claude regression suite (TestAgentLifecycle) passes unchanged -- claude argv byte-for-byte preserved across the whole slice.

## Verification

go test ./... -> all 13 packages ok (cmd, api, diff, github, migrate, quota, reaper, session, settings, store, tmux, worktree, ws). go vet ./... clean. cd web && npx tsc -b (exit 0) && npx vite build (exit 0). TestCustomEngineStatusRunningExited/TestCustomEngineStatusExited/TestClaudeEngineKeepsHeuristicStates all PASS. TestAgentLifecycle (fake-claude) PASS unchanged.

## Verification Evidence

| # | Command | Exit Code | Verdict | Duration |
|---|---------|-----------|---------|----------|
| 1 | `go test ./...` | 0 | ✅ pass | 170000ms |
| 2 | `go vet ./...` | 0 | ✅ pass | 4000ms |
| 3 | `cd web && npx tsc -b` | 0 | ✅ pass | 8000ms |
| 4 | `cd web && npx vite build` | 0 | ✅ pass | 5000ms |
| 5 | `go test ./internal/session/... -run 'TestCustomEngine|TestClaudeEngine' -v` | 0 | ✅ pass | 55000ms |

## Deviations

The custom-agent status is collapsed at the session layer (agentStatusLocked branches on s.engine), not at the status handler as the task description vaguely allowed -- this is cleaner (the session knows its engine) and keeps the status handler unchanged. The claude regression guard asserts the heuristic-set membership rather than a single state, since a fresh claude spawn's working-vs-idle depends on activity timing.

## Known Issues

None for S01's scope. The full Agents CRUD client + Settings/Project-selector UI is S02.

## Files Created/Modified

- `internal/session/session.go`
- `internal/session/manager.go`
- `internal/session/agent_engine_test.go`
- `web/src/api/types.ts`
- `web/src/api/agents.ts`
- `web/src/components/StatusDot.tsx`
- `web/src/components/board/PRCard.tsx`
