---
phase: 12-open-a-review
plan: 02
subsystem: api
tags: [go, sqlite, kanban, board-leak, github-pr, source-discriminator]

# Dependency graph
requires:
  - phase: 10-github-foundations
    provides: "migration 00007 tasks.source/pr_number/pr_base_ref columns (CHECK manual/github_pr, default manual)"
provides:
  - "source/pr_number/pr_base_ref on the Task JSON wire shape (frontend branches on source)"
  - "GHREV-04 board-leak guard: a source='github_pr' row never appears on the board, never receives a board position (/move 409), and never perturbs manual positioning math"
  - "column-aligned scanTask AND loadTaskRepo for the extended taskColumns"
affects: [12-03-diff-base, 12-04-open-endpoint, 12-05-frontend, phase-13-cleanup]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "source='manual' filter on EVERY board/position query; by-id queries left unfiltered so the review view can deep-link its own PR row"
    - "defense-in-depth 409 guard in /move (source != 'manual') paired with the position-query filters"

key-files:
  created: []
  modified:
    - internal/api/tasks.go
    - internal/api/worktrees.go
    - internal/api/tasks_test.go

key-decisions:
  - "Five board/position queries gain AND source = 'manual' (RESEARCH §4 #1-5); the /move mover-load gains a source != 'manual' 409 guard (#6); by-id get/update/delete/afterPosition stay unfiltered"
  - "loadTaskRepo (worktrees.go) is the one cross-file scanner of taskColumns — its manual Scan gained the 3 new targets before &repo to stay column-aligned"

patterns-established:
  - "Per-query board-leak verdict: filter only the queries that treat tasks as board cards; never filter by-id lookups (the review view legitimately fetches its own PR row by id)"

requirements-completed: [GHREV-04]

# Metrics
duration: 7min
completed: 2026-06-14
---

# Phase 12 Plan 02: Board-Leak Guard + source Wire Shape Summary

**A source='github_pr' task can never render as a kanban card, never get a board position (/move 409), and never perturb a manual task's position — enforced at the data layer with `AND source = 'manual'` on five board/position queries plus a 409 mover-load guard; the Task JSON now carries `source`/`pr_number`/`pr_base_ref`.**

## Performance

- **Duration:** ~7 min
- **Started:** 2026-06-14T05:36:56Z
- **Completed:** 2026-06-14T05:44:09Z
- **Tasks:** 2 (both TDD)
- **Files modified:** 3

## Accomplishments
- Extended the Task wire shape with `source`/`pr_number`/`pr_base_ref` (taskColumns + struct + scanTask) and kept `loadTaskRepo`'s manual scan column-aligned — the one cross-file consequence of touching `taskColumns`.
- Applied RESEARCH §4 verbatim: `AND source = 'manual'` on listByProject (board list), create top-of-ToDo, move drop-at-top, nextPosition, and renumberColumn.
- Added a defense-in-depth `/move` 409 guard: the mover-load now selects `source` and rejects `source != 'manual'` with `409 "PR reviews are not board tasks"` before any position math.
- Locked the milestone's single highest-severity regression behind two dedicated tests (board absence + /move 409 + positioning purity, and create-MIN exclusion) — proven RED before the fix, GREEN after.

## Task Commits

Each task was committed atomically (TDD: test → feat):

1. **Task 1: Add source/pr_number/pr_base_ref to the Task wire shape**
   - `9d3529d` (test) — failing wire-shape round-trip test
   - `a5c8a75` (feat) — taskColumns + Task struct + scanTask + loadTaskRepo alignment
2. **Task 2: Board-leak filters on 5 queries + /move 409 guard**
   - `b596076` (test) — failing board-leak regression test
   - `f0bae70` (feat) — five `source = 'manual'` filters + /move 409 guard

_No REFACTOR commits — both implementations were minimal and clean._

## Files Created/Modified
- `internal/api/tasks.go` — `taskColumns`/`Task`/`scanTask` extended; five board/position queries filtered by `source = 'manual'`; `/move` mover-load selects `source` and 409-guards non-manual movers.
- `internal/api/worktrees.go` — `loadTaskRepo`'s manual scan gained `&t.Source, &t.PRNumber, &t.PRBaseRef` before `&repo` (column alignment for the extended `taskColumns`).
- `internal/api/tasks_test.go` — `insertPRRow` helper; `TestTaskJSONIncludesSourceFields`, `TestPRReviewNeverLeaksToBoard`, `TestPRReviewExcludedFromCreatePositioning`.

## Decisions Made
- **By-id queries deliberately left unfiltered** (get/update/delete/afterPosition): the review view legitimately fetches its own PR row by id (deep-link), so a `source` filter there would break the open path. Only the queries that treat tasks as *board cards* are filtered. This matches RESEARCH §4's per-query verdict table exactly.
- **Defense-in-depth, not either/or:** the `/move` 409 guard AND the position-query filters both ship. The guard guarantees a PR row can never receive a position even via a hand-crafted request; the filters guarantee the math never *sees* a PR row (so a co-existing PR row at any position can't poison MIN/nextPosition/renumber).

## Deviations from Plan

None - plan executed exactly as written. The five filters, the guard, and both scanners landed verbatim per RESEARCH §4; the negative-guard acceptance criteria (by-id queries unchanged) were verified by grep.

## Issues Encountered
None. Both RED phases failed for exactly the predicted reasons (wire fields absent; PR row leaked to board + /move 200 instead of 409 + create MIN poisoned to -6.0), and both GREEN phases passed on the first implementation pass. `grep -c "source = 'manual'"` returns exactly 5. Full `go test ./...` is green (no scan-arity or query regression anywhere, including worktrees/diffs/sessions which share `taskColumns`).

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- **12-03 (diff base):** can now read `source`/`pr_base_ref` from the row to branch the diff base. (12-03 extends `diffs.go`'s SELECT itself; it does not depend on `taskColumns` carrying these — but the `Task` struct now has the fields if needed.)
- **12-04 (open endpoint):** the find-or-create path returns the full `Task` row, which now carries the `source` discriminator the frontend needs; `pr_number`/`pr_base_ref` round-trip through `scanTask` for free.
- **12-05 (frontend):** `task.source === 'github_pr'` is now available on the wire to gate the read-only title / PR meta line / read-only Description deltas.
- No blockers. The board-leak regression (the milestone's highest-severity risk) is locked at the data layer with a dedicated, grep-verifiable test.

## Self-Check: PASSED

All claimed files exist (internal/api/tasks.go, internal/api/worktrees.go, internal/api/tasks_test.go, 12-02-SUMMARY.md) and all four task commits (9d3529d, a5c8a75, b596076, f0bae70) are present in git history. `go test ./...` green; `grep -c "source = 'manual'" internal/api/tasks.go` == 5.

---
*Phase: 12-open-a-review*
*Completed: 2026-06-14*
