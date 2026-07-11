---
phase: 03-opencode-engine-spawn-and-activity-based
plan: "02"
subsystem: session
tags: [opencode, status-heuristics, agent-status, engine-matrix, hook-env]

requires:
  - phase: 03-opencode-engine-spawn-and-activity-based
    provides: opencode system agent seed (03-01)
provides:
  - opencode treated as a full-heuristics engine in agentStatusLocked
  - opencode spawn injects the D014 KAMACU_* hook env
  - Engine-matrix test (opencode_engine_test.go)
affects: [gated-status-plugin, opencode-status]

tech-stack:
  added: []
  patterns: [opencode-full-heuristics-engine, hook-env-injection-for-activity]

key-files:
  created:
    - internal/session/opencode_engine_test.go
  modified:
    - internal/session/manager.go
    - internal/session/session.go

key-decisions:
  - "opencode is treated as a full-heuristics engine in agentStatusLocked (working/waiting/idle), unlike custom which is running/exited only — because opencode drives the activity hooks"
  - "opencode spawn injects the D014 KAMACU_* hook env so the status plugin receives activity signals"
  - "custom and claude paths verified byte-for-byte unchanged"

patterns-established:
  - "Engines that ship an activity plugin get full heuristic status; bare-custom TUIs stay at running/exited"

requirements-completed: []

coverage:
  - id: D1
    description: opencode heuristic status branch + hook env injection, engine-matrix test
    requirement: ""
    verification:
      - kind: unit
        ref: "internal/session/opencode_engine_test.go (engine-matrix: claude/custom/opencode)"
        status: pass
    human_judgment: false

duration: ~2h
completed: 2026-07-07
status: complete
---

# Plan 03-02: opencode heuristic status branch + engine-matrix test Summary

**opencode is now treated as a full-heuristics engine in agentStatusLocked (working/waiting/idle via the activity plugin), spawn injects the D014 KAMACU_* hook env, and an engine-matrix test proves claude/custom/opencode each report the correct status states.**

## Performance

- **Completed:** 2026-07-07
- **Tasks:** 1
- **Files created:** 1
- **Files modified:** 2

## Accomplishments
- `agentStatusLocked` opencode branch — full heuristics (working/waiting/idle), driven by the activity hooks
- opencode spawn injects the D014 `KAMACU_*` hook env so the status plugin receives activity signals
- `opencode_engine_test.go` — engine-matrix test (claude/custom/opencode status states)
- claude and custom paths verified byte-for-byte unchanged

## Task Commits

1. **T02: opencode spawn hook env + status heuristics + engine-matrix test** — `277f506` (feat)

## Files Created/Modified
- `internal/session/opencode_engine_test.go` — engine-matrix status test
- `internal/session/manager.go` — opencode status branch + hook env injection
- `internal/session/session.go` — engine/Info plumbing

## Decisions Made
- opencode gets full heuristics (unlike custom's running/exited-only) because it ships an activity plugin that emits the hook signals

## Deviations from Plan

None — work was already implemented and committed; this summary documents the delivered state.

## Issues Encountered
None

## Next Phase Readiness
- Status heuristics wired; ready for the opencode spawn branch + fake-opencode stub (03-03)

---
*Phase: 03-opencode-engine-spawn-and-activity-based*
*Completed: 2026-07-07*
