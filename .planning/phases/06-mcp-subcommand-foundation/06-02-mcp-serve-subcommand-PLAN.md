---
phase: 06-mcp-subcommand-foundation
plan: 02
slug: mcp-serve-subcommand
type: execute
wave: 2
depends_on:
  - 06-01-cli-dispatcher-refactor
files_modified:
  - cmd/kamacu/main.go
  - cmd/kamacu/mcp.go
  - internal/mcp/server.go
  - internal/mcp/bridge.go
  - internal/mcp/server_test.go
  - internal/mcp/bridge_test.go
  - go.mod
  - go.sum
autonomous: true
requirements: [MCPPROC-01, MCPPROC-02, MCPPROC-03]
must_haves:
  truths:
    # SC1 — long-lived MCP server over stdio using the official SDK
    - "Running `kamacu mcp serve` from a shell starts a long-lived MCP server constructed via `mcp.NewServer(&mcp.Implementation{Name: \"kamacu\", Version: \"dev\"}, nil)` (from `github.com/modelcontextprotocol/go-sdk` v1.6.1) that speaks JSON-RPC over stdio and bridges tool calls to the running Kamacu HTTP API"
    - "`go.mod` contains `github.com/modelcontextprotocol/go-sdk v1.6.1` as a direct requirement"
    - "The subcommand's process lifecycle is correct: on stdin EOF the SDK's `s.Run(ctx, &mcp.StdioTransport{})` returns nil and `mcpCmd.Execute` maps that to `subcommands.ExitSuccess` (exit code 0) — `echo | kamacu mcp serve; echo $?` prints `0` (06-RESEARCH.md Pitfall 5)"
    # SC2 — tools/list over stdout, ONLY valid MCP messages
    - "An MCP client connecting to the subcommand's stdio and calling `tools/list` receives a valid JSON-RPC response over stdout listing EXACTLY ONE tool named `list_projects` (D-10) — no stubs, no future-tool placeholders"
    - "The SC2 malformed-tool regression test (D-11) — a unit test localized to `internal/mcp/server_test.go` — registers two deliberately-erroring handlers: (a) one returning `(nil, errors.New(\"...\"))` exercising the JSON-RPC error-response path (error.code = -32603), and (b) one returning `(&mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: \"boom\"}}}, nil)` exercising the MCP-level IsError path. The test asserts each produces a clean JSON-RPC response on stdout (never a process crash, never a non-JSON line, never a stream desync). The test contains NO panic-handler case (06-RESEARCH.md Pitfall 1: the SDK does not recover panics; a panic-handler test would fail against the as-shipped SDK)."
    # SC3 — bridge returns real Kamacu data using KAMACU_HOOK_TOKEN
    - "A `tools/call` request for `list_projects` routed through the bridge returns real Kamacu data — the JSON body of `GET /api/projects` — as a single `mcp.TextContent` block inside a `*mcp.CallToolResult` (raw passthrough; per `the agent's Discretion` in CONTEXT.md)"
    - "Every bridge HTTP request sets the header `X-Kamacu-Token` to the value of `KAMACU_HOOK_TOKEN` env (D-06/D-07). On `/api/projects` Kamacu ignores the header (loopback binding is the v1.11 auth boundary), but the wire contract is correct for any future server-side check"
    - "The bridge's httptest-based integration test (`internal/mcp/bridge_test.go`) follows the project's established pattern (`internal/api/agent_integration_test.go`): stand up a `httptest.Server` with Kamacu's project list handler, point the bridge at it via `KAMACU_HOOK_BASE`, invoke `list_projects`'s handler, and assert the response carries the test fixture projects"
    # SC4 — KAMACU_HOOK_BASE override + default
    - "When `KAMACU_HOOK_BASE` env is set, the bridge parses that URL and uses it as the HTTP base. When unset, the bridge defaults to `http://127.0.0.1:7333` (MCPPROC-03). Invalid `KAMACU_HOOK_BASE` values surface as a startup error from `newBridgeFromEnv`."
    # D-12 — stdout cleanliness (stdlib defaults + code review)
    - "`internal/mcp/` package code is `fmt.Print*`-free, `log.Print*`-free, `os.Stdout.Write`-free by convention (D-12). `slog.Default()` writes to stderr (Go stdlib default). `mcp.NewServer` is constructed with `nil` options so the SDK's own default logger discards log output entirely (`slog.DiscardHandler` per 06-RESEARCH.md). The SDK's `OnInternalError` hook uses stdlib `log.Printf` which defaults to stderr — verified that no init in `internal/mcp/` or its import graph calls `log.SetOutput(os.Stdout)`."
    # D-09 — final production code
    - "The `list_projects` tool registration and its handler are FINAL production code (D-09) — Phase 07 inherits this tool unchanged and ADDS the remaining 12 tools on the same pattern. Plan 02 does NOT flag the tool as demo-grade or throwaway."
  artifacts:
    - cmd/kamacu/main.go (adds mcpCmd registration)
    - cmd/kamacu/mcp.go (NEW — mcpCmd with nested subcommands.Commander)
    - internal/mcp/server.go (NEW — SDK init, list_projects tool registration, Serve loop)
    - internal/mcp/bridge.go (NEW — HTTP client, env parsing, list_projects handler)
    - internal/mcp/server_test.go (NEW — SC2 malformed-tool regression test, D-11)
    - internal/mcp/bridge_test.go (NEW — httptest-based bridge integration test for list_projects)
    - go.mod (adds github.com/modelcontextprotocol/go-sdk v1.6.1)
    - go.sum
  key_links:
    - "subcommands.Register(mcpCmd{}, \"\") added to cmd/kamacu/main.go → mcpCmd.Execute in cmd/kamacu/mcp.go"
    - "mcpCmd.Execute constructs an inner subcommands.Commander and registers kamacumcp.ServeCommand{} → internal/mcp.ServeCommand.Execute calls internal/mcp.Serve(ctx)"
    - "internal/mcp.Serve → mcp.NewServer + registerTools(s) + s.Run(ctx, &mcp.StdioTransport{})"
    - "registerTools → s.AddTool(&mcp.Tool{Name: \"list_projects\", ...}, listProjectsHandler) — exactly one tool"
    - "listProjectsHandler(ctx, *mcp.CallToolRequest) → bridgeFromContext(ctx).do(ctx, GET, /api/projects, nil) → Kamacu HTTP API → TextContent passthrough"
    - "newBridgeFromEnv() reads os.Getenv(\"KAMACU_HOOK_BASE\") (default http://127.0.0.1:7333) and os.Getenv(\"KAMACU_HOOK_TOKEN\") and constructs *bridge with 10s http.Client timeout"
  prohibitions:
    - statement: "NO future-subcommand stubs — the `mcp` group registers ONLY `serve` plus the inner Commander's HelpCommand (D-05). No `mcp doctor`, no `mcp tokens`, no top-level non-`mcp` Commands besides `serve` and the top-level `help`."
      status: resolved
      verification: "`grep -rn 'Register(' cmd/kamacu/ internal/mcp/` shows registrations for serveCmd, mcpCmd, the inner Commander's HelpCommand, and ServeCommand — and nothing else"
    - statement: "NO `tools/list` entry besides `list_projects` (D-10). No stub tools, no placeholders for Phase 07+ tools. The registerTools function calls `s.AddTool` exactly ONCE."
      status: resolved
      verification: "`grep -c 's.AddTool\\|mcp.AddTool' internal/mcp/server.go` returns exactly 1; a live `tools/list` JSON-RPC roundtrip in the bridge_test returns exactly one tool name"
    - statement: "NO active stdout guard (os.Stdout redirect, buffering wrapper, or CI grep check) — D-12 ships only stdlib defaults + code-review convention. Full MCPHARD-02 is v1.12."
      status: resolved
      verification: "internal/mcp/*.go contains no `os.Stdout.Write` call, no init-time `log.SetOutput(os.Stdout)`; the Makefile gains no stdout-pollution grep check in this plan"
    - statement: "NO panic-recovery wrapping in tool handlers — `defer recover()` is explicitly Phase 08 work (06-RESEARCH.md Open Question 1). The SC2 unit test MUST NOT include a panic-handler case (Pitfall 1: the SDK does not recover panics, so such a test would fail against the as-shipped SDK)."
      status: resolved
      verification: "internal/mcp/server_test.go contains zero test cases that invoke a panicking handler; the list_projects handler has no `defer recover()`"
    - statement: "NO server-side middleware validating `X-Kamacu-Token` on `/api/*` routes (D-06) — loopback binding (`hostCheck` + `ensureLoopback` + WS Origin allowlist) remains the auth boundary. The bridge sends the header; Kamacu ignores it on `/api/projects`."
      status: resolved
      verification: "internal/api/routes.go and internal/api/projects.go are NOT modified by this plan; `git diff --name-only` for this plan shows no files under `internal/api/`"
    - statement: "NO e2e harness with build tag `//go:build mcp_e2e` — MCPHARD-03 is v1.12 (D-11). The SC2 test is a unit test localized to `internal/mcp/server_test.go`."
      status: resolved
      verification: "`grep -rn 'go:build mcp_e2e' .` returns zero results after this plan"
    - statement: "NO typed JSON-RPC error taxonomy — MCPHARD-01 is v1.12. Bridge errors surface as generic MCP errors via the handler returning `(nil, error)` (the SDK wraps it as a JSON-RPC error.code = -32603 response)."
      status: resolved
      verification: "internal/mcp/bridge.go returns plain `errors.New` / `fmt.Errorf` errors with no typed error-code struct"
    - statement: "DO NOT use the typed `mcp.AddTool[In, Out any]` helper — use the low-level `s.AddTool(*mcp.Tool, handler)` with an explicit `json.RawMessage` InputSchema (06-RESEARCH.md Anti-Patterns: the typed helper would force every Phase 07 tool to declare a Go In struct mirroring its wire schema)."
      status: resolved
      verification: "internal/mcp/server.go uses `s.AddTool(&mcp.Tool{...}, listProjectsHandler)` (method on Server) — NOT `mcp.AddTool(s, &mcp.Tool{...}, typedHandlerFn)` (top-level generic function)"
    - statement: "DO NOT call `/api/projects?project_id=…` — the endpoint ignores query params today (D-08). The optional `project_id` arg is a documented no-op until Phase 07."
      status: resolved
      verification: "internal/mcp/bridge.go's list_projects handler does not append `?project_id=` (or any query string) to the request URL; the handler constructs `bridge.do(ctx, http.MethodGet, \"/api/projects\", nil)`"
    - statement: "DO NOT use the typed `ToolHandlerFor[In,Out]` API or `google/jsonschema-go`'s typed schema inference. The `InputSchema` field is `json.RawMessage`, not a Go struct annotated with `jsonschema:` tags."
      status: resolved
      verification: "internal/mcp/server.go's InputSchema literal is `json.RawMessage(\\`{...}\\`)`; the file imports `encoding/json` but does not import `github.com/google/jsonschema-go`"
    - statement: "DO NOT use WebSocket text frames for any bridge traffic — the bridge is plain HTTP (not WS) in this phase; binary/text framing is irrelevant here but called out to prevent accidental WS introduction."
      status: resolved
      verification: "internal/mcp/bridge.go imports `net/http` only; no `coder/websocket` or `gorilla/websocket` import"
---

# Plan 02: `internal/mcp/` package + `kamacu mcp serve` body + SC2 unit test

<objective>
Build the MCP stdio subcommand body in a new `internal/mcp/` package, wire it into the CLI dispatcher from Plan 01 via a `mcpCmd` that owns a nested `subcommands.Commander` (Pattern 2 — `google/subcommands` is FLAT, not nested), register exactly ONE production tool (`list_projects` per D-08/D-09), and ship the SC2 malformed-tool regression test as a unit test localized to `internal/mcp/` (D-11). This plan delivers SC1, SC2, SC3, and SC4 verbatim and satisfies MCPPROC-01, MCPPROC-02, MCPPROC-03.

Purpose: This is the architecture-proof phase for v1.11. The bridge pattern proven here — env parse → HTTP client with X-Kamacu-Token header → call existing Kamacu endpoint → translate response to MCP tool output — is mechanically repeated 12+ times in Phase 07 and 16+ more times across Phases 08–09. The proof tool `list_projects` is FINAL production code (D-09); Phase 07 inherits it unchanged.

Output: `internal/mcp/{server.go, bridge.go, server_test.go, bridge_test.go}` — the package both Plan 02 of this phase and ALL of Phases 07–09 build on. `cmd/kamacu/mcp.go` — the dispatcher entry that future `mcp <other>` subcommands slot into. `cmd/kamacu/main.go` — adds the one-line `subcommands.Register(mcpCmd{}, "")` call.
</objective>

<execution_context>
@/home/jordi/.config/opencode/gsd-core/workflows/execute-plan.md
@/home/jordi/.config/opencode/gsd-core/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/phases/06-mcp-subcommand-foundation/06-CONTEXT.md
@.planning/phases/06-mcp-subcommand-foundation/06-RESEARCH.md
@.planning/phases/06-mcp-subcommand-foundation/06-01-SUMMARY.md
@cmd/kamacu/main.go
@cmd/kamacu/serve.go
@internal/api/routes.go
@internal/api/projects.go
@internal/session/manager.go
@internal/api/agent_integration_test.go
@go.mod
</context>

<tasks>

<task type="auto">
  <name>Task 1: Create internal/mcp/ package (server.go + bridge.go) with list_projects tool + SC2 malformed-tool unit test</name>
  <files>internal/mcp/server.go, internal/mcp/bridge.go, internal/mcp/server_test.go, go.mod, go.sum</files>
  <read_first>
    - `.planning/phases/06-mcp-subcommand-foundation/06-RESEARCH.md` § "Standard Stack", "Architecture Patterns" (System Architecture Diagram + Pattern 1 + Pattern 3 + list_projects tool registration), "Pitfall 1" (SDK does NOT recover tool-handler panics), "Pitfall 2" (`AddTool` panics on missing/invalid schema — InputSchema MUST be `json.RawMessage(`{"type":"object"}`)` minimum), "Pitfall 5" (stdin EOF returns nil from Run; map to ExitSuccess), "Code Examples" (canonical SDK README example + low-level tool registration with explicit schema + error-returning handler), and "Anti-Patterns to Avoid" (do NOT use typed AddTool[In,Out]; do NOT inline the HTTP call; do NOT write to stdout). The handler signature `func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)` is verified at the source level.
    - `.planning/phases/06-mcp-subcommand-foundation/06-CONTEXT.md` decisions D-06 (loopback-only — bridge sends X-Kamacu-Token but Kamacu ignores on /api/*), D-08 (proof tool = list_projects, calls GET /api/projects UNCHANGED, project_id arg is a no-op today), D-09 (FINAL production code — Phase 07 inherits unchanged), D-10 (tools/list returns exactly ONE tool), D-11 (SC2 test is a UNIT test localized to internal/mcp/), D-12 (stdout cleanliness via stdlib defaults + code review).
    - `internal/api/routes.go` line 32 — confirms `mux.HandleFunc("GET /api/projects", p.list)` is the bridge target.
    - `internal/api/projects.go` around lines 126-148 — confirms `projectHandlers.list` writes `[]Project` JSON via `writeJSON(w, http.StatusOK, projects)`; the response shape is the array the bridge passes through.
    - `internal/session/manager.go` lines 185-187 — confirms `KAMACU_SESSION_ID`, `KAMACU_HOOK_TOKEN`, `KAMACU_HOOK_BASE` are the env var names (the bridge reads the latter two). NO changes to manager.go in this plan.
    - `go.mod` — currently lacks the SDK; this task adds it.
  </read_first>
  <action>
    1. Add the SDK: `go get github.com/modelcontextprotocol/go-sdk@v1.6.1 && go mod tidy`. Verify with `grep 'github.com/modelcontextprotocol/go-sdk v1.6.1' go.mod`. The transitive deps (`google/jsonschema-go`, `segmentio/encoding`, `yosida95/uritemplate/v3`, `golang-jwt/jwt/v5`, `golang.org/x/oauth2`, `golang.org/x/tools`) are expected and pre-verified by 06-RESEARCH.md § "Transitive deps the SDK will add to go.mod".

    2. Create `internal/mcp/bridge.go` (package `mcp`) defining the per-process HTTP bridge:
       - `type bridge struct { base string; token string; client *http.Client }` — `base` is the Kamacu base URL with no trailing slash (default `http://127.0.0.1:7333`); `token` is the `KAMACU_HOOK_TOKEN` value (may be `""` — Kamacu ignores on /api/* per D-06); `client` is `&http.Client{Timeout: 10 * time.Second}` (fail fast; no retry per 06-RESEARCH.md the agent's Discretion).
       - `func newBridgeFromEnv() (*bridge, error)` — reads `os.Getenv("KAMACU_HOOK_BASE")` (falls back to `http://127.0.0.1:7333` when empty), `url.Parse`s it to validate (returns `fmt.Errorf("invalid KAMACU_HOOK_BASE %q: %w", base, err)` on parse failure), trims trailing slashes via `strings.TrimRight(base, "/")`, reads `os.Getenv("KAMACU_HOOK_TOKEN")` verbatim, and returns the constructed `*bridge`.
       - `func (b *bridge) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error)` — constructs `http.NewRequestWithContext(ctx, method, b.base+path, body)`, sets `req.Header.Set("X-Kamacu-Token", b.token)` on EVERY call (D-06/D-07 wire contract — sent even though Kamacu ignores on /api/*), and returns `b.client.Do(req)`.
       - `func (b *bridge) listProjects(ctx context.Context) (*mcp.CallToolResult, error)` — the tool handler body. Calls `b.do(ctx, http.MethodGet, "/api/projects", nil)`, defers `resp.Body.Close()`, reads at most 1 MiB via `io.ReadAll(io.LimitReader(resp.Body, 1<<20))`. On transport error returns `fmt.Errorf("kamacu bridge: %w", err)`. On non-200 status returns `fmt.Errorf("kamacu GET /api/projects: HTTP %d: %s", resp.StatusCode, string(body))`. On 200 returns `(&mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}}, nil)` — raw JSON passthrough per `the agent's Discretion` in CONTEXT.md. Does NOT append `?project_id=…` to the URL (D-08 — the arg is a no-op today).

    3. Create `internal/mcp/server.go` (package `mcp`):
       - `type ServeCommand struct{}` — implements `subcommands.Command`:
         - `Name() string` returns `"serve"`.
         - `Synopsis() string` returns `"run the Kamacu MCP stdio server"`.
         - `Usage() string` returns a short multi-line block describing the env-driven contract (`KAMACU_HOOK_BASE`, `KAMACU_HOOK_TOKEN`) and that it's spawned by an agent CLI inside a Kamacu task PTY.
         - `SetFlags(*flag.FlagSet)` is empty — Phase 06 is env-driven, no flags (D-05 / 06-RESEARCH.md Anti-Patterns).
         - `Execute(ctx context.Context, _ *flag.FlagSet, _ ...any) subcommands.ExitStatus`:
           1. `slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))` — pins the Go stdlib default (stderr) explicitly so future import-graph changes can't redirect slog to stdout (D-12(a)).
           2. `bridge, err := newBridgeFromEnv()` — on error: `slog.Error("mcp bridge env", "error", err); return subcommands.ExitFailure`.
           3. `srv := mcp.NewServer(&mcp.Implementation{Name: "kamacu", Version: "dev"}, nil)` — `nil` options means the SDK's default logger discards log output (`slog.DiscardHandler` per 06-RESEARCH.md).
           4. `registerTools(srv, bridge)` — registers exactly `list_projects` (see below).
           5. `err = srv.Run(ctx, &mcp.StdioTransport{})` — blocks until ctx cancel or stdin EOF.
           6. Map: `if err == nil || errors.Is(err, context.Canceled) { return subcommands.ExitSuccess }; slog.Error("mcp server ended", "error", err); return subcommands.ExitFailure`. The `nil → ExitSuccess` mapping is 06-RESEARCH.md Pitfall 5 (load-bearing — `echo | kamacu mcp serve; echo $?` must print `0`).
       - `func registerTools(s *mcp.Server, b *bridge)` — calls `s.AddTool` EXACTLY ONCE:
         - `s.AddTool(&mcp.Tool{Name: "list_projects", Description: "List all Kamacu projects. Returns the raw JSON array from GET /api/projects. The optional project_id is currently a no-op (filtering arrives in Phase 07).", InputSchema: json.RawMessage(`{"type":"object","properties":{"project_id":{"type":"integer","description":"Optional project id. Currently ignored — list_projects always returns all projects. Will be honored in Phase 07."}}}`)}, b.listProjects)` — uses the low-level `Server.AddTool` method with an explicit `json.RawMessage` schema (NOT the typed `mcp.AddTool[In,Out]` helper — see 06-RESEARCH.md Anti-Patterns). `InputSchema` MUST be `json.RawMessage` with `type:"object"` (Pitfall 2: `AddTool` panics otherwise).
       - The `listProjects` field on `*bridge` is the `mcp.ToolHandler`-shaped method (signature `func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)`). It accepts the `*mcp.CallToolRequest`, does NOT unmarshal `req.Params.Arguments` (the optional `project_id` is a documented no-op), and delegates to `b.listProjects(ctx)`.

    4. Create `internal/mcp/server_test.go` (package `mcp`) implementing the SC2 regression test (D-11):
       - `TestSC2_HandlerReturningError_ProducesCleanJSONRPCErrResponse` — constructs an `mcp.NewServer(...)` , registers a tool via `s.AddTool(&mcp.Tool{Name: "fail_err", InputSchema: json.RawMessage(`{"type":"object"}`)}, func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) { return nil, errors.New("boom") })`, drives the server over an in-memory stdio transport (stdin/stdout pipes via `io.Pipe` or by directly invoking the SDK's handler with a synthesized `*mcp.CallToolRequest`), and asserts the response is a valid JSON-RPC error with `error.code == -32603` and `error.message` containing `"boom"`. The exact mechanism (in-memory stdio vs. direct handler invocation) follows whichever the SDK exposes cleanly at v1.6.1 — prefer direct handler invocation if the SDK exposes a `Server.Handle` or similar testing affordance; fall back to stdio pipes otherwise. Either way the assertion is: the response is valid JSON, no non-JSON bytes are emitted, and no panic escapes.
       - `TestSC2_HandlerReturningIsErrorResult_ProducesCleanJSONRPCSuccessWithIsErrorTrue` — same setup but the handler returns `(&mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "boom"}}}, nil)`. Assert the response is a valid JSON-RPC SUCCESS (no `error` field) with `result.isError == true` and `result.content[0].text == "boom"`.
       - DO NOT write a `TestSC2_HandlerThatPanics_...` case — Pitfall 1 explicitly: the SDK does NOT recover panics, so such a test would crash the process and fail. Panic-recovery wrapping is Phase 08 work (06-RESEARCH.md Open Question 1).
       - DO NOT use a `//go:build mcp_e2e` build tag — D-11 locks this as a unit test. It must run under the default `go test ./...` invocation.

    5. Verify the package compiles and the SC2 test runs RED-then-GREEN cleanly: `go test ./internal/mcp/... -run TestSC2 -v`. Both sub-tests must PASS.
  </action>
  <verify>
    <automated>
      set -e
      grep -q 'github.com/modelcontextprotocol/go-sdk v1.6.1' go.mod
      test -f internal/mcp/bridge.go
      test -f internal/mcp/server.go
      test -f internal/mcp/server_test.go
      # registerTools registers exactly ONE tool
      test "$(grep -c 's.AddTool\|mcp.AddTool' internal/mcp/server.go)" -eq 1
      # No typed AddTool[In,Out] generic
      ! grep -q 'mcp.AddTool\[' internal/mcp/server.go
      # No defer recover() in production handler
      ! grep -A3 'func (b \*bridge) listProjects' internal/mcp/bridge.go | grep -q 'recover()'
      # No ?project_id= query appended
      ! grep -q 'project_id=' internal/mcp/bridge.go
      # No os.Stdout.Write / fmt.Print* / log.Print* in the package
      ! grep -rn 'os.Stdout.Write\|fmt.Print\|log.Print' internal/mcp/server.go internal/mcp/bridge.go
      # SC2 test exists and has exactly the two required sub-tests, no panic test
      grep -q 'TestSC2_HandlerReturningError_ProducesCleanJSONRPCErrResponse' internal/mcp/server_test.go
      grep -q 'TestSC2_HandlerReturningIsErrorResult_ProducesCleanJSONRPCSuccessWithIsErrorTrue' internal/mcp/server_test.go
      ! grep -q 'Panic' internal/mcp/server_test.go
      # No mcp_e2e build tag
      ! grep -rn 'go:build mcp_e2e' internal/mcp/
      # go vet and go test pass for the new package
      go vet ./internal/mcp/...
      go test ./internal/mcp/... -run TestSC2 -v
    </automated>
  </verify>
  <done>
    - go.mod contains `github.com/modelcontextprotocol/go-sdk v1.6.1`
    - internal/mcp/bridge.go defines `bridge`, `newBridgeFromEnv`, `bridge.do`, `bridge.listProjects`; sets `X-Kamacu-Token` header on every request; defaults to `http://127.0.0.1:7333`
    - internal/mcp/server.go defines `ServeCommand` (5-method subcommands.Command impl) and `registerTools` (calls `s.AddTool` exactly once for `list_projects`)
    - internal/mcp/server.go's `ServeCommand.Execute` maps `srv.Run` nil return to `subcommands.ExitSuccess` (Pitfall 5)
    - internal/mcp/server_test.go has the two SC2 sub-tests (error-returning handler + IsError-returning handler), no panic test
    - `go vet ./internal/mcp/...` passes
    - `go test ./internal/mcp/... -run TestSC2 -v` passes (both sub-tests GREEN)
    - internal/mcp/ contains no `os.Stdout.Write`, `fmt.Print*`, `log.Print*`, or `//go:build mcp_e2e`
  </done>
</task>

<task type="auto">
  <name>Task 2: Wire mcpCmd into main.go (Pattern 2 nested Commander), register in dispatcher, add bridge integration test</name>
  <files>cmd/kamacu/main.go, cmd/kamacu/mcp.go, internal/mcp/bridge_test.go</files>
  <read_first>
    - `cmd/kamacu/main.go` (post-Plan-01) — the thin dispatcher to be extended with `subcommands.Register(mcpCmd{}, "")`. The existing two Register calls (serveCmd + HelpCommand) stay; this plan adds exactly one new Register line.
    - `.planning/phases/06-mcp-subcommand-foundation/06-RESEARCH.md` § "Pattern 2: Nested `kamacu mcp serve` dispatch" — the load-bearing library surprise: `google/subcommands`'s `Execute` looks ONLY at the first positional arg. "Groups" are help-organization only. To get `kamacu mcp serve`, the `mcp` Command must own its own inner `subcommands.Commander`. The exact idiom (from RESEARCH lines ~317-358): `func (mcpCmd) Execute(ctx, f, ...) { cdr := subcommands.NewCommander(f, "kamacu mcp"); cdr.Register(subcommands.HelpCommand(cdr), ""); cdr.Register(kamacumcp.ServeCommand{}, ""); return cdr.Execute(ctx) }`.
    - `.planning/phases/06-mcp-subcommand-foundation/06-RESEARCH.md` § "Pitfall 3" — reiterates: do NOT register a Command literally named `"mcp serve"` with a space; the library matches on the first arg only.
    - `internal/api/agent_integration_test.go` lines 48-73 — the project's established httptest pattern: register handlers on a test mux, spin up `httptest.NewServer`, exercise real HTTP roundtrips. The bridge_test follows this shape.
    - `internal/api/projects.go` `projectHandlers.list` (around line 127) — the handler the bridge targets. The test fixture should mirror `[]Project` JSON.
    - `internal/mcp/server.go` and `internal/mcp/bridge.go` (post-Task-1 of this plan) — the production code under test.
  </read_first>
  <action>
    1. Create `cmd/kamacu/mcp.go` (package `main`) defining `mcpCmd` as the nested-dispatch wrapper per 06-RESEARCH.md Pattern 2:
       - `type mcpCmd struct{}` — implements `subcommands.Command`:
         - `Name() string` returns `"mcp"`.
         - `Synopsis() string` returns `"MCP-related subcommands"`.
         - `Usage() string` returns a short block: `mcp <subcommand>:\n  mcp serve   Run the Kamacu MCP stdio server.\n`.
         - `SetFlags(*flag.FlagSet)` is empty.
         - `Execute(ctx context.Context, f *flag.FlagSet, _ ...any) subcommands.ExitStatus`:
           - `cdr := subcommands.NewCommander(f, "kamacu mcp")` — names the inner commander for help output.
           - `cdr.Register(subcommands.HelpCommand(cdr), "")` — library's help command for the `mcp` group.
           - `cdr.Register(kamacumcp.ServeCommand{}, "")` — the body from `internal/mcp.ServeCommand` (Task 1 of this plan).
           - `return cdr.Execute(ctx)` — dispatches to `mcp serve` (which becomes the inner commander's first positional arg).
       - Imports: `context`, `flag`, `github.com/google/subcommands`, `kamacu/internal/mcp` (aliased or unaliased — the RESEARCH example uses `kamacumcp` to avoid collision with the SDK's `mcp` package; pick that alias).

    2. Edit `cmd/kamacu/main.go` to add the `mcpCmd` registration:
       - Add `subcommands.Register(mcpCmd{}, "")` immediately after the existing `subcommands.Register(serveCmd{}, "")` line.
       - main.go now registers exactly THREE things: `HelpCommand`, `serveCmd`, `mcpCmd`. (No `mcp doctor`, no `mcp tokens`, no other Commands — D-05.)

    3. Create `internal/mcp/bridge_test.go` (package `mcp`) — the bridge integration test using the project's httptest pattern:
       - `TestBridge_ListProjects_PassesThroughKamacuJSON` — stands up an `httptest.NewServer` with a handler that returns a canned `[]Project` JSON body (e.g. `[{"id":1,"name":"alpha",...},{"id":2,"name":"beta",...}]`) and asserts the bridge, when its `base` is pointed at the test server and its `token` is set to a known value, returns an `*mcp.CallToolResult` whose first `Content` entry is a `*mcp.TextContent` whose `Text` field equals the canned JSON.
       - The test constructs the bridge DIRECTLY (not via `newBridgeFromEnv`) — e.g. `b := &bridge{base: srv.URL, token: "test-token", client: &http.Client{Timeout: 10*time.Second}}` — so the test does not depend on env. Then calls `b.listProjects(ctx)` and inspects the result.
       - Also assert the request the test server received had `X-Kamacu-Token: test-token` header set (proves the bridge sends it on every call — D-06/D-07 wire contract).
       - Negative case `TestBridge_ListProjects_KamacuReturnsNon200_ReturnsError`: test server returns 500; assert `b.listProjects(ctx)` returns a non-nil error containing `"HTTP 500"`. Proves the bridge surfaces server errors as handler errors (which the SDK wraps as JSON-RPC error.code = -32603).
       - Negative case `TestBridge_ListProjects_KamacuUnreachable_ReturnsError`: bridge points at a closed port (`127.0.0.1:1` — refused); assert the call returns a non-nil error wrapping the transport error.
       - Optional `TestNewBridgeFromEnv_DefaultsAndOverride`: set `KAMACU_HOOK_BASE` via `t.Setenv("KAMACU_HOOK_BASE", "http://example")`, call `newBridgeFromEnv()`, assert `base == "http://example"`; unset via `t.Setenv("KAMACU_HOOK_BASE", "")`, call again, assert `base == "http://127.0.0.1:7333"` (default). Both honor `MCPPROC-03` / SC4.
       - Use `t.Setenv` (auto-cleanup) — never mutate env without it.
  </action>
  <verify>
    <automated>
      set -e
      test -f cmd/kamacu/mcp.go
      test -f internal/mcp/bridge_test.go
      # main.go registers exactly three commands
      test "$(grep -c 'subcommands.Register(' cmd/kamacu/main.go)" -eq 3
      # mcpCmd uses the nested Commander pattern (Pattern 2)
      grep -q 'subcommands.NewCommander' cmd/kamacu/mcp.go
      grep -q 'kamacumcp.ServeCommand{}' cmd/kamacu/mcp.go
      # No literal "mcp serve" command name with a space (Pitfall 3)
      ! grep -q 'Name() string.*return "mcp serve"' cmd/kamacu/mcp.go
      # No future-subcommand stubs beyond serve
      ! grep -q 'doctor\|tokens' cmd/kamacu/mcp.go
      # go vet + go build
      go vet ./...
      go build -o /tmp/kamacu-plan02 ./cmd/kamacu
      # Full test suite (Plan 02 tests)
      go test ./internal/mcp/... -v
      # Functional smoke: bare invocation lists `serve` and `mcp` in help
      /tmp/kamacu-plan02 2>&1 | grep -q 'serve'
      /tmp/kamacu-plan02 2>&1 | grep -q 'mcp'
      # `kamacu mcp` prints inner help
      /tmp/kamacu-plan02 mcp 2>&1 | grep -q 'serve'
      # `kamacu mcp serve` exits 0 on stdin EOF (Pitfall 5 — load-bearing SC1 detail)
      echo -n '' | /tmp/kamacu-plan02 mcp serve
      test $? -eq 0
    </automated>
  </verify>
  <done>
    - cmd/kamacu/mcp.go exists and defines mcpCmd using Pattern 2 (inner subcommands.Commander, registers kamacumcp.ServeCommand{})
    - cmd/kamacu/main.go registers exactly three Commands (HelpCommand, serveCmd, mcpCmd)
    - internal/mcp/bridge_test.go has positive + negative cases; uses httptest pattern; asserts X-Kamacu-Token header is sent
    - `go vet ./...` passes
    - `go build -o /tmp/kamacu-plan02 ./cmd/kamacu` succeeds
    - `go test ./internal/mcp/... -v` passes all tests (SC2 unit + bridge integration)
    - bare `/tmp/kamacu-plan02` lists both `serve` and `mcp` in help
    - `echo -n '' | /tmp/kamacu-plan02 mcp serve` exits 0 (Pitfall 5 — SC1 stdin-EOF lifecycle)
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| agent CLI → MCP subcommand stdin | The agent CLI (claude / opencode) spawns `kamacu mcp serve` and writes JSON-RPC requests to its stdin. This is a TRUSTED channel (the agent CLI is spawned by Kamacu itself inside a task PTY), but malformed input is possible if the agent CLI has a bug. |
| MCP subcommand stdout → agent CLI | JSON-RPC responses. The agent CLI parses these; any non-JSON bytes on stdout corrupt the stream (the SC2 invariant). |
| MCP subcommand → Kamacu HTTP API (loopback) | Plain HTTP requests to `127.0.0.1:7333` (or `KAMACU_HOOK_BASE`). Same trust boundary as the SPA's fetch calls — loopback binding is the actual auth boundary (D-06). |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-06-03 | Spoofing | `X-Kamacu-Token` header on bridge requests | low | accept | D-06: Kamacu ignores the header on `/api/*` routes; loopback binding (`hostCheck` + `ensureLoopback` + WS Origin allowlist) is the v1.11 auth boundary. The token is decorative on general routes. Adding server-side middleware is explicitly out of v1.11 scope (D-06). The bridge sends the header for forward compatibility. |
| T-06-04 | Tampering / DoS | stdout pollution desyncing JSON-RPC stream | high | mitigate | D-12: stdlib defaults (slog → stderr; `mcp.NewServer(...)` with nil options → SDK logger discards log output; SDK's `OnInternalError` uses stdlib `log.Printf` → stderr). Code-review convention: no `fmt.Print*` / `log.Print*` / `os.Stdout.Write` in `internal/mcp/`. The SC2 unit test (D-11) covers the SDK's clean-error-response invariant (handlers returning errors → JSON-RPC error on stdout, never crashes, never non-JSON bytes). Full MCPHARD-02 (os.Stdout redirect + CI grep check) is v1.12. |
| T-06-05 | Information Disclosure | `KAMACU_HOOK_BASE` env pointing to non-loopback URL | medium | accept | D-06: loopback binding is enforced by the SERVER (Kamacu's `ensureLoopback` + `hostCheck`), not the client. Even if the subcommand points at a non-loopback URL, a real Kamacu server refuses non-loopback Host headers. A non-Kamacu server at the redirected URL would not have Kamacu's data shape — the bridge returns whatever it gets. Acceptable for a single-user local app. |
| T-06-06 | Denial of Service | bridged HTTP timeout | low | accept | 10-second `http.Client.Timeout`, no retry (06-RESEARCH.md the agent's Discretion). A timeout means Kamacu is down; surfacing the error to the agent is the correct behavior. |
| T-06-07 | Tampering | process crash on tool-handler panic | medium | accept | 06-RESEARCH.md Pitfall 1: the SDK does NOT recover tool-handler panics. Phase 06 ships NO panic-recovery wrapping (Open Question 1 — deferred to Phase 08 when `subscribe_session_output` introduces more panic surface area). The list_projects handler has minimal panic surface area (no map writes, no slice indexing on user input — `req.Params.Arguments` is not unmarshaled). |
| T-06-08 | Elevation of Privilege | bridge response size unbounded | low | mitigate | The handler reads at most 1 MiB via `io.ReadAll(io.LimitReader(resp.Body, 1<<20))` — bounds the response size against a runaway Kamacu server. If a Phase 07+ tool returns more, bump the cap (one-line change). |
| T-06-SC | Tampering | go.mod adds `github.com/modelcontextprotocol/go-sdk v1.6.1` + transitive deps | medium | accept | Package Legitimacy Audit in 06-RESEARCH.md § "Package Legitimacy Audit" verified both the SDK and `google/subcommands` as OK (direct source read at v1.6.1 tag + proxy.golang.org + pkg.go.dev). All transitive deps (`google/jsonschema-go`, `segmentio/encoding`, `yosida95/uritemplate/v3`, `golang-jwt/jwt/v5`, `golang.org/x/oauth2`, `golang.org/x/tools`) are well-known and pre-evaluated as Low risk in 06-RESEARCH.md. No `[SUS]` / `[SLOP]` verdicts; no blocking human checkpoint required. |

</threat_model>

<verification>
- `go.mod` includes `github.com/modelcontextprotocol/go-sdk v1.6.1` as a direct requirement
- `internal/mcp/` package exists with `server.go`, `bridge.go`, `server_test.go`, `bridge_test.go`
- `cmd/kamacu/mcp.go` exists with `mcpCmd` using the Pattern 2 nested-Commander idiom
- `cmd/kamacu/main.go` registers exactly three Commands (HelpCommand, serveCmd, mcpCmd)
- `internal/mcp/server.go`'s `registerTools` calls `s.AddTool` exactly once for `list_projects`
- `internal/mcp/server_test.go` has the two SC2 sub-tests (error-returning + IsError-returning handler), NO panic test
- `internal/mcp/bridge_test.go` covers positive passthrough + non-200 + unreachable + env-default cases
- `internal/mcp/` is free of `os.Stdout.Write`, `fmt.Print*`, `log.Print*`, and `//go:build mcp_e2e`
- `go vet ./...` passes; `go test ./...` passes; `go build -o /tmp/kamacu-plan02 ./cmd/kamacu` succeeds
- `echo -n '' | /tmp/kamacu-plan02 mcp serve; echo $?` prints `0` (Pitfall 5)
- bare `/tmp/kamacu-plan02` help lists `serve` and `mcp`
</verification>

<success_criteria>
Phase 06 success criteria SC1, SC2, SC3, SC4 all delivered by this plan:
- SC1: `kamacu mcp serve` starts a long-lived MCP server using `github.com/modelcontextprotocol/go-sdk` v1.6.1 over stdio
- SC2: `tools/list` returns valid JSON-RPC; the SC2 unit test proves handlers returning errors emit clean JSON-RPC error responses on stdout (never crashes, never stream desync)
- SC3: a `tools/call` for `list_projects` routed through the bridge returns real Kamacu data; `X-Kamacu-Token` header sent on every request
- SC4: `KAMACU_HOOK_BASE` override and `127.0.0.1:7333` default both work (TestNewBridgeFromEnv_DefaultsAndOverride)

Phase 06 requirements MCPPROC-01, MCPPROC-02, MCPPROC-03 all delivered by this plan (Plan 01 contributes the dispatcher slot that MCPPROC-01 needs; Plan 03 contributes the X-Kamacu-Token header rename that MCPPROC-02 implies).
</success_criteria>

<output>
Create `.planning/phases/06-mcp-subcommand-foundation/06-02-SUMMARY.md` when done
</output>

## Artifacts this phase produces

This plan creates the following new symbols / files / struct fields (consumed by Phase 07+ plan-review-convergence source-grounding AND by Phases 07–09 themselves):

- **Go package (new):** `internal/mcp` — the bridge-pattern package every subsequent phase extends
- **CLI Command (new):** `ServeCommand` struct in `internal/mcp/server.go` implementing `subcommands.Command` (the body of `kamacu mcp serve`)
- **CLI Command (new):** `mcpCmd` struct in `cmd/kamacu/mcp.go` implementing `subcommands.Command` (the `mcp` group wrapper using Pattern 2 nested Commander)
- **Bridge type (new):** `bridge struct { base string; token string; client *http.Client }` in `internal/mcp/bridge.go`
  - Methods: `newBridgeFromEnv() (*bridge, error)`, `(b *bridge) do(ctx, method, path, body) (*http.Response, error)`, `(b *bridge) listProjects(ctx) (*mcp.CallToolResult, error)` — the listProjects method is the `mcp.ToolHandler` for `list_projects`
- **Tool registration function (new):** `registerTools(s *mcp.Server, b *bridge)` in `internal/mcp/server.go` — calls `s.AddTool` exactly once for `list_projects`
- **MCP tool (new, FINAL per D-09):** `list_projects` — registered with `InputSchema: json.RawMessage({"type":"object","properties":{"project_id":{"type":"integer","description":"..."}}})`, handler signature `func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)`, returns raw `GET /api/projects` JSON as a single `mcp.TextContent` block
- **Dependency (new in go.mod):** `github.com/modelcontextprotocol/go-sdk v1.6.1` (direct), plus transitive deps `google/jsonschema-go v0.4.3`, `segmentio/encoding v0.5.4`, `segmentio/asm v1.1.3`, `yosida95/uritemplate/v3 v3.0.2`, `golang-jwt/jwt/v5 v5.3.1`, `golang.org/x/oauth2 v0.35.0`, `golang.org/x/tools v0.42.0`
- **New file paths:** `cmd/kamacu/mcp.go`, `internal/mcp/server.go`, `internal/mcp/bridge.go`, `internal/mcp/server_test.go`, `internal/mcp/bridge_test.go`
- **Test functions (new):** `TestSC2_HandlerReturningError_ProducesCleanJSONRPCErrResponse`, `TestSC2_HandlerReturningIsErrorResult_ProducesCleanJSONRPCSuccessWithIsErrorTrue` (SC2 regression, D-11), `TestBridge_ListProjects_PassesThroughKamacuJSON`, `TestBridge_ListProjects_KamacuReturnsNon200_ReturnsError`, `TestBridge_ListProjects_KamacuUnreachable_ReturnsError`, `TestNewBridgeFromEnv_DefaultsAndOverride` (bridge integration)
- **main.go edit:** exactly one new line `subcommands.Register(mcpCmd{}, "")` added after the existing serveCmd registration
