---
phase: 14-managed-checkout-foundations
plan: 04
subsystem: api
tags: [projects, delete, worktree, gated-cleanup, gh, clone, go, sqlite]

# Dependency graph
requires:
  - phase: 14-managed-checkout-foundations
    provides: "projectHandlers.wt/mgr/tmuxClient deps (14-01); the managed marker + scanProject (14-01); createByRepo in projects.go (14-02)"
  - phase: 13-pr-worktree-auto-cleanup
    provides: "CleanupWorktreeGated shared gated-removal core + the worktree gate primitives (DirtyCount/UnpushedCount/StashCount/Remove) the managed delete reuses"
provides:
  - "Managed-project gated delete: a two-pass all-or-nothing branch in projectHandlers.delete that gates every task/PR worktree AND the clone root on dirty/unpushed/stash/sessions, refuses with a 409 reasons list when any gate trips (removes nothing), and on all-clear removes linked worktrees first → os.RemoveAll(clone) → FK-ordered rows (204)"
  - "deleteBlocker {kind,target} structured 409 payload the Phase 15 cleanup dialog can enumerate"
  - "session-count helpers (runningSessions/liveTmuxNames/cleanupSessionCount) on projectHandlers"
affects: [15-repo-first-creation-flow]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Two-pass gate-all-then-remove-all for an atomic multi-target delete: the GATE phase computes the four gates over N worktrees + the clone mutating NOTHING; the REMOVE phase runs only if every gate passed (all-or-nothing, D-07)"
    - "Order-load-bearing teardown: linked worktrees removed (CleanupWorktreeGated → wt.Remove) BEFORE os.RemoveAll(clone); the clone root is the main worktree and is removed with os.RemoveAll, never `git worktree remove`"
    - "Network-free conservative unpushed gate: rev-list origin/<default>..HEAD with NO fetch; an unresolved default branch is treated as a blocker (fail safe, not fail open)"
    - "Marker-gated disk removal: the managed=1 column is the single source of truth for `Kangent owns this dir`; folder (managed=0) delete is byte-for-byte unchanged, dir never touched"

key-files:
  created:
    - "internal/api/projects_delete_test.go"
  modified:
    - "internal/api/projects.go"

key-decisions:
  - "Implemented the remove phase (removeManaged) in the same commit as the gate phase (Task 1 GREEN) because deleteManaged structurally references it to compile — the same shared-helper TDD sequencing 14-02 used for reattachManaged; Task 2's commit adds the all-clean/ordering/folder tests that prove the remove-phase behavior."
  - "The clone-root unpushed gate and every task-worktree unpushed gate share ONE base, origin/<default> (resolved once via worktree.DefaultBranch); a DefaultBranch read failure becomes a conservative `unpushed` blocker on the managed checkout rather than silently skipping the gate (D-08 safety net)."
  - "PR-review worktrees (source='github_pr') are gated/removed alongside task worktrees — no source filter on the enumeration query — because they branch off the same managed clone."
  - "409 payload is a structured { error, reasons:[{kind,target}] } list (research Open Question 3, recommended) so a Phase 15 dialog can render per-blocker; kind ∈ uncommitted|unpushed|stash|sessions, target ∈ `task #<id>`|`the managed checkout`."

patterns-established:
  - "Multi-target atomic delete: gate ALL targets first, mutate NONE during gating, then remove ALL only on all-clear (Pitfall 8)."
  - "FK-ordered row cleanup mirrors reaper.go/tasks.go: DELETE tmux_sessions of the project's tasks, then DELETE the project (CASCADE removes task rows)."

requirements-completed: [CKOUT-03]

# Metrics
duration: 7 min
completed: 2026-06-14
---

# Phase 14 Plan 04: Gated Managed-Clone Delete Summary

**A two-pass, all-or-nothing managed-project delete in `projectHandlers.delete`: it gates every task/PR worktree AND the clone root on dirty/unpushed/stash/running-session (unpushed via `origin/<default>..HEAD`, no fetch), refuses with a 409 `{reasons:[{kind,target}]}` list that removes nothing, and on all-clear tears linked worktrees down before `os.RemoveAll`'ing the clone then deleting the rows (204) — while folder (managed=0) delete stays byte-for-byte unchanged.**

## Performance

- **Duration:** ~7 min
- **Started:** 2026-06-14T17:36:09Z
- **Completed:** 2026-06-14T17:43:34Z
- **Tasks:** 2 (both TDD)
- **Files modified:** 2 (1 created, 1 modified)

## Accomplishments

- **Managed gated delete (CKOUT-03/D-07).** `projectHandlers.delete` now loads `managed` + `repo_path` and branches: `managed=0` runs today's exact `DELETE FROM projects` (dir untouched, D-09); `managed=1` runs `deleteManaged`. The GATE PHASE enumerates the project's task/PR worktrees (`worktree_path IS NOT NULL`, no source filter) and computes `DirtyCount` / `UnpushedCount(origin/<default>)` / `StashCount` / `cleanupSessionCount` per worktree AND on the clone root — mutating nothing. Any blocker → 409 with the aggregated `reasons` list and **nothing removed** (two-pass, Pitfall 8).
- **Order-load-bearing remove phase (D-08/Pitfall 1/2).** On all-clear, `removeManaged` calls the shared `CleanupWorktreeGated` once per linked worktree (stop sessions → kill tmux → `wt.Remove` → null columns), THEN `os.RemoveAll(clone)` (the main worktree + `.git`), THEN FK-ordered row deletes (`tmux_sessions` of the project's tasks, then the project; CASCADE removes task rows) → 204. The clone root is **never** passed to `wt.Remove` (git refuses the main worktree, exit 128).
- **D-08 unpushed safety net.** The unpushed gate base is `origin/<default>` resolved once via `worktree.DefaultBranch`, with **no fetch** (network-free, conservative — research Open Question 2). A `DefaultBranch` read failure is a conservative `unpushed` blocker on the managed checkout, so the delete never proceeds when local-only branches can't be verified.
- **Structured 409 contract for Phase 15.** Added `deleteBlocker{Kind, Target}`; the 409 body is `{ "error": "the project can't be deleted yet", "reasons": [ {kind, target}, ... ] }` (kinds: `uncommitted`/`unpushed`/`stash`/`sessions`; targets: `task #<id>` / `the managed checkout`) so the future cleanup dialog can enumerate blockers.
- **Folder path proven unchanged (D-09/PROJ-03).** `TestFolderDeleteNeverTouchesDir` deletes a `managed=0` project and asserts the directory still exists on disk + the row is gone (204). The managed disk-removal code is unreachable for folder projects.

## Task Commits

Each task committed atomically (TDD: test → feat → test):

1. **Task 1: Gate phase — two-pass gate, 409 reasons, nothing removed** (TDD)
   - `71ed440` (test/RED) — failing gate tests (dirty / unpushed / stash / clone-root-unpushed → 409 + reasons + nothing removed)
   - `8e9c2da` (feat/GREEN) — managed branch in `delete`: gate phase + `deleteBlocker` payload + `removeManaged` scaffold + session-count helpers
2. **Task 2: Remove phase — ordered worktree→clone removal, row delete, 409 shape** (TDD)
   - `55d33a0` (test) — all-clean (204, clone+worktree+rows gone), ordering (no orphaned worktree), folder-untouched, unknown-id 404

_The remove-phase implementation landed in Task 1's GREEN commit because `deleteManaged` structurally references `removeManaged` to compile — the same shared-helper TDD sequencing 14-02 used for `reattachManaged`. Task 2's commit adds the tests that rigorously prove the remove-phase behavior._

## Files Created/Modified

- `internal/api/projects.go` — `delete` loads `managed`+`repo_path` and branches (folder unchanged / managed → `deleteManaged`); new `deleteManaged` (gate phase, two-pass), `removeManaged` (ordered remove phase), `projectWorktrees` enumeration, `deleteBlocker`/`managedTaskWorktree` types, `gateReasonKind`, and `runningSessions`/`liveTmuxNames`/`cleanupSessionCount` replicated on `projectHandlers`; added `session` import usage
- `internal/api/projects_delete_test.go` (new) — `makeManagedProject` (file:// clone fixture), `addManagedTaskWorktree`, `assertBlocked`, and 8 tests: 4 gate tests (dirty/unpushed/stash/clone-root-unpushed → 409 + reasons + nothing removed), all-clean (204 + clone/worktree/rows gone), ordering (no orphaned worktree), folder-never-touched (D-09), unknown-id 404

## Decisions Made

- **Remove phase implemented in Task 1 GREEN (structural dependency).** `deleteManaged` calls `removeManaged` on its all-clear path, so the function must exist for the managed branch to compile. Rather than leave Task 1 non-compiling, the full remove phase landed in the Task 1 GREEN commit and Task 2 added the dedicated all-clean/ordering/folder tests. This mirrors 14-02's `reattachManaged` sequencing precedent.
- **One shared `origin/<default>` unpushed base for the clone + all worktrees.** Task worktrees branch off the clone's default branch, so `origin/<default>..HEAD` is the correct conservative, network-free base for every target. Resolved once; an unresolvable base is a blocker (fail safe).
- **PR-review worktrees included.** The enumeration has no `source` filter — `github_pr` worktrees branch off the same clone and must be gated/removed too.
- **`cleanupSessionCount`/`liveTmuxNames`/`runningSessions` duplicated (~30 LOC) onto `projectHandlers`** rather than factoring a shared helper, per the plan's interface note — keeps the package boundary clean and matches the proven `worktreeHandlers` computation.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] removeManaged implemented in Task 1 (GREEN) rather than purely in Task 2**
- **Found during:** Task 1 (GREEN)
- **Issue:** `deleteManaged`'s all-clear path references `removeManaged`, so the function must exist for the managed branch to compile. A pure RED-first Task 2 (no remove-phase code yet) would have left Task 1's GREEN non-compiling.
- **Fix:** Added the full `removeManaged` implementation in the Task 1 GREEN commit (`8e9c2da`); Task 2's commit (`55d33a0`) adds the all-clean / ordering / folder-untouched tests that prove the remove-phase behavior (204, clone+worktree+rows gone, no orphaned worktree, folder dir untouched).
- **Files modified:** internal/api/projects.go (Task 1), internal/api/projects_delete_test.go (Task 2)
- **Verification:** `TestManagedDeleteAllClean`/`TestManagedDeleteOrdering`/`TestFolderDeleteNeverTouchesDir` all pass; `os.RemoveAll(clone)` provably runs after the `CleanupWorktreeGated` loop; the clone root is never passed to `wt.Remove`.
- **Committed in:** `8e9c2da` (implementation), `55d33a0` (tests)

---

**Total deviations:** 1 (TDD sequencing for a structurally-required helper — Rule 3 blocking).
**Impact on plan:** None on behavior or scope. The gate phase, remove-phase ordering, 409 reasons shape, and folder-untouched contract are all proven by test; the only change is which commit the `removeManaged` body landed in.

## Issues Encountered

None. `go build ./...`, `go vet ./internal/api/...`, and the full `go test ./internal/...` suite are all green.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- CKOUT-03 closes Phase 14: the managed-checkout backend (clone/provision, marker, pre-task fetch, reattach, gated delete) is complete. **Phase 14 is fully implemented.**
- Phase 15 (Repo-First Creation Flow) can drive the create-by-repo primitive (14-02) and, for the cleanup UX, render the `deleteBlocker` `reasons` list this plan returns on a 409 — the backend contract the future cleanup dialog enumerates is in place.
- No further migration is needed for v1.4 (migration 00008 landed in 14-01).

---
*Phase: 14-managed-checkout-foundations*
*Completed: 2026-06-14*

## Self-Check: PASSED

- All created/modified files exist on disk (projects.go, projects_delete_test.go, 14-04-SUMMARY.md).
- All task commits present in git history (71ed440 test/RED, 8e9c2da feat/GREEN, 55d33a0 test).
- `go build ./...`, `go vet ./internal/api/...`, and `go test ./internal/...` all green.
