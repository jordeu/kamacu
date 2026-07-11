---
id: S01
parent: M001
milestone: M001
provides:
  - agents table + projects.agent_id FK + BackfillAgents (data foundation)
  - /api/agents CRUD + set-default (configuration surface for S02 UI)
  - agent.engine-resolved spawn (claude unchanged, custom render)
  - custom-agent running/exited status (board + sessions bar)
  - agent_id on the Project wire type (for the S02 project selector)
requires:
  []
affects:
  - S02 — consumes the /api/agents CRUD + agent_id on projects + the running status state
key_files:
  - internal/store/migrations/00013_agents.sql
  - internal/store/agents_migration_test.go
  - internal/api/agents_backfill.go
  - internal/api/agents_backfill_test.go
  - internal/api/agents_crud.go
  - internal/api/agents_crud_test.go
  - internal/api/projects_agent_test.go
  - internal/session/session.go
  - internal/session/manager.go
  - internal/session/agent_engine_test.go
  - internal/api/sessions.go
  - internal/api/routes.go
  - internal/api/projects.go
  - cmd/kamacu/main.go
  - web/src/api/types.ts
  - web/src/api/agents.ts
  - web/src/components/StatusDot.tsx
  - web/src/components/board/PRCard.tsx
key_decisions:
  - engine column encodes capability tier (claude|custom) — spawn branches on it, never name-string matching; extensible to future engines
  - Custom command render = tokenize-first-then-substitute-per-token (paths with spaces survive as single tokens)
  - Session stays a pure leaf: API renders the template (reusing settings.Tokenize, no new deps); session does LookPath + exec.Command
  - Custom agents report only running/exited (D-M001-2); claude keeps working/waiting/idle via the hook overlay
  - Migration 00013 mirrors 00012 exactly (NO TRANSACTION + PRAGMA FK off/on) — the proven reversible shape for NOT NULL REFERENCES ADD COLUMN
patterns_established:
  - M001 agents mirror the v1.9 workspaces pattern end-to-end (table+seed+FK+backfill, CRUD with is_default/is_system guards, threading the FK through projects)
  - SpawnOpts gains engine + pre-split args; the engine branch keeps the proven claude path frozen and adds a parallel custom path
  - agentStatusLocked engine branch: capability-tier-specific status reporting without a separate status pipeline
observability_surfaces:
  - none
drill_down_paths:
  []
duration: ""
verification_result: passed
completed_at: 2026-07-06T16:10:47.600Z
blocker_discovered: false
---

# S01: Agent data foundation and spawn engine

**Stood up the agents data layer (migration 00013 + BackfillAgents + /api/agents CRUD), threaded agent_id through projects, and forked the spawn engine on agent.engine — claude byte-for-byte unchanged, custom renders the command template in the worktree PTY with running/exited status.**

## What Happened

S01 delivered the full backend of Configurable Agents across 6 tasks. The agents table (migration 00013, mirroring 00012_workspaces.sql exactly) seeds Claude Code as a non-deletable default (is_system=1, is_default=1, engine='claude', id=1) and adds projects.agent_id NOT NULL DEFAULT 1 REFERENCES agents(id) ON DELETE RESTRICT — so every existing and new project lands on Claude with zero migration action, preserving v1.9 behavior. A staged-upgrade test proves pre-existing data survives and all invariants hold (exactly-one default, NOT NULL, RESTRICT, FK re-armed ON, idempotent).

BackfillAgents (mirroring BackfillWorkspaces) is the startup safety net, wired after BackfillWorkspaces in main.go. The /api/agents CRUD (list/create/update/delete/set-default) reuses the workspaces.go handler shape with two guards: is_system agents are non-deletable, and in-use agents are blocked-until-unassigned (ON DELETE RESTRICT backstop). agent_id is threaded through the projects API identically to workspace_id (resolve-at-create-default, validate-on-PATCH).

The spawn engine fork (the milestone's risk center) branches on a new SpawnOpts.AgentEngine: the claude path is byte-for-byte unchanged (session-id/--resume, --settings hook overlay, ExtraArgs, BEL scanning, quota — the fake-claude regression suite TestAgentLifecycle passes unchanged, retiring the key risk), and the custom path renders the agent's command template in the worktree PTY. The template render tokenizes FIRST then substitutes per-token (a real bug — initial replace-then-tokenize broke {{worktree}} values with spaces; caught by unit test, fixed). Custom agents report only running/exited via an engine branch in agentStatusLocked, skipping the claude-hook-driven working/waiting/idle heuristics that were explicitly rejected for custom TUIs (D-M001-2). Claude keeps the full heuristic states.

Two self-inflicted clobbers early (overwrote pre-existing agents.go/agents_test.go before reading them — restored via git) reinforced read-before-overwrite applies to test files carrying shared fixtures. Otherwise the slice was mechanical: every task mirrored an existing v1.9 pattern (00012, BackfillWorkspaces, workspaces.go CRUD, workspace_id threading).

## Verification

FULL Go suite green: go test ./... -> all 13 packages ok (api 89.5s, ws 61s, session 8.5s, store, diff, github, migrate, quota, reaper, settings, tmux, worktree, cmd). go vet ./... clean. Frontend gate: cd web && tsc -b (exit 0) && vite build (exit 0). Key regression gate: TestAgentLifecycle (fake-claude, full spawn+hooks+resume lifecycle) PASS UNCHANGED — the claude argv is byte-for-byte preserved across the entire slice. New tests: TestAgentsMigration (staged-upgrade), TestBackfillAgents (idempotent), 11 CRUD handler tests, 4 projects-agent tests, 7 renderAgentCommand unit cases, 3 custom-engine status tests.

## Requirements Advanced

- R012 — agents table + Claude seed (is_system/is_default/engine='claude') via migration 00013
- R013 — projects.agent_id FK + BackfillAgents startup hook
- R014 — /api/agents create (name + command template)
- R015 — /api/agents update (system engine locked, name+command editable)
- R016 — claude spawn path byte-for-byte unchanged (fake-claude regression green)
- R017 — custom spawn engine: template render in worktree PTY
- R018 — delete guard (is_system non-deletable + in-use block-until-unassigned)
- R019 — set-default (transactional exactly-one invariant)
- R020 — agent_id resolved at spawn (read-at-use); new projects default

## Requirements Validated

None.

## New Requirements Surfaced

None.

## Requirements Invalidated or Re-scoped

None.

## Operational Readiness

None.

## Deviations

None.

## Known Limitations

None for S01's scope. The Agents management UI (Settings section + Project selector + Agent-tab quota-awareness) is S02.

## Follow-ups

None.

## Files Created/Modified

None.
