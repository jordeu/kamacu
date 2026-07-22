# Phase 07: Tasks, Projects & Workspaces Tools - Context

**Gathered:** 2026-07-22
**Status:** Ready for planning

<domain>
## Phase Boundary

The mechanical bulk of v1.11: ship the 12 remaining task/project/workspace MCP tools by repeating Phase 06's bridge pattern verbatim. Together with Phase 06's `list_projects`, this completes the 13-tool CRUD surface that lets an agent inside a Kamacu task PTY fully drive Kamacu's task, project, and workspace surface — every action the browser-attached user can do via the SPA, the agent can do via a tool.

**In this phase (12 new tools + 2 small endpoint additions to Kamacu):**

1. **6 task tools** — `list_tasks(project_id?)`, `get_task(task_id)`, `create_task(project_id, title, description?)`, `update_task(task_id, title?, description?)`, `move_task(task_id, status)`, `delete_task(task_id)`
2. **4 new project tools** — `get_project(project_id)`, `create_project(name, repo_path?, repo?, workspace_id?)`, `update_project(project_id, name?, description?, github_repo?, icon_letters?, icon_color?)`, `delete_project(project_id)`. (`list_projects` is FINAL production code inherited unchanged from Phase 06 — D-09.)
3. **4 workspace tools + 1 transfer tool** — `list_workspaces`, `create_workspace(name)`, `update_workspace(workspace_id, name)`, `delete_workspace(workspace_id)`, `move_project_to_workspace(project_id, workspace_id)`
4. **2 small Kamacu endpoint additions** (the only internal/api/ touches in this phase):
   - `GET /api/tasks` (unscoped; new) — backs `list_tasks` when `project_id` is omitted. Mirrors `listByProject`'s SELECT with the `WHERE project_id = ?` clause dropped; keeps the `source = 'manual'` filter (PR reviews stay off the board per GHREV-04) and the `ORDER BY status, position ASC` ordering.
   - `?workspace_id=N` query param on `GET /api/projects` (existing; extended) — backs `list_projects(workspace_id?)`. Backward-compatible: no query param = return all (current behavior; SPA and Phase 06's `list_projects` tool unchanged).

**Out of this phase:**

- The session/terminal tools (`list_sessions`, `get_session`, `get_session_output`, `subscribe_session_output`) — Phase 08 (MCPSESS × 4). Phase 07 handlers have minimal panic surface area; SDK panic-recovery wrapping is deferred to Phase 08 when streaming arrives (per 06-RESEARCH Open Question 1).
- The PR-review tools (`list_reviews`, `open_review`, `get_review_diff`) — Phase 09 (MCPREV × 3).
- Per-task auto-scoping (`get_my_task`, ToolFilter rewriting the visible tool list per session) — MCPAUTO-01..03, deferred to v1.12. Agents pass explicit IDs in v1.11.
- Agent-CLI auto-registration of `kamacu mcp serve` at spawn — MCPREG-01..03, deferred to v1.12. Users manually add the line.
- Typed JSON-RPC error taxonomy, full stdout-pollution guards (os.Stdout redirect + CI grep check), real-binary e2e harness — MCPHARD-01..03, deferred to v1.12.
- Server-side token validation middleware on general `/api/*` routes — explicitly out of scope for v1.11 (Phase 06 D-06); loopback binding remains the auth boundary.
- Additional tool categories (Agents CRUD, Settings get/update, Worktree cleanup ops) — MCPMORE-01..03, deferred to v1.12.
- Agent reassignment via `update_project` — the underlying PATCH accepts `agent_id`, but the MCP tool does NOT expose it (Out-of-Scope per MCPMORE-01; agent changes belong to a future agents-tools phase).
- PTY keystroke injection via MCP — permanently out of scope per REQUIREMENTS.md.

**Requirements covered:** MCPTASK-01, MCPTASK-02, MCPTASK-03, MCPTASK-04, MCPTASK-05, MCPTASK-06, MCPPROJ-01, MCPPROJ-02, MCPPROJ-03, MCPPROJ-04, MCPPROJ-05, MCPPROJ-06, MCPPROJ-07.

</domain>

<decisions>
## Implementation Decisions

### Endpoint gaps (D-01..D-02)

- **D-01:** **`list_tasks(project_id?)` resolves the missing global task list by adding `GET /api/tasks` to Kamacu.** The Kamacu API has only `GET /api/projects/{id}/tasks` (project-scoped); the REQUIREMENTS signature `list_tasks(project_id?)` marks the arg as optional. Phase 07 ADDS an unscoped `GET /api/tasks` endpoint to `internal/api/tasks.go` (route in `internal/api/routes.go`) that mirrors `listByProject`'s SELECT with the `WHERE project_id = ?` clause dropped, keeps the `source = 'manual'` filter (PR reviews stay off the board — GHREV-04), and the `ORDER BY status, position ASC` ordering. **Bridge behavior:** when `project_id` is supplied → `GET /api/projects/{id}/tasks` (existing, untouched); when `project_id` is omitted → `GET /api/tasks` (new). The scoped route stays the source of truth for the board; the new route is the unscoped fallback only.
- **D-02:** **`list_projects(workspace_id?)` adds a backward-compatible `?workspace_id=N` query param to the existing `GET /api/projects` endpoint.** ~5-line extension in `projectHandlers.list`: read `r.URL.Query().Get("workspace_id")`; when non-empty and parses as int, append `WHERE workspace_id = ?` to the SELECT. No query param = current behavior (all projects, ORDER BY name COLLATE NOCASE). SPA and Phase 06's `list_projects` tool are byte-for-byte unchanged (they don't send the query param). **Bridge behavior:** when `workspace_id` is supplied → `GET /api/projects?workspace_id=N`; when omitted → `GET /api/projects`. Symmetric with D-01.

### MCP argument shapes (D-03..D-04)

- **D-03:** **`update_*` tools expose PATCH fields as flat optional top-level args** — mirrors Kamacu's PATCH body shape exactly, no nested `updates` object.
  - `update_task`: `{task_id: int (required), title?: string, description?: string}`. Bridge sends the PATCH body `{title?, description?}` (omits nil fields — matches Kamacu's partial-PATCH semantics).
  - `update_project`: `{project_id: int (required), name?: string, description?: string (≤280), github_repo?: string (empty string unlinks → NULL; non-empty undergoes gh validation), icon_letters?: string (≤2 upper alnum), icon_color?: string (curated-palette member)}`. **Excludes `workspace_id` and `agent_id`** — workspace transfer has its own tool (`move_project_to_workspace`, MCPPROJ-07); agent reassignment is Out-of-Scope for v1.11 (MCPMORE-01). The bridge sends only the supplied fields; Kamacu's existing validation gates (validateIconLetters, validateIconColor, gh canonicalization, 280-cap, etc.) apply unchanged.
  - `update_workspace`: `{workspace_id: int (required), name?: string}`. Single-field rename; matches the underlying PATCH shape.
- **D-04:** **`create_project` exposes two distinct optional args — `repo_path?` and `repo?` — mirroring the underlying POST fork exactly.** Schema: `{name: string (required), repo_path?: string (absolute path → folder project), repo?: string (owner/name → v1.4 managed-checkout atomic create), workspace_id?: int (default Personal)}`. The bridge sends `{name, repo_path, repo, workspace_id}` as-is to `POST /api/projects`. **The handler's existing fork does the dispatch** (`string.TrimSpace(req.Repo) != ""` → `createByRepo` → `gh` validation + `gh repo clone` + atomic INSERT; else → folder-path validation + INSERT). Zero bridge-side type detection; zero new dispatch logic; one-to-one mapping with the underlying API.

### Error response shape (D-05)

- **D-05:** **Keep Phase 06's wrap-as-single-error-string pattern.** Non-200 responses from Kamacu are wrapped as `fmt.Errorf("kamacu %s %s: HTTP %d: %s", method, path, code, body)` — where `body` is Kamacu's raw JSON `{"status":N,"message":"..."}` string. The SDK wraps this as JSON-RPC `error.code = -32603` with the string in the message field. Agents see something like `"kamacu POST /api/workspaces: HTTP 409: {\"status\":409,\"message\":\"a workspace with that name already exists\"}"`. The Kamacu `message` is preserved verbatim — actionable. No bridge-side JSON parsing, no `IsError` result plumbing, no SC2 regression churn. Consistent with Phase 06's bridge; the full MCPHARD-01 typed-error taxonomy is deferred to v1.12.

### move_task position semantics (D-06)

- **D-06:** **`move_task` always sends `after_id: null` (lands at the top of the target column).** The MCP signature is `move_task(task_id, status)` with NO `after_id` arg. The bridge sends `{status, after_id: null}` to `POST /api/tasks/{id}/move`; the handler's existing `nextPosition` logic treats null `after_id` as "top of column" (`MIN(position) - 1.0`). Agents get deterministic, predictable behavior — no need to reason about board ordering or know other tasks' IDs. Preserves the existing /move handler byte-for-byte; the position math (renumbering, float positions) is unchanged.

### File organization & test breadth (D-07..D-08)

- **D-07:** **Split `internal/mcp/` per resource, mirroring `internal/api/`'s pattern.** New files: `tasks.go`, `projects.go`, `workspaces.go` under `internal/mcp/`. Each owns its `*bridge` methods (e.g., `bridge.listTasks`, `bridge.createTask`, `bridge.deleteTask`) and a `register<Task|Project|Workspace>Tools(s, b)` function called once from `registerTools` in `server.go`. `bridge.go` keeps the shared primitives (`bridge struct`, `newBridgeFromEnv`, `bridge.do`, `maxBodyBytes`, `defaultBase`). `server.go` shrinks to `ServeCommand` + a thin `registerTools` that delegates to the per-resource registrars. `list_projects` (Phase 06's tool) moves into `projects.go` alongside the new project tools — single ownership of the project resource. Test files mirror the split: `tasks_test.go`, `projects_test.go`, `workspaces_test.go` (alongside Phase 06's `bridge_test.go` and `server_test.go`).
- **D-08:** **One representative test per tool, per resource.** Each resource's test file covers every tool in that resource with a happy path + one representative error case (e.g., bad project_id → Kamacu 404, illegal workspace name → Kamacu 400, transport failure → wrapped error). Approximate case count: tasks ~12, projects ~10, workspaces ~8 — total ~30 new cases. Tests use the existing `httptest` pattern (Phase 06's `bridge_test.go` shape): spin up an `httptest.Server` with the relevant Kamacu routes registered, point the bridge at it, exercise the tool handler, assert the TextContent + error wrapping. The SC2 regression (`TestSC2_*`, 2 sub-tests) and the env-default/transport tests in `bridge_test.go` are unchanged.

### the agent's Discretion

- **Bridge method signatures** — exact function shapes (`func (b *bridge) listTasks(ctx context.Context, projectID *int64) (*mcp.CallToolResult, error)` vs taking a typed struct) are the agent's call. The `map[string]any` schema → typed Go extraction pattern in each handler is mechanical.
- **Exact InputSchema map shape per tool** — the property maps, descriptions, and required-arrays. Phase 06's `list_projects` shape (flat `map[string]any` with `type: object` + `properties` + per-field descriptions) is the template. No user preference on description copy.
- **Whether the bridge has a shared "callJSON helper"** — a `func (b *bridge) doJSON(ctx, method, path, in, out) error` that marshals the body, calls `do`, unmarshals the response, vs each tool handler repeating the `io.ReadAll(LimitReader) + TextContent` pattern inline. Phase 06 inlined it; Phase 07's volume might justify a helper. the agent's call.
- **Test naming convention** — `TestBridge_<Verb><Resource>_<Scenario>` (Phase 06's shape) vs `TestTasks_Create_Happy` etc. Follow whichever is consistent with the existing tests.
- **GET /api/tasks handler naming** — `taskHandlers.list` (parallel to `listByProject`) vs `listAll` vs some other name. Pick whatever reads cleanest.
- **`?workspace_id=N` query param parsing strictness** — empty string → ignore (return all); non-numeric → 400 or silently ignore. The Kamacu API convention is to silently ignore unknown/empty query params; follow that.
- **`move_task` schema description copy** — how to phrase "lands at the top of the target column" to agents. the agent's wordsmiths.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & roadmap (scope anchors)
- `.planning/ROADMAP.md` § "Phase 07: Tasks, Projects & Workspaces Tools" — the goal and the **4 success criteria** (six task tools drive the board; five project tools drive the sidebar with v1.4 managed-create/gated-delete; four workspace tools + transfer drive the switcher with v1.9 guarded delete; invalid input surfaces actionable MCP errors with no Kamacu state change).
- `.planning/REQUIREMENTS.md` § "Task Management (MCPTASK)" lines 16–23 — **MCPTASK-01..06** signatures locked (incl. the optional `project_id?` on `list_tasks` and `description?` on `create_task`/`update_task`).
- `.planning/REQUIREMENTS.md` § "Projects & Workspaces (MCPPROJ)" lines 38–46 — **MCPPROJ-01..07** signatures locked (incl. `create_project(name, repo_path_or_github_url, workspace_id?)` — resolved by D-04 to two distinct optional args; `update_project` partial-PATCH; `move_project_to_workspace` reusing v1.9 PATCH).
- `.planning/REQUIREMENTS.md` § "Out of Scope" + "v1.12+ Requirements" — MCPAUTO/MCPREG/MCPHARD/MCPMORE all intentionally NOT being built in v1.11. Read before planning so none of these leak in.
- `.planning/PROJECT.md` § "Active: v1.11 Kamacu MCP Server" — milestone goal; the "Backend-only milestone: no DB schema changes, no migrations, no frontend changes, no new long-lived goroutines inside the Kamacu binary" rule (D-01/D-02's endpoint additions are pure SQL + route additions — no schema change, no migration, no goroutine).

### Phase 06 foundation (the pattern Phase 07 repeats verbatim)
- `.planning/phases/06-mcp-subcommand-foundation/06-CONTEXT.md` — **the bridge pattern is fully locked here**. Read D-06..D-09 (proof-tool selection + bridge shape) and the `<code_context>` section. Phase 07 inherits `list_projects` unchanged (D-09 FINAL); the `*bridge` injection-via-closure pattern; the `map[string]any` schema choice over the typed generic `AddTool[In,Out]`; the raw TextContent passthrough; the 1 MiB `maxBodyBytes` cap; the 10s `http.Client.Timeout` (no retry).
- `.planning/phases/06-mcp-subcommand-foundation/06-02-SUMMARY.md` — what actually shipped in Plan 02. Documents the two API-drift auto-fixes (Tool.InputSchema is `any` not `json.RawMessage`; `cdr.HelpCommand()` is the method form). Phase 07 plans must respect both — the planner/executor will hit the same SDK shapes.
- `.planning/phases/06-mcp-subcommand-foundation/06-RESEARCH.md` § "Open Question 1" — confirms panic-recovery wrapping is deferred to Phase 08 (when `subscribe_session_output` introduces streaming panic surface area). Phase 07 handlers do NOT need `defer recover()`.

### Bridge pattern (the code Phase 07 extends)
- `internal/mcp/bridge.go` — the per-process HTTP client + shared primitives. `newBridgeFromEnv`, `bridge.do(ctx, method, path, body)`, the `X-Kamacu-Token` header set on every call, the 1 MiB `LimitReader`, the 10s timeout. Every Phase 07 tool handler starts from this pattern.
- `internal/mcp/bridge.go:74-99` (`bridge.listProjects`) — the canonical tool-handler shape Phase 07 repeats: call `b.do(ctx, method, path, nil-or-reader)` → `defer resp.Body.Close()` → `io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))` → non-200 wrap → 200 TextContent passthrough.
- `internal/mcp/server.go:72-130` (`registerTools`) — where Phase 07's new tools slot in. D-07 splits this into per-resource registrars; the single `s.AddTool` for `list_projects` moves into `projects.go`'s `registerProjectTools`.
- `internal/mcp/bridge_test.go` — the `httptest`-based test pattern Phase 07's three new test files mirror. Read all four tests (`TestBridge_ListProjects_PassesThroughKamacuJSON`, `_KamacuReturnsNon200_ReturnsError`, `_KamacuUnreachable_ReturnsError`, `TestNewBridgeFromEnv_DefaultsAndOverride`).
- `internal/mcp/server_test.go` — the SC2 regression pattern. Phase 07 must NOT regress this; the two sub-tests stay green.

### Kamacu endpoints Phase 07 bridges to
- `internal/api/routes.go:32-43` — the task and project routes. Phase 07 bridges all of these.
- `internal/api/routes.go:22-25` — the workspace routes. Phase 07 bridges all four.
- `internal/api/tasks.go:183-272` — `taskHandlers.listByProject` (the SELECT pattern `listAll` mirrors with the WHERE dropped) and `taskHandlers.create` (project_id from path; auto-provisions worktree+branch on create; `source='manual'` filter; ALWAYS status='todo' on create).
- `internal/api/tasks.go:275-346` — `taskHandlers.get` and `taskHandlers.update` (the partial-PATCH path that `update_task` mirrors).
- `internal/api/tasks.go:347-552` — `taskHandlers.move` (the position math — `afterPosition`/`nextPosition`/`renumberColumn` — that D-06's `after_id=null` triggers). Read to understand `nextPosition`'s null-after_id path (top of column).
- `internal/api/tasks.go:553-593` — `taskHandlers.delete` (the gated cleanup that runs on `delete_task`).
- `internal/api/projects.go:127-148` — `projectHandlers.list` (where D-02's `?workspace_id=N` query param lands).
- `internal/api/projects.go:163-245` — `projectHandlers.create` (the folder-vs-repo fork that D-04's two-arg shape maps to 1:1).
- `internal/api/projects.go:455-604` — `projectHandlers.update` (the partial-PATCH with all the validation gates `update_project` surfaces).
- `internal/api/projects.go:671-822` — `projectHandlers.delete` + `deleteManaged` (the v1.4 all-or-nothing gated delete that `delete_project` triggers).
- `internal/api/workspaces.go:77-232` — `workspaceHandlers.{list,create,update,delete}` (the v1.9 surface `list_workspaces`/`create_workspace`/`update_workspace`/`delete_workspace` mirror; note `delete`'s two guards: `is_default` non-deletable + non-empty `COUNT` block).

### Established project patterns (constraints, not files)
- **`internal/*` per-resource package convention** — every domain has its own package; `internal/mcp/` mirrors `internal/api/`'s per-resource split (D-07).
- **`httptest`-based integration tests** — every API package uses `httptest.NewServer` + real HTTP roundtrips; Phase 07's three new test files mirror Phase 06's `bridge_test.go` shape.
- **`slog` to stderr** — every package uses `log/slog` structured logging to stderr; the MCP subcommand already pins this in Plan 02 (D-12(a)). Phase 07 handlers add no new logging concerns.
- **Single-writer SQLite discipline** — Phase 07 bridge code must NOT import `internal/store` directly; it goes through the HTTP API like every external client. The two endpoint additions (D-01/D-02) ARE inside `internal/api/` and use the existing `*sql.DB` like every other handler.

No external ADRs/specs — requirements are fully captured in the decisions above plus these in-repo references.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`internal/mcp/bridge.go` `*bridge`** — the per-process HTTP client every Phase 07 tool handler closes over. `bridge.do(ctx, method, path, body)` is the single entry point; `X-Kamacu-Token` header set on every call; 1 MiB response cap; 10s timeout.
- **`internal/mcp/bridge.go:74-99` `bridge.listProjects`** — the canonical 25-line tool-handler template. Phase 07's 12 new handlers clone this shape: `b.do` → `defer Close` → `io.ReadAll(LimitReader)` → non-200 wrap → 200 TextContent passthrough.
- **`internal/mcp/server.go:81-129` `registerTools` + the single `s.AddTool` call** — the registration shape. D-07 splits per resource; the `map[string]any` InputSchema pattern (not typed generic `AddTool[In,Out]`) is locked from Phase 06.
- **`internal/api/tasks.go:183` `taskHandlers.listByProject`** — the SELECT template for D-01's new `listAll` handler. Drop the `WHERE project_id = ?` clause; keep everything else (`source = 'manual'`, `ORDER BY status, position ASC`).
- **`internal/api/projects.go:127` `projectHandlers.list`** — where D-02's `?workspace_id=N` query param is read. ~5-line addition: `r.URL.Query().Get("workspace_id")` → parse int → conditionally append `WHERE workspace_id = ?`.
- **`internal/mcp/bridge_test.go`** — the `httptest.NewServer` + `mux.HandleFunc` + `bridge.do` roundtrip pattern. Phase 07's three new test files clone this shape per resource.

### Established Patterns
- **Bridge pattern (locked in Phase 06)** — env parse once at startup → `*bridge` HTTP client with `X-Kamacu-Token` header → call Kamacu endpoint → raw TextContent passthrough. Every Phase 07 tool follows this shape; no tool re-implements HTTP client construction.
- **Per-resource file split** — `internal/api/{tasks.go, projects.go, workspaces.go, workspaces.go}` is the established layout. D-07 mirrors it in `internal/mcp/{tasks.go, projects.go, workspaces.go}`.
- **Partial-PATCH via optional pointer fields** — Kamacu's update handlers accept `*string` / `*int64` fields; `nil` means "leave untouched", supplied means "update to this value". Phase 07's `update_*` MCP schemas model the same shape (D-03).
- **Two-path create fork (`repo` vs `repo_path`)** — Kamacu's `POST /api/projects` already dispatches on `string.TrimSpace(req.Repo) != ""`. D-04's two-arg MCP shape maps to this 1:1 — zero bridge-side dispatch logic.
- **`source='manual'` filter** — every board query filters out PR-review tasks (GHREV-04). D-01's new `GET /api/tasks` endpoint keeps this filter.
- **`map[string]any` InputSchema** — avoids the SDK's typed-generic `AddTool[In,Out]` helper (06-RESEARCH Anti-Patterns). Phase 07 inherits this verbatim.

### Integration Points
- **New `GET /api/tasks` route** — register in `internal/api/routes.go` (one `mux.HandleFunc` line). Handler `taskHandlers.listAll` (or similar name) in `internal/api/tasks.go`. Bridge's `listTasks` calls this when `project_id` is omitted; calls `GET /api/projects/{id}/tasks` when supplied.
- **`?workspace_id=N` query param on `GET /api/projects`** — modify `projectHandlers.list` in `internal/api/projects.go:127-148`. Read query param; conditionally add WHERE clause. Bridge's `listProjects(workspace_id?)` sends the query param when supplied.
- **Per-resource registrars in `internal/mcp/`** — `registerTaskTools(s, b)`, `registerProjectTools(s, b)`, `registerWorkspaceTools(s, b)` called from `server.go`'s `registerTools`. Phase 06's `list_projects` AddTool call moves into `registerProjectTools` (single ownership).
- **Bridge methods on `*bridge`** — `listTasks`, `getTask`, `createTask`, `updateTask`, `moveTask`, `deleteTask`, `getProject`, `createProject`, `updateProject`, `deleteProject`, `listWorkspaces`, `createWorkspace`, `updateWorkspace`, `deleteWorkspace`, `moveProjectToWorkspace`. Each follows the `listProjects` shape (25-ish lines).
- **Three new test files** — `internal/mcp/tasks_test.go`, `internal/mcp/projects_test.go`, `internal/mcp/workspaces_test.go`. Each covers its resource's tools with happy + error cases (D-08).
- **`cmd/kamacu/main.go` is NOT modified** — Phase 06 already registers `mcpCmd`; Phase 07 only adds tools inside `internal/mcp/`. (Phase 06 D-05: no future-subcommand stubs; Phase 07 adds none.)

</code_context>

<specifics>
## Specific Ideas

- **This phase is the bridge-pattern stress test.** Phase 06 proved the pattern with ONE tool; Phase 07 proves it scales to 13. If any tool needs meaningfully different bridge logic, that's a signal the pattern needs revisiting — surface it to the user, don't silently diverge.
- **The two Kamacu endpoint additions (D-01/D-02) are the only non-`internal/mcp/` code touched.** Everything else is bridge methods + AddTool registrations inside `internal/mcp/`. The v1.11 milestone rule "Backend-only milestone: no DB schema changes, no migrations, no frontend changes, no new long-lived goroutines" is preserved — D-01/D-02 are pure SELECT additions and route registrations.
- **Agents are the primary user.** Tool descriptions should be agent-readable: lead with what the tool does, follow with the Kamacu endpoint it bridges to (for transparency). Phase 06's `list_projects` description is the template ("List all Kamacu projects. Returns the raw JSON array from GET /api/projects. The optional project_id is currently a no-op...").
- **No new long-lived goroutines.** Phase 07's tools are all request/response HTTP calls — no streaming, no polling, no background work. (Streaming arrives in Phase 08; the goroutine-leak concern is documented there.)
- **`list_projects` ownership moves.** Phase 06's `list_projects` AddTool call (currently in `server.go`'s `registerTools`) moves into `projects.go`'s `registerProjectTools` in Phase 07. The tool itself is unchanged — single ownership of the project resource across all 5 project tools.
- **Validation is the existing Kamacu handlers' job, not the bridge's.** D-03/D-04 specifically choose "send fields as-is, let Kamacu validate" over "validate in the bridge". This means an invalid `icon_color` (off-palette) → Kamacu returns 400 → bridge wraps as the error string → agent sees the message. SC4 ("every existing validation gate applies unchanged through the bridge") is satisfied by NOT re-implementing validation in the bridge.
</specifics>

<deferred>
## Deferred Ideas

- **MCPAUTO-01..03** — per-task auto-scoping (`get_my_task`, `get_my_session`, ToolFilter rewriting the visible tool list per session). Useful DX; agents pass explicit IDs in v1.11. Deferred to v1.12.
- **MCPREG-01..03** — auto-registration of `kamacu mcp serve` with agent CLIs at spawn time. v1.11 users manually add it once. Deferred to v1.12.
- **MCPMORE-01..03** — additional tool categories: Agents CRUD (`create_agent`/`update_agent`/`delete_agent`/`set_default_agent`), Settings get/update, Worktree cleanup ops. Out of scope for v1.11; deferred to v1.12. The `agent_id` field is intentionally omitted from `update_project` (D-03) until the agents-tools phase ships.
- **MCPHARD-01** — typed JSON-RPC error taxonomy for the 6 bridge failure modes (`kamacu_down` / `port_mismatch` / `token_rejected` / `route_404` / `timeout` / `too_large`). Phase 07 surfaces generic MCP errors via the D-05 wrap; the typed taxonomy is v1.12 work.
- **MCPHARD-02/03** — full stdout-pollution guards and real-binary e2e harness. Deferred to v1.12.
- **`update_project` exposing `agent_id`** — the underlying PATCH accepts `agent_id` (changes the project's assigned agent), but the MCP tool excludes it (D-03). Ships when the agents-tools phase (MCPMORE-01) lands.
- **Positional move for `move_task`** — D-06 sends `after_id: null` (top of column). Exposing `after_id` as an optional third arg is a future enhancement once agents have a reason to care about board ordering. Deferred indefinitely.
- **Pagination / filtering on `list_tasks` / `list_projects`** — D-01/D-02 add only project_id and workspace_id scoping. Status filter (`?status=in_progress`), since-cursor, result caps are NOT added. If a future phase needs them, they extend D-01/D-02's endpoints then.
- **Token auth on general `/api/*` routes** — server-side middleware validating `X-Kamacu-Token` on all `/api/*` calls. Explicitly NOT in v1.11 (Phase 06 D-06); loopback binding remains the boundary.

None of these were pulled into Phase 07 — discussion stayed within the MCPTASK-01..06 + MCPPROJ-01..07 boundary.
</deferred>

---

*Phase: 07-tasks-projects-workspaces-tools*
*Context gathered: 2026-07-22*
