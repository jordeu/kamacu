---
phase: 07-tasks-projects-workspaces-tools
plan: 01
slug: endpoints-and-mcp-scaffold
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/api/routes.go
  - internal/api/tasks.go
  - internal/api/projects.go
  - internal/mcp/server.go
  - internal/mcp/bridge.go
  - internal/mcp/projects.go
  - internal/mcp/tasks.go
  - internal/mcp/workspaces.go
  - internal/mcp/bridge_test.go
  - internal/api/tasks_test.go
  - internal/api/projects_test.go
autonomous: true
requirements: [MCPPROJ-01]
must_haves:
  truths:
    # D-01: GET /api/tasks (unscoped) backs list_tasks when project_id is omitted
    - "`internal/api/routes.go` registers `mux.HandleFunc(\"GET /api/tasks\", t.list)` — a NEW unscoped task-list route distinct from the existing `GET /api/projects/{id}/tasks` (`t.listByProject`). The new `taskHandlers.list` handler mirrors `listByProject`'s SELECT with the `WHERE project_id = ?` clause dropped, KEEPS the `source = 'manual'` filter (GHREV-04 — PR reviews stay off the board), and KEEPS the `ORDER BY status, position ASC` ordering."
    # D-02: ?workspace_id= query param on GET /api/projects
    - "`internal/api/projects.go`'s `projectHandlers.list` reads `r.URL.Query().Get(\"workspace_id\")`; when non-empty AND parses as int via `strconv.Atoi`, the SELECT gains `WHERE workspace_id = ?` with the parsed value. When empty, unparseable, or absent, the handler returns ALL projects (current behavior — backward-compatible; the SPA and Phase 06 callers that omit the param are byte-for-byte unchanged). Per the Kamacu convention, an unparseable value is silently ignored (not a 400)."
    # Gap 1: GET /api/projects/{id} — the THIRD endpoint addition the CONTEXT omitted
    - "`internal/api/routes.go` registers `mux.HandleFunc(\"GET /api/projects/{id}\", p.get)` — a NEW single-project fetch. `projectHandlers.get` mirrors `taskHandlers.get` (`internal/api/tasks.go:275-290`): `pathID` parse → `SELECT projectColumns FROM projects WHERE id = ?` → 404 `writeError(w, http.StatusNotFound, \"project not found\")` on `sql.ErrNoRows` → `writeJSON(w, http.StatusOK, p)` on hit. Go 1.22 ServeMux longest-pattern-wins means `/api/projects/{id}/tasks` and `/api/projects/{id}/github-origin` still match their more specific paths — no conflict."
    # D-07: per-resource file split scaffold
    - "`internal/mcp/` contains three new files: `tasks.go`, `projects.go`, `workspaces.go`. Each defines a `register<Task|Project|Workspace>Tools(s *mcp.Server, b *bridge)` function. `server.go`'s `registerTools` is reduced to a three-line delegator calling all three registrars. `registerTaskTools` and `registerWorkspaceTools` have EMPTY bodies in this plan (no `s.AddTool` calls) — they exist only so `registerTools` compiles; Plans 02/03/04 fill them with tools."
    - "`bridge.go` no longer defines `listProjects` — that method MOVES to `projects.go` (single ownership of the project resource across all 5 project tools). The shared primitives (`bridge struct`, `newBridgeFromEnv`, `bridge.do`, `maxBodyBytes`, `defaultBase`) STAY in `bridge.go`."
    # bridge.call shared helper (Claude's discretion per CONTEXT.md; chosen to halve per-tool line count for the 13 tools in Plans 02/03/04)
    - "`bridge.go` defines `func (b *bridge) call(ctx context.Context, method, path string, body io.Reader) (*mcp.CallToolResult, error)` — the shared response-handling half. It encapsulates the Phase 06 listProjects shape: `b.do` → `defer resp.Body.Close()` → `io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))` → transport-error wrap (`fmt.Errorf(\"kamacu bridge: %w\", err)`) → non-200 wrap (`fmt.Errorf(\"kamacu %s %s: HTTP %d: %s\", method, path, resp.StatusCode, string(respBody))`) → 200 TextContent passthrough. The D-05 wrap pattern is verbatim (body passed as raw string — shape-agnostic per 07-RESEARCH Gap 2)."
    # list_projects updated for workspace_id? (MCPPROJ-01) — Pitfall 6 / D-02
    - "The `list_projects` tool's InputSchema in `projects.go` declares `workspace_id` (type integer, optional) as its ONLY property — the stale `project_id` no-op arg from Phase 06 is REMOVED. The description references the real `workspace_id?` arg and the `?workspace_id=` query param on `GET /api/projects`; the stale 'Phase 07 will honor it' language is gone."
    - "`bridge.listProjects(ctx, req)` extracts `workspace_id` from `req.Params.Arguments` into `*int64`; when non-nil, the request path is `fmt.Sprintf(\"/api/projects?workspace_id=%d\", *args.WorkspaceID)`; when nil (or args empty), the path is `/api/projects` (current behavior). The handler delegates to `b.call(ctx, http.MethodGet, path, nil)`. The AddTool closure passes `req` through (no longer discards it)."
    # Existing tests stay green (or are updated to the new signature)
    - "`TestBridge_ListProjects_PassesThroughKamacuJSON`, `_KamacuReturnsNon200_ReturnsError`, and `_KamacuUnreachable_ReturnsError` in `bridge_test.go` are updated to construct a `*mcp.CallToolRequest` (empty `Params.Arguments`) and call `b.listProjects(ctx, req)` instead of `b.listProjects(ctx)`. Their assertions (X-Kamacu-Token header sent, raw passthrough, HTTP 500 substring, `kamacu bridge:` substring) are unchanged. `TestNewBridgeFromEnv_DefaultsAndOverride` is unchanged."
    - "A NEW test `TestBridge_ListProjects_WorkspaceIDAppendsQueryParam` (in `bridge_test.go` or `projects_test.go`) proves the workspace_id? path: the test server captures `r.URL.RawQuery`, the bridge is invoked with a `CallToolRequest` whose `Params.Arguments` is `{\"workspace_id\":2}`, and the assertion is `r.URL.RawQuery == \"workspace_id=2\"` AND `r.URL.Path == \"/api/projects\"`."
    - "`TestSC2_HandlerReturningError_ProducesCleanJSONRPCErrResponse` and `TestSC2_HandlerReturningIsErrorResult_ProducesCleanJSONRPCSuccessWithIsErrorTrue` in `server_test.go` are byte-for-byte UNCHANGED (the D-07 refactor does not touch the SDK's error-response contract). `go test ./internal/mcp/... -run TestSC2 -v` stays GREEN."
    # Bridge tools layer through HTTP only (no internal/store import)
    - "No file under `internal/mcp/` imports `kamacu/internal/store` or `kamacu/internal/api` — the bridge goes through HTTP via `bridge.do`/`bridge.call` exclusively. The endpoint additions in `internal/api/{routes.go, tasks.go, projects.go}` use the existing `*sql.DB` like every other handler."
  artifacts:
    # Kamacu endpoint additions (internal/api/)
    - internal/api/routes.go (adds 2 new mux.HandleFunc lines: GET /api/tasks, GET /api/projects/{id})
    - internal/api/tasks.go (NEW method: func (h *taskHandlers) list(w http.ResponseWriter, r *http.Request))
    - internal/api/projects.go (NEW method: func (h *projectHandlers) get(w http.ResponseWriter, r *http.Request); MODIFIED method: projectHandlers.list reads ?workspace_id= query param)
    # internal/mcp/ per-resource split (D-07)
    - internal/mcp/tasks.go (NEW — registerTaskTools(s, b) with empty body in this plan)
    - internal/mcp/projects.go (NEW — registerProjectTools(s, b) registering the updated list_projects; bridge.listProjects method MOVED here from bridge.go)
    - internal/mcp/workspaces.go (NEW — registerWorkspaceTools(s, b) with empty body in this plan)
    - internal/mcp/server.go (MODIFIED — registerTools shrinks to a 3-line delegator)
    - internal/mcp/bridge.go (MODIFIED — listProjects method REMOVED; NEW method bridge.call shared helper)
    - internal/mcp/bridge_test.go (MODIFIED — list_projects tests updated to new (ctx, req) signature; NEW TestBridge_ListProjects_WorkspaceIDAppendsQueryParam)
    # Tests for the new Kamacu endpoints (httptest pattern)
    - internal/api/tasks_test.go (NEW or EXTENDED — TestTaskList_All_ManualOnly_OrderedByStatusPosition; TestTaskList_EmptyReturnsArray)
    - internal/api/projects_test.go (NEW or EXTENDED — TestProjectGet_Found; TestProjectGet_NotFound404; TestProjectList_WorkspaceIDFilter; TestProjectList_NoFilterReturnsAll)
  key_links:
    - "internal/api/routes.go: `mux.HandleFunc(\"GET /api/tasks\", t.list)` (new) — backs the unscoped branch of list_tasks (MCPTASK-01, delivered in Plan 02)"
    - "internal/api/routes.go: `mux.HandleFunc(\"GET /api/projects/{id}\", p.get)` (new) — backs get_project (MCPPROJ-02, delivered in Plan 03)"
    - "internal/api/projects.go: projectHandlers.list reads `r.URL.Query().Get(\"workspace_id\")` — backs list_projects(workspace_id?) (MCPPROJ-01, delivered HERE)"
    - "internal/mcp/server.go: registerTools(s, b) → { registerTaskTools(s, b); registerProjectTools(s, b); registerWorkspaceTools(s, b) }"
    - "internal/mcp/projects.go: registerProjectTools(s, b) → s.AddTool(&mcp.Tool{Name: \"list_projects\", InputSchema: {workspace_id?}}, b.listProjects) — the tool Phase 06 shipped, now with the real workspace_id? arg"
    - "internal/mcp/bridge.go: bridge.call(ctx, method, path, body) — the shared helper Plans 02/03/04 invoke from every tool handler"
  prohibitions:
    - statement: "NO tool besides `list_projects` is registered in this plan — registerTaskTools and registerWorkspaceTools have EMPTY bodies. Plans 02/03/04 add the remaining 12 tools."
      status: resolved
      verification: "`grep -c 's.AddTool' internal/mcp/tasks.go internal/mcp/workspaces.go` returns 0 for both files; `grep -c 's.AddTool' internal/mcp/projects.go` returns exactly 1 (list_projects)"
    - statement: "NO bridge-side validation of workspace_id values (e.g. rejecting 0 or negative) — the Kamacu handler is the validator. The bridge sends the query param as-is; Kamacu's existing gate applies."
      status: resolved
      verification: "internal/mcp/projects.go's listProjects handler does not call any validation function on args.WorkspaceID before appending it to the query string"
    - statement: "NO DB schema changes, NO new migrations, NO new long-lived goroutines — the v1.11 milestone rule. The three endpoint additions are pure SELECT additions / route registrations."
      status: resolved
      verification: "`git diff --name-only` for this plan shows no files under `internal/store/migrations/`; no `go func()` call added to internal/api/{tasks.go,projects.go}"
    - statement: "DO NOT parse Kamacu error JSON in the bridge — D-05 passes the body as a raw string. Test assertions MUST use substrings of Kamacu message text (e.g. `\"already exists\"`, `\"not found\"`), NOT JSON key names like `\"message\"` (07-RESEARCH Gap 2: the real shape is `{\"error\":\"...\"}` not `{\"status\",\"message\"}`)."
      status: resolved
      verification: "internal/mcp/bridge_test.go contains no assertion matching on the literal string `message` as a JSON key; error assertions match on `HTTP 500` / `HTTP 404` / `kamacu bridge:` substrings or Kamacu message text"
    - statement: "DO NOT use the typed `mcp.AddTool[In, Out any]` generic helper — keep using the low-level `s.AddTool(*mcp.Tool, handler)` with `map[string]any` InputSchema (Phase 06 locked this; 06-02-SUMMARY documents `Tool.InputSchema` is `any` at SDK v1.6.1)."
      status: resolved
      verification: "`grep -rn 'mcp.AddTool\\[' internal/mcp/` returns zero results"
    - statement: "DO NOT break the SC2 regression tests — the D-07 refactor must leave `internal/mcp/server_test.go` byte-for-byte unchanged and `go test ./internal/mcp/... -run TestSC2 -v` GREEN."
      status: resolved
      verification: "`git diff internal/mcp/server_test.go` for this plan is empty; `go test ./internal/mcp/... -run TestSC2 -v` passes"
---

# Plan 01: Kamacu endpoint additions + internal/mcp/ D-07 scaffold + list_projects(workspace_id?)

<objective>
Lay the foundation for Plans 02/03/04 by (a) adding the three Kamacu endpoint additions the bridge tools need (D-01 `GET /api/tasks`, D-02 `?workspace_id=` on `GET /api/projects`, Gap 1 `GET /api/projects/{id}`), (b) executing the D-07 per-resource file split so Plans 02/03/04 each own a single new file, (c) extracting the shared `bridge.call` helper so every Phase 07 tool handler is a 5-line build-path-and-delegate, and (d) updating `list_projects` for the real `workspace_id?` arg (delivering MCPPROJ-01 here, since D-02's query param lands in this plan).

Purpose: this plan is the dependency for everything else in Phase 07. Plans 02/03/04 each touch ONLY their own new file (`tasks.go`/`projects.go`/`workspaces.go`) and the corresponding test file — they never touch `server.go`, `bridge.go`, or each other. That parallelism is only possible because this plan ships the empty `registerTaskTools`/`registerWorkspaceTools` shells and the populated `registerProjectTools` (with list_projects) in advance.

Output: three new routes in `internal/api/routes.go`, three new handlers in `internal/api/{tasks.go, projects.go}`, three new files in `internal/mcp/` (`tasks.go`, `projects.go`, `workspaces.go`), a refactored `registerTools` in `internal/mcp/server.go`, a new `bridge.call` helper in `internal/mcp/bridge.go`, the moved+updated `listProjects` in `internal/mcp/projects.go`, and updated/new tests.
</objective>

<execution_context>
@/home/jordi/.config/opencode/gsd-core/workflows/execute-plan.md
@/home/jordi/.config/opencode/gsd-core/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/phases/07-tasks-projects-workspaces-tools/07-CONTEXT.md
@.planning/phases/07-tasks-projects-workspaces-tools/07-RESEARCH.md
@.planning/phases/06-mcp-subcommand-foundation/06-02-SUMMARY.md
@internal/api/routes.go
@internal/api/tasks.go
@internal/api/projects.go
@internal/api/workspaces.go
@internal/api/respond.go
@internal/mcp/server.go
@internal/mcp/bridge.go
@internal/mcp/bridge_test.go
@internal/mcp/server_test.go
</context>

<tasks>

<task type="auto">
  <name>Task 1: Add the three Kamacu endpoint additions (D-01 GET /api/tasks, D-02 ?workspace_id= query param, Gap 1 GET /api/projects/{id})</name>
  <files>internal/api/routes.go, internal/api/tasks.go, internal/api/projects.go, internal/api/tasks_test.go, internal/api/projects_test.go</files>
  <read_first>
    - `.planning/phases/07-tasks-projects-workspaces-tools/07-RESEARCH.md` § "Gap 1: get_project needs a third Kamacu endpoint addition" (the load-bearing planning correction — there are 3 additions, not 2) and § "Detailed Findings → Gap 1" (the exact `projectHandlers.get` shape mirroring `taskHandlers.get` at tasks.go:275-290, plus the ServeMux longest-pattern-wins note)
    - `.planning/phases/07-tasks-projects-workspaces-tools/07-CONTEXT.md` decisions D-01 (`GET /api/tasks` mirrors `listByProject` SELECT with WHERE dropped; keeps `source='manual'` filter + `ORDER BY status, position ASC`) and D-02 (`?workspace_id=N` query param on `GET /api/projects`, backward-compatible, ~5-line extension)
    - `internal/api/tasks.go` lines 182-219 — `taskHandlers.listByProject` is the SELECT template for the new `list` method (drop the `WHERE project_id = ?` clause; keep everything else verbatim including the `source = 'manual'` filter and `ORDER BY status, position ASC`). Note the `taskColumns` const and `scanTask` helper are reused.
    - `internal/api/tasks.go` lines 274-290 — `taskHandlers.get` is the template for the new `projectHandlers.get` (the Gap 1 endpoint). Note `pathID`, `writeError`, `writeJSON`, `scanProject`/`scanTask` are package-level helpers.
    - `internal/api/projects.go` lines 103-124 — `projectColumns` const and `scanProject` helper (the new `get` handler SELECTs `projectColumns` and scans via `scanProject`).
    - `internal/api/projects.go` lines 127-148 — `projectHandlers.list` is the handler D-02 extends (read `r.URL.Query().Get("workspace_id")`, conditionally append `WHERE workspace_id = ?` to the existing SELECT).
    - `internal/api/routes.go` lines 32-43 — the existing project/task route registrations; this task adds two new lines (GET /api/tasks, GET /api/projects/{id}).
    - `internal/api/respond.go` lines 8-18 — `writeJSON` and `writeError` (the response helpers every handler uses; `writeError` produces `{"error": msg}` — 07-RESEARCH Gap 2 confirms this is the real shape, not D-05's described `{"status","message"}`).
    - `internal/api/tasks_test.go` and `internal/api/projects_test.go` (if they exist) — the established `httptest` test pattern this task mirrors for the new endpoint coverage. If these files don't exist yet, the task creates them following the pattern from a sibling test file like `internal/api/agent_integration_test.go`.
  </read_first>
  <action>
    1. **D-01: Add `GET /api/tasks` (unscoped).** In `internal/api/tasks.go`, add a new method `func (h *taskHandlers) list(w http.ResponseWriter, r *http.Request)` whose body clones `listByProject` (lines 182-219) with TWO changes: (a) drop the `pathID` parse and the project-existence check (no path id); (b) change the SELECT from `SELECT taskColumns FROM tasks WHERE project_id = ? AND source = 'manual' ORDER BY status, position ASC` to `SELECT taskColumns FROM tasks WHERE source = 'manual' ORDER BY status, position ASC` (drop the `WHERE project_id = ?` clause; KEEP the `source = 'manual'` filter per GHREV-04 and the `ORDER BY status, position ASC` ordering). The `rows`/`scanTask`/`tasks := []Task{}`/`writeJSON(w, http.StatusOK, tasks)` shape is identical to `listByProject`. In `internal/api/routes.go`, add the registration `mux.HandleFunc("GET /api/tasks", t.list)` near the existing `GET /api/projects/{id}/tasks` line.

    2. **Gap 1: Add `GET /api/projects/{id}`.** In `internal/api/projects.go`, add a new method `func (h *projectHandlers) get(w http.ResponseWriter, r *http.Request)` mirroring `taskHandlers.get` (tasks.go:275-290): `id, ok := pathID(w, r); if !ok { return }` → `p, err := scanProject(h.db.QueryRow("SELECT "+projectColumns+" FROM projects WHERE id = ?", id))` → `if errors.Is(err, sql.ErrNoRows) { writeError(w, http.StatusNotFound, "project not found"); return }` → `if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }` → `writeJSON(w, http.StatusOK, p)`. In `internal/api/routes.go`, add the registration `mux.HandleFunc("GET /api/projects/{id}", p.get)` BEFORE the existing `GET /api/projects/{id}/github-origin` and `GET /api/projects/{id}/tasks` lines (defensive ordering — Go 1.22 ServeMux longest-pattern-wins means there's no actual conflict, but matching the file's existing top-down ordering convention is safer and clearer; 07-RESEARCH Pitfall 5 confirms no conflict either way).

    3. **D-02: Extend `GET /api/projects` with `?workspace_id=N`.** In `internal/api/projects.go`'s `projectHandlers.list` (lines 127-148), insert query-param reading BEFORE the SELECT: `wsIDStr := r.URL.Query().Get("workspace_id")`. If `wsIDStr != ""`, attempt `wsID, err := strconv.Atoi(wsIDStr)`; on `err == nil` (parse success), use a SELECT with `WHERE workspace_id = ?` appended: `h.db.Query("SELECT "+projectColumns+" FROM projects WHERE workspace_id = ? ORDER BY name COLLATE NOCASE", wsID)`. On parse failure OR empty string, fall through to the existing `h.db.Query("SELECT "+projectColumns+" FROM projects ORDER BY name COLLATE NOCASE")` (current behavior — silently ignore per the Kamacu API convention, per CONTEXT.md the agent's Discretion). The rest of the handler (rows scan loop, `writeJSON(w, http.StatusOK, projects)`) is unchanged. Add `"strconv"` to the imports if not already present (it is already imported for `pathID`).

    4. **Tests for the new endpoints.** Add coverage in `internal/api/tasks_test.go` and `internal/api/projects_test.go` (create the files if absent, following the established `httptest` pattern from `internal/api/agent_integration_test.go` or a sibling _test.go file). Each test stands up a test mux with `api.Routes` (or directly registers the handler under test) against an in-memory SQLite DB seeded with fixture rows. Required cases:
       - `TestTaskList_All_ReturnsOnlyManualTasksOrderedByStatusPosition` — seed a project with one `source='manual'` task and one `source='github_pr'` task; GET /api/tasks returns ONLY the manual task, ordered by `status, position ASC`.
       - `TestTaskList_EmptyReturnsEmptyArray` — empty DB returns `[]` (not `null`) — matches `listByProject`'s `tasks := []Task{}` initialization.
       - `TestProjectGet_Found_ReturnsProjectJSON` — seed a project; GET /api/projects/{id} returns the project JSON via `writeJSON`.
       - `TestProjectGet_NotFound_Returns404` — GET /api/projects/9999 returns 404 with `{"error":"project not found"}` body.
       - `TestProjectList_WorkspaceIDFilter_ReturnsOnlyThatWorkspace` — seed two projects in different workspaces; GET /api/projects?workspace_id=2 returns only the workspace-2 project.
       - `TestProjectList_NoFilter_ReturnsAll` (regression) — GET /api/projects (no query param) returns both projects — proves the D-02 extension is backward-compatible.
       - `TestProjectList_WorkspaceIDUnparseable_IgnoresAndReturnsAll` — GET /api/projects?workspace_id=abc returns all projects (silently ignored per Kamacu convention).
       All error-body assertions use substrings of the Kamacu message text (e.g. `"project not found"`), NOT JSON key names — per 07-RESEARCH Gap 2 / Pitfall 2.

    5. **Verify.** Run `go vet ./internal/api/...` and `go test ./internal/api/... -v`. All new and existing API tests must pass. The new routes must appear in `internal/api/routes.go` — confirm with `grep -c 'mux.HandleFunc' internal/api/routes.go` (the count increases by exactly 2: GET /api/tasks and GET /api/projects/{id}).
  </action>
  <verify>
    <automated>
      set -e
      # D-01: GET /api/tasks registered + handler exists
      grep -q 'mux.HandleFunc("GET /api/tasks"' internal/api/routes.go
      grep -q 'func (h \*taskHandlers) list(' internal/api/tasks.go
      # D-02: projectHandlers.list reads workspace_id query param
      grep -q 'r.URL.Query().Get("workspace_id")' internal/api/projects.go
      # Gap 1: GET /api/projects/{id} registered + handler exists
      grep -q 'mux.HandleFunc("GET /api/projects/{id}"' internal/api/routes.go
      grep -q 'func (h \*projectHandlers) get(' internal/api/projects.go
      # The new task list handler keeps the source='manual' filter (GHREV-04)
      grep -A3 'func (h \*taskHandlers) list(' internal/api/tasks.go | grep -q "source = 'manual'"
      # No new migrations, no new goroutines
      ! git diff --name-only HEAD -- internal/store/migrations/ | grep .
      ! grep -n 'go func()' internal/api/tasks.go internal/api/projects.go | grep -v '//' || true
      # go vet and tests
      go vet ./internal/api/...
      go test ./internal/api/... -v
    </automated>
  </verify>
  <acceptance_criteria>
    - `grep -q 'mux.HandleFunc("GET /api/tasks", t.list)' internal/api/routes.go` succeeds (D-01 route registered)
    - `grep -q 'func (h \*taskHandlers) list(w http.ResponseWriter, r \*http.Request)' internal/api/tasks.go` succeeds (D-01 handler exists)
    - The new `taskHandlers.list` SELECT contains the literal `source = 'manual'` filter and `ORDER BY status, position ASC` ordering (asserted via grep)
    - The new `taskHandlers.list` SELECT does NOT contain `WHERE project_id = ?` (the unscoped branch — assert via `! grep`)
    - `grep -q 'r.URL.Query().Get("workspace_id")' internal/api/projects.go` succeeds (D-02 query param read)
    - `grep -q 'mux.HandleFunc("GET /api/projects/{id}", p.get)' internal/api/routes.go` succeeds (Gap 1 route registered)
    - `grep -q 'func (h \*projectHandlers) get(w http.ResponseWriter, r \*http.Request)' internal/api/projects.go` succeeds (Gap 1 handler exists)
    - `go test ./internal/api/... -run TestTaskList_All -v` passes (D-01 coverage)
    - `go test ./internal/api/... -run TestProjectGet_NotFound -v` passes (Gap 1 404 coverage)
    - `go test ./internal/api/... -run TestProjectList_WorkspaceIDFilter -v` passes (D-02 coverage)
    - `go test ./internal/api/... -run TestProjectList_NoFilter -v` passes (D-02 backward-compat regression)
    - `go test ./internal/api/...` passes (no existing API test regressed)
    - `git diff --name-only HEAD -- internal/store/migrations/` is empty (no schema change — v1.11 milestone rule)
  </acceptance_criteria>
  <done>
    - GET /api/tasks is registered and returns manual-only tasks ordered by status, position ASC
    - GET /api/projects/{id} is registered and returns a single project (404 on missing)
    - GET /api/projects reads ?workspace_id= and filters when parseable, ignores when not, returns all when absent
    - internal/api/{tasks_test.go, projects_test.go} cover the three additions + the backward-compat regression
    - `go vet ./internal/api/...` and `go test ./internal/api/...` pass
  </done>
</task>

<task type="auto">
  <name>Task 2: D-07 per-resource file split + bridge.call shared helper + list_projects(workspace_id?) + test updates</name>
  <files>internal/mcp/server.go, internal/mcp/bridge.go, internal/mcp/projects.go, internal/mcp/tasks.go, internal/mcp/workspaces.go, internal/mcp/bridge_test.go</files>
  <read_first>
    - `.planning/phases/07-tasks-projects-workspaces-tools/07-RESEARCH.md` § "Pattern: per-resource file split (D-07)" (the target directory layout, the shrunken `registerTools` body, the code-motion caution about `list_projects` being FINAL Phase 06 production code), § "Shared helper question" (the non-binding recommendation to extract `bridge.call`), § "Pitfall 4" (SC2 + list_projects tests must stay green), § "Pitfall 6" (the list_projects description/schema must drop the stale `project_id` no-op arg and reference the real `workspace_id?`), § "Common Pitfalls → Pitfall 2" (test assertions on error bodies use substrings, not JSON key names), § "Detailed Findings → Gap 2" (the actual Kamacu error shape is `{"error":"..."}`)
    - `.planning/phases/07-tasks-projects-workspaces-tools/07-CONTEXT.md` decisions D-05 (wrap pattern — `fmt.Errorf("kamacu %s %s: HTTP %d: %s", method, path, code, body)`, body passed as raw string), D-07 (per-resource split — `tasks.go`, `projects.go`, `workspaces.go` each own their `*bridge` methods + `register<Task|Project|Workspace>Tools(s, b)` function; `bridge.go` keeps shared primitives; `list_projects` MOVES into `projects.go`), and the `<decisions>` section "the agent's Discretion" bullets (bridge method signatures, exact InputSchema shape, whether a shared `doJSON` helper is justified — this plan chooses YES on the helper, named `bridge.call`)
    - `internal/mcp/bridge.go` (full file) — the current `bridge` struct, `newBridgeFromEnv`, `bridge.do`, and `listProjects` (the method that MOVES to projects.go and gets the `workspace_id?` update). The `bridge.call` helper is extracted from the body of `listProjects` lines 80-97.
    - `internal/mcp/server.go` (full file) — the current `registerTools` function (lines 81-102) that this task shrinks to a 3-line delegator. The `ServeCommand` body (lines 46-70) is UNCHANGED. The `s.AddTool` call for `list_projects` (lines 82-102) MOVES into `projects.go`'s `registerProjectTools`.
    - `internal/mcp/bridge_test.go` (full file) — the four existing tests. The three `TestBridge_ListProjects_*` tests call `b.listProjects(context.Background())` (one-arg); after this task they call `b.listProjects(ctx, req)` (two-arg) where `req` is a `*mcp.CallToolRequest` with empty `Params.Arguments`. `TestNewBridgeFromEnv_DefaultsAndOverride` is unchanged.
    - `internal/mcp/server_test.go` (full file) — the two SC2 regression tests. This task MUST NOT modify this file; the tests must stay GREEN byte-for-byte.
    - `.planning/phases/06-mcp-subcommand-foundation/06-02-SUMMARY.md` — the API-drift auto-fixes Phase 06 hit: `Tool.InputSchema` is `any` (not `json.RawMessage` which marshals as base64), so use `map[string]any`; `cdr.HelpCommand()` is the method form. This plan inherits both — every InputSchema is a `map[string]any`.
  </read_first>
  <action>
    1. **Extract `bridge.call` shared helper into `internal/mcp/bridge.go`.** Add a new method `func (b *bridge) call(ctx context.Context, method, path string, body io.Reader) (*mcp.CallToolResult, error)` whose body is the response-handling half of the current `listProjects` (bridge.go lines 81-97): `resp, err := b.do(ctx, method, path, body)` → `if err != nil { return nil, fmt.Errorf("kamacu bridge: %w", err) }` → `defer resp.Body.Close()` → `respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))` → `if err != nil { return nil, fmt.Errorf("kamacu %s %s: read body: %w", method, path, err) }` → `if resp.StatusCode != http.StatusOK { return nil, fmt.Errorf("kamacu %s %s: HTTP %d: %s", method, path, resp.StatusCode, string(respBody)) }` (D-05 wrap — body passed verbatim as string; shape-agnostic per Gap 2) → `return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(respBody)}}}, nil`.

    2. **REMOVE `listProjects` from `internal/mcp/bridge.go`.** Delete the method (lines 69-97). It MOVES to `projects.go` in step 4 with the `workspace_id?` update. The shared primitives (`bridge struct`, `newBridgeFromEnv`, `bridge.do`, `maxBodyBytes`, `defaultBase`) STAY in bridge.go.

    3. **Refactor `internal/mcp/server.go`'s `registerTools`.** Replace the body of `registerTools` (lines 81-102 — the single `s.AddTool` for list_projects) with a three-line delegator:
       ```
       func registerTools(s *mcp.Server, b *bridge) {
           registerTaskTools(s, b)
           registerProjectTools(s, b)
           registerWorkspaceTools(s, b)
       }
       ```
       Remove the `mcp` import only if it becomes unused (it stays — `*mcp.Server` is a parameter type). The `ServeCommand`, `Execute`, `Name`/`Synopsis`/`Usage`/`SetFlags` methods above are byte-for-byte unchanged.

    4. **Create `internal/mcp/projects.go` (package `mcp`)** with `registerProjectTools` and the moved+updated `listProjects`:
       - Imports: `context`, `encoding/json`, `fmt`, `net/http`, `strings` (only if needed), `github.com/modelcontextprotocol/go-sdk/mcp`.
       - `func registerProjectTools(s *mcp.Server, b *bridge)` — calls `s.AddTool` EXACTLY ONCE for `list_projects` (the other 4 project tools land in Plan 03). The `Tool` definition:
         - `Name: "list_projects"`
         - `Description:` updated to drop the stale `project_id` no-op language (Pitfall 6) and describe the real `workspace_id?` arg + the `?workspace_id=` query param. Example: `"List all Kamacu projects, optionally scoped to a workspace. Returns the raw JSON array from GET /api/projects (with ?workspace_id=N when workspace_id is supplied)."`
         - `InputSchema: map[string]any{"type": "object", "properties": map[string]any{"workspace_id": map[string]any{"type": "integer", "description": "Optional workspace id. When supplied, only projects in that workspace are returned."}}}`  — note: NO `required` array (workspace_id is optional); NO `project_id` property (Pitfall 6 — the stale arg is gone).
       - The AddTool handler closure: `func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) { return b.listProjects(ctx, req) }` — passes `req` through (Phase 06 discarded it; Phase 07 needs it for arg extraction).
       - `func (b *bridge) listProjects(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)` — the moved+updated handler:
         - Declare `var args struct { WorkspaceID *int64 \`json:"workspace_id"\` }`.
         - `if len(req.Params.Arguments) > 0 { if err := json.Unmarshal(req.Params.Arguments, &args); err != nil { return nil, fmt.Errorf("list_projects: invalid arguments: %w", err) } }` — the `len > 0` guard avoids unmarshaling an empty/nil slice.
         - `path := "/api/projects"`; `if args.WorkspaceID != nil { path = fmt.Sprintf("/api/projects?workspace_id=%d", *args.WorkspaceID) }`.
         - `return b.call(ctx, http.MethodGet, path, nil)` — delegates to the shared helper.

    5. **Create `internal/mcp/tasks.go` (package `mcp`)** with an EMPTY registrar shell for Plan 02 to fill:
       - `package mcp`
       - Imports: `context`, `github.com/modelcontextprotocol/go-sdk/mcp` (the `*mcp.Server` and `*mcp.CallToolRequest` types will be referenced once Plan 02 adds tools; for the empty shell, only `context` and `mcp` are needed).
       - `func registerTaskTools(s *mcp.Server, b *bridge) {}` — empty body. A doc comment explains Plans 02 adds the six task tools here.
       - NO `s.AddTool` calls in this plan.

    6. **Create `internal/mcp/workspaces.go` (package `mcp`)** with an EMPTY registrar shell for Plan 04 to fill:
       - Same shape as `tasks.go` above: `package mcp`, imports `context` + `mcp`, `func registerWorkspaceTools(s *mcp.Server, b *bridge) {}` empty body. NO `s.AddTool` calls.

    7. **Update `internal/mcp/bridge_test.go`.** The three `TestBridge_ListProjects_*` tests change from `b.listProjects(context.Background())` to `b.listProjects(ctx, req)` where `req` is constructed as `&mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: nil}}` (or equivalent — the SDK's zero value for empty args). The test assertions (X-Kamacu-Token header sent, raw passthrough, HTTP 500 substring, `kamacu bridge:` substring) are unchanged. Add a NEW test `TestBridge_ListProjects_WorkspaceIDAppendsQueryParam`:
       - Stand up an `httptest.Server` capturing `r.URL.Path` and `r.URL.RawQuery`.
       - Construct the bridge: `b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10*time.Second}}`.
       - Construct the request: `req := &mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: json.RawMessage(`{"workspace_id":2}`)}}`.
       - Call `b.listProjects(context.Background(), req)`.
       - Assert NO error; assert the captured `r.URL.Path == "/api/projects"` AND `r.URL.RawQuery == "workspace_id=2"`.
       - Add a second sub-case or sibling test `TestBridge_ListProjects_NoWorkspaceID_NoQueryParam` proving that when `Arguments` is nil/empty, `r.URL.RawQuery == ""` (current behavior — backward-compatible).
       - `TestNewBridgeFromEnv_DefaultsAndOverride` is unchanged.

    8. **Verify the SC2 regression is byte-for-byte unchanged.** `git diff internal/mcp/server_test.go` should be empty. Run `go test ./internal/mcp/... -run TestSC2 -v` — both sub-tests must pass.

    9. **Verify the full mcp package.** `go vet ./internal/mcp/...` and `go test ./internal/mcp/... -v` — all tests (SC2 + list_projects + env-default + the new workspace_id? test) must pass.
  </action>
  <verify>
    <automated>
      set -e
      # bridge.call helper exists in bridge.go
      grep -q 'func (b \*bridge) call(' internal/mcp/bridge.go
      # listProjects REMOVED from bridge.go
      ! grep -q 'func (b \*bridge) listProjects(' internal/mcp/bridge.go
      # Three new per-resource files exist
      test -f internal/mcp/tasks.go
      test -f internal/mcp/projects.go
      test -f internal/mcp/workspaces.go
      # registerTools delegates to three registrars (no inline s.AddTool)
      grep -A4 'func registerTools(' internal/mcp/server.go | grep -q 'registerTaskTools'
      grep -A4 'func registerTools(' internal/mcp/server.go | grep -q 'registerProjectTools'
      grep -A4 'func registerTools(' internal/mcp/server.go | grep -q 'registerWorkspaceTools'
      ! grep -A6 'func registerTools(' internal/mcp/server.go | grep -q 's.AddTool'
      # list_projects lives in projects.go with workspace_id? arg (not project_id)
      grep -q 'func registerProjectTools(' internal/mcp/projects.go
      grep -q 'func (b \*bridge) listProjects(' internal/mcp/projects.go
      grep -q '"workspace_id"' internal/mcp/projects.go
      ! grep -q '"project_id"' internal/mcp/projects.go
      # tasks.go and workspaces.go have EMPTY registrars (no AddTool yet)
      ! grep -q 's.AddTool' internal/mcp/tasks.go
      ! grep -q 's.AddTool' internal/mcp/workspaces.go
      # SC2 test byte-for-byte unchanged
      test -z "$(git diff HEAD -- internal/mcp/server_test.go)"
      # No typed AddTool[In,Out] generic anywhere
      ! grep -rn 'mcp.AddTool\[' internal/mcp/
      # New workspace_id? test exists
      grep -q 'TestBridge_ListProjects_WorkspaceIDAppendsQueryParam' internal/mcp/bridge_test.go
      # go vet + tests pass
      go vet ./internal/mcp/...
      go test ./internal/mcp/... -v
      go test ./internal/mcp/... -run TestSC2 -v
    </automated>
  </verify>
  <acceptance_criteria>
    - `grep -q 'func (b \*bridge) call(ctx context.Context, method, path string, body io.Reader) (\*mcp.CallToolResult, error)' internal/mcp/bridge.go` succeeds (shared helper exists)
    - `grep -q 'fmt.Errorf("kamacu %s %s: HTTP %d: %s", method, path, resp.StatusCode, string(respBody))' internal/mcp/bridge.go` succeeds (D-05 wrap pattern verbatim, body as raw string)
    - `! grep -q 'func (b \*bridge) listProjects' internal/mcp/bridge.go` succeeds (method moved out)
    - `test -f internal/mcp/tasks.go && test -f internal/mcp/projects.go && test -f internal/mcp/workspaces.go` succeeds (D-07 files created)
    - `grep -A4 'func registerTools(' internal/mcp/server.go` shows exactly three registrar calls and NO inline `s.AddTool` (delegator shape)
    - `grep -q '"workspace_id"' internal/mcp/projects.go` succeeds (list_projects schema uses the real arg)
    - `! grep -q '"project_id"' internal/mcp/projects.go` succeeds (stale arg removed — Pitfall 6)
    - `! grep -q 's.AddTool' internal/mcp/tasks.go` succeeds (empty shell — Plan 02 fills it)
    - `! grep -q 's.AddTool' internal/mcp/workspaces.go` succeeds (empty shell — Plan 04 fills it)
    - `test -z "$(git diff HEAD -- internal/mcp/server_test.go)"` succeeds (SC2 test byte-for-byte unchanged — Pitfall 4)
    - `grep -q 'TestBridge_ListProjects_WorkspaceIDAppendsQueryParam' internal/mcp/bridge_test.go` succeeds (new coverage)
    - `go test ./internal/mcp/... -run TestSC2 -v` passes (both sub-tests GREEN — Pitfall 4)
    - `go test ./internal/mcp/... -v` passes (all mcp tests GREEN — list_projects with new signature, workspace_id? coverage, env-default unchanged)
    - `! grep -rn 'mcp.AddToast\[' internal/mcp/` — typo check; the real assertion is `! grep -rn 'mcp.AddTool\[' internal/mcp/` succeeds (no typed generic — Phase 06 lock)
  </acceptance_criteria>
  <done>
    - internal/mcp/bridge.go has bridge.call helper; listProjects method removed
    - internal/mcp/projects.go owns registerProjectTools + the moved/updated listProjects (workspace_id? arg)
    - internal/mcp/tasks.go and internal/mcp/workspaces.go have empty registerXTools shells
    - internal/mcp/server.go's registerTools delegates to the three registrars (no inline AddTool)
    - internal/mcp/bridge_test.go updated for the new (ctx, req) listProjects signature + new workspace_id? test
    - internal/mcp/server_test.go byte-for-byte unchanged; SC2 tests GREEN
    - `go vet ./internal/mcp/...` and `go test ./internal/mcp/...` pass
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| agent CLI → MCP subcommand stdin | Unchanged from Phase 06 — TRUSTED channel (the agent CLI is spawned by Kamacu inside a task PTY). |
| MCP subcommand → Kamacu HTTP API (loopback) | Plain HTTP to `127.0.0.1:7333`. Loopback binding remains the auth boundary (Phase 06 D-06). The two new GET routes inherit the same boundary — no new middleware. |
| Kamacu SQLite DB ← new GET handlers | Read-only SELECTs (no writes). The `?workspace_id=` query param is parameterized (`?` placeholder), not string-concatenated — no SQL injection surface. |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-07-01 | Spoofing | New GET routes have no auth check | low | accept | Inherits Phase 06 D-06: loopback binding (`hostCheck` + `ensureLoopback` + WS Origin allowlist) is the v1.11 auth boundary. No server-side token middleware is added (explicitly out of v1.11 scope). |
| T-07-02 | Injection | `?workspace_id=` query param in projectHandlers.list | low | mitigate | The query param is passed via a `?` SQL placeholder (`WHERE workspace_id = ?`, value from `strconv.Atoi`), never string-concatenated into the SELECT. An unparseable value is silently ignored (returns all projects) — no error path, no injection surface. |
| T-07-03 | Information Disclosure | `GET /api/projects/{id}` returns full project JSON to any loopback caller | low | accept | Same loopback trust boundary as every other `/api/*` route. The response shape is identical to what `GET /api/projects` already returns per-project. No new field exposed. |
| T-07-04 | Tampering | D-07 code-motion could regress SC2 or list_projects behavior | medium | mitigate | Task 2 step 8 enforces `git diff internal/mcp/server_test.go` is empty AND `go test ./internal/mcp/... -run TestSC2 -v` stays GREEN. The existing list_projects tests are UPDATED (not deleted) to the new `(ctx, req)` signature with assertions unchanged. |

</threat_model>

<verification>
- `internal/api/routes.go` registers `GET /api/tasks` and `GET /api/projects/{id}` (2 new lines)
- `internal/api/tasks.go` defines `taskHandlers.list` (manual-only, ordered by status/position)
- `internal/api/projects.go` defines `projectHandlers.get` (single-project fetch, 404 on missing) and `projectHandlers.list` reads `?workspace_id=` (filters when parseable, ignores when not)
- `internal/mcp/{tasks.go, projects.go, workspaces.go}` exist; the latter two define `registerXTools(s, b)`
- `internal/mcp/server.go`'s `registerTools` is a 3-line delegator (no inline AddTool)
- `internal/mcp/bridge.go` defines `bridge.call` and NO LONGER defines `listProjects`
- `internal/mcp/projects.go` defines `registerProjectTools` (1 AddTool for list_projects) and the moved `listProjects(ctx, req)` with `workspace_id?` arg
- `internal/mcp/tasks.go` and `internal/mcp/workspaces.go` have empty `registerXTools` bodies
- `internal/mcp/server_test.go` is byte-for-byte unchanged; SC2 tests GREEN
- `internal/mcp/bridge_test.go` updated for new listProjects signature + new workspace_id? test
- `go vet ./internal/api/... ./internal/mcp/...` passes
- `go test ./internal/api/... ./internal/mcp/...` passes
</verification>

<success_criteria>
This plan delivers MCPPROJ-01 (`list_projects(workspace_id?)` — the workspace_id? arg is now functional via D-02's query param) and lays the foundation for Plans 02/03/04 to deliver MCPTASK-01..06, MCPPROJ-02..05, MCPPROJ-06..07. The three Kamacu endpoint additions (D-01, D-02, Gap 1) are the prerequisites for MCPTASK-01 (list_tasks needs GET /api/tasks), MCPPROJ-01 (list_projects needs ?workspace_id=), and MCPPROJ-02 (get_project needs GET /api/projects/{id}). The D-07 scaffold (three new files + empty registrars + bridge.call helper) is the prerequisite for the parallel Wave 2 plans.
</success_criteria>

<output>
Create `.planning/phases/07-tasks-projects-workspaces-tools/07-01-SUMMARY.md` when done
</output>

## Artifacts this phase produces

This plan creates the following new symbols / files / struct fields (consumed by Plans 02/03/04 and by the plan-review-convergence source-grounding pass):

- **Kamacu API routes (new, in `internal/api/routes.go`):**
  - `mux.HandleFunc("GET /api/tasks", t.list)` — D-01
  - `mux.HandleFunc("GET /api/projects/{id}", p.get)` — Gap 1
- **Kamacu API handlers (new methods):**
  - `func (h *taskHandlers) list(w http.ResponseWriter, r *http.Request)` in `internal/api/tasks.go` — D-01 (unscoped manual-only task list)
  - `func (h *projectHandlers) get(w http.ResponseWriter, r *http.Request)` in `internal/api/projects.go` — Gap 1 (single-project fetch, 404 on missing)
- **Kamacu API handler (modified):**
  - `projectHandlers.list` in `internal/api/projects.go` — D-02 extension: reads `r.URL.Query().Get("workspace_id")`, parameterized `WHERE workspace_id = ?` when parseable
- **MCP package files (new):**
  - `internal/mcp/tasks.go` — `func registerTaskTools(s *mcp.Server, b *bridge)` (empty body in this plan; Plan 02 fills it)
  - `internal/mcp/projects.go` — `func registerProjectTools(s *mcp.Server, b *bridge)` + `func (b *bridge) listProjects(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)` (moved from bridge.go, updated for `workspace_id?`)
  - `internal/mcp/workspaces.go` — `func registerWorkspaceTools(s *mcp.Server, b *bridge)` (empty body in this plan; Plan 04 fills it)
- **MCP bridge helper (new method on `*bridge`):**
  - `func (b *bridge) call(ctx context.Context, method, path string, body io.Reader) (*mcp.CallToolResult, error)` in `internal/mcp/bridge.go` — the shared response-handling half every Phase 07 tool handler delegates to
- **MCP method (moved):**
  - `bridge.listProjects` — physically relocates from `internal/mcp/bridge.go` to `internal/mcp/projects.go` (same package; signature changes from `(ctx)` to `(ctx, req *mcp.CallToolRequest)`; gains `workspace_id?` arg extraction)
- **MCP function (modified):**
  - `registerTools(s *mcp.Server, b *bridge)` in `internal/mcp/server.go` — shrinks from a single inline `s.AddTool` for list_projects to a 3-line delegator calling `registerTaskTools` / `registerProjectTools` / `registerWorkspaceTools`
- **MCP tool (modified, was FINAL Phase 06 production code):**
  - `list_projects` — InputSchema changes from `{project_id?: integer}` to `{workspace_id?: integer}`; description drops stale `project_id` no-op language; handler now extracts `workspace_id` and appends `?workspace_id=N` to the path when supplied (Pitfall 6)
- **Test functions (new):**
  - `internal/api/tasks_test.go`: `TestTaskList_All_ReturnsOnlyManualTasksOrderedByStatusPosition`, `TestTaskList_EmptyReturnsEmptyArray`
  - `internal/api/projects_test.go`: `TestProjectGet_Found_ReturnsProjectJSON`, `TestProjectGet_NotFound_Returns404`, `TestProjectList_WorkspaceIDFilter_ReturnsOnlyThatWorkspace`, `TestProjectList_NoFilter_ReturnsAll`, `TestProjectList_WorkspaceIDUnparseable_IgnoresAndReturnsAll`
  - `internal/mcp/bridge_test.go`: `TestBridge_ListProjects_WorkspaceIDAppendsQueryParam` (and `TestBridge_ListProjects_NoWorkspaceID_NoQueryParam` sub-case)
- **Test functions (modified for new signature, NOT new):**
  - `TestBridge_ListProjects_PassesThroughKamacuJSON`, `TestBridge_ListProjects_KamacuReturnsNon200_ReturnsError`, `TestBridge_ListProjects_KamacuUnreachable_ReturnsError` — updated to call `b.listProjects(ctx, req)` instead of `b.listProjects(ctx)`
