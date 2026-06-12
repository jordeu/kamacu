---
phase: 07-claude-quota-indicator
plan: 02
subsystem: ui
tags: [react, tanstack-query, radix-ui, shadcn, hover-card, quota]

# Dependency graph
requires:
  - phase: 06-settings
    provides: "TaskPage full-width header row (UI-01) the indicator mounts into"
provides:
  - "QuotaIndicator component: 'Claude 5h' trigger + thin threshold bar, all-windows hover popup"
  - "useQuota (60s visible-only poll) + useRefreshQuota (cache-bypass mutation) on the [\"usage\"] query key"
  - "shadcn hover-card copy-in (radix-ui import, zero new npm deps)"
  - "Header mounts on BoardPage (beside New task) and TaskPage (beside three-dots menu)"
affects: [07-claude-quota-indicator verification, 07-03]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Hand-rolled two-div threshold bars (no Progress primitive) with width/color decoupled"
    - "Mutation-then-setQueryData manual refresh (useSpawnSession precedent)"

key-files:
  created:
    - web/src/components/ui/hover-card.tsx
    - web/src/api/usage.ts
    - web/src/components/quota/QuotaIndicator.tsx
  modified:
    - web/src/pages/BoardPage.tsx
    - web/src/pages/TaskPage.tsx

key-decisions:
  - "Trigger bar width tracks the 5h window while color tracks max utilization across ALL windows (QUOTA-07/D-70)"
  - "Warning chip for empty auth_expired/error states: TriangleAlert in text-amber-400 replacing the bar, motion-free (D-72 discretion per research recommendation)"
  - "'Updated Xm ago' computed at render — the 60s query tick re-renders; no extra timer (research Pitfall 7)"

patterns-established:
  - "Quota threshold palette: bg-zinc-600 <60, bg-amber-400 60-84, bg-red-500 >=85 — the low band stays neutral (D-69)"
  - "Server-driven popup rows: map data.windows as-is, never reorder or hardcode the window set (QUOTA-02/D-75)"

requirements-completed: [QUOTA-01, QUOTA-02, QUOTA-03, QUOTA-04, QUOTA-07]

# Metrics
duration: 5min
completed: 2026-06-12
---

# Phase 7 Plan 02: Quota Indicator Frontend Summary

**"Claude 5h" trigger with width=5h/color=max-of-all threshold bar, server-driven hover popup with reset countdowns and manual refresh, mounted in both page headers polling /api/usage every 60s while visible — zero new npm dependencies**

## Performance

- **Duration:** 5 min
- **Started:** 2026-06-12T04:26:13Z
- **Completed:** 2026-06-12T04:31:39Z
- **Tasks:** 3
- **Files modified:** 5

## Accomplishments

- shadcn `hover-card` copy-in landed via the CLI with a verified-empty `web/package.json` diff (consolidated `radix-ui@1.5.0` import pattern, matching tooltip.tsx)
- `useQuota`/`useRefreshQuota` hooks implement the pinned `/api/usage` contract: 60s `refetchInterval` with `refetchIntervalInBackground: false` (QUOTA-04) and a `?refresh=1` mutation that writes back into the `["usage"]` cache (QUOTA-03)
- QuotaIndicator implements the five-state render mapping exactly: nothing while loading or `no_credentials` (D-71), normal bar + popup when windows exist (with the D-73 amber `error · Xm old` stale footer), and a motion-free amber TriangleAlert warning chip for empty `auth_expired` (`` Token expired — re-authenticate with `claude` ``) and `error` (`Quota unavailable`) states (D-72)
- The QUOTA-07 subtlety is explicit in code: bar width = `five_hour` utilization, bar color = `barColor(Math.max(...windows.map(w => w.utilization)))` — a low 5h bar turns red when any window hits 85
- Mounted per-page (no invented shared header): BoardPage header right group beside New task, TaskPage header between the title block and the DropdownMenu

## Task Commits

Each task was committed atomically:

1. **Task 1: shadcn hover-card copy-in + usage hooks** - `54ef83a` (feat)
2. **Task 2: QuotaIndicator component — trigger, popup, five-state mapping** - `796a7d5` (feat)
3. **Task 3: Mount QuotaIndicator in BoardPage and TaskPage headers** - `3880a8b` (feat)

## Files Created/Modified

- `web/src/components/ui/hover-card.tsx` - shadcn HoverCard/HoverCardTrigger/HoverCardContent copy-in (radix-ui, zero deps)
- `web/src/api/usage.ts` - UsageWindow/UsageResponse types, useQuota poll hook, useRefreshQuota mutation
- `web/src/components/quota/QuotaIndicator.tsx` - trigger + popup, threshold colors, formatAgo/formatReset helpers, five-state mapping (162 lines)
- `web/src/pages/BoardPage.tsx` - header right flex group wrapping QuotaIndicator + New task button
- `web/src/pages/TaskPage.tsx` - QuotaIndicator inserted between title block and three-dots menu

## Decisions Made

- Warning-chip visual (D-72 discretion): kept the `Claude 5h` text with a `size-3.5` `TriangleAlert` in `text-amber-400` replacing the bar — same footprint, motion-free, per the research recommendation
- Reset column: `—` for null `resetsAt` (no prefix), `Resets now` when past, otherwise `Resets in {37m | 4h 12m | 2d 5h}`
- No extra freshness timer: `Updated Xm ago` recomputes on each 60s query tick (minute granularity makes that sufficient)

## Deviations from Plan

None - plan executed exactly as written.

(One non-code note: the verification `npm run build` overwrote the committed `web/dist/index.html` placeholder with real build output; restored via `git checkout` since dist assets are gitignored and the placeholder is what's meant to be tracked.)

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Frontend half of the quota indicator complete; it degrades to "render nothing" until plan 07-01's `GET /api/usage` lands (loading state returns null), so the parallel plans compose safely
- Live six-state behavior (stale footer, warning chips, red-bar cross-window signal) is verifiable end-to-end once 07-01 merges — covered by phase verification / 07-03

## Self-Check: PASSED

All created files verified on disk; all three task commits (`54ef83a`, `796a7d5`, `3880a8b`) verified in git log.

---
*Phase: 07-claude-quota-indicator*
*Completed: 2026-06-12*
