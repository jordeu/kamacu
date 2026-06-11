---
phase: 04-claude-code-agent-sessions
plan: 05
subsystem: testing
tags: [go, integration-test, pty, hooks, fake-claude, human-verify]

# Dependency graph
requires:
  - phase: 04-claude-code-agent-sessions (plan 01)
    provides: AgentConfig{ClaudeBin} injection seam, hook overlay, stopRequested semantics
  - phase: 04-claude-code-agent-sessions (plan 02)
    provides: kind-discriminated spawn REST, /api/agents/status, hook token receiver, delete-stops-sessions, attach-clears-waiting
  - phase: 04-claude-code-agent-sessions (plan 03)
    provides: status dots on cards/sidebar driven by agent-statuses polling
  - phase: 04-claude-code-agent-sessions (plan 04)
    provides: Agent tab lifecycle UI (pre-start / running / exited)
  - phase: 03-worktree-isolation
    provides: integration-test fixture pattern (outer-t git repo, full mux, SQLite in TempDir)
provides:
  - End-to-end agent lifecycle integration test (8 sequential stages on one fixture)
  - internal/api/testdata/fake-claude CI stub (no auth, no API cost, exit 143 on TERM)
  - Human-verified Phase 4 sign-off against real claude v2.1.170 (all four roadmap success criteria)
  - Exited-agent UI revisions: dimmed terminal, code-free banner, "Reset session" action
affects: [05 recovery/resume, milestone UAT]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Agent lifecycle tested end-to-end via ClaudeBin stub injection — real claude never spawned in CI"
    - "Lifecycle stages as sequential subtests on one fixture task; status transitions polled with bounded deadlines"

key-files:
  created:
    - internal/api/agent_integration_test.go
    - internal/api/testdata/fake-claude
  modified:
    - internal/api/sessions.go
    - internal/api/sessions_test.go
    - web/src/components/task/AgentTab.tsx
    - web/src/components/terminal/TerminalPane.tsx
    - .planning/phases/04-claude-code-agent-sessions/04-UI-SPEC.md
    - .planning/phases/04-claude-code-agent-sessions/04-CONTEXT.md

key-decisions:
  - "Exited agent terminal renders dimmed (clear disabled affordance) instead of full-color frozen output"
  - "Exit codes hidden from the agent banner — copy is 'Agent session ended.'; codes stay in logs/API (D-41 revised)"
  - "Exited-agent primary action renamed 'Start again' → 'Reset session' (sets up Phase 5 'Resume session' as the future primary)"
  - "Phase 5 design note recorded: exited banner gains 'Resume session' primary via claude --resume + persisted claude_session_id; 'Reset session' becomes secondary (RCVR-02)"

patterns-established:
  - "fake-claude stub: bracketed-paste preamble + trap 'exit 143' TERM + sleep-and-wait — minimal mirror of verified real-claude PTY behavior"
  - "Status-transition assertions use short poll loops with explicit deadlines (≤2s attach-clear, ≤7s stop covering the 5s grace), never fixed sleeps"

requirements-completed: [TERM-01, STAT-01, STAT-02]

# Metrics
duration: 25min active (human-verify checkpoint spanned overnight)
completed: 2026-06-11
---

# Phase 4 Plan 05: Phase Verification Summary

**8-stage agent-lifecycle integration test against a fake-claude stub plus human-approved live verification of real claude v2.1.170, closing Phase 4 with one checkpoint feedback cycle (dimmed exited terminal, code-free banner, Reset session)**

## Performance

- **Duration:** ~25 min active execution (Tasks 1–2 on 2026-06-10 evening; checkpoint feedback + approval on 2026-06-11 morning)
- **Started:** 2026-06-10T19:22:47Z (after 04-04 completion)
- **Completed:** 2026-06-11T04:33:39Z (checkpoint approved)
- **Tasks:** 3 (2 auto + 1 human-verify checkpoint, with one feedback cycle)
- **Files modified:** 8

## Accomplishments

- `internal/api/agent_integration_test.go`: one deterministic lifecycle test, 8 sequential stages on a single fixture task — spawn (201, agentStatus working, parseable claude_session_id in DB), one-per-task 409, hook token security (no/wrong token → 401, correct → 204), Notification hook → waiting, WS attach clears waiting → idle (D-45), stop → exited(143, stopRequested=true) within the 5s grace, fresh restart with a new claude_session_id (newest-wins), and DELETE /api/tasks/{id} leaving zero running sessions. All three research gap fixes locked in.
- `internal/api/testdata/fake-claude`: 6-line executable bash stub (bracketed-paste preamble, `trap 'exit 143' TERM`, idle wait) — CI never touches the real claude binary, no auth or API cost.
- Full build gates green: `go vet ./...`, `go test ./... -count=1`, `go test ./internal/session/ -race -count=1`, `cd web && npm run build`, `make build`. Zero new dependencies (phase contract held).
- Human verification with real claude v2.1.170 APPROVED after one feedback round: TUI fidelity incl. slash commands and plan mode (SC-1), permission prompt → amber dot/border/sidebar chip within ~5s (SC-2/SC-3), attach clears waiting and turn-end goes idle without spurious flashes, clean detach/reattach with resize-while-detached repaint (SC-4), insert-description bracketed paste, stop → muted-gray exited semantics, cleanup-dialog stop integration, and a warning-free hook pipeline (SessionStart canary).

## Task Commits

1. **Task 1: End-to-end agent lifecycle integration test** - `aac114d` (test)
2. **Task 2: Full build gates** - verification-only, no commit (all gates green; no fixes required)
3. **Task 3 feedback cycle: exited-agent UI revisions** - `e7dda9e` (fix) + `7e45833` (docs: UI-SPEC/CONTEXT sync)
4. **Task 3: checkpoint:human-verify** - APPROVED by user against real claude v2.1.170

**Plan metadata:** (this commit — docs(04-05))

## Files Created/Modified

- `internal/api/agent_integration_test.go` - 8-stage lifecycle integration test on the Phase 3 fixture pattern
- `internal/api/testdata/fake-claude` - executable claude stand-in for CI (bracketed paste on, exit 143 on TERM)
- `internal/api/sessions.go`, `internal/api/sessions_test.go` - checkpoint feedback copy revision
- `web/src/components/task/AgentTab.tsx` - code-free `Agent session ended.` banner, `Reset session` label
- `web/src/components/terminal/TerminalPane.tsx` - dimmed terminal rendering for exited sessions
- `04-UI-SPEC.md`, `04-CONTEXT.md` - D-41 revision + Phase 5 Resume design note synced

## Decisions Made

- **Dimmed exited terminal:** frozen scrollback now renders visibly dimmed so the disabled state is unmistakable (checkpoint feedback).
- **Exit codes out of the banner:** agent banner reads `Agent session ended.` with no code; exit codes remain in logs and the sessions API (D-41 revised).
- **`Start again` → `Reset session`:** clearer that the action discards the conversation; deliberately leaves naming room for Phase 5's `Resume session`.
- **Phase 5 Resume design note (RCVR-02):** exited banner gains `Resume session` as primary (via `claude --resume` + the persisted `claude_session_id`), `Reset session` becomes secondary. Recorded in 04-CONTEXT.md; user asked about this during verification and it is confirmed Phase 5 scope.

## Deviations from Plan

### Checkpoint Feedback Revisions (human-verify cycle, not rule deviations)

**1. Exited-agent UI affordance revisions**
- **Found during:** Task 3 (live verification, user feedback round 1)
- **Issue:** Exited agent terminal looked active (no disabled affordance); banner exposed raw exit codes; `Start again` implied continuation
- **Fix:** Dimmed exited terminal rendering, code-free `Agent session ended.` banner, action renamed `Reset session`; UI-SPEC and CONTEXT (D-41 revision) synced
- **Files modified:** internal/api/sessions.go, internal/api/sessions_test.go, web/src/components/task/AgentTab.tsx, web/src/components/terminal/TerminalPane.tsx, 04-UI-SPEC.md, 04-CONTEXT.md
- **Verification:** User re-verified live and approved
- **Committed in:** `e7dda9e` (code), `7e45833` (docs sync)

Also: `web/dist/index.html` placeholder was overwritten by Task 2's `npm run build`; restored to the committed placeholder per the Phase 01 convention (real build output never committed). No commit needed.

---

**Total deviations:** 0 rule-based auto-fixes; 1 checkpoint feedback cycle (expected human-verify flow)
**Impact on plan:** Feedback was UI-polish scoped to the exited-agent state; no architectural or contract changes beyond the D-41 copy revision.

## Issues Encountered

None during execution.

## Open Questions Carried Forward

- **Plan-mode exit-plan approval → amber (research OQ1):** checklist item 2 asked the user to confirm that plan mode's exit-plan approval prompt also flips the dot amber. The user approved the checkpoint overall but did NOT explicitly answer this item. **Still open — follow up in Phase 5 / milestone UAT.** (Permission-prompt amber itself was confirmed working.)
- **Known accepted caveat (Pitfall 7):** a repo never opened in claude before shows the trust dialog in-terminal without an amber dot — documented v1 behavior, unchanged.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Phase 4 complete: TERM-01, STAT-01, STAT-02 demonstrably true (test + human verification). All 5 plans summarized.
- Phase 5 (recovery/resume) inputs ready: `claude_session_id` persisted per task and proven overwritten on fresh spawn; Resume design note recorded (RCVR-02: `Resume session` primary via `claude --resume`, `Reset session` secondary); `--resume` cwd-keyed semantics still MEDIUM confidence — verify against the installed version during Phase 5 planning (existing blocker stands).
- Carry the plan-mode amber open question into Phase 5/UAT.

---
*Phase: 04-claude-code-agent-sessions*
*Completed: 2026-06-11*

## Self-Check: PASSED

- internal/api/agent_integration_test.go — FOUND
- internal/api/testdata/fake-claude — FOUND (executable)
- Commits aac114d, e7dda9e, 7e45833 — FOUND
