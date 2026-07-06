---
id: T03
parent: S01
milestone: M001
key_files:
  - internal/api/agents_crud.go
  - internal/api/agents_crud_test.go
  - internal/api/routes.go
key_decisions:
  - Distinct handler types for distinct concerns: agentHandlers (status, pre-existing) vs agentCRUDHandlers (configuration CRUD, this task) — avoids one mega-handler file
  - Create always forces engine='custom'+is_default=0+is_system=0 — the claude/system seed is only ever created by migration 00013 or BackfillAgents, never by the CRUD create handler (can't forge a system agent via API)
  - System-engine lock is field-level, not row-level: PATCH on a system agent can edit name+command (e.g. point at a specific binary path) but NOT engine — the hook/resume plumbing is internal, the binary path is legitimately editable
  - set-default is a two-statement transactional UPDATE (clear-all then set-target) under SetMaxOpenConns(1), preserving the exactly-one invariant
  - Route paths coexist with the pre-existing GET /api/agents/status (exact vs exact, no ServeMux conflict); no GET /api/agents/{id} added precisely to avoid wildcard/status pattern ambiguity
duration: 
verification_result: passed
completed_at: 2026-07-06T15:50:43.486Z
blocker_discovered: false
---

# T03: Added /api/agents CRUD (list/create/update/delete/set-default) with is_system + in-use guards, mirroring the workspaces handler shape; 11 handler tests green.

**Added /api/agents CRUD (list/create/update/delete/set-default) with is_system + in-use guards, mirroring the workspaces handler shape; 11 handler tests green.**

## What Happened

Added the /api/agents CRUD surface in internal/api/agents_crud.go, mirroring the workspaces.go handler shape: Agent struct (bool-from-INTEGER for is_default/is_system), agentColumns const, scanAgent, and a agentCRUDHandlers type with list/create/update/delete/setDefault. Registered the five routes inline in routes.go alongside the workspaces routes (ah := &agentCRUDHandlers{db: db}). 

Handler semantics: list orders default-first then name-sorted (COLLATE NOCASE); create always forces engine='custom'+is_default=0+is_system=0 (the system seed is only ever created by migration/BackfillAgents, never by the API); update enforces a field-level system-engine lock (system agent name+command editable, engine immutable) and validates engine in {'claude','custom'}; delete guards on is_system (409) then COUNT(projects WHERE agent_id=?) (409 block-until-unassigned) with the ON DELETE RESTRICT FK as backstop; setDefault is a transactional clear-all-then-set-target preserving the exactly-one invariant.

Added 11 handler tests in agents_crud_test.go covering list (seed shape), create (+dup 409, +empty 400), update custom (name+command+engine), update system (engine locked 400, name ok), delete unused (204), delete system (409), delete in-use (409), and set-default (flag moves + exactly-one invariant). All pass; the pre-existing agent status tests (TestAgentLifecycle, TestAgentStatus*) still pass unchanged — no regression.

## Verification

go test ./internal/api/... -run 'TestAgent' -v -> all 11 new CRUD tests PASS (list, create, create-dup-409, create-empty-400, update-custom, update-system-engine-locked-400, delete-unused-204, delete-system-409, delete-in-use-409, set-default + exactly-one invariant); pre-existing TestAgentLifecycle/Status tests still PASS (no regression). Full api package: go test ./internal/api/... -> ok 87.643s (tmux integration tests dominate wall-clock per the retrospective). go build ./cmd/kamacu/... ok. go vet ./internal/api/... clean.

## Verification Evidence

| # | Command | Exit Code | Verdict | Duration |
|---|---------|-----------|---------|----------|
| 1 | `go test ./internal/api/... -run 'TestAgent' -v` | 0 | ✅ pass | 6000ms |
| 2 | `go test ./internal/api/...` | 0 | ✅ pass | 87643ms |
| 3 | `go vet ./internal/api/...` | 0 | ✅ pass | 3000ms |
| 4 | `go build ./cmd/kamacu/...` | 0 | ✅ pass | 3000ms |

## Deviations

Placed the CRUD in internal/api/agents_crud.go (not agents.go) as planned — the pre-existing agents.go owns the agent STATUS handler (AgentRoutes/agentHandlers), a distinct concern. Named the handler type agentCRUDHandlers to avoid clashing with the existing agentHandlers. The set-default endpoint (POST /api/agents/{id}/default) is new — workspaces had no equivalent (its default was fixed); agents need it per R019.

## Known Issues

None.

## Files Created/Modified

- `internal/api/agents_crud.go`
- `internal/api/agents_crud_test.go`
- `internal/api/routes.go`
