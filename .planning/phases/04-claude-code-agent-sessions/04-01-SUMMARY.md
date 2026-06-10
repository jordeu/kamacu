---
phase: 04-claude-code-agent-sessions
plan: 01
subsystem: session-engine
tags: [go, pty, claude-cli, hooks, state-machine, tdd]

# Dependency graph
requires:
  - phase: 02-terminal-engine
    provides: internal/session Manager/Session (PTY lifecycle, ring replay, Stop teardown)
  - phase: 03-worktree-isolation
    provides: per-task worktree cwds and ListByTask/StopAllForTask task scoping
provides:
  - Kind discriminator (bash/agent) on SpawnOpts; zero value stays bash byte-for-byte
  - Agent spawn of the claude CLI with --session-id <uuid> + inline --settings hook overlay, inherit-all env (D-51/D-52/D-53)
  - buildOverlayJSON producing the verified v2.1.170 overlay shape + SessionStart hooks-alive canary
  - D-47 agent status state machine (working/idle/waiting/exited) with sticky waiting, 2s settle window, 10s quiet threshold
  - SetWaiting/SetIdle/MarkHooksAlive/ClearWaitingOnAttach methods for the 04-02 hook receiver and WS attach path
  - stopRequested flag distinguishing server Stop (exit 143 gray) from unexpected death (red)
  - ClaudeSessionID accessor (Phase 5 --resume key)
affects: [04-02 REST/WS layer, 04-03 board status surfaces, 05 recovery/resume]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Lazy status computation in Info() under s.mu — no ticker goroutine; the 5s poll is the only consumer"
    - "fake-claude stub via AgentConfig.ClaudeBin override — agent tests never touch the real binary"
    - "Test helpers call the same locked mutations the pump performs (noteAgentOutputLocked) instead of flaky PTY byte injection"

key-files:
  created:
    - internal/session/agent.go
    - internal/session/agent_test.go
  modified:
    - internal/session/manager.go
    - internal/session/session.go
    - internal/session/session_test.go

key-decisions:
  - "Kangent session id generated up front in Spawn so the overlay hook URL embeds it before the process starts"
  - "SetAgentConfig stores config on Manager under m.mu; Spawn reads a copy — no per-spawn plumbing"
  - "Agent-status pump logic factored into noteAgentOutputLocked so tests drive the real code path via an in-package _test helper"
  - "stopRequested set inside stopOnce.Do before SIGTERM — natural exits can never observe it true"

patterns-established:
  - "Agent env: os.Environ() + TERM/COLORTERM pin is agent-only; bash keeps the Phase 2 minimal explicit env"
  - "Settle window anchored on stopHookAt; tests move the anchor instead of sleeping through real windows"

requirements-completed: [TERM-01, STAT-02]

# Metrics
duration: 11min
completed: 2026-06-10
---

# Phase 4 Plan 01: Agent Session Kind & Status Core Summary

**Agent session kind spawning the real claude CLI (--session-id + inline --settings hook overlay, inherit-all env) plus the D-47 working/idle/waiting state machine with settle-gated activity and hooks-dead BEL fallback, all TDD-driven against a fake-claude stub**

## Performance

- **Duration:** 11 min
- **Started:** 2026-06-10T18:36:53Z
- **Completed:** 2026-06-10T18:48:16Z
- **Tasks:** 2 (both TDD: RED+GREEN commits each)
- **Files modified:** 5

## Accomplishments

- `Manager.Spawn` with `Kind: KindAgent` launches `claude --session-id <uuid> --settings <inline JSON>` in the task worktree with inherit-all env + TERM/COLORTERM; bash sessions are byte-for-byte unaffected (all Phase 2/3 tests untouched and green)
- `buildOverlayJSON` marshals the empirically verified v2.1.170 overlay (Notification matcher `permission_prompt|elicitation_dialog`, Stop and SessionStart without matcher keys, async curl hooks with timeout 5, `preferredNotifChannel: terminal_bell`) via encoding/json — nothing ever written to disk (D-53)
- Full D-47 state machine on Session: hooks drive precise transitions (SetWaiting/SetIdle/MarkHooksAlive), waiting is sticky against output, the 2s settle window kills the spurious green-after-every-turn (Pitfall 1), the OSC-aware BEL fallback fires only in hooks-dead mode (Pitfall 2), and 10s quiet computes idle lazily — no ticker
- `stopRequested` recorded inside `stopOnce` so 04-03's dot logic can render server-initiated exit 143 gray instead of red
- 12 new tests incl. full argv-contract verification through a fake-claude PTY stub; suite passes `-race`

## Task Commits

Each task was committed atomically (TDD: test then feat):

1. **Task 1: Agent kind, claude spawn, and overlay builder**
   - RED: `a3e40e0` (test) — overlay shape, argv contract, cwd requirement, bash default
   - GREEN: `aedc70f` (feat) — agent.go, Spawn branch, Info.Kind, ClaudeSessionID
2. **Task 2: Agent status state machine on Session**
   - RED: `4e040e9` (test) — D-47 transitions, settle window, BEL gating, stopRequested
   - GREEN: `df74bb2` (feat) — state machine fields/methods, pump hook, Info extensions

No REFACTOR commits — implementations came out clean on first pass.

## Files Created/Modified

- `internal/session/agent.go` — Kind type, AgentConfig, buildOverlayJSON (verified v2.1.170 shape + SessionStart canary), OSC-aware belScanner
- `internal/session/manager.go` — SpawnOpts.Kind, SetAgentConfig, agent spawn branch (claude binary resolution, inline overlay, inherit-all env, "Agent" label, up-front session id)
- `internal/session/session.go` — agent status fields under s.mu, SetWaiting/SetIdle/MarkHooksAlive/ClearWaitingOnAttach/ClaudeSessionID, noteAgentOutputLocked in the pump, agentStatusLocked lazy computation, Info gains Kind/AgentStatus/StopRequested, stopRequested in Stop
- `internal/session/agent_test.go` — fake-claude stub writer, overlay shape pinning, argv/identity verification, cwd gate, bash default
- `internal/session/session_test.go` — state machine suite (sticky waiting, settle window, BEL gating, stop-vs-natural exit, quiet threshold) + locked test helpers

## Decisions Made

- Kangent session id generated at the top of Spawn (before command construction) so the overlay hook URL embeds it — the plan's restructure note, applied minimally
- Pump's agent logic factored into `noteAgentOutputLocked` called under the existing critical section; tests inject chunks through the same method (plan's recommended _test-helper alternative to flaky PTY byte injection)
- Settle-window and quiet-threshold tests manipulate `stopHookAt`/`lastActivity` via _test-only setters instead of sleeping — deterministic and fast
- Comment wording in agent.go avoids the literal string `idle_prompt` so the acceptance grep (which guards the matcher) stays meaningful

## Deviations from Plan

None - plan executed exactly as written. (Task 1 placed the `kind`/`claudeSessionID`/`lastActivity` Session fields and the `ClaudeSessionID` accessor in session.go — required by Task 1's own behavior tests and explicitly listed in the plan's files_modified; Task 2 built on them as specced.)

## Issues Encountered

None. Note for the verifier: a parallel executor (plan 04-03) committed frontend work to the same branch during this run; `web/dist/index.html` was left dirty by that workstream and is untouched here.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- `internal/session` now exposes everything plan 04-02 needs as plain method calls: `Spawn(Kind: KindAgent)`, `SetAgentConfig`, `SetWaiting`/`SetIdle`/`MarkHooksAlive` for the hook receiver, `ClearWaitingOnAttach` for the WS attach path, and `Info().AgentStatus`/`StopRequested` for the status endpoint
- `ClaudeSessionID()` ready for the 04-02 tasks-table persistence (Phase 5 --resume prep)
- Zero new module dependencies; go.mod untouched

---
*Phase: 04-claude-code-agent-sessions*
*Completed: 2026-06-10*

## Self-Check: PASSED

- All 5 key files exist on disk
- All 4 task commits (a3e40e0, aedc70f, 4e040e9, df74bb2) present in git log
- `go build ./...`, `go test ./... -count=1`, `go vet`, and `-race` all pass
