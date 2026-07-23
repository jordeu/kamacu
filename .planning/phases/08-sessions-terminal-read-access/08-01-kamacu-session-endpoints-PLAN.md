---
phase: 08-sessions-terminal-read-access
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/api/sessions.go
  - internal/api/sessions_test.go
autonomous: true
requirements:
  - MCPSESS-01
  - MCPSESS-02
  - MCPSESS-03
  - MCPSESS-04
must_haves:
  truths:
    - "GET /api/sessions?project_id=N returns only sessions whose task belongs to that project, each carrying taskTitle/projectName/agentName (D-10 JOIN)"
    - "GET /api/sessions/{id} returns session.Info plus taskTitle/projectName/agentName for a live OR exited in-memory session; unknown id returns 404 (D-12, D-13)"
    - "GET /api/sessions/{id}/output?bytes=N returns a D-08 JSON envelope {encoding:base64, output, bytes, clamped}; bytes defaults to 4096 and is clamped to ~512 KiB (D-05/D-06)"
    - "GET /api/sessions/{id}/subscribe streams raw PTY octets for a bounded duration (default 30s, cap 300s), drains the ring replay when include_history is absent, and calls Session.Detach(connID) on every return path including client disconnect (D-01..D-04, SC3)"
  artifacts:
    - internal/api/sessions.go extended with getSession, getSessionOutput, subscribeSessionOutput handlers + sessionDetail/sessionOutputEnvelope types + output/subscribe constants
    - 3 new routes registered inside SessionRoutes (GET /api/sessions/{id}, GET /api/sessions/{id}/output, GET /api/sessions/{id}/subscribe)
    - internal/api/sessions_test.go extended with happy/404/clamp/project-filter/streaming-detach tests
  key_links:
    - "getSession/getSessionOutput/subscribeSessionOutput consume ONLY session.Snapshot/Attach/Detach/Done/Info — never the PTY-write primitive (D-14)"
    - "subscribeSessionOutput mirrors internal/ws/handler.go's attach->stream->detach loop minus the readLoop (read-only half)"
  prohibitions:
    - "The new handlers never call the public PTY-write method on *session.Session (session.go:368)"
    - "The subscribe endpoint uses plain HTTP chunked streaming — it never speaks the WS frame protocol and never sends a FrameData PTY-input frame (D-14)"
---

<objective>
Add the read-only Kamacu HTTP endpoints that back Phase 08's four MCP session tools. Three new GET handlers under /api/sessions (get_session, get_session_output, subscribe_session_output) plus a ?project_id=N filter and a task->project->agent JOIN on the existing list. This is the server-side half of MCPSESS-01..04: the MCP subcommand (separate process) reaches the in-memory session engine only over these endpoints.

Purpose: Every primitive the tools need already exists on *session.Session (Snapshot/Attach/Detach/Done/Info). This plan wires new READ-ONLY HTTP paths through them — no DB schema change, no migration, no new long-lived goroutine (the subscribe reader is request-scoped).
Output: extended internal/api/sessions.go + internal/api/sessions_test.go.
</objective>

<execution_context>
@/home/jordi/.config/opencode/gsd-core/workflows/execute-plan.md
@/home/jordi/.config/opencode/gsd-core/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/phases/08-sessions-terminal-read-access/08-CONTEXT.md
@.planning/phases/08-sessions-terminal-read-access/08-RESEARCH.md
</context>

<tasks>

<task type="auto">
  <name>Task 1: list ?project_id JOIN + get_session + get_session_output endpoints (D-09, D-10, D-11, D-12, D-13, MCPSESS-01/02/03)</name>
  <files>internal/api/sessions.go, internal/api/sessions_test.go</files>
  <read_first>
    - internal/api/sessions.go (the file being modified — current list/reconcileTmux/SessionRoutes/sessionHandlers)
    - internal/session/session.go lines 199-224 (Info struct + JSON tags), 383-391 (Snapshot seam), 342-363 (Attach/Detach)
    - internal/api/agents.go lines 44-152 (the canonical tasks->projects->agents JOIN at line 99 — copy this shape for D-10)
    - internal/api/respond.go (writeJSON/writeError helpers)
    - internal/api/sessions_test.go lines 1-90 (newSessionServer/newTaskSessionServer harness — real Manager + temp DB; reuse for new tests)
  </read_first>
  <behavior>
    - GET /api/sessions?project_id=N returns only sessions whose info.TaskID is in the project's task set (one SQL query SELECT id FROM tasks WHERE project_id=? + in-memory filter of mgr.List() — never N round-trips per D-11/Pitfall 4)
    - GET /api/sessions (no project_id) behavior unchanged except every entry now also carries taskTitle/projectName/agentName
    - GET /api/sessions/{id} returns 404 for unknown/gone ids; 200 with session.Info + JOIN fields for live AND exited in-memory sessions (D-12)
    - GET /api/sessions/{id}/output?bytes=N clamps bytes to [1, 512KiB]; sets clamped=true when the requested value exceeded the cap; returns base64 of the LAST N ring bytes
  </behavior>
  <action>
    Define a response type `sessionDetail` that embeds `session.Info` (so all existing JSON tags flow through) and adds `TaskTitle string json:"taskTitle,omitempty"`, `ProjectName string json:"projectName,omitempty"`, `AgentName string json:"agentName,omitempty"`. Define `sessionOutputEnvelope` with fields `Encoding string json:"encoding"`, `Output string json:"output"`, `Bytes int json:"bytes"`, `Clamped bool json:"clamped"` (the D-08 snapshot shape). Add constants `defaultOutputBytes = 4096` and `maxOutputBytes = 512 * 1024` (per D-06; base64 of 512KiB ~ 683KiB fits the bridge's 1MiB maxBodyBytes ceiling).

    Add a batch JOIN helper that, given the list of session.Info entries with non-zero TaskID, runs ONE query shaped exactly like agents.go:99-103 — `SELECT t.id, t.title, p.name, a.name FROM tasks t JOIN projects p ON p.id=t.project_id JOIN agents a ON a.id=p.agent_id WHERE t.id IN (...)` — and returns a map[int64]{taskTitle, projectName, agentName}. Entries whose taskID is missing from the DB (deleted under a still-tracked session) get empty strings (mirrors agents.go's `continue` posture). Reuse this helper for BOTH the list and get_session handlers (D-10).

    Extend `(h *sessionHandlers) list` (sessions.go:50): when `r.URL.Query().Get("project_id")` is present, parse int64 (400 on parse error like the existing task_id branch), query the project's task IDs into a set, and filter the in-memory infos by `info.TaskID in set`. The existing `?task_id=N` branch and reconcileTmux (sessions.go:83) stay byte-for-byte unchanged — the SPA still gets its orphaned tmux-survivor rows (D-13 filters those in the BRIDGE, not here). Convert the final infos slice to []sessionDetail by attaching the batch JOIN fields; return via writeJSON 200. Dev sessions (TaskID==0) keep empty JOIN fields (omitted on the wire).

    Add `(h *sessionHandlers) getSession(w, r)`: `h.mgr.Get(r.PathValue("id"))` -> if !ok, writeError 404 "session not found"; else single-task JOIN (the batch helper with a one-element slice, or a direct QueryRow) -> build sessionDetail -> writeJSON 200. This works for exited in-memory sessions because Snapshot/Info survive exit (D-12).

    Add `(h *sessionHandlers) getSessionOutput(w, r)`: `h.mgr.Get` -> 404 if missing. Parse `bytes` query with strconv.Atoi (default defaultOutputBytes when absent/empty; clamp to [1, maxOutputBytes]; set clamped = (parsed > maxOutputBytes)). `snap := sess.Snapshot()`; `start := max(0, len(snap)-effective)`; `lastN := snap[start:]`. `output := base64.StdEncoding.EncodeToString(lastN)`. Build sessionOutputEnvelope{Encoding:"base64", Output:output, Bytes:len(lastN), Clamped:clamped}; writeJSON 200. Use encoding/base64 stdlib. Snapshot is read-only on the existing 1MiB ring — no new long-lived state (MCPSESS-03).

    Register the two new routes inside SessionRoutes (sessions.go:31-38) as `mux.HandleFunc("GET /api/sessions/{id}", s.getSession)` and `mux.HandleFunc("GET /api/sessions/{id}/output", s.getSessionOutput)`. The existing `DELETE/PATCH /api/sessions/{id}` routes already use the {id} wildcard; Go 1.22 ServeMux disambiguates by method+path tail.

    Read-only enforcement (D-14): these handlers consume ONLY sess.Info(), sess.Snapshot() (and, for Task 2, sess.Attach/Detach/Done). The public PTY-write primitive at session.go:368 is never in scope on these read paths — do not call it. The subscribe path (Task 2) is plain HTTP, never the WS frame protocol.

    Tests in sessions_test.go (mirror the existing Test* shape using newSessionServer/newTaskSessionServer): (a) get_session happy — spawn a bash session via POST /api/sessions, GET /api/sessions/{id} returns 200 with id+status; (b) get_session 404 for a random uuid; (c) get_session_output returns 200 with encoding=="base64" and bytes<=requested; (d) get_session_output?bytes=999999 clamps and sets clamped==true; (e) list?project_id with no matching tasks returns [] (empty array, not null); (f) list without filter still returns the existing shape (regression — no panic, JSON array).
  </action>
  <verify>
    <automated>cd /home/jordi/workspace/github/kangent && go test ./internal/api/ -run 'TestGetSession|TestGetSessionOutput|TestListSessions|TestSessions' -count=1</automated>
  </verify>
  <acceptance_criteria>
    - `go build ./internal/api/` succeeds with no new imports beyond encoding/base64 and existing packages.
    - GET /api/sessions/{id} for a live session returns JSON containing keys id, status, taskTitle, projectName, agentName; for an unknown id returns HTTP 404 with body {"error":"session not found"}.
    - GET /api/sessions/{id}/output returns JSON with "encoding":"base64" and "bytes" an integer <= the requested bytes; with bytes=999999 the response has "clamped":true and bytes <= 524288.
    - GET /api/sessions?project_id=<id with no tasks> returns HTTP 200 with body `[]` (empty array, not null).
    - The list response entries include taskTitle/projectName/agentName keys for task-scoped sessions.
    - Scoped grep proof of read-only: `grep -c 'WriteInput' internal/api/sessions.go` == 0 (the new handlers never reach the PTY-write primitive).
    - `go vet ./internal/api/` is clean.
  </acceptance_criteria>
  <done>
    get_session/get_session_output/list(?project_id) return current session state and the ring snapshot with no new long-lived state; unknown ids 404; bytes is clamped to 512KiB with a clamped flag (MCPSESS-01/02/03 server-side). Read-only: internal/api/sessions.go contains zero references to the PTY-write primitive (D-14).
  </done>
</task>

<task type="auto">
  <name>Task 2: subscribe streaming endpoint + Kamacu-side SC3 detach proof (D-01..D-04, D-11, D-14, MCPSESS-04 server-side)</name>
  <files>internal/api/sessions.go, internal/api/sessions_test.go</files>
  <read_first>
    - internal/api/sessions.go (after Task 1 — the sessionHandlers struct already holds mgr+db)
    - internal/ws/handler.go lines 39-82 (the canonical attach->stream->detach loop to mirror, MINUS the readLoop/writeLoop/WriteInput half)
    - internal/session/session.go lines 342-363 (Attach queues ring replay as msg[0] under the same lock — D-02 drain target), 395-397 (Done channel), 356-363 (Detach never touches PTY)
    - internal/api/sessions_test.go (newSessionServer harness + exitSession helper; spawn a bash session to drive a real attach channel)
  </read_first>
  <behavior>
    - GET /api/sessions/{id}/subscribe?duration_seconds=N&include_history=1 attaches a fresh uuid connID, streams raw PTY octets as application/octet-stream for the bounded duration, and Detaches on EVERY return path
    - When include_history is absent/false the ring-replay first message is drained before live streaming begins (D-02)
    - On client disconnect (r.Context().Done()) the handler returns promptly and defer Detach runs — SC3's <100ms detach target
    - duration_seconds defaults to 30, hard-capped at 300 (MCPSESS-04); values <1 clamp to 1
    - On session exit mid-stream the channel closes (ok==false) and the handler returns cleanly
  </behavior>
  <action>
    Add constants `defaultSubscribeDuration = 30 * time.Second` and `maxSubscribeDuration = 300 * time.Second` (MCPSESS-04). Import `github.com/google/uuid` (already used elsewhere in the package) and `time`.

    Add `(h *sessionHandlers) subscribeSessionOutput(w http.ResponseWriter, r *http.Request)`: `h.mgr.Get(r.PathValue("id"))` -> if !ok writeError 404 "session not found" and return BEFORE writing any stream bytes. Parse `duration_seconds` query (strconv.Atoi; default defaultSubscribeDuration when absent; clamp to [1s, maxSubscribeDuration] via time.Duration(seconds)*time.Second; values parsed as <1 -> 1s). Parse `include_history` as true iff the query is present and one of "1","true","yes" (case-insensitive); default false (D-02).

    Set `w.Header().Set("Content-Type", "application/octet-stream")`; `w.WriteHeader(http.StatusOK)`; assert `flusher, _ := w.(http.Flusher)` (may be nil — guard every Flush call). `connID := uuid.NewString()`; `q := sess.Attach(connID)`; `defer sess.Detach(connID)` (SC3 — unconditional cleanup on EVERY return path; Detach NEVER touches the PTY per session.go:356). If `!includeHistory` drain the replay: `<-q` exactly once (Attach queues the full ring replay as the channel's FIRST message under the same lock that registers the queue — session.go:346; draining it once atomically removes it so only post-attach output streams; D-02). Guard the drain with a select on r.Context().Done() so a client that disconnected during replay-drain still returns promptly.

    `timer := time.NewTimer(duration)`; `defer timer.Stop()`. Enter a select loop:
    - `case chunk, ok := <-q:` if `!ok` { return } (queue closed = session exited per markExited session.go:192-194; buffered output was already delivered before close). Otherwise `w.Write(chunk)` and `if flusher != nil { flusher.Flush() }`.
    - `case <-timer.C:` return (duration cap backstop — D-11).
    - `case <-r.Context().Done():` return (client/bridge closed the request — the cancellation path that makes SC3 testable).

    The handler writes ONLY to the ResponseWriter; it reads ONLY from the attach channel and r.Context(). It does NOT spawn any goroutine that outlives the request (milestone rule: no new long-lived/background goroutines inside Kamacu; the per-request reader IS the handler itself, request-scoped). It does NOT import internal/ws and does NOT speak the WS frame protocol — plain HTTP chunked output only (D-14: no WS PTY-input frame is ever sent on this read path). The public PTY-write primitive at session.go:368 is never referenced.

    Register `mux.HandleFunc("GET /api/sessions/{id}/subscribe", s.subscribeSessionOutput)` inside SessionRoutes.

    Tests in sessions_test.go using newSessionServer (real Manager — bash spawns are cheap per the harness comment): (a) TestSubscribe_Happy_ReturnsOctetsAndDetaches — POST a bash session, write a few bytes via the returned session's input (use mgr.Get + the session's input method through the test helper, NOT via HTTP), GET /api/sessions/{id}/subscribe?duration_seconds=1 with include_history=1, assert the response body is non-empty and Content-Type is application/octet-stream, and assert after the request completes the session is still operable (a subsequent GET /api/sessions/{id} returns 200); (b) TestSubscribe_UnknownID_404 — GET /api/sessions/{random}/subscribe returns 404 with no body bytes streamed; (c) TestSubscribe_ClientCancel_DetachesPromptly (SC3 Kamacu half) — POST a bash session, start the subscribe request with a cancellable context in a goroutine, sleep briefly so the attach registers, cancel the request context, assert the handler returns within a generous bound (e.g. 1s — the production target is <100ms but the test proves promptness + no hang) AND assert the session's internal conns no longer holds the subscribe connID (no leak): since conns is private, assert indirectly by confirming a SECOND subscribe on the same session attaches cleanly and the first request's goroutine has exited (runtime.NumGoroutine stable, or use a wg). Mirror whatever attach/detach observability the existing ws tests use; if none exists, assert via a short bounded wait that the first handler returned. (d) TestSubscribe_DurationCap — duration_seconds=999 streams for at most maxSubscribeDuration (assert the request returns without hanging past 300s+grace by using a test duration_seconds=1 and confirming it returns near 1s).
  </action>
  <verify>
    <automated>cd /home/jordi/workspace/github/kangent && go test ./internal/api/ -run 'TestSubscribe' -count=1 -timeout 60s</automated>
  </verify>
  <acceptance_criteria>
    - GET /api/sessions/{id}/subscribe streams application/octet-stream and returns by the duration cap without hanging.
    - GET /api/sessions/{random}/subscribe returns HTTP 404 BEFORE writing any stream body.
    - The SC3 Kamacu-side test proves cancelling the request context makes the handler return promptly (no hang) and the attach is cleaned up (a follow-up subscribe works; goroutine count does not grow across the cycle).
    - duration_seconds=999 does NOT stream longer than maxSubscribeDuration (server-side cap holds).
    - Scoped read-only proof: `grep -c 'WriteInput' internal/api/sessions.go` == 0 and `grep -c 'FrameData' internal/api/sessions.go` == 0 (the subscribe path is plain HTTP, never WS, never a PTY-write).
    - The handler introduces NO goroutine launched with `go ` that is not request-scoped (the handler body itself is the reader; no `go func` that outlives the request).
    - `go vet ./internal/api/` is clean.
  </acceptance_criteria>
  <done>
    subscribe_session_output streams bounded raw PTY octets with default 30s / cap 300s, drains the replay for live-only, and Detaches within the SC3 window on cancel/exit/duration without leaking. No new background goroutine; read-only enforced (no PTY-write primitive, no WS FrameData input frame). Delivers MCPSESS-04 server-side.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| MCP subcommand -> Kamacu HTTP (loopback) | The bridge (separate process) crosses here over 127.0.0.1; auth is loopback-binding only (Phase 06 D-06), X-Kamacu-Token sent but not enforced on /api/*. |
| Kamacu handler -> in-memory PTY session | The new read endpoints reach the session engine; the read-only contract (D-14) must hold at this boundary. |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-08-01 | Tampering / Elevation of Privilege | new /api/sessions/{id}* handlers | critical | mitigate | Type-level read-only (D-14): handlers consume ONLY Snapshot/Attach/Detach/Done/Info; subscribe is plain HTTP chunked (no WS FrameData input frame). Scoped grep gate in acceptance criteria proves the PTY-write primitive is absent from internal/api/sessions.go. |
| T-08-02 | Denial of Service | GET /api/sessions/{id}/output?bytes=N | medium | mitigate | Server-side clamp to maxOutputBytes (512KiB) + clamped flag (D-06/Pitfall 8); base64 result (~683KiB) fits bridge maxBodyBytes. |
| T-08-03 | Denial of Service | GET /api/sessions/{id}/subscribe?duration_seconds=N | medium | mitigate | Server-side clamp to maxSubscribeDuration (300s) per MCPSESS-04; SDK ctx cancel is the real backstop on the bridge side. |
| T-08-04 | Denial of Service | subscribe unbounded memory | medium | mitigate | Bridge-side (Plan 03) 1MiB tail cap; Kamacu side streams incrementally with http.Flusher so it never buffers unbounded. |
| T-08-05 | Denial of Service | subscribe goroutine/conn leak on cancel | high | mitigate | defer sess.Detach(connID) on every return path (SC3); request-scoped reader (handler is the reader). SC3 Kamacu-side test is the regression gate. |
| T-08-07 | Information Disclosure | terminal output sensitivity | low | accept | Loopback-only (Phase 06 D-06); delivered only to a user-spawned process; no logging of output bytes in these handlers. |

Package legitimacy: Phase 08 installs ZERO new packages (08-RESEARCH confirms — all deps from Phase 06/07). No install tasks; no T-08-SC slopcheck gate required.
</threat_model>

<verification>
- `go build ./internal/api/` compiles.
- `go test ./internal/api/ -count=1` passes (existing sessions tests unchanged + new get/output/list/subscribe tests green).
- `go vet ./internal/api/` clean.
- Scoped read-only grep: `grep -c 'WriteInput' internal/api/sessions.go` == 0 and `grep -c 'FrameData' internal/api/sessions.go` == 0.
</verification>

<success_criteria>
- get_session / get_session_output / list(?project_id) return current session state + ring snapshot (SC1/SC2 server-side).
- subscribe streams bounded raw output with duration cap + Detach-on-cancel (SC3 Kamacu half).
- No new DB schema/migration, no new long-lived goroutine, no PTY-write path (SC4 server-side + milestone rules).
</success_criteria>

<output>
Create `.planning/phases/08-sessions-terminal-read-access/08-01-SUMMARY.md` when done.
</output>

## Artifacts this phase produces (Plan 01)

**internal/api/sessions.go (new symbols):**
- Type `sessionDetail` (embeds `session.Info` + TaskTitle/ProjectName/AgentName JSON fields) — D-10 JOIN envelope.
- Type `sessionOutputEnvelope` (Encoding/Output/Bytes/Clamped) — D-08 snapshot envelope.
- Constants `defaultOutputBytes = 4096`, `maxOutputBytes = 512*1024`, `defaultSubscribeDuration = 30*time.Second`, `maxSubscribeDuration = 300*time.Second`.
- Helper `joinSessionContext` (or similarly named) — batch tasks->projects->agents JOIN.
- Handlers `(h *sessionHandlers) getSession`, `(h *sessionHandlers) getSessionOutput`, `(h *sessionHandlers) subscribeSessionOutput`.
- New `?project_id=N` branch inside `(h *sessionHandlers) list`.
- Route registrations: `GET /api/sessions/{id}`, `GET /api/sessions/{id}/output`, `GET /api/sessions/{id}/subscribe` (inside SessionRoutes).

**internal/api/sessions_test.go (new tests):** TestGetSession_*, TestGetSessionOutput_* (incl. clamp), TestListSessions_ProjectID_*, TestSubscribe_* (incl. SC3 detach-on-cancel + duration cap).

**No new:** DB tables, migrations, background goroutines, package imports beyond encoding/base64 + existing.
