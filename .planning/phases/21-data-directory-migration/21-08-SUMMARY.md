---
phase: 21-data-directory-migration
plan: 08
subsystem: infra
tags: [migration, git-worktree, sqlite, path-rewrite, go]

# Dependency graph
requires:
  - phase: 21-data-directory-migration
    provides: "migrate.Complete Part-2 hook (rewriteManagedPaths -> repairWorktrees -> deleteOldTmuxRows) and its git-integration/no-git test harnesses"
provides:
  - "repairWorktrees broadened to EVERY project owning a migrated worktree (managed and folder-pointed), gated on under-new-root worktree ownership rather than the managed flag — folder-pointed repos' moved worktrees now survive a later git worktree prune"
  - "Go-side prefix-boundary rechecks on the LIKE-gated path rewrites (rewritePrefixColumn) and a hasRootPrefix separator boundary on the worktree_base setting rewrite — no path outside the moved root is ever corrupted or touched"
  - "Two regression tests: a CR-01 folder-pointed prune-survival git-integration test and WR-01/WR-02 no-git boundary tests"
affects: [phase-23-cleanup, data-directory-migration]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Under-root ownership gate lives on the WORKTREE path (worktree_path LIKE NewRoot||'/%' + Go-side HasPrefix recheck), not the project's managed flag — because provisionWorktree places worktrees under worktree_base for all project types"
    - "Every SQLite LIKE prefix gate is backed by a Go-side HasPrefix boundary recheck so a '_'/'%' home-path metacharacter can never drive a corrupted UPDATE"

key-files:
  created: []
  modified:
    - internal/migrate/paths.go
    - internal/migrate/paths_test.go
    - internal/migrate/worktree_repair_test.go

key-decisions:
  - "Repair gate = under-new-root worktree ownership, not managed flag: dropped both the managed=1 predicate AND the repo_path LIKE gate from repairWorktrees' project select (a folder-pointed repo_path is the external, unmoved checkout, not under NewRoot); moved the under-root filter onto the task worktree_path in existingWorktreePaths"
  - "Go-side HasPrefix recheck is the load-bearing WR-01 fix (LIKE ESCAPE left as optional defense-in-depth, not added) — one guard covers both the projects and tasks rewrites since both route through rewritePrefixColumn"
  - "hasRootPrefix (value == root || HasPrefix(value, root+\"/\")) replaces both bare HasPrefix cases in rewriteWorktreeBaseSetting, mirroring the DB rewrite's OldRoot||'/%' boundary discipline"

patterns-established:
  - "Pattern 1: SQLite LIKE prefix gates are always paired with a Go-side exact-or-separator-boundary recheck (p == root || HasPrefix(p, root+\"/\"))"
  - "Pattern 2: git worktree repair scope is derived from worktree placement (worktree_base), never from the project's managed flag"

requirements-completed: [MIGRATE-02, MIGRATE-03]

# Metrics
duration: 7min
completed: 2026-07-01
---

# Phase 21 Plan 08: Worktree-Repair Scope + Path-Rewrite Boundary Gap-Closure Summary

**Broadened `~/.kangent` → `~/.kamacu` worktree repair to folder-pointed (managed=0) repos so their moved worktrees survive a later `git worktree prune`, and boundary-checked the LIKE-gated path rewrites so a `_`/`%` home-path metacharacter or a sibling `~/.kangent-backup` dir can never be corrupted.**

## Performance

- **Duration:** 7 min
- **Started:** 2026-07-01T17:52:28Z
- **Completed:** 2026-07-01T17:59:08Z
- **Tasks:** 2 (both TDD: RED test → GREEN fix)
- **Files modified:** 3

## Accomplishments
- **CR-01 closed:** `repairWorktrees` now iterates ALL projects and repairs from each project's own `repo_path`; the under-moved-root gate moved onto the task `worktree_path` (`existingWorktreePaths`). A folder-pointed repo's migrated worktrees are repaired and survive a subsequent `git worktree prune` — no more silent sibling-worktree deregistration.
- **WR-01 closed:** a Go-side `oldPath == OldRoot || HasPrefix(oldPath, OldRoot+"/")` recheck in `rewritePrefixColumn` skips a LIKE metachar over-match instead of writing a `filepath.Join`-nested corrupted path (covers both the projects and tasks rewrites).
- **WR-02 closed:** new `hasRootPrefix` helper enforces an exact-or-separator boundary on the `worktree_base` setting rewrite, leaving a sibling `~/.kangent-backup` untouched while still rewriting the real `~/.kangent`.
- Full suite green: `go test ./...`, `go build ./...`, `go vet ./internal/migrate/`, `gofmt -l internal/migrate/` all clean; the pre-existing repair/scope/idempotency tests stayed green through the broadening.

## Task Commits

Each task was committed atomically (TDD: test → feat):

1. **Task 1 (RED): CR-01 folder-pointed repair regression** - `faa43ae` (test)
2. **Task 1 (GREEN): broaden repair to folder-pointed repos** - `069ec50` (feat)
3. **Task 2 (RED): WR-01/WR-02 boundary regressions** - `2fddbac` (test)
4. **Task 2 (GREEN): boundary-check the LIKE-gated rewrites** - `4a2145c` (feat)

_No REFACTOR commits — the GREEN implementations were already minimal and clean._

## Files Created/Modified
- `internal/migrate/paths.go` - Broadened `repairWorktrees` project select to all projects; added the under-NewRoot `worktree_path LIKE` gate + Go-side HasPrefix recheck to `existingWorktreePaths` (new `cfg Config` param); added the WR-01 guard in `rewritePrefixColumn`; added `hasRootPrefix` and applied it in `rewriteWorktreeBaseSetting`; updated the `repairWorktrees`/`existingWorktreePaths` doc comments.
- `internal/migrate/worktree_repair_test.go` - Added `TestCompleteRepairsFolderPointedWorktreesAndSurvivesPrune` (git-integration: external managed=0 repo, two under-root worktrees, rename, Complete, then a prune-survival assertion).
- `internal/migrate/paths_test.go` - Added `TestRewriteSkipsMetacharFalseMatch` (WR-01) and `TestRewriteWorktreeBaseRequiresSeparatorBoundary` (WR-02, two sub-cases: sibling untouched + real root still rewritten).

## Decisions Made
- **Under-root gate on the worktree path, not the managed flag.** Dropped `WHERE managed=1 AND repo_path LIKE ?` from `repairWorktrees`' select entirely; a folder-pointed repo's `repo_path` is the external checkout and is not under NewRoot, so any repo_path gate would wrongly exclude it. Scoping to migrated worktrees now happens in `existingWorktreePaths` via `worktree_path LIKE NewRoot||'/%'` plus a Go-side recheck.
- **Go-side recheck is the load-bearing WR-01 fix.** The optional LIKE `ESCAPE '\'` defense-in-depth was deliberately NOT added — the `HasPrefix` guard fully closes the corruption path and keeps the query text simple. One guard in `rewritePrefixColumn` covers both the projects and tasks rewrites since both route through it.
- Preserved the 21-06 per-path arg-array repair + `slog.Warn` log-and-skip tolerance and the `SetMaxOpenConns(1)` collect-then-query cursor discipline unchanged.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None. Both RED tests failed for exactly the predicted reasons (CR-01: worktrees left "prunable" then deregistered by prune; WR-01: `/home/john_doe/.kamacu/home/johnXdoe/.kangent/...` nesting; WR-02: `~/.kamacu-backup/...` mis-rewrite) and both GREEN fixes flipped them without regressing any pre-existing test.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- MIGRATE-02/MIGRATE-03 gap-closure complete: worktrees stay valid across the data-root move for ALL project types (managed and folder-pointed) with no prune-induced loss, and the path rewrite never touches a path outside the moved root.
- Deferred (optional, per 21-REVIEW.md, intentionally NOT closed here): WR-03 (frontend `migrateStorage.ts` index-mutation), IN-01 (repair ctx timeout), IN-02 (symlinked data root).

## Self-Check: PASSED

- `21-08-SUMMARY.md` present on disk.
- All task commits present in git history: `faa43ae` (RED CR-01), `069ec50` (GREEN CR-01), `2fddbac` (RED WR-01/WR-02), `4a2145c` (GREEN WR-01/WR-02), `21fb790` (summary).
- `STATE.md`/`ROADMAP.md` intentionally untouched (orchestrator owns those writes).

---
*Phase: 21-data-directory-migration*
*Completed: 2026-07-01*
