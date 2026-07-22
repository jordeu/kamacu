---
phase: 07-tasks-projects-workspaces-tools
plan: 04
slug: workspaces-tools
type: execute
wave: 2
depends_on:
  - 07-01-endpoints-and-mcp-scaffold
files_modified:
  - internal/mcp/workspaces.go
  - internal/mcp/workspaces_test.go
autonomous: true
requirements: [MCPPROJ-06, MCPPROJ-07]
must_haves:
  truths:
    # MCPPROJ-06: workspace CRUD (list/create/update/delete) — v1.9 guarded delete keyed off is_default + non-empty COUNT
    - "`registerWorkspaceTools` in `internal/mcp/workspaces.go` registers four tools: `list_workspaces` (no args), `create_workspace` (`{name: string (required)}`), `update_workspace` (`{workspace_id: integer (required), name?: string}` per D-03 — single-field rename), `delete_workspace` (`{workspace_id: integer (required)}`). Each bridges to the corresponding v1.9 `/api/workspaces` route. The existing two delete guards (is_default → 409 \"the default workspace can't be deleted\"; non-empty COUNT → 409 \"move or remove its N project(s) first\") run unchanged through the bridge."
    # MCPPROJ-07: move_project_to_workspace(project_id, workspace_id) — reuses v1.9 PATCH /api/projects/{id} workspace_id branch
    - "`registerWorkspaceTools` registers a fifth tool `move_project_to_workspace` with InputSchema `{properties: {project_id: {type: integer}, workspace_id: {type: integer}}, required: [project_id, workspace_id]}`. The handler marshals `{\"workspace_id\": args.WorkspaceID}` as the PATCH body and calls `b.call(ctx, http.MethodPatch, fmt.Sprintf(\"/api/projects/%d\", args.ProjectID), bytes.NewReader(body))` — bridging to the existing `PATCH /api/projects/{id}` route's WorkspaceID branch (projects.go:557-575). Kamacu's existing validation (target workspace exists → 400 \"workspace not found\" if not) applies unchanged."
    # Pattern fidelity — every handler delegates to bridge.call
    - "Every workspace tool handler in `workspaces.go` follows the same shape Plans 02/03 established: declare a typed args struct (or no args for list_workspaces) → `json.Unmarshal(req.Params.Arguments, &args)` where args exist (with error wrap) → build path/body → `return b.call(ctx, method, path, body)`. NO handler re-implements the `b.do` → `defer Close` → `io.ReadAll(LimitReader)` sequence inline — all delegate to `bridge.call` (extracted in Plan 01)."
    # move_project_to_workspace is the workspace-resource tool that PATCHes the project route (intentional — the underlying v1.9 surface reuses PATCH /api/projects/{id})
    - "`move_project_to_workspace` is registered in `registerWorkspaceTools` (not `registerProjectTools`) because it is a workspace-management action (transferring a project INTO a workspace), even though it PATCHes the project route. The InputSchema exposes `project_id` and `workspace_id` as required args; the body contains ONLY `workspace_id` (D-03 excludes workspace_id from update_project specifically because this dedicated tool owns the transfer)."
    # Test breadth (D-08)
    - "`internal/mcp/workspaces_test.go` exists with one happy + one representative error case per tool (~8 total sub-tests): `TestBridge_ListWorkspaces_*`, `TestBridge_CreateWorkspace_*`, `TestBridge_UpdateWorkspace_*`, `TestBridge_DeleteWorkspace_*`, `TestBridge_MoveProjectToWorkspace_*`. Each test stands up an `httptest.Server`, constructs a `*bridge`, invokes the handler, and asserts the captured request AND result. The delete-guard tests assert the v1.9 409 messages (`\"the default workspace\"`, `\"project(s) first\"`) as substrings — NOT JSON key names (07-RESEARCH Gap 2)."
    # No bridge-side validation (Kamacu's handlers validate)
    - "`workspaces.go` performs NO validation of workspace_id values, name emptiness, or target-workspace existence — it sends fields as-is and lets Kamacu's existing v1.9 handlers validate (the existing gates: empty name → 400 \"name is required\"; case-insensitive dup → 409 \"a workspace with that name already exists\"; is_default delete → 409 \"the default workspace can't be deleted\"; non-empty delete → 409 \"move or remove its N project(s) first\"; missing workspace → 404 \"workspace not found\"; move target missing → 400 \"workspace not found\"). SC4 is satisfied by NOT duplicating validation in the bridge."
  artifacts:
    - internal/mcp/workspaces.go (EXTENDED — registerWorkspaceTools body filled with 5 s.AddTool calls; NEW bridge methods: listWorkspaces, createWorkspace, updateWorkspace, deleteWorkspace, moveProjectToWorkspace)
    - internal/mcp/workspaces_test.go (NEW — ~8 test cases covering all 5 tools, happy + error per D-08)
  key_links:
    - "registerWorkspaceTools(s, b) → 5 s.AddTool calls for list_workspaces, create_workspace, update_workspace, delete_workspace, move_project_to_workspace"
    - "bridge.listWorkspaces(ctx, req) → b.call(GET, /api/workspaces, nil)"
    - "bridge.createWorkspace(ctx, req) → b.call(POST, /api/workspaces, {name} body)"
    - "bridge.updateWorkspace(ctx, req) → b.call(PATCH, /api/workspaces/{id}, {name} body — D-03 single-field rename)"
    - "bridge.deleteWorkspace(ctx, req) → b.call(DELETE, /api/workspaces/{id}, nil) — v1.9 guarded delete runs unchanged"
    - "bridge.moveProjectToWorkspace(ctx, req) → b.call(PATCH, /api/projects/{id}, {workspace_id} body) — reuses the v1.9 PATCH workspace_id branch"
  prohibitions:
    - statement: "NO bridge-side validation — Kamacu's v1.9 handlers are the validator. The bridge sends workspace_id / name / project_id as-is; an invalid value surfaces Kamacu's 400/404/409 verbatim through the D-05 wrap."
      status: resolved
      verification: "internal/mcp/workspaces.go contains no validation function calls; no `if args.Name == \"\"` style guard; fields pass straight into the body map / path"
    - statement: "NO new route on /api/projects for move_project_to_workspace — the tool reuses the EXISTING PATCH /api/projects/{id} route's WorkspaceID branch (projects.go:557-575). The bridge sends `{workspace_id: N}` as the PATCH body; Kamacu's existing validation (target exists → 400 \"workspace not found\") applies unchanged."
      status: resolved
      verification: "internal/mcp/workspaces.go's moveProjectToWorkspace handler calls b.call with http.MethodPatch and path /api/projects/{id}; it does NOT call any /api/workspaces route"
    - statement: "DO NOT touch internal/api/, internal/mcp/server.go, internal/mcp/bridge.go, internal/mcp/tasks.go, or internal/mcp/projects.go — this plan owns ONLY internal/mcp/workspaces.go and internal/mcp/workspaces_test.go. Plan 01 already shipped the bridge.call helper and the registerWorkspaceTools shell."
      status: resolved
      verification: "`git diff --name-only` for this plan shows exactly two files: internal/mcp/workspaces.go and internal/mcp/workspaces_test.go"
    - statement: "DO NOT parse Kamacu error JSON in the bridge — D-05 passes the body as a raw string. Test assertions MUST use substrings of Kamacu message text (e.g. `\"already exists\"`, `\"the default workspace\"`, `\"project(s) first\"`, `\"workspace not found\"`), NOT JSON key names (07-RESEARCH Gap 2 / Pitfall 2: real shape is `{\"error\":\"...\"}`)."
      status: resolved
      verification: "internal/mcp/workspaces_test.go contains no assertion matching on the literal string `\"message\"` or `\"status\"` as a JSON key"
    - statement: "DO NOT use the typed `mcp.AddTool[In, Out any]` generic — keep using the low-level `s.AddTool(*mcp.Tool, handler)` with `map[string]any` InputSchema (Phase 06 lock, 06-02-SUMMARY)."
      status: resolved
      verification: "`grep -rn 'mcp.AddTool\\[' internal/mcp/workspaces.go` returns zero results"
---

# Plan 04: Five workspace tools (list_workspaces, create_workspace, update_workspace, delete_workspace, move_project_to_workspace)

<objective>
Fill the `registerWorkspaceTools` shell Plan 01 created with the five MCPPROJ-06/07 tools. Four are workspace CRUD (`list_workspaces`, `create_workspace`, `update_workspace`, `delete_workspace`) bridging to the v1.9 `/api/workspaces` routes; the fifth (`move_project_to_workspace`) is a workspace-management action that PATCHes the project route's `workspace_id` branch (the v1.9 transfer surface). Every tool is a thin HTTP-bridge handler delegating to `bridge.call`. This plan delivers MCPPROJ-06 and MCPPROJ-07 and satisfies SC3 (the four workspace tools + the transfer tool drive the switcher, including the v1.9 guarded delete keyed off `is_default` + non-empty `COUNT`).

Purpose: complete the bridge-pattern stress test. With Plan 02 (tasks) and Plan 03 (projects), this plan brings the total to 13 tools — proving the pattern scales from Phase 06's single `list_projects` to the full CRUD surface. The v1.9 guarded delete (two 409 cases) and the cross-route transfer (PATCH /api/projects/{id} from the workspace resource) are the only nuances — both resolved by sending fields as-is and letting Kamacu's existing handlers validate.

Output: `internal/mcp/workspaces.go` with five `bridge.<verb>Workspace`/`moveProjectToWorkspace` methods + five `s.AddTool` calls in `registerWorkspaceTools`; `internal/mcp/workspaces_test.go` with ~8 happy/error test cases per D-08.
</objective>

<execution_context>
@/home/jordi/.config/opencode/gsd-core/workflows/execute-plan.md
@/home/jordi/.config/opencode/gsd-core/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/phases/07-tasks-projects-workspaces-tools/07-CONTEXT.md
@.planning/phases/07-tasks-projects-workspaces-tools/07-RESEARCH.md
@.planning/phases/07-tasks-projects-workspaces-tools/07-01-SUMMARY.md
@internal/api/workspaces.go
@internal/api/projects.go
@internal/api/routes.go
@internal/mcp/bridge.go
@internal/mcp/workspaces.go
@internal/mcp/bridge_test.go
</context>

<tasks>

<task type="auto">
  <name>Task 1: Implement the five workspace tool handlers in registerWorkspaceTools (list_workspaces, create_workspace, update_workspace, delete_workspace, move_project_to_workspace)</name>
  <files>internal/mcp/workspaces.go</files>
  <read_first>
    - `.planning/phases/07-tasks-projects-workspaces-tools/07-RESEARCH.md` § "Tool-to-Endpoint Mapping → Workspace tools" (the table with MCP args, bridge HTTP call, Kamacu handler line reference, and per-tool notes for all five tools — note move_project_to_workspace PATCHes /api/projects/{id} not /api/workspaces), § "Detailed Findings → Gap 2" (the v1.9 workspace delete 409 shapes: `{"error":"the default workspace can't be deleted"}` for is_default, `{"error":"move or remove its 3 project(s) first"}` for non-empty), § "Architecture Patterns → Tool-handler canonical shape", § "Common Pitfalls → Pitfall 2" (test assertions on Kamacu message substrings)
    - `.planning/phases/07-tasks-projects-workspaces-tools/07-CONTEXT.md` decisions D-03 (`update_workspace` is `{workspace_id, name?}` — single-field rename; matches the underlying PATCH shape), D-05 (error wrap), D-07 (workspaces.go owns the workspace resource + move_project_to_workspace), D-08 (one representative test per tool per resource — ~8 cases for workspaces: 5 tools × ~1-2 cases each)
    - `internal/api/workspaces.go` (relevant ranges) — lines 77-98 (`list` — returns `[]`, never nil), lines 103-132 (`create` — empty name → 400 "name is required"; case-insensitive dup → 409 "a workspace with that name already exists"; returns 201), lines 137-179 (`update` — `*string` Name field, nil → 400 "nothing to update", empty → 400 "name is required"; dup-excluding-self → 409; default workspace IS renamable; returns 200 or 404), lines 192-233 (`delete` — two guards: `is_default == 1` → 409 "the default workspace can't be deleted"; `COUNT > 0` → 409 "move or remove its N project(s) first"; empty non-default → 204 or 404)
    - `internal/api/projects.go` lines 557-575 — the `update` handler's WorkspaceID branch (the v1.9 transfer surface `move_project_to_workspace` reuses): validates target workspace exists via `SELECT 1 FROM workspaces WHERE id = ?` → 400 "workspace not found" on missing → appends `workspace_id = ?` to the PATCH sets.
    - `internal/api/routes.go` lines 22-25 — the workspace routes (`GET /api/workspaces`, `POST /api/workspaces`, `PATCH /api/workspaces/{id}`, `DELETE /api/workspaces/{id}`) this plan bridges to, plus line 34 (`PATCH /api/projects/{id}`) for move_project_to_workspace.
    - `internal/mcp/workspaces.go` (post-Plan-01) — the empty `registerWorkspaceTools(s, b) {}` shell this plan fills.
    - `internal/mcp/bridge.go` (post-Plan-01) — the `bridge.call(ctx, method, path, body)` helper every handler delegates to.
    - `internal/mcp/bridge_test.go` (post-Plan-01) — the `TestBridge_ListProjects_*` test pattern this plan's `workspaces_test.go` mirrors.
  </read_first>
  <action>
    Fill `registerWorkspaceTools` in `internal/mcp/workspaces.go` with five `s.AddTool` calls and add the five corresponding `*bridge` methods. Each method follows the canonical shape Plans 02/03 established: typed args struct (or none for list_workspaces) → `json.Unmarshal(req.Params.Arguments, &args)` where args exist (wrap unmarshal errors as `fmt.Errorf("<tool_name>: invalid arguments: %w", err)`) → build path/body → `return b.call(ctx, method, path, body)`. Imports needed: `bytes`, `context`, `encoding/json`, `fmt`, `net/http`, `github.com/modelcontextprotocol/go-sdk/mcp`.

    1. **list_workspaces (MCPPROJ-06).** InputSchema: `{type: object, properties: {}}` (no args). Description: "List all Kamacu workspaces. Returns the raw JSON array from GET /api/workspaces (never nil — empty array when no workspaces exist)." Handler `bridge.listWorkspaces(ctx, req)`:
       - No args extraction needed (the handler ignores `req.Params.Arguments`).
       - `return b.call(ctx, http.MethodGet, "/api/workspaces", nil)`

    2. **create_workspace (MCPPROJ-06).** InputSchema: `{type: object, properties: {name: {type: string, description: "Workspace name. Case-insensitive duplicate → 409."}}, required: [name]}`. Handler `bridge.createWorkspace(ctx, req)`:
       - `var args struct { Name string \`json:"name"\` }`
       - unmarshal with error wrap
       - `body, _ := json.Marshal(map[string]any{"name": args.Name})`
       - `return b.call(ctx, http.MethodPost, "/api/workspaces", bytes.NewReader(body))`

    3. **update_workspace (MCPPROJ-06, D-03 single-field rename).** InputSchema: `{type: object, properties: {workspace_id: {type: integer, description: "..."}, name: {type: string, description: "New workspace name. The default workspace IS renamable. Case-insensitive duplicate (excluding self) → 409."}}, required: [workspace_id]}` (name optional per D-03 — though Kamacu's handler treats nil name as 400 "nothing to update"; the bridge sends as-is and lets Kamacu validate). Handler `bridge.updateWorkspace(ctx, req)`:
       - `var args struct { WorkspaceID int64 \`json:"workspace_id"\`; Name *string \`json:"name"\` }` — pointer field for name (D-03: nil = omitted, non-nil = supplied).
       - unmarshal with error wrap
       - Build body conditionally: `body := map[string]any{}; if args.Name != nil { body["name"] = *args.Name }`
       - `jsonBody, _ := json.Marshal(body)`
       - `return b.call(ctx, http.MethodPatch, fmt.Sprintf("/api/workspaces/%d", args.WorkspaceID), bytes.NewReader(jsonBody))`

    4. **delete_workspace (MCPPROJ-06 — v1.9 guarded delete).** InputSchema: `{type: object, properties: {workspace_id: {type: integer, description: "The workspace id. The default workspace (is_default) → 409. A workspace still owning projects → 409 with a count-carrying message."}}, required: [workspace_id]}`. Handler `bridge.deleteWorkspace(ctx, req)`:
       - `var args struct { WorkspaceID int64 \`json:"workspace_id"\` }`
       - unmarshal with error wrap
       - `return b.call(ctx, http.MethodDelete, fmt.Sprintf("/api/workspaces/%d", args.WorkspaceID), nil)` — the v1.9 two-guard delete (is_default → 409, non-empty COUNT → 409) runs unchanged.

    5. **move_project_to_workspace (MCPPROJ-07 — reuses v1.9 PATCH /api/projects/{id}).** InputSchema: `{type: object, properties: {project_id: {type: integer, description: "The project to transfer."}, workspace_id: {type: integer, description: "The target workspace id. Must exist → 400 \"workspace not found\" if not."}}, required: [project_id, workspace_id]}`. Handler `bridge.moveProjectToWorkspace(ctx, req)`:
       - `var args struct { ProjectID int64 \`json:"project_id"\`; WorkspaceID int64 \`json:"workspace_id"\` }`
       - unmarshal with error wrap
       - `body, _ := json.Marshal(map[string]any{"workspace_id": args.WorkspaceID})` — body contains ONLY workspace_id (this is the dedicated transfer tool; update_project in Plan 03 excludes workspace_id precisely because this tool owns it).
       - `return b.call(ctx, http.MethodPatch, fmt.Sprintf("/api/projects/%d", args.ProjectID), bytes.NewReader(body))` — bridges to the existing PATCH /api/projects/{id} route's WorkspaceID branch (projects.go:557-575). Kamacu validates the target exists → 400 "workspace not found" if not.

    6. **Register all five** in `registerWorkspaceTools` via `s.AddTool(&mcp.Tool{Name: "...", Description: "...", InputSchema: map[string]any{...}}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) { return b.<verb>Workspace(ctx, req) })` (or `b.moveProjectToWorkspace` for the transfer tool). Descriptions should be agent-readable.
  </action>
  <verify>
    <automated>
      set -e
      # All five tools registered
      for tool in list_workspaces create_workspace update_workspace delete_workspace move_project_to_workspace; do
        grep -q "Name: \"$tool\"" internal/mcp/workspaces.go || { echo "missing $tool"; exit 1; }
      done
      # All five bridge methods exist
      for method in listWorkspaces createWorkspace updateWorkspace deleteWorkspace moveProjectToWorkspace; do
        grep -q "func (b \*bridge) $method(" internal/mcp/workspaces.go || { echo "missing $method"; exit 1; }
      done
      # move_project_to_workspace PATCHes /api/projects/{id} (NOT a /api/workspaces route)
      grep -A10 'func (b \*bridge) moveProjectToWorkspace' internal/mcp/workspaces.go | grep -q '/api/projects/'
      grep -A10 'func (b \*bridge) moveProjectToWorkspace' internal/mcp/workspaces.go | grep -q 'http.MethodPatch'
      # update_workspace uses pointer Name field (D-03)
      grep -q 'Name \*string' internal/mcp/workspaces.go
      # Every handler delegates to b.call (no inline b.do + io.ReadAll)
      ! grep -q 'io.ReadAll' internal/mcp/workspaces.go
      ! grep -q 'b\.do(' internal/mcp/workspaces.go
      # No typed AddTool[In,Out] generic
      ! grep -q 'mcp.AddTool\[' internal/mcp/workspaces.go
      # No bridge-side validation
      ! grep -q 'if args\..*== ""' internal/mcp/workspaces.go
      # go vet + build
      go vet ./internal/mcp/...
      go build ./...
    </automated>
  </verify>
  <acceptance_criteria>
    - `grep -c 'Name: "' internal/mcp/workspaces.go` returns 5
    - For each of list_workspaces/create_workspace/update_workspace/delete_workspace/move_project_to_workspace: `grep -q "Name: \"<tool>\"" internal/mcp/workspaces.go` succeeds
    - For each of listWorkspaces/createWorkspace/updateWorkspace/deleteWorkspace/moveProjectToWorkspace: `grep -q "func (b \*bridge) <method>(" internal/mcp/workspaces.go` succeeds
    - `grep -A10 'func (b \*bridge) moveProjectToWorkspace' internal/mcp/workspaces.go | grep -q '/api/projects/'` succeeds (PATCHes the project route, not a workspace route)
    - `grep -A10 'func (b \*bridge) moveProjectToWorkspace' internal/mcp/workspaces.go | grep -q 'http.MethodPatch'` succeeds
    - `grep -q 'Name \*string' internal/mcp/workspaces.go` succeeds (update_workspace pointer field — D-03)
    - `! grep -q 'b\.do(' internal/mcp/workspaces.go` succeeds (every handler delegates to b.call)
    - `! grep -q 'io\.ReadAll' internal/mcp/workspaces.go` succeeds (no inline response reading)
    - `! grep -q 'mcp\.AddTool\[' internal/mcp/workspaces.go` succeeds (no typed generic)
    - `go vet ./internal/mcp/...` passes
    - `go build ./...` passes
  </acceptance_criteria>
  <done>
    - registerWorkspaceTools registers all five tools (list_workspaces, create_workspace, update_workspace, delete_workspace, move_project_to_workspace)
    - Five bridge methods each delegate to b.call
    - move_project_to_workspace PATCHes /api/projects/{id} with {workspace_id} body (reuses v1.9 transfer surface)
    - update_workspace uses *string pointer Name field (D-03)
    - No handler re-implements b.do + io.ReadAll; no bridge-side validation
    - go vet + go build pass
  </done>
</task>

<task type="auto">
  <name>Task 2: Add workspaces_test.go with ~8 happy/error test cases per D-08 (including v1.9 guarded-delete 409 cases)</name>
  <files>internal/mcp/workspaces_test.go</files>
  <read_first>
    - `.planning/phases/07-tasks-projects-workspaces-tools/07-RESEARCH.md` § "Detailed Findings → Gap 2" (the v1.9 workspace delete 409 shapes: `{"error":"the default workspace can't be deleted"}` for is_default guard, `{"error":"move or remove its 3 project(s) first"}` for non-empty guard — keyed on `error` not `message`), § "Common Pitfalls → Pitfall 2" (test assertions on Kamacu message substrings, not JSON key names)
    - `.planning/phases/07-tasks-projects-workspaces-tools/07-CONTEXT.md` D-08 (one representative test per tool per resource — ~8 cases for workspaces: 5 tools × ~1-2 cases each)
    - `internal/mcp/bridge_test.go` (post-Plan-01) — the `httptest.NewServer` + `&bridge{base: srv.URL, ...}` + `*mcp.CallToolRequest` pattern this plan mirrors.
    - `internal/mcp/workspaces.go` (post-Task-1 of this plan) — the production code under test.
    - `internal/api/workspaces.go` — the v1.9 handler behavior the test server simulates (e.g. `list` returns `[]`; `create` returns 201 or 400/409; `update` returns 200 or 400/404/409; `delete` returns 204 or 404/409 with the two guard messages).
    - `internal/api/projects.go` lines 557-575 — the WorkspaceID branch of update that move_project_to_workspace triggers (target validation → 400 "workspace not found").
  </read_first>
  <action>
    Create `internal/mcp/workspaces_test.go` (package `mcp`) with one happy + one representative error case per tool (~8 total sub-tests). Follow the `TestBridge_<Verb><Resource>_<Scenario>` naming convention. Imports: `context`, `encoding/json`, `net/http`, `net/http/httptest`, `strings`, `testing`, `time`, `github.com/modelcontextprotocol/go-sdk/mcp`.

    Each test stands up an `httptest.Server` capturing `r.Method`, `r.URL.Path`, and request body, returns a canned response, and asserts the captured request + result. Required cases:

    1. `TestBridge_ListWorkspaces_Happy_CallsGetPath` — Arguments `{}`; assert `r.URL.Path == "/api/workspaces"` AND method GET AND TextContent passthrough of the canned `[]` array.
    2. `TestBridge_CreateWorkspace_Happy_SendsNameBody` — Arguments `{"name":"Research"}`; assert method POST AND `r.URL.Path == "/api/workspaces"` AND body decodes to `{"name":"Research"}`.
    3. `TestBridge_CreateWorkspace_DuplicateName_Kamacu409` — server returns 409 with `{"error":"a workspace with that name already exists"}`; assert error contains `"HTTP 409"` (substring, not JSON key).
    4. `TestBridge_UpdateWorkspace_Happy_SendsNameBody` — Arguments `{"workspace_id":2,"name":"Renamed"}`; assert method PATCH AND `r.URL.Path == "/api/workspaces/2"` AND body decodes to `{"name":"Renamed"}`.
    5. `TestBridge_UpdateWorkspace_NoName_EmptyBody_Kamacu400` — Arguments `{"workspace_id":2}` (name omitted); assert method PATCH AND body is `{}` (no name key) AND the canned 400 `{"error":"nothing to update"}` surfaces as an error containing `"HTTP 400"`.
    6. `TestBridge_DeleteWorkspace_Happy_CallsDeletePath` — Arguments `{"workspace_id":3}`; assert `r.URL.Path == "/api/workspaces/3"` AND method DELETE.
    7. `TestBridge_DeleteWorkspace_DefaultGuard_Kamacu409` — server returns 409 with `{"error":"the default workspace can't be deleted"}`; assert error contains `"HTTP 409"` AND `"the default workspace"` (substring of Kamacu message — NOT a JSON key assertion per Gap 2).
    8. `TestBridge_DeleteWorkspace_NonEmptyGuard_Kamacu409` — server returns 409 with `{"error":"move or remove its 3 project(s) first"}`; assert error contains `"HTTP 409"` AND `"project(s) first"` (substring).
    9. `TestBridge_MoveProjectToWorkspace_Happy_PATCHesProjectRoute` — Arguments `{"project_id":5,"workspace_id":2}`; assert method PATCH AND `r.URL.Path == "/api/projects/5"` (NOT /api/workspaces) AND body decodes to `{"workspace_id":2}` with NO other keys.
    10. `TestBridge_MoveProjectToWorkspace_TargetMissing_Kamacu400` — server returns 400 with `{"error":"workspace not found"}`; assert error contains `"HTTP 400"` AND `"workspace not found"`.

    Use `t.Cleanup(srv.Close)` for every test server. For body map assertions, unmarshal the captured body into `map[string]any` and check key presence/absence. For the move test, assert the body contains ONLY `workspace_id` (no `name`, no `description`, etc.).
  </action>
  <verify>
    <automated>
      set -e
      test -f internal/mcp/workspaces_test.go
      # All five tools have at least one happy + one error test
      for tool in ListWorkspaces CreateWorkspace UpdateWorkspace DeleteWorkspace MoveProjectToWorkspace; do
        grep -q "TestBridge_$tool" internal/mcp/workspaces_test.go || { echo "missing $tool tests"; exit 1; }
      done
      # v1.9 guarded-delete coverage: both 409 cases
      grep -q 'TestBridge_DeleteWorkspace_DefaultGuard' internal/mcp/workspaces_test.go
      grep -q 'TestBridge_DeleteWorkspace_NonEmptyGuard' internal/mcp/workspaces_test.go
      # move_project_to_workspace coverage: PATCHes /api/projects/{id}
      grep -B2 -A10 'TestBridge_MoveProjectToWorkspace_Happy' internal/mcp/workspaces_test.go | grep -q '/api/projects/'
      # No JSON key name assertions on error bodies (Gap 2 / Pitfall 2)
      ! grep -q '"message"' internal/mcp/workspaces_test.go
      # Uses httptest pattern
      grep -q 'httptest.NewServer' internal/mcp/workspaces_test.go
      # go vet + tests pass
      go vet ./internal/mcp/...
      go test ./internal/mcp/... -v
    </automated>
  </verify>
  <acceptance_criteria>
    - `test -f internal/mcp/workspaces_test.go` succeeds
    - `grep -c 'func TestBridge_' internal/mcp/workspaces_test.go` returns at least 8 (happy + error per the 5 tools, per D-08)
    - For each of ListWorkspaces/CreateWorkspace/UpdateWorkspace/DeleteWorkspace/MoveProjectToWorkspace: `grep -q "TestBridge_<Tool>_" internal/mcp/workspaces_test.go` succeeds
    - `grep -q 'TestBridge_DeleteWorkspace_DefaultGuard' internal/mcp/workspaces_test.go` succeeds (v1.9 is_default guard coverage)
    - `grep -q 'TestBridge_DeleteWorkspace_NonEmptyGuard' internal/mcp/workspaces_test.go` succeeds (v1.9 non-empty COUNT guard coverage)
    - `grep -B2 -A10 'TestBridge_MoveProjectToWorkspace_Happy' internal/mcp/workspaces_test.go | grep -q '/api/projects/'` succeeds (the transfer PATCHes the project route)
    - `! grep -q '"message"' internal/mcp/workspaces_test.go` succeeds (Gap 2 / Pitfall 2 — no JSON key name assertions)
    - `go test ./internal/mcp/... -v` passes (all workspace tests GREEN, plus Plan 01/02/03 tests still GREEN)
  </acceptance_criteria>
  <done>
    - internal/mcp/workspaces_test.go exists with ~8 test cases covering all 5 tools (happy + error each)
    - delete_workspace tests cover BOTH v1.9 guards (is_default 409, non-empty COUNT 409) with substring assertions (Gap 2)
    - move_project_to_workspace test asserts PATCH /api/projects/{id} with {workspace_id}-only body
    - No error-body assertion matches on JSON key names (Gap 2 / Pitfall 2)
    - `go test ./internal/mcp/... -v` passes
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| agent CLI → MCP subcommand stdin | Unchanged. The five new tools accept `*mcp.CallToolRequest`; arguments unmarshaled into typed structs. |
| MCP subcommand → Kamacu HTTP API (loopback) | Plain HTTP to existing workspace routes + the existing PATCH /api/projects/{id} route (for move_project_to_workspace). No new routes added by this plan. |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-07-11 | Tampering | Agent supplies malformed JSON in tool arguments | low | mitigate | Every handler wraps `json.Unmarshal` errors as `fmt.Errorf("<tool>: invalid arguments: %w", err)` and returns `(nil, err)`. No panic surface. |
| T-07-12 | Denial of Service | `delete_workspace` on the default workspace | low | accept | The v1.9 handler's is_default guard (workspaces.go:209-212) returns 409 "the default workspace can't be deleted" BEFORE any mutation. The bridge surfaces this verbatim. WSMGMT-04 (at least one workspace always exists) is preserved — the agent cannot bypass the guard. |
| T-07-13 | Tampering | `move_project_to_workspace` to a non-existent workspace | low | accept | The v1.9 handler's WorkspaceID branch (projects.go:563-568) validates `SELECT 1 FROM workspaces WHERE id = ?` → 400 "workspace not found" before appending to the PATCH sets. The bridge surfaces this verbatim. No orphaned project. |

</threat_model>

<verification>
- `internal/mcp/workspaces.go`'s `registerWorkspaceTools` calls `s.AddTool` exactly 5 times for list_workspaces, create_workspace, update_workspace, delete_workspace, move_project_to_workspace
- Five `bridge.<verb>Workspace`/`moveProjectToWorkspace` methods exist, each delegating to `b.call`
- `move_project_to_workspace` PATCHes `/api/projects/{id}` with `{workspace_id}`-only body (reuses v1.9 transfer surface)
- `update_workspace` uses `*string` pointer Name field (D-03)
- `internal/mcp/workspaces_test.go` has ~8 test cases; both v1.9 guarded-delete 409 cases covered with substring assertions (Gap 2)
- No error-body assertion matches on JSON key names (Gap 2 / Pitfall 2)
- `go vet ./internal/mcp/...` passes; `go test ./internal/mcp/...` passes; `go build ./...` succeeds
</verification>

<success_criteria>
This plan delivers MCPPROJ-06 (workspace CRUD: list/create/update/delete with v1.9 guarded delete) and MCPPROJ-07 (move_project_to_workspace reusing v1.9 PATCH surface). This satisfies SC3 (the four workspace tools + transfer tool drive the switcher, including the guarded delete keyed off `is_default` + non-empty `COUNT`). Combined with Plans 01/02/03, all 13 Phase 07 requirements are delivered and SC4 (invalid input surfaces actionable MCP errors with no Kamacu state change — every existing validation gate applies unchanged through the bridge) is satisfied by the consistent "send fields as-is, let Kamacu validate" design across all 13 tools.
</success_criteria>

<output>
Create `.planning/phases/07-tasks-projects-workspaces-tools/07-04-SUMMARY.md` when done
</output>

## Artifacts this phase produces

This plan creates the following new symbols (consumed by the plan-review-convergence source-grounding pass; Plans 02/03 do NOT depend on these — they own their own files):

- **MCP tools (new, registered in `registerWorkspaceTools`):**
  - `list_workspaces` — InputSchema `{}` (no args), handler bridges to `GET /api/workspaces`
  - `create_workspace` — InputSchema `{name: string (required)}`, handler bridges to `POST /api/workspaces`
  - `update_workspace` — InputSchema `{workspace_id: integer (required), name?: string}` (D-03 single-field rename), handler bridges to `PATCH /api/workspaces/{id}`
  - `delete_workspace` — InputSchema `{workspace_id: integer (required)}`, handler bridges to `DELETE /api/workspaces/{id}` (v1.9 guarded delete runs unchanged)
  - `move_project_to_workspace` — InputSchema `{project_id: integer (required), workspace_id: integer (required)}`, handler bridges to `PATCH /api/projects/{id}` with `{workspace_id}`-only body (reuses v1.9 transfer surface — NOT a /api/workspaces route)
- **Bridge methods (new on `*bridge` in `internal/mcp/workspaces.go`):**
  - `func (b *bridge) listWorkspaces(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)`
  - `func (b *bridge) createWorkspace(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)`
  - `func (b *bridge) updateWorkspace(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)`
  - `func (b *bridge) deleteWorkspace(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)`
  - `func (b *bridge) moveProjectToWorkspace(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)`
- **Test functions (new in `internal/mcp/workspaces_test.go`):**
  - `TestBridge_ListWorkspaces_Happy_CallsGetPath`
  - `TestBridge_CreateWorkspace_Happy_SendsNameBody`, `TestBridge_CreateWorkspace_DuplicateName_Kamacu409`
  - `TestBridge_UpdateWorkspace_Happy_SendsNameBody`, `TestBridge_UpdateWorkspace_NoName_EmptyBody_Kamacu400`
  - `TestBridge_DeleteWorkspace_Happy_CallsDeletePath`, `TestBridge_DeleteWorkspace_DefaultGuard_Kamacu409`, `TestBridge_DeleteWorkspace_NonEmptyGuard_Kamacu409`
  - `TestBridge_MoveProjectToWorkspace_Happy_PATCHesProjectRoute`, `TestBridge_MoveProjectToWorkspace_TargetMissing_Kamacu400`
