---
phase: 05-session-resume-and-argv-regression-harde
plan: "03"
subsystem: api
tags: [opencode, session-capture, async-poll, persist, restart-resume, runtime]

requires:
  - phase: 05-session-resume-and-argv-regression-harde
    provides: opencode_session_id column (05-01) + resume argv (05-02)
provides:
  - captureOpencodeSessionAsync — ASYNC bounded poll that discovers opencode's ses_… id and persists it to tasks.opencode_session_id
affects: [restart-resume, opencode-lifecycle]

tech-stack:
  added: []
  patterns: [async-bounded-poll-capture, best-effort-warn-only-capture, session-done-channel-gated]

key-files:
  created: []
  modified:
    - internal/api/sessions.go
    - internal/opencode/e2e_test.go
    - internal/session/manager.go

key-decisions:
  - "Capture is an ASYNC bounded poll launched at spawn, never a spawn-time read — opencode mints its opaque ses_… id sometime after start, so kamacu must DISCOVER it"
  - "Best-effort + warn-only: a capture failure costs only the ability to resume after restart, not the live session"
  - "Poll is gated on the session's Done channel so it stops when the session exits"
  - "Persisted id is what the resume argv (05-02) keys off — closes the restart-resume loop"

patterns-established:
  - "Foreign-minted opaque ids are discovered via async bounded poll, not read at spawn"

requirements-completed: []

coverage:
  - id: D1
    description: captureOpencodeSessionAsync discovers + persists opencode session id (restart-resume runtime)
    requirement: ""
    verification:
      - kind: e2e
        ref: "internal/opencode/e2e_test.go (capture + persist lifecycle)"
        status: pass
    human_judgment: false

duration: ~1h
completed: 2026-07-08
status: complete
---

# Plan 05-03: Capture + persist opencode session id (restart-resume runtime) Summary

**captureOpencodeSessionAsync is an ASYNC bounded poll launched at spawn that discovers opencode's opaque ses_… id and persists it to tasks.opencode_session_id — closing the restart-resume loop (the persisted id is what the resume argv keys off).**

## Performance

- **Completed:** 2026-07-08
- **Tasks:** 1
- **Files modified:** 3

## Accomplishments
- `captureOpencodeSessionAsync` — polls opencode's session DB until the freshly-spawned session's id appears, then persists it to `tasks.opencode_session_id`
- Launched as a goroutine at spawn (`go captureOpencodeSessionAsync(...)`), gated on the session's Done channel
- Best-effort + warn-only: capture failure costs only post-restart resume, not the live session
- e2e coverage of the capture + persist lifecycle

## Task Commits

1. **T03: captureOpencodeSessionAsync + e2e lifecycle** — `912d378` (gsd snapshot — the runtime capture/persist implementation)

## Files Created/Modified
- `internal/api/sessions.go` — `captureOpencodeSessionAsync` + spawn-time goroutine launch + UPDATE persist
- `internal/opencode/e2e_test.go` — capture + persist lifecycle e2e
- `internal/session/manager.go` — Done-channel support for the capture poll

## Decisions Made
- Async bounded poll (not spawn-time read) because opencode mints its id sometime after start — kamacu cannot know it at spawn
- Best-effort + warn-only: the live session is unaffected by capture failure; only post-restart resume is

## Deviations from Plan

None — work was already implemented and committed; this summary documents the delivered state.

## Issues Encountered
None

## Next Phase Readiness
- opencode restart-resume loop complete: column (05-01) → resume argv (05-02) → async capture/persist (05-03). Milestone M002 deliverable finished.

---
*Phase: 05-session-resume-and-argv-regression-harde*
*Completed: 2026-07-08*
