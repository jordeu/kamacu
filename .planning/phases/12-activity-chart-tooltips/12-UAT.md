---
status: complete
phase: 12-activity-chart-tooltips
source: [12-VERIFICATION.md]
started: 2026-08-01T14:05:00Z
updated: 2026-08-01T14:25:00Z
---

# Phase 12 — Human Verification (UAT)

All automated checks passed (build exit 0, lint exit 0, 10/15 must-haves structurally verified, requirements CHART-01/STAT-01/ENTRY-01 satisfied, no gaps). The 10 items below require human visual/interaction testing before this phase can be marked complete — this project's frontend has no `npm test` runner, so visual/interaction/runtime-state UAT is deferred here.

**How to run:** start the dev server (`cd web && npm run dev`, or the full app), open the Activity page, and walk through each test. Mark each `result:` as `pass` / `fail` / `blocked`. On failure, describe the issue inline.

## Current Test

[testing complete]

## Tests

### 1. Chart renders 7 stacked-bar columns in Week window
expected: All 7 bars present (D-05); tasks (muted blue #5e719c) bottom, reviews (muted teal #46776f) top (D-04); weekday labels legible in dark theme.
result: pass

### 2. Hover tooltip on every Week bar (incl. zero-day stubs)
expected: Tooltip appears with absolute date header (e.g. `Wed, Jul 29`) + two rows (Tasks / Reviews) with counts. Zero-day bars show `0 / 0`. Cursor highlight is muted (`var(--accent)` @ 40%).
result: pass

### 3. Month window shows sparse markers + 30 bars
expected: 30 bars render with ~5 sparse date markers (indices 0,7,14,21,28 → `M/D` labels like `7/2`); timeline does NOT crowd (D-06). Zero-activity days still show the 2px baseline stub.
result: pass

### 4. Zero-day baseline stub renders correctly
expected: A 2px muted (`var(--border)`) baseline tick sits at the x-axis baseline — NOT a gap, NOT a full-height bar. Confirms D-05 ZeroDayStub custom `shape` renders correctly (Assumption A3 — MEDIUM risk).
result: pass

### 5. Scope/window change rebuckets the chart
expected: Toggling scope (Global → Workspace → Project) and window (Week ↔ Month) rebuckets the chart from the fresh useActivity data. No new fetch spinner, no chart-local loading state.
result: pass

### 6. HARD_DEGRADE renders tasks-only bars (D-07)
expected: With GitHub integration disabled (or gh absent), the reviews segment reads 0 on every bar (tasks-only bars); the chart never breaks or hides. Reviews-done list below suppresses entirely (Phase 11 D-12).
result: pass

### 7. Stat tooltip opens on hover/focus with D-08 copy
expected: Hover/focus the Info (i) icon beside each of the three time-stat rows (Cycle, In progress, In review). Tooltip opens instantly (delayDuration=0) showing the D-08 copy: metric definition + `min · median · max = fastest · typical · slowest`. Mouse-leave/blur closes it.
result: pass

### 8. Keyboard reachability of stat tooltips
expected: Tab through the StatsStrip — Tab reaches each of the three Info `<button>` elements (aria-label present); focus alone opens the tooltip; Esc dismisses. Counts (taskCount/reviewCount) are NOT in the tab order for tooltips.
result: pass

### 9. Per-entry absolute timestamps render correctly
expected: Each tasks-done and reviews-done entry shows an absolute `Jul 29, 14:32` (compact month-abbrev + day + 24h HH:MM, no weekday/year) — NOT a relative `3h ago` (D-10/D-11). Null/invalid timestamps render as em-dash `—`.
result: pass

### 10. Chart positioned above StatsStrip
expected: Page order top→bottom: heading → control bar (scope + window) → ActivityChart → StatsStrip → tasks-done → reviews-done (D-03).
result: pass

## Summary

total: 10
passed: 10
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps
