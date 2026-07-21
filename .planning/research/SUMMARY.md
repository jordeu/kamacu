# Project Research Summary

**Project:** Kamacu — v1.11 Kamacu MCP Server
**Domain:** MCP (Model Context Protocol) stdio bridge subcommand added to an existing local-only Go single-binary app, plus spawn-time agent-CLI registration
**Researched:** 2026-07-21
**Confidence:** HIGH

> **Naming note.** The research files consistently call the product "Kamacu" (binary `kamacu`); the repo directory and broader project context call it "Kangent" (`kangent`). The discrepancy appears to be a research-side rename for this milestone. This SUMMARY uses "Kamacu" throughout to match the four source files and the milestone brief verbatim; downstream agents reading the research files will see consistent naming.

---

## Executive Summary

v1.11 adds a `kamacu mcp serve` stdio subcommand to the existing Kamacu binary so that agents running inside Kamacu task PTYs can drive the entire app as the user's delegate. The architecture is overwhelmingly **additive**: one new subcommand, two new leaf packages (`internal/mcp` for the server, `internal/mcpconfig` for spawn-time config writers), two new read-only HTTP endpoints on the existing Kamacu server (`/api/sessions/{id}/snapshot`, `/api/sessions/{id}/tail`) to expose the existing ring buffer to the bridge, and one new step in the v1.10 spawn engine. **No migrations, no DB schema changes, no frontend changes, no new long-lived goroutines inside the Kamacu binary.** Every tool handler is a thin HTTP client call — the existing validation, side effects, reaper, and hooks all apply unchanged.

The recommended approach is settled and unanimous across all four research tracks: **use the official `github.com/modelcontextprotocol/go-sdk` v1.6.1**, the only MCP Go SDK that is v1.x stable-tagged, conformance-tested against the official suite, and shipping a documented proxy example (`examples/server/proxy/main.go`) that is structurally identical to Kamacu's stdio→HTTP bridge use case. The three-process model is forced by MCP semantics — the agent CLI is the MCP *client*; `kamacu mcp serve` is the MCP *server* it spawns; the long-running Kamacu binary is the *upstream HTTP API* the server bridges to. Per-task identity rides on the existing `KAMACU_SESSION_ID` / `KAMACU_HOOK_TOKEN` / `KAMACU_HOOK_BASE` env injection (v1.10), not on per-task config files. The MCP surface is **tools-only** for v1.11 (resources, prompts, completion deferred to v1.12+) and **read-only for terminals by contract** — no keystroke injection, ever.

The dominant risks are preventable and concentrated in three places. **(1) Stdout pollution** is the #1 way MCP servers die: any non-JSON byte on stdout desyncs the JSON-RPC stream — redirect stdout, pin every logger to stderr, ban `fmt.Print*` from the subcommand package. **(2) Token leakage**: the `KAMACU_HOOK_TOKEN` must live in a single bridge struct constructed once at startup and never enter tool closures, logs, or error strings — the agent IS the LLM, so any leaked token lands in its context window and is exfiltrable via prompt injection. **(3) Long-running subscribe goroutine leaks**: the `subscribe_session_output` tool is the milestone's risk center — `ctx.Done()` must translate to `session.Detach` immediately, duration must be capped well below Claude Code's 30-min stdio idle timeout, and a goroutine-leak test must gate the phase. Secondary risks — bridge-to-HTTP error taxonomy (6 typed codes not wrapped errors), agent-CLI config-write races (project-scope only, read-modify-write + flock), and process lifecycle (zombie subcommands) — each have documented mitigations landing in their dedicated phases.

---

## Key Findings

### Recommended Stack

**One new dependency, four small transitive deps, all pure Go, no CGO.** The stack story for v1.11 is a single `go get` and `go mod tidy`. Nothing else changes — no frontend additions (v1.11 is backend-only), no schema migration, no new HTTP server, no new goroutine in the Kamacu binary.

**Core stack addition:**

| Package | Version | Purpose | Integration Points | Why This One |
|---------|---------|---------|--------------------|--------------|
| `github.com/modelcontextprotocol/go-sdk` (+ `/mcp`) | **v1.6.1** (stable, May 22 2026) | The MCP SDK — server, handlers, stdio transport | (1) `cmd/kamacu/mcp_subcommand.go` calls `mcp.Run`; (2) `internal/mcp/server.go` uses `mcp.NewServer` + `mcp.AddTool` typed handlers + `mcp.StdioTransport`; (3) `internal/mcp/tools_*.go` per-domain handlers using `func(ctx, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error)`; (4) `internal/mcp/tools_terminal.go` uses `req.Session.NotifyProgress` + `ctx.Done()` for the subscribe tool. | Official, modelcontextprotocol org, Apache-2.0/MIT, conformance-tested, v1.x stable-tagged. The SDK's `examples/server/proxy/main.go` is structurally identical to Kamacu's stdio→HTTP bridge. Tracks the in-flux spec (pre-release v1.7.0-pre.3 already supports the in-progress `2026-07-28` rewrite). `go.mod` requires `go 1.25.0`; Kamacu's `go 1.26` satisfies it. |

**Transitive deps (auto-pulled, pure Go):** `google/jsonschema-go` v0.4.3 (auto-derives tool input/output schemas from struct tags), `yosida95/uritemplate/v3` v3.0.2 (resource templates — not used in v1.11 tools-only scope), `segmentio/encoding` v0.5.4 (SDK-internal JSON), `golang.org/x/tools` v0.42.0 (SDK codegen). The SDK's `auth`/`oauthex` sub-packages reference `golang-jwt/jwt/v5` and `golang.org/x/oauth2` — they appear in `go.mod` but **are not compiled into a stdio-only binary** (lazy package loading); do not import them.

**What NOT to add (explicit rejections):**

| Rejected | Why Rejected |
|----------|--------------|
| `github.com/mark3labs/mcp-go` v0.56.0 | Three reasons. (1) **Pre-v1 unstable-tagged** — the author reserves the right to break APIs between minor versions; Kamacu would be opting into churn. (2) **Its distinguishing features are dead weight in a localhost stdio bridge**: CORS, OAuth Protected Resource Metadata, DNS-rebinding protection, OpenTelemetry tracing, panic recovery, task-augmented tools — all aimed at HTTP/SaaS deployments. (3) **Behind on the spec** — supports `2025-11-25` only while the official SDK has a pre-release supporting `2026-07-28`. |
| Hand-rolled stdio JSON-RPC (~300 LOC) | **False economy.** (a) The spec is mid-rewrite — `2026-07-28` removes `initialize`, adds `server/discover`, replaces notifications with `subscriptions/listen`, introduces MRTR for elicitation/sampling/roots. (b) Edge cases are easy to get wrong (JSON-RPC error codes, cancellation via `notifications/cancelled` + `ctx.Done()`, progress tokens, batched requests — banned in `2025-06-18+`). (c) No conformance tests. (d) The official SDK's transitive deps are all pure Go — the "zero deps" benefit is marginal. |
| Streamable HTTP transport / mounting MCP inside Kamacu binary | **Out of scope per PROJECT.md.** Stdio is the right transport for agents spawned *inside* Kamacu — they're already subprocesses. HTTP/SSE is for the future external-editor (Claude Desktop / Cursor) milestone. |
| Any MCP SDK that pulls CGO | **None required.** Both viable candidates (official + mcp-go) are pure Go. |

### Architecture Approach

**Three-process model (forced by MCP semantics):**

| Process | Lifetime | Role |
|---------|----------|------|
| **Kamacu binary** (existing v1.10) | Long-lived, started once by user | HTTP + WS server at `127.0.0.1:7333`; owns DB, SessionManager, reaper, SPA. **Unchanged except two new read endpoints.** |
| **Agent CLI** (`claude` / `opencode`) | Per-task, spawned by Kamacu in worktree PTY | Owns the TUI the browser renders. Acts as the MCP *client*. **Unchanged from v1.10.** |
| **`kamacu mcp serve` subcommand** (NEW) | Per-agent-session, spawned by the agent CLI | Stdio MCP *server* that bridges tool calls to Kamacu's HTTP API. Reads `KAMACU_*` env once at startup; lives and dies with the agent CLI. |

**Major new/modified components:**
1. **`cmd/kamacu/mcp_subcommand.go`** — stdlib subcommand dispatch (`if os.Args[1] == "mcp"` at top of `main()` before `flag.Parse`), calls `internal/mcp.Run`.
2. **`internal/mcp/` (NEW)** — the server. `server.go` (NewServer + tool registration + StdioTransport.Run), `bridge.go` (authenticated HTTP client wrapper, one `*http.Client` reused, holds token), `scope.go` (`KAMACU_SESSION_ID` → `task_id`/`project_id` resolution, lazily resolved + cached), `tools_*.go` (one file per Kamacu resource area). Mirrors `internal/api/` and `internal/ws/` naming convention. **Does NOT import `internal/session`** — terminal reads go through HTTP, keeping the subcommand a clean bridge.
3. **`internal/mcpconfig/` (NEW LEAF)** — spawn-time config writers. `WriteClaude(worktreeDir, env)` writes `.mcp.json`; `WriteOpenCode(worktreeDir, env)` writes `opencode.json`. Pure functions, no state. **Shares no code with `internal/mcp`** (separate processes, separate concerns — merging would create an import cycle).
4. **`internal/api/terminal_reads.go` (NEW)** — two read-only endpoints exposing the existing Session ring buffer: `GET /api/sessions/{id}/snapshot` (full ring), `GET /api/sessions/{id}/tail?since=N` (incremental). Auth: existing `X-Kamacu-Token` envelope.
5. **v1.10 spawn engine (MODIFIED)** — one new step between worktree creation and PTY start: `mcpconfig.Write(engine, worktreePath, env)` for `engine=claude` or `engine=opencode`; custom engines skipped.
6. **`cmd/kamacu/main.go` (MODIFIED)** — one new dispatch branch at the top of `main()`; existing HTTP-server path unchanged.

**Per-task env scoping:** `KAMACU_SESSION_ID` / `KAMACU_HOOK_TOKEN` / `KAMACU_HOOK_BASE` are already injected by the v1.10 spawn engine. The MCP subcommand inherits them through normal env inheritance. **No per-task MCP config file content** — the *same* `.mcp.json`/`opencode.json` template works for every task because per-task identity rides on env. The scope resolver does ONE HTTP GET on first `my_*` tool call to map session→task→project, caches the result on the `Server` struct for the subcommand's lifetime.

**Agent-CLI config shapes (engine-specific; critical to get right):**

| Field | Claude Code (`<worktree>/.mcp.json`) | opencode (`<worktree>/opencode.json`) |
|-------|--------------------------------------|----------------------------------------|
| Top-level key | `mcpServers` | `mcp` |
| Server type discriminator | `"type": "stdio"` (default, but explicit is safer) | `"type": "local"` (**required**) |
| Command shape | `"command": "kamacu"` + `"args": ["mcp", "serve"]` | `"command": ["kamacu", "mcp", "serve"]` (single array) |
| Env vars key | `"env": {...}` | `"environment": {...}` |
| Env-value expansion | `${VAR}` and `${VAR:-default}` | `{env:VAR_NAME}` (different syntax) |
| Approval | First-use prompt per project (project scope) | None — trusted from config file |
| Reserved server names to avoid | `workspace`, `claude-in-chrome`, `Claude Preview`, `Claude Browser`, `computer-use` | (none documented; `kamacu` is safe everywhere) |

**Project-scope (worktree-root) is the right layer** — never the user-global `~/.claude.json` or `~/.config/opencode/opencode.json`. The worktree is per-task; the file is per-task; deleting the worktree auto-cleans the config. Zero global pollution. **Write literal values, not `${VAR}` placeholders** — the token is already per-process secret per the v1.10 injection posture; agent-CLI env expansion would just add a step that reads from a different env.

### Tool Surface

**MCP primitive selection: default everything to Tools.** The 2026 reality: agent CLIs (Claude Code, opencode) auto-invoke **Tools**; Resources are application-pulled via host context-pickers (user picks them, not the LLM); Prompts are slash-command templates. v1.11 is **tools-only**. The narrow exceptions (a `kamacu://tasks/{id}/brief` resource for host-context-injection; 2–3 prompts for canned workflows like `open-pr-review`) **defer to v1.12+**.

**Total v1.11 surface: ~23 generic tools + 4 scoped variants + 2 spawn integrations = ~29 surfaces** (matches the milestone's "~30 tools" target). Every tool maps 1:1 to an existing Kamacu HTTP endpoint — the MCP handler is a thin HTTP client call. No business logic in the MCP layer.

**Per-domain breakdown:**

| Domain | Tools | Notes |
|--------|-------|-------|
| **Task lifecycle** | 6: `list_tasks`, `get_task`, `create_task`, `update_task`, `move_task`, `delete_task` | Maps to `/api/tasks*`. The agent must be able to read and modify the board. |
| **Session lifecycle** | 4: `list_sessions`, `get_session`, `start_session`, `stop_session` | `start_session`/`stop_session` are side-effecting (spawn/kill PTY) → annotate `destructiveHint` appropriately. |
| **Terminal output (read-only)** | 2: `tail_session_output` (snapshot, v1.11), `subscribe_session_output` (live, v1.11 risk-center) | Maps to the two NEW endpoints. Snapshot is table-stakes; subscribe is the differentiator. **No keystroke injection tool — ever.** |
| **Project / workspace / agent / settings** | 5: `list_projects`, `get_project`, `list_workspaces`, `list_agents`, `get_settings` | Read-only context. |
| **GitHub PR review** | 2: `list_pr_reviews`, `get_pr_review` | Read-only + open-review-workspace (reuses existing gated path). |
| **Diff & worktree** | 5: `get_task_diff`, `mark_file_viewed`, `list_worktree_cleanup_candidates`, `clean_worktree_eligible`, `remove_worktree` | Reuses existing endpoints; never bypasses dirty-tree gates. |
| **Scoped variants (auto-injected when `KAMACU_SESSION_ID` set)** | 4: `list_my_tasks`, `get_my_task`, `get_my_session`, `tail_my_output` | Simpler schemas (no `project_id`/`session_id` arg); removes ~50% of the boilerplate args an agent would otherwise need. `get_my_brief` composite tool deferred to v1.12. |
| **Spawn-time agent-CLI integrations** | 2 paths: `WriteClaude`, `WriteOpenCode` | Without these, tools exist but no agent discovers them. |

**ToolAnnotations on every tool** (`readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint`) — clients (Claude Code, opencode) use these to decide whether to prompt the user for confirmation before calling. All Kamacu tools are `openWorldHint: false` (they operate on local Kamacu state, not the open internet).

**Per-task auto-scoping mechanism:** A ToolFilter (mcp-go's `server.WithToolFilter(func(ctx, tools) []Tool)` in the FEATURES doc; the equivalent in the official SDK is plain conditional registration driven by env) reads `KAMACU_SESSION_ID` once at startup. When present, scoped variants are added to the visible tool list; when absent (dev/standalone invocation), they are hidden. Generic tools are always present. The scoped variants resolve their implicit `session_id`/`task_id`/`project_id` via the lazily-cached scope resolver (one HTTP GET on first call, then zero overhead).

### Terminal Read + Subscribe — the one genuinely new capability

Everything else in v1.11 is translation; this is the one place v1.11 adds a behavior Kamacu didn't have. Two tools, shipped in this order:

1. **`tail_session_output` (snapshot) — TRIVIAL, table-stakes.** Returns a bounded slice from the existing Session ring buffer (256KB–1MB). The buffer already exists (built in v1.0 Phase 2 for WS attach replay). One new HTTP endpoint: `GET /api/sessions/{id}/snapshot?lines=N&bytes=N`. Handler calls the equivalent of `mgr.Get(id).Snapshot()`, returns the bytes as TextContent (base64 if non-UTF-8). Ship first — proves the data path with no streaming complexity.

2. **`subscribe_session_output` (live-tail) — MEDIUM/HIGH complexity, the risk center.** Streams chunks via `notifications/progress` for up to `duration_seconds` (default 30, hard-cap 300), then returns. Reuses the existing Session broadcast channel — the MCP subscribe is just one more Attach. **Cancellation is free via the SDK** — when the agent CLI sends `notifications/cancelled`, the handler's `ctx.Done()` fires; the handler MUST `defer s.Detach(connID)` as the first action, then return `(finalResult, nil)` (the SDK suppresses the response on cancellation; returning a clean result with the partial stream collected so far is safe and gives the agent value).

**Duration cap:** 300s hard-cap (configurable via flag/env) — well below Claude Code's 30-min stdio idle timeout. With progress notifications every <60s, even the 5-min `CLAUDE_CODE_MCP_TOOL_IDLE_TIMEOUT` is comfortably avoided. Document the cap in the tool description so the LLM knows.

**The read-only contract is enforced at the type level.** The WS the tool opens is read-only by construction: a `wsReader` wrapper that exposes only the read channel and has no public Write method. The existing WS handler accepts `FrameData` frames as PTY input — the MCP subcommand MUST NOT send those. (Alternatively, the HTTP `/tail` endpoint can be a polled fallback that doesn't even open a WS — simpler, no input risk at all. **Recommended primary path.**)

**Open question (defer to plan-phase): polling vs streaming.** Two viable shapes for the live tail:
- **Polling HTTP `/tail?since=N`**: simpler, no WS risk, one HTTP round-trip per chunk (every 200–500ms); works through any HTTP intermediary.
- **WS streaming**: zero-overhead fan-out via the existing broadcast channel; chunks arrive as they're produced; requires the `wsReader` wrapper discipline.

Research recommends the **HTTP tail as primary** (simpler, no input risk) with WS as a future optimization. **Decide during Phase 5 planning.**

### Critical Pitfalls

| # | Pitfall | Severity | Prevention |
|---|---------|----------|------------|
| 1 | **Stdout pollution corrupts JSON-RPC stream** — any non-JSON byte on stdout (stray `fmt.Println`, panic trace, library logging) desyncs the stream; agent CLI tears down the connection | **CRITICAL — the #1 way MCP servers die** | Redirect `os.Stdout` from line zero; pin every logger to stderr (`slog.NewTextHandler(os.Stderr, …)`); CI grep `rg -n 'fmt\.Print\|log\.Print\|log\.Fatal' ./internal/mcp/ ./cmd/kamacu/` returns zero hits; malformed-tool test confirms stdout stays JSON-clean. |
| 3 | **`KAMACU_HOOK_TOKEN` leaks via tool output, logs, or errors** — the agent IS the LLM; any leaked token lands in its context window and is exfiltrable via prompt injection | **CRITICAL — privilege boundary breach** | Token lives in a single `bridge` struct constructed once at startup; never passed to tool handlers, never embedded in returned values, never included in errors. Redacting slog handler (`ReplaceAttr` with `strings.ReplaceAll`) as defense in depth. No tool returns `os.Environ()` or process info unredacted. Constant-time compare stays in `internal/api/hooks.go` — the subcommand PRESENTS the token, doesn't CHECK it. |
| 4 | **Bridge-to-HTTP failure modes surface as confusing wrapped errors** — agent burns turns guessing, or worse, bypasses Kamacu (e.g. creates a worktree with raw `git` instead of via Kamacu, bypassing the dirty-tree gate) | **HIGH** | Map every HTTP failure to a typed MCP error code: `kamacu_down`, `kamacu_port_mismatch`, `kamacu_token_rejected`, `kamacu_timeout`, `kamacu_route_404`, `kamacu_request_too_large` (6 typed codes — NOT wrapped `fmt.Errorf`). Each has a `code`, a `message`, and structured `data`. Each error string includes a one-line recovery action. Typed `http.Client{Timeout: 10s}` (60s for subscribe). Never retry `kamacu_token_rejected` — that's a "restart the task" condition. |
| 5 | **Long-running `subscribe`/`tail` leaks goroutines, ignores cancellation, or blocks the stdio reader** — head-of-line blocking; agent CLI kills the call after idle timeout; user loses the tail | **HIGH — risk center** | Two-mode tool surface (sync `snapshot` + long `subscribe`); `duration_seconds` parameter default 30s, hard-cap 300s; `defer s.Detach(connID)` as first handler action; emit `notifications/progress` every <60s; per-session fine-grained locks (never a single bridge-level mutex around HTTP calls); goroutine-leak test (`runtime.NumGoroutine()` stable across N subscribe/cancel cycles). |
| 7 | **Agent-CLI config write corrupts `~/.claude.json` / `opencode.json` or pollutes global config** — concurrent task spawns race; user's hand-edits get clobbered; stale entries linger forever | **HIGH** | Project-scope only (`.mcp.json` / `opencode.json` at worktree root) — NEVER user-global. Read-modify-write with `flock`: read existing JSON, unmarshal into `map[string]any` (preserve unknown fields), set the `kamacu` entry, marshal, write to tempfile, `os.Rename` atomically. Idempotent writes (skip on byte-identical). Cleanup at task Done via the existing Done-TTL reaper hook. Server name: literal `kamacu` (no dots, no reserved words). |
| 8 | **Process lifecycle — zombie MCP subcommands, premature stdin close, SIGTERM races** | **MEDIUM-HIGH** | Treat stdin EOF as shutdown signal; bound shutdown to 3s (small per-call HTTP timeouts so in-flight calls finish within the window); install HTTP-client-wide context tied to subcommand lifetime; install `PR_SET_PDEATHSIG` via syscall wrapper on Linux (5-line cgo-free); never write state on shutdown (subcommand is stateless). Zombie-process test: spawn agent CLI, kill -9, check no `kamacu mcp serve` lingers. |

(Secondary pitfalls — stderr ambiguity across agent CLIs, JSON-RPC batching / protocol-version drift / `2026-07-28` deprecation wave — are documented in PITFALLS.md. The full 8-pitfall list, technical-debt patterns, integration gotchas, performance traps, security mistakes, and the "Looks Done But Isn't" checklist all live there.)

---

## Implications for Roadmap

Based on combined research, suggested **6-phase structure**. All four research tracks converged on 4–6 phases with broad agreement; this decomposition picks the cleanest grouping where each phase has a single clear gate, isolates one risk class, and could in principle ship independently.

### Phase 1 — Subcommand foundation: skeleton + transport + lifecycle
**Rationale:** Everything else depends on a working `kamacu mcp serve` that speaks the protocol. This is also where the lifecycle and stdout discipline are set — getting either wrong later means rewriting every tool.
**Delivers:** `cmd/kamacu/mcp_subcommand.go` (stdlib dispatch); `internal/mcp/server.go` minimal (`mcp.NewServer` + `StdioTransport.Run`, no tools yet); stdout redirect to stderr; `slog` redacting logger to stderr; stdin EOF → graceful shutdown within 3s; SIGTERM handling. `go.mod` updated with the official SDK.
**Avoids:** Pitfall 1 (stdout pollution — foundational discipline), Pitfall 2 (stderr ambiguity — WARN default), Pitfall 8 basic (clean shutdown on stdin EOF).
**Success gate:** Agent CLI connects, sees `tools/list` = `[]`, disconnects cleanly. Malformed-tool test: a handler that calls `fmt.Println("debug")` produces a clean JSON-RPC error on stdout, NOT a stream desync.

### Phase 2 — Bridge layer + typed error taxonomy + first tool (minimal vertical slice)
**Rationale:** The bridge + error taxonomy is the gate for every subsequent tool — without it, Phase 3's CRUD tools would each invent their own error format. The vertical slice (one tool end-to-end through the bridge) proves the architecture.
**Delivers:** `internal/mcp/bridge.go` (one `*http.Client` reused, holds token, attaches `X-Kamacu-Token` header, typed `bridgeError` with the 6-code taxonomy); `internal/mcp/scope.go` (lazy `KAMACU_SESSION_ID`→`task_id`/`project_id` resolution); `internal/mcp/tools_tasks.go` minimal — `list_tasks` + the scoped `list_my_tasks` variant; per-tool tests with mocked bridge; bridge tests with `httptest.NewServer` exercising each of the 6 typed errors.
**Avoids:** Pitfall 3 (token never enters tool closures), Pitfall 4 (typed errors from day one — never wrapped `fmt.Errorf`).
**Success gate:** "From inside a Claude session spawned by Kamacu, I can ask 'list my tasks' and it correctly answers." Manually killing Kamacu mid-call surfaces a typed `kamacu_down` error to the agent — not a confusing wrapped string.

### Phase 3 — CRUD tool surface (full table-stakes parity)
**Rationale:** Once the bridge + error patterns are proven, the long tail of CRUD tools is mechanical: each is a thin HTTP client call. Grouping them in one phase keeps the pattern consistent.
**Delivers:** The remaining tools from FEATURES.md §"Tool Surface Reference" — Task lifecycle (5 more), Session lifecycle (4), Project/workspace/agent/settings (5), GitHub PR review (2), Diff & worktree (5), plus scoped variants (`get_my_task`, `get_my_session`, `tail_my_output`). Per-tool tests with mocked bridge. `ToolAnnotations` on every tool.
**Avoids:** Pitfall 6 (no deprecated capabilities — tools-only); reuse of Phase 2's bridge pattern means no new error-handling foot-guns.
**Success gate:** `tools/list` returns ~23 generic tools + 4 scoped variants; each tool's handler test covers happy path + at least one bridge error code; UI parity with the board (every action the user can do via the SPA, the agent can do via a tool).

### Phase 4 — Spawn-time agent-CLI registration (Claude + opencode)
**Rationale:** Decoupled from Phase 3 — could ship in parallel — but stays fourth because the writer pattern is established in Phase 1 (the subcommand exists) and tools must exist before registration is meaningful end-to-end. Claude Code is the default agent and proves the pattern; opencode is the second engine and surfaces any engine-specific config quirks.
**Delivers:** `internal/mcpconfig/claude.go` (`WriteClaude` — `.mcp.json` at worktree root); `internal/mcpconfig/opencode.go` (`WriteOpenCode` — `opencode.json` at worktree root); spawn engine branch on `agent.engine`; absolute-path resolution via `os.Executable()` so PATH changes don't break discovery; read-modify-write + `flock` + preserve-unknown-fields + atomic `os.Rename`; idempotent skip on byte-identical; cleanup on task Done via the existing reaper. Test matrix mirrors `internal/opencode/plugin_test.go`.
**Avoids:** Pitfall 7 (project-scope only, atomic writes, no clobber, no pollution, concurrent-safe); decision documented that custom agents don't get auto-injection.
**Success gate:** Spawn a Claude task → agent CLI auto-discovers the `kamacu` server and lists its tools with no user action. Spawn an opencode task → same. Kill the Kamacu process mid-task → next task spawn writes a fresh config (no stale entries). Two concurrent task spawns on the same project → both configs written correctly (no race).

### Phase 5 — Live subscribe/tail (the risk center)
**Rationale:** The genuinely new capability — the one place v1.11 adds a behavior Kamacu didn't have. Saving it for after the surface is built lets it land on a stable foundation. Its failure modes (goroutine leaks, cancellation, idle-timeout kills) are qualitatively different from CRUD tools and deserve a dedicated phase.
**Depends on:** Phase 1 (server skeleton); the two new HTTP endpoints in `internal/api/terminal_reads.go`.
**Delivers:** `internal/api/terminal_reads.go` (`GET /api/sessions/{id}/snapshot`, `/tail`); `internal/mcp/tools_terminal.go` — `tail_session_output` (snapshot, trivial) + `subscribe_session_output` (long-running, `duration_seconds` default 30 / max 300); `wsReader` wrapper with no Write method (compile-time read-only guarantee); cancellation + progress tests using `mcp.NewInMemoryTransport`; goroutine-leak test.
**Avoids:** Pitfall 5 (cancellation via `ctx.Done()`, `defer s.Detach` first, `notifications/progress` every <60s, bounded duration, goroutine-stable); Anti-pattern 5 (PTY write — `wsReader` enforces).
**Success gate:** "From inside a Claude session, I can ask 'watch my other agent's terminal for 30 seconds and summarise what it did' and Claude correctly calls the subscribe tool, waits for it to return, and summarises the output." Cancellation test: `subscribe` cancelled mid-stream detaches within 100ms; `runtime.NumGoroutine()` returns to baseline after N subscribe/cancel cycles.

### Phase 6 — Hardening: multi-agent-CLI smoke + security review + lifecycle hardening
**Rationale:** Cross-cutting verification. Lands last because it depends on everything else being functional. Catches the residual Pitfall 8 (PR_SET_PDEATHSIG), Pitfall 2 (multi-client smoke), Pitfall 6 (version compat).
**Delivers:** `PR_SET_PDEATHSIG` syscall wrapper (Linux, cgo-free); real-binary e2e harness `//go:build mcp_e2e` modeled on the existing `//go:build opencode_e2e`; test matrix: current Claude Code, current opencode, Claude Code one minor version back; deliberate prompt-injection test (token not extractable via any tool surface); version-compat smoke (initialize advertises SDK defaults, no JSON-RPC batches emitted).
**Success gate:** Subcommand exits within 3s of stdin EOF / SIGTERM; no zombies after N agent sessions; `rg -n 'exec\.Command|os\.WriteFile' ./internal/mcp/` returns zero hits (read-only surface enforced); prompt-injection test fails to extract the token; multi-CLI smoke passes against the test matrix.

### Phase Ordering Rationale

- **Dependencies force the order:** Phase 1 (transport + lifecycle) is foundational. Phase 2's error taxonomy is the gate for every subsequent tool. Phase 3's CRUD tools reuse Phase 2's bridge pattern mechanically. Phase 4's config writer needs Phase 1's subcommand to exist (otherwise it registers nothing) and benefits from Phase 3's tools being there (end-to-end meaningful). Phase 5 needs the two new HTTP endpoints AND Phase 1's skeleton. Phase 6 cross-cuts and verifies.
- **Architecture grouping:** Phase 1+2 establish the bridge architecture; Phase 3 is mechanical CRUD parity; Phase 4 is the spawn-engine integration (separate package, separate concerns); Phase 5 is the genuinely new capability with qualitatively different failure modes; Phase 6 is verification.
- **Risk placement:** the highest-risk work — `subscribe` (Pitfall 5) and agent-CLI config write (Pitfall 7) — each lands in its own dedicated phase where it is the explicit focus, not buried inside a CRUD phase.

### Research Flags

Phases likely needing deeper research during planning (`/gsd-plan-phase --research-phase`):

- **Phase 5 (Live subscribe/tail):** Highest-risk phase. Needs dedicated research on Go SDK progress-notification semantics, the 5s `notifyCancellationTimeout` handshake, and the **polling-vs-streaming** choice (HTTP `/tail` vs WS). Likely a separate research spike before planning.
- **Phase 4 (Agent-CLI registration):** Claude Code's `${VAR}` expansion + the three-scope model need a verification pass against the actual `~/.claude.json` schema (which evolves). opencode's `{env:VAR_NAME}` expansion parity needs verification against opencode 1.17.x+. A spec-correlation task before planning.
- **Phase 6 (Hardening):** The `PR_SET_PDEATHSIG` syscall wrapper is non-obvious in pure Go — research the cleanest stdlib-only approach. Real-binary e2e harness shape needs design.

Phases with standard / verified patterns (likely skip research-phase):

- **Phase 1 (Subcommand foundation):** Stdlib subcommand dispatch (`flag.NewFlagSet` + `os.Args[1]` check) is the documented Go pattern; SDK stdio transport is the documented happy path.
- **Phase 2 (Bridge + first tool):** SDK's `examples/server/proxy/main.go` is structurally identical; the typed error taxonomy is a small enum.
- **Phase 3 (CRUD tool surface):** Mechanical repetition of Phase 2's pattern across existing endpoints; no new architectural surface.

---

## Open Questions to Defer to Plan-Phase

These are not research gaps (the technical options are clear) — they are *scoping* decisions that belong in plan-phase where the planner can weigh trade-offs against the actual codebase state.

| # | Question | Options | Notes |
|---|----------|---------|-------|
| 1 | **Terminal subscribe shape: poll vs stream** | (a) Polling HTTP `/tail?since=N` every 200–500ms; (b) WS streaming via existing broadcast channel with `wsReader` wrapper | Research recommends (a) as primary (simpler, no input risk); (b) is a future optimization. Decide during Phase 5 planning. |
| 2 | **Tool-name prefix convention** | (a) snake_case no prefix (`list_tasks`) — host CLI namespaces automatically; (b) dotted namespaced (`kamacu.tasks.list`) — clearer in `/mcp` panel | FEATURES.md recommends (a) (matches the `git` reference server); PITFALLS UX section suggests (b). Low-stakes; pick (a) to match the reference server unless the panel UX is materially worse. |
| 3 | **`KAMACU_HOOK_TOKEN` scope on existing API** | Does the existing Kamacu HTTP API accept `KAMACU_HOOK_TOKEN` for ALL endpoints, or only the `/api/hooks/*` paths the v1.10 spawn engine uses? | If only hooks paths, Phase 2 must extend the token's acceptance to all endpoints the MCP bridge calls (or mint a separate MCP-scope token). Verify in Phase 2 against `internal/api/hooks.go`. |
| 4 | **opencode `${VAR}` / `{env:VAR}` expansion parity** | Does opencode's `{env:VAR_NAME}` expand env vars the same way Claude Code's `${VAR}` does? | If yes, can use placeholders in the config template; if no (or unreliable), write literal values (current recommendation). Verify during Phase 4 against opencode 1.17.x+. |
| 5 | **Real-binary e2e harness shape** | Build tag (`//go:build mcp_e2e` mirroring `opencode_e2e`)? Separate test suite? Run in CI or manual? | Phase 6 design decision; the v1.10 `opencode/e2e_test.go` pattern is the template. |

---

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| **Stack** | HIGH | Official SDK v1.6.1 verified on pkg.go.dev (hand-tested API shapes); raw go.mod read; transitive deps all pure Go; `examples/server/proxy/main.go` read in full and structurally identical to Kamacu's bridge use case. The official-vs-mcp-go-vs-hand-rolled decision is unanimous across all four research files. |
| **Features** | HIGH | MCP spec 2025-06-18 / 2025-11-25 read in full for tools, lifecycle, cancellation; canonical reference implementations (`everything`, `git`) analyzed; agent-CLI config shapes verified against live Anthropic + opencode docs. FEATURES.md initially recommended `mark3labs/mcp-go` for the framework but STACK.md overruled with the official SDK — the tool surface is identical regardless of SDK pick. |
| **Architecture** | HIGH | Every integration point names the real v1.10 file/function/table it touches (verified by reading the working tree). The 3-process model is forced by MCP semantics. Per-task scoping via env is the existing v1.10 contract. The two config shapes are documented at the field level. |
| **Pitfalls** | HIGH | All 8 pitfalls grounded in MCP spec, Go SDK source (line-level), Anthropic Claude Code docs, opencode docs, and the existing Kamacu codebase (`internal/api/hooks.go`, `internal/session/{session,manager}.go`, `internal/opencode/plugin.go`). Security pitfalls cross-checked against Palo Alto Unit 42 PoCs and Practical DevSecOps 2026 review. |

**Overall confidence:** HIGH

### Gaps to Address

- **Polling-vs-streaming for terminal subscribe** — not a research gap (both are well-understood); a Phase 5 scoping call. Both work; research recommends polling-HTTP-tail as primary.
- **`KAMACU_HOOK_TOKEN` endpoint scope** — needs verification against the existing `internal/api/hooks.go` receiver pattern; if the token is hooks-only today, Phase 2 must extend it (or mint an MCP-scope token). This is the one place v1.11 might require a small change to existing API surface beyond the two new read endpoints.
- **opencode `{env:VAR}` expansion parity** — verify against opencode 1.17.x+ before Phase 4 commits to either literal values or placeholders. Low-stakes: literal values work regardless.
- **`PR_SET_PDEATHSIG` in pure Go** — the 5-line syscall wrapper exists but isn't stdlib; Phase 6 research should confirm the cleanest cgo-free approach for Linux (and confirm macOS fallback behavior — agent CLIs run on both).

---

## Sources

### Primary (HIGH confidence)

- **[pkg.go.dev/github.com/modelcontextprotocol/go-sdk](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk)** v1.6.1 — API surface (`mcp.NewServer`, `mcp.AddTool` typed-generic handlers, `mcp.StdioTransport`, `mcp.CallToolRequest`/`Result`, `req.Session.NotifyProgress`); license Apache-2.0/MIT; mcp package imported by 1,443 modules. Verified 2026-07-21.
- **[raw go.mod for modelcontextprotocol/go-sdk v1.6.1](https://raw.githubusercontent.com/modelcontextprotocol/go-sdk/v1.6.1/go.mod)** — `go 1.25.0` floor; transitive deps verified.
- **[raw examples/server/proxy/main.go from go-sdk v1.6.1](https://raw.githubusercontent.com/modelcontextprotocol/go-sdk/v1.6.1/examples/server/proxy/main.go)** — the documented HTTP→HTTP MCP proxy pattern; structurally identical to Kamacu's stdio→HTTP bridge use case.
- **[MCP spec 2025-06-18 — Transports](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports)** — stdio = newline-delimited JSON-RPC over stdin/stdout (NOT LSP Content-Length); server MAY write logs to stderr; "MUST NOT write anything to stdout that is not a valid MCP message." Verified 2026-07-21.
- **[MCP spec 2025-11-25 — Cancellation / Tools / Lifecycle](https://modelcontextprotocol.io/specification/2025-11-25)** — `notifications/cancelled` flow + `ctx.Done()` semantics; tool definition shape; tool result types; two-class error handling (protocol errors via JSON-RPC codes vs tool execution errors via `result.isError`); initialize/initialized handshake.
- **[Claude Code MCP docs](https://docs.anthropic.com/en/docs/claude-code/mcp)** — three scopes (local/project/user); `.mcp.json` shape (`mcpServers` + `command`+`args` + `env` + optional `type:"stdio"`); `${VAR}` / `${VAR:-default}` expansion; `CLAUDE_PROJECT_DIR`; reserved server names; stdio idle timeout (30 min default); `MCP_TOOL_TIMEOUT`.
- **[opencode config docs](https://opencode.ai/docs/config/)** + **[opencode MCP servers docs](https://opencode.ai/docs/mcp-servers/)** — `mcp` key (NOT `mcpServers`); `type: "local"` (NOT `"stdio"`); `command` as single array; `environment` (NOT `env`); `{env:VAR_NAME}` interpolation; precedence order.
- **[`modelcontextprotocol/servers` reference implementations](https://github.com/modelcontextprotocol/servers)** — `everything` (showcase, ~18 tools, ToolAnnotations discipline, `trigger-long-running-operation` progress-notification pattern); `git` (closest CLI-wrapper analog, 12 verb-per-tool discipline, per-tool Pydantic schemas, per-tool `ToolAnnotations`, input-injection defense, `validate_repo_path` defense-in-depth); `filesystem` (allowed-roots enforcement → for Kamacu, "allowed sessions" enforcement analog).
- **[Palo Alto Unit 42 — MCP Prompt Injection PoCs](https://unit42.paloaltonetworks.com/model-context-protocol-attack-vectors/)** + **[Practical DevSecOps — MCP Server Vulnerabilities 2026](https://www.practical-devsecops.com/mcp-security-vulnerabilities/)** — token leakage threat model, tool poisoning, prompt-injection exfiltration.
- **[anthropics/claude-code issue #47677](https://github.com/anthropics/claude-code/issues/47677)** — v2.1.105 broke all stdio MCP servers by closing stdin immediately (cautionary tale on stdio fragility across agent-CLI versions).
- **[modelcontextprotocol/specification issue #177](https://github.com/modelcontextprotocol/modelcontextprotocol/issues/177)** — stderr semantics ambiguity across clients (Roo false-alarms on benign INFO logs).
- **[Live Kamacu codebase at v1.10](https://github.com/jordi/kamacu)** — every integration point verified in the working tree: `cmd/kamacu/main.go` (hookToken, hookBaseURL normalization, stdlib flag style); `internal/api/{routes,sessions,hooks}.go` (envelope auth, constant-time compare); `internal/session/{manager,session}.go` (D014 env contract at spawn, `Session.Attach`/`Snapshot`/`Detach`); `internal/ws/{handler,proto}.go` (`FrameData` PTY-input frame); `internal/opencode/plugin.go` (write-if-absent / no-clobber / `// kamacu-managed` header discipline); `internal/opencode/e2e_test.go` (`//go:build opencode_e2e` real-binary harness pattern); migrations directory.

### Secondary (MEDIUM confidence)

- **[github.com/mark3labs/mcp-go](https://github.com/mark3labs/mcp-go)** v0.56.0 — rejected alternative; raw go.mod confirms pre-v1 status; README candid about instability; spec-2025-11-25 only.
- **[github.com/BloopAI/vibe-kanban](https://github.com/BloopAI/vibe-kanban)** — closest architectural twin (worktree-per-task, local-only single binary); sunsetting but informed the milestone shape.
- **[mcp-stdio-guard](https://github.com/orgs/modelcontextprotocol/discussions/753)** — community CLI to catch stdout pollution; corroborates Pitfall 1 severity.
- **[Foojay — Understanding MCP Through Raw STDIO Communication](https://foojay.io/today/understanding-mcp-through-raw-stdio-communication/)** — line-delimited JSON pattern; "Log to file, not stdout!" corroboration.

### Tertiary (LOW confidence)

- Web-search corroboration on Claude Code `mcpServers` config (klymentiev.com 2026 guide, claudelog.com, github.com/anthropics/claude-code/issues/5037) — consistent with primary docs.

---
*Research completed: 2026-07-21*
*Ready for roadmap: yes*
