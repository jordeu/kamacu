# Phase 9: tmux Restart Resume & Cleanup Integration - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-13
**Phase:** 09-tmux-restart-resume-cleanup-integration
**Areas discussed:** Restored-tab UX after restart, Reaper clock & setting, Cleanup & orphan handling, Reaper visibility & edges

---

## Restored-tab UX after restart (TMUX-05)

| Option | Description | Selected |
|--------|-------------|----------|
| Auto-reattach, invisible | Tab silently reappears + reconnects, no button/banner | ✓ |
| Explicit Resume button (agent-style) | Ghost tab with Resume button, mirrors Phase 5 D-54 | |
| Auto-reattach, connect on click | Tab reappears but PTY connects only on click | |

**User's choice:** Auto-reattach, invisible → D-88
**Notes:** Justified divergence from the agent explicit-Resume because tmux reattach is cheap/lossless (no tokens, no new process). Stays invisible per D-77/D-78.

| Option (dead-on-restart) | Description | Selected |
|--------|-------------|----------|
| Tab silently absent, GC the row | No tab, lazy row delete — matches D-58 | ✓ |
| Exited stub tab | Show exited state for the dead session | |

**User's choice:** Tab silently absent → D-89

## Reaper clock & setting (REAP-01)

| Option (clock) | Description | Selected |
|--------|-------------|----------|
| Dedicated done_at timestamp | New column, time-in-Done, edits don't reset | ✓ (extended) |
| Reuse updated_at | TTL from last edit of any kind | |

**User's choice (verbatim):** "add a done_at timestamp but also: todo_at, inprogress_at and inreview_at for consistency and it will be useful in the future to show some stats" → D-90 (full per-status timestamp set; reaper keyed on done_at, others banked for future stats)

| Option (setting format) | Description | Selected |
|--------|-------------|----------|
| Duration string, default "24h" | Go-style duration; empty/0/never disables | ✓ |
| Whole hours, default 24 | Integer hours; 0 disables | |

**User's choice:** Duration string "24h" → D-91

## Cleanup & orphan handling (TMUX-08)

| Option (count) | Description | Selected |
|--------|-------------|----------|
| Count them in, one number | Live tmux folded into "N sessions running" | ✓ |
| Distinct tmux line | Break out "N tmux sessions will be terminated" | |

**User's choice:** One unified number → D-92

| Option (orphan kill) | Description | Selected |
|--------|-------------|----------|
| Kill by DB rows + orphan sweep | Kill tracked sessions before remove + startup sweep | ✓ |
| Kill by DB rows only | No startup sweep | |

**User's choice:** DB rows + startup orphan sweep → D-93

Follow-ups:
| Question | Selected |
|----------|----------|
| Sweep timing | **Once at startup, DB-driven** (not periodic) → D-93 detail |
| Add ListSessions to internal/tmux? | **Yes** — keep all tmux verbs in the leaf package → D-94 |

## Reaper visibility & edges (REAP-01)

| Option (UX) | Description | Selected |
|--------|-------------|----------|
| Nothing special; reflect reality on open | No notification/badge; sessions gone on next open | ✓ |
| Subtle card indicator | Mark reaped Done tasks on the board | |

**User's choice:** Silent, reflect reality → D-95

| Option (agent reap) | Description | Selected |
|--------|-------------|----------|
| Kill PTY, keep resumable | Stop agent process; transcript + csid remain → resumable ghost | ✓ |
| Exclude agent from reaper | Only bash + tmux reaped | |

**User's choice:** Kill PTY, keep resumable → D-96

## Claude's Discretion

- Reaper tick interval; existing-row backfill of per-status timestamps; restored-tab labels/counter; eager vs on-activate connect for non-active restored tabs; dialog/settings/log copy; README note on `tmux -L kangent kill-server`.

## Deferred Ideas

- Board cycle-time/dwell stats on the new per-status timestamps (banked now, surfaced later)
- Subtle board indicator for reaped tasks (declined for invisibility)
- Worktree auto-removal on TTL (declined in Phase 8, D-87)
- Periodic orphan sweep (declined — startup-only sufficient)
