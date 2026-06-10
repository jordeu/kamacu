---
phase: 04-claude-code-agent-sessions
plan: 03
subsystem: ui
tags: [react, tanstack-query, tailwind, status-dots, kanban]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: TaskCard/CardShell board anatomy, ProjectSidebar menu rows, shadcn tooltip, zinc dark design system
  - phase: 02-terminal-engine
    provides: useQuery + refetchInterval 5000 polling convention (sessions.ts), api/client get helper
provides:
  - useAgentStatuses hook — shared 5s-polled ["agent-statuses"] query against GET /api/agents/status
  - AgentStatusEntry type (frontend transcription of the 04-02 backend contract)
  - StatusDot component + dotMeta pure mapping — single source of the status palette, reused by 04-04's Agent tab dot
  - Card dots with D-44 waiting treatment (pulse + amber-400/40 border)
  - D-49 sidebar waiting-count chip
affects: [04-04 (Agent tab dot reuses StatusDot + query), 04-05, verification]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "One shared status query (['agent-statuses'], 5s poll) — per-card hook calls dedupe via React Query, no fan-out"
    - "Status palette contained to StatusDot.tsx only; status communicated by dot + tooltip, never colored text"
    - "dotMeta as pure mapping separate from the component so other surfaces can consume classes/tooltip"

key-files:
  created:
    - web/src/api/agents.ts
    - web/src/components/StatusDot.tsx
  modified:
    - web/src/components/board/TaskCard.tsx
    - web/src/components/sidebar/ProjectSidebar.tsx

key-decisions:
  - "Extracted CardRow as the single inner layout shared by CardShell, TaskCard, and TaskCardOverlay; entry passed down so hook calls stay in the card-rendering components"
  - "shrink-0 applied to both the tooltip wrapper span (the actual flex item) and the inner dot span, so card consumers never need layout classes beyond the optical offset"

patterns-established:
  - "Status palette family appears only in StatusDot.tsx (dots), the waiting card border, and the sidebar chip — per UI-SPEC three-surface rule"

requirements-completed: [STAT-01]

# Metrics
duration: 4min
completed: 2026-06-10
---

# Phase 4 Plan 03: Board-as-Dispatcher Status Surfaces Summary

**Live agent status on the kanban: 5s-polled useAgentStatuses hook, shared StatusDot with the exact UI-SPEC palette (pulsing amber waiting, gray-not-red stop exits), D-44 waiting card border, and D-49 sidebar waiting-count chips**

## Performance

- **Duration:** 4 min
- **Started:** 2026-06-10T18:36:37Z
- **Completed:** 2026-06-10T18:40:17Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- `useAgentStatuses` — one `["agent-statuses"]` query polled at 5s against `GET /api/agents/status`, transcribing the shared 04-02 contract (`AgentStatusEntry` with taskId/projectId/sessionId/status/exitCode/stopRequested)
- `StatusDot` + `dotMeta` — the single component owning the status palette: green working, pulsing amber waiting (`motion-reduce:animate-none` guard), zinc-400 idle, zinc-600 clean/stop exit, red-500 error exit; tooltip always shows the real exit code, `aria-label="Agent status: …"` on a non-interactive dot
- Task cards render the 8px dot in a shared flex row (CardRow) used by CardShell, TaskCard, and TaskCardOverlay; cards without an agent entry render pixel-identical to Phase 1 (no dot, no reserved gutter)
- Waiting cards swap `border-border` for `border-amber-400/40` in both TaskCard and CardShell — under reduced motion the border + chip remain the signal
- Sidebar project rows show a static amber count chip (`bg-amber-400/10`, tabular-nums, 18px min-width) when ≥1 agent in that project is waiting; chip absent at zero, never pulses, not independently clickable

## Task Commits

Each task was committed atomically:

1. **Task 1: useAgentStatuses hook + StatusDot component** - `0c84abb` (feat)
2. **Task 2: Card dot + waiting border, sidebar waiting chip** - `3ef1145` (feat)

## Files Created/Modified

- `web/src/api/agents.ts` - AgentStatusEntry type + useAgentStatuses 5s-polled query (created)
- `web/src/components/StatusDot.tsx` - dotMeta pure mapping + tooltip-wrapped StatusDot, exact UI-SPEC palette/copy (created)
- `web/src/components/board/TaskCard.tsx` - shared CardRow layout, per-card status lookup, D-44 waiting border (modified)
- `web/src/components/sidebar/ProjectSidebar.tsx` - waitingByProject count map + D-49 amber chip, name span truncates (modified)

## Decisions Made

- Extracted `CardRow` as the one inner layout shared by all three card renderers; CardShell and TaskCard each call `useAgentStatuses()` (deduped by React Query) so both can also drive the waiting border
- `shrink-0` placed on the StatusDot tooltip wrapper span (the real flex item) in addition to the inner dot span, so card call sites only pass the `mt-[6px]` optical offset

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## Known Stubs

`GET /api/agents/status` has no backend handler yet — it lands in parallel via plan 04-02 (Task 2), which transcribes the identical contract. This is the planned wave-1 parallel split, not an accidental stub: the frontend compiles standalone, and until 04-02 merges the query simply errors and `data` stays undefined (no dots/chips render — the correct empty state).

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- `StatusDot` + `dotMeta` exported and ready for 04-04's Agent tab dot (same state source, same component)
- `["agent-statuses"]` cache key established — 04-04/04-05 can invalidate or optimistically update it (D-45 waiting-clears-on-attach)
- Live behavior verifiable once 04-02's `/api/agents/status` endpoint lands (same wave)

## Self-Check: PASSED

All 5 key files exist on disk; both task commits (0c84abb, 3ef1145) present in git log.

---
*Phase: 04-claude-code-agent-sessions*
*Completed: 2026-06-10*
