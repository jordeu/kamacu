---
phase: 17-global-active-sessions-bar
plan: 02
subsystem: frontend
tags: [react, typescript, tailwind, app-shell, agent-status, localstorage, overlay, accessibility]

# Dependency graph
requires:
  - phase: 17-global-active-sessions-bar
    provides: "Plan 01 widened /api/agents/status (and the TS AgentStatusEntry type) with taskTitle + projectName — the row labels this bar renders"
provides:
  - "ActiveSessionsBar: a persistent, collapsible bottom overlay showing every LIVE agent session app-wide (collapsed per-state counts with waiting emphasis; expanded attention-first flat list with cross-project click-through)"
  - "AppLayout mounts ActiveSessionsBar once outside <Outlet/> so it appears on every route (board, task, settings)"
affects: [global-active-sessions-bar, AppLayout, app-shell]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Fixed-to-viewport-bottom overlay (fixed inset-x-0 bottom-0 z-30) so the bar floats over content and never reflows the xterm terminals (D-03) — panel renders ABOVE the bar in a flex column so it floats UP"
    - "Reuse useAgentStatuses() as a fourth consumer of the single 5s poll (D-12) — no new hook, no second poll"
    - "All status colors sourced from dotMeta() (never hardcoded hex) for collapsed counts AND per-row dots"
    - "GLOBAL localStorage collapse key (kangent:sessions-bar-collapsed) mirroring the ReviewColumn !== \"0\" default-collapsed idiom"

key-files:
  created:
    - web/src/components/layout/ActiveSessionsBar.tsx
  modified:
    - web/src/components/layout/AppLayout.tsx
    - web/dist (Vite rebuild embedded for the single binary)

key-decisions:
  - "Overlay, not reserved space (D-03): the bar is fixed to the viewport bottom; <main> layout is untouched and bottom padding is NOT added, so the xterm terminal never resizes/reflows when the bar expands or collapses"
  - "Stable sort by attention rank only (waiting=0, working=1, idle=2): equal-rank rows keep feed order, avoiding 5s-poll jitter (SBAR-05 / D-05)"
  - "Live-only filter (D-09): only working/waiting/idle render; exited (incl. DB-derived) entries never appear in the bar"
  - "Current-task highlight is non-chromatic (bg-sidebar-accent, D-07) — never a status color — so it doesn't read as a state"
  - "Waiting emphasis is conditional: amber chip + pulse only when waiting > 0; at zero the waiting group uses the same muted treatment as the others (SBAR-03)"

patterns-established:
  - "App-shell overlays mount once inside SidebarProvider but outside <Outlet/> and self-position with fixed inset-x-0 bottom-0; a single mount is global across all routes"

requirements-completed: [SBAR-01, SBAR-02, SBAR-03, SBAR-04, SBAR-05, SBAR-06, SBAR-07, SBAR-08, SBAR-09]

# Metrics
duration: ~9min active (Tasks 1-2; Task 3 human-verify gate spanned the pause)
completed: 2026-06-18
---

# Phase 17 Plan 02: Global Active Sessions Bar Summary

**A persistent, collapsible bottom bar (`ActiveSessionsBar.tsx`) mounted globally in `AppLayout` shows every LIVE Claude agent session across all projects — collapsed it renders per-state counts (working/waiting/idle) + total with the waiting count amber-pulsing only when > 0; expanded it floats a panel UP over content (a fixed overlay that never reflows the xterm terminals) listing live sessions attention-first (waiting → working → idle), each row click-through to that task's agent view including cross-project, with collapse state persisted in localStorage and ~5s freshness off the existing poll. Covers SBAR-01..SBAR-09.**

## Performance

- **Duration:** ~9 min active implementation (Tasks 1-2); Task 3 was a blocking human-verify gate.
- **Completed:** 2026-06-18
- **Tasks:** 3 (2 auto + 1 human-verify checkpoint)
- **Files:** 1 created, 1 modified (+ `web/dist` rebuilt for the embedded SPA)

## Accomplishments

- Built `web/src/components/layout/ActiveSessionsBar.tsx` (244 lines): a single fixed-to-viewport-bottom overlay (`fixed inset-x-0 bottom-0 z-30`) returning a flex column with the expanded panel ABOVE the collapsed bar, so the panel floats UP over content.
- **Collapsed bar (SBAR-01/02/03/09):** `h-9` strip, `bg-[#101013]`, with three count groups (working/waiting/idle) using `dotMeta()`-sourced dot colors + a total. The waiting group uses the ProjectSidebar amber chip (`text-amber-400`) with a pulsing dot only when `waiting > 0`; at zero it falls back to the muted treatment. Quiet loading (`Skeleton`) and a muted "No active sessions" zero state keep the bar always present and non-blocking.
- **Expanded panel (SBAR-04/05):** a `max-h-80 overflow-y-auto` capped/scrolling overlay listing live sessions sorted attention-first (stable rank sort: waiting → working → idle). Each row = state dot (`dotMeta(entry)` — gives waiting its pulse) · project name · truncated task/PR title, with a `#<n>` PR badge for `source === "github_pr"` rows and a non-chromatic `bg-sidebar-accent` highlight on the currently-open task's row.
- **Navigation (SBAR-06):** rows are keyboard-focusable (`role="button"`, `tabIndex={0}`, Enter/Space) and `useNavigate` to `/projects/${projectId}/tasks/${taskId}` — works cross-project.
- **Persistence + freshness (SBAR-07/08):** collapse state mirrors the ReviewColumn idiom with a GLOBAL `kangent:sessions-bar-collapsed` key (absent ⇒ collapsed default); data comes solely from `useAgentStatuses()` (the existing 5s poll, D-12) filtered to live states (D-09) — no new hook, no second poll.
- Mounted `<ActiveSessionsBar />` once in `AppLayout.tsx` inside `SidebarProvider` but AFTER `</main>` (outside `<Outlet/>`), so it appears on every route without altering `<main>`'s `flex-1 overflow-hidden` layout. Rebuilt `web/dist`.

## Task Commits

Each task was committed atomically:

1. **Task 1: Build ActiveSessionsBar.tsx** — `766f5ae` (feat)
2. **Task 2: Mount ActiveSessionsBar in AppLayout** — `5ab45ef` (feat; also rebuilt `web/dist`)
3. **Task 3: Human verification (human-verify checkpoint)** — no code commit; the pause was recorded in `af136e2` (docs). User responded **"approved"** — all 7 verification items passed.

## Files Created/Modified

- `web/src/components/layout/ActiveSessionsBar.tsx` (created) — the Global Active Sessions Bar: collapsed counts with conditional waiting emphasis, expanded attention-first overlay list, cross-project navigation, localStorage default-collapsed persistence, live-only filter, quiet loading/empty states; all status colors from `dotMeta`, all tokens per 17-UI-SPEC.
- `web/src/components/layout/AppLayout.tsx` (modified) — imports and renders `<ActiveSessionsBar />` once, outside `<Outlet/>`, last child of `SidebarProvider`; `<main>` className unchanged.
- `web/dist` — rebuilt by `vite build` so the embedded single-binary SPA carries the new bar.

## Decisions Made

- **Overlay, never reserved space (D-03):** kept the bar `fixed` and added NO bottom padding to `<main>`, so the xterm terminal's height is unaffected — expand/collapse does not resize or reflow the terminal (verified at the human-verify gate, item 6).
- **Stable attention-only sort (SBAR-05 / D-05):** sorted by `{ waiting: 0, working: 1, idle: 2 }` rank only; equal-rank rows keep feed order so the list doesn't jitter on each 5s refetch.
- **Non-chromatic current-row highlight (D-07):** used `bg-sidebar-accent` rather than any status color so "you are here" never reads as a session state.
- **Conditional waiting emphasis (SBAR-03):** amber chip + pulse only when `waiting > 0`; otherwise muted like the other counts.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None. Note (carried, not an issue): with `--dangerously-skip-permissions` on by default the amber waiting state rarely fires (standing decision D-51 / STATE.md blocker); the user confirmed the amber-emphasis path during the human-verify gate.

## Verification Performed

- **Automated:** `cd web && npx tsc -b` exits 0; `npx vite build` exits 0 (embedded SPA `web/dist` rebuilt). `grep -c 'dotMeta'` == 4 (≥ 2 required — no hardcoded status colors). Bar is `fixed inset-x-0 bottom-0` and mounted after `</main>` (outside `<Outlet/>`).
- **Human-verify (Task 3, approved):** all 7 items passed against a live instance — (1) bar present incl. zero state on board/task/settings; (2/3) collapsed colored counts + total with waiting amber-pulse; (4/5/7) expand floats UP without pushing content, attention-first list, PR `#n` badges, collapse persistence with collapsed default; (6, critical) the xterm terminal does NOT resize/reflow on repeated expand/collapse; (8) ~5s appear/drop freshness with exited sessions filtered out.

## Requirements Covered

SBAR-01, SBAR-02, SBAR-03, SBAR-04, SBAR-05, SBAR-06, SBAR-07, SBAR-08, SBAR-09 — all completed this plan. (SBAR-10, the backend feed widening, was completed in Plan 01.)

## Known Stubs

None — the bar is fully wired to the live `useAgentStatuses()` feed; no placeholder data, no hardcoded empty values flowing to the UI.

## Next Phase Readiness

- All nine frontend SBAR requirements (SBAR-01..SBAR-09) plus Plan 01's SBAR-10 are implemented and verified; Phase 17 is functionally complete pending the orchestrator's phase-level verification.
- The bar is a clean fourth consumer of the single agent-status poll, leaving room for the deferred follow-ups (SBAR-FUT-03/04 OS/tab notifications, SBAR-FUT-05 bash/tmux sessions) without re-architecting the feed.

## Self-Check: PASSED

All claimed files exist and all task commits are present in git history (verified below).

---
*Phase: 17-global-active-sessions-bar*
*Completed: 2026-06-18*
