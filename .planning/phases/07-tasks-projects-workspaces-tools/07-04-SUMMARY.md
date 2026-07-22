---
phase: 07-tasks-projects-workspaces-tools
plan: 04
subsystem: api
tags: [mcp, http-bridge, kamacu-api, workspace-tools, partial-patch, guarded-delete, cross-route-transfer]

requires:
  - phase: 07-tasks-projects-workspaces-tools
    provides: "Plan 01's empty registerWorkspaceTools(s, b) shell; bridge.call(ctx, method, path, body) shared helper; bridge_test.go's newCallToolRequest helper + httptest pattern"
  - phase: 07-tasks-projects-workspaces-tools
    provides: "Plan 02 widened bridge.call from HTTP 200-only to the full 2xx range — necessary so this plan's create_workspace (POST → 201) and delete_workspace (DELETE → 204) surface clean results instead of errors"
provides:
  - "Five workspace MCP tools registered in registerWorkspaceTools: list_workspaces (MCPPROJ-06), create_workspace (MCPPROJ-06), update_workspace (MCPPROJ-06, D-03 single-field rename), delete_workspace (MCPPROJ-06, v1.9 guarded delete), move_project_to_workspace (MCPPROJ-07, cross-route transfer)"
  - "Five *bridge methods in internal/mcp/workspaces.go, each a thin build-path-and-delegate to bridge.call"
  - "internal/mcp/workspaces_test.go with 10 test cases (one happy + representative error per tool, per D-08) — D-03 nil-pointer body assertion; v1.9 BOTH guarded-delete 409 cases asserted (Gap 2 substring); move_project_to_workspace cross-route PATCH asserted (body is {workspace_id}-only, NOT a /api/workspaces route)"
  - "Combined with Plans 01/02/03, all 13 Phase 07 MCP tools shipped (1 list_projects + 6 task + 5 project + 5 workspace = 17 actually — Plan 03 summary miscounted; the canonical Phase 07 total is 1 + 6 + 4 + 5 = 16 tools across the 3 resource files, with list_projects in projects.go + 4 project tools in projects.go + 6 task tools + 5 workspace tools)"
affects: []  # last plan in Phase 07

tech-stack:
  added: []
  patterns:
    - "D-03 single-field rename pattern: update_workspace uses *string pointer for the lone optional name field — nil = omitted (Kamacu 400 'nothing to update'), non-nil = supplied (rename). Body map built conditionally so omitted keys never reach Kamacu."
    - "Cross-route transfer pattern: move_project_to_workspace is a workspace-resource tool registered in registerWorkspaceTools even though it PATCHes the project route (/api/projects/{id}). The body contains ONLY workspace_id — update_project (Plan 03) excludes workspace_id from its InputSchema specifically because this dedicated tool owns the transfer. This is the 07-CONTEXT D-03 / 07-RESEARCH Pitfall 3 double-lock from the other side."
    - "Gap 2 / Pitfall 2 error-body assertions extended to BOTH v1.9 guarded-delete 409 messages: 'the default workspace can't be deleted' (is_default guard) and 'move or remove its N project(s) first' (non-empty COUNT guard) — both asserted as Kamacu message substrings inside the wrapped error, NOT as JSON key names."

key-files:
  created:
    - internal/mcp/workspaces_test.go
  modified:
    - internal/mcp/workspaces.go

key-decisions:
  - "registerWorkspaceTools owns move_project_to_workspace even though the handler PATCHes /api/projects/{id} (NOT a /api/workspaces route). The tool is a workspace-management action (transferring a project INTO a workspace); D-07 per-resource ownership tracks the resource being acted on, not the route being called. The InputSchema exposes {project_id, workspace_id}; the body contains ONLY workspace_id (D-03 excludes workspace_id from update_project specifically because this tool owns the transfer)."
  - "update_workspace uses *string pointer for the lone Name field (D-03 single-field rename). The InputSchema declares name as optional (NOT in required — only workspace_id is required). When name is omitted, the body is {} and Kamacu's PATCH handler returns 400 'nothing to update' verbatim through the D-05 wrap. When name is supplied (incl. explicit ''), the rename triggers. The bridge performs NO validation — Kamacu's existing gates (empty after trim → 400 'name is required'; case-insensitive dup excluding self → 409; default workspace IS renamable per D-04) apply unchanged."
  - "delete_workspace is a one-liner delegating to DELETE /api/workspaces/{id}. The v1.9 two-guard delete (is_default → 409 'the default workspace can't be deleted'; non-empty COUNT → 409 'move or remove its N project(s) first') runs unchanged through the bridge. The tool surfaces the same gates the browser-attached user hits — no privilege escalation (T-07-12 accept)."
  - "Test error assertions match on HTTP status substrings ('HTTP 4NN') AND Kamacu message substrings ('the default workspace', 'project(s) first', 'workspace not found') per 07-RESEARCH Gap 2 / Pitfall 2 — never on JSON key names. The D-05 wrap passes the body as a raw string, so the structured {error: ...} shape survives in the error text; assertions match on the human-readable message text, NOT on the 'message' JSON key name."
  - "Plan's literal grep acceptance criteria (e.g. `grep -q 'Name: \"<tool>\"'`) was satisfied via the regex form `grep -qE 'Name:[[:space:]]+\"<tool>\"'` — same approach Plans 02 and 03 took and documented. gofmt aligns consecutive struct field assignments and Tool field assignments with whitespace; the plan's literal pattern doesn't match the gofmt-aligned reality. Semantic intent (the tool name exists as a `Name:` value) is preserved verbatim. NOT a deviation — verification-pattern adjustment matching established Plan 02/03 practice."

patterns-established:
  - "Pattern: cross-route resource ownership — an MCP tool registered in registerXTools may legitimately call a different resource's route when the action is conceptually owned by the first resource (move_project_to_workspace registers in workspaces.go but PATCHes /api/projects/{id}). The D-07 split tracks the resource being acted on, not the route."
  - "Pattern: dedicated transfer tool paired with InputSchema exclusion — when Kamacu's PATCH handler accepts a field that crosses resource boundaries (workspace_id on PATCH /api/projects/{id}), the bridge exposes that field ONLY through the dedicated transfer tool, never through update_project. update_project's args struct has NO WorkspaceID field (Plan 03 / Pitfall 3 double-lock); move_project_to_workspace owns it."
  - "Pattern: both branches of a multi-branch guard must have dedicated substring-assertion tests (Gap 2). The v1.9 workspace delete has two distinct 409 messages (is_default, non-empty COUNT) — Tests TestBridge_DeleteWorkspace_DefaultGuard_Kamacu409 AND TestBridge_DeleteWorkspace_NonEmptyGuard_Kamacu409 cover them independently, asserting on the Kamacu message substring of each."

requirements-completed: [MCPPROJ-06, MCPPROJ-07]

coverage:
  - id: D1
    description: "MCPPROJ-06 list_workspaces: registered with InputSchema {} (no args) — handler bridges to GET /api/workspaces (Kamacu returns [] not nil)"
    requirement: MCPPROJ-06
    verification:
      - kind: integration
        ref: "grep -qE 'Name:[[:space:]]+\"list_workspaces\"' internal/mcp/workspaces.go AND InputSchema properties is empty map"
        status: pass
      - kind: unit
        ref: "internal/mcp/workspaces_test.go#TestBridge_ListWorkspaces_Happy_CallsGetPath — PASS (no args → GET /api/workspaces, [] passthrough in TextContent)"
        status: pass
    human_judgment: false
  - id: D2
    description: "MCPPROJ-06 create_workspace: registered with required [name] — handler bridges to POST /api/workspaces with {name} body; Kamacu's empty-name 400 and dup 409 surface verbatim"
    requirement: MCPPROJ-06
    verification:
      - kind: integration
        ref: "grep -qE 'Name:[[:space:]]+\"create_workspace\"' internal/mcp/workspaces.go AND required: [name]"
        status: pass
      - kind: unit
        ref: "internal/mcp/workspaces_test.go#TestBridge_CreateWorkspace_Happy_SendsNameBody — PASS ({name:Research} → POST /api/workspaces, body decodes to {name:Research})"
        status: pass
      - kind: unit
        ref: "internal/mcp/workspaces_test.go#TestBridge_CreateWorkspace_DuplicateName_Kamacu409 — PASS (HTTP 409 substring; Gap 2 substring assertion)"
        status: pass
    human_judgment: false
  - id: D3
    description: "MCPPROJ-06 update_workspace: registered with required [workspace_id] (D-03 single-field rename — name optional via *string pointer). Handler PATCHes only when name supplied; nil pointer → body {} → Kamacu 400 'nothing to update'"
    requirement: MCPPROJ-06
    verification:
      - kind: integration
        ref: "grep -qE 'Name \*string' internal/mcp/workspaces.go (D-03 pointer field for partial-PATCH)"
        status: pass
      - kind: integration
        ref: "grep -A6 'func (b \\*bridge) updateWorkspace' internal/mcp/workspaces.go shows body := map[string]any{} with conditional name inclusion; NO value-guard branches"
        status: pass
      - kind: unit
        ref: "internal/mcp/workspaces_test.go#TestBridge_UpdateWorkspace_Happy_SendsNameBody — PASS ({workspace_id:2,name:Renamed} → PATCH /api/workspaces/2, body {name:Renamed})"
        status: pass
      - kind: unit
        ref: "internal/mcp/workspaces_test.go#TestBridge_UpdateWorkspace_NoName_EmptyBody_Kamacu400 — PASS (name omitted → body has NO name key; canned 400 'nothing to update' surfaces as HTTP 400 error)"
        status: pass
    human_judgment: false
  - id: D4
    description: "MCPPROJ-06 delete_workspace: registered with required [workspace_id] — handler bridges to DELETE /api/workspaces/{id}; v1.9 two-guard delete (is_default → 409; non-empty COUNT → 409) runs unchanged"
    requirement: MCPPROJ-06
    verification:
      - kind: integration
        ref: "grep -qE 'Name:[[:space:]]+\"delete_workspace\"' internal/mcp/workspaces.go AND handler is a single b.call(DELETE, /api/workspaces/{id}, nil)"
        status: pass
      - kind: unit
        ref: "internal/mcp/workspaces_test.go#TestBridge_DeleteWorkspace_Happy_CallsDeletePath — PASS ({workspace_id:3} → DELETE /api/workspaces/3, 204 clean result)"
        status: pass
      - kind: unit
        ref: "internal/mcp/workspaces_test.go#TestBridge_DeleteWorkspace_DefaultGuard_Kamacu409 — PASS (HTTP 409 + 'the default workspace' substring — Gap 2)"
        status: pass
      - kind: unit
        ref: "internal/mcp/workspaces_test.go#TestBridge_DeleteWorkspace_NonEmptyGuard_Kamacu409 — PASS (HTTP 409 + 'project(s) first' substring — Gap 2)"
        status: pass
    human_judgment: false
  - id: D5
    description: "MCPPROJ-07 move_project_to_workspace: registered with required [project_id, workspace_id] — handler bridges to PATCH /api/projects/{id} (NOT a /api/workspaces route) with {workspace_id}-only body. Reuses the v1.9 transfer surface; Kamacu's target validation (workspace exists → 400 'workspace not found' if not) applies unchanged"
    requirement: MCPPROJ-07
    verification:
      - kind: integration
        ref: "grep -qE 'Name:[[:space:]]+\"move_project_to_workspace\"' internal/mcp/workspaces.go (registered in workspaces.go, NOT projects.go)"
        status: pass
      - kind: integration
        ref: "grep -A16 'func (b \\*bridge) moveProjectToWorkspace' internal/mcp/workspaces.go shows http.MethodPatch + /api/projects/{id} + body containing ONLY workspace_id"
        status: pass
      - kind: unit
        ref: "internal/mcp/workspaces_test.go#TestBridge_MoveProjectToWorkspace_Happy_PATCHesProjectRoute — PASS ({project_id:5,workspace_id:2} → PATCH /api/projects/5, body {workspace_id:2} with NO name/description/github_repo/etc.)"
        status: pass
      - kind: unit
        ref: "internal/mcp/workspaces_test.go#TestBridge_MoveProjectToWorkspace_TargetMissing_Kamacu400 — PASS (HTTP 400 + 'workspace not found' substring — Gap 2)"
        status: pass
    human_judgment: false
  - id: D6
    description: "Pattern fidelity: every workspace tool handler delegates to bridge.call — NO handler re-implements b.do + io.ReadAll + non-200 wrap inline; NO typed mcp.AddTool[In,Out] generic used (Phase 06 lock preserved)"
    requirement: MCPPROJ-06
    verification:
      - kind: integration
        ref: "! grep -q 'io.ReadAll' internal/mcp/workspaces.go (no inline response reading)"
        status: pass
      - kind: integration
        ref: "! grep -q 'b\\.do(' internal/mcp/workspaces.go (no direct b.do calls — all via b.call)"
        status: pass
      - kind: integration
        ref: "! grep -q 'mcp.AddTool\\[' internal/mcp/workspaces.go (no typed generic)"
        status: pass
    human_judgment: false
  - id: D7
    description: "No bridge-side validation: workspaces.go performs no validation of workspace_id values, name emptiness, or target-workspace existence — Kamacu's existing handlers validate (SC4 satisfied by NOT duplicating validation in the bridge)"
    requirement: MCPPROJ-06
    verification:
      - kind: integration
        ref: "! grep -qE 'if args\\.[A-Za-z]+ == \"\"' internal/mcp/workspaces.go (no value guards)"
        status: pass
      - kind: unit
        ref: "TestBridge_CreateWorkspace_DuplicateName_Kamacu409 proves Kamacu rejects dup names with 409 (not the bridge)"
        status: pass
      - kind: unit
        ref: "TestBridge_DeleteWorkspace_DefaultGuard_Kamacu409 + TestBridge_DeleteWorkspace_NonEmptyGuard_Kamacu409 prove Kamacu rejects the two delete-guard cases with 409 (not the bridge)"
        status: pass
      - kind: unit
        ref: "TestBridge_MoveProjectToWorkspace_TargetMissing_Kamacu400 proves Kamacu validates target existence (not the bridge)"
        status: pass
    human_judgment: false
  - id: D8
    description: "SC3 satisfied: the four workspace tools + the transfer tool drive the switcher (workspace CRUD with v1.9 guarded delete + project transfer across workspaces). Combined with Plans 01/02/03, all Phase 07 requirements delivered."
    requirement: MCPPROJ-06
    verification:
      - kind: integration
        ref: "grep -cE 'Name:[[:space:]]+\"' internal/mcp/workspaces.go returns 5 (list/create/update/delete + move_project_to_workspace)"
        status: pass
      - kind: integration
        ref: "go test ./internal/mcp/... — 43 tests PASS (8 carried-over Plan 01/Phase 06 + 13 Plan 02 task + 12 Plan 03 project + 10 Plan 04 workspace)"
        status: pass
    human_judgment: false
  - id: D9
    description: "go vet ./internal/mcp/... + go test ./internal/mcp/... + go build ./... all clean"
    requirement: MCPPROJ-06
    verification:
      - kind: integration
        ref: "go vet ./internal/mcp/... — clean"
        status: pass
      - kind: integration
        ref: "go test ./internal/mcp/... — 43 tests PASS"
        status: pass
      - kind: integration
        ref: "go build ./... — clean (whole codebase compiles)"
        status: pass
    human_judgment: false

duration: 7min
completed: 2026-07-22
status: complete
---

# Phase 07 Plan 04: Five Workspace MCP Tools Summary

**Five workspace MCP tools (list_workspaces / create_workspace / update_workspace / delete_workspace / move_project_to_workspace) bridging to Kamacu's workspace endpoints plus the v1.9 PATCH /api/projects/{id} transfer surface — each a thin build-path-and-delegate to bridge.call, with 10 httptest cases covering both v1.9 guarded-delete 409 messages and the cross-route transfer's {workspace_id}-only body.**

## Performance

- **Duration:** ~7 min
- **Started:** 2026-07-22T08:41:34Z
- **Completed:** 2026-07-22T08:49:14Z
- **Tasks:** 2
- **Files modified:** 2 (1 modified + 1 new)

## Accomplishments
- Filled the empty `registerWorkspaceTools(s, b)` shell from Plan 01 with five `s.AddTool` calls and five corresponding `*bridge` methods in `internal/mcp/workspaces.go`. Every handler follows the canonical shape Plans 02/03 established: typed args struct → `json.Unmarshal` with error wrap → conditional body map (PATCH only) → marshal → `b.call(ctx, method, path, body)`. No handler re-implements the response-handling half. MCPPROJ-06 (workspace CRUD) and MCPPROJ-07 (move_project_to_workspace) both delivered; SC3 (the four workspace tools + transfer tool drive the switcher, including the v1.9 guarded delete) satisfied.
- `list_workspaces` (MCPPROJ-06) bridges to `GET /api/workspaces` with no args and no body — Kamacu returns the workspaces array (never nil; empty array when no workspaces exist). The handler is a one-line delegate to `b.call`.
- `create_workspace` (MCPPROJ-06) bridges to `POST /api/workspaces` with `{name}` body. Kamacu's empty-name 400 ("name is required") and case-insensitive dup 409 ("a workspace with that name already exists") surface verbatim through the D-05 wrap.
- `update_workspace` (MCPPROJ-06, D-03 single-field rename) bridges to `PATCH /api/workspaces/{id}` with `{name}` when supplied. The Name field is a `*string` pointer (D-03 — nil = omitted → Kamacu returns 400 "nothing to update"; non-nil = supplied → triggers rename). The default workspace IS renamable per D-04; a case-insensitive dup excluding self → 409.
- `delete_workspace` (MCPPROJ-06) is a one-liner delegating to `DELETE /api/workspaces/{id}`. The v1.9 two-guard delete (is_default → 409 "the default workspace can't be deleted"; non-empty COUNT → 409 "move or remove its N project(s) first") runs unchanged through the bridge. WSMGMT-04 (at least one workspace always exists) is preserved — the agent cannot bypass the is_default guard (T-07-12 accept).
- `move_project_to_workspace` (MCPPROJ-07) PATCHes `/api/projects/{id}` (NOT a `/api/workspaces` route) with a `{workspace_id}`-only body. Registered in `registerWorkspaceTools` (not `registerProjectTools`) because it is a workspace-management action (transferring a project INTO a workspace). The body contains ONLY `workspace_id` — D-03 excludes workspace_id from `update_project` (Plan 03) specifically because this dedicated tool owns the transfer. Kamacu's PATCH WorkspaceID branch validates target existence via `SELECT 1 FROM workspaces WHERE id = ?` → 400 "workspace not found" if not (T-07-13 accept).
- Created `internal/mcp/workspaces_test.go` with **10 test cases** (one happy + representative error per tool per D-08). Each test stands up an `httptest.Server` capturing method/path/body, constructs a `*mcp.CallToolRequest` with canned `Params.Arguments`, invokes the bridge method directly, and asserts the captured request + result TextContent / error substring. Error assertions match on `"HTTP 4NN"` substrings AND Kamacu message substrings per Gap 2 / Pitfall 2 — never on JSON key names.
- **Both v1.9 guarded-delete 409 cases covered independently:** `TestBridge_DeleteWorkspace_DefaultGuard_Kamacu409` (is_default guard) and `TestBridge_DeleteWorkspace_NonEmptyGuard_Kamacu409` (non-empty COUNT guard) — each asserts the unique Kamacu message substring (`"the default workspace"`, `"project(s) first"`) inside the wrapped error. The raw body survives in the D-05 wrap, so the message text is reachable.
- **Cross-route transfer coverage:** `TestBridge_MoveProjectToWorkspace_Happy_PATCHesProjectRoute` asserts the captured path is `/api/projects/5` (NOT `/api/workspaces/...`), the method is PATCH, AND the body decodes to `{"workspace_id":2}` with NO other keys (no name, no description, no github_repo, no icon_*, no agent_id, no repo, no repo_path). The dedicated transfer tool owns the workspace_id field; update_project excludes it (Plan 03 double-lock).

## Task Commits

Each task was committed atomically:

1. **Task 1: Implement the five workspace tool handlers in registerWorkspaceTools (list_workspaces, create_workspace, update_workspace, delete_workspace, move_project_to_workspace)** — `137fc6b` (feat)
2. **Task 2: Add workspaces_test.go with 10 happy/error test cases per D-08 (including both v1.9 guarded-delete 409 cases)** — `c6ab92f` (test)

**Plan metadata:** this commit (docs: complete workspaces-tools plan)

## Files Created/Modified
- `internal/mcp/workspaces.go` — `registerWorkspaceTools` body filled from the empty shell with five `s.AddTool` calls (each `&mcp.Tool{Name, Description, InputSchema: map[string]any{...}}` + a delegating closure to the corresponding `b.<verb>Workspace` / `b.moveProjectToWorkspace` method). Five new `*bridge` methods: `listWorkspaces` (one-liner GET, no args), `createWorkspace` (marshals `{name}`), `updateWorkspace` (D-03 `*string` pointer field + conditional body map), `deleteWorkspace` (one-liner DELETE), `moveProjectToWorkspace` (cross-route — PATCHes `/api/projects/{id}` with `{workspace_id}`-only body).
- `internal/mcp/workspaces_test.go` (NEW) — 10 test cases mirroring Plan 02/03's `tasks_test.go` / `projects_test.go` pattern: `TestBridge_ListWorkspaces_Happy_CallsGetPath`, `TestBridge_CreateWorkspace_{Happy_SendsNameBody,DuplicateName_Kamacu409}`, `TestBridge_UpdateWorkspace_{Happy_SendsNameBody,NoName_EmptyBody_Kamacu400}`, `TestBridge_DeleteWorkspace_{Happy_CallsDeletePath,DefaultGuard_Kamacu409,NonEmptyGuard_Kamacu409}`, `TestBridge_MoveProjectToWorkspace_{Happy_PATCHesProjectRoute,TargetMissing_Kamacu400}`.

## Decisions Made
- **`registerWorkspaceTools` owns `move_project_to_workspace`** even though the handler PATCHes `/api/projects/{id}` (NOT a `/api/workspaces` route). The tool is a workspace-management action (transferring a project INTO a workspace); D-07 per-resource ownership tracks the resource being acted on, not the route being called. The InputSchema exposes `{project_id, workspace_id}`; the body contains ONLY `workspace_id` (D-03 excludes `workspace_id` from `update_project` specifically because this tool owns the transfer).
- **`update_workspace` uses `*string` pointer for the lone Name field** (D-03 single-field rename). The InputSchema declares `name` as optional (NOT in `required` — only `workspace_id` is required). When `name` is omitted, the body is `{}` and Kamacu's PATCH handler returns 400 "nothing to update" verbatim through the D-05 wrap. When `name` is supplied (incl. explicit `""`), the rename triggers. The bridge performs NO validation — Kamacu's existing gates (empty after trim → 400 "name is required"; case-insensitive dup excluding self → 409; default workspace IS renamable per D-04) apply unchanged.
- **`delete_workspace` is a one-liner delegating to `DELETE /api/workspaces/{id}`**. The v1.9 two-guard delete (`is_default` → 409 "the default workspace can't be deleted"; non-empty COUNT → 409 "move or remove its N project(s) first") runs unchanged through the bridge. The tool surfaces the same gates the browser-attached user hits — no privilege escalation (T-07-12 accept; WSMGMT-04 preserved).
- **Test error assertions match on HTTP status substrings** (`"HTTP 4NN"`) AND Kamacu message substrings (`"the default workspace"`, `"project(s) first"`, `"workspace not found"`) per 07-RESEARCH Gap 2 / Pitfall 2 — never on JSON key names. The real Kamacu error shape is `{"error":"..."}`; matching on `message`/`status` keys would be brittle. The raw body survives in the D-05 wrap, so the human-readable message text is reachable.
- **Plan's literal grep acceptance criteria** (e.g. `grep -q 'Name: "<tool>"'`) was satisfied via the regex form `grep -qE 'Name:[[:space:]]+"<tool>"'` — same approach Plans 02 and 03 took and documented in their SUMMARYs. gofmt aligns consecutive Tool struct field assignments with whitespace; the plan's literal pattern doesn't match the gofmt-aligned reality. Semantic intent (the tool name exists as a `Name:` value) is preserved verbatim. NOT a deviation — verification-pattern adjustment matching established Plan 02/03 practice.

## Deviations from Plan

None - plan executed exactly as written. The only adjustment was the regex form of the grep acceptance criteria (see Decisions Made); semantic intent preserved verbatim, matching Plans 02/03's documented practice. The plan's prohibitions (no touching of `internal/api/`, `internal/mcp/server.go`, `internal/mcp/bridge.go`, `internal/mcp/tasks.go`, or `internal/mcp/projects.go`; no typed `mcp.AddTool[In,Out]` generic; no bridge-side validation; no JSON key name assertions on error bodies; `move_project_to_workspace` PATCHes `/api/projects/{id}` not a `/api/workspaces` route) are all respected. `git diff --name-only HEAD~2 HEAD` for this plan shows exactly two files: `internal/mcp/workspaces.go` and `internal/mcp/workspaces_test.go`.

## Issues Encountered
None. The `go vet ./internal/mcp/...`, `go test ./internal/mcp/...` (43 tests PASS — 10 new workspace + 12 Plan 03 project + 13 Plan 02 task + 8 carried-over Plan 01 / Phase 06), and `go build ./...` are all clean. Plan 02's Rule 1 fix widening `bridge.call` to the full 2xx range is what made this plan's `create_workspace` (POST → 201) and `delete_workspace` (DELETE → 204) test cases pass without further bridge work.

## User Setup Required
None — this plan adds five MCP tools and tests; no external service configuration is required.

## Next Phase Readiness
- **Phase 07 complete.** All four plans delivered: Plan 01 (scaffold + 3 Kamacu endpoint additions + `list_projects(workspace_id?)`), Plan 02 (6 task tools), Plan 03 (4 project tools), Plan 04 (5 workspace tools). Combined Phase 07 tool surface: 1 list_projects + 6 task + 4 project + 5 workspace = **16 MCP tools** across the 3 resource files (`internal/mcp/tasks.go`, `internal/mcp/projects.go`, `internal/mcp/workspaces.go`), each a thin build-path-and-delegate to the shared `bridge.call` helper.
- **MCPPROJ-06 + MCPPROJ-07 delivered HERE.** Combined with Plans 01/02/03, all Phase 07 requirements (MCPPROJ-01..07 + MCPTASK-01..06) are now live.
- **SC3 + SC4 satisfied.** SC3: the four workspace tools + transfer tool drive the switcher, including the v1.9 guarded delete keyed off `is_default` + non-empty `COUNT`. SC4: every existing Kamacu validation gate (empty name → 400; case-insensitive dup → 409; is_default delete → 409; non-empty delete → 409; missing workspace → 404/400; move target missing → 400) applies unchanged through the bridge — the consistent "send fields as-is, let Kamacu validate" design across all 16 tools satisfies SC4 by NOT duplicating validation in the bridge.
- **Phase 07 ready for verification.** Next step is `/gsd-verify-work 07` (consolidated UAT) or, if the milestone is complete, `/gsd-complete-milestone`.

---
*Phase: 07-tasks-projects-workspaces-tools*
*Completed: 2026-07-22*

## Self-Check: PASSED

- Created files exist on disk: `internal/mcp/workspaces_test.go`, `.planning/phases/07-tasks-projects-workspaces-tools/07-04-SUMMARY.md` — all FOUND.
- Modified files exist on disk: `internal/mcp/workspaces.go` — FOUND.
- Task commits exist in git log: `137fc6b` (Task 1, feat) and `c6ab92f` (Task 2, test) — both FOUND.
- Re-ran `go test ./internal/mcp/...` post-SUMMARY: package `ok` (43 tests PASS — 10 new workspace + 12 Plan 03 project + 13 Plan 02 task + 8 carried-over Plan 01 / Phase 06).
- Plan-level verification gate (9 coverage entries) all PASS: 5 AddTool calls in registerWorkspaceTools (list/create/update/delete + move_project_to_workspace); 5 bridge methods delegating to b.call; move_project_to_workspace PATCHes /api/projects/{id} with {workspace_id}-only body; update_workspace uses *string Name field (D-03); both v1.9 guarded-delete 409 cases covered with substring assertions (Gap 2); no JSON-key error assertions; no typed mcp.AddTool generic; no bridge-side validation; no io.ReadAll / b.do inline; go vet + go test + go build clean.
