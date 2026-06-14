# Phase 13: PR Worktree Auto-Cleanup - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-14
**Phase:** 13-pr-worktree-auto-cleanup
**Areas discussed:** Manual cleanup affordance, Merged/closed state in review view, PR task row fate, Auto-removal gate strictness

---

## Area selection

| Option | Selected |
|--------|----------|
| Manual cleanup affordance (GHCLN-03) | ✓ |
| Merged/closed state in the review view | ✓ |
| Fate of the PR task row after auto-cleanup | ✓ |
| Auto-removal gate strictness (GHCLN-02) | ✓ |

**User's choice:** All four.

---

## Manual cleanup affordance (GHCLN-03)

| Option | Description | Selected |
|--------|-------------|----------|
| Re-add ⋯ menu → Clean up worktree (Recommended) | Bring back the ⋯ menu for PR reviews with one 'Clean up worktree' item → existing gated CleanupWorktreeDialog | ✓ |
| Dedicated 'Clean up' button | A visible cleanup button in the review header/meta | |

**User's choice:** Re-add ⋯ menu → Clean up worktree. → CONTEXT D-08.

---

## Merged/closed state in the review view

| Option | Description | Selected |
|--------|-------------|----------|
| Banner with cleanup prompt (Recommended) | Plumb PR state into the GET detail endpoint; show 'This PR was merged/closed' + manual-cleanup prompt for skipped/leftover worktrees | ✓ |
| Silent degrade | No special UI; worktree-less view behaves like a task without a worktree | |

**User's choice:** Banner with cleanup prompt. → CONTEXT D-09.

---

## Fate of the PR task row after cleanup

| Option | Description | Selected |
|--------|-------------|----------|
| Auto: delete row; manual: keep nulled (Recommended) | Merged/closed auto-cleanup deletes the row (review over, no orphans); manual cleanup keeps the row nulled (reopen re-provisions) | ✓ |
| Always keep, null the worktree | Both paths keep the row; merged-PR rows accumulate as invisible orphans | |
| Always delete the row | Both paths delete; open review 404s after an auto-clean | |

**User's choice:** Auto: delete row; manual: keep nulled. → CONTEXT D-07.

---

## Auto-removal gate strictness (GHCLN-02)

| Option | Description | Selected |
|--------|-------------|----------|
| Conservative — block on anything (Recommended) | Skip auto-removal on ANY of: uncommitted changes, unpushed/local-only commits (rev-list), a stash, or a running/detached session (incl. live agent) | ✓ |
| Match the existing manual gate only | Block only on uncommitted changes + running sessions; no unpushed/stash checks | |

**User's choice:** Conservative — block on anything. → CONTEXT D-05.

## Claude's Discretion

- cleanupWorktreeGated signature + how the reaper signals "delete row after auto-remove"; PRState as a new method vs ViewPR extended with state; banner copy/placement; reaper pass batching/rate-limit; optional refresh-triggered reconcile.

## Deferred Ideas

- Stale pr/<n> branch-ref purge; snappier refresh-triggered reconcile; auto-cleanup notifications.
