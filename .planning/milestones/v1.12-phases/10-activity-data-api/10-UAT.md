---
status: complete
phase: 10-activity-data-api
source: [10-01-SUMMARY.md, 10-02-SUMMARY.md, 10-03-SUMMARY.md, 10-VERIFICATION.md]
started: 2026-07-30T14:01:13Z
updated: 2026-07-30T19:12:00Z
---

## Current Test

[testing complete]

## Tests

### 1. Live gh integration smoke against a real linked repo with reviewed-then-closed PRs

**Prerequisites:**
- `gh` CLI installed and authenticated (`gh auth status`).
- The `github_integration` setting is `"on"` (toggle in Settings, backed by migration 00007).
- At least one project linked to a GitHub repo where you have reviewed PRs that were later merged/closed.

**Steps:**
1. Start the server: `go run ./cmd/kamacu/serve.go` (or the built binary).
2. `curl -s -o /dev/null -w "%{http_code}\n" 'http://localhost:<port>/api/activity?scope=global&window=month'` → expect `200`.
3. `curl -s 'http://localhost:<port>/api/activity?scope=global&window=month' | jq '.reviews'` → expect `state` in `{ok, partial}` and a non-empty `prs[]` with each entry carrying `number`, `title`, `completedAt`, `url`, `projectName`, `projectId`, `repo`.
4. (Optional) Confirm window filter: `?window=week` drops PRs closed >7d ago; `?window=month` keeps PRs closed within 30d.

expected: HTTP 200 always; reviews.prs non-empty for a repo with reviewed-then-closed PRs; degradation (state=partial/no_gh) does not break tasks-done or stats when gh is absent.
result: pass

### 2. [auto] ReviewDoneSummary wire type (10-01 D4, REVIEWS-01)
expected: number/title/completedAt/url + provenance fields (projectName/projectId/repo) zero-valued for the aggregation loop.
result: pass
source: automated
coverage_id: 10-01-D4
verification: unit — TestParseCompletedReviews/maps_closedAt_to_CompletedAt

### 3. [auto] listCompletedReviews no-spawn-when-no-gh + parse (10-01 D1, REVIEWS-01)
expected: Available()=false short-circuits before any spawn; closedAt→CompletedAt mapping, desc sort, empty, malformed all handled.
result: pass
source: automated
coverage_id: 10-01-D1
verification: unit — TestParseCompletedReviews, TestListCompletedReviews/no_gh_returns_empty_+_no_spawn
note: live gh SPAWN path is the human checkpoint (test 1); classification/parse/no-spawn are unit-proven here.

### 4. [auto] classifyGhListError shared helper (10-01 D2, REVIEWS-04)
expected: auth_required vs error classification used by BOTH listPRs and listCompletedReviews (no drift).
result: pass
source: automated
coverage_id: 10-01-D2
verification: unit — TestClassifyGhListError; grep 3 refs (1 decl + 2 call sites)

### 5. [auto] GetMergedClosed 5min-TTL decoupled cache (10-01 D3, REVIEWS-01)
expected: Get gate-ladder shape (in-flight dedup + attemptFloor + drop-after-N), decoupled from review-column entries map (D-03).
result: pass
source: automated
coverage_id: 10-01-D3
verification: unit — TestGetMergedClosed (9 sub-tests); D-03 separation grep

### 6. [auto] GET /api/activity handler — scope/window/always-200 (10-02 D1, TASKS-01)
expected: combined {tasks,reviews,stats} for any global/workspace/project scope + week/month window; always HTTP 200; invalid params → 400.
result: pass
source: automated
coverage_id: 10-02-D1
verification: unit — TestActivityHandler_GlobalScopeAndWindow, InvalidScope, InvalidWindow; go build + vet exit 0; route registered once

### 7. [auto] GATE 1 toggle enforcement (10-02 D3, REVIEWS-04)
expected: github_integration != "on" → reviews.state="disabled" with ZERO gh spawns; tasks+stats still populate.
result: pass
source: automated
coverage_id: 10-02-D3
verification: unit — TestActivityHandler_Gate1Disabled; grep confirms disabled branch precedes aggregateReviews call

### 8. [auto] tasks-done SQL (10-02 D4, TASKS-01)
expected: status='done' AND source='manual' AND done_at>=? + scope clause, ORDER BY done_at DESC; scope id parameterized (D-12).
result: pass
source: automated
coverage_id: 10-02-D4
verification: unit — TestActivityHandler_WorkspaceScope, ProjectScope, WindowMonth; grep source='manual'/done_at>=?/ORDER BY

### 9. [auto] aggregateReviews concurrent fan-out + window filter (10-02 D2, REVIEWS-01)
expected: errgroup SetLimit(5) + per-repo 12s timeout; reviews window filter drops PRs with completedAt<cutoff; ok/partial/degraded rollup.
result: pass
source: automated
coverage_id: 10-02-D2
verification: unit — TestActivityHandler_ReviewsWindowFilter, Partial, NoGh, RefreshPropagation

### 10. [auto] in-Go cycle/dwell/median stats (10-02 D5, STATS-01)
expected: taskCount/reviewCount + cycle/dwellInProgress/dwellInReview {n,min,max,median} int64 seconds; reviews contribute count only.
result: pass
source: automated
coverage_id: 10-02-D5
verification: unit — TestActivityHandler_StatsOverSeededDB (hand-computed incl even-N median + Open Q3 fallback)

### 11. [auto] parseActivityTime mismatched-precision parser (10-03 D1, STATS-02)
expected: ms .000Z then bare-Z; returns (time,bool), false on empty/junk, never panics.
result: pass
source: automated
coverage_id: 10-03-D1
verification: unit — TestParseActivityTime

### 12. [auto] parseScope + parseWindow validation gate (10-03 D2, STATS-02)
expected: global/workspace:N/project:N; week/month; errors handler maps to 400.
result: pass
source: automated
coverage_id: 10-03-D2
verification: unit — TestParseScope (7 cases), TestParseWindow

### 13. [auto] computeTimeStat min/max/median (10-03 D3, STATS-02)
expected: int64 seconds, correct even-N median parity, no caller-slice mutation.
result: pass
source: automated
coverage_id: 10-03-D3
verification: unit — TestComputeTimeStat

### 14. [auto] buildDurationSlices cycle/dwell + per-slice NULL skip (10-03 D4, STATS-03)
expected: cycle/dwellInProgress/dwellInReview with Open Q3 dwellInProgress semantic; NULL/unparseable per-slice skip.
result: pass
source: automated
coverage_id: 10-03-D4
verification: unit — TestBuildDurationSlices (6 paths)

### 15. [auto] rollupReviewState ok/partial/degraded priority (10-03 D5, STATS-03)
expected: stable priority error>auth_required>no_gh (D-08 contract).
result: pass
source: automated
coverage_id: 10-03-D5
verification: unit — TestRollupReviewState (7 cases)

### 16. [auto] activity_helpers.go stdlib-only purity (10-03 D6, STATS-03)
expected: imports neither internal/github nor database/sql; import block {fmt,sort,strconv,strings,time}.
result: pass
source: automated
coverage_id: 10-03-D6
verification: grep — 0 'internal/github', 0 'database/sql'

## Summary

total: 16
passed: 16
issues: 0
pending: 0
skipped: 0
blocked: 0

## Coverage-block note

The three SUMMARY `coverage:` blocks used `kind: automated_procedural`, which is not in the classifier's enum (unit/integration/e2e/automated_ui/manual_procedural/other). This caused 6 already-green deliverables (10-01 D2/D3, 10-02 D1/D3/D4, 10-03 D6) to surface as `present[]` with `reason: validation_failed`. All have passing unit/grep verification and `human_judgment: false`; recorded above as `source: automated`. The SUMMARYs' coverage blocks should be fixed (`automated_procedural` → `manual_procedural` or `other`) at a convenient moment — non-blocking.

## Gaps
