# Phase 24: Session-Bar, Board & Tab Polish - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-07-03
**Phase:** 24-session-bar-board-tab-polish
**Areas discussed:** Tab rename UX, Tab persistence rules, Review-prompt menu, Agent header layout

Areas offered but pre-resolved as clear-cut (captured without discussion): POLISH-01
(drop total count), POLISH-02 (auto-collapse on outside click), POLISH-03 (remove To
Do `+ New task` — task creation survives via the board-header button + `n` shortcut).

---

## Tab rename UX

| Option | Description | Selected |
|--------|-------------|----------|
| Double-click inline edit | Double-click the label → inline `<Input autoFocus>` in the strip; Enter/Esc/blur commit/cancel; reuses the task-title edit pattern; single-click still selects. | ✓ |
| Per-tab ⋯ menu | Small ⋯ dropdown on each bash tab with Rename / Stop&close; more discoverable but adds a second control to every trigger. | |
| Hover pencil icon | Pencil icon appears on hover next to the ×; clicking starts inline edit; icon clutter. | |

**User's choice:** Double-click inline edit.
**Notes:** Explicitly likened to the existing task-title double-click editor. Applies to bash/tmux tabs only; Agent/Description/Diff stay fixed.

---

## Tab persistence rules

**Q1 — persistence contract given plain-bash tabs don't survive a restart**

| Option | Description | Selected |
|--------|-------------|----------|
| Rename any tab; persist what survives | Rename on every bash tab; name persists in memory (survives reopen) for all; additionally survives restart for tmux tabs via `tmux_sessions.label`; plain-bash tabs vanish on restart anyway; also fix the `Bash ?` bug by persisting the default `Bash N`. | ✓ |
| Restrict rename to tmux tabs only | Only tmux-backed tabs get a rename affordance so rename always implies full restart-persistence; default-shell (plain bash) user can't rename at all. | |

**User's choice:** Rename any tab; persist what survives.

**Q2 — empty / whitespace-only name commit**

| Option | Description | Selected |
|--------|-------------|----------|
| Revert to auto default | Empty commit resets to the original `Bash N`; whitespace-only treated as empty; trimmed before save. | ✓ |
| Reject empty, keep previous | Empty commit ignored; tab keeps its previous name; no reset short of retyping. | |

**User's choice:** Revert to auto default.
**Notes:** Default shell is `bash` (not tmux), which is why restart-persistence only meaningfully applies to opt-in tmux tabs. The `tmux_sessions.label` column already exists but is currently never written (the `Bash ?` latent bug).

---

## Review-prompt menu

| Option | Description | Selected |
|--------|-------------|----------|
| PR reviews only, while running | Item shows only when task is a PR review AND `pr_review_seed` template non-empty AND agent running; mirrors today's seed derivation; non-PR agent menu just has Stop. | ✓ |
| Any task, while running | Offer for every agent using the template; but the template interpolates PR `<n>`/`<title>` → half-broken for non-PR. | |
| Always visible, disabled when N/A | Always render, gray out when not a PR review / not running; adds a permanently-dead control to non-PR views. | |

**User's choice:** PR reviews only, while running.
**Notes:** REVMENU-02 — remove the connect-time auto-paste; the same bracketed-paste mechanism now fires only from the menu item (still prefilled, never auto-sent).

---

## Agent header layout

| Option | Description | Selected |
|--------|-------------|----------|
| Everything in the ⋯ menu | Header right side = single ⋯ (mirrors task-actions ⋯). While running: Insert description (if desc non-empty) → Insert review prompt (PR only) → separator → Stop. Insert description folds into the menu. | ✓ |
| Keep Insert description as a button | Insert description stays a one-click button; only Stop + Insert review prompt move into the ⋯. Header: [Insert description] [⋯]. | |

**User's choice:** Everything in the ⋯ menu.
**Notes:** ⋯ replaces Stop on the agent pane ONLY — bash panes (shared `TerminalPane`) keep their inline Stop button.

---

## Claude's Discretion

- Rename API shape (`PATCH /api/sessions/{id}` with `{label}` or similar) and making `Session.label` mutable under the manager mutex.
- Whether the default `Bash N` is stored at spawn (extend the INSERT) or re-derived from the tmux name's `<n>` on reconcile.
- The `TerminalPane` mechanism for the agent-only header-menu swap.
- Rename behavior on exited / closing / orphaned-survivor bash tabs.
- Outside-click listener details (capture phase, inside-surface set, whether Escape also collapses).
- Menu copy/ordering, dropdown alignment, label max-length / truncation.

## Deferred Ideas

None — discussion stayed within phase scope. Explicitly ruled out (not deferred):
showing "Insert review prompt" for non-PR tasks; restricting rename to tmux tabs only.
