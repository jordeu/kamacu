# Phase 08: Sessions & Terminal Read Access - Context

**Gathered:** 2026-07-23
**Status:** Ready for planning

<domain>
## Phase Boundary

The milestone's risk center and only genuinely new capability: live, **read-only** terminal access for agents. An agent inside a Kamacu task PTY can enumerate sessions across the board and read terminal output — snapshots immediately, live tails bounded — without ever injecting bytes into a PTY. Everything else in v1.11 is translation; this phase introduces streaming read access from a separate process (the MCP subcommand) to Kamacu's in-memory session engine.

Delivers:

1. **4 MCP session/terminal-read tools** in a new `internal/mcp/sessions.go` (D-07 per-resource split, repeating the Phase 06/07 bridge pattern):
   - `list_sessions(project_id? or task_id?)` — enumerate live in-memory terminal sessions (bash + agent) with status.
   - `get_session(session_id)` — full detail (status, task, project, agent, started_at).
   - `get_session_output(session_id, bytes?)` — read-only snapshot of the session's PTY ring buffer (last N bytes, default 4 KB).
   - `subscribe_session_output(session_id, duration_seconds?, include_history?)` — bounded live tail (default 30s, cap 300s); collect-and-return; cancellation via `ctx.Done()` → `Session.Detach`; no PTY keystroke injection (read-only).
2. **New read-only Kamacu HTTP endpoints** exposing the existing v1.0 Phase 2 session engine to the bridge (the MCP subcommand is a separate process and reaches Kamacu only over HTTP):
   - `GET /api/sessions/{id}` — get_session (session.Info + task/project/agent JOIN).
   - `GET /api/sessions/{id}/output?bytes=N` — get_session_output (raw ring snapshot).
   - A bounded-duration streaming endpoint backing `subscribe_session_output` (transport shape is the agent's discretion — see D-09).
   - `?project_id=N` query param on the existing `GET /api/sessions` (list_sessions project-scoping needs a task→project JOIN; the existing `?task_id=N` stays).

**Out of this phase (per REQUIREMENTS.md Out of Scope / Deferred):**

- **PTY keystroke injection via MCP** — permanently out of scope. SC4 enforces read-only at the type level (a wrapper exposing Attach/Snapshot/Detach with NO public Write method — never `Session.WriteInput`, never a `FrameData` PTY-input frame). PTY write stays browser-only.
- The PR-review tools (`list_pending_reviews`, `list_recently_reviewed`, `open_review`) — Phase 09 (MCPREV × 3).
- Per-task auto-scoping (`get_my_session`, ToolFilter) — MCPAUTO-01..03, deferred to v1.12. Agents pass explicit `session_id` in v1.11.
- Agent-CLI auto-registration of `kamacu mcp serve` — MCPREG-01..03, deferred to v1.12.
- Additional tool categories (Agents CRUD, Settings, Worktree cleanup) — MCPMORE-01..03, deferred to v1.12.
- Typed JSON-RPC error taxonomy, full stdout-pollution guards, real-binary e2e harness — MCPHARD-01..03, deferred to v1.12. Phase 08 surfaces generic MCP errors via the Phase 07 D-05 wrap; SDK panic-recovery wrapping IS in scope here (subscribe introduces the streaming panic surface area deferred from Phase 06's Open Question 1).
- Token auth middleware on general `/api/*` routes — explicitly out of scope for v1.11 (Phase 06 D-06); loopback binding remains the auth boundary. The new session endpoints inherit this posture unchanged.
- **MCP progress-notification streaming of raw PTY bytes** — D-01 chose collect-and-return; progress notifications are not used to carry terminal output (they are a poor fit for binary streams). A liveness-heartbeat use of progress notifications was also not chosen.
- **Headless-VT-emulator snapshots** — `Session.Snapshot()` returns raw PTY bytes today (D-02). A clean text snapshot rendered by a headless VT is a future swap behind the same signature (the Phase 4 seam comment at `session.go:384` explicitly anticipates this); not in v1.11.

**Requirements covered:** MCPSESS-01, MCPSESS-02, MCPSESS-03, MCPSESS-04.

</domain>

<decisions>
## Implementation Decisions

### Live-tail delivery model (D-01..D-04) — the risk center

- **D-01:** **`subscribe_session_output` is collect-and-return.** The tool handler ranges over the session's attach-channel output for the bounded `duration_seconds` (default 30s, hard cap 300s per MCPSESS-04), OR until `ctx.Done()` fires (the agent CLI sent `notifications/cancelled` — the SDK cancels the handler ctx, verified in the SDK's `Example_cancellation`), accumulating every chunk. It returns ONE `CallToolResult` (a single TextContent blob) at the end. This matches SC3 verbatim — "returns the partial stream collected so far" — and is both simplest and most correct: MCP progress notifications are designed for progress bars (a `Message` string + progress/total floats), not high-volume binary terminal streams, so they are NOT used to carry PTY output. The agent gets a bounded dump of what happened in the window, not a live feed. (The SDK does support `req.Session.NotifyProgress`; we deliberately do not use it for output transport.)
- **D-02:** **An `include_history?` boolean arg (default `false` = live-only) controls the ring-buffer replay.** `Session.Attach(connID)` queues the entire ring replay as the channel's FIRST message (`session.go:346`, under the same lock that registers the queue). Live-only (default): the handler attaches, drains+discards that first replay message, then collects ONLY output produced after subscribe started — gap-free vs the live stream (the same lock guarantee the WS handler relies on). `include_history=true`: the replay is kept, so the tail is self-contained (replay + live) for a one-shot "give me everything" call. The default `false` avoids duplicating `get_session_output` (the dedicated snapshot tool) and bounds the return to just what's new.
- **D-03:** **On session exit mid-tail, return early + a structured exit marker.** `Session.Done()` fires → stop collecting immediately, return the bytes gathered so far PLUS a structured note that the session exited (exit code if available, and whether it was a server-requested stop — `Info().StopRequested`). Highest-value outcome: the agent learns the sibling finished/crashed and what it last said. (This is why the result is a JSON envelope, not pure bytes — see D-08.)
- **D-04:** **Cap the accumulated buffer at ~1 MiB; keep the MOST RECENT bytes; append a `truncated` note.** A 300s tail of a chatty agent could exceed memory/response budgets. When accumulated output exceeds the cap, drop from the FRONT and keep the most recent N (what an agent most needs — what just happened), and set a `truncated:true` flag in the result envelope. Predictable, bounded, never OOMs the subcommand.

### Terminal-output encoding (D-05..D-08)

- **D-05:** **Deliver raw PTY bytes base64-encoded.** The ring buffer holds arbitrary bytes — ANSI color/cursor escapes, control codes, and for a full-screen TUI (claude) constant full-screen redraws; it may contain invalid UTF-8. MCP `TextContent` is a UTF-8 string over JSON, so raw bytes cannot be inlined. Base64 is faithful (no server-side lossy transformation), safe over JSON regardless of byte validity, and matches the project's honest-no-transform posture (cf. D-M001-2: custom-agent status reports only running/exited because TUIs can't be reliably introspected — we apply the same "don't fake-clean what we can't reliably interpret" principle here). The result notes the encoding so the agent CLI knows to decode + strip ANSI as needed. (ANSI-stripped text was rejected as lossy and because a TUI's raw redraw bytes stay messy even after stripping.)
- **D-06:** **`bytes?` = the LAST N bytes of the ring (most recent output); default 4096 (locked by MCPSESS-03); hard cap ~512 KiB raw.** "Last N" is what an agent almost always wants (what just happened). The ~512 KiB cap is set so the base64-encoded result (~683 KiB) comfortably fits the bridge's existing 1 MiB `maxBodyBytes` response ceiling (`bridge.go:23`) with no special-case bypass. Requesting more than the cap → the server returns the last ~512 KiB + a `clamped`/`truncated` note in the envelope.
- **D-07:** **Base64 is applied so the result fits the bridge ceiling; no bypass of `maxBodyBytes` for snapshots.** (Follows from D-06. If a future phase wants full-ring snapshots, it raises the cap deliberately then.)
- **D-08:** **The result is a JSON envelope returned as TextContent.** Shape (subscribe carries the exit/truncation fields; snapshot carries the clamp field):
  ```
  { "encoding":"base64", "output":"<base64>", "bytes":<int>,
    "truncated":<bool>,                 // D-04 tail cap OR D-06 snapshot clamp hit
    "exited":<bool>, "exitCode":<int?>, "stopRequested":<bool?> }  // D-03 subscribe only
  ```
  Consistent with Kamacu's other JSON endpoints (the bridge passes Kamacu's JSON through as TextContent, exactly like the Phase 07 tools), and machine-parseable so the agent can reason about metadata (did the tail end because the session exited? was it truncated?) rather than parsing human-readable prose.

### list_sessions / get_session scope & endpoints (D-09..D-11)

- **D-09:** **`list_sessions` enumerates ALL live in-memory terminal sessions (bash tabs AND agent sessions) via `/api/sessions` — NOT `/api/agents/status`.** The 4 tools share one coherent session-id space: every id `list_sessions` returns is a real PTY-backed session with a ring buffer that `get_session`/`get_session_output`/`subscribe_session_output` can operate on. The Active Sessions bar (`/api/agents/status`) is a separate agent-only UI view (one-newest-per-task, includes post-restart "resumable" entries with `sessionId=""` and no live PTY) and is NOT what these tools expose — those entries would be un-operable by the terminal-read tools. Status semantics: each entry carries `session.Info.Status` (running/exited — "running" is the bash/plain state in the MCPSESS-01 enum) AND, for `kind=="agent"`, `Info.AgentStatus` (working/waiting/idle/exited — the agent sub-states in the enum). The mixed enum is thus satisfied by returning both fields per entry.
- **D-10:** **Both `list_sessions` and `get_session` entries JOIN task/project/agent context.** SC1 requires entries to carry "task, project, agent, started_at", but `session.Info` only has `taskID` (no project/agent name). So the `/api/sessions` listing and the new `GET /api/sessions/{id}` both JOIN `tasks → projects → agents` (the same JOIN `/api/agents/status` already does — `agents.go:99`) to surface `taskTitle`/`projectName`/`agentName`. The agent sees full context in one call; no extra `get_task` round-trips.
- **D-11:** **New endpoints on Kamacu (all read-only, all under `/api/sessions`):**
  - `GET /api/sessions/{id}` — `get_session` (session.Info + the D-10 JOIN).
  - `GET /api/sessions/{id}/output?bytes=N` — `get_session_output` (raw ring snapshot per D-05/D-06).
  - A bounded-duration streaming endpoint backing `subscribe_session_output` per D-01..D-04. **Transport shape (HTTP chunked streaming vs SSE-shaped vs a tailored WS variant) is the agent's discretion** — but it MUST: (a) attach via `Session.Attach(uniqueConnID)`; (b) for live-only, drain the replay first; (c) stream chunks to the response as they arrive; (d) on client disconnect (`r.Context().Done()` — the bridge closed the request because the MCP handler's ctx was cancelled) call `Session.Detach(connID)` promptly (SC3: within ~100ms); (e) honor the duration cap server-side as a backstop. The bridge CANNOT use its 10s-timeout `http.Client` (`bridge.go:53`) or the `bridge.call` helper (which buffers the full body) for this — subscribe needs a dedicated streaming read with no short timeout. Request-scoped goroutines (one reader per active subscribe) are expected and fine; NO new background/long-lived goroutines inside Kamacu (the milestone rule).
  - `?project_id=N` query param on the existing `GET /api/sessions` (project-scoping needs a task→project JOIN — find the project's task ids, then filter; the existing `?task_id=N` stays, mirroring the Phase 07 D-01/D-02 endpoint-extension pattern).

### Dead-session & edge behavior (D-12..D-13)

- **D-12:** **Tools OPERATE on EXITED in-memory sessions — read their final output.** The ring buffer survives exit (`Snapshot()` always works; `Attach()` post-exit returns the replay then closes — `session.go:347`). So: `get_session` returns `status=exited`+`exitCode`; `get_session_output` returns the final ring snapshot (what the sibling left on screen — high-value "what did it say before dying"); `subscribe_session_output` returns immediately — live-only: empty tail + exit marker (D-03), `include_history=true`: the replay + exit marker. The agent doesn't have to race the window before the ring is reaped/deleted.
- **D-13:** **`list_sessions` FILTERS OUT orphaned tmux-survivor entries; unknown/gone `session_id` → 404.** `/api/sessions` currently synthesizes orphaned tmux-survivor rows after a server restart (`sessions.go:135`, `orphaned:true`, `tmuxName` set) that have NO live in-memory session until the user reopens the tab. Those are a SPA-reattach concern, not terminal-read targets (no ring buffer yet), so the MCP listing excludes them — every id `list_sessions` returns is operable. An unknown / already-deleted / reaped `session_id` → Kamacu returns 404 → the bridge wraps it per the Phase 07 D-05 error pattern (`kamacu GET /api/sessions/{id}: HTTP 404: ...`).

### Read-only enforcement (D-14)

- **D-14:** **The read-only contract is enforced at the type level, not by convention.** The new Kamacu endpoints and the MCP `sessions.go` handlers consume ONLY `Session.Snapshot()` / `Session.Attach()` / `Session.Detach()` / `Session.Done()` / `Session.Info()` — NEVER `Session.WriteInput` and NEVER a WS `FrameData` PTY-input frame. SC4's "a wrapper with no public Write method" means the code path literally has no access to a write primitive. (The existing WS handler at `ws/handler.go` is the only PTY-write surface and stays browser-only; Phase 08 adds no new write path.) Researcher/planner should structure the streaming endpoint so it physically cannot write to the PTY.

### the agent's Discretion

- **Live-tail HTTP transport shape** — chunked HTTP streaming, an SSE-shaped response, or a tailored read-only WS variant. The constraints in D-11 (attach/detach, drain-replay-for-live-only, ~100ms detach on disconnect, server-side duration cap, request-scoped goroutines only) are locked; the wire shape is the agent's call. The coder/websocket library is already a dependency (used by `internal/ws`), but a plain HTTP chunked stream is likely simpler for a read-only single-consumer tail.
- **Exactly where base64 is applied** — Kamacu returns raw octet-stream and the bridge base64-encodes into the JSON envelope, vs Kamacu returns the full JSON envelope with base64 already inline. The bridge currently passes Kamacu's JSON through as TextContent; whichever is consistent with that and the D-08 envelope shape. Researcher's call.
- **`?project_id=N` JOIN implementation** — query the project's task ids then filter in-memory sessions, vs a single SQL pass. Pick whichever is cleanest given `manager.List()` is in-memory.
- **The exact ~512 KiB / ~1 MiB constants** — D-06/D-04 say "~512 KiB" snapshot cap and "~1 MiB" tail cap; the precise numbers (and whether they share one const) are the agent's. Keep both comfortably under the base64-of-1 MiB budget.
- **`internal/mcp/sessions.go` handler + test layout** — mirror Phase 07's per-resource shape (D-07): `registerSessionTools(s, b)` called from `registerTools`; `bridge.listSessions`/`get_session`/etc. methods; one `sessions_test.go` with happy + error cases per tool. `subscribe` needs a cancellation/leak test (SC3) — the SDK's in-memory transport + a fake streaming server can exercise the ctx.Done path. Test shape is the agent's.
- **Exact InputSchema map shape per tool** — property maps, descriptions, required-arrays. Phase 07's flat `map[string]any` template applies. No user preference on description copy beyond "lead with what it does, note it's read-only".
- **SDK panic-recovery wrapping** — Phase 06 deferred `defer recover()` to Phase 08 (06-RESEARCH Open Question 1) because subscribe introduces the streaming panic surface. Whether to wrap every session handler or just the streaming one is the agent's call; the SC2 stream-desync invariant must hold.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & roadmap (scope anchors)
- `.planning/ROADMAP.md` § "Phase 08: Sessions & Terminal Read Access" — the goal and the **4 success criteria** (list/get return current state identical to the bar's semantics; get_session_output returns a ring snapshot with no new long-lived state; subscribe tails bounded with <100ms detach on cancel and stable goroutine count across N cycles; NO tool exposes any PTY-input/keystroke capability, enforced at the type level).
- `.planning/REQUIREMENTS.md` § "Sessions & Terminal (MCPSESS)" lines 25–30 — **MCPSESS-01..04** signatures locked: `list_sessions(project_id? or task_id?)`, `get_session(session_id)`, `get_session_output(session_id, bytes?)` (default 4 KB, read-only), `subscribe_session_output(session_id, duration_seconds?)` (default 30s, cap 300s, cancellation via `ctx.Done()` → `Session.Detach`, read-only).
- `.planning/REQUIREMENTS.md` § "Out of Scope" — PTY keystroke injection via MCP (permanent), MCP server for external editors, auto-registration, per-task auto-scoping, additional tool categories, and the MCPHARD-* list are all out of Phase 08. Read before planning so none leak in.
- `.planning/PROJECT.md` § "Active: v1.11 Kamacu MCP Server" — milestone goal; the **"Backend-only milestone: no DB schema changes, no migrations, no frontend changes, no new long-lived goroutines inside the Kamacu binary"** rule. Phase 08's new endpoints are pure route+handler additions (no schema, no migration); subscribe's per-request reader goroutine is request-scoped, NOT a new background goroutine — satisfies the rule.

### Phase 06 + 07 foundation (the pattern Phase 08 repeats)
- `.planning/phases/06-mcp-subcommand-foundation/06-CONTEXT.md` — the bridge pattern is fully locked here. Read D-06/D-07 (loopback auth boundary; `X-Kamacu-Token` header on every call), D-09 (proof tool is FINAL), D-12 (stdout cleanliness). Phase 08's session tools inherit the `*bridge`, the `map[string]any` InputSchema, the raw TextContent passthrough, and the loopback-only auth posture verbatim.
- `.planning/phases/06-mcp-subcommand-foundation/06-RESEARCH.md` § "Open Question 1" — confirms SDK panic-recovery wrapping is deferred to Phase 08 (when `subscribe_session_output` introduces streaming panic surface area). Phase 08 MUST address it.
- `.planning/phases/07-tasks-projects-workspaces-tools/07-CONTEXT.md` — D-05 (error wrap as single string: `kamacu %s %s: HTTP %d: %s`), D-07 (per-resource file split — Phase 08 adds `sessions.go`), D-08 (one representative test per tool). The `bridge.call` helper and the endpoint-extension pattern (D-01/D-02 added small query params to existing Kamacu routes) are the direct templates for D-11's `?project_id=N`.

### Session engine (the code Phase 08 reads out — the whole point of the phase)
- `internal/session/session.go:387` `Session.Snapshot()` — **the MCPSESS-03 seam.** Its doc literally says "This method is the Phase 4 seam: the WS layer must only ever consume Attach/Snapshot — never reach into the ring." Returns a copy of the 1 MiB ring (`circbuf.Buffer`). D-06 slices the last N bytes.
- `internal/session/session.go:342` `Session.Attach(connID)` — **the MCPSESS-04 fan-out primitive** (same one the WS handler uses). Queues the ring replay as the channel's FIRST item under the same lock that registers the queue (gap-free). D-02's live-only path drains this first message. Returns `<-chan []byte`, buffered 256.
- `internal/session/session.go:356` `Session.Detach(connID)` — unsubscribe; SC3's "<100ms detach on cancel" calls this. NEVER touches the PTY.
- `internal/session/session.go:395` `Session.Done()` — closed after exit; D-03's "return early + exit marker" selects on this. Once closed, `Info().ExitCode` is final.
- `internal/session/session.go:199` `Session.Info()` — the JSON-ready snapshot (ID/Label/Status/CreatedAt/TaskID/Kind/Engine/AgentStatus/StopRequested/ExitCode). D-09/D-10's listing and get_session build on this + the JOIN.
- `internal/session/session.go:125` `Session.pump()` — the single reader that fans chunks to the ring + every attached queue, dropping slow consumers (delete+close under the lock). Subscribe is just another consumer; it must not be a slow consumer (D-04's cap prevents unbounded buffering, but the handler must keep draining).
- `internal/session/session.go:368` `Session.WriteInput()` — **the PTY-write primitive Phase 08 MUST NOT touch** (D-14). The WS handler is the only legitimate caller.
- `internal/session/manager.go:405/414/421` `Manager.Get/List/ListByTask` — the listing primitives. No `ListByProject` (D-11's `?project_id=N` adds it via JOIN).

### Kamacu HTTP surface Phase 08 extends
- `internal/api/routes.go:32-46` — where the new `/api/sessions/{id}`, `/api/sessions/{id}/output`, and the streaming route register (alongside the existing `GET /api/sessions`). `Routes()` owns the `/api/*` tree.
- `internal/api/sessions.go:31-38` `SessionRoutes` + `sessionHandlers` — the existing `/api/sessions` surface (list/create/stop/delete/rename). D-11's `?project_id=N` extends `list` (`sessions.go:50`); the new get/output/stream handlers join `sessionHandlers` (which already holds `mgr` + `db` — everything needed for the JOIN and ring access).
- `internal/api/sessions.go:83` `reconcileTmux` — synthesizes the orphaned tmux-survivor entries D-13 filters OUT of the MCP listing (they have no live session/ring).
- `internal/api/agents.go:44-219` `agentHandlers.status` — **the JOIN template** for D-10 (`tasks → projects → agents`, surfacing taskTitle/projectName/engine). The MCP session listing/get_session reuse this exact JOIN shape.
- `internal/ws/handler.go:39-82` `Handler.ServeHTTP` + `writeLoop`/`readLoop` — **the canonical attach→stream→detach pattern** the streaming endpoint mirrors (read-only: omit the readLoop/WriteInput half). Note the uuid `connID`, `defer sess.Detach(connID)`, and the `ctx, cancel := context.WithCancel(r.Context())` shape.
- `internal/ws/proto.go` — `FrameData` ('0', bidirectional) / `FrameResize` / `FrameExit`. D-14: a subscribe/streaming path sends OUTPUT only — never a `FrameData` input frame.

### Bridge pattern (the code Phase 08 extends — and where it must diverge for streaming)
- `internal/mcp/bridge.go:31-67` `bridge` struct + `do()` — the per-process HTTP client with `X-Kamacu-Token` on every call. D-11: subscribe CANNOT use this client as-is (10s timeout).
- `internal/mcp/bridge.go:53` `http.Client{Timeout: 10 * time.Second}` — **the constraint driving D-11's dedicated streaming transport.** A 30–300s tail times out here; subscribe needs a separate client/transport without the short timeout (or with a >300s timeout).
- `internal/mcp/bridge.go:88-106` `bridge.call()` — the shared response-handling helper every Phase 07 tool delegates to (`do` → `LimitReader(maxBodyBytes)` → non-2xx wrap → 2xx TextContent). `get_session`/`get_session_output`/`list_sessions` reuse it; `subscribe_session_output` does NOT (it streams, then assembles its own envelope per D-08).
- `internal/mcp/bridge.go:23` `maxBodyBytes = 1<<20` (1 MiB) — the response ceiling D-06's ~512 KiB snapshot cap is sized to fit after base64.
- `internal/mcp/server.go:82-86` `registerTools` — where `registerSessionTools(s, b)` slots in alongside the Phase 07 registrars (D-07 split).
- `internal/mcp/{tasks,projects,workspaces}.go` — the per-resource handler template (one file, `register*Tools` + `bridge.*` methods, `map[string]any` InputSchema). `sessions.go` mirrors this.

### MCP SDK (the dependency Phase 08 pushes on — researcher must verify)
- `github.com/modelcontextprotocol/go-sdk` @ v1.6.1, `mcp/tool.go:30` `ToolHandler func(context.Context, *CallToolRequest) (*CallToolResult, error)` — the handler signature subscribe uses.
- `mcp/mcp_example_test.go:108` `Example_cancellation` — **proves** the SDK cancels the handler `ctx` on `notifications/cancelled` (the `select { case <-ctx.Done(): }` shape). D-01/D-03 depend on this.
- `mcp/mcp_example_test.go:60` `Example_progress` — shows `req.Session.NotifyProgress` + `req.Params.GetProgressToken()`. D-01 deliberately does NOT use this for output transport (kept here as the reference should a future phase revisit liveness heartbeats).
- `mcp/protocol.go:1607` `notificationCancelled = "notifications/cancelled"` — the cancellation notification name.

### Established project patterns (constraints, not files)
- **Read-only enforced at the type level (D-14)** — consume only Snapshot/Attach/Detach/Done/Info; never WriteInput, never a FrameData input frame.
- **Loopback-only auth boundary (Phase 06 D-06)** — the new session endpoints inherit it; no token middleware added.
- **`httptest`-based integration tests** — `internal/api/*_test.go` and `internal/mcp/*_test.go` use real `httptest.Server`s; Phase 08's `sessions_test.go` mirrors them, plus an in-memory-transport cancellation test for SC3.
- **`slog` to stderr** — every package uses `log/slog` to stderr; the MCP subcommand pins this (`server.go:49`). The streaming endpoint adds no stdout writes (SC2 stream-desync invariant).
- **Single-writer SQLite discipline** — the bridge code must NOT import `internal/store`; it goes through the HTTP API. The new endpoints ARE inside `internal/api/` and use the existing `*sql.DB` for the JOIN.

No external ADRs/specs — requirements are fully captured in the decisions above plus these in-repo references.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`Session.Snapshot()` (`session.go:387`)** — IS `get_session_output`. Returns a copy of the 1 MiB ring; D-06 slices the last N bytes. No new ring state (MCPSESS-03's "no new long-lived state" satisfied for free).
- **`Session.Attach(connID)`/`Detach(connID)`/`Done()` (`session.go:342/356/395`)** — ARE `subscribe_session_output`'s fan-out primitives, identical to what the WS handler consumes. Attach queues the replay first (D-02 live-only drains it); Done fires on exit (D-03 return-early); Detach is the SC3 cleanup.
- **`agentHandlers.status` JOIN (`agents.go:99-151`)** — the exact `tasks → projects → agents` query D-10 copies for the session listing/get_session context.
- **`internal/ws/handler.go` attach→stream→detach loop** — the canonical pattern the streaming endpoint mirrors (read-only half: attach, range the channel, write chunks, defer Detach, select on ctx).
- **`bridge.call` (`bridge.go:88`)** — the response-handling helper for the 3 non-streaming tools (list/get/get_output). Subscribe diverges (D-11).

### Established Patterns
- **Bridge pattern (locked in 06/07)** — env → `*bridge` HTTP client (`X-Kamacu-Token` header) → Kamacu endpoint → TextContent passthrough. The 3 non-streaming session tools repeat it verbatim; subscribe extends it with a streaming read.
- **Per-resource file split (D-07)** — `internal/mcp/sessions.go` owns the 4 tools' registrars + `bridge.*Session` methods + `sessions_test.go`, called once from `registerTools`.
- **Endpoint extension via query param (Phase 07 D-01/D-02)** — `?project_id=N` on `GET /api/sessions` mirrors `?workspace_id=N` on `GET /api/projects` and `GET /api/tasks`. Backward-compatible (no param = current behavior).
- **Raw TextContent passthrough of Kamacu JSON** — the Phase 07 tools pass Kamacu's JSON arrays/objects through as TextContent. D-08's JSON envelope is consistent with this (Kamacu returns the envelope; bridge passes it through).

### Integration Points
- **New routes in `internal/api/routes.go`** (or `SessionRoutes`) — `GET /api/sessions/{id}`, `GET /api/sessions/{id}/output`, and the streaming route. `sessionHandlers` already holds `mgr` + `db`; the new handlers join it.
- **`?project_id=N` on `GET /api/sessions`** — extend `sessionHandlers.list` (`sessions.go:50`) with the project JOIN; filter the orphaned tmux survivors out for the MCP path (D-13) — or filter in the bridge.
- **`registerSessionTools(s, b)` in `internal/mcp/server.go`'s `registerTools`** — one new line alongside the Phase 07 registrars.
- **`bridge.subscribeSessionOutput` (or similar)** — the one handler that does NOT use `bridge.call`: dedicated streaming HTTP read (no 10s timeout), accumulate + cap (D-04), drain-replay-if-live-only (D-02), select on ctx.Done (D-01/D-03), assemble the D-08 envelope.
- **`cmd/kamacu/main.go` is NOT modified** — Phase 06 already registers `mcpCmd`; Phase 08 only adds tools inside `internal/mcp/` + endpoints inside `internal/api/`.

</code_context>

<specifics>
## Specific Ideas

- **The agent is the user; honesty over hand-holding.** D-05 (raw base64, not ANSI-stripped) deliberately mirrors the project's D-M001-2 posture: don't fake-clean terminal output we can't reliably interpret. A full-screen TUI's redraw bytes are messy however we slice them; handing the agent the faithful bytes + telling it the encoding is more honest than a lossy strip that implies false cleanliness. The agent CLI is told "base64 raw PTY bytes; decode + strip ANSI as needed."
- **"Return the partial stream collected so far" is load-bearing.** SC3's exact words drove D-01 (collect-and-return) and D-03 (exit marker). The contract is a bounded dump of a window, not a live feed. Progress-notification streaming was considered and rejected as a misuse of the mechanism for binary terminal output.
- **Every listed id is operable.** D-09 (terminal sessions, not the bar) + D-13 (filter tmux orphans) together guarantee that any `session_id` from `list_sessions` works in the other 3 tools. No "listed but can't read" dead ends.
- **The ring buffer already exists; this phase adds read paths, not state.** MCPSESS-03's "no new long-lived state" and the milestone's "no new long-lived goroutines" rule are both satisfied by construction: Snapshot/Attach/Detach are existing primitives; subscribe's reader is request-scoped (dies with the HTTP request).
- **The read-only contract is a type-level constraint, not a runtime check.** D-14: the new code paths physically cannot reach `WriteInput`. Reviewer/planner should structure it so a write primitive is not even in scope at the call site.

</specifics>

<deferred>
## Deferred Ideas

- **MCP progress-notification streaming / liveness heartbeats** — D-01 chose collect-and-return. Using `req.Session.NotifyProgress` either to carry raw PTY bytes or as a "streaming… N bytes so far" heartbeat was considered and rejected for v1.11. If future agent CLIs turn out to need true live (incremental) output, revisit then.
- **Headless-VT-emulator clean-text snapshots** — `Session.Snapshot()` returns raw bytes today (D-05). A clean text snapshot rendered by a headless VT (xterm.js parser in a worker, or a Go VT lib) behind the same signature is a future swap the Phase 4 seam comment (`session.go:384`) explicitly anticipates. Not in v1.11.
- **Full-ring (1 MiB) snapshots** — D-06 caps at ~512 KiB so base64 fits the bridge ceiling without a bypass. Raising the cap (and `maxBodyBytes`) is a deliberate future change if an agent needs whole-history snapshots in one call.
- **`include_history` defaulting to true** — D-02 defaults it to false (live-only) to avoid duplicating `get_session_output`. If the common usage pattern turns out to want self-contained tails, flip the default later.
- **MCPAUTO-02 (`get_my_session`)** — per-task auto-scoping so an agent resolves its own session from `KAMACU_SESSION_ID` with no params. Deferred to v1.12; v1.11 agents pass explicit `session_id` from `list_sessions`.
- **MCPHARD-01..03** — typed bridge error taxonomy, stdout-pollution guards (os.Stdout redirect + CI grep), real-binary e2e harness. Phase 08 surfaces generic MCP errors via the D-05 wrap; the typed taxonomy + harness are v1.12.
- **Subscribe over WebSocket** — a tailored read-only WS variant of the streaming endpoint is technically possible (coder/websocket is already a dep), but D-11 leaves transport to the agent; plain HTTP chunked streaming is likely simpler for a single-consumer read-only tail.
- **Agent-CLI auto-registration (MCPREG)** — v1.11 users manually add `kamacu mcp serve` to their agent CLI's MCP config once. Deferred to v1.12.

None of these were pulled into Phase 08 — discussion stayed within the MCPSESS-01..04 boundary.

</deferred>

---

*Phase: 08-sessions-terminal-read-access*
*Context gathered: 2026-07-23*
