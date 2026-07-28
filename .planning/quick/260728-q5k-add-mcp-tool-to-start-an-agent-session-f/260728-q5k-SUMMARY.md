---
quick_id: 260728-q5k
slug: add-mcp-tool-to-start-an-agent-session-f
status: complete
completed: "2026-07-28"
commits: 2
files_modified:
  - internal/mcp/sessions.go
  - internal/mcp/sessions_test.go
tests_added: 3
---

# Quick Task 260728-q5k: Summary

## What shipped

Added the `start_task_agent` MCP tool — a thin HTTP bridge that lets an agent
running inside a Kamacu task PTY spawn (or resume) the agent session for
another task. The tool POSTs `{"kind":"agent","task_id":N,"resume":bool}` to
the existing `POST /api/sessions` endpoint and passes Kamacu's 201 session-row
JSON back verbatim via `bridge.call`.

This reverses the documented "no auto-start" rule for the **agent-delegate
surface only**. The browser's explicit Start button, the task-creation flow,
and PROJECT.md's "Out of Scope" entry are all untouched. The agent IS the
user's delegate at the same trust level as `delete_task` and `open_review`.

## Implementation

- `internal/mcp/sessions.go`: added `"bytes"` to imports; added a 5th
  `s.AddTool` call inside `registerSessionTools` (for `start_task_agent`); added
  the `startTaskAgent` method at the bottom. The handler closure is wrapped in
  `withRecover("start_task_agent", ...)` to match the file's convention.
  InputSchema uses the locked `map[string]any` form (NOT the `mcp.With*`
  helpers). `server.go` was NOT modified — `registerSessionTools` is already
  wired at server.go:88.
- `internal/mcp/sessions_test.go`: added 3 tests modeled on
  `createTask` (POST-with-body happy path) and `openReview` (409 dedup wrap):
  - `TestBridge_StartTaskAgent_Happy_PostsAgentBody` — 201 passthrough, body
    has `kind=agent` + `task_id=7` + `resume=false`.
  - `TestBridge_StartTaskAgent_ResumeTrue_SendsResumeBody` — resume flag
    routing (resume:true round-trips into the body).
  - `TestBridge_StartTaskAgent_AlreadyRunning_Kamacu409` — the dedup error
    path: Kamacu's 409 surfaces as a D-05 wrapped error containing both
    "HTTP 409" and the Kamacu message ("agent session already running").

## Verification

- `go build ./...` — passes.
- `go vet ./internal/mcp/...` — passes.
- `go test ./internal/mcp/... -count=1` — all package tests green
  (60+ existing + 3 new).

## Scope guard

No modifications to `internal/api/*`, `web/src/*`, `cmd/*`,
`.planning/PROJECT.md`, or `.planning/ROADMAP.md`. The reversal is MCP-only.
