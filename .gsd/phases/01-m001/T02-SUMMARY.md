---
id: T02
parent: S02
milestone: M001
key_files:
  - web/src/components/sidebar/ProjectSettingsDialog.tsx
  - web/src/api/mutations.ts
key_decisions:
  - (none)
duration: 
verification_result: passed
completed_at: 2026-07-06T16:18:14.295Z
blocker_discovered: false
---

# T02: Added Agent selector (shadcn Select) to Project Settings with conditional-PATCH on change; extended useUpdateProjectSettings to carry agent_id. tsc + vite green.

**Added Agent selector (shadcn Select) to Project Settings with conditional-PATCH on change; extended useUpdateProjectSettings to carry agent_id. tsc + vite green.**

## What Happened

Added an Agent selector (shadcn Select) to ProjectSettingsDialog. New agentId state (string, init from project.agent_id), reset on dialog open alongside the other fields, rendered as a Select populated by useAgents() with each option labeled "Name (default)" for the default agent. The Save handler sends agent_id via useUpdateProjectSettings only when it actually changed (the conditional-PATCH pattern, matching github_repo/icon_letters). Extended useUpdateProjectSettings in mutations.ts to accept agent_id (undefined = untouched). Verified tsc -b + vite build both exit 0.

## Verification

cd web && npx tsc -b (exit 0) && npx vite build (exit 0). The selector renders from useAgents(), defaults to the project's current agent_id, and PATCHes only on change.

## Verification Evidence

| # | Command | Exit Code | Verdict | Duration |
|---|---------|-----------|---------|----------|
| 1 | `cd web && npx tsc -b` | 0 | ✅ pass | 8000ms |
| 2 | `cd web && npx vite build` | 0 | ✅ pass | 5000ms |

## Deviations

Also extended useUpdateProjectSettings in mutations.ts to accept agent_id (conditional-PATCH, same shape as the icon fields). The agent_id is held as a string in component state (Select control value) and parsed to number on PATCH.

## Known Issues

None.

## Files Created/Modified

- `web/src/components/sidebar/ProjectSettingsDialog.tsx`
- `web/src/api/mutations.ts`
