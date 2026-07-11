---
phase: 01-agent-data-foundation-and-spawn-engine
plan: "03"
subsystem: api
tags: [rest, crud, agents, set-default, delete-guards, is-system]

requires:
  - phase: 01-agent-data-foundation-and-spawn-engine
    provides: agents table + Claude seed (migration 00013)
provides:
  - /api/agents CRUD (list/create/update/delete/set-default)
  - is_system + in-use guards (409 on system delete, 409 block-until-unassigned)
  - renderAgentCommand helper (reused by 01-05 spawn engine)
affects: [configurable-agents-ui, projects-agent-threading]

tech-stack:
  added: []
  patterns: [handler-mirrors-workspaces-shape, distinct-handler-type-per-concern]

key-files:
  created:
    - internal/api/agents_crud.go
    - internal/api/agents_crud_test.go
  modified:
    - internal/api/routes.go

key-decisions:
  - "Create forces engine='custom', is_default=0, is_system=0 — the system seed is only ever created by migration 00013 / BackfillAgents, never via API"
  - "Update enforces a field-level system-engine lock: system agent name+command editable, engine immutable; engine validated to 'claude'|'custom'"
  - "set-default is transactional clear-all + set-target to preserve the exactly-one default invariant"
  - "Distinct type agentCRUDHandlers vs the pre-existing agentHandlers (status) — separate concerns, no collision"

patterns-established:
  - "Handler shape mirroring workspaces.go: list default-first then COLLATE NOCASE name, create/update/delete/set-default"
  - "DB ON DELETE RESTRICT FK backstop behind handler-level in-use guard"

requirements-completed: []

coverage:
  - id: D1
    description: /api/agents CRUD with is_system + in-use guards
    requirement: ""
    verification:
      - kind: unit
        ref: "internal/api/agents_crud_test.go (11 handler tests)"
        status: pass
    human_judgment: false

duration: ~6min
completed: 2026-07-06
status: complete
---

# Plan 01-03: /api/agents CRUD + set-default + delete guards Summary

**Full REST CRUD for agents in a new agents_crud.go mirroring the workspaces handler shape — create locks custom-only, update enforces a system-engine field lock, delete guards system + in-use, set-default preserves the exactly-one invariant.**

## Performance

- **Completed:** 2026-07-06
- **Tasks:** 1
- **Files created:** 2
- **Files modified:** 1

## Accomplishments
- `list` (default-first, then name COLLATE NOCASE), `create`, `update`, `delete`, `set-default` endpoints
- Create forces engine='custom'/is_default=0/is_system=0 (system seed never API-created)
- Update field-level system-engine lock (system agent name+command editable, engine immutable)
- Delete guards: 409 on is_system=1; 409 block-until-unassigned on COUNT(projects) > 0 (ON DELETE RESTRICT FK backstop)
- set-default transactional clear-all + set-target (exactly-one invariant)
- Routes registered inline in routes.go alongside workspaces

## Task Commits

1. **T03: /api/agents CRUD + set-default + delete guards** — `a22cb33` (feat)

## Files Created/Modified
- `internal/api/agents_crud.go` — CRUD handlers in `agentCRUDHandlers` (distinct from status `agentHandlers`)
- `internal/api/agents_crud_test.go` — 11 handler tests (list, create+dup+empty, update custom+system-locked, delete unused+system+in-use, set-default invariant)
- `internal/api/routes.go` — agent CRUD routes registered alongside workspaces

## Decisions Made
- Distinct `agentCRUDHandlers` type vs pre-existing `agentHandlers` (status) — separate concerns, no collision; route paths coexist with pre-existing `GET /api/agents/status` (exact vs exact, no ServeMux conflict)

## Deviations from Plan

None — work was already implemented and committed; this summary documents the delivered state.

## Issues Encountered
None

## Next Phase Readiness
- CRUD ready; `renderAgentCommand` helper added in 01-05 will reuse `settings.Tokenize`
- Agent management UI (client CRUD + dialog) deferred to a later slice

---
*Phase: 01-agent-data-foundation-and-spawn-engine*
*Completed: 2026-07-06*
