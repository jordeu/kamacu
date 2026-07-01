---
phase: 21-data-directory-migration
plan: 03
subsystem: infra
tags: [migration, sqlite, git-worktree, tmux, startup-hook, idempotent, go]

# Dependency graph
requires:
  - phase: 21-data-directory-migration
    provides: "internal/migrate Part-1 (Prepare) + the Config/Decision contract — the dir has already moved and the DB file renamed when Complete runs"
provides:
  - "internal/migrate Part-2 hook: Complete(ctx, db, cfg) — rewrite managed DB paths (D-15), git worktree repair, delete kangent-* tmux rows (D-07/D-08)"
  - "rewriteManagedPaths (prefix-swap, managed/under-root only), repairWorktrees (os.Stat-filtered arg-array repair), deleteOldTmuxRows"
affects: [21-04-main-wiring]

# Tech tracking
tech-stack:
  added: []  # no new dependencies — stdlib (database/sql, os/exec, path/filepath, strings) + existing internal packages + system git
  patterns:
    - "Derived-consistency self-gating (LIKE OldRoot||'/%' stops matching once rewritten; repair is a no-op on a healthy tree; DELETE empties once) — idempotent D-04 roll-forward with no marker file"
    - "Prefix swap (strings.TrimPrefix + filepath.Join), NEVER a global strings.Replace — the D-15 data-loss guard"
    - "Collect-then-update + collect-repos-then-query-tasks: at most one DB cursor open at a time under SetMaxOpenConns(1)"
    - "os.Stat-filtered arg-array `git worktree repair <new-paths...>` — explicit new paths (bare repair is a no-op when both endpoints move), stale paths filtered so git never exits 1"

key-files:
  created:
    - "internal/migrate/paths.go"
    - "internal/migrate/paths_test.go"
    - "internal/migrate/worktree_repair_test.go"
  modified: []

key-decisions:
  - "Managed-path rewrite gates on `managed=1 AND repo_path LIKE OldRoot||'/%'` and is a pure prefix swap — an unmanaged project at /home/u/dev/kangent-thing is provably byte-for-byte untouched (D-15)"
  - "worktree_base setting rewrite handles BOTH the raw '~/.kangent' form and the expanded OldRoot form, preserving the exact suffix (incl. trailing slash) via string prefix swap, not filepath.Join"
  - "repairWorktrees passes the explicit NEW worktree paths, os.Stat-filtered — the live-verified fix for RESEARCH Pitfall 2 (bare repair no-ops when repo+worktrees move together; a stale path exits 1)"
  - "All git exec is an arg-array `git -C <repo> worktree repair ...` (ASVS V5 / worktree.go invariant), never `sh -c`"

patterns-established:
  - "Complete = rewriteManagedPaths -> repairWorktrees -> deleteOldTmuxRows, first-error return; every step self-gates so re-running is safe (D-04)"
  - "Git-integration test builds a real repos/+worktrees/ layout, renames the root, and asserts `worktree list` clean + `git status` in the moved worktree; skip-if-no-git + t.Setenv config isolation"

requirements-completed: [MIGRATE-02, MIGRATE-03]

# Metrics
duration: 8min
completed: 2026-07-01
---

# Phase 21 Plan 03: Path Rewrite + Worktree Repair Summary

**`internal/migrate` Part-2 hook `Complete`: rewrite the DB's managed-only stored paths under the old root (D-15 prefix swap), `git worktree repair` each managed repo with os.Stat-filtered new paths, and delete the retired `kangent-*` tmux rows — every step self-gating so it is idempotent and safe to re-run on the D-04 roll-forward path.**

## Performance

- **Duration:** ~8 min
- **Tasks:** 2 (both TDD: RED test commit then GREEN feat commit)
- **Files:** 3 created

## Accomplishments

- **Managed-path rewrite (D-15, MIGRATE-02)** — `rewriteManagedPaths` prefix-swaps `projects.repo_path` (gated `managed=1 AND repo_path LIKE OldRoot||'/%'`), `tasks.worktree_path` (`LIKE OldRoot||'/%'`), and the raw `worktree_base` setting from the old data root to the new one. It is a pure `strings.TrimPrefix` + `filepath.Join` prefix swap — never a global `strings.Replace` — so a user's external repo living at e.g. `/home/u/dev/kangent-thing` (managed=0, outside the old root) is provably byte-for-byte untouched (test-asserted). Collect-then-update discipline honors `SetMaxOpenConns(1)` (close the SELECT cursor before any UPDATE); all SQL uses `?` placeholders.
- **git worktree repair driver (MIGRATE-02)** — `repairWorktrees` runs after the rewrite: for each managed repo (now under the new root), it gathers that repo's tasks' new `worktree_path` values, `os.Stat`-filters user-deleted trees, and runs one `git -C <newRepoPath> worktree repair <p1> <p2> ...` via an arg-array `exec.CommandContext` (never a shell). Passing the explicit new paths is the live-verified fix for RESEARCH Pitfall 2 (a bare `repair` is a no-op when the repo and its worktrees move together); the `os.Stat` filter keeps git from ever exiting 1 on a stale path.
- **tmux-row cleanup (D-07/D-08, MIGRATE-03)** — `deleteOldTmuxRows` runs `DELETE FROM tmux_sessions WHERE name LIKE 'kangent-%'` (reaper.go idiom) so a reopened task respawns a fresh shell on the new `-L kamacu` socket rather than reattaching to a dead `kangent-*` name; a `kamacu-*` row is left alone.
- **`Complete` orchestration + idempotency** — the exported hook chains the three steps with first-error return. Every step self-gates on observable state (the `LIKE` gate stops matching once rewritten; `repair` on a healthy tree is a clean exit-0 no-op; the `DELETE` empties once), so `Complete` is safe to re-run on the roll-forward path with no marker file — the git-integration test runs it twice and asserts the second call is a clean no-op.

## Task Commits

Each TDD task produced a RED (test) then GREEN (feat) commit:

1. **Task 1: Managed-path rewrite (D-15) + tmux-row cleanup**
   - `6fd5c38` test(21-03): add failing managed-path rewrite + tmux-row cleanup tests
   - `5ea680c` feat(21-03): implement managed-path rewrite + tmux-row cleanup
2. **Task 2: git worktree repair driver + Complete orchestration**
   - `e70b7c1` test(21-03): add failing git worktree repair + Complete integration tests
   - `e022331` feat(21-03): implement git worktree repair driver + Complete orchestration

_No REFACTOR commits were needed (code was clean at GREEN)._

## Files Created/Modified

- `internal/migrate/paths.go` (258 lines) — `Complete` (exported hook), `rewriteManagedPaths` + `rewritePrefixColumn` + `rewriteWorktreeBaseSetting`, `deleteOldTmuxRows`, `repairWorktrees` + `existingWorktreePaths`.
- `internal/migrate/paths_test.go` (240 lines) — no-git DB tests: `TestRewriteManagedPaths`, `TestPathScopeUnmanagedUntouched` (D-15 guard, exact-string compare), `TestRewriteIdempotent` (run twice), `TestRewriteAlreadyUnderNewRootLeftAlone` (self-gate no-match), `TestDeleteOldTmuxRows` (kamacu-* survives).
- `internal/migrate/worktree_repair_test.go` (175 lines) — git-integration (skip-if-no-git, `t.Setenv` config isolation): `TestCompleteRepairsMovedWorktrees` (real repos/+worktrees/ layout, rename the root, assert `worktree list` clean + `git status` in the moved worktree succeeds + tmux row deleted + second Complete is a no-op) and `TestCompleteStalePathFiltered` (a deleted worktree dir is os.Stat-filtered out; Complete still succeeds and the valid tree repairs).

## Decisions Made

- **Managed-flag + old-root-prefix gate, never a blind replace (D-15):** the rewrite matches only `managed=1` rows whose `repo_path` is `LIKE OldRoot||'/%'` (and `tasks.worktree_path` under the old root), then swaps the prefix in Go. This mirrors `projects.go:511` ("the managed flag is the SINGLE source of truth; never derive it from the path — data-loss hazard"). An unmanaged/outside-root row is provably untouched — the load-bearing test asserts an exact-string compare.
- **`worktree_base` handles both storage forms:** the default is stored RAW (`~/.kangent/worktrees/`), but the rewrite also matches the expanded `cfg.OldRoot` form, using a string prefix swap (not `filepath.Join`) so the exact suffix incl. the trailing slash is preserved. A missing row (the live install has none) is a clean no-op.
- **Explicit os.Stat-filtered repair paths (Pitfall 2):** bare `git worktree repair` is a no-op when the repo and its worktrees move together, so the new worktree paths are passed explicitly; a stale path would make git exit 1, so each is `os.Stat`-filtered first (`projects.go:576` idiom). Repairing a healthy tree is idempotent.
- **Cursor discipline under `SetMaxOpenConns(1)`:** the managed repos are collected and the SELECT cursor closed BEFORE the per-repo task queries run, so at most one DB cursor is ever open at a time — the same single-writer rule `BackfillProjectIcons` follows.

## Deviations from Plan

None - plan executed exactly as written. (The git-integration test isolates the host git config with per-test `t.Setenv("GIT_CONFIG_GLOBAL"/"GIT_CONFIG_SYSTEM", os.DevNull)` — inherited by the package's own `worktree repair` exec — rather than adding a second `TestMain`, since the package already has one in `migrate_test.go`. This is standard hermetic-test infrastructure within the plan's `worktree_repair_test.go` file, not a scope change.)

## Issues Encountered

None. All 18 package tests pass (7 pre-existing Part-1 + 5 new no-git + 2 new git-integration, plus the gate/string table tests); `go vet ./internal/migrate/`, `go build ./...`, and `gofmt` are clean; the full repo suite (`go test ./...`, 13 packages) is green.

## Threat Surface Scan

No new security surface beyond the plan's `<threat_model>`. All mitigations are in place and test-asserted: the D-15 managed-flag/prefix rewrite gate (T-21-02-01), arg-array `git worktree repair` with no shell (T-21-02-02), the `os.Stat` stale-path filter (T-21-02-03), and per-step idempotency (T-21-03-01). No new dependencies (T-21-02-SC).

## Notes for Downstream Plans

- **Plan 21-04 (main.go wiring):** call `migrate.Complete(ctx, db, cfg)` right after `store.Migrate` and before `BackfillProjectIcons`, for a `DoMigrate` or `RollForward` decision only (skip on `SkipCustom`/`FreshInstall`; `RefuseBoot` already errored in `Prepare`). `cfg` is the `Config` returned by `Prepare` (Part-1). A non-nil error from `Complete` maps to refuse-to-boot (D-11), the same as a `Prepare` error.
- The client-side localStorage migration (MIGRATE-04) and the tmux socket/prefix code-literal flip remain for Plan 21-04.

## Self-Check: PASSED

- Files verified on disk: `internal/migrate/paths.go` (258 lines), `internal/migrate/paths_test.go` (240 lines), `internal/migrate/worktree_repair_test.go` (175 lines)
- Commits verified in git: `6fd5c38`, `5ea680c`, `e70b7c1`, `e022331`

---
*Phase: 21-data-directory-migration*
*Completed: 2026-07-01*
