---
id: T05
parent: S02
milestone: M001
key_files:
  - web/src/components/layout/ActiveSessionsBar.tsx
  - internal/store/migrations/00014_agent_extra_params.sql
  - internal/api/agents_backfill.go
  - internal/api/agents_crud.go
  - internal/api/sessions.go
  - internal/api/sessions_test.go
  - web/src/components/settings/AgentNameDialog.tsx
  - web/src/pages/SettingsPage.tsx
  - web/src/api/types.ts
  - web/src/api/mutations.ts
key_decisions:
  - (none)
duration: 
verification_result: passed
completed_at: 2026-07-06T17:19:50.932Z
blocker_discovered: false
---

# T05: Human-verify gate PASSED (6-step loop approved); fixed two gate-surfaced gaps (custom agents in the Active Sessions bar; relocated extra claude params into the Claude agent edit dialog).

**Human-verify gate PASSED (6-step loop approved); fixed two gate-surfaced gaps (custom agents in the Active Sessions bar; relocated extra claude params into the Claude agent edit dialog).**

## What Happened

The human-verify gate ran the full 6-step loop against a live binary and APPROVED. The gate surfaced two genuine gaps that became in-scope fixes (D012: the gate may drive redesign):

1. Custom-agent sessions invisible in the Active Sessions bar — the bar's LIVE filter (built v1.6, pre-dating the 'running' state) only passed working/waiting/idle, so a custom agent's 'running' status was fetched but dropped. Fixed: widened the filter, added a running CountGroup (blue, via dotMeta), ranked running between working and idle in the expanded list, included it in the aria-label. SBAR-01's 'every active agent session' is now honest for custom agents.

2. Standalone 'Extra claude parameters' global Settings section felt orphaned — relocated onto the Claude agent row so the agent is configured wholly in its edit dialog. Migration 00014 (agents.extra_params) + BackfillAgentExtraParams startup hook (copies the effective setting value — code default OR user override — into the claude seed, idempotent) + spawn-path swap (reads extra_params from the joined agent row instead of settings.Get; fake-claude regression intact) + AgentNameDialog 'Extra parameters' field for claude-engine agents + removed the global section. This reversed M001's explicit deferral after the user confirmed (extra_params_move). 3 existing extra-params tests updated to the agent-row model.

The gate itself: (1) add a custom agent in Settings (echo/sleep), (2) set it default, (3) select it on a project, (4) spawn it in the Agent tab (saw running then exited), (5) switch back to Claude (working/waiting + resume + quota all returned with zero regression), (6) confirmed custom agents now appear in the Active Sessions bar AND the Claude agent's edit dialog carries the extra parameters. User: 'both look right, approved.'

## Verification

Human-verify gate: user ran the full 6-step loop against a live binary (./bin/kamacu) and reported 'both look right, approved'. Confirmed: (a) custom agent runs in the Agent tab with running/exited status; (b) Claude non-regression (working/waiting + resume + quota intact); (c) custom-agent sessions now appear in the Active Sessions bar (gate-surfaced fix); (d) extra claude parameters now edited in the Claude agent dialog, global section removed (gate-surfaced relocation). Automated gates throughout: go test ./... all 13 packages green; go vet clean; tsc -b + vite build exit 0; fake-claude regression (TestAgentLifecycle) intact across every spawn-path change.

## Verification Evidence

| # | Command | Exit Code | Verdict | Duration |
|---|---------|-----------|---------|----------|
| 1 | `manual human-verify gate (6-step loop, ./bin/kamacu)` | 0 | ✅ pass (user-approved) | 0ms |
| 2 | `go test ./... (after both gate fixes)` | 0 | ✅ pass | 170000ms |
| 3 | `cd web && tsc -b && vite build` | 0 | ✅ pass | 13000ms |

## Deviations

The gate surfaced two genuine gaps that became in-scope fixes (the gate working as designed, per D012): (1) custom-agent sessions were invisible in the Active Sessions bar (status 'running' wasn't in the bar's LIVE filter / count groups) — fixed by widening the filter + adding a running CountGroup in ActiveSessionsBar.tsx; (2) the standalone 'Extra claude parameters' global Settings section felt orphaned now that Claude is one agent — relocated onto the Claude agent row (migration 00014 + BackfillAgentExtraParams + spawn-path swap + AgentNameDialog field), folding what M001's CONTEXT had explicitly deferred into the milestone because the gate showed the deferral shipped an inconsistency. The deferral reversal was user-approved (extra_params_move = 'Do it properly now').

## Known Issues

None.

## Files Created/Modified

- `web/src/components/layout/ActiveSessionsBar.tsx`
- `internal/store/migrations/00014_agent_extra_params.sql`
- `internal/api/agents_backfill.go`
- `internal/api/agents_crud.go`
- `internal/api/sessions.go`
- `internal/api/sessions_test.go`
- `web/src/components/settings/AgentNameDialog.tsx`
- `web/src/pages/SettingsPage.tsx`
- `web/src/api/types.ts`
- `web/src/api/mutations.ts`
