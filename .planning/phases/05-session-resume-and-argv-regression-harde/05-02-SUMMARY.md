---
phase: 05-session-resume-and-argv-regression-harde
plan: "02"
subsystem: api
tags: [opencode, resume, argv, resumable-flag, fake-opencode, regression]

requires:
  - phase: 05-session-resume-and-argv-regression-harde
    provides: tasks.opencode_session_id column (05-01)
provides:
  - Engine-branched opencode resume argv (opencode -s <id>)
  - Engine-gated resumable flag
  - fake-opencode argv guard (testdata)
affects: [session-resume, opengine-resume-regression]

tech-stack:
  added: []
  patterns: [engine-branched-resume-argv, fake-binary-argv-regression-guard]

key-files:
  created:
    - internal/api/testdata/fake-opencode
    - internal/api/opencode_resume_test.go
  modified:
    - internal/api/sessions.go
    - internal/api/agents.go

key-decisions:
  - "opencode resume appends `opencode -s <persisted-id>` — opencode's own resume flag, NOT claude's --resume and NOT a fresh `opencode`"
  - "Resume keys off the persisted opencode_session_id only — no claude transcript glob (opencode has no transcript files)"
  - "Engine-gated resumable flag: only opencode tasks with a persisted id are resumable this way"
  - "fake-opencode argv stub locks the resume shape so it's regression-testable without the real binary"

patterns-established:
  - "Resume argv is engine-branched; each engine keys off its own session-id source"

requirements-completed: []

coverage:
  - id: D1
    description: opencode resume argv + resumable flag + fake-opencode argv regression guard
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

# Plan 05-02: opencode resume argv + resumable flag + fake-opencode guard Summary

**The resume path is engine-branched for opencode: a task with a persisted opencode_session_id resumes via `opencode -s <id>` (opencode's own flag), gated behind a fake-opencode argv stub that locks the resume shape for regression.**

## Performance

- **Completed:** 2026-07-07
- **Tasks:** 1
- **Files created:** 2
- **Files modified:** 2

## Accomplishments
- opencode resume argv in sessions.go — appends `opencode -s <persisted-id>` (opencode's resume flag)
- Engine-gated resumable flag in agents.go — only opencode tasks with a persisted id take this path
- `testdata/fake-opencode` — argv stub locking the resume shape
- `opencode_resume_test.go` — resume argv regression against the stub

## Task Commits

1. **T02: engine-branched opencode resume argv + resumable flag + fake-opencode stub** — `e9fc375` (feat)

## Files Created/Modified
- `internal/api/testdata/fake-opencode` — fake binary argv stub
- `internal/api/opencode_resume_test.go` — resume argv regression
- `internal/api/sessions.go` — opencode resume argv branch
- `internal/api/agents.go` — AgentConfig.OpencodeBin + resumable flag

## Decisions Made
- opencode resume uses opencode's own `-s <id>` flag, never claude's --resume — the two engines have incompatible resume semantics
- Resume keys off persisted id only (opencode has no transcripts to glob)

## Deviations from Plan

None — work was already implemented and committed; this summary documents the delivered state.

## Issues Encountered
None

## Next Phase Readiness
- Resume argv locked; needs the runtime that populates opencode_session_id (05-03) to be functional end-to-end

---
*Phase: 05-session-resume-and-argv-regression-harde*
*Completed: 2026-07-07*
