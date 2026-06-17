# Phase 16: Sharper Review Column - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-17
**Phase:** 16-sharper-review-column
**Areas discussed:** CI status icons, Open-session emphasis, Recently Reviewed section, Open-session wiring (+ border-color follow-up)

---

## CI status icons

| Option | Description | Selected |
|--------|-------------|----------|
| Circle icons, static run | CircleCheck / CircleX / CircleDot; running = static orange CircleDot | |
| Circle icons, spinner run | CircleCheck / CircleX; running = spinning LoaderCircle (orange) | |
| Bare check / cross | Plain Check (green) / X (red) glyphs, no ring; running = thin Circle outline (orange) | ✓ |

**User's choice:** Bare check / cross
**Notes:** Lightweight CI indicator; running stays a static orange circle outline (no spinner). `none` renders nothing (carried from milestone questioning).

---

## Open-session emphasis (left border)

| Option | Description | Selected |
|--------|-------------|----------|
| Always-on session border | Distinct colored left border whenever a session is open, regardless of agent state | ✓ |
| Waiting-only (task parity) | Border tints only while waiting (exact D-44 parity); idle/working sessions marked by the dot alone | |

**User's choice:** Always-on session border
**Notes:** Intentionally diverges from the task-card D-44 waiting-only rule so "which PRs am I working on" stays visible even when the agent is idle/working.

---

## Border color follow-up

| Option | Description | Selected |
|--------|-------------|----------|
| Blue edge, amber when waiting | Blue left border for any session; shifts to pulsing amber when the agent is waiting | ✓ |
| One color, state via dot | Single fixed accent border; state conveyed entirely by the dot | |

**User's choice:** Blue edge, amber when waiting
**Notes:** Two distinguishable edges — blue = "session here", amber = "needs input" — preserving the established "waiting = amber" meaning.

---

## Recently Reviewed section

| Option | Description | Selected |
|--------|-------------|----------|
| Labeled subheader | "Recently reviewed" divider below the awaiting-review list, same scroll area, not independently collapsible; most-recent first; drafts excluded; no cap | ✓ |
| Own collapse toggle | Independent collapse chevron, default expanded | |

**User's choice:** Labeled subheader
**Notes:** Whole column already collapses as one; simplest for v1. Quietly omitted when empty.

---

## Open-session wiring

| Option | Description | Selected |
|--------|-------------|----------|
| Extend existing surfaces | Add pr_number(+source) to /api/agents/status (reuse 5s poll); add reviewed-by list to the existing pull-requests endpoint + cache; server-side dedup | ✓ |
| New dedicated endpoints | Separate PR→session and reviewed-PRs endpoints; second poll loop | |
| You decide | Claude picks during research/planning | |

**User's choice:** Extend existing surfaces
**Notes:** One poll, one cached gh cycle. PRCard reuses the shared `useAgentStatuses` query; the reviewed query rides the same per-repo TTL cache `Service`.

---

## Claude's Discretion

- Exact Tailwind class values for CI icons and the blue/amber left border.
- Card row element ordering (recommended: title · agent dot · CI icon · ↗).
- JSON field names (`reviewed` array; `prNumber`/`source` on agent entries).
- Whether the Recently-reviewed subheader shows its own count.
- Non-blocking lint cleanup of the `Date.now()`-in-render advisories in the two touched files.

## Deferred Ideas

- Time window / cap on Recently Reviewed (REVWD-FUT-01).
- Review-decision badge on reviewed cards (REVWD-FUT-02).
- Richer PR card fields (GHCARD-01..04).
- Independent collapse for the Recently-reviewed subsection.
