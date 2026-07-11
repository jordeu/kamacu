---
phase: 04-gated-status-plugin-unlocks-waiting-and-
plan: "02"
subsystem: testing
tags: [opencode, e2e, build-tagged, real-binary, lifecycle, status-plugin]

requires:
  - phase: 04-gated-status-plugin-unlocks-waiting-and-
    provides: fetch-based status plugin (04-01)
provides:
  - Build-tagged real-opencode e2e harness proving the shipped plugin loads + posts reach a receiver
affects: [opencode-status-heuristics, release-confidence]

tech-stack:
  added: []
  patterns: [build-tagged-real-binary-e2e, env-gate-no-op-against-real-binary]

key-files:
  created:
    - internal/opencode/e2e_test.go
  modified: []

key-decisions:
  - "The e2e harness is build-tagged so it only runs when a real opencode binary is present (CI/local opt-in) — it does not burden the normal test suite"
  - "Proves three things end-to-end: the shipped fetch-based plugin loads in opencode 1.17.15, its POSTs reach a receiver (Stop observed → idle unlocks), and the env-gate no-ops against the real binary"

patterns-established:
  - "Real-binary e2e tests are build-tagged and opt-in; unit tests cover the default suite"

requirements-completed: []

coverage:
  - id: D1
    description: real-opencode e2e proving plugin loads + posts reach receiver + env-gate no-ops
    requirement: ""
    verification:
      - kind: e2e
        ref: "internal/opencode/e2e_test.go (build-tagged real-opencode harness)"
        status: pass
    human_judgment: false

duration: ~1h
completed: 2026-07-07
status: complete
---

# Plan 04-02: Real-opencode end-to-end lifecycle proof Summary

**A build-tagged real-opencode e2e harness proves the shipped fetch-based plugin loads in opencode 1.17.15, its POSTs reach a receiver (Stop observed → idle unlocks), and the env-gate no-ops against the real binary — without burdening the normal test suite.**

## Performance

- **Completed:** 2026-07-07
- **Tasks:** 1
- **Files created:** 1

## Accomplishments
- `internal/opencode/e2e_test.go` — build-tagged real-opencode e2e harness
- Proves the shipped fetch-based plugin loads in opencode 1.17.15
- Verifies POSTs reach a receiver (Stop observed → idle unlocks the waiting state)
- Confirms the env-gate no-ops against the real binary (not active in unrelated runs)

## Task Commits

1. **T02: build-tagged real-opencode e2e harness** — `c14d56f` (feat)

## Files Created/Modified
- `internal/opencode/e2e_test.go` — build-tagged real-binary e2e

## Decisions Made
- Build-tagged so it only runs with a real opencode binary present (opt-in); the default `go test ./internal/opencode/...` suite stays fast and dependency-free

## Deviations from Plan

None — work was already implemented and committed; this summary documents the delivered state.

## Issues Encountered
None

## Next Phase Readiness
- Gated status plugin fully proven end-to-end; waiting/idle unlock verified against a real opencode binary

---
*Phase: 04-gated-status-plugin-unlocks-waiting-and-*
*Completed: 2026-07-07*
