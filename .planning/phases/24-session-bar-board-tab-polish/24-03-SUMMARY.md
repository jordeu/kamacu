---
phase: 24-session-bar-board-tab-polish
plan: 03
subsystem: ui
tags: [react, xterm, dropdown-menu, bracketed-paste, pr-review, terminal]

# Dependency graph
requires:
  - phase: 12-open-a-review
    provides: "PR-review seed (pr_review_seed setting) prefilled-once-per-session and the pasteApiRef bracketed-paste handle"
  - phase: 06-settings-task-header
    provides: "the task-actions ⋯ DropdownMenu pattern in TaskPage.tsx that the agent menu mirrors"
provides:
  - "TerminalPane.headerMenu prop — an agent-only ⋯ menu slot that supersedes the inline Stop button (bash panes keep their Stop)"
  - "AgentTab ⋯ menu folding Insert description / Insert review prompt / Stop into one dropdown"
  - "Manual-only PR-review-seed insertion — the connect-time auto-paste (seededSessionIds) is removed; the seed now enters only via the menu item"
affects: [phase-24-plan-04, session-bar-board-tab-polish]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Agent-only header menu supersedes the shared pane's Stop button via an optional ReactNode prop (headerMenu), keeping bash panes byte-for-byte unchanged"
    - "Bracketed-paste prefill re-homed from a connect-time side effect to an explicit user-gesture menu item (same mechanism, user-triggered trigger)"

key-files:
  created: []
  modified:
    - web/src/components/terminal/TerminalPane.tsx
    - web/src/components/task/AgentTab.tsx

key-decisions:
  - "The agent menu owns its own useStopSession Stop item (D-07 cleanest-fit); TerminalPane's handleStop/stopping/showStop stay in the pane for bash panes"
  - "headerMenu supersedes (renders instead of) showStop only when provided; bash panes pass nothing, so their inline Stop is untouched"
  - "Menu copy/order: Insert description → Insert review prompt → separator → Stop; align=end; Stop styled variant=destructive to match the old red Stop button"
  - "Insert review prompt visibility gate is Boolean(seed) && running — seed is already undefined for non-PR tasks and blank templates (derived upstream in TaskPage, D-08)"

patterns-established:
  - "Optional ReactNode header-slot prop that supersedes a built-in control for one caller only (agent) while leaving all other callers (bash) unchanged"
  - "Removing an auto-side-effect and re-homing its payload behind an explicit menu item — a net reduction in unintended-action surface (T-24-07)"

requirements-completed: [REVMENU-01, REVMENU-02]

# Metrics
duration: ~15min
completed: 2026-07-03
---

# Phase 24 Plan 03: Agent ⋯ Menu & No Auto-Insert Summary

**The agent view's standalone Stop button is replaced by a single ⋯ dropdown (Insert description / Insert review prompt / Stop), and the PR-review seed's connect-time auto-paste is deleted so the seed enters only when the user picks "Insert review prompt".**

## Performance

- **Duration:** ~15 min
- **Started:** 2026-07-03T17:15:00Z
- **Completed:** 2026-07-03T17:30:00Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- Added an agent-only `headerMenu?: ReactNode` prop to `TerminalPane` that renders **instead of** the inline Stop button when provided; bash panes (which pass no `headerMenu`) keep their inline Stop and all of `handleStop`/`stopping`/`showStop` untouched (D-07).
- Built the agent ⋯ menu in `AgentTab` (`Insert description` → `Insert review prompt` → separator → `Stop`, `align="end"`, ghost `icon-sm` trigger wrapping `<Ellipsis>`), mirroring the TaskPage task-actions menu; passed it as `headerMenu={agentMenu}` and removed the old `headerActions={insertAction}` standalone button.
- Deleted the connect-time PR-review-seed auto-paste: removed the module-scope `seededSessionIds` set and the seed block inside `handleConnect` (which now does ONLY the D-45 optimistic waiting→idle clear). The seed now fires solely from the `Insert review prompt` menu item's `onSelect` — same bracketed, never-auto-sent paste (D-09).

## Task Commits

Each task was committed atomically:

1. **Task 1: Add agent-only headerMenu prop to TerminalPane (D-07)** — `cf9619e` (feat)
2. **Task 2: Build the agent ⋯ menu + delete connect-time auto-paste (D-06/D-08/D-09)** — `f793932` (feat)

## Files Created/Modified
- `web/src/components/terminal/TerminalPane.tsx` — new `headerMenu?: ReactNode` prop; the header right group renders `headerMenu` in place of the `showStop` Stop button when it is provided, and keeps the inline Stop button when `headerMenu` is undefined (every bash pane). `handleStop`/`stopping`/`showStop` retained (bash panes unaffected).
- `web/src/components/task/AgentTab.tsx` — imports the dropdown-menu barrel, `Ellipsis`, and `useStopSession`; builds a running-only `agentMenu` ⋯ dropdown (Insert description / Insert review prompt / Stop) and passes it as `headerMenu`; removed the `seededSessionIds` module-scope set + doc-comment and the connect-time seed paste in `handleConnect`; removed the standalone `insertAction` Tooltip button.

## Decisions Made
- **Agent menu owns its own Stop (D-07):** the `Stop` menu item calls `useStopSession().mutate(agentSession.id)` because `AgentTab` already holds `agentSession.id` — the cleanest fit noted in the plan. The pane's own `handleStop`/`stopping`/`showStop` stay for bash panes.
- **`headerMenu` supersedes, not merges:** when `headerMenu` is truthy the pane renders it in place of the Stop button; `headerActions` (rendered before this slot) is left intact for any future bash use. Bash panes pass no `headerMenu`, so behavior is byte-for-byte unchanged.
- **Menu copy/order & style (Claude's Discretion, per plan):** `Insert description` → `Insert review prompt` → `<DropdownMenuSeparator />` (only when at least one insert item renders) → `Stop`; `align="end"`; `aria-label="Agent actions"` on the trigger; the `Stop` item uses `variant="destructive"` to preserve the old red-Stop affordance's visual weight. Dropdown items carry no Tooltip wrapper (the old header button's tooltip is dropped — dropdown items are self-describing).
- **Insert review prompt gate:** `Boolean(seed) && running`. `seed` is already undefined for non-PR tasks and blank/whitespace templates (D-08 gate lives upstream in `TaskPage`), so no additional PR check is needed here.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- The worktree had no `web/node_modules` (deps not installed in the fresh worktree), so `npm run build`/`npm run lint` initially reported `tsc: not found` / `eslint: not found`. Resolved by running `npm ci` (633 packages, 0 vulnerabilities) before verifying — not a code change, purely environment setup.
- `npm run lint` reports 21 pre-existing `react-hooks` errors (the ~18–20 tech-debt errors documented in STATE.md "Known tech debt", in `use-mobile.ts`, `TaskPage.tsx`, and unchanged `TerminalPane.tsx` lines 64/67/69/90/192/195 — refs-during-render / setState-in-effect). These are OUT OF SCOPE (not caused by this plan's changes); the count stayed at exactly 21 before and after my edits, and the two modified files have zero lint errors on the lines I changed. The gating build (`tsc -b && vite build`) is green.

## Threat Surface
Per the plan's threat model, this plan is a net REDUCTION in surface: removing the connect-time auto-paste (D-09) eliminates the only path that wrote to the agent prompt without an explicit user gesture (T-24-07 mitigated). The remaining insertion is strictly user-triggered via the menu, using the unchanged, vetted xterm bracketed-paste mechanism that never auto-submits (T-24-08 accepted). No new packages (T-24-SC — dropdown-menu, lucide, sessions API all already in-repo). No new security-relevant surface introduced.

## Next Phase Readiness
- REVMENU-01 (⋯ menu replaces Stop) and REVMENU-02 (no auto-inserted PR review prompt) are implemented and build-verified.
- Plan 04 (tab rename frontend) is independent of these files.
- A human-verify gate for the runtime behavior (open a PR review → agent no longer auto-pastes on connect; `Insert review prompt` pastes it un-sent; non-PR task never shows the item; bash tabs keep their inline Stop) is appropriate at phase close (no frontend test framework in this repo).

## Self-Check: PASSED
- FOUND: web/src/components/terminal/TerminalPane.tsx
- FOUND: web/src/components/task/AgentTab.tsx
- FOUND commit: cf9619e (Task 1)
- FOUND commit: f793932 (Task 2)
- Source assertions: `headerMenu?: ReactNode` present in TerminalPane; `headerMenu={agentMenu}` + `DropdownMenu` present in AgentTab; `grep -c seededSessionIds` = 0.
- Verification: `npm run build` exits 0; `grep -c seededSessionIds web/src/components/task/AgentTab.tsx` = 0; lint at pre-existing 21-error baseline with zero errors in the modified files.

---
*Phase: 24-session-bar-board-tab-polish*
*Completed: 2026-07-03*
