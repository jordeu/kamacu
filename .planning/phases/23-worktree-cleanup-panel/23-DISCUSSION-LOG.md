# Phase 23: Worktree Cleanup Panel - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-07-02
**Phase:** 23-worktree-cleanup-panel
**Areas discussed:** Permission-blocked removal, "Clean eligible" scope, What counts as a worktree, Panel UX & status loading

---

## Permission-blocked removal

| Option | Description | Selected |
|--------|-------------|----------|
| Surface & instruct | Attempt normal remove; on permission error leave the worktree, mark "Blocked — needs manual removal" with the exact path + copyable `sudo rm -rf` hint, keep git registration + DB row intact. No privilege escalation. | ✓ |
| Best-effort partial | Remove what's deletable, deregister from git + null the DB row, flag the leftover on-disk dir as orphaned junk. | |
| Attempt elevated removal | App tries a privileged delete (sudo/pkexec) to force it through. | |

**User's choice:** Surface & instruct
**Notes:** Directly addresses the memory'd uid-70 `./.db` (container-owned Postgres dir) case; keeps the app an unprivileged localhost process. → CONTEXT D-01.

---

## "Clean eligible" scope

| Option | Description | Selected |
|--------|-------------|----------|
| Orphans + pristine finished | Orphans (no DB task) PLUS gate-passing referenced worktrees whose task is Done / PR merged-closed. Never forces. | ✓ |
| Orphans only | Bulk clears only orphaned worktrees; referenced removed individually. | |
| Any pristine & idle | Any gate-passing worktree regardless of task status, plus orphans. | |

**User's choice:** Orphans + pristine finished
**Notes:** Bulk is never destructive — anything with a tripped gate or blocked is skipped for a deliberate per-item force. → CONTEXT D-04/D-05.

---

## What counts as a worktree

| Option | Description | Selected |
|--------|-------------|----------|
| Union git + DB, all repos | `git worktree list` at every project's repo root + managed clones, unioned with DB rows; git-but-no-task = orphan, DB-row-but-gone = stale pointer (clearable); grouped by project. | ✓ |
| Git list only | Enumerate strictly from `git worktree list` per repo; don't reconcile stale DB rows. | |
| DB rows only | List only DB-tracked worktrees — no orphan detection (under-delivers WTREE-04). | |

**User's choice:** Union git + DB, all repos
**Notes:** Reverse-orphans (stale DB pointers) surfaced with a clear-pointer action. → CONTEXT D-06/D-07.

---

## Panel UX & status loading

| Option | Description | Selected |
|--------|-------------|----------|
| Settings section, eager load + refresh | New section in the Settings page; one GET returns the server-annotated list (dirty/unpushed/stash computed per worktree), fetch-on-open + manual Refresh, no polling; force-remove + bulk each behind a confirm naming what'll be removed. | ✓ |
| Dedicated route | Its own full-page route (/worktrees), same eager-load model. | |
| Lazy per-row flags | List loads instantly; flags computed lazily per row on expand. | |

**User's choice:** Settings section, eager load + refresh
**Notes:** Mirrors the Diff tab's fetch-on-mount + manual-refresh (D-61). → CONTEXT D-09/D-10.

---

## Claude's Discretion

Accepted derived defaults (confirmed at the wrap-up gate): branch always kept on removal
incl. orphans (D-02); force-remove also stops live sessions (D-03); bulk clean skips
blocked/dirty/unpushed/stashed/session-live (D-04); global, not per-project (D-09).
Left open for research/planning: exact API shape + endpoint naming, the
`git worktree list --porcelain` parser, precise "unpushed" base-ref resolution,
confirm-dialog copy/severity, empty-state text, one-GET-vs-list+detail, and modeling the
new "blocked" outcome distinctly from a generic error.

## Deferred Ideas

- WTREE-FUT-01 — scheduled/automatic stale-worktree purge (deferred MAINT-01); this phase is manual only.
- App-driven elevated (sudo/pkexec) removal — rejected for D-01; parked as a fallback.
- Per-project cleanup settings — out of scope for v1.8 (global only).
