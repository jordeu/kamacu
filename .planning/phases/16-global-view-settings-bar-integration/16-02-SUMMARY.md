---
phase: 16-global-view-settings-bar-integration
plan: 02
subsystem: ui
tags: [react, tanstack-query, typescript, global-scratchpad, tasktabs]

# Dependency graph
requires:
  - phase: 15-global-sessions-backend
    provides: scope:"global" spawn/list/reattach wire, source:"global" status entries
  - phase: 16-global-view-settings-bar-integration/01
    provides: the ["global"] query, six ["sessions","global"] hooks, widened source union, bar /global navigation
provides:
  - web/src/pages/GlobalTaskPage.tsx — the /global Scratchpad shell (7-state view matrix, Agent + bash tabs only)
  - AgentTabScope discriminator + the loosened single-file AgentTab serving both task and global scopes
  - The /global route in App.tsx — the 16-01 bar click-through destination (GINT-01 destination half)
affects: [16-03-settings-section, phase-17-uat]

# Tech tracking
tech-stack:
  added: []  # zero new packages
  patterns:
    - "Scope-discriminated component: one AgentTab file branching off scope.kind (both hook sets declared, unused set never fires) instead of a fork"
    - "Adjust-state-during-render accumulator (AddProjectDialog prevOpen-tracker idiom) for the keepExited poll — new files carry zero set-state-in-effect debt"

key-files:
  created:
    - web/src/pages/GlobalTaskPage.tsx
  modified:
    - web/src/components/task/AgentTab.tsx
    - web/src/pages/TaskPage.tsx
    - web/src/App.tsx
    - web/dist/index.html

key-decisions:
  - "Task-branch worktree metadata (gating + branch copy) reads the shared [\"task\", id] cache via useTask — NaN-disabled on the global branch — keeping the locked AgentTabScope shape and the scope/description/seed prop surface; the global branch symmetrically reads useGlobal() (research Pattern 4)"
  - "keepExited accumulation converted from TaskPage's effect idiom to the adjust-state-during-render tracker (research Pitfall 8) so GlobalTaskPage lints clean in isolation — behaviorally identical converging accumulator"
  - "GVIEW-01/GVIEW-04/GINT-01 left unchecked in REQUIREMENTS.md: 16-02 lands the implementation halves; the requirements close with end-of-phase/Phase 17 UAT (16-01 decision carried forward)"

patterns-established:
  - "Copy-then-trim page reuse: duplicate the bash-tab lifecycle block wholesale, delete task-only branches, swap scope-aware hooks — never extract a shared shell (research Anti-Patterns)"

# GVIEW-01/GVIEW-04/GINT-01 intentionally NOT listed — implementation halves
# ship here per the plan's success criteria; requirements close with
# end-of-phase/Phase 17 UAT (16-01 key-decision carried forward).
requirements-completed: []

# Coverage metadata (#1602)
coverage:
  - id: D1
    description: "Loosened AgentTab: exported AgentTabScope ({kind:\"task\";taskId}|{kind:\"global\"}), scope-aware hooks/resumable/cache writes, global pre-start (D-41 root-naming, unconditional CTA), global ⋯ menu = Stop only (D-36), dead projectId prop deleted; task path provably unchanged (TaskPage call-site-only diff)"
    requirement: GVIEW-01
    verification:
      - kind: other
        ref: "npm --prefix web run build + npx eslint AgentTab.tsx clean + grep gates (AgentTabScope exported, kind: \"global\" present, projectId count 0) + git diff TaskPage.tsx = 2-line call-site change"
        status: pass
    human_judgment: false
  - id: D2
    description: "GlobalTaskPage 7-state view matrix off the shared [\"global\"] query: pending skeleton, load-error retry, D-39 unconfigured hero, D-40 vanished-root hero (root_path verbatim in mono span), shell with exactly one of {D-42 warn, D-38 isolation, none}"
    requirement: GVIEW-04
    verification:
      - kind: other
        ref: "grep gates: all locked copy strings present ×1 (D-39/D-40/D-38/D-42/state-2), Description=0, dangerouslySetInnerHTML=0; build + eslint clean"
        status: pass
    human_judgment: true
    rationale: "State transitions require a running app driven through real config changes (unconfigured → folder root → vanished-while-live → vanished-idle → managed clone) — the plan's verification section defers this to end-of-phase manual UAT"
  - id: D3
    description: "The trimmed task-parity shell: static Scratchpad h1 (D-35), QuotaIndicator iff agent.engine === \"claude\" from the [\"global\"] query (D-37), TaskTabs strip of exactly [Agent + bash tabs], wholesale bash-tab lifecycle (visibleSessions memo, Pitfall-7 neighbor reactivation, tmux reattach reattachedRef guard), always-enabled trailing + with 409-verbatim error span"
    requirement: GVIEW-01
    verification:
      - kind: other
        ref: "grep gates: source === \"global\" agentEntry lookup, engine === \"claude\" gating, reattachedRef guard present, keydown=0, BoardWorkspaceSync=0; build + eslint clean"
        status: pass
    human_judgment: true
    rationale: "Tab lifecycle, streaming, and Start/Stop/Resume parity are observable only with live sessions and a configured root — end-of-phase UAT owns the behavior sign-off"
  - id: D4
    description: "/global route registered in App.tsx: plain default import, sibling of /settings inside AppLayout, NOT wrapped in BoardWorkspaceSync, no URL params — the 16-01 bar click-through destination (GINT-01 destination half)"
    requirement: GINT-01
    verification:
      - kind: other
        ref: "grep 'path=\"/global\"' App.tsx:141 + plain import line; BoardWorkspaceSync count 0 in GlobalTaskPage.tsx; build green"
        status: pass
    human_judgment: false

# Metrics
duration: 38 min
completed: 2026-08-27
status: complete
---

# Phase 16 Plan 02: Global view — GlobalTaskPage + /global route Summary

**The /global Scratchpad view: a copy-then-trim GlobalTaskPage rendering the 7-state matrix off the shared ["global"] query (D-39/D-40 heroes, D-38/D-42 banner slot), AgentTab loosened behind an AgentTabScope discriminator (task path byte-for-byte unchanged), and the /global route registered as the bar's click-through destination**

## Performance

- **Duration:** 38 min
- **Started:** 2026-08-27T10:36:19Z
- **Completed:** 2026-08-27T11:17:27Z
- **Tasks:** 2
- **Files modified:** 5 (4 src + tracked dist artifact refresh)

## Accomplishments
- AgentTab serves both scopes from ONE file: task branch keeps worktree gating + branch copy byte-for-byte (via the shared useTask cache read), global branch gets the D-41 root-naming pre-start with unconditional CTA and the D-36 Stop-only ⋯ menu — the dead `projectId` prop is gone
- GlobalTaskPage ships the full 7-state view matrix: honest degraded heroes (unconfigured vs vanished-root, path named verbatim in mono spans), warn-don't-disturb state 5, persistent D-38 isolation banner, managed-clone state 7 bare
- The whole TaskPage bash-tab lifecycle copied wholesale with scope-aware hooks: visibleSessions filter+sort, Pitfall-7 neighbor-reactivation layout effect, tmux reattach with the reattachedRef one-shot guard, removeTab/handleCloseTab/handleSpawn firing from click handlers only
- QuotaIndicator gated on `engine === "claude"` off the ["global"] query — the D-37 locked outcome, NOT TaskPage's pattern-based predicate (stale-union pitfall avoided)
- App.tsx registers /global as a plain sibling of /settings inside AppLayout — the 16-01 bar rows now navigate to a real page

## Task Commits

Each task was committed atomically:

1. **Task 1: Loosen AgentTab behind a task/global scope discriminator** - `571e96a` (feat)
2. **Task 2: Create GlobalTaskPage (7-state matrix + shell) and register the /global route** - `e221c4a` (feat)

**Artifact refresh:** `753d1c3` (chore: rebuild embedded SPA dist — repo convention)

## Files Created/Modified
- `web/src/pages/GlobalTaskPage.tsx` - NEW: the trimmed TaskPage shell + 7-state view matrix + degraded heroes + banner slot
- `web/src/components/task/AgentTab.tsx` - AgentTabScope discriminator, scope-aware hooks/lookups/cache writes, D-36/D-41 global branches
- `web/src/pages/TaskPage.tsx` - the AgentTab call site ONLY (scope + description props; 2-line diff)
- `web/src/App.tsx` - /global route registration (plain import, AppLayout sibling of /settings)
- `web/dist/index.html` - rebuilt hash (tracked artifact)

## Decisions Made
- Task-branch worktree metadata (gating + branch copy) comes from the shared `["task", id]` cache via `useTask` — NaN-disabled on the global branch (useTask's `!isNaN` guard) — instead of new props: keeps the locked `AgentTabScope` shape and the documented scope/description/seed surface; the global branch symmetrically reads `useGlobal()` (research Pattern 4's self-sufficient component)
- Both hook sets (task + global spawn/resume) are declared unconditionally and selected off `scope.kind` — the unused set never fires (mutations only hit the wire on `.mutate()`), satisfying rules-of-hooks with zero behavioral cost
- The render-phase keepExited accumulator (see Deviations) uses a `missingRunning` guard so it converges — once every running id is in the set the branch stops firing
- GVIEW-01/GVIEW-04/GINT-01 left unchecked in REQUIREMENTS.md — implementation halves complete here; closure rides end-of-phase/Phase 17 UAT (16-01 decision carried forward)
- Dist artifact refreshed as a separate `chore:` commit per repo precedent

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] keepExited accumulator converted to adjust-state-during-render**
- **Found during:** Task 2 (GlobalTaskPage creation)
- **Issue:** The plan prescribes copying TaskPage's bash-tab lifecycle "wholesale", including the keepExited poll effect (`TaskPage.tsx:134-146`) — but that exact idiom trips `react-hooks/set-state-in-effect`, and the plan's own verification requires the NEW file to lint clean in isolation (the 29 pre-existing repo errors are TaskPage's known debt)
- **Fix:** Converted the accumulation to React's adjust-state-during-render pattern (the `AddProjectDialog.tsx:73-95` prevOpen-tracker idiom research Pitfall 8 prescribes for exactly this): a guarded render-phase `setKeepExitedIds` union that converges once every running id is retained — same accumulator semantics, zero effect
- **Files modified:** web/src/pages/GlobalTaskPage.tsx
- **Verification:** `npx eslint src/pages/GlobalTaskPage.tsx` clean; build green; the tmux reattach effect (a genuine external-system sync) keeps its effect form verbatim
- **Committed in:** e221c4a (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Necessary to satisfy the plan's own lint-clean gate on new files; behaviorally identical accumulator. No scope creep.

## Issues Encountered

None

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- The /global route is live: the 16-01 bar rows, highlight, and click-through now resolve to a real page (GINT-01 destination half closed)
- AgentTab is scope-loosened — 16-03 (Settings Scratchpad section) composes the same ["global"] query + PUT hook with zero further AgentTab/GlobalTaskPage edits needed
- End-of-phase manual UAT (per the plan's verification section) drives the state matrix: unconfigured hero → folder root + D-38 banner → Start agent → rename root on disk → state 5 warn banner with terminals streaming → stop all + vanish → state 4 hero → repo root (state 7, no banner) → opencode default agent (no quota chip)
- Zero new packages; TaskPage untouched beyond the 2-line call site; TaskTabs/TerminalPane/QuotaIndicator untouched

## Self-Check: PASSED

All 4 key src files exist on disk (GlobalTaskPage.tsx NEW); all 3 task/artifact commits (571e96a, e221c4a, 753d1c3) found in git log; plan-level build green, both files eslint-clean in isolation, all grep gates re-run green (route, copy strings, source/engine lookups, zero Description/keydown/dangerouslySetInnerHTML/BoardWorkspaceSync).
