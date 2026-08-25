---
phase: 09-pr-review-tools
plan: 01
subsystem: api
tags: [mcp, go, http-bridge, pr-review, github-integration, json-rpc]

# Dependency graph
requires:
  - phase: 06-mcp-server
    provides: bridge.call / bridge.do helpers, the low-level AddTool + map[string]any InputSchema pattern, the X-Kamacu-Token wire contract, maxBodyBytes cap, slog-to-stderr (D-12)
  - phase: 07-tasks-projects-workspaces-tools
    provides: the per-resource registrar split (D-07), the int64-arg + fmt.Sprintf path template (tasks.go), the two-required-args InputSchema (move_task), the httptest test scaffold + newCallToolRequest helper
  - phase: 08-session-output-tools
    provides: the scoped-divergence-from-bridge.call precedent (subscribeSessionOutput calls b.do directly and builds its own CallToolResult) — the shape listReviews clones
provides:
  - "3 MCP tools (list_pending_reviews, list_recently_reviewed, open_review) bridging Kamacu's existing v1.3/v1.5 GitHub-integration HTTP surface — the LAST tools of the v1.11 milestone"
  - "registerReviewTools(s, b) — the per-resource registrar wired into server.go's registerTools (one-line integration)"
  - "listReviews(ctx, projectID, queue) — the shared state-inspecting helper (the IsError hybrid D-01/D-02/D-03): one cached GET call site serves both review queues (v1.5 no-N+1 invariant)"
  - "openReview — a one-line bridge.call POST wrapper (D-05 verbatim passthrough)"
affects: [v1.11-milestone-close, agent-cli-review-workflow, verify-work-uat]

# Tech tracking
tech-stack:
  added: []  # zero new dependencies — pure translation over existing endpoints
  patterns:
    - "IsError hybrid (D-01/D-02/D-03): a bridge handler that inspects a JSON body field (state) to set CallToolResult.IsError on a 2xx response, diverging from bridge.call which treats 2xx as unconditional success. Modeled on subscribeSessionOutput's scoped divergence. IsError gated PURELY on state != ok, never on array nullness (Pitfall 1 stale-cache edge)."
    - "Shared-helper queue projection: two MCP tools (list_pending_reviews / list_recently_reviewed) delegate to ONE helper with a queue parameter — one gh cycle serves both review queues per repo (v1.5 no-N+1 invariant satisfied by construction)."
    - "Clean-import streak (Pitfall 5): the state decode uses a LOCAL anonymous struct with json.RawMessage fields, NOT the domain Result type — internal/mcp stays type-decoupled from internal/github, internal/api, internal/store. Negative-grep sentinel enforced (zero 'internal/github' tokens in reviews.go, including doc comments)."

key-files:
  created:
    - internal/mcp/reviews.go
    - internal/mcp/reviews_test.go
  modified:
    - internal/mcp/server.go

key-decisions:
  - "listPendingReviews and listRecentlyReviewed share ONE cached GET call site (listReviews) — one gh cycle per repo serves both queues (v1.5 no-N+1 invariant by construction)."
  - "listReviews diverges from bridge.call (the only other diverging handler is subscribeSessionOutput) because the GET endpoint ALWAYS writes HTTP 200 and carries degradation in the github.Result state field; bridge.call would treat that 200 as success and lose the degrade signal."
  - "IsError is set PURELY on state != ok — never gated on prs/reviewed array nullness (Pitfall 1: internal/github resultLocked carries cached arrays independently of state, so a transient error after prior success returns non-null arrays + stale:true)."
  - "openReview is a one-line bridge.call wrapper (D-05) — the POST endpoint returns real HTTP error codes (409/502/400) that bridge.call wraps faithfully; the bridge adds NO gate logic and cannot reach gh when a gate is closed (SC3 satisfied by NOT bypassing the two-gate ladder)."
  - "project_id is REQUIRED (not optional) on all three tools; open_review additionally requires pr_number (D-04). No refresh/force-fetch parameter on any tool (D-06 — freshness rides the existing 60s per-repo cache TTL)."
  - "Doc comments refer to in-repo packages descriptively (the github integration, the api package) rather than by import path, so the Pitfall 5 negative-grep sentinel (grep -c internal/github reviews.go == 0) is unambiguous for future audits."

patterns-established:
  - "IsError hybrid: when an endpoint always returns 2xx and carries degradation in a body field, diverge from bridge.call and set CallToolResult.IsError on the field value — return the raw body verbatim as TextContent (D-02) so the agent can self-correct."
  - "Shared-helper queue projection: when two tools hit the same endpoint and project different fields, factor ONE helper taking a queue discriminator — avoids N+1 gh calls."
  - "Local anonymous struct decode with json.RawMessage fields: bridge a typed wire contract WITHOUT importing the domain package (preserves the internal/mcp clean-import streak; re-marshal the chosen field verbatim)."

requirements-completed: [MCPREV-01, MCPREV-02, MCPREV-03]

# Coverage metadata (#1602) — one entry per shipped deliverable.
coverage:
  - id: D1
    description: "list_pending_reviews MCP tool (MCPREV-01): bridges GET /api/projects/{id}/pull-requests, projects the prs (awaiting-review) queue on state:ok, returns raw Result JSON + IsError=true on degrade."
    requirement: MCPREV-01
    verification:
      - kind: unit
        ref: internal/mcp/reviews_test.go#TestBridge_ListPendingReviews_Happy_StateOk_ProjectsPrsArray
        status: pass
      - kind: unit
        ref: internal/mcp/reviews_test.go#TestBridge_ListPendingReviews_StateNoGh_IsErrorWithRawResult
        status: pass
      - kind: unit
        ref: internal/mcp/reviews_test.go#TestBridge_ListPendingReviews_StateErrorWithNonNullArrays_IsErrorStillTrue
        status: pass
    human_judgment: false
  - id: D2
    description: "list_recently_reviewed MCP tool (MCPREV-02): same GET endpoint, projects the reviewed queue; shares the listReviews helper (one gh cycle per repo)."
    requirement: MCPREV-02
    verification:
      - kind: unit
        ref: internal/mcp/reviews_test.go#TestBridge_ListRecentlyReviewed_Happy_StateOk_ProjectsReviewedArray
        status: pass
    human_judgment: false
  - id: D3
    description: "open_review MCP tool (MCPREV-03): bridges POST /api/projects/{id}/pull-requests/{n}/review, returns raw {task,pr} envelope verbatim (D-05), wraps 409/502/400 via bridge.call."
    requirement: MCPREV-03
    verification:
      - kind: unit
        ref: internal/mcp/reviews_test.go#TestBridge_OpenReview_Happy_PostsReviewPath
        status: pass
      - kind: unit
        ref: internal/mcp/reviews_test.go#TestBridge_OpenReview_GateBlocked_Kamacu409
        status: pass
    human_judgment: false

# Metrics
duration: 7 min
completed: 2026-07-23
status: complete
---

# Phase 9 Plan 1: PR Review Tools Summary

**3 MCP tools (list_pending_reviews, list_recently_reviewed, open_review) bridging Kamacu's existing v1.3/v1.5 GitHub-integration HTTP surface — the milestone's mechanical coda: pure translation, zero new endpoints/gh-calls/gates**

## Performance

- **Duration:** 7 min
- **Started:** 2026-07-23T15:56:06Z
- **Completed:** 2026-07-23T16:02:42Z
- **Tasks:** 2
- **Files modified:** 3 (2 created, 1 modified one line)

## Accomplishments
- Shipped the last 3 MCP tools of v1.11 — every Kamacu surface the milestone targets (tasks / projects / workspaces / sessions / reviews) is now reachable from an agent CLI via a tool call.
- listReviews shared helper (the IsError hybrid D-01/D-02/D-03): one cached GET call site serves BOTH review queues per repo (v1.5 no-N+1 invariant by construction); on state != ok returns the raw Result JSON verbatim with IsError=true so the agent can self-correct.
- Pitfall 1 (stale-cache edge) defended by a load-bearing test: state:error + non-null arrays + stale:true still yields IsError=true (a regression to nil-gating would be caught).
- Clean-import streak preserved (Pitfall 5): reviews.go imports ONLY stdlib + the MCP SDK — zero coupling to internal/github, internal/api, internal/store (verified by both `go list` import graph AND the negative-grep sentinel).
- Phase 06 D-12 (slog-to-stderr, no stdout pollution) intact: zero fmt.Print* calls in reviews.go.

## Task Commits

Each task was committed atomically:

1. **Task 1: Create internal/mcp/reviews.go + wire server.go** - `3af8988` (feat)
2. **Task 2: Create internal/mcp/reviews_test.go (6 tests)** - `677c724` (test)
3. **Plan-level fix: reword doc comments for Pitfall 5 grep sentinel** - `d8861e6` (fix)

_Plus the plan-metadata commit (below)._

## Files Created/Modified
- `internal/mcp/reviews.go` (NEW) - 3 review tool handlers + registerReviewTools registrar + listReviews shared helper (the IsError hybrid) + openReview one-line bridge.call wrapper. Imports only stdlib + MCP SDK.
- `internal/mcp/reviews_test.go` (NEW) - 6 tests (happy + error per tool, 07 D-08), reusing the workspaces_test.go / bridge_test.go scaffold and newCallToolRequest.
- `internal/mcp/server.go` (MODIFIED) - one new line: `registerReviewTools(s, b)` as the final call in registerTools (07 D-07 per-resource split).

## Decisions Made
- listReviews diverges from bridge.call (modeled on subscribeSessionOutput) because the GET endpoint always returns HTTP 200 with degradation in the body's state field — bridge.call would lose that signal.
- IsError gated PURELY on state != ok (never on array nullness) — Pitfall 1 stale-cache edge documented and pinned by a dedicated regression test.
- openReview delegates verbatim to bridge.call (D-05) — the POST endpoint's real HTTP error codes (409/502/400) are the correct degrade surface; the bridge adds no gate logic.
- Doc comments refer to in-repo packages descriptively to keep the Pitfall 5 negative-grep sentinel unambiguous.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Reworded doc comments to satisfy the Pitfall 5 negative-grep sentinel**
- **Found during:** Plan-level verification (after Task 1 + Task 2 commits)
- **Issue:** The plan's verification gate 5 (`grep -c 'internal/github' internal/mcp/reviews.go` returns 0) and success criterion ("Pitfall 5 defended: grep gate returns 0") tripped on doc-comment references to the package by import path (e.g., "internal/github HTTP surface", "internal/github.Service"). The functional requirement was ALWAYS met — `go list` confirmed zero imports of kamacu/internal/{github,api,store} before any edits — but the literal grep sentinel matched the documentation prose. Same class as the Task-1-time `SetError` comment (which I reworded before the Task 1 commit).
- **Fix:** Reworded 6 doc-comment references to use descriptive package names ("the github integration", "the api package", "the domain Result type") instead of import paths. Documentation intent fully preserved (the comments still explain WHY the file is decoupled and WHAT it bridges to). No behavior change — doc-only.
- **Files modified:** internal/mcp/reviews.go
- **Verification:** `grep -c 'internal/github' internal/mcp/reviews.go` → 0; `grep -c 'internal/api'` → 0; `grep -c 'internal/store'` → 0; `go list` import graph unchanged (still 0 domain imports); build + vet + all 6 tests still pass.
- **Committed in:** d8861e6

**2. [Rule 3 - Blocking] Reworded the IsError struct-field comment to avoid the bare `SetError` token**
- **Found during:** Task 1 acceptance-criteria verification
- **Issue:** The acceptance gate `grep -c 'SetError' internal/mcp/reviews.go` returns 0 tripped on a doc comment explaining why IsError is set as a struct field ("NEVER via SetError"). No code ever called SetError.
- **Fix:** Reworded the comment to "IsError is set as a struct field directly. D-02 requires the raw Kamacu Result JSON to be preserved verbatim in Content; no result-mutator helper is used" — preserves the documentation without the bare token.
- **Files modified:** internal/mcp/reviews.go (part of the Task 1 commit 3af8988)
- **Verification:** `grep -c 'SetError' internal/mcp/reviews.go` → 0.
- **Committed in:** 3af8988 (part of Task 1)

---

**Total deviations:** 2 auto-fixed (2 blocking — both doc-comment rewords to satisfy literal negative-grep regression sentinels; zero behavior change, zero scope creep).
**Impact on plan:** Negligible — both fixes are doc-only and preserve the plan's documentation intent while making the regression-sentinel grep gates unambiguous for future audits. The functional invariants (no domain-package imports, no SetError calls) were met from the first commit.

## Issues Encountered
None - plan executed exactly as written. The two deviations above are doc-comment rewords discovered during acceptance-grep verification, not implementation problems.

## User Setup Required
None - no external service configuration required. The tools bridge Kamacu's existing loopback HTTP API; no new env vars, endpoints, DB/migration/frontend changes.

## Next Phase Readiness
- Phase 09 is the final phase of v1.11. With MCPREV-01/02/03 closed, every Kamacu surface the milestone targets is reachable from an agent CLI.
- Ready for `/gsd-verify-work 09` (UAT) and milestone close (`/gsd-complete-milestone`).
- No blockers. The full milestone binary `go build ./...` exits 0; the full internal/mcp test suite is green (no regression).

## Plan-Level Verification Results

All gates from the plan's `<verification>` and `<success_criteria>` sections:

| Gate | Result |
|------|--------|
| 1. `go build ./internal/mcp/... && go vet ./internal/mcp/...` | PASS (exit 0) |
| 2. `go test ./internal/mcp/ -count=1` (full suite, no regression) | PASS (ok, 0.73s) |
| 3. `go build ./...` (full milestone binary) | PASS (exit 0) |
| 4. `registerReviewTools(s, b)` wired in server.go | PASS (server.go:89, final call in registerTools) |
| 5. SC3 Pitfall 5: `grep -c 'internal/github' reviews.go` | PASS (0) |
| 6. D-12: `grep -cE 'fmt\.Print(ln\|f)?\(' reviews.go` | PASS (0) |
| D-01: `res.State != "ok"` branch present | PASS (1) |
| D-03: queue pick `queue == "reviewed"` | PASS (code branch at reviews.go:260) |
| D-04: list tools `required:[project_id]` (×2) | PASS (2) |
| D-04: open_review `required:[project_id,pr_number]` | PASS (1) |
| D-05: openReview `return b.call(ctx, http.MethodPost, path, nil)` | PASS (1) |
| D-06: no `refresh` param | PASS (0) |
| Pitfall 1: no `res.(PRs\|Reviewed) == nil` gating | PASS (0) |
| Pitfall 5: import graph (github/api/store) | PASS (0/0/0) |
| No `withRecover` (matches tasks.go/workspaces.go) | PASS (0) |
| No `SetError` | PASS (0) |
| 6 named test functions, all passing | PASS |

## Self-Check: PASSED

- [x] `internal/mcp/reviews.go` exists (package mcp) — FOUND
- [x] `internal/mcp/reviews_test.go` exists (package mcp) — FOUND
- [x] `internal/mcp/server.go` contains `registerReviewTools(s, b)` — FOUND (server.go:89)
- [x] Commit `3af8988` exists — FOUND (feat: 3 review tools + wiring)
- [x] Commit `677c724` exists — FOUND (test: 6 tests)
- [x] Commit `d8861e6` exists — FOUND (fix: doc-comment reword for Pitfall 5 sentinel)
- [x] All 6 tests pass — VERIFIED
- [x] Full milestone `go build ./...` exits 0 — VERIFIED

---
*Phase: 09-pr-review-tools*
*Completed: 2026-07-23*
