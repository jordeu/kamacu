---
phase: 10-activity-data-api
plan: 02
subsystem: api
tags: [go, backend, http, activity, statistics, errgroup, concurrency, degrade, github]

# Dependency graph
requires:
  - phase: 10-01 (GetMergedClosed extension)
    provides: "(*github.Service).GetMergedClosed(ctx, repo, repoDir, force) + MergedClosedResult{state,stale,fetchedAt,prs} + ReviewDoneSummary (with provenance fields left zero for the aggregation loop to annotate)"
  - phase: 10-03 (activity pure helpers)
    provides: "parseScope/parseWindow/parseActivityTime/computeTimeStat/buildDurationSlices/rollupReviewState + activityTask/timeStat/statsBlock wire types (stdlib-only, imported — NOT redeclared)"
  - phase: v1.8 (per-status timestamps)
    provides: "migration 00006 *_at columns + the reaper's ms-ISO done_at storage format the lexical SQL cutoff mirrors"
provides:
  - "GET /api/activity?scope=global|workspace:N|project:N&window=week|month&refresh=1 — the combined {tasks, reviews, stats} endpoint Phase 11's Activity page renders"
  - "api.activityResponse wire type (Tasks/Reviews/Stats) + api.aggregateReviewsResult (mirrors github.MergedClosedResult shape)"
  - "api.aggregateReviews(ctx, ghSvc, repos, force, cutoff time.Time) — concurrent bounded (errgroup SetLimit 5) cross-repo fan-out with per-repo 12s timeout, the reviews WINDOW FILTER (completedAt < cutoff dropped), and the D-08 ok/partial/degraded rollup"
  - "api.ActivityRoutes(mux, db, ghSvc) registrar — registered alongside PullRequestRoutes on the SAME shared *github.Service"
affects:
  - "Phase 11 Activity Page (one TanStack query against /api/activity renders all three sections; branches on reviews.state for the degrade UX)"
  - "Future stats/audit phases (the in-Go cycle/dwell compute + statsBlock shape is the contract they extend)"

# Tech tracking
tech-stack:
  added:
    - "golang.org/x/sync/errgroup (promoted indirect→direct, v0.20.0) — bounded concurrent cross-repo fan-out (D-07)"
  patterns:
    - "Two-representation cutoff from one instant — cutoffStr (ms-ISO '2006-01-02T15:04:05.000Z', lexical SQL compare) AND cutoffTime (time.Time, reviews time-compare) both derive from now.Add(-window); never string-compare timestamps across the ms/s precision mismatch (parse both sides via parseActivityTime)"
    - "errgroup with nil-on-degrade closures + per-repo context.WithTimeout — a failing/slow repo returns its degrade state and NEVER cancels the group (Pitfall 4); SetLimit(5) bounds concurrency"
    - "GATE 1 short-circuit precedes the aggregate — settings.Get(KeyGithubIntegration) != 'on' sets reviews.state='disabled' BEFORE any GetMergedClosed call (mirrors pullrequests.go:56-64)"
    - "Structural scope branching + parameterized ? placeholders — the parsed scope kind selects the WHERE clause; the numeric N is a ? arg, never string-concatenated (Pitfall 5, T-10-05)"

key-files:
  created:
    - "internal/api/activity.go — aggregateReviewsResult/activityResponse/repoRow wire types; aggregateReviews (errgroup fan-out + window filter + rollup); fetchActivityTasks (tasks-done SQL); fetchLinkedRepos (in-scope linked-projects query); ActivityRoutes (the GET handler + registrar)"
    - "internal/api/activity_test.go — 12 hermetic integration tests + helpers (newActivityEnv, seedActivityProject, seedDoneTask, completedRunnerByRepo, decodeActivity, msISO/secISO)"
  modified:
    - "cmd/kamacu/serve.go — api.ActivityRoutes(mux, db, ghSvc) registered on the line after api.PullRequestRoutes (same shared ghSvc, read-only — no wtSvc)"
    - "go.mod / go.sum — golang.org/x/sync promoted from // indirect to a direct dependency"

key-decisions:
  - "errgroup (not WaitGroup+semaphore) for the concurrent fan-out — already present as an indirect dep (v0.20.0), promoting it to direct via go mod tidy needs no registry verification (T-10-SC2). SetLimit(5) bounds concurrency; closures return nil on every outcome so a degrade rides in the per-repo result's state, never as an errgroup error (Pitfall 4)."
  - "The reviews WINDOW FILTER compares time.Time-vs-time.Time via parseActivityTime — the cutoff (ms-ISO from now−window) and the PR completedAt (gh closedAt, second-precision) are both parsed then compared with Before(cutoff); never a lexical string compare across the precision mismatch (REVIEWS-01 / STATS-01)."
  - "Two cutoff representations from one instant — cutoffStr (ms-ISO) feeds the lexical SQL done_at>=? compare; cutoffTime (time.Time) threads into aggregateReviews for the completedAt window compare. Same now.Add(-window), two shapes, zero drift."
  - "fetchActivityTasks/fetchLinkedRepos factored out as helpers but the SQL literals stay inline in activity.go so the acceptance greps (source='manual', done_at>=?, ORDER BY t.done_at DESC) find them — the query-building IS the grep target."

patterns-established:
  - "Pattern: combined endpoint for a page's whole data need — when one page renders three logically-distinct sections (tasks list / reviews list / stats), expose ONE endpoint returning {tasks, reviews, stats} so the frontend issues ONE query with ONE loading state, not three. Degradation rides in the sub-object's state field; the whole response is always 200."
  - "Pattern: concurrent bounded cross-repo fan-out with degrade-don't-cancel — errgroup.Go closures that ALWAYS return nil (degrade rides in the per-repo result), a per-repo context.WithTimeout so one slow repo can't block the rest, and SetLimit to bound the gh spawn rate. The aggregate state is rolled up from the per-repo states after Wait()."

requirements-completed: [TASKS-01, REVIEWS-01, REVIEWS-04, STATS-01, STATS-02, STATS-03, STATS-04]

# Coverage metadata (#1602) — one entry per shipped deliverable.
coverage:
  - id: D1
    description: "GET /api/activity handler — combined {tasks, reviews, stats} for any global/workspace/project scope and week/month window, always HTTP 200 (degradation rides in reviews.state); GATE 1 toggle enforced; invalid scope/window → 400"
    requirement: TASKS-01
    verification:
      - kind: unit
        ref: "internal/api/activity_test.go#TestActivityHandler_GlobalScopeAndWindow (in-window present, out-of-window + github_pr excluded, done_at DESC order)"
        status: pass
      - kind: unit
        ref: "internal/api/activity_test.go#TestActivityHandler_InvalidScope + #TestActivityHandler_InvalidWindow (400 on bad params)"
        status: pass
      - kind: automated_procedural
        ref: "go build ./... && go vet ./internal/api/ ./cmd/kamacu/ both exit 0; grep confirms GET /api/activity registered once + ActivityRoutes wired in serve.go"
        status: pass
    human_judgment: false

  - id: D2
    description: "aggregateReviews — concurrent bounded (errgroup SetLimit 5) cross-repo fan-out with per-repo 12s timeout; reviews WINDOW FILTER drops PRs whose completedAt < cutoff (parsed via parseActivityTime, not string-compared); ok/partial/degraded rollup (D-07/D-08)"
    requirement: REVIEWS-01
    verification:
      - kind: unit
        ref: "internal/api/activity_test.go#TestActivityHandler_ReviewsWindowFilter (10d PR dropped under week, both kept under month — proves cutoff threads in + second-precision closedAt parsed)"
        status: pass
      - kind: unit
        ref: "internal/api/activity_test.go#TestActivityHandler_Partial (ok+no_gh → 'partial', only successful repo's PRs with provenance) + #TestActivityHandler_NoGh (all degrade → 'no_gh', prs empty)"
        status: pass
      - kind: unit
        ref: "internal/api/activity_test.go#TestActivityHandler_RefreshPropagation (force fans out to every repo)"
        status: pass
    human_judgment: false

  - id: D3
    description: "GATE 1 toggle enforcement — settings.Get(KeyGithubIntegration) != 'on' → reviews.state='disabled' with ZERO gh spawns; tasks+stats still populate (D-02, REVIEWS-04)"
    requirement: REVIEWS-04
    verification:
      - kind: unit
        ref: "internal/api/activity_test.go#TestActivityHandler_Gate1Disabled (state='disabled', runner 0 calls, tasks+stats non-empty)"
        status: pass
      - kind: automated_procedural
        ref: "grep confirms the 'disabled' branch (activity.go:260) precedes the aggregateReviews call (activity.go:268)"
        status: pass
    human_judgment: false

  - id: D4
    description: "tasks-done SQL — status='done' AND source='manual' AND done_at>=? (cutoffStr) + scope clause, ORDER BY done_at DESC; scope id passed as parameterized ? placeholder (D-12, T-10-05)"
    requirement: TASKS-01
    verification:
      - kind: unit
        ref: "internal/api/activity_test.go#TestActivityHandler_WorkspaceScope + #TestActivityHandler_ProjectScope + #TestActivityHandler_WindowMonth (scope/window narrowing)"
        status: pass
      - kind: automated_procedural
        ref: "grep confirms source='manual' (1), done_at>=? (1), ORDER BY t.done_at DESC (1) in activity.go"
        status: pass
    human_judgment: false

  - id: D5
    description: "in-Go cycle/dwell/median stats — buildDurationSlices + computeTimeStat over the *_at timestamps; taskCount/reviewCount + cycle/dwellInProgress/dwellInReview {n,min,max,median} in int64 seconds; reviews contribute a count only (STATS-04)"
    requirement: STATS-01
    verification:
      - kind: unit
        ref: "internal/api/activity_test.go#TestActivityHandler_StatsOverSeededDB (hand-computed cycle{10800,7200,9000} / dwellInProgress{3600,7200,5400} / dwellInReview{7200} incl even-N median + Open Q3 fallback)"
        status: pass
    human_judgment: false

# Metrics
duration: 9 min
completed: 2026-07-30
status: complete
---

# Phase 10 Plan 02: Activity Data API Handler Summary

**`GET /api/activity` combined endpoint — concurrent bounded cross-repo reviews aggregation (errgroup, window-filtered), GATE 1 toggle, in-Go cycle/dwell/median stats, and the always-200 degrade contract — wired alongside PullRequestRoutes on the shared ghSvc.**

## Performance

- **Duration:** 9 min
- **Started:** 2026-07-30T13:37:32Z
- **Completed:** 2026-07-30T13:46:27Z
- **Tasks:** 2 (both tdd-tagged; tdd_mode off for the phase, so executed as build-then-verify)
- **Files modified:** 4 (2 created, 2 modified)

## Accomplishments

- **`GET /api/activity` handler** — the combined `{tasks, reviews, stats}` endpoint Phase 11 renders from one TanStack query. Parses+validates scope (global/workspace:N/project:N) and window (week/month) via the plan 10-03 helpers, returning 400 on any invalid param. Always HTTP 200; degradation rides in `reviews.state` (D-01, REVIEWS-04).
- **`aggregateReviews` — concurrent bounded fan-out with the reviews WINDOW FILTER.** errgroup with `SetLimit(5)` fires one `GetMergedClosed` per in-scope linked repo under a per-repo `context.WithTimeout(ctx, 12s)`. Each closure annotates provenance, parses the PR's `completedAt` (gh closedAt, second-precision) via `parseActivityTime`, and DROPS it when `Before(cutoff)` — the cutoff threads in as a `time.Time` so the compare is time-vs-time, never a string compare across the ms/s precision mismatch (REVIEWS-01 / STATS-01). Closures return nil on every outcome — a degrade rides in the per-repo result's state, never as an errgroup error (Pitfall 4). The aggregate state rolls up via `rollupReviewState` (ok/partial/degraded, D-08).
- **Two cutoff representations from one instant.** `now.Add(-window)` produces both `cutoffStr` (ms-ISO `2006-01-02T15:04:05.000Z`, the reaper's done_at format → lexical SQL `done_at >= ?` compare) and `cutoffTime` (`time.Time` → `aggregateReviews` completedAt compare). Same instant, two shapes, zero drift.
- **GATE 1 enforced.** `settings.Get(db, settings.KeyGithubIntegration)` short-circuits to `reviews.state="disabled"` with ZERO gh spawns when the toggle is off; tasks + stats still populate (mirrors `pullrequests.go:56-64`). The disabled branch provably precedes the `aggregateReviews` call.
- **In-Go stats.** `buildDurationSlices` + `computeTimeStat` produce `cycle` / `dwellInProgress` / `dwellInReview` `{n, min, max, median}` as int64 seconds over the `*_at` timestamps (D-10/D-11). `reviewCount` is a plain int; reviews carry no time-stat object (STATS-04).
- **Route wired on the shared ghSvc** — `api.ActivityRoutes(mux, db, ghSvc)` registered on the line after `api.PullRequestRoutes` in `serve.go`; no new `github.New` construction, no `wtSvc` (read-only).

## Task Commits

Each task was committed atomically:

1. **Task 1: activity.go handler — wire types, aggregateReviews (with window cutoff), GATE 1, route registration** — `3955659` (feat)
2. **Task 2: activity handler integration tests — scope/window variants, GATE 1, degrade rollup, reviews window filter, stats over seeded DB, refresh propagation** — `9d043d5` (test)

**Plan metadata:** `docs(10-02): complete activity-data-api-handler plan` (this commit)

## Files Created/Modified

- `internal/api/activity.go` (created) — `aggregateReviewsResult` / `activityResponse` / `repoRow` wire types; `aggregateReviews` (errgroup fan-out + window filter + rollup); `fetchActivityTasks` (tasks-done SQL, `source='manual'` + `done_at>=?` + scope clause + `ORDER BY t.done_at DESC`); `fetchLinkedRepos` (in-scope linked-projects query); `ActivityRoutes` (the GET handler + registrar).
- `internal/api/activity_test.go` (created) — 12 hermetic integration tests + helpers (`newActivityEnv`, `seedActivityProject`, `seedDoneTask`, `completedRunnerByRepo`, `decodeActivity`, `msISO`/`secISO`). No real gh spawn (fake CompletedRunner keyed by repo; nil defaults to a no-spawn `no_gh` runner).
- `cmd/kamacu/serve.go` (modified) — `api.ActivityRoutes(mux, db, ghSvc)` registered on the line after `api.PullRequestRoutes`.
- `go.mod` / `go.sum` (modified) — `golang.org/x/sync` promoted from `// indirect` to a direct dependency.

## Decisions Made

- **errgroup over WaitGroup+semaphore.** Already present as an indirect dep (v0.20.0, verified in plan 10-01 RESEARCH § Package Legitimacy Audit); promoting it to direct via `go mod tidy` requires no registry verification (T-10-SC2: accept). `SetLimit(5)` bounds concurrency; closures that always return nil make a degrade ride in the per-repo result's state rather than canceling the group (Pitfall 4).
- **SQL literals kept inline (grep targets).** `fetchActivityTasks`/`fetchLinkedRepos` are factored into helpers for readability, but the SQL string literals (`source = 'manual'`, `done_at >= ?`, `ORDER BY t.done_at DESC`) stay inline in `activity.go` so the acceptance greps find them — the query-building IS the grep target.
- **Test seeding via INSERT-with-columns + NULL mapping.** `seedDoneTask` inserts with the `*_at` columns set directly (empty string → SQL NULL via `strToNull`), since the move endpoint isn't exercised here. This keeps each test hermetic with no real status transitions.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] `completedRunnerByRepo` tolerated a nil call counter**
- **Found during:** Task 2 (first test run — TestActivityHandler_Partial panicked)
- **Issue:** `completedRunnerByRepo` unconditionally called `calls.Add(1)`, but `TestActivityHandler_Partial` (and others) pass `nil` for the counter when they don't need call counts. The nil `*atomic.Int64` dereference segfaulted inside the errgroup goroutine.
- **Fix:** Guarded the `Add` behind `if calls != nil`, so the helper works both for tests that assert call counts (Gate1/NoGh/Refresh) and tests that only assert state/PRs (Partial).
- **Files modified:** `internal/api/activity_test.go`
- **Verification:** `go test ./internal/api/ -run 'TestActivityHandler' -count=1` passes all 12 tests (was: panic in Partial).
- **Committed in:** `9d043d5` (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug — nil-counter dereference in the test helper)
**Impact on plan:** Test-only fix; no production behavior change. The handler itself was unaffected.

## Issues Encountered

- **Pre-existing test-name collision.** `seedWorkspace` was already declared in `internal/api/projects_test.go` (same package, identical body). Removed the duplicate from `activity_test.go` and reused the existing helper.
- **Full `internal/api` test suite hangs (pre-existing, out of scope).** As documented in plan 10-03's SUMMARY, `go test ./internal/api/...` (the whole package) exceeds 120s due to heavy integration tests with real gh/PTY/process spawns unrelated to this plan. Verification used the filtered run `go test ./internal/api/ -run 'TestActivityHandler' -count=1` (passes in ~1s) plus `go build ./...` + `go vet ./internal/api/ ./cmd/kamacu/`. This plan only ADDED files; existing tests are unaffected.
- **Older `10-02` commits in git log.** A prior milestone's phase reused the conventional-commit scope `10-02` (github-origin endpoint work). My two commits (`3955659`, `9d043d5`) are the ones for THIS plan; they are the most recent and clearly labeled "GET /api/activity handler" / "activity handler integration tests". Pre-existing, not introduced here.

## User Setup Required

None — no external service configuration required. The endpoint reuses the already-installed `gh` CLI (soft dependency via `github.Available()`) and the existing SQLite DB; `errgroup` is a stdlib-adjacent Go-team library already in the module graph.

## Next Phase Readiness

- **Phase 10 is COMPLETE** (this was the final plan; 10-01 and 10-03 shipped in wave 1). All three plans' requirements (TASKS-01, REVIEWS-01, REVIEWS-04, STATS-01..04) are delivered.
- **Ready for Phase 11** (Activity Page & Controls): the `GET /api/activity` endpoint is the single data source — one TanStack query renders the tasks-done list, the reviews-done list (branching on `reviews.state` for the degrade UX), and the stats block. The wire shapes (`activityResponse`, `aggregateReviewsResult` mirroring `github.MergedClosedResult`, `statsBlock`) are the rendering contract.
- **No blockers.** The always-200 + degrade-in-`reviews.state` contract (REVIEWS-04) is verified end-to-end across GATE 1 off / all-degraded / partial / ok / window-filter paths.

---
*Phase: 10-activity-data-api*
*Completed: 2026-07-30*

## Self-Check: PASSED

- SUMMARY.md exists at `.planning/phases/10-activity-data-api/10-02-SUMMARY.md` ✓
- Task commits found in git log: `3955659` (feat T1), `9d043d5` (test T2) ✓
- `go build ./...` passes ✓
- `go test ./internal/api/ -run 'TestActivityHandler' -count=1` passes (all 12 tests) ✓
- `go vet ./internal/api/ ./cmd/kamacu/` clean ✓
- `go test ./internal/github/ -count=1` still passes (plan 10-01 unchanged) ✓
- Acceptance greps green: ActivityRoutes declared; GET /api/activity registered (1); aggregateReviews signature with `cutoff time.Time`; no redeclared pure helpers (0); `source='manual'` (1); `done_at>=?` (1); `ORDER BY t.done_at DESC` (1); cutoff layout literal (1); GATE 1 disabled branch precedes aggregateReviews call; errgroup+SetLimit+WithTimeout present; serve.go wiring present; go.mod errgroup direct (no `// indirect`) ✓
