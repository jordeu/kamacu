---
phase: 02-agent-management-ui-and-per-project-sele
plan: "05"
subsystem: ui
tags: [human-verify, active-sessions-bar, running-status, extra-params, migration]

requires:
  - phase: 02-agent-management-ui-and-per-project-sele
    provides: agent management UI (02-01..02-04)
  - phase: 01-agent-data-foundation-and-spawn-engine
    provides: custom-agent running/exited status (01-06)
provides:
  - Human-verify gate passed (6-step loop approved)
  - ActiveSessionsBar shows custom-agent running sessions
  - Extra claude params moved into the Claude agent edit dialog (backend + UI)
affects: [configurable-agents, active-sessions, spawn-extra-params]

tech-stack:
  added: []
  patterns: [verify-gate-surfaces-real-gaps, agent-row-holds-all-config]

key-files:
  created:
    - internal/store/migrations/00014_agent_extra_params.sql
    - internal/api/agents_extra_params_backfill.go
  modified:
    - web/src/components/layout/ActiveSessionsBar.tsx
    - internal/api/agents_crud.go
    - internal/api/sessions.go
    - cmd/kamacu/main.go
    - web/src/components/settings/AgentNameDialog.tsx
    - web/src/pages/SettingsPage.tsx

key-decisions:
  - "ActiveSessionsBar LIVE filter widened to include 'running' so custom-agent sessions (running/exited only per D-M001-2) appear; ranked running between working and idle"
  - "Extra claude params now live on the agents row (migration 00014: agents.extra_params TEXT NOT NULL DEFAULT '') and are edited in the Claude agent's edit dialog — configured wholly in one place"
  - "BackfillAgentExtraParams copies the effective setting value (code default --dangerously-skip-permissions OR user override) into the claude seed row — no one loses their config or the default"
  - "Spawn path (sessions.go) reads extra_params from the joined agent row instead of settings.Get (claude-engine only); fake-claude regression intact"

patterns-established:
  - "Agent configuration consolidated on the agent row, not in orphaned global settings"

requirements-completed: []

coverage:
  - id: D1
    description: ActiveSessionsBar shows custom-agent running sessions
    requirement: ""
    verification:
      - kind: manual_procedural
        ref: "T05 human-verify gate (6-step loop, approved)"
        status: pass
    human_judgment: true
    rationale: "Manual verify-gate confirmation of live custom-agent session visibility in the Active Sessions bar"
  - id: D2
    description: Extra claude params moved into Claude agent edit dialog (migration + backfill + spawn)
    requirement: ""
    verification:
      - kind: integration
        ref: "go test ./... + tsc + vite green; fake-claude regression intact"
        status: pass
    human_judgment: false

duration: ~31min
completed: 2026-07-06
status: complete
---

# Plan 02-05: Human-verify gate passed + two gate-surfaced fixes Summary

**The 6-step human-verify loop passed; two gaps it surfaced were fixed — custom-agent running sessions now appear in the Active Sessions bar, and extra claude params moved from an orphaned global Settings section into the Claude agent's edit dialog (backend migration + backfill + spawn read).**

## Performance

- **Completed:** 2026-07-06
- **Tasks:** 1
- **Files created:** 2
- **Files modified:** 6+

## Accomplishments
- **Fix 1 (ActiveSessionsBar):** widened the LIVE filter to include 'running'; added a running count + CountGroup; ranked running between working and idle; included in aria-label — D-M001-2's running/exited contract now lands fully on the bar
- **Fix 2 (extra params consolidation):**
  - Backend: migration 00014 (`agents.extra_params TEXT NOT NULL DEFAULT ''`), BackfillAgentExtraParams startup hook (copies effective setting into claude seed, idempotent), spawn reads extra_params from the joined agent row (claude-engine only), CRUD update accepts extra_params
  - Frontend: Agent interface + useUpdateAgent carry extra_params; AgentNameDialog shows an 'Extra parameters' field for claude-engine agents; removed the standalone 'Agent' section from SettingsPage

## Task Commits

1. **Fix: custom-agent sessions in Active Sessions bar** — `ce4677e` (fix)
2. **Feat: extra claude params into Claude agent edit dialog** — `61952a5` (feat)

## Files Created/Modified
- `web/src/components/layout/ActiveSessionsBar.tsx` — running in LIVE filter + count + rank
- `internal/store/migrations/00014_agent_extra_params.sql` — agents.extra_params column
- `internal/api/agents_extra_params_backfill.go` — BackfillAgentExtraParams hook
- `internal/api/agents_crud.go` — extra_params in columns/struct/scan/update
- `internal/api/sessions.go` — spawn reads extra_params from joined agent row
- `cmd/kamacu/main.go` — BackfillAgentExtraParams wired
- `web/src/components/settings/AgentNameDialog.tsx` — Extra parameters field (claude only)
- `web/src/pages/SettingsPage.tsx` — removed standalone Agent section

## Decisions Made
- Running ranked between working and idle: alive but no finer state (less actionable than claude 'working', more than 'idle')
- Extra-params backfill preserves both user overrides and the code default — no config loss

## Deviations from Plan

None — work was already implemented and committed; this summary documents the delivered state. The two fixes were themselves gate-surfaced gaps, now resolved.

## Issues Encountered
None

## Next Phase Readiness
- Configurable Agents (M001) frontend complete; verify gate passed; full Go suite + tsc + vite green

---
*Phase: 02-agent-management-ui-and-per-project-sele*
*Completed: 2026-07-06*
