# Phase 08: Sessions & Terminal Read Access - Research

**Researched:** 2026-07-23
**Domain:** MCP tooling over Kamacu's in-memory session engine — bounded-duration streaming terminal-read access via a separate process (the MCP subcommand) reaching Kamacu over HTTP. Read-only by construction.
**Confidence:** HIGH (every load-bearing claim verified against the SDK source at `go-sdk@v1.6.1` in the Go module cache, or against Kamacu source in-repo)

## Summary

Phase 08 is the milestone's risk center because it is the only phase that introduces a **genuinely new capability** (everything else in v1.11 is translation): a streaming read path from the MCP subcommand (separate process) to Kamacu's in-memory PTY session engine. Three of the four tools (`list_sessions`, `get_session`, `get_session_output`) repeat the Phase 06/07 bridge pattern verbatim — they hit Kamacu JSON endpoints and pass the body through as `TextContent`. The fourth, `subscribe_session_output`, diverges: it cannot use the bridge's 10-second-timeout `http.Client` (`bridge.go:53`) or the body-buffering `bridge.call` helper (`bridge.go:88`); it needs a dedicated streaming HTTP read with no short timeout, accumulating output for a bounded window (30s default, 300s hard cap) and returning a single `CallToolResult` at the end.

The good news: every primitive `subscribe_session_output` needs already exists on `*session.Session` and is the SAME surface the WS handler consumes (`internal/ws/handler.go:39-82`). `Session.Attach(connID)` queues the ring replay as the channel's first message under the same lock that registers the queue (gap-free vs the live stream — `session.go:342-353`); `Session.Detach(connID)` deletes the queue and never touches the PTY (`session.go:356-363`); `Session.Done()` is closed on exit (`session.go:395-397`); `Session.Snapshot()` returns a ring copy (`session.go:387-391`). **MCPSESS-03's "no new long-lived state" is satisfied for free** — `Snapshot()` is read-only on the existing 1 MiB ring. **MCPSESS-04's "stable goroutine count across N cycles"** is satisfied because the subscribe reader goroutine is request-scoped (dies with the HTTP request) and `Detach` is unconditional via `defer`.

The MCP SDK cancellation flow is **VERIFIED** end-to-end: the agent CLI sends `notifications/cancelled` → the SDK's `canceller.Preempt` (`go-sdk@v1.6.1/mcp/transport.go:205-218`) calls `go c.conn.Cancel(id)` → the jsonrpc2 layer cancels the context associated with that in-flight request → the `ctx` parameter of the handler's `select { case <-ctx.Done(): }` fires. The SDK's `Example_cancellation` (`mcp_example_test.go:108-164`) confirms a handler can return `(partialResult, nil)` after observing `ctx.Done()` — which is exactly D-01's "return the partial stream collected so far" contract.

**Primary recommendation:** Plain HTTP chunked streaming for the live-tail transport (not SSE, not WS). It is the simplest read-only single-consumer shape, requires no new dependencies (coder/websocket is overkill for a one-way stream), maps directly to the WS handler's attach→stream→detach pattern (minus the readLoop), and lets the bridge cancel cleanly via `r.Context().Done()` when the MCP handler's `ctx` is cancelled. Apply base64 inside Kamacu (return the full D-08 JSON envelope) so the bridge's JSON-passthrough pattern stays intact.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Session listing / detail (D-09/D-10) | Kamacu HTTP API (`internal/api`) | MCP bridge (`internal/mcp`) | Kamacu owns the `*session.Manager` and the DB JOIN; the bridge just translates. Matches Phase 07's split. |
| Ring snapshot read (D-05/D-06) | Kamacu HTTP API | MCP bridge | Snapshot is a method on `*session.Session` (in-memory); the bridge cannot reach the ring directly (it's a separate process). |
| Bounded live tail (D-01..D-04) | Kamacu HTTP API (streaming handler) | MCP bridge (streaming reader) | Kamacu owns the PTY and the attach fan-out; the bridge owns the bounded accumulation, the cap, and the ctx-cancellation translation. **Both sides are request-scoped** — no new background goroutines inside Kamacu (milestone rule). |
| Cancellation propagation | MCP SDK (canceller) | Bridge handler (ctx.Done select) | The SDK owns the JSON-RPC `notifications/cancelled` → ctx-cancel translation; the handler's `select` is the response. **No new code needed in the SDK**. |
| Read-only enforcement (D-14) | Type-level on both tiers | — | Kamacu endpoints consume only `Snapshot/Attach/Detach/Done/Info`; bridge handlers consume only the HTTP response. `WriteInput` (`session.go:368`) and the WS `FrameData` input frame are never in scope at any call site. |
| Panic recovery (Phase 06 Open Q1) | MCP bridge handler (defer recover) | — | SDK does NOT wrap tool handlers (verified at `go-sdk@v1.6.1/mcp/server.go:753` — `st.handler(ctx, req)` is called directly). The handler must `defer recover()` itself to preserve SC2. |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/modelcontextprotocol/go-sdk` | v1.6.1 (locked in `go.mod`) | MCP SDK + tool registration + in-memory transport for tests | The milestone's only MCP choice (MCPPROC-01). `ToolHandler func(context.Context, *CallToolRequest) (*CallToolResult, error)` (`mcp/tool.go:30`) is the handler signature every Phase 08 tool uses. `[VERIFIED: go module cache — mcp/tool.go:30, mcp/transport.go:205-218]` |
| Go stdlib `net/http` | stdlib (Go 1.26) | Kamacu HTTP routes + streaming endpoint | The stdlib `http.ServeMux` (Go 1.22+ method/path patterns) already routes `/api/sessions/{id}`. Chunked streaming is built into `http.ResponseWriter` (an `http.Flusher` flushes mid-response). `[VERIFIED: internal/api/routes.go, internal/api/sessions.go:33-37]` |
| `github.com/google/uuid` | already in `go.mod` | Per-request `connID` for `Session.Attach` | Already used by `internal/ws/handler.go:67` for the same purpose. `[VERIFIED: internal/ws/handler.go:9,67]` |
| `encoding/base64` | stdlib | D-05 base64 envelope encoding for raw PTY bytes | Stdlib; no choice to make. `[VERIFIED: training knowledge, stdlib]` |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/coder/websocket` | v1.8.14 (in `go.mod`) | Already used by `internal/ws` for the browser terminal | **NOT recommended for the live-tail transport** (D-11 discretion). coder/websocket's value is bidirectional, multiplexed, browser-facing. Subscribe is single-consumer, one-way, process-to-process. Plain HTTP chunked streaming is simpler and avoids the WS handshake + frame protocol for a one-shot read. `[VERIFIED: internal/ws/handler.go:8 — the only legitimate caller]` |
| `log/slog` | stdlib | Structured logging to stderr | Project-wide convention (Phase 06 pins `slog.SetDefault(slog.NewTextHandler(os.Stderr, nil))` at `internal/mcp/server.go:49`). `[VERIFIED: internal/mcp/server.go:49]` |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Plain HTTP chunked streaming for live tail | SSE-shaped response (`text/event-stream`) | SSE adds a wire format (`data: ...\n\n`) and an `id:`/`event:` discipline that buys nothing for a single binary base64 stream consumed by one Go process. SSE is for browser event-push; the bridge is a Go HTTP client. **Plain chunked HTTP wins** — fewer moving parts, the bridge already speaks HTTP, and the envelope (D-08) carries metadata once at the end. `[ASSUMED — based on transport-shape analysis, not external sources]` |
| Plain HTTP chunked streaming | Tailored read-only WS variant (coder/websocket) | coder/websocket is already a dep, but using it for subscribe means: a WS handshake, the WS frame protocol, `OriginPatterns` config that makes no sense for a server-to-server loopback call, and `websocket.Accept` machinery. For a single-consumer, one-way, bounded-duration read, that is overkill. **Plain chunked HTTP wins.** `[ASSUMED]` |
| Base64-encoded PTY bytes (D-05) | ANSI-stripped text | Rejected by D-05/CONTEXT.md — a TUI's redraw bytes are messy even after stripping; lossy transformation implies false cleanliness (mirrors D-M001-2 posture). Base64 is faithful to arbitrary bytes. **Base64 stays.** `[VERIFIED: 08-CONTEXT.md D-05]` |

**Installation:**
```bash
# NO new packages — Phase 08 uses only what Phase 06/07 already installed.
# go.mod already contains:
#   github.com/modelcontextprotocol/go-sdk v1.6.1
#   github.com/coder/websocket v1.8.14
#   github.com/google/uuid (for connID)
# All other deps are stdlib.
```

**Version verification:** Already verified against `go.mod` and `go.sum` — Phase 08 adds zero dependencies.

## Package Legitimacy Audit

> Phase 08 installs NO external packages. All dependencies (`go-sdk` v1.6.1, `coder/websocket` v1.8.14, `google/uuid`) were installed by Phase 06 and verified there. **Skip — no new packages to audit.**

## Architecture Patterns

### System Architecture Diagram

```
                                    PROCESS BOUNDARY
        Agent CLI (claude/opencode)    │              Kamacu binary
        running inside a Kamacu PTY    │              (one process, owns PTYs)
                                         │
  ┌────────────────────────────────┐     │     ┌────────────────────────────────────┐
  │ 1. Agent calls                 │     │     │ Kamacu HTTP routes (read-only)     │
  │    subscribe_session_output    │     │     │                                    │
  │    (over the agent's MCP       │     │     │ GET /api/sessions                  │
  │    config: kamacu mcp serve)   │     │     │   ?task_id=N | ?project_id=N (D-11)│
  │              │                 │     │     │ GET /api/sessions/{id}             │
  │              ▼                 │     │     │ GET /api/sessions/{id}/output      │
  │ 2. `kamacu mcp serve`          │     │     │   ?bytes=N                         │
  │    (subcommand = MCP server)   │     │     │ GET /api/sessions/{id}/subscribe   │
  │              │                 │     │     │   ?duration=N&include_history=1    │
  │   ToolHandler(ctx, req)        │     │     │              │                     │
  │              │                 │     │     │              ▼                     │
  │   subscribe handler:           │     │     │   Session.Attach(uuid connID)      │
  │   ┌─ HTTP GET (no 10s timeout) │─────┼─────┼──▶queue replay as msg[0] under lock│
  │   │  with ctx (from SDK)       │     │     │   then live chunks → chan []byte   │
  │   │                            │     │     │              │                     │
  │   │  accumulate chunks         │     │     │   pump() fans PTY chunks to        │
  │   │  cap at 1 MiB (most recent)│     │     │   ring + every attached queue      │
  │   │                            │     │     │              │                     │
  │   │  select {                  │     │     │   Session.Done() on exit           │
  │   │   <-stream done:           │     │     │              │                     │
  │   │   <-ctx.Done() (cancel):   │     │     │              ▼                     │
  │   │   <-time.After(duration):  │     │     │   HTTP handler: r.Context().Done() │
  │   │   <-sess.Done() (exit):    │     │     │   fires when bridge closes request │
  │   │  }                         │     │     │   → defer Session.Detach(connID)    │
  │   │                            │     │     │     (NEVER touches PTY)             │
  │   ▼ assemble D-08 envelope     │     │     │                                    │
  │   (base64 output + metadata)   │     │     │                                    │
  │              │                 │     │     │                                    │
  │   return (*CallToolResult, nil)│     │     │                                    │
  │              │                 │     │     │                                    │
  │ 3. SDK surfaces result to      │     │     │                                    │
  │    agent as the tool reply     │     │     │                                    │
  └────────────────────────────────┘     │     └────────────────────────────────────┘
                                         │
                  Cancellation flow (verified):
                  Agent CLI → notifications/cancelled (JSON-RPC)
                          → SDK canceller.Preempt (transport.go:205)
                          → conn.Cancel(id)
                          → handler ctx.Done() fires
                          → bridge closes HTTP request
                          → Kamacu r.Context().Done() fires
                          → defer Session.Detach(connID)
                          → SC3 "<100ms detach on cancel" satisfied
```

A reader can trace the primary use case: agent calls `subscribe_session_output` → MCP handler accumulates → ctx.Done() or duration/exit fires → envelope returned → Session.Detach runs via `defer`. The reverse direction (cancellation) is what makes SC3 testable.

### Recommended Project Structure
```
internal/api/
├── sessions.go              # EXTEND: ?project_id=N on list; ADD getSession, getSessionOutput, subscribeSessionOutput handlers
└── sessions_test.go         # EXTEND: happy + 404 + clamping tests for new handlers

internal/mcp/
├── sessions.go              # NEW (D-07 per-resource split): registerSessionTools + bridge.{list,get,getOutput,subscribe}Session methods
├── sessions_test.go         # NEW: per-tool happy + 404 + the SC3 cancellation/leak test (in-memory SDK transport + fake streaming server)
└── server.go                # EXTEND: registerTools calls registerSessionTools(s, b)

cmd/kamacu/serve.go          # NOT MODIFIED — SessionRoutes already wires /api/sessions/{id}/ws; new /api/sessions/{id}[/output][/subscribe] register inside SessionRoutes
```

### Pattern 1: Per-Resource File Split (D-07, repeating Phase 06/07)
**What:** One file per MCP resource owns its `register*Tools` function + `bridge.*` methods. `registerTools` calls each registrar once.
**When to use:** Every MCP resource added in v1.11. Phase 08 adds `internal/mcp/sessions.go`.
**Example:**
```go
// internal/mcp/server.go — registerTools (verified at server.go:82-86)
func registerTools(s *mcp.Server, b *bridge) {
    registerTaskTools(s, b)
    registerProjectTools(s, b)
    registerWorkspaceTools(s, b)
    registerSessionTools(s, b) // NEW — Phase 08
}
```
Source: `[VERIFIED: internal/mcp/server.go:82-86]`

### Pattern 2: Bridge HTTP call (3 non-streaming tools — repeat Phase 07 verbatim)
**What:** For `list_sessions`, `get_session`, `get_session_output`: extract args → build path/body → `b.call(ctx, method, path, body)` → Kamacu JSON returned verbatim as TextContent. Errors wrapped as `fmt.Errorf("kamacu %s %s: HTTP %d: %s", method, path, code, body)`.
**When to use:** Every non-streaming bridge call.
**Example:**
```go
// Source: [VERIFIED: internal/mcp/tasks.go:182-198 (listTasks), internal/mcp/bridge.go:88-106 (bridge.call)]
func (b *bridge) getSession(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    var args struct{ SessionID string `json:"session_id"` }
    if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
        return nil, fmt.Errorf("get_session: invalid arguments: %w", err)
    }
    return b.call(ctx, http.MethodGet, "/api/sessions/"+url.PathEscape(args.SessionID), nil)
}
```

### Pattern 3: Subscribe diverges from bridge.call (D-11 — the new pattern)
**What:** `subscribe_session_output` cannot use `bridge.call` (10s `http.Client.Timeout`, body-buffered). It needs a dedicated streaming HTTP client with NO short timeout (or a >300s timeout), reads the chunked response incrementally, accumulates with a 1 MiB cap keeping the most-recent bytes (D-04), and assembles its own D-08 envelope. The handler `select`s on the SDK's ctx, a duration timer, the streaming reader's done-signal, and `ctx.Done()` for cancellation.
**When to use:** Only for `subscribe_session_output`.
**Example shape (the handler skeleton):**
```go
// Conceptual — the agent finalizes the exact structure.
func (b *bridge) subscribeSessionOutput(ctx context.Context, req *mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
    // ... parse args (session_id, duration_seconds?, include_history?) ...
    // Cap duration: default 30s, hard cap 300s (MCPSESS-04).

    // Use a SEPARATE *http.Client — NOT b.client (which has Timeout: 10s).
    streamingClient := &http.Client{} // no Timeout, or Timeout > 300s + grace

    httpReq, _ := http.NewRequestWithContext(ctx, "GET",
        b.base+"/api/sessions/"+url.PathEscape(args.SessionID)+"/subscribe?duration=...&include_history=...", nil)
    httpReq.Header.Set("X-Kamacu-Token", b.token)
    resp, err := streamingClient.Do(httpReq)
    if err != nil { return nil, fmt.Errorf("kamacu subscribe: %w", err) }
    defer resp.Body.Close()
    if resp.StatusCode != 200 {
        // D-05 wrap (non-2xx)
    }

    // D-04 cap: accumulate up to 1 MiB, keep MOST RECENT bytes when over.
    var buf []byte
    const tailCap = 1 << 20
    truncated := false
    chunk := make([]byte, 8192)
    for {
        n, readErr := resp.Body.Read(chunk)
        if n > 0 {
            buf = append(buf, chunk[:n]...)
            if len(buf) > tailCap {
                buf = buf[len(buf)-tailCap:]
                truncated = true
            }
        }
        if readErr != nil { break } // EOF, ctx cancel, or server-closed
    }

    // D-08 envelope. The Kamacu streaming endpoint closes the response with
    // a trailing JSON line carrying the exit metadata; the bridge merges it.
    return assembleEnvelope(buf, truncated, exitInfo), nil
}
```

### Pattern 4: Kamacu streaming endpoint mirrors the WS attach→stream→detach (read-only half)
**What:** The Kamacu `/api/sessions/{id}/subscribe` handler is the WS handler with the readLoop removed. It attaches a fresh `uuid` connID, ranges the attach channel writing chunks to the response, defers `Detach`, and selects on `r.Context().Done()` (bridge closed) + `sess.Done()` (session exited) + a duration timer.
**When to use:** For the streaming Kamacu endpoint only.
**Example:**
```go
// Source: [VERIFIED: internal/ws/handler.go:64-82 — the canonical pattern]
// Subscribe handler shape:
func (h *sessionHandlers) subscribeSessionOutput(w http.ResponseWriter, r *http.Request) {
    sess, ok := h.mgr.Get(r.PathValue("id"))
    if !ok { writeError(w, 404, "session not found"); return }

    // Parse duration_seconds (default 30, cap 300) + include_history.
    flusher, _ := w.(http.Flusher)
    w.Header().Set("Content-Type", "application/octet-stream")
    w.WriteHeader(200)

    connID := uuid.NewString()
    q := sess.Attach(connID)
    defer sess.Detach(connID) // SC3: cleanup on EVERY return path

    // include_history=false (default): drain the FIRST message (ring replay)
    // — verified at session.go:346 that Attach queues replay as msg[0].
    if !includeHistory {
        <-q // discard replay
    }

    timer := time.NewTimer(duration)
    defer timer.Stop()
    for {
        select {
        case chunk, ok := <-q:
            if !ok { // session exited (queue closed by markExited — session.go:192-194)
                // write exit trailer
                return
            }
            w.Write(chunk)
            if flusher != nil { flusher.Flush() }
        case <-timer.C:
            return
        case <-r.Context().Done(): // bridge closed (ctx cancelled)
            return
        }
    }
}
```

### Pattern 5: Read-only enforcement via type-level isolation (D-14)
**What:** The new code paths (Kamacu endpoints + bridge handlers) consume ONLY `Snapshot/Attach/Detach/Done/Info` — never `WriteInput`, never a WS `FrameData` input frame. The contract is enforced by what is in scope at the call site, not by a runtime check.
**When to use:** Every Phase 08 file. The reviewer verifies by inspection.
**Why it works:** `Session.WriteInput` (`session.go:368`) is a public method, but the Kamacu `/api/sessions/{id}/subscribe` handler literally has no reason to call it (it's reading from the attach channel, not writing to the PTY). The bridge handler is in `internal/mcp/` and speaks HTTP — it has no access to `*session.Session` at all (the bridge cannot import `internal/session` per Phase 06's split).

### Anti-Patterns to Avoid
- **Subscribing via the WS endpoint.** The existing `GET /api/sessions/{id}/ws` (`internal/ws/handler.go`) is bidirectional and browser-targeted. A subscribe handler that reused it would have to either speak the WS frame protocol in the bridge (pointless for a one-way read) or somehow ignore the readLoop (the WS handler calls `sess.WriteInput` on `FrameData` — D-14 forbids any input path). **Use a new HTTP chunked endpoint.** `[VERIFIED: internal/ws/handler.go:164 — WriteInput on FrameData]`
- **Reusing `b.client` for subscribe.** `bridge.client.Timeout = 10 * time.Second` (`bridge.go:53`) — a 30s default subscribe times out at 10s. **Use a dedicated `*http.Client` with no timeout (or >300s).** `[VERIFIED: internal/mcp/bridge.go:53]`
- **Reusing `bridge.call` for subscribe.** It buffers the full body via `io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))` (`bridge.go:94`) — defeats the point of streaming and caps at 1 MiB before the tail can grow naturally. **Subscribe assembles its own envelope.** `[VERIFIED: internal/mcp/bridge.go:88-106]`
- **Adding a new background goroutine inside Kamacu.** The milestone rule forbids it. The subscribe reader is per-request (dies with the HTTP request), not background. The Kamacu handler must NOT spawn anything that outlives the request. `[VERIFIED: 08-CONTEXT.md §"Backend-only milestone"]`
- **Wrapping every Phase 08 tool in `defer recover()` individually.** SC2's stream-desync invariant matters for subscribe (long-running, panic surface area). The 3 non-streaming tools are short and low-risk. Either wrap only subscribe, or factor a `recoverToError(h ToolHandler) ToolHandler` wrapper applied to session tools. `[VERIFIED: go-sdk@v1.6.1/mcp/server.go:753 — SDK does NOT recover]`
- **Trusting `?bytes=N` from the agent without clamping.** A request for 10 MiB base64-expands to ~13 MiB and blows past `maxBodyBytes`. **Always clamp to the ~512 KiB cap server-side and emit the `clamped` flag.** `[VERIFIED: bridge.go:23 — maxBodyBytes = 1<<20]`
- **Trying to use the SPA's `/api/agents/status` for `list_sessions`.** It's a different listing — one-newest-per-task, includes post-restart "resumable" entries with `sessionId=""` and no live PTY (D-09). The 4 tools share one coherent session-id space — every id must be a real PTY-backed session. **Use `/api/sessions`.** `[VERIFIED: internal/api/agents.go:201-213 — sessionId:"" for post-restart entries]`

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Live PTY tail fan-out | A new pub/sub mechanism on `*session.Session` | `Session.Attach(uuid)` / `Detach(uuid)` | Already built (`session.go:342-363`), already used by `internal/ws/handler.go`. Subscribe is just another consumer. The pump already drops slow consumers safely (`session.go:140-142`). |
| Ring snapshot | A new "give me the last N bytes" method | `Session.Snapshot()` + slice last-N | Snapshot returns the full ring copy; `lastN := snap[max(0,len-N):]` is one line. MCPSESS-03's "no new long-lived state" is satisfied for free. |
| Cancellation propagation | A custom out-of-band cancel channel | The handler's `ctx` parameter (SDK-managed) | The SDK already cancels the ctx on `notifications/cancelled` (transport.go:215). Just `select` on `ctx.Done()`. |
| Per-conn unique ID | A counter or a hash | `uuid.NewString()` | Already the pattern in `internal/ws/handler.go:67`. Same library, same shape. |
| Streaming HTTP response flushing | Manual chunked-transfer encoding | `http.ResponseWriter` + `http.Flusher` interface assertion | Go's stdlib handles chunked encoding automatically when there is no `Content-Length`; `flusher.Flush()` pushes bytes to the wire. |
| MCP tool registration | The typed generic `mcp.AddTool[In, Out]` | The low-level `s.AddTool(*mcp.Tool, ToolHandler)` with `map[string]any` InputSchema | Phase 06 lock (verified at `internal/mcp/server.go:80-82`); consistent with Phase 07's 13 tools. |
| Base64 encoding | A custom binary-to-text transform | `encoding/base64.StdEncoding.EncodeToString` | Stdlib, correct for arbitrary bytes (including invalid UTF-8 PTY output). |

**Key insight:** Every primitive Phase 08 needs already exists. The phase is about **wiring a new read path** through existing primitives, not building new state. This is why MCPSESS-03 ("no new long-lived state") and the milestone's "no new background goroutines" rule are both satisfied by construction.

## Common Pitfalls

### Pitfall 1: The 10-second `http.Client.Timeout` kills subscribe silently
**What goes wrong:** A bridge handler that uses `b.client` (the bridge's `*http.Client`) for subscribe times out at exactly 10 seconds, every time, regardless of the requested duration.
**Why it happens:** `bridge.go:53` pins `Timeout: 10 * time.Second` for Phase 06's fail-fast posture (non-streaming tools).
**How to avoid:** `subscribe_session_output` uses a **separate** `*http.Client` with no Timeout (the SDK's ctx is the cancellation mechanism, not the client's). Document the divergence at the call site.
**Warning signs:** A subscribe test that always returns at ~10s; an agent CLI complaining that subscribe "hangs then errors" after exactly 10s.
Source: `[VERIFIED: internal/mcp/bridge.go:53]`

### Pitfall 2: The SDK does NOT recover tool-handler panics
**What goes wrong:** A panic in `subscribe_session_output` (e.g., nil-pointer on the streaming response) propagates up through `st.handler(ctx, req)` (`server.go:753`) and crashes the subcommand, corrupting the JSON-RPC stdio stream — SC2 violated.
**Why it happens:** The SDK calls handlers directly without a recover wrapper (verified: no `recover()` in non-test SDK code; `server.go:753` is the dispatch site).
**How to avoid:** Wrap the subscribe handler (or all session handlers) in `defer func() { if r := recover(); r != nil { err = fmt.Errorf("kamacu subscribe: panic: %v", r) } }()`. The handler must still return `(partialResult, nil)` or `(nil, err)` — the recover converts a panic to the latter.
**Warning signs:** A flaky subscribe crash; "stream desync" errors from the agent CLI; the subcommand exiting non-zero mid-session.
Source: `[VERIFIED: go-sdk@v1.6.1/mcp/server.go:743-760 — direct dispatch, no recover]`

### Pitfall 3: Forgetting to drain the replay message when `include_history=false`
**What goes wrong:** Subscribe returns the entire ring buffer (up to 1 MiB) plus the live tail — duplicating `get_session_output` and bursting past the 1 MiB cap.
**Why it happens:** `Session.Attach` queues the ring replay as the channel's FIRST message under the same lock that registers the queue (`session.go:346`). The WS handler wants this (browser reconnect needs replay). Subscribe (live-only default) does not.
**How to avoid:** When `include_history` is false, drain `<-q` once before entering the select loop. The drained message IS the replay (it's atomic with registration — same lock).
**Warning signs:** Subscribe returns ~1 MiB on the first call even with a 1-second duration; `truncated:true` on every live-only subscribe of a chatty session.
Source: `[VERIFIED: internal/session/session.go:342-353 — replay queued under same lock as registration]`

### Pitfall 4: `?project_id=N` JOIN done naively queries N times
**What goes wrong:** A list_sessions implementation that loops the manager's sessions, looks up each session's taskID's projectID one-by-one, and filters — N round-trips to SQLite per call.
**Why it happens:** `session.Info` has `taskID` but not `projectID` (D-10).
**How to avoid:** One SQL query: `SELECT id FROM tasks WHERE project_id = ?` → collect task IDs into a set → filter `mgr.List()` in-memory by `info.TaskID ∈ set`. Two total queries (or one if you do `WHERE project_id IN (...)` on the reverse).
**Warning signs:** A `list_sessions?project_id=1` call taking >100ms with many sessions.
Source: `[VERIFIED: internal/session/manager.go:414-423 — ListByTask is in-memory; no ListByProject]`

### Pitfall 5: Including orphaned tmux-survivor entries in the MCP listing
**What goes wrong:** `list_sessions` returns entries with `id=""` (the post-restart synthesized rows from `reconcileTmux` at `sessions.go:83`) — but the agent then can't `get_session("")`, can't subscribe, can't snapshot. The 4-tool contract ("every listed id is operable") is broken.
**Why it happens:** `/api/sessions` today synthesizes these rows so the SPA can offer "Reattach" — they have no live in-memory session.
**How to avoid:** Filter `info.Orphaned == false` (the `Orphaned` field at `session.go:66` is the marker) in either the Kamacu handler (when the request is from the bridge — but there's no clean way to tell) OR in the bridge handler after parsing the JSON. The latter is simpler and keeps Kamacu's SPA behavior unchanged.
**Warning signs:** An agent calling `get_session` on a `sessionId` from `list_sessions` and getting 404.
Source: `[VERIFIED: internal/api/sessions.go:135-143 — Orphaned:true, TmuxName set; session.go:66-67 — Orphaned/TmuxName fields]`

### Pitfall 6: Forgetting the `FrameData` PTY-input direction (D-14 type-level check)
**What goes wrong:** A reviewer assumes "no WS = no PTY write" — but D-14 is bidirectional: the new code paths must not just avoid `WriteInput`, they must avoid sending ANY `FrameData` input frame. If a future Phase 09+ adds a WS-based subscribe variant, this pitfall re-emerges.
**Why it happens:** `FrameData` ('0', `proto.go:16`) is bidirectional in the existing protocol — client→server is stdin, server→client is PTY output. The same byte means different things in different directions.
**How to avoid:** Phase 08's HTTP chunked streaming endpoint does NOT use the WS protocol at all — no `FrameData` frame is sent in either direction. The read-only contract is satisfied by transport choice, not by frame-type filtering.
**Warning signs:** Any new WS path that reuses `FrameData` for output; a comment that says "FrameData is output-only here" (it isn't, by spec).
Source: `[VERIFIED: internal/ws/proto.go:14-17 — FrameData is bidirectional]`

### Pitfall 7: A subscribe handler that becomes a slow consumer
**What goes wrong:** The Kamacu pump drops slow-consumer queues (`session.go:140-142` — `delete(s.conns, id); close(q)`). If the subscribe handler stalls (e.g., the HTTP write blocks because the bridge stopped reading), its queue is dropped and the channel closes mid-range — `ok == false` from `<-q` looks identical to "session exited".
**Why it happens:** The handler writes chunks to the HTTP response; if the client (bridge) isn't reading (e.g., the agent's ctx is cancelled but the cancellation hasn't propagated to the Kamacu request yet), the write blocks, the queue fills, the pump drops it.
**How to avoid:** Two mitigations: (a) the Kamacu handler's `select` on `r.Context().Done()` fires the moment the bridge closes the request — closing the response, unblocking the write, returning from the handler; (b) the bridge, on its own ctx.Done(), closes the HTTP response body (`resp.Body.Close()`), which propagates to the server as a closed connection. The 100ms SC3 budget assumes this propagation is fast (loopback TCP, no proxy).
**Warning signs:** A subscribe that returns "session exited" when the session is still running; an orphaned queue in `s.conns` after subscribe returns.
Source: `[VERIFIED: internal/session/session.go:140-142 — slow-consumer drop]`

### Pitfall 8: The `bytes?` cap is wrong because base64 inflates by 4/3
**What goes wrong:** A snapshot cap of 1 MiB "raw bytes" produces a base64-encoded result of ~1.33 MiB, which blows past the bridge's 1 MiB `maxBodyBytes` (`bridge.go:23`) — the snapshot returns as a transport error.
**Why it happens:** base64 encodes 3 bytes as 4 characters.
**How to avoid:** D-06's ~512 KiB raw cap is sized so `512 * 4/3 ≈ 683 KiB < 1 MiB`. Keep the cap at 512 KiB (or lower); do NOT bump it to 1 MiB without also raising `maxBodyBytes`. The cap is enforced server-side (clamp + `clamped:true` envelope flag).
**Warning signs:** A `get_session_output?bytes=1000000` returning a transport error; the bridge error message containing "HTTP 200" but the body being truncated.
Source: `[VERIFIED: 08-CONTEXT.md D-06; internal/mcp/bridge.go:23 — maxBodyBytes = 1<<20]`

## Code Examples

### Example 1: Tool registration (D-07 shape — verbatim from Phase 07)
```go
// Source: [VERIFIED: internal/mcp/tasks.go:29-48 + internal/mcp/server.go:82-86]
// internal/mcp/sessions.go
package mcp

import (
    "context"
    "github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerSessionTools(s *mcp.Server, b *bridge) {
    // MCPSESS-01: list_sessions
    s.AddTool(
        &mcp.Tool{
            Name:        "list_sessions",
            Description: "List live Kamacu terminal sessions (bash tabs AND agent sessions), optionally scoped by project_id or task_id. Read-only — every returned id is operable by get_session / get_session_output / subscribe_session_output.",
            InputSchema: map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "project_id": map[string]any{"type": "integer", "description": "Optional project id scope."},
                    "task_id":    map[string]any{"type": "integer", "description": "Optional task id scope."},
                },
            },
        },
        func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
            return b.listSessions(ctx, req)
        },
    )
    // ... get_session, get_session_output, subscribe_session_output ...
}
```

### Example 2: Verifying the SDK cancellation path (the SC3 test target)
```go
// Source: [VERIFIED: go-sdk@v1.6.1/mcp/mcp_example_test.go:108-164 — Example_cancellation]
// The shape of the SC3 cancellation/leak test:
func TestSubscribe_CancelledViaContext_DetachesAndReturnsPartial(t *testing.T) {
    ctx := context.Background()
    t1, t2 := mcp.NewInMemoryTransports() // [VERIFIED: transport.go:147]

    srv := mcp.NewServer(&mcp.Implementation{Name: "kamacu-test"}, nil)
    started := make(chan struct{})
    registerSessionTools(srv, testBridge) // points at a fake streaming httptest.Server
    serverSession, _ := srv.Connect(ctx, t1, nil)
    defer serverSession.Close()

    client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
    clientSession, _ := client.Connect(ctx, t2, nil)
    defer clientSession.Close()

    ctx, cancel := context.WithCancel(ctx)
    go func() {
        <-started
        cancel() // simulates the agent CLI sending notifications/cancelled
    }()
    res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
        Name: "subscribe_session_output",
        Arguments: map[string]any{"session_id": "xyz"},
    })
    // SC3: returns partial result, NOT an empty result, NOT a transport error
    // (the handler observes ctx.Done() and returns its accumulated buffer)
}
```

### Example 3: `Session.Attach` replay-first invariant (D-02 foundation)
```go
// Source: [VERIFIED: internal/session/session.go:342-353]
// Attach queues the ring replay as the channel's FIRST item, under the same
// lock that registers the queue. There is no gap between "I'm subscribed" and
// "the replay starts" — the WS handler relies on this, and so does subscribe.
//
// For include_history=false (default): drain the FIRST message before ranging.
// For include_history=true: keep the first message (replay + live tail).

// Wrong (live-only without draining):
//   for chunk := range q { ... } // first chunk is the FULL replay!

// Right (live-only):
<-q // drain the replay atomically
for chunk := range q { ... } // now only live output
```

### Example 4: The forbidden primitive (D-14 — what NOT to call)
```go
// Source: [VERIFIED: internal/session/session.go:365-381]
// WriteInput is the ONLY PTY-write primitive. The browser WS handler calls it
// on FrameData input frames (internal/ws/handler.go:164). Phase 08 MUST NOT
// call it — neither from the Kamacu streaming endpoint nor from anywhere in
// internal/mcp/.
//
// Read-only enforcement = "this method is never in scope at the call site":
//   - internal/mcp/sessions.go cannot import internal/session (Phase 06 split).
//   - The Kamacu /api/sessions/{id}/subscribe handler reads from the attach
//     channel; WriteInput is not needed for reading.
func (s *Session) WriteInput(p []byte) error {
    // ... writes to s.ptmx ...
}
```

### Example 5: The D-08 JSON envelope
```json
// Source: [VERIFIED: 08-CONTEXT.md D-08 — the locked envelope shape]
// Returned as TextContent for get_session_output AND subscribe_session_output.

// Snapshot (get_session_output):
{
  "encoding": "base64",
  "output": "<base64 of last N bytes>",
  "bytes": 4096,
  "clamped": false           // true if bytes? exceeded the ~512 KiB cap
}

// Subscribe (subscribe_session_output):
{
  "encoding": "base64",
  "output": "<base64 of accumulated tail>",
  "bytes": 23456,
  "truncated": false,         // D-04: true if the 1 MiB tail cap was hit
  "exited": true,             // D-03: true if Session.Done() fired mid-tail
  "exitCode": 0,              // D-03: present iff exited
  "stopRequested": false      // D-03: present iff exited (info.StopRequested)
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| gorilla/websocket | coder/websocket v1.8.14 | Phase 02 STACK decision | Concurrent-write-safe; the WS handler can have one writer goroutine without serialization. **Phase 08 does NOT extend this — uses plain HTTP for subscribe.** |
| Raw `database/sql` for sessions | (unchanged — still raw `database/sql`) | n/a | The session engine is in-memory (`*session.Manager`); SQLite is only for the task→project→agent JOIN (D-10). |
| Phase 06 bridge `http.Client{Timeout: 10s}` | Same for non-streaming tools; **new dedicated client for subscribe** | Phase 08 | Subscribe diverges from the bridge pattern for the first time. The divergence is documented at the call site. |
| Hand-built MCP tool registration | Same — `s.AddTool(*mcp.Tool, ToolHandler)` with `map[string]any` InputSchema | Phase 06 lock | No typed generics; consistent across Phases 06/07/08. |

**Deprecated/outdated:**
- The unscoped `xterm` npm package and `@xterm/addon-canvas` — irrelevant to Phase 08 (frontend concern), already documented in STACK.
- `gorilla/websocket` — already replaced project-wide by `coder/websocket`.
- The Phase 06 Open Question 1 (panic-recovery wrapping) — **Phase 08 resolves it**: subscribe introduces the panic surface; wrap session handlers with `defer recover()`.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Plain HTTP chunked streaming is the best transport for subscribe (vs SSE / WS) | Standard Stack / Alternatives | If a future need emerges for multi-consumer or bidirectional, the choice can be revisited — but for v1.11's single-consumer one-way bounded read, chunked HTTP is simplest. |
| A2 | Base64 should be applied inside Kamacu (Kamacu returns the full D-08 envelope) | Summary recommendation | If the bridge applied base64 instead, Kamacu would return raw octet-stream — inconsistent with the Phase 07 JSON-passthrough pattern. The bridge's JSON-passthrough is the established convention; Kamacu-side encoding keeps it. Low risk; reversible. |
| A3 | The exact 1 MiB tail cap and 512 KiB snapshot cap can share a single `const tailCap = 1 << 20` if the planner prefers | Common Pitfalls 8 | Both numbers are "~" in D-04/D-06. Sharing one const is cleaner. If they diverge later, split. |
| A4 | The `?project_id=N` JOIN should be done in Kamacu (one SQL query → in-memory filter) | Recommended Project Structure | Could also be done in the bridge (it has the project_id and could call `/api/projects/{id}/tasks` first). Kamacu-side is fewer round-trips. Low risk. |
| A5 | `defer recover()` wrapping should apply to ALL session tools, not just subscribe | Pattern 5 / Pitfall 2 | Wrapping all 4 keeps the panic posture uniform and matches Phase 06's belt-and-braces recommendation. Marginal overhead. The agent's discretion per CONTEXT.md. |
| A6 | The `Orphaned:true` filter happens in the bridge (not Kamacu) | Pitfall 5 | Could happen in Kamacu (e.g., a new `?live_only=true` query param). Bridge-side keeps Kamacu's SPA behavior unchanged. If the SPA later wants the same filter, promote to a query param. Low risk. |
| A7 | The slow-consumer drop by `pump()` is acceptable for subscribe (it surfaces as "stream ended early" — the agent can re-subscribe) | Pitfall 7 | If the agent CLI sees this as an error, the test will catch it. Acceptable for v1.11 single-user localhost. Documented in Pitfall 7. |

**If this table is empty:** N/A — 7 assumptions, all low-risk and reversible. None block planning.

## Open Questions (RESOLVED)

> All five questions below are substantively resolved by the Phase 08 plans. Each carries an explicit `RESOLVED:` marker noting which plan adopts its recommendation (and any deliberate divergence). Dimension 11 (research resolution) treats these as accepted-as-resolved.

1. **Does the bridge need to handle the partial-final-chunk case for base64?**
   - What we know: The Kamacu streaming endpoint writes raw PTY bytes in chunks; the bridge accumulates and base64-encodes the WHOLE accumulated buffer at the end (D-08).
   - What's unclear: None, really — base64 of the accumulated `[]byte` is a single operation. The only question is whether to base64-encode incrementally (no benefit) or once at the end (simpler).
   - Recommendation: Once at the end. The accumulated buffer is bounded by D-04's 1 MiB cap.
   - RESOLVED: Adopted by Plan 08-03 Task 1 — base64 is applied once at the end via `base64.StdEncoding.EncodeToString(buf)` on the accumulated buffer (D-04's 1 MiB cap bounds the input).

2. **Should the Kamacu streaming endpoint write the exit-trailer as a separate chunk, or close the response with the trailer inline?**
   - What we know: The HTTP chunked response is `application/octet-stream` raw bytes. The exit metadata (D-03's `exited/exitCode/stopRequested`) needs to reach the bridge somehow.
   - What's unclear: Two options — (a) Kamacu writes a trailing JSON line after the last byte chunk, and the bridge parses it out; (b) the bridge infers exit by the HTTP response closing and re-fetches `GET /api/sessions/{id}` for the final `Info().ExitCode`.
   - Recommendation: Option (a) — write a trailing JSON line (delimited by a sentinel byte or a length prefix). Avoids a second round-trip. The agent finalizes the exact framing. `[ASSUMED]`
   - RESOLVED: Resolved by Plan 08-03 Task 1 — **option (b) follow-up GET adopted over option (a)**. The plan deliberately diverges from the research recommendation: option (b) avoids introducing sentinel/length-prefix framing into the raw octet-stream (which would complicate the D-08 envelope contract), and reuses the existing `GET /api/sessions/{id}` endpoint delivered by Plan 08-01 (no new Kamacu-side trailer logic). The extra round-trip is one loopback GET; acceptable for v1.11 single-user localhost.

3. **Should subscribe's bridge handler use `io.ReadAll` or read incrementally?**
   - What we know: `bridge.call` uses `io.ReadAll(io.LimitReader(...))` (`bridge.go:94`) — buffers the full body. Subscribe needs incremental reads with the D-04 cap applied incrementally.
   - What's unclear: Whether to read fixed-size chunks (8 KiB) and trim-front on each read, or to read into a growing buffer and trim once at EOF.
   - Recommendation: Fixed-size chunks (8 KiB) with trim-front when `len(buf) > tailCap`. Bounds memory tightly; the trim cost is amortized.
   - RESOLVED: Adopted verbatim by Plan 08-03 Task 1 — fixed-size 8 KiB chunks (`chunk := make([]byte, 8192)`) with trim-front on overflow (`buf = buf[len(buf)-subscribeTailCap:]`).

4. **Should the SC3 cancellation/leak test use a real `httptest.Server` or a fake in-memory transport?**
   - What we know: The SDK provides `mcp.NewInMemoryTransports()` (`transport.go:147`) for end-to-end tool testing without stdio. The bridge's HTTP client needs a real `httptest.Server` to exercise the streaming path.
   - What's unclear: Whether the test should be (a) end-to-end (in-memory MCP transport + httptest streaming server + bridge handler) or (b) two-layer (bridge handler test + separate SDK-level cancellation test).
   - Recommendation: Both. The bridge handler test (httptest + bridge.subscribeSessionOutput) proves the HTTP-streaming cancellation. A second test using `NewInMemoryTransports` proves the SDK-level ctx propagation through to a handler (the shape of `Example_cancellation`). Together they cover SC3. `[VERIFIED: go-sdk@v1.6.1/mcp/mcp_example_test.go:108-164]`
   - RESOLVED: Adopted by Plan 08-03 Task 2 — both layers are present. Bridge-level tests (TestBridge_Subscribe_*) use httptest + `&bridge{base,token,client}`; the SC3 gate (TestSubscribe_CancelledViaContext_ReturnsPartialAndDetaches) and the leak gate (TestSubscribe_GoroutineStabilityAcrossCycles) use `mcp.NewInMemoryTransports` for SDK-level ctx propagation.

5. **The `cmd/kamacu/serve.go` route wiring — is anything beyond `SessionRoutes` needed?**
   - What we know: `serve.go:261` already calls `api.SessionRoutes(mux, mgr, db, tmuxClient)`. The new `/api/sessions/{id}`, `/api/sessions/{id}/output`, `/api/sessions/{id}/subscribe` routes register INSIDE `SessionRoutes` (they all join the existing `sessionHandlers` struct that already holds `mgr + db`).
   - What's unclear: Nothing — confirmed by reading `internal/api/sessions.go:31-38`. `serve.go` is NOT modified.
   - Recommendation: None needed; this is a closed question. `[VERIFIED: internal/api/sessions.go:31-38, cmd/kamacu/serve.go:261]`
   - RESOLVED: Closed question, confirmed by Plan 08-01 — all three new routes register inside `SessionRoutes` (sessions.go:31-38); `cmd/kamacu/serve.go` is not modified.

## Environment Availability

> Phase 08 has NO external dependencies beyond what Phase 06/07 already verified. All work is Go code against existing in-repo packages and the already-installed SDK.

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | All code | ✓ | go1.26.0 (verified `go version`) | — |
| `github.com/modelcontextprotocol/go-sdk` | MCP server + cancellation + in-memory transport for tests | ✓ | v1.6.1 (in `go.mod`) | — |
| `github.com/coder/websocket` | (NOT used by Phase 08 subscribe — plain HTTP) | ✓ | v1.8.14 | n/a |
| `github.com/google/uuid` | connID for `Session.Attach` | ✓ | (in `go.mod`) | — |
| `database/sql` + `*sql.DB` | The D-10 task→project→agent JOIN | ✓ | stdlib | — |

**Missing dependencies with no fallback:** None.
**Missing dependencies with fallback:** None needed.

## Security Domain

> Required when `security_enforcement` is enabled (absent = enabled). The config has no `security_enforcement` key, so this section is included. Phase 08 introduces no new auth and no new untrusted-input surfaces beyond what Phase 06 already established.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | None — loopback-only, no auth on `/api/*` (Phase 06 D-06). The new session endpoints inherit the loopback-binding auth boundary unchanged. |
| V3 Session Management | no | MCP sessions are process-lifetime; no auth tokens, no cookies. The Kamacu `*session.Manager` sessions are PTY-backed, not HTTP sessions. |
| V4 Access Control | yes (read-only) | **Type-level** (D-14): the new code paths cannot reach `WriteInput` or send a `FrameData` input frame. No runtime check needed; the contract is enforced by what's in scope. |
| V5 Input Validation | yes | `session_id` is `url.PathEscape`d before path interpolation; `bytes?` is `strconv.Atoi` + clamp to ~512 KiB; `duration_seconds?` is `strconv.Atoi` + clamp to [1, 300]; `project_id`/`task_id` are `strconv.ParseInt`. |
| V6 Cryptography | no | No crypto in Phase 08. The `KAMACU_HOOK_TOKEN` is sent as-is on `X-Kamacu-Token` (Phase 06 contract); Phase 08 does not change this. |
| V7 Error Handling | yes | Panic recovery (`defer recover()`) on subscribe (Pitfall 2). All Kamacu endpoints return JSON errors via `writeError`. |
| V8 Data Protection | no | Terminal output IS sensitive (could contain secrets the user typed). Mitigation: it is delivered ONLY over loopback HTTP to a process the user spawned. No logging of output bytes. |
| V13 API & Web Service | yes | REST conventions; `GET` for read-only (snapshot, list, get); the streaming endpoint is also `GET` (idempotent, read-only). No `POST`/`PUT`/`DELETE` introduced. |

### Known Threat Patterns for the MCP-bridge-over-loopback-HTTP stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| PTY keystroke injection via the new endpoint | Tampering / Elevation of Privilege | **Type-level read-only** (D-14): no `WriteInput` call path exists in the new code. Reviewed by inspection. |
| Snapshot size DoS (huge `bytes?`) | Denial of Service | Server-side clamp to ~512 KiB (Pitfall 8); `clamped:true` flag in the envelope. |
| Subscribe duration DoS (huge `duration_seconds?`) | Denial of Service | Server-side clamp to 300s hard cap (MCPSESS-04); the SDK ctx cancel is the real backstop. |
| Subscribe memory DoS (unbounded accumulation) | Denial of Service | D-04: 1 MiB tail cap with most-recent-bytes retention; `truncated:true` flag. |
| Goroutine leak on cancel | Denial of Service | `defer sess.Detach(connID)` on every return path (SC3); request-scoped reader goroutine (dies with the HTTP request). The SC3 leak test is the regression gate. |
| JSON-RPC stream pollution from panic | Tampering | `defer recover()` in the subscribe handler (Pitfall 2); converts panic to `(nil, err)` per SC2's pattern. |
| Untrusted terminal output treated as instructions | (Prompt injection via PTY bytes) | The agent CLI is the consumer; base64-encoded raw bytes are DATA, not instructions. The MCP `TextContent` payload is a string the agent reads, not executes. Documented in the envelope (`"encoding":"base64"`). |

## Sources

### Primary (HIGH confidence — verified by direct read of source)
- **`internal/session/session.go`** — `Snapshot()` (387), `Attach()` (342), `Detach()` (356), `Done()` (395), `Info()` (199), `pump()` (125), `WriteInput()` (368), `markExited()` (184). The Phase 4 seam comment at 383-386.
- **`internal/session/manager.go`** — `Get` (405), `List` (414), `ListByTask` (421), `HasLiveTmux` (393), `listWhere` (427).
- **`internal/api/sessions.go`** — `SessionRoutes` (31), `sessionHandlers` struct (40), `list` (50), `reconcileTmux` (83).
- **`internal/api/agents.go`** — `agentStatusEntry` (26), `status` JOIN at 99-103 (`tasks → projects → agents`), post-restart entries at 201-213.
- **`internal/api/respond.go`** — `writeJSON` (9), `writeError` (16).
- **`internal/api/resume.go`** — `defaultTranscriptGlobRoot` (30).
- **`internal/api/routes.go`** — `Routes` registration (18-46).
- **`internal/ws/handler.go`** — `ServeHTTP` attach→stream→detach pattern (39-82), `writeLoop` (89), `readLoop` `WriteInput` call (164).
- **`internal/ws/proto.go`** — `FrameData` ('0', bidirectional, 16), `FrameResize` ('1'), `FrameExit` ('x').
- **`internal/mcp/bridge.go`** — `bridge` struct (31), `do` (60), `bridge.call` (88), `maxBodyBytes = 1<<20` (23), `http.Client{Timeout: 10s}` (53).
- **`internal/mcp/server.go`** — `registerTools` (82), `slog.SetDefault` to stderr (49), `ServeCommand.Execute` (46).
- **`internal/mcp/tasks.go`** — `registerTaskTools` (29), `listTasks` (182) — the per-resource template.
- **`internal/mcp/tasks_test.go`** — `newCallToolRequest` helper, the httptest+bridge test shape.
- **`internal/mcp/server_test.go`** — `TestSC2_*` tests via `mcp.NewInMemoryTransports()` (the SC2 regression pattern Phase 08 inherits for SC3).
- **`cmd/kamacu/serve.go:261`** — `api.SessionRoutes(mux, mgr, db, tmuxClient)`; the WS handler registration at 275.
- **`go-sdk@v1.6.1/mcp/tool.go:30`** — `ToolHandler func(context.Context, *CallToolRequest) (*CallToolResult, error)`.
- **`go-sdk@v1.6.1/mcp/transport.go:127-149`** — `InMemoryTransport`, `NewInMemoryTransports`.
- **`go-sdk@v1.6.1/mcp/transport.go:200-218`** — `canceller.Preempt` — proves `notifications/cancelled` → `conn.Cancel(id)` → handler ctx cancellation.
- **`go-sdk@v1.6.1/mcp/transport.go:220-253`** — `call` — the client-side ctx-cancel → `Notify(notificationCancelled)` path.
- **`go-sdk@v1.6.1/mcp/protocol.go:1607`** — `notificationCancelled = "notifications/cancelled"`.
- **`go-sdk@v1.6.1/mcp/protocol.go:175-191`** — `CancelledParams` with `RequestID`.
- **`go-sdk@v1.6.1/mcp/server.go:743-760`** — `Server.callTool` dispatches `st.handler(ctx, req)` directly with NO recover wrapper.
- **`go-sdk@v1.6.1/mcp/mcp_example_test.go:108-164`** — `Example_cancellation` proves a handler observing `ctx.Done()` can return cleanly.
- **`08-CONTEXT.md`** — All D-01..D-14 locked decisions; the deferred-ideas list; the canonical references.

### Secondary (MEDIUM confidence)
- `06-RESEARCH.md` lines 530, 754-759 — the deferred panic-recovery Open Question Phase 08 resolves.
- `07-CONTEXT.md` D-05/D-07/D-08 — the bridge error-wrap, per-resource split, and test-count templates Phase 08 repeats.

### Tertiary (LOW confidence)
- None. Every load-bearing claim was verified against the SDK source or Kamacu source.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — no new deps; everything verified in `go.mod` and the module cache.
- Architecture: HIGH — every code seam (`Session.Attach/Detach/Done/Snapshot/Info`, `bridge.call`, `routes.go`, the WS handler pattern) read directly; the SDK cancellation path verified end-to-end through `transport.go` and `mcp_example_test.go`.
- Pitfalls: HIGH — each pitfall cites the file:line where the trap originates.
- Subscribe transport choice (chunked HTTP vs SSE vs WS): MEDIUM — based on architectural reasoning, not external sources (the `[ASSUMED]` tag in the Alternatives table reflects this).

**Research date:** 2026-07-23
**Valid until:** 2026-08-23 (30 days — stable; the SDK is locked at v1.6.1, the codebase is at HEAD, and the milestone is in active execution. The only fast-moving variable is whether Phase 09 planning reveals a need to revisit the transport shape, which would be a Phase 09 concern.)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Live-tail delivery model (D-01..D-04) — the risk center
- **D-01:** `subscribe_session_output` is **collect-and-return** — ranges the session's attach-channel output for the bounded `duration_seconds` (default 30s, hard cap 300s per MCPSESS-04), OR until `ctx.Done()` fires (the SDK cancels the handler ctx on `notifications/cancelled` — verified in the SDK's `Example_cancellation`), accumulating every chunk. Returns ONE `CallToolResult` (a single TextContent blob) at the end. MCP progress notifications are NOT used to carry PTY output. (The SDK does support `req.Session.NotifyProgress`; we deliberately do not use it for output transport.)
- **D-02:** An `include_history?` boolean arg (default `false` = live-only) controls ring-buffer replay. `Session.Attach(connID)` queues the entire ring replay as the channel's FIRST message. Live-only (default): handler attaches, drains+discards that first replay message, then collects ONLY output produced after subscribe started. `include_history=true`: replay is kept, tail = replay + live.
- **D-03:** On session exit mid-tail (`Session.Done()` fires), stop collecting immediately, return bytes gathered so far PLUS a structured note that the session exited (exit code if available, whether server-requested stop via `Info().StopRequested`).
- **D-04:** Cap the accumulated buffer at ~1 MiB; keep the MOST RECENT bytes; append a `truncated:true` flag in the result envelope when the cap is hit.

#### Terminal-output encoding (D-05..D-08)
- **D-05:** Deliver raw PTY bytes base64-encoded. The ring buffer holds arbitrary bytes (ANSI escapes, control codes, possibly invalid UTF-8). Base64 is faithful and safe over JSON.
- **D-06:** `bytes?` = the LAST N bytes of the ring (most recent output); default 4096 (locked by MCPSESS-03); hard cap ~512 KiB raw (so base64-encoded ~683 KiB fits the bridge's 1 MiB `maxBodyBytes`).
- **D-07:** Base64 is applied so the result fits the bridge ceiling; no bypass of `maxBodyBytes` for snapshots.
- **D-08:** The result is a JSON envelope returned as TextContent. Shape:
  ```
  { "encoding":"base64", "output":"<base64>", "bytes":<int>,
    "truncated":<bool>,                 // D-04 tail cap OR D-06 snapshot clamp hit
    "exited":<bool>, "exitCode":<int?>, "stopRequested":<bool?> }  // D-03 subscribe only
  ```

#### list_sessions / get_session scope & endpoints (D-09..D-11)
- **D-09:** `list_sessions` enumerates ALL live in-memory terminal sessions (bash tabs AND agent sessions) via `/api/sessions` — NOT `/api/agents/status`. Every id returned is a real PTY-backed session operable by the other 3 tools. Each entry carries `session.Info.Status` (running/exited) AND, for `kind=="agent"`, `Info.AgentStatus` (working/waiting/idle/exited).
- **D-10:** Both `list_sessions` and `get_session` entries JOIN task/project/agent context (`tasks → projects → agents` — same JOIN `/api/agents/status` already does at `agents.go:99`) to surface `taskTitle`/`projectName`/`agentName`. The agent sees full context in one call.
- **D-11:** New endpoints on Kamacu (all read-only, all under `/api/sessions`):
  - `GET /api/sessions/{id}` — `get_session`.
  - `GET /api/sessions/{id}/output?bytes=N` — `get_session_output`.
  - A bounded-duration streaming endpoint backing `subscribe_session_output` per D-01..D-04. **Transport shape is the agent's discretion.** MUST: (a) attach via `Session.Attach(uniqueConnID)`; (b) for live-only, drain the replay first; (c) stream chunks to the response as they arrive; (d) on client disconnect (`r.Context().Done()`) call `Session.Detach(connID)` within ~100ms (SC3); (e) honor the duration cap server-side as a backstop. The bridge CANNOT use its 10s-timeout `http.Client` or the `bridge.call` helper. Request-scoped goroutines only; NO new background/long-lived goroutines inside Kamacu.
  - `?project_id=N` query param on the existing `GET /api/sessions` (project-scoping needs a task→project JOIN).

#### Dead-session & edge behavior (D-12..D-13)
- **D-12:** Tools OPERATE on EXITED in-memory sessions — read their final output. `Snapshot()` always works; `Attach()` post-exit returns the replay then closes. `get_session` returns `status=exited`+`exitCode`; `get_session_output` returns the final ring snapshot; `subscribe_session_output` returns immediately (live-only: empty tail + exit marker; include_history=true: replay + exit marker).
- **D-13:** `list_sessions` FILTERS OUT orphaned tmux-survivor entries (`Orphaned:true` rows synthesized by `reconcileTmux`); unknown/gone `session_id` → 404 → bridge wraps per Phase 07 D-05.

#### Read-only enforcement (D-14)
- **D-14:** The read-only contract is enforced at the type level, not by convention. New Kamacu endpoints + MCP `sessions.go` handlers consume ONLY `Session.Snapshot()` / `Session.Attach()` / `Session.Detach()` / `Session.Done()` / `Session.Info()` — NEVER `Session.WriteInput` and NEVER a WS `FrameData` PTY-input frame. SC4's "a wrapper with no public Write method" means the code path literally has no access to a write primitive.

### the agent's Discretion
- **Live-tail HTTP transport shape** — chunked HTTP streaming, SSE-shaped, or tailored read-only WS variant. Constraints in D-11 are locked; the wire shape is the agent's call. coder/websocket is already a dep but plain HTTP chunked is likely simpler.
- **Exactly where base64 is applied** — Kamacu returns raw octet-stream and the bridge base64-encodes into the JSON envelope, vs Kamacu returns the full JSON envelope with base64 already inline. Whichever is consistent with the bridge's JSON-passthrough pattern.
- **`?project_id=N` JOIN implementation** — query the project's task ids then filter in-memory sessions, vs a single SQL pass. Pick whichever is cleanest given `manager.List()` is in-memory.
- **The exact ~512 KiB / ~1 MiB constants** — D-06/D-04 say "~"; the precise numbers are the agent's. Keep both comfortably under the base64-of-1 MiB budget.
- **`internal/mcp/sessions.go` handler + test layout** — mirror Phase 07's per-resource shape. Subscribe needs a cancellation/leak test (SC3) using the SDK's in-memory transport + a fake streaming server.
- **Exact InputSchema map shape per tool** — property maps, descriptions, required-arrays. Phase 07's flat `map[string]any` template applies.
- **SDK panic-recovery wrapping** — Phase 06 deferred `defer recover()` to Phase 08 (06-RESEARCH Open Question 1) because subscribe introduces the streaming panic surface. Whether to wrap every session handler or just the streaming one is the agent's call; SC2's stream-desync invariant must hold.

### Deferred Ideas (OUT OF SCOPE)
- MCP progress-notification streaming of raw PTY bytes / liveness heartbeats — D-01 chose collect-and-return.
- Headless-VT-emulator clean-text snapshots — `Session.Snapshot()` returns raw bytes today (D-05); a clean text snapshot behind the same signature is a future swap the Phase 4 seam comment (`session.go:384`) anticipates.
- Full-ring (1 MiB) snapshots — D-06 caps at ~512 KiB so base64 fits the bridge ceiling without a bypass.
- `include_history` defaulting to true — D-02 defaults false to avoid duplicating `get_session_output`.
- MCPAUTO-02 (`get_my_session`) — per-task auto-scoping; v1.11 agents pass explicit `session_id`.
- MCPHARD-01..03 — typed bridge error taxonomy, stdout-pollution guards, real-binary e2e harness.
- Subscribe over WebSocket — technically possible (coder/websocket is a dep); D-11 leaves transport to the agent.
- Agent-CLI auto-registration (MCPREG) — v1.11 users manually add `kamacu mcp serve`.

</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| MCPSESS-01 | `list_sessions(project_id? or task_id?)` returns sessions with status (working / waiting / idle / exited / running) | `?project_id=N` JOIN (D-11); `?task_id=N` (existing); `session.Info` carries both `Status` (running/exited) and `AgentStatus` for agents (D-09); orphaned-row filter (D-13, Pitfall 5); reuse `agentHandlers.status` JOIN shape (`agents.go:99-103`, Pattern D-10). |
| MCPSESS-02 | `get_session(session_id)` returns full session detail (status, task, project, agent, started_at) | New `GET /api/sessions/{id}` endpoint; `session.Info()` (`session.go:199`) is JSON-ready; the D-10 JOIN adds taskTitle/projectName/agentName. Dead sessions still readable (D-12). |
| MCPSESS-03 | `get_session_output(session_id, bytes?)` returns a snapshot of the session's PTY ring buffer (last N bytes, default 4 KB) — read-only | `Session.Snapshot()` (`session.go:387`) — read-only on existing 1 MiB ring (NO new long-lived state). Slice last-N; default 4096; cap ~512 KiB (D-06, Pitfall 8). D-05 base64. D-08 JSON envelope. New `GET /api/sessions/{id}/output?bytes=N`. |
| MCPSESS-04 | `subscribe_session_output(session_id, duration_seconds?)` tails live PTY output for a bounded duration (default 30s, cap 300s); cancellation propagates via `ctx.Done()` → `Session.Detach`; no PTY keystroke injection (read-only) | Collect-and-return (D-01) verified against SDK cancellation (`transport.go:205-218` + `Example_cancellation`). `Session.Attach/Detach/Done` (`session.go:342/356/395`). Include_history arg (D-02). 1 MiB tail cap (D-04). Exit marker (D-03). New `/api/sessions/{id}/subscribe` streaming endpoint (Pattern 3 + 4). Read-only enforced at type level (D-14). SC3 detach-within-100ms via `defer Detach` + ctx propagation. The SC3 leak test (Example 2). |

</phase_requirements>
