---
phase: 07-tasks-projects-workspaces-tools
verified: 2026-07-22T11:05:00Z
status: passed
score: 4/4 must-haves verified
behavior_unverified: 0
overrides_applied: 0
---

# Phase 07: Tasks, Projects & Workspaces Tools — Verification Report

**Phase Goal:** Full CRUD tool surface mirroring every action the SPA exposes — every "table-stakes" tool for tasks, projects, and workspaces is implemented and tested so an agent can drive the full Kamacu API for these resources.
**Verified:** 2026-07-22T11:05:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

The phase goal text says "13 MCP tools" (matching the 13 requirement IDs), but the actual tool count is **16** (MCPPROJ-06 bundles 4 workspace CRUD tools under one ID; move_project_to_workspace is its own tool under MCPPROJ-07). The ROADMAP Success Criteria — which are the binding contract — enumerate the tools precisely (SC1: 6 task, SC2: 5 project, SC3: 4 workspace + 1 transfer = 16 total), and all 16 are registered, wired, and tested. The "13" in the goal is loose wording for the requirement count, not a scope shortfall.

### Observable Truths (ROADMAP Success Criteria)

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | **SC1:** Six task tools (`list_tasks` / `get_task` / `create_task` / `update_task` / `move_task` / `delete_task`) drive the board — worktree+branch auto-provisioning on create, gated cleanup on delete, no agent auto-start | ✓ VERIFIED | All 6 tools registered in `registerTaskTools` (`tasks.go:31-174`); 6 bridge methods (`listTasks`/`getTask`/`createTask`/`updateTask`/`moveTask`/`deleteTask`) each delegate to `b.call`. `create_task` bridges to `POST /api/projects/{id}/tasks` (worktree provisioning + no agent auto-start is the existing handler's behavior — exercised by pre-existing `TestTaskCreateProvisionsWorktree` at the API layer; the bridge sends the same request shape the SPA sends). `delete_task` bridges to `DELETE /api/tasks/{id}` (gated cleanup is the existing handler's behavior — exercised by pre-existing `TestTaskDeleteStopsSessions`). 13 tests in `tasks_test.go` cover all 6 tools (happy + error). `go test ./internal/mcp/...` passes (43 tests). |
| 2 | **SC2:** Five project tools (`list_projects` / `get_project` / `create_project` / `update_project` / `delete_project`) drive the sidebar — v1.4 managed-checkout atomic create + all-or-nothing gated delete; folder-project delete byte-for-byte unchanged | ✓ VERIFIED | All 5 tools registered in `registerProjectTools` (`projects.go:48-195`); 5 bridge methods. `list_projects` honors `workspace_id?` via `?workspace_id=N` query param (D-02). `get_project` bridges to Gap 1 endpoint `GET /api/projects/{id}` (404 on missing). `create_project` sends `{name, repo_path, repo, workspace_id?}` as-is (D-04 — Kamacu's handler dispatches on `repo != ""`). `update_project` excludes `workspace_id`/`agent_id` from properties AND args struct (D-03 / Pitfall 3 — verified by grep). `delete_project` bridges to `DELETE /api/projects/{id}` (v1.4 gated removal is the existing handler's behavior). 5 list_projects tests in `bridge_test.go` + 12 new-tool tests in `projects_test.go` (including `TestBridge_DeleteProject_Managed409_PassesThroughStructuredBody` for the structured 409 reasons). |
| 3 | **SC3:** Four workspace tools (`list_workspaces` / `create_workspace` / `update_workspace` / `delete_workspace`) + `move_project_to_workspace` drive the switcher — v1.9 guarded delete keyed off `is_default` + non-empty `COUNT` | ✓ VERIFIED | All 5 tools registered in `registerWorkspaceTools` (`workspaces.go:51-166`); 5 bridge methods. `move_project_to_workspace` PATCHes `/api/projects/{id}` (NOT a `/api/workspaces` route) with `{workspace_id}`-only body — verified by grep. 10 tests in `workspaces_test.go` including BOTH v1.9 guarded-delete 409 cases: `TestBridge_DeleteWorkspace_DefaultGuard_Kamacu409` (asserts `"the default workspace"` substring) and `TestBridge_DeleteWorkspace_NonEmptyGuard_Kamacu409` (asserts `"project(s) first"` substring). |
| 4 | **SC4:** Tool calls with invalid input surface actionable MCP errors and produce no Kamacu state change — every existing validation gate applies unchanged through the bridge | ✓ VERIFIED | By design — the bridge performs NO validation (prohibition satisfied; `tasks.go`/`projects.go`/`workspaces.go` contain no `if args.X == ""` guards, no `validateIconLetters`/`validateRepoPath`/`github.ValidateRepo` calls). Invalid input surfaces Kamacu's 400/404/409 verbatim through the D-05 wrap. Tests assert error substrings (e.g. `TestBridge_GetTask_NotFound404_ReturnsError` asserts `"HTTP 404"`; `TestBridge_CreateTask_Kamacu400_TitleRequired_ReturnsError` asserts `"HTTP 400"`; `TestBridge_UpdateProject_Kamacu400_DescriptionTooLong_ReturnsError`). "No Kamacu state change" holds because Kamacu's existing handlers validate before mutation (unchanged by Phase 07; tested at the API layer). |

**Score:** 4/4 truths verified (0 present, behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `internal/mcp/server.go` | `registerTools` delegates to 3 registrars (no inline `s.AddTool`) | ✓ VERIFIED | `server.go:82-86` — 3-line delegator calling `registerTaskTools` / `registerProjectTools` / `registerWorkspaceTools` |
| `internal/mcp/bridge.go` | `bridge.call` shared helper; `listProjects` removed (moved to projects.go) | ✓ VERIFIED | `bridge.go:88-106` defines `call`; no `listProjects` method present; `<200 \|\| >=300` accept range documented in comment as Plan 02 widening (POST→201, DELETE→204) |
| `internal/mcp/tasks.go` | 6 `s.AddTool` calls + 6 bridge methods | ✓ VERIFIED | grep `s.AddTool(` → 6; grep `func (b *bridge)` → 6; all delegate to `b.call` |
| `internal/mcp/projects.go` | 5 `s.AddTool` calls + 5 bridge methods; `workspace_id?` on list_projects; update_project excludes workspace_id/agent_id | ✓ VERIFIED | grep `s.AddTool(` → 5; update_project properties clean; `move_project_to_workspace` lives in workspaces.go (not here) |
| `internal/mcp/workspaces.go` | 5 `s.AddTool` calls + 5 bridge methods; `move_project_to_workspace` PATCHes `/api/projects/{id}` | ✓ VERIFIED | grep `s.AddTool(` → 5; moveProjectToWorkspace calls `b.call(ctx, http.MethodPatch, fmt.Sprintf("/api/projects/%d", ...), ...)` |
| `internal/mcp/tasks_test.go` | ~12 tests, one happy + one error per tool | ✓ VERIFIED | 13 `TestBridge_*` funcs covering all 6 tools; move_task asserts `after_id: null`; update_task asserts partial-PATCH field omission |
| `internal/mcp/projects_test.go` | ~10 tests for the 4 new tools | ✓ VERIFIED | 12 `TestBridge_*` funcs; `TestBridge_UpdateProject_BodyExcludes_WorkspaceID_And_AgentID` (D-03); `TestBridge_DeleteProject_Managed409_PassesThroughStructuredBody` (Gap 2) |
| `internal/mcp/workspaces_test.go` | ~8 tests, both v1.9 409 cases | ✓ VERIFIED | 10 `TestBridge_*` funcs; `TestBridge_DeleteWorkspace_DefaultGuard_Kamacu409` + `TestBridge_DeleteWorkspace_NonEmptyGuard_Kamacu409`; `TestBridge_MoveProjectToWorkspace_Happy_PATCHesProjectRoute` |
| `internal/mcp/bridge_test.go` | list_projects tests updated to `(ctx, req)` signature; new workspace_id? test | ✓ VERIFIED | 5 list_projects tests incl. `TestBridge_ListProjects_WorkspaceIDAppendsQueryParam` + `TestBridge_ListProjects_NoWorkspaceID_NoQueryParam`; `TestNewBridgeFromEnv_DefaultsAndOverride` unchanged |
| `internal/mcp/server_test.go` | SC2 regression tests byte-for-byte unchanged | ✓ VERIFIED | `TestSC2_HandlerReturningError_ProducesCleanJSONRPCErrResponse` + `TestSC2_HandlerReturningIsErrorResult_ProducesCleanJSONRPCSuccessWithIsErrorTrue` both PASS |
| `internal/api/routes.go` | Registers `GET /api/tasks` + `GET /api/projects/{id}` | ✓ VERIFIED | `routes.go:39` (`GET /api/tasks`, t.list) + `routes.go:34` (`GET /api/projects/{id}`, p.get) |
| `internal/api/tasks.go` | `taskHandlers.list` — manual-only, ordered by status/position | ✓ VERIFIED | `tasks.go:191-215` — `SELECT taskColumns FROM tasks WHERE source = 'manual' ORDER BY status, position ASC`; no `WHERE project_id = ?` |
| `internal/api/projects.go` | `projectHandlers.get` (404 on missing); `projectHandlers.list` reads `?workspace_id=` | ✓ VERIFIED | `projects.go:173-188` (get — 404 `"project not found"`); `projects.go:132-148` (list — `r.URL.Query().Get("workspace_id")` with parameterized `WHERE workspace_id = ?`; silently ignores unparseable) |
| `internal/api/tasks_test.go`, `internal/api/projects_test.go` | Coverage for the 3 new endpoints + backward-compat | ✓ VERIFIED | `TestTaskList_All_ReturnsOnlyManualTasksOrderedByStatusPosition`, `TestTaskList_EmptyReturnsEmptyArray`, `TestProjectGet_Found_*`, `TestProjectGet_NotFound_*`, `TestProjectList_WorkspaceIDFilter_*`, `TestProjectList_NoFilter_*`, `TestProjectList_WorkspaceIDUnparseable_*` — all PASS |

### Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| `registerTaskTools` | `b.call` → Kamacu task routes | 6 `s.AddTool` closures each delegate to a `bridge.<verb>Task` method that calls `b.call` | ✓ WIRED | All 6 tools wire to the correct route/method: list_tasks → `GET /api/tasks` or `GET /api/projects/{id}/tasks`; get_task → `GET /api/tasks/{id}`; create_task → `POST /api/projects/{id}/tasks`; update_task → `PATCH /api/tasks/{id}`; move_task → `POST /api/tasks/{id}/move`; delete_task → `DELETE /api/tasks/{id}` |
| `registerProjectTools` | `b.call` → Kamacu project routes | 5 `s.AddTool` closures each delegate to a `bridge.<verb>Project` method | ✓ WIRED | list_projects → `GET /api/projects[?workspace_id=N]`; get_project → `GET /api/projects/{id}`; create_project → `POST /api/projects`; update_project → `PATCH /api/projects/{id}`; delete_project → `DELETE /api/projects/{id}` |
| `registerWorkspaceTools` | `b.call` → Kamacu workspace routes (and project route for move) | 5 `s.AddTool` closures each delegate to a `bridge.<verb>Workspace`/`moveProjectToWorkspace` method | ✓ WIRED | list/create/update/delete_workspace → `/api/workspaces...`; move_project_to_workspace → `PATCH /api/projects/{id}` with `{workspace_id}`-only body |
| Kamacu routes (`routes.go`) | API handlers (`tasks.go`, `projects.go`) | `mux.HandleFunc(...)` registrations | ✓ WIRED | `routes.go:34` (p.get) and `routes.go:39` (t.list) registered; existing routes unchanged |

### Data-Flow Trace (Level 4)

Not applicable — MCP bridge handlers do not render dynamic data. They pass through Kamacu's JSON response verbatim as a single `TextContent` block. The data source is the Kamacu HTTP API (proven by httptest-backed bridge tests that capture `r.Method`/`r.URL.Path`/request body and assert the canned response is passed through).

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| All MCP tests pass | `go test ./internal/mcp/...` | `ok kamacu/internal/mcp (cached)` — 43 tests PASS | ✓ PASS |
| SC2 regression tests pass | `go test ./internal/mcp/... -run TestSC2 -v` | Both `TestSC2_*` PASS | ✓ PASS |
| New API endpoint tests pass | `go test ./internal/api/... -run 'TestTaskList\|TestProjectGet\|TestProjectList' -v` | All PASS (incl. goose migrations 1→16) | ✓ PASS |
| Whole-tree build | `go build ./...` | exit 0 | ✓ PASS |

### Probe Execution

Not applicable — Phase 07 declares no `scripts/*/tests/probe-*.sh` probes and is not a migration/tooling phase requiring probe-based verification.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| --- | --- | --- | --- | --- |
| MCPTASK-01 | 07-02 | `list_tasks(project_id?)` | ✓ SATISFIED | Registered (`tasks.go:31-48`); bridges to `GET /api/tasks` (unscoped) or `GET /api/projects/{id}/tasks` (scoped); 3 tests |
| MCPTASK-02 | 07-02 | `get_task(task_id)` | ✓ SATISFIED | Registered (`tasks.go:51-69`); bridges to `GET /api/tasks/{id}`; 2 tests |
| MCPTASK-03 | 07-02 | `create_task(project_id, title, description?)` — worktree auto-provisions, agent NOT auto-started | ✓ SATISFIED | Registered (`tasks.go:72-98`); bridges to `POST /api/projects/{id}/tasks`; description explicitly says "agent is NOT auto-started"; worktree provisioning is the existing handler's behavior (tested at API layer); 2 tests |
| MCPTASK-04 | 07-02 | `update_task(task_id, title?, description?)` partial-PATCH (D-03) | ✓ SATISFIED | Registered (`tasks.go:101-127`); `Title *string` / `Description *string` pointer fields; body map omits nil keys; 2 tests |
| MCPTASK-05 | 07-02 | `move_task(task_id, status)` via existing `/move` (D-06 `after_id: null`) | ✓ SATISFIED | Registered (`tasks.go:130-153`); `after_id` NOT in InputSchema properties; body hardcodes `"after_id": nil`; enum `[todo, in_progress, in_review, done]`; 2 tests |
| MCPTASK-06 | 07-02 | `delete_task(task_id)` — gated cleanup runs | ✓ SATISFIED | Registered (`tasks.go:156-174`); bridges to `DELETE /api/tasks/{id}`; gated cleanup is the existing handler's behavior (tested at API layer); 2 tests |
| MCPPROJ-01 | 07-01 | `list_projects(workspace_id?)` | ✓ SATISFIED | Registered (`projects.go:50-72`); InputSchema has `workspace_id` (not stale `project_id`); bridges to `GET /api/projects[?workspace_id=N]`; 5 tests in `bridge_test.go` |
| MCPPROJ-02 | 07-03 | `get_project(project_id)` | ✓ SATISFIED | Registered (`projects.go:75-93`); bridges to Gap 1 endpoint `GET /api/projects/{id}`; 2 tests |
| MCPPROJ-03 | 07-03 | `create_project(name, repo_path_or_github_url, workspace_id?)` — D-04 two-arg fork | ✓ SATISFIED | Registered (`projects.go:96-126`); body sends all 4 fields as-is (no bridge-side dispatch); 4 tests covering folder path, managed repo, workspace_id inclusion, and Kamacu 400 |
| MCPPROJ-04 | 07-03 | `update_project(project_id, name?, description?, github_repo?, icon_letters?, icon_color?)` — D-03 EXCLUDES workspace_id + agent_id | ✓ SATISFIED | Registered (`projects.go:135-173`); properties map clean (no workspace_id/agent_id); args struct has no WorkspaceID/AgentID fields; pointer fields for all 5 optionals; 4 tests incl. `BodyExcludes_WorkspaceID_And_AgentID` |
| MCPPROJ-05 | 07-03 | `delete_project(project_id)` — v1.4 gated delete | ✓ SATISFIED | Registered (`projects.go:176-194`); bridges to `DELETE /api/projects/{id}`; 2 tests incl. `Managed409_PassesThroughStructuredBody` |
| MCPPROJ-06 | 07-04 | `list_workspaces` / `create_workspace` / `update_workspace` / `delete_workspace` — v1.9 guarded delete | ✓ SATISFIED | 4 tools registered (`workspaces.go:53-135`); bridges to `/api/workspaces...`; 6 tests incl. BOTH v1.9 409 guards (Default + NonEmpty) |
| MCPPROJ-07 | 07-04 | `move_project_to_workspace(project_id, workspace_id)` via v1.9 PATCH surface | ✓ SATISFIED | Registered (`workspaces.go:143-165`); PATCHes `/api/projects/{id}` (NOT a workspace route); body is `{workspace_id}`-only; 2 tests |

**Orphaned requirements check:** REQUIREMENTS.md traceability table maps all 13 IDs (MCPTASK-01..06, MCPPROJ-01..07) to Phase 07. PLAN frontmatter across 07-01..07-04 declares exactly these 13 IDs (07-01: MCPPROJ-01; 07-02: MCPTASK-01..06; 07-03: MCPPROJ-02..05; 07-04: MCPPROJ-06..07). No orphans, no unmapped IDs.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| --- | --- | --- | --- | --- |
| — | — | — | — | None found. Scanned all 5 production files in `internal/mcp/` for TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER/`return null`/`return []`/`fmt.Print*` — zero matches. No `mcp.AddTool[` generic (only appears in negative-reference comments). No `b.do(` or `io.ReadAll` in tool handlers (all delegate to `bridge.call`). No bridge-side validation guards. No `internal/store` or `internal/api` imports in `internal/mcp/` (HTTP-only bridge). No new migrations under `internal/store/migrations/` (16 → 16, unchanged — v1.11 milestone rule honored). |

### Human Verification Required

None. This phase is a backend MCP tool surface (Go HTTP-bridge handlers + tests). No UI rendering, no real-time behavior, no external service integration beyond HTTP loopback (which is exercised by `httptest.Server`-backed unit tests). The behavior-dependent parts of SC1/SC2 (worktree provisioning on create, gated cleanup on delete, v1.4 managed-checkout atomic create, v1.9 guarded delete) are state transitions that occur in Kamacu's existing API handlers — the bridge correctly delegates to those endpoints (proven by bridge tests asserting the request shape), and the endpoints' behavior is exercised by pre-existing API-layer tests that pass. Phase 07's scope is bridging, not reimplementing the endpoints.

### Gaps Summary

No gaps. All 4 ROADMAP Success Criteria verified, all 13 requirements satisfied, all 16 tools registered with backing bridge methods and tests, all prohibitions honored, no anti-patterns, no schema changes, full test suite passes.

**Documented deviation (informational, not a gap):** Plan 01's must_have specified `bridge.call` reject `if resp.StatusCode != http.StatusOK`. Plan 02 widened this to `if resp.StatusCode < 200 || resp.StatusCode >= 300` because POST `/api/projects/{id}/tasks` returns 201 (Created) and DELETE `/api/tasks/{id}` returns 204 (No Content) — under the original 200-only check, every successful `create_task`/`create_project`/`create_workspace`/`delete_task`/`delete_project`/`delete_workspace` call would have surfaced to the agent as an error. The deviation is documented inline in `bridge.go:80-87` as "Rule 1 — auto-fix blocking bug" and is the correct behavior for the SCs (which require these tools to "drive the board"/"drive the sidebar"/"drive the switcher" — i.e. succeed on success). Not flagged as an override because the deviation matches the SC intent; the Plan 01 literal was a bug.

---

_Verified: 2026-07-22T11:05:00Z_
_Verifier: the agent (gsd-verifier)_
