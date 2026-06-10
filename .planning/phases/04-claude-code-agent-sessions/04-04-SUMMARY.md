---
phase: 04-claude-code-agent-sessions
plan: 04
subsystem: ui
tags: [react, tanstack-query, xterm, bracketed-paste, tabs, status-dots]

# Dependency graph
requires:
  - phase: 04-claude-code-agent-sessions (plan 02)
    provides: POST /api/sessions kind discriminator, 409 contracts, WS attach clears waiting (D-45 server side)
  - phase: 04-claude-code-agent-sessions (plan 03)
    provides: useAgentStatuses ["agent-statuses"] query, StatusDot component, AgentStatusEntry type
  - phase: 03-worktree-isolation
    provides: TaskPage tab strip, TaskTabs TabDef seam, worktree presence states
  - phase: 02-terminal-engine
    provides: TerminalPane attach-only contract, useSpawnSession cache-write pattern
provides:
  - useSpawnAgent mutation (kind:"agent" spawn with scoped cache write + agent-statuses invalidation)
  - TermSession kind/agentStatus/stopRequested fields (04-02 Info JSON transcription)
  - TerminalPane additive props — headerActions, exitedPrimaryLabel, showExitedClose, onReady, onConnect
  - AgentTab component — pre-start / running / exited lifecycle per UI-SPEC copy contract
  - Permanent first Agent tab in TaskPage with status dot (D-50), agent-excluded bash tabs, D-39 landing
  - Insert-description bracketed paste via TerminalPane onReady paste handle
  - D-45 optimistic waiting→idle cache write on agent WS connect
affects: [04-05 verification, 05 recovery/resume]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "TerminalPane extension is strictly additive optional props — bash panes render byte-identically by default"
    - "Imperative xterm access crosses component boundaries via onReady ref handle, never by lifting the Terminal"
    - "Agent tab fallbacks: 'agent' replaces 'description' everywhere a tab id can dangle (it can never disappear)"

key-files:
  created:
    - web/src/components/task/AgentTab.tsx
  modified:
    - web/src/api/sessions.ts
    - web/src/components/terminal/TerminalPane.tsx
    - web/src/components/task/TaskTabs.tsx
    - web/src/pages/TaskPage.tsx

key-decisions:
  - "showExitedClose also gates the not-found banner's Close (plan-specified) so a permanent Agent tab never offers Close on any banner"
  - "AgentTab comments avoid the words resume/--continue entirely to keep the Phase 5 scope grep clean"
  - "onConnect fires inside the existing conn-state effect (ref-stabilized) so the D-45 optimistic clear re-fires on every reconnect, matching server-side re-clear"

patterns-established:
  - "Header right group order: connection status → headerActions slot → Stop (D-36)"
  - "TabDef.leading renders inside a flex items-center gap-1 wrapper so dots never change trigger height"

requirements-completed: [TERM-01, STAT-01]

# Metrics
duration: 6min
completed: 2026-06-10
---

# Phase 4 Plan 04: Agent Tab Summary

**Permanent first Agent tab with Start/Start-again fresh-spawn lifecycle, Insert-description bracketed paste through a TerminalPane onReady handle, kind-aware session API, and the tab-label StatusDot with D-45 optimistic waiting clear**

## Performance

- **Duration:** 6 min
- **Started:** 2026-06-10T19:14:28Z
- **Completed:** 2026-06-10T19:21:05Z
- **Tasks:** 3
- **Files modified:** 5

## Accomplishments

- `useSpawnAgent` posts `{task_id, kind:"agent"}`, writes the fresh session into the scoped `["sessions", taskId]` cache (spawn-select race fix carried over) and additionally invalidates `["agent-statuses"]` so board/tab dots appear without waiting a poll period; `TermSession` transcribes the server's `kind`/`agentStatus`/`stopRequested` fields
- TerminalPane gained exactly five additive optional props — `headerActions` (slot before Stop), `exitedPrimaryLabel` (default "New terminal"), `showExitedClose` (default true; false hides Close on the exited AND not-found banners), `onReady` (imperative `{paste}` handle, ref-stabilized, nulled on cleanup), `onConnect` (fires in the conn-state effect on "connected") — bash panes render byte-identically by default; zincTheme, banner copy, Stop semantics, and the resize/jiggle flow untouched
- `AgentTab` implements the three lifecycle states with verbatim UI-SPEC copy in single template literals: pre-start ("No agent session" + branch-in-mono body + "Start agent"/"Starting…" CTA), no-worktree variant (disabled CTA in a tooltip-bearing span), running (TerminalPane with the "Insert description" ghost action — hidden when description is empty, pastes via bracketed paste, never submits), exited ("Start again" spawns FRESH, no Close, frozen output stays readable)
- TaskPage: Agent is the permanent first tab (`keepMounted`, no onClose), Description second, bash tabs third — with `s.kind !== "agent"` excluding the agent from closable bash tabs; landing tab is always "agent" (D-39) and both dangling-tab fallbacks (Pitfall-7 layout effect, removeTab) now fall back to "agent"; the tab label carries the shared `StatusDot` from the same `["agent-statuses"]` query as the board card (D-50)
- D-45 optimistic clear: `onConnect` rewrites waiting→idle for this task in the `["agent-statuses"]` cache; the server clears authoritatively on WS attach (04-02), other surfaces converge next poll
- Cleanup gate (D-42) confirmed zero-change: `CleanupWorktreeDialog` derives session counts from the server worktree-state endpoint, so the agent session counts and is stopped with no agent-specific wording

## Task Commits

Each task was committed atomically:

1. **Task 1: Kind-aware session API + additive TerminalPane props** - `65dd5a0` (feat)
2. **Task 2: AgentTab component (pre-start / running / exited)** - `60ccff7` (feat)
3. **Task 3: TaskPage integration — tab order, landing, tab dot, agent/bash separation** - `95439a7` (feat)

## Files Created/Modified

- `web/src/components/task/AgentTab.tsx` - Agent pane lifecycle component (created)
- `web/src/api/sessions.ts` - TermSession kind fields + useSpawnAgent mutation
- `web/src/components/terminal/TerminalPane.tsx` - five additive props, header slot, banner label/Close overrides, paste handle, connect hook
- `web/src/components/task/TaskTabs.tsx` - TabDef.leading rendered before the label in a gap-1 wrapper
- `web/src/pages/TaskPage.tsx` - agent tab wiring, D-39 landing, agent-excluded bash derivation, StatusDot on the trigger

## Decisions Made

- `showExitedClose` gates Close on both the exited and not-found banners (per plan) — a permanent Agent tab must never offer Close regardless of which banner shows
- AgentTab comments avoid the literal words "resume"/"--continue" so the plan's Phase 5 scope grep stays clean
- `onConnect` lives in the existing conn-state effect via a ref (same pattern as onSessionExit), so the optimistic clear re-fires on every reconnect — mirroring the server, which re-clears waiting on every attach

## Deviations from Plan

None - plan executed exactly as written.

## Authentication Gates

None encountered.

## Issues Encountered

None. Note for the verifier: `web/dist/index.html` was already dirty before this run (parallel-workstream build artifact, same note as the 04-01/04-02 summaries) and remains uncommitted per the Phase 1 placeholder-only rule.

## Known Stubs

None — all three states are wired to live endpoints (`POST /api/sessions` kind=agent, `["sessions", taskId]`, `["agent-statuses"]`). No placeholder values or TODO markers in any touched file.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- The full TERM-01 user flow is code-complete: Start agent → claude in the worktree pane → Stop/exit → Start again (fresh) → reattach via the unchanged Phase 2 replay+jiggle mechanism
- 04-05 (verification plan) can exercise the live flow end-to-end; no frontend blockers
- `useSpawnAgent`'s `["agent-statuses"]` invalidation plus the D-45 optimistic write give 04-05 deterministic dot behavior to verify against

## Self-Check: PASSED

- All 5 key files exist on disk
- All 3 task commits (65dd5a0, 60ccff7, 95439a7) present in git log
- `cd web && npm run build` clean; no `--resume`/`--continue` anywhere in web/src

---
*Phase: 04-claude-code-agent-sessions*
*Completed: 2026-06-10*
