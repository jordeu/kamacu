---
phase: 11-activity-page-controls
plan: 02
subsystem: ui
tags: [react, typescript, tanstack-query, activity, sidebar, router, dropdown-menu, tabs]

requires:
  - phase: 10-activity-data-api
    provides: GET /api/activity combined endpoint (always HTTP 200; tasks + reviews + stats)
  - phase: 11-activity-page-controls/01
    provides: useActivity query, wire types (ActivityResponse/StatsBlock/TimeStat/ActivityTask/ReviewDoneSummary/ActivityReviewState), formatDuration, useActivityView hook, ActivityScope/ActivityWindow aliases
provides:
  - /activity top-level route (bare, sibling of /settings, NOT wrapped in BoardWorkspaceSync)
  - ActivityPage — page shell (loading/error states per SettingsPage idiom) + control bar + section composition
  - ScopeSelector — single dropdown (Global/Workspaces/Projects) writing kamacu.activity.scope
  - WindowToggle — Week/Month segmented control (default-variant Tabs) writing kamacu.activity.window
  - StatsStrip — counts + 3 TimeStat rows (min·median·max) with em-dash guard on n===0
  - ActivityList — tasks-done grouped by project, click→navigate to task view
  - ReviewsList — reviews-done grouped by project, degrade-aware (null on hard-degrade), click→external PR
  - groupByProject util — client-side bucketing shared by both lists
  - Activity sidebar entry — reachable in BOTH expanded footer and collapsed icon rail (D-02)
affects: [activity-page-rendering, sidebar-navigation, uat-11]

tech-stack:
  added: []
  patterns:
    - "Per-child collapsed-rail visibility — removed SidebarFooter's wholesale group-data-[collapsible=icon]:hidden and re-applied it per-child (Add project + Settings gear keep hiding; Activity stays reachable). Generalizes the project-row dual-surface idiom to footer utility buttons."
    - "Degrade-state set with auth_required inclusion — HARD_DEGRADE ReadonlySet<ActivityReviewState> includes auth_required even though CONTEXT D-12 omits it (backend emits it; Pitfall 2). The quietly-empty return-null idiom replaces ReviewColumn's amber hint for Activity."
    - "Cross-repo merge client-side sort — reviews (unsorted server-side across repos) sorted by completedAt DESC via localeCompare before grouping; tasks (already doneAt DESC from SQL) NOT re-sorted."
    - "External link via plain <a> — never window.open (popup-blocked), never router <Link> (external URL). Mirrors PRCard.tsx idiom with aria-label + rel=noreferrer."

key-files:
  created:
    - web/src/pages/ActivityPage.tsx
    - web/src/components/activity/ScopeSelector.tsx
    - web/src/components/activity/WindowToggle.tsx
    - web/src/components/activity/StatsStrip.tsx
    - web/src/components/activity/ActivityList.tsx
    - web/src/components/activity/ReviewsList.tsx
  modified:
    - web/src/App.tsx
    - web/src/components/sidebar/ProjectSidebar.tsx

key-decisions:
  - "Removed SidebarFooter's wholesale group-data-[collapsible=icon]:hidden and re-applied it per-child rather than lifting Activity into a separate sidebar menu — the plan's recommended fix, keeps the footer as a single semantic unit while satisfying D-02 (Activity always reachable)."
  - "Sized the Activity rail button to size-8 when collapsed (group-data-[collapsible=icon]:size-8 rounded-md) instead of leaving it at icon-sm — matches the rail's larger target sizes while staying smaller than the primary project avatars (size-10)."
  - "Extracted groupByProject as a shared exported util in ActivityList.tsx, imported by ReviewsList.tsx — avoids duplicating the bucketing logic and keeps the two lists symmetric (the plan permitted either duplication or extraction)."
  - "ActivityList entry uses muted-foreground text that brightens on hover (text-muted-foreground hover:text-foreground) rather than a card/row background — keeps the page a quiet read-out, not an interactive dashboard (UI-SPEC: Activity is read-only)."

patterns-established:
  - "Per-child collapsed-rail visibility for sidebar footer utility buttons (D-02 generalization) — replace parent-level group-data-[collapsible=icon]:hidden with per-child application when one button must stay rail-reachable."
  - "Degrade-branch null-return for Activity sections — suppress the ENTIRE section (heading + body) on hard-degrade states; never render a hint/amber note (contrast ReviewColumn)."

requirements-completed: [ACT-01, ACT-02, ACT-03, ACT-04, TASKS-02, TASKS-03, REVIEWS-02, REVIEWS-03]

coverage:
  - id: D1
    description: "/activity top-level route registered as a bare sibling of /settings, NOT wrapped in BoardWorkspaceSync (scope/window live in localStorage per D-06)"
    requirement: ACT-01
    verification:
      - kind: other
        ref: "grep 'path=\"/activity\"' web/src/App.tsx → match at line 140; grep 'BoardWorkspaceSync.*Activity' → 0 matches; npm run build exits 0"
        status: pass
    human_judgment: false
  - id: D2
    description: "Activity sidebar entry reachable in BOTH expanded footer and collapsed icon rail (D-02) with active state from location.pathname === '/activity'"
    requirement: ACT-01
    verification:
      - kind: other
        ref: "grep confirms Activity lucide import, Link to='/activity' aria-label='Activity', location.pathname === '/activity' active-state class; SidebarFooter's wholesale hide removed and re-applied per-child (Add project + Settings gear carry it; Activity button does not)"
        status: pass
    human_judgment: true
    rationale: "Visual reachability in both sidebar states (expanded + collapsed via Ctrl+B) and correct active-state rendering require human verification in a browser — static grep proves the class structure but not the rendered result."
  - id: D3
    description: "ScopeSelector single dropdown (Global/Workspaces/Projects sections with DropdownMenuLabel + DropdownMenuCheckboxItem) writing kamacu.activity.scope"
    requirement: ACT-02
    verification:
      - kind: other
        ref: "grep confirms DropdownMenuLabel/CheckboxItem usage; useActivity queryKey ['activity', scope, window] (Plan 01) → one refetch on scope change"
        status: pass
    human_judgment: true
    rationale: "Dropdown opens, lists all workspaces+projects with the active scope checked, and triggers a single refetch — interactive behavior needs browser verification."
  - id: D4
    description: "WindowToggle Week/Month segmented control (default-variant Tabs) writing kamacu.activity.window"
    requirement: ACT-03
    verification:
      - kind: other
        ref: "grep confirms TabsTrigger value='week'|'month'; bound to useActivityView().setWindow → useActivity queryKey"
        status: pass
    human_judgment: true
    rationale: "Segmented control visual (active pill via data-active:bg-background) and single-refetch behavior need browser verification."
  - id: D5
    description: "Reviews section renders NOTHING (null) when reviews.state is in {disabled, no_gh, auth_required, error} with empty prs — includes auth_required per Pitfall 2 (ACT-04 / D-12 quietly empty)"
    requirement: ACT-04
    verification:
      - kind: other
        ref: "grep confirms HARD_DEGRADE set includes all four states incl auth_required; ReviewsList returns null when HARD_DEGRADE.has(state) && prs.length===0"
        status: pass
    human_judgment: true
    rationale: "The 'quietly empty' behavior (no heading, no hint, no toast) with GitHub integration off / gh absent needs browser verification — static grep proves the branch but not the rendered silence."
  - id: D6
    description: "StatsStrip renders counts as lead numbers + 3 TimeStat rows (min·median·max) with em-dash on n===0 BEFORE formatDuration (D-08/D-09/D-11, Pitfall 4)"
    requirement: ACT-02
    verification:
      - kind: other
        ref: "grep confirms stat.n === 0 guard precedes formatDuration calls; counts render literal; npm run build exits 0"
        status: pass
    human_judgment: true
    rationale: "Stat row visual (min·median·max spacing, em-dash rendering, 'over N tasks' caption) needs browser verification."
  - id: D7
    description: "ActivityList groups tasks-done by project under project-name subheading, each entry navigates to /projects/:projectId/tasks/:taskId"
    requirement: TASKS-02
    verification:
      - kind: other
        ref: "grep confirms groupByProject util + Link to={`/projects/${task.projectId}/tasks/${task.id}`; npm run build exits 0"
        status: pass
    human_judgment: true
    rationale: "Grouped rendering under project subheadings + click-through navigation need browser verification."
  - id: D8
    description: "ActivityList entry click navigates to that task's view (D-15 — activityTask carries id + projectId, no lookup)"
    requirement: TASKS-03
    verification:
      - kind: other
        ref: "grep confirms Link to={`/projects/${task.projectId}/tasks/${task.id}`} (internal navigation, not external)"
        status: pass
    human_judgment: true
    rationale: "Click-through reaches the correct task view — needs browser verification."
  - id: D9
    description: "ReviewsList groups reviews-done by project under project-name subheading, each entry shows #number + title + merge/close date"
    requirement: REVIEWS-02
    verification:
      - kind: other
        ref: "grep confirms groupByProject reuse; entry renders #{number} {title} + formatAgo(completedAt); prs sorted by completedAt DESC via localeCompare (Pitfall 5)"
        status: pass
    human_judgment: true
    rationale: "Grouped rendering + reverse-chronological ordering within groups need browser verification."
  - id: D10
    description: "ReviewsList entry click opens PR URL in a new tab via plain <a href target=_blank rel=noreferrer> (D-14 universal PR-URL fallback; never window.open, never router Link)"
    requirement: REVIEWS-03
    verification:
      - kind: other
        ref: "grep confirms rel=\"noreferrer\" present (2 matches: the entry + docstring); window.open appears ONLY in a NEVER-comment (0 actual calls); href={review.url}; aria-label `Open PR #${number} on GitHub`"
        status: pass
    human_judgment: true
    rationale: "Click opens the GitHub PR in a new tab without popup-blocking — needs browser verification. D-14 is a deliberate simplification (universal PR URL, no in-app workspace lookup) to flag at UAT."

duration: 5 min
completed: 2026-07-31
status: complete
---

# Phase 11 Plan 02: Activity Page & Controls Summary

**Dedicated /activity route + 6 new components (ActivityPage, ScopeSelector, WindowToggle, StatsStrip, ActivityList, ReviewsList) + sidebar entry — renders Phase 10's GET /api/activity as a usable page with scope/window controls, grouped lists, and quiet degrade**

## Performance

- **Duration:** 5 min
- **Started:** 2026-07-31T05:54:51Z
- **Completed:** 2026-07-31T06:00:32Z
- **Tasks:** 2
- **Files modified:** 8 (6 new, 2 modified)

## Accomplishments

- `/activity` top-level route registered as a bare sibling of `/settings` (NOT wrapped in BoardWorkspaceSync — scope/window live in localStorage per D-06)
- ActivityPage mirrors the SettingsPage shell (loading Skeletons, error+retry, max-w-[640px] column) and composes ScopeSelector + WindowToggle + StatsStrip + ActivityList + ReviewsList in a stacked single column (D-08)
- ScopeSelector: single dropdown (D-04) with Global/Workspaces/Projects sections via DropdownMenuLabel + DropdownMenuCheckboxItem; name-sorted for stable order
- WindowToggle: default-variant Tabs as the Week/Month segmented control (D-07) — zero-new-code reuse
- StatsStrip: taskCount/reviewCount lead numbers + 3 TimeStat rows (Cycle / In progress / In review) rendered as `min · median · max`; em-dash guard on `stat.n === 0` BEFORE any formatDuration call (D-11 / Pitfall 4)
- ActivityList: tasks-done grouped by project via a shared `groupByProject` util; each entry is a Link to `/projects/:projectId/tasks/:taskId` (D-15); tasks arrive doneAt DESC from server so NO re-sort
- ReviewsList: degrade branch returns `null` on HARD_DEGRADE `{disabled, no_gh, auth_required, error}` with empty prs (D-12 quietly empty, includes auth_required per Pitfall 2); sorts prs by completedAt DESC client-side via localeCompare (Pitfall 5); entries are plain `<a href target=_blank rel=noreferrer>` (D-14, Pitfall 7)
- Activity sidebar entry reachable in BOTH expanded footer and collapsed icon rail — removed the footer's wholesale `group-data-[collapsible=icon]:hidden` and re-applied it per-child (Add project + Settings gear keep hiding; Activity stays reachable), with active state from `location.pathname === "/activity"` (D-01/D-02/D-03)

## Task Commits

Each task was committed atomically:

1. **Task 1: /activity route, ActivityPage shell, ScopeSelector, WindowToggle, StatsStrip** — `bc85fac` (feat)
2. **Task 2: ActivityList, ReviewsList, wire lists into ActivityPage, sidebar entry** — `d4245dd` (feat)

**Plan metadata:** (pending final docs commit)

## Files Created/Modified

- `web/src/App.tsx` — Added `import ActivityPage` + bare `<Route path="/activity" element={<ActivityPage />} />` as sibling of /settings (NOT wrapped in BoardWorkspaceSync)
- `web/src/pages/ActivityPage.tsx` — NEW: page shell mirroring SettingsPage (loading Skeletons, error+retry, max-w-[640px]); calls useActivityView() + useActivity(scope, window); composes the 5 sub-components
- `web/src/components/activity/ScopeSelector.tsx` — NEW: single dropdown (D-04) with Global/Workspaces/Projects sections; writes scope via onScopeChange
- `web/src/components/activity/WindowToggle.tsx` — NEW: Week/Month segmented control via default-variant Tabs (D-07); writes window via onWindowChange
- `web/src/components/activity/StatsStrip.tsx` — NEW: counts + 3 TimeStat rows with em-dash guard on n===0 (D-08/D-09/D-11)
- `web/src/components/activity/ActivityList.tsx` — NEW: tasks-done grouped by project (exports `groupByProject` util); click→navigate to task view (D-15)
- `web/src/components/activity/ReviewsList.tsx` — NEW: reviews-done grouped by project; HARD_DEGRADE null-return (D-12 + auth_required); client-side completedAt DESC sort (Pitfall 5); plain `<a>` external link (D-14, Pitfall 7)
- `web/src/components/sidebar/ProjectSidebar.tsx` — Added Activity lucide icon entry; removed footer's wholesale hide and re-applied per-child so Activity stays rail-reachable (D-02 / Pitfall 1)

## Decisions Made

- **Per-child collapsed-rail visibility over a separate sidebar menu:** Removed `SidebarFooter`'s wholesale `group-data-[collapsible=icon]:hidden` and re-applied it per-child (Add project + Settings gear keep hiding; Activity does not) rather than lifting Activity into a separate SidebarMenu element. The plan's recommended fix — keeps the footer as a single semantic unit while satisfying D-02.
- **Rail button sizing:** Sized the Activity rail button to `size-8` when collapsed (`group-data-[collapsible=icon]:size-8 rounded-md`) instead of leaving it at `icon-sm` (size-7). Matches the rail's larger target sizes while staying smaller than the primary project avatars (size-10) so it reads as a footer utility, not primary navigation.
- **Shared `groupByProject` util:** Extracted the bucketing util as an exported function in ActivityList.tsx and imported it into ReviewsList.tsx, rather than duplicating it locally. The plan permitted either; extraction keeps the two lists symmetric and DRY.
- **Muted entry styling:** ActivityList/ReviewsList entries use `text-muted-foreground hover:text-foreground` rather than a card/row background — keeps the page a quiet read-out, not an interactive dashboard (UI-SPEC: Activity is read-only with no primary CTA).

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None — the plan was well-specified and both Wave 1 symbols (importable as documented in 11-01-SUMMARY) and the analog templates (SettingsPage shell, WorkspaceSwitcher dropdown, ProjectSidebar footer) were exact. The one structural wrinkle (D-02 / Pitfall 1 — the footer's wholesale hide) was anticipated by the plan with a concrete recommended fix.

## User Setup Required

None — no external service configuration required. This plan adds zero npm packages (all dependencies already in package.json — verified in RESEARCH §Package Legitimacy Audit; threat model T-11-SC trivially satisfied).

## Next Phase Readiness

- **Phase 11 complete:** All 8 Phase 11 requirements (ACT-01..04, TASKS-02/03, REVIEWS-02/03) are functionally delivered. The Activity page is reachable app-wide from the sidebar in both expanded and collapsed states.
- **Ready for UAT:** The `<verification>` block's manual checks (open /activity, verify sidebar entry in both states, scope dropdown lists Global+workspaces+projects, Week/Month toggle switches the window, stats render with em-dash on empty, tasks-done navigate to task view, reviews-done open PR in new tab, reviews section absent with GitHub integration off) are all browser-verifiable.
- **D-14 UAT flag:** REVIEWS-03's "review workspace if it exists, else the PR" is implemented as the universal PR-URL fallback (always opens GitHub). If UAT insists on the workspace preference, the resolver (find a `source='github_pr'` task matching `pr_number` in the entry's project) is the upgrade path — documented in CONTEXT D-14.
- **No blockers:** `npm run build` (tsc -b && vite build) passes. The Go binary's embed.FS can consume the dist/ output.

## Self-Check: PASSED

- All 6 new files exist on disk (ActivityPage.tsx, ScopeSelector.tsx, WindowToggle.tsx, StatsStrip.tsx, ActivityList.tsx, ReviewsList.tsx) — `[ -f ]` confirmed via the acceptance-criteria greps
- Both modified files updated (App.tsx /activity route at line 140; ProjectSidebar.tsx Activity entry at line 196)
- Both commits found in git log (bc85fac Task 1, d4245dd Task 2)
- `cd web && npx tsc --noEmit` exits 0
- `cd web && npm run build` (tsc -b && vite build) exits 0
- All 11 Task 1 + 12 Task 2 acceptance criteria greps return matches (verified inline before each commit)

---
*Phase: 11-activity-page-controls*
*Completed: 2026-07-31*
