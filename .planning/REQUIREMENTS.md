# Requirements: Kamacu v1.11 Kamacu MCP Server

**Defined:** 2026-07-21
**Core Value:** One place to see and drive all agent work — every task gets its own isolated worktree and a persistent agent session you can open, leave, and reattach to from the browser.

**Milestone goal:** Expose Kamacu's task / session / review / project surface as MCP tools so an agent running inside a Kamacu task PTY can drive the app as the user's delegate. Read + subscribe only on terminals (no PTY keystroke injection — write stays browser-only).

## v1.11 Requirements

### Process Foundation (MCPPROC)

- [x] **MCPPROC-01**: `kamacu mcp serve` stdio subcommand starts an MCP server using `github.com/modelcontextprotocol/go-sdk` (v1.6.1) and bridges tool calls to the running Kamacu HTTP API at `127.0.0.1:7333`
- [x] **MCPPROC-02**: MCP subcommand authenticates to the Kamacu HTTP API via `KAMACU_HOOK_TOKEN` env (reuses the existing envelope — already injected into spawned agents)
- [x] **MCPPROC-03**: MCP subcommand locates the Kamacu HTTP base URL via `KAMACU_HOOK_BASE` env (already injected) with a `127.0.0.1:7333` default

### Task Management (MCPTASK)

- [x] **MCPTASK-01**: `list_tasks(project_id?)` returns tasks, optionally scoped to a project
- [x] **MCPTASK-02**: `get_task(task_id)` returns full task detail (title, description, status, position, project)
- [x] **MCPTASK-03**: `create_task(project_id, title, description?)` creates a task — the existing surface auto-creates the worktree + branch; the agent is NOT auto-started (auto-start remains Out of Scope per PROJECT.md)
- [x] **MCPTASK-04**: `update_task(task_id, title?, description?)` partial-PATCH update
- [x] **MCPTASK-05**: `move_task(task_id, status)` moves a task between columns (To Do / In Progress / In Review / Done) via the existing `/move` surface
- [x] **MCPTASK-06**: `delete_task(task_id)` deletes a task (the existing gated cleanup runs)

### Sessions & Terminal (MCPSESS)

- [ ] **MCPSESS-01**: `list_sessions(project_id? or task_id?)` returns sessions with status (working / waiting / idle / exited / running)
- [ ] **MCPSESS-02**: `get_session(session_id)` returns full session detail (status, task, project, agent, started_at)
- [ ] **MCPSESS-03**: `get_session_output(session_id, bytes?)` returns a snapshot of the session's PTY ring buffer (last N bytes, default 4 KB) — read-only
- [ ] **MCPSESS-04**: `subscribe_session_output(session_id, duration_seconds?)` tails live PTY output for a bounded duration (default 30s, cap 300s); cancellation propagates via `ctx.Done()` → `Session.Detach`; no PTY keystroke injection (read-only)

### PR Review (MCPREV)

- [ ] **MCPREV-01**: `list_pending_reviews(project_id?)` lists PRs awaiting the user's review (`user-review-requested:@me draft:false` via existing v1.3 surface)
- [ ] **MCPREV-02**: `list_recently_reviewed(project_id?)` lists open PRs the user has already reviewed (`reviewed-by:@me draft:false` via existing v1.5 surface)
- [ ] **MCPREV-03**: `open_review(project_id, pr_number)` opens a PR as a review workspace (reuses v1.3 Phase 12 open-a-review — find-or-create `source='github_pr'` task)

### Projects & Workspaces (MCPPROJ)

- [x] **MCPPROJ-01**: `list_projects(workspace_id?)` returns projects, optionally scoped to a workspace
- [ ] **MCPPROJ-02**: `get_project(project_id)` returns full project detail (name, description, repo, workspace, agent, icon)
- [ ] **MCPPROJ-03**: `create_project(name, repo_path_or_github_url, workspace_id?)` creates a project — folder path or `owner/name` triggers the existing v1.4 managed-checkout surface
- [ ] **MCPPROJ-04**: `update_project(project_id, name?, description?, github_repo?, icon_letters?, icon_color?)` partial-PATCH update
- [ ] **MCPPROJ-05**: `delete_project(project_id)` deletes a project — the existing v1.4 gated delete runs (folder projects byte-for-byte unchanged; managed projects all-or-nothing gated)
- [ ] **MCPPROJ-06**: `list_workspaces` / `create_workspace(name)` / `update_workspace(workspace_id, name)` / `delete_workspace(workspace_id)` — workspace CRUD via the existing v1.9 surface (guarded delete keyed off `is_default` + non-empty `COUNT`)
- [ ] **MCPPROJ-07**: `move_project_to_workspace(project_id, workspace_id)` transfers a project to another workspace via the existing v1.9 `PATCH /api/projects/{id}` surface

## v1.12+ Requirements (Deferred from v1.11)

### Per-task Auto-scoping (MCPAUTO)

- **MCPAUTO-01**: `get_my_task` convenience tool — auto-resolves the calling agent's task from `KAMACU_SESSION_ID` → `tasks.claude_session_id` / `tasks.opencode_session_id` join, no params required
- **MCPAUTO-02**: `get_my_session` convenience tool — same auto-resolution for session
- **MCPAUTO-03**: Tool-filter pattern (e.g., `server.WithToolFilter`) rewrites the visible tool list per session so cross-task tools accept an implicit `task_id` default

### Agent-CLI Auto-registration (MCPREG)

- **MCPREG-01**: At task spawn, Kamacu writes a per-task `.mcp.json` at the worktree root for Claude Code (`mcpServers.kamacu` shape)
- **MCPREG-02**: At task spawn, Kamacu updates the per-task `opencode.json` `mcp` key for opencode (`mcp.kamacu` shape)
- **MCPREG-03**: Both writers use read-modify-write + flock + preserve-unknown-fields + skip-on-byte-identical + cleanup on task Done

### Additional Tool Categories (MCPMORE)

- **MCPMORE-01**: Agents CRUD (`list_agents` / `get_agent` / `create_agent` / `update_agent` / `delete_agent` / `set_default_agent`) — mirrors v1.10
- **MCPMORE-02**: Settings get + update (`get_settings` / `update_setting`) — mirrors the global Settings page
- **MCPMORE-03**: Worktree cleanup ops (`list_worktree_cleanup_candidates` / `force_remove_worktree` / `clean_eligible_worktrees`) — mirrors v1.8 Phase 23

### Quality / Hardening (MCPHARD)

- **MCPHARD-01**: Typed JSON-RPC error taxonomy for the 6 bridge failure modes (`kamacu_down` / `port_mismatch` / `token_rejected` / `route_404` / `timeout` / `too_large`) with actionable recovery actions
- **MCPHARD-02**: stdout-pollution guards — redirect `os.Stdout` from line zero, ban `fmt.Print*` from `internal/mcp/`, CI grep check
- **MCPHARD-03**: Real-binary e2e harness (`//go:build mcp_e2e`) — smallest real Claude Code + real opencode invocation that exercises `kamacu mcp serve` end-to-end without burdening default CI

## Out of Scope

| Feature | Reason |
|---------|--------|
| **PTY keystroke injection via MCP** | An agent observing a sibling session must never inject bytes into its PTY — would conflict with the browser-attached user and the other agent's intent. PTY write stays browser-only. |
| **MCP server for external AI editors (Claude Desktop / Cursor / VS Code)** | v1.11 targets agents running inside Kamacu. An external-editor MCP surface (different transport / auth / process model) is a separate future milestone. |
| **Auto-registration with agent CLIs at spawn time** | v1.11 ships the MCP server; users manually add `kamacu mcp serve` to their agent CLI's MCP config (one-time setup). Auto-registration is MCPREG-01..03, deferred to v1.12. |
| **Per-task auto-scoping of tools (`get_my_task`, ToolFilter)** | Useful DX but not load-bearing for v1.11 — agents pass explicit IDs. MCPAUTO-01..03 deferred to v1.12. |
| **Agents / Settings / Worktree-cleanup tool categories** | Out of scope for v1.11 to keep the tool surface tight. MCPMORE-01..03 deferred to v1.12. |
| **Typed bridge error taxonomy, stdout guards, real-binary e2e harness** | Recommended by research but not required for a working v1.11. MCPHARD-01..03 deferred to v1.12. |
| **Merge/PR write automation** | Inherits the existing Out-of-Scope rule — Kamacu never writes to GitHub (approve/request-changes/comment/merge done in terminal). `trigger_review` is column-move only. |
| **External task services, multi-user, remote deployment, desktop packaging** | Inherits existing Out-of-Scope rules. |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| MCPPROC-01 (`kamacu mcp serve` stdio MCP subcommand via official Go SDK, bridging to Kamacu HTTP API) | Phase 06 | Pending |
| MCPPROC-02 (authenticates to Kamacu HTTP API via `KAMACU_HOOK_TOKEN`) | Phase 06 | Pending |
| MCPPROC-03 (locates Kamacu HTTP base URL via `KAMACU_HOOK_BASE`, default `127.0.0.1:7333`) | Phase 06 | Pending |
| MCPTASK-01 (`list_tasks(project_id?)`) | Phase 07 | Pending |
| MCPTASK-02 (`get_task(task_id)`) | Phase 07 | Pending |
| MCPTASK-03 (`create_task(project_id, title, description?)` — worktree auto-created, agent NOT auto-started) | Phase 07 | Pending |
| MCPTASK-04 (`update_task(task_id, title?, description?)` partial-PATCH) | Phase 07 | Pending |
| MCPTASK-05 (`move_task(task_id, status)` via existing `/move`) | Phase 07 | Pending |
| MCPTASK-06 (`delete_task(task_id)` — gated cleanup runs) | Phase 07 | Pending |
| MCPSESS-01 (`list_sessions(project_id? or task_id?)` with status) | Phase 08 | Pending |
| MCPSESS-02 (`get_session(session_id)` full detail) | Phase 08 | Pending |
| MCPSESS-03 (`get_session_output(session_id, bytes?)` snapshot, read-only) | Phase 08 | Pending |
| MCPSESS-04 (`subscribe_session_output(session_id, duration_seconds?)` bounded live tail; cancellation via `ctx.Done()` → `Session.Detach`; read-only) | Phase 08 | Pending |
| MCPREV-01 (`list_pending_reviews(project_id?)` via v1.3 surface) | Phase 09 | Pending |
| MCPREV-02 (`list_recently_reviewed(project_id?)` via v1.5 surface) | Phase 09 | Pending |
| MCPREV-03 (`open_review(project_id, pr_number)` via v1.3 Phase 12 open-a-review) | Phase 09 | Pending |
| MCPPROJ-01 (`list_projects(workspace_id?)`) | Phase 07 | Pending |
| MCPPROJ-02 (`get_project(project_id)`) | Phase 07 | Pending |
| MCPPROJ-03 (`create_project(name, repo_path_or_github_url, workspace_id?)` — folder or v1.4 managed-checkout) | Phase 07 | Pending |
| MCPPROJ-04 (`update_project(project_id, …)` partial-PATCH) | Phase 07 | Pending |
| MCPPROJ-05 (`delete_project(project_id)` — v1.4 gated delete runs) | Phase 07 | Pending |
| MCPPROJ-06 (`list_workspaces` / `create_workspace` / `update_workspace` / `delete_workspace` via v1.9 surface) | Phase 07 | Pending |
| MCPPROJ-07 (`move_project_to_workspace(project_id, workspace_id)` via v1.9 PATCH surface) | Phase 07 | Pending |

**Coverage:**

- v1.11 requirements: 23 total (MCPPROC×3 + MCPTASK×6 + MCPSESS×4 + MCPREV×3 + MCPPROJ×7) — _the prior "21" figure was an arithmetic miscount; per-category sums to 23_
- Mapped to phases: 23/23 ✓
- Unmapped: 0
- Per-phase: Phase 06 = 3 (MCPPROC), Phase 07 = 13 (MCPTASK + MCPPROJ), Phase 08 = 4 (MCPSESS), Phase 09 = 3 (MCPREV)

---
*Requirements defined: 2026-07-21*
*Last updated: 2026-07-21 — traceability populated during roadmap creation (Phases 06–09)*
