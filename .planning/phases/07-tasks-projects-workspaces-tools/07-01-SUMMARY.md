---
phase: 07-tasks-projects-workspaces-tools
plan: 01
subsystem: api
tags: [mcp, http-bridge, rest-endpoints, kamacu-api, per-resource-split]

requires:
  - phase: 06-mcp-subcommand-foundation
    provides: "internal/mcp package with *bridge HTTP client + list_projects tool as FINAL Phase 06 production code; bridge.listProjects shape (b.do → defer Close → io.ReadAll(LimitReader) → non-200 wrap → TextContent passthrough); map[string]any InputSchema lock; SC2 regression test"
provides:
  - "Three Kamacu endpoint additions (D-01 GET /api/tasks, D-02 ?workspace_id= on GET /api/projects, Gap 1 GET /api/projects/{id}) — the prerequisites for MCPTASK-01 / MCPPROJ-01 / MCPPROJ-02"
  - "internal/mcp per-resource split (D-07): tasks.go + projects.go + workspaces.go each own a register*Tools(s, b); server.go's registerTools is a 3-line delegator"
  - "bridge.call(ctx, method, path, body) shared helper in bridge.go — the response-handling half every Phase 07 tool handler delegates to (halves per-tool line count)"
  - "list_projects updated for the real workspace_id? arg (Pitfall 6 — stale project_id no-op arg removed; D-02 ?workspace_id= query param wired through) — MCPPROJ-01 delivered HERE"
  - "Empty registerTaskTools + registerWorkspaceTools shells so Plans 02/03/04 each touch ONLY their own new file (clean parallel wave-2 boundaries)"
affects: [07-02-task-tools, 07-03-project-tools, 07-04-workspace-tools]

tech-stack:
  added: []
  patterns:
    - "Per-resource file split under internal/mcp/ mirroring internal/api/ (D-07): each resource owns its *bridge methods + registerXTools(s, b); bridge.go keeps shared primitives"
    - "bridge.call(ctx, method, path, body) shared helper — extracts the response-handling half of the Phase 06 listProjects pattern (do → close → LimitReader → non-200 wrap → TextContent)"
    - "Optional-arg extraction in MCP handlers: typed struct with *int64 pointer fields + json.Unmarshal(req.Params.Arguments, &args) when len(Arguments) > 0"
    - "Backward-compatible query-param extension: empty/unparseable value silently ignored (Kamacu API convention); only parseable values change behavior"

key-files:
  created:
    - internal/mcp/tasks.go
    - internal/mcp/projects.go
    - internal/mcp/workspaces.go
  modified:
    - internal/api/routes.go
    - internal/api/tasks.go
    - internal/api/projects.go
    - internal/api/tasks_test.go
    - internal/api/projects_test.go
    - internal/mcp/server.go
    - internal/mcp/bridge.go
    - internal/mcp/bridge_test.go

key-decisions:
  - "Extracted bridge.call as the shared response-handling helper (Claude's discretion per CONTEXT.md) — chosen because it halves per-tool line count for the 13 tools in Plans 02/03/04 and structurally enforces the 'every tool is the same shape' invariant."
  - "list_projects InputSchema: removed the stale project_id no-op property (Pitfall 6); declared workspace_id (integer, optional) as the ONLY property — no required array (optional arg)."
  - "list_projects handler: extract workspace_id into *int64; only append ?workspace_id=N when non-nil; bare /api/projects path otherwise (Phase 06 backward-compatible)."
  - "GET /api/tasks handler named taskHandlers.list (parallel to listByProject; the agent's Discretion per CONTEXT.md) — SELECT keeps source='manual' filter (GHREV-04) + ORDER BY status, position ASC; drops only the WHERE project_id = ? clause."
  - "GET /api/projects/{id} registered BEFORE /api/projects/{id}/github-origin and /api/projects/{id}/tasks (defensive ordering — Go 1.22 ServeMux longest-pattern-wins means no actual conflict, but matches the file's existing top-down convention)."
  - "?workspace_id= parsing strictness: empty string OR strconv.Atoi failure → silently ignored (returns all projects); only a parseable int triggers the WHERE clause. Kamacu API convention."
  - "Used *mcp.CallToolParamsRaw (not CallToolParams) when constructing *mcp.CallToolRequest in tests — the SDK's CallToolRequest = ServerRequest[*CallToolParamsRaw] so Params is *CallToolParamsRaw with Arguments as json.RawMessage."

patterns-established:
  - "Pattern: bridge.call(ctx, method, path, body) — every Phase 07+ tool handler builds path/body and delegates; the response-handling half is no longer inlined (Phase 06 inlined it once; Phase 07 inlines it zero times)."
  - "Pattern: per-resource register*Tools(s, b) — registerTools is now a thin orchestrator; each resource file owns its s.AddTool calls + its *bridge methods (single ownership across files)."
  - "Pattern: optional MCP arg via *T pointer + len(Arguments) > 0 guard before json.Unmarshal — avoids unmarshaling an empty/nil slice and cleanly distinguishes omitted (nil) from explicit zero."

requirements-completed: [MCPPROJ-01]

coverage:
  - id: D1
    description: "D-01: GET /api/tasks registered in routes.go + taskHandlers.list handler returns manual-only tasks ordered by status, position ASC"
    requirement: MCPTASK-01
    verification:
      - kind: integration
        ref: "grep -q 'mux.HandleFunc(\"GET /api/tasks\", t.list)' internal/api/routes.go"
        status: pass
      - kind: integration
        ref: "grep -A3 'func (h \\*taskHandlers) list(' internal/api/tasks.go | grep -q \"source = 'manual'\""
        status: pass
      - kind: unit
        ref: "internal/api/tasks_test.go#TestTaskList_All_ReturnsOnlyManualTasksOrderedByStatusPosition — PASS (1 manual + 1 PR seeded; only manual returned, ordered done<todo by string-compare status ASC)"
        status: pass
      - kind: unit
        ref: "internal/api/tasks_test.go#TestTaskList_EmptyReturnsEmptyArray — PASS (empty DB returns [] not null)"
        status: pass
    human_judgment: false
  - id: D2
    description: "D-02: GET /api/projects reads ?workspace_id= query param, filters when parseable, ignores when empty/unparseable (Kamacu convention)"
    requirement: MCPPROJ-01
    verification:
      - kind: integration
        ref: "grep -q 'r.URL.Query().Get(\"workspace_id\")' internal/api/projects.go"
        status: pass
      - kind: unit
        ref: "internal/api/projects_test.go#TestProjectList_WorkspaceIDFilter_ReturnsOnlyThatWorkspace — PASS (workspace 2 filter returns only the workspace-2 project)"
        status: pass
      - kind: unit
        ref: "internal/api/projects_test.go#TestProjectList_NoFilter_ReturnsAll — PASS (backward-compat regression: no query param → all projects)"
        status: pass
      - kind: unit
        ref: "internal/api/projects_test.go#TestProjectList_WorkspaceIDUnparseable_IgnoresAndReturnsAll — PASS (abc silently ignored → all returned)"
        status: pass
    human_judgment: false
  - id: D3
    description: "Gap 1: GET /api/projects/{id} registered + projectHandlers.get returns single project JSON (404 on missing) — backs MCPPROJ-02 (delivered in Plan 03)"
    requirement: MCPPROJ-02
    verification:
      - kind: integration
        ref: "grep -q 'mux.HandleFunc(\"GET /api/projects/{id}\", p.get)' internal/api/routes.go"
        status: pass
      - kind: unit
        ref: "internal/api/projects_test.go#TestProjectGet_Found_ReturnsProjectJSON — PASS (existing id returns project JSON via writeJSON)"
        status: pass
      - kind: unit
        ref: "internal/api/projects_test.go#TestProjectGet_NotFound_Returns404 — PASS (9999 → 404 with {\"error\":\"project not found\"})"
        status: pass
    human_judgment: false
  - id: D4
    description: "D-07 per-resource split: tasks.go/projects.go/workspaces.go created; registerTools delegates to three registrars (no inline AddTool)"
    requirement: MCPPROJ-01
    verification:
      - kind: integration
        ref: "test -f internal/mcp/{tasks.go,projects.go,workspaces.go}"
        status: pass
      - kind: integration
        ref: "grep -A6 'func registerTools(' internal/mcp/server.go shows three registrar calls and no inline s.AddTool"
        status: pass
      - kind: integration
        ref: "projects.go: 1 s.AddTool call (list_projects only); tasks.go + workspaces.go: 0 s.AddTool calls (empty shells for Plans 02/04)"
        status: pass
    human_judgment: false
  - id: D5
    description: "bridge.call shared helper extracted to bridge.go; listProjects method MOVED out of bridge.go into projects.go"
    requirement: MCPPROJ-01
    verification:
      - kind: integration
        ref: "grep -q 'func (b \\*bridge) call(ctx context.Context, method, path string, body io.Reader) (\\*mcp.CallToolResult, error)' internal/mcp/bridge.go"
        status: pass
      - kind: integration
        ref: "! grep -q 'func (b \\*bridge) listProjects(' internal/mcp/bridge.go (method moved out)"
        status: pass
      - kind: integration
        ref: "grep -q 'fmt.Errorf(\"kamacu %s %s: HTTP %d: %s\", method, path, resp.StatusCode, string(respBody))' internal/mcp/bridge.go (D-05 wrap verbatim, body as raw string)"
        status: pass
    human_judgment: false
  - id: D6
    description: "list_projects updated for the real workspace_id? arg: stale project_id no-op arg removed; handler extracts workspace_id and appends ?workspace_id=N when supplied (Pitfall 6 + D-02)"
    requirement: MCPPROJ-01
    verification:
      - kind: integration
        ref: "grep -q '\"workspace_id\"' internal/mcp/projects.go AND ! grep -q '\"project_id\"' internal/mcp/projects.go"
        status: pass
      - kind: unit
        ref: "internal/mcp/bridge_test.go#TestBridge_ListProjects_WorkspaceIDAppendsQueryParam — PASS (Arguments {workspace_id:2} → URL.Path=/api/projects, RawQuery=workspace_id=2)"
        status: pass
      - kind: unit
        ref: "internal/mcp/bridge_test.go#TestBridge_ListProjects_NoWorkspaceID_NoQueryParam — PASS (no args → URL.Path=/api/projects, RawQuery=\"\" — Phase 06 backward-compat)"
        status: pass
      - kind: unit
        ref: "internal/mcp/bridge_test.go#TestBridge_ListProjects_PassesThroughKamacuJSON — PASS (X-Kamacu-Token sent, raw passthrough, /api/projects path, GET method)"
        status: pass
      - kind: unit
        ref: "internal/mcp/bridge_test.go#TestBridge_ListProjects_KamacuReturnsNon200_ReturnsError — PASS (HTTP 500 substring in error)"
        status: pass
      - kind: unit
        ref: "internal/mcp/bridge_test.go#TestBridge_ListProjects_KamacuUnreachable_ReturnsError — PASS (kamacu bridge: substring in error)"
        status: pass
    human_judgment: false
  - id: D7
    description: "SC2 regression byte-for-byte unchanged (Pitfall 4) — server_test.go has zero diff vs HEAD~2; both TestSC2_* sub-tests GREEN"
    requirement: MCPPROJ-01
    verification:
      - kind: integration
        ref: "test -z \"$(git diff HEAD~2 -- internal/mcp/server_test.go)\""
        status: pass
      - kind: unit
        ref: "go test ./internal/mcp/... -run TestSC2 -v — both sub-tests PASS"
        status: pass
    human_judgment: false
  - id: D8
    description: "No schema changes / migrations / new goroutines added — v1.11 milestone rule preserved"
    requirement: MCPPROJ-01
    verification:
      - kind: integration
        ref: "git diff --name-only HEAD~2 HEAD -- internal/store/migrations/ returns empty (no migration files touched)"
        status: pass
      - kind: integration
        ref: "No 'go func()' added to internal/api/{tasks.go,projects.go} or internal/mcp/* (grep returns nothing)"
        status: pass
    human_judgment: false
  - id: D9
    description: "go vet + go test on internal/api/... + internal/mcp/... all clean"
    requirement: MCPPROJ-01
    verification:
      - kind: integration
        ref: "go vet ./internal/api/... ./internal/mcp/... — clean"
        status: pass
      - kind: integration
        ref: "go test ./internal/api/... ./internal/mcp/... — both packages ok (full API suite ~100s, mcp suite <1s)"
        status: pass
    human_judgment: false

duration: 14min
completed: 2026-07-22
status: complete
---

# Phase 07 Plan 01: Kamacu Endpoints + internal/mcp D-07 Scaffold + list_projects(workspace_id?) Summary

**Three Kamacu endpoint additions (D-01 GET /api/tasks, D-02 ?workspace_id= on GET /api/projects, Gap 1 GET /api/projects/{id}) plus the D-07 per-resource split (tasks.go/projects.go/workspaces.go) and bridge.call shared helper — laying the foundation Plans 02/03/04 each fill one file.**

## Performance

- **Duration:** ~14 min
- **Started:** 2026-07-22T05:35:37Z
- **Completed:** 2026-07-22T05:50:15Z
- **Tasks:** 2
- **Files modified:** 11 (5 new + 6 modified)

## Accomplishments
- Added the three Kamacu endpoint additions the bridge tools depend on: `GET /api/tasks` (D-01 — unscoped manual-only list, `source='manual'` filter + `ORDER BY status, position ASC` preserved), `GET /api/projects/{id}` (Gap 1 — single-project fetch, 404 on missing), and `?workspace_id=N` query param on `GET /api/projects` (D-02 — backward-compatible, silently ignores unparseable values per Kamacu convention).
- Added 7 new API integration tests covering all three additions + the backward-compat regression (`TestTaskList_All_ReturnsOnlyManualTasksOrderedByStatusPosition`, `TestTaskList_EmptyReturnsEmptyArray`, `TestProjectGet_Found_ReturnsProjectJSON`, `TestProjectGet_NotFound_Returns404`, `TestProjectList_WorkspaceIDFilter_ReturnsOnlyThatWorkspace`, `TestProjectList_NoFilter_ReturnsAll`, `TestProjectList_WorkspaceIDUnparseable_IgnoresAndReturnsAll`). Full `./internal/api/...` suite green.
- Executed the D-07 per-resource split: created `internal/mcp/tasks.go`, `internal/mcp/projects.go`, `internal/mcp/workspaces.go` — each with a `register<Task|Project|Workspace>Tools(s, b)` function. `registerTools` in `server.go` shrunk from a single inline `s.AddTool` for `list_projects` to a 3-line delegator calling all three registrars. The split gives Plans 02/03/04 clean parallel wave-2 boundaries (each touches only its own new file).
- Extracted `bridge.call(ctx, method, path, body)` shared helper into `internal/mcp/bridge.go` — encapsulates the response-handling half of Phase 06's `listProjects` shape (`b.do` → `defer Close` → `io.ReadAll(LimitReader)` → transport-error wrap → non-200 D-05 wrap → 200 TextContent passthrough). Every Phase 07 tool handler in Plans 02/03/04 will be a ~5-line build-path-and-delegate.
- Moved `listProjects` method from `bridge.go` to `projects.go` (single ownership of the project resource) AND updated it for the real `workspace_id?` arg (Pitfall 6 — stale `project_id` no-op arg removed; `*int64` extraction + `?workspace_id=N` path append when supplied). Delivers MCPPROJ-01 in this plan.
- Updated `bridge_test.go` for the new `(ctx, req *mcp.CallToolRequest)` `listProjects` signature; added `TestBridge_ListProjects_WorkspaceIDAppendsQueryParam` + `TestBridge_ListProjects_NoWorkspaceID_NoQueryParam` proving the workspace_id? path AND backward-compat. SC2 regression test byte-for-byte unchanged; both sub-tests GREEN.
- Verified the v1.11 milestone rule is preserved: no schema changes, no migrations, no new long-lived goroutines (all 3 endpoint additions are pure SELECT additions / route registrations).

## Task Commits

Each task was committed atomically:

1. **Task 1: Add the three Kamacu endpoint additions (D-01 GET /api/tasks, D-02 ?workspace_id= query param, Gap 1 GET /api/projects/{id})** — `fd9c78a` (feat)
2. **Task 2: D-07 per-resource file split + bridge.call shared helper + list_projects(workspace_id?) + test updates** — `077ca1c` (feat)

**Plan metadata:** this commit (docs: complete endpoints-and-mcp-scaffold plan)

## Files Created/Modified
- `internal/api/routes.go` — 2 new `mux.HandleFunc` lines (`GET /api/tasks`, `GET /api/projects/{id}`); route count 21 → 23.
- `internal/api/tasks.go` — NEW method `func (h *taskHandlers) list(w, r)`: clones `listByProject` SELECT with `WHERE project_id = ?` clause dropped; keeps `source = 'manual'` filter (GHREV-04) + `ORDER BY status, position ASC`.
- `internal/api/projects.go` — NEW method `func (h *projectHandlers) get(w, r)` (mirrors `taskHandlers.get`, 404 on `sql.ErrNoRows`); MODIFIED `projectHandlers.list` reads `r.URL.Query().Get("workspace_id")`, parameterized `WHERE workspace_id = ?` when parseable (D-02).
- `internal/api/tasks_test.go` — `TestTaskList_All_ReturnsOnlyManualTasksOrderedByStatusPosition`, `TestTaskList_EmptyReturnsEmptyArray` (uses existing `createProject`/`createTask`/`moveTask`/`insertPRRow`/`doJSONList` helpers).
- `internal/api/projects_test.go` — `TestProjectGet_Found_ReturnsProjectJSON`, `TestProjectGet_NotFound_Returns404`, `TestProjectList_WorkspaceIDFilter_ReturnsOnlyThatWorkspace`, `TestProjectList_NoFilter_ReturnsAll`, `TestProjectList_WorkspaceIDUnparseable_IgnoresAndReturnsAll`.
- `internal/mcp/server.go` — `registerTools` shrunk to 3-line delegator (`registerTaskTools` / `registerProjectTools` / `registerWorkspaceTools`); the `ServeCommand` body is byte-for-byte unchanged.
- `internal/mcp/bridge.go` — `listProjects` method REMOVED (moved to `projects.go`); NEW `bridge.call(ctx, method, path, body) (*mcp.CallToolResult, error)` shared helper with D-05 wrap verbatim (`fmt.Errorf("kamacu %s %s: HTTP %d: %s", method, path, resp.StatusCode, string(respBody))`).
- `internal/mcp/projects.go` (NEW) — `registerProjectTools(s, b)` registers `list_projects` with the real `workspace_id?` arg; `func (b *bridge) listProjects(ctx, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)` extracts `workspace_id` into `*int64` and delegates to `b.call(ctx, GET, path, nil)`.
- `internal/mcp/tasks.go` (NEW) — empty `registerTaskTools(s, b)` shell (Plan 02 fills it).
- `internal/mcp/workspaces.go` (NEW) — empty `registerWorkspaceTools(s, b)` shell (Plan 04 fills it).
- `internal/mcp/bridge_test.go` — three `TestBridge_ListProjects_*` tests updated to construct `*mcp.CallToolRequest` and call `b.listProjects(ctx, req)`; NEW `TestBridge_ListProjects_WorkspaceIDAppendsQueryParam` + `TestBridge_ListProjects_NoWorkspaceID_NoQueryParam`; `TestNewBridgeFromEnv_DefaultsAndOverride` unchanged.

## Decisions Made
- **Extracted `bridge.call` as the shared helper** (Claude's discretion per CONTEXT.md "Whether the bridge has a shared `doJSON helper`"). Chose YES over inline because (a) Plans 02/03/04 add 12 more tools — halving per-tool line count pays off immediately; (b) it structurally enforces the "every tool is the same shape" invariant (the per-resource handlers can't accidentally diverge); (c) the response-handling half is identical across GET/POST/PATCH/DELETE so extraction has zero conditional logic.
- **`list_projects` InputSchema**: `workspace_id` (integer, optional) as the ONLY property — no `required` array. Stale `project_id` no-op arg from Phase 06 REMOVED (Pitfall 6).
- **`list_projects` handler**: declared `var args struct { WorkspaceID *int64 \`json:"workspace_id"\` }`, unmarshal only when `len(req.Params.Arguments) > 0` (avoids error on nil/empty args), bare `/api/projects` path when `args.WorkspaceID == nil` (Phase 06 backward-compatible).
- **`GET /api/tasks` handler named `taskHandlers.list`** (parallel to `listByProject`) per CONTEXT.md the agent's Discretion. Avoids `listAll` (the spec calls it "unscoped", not "all" — `list` reads cleaner).
- **`GET /api/projects/{id}` registered BEFORE `/api/projects/{id}/github-origin`** — defensive ordering matching the file's existing top-down convention. Go 1.22 ServeMux longest-pattern-wins means no actual conflict, but the explicit ordering is safer and clearer (07-RESEARCH Pitfall 5).
- **`?workspace_id=` parsing strictness** per CONTEXT.md the agent's Discretion: empty string OR `strconv.Atoi` failure → silently ignored (returns all projects). Only a parseable int triggers `WHERE workspace_id = ?`. Matches the Kamacu API convention of silently ignoring unknown/unparseable query params.
- **Used `*mcp.CallToolParamsRaw`** (not `CallToolParams`) when constructing `*mcp.CallToolRequest` in tests — SDK v1.6.1 defines `CallToolRequest = ServerRequest[*CallToolParamsRaw]`, so `Params` is `*CallToolParamsRaw` with `Arguments` as `json.RawMessage`. (Phase 06's tests didn't construct one because `listProjects` took only `ctx`; the new `(ctx, req)` signature forced this clarification.)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Test Logic] TestTaskList_All status-ordering assertion was inverted**
- **Found during:** Task 1 (running the new test)
- **Issue:** The test asserted `todo` < `done` (assuming board-column order), but the SQL `ORDER BY status, position ASC` is a plain string comparison — alphabetically `done` < `in_progress` < `in_review` < `todo`. The test's expected ordering did not match what the production SELECT actually does.
- **Fix:** Inverted the assertion to `list[0].status == "done"` and `list[1].status == "todo"` (matching the actual SQL string-compare behavior). The plan's `must_haves` only require "ORDER BY status, position ASC ordering" preserved — which it is, verbatim.
- **Files modified:** `internal/api/tasks_test.go`
- **Verification:** `go test ./internal/api/... -run TestTaskList_All -v` — PASS.
- **Committed in:** `fd9c78a` (Task 1 commit).

**2. [Rule 1 - Test Fixture] TestProjectList_NoFilter_ReturnsAll and TestProjectList_WorkspaceIDFilter reused the same repo_path**
- **Found during:** Task 1 (running the new tests)
- **Issue:** The tests seeded a second project with the same `repo := gitRepo(t)` value, but `projects.repo_path` has a UNIQUE constraint → `UNIQUE constraint failed: projects.repo_path (2067)`.
- **Fix:** Each test now creates a SECOND distinct git repo via a separate `gitRepo(t)` call for the workspace-2 fixture project (different temp dir → different path → constraint satisfied).
- **Files modified:** `internal/api/projects_test.go`
- **Verification:** `go test ./internal/api/... -run 'TestProjectList_WorkspaceIDFilter|TestProjectList_NoFilter' -v` — both PASS.
- **Committed in:** `fd9c78a` (Task 1 commit).

**3. [Rule 1 - API Drift] Used `mcp.CallToolParams` instead of `*mcp.CallToolParamsRaw` in test helper**
- **Found during:** Task 2 (running the mcp tests after updating listProjects signature)
- **Issue:** SDK v1.6.1 defines `type CallToolRequest = ServerRequest[*CallToolParamsRaw]`, so the `Params` field is `*CallToolParamsRaw` (Arguments as `json.RawMessage`), NOT `CallToolParams` (Arguments as `any`). Initial test helper used `CallToolParams` which `go vet` rejected: `cannot use mcpsdk.CallToolParams{…} as *mcpsdk.CallToolParamsRaw value in struct literal`.
- **Fix:** Changed `newCallToolRequest` helper to construct `Params: &mcpsdk.CallToolParamsRaw{Arguments: args}`. The production code in `projects.go` already used `req.Params.Arguments` (json.RawMessage) correctly — only the test helper was wrong.
- **Files modified:** `internal/mcp/bridge_test.go`
- **Verification:** `go vet ./internal/mcp/...` clean; all 8 mcp tests PASS.
- **Committed in:** `077ca1c` (Task 2 commit).

---

**Total deviations:** 3 auto-fixed (all Rule 1 — test logic / fixture / API drift).
**Impact on plan:** All three auto-fixes preserve the plan's intent (correct status-ordering assertion; UNIQUE-constraint-respecting fixtures; correct SDK type). No scope creep. The plan's prohibitions (no schema change; no migrations; no new goroutines; no typed `mcp.AddTool[In,Out]`; SC2 byte-for-byte unchanged; `source='manual'` filter preserved; no `project_id` in InputSchema) are all respected.

## Issues Encountered
None beyond the three Rule 1 auto-fixes above. The full `go test ./internal/api/... ./internal/mcp/...` is clean. SC2 regression is byte-for-byte unchanged (zero diff vs `HEAD~2` on `internal/mcp/server_test.go`).

## User Setup Required
None — this plan adds three localhost-only REST endpoints and restructures the internal/mcp package; no external service configuration is required.

## Next Phase Readiness
- **Plan 02 (task tools) ready.** `internal/mcp/tasks.go` has the empty `registerTaskTools(s, b)` shell; the Plan 02 executor adds the six `s.AddTool` calls for `list_tasks` / `get_task` / `create_task` / `update_task` / `move_task` / `delete_task` and the six `*bridge` methods, each delegating to `bridge.call`. `GET /api/tasks` (D-01) is already registered.
- **Plan 03 (project tools) ready.** `internal/mcp/projects.go` already owns `registerProjectTools` and the updated `listProjects`; Plan 03 ADDS the four remaining project tools (`get_project` / `create_project` / `update_project` / `delete_project`) in the same file. `GET /api/projects/{id}` (Gap 1) is already registered.
- **Plan 04 (workspace tools) ready.** `internal/mcp/workspaces.go` has the empty `registerWorkspaceTools(s, b)` shell; Plan 04 adds the five workspace tools. The underlying `GET/POST/PATCH/DELETE /api/workspaces` endpoints already exist unchanged.
- **MCPPROJ-01 delivered HERE** (list_projects with the real `workspace_id?` arg via D-02's query param). MCPTASK-01..06, MCPPROJ-02..05, MCPPROJ-06..07 land in Plans 02/03/04.

---
*Phase: 07-tasks-projects-workspaces-tools*
*Completed: 2026-07-22*

## Self-Check: PASSED

- Created files exist on disk: `internal/mcp/tasks.go`, `internal/mcp/projects.go`, `internal/mcp/workspaces.go`, `.planning/phases/07-tasks-projects-workspaces-tools/07-01-SUMMARY.md` — all FOUND.
- Task commits exist in git log: `fd9c78a` (Task 1, feat) and `077ca1c` (Task 2, feat) — both FOUND.
- Re-ran `go test ./internal/mcp/... ./internal/api/...` post-SUMMARY: both packages `ok` (cached, but the live run during Task verification matches).
- Plan-level verification gate (12 checks) all PASS: route count 21→23, no migrations touched, no new goroutines, SC2 byte-for-byte unchanged, bridge.call + 3 registrars + list_projects(workspace_id?) all in their target files.
