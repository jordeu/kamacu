# Phase 07: Tasks, Projects & Workspaces Tools - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-07-22
**Phase:** 07-tasks-projects-workspaces-tools
**Areas discussed:** Endpoint gaps (list_tasks scope, list_projects workspace filter), MCP argument shapes (update_*, create_project fork), Error response shape, move_task position semantics, File organization & test breadth

---

## Endpoint gaps (list_tasks scope, list_projects workspace filter)

### list_tasks without project_id

| Option | Description | Selected |
|--------|-------------|----------|
| Add GET /api/tasks unscoped endpoint to Kamacu | New endpoint in internal/api/tasks.go + route in routes.go. Returns ALL manual tasks across all projects. Bridge calls it when project_id omitted, falls through to GET /api/projects/{id}/tasks when supplied. | ✓ |
| Make project_id required | Reject list_tasks calls that omit project_id. Zero new code in Kamacu. Diverges from the documented (project_id?) signature. | |
| Bridge-side fan-out | Bridge calls GET /api/projects, then GET /api/projects/{id}/tasks for each, concatenates. Zero internal/api/ changes. N+1 HTTP calls. | |

**User's choice:** Add GET /api/tasks unscoped endpoint to Kamacu.
**Notes:** Symmetric with the list_projects+workspace_id decision (both extend Kamacu's endpoints rather than push logic into the bridge or restrict the documented optional-arg signature).

### list_projects with workspace_id

| Option | Description | Selected |
|--------|-------------|----------|
| Add ?workspace_id=N query param to GET /api/projects | ~5-line extension in projectHandlers.list. Backward-compatible (no param = current behavior). | ✓ |
| Bridge-side filter on the full list | Bridge calls GET /api/projects, filters the array in Go. Zero Kamacu changes. Silently [] for a bogus id. | |
| Keep workspace_id as a documented no-op | Same posture as Phase 06's project_id. Diverges from the documented (workspace_id?) semantics. | |

**User's choice:** Add ?workspace_id=N query param to GET /api/projects.

### GET /api/tasks exact shape

| Option | Description | Selected |
|--------|-------------|----------|
| GET /api/tasks (no params) + keep GET /api/projects/{id}/tasks untouched | Two clean paths. New endpoint mirrors listByProject (source='manual', ORDER BY status/position). | ✓ |
| GET /api/tasks?project_id=N (single endpoint, optional query) | One endpoint handles both cases. Duplicates the scoped-route semantics. | |
| You decide — minimal is fine | Planner/researcher picks shape. Lock only source='manual' + status/position ordering + no extra query params. | |

**User's choice:** GET /api/tasks (no params) + keep GET /api/projects/{id}/tasks untouched.

---

## MCP argument shapes (update_*, create_project fork)

### update_* schema shape

| Option | Description | Selected |
|--------|-------------|----------|
| Flat optional fields at the top level | Mirrors Kamacu's PATCH body shape exactly. Matches Phase 06's flat map. | ✓ |
| Single nested `updates` object | Groups the PATCH body explicitly. Extra nesting layer; the Kamacu shape is already flat. | |
| You decide | Lock only the rule: every existing PATCH field without a dedicated tool is exposed as optional. | |

**User's choice:** Flat optional fields at the top level.
**Notes:** update_project excludes workspace_id (has move_project_to_workspace) and agent_id (Out-of-Scope per MCPMORE-01).

### create_project schema shape

| Option | Description | Selected |
|--------|-------------|----------|
| Two distinct optional args: repo_path? and repo? | Mirrors underlying POST shape. Handler's existing fork does the dispatch — zero bridge logic. | ✓ |
| One `source` arg with type detection | Tighter signature, matches REQUIREMENTS.md literally. Bridge encodes type-detection logic the API already handles. | |
| You decide | Lock only: must support both folder + managed-checkout. Planner picks one-arg-vs-two-arg. | |

**User's choice:** Two distinct optional args: repo_path? and repo?.

---

## Error response shape

| Option | Description | Selected |
|--------|-------------|----------|
| Keep Phase 06 pattern: wrap as a single error string | fmt.Errorf("HTTP %d: %s", code, body) — body is raw JSON, message preserved verbatim. Zero parsing. | ✓ |
| Parse Kamacu's JSON body and surface just the `message` field | Cleaner agent-facing error. ~10-line shared parse-error helper. Drops HTTP status code from message. | |
| Typed MCP error content (IsError result instead of Go error) | Distinguishes "Kamacu rejected" from "transport failed". ~25 lines plumbing per handler; diverges from Phase 06. | |
| You decide — defer to researcher | Lock only: errors actionable + SC2 invariant holds. | |

**User's choice:** Keep the Phase 06 pattern: wrap as a single error string.

---

## move_task position semantics

| Option | Description | Selected |
|--------|-------------|----------|
| Always send after_id = null (top of column) | Predictable behavior. Agent never reasons about board ordering. | ✓ |
| Expose after_id as an optional third arg | Lets agent preserve relative ordering. Agents rarely know other tasks' IDs. | |
| You decide | Lock only: must produce a board-visible move. | |

**User's choice:** Always send after_id = null (top of column).

---

## File organization & test breadth

### internal/mcp/ file layout

| Option | Description | Selected |
|--------|-------------|----------|
| Split per resource: tasks.go, projects.go, workspaces.go | Mirrors internal/api/* pattern. Each file stays ~150 lines. Tests mirror the split. | ✓ |
| Keep everything in bridge.go + server.go | Fewer files; matches Phase 06 starting shape. Large files get harder to navigate. | |
| You decide | Lock only: mirror internal/api/* per-resource pattern. | |

**User's choice:** Split per resource: tasks.go, projects.go, workspaces.go under internal/mcp/.

### Test breadth

| Option | Description | Selected |
|--------|-------------|----------|
| One representative test per tool, per resource | ~30 new cases total. Mirror bridge_test.go httptest pattern. | ✓ |
| Pareto: deep canary per resource, smoke the rest | ~15-20 cases. Faster; misses per-tool validation regressions. | |
| Comprehensive: 3-5 cases per tool | ~50-60 cases. Most thorough; slowest to write. | |
| You decide | Lock only: httptest pattern + SC2 green + ≥1 happy path per tool. | |

**User's choice:** One representative test per tool, per resource.

---

## the agent's Discretion

Areas the user explicitly left to the researcher/planner:
- Bridge method exact signatures (typed struct arg vs individual args)
- Exact InputSchema map shape per tool (property maps, descriptions, required-arrays)
- Whether the bridge gets a shared `doJSON` helper vs each handler inlining ReadAll+TextContent
- Test naming convention (follow Phase 06's `TestBridge_<Verb><Resource>_<Scenario>` shape)
- GET /api/tasks handler naming (`taskHandlers.list` vs `listAll`)
- `?workspace_id=N` query-param parsing strictness (follow Kamacu's "silently ignore unknown/empty" convention)
- `move_task` schema description copy

## Deferred Ideas

Ideas considered but kept out of Phase 07:
- **MCPAUTO-01..03** — per-task auto-scoping (deferred to v1.12)
- **MCPREG-01..03** — agent-CLI auto-registration (deferred to v1.12)
- **MCPMORE-01..03** — additional tool categories: Agents CRUD, Settings, Worktree cleanup (deferred to v1.12); `update_project`'s `agent_id` field is omitted until agents-tools phase ships
- **MCPHARD-01..03** — typed JSON-RPC error taxonomy, full stdout guards, real-binary e2e harness (deferred to v1.12)
- **Positional `move_task(after_id)`** — exposing after_id for relative ordering (deferred indefinitely; agents have no reason to care)
- **Pagination/filtering on list_tasks/list_projects** — status filter, since-cursor, result caps NOT added in Phase 07
- **Token auth on general /api/* routes** — explicitly out of v1.11 (loopback binding is the boundary)
