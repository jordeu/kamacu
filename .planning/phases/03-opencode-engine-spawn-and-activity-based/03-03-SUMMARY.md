---
phase: 03-opencode-engine-spawn-and-activity-based
plan: "03"
subsystem: session
tags: [opencode, spawn-branch, argv, fake-opencode, opencode-bin, resume]

requires:
  - phase: 03-opencode-engine-spawn-and-activity-based
    provides: opencode status heuristics + seed (03-01, 03-02)
provides:
  - opencode spawn branch (engine-branched resume argv keyed off opencode_session_id)
  - AgentConfig.OpencodeBin (resolved binary path)
  - fake-opencode argv stub (testdata)
affects: [03-04, session-resume]

tech-stack:
  added: []
  patterns: [engine-branched-resume-argv, fake-binary-argv-stub]

key-files:
  created:
    - internal/api/testdata/fake-opencode
    - internal/api/opencode_resume_test.go
  modified:
    - internal/api/agents.go
    - internal/api/sessions.go

key-decisions:
  - "opencode resume argv is engine-branched: keys off the persisted opencode_session_id (no claude transcript glob), appends opencode's own resume flag"
  - "AgentConfig.OpencodeBin resolves the opencode binary path (mirrors the claude binary resolution)"
  - "fake-opencode stub in testdata locks the argv shape so resume regression is testable without the real binary"

patterns-established:
  - "Per-engine resume argv: each engine keys off its own session-id source, not a shared transcript glob"

requirements-completed: []

coverage:
  - id: D1
    description: opencode spawn branch + AgentConfig.OpencodeBin + fake-opencode argv stub
    requirement: ""
    verification:
      - kind: unit
        ref: "internal/api/opencode_resume_test.go (fake-opencode argv regression)"
        status: pass
    human_judgment: false

duration: ~2h
completed: 2026-07-07
status: complete
---

# Plan 03-03: opencode spawn branch + AgentConfig.OpencodeBin + fake-opencode stub Summary

**The opencode spawn path is engine-branched — resume argv keys off the persisted opencode_session_id (no claude transcript glob), AgentConfig.OpencodeBin resolves the binary, and a fake-opencode argv stub locks the resume shape for regression testing.**

## Performance

- **Completed:** 2026-07-07
- **Tasks:** 1
- **Files created:** 2
- **Files modified:** 2

## Accomplishments
- opencode resume argv engine-branched in sessions.go — keys off persisted `opencode_session_id`, appends opencode's resume flag
- `AgentConfig.OpencodeBin` in agents.go — resolves the opencode binary path
- `testdata/fake-opencode` — argv stub that locks the resume argv shape
- `opencode_resume_test.go` — resume argv regression test against the stub

## Task Commits

1. **T03: opencode spawn injects hook env** — `277f506` (feat) — spawn branch hook env
2. **Engine-branched opencode resume argv + fake-opencode stub** — `e9fc375` (feat) — OpencodeBin + resume argv + stub

## Files Created/Modified
- `internal/api/testdata/fake-opencode` — fake binary argv stub
- `internal/api/opencode_resume_test.go` — resume argv regression
- `internal/api/agents.go` — AgentConfig.OpencodeBin
- `internal/api/sessions.go` — opencode resume argv branch

## Decisions Made
- opencode resume keys off its own session-id source (persisted opencode_session_id), not claude's transcript glob — the two engines have incompatible session models

## Deviations from Plan

None — work was already implemented and committed; this summary documents the delivered state.

## Issues Encountered
None

## Next Phase Readiness
- Spawn + resume argv locked; ready for sessions.go engine wiring + real-opencode e2e integration (03-04)

---
*Phase: 03-opencode-engine-spawn-and-activity-based*
*Completed: 2026-07-07*
