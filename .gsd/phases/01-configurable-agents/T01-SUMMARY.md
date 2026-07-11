---
id: T01
parent: S02
milestone: M001
key_files:
  - web/src/api/types.ts
  - web/src/api/queries.ts
  - web/src/api/mutations.ts
key_decisions:
  - (none)
duration: 
verification_result: passed
completed_at: 2026-07-06T16:16:06.402Z
blocker_discovered: false
---

# T01: Added typed agents API client (Agent type + useAgents query + 4 CRUD/default mutations) mirroring the workspaces shapes; tsc clean.

**Added typed agents API client (Agent type + useAgents query + 4 CRUD/default mutations) mirroring the workspaces shapes; tsc clean.**

## What Happened

Added the typed TanStack Query API surface for agents. Agent interface in types.ts (id, name, command, engine: 'claude'|'custom', is_default, is_system, timestamps) mirrors the backend api.Agent struct. useAgents() query in queries.ts (queryKey ['agents'], GET /api/agents). Four mutations in mutations.ts mirroring the workspaces shapes: useCreateAgent ({name, command} -> POST), useUpdateAgent ({id, name?, command?, engine?} -> PATCH), useDeleteAgent (id -> DELETE), useSetDefaultAgent (id -> POST /api/agents/{id}/default). Each invalidates ['agents']; delete + setDefault also invalidate ['projects'] (a default change affects new-project assignment; a delete could affect the project list defensively).

## Verification

cd web && npx tsc -b -> exit 0 (all new types/mutations compile, no unused or missing imports).

## Verification Evidence

| # | Command | Exit Code | Verdict | Duration |
|---|---------|-----------|---------|----------|
| 1 | `cd web && npx tsc -b` | 0 | ✅ pass | 8000ms |

## Deviations

None. Mirrored the workspaces mutation shapes exactly.

## Known Issues

None.

## Files Created/Modified

- `web/src/api/types.ts`
- `web/src/api/queries.ts`
- `web/src/api/mutations.ts`
