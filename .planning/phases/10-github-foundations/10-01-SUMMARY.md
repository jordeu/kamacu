---
phase: 10-github-foundations
plan: 01
subsystem: database
tags: [sqlite, goose, migrations, settings, github]

# Dependency graph
requires:
  - phase: 09-tmux-restart-resume-cleanup-integration
    provides: migration 00006 (task status timestamps) — the latest schema version 00007 builds on
provides:
  - "migration 00007: projects.description, projects.github_repo, tasks.source/pr_number/pr_base_ref columns"
  - "github_integration settings KV key (code default 'on', on/off validation)"
affects: [10-02-project-link, 10-03-frontend, 11-pr-review-column, 12-pr-tasks, 13-pr-cleanup]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Migration: one ALTER...ADD COLUMN per statement (modernc/SQLite), Down drops in reverse order"
    - "Settings: absent-row=code-default; new KV key needs no migration, only a Defaults entry"

key-files:
  created:
    - internal/store/migrations/00007_github_foundations.sql
  modified:
    - internal/settings/settings.go
    - internal/settings/validate.go
    - internal/settings/validate_test.go
    - internal/settings/settings_test.go

key-decisions:
  - "github_integration default 'on' (D-01): absent settings row reads as 'on' for free via the existing Get fallback — no Get/GetAll/Set change needed"
  - "All five Phase-10/12 columns land in one migration (00007) so Phase 12 needs no further migration"
  - "tasks.source CHECK (source IN ('manual','github_pr')) defaults 'manual' — board excludes PRs via WHERE source='manual' in Phase 12"

patterns-established:
  - "Settings on/off toggle validation mirrors the KeyShell membership-check shape; canonical error copy 'Choose on or off.'"

requirements-completed: [GHSET-01]

# Metrics
duration: 7min
completed: 2026-06-13
---

# Phase 10 Plan 01: GitHub Foundations Summary

**Migration 00007 adds the five v1.3 GitHub schema columns (projects.description/github_repo, tasks.source/pr_number/pr_base_ref) and the github_integration settings KV key defaulting to "on" with on/off validation.**

## Performance

- **Duration:** ~7 min
- **Started:** 2026-06-13T14:09:02Z
- **Completed:** 2026-06-13T14:16:00Z
- **Tasks:** 2
- **Files modified:** 5 (1 created, 4 modified)

## Accomplishments
- Migration 00007 lands the v1.3 data substrate in a single migration: `projects.description` (NOT NULL DEFAULT ''), `projects.github_repo` (nullable, NULL = not linked), and the three Phase-12 task columns `tasks.source` (CHECK manual/github_pr, default 'manual'), `tasks.pr_number`, `tasks.pr_base_ref` — so Phase 12 needs no further migration.
- `github_integration` is now a validated settings key defaulting to "on"; an absent row reads as "on" with no code change to Get/GetAll/Set.
- Validation accepts only the lowercase literals "on"/"off" (case-sensitive), rejecting everything else with the canonical UI-SPEC copy "Choose on or off."

## Task Commits

Each task was committed atomically:

1. **Task 1: Write migration 00007 (project + task columns)** - `bd1ec2a` (feat)
2. **Task 2 (TDD RED): add failing test for github_integration validation** - `512ec89` (test)
3. **Task 2 (TDD GREEN): add github_integration key + on/off validation** - `1787120` (feat)

**Plan metadata:** committed separately (docs: complete plan)

## Files Created/Modified
- `internal/store/migrations/00007_github_foundations.sql` - Created. The 5 GitHub-foundation columns; one ALTER...ADD COLUMN per statement, Down drops in reverse order.
- `internal/settings/settings.go` - Added `KeyGithubIntegration` const + `Defaults["github_integration"] = "on"`.
- `internal/settings/validate.go` - Added the `KeyGithubIntegration` on/off validation case ("Choose on or off.").
- `internal/settings/validate_test.go` - Added `TestValidateGithubIntegration` (on/off valid; yes/empty/ON rejected).
- `internal/settings/settings_test.go` - Updated `TestDefaultsValues` to include the new default (see Deviations).

## Decisions Made
- Followed the plan exactly for the migration shape and the settings key/validation. The absent-row="on" behavior is covered for free by the existing `Get`/`TestGetReturnsDefaultOnFreshDB` iteration over `Defaults`, so no dedicated Get test was needed beyond extending the canonical defaults assertion.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Updated TestDefaultsValues to include the new default key**
- **Found during:** Task 2 (GREEN step)
- **Issue:** The existing `TestDefaultsValues` in `internal/settings/settings_test.go` hard-codes the complete expected `Defaults` map and asserts `len(Defaults) == len(want)`. Adding the 6th key (`github_integration`) made the full Go suite fail with `Defaults has 6 keys, want 5`.
- **Fix:** Added `settings.KeyGithubIntegration: "on"` to the test's `want` map. This is the canonical assertion of the Defaults contract; it must track every key. This also strengthens coverage of the plan's stated behavior "Get returns 'on' on a fresh DB" via the `TestGetReturnsDefaultOnFreshDB` loop over `Defaults`.
- **Files modified:** internal/settings/settings_test.go
- **Verification:** `go test ./internal/settings/...` and `go test ./...` both exit 0.
- **Committed in:** `1787120` (Task 2 GREEN commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** The single deviation was a test-contract update mechanically required by adding a new Defaults key (the plan's own intent). No scope creep; no production behavior beyond the plan.

## Issues Encountered
- None beyond the deviation above. The migration applied cleanly on every fresh-DB test harness (visible in the goose log: `successfully migrated database to version: 7`).

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- The GitHub schema substrate is in place: Plans 02 (project link/PATCH) and 03 (frontend) can read `projects.description`/`github_repo` and the `github_integration` setting. Phase 12 can read `tasks.source`/`pr_number`/`pr_base_ref` with no further migration.
- No `scanProject`/`scanTask` changes were made in this plan (by design) — the columns simply exist and don't break existing SELECTs (verified by the full API suite passing). Plan 02 owns wiring `description`/`github_repo` into the project read/write path.
- Reminder for Phase 12 (carried from STATE.md): the board-leak regression — every `SELECT ... FROM tasks` must add the `source='manual'` filter. This plan only adds the column; no SELECT consumes it yet.

## Self-Check: PASSED

All created/modified files verified present on disk; all three task commits (`bd1ec2a`, `512ec89`, `1787120`) verified in git history.

---
*Phase: 10-github-foundations*
*Completed: 2026-06-13*
