---
phase: 04-gated-status-plugin-unlocks-waiting-and-
plan: "01"
subsystem: infra
tags: [opencode, status-plugin, fetch, abortcontroller, defect-fix]

requires:
  - phase: 03-opencode-engine-spawn-and-activity-based
    provides: env-gated opencode status plugin (f573c63)
provides:
  - opencode status plugin notify() on fetch()+AbortController (no curl dependency)
affects: [opencode-status-heuristics, waiting-idle-states]

tech-stack:
  added: []
  patterns: [fetch-with-timeout-over-shell-out, no-silent-no-op-failure]

key-files:
  created: []
  modified:
    - internal/opencode/kamacu-status.js
    - internal/opencode/plugin.go
    - internal/opencode/plugin_test.go

key-decisions:
  - "notify() switched from a curl shell-out to fetch()+AbortController(3s) — curl was a silent no-op when absent, so activity signals never reached the receiver and waiting/idle never unlocked"
  - "fetch is available in the opencode plugin runtime; the 3s AbortController bounds the post so a stuck receiver can't hang the plugin"

patterns-established:
  - "Status-signal delivery must fail loudly or time out, not silently no-op"

requirements-completed: []

coverage:
  - id: D1
    description: opencode status plugin notify() via fetch()+AbortController (curl defect fix)
    requirement: ""
    verification:
      - kind: unit
        ref: "internal/opencode/plugin_test.go"
        status: pass
    human_judgment: false

duration: ~1h
completed: 2026-07-07
status: complete
---

# Plan 04-01: opencode status plugin curl→fetch (load-bearing defect fix) Summary

**The opencode status plugin's notify() was switched from a curl shell-out (which silently no-op'd when curl was absent, so activity signals never unlocked waiting/idle) to fetch()+AbortController(3s) — removing the silent failure mode.**

## Performance

- **Completed:** 2026-07-07
- **Tasks:** 1
- **Files modified:** 3

## Accomplishments
- `notify()` rewritten on `fetch()` + `AbortController(3s)` — no external curl dependency
- Removed the silent no-op failure mode (curl absent → signals lost → waiting/idle never unlocked)
- plugin_test.go updated for the fetch-based path

## Task Commits

1. **T01: notify() curl→fetch+AbortController** — `bc1fdd2` (feat)

## Files Created/Modified
- `internal/opencode/kamacu-status.js` — fetch()+AbortController notify
- `internal/opencode/plugin.go` — plugin wiring
- `internal/opencode/plugin_test.go` — fetch-path tests

## Decisions Made
- fetch is available in the opencode plugin runtime; the 3s AbortController bounds the post so a stuck receiver can't hang the plugin

## Deviations from Plan

None — work was already implemented and committed; this summary documents the delivered state.

## Issues Encountered
None

## Next Phase Readiness
- Status-signal delivery now reliable; ready for the real-opencode e2e proof (04-02)

---
*Phase: 04-gated-status-plugin-unlocks-waiting-and-*
*Completed: 2026-07-07*
