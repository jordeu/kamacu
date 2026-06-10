# Phase 3: Worktree Isolation & Bash Tabs - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-10
**Phase:** 3-worktree-isolation-bash-tabs
**Areas discussed:** Branch & worktree naming, Base branch & failures, Bash tabs UX, Done & cleanup flow

---

## Branch & Worktree Naming

| Option | Description | Selected |
|--------|-------------|----------|
| kangent/<slug>-<id> (Recommended) | App-namespaced branches | |
| task/<slug>-<id> | Generic namespace, same structure | ✓ |
| <slug>-<id>, no prefix | Shortest, mixes with user branches | |

| Option | Description | Selected |
|--------|-------------|----------|
| ~/.kangent/worktrees/... (Recommended) | Central app-managed location | ✓ |
| Sibling of the repo | <repo>-worktrees/ next to checkout | |
| You decide | Claude picks | |

---

## Base Branch & Failures

| Option | Description | Selected |
|--------|-------------|----------|
| Default branch tip (Recommended) | origin/HEAD → main/master, fallback current HEAD | ✓ |
| Repo's current HEAD | Whatever is checked out | |
| Per-project setting | Configurable base branch field | |

| Option | Description | Selected |
|--------|-------------|----------|
| Task still created, marked (Recommended) | 'No worktree' warning state + Retry | ✓ |
| Creation fails atomically | No task without worktree | |
| You decide | Claude picks | |

| Option | Description | Selected |
|--------|-------------|----------|
| Lazy: create on demand (Recommended) | Create worktree button on old tasks | ✓ |
| Backfill on upgrade | Server creates all at startup | |

---

## Bash Tabs UX

| Option | Description | Selected |
|--------|-------------|----------|
| + button on the tab strip (Recommended) | Explicit spawn, Bash 1/2/... | ✓ |
| One default bash tab | Always-present tab, spawn on click | |
| You decide | Claude picks | |

| Option | Description | Selected |
|--------|-------------|----------|
| Tabs mirror live sessions (Recommended) | Server is source of truth on revisit | ✓ |
| Tabs are per-visit | Tabs drop on leave | |

| Option | Description | Selected |
|--------|-------------|----------|
| Close = stop session (Recommended) | × stops (SIGTERM→SIGKILL) and removes | ✓ |
| Detach only | × hides; session keeps running | |
| You decide | Claude picks | |

---

## Done & Cleanup Flow

| Option | Description | Selected |
|--------|-------------|----------|
| On move to Done (Recommended) | Drag to Done pops the offer | ✓ (effectively "Both" — also a task-view action, captured as D-31) |
| Only in task view | Menu action only | |
| Both | Prompt + menu action | |

| Option | Description | Selected |
|--------|-------------|----------|
| Offer to stop them (Recommended) | "Stop sessions and clean up" in dialog | ✓ |
| Hard refuse | Must stop sessions manually first | |

| Option | Description | Selected |
|--------|-------------|----------|
| Type-to-confirm delete (Recommended) | Dirty tree requires typing task slug | ✓ |
| Block until clean | Must commit/stash first | |

---

## Claude's Discretion

- Slug rules, dialog copy, DB schema, session→task association mechanics
- /terminal dev route fate
- Submodule handling

## Deferred Ideas

- Stale-worktree purge list (v2 MAINT-01)
- Worktree status on cards (maybe with Phase 4 badges)
- Per-project base-branch setting
