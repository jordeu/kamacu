# Phase 09: PR Review Tools - Context

**Gathered:** 2026-07-23
**Status:** Ready for planning

<domain>
## Phase Boundary

The final, thinnest phase of v1.11: ship the last 3 MCP tools (`list_pending_reviews`, `list_recently_reviewed`, `open_review`) by bridging Kamacu's existing v1.3 / v1.5 `internal/github` surface. An agent inside a Kamacu task PTY can triage the user's PR review load and open review workspaces — reusing the existing gh integration with **no new `gh` calls, no new gates, no new endpoints**. This is the milestone's mechanical coda: everything is translation over endpoints that already exist.

**In this phase (3 new MCP tools in ONE new file `internal/mcp/reviews.go` — ZERO Kamacu endpoint changes):**

1. **`list_pending_reviews(project_id)`** — PRs awaiting the user's review. Bridges `GET /api/projects/{id}/pull-requests` and returns the `prs` array (the `user-review-requested:@me draft:false` queue).
2. **`list_recently_reviewed(project_id)`** — open PRs the user already reviewed. Bridges the SAME `GET /api/projects/{id}/pull-requests` and returns the `reviewed` array (the `reviewed-by:@me draft:false` queue).
3. **`open_review(project_id, pr_number)`** — open-or-reattach a PR as a review workspace. Bridges `POST /api/projects/{id}/pull-requests/{n}/review` verbatim.

**Critical code-scout fact that shapes this phase:** the two list queries ride ONE cached endpoint. `GET /api/projects/{id}/pull-requests` returns `github.Result{State, Stale, FetchedAt, PRs, Reviewed}` where `PRs` = awaiting and `Reviewed` = reviewed-by:@me, both fetched in one `gh` cycle under one per-repo `repoEntry` (60s success TTL, 10s attempt floor). So `list_pending_reviews` and `list_recently_reviewed` hit the *same* URL and project the same `Result` into different arrays. The second call is cache-free (the milestone's "no N+1 `gh` calls" rule is satisfied by construction — one `gh` cycle serves both queues for a repo).

**Out of this phase (per REQUIREMENTS.md Out of Scope / Deferred):**

- **The `GET /api/projects/{id}/pull-requests/{n}` live-PR-detail endpoint** (re-hydration for hard reloads) — exists in `pullrequests.go:93` but is NOT exposed as an MCP tool. The 3 MCPREV tools don't include a `get_pr`/`get_review_diff`; the SCs don't mention one.
- **Global / cross-project PR triage** (`list_pending_reviews()` with no `project_id` aggregating across all linked repos) — D-04 makes `project_id` required. An agent wanting a global view calls `list_projects` then loops. The aggregate path is deferred (see `<deferred>`).
- **PTY keystroke injection, MCP server for external editors, agent-CLI auto-registration (MCPREG), per-task auto-scoping (MCPAUTO), additional tool categories (MCPMORE), typed bridge error taxonomy / stdout guards / e2e harness (MCPHARD)** — all inherited Out-of-Scope from v1.11, unchanged. Phase 09 surfaces generic MCP errors via the Phase 07 D-05 wrap (and the D-01 IsError flag for the list tools' state field); the typed taxonomy stays v1.12.
- **Merge / PR write automation** — Kamacu never writes to GitHub (approve/request-changes/comment/merge stay terminal-only). `open_review` provisions a workspace; it does not approve/merge.
- **Token auth middleware on `/api/*` routes** — Phase 06 D-06; loopback binding remains the boundary. The review endpoints inherit this unchanged.

**Requirements covered:** MCPREV-01, MCPREV-02, MCPREV-03.

</domain>

<decisions>
## Implementation Decisions

### Degradation contract for the list tools (D-01..D-03) — the one genuine divergence

The load-bearing detail: `GET /api/projects/{id}/pull-requests` **always returns HTTP 200** — degradation rides in the `state` field of `github.Result` (`ok` | `disabled` | `no_gh` | `auth_required` | `error`), NEVER in the HTTP status (this is GHSET-03 / GHCOL-05; the frontend branches on `state`/`stale`, never on the status). The locked `bridge.call` pattern (2xx → TextContent passthrough, non-2xx → D-05 error wrap) therefore does NOT fit the list tools — a `state:"no_gh"` would surface as a successful result. SC3 demands an "actionable MCP error" when github is off / unlinked / gh absent.

- **D-01:** **IsError hybrid.** The two list tools do NOT use `bridge.call` verbatim. A review-specific bridge helper does `b.do` → read body → parse the top-level `state` field → branch: when `state == "ok"` return `CallToolResult{Content:[TextContent{...}], IsError:false}`; when `state != "ok"` return `CallToolResult{Content:[TextContent{...}], IsError:true}`. This is the **first bridge-side JSON inspection in the milestone** (Phase 08's subscribe diverged from `bridge.call` for streaming; Phase 09 diverges for the state-field model) — but it is narrowly scoped: it reads ONE top-level string field and sets ONE bool. Transport errors and any genuine non-2xx still wrap via the D-05 shape (`kamacu %s %s: ...`). `open_review` is UNAFFECTED — its gates return real non-2xx (409/502/400) so it uses `bridge.call` verbatim (D-05).
- **D-02:** **Pure JSON body on error.** When `IsError=true`, the TextContent payload is the raw Kamacu `Result` JSON verbatim (e.g. `{"state":"no_gh","prs":null,"reviewed":null,...}`) — NO prose prefix, NO state→message map. The `state` value itself names the problem (`disabled` = toggle off or unlinked; `no_gh` = gh absent; `auth_required` = needs `gh auth login`; `error` = transient). An LLM agent reads the JSON fine; the IsError flag is the actionable signal and the state field is the actionable reason. Consistent with the project's honesty-over-hand-holding posture (cf. 08 D-05 / D-M001-2: don't fake-clean what we can't reliably interpret — hand the agent the faithful payload and tell it the shape).
- **D-03:** **Per-tool array on success; full Result on degradation.** Because the bridge already parses `state`, projecting the right queue is nearly free, and it gives each tool a distinct, non-redundant result:
  - `list_pending_reviews` success → the `prs` array (re-marshalled; just the awaiting PRs).
  - `list_recently_reviewed` success → the `reviewed` array (just the reviewed PRs).
  - Degradation (`state != "ok"`) for EITHER tool → `IsError=true` + the full `Result` JSON (state visible, both arrays null). The agent checks `IsError` first, so the success-array-vs-error-object shape difference is unambiguous. (Rejected: "both tools return the identical full Result" — byte-identical payloads across two tools is a design smell; "per-tool envelope `{state, fetchedAt, <queue>}`" — more rebuild logic, no win over the array + IsError-flag combination.)

### Optional `project_id` semantics (D-04)

- **D-04:** **`project_id` is REQUIRED on both list tools** (and on `open_review`, which already requires it via the POST path). This is a deliberate refinement of the REQUIREMENTS signatures `list_pending_reviews(project_id?)` / `list_recently_reviewed(project_id?)` — the `?` is dropped. Rationale: PRs are inherently per-repo, repos are per-project, the endpoint is per-project (`/api/projects/{id}/pull-requests`), and an agent runs inside one project+task PTY so it always knows its `project_id`. Requiring it keeps every Phase 09 tool a SINGLE `bridge.call` (the locked 06/07/08 "every tool is the same shape" invariant), needs NO new Kamacu endpoint, and trivially satisfies the "no N+1 `gh` calls" rule. An agent wanting a global view calls `list_projects` then `list_pending_reviews(project_id)` per linked project (each is a cached read). The aggregate-all path was considered and deferred (see `<deferred>`). The InputSchema for both list tools declares `project_id` (integer) as required; omitting it returns a clear MCP error before any HTTP call.

### `open_review` result & freshness (D-05..D-06)

- **D-05:** **`open_review` returns the raw `{task, pr}` envelope as TextContent** (IsError=false) via `bridge.call` verbatim — the locked 06/07/08 passthrough default. Kamacu's `POST .../review` already returns `{task, pr}` (200) where `task` is the find-or-create `source='github_pr'` row (id, worktree_path, status='todo', pr_number, pr_base_ref) and `pr` is the live `gh pr view` detail (`prWire`: number/title/body/author/url/baseRefName/headRefName/commits/state). The agent gets `task.id` (to reference the review task in later tool calls), the provisioned `worktree_path`, and the live PR meta. No trimming, no bridge-side shaping. **No agent auto-start** — `open_review` provisions the workspace only (status='todo', worktree on the PR head); the browser-attached user or a separate session opens it, per the milestone's "no auto-start" rule. SC2's "opens identically to clicking the PR card in the browser (open-or-reattach, no duplicates, board-leak guard intact)" is satisfied because the bridge calls the exact POST the browser calls — the find-or-create + `CheckoutPR` + board-leak filter all run server-side, unchanged.
- **D-06:** **No `refresh` / force-fetch param on any tool.** The list tools rely on the existing per-repo cache (60s success TTL `cacheTTL`, 10s attempt floor `attemptFloor` — `service.go:86-90`); agents get fresh-enough data and the tool surface stays minimal. The browser keeps its refresh button (`?refresh=1`); the MCP tools do not expose one for v1.11. (Deferred — see `<deferred>`.)

### SC3 satisfaction (how degradation never bypasses the two-gate ladder)

The v1.3 two-gate ladder (toggle → link) is enforced **server-side** in `pullrequests.go` on every call; the bridge adds NO gate logic and NO bypass. Specifically: the list GET runs GATE 1 (settings toggle) + GATE 2 (project linked) before ever spawning `gh`, returning `state:"disabled"` when either gate blocks; `open_review`'s POST returns **409** when either gate blocks (and 502 on `gh`/worktree failure). So: list tools surface a blocked gate as `IsError=true` + `state:"disabled"` (D-01/D-02); `open_review` surfaces it as a D-05 wrapped `HTTP 409: ...` error. Both are actionable MCP errors; neither can reach `gh` when a gate is closed.

### the agent's Discretion

- **`internal/mcp/reviews.go` handler + test layout** — mirror Phase 07's per-resource shape (D-07): `registerReviewTools(s, b)` called from `registerTools` in `server.go`; `bridge.listPendingReviews` / `listRecentlyReviewed` / `openReview` methods; one `reviews_test.go` with happy + error cases per tool (07 D-08). The review-specific list helper (the D-01 state-inspecting divergence) lives here alongside the `bridge.call`-using `openReview`.
- **How `state` is parsed** — unmarshal the body into `github.Result` (type-safe; couples `internal/mcp` to `internal/github` for the type, acceptable since `internal/github` is a leaf domain package with no DB/store import) OR a tiny local struct / `map[string]any` reading only the `state` + `prs`/`reviewed` keys (no coupling). Pick whichever is cleanest; the milestone has not coupled `internal/mcp` to another `internal/*` package yet, so the local-struct path keeps that streak, but `github.Result`/`PRSummary` are stable wire types. Agent's call.
- **The review-specific list helper signature/name** — e.g. `func (b *bridge) listReviews(ctx, projectID int64, queue string) (*mcp.CallToolResult, error)` shared by both list tools (only `queue` differs: "prs" vs "reviewed"), vs two separate methods. The shared-helper shape is natural since both hit the same URL; agent's call.
- **Exact InputSchema map shape per tool** — property maps, descriptions, required-arrays. Phase 07's flat `map[string]any` template applies. `project_id` is required on all 3 tools; `pr_number` (integer) required on `open_review`. No user preference on description copy beyond "lead with what it does; note the Kamacu endpoint it bridges to" (07 `<specifics>`).
- **Error-case test coverage** — one representative error per tool (07 D-08): e.g. list tool with a disabled/unlinked project → assert `IsError=true` + state field; `open_review` with integration off → assert D-05 wrapped `HTTP 409`. The `httptest` pattern (Phase 06/07 `bridge_test.go`) applies; the list-tool test must exercise the state→IsError branch (a fake handler returning `{"state":"no_gh",...}` with HTTP 200).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & roadmap (scope anchors)
- `.planning/ROADMAP.md` § "Phase 09: PR Review Tools" — the goal and the **3 success criteria** (list tools return the same PR lists the browser Review column shows, riding the per-repo TTL cache with no N+1 `gh` calls; `open_review` reuses the v1.3 Phase 12 find-or-create `source='github_pr'` path with worktree on the PR's real head, opening identically to clicking the PR card; graceful degradation when github off / unlinked / gh absent — actionable MCP error, never bypassing the two-gate ladder).
- `.planning/REQUIREMENTS.md` § "PR Review (MCPREV)" lines 32–36 — **MCPREV-01/02/03** signatures. NOTE: D-04 refines MCPREV-01/02 — `project_id` is REQUIRED, not optional as the `?` in the signature suggests.
- `.planning/REQUIREMENTS.md` § "Out of Scope" + "v1.12+ Requirements" — the `GET .../pull-requests/{n}` detail endpoint is NOT a tool; global aggregate triage, MCPAUTO/MCPREG/MCPHARD/MCPMORE all stay out. Read before planning so none leak in.
- `.planning/PROJECT.md` § "Active: v1.11 Kamacu MCP Server" — the milestone goal and the **"Backend-only milestone: no DB schema changes, no migrations, no frontend changes, no new long-lived goroutines inside the Kamacu binary"** rule. Phase 09 is the purest expression: ZERO Kamacu endpoint changes, ZERO DB/migration/frontend/goroutine work — only `internal/mcp/` additions.

### Phase 06 + 07 + 08 foundation (the pattern Phase 09 repeats)
- `.planning/phases/06-mcp-subcommand-foundation/06-CONTEXT.md` — the bridge pattern is locked here. D-06/D-07 (loopback auth; `X-Kamacu-Token` on every call), D-09 (proof tool is FINAL), D-12 (stdout cleanliness). Phase 09 inherits `*bridge`, `map[string]any` InputSchema, raw TextContent passthrough, loopback-only auth verbatim.
- `.planning/phases/07-tasks-projects-workspaces-tools/07-CONTEXT.md` — D-05 (error wrap as single string `kamacu %s %s: HTTP %d: %s`), D-07 (per-resource file split — Phase 09 adds `reviews.go`), D-08 (one representative test per tool). The `bridge.call` helper and the endpoint-extension pattern are the direct templates.
- `.planning/phases/08-sessions-terminal-read-access/08-CONTEXT.md` — the precedent that a tool MAY diverge from `bridge.call` when the endpoint demands it (subscribe diverged for streaming; Phase 09's list tools diverge for the state-field model). D-01's IsError hybrid is the second such scoped divergence.

### The v1.3 / v1.5 PR surface (the code Phase 09 bridges to — the whole point of the phase)
- `internal/api/pullrequests.go:39` `PullRequestRoutes` — registers the 3 routes Phase 09 bridges to. **No changes needed.**
- `internal/api/pullrequests.go:40-81` — the GET list handler. GATE 1 (settings toggle off → `state:"disabled"`, `pullrequests.go:54`) + GATE 2 (project unlinked/unknown → `state:"disabled"`, `:64`/`:74`) run BEFORE any `gh` spawn; on success calls `svc.Get(...)` (`:80`) returning `github.Result`. Always 200. This is the endpoint backing both `list_pending_reviews` and `list_recently_reviewed`.
- `internal/api/pullrequests.go:148-268` — the POST review handler (open-or-reattach). GATE 1 → 409 (`:166`), GATE 2 → 409 (`:175`/`:183`), `gh` failure → 502 (`:207`), worktree failure → 502 (`:246-258`). Find-or-create `source='github_pr'` task (`:191`/`:231`), `CheckoutPR` on the PR head (`:256`), returns `{task, pr}` via `writeReview` (`:267`/`:308`). This is `open_review`'s bridge target — used verbatim.
- `internal/api/pullrequests.go:93-137` — the `GET .../pull-requests/{n}` live-detail handler. **NOT exposed as an MCP tool** (out of scope); documented here so the planner knows it exists and is intentionally skipped.
- `internal/api/pullrequests.go:277-305` `prWire` + `prWireFrom` — the live PR detail shape `open_review` returns inside `{task, pr}`.
- `internal/github/service.go:19-25` `github.Result` — **the wire contract D-01 inspects.** Fields: `State` (`ok`|`disabled`|`no_gh`|`auth_required`|`error`), `Stale`, `FetchedAt`, `PRs` (awaiting), `Reviewed`. D-01 reads `State`; D-03 projects `PRs` vs `Reviewed`.
- `internal/github/service.go:34-43` `repoEntry` + `:86-90` (`cacheTTL=60s`, `attemptFloor=10s`, `maxFailures=3`) — the per-repo cache. Both queues ride ONE entry/fetchedAt/TTL (D-12 of v1.5). D-06's "no refresh param" relies on this.
- `internal/github/service.go:109` `Service.Get` — the demand-driven cache entry point the GET handler calls. No changes.
- `internal/github/prlist.go:22-34` `PRSummary` — the per-PR shape in both arrays (number/title/author/updatedAt/url/checks/headRef*/baseRef*/isCrossRepository). NOTE: no `repo`/`project` field — relevant to why D-04 requires `project_id` (a flat cross-project merge would lose provenance).
- `internal/github/prlist.go:194-197` `searchAwaiting` / `searchReviewed` — the two `--search` args (`user-review-requested:@me draft:false` and `reviewed-by:@me draft:false`) that define the two queues.
- `internal/github/prlist.go:293-304` `fetchLists` — proves both queues come from ONE `gh` cycle (the "no N+1" guarantee).

### Bridge pattern (the code Phase 09 extends — and where it diverges)
- `internal/mcp/bridge.go:31-67` `bridge` struct + `do()` — the per-process HTTP client with `X-Kamacu-Token` on every call. All 3 review tools start here.
- `internal/mcp/bridge.go:88-106` `bridge.call()` — the shared 2xx→passthrough / non-2xx→D-05-wrap helper. `open_review` uses it verbatim. **The two list tools do NOT** — they need the D-01 state-inspecting variant (do → read → parse `state` → IsError branch).
- `internal/mcp/bridge.go:23` `maxBodyBytes = 1<<20` (1 MiB) — the response ceiling. PR lists are tiny; no concern.
- `internal/mcp/server.go` `registerTools` — where `registerReviewTools(s, b)` slots in alongside `registerSessionTools` / `registerTaskTools` / `registerProjectTools` / `registerWorkspaceTools` (07 D-07 split). One new line.
- `internal/mcp/{tasks,projects,workspaces,sessions}.go` — the per-resource handler template. `reviews.go` mirrors this shape.

### MCP SDK (the dependency surface)
- `github.com/modelcontextprotocol/go-sdk` @ v1.6.1, `mcp.CallToolResult` — note the `IsError bool` field (D-01 sets it). The Phase 07/08 tools left it false; Phase 09's list tools set it true on `state != "ok"`.

### Established project patterns (constraints, not files)
- **Bridge pattern (locked in 06/07/08)** — env → `*bridge` HTTP client (`X-Kamacu-Token`) → Kamacu endpoint → TextContent passthrough. `open_review` follows it verbatim; the list tools extend it with a state-field inspection (D-01).
- **Per-resource file split (07 D-07)** — `internal/mcp/reviews.go` owns the 3 tools' registrar + `bridge.*Review*` methods + `reviews_test.go`.
- **`httptest`-based integration tests** — `internal/mcp/*_test.go` spin up `httptest.Server`s; `reviews_test.go` mirrors them, plus a state→IsError case for the list tools.
- **`slog` to stderr** — the MCP subcommand pins this (`server.go`); review handlers add no stdout writes (SC2 stream-desync invariant from Phase 06 stays green).
- **Single-writer SQLite discipline** — bridge code MUST NOT import `internal/store`; it goes through the HTTP API. (Phase 09 touches no DB code at all.)

No external ADRs/specs — requirements are fully captured in the decisions above plus these in-repo references.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`bridge.call` (`bridge.go:88`)** — IS `open_review`'s handler body (POST → `{task, pr}` passthrough; 409/502/400 → D-05 error). Zero adaptation.
- **`GET /api/projects/{id}/pull-requests` (`pullrequests.go:40`)** — IS both list tools' bridge target. Returns `github.Result` with both queues + state. The list helper parses `state`, projects `prs`/`reviewed`.
- **`POST /api/projects/{id}/pull-requests/{n}/review` (`pullrequests.go:148`)** — IS `open_review`'s bridge target. Find-or-create + `CheckoutPR` + `{task, pr}` envelope, all server-side.
- **`github.Result` / `PRSummary` (`service.go:19` / `prlist.go:22`)** — the wire types the list helper decodes (or mirrors locally) to read `state` and project the queue.
- **`registerTools` (`server.go`)** — the one-line integration point for `registerReviewTools(s, b)`.

### Established Patterns
- **Bridge pattern (locked in 06/07)** — every Phase 09 tool starts from `b.do`; `open_review` completes via `bridge.call`, the list tools via a state-inspecting sibling.
- **Per-resource file split (07 D-07)** — `reviews.go` mirrors `tasks.go`/`projects.go`/`sessions.go`.
- **Scoped divergence from `bridge.call` (08 precedent)** — when an endpoint's contract doesn't fit 2xx-success/non-2xx-error, a resource-specific helper is acceptable (subscribe streamed; the list tools inspect state). Narrowly scoped, documented.
- **Raw TextContent passthrough of Kamacu JSON** — `open_review` passes `{task, pr}` through; the list tools pass the projected array / full Result through. No bridge-side result shaping beyond the IsError flag + queue projection.

### Integration Points
- **`registerReviewTools(s, b)` in `internal/mcp/server.go`'s `registerTools`** — one new line alongside the Phase 07/08 registrars.
- **`bridge.listPendingReviews` / `listRecentlyReviewed` / `openReview` in `internal/mcp/reviews.go`** — the 3 handler methods. The two list methods likely share one state-inspecting helper (only the projected field differs); `openReview` is a thin `bridge.call` wrapper.
- **`internal/mcp/reviews_test.go`** — happy + error per tool (07 D-08), incl. a state→IsError case (fake handler returning `{"state":"no_gh",...}` with HTTP 200) and an `open_review` 409 case.
- **`cmd/kamacu/main.go` is NOT modified** — Phase 06 already registers `mcpCmd`; Phase 09 only adds tools inside `internal/mcp/`. **No `internal/api/`, `internal/github/`, DB, migration, or frontend changes.**

</code_context>

<specifics>
## Specific Ideas

- **This is the milestone's simplest phase — pure translation, zero server changes.** Every other v1.11 phase touched `internal/api/` (07 added 2 endpoints; 08 added 3 + streaming) or introduced genuinely new capability (08's read-only terminal access). Phase 09 adds ONLY `internal/mcp/reviews.go` + `reviews_test.go` + one `registerTools` line. The v1.3/v1.5 gh integration does all the real work; the bridge just routes to it.
- **One cached call serves both queues — exploit it.** The two list tools hit the same URL; the second is cache-free. The cleanest shape is one shared list-helper that takes the queue name ("prs" vs "reviewed"), so the state→IsError logic lives in exactly one place. (Agent's discretion per the discretion section, but this is the natural factoring.)
- **The agent is the user; honesty over hand-holding.** D-02 (pure JSON on error, no prose map) deliberately mirrors the project's D-M001-2 / 08-D-05 posture: the `state` field names the problem faithfully; don't dress it up. The IsError flag is the actionable signal; the JSON is the actionable reason.
- **`project_id` required is a feature, not a compromise.** An agent lives in one project; making `project_id` required matches its scope and keeps every tool a single `bridge.call`. The REQUIREMENTS `?` was a roadmap-time API sketch; the per-repo reality refines it. Document the refinement in the tool description so agents know PRs are per-repo.
- **SC3 is satisfied by NOT bypassing — the bridge adds no gate logic.** The two-gate ladder runs server-side on every call (list GET returns `state:"disabled"`; `open_review` POST returns 409). The bridge surfaces these (IsError for lists, D-05 wrap for open_review); it never reaches `gh` when a gate is closed. No new gates, no bypass — pure translation.

</specifics>

<deferred>
## Deferred Ideas

- **Global / cross-project PR triage** — `list_pending_reviews()` / `list_recently_reviewed()` with no `project_id` enumerating all GitHub-linked projects and merging. D-04 makes `project_id` required instead. The aggregate path would be the first multi-call aggregation in the bridge (enumerate `GET /api/projects` → filter `github_repo != ''` → N cached GETs → merge), and `PRSummary` has no `repo`/`project` field so a useful merge would need per-PR provenance annotation (shape change). Revisit if agent usage shows a real need for global triage; until then agents loop over `list_projects`.
- **`refresh?` cache-bypass param on the list tools** — D-06 ships without it (rely on the 60s TTL + 10s floor). If an agent ever needs to force a fresh fetch, add an optional `refresh` boolean mapping to `?refresh=1` (still bound by the 10s attempt floor) in a later phase.
- **`get_pr` / `get_review_diff` MCP tool** — exposing `GET /api/projects/{id}/pull-requests/{n}` (live PR detail, `pullrequests.go:93`) or a diff view as an MCP tool. Not in MCPREV-01..03; the 3 shipped tools don't include a per-PR read. Future tool-category work (MCPMORE-adjacent) if agents need to read a single PR's detail/diff.
- **Trimmed `open_review` result** — D-05 chose raw `{task, pr}` passthrough. A trimmed envelope (`{task_id, worktree_path, pr_title, pr_url}`) was rejected as bridge-side shaping with no clear win. Revisit only if agents find the full envelope noisy.
- **MCPHARD-01..03** — typed bridge error taxonomy, stdout-pollution guards, real-binary e2e harness. Phase 09 surfaces generic MCP errors (D-05 wrap + D-01 IsError); the typed taxonomy + harness are v1.12, inherited.
- **Per-task auto-scoping (MCPAUTO)** — `get_my_project` so an agent resolves its own project from `KAMACU_SESSION_ID` with no `project_id` arg. Would make D-04's required `project_id` less burdensome. Deferred to v1.12.

None of these were pulled into Phase 09 — discussion stayed within the MCPREV-01/02/03 boundary.

</deferred>

---

*Phase: 09-pr-review-tools*
*Context gathered: 2026-07-23*
