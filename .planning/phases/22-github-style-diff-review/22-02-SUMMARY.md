---
phase: 22-github-style-diff-review
plan: 02
subsystem: api
tags: [sqlite, goose, diff, database/sql, http, viewed-state]

# Dependency graph
requires:
  - phase: 22-01
    provides: "diff.File.Hash (rendered-diff sha256 fingerprint) + diff.File.Viewed (json field, default false)"
provides:
  - "diff_viewed keep-history table (migration 00011): composite PK (task_id, file_path, diff_hash) + FK ON DELETE CASCADE"
  - "GET /api/tasks/{id}/diff read-merge: File.Viewed set iff a row exists at the file's CURRENT rendered hash"
  - "PUT /api/tasks/{id}/diff/viewed write endpoint: validated, parameterized INSERT/DELETE toggle of the exact (task,path,hash) row"
affects: [22-03, github-style-diff-review-frontend]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Keep-history persistence keyed by (task, path, content-hash): row existence at the CURRENT hash means 'viewed'; a changed hash auto-resets, a reverted hash restores"
    - "Drain-then-close SELECT before any further work under SetMaxOpenConns(1) (icons.go discipline)"
    - "Trust-the-client-hash-on-write: no server re-Compute; a stale row is harmless keep-history, cascade-pruned on task delete"

key-files:
  created:
    - "internal/store/migrations/00011_diff_viewed.sql"
  modified:
    - "internal/api/diffs.go"
    - "internal/api/diffs_test.go"

key-decisions:
  - "Inline database/sql helpers in diffs.go (no new package) — tight footprint, matches house style"
  - "Composite PK doubles as the read index (leftmost-prefix task_id); no separate CREATE INDEX"
  - "Hash validated as exactly 64 lowercase hex; path bounded to <=4096 bytes and non-empty (T-22-03)"

patterns-established:
  - "Keep-history 'Viewed' table: content-hash-keyed rows, existence == viewed, no updates/backfill"
  - "Parameterized-only SQL (? placeholders) for all four statements — never string-concatenated (T-22-01)"

requirements-completed: [DIFF-03, DIFF-04]

# Metrics
duration: 7min
completed: 2026-07-02
---

# Phase 22 Plan 02: Per-File "Viewed" Persistence Summary

**Server-side keep-history per-file diff "Viewed" state: a SQLite table keyed by (task, path, rendered-diff hash) that survives restart (DIFF-03) and auto-resets when a file's rendered diff changes while restoring on revert (DIFF-04), merged onto GET diff and toggled via a validated PUT endpoint.**

## Performance

- **Duration:** 7 min
- **Started:** 2026-07-02T04:58:01Z
- **Completed:** 2026-07-02T05:05:00Z
- **Tasks:** 3
- **Files modified:** 3 (1 created, 2 modified)

## Accomplishments
- New `diff_viewed` keep-history table (goose migration 00011): composite PK `(task_id, file_path, diff_hash)` that enforces uniqueness AND serves as the read index; FK `task_id -> tasks(id) ON DELETE CASCADE` prunes rows on task delete.
- `GET /api/tasks/{id}/diff` now merges Viewed: one drained SELECT of the task's rows, cursor closed before touching `d.Files`, each `File.Viewed` set iff a row exists at that file's current rendered hash.
- `PUT /api/tasks/{id}/diff/viewed` write endpoint: validated (`?`-placeholder only) INSERT-on-true / DELETE-on-false of the exact `(task, path, hash)` row; 400 on empty/oversized path or non-64-hex hash; trusts the client hash (no re-Compute); responds 204.
- Go tests prove persistence, insert/delete toggle, keep-history auto-reset + revert-restore, input validation (400s), and FK-cascade pruning against a live `store.Open` DB.

## Task Commits

Each task was committed atomically:

1. **Task 1: 00011_diff_viewed goose migration** - `239fade` (feat)
2. **Task 2: Viewed read-merge on GET + PUT .../diff/viewed write endpoint** - `8146d0c` (feat)
3. **Task 3: Viewed read/write, keep-history, and FK-cascade tests** - `c1f5d68` (test)

## Files Created/Modified
- `internal/store/migrations/00011_diff_viewed.sql` - keep-history `diff_viewed` table; composite PK + FK cascade; goose Up/Down.
- `internal/api/diffs.go` - read-merge in `h.get` (drained SELECT → in-memory `(path,hash)` set → `File.Viewed`), new `h.setViewed` handler + `isDiffHash` validator, PUT route registration.
- `internal/api/diffs_test.go` - `TestViewedPersistToggleAndKeepHistory`, `TestViewedFKCascadeOnTaskDelete`, `TestViewedRejectsBadInput` plus `getDiffFile`/`putViewed` helpers.

## Decisions Made
- None beyond the plan's locked choices: inline `database/sql` helpers (no new package), composite PK as the sole index, trust-client-hash-on-write (no re-Compute), no startup backfill. All followed as specified.

## Deviations from Plan

None - plan executed exactly as written.

(Two cosmetic in-file comment rewordings were made so the acceptance-criteria greps counted only the intended DDL/route occurrences — `ON DELETE CASCADE`/`CREATE INDEX` in the migration and the `PUT .../diff/viewed` route literal in `diffs.go`. These are comment-text adjustments within the same tasks, not behavioral or scope changes.)

## Issues Encountered
None. All verification commands passed on the first functional run: `go build ./...`, `go vet ./...`, `go test ./internal/store/...`, `go test ./internal/api/...` (full package suite green, incl. `-run Viewed`).

## User Setup Required
None - no external service configuration required. Migration 00011 auto-applies at startup via the embedded goose runner (no new wiring).

## Next Phase Readiness
- Backend contract for the GitHub-style diff frontend is complete: GET ships `viewed` per file; PUT persists the toggle. Plan 22-03 (frontend file-tree / collapse / Viewed checkbox) can wire the checkbox to `PUT /api/tasks/{id}/diff/viewed` and re-read authoritative state on `onSettled`.
- Accepted carry-through limitation (from Plan 01): a binary file's hash is header-constant, so a binary marked Viewed will not reset on a byte change. Documented and intentional.

---
*Phase: 22-github-style-diff-review*
*Completed: 2026-07-02*
