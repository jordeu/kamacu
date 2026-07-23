---
phase: 08-sessions-terminal-read-access
plan: 01
subsystem: api
tags: [http, sessions, pty, streaming, base64, read-only, loopback]

# Dependency graph
requires:
  - phase: 06-bridge-foundations
    provides: in-memory session engine (session.Session Snapshot/Attach/Detach/Done/Info) consumed by the new read handlers
provides:
  - GET /api/sessions/{id} (MCPSESS-02) — session.Info + D-10 task->project->agent JOIN
  - GET /api/sessions/{id}/output?bytes=N (MCPSESS-03) — D-08 base64 ring-snapshot envelope
  - GET /api/sessions/{id}/subscribe (MCPSESS-04) — bounded raw PTY octet-stream with Detach-on-cancel
  - GET /api/sessions?project_id=N (MCPSESS-01) — project-scoped session filter
affects: [08-02-mcp-session-tools, 08-03-mcp-subscribe-streaming]

# Tech tracking
tech-stack:
  added: []  # encoding/base64 is stdlib — no new dependency
  patterns:
    - "Type-level read-only contract (D-14): handlers consume ONLY session.Info/Snapshot/Attach/Detach/Done; scoped grep gate proves zero references to the PTY-write primitive and zero references to WS FrameData"
    - "Plain HTTP chunked octet-stream for terminal replay — never the WS frame protocol — keeping the read path off the WS upgrade entirely"
    - "Batch tasks->projects->agents JOIN in ONE query (mirrors agents.go:99); never N round-trips per session (D-11)"
    - "Request-scoped handler IS the reader (no goroutine launched by the handler outlives the request — milestone rule)"
    - "defer Detach on every return path (SC3) — Detach never touches the PTY (session.go:356)"

key-files:
  created: []
  modified:
    - internal/api/sessions.go
    - internal/api/sessions_test.go

key-decisions:
  - "sessionDetail embeds session.Info so every existing JSON tag flows through unchanged; taskTitle/projectName/agentName ride as additional top-level fields (omitempty for dev sessions)"
  - "get_session_output clamps bytes to [1, 512KiB]; base64 of 512KiB (~683KiB) fits the bridge's 1MiB maxBodyBytes ceiling (D-06 / T-08-02)"
  - "subscribe drains the Attach replay as the channel's first message (D-02) when include_history is absent — one <-q read atomically removes the replay so only post-attach output streams"
  - "subscribe streams application/octet-stream (not text) — PTY output is arbitrary bytes; a text frame would corrupt split multi-byte sequences"
  - "subscribe duration_seconds is clamped to [1s, 300s] server-side (T-08-03) — the SDK ctx cancel on the bridge side is the real backstop, but the server enforces its own bound regardless of bridge behavior"
  - "joinSessionContext degrades to empty fields on DB error or a missing task row (mirrors agents.go's `continue` posture) — the read path stays usable even if the task was deleted under a still-tracked session"

patterns-established:
  - "Read-only endpoint pattern: scoped grep gate (WriteInput==0, FrameData==0) is the type-level proof that a handler never reaches the PTY-write primitive"
  - "Batch JOIN helper (joinSessionContext): one query for N session infos, not N queries — reusable shape for any future endpoint that needs the task/project/agent context"
  - "Request-scoped streaming: the handler body itself is the reader goroutine (no `go func`); defer Detach guarantees attach cleanup on every return path"

requirements-completed: [MCPSESS-01, MCPSESS-02, MCPSESS-03, MCPSESS-04]

# Coverage metadata (#1602)
coverage:
  - id: D1
    description: "GET /api/sessions/{id} returns session.Info + taskTitle/projectName/agentName for live sessions; 404 for unknown ids"
    requirement: MCPSESS-02
    verification:
      - kind: unit
        ref: internal/api/sessions_test.go#TestGetSession_Happy
        status: pass
      - kind: unit
        ref: internal/api/sessions_test.go#TestGetSession_UnknownID_404
        status: pass
      - kind: unit
        ref: internal/api/sessions_test.go#TestGetSession_ExitedStillWorks
        status: pass
    human_judgment: false
  - id: D2
    description: "GET /api/sessions/{id}/output?bytes=N returns D-08 base64 envelope; bytes defaults to 4096, clamps to 512KiB with clamped=true"
    requirement: MCPSESS-03
    verification:
      - kind: unit
        ref: internal/api/sessions_test.go#TestGetSessionOutput_Happy
        status: pass
      - kind: unit
        ref: internal/api/sessions_test.go#TestGetSessionOutput_Clamp
        status: pass
      - kind: unit
        ref: internal/api/sessions_test.go#TestGetSessionOutput_UnknownID_404
        status: pass
      - kind: unit
        ref: internal/api/sessions_test.go#TestGetSessionOutput_BadBytes_400
        status: pass
    human_judgment: false
  - id: D3
    description: "GET /api/sessions?project_id=N filters to sessions whose task is in the project; empty project returns JSON []"
    requirement: MCPSESS-01
    verification:
      - kind: unit
        ref: internal/api/sessions_test.go#TestListSessions_ProjectID
        status: pass
      - kind: unit
        ref: internal/api/sessions_test.go#TestListSessions_JoinFields
        status: pass
    human_judgment: false
  - id: D4
    description: "GET /api/sessions/{id}/subscribe streams raw PTY octets for a bounded duration; drains replay by default; Detaches on cancel/exit/cap (SC3)"
    requirement: MCPSESS-04
    verification:
      - kind: unit
        ref: internal/api/sessions_test.go#TestSubscribe_Happy_ReturnsOctets
        status: pass
      - kind: unit
        ref: internal/api/sessions_test.go#TestSubscribe_UnknownID_404
        status: pass
      - kind: unit
        ref: internal/api/sessions_test.go#TestSubscribe_ClientCancel_DetachesPromptly
        status: pass
      - kind: unit
        ref: internal/api/sessions_test.go#TestSubscribe_DurationCap
        status: pass
      - kind: unit
        ref: internal/api/sessions_test.go#TestSubscribe_DrainsReplayByDefault
        status: pass
    human_judgment: false
  - id: D5
    description: "Read-only contract (D-14): handlers consume ONLY Info/Snapshot/Attach/Detach/Done — never the PTY-write primitive, never the WS frame protocol"
    requirement: MCPSESS-04
    verification:
      - kind: automated_ui
        ref: "grep -c 'WriteInput' internal/api/sessions.go == 0; grep -c 'FrameData' internal/api/sessions.go == 0"
        status: pass
    human_judgment: false

# Metrics
duration: 21min
completed: 2026-07-23
status: complete
---

# Phase 08 Plan 01: Kamacu Session Endpoints Summary

**Read-only HTTP endpoints (get_session / get_session_output / subscribe / list project_id filter) backing the four MCP session tools, with type-level read-only enforcement and no new long-lived goroutines**

## Performance

- **Duration:** 21 min
- **Started:** 2026-07-23T05:39:23Z
- **Completed:** 2026-07-23T06:00:29Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- Three new read-only GET handlers under /api/sessions (get_session, get_session_output, subscribe_session_output) plus a ?project_id=N filter on the existing list — the server-side half of MCPSESS-01..04
- D-10 batch JOIN (taskTitle/projectName/agentName) wired into the list + get_session responses via a single batch query (mirrors agents.go:99) — never N round-trips per session
- D-08 base64 envelope for get_session_output with server-side clamp to 512KiB + clamped flag (T-08-02)
- subscribe_session_output streams raw PTY octets as application/octet-stream with default 30s / cap 300s duration (MCPSESS-04), drains the ring replay by default (D-02), and Detaches on every return path (SC3 / T-08-05)
- Type-level read-only contract enforced (D-14): scoped grep gate proves zero references to the PTY-write primitive and zero references to WS FrameData — the read paths can never reach the PTY writer

## Task Commits

Each task was committed atomically:

1. **Task 1: get_session + get_session_output + list project_id filter** — `ba7f16a` (feat)
2. **Task 2: subscribe_session_output streaming endpoint** — `66e4063` (feat)

## Files Created/Modified
- `internal/api/sessions.go` — Added sessionDetail/sessionOutputEnvelope types, defaultOutputBytes/maxOutputBytes/defaultSubscribeDuration/maxSubscribeDuration constants, joinSessionContext batch JOIN helper, getSession/getSessionOutput/subscribeSessionOutput handlers, project_id branch inside list, three new route registrations
- `internal/api/sessions_test.go` — Added TestGetSession_Happy/UnknownID_404/ExitedStillWorks, TestGetSessionOutput_Happy/Clamp/UnknownID_404/BadBytes_400, TestListSessions_ProjectID/JoinFields, TestSubscribe_Happy_ReturnsOctets/UnknownID_404/ClientCancel_DetachesPromptly/DurationCap/DrainsReplayByDefault, plus getJSON/waitForOutput test helpers

## Decisions Made
- **sessionDetail embeds session.Info** so every existing JSON tag (id/status/taskId/label/etc.) flows through unchanged; the new taskTitle/projectName/agentName ride as additional top-level fields. Dev sessions (TaskID==0) keep empty JOIN fields omitted on the wire via `omitempty` — no wire-shape change for the SPA's existing dev-session rows.
- **get_session_output clamps bytes to [1, 512KiB]** — a `bytes=0` request is floored at 1 (a 0-byte slice is never useful), and `bytes=999999` is clamped to 512KiB with `clamped=true`. base64 of 512KiB (~683KiB) fits the bridge's 1MiB maxBodyBytes ceiling (D-06).
- **subscribe drains the Attach replay as the channel's first message** (D-02) when `include_history` is absent — one `<-q` read atomically removes the replay queued under the same lock that registered the queue (session.go:346), so only post-attach output streams. The drain is guarded with `r.Context().Done()` so a client that disconnected during replay-drain still returns promptly.
- **subscribe streams application/octet-stream** (not text) — PTY output is arbitrary bytes; a text frame would corrupt split multi-byte sequences. Plain HTTP chunked output, never the WS frame protocol (D-14).
- **subscribe duration_seconds is clamped to [1s, 300s] server-side** (T-08-03) — the SDK ctx cancel on the bridge side is the real backstop, but the server enforces its own bound regardless of bridge behavior. A parsed `<1` value clamps to 1s.
- **joinSessionContext degrades to empty fields on DB error or a missing task row** — mirrors agents.go's `continue` posture. The read path stays usable even if the task was deleted under a still-tracked session (a session outlived its task row).

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required. The handlers reuse the existing in-memory session engine and the existing SQLite DB; no new env vars, no new packages (encoding/base64 is stdlib).

## Next Phase Readiness
- Plan 02 (08-02-mcp-session-tools) can wire the bridge's MCP tools to these endpoints: get_session, get_session_output, list_sessions(project_id).
- Plan 03 (08-03-mcp-subscribe-streaming) can wire the bridge's subscribe tool to the streaming endpoint.
- Read-only contract (D-14) holds at the type level: scoped grep gate is green, no PTY-write primitive reachable from these handlers.

---
*Phase: 08-sessions-terminal-read-access*
*Completed: 2026-07-23*

## Self-Check: PASSED

- [x] `internal/api/sessions.go` exists on disk (FOUND)
- [x] `internal/api/sessions_test.go` exists on disk (FOUND)
- [x] Commit `ba7f16a` exists in git log (FOUND)
- [x] Commit `66e4063` exists in git log (FOUND)
- [x] `go build ./internal/api/` succeeds
- [x] `go vet ./internal/api/` clean
- [x] `go test ./internal/api/ -count=1` passes (184s)
- [x] `grep -c 'WriteInput' internal/api/sessions.go` == 0 (0)
- [x] `grep -c 'FrameData' internal/api/sessions.go` == 0 (0)
