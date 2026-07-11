---
phase: 01-agent-data-foundation-and-spawn-engine
plan: "02"
subsystem: infra
tags: [startup-hook, backfill, agents, seed, idempotent]

requires:
  - phase: 01-agent-data-foundation-and-spawn-engine
    provides: agents table + Claude seed (migration 00013)
provides:
  - BackfillAgents(db) startup hook that re-creates the Claude seed if missing
  - main.go wiring (BackfillProjectIcons -> BackfillWorkspaces -> BackfillAgents)
affects: [configurable-agents, startup-ordering]

tech-stack:
  added: []
  patterns: [backfill-mirrors-workspaces, dedicated-file-avoids-shared-test-helper-clobber]

key-files:
  created:
    - internal/api/agents_backfill.go
    - internal/api/agents_backfill_test.go
  modified:
    - cmd/kamacu/main.go

key-decisions:
  - "BackfillAgents lives in agents_backfill.go (not the pre-existing agents.go which holds the agent STATUS handler + shared test helpers) to avoid clobbering TestMain setup"
  - "Hook only INSERTs when the default agent is missing — idempotent no-op on healthy boot"

patterns-established:
  - "One backfill concern per file mirroring the workspaces backfill shape"

requirements-completed: []

coverage:
  - id: D1
    description: BackfillAgents startup hook restores Claude seed with engine/is_system markers
    requirement: ""
    verification:
      - kind: unit
        ref: "internal/api/agents_backfill_test.go#TestBackfillAgents"
        status: pass
    human_judgment: false

duration: ~81min
completed: 2026-07-06
status: complete
---

# Plan 01-02: BackfillAgents startup hook + main.go wiring Summary

**Idempotent startup hook that re-creates the Claude Code default agent if missing, mirroring BackfillWorkspaces and wired into main.go after the workspace backfill.**

## Performance

- **Completed:** 2026-07-06
- **Tasks:** 1
- **Files created:** 2
- **Files modified:** 1

## Accomplishments
- `BackfillAgents(db)`: SELECT the default agent, no-op if found, INSERT the Claude seed (engine='claude', is_default=1, is_system=1) only if missing
- main.go wiring: `BackfillProjectIcons -> BackfillWorkspaces -> BackfillAgents` ordering
- Idempotency test mirrors TestBackfillWorkspaces (healthy-boot no-op, missing-default re-create, markers survive, idempotent)

## Task Commits

1. **T02: BackfillAgents hook + wiring + test** — `99a6b63` (feat)

## Files Created/Modified
- `internal/api/agents_backfill.go` — BackfillAgents(db) mirroring BackfillWorkspaces
- `internal/api/agents_backfill_test.go` — no-op / re-create / marker / idempotency cases
- `cmd/kamacu/main.go` — hook wired after BackfillWorkspaces

## Decisions Made
- Placed in a dedicated `agents_backfill.go` rather than the pre-existing `agents.go` (which holds the agent STATUS handler `AgentRoutes` and shared test helpers in `agents_test.go`) to avoid clobbering TestMain setup
- T03's CRUD deliberately landed separately in `agents_crud.go`

## Deviations from Plan

None — work was already implemented and committed; this summary documents the delivered state.

## Issues Encountered
None

## Next Phase Readiness
- Startup ordering established; ready for /api/agents CRUD (01-03) which will land in the sibling `agents_crud.go`

---
*Phase: 01-agent-data-foundation-and-spawn-engine*
*Completed: 2026-07-06*
