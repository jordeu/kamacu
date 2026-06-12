---
phase: 08-tmux-shells-spawn-detach-lifecycle
plan: 02
subsystem: settings
tags: [go, tmux, lookpath, settings, validation]

# Dependency graph
requires:
  - phase: 06-global-settings
    provides: "shell setting key, AllowedShells single source of truth, server-driven options array (SHELL-FUT-01 seam)"
provides:
  - "settings.AllowedShells() — call-time function offering tmux only while exec.LookPath('tmux') resolves (TMUX-01)"
  - "GET /api/settings shell options and save-time validation sharing one LookPath truth"
affects: [08-03, 08-04, tmux-spawn, settings]

# Tech tracking
tech-stack:
  added: []
  patterns: ["call-time capability check: conditional setting options derive from exec.LookPath at request time, not startup"]

key-files:
  created: []
  modified:
    - internal/settings/validate.go
    - internal/settings/validate_test.go
    - internal/api/settings.go
    - internal/api/settings_test.go

key-decisions:
  - "AllowedShells converted var->func with LookPath checked at call time, so tmux install/uninstall is reflected without a server restart"
  - "PATH-present test assertions skip on tmux-less hosts; PATH-scrubbed assertions run unconditionally via t.Setenv"

patterns-established:
  - "Conditional shell offering: dropdown options and save acceptance must always derive from the same function call (never cache the slice)"

requirements-completed: [TMUX-01]

# Metrics
duration: 8min
completed: 2026-06-12
---

# Phase 8 Plan 02: Conditional tmux Shell Offering Summary

**settings.AllowedShells is now a call-time LookPath function: the shell dropdown offers "tmux" only while the binary resolves on PATH, and save-time validation tracks the exact same truth — zero frontend changes**

## Performance

- **Duration:** 8 min
- **Started:** 2026-06-12T21:54:46Z
- **Completed:** 2026-06-12T22:03:03Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- `var AllowedShells = []string{"bash"}` replaced with `func AllowedShells() []string` that appends "tmux" when `exec.LookPath("tmux")` succeeds — checked per call, so install/uninstall is reflected without a restart
- Both consumers updated against the shared function: `Validate` KeyShell arm (save-time, verbatim "Unknown shell." copy preserved) and `entryFor` (GET /api/settings options array)
- Both PATH states tested at the validation layer (`TestAllowedShellsWithTmuxOnPath`, `TestAllowedShellsWithPathScrubbed`) and at the HTTP layer (`TestSettingsShellOptionsTrackPath`: tmux offered AND accepted with PATH present; option vanishes AND PUT shell=tmux returns 400 "Unknown shell." with PATH scrubbed)
- Zero frontend diffs — the dropdown already renders the server-driven options array (SHELL-FUT-01 seam paid off exactly as designed)

## Task Commits

Each task was committed atomically:

1. **Task 1: Convert AllowedShells to a call-time LookPath function** - `68052f2` (feat)
2. **Task 2: Tests for conditional tmux offering** - `c804c5c` (test)

_Note: Task 2 was flagged tdd="true"; the implementation landed in Task 1 per the plan's structure, so the pre-existing failing API tests (options hardcoded to ["bash"] on a tmux-equipped host) served as the RED state, made green by the new PATH-truth assertions._

## Files Created/Modified

- `internal/settings/validate.go` - `AllowedShells()` function (var converted), `Validate` KeyShell arm calls it
- `internal/api/settings.go` - `entryFor` calls `settings.AllowedShells()` ("Same function as save-time validation — one source of truth.")
- `internal/settings/validate_test.go` - PATH-present (skip without tmux) and PATH-scrubbed (t.Setenv) coverage; exact "Unknown shell." copy asserted
- `internal/api/settings_test.go` - `TestSettingsShellOptionsTrackPath` end-to-end; existing options assertions now compare against `AllowedShells()` PATH truth via new `optionStrings` helper

## Decisions Made

- Existing API tests that hardcoded options == ["bash"] were rewritten to assert against `settings.AllowedShells()` directly, keeping them deterministic on any host while still pinning "bash" first
- PATH-scrubbed HTTP test also asserts the PUT rejection copy verbatim, proving the dropdown vanishing and the save rejection track the same LookPath call

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Existing test referenced the old var shape, breaking compilation**
- **Found during:** Task 1 (var-to-func conversion)
- **Issue:** `internal/settings/validate_test.go:75` used `len(settings.AllowedShells)` on the former slice, failing `go vet` after the conversion
- **Fix:** Minimal compile fix asserting `AllowedShells()` returns "bash" first regardless of PATH state; full PATH-state coverage followed in Task 2 as planned
- **Files modified:** internal/settings/validate_test.go
- **Verification:** `go build ./... && go vet ./internal/settings/ ./internal/api/` green
- **Committed in:** 68052f2 (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Trivial compile fix the plan implicitly required (Task 1 verify includes vet); no scope creep.

## Issues Encountered

- Two pre-existing API tests (`TestSettingsGetAllDefaults`, `TestSettingsPutShellResponseIncludesOptions`) failed on this tmux-equipped host immediately after Task 1 because they asserted options exactly ["bash"]. Resolved in Task 2 by comparing against `AllowedShells()` — this was the plan's anticipated "verify no existing test breaks" work.

## User Setup Required

None - no external service configuration required. (tmux 3.x on PATH is the runtime prerequisite the feature itself detects.)

## Next Phase Readiness

- TMUX-01 complete: the dropdown and validation share one LookPath truth; `curl -s localhost:7333/api/settings | jq '.shell.options'` includes "tmux" on this host
- Honest edge in place for 08-04: an already-stored `shell=tmux` row still flows to spawn after a tmux uninstall, where D-84's honest spawn error fires
- Full `go test ./... -count=1` green — no other package consumed the old var

---
*Phase: 08-tmux-shells-spawn-detach-lifecycle*
*Completed: 2026-06-12*

## Self-Check: PASSED

- All 4 modified files + SUMMARY exist on disk
- Task commits 68052f2, c804c5c present in git log
- Key link verified: internal/api/settings.go calls settings.AllowedShells()
