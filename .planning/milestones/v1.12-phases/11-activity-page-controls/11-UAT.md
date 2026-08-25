---
status: complete
phase: 11-activity-page-controls
source: [11-VERIFICATION.md]
started: 2026-07-31T06:18:59Z
updated: 2026-07-31T17:52:18Z
---

## Current Test

[testing complete]

## Tests

### 1. Activity sidebar entry reachable in both expanded and collapsed rail
expected: |
  Open /activity with the sidebar expanded AND collapsed (Ctrl+B). The Activity
  entry (lucide Activity icon) is reachable in BOTH states — visible in the footer
  when expanded, visible as a rail icon when collapsed. Active state (bg-sidebar-accent)
  appears when on /activity. Confirms the per-child collapsed-rail visibility refactor
  (ProjectSidebar.tsx:162-202).
result: pass
prior_result: issue (blocker — black page on empty instance, reported 2026-07-31 vs http://127.0.0.1:7334/activity)
reverify_after: fix(11-03) ef4c585 — non-nil activity wire contract + null-guard render path
reverify_note: "2026-07-31 — re-verified against a fresh/empty instance on 7334 (--db /tmp/kamacu-uat.db); page now renders instead of blanking. Fix confirmed."

### 2. Scope dropdown lists Global + every workspace + every project, active scope checked
expected: |
  Open the scope dropdown; verify the three sections (Global / Workspaces / Projects)
  list every workspace and project with the active scope ✓-checked. All workspaces and
  projects appear (name-sorted); the current scope's item shows ✓; selecting an item
  updates the lists and stats to that scope. (Requires an instance WITH DATA — prod 7333.)
result: pass

### 3. Week/Month toggle shows active pill and refreshes lists
expected: |
  Toggle Week ↔ Month. The segmented control shows the active pill on the chosen
  window; the lists and stats refresh for the new window.
result: pass
reverify_note: "2026-07-31 — verified on 7334 with seeded data (5 tasks in Week, 8 in Month); toggle + counts correct."

### 4. Click a tasks-done entry navigates to the task view
expected: |
  Click a tasks-done entry. Navigates to /projects/:projectId/tasks/:taskId
  (the task's view).
result: pass
reverify_note: "2026-07-31 — verified on 7334 seeded data; route navigation works (task-view shell loads; fake repo_paths are out of scope)."

### 5. Click a reviews-done entry opens the GitHub PR in a new tab
expected: |
  Click a reviews-done entry. Opens the GitHub PR URL in a new browser tab
  (no popup-blocker prompt). rel="noreferrer" strips the referrer.
result: pass
reverify_note: "2026-07-31 — verified on 7334 against a sanitized hot-copy of prod (13 linked repos incl. seqeralabs/sched); reviews populated live via gh; click opened the PR in a new tab."

### 6. StatsStrip visual layout — counts, min·median·max rows, em-dash on empty
expected: |
  Verify the StatsStrip visual layout: counts as lead numbers; three TimeStat rows
  as `min · median · max` with `over N tasks` caption; em-dash cluster on rows where
  n===0. Stat values render in font-mono; n===0 rows show `—` (not 0m/0s);
  taskCount/reviewCount render literal 0 when empty.
result: pass
reverify_note: "2026-07-31 — verified on 7334 prod-copy; counts, min·median·max rows, em-dash on empty, and font-mono rendering all correct; recomputed on scope change."

### 7. UAT flag — REVIEWS-03 / D-14 universal-PR-URL fallback
expected: |
  Confirm whether always opening the GitHub PR URL (the D-14 universal fallback) is
  acceptable. The implementation does NOT prefer an in-app review workspace even when
  one exists — every reviews-done click opens the PR on GitHub. D-14 is a deliberate
  simplification flagged at UAT (11-CONTEXT.md:58, 11-02-SUMMARY.md:218). The success
  criterion's primary intent ("opens the PR") is met; the workspace-preferring
  refinement was consciously dropped. A human decides if this is acceptable or whether
  the workspace-preferring resolver should be added as a follow-up.
result: pass
decision: "acceptable — always-opening-GitHub (D-14 universal fallback) is fine; no in-app-workspace-preferring follow-up needed."

## Summary

total: 7
passed: 7
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps

- truth: "GET /activity renders the activity page (sidebar entry + scope/window controls + stats + tasks + reviews sections). On an empty instance it renders a BLANK/BLACK page instead."
  status: resolved
  reason: "User reported: 'when I go to /activity page I get a black page, nothing else' (verified against http://127.0.0.1:7334/activity, a fresh/empty Kamacu instance on a separate DB)"
  severity: blocker
  test: 1
  resolved_by: "fix(11-03) ef4c585 — non-nil activity wire contract (tasks:[]/reviews.prs:[]) on every backend path + frontend null guards + byte-level regression test"
  root_cause: >
    Backend emits `tasks: null` and `reviews.prs: null` because Go nil slices
    marshal to JSON null (not []). On an empty instance fetchActivityTasks
    returns a nil []activityTask (activity.go:152) and aggregateReviews returns
    nil PRs (activity.go:61, 104), so the API violates its own ActivityTask[]/
    ReviewDoneSummary[] contract. The frontend has no null guard, so React's
    render throws and unmounts to a blank page. The main 7333 instance has real
    done tasks/reviews so the slices are non-nil — that is why dev + phase-11
    verification passed but a fresh instance blanks.
  artifacts:
    - path: "internal/api/activity.go"
      issue: "fetchActivityTasks uses `var out []activityTask` (nil) -> null when empty (L152); error-path omits Tasks -> null (L238-241); aggregateReviews empty-repos path returns nil PRs (L61) and rollup allPRs is nil when no PRs (L104)."
    - path: "web/src/components/activity/ActivityList.tsx"
      issue: "groupByProject does `for (const r of rows)` with no null guard (L33) -> TypeError when tasks=null."
    - path: "web/src/components/activity/ReviewsList.tsx"
      issue: "L59 `[...reviews.prs].sort(...)` -> TypeError when prs=null (would crash after tasks fix)."
    - path: "web/src/pages/ActivityPage.tsx"
      issue: "L58-59 passes data.tasks / data.reviews straight through with no `?? []` guard."
  missing:
    - "Backend (canonical contract fix): initialize empty slices so empty collections marshal to [] — fetchActivityTasks `out := []activityTask{}`; aggregateReviews empty paths `PRs: []github.ReviewDoneSummary{}`; error-path activityResponse `Tasks: []activityTask{}`."
    - "Frontend (defense in depth): null-guard groupByProject (`rows ?? []`) and ActivityPage (`data.tasks ?? []`; `reviews.prs ?? []`)."
    - "Add backend test asserting empty activity -> tasks:[] and reviews.prs:[] (not null)."
  debug_session: ""
