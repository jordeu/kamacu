---
phase: 21-data-directory-migration
plan: 02
subsystem: infra
tags: [migration, sqlite, wal, os.Rename, tmux, startup-hook, go]

# Dependency graph
requires:
  - phase: 20-kamacu-rebrand-and-brand
    provides: "kamacu Go module + binary; runtime data paths (~/.kangent), -L kangent tmux socket, and DB filename deliberately left un-flipped for this phase"
provides:
  - "internal/migrate package: pure Gate decision table (D-13/D-04/D-11) + Part-1 startup one-shot Prepare"
  - "Prepare(ctx, resolvedDBPath, defaultDBFlag) -> (Decision, Config, error): gate -> preflight -> atomic os.Rename commit point -> checkpoint-first DB file rename -> idempotent tmux retire"
  - "Config/Decision contract for Plan 21-03 (paths.go Complete hook) and Plan 21-04 (main.go wiring)"
affects: [21-03-path-rewrite-worktree-repair, 21-04-main-wiring]

# Tech tracking
tech-stack:
  added: []  # no new dependencies — stdlib (os/syscall/path/filepath) + existing internal packages only
  patterns:
    - "Pure gate decision function (I/O-free) driving all trigger/roll-forward/anomaly behavior"
    - "Derived-consistency roll-forward: each post-rename step self-gates on observable state (no marker file)"
    - "Checkpoint-first WAL-safe single-file DB rename"
    - "Package-var seams (sameFilesystemFn, retireTmux) for hermetic unit tests"

key-files:
  created:
    - "internal/migrate/migrate.go"
    - "internal/migrate/migrate_test.go"
  modified: []

key-decisions:
  - "os.Rename is the single commit point (D-03); all refuse-capable checks (preflight, same-fs Dev, EXDEV, both-dirs anomaly) run before it so a failure leaves ~/.kangent untouched (D-02/MIGRATE-05)"
  - "WAL folded with PRAGMA wal_checkpoint(TRUNCATE) before the single-file DB rename; a guard test proves a naive rename loses the hot-WAL row (Pitfall 1)"
  - "tmux retirement targets the LITERAL -L kangent socket, never tmux.DefaultSocket (which 21-04 flips to kamacu)"
  - "sameFilesystemFn/retireTmux are package vars so tests inject EXDEV and never kill a real host tmux server"

patterns-established:
  - "Gate: pure (customDB, srcExists, dstExists) -> Decision 5-branch switch, table-tested in isolation"
  - "completeDBRename self-gates (no-op once kamacu.db exists) enabling RollForward idempotency"

requirements-completed: [MIGRATE-01, MIGRATE-05]

# Metrics
duration: 6min
completed: 2026-07-01
---

# Phase 21 Plan 02: Migrate Package + Part-1 Startup One-Shot Summary

**`internal/migrate` package: a pure 5-branch Gate decision table plus `Prepare` — gate → preflight → atomic `os.Rename` of `~/.kangent`→`~/.kamacu` → checkpoint-first `kangent.db`→`kamacu.db` rename → idempotent `-L kangent` tmux retirement, failure-safe by construction.**

## Performance

- **Duration:** ~6 min
- **Started:** 2026-07-01T14:48:59Z
- **Completed:** 2026-07-01T14:55:28Z
- **Tasks:** 2 (both TDD)
- **Files modified:** 2 created

## Accomplishments
- **Pure Gate decision table** — `Gate(customDB, srcExists, dstExists) Decision` maps three observable booleans to exactly one of `SkipCustom / DoMigrate / RollForward / FreshInstall / RefuseBoot` (D-13 trigger, D-04 roll-forward, D-11 both-dirs anomaly), fully table-tested with no I/O.
- **Part-1 `Prepare`** — the pre-DB startup one-shot: resolves endpoints via `settings.ExpandHome`, detects a custom `--db` (opt-out), runs the pre-rename preflight (source exists, destination absent, same-filesystem `syscall.Stat_t.Dev` check), performs the atomic `os.Rename` commit point (EXDEV → refuse-to-boot), then the WAL-safe DB file rename and the tmux retirement. `RollForward` re-runs only the remaining idempotent steps.
- **Hot-WAL data-integrity proof** — a guard test empirically reproduces the Pitfall-1 data loss (a naive `kangent.db`→`kamacu.db` rename orphans the hot `-wal` and loses the committed row), while the checkpoint-first path preserves it; the roll-forward idempotency and preflight-abort-leaves-source-intact behaviors are likewise covered.

## Task Commits

Each TDD task produced a RED (test) then GREEN (feat) commit:

1. **Task 1: Gate decision table + package scaffolding**
   - `75203a4` test(21-02): add failing gate decision table test
   - `55b639e` feat(21-02): implement gate decision table
2. **Task 2: Part 1 — preflight, dir rename, WAL-safe DB rename, tmux retire**
   - `1030c3e` test(21-02): add failing part-1 migration tests
   - `7cbd4cc` feat(21-02): implement part-1 preflight, dir rename, WAL-safe DB rename, tmux retire

_No REFACTOR commits were needed (code was clean at GREEN)._

## Files Created/Modified
- `internal/migrate/migrate.go` (300 lines) — `Decision`/`Config` types, fixed-endpoint consts, pure `Gate`, `Prepare`, and helpers `preflight` / `completeDBRename` / `sameFilesystem` / `dirExists` / `fileExists`; `sameFilesystemFn` and `retireTmux` seams.
- `internal/migrate/migrate_test.go` (395 lines) — `TestGate`, `TestDecisionString`, the hot-WAL fixture builder, the naive-rename guard test, and DoMigrate / RollForward / cross-device / RefuseBoot / SkipCustom / FreshInstall behavior tests; `TestMain` stubs the tmux retirement so `go test` never kills a real host server.

## Decisions Made
- **`os.Rename` as the single commit point (D-03):** every refuse-capable check runs before it, so the source is byte-for-byte intact on any pre-rename failure (D-02/MIGRATE-05). EXDEV at the rename itself is caught with `errors.Is(err, syscall.EXDEV)` and mapped to refuse-to-boot (no copy-then-swap fallback, D-01).
- **Checkpoint-first DB rename (Pitfall 1):** `PRAGMA wal_checkpoint(TRUNCATE)` folds the hot WAL before the single-file rename; `completeDBRename` self-gates on `kangent.db` still present so it is a clean no-op on a second (RollForward) boot.
- **Literal `"kangent"` tmux socket for retirement:** deliberately NOT `tmux.DefaultSocket` (Plan 21-04 flips that to `"kamacu"`); `KillServer` treats "no server" as idempotent success, so the error is ignored.
- **Test seams as package vars (`sameFilesystemFn`, `retireTmux`):** allow injecting a cross-device layout and counting tmux retirement without a real second filesystem or killing a live tmux server.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- **Forcing a persistent hot WAL in a Go test:** a clean `db.Close()` checkpoints (folding the WAL away), so the fixture builds the DB in a scratch dir, folds schema+clean row with `wal_checkpoint(TRUNCATE)`, disables `wal_autocheckpoint`, commits the "hot" row, then raw-copies the `.db`/`-wal`/`-shm` files into the fixture dir *before* any close — a faithful "killed mid-transaction" snapshot. The guard test asserts the copied `-wal` is non-empty so the fixture cannot silently degrade.

## Notes for Downstream Plans
- The unexported `defaultDBFlag = "~/.kamacu/kamacu.db"` const documents the flipped `--db` default (D-12). It is scaffolding for Plan 21-04's `main.go` wiring, which passes the default flag value into `Prepare`; it is intentionally not referenced within this plan (`Prepare` uses its `defaultDBFlag` parameter).
- Part 2 (managed-path rewrite under the old root + `git worktree repair` + `kangent-*` tmux-row cleanup) is Plan 21-03's `migrate.CompletePaths`/`paths.go`, called after `store.Migrate`. Nothing imports `internal/migrate` yet.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- `Gate`, `Prepare`, `Config`, `Decision` (+ the five variants) are exported and stable for 21-03 (adds `Complete`/`CompletePaths`) and 21-04 (main.go wiring).
- Full repo `go test ./...` green (13 packages), `go vet ./internal/migrate/` + `go build ./...` clean, `gofmt` clean, no `sh -c`/string-interpolated exec in the package.

---
*Phase: 21-data-directory-migration*
*Completed: 2026-07-01*
