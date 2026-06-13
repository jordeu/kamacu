---
phase: 09-tmux-restart-resume-cleanup-integration
plan: 04
subsystem: api
tags: [tmux, worktree-cleanup, task-delete, orphan-sweep, sqlite, go]

# Dependency graph
requires:
  - phase: 09-01
    provides: tmux.Client ListSessions/HasSession/KillSession verbs on the dedicated socket
  - phase: 09-03
    provides: Manager.HasLiveTmux(name) de-dup probe; tmux_sessions lazy GC
provides:
  - "Cleanup dialog running_sessions count folds in live DETACHED tmux sessions (one number, no tmux wording)"
  - "Worktree-remove kills every live tmux session of the task before wt.Remove"
  - "Task-delete kills the task's live tmux sessions and removes its tmux_sessions rows before the row delete"
  - "Synchronous once-at-startup orphan sweep kills DB-less kangent-* sessions before serving"
affects: [phase-09-05-done-ttl-reaper, tmux-cleanup, worktree-lifecycle]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Kill-by-DB-row: detached tmux survivors (no in-memory session) are reached via their tmux_sessions row, not the Manager"
    - "Folded session count: in-memory running + live tmux rows the Manager has no live session for (HasLiveTmux de-dup) = one honest number (D-77)"
    - "Startup DB-driven reconcile: JOIN tmux_sessions against tasks so rows whose task vanished while down get swept"

key-files:
  created: []
  modified:
    - internal/api/worktrees.go
    - internal/api/worktrees_test.go
    - internal/api/tasks.go
    - internal/api/routes.go
    - internal/api/sessions_test.go
    - cmd/kangent/main.go

key-decisions:
  - "liveTmuxNames collects a name only on a CONCLUSIVE has-session (alive && err==nil) — a broken/hung tmux neither inflates the count nor triggers a kill (Pitfall 6 honesty)"
  - "Task-delete removes tmux_sessions rows EXPLICITLY (no FK cascade per migration 00005); kill is the action, not cascade"
  - "Startup sweep is synchronous and once before ListenAndServe, never periodic (D-99 declined): tmux only orphans through Kangent-controlled paths"
  - "Test harnesses (newTmuxSessionServer) thread the SAME tmux client through Routes so the delete kill path is exercised end-to-end"

patterns-established:
  - "Kill-before-remove on BOTH worktree-remove and task-delete, plus a startup sweep — every orphan path covered"
  - "tmux failures are warn-only and never block cleanup/delete (Pitfall 5)"

requirements-completed: [TMUX-08]

# Metrics
duration: 14min
completed: 2026-06-13
---

# Phase 9 Plan 4: Cleanup/Delete tmux Accounting + Startup Orphan Sweep Summary

**TMUX-08: cleanup dialog folds live detached tmux into one running_sessions number, worktree-remove and task-delete kill the task's tmux sessions before removal, and a synchronous startup sweep reaps DB-less kangent-* sessions — closing every orphan-shell hole.**

## Performance

- **Duration:** 14 min
- **Started:** 2026-06-13T06:42:15Z
- **Completed:** 2026-06-13T06:56:38Z
- **Tasks:** 3
- **Files modified:** 6 (source) + 6 (test-wiring fan-out)

## Accomplishments
- Cleanup dialog `running_sessions` count now folds in live DETACHED tmux survivors (a session that outlived a leave/restart has no in-memory session) as ONE honest number with no tmux-specific field or wording (D-92/D-77). Gate 1 (sessions-running 409) trips for them automatically.
- Confirmed worktree-remove kills every live tmux session of the task (`has-session` → `kill-session`) BEFORE `wt.Remove`, so git can no longer delete a tree with a daemonized tmux server still cwd'd inside it (D-92/D-93).
- Task-delete kills the task's live tmux sessions and removes its `tmux_sessions` rows before the task row delete — order: `StopAllForTask` → kill tmux → delete rows → delete task (D-93).
- A once-at-startup, synchronous, DB-driven orphan sweep enumerates live `kangent-*` sessions and kills any with no matching `tmux_sessions`+`tasks` row, reconciling deletes/removes that happened while Kangent was down (D-93). The branch is always kept (D-34); the sweep never touches worktrees (D-87).

## Task Commits

Each task was committed atomically:

1. **Task 1: Fold live-tmux count + kill before wt.Remove** - `46c6cb3` (feat)
2. **Task 2: Kill tmux sessions on task delete** - `f6b98dd` (feat)
3. **Task 3: Startup orphan sweep before serving** - `360e127` (feat)

**Plan metadata:** _(this commit)_ (docs: complete plan)

## Files Created/Modified
- `internal/api/worktrees.go` - `WorktreeRoutes`/`worktreeHandlers` gain a `tmux.Client`; new `liveTmuxNames` (conclusive has-session probe of each `tmux_sessions` row) and `cleanupSessionCount` (folded, HasLiveTmux-deduped count); `get` returns the folded `running_sessions`; `remove` kills live tmux before `wt.Remove` and gates on the folded count.
- `internal/api/worktrees_test.go` - `newWorktreeServerWithTmux` helper; `TestWorktreeCleanupCountsAndKillsDetachedTmux` (skip-guarded) asserts a detached survivor folds into the count, trips the gate, and is killed before remove.
- `internal/api/tasks.go` - `taskHandlers` gains a `tmux.Client`; `delete` kills live tmux + deletes `tmux_sessions` rows before the task row; new `taskTmuxNames` helper.
- `internal/api/routes.go` - `Routes` gains a `tmuxClient tmux.Client` param, set on `taskHandlers`.
- `internal/api/sessions_test.go` - `newTmuxSessionServer` threads the real tmux client through `Routes`; `TestTaskDeleteKillsDetachedTmux` (skip-guarded) asserts the delete kills a detached survivor and clears its rows.
- `cmd/kangent/main.go` - threads `tmuxClient` into `api.Routes`/`api.WorktreeRoutes`; new `sweepOrphanTmux` run synchronously before `http.ListenAndServe`.
- Test-wiring fan-out (signature threading only): `agents_test.go`, `agent_integration_test.go`, `diffs_test.go`, `integration_test.go`, `projects_test.go`, `recovery_integration_test.go` updated to pass `tmux.Client{}` to the new `Routes`/`WorktreeRoutes` signatures.

## Decisions Made
- **Conclusive-only liveness:** `liveTmuxNames` collects a name only when `has-session` returns `alive && err == nil`. A broken/hung tmux binary (inconclusive) neither inflates the dialog count nor triggers a kill — Pitfall 6 honesty.
- **Explicit row deletion over FK cascade:** migration 00005 has `task_id ... REFERENCES tasks(id)` with NO `ON DELETE CASCADE` (and SQLite FKs default off). The delete handler kills sessions then removes `tmux_sessions` rows explicitly; cascade was deliberately NOT added — kill is the correct action and explicit cleanup keeps the startup sweep light.
- **Startup sweep is synchronous + once, not periodic:** D-99's periodic sweep was declined as redundant — tmux sessions only orphan through Kangent-controlled paths, so startup reconcile is sufficient. The sweep blocks before serving so the server starts clean.
- **Harness fidelity:** `newTmuxSessionServer` now passes the SAME tmux client to `Routes` (it previously passed a zero-value), mirroring production's single hoisted client, so the delete-kill path is actually exercised.

## Deviations from Plan

None - plan executed exactly as written.

The plan's "thread the tmuxClient local from 09-03" contingencies did not fire: `tmuxClient` and `mgr.HasLiveTmux` were already present in `cmd/kangent/main.go` and `internal/session/manager.go` from 09-01/09-03, so this plan reused them directly with no idempotent re-adds needed.

## Issues Encountered
- Adding the `tmux.Client` parameter to `WorktreeRoutes` and `Routes` rippled to every test caller in the `internal/api` package. Resolved by threading `tmux.Client{}` (zero-value) through each non-tmux harness — on an unconfigured socket `HasSession` errors for every name, so `liveTmuxNames` collects nothing, which is exactly the tmux-absent behavior those tests want. `internal/api/projects_test.go` used a distinct `Routes(...)` argument shape and lacked the tmux import; fixed individually.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Every tmux orphan path is now closed (cleanup count, worktree-remove kill, task-delete kill, startup sweep), so 09-05 (Done-TTL reaper) can lean on these kill paths and the persisted `tmux_sessions` rows without re-deriving liveness.
- The reaper (09-05) and this sweep both call `KillSession` idempotently; no shared-state coordination needed.

---
*Phase: 09-tmux-restart-resume-cleanup-integration*
*Completed: 2026-06-13*

## Self-Check: PASSED

All claimed files exist (worktrees.go, tasks.go, routes.go, main.go, worktrees_test.go, sessions_test.go, SUMMARY.md) and all three task commits (46c6cb3, f6b98dd, 360e127) are present in git history. `go build ./...`, `go vet ./...`, and `go test ./internal/api ./internal/session` all pass.
