---
phase: 09-pr-review-tools
status: passed
verified_at: 2026-07-28T10:52:23Z
score: 13/13 must-have truths verified (3/3 Success Criteria)
requirements_verified: MCPREV-01, MCPREV-02, MCPREV-03
next_action: Run `/gsd-verify-work 09` for conversational UAT, then milestone close.
---

# Verification: Phase 09 — PR Review Tools

## Summary

Phase 09 delivers its goal: three thin MCP tools (`list_pending_reviews`, `list_recently_reviewed`, `open_review`) that bridge Kamacu's existing v1.3/v1.5 `internal/github` HTTP surface with zero new endpoints, zero new `gh` calls, and zero new gates. The "reuses existing surface" claim — the heart of the phase — holds under direct inspection of the upstream code paths: both list tools hit the cached `GET /api/projects/{id}/pull-requests` endpoint via the shared `listReviews` helper (one `gh` cycle serves both queues), and `open_review` is a verbatim `bridge.call` POST to the v1.3 Phase 12 find-or-create `source='github_pr'` path. All 6 review tests pass, build/vet/full-binary are clean, and the clean-import streak is intact. One non-blocking Warning (WR-01: empty-queue success path emits `"null"` not `"[]"`) is unresolved but does not fail any Success Criterion.

## Requirement Traceability

| Requirement | Description | Status | Evidence |
|-------------|-------------|--------|----------|
| MCPREV-01 | `list_pending_reviews` — awaiting-review queue (`user-review-requested:@me draft:false`) | ✓ Verified | `reviews.go:90-108` (AddTool), `:283-291` (handler → `listReviews(...,"prs")`), `:209-272` (helper); upstream query `prlist.go:195` |
| MCPREV-02 | `list_recently_reviewed` — reviewed-by queue (`reviewed-by:@me draft:false`) | ✓ Verified | `reviews.go:114-132` (AddTool), `:297-305` (handler → `listReviews(...,"reviewed")`); upstream query `prlist.go:196` |
| MCPREV-03 | `open_review` — opens PR as review workspace (find-or-create `source='github_pr'`) | ✓ Verified | `reviews.go:140-162` (AddTool), `:328-338` (handler → `b.call(POST .../review)`); upstream find-or-create `pullrequests.go:191-193, 231-234` |

## Success Criteria Verification

### SC-1: list_pending_reviews + list_recently_reviewed

**Status:** ✓ Verified

**Evidence:**
- Both tools registered: `server.go:89` calls `registerReviewTools(s, b)`; `reviews.go:90-108` registers `list_pending_reviews`, `reviews.go:114-132` registers `list_recently_reviewed`.
- Bridge to existing HTTP API (not a new `gh` call): both handlers delegate to `listReviews` (`reviews.go:290`, `:304`), which calls `b.do(ctx, http.MethodGet, "/api/projects/%d/pull-requests", nil)` at `reviews.go:210-211`. The target endpoint is registered at `pullrequests.go:40`. No `gh` import in `reviews.go` (clean-import streak: 0 `kamacu/internal/*` matches).
- Rides the per-repo TTL cache (no N+1): the GET handler calls `svc.Get(...)` (`pullrequests.go:80` → `service.go:109`), which uses the `repoEntry` cache (`service.go:34-43`) with `cacheTTL=60s` / `attemptFloor=10s` (`service.go:87-88`). `fetchLists` (`prlist.go:293-304`) fetches BOTH queues in ONE `gh` cycle — the no-N+1 invariant is structural. The bridge makes one GET per tool and both share the same cached endpoint.
- gh query strings match SC-1 exactly: `searchAwaiting = "user-review-requested:@me draft:false"` (`prlist.go:195`); `searchReviewed = "reviewed-by:@me draft:false"` (`prlist.go:196`).
- D-03 queue projection: `reviews.go:263-266` picks `res.Reviewed` when `queue == "reviewed"`, else `res.PRs`. Tests `TestBridge_ListPendingReviews_Happy_*` (asserts prs entries 101/102 present, reviewed 201 absent) and `TestBridge_ListRecentlyReviewed_Happy_*` (asserts reviewed 201 present, prs 101/102 absent) confirm routing.
- `project_id` is REQUIRED (InputSchema `"required":[]string{"project_id"}` at `reviews.go:102`, `:126`). This is the locked D-04 refinement — the REQUIREMENTS `?` is deliberately dropped (documented in `09-CONTEXT.md` D-04 / `09-RESEARCH.md` Pitfall 4). Not a gap: a deliberate, user-locked decision that keeps every tool a single HTTP call.

**Notes:**
- D-04 (project_id required, not optional) is an intentional refinement of the roadmap signature — honored, not a deviation.

### SC-2: open_review

**Status:** ✓ Verified

**Evidence:**
- Registered: `reviews.go:140-162` (third `s.AddTool`, `Name: "open_review"`).
- Calls `POST /api/projects/{id}/pull-requests/{n}/review` via `bridge.call` verbatim: `reviews.go:336-337` — `path := fmt.Sprintf("/api/projects/%d/pull-requests/%d/review", ...)` then `return b.call(ctx, http.MethodPost, path, nil)`. `bridge.call` is the locked helper at `bridge.go:88-106` — no reimplementation.
- Test `TestBridge_OpenReview_Happy_PostsReviewPath` confirms `gotPath == "/api/projects/3/pull-requests/42/review"`, `gotMethod == POST`, and verbatim `{task,pr}` passthrough.
- Upstream reuses v1.3 Phase 12 find-or-create `source='github_pr'` path: POST handler at `pullrequests.go:148-268` — find existing (`:191-193` `WHERE project_id=? AND pr_number=? AND source='github_pr'`), insert new (`:231-234` `INSERT ... source='github_pr' ...`), `CheckoutPR` for worktree (`:256`), returns `{task, pr}` via `writeReview` (`:267`/`:308`).
- Does NOT bypass board-leak guard or duplicate detection: both run server-side in `pullrequests.go` (board-leak filter documented at `:189-190`, `:224-229`; find-or-create dedup key at `:191-193`). The bridge adds zero logic — confirmed by reading `reviews.go:328-338` (no gate checks, no `settings` references, no DB access).
- No agent auto-start (D-05): task inserted with `status='todo'` (`pullrequests.go:233`); the bridge passes `{task, pr}` through without triggering a session.

### SC-3: Graceful degrade

**Status:** ✓ Verified

**Evidence:**
- List tools return `IsError: true` with actionable raw body when state != ok: `reviews.go:249-259` — `if res.State != "ok" { return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}}, nil }`.
  - GitHub integration off → GET returns `state:"disabled"` (`pullrequests.go:55`) → IsError=true.
  - Project has no linked repo → GET returns `state:"disabled"` (`pullrequests.go:67`, `:75`) → IsError=true.
  - `gh` absent → `listPRs` returns `"no_gh"` (`prlist.go:213-214`), propagates through `Service.Get` → GET returns `state:"no_gh"` → IsError=true.
  - Test `TestBridge_ListPendingReviews_StateNoGh_IsErrorWithRawResult` confirms IsError=true + raw body containing `"state":"no_gh"`.
- D-02 raw body preserved verbatim: `reviews.go:256` `Text: string(body)` (the full Kamacu Result JSON, no prose prefix). Test asserts the `"state":"no_gh"` substring survives.
- Pitfall 1 (stale-cache edge) defended: IsError gated PURELY on `state != "ok"`, never on array nullness. Load-bearing test `TestBridge_ListPendingReviews_StateErrorWithNonNullArrays_IsErrorStillTrue` asserts IsError=true on `state:"error"` + non-null arrays + `stale:true`.
- `open_review` degrade via real HTTP error codes: gates block → POST returns 409 (`pullrequests.go:166`, `:175`, `:183`) → `bridge.call` wraps as error (`bridge.go:98-99`). Test `TestBridge_OpenReview_GateBlocked_Kamacu409` confirms non-nil error containing `"HTTP 409"` and `"GitHub integration is off"`.
- Two-gate ladder NOT bypassed: GATE 1 (settings toggle) + GATE 2 (project link) run server-side in `pullrequests.go` BEFORE any `gh` spawn (GET: `:49-77`; POST: `:160-185`). The bridge adds NO gate logic (no `settings` import, no gate checks in `reviews.go`). SC3 satisfied by NOT bypassing.

## Automated Checks

- `go test ./internal/mcp/ -run 'TestBridge_(ListPendingReviews|ListRecentlyReviewed|OpenReview)' -v -count=1`: ✓ PASS (6/6 tests — happy + error per tool, incl. Pitfall 1 stale-cache + 409 wrap).
- Full `internal/mcp` suite: ✓ PASS (`ok kamacu/internal/mcp 0.818s` — 67 tests total, no regression).
- `go build ./internal/mcp/... && go vet ./internal/mcp/...`: ✓ PASS (exit 0).
- `go build ./...` (full milestone binary): ✓ PASS.
- Clean-import streak (Pitfall 5): `go list` imports for `./internal/mcp` → 0 `kamacu/internal/*` matches; `grep -c 'kamacu/internal/{github,api,store}' reviews.go` → 0/0/0.
- D-12 (no stdout pollution): `grep -cE 'fmt\.Print(ln|f)?\(' reviews.go` → 0.
- No `SetError` / no `withRecover`: both grep counts 0.
- Code review (`09-REVIEW.md`): 3 findings (0 Critical, 1 Warning, 2 Info). **WR-01 (Warning, unresolved):** empty-queue success path emits `"null"` instead of `"[]"` because Go marshals a nil slice as JSON `null` — a degraded-but-not-broken experience when a user has zero PRs awaiting review. Does not block `passed` (Warnings are non-blocking per verification rules). IN-01/IN-02 (Info): queue discriminator fallthrough and test index-without-length-guard — both consistent with existing codebase conventions.

## Gaps

None — all three Success Criteria are verified with file:line evidence. The one unresolved Warning (WR-01) is an edge-case fidelity issue (empty queue renders `null` not `[]`), not a Success Criterion failure: the tools list PRs correctly, project the right queue, and degrade gracefully. Recommend addressing WR-01 in a follow-up (coerce nil `json.RawMessage` to `[]byte("[]")` on the success branch only) but it does not block phase completion.

## Notes

- D-04 (project_id required, dropping the REQUIREMENTS `?`) is a locked user decision documented in `09-CONTEXT.md` and `09-DISCUSSION-LOG.md:58`. Honored as specified — not a deviation.
- The phase is a pure HTTP bridge; end-to-end testing against real `gh`/GitHub is out of scope (the upstream `internal/github` + `internal/api/pullrequests.go` surface carries its own v1.3/v1.5 coverage). The bridge-level httptest suite fully covers the SCs.
