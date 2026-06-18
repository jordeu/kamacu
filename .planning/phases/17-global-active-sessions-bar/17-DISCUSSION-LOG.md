# Phase 17: Global Active Sessions Bar - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-18
**Phase:** 17-global-active-sessions-bar
**Areas discussed:** Collapsed summary, Expand behavior, List shape, Scope edges

---

## Gray-area selection

| Option | Description | Selected |
|--------|-------------|----------|
| Collapsed summary | Per-state counts vs attention-first vs total+badge; empty-state wording | ✓ |
| Expand behavior | Overlay-up vs push-content; whole-bar vs chevron toggle | ✓ |
| List shape | Flat attention-first vs grouped-by-project; row content | ✓ |
| Scope edges | Live-only confirm; overflow; current-row highlight | ✓ |

**User's choice:** All four areas.

---

## Collapsed summary

### How to present collapsed stats

| Option | Description | Selected |
|--------|-------------|----------|
| Colored counts + total | Colored dots per state (dotMeta) with counts + total; waiting pulses | ✓ |
| Attention-first | Foreground "waiting", tuck working/idle into a quiet total | |
| Total + waiting badge | "N active" + pulsing "N waiting" badge; no breakdown until expand | |

**User's choice:** Colored counts + total.

### Zero/empty state

| Option | Description | Selected |
|--------|-------------|----------|
| Quiet "No active sessions" | Muted text; bar present but recedes | ✓ |
| Just "0 sessions" | Minimal numeric | |
| Dim the whole bar | Reduced opacity when idle | |

**User's choice:** Quiet "No active sessions".

---

## Expand behavior

### Panel space occupancy

| Option | Description | Selected |
|--------|-------------|----------|
| Float up over content | Overlay upward; content + xterm terminals never reflow/resize | ✓ |
| Push content up | Main area shrinks; terminals resize via fit addon | |

**User's choice:** Float up over content.
**Notes:** Chosen to avoid xterm terminal resize churn.

### Toggle trigger

| Option | Description | Selected |
|--------|-------------|----------|
| Click anywhere on the bar | Whole bar toggles (mirrors ReviewColumn rail); chevron shows direction | ✓ |
| Only a chevron button | Bar passive; dedicated chevron toggles | |

**User's choice:** Click anywhere on the bar.

---

## List shape

### Organization

| Option | Description | Selected |
|--------|-------------|----------|
| Flat, attention-first | One list across projects, waiting→working→idle; project per row | ✓ |
| Grouped by project | Sections per project, attention-sorted within | |

**User's choice:** Flat, attention-first.

### Row content

| Option | Description | Selected |
|--------|-------------|----------|
| State · project · title | Status indicator + project + task/PR title; PR badge for review rows | ✓ |
| Title · project only | No explicit per-row state indicator | |

**User's choice:** State · project · title.

---

## Scope edges

### Live-only confirmation

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, live only | Working/waiting/idle only; exited drops off immediately | ✓ |
| Show exited briefly | Dimmed grace period before drop-off | |

**User's choice:** Yes, live only.

### Current-row highlight

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, highlight current | Subtle "you are here" on the open task's row | ✓ |
| No highlight | All rows equal | |

**User's choice:** Yes, highlight current.

---

## Claude's Discretion

- Overflow handling (height-capped scroll panel, reusing ReviewColumn's `overflow-y-auto`) — defaulted, not asked.
- Exact bar/panel dimensions, animation, loading-vs-empty treatment, highlight styling.

## Deferred Ideas

- SBAR-FUT-01..06 (per-session actions, time-in-state, browser notifications, tab/favicon badge, terminal sessions in bar, group/filter).
- "Show exited briefly" and "Push content up" — considered and rejected during this discussion.
