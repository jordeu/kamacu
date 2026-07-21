# Phase 06: MCP Subcommand Foundation - Context

**Gathered:** 2026-07-21
**Status:** Ready for planning

<domain>
## Phase Boundary

The architecture-proof phase for v1.11: stand up the `kamacu mcp serve` stdio subcommand that speaks the MCP protocol over stdio and bridges tool calls to the running Kamacu HTTP API. The bridge pattern is proven end-to-end via **ONE production-grade read-only tool** (`list_projects`); every subsequent phase (07–09) repeats that pattern mechanically.

Delivers:
1. **CLI dispatcher refactor** — `cmd/kamacu/main.go` adopts `github.com/google/subcommands`; today's flag-based server path becomes `kamacu serve`; the MCP subcommand becomes `kamacu mcp serve`. `kamacu` with no args prints help (the library default). Breaking change, accepted (D-03/D-04).
2. **`kamacu mcp serve` subcommand** — a long-lived stdio process spawned by an agent CLI (claude/opencode) inside a Kamacu task PTY. Uses `github.com/modelcontextprotocol/go-sdk` v1.6.1 to speak JSON-RPC over stdio. Body lives in a new `internal/mcp/` package; main.go only registers Commands and dispatches.
3. **HTTP bridge to Kamacu** — tool calls translate to HTTP requests against the running Kamacu server at the URL in `KAMACU_HOOK_BASE` (default `127.0.0.1:7333`). Auth rides on `KAMACU_HOOK_TOKEN` env, sent in a `X-Kamacu-Token` header. Loopback binding remains the actual auth boundary (D-06).
4. **Token header rename** — Phase 06 renames `X-Kangent-Token` → `X-Kamacu-Token` everywhere: the hook receiver, the claude settings overlay curl template, the opencode plugin source, and the relevant tests. Coordinated Go + JS + test rename; no fallback during transition (D-07).
5. **One production tool** — `list_projects(project_id?)` is registered, calls `GET /api/projects` unchanged, and returns real Kamacu data. This tool is FINAL production code; Phase 07 simply adds 12 more tools on the same pattern (D-08/D-09).
6. **SC2 regression test** — a unit test in `internal/mcp/` registers a deliberately-malformed tool and asserts the server emits a clean JSON-RPC error response on stdout (never crashes, never emits a non-JSON line). Localized to the package; no e2e harness (D-11).

**Requirements covered:** MCPPROC-01, MCPPROC-02, MCPPROC-03.

**Out of this phase (per REQUIREMENTS.md Out of Scope / Deferred):**
- Per-task auto-scoping (`get_my_task`, ToolFilter) — MCPAUTO-01..03, deferred to v1.12.
- Agent-CLI auto-registration (`.mcp.json` / `opencode.json` writers at spawn) — MCPREG-01..03, deferred to v1.12. Users manually add `kamacu mcp serve` to their agent CLI's MCP config in v1.11.
- The other 12 task/project/workspace tools (MCPTASK × 6 + MCPPROJ × 7) — Phase 07.
- All session/terminal tools (MCPSESS × 4) — Phase 08.
- All PR-review tools (MCPREV × 3) — Phase 09.
- Typed JSON-RPC error taxonomy, active stdout guards (os.Stdout redirect), CI grep check, real-binary e2e harness — MCPHARD-01..03, deferred to v1.12. Phase 06 ships only the SC2 unit test (D-11) and relies on stdlib defaults + code review for stdout cleanliness (D-12).
- Server-side token validation middleware on general `/api/*` routes — explicitly out of scope for v1.11 (D-06); loopback binding is the auth boundary.
- PTY keystroke injection via MCP — permanently out of scope per REQUIREMENTS.md.
</domain>

<decisions>
## Implementation Decisions

### CLI dispatcher refactor (D-01..D-05)
- **D-01:** Adopt **`github.com/google/subcommands`** for CLI dispatch. Refactor `cmd/kamacu/main.go` to use the `Command` interface: today's flag-based server path becomes the `serve` `Command`; `mcp serve` is a second `Command` (registered under a `mcp` command group). Adds one stdlib-adjacent dep (zero transitive deps). The existing `flag.Parse()` block becomes the `serve` Command's `SetFlags` method.
- **D-02:** **Thin dispatch in main.go, body in `internal/mcp/`.** main.go only registers `Command`s with `subcommands.Register(...)` and calls `subcommands.Execute(context.Background(), ...)`. The MCP server body — SDK init, tool registration, stdio loop, env parsing — lives in a new `internal/mcp/` package, testable in isolation without building the binary. Matches the project pattern (`internal/api/`, `internal/session/`, `internal/worktree/`, etc.).
- **D-03:** Server subcommand name is the **explicit `serve`**. `kamacu serve` runs the HTTP server (today's main.go body); `kamacu mcp serve` runs the MCP stdio subcommand; `kamacu` (no args) prints help (the `google/subcommands` default). The MCP subcommand is registered as `mcp serve` (i.e., under a `mcp` command group with `serve` as its name) so future `mcp <other>` subcommands land cleanly.
- **D-04:** **Break clean on the new `serve` requirement** — no transitional shim. Update `README.md`, in-repo docs (`docs/`), scripts (`Makefile`, `Taskfile` if present), and any examples. Single-user local app = small blast radius. A bare `kamacu` invocation prints help pointing to `kamacu serve` (the library default behavior). Existing `--addr`, `--db`, `--claude-bin`, `--dev-origin`, `--insecure-allow-remote` flags become the `serve` Command's FlagSet; their semantics are unchanged.
- **D-05:** **Register only `mcp serve`** in Phase 06 — no future-subcommand stubs (no `mcp doctor`, no `mcp tokens`, no top-level non-`mcp` Commands besides `serve` and the library's built-in `help`). The `Command` interface is in place; future subcommands slot in when their phase lands. No dead code, no `not implemented` paths to maintain.

### Token validation scope (D-06..D-07)
- **D-06:** **Loopback-only enforcement for v1.11.** The MCP subcommand sends `KAMACU_HOOK_TOKEN` in the `X-Kamacu-Token` header on every bridged HTTP request, but Phase 06 adds **NO new server-side middleware on `/api/*` routes**. Loopback binding (`hostCheck` middleware + WS Origin allowlist + `ensureLoopback` on `--addr`) remains the actual auth boundary for general `/api/*` routes — the same posture the SPA fetches have always had. The token is decorative on general routes in v1.11; MCPPROC-02 is satisfied by "the subcommand sends the token AND is only meaningful when running on the same host as the Kamacu server (loopback-bound)." The typed JSON-RPC error taxonomy (MCPHARD-01) is deferred to v1.12; if research surfaces a clean way to add a server-side check without SPA impact, escalate to the user before implementing.
- **D-07:** **Header name is `X-Kamacu-Token`** (rebrand-aligned). Phase 06 renames the existing `X-Kangent-Token` usage **everywhere** in one coordinated change:
  - **Receiver:** `internal/api/hooks.go:39` — `r.Header.Get("X-Kangent-Token")` → `r.Header.Get("X-Kamacu-Token")`. Update the inline comments that reference the old name.
  - **Claude overlay:** `internal/session/agent.go:23` (Token field doc comment) and `internal/session/agent.go:63` (the curl template: `"curl -s -m 3 -H 'X-Kangent-Token: %s' ..."` → `'X-Kamacu-Token: %s'`).
  - **opencode plugin:** `internal/opencode/kamacu-status.js:79` — the literal header name string in the fetch call. This ships in the installed plugin file — `opencode.InstallPlugin` writes the new content on the next boot (existing installs get the renamed header via the regenerate-on-boot path).
  - **Tests:** `internal/session/agent_test.go:94`, `internal/opencode/plugin_test.go:195`, `internal/opencode/e2e_test.go:136` and `:144`, `internal/api/hooks_test.go:87` and the `X-Kangent-Token` literal in `:103`'s comment.
  - **No `X-Kangent-Token` fallback during transition** — break clean. The hook token is regenerated fresh on every Kamacu start and re-injected into spawned agents, so a Kamacu server and its agents are always from the same release.

### Proof tool selection (D-08..D-09)
- **D-08:** **Proof tool is `list_projects`.** Calls the existing `GET /api/projects` endpoint (registered in `internal/api/routes.go:32`, implemented in `internal/api/projects.go` `projectHandlers.list`) **unchanged** — no new endpoint, no schema change. The MCP tool signature is `list_projects(project_id?: int)` returning the projects array as MCP tool output. (The endpoint doesn't currently filter by `workspace_id` at the handler level; if Phase 07 wants workspace filtering it adds it there. Phase 06 returns all projects verbatim.) `MCPTASK-01`'s unscoped `list_tasks` becomes Phase 07's work — either add `GET /api/tasks` (unscoped) or make `list_tasks`'s `project_id` required.
- **D-09:** **Phase 06 proof tool is FINAL production code.** The same `list_projects` tool ships unchanged in Phase 07. Phase 07 simply ADDS the remaining 12 tools (6 task + 5 project + 7 workspace CRUD + transfer — wait, that's 19; per REQUIREMENTS.md traceability it's 13 in Phase 07 counting `move_project_to_workspace`). The bridge pattern proven here — env parse → HTTP client with token header → call existing endpoint → translate response to MCP tool output — is mechanically repeated. Phase 07 does NOT rebuild `list_projects`.

### Tool registration breadth & testing (D-10..D-12)
- **D-10:** **`tools/list` returns ONE tool: `list_projects`.** Matches SC3 literally. Phase 07 adds the other 12 task/project/workspace tools in one batch; Phase 08 adds 4 session tools; Phase 09 adds 3 review tools. No "not implemented until Phase 07+" stubs — stubs are dead code that would have to be removed and could confuse the SC2 / SC3 e2e proof.
- **D-11:** **SC2's malformed-tool regression test is a UNIT test localized to `internal/mcp/`.** Registers a deliberately-malformed tool (e.g., a handler that returns invalid JSON-RPC, or a tool whose schema is broken) and asserts the server emits a clean JSON-RPC error response on stdout (never crashes, never emits a non-JSON line, never desyncs the stream). This tests the SDK's invariant from SC2 directly at the package boundary. **No build-tagged e2e harness** — MCPHARD-03 (real-binary e2e with real Claude Code + real opencode) is deferred to v1.12.
- **D-12:** **Stdout cleanliness in Phase 06 relies on stdlib defaults + code-review convention.** Specifically: (a) verify in research that `slog.Default()` writes to **stderr** (the Go stdlib default — `slog.NewTextHandler(os.Stderr, nil)`; if anything in the subcommand's init path redirectss slog to stdout, fix it); (b) `internal/mcp/` package code is **`fmt.Print*`-free, `log.Print*`-free, `os.Stdout.Write`-free by convention** — code review enforces, no automated check; (c) no active stdout guard wrapper (no buffering Write calls), no CI grep check. The full MCPHARD-02 stdout-pollution defense (os.Stdout redirect from line zero, CI grep check) is deferred to v1.12.

### the agent's Discretion
- **google/subcommands registration shape** — whether to register `mcp` as a command group via `subcommands.Register(subcommands.Group("mcp", "..."), ...)` or as individual commands. Pick whatever the library idiom suggests.
- **`internal/mcp/` package file layout** — single `internal/mcp/serve.go` vs. split (e.g., `server.go`, `bridge.go`, `tools.go`, `client.go`). Follow existing package-file patterns (`internal/api/projects.go` is one file for one resource; the analog here is small enough to be one or two files).
- **MCP server identification** — the name and version string the server announces in its MCP handshake (likely `"kamacu"` and the binary's build version). No user preference; researcher picks based on SDK conventions.
- **HTTP client construction** — timeout, transport, retry behavior for the bridge client. No user preference; conservative defaults (short timeout, no retry — fail fast and surface the MCP error).
- **Exact malformed-tool unit test cases** — which malformations to cover (broken schema, panic in handler, invalid return type). No user preference; cover the SDK's documented failure modes.
- **`list_projects` MCP output shape** — whether to return the raw JSON the Kamacu API returns, wrap it in a structured MCP result, or massage fields. No user preference; pass through the Kamacu response as MCP tool output content with minimal massaging.
- **Stdin / lifecycle behavior** — graceful shutdown on stdin close (the SDK's default), exit codes on error. Follow the SDK defaults.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & roadmap (scope anchors)
- `.planning/ROADMAP.md` § "Phase 06: MCP Subcommand Foundation" — the goal and the **4 success criteria** (long-lived MCP server; `tools/list` over stdout-only; bridge returns real Kamacu data via the token; `KAMACU_HOOK_BASE` override + `127.0.0.1:7333` default). SC2's malformed-tool regression test is the load-bearing detail.
- `.planning/REQUIREMENTS.md` — **MCPPROC-01 / MCPPROC-02 / MCPPROC-03** (locked). Read the **Out of Scope** table carefully — PTY injection, auto-registration, per-task auto-scoping, additional tool categories, and the entire MCPHARD-* list are all out of Phase 06. Read the **v1.12+ Requirements** section to understand what is intentionally NOT being built yet (so it isn't accidentally pulled in).
- `.planning/PROJECT.md` § "Active: v1.11 Kamacu MCP Server" — the milestone goal paragraph and the explicit "Backend-only milestone: no DB schema changes, no migrations, no frontend changes, no new long-lived goroutines inside the Kamacu binary."

### MCP SDK (new dependency — researcher must read)
- `github.com/modelcontextprotocol/go-sdk` @ **v1.6.1** — the SDK to use. **Researcher must verify** the API surface for: stdio server construction, tool registration, tool handler signature, JSON-RPC error response shape, stdout/stdin handling, and graceful shutdown on stdin close. The Phase 06 plan depends on these specifics.
- `go.mod` — currently does NOT include the SDK; Phase 06 adds both `github.com/modelcontextprotocol/go-sdk` and `github.com/google/subcommands`.

### Existing CLI / flag handling (D-01..D-05 affect these)
- `cmd/kamacu/main.go` — today's bare-flag entry point. The whole `main()` body from `flag.String("addr", ...)` through `http.ListenAndServe(...)` becomes the `serve` `Command`'s `Execute` method. `hostCheck` (line 419), `ensureLoopback` (line 408), `hookBaseURL`, `sweepOrphanTmux`, and the backfill one-shots (`BackfillProjectIcons`, `BackfillWorkspaces`, `BackfillAgents`, `BackfillAgentExtraParams`, `BackfillOpenCodeAgent`) all move with it verbatim.
- `cmd/kamacu/main.go:45` — `flag.Parse()` is the last "today's behavior" line. Under `google/subcommands`, this is replaced by `subcommands.Execute(context.Background(), os.Args[1:])` after registering commands.

### Token envelope (D-06/D-07 affect these — rename X-Kangent-Token → X-Kamacu-Token)
- `internal/api/hooks.go:39` — `got := r.Header.Get("X-Kangent-Token")` — the receiver's constant-time compare (line 40 `subtle.ConstantTimeCompare`). Read the comments at lines 17–18 about why the token is required (defense-in-depth against no-CORS fetch POSTs from malicious webpages); the rename preserves the security posture byte-for-byte.
- `internal/session/agent.go:23` — `Token     string // per-instance X-Kangent-Token value` (the `AgentConfig` field doc comment).
- `internal/session/agent.go:63` — the claude settings overlay curl template: `"curl -s -m 3 -H 'X-Kangent-Token: %s' --data-binary @- %s/api/hooks/sessions/%s"`. This is the literal string embedded in the agent CLI's settings overlay; renaming it to `'X-Kamacu-Token: %s'` is the entire change.
- `internal/opencode/kamacu-status.js:79` — `'X-Kangent-Token': token,` in the fetch headers literal. This is the on-disk plugin file written by `opencode.InstallPlugin`; the rename ships in the next-boot regenerate path (existing installs update automatically).
- `internal/session/manager.go:185-187` — the spawn-time env injection (`KAMACU_SESSION_ID`, `KAMACU_HOOK_TOKEN`, `KAMACU_HOOK_BASE`). **No changes needed here in Phase 06** — the env var NAMES stay `KAMACU_*` (only the HTTP header name changes); the MCP subcommand reads `KAMACU_HOOK_TOKEN` and `KAMACU_HOOK_BASE` from env and constructs the `X-Kamacu-Token` HTTP header on every bridge request.
- Tests: `internal/session/agent_test.go:94`, `internal/opencode/plugin_test.go:195`, `internal/opencode/e2e_test.go:136` & `:144`, `internal/api/hooks_test.go:87` & `:103`. All become mechanical `s/X-Kangent-Token/X-Kamacu-Token/g` updates.

### Existing /api/* routes (proof tool bridge target — D-08)
- `internal/api/routes.go:32` — `mux.HandleFunc("GET /api/projects", p.list)` — the `list_projects` MCP tool's HTTP bridge target.
- `internal/api/projects.go` `projectHandlers.list` — the handler implementation. **No changes needed**; the MCP bridge calls this endpoint and passes the response through as MCP tool output.

### Server auth posture (D-06 — what NOT to change)
- `cmd/kamacu/main.go:419-439` — `hostCheck` middleware (DNS-rebinding defense; loopback-only Host header).
- `cmd/kamacu/main.go:408-417` — `ensureLoopback` (refuses non-loopback `--addr`).
- `internal/ws/handler.go:20-50` — WS Origin allowlist (loopback origins + `--dev-origin` entries; non-browser clients allowed).
- These three layers ARE the v1.11 auth boundary for general `/api/*`. D-06 means Phase 06 does NOT add token validation alongside them.

### Established project patterns (constraints, not files)
- **`internal/*` package convention** — every domain has its own package (`api`, `session`, `worktree`, `opencode`, `github`, `reaper`, etc.); MCP belongs in `internal/mcp/`. Main only dispatches.
- **`httptest`-based integration tests** — `internal/api/*_test.go` (e.g. `agent_integration_test.go`) and `internal/session/*_test.go` set up real `httptest.Server`s and exercise real HTTP roundtrips; the `internal/mcp/` bridge integration test should follow this pattern.
- **`slog` structured logging** — every package uses `log/slog` with structured fields. The MCP subcommand logs to **stderr** (slog default), never stdout.
- **Single-writer SQLite discipline** (`internal/store/store.go`) — NOT relevant to Phase 06 (no DB changes), but the bridge code must not import `internal/store` directly; it goes through the HTTP API like every other external client.

No external ADRs/specs — requirements are fully captured in the decisions above plus these in-repo references.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`cmd/kamacu/main.go`** — the existing flag-parsing + server-bootstrap code becomes the `serve` `Command`'s `Execute` body verbatim (with the `flag.X` calls moved into a `SetFlags` method on the Command). The backfill one-shots, reaper goroutine, orphan sweep, `hostCheck`, `ensureLoopback`, and `hookBaseURL` helpers all stay where they are.
- **`subcommands.Execute` pattern** — `github.com/google/subcommands` is stdlib-adjacent (it lives under `github.com/google/`), zero-dep, and the idiomatic Go pattern for multi-subcommand CLIs without adopting a full framework like cobra. Phase 06 is the first subcommand refactor; future subcommands slot in cleanly.
- **Existing `httptest` integration test pattern** (`internal/api/agent_integration_test.go:48-73`) — registers Routes + SessionRoutes + HookRoutes on a test mux, spins up `httptest.NewServer`, and exercises real HTTP roundtrips. The `internal/mcp/` bridge integration test follows this shape: stand up the Kamacu routes on a test server, point the MCP bridge at it, exercise the tool handler.
- **`internal/api/projects.go` `projectHandlers.list`** — the existing endpoint the proof tool calls. Returns JSON projects array; the MCP bridge passes it through as MCP tool output content.
- **`log/slog` to stderr by default** — Go stdlib's `slog.NewTextHandler(os.Stderr, ...)` is the default; verify in research that nothing in the subcommand's init path redirectss slog to stdout.

### Established Patterns
- **Per-instance hook token** — generated fresh in `cmd/kamacu/main.go` at every server start (line ~225, `tokenBytes := make([]byte, 32); rand.Read(...); hookToken := hex.EncodeToString(...)`), held in memory only, never persisted. Embedded in spawned agent env via `mgr.SetAgentConfig(... { Token: hookToken })`. The MCP subcommand inherits it via env (the agent CLI's child process) — D-06/D-07 ride this existing envelope.
- **Constant-time token compare** (`crypto/subtle.ConstantTimeCompare` in `internal/api/hooks.go:40`) — the rename in D-07 preserves this exactly; only the header string changes.
- **`internal/*` package boundary** — main is a thin dispatcher; real logic lives in versioned packages. The MCP subcommand body in `internal/mcp/` is testable in isolation without building the binary.
- **`KAMACU_*` env var convention** — `KAMACU_SESSION_ID`, `KAMACU_HOOK_TOKEN`, `KAMACU_HOOK_BASE` are the established names (v1.10 Phase 03 D014). Phase 06 reads them verbatim; no new env vars.
- **Loopback-only binding** — the security posture since v1.0; `ensureLoopback` + `hostCheck` + WS Origin allowlist form the auth boundary. D-06 says Phase 06 does not extend token auth onto `/api/*`; loopback remains it.

### Integration Points
- **New `internal/mcp/` package** — at minimum an `mcp.Serve(args []string, env []string) int` entry point (or similar — researcher picks the exact signature) that the `mcp serve` `Command` calls. Holds: env parsing (`KAMACU_HOOK_TOKEN`, `KAMACU_HOOK_BASE`), HTTP client construction, MCP SDK init, tool registration (just `list_projects` for now), stdio loop.
- **`cmd/kamacu/main.go`** — refactored to register `serve` and `mcp serve` `Command`s and call `subcommands.Execute(...)`. Today's flag definitions move into the `serve` Command's `SetFlags`. The whole HTTP-server body moves into `serve.Execute`.
- **Coordinated rename** — `X-Kangent-Token` → `X-Kamacu-Token` across Go (3 files: `hooks.go`, `agent.go`, plus tests), JS (1 file: `kamacu-status.js`, plus tests), and Go tests (3 files). The `opencode.InstallPlugin` regeneration path picks up the JS rename on next boot.
- **`go.mod`** — adds `github.com/modelcontextprotocol/go-sdk v1.6.1` and `github.com/google/subcommands` (latest). Run `go mod tidy`.
- **README / docs / scripts** — every "run kamacu" reference becomes "run `kamacu serve`" (D-04). Search the repo for bare `kamacu` invocations in examples and update.
</code_context>

<specifics>
## Specific Ideas

- **The "break clean" posture is intentional and runs through D-03, D-04, D-07, D-12.** The user actively preferred honest breaking changes over transitional shims: no `kamacu`-bare-runs-server shim (D-04), no `X-Kangent-Token` fallback header (D-07), no half-renamed state. Single-user local app, fresh-per-start token, server+agents always same-release → transitional code is unnecessary debt.
- **The proof tool is a real tool, not a sketch.** D-09 is important: Phase 06's `list_projects` is the production tool that Phase 07 inherits unchanged. The work product of Phase 06 is the **bridge pattern** (env → client → endpoint → MCP output), not a throwaway demo. Phase 07 repeats the pattern 12+ times.
- **The MCP subcommand runs OUTSIDE the Kamacu server process.** It's a separate process spawned by the agent CLI (claude/opencode) inside a Kamacu task PTY. The Kamacu server (`kamacu serve`) is unaware of any MCP subcommand's existence; it just sees HTTP requests from loopback with an `X-Kamacu-Token` header. This is why D-06 (loopback-only enforcement) is coherent — the subcommand is just another loopback HTTP client.
- **Stdout discipline is the SC2 invariant.** MCP stdio transport REQUIRES stdout contain only JSON-RPC messages. The single biggest risk in Phase 06 is some stray `fmt.Println` in a shared init path polluting stdout. D-12 keeps the defense light (stdlib default + code review) but the researcher should explicitly verify the SDK's stdio initialization doesn't fight any existing init code that the subcommand triggers. (The subcommand's init path is much smaller than `kamacu serve`'s — no DB, no migrations, no reaper — so the surface is small.)
- **No new long-lived goroutines inside the Kamacu binary** (per PROJECT.md). The MCP subcommand is a SEPARATE PROCESS — this constraint is naturally satisfied. Don't add an in-process MCP server to the Kamacu binary; that's not what the milestone is.
</specifics>

<deferred>
## Deferred Ideas

- **MCPAUTO-01..03** — per-task auto-scoping (`get_my_task`, `get_my_session`, `ToolFilter` rewriting the visible tool list per session). Useful DX; agents pass explicit IDs in v1.11. Deferred to v1.12.
- **MCPREG-01..03** — auto-registration of `kamacu mcp serve` with agent CLIs at spawn time (write per-task `.mcp.json` for Claude Code; update per-task `opencode.json` `mcp` key for opencode). v1.11 users manually add it once. Deferred to v1.12.
- **MCPMORE-01..03** — additional tool categories (Agents CRUD, Settings get/update, Worktree cleanup ops). Out of scope for v1.11 to keep the tool surface tight. Deferred to v1.12.
- **MCPHARD-01** — typed JSON-RPC error taxonomy for the 6 bridge failure modes (`kamacu_down` / `port_mismatch` / `token_rejected` / `route_404` / `timeout` / `too_large`). Phase 06 surfaces generic MCP errors; the typed taxonomy is v1.12 work.
- **MCPHARD-02** — full stdout-pollution guards (`os.Stdout` redirect from line zero, ban `fmt.Print*` from `internal/mcp/` with CI grep check). Phase 06 uses stdlib defaults + code review only (D-12). Full version is v1.12 work.
- **MCPHARD-03** — real-binary e2e harness (`//go:build mcp_e2e`) — smallest real Claude Code + real opencode invocation that exercises `kamacu mcp serve` end-to-end. Phase 06 ships only the unit test (D-11). Full e2e is v1.12 work.
- **`list_tasks` unscoped** — adding `GET /api/tasks` (unscoped) or making `list_tasks`'s `project_id` required in MCP is Phase 07's call. Phase 06 uses `list_projects` precisely to avoid this question.
- **Workspace filter on `list_projects`** — the Kamacu `GET /api/projects` endpoint doesn't currently filter by `workspace_id` at the handler level. Phase 07 may want it; Phase 06 returns all projects.
- **Token auth on general `/api/*` routes** — server-side middleware validating `X-Kamacu-Token` on all `/api/*` calls. Explicitly NOT in v1.11 (D-06). Loopback binding is the boundary.
- **External-editor MCP server** (Claude Desktop / Cursor / VS Code) — different transport / auth / process model. Out of scope for v1.11 entirely; a separate future milestone.

None of these were pulled into Phase 06 — discussion stayed within the MCPPROC-01/02/03 boundary.
</deferred>

---

*Phase: 06-mcp-subcommand-foundation*
*Context gathered: 2026-07-21*
