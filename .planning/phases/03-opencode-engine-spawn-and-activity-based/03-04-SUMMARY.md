---
phase: 03-opencode-engine-spawn-and-activity-based
plan: "04"
subsystem: api
tags: [opencode, engine-wiring, custom-render-exemption, e2e, session-id, migration]

requires:
  - phase: 03-opencode-engine-spawn-and-activity-based
    provides: opencode spawn branch + resume argv (03-03)
provides:
  - sessions.go engine wiring (opencode exempted from custom-render)
  - tasks.opencode_session_id column (migration 00016) for resume
  - real-opencode e2e harness (build-tagged)
affects: [session-resume, opencode-integration]

tech-stack:
  added: []
  patterns: [opencode-exempt-from-custom-render, build-tagged-real-binary-e2e]

key-files:
  created:
    - internal/store/migrations/00016_opencode_session_id.sql
    - internal/store/opencode_session_id_test.go
    - internal/opencode/e2e_test.go
  modified:
    - internal/api/sessions.go

key-decisions:
  - "opencode is exempted from the custom-render path (renderAgentCommand) — it gets its own engine branch in sessions.go, not the tokenize-substitute custom path"
  - "tasks.opencode_session_id (nullable, migration 00016) persists the opaque opencode session id for resume — opencode has NO transcript files (sessions live in opencode.db)"
  - "real-opencode e2e harness is build-tagged so it only runs when a real opencode binary is present; it proves the shipped fetch-based plugin loads in opencode 1.17.15 and its POSTs reach a receiver"

patterns-established:
  - "opencode session identity = persisted opaque id; claude session identity = transcript glob — never mixed"

requirements-completed: []

coverage:
  - id: D1
    description: sessions.go engine wiring (opencode exempt from custom-render) + integration
    requirement: ""
    verification:
      - kind: integration
        ref: "internal/api/opencode_resume_test.go + internal/store/opencode_session_id_test.go"
        status: pass
      - kind: e2e
        ref: "internal/opencode/e2e_test.go (build-tagged real-opencode harness)"
        status: pass
    human_judgment: false

duration: ~1h
completed: 2026-07-07
status: complete
---

# Plan 03-04: sessions.go engine wiring + integration Summary

**sessions.go wires the opencode engine as its own branch (exempt from custom-render), migration 00016 adds tasks.opencode_session_id for resume identity, and a build-tagged real-opencode e2e harness proves the shipped plugin loads and its POSTs reach a receiver.**

## Performance

- **Completed:** 2026-07-07
- **Tasks:** 1
- **Files created:** 3
- **Files modified:** 1

## Accomplishments
- sessions.go opencode engine branch — exempt from `renderAgentCommand` (the custom tokenize-substitute path); opencode gets its own argv construction
- `tasks.opencode_session_id` nullable column (migration 00016) — persists the opaque opencode session id; opencode has no transcript files so resume keys off this id alone
- `internal/opencode/e2e_test.go` — build-tagged real-opencode e2e harness: proves the fetch-based plugin loads in opencode 1.17.15, Stop observed → idle unlocks, env-gate no-ops against the real binary

## Task Commits

1. **Engine-branched opencode resume argv + sessions wiring** — `e9fc375` (feat)
2. **tasks.opencode_session_id column** — `785a71d` (test) — migration 00016
3. **real-opencode e2e harness** — `c14d56f` (feat)

## Files Created/Modified
- `internal/store/migrations/00016_opencode_session_id.sql` — nullable session-id column
- `internal/store/opencode_session_id_test.go` — upgrade + fresh-install migration tests
- `internal/opencode/e2e_test.go` — build-tagged real-opencode e2e
- `internal/api/sessions.go` — opencode engine branch (custom-render exemption + resume wiring)

## Decisions Made
- opencode exempted from custom-render because it is a first-party engine with its own argv contract, not a user template
- Resume identity per-engine: opencode uses persisted opaque id; claude uses transcript glob — never conflated

## Deviations from Plan

None — work was already implemented and committed; this summary documents the delivered state.

## Issues Encountered
None

## Next Phase Readiness
- opencode engine fully wired (seed → status → spawn → resume → e2e); ready for the gated status plugin (Phase 04) which unlocks waiting/idle

---
*Phase: 03-opencode-engine-spawn-and-activity-based*
*Completed: 2026-07-07*
