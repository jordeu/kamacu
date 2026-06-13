---
phase: 09-tmux-restart-resume-cleanup-integration
plan: 05
subsystem: infra
tags: [reaper, goroutine, ttl, sessions, sqlite, slog, kanban]

# Dependency graph
requires:
  - phase: 09-01
    provides: migration 00006 per-status *_at columns (todo_at/in_progress_at/in_review_at/done_at) with done_at backfill
  - phase: 09-02
    provides: done_session_ttl setting + settings.ParseDoneSessionTTL disable-semantics helper
  - phase: 09-04
    provides: Manager.StopAllForTask kill-all path + startup main.go seam (orphan sweep before ListenAndServe)
provides:
  - "move handler stamps the entered status's *_at column (last-entry-wins); done_at is the reaper's clock (D-90)"
  - "internal/reaper package: a Done-TTL session reaper (reapOnce scan + Run ticker goroutine)"
  - "main.go starts the reaper goroutine at boot — the codebase's first background goroutine"
affects: [stats, dwell-time, cycle-time, session-lifecycle, future-reapers]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Background ticker goroutine with reapOnce as the testable per-tick unit (now() clock seam mirrors quota.Config.Now)"
    - "Locally-defined SessionStopper interface so the package is unit-testable with a spy, no PTYs (*session.Manager satisfies it)"
    - "Lexical ISO-8601 cutoff comparison in SQL (done_at < ?) — no per-row time parsing, because strftime millisecond ISO sorts chronologically"

key-files:
  created:
    - internal/reaper/reaper.go
    - internal/reaper/reaper_test.go
  modified:
    - internal/api/tasks.go
    - internal/api/tasks_test.go
    - cmd/kangent/main.go

key-decisions:
  - "Reaper tick is 10 min (research discretion 5-15min); reapOnce runs once at start so a restart promptly reaps long-Done tasks without waiting a full tick"
  - "Reaper degrades to off on a bad/unreadable done_session_ttl (slog.Warn, never crash the goroutine) — a saved value is validated, so this is defensive only"
  - "Reaper has zero DB writes: one SELECT id gated on status='done' AND done_at IS NOT NULL AND done_at < cutoff, then StopAllForTask; claude_session_id/transcript and worktrees are never touched (D-96/D-87 automatic)"
  - "move stamps only the entered status's column via a fixed status->column map (never raw req.Status); leaving Done never clears done_at — the status='done' gate cancels reaping, not done_at (D-90)"

patterns-established:
  - "First background goroutine: process-lifetime ticker started with context.Background() (no graceful shutdown — process death is the stop), launched after the orphan sweep and before ListenAndServe"
  - "Reaper unit tests seed a real migrated SQLite DB + spy SessionStopper + pinned now() — deterministic TTL expiry with no sleeps"

requirements-completed: [REAP-01]

# Metrics
duration: 12min
completed: 2026-06-13
---

# Phase 9 Plan 5: Done-TTL Session Reaper Summary

**A background ticker goroutine that kills bash + tmux + agent sessions of tasks left in Done past `done_session_ttl` (clocked from `done_at`), keeping every agent resumable and never touching worktrees — the codebase's first background goroutine.**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-06-13T08:58Z
- **Completed:** 2026-06-13T09:10Z
- **Tasks:** 3
- **Files modified:** 5 (2 created, 3 modified)

## Accomplishments

- The `move` handler now stamps the entered status's `*_at` column (last-entry-wins) via a fixed `status->column` map; entering Done sets `done_at` — the reaper's clock (D-90). The other three columns are banked for future stats.
- New `internal/reaper` package: `reapOnce` reads the TTL via `settings.ParseDoneSessionTTL` (shared disable semantics with the save-time validator), queries Done tasks gated on `status='done' AND done_at IS NOT NULL AND done_at < cutoff`, skips tasks with no running session, and calls `StopAllForTask` — logging each reap (D-95). `Run` ticks every 10 min for the process lifetime and reaps once at start.
- `cmd/kangent/main.go` launches `go reaper.New(db, mgr).Run(context.Background())` after the orphan sweep and before `ListenAndServe` — the app's first background goroutine.

## Task Commits

Each task was committed atomically:

1. **Task 1: Stamp the entered-status *_at column on move (D-90)** — `300d0b7` (feat) — TDD test + impl in one api-package commit
2. **Task 2: internal/reaper package — Done-TTL reaper logic (REAP-01, D-95/D-96)** — `7d9ff84` (feat) — TDD test + impl
3. **Task 3: Start the reaper goroutine at startup** — `ebaca29` (feat)

**Plan metadata:** _(docs commit — this SUMMARY + STATE + ROADMAP + REQUIREMENTS)_

_Note: Task 1 and Task 2 were `tdd="true"`; tests were written first (verified RED), then the implementation (verified GREEN), committed together per package since the test and behavior are the same package-level change._

## Files Created/Modified

- `internal/reaper/reaper.go` — created: `Reaper` struct, `New`, `Run` (ticker), `reapOnce` (the scan), and the `SessionStopper` interface
- `internal/reaper/reaper_test.go` — created: spy `SessionStopper` + pinned `now()` + 7 behavior tests (expired+running reaped, within-TTL not reaped, left-Done not reaped, disabled never/0/empty not reaped, NULL done_at not reaped, no-running-sessions skipped, mixed-set only-expired-reaped)
- `internal/api/tasks.go` — modified: `move` step 5 UPDATE now also stamps `statusAtCol` (fixed map keyed by validated `req.Status`)
- `internal/api/tasks_test.go` — modified: `statusAt` helper + 3 tests (stamps done_at on entry, stamps only the entered column, last-entry-wins overwrite + leaving-Done preserves done_at)
- `cmd/kangent/main.go` — modified: import `kangent/internal/reaper`; launch the goroutine; `slog.Info("Done-TTL reaper started")`

## Decisions Made

- **Tick = 10 min, reap-once-at-start.** A Done-TTL measured in hours needs no sub-minute precision; reaping once on `Run` start means a restart promptly reaps tasks that were already long-Done.
- **No DB writes in the reaper.** Its only statement is `SELECT id`; its only effect is `StopAllForTask`. This makes D-96 (keep `claude_session_id`/transcript) and D-87 (never touch worktrees) automatic rather than something to re-implement — verified by grep: the only `claude_session_id` references in the reaper are comments stating it is never deleted.
- **Degrade, don't crash.** A bad/unreadable `done_session_ttl` makes the tick a no-op with a `slog.Warn`; a saved value is validated by `ParseDoneSessionTTL`, so this is defensive against a hand-edited row only.
- **Stamp only the entered column.** Leaving Done never clears `done_at`; the reaper's `status='done'` gate (not `done_at`) is what cancels reaping (D-90).

## Deviations from Plan

None — plan executed exactly as written. The plan's suggested `New(db, mgr) *Reaper` and `Run`/`reapOnce` signatures, the locally-defined `SessionStopper` interface, the lexical SQL cutoff, and the `now()` seam were all implemented verbatim.

## Issues Encountered

None. RED → GREEN was clean for both TDD tasks. The full `internal/api` suite (which exercises real git worktree provisioning) takes ~90s but passes; reaper and settings suites are sub-second.

## User Setup Required

None — no external service configuration required. The reaper is governed entirely by the existing `done_session_ttl` setting (default `24h`; `never`/`0`/empty disables).

## Next Phase Readiness

- REAP-01 is fully satisfied: sessions of tasks left in Done past the configurable TTL are killed (bash + tmux + agent), clocked from `done_at`, cancellable by leaving Done, disablable by `never`/`0`/empty.
- Reaping is silent with server logs (D-95); agents remain fully resumable (D-96); worktrees are never auto-removed (D-87).
- The codebase now has a clean, tested background-goroutine pattern any future periodic worker can follow.
- This completes Phase 09 (plan 5 of 5).

## Self-Check: PASSED

- `internal/reaper/reaper.go` — FOUND
- `internal/reaper/reaper_test.go` — FOUND
- Commit `300d0b7` — FOUND
- Commit `7d9ff84` — FOUND
- Commit `ebaca29` — FOUND
- `go build ./...` exits 0; `go test ./internal/api ./internal/reaper ./internal/settings` all pass
- D-96 guard: reaper has no DELETE / no `UPDATE tasks SET` / no `claude_session_id` write (verified by grep)

---
*Phase: 09-tmux-restart-resume-cleanup-integration*
*Completed: 2026-06-13*
