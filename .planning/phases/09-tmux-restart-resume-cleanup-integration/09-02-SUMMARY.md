---
phase: 09-tmux-restart-resume-cleanup-integration
plan: 02
subsystem: settings
tags: [go, settings, duration, validation, react, tailwind, reaper]

# Dependency graph
requires:
  - phase: 06-settings
    provides: "settings KV seam (Keys/Defaults/Get/Set, Validate, data-driven GET /api/settings, SettingsField)"
provides:
  - "done_session_ttl global setting (default 24h, read-at-use) — the reaper's only config input"
  - "ParseDoneSessionTTL(value) (ttl, disabled, err) — shared parse+disable helper, single source of truth"
  - "Save-time validation rejecting garbage with canonical UI copy; empty/0/never/<=0 disable reaping"
  - "Done session TTL field rendered in a new Cleanup section on the settings page"
affects: [09-05-reaper, reaper, done-ttl, REAP-01]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Shared parse helper as single source of truth for validator + downstream consumer (mirrors AllowedShells shared by Validate + GET options)"
    - "New setting flows through the data-driven settings seam with zero type/API changes — only a const+default+Validate case+SettingsField"

key-files:
  created: []
  modified:
    - "internal/settings/settings.go"
    - "internal/settings/validate.go"
    - "internal/settings/validate_test.go"
    - "internal/settings/settings_test.go"
    - "web/src/pages/SettingsPage.tsx"

key-decisions:
  - "ParseDoneSessionTTL is the single source of truth for disable semantics, called by both Validate (save-time) and the 09-05 reaper (read-at-use) — they can never disagree"
  - "A Go-style duration <= 0 (e.g. 0s, -5m) is treated as disabled rather than parse-error or reap-everything: it can never expire"
  - "Validate rejects garbage with the canonical period-terminated copy mirrored verbatim by the frontend inline error"

patterns-established:
  - "Setting-with-downstream-consumer: add an exported parse helper alongside the Validate case so the consumer (reaper) imports one truth, not a re-implementation"

requirements-completed: [REAP-01]

# Metrics
duration: 4min
completed: 2026-06-13
---

# Phase 9 Plan 2: Done-TTL Setting Summary

**`done_session_ttl` global setting (Go-style duration, default 24h; empty/0/never disable) with a shared `ParseDoneSessionTTL` helper that is the single source of truth for both save-time validation and the 09-05 reaper, plus a Cleanup field on the settings page.**

## Performance

- **Duration:** 4 min
- **Started:** 2026-06-13T06:20:10Z
- **Completed:** 2026-06-13T06:24:04Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- Added `KeyDoneSessionTTL = "done_session_ttl"` with a `24h` default (REAP-01/D-91), read-at-use like every other setting — flows through the data-driven `GetAll`/`GET /api/settings` seam with no API or type change.
- Added `ParseDoneSessionTTL(value) (ttl time.Duration, disabled bool, err error)` — the shared parse+disable helper. Empty / `"0"` / `"never"` and any duration `<= 0` disable reaping; a valid positive duration returns `(d, false, nil)`; garbage returns a non-nil error.
- Wired `Validate(KeyDoneSessionTTL)` to call the helper and reject garbage with the canonical UI copy `Enter a duration like 24h, 90m, or 'never' to disable.` (period-terminated, mirrored verbatim by the frontend inline error).
- Rendered a "Done session TTL" field in a new **Cleanup** section on the settings page, with help text covering the disable path and the worktree-never-removed guarantee.

## Task Commits

Each task was committed atomically (TDD on Task 1):

1. **Task 1 (RED): failing tests for done_session_ttl parse + validate** - `d4790ac` (test)
2. **Task 1 (GREEN): done_session_ttl setting + ParseDoneSessionTTL helper** - `e43bf5a` (feat)
3. **Task 2: render Done session TTL field in settings Cleanup section** - `a500e81` (feat)

_No refactor commit — the GREEN implementation was already minimal._

## Files Created/Modified
- `internal/settings/settings.go` - Added `KeyDoneSessionTTL` const + `"24h"` Defaults entry (REAP-01/D-91).
- `internal/settings/validate.go` - Added `ParseDoneSessionTTL` helper + `case KeyDoneSessionTTL` in `Validate`; added `time` import.
- `internal/settings/validate_test.go` - `TestParseDoneSessionTTL` (parse/disable/error table) + `TestValidateDoneSessionTTL` (canonical-copy table).
- `internal/settings/settings_test.go` - Extended `TestDefaultsValues` guard `want` map with the new key (kept the exact-count assertion honest).
- `web/src/pages/SettingsPage.tsx` - `DONE_TTL_HELP` constant + new Cleanup section with a data-driven `SettingsField` for `done_session_ttl`.

## Decisions Made
- **`ParseDoneSessionTTL` as single source of truth** — both `Validate` (save-time) and the 09-05 reaper (read-at-use) call it, so the validator and reaper can never disagree about what "disabled" means. This mirrors the established `AllowedShells` pattern (shared by `Validate` and the GET options).
- **`<= 0` durations treated as disabled** (not error, not immediate reap) — a zero/negative window can never expire, so the safe interpretation is "reaping off." Covered by `0s` and `-5m` test cases.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Updated `TestDefaultsValues` guard for the new key**
- **Found during:** Task 1 (GREEN, running the full settings package test suite)
- **Issue:** `TestDefaultsValues` pins the exact `Defaults` map (`want` literal + `len == 4` assertion). Adding the 5th key (`done_session_ttl`) correctly broke it — the guard had to learn about the new key or the whole package test would fail.
- **Fix:** Added `settings.KeyDoneSessionTTL: "24h"` to the test's `want` map (the `len` assertion then matches automatically) and refreshed one stale "all four keys" comment to "every known key."
- **Files modified:** `internal/settings/settings_test.go`
- **Verification:** `go test ./internal/settings/ -count=1` exits 0; `go vet ./internal/settings/` exits 0.
- **Committed in:** `e43bf5a` (Task 1 GREEN commit)

---

**Total deviations:** 1 auto-fixed (1 in-scope guard-test update directly caused by the planned `Defaults` change)
**Impact on plan:** Necessary to keep the package test suite green; the guard test exists precisely to flag `Defaults` map changes. No scope creep.

## Issues Encountered
None — both verifications (`go test`/`go vet`, `tsc`/`npm run build`) passed. The Vite "chunk larger than 500 kB" warning is pre-existing and unrelated to this change.

## Known Stubs
None. The setting is fully wired end to end: the key flows through the data-driven settings seam to the UI, and `ParseDoneSessionTTL` implements the complete disable semantics. The reaper that consumes it is intentionally out of scope (09-05) — this plan is the isolated Wave-1 config input by design.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- The 09-05 reaper can read its window with `v, _ := settings.Get(db, settings.KeyDoneSessionTTL); ttl, disabled, _ := settings.ParseDoneSessionTTL(v)` — no further wiring, no risk of disagreeing with save-time validation.
- The user can already configure the TTL from the settings page (Cleanup section).
- No blockers.

## Self-Check: PASSED

- All 5 modified files exist on disk.
- SUMMARY.md created.
- All 3 task commits present in git history (`d4790ac`, `e43bf5a`, `a500e81`).

---
*Phase: 09-tmux-restart-resume-cleanup-integration*
*Completed: 2026-06-13*
