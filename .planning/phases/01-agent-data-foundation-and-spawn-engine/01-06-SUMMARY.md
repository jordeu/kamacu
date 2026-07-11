---
phase: 01-agent-data-foundation-and-spawn-engine
plan: "06"
subsystem: session
tags: [agent-status, custom-engine, frontend-types, running-exited, exhaustive-switch]

requires:
  - phase: 01-agent-data-foundation-and-spawn-engine
    provides: spawn engine fork on agent.engine (01-05)
provides:
  - Custom-engine agents report running/exited only (engine branch in agentStatusLocked)
  - Frontend type additions keeping the build gate green
affects: [configurable-agents-ui, agent-status-heuristics]

tech-stack:
  added: []
  patterns: [engine-branch-in-status, claude-heuristics-claude-only]

key-files:
  created:
    - internal/session/agent_engine_test.go
  modified:
    - internal/session/manager.go
    - internal/session/session.go
    - web/src/api/agents.ts
    - web/src/api/types.ts
    - web/src/components/StatusDot.tsx
    - web/src/components/board/PRCard.tsx

key-decisions:
  - "Custom-engine agents report ONLY running/exited — the working/waiting/idle heuristics are claude-hook-driven and explicitly rejected for custom TUIs (D-M001-2)"
  - "Claude (and '' back-compat) keeps the full heuristic states"
  - "Engine exposed on Info so the frontend can branch rendering"

patterns-established:
  - "Custom agents get the minimal honest status set (running/exited); rich heuristics stay claude-only"

requirements-completed: []

coverage:
  - id: D1
    description: custom-agent running/exited status with full suite green
    requirement: ""
    verification:
      - kind: unit
        ref: "internal/session/agent_engine_test.go"
        status: pass
      - kind: integration
        ref: "go test ./... (13 packages) + tsc -b + vite build + fake-claude regression"
        status: pass
    human_judgment: false

duration: ~7min
completed: 2026-07-06
status: complete
---

# Plan 01-06: Custom-agent running/exited status + frontend gate green Summary

**Custom-engine agents report only running/exited via an engine branch in agentStatusLocked; claude keeps full heuristics. Frontend types added (agent_id, running status, blue dot) keep the TS build gate and exhaustive switches green.**

## Performance

- **Completed:** 2026-07-06
- **Tasks:** 1
- **Files created:** 1
- **Files modified:** 6

## Accomplishments
- Engine branch in `agentStatusLocked`: custom agents emit running/exited only; claude (and '' back-compat) keeps working/waiting/idle heuristics
- Engine exposed on Info
- Frontend type additions to keep the build gate green:
  - `agent_id` on `Project` (mirrors workspace_id)
  - `"running"` in the `AgentStatusEntry` status union
  - a running case (blue) in `StatusDot.dotMeta` and `PRCard.agentRail` so exhaustive switches compile

## Task Commits

1. **T06: custom-agent running/exited status + frontend gate green** — `6a0e6b5` (feat)

## Files Created/Modified
- `internal/session/agent_engine_test.go` — custom-engine status assertions
- `internal/session/manager.go` — engine branch in `agentStatusLocked`
- `internal/session/session.go` — Engine on Info
- `web/src/api/agents.ts`, `web/src/api/types.ts` — agent_id + running status
- `web/src/components/StatusDot.tsx`, `web/src/components/board/PRCard.tsx` — running (blue) cases

## Decisions Made
- Custom TUIs cannot drive the claude hook heuristics, so they report the minimal honest set (running/exited) per D-M001-2
- Full Agents CRUD client + management UI deferred to a later slice (S02)

## Deviations from Plan

None — work was already implemented and committed; this summary documents the delivered state.

## Issues Encountered
None

## Next Phase Readiness
- Slice S01 backend complete; full Go suite (13 pkgs) + frontend gate green; fake-claude regression intact
- UAT for the configurable-agents feature deferred to a human-verify gate

---
*Phase: 01-agent-data-foundation-and-spawn-engine*
*Completed: 2026-07-06*
