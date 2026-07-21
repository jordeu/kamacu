# Phase 06: MCP Subcommand Foundation - Research

**Researched:** 2026-07-21
**Domain:** Go CLI dispatcher + MCP stdio server + HTTP bridge to a running Kamacu process
**Confidence:** HIGH

## Landmines (read before planning)

These are load-bearing surprises the planner MUST accommodate. Each is detailed inline below; this section exists so it cannot be missed.

1. **`google/subcommands` does NOT natively dispatch `kamacu mcp serve` as a 2-token path.** Its `Execute` looks only at the FIRST positional arg (the command name); "groups" are help-organization only. To get `kamacu mcp serve`, the `mcp` Command itself must own a nested dispatcher. See [Pattern 2](#pattern-2-nested-kamacu-mcp-serve-dispatch).
2. **The MCP Go SDK v1.6.1 has a non-trivial transitive dependency set** (`google/jsonschema-go`, `segmentio/encoding`, `yosida95/uritemplate/v3`, `golang-jwt/jwt/v5`, `golang.org/x/oauth2`, `golang.org/x/tools`). It is NOT zero-dep — only `google/subcommands` is. The Kamacu `go.mod` will gain ~6 direct + ~2 indirect dependencies. [VERIFIED: proxy.golang.org]
3. **The MCP Go SDK does NOT recover tool-handler panics.** A panicking handler crashes the whole process; there is no per-request `recover()` in `internal/jsonrpc2/conn.go::handleAsync`. The SC2 unit test must use a handler that RETURNS an error, not one that panics. See [Pitfall 1](#pitfall-1-sdk-does-not-recover-tool-handler-panics).
4. **`mcp.AddTool` PANICS at registration time if `InputSchema` is nil or not `{"type":"object"}`.** For the SC2 malformed-tool test, "register a malformed tool" must mean a handler that errors at runtime — schema malformation is a startup-time panic, not a runtime stream-desync test. See [Code Examples — list_projects tool registration](#list_projects-tool-registration).
5. **The SDK's default logger writes to `slog.DiscardHandler` (NOT stderr).** It is silent by default. The subcommand's own `slog.Default()` is the only thing that needs to keep off stdout — and Go's stdlib default already routes there. D-12(a) is satisfied trivially. See [D-12(a) slog posture](#d-12a-slog-posture).
6. **The SDK's internal-error sink is `log.Printf` (stdlib `log`), which writes to stderr by default** — set by `connect()` in `mcp/transport.go`. So even SDK-internal errors stay off stdout without any explicit configuration. [VERIFIED: source]
7. **`os.Stdin.Close()` in the SDK's `StdioTransport` is called by `ioConn.Close()`.** On stdin EOF the SDK returns cleanly from `Run` (it returns `nil`, not an error) — the subcommand's `Execute` should map that to `ExitStatus(0)`. See [Stdio lifecycle](#stdio-lifecycle).

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Adopt `github.com/google/subcommands`. Refactor `cmd/kamacu/main.go` to the `Command` interface. Today's flag-based server path becomes `serve`; `mcp serve` is a second Command. Existing `flag.Parse()` becomes the `serve` Command's `SetFlags`.
- **D-02:** Thin dispatch in main.go, body in `internal/mcp/`. main.go only registers Commands and dispatches. The MCP server body — SDK init, tool registration, stdio loop, env parsing — lives in a new `internal/mcp/` package.
- **D-03:** Subcommand names: `kamacu serve` (HTTP server, today's main.go body); `kamacu mcp serve` (MCP stdio subcommand); `kamacu` (no args) prints help. The MCP subcommand is registered as `mcp serve` (under a `mcp` command group with `serve` as its name).
- **D-04:** Break clean on `serve`. No transitional shim. Update README.md, docs/, Makefile, Taskfile, scripts/. Bare `kamacu` prints help (library default). Existing flags (`--addr`, `--db`, `--claude-bin`, `--dev-origin`, `--insecure-allow-remote`) become the `serve` Command's FlagSet; semantics unchanged.
- **D-05:** Register only `mcp serve` in Phase 06. No `mcp doctor` / `mcp tokens` / non-`mcp` Commands besides `serve` and the library's `help`.
- **D-06:** Loopback-only enforcement for v1.11. The MCP subcommand sends `KAMACU_HOOK_TOKEN` in the `X-Kamacu-Token` header on every bridged HTTP request; Phase 06 adds NO new server-side middleware on `/api/*`. Loopback binding (`hostCheck` + WS Origin allowlist + `ensureLoopback`) remains the actual auth boundary. Typed JSON-RPC error taxonomy (MCPHARD-01) deferred to v1.12.
- **D-07:** Header name is `X-Kamacu-Token`. Phase 06 renames `X-Kangent-Token` everywhere in one coordinated change (receiver + claude overlay + opencode plugin + tests). No fallback during transition.
- **D-08:** Proof tool is `list_projects(project_id?: int)`. Calls existing `GET /api/projects` UNCHANGED. Phase 06 returns all projects verbatim (the endpoint doesn't filter by `workspace_id` at the handler level today; Phase 07 may add it).
- **D-09:** Phase 06 proof tool is FINAL production code. Same `list_projects` ships unchanged in Phase 07. Phase 07 adds 12 more tools on the same pattern.
- **D-10:** `tools/list` returns ONE tool: `list_projects`. No stubs.
- **D-11:** SC2's malformed-tool regression test is a UNIT test localized to `internal/mcp/`. Registers a deliberately-malformed tool, asserts the SDK emits a clean JSON-RPC error response on stdout (never crashes, never emits a non-JSON line, never desyncs). No build-tagged e2e harness.
- **D-12:** Stdout cleanliness in Phase 06 relies on stdlib defaults + code-review convention. (a) Verify `slog.Default()` writes to stderr; (b) `internal/mcp/` package is `fmt.Print*`-free, `log.Print*`-free, `os.Stdout.Write`-free by convention — code review enforces; (c) no active stdout guard, no CI grep check. Full MCPHARD-02 is v1.12.

### the agent's Discretion
- **google/subcommands registration shape** — pick whatever the library idiom suggests. → See Pattern 2: nested `mcp` Command with an inner Commander.
- **`internal/mcp/` package file layout** — single `serve.go` vs. split. → Recommendation: 2-file split (`server.go` for SDK init + tool registration + Run; `bridge.go` for HTTP client + tool handlers). Phase 07 will add `tools_<resource>.go` files alongside.
- **MCP server identification** — name and version string. → Recommend `"kamacu"` + the binary's compile-time version string (or `"dev"` if unset).
- **HTTP client construction** — timeout, transport, retry. → Recommend 10s timeout, default transport, NO retry (fail fast).
- **Exact malformed-tool unit test cases** — which malformations. → Recommend: handler returns `errors.New("...")` (exercises JSON-RPC error path); handler returns `*CallToolResult{IsError: true, Content: [TextContent]}` (exercises MCP-level error path). Do NOT test handler-panic (SDK crashes the process; out of scope for Phase 06).
- **`list_projects` MCP output shape** — passthrough, wrap, or massage. → Recommend raw passthrough: marshal the Kamacu `[]Project` response as a single `TextContent{Text: string(jsonBytes)}` block. Agents parse JSON robustly; structured output is overkill for v1.11.
- **Stdin / lifecycle behavior** — graceful shutdown. → Follow SDK defaults: stdin EOF → `server.Run` returns nil → exit 0.

### Deferred Ideas (OUT OF SCOPE)
- MCPAUTO-01..03 (per-task auto-scoping) — v1.12.
- MCPREG-01..03 (agent-CLI auto-registration at spawn) — v1.12.
- MCPMORE-01..03 (additional tool categories) — v1.12.
- MCPHARD-01 (typed JSON-RPC error taxonomy) — v1.12.
- MCPHARD-02 (full stdout-pollution guards) — v1.12.
- MCPHARD-03 (real-binary e2e harness) — v1.12.
- `list_tasks` unscoped endpoint — Phase 07's call.
- Workspace filter on `list_projects` — Phase 07 may want it; Phase 06 returns all.
- Token auth on general `/api/*` — explicitly NOT in v1.11 (D-06).
- External-editor MCP server (Claude Desktop / Cursor / VS Code) — separate future milestone.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| MCPPROC-01 | `kamacu mcp serve` stdio subcommand starts an MCP server using `github.com/modelcontextprotocol/go-sdk` v1.6.1 and bridges tool calls to the running Kamacu HTTP API at `127.0.0.1:7333` | SDK v1.6.1 verified published May 22 2026; stdio server construction + bridge client pattern documented in [Standard Stack](#standard-stack), [Pattern 1](#pattern-1-mcp-stdio-server-construction) and [Pattern 3](#pattern-3-bridge-client-construction); SDK requires Go 1.25, Kamacu on Go 1.26 ✅ |
| MCPPROC-02 | MCP subcommand authenticates to the Kamacu HTTP API via `KAMACU_HOOK_TOKEN` env (reuses the existing envelope — already injected into spawned agents) | Env var name `KAMACU_HOOK_TOKEN` confirmed present in `internal/session/manager.go:186` injection for the opencode engine; MCP subcommand reads it from `os.Getenv` and constructs the `X-Kamacu-Token` HTTP header on every bridge request (per D-07) |
| MCPPROC-03 | MCP subcommand locates the Kamacu HTTP base URL via `KAMACU_HOOK_BASE` env (already injected) with a `127.0.0.1:7333` default | Env var name `KAMACU_HOOK_BASE` confirmed present in `internal/session/manager.go:187`; default `http://127.0.0.1:7333` when unset (matches the existing `flag.String("addr", "127.0.0.1:7333", ...)` default in `cmd/kamacu/main.go:36`) |
</phase_requirements>

## Summary

Phase 06 is a focused plumbing phase: refactor `cmd/kamacu/main.go` onto `github.com/google/subcommands`, add a new `internal/mcp/` package whose body is a long-lived stdio MCP server built on `github.com/modelcontextprotocol/go-sdk` v1.6.1, bridge exactly ONE read-only tool (`list_projects`) to the existing Kamacu HTTP API, and rename the `X-Kangent-Token` HTTP header to `X-Kamacu-Token` in one coordinated sweep across Go + JS + tests. Nothing else — no DB, no migrations, no frontend, no new endpoints, no new long-lived goroutines inside the Kamacu binary, no token-validation middleware.

The technical surface is small and now well-mapped. The MCP SDK v1.6.1 API is verified at the source level (constructor signatures, tool registration, transport behavior, panic posture, EOF behavior, default logger). `google/subcommands` v1.2.0 is verified — and the load-bearing surprise is that it does NOT natively dispatch 2-token paths like `kamacu mcp serve`, so the `mcp` Command must own its own nested dispatcher (a one-struct pattern, documented in Pattern 2). The bridge itself is the simplest possible HTTP client (env parse → base URL → token header → GET → passthrough), proven against the unchanged `GET /api/projects` endpoint.

**Primary recommendation:** Write `internal/mcp/` as a 2-file package (`server.go` for SDK init/tool-registration/Run; `bridge.go` for the HTTP client + tool handlers), test it with stdlib `httptest` (the project's established pattern in `internal/api/agent_integration_test.go:48-73`), and pin the SC2 unit test against the SDK's actual error-vs-panic semantics (handlers that return errors → clean JSON-RPC error response; handlers that panic → process crash, which is out of scope for Phase 06).

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| CLI dispatch (`kamacu` / `kamacu serve` / `kamacu mcp serve`) | `cmd/kamacu/main.go` (thin) | `google/subcommands` library | One-paragraph dispatcher; all real logic in versioned internal packages (existing project pattern) |
| HTTP server (today's `main.go` body) | `cmd/kamacu/main.go` `serveCmd.Execute` | — | Refactor is mechanical (move flag declarations into `SetFlags`, body into `Execute`); no behavior change |
| MCP stdio server construction + tool registration + Run loop | `internal/mcp/server.go` | MCP SDK | Body lives in versioned package so it's testable without building the binary (existing project convention) |
| MCP tool → HTTP bridge | `internal/mcp/bridge.go` | net/http stdlib | One client per subcommand process; reads `KAMACU_HOOK_TOKEN` + `KAMACU_HOOK_BASE` from env; calls existing `/api/*` endpoints unchanged |
| Hook-token header name | All Go + JS + tests | — | Coordinated rename `X-Kangent-Token` → `X-Kamacu-Token`; one-shoot, no fallback (D-07) |
| Loopback auth boundary | Existing `cmd/kamacu/main.go::hostCheck` + `ensureLoopback` + WS Origin allowlist | — | D-06: Phase 06 does NOT add server-side token middleware on `/api/*` |

## Standard Stack

### Core (new in Phase 06)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/modelcontextprotocol/go-sdk` | **v1.6.1** (verified via proxy.golang.org; published May 22 2026, current stable line; v1.7.0-pre.3 is pre-release) | MCP stdio server construction, tool registration, JSON-RPC over stdio loop | Official Go SDK maintained under the `modelcontextprotocol` org; supports MCP spec 2025-11-25, 2025-06-18, 2025-03-26, 2024-11-05. [VERIFIED: pkg.go.dev + proxy.golang.org] |
| `github.com/google/subcommands` | **v1.2.0** (verified via proxy.golang.org; published Sep 2019, stable for 6+ years) | CLI subcommand dispatcher | Lives under `github.com/google/`, stdlib-adjacent, zero transitive deps. The Go-blessed way to add subcommands without pulling cobra. [VERIFIED: proxy.golang.org + source] |

### Supporting (carried over — NOT new)

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `net/http` (stdlib) | go1.26 | Bridge HTTP client (`http.Client` with timeout) | Inside `internal/mcp/bridge.go` to call the running Kamacu server's `/api/*` |
| `log/slog` (stdlib) | go1.26 | Structured logging | All logging in `internal/mcp/` goes through `slog.Default()`, which writes to **stderr** by Go stdlib default. NEVER `fmt.Println` / `log.Print` in `internal/mcp/` |
| `encoding/json` (stdlib) | go1.26 | Translate Kamacu response → MCP tool content | `bridge.go` marshals the response body as a `TextContent` block |

### Transitive deps the SDK will add to go.mod

Verified from the v1.6.1 `go.mod` (`https://proxy.golang.org/github.com/modelcontextprotocol/go-sdk/@v/v1.6.1.mod`):

| Transitive dep | Version | Why pulled in | Risk |
|----------------|---------|---------------|------|
| `github.com/google/jsonschema-go` | v0.4.3 | JSON Schema inference for `AddTool[In,Out any]` typed handlers | Low — Google-maintained, used only at tool-registration time |
| `github.com/segmentio/encoding` + `segmentio/asm` | v0.5.4 / v1.1.3 | Fast JSON for the wire layer | Low — widely deployed |
| `github.com/yosida95/uritemplate/v3` | v3.0.2 | URI templates (resource feature; not used by Phase 06) | Low |
| `github.com/golang-jwt/jwt/v5` | v5.3.1 | OAuth support (not used by stdio transport) | Low |
| `golang.org/x/oauth2` | v0.35.0 | OAuth (not used) | Low — already common in Go ecosystems |
| `golang.org/x/tools` | v0.42.0 | (Indirect via OAuth/tools chain) | Low |

The SDK requires **`go 1.25.0`** in its `go.mod`; Kamacu is on `go 1.26` ✅.

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `google/subcommands` | `spf13/cobra` | Cobra is the heavyweight default; subcommands is stdlib-adjacent and matches the project's no-framework posture (the existing `net/http` ServeMux choice). Stick with subcommands. |
| `modelcontextprotocol/go-sdk` | `mark3labs/mcp-go` (community) | The official SDK is now the canonical choice (mark3labs is acknowledged in the official SDK's README as inspiration). The official SDK is what `MCPPROC-01` locks; do not explore alternatives. |
| Raw passthrough for `list_projects` output | `StructuredContent` typed output | Phase 06 is the architecture proof — raw JSON passthrough keeps `bridge.go` trivial and is exactly the shape Phase 07 will repeat 12 more times. Typed `StructuredContent` is a v1.12 concern. |

**Installation:**
```bash
go get github.com/modelcontextprotocol/go-sdk@v1.6.1
go get github.com/google/subcommands@v1.2.0
go mod tidy
```

## Package Legitimacy Audit

> The `gsd-tools query package-legitimacy` tool was invoked against npm — wrong ecosystem (these are Go modules). The authoritative Go registry (proxy.golang.org) was queried directly for verification.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| `github.com/modelcontextprotocol/go-sdk` | proxy.golang.org | v1.6.1 published May 22 2026 (~2 months ago at research time) | n/a (Go modules have no per-package download counter; tracked via pkg.go.dev) | github.com/modelcontextprotocol/go-sdk (official MCP org) | OK | Approved — verified via proxy.golang.org `@latest` + pkg.go.dev + raw source read of `mcp/server.go`, `mcp/tool.go`, `mcp/transport.go`, `mcp/protocol.go`, `mcp/content.go`, `mcp/logging.go`, `internal/jsonrpc2/conn.go`, `internal/jsonrpc2/jsonrpc2.go` |
| `github.com/google/subcommands` | proxy.golang.org | v1.2.0 published Sep 2019 (~7 years stable) | n/a | github.com/google/subcommands (under the `google/` org) | OK | Approved — verified via proxy.golang.org `@latest` + raw source read of `subcommands.go` |

**Packages removed due to [SLOP] verdict:** none.
**Packages flagged as suspicious [SUS]:** none.

Both packages are referenced as locked decisions in CONTEXT.md (D-01, MCPPROC-01) and were verified directly against authoritative sources (proxy.golang.org + raw GitHub source). No `npm`-registry-based verdict is meaningful for these Go modules; the Go proxy + direct source verification supersedes it.

## Architecture Patterns

### System Architecture Diagram

```
                    ┌────────────────────────────────────────────────────────┐
                    │                   User's shell                         │
                    │                                                        │
                    │   $ kamacu mcp serve  (no useful flags in Phase 06)    │
                    └─────────────────┬──────────────────────────────────────┘
                                      │ exec
                                      ▼
            ┌─────────────────────────────────────────────────────────────┐
            │  kamacu mcp serve process (separate from kamacu serve)      │
            │                                                             │
            │  ┌─────────────────────────────────────────────────────┐    │
            │  │  cmd/kamacu/main.go                                  │    │
            │  │   subcommands.Register(serveCmd{}, "")              │    │
            │  │   subcommands.Register(mcpCmd{}, "")   ← nested     │    │
            │  │   subcommands.Register(subcommands.HelpCommand(),"")│    │
            │  │   flag.Parse()                                      │    │
            │  │   os.Exit(int(subcommands.Execute(ctx)))            │    │
            │  └────────────────┬────────────────────────────────────┘    │
            │                   │ dispatch to mcpCmd.Execute               │
            │                   ▼                                          │
            │  ┌─────────────────────────────────────────────────────┐    │
            │  │  internal/mcp/server.go                              │    │
            │  │   s := mcp.NewServer(&mcp.Implementation{            │    │
            │  │            Name: "kamacu", Version: <build>,         │    │
            │  │        }, nil)                                       │    │
            │  │   mcp.AddTool(s, &mcp.Tool{                          │    │
            │  │            Name: "list_projects",                    │    │
            │  │            Description: "...",                      │    │
            │  │        }, listProjectsHandler)                       │    │
            │  │   err := s.Run(ctx, &mcp.StdioTransport{})           │    │
            │  │   // blocks until stdin EOF or ctx cancel            │    │
            │  └────────────────┬────────────────────────────────────┘    │
            │                   │                                          │
            │  stdin  ◄──── JSON-RPC requests (tools/list, tools/call)    │
            │  stdout ────► JSON-RPC responses  (OWNED BY SDK)            │
            │  stderr ────► slog.Default()  (free for app logs)           │
            │                                                             │
            │  ┌─────────────────────────────────────────────────────┐    │
            │  │  internal/mcp/bridge.go  (invoked by the tool        │    │
            │  │  handler on each tools/call)                         │    │
            │  │                                                      │    │
            │  │   baseURL := os.Getenv("KAMACU_HOOK_BASE")           │    │
            │  │            ?: "http://127.0.0.1:7333"                │    │
            │  │   token   := os.Getenv("KAMACU_HOOK_TOKEN")          │    │
            │  │                                                      │    │
            │  │   req, _ := http.NewRequest("GET", baseURL+"/api/    │    │
            │  │                              projects", nil)          │    │
            │  │   req.Header.Set("X-Kamacu-Token", token)            │    │
            │  │   resp, err := httpClient.Do(req)                    │    │
            │  │   …                                                  │    │
            │  └────────────────┬────────────────────────────────────┘    │
            └───────────────────┼─────────────────────────────────────────┘
                                │ HTTP GET (loopback only)
                                ▼
            ┌─────────────────────────────────────────────────────────────┐
            │  kamacu serve process  (unchanged in Phase 06)             │
            │  internal/api/projects.go::projectHandlers.list            │
            │  → GET /api/projects → []Project JSON                       │
            │  (loopback-bound; X-Kamacu-Token ignored per D-06)         │
            └─────────────────────────────────────────────────────────────┘
```

**Reading the diagram:** the agent CLI spawns `kamacu mcp serve` as a child process; its stdin/stdout carry JSON-RPC; the subcommand is just another loopback HTTP client of the Kamacu server. The Kamacu server has no awareness of the MCP subcommand's existence — it sees HTTP requests from loopback with an `X-Kamacu-Token` header.

### Recommended Project Structure

```
cmd/kamacu/
├── main.go              # REFACTORED: thin subcommands.Register + dispatch
├── serve.go             # NEW: serveCmd{} Command (today's main.go body)
└── mcp.go               # NEW: mcpCmd{} Command (owns nested dispatcher)
internal/mcp/
├── server.go            # NEW: SDK init, tool registration, s.Run
├── bridge.go            # NEW: HTTP client + tool handler bodies
├── server_test.go       # NEW: SC2 malformed-tool regression test (D-11)
└── bridge_test.go       # NEW: httptest-based bridge integration test
internal/api/
├── hooks.go             # EDIT: X-Kangent-Token → X-Kamacu-Token (line 39)
└── hooks_test.go        # EDIT: header literal (line 87) + comment (line 103)
internal/session/
├── agent.go             # EDIT: header literal (lines 23, 63)
└── agent_test.go        # EDIT: header literal assertion (line 94)
internal/opencode/
├── kamacu-status.js     # EDIT: header literal (line 79)
├── plugin_test.go       # EDIT: header literal must-contain (line 195)
└── e2e_test.go          # EDIT: header literal (lines 136, 144)
```

### Pattern 1: MCP stdio server construction
**What:** Minimal MCP server over stdio with one tool registered.
**When to use:** The exact body of `internal/mcp/server.go::Run`.
**Example:**
```go
// Source: github.com/modelcontextprotocol/go-sdk@v1.6.1/mcp/server.go (NewServer, Server.Run)
//         + README example at https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk@v1.6.1
package mcp

import (
	"context"
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/google/subcommands"
)

type serveCmd struct {  // registered under "mcp" — see Pattern 2 for the wrapping mcpCmd
	// no flags in Phase 06 (env-driven); add --base / --token later if needed
}

func (serveCmd) Name() string     { return "serve" }
func (serveCmd) Synopsis() string { return "run the Kamacu MCP stdio server" }
func (serveCmd) Usage() string {
	return `mcp serve:
  Runs the Kamacu MCP server over stdio. Reads KAMACU_HOOK_BASE
  (default http://127.0.0.1:7333) and KAMACU_HOOK_TOKEN from env.
  Designed to be spawned by an agent CLI (claude/opencode) inside a
  Kamacu task PTY — see the v1.11 milestone docs.
`
}
func (serveCmd) SetFlags(*flag.FlagSet) {}  // no flags
func (c serveCmd) Execute(ctx context.Context, _ *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	// Default slog goes to stderr (Go stdlib); nothing in this package
	// redirects it. SDK's own default logger discards logs entirely, so
	// the SDK never touches stdout OR stderr unless we hand it a logger.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	srv := mcp.NewServer(
		&mcp.Implementation{
			Name:    "kamacu",
			Version: "dev", // TODO: wire to a compile-time version string
		},
		nil, // *ServerOptions — defaults are fine; Logger=nil → slog.DiscardHandler
	)
	registerTools(srv) // adds list_projects

	// Run blocks until ctx is cancelled OR stdin hits EOF (returns nil).
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
		// ctx cancel returns ctx.Err(); any other error is exceptional.
		slog.Error("mcp server ended with error", "error", err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}
```

### Pattern 2: Nested `kamacu mcp serve` dispatch
**What:** `google/subcommands`'s `Execute` only looks at the FIRST positional arg. To get `kamacu mcp serve`, the `mcp` Command owns its own inner dispatcher.
**When to use:** This is the locked D-03 subcommand name (`mcp serve`).
**Why:** Verified from `subcommands.go::Execute` (line 174 of master):
```go
func (cdr *Commander) Execute(ctx context.Context, args ...interface{}) ExitStatus {
	if cdr.topFlags.NArg() < 1 { /* help */ }
	name := cdr.topFlags.Arg(0)  // ONLY first arg
	for _, group := range cdr.commands {
		for _, cmd := range group.commands {
			if name != cmd.Name() { continue }
			// dispatches with cdr.topFlags.Args()[1:] as the sub-args
		}
	}
}
```
"Groups" are help-organization only — they do NOT nest dispatch.

**Example:**
```go
// cmd/kamacu/mcp.go
package main

import (
	"context"
	"flag"

	"github.com/google/subcommands"
	kamacumcp "kamacu/internal/mcp"
)

type mcpCmd struct{}

func (mcpCmd) Name() string     { return "mcp" }
func (mcpCmd) Synopsis() string { return "MCP-related subcommands" }
func (mcpCmd) Usage() string {
	return `mcp <subcommand>:
  mcp serve   Run the Kamacu MCP stdio server.
`
}
func (mcpCmd) SetFlags(*flag.FlagSet) {}
func (mcpCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	// Inner dispatcher for `mcp <subcommand>`. In Phase 06 only `serve` is
	// registered; D-05 says no future-subcommand stubs.
	cdr := subcommands.NewCommander(f, "kamacu mcp")
	cdr.Register(subcommands.HelpCommand(cdr), "")
	cdr.Register(kamacumcp.ServeCommand{}, "") // see Pattern 1 for ServeCommand
	return cdr.Execute(ctx)
}
```

And in `main.go`:
```go
func main() {
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(serveCmd{}, "")   // `kamacu serve`
	subcommands.Register(mcpCmd{}, "")     // `kamacu mcp …`
	flag.Parse()
	os.Exit(int(subcommands.Execute(context.Background())))
}
```

### Pattern 3: Bridge client construction
**What:** One `http.Client` per MCP subcommand process, configured conservatively.
**When to use:** Inside `internal/mcp/bridge.go`, called by each tool handler.
**Example:**
```go
// internal/mcp/bridge.go
package mcp

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"
)

// bridge is the per-process Kamacu HTTP client. Constructed ONCE in Run(),
// shared by all tool handlers. Loopback-only by Kamacu's own --addr gate.
type bridge struct {
	base    string          // e.g. "http://127.0.0.1:7333" — no trailing slash
	token   string          // X-Kamacu-Token; empty is allowed (Kamacu ignores on /api/*)
	client  *http.Client
}

func newBridgeFromEnv() (*bridge, error) {
	base := os.Getenv("KAMACU_HOOK_BASE")
	if base == "" {
		base = "http://127.0.0.1:7333"
	}
	if _, err := url.Parse(base); err != nil {
		return nil, fmt.Errorf("invalid KAMACU_HOOK_BASE %q: %w", base, err)
	}
	return &bridge{
		base:  strings.TrimRight(base, "/"),
		token: os.Getenv("KAMACU_HOOK_TOKEN"), // may be ""; Kamacu ignores on /api/* (D-06)
		client: &http.Client{
			Timeout: 10 * time.Second, // fail fast; Phase 06 has no streaming tools
			// default Transport is fine — single user, localhost
		},
	}, nil
}

// do issues a bridge request with the X-Kamacu-Token header set. The header
// is sent even though Kamacu ignores it on /api/* in v1.11 (D-06); doing so
// (a) makes the bridge code correct against any future server-side check, and
// (b) makes the wire contract match the existing hook receiver.
func (b *bridge) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, b.base+path, body)
	if err != nil { return nil, err }
	req.Header.Set("X-Kamacu-Token", b.token)
	return b.client.Do(req)
}
```

### list_projects tool registration

**What:** The single tool Phase 06 ships. Calls `GET /api/projects` unchanged and returns the response as a single TextContent block.
**Why raw passthrough (per agent's Discretion):** keeps `bridge.go` to ~15 lines; agents parse JSON robustly; structured output is overkill for v1.11 and would force every Phase 07 tool to declare an output type. Massaging is a v1.12 concern.

**Kamacu endpoint response shape (verified from `internal/api/projects.go::projectHandlers.list`, line 127):**
```go
// writeJSON(w, http.StatusOK, projects)  where projects is []Project
//
// Wire shape: an array of:
type Project struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	RepoPath    string  `json:"repo_path"`
	Description string  `json:"description"`
	GithubRepo  *string `json:"github_repo"`
	Managed     bool    `json:"managed"`
	IconLetters string  `json:"icon_letters"`
	IconColor   string  `json:"icon_color"`
	WorkspaceID int64   `json:"workspace_id"`
	AgentID     int64   `json:"agent_id"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}
```

**Note on `project_id` arg (D-08):** the endpoint does NOT accept a `project_id` query param — it always returns all projects. The MCP tool's `project_id?` parameter is part of the locked signature (MCPPROC + D-08) but is a no-op in Phase 06 (when supplied, the handler ignores it and the description notes that filtering arrives in Phase 07). Do not actually call `/api/projects?project_id=…` — it would be silently ignored today.

**Tool registration and handler (using the low-level `Server.AddTool` so we control exactly what's emitted):**
```go
// internal/mcp/server.go
func registerTools(s *mcp.Server) {
	// InputSchema is REQUIRED and must be a JSON Schema with type:"object"
	// (AddTool panics otherwise). Use json.RawMessage to avoid pulling in
	// jsonschema-go's typed API for this trivial no-required-args schema.
	s.AddTool(
		&mcp.Tool{
			Name:        "list_projects",
			Description: "List all Kamacu projects (optionally filtered by project_id; today returns all, ignores the arg).",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"project_id": {
						"type": "integer",
						"description": "Optional project id. Currently ignored — list_projects always returns all projects. Will be honored in Phase 07."
					}
				}
			}`),
		},
		listProjectsHandler,
	)
}

// listProjectsHandler is a low-level ToolHandler (not a ToolHandlerFor[In,Out])
// because Phase 06 does not need typed argument parsing — the optional
// project_id is intentionally a no-op (D-08), so accepting raw arguments and
// ignoring them is the simplest correct shape.
func listProjectsHandler(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	bridge := bridgeFromContext(ctx) // set by Run; bridgeFromContext is a tiny helper
	resp, err := bridge.do(ctx, http.MethodGet, "/api/projects", nil)
	if err != nil {
		// Network error → MCP-level error (the agent can self-correct)
		return nil, fmt.Errorf("kamacu bridge: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kamacu GET /api/projects: HTTP %d: %s", resp.StatusCode, string(body))
	}
	// Raw passthrough: ship the JSON body as a single text content block.
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(body)},
		},
	}, nil
}
```

### Anti-Patterns to Avoid

- **Don't use `mcp.AddTool[In, Out any]` (the typed helper) in Phase 06.** The typed helper auto-generates a schema from the In struct via `google/jsonschema-go` reflection. For a no-op optional arg that's overkill and would force every Phase 07 tool to declare a Go struct that mirrors its wire schema. Use the low-level `Server.AddTool` with an explicit `json.RawMessage` schema — it's 3 lines of schema JSON and keeps `bridge.go` readable.
- **Don't put the bridge HTTP call inline in the handler.** Wrap it in a `*bridge` type so the SC2 unit test (D-11) and the Phase 07 httptest integration test can inject a fake bridge. Set it via context or a package-level constructor called from `Run`.
- **Don't write to stdout.** Ever. From anywhere in `internal/mcp/`. Code review enforces; no automated check (D-12).
- **Don't use `flag.Parse()` inside `serveCmd.SetFlags` or `mcpCmd.SetFlags`.** The library owns the FlagSet; declare flags on the provided `*flag.FlagSet` and the library parses them.
- **Don't try to dispatch `kamacu mcp serve` by registering a command literally named `"mcp serve"` with a space in it.** Use Pattern 2 (nested Commander inside `mcpCmd.Execute`).
- **Don't try to set `server.SetFlags` to register a `--addr` flag on `mcp serve`.** Phase 06 is env-driven; flags are deferred.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| JSON-RPC framing over stdio | Newline-delimited JSON encoder/decoder | `mcp.StdioTransport{}` | The SDK owns stdin/stdout and handles concurrent-write safety, framing, EOF, and the protocol's batching rules (which changed in 2025-06-18+). Hand-rolling would have to track spec drift. |
| Tool schema validation | Manual `json.Unmarshal` into interface + range-over-fields | `mcp.AddTool[In,Out any]` OR explicit `json.RawMessage` schema | The SDK panics if InputSchema isn't `type:"object"` — the safety net is built-in. |
| Subcommand dispatch | A `switch os.Args[1]` block | `github.com/google/subcommands` | D-01 locks this; the library handles help, flag-sets-per-command, exit-status propagation. |
| HTTP client retry / circuit breaking | Custom backoff | None — fail fast (D-09 the agent's Discretion) | Loopback to a single-user local server. A timeout means the server is down; surfacing the error is the right behavior. |
| Token validation middleware on `/api/*` | A new middleware in `internal/api/` | Nothing (deferred per D-06) | Loopback binding is the v1.11 auth boundary. Adding middleware would have SPA impact that needs its own phase. |

**Key insight:** Phase 06's value is the **bridge pattern**, not novel infrastructure. Every primitive already exists (HTTP server, stdio MCP SDK, env vars, hook token envelope). The phase proves the pattern by composing them; Phase 07–09 repeat the composition.

## Common Pitfalls

### Pitfall 1: SDK does NOT recover tool-handler panics
**What goes wrong:** A tool handler (or any code it calls) panics. The process crashes. No JSON-RPC error is emitted; stdout may contain a partial message.
**Why it happens:** The SDK's `internal/jsonrpc2/conn.go::handleAsync` spawns handler goroutines without `recover()`:
```go
go func() {
    defer releaser.release(true)
    result, err := c.handler.Handle(ctx, req.Request)  // ← panic escapes here
    c.processResult(c.handler, req, result, err)
}()
```
**How to avoid:** The SC2 unit test (D-11) MUST use a handler that returns an error, not one that panics. Two test cases cover the SDK's actual error paths:
1. Handler returns `(nil, errors.New("..."))` → SDK emits a JSON-RPC error response (`error.code = -32603`, `error.message = "..."`).
2. Handler returns `(&mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "boom"}}}, nil)` → SDK emits a JSON-RPC *success* response with `result.isError: true` (the MCP-level error path; the LLM sees and can self-correct).

For a panic-safety belt-and-braces, wrap each tool handler body in a `defer recover()` that converts to a returned error. Optional in Phase 06; recommended given Phase 08 introduces long-running `subscribe_session_output` goroutines that have more panic surface area.
**Warning signs:** Test that asserts "panic produces clean JSON-RPC error" will fail against the SDK as-shipped. Don't write that test.

### Pitfall 2: `mcp.AddTool` panics at registration time on missing/invalid schema
**What goes wrong:** Calling `s.AddTool(&mcp.Tool{Name: "x"}, handler)` without an `InputSchema` panics on the main goroutine during startup — before the server starts. The crash message is `panic: AddTool "x": missing input schema`.
**Why it happens:** `mcp/server.go::Server.AddTool` explicitly panics:
```go
if t.InputSchema == nil {
    panic(fmt.Errorf("AddTool %q: missing input schema", t.Name))
}
// also panics if the schema is not type:"object"
```
**How to avoid:** Always set `InputSchema` to a JSON Schema object. For a tool with no parameters, the minimal valid schema is `json.RawMessage(`{"type":"object"}`)`.
**Warning signs:** Process exits with a Go panic stack trace on stderr before any client connects.

### Pitfall 3: Forgetting that `google/subcommands` is flat, not nested
**What goes wrong:** Registering a Command literally named `"mcp serve"` (with a space) doesn't work — the library matches on the FIRST positional arg only. `kamacu mcp serve` ends up looking for a command named `"mcp"` and passing `["serve"]` as its args.
**Why it happens:** Group names in the Register call are for HELP ORGANIZATION ONLY. The Execute method is documented as `name := cdr.topFlags.Arg(0)` (verified in source).
**How to avoid:** Use Pattern 2 — register a `mcpCmd` at the top level whose own Execute constructs an inner `subcommands.Commander` and dispatches.
**Warning signs:** `kamacu mcp serve` prints top-level help instead of starting the MCP server.

### Pitfall 4: Stdout pollution from the SDK's internal `log.Printf`
**What goes wrong:** Some other (non-MCP) code path in the SDK calls `log.Printf`, which by default writes to stderr. But if the Kamacu binary (or any imported package) calls `log.SetOutput(os.Stdout)` at init time, those logs would land on stdout and corrupt the JSON-RPC stream.
**Why it happens:** The SDK sets `OnInternalError: func(err error) { log.Printf("jsonrpc2 error: %v", err) }` inside `mcp/transport.go::connect`. The stdlib `log` package defaults to stderr, but package-level `init()` functions anywhere in the import graph could change that.
**How to avoid:** The `internal/mcp/` package's `Run` is the ONLY place the subcommand's code is invoked. Audit that path: `cmd/kamacu/main.go` dispatches; `internal/mcp/` runs. None of `internal/mcp/`'s imports touch `log.SetOutput` (verified by grep — only `log/slog` is used elsewhere in the project; stdlib `log` is not used anywhere in `internal/`). The Kamacu server binary's startup path (which DOES use `log/slog`) is never invoked by `mcp serve`.
**Warning signs:** A client connecting via stdio reports `invalid JSON` errors intermittently.

### Pitfall 5: `kamacu mcp serve` exit code on stdin EOF
**What goes wrong:** The SDK's `Run` returns `nil` on stdin EOF (clean shutdown). If `mcpCmd.Execute` returns `subcommands.ExitFailure` on nil, the agent CLI sees a non-zero exit and may treat it as a crash.
**Why it happens:** Pattern: `if err := srv.Run(...); err != nil { return ExitFailure }`. Easy to forget the happy-path mapping.
**How to avoid:** Map `nil` from `Run` to `subcommands.ExitSuccess` (0). Map `ctx.Err()` to `subcommands.ExitSuccess` too (cancelled by parent — also clean). Map everything else to `ExitFailure`.
**Warning signs:** `echo | kamacu mcp serve; echo $?` should print `0`, not `1`.

### Pitfall 6: Coordinated rename missing the JS plugin
**What goes wrong:** The Go side renames `X-Kangent-Token` → `X-Kamacu-Token` everywhere, but the on-disk opencode plugin still ships the old header. opencode tasks POST with the wrong header; the hook receiver rejects with 401; agent status hooks silently die.
**Why it happens:** The plugin is regenerated by `opencode.InstallPlugin` only on the next Kamacu server boot (D-07). The Kamacu server is NOT running between the rename and the next `kamacu serve` — so existing installs of `kamacu-status.js` are stale until then.
**How to avoid:** Two-layer protection (already in place): (a) the rename lands in `internal/opencode/kamacu-status.js` (the embedded source) AND (b) the rename lands in the receiver (`internal/api/hooks.go`). After the next `kamacu serve`, `InstallPlugin` regenerates the on-disk file with the new header. Until then, opencode tasks in already-running sessions continue to POST with the OLD header — but the receiver still accepts the old name UNTIL the new release is running (single-user, fresh-per-start token, server+agents always same-release is the D-07 invariant). The break-clean posture is intentional.
**Warning signs:** An opencode task's status gets stuck on "working" after a server restart with the new release. (This is expected once during the rename; subsequent restarts are fine.)

## Code Examples

Verified patterns from authoritative sources (v1.6.1 source + pkg.go.dev README):

### Minimal stdio MCP server with one tool (canonical hello-world from the SDK README)
```go
// Source: https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk@v1.6.1 (README example)
package main

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	Name string `json:"name" jsonschema:"the name of the person to greet"`
}

type Output struct {
	Greeting string `json:"greeting" jsonschema:"the greeting to tell to the user"`
}

func SayHi(ctx context.Context, req *mcp.CallToolRequest, input Input) (
	*mcp.CallToolResult,
	Output,
	error,
) {
	return nil, Output{Greeting: "Hi " + input.Name}, nil
}

func main() {
	server := mcp.NewServer(&mcp.Implementation{Name: "greeter", Version: "v1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "greet", Description: "say hi"}, SayHi)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
```

### Low-level tool registration with explicit schema (the Phase 06 pattern)
```go
// Source: github.com/modelcontextprotocol/go-sdk@v1.6.1/mcp/server.go (Server.AddTool + Tool type)
//         + mcp/tool.go (ToolHandler signature)
//         + mcp/protocol.go (CallToolResult, CallToolParamsRaw)
//         + mcp/content.go (TextContent)
s := mcp.NewServer(&mcp.Implementation{Name: "kamacu", Version: "dev"}, nil)

// ToolHandler signature: func(context.Context, *CallToolRequest) (*CallToolResult, error)
// CallToolRequest is an alias for ServerRequest[*CallToolParamsRaw] (verified in protocol.go).
// CallToolParamsRaw carries Arguments as json.RawMessage — caller unmarshals.
var handler mcp.ToolHandler = func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// req.Params.Arguments is json.RawMessage; unmarshal if you care.
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: `{"hello":"world"}`},
		},
		// IsError: false (default) → MCP success response
		// IsError: true           → MCP success response with result.isError=true
		//                            (the LLM sees it and self-corrects)
	}, nil
}

s.AddTool(&mcp.Tool{
	Name:        "noop",
	Description: "proof-of-concept",
	InputSchema: json.RawMessage(`{"type":"object"}`), // REQUIRED, type:"object" — AddTool panics otherwise
}, handler)

// Returning an error: emits a JSON-RPC error response with error.code = -32603.
var errHandler mcp.ToolHandler = func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return nil, errors.New("kamacu_down: bridge connection refused")
}

if err := s.Run(ctx, &mcp.StdioTransport{}); err != nil {
	// ctx cancel → returns ctx.Err(); stdin EOF → returns nil.
	log.Fatal(err)
}
```

### Token-receiver coordinated rename (the D-07 surface)
**Surface inventory — every `X-Kangent-Token` occurrence in the repo (verified via `grep -r X-Kangent-Token`):**

| File | Line | What | Phase 06 action |
|------|------|------|-----------------|
| `internal/api/hooks.go` | 39 | `got := r.Header.Get("X-Kangent-Token")` | Rename literal → `"X-Kamacu-Token"` |
| `internal/api/hooks.go` | 17–18 | Comments referencing the header name | Update text |
| `internal/api/hooks_test.go` | 87 | `req.Header.Set("X-Kangent-Token", token)` | Rename literal |
| `internal/api/hooks_test.go` | 103 | Comment "X-Kangent-Token and never mutates..." | Update text |
| `internal/session/agent.go` | 23 | `Token string // per-instance X-Kangent-Token value` | Update doc comment |
| `internal/session/agent.go` | 63 | `"curl -s -m 3 -H 'X-Kangent-Token: %s' ..."` | Rename literal → `'X-Kamacu-Token: %s'` |
| `internal/session/agent_test.go` | 94 | `if !strings.Contains(h.Command, "X-Kangent-Token: TOK")` | Rename literal |
| `internal/opencode/kamacu-status.js` | 79 | `'X-Kangent-Token': token,` (in the fetch headers literal) | Rename literal → `'X-Kamacu-Token': token,` |
| `internal/opencode/plugin_test.go` | 195 | `"X-Kangent-Token",` in `mustContain` slice | Rename literal |
| `internal/opencode/e2e_test.go` | 136 | Comment "receiver contract (X-Kangent-Token constant-time compare..." | Update text |
| `internal/opencode/e2e_test.go` | 144 | `got := r.Header.Get("X-Kangent-Token")` | Rename literal |

**Non-source occurrences** (history only — out of scope, do NOT touch in Phase 06):
- `.planning/PROJECT.md:227, 299` — milestone-history narrative; the literal is describing a PAST state ("left unchanged for Phase 21"). Do not edit; the prose is time-stamped.
- `.planning/phases/06-mcp-subcommand-foundation/06-CONTEXT.md` (multiple lines) — this research's own source artifact.
- `.planning/phases/06-mcp-subcommand-foundation/06-DISCUSSION-LOG.md:89, 96` — historical discussion log.
- `.gsd/phases/02-opencode-built-in-agent-engine/*` and `.gsd/DECISIONS.md:22` — the v1.10 historical record (D014 decision). Do NOT touch; the historical record references the name AS IT WAS AT THE TIME.
- `.gsd/uat/M002/S01/attempt-1.json:74, 89` — historical UAT artifact. Out of scope.

**opencode.InstallPlugin regeneration path:** the embedded plugin source lives in `internal/opencode/kamacu-status.js` and is read at compile time via `//go:embed`. Once Phase 06 lands the renamed literal, the next `kamacu serve` boot calls `opencode.InstallPlugin()`, which writes the new bytes to `${XDG_CONFIG_HOME:-~/.config}/opencode/plugin/kamacu-status.js` (skip-on-byte-identical). Existing installs auto-update on next boot — no separate migration task.

### `google/subcommands` minimal pattern (verified against v1.2.0 source)
```go
// Source: github.com/google/subcommands@v1.2.0/subcommands.go
package main

import (
	"context"
	"flag"
	"os"

	"github.com/google/subcommands"
)

type serveCmd struct {
	addr                string
	db                  string
	claudeBin           string
	devOrigins          []string
	insecureAllowRemote bool
}

func (serveCmd) Name() string     { return "serve" }
func (serveCmd) Synopsis() string { return "run the Kamacu HTTP server (today's main)" }
func (serveCmd) Usage() string {
	return `serve [flags]:
  Run the Kamacu HTTP server. Today's default behavior.
`
}
func (c *serveCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&c.addr, "addr", "127.0.0.1:7333", "listen address (localhost-only by design)")
	f.StringVar(&c.db, "db", "~/.kamacu/kamacu.db", "path to SQLite database file")
	f.StringVar(&c.claudeBin, "claude-bin", "", "path to the claude binary")
	f.Func("dev-origin", "additional allowed Origin host:port (repeatable)", func(v string) error {
		c.devOrigins = append(c.devOrigins, v)
		return nil
	})
	f.BoolVar(&c.insecureAllowRemote, "insecure-allow-remote", false, "opt-in: allow binding --addr to non-loopback (NO AUTH)")
}
func (c *serveCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	// === TODAY'S main.go BODY GOES HERE VERBATIM ===
	// (ensureLoopback, migrate.Prepare, store.Open, backfills, reaper, http.ListenAndServe)
	return subcommands.ExitSuccess
}

func main() {
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(serveCmd{}, "")
	// (mcpCmd registered too — see Pattern 2)
	flag.Parse()
	os.Exit(int(subcommands.Execute(context.Background())))
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `flag.Parse()` + flat main() in `cmd/kamacu/main.go` | `subcommands.Execute(ctx)` with per-Command `SetFlags` | Phase 06 (this phase) | Bare `kamacu` no longer starts the server — D-04 break clean. README / Makefile / scripts / smoke.sh updated. |
| `X-Kangent-Token` HTTP header | `X-Kamacu-Token` HTTP header | Phase 06 (this phase) | Coordinated Go + JS + test rename; opencode.InstallPlugin regenerates on next boot. |
| Kamacu tasks have no MCP tool surface | Kamacu tasks can drive Kamacu via MCP (Phase 06 ships ONE tool; Phase 07–09 add 16 more) | v1.11 milestone | The milestone's headline architecture. |

**Deprecated/outdated:**
- `github.com/mark3labs/mcp-go` — community SDK acknowledged but superseded by the official `modelcontextprotocol/go-sdk`. Not used in Kamacu.
- `xterm` (unscoped npm package) — already absent from Kamacu (uses `@xterm/xterm` 6.0.0). Not relevant to Phase 06.

## Assumptions Log

> All claims tagged `[ASSUMED]` in this research. The planner and discuss-phase use this section to identify decisions that need user confirmation before execution.

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The MCP Go SDK's binary name on disk is whatever the user names it (no SDK constraint); `mcp.NewServer(&mcp.Implementation{Name: "kamacu", Version: "dev"}, nil)` is the announced identification. | Pattern 1, list_projects tool registration | Low — the SDK accepts any name/version; if the agent-CLI protocol evolves to constrain them, Phase 06 can update trivially. |
| A2 | Compile-time version string injection (e.g. via `-ldflags "-X main.version=…"`) is deferred to a later phase; `"dev"` is acceptable for Phase 06. | Pattern 1 | Low — only affects the `serverInfo` field in the MCP handshake; agents don't gate on it. |
| A3 | The Kamacu `GET /api/projects` response is small enough (single user, dozens of projects) that the 1 MiB `io.LimitReader` cap never trips in practice. | list_projects tool registration | Low — if a user has hundreds of projects with long descriptions, the cap could truncate. Bumping to 4 MiB is a one-line change. |
| A4 | No init-time `log.SetOutput(os.Stdout)` exists anywhere in Kamacu's import graph (the subcommand's `mcp serve` path). Verified by grep on `internal/`. | Pitfall 4 | Low — if a future dependency redirects log to stdout, MCP clients would see stream corruption. MCPHARD-02 (deferred) would catch this with a CI grep check. |

**If this table is otherwise empty:** all other claims were verified or cited directly.

## Open Questions

1. **Should the SC2 unit test (D-11) also include a panic-recovery test?**
   - What we know: The SDK does NOT recover panics. A panicking handler crashes the process.
   - What's unclear: Should Phase 06 wrap each tool handler in a `defer recover()` to convert panics to errors, OR accept that panics crash the process (consistent with the existing main process posture)?
   - Recommendation: SKIP panic recovery in Phase 06 (the load-bearing assertion of SC2 is "stream stays clean on handler error", which the SDK already handles). Defer panic-recovery wrapping to Phase 08 when `subscribe_session_output` introduces more panic surface area. Document this as an explicit choice in the plan.

2. **What version string should the MCP server announce?**
   - What we know: The Kamacu binary has no compile-time version injection today (no `-ldflags` in Makefile).
   - What's unclear: Whether Phase 06 should add ldflags-based versioning as part of the SDK `Implementation.Version`.
   - Recommendation: Use the literal `"dev"` for Phase 06. Adding ldflags is orthogonal scope; the milestone is about MCP plumbing, not release engineering. Open a separate quick-task if desired.

## Environment Availability

> Phase 06's external dependencies are the Kamacu binary (already required to exist for testing) and Go 1.26 (already required to build). No new external tools.

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | Build + test | ✓ (Kamacu requires Go 1.26) | 1.26.x | — |
| Network (loopback HTTP) | Bridge integration test | ✓ (stdlib `httptest` only; no real network) | — | — |
| `claude` binary | NOT required (Phase 06 doesn't touch the agent CLI) | n/a | — | — |
| `opencode` binary | NOT required (Phase 06 doesn't touch the agent CLI) | n/a | — | — |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** none.

Step 2.6: SKIPPED (no NEW external dependencies — Phase 06 is Go source + tests only).

## Security Domain

> `security_enforcement` is not present in `.planning/config.json`, so it defaults to enabled. The phase does not introduce new auth or crypto; this section is brief because Phase 06's security posture is "preserve the existing one byte-for-byte".

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | n/a — Kamacu remains single-user, loopback-bound; no new auth surface in Phase 06 |
| V3 Session Management | no | n/a — MCP subcommand sessions are stateless HTTP clients of the Kamacu server; no session state in `internal/mcp/` |
| V4 Access Control | yes (preserved) | Existing `hostCheck` middleware + WS Origin allowlist + `ensureLoopback` — UNCHANGED in Phase 06. The MCP subcommand is just another loopback HTTP client. |
| V5 Input Validation | yes | MCP SDK validates the JSON-RPC envelope and tool input schema (type:"object" required; the SDK panics otherwise). Tool args at the handler level are accepted as `json.RawMessage` and explicitly ignored (`list_projects`'s `project_id?` is a no-op per D-08). No injection surface — the handler does not interpolate args into SQL, shell, or URL. |
| V6 Cryptography | yes (preserved) | `crypto/subtle.ConstantTimeCompare` for hook token validation in `internal/api/hooks.go:40` — UNCHANGED in Phase 06. The rename preserves the constant-time compare byte-for-byte. |

### Known Threat Patterns for the Phase 06 stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Webpage fires no-CORS fetch POST at `localhost:7333` with a guessed hook token | Spoofing / Elevation of privilege | 32-byte `crypto/rand` token, constant-time compare, fresh-per-start, regenerated into spawned agents every boot. Renaming the header does not weaken this. |
| MCP subcommand prints stray bytes to stdout, desyncing the JSON-RPC stream | Tampering / Denial of service | D-12: stdlib defaults (SDK owns stdout; default logger discards; `log/slog` defaults to stderr) + code-review convention (no `fmt.Print*` in `internal/mcp/`). Phase 06 ships the SC2 unit test (D-11) covering the SDK's clean-error-response invariant. Full stdout guards (MCPHARD-02) are deferred to v1.12. |
| Bridge HTTP request to non-loopback address (KAMACU_HOOK_BASE points elsewhere) | Information disclosure | Loopback binding is enforced by the SERVER (Kamacu's `ensureLoopback` + `hostCheck`), not the client. Even if the subcommand points at a non-loopback URL, the targeted server (if it's a real Kamacu) refuses non-loopback Host headers. |

## Sources

### Primary (HIGH confidence)
- `proxy.golang.org/github.com/modelcontextprotocol/go-sdk/@v/v1.6.1.mod` — confirmed exists, requires Go 1.25, direct deps enumerated
- `proxy.golang.org/github.com/modelcontextprotocol/go-sdk/@v/list` — confirmed v1.6.1 is the latest stable tag (v1.7.0-pre.3 is pre-release)
- `proxy.golang.org/github.com/google/subcommands/@latest` — confirmed latest stable v1.2.0, Sep 2019
- `pkg.go.dev/github.com/modelcontextprotocol/go-sdk@v1.6.1` — published May 22 2026, stable major, Apache-2.0/MIT/CC-BY-4.0 license, MCP spec 2025-11-25 / 2025-06-18 / 2025-03-26 / 2024-11-05
- **Raw source read at v1.6.1 tag** (HIGH confidence — direct from the authoritative repo):
  - `mcp/server.go` — `NewServer`, `Server.Run`, `Server.Connect`, `ServerOptions` (Logger defaults to discard handler)
  - `mcp/tool.go` — `ToolHandler`, `ToolHandlerFor[In,Out]`, `serverTool`, `validateToolName` (alphanumeric + `_-.`, ≤128 chars)
  - `mcp/server.go::Server.AddTool` — requires non-nil InputSchema with type:"object"; panics otherwise
  - `mcp/server.go::Server.AddTool` (top-level generic) — typed helper, auto-generates schema
  - `mcp/transport.go` — `StdioTransport.Connect` returns `newIOConn(rwc{os.Stdin, nopCloserWriter{os.Stdout}})`; `ioConn.Read` returns `io.EOF` on stdin close; `ioConn.Write` is newline-delimited, mutex-protected
  - `mcp/transport.go::connect` — `OnInternalError: func(err error) { log.Printf("jsonrpc2 error: %v", err) }` (logs to stderr by stdlib default)
  - `mcp/protocol.go` — `CallToolParams`, `CallToolParamsRaw` (Arguments is `json.RawMessage`), `CallToolResult` (Content, StructuredContent, IsError), `Implementation` (Name + Version required), `Tool` (InputSchema required), `ListToolsParams/Result`
  - `mcp/content.go` — `TextContent{Text string, Meta, Annotations}`, `Content` interface
  - `mcp/logging.go::ensureLogger` — `slog.New(slog.DiscardHandler)` when Logger is nil (silent by default)
  - `internal/jsonrpc2/jsonrpc2.go` — `Handler` interface, `ErrNotHandled`, `ErrMethodNotFound`
  - `internal/jsonrpc2/conn.go::handleAsync` — handler goroutine spawns WITHOUT `recover()`; panics escape
  - `internal/jsonrpc2/conn.go::processResult` — non-nil error → JSON-RPC error response with `err.code = -32603` (Internal); nil result + nil error for a Call → internalErrorf → logged to stderr
- `proxy.golang.org/github.com/google/subcommands/@v/list` + raw source `subcommands.go` at master — `Command` interface (5 methods), `Execute` looks at FIRST positional arg only, "groups" are help-organization only, `subcommands.NewCommander(f, name)` for nested dispatch

### Secondary (MEDIUM confidence)
- pkg.go.dev README example — minimal `mcp.NewServer` + `mcp.AddTool` + `s.Run(ctx, &mcp.StdioTransport{})` shape
- (No WebSearch results needed — every claim is traceable to one of the above primary sources)

### Tertiary (LOW confidence)
- None. Every load-bearing claim was verified at the source level.

## Metadata

**Confidence breakdown:**
- Standard stack: **HIGH** — both packages verified against proxy.golang.org + raw source at the pinned tag.
- Architecture: **HIGH** — `google/subcommands` flat-dispatch surprise was caught by reading source; nested-dispatch pattern (Pattern 2) is verified.
- Pitfalls: **HIGH** — panic-handling and EOF behavior verified by reading `internal/jsonrpc2/conn.go::handleAsync` and `mcp/transport.go::ioConn.Read` directly.
- Coordinated rename surface: **HIGH** — exhaustive grep over the entire repo; every source occurrence enumerated.

**Research date:** 2026-07-21
**Valid until:** 2026-08-21 (30 days; both pinned versions are stable lines with no breaking-change risk on the patch level. If v1.7.0 of the MCP SDK lands in that window and Phase 06 hasn't started, re-verify the `Server.Run` / `Server.AddTool` / `StdioTransport` API surface — those are the load-bearing types.)
