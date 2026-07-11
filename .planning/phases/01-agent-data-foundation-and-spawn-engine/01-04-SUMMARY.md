---
phase: 01-agent-data-foundation-and-spawn-engine
plan: "04"
subsystem: api
tags: [projects, agent-id, fk-validation, threading]

requires:
  - phase: 01-agent-data-foundation-and-spawn-engine
    provides: agents table + Claude seed (migration 00013)
provides:
  - agent_id threaded through the projects API (columns/scan/create-default/INSERT/PATCH)
  - New projects default to the global default agent (read-at-use, no restart)
affects: [configurable-agents-ui, spawn-engine]

tech-stack:
  added: []
  patterns: [mirror-workspace_id-threading, read-at-use-default-resolution]

key-files:
  created:
    - internal/api/projects_agent_test.go
  modified:
    - internal/api/projects.go

key-decisions:
  - "New projects default to the global default agent read-at-use (no restart needed for a default flip)"
  - "PATCH agent_id validates the target against the agents table (400 'agent not found', no mutation on failure)"
  - "Both INSERT paths (folder + managed) carry agent_id"

patterns-established:
  - "Threading a new FK through projects mirrors the workspace_id pattern: field + columns entry + scan slot + default helper + both INSERT paths + PATCH validation"

requirements-completed: []

coverage:
  - id: D1
    description: agent_id threaded through projects API with default + validation
    requirement: ""
    verification:
      - kind: unit
        ref: "internal/api/projects_agent_test.go (4 tests: create-default, create-explicit, create-unknown, patch-agent)"
        status: pass
    human_judgment: false

duration: ~5min
completed: 2026-07-06
status: complete
---

# Plan 01-04: Thread agent_id through the projects API Summary

**agent_id added to the projects API mirroring the workspace_id pattern — columns, scan, create-default, both INSERT paths, and PATCH validation against the agents table. New projects default to the global default agent.**

## Performance

- **Completed:** 2026-07-06
- **Tasks:** 1
- **Files created:** 1
- **Files modified:** 1

## Accomplishments
- `AgentID` field, `projectColumns` entry, `scanProject` slot added
- `defaultAgentID` + `resolveCreateAgentID` helpers — new projects default to the global default agent (read-at-use, no restart)
- Both INSERT paths (folder + managed) carry agent_id
- PATCH agent_id block validates target against agents table (400 'agent not found', no mutation)
- Backward-compatible scan/columns change — existing project + workspace tests pass unchanged

## Task Commits

1. **T04: thread agent_id through projects API** — `3a2ea84` (feat)

## Files Created/Modified
- `internal/api/projects.go` — AgentID field, columns/scan, default helpers, both INSERT paths, PATCH validation
- `internal/api/projects_agent_test.go` — 4 tests (create-default lands on id 1, create-explicit honored, create-unknown 400 no row, patch-agent changes + validates)

## Decisions Made
- Read-at-use default resolution so flipping the global default takes effect without a restart

## Deviations from Plan

None — work was already implemented and committed; this summary documents the delivered state.

## Issues Encountered
None

## Next Phase Readiness
- Projects carry agent_id; ready for the spawn engine fork (01-05) which reads the agent's engine to pick claude vs custom argv

---
*Phase: 01-agent-data-foundation-and-spawn-engine*
*Completed: 2026-07-06*
