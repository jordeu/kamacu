---
phase: 03-worktree-isolation-bash-tabs
plan: 04
subsystem: ui
tags: [react, tanstack-query, radix-tabs, xterm, worktree, sessions]

# Dependency graph
requires:
  - phase: 03-worktree-isolation-bash-tabs (plan 03-03)
    provides: Task JSON worktree fields, POST/GET/DELETE /api/tasks/{id}/worktree, POST /api/sessions {task_id}, GET /api/sessions?task_id=
  - phase: 02-terminal-engine
    provides: TerminalPane (attach-only, D-12), useStopSession/useDeleteSession, replay-on-reattach
provides:
  - web/src/api/worktrees.ts — WorktreeState, useWorktreeState (gcTime 0 Pitfall 8 guard), useCreateWorktree, useCleanupWorktree
  - Task-scoped useSessions(taskId)/useSpawnSession(taskId) with spawn-race setQueryData prepend; no-arg dev usage unchanged
  - WorktreeMetaLine with active/failed/absent states (D-25/D-26) and verbatim UI-SPEC copy
  - Controlled TaskTabs (value/onValueChange, closable triggers, trailing slot, keepMounted terminal content)
  - TaskPage full bash-tab lifecycle — spawn/mirror/close (D-27..D-30), full-width layout with 860px prose islands
affects: [03-05 cleanup dialog (useWorktreeState/useCleanupWorktree ready), 03-06 e2e integration, phase-04 agent tab]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "TabDef gains onClose/muted/keepMounted; terminal TabsContent renders forceMount + data-[state=inactive]:hidden so tab switches never detach the WS"
    - "Tab-removal neighbor activation: prevTabIdsRef snapshot + useLayoutEffect fixup (before paint) for poll-driven removals; synchronous setActiveTab for user-initiated removeTab (Pitfall 7)"
    - "keepExitedIds (ids seen running this mount) + closingIds sets implement the D-28/D-29 exited-tab-ghost rules from RESEARCH Open Question 2"

key-files:
  created:
    - web/src/api/worktrees.ts
    - web/src/components/task/WorktreeMetaLine.tsx
  modified:
    - web/src/api/types.ts
    - web/src/api/client.ts
    - web/src/api/sessions.ts
    - web/src/components/task/TaskTabs.tsx
    - web/src/pages/TaskPage.tsx

key-decisions:
  - "The tab x renders only while the session is running and not yet closing — closing/exited tabs drop the affordance (exited tabs close via the Phase 2 banner), interpreting UI-SPEC's 'x disabled' as removal of the dead control"
  - "TaskPage switched to a flex h-full column (like TerminalPage) instead of viewport calc(): bash content fills the area below the strip robustly; Description scrolls within its 860px island"
  - "TabDef gained keepMounted (beyond the planned onClose/muted) — onClose presence cannot discriminate mounting since closing tabs lose onClose but must keep their WS"

patterns-established:
  - "Disabled-button tooltips: wrap the Button in an inline-flex span so Radix Tooltip fires on disabled targets (D-30)"
  - "Scoped query keys: ['sessions', taskId] prefix-invalidated by ['sessions'] — one invalidation covers dev and task-scoped lists"

requirements-completed: [GIT-01, TERM-04]

# Metrics
duration: 15min
completed: 2026-06-10
---

# Phase 3 Plan 04: Worktree Meta Line & Bash Tabs UI Summary

**Task view is now the working surface: typed worktree/session API hooks, the branch/failed/absent meta line with Retry/Create, and live bash tabs (spawn via +, mirror server sessions, x-to-stop with left-neighbor activation) mounted through a controlled TaskTabs — verified against the real server end-to-end at the API level**

## Performance

- **Duration:** 15 min
- **Started:** 2026-06-10T15:01:00Z
- **Completed:** 2026-06-10T15:16:54Z
- **Tasks:** 3
- **Files modified:** 7

## Accomplishments

- API layer covers every Phase 3 endpoint: `useWorktreeState` (gcTime 0 + enabled-on-open — the Pitfall 8 stale-dialog guard), `useCreateWorktree` (response-is-truth `setQueryData`, since a 200 may carry a fresh `worktree_error`), `useCleanupWorktree` (DELETE with `{stop_sessions, force}` body via the extended `del()`), and task-scoped `useSessions`/`useSpawnSession` whose no-arg forms keep TerminalPage compiling byte-identical
- WorktreeMetaLine renders the three D-25/D-26 states with verbatim UI-SPEC copy: branch line (GitBranch icon + 12px mono muted, absolute-path tooltip), `Couldn't create a worktree: {error}` + outline `Retry creation`, and neutral `No worktree yet.` + `Create worktree`; both buttons flip to disabled `Creating…` in flight
- TaskTabs is controlled and extensible: closable triggers (span[role=button] x — never red, `Stop {label} and close tab` aria, `Stop and close` tooltip, Enter/Space support, stopPropagation), a trailing slot for the `+` button, horizontal scroll instead of wrap, and `forceMount` + `data-[state=inactive]:hidden` terminal content so switching tabs never detaches a live WebSocket
- TaskPage implements the full D-27..D-30 lifecycle with the server as tab truth: tabs mirror `GET /api/sessions?task_id=` (5s poll + 'x'-frame invalidation), `+` spawns and activates immediately (spawn-race `setQueryData`), x stops then removes on exit with left-neighbor activation computed before paint, self-exited tabs stay muted until banner-Close within the visit, and the disabled `+` explains `Bash sessions need a worktree`
- Live server smoke proved the consumed contract end-to-end: create task → `branch`/`worktree_path` populated → `POST /api/sessions {task_id}` returns `Bash 1` with `taskId` → filtered list → `GET /worktree` `{branch, path, dirty_files, running_sessions}` → `DELETE {stop_sessions:true}` 204 → task back to the absent state

## Task Commits

Each task was committed atomically:

1. **Task 1: API layer — worktree fields, worktrees.ts hooks, task-scoped session hooks** - `d60ce05` (feat)
2. **Task 2: WorktreeMetaLine + TaskPage full-width layout** - `4ec6eb1` (feat)
3. **Task 3: Controlled TaskTabs + bash session lifecycle in TaskPage** - `93b8018` (feat)

## Files Created/Modified

- `web/src/api/worktrees.ts` — WorktreeState type + the three worktree hooks (03-05 consumes useWorktreeState/useCleanupWorktree as-is)
- `web/src/api/sessions.ts` — TermSession gains `taskId?`; taskId-scoped query keys and spawn body
- `web/src/api/types.ts` — Task gains `branch`/`worktree_path`/`worktree_error`
- `web/src/api/client.ts` — `del()` passes an optional JSON body
- `web/src/components/task/WorktreeMetaLine.tsx` — three-state presence row per UI-SPEC
- `web/src/components/task/TaskTabs.tsx` — controlled Tabs, closable triggers, trailing slot, keepMounted content
- `web/src/pages/TaskPage.tsx` — full-width flex layout, meta line slot, bash tab lifecycle wiring

## Decisions Made

- **x affordance scope:** plan-literal `onClose: running && !closing` means closing and exited tabs show no x (instead of a disabled x); exited tabs close via the Phase 2 banner per D-29's "also" clause. Noting as the chosen interpretation of UI-SPEC's "x disabled" closing-state row
- **Height strategy:** instead of a brittle `calc(100vh - Npx)`, TaskPage became a `flex h-full` column (the proven TerminalPage pattern in this same `<main>`): bash content gets `h-full min-h-[320px]`, Description scrolls inside its 860px island with header/strip pinned
- **TabDef.keepMounted added** beyond the planned shape — required for correctness: a closing tab loses `onClose` but its TerminalPane must stay mounted, so mount behavior needs its own flag
- Removed TaskPage's redundant local `TooltipProvider` — AppLayout already provides one app-wide (Phase 1 decision)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] TabDef gained a `keepMounted` flag for terminal content**
- **Found during:** Task 3 (TaskTabs controlled rewrite)
- **Issue:** The plan's TabDef shape (`onClose?`, `muted?`) cannot discriminate which TabsContent needs `forceMount`: closing tabs lose `onClose` but unmounting them would detach the live WebSocket mid-close
- **Fix:** Explicit `keepMounted?: boolean` on TabDef; TaskPage sets it for every bash tab
- **Files modified:** web/src/components/task/TaskTabs.tsx, web/src/pages/TaskPage.tsx
- **Verification:** tsc + production build green; closing-state tabs keep their pane mounted by construction
- **Committed in:** 93b8018 (Task 3 commit)

---

**Total deviations:** 1 auto-fixed (1 missing critical)
**Impact on plan:** Minimal API extension required for WS-survival correctness. No scope creep.

## Issues Encountered

- `web/dist/index.html` still carries the pre-existing uncommitted local build output (logged in 03-03's deferred-items); this plan's `npm run build` regenerated it — left uncommitted per the Phase 01 "real build output never committed" decision

## Flagged for live verification (03-06)

- `forceMount` + hidden-tab fit guard: RESEARCH flags the Radix forceMount vs TerminalPane fit-guard interplay for browser verification; the fallback (default unmount + replay-on-reattach) was NOT needed at build time but the interaction needs eyes
- Tab-strip `overflow-x-auto` may clip ~1px of the active-tab blue indicator (the indicator extends 1px past the list's padding box); check when many tabs overflow

## Known Stubs

None — every surface is wired to the live API; the smoke run exercised create→spawn→list→state→cleanup over HTTP.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 03-05 (cleanup dialog) consumes `useWorktreeState(taskId, open)` and `useCleanupWorktree` exactly as shipped; the two 409 error strings from 03-03 discriminate the dialog variants
- 03-06 should verify in the browser: meta-line states, `+` spawn cwd (= worktree `pwd`), x close with neighbor activation, disabled-`+` tooltip, forceMount fit behavior, tab-strip overflow

---
*Phase: 03-worktree-isolation-bash-tabs*
*Completed: 2026-06-10*

## Self-Check: PASSED

All created/modified files and the SUMMARY exist on disk; all 3 task commits (d60ce05, 4ec6eb1, 93b8018) present in git log; `npx tsc -b --noEmit` and `npm run build` green; live-server smoke validated the consumed API contract end-to-end.
