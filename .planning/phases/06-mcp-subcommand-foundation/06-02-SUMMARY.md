---
phase: 06-mcp-subcommand-foundation
plan: 02
subsystem: mcp
tags: [mcp, json-rpc, stdio, modelcontextprotocol, subcommand, bridge]

requires:
  - phase: 06-01
    provides: "Thin cmd/kamacu/main.go dispatcher on google/subcommands; mcpCmd extends the registration list with one new line"
provides:
  - "internal/mcp package — bridge-pattern foundation every Phase 07-09 tool mechanically repeats"
  - "`kamacu mcp serve` subcommand body using github.com/modelcontextprotocol/go-sdk v1.6.1 over stdio"
  - "`list_projects` MCP tool (FINAL production code, D-09) — Phase 07 inherits unchanged and adds 12 more tools on the same pattern"
  - "SC2 malformed-tool regression test (D-11) — proves handlers returning errors emit clean JSON-RPC error responses on stdout (never crashes, never stream desync)"
  - "Bridge integration tests using httptest pattern — proves X-Kamacu-Token header sent on every call, list_projects returns raw /api/projects JSON passthrough, KAMACU_HOOK_BASE override + 127.0.0.1:7333 default (MCPPROC-03/SC4)"
affects: [07-task-project-workspace-tools, 08-session-terminal-tools, 09-pr-review-tools]

tech-stack:
  added:
    - github.com/modelcontextprotocol/go-sdk v1.6.1
    - github.com/google/jsonschema-go v0.4.3 (transitive)
    - github.com/segmentio/encoding v0.5.4 (transitive)
    - github.com/segmentio/asm v1.1.3 (transitive)
    - github.com/yosida95/uritemplate/v3 v3.0.2 (transitive)
    - github.com/golang-jwt/jwt/v5 v5.3.1 (transitive)
    - golang.org/x/oauth2 v0.35.0 (transitive)
    - golang.org/x/tools v0.42.0 (transitive)
  patterns:
    - "Bridge pattern: env parse → *bridge HTTP client with X-Kamacu-Token header → call Kamacu endpoint → return as MCP TextContent passthrough"
    - "Pattern 2 nested Commander — `google/subcommands` is FLAT; an `mcp` Command owns an inner subcommands.Commander to dispatch `mcp serve`"
    - "Low-level Server.AddTool with explicit map[string]any InputSchema — NOT the typed generic AddTool helper (06-RESEARCH.md Anti-Patterns)"

key-files:
  created:
    - cmd/kamacu/mcp.go
    - internal/mcp/server.go
    - internal/mcp/bridge.go
    - internal/mcp/server_test.go
    - internal/mcp/bridge_test.go
  modified:
    - cmd/kamacu/main.go
    - go.mod
    - go.sum

key-decisions:
  - "Used `map[string]any` for Tool.InputSchema instead of `json.RawMessage`. SDK v1.6.1's Tool.InputSchema field is `any` (changed from earlier SDK versions the plan/RESEARCH referenced). []byte gets marshaled as base64 → AddTool panics. `map[string]any{\"type\": \"object\", ...}` satisfies the type:object requirement without pulling in jsonschema-go's typed reflection API (Rule 1 auto-fix — preserves the plan's intent of avoiding typed-schema coupling)."
  - "Used `cdr.HelpCommand()` (method on inner Commander) instead of `subcommands.HelpCommand(cdr)` (which doesn't exist in v1.2.0). The top-level `subcommands.HelpCommand()` is for DefaultCommander; inner commanders expose help via their own method (Rule 1 auto-fix — API drift from RESEARCH.md)."
  - "Drove the SC2 test via `mcp.NewInMemoryTransports()` + real `mcp.Client` end-to-end — exercises the full handler → SDK → wire → client SDK → caller roundtrip, not just direct handler invocation. This catches any future regression where the SDK starts emitting non-JSON bytes or crashing on handler errors."
  - "SDK `nil` options (not `mcp.NewServer(...).WithToolCapabilities(...)` etc.) — the SDK's default logger discards log output entirely (slog.DiscardHandler per RESEARCH Pitfall 5), so D-12(a) is satisfied trivially without an explicit logger config."

patterns-established:
  - "Pattern: `*bridge` struct injected into tool handlers via closure — every Phase 07+ tool follows this shape (env parse once at startup, *bridge threaded into handler closures)"
  - "Pattern: tool handler signature `func(ctx, *mcp.CallToolRequest) (*mcp.CallToolResult, error)` returning raw TextContent passthrough; transport/non-200 errors return wrapped errors (SDK wraps as JSON-RPC error.code = -32603)"
  - "Pattern: `subcommands.Register(&cmd{}, \"\")` for Commands needing a nested Commander — mcpCmd's Execute constructs `subcommands.NewCommander(f, \"kamacu mcp\")` and dispatches"

requirements-completed: [MCPPROC-01, MCPPROC-02, MCPPROC-03]

coverage:
  - id: D1
    description: "`go.mod` contains `github.com/modelcontextprotocol/go-sdk v1.6.1` as a direct requirement"
    requirement: MCPPROC-01
    verification:
      - kind: integration
        ref: "grep -q 'github.com/modelcontextprotocol/go-sdk v1.6.1' go.mod"
        status: pass
    human_judgment: false
  - id: D2
    description: "`internal/mcp/bridge.go` defines `bridge`, `newBridgeFromEnv`, `bridge.do`, `bridge.listProjects`; sets `X-Kamacu-Token` header on every request; defaults to `http://127.0.0.1:7333`"
    requirement: MCPPROC-02
    verification:
      - kind: unit
        ref: "internal/mcp/bridge_test.go#TestBridge_ListProjects_PassesThroughKamacuJSON — asserts X-Kamacu-Token header is set"
        status: pass
      - kind: unit
        ref: "internal/mcp/bridge_test.go#TestNewBridgeFromEnv_DefaultsAndOverride — proves default + override + trailing-slash trim"
        status: pass
    human_judgment: false
  - id: D3
    description: "`internal/mcp/server.go` defines `ServeCommand` (5-method subcommands.Command impl) and `registerTools` (calls `s.AddTool` exactly once for `list_projects`)"
    requirement: MCPPROC-01
    verification:
      - kind: integration
        ref: "grep -c 's.AddTool\\|mcp.AddTool' internal/mcp/server.go → 1"
        status: pass
      - kind: integration
        ref: "go build + go vet ./internal/mcp/..."
        status: pass
    human_judgment: false
  - id: D4
    description: "SC2 regression test (D-11): two sub-tests prove handler-returned errors emit clean JSON-RPC error responses on stdout; NO panic test (Pitfall 1)"
    requirement: MCPPROC-01
    verification:
      - kind: unit
        ref: "internal/mcp/server_test.go#TestSC2_HandlerReturningError_ProducesCleanJSONRPCErrResponse — PASS (asserts 'boom' in error message)"
        status: pass
      - kind: unit
        ref: "internal/mcp/server_test.go#TestSC2_HandlerReturningIsErrorResult_ProducesCleanJSONRPCSuccessWithIsErrorTrue — PASS (asserts result.IsError=true, result.Content[0].Text='boom')"
        status: pass
    human_judgment: false
  - id: D5
    description: "`ServeCommand.Execute` maps `srv.Run` nil return to `subcommands.ExitSuccess` (Pitfall 5 load-bearing — `echo | kamacu mcp serve; echo $?` prints 0)"
    requirement: MCPPROC-01
    verification:
      - kind: integration
        ref: "echo -n '' | /tmp/kamacu-plan02 mcp serve; echo $? → 0"
        status: pass
    human_judgment: false
  - id: D6
    description: "`cmd/kamacu/mcp.go` defines `mcpCmd` using Pattern 2 (inner subcommands.Commander, registers kamacumcp.ServeCommand{}); `cmd/kamacu/main.go` registers exactly three Commands (HelpCommand, serveCmd, mcpCmd)"
    requirement: MCPPROC-01
    verification:
      - kind: integration
        ref: "grep -c 'subcommands.Register(' cmd/kamacu/main.go → 3; grep 'subcommands.NewCommander' cmd/kamacu/mcp.go; grep 'kamacumcp.ServeCommand{}' cmd/kamacu/mcp.go"
        status: pass
      - kind: integration
        ref: "/tmp/kamacu-plan02 2>&1 | grep -q serve && grep -q mcp — both subcommands listed in top-level help"
        status: pass
      - kind: integration
        ref: "/tmp/kamacu-plan02 mcp 2>&1 | grep -q serve — inner help lists serve"
        status: pass
    human_judgment: false
  - id: D7
    description: "`internal/mcp/bridge_test.go` covers positive passthrough + non-200 + unreachable + env-default cases (httptest pattern); uses t.Setenv for env manipulation"
    requirement: MCPPROC-03
    verification:
      - kind: unit
        ref: "internal/mcp/bridge_test.go#TestBridge_ListProjects_PassesThroughKamacuJSON — PASS (asserts canned JSON, X-Kamacu-Token header, path, method)"
        status: pass
      - kind: unit
        ref: "internal/mcp/bridge_test.go#TestBridge_ListProjects_KamacuReturnsNon200_ReturnsError — PASS (asserts 'HTTP 500' in error)"
        status: pass
      - kind: unit
        ref: "internal/mcp/bridge_test.go#TestBridge_ListProjects_KamacuUnreachable_ReturnsError — PASS (asserts 'kamacu bridge:' in error)"
        status: pass
      - kind: unit
        ref: "internal/mcp/bridge_test.go#TestNewBridgeFromEnv_DefaultsAndOverride — PASS (default, override, trailing-slash trim)"
        status: pass
    human_judgment: false
  - id: D8
    description: "`internal/mcp/` is free of `os.Stdout.Write`, `fmt.Print*`, `log.Print*`, and `//go:build mcp_e2e` (D-12)"
    requirement: MCPPROC-01
    verification:
      - kind: integration
        ref: "grep -rn 'os.Stdout.Write\\|fmt.Print\\|log.Print' internal/mcp/ → no matches; grep -rn 'go:build mcp_e2e' internal/mcp/ → no matches"
        status: pass
    human_judgment: false
  - id: D9
    description: "`go vet ./...` passes; `go test ./...` passes; `go build -o /tmp/kamacu-plan02 ./cmd/kamacu` succeeds"
    requirement: MCPPROC-01
    verification:
      - kind: integration
        ref: "go vet ./... + go test ./... + go build — all clean"
        status: pass
    human_judgment: false

duration: 22min
completed: 2026-07-21
status: complete
---

# Phase 06 Plan 02: MCP Serve Subcommand Summary

**`internal/mcp/` package built on `github.com/modelcontextprotocol/go-sdk` v1.6.1 — `kamacu mcp serve` speaks JSON-RPC over stdio, registers exactly one production tool (`list_projects`), and is wired into the dispatcher via Pattern 2 nested Commander. The SC2 malformed-tool regression test (D-11) proves error handlers emit clean JSON-RPC error responses on stdout — never crashes, never stream desync.**

## Performance

- **Duration:** ~22 min
- **Started:** 2026-07-21T19:00:00Z
- **Completed:** 2026-07-21T19:22:00Z
- **Tasks:** 2
- **Files modified:** 8 (5 new + 3 edited)

## Accomplishments
- Added `github.com/modelcontextprotocol/go-sdk v1.6.1` to `go.mod` as a direct requirement (plus pre-verified transitive deps: `google/jsonschema-go`, `segmentio/encoding`+`asm`, `yosida95/uritemplate/v3`, `golang-jwt/jwt/v5`, `golang.org/x/oauth2`, `golang.org/x/tools`).
- Created `internal/mcp/bridge.go` — the per-process HTTP bridge: `newBridgeFromEnv` reads `KAMACU_HOOK_BASE` (default `http://127.0.0.1:7333` per MCPPROC-03/SC4) + `KAMACU_HOOK_TOKEN`, sets `X-Kamacu-Token` on every request (D-06/D-07 wire contract), 10s timeout, no retry. `listProjects` returns raw `GET /api/projects` JSON as a single `TextContent` block (D-08 raw passthrough; the optional `project_id` is a documented no-op).
- Created `internal/mcp/server.go` — `ServeCommand` implements `subcommands.Command`, runs `mcp.NewServer(...)` with nil options (SDK discards log output), `registerTools` calls `s.AddTool` exactly once for `list_projects` (D-09/D-10), and `srv.Run` over `mcp.StdioTransport`. Pitfall 5 mapping: nil return from `Run` (stdin EOF) → `subcommands.ExitSuccess` so `echo | kamacu mcp serve; echo $?` prints `0`.
- Created `internal/mcp/server_test.go` — SC2 regression test (D-11): two sub-tests covering error-returning handler (asserts clean JSON-RPC error with `"boom"` message) and IsError-returning handler (asserts JSON-RPC success with `result.isError=true`). NO panic test (Pitfall 1). Drives server+client over `mcp.NewInMemoryTransports()` end-to-end.
- Created `cmd/kamacu/mcp.go` — `mcpCmd` uses Pattern 2 nested Commander (`subcommands.NewCommander(f, "kamacu mcp")`, registers `cdr.HelpCommand()` + `kamacumcp.ServeCommand{}`, dispatches via `cdr.Execute(ctx)`). No future-subcommand stubs (D-05).
- Edited `cmd/kamacu/main.go` — exactly one new line: `subcommands.Register(mcpCmd{}, "")`. main.go now registers exactly three Commands (HelpCommand, serveCmd, mcpCmd).
- Created `internal/mcp/bridge_test.go` — four bridge integration tests using the project's established httptest pattern: positive passthrough (asserts canned `[]Project` JSON, `X-Kamacu-Token` header, path, method), non-200 negative (asserts `"HTTP 500"` in error), unreachable negative (asserts `"kamacu bridge:"` in error), and `TestNewBridgeFromEnv_DefaultsAndOverride` (default, override, trailing-slash trim).
- Verified all functional behaviors: bare `kamacu` help lists both `serve` and `mcp`; `kamacu mcp` lists `serve`; `echo -n '' | kamacu mcp serve` exits 0 (Pitfall 5); `go vet ./...`, `go test ./...`, and `go build` all clean.

## Task Commits

Each task was committed atomically:

1. **Task 1: Create internal/mcp package (server.go + bridge.go) with list_projects tool + SC2 malformed-tool unit test** — `1d3af44` (feat)
2. **Task 2: Wire mcpCmd into main.go (Pattern 2 nested Commander), register in dispatcher, add bridge integration test** — `f91f239` (feat)

**Plan metadata:** this commit (docs: complete mcp-serve-subcommand plan)

## Files Created/Modified
- `cmd/kamacu/mcp.go` — NEW (~50 lines): `mcpCmd` struct implementing `subcommands.Command` with nested-Commander dispatch.
- `internal/mcp/server.go` — NEW (~110 lines): `ServeCommand` (the `mcp serve` body) + `registerTools` (registers exactly one tool).
- `internal/mcp/bridge.go` — NEW (~100 lines): `bridge` struct, `newBridgeFromEnv`, `bridge.do`, `bridge.listProjects`.
- `internal/mcp/server_test.go` — NEW (~120 lines): two SC2 sub-tests driving server+client over in-memory transport.
- `internal/mcp/bridge_test.go` — NEW (~140 lines): four bridge integration tests using httptest pattern.
- `cmd/kamacu/main.go` — one new line: `subcommands.Register(mcpCmd{}, "")`.
- `go.mod` / `go.sum` — adds SDK + transitive deps.

## Decisions Made
- **Used `map[string]any` for `Tool.InputSchema` instead of `json.RawMessage`.** The plan and RESEARCH.md assumed `InputSchema` was `json.RawMessage`, but at SDK v1.6.1 the field is `any` (changed since the RESEARCH was written). Using `[]byte` causes `AddTool` to panic because Go marshals `[]byte` as base64 — `can't marshal input schema to a JSON object: json: cannot unmarshal "\"eyJ0eXBlIjoib2JqZWN0In0=\"" into Go value of type map[string]interface {}`. A `map[string]any{"type": "object", "properties": ...}` satisfies the type:object requirement cleanly without pulling in jsonschema-go's typed reflection API (preserves the plan's intent). This is a Rule 1 auto-fix (compile-time API drift).
- **Used `cdr.HelpCommand()` (method) instead of `subcommands.HelpCommand(cdr)` (which doesn't exist).** At `google/subcommands` v1.2.0, `subcommands.HelpCommand()` is parameterless (for `DefaultCommander`); inner commanders expose help via the method `cdr.HelpCommand()`. Rule 1 auto-fix (API drift from RESEARCH.md's pseudocode).
- **Drove SC2 test via `mcp.NewInMemoryTransports()` end-to-end.** Rather than synthesizing a `*mcp.CallToolRequest` directly, the test stands up a real `mcp.Client` over the in-memory transport and calls `clientSession.CallTool(ctx, params)`. This exercises the full handler → SDK → wire → client SDK → caller roundtrip — so any future regression in the SDK's error-framing would surface. The test is still a unit test (no build tag, runs under default `go test ./...`).
- **SDK `nil` options means the default logger discards output entirely.** Per RESEARCH Pitfall 5, `mcp.NewServer(impl, nil)` is silent (no logger overhead). The subcommand's own `slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))` pins Go's stdlib default to stderr so future import-graph changes can't redirect slog to stdout (D-12(a)).
- **Used `mcpsdk` alias for the SDK import in `bridge_test.go`.** Our package is also named `mcp`; aliasing the SDK import as `mcpsdk` lets the test reference `*mcpsdk.TextContent` without shadowing our own package's namespace.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - API Drift] Tool.InputSchema is `any`, not `json.RawMessage`**
- **Found during:** Task 1 (writing `registerTools`)
- **Issue:** Plan/RESEARCH documented `InputSchema: json.RawMessage(\`{"type":"object"}\`)`, but SDK v1.6.1's `Tool.InputSchema` is `any`. Using `[]byte` panics in `AddTool` with `can't marshal input schema to a JSON object: json: cannot unmarshal "<base64>" into Go value of type map[string]interface {}` because Go marshals `[]byte` as base64.
- **Fix:** Use `map[string]any{"type": "object", "properties": ...}` for the schema. This preserves the plan's intent (avoid jsonschema-go's typed reflection; explicit schema over inferred) while satisfying the v1.6.1 API.
- **Files modified:** `internal/mcp/server.go` (production), `internal/mcp/server_test.go` (test schema literals).
- **Verification:** `go test ./internal/mcp/... -run TestSC2 -v` — both sub-tests PASS.
- **Committed in:** `1d3af44` (Task 1 commit).

**2. [Rule 1 - API Drift] `subcommands.HelpCommand()` signature changed**
- **Found during:** Task 2 (writing `mcpCmd.Execute`)
- **Issue:** Plan/RESEARCH documented `subcommands.HelpCommand(cdr)` (passing the inner commander), but at `google/subcommands` v1.2.0 the top-level `subcommands.HelpCommand()` takes no arguments (it's for `DefaultCommander`); inner commanders expose help via the method `cdr.HelpCommand()`.
- **Fix:** Use `cdr.HelpCommand()` instead of `subcommands.HelpCommand(cdr)`.
- **Files modified:** `cmd/kamacu/mcp.go`.
- **Verification:** `go vet ./...` clean; `go build -o /tmp/kamacu-plan02 ./cmd/kamacu` succeeds; `/tmp/kamacu-plan02 mcp` lists `help` and `serve`.
- **Committed in:** `f91f239` (Task 2 commit).

---

**Total deviations:** 2 auto-fixed (both Rule 1 — SDK/library API drift from RESEARCH.md pseudocode).
**Impact on plan:** Both fixes preserve the plan's intent (explicit schema over inferred; nested Commander shape). No scope creep. The plan's prohibitions (no typed AddTool[In,Out]; no `?project_id=` query; no stdout writes; no panic test) are all respected.

## Issues Encountered
None beyond the two API-drift auto-fixes above. The full `go test ./...` is clean on this run (no PTY-timing flakes this round).

## User Setup Required
None - the subcommand is env-driven. Users add `kamacu mcp serve` to their agent CLI's MCP config (`.mcp.json` for claude, `opencode.json` for opencode) — that registration step is MCPREG-01..03, deferred to v1.12 per the plan. In v1.11 users manually add the line.

## Next Phase Readiness
- **Phase 07 ready.** The `internal/mcp/` package is the foundation; Phase 07 inherits `list_projects` unchanged (D-09) and adds the remaining task/project/workspace tools on the same `*bridge` pattern. Each Phase 07 tool is a `bridge.<verb>` method + one new `s.AddTool` line in `registerTools`.
- **Phase 08 ready.** The `internal/mcp/` package shape supports the 4 session/terminal tools (with a small `defer recover()` addition when `subscribe_session_output` introduces more panic surface area — explicitly Phase 08 work per 06-RESEARCH Open Question 1).
- **Phase 09 ready.** The 3 PR-review tools follow the same pattern.
- **MCPPROC-01, MCPPROC-02, MCPPROC-03 all delivered.** Phase 06 success criteria SC1, SC2, SC3, SC4 all met:
  - **SC1:** `kamacu mcp serve` starts a long-lived MCP server over stdio using the SDK.
  - **SC2:** `tools/list` over stdout emits valid JSON-RPC; SC2 test proves error handlers emit clean JSON-RPC error responses.
  - **SC3:** `tools/call list_projects` routed through the bridge returns real Kamacu data; `X-Kamacu-Token` header sent on every request.
  - **SC4:** `KAMACU_HOOK_BASE` override and `127.0.0.1:7333` default both work (TestNewBridgeFromEnv_DefaultsAndOverride).

---
*Phase: 06-mcp-subcommand-foundation*
*Completed: 2026-07-21*
