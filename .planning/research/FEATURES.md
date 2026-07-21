# Feature Research

**Domain:** MCP (Model Context Protocol) server capability added to an existing local-only Go + React single-binary app — exposing Kamacu's existing HTTP API surface as MCP primitives so agents running inside Kamacu task PTYs can drive the app as the user's delegate.
**Researched:** 2026-07-21
**Confidence:** HIGH — all primitive semantics, lifecycle requirements, error codes, and config-file shapes below are taken live from the MCP spec (2025-06-18, the version both Claude Code and opencode currently negotiate to), the canonical `modelcontextprotocol/servers` reference implementations (`everything`, `git`, `filesystem`), the `mark3labs/mcp-go` framework README + source tree, and the official opencode MCP docs (all fetched 2026-07-21).

---

## TL;DR for the roadmap author

- **One new subcommand, one new Go package, zero new HTTP surface.** `kamacu mcp serve` is a stdio JSON-RPC process that bridges to the *existing* Kamacu HTTP API on localhost. Every tool handler is a thin HTTP client call — the validation, side effects, and SQLite writes stay in the existing API layer. Do **not** reimplement business logic in the MCP server.
- **Default everything to Tools.** Despite the spec offering three primitives (Tools, Resources, Prompts), the dominant 2026 reality is: agent-CLIs (Claude Code, opencode) auto-invoke **Tools**; Resources are application-pulled via host context-pickers (the user picks them, not the LLM); Prompts are slash-command templates. **Kamacu reads/writes should be Tools**, with two narrow exceptions: a `kamacu://tasks/{id}/brief` resource for host-context-injection, and 2–3 prompts for canned workflows.
- **Per-task auto-scoping is a Tool Filter, not a separate tool set.** mcp-go's `server.WithToolFilter(func(ctx, tools) []Tool)` reads `KAMACU_SESSION_ID` from env (already injected at spawn) and rewrites the visible tool list per session — convenience tools (`list_my_tasks`, `get_my_session`) appear when scoped, generic tools (`list_tasks`, `get_session`) always present.
- **The "subscribe terminal output" question has three correct answers; ship them in order.** (1) **Snapshot tool** — `tail_session_output(session_id, lines=200)` returns a bounded slice from the existing ring buffer (table stakes, trivial). (2) **Bounded live-tail tool** with progress notifications — `tail_session_output_live(session_id, duration_seconds=30)` streams `notifications/progress` chunks then returns (differentiator, medium complexity). (3) **Resource + subscribe** — `kamacu://sessions/{id}/output` as a subscribable resource (semantically pure, but verify host support before building; defer to v1.12+).
- **No keystroke injection. Ever.** Surfaced as the headline anti-feature. An agent observing a sibling session must never write bytes into its PTY — it conflicts with the browser-attached user and the other agent's intent. Read-only terminal access is the contract.
- **Two different config-file shapes for the two supported agent CLIs.** Claude Code reads `~/.claude.json` → `mcpServers` (separate `command` + `args` fields). opencode reads `opencode.json` → `mcp` (single `command` array). The spawn-time integrator writes the right shape per engine. Getting this wrong = silent tool discovery failure.
- **`KAMACU_HOOK_TOKEN` is the auth, full stop.** MCP spec explicitly says stdio servers retrieve credentials from the environment; Kamacu already injects the token at spawn. No OAuth, no API key, no new auth surface.

---

## Recommended Stack (referenced from FEATURES perspective)

The full technology rationale belongs in STACK.md; this section captures only the **feature-shaping** choices.

| Choice | Why it shapes the feature surface |
|--------|----------------------------------|
| **`mark3labs/mcp-go`** as the MCP framework | Implements spec 2025-11-25 with back-compat to 2025-06-18 (the version Claude Code/opencode negotiate to). Provides `server.NewMCPServer` + `server.ServeStdio` (so `kamacu mcp serve` is a 10-line main), `mcp.NewTool` builder with `WithString`/`WithNumber`/`Enum`/`Pattern`/`Required` for input schemas, `s.AddTool(tool, handler)` registration, `mcp.NewToolResultText`/`NewToolResultError`, and crucially `server.WithToolFilter` for per-session scoping. Without this framework, you'd hand-roll JSON-RPC + capability negotiation + the tools dispatcher — weeks of work for zero product value. |
| **stdio transport** (not HTTP/SSE) | The agent CLI spawns `kamacu mcp serve` as a child process; stdin/stdout is the JSON-RPC transport. No port to allocate, no firewall, no auth header — the parent (agent CLI) is the only client. HTTP/SSE/streamable-HTTP transports exist in mcp-go but are for the external-editor use case (explicitly out of scope per PROJECT.md). |
| **HTTP-bridge architecture** (MCP tool → localhost:7333 HTTP) | Every tool handler is a thin `http.Client` call to the existing API, attaching `X-Kamacu-Token: $KAMACU_HOOK_TOKEN`. Zero business logic in the MCP server. This means: existing validation runs, existing side effects fire (reaper, hooks, status updates), existing tests cover the data path. The MCP layer is a translation, not a reimplementation. |
| **JSON Schema (inputSchema) generated from Go struct tags** | mcp-go's `mcp.NewTool` builder produces the JSON Schema the agent-CLI shows the LLM. The LLM decides which tool to call based on `name` + `description` + `inputSchema` — so descriptions must be LLM-legible (state the unit, the side effect, the return shape). |

---

## Feature Landscape

### Table Stakes (Users — i.e., Agents — Expect These)

A v1.11 MCP server that omits any of these feels broken: the agent can see the board in the UI but can't reproduce the action programmatically. Every item maps 1:1 to an existing Kamacu HTTP endpoint.

#### Protocol & Lifecycle (Required by the MCP spec)

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| `initialize` handshake + capability negotiation | Spec-mandated. Without it the agent-CLI drops the connection. mcp-go handles this; you only set `serverInfo{name, version}` + capability flags. | LOW | mcp-go auto-advertises `tools` capability when you call `s.AddTool`. Set `listChanged: true` if you ship dynamic tool filtering. |
| `tools/list` (with pagination cursor) | Spec-mandated for the `tools` capability. Agent-CLI calls this once at startup; the result defines the agent's entire tool vocabulary. | LOW | mcp-go handles. Cap at 100 tools/page; with ~30 tools you'll fit on one page. |
| `tools/call` dispatch | Spec-mandated. Routes `(name, arguments)` to the registered handler. | LOW | mcp-go handles. Your job is per-tool handlers. |
| `ping` | Spec-mandated cheap keepalive. Some agent-CLIs ping every 30s to detect dead servers. | LOW | mcp-go handles. |
| `notifications/initialized` | Spec-mandated (client → server). Gates when you can start sending requests. | LOW | mcp-go handles. |
| Clean stdio shutdown (close stdin → SIGTERM → SIGKILL) | Spec-mandated. If `kamacu mcp serve` leaks when the agent-CLI exits, you get zombie processes. | LOW | mcp-go handles via `server.ServeStdio` context cancellation. Verify with a "spawn agent → kill -9 agent CLI → check no kamacu mcp processes" test. |
| `ToolAnnotations` on **every** tool (`readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint`) | Clients (Claude Code, opencode) use these to decide whether to prompt the user for confirmation before calling. The reference `git` server annotates every tool; you should too. Trust/safety surface. | LOW | All Kamacu tools are `openWorldHint: false` (they operate on local Kamacu state, not the open internet). Reads get `readOnlyHint: true`. Moves/creates/deletes get the appropriate flags. |
| Two-class error handling | Spec: protocol errors (JSON-RPC error codes: `-32602` unknown tool / invalid args, `-32603` internal, `-32002` not-found) vs tool execution errors (`result.isError: true` with a `text` block). Mixing them confuses agent-CLIs. | LOW | mcp-go: return `(nil, err)` for protocol errors; return `(mcp.NewToolResultError(msg), nil)` for tool execution errors. |

#### Task Lifecycle Tools (Core Delegate Surface)

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| `list_tasks(project_id?, status?)` | The agent needs to read the board. Maps to `GET /api/tasks`. | LOW | Paginate via cursor; support filters. With KAMACU_SESSION_ID set, a `list_my_tasks` variant narrows to the calling session's project. |
| `get_task(task_id)` | Read one task's full state (title, description, status, timestamps). `GET /api/tasks/{id}`. | LOW | Return as structured content + text mirror (spec backwards-compat). |
| `create_task(title, description?, project_id?)` | Create a sibling task. `POST /api/tasks`. | LOW | When KAMACU_SESSION_ID is set, default `project_id` to the calling session's project — the agent doesn't need to discover it. |
| `update_task(task_id, title?, description?, status?)` | Edit task fields. `PATCH /api/tasks/{id}`. | LOW | Partial PATCH semantics (only send changed fields). |
| `move_task(task_id, status, before_id?, after_id?)` | Move card on the board (manual order). `POST /api/tasks/{id}/move`. | LOW | Existing endpoint already handles positioning. |
| `delete_task(task_id)` | Remove a task. `DELETE /api/tasks/{id}`. | LOW | Existing gated worktree-cleanup runs server-side; no special handling. |

#### Session Lifecycle Tools (Limited — Read + Control, No Write)

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| `list_sessions(project_id?, status?)` | What's running. `GET /api/sessions`. | LOW | Filtered to live sessions by default. |
| `get_session(session_id)` | Session metadata (status, started_at, task link). `GET /api/sessions/{id}`. | LOW | |
| `start_session(task_id)` | Spawn the agent for a task (the same Start button the UI has). `POST /api/tasks/{id}/sessions`. | MEDIUM | Side-effecting (spawns a PTY); annotate `destructiveHint: false, idempotentHint: false`. Useful for "kick off a review session on this PR" workflows. |
| `stop_session(session_id)` | Stop a session. `POST /api/sessions/{id}/stop`. | MEDIUM | Side-effecting (kills PTY); annotate `destructiveHint: true`. Useful for an agent to clean up after itself. |

#### Project / Workspace / Agent Metadata Tools

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| `list_projects(workspace_id?)` | Discover what projects exist. `GET /api/projects`. | LOW | Scoped to active workspace by default. |
| `get_project(project_id)` | Full project metadata (repo, agent, github link). `GET /api/projects/{id}`. | LOW | |
| `list_workspaces()` | Discover workspaces. `GET /api/workspaces`. | LOW | |
| `list_agents()` | Discover configured agents. `GET /api/agents`. | LOW | |
| `get_settings()` | Read the global Kamacu config. `GET /api/settings`. | LOW | Read-only view; the agent shouldn't usually change settings. |

#### GitHub PR Review Tools (Read-Only — Matches Existing App Policy)

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| `list_pr_reviews(project_id)` | List PRs awaiting review. `GET /api/projects/{id}/pull-requests`. | LOW | Mirrors the Review column; cached `gh` cycle server-side. |
| `get_pr_review(project_id, pr_number)` | Open-or-reattach a review workspace. `POST /api/projects/{id}/pull-requests/{n}/review` + `GET`. | MEDIUM | Side-effecting (creates a worktree); reuse existing gated path. |

#### Diff & Worktree Tools

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| `get_task_diff(task_id)` | The diff a task produced. Reuse the existing `/api/tasks/{id}/diff` renderer output. | LOW | Returns markdown/unified diff as text. |
| `mark_file_viewed(task_id, file_path, viewed)` | Toggle the per-file Viewed flag. `POST /api/tasks/{id}/diff/viewed`. | LOW | Annotate `idempotentHint: true`. |
| `list_worktree_cleanup_candidates()` | List orphan/stale/finished worktrees. `GET /api/worktrees`. | LOW | |
| `clean_worktree_eligible()` | Bulk safe cleanup. `POST /api/worktrees/clean-eligible`. | MEDIUM | Reuses existing never-forces path. |
| `remove_worktree(worktree_path, force?)` | Force-remove one worktree (always keeps the branch). `POST /api/worktrees/remove`. | MEDIUM | `destructiveHint: true`. |

#### Agent-CLI Integration (Spawn-Time Registration)

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Write `mcpServers.kamacu` into Claude Code config at spawn | Without this, the agent never discovers Kamacu tools. | MEDIUM | Patch `~/.claude.json` → `mcpServers.kamacu = {command: "kamacu", args: ["mcp", "serve"], env: {KAMACU_HOOK_TOKEN, KAMACU_SESSION_ID}}`. Must be the *user-level* file, not `settings.json` (silently ignored per danielmiessler/Personal_AI_Infrastructure#646). |
| Write `mcp.kamacu` into opencode config at spawn | Same as above, different shape. | MEDIUM | Patch `opencode.json` → `mcp.kamacu = {type: "local", command: ["kamacu", "mcp", "serve"], environment: {...}, enabled: true}`. **Note: `command` is an array** (command + args combined), and the config key is `mcp` not `mcpServers`. Getting either wrong = silent no-op. |

**Table-stakes count: 28 tools + 2 spawn integrations.** This matches the milestone target of "~30 tools."

---

### Differentiators (Competitive Advantage)

These are not expected by agents (no agent has them today), but they materially increase agent autonomy and align with Kamacu's core value ("one place to see and drive all agent work").

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **Per-task auto-scoping tool filter** | The agent doesn't have to pass `project_id`/`session_id` on every call — the server reads `KAMACU_SESSION_ID` from env and scopes convenience tools (`list_my_tasks`, `get_my_session`, `tail_my_output`) automatically. Removes ~50% of the boilerplate args an agent would otherwise need. | MEDIUM | mcp-go's `server.WithToolFilter(func(ctx, tools) []Tool)` is the hook. When `KAMACU_SESSION_ID` is set, append the scoped variants; otherwise hide them. The scoped variants have simpler schemas → less LLM confusion. |
| **Bounded live-tail tool** (`tail_session_output_live(session_id, duration_seconds=30, lines?)`) | An agent can watch a sibling session for "is it done yet?" / "did it produce errors?" without leaving its own context. Bounded duration means the tool call always returns — agents don't do well with infinite subscriptions. | HIGH | Uses `notifications/progress` to stream chunks of the existing ring buffer + live PTY output. Server-side goroutine reads the existing Session's broadcast channel, chunks every 500ms or 4KB, sends each as a progress notification with `{progress: bytes_sent, total: budget_bytes, progressToken}`. Cancels on context.Done(). Honors the "no write" contract. This is the headline differentiator. |
| **`get_my_brief` tool** | Returns a single structured blob the agent reads at session-start: "You are working on task X (id, title, description, status) in project Y (repo path, agent) — your worktree is at Z, the base branch is B, here are your sibling tasks [list]." One tool call instead of 4. | MEDIUM | Composite read: `get_task` + `get_project` + `list_tasks(siblings)` + worktree path. Cached for 30s. Drops cold-start token cost dramatically. |
| **Resource subscriptions for terminal output** (`kamacu://sessions/{id}/output`) | The semantically-correct way to expose live terminal state: agent subscribes once, gets `notifications/resources/updated` whenever new output arrives. Matches the MCP spec's intent for "data that changes over time." | HIGH | Requires verifying Claude Code/opencode actually wire up resource subscription loops in their agent loop. If they don't (likely in 2026), this is invisible to agents and becomes v1.12 work. Resource + subscribe is the right *long-term* shape; the bounded live-tail tool is the pragmatic *now* shape. Ship the tool first. |
| **Task-brief Resource** for host-context-injection (`kamacu://tasks/{id}/brief`) | A Resource the user can `@mention` in Claude Code / opencode to inject task context into the agent's prompt. Resources are application-pulled (host picks them via UI), which is exactly the @mention semantic. | LOW | Cheap to expose — same data as `get_task`, different primitive. Useful even if no agent actively subscribes; Claude Code's `@`-picker surfaces it. |
| **Prompts for canned workflows** | `open-pr-review(pr_number)`, `start-task(title, description)`, `prepare-for-review(task_id)` — multi-message prompt templates that pre-fill the agent's context with the right task/diff/repo data. User-triggered via `/open-pr-review` slash-command-style UI in the agent CLI. | MEDIUM | Each prompt returns a `GetPromptResult` with a sequence of `PromptMessage`s (text + embedded resources). mcp-go: `s.AddPrompt(mcp.NewPrompt("open-pr-review", mcp.WithArgument(...)), handler)`. |
| **Completion providers** for task/project IDs | When the user types a tool argument in the agent CLI, Kamacu autocompletes from live DB state. "list_tasks(project_id=|"<tab>" → list of project names. | MEDIUM | mcp-go: `server.WithCompletions()` + `WithPromptCompletionProvider` / `WithResourceCompletionProvider`. Optional polish; defer if scope is tight. |
| **Structured output schemas** (`outputSchema` on read tools) | Tools that return `structuredContent` validated against a JSON Schema let agent-CLIs (and downstream programmatic consumers) parse results reliably instead of regexing text. Spec-recommended for any tool with non-trivial return shape. | MEDIUM | mcp-go supports via `mcp.WithOutputSchema(...)` (or by registering a typed tool — `examples/typed_tools/`). Adds ~30 min per tool but materially improves agent accuracy on `list_tasks`/`get_session`/etc. |
| **Logging notifications** (`notifications/log` + `logging/setLevel`) | Kamacu emits structured log lines for tool calls (debug-level) and lifecycle events (info-level). Agent-CLI surfaces these in its own UI for debugging. Cheap observability win. | LOW | mcp-go: `server.WithHooks(...)` to wrap every tool call with `slog.Debug`. Set the `logging` capability on the server. |

---

### Anti-Features (Commonly Requested, Often Problematic)

Each anti-feature is documented with *why* it's tempting, *why* it breaks something, and *what to do instead*. The first one is the milestone's headline contract.

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| **PTY keystroke injection** (`send_input(session_id, bytes)`, `type_in_terminal(...)`) | "If the agent can read the terminal, surely it should write to it too — to nudge a sibling, answer a prompt, send Ctrl-C." | (1) Conflicts with the browser-attached user — two writers to one PTY produce garbage interleaving and confusing TUI redraws. (2) Conflicts with the *other* agent's intent — the agent in the PTY is a separate actor with its own goals; injecting bytes hijacks it. (3) Bypasses Claude Code's permission prompt UX (the agent would effectively have keyboard power of attorney). (4) PROJECT.md explicitly rules this out: "MCP write to live PTYs (keystroke injection) — v1.11 deliberately exposes read + subscribe only." | Read-only tools: `tail_session_output`, `get_session_status`. If an agent wants to stop a sibling, `stop_session(session_id)` (clean, gated, side-effects fire properly). If it wants to send a message, surface it in the *task description* via `update_task` and let the sibling agent pick it up. |
| **Direct DB access** (`run_sql(query)`, raw SQLite reads) | "Faster than HTTP — skip the API layer, just query the DB." | (1) Bypasses all validation and side effects (reaper, hooks, status updates, worktree gates). (2) Schema migrations break the agent silently. (3) Single escape hatch that punches a hole in the entire data model. | Always go through the HTTP API. The MCP server is a *translation layer*, not a data layer. If you need a query the API doesn't expose, add an HTTP endpoint first. |
| **One mega-tool with `action` parameter** (`kamacu(action: "list_tasks"|"create_task"|..., ...args)`) | "Simpler to register — one handler." | (1) The LLM sees a single tool with a giant union schema; it picks `action` less reliably than it picks between 30 named tools. (2) ToolAnnotations become meaningless (one tool can't be both read-only and destructive). (3) The reference `git` server deliberately ships 12 separate tools (`git_status`, `git_diff_staged`, `git_commit`, ...) for exactly this reason. | One verb-noun tool per action: `list_tasks`, `create_task`, `move_task`, etc. The LLM's tool-selection accuracy goes up sharply with named verbs. |
| **External/internet fetches** (`fetch_url`, `search_web`) | "The agent needs to read docs anyway — let's add a fetch tool." | (1) Scope creep — Kamacu is a local-only agent *orchestrator*, not a general-purpose toolkit. (2) `openWorldHint: true` tools erode the trust model (the agent can now exfiltrate). (3) Other MCP servers (the `fetch` reference server) already do this better. | Let the agent use its own host's fetch/Context7/etc. tools. Kamacu's tools are `openWorldHint: false` — they only touch local Kamacu state. |
| **Cross-user / admin tools** (`delete_all_tasks`, `reset_database`, `stop_all_sessions`) | "Useful for cleanup." | (1) An agent inside Kamacu is acting as the user's *delegate on a specific task* — admin powers violate least-privilege. (2) One bad agent prompt could nuke the board. | Surface only per-task / per-session operations. Bulk operations stay in the human-driven Settings UI. |
| **Static token in mcpServers config file** | "Just write the token into the config and forget about it." | (1) Token rotation invalidates the config silently — agent loses tool access with no error message. (2) Config file on disk = at-rest credential. | Inject env at spawn (`environment: {KAMACU_HOOK_TOKEN: "<rotated-token>"}`); the existing v1.10 spawn path already does this. Config file references the env name, never the literal token. |
| **Tool for tool-discovery** (`list_available_tools`, `describe_tool(...)`) | "Let the agent introspect the tool surface." | (1) Redundant — `tools/list` already does this at the protocol level. (2) An agent that needs to ask "what tools do you have?" is an agent that didn't read `tools/list` output. | Rely on the protocol. Improve tool `description` fields if the LLM is misusing tools — that's the root cause. |
| **Subscribing to ALL sessions** (`subscribe_all_sessions()`) | "Let the agent watch the whole board." | (1) Firehose — most agents only need *their* session. (2) Privacy / least-privilege violation across tasks. | Per-session subscriptions only: `tail_session_output(session_id)`. If global observability is needed, `list_sessions(status="running")` is enough. |
| **MCP server exposed to external editors (Claude Desktop / Cursor)** | "I want to use Cursor to drive Kamacu too." | Explicitly out of scope per PROJECT.md ("MCP server for external AI editors — separate future milestone"). Different transport (HTTP/SSE, not stdio), different auth (token-on-the-wire), different threat model (network reachable). | Stay stdio-only for v1.11. External-editor MCP is a separate v1.13+ milestone with its own research. |

---

## Feature Dependencies

```
[kamacu mcp serve stdio subcommand]
    │
    ├──requires──> [mark3labs/mcp-go dependency added to go.mod]
    │
    ├──requires──> [KAMACU_HOOK_TOKEN env contract (existing from v1.10)]
    │
    └──enables──> [Tool surface — each tool is an HTTP bridge to existing API]

[Tool surface — table-stakes CRUD]
    │
    ├──requires──> [Existing Kamacu HTTP API (v1.0–v1.10 — all built)]
    │
    ├──enhances──> [Per-task auto-scoping tool filter]
    │                  └──requires──> [KAMACU_SESSION_ID env (existing from v1.4)]
    │
    └──enhances──> [ToolAnnotations on every tool]

[Bounded live-tail tool — tail_session_output_live]
    │
    ├──requires──> [Snapshot tool — tail_session_output]  (ship snapshot first)
    │
    ├──requires──> [Existing ring buffer + Session broadcast channel (v1.0 Phase 2)]
    │
    └──requires──> [mcp-go progress notification support]

[Resource subscriptions for terminal output]
    │
    ├──requires──> [Bounded live-tail tool — proves the read path]
    │
    ├──requires──> [Verify host (Claude Code / opencode) actually subscribes]
    │                  └── if NO ──> defer to v1.12+
    │
    └──conflicts──> [None — coexists with the bounded tool]

[Spawn-time mcpServers integration]
    │
    ├──requires──> [kamacu mcp serve subcommand exists]
    │
    ├──branches──> [Claude Code path: ~/.claude.json mcpServers.kamacu]
    │
    └──branches──> [opencode path: opencode.json mcp.kamacu]

[Prompts (canned workflows)]
    │
    └──requires──> [Tool surface exists]  (prompts embed tool results as resources)

[Completion providers]
    │
    └──enhances──> [Tool input schemas]  (autocomplete task_id, project_id from live DB)
```

### Dependency Notes

- **`kamacu mcp serve` requires the HTTP API** — every tool handler is an HTTP client call. The MCP server is structurally a *client* of the existing Kamacu API, not a peer. This is the most important architectural constraint: **no business logic in the MCP layer.**
- **Per-task auto-scoping requires `KAMACU_SESSION_ID`** — already injected at agent spawn (Phase 4 / v1.10). The MCP server reads it once at startup; the tool filter appends/removes scoped tools based on its presence.
- **Bounded live-tail requires the snapshot tool first** — the snapshot tool (`tail_session_output`) exercises the ring-buffer read path with no streaming complexity. Ship it first, prove the data path, then add live streaming.
- **Resource subscriptions require host-side verification** — before building the resource + subscribe surface, verify that Claude Code and opencode actually wire up `resources/subscribe` → `notifications/resources/updated` → `resources/read` loops in their agent loop. If they only consume Tools (likely in 2026), the resource is invisible and the work is wasted. Defer to v1.12+ if unverified.
- **Prompts require the tool surface to exist** — a prompt like `open-pr-review` embeds a resource link to `kamacu://tasks/{id}/brief`; the resource handler calls `get_task`. Build tools first, then layer prompts.
- **Claude Code vs opencode integration paths diverge at the config-file shape** — two separate code paths in the spawn integrator, one per engine branch (mirrors the existing v1.10 engine fork on `agent.engine`).

---

## MVP Definition

### Launch With (v1.11 — Table-Stakes Milestone)

The minimum surface that delivers on PROJECT.md's "agent can drive the entire app as the user's delegate" promise. This is the milestone's must-have list.

- [ ] **`kamacu mcp serve` stdio subcommand** — the runnable thing. mcp-go `server.NewMCPServer + ServeStdio`.
- [ ] **Task CRUD tools** (6) — `list_tasks`, `get_task`, `create_task`, `update_task`, `move_task`, `delete_task`. The agent must be able to read and modify the board.
- [ ] **Session lifecycle tools, read + control** (4) — `list_sessions`, `get_session`, `start_session`, `stop_session`. Side-effecting ones get appropriate ToolAnnotations.
- [ ] **Project/workspace/agent/settings metadata tools** (5) — `list_projects`, `get_project`, `list_workspaces`, `list_agents`, `get_settings`. Read-only context.
- [ ] **GitHub PR review tools** (2) — `list_pr_reviews`, `get_pr_review`. Read-only + open-review-workspace (reuses existing gated path).
- [ ] **Diff & worktree tools** (5) — `get_task_diff`, `mark_file_viewed`, `list_worktree_cleanup_candidates`, `clean_worktree_eligible`, `remove_worktree`.
- [ ] **Snapshot terminal-output tool** (1) — `tail_session_output(session_id, lines=200)`. The read-only "what did the agent print?" surface. Honors the no-injection contract.
- [ ] **Spawn-time agent-CLI integration** (2 paths) — write `mcpServers.kamacu` (Claude Code) and `mcp.kamacu` (opencode) at agent spawn, with rotated env. Without this, tools exist but no agent discovers them.
- [ ] **Per-task auto-scoping tool filter** — `KAMACU_SESSION_ID` from env → scoped convenience tools appear (`list_my_tasks`, `get_my_session`, etc.). The differentiator that makes the table-stakes surface usable in practice.
- [ ] **ToolAnnotations on every tool** — `readOnlyHint` / `destructiveHint` / `idempotentHint` / `openWorldHint: false` everywhere. Trust/safety surface.

**v1.11 = ~23 tools + 2 spawn integrations + 1 tool filter.** This is the realistic milestone scope.

### Add After Validation (v1.12 — Differentiator Round)

Ship these once v1.11 is proven in real agent sessions.

- [ ] **Bounded live-tail tool** (`tail_session_output_live`) — once you see agents wanting to "watch" sibling sessions, build the streaming-progress version. Trigger: agents doing "wait for sibling to finish" loops via repeated snapshot polls.
- [ ] **`get_my_brief` composite tool** — once you see agents making 4 calls at session start, fold them into one. Trigger: cold-start token cost > 2K tokens.
- [ ] **Prompts for canned workflows** (`open-pr-review`, `start-task`) — trigger: users typing the same multi-step sequences repeatedly.
- [ ] **Completion providers** — trigger: users mistyping IDs in the agent CLI.
- [ ] **Structured output schemas on all read tools** — trigger: agents mis-parsing text results.
- [ ] **Logging notifications** — trigger: debugging agent tool-selection issues.

### Future Consideration (v1.13+)

- [ ] **Resource subscriptions for terminal output** (`kamacu://sessions/{id}/output`) — defer until Claude Code/opencode verifiably subscribe to resources in their agent loop. Likely a v1.13 item once hosts catch up.
- [ ] **External-editor MCP server (HTTP/SSE transport)** — explicitly a separate milestone per PROJECT.md. Different auth, different threat model.
- [ ] **Bidirectional MCP Tasks (SEP-1686)** — for truly long-running agent operations (a Kamacu task that takes 30+ minutes). mcp-go supports it via `WithTaskSupport`; likely overkill for v1.x.
- [ ] **Sampling integration** — let a Kamacu-driving agent delegate sub-queries back to the host LLM via `sampling/createMessage`. Advanced; not needed for the delegate use case.

---

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| `kamacu mcp serve` stdio subcommand | HIGH (foundation) | LOW (mcp-go handles) | **P1** |
| Task CRUD tools (6) | HIGH (core delegate) | LOW (HTTP bridges) | **P1** |
| Session lifecycle tools (4) | HIGH (core delegate) | MEDIUM (side effects) | **P1** |
| Project/workspace/agent/settings tools (5) | MEDIUM (context) | LOW (read bridges) | **P1** |
| GitHub PR review tools (2) | MEDIUM | LOW (reuses existing) | **P1** |
| Diff & worktree tools (5) | MEDIUM | LOW–MEDIUM | **P1** |
| Snapshot terminal-output tool | HIGH (headline contract: read-only terminal) | LOW (ring buffer exists) | **P1** |
| Spawn-time mcpServers integration (Claude Code + opencode) | HIGH (without it, nothing works) | MEDIUM (two config shapes) | **P1** |
| Per-task auto-scoping tool filter | HIGH (usable-in-practice) | MEDIUM (filter logic) | **P1** |
| ToolAnnotations everywhere | MEDIUM (trust/safety) | LOW (annotations on each tool) | **P1** |
| Bounded live-tail tool | HIGH (differentiator) | HIGH (streaming + progress) | **P2** |
| `get_my_brief` composite tool | MEDIUM | LOW (composite read) | **P2** |
| Prompts (canned workflows) | MEDIUM | MEDIUM | **P2** |
| Completion providers | LOW | MEDIUM | **P3** |
| Structured output schemas | MEDIUM (agent accuracy) | MEDIUM (per-tool schema) | **P2** |
| Logging notifications | LOW (debugging) | LOW | **P3** |
| Resource subscriptions for terminal output | MEDIUM (semantic purity) | HIGH (verify host support first) | **P3** |

**Priority key:**
- **P1**: Must have for v1.11 launch — without these, the milestone doesn't deliver.
- **P2**: Differentiators — ship in v1.12 once v1.11 is validated.
- **P3**: Polish/future — defer until hosts/clients catch up or trigger conditions hit.

---

## Competitor / Reference Implementation Analysis

What existing MCP servers do, and what Kamacu copies vs. does differently.

| Aspect | `everything` (reference showcase) | `git` (closest CLI-wrapper analog) | `filesystem` (allowed-roots pattern) | **Kamacu's approach** |
|--------|-----------------------------------|------------------------------------|--------------------------------------|----------------------|
| **Tool naming** | hyphen-case (`trigger-long-running-operation`) | snake_case (`git_status`, `git_diff_staged`) | hyphen-case (`read_file`, `list_directory`) | **snake_case, no namespace prefix** (`list_tasks`, not `kamacu_list_tasks`) — the host CLI namespaces automatically (`kamacu_list_tasks` is what the LLM sees). |
| **Tool count** | ~18 (showcase) | 12 (one per verb) | ~8 | ~23 in v1.11, ~30 with differentiators. Larger surface than most; justified by UI parity. |
| **ToolAnnotations** | On every tool | On every tool ( readOnlyHint/destructiveHint/idempotentHint/openWorldHint ) | On every tool | **Copy this discipline.** Every Kamacu tool gets annotations; clients use them for confirmation prompts. |
| **Input validation** | Zod schemas | Pydantic schemas + injection defense (reject args starting with `-`); path validation | Path validation (allowed roots) | **mcp-go `mcp.WithString(... Enum(...), Pattern(...))`** for declarative validation; explicit `strings.HasPrefix(arg, "-")` rejection not needed (no shell-out from tools — tools call HTTP). Validate `session_id`/`task_id` ownership via the existing HTTP layer. |
| **Long-running ops** | `trigger-long-running-operation` (progress notifications); SEP-1686 tasks (`simulate-research-query`) | None (all synchronous) | None | **Bounded live-tail tool** with progress notifications (Phase-2 differentiator). |
| **Resources** | Dynamic text/blob templates, session-scoped resources, static docs | None (tools only) | Files as resources (`file://` URIs) | **One resource for host-context-injection** (`kamacu://tasks/{id}/brief`); terminal-output resource deferred to v1.13. |
| **Prompts** | 4 demos (simple, args, completable, resource) | None | None | **2–3 canned workflows** in v1.12 (`open-pr-review`, `start-task`). |
| **Auth** | n/a (showcase) | n/a (local) | n/a (local) | **`KAMACU_HOOK_TOKEN` env**, attached as `X-Kamacu-Token` header on every HTTP call to the existing API. Spec-compliant (stdio: "retrieve credentials from the environment"). |
| **CLI shell-out** | n/a | Wraps `GitPython` (which shells out to `git`) | Direct filesystem ops | **Wraps Kamacu HTTP** — no CLI shell-out from the MCP layer. The existing API handles git/claude/gh/tmux shell-outs. |
| **Discovery config** | `mcpServers.everything = {command: "npx", args: [...]}` | `mcpServers.git = {command: "uvx", args: [...]}` | `mcpServers.filesystem = ...` | **Per-engine shape**: Claude Code uses `mcpServers` (separate `command`+`args`); opencode uses `mcp` (single `command` array). Spawn integrator writes the right shape. |

### What to copy from each reference

- **From `everything`**: ToolAnnotations on every tool (non-negotiable); the `trigger-long-running-operation` progress-notification pattern for the bounded live-tail; the resource subscription demo (if you build the terminal-output resource).
- **From `git`**: The verb-per-tool discipline (12 separate `git_*` tools, not one mega-tool); the per-tool input-schema discipline (Pydantic in their case, mcp-go builder in ours); the `validate_repo_path` style of defense-in-depth (validate `task_id`/`session_id` belong to the calling agent's project before returning data — least-privilege).
- **From `filesystem`**: Allowed-paths enforcement → for Kamacu, "allowed sessions" enforcement (an agent scoped to task X should not be able to read task Y's terminal output unless explicitly granted).
- **From `sequentialthinking`**: Proof that a stateful server with even one tool is fine if the abstraction is right. Don't pad the tool count for its own sake.

### What to do differently

- **HTTP bridge, not direct state.** Most reference servers (git, filesystem) operate directly on the resource (filesystem, repo). Kamacu's MCP server operates on the *HTTP API* — the existing validation, side effects, and tests all still apply. This is unusual but correct for Kamacu: the API is the source of truth, the MCP server is one more (very privileged) client.
- **Per-task auto-scoping.** No reference server does this (they have no notion of "the calling session"). Kamacu's `KAMACU_SESSION_ID` env contract makes it possible and it's the headline DX feature.
- **Read-only terminal access by contract.** Most terminal-MCP servers (gotty-style) expose bidirectional PTY. Kamacu's agents-get-read-only is a deliberate safety choice tied to the browser-attached user.

---

## Tool Surface Reference (Concrete v1.11 List)

For the roadmap author and PLAN-phase. Each tool maps to an existing HTTP endpoint — the MCP handler is a thin HTTP client call.

### Task Lifecycle (6 tools)

| Tool | Input | Output | HTTP Bridge | Annotations |
|------|-------|--------|-------------|-------------|
| `list_tasks` | `project_id?`, `status?`, `limit?`, `cursor?` | Task[] (structured) | `GET /api/tasks` | RO, idempotent, !openWorld |
| `get_task` | `task_id` | Task (structured) | `GET /api/tasks/{id}` | RO, idempotent, !openWorld |
| `create_task` | `title`, `description?`, `project_id?`, `status?` | Task | `POST /api/tasks` | !RO, !destructive, !idempotent, !openWorld |
| `update_task` | `task_id`, `title?`, `description?`, `status?` | Task | `PATCH /api/tasks/{id}` | !RO, !destructive, idempotent, !openWorld |
| `move_task` | `task_id`, `status`, `before_id?`, `after_id?` | Task | `POST /api/tasks/{id}/move` | !RO, !destructive, idempotent, !openWorld |
| `delete_task` | `task_id` | `{deleted: true}` | `DELETE /api/tasks/{id}` | !RO, destructive, !idempotent, !openWorld |

### Session Lifecycle (4 tools)

| Tool | Input | Output | HTTP Bridge | Annotations |
|------|-------|--------|-------------|-------------|
| `list_sessions` | `project_id?`, `status?` | Session[] | `GET /api/sessions` | RO, idempotent, !openWorld |
| `get_session` | `session_id` | Session | `GET /api/sessions/{id}` | RO, idempotent, !openWorld |
| `start_session` | `task_id` | Session | `POST /api/tasks/{id}/sessions` | !RO, !destructive, !idempotent, !openWorld |
| `stop_session` | `session_id` | `{stopped: true}` | `POST /api/sessions/{id}/stop` | !RO, destructive, !idempotent, !openWorld |

### Terminal Output — Read Only (1 table-stakes + 1 differentiator)

| Tool | Input | Output | HTTP Bridge | Annotations |
|------|-------|--------|-------------|-------------|
| `tail_session_output` | `session_id`, `lines?=200`, `bytes?=131072` | Text (raw terminal bytes, UTF-8 lossy) | `GET /api/sessions/{id}/output` (new endpoint, reads ring buffer) | RO, idempotent, !openWorld |
| `tail_session_output_live` *(v1.12)* | `session_id`, `duration_seconds?=30`, `lines?` | Text + `notifications/progress` chunks | WS attach to existing session broadcast | RO, idempotent, !openWorld |

### Project / Workspace / Agent / Settings (5 tools)

| Tool | Input | Output | HTTP Bridge | Annotations |
|------|-------|--------|-------------|-------------|
| `list_projects` | `workspace_id?` | Project[] | `GET /api/projects` | RO, idempotent, !openWorld |
| `get_project` | `project_id` | Project | `GET /api/projects/{id}` | RO, idempotent, !openWorld |
| `list_workspaces` | — | Workspace[] | `GET /api/workspaces` | RO, idempotent, !openWorld |
| `list_agents` | — | Agent[] | `GET /api/agents` | RO, idempotent, !openWorld |
| `get_settings` | — | Settings | `GET /api/settings` | RO, idempotent, !openWorld |

### GitHub PR Review (2 tools)

| Tool | Input | Output | HTTP Bridge | Annotations |
|------|-------|--------|-------------|-------------|
| `list_pr_reviews` | `project_id` | PR[] | `GET /api/projects/{id}/pull-requests` | RO, idempotent, !openWorld |
| `get_pr_review` | `project_id`, `pr_number` | Review workspace Task | `POST /api/projects/{id}/pull-requests/{n}/review` | !RO, !destructive, idempotent, !openWorld |

### Diff & Worktree (5 tools)

| Tool | Input | Output | HTTP Bridge | Annotations |
|------|-------|--------|-------------|-------------|
| `get_task_diff` | `task_id` | Text (unified diff) | `GET /api/tasks/{id}/diff` | RO, idempotent, !openWorld |
| `mark_file_viewed` | `task_id`, `file_path`, `viewed` | `{updated: true}` | `POST /api/tasks/{id}/diff/viewed` | !RO, !destructive, idempotent, !openWorld |
| `list_worktree_cleanup_candidates` | — | CleanupCandidate[] | `GET /api/worktrees` | RO, idempotent, !openWorld |
| `clean_worktree_eligible` | — | `{removed: N}` | `POST /api/worktrees/clean-eligible` | !RO, destructive, idempotent, !openWorld |
| `remove_worktree` | `worktree_path`, `force?=false` | `{removed: true}` | `POST /api/worktrees/remove` | !RO, destructive, !idempotent, !openWorld |

### Scoped Variants (auto-injected when `KAMACU_SESSION_ID` set)

These appear *in addition to* the generic tools above when the MCP server detects the env var. They have simpler schemas (no `project_id`/`session_id` arg).

| Tool | Input | Output | Maps to |
|------|-------|--------|---------|
| `list_my_tasks` | `status?` | Task[] | `list_tasks(project_id=<from session>)` |
| `get_my_task` | — | Task | `get_task(task_id=<from session>)` |
| `get_my_session` | — | Session | `get_session(session_id=$KAMACU_SESSION_ID)` |
| `tail_my_output` | `lines?` | Text | `tail_session_output($KAMACU_SESSION_ID)` |
| `get_my_brief` *(v1.12)* | — | Brief (composite) | composite of get_task + get_project + list_tasks(siblings) |

**Total v1.11: 23 generic tools + 4 scoped variants + 2 spawn integrations = 29 surfaces.** Matches the milestone's ~30 target.

---

## Sources

- [MCP Specification 2025-06-18 — Base Protocol (lifecycle, messages, auth)](https://modelcontextprotocol.io/specification/2025-06-18/basic/index) — HIGH (official spec)
- [MCP Specification 2025-06-18 — Server Tools](https://modelcontextprotocol.io/specification/2025-06-18/server/tools) — HIGH (official spec; tools/list, tools/call, ToolAnnotations, structuredContent, outputSchema, two-class error handling)
- [MCP Specification 2025-06-18 — Server Resources](https://modelcontextprotocol.io/specification/2025-06-18/server/resources) — HIGH (official spec; resources/subscribe, notifications/resources/updated, URI schemes, annotations)
- [MCP Specification 2025-06-18 — Lifecycle](https://modelcontextprotocol.io/specification/2025-06-18/basic/lifecycle) — HIGH (initialize/initialized handshake, capability negotiation, stdio shutdown contract, timeouts)
- [`modelcontextprotocol/servers/src/everything`](https://github.com/modelcontextprotocol/servers/tree/main/src/everything) — HIGH (canonical reference; `docs/features.md` enumerates every protocol feature; `tools/trigger-long-running-operation.ts` is the canonical progress-notification pattern)
- [`modelcontextprotocol/servers/src/git/src/mcp_server_git/server.py`](https://github.com/modelcontextprotocol/servers/blob/main/src/git/src/mcp_server_git/server.py) — HIGH (closest CLI-wrapper analog; 12 verb-per-tool discipline; per-tool Pydantic schemas; per-tool `ToolAnnotations`; input injection defense; `validate_repo_path` defense-in-depth)
- [`mark3labs/mcp-go` README + source tree](https://github.com/mark3labs/mcp-go) — HIGH (the Go MCP framework; spec 2025-11-25 with back-compat to 2025-06-18; `server.NewMCPServer` + `server.ServeStdio` is the entire main; `mcp.NewTool` builder; `server.WithToolFilter` for per-session scoping; `mcp.WithTaskSupport` + `s.AddTaskTool` for SEP-1686 tasks; `server.WithHooks`, `server.WithRecovery`, `server.WithToolHandlerMiddleware` for observability)
- [opencode MCP servers docs](https://opencode.ai/docs/mcp-servers/) — HIGH (official; opencode.json `mcp` key with `type: "local"` + `command: [array]` shape; `environment`, `cwd`, `enabled`, `timeout` options; per-agent tool glob filters; `{env:VAR}` interpolation)
- Web search corroboration on Claude Code `mcpServers` config (klymentiev.com 2026 guide, claudelog.com configuration guide, github.com/anthropics/claude-code/issues/5037, modelcontextprotocol discussions #681) — MEDIUM–HIGH (consistent: `~/.claude.json` → `mcpServers` with separate `command`+`args` fields; `--mcp-config` flag for one-shot; `settings.json` mcpServers silently ignored)
- MCP spec authorization note: "implementations using STDIO transport SHOULD NOT follow [the HTTP Authorization spec], and instead retrieve credentials from the environment" — HIGH (official; confirms `KAMACU_HOOK_TOKEN` env pattern is spec-compliant)

---
*Feature research for: MCP (Model Context Protocol) server capability added to an existing local-only Go + React single-binary app — Kamacu v1.11*
*Researched: 2026-07-21*
