# Architecture Research

**Domain:** MCP (Model Context Protocol) server capability added to an existing local-only Go single-binary app — a stdio MCP subcommand that bridges to the app's existing HTTP API, plus the spawn-time integration that auto-registers it with the agent CLIs Kamacu spawns.
**Researched:** 2026-07-21
**Confidence:** HIGH — every claim below is grounded in: (a) the live Kamacu codebase as it stands at v1.10 (every file/function name in the integration map was read in the actual repo); (b) the official `modelcontextprotocol/go-sdk` v1.6.1 on pkg.go.dev (hand-tested API shapes); (c) the MCP spec pages at `modelcontextprotocol.io/specification/2025-11-25` (read in full for cancellation, tools, lifecycle); (d) the live Claude Code MCP docs at `docs.anthropic.com/en/docs/claude-code/mcp` and the opencode config docs at `opencode.ai/docs/config/` (both fetched 2026-07-21). Aligned with STACK.md (official SDK pick) and FEATURES.md (tool surface).

---

## TL;DR for the roadmap author

- **The architecture is overwhelmingly additive.** One new subcommand wired through stdlib subcommand dispatch, two new leaf packages (`internal/mcp` for the server, `internal/mcpconfig` for the spawn-time config writers), two new HTTP endpoints on the existing Kamacu server (`/api/sessions/{id}/snapshot`, `/api/sessions/{id}/tail`) to expose the existing ring buffer read-only to the bridge. No new long-running goroutines inside the Kamacu server binary. No migrations. No DB schema changes. No frontend changes. The v1.10 spawn engine gets one new step between worktree creation and PTY start: write `.mcp.json` (Claude) or `opencode.json` (opencode) into the worktree root.
- **The subcommand is a child of the agent CLI, not of the Kamacu binary.** This is the central invariant. The agent CLI (`claude` or `opencode`) is the MCP *client*; `kamacu mcp serve` is the MCP *server* it spawns. The MCP subcommand then makes HTTP calls to the long-running Kamacu binary at `127.0.0.1:7333` — it is a thin stdio→HTTP *bridge*. This separates lifecycle concerns: the MCP subcommand lives and dies with the agent CLI; the Kamacu binary can restart independently.
- **Per-task scoping comes from env inheritance, not config files.** Each Kamacu task spawn already injects `KAMACU_SESSION_ID` + `KAMACU_HOOK_TOKEN` + `KAMACU_HOOK_BASE` into the agent CLI's env (v1.10 opencode path; claude path uses the `--settings` overlay but the env is inherit-all). The agent CLI passes those through to the MCP subcommand it spawns. The subcommand reads them once at startup, resolves `KAMACU_SESSION_ID` → `task_id` via one HTTP GET, and every "my_*" tool reuses the result. No per-task MCP config file content — the *same* `.mcp.json`/`opencode.json` template works for every task because per-task identity rides on env.
- **Cancellation works for free.** The official Go SDK's tool handler signature is `func(ctx context.Context, req *mcp.CallToolRequest, in Input) (...)`. When the agent CLI sends `notifications/cancelled`, the SDK cancels the context — the handler's `ctx.Done()` fires. For the long-running `subscribe_session_output` tool, this means: block on a `select { case <-ctx.Done(): ... case chunk := <-tailCh: ... }` loop, and cancellation just works. No bespoke plumbing.
- **Two config-file shapes; one writer per engine.** Claude Code: `~/.claude.json` style would be global, but writing per-worktree `.mcp.json` at the worktree root is the **project-scoped** config Claude Code documents and prefers — each task gets exactly the kamacu entry it needs and nothing pollutes the user's global config. opencode: same idea, `opencode.json` at the worktree root under the `mcp` key. Getting either shape wrong (`mcpServers` vs `mcp`, `command: "bin"` + `args: [...]` vs `command: [bin, ...]`, `env` vs `environment`, `"stdio"` vs `"local"`) = silent tool discovery failure. The writer has to be engine-aware.
- **Build order is dictated by three dependencies.** (1) The subcommand must exist and speak the protocol before config injection is useful. (2) Config injection must work before any tool that relies on `KAMACU_SESSION_ID` inheritance. (3) The two new HTTP read endpoints must exist before the snapshot/subscribe tools can call them. So the minimal vertical slice is: subcommand skeleton + ONE read-only tool (`get_my_task`) → agent-CLI config writer for ONE engine (Claude) → end-to-end "claude inside kamacu can call a tool" demo. Everything else fans out from that.

---

## Standard Architecture

### System Overview

```
                              ┌─────────────────────────────┐
                              │        Host filesystem       │
                              │  (~.kamacu, worktrees, etc.) │
                              └─────────────────────────────┘
                                          ▲
                                          │
        ┌─────────────────────────────────┼─────────────────────────────────┐
        │                                 │                                 │
        │      KAMACU BINARY              │        AGENT CLI                 │
        │      (long-lived, v1.10)        │        (claude / opencode)       │
        │                                 │        child of kamacu,          │
        │  ┌────────────────────┐         │        short-lived per task      │
        │  │ HTTP + WS server   │         │                                 │
        │  │ 127.0.0.1:7333     │         │   ┌──────────────────────┐      │
        │  │                    │         │   │ PTY + TUI             │      │
        │  │ Routes:            │         │   │ (the agent proper)    │      │
        │  │  /api/projects     │         │   └──────────┬───────────┘      │
        │  │  /api/tasks        │         │              │ spawns            │
        │  │  /api/sessions     │         │              ▼                   │
        │  │  /api/sessions/{id}/ws ◄─────┼─ WS attach  ┌──────────────────┐ │
        │  │  /api/sessions/{id}/snapshot │   (browser)│ MCP CLIENT       │ │
        │  │  /api/sessions/{id}/tail   ◄─┼─────────── │ in agent runtime │ │
        │  │  /api/hooks/sessions/{id}  ◄─┼── hooks ───┤                  │ │
        │  │  /api/agents        │       │            │ spawns (stdio)   │ │
        │  │  /api/worktrees     │       │            ▼                   │ │
        │  │  ... (rest)         │       │   ┌──────────────────────┐     │ │
        │  │                    │       │   │ kamacu mcp serve     │     │ │
        │  │ SessionManager     │       │   │ (MCP SERVER)         │     │ │
        │  │  └ PTY children    │       │   │                      │     │ │
        │  │  └ ring buffers    │       │   │ Reads env:           │     │ │
        │  │ Reaper goroutine   │       │   │  KAMACU_SESSION_ID   │     │ │
        │  └─────────┬──────────┘       │   │  KAMACU_HOOK_TOKEN   │     │ │
        │            │                  │   │  KAMACU_HOOK_BASE    │     │ │
        │            │                  │   │                      │     │ │
        │            ▼                  │   │ Bridges → HTTP calls │     │ │
        │  ┌────────────────────┐       │   │ to 127.0.0.1:7333    │     │ │
        │  │ SQLite (~.kamacu)   │      │   │ (X-Kamacu-Token hdr) │     │ │
        │  │ tasks, projects,    │ ◄────┼───│                      │     │ │
        │  │ sessions, ...       │      │   │ Logs → stderr only   │     │ │
        │  └────────────────────┘       │   │ (stdout = MCP wire)  │     │ │
        │            ▲                  │   └──────────────────────┘     │ │
        │            │                  │                                 │ │
        │            │ KAMACU_SESSION_ID│                                 │ │
        │            │ KAMACU_HOOK_TOKEN│  (inherited env at spawn)        │ │
        │            │ KAMACU_HOOK_BASE │                                 │ │
        │            └──────────────────┼─── spawn injects ───────────────┘ │
        │                               │                                   │ │
        │  ┌────────────────────┐       │                                   │
        │  │ spawn engine       │       │   ALSO at spawn:                  │
        │  │ (v1.10 + 1 new     │       │   writes .mcp.json (claude)       │
        │  │  step: mcpconfig)  │       │   or opencode.json (opencode)     │
        │  └────────────────────┘       │   into the worktree root          │
        │                               │                                   │
        └───────────────────────────────┼───────────────────────────────────┘
                                         │
                                  ┌──────┴──────┐
                                  │  Browser    │
                                  │  (the user) │
                                  └─────────────┘
```

Three processes are involved per task:

1. **The Kamacu binary** (`kamacu` proper) — the long-lived HTTP/WS/PTY server. Started once by the user. Owns the DB, SessionManager, reaper, and the SPA. **Already exists unchanged from v1.10** plus two new read endpoints.
2. **The agent CLI** (`claude` or `opencode`) — spawned per task by Kamacu's spawn engine inside the task's worktree PTY. Owns the TUI the browser renders. **Already exists unchanged from v1.10.**
3. **The MCP subcommand** (`kamacu mcp serve`) — spawned by the agent CLI as a child process at agent startup, after the agent CLI reads the worktree-root `.mcp.json` or `opencode.json` Kamacu wrote at task-spawn time. **New in v1.11.**

The three-process model is forced by MCP semantics: a stdio MCP server is a process whose stdin/stdout the client owns. The agent CLI is the MCP client; Kamacu's `kamacu mcp serve` is the MCP server. Kamacu-the-binary is the *upstream HTTP API* the server bridges to. The subcommand cannot live inside the Kamacu binary (would require HTTP/SSE transport, which is the wrong fit and explicitly out of scope per PROJECT.md) and cannot be replaced by the agent CLI talking HTTP directly (the agent would have to discover and re-implement every endpoint — MCP exists precisely to avoid that).

### Component Responsibilities

| Component | Responsibility | Typical Implementation |
|-----------|----------------|------------------------|
| **`kamacu mcp serve` subcommand** | Be a stdio MCP server that translates MCP `tools/call` requests into HTTP calls against the running Kamacu binary. Read envelope env (`KAMACU_*`) once at startup; build one `*http.Client`; register tools; `mcpSrv.Run(ctx, &mcp.StdioTransport{})`. | New `internal/mcp` package. Stdlib subcommand dispatch in `cmd/kamacu/main.go` (no cobra). Uses `github.com/modelcontextprotocol/go-sdk/mcp` v1.6.1 (per STACK.md). |
| **MCP tool handlers** | Per-tool: validate input, build an `http.Request`, attach `X-Kamacu-Token`, fire it at the right endpoint, translate the response into a `*mcp.CallToolResult`. No business logic. | One Go func per tool, registered via `mcp.AddTool(server, tool, handler)`. Typed-struct handlers so JSON Schemas are auto-derived. |
| **Per-task scope resolver** | Map the inherited `KAMACU_SESSION_ID` to a `task_id` (one HTTP GET on startup), cache it in the `Server` struct. | One method on the MCP `Server`: `sessionProjectID(ctx) (string, error)`. Resolves via `GET /api/sessions/{KAMACU_SESSION_ID}` → `taskID` → `GET /api/tasks/{id}` → `projectID`. The first call's result is cached for the subcommand's lifetime; the task's project cannot change while the session is running. |
| **Spawn-time MCP config writer** | At task spawn, after worktree creation, before the agent CLI starts: write the correct MCP config file into the worktree root for the configured agent's engine. | New `internal/mcpconfig` leaf package. Two writers: `WriteClaude(worktreeDir, env)` writes `.mcp.json`; `WriteOpenCode(worktreeDir, env)` writes `opencode.json`. Called from the existing spawn handler. |
| **New Kamacu HTTP endpoints (terminal reads)** | Expose the existing Session ring buffer read-only for the MCP bridge to consume. Two endpoints: snapshot (full ring) + tail (incremental from offset). | New file under `internal/api/` (e.g. `terminal_reads.go`); route registration alongside the existing `SessionRoutes`. Reuse `mgr.Get(id).Snapshot()` and add an offset-aware variant. Auth: existing `X-Kamacu-Token` envelope. |
| **Existing Kamacu binary** | Continue serving HTTP + WS + SPA, owning PTYs, reaping, hooks. Unchanged surface; the two new endpoints above are additive. | No change to `cmd/kamacu/main.go` startup except the route registration call (and the new subcommand dispatch). |

---

## Recommended Project Structure

```
cmd/kamacu/
├── main.go                      # MODIFIED: subcommand dispatch (stdlib)
├── main_test.go                 # existing
└── mcp_subcommand.go            # NEW: the `mcp serve` dispatch entry (calls internal/mcp)

internal/
├── api/                         # existing package, extended
│   ├── routes.go                # existing
│   ├── sessions.go              # existing
│   ├── terminal_reads.go        # NEW: GET /api/sessions/{id}/snapshot, /tail
│   └── terminal_reads_test.go   # NEW
├── mcp/                         # NEW PACKAGE: the MCP server
│   ├── server.go                # mcp.NewServer + tool registration + StdioTransport.Run
│   ├── server_test.go           # in-memory transport tests (mcp.NewInMemoryTransport)
│   ├── bridge.go                # http.Client wrapper: getJSON/postJSON/doRequest with auth header
│   ├── bridge_test.go           # round-trip tests against httptest.NewServer
│   ├── scope.go                 # KAMACU_SESSION_ID → task_id/project_id resolution + caching
│   ├── scope_test.go
│   ├── tools.go                 # tool registration glue (one func per tool group)
│   ├── tools_tasks.go           # list_tasks, get_task, create_task, move_task, ...
│   ├── tools_sessions.go        # list_sessions, get_session, snapshot_session, subscribe_session
│   ├── tools_projects.go        # list_projects, get_project, list_workspaces, list_agents
│   ├── tools_terminal.go        # get_session_output (snapshot) + subscribe_session_output (long-running)
│   ├── tools_github.go          # list_pr_reviews, get_pr_review
│   ├── tools_diff.go            # get_task_diff, mark_file_viewed
│   ├── tools_worktree.go        # list_worktree_cleanup_candidates, clean_worktree_eligible, remove_worktree
│   └── tools_test.go            # per-tool handler tests (bridge mocked)
├── mcpconfig/                   # NEW LEAF PACKAGE: spawn-time config writers
│   ├── claude.go                # WriteClaude(worktreeDir, env) → .mcp.json
│   ├── opencode.go              # WriteOpenCode(worktreeDir, env) → opencode.json
│   ├── claude_test.go
│   ├── opencode_test.go
│   └── doc.go                   # package-level docs
├── session/                     # existing — minor additive change
│   └── session.go               # already has Snapshot(); add Tail(since int) if needed
└── ... (existing packages unchanged)
```

### Structure Rationale

- **`internal/mcp/` (NEW)** mirrors `internal/api/` (REST handlers) and `internal/ws/` (WebSocket handlers): each is one network-facing translation layer over the same `internal/session` + DB substrate. Naming it `mcp` (not `mcpserver`, not `mcpbridge`) follows the existing one-word convention (`api`, `ws`, `diff`, `quota`, `tmux`, `reaper`). Dependency direction is enforced: `internal/mcp` imports `internal/api` types where they exist (the request/response structs) so wire shapes stay in one place, but it does NOT import `internal/session` — terminal reads go through HTTP, not direct Session access, keeping the subcommand a clean bridge.
- **`internal/mcpconfig/` (NEW LEAF)** mirrors `internal/tmux` (CLI shell-out) and `internal/opencode` (plugin install): each is a leaf that owns one side effect at the OS boundary. Splitting it from `internal/mcp` is deliberate — `mcpconfig` runs *inside the Kamacu binary at task spawn* and writes files; `mcp` runs *inside the subcommand* and speaks the protocol. They share no code; conflating them would create an import cycle (the spawn engine shouldn't depend on the MCP SDK).
- **Tool handlers split by domain** (`tools_tasks.go`, `tools_sessions.go`, etc.) — one file per Kamacu resource area, mirroring how `internal/api/` is split (`tasks.go`, `sessions.go`, `agents.go`, `workspaces.go`). Easier to navigate; smaller diffs.
- **`cmd/kamacu/main.go` gets a single new function** (`runMCPSubcommand`) and a dispatch branch at the top. The subcommand body lives in `internal/mcp` so it's testable without spawning a process.

---

## Architectural Patterns

### Pattern 1: Stdlib subcommand dispatch (no framework)

**What:** The Kamacu binary serves two modes from one binary. `kamacu [global flags]` runs the HTTP server (default, unchanged); `kamacu mcp serve` runs the MCP subcommand. Use stdlib `flag.FlagSet`, not cobra/spf13.

**When to use:** Whenever a binary needs two modes and Kamacu's ethos is stdlib-only.

**Trade-offs:** Slightly more code than cobra (~30 LOC vs ~5 LOC for the dispatch), but zero new dependencies, matches the existing `flag` style in `main.go` (which already uses `flag.String` for `--addr`, `--db`, etc.), and avoids pulling cobra's transitive deps. Cobra is justified if subcommands proliferate (5+) or have nested subcommands; Kamacu has exactly one.

**Example:**
```go
// cmd/kamacu/main.go
func main() {
    if len(os.Args) >= 2 && os.Args[1] == "mcp" {
        // Subcommand mode: kamacu mcp [serve]
        os.Exit(runMCPSubcommand(os.Args[2:]))
    }
    // Default mode: kamacu (HTTP server) — existing flag.Parse() block unchanged
    addr := flag.String("addr", "127.0.0.1:7333", "...")
    // ... rest of existing main ...
}

// cmd/kamacu/mcp_subcommand.go
func runMCPSubcommand(args []string) int {
    fs := flag.NewFlagSet("kamacu mcp", flag.ExitOnError)
    // No flags today; room for --transport http later (future milestone)
    fs.Usage = func() {
        fmt.Fprintln(os.Stderr, "Usage: kamacu mcp serve")
        fmt.Fprintln(os.Stderr, "  Runs the Kamacu MCP server over stdio (JSON-RPC).")
    }
    if err := fs.Parse(args); err != nil {
        return 2
    }
    if fs.NArg() == 0 || fs.Arg(0) != "serve" {
        fs.Usage()
        return 2
    }
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer cancel()
    if err := mcp.Run(ctx, mcp.OptionsFromEnv()); err != nil {
        slog.Error("mcp server stopped", "error", err)
        return 1
    }
    return 0
}
```

Two-level dispatch (`mcp` then `serve`) leaves room for `kamacu mcp list-tools` or `kamacu mcp inspect` (debug subcommands) without further restructuring. If v2 ever adds more subcommands (`kamacu config`, `kamacu doctor`), the same pattern extends.

### Pattern 2: HTTP-bridge, not in-process

**What:** The MCP subcommand talks to the running Kamacu binary exclusively over HTTP at `127.0.0.1:7333`. It does NOT import `internal/session`, `internal/store`, or any internal package directly. Auth is the existing `X-Kamacu-Token` envelope header.

**When to use:** Always, for v1.11. The alternative — importing internal packages and running the MCP server inside the Kamacu binary — would require HTTP/SSE transport (out of scope) and entangle MCP request lifetimes with the long-lived server's goroutines.

**Trade-offs:**
- **Pros:** Clean separation; existing validation/side effects/reaper/hooks all apply unchanged; existing tests cover the data path; the bridge is trivially substitutable (swap `http.DefaultClient` for a fake in tests); the subcommand can be developed and tested in complete isolation from Kamacu's binary.
- **Cons:** One extra TCP round-trip per tool call (~1ms on localhost — irrelevant); the bridge duplicates the request/response struct definitions (mitigated by sharing `internal/api/types.go` where they exist).

The "cons" list is short and minor. The bridge pattern is the documented happy path of the official Go SDK — its `examples/server/proxy/main.go` is structurally identical (stdio MCP→HTTP upstream). Kamacu's only adaptation is that the upstream is a REST API rather than another MCP server, which makes each handler *simpler* than the proxy example (no MCP-version negotiation with the upstream).

**Example:**
```go
// internal/mcp/bridge.go
type Bridge struct {
    client  *http.Client
    baseURL string                    // http://127.0.0.1:7333
    token   string                    // KAMACU_HOOK_TOKEN
}

func (b *Bridge) getJSON(ctx context.Context, path string, out any) error {
    req, err := http.NewRequestWithContext(ctx, "GET", b.baseURL+path, nil)
    if err != nil { return err }
    req.Header.Set("X-Kamacu-Token", b.token)   // existing envelope
    req.Header.Set("Accept", "application/json")
    res, err := b.client.Do(req)
    if err != nil { return fmt.Errorf("kamacu bridge: %w", err) }
    defer res.Body.Close()
    if res.StatusCode >= 400 {
        return &bridgeError{Status: res.StatusCode, Path: path}
    }
    if out == nil { return nil }
    return json.NewDecoder(res.Body).Decode(out)
}
```

### Pattern 3: Per-task scoping via env + lazy resolution

**What:** Convenience tools (`get_my_task`, `list_my_tasks`, `subscribe_my_session_output`) need no parameters — they operate on the inherited `KAMACU_SESSION_ID`. The subcommand resolves it to a `task_id` lazily on first use, caches the result on the `Server` struct.

**When to use:** Whenever the subcommand has `KAMACU_SESSION_ID` in env (i.e., always — Kamacu injects it for every spawned agent).

**Trade-offs:**
- **Pros:** Zero per-tool-call overhead after the first resolution; clear separation of convenience tools (no params) and cross-task tools (explicit IDs); no SDK feature needed (plain Go struct field).
- **Cons:** If the task is reassigned to another project mid-session (impossible today but a future concern), the cache is stale. Mitigation: cache for the subcommand's lifetime — a single MCP subcommand lives for one agent-CLI session, which is shorter than any conceivable reassignment flow.

**Example:**
```go
// internal/mcp/scope.go
type Scope struct {
    bridge    *Bridge
    sessionID string                  // KAMACU_SESSION_ID
    cached    scopeCache
}

type scopeCache struct {
    taskID    int64
    projectID int64
    resolved  bool
}

// taskID resolves KAMACU_SESSION_ID -> tasks.id via one GET. Cached.
func (s *Scope) taskID(ctx context.Context) (int64, error) {
    if s.cached.resolved { return s.cached.taskID, nil }
    var info session.Info
    if err := s.bridge.getJSON(ctx, "/api/sessions/"+s.sessionID, &info); err != nil {
        return 0, err
    }
    if info.TaskID == 0 {
        return 0, errUnscopedSession    // dev session, not a task
    }
    s.cached = scopeCache{taskID: info.TaskID, projectID: 0, resolved: true}
    return s.cached.taskID, nil
}
```

The lookup happens *lazily on first call*, not at startup. This is deliberate: if Kamacu restarts between agent-CLI spawn and the first MCP tool call (rare but possible), the session row may be briefly gone. A lazy resolver surfaces that as a clean MCP error on the failing tool call; an eager one crashes the subcommand at startup.

### Pattern 4: Spawn-time config injection at the worktree root

**What:** At task spawn, after `worktree.Add` and before `mgr.Spawn`, Kamacu writes a `.mcp.json` (Claude Code) or `opencode.json` (opencode) into the worktree root. The file contains exactly one MCP server entry pointing at `kamacu mcp serve`, with env vars carrying the per-task identity.

**When to use:** For every task with `engine=claude` or `engine=opencode`. Custom agents are skipped — they may not speak MCP.

**Trade-offs:**
- **Pros:** Zero global-config pollution — each worktree's MCP entry is scoped to that worktree's agent; committed worktrees (rare but possible) don't leak credentials across projects; Claude Code's `--scope project` is the documented pattern for team-shareable MCP server registrations; opencode's per-project `opencode.json` is the highest-precedence standard config layer.
- **Cons:** Two config shapes to maintain (Claude vs opencode — see table below); the agent CLI must be configured to look at the worktree-root config (Claude Code does this by default for `.mcp.json`; opencode does this by default for `opencode.json`); removing a task removes its worktree, which auto-cleans the config file — no separate GC needed.

**The two shapes (critical to get right):**

| Aspect | Claude Code `.mcp.json` | opencode `opencode.json` |
|--------|-------------------------|---------------------------|
| Top-level key | `mcpServers` | `mcp` |
| Server entry discriminator | `"type": "stdio"` (optional, default) | `"type": "local"` (required) |
| Command shape | `"command": "kamacu"` + `"args": ["mcp", "serve"]` | `"command": ["kamacu", "mcp", "serve"]` (single array) |
| Env key | `"env": {...}` | `"environment": {...}` |
| Env-value expansion | `${VAR}` and `${VAR:-default}` | `{env:VAR_NAME}` |
| Approval | First-use prompt per project (project scope) | None — trusted from config file |
| Per-project file location | `<worktree>/.mcp.json` | `<worktree>/opencode.json` |

**Example (Claude):**
```json
{
  "mcpServers": {
    "kamacu": {
      "type": "stdio",
      "command": "kamacu",
      "args": ["mcp", "serve"],
      "env": {
        "KAMACU_HOOK_TOKEN": "abc123...",
        "KAMACU_SESSION_ID": "550e8400-e29b-41d4-a716-446655440000",
        "KAMACU_HOOK_BASE": "http://127.0.0.1:7333"
      }
    }
  }
}
```

**Example (opencode):**
```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "kamacu": {
      "type": "local",
      "command": ["kamacu", "mcp", "serve"],
      "environment": {
        "KAMACU_HOOK_TOKEN": "abc123...",
        "KAMACU_SESSION_ID": "550e8400-e29b-41d4-a716-446655440000",
        "KAMACU_HOOK_BASE": "http://127.0.0.1:7333"
      },
      "enabled": true
    }
  }
}
```

Kamacu writes the literal env values (not `${VAR}` placeholders) because the values are already per-process secret per the existing v1.10 injection posture — agent-CLI env expansion would just add a step that reads from a different env. Note: `kamacu mcp serve` resolves its own absolute path (or relies on PATH lookup by the agent CLI) — the writer should use `os.Executable()` to get the absolute path of the currently-running `kamacu` binary so the spawned subcommand is unambiguous regardless of the agent CLI's PATH.

### Pattern 5: Long-running tool with cancellation + progress

**What:** The `subscribe_session_output` tool is the only long-running tool in v1.11. Its handler blocks for up to `duration_seconds` (default 30, max 300) collecting output chunks via the existing ring buffer + a short-lived WS attach. On agent cancellation (`notifications/cancelled`), it returns immediately with whatever was collected so far.

**When to use:** Whenever the agent wants a "watch this session for ~30s and tell me what happened" affordance. Single-shot snapshots use `get_session_output` instead.

**Trade-offs:**
- **Pros:** Reuses the existing attach/ring-buffer machinery (no new fan-out path); cancellation works for free via the SDK's context propagation; progress notifications keep the agent-CLI's idle timer from firing (Claude Code's stdio idle default is 30 min; opencode's similar — plenty of headroom); bounded duration means the tool always returns — agents handle bounded waits well and unbounded subscriptions poorly.
- **Cons:** Two transports in one tool (HTTP for snapshot, WS for live tail) — minor complexity; if the agent-CLI ignores `notifications/progress` (some older versions might), the tool still works but the agent sees no incremental updates until the tool returns.

**Mechanics (textual data flow):**

```
1. Agent calls subscribe_session_output(session_id, duration_seconds=30, lines?)
2. MCP handler enters; ctx from SDK carries cancellation semantics
3. Handler:
   a. GET /api/sessions/{id}/snapshot?lines=N  (HTTP, ring-buffer slice)
   b. Emit initial chunk via req.Session.NotifyProgress(ctx, {message:"snapshot", progress:0, total:estimated_bytes})
   c. Open WS to ws://127.0.0.1:7333/api/sessions/{id}/ws (no input frames — READ-ONLY contract)
   d. Loop:
      select {
      case chunk := <-wsRecvCh:
         collected = append(collected, chunk...)
         req.Session.NotifyProgress(ctx, {message:"tail", progress: bytes_so_far, total: budget_bytes})
      case <-time.After(duration_seconds * time.Second):
         return final_result(collected)
      case <-ctx.Done():
         return final_result(collected)    // cancellation: clean exit, no error
      }
4. Final result: *mcp.CallToolResult with TextContent of the collected bytes (base64 if non-UTF-8).
```

**Critical detail — no PTY writes:** The WS the tool opens is *read-only by contract*. The existing WS handler accepts `FrameData` frames as PTY input — the MCP subcommand MUST NOT send those. Implement this as a `wsReader` wrapper that exposes only the read channel and has no public Write method. (Alternatively, the new `/api/sessions/{id}/tail` HTTP endpoint can be a polled fallback that doesn't even open a WS — simpler, no input risk at all. Recommend the HTTP tail endpoint as the primary path, with the WS as a future optimisation.)

**Cancellation correctness:** The MCP cancellation spec says the receiver SHOULD stop processing and not send a response. The Go SDK's Cancellation example confirms: when the client sends `notifications/cancelled`, the handler's `ctx.Done()` fires. The handler returns `(result, nil)` normally — the SDK suppresses the response for a cancelled request, so the agent sees no spurious reply. Returning the partial result is therefore safe; if the SDK throws away the response, no harm done.

**Max duration:** Hard-cap at 300s (configurable via flag/env). This matches Claude Code's default 30-min stdio idle timeout with ample headroom; tools that run longer should be split into multiple calls. Document this in the tool description so the LLM knows.

---

## Data Flow

### Request Flow: MCP tool call → Kamacu HTTP

```
Agent (LLM)
   │ decides to call a tool
   ▼
Agent CLI (claude/opencode)
   │ tools/call JSON-RPC over stdio
   │ {"method":"tools/call","params":{"name":"list_tasks","arguments":{...}}}
   ▼
kamacu mcp serve (subcommand)
   │ SDK parses request, dispatches to handler by name
   │ handler reads input struct (auto-unmarshalled + JSON-Schema-validated)
   │ handler calls bridge.getJSON(ctx, "/api/projects/123/tasks", &out)
   ▼
Bridge (in subcommand)
   │ http.NewRequestWithContext + X-Kamacu-Token header
   ▼
Kamacu binary (HTTP server, 127.0.0.1:7333)
   │ hostCheck middleware, X-Kamacu-Token validated (existing envelope)
   │ existing GET /api/projects/123/tasks handler
   │ SELECT FROM tasks WHERE project_id=123 ORDER BY position
   ▼
Response flows back:
   Kamacu JSON → Bridge → handler packs into CallToolResult → SDK serialises → stdout
   ▼
Agent CLI receives tool result, hands to LLM
```

### State Management: Per-task scope resolution

```
At task spawn (existing v1.10 + new step):
   session.Manager.Spawn
       ├ injects env (KAMACU_SESSION_ID, KAMACU_HOOK_TOKEN, KAMACU_HOOK_BASE)
       ├ NEW: writes .mcp.json / opencode.json into worktree root
       └ starts agent CLI in worktree PTY

Agent CLI starts:
   ├ reads worktree-root MCP config (one entry: kamacu mcp serve)
   ├ spawns `kamacu mcp serve` as child process
   └ inherits env through to subcommand

kamacu mcp serve starts:
   ├ reads KAMACU_* env once
   ├ builds Bridge{baseURL, token}
   ├ builds Scope{sessionID}
   ├ registers tools
   └ server.Run(ctx, StdioTransport{})  -- blocks here

On first my_* tool call:
   Scope.taskID(ctx)
       ├ GET /api/sessions/{KAMACU_SESSION_ID}
       ├ parse Info.TaskID
       └ cache on Scope struct

Subsequent my_* calls:
   reuse cached taskID, projectID  -- zero HTTP overhead for scoping
```

### Lifecycle: normal exit, kamacu restart, agent-CLI death

| Event | What happens | Detection |
|-------|--------------|-----------|
| **Agent CLI exits normally** | Closes stdin of `kamacu mcp serve`. The SDK's StdioTransport sees EOF on stdin, `server.Run` returns, subcommand exits 0. | Stdin EOF — clean. |
| **Agent CLI crashes / killed** | Same as above from the OS's perspective — closing the parent's stdout pipe to the child's stdin causes an EOF or EIO. The subcommand exits within a few ms. | Stdin EOF or SIGPIPE on stdout write. |
| **`kamacu mcp serve` crashes** | The agent CLI sees the stdio stream close. Most agent CLIs mark the server as failed in their `/mcp` panel and either retry once or stop calling its tools. The Kamacu binary is unaffected. | Tool call returns a transport error; agent CLI handles per its own retry policy. |
| **Kamacu binary restarts** | All in-flight HTTP calls from the subcommand fail with connection-refused. The subcommand's tool handlers return MCP error results (`CallToolResult{IsError:true, ...}` with a clear message). The Kamacu binary comes back; the *next* tool call succeeds. The subcommand itself stays alive — its stdin/stdout are still connected to the agent CLI. | HTTP 5xx or connection-refused → `bridgeError` → tool returns `isError` result. |
| **Kamacu binary dies and stays dead** | Same as above but every subsequent tool call also fails. The agent CLI may eventually give up on the kamacu server (after N consecutive failures). The subcommand stays alive — agent can still see the failed state in `/mcp`. | Repeated bridge errors. |
| **Subcommand outlives agent CLI** (shouldn't happen) | If a bug causes the subcommand to ignore stdin EOF (e.g., a leaked goroutine holding the context), the process could linger. Mitigation: the SDK's `server.Run` returns on stdin EOF; subcommand also installs `signal.NotifyContext(SIGTERM, SIGINT)`. | None needed if SDK behaves; verify with a "kill -9 agent CLI, check no kamacu mcp processes" test. |
| **Kamacu binary outlives subcommand** (normal) | The Kamacu binary has no idea the subcommand existed. It just served HTTP requests. No state to clean up. The task's PTY, ring buffer, etc. are owned by the Kamacu binary and continue independently. | None. |

**Key invariants:**
- The subcommand holds **no Kamacu state** that isn't derivable from env or HTTP. It is a stateless translator.
- The Kamacu binary holds **no per-subcommand state** — it doesn't know which HTTP requests come from the subcommand vs the SPA. (Could add an `X-Kamacu-Source: mcp` header later for observability, but not required.)
- A subcommand outliving its agent CLI by more than seconds is a **bug**. Test explicitly.

### Key Data Flows

1. **Task spawn → MCP auto-discovery:** `session.Manager.Spawn` → worktree creation → `mcpconfig.Write(worktree, engine, env)` → PTY start → agent CLI reads worktree-root config → spawns `kamacu mcp serve` → subcommand resolves `KAMACU_SESSION_ID` → tools available. End-to-end, no user action.
2. **Tool call → Kamacu state change:** `create_task` MCP tool → bridge POST → existing `POST /api/projects/{id}/tasks` handler → `provisionWorktree` runs (worktree + branch auto-created) → task row written → response flows back → MCP tool returns the new task ID. The browser-attached user sees the new card appear via the existing board-poll.
3. **Terminal subscribe (long-running):** `subscribe_session_output` MCP tool → HTTP snapshot → optional WS attach → progress notifications stream chunks → return final result after duration OR cancellation. The browser-attached user is unaffected (the WS attach is read-only; the existing fan-out treats it like any other client).

---

## Scaling Considerations

| Scale | Architecture Adjustments |
|-------|--------------------------|
| **1–5 concurrent task agents** (typical Kamacu load) | One `kamacu mcp serve` process per active agent CLI. Each makes a handful of HTTP calls/min against the Kamacu binary. Negligible load. No adjustment needed. |
| **20–50 concurrent task agents** (heavy user) | ~20–50 subcommand processes, each holding an `*http.Client` (cheap — Go pools connections). Kamacu binary sees ~100–500 HTTP req/min from MCP, indistinguishable from a busy SPA. The Kamacu binary's `hostCheck` middleware and SQLite handle this trivially. Still no adjustment. |
| **100+ concurrent task agents** | Not a v1.11 concern — Kamacu is single-user local. If reached: add request rate-limiting per token, switch the bridge to HTTP keep-alive with a shared `*http.Client` (already does), consider an SSE-broadcast endpoint to replace per-subcommand polling. |

### Scaling Priorities

1. **First bottleneck:** none expected at single-user scale. The Kamacu binary's HTTP server, SQLite, and SessionManager were built for v1.0's full-board-with-10-agents scenario; MCP traffic is a marginal addition.
2. **Second bottleneck (theoretical):** per-subcommand goroutine count. Each `subscribe_session_output` call holds one goroutine for up to 5 minutes. With 50 concurrent subscriptions across 50 agents, that's 50 goroutines — Go handles tens of thousands trivially.

---

## Anti-Patterns

### Anti-Pattern 1: Importing Kamacu internals in the MCP subcommand

**What people do:** `import "kamacu/internal/session"` in the MCP server code so it can call `mgr.Get(id).Snapshot()` directly, "saving an HTTP round-trip."
**Why it's wrong:** (a) Couples the subcommand's lifecycle to the Kamacu binary's in-memory state — if the Kamacu binary restarts, the subcommand's reference is to a *different* process's memory. (b) Requires the subcommand to run in the same process as the Kamacu binary, which means HTTP/SSE transport, which is out of scope. (c) Bypasses all the existing HTTP-layer validation, hooks, and audit surfaces. (d) Creates an import cycle risk between `internal/mcp` and `internal/session`.
**Do this instead:** Bridge over HTTP. One extra millisecond per call is irrelevant on localhost.

### Anti-Pattern 2: Writing the MCP config to the user-global file

**What people do:** Patch `~/.claude.json` or `~/.config/opencode/opencode.json` at task spawn, with the per-task KAMACU_SESSION_ID baked in.
**Why it's wrong:** (a) Pollutes the user's global config with a per-task entry — every time a task is created or deleted, the global file changes. (b) For opencode, the global config is the *second-highest* precedence layer — a per-project file in the worktree root overrides it cleanly, which is what we want. (c) For Claude Code, the *project-scope* `.mcp.json` is the documented "team-shared" mechanism, while the user-global `mcpServers` is for personal cross-project tools. Per-task MCP entries are neither. (d) Concurrent task spawns would race on the global file.
**Do this instead:** Write `.mcp.json` / `opencode.json` at the worktree root. The worktree is per-task; the file is per-task; deleting the worktree (on task delete or PR merge) auto-cleans the config. Zero global pollution.

### Anti-Pattern 3: One config writer for both engines

**What people do:** Write a generic `WriteMCPConfig(worktreeDir, env)` that emits the same JSON for both engines, "because they're both MCP."
**Why it's wrong:** The two config shapes are *subtly* different (table above). Claude Code uses `mcpServers` + `command` + `args` + `env` + optional `type:"stdio"`. opencode uses `mcp` + `command:[array]` + `environment` + required `type:"local"`. Get any one of these wrong and the agent CLI silently fails to discover the server — no error, no tools, just an agent that "doesn't know about Kamacu."
**Do this instead:** Two functions in `internal/mcpconfig`: `WriteClaude` and `WriteOpenCode`. Each emits exactly the right shape. The spawn engine branches on `agent.engine` (the existing v1.10 discriminator).

### Anti-Pattern 4: Long-running tool with no cancellation story

**What people do:** Implement `subscribe_session_output` as an unbounded loop that only returns when the session exits or the agent explicitly sends a "stop" parameter.
**Why it's wrong:** (a) LLMs don't naturally send "stop" parameters — they wait for the tool to return. (b) Claude Code's stdio idle timeout (30 min) would eventually kill the call, but that's a poor experience. (c) The agent can't decide "I have enough output, let me move on" — it's stuck.
**Do this instead:** Always include a `duration_seconds` parameter (default 30, max 300). Use the SDK's `ctx.Done()` channel for cancellation — when the agent CLI sends `notifications/cancelled`, the context cancels and the handler returns immediately. Emit `notifications/progress` regularly so the agent sees incremental output and can decide to cancel when satisfied.

### Anti-Pattern 5: PTY write from the MCP bridge

**What people do:** Add a `send_input_to_session` tool that calls the existing WS `FrameData` path "because the agent asked nicely."
**Why it's wrong:** Explicitly out of scope per PROJECT.md. An agent observing a sibling session must never inject bytes into its PTY — it conflicts with the browser-attached user (the user's keystrokes would race the agent's) and with the other agent's intent (the other agent didn't consent to being driven). Read + subscribe is the contract.
**Do this instead:** Read-only tools only. If an agent needs to *drive* another session, the user can open that task's view in the browser and type themselves. (Future milestone could add a *user-initiated* "inject prompt" affordance with explicit UI confirmation; that's not the MCP bridge's job.)

### Anti-Pattern 6: Hand-rolling JSON-RPC

**What people do:** "The MCP wire protocol is just newline-delimited JSON-RPC 2.0 — I'll write the 300 LOC myself and skip the SDK dependency."
**Why it's wrong:** (a) The spec is mid-rewrite (2026-07-28 is a near-complete redesign — stateless model, `server/discover` replacing `initialize`, `subscriptions/listen` stream, MRTR for elicitation). The SDK absorbs that churn; a hand-rolled server would re-implement it for zero benefit. (b) Edge cases are easy to get wrong: JSON-RPC error codes, cancellation propagation, progress tokens, batched requests, partial frames on stdin. (c) No conformance tests. (d) The SDK's transitive deps are all pure Go (no CGO) — the "zero deps" benefit is marginal.
**Do this instead:** Use `github.com/modelcontextprotocol/go-sdk` v1.6.1 (per STACK.md).

### Anti-Pattern 7: Mounting MCP inside the Kamacu HTTP server

**What people do:** Run the MCP server on a goroutine inside `cmd/kamacu/main.go`, exposed via the SDK's `StreamableHTTPHandler` on `/mcp`.
**Why it's wrong:** Out of scope per PROJECT.md ("MCP server for external AI editors" is a future milestone). Stdio is the right transport for agents spawned inside Kamacu — they're already subprocesses. An HTTP listener adds auth surface (DNS-rebinding protection, Origin checks, OAuth) that the envelope-token auth avoids. The Kamacu binary would also need to know which agent CLI is calling, breaking the stateless-bridge invariant.
**Do this instead:** `kamacu mcp serve` as a dedicated subcommand. If a future milestone wants external-editor MCP, add `--transport http` then — the handler registrations don't change.

---

## Integration Points

### New files

| File | Purpose | Lines (est.) |
|------|---------|--------------|
| `cmd/kamacu/mcp_subcommand.go` | Subcommand dispatch entry; parses `mcp serve`, calls `internal/mcp.Run` | ~40 |
| `internal/mcp/server.go` | `mcp.NewServer` + tool registration + `StdioTransport.Run` | ~80 |
| `internal/mcp/bridge.go` | Authenticated HTTP client to Kamacu binary | ~80 |
| `internal/mcp/scope.go` | `KAMACU_SESSION_ID` → task/project resolution + cache | ~60 |
| `internal/mcp/tools*.go` | Per-domain tool handlers (one file per Kamacu resource area) | ~400 (across 6–8 files) |
| `internal/mcpconfig/claude.go` | Writes `.mcp.json` at worktree root | ~50 |
| `internal/mcpconfig/opencode.go` | Writes `opencode.json` at worktree root | ~50 |
| `internal/api/terminal_reads.go` | `GET /api/sessions/{id}/snapshot`, `/tail` HTTP handlers | ~100 |
| (Tests for all of the above) | | ~800 |

**Estimated new code: ~1,600 LOC including tests** (within the typical range for a Kamacu milestone phase).

### Modified files

| File | Change | Why |
|------|--------|-----|
| `cmd/kamacu/main.go` | Add subcommand dispatch at the top of `main()`: `if len(os.Args) >= 2 && os.Args[1] == "mcp" { os.Exit(runMCPSubcommand(os.Args[2:])) }`. Add a new import (`kamacu/internal/mcp`). | One new branch; existing HTTP-server path unchanged. |
| `internal/api/routes.go` or `sessions.go` | Register the two new read endpoints: `mux.HandleFunc("GET /api/sessions/{id}/snapshot", s.snapshot)` and `mux.HandleFunc("GET /api/sessions/{id}/tail", s.tail)`. | One new `SessionRoutes`-adjacent function. |
| `internal/session/session.go` | Possibly add `Tail(since int) []byte` if the existing `Snapshot()` isn't sufficient. (Likely unnecessary — `Snapshot()` returns the full ring; the tail endpoint can compute a slice server-side from a byte-offset parameter.) | Only if the read endpoints need a method `Session` doesn't already expose. |
| `internal/session/manager.go` or `internal/api/sessions.go` | One new step in the spawn path: after `provisionWorktree` returns, before `mgr.Spawn`, call `mcpconfig.Write(engine, worktreePath, env)`. Engine-branch on `agent.engine` (`claude` → `WriteClaude`, `opencode` → `WriteOpenCode`, others → skip). | The single integration point with the v1.10 spawn engine. |
| `go.mod` | Add `github.com/modelcontextprotocol/go-sdk v1.6.1` and its transitive deps (all pure Go, no CGO). | One `go get` + `go mod tidy`. |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| `internal/mcp` ↔ Kamacu binary | HTTP (loopback) + envelope token | Clean network boundary. The subcommand cannot access Kamacu's in-memory state. |
| `internal/mcp` ↔ `internal/api` types | Go import (compile-time) | The bridge shares request/response struct definitions with the API layer where they exist, so wire shapes stay in one place. Import direction: `internal/mcp` imports `internal/api`, never the reverse. |
| `internal/mcp` ↔ `internal/mcpconfig` | None (separate packages, separate processes) | The subcommand reads env; the config writer writes env into a file. They share no code. Don't merge them. |
| `internal/mcpconfig` ↔ spawn engine | Function call | `mcpconfig.WriteClaude(worktreePath, env)` is called from the existing spawn handler. Pure function, no state. |
| Agent CLI ↔ `kamacu mcp serve` | stdio (JSON-RPC) | The agent CLI spawns the subcommand; stdin/stdout are the MCP wire. |
| `kamacu mcp serve` ↔ Kamacu binary WS | WS (read-only) | For the long-running subscribe tool. The subcommand opens a WS as a read-only client — it MUST NOT send `FrameData` (PTY input) frames. Enforced by a `wsReader` wrapper with no Write method. |

---

## Suggested Build Order (dependency-aware)

The build order is forced by three dependencies: (1) the subcommand must speak MCP before config injection is useful; (2) config injection must work before any `KAMACU_SESSION_ID`-inheriting tool is meaningful end-to-end; (3) the new HTTP read endpoints must exist before the terminal tools can call them. The minimal vertical slice that proves the architecture is **subcommand skeleton + one tool + Claude config injection + end-to-end demo**.

### Phase 1 — Subcommand skeleton + minimal vertical slice

**Goal:** Prove the architecture end-to-end. `kamacu mcp serve` exists, speaks the protocol, and answers ONE read-only tool (`get_my_task`) by bridging to the existing Kamacu HTTP API. The agent CLI (Claude Code) discovers it via a worktree-root `.mcp.json` and successfully calls the tool from inside a Claude session.

**Delivers:**
- `cmd/kamacu/mcp_subcommand.go` — stdlib dispatch
- `internal/mcp/server.go` + `bridge.go` + `scope.go` — minimal SDK-wired server
- `internal/mcp/tools_tasks.go` — `get_my_task` tool only
- `internal/mcpconfig/claude.go` — writes `.mcp.json` at worktree root
- One-line modification to the spawn engine: call `mcpconfig.WriteClaude` for `engine=claude` after `provisionWorktree`
- `go.mod` updated with the official SDK
- End-to-end smoke test: spawn a Claude task, observe `kamacu mcp serve` start, call `get_my_task` from inside Claude, verify the response matches the task

**Avoids:** Anti-patterns 1 (importing internals), 5 (PTY write — none in this phase), 6 (hand-rolled JSON-RPC).

**Success criterion:** "From inside a Claude session spawned by Kamacu, I can ask Claude 'what task am I working on?' and it correctly answers by calling the Kamacu MCP tool."

**Why first:** Establishes the architecture (subcommand, dispatch, bridge, scope, config injection) before any of the long tail of tools is built. If the architecture is wrong, this is the cheapest place to discover it.

### Phase 2 — Full table-stakes tool surface

**Goal:** Every "table-stakes" tool from FEATURES.md is implemented and tested. The agent can drive the full Kamacu API: tasks, projects, workspaces, agents, sessions, diff, worktree cleanup.

**Depends on:** Phase 1 (server skeleton + scope + bridge).
**Delivers:**
- `internal/mcp/tools_tasks.go` (rest of task tools)
- `internal/mcp/tools_projects.go`, `tools_sessions.go`, `tools_diff.go`, `tools_worktree.go`, `tools_github.go`
- Per-tool tests with mocked bridge

**Avoids:** Anti-pattern 7 (no mounting inside Kamacu binary — all tools bridge).

### Phase 3 — opencode integration + custom-engine decision

**Goal:** `mcpconfig.WriteOpenCode` ships; opencode tasks get the same auto-discovery. Custom agents are explicitly skipped (documented).

**Depends on:** Phase 1 (the writer pattern is established).
**Delivers:**
- `internal/mcpconfig/opencode.go`
- Spawn engine branch on `engine=opencode` calls `WriteOpenCode`
- Decision documented: custom agents don't get auto-injection (their CLIs may not speak MCP; users can drop their own config file if they want)

**Why third:** Decoupled from Phase 2 — could ship in parallel. Stays third because Claude Code is the default agent and proves the pattern first; opencode is the second engine and surfaces any engine-specific config quirks.

### Phase 4 — Terminal read access (snapshot + subscribe)

**Goal:** The headline differentiator. The agent can read terminal state. Ship the snapshot tool first, then the bounded subscribe tool with cancellation + progress.

**Depends on:** Phase 1 (server skeleton); the two new HTTP endpoints in `internal/api/terminal_reads.go`.
**Delivers:**
- `internal/api/terminal_reads.go` — `GET /api/sessions/{id}/snapshot`, `/tail`
- `internal/mcp/tools_terminal.go` — `get_session_output` (snapshot) + `subscribe_session_output` (long-running)
- The `wsReader` wrapper enforcing the read-only contract
- Cancellation + progress tests using the SDK's `mcp.NewInMemoryTransport`

**Avoids:** Anti-pattern 4 (long-running tool with no cancellation — `duration_seconds` parameter + `ctx.Done()` are explicit), Anti-pattern 5 (PTY write — `wsReader` enforces).

**Success criterion:** "From inside a Claude session, I can ask 'watch my other agent's terminal for 30 seconds and summarise what it did' and Claude correctly calls the subscribe tool, waits for it to return, and summarises the output."

**Why fourth:** The genuinely new capability. Everything before it is translation; this is the one place v1.11 adds a behaviour Kamacu didn't have. Saving it for after the surface is built lets it land on a stable foundation.

### Phase 5 — Polish + edge cases

**Goal:** Lifecycle edge cases tested (kamacu restart mid-tool-call, agent-CLI crash, leaked goroutines). The `get_my_brief` composite tool ships. Observability (`X-Kamacu-Source: mcp` header? logging?) is added if needed.

**Depends on:** All earlier phases.

---

## Phase-Specific Warnings (for PITFALLS.md and PLAN-phase)

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| Phase 1 (subcommand dispatch) | Forgetting that `flag.Parse()` consumes `os.Args[1:]` — must dispatch BEFORE flag.Parse, not after | Check `os.Args[1] == "mcp"` at the very top of `main()`, before any flag registration. |
| Phase 1 (config injection) | Writing `${KAMACU_HOOK_TOKEN}` (template) instead of the literal value — Claude Code does env expansion in its *own* env, which doesn't have the token | Write literal values from the in-memory `hookToken` variable in `main.go`. Pass them through the spawn call chain. |
| Phase 1 (binary path) | Hard-coding `"kamacu"` in the config — breaks if the user installed to a non-PATH location | Use `os.Executable()` in the Kamacu binary at spawn time to get the absolute path; write that into the config. |
| Phase 4 (WS read-only) | Accidentally sending `FrameData` frames from the subscribe tool — would inject bytes into the sibling agent's PTY | `wsReader` wrapper with no Write method; compile-time guarantee. Reviewer-enforced. |
| Phase 4 (cancellation) | Returning an error from the handler when `ctx.Done()` fires — SDK treats this as a tool failure, agent sees error | Return `(finalResult, nil)` — the SDK suppresses the response on cancellation; returning a clean result is safe. |
| Phase 4 (idle timeout) | Tool call blocking past Claude Code's stdio idle timeout (30 min default) — agent kills it | Hard-cap `duration_seconds` at 300; document in the tool description. |
| All phases (logging) | `fmt.Println` or `log.Println` to stdout — corrupts the MCP JSON-RPC stream | All logging goes through `log/slog` to stderr. Add a test that asserts stdout contains only valid JSON-RPC messages. |

---

## Sources

### Primary (HIGH confidence)

- **Live Kamacu codebase at v1.10** — every integration point above (`cmd/kamacu/main.go`, `internal/api/{routes,sessions,hooks}.go`, `internal/session/{manager,session}.go`, `internal/ws/{handler,proto}.go`, `internal/opencode/plugin.go`, `internal/store/migrations/`) was read in full from the working tree on 2026-07-21. Every file/function/table name in the integration map is real.
- **[pkg.go.dev/github.com/modelcontextprotocol/go-sdk](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk) v1.6.1** — official Go SDK, 4.8k stars, Apache-2.0/MIT, maintained with Google. `mcp.NewServer` + `mcp.AddTool` + `mcp.StdioTransport` API verified; Cancellation example read in full (confirms `ctx.Done()` fires on `notifications/cancelled`); Progress example read in full (confirms `req.Session.NotifyProgress`).
- **[pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp@v1.6.1](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk@v1.6.1/mcp)** — full API surface: Server, CallToolRequest/Result, sessions, middleware, all transport types.
- **[MCP spec 2025-11-25: Cancellation](https://modelcontextprotocol.io/specification/2025-11-25/basic/utilities/cancellation)** — `notifications/cancelled` flow, behavior requirements, timing considerations.
- **[MCP spec 2025-11-25: Tools](https://modelcontextprotocol.io/specification/2025-11-25/server/tools)** — tool definition shape, tool result types (text/image/structured), error handling (protocol vs execution), security considerations.
- **[Claude Code MCP docs](https://docs.anthropic.com/en/docs/claude-code/mcp)** — three scopes (local/project/user), `.mcp.json` shape, `${VAR}` expansion, `CLAUDE_PROJECT_DIR` env, stdio idle timeout (30min default), `MCP_TOOL_TIMEOUT`, per-server `timeout` field.
- **[opencode config docs](https://opencode.ai/docs/config/)** — `mcp` key (NOT `mcpServers`), `type: "local"` vs `"remote"`, `command` as single array, `environment` (NOT `env`), precedence order, `{env:VAR_NAME}` interpolation.
- **[github-mcp-server install-opencode.md](https://github.com/github/github-mcp-server/blob/main/docs/installation-guides/install-opencode.md)** — corroborating real-world example of the opencode MCP config shape.
- **[Subcommands with Go's flag package (Abhinav Gupta)](https://abhinavg.net/2022/08/13/flag-subcommand/)** — stdlib subcommand pattern with `flag.NewFlagSet` + `flag.Args()[0]` dispatch; matches Kamacu's stdlib-only ethos.

### Aligned research (this milestone)

- **[.planning/research/STACK.md](./STACK.md)** — official `modelcontextprotocol/go-sdk` v1.6.1 pick, bridge pattern from `examples/server/proxy/main.go`, transitive deps verified, agent-CLI config shapes for both engines.
- **[.planning/research/FEATURES.md](./FEATURES.md)** — tool surface (28 tools + 2 spawn integrations), `ToolAnnotations` discipline, two-class error handling, subscribe mechanics. (Note: FEATURES.md initially recommended `mark3labs/mcp-go`; STACK.md overrules with the official SDK. This document aligns with STACK.md.)

---

*Architecture research for: MCP (Model Context Protocol) server capability added to an existing local-only Go single-binary app — stdio MCP subcommand bridging to the existing HTTP API, plus spawn-time agent-CLI integration.*
*Researched: 2026-07-21*
*Ready for roadmap: yes*
