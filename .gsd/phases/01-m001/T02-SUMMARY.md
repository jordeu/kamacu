---
id: T02
parent: S01
milestone: M001
key_files:
  - internal/api/agents_backfill.go
  - internal/api/agents_backfill_test.go
  - cmd/kamacu/main.go
key_decisions:
  - BackfillAgents lives in its own file (agents_backfill.go), not the pre-existing agents.go (status handler), to avoid overwriting shared test helpers and keep the M001 CRUD concerns separable for T03
  - The re-created seed preserves engine='claude' + is_system=1 (capability tier + non-deletability survive the defensive re-create path), asserted by the test
duration: 
verification_result: passed
completed_at: 2026-07-06T15:44:31.871Z
blocker_discovered: false
---

# T02: Added BackfillAgents startup hook (mirrors BackfillWorkspaces) + main.go wiring + idempotency test; verified the re-create path restores the Claude seed with engine/is_system markers.

**Added BackfillAgents startup hook (mirrors BackfillWorkspaces) + main.go wiring + idempotency test; verified the re-create path restores the Claude seed with engine/is_system markers.**

## What Happened

Added BackfillAgents(db) in a new file internal/api/agents_backfill.go, mirroring BackfillWorkspaces exactly: SELECT id FROM agents WHERE is_default=1; no-op if found; INSERT the Claude seed (engine='claude', is_default=1, is_system=1) only if the default is missing. Wired it in cmd/kamacu/main.go right after the BackfillWorkspaces block (preserving the documented ordering: BackfillProjectIcons -> BackfillWorkspaces -> BackfillAgents), with the same error-posture (log + os.Exit(1)).

Added TestBackfillAgents in internal/api/agents_backfill_test.go mirroring TestBackfillWorkspaces: (1) healthy boot is a no-op (migration already seeded Claude), (2) missing-default re-creates the seed, (3) the re-created seed carries engine='claude' + is_system=1, (4) idempotent on a second call.

Caught a self-inflicted clobber during verification: my first write replaced the pre-existing internal/api/agents.go (which holds the agent STATUS handler AgentRoutes + the agentHandlers type) and agents_test.go (which holds shared test helpers newAgentServer/setTaskClaudeSession used across sessions_test.go and tasks_test.go), breaking the whole api test build with undefined symbols. Root cause: I didn't read those files before writing. Fixed by git-restoring both originals and putting BackfillAgents + its test in dedicated companion files (agents_backfill.go / agents_backfill_test.go). Lesson reinforced: read-before-overwrite applies to test files too -- they often carry shared fixtures.

## Verification

go test ./internal/api/... -run TestBackfillAgents -v -> PASS (healthy-boot no-op, missing-default re-create, engine/is_system markers, idempotent re-run). Full api package test build green (shared test helpers intact). go build ./cmd/kamacu/... ok (BackfillAgents call compiles). go vet ./internal/api/... ./cmd/kamacu/... clean. Regression: TestBackfillWorkspaces still passes; internal/store green.

## Verification Evidence

| # | Command | Exit Code | Verdict | Duration |
|---|---------|-----------|---------|----------|
| 1 | `go test ./internal/api/... -run TestBackfillAgents -v` | 0 | ✅ pass | 45000ms |
| 2 | `go test ./internal/api/... -run 'TestBackfillWorkspaces|TestBackfillAgents'` | 0 | ✅ pass | 82000ms |
| 3 | `go test ./internal/store/...` | 0 | ✅ pass | 1000ms |
| 4 | `go build ./cmd/kamacu/...` | 0 | ✅ pass | 3000ms |
| 5 | `go vet ./internal/api/... ./cmd/kamacu/...` | 0 | ✅ pass | 3000ms |

## Deviations

None functionally. File placement changed: BackfillAgents lives in a new internal/api/agents_backfill.go (and its test in agents_backfill_test.go) rather than in the pre-existing internal/api/agents.go (which holds the agent STATUS handler + AgentRoutes, unrelated to this task). This keeps the status handler and the M001 CRUD concerns in separate files and avoids clobbering the existing agents_test.go's shared test helpers (newAgentServer, setTaskClaudeSession).

## Known Issues

None.

## Files Created/Modified

- `internal/api/agents_backfill.go`
- `internal/api/agents_backfill_test.go`
- `cmd/kamacu/main.go`
