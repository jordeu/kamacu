---
phase: 07-tasks-projects-workspaces-tools
plan: 03
subsystem: api
tags: [mcp, http-bridge, kamacu-api, project-tools, partial-patch, managed-checkout, gated-delete]

requires:
  - phase: 07-tasks-projects-workspaces-tools
    provides: "Plan 01's registerProjectTools(s, b) with list_projects(workspace_id?) shipped; bridge.call(ctx, method, path, body) shared helper; GET /api/projects/{id} Gap 1 endpoint registered; bridge_test.go's newCallToolRequest helper + httptest pattern"
  - phase: 07-tasks-projects-workspaces-tools
    provides: "Plan 02 widened bridge.call from HTTP 200-only to the full 2xx range — necessary so this plan's create_project (POST → 201) and delete_project (DELETE → 204) surface clean results instead of errors"
provides:
  - "Four project MCP tools registered in registerProjectTools: get_project (MCPPROJ-02), create_project (MCPPROJ-03, D-04 two-arg fork), update_project (MCPPROJ-04, D-03 partial-PATCH), delete_project (MCPPROJ-05)"
  - "Four *bridge methods in internal/mcp/projects.go, each a thin build-path-and-delegate to bridge.call"
  - "internal/mcp/projects_test.go with 12 test cases (one happy + representative error per tool, per D-08) — D-03 excluded-fields asserted; D-04 all-fields-as-is asserted; Gap 2 managed-delete 409 structured body asserted"
  - "registerProjectTools now registers 5 tools total (list_projects from Plan 01 + the four new ones) — SC2 satisfied"
affects: [07-04-workspace-tools]

tech-stack:
  added: []
  patterns:
    - "D-04 two-arg fork pattern: when Kamacu's underlying handler dispatches on a single field (e.g. `strings.TrimSpace(req.Repo) != \"\"`), the bridge sends ALL fields as-is with ZERO type detection. The dispatch stays in Kamacu — the bridge is a thin shape-preserving translator."
    - "D-03 partial-PATCH with EXCLUDED fields: when a Kamacu PATCH handler accepts more fields than the MCP tool should expose (workspace_id, agent_id), the args struct declares ONLY the exposed fields. json.Unmarshal silently drops unknown properties (MCP spec), AND the body-map construction only iterates declared fields — a double lock ensuring excluded fields never reach Kamacu even if an agent supplies them."
    - "Explicit empty-string vs omitted distinction preserved via *T pointer fields: `*args.GithubRepo` non-nil AND empty = unlink (Kamacu stores NULL); `*args.GithubRepo` nil = leave untouched. Body map construction: `if args.GithubRepo != nil { body[\"github_repo\"] = *args.GithubRepo }`."
    - "Gap 2 / Pitfall 2 error-body assertions: test error assertions match on HTTP status substrings (\"HTTP 4NN\"/\"HTTP 5NN\") and Kamacu message substrings (\"can't be deleted\"), NEVER on JSON key names. The D-05 wrap passes the body as a raw string, so the structured {error, reasons} shape of the managed-delete 409 survives in the error text."

key-files:
  created:
    - internal/mcp/projects_test.go
  modified:
    - internal/mcp/projects.go

key-decisions:
  - "create_project body marshals {name, repo_path, repo} unconditionally and workspace_id only when non-nil (D-04). The alternative (conditional repo/repo_path inclusion) would require bridge-side type detection — explicitly prohibited. Kamacu's `strings.TrimSpace(req.Repo) != \"\"` check is the canonical dispatch; the bridge sends every field as-is."
  - "create_project does NOT expose agent_id in its InputSchema or args struct even though Kamacu's create handler accepts it. New projects land on the global default agent (Claude seed on a fresh install). Agent assignment at create-time is deferred to a future tool (MCPMORE-01 Out of Scope)."
  - "update_project args struct has NO WorkspaceID and NO AgentID fields (D-03 / 07-RESEARCH Pitfall 3). Workspace transfer has its own tool in a later plan; agent reassignment is Out of Scope. The double-lock (struct declaration + body-map construction) ensures excluded fields never reach Kamacu even if an agent supplies them — verified by TestBridge_UpdateProject_BodyExcludes_WorkspaceID_And_AgentID."
  - "update_project uses *string pointer fields for ALL five optional args (Name, Description, GithubRepo, IconLetters, IconColor) — nil = omitted vs \"\" = explicit value. Critical for github_repo where empty = unlink (Kamacu stores NULL) vs omitted = leave untouched."
  - "delete_project description in the InputSchema enumerates the v1.4 gated-removal behavior verbatim (folder: hard delete, dir untouched; managed: gate → 409 with reasons / REMOVE on all-clear). The tool surfaces the same gate the browser-attached user hits — no privilege escalation (T-07-09 accept)."
  - "No deviation: the plan's literal grep acceptance criteria (e.g. `grep -q 'Name: \"<tool>\"'`) was satisfied via the regex form `grep -qE 'Name:[[:space:]]+\"<tool>\"'` — same approach Plan 02 took. gofmt aligns consecutive struct field assignments with whitespace; the plan's literal pattern doesn't match the gofmt-aligned reality. Semantic intent (the tool name exists as a Name: value) is preserved."
  - "Plan's acceptance criterion `grep -c 'Name: \"' internal/mcp/projects.go returns 5` adjusted to `grep -cE 'Name:[[:space:]]+\"'` for the same gofmt-alignment reason. 5 tools registered verbatim."

patterns-established:
  - "Pattern: D-04 two-arg fork — when Kamacu's underlying handler dispatches on a field, the bridge sends all fields as-is with zero dispatch logic. The dispatch stays in Kamacu; the bridge is shape-preserving."
  - "Pattern: D-03 double-lock on excluded fields — args struct declaration (json.Unmarshal drops unknowns) + body-map construction (only declared fields iterated). Both must hold; an agent cannot smuggle excluded fields through."
  - "Pattern: pointer-field partial-PATCH extended beyond strings — *string for all five update_project optional args. Nil = omit (Kamacu leaves untouched); non-nil = supplied (including \"\"). Body map built conditionally so omitted keys never reach Kamacu."
  - "Pattern: error-body assertion via status substring + Kamacu message substring (Gap 2 / Pitfall 2). The managed-delete 409 structured body is asserted via \"can't be deleted\" + \"reasons\" substrings — never JSON key names."

requirements-completed: [MCPPROJ-02, MCPPROJ-03, MCPPROJ-04, MCPPROJ-05]

coverage:
  - id: D1
    description: "MCPPROJ-02: get_project registered with required [project_id] — handler bridges to GET /api/projects/{id} (Gap 1 endpoint from Plan 01); Kamacu's 404 surfaces verbatim"
    requirement: MCPPROJ-02
    verification:
      - kind: integration
        ref: "grep -qE 'Name:[[:space:]]+\"get_project\"' internal/mcp/projects.go AND required: [project_id]"
        status: pass
      - kind: unit
        ref: "internal/mcp/projects_test.go#TestBridge_GetProject_Happy_CallsCorrectPath — PASS ({project_id:3} → GET /api/projects/3, TextContent passthrough)"
        status: pass
      - kind: unit
        ref: "internal/mcp/projects_test.go#TestBridge_GetProject_NotFound404_ReturnsError — PASS (HTTP 404 substring in error)"
        status: pass
    human_judgment: false
  - id: D2
    description: "MCPPROJ-03: create_project registered with required [name], D-04 two-arg fork (repo_path? folder / repo? managed / workspace_id?) — handler bridges to POST /api/projects with all fields as-is; Kamacu's create handler dispatches on repo != \"\""
    requirement: MCPPROJ-03
    verification:
      - kind: integration
        ref: "grep -qE 'Name:[[:space:]]+\"create_project\"' internal/mcp/projects.go AND properties block lists name, repo_path, repo, workspace_id (NO agent_id)"
        status: pass
      - kind: integration
        ref: "grep -A20 'func (b \\*bridge) createProject' internal/mcp/projects.go shows body map with name/repo_path/repo unconditionally + workspace_id when non-nil; NO `if args.Repo != \"\"` branch (D-04 zero bridge-side dispatch)"
        status: pass
      - kind: unit
        ref: "internal/mcp/projects_test.go#TestBridge_CreateProject_FolderPath_SendsAllFieldsAsIs — PASS ({name:foo, repo_path:/abs/path} → POST /api/projects, body has name/repo_path/repo keys; workspace_id ABSENT when omitted)"
        status: pass
      - kind: unit
        ref: "internal/mcp/projects_test.go#TestBridge_CreateProject_ManagedRepo_SendsAllFieldsAsIs — PASS ({name:foo, repo:owner/name} → body has repo=owner/name; repo_path present as empty string)"
        status: pass
      - kind: unit
        ref: "internal/mcp/projects_test.go#TestBridge_CreateProject_WorkspaceIDIncluded_WhenSupplied — PASS ({workspace_id:2} → body has workspace_id: 2)"
        status: pass
      - kind: unit
        ref: "internal/mcp/projects_test.go#TestBridge_CreateProject_Kamacu400_InvalidPath_ReturnsError — PASS (HTTP 400 substring; bridge performs NO path validation)"
        status: pass
    human_judgment: false
  - id: D3
    description: "MCPPROJ-04: update_project registered with required [project_id] (D-03 flat optional args — name/description/github_repo/icon_letters/icon_color optional via *string pointer fields). InputSchema and args struct EXCLUDE workspace_id and agent_id per Pitfall 3. Handler PATCHes only supplied fields; github_repo \"\" unlinks (NULL); Kamacu's partial-PATCH leaves omitted keys untouched"
    requirement: MCPPROJ-04
    verification:
      - kind: integration
        ref: "grep -qE 'Name:[[:space:]]+\"update_project\"' internal/mcp/projects.go AND properties block does NOT list workspace_id or agent_id"
        status: pass
      - kind: integration
        ref: "! grep -B2 -A12 'func (b \\*bridge) updateProject' internal/mcp/projects.go | grep -E 'WorkspaceID|AgentID' — args struct has no such fields"
        status: pass
      - kind: integration
        ref: "grep -qE '(Name|Description|GithubRepo|IconLetters|IconColor)[[:space:]]+\\*string' internal/mcp/projects.go — five pointer fields for partial-PATCH"
        status: pass
      - kind: unit
        ref: "internal/mcp/projects_test.go#TestBridge_UpdateProject_PartialPATCH_SendsOnlySuppliedFields — PASS ({name:new} only → body has name:new, NO description/github_repo/icon_letters/icon_color keys)"
        status: pass
      - kind: unit
        ref: "internal/mcp/projects_test.go#TestBridge_UpdateProject_GithubRepoEmptyString_Unlinks — PASS ({github_repo:\"\"} → body has github_repo: \"\" explicit empty, distinct from omitted)"
        status: pass
      - kind: unit
        ref: "internal/mcp/projects_test.go#TestBridge_UpdateProject_BodyExcludes_WorkspaceID_And_AgentID — PASS (agent supplies workspace_id=2 + agent_id=3; body has NEITHER — D-03 / Pitfall 3 double-lock)"
        status: pass
      - kind: unit
        ref: "internal/mcp/projects_test.go#TestBridge_UpdateProject_Kamacu400_DescriptionTooLong_ReturnsError — PASS (HTTP 400 substring; bridge does NOT enforce the 280-char cap)"
        status: pass
    human_judgment: false
  - id: D4
    description: "MCPPROJ-05: delete_project registered with required [project_id] — handler bridges to DELETE /api/projects/{id}; v1.4 two-pass gated removal (folder: hard delete never touching dir; managed: all-or-nothing with structured 409 reasons) runs unchanged through the bridge"
    requirement: MCPPROJ-05
    verification:
      - kind: integration
        ref: "grep -qE 'Name:[[:space:]]+\"delete_project\"' internal/mcp/projects.go AND handler is a single b.call(DELETE, /api/projects/{id}, nil)"
        status: pass
      - kind: unit
        ref: "internal/mcp/projects_test.go#TestBridge_DeleteProject_Happy_CallsDeletePath — PASS ({project_id:7} → DELETE /api/projects/7, 204 returns clean result)"
        status: pass
      - kind: unit
        ref: "internal/mcp/projects_test.go#TestBridge_DeleteProject_Managed409_PassesThroughStructuredBody — PASS (409 with {error, reasons} body; error contains \"HTTP 409\" + \"can't be deleted\" + \"reasons\" substrings — Gap 2 structured-body passthrough)"
        status: pass
    human_judgment: false
  - id: D5
    description: "Pattern fidelity: every new project tool handler delegates to bridge.call — NO handler re-implements b.do + io.ReadAll + non-200 wrap inline; NO typed mcp.AddTool[In,Out] generic used (Phase 06 lock preserved)"
    requirement: MCPPROJ-02
    verification:
      - kind: integration
        ref: "! grep -q 'io.ReadAll' internal/mcp/projects.go (no inline response reading)"
        status: pass
      - kind: integration
        ref: "! grep -q 'b\\.do(' internal/mcp/projects.go (no direct b.do calls — all via b.call)"
        status: pass
      - kind: integration
        ref: "! grep -q 'mcp.AddTool\\[' internal/mcp/projects.go (no typed generic)"
        status: pass
    human_judgment: false
  - id: D6
    description: "No bridge-side validation: projects.go performs no validation of project_id values, name emptiness, repo_path syntax, repo ref canonicalization, icon_letters/icon_color palette membership, or description length — Kamacu's existing handlers validate (SC4 satisfied by NOT duplicating validation in the bridge)"
    requirement: MCPPROJ-02
    verification:
      - kind: integration
        ref: "! grep -qE 'validateIconLetters|validateIconColor|validateRepoPath|github.ValidateRepo' internal/mcp/projects.go (no Kamacu validation logic imported)"
        status: pass
      - kind: integration
        ref: "! grep -qE 'if args\\..*== \"\"' internal/mcp/projects.go (no value guards)"
        status: pass
      - kind: unit
        ref: "TestBridge_CreateProject_Kamacu400_InvalidPath_ReturnsError proves Kamacu rejects bad paths with 400 (not the bridge)"
        status: pass
      - kind: unit
        ref: "TestBridge_UpdateProject_Kamacu400_DescriptionTooLong_ReturnsError proves Kamacu rejects >280-char descriptions with 400 (not the bridge)"
        status: pass
    human_judgment: false
  - id: D7
    description: "SC2 satisfied: registerProjectTools now registers 5 tools total (list_projects from Plan 01 + get_project, create_project, update_project, delete_project from this plan) — the five project tools drive the sidebar (list/get/create/update/delete, including v1.4 managed-checkout atomic create + all-or-nothing gated delete)"
    requirement: MCPPROJ-02
    verification:
      - kind: integration
        ref: "grep -cE 'Name:[[:space:]]+\"' internal/mcp/projects.go returns 5 (list_projects + 4 new)"
        status: pass
      - kind: integration
        ref: "go test ./internal/mcp/... — 33 tests PASS (8 carried-over Plan 01/Phase 06 + 13 Plan 02 task + 12 Plan 03 project)"
        status: pass
    human_judgment: false
  - id: D8
    description: "go vet ./internal/mcp/... + go test ./internal/mcp/... + go build ./... all clean"
    requirement: MCPPROJ-02
    verification:
      - kind: integration
        ref: "go vet ./internal/mcp/... — clean"
        status: pass
      - kind: integration
        ref: "go test ./internal/mcp/... — 33 tests PASS"
        status: pass
      - kind: integration
        ref: "go build ./... — clean (whole codebase compiles)"
        status: pass
    human_judgment: false

duration: 7min
completed: 2026-07-22
status: complete
---

# Phase 07 Plan 03: Four Project MCP Tools Summary

**Four project MCP tools (get_project / create_project / update_project / delete_project) bridging to Kamacu's project endpoints — each a thin build-path-and-delegate to bridge.call, with 12 httptest cases covering the D-04 two-arg fork, D-03 partial-PATCH with excluded fields, and the Gap 2 managed-delete 409 structured body.**

## Performance

- **Duration:** ~7 min
- **Started:** 2026-07-22T06:09:55Z
- **Completed:** 2026-07-22T06:17:25Z
- **Tasks:** 2
- **Files modified:** 2 (1 new + 1 modified)

## Accomplishments
- Extended `registerProjectTools` in `internal/mcp/projects.go` from 1 `s.AddTool` call (Plan 01's `list_projects`) to 5 — adding `get_project`, `create_project`, `update_project`, `delete_project`. Every handler follows the canonical shape Plan 02 established for tasks: typed args struct → `json.Unmarshal` with error wrap → conditional body map (PATCH only) → marshal → `b.call(ctx, method, path, body)`. No handler re-implements the response-handling half. MCPPROJ-02 through MCPPROJ-05 all delivered; SC2 (the five project tools drive the sidebar) satisfied.
- `get_project` (MCPPROJ-02) bridges to `GET /api/projects/{id}` — the Gap 1 endpoint Plan 01 added. A missing project surfaces Kamacu's 404 body verbatim through the D-05 wrap.
- `create_project` (MCPPROJ-03, D-04 two-arg fork) bridges to `POST /api/projects` with `{name, repo_path, repo, workspace_id?}` body sent AS-IS. Zero bridge-side dispatch — Kamacu's `if strings.TrimSpace(req.Repo) != "" { h.createByRepo(...) }` does the fork. `agent_id` is intentionally not exposed (new projects land on the global default agent — Claude seed on a fresh install; MCPMORE-01 Out of Scope).
- `update_project` (MCPPROJ-04, D-03 partial-PATCH) uses `*string` pointer fields for all five optional args (Name, Description, GithubRepo, IconLetters, IconColor). **The args struct has NO WorkspaceID and NO AgentID fields** (D-03 / 07-RESEARCH Pitfall 3) — workspace transfer is its own tool in a later plan; agent reassignment is Out of Scope. The double-lock (struct declaration + body-map construction) ensures excluded fields never reach Kamacu even if an agent supplies them.
- `delete_project` (MCPPROJ-05) is a one-liner delegating to `DELETE /api/projects/{id}` — the v1.4 two-pass gated removal (folder: hard delete, directory never touched; managed: gate phase computes dirty/unpushed/stash/sessions, REMOVE phase tears down on all-clear, 409 with structured `{error, reasons}` body otherwise) runs unchanged through the bridge.
- Created `internal/mcp/projects_test.go` with **12 test cases** (one happy + one representative error per tool per D-08, plus targeted D-03 / D-04 / Gap 2 coverage). Each test stands up an `httptest.Server` capturing method/path/body, constructs a `*mcp.CallToolRequest` with canned `Params.Arguments`, invokes the bridge method directly, and asserts the captured request + result TextContent / error substring. Error assertions match on `"HTTP 4NN"` / `"HTTP 5NN"` substrings per Gap 2 / Pitfall 2 — never on JSON key names.
- D-04 coverage: `TestBridge_CreateProject_FolderPath_SendsAllFieldsAsIs` + `TestBridge_CreateProject_ManagedRepo_SendsAllFieldsAsIs` assert all four fields (name/repo_path/repo + workspace_id when supplied) are sent as-is — including the empty string for whichever of `repo`/`repo_path` was omitted. The bridge does NOT branch on `args.Repo != ""`.
- D-03 coverage: `TestBridge_UpdateProject_BodyExcludes_WorkspaceID_And_AgentID` feeds the agent-supplied excluded fields (`workspace_id:2, agent_id:3`) and asserts NEITHER appears in the captured body — the args struct drops them at unmarshal time AND the body map construction only iterates declared fields. `TestBridge_UpdateProject_PartialPATCH_SendsOnlySuppliedFields` asserts description/github_repo/icon_letters/icon_color are ABSENT when only name is supplied.
- Gap 2 coverage: `TestBridge_DeleteProject_Managed409_PassesThroughStructuredBody` simulates the real Kamacu managed-409 body `{"error":"the project can't be deleted yet","reasons":[{"kind":"uncommitted","target":"task #3"}]}` and asserts the wrapped error contains `"HTTP 409"`, `"can't be deleted"` (message substring), AND `"reasons"` (structured body survived in the raw string). The D-05 wrap passes the body as a raw string so the structured shape survives — but assertions match on substrings of Kamacu message text, NOT JSON key names.
- github_repo empty-string unlink coverage: `TestBridge_UpdateProject_GithubRepoEmptyString_Unlinks` asserts an explicit `""` IS sent in the body (Kamacu unlinks → NULL), distinct from omitted (nil pointer → left untouched).

## Task Commits

Each task was committed atomically:

1. **Task 1: Implement the four new project tool handlers in registerProjectTools (get_project, create_project, update_project, delete_project)** — `7b7ff5a` (feat)
2. **Task 2: Add projects_test.go with 12 happy/error test cases per D-08 (including managed-delete 409 structured body)** — `c941bb7` (test)

**Plan metadata:** this commit (docs: complete projects-tools plan)

## Files Created/Modified
- `internal/mcp/projects.go` — `registerProjectTools` body extended from 1 `s.AddTool` call (list_projects) to 5 (the four new tools added). Four new `*bridge` methods: `getProject` (one-liner GET), `createProject` (D-04 — body map with name/repo_path/repo always + workspace_id when non-nil, no bridge-side dispatch), `updateProject` (D-03 — `*string` pointer fields for all five optional args, body map conditionally populated, NO WorkspaceID/AgentID fields), `deleteProject` (one-liner DELETE).
- `internal/mcp/projects_test.go` (NEW) — 12 test cases mirroring Plan 02's `tasks_test.go` pattern: `TestBridge_GetProject_{Happy,NotFound404}`, `TestBridge_CreateProject_{FolderPath,ManagedRepo,WorkspaceIDIncluded,Kamacu400}`, `TestBridge_UpdateProject_{PartialPATCH,GithubRepoEmptyString,BodyExcludes,Kamacu400DescriptionTooLong}`, `TestBridge_DeleteProject_{Happy,Managed409}`.

## Decisions Made
- **`create_project` body marshals `{name, repo_path, repo}` unconditionally and `workspace_id` only when non-nil** (D-04). The alternative (conditional `repo`/`repo_path` inclusion) would require bridge-side type detection — explicitly prohibited. Kamacu's `strings.TrimSpace(req.Repo) != ""` check is the canonical dispatch; the bridge sends every field as-is.
- **`create_project` does NOT expose `agent_id`** in its InputSchema or args struct even though Kamacu's create handler accepts it. New projects land on the global default agent (Claude seed on a fresh install). Agent assignment at create-time is deferred to a future tool (MCPMORE-01 Out of Scope).
- **`update_project` args struct has NO `WorkspaceID` and NO `AgentID` fields** (D-03 / 07-RESEARCH Pitfall 3). Workspace transfer has its own tool in a later plan; agent reassignment is Out of Scope. The double-lock (struct declaration + body-map construction) ensures excluded fields never reach Kamacu even if an agent supplies them — verified by `TestBridge_UpdateProject_BodyExcludes_WorkspaceID_And_AgentID`.
- **`update_project` uses `*string` pointer fields for ALL five optional args** (Name, Description, GithubRepo, IconLetters, IconColor) — nil = omitted vs `""` = explicit value. Critical for `github_repo` where empty = unlink (Kamacu stores NULL) vs omitted = leave untouched.
- **`delete_project` description in the InputSchema** enumerates the v1.4 gated-removal behavior verbatim (folder: hard delete, dir untouched; managed: gate → 409 with reasons / REMOVE on all-clear). The tool surfaces the same gate the browser-attached user hits — no privilege escalation (T-07-09 accept).
- **Plan's literal grep acceptance criteria** (e.g. `grep -q 'Name: "<tool>"'`) was satisfied via the regex form `grep -qE 'Name:[[:space:]]+"<tool>"'` — same approach Plan 02 took. gofmt aligns consecutive struct field assignments with whitespace; the plan's literal pattern doesn't match the gofmt-aligned reality. Semantic intent (the tool name exists as a `Name:` value) is preserved. NOT a deviation — it's a verification-pattern adjustment that matches Plan 02's documented practice.

## Deviations from Plan

None - plan executed exactly as written. The only adjustment was the regex form of the grep acceptance criteria (see Decisions Made); semantic intent preserved verbatim. The plan's prohibitions (no touching of internal/api/, internal/mcp/server.go, internal/mcp/bridge.go, internal/mcp/tasks.go, or internal/mcp/workspaces.go; no typed `mcp.AddTool[In,Out]` generic; no bridge-side validation; no `workspace_id`/`agent_id` in update_project; no bridge-side dispatch in create_project; no JSON key name assertions on error bodies) are all respected. `git diff --name-only` for this plan shows exactly two files: `internal/mcp/projects.go` and `internal/mcp/projects_test.go`.

## Issues Encountered
None. The `go vet ./internal/mcp/...`, `go test ./internal/mcp/...` (33 tests PASS — 8 carried-over Plan 01/Phase 06 + 13 Plan 02 task + 12 new Plan 03 project), and `go build ./...` are all clean. Plan 02's Rule 1 fix widening `bridge.call` to the full 2xx range is what made this plan's `create_project` (POST → 201) and `delete_project` (DELETE → 204) test cases pass without further bridge work.

## User Setup Required
None — this plan adds four MCP tools and tests; no external service configuration is required.

## Next Phase Readiness
- **Plan 04 (workspace tools) ready.** `internal/mcp/workspaces.go` has the empty `registerWorkspaceTools(s, b)` shell from Plan 01; Plan 04 adds the five workspace tools (`list_workspaces` / `get_workspace` / `create_workspace` / `update_workspace` / `delete_workspace`) in the same file. The underlying `GET/POST/PATCH/DELETE /api/workspaces` endpoints already exist unchanged. The bridge.call helper, the httptest test pattern, and the per-resource-split shape are all proven across Plans 01/02/03.
- **MCPPROJ-02..05 delivered HERE.** Combined with Plan 01's `list_projects(workspace_id?)` (MCPPROJ-01), all five project tools are now live. MCPPROJ-06..07 (workspace tools) land in Plan 04 — completing Phase 07's wave-2 MCP tool surface.

---
*Phase: 07-tasks-projects-workspaces-tools*
*Completed: 2026-07-22*

## Self-Check: PASSED

- Created files exist on disk: `internal/mcp/projects_test.go`, `.planning/phases/07-tasks-projects-workspaces-tools/07-03-SUMMARY.md` — all FOUND.
- Modified files exist on disk: `internal/mcp/projects.go` — FOUND.
- Task commits exist in git log: `7b7ff5a` (Task 1, feat) and `c941bb7` (Task 2, test) — both FOUND.
- Re-ran `go test ./internal/mcp/...` post-SUMMARY: package `ok` (33 tests PASS — 12 new project tests + 8 carried-over Plan 01/Phase 06 + 13 Plan 02 task tests).
- Plan-level verification gate (8 coverage entries) all PASS: 5 AddTool calls in registerProjectTools (list_projects + 4 new); 4 new bridge methods delegating to b.call; update_project excludes workspace_id/agent_id (D-03 / Pitfall 3); create_project sends all fields as-is (D-04); no JSON-key error assertions (Gap 2 / Pitfall 2); no typed mcp.AddTool generic; no bridge-side validation; no io.ReadAll / b.do inline; go vet + go test + go build clean.
