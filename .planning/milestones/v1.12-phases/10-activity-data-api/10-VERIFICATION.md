---
phase: 10-activity-data-api
verified: 2026-07-30T16:05:00Z
status: passed
score: 5/5 must-haves verified
behavior_unverified: 0
overrides_applied: 0
human_verification:

  - test: "Live gh integration smoke against a real linked repo with reviewed-then-closed PRs"
    expected: "curl -s 'http://localhost:<port>/api/activity?scope=global&window=month' returns HTTP 200 with non-empty reviews.prs (each carrying number/title/completedAt/url + projectName/projectId/repo), reviews.state in {ok,partial}; confirms the verified gh search 'reviewed-by:@me draft:false is:closed' + --limit 100 + closedAt→completedAt mapping returns real merged+closed data end-to-end"
    why_human: "The ok/auth_required/error SPAWN paths of listCompletedReviews are not unit-tested with a live gh (consistent with the existing listPRs pattern from v1.3/v1.5 — only Config.CompletedRunner is the test seam). Classification is proven by TestClassifyGhListError; the closedAt→completedAt transformation by TestParseCompletedReviews; the no-spawn-on-no_gh path by TestListCompletedReviews/no_gh; but the real gh JSON output shape for the verified search can only be confirmed by a live spawn. CONTEXT.md flags this as 'the single riskiest detail'. PLAN 10-02's verification section explicitly lists this as a post-execute manual smoke."
---

# Phase 10: Activity Data & API Verification Report

**Phase Goal:** The backend answers "which tasks were done, which reviews were completed (merged/closed), and what are the cycle/dwell stats" for any Global / Workspace / Project scope and a Week (7d) / Month (30d) window — including the new merged/closed `reviewed-by:@me` `gh` search dimension and graceful degradation when `gh` is absent.
**Verified:** 2026-07-30T16:05:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

The phase goal is achieved at the code level: every Success Criterion is backed by substantive, wired, behaviorally-tested artifacts. One human-verification item remains — a live gh integration smoke — because the real gh spawn path of `listCompletedReviews` cannot be unit-tested (consistent with the `listPRs` precedent) and CONTEXT.md flags the merged/closed search as the phase's riskiest detail.

### Observable Truths

Must-haves sourced from ROADMAP.md § Phase 10 Success Criteria (the contract), cross-checked against the granular truths in 10-01/10-02/10-03 PLAN frontmatter (all granular truths roll up to these five).

| # | Truth (ROADMAP Success Criterion) | Status | Evidence |
|---|-----------------------------------|--------|----------|
| 1 | Activity endpoint returns every task completed (moved to Done) within scope+window, each with project name, title, `done_at` | ✓ VERIFIED | `fetchActivityTasks` (activity.go:130) runs `WHERE status='done' AND source='manual' AND done_at IS NOT NULL AND done_at >= ?` + structural scope clause + `ORDER BY t.done_at DESC`; TestActivityHandler_GlobalScopeAndWindow asserts in-window present, out-of-window + github_pr excluded, done_at DESC order |
| 2 | Activity endpoint returns PRs reviewed-by:@me MERGED/CLOSED in window, with PR number/title/merge-close date, backed by NEW gh search aggregated across in-scope projects | ✓ VERIFIED | `listCompletedReviews` (prlist.go:428) builds `gh pr list --search "reviewed-by:@me draft:false is:closed" --state closed --limit 100 --json number,title,closedAt,url`; `aggregateReviews` (activity.go:59) fans out via errgroup SetLimit(5) + per-repo 12s timeout, annotates provenance, applies completedAt<cutoff window filter; TestActivityHandler_ReviewsWindowFilter (10d PR dropped under week, kept under month) + TestActivityHandler_Partial (provenance asserted). **Live gh spawn not unit-tested** → human smoke item below |
| 3 | When gh unavailable / integration off / repo fetch fails, reviews-done degrades without breaking tasks-done or stats (degrade-don't-break) | ✓ VERIFIED | Handler ALWAYS writes HTTP 200 (activity.go:238,273 — writeJSON OK); only 400 on invalid params (activity.go:217,222); GATE 1 short-circuits to `reviews.state="disabled"` BEFORE any spawn (activity.go:260 precedes aggregateReviews at :268); TestActivityHandler_Gate1Disabled (0 spawns, tasks+stats populate) + TestActivityHandler_NoGh (all degrade) + TestActivityHandler_Partial (ok+no_gh→partial) |
| 4 | Activity endpoint returns counts of tasks done and reviews done within scope+window | ✓ VERIFIED | `statsBlock.TaskCount` = len(tasks) (activity.go:246); `statsBlock.ReviewCount` = len(reviews.PRs) (activity.go:272); TestActivityHandler_StatsOverSeededDB asserts TaskCount=2, ReviewCount=0 |
| 5 | Activity endpoint returns min/max/median cycle (In-Progress→Done) + per-column dwell (In-Progress, In-Review) for tasks done; reviews contribute count only | ✓ VERIFIED | `computeTimeStat` (activity_helpers.go:84) — odd-N middle, even-N two-middle avg, int64 seconds; `buildDurationSlices` (activity_helpers.go:106) — cycle=done−inProg, dwellInReview=done−inRev, dwellInProgress=inRev−inProg (Open Q3) or done−inProg fallback; TestActivityHandler_StatsOverSeededDB asserts hand-computed cycle{7200,10800,med=9000}/dwellInProgress{3600,7200,med=5400}/dwellInReview{7200}; TestComputeTimeStat even-N parity; reviews carry no cycle/dwell object (STATS-04 — reviewCount is plain int) |

**Score:** 5/5 truths verified (0 present, behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/github/prlist.go` | `ReviewDoneSummary`, `searchCompletedReviews`, `completedRaw`, `classifyGhListError`, `parseCompletedReviews`, `listCompletedReviews`; `listPRs` refactored to call `classifyGhListError` | ✓ VERIFIED | All symbols present; `is:closed` in const (1), `--state closed`/`--limit 100`/`number,title,closedAt,url` in spawn (1 each); `classifyGhListError` called from both listPRs (:230) and listCompletedReviews (:444) — 3 refs total (1 decl + 2 sites); provenance fields zero-valued by GetMergedClosed (TestParseCompletedReviews asserts) |
| `internal/github/service.go` | `mergedClosedTTL=5min`, `mergedClosedEntry`, `MergedClosedResult`, `Config.CompletedRunner`, `Service.completed` map + `completedRunner` field, `entryMergedClosed`, `GetMergedClosed` method | ✓ VERIFIED | `mergedClosedTTL = 5 * time.Minute` (:140); `cacheTTL = 60 * time.Second` unchanged (:137); `GetMergedClosed` method (:280) with gate ladder mirroring Get; D-03 separation verified by grep (GetMergedClosed body 0 `s.entries` refs; Get body 0 `s.completed` refs) AND by behavioral test (TestGetMergedClosed/D-03); never assigns "disabled" (grep 0 in method body) |
| `internal/api/activity_helpers.go` | `activityTask`/`timeStat`/`statsBlock` wire types + `parseActivityTime`/`parseScope`/`parseWindow`/`computeTimeStat`/`buildDurationSlices`/`rollupReviewState` (stdlib-only) | ✓ VERIFIED | All 6 helpers + 3 types present; import block = {fmt, sort, strconv, strings, time}; purity grep: 0 `internal/github`, 0 `database/sql`; parseActivityTime dual-layout (ms .000Z then bare-Z); TestParseActivityTime/Scope/Window/ComputeTimeStat/BuildDurationSlices/RollupReviewState all PASS (30 sub-tests) |
| `internal/api/activity.go` | `aggregateReviewsResult`/`activityResponse`/`repoRow` wire types; `aggregateReviews` (errgroup + window filter + rollup); `fetchActivityTasks`; `fetchLinkedRepos`; `ActivityRoutes` handler + registrar | ✓ VERIFIED | All present; `aggregateReviews` signature with `cutoff time.Time` (:59); pure helpers NOT redeclared (grep 0); `source = 'manual'` (1), `done_at >= ?` (1), `ORDER BY t.done_at DESC` (1); cutoff layout literal `"2006-01-02T15:04:05.000Z"` (1); errgroup+SetLimit(5)+WithTimeout(12s); GATE 1 disabled branch (:260) precedes aggregateReviews (:268) |
| `internal/api/activity_test.go` | 12 hermetic integration tests (scope/window variants, GATE 1, degrade rollup, reviews window filter, stats, refresh, 400s) | ✓ VERIFIED | 12 TestActivityHandler_* functions present + PASS; no real gh spawns (fake CompletedRunner); TestActivityHandler_ReviewsWindowFilter asserts cutoff threading + second-precision closedAt parsing; TestActivityHandler_Partial asserts provenance (projectName/projectId/repo) on surviving PRs |
| `cmd/kamacu/serve.go` | `api.ActivityRoutes(mux, db, ghSvc)` registered alongside PullRequestRoutes | ✓ VERIFIED | Present at :270, immediately after `api.PullRequestRoutes` (:269); same shared ghSvc (no new github.New) |

**Artifacts:** 6/6 verified

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| `ActivityRoutes` (activity.go:211) | `(*github.Service).GetMergedClosed` | `aggregateReviews` calls `ghSvc.GetMergedClosed` per in-scope repo (activity.go:79) | ✓ WIRED | One call per linked repo, annotated with provenance, window-filtered, rolled up |
| Handler (activity.go:215) | `parseScope`/`parseWindow` (activity_helpers.go) | imported (same package), errors → 400 | ✓ WIRED | parseScope/parseWindow called at :215,:220; writeError 400 on err |
| Handler (activity.go:244) | `buildDurationSlices`/`computeTimeStat` (activity_helpers.go) | imported, results → statsBlock | ✓ WIRED | :244 buildDurationSlices; :247-249 computeTimeStat per slice |
| GATE 1 (activity.go:256) | `settings.Get(db, KeyGithubIntegration)` | short-circuits to "disabled" before aggregateReviews | ✓ WIRED | :256 settings.Get; :260 disabled; :268 aggregateReviews (only in else branch) |
| Handler (activity.go:232) | reaper's ms-ISO cutoff format | `cutoffTime.Format("2006-01-02T15:04:05.000Z")` | ✓ WIRED | Same format as reaper done_at storage; cutoffTime threaded into aggregateReviews (:268) |
| `listCompletedReviews` (prlist.go:428) | `classifyGhListError` (prlist.go:362) | shared with listPRs (no drift) | ✓ WIRED | classifyGhListError called from listPRs (:230) + listCompletedReviews (:444) |
| serve.go (:270) | `api.ActivityRoutes` | route registration | ✓ WIRED | Registered once, after PullRequestRoutes |

**Wiring:** 7/7 connections verified

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|----|
| `fetchActivityTasks` | `tasks []activityTask` | SQLite `tasks JOIN projects WHERE status='done' AND source='manual' AND done_at>=?` | Yes (real DB query, parameterized) | ✓ FLOWING |
| `aggregateReviews` | `reviews.PRs []ReviewDoneSummary` | `ghSvc.GetMergedClosed` per linked repo (fake runner in tests; real gh in prod) | Yes (test: fakeCompletedRunner returns canned PRs; prod: real gh spawn) | ✓ FLOWING (test-verified; prod gh = human smoke) |
| `statsBlock` | `Cycle/DwellInProgress/DwellInReview timeStat` | `computeTimeStat(buildDurationSlices(tasks))` | Yes (computed from real *_at timestamps) | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| `go build ./...` | `go build ./...` | exit 0 | ✓ PASS |
| `go vet` on touched packages | `go vet ./internal/github/ ./internal/api/ ./cmd/kamacu/` | exit 0 | ✓ PASS |
| github package tests (Plan 01) | `go test ./internal/github/ -count=1` | ok kamacu/internal/github 0.117s (TestGetMergedClosed 9 sub-tests + TestListCompletedReviews + TestClassifyGhListError + TestParseCompletedReviews + pre-existing) | ✓ PASS |
| activity pure-helper tests (Plan 03) | `go test ./internal/api/ -run 'TestParseActivityTime\|TestParseScope\|TestParseWindow\|TestComputeTimeStat\|TestBuildDurationSlices\|TestRollupReviewState' -count=1` | 6 functions, 30 sub-tests, all PASS | ✓ PASS |
| activity handler tests (Plan 02) | `go test ./internal/api/ -run 'TestActivityHandler' -count=1` | 12 tests all PASS (GlobalScopeAndWindow, WorkspaceScope, ProjectScope, WindowMonth, ReviewsWindowFilter, Gate1Disabled, NoGh, Partial, StatsOverSeededDB, RefreshPropagation, InvalidScope, InvalidWindow) | ✓ PASS |

Note: The full `go test ./internal/api/...` suite hangs (>120s) due to pre-existing integration tests with real gh/PTY spawns — a pre-existing condition documented in 10-02-SUMMARY and 10-03-SUMMARY, NOT a Phase 10 defect. Verification used scoped `-run` filters per the runtime guidance.

### Probe Execution

N/A — no `scripts/*/tests/probe-*.sh` probes declared or conventional for this phase.

## Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| TASKS-01 | 10-02 | User can see a list of tasks completed within the selected window and scope | ✓ SATISFIED | `fetchActivityTasks` (source='manual' + done_at>=? + scope clause + ORDER BY done_at DESC); TestActivityHandler_GlobalScopeAndWindow/WorkspaceScope/ProjectScope |
| REVIEWS-01 | 10-01, 10-02 | User can see PRs reviewed-by:@me MERGED/CLOSED within window+scope, with merge/close date | ✓ SATISFIED | `listCompletedReviews` (is:closed + --limit 100 + closedAt→completedAt) + `aggregateReviews` (window filter + provenance); TestActivityHandler_ReviewsWindowFilter. Live gh smoke pending (human item) |
| REVIEWS-04 | 10-01, 10-02 | Reviews-done degrades gracefully when gh unavailable/repo fetch fails (never blocks the page) | ✓ SATISFIED | GATE 1 disabled short-circuit + per-repo degrade + always-200 + errgroup nil-on-degrade; TestActivityHandler_Gate1Disabled/NoGh/Partial |
| STATS-01 | 10-02 | Count of tasks done and reviews done within window+scope | ✓ SATISFIED | statsBlock.TaskCount + ReviewCount; TestActivityHandler_StatsOverSeededDB |
| STATS-02 | 10-02, 10-03 | min/max/median cycle time (In Progress→Done) | ✓ SATISFIED | computeTimeStat + buildDurationSlices cycle; TestComputeTimeStat (even-N median parity) + TestActivityHandler_StatsOverSeededDB (cycle{7200,10800,med=9000}) |
| STATS-03 | 10-02, 10-03 | min/max/median dwell per column (In Progress, In Review) | ✓ SATISFIED | buildDurationSlices dwellInProgress (Open Q3 semantic) + dwellInReview; TestBuildDurationSlices + TestActivityHandler_StatsOverSeededDB (dwellInProgress{3600,7200,med=5400}/dwellInReview{7200}) |
| STATS-04 | 10-02 | Cycle/dwell covers tasks only; reviews contribute count, not time stats | ✓ SATISFIED | reviewCount is plain int; reviews sub-object carries no cycle/dwell; TestActivityHandler_StatsOverSeededDB asserts reviewCount=0 with no time-stat object under reviews |

**Coverage:** 7/7 requirements satisfied. No orphaned requirements (REQUIREMENTS.md traceability table maps exactly TASKS-01, REVIEWS-01, REVIEWS-04, STATS-01..04 to Phase 10 — all accounted for).

## Decision Coverage

All 12 trackable CONTEXT.md decisions (D-01 through D-12) are honored by shipped artifacts (`gsd-tools check.decision-coverage-verify`: 12/12 honored, 0 not_honored). Non-blocking gate.

## Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| internal/api/activity.go | 128, 175 | `placeholder` (in comments referring to SQL `?` parameterized placeholder) | ℹ️ Info | Not a stub — the word "placeholder" describes the parameterized `?` bind mechanism, not missing content |
| internal/github/prlist_test.go | 242 | `t.Skipf` in `exitErrOf` helper | ℹ️ Info | Defensive skip if `sh -c 'exit N'` exits 0 unexpectedly (cannot construct *exec.ExitError); runs normally on Linux/macOS where sh is present; not a disabled requirement test |

**Anti-patterns:** 0 blockers, 0 warnings, 2 info. No TBD/FIXME/XXX/TODO/HACK markers in any Phase 10 file.

## Human Verification Required

### 1. Live gh integration smoke (REVIEWS-01 end-to-end)

**Test:** With `gh` installed and authenticated, and at least one linked project whose repo has PRs you reviewed that merged or closed in the last 30 days, start the Kangent server and run:

```
curl -s 'http://localhost:<port>/api/activity?scope=global&window=month' | jq
```

**Expected:** HTTP 200 with a JSON body `{tasks, reviews, stats}` where `reviews.state` is `ok` or `partial` and `reviews.prs` is non-empty — each PR carrying `number`, `title`, `completedAt` (the merge/close date), `url`, and provenance (`projectName`, `projectId`, `repo`). Also verify `?window=week` drops PRs closed >7d ago (window filter against real gh closedAt).
**Why human:** The ok/auth_required/error SPAWN paths of `listCompletedReviews` are not unit-testable with a live gh — consistent with the existing `listPRs` pattern from v1.3/v1.5 (only `Config.CompletedRunner` is the test seam). Unit tests prove: the gh command is built correctly (grep: `--search "reviewed-by:@me draft:false is:closed" --state closed --limit 100 --json number,title,closedAt,url`), the failure classification is correct (TestClassifyGhListError), the closedAt→completedAt transformation is correct (TestParseCompletedReviews), and the no_gh path never spawns (TestListCompletedReviews/no_gh). But the real gh JSON output shape for the verified search — and the claim that `is:closed` stably includes merged per GitHub search docs — can only be confirmed by a live spawn. CONTEXT.md flags this as "the single riskiest detail" of the phase. PLAN 10-02's verification section explicitly lists this as a post-execute manual smoke.

## Gaps Summary

**No code-level gaps found.** All 5 ROADMAP Success Criteria are verified at the code level with substantive, wired, behaviorally-tested artifacts. All 7 requirements are satisfied. All 12 CONTEXT.md decisions are honored. All prohibitions are resolved and verified. Build, vet, and scoped tests all pass clean.

The single outstanding item is a **live gh integration smoke** (human_verification item #1) — an external service integration that cannot be unit-tested and which CONTEXT.md identifies as the phase's riskiest detail. This drives `status: human_needed`. Once a human confirms the live gh search returns real merged+closed PR data end-to-end, the phase is fully verified and ready for Phase 11 to build the Activity page on top of it.

## Verification Metadata

**Verification approach:** Goal-backward (ROADMAP Success Criteria as primary truths, cross-checked against PLAN frontmatter must_haves)
**Must-haves source:** ROADMAP.md § Phase 10 Success Criteria (5) + 10-01/10-02/10-03 PLAN frontmatter truths (granular, all roll up to the 5 SCs)
**Automated checks:** go build (1 pass), go vet (1 pass), go test ./internal/github/ (1 pass), go test ./internal/api/ scoped (18 test functions / 30+ sub-tests pass)
**Human checks required:** 1 (live gh smoke)
**Decision coverage:** 12/12 honored (non-blocking gate)

---
*Verified: 2026-07-30T16:05:00Z*
*Verifier: the agent (gsd-verifier)*
