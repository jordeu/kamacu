---
phase: 05-recovery-review
plan: 01
subsystem: api
tags: [claude-resume, session-recovery, reconciliation, agent-status, go, sqlite]

# Dependency graph
requires:
  - phase: 04-claude-code-agent-sessions
    provides: "tasks.claude_session_id persisted at agent spawn; /api/agents/status manager-derived contract; AgentConfig.ClaudeBin fake-claude stub mechanism; one-agent-per-task 409 gate"
provides:
  - "session.SpawnOpts.ResumeSessionID — agent spawn switches between `claude --resume <id>` (reuse) and `claude --session-id <fresh uuid>` (mint)"
  - "internal/api/resume.go transcriptExists glob helper — the single resumability rule (transcript existence == resumability)"
  - "/api/agents/status carries a `resumable` field on every entry + DB-derived post-restart exited entries (RCVR-01 reconciliation, zero schema)"
  - "POST /api/sessions {resume:true} variant — riding the existing one-per-task 409 gate, with honest 409 for unresumable tasks"
  - "testdata/fake-claude argv recording (FAKE_CLAUDE_ARGS_FILE) + FAKE_CLAUDE_RESUME_FAIL mode for CI"
affects: [frontend resume UI (AgentTab resumable banner + pre-start), 05-recovery-review remaining plans]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Resumability derived from transcript-glob existence (~/.claude/projects/*/<uuid>.jsonl), not from a DB status column — matches claude's own global lookup and self-heals D-56"
    - "RCVR-01 reconciliation by architecture: DB never records 'running', so a fresh manager + DB-derived /api/agents/status entries IS the whole reconciliation — no migration, no startup mutation pass"
    - "Injectable globRoot field on agentHandlers/sessionHandlers (route-registration sets defaultTranscriptGlobRoot; tests construct the handler directly with a temp dir)"

key-files:
  created:
    - internal/api/resume.go
    - internal/api/resume_test.go
  modified:
    - internal/session/manager.go
    - internal/session/agent_test.go
    - internal/api/agents.go
    - internal/api/agents_test.go
    - internal/api/sessions.go
    - internal/api/sessions_test.go
    - internal/api/testdata/fake-claude

key-decisions:
  - "Resume rides the existing one-agent-per-task 409 gate, validated AFTER it (D-67 never-two-PTYs); the 'no session to resume' 409 is the server-side honest-failure guard against a stale client snapshot"
  - "claude_session_id is never cleared on resume failure — the transcript glob already makes resumable false, and clearing would break the 'crashed but still resumable' case"
  - "globRoot is injected for tests; AgentRoutes/SessionRoutes signatures stay unchanged (no main.go edit needed this plan)"

patterns-established:
  - "Transcript-glob resumability: transcriptExists(globRoot, id) with a uuid.Parse glob-injection guard, used by both the status handler and resume-spawn validation"
  - "DB-derived status entries emitted ONLY when resumable — non-resumable past sessions get no dot (D-57 only constrains resumable tasks)"

requirements-completed: [RCVR-01, RCVR-02]

# Metrics
duration: 17min
completed: 2026-06-11
---

# Phase 5 Plan 01: Backend Recovery (Resume Spawn + Post-Restart Status Carrier) Summary

**`claude --resume <persisted uuid>` as a body-flag variant of the existing agent spawn endpoint, plus a transcript-glob `resumable` flag and DB-derived post-restart exited entries on `/api/agents/status` — RCVR-01 reconciled by architecture with zero schema change.**

## Performance

- **Duration:** 17 min
- **Started:** 2026-06-11T09:08:36Z
- **Completed:** 2026-06-11T09:26:27Z
- **Tasks:** 3 (all TDD: RED -> GREEN, no refactor needed)
- **Files modified:** 7 (1 created: internal/api/resume.go; 1 created test: internal/api/resume_test.go)

## Accomplishments

- **Resume spawn engine (RCVR-02):** `session.SpawnOpts.ResumeSessionID` switches the agent branch between `--resume <stored id>` (reuse — claude keeps the id, no `--fork-session`) and `--session-id <fresh uuid>` (mint, D-55 newest-wins). The `--settings` hook overlay applies on both paths, so the status machine is unchanged. Resume on a non-agent kind errors before any PTY work.
- **Resumability rule (Pattern 1):** new `internal/api/resume.go` with `transcriptExists(globRoot, id)` — a glob of `<globRoot>/*/<uuid>.jsonl` guarded by `uuid.Parse` (no raw input ever reaches the glob). Transcript existence == resumability, matching claude's own global lookup; this self-heals D-56 (a failed resume means the transcript is gone → next poll reports resumable:false with zero state mutation).
- **Post-restart status carrier (RCVR-01):** `/api/agents/status` entries gain a `resumable` field; a DB-derived pass appends `{sessionId:"", status:"exited", exitCode:null, stopRequested:false, resumable:true}` for tasks with a persisted id + worktree + transcript but no manager entry. The DB never records "running" (verified across migrations 00001–00003), so an empty manager + these derived entries IS the whole reconciliation — no migration 00004, no startup mutation pass.
- **Resume REST variant (RCVR-02):** `POST /api/sessions {task_id, kind:"agent", resume:true}` reads `claude_session_id` fresh in-handler (single source of truth, Pitfall 6), rides the existing one-agent-per-task 409 gate (D-67), and returns 409 "no session to resume" for a NULL id or missing transcript. The post-spawn `UPDATE` rewrites the same id on resume (one code path).
- **CI stub extension:** `testdata/fake-claude` now records argv to `FAKE_CLAUDE_ARGS_FILE` and supports `FAKE_CLAUDE_RESUME_FAIL` (the verified v2.1.173 missing-transcript exit-1 path), while preserving the 04-05 contract (bracketed-paste enable + SIGTERM → exit 143).

## Task Commits

Each task was TDD (failing test committed first, then implementation):

1. **Task 1: SpawnOpts.ResumeSessionID flag switch**
   - `9852b21` test(05-01): add failing tests for SpawnOpts.ResumeSessionID
   - `1835ee7` feat(05-01): SpawnOpts.ResumeSessionID --resume/--session-id flag switch
2. **Task 2: transcriptExists helper + status resumable carrier**
   - `48f3f78` test(05-01): add failing tests for transcriptExists + status resumable carrier
   - `911e33b` feat(05-01): transcriptExists helper + /api/agents/status resumable carrier
3. **Task 3: Resume REST variant + fake-claude extension**
   - `7cfafef` test(05-01): add failing tests for resume REST variant on POST /api/sessions
   - `45a38f1` feat(05-01): resume REST variant on POST /api/sessions + fake-claude argv/resume-fail

_Note: 05-02 (diff service) commits are interleaved in `git log` — it executed in the same parallel wave._

## Files Created/Modified

- `internal/api/resume.go` (created) — `transcriptExists` glob helper + `defaultTranscriptGlobRoot`
- `internal/api/resume_test.go` (created) — `transcriptExists` unit tests (hit/miss/empty/non-uuid)
- `internal/session/manager.go` — `SpawnOpts.ResumeSessionID`, agent-only guard, `--resume`/`--session-id` flag switch
- `internal/session/agent_test.go` — resume/fresh argv assertions + bash-resume rejection
- `internal/api/agents.go` — `globRoot` field, `Resumable` JSON key, extended IN-query, DB-derived post-restart pass
- `internal/api/agents_test.go` — post-restart resumable/no-transcript, running-not-resumable, exited-resumable
- `internal/api/sessions.go` — `globRoot` field, `Resume` body flag, kind validation, fresh `claude_session_id` read, resume validation after the gate
- `internal/api/sessions_test.go` — resume argv/persistence, no-session 409s, gate precedence, kind 400, fresh-spawn regression
- `internal/api/testdata/fake-claude` — argv recording + resume-fail mode (04-05 contract preserved)

## Decisions Made

- **Resume validation comes AFTER the one-per-task 409 gate** so a running agent always wins the race (D-67 never-two-PTYs); the "no session to resume" 409 is purely a server-side guard against a stale client snapshot.
- **`claude_session_id` is never cleared** — the transcript glob makes `resumable` false on its own, and clearing would break the "crashed but still resumable" case.
- **`globRoot` is injected for tests** (handlers constructed directly); production route registration sets `defaultTranscriptGlobRoot()`, so `AgentRoutes`/`SessionRoutes` signatures stay unchanged and `cmd/kangent/main.go` needs no edit.

## Deviations from Plan

None - plan executed exactly as written.

The full-module `go test ./...` showed a transient `FAIL` in `kangent/internal/api` only when two `go test` invocations of the api package ran concurrently (PTY/process contention under doubled parallel load). The api suite passes deterministically in isolation, and `go test ./... -p 1` (serial) is fully green. This is a test-runner concurrency artifact, not a code defect.

## Issues Encountered

None - all three tasks went RED -> GREEN cleanly with no refactor step required.

## User Setup Required

None - no external service configuration required. (Verification of live `claude --resume` behavior against the real binary is a human-verify item for the phase UAT, not a setup task — CI uses the fake-claude stub exclusively.)

## Next Phase Readiness

- **RCVR-01 backend complete:** post-restart status output is correct (DB-derived resumable entries, `exitCode: null`) with zero schema and zero startup mutation. The frontend `dotMeta` null-exit-code → gray branch (Pitfall 3) and `AgentStatusEntry.resumable` consumption remain for the UI plan.
- **RCVR-02 backend complete:** the resume spawn variant and honest 409s are wired and tested. The frontend `useResumeAgent` hook + AgentTab resumable banner/pre-start (D-54) remain for the UI plan.
- No blockers. Open Question carried for UAT: live verification of `claude --resume` + plan-mode amber dot against the installed binary (research OQ1/OQ2).

---
*Phase: 05-recovery-review*
*Completed: 2026-06-11*

## Self-Check: PASSED

- Created files exist: internal/api/resume.go, internal/api/resume_test.go, 05-01-SUMMARY.md
- All 6 task commits present: 9852b21, 1835ee7, 48f3f78, 911e33b, 7cfafef, 45a38f1
- All plan + plan-level verifications green: go build, go vet, go test ./... (-p 1), session -race
