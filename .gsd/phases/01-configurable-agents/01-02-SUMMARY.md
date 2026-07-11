---
id: S02
parent: M001
milestone: M001
provides:
  - Agents management UI (global Settings section + Project Settings selector)
  - Engine-aware QuotaIndicator (hidden for non-claude projects)
  - Custom agents fully carried through the Active Sessions bar (running count + row)
  - Extra claude params edited in the Claude agent dialog (relocated from a global setting)
requires:
  []
affects:
  []
key_files:
  - web/src/api/types.ts
  - web/src/api/queries.ts
  - web/src/api/mutations.ts
  - web/src/components/sidebar/ProjectSettingsDialog.tsx
  - web/src/components/settings/AgentNameDialog.tsx
  - web/src/components/settings/AgentsSection.tsx
  - web/src/pages/SettingsPage.tsx
  - web/src/pages/TaskPage.tsx
  - web/src/pages/BoardPage.tsx
  - web/src/components/task/AgentTab.tsx
  - web/src/components/layout/ActiveSessionsBar.tsx
  - internal/store/migrations/00014_agent_extra_params.sql
  - internal/api/agents_backfill.go
  - internal/api/agents_crud.go
  - internal/api/sessions.go
key_decisions:
  - The Agents management section uses a star (★/☆) set-default control + a Badge engine tag (Claude/Custom) — reads cleanly in the row's icon-button vocabulary
  - QuotaIndicator engine-awareness lives in the parent pages (conditional render) not on the widget — keeps QuotaIndicator self-contained with no project/agent coupling
  - The gate's two surfaced gaps (bar visibility, extra-params relocation) were folded in-scope rather than deferred — the gate's job (D012) is to catch exactly these, and shipping a known inconsistency was the worse trade
  - Extra-params relocation used a backfill (not an in-migration UPDATE) so the code default survives even when no settings row exists — mirrors BackfillAgents/BackfillWorkspaces
patterns_established:
  - (none)
observability_surfaces:
  - none
drill_down_paths:
  []
duration: ""
verification_result: passed
completed_at: 2026-07-06T17:20:20.046Z
blocker_discovered: false
---

# S02: Agent management UI and per-project selection

**Built the agent management UI (Settings Agents section + Project selector + engine-aware quota) and passed the human-verify gate, which surfaced two in-scope fixes (custom agents in the Active Sessions bar; extra claude params relocated into the Claude agent edit dialog).**

## What Happened

S02 delivered the full UI of Configurable Agents across 5 tasks. The frontend agents API client (Agent type, useAgents query, 4 CRUD/default mutations) mirrors the workspaces surface. The Project Settings agent selector (shadcn Select, conditional-PATCH on change) lets the user pick an agent per project. The global Settings Agents section (AgentNameDialog create/edit + AgentsSection hub with set-default star control, edit, guarded delete with is_system/in-use tooltips, engine badge) is the management surface. The QuotaIndicator is hidden for non-Claude projects (TaskPage + BoardPage via cached engine lookups), and the AgentTab string generalized from "claude CLI" to "agent".

The human-verify gate (T05) ran the 6-step loop against a live binary and PASSED, then surfaced two genuine gaps that became in-scope fixes (D012: the gate may redesign): (1) custom-agent sessions were invisible in the Active Sessions bar because the v1.6 bar's LIVE filter predated the new 'running' state -- fixed by widening the filter + adding a running count group; (2) the standalone 'Extra claude parameters' global Settings section felt orphaned now that Claude is one agent -- relocated onto the Claude agent row (migration 00014 + BackfillAgentExtraParams + spawn-path swap + AgentNameDialog field), reversing an explicit M001 deferral after the user confirmed.

Verification: the gate is the human-verify pass. Automated gates throughout: go test ./... all 13 packages green; go vet clean; tsc -b + vite build exit 0; fake-claude regression intact.

## Verification

Human-verify gate PASSED (user: "both look right, approved"). Full automated suite green throughout S02: go test ./... all 13 packages; go vet clean; tsc -b + vite build exit 0; fake-claude regression (TestAgentLifecycle) intact across all spawn-path changes including the extra-params relocation.

## Requirements Advanced

- R021 — Settings Agents section + AgentNameDialog (create/edit)
- R022 — Project Settings agent selector (conditional-PATCH)
- R023 — AgentTab renders any agent; QuotaIndicator hidden for non-claude; running status in StatusDot + bar
- R017 — Custom agents now appear in the Active Sessions bar (gate fix)
- R015 — Extra claude params relocated into the Claude agent edit dialog (gate fix)

## Requirements Validated

- R021 — Human-verify gate: user added/edited/deleted agents in the Settings Agents section, set a default
- R022 — Human-verify gate: user selected a custom agent on a project via the dropdown
- R023 — Human-verify gate: custom agent ran with running/exited status; Claude quota returned on switch-back
- R014 — Human-verify gate: user created a custom agent (name + command template) in Settings
- R016 — Human-verify gate + fake-claude regression: claude path working/waiting/resume intact across all spawn-path changes
- R017 — Human-verify gate: custom agent ran in the Agent tab with running then exited status; bar shows running count

## New Requirements Surfaced

None.

## Requirements Invalidated or Re-scoped

None.

## Operational Readiness

None.

## Deviations

None.

## Known Limitations

None. The agent_extra_params global setting key remains in the settings defaults/validate maps as dead config (harmless; removing risks a stale-frontend PUT returning 400). Could be cleaned up in a future milestone.

## Follow-ups

None.

## Files Created/Modified

None.
