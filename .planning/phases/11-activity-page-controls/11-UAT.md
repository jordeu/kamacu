---
status: testing
phase: 11-activity-page-controls
source: [11-VERIFICATION.md]
started: 2026-07-31T06:18:59Z
updated: 2026-07-31T06:18:59Z
---

## Current Test

number: 1
name: Activity sidebar entry reachable in both expanded and collapsed rail
expected: |
  Open /activity in the browser with the sidebar expanded AND collapsed (Ctrl+B).
  The Activity entry (lucide Activity icon) is reachable in BOTH states — visible
  in the footer when expanded, visible as a rail icon when collapsed. Active state
  (bg-sidebar-accent) appears when on /activity.
awaiting: user response

## Tests

### 1. Activity sidebar entry reachable in both expanded and collapsed rail
expected: |
  Open /activity with the sidebar expanded AND collapsed (Ctrl+B). The Activity
  entry (lucide Activity icon) is reachable in BOTH states — visible in the footer
  when expanded, visible as a rail icon when collapsed. Active state (bg-sidebar-accent)
  appears when on /activity. Confirms the per-child collapsed-rail visibility refactor
  (ProjectSidebar.tsx:162-202).
result: [pending]

### 2. Scope dropdown lists Global + every workspace + every project, active scope checked
expected: |
  Open the scope dropdown; verify the three sections (Global / Workspaces / Projects)
  list every workspace and project with the active scope ✓-checked. All workspaces and
  projects appear (name-sorted); the current scope's item shows ✓; selecting an item
  updates the lists and stats to that scope.
result: [pending]

### 3. Week/Month toggle shows active pill and refreshes lists
expected: |
  Toggle Week ↔ Month. The segmented control shows the active pill on the chosen
  window; the lists and stats refresh for the new window.
result: [pending]

### 4. Click a tasks-done entry navigates to the task view
expected: |
  Click a tasks-done entry. Navigates to /projects/:projectId/tasks/:taskId
  (the task's view).
result: [pending]

### 5. Click a reviews-done entry opens the GitHub PR in a new tab
expected: |
  Click a reviews-done entry. Opens the GitHub PR URL in a new browser tab
  (no popup-blocker prompt). rel="noreferrer" strips the referrer.
result: [pending]

### 6. StatsStrip visual layout — counts, min·median·max rows, em-dash on empty
expected: |
  Verify the StatsStrip visual layout: counts as lead numbers; three TimeStat rows
  as `min · median · max` with `over N tasks` caption; em-dash cluster on rows where
  n===0. Stat values render in font-mono; n===0 rows show `—` (not 0m/0s);
  taskCount/reviewCount render literal 0 when empty.
result: [pending]

### 7. UAT flag — REVIEWS-03 / D-14 universal-PR-URL fallback
expected: |
  Confirm whether always opening the GitHub PR URL (the D-14 universal fallback) is
  acceptable. The implementation does NOT prefer an in-app review workspace even when
  one exists — every reviews-done click opens the PR on GitHub. D-14 is a deliberate
  simplification flagged at UAT (11-CONTEXT.md:58, 11-02-SUMMARY.md:218). The success
  criterion's primary intent ("opens the PR") is met; the workspace-preferring
  refinement was consciously dropped. A human decides if this is acceptable or whether
  the workspace-preferring resolver should be added as a follow-up.
result: [pending]

## Summary

total: 7
passed: 0
issues: 0
pending: 7
skipped: 0
blocked: 0

## Gaps
