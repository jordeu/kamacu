---
phase: 04-claude-code-agent-sessions
plan: 02
subsystem: api
tags: [go, rest, websocket, hooks, claude-cli, sqlite, goose, security]

# Dependency graph
requires:
  - phase: 04-claude-code-agent-sessions (plan 01)
    provides: Kind/SpawnOpts/AgentConfig, SetWaiting/SetIdle/MarkHooksAlive, ClearWaitingOnAttach, ClaudeSessionID, Info.AgentStatus/StopRequested
  - phase: 03-worktree-isolation
    provides: task worktree_path lookup, ListByTask/StopAllForTask, task-scoped session REST
  - phase: 02-terminal-engine
    provides: WS attach path, hostCheck middleware, session REST surface
provides:
  - POST /api/sessions kind discriminator — agent spawn with one-per-task 409 and claude_session_id persistence (latest wins)
  - POST /api/hooks/sessions/{id} token-gated hook receiver (Notification→waiting, Stop→idle, SessionStart→hooks alive)
  - GET /api/agents/status — newest agent per task with exact 04-03 JSON contract, [] when empty
  - DELETE /api/tasks/{id} stops all running task sessions before the row delete (gap #1 closed)
  - WS attach clears waiting server-side (D-45)
  - migration 00003 tasks.claude_session_id (Phase 5 --resume key)
  - per-instance crypto/rand hook token + --claude-bin override wired in main.go
affects: [04-03 board status surfaces, 04-04, 04-05, 05 recovery/resume]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Hook receiver order: token (constant-time, empty-config rejects) → session lookup → exited-ignore → decode → dispatch on hook_event_name only"
    - "Status aggregation: mgr.List() newest-first, first agent entry per task wins; one IN-query resolves project ids"
    - "api/ws test packages carry their own fake-claude stub writers (AgentConfig.ClaudeBin) — the real binary is never spawned in tests"

key-files:
  created:
    - internal/store/migrations/00003_agent_sessions.sql
    - internal/api/hooks.go
    - internal/api/hooks_test.go
    - internal/api/agents.go
    - internal/api/agents_test.go
  modified:
    - internal/api/sessions.go
    - internal/api/sessions_test.go
    - internal/api/tasks.go
    - internal/api/tasks_test.go
    - internal/api/routes.go
    - internal/api/projects_test.go
    - internal/api/worktrees_test.go
    - internal/api/integration_test.go
    - internal/ws/handler.go
    - internal/ws/handler_test.go
    - cmd/kangent/main.go

key-decisions:
  - "kind=agent without task_id returns the same 409 'task has no worktree' body as a worktree-less task — one copy contract for the no-worktree gate"
  - "Hook receiver rejects an empty configured token outright — the endpoint can never run open even if main's wiring regressed"
  - "hooks-alive gating tested observably from the api package: the fake-claude stub answers each input line with a bare BEL + marker in one write, so Snapshot() containing the marker proves the BEL was processed under the same lock"
  - "Exited-session hook POSTs checked before body decode — late async curls are ignored with 200 even when malformed"

patterns-established:
  - "Status transport entries always marshal exitCode explicitly (null while running) — frontend never branches on key presence"
  - "Per-package fake-claude stubs: hooks_test.go (interactive BEL echo) vs ws handler_test.go (idle-until-TERM)"

requirements-completed: [TERM-01, STAT-01, STAT-02]

# Metrics
duration: 18min
completed: 2026-06-10
---

# Phase 4 Plan 02: Agent REST/WS Surface Summary

**Full agent backend over REST and WS: kind-discriminated agent spawn with one-per-task 409 and claude_session_id persistence, a constant-time token-gated hook receiver driving the D-47 state machine, the 5s-pollable GET /api/agents/status board source, delete-stops-sessions, and D-45 attach-clears-waiting — all TDD against fake-claude stubs**

## Performance

- **Duration:** 18 min
- **Started:** 2026-06-10T18:51:34Z
- **Completed:** 2026-06-10T19:10:01Z
- **Tasks:** 3 (all TDD: RED+GREEN commits each)
- **Files modified:** 16

## Accomplishments

- `POST /api/hooks/sessions/{id}` translates verified v2.1.170 hook payloads into precise status transitions behind a per-instance `crypto/rand` token (constant-time compare, never logged, empty-config rejects everything) — research gap #2 (localhost hook spoofing) closed with explicit 401 tests
- `POST /api/sessions` gained `kind`: agent spawns require a task worktree, enforce exactly one running agent per task (409 `agent session already running`, fresh spawn allowed after exit per D-41 "Start again"), and persist `claude_session_id` to the tasks row before replying (Phase 5 `--resume` key, latest wins)
- `GET /api/agents/status` serves the 04-03 frontend contract verbatim: newest agent session per task, `taskId/projectId/sessionId/status/exitCode/stopRequested`, `[]` never null, bash sessions excluded (D-48), exit 143 + `stopRequested:true` for the gray-dot discriminator
- `DELETE /api/tasks/{id}` now calls `StopAllForTask` before the row delete — research gap #1 (deleted task leaks a headless claude) closed with a behavioral test
- WS attach calls `ClearWaitingOnAttach` right after `Attach` (D-45 server-side authority); proven no-op for bash
- Migration 00003 adds `tasks.claude_session_id`; `--claude-bin` flag lets tests and unusual installs override binary resolution
- 16 new tests across api and ws packages; `go vet` and the full suite green

## Task Commits

Each task was committed atomically (TDD: test then feat):

1. **Task 1: Migration 00003, per-instance token, hook receiver endpoint**
   - RED: `52f3014` (test) — token gate, event dispatch, hooks-alive BEL gating, 404/200/400/204 edges
   - GREEN: `67e646a` (feat) — hooks.go, migration, main.go token + AgentConfig + --claude-bin
2. **Task 2: Agent spawn REST (kind, 409, persistence) + GET /api/agents/status**
   - RED: `55a8466` (test) — agent spawn contract, one-per-task, restart-overwrites, validation, status endpoint
   - GREEN: `129a746` (feat) — sessions.go kind handling, agents.go, AgentRoutes registration
3. **Task 3: Task-delete stops sessions (gap fix) + WS attach clears waiting (D-45)**
   - RED: `07c787e` (test) — delete leak proof, attach-clears-waiting, bash no-op
   - GREEN: `ab456f4` (feat) — StopAllForTask in delete, ClearWaitingOnAttach in ws handler, Routes signature threading

No REFACTOR commits — implementations came out clean on first pass.

## Files Created/Modified

- `internal/store/migrations/00003_agent_sessions.sql` — claude_session_id column (up/down)
- `internal/api/hooks.go` — HookRoutes + token-gated receive (constant-time, exited-ignore, event dispatch)
- `internal/api/agents.go` — AgentRoutes + status aggregation (newest-per-task, one project-id query, exact JSON tags)
- `internal/api/sessions.go` — kind discriminator, no-worktree gate for agents, one-agent 409, claude_session_id UPDATE
- `internal/api/tasks.go` / `routes.go` — taskHandlers gains mgr; StopAllForTask before DELETE
- `internal/ws/handler.go` — ClearWaitingOnAttach after Attach
- `cmd/kangent/main.go` — token generation, SetAgentConfig (BaseURL/token/--claude-bin), HookRoutes + AgentRoutes registration, mgr created before Routes
- `internal/api/hooks_test.go`, `agents_test.go`, additions to `sessions_test.go`/`tasks_test.go`, `internal/ws/handler_test.go` — fake-claude-stub-driven suites
- Test harness signature updates: `projects_test.go`, `worktrees_test.go`, `integration_test.go`

## Decisions Made

- `kind=agent` without `task_id` short-circuits to the same 409 `task has no worktree` body as a worktree-less task — the plan specified the gate; reusing the exact copy keeps one UI contract
- Hook receiver checks exited-session BEFORE decoding the body, so a malformed late curl against an exited session is still a benign 200 (ignore semantics dominate)
- The hooks-alive observable test uses a control session (hooks-dead BEL → waiting) before asserting the alive session ignores BEL — the negative assertion can never pass vacuously
- ws and api test packages each carry a local fake-claude writer instead of sharing one — Go test files cannot be imported across packages, and the two stubs intentionally differ (interactive BEL echo vs idle)

## Deviations from Plan

None - plan executed exactly as written.

## Authentication Gates

None encountered.

## Issues Encountered

None. Note for the verifier: `web/dist/index.html` was already dirty from the parallel 04-03 frontend workstream before this run and is untouched here (same note as the 04-01 SUMMARY).

## Known Stubs

None — no placeholder values, empty-wired data, or TODO markers in any file this plan touched. The fake-claude scripts are test fixtures by design (AgentConfig.ClaudeBin override), not production stubs.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- The Phase 4 backend surface is complete: 04-03's `useAgentStatuses` poll target (`GET /api/agents/status`) and the agent spawn/stop REST paths are live with the exact JSON contracts the UI spec consumes
- `tasks.claude_session_id` is populated on every agent spawn — Phase 5 resume needs only `claude --resume <uuid>` in the recorded worktree
- Remaining Phase 4 plans (04-04, 04-05) build frontend/verification on top; no backend blockers

---
*Phase: 04-claude-code-agent-sessions*
*Completed: 2026-06-10*

## Self-Check: PASSED

- All 6 key created files exist on disk
- All 6 task commits (52f3014, 67e646a, 55a8466, 129a746, 07c787e, ab456f4) present in git log
- `go build ./...`, `go vet ./...`, and `go test ./... -count=1` all pass
