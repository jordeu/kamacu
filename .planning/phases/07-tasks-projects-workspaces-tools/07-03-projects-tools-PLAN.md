---
phase: 07-tasks-projects-workspaces-tools
plan: 03
slug: projects-tools
type: execute
wave: 2
depends_on:
  - 07-01-endpoints-and-mcp-scaffold
files_modified:
  - internal/mcp/projects.go
  - internal/mcp/projects_test.go
autonomous: true
requirements: [MCPPROJ-02, MCPPROJ-03, MCPPROJ-04, MCPPROJ-05]
must_haves:
  truths:
    # MCPPROJ-02: get_project(project_id) — bridges to Gap 1 endpoint from Plan 01
    - "`registerProjectTools` in `internal/mcp/projects.go` registers a tool named `get_project` with InputSchema `{properties: {project_id: {type: integer}}, required: [project_id]}`. The handler extracts `project_id` into `int64` and calls `b.call(ctx, http.MethodGet, fmt.Sprintf(\"/api/projects/%d\", args.ProjectID), nil)` — bridging to the Gap 1 endpoint (`GET /api/projects/{id}`) added in Plan 01. A missing project surfaces Kamacu's 404 body verbatim through the D-05 wrap."
    # MCPPROJ-03: create_project(name, repo_path?, repo?, workspace_id?) — D-04 two-arg fork, zero bridge-side dispatch
    - "`registerProjectTools` registers `create_project` with InputSchema `{properties: {name: {type: string}, repo_path: {type: string, description: 'Absolute path to a local git checkout → folder project'}, repo: {type: string, description: 'GitHub owner/name → managed checkout (gh validates + clones)'}, workspace_id: {type: integer, description: 'Optional target workspace id (default: the Personal workspace)'}}, required: [name]}` (repo_path/repo/workspace_id optional). The handler marshals `{name, repo_path, repo, workspace_id}` as-is to the body and calls `b.call(ctx, http.MethodPost, \"/api/projects\", bytes.NewReader(body))`. The Kamacu handler's existing fork (`string.TrimSpace(req.Repo) != \"\"` → `createByRepo` → gh validate + clone + atomic INSERT; else → folder-path validate + INSERT) does ALL the dispatch — zero bridge-side type detection (D-04)."
    # MCPPROJ-04: update_project(project_id, name?, description?, github_repo?, icon_letters?, icon_color?) — D-03 excludes workspace_id + agent_id
    - "`registerProjectTools` registers `update_project` with InputSchema `{properties: {project_id: {type: integer}, name: {type: string}, description: {type: string}, github_repo: {type: string}, icon_letters: {type: string}, icon_color: {type: string}}, required: [project_id]}` — **the properties map does NOT list `workspace_id` or `agent_id`** (D-03: workspace transfer has its own tool `move_project_to_workspace` in Plan 04; agent reassignment is Out of Scope per MCPMORE-01). The handler uses `*string` pointer fields for name/description/github_repo/icon_letters/icon_color (D-03 partial-PATCH: nil = omitted); builds the body map conditionally with ONLY non-nil fields; calls `b.call(ctx, http.MethodPatch, fmt.Sprintf(\"/api/projects/%d\", args.ProjectID), bytes.NewReader(body))`. Kamacu's existing validation gates (icon palette, 280-cap, gh canonicalization, name-required) apply unchanged through the bridge — the bridge does NOT re-implement validation."
    # MCPPROJ-05: delete_project(project_id) — v1.4 gated delete runs unchanged
    - "`registerProjectTools` registers `delete_project` with InputSchema `{properties: {project_id: {type: integer}}, required: [project_id]}`. The handler extracts `project_id` into `int64` and calls `b.call(ctx, http.MethodDelete, fmt.Sprintf(\"/api/projects/%d\", args.ProjectID), nil)` — bridging to the existing `DELETE /api/projects/{id}` route. The v1.4 two-pass gated removal (folder projects: hard delete, never touch dir; managed projects: all-or-nothing with structured 409 reasons) runs unchanged through the bridge."
    # Pattern fidelity — every handler delegates to bridge.call
    - "Every new project tool handler in `projects.go` follows the same shape Plan 02's task handlers established: declare a typed args struct → `json.Unmarshal(req.Params.Arguments, &args)` (with error wrap) → build path/body → `return b.call(ctx, method, path, body)`. NO handler re-implements the `b.do` → `defer Close` → `io.ReadAll(LimitReader)` sequence inline — all delegate to `bridge.call` (extracted in Plan 01)."
    # Test breadth (D-08)
    - "`internal/mcp/projects_test.go` exists with one happy + one representative error case per tool (~10 total sub-tests): `TestBridge_GetProject_*`, `TestBridge_CreateProject_*`, `TestBridge_UpdateProject_*`, `TestBridge_DeleteProject_*`. Each test stands up an `httptest.Server` capturing method/path/body, constructs a `*bridge` pointed at it, invokes the handler with a `*mcp.CallToolRequest`, and asserts the captured request AND result. The managed-delete 409 test asserts the structured body is passed through verbatim (the D-05 wrap passes `body` as a raw string — Gap 2 confirms the real shape is `{\"error\":\"...\",\"reasons\":[...]}`, keyed on `error` not `message`)."
    # update_project MUST NOT send excluded fields
    - "The `update_project` args struct in `projects.go` has NO `WorkspaceID` and NO `AgentID` field — even if an agent supplies them in the arguments, the SDK ignores unknown properties (MCP spec), and the handler's body map construction only iterates the declared fields. The PATCH body sent to Kamacu therefore never carries `workspace_id` or `agent_id` (D-03 / 07-RESEARCH Pitfall 3)."
  artifacts:
    - internal/mcp/projects.go (EXTENDED — registerProjectTools gains 4 more s.AddTool calls alongside list_projects from Plan 01; NEW bridge methods: getProject, createProject, updateProject, deleteProject)
    - internal/mcp/projects_test.go (NEW — ~10 test cases covering the 4 new tools, happy + error per D-08; list_projects coverage stays in bridge_test.go from Plan 01)
  key_links:
    - "registerProjectTools(s, b) → 5 s.AddTool calls total (list_projects from Plan 01 + get_project, create_project, update_project, delete_project from this plan)"
    - "bridge.getProject(ctx, req) → b.call(GET, /api/projects/{id}, nil) — Gap 1 endpoint from Plan 01"
    - "bridge.createProject(ctx, req) → b.call(POST, /api/projects, {name, repo_path, repo, workspace_id} body as-is — D-04)"
    - "bridge.updateProject(ctx, req) → b.call(PATCH, /api/projects/{id}, {name?, description?, github_repo?, icon_letters?, icon_color?} partial body — D-03, NO workspace_id/agent_id)"
    - "bridge.deleteProject(ctx, req) → b.call(DELETE, /api/projects/{id}, nil) — v1.4 gated delete runs unchanged"
  prohibitions:
    - statement: "NO bridge-side validation of name/repo_path/repo/icon_letters/icon_color — Kamacu's handlers are the validator (D-03/D-04). The bridge sends fields as-is; an invalid value surfaces Kamacu's 400 verbatim through the D-05 wrap."
      status: resolved
      verification: "internal/mcp/projects.go contains no call to validateIconLetters/validateIconColor/validateRepoPath/github.ValidateRepo; no `if args.Name == \"\"` style guard; fields pass straight into the body map"
    - statement: "NO `workspace_id` or `agent_id` in the update_project InputSchema properties — D-03 excludes them. The args struct has NO WorkspaceID/AgentID fields. Even if an agent supplies them, the SDK ignores unknown properties AND the handler's body map only includes the declared fields."
      status: resolved
      verification: "`grep -A20 'Name: \"update_project\"' internal/mcp/projects.go | grep properties` does NOT contain `workspace_id` or `agent_id`; the updateProject args struct declaration has no WorkspaceID/AgentID field"
    - statement: "NO bridge-side dispatch between folder and managed create paths — D-04 maps the two-arg shape to the underlying fork 1:1. The handler sends `{name, repo_path, repo, workspace_id}` as-is; Kamacu's `string.TrimSpace(req.Repo) != \"\"` check does the dispatch."
      status: resolved
      verification: "internal/mcp/projects.go's createProject handler contains no `if args.Repo != \"\"` or `if args.RepoPath != \"\"` branch — the body map is built unconditionally with all four fields and passed to b.call(POST, /api/projects, ...)"
    - statement: "DO NOT touch internal/api/, internal/mcp/server.go, internal/mcp/bridge.go, internal/mcp/tasks.go, or internal/mcp/workspaces.go — this plan owns ONLY internal/mcp/projects.go and internal/mcp/projects_test.go. Plan 01 already shipped the Gap 1 endpoint, the bridge.call helper, the list_projects move, and the registerProjectTools shell."
      status: resolved
      verification: "`git diff --name-only` for this plan shows exactly two files: internal/mcp/projects.go and internal/mcp/projects_test.go"
    - statement: "DO NOT parse Kamacu error JSON in the bridge — D-05 passes the body as a raw string. Test assertions MUST use substrings of Kamacu message text (e.g. `\"project not found\"`, `\"name is required\"`, `\"the project can't be deleted yet\"`), NOT JSON key names (07-RESEARCH Gap 2 / Pitfall 2: real shape is `{\"error\":\"...\"}`, structured delete-409 is `{\"error\":\"...\",\"reasons\":[...]}`)."
      status: resolved
      verification: "internal/mcp/projects_test.go contains no assertion matching on the literal string `\"message\"` or `\"status\"` as a JSON key; the managed-delete 409 test asserts on `\"can't be deleted\"` or `\"reasons\"` substrings"
    - statement: "DO NOT use the typed `mcp.AddTool[In, Out any]` generic — keep using the low-level `s.AddTool(*mcp.Tool, handler)` with `map[string]any` InputSchema (Phase 06 lock, 06-02-SUMMARY)."
      status: resolved
      verification: "`grep -rn 'mcp.AddTool\\[' internal/mcp/projects.go` returns zero results"
---

# Plan 03: Four new project tools (get_project, create_project, update_project, delete_project)

<objective>
Extend `registerProjectTools` in `internal/mcp/projects.go` (which Plan 01 populated with the moved+updated `list_projects`) with the four remaining MCPPROJ tools. Every tool is a thin HTTP-bridge handler delegating to `bridge.call` — the same shape Phase 06 proved with `list_projects` and Plan 02 applies to tasks. This plan delivers MCPPROJ-02 through MCPPROJ-05 and (combined with Plan 01's `list_projects(workspace_id?)`) satisfies SC2 (the five project tools drive the sidebar, including v1.4 managed-checkout atomic create + all-or-nothing gated delete, with folder-project delete byte-for-byte unchanged).

Purpose: continue proving the bridge pattern scales. Four tools, each ~15-25 lines of handler + one `s.AddTool` call, all delegating to the shared helper. The two-arg `create_project` fork (D-04) and the partial-PATCH `update_project` with excluded fields (D-03) are the only shape nuances — both resolved by sending fields as-is and letting Kamacu's existing handlers validate/dispatch.

Output: `internal/mcp/projects.go` extended with four `bridge.<verb>Project` methods + four new `s.AddTool` calls; `internal/mcp/projects_test.go` with ~10 happy/error test cases per D-08.
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
@internal/api/projects.go
@internal/api/routes.go
@internal/mcp/bridge.go
@internal/mcp/projects.go
@internal/mcp/bridge_test.go
</context>

<tasks>

<task type="auto">
  <name>Task 1: Implement the four new project tool handlers in registerProjectTools (get_project, create_project, update_project, delete_project)</name>
  <files>internal/mcp/projects.go</files>
  <read_first>
    - `.planning/phases/07-tasks-projects-workspaces-tools/07-RESEARCH.md` § "Tool-to-Endpoint Mapping → Project tools" (the table with MCP args, bridge HTTP call, Kamacu handler line reference, and per-tool notes for all five project tools including the inherited list_projects), § "create_project two-arg fork (D-04)" (the Kamacu handler's `string.TrimSpace(req.Repo) != ""` dispatch — the bridge does ZERO dispatch logic), § "update_project validation gates (D-03)" (the list of Kamacu-side gates: name required, description ≤280, github_repo gh-canonicalized, icon_letters validateIconLetters, icon_color validateIconColor palette member, empty body → 400 "nothing to update"), § "Detailed Findings → Gap 2" (the managed-delete 409 structured body shape `{"error":"the project can't't be deleted yet","reasons":[...]}`), § "Common Pitfalls → Pitfall 3" (update_project MUST NOT send workspace_id or agent_id — exclude from properties AND args struct)
    - `.planning/phases/07-tasks-projects-workspaces-tools/07-CONTEXT.md` decisions D-03 (`update_project` exposes `{project_id, name?, description?, github_repo?, icon_letters?, icon_color?}` — **excludes `workspace_id` and `agent_id`**; flat optional top-level args, no nested `updates` object), D-04 (`create_project` exposes two distinct optional args — `repo_path?` (folder) and `repo?` (owner/name → managed checkout); the handler's existing fork dispatches; zero bridge-side type detection), D-05 (error wrap pattern — body as raw string)
    - `internal/api/projects.go` (relevant ranges) — lines 127-148 (`list` — extended by Plan 01 for `?workspace_id=`; this plan does NOT touch it), lines 163-239 (`create` — the folder-vs-repo fork at line 203: `if strings.TrimSpace(req.Repo) != "" { h.createByRepo(...); return }`; the folder path validateRepoPath + dedup + INSERT), lines 455-604 (`update` — the partial-PATCH with `*string`/`*int64` pointer fields; the validation gates: name empty → 400 "name is required", description >280 → 400 "Description is too long.", github_repo empty → unlink NULL, non-empty → gh validate + canonicalize, icon_letters validateIconLetters, icon_color validateIconColor, workspace_id validated → 400 "workspace not found", agent_id validated → 400 "agent not found", all-nil → 400 "nothing to update"), lines 671-708 (`delete` — folder path: `DELETE FROM projects WHERE id = ?` never touch dir; managed path: `deleteManaged` two-pass gated removal), lines 808-815 (the managed 409 structured body `{"error":"the project can't be deleted yet","reasons":[...]}`)
    - `internal/api/routes.go` lines 32-37 — the project routes this plan bridges to (GET /api/projects, POST /api/projects, PATCH /api/projects/{id}, GET /api/projects/{id} from Plan 01 Gap 1, DELETE /api/projects/{id})
    - `internal/mcp/projects.go` (post-Plan-01) — the starting state: `registerProjectTools` with the single list_projects `s.AddTool` call, and `bridge.listProjects(ctx, req)` with the workspace_id? arg. This plan ADDS four more `s.AddTool` calls and four more bridge methods alongside the existing list_projects.
    - `internal/mcp/bridge.go` (post-Plan-01) — the `bridge.call(ctx, method, path, body)` helper every handler delegates to.
    - `internal/mcp/bridge_test.go` (post-Plan-01) — the `TestBridge_ListProjects_*` test pattern this plan's `projects_test.go` mirrors.
  </read_first>
  <action>
    Extend `registerProjectTools` in `internal/mcp/projects.go` with four new `s.AddTool` calls and add the four corresponding `*bridge` methods. Each method follows the canonical shape Plan 02 established: typed args struct → `json.Unmarshal(req.Params.Arguments, &args)` (wrap unmarshal errors as `fmt.Errorf("<tool_name>: invalid arguments: %w", err)`) → build path/body → `return b.call(ctx, method, path, body)`. Imports needed (add to the existing import block): `bytes`, `encoding/json`, `fmt`, `net/http` (some already present from Plan 01's listProjects).

    1. **get_project (MCPPROJ-02).** InputSchema: `{type: object, properties: {project_id: {type: integer, description: "The project id."}}, required: [project_id]}`. Handler `bridge.getProject(ctx, req)`:
       - `var args struct { ProjectID int64 \`json:"project_id"\` }`
       - unmarshal with error wrap
       - `return b.call(ctx, http.MethodGet, fmt.Sprintf("/api/projects/%d", args.ProjectID), nil)` — bridges to the Gap 1 endpoint from Plan 01.

    2. **create_project (MCPPROJ-03, D-04 two-arg fork).** InputSchema: `{type: object, properties: {name: {type: string, description: "Project name."}, repo_path: {type: string, description: "Absolute path to a local git checkout. Creates a folder project (Kamacu never touches the dir on delete)."}, repo: {type: string, description: "GitHub owner/name (e.g. 'owner/repo'). Creates a managed checkout — Kamacu gh-validates, clones into ~/.kamacu/repos/<owner>/<name>, and owns the dir (gated remove on delete)."}, workspace_id: {type: integer, description: "Optional target workspace id. Omitted → the default Personal workspace."}}, required: [name]}` (repo_path/repo/workspace_id optional — the handler's fork decides based on whether `repo` is non-empty). Handler `bridge.createProject(ctx, req)`:
       - `var args struct { Name string \`json:"name"\`; RepoPath string \`json:"repo_path"\`; Repo string \`json:"repo"\`; WorkspaceID *int64 \`json:"workspace_id"\` }` — WorkspaceID is a pointer so omitted = nil (Kamacu falls back to default); the other three are plain strings (empty = absent).
       - unmarshal with error wrap
       - Build body map with ALL four fields (D-04: send as-is, let Kamacu's fork dispatch): `body := map[string]any{"name": args.Name, "repo_path": args.RepoPath, "repo": args.Repo}; if args.WorkspaceID != nil { body["workspace_id"] = *args.WorkspaceID }` — include workspace_id only when supplied (matches Kamacu's `*int64` decode where omitted = default fallback).
       - `jsonBody, _ := json.Marshal(body)`
       - `return b.call(ctx, http.MethodPost, "/api/projects", bytes.NewReader(jsonBody))` — the Kamacu handler's `if strings.TrimSpace(req.Repo) != "" { h.createByRepo(...) }` does the dispatch.

    3. **update_project (MCPPROJ-04, D-03 partial-PATCH with EXCLUDED fields).** InputSchema: `{type: object, properties: {project_id: {type: integer, description: "..."}, name: {type: string, description: "Optional new name."}, description: {type: string, description: "Optional new description (≤280 chars)."}, github_repo: {type: string, description: "Optional new GitHub repo link (owner/name). Empty string unlinks."}, icon_letters: {type: string, description: "Optional new icon letters (≤2 upper alnum)."}, icon_color: {type: string, description: "Optional new icon color (curated palette member)."}}, required: [project_id]}` — **NO `workspace_id`, NO `agent_id` in properties** (D-03 / 07-RESEARCH Pitfall 3). Handler `bridge.updateProject(ctx, req)`:
       - `var args struct { ProjectID int64 \`json:"project_id"\`; Name *string \`json:"name"\`; Description *string \`json:"description"\`; GithubRepo *string \`json:"github_repo"\`; IconLetters *string \`json:"icon_letters"\`; IconColor *string \`json:"icon_color"\` }` — pointer fields for all five optional args (D-03: nil = omitted, non-nil = supplied, `""` = explicit empty). **NO WorkspaceID, NO AgentID fields.**
       - unmarshal with error wrap
       - Build body map conditionally — ONLY non-nil fields: `body := map[string]any{}; if args.Name != nil { body["name"] = *args.Name }; if args.Description != nil { body["description"] = *args.Description }; if args.GithubRepo != nil { body["github_repo"] = *args.GithubRepo }; if args.IconLetters != nil { body["icon_letters"] = *args.IconLetters }; if args.IconColor != nil { body["icon_color"] = *args.IconColor }` — note: `github_repo` empty string IS a valid value (unlinks → NULL), so `*args.GithubRepo` (including `""`) is added when non-nil.
       - `jsonBody, _ := json.Marshal(body)`
       - `return b.call(ctx, http.MethodPatch, fmt.Sprintf("/api/projects/%d", args.ProjectID), bytes.NewReader(jsonBody))` — Kamacu's validation gates (name required, 280-cap, gh canonicalization, icon palette) apply unchanged.

    4. **delete_project (MCPPROJ-05).** InputSchema: `{type: object, properties: {project_id: {type: integer, description: "The project id. Folder projects: hard delete, directory never touched. Managed projects: two-pass gated removal — 409 with structured reasons if dirty/unpushed/stash/sessions."}}, required: [project_id]}`. Handler `bridge.deleteProject(ctx, req)`:
       - `var args struct { ProjectID int64 \`json:"project_id"\` }`
       - unmarshal with error wrap
       - `return b.call(ctx, http.MethodDelete, fmt.Sprintf("/api/projects/%d", args.ProjectID), nil)` — the existing gated delete (folder: hard delete; managed: two-pass with structured 409) runs unchanged through the bridge.

    5. **Register all four** in `registerProjectTools` via `s.AddTool(&mcp.Tool{Name: "...", Description: "...", InputSchema: map[string]any{...}}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) { return b.<verb>Project(ctx, req) })`, alongside the existing list_projects `s.AddTool` call from Plan 01. Descriptions should be agent-readable and lead with what the tool does.
  </action>
  <verify>
    <automated>
      set -e
      # All four new tools registered (plus list_projects from Plan 01 = 5 total)
      for tool in get_project create_project update_project delete_project; do
        grep -q "Name: \"$tool\"" internal/mcp/projects.go || { echo "missing $tool"; exit 1; }
      done
      test "$(grep -c 'Name: "' internal/mcp/projects.go)" -eq 5
      # All four bridge methods exist
      for method in getProject createProject updateProject deleteProject; do
        grep -q "func (b \*bridge) $method(" internal/mcp/projects.go || { echo "missing $method"; exit 1; }
      done
      # list_projects (workspace_id?) from Plan 01 is still present and unchanged
      grep -q 'func (b \*bridge) listProjects(' internal/mcp/projects.go
      grep -q '"workspace_id"' internal/mcp/projects.go
      # update_project does NOT expose workspace_id or agent_id (D-03 / Pitfall 3)
      ! grep -A30 'Name: "update_project"' internal/mcp/projects.go | grep -E '"workspace_id"|"agent_id"'
      # The updateProject args struct has NO WorkspaceID/AgentID field
      ! grep -B2 -A10 'func (b \*bridge) updateProject' internal/mcp/projects.go | grep -E 'WorkspaceID|AgentID'
      # create_project sends all four fields as-is (D-04 — no bridge-side dispatch)
      grep -A15 'func (b \*bridge) createProject' internal/mcp/projects.go | grep -q '"repo_path"'
      grep -A15 'func (b \*bridge) createProject' internal/mcp/projects.go | grep -q '"repo"'
      # Every handler delegates to b.call (no inline b.do + io.ReadAll)
      ! grep -q 'io.ReadAll' internal/mcp/projects.go
      ! grep -q 'b\.do(' internal/mcp/projects.go
      # No typed AddTool[In,Out] generic
      ! grep -q 'mcp.AddTool\[' internal/mcp/projects.go
      # No bridge-side validation
      ! grep -q 'validateIconLetters\|validateIconColor\|validateRepoPath\|github.ValidateRepo' internal/mcp/projects.go
      # go vet + build
      go vet ./internal/mcp/...
      go build ./...
    </automated>
  </verify>
  <acceptance_criteria>
    - `grep -c 'Name: "' internal/mcp/projects.go` returns 5 (list_projects from Plan 01 + the 4 new tools)
    - For each of get_project/create_project/update_project/delete_project: `grep -q "Name: \"<tool>\"" internal/mcp/projects.go` succeeds
    - For each of getProject/createProject/updateProject/deleteProject: `grep -q "func (b \*bridge) <method>(" internal/mcp/projects.go` succeeds
    - `grep -A30 'Name: "update_project"' internal/mcp/projects.go | grep -E '"workspace_id"|"agent_id"'` returns NOTHING (D-03 / Pitfall 3 — excluded fields not in properties)
    - `grep -B2 -A10 'func (b \*bridge) updateProject' internal/mcp/projects.go | grep -E 'WorkspaceID|AgentID'` returns NOTHING (args struct has no such fields)
    - `grep -A15 'func (b \*bridge) createProject' internal/mcp/projects.go | grep -q '"repo_path"'` succeeds (D-04 — body includes repo_path)
    - `grep -A15 'func (b \*bridge) createProject' internal/mcp/projects.go | grep -q '"repo"'` succeeds (D-04 — body includes repo)
    - `! grep -q 'b\.do(' internal/mcp/projects.go` succeeds (every handler delegates to b.call)
    - `! grep -q 'io\.ReadAll' internal/mcp/projects.go` succeeds (no inline response reading)
    - `! grep -q 'mcp\.AddTool\[' internal/mcp/projects.go` succeeds (no typed generic)
    - `! grep -q 'validateIconLetters\|validateIconColor\|validateRepoPath\|github\.ValidateRepo' internal/mcp/projects.go` succeeds (no bridge-side validation — D-03/D-04)
    - `go vet ./internal/mcp/...` passes
    - `go build ./...` passes
  </acceptance_criteria>
  <done>
    - registerProjectTools registers 5 tools total (list_projects from Plan 01 + get_project, create_project, update_project, delete_project)
    - Four bridge methods (getProject, createProject, updateProject, deleteProject) each delegate to b.call
    - update_project InputSchema excludes workspace_id and agent_id; args struct has no such fields (D-03 / Pitfall 3)
    - create_project sends all four fields as-is to POST /api/projects (D-04 — no bridge-side dispatch)
    - No handler re-implements b.do + io.ReadAll; no bridge-side validation
    - go vet + go build pass
  </done>
</task>

<task type="auto">
  <name>Task 2: Add projects_test.go with ~10 happy/error test cases per D-08 (including managed-delete 409 structured body)</name>
  <files>internal/mcp/projects_test.go</files>
  <read_first>
    - `.planning/phases/07-tasks-projects-workspaces-tools/07-RESEARCH.md` § "Detailed Findings → Gap 2" (the actual Kamacu error shapes: standard `{"error":"<message>"}`, managed-delete 409 `{"error":"the project can't be deleted yet","reasons":[{"kind":"uncommitted","target":"task #N"}]}` — keyed on `error`, NOT `message`), § "Common Pitfalls → Pitfall 2" (test assertions on Kamacu message substrings, not JSON key names), § "Common Pitfalls → Pitfall 3" (the update_project test should verify the body does NOT contain workspace_id/agent_id)
    - `.planning/phases/07-tasks-projects-workspaces-tools/07-CONTEXT.md` D-08 (one representative test per tool per resource — ~10 cases for projects: 4 new tools × ~2-3 cases each)
    - `internal/mcp/bridge_test.go` (post-Plan-01) — the `httptest.NewServer` + `&bridge{base: srv.URL, ...}` + `*mcp.CallToolRequest` pattern this plan mirrors.
    - `internal/mcp/projects.go` (post-Task-1 of this plan) — the production code under test; the test invokes each `bridge.<verb>Project` method directly.
    - `internal/api/projects.go` — the Kamacu handler behavior the test server simulates (e.g. `get` returns project JSON or 404 `{"error":"project not found"}`; `create` returns 201 or 400/409; `update` returns 200 or 400; `delete` returns 204 or 409 with structured reasons for managed projects).
  </read_first>
  <action>
    Create `internal/mcp/projects_test.go` (package `mcp`) with one happy + one representative error case per tool (~10 total sub-tests). Follow the `TestBridge_<Verb><Resource>_<Scenario>` naming convention. Imports: `context`, `encoding/json`, `net/http`, `net/http/httptest`, `strings`, `testing`, `time`, `github.com/modelcontextprotocol/go-sdk/mcp`.

    Each test stands up an `httptest.Server` capturing `r.Method`, `r.URL.Path`, and request body, returns a canned response, and asserts the captured request + result. Required cases:

    1. `TestBridge_GetProject_Happy_CallsCorrectPath` — Arguments `{"project_id":3}`; assert `r.URL.Path == "/api/projects/3"` AND method GET AND TextContent passthrough of the canned project JSON.
    2. `TestBridge_GetProject_NotFound404_ReturnsError` — server returns 404 with `{"error":"project not found"}`; assert error contains `"HTTP 404"` (substring, not JSON key).
    3. `TestBridge_CreateProject_FolderPath_SendsAllFieldsAsIs` — Arguments `{"name":"foo","repo_path":"/abs/path"}` (repo empty); assert method POST AND `r.URL.Path == "/api/projects"` AND the body decodes to a map containing `name`, `repo_path`, `repo` keys (D-04: all four sent, even when some are empty strings).
    4. `TestBridge_CreateProject_ManagedRepo_SendsAllFieldsAsIs` — Arguments `{"name":"foo","repo":"owner/name"}` (repo_path empty); assert body contains `repo: "owner/name"` (the field Kamacu's fork dispatches on).
    5. `TestBridge_CreateProject_WorkspaceIDIncluded_WhenSupplied` — Arguments `{"name":"foo","repo_path":"/x","workspace_id":2}`; assert body contains `workspace_id: 2`.
    6. `TestBridge_CreateProject_Kamacu400_InvalidPath_ReturnsError` — server returns 400 with `{"error":"path must be absolute"}`; assert error contains `"HTTP 400"`.
    7. `TestBridge_UpdateProject_PartialPATCH_SendsOnlySuppliedFields` — Arguments `{"project_id":5,"name":"new"}` (all other fields OMITTED); assert method PATCH AND body decodes to `{"name":"new"}` with NO other keys.
    8. `TestBridge_UpdateProject_GithubRepoEmptyString_Unlinks` — Arguments `{"project_id":5,"github_repo":""}`; assert body contains `github_repo: ""` (explicit empty string IS sent — unlinks → NULL, distinct from omitted).
    9. `TestBridge_UpdateProject_BodyExcludes_WorkspaceID_And_AgentID` — Arguments `{"project_id":5,"name":"new","workspace_id":2,"agent_id":3}` (agent supplies excluded fields); assert the captured body does NOT contain `workspace_id` or `agent_id` keys (D-03 / Pitfall 3 — the args struct doesn't declare them, so they're dropped).
    10. `TestBridge_UpdateProject_Kamacu400_DescriptionTooLong_ReturnsError` — server returns 400 with `{"error":"Description is too long."}`; assert error contains `"HTTP 400"`.
    11. `TestBridge_DeleteProject_Happy_CallsDeletePath` — Arguments `{"project_id":7}`; assert `r.URL.Path == "/api/projects/7"` AND method DELETE.
    12. `TestBridge_DeleteProject_Managed409_PassesThroughStructuredBody` — server returns 409 with `{"error":"the project can't be deleted yet","reasons":[{"kind":"uncommitted","target":"task #3"}]}`; assert error contains `"HTTP 409"` AND the error string contains `"can't be deleted"` (substring of the Kamacu message — NOT a JSON key assertion per Gap 2). Optionally also assert the error string contains `"reasons"` (the structured body survives in the raw string).

    Use `t.Cleanup(srv.Close)` for every test server. For body map assertions, unmarshal the captured body into `map[string]any` and check key presence/absence. For the D-03 exclusion test, assert `_, ok := body["workspace_id"]; !ok` AND `_, ok := body["agent_id"]; !ok`.
  </action>
  <verify>
    <automated>
      set -e
      test -f internal/mcp/projects_test.go
      # All four new tools have at least one happy + one error test
      for tool in GetProject CreateProject UpdateProject DeleteProject; do
        grep -q "TestBridge_$tool" internal/mcp/projects_test.go || { echo "missing $tool tests"; exit 1; }
      done
      # D-03 coverage: update_project body excludes workspace_id/agent_id
      grep -B2 -A15 'TestBridge_UpdateProject_BodyExcludes' internal/mcp/projects_test.go | grep -q 'workspace_id'
      grep -B2 -A15 'TestBridge_UpdateProject_BodyExcludes' internal/mcp/projects_test.go | grep -q 'agent_id'
      # D-04 coverage: create_project sends all fields as-is
      grep -B2 -A15 'TestBridge_CreateProject_FolderPath\|TestBridge_CreateProject_ManagedRepo' internal/mcp/projects_test.go | grep -q 'repo_path'
      # Gap 2 coverage: managed-delete 409 structured body test exists
      grep -q 'TestBridge_DeleteProject_Managed409' internal/mcp/projects_test.go
      # No JSON key name assertions on error bodies (Gap 2 / Pitfall 2)
      ! grep -q '"message"' internal/mcp/projects_test.go
      # Uses httptest pattern
      grep -q 'httptest.NewServer' internal/mcp/projects_test.go
      # go vet + tests pass
      go vet ./internal/mcp/...
      go test ./internal/mcp/... -v
    </automated>
  </verify>
  <acceptance_criteria>
    - `test -f internal/mcp/projects_test.go` succeeds
    - `grep -c 'func TestBridge_' internal/mcp/projects_test.go` returns at least 10 (one happy + one error per the 4 new tools, per D-08)
    - For each of GetProject/CreateProject/UpdateProject/DeleteProject: `grep -q "TestBridge_<Tool>_" internal/mcp/projects_test.go` succeeds
    - `grep -B2 -A15 'TestBridge_UpdateProject_BodyExcludes' internal/mcp/projects_test.go | grep -q 'workspace_id'` succeeds (D-03 / Pitfall 3 coverage — exclusion asserted)
    - `grep -B2 -A15 'TestBridge_UpdateProject_BodyExcludes' internal/mcp/projects_test.go | grep -q 'agent_id'` succeeds (D-03 / Pitfall 3 coverage)
    - `grep -q 'TestBridge_DeleteProject_Managed409' internal/mcp/projects_test.go` succeeds (Gap 2 coverage — structured 409 body)
    - `! grep -q '"message"' internal/mcp/projects_test.go` succeeds (Gap 2 / Pitfall 2 — no JSON key name assertions)
    - `go test ./internal/mcp/... -v` passes (all project tests GREEN, plus Plan 01/02 tests still GREEN)
  </acceptance_criteria>
  <done>
    - internal/mcp/projects_test.go exists with ~10 test cases covering the 4 new tools (happy + error each)
    - update_project BodyExcludes test asserts workspace_id and agent_id are NOT in the body (D-03 / Pitfall 3)
    - create_project tests assert all fields are sent as-is (D-04)
    - delete_project Managed409 test asserts the structured body survives in the wrapped error (Gap 2)
    - No error-body assertion matches on JSON key names (Gap 2 / Pitfall 2)
    - `go test ./internal/mcp/... -v` passes
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| agent CLI → MCP subcommand stdin | Unchanged. The four new tools accept `*mcp.CallToolRequest`; arguments unmarshaled into typed structs. |
| MCP subcommand → Kamacu HTTP API (loopback) | Plain HTTP to existing project routes + the Gap 1 GET /api/projects/{id} from Plan 01. |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-07-08 | Tampering | Agent supplies excluded `workspace_id`/`agent_id` via update_project | low | mitigate | D-03 / 07-RESEARCH Pitfall 3: the updateProject args struct has NO WorkspaceID/AgentID fields, so `json.Unmarshal` silently drops them. The body map construction only iterates declared fields. Even if an agent supplies them, they never reach Kamacu's PATCH. Test `TestBridge_UpdateProject_BodyExcludes_WorkspaceID_And_AgentID` verifies this. |
| T-07-09 | Elevation of Privilege | `delete_project` on a managed project triggers gated removal | medium | accept | The underlying DELETE /api/projects/{id} handler runs the v1.4 two-pass gated removal unchanged — folder projects hard-delete (never touch dir), managed projects 409 with structured reasons if dirty/unpushed/stash/sessions. The bridge surfaces the 409 verbatim through the D-05 wrap; the agent sees the reasons and can act (commit/push/stop sessions). This is the SAME gate the browser-attached user hits — no privilege escalation. |
| T-07-10 | Information Disclosure | `create_project` with `repo` triggers a `gh repo clone` (network egress) | low | accept | The underlying createByRepo handler shells out to `gh repo clone` into ~/.kamacu/repos/. This is existing v1.4 behavior the browser already triggers; the MCP tool inherits it unchanged. Loopback-only deployment means the agent is already trusted (spawned by Kamacu inside a task PTY). |

</threat_model>

<verification>
- `internal/mcp/projects.go`'s `registerProjectTools` calls `s.AddTool` exactly 5 times (list_projects from Plan 01 + get_project, create_project, update_project, delete_project)
- Four new `bridge.<verb>Project` methods exist, each delegating to `b.call`
- `update_project` InputSchema excludes `workspace_id` and `agent_id`; args struct has no such fields (D-03 / Pitfall 3)
- `create_project` sends all four fields (name, repo_path, repo, workspace_id) as-is — no bridge-side dispatch (D-04)
- `internal/mcp/projects_test.go` has ~10 test cases; update_project BodyExcludes test asserts field exclusion; delete_project Managed409 test asserts structured body passthrough
- No error-body assertion matches on JSON key names (Gap 2 / Pitfall 2)
- `go vet ./internal/mcp/...` passes; `go test ./internal/mcp/...` passes; `go build ./...` succeeds
</verification>

<success_criteria>
This plan delivers MCPPROJ-02 (get_project), MCPPROJ-03 (create_project with D-04 two-arg fork), MCPPROJ-04 (update_project partial-PATCH with D-03 excluded fields), MCPPROJ-05 (delete_project with v1.4 gated delete). Combined with Plan 01's `list_projects(workspace_id?)` delivering MCPPROJ-01, this completes SC2 (the five project tools drive the sidebar — list/get/create/update/delete, including v1.4 managed-checkout atomic create + all-or-nothing gated delete, folder-project delete byte-for-byte unchanged).
</success_criteria>

<output>
Create `.planning/phases/07-tasks-projects-workspaces-tools/07-03-SUMMARY.md` when done
</output>

## Artifacts this phase produces

This plan creates the following new symbols (consumed by the plan-review-convergence source-grounding pass; Plans 02/04 do NOT depend on these — they own their own files):

- **MCP tools (new, registered in `registerProjectTools` alongside list_projects from Plan 01):**
  - `get_project` — InputSchema `{project_id: integer (required)}`, handler bridges to `GET /api/projects/{id}` (Gap 1 endpoint from Plan 01)
  - `create_project` — InputSchema `{name: string (required), repo_path?: string, repo?: string, workspace_id?: integer}` (D-04 two-arg fork), handler bridges to `POST /api/projects` with all fields as-is
  - `update_project` — InputSchema `{project_id: integer (required), name?: string, description?: string, github_repo?: string, icon_letters?: string, icon_color?: string}` (D-03 — **excludes workspace_id and agent_id**), handler bridges to `PATCH /api/projects/{id}` with partial body
  - `delete_project` — InputSchema `{project_id: integer (required)}`, handler bridges to `DELETE /api/projects/{id}` (v1.4 gated delete runs unchanged)
- **Bridge methods (new on `*bridge` in `internal/mcp/projects.go`):**
  - `func (b *bridge) getProject(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)`
  - `func (b *bridge) createProject(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)`
  - `func (b *bridge) updateProject(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)`
  - `func (b *bridge) deleteProject(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)`
- **Test functions (new in `internal/mcp/projects_test.go`):**
  - `TestBridge_GetProject_Happy_CallsCorrectPath`, `TestBridge_GetProject_NotFound404_ReturnsError`
  - `TestBridge_CreateProject_FolderPath_SendsAllFieldsAsIs`, `TestBridge_CreateProject_ManagedRepo_SendsAllFieldsAsIs`, `TestBridge_CreateProject_WorkspaceIDIncluded_WhenSupplied`, `TestBridge_CreateProject_Kamacu400_InvalidPath_ReturnsError`
  - `TestBridge_UpdateProject_PartialPATCH_SendsOnlySuppliedFields`, `TestBridge_UpdateProject_GithubRepoEmptyString_Unlinks`, `TestBridge_UpdateProject_BodyExcludes_WorkspaceID_And_AgentID`, `TestBridge_UpdateProject_Kamacu400_DescriptionTooLong_ReturnsError`
  - `TestBridge_DeleteProject_Happy_CallsDeletePath`, `TestBridge_DeleteProject_Managed409_PassesThroughStructuredBody`
