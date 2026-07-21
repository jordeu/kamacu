# Stack Research

**Domain:** MCP (Model Context Protocol) server capability added to an existing Go-backend single-binary app — a stdio MCP subcommand that bridges to the app's existing HTTP API.
**Researched:** 2026-07-21
**Confidence:** HIGH — all versions, go.mod requirements, and API shapes below were verified live on pkg.go.dev, raw GitHub `go.mod` files, and the official MCP / Claude Code / opencode docs (2026-07-21). The Go SDK's `examples/server/proxy/main.go` was read in full to confirm the bridge pattern.

## TL;DR for the roadmap author

- **Use the official `github.com/modelcontextprotocol/go-sdk` v1.6.1.** Stable v1.x, Apache-2.0/MIT, MCP conformance-tested, supports MCP spec `2025-11-25` (current published) with `2026-07-28` support in pre-release (`v1.7.0-pre.3`). `go.mod` requires `go 1.25.0` — Kamacu's `go 1.26` satisfies it.
- **The bridge pattern is the SDK's documented happy path.** Its `examples/server/proxy/main.go` is *exactly* the shape Kamacu needs: an `mcp.NewServer` whose tool handlers call an upstream over a client transport. The only adaptation is that Kamacu's upstream is its own HTTP REST API at `http://127.0.0.1:7333` (with the existing `X-Kamacu-Token` envelope header) rather than another MCP server — so each tool handler does a plain `http.Client.Do(req)`, not `clientSession.CallTool`. No new architectural pattern needs inventing.
- **Stdio is the transport.** MCP stdio = newline-delimited JSON-RPC 2.0 over stdin/stdout (NOT LSP-style `Content-Length` headers — verified against the 2025-06-18 spec, still current in `2025-11-25`). One call: `server.Run(ctx, &mcp.StdioTransport{})`. Logging MUST go to `stderr` — never `stdout`.
- **Auth is already done.** Kamacu already injects `KAMACU_SESSION_ID`, `KAMACU_HOOK_TOKEN`, `KAMACU_HOOK_BASE` into spawned agent CLIs (v1.10). The MCP subcommand inherits those env vars when the agent CLI spawns it. No OAuth, no MCP-level auth needed — the bridge just copies `KAMACU_HOOK_TOKEN` into the `X-Kamacu-Token` request header the way the existing hook receiver paths already do.
- **Agent-CLI config wiring differs by engine.** Claude Code consumes `.mcp.json` at the worktree root (shape `{type: "stdio", command, args, env}` with `${VAR}` expansion). opencode consumes `opencode.json` at the worktree root (shape `{type: "local", command: [array], environment}` — note: different field names). At task spawn, Kamacu writes the appropriate file for the configured agent's engine.
- **Do NOT hand-roll the MCP wire protocol.** It is thin (newline-delimited JSON-RPC) and a tools-only server is technically ~300 LOC, but the spec is in active flux — `2026-07-28` is a near-complete rewrite (stateless model, `server/discover` RPC replacing `initialize`, `subscriptions/listen` stream, MRTR). The official SDK absorbs that churn; a hand-rolled server would re-implement it for zero benefit and a handful of transitive deps (all pure Go, no CGO).

---

## Recommended Stack

### Core Technologies — Backend (Go, additive only)

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| `github.com/modelcontextprotocol/go-sdk` | **v1.6.1** (May 22, 2026 stable; v1.7.0-pre.3 Jul 17 targets MCP `2026-07-28`) | The MCP SDK — server, handlers, stdio transport | The official Go implementation of the MCP spec, maintained under the `modelcontextprotocol` GitHub org (Anthropic-backed). Stable v1.x with tagged releases and an active pre-release track. Conformance-tested against the official MCP test suite (added v1.7.0-pre.2). Ships a documented proxy example that is structurally identical to Kamacu's bridge use case. 4.8k GitHub stars; the `mcp` package is imported by 1,443 other modules (pkg.go.dev, v1.6.1). License Apache-2.0 + MIT (new contributions Apache-2.0, original code MIT). Pinned to `go 1.25.0` in go.mod — Kamacu's `go 1.26` is above the floor. |
| `github.com/modelcontextprotocol/go-sdk/mcp` | v1.6.1 | Primary import — server, tools, transport types | One import, one package. `mcp.NewServer`, `mcp.AddTool`, `mcp.StdioTransport`, `mcp.CallToolRequest`/`Result`. No sub-package juggling. |
| `net/http` (stdlib, already in go.mod via existing API server) | stdlib | The MCP subcommand calls Kamacu's existing REST API | The bridge calls `http.NewRequest` + `http.DefaultClient.Do` against `http://127.0.0.1:7333/api/...` with the `X-Kamacu-Token` header. No new HTTP library; reuse a thin client wrapper. |
| `os`, `os/exec`, `encoding/json`, `log/slog` (all stdlib) | stdlib | Env-var reads, JSON marshalling, structured logging to stderr | The MCP subcommand is a leaf — env-var reads for `KAMACU_HOOK_TOKEN` / `KAMACU_SESSION_ID` / `KAMACU_HOOK_BASE`, slog to stderr (NEVER stdout — that's the MCP wire). |

### Supporting Libraries (transitive, via the SDK)

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/google/jsonschema-go` | v0.4.3 | JSON Schema generation from Go types | Auto-used by `mcp.AddTool` when you pass a typed Go handler — derives the tool's `inputSchema`/`outputSchema` from struct tags. No direct call needed. |
| `github.com/yosida95/uritemplate/v3` | v3.0.2 | RFC 6570 URI templates (resource templates) | Auto-used by the SDK for resource-template URIs. Only relevant if Kamacu later exposes MCP *resources* (e.g., `kamacu://task/{id}/diff`) — v1.11 is tools-only per PROJECT.md. |
| `github.com/segmentio/encoding` | v0.5.4 | Faster JSON encoding | Internal optimisation in the SDK's JSON-RPC layer. Invisible to consumers. |
| `golang.org/x/oauth2` + `github.com/golang-jwt/jwt/v5` | v0.35.0 / v5.3.1 | OAuth flows | **NOT USED** by Kamacu (auth is the existing envelope token, not OAuth). They appear in `go.mod` because the SDK's `auth`/`oauthex` sub-packages reference them. Go's lazy package loading means they are not compiled into a binary that doesn't import those sub-packages — `go mod tidy` keeps them in go.mod but they don't bloat the binary. |
| `golang.org/x/tools` | v0.42.0 | Internal SDK tooling | Only used by the SDK's own code-generation; pulled because the SDK ships generated code. Harmless indirect dep. |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| `go get github.com/modelcontextprotocol/go-sdk@v1.6.1` | Add the dependency | One line; `go mod tidy` resolves the four transitive deps above. No CGO, no system libraries. |
| `mcp-cli` / inspector (optional) | Local MCP debugging | The `examples/server/proxy` shape lets you spin up the bridge against a fake Kamacu HTTP server for isolated testing. The wider MCP ecosystem has inspector tools; not required for v1.11. |
| stdio framing test | Verify the server speaks correct newline-delimited JSON-RPC | Trivial smoke test: pipe `{"jsonrpc":"2.0","id":1,"method":"initialize",...}\n` into `kamacu mcp serve` and parse the response from stdout. The SDK handles this; do it once as a regression guard. |

---

## The bridge pattern in concrete shape (for the plan-phase author)

The official SDK's `examples/server/proxy/main.go` demonstrates an HTTP→HTTP MCP proxy. Kamacu's adaptation — **stdio MCP → HTTP REST** — is even simpler because there's no upstream MCP to speak; each tool is one or two `http.Client.Do` calls. Skeleton:

```go
package mcpserve

import (
    "context"
    "net/http"
    "os"

    "github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
    httpClient *http.Client
    baseURL    string         // http://127.0.0.1:7333
    authToken  string         // from KAMACU_HOOK_TOKEN
    sessionID  string         // from KAMACU_SESSION_ID (per-task convenience scope)
}

func Run(ctx context.Context) error {
    s := &Server{
        httpClient: http.DefaultClient,
        baseURL:    envDefault("KAMACU_HOOK_BASE", "http://127.0.0.1:7333"),
        authToken:  os.Getenv("KAMACU_HOOK_TOKEN"),
        sessionID:  os.Getenv("KAMACU_SESSION_ID"),
    }

    mcpSrv := mcp.NewServer(&mcp.Implementation{Name: "kamacu", Version: buildVersion}, nil)
    s.registerTools(mcpSrv)   // ~15 tools: list_tasks, move_task, create_task, read_terminal, ...

    // Stdio transport. Reads JSON-RPC from stdin, writes to stdout, logs to stderr.
    return mcpSrv.Run(ctx, &mcp.StdioTransport{})
}

// Typed handler — input/output JSON schemas auto-derived from the struct tags.
type listTasksInput struct {
    ProjectID  string `json:"project_id,omitempty" jsonschema:"the project to list tasks in; omit to use the inherited session's project"`
    Status     string `json:"status,omitempty"     jsonschema:"one of todo|in_progress|in_review|done"`
}

type listTasksOutput struct {
    Tasks []taskSummary `json:"tasks"`
}

func (s *Server) listTasks(ctx context.Context, req *mcp.CallToolRequest, in listTasksInput) (*mcp.CallToolResult, listTasksOutput, error) {
    // Convenience scope: if ProjectID omitted, use the inherited session's project.
    projectID := in.ProjectID
    if projectID == "" {
        projectID = s.sessionProjectID(ctx)  // resolves KAMACU_SESSION_ID → project_id via one GET
    }
    // Bridge: call Kamacu's existing REST endpoint with the envelope auth.
    var out listTasksOutput
    if err := s.getJSON(ctx, "/api/projects/"+projectID+"/tasks", &out.Tasks); err != nil {
        return nil, listTasksOutput{}, err
    }
    return nil, out, nil
}
```

Two structural points worth flagging in plan-phase:

1. **The SDK's typed handler signature is `func(ctx, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error)`** — return `(*mcp.CallToolResult, Out, error)`. Pass `nil` for the first when you want the SDK to wrap `Out` automatically (its schema was derived at registration). Return a non-nil `*mcp.CallToolResult` only when you need to attach text content, errors via `SetError`, or structured content side-by-side. This is documented in the package example and the `toolschemas` example.
2. **`req.Extra.Header` exists** (the SDK propagates MCP transport metadata) but for Kamacu it's irrelevant — auth is env-var-derived, not header-derived. Don't over-engineer per-request auth.

### Session scoping (per-task vs cross-task tools)

PROJECT.md specifies two scopes:
- **Per-task convenience tools** (no params) — use the inherited `KAMACU_SESSION_ID` from env at startup. ~80% of the tool surface (read *this* task's terminal, list *this* project's tasks, etc.).
- **Cross-task tools** (explicit IDs) — accept `task_id` / `project_id` / `session_id` params and route accordingly. Needed for "move a task in *another* project", "list *all* my review-requested PRs", etc.

Both fall out of the same pattern — `s.sessionID` is read once at startup and used as the default in handlers that take an optional ID param. No SDK feature is needed for this; it's plain Go.

---

## Agent-CLI config wiring (the spawn-time integration)

This is the part that connects Kamacu's spawn engine (v1.10) to the new MCP subcommand. At task spawn, after the worktree is created and before the agent CLI starts, Kamacu writes the agent's MCP config file into the worktree root.

### Claude Code (engine = `claude`)

Claude Code looks for `.mcp.json` at the project (worktree) root — the project-scoped config (committed per VCS, but each worktree gets its own copy). Shape verified from `code.claude.com/docs/en/mcp` (2026-07-21):

```json
{
  "mcpServers": {
    "kamacu": {
      "type": "stdio",
      "command": "kamacu",
      "args": ["mcp", "serve"],
      "env": {
        "KAMACU_HOOK_TOKEN": "<inherited-from-spawn-env>",
        "KAMACU_SESSION_ID": "<task-session-uuid>",
        "KAMACU_HOOK_BASE": "http://127.0.0.1:7333"
      }
    }
  }
}
```

Notes:
- `type: "stdio"` is the implicit default (an entry without `type` is read as stdio) but explicit is safer.
- `${VAR}` and `${VAR:-default}` expansion is supported in `command`/`args`/`env`. Useful if Kamacu wants to write the file once with `${KAMACU_HOOK_TOKEN}` and let the agent CLI expand the env at spawn — but writing the literal values at task-spawn time is simpler and matches how the existing hook-token env injection works.
- Claude Code sets `CLAUDE_PROJECT_DIR` to the worktree root in the spawned server's env; Kamacu can read this as a fallback path resolver but it's redundant since `KAMACU_SESSION_ID` already disambiguates.
- Reserved server names to avoid: `workspace`, `claude-in-chrome`, `computer-use`, `Claude Preview`, `Claude Browser`. `kamacu` is safe.

### opencode (engine = `opencode`)

opencode looks for `opencode.json` (or `.jsonc`) at the workspace root. Shape verified from `opencode.ai/docs/mcp-servers` (2026-07-21):

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "kamacu": {
      "type": "local",
      "command": ["kamacu", "mcp", "serve"],
      "environment": {
        "KAMACU_HOOK_TOKEN": "<inherited-from-spawn-env>",
        "KAMACU_SESSION_ID": "<task-session-uuid>",
        "KAMACU_HOOK_BASE": "http://127.0.0.1:7333"
      },
      "enabled": true
    }
  }
}
```

**Critical differences from Claude Code's shape** (these are easy to get wrong):
| Field | Claude Code (`.mcp.json`) | opencode (`opencode.json`) |
|-------|--------------------------|------------------------------|
| Top-level key | `mcpServers` | `mcp` |
| Server type for stdio | `"stdio"` | `"local"` |
| Command shape | `"command": "kamacu"` + `"args": ["mcp", "serve"]` | `"command": ["kamacu", "mcp", "serve"]` (single array) |
| Env vars key | `"env"` | `"environment"` |
| Expansion | `${VAR}` / `${VAR:-default}` | `{env:VAR}` (different syntax — `opencode.ai/docs/config`) |

A custom-engine agent (`engine: custom`) has no first-class MCP discovery mechanism in Kamacu v1.11 — those CLIs are user-defined and may or may not speak MCP. Leave their config alone; if the user's custom CLI happens to consume `.mcp.json` (some do), they can drop one in the worktree themselves.

---

## Installation

```bash
# Add the SDK to the existing Go module
go get github.com/modelcontextprotocol/go-sdk@v1.6.1

# Verify
go mod tidy && go build ./...

# Transitive deps pulled in (verified from the v1.6.1 go.mod):
#   github.com/google/jsonschema-go v0.4.3
#   github.com/yosida95/uritemplate/v3 v3.0.2
#   github.com/segmentio/encoding v0.5.4 (+ asm v1.1.3 indirect)
#   golang.org/x/oauth2 v0.35.0   (only used by auth sub-packages, NOT by stdio-only consumers)
#   golang.org/x/tools v0.42.0
#   github.com/golang-jwt/jwt/v5 v5.3.1  (only used by auth sub-packages)
#   golang.org/x/sys (already in go.mod via creack/pty et al.)
```

No npm additions — v1.11 is Go-backend-only per the milestone brief. No frontend framework changes needed; the React app already talks to the existing HTTP API which the MCP subcommand will reuse unchanged.

---

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| **Official `modelcontextprotocol/go-sdk` v1.6.1** | `github.com/mark3labs/mcp-go` v0.56.0 | If you specifically want the builder-pattern API (`mcp.NewTool("x", mcp.WithString("y", mcp.Required()))` instead of typed-struct handlers), or need one of its batteries-included features for an HTTP/SaaS deployment (CORS, OAuth Protected Resource Metadata endpoint, DNS-rebinding protection, OpenTelemetry tracing). None of these apply to a localhost stdio bridge — see "Why NOT mcp-go" below. |
| Official go-sdk | `github.com/metoro-io/mcp-golang` | No reason in 2026. The metoro SDK was an early community effort; the official README acknowledges it as a viable alternative but it's largely dormant now that the official SDK exists. Smaller community, no conformance tests. |
| Official go-sdk | `github.com/ThinkInAIXYZ/go-mcp` | Same — early community SDK, smaller adoption. The official SDK supersedes it. |
| Official go-sdk | **Hand-rolled stdio JSON-RPC** | See dedicated section below — technically feasible for a tools-only v1 server, but a false economy. Only justified if you're building for a wildly resource-constrained target or want to learn the protocol internals. |
| `mcp.NewServer` + `server.Run(ctx, &mcp.StdioTransport{})` | Mount MCP inside the existing Kamacu HTTP server on `/mcp` (Streamable HTTP transport) | When (if) a future milestone exposes Kamacu as an MCP server to *external* editors (Claude Desktop, Cursor, VS Code). That's explicitly out of scope for v1.11 per PROJECT.md (Out of Scope: "MCP server for external AI editors"). Stdio is the right transport for agents spawned *inside* Kamacu because the agent CLI is already a subprocess Kamacu owns — adding an HTTP listener gains nothing and adds auth surface. |
| Typed-struct handlers via `mcp.AddTool` | Untyped handlers via `s.AddTool(tool, func(ctx, req) (...))` + raw `map[string]any` args | If a tool's input schema is so dynamic it can't be expressed as a Go struct (e.g., user-defined shapes from settings). All Kamacu tools have fixed shapes — use typed handlers for the auto-generated JSON schema and input validation. |

---

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `github.com/mark3labs/mcp-go` for **this milestone** | Three reasons. (1) **Not stable-tagged** — current is v0.56.0, still pre-v1, so the author reserves the right to break APIs between minor versions. Kamacu would be opting into churn. (2) **Its "batteries"** (CORS, OAuth Protected Resource Metadata, DNS rebinding protection, OpenTelemetry tracing, panic recovery, task-augmented tools, completion providers, per-session tools) are aimed at HTTP/SaaS deployments — a stdio bridge to localhost uses none of them. (3) **It's behind on the spec** — supports `2025-11-25` while the official SDK has a pre-release supporting `2026-07-28`. Its README is candid: "🚨 🏗️ MCP Go is under active development, as is the MCP specification itself." | `github.com/modelcontextprotocol/go-sdk` v1.6.1 |
| Hand-rolling the MCP stdio wire protocol | Honest evaluation: stdio MCP is newline-delimited JSON-RPC 2.0 — *thin*. A tools-only server is ~300 LOC (initialize/initialized handshake, tools/list, tools/call, ping, notifications/cancelled). BUT: (a) the spec is mid-rewrite — `2026-07-28` removes `initialize`, adds `server/discover`, replaces notifications with a `subscriptions/listen` stream, introduces MRTR for elicitation/sampling/roots; (b) edge cases are easy to get wrong — JSON-RPC error codes (`-32602` Invalid params, `-32601` Method not found, `-32002` Resource not found), cancellation propagation via `notifications/cancelled` + `context.Context`, progress tokens, batched requests; (c) no conformance tests means bugs surface only against real clients; (d) the official SDK's transitive deps are all pure Go (no CGO, no system libs), so the "zero deps" benefit is marginal. Net: the SDK costs ~4 small transitive deps and saves ongoing spec-chase work. | The official SDK |
| Any MCP SDK that pulls in CGO or a heavy framework | Confirmed: both viable candidates (official + mcp-go) are pure Go. The official SDK's deps (jsonschema-go, uritemplate, segmentio/encoding) are all pure Go. | The official SDK |
| Exposing Kamacu as an HTTP/SSE MCP server for v1.11 | Out of scope per PROJECT.md ("MCP server for external AI editors" is a future milestone). Stdio is the right transport for agents spawned inside Kamacu — they're already subprocesses. Adding an HTTP listener gains nothing and adds the spec-mandated DNS-rebinding + Origin-header-check surface. | stdio MCP subcommand |
| The `golang-jwt/jwt/v5` and `golang.org/x/oauth2` transitive deps *as code* | They appear in go.mod because the SDK's `auth` / `oauthex` sub-packages reference them. Kamacu doesn't use those sub-packages (auth is the existing envelope token). Do NOT import them; `go mod tidy` keeps them in go.mod but they are not compiled into the binary. | The existing `X-Kamacu-Token` envelope auth |
| Mounting the MCP server inside the existing Kamacu binary on a goroutine | The MCP subcommand MUST be a separate process spawned by the agent CLI — the agent CLI is the MCP *client*, Kamacu's `kamacu mcp serve` is the MCP *server*. A stdio MCP server is fundamentally a process whose stdin/stdout the client owns. Embedding it in the long-lived Kamacu server would require an HTTP/SSE transport, which is the wrong choice for v1.11. | `kamacu mcp serve` as a dedicated subcommand of the existing binary |
| Logging to stdout in the MCP subcommand | Stdio MCP servers MUST NOT write anything to stdout that isn't a valid MCP message — the spec is explicit. Any stray `fmt.Println` corrupts the JSON-RPC stream and breaks the agent connection. | `log/slog` to stderr (the existing project logger pattern) |

---

## Stack Patterns by Variant

**If the agent engine is Claude Code (default):**
- Write `.mcp.json` at the worktree root at task-spawn time with the shape above.
- Use `KAMACU_HOOK_TOKEN` literal in the file (already secret per-process) OR `${KAMACU_HOOK_TOKEN}` if you want the agent CLI to expand its own env. Literal is simpler and matches the existing hook-token injection.

**If the agent engine is opencode:**
- Write `opencode.json` at the worktree root. Remember the three shape differences from Claude Code: top-level `mcp` not `mcpServers`, `type: "local"` not `"stdio"`, `command` as a single array, env under `environment` not `env`.

**If the agent engine is `custom`:**
- Don't write any MCP config file by default. Custom agents may not speak MCP at all. Document the expected file shape per-CLI so users can opt in.

**If the inherited `KAMACU_HOOK_TOKEN` is missing or expired:**
- The MCP subcommand should start (so `/mcp list` from the agent shows it) but every tool call returns an MCP error result (`*mcp.CallToolResult` with `IsError: true` and a clear message pointing to restart). Do NOT crash-loop the subcommand — the agent CLI may give up after N restarts and the user loses the whole surface.

**If a future milestone wants external-editor MCP (Claude Desktop / Cursor):**
- Add a `kamacu mcp serve --transport http` mode using the same SDK's `StreamableHTTPHandler`. The handler registrations don't change — only the transport. This is exactly the stdio→HTTP upgrade path the SDK is designed for. Park as future work.

---

## Version Compatibility

| Package A | Compatible With | Notes |
|-----------|-----------------|-------|
| `github.com/modelcontextprotocol/go-sdk` v1.6.1 | **Go 1.25.0+** (Kamacu's `go 1.26` is fine) | Verified from the v1.6.1 `go.mod` raw file on GitHub. Go 1.26 is well within the supported window — Go's policy is the latest two major versions. |
| `github.com/modelcontextprotocol/go-sdk` v1.6.1 | **MCP spec 2025-11-25** (current published) | Per the README's Version Compatibility table, v1.4.0+ supports 2025-11-25, 2025-06-18, 2025-03-26, 2024-11-05. |
| `github.com/modelcontextprotocol/go-sdk` v1.7.0-pre.3 | MCP spec **2026-07-28** | Pre-release; not for v1.11 adoption. Tracks the in-progress spec rewrite. Useful to know the SDK is keeping up — when `2026-07-28` finalises and the SDK ships a stable v1.7.0, Kamacu can upgrade. |
| `github.com/modelcontextprotocol/go-sdk` v1.6.1 | **Claude Code** (current, ~v2.1.x) | Claude Code negotiates protocol version during initialize; falls back gracefully. Verified indirectly via the SDK's own conformance test suite, which exercises the same handshake. |
| `github.com/modelcontextprotocol/go-sdk` v1.6.1 | **opencode** (current) | Same handshake; opencode uses the same MCP spec. |
| `github.com/modelcontextprotocol/go-sdk` v1.6.1 | Kamacu's existing `coder/websocket` v1.8.14, `creack/pty` v1.1.24, `modernc.org/sqlite` v1.52.0, `pressly/goose/v3` v3.27.1 | Zero overlap in transitive deps. The SDK brings only its own small set; `golang.org/x/sys` is shared (already present via pty/websocket/sys). No version conflicts anticipated. |
| `mcp.AddTool` typed handlers | `encoding/json` reflection | Auto-derives JSON Schema from struct tags via `google/jsonschema-go`. Struct fields need `json:"name,omitempty"` and optionally `jsonschema:"description"` tags. |
| Stdio transport | OS pipe semantics | Standard on Linux/macOS/Windows. The SDK handles non-blocking reads, partial frames, and graceful shutdown on stdin EOF. |

---

## Integration points with existing Kamacu code (for the roadmap author)

- **New subcommand**: `kamacu mcp serve` — register in `cmd/kamacu/main.go` alongside the existing root command. Reads the `KAMACU_*` env vars, builds an `*http.Client` once, registers ~15 tools, calls `mcpSrv.Run(ctx, &mcp.StdioTransport{})`. Lives in a new `internal/mcp` package mirroring `internal/api` (REST handlers) and `internal/tmux` (CLI shell-out). Reuses the existing `internal/api/types.go` request/response structs where they exist (so the wire types stay in one place).
- **New spawn hook in the v1.10 spawn engine**: at task spawn, *after* worktree creation but *before* the agent CLI starts, write the appropriate MCP config file (`.mcp.json` for Claude, `opencode.json` for opencode) into the worktree root. This is a new step in the existing `internal/spawn` flow, gated on the agent's engine. One small leaf package (`internal/mcpconfig`) owning the two writers.
- **No schema migration**: all the MCP server needs is already in the DB. The MCP subcommand is a read-only-by-default view *of* Kamacu's API. The few state-changing tools (move task, create task, etc.) call the existing PATCH/POST endpoints; Kamacu's existing validation, worktree gating, and reaper logic apply unchanged.
- **Session-terminal read access**: PROJECT.md says MCP exposes read + subscribe on terminals (no keystroke injection). This is the one genuinely new capability. Two new read endpoints on the existing Kamacu HTTP API (or extend the existing `/api/sessions/{id}` shape): `GET /api/sessions/{id}/snapshot` (current ring-buffer contents — the same bytes the WS replay sends on attach) and `GET /api/sessions/{id}/tail?since=N` (incremental). The MCP `read_terminal` tool calls the snapshot endpoint; the `subscribe_terminal` tool (if v1.11 ships it) can either poll tail or upgrade to a long-lived tool call streaming chunks via `mcp.ProgressNotification` — defer the streaming variant to a later phase if it's risky.
- **Env-var reuse**: `KAMACU_HOOK_TOKEN`, `KAMACU_SESSION_ID`, `KAMACU_HOOK_BASE` are already injected at task spawn (v1.10). No new env vars. The MCP subcommand reads them once at startup.
- **No frontend changes**: v1.11 is backend-only per the milestone brief. If the UI later wants to show MCP tool-call counts or errors per session, the existing `/api/agents/status` poll can be extended — out of scope for v1.11.

---

## Why the official SDK over mcp-go — the decision in one paragraph

mcp-go has more GitHub stars (8.9k vs 4.8k) and a richer feature surface, and was the de-facto choice before May 2025 when the official SDK didn't exist. But for **a stdio bridge in a Go project that already ships as a single binary**, the official SDK is the right pick: (1) it's the canonical implementation under the `modelcontextprotocol` org, with conformance tests against the official suite; (2) it's v1.x stable-tagged while mcp-go is candidly pre-v1; (3) its API is more idiomatic Go (typed generic handlers with auto-derived JSON schemas vs mcp-go's runtime builder pattern); (4) it tracks the in-flux spec faster (pre-release already supports `2026-07-28`); (5) mcp-go's distinguishing features — CORS, OAuth Protected Resource Metadata, DNS rebinding protection, OTel, task tools, completion providers — are all aimed at HTTP/SaaS deployments and are dead weight in a localhost stdio bridge. The official SDK is the boring, correct choice.

---

## Sources

- [pkg.go.dev/github.com/modelcontextprotocol/go-sdk](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk) — v1.6.1 current stable, v1.7.0-pre.3 latest pre-release; license Apache-2.0/MIT; mcp package imported by 1,443 modules (verified 2026-07-21) — **HIGH**
- [pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk@v1.6.1/mcp) — full API surface: `mcp.NewServer`, `mcp.AddTool` (generic), `mcp.StdioTransport`, `mcp.CallToolRequest/Result`, sessions, middleware, all transport types — **HIGH**
- [raw go.mod for modelcontextprotocol/go-sdk v1.6.1](https://raw.githubusercontent.com/modelcontextprotocol/go-sdk/v1.6.1/go.mod) — `go 1.25.0`; transitive deps: jsonschema-go v0.4.3, uritemplate v3.0.2, segmentio/encoding v0.5.4, oauth2 v0.35.0, tools v0.42.0, golang-jwt/jwt/v5 v5.3.1 — **HIGH**
- [raw go.mod for mark3labs/mcp-go v0.56.0](https://raw.githubusercontent.com/mark3labs/mcp-go/v0.56.0/go.mod) — `go 1.25.5`; transitive deps: jsonschema-go v0.4.2, uuid v1.6.0, santhosh-tekuri/jsonschema/v6, spf13/cast, testify, uritemplate v3.0.2 — **HIGH**
- [github.com/modelcontextprotocol/go-sdk/releases](https://github.com/modelcontextprotocol/go-sdk/releases) — v1.6.1 (May 22 2026), v1.7.0-pre.1..pre.3 (Jun-Jul 2026) targeting `2026-07-28`; conformance tests added pre.2; OAuth session persistence pre.3 — **HIGH**
- [github.com/mark3labs/mcp-go/releases](https://github.com/mark3labs/mcp-go/releases) — v0.56.0 (Jul 9 2026); v0.x lineage confirmed; supports spec 2025-11-25 only — **HIGH**
- [raw examples/server/proxy/main.go from go-sdk v1.6.1](https://raw.githubusercontent.com/modelcontextprotocol/go-sdk/v1.6.1/examples/server/proxy/main.go) — the documented HTTP→HTTP MCP proxy pattern; structurally identical to Kamacu's stdio→HTTP bridge use case — **HIGH**
- [modelcontextprotocol.io/specification/2025-06-18/basic/transports](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports) — stdio = newline-delimited JSON-RPC over stdin/stdout (NOT Content-Length); server MAY write logs to stderr; Streamable HTTP is the alternative transport — **HIGH**
- [code.claude.com/docs/en/mcp](https://code.claude.com/docs/en/mcp) — Claude Code `.mcp.json` shape, three scopes (local/project/user), `${VAR}` expansion, `CLAUDE_PROJECT_DIR`, reserved server names, `list_changed` support — **HIGH**
- [opencode.ai/docs/mcp-servers](https://opencode.ai/docs/mcp-servers) — opencode `opencode.json` shape: `mcp` (not `mcpServers`), `type: "local"` (not `"stdio"`), `command` as array (not split), `environment` (not `env`); per-agent disable via `tools` glob — **HIGH**
- [modelcontextprotocol.io/quickstart/server](https://modelcontextprotocol.io/quickstart/server) — the stdio "never log to stdout" rule is universal across all SDK quickstarts — **HIGH**

---
*Stack research for: MCP (Model Context Protocol) server capability in a Go-backend single-binary app — stdio bridge to an existing HTTP API*
*Researched: 2026-07-21*
