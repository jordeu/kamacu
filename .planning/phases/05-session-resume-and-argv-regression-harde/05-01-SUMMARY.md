---
phase: 05-session-resume-and-argv-regression-harde
plan: "01"
subsystem: database
tags: [sqlite, migration, opencode, session-id, nullable-column]

requires:
  - phase: 03-opencode-engine-spawn-and-activity-based
    provides: opencode engine spawn + sessions.go wiring
provides:
  - tasks.opencode_session_id nullable column (migration 00016)
affects: [05-02, 05-03, opencode-resume]

tech-stack:
  added: []
  patterns: [nullable-fk-style-column-for-opaque-foreign-id, upgrade-and-fresh-install-migration-tests]

key-files:
  created:
    - internal/store/migrations/00016_opencode_session_id.sql
    - internal/store/opencode_session_id_test.go
  modified: []

key-decisions:
  - "Column is nullable (opencode mints its own opaque ses_… id asynchronously, so it is absent at spawn time and discovered later) — unlike claude_session_id which kamacu mints up front"
  - "opencode has NO transcript files (sessions live in opencode.db), so resume must key off this persisted id alone"

patterns-established:
  - "Per-engine session identity columns: claude_session_id (kamacu-minted, present at spawn) vs opencode_session_id (opaque, discovered async, nullable)"

requirements-completed: []

coverage:
  - id: D1
    description: tasks.opencode_session_id nullable column with upgrade + fresh-install migration tests
    requirement: ""
    verification:
      - kind: integration
        ref: "internal/store/opencode_session_id_test.go"
        status: pass
    human_judgment: false

duration: ~1h
completed: 2026-07-07
status: complete
---

# Plan 05-01: tasks.opencode_session_id column (migration 00016) Summary

**Migration 00016 adds the nullable tasks.opencode_session_id column — the persisted identity opencode resume keys off (opencode mints its opaque ses_… id asynchronously, so the column is nullable unlike claude_session_id).**

## Performance

- **Completed:** 2026-07-07
- **Tasks:** 1
- **Files created:** 2

## Accomplishments
- `00016_opencode_session_id.sql` — nullable `tasks.opencode_session_id` column
- Upgrade + fresh-install migration tests proving the column lands correctly in both paths

## Task Commits

1. **T01: migration 00016 + tests** — `785a71d` (test)

## Files Created/Modified
- `internal/store/migrations/00016_opencode_session_id.sql` — nullable column
- `internal/store/opencode_session_id_test.go` — upgrade + fresh-install tests

## Decisions Made
- Nullable because opencode's id is discovered asynchronously (absent at spawn), unlike claude_session_id which kamacu mints up front

## Deviations from Plan

None — work was already implemented and committed; this summary documents the delivered state.

## Issues Encountered
None

## Next Phase Readiness
- Column ready for the resume argv (05-02) and the async capture/persist runtime (05-03)

---
*Phase: 05-session-resume-and-argv-regression-harde*
*Completed: 2026-07-07*
