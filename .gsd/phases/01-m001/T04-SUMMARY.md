---
id: T04
parent: S01
milestone: M001
key_files:
  - internal/api/projects.go
  - internal/api/projects_agent_test.go
key_decisions:
  - agent_id threads through identically to workspace_id: Project field, projectColumns entry, scanProject slot, resolveCreateAgentID helper, defaultAgentID helper, both INSERT paths (folder + managed), PATCH validation block
  - New projects default to the global default agent via resolveCreateAgentID -> defaultAgentID (read-at-use, no restart) — R020
  - PATCH agent_id validates the target against the agents table before the row is touched (400 'agent not found', no mutation) — same posture as the workspace_id PATCH
duration: 
verification_result: passed
completed_at: 2026-07-06T15:55:20.972Z
blocker_discovered: false
---

# T04: Threaded agent_id through the projects API (columns/scan/create-default/both INSERT paths/PATCH validation) mirroring the workspace_id pattern; 4 tests green, no project/workspace regression.

**Threaded agent_id through the projects API (columns/scan/create-default/both INSERT paths/PATCH validation) mirroring the workspace_id pattern; 4 tests green, no project/workspace regression.**

## What Happened

Threaded projects.agent_id through the projects API, mirroring how workspace_id was threaded in Phase 26. Edits to internal/api/projects.go (13 coordinated changes): (1) AgentID field on Project; (2) agent_id added to projectColumns; (3) &p.AgentID added to scanProject; (4) new defaultAgentID() helper; (5) new resolveCreateAgentID() helper (validates supplied id, falls back to default); (6) create req struct gains AgentID; (7) create resolves agID up front and passes to both paths; (8) folder INSERT adds agent_id column + agID arg; (9) createByRepo signature gains agID; (10) managed INSERT adds agent_id + agID; (11) PATCH req struct gains AgentID; (12) PATCH "nothing to update" check includes req.AgentID; (13) PATCH agent_id set block (validates against agents table, 400 on unknown). New projects default to the global default agent (read-at-use).

Added projects_agent_test.go with 4 tests: create-default-agent (lands on id 1), create-explicit-agent (honored), create-unknown-agent (400, no row), patch-agent (changes + validates, failed patch leaves it unchanged). All pass.

## Verification

go test ./internal/api/... -run 'TestProjectCreateDefaultAgent|TestProjectCreateExplicitAgent|TestProjectCreateUnknownAgent|TestProjectPatchAgent' -v -> all 4 PASS. Regression: TestProject*+TestWorkspace*+TestCreate ok (1.187s) — the projectColumns/scanProject change did not regress existing project/workspace tests. Full api package ok (88.247s). go vet clean.

## Verification Evidence

| # | Command | Exit Code | Verdict | Duration |
|---|---------|-----------|---------|----------|
| 1 | `go test ./internal/api/... -run 'TestProjectCreateDefaultAgent|TestProjectCreateExplicitAgent|TestProjectCreateUnknownAgent|TestProjectPatchAgent' -v` | 0 | ✅ pass | 171000ms |
| 2 | `go test ./internal/api/... -run 'TestProject|TestWorkspace|TestCreate'` | 0 | ✅ pass | 1187000ms |
| 3 | `go test ./internal/api/...` | 0 | ✅ pass | 88247ms |
| 4 | `go vet ./internal/api/...` | 0 | ✅ pass | 3000ms |

## Deviations

None functionally. Threading pattern mirrors workspace_id exactly (resolve helper + field + column + scan + both INSERT paths + PATCH block).

## Known Issues

None.

## Files Created/Modified

- `internal/api/projects.go`
- `internal/api/projects_agent_test.go`
