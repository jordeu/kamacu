---
phase: 07-tasks-projects-workspaces-tools
plan: 02
slug: tasks-tools
type: execute
wave: 2
depends_on:
  - 07-01-endpoints-and-mcp-scaffold
files_modified:
  - internal/mcp/tasks.go
  - internal/mcp/tasks_test.go
autonomous: true
requirements: [MCPTASK-01, MCPTASK-02, MCPTASK-03, MCPTASK-04, MCPTASK-05, MCPTASK-06]
must_haves:
  truths:
    # MCPTASK-01: list_tasks(project_id?)
    - "`registerTaskTools` in `internal/mcp/tasks.go` registers a tool named `list_tasks` with InputSchema `{type: object, properties: {project_id: {type: integer, description: ...}}}` (project_id optional — NO `required` array). The handler extracts `project_id` into `*int64`; when non-nil the bridge calls `GET /api/projects/{id}/tasks` (existing route, untouched); when nil the bridge calls `GET /api/tasks` (the D-01 endpoint added in Plan 01). Both paths delegate to `b.call(ctx, http.MethodGet, path, nil)`."
    # MCPTASK-02: get_task(task_id)
    - "`registerTaskTools` registers `get_task` with InputSchema `{properties: {task_id: {type: integer}}, required: [task_id]}`. The handler extracts `task_id` into `int64` and calls `b.call(ctx, http.MethodGet, fmt.Sprintf(\"/api/tasks/%d\", id), nil)` — bridging to the existing `GET /api/tasks/{id}` route. A missing/invalid task_id surfaces Kamacu's 404 body verbatim through the D-05 wrap."
    # MCPTASK-03: create_task(project_id, title, description?) — agent NOT auto-started (existing behavior)
    - "`registerTaskTools` registers `create_task` with InputSchema `{properties: {project_id: {integer}, title: {string}, description: {string}}, required: [project_id, title]}`. The handler marshals `{\"title\": args.Title, \"description\": args.Description}` as the body and calls `b.call(ctx, http.MethodPost, fmt.Sprintf(\"/api/projects/%d/tasks\", args.ProjectID), bytes.NewReader(body))` — bridging to the existing `POST /api/projects/{id}/tasks` route. The agent is NOT auto-started (the underlying handler never starts one — auto-start remains Out of Scope per PROJECT.md / REQUIREMENTS.md)."
    # MCPTASK-04: update_task(task_id, title?, description?) — partial-PATCH, D-03 flat optional args
    - "`registerTaskTools` registers `update_task` with InputSchema `{properties: {task_id: {integer}, title: {string}, description: {string}}, required: [task_id]}` (title/description optional — NO `required` beyond task_id). The handler extracts `task_id` into `int64` and `title`/`description` into `*string` pointer fields (D-03: nil = omitted, non-nil = supplied). It builds the PATCH body conditionally — ONLY non-nil fields are added to the body map (matches Kamacu's partial-PATCH semantics where omitted keys are left untouched). Calls `b.call(ctx, http.MethodPatch, fmt.Sprintf(\"/api/tasks/%d\", args.TaskID), bytes.NewReader(body))` — bridging to the existing `PATCH /api/tasks/{id}` route."
    # MCPTASK-05: move_task(task_id, status) — D-06 after_id: null (top of column)
    - "`registerTaskTools` registers `move_task` with InputSchema `{properties: {task_id: {integer}, status: {string, enum: [todo, in_progress, in_review, done]}}, required: [task_id, status]}`. The handler marshals `{\"status\": args.Status, \"after_id\": null}` (D-06: after_id is ALWAYS null — the MCP tool exposes NO after_id arg; the bridge hardcodes it) and calls `b.call(ctx, http.MethodPost, fmt.Sprintf(\"/api/tasks/%d/move\", args.TaskID), bytes.NewReader(body))` — bridging to the existing `POST /api/tasks/{id}/move` route. The handler's existing `MIN(position) - 1.0` math for null after_id (top of column) is unchanged."
    # MCPTASK-06: delete_task(task_id) — gated cleanup runs
    - "`registerTaskTools` registers `delete_task` with InputSchema `{properties: {task_id: {integer}}, required: [task_id]}`. The handler extracts `task_id` into `int64` and calls `b.call(ctx, http.MethodDelete, fmt.Sprintf(\"/api/tasks/%d\", args.TaskID), nil)` — bridging to the existing `DELETE /api/tasks/{id}` route. The existing gated cleanup (stop sessions, kill tmux, delete row) runs unchanged through the bridge."
    # Test breadth (D-08)
    - "`internal/mcp/tasks_test.go` exists with one happy + one representative error case per tool (~12 total sub-tests): `TestBridge_ListTasks_*`, `TestBridge_GetTask_*`, `TestBridge_CreateTask_*`, `TestBridge_UpdateTask_*`, `TestBridge_MoveTask_*`, `TestBridge_DeleteTask_*`. Each test stands up an `httptest.Server` capturing method/path/body, constructs a `*bridge` pointed at it, invokes the handler with a `*mcp.CallToolRequest` carrying canned `Params.Arguments`, and asserts the captured request matches the expected method/path/body AND the result TextContent matches the canned response. Error cases assert the D-05 wrap substring (e.g. `\"HTTP 404\"`, `\"HTTP 400\"`) — NOT JSON key names (07-RESEARCH Gap 2: real shape is `{\"error\":\"...\"}`)."
    # Pattern fidelity — every handler delegates to bridge.call
    - "Every task tool handler in `tasks.go` follows the same shape: declare a typed args struct → `json.Unmarshal(req.Params.Arguments, &args)` (with error wrap) → build path/body → `return b.call(ctx, method, path, body)`. NO handler re-implements the `b.do` → `defer Close` → `io.ReadAll(LimitReader)` → non-200 wrap → TextContent sequence inline — all delegate to `bridge.call` (extracted in Plan 01)."
    # No bridge-side validation (D-03/D-04 — Kamacu's handlers validate)
    - "`tasks.go` performs NO validation of task_id values, title emptiness, status enum membership, or project_id existence — it sends fields as-is and lets Kamacu's existing handlers validate (the existing gates: empty title → 400 \"title is required\"; invalid status → 400 \"invalid status\"; missing task → 404 \"task not found\"; PR-review source on /move → 409 \"PR reviews are not board tasks\"). SC4 is satisfied by NOT duplicating validation in the bridge."
  artifacts:
    - internal/mcp/tasks.go (EXTENDED — registerTaskTools body filled with 6 s.AddTool calls; NEW bridge methods: listTasks, getTask, createTask, updateTask, moveTask, deleteTask)
    - internal/mcp/tasks_test.go (NEW — ~12 test cases covering all 6 tools, happy + error per D-08)
  key_links:
    - "registerTaskTools(s, b) → 6 s.AddTool calls for list_tasks, get_task, create_task, update_task, move_task, delete_task"
    - "bridge.listTasks(ctx, req) → b.call(GET, /api/projects/{id}/tasks OR /api/tasks, nil) — D-01 endpoint from Plan 01"
    - "bridge.getTask(ctx, req) → b.call(GET, /api/tasks/{id}, nil)"
    - "bridge.createTask(ctx, req) → b.call(POST, /api/projects/{id}/tasks, {title, description} body)"
    - "bridge.updateTask(ctx, req) → b.call(PATCH, /api/tasks/{id}, {title?, description?} partial body — D-03)"
    - "bridge.moveTask(ctx, req) → b.call(POST, /api/tasks/{id}/move, {status, after_id: null} body — D-06)"
    - "bridge.deleteTask(ctx, req) → b.call(DELETE, /api/tasks/{id}, nil)"
  prohibitions:
    - statement: "NO bridge-side validation — Kamacu's handlers are the validator. The bridge sends task_id / title / status / project_id as-is; an invalid value surfaces Kamacu's 400/404/409 verbatim through the D-05 wrap."
      status: resolved
      verification: "internal/mcp/tasks.go contains no call to a validation function; no `if args.Title == \"\"` style guard; the args struct fields are passed straight into the body map / path"
    - statement: "NO `after_id` arg exposed on move_task — D-06 hardcodes `after_id: null` in the body. The InputSchema for move_task lists ONLY `task_id` and `status` in properties; the handler marshals `{\"status\": ..., \"after_id\": nil}` unconditionally."
      status: resolved
      verification: "internal/mcp/tasks.go's move_task InputSchema `properties` map contains exactly `task_id` and `status`; the handler body sets `after_id` to nil (or omits it from the args struct and hardcodes it in the body map)"
    - statement: "NO agent auto-start on create_task — the underlying POST /api/projects/{id}/tasks handler never starts an agent; the MCP tool inherits this unchanged. The create_task description must NOT promise agent startup."
      status: resolved
      verification: "internal/mcp/tasks.go's create_task description does not contain the words 'start', 'spawn', or 'launch' near 'agent'; the handler makes a single b.call to POST /api/projects/{id}/tasks and returns"
    - statement: "DO NOT parse Kamacu error JSON in the bridge — D-05 passes the body as a raw string. Test assertions MUST use substrings of Kamacu message text (e.g. `\"task not found\"`, `\"title is required\"`, `\"invalid status\"`), NOT JSON key names (07-RESEARCH Gap 2 / Pitfall 2)."
      status: resolved
      verification: "internal/mcp/tasks_test.go contains no assertion matching on the literal string `\"message\"` or `\"status\"` as a JSON key; error assertions match on `HTTP 4NN`/`HTTP 5NN` substrings or Kamacu message text"
    - statement: "DO NOT touch internal/api/, internal/mcp/server.go, internal/mcp/bridge.go, internal/mcp/projects.go, or internal/mcp/workspaces.go — this plan owns ONLY internal/mcp/tasks.go and internal/mcp/tasks_test.go. Plan 01 already shipped the D-01 endpoint and the bridge.call helper this plan depends on."
      status: resolved
      verification: "`git diff --name-only` for this plan shows exactly two files: internal/mcp/tasks.go and internal/mcp/tasks_test.go"
    - statement: "DO NOT use the typed `mcp.AddTool[In, Out any]` generic — keep using the low-level `s.AddTool(*mcp.Tool, handler)` with `map[string]any` InputSchema (Phase 06 lock, 06-02-SUMMARY)."
      status: resolved
      verification: "`grep -rn 'mcp.AddTool\\[' internal/mcp/tasks.go` returns zero results"
---

# Plan 02: Six task tools (list_tasks, get_task, create_task, update_task, move_task, delete_task)

<objective>
Fill the `registerTaskTools` shell Plan 01 created with the six MCPTASK tools. Every tool is a thin HTTP-bridge handler delegating to `bridge.call` (extracted in Plan 01) — the same shape Phase 06 proved with `list_projects`, now applied to the task resource. This plan delivers MCPTASK-01 through MCPTASK-06 and satisfies SC1 (the six task tools drive the board, including worktree+branch auto-provisioning on create and the gated cleanup on delete).

Purpose: prove the bridge pattern scales. Six tools, each ~15 lines of handler + one `s.AddTool` registration, all delegating to the shared helper. No novel architecture — if any tool needs meaningfully different bridge logic, that's a signal to surface it (per 07-CONTEXT.md `<specifics>`).

Output: `internal/mcp/tasks.go` with six `bridge.<verb>Task` methods + six `s.AddTool` calls in `registerTaskTools`; `internal/mcp/tasks_test.go` with ~12 happy/error test cases per D-08.
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
@internal/api/tasks.go
@internal/api/routes.go
@internal/mcp/bridge.go
@internal/mcp/tasks.go
@internal/mcp/bridge_test.go
</context>

<tasks>

<task type="auto">
  <name>Task 1: Implement the six task tool handlers in registerTaskTools (list_tasks, get_task, create_task, update_task, move_task, delete_task)</name>
  <files>internal/mcp/tasks.go</files>
  <read_first>
    - `.planning/phases/07-tasks-projects-workspaces-tools/07-RESEARCH.md` § "Tool-to-Endpoint Mapping → Task tools" (the table with MCP args, bridge HTTP call, Kamacu handler line reference, and per-tool notes for all six tools), § "move_task position semantics (D-06)" (the `after_id == nil` → `MIN(position) - 1.0` top-of-column path at tasks.go:393-403), § "Architecture Patterns → Tool-handler canonical shape" (the 5-step shape every handler follows), § "Argument extraction in handlers" (the typed-struct `json.Unmarshal(req.Params.Arguments, &args)` pattern, with `*string` pointer fields for optional args per D-03), § "Common Pitfalls → Pitfall 2" (test assertions on Kamacu message substrings, not JSON key names)
    - `.planning/phases/07-tasks-projects-workspaces-tools/07-CONTEXT.md` decisions D-03 (`update_*` flat optional top-level args — `update_task` is `{task_id, title?, description?}`), D-05 (error wrap `fmt.Errorf("kamacu %s %s: HTTP %d: %s", method, path, code, body)` — body as raw string), D-06 (`move_task` always sends `after_id: null`), D-08 (one representative test per tool per resource), and the "the agent's Discretion" bullets (bridge method signatures, exact InputSchema map shape — Phase 06's flat `map[string]any` is the template)
    - `internal/api/tasks.go` (relevant ranges) — lines 182-219 (`listByProject` — the SELECT pattern; the new `list` handler from Plan 01 mirrors it), lines 223-272 (`create` — worktree auto-provisions synchronously, agent NOT auto-started; status ALWAYS 'todo'), lines 275-290 (`get` — 404 on missing), lines 294-341 (`update` — partial-PATCH with `*string` pointer fields), lines 347-471 (`move` — the `after_id == nil` top-of-column math at 393-403; the `source != "manual"` → 409 guard at 383-386; the invalid-status → 400 guard at 387-390), lines 553-589 (`delete` — gated cleanup: stop sessions, kill tmux, delete row, 204 on success, 404 if missing)
    - `internal/api/routes.go` lines 38-43 — the task routes this plan bridges to (GET /api/tasks from Plan 01, GET /api/projects/{id}/tasks, POST /api/projects/{id}/tasks, GET /api/tasks/{id}, PATCH /api/tasks/{id}, POST /api/tasks/{id}/move, DELETE /api/tasks/{id})
    - `internal/mcp/bridge.go` (post-Plan-01) — the `bridge.call(ctx, method, path, body)` helper every handler delegates to, plus `bridge.do`, `newBridgeFromEnv`, `maxBodyBytes`. The handler pattern is: declare args struct → unmarshal → build path/body → `return b.call(...)`.
    - `internal/mcp/tasks.go` (post-Plan-01) — the empty `registerTaskTools(s, b) {}` shell this plan fills. The package declaration, imports, and the empty function body are the starting point.
    - `internal/mcp/bridge_test.go` (post-Plan-01) — the `TestBridge_ListProjects_*` test pattern this plan's `tasks_test.go` mirrors: stand up `httptest.Server`, construct `&bridge{base: srv.URL, ...}`, build a `*mcp.CallToolRequest` with canned `Params.Arguments`, invoke the handler, assert captured request + result TextContent.
    - `.planning/phases/06-mcp-subcommand-foundation/06-02-SUMMARY.md` — the API-drift fix: `Tool.InputSchema` is `any` at SDK v1.6.1; use `map[string]any{"type": "object", "properties": {...}, "required": [...]}`, NOT `json.RawMessage` (which marshals as base64).
  </read_first>
  <action>
    Fill `registerTaskTools` in `internal/mcp/tasks.go` with six `s.AddTool` calls and add the six corresponding `*bridge` methods. Each method follows the canonical shape: typed args struct → `json.Unmarshal(req.Params.Arguments, &args)` (wrap unmarshal errors as `fmt.Errorf("<tool_name>: invalid arguments: %w", err)`) → build path/body → `return b.call(ctx, method, path, body)`. Imports needed: `bytes`, `context`, `encoding/json`, `fmt`, `net/http`, `github.com/modelcontextprotocol/go-sdk/mcp`.

    1. **list_tasks (MCPTASK-01).** InputSchema: `{type: object, properties: {project_id: {type: integer, description: "Optional project id. When supplied, lists tasks for that project via GET /api/projects/{id}/tasks. When omitted, lists all manual tasks across all projects via GET /api/tasks."}}}` — NO `required` array. Handler `bridge.listTasks(ctx, req)`:
       - `var args struct { ProjectID *int64 \`json:"project_id"\` }`
       - `if len(req.Params.Arguments) > 0 { json.Unmarshal(req.Params.Arguments, &args) }` (guard empty/nil args)
       - `if args.ProjectID != nil { return b.call(ctx, http.MethodGet, fmt.Sprintf("/api/projects/%d/tasks", *args.ProjectID), nil) }`
       - `return b.call(ctx, http.MethodGet, "/api/tasks", nil)` — the D-01 endpoint from Plan 01.

    2. **get_task (MCPTASK-02).** InputSchema: `{type: object, properties: {task_id: {type: integer, description: "The task id."}}, required: [task_id]}`. Handler `bridge.getTask(ctx, req)`:
       - `var args struct { TaskID int64 \`json:"task_id"\` }`
       - unmarshal with error wrap
       - `return b.call(ctx, http.MethodGet, fmt.Sprintf("/api/tasks/%d", args.TaskID), nil)`

    3. **create_task (MCPTASK-03).** InputSchema: `{type: object, properties: {project_id: {type: integer, description: "..."}, title: {type: string, description: "..."}, description: {type: string, description: "Optional task description."}}, required: [project_id, title]}`. The description field's schema description should note the worktree auto-provisions synchronously (up to 30s) and the agent is NOT auto-started. Handler `bridge.createTask(ctx, req)`:
       - `var args struct { ProjectID int64 \`json:"project_id"\`; Title string \`json:"title"\`; Description string \`json:"description"\` }`
       - unmarshal with error wrap
       - `body, _ := json.Marshal(map[string]any{"title": args.Title, "description": args.Description})`
       - `return b.call(ctx, http.MethodPost, fmt.Sprintf("/api/projects/%d/tasks", args.ProjectID), bytes.NewReader(body))`

    4. **update_task (MCPTASK-04, D-03 flat optional args).** InputSchema: `{type: object, properties: {task_id: {type: integer, description: "..."}, title: {type: string, description: "Optional new title."}, description: {type: string, description: "Optional new description."}}, required: [task_id]}` (title/description NOT in required — D-03 partial-PATCH). Handler `bridge.updateTask(ctx, req)`:
       - `var args struct { TaskID int64 \`json:"task_id"\`; Title *string \`json:"title"\`; Description *string \`json:"description"\` }` — pointer fields so nil = omitted vs `""` = explicit empty (D-03).
       - unmarshal with error wrap
       - Build body map conditionally: `body := map[string]any{}; if args.Title != nil { body["title"] = *args.Title }; if args.Description != nil { body["description"] = *args.Description }`
       - `jsonBody, _ := json.Marshal(body)`
       - `return b.call(ctx, http.MethodPatch, fmt.Sprintf("/api/tasks/%d", args.TaskID), bytes.NewReader(jsonBody))` — Kamacu's partial-PATCH leaves omitted keys untouched (empty body → Kamacu returns the current row, no error).

    5. **move_task (MCPTASK-05, D-06 after_id: null).** InputSchema: `{type: object, properties: {task_id: {type: integer, description: "..."}, status: {type: string, enum: ["todo", "in_progress", "in_review", "done"], description: "Target column. The task lands at the TOP of the target column."}}, required: [task_id, status]}`. Handler `bridge.moveTask(ctx, req)`:
       - `var args struct { TaskID int64 \`json:"task_id"\`; Status string \`json:"status"\` }` — NO `AfterID` field (D-06: not exposed).
       - unmarshal with error wrap
       - `body, _ := json.Marshal(map[string]any{"status": args.Status, "after_id": nil})` — `after_id` hardcoded to nil (top of column via the handler's `MIN(position) - 1.0` path).
       - `return b.call(ctx, http.MethodPost, fmt.Sprintf("/api/tasks/%d/move", args.TaskID), bytes.NewReader(body))`

    6. **delete_task (MCPTASK-06).** InputSchema: `{type: object, properties: {task_id: {type: integer, description: "..."}}, required: [task_id]}`. Handler `bridge.deleteTask(ctx, req)`:
       - `var args struct { TaskID int64 \`json:"task_id"\` }`
       - unmarshal with error wrap
       - `return b.call(ctx, http.MethodDelete, fmt.Sprintf("/api/tasks/%d", args.TaskID), nil)` — the existing gated cleanup (stop sessions, kill tmux, delete row) runs unchanged through the bridge.

    7. **Register all six** in `registerTaskTools` via `s.AddTool(&mcp.Tool{Name: "...", Description: "...", InputSchema: map[string]any{...}}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) { return b.<verb>Task(ctx, req) })`. Descriptions should be agent-readable: lead with what the tool does, follow with the Kamacu endpoint it bridges to (Phase 06's `list_projects` description is the template).
  </action>
  <verify>
    <automated>
      set -e
      # All six tools registered
      for tool in list_tasks get_task create_task update_task move_task delete_task; do
        grep -q "Name: \"$tool\"" internal/mcp/tasks.go || { echo "missing $tool"; exit 1; }
      done
      # All six bridge methods exist
      for method in listTasks getTask createTask updateTask moveTask deleteTask; do
        grep -q "func (b \*bridge) $method(" internal/mcp/tasks.go || { echo "missing $method"; exit 1; }
      done
      # move_task hardcodes after_id: nil (D-06) and exposes NO after_id arg
      grep -A2 'move_task\|moveTask' internal/mcp/tasks.go | grep -q '"after_id"'
      ! grep -q '"after_id"' internal/mcp/tasks.go | head -1 || true  # after_id appears ONLY in the body map, not in properties
      # update_task uses pointer fields (D-03 partial-PATCH)
      grep -q 'Title \*string' internal/mcp/tasks.go
      grep -q 'Description \*string' internal/mcp/tasks.go
      # Every handler delegates to b.call (no inline b.do + io.ReadAll sequence)
      ! grep -q 'io.ReadAll' internal/mcp/tasks.go
      ! grep -q 'b\.do(' internal/mcp/tasks.go
      # No typed AddTool[In,Out] generic
      ! grep -q 'mcp.AddTool\[' internal/mcp/tasks.go
      # No bridge-side validation guards
      ! grep -q 'if args\..*== ""' internal/mcp/tasks.go
      # go vet + build
      go vet ./internal/mcp/...
      go build ./...
    </automated>
  </verify>
  <acceptance_criteria>
    - `grep -c 'Name: "list_tasks"\|Name: "get_task"\|Name: "create_task"\|Name: "update_task"\|Name: "move_task"\|Name: "delete_task"' internal/mcp/tasks.go` returns 6
    - `grep -c 'func (b \*bridge) \(listTasks\|getTask\|createTask\|updateTask\|moveTask\|deleteTask)(' internal/mcp/tasks.go` returns 6
    - `grep -A2 'func (b \*bridge) moveTask' internal/mcp/tasks.go | tail -20` shows `"after_id": nil` in the body map (D-06) — assert via grep for `"after_id"` in the moveTask body
    - `! grep -q 'after_id' internal/mcp/tasks.go` would FAIL because after_id appears in the body — instead assert the move_task InputSchema `properties` map does NOT contain `after_id`: `grep -B2 -A20 'Name: "move_task"' internal/mcp/tasks.go | grep properties | head -1` should NOT contain `after_id`
    - `grep -q 'Title \*string' internal/mcp/tasks.go && grep -q 'Description \*string' internal/mcp/tasks.go` succeeds (update_task pointer fields — D-03)
    - `! grep -q 'b\.do(' internal/mcp/tasks.go` succeeds (every handler delegates to b.call, not b.do)
    - `! grep -q 'io\.ReadAll' internal/mcp/tasks.go` succeeds (no inline response reading — all via b.call)
    - `! grep -q 'mcp\.AddTool\[' internal/mcp/tasks.go` succeeds (no typed generic)
    - `go vet ./internal/mcp/...` passes
    - `go build ./...` passes (the package compiles with the rest of the codebase)
  </acceptance_criteria>
  <done>
    - registerTaskTools registers all six task tools (list_tasks, get_task, create_task, update_task, move_task, delete_task)
    - Six bridge methods (listTasks, getTask, createTask, updateTask, moveTask, deleteTask) each delegate to b.call
    - move_task hardcodes after_id: nil in the body (D-06); InputSchema exposes ONLY task_id + status
    - update_task uses *string pointer fields for title/description (D-03 partial-PATCH)
    - No handler re-implements the b.do + io.ReadAll sequence — all delegate to bridge.call
    - go vet + go build pass
  </done>
</task>

<task type="auto">
  <name>Task 2: Add tasks_test.go with ~12 happy/error test cases per D-08</name>
  <files>internal/mcp/tasks_test.go</files>
  <read_first>
    - `.planning/phases/07-tasks-projects-workspaces-tools/07-RESEARCH.md` § "Common Pitfalls → Pitfall 2" (test assertions on Kamacu message substrings, not JSON key names — the real error shape is `{"error":"..."}` per Gap 2), § "Don't Hand-Roll" (use `json.Unmarshal(req.Params.Arguments, &typedStruct)` — already done in production code; tests mirror by constructing `*mcp.CallToolRequest` with `Params.Arguments: json.RawMessage(...)`), § "Architecture Patterns → Tool-handler canonical shape"
    - `.planning/phases/07-tasks-projects-workspaces-tools/07-CONTEXT.md` D-08 (one representative test per tool per resource — ~12 cases for tasks: 6 tools × ~2 cases each)
    - `internal/mcp/bridge_test.go` (post-Plan-01) — the `httptest.NewServer` + `&bridge{base: srv.URL, ...}` + `*mcp.CallToolRequest` pattern this plan mirrors exactly. Note how the test captures `r.Method`, `r.URL.Path`, and the request body via a handler-scoped variable, then asserts after invoking the handler.
    - `internal/mcp/tasks.go` (post-Task-1 of this plan) — the production code under test; the test invokes each `bridge.<verb>Task` method directly (not through the SDK).
    - `internal/api/tasks.go` — the Kamacu handler behavior the test server simulates (e.g. `get` returns the task JSON or 404 with `{"error":"task not found"}`; `create` returns 201 with the task JSON; `move` returns 200 with the updated task; `delete` returns 204).
  </read_first>
  <action>
    Create `internal/mcp/tasks_test.go` (package `mcp`) with one happy + one representative error case per tool (~12 sub-tests total). Follow the `TestBridge_<Verb><Resource>_<Scenario>` naming convention from `bridge_test.go`. Imports: `context`, `encoding/json`, `net/http`, `net/http/httptest`, `strings`, `testing`, `time`, `github.com/modelcontextprotocol/go-sdk/mcp`.

    Each test stands up an `httptest.Server` whose handler captures `r.Method`, `r.URL.Path`, and (for POST/PATCH) the request body, then returns a canned response. The test constructs `b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10*time.Second}}`, builds a `*mcp.CallToolRequest` with `Params.Arguments: json.RawMessage(`{...}`)`, invokes the handler, and asserts:
    - The captured request method/path/body matches expectations.
    - On success: `res.Content[0].(*mcp.TextContent).Text` matches the canned response body.
    - On error: the returned error message contains the expected substring (e.g. `"HTTP 404"`, `"HTTP 400"`) — NOT JSON key names.

    Required cases (one happy + one error per tool, ~12 total):
    1. `TestBridge_ListTasks_NoProjectID_CallsUnscopedEndpoint` — Arguments `{}`; assert `r.URL.Path == "/api/tasks"` AND `r.Method == "GET"`.
    2. `TestBridge_ListTasks_WithProjectID_CallsScopedEndpoint` — Arguments `{"project_id":5}`; assert `r.URL.Path == "/api/projects/5/tasks"` AND `r.Method == "GET"`.
    3. `TestBridge_ListTasks_Kamacu500_ReturnsError` — server returns 500 with `{"error":"db down"}`; assert error contains `"HTTP 500"` (substring, not JSON key).
    4. `TestBridge_GetTask_Happy_CallsCorrectPath` — Arguments `{"task_id":7}`; assert `r.URL.Path == "/api/tasks/7"` AND method GET AND TextContent passthrough.
    5. `TestBridge_GetTask_NotFound404_ReturnsError` — server returns 404 with `{"error":"task not found"}`; assert error contains `"HTTP 404"`.
    6. `TestBridge_CreateTask_Happy_SendsTitleDescriptionBody` — Arguments `{"project_id":3,"title":"foo","description":"bar"}`; assert `r.URL.Path == "/api/projects/3/tasks"` AND method POST AND the request body decodes to `{"title":"foo","description":"bar"}`.
    7. `TestBridge_CreateTask_Kamacu400_TitleRequired_ReturnsError` — server returns 400 with `{"error":"title is required"}`; assert error contains `"HTTP 400"`.
    8. `TestBridge_UpdateTask_PartialPATCH_SendsOnlySuppliedFields` — Arguments `{"task_id":9,"title":"new"}` (description OMITTED); assert method PATCH AND the request body decodes to `{"title":"new"}` with NO `description` key (D-03).
    9. `TestBridge_UpdateTask_NoFields_KamacuReturnsCurrentRow` — Arguments `{"task_id":9}`; assert method PATCH AND body is `{}` AND the canned 200 response is passed through (Kamacu returns the current row for empty body).
    10. `TestBridge_MoveTask_HardcodedAfterIDNull` — Arguments `{"task_id":4,"status":"done"}`; assert method POST AND `r.URL.Path == "/api/tasks/4/move"` AND the request body decodes to `{"status":"done","after_id":null}` (D-06 — verify after_id is null in the body).
    11. `TestBridge_MoveTask_InvalidStatus_Kamacu400` — server returns 400 with `{"error":"invalid status"}`; assert error contains `"HTTP 400"`.
    12. `TestBridge_DeleteTask_Happy_CallsDeletePath` — Arguments `{"task_id":2}`; assert `r.URL.Path == "/api/tasks/2"` AND method DELETE.
    13. `TestBridge_DeleteTask_NotFound404_ReturnsError` — server returns 404 with `{"error":"task not found"}`; assert error contains `"HTTP 404"`.

    Use `t.Cleanup(srv.Close)` for every test server. Use `t.Setenv` only if a test exercises env (none should — all construct the bridge directly). For request body assertions, unmarshal the captured body into `map[string]any` and check key presence/absence (e.g. for the partial-PATCH test: `_, ok := body["description"]; assert !ok`).
  </action>
  <verify>
    <automated>
      set -e
      test -f internal/mcp/tasks_test.go
      # All six tools have at least one happy + one error test
      for tool in ListTasks GetTask CreateTask UpdateTask MoveTask DeleteTask; do
        grep -q "TestBridge_$tool" internal/mcp/tasks_test.go || { echo "missing $tool tests"; exit 1; }
      done
      # D-06 assertion exists: after_id is null in move_task body
      grep -B2 -A8 'TestBridge_MoveTask_HardcodedAfterIDNull' internal/mcp/tasks_test.go | grep -q 'after_id'
      # D-03 assertion exists: partial-PATCH omits description
      grep -B2 -A15 'TestBridge_UpdateTask_PartialPATCH' internal/mcp/tasks_test.go | grep -q 'description'
      # No JSON key name assertions on error bodies (Gap 2 / Pitfall 2)
      ! grep -q '"message"' internal/mcp/tasks_test.go
      # Uses httptest pattern
      grep -q 'httptest.NewServer' internal/mcp/tasks_test.go
      grep -q 'mcp.CallToolRequest' internal/mcp/tasks_test.go
      # go vet + tests pass
      go vet ./internal/mcp/...
      go test ./internal/mcp/... -v
    </automated>
  </verify>
  <acceptance_criteria>
    - `test -f internal/mcp/tasks_test.go` succeeds
    - `grep -c 'func TestBridge_' internal/mcp/tasks_test.go` returns at least 12 (one happy + one error per tool, per D-08)
    - For each of ListTasks/GetTask/CreateTask/UpdateTask/MoveTask/DeleteTask: `grep -q "TestBridge_<Tool>_" internal/mcp/tasks_test.go` succeeds
    - `grep -B2 -A10 'TestBridge_MoveTask_HardcodedAfterIDNull' internal/mcp/tasks_test.go | grep -q 'after_id'` succeeds (D-06 coverage — after_id asserted in the test)
    - `grep -B2 -A15 'TestBridge_UpdateTask_PartialPATCH' internal/mcp/tasks_test.go | grep -q 'description'` succeeds (D-03 coverage — partial-PATCH field omission asserted)
    - `! grep -q '"message"' internal/mcp/tasks_test.go` succeeds (Gap 2 / Pitfall 2 — no JSON key name assertions on error bodies)
    - `go test ./internal/mcp/... -v` passes (all task tests GREEN, plus Plan 01's tests still GREEN)
  </acceptance_criteria>
  <done>
    - internal/mcp/tasks_test.go exists with ~12 test cases covering all 6 tools (happy + error each)
    - move_task test asserts after_id: null in the body (D-06 coverage)
    - update_task test asserts partial-PATCH field omission (D-03 coverage)
    - No error-body assertion matches on JSON key names (Gap 2 / Pitfall 2)
    - `go test ./internal/mcp/... -v` passes
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| agent CLI → MCP subcommand stdin | Unchanged from Phase 06. The six new tools accept `*mcp.CallToolRequest` from the agent CLI; arguments are unmarshaled into typed structs. |
| MCP subcommand → Kamacu HTTP API (loopback) | Plain HTTP to existing task routes. No new routes added by this plan (Plan 01 added the only new one: GET /api/tasks). |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-07-05 | Tampering | Agent supplies malformed JSON in tool arguments | low | mitigate | Every handler wraps `json.Unmarshal(req.Params.Arguments, &args)` errors as `fmt.Errorf("<tool>: invalid arguments: %w", err)` and returns `(nil, err)` — the SDK surfaces this as a JSON-RPC error.code = -32603. No panic surface (no map writes on user input). |
| T-07-06 | Elevation of Privilege | `create_task` could auto-start an agent | low | accept | The underlying POST /api/projects/{id}/tasks handler never starts an agent (auto-start is Out of Scope per PROJECT.md). The MCP tool inherits this — it makes a single HTTP call and returns. No `mgr.Start` or equivalent is invoked from internal/mcp/. |
| T-07-07 | Denial of Service | `create_task` bridge timeout (10s) vs handler's 30s git timeout | low | accept | 07-RESEARCH "create_task timeout interaction" — pre-existing characteristic of the endpoint (the SPA has the same race). The bridge's 10s timeout may fire before a slow worktree-provision completes; the task may still get created server-side. NOT a Phase 07 bug; no retry logic added (Phase 06 locked 10s/no-retry). |

</threat_model>

<verification>
- `internal/mcp/tasks.go`'s `registerTaskTools` calls `s.AddTool` exactly 6 times for list_tasks, get_task, create_task, update_task, move_task, delete_task
- Six `bridge.<verb>Task` methods exist, each delegating to `b.call`
- `move_task` hardcodes `after_id: nil` in the body; InputSchema exposes only task_id + status (D-06)
- `update_task` uses `*string` pointer fields for title/description; partial-PATCH body omits nil fields (D-03)
- `internal/mcp/tasks_test.go` has ~12 test cases (happy + error per tool); move_task test asserts after_id: null; update_task test asserts partial-PATCH field omission
- No error-body assertion matches on JSON key names (Gap 2 / Pitfall 2)
- `go vet ./internal/mcp/...` passes; `go test ./internal/mcp/...` passes; `go build ./...` succeeds
</verification>

<success_criteria>
This plan delivers MCPTASK-01 through MCPTASK-06 and satisfies SC1 (the six task tools drive the board — list/get/create/update/move/delete, including worktree+branch auto-provisioning on create and the gated cleanup on delete, with no agent auto-start).
</success_criteria>

<output>
Create `.planning/phases/07-tasks-projects-workspaces-tools/07-02-SUMMARY.md` when done
</output>

## Artifacts this phase produces

This plan creates the following new symbols (consumed by the plan-review-convergence source-grounding pass; Plans 03/04 do NOT depend on these symbols — they own their own files):

- **MCP tools (new, registered in `registerTaskTools`):**
  - `list_tasks` — InputSchema `{project_id?: integer}`, handler bridges to `GET /api/projects/{id}/tasks` (scoped) or `GET /api/tasks` (unscoped, Plan 01's D-01 endpoint)
  - `get_task` — InputSchema `{task_id: integer (required)}`, handler bridges to `GET /api/tasks/{id}`
  - `create_task` — InputSchema `{project_id: integer (required), title: string (required), description?: string}`, handler bridges to `POST /api/projects/{id}/tasks`; agent NOT auto-started
  - `update_task` — InputSchema `{task_id: integer (required), title?: string, description?: string}` (D-03 flat optional args), handler bridges to `PATCH /api/tasks/{id}` with partial body
  - `move_task` — InputSchema `{task_id: integer (required), status: string (required, enum)}`, handler bridges to `POST /api/tasks/{id}/move` with `{status, after_id: null}` body (D-06)
  - `delete_task` — InputSchema `{task_id: integer (required)}`, handler bridges to `DELETE /api/tasks/{id}`
- **Bridge methods (new on `*bridge` in `internal/mcp/tasks.go`):**
  - `func (b *bridge) listTasks(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)`
  - `func (b *bridge) getTask(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)`
  - `func (b *bridge) createTask(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)`
  - `func (b *bridge) updateTask(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)`
  - `func (b *bridge) moveTask(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)`
  - `func (b *bridge) deleteTask(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)`
- **Test functions (new in `internal/mcp/tasks_test.go`):**
  - `TestBridge_ListTasks_NoProjectID_CallsUnscopedEndpoint`, `TestBridge_ListTasks_WithProjectID_CallsScopedEndpoint`, `TestBridge_ListTasks_Kamacu500_ReturnsError`
  - `TestBridge_GetTask_Happy_CallsCorrectPath`, `TestBridge_GetTask_NotFound404_ReturnsError`
  - `TestBridge_CreateTask_Happy_SendsTitleDescriptionBody`, `TestBridge_CreateTask_Kamacu400_TitleRequired_ReturnsError`
  - `TestBridge_UpdateTask_PartialPATCH_SendsOnlySuppliedFields`, `TestBridge_UpdateTask_NoFields_KamacuReturnsCurrentRow`
  - `TestBridge_MoveTask_HardcodedAfterIDNull`, `TestBridge_MoveTask_InvalidStatus_Kamacu400`
  - `TestBridge_DeleteTask_Happy_CallsDeletePath`, `TestBridge_DeleteTask_NotFound404_ReturnsError`
