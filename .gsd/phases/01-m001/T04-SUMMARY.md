---
id: T04
parent: S02
milestone: M001
key_files:
  - web/src/components/task/AgentTab.tsx
  - web/src/pages/TaskPage.tsx
  - web/src/pages/BoardPage.tsx
key_decisions:
  - (none)
duration: 
verification_result: passed
completed_at: 2026-07-06T16:22:50.307Z
blocker_discovered: false
---

# T04: QuotaIndicator now hidden for non-claude projects (TaskPage + BoardPage via cached useProjects+useAgents lookups); AgentTab empty-state string generalized from "claude CLI" to "agent". tsc + vite green.

**QuotaIndicator now hidden for non-claude projects (TaskPage + BoardPage via cached useProjects+useAgents lookups); AgentTab empty-state string generalized from "claude CLI" to "agent". tsc + vite green.**

## What Happened

Made the QuotaIndicator engine-aware and generalized a claude-specific AgentTab string. (1) AgentTab empty-state string changed from "Runs the claude CLI in this task's worktree" to "Runs the agent in this task's worktree" — honest for a custom-engine project. (2) TaskPage resolves the active project's agent engine via cached useProjects()+useAgents() lookups (project -> agent_id -> engine) and conditionally renders <QuotaIndicator /> only when engine !== "custom" (undefined/loading and "claude" both show; the loading case avoids a flash). (3) BoardPage got the same treatment for consistency — the board is project-scoped, so viewing a non-Claude project's board without the Claude quota widget is the honest state. The QuotaIndicator widget itself is unchanged (stays a self-contained Claude-OAuth-usage widget); the parent owns the show/hide decision. The running/exited status rendering was already handled in S01 T06 (the "running" case in StatusDot/PRCard), so no Agent-tab status work was needed here.

## Verification

cd web && npx tsc -b (exit 0) && npx vite build (exit 0). Engine resolution uses cached queries (no new fetches); undefined/loading engine -> show the indicator (avoids flash on load).

## Verification Evidence

| # | Command | Exit Code | Verdict | Duration |
|---|---------|-----------|---------|----------|
| 1 | `cd web && npx tsc -b` | 0 | ✅ pass | 8000ms |
| 2 | `cd web && npx vite build` | 0 | ✅ pass | 5000ms |

## Deviations

QuotaIndicator engine-awareness implemented via conditional rendering in the parent pages (TaskPage + BoardPage) rather than a prop on QuotaIndicator — cleaner because QuotaIndicator stays a self-contained "show my Claude quota" widget with no project/agent coupling, and the parent owns the "should this show here?" decision. Both pages resolve the engine via cached useProjects()+useAgents() lookups (no new fetches). BoardPage got the same treatment as TaskPage for consistency — viewing a non-Claude project's board without the Claude quota widget is the honest state.

## Known Issues

None.

## Files Created/Modified

- `web/src/components/task/AgentTab.tsx`
- `web/src/pages/TaskPage.tsx`
- `web/src/pages/BoardPage.tsx`
