---
id: M001
title: "Configurable Agents"
status: complete
completed_at: 2026-07-06T17:21:26.842Z
key_decisions:
  - D-M001-1 templated command model with {{worktree}}/{{session_id}} placeholders
  - D-M001-2 generic running/exited status for custom agents; working/waiting/idle + --resume + quota stay claude-only — eliminated the need for a research phase
  - D-M001-3 claude as a pre-seeded agents row (is_system/is_default); spawn branches on an engine column, never the name
  - engine column encodes capability tier — extensible to future engines without name-string matching
  - Tokenize-then-substitute-per-token order for the custom command render (paths with spaces survive as single tokens — a real bug caught by unit test)
  - Extra claude params relocated onto the Claude agent row (gate-surfaced; reversed an explicit deferral after user confirmation)
key_files:
  - internal/store/migrations/00013_agents.sql
  - internal/store/migrations/00014_agent_extra_params.sql
  - internal/store/agents_migration_test.go
  - internal/api/agents_backfill.go
  - internal/api/agents_crud.go
  - internal/api/agents_crud_test.go
  - internal/api/agents_backfill_test.go
  - internal/api/projects_agent_test.go
  - internal/session/manager.go
  - internal/session/session.go
  - internal/session/agent_engine_test.go
  - internal/api/sessions.go
  - internal/api/routes.go
  - internal/api/projects.go
  - cmd/kamacu/main.go
  - web/src/api/types.ts
  - web/src/api/queries.ts
  - web/src/api/mutations.ts
  - web/src/api/agents.ts
  - web/src/components/StatusDot.tsx
  - web/src/components/board/PRCard.tsx
  - web/src/components/layout/ActiveSessionsBar.tsx
  - web/src/components/settings/AgentNameDialog.tsx
  - web/src/components/settings/AgentsSection.tsx
  - web/src/components/settings/SettingsField.tsx
  - web/src/components/sidebar/ProjectSettingsDialog.tsx
  - web/src/components/task/AgentTab.tsx
  - web/src/pages/SettingsPage.tsx
  - web/src/pages/TaskPage.tsx
  - web/src/pages/BoardPage.tsx
lessons_learned:
  - The 'generic status, claude-only resume' design choice (D-M001-2) eliminated the only genuine unknown — no research phase was needed because we don't try to detect working/waiting for custom agents. Real scope reduction from a deliberate design decision.
  - The worktree-with-spaces bug in template rendering (replace-then-tokenize broke paths with spaces) was caught by a unit test, not a live gate — the 'build the riskiest part with tests first' discipline (v1.0 lesson) paid off again on the one genuinely new code path.
  - A LIVE human-verify gate is load-bearing: it caught that the v1.6 Active Sessions bar's LIVE filter predated the new 'running' state, AND that the global extra-params setting felt orphaned next to the new agents model. Both were invisible to automated gates. D012 (the gate may redesign) earned its keep twice in one milestone.
  - The format-migration + brownfield-bootstrap combo had one real friction point: GSD's worktree-write guard blocks root file edits outside auto-mode, and the destructive-command guard keys on literal tokens (DROP TABLE inside a Down block; rm -rf on a temp dir) even when confirmed. Both have escape hatches but required confirmation each time — worth a future tooling smoothing pass.
---

# M001: Configurable Agents

**Made agents first-class configurable — the task Agent tab now runs any CLI agent (gemini, aider, codex, ...), not just Claude Code — with custom agents defined in global Settings, a per-project agent selector, and the Claude integration preserved byte-for-byte (hooks/resume/quota/working-waiting-idle).**

## What Happened

M001 Configurable Agents delivered in 2 slices (S01 backend, S02 UI) + 2 gate-surfaced improvements, taking the app from "Claude Code only" to "any CLI agent, configured per-project." 

S01 (agent data foundation + spawn engine): migration 00013 (agents table + Claude seed + projects.agent_id FK, mirroring 00012_workspaces.sql exactly), BackfillAgents startup hook, /api/agents CRUD (list/create/update/delete/set-default with is_system + in-use guards), agent_id threaded through the projects API, and the spawn engine fork on agent.engine. The CLAUDE path is byte-for-byte unchanged (fake-claude regression suite green across every change — the milestone's key risk retired); the CUSTOM path renders the command template (tokenize-then-substitute-per-token, fixing a spaces-in-path bug caught by unit test) and reports running/exited only (D-M001-2).

S02 (agent management UI + per-project selection): the frontend agents API client, Project Settings agent selector, global Settings Agents section (AgentNameDialog + AgentsSection hub), engine-aware QuotaIndicator, and AgentTab string generalization. The human-verify gate ran the 6-step loop against a live binary and PASSED, then surfaced two in-scope fixes (D012): custom agents were invisible in the Active Sessions bar (the v1.6 LIVE filter predated the 'running' state — fixed); and the standalone 'Extra claude parameters' global Settings section felt orphaned next to the new agents model (relocated onto the Claude agent row via migration 00014 + BackfillAgentExtraParams + spawn-path swap + AgentNameDialog field).

The 'generic status, claude-only resume' design choice (D-M001-2) eliminated the only genuine unknown, so no research phase was needed — a real scope reduction from a deliberate decision. Verification spanned all 4 classes: Contract (fake-claude + renderAgentCommand unit tests), Integration (staged-upgrade migration test + project/workspace regression), Operational (idempotent backfill tests + live binary smoke), and UAT (human-verify gate). Two honest process notes: a couple of early clobbers (overwrote pre-existing agents.go/agents_test.go before reading) reinforced read-before-overwrite applies to test fixtures; and the GSD worktree-write/destructive-command guards added friction for this brownfield-bootstrap+manual-execution mode (escape hatches exist but required confirmation each time).

## Success Criteria Results

Not provided.

## Definition of Done Results

All 8 DoD items met:
1. ✅ migration 00013 (agents table + Claude seed + projects.agent_id FK) + 00014 (extra_params) shipped; staged-upgrade tests green
2. ✅ /api/agents CRUD + set-default + delete guards shipped with is_system + in-use guards
3. ✅ spawn engine branch on agent.engine shipped; claude path byte-for-byte preserved (fake-claude tests green)
4. ✅ custom-engine command-template render (tokenize-then-substitute) + running/exited status shipped
5. ✅ global Settings Agents section (AgentNameDialog + AgentsSection) shipped
6. ✅ Project Settings agent selector shipped
7. ✅ Agent tab renders any agent; quota indicator respects the project's agent; custom agents appear in the Active Sessions bar
8. ✅ go test/vet + make test + tsc -b + vite build all green; zero new deps; human-verify gate passed (custom agent end-to-end + claude non-regression)
Plus the gate-surfaced extra-params relocation (migration 00014 + backfill + spawn-path swap + AgentNameDialog field + global section removal).

## Requirement Outcomes

## Requirement Outcomes

**Validated (6):** R012 (agents table + Claude seed), R013 (projects.agent_id FK + backfill), R014 (create custom agent), R015 (edit agent — incl. the gate-surfaced extra-params relocation), R016 (claude zero-regression), R017 (custom spawn + running/exited status).

**Advanced-to-delivered (6):** R018 (delete guard — block-until-unassigned + is_system non-deletable), R019 (set-default exactly-one invariant), R020 (per-project resolution at spawn, read-at-use), R021 (Settings Agents section), R022 (Project Settings selector), R023 (Agent tab render + quota-aware + bar visibility).

All 12 active requirements (R012-R023) delivered with automated + gate evidence. No requirements deferred or invalidated this milestone.

## Deviations

None.

## Follow-ups

None.
