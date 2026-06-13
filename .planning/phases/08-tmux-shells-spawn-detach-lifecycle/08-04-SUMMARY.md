---
phase: 08-tmux-shells-spawn-detach-lifecycle
plan: 04
subsystem: api
tags: [tmux, sessions, pty, terminal, go, react, sqlite]

# Dependency graph
requires:
  - phase: 08-01
    provides: internal/tmux package (WriteConfig, Client, DefaultSocket) + migration 00005 tmux_sessions table
  - phase: 08-02
    provides: AllowedShells() with LookPath("tmux") so the shell dropdown offers tmux only when present (TMUX-01)
  - phase: 08-03
    provides: Manager.SetTmuxClient + SpawnOpts.TmuxName + ErrTmuxNotFound + per-session killer (Stop kills the tmux session)
provides:
  - "End-to-end tmux spawn: POST /api/sessions with shell=tmux mints/persists kangent-<task>-<n> and attaches under creack/pty on the -L kangent socket (TMUX-02)"
  - "DB-minted session names (COALESCE(MAX(n),0)+1 over tmux_sessions) — unique across server restarts, never the in-memory counter"
  - "Startup tmux config generation (status off, mouse on, history 50000) written next to the DB before any spawn"
  - "Honest 409 spawn errors surfaced verbatim in the tab header (D-84 tmux-missing, dev-route, no-worktree)"
  - "HTTP-layer x-kill proof: stopping a tmux session removes the tmux session itself (TMUX-04)"
affects: [09-tmux-restart-reconcile-resume, sessions, settings]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "DB-as-name-authority: reserve n with an INSERT before Spawn; release the row on spawn failure; back-fill the label warn-only after success"
    - "409-as-human-copy: deliberate conflict messages travel to the frontend verbatim; 500s stay generic"

key-files:
  created: []
  modified:
    - cmd/kangent/main.go
    - internal/api/sessions.go
    - internal/api/sessions_test.go
    - web/src/pages/TaskPage.tsx

key-decisions:
  - "tmux session names come from tmux_sessions MAX(n)+1 (reserved by INSERT before Spawn), never the in-memory counter — survives restarts, UNIQUE(task_id,n) is the backstop"
  - "shell=tmux on the unscoped /terminal dev route (no task_id) is an honest 409 'tmux shells need a task', never a bare tmux exec on the user's default socket"
  - "D-84 maps ErrTmuxNotFound to 409 (not 500) so the frontend renders the exact copy 'tmux not found — change the shell setting or reinstall' verbatim; never a silent bash fallback"
  - "Spawn failure releases the reserved tmux_sessions row (DELETE by name); label back-fill after success is warn-only (same degradation posture as the agent claude_session_id persist)"
  - "Frontend stays 100% tmux-unaware (D-77): the only change is ApiError status===409 → message verbatim, else the generic retry copy"

patterns-established:
  - "Reserve-before-spawn / release-on-failure for externally-named resources keyed by a UNIQUE DB constraint"
  - "Server-authored 409 copy is the single source of UI conflict text — frontend forwards, never re-authors"

requirements-completed: [TMUX-02, TMUX-03, TMUX-04]

# Metrics
duration: 52min
completed: 2026-06-13
---

# Phase 8 Plan 4: tmux Spawn Wiring & Honest Errors Summary

**End-to-end invisible tmux: POST /api/sessions with shell=tmux mints and persists `kangent-<task>-<n>` from the tmux_sessions table, attaches it under creack/pty on the dedicated `-L kangent` socket, and surfaces honest 409 spawn errors verbatim in the tab header — with zero tmux markers reaching the UI or the wire (D-77).**

## Performance

- **Duration:** ~52 min
- **Started:** 2026-06-12T22:16:30Z
- **Completed:** 2026-06-13T05:10:00Z (wall clock includes the human-verify checkpoint pause)
- **Tasks:** 3 (2 automated + 1 human-verify checkpoint)
- **Files modified:** 4

## Accomplishments
- `cmd/kangent/main.go` writes `~/.kangent/kangent-tmux.conf` (status off, mouse on, history 50000) at every startup and installs `tmux.Client{Socket: DefaultSocket, ConfPath: ...}` on the Manager — verified live: config present, server healthy.
- `internal/api/sessions.go` `create` handler grew the `shell=="tmux"` branch: dev-route 409, DB-minted name (`COALESCE(MAX(n),0)+1`) reserved by an INSERT, `opts.TmuxName` set, ErrTmuxNotFound→409 with release of the reserved row, and warn-only label back-fill.
- Three HTTP-layer integration tests lock the behavior: happy-path mint/persist + D-77 wire audit + TMUX-04 x-kill on a per-test socket, D-84 missing-binary 409 with row release, and dev-route 409.
- `web/src/pages/TaskPage.tsx` renders the server's 409 message verbatim (D-84 copy, "tmux shells need a task", "task has no worktree") while keeping the generic fallback for 500s — and stays entirely tmux-unaware (grep `tmux` == 0).
- Human verification confirmed: indistinguishable tab (no status bar), clean reattach repaint across task-view navigation, mouse-wheel scrollback, and x-kill parity.

## Task Commits

Each task was committed atomically:

1. **Task 1: main.go config wiring + handler tmux branch** - `6c9aa80` (feat)
2. **Task 2 (TDD/integration): handler tmux tests** - `3bb7390` (test)
3. **Task 2: frontend honest-error rendering** - `aae3bb0` (feat)

**Plan metadata:** _(this commit)_ (docs: complete plan)

## Files Created/Modified
- `cmd/kangent/main.go` - tmux config written at startup (next to the DB) + `mgr.SetTmuxClient` wiring; `kangent/internal/tmux` imported.
- `internal/api/sessions.go` - `shell=="tmux"` branch in `create`: dev-route 409, DB name minting (`kangent-%d-%d`), reserved-row INSERT, ErrTmuxNotFound→409 + row release, warn-only label back-fill; `fmt` imported.
- `internal/api/sessions_test.go` - `TestSessionTmuxSpawnHappyPath`, `TestSessionTmuxSpawnMissingBinaryHTTP`, `TestSessionTmuxDevRouteRejected` + `newTmuxSessionServer`/`awaitHasSession` helpers.
- `web/src/pages/TaskPage.tsx` - spawn error span renders `ApiError` message when `status === 409`, generic copy otherwise; `ApiError` imported from `@/api/client`.

## Decisions Made
- None beyond the plan — all key decisions (DB-as-name-authority, dev-route 409, D-84 as 409, reserve/release, frontend forwards 409 copy) were specified in the plan and implemented as written.

## Deviations from Plan

None - plan executed exactly as written. The plan's two automated tasks built and tested clean on the first pass (`go test ./... -count=1` and `make build` both green); the human-verify checkpoint passed without reported issues.

## Issues Encountered
None during planned work.

## Known Limitations / Phase 9 Seam (TMUX-05)

**Server-restart reattach is intentionally NOT in this phase.** During verification the user confirmed that after a Kangent *server* restart:
- the tmux session **survives** (correct — it lives on the `-L kangent` socket, independent of Kangent's process), but
- the task-view tab **disappears** (no reattach / no Resume affordance).

This is correct and expected for Phase 8. Restart reconciliation + Resume is **TMUX-05, scoped to Phase 9**. The reason the tab vanishes:
- `tmux_sessions` is **persisted** (every spawn inserts a row, label back-filled) — so the durable identity exists on disk —
- but it is **not yet read at startup**, and `Manager.ListByTask` is **in-memory only** (it returns live `*Session` snapshots, which a fresh process does not have).

Phase 9 closes this by reading `tmux_sessions` on startup, probing `tmux has-session` for liveness, and offering a Resume affordance that re-attaches a fresh PTY to the surviving tmux session (mirroring the Phase 5 `claude --resume` reconcile pattern). The DB row + the `detachedAlive`/honest-exited posture from Plan 08-03 are the seam Phase 9 builds on; no schema change is required.

## User Setup Required
None - no external service configuration required. (tmux must be installed on the host for the shell option to appear, which it is: tmux 3.4 verified on PATH.)

## Next Phase Readiness
- Phase 8 closes the tmux spawn/detach/kill lifecycle as user-observable behavior: TMUX-01 (option), TMUX-02 (spawn), TMUX-03 (reattach within a running server), TMUX-04 (x-kill), TMUX-06 (inner exit), D-77/D-79/D-80/D-83/D-84 all validated.
- Phase 9 (TMUX-05) is unblocked: the persisted `tmux_sessions` table is the only missing read; no migration needed, the reconcile pattern mirrors Phase 5.

## Self-Check: PASSED

---
*Phase: 08-tmux-shells-spawn-detach-lifecycle*
*Completed: 2026-06-13*
