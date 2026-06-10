---
phase: 03-worktree-isolation-bash-tabs
plan: 05
subsystem: ui
tags: [react, radix-alert-dialog, tanstack-query, worktree, dnd-kit]

# Dependency graph
requires:
  - phase: 03-worktree-isolation-bash-tabs (plan 03-03)
    provides: GET/DELETE /api/tasks/{id}/worktree, exact 409 bodies ("sessions running" / "worktree has uncommitted changes"), Task worktree fields in taskColumns
  - phase: 03-worktree-isolation-bash-tabs (plan 03-04)
    provides: useWorktreeState (gcTime 0 fetch-on-open), useCleanupWorktree (DELETE {stop_sessions, force} + invalidations), WorktreeMetaLine absent state
provides:
  - web/src/components/task/CleanupWorktreeDialog.tsx — one AlertDialog, four state-derived variants (clean / sessions / dirty / dirty+sessions), type-to-confirm gate, inline failure with state refetch
  - Board.tsx done-transition trigger — call-site onSuccess on moveTask.mutate fires the offer AFTER the move persists (D-31)
  - TaskPage ellipsis "Clean up worktree" neutral menu item above Delete task (D-31 "Both")
affects: [03-06 e2e verification, phase-04 agent tab]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Call-site mutate callbacks for per-surface side effects: moveTask.mutate(args, { onSuccess }) in Board keeps the shared useMoveTask hook surface-agnostic"
    - "Dialog variant derivation from the fresh query RESPONSE only (loading renders a Skeleton body with footer disabled — no variant guessing from cache)"

key-files:
  created:
    - web/src/components/task/CleanupWorktreeDialog.tsx
  modified:
    - web/src/components/board/Board.tsx
    - web/src/pages/TaskPage.tsx

key-decisions:
  - "Verbatim copy strings kept as single template literals in JSX so the UI-SPEC phrases stay grep-able and Prettier can never split them"
  - "Type-to-confirm input autoFocus relies on mount timing: the input mounts only after the fresh state arrives (post-open), so the attribute wins over Radix's cancel-button default focus"
  - "Cleanup onError refetches the worktree state query so a 409 raced-state response re-derives the correct variant in place (Pitfall 8 server re-check showing through)"

patterns-established:
  - "Board renders the cleanup dialog from a Task|null state slot set only by the move's server response — re-entering Done always re-prompts, no suppression flag"

requirements-completed: [GIT-02, GIT-03]

# Metrics
duration: 6min
completed: 2026-06-10
---

# Phase 3 Plan 05: Cleanup Dialog & Triggers Summary

**The GIT-02/GIT-03 user surface: one CleanupWorktreeDialog composing four variants from state fetched at open (clean / N-sessions-stopped / dirty type-to-confirm / both), wired to fire after every persisted move into Done and from the task-view ellipsis menu — never a silent kill, branch always kept**

## Performance

- **Duration:** 6 min
- **Started:** 2026-06-10T15:21:04Z
- **Completed:** 2026-06-10T15:27:30Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments

- CleanupWorktreeDialog (194 lines) renders all four UI-SPEC variants from `useWorktreeState(taskId, open)` — the response, never board cache (Pitfall 8): clean shows `The worktree for "{title}" will be removed.`; sessions show the pluralized stop line + `Stop sessions and clean up` CTA (D-32); dirty trees show the neutral TriangleAlert warning row (`{N} changed files... permanently deletes those changes`) gated behind typing the exact worktree dir name (D-33); every variant ends `The branch {branch} is kept.` in mono (D-34)
- The gate is real: CTA disabled until the case-sensitive basename match; Enter in the input activates confirm only when enabled; pending collapses stop+remove into one `Cleaning up…` state; failure renders `Couldn't remove the worktree: {git error}` inline, re-enables the CTA, AND refetches state so a raced 409 re-derives the variant
- Declining (cancel/Esc) only closes — no code path in the component stops sessions or removes anything without the explicit confirm; a 404 on open (worktree vanished between trigger and open) closes silently
- Board.tsx fires the offer from a call-site `onSuccess` on `moveTask.mutate` — after the move persists, predicated on the server response's `status === "done" && worktree_path`, never blocking or rolling back the move; the shared useMoveTask hook is untouched; cancel reads `Keep worktree` on this path
- TaskPage's ellipsis menu gains a neutral `Clean up worktree` item (worktree-bearing tasks only) above a separator and the destructive `Delete task`; this path's cancel reads `Cancel`; post-cleanup the existing hook invalidations flip the meta line to `No worktree yet.` and disable `+` with no extra wiring

## Task Commits

Each task was committed atomically:

1. **Task 1: CleanupWorktreeDialog — four variants, fresh-state fetch, type-to-confirm gate** - `025cdad` (feat)
2. **Task 2: Triggers — Done transition on the board + task-view menu item** - `49571a5` (feat)

## Files Created/Modified

- `web/src/components/task/CleanupWorktreeDialog.tsx` — the dialog family: variant derivation, gate, pending/failure states, verbatim UI-SPEC copy
- `web/src/components/board/Board.tsx` — `cleanupTask` state + call-site onSuccess in handleDragEnd, dialog rendered after DragOverlay with `trigger="done"`
- `web/src/pages/TaskPage.tsx` — `Clean up worktree` menu item + DropdownMenuSeparator, dialog with `trigger="menu"`

## Decisions Made

- Verbatim copy phrases (`is kept.`, `permanently deletes those changes`) kept inside single string/template literals so JSX line-wrapping can never split the contract strings (two acceptance greps initially failed on wrapped JSX text — restructured)
- Input autoFocus works through mount timing: the type-to-confirm block mounts only after the async state fetch resolves, after Radix's open-autofocus has already run, so the attribute reliably lands focus
- `onError` at the mutate call site triggers `state.refetch()` — the dialog stays open showing the error while the body re-derives against post-409 reality

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- `web/dist/index.html` still carries the pre-existing uncommitted local build output (logged in 03-03's deferred-items); this plan's `npm run build` regenerated it — left uncommitted per the Phase 01 "real build output never committed" decision
- Plan-level dev smoke (drag-to-Done → variant (a); dirty + running session → variant (d) → type-to-confirm → tabs gone, branch kept) requires a browser and is deferred to 03-06, the phase's dedicated verification plan — automated verification (tsc, production build, all acceptance greps) is green

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 03-06 verifies in the browser: all four dialog variants against live worktree state, the Done-drag trigger, the menu trigger, type-to-confirm gating, post-cleanup absent state + branch survival (`git branch --list 'task/*'`)
- Phase success criteria 3 and 4 (offer-on-Done keeping the branch; dirty warning with typed confirmation; explicit stop-sessions gate; no silent path) are implemented end-to-end and await human verification

## Known Stubs

None — the dialog and both triggers are fully wired to the live 03-03/03-04 API surface.

---
*Phase: 03-worktree-isolation-bash-tabs*
*Completed: 2026-06-10*

## Self-Check: PASSED

CleanupWorktreeDialog.tsx and the SUMMARY exist on disk; both task commits (025cdad, 49571a5) present in git log; `npx tsc -b --noEmit`, `npm run build`, and every acceptance-criteria grep pass; mutations.ts confirmed byte-identical (shared hook untouched).
