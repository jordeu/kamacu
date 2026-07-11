---
phase: 02-agent-management-ui-and-per-project-sele
plan: "01"
subsystem: ui
tags: [react, api-client, tanstack-query, agents, crud]

requires:
  - phase: 01-agent-data-foundation-and-spawn-engine
    provides: /api/agents CRUD endpoints
provides:
  - Typed Agent type + useAgents query + 4 CRUD/default mutations (frontend)
affects: [02-02, 02-03, 02-04, agent-management-ui]

tech-stack:
  added: []
  patterns: [api-client-mirrors-workspaces-shapes, tanstack-query-cache-invalidation]

key-files:
  created: []
  modified:
    - web/src/api/types.ts
    - web/src/api/queries.ts
    - web/src/api/mutations.ts

key-decisions:
  - "Mirrored the workspaces API-client shapes (query + mutations) so agent CRUD follows the established pattern"
  - "Mutations invalidate the agents query cache so the UI refreshes after create/update/delete/set-default"

patterns-established:
  - "Per-resource API client: type + query hook + mutation hooks in queries.ts/mutations.ts"

requirements-completed: []

coverage:
  - id: D1
    description: typed agents API client with useAgents + CRUD/default mutations
    requirement: ""
    verification:
      - kind: unit
        ref: "tsc -b clean (type-level)"
        status: pass
    human_judgment: false

duration: ~2min
completed: 2026-07-06
status: complete
---

# Plan 02-01: Frontend agents API client + Agent type Summary

**Typed Agent type plus useAgents query and four CRUD/default mutations mirroring the workspaces client shapes, giving the frontend full access to the /api/agents endpoints built in Phase 01.**

## Performance

- **Completed:** 2026-07-06
- **Tasks:** 1
- **Files modified:** 3

## Accomplishments
- `Agent` type in types.ts (id/name/command/engine/is_default/is_system)
- `useAgents` query hook (GET /api/agents)
- Four mutation hooks: create/update/delete/set-default — each invalidates the agents query cache on success

## Task Commits

1. **T01: frontend agents API client + Agent type** — `da036fa` (feat)

## Files Created/Modified
- `web/src/api/types.ts` — Agent type
- `web/src/api/queries.ts` — useAgents query
- `web/src/api/mutations.ts` — useCreateAgent/useUpdateAgent/useDeleteAgent/useSetDefaultAgent

## Decisions Made
- Mirrored workspaces client shapes for consistency

## Deviations from Plan

None — work was already implemented and committed; this summary documents the delivered state.

## Issues Encountered
None

## Next Phase Readiness
- API client ready for the Project Settings agent selector (02-02) and global Settings Agents section (02-03)

---
*Phase: 02-agent-management-ui-and-per-project-sele*
*Completed: 2026-07-06*
