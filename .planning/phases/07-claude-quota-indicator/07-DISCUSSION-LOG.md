# Phase 7: Claude Quota Indicator - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-12
**Phase:** 07-claude-quota-indicator
**Areas discussed:** Compact trigger look, No-data states, Popup content details

---

## Compact trigger look

| Option | Description | Selected |
|--------|-------------|----------|
| SlayZone-exact (Recommended) | "Claude 5h" text + thin colored bar underneath | ✓ |
| Label + bar + percentage | Adds rounded % next to the bar | |
| Bar only | Just the thin bar, label only in popup | |

**User's choice:** SlayZone-exact

| Option | Description | Selected |
|--------|-------------|----------|
| Full traffic-light bar (Recommended) | Always green/yellow/red by threshold, SlayZone-exact | |
| Neutral until it matters | Muted zinc <60%, yellow 60–84, red ≥85; green never appears | ✓ |
| You decide | Claude picks honoring palette discipline | |

**User's choice:** Neutral until it matters — declined the recommended option in favor of the project's quiet-palette discipline

| Option | Description | Selected |
|--------|-------------|----------|
| Red bar + red label text (Recommended) | Label also turns red at ≥85% | |
| Red bar only | Bar color alone carries the signal | ✓ |
| You decide | Claude picks something loud but motion-free | |

**User's choice:** Red bar only

---

## No-data states

| Option | Description | Selected |
|--------|-------------|----------|
| Hide trigger entirely (Recommended) | No creds → render nothing | ✓ |
| Grayed chip with explanation | Muted "Claude —" chip with explanatory popup | |

**User's choice:** Hide trigger entirely (no OAuth credentials / API-key-only)

| Option | Description | Selected |
|--------|-------------|----------|
| Warning chip + popup message (Recommended) | Trigger visible with warning; popup says re-authenticate with claude | ✓ |
| Hide trigger | Same as no-creds | |

**User's choice:** Warning chip + popup message (expired/rejected token, no cache)

| Option | Description | Selected |
|--------|-------------|----------|
| Subtle: popup-only notice (Recommended) | Trigger keeps last-known bar; popup footer "error · Xm old" in amber | ✓ |
| Visible on trigger too | Trigger dims / warning dot while stale | |

**User's choice:** Subtle, popup-only staleness

---

## Popup content details

| Option | Description | Selected |
|--------|-------------|----------|
| Full short names (Recommended) | "5h", "7d", "Opus", "Sonnet"; unknown windows render raw key | ✓ |
| SlayZone-exact | Including "Son." truncation | |
| Descriptive | "Session (5h)", "Weekly (7d)", … | |

**User's choice:** Full short names

| Option | Description | Selected |
|--------|-------------|----------|
| API order, 5h first (Recommended) | 5h, 7d, then per-model as returned; stable | ✓ |
| Most-used first | Sort by utilization descending | |

**User's choice:** API order, 5h first

| Option | Description | Selected |
|--------|-------------|----------|
| SlayZone-exact (Recommended) | "Updated Xm ago" left, refresh icon right | ✓ |
| Add last-error line | Footer also shows most recent fetch error | |

**User's choice:** SlayZone-exact footer

---

## Claude's Discretion

- Exact trigger dimensions/spacing, hover affordance, popup width
- Reset countdown format details and the "now"/null case
- Warning-chip visual for error states
- Pre-first-fetch loading state

## Deferred Ideas

- Extra-usage credits block (QUOTA-FUT-01), pinnable inline bars (QUOTA-FUT-02), quota notifications (QUOTA-FUT-03) — already tracked in REQUIREMENTS.md futures
