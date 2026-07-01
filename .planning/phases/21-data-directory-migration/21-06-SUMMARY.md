---
phase: 21-data-directory-migration
plan: 06
subsystem: infra
tags: [migration, git-worktree, gap-closure, idempotent, slog, go]

# Dependency graph
requires:
  - phase: 21-data-directory-migration
    provides: "internal/migrate Part-2 Complete/repairWorktrees (21-03) — the batch git worktree repair this plan hardens"
provides:
  - "repairWorktrees per-path git worktree repair that tolerates existing-but-unregistered worktree dirs (slog.Warn + skip, never abort) — Complete survives the real sched repo's 4 stale dirs and always reaches deleteOldTmuxRows"
affects: []

# Tech tracking
tech-stack:
  added: []  # no new dependencies — only stdlib log/slog added; fmt removed (now unused)
  patterns:
    - "Per-path (not batch) git worktree repair: a single stale straggler is logged + skipped, never a fatal error — a batch invocation would fail the whole call"
    - "Warn-only best-effort failure via log/slog (reaper.go idiom): terminal-visible surface (D-11), non-destructive skip, idempotency preserved"

key-files:
  created: []
  modified:
    - "internal/migrate/paths.go"
    - "internal/migrate/worktree_repair_test.go"

key-decisions:
  - "Per-path repair with slog.Warn + continue on nonzero exit — the os.Stat filter only drops NON-EXISTENT paths (RESEARCH G); it misses the existing-but-unregistered case (Gap 1), so tolerance must live at the exec, not the filter"
  - "Faithful fixture: git worktree add a second tree, then rm its .git/worktrees/<id> registration, leaving a dangling .git gitfile — reproduces the 4 real sched dirs so git worktree repair exits 1"
  - "Swap fmt import for log/slog (the fatal fmt.Errorf is gone); arg-array exec.CommandContext preserved — no sh -c introduced (ASVS V5 / worktree.go invariant)"

requirements-completed: [MIGRATE-02, MIGRATE-03]

# Metrics
duration: 10min
completed: 2026-07-01
---

# Phase 21 Plan 06: Harden Worktree Repair Against Stale Worktrees Summary

**`migrate.repairWorktrees` now repairs each managed worktree PER PATH and tolerates a DB-referenced dir that exists on disk but is unregistered in `.git/worktrees/` — logging a terminal-visible `slog.Warn` and skipping it instead of aborting the whole migration — so the real `sched` repo's 4 stale dirs no longer make `Complete` return an error, `deleteOldTmuxRows` (MIGRATE-03) always runs, and the app boots (Gap 1 closed).**

## Performance

- **Duration:** ~10 min
- **Tasks:** 1 (TDD: RED test commit then GREEN feat commit)
- **Files:** 2 modified

## Accomplishments

- **Gap 1 root cause fixed (MIGRATE-01/02/03/05 unblocked)** — `repairWorktrees` no longer passes every `os.Stat`-existing worktree path to a single `git worktree repair` invocation whose nonzero exit was fatal. It now loops PER PATH: each `exec.CommandContext(ctx, "git", "-C", <repo>, "worktree", "repair", p)` runs independently, and a nonzero exit emits `slog.Warn("migrate: skipping unrepairable worktree (stale/unregistered)", "repo", …, "worktree", p, "error", …)` then `continue`s — it never returns the git error. So a stale/unregistered straggler (the real `sched` repo's `add-nvidia-gpu-support…-2`, `cluster-gc…-28`, `pr-561-22`, `vmscalingservice…-18`) is logged and skipped, `Complete` proceeds to `deleteOldTmuxRows`, and the app boots.
- **The os.Stat filter's blind spot is now covered** — 21-03's `existingWorktreePaths` os.Stat filter only drops NON-EXISTENT paths (RESEARCH pitfall G). The missed case (this gap) is an EXISTING dir that git no longer tracks: `git worktree repair <that path>` exits 1 with "does not reference a repository". Tolerance had to live at the exec boundary (log+skip), not in the stat filter — the filter can't distinguish a registered from an unregistered on-disk dir.
- **Every prior verified behavior preserved** — a valid moved-but-registered worktree still repairs (RESEARCH D: per-path `git worktree repair <new path>` exits 0 and fixes the two-way links), a healthy tree is a clean no-op (RESEARCH E), so D-04 roll-forward idempotency is intact (a second `Complete` on the same state returns nil and the valid tree stays usable). `rewriteManagedPaths`, `deleteOldTmuxRows`, `Complete`'s step order, and the Gate/Prepare/Config contract are untouched.
- **Regression test reproduces the abort (RED) and the fix (GREEN)** — `TestCompleteToleratesUnregisteredWorktree` builds a valid registered worktree plus a second one whose `.git/worktrees/<id>` registration is removed (dangling `.git` gitfile), renames the root (the Part-1 commit point), and asserts: `Complete` returns nil; the valid worktree is repaired (`git status` exits 0, listed, not prunable); zero `kangent-%` tmux rows remain; a second `Complete` is a clean no-op. A fixture-sanity assertion proves the ghost is genuinely unrepairable (git exits non-zero) so the test keeps reproducing the bug it guards.

## Task Commits

1. **Task 1: Per-path worktree repair tolerating unregistered/stale dirs (log+skip)**
   - `8875d6e` test(21-06): add failing regression for stale-unregistered worktree abort
   - `1bbd0e8` feat(21-06): tolerate stale/unregistered worktrees in repair (log+skip)

_No REFACTOR commit was needed (code was clean at GREEN)._

## Files Created/Modified

- `internal/migrate/paths.go` (modified) — `repairWorktrees` batch invocation replaced with a per-path loop (`slog.Warn` + `continue` on nonzero exit); docstring updated to explain the Gap-1 tolerance; `fmt` import swapped for `log/slog` (the fatal `fmt.Errorf` is gone). `existingWorktreePaths`' os.Stat filter, the collect-repos-then-query cursor discipline (`SetMaxOpenConns(1)`), and the arg-array exec are all unchanged.
- `internal/migrate/worktree_repair_test.go` (modified) — added `TestCompleteToleratesUnregisteredWorktree` in the existing git-integration style (`gitIntegrationSetup` skip-if-no-git + `t.Setenv` config isolation, `buildRepoWithWorktree`, `gitTest`).

## Decisions Made

- **Tolerance at the exec, not the filter:** the plan offered two options (intersect with `git worktree list --porcelain`, or per-path log+skip). Per-path log+skip was chosen — it is the smaller, lower-risk change, mirrors the in-tree `reaper.go` warn-only idiom, and keeps every git invocation an explicit arg-array with no extra parse step. Intersecting with `worktree list` would add a second git call and a parser for no boot-critical benefit.
- **Faithful fixture over a synthetic one:** the ghost worktree is created by a real `git worktree add` then removing its `.git/worktrees/<id>` registration, so its checkout dir carries a dangling `.git` gitfile — byte-for-byte the shape of the 4 real `sched` dirs (existing on disk + in the DB, absent from `.git/worktrees/`). A fixture-sanity assertion (`git worktree repair <ghost>` must exit non-zero) guards against the reproduction silently rotting.
- **`slog.Warn` is terminal-visible by design (D-11):** each skip names the repo, the worktree path, and the trimmed git stderr. A genuinely unexpected repair failure is still surfaced to the operator; the skip is non-destructive because a stale/unopenable dir was already unusable and is Phase-23 cleanup's job (threat T-21-06-03 accepted).

## Deviations from Plan

None - plan executed exactly as written. (The `fmt` import became unused once the fatal `fmt.Errorf` was removed, so it was dropped and `log/slog` added — the import change the plan called for.)

## Issues Encountered

None. `gofmt` clean, `go vet ./internal/migrate/` clean, `go build ./...` clean, and the full repo suite `go test ./...` (13 packages) is green, including the new `TestCompleteToleratesUnregisteredWorktree` and the pre-existing `TestCompleteRepairsMovedWorktrees` / `TestCompleteStalePathFiltered`.

## Threat Surface Scan

No new security surface beyond the plan's `<threat_model>`. Mitigations in place: arg-array `git worktree repair` with no shell (T-21-06-01), per-path log+skip so a stale straggler can never make `Complete` error (T-21-06-02, the gap), terminal-visible `slog.Warn` on each accepted skip (T-21-06-03). No new dependencies — only stdlib `log/slog` added (T-21-06-SC).

## Notes for Downstream Plans

- **Phase 23 (worktree cleanup):** the stale/unregistered dirs this plan logs-and-skips are left on disk untouched. They surface as `migrate: skipping unrepairable worktree …` warnings at boot and are the intended input for a future GC pass.
- **21-07 (git-safe UAT recipe):** unblocked — with `Complete` now surviving the real install's stale dirs, the end-to-end migration smoke can reach boot/serve and the human-verify walkthrough.

## Self-Check: PASSED

- Files verified on disk: `internal/migrate/paths.go` (278 lines), `internal/migrate/worktree_repair_test.go` (269 lines)
- Commits verified in git: `8875d6e` (RED test), `1bbd0e8` (GREEN feat)

---
*Phase: 21-data-directory-migration*
*Completed: 2026-07-01*
