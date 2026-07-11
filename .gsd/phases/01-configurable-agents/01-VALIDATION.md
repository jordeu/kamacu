---
verdict: pass
remediation_round: 0
---

# Milestone Validation: M001

## Success Criteria Checklist
## Success Criteria — all PASS

| # | Criterion | Verdict | Evidence |
|---|-----------|---------|----------|
| 1 | A user can add a custom agent (name + command template) in global Settings and it persists | ✅ PASS | T03 AgentsSection + useCreateAgent; human-verify gate confirmed creation; TestAgentCreate green |
| 2 | A user can set any agent as the global default; exactly one default always exists | ✅ PASS | T03 set-default star + useSetDefaultAgent; TestAgentSetDefault proves exactly-one invariant; human-verify confirmed |
| 3 | A user can choose per-project which agent runs; existing projects default to Claude | ✅ PASS | T02 Project Settings selector; migration 00013 DEFAULT 1 backfills existing projects; TestProjectCreateDefaultAgent + TestAgentsMigration |
| 4 | Spawning a task whose project uses a custom agent runs that command in the worktree PTY, shows running/exited, terminal reattach works | ✅ PASS | T05 custom engine spawn (manager.go); TestCustomEngineStatusRunningExited; human-verify gate ran the loop |
| 5 | Spawning a Claude task is byte-for-byte unchanged (working/waiting/idle, --resume, quota) | ✅ PASS | TestAgentLifecycle fake-claude regression green after every spawn-path change; TestClaudeEngineKeepsHeuristicStates |
| 6 | An agent in use by any project can't be deleted; the claude seed is non-deletable | ✅ PASS | TestAgentDeleteSystem + TestAgentDeleteInUse; ON DELETE RESTRICT FK backstop |
| 7 | Zero new Go/npm deps; frontend + Go gates green | ✅ PASS | renderAgentCommand reuses settings.Tokenize (no new deps); go test ./... + tsc -b + vite build all exit 0 |
| 8 | Human-verify gate passes | ✅ PASS | User: "both look right, approved" |

## Slice Delivery Audit
## Slice Delivery Audit

| Slice | Planned deliverable | Delivered | Status |
|-------|---------------------|-----------|--------|
| S01 | agents data foundation + spawn engine | migration 00013 + 00014, BackfillAgents + BackfillAgentExtraParams, /api/agents CRUD, agent_id threading, spawn engine fork (claude unchanged, custom render), running/exited status | As planned + the extra-params relocation (gate-surfaced) |
| S02 | agent management UI + per-project selection | Settings Agents section (AgentNameDialog + AgentsSection), Project Settings selector, engine-aware QuotaIndicator, AgentTab generalization, Active Sessions bar running state (gate-surfaced) | As planned + the two gate-surfaced fixes |

## Cross-Slice Integration
## Cross-Slice Integration

The S01→S02 boundary held cleanly:
- **Agent resolution:** S02's Project Settings selector writes `agent_id`; S01's sessions.go handler resolves `agent.engine` + `agent.command` + `agent.extra_params` from the JOIN at spawn (T05/T06). The seam is the typed `/api/agents` feed + the `projects.agent_id` column.
- **Status flow:** S01's engine branch in `agentStatusLocked` produces "running" for custom agents; S02's StatusDot/PRCard/ActiveSessionsBar consume it. The "running" case was added in S01 T06 but the bar's LIVE filter (S02 T04 scope-adjacent) was caught by the gate — fixed and re-verified.
- **Extra params:** the gate-surfaced relocation spanned both slices (migration 00014 + backfill + spawn-path in the S01 backend; AgentNameDialog field + Settings section removal in the S02 frontend). No boundary mismatch — it crossed slices correctly as a vertical feature.

No cross-slice contract drift. The `.gsd/` planning artifacts (01-01-PLAN, 01-02-PLAN) accurately tracked the work.

## Requirement Coverage
## Requirement Coverage — all 12 active requirements addressed

**Validated (6):** R012 agents table+seed, R013 projects.agent_id FK+backfill, R014 create custom agent, R015 edit agent (incl. extra_params relocation), R016 claude zero-regression, R017 custom spawn+status.

**Advanced (6):** R018 delete guard, R019 set-default exactly-one, R020 per-project resolution, R021 Settings Agents section, R022 Project selector, R023 Agent tab render + quota-awareness.

All 12 active requirements (R012-R023) have either validated proof (gate + automated) or advanced-to-delivered status. No unaddressed requirements.

## Verification Class Compliance
## Verification Class Compliance

| Class | Planned | Delivered | Status |
|-------|---------|-----------|--------|
| **Contract** | Spawn argv invariants (claude byte-for-byte; custom template render) | TestAgentLifecycle (fake-claude) green across every spawn-path change; TestRenderAgentCommand 7 cases (incl. spaces-in-path bug caught+fixed); TestCustomEngineStatus* | ✅ PASS |
| **Integration** | Agent resolution threads task→project→agent at spawn; CRUD + threading seams | TestAgentsMigration (staged-upgrade); TestProjectCreateDefaultAgent/PatchAgent; agentColumns/scanProject changes verified via the full project/workspace regression suite | ✅ PASS |
| **Operational** | Backfill hooks idempotent; startup applies migrations cleanly | TestBackfillAgents + TestBackfillAgentExtraParams (idempotent); live binary smoke test (migration 00013+00014 applied, CRUD endpoints responded, clean teardown) | ✅ PASS |
| **UAT** | Human-verify gate (6-step loop) | User ran the full loop against ./bin/kamacu: custom agent created/defaulted/selected/spawned (running/exited), Claude non-regression confirmed, bar visibility confirmed, extra-params dialog confirmed. "both look right, approved." | ✅ PASS |


## Verdict Rationale
All 8 success criteria PASS with objective evidence (automated tests + human-verify gate). Both slices delivered their planned outputs plus two gate-surfaced improvements folded in-scope (custom agents in the Active Sessions bar; extra claude params relocated into the Claude agent edit dialog). All 4 verification classes (Contract, Integration, Operational, UAT) PASS. The milestone's key risk — generalizing the spawn path without regressing Claude — is retired (fake-claude regression green across every change). No blockers, no known issues blocking completion.
