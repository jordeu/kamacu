---
phase: 17-global-active-sessions-bar
plan: 01
subsystem: api
tags: [go, sqlite, sql-join, agent-status, typescript, wire-contract]

# Dependency graph
requires:
  - phase: 16-sharper-review-column
    provides: "The /api/agents/status feed already widened with prNumber + source via the same SELECT-widening pattern (no migration); columns tasks.title + projects.name pre-exist in schema (migrations 00001/00007)"
provides:
  - "GET /api/agents/status now returns taskTitle + projectName on every entry, from BOTH the manager-derived and DB-derived/post-restart passes (SBAR-10)"
  - "TS AgentStatusEntry type carries taskTitle: string + projectName: string, ready for Plan 02's bottom bar to render row labels"
affects: [17-02, global-active-sessions-bar, AppLayout, useAgentStatuses]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Widen the existing /api/agents/status SELECTs via a JOIN (no new endpoint, no new migration) — same approach v1.5 used for prNumber/source"

key-files:
  created: []
  modified:
    - internal/api/agents.go
    - internal/api/agents_test.go
    - web/src/api/agents.ts

key-decisions:
  - "No new endpoint, no new migration: taskTitle + projectName come from a JOIN onto the existing status query (columns pre-exist) — both passes joined identically"
  - "tasks.title + projects.name are NOT NULL in the schema, so plain string scan targets (not sql.NullString) are correct"
  - "projectName asserted non-empty (not hardcoded basename) because createProject derives the name from a temp-dir path"

patterns-established:
  - "Both /api/agents/status passes (manager-derived + DB-derived) are kept symmetric: any new per-task column is added to both SELECTs, both Scans, and both append blocks"

requirements-completed: [SBAR-10]

# Metrics
duration: 9min
completed: 2026-06-18
---

# Phase 17 Plan 01: Widen /api/agents/status with task title + project name Summary

**The agent-status feed now carries `taskTitle` + `projectName` on every entry (both the manager-derived and post-restart passes) via a `JOIN projects`, and the TS `AgentStatusEntry` type exposes them — the sole backend change for the Global Active Sessions Bar (SBAR-10), with no new endpoint and no new migration.**

## Performance

- **Duration:** ~9 min
- **Started:** 2026-06-18T06:14Z
- **Completed:** 2026-06-18T06:24Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- Extended `agentStatusEntry` (Go) with `TaskTitle` + `ProjectName` fields and JOINed `projects` into both SELECT passes, selecting `tasks.title` + `projects.name` (qualified columns now that two tables are joined).
- Wired the new fields into both `entries = append(...)` blocks so the manager-derived AND DB-derived/post-restart paths both populate the labels.
- Added field assertions to two existing tests, exercising both JOIN paths: `TestAgentStatusSingleEntry` (manager-derived) and `TestAgentStatusPRLinkFields` (DB-derived, manual + github_pr).
- Extended the TS `AgentStatusEntry` interface with `taskTitle: string` + `projectName: string`, leaving `useAgentStatuses()` (the single 5s poll, D-12) untouched.

## Task Commits

Each task was committed atomically:

1. **Task 1: Widen both /api/agents/status SELECTs to JOIN task title + project name** - `5caae7f` (feat)
2. **Task 2: Assert taskTitle + projectName in agents_test.go and extend the TS wire type** - `f4977a0` (test)

_Note: Task 1 (`tdd="true"`) is the implementation; Task 2 (`tdd="true"`) is the test assertions + TS type — the plan concentrates the new-field assertions in Task 2 per its explicit structure, so each task produced a single commit._

## Files Created/Modified
- `internal/api/agents.go` - `agentStatusEntry` gains `TaskTitle`/`ProjectName`; both SELECT passes JOIN `projects` and select `tasks.title` + `projects.name`; both Scans + append blocks populate the fields.
- `internal/api/agents_test.go` - `TestAgentStatusSingleEntry` and `TestAgentStatusPRLinkFields` assert `taskTitle`/`projectName` (covering the manager-derived and DB-derived passes respectively).
- `web/src/api/agents.ts` - `AgentStatusEntry` interface extended with `taskTitle: string` + `projectName: string`.

## Decisions Made
- None beyond the plan: followed the plan's three concrete edits per task exactly. Confirmed schema NOT-NULL guarantees on `tasks.title`/`projects.name` justify plain `string` scan targets, and used a non-empty check for `projectName` (temp-dir-derived) rather than hardcoding the basename, as the plan directed.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- The wire contract Plan 02 depends on is in place: `/api/agents/status` returns `taskTitle` + `projectName` on every entry from both passes, and the TS type exposes them.
- `useAgentStatuses()` remains the single 5s-poll source the bottom bar will reuse (D-12) — Plan 02 can build `AppLayout.tsx`'s bar directly against `AgentStatusEntry` with no further backend work.
- Verification all green: `go build ./...`, `go vet ./...`, `go test ./internal/api/`, and `npx tsc -b` all exit 0; JOIN count == 2; no migration added.

## Self-Check: PASSED

All claimed files exist (SUMMARY.md, agents.go, agents_test.go, agents.ts) and both task commits (`5caae7f`, `f4977a0`) are present in git history.

---
*Phase: 17-global-active-sessions-bar*
*Completed: 2026-06-18*
