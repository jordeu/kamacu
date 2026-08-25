# Phase 07: Tasks, Projects & Workspaces Tools - Research

**Researched:** 2026-07-22
**Domain:** MCP HTTP-bridge tools (Go) over existing Kamacu REST endpoints
**Confidence:** HIGH

## Landmines (read before planning)

These are load-bearing surprises the planner MUST accommodate. Each is detailed inline below.

1. **`get_project` (MCPPROJ-02) has NO existing Kamacu endpoint to bridge to.** There is no `GET /api/projects/{id}` route. The CONTEXT.md says "2 small Kamacu endpoint additions" but there are actually **3** — the third is a single-project fetch for `get_project`. See [Gap 1](#gap-1-get_project-needs-a-third-kamacu-endpoint-addition).

2. **D-05's error-body description is factually wrong about the JSON shape.** D-05 says Kamacu error bodies are `{"status":N,"message":"..."}`. The ACTUAL `writeError` produces `{"error": msg}` (verified in `internal/api/respond.go:16`). The managed-delete 409 is a richer `{"error":"...","reasons":[...]}`. The D-05 wrap pattern (`fmt.Errorf("... HTTP %d: %s", code, body)`) is unaffected because it passes `body` through verbatim as a string — but test assertions and the D-05 example string MUST use the real `{"error":"..."}` shape, not the described `{"status","message"}` shape. See [Gap 2](#gap-2-d-05-error-body-shape-description-is-wrong).

3. **Phase 06's `list_projects` tool registration physically MOVES files in this phase (D-07).** The `s.AddTool` call currently in `server.go:82-102` moves into the new `projects.go`'s `registerProjectTools`. This is a code-motion change to FINAL production code — the tool's behavior is byte-for-byte unchanged, but `server.go` shrinks and a new file owns it. The planner must sequence this so `registerTools` in `server.go` delegates to three per-resource registrars without breaking the SC2 test or the `list_projects` behavior. See [Pattern: per-resource split](#pattern-per-resource-file-split-d-07).

4. **`move_task` sends `after_id: null` — the handler treats this as "top of column" via a specific code path.** D-06 locks this, but the planner should read `tasks.go:393-403` to confirm the `MIN(position) - 1.0` math is the null-after_id branch (it is). The MCP tool has NO `after_id` arg; the bridge hardcodes `null`. See [move_task semantics](#move_task-position-semantics-d-06).

5. **`create_task` does NOT auto-start an agent (by design) but DOES synchronously provision a worktree (up to 30s).** The `provisionWorktree` call in `tasks.go:263` runs inline before the 201 response. The bridge's 10s `http.Client.Timeout` (locked in Phase 06) WILL fire before the 30s git timeout if git is slow — the agent sees a timeout error but the task + worktree MAY still get created server-side. This is a pre-existing characteristic of the endpoint, not a Phase 07 bug, but the planner should be aware the bridge's 10s timeout can truncate a slow create. See [create_task timeout interaction](#create_task-timeout-interaction).

<user_constraints>
## User Constraints (from 07-CONTEXT.md)

### Locked Decisions (D-01..D-08)

- **D-01:** Add `GET /api/tasks` (unscoped) to Kamacu. Mirrors `listByProject`'s SELECT with `WHERE project_id = ?` dropped; keeps `source = 'manual'` filter + `ORDER BY status, position ASC`. Bridge: `project_id` supplied → `GET /api/projects/{id}/tasks`; omitted → `GET /api/tasks`.
- **D-02:** Add backward-compatible `?workspace_id=N` query param on `GET /api/projects`. ~5-line extension. No param = current behavior. Bridge: `workspace_id` supplied → `GET /api/projects?workspace_id=N`; omitted → `GET /api/projects`.
- **D-03:** `update_*` tools expose PATCH fields as flat optional top-level args (no nested `updates` object). `update_task`: `{task_id, title?, description?}`. `update_project`: `{project_id, name?, description?, github_repo?, icon_letters?, icon_color?}` — **excludes `workspace_id` and `agent_id`**. `update_workspace`: `{workspace_id, name?}`.
- **D-04:** `create_project` exposes two distinct optional args — `repo_path?` (folder) and `repo?` (owner/name → managed checkout). The handler's existing fork dispatches; zero bridge-side type detection.
- **D-05:** Keep Phase 06's wrap-as-single-error-string pattern. Non-200 → `fmt.Errorf("kamacu %s %s: HTTP %d: %s", method, path, code, body)`. (See [Gap 2](#gap-2-d-05-error-body-shape-description-is-wrong) — the described body shape is wrong.)
- **D-06:** `move_task` always sends `after_id: null` (top of target column). Signature is `move_task(task_id, status)` with NO `after_id` arg.
- **D-07:** Split `internal/mcp/` per resource: `tasks.go`, `projects.go`, `workspaces.go`. Each owns its `*bridge` methods + a `register<Task|Project|Workspace>Tools(s, b)` function. `bridge.go` keeps shared primitives. `list_projects` moves into `projects.go`.
- **D-08:** One representative test per tool per resource (~30 new cases). httptest pattern mirroring `bridge_test.go`.

### the agent's Discretion
- Bridge method signatures (typed struct vs `map[string]any` extraction).
- Exact InputSchema map shape per tool (Phase 06's flat `map[string]any` is the template).
- Whether a shared `doJSON` helper is justified (Phase 06 inlined it).
- Test naming convention.
- `GET /api/tasks` handler naming (`list` vs `listAll`).
- `?workspace_id=N` query param parsing strictness (empty → ignore; non-numeric → ignore per Kamacu convention).
- `move_task` schema description copy.

### Deferred Ideas (OUT OF SCOPE)
- MCPAUTO-01..03 (per-task auto-scoping) — v1.12.
- MCPREG-01..03 (agent-CLI auto-registration) — v1.12.
- MCPMORE-01..03 (Agents/Settings/Worktree-cleanup tools) — v1.12. `update_project` excludes `agent_id`.
- MCPHARD-01..03 (typed errors, stdout guards, e2e harness) — v1.12.
- Session/terminal tools (MCPSESS × 4) — Phase 08.
- PR-review tools (MCPREV × 3) — Phase 09.
- Pagination/filtering beyond project_id/workspace_id scoping.
- Token auth middleware on `/api/*` — explicitly out (Phase 06 D-06).
- PTY keystroke injection — permanently out.

</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| MCPTASK-01 | `list_tasks(project_id?)` | D-01: needs NEW `GET /api/tasks` endpoint. Scoped → existing `GET /api/projects/{id}/tasks`. ✅ endpoint mapped |
| MCPTASK-02 | `get_task(task_id)` | Existing `GET /api/tasks/{id}` (`tasks.go:275`). ✅ no endpoint change |
| MCPTASK-03 | `create_task(project_id, title, description?)` | Existing `POST /api/projects/{id}/tasks` (`tasks.go:223`). Worktree auto-provisions; agent NOT auto-started. ✅ no endpoint change |
| MCPTASK-04 | `update_task(task_id, title?, description?)` partial-PATCH | Existing `PATCH /api/tasks/{id}` (`tasks.go:294`). D-03 flat optional args. ✅ no endpoint change |
| MCPTASK-05 | `move_task(task_id, status)` via `/move` | Existing `POST /api/tasks/{id}/move` (`tasks.go:347`). D-06 hardcodes `after_id: null`. ✅ no endpoint change |
| MCPTASK-06 | `delete_task(task_id)` gated cleanup | Existing `DELETE /api/tasks/{id}` (`tasks.go:553`). ✅ no endpoint change |
| MCPPROJ-01 | `list_projects(workspace_id?)` | D-02: `?workspace_id=N` query param on existing `GET /api/projects`. Phase 06's tool MOVES to `projects.go`. ✅ endpoint extended |
| MCPPROJ-02 | `get_project(project_id)` | **⚠️ NO existing endpoint.** Needs NEW `GET /api/projects/{id}`. See [Gap 1](#gap-1-get_project-needs-a-third-kamacu-endpoint-addition). |
| MCPPROJ-03 | `create_project(name, repo_path_or_github_url, workspace_id?)` | Existing `POST /api/projects` (`projects.go:163`). D-04 two-arg shape maps to existing fork. ✅ no endpoint change |
| MCPPROJ-04 | `update_project(project_id, …)` partial-PATCH | Existing `PATCH /api/projects/{id}` (`projects.go:455`). D-03 excludes `workspace_id`+`agent_id`. ✅ no endpoint change |
| MCPPROJ-05 | `delete_project(project_id)` v1.4 gated delete | Existing `DELETE /api/projects/{id}` (`projects.go:671`). ✅ no endpoint change |
| MCPPROJ-06 | workspace CRUD (list/create/update/delete) | Existing `GET/POST/PATCH/DELETE /api/workspaces` (`workspaces.go`). ✅ no endpoint change |
| MCPPROJ-07 | `move_project_to_workspace(project_id, workspace_id)` | Existing `PATCH /api/projects/{id}` with `workspace_id` field (`projects.go:557-575`). ✅ no endpoint change |

**Coverage:** 13/13 requirements mapped. **12 tools bridge to existing endpoints unchanged; 1 tool (`get_project`) needs a new endpoint.** Plus D-01 (`GET /api/tasks`) and D-02 (`?workspace_id=` query param).

</phase_requirements>

## Gaps Found (planner MUST address)

### Gap 1: `get_project` needs a THIRD Kamacu endpoint addition

**The problem:** MCPPROJ-02 requires `get_project(project_id)` returning "full project detail." But there is **no `GET /api/projects/{id}` route** in `internal/api/routes.go`. Verified: routes.go:32-37 registers `GET /api/projects`, `POST /api/projects`, `PATCH /api/projects/{id}`, `GET /api/projects/{id}/github-origin`, `DELETE /api/projects/{id}` — but no single-project GET.

The CONTEXT.md `<domain>` says "2 small Kamacu endpoint additions" (D-01 `GET /api/tasks` + D-02 `?workspace_id=` query param). It does not mention `get_project`'s missing endpoint. The `<canonical_refs>` section claims "Phase 07 bridges all of these" for routes.go:32-43, but a single-project GET isn't among them.

**Resolution options (planner's call, recommend option A):**
- **(A) Add `GET /api/projects/{id}` — a third endpoint addition.** Trivial: a `projectHandlers.get` method mirroring `taskHandlers.get` (`tasks.go:275-290`) — `SELECT projectColumns FROM projects WHERE id = ?`, 404 on missing. ~15 lines in `projects.go`, one `mux.HandleFunc` line in `routes.go`. Same "no schema change, no migration, no goroutine" character as D-01/D-02. **This is the correct resolution** — it matches `get_task`'s pattern and gives the agent a real single-project fetch.
- **(B) Implement `get_project` client-side** by calling `list_projects` and filtering. **Reject this** — it returns ALL projects to find one, violates the "thin bridge" principle, and breaks if pagination is ever added.

**Recommendation:** Treat this as a third D-01/D-02-style endpoint addition. The CONTEXT.md "2 small additions" becomes "3 small additions." Update the plan's wave structure accordingly. This is the single most important planning correction.

### Gap 2: D-05 error body shape description is wrong

**The problem:** D-05 states Kamacu error bodies are `{"status":N,"message":"..."}` and gives this example:
> `"kamacu POST /api/workspaces: HTTP 409: {\"status\":409,\"message\":\"a workspace with that name already exists\"}"`

The ACTUAL `writeError` (`internal/api/respond.go:16`) produces:
```go
func writeError(w http.ResponseWriter, status int, msg string) {
    writeJSON(w, status, map[string]string{"error": msg})
}
```
So the real body is `{"error":"a workspace with that name already exists"}` — **no `status` key, no `message` key, key is `error`.**

**Impact:** The D-05 wrap pattern itself is unaffected (`fmt.Errorf("...HTTP %d: %s", code, body)` passes `body` as a raw string — it doesn't parse JSON). But:
1. The D-05 example string is wrong — agents will actually see `...HTTP 409: {"error":"a workspace with that name already exists"}`.
2. **Test assertions MUST use the real shape.** If a test asserts the error string contains `"message"` it will fail; it should assert on the Kamacu message text (e.g. `"already exists"`) which survives in the raw body either way.
3. The managed-delete 409 is richer: `{"error":"the project can't be deleted yet","reasons":[{"kind":"uncommitted","target":"task #N"}]}` — still keyed on `error`, not `message`.

**Resolution:** The planner/executor uses the D-05 wrap pattern verbatim (it's shape-agnostic). Test assertions should assert on substrings of the Kamacu message text (e.g. `"already exists"`, `"name is required"`, `"the default workspace"`) rather than on JSON key names. No CONTEXT.md change needed for planning to proceed — just don't trust the `{"status","message"}` example.

## Summary

Phase 07 is the mechanical-bulk phase: ship 13 thin HTTP-bridge MCP tools by repeating Phase 06's proven pattern. Twelve of the thirteen tools bridge to existing Kamacu endpoints with zero endpoint changes; the thirteenth (`get_project`) needs a trivial new `GET /api/projects/{id}` route (Gap 1). Two small endpoint extensions (D-01 `GET /api/tasks`, D-02 `?workspace_id=` query param) plus the Gap 1 addition are the only `internal/api/` touches — all pure SELECT additions / route registrations, no schema change, no migration, no new goroutines (preserving the v1.11 milestone rule).

The technical surface is fully mapped. The bridge pattern is locked (Phase 06 shipped it with `list_projects` as FINAL production code). Each new tool is a ~25-line `bridge.<verb>` method cloning `bridge.listProjects`'s shape (`b.do` → `defer Close` → `io.ReadAll(LimitReader)` → non-200 wrap → 200 TextContent passthrough), plus one `s.AddTool` call with a `map[string]any` InputSchema. The only architectural decision is the per-resource file split (D-07), which is a code-motion refactor of `server.go`'s `registerTools` into three registrars.

**Primary recommendation:** Plan three waves — (1) the three Kamacu endpoint additions (`GET /api/tasks`, `?workspace_id=` param, `GET /api/projects/{id}`) since the bridge tools depend on them; (2) the per-resource bridge methods + registrars + test files (can be parallelized per resource); (3) verification. Flag Gap 1 to the user before planning if the "2 additions" count in CONTEXT.md is load-bearing for their mental model.

## Tool-to-Endpoint Mapping (the complete bridge surface)

Every tool, its HTTP method/path, body shape, and Kamacu handler line reference.

### Task tools (`internal/mcp/tasks.go`)

| Tool | MCP args | Bridge HTTP call | Kamacu handler | Notes |
|------|----------|------------------|----------------|-------|
| `list_tasks` | `project_id?: int` | `project_id` set → `GET /api/projects/{id}/tasks`; omitted → `GET /api/tasks` (NEW, D-01) | `tasks.go:183` `listByProject` / NEW `list` | `source='manual'` filter + `ORDER BY status, position ASC` both endpoints |
| `get_task` | `task_id: int` | `GET /api/tasks/{id}` | `tasks.go:275` `get` | 404 on missing |
| `create_task` | `project_id: int, title: string, description?: string` | `POST /api/projects/{id}/tasks` body `{title, description}` | `tasks.go:223` `create` | Status ALWAYS `todo`; worktree auto-provisions synchronously (30s cap); agent NOT auto-started |
| `update_task` | `task_id: int, title?: string, description?: string` | `PATCH /api/tasks/{id}` body `{title?, description?}` (omit nil fields, D-03) | `tasks.go:294` `update` | Partial-PATCH; empty body → returns current row (no error) |
| `move_task` | `task_id: int, status: string` | `POST /api/tasks/{id}/move` body `{status, after_id: null}` (D-06) | `tasks.go:347` `move` | `after_id: null` → top of column (`MIN(position)-1.0`); PR-review source → 409 |
| `delete_task` | `task_id: int` | `DELETE /api/tasks/{id}` | `tasks.go:553` `delete` | Gated cleanup: stops sessions, kills tmux, deletes row; 204 on success, 404 if missing |

### Project tools (`internal/mcp/projects.go`)

| Tool | MCP args | Bridge HTTP call | Kamacu handler | Notes |
|------|----------|------------------|----------------|-------|
| `list_projects` | `workspace_id?: int` | `workspace_id` set → `GET /api/projects?workspace_id=N`; omitted → `GET /api/projects` | `projects.go:127` `list` (D-02 extended) | **Inherited from Phase 06** (D-09 FINAL); registration MOVES here from `server.go` |
| `get_project` | `project_id: int` | `GET /api/projects/{id}` (**NEW — Gap 1**) | NEW `get` (mirror `taskHandlers.get`) | **No existing endpoint.** Planner must add it. |
| `create_project` | `name: string, repo_path?: string, repo?: string, workspace_id?: int` | `POST /api/projects` body `{name, repo_path, repo, workspace_id}` as-is (D-04) | `projects.go:163` `create` | Handler forks: `repo` non-empty → `createByRepo` (gh validate + clone + atomic INSERT); else → folder-path validate + INSERT |
| `update_project` | `project_id: int, name?, description?, github_repo?, icon_letters?, icon_color?` | `PATCH /api/projects/{id}` body with supplied fields only (D-03, excludes `workspace_id`+`agent_id`) | `projects.go:455` `update` | Partial-PATCH; all validation gates apply (icon palette, 280-cap, gh canonicalization); empty body → 400 "nothing to update" |
| `delete_project` | `project_id: int` | `DELETE /api/projects/{id}` | `projects.go:671` `delete` | Folder: hard delete, never touch dir. Managed: two-pass gated removal (409 with structured `reasons` if dirty/unpushed/stash/sessions) |

### Workspace tools (`internal/mcp/workspaces.go`)

| Tool | MCP args | Bridge HTTP call | Kamacu handler | Notes |
|------|----------|------------------|----------------|-------|
| `list_workspaces` | (none) | `GET /api/workspaces` | `workspaces.go:77` `list` | Returns `[]`, never nil |
| `create_workspace` | `name: string` | `POST /api/workspaces` body `{name}` | `workspaces.go:103` `create` | Case-insensitive dup → 409 |
| `update_workspace` | `workspace_id: int, name?: string` | `PATCH /api/workspaces/{id}` body `{name}` | `workspaces.go:137` `update` | Rename; default workspace IS renamable; dup-excluding-self → 409 |
| `delete_workspace` | `workspace_id: int` | `DELETE /api/workspaces/{id}` | `workspaces.go:192` `delete` | Two guards: `is_default` → 409; non-empty COUNT → 409 |
| `move_project_to_workspace` | `project_id: int, workspace_id: int` | `PATCH /api/projects/{id}` body `{workspace_id}` | `projects.go:557-575` (the `WorkspaceID` branch of `update`) | Validates target workspace exists → 400 "workspace not found" if not |

## Detailed Findings

### Gap 1: get_project needs a third Kamacu endpoint addition

(Full write-up above in [Gaps Found](#gap-1-get_project-needs-a-third-kamacu-endpoint-addition).)

**The new `projectHandlers.get` handler** (recommend the planner specify it explicitly) mirrors `taskHandlers.get` at `tasks.go:275-290`:
```go
func (h *projectHandlers) get(w http.ResponseWriter, r *http.Request) {
    id, ok := pathID(w, r)
    if !ok { return }
    p, err := scanProject(h.db.QueryRow(`SELECT `+projectColumns+` FROM projects WHERE id = ?`, id))
    if errors.Is(err, sql.ErrNoRows) {
        writeError(w, http.StatusNotFound, "project not found"); return
    }
    if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }
    writeJSON(w, http.StatusOK, p)
}
```
Route: `mux.HandleFunc("GET /api/projects/{id}", p.get)` in `routes.go`. **Caution:** this route MUST be registered BEFORE `GET /api/projects/{id}/github-origin` and `GET /api/projects/{id}/tasks` to avoid Go 1.22+ ServeMux pattern conflicts (more specific patterns win, but explicit ordering is safer and matches the existing file's ordering convention). Actually Go 1.22 ServeMux matches the longest pattern, so `/api/projects/{id}/tasks` wins over `/api/projects/{id}` for that path — no conflict. But verify during implementation.

### Gap 2: D-05 error body shape description is wrong

(Full write-up above in [Gaps Found](#gap-2-d-05-error-body-shape-description-is-wrong).)

**The actual Kamacu error shapes** the bridge will wrap verbatim:
- Standard error (most endpoints): `{"error":"<message>"}` via `writeError`.
- Managed-project delete 409: `{"error":"the project can't be deleted yet","reasons":[{"kind":"uncommitted|unpushed|stash|sessions","target":"task #N | the managed checkout"}]}` via `writeJSON`.
- Workspace delete 409 (default): `{"error":"the default workspace can't be deleted"}`.
- Workspace delete 409 (non-empty): `{"error":"move or remove its 3 project(s) first"}`.

The bridge wrap produces e.g. `kamacu DELETE /api/workspaces/1: HTTP 409: {"error":"the default workspace can't be deleted"}`. Test assertions should match on substring `"the default workspace"` or `"can't be deleted"`, NOT on `"message"`.

### move_task position semantics (D-06)

`move_task(task_id, status)` sends `POST /api/tasks/{id}/move` with body `{"status": <status>, "after_id": null}`.

The handler at `tasks.go:393-403` treats `after_id == nil` as "top of target column":
```go
if req.AfterID == nil {
    err = tx.QueryRowContext(ctx,
        `SELECT COALESCE(MIN(position), 2.0) - 1.0 FROM tasks
         WHERE project_id = ? AND status = ? AND id != ? AND source = 'manual'`,
        projectID, req.Status, id).Scan(&pos)
```
This is deterministic and predictable — the agent doesn't need to know other tasks' IDs or the board's current ordering. The bridge hardcodes `null`; the MCP tool exposes no `after_id` arg. The existing `/move` handler is byte-for-byte unchanged.

**Validation gates the agent hits through the bridge:** invalid status → 400 "invalid status"; PR-review source task → 409 "PR reviews are not board tasks"; missing task → 404 "task not found". All surface verbatim through the D-05 wrap.

### create_task timeout interaction

`create_task` → `POST /api/projects/{id}/tasks` triggers synchronous `provisionWorktree` (`tasks.go:263`) with a 30s git timeout. The bridge's `http.Client.Timeout` is **10s** (locked in Phase 06 `bridge.go:53`).

**Implication:** if git worktree creation takes >10s (large repo, slow disk, managed-clone fetch), the bridge's HTTP client times out and the agent sees a `context deadline exceeded` error — BUT the Kamacu handler continues running server-side and may successfully create the task + worktree. The agent's next `list_tasks` would then show the task that "failed" to create.

This is a **pre-existing characteristic** of the endpoint (the SPA has the same race — its fetch can time out while the server finishes). It is NOT a Phase 07 bug. The planner should NOT add retry logic or bump the timeout (Phase 06 locked 10s/no-retry as the agent's Discretion). Document it in the `create_task` tool description if desired, but no code change is needed.

### create_project two-arg fork (D-04)

`create_project(name, repo_path?, repo?, workspace_id?)` sends `{name, repo_path, repo, workspace_id}` as-is to `POST /api/projects`. The Kamacu handler at `projects.go:203` dispatches:
```go
if strings.TrimSpace(req.Repo) != "" {
    h.createByRepo(w, r, req.Repo, req.Name, wsID, agID)
    return
}
```
- `repo` non-empty → managed-checkout path: `gh` validation → `gh repo clone` into `~/.kamacu/repos/<owner>/<name>` → atomic INSERT (managed=1). Can take significant time (clone). Same 10s-bridge-timeout consideration as `create_task` applies but is MORE likely to fire (cloning is slower than worktree add).
- `repo` empty + `repo_path` set → folder path: `validateRepoPath` (absolute, exists, is git repo) → INSERT (managed=0).
- Both empty → `validateRepoPath("")` fails → 400 "path must be absolute".

**The bridge does ZERO dispatch logic** — it passes both fields and lets Kamacu's existing fork decide. This is the locked D-04 design.

### update_project validation gates (D-03)

`update_project` surfaces these Kamacu validation gates unchanged (the bridge sends fields as-is, Kamacu validates):
- `name`: trim, empty → 400 "name is required".
- `description`: >280 chars → 400 "Description is too long."
- `github_repo`: empty string → unlinks (NULL); non-empty → `gh` canonicalization + mandatory verification → 400 on fail (syntactically invalid / not found / gh absent).
- `icon_letters`: `validateIconLetters` — trim, alnum-only, upper, ≤2 chars, ≥1 required → 400 on empty.
- `icon_color`: `validateIconColor` — must be a member of `projectPalette` (case-insensitive) → 400 "off palette" if not.
- Empty body (all fields nil) → 400 "nothing to update".

**Excluded fields** (D-03): `workspace_id` (use `move_project_to_workspace`), `agent_id` (Out-of-Scope, MCPMORE-01). The bridge MUST NOT send these fields even if an agent supplies them — strip them or omit from the schema's properties entirely.

### Pattern: per-resource file split (D-07)

**Current state (`internal/mcp/`):**
```
internal/mcp/
├── server.go      # ServeCommand + registerTools (one AddTool for list_projects)
├── bridge.go      # bridge struct, newBridgeFromEnv, bridge.do, bridge.listProjects
├── server_test.go # SC2 regression (2 sub-tests)
└── bridge_test.go # list_projects integration tests (4 tests)
```

**Target state (after Phase 07):**
```
internal/mcp/
├── server.go      # ServeCommand + registerTools (delegates to 3 registrars)
├── bridge.go      # bridge struct, newBridgeFromEnv, bridge.do, maxBodyBytes, defaultBase (shared primitives)
├── tasks.go       # bridge.listTasks/getTask/createTask/updateTask/moveTask/deleteTask + registerTaskTools(s,b)
├── projects.go    # bridge.listProjects (MOVED) + getProject/createProject/updateProject/deleteProject + registerProjectTools(s,b)
├── workspaces.go  # bridge.listWorkspaces/createWorkspace/updateWorkspace/deleteWorkspace/moveProjectToWorkspace + registerWorkspaceTools(s,b)
├── server_test.go # SC2 regression (UNCHANGED — must stay green)
├── bridge_test.go # list_projects + env-default tests (list_projects test may move to projects_test.go)
├── tasks_test.go  # NEW — ~12 cases
├── projects_test.go # NEW — ~10 cases
└── workspaces_test.go # NEW — ~8 cases
```

**`registerTools` in `server.go` shrinks to:**
```go
func registerTools(s *mcp.Server, b *bridge) {
    registerTaskTools(s, b)
    registerProjectTools(s, b)
    registerWorkspaceTools(s, b)
}
```

**Code-motion caution:** `bridge.listProjects` and its Phase 06 test (`TestBridge_ListProjects_*`) are FINAL production code. The method body moves from `bridge.go` to `projects.go` (same package, so no import change). The test can stay in `bridge_test.go` or move to `projects_test.go` — either works since they're the same package. The `TestNewBridgeFromEnv_DefaultsAndOverride` test stays in `bridge_test.go` (it tests `bridge.go`'s env parsing, not a tool). The SC2 tests in `server_test.go` are untouched.

### Shared helper question (the agent's Discretion)

Phase 06 inlined the `b.do → defer Close → io.ReadAll(LimitReader) → non-200 wrap → TextContent` pattern in `listProjects`. Phase 07 repeats this 12+ times. CONTEXT.md flags a shared `doJSON` helper as the agent's call.

**Observation:** GET/DELETE tools (8 of 15) take no body and differ only in method/path — a shared helper saves ~10 lines each. POST/PATCH tools (7 of 15) need body marshaling — a `doJSON(ctx, method, path, body)` that marshals to `bytes.Reader` then calls `do` saves the `json.Marshal` + `bytes.NewReader` boilerplate. But the response-handling half (`io.ReadAll(LimitReader)` + non-200 wrap + TextContent) is identical across ALL tools and is the stronger candidate for extraction.

**Recommendation (non-binding):** Extract a `bridge.call(ctx, method, path, body) (*mcp.CallToolResult, error)` helper that encapsulates the full response-handling half (do → close → read → wrap → passthrough). Each tool handler becomes: build path/args → `return b.call(ctx, method, path, body)`. This halves the per-tool line count and makes the "every tool is the same shape" invariant structurally enforced. But if the agent prefers inline (Phase 06's choice), that's fine — just more lines.

### InputSchema shape (map[string]any, locked from Phase 06)

Phase 06's 06-02-SUMMARY documents the API-drift auto-fix: `Tool.InputSchema` is `any` at SDK v1.6.1, and `[]byte` (json.RawMessage) panics because Go marshals it as base64. The fix was `map[string]any{"type": "object", "properties": {...}}`. **Phase 07 inherits this verbatim** — every tool's InputSchema is a `map[string]any`, never `json.RawMessage`.

Example shape for a tool with required + optional args (`create_task`):
```go
InputSchema: map[string]any{
    "type": "object",
    "properties": map[string]any{
        "project_id": map[string]any{"type": "integer", "description": "..."},
        "title":       map[string]any{"type": "string", "description": "..."},
        "description": map[string]any{"type": "string", "description": "..."},
    },
    "required": []string{"project_id", "title"},
},
```

### Argument extraction in handlers

The handler receives `*mcp.CallToolRequest` whose `Params.Arguments` is `json.RawMessage`. Phase 06's `list_projects` ignored args entirely (the `project_id` was a no-op). Phase 07 tools MUST extract args. The established pattern (the agent's Discretion per CONTEXT.md) is to unmarshal into a typed struct or a `map[string]any`:

```go
func (b *bridge) createTask(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    var args struct {
        ProjectID   int64  `json:"project_id"`
        Title       string `json:"title"`
        Description string `json:"description,omitempty"`
    }
    if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
        return nil, fmt.Errorf("create_task: invalid arguments: %w", err)
    }
    body, _ := json.Marshal(map[string]any{"title": args.Title, "description": args.Description})
    return b.call(ctx, http.MethodPost, fmt.Sprintf("/api/projects/%d/tasks", args.ProjectID), bytes.NewReader(body))
}
```

For optional scalar args (`update_task`'s `title?`, `description?`), use pointer fields (`*string`) in the args struct so `nil` = omitted vs `""` = explicit empty. Then build the PATCH body conditionally (D-03: omit nil fields to match Kamacu's partial-PATCH semantics).

## Architecture Patterns

### Tool-handler canonical shape (Phase 06 locked, Phase 07 repeats)

Every Phase 07 tool handler clones `bridge.listProjects` (`bridge.go:80-97`):
1. `resp, err := b.do(ctx, method, path, body)` — `body` is `nil` for GET/DELETE, `io.Reader` for POST/PATCH.
2. `defer resp.Body.Close()`.
3. `body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))` — 1 MiB cap.
4. Non-200 → `return nil, fmt.Errorf("kamacu %s %s: HTTP %d: %s", method, path, resp.StatusCode, string(body))`.
5. 200 → `return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}}, nil`.

No tool re-implements HTTP client construction. No tool parses the response JSON (raw TextContent passthrough). No tool adds validation (Kamacu's handlers validate; D-03/D-04).

### Bridge wire contract (Phase 06 locked)

- `X-Kamacu-Token` header on every request (`bridge.do` sets it unconditionally).
- 10s `http.Client.Timeout`, no retry (fail fast).
- 1 MiB response cap (`maxBodyBytes = 1 << 20`).
- Base URL from `KAMACU_HOOK_BASE` env, default `http://127.0.0.1:7333`.
- Kamacu ignores the token on `/api/*` in v1.11 (loopback is the auth boundary); the header is sent for future correctness.

## Common Pitfalls

### Pitfall 1: Forgetting the Gap 1 endpoint
**What goes wrong:** The planner follows CONTEXT.md's "2 small additions" count and ships only D-01 + D-02. `get_project` has nothing to bridge to → the tool either fails at runtime (404 from Kamacu because the route doesn't exist → Go ServeMux returns 404) or the planner implements it wrong.
**How to avoid:** Read this research's [Gap 1](#gap-1-get_project-needs-a-third-kamacu-endpoint-addition). Add `GET /api/projects/{id}` as a third endpoint addition. It's ~15 lines.

### Pitfall 2: Test assertions on the wrong error JSON shape
**What goes wrong:** A test asserts the wrapped error contains `"message"` (trusting D-05's description) but the actual body is `{"error":"..."}` → test fails.
**How to avoid:** Assert on Kamacu message text substrings (e.g. `"already exists"`, `"name is required"`), not on JSON key names. See [Gap 2](#gap-2-d-05-error-body-shape-description-is-wrong).

### Pitfall 3: `update_project` sending excluded fields
**What goes wrong:** The bridge forwards `workspace_id` or `agent_id` to Kamacu's PATCH, violating D-03 (these are excluded — workspace transfer has its own tool; agent reassignment is Out-of-Scope).
**How to avoid:** The `update_project` InputSchema MUST NOT list `workspace_id` or `agent_id` in properties. Even if an agent supplies them, the SDK won't pass unknown properties (MCP spec: servers ignore unknown args). But to be safe, the handler's args struct should not include those fields.

### Pitfall 4: Regressing the SC2 test or list_projects behavior
**What goes wrong:** The D-07 code-motion (moving `list_projects` from `server.go` to `projects.go`) breaks the SC2 regression test or the list_projects integration tests.
**How to avoid:** The move is same-package (`internal/mcp`), so no import changes. `registerTools` must still call the registrars in an order that doesn't matter (tools are independent). Run `go test ./internal/mcp/...` after the split — all 6 existing tests (2 SC2 + 4 bridge) must stay green.

### Pitfall 5: ServeMux pattern conflict on `GET /api/projects/{id}`
**What goes wrong:** Registering `GET /api/projects/{id}` conflicts with existing `GET /api/projects/{id}/github-origin` or `GET /api/projects/{id}/tasks`.
**How to avoid:** Go 1.22+ ServeMux matches the longest registered pattern, so `/api/projects/{id}/tasks` wins for that exact path and `/api/projects/{id}` catches everything else under that prefix. No conflict. Verify with `go test ./internal/api/...` after adding the route.

### Pitfall 6: `list_projects` description still says "Phase 07 will honor it"
**What goes wrong:** Phase 06's `list_projects` description says "The optional project_id is currently a no-op (filtering arrives in Phase 07)." Phase 07 makes `workspace_id` functional (D-02) but the old description references `project_id` (which was never the filter arg — it was always `workspace_id`). If the description is updated as part of moving the tool to `projects.go`, it should describe `workspace_id?` instead.
**How to avoid:** When moving `list_projects` into `projects.go`, update the description to reflect the real `workspace_id?` arg and remove the stale `project_id` no-op language. The tool's InputSchema changes from `{project_id?}` to `{workspace_id?}` — this is a legitimate Phase 07 change (D-02 makes it functional), not a regression.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Argument parsing per tool | Custom `map[string]any` field-by-field extraction | `json.Unmarshal(req.Params.Arguments, &typedStruct)` | Mechanical, type-safe, the agent's Discretion endorsed this |
| Response JSON massaging | Parse Kamacu JSON → restructure → re-marshal | Raw TextContent passthrough | Phase 06 locked this; agents parse JSON robustly |
| Validation in the bridge | Re-implement icon palette / 280-cap / gh validation | Send fields as-is, let Kamacu validate (D-03/D-04) | SC4 satisfied by NOT duplicating validation |
| Board position math | Compute positions client-side | `move_task` sends `after_id: null` (D-06) | Kamacu's `/move` handler owns all position math |
| HTTP retry / backoff | Custom retry on timeout | Fail fast (Phase 06 locked 10s/no-retry) | Loopback single-user; a timeout means the server is slow/down |

## Key Insight

Phase 07's value is **volume execution of a proven pattern**, not novel architecture. The bridge pattern is locked; every primitive exists. The phase proves the pattern scales from 1 tool to 13. If any tool needs meaningfully different bridge logic, that's a signal to surface to the user (per CONTEXT.md `<specifics>`). The two real planning corrections are Gap 1 (add `GET /api/projects/{id}`) and Gap 2 (use real error body shape in tests).

## RESEARCH COMPLETE
