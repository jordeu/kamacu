---
phase: 16-sharper-review-column
plan: 01
subsystem: api
tags: [github, gh, pr-review, cache, sqlite, typescript, react-query]

# Dependency graph
requires:
  - phase: 11-pr-review-column
    provides: "internal/github list path (runGH/reduceChecks/prRaw), per-repo TTL cache Service, always-200 /api/projects/{id}/pull-requests endpoint, usePullRequests/PRSummary/PullRequestsResponse"
  - phase: 12-open-a-review
    provides: "source='github_pr' task↔PR link (tasks.source/pr_number), /api/agents/status shared poll, useAgentStatuses"
  - phase: 10-github-foundations
    provides: "migration 00007 (tasks.source + tasks.pr_number columns)"
provides:
  - "Second reviewed-by:@me draft:false gh search cloned from the awaiting path, riding the SAME per-repo TTL cache cycle (D-12)"
  - "PRLists{Awaiting, Reviewed} combined runner + server-side top-precedence dedup (dedupeReviewed, D-14)"
  - "Widened github.Result with a `reviewed` array sharing state/stale/fetchedAt with `prs`"
  - "/api/agents/status entries carrying prNumber (nullable) + source so a PR card maps to its review session (D-15)"
  - "Frontend PullRequestsResponse.reviewed + AgentStatusEntry.prNumber/source wire types (the 16-02 contracts)"
affects: [16-02, rendering, ReviewColumn, PRCard]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Two gh searches fan into ONE PRLists struct fetched in a single cache cycle; a single failure degrades both lists together (no partial result)"
    - "Server-side single-section dedup: top list (awaiting) takes precedence, reviewed entries with overlapping PR numbers are dropped before the wire"
    - "SELECT-widening (not migration) to join pre-existing columns onto an existing JSON contract"

key-files:
  created: []
  modified:
    - internal/github/prlist.go
    - internal/github/service.go
    - internal/github/prlist_test.go
    - internal/github/service_test.go
    - internal/api/agents.go
    - internal/api/agents_test.go
    - internal/api/pullrequests_test.go
    - web/src/api/pullRequests.ts
    - web/src/api/agents.ts

key-decisions:
  - "Refactored runGH into listPRs(ctx, repo, repoDir, search) — one gh-list primitive parameterized on --search; both queues share field set, draft filter, reduceChecks, sort, and auth/error classification"
  - "repoEntry caches two slices (cachedAwaiting/cachedReviewed) + a hasCache bool rather than a single nilable slice, keeping resultLocked explicit and the gate-ladder timing byte-for-byte unchanged"
  - "fetchLists short-circuits on the first non-ok search so one gh failure degrades BOTH sections together (REVWD-04/D-12)"
  - "agent-status PR linkage is a SELECT + struct widening on BOTH passes — columns pre-exist from migration 00007, so NO new migration"

patterns-established:
  - "PRLists combined-runner shape: the Service runner returns ({Awaiting, Reviewed}, state, err); both lists ride one fetchedAt/TTL/floor"
  - "dedupeReviewed: top-precedence map-set drop preserving reviewed order"

requirements-completed: [REVWD-01, REVWD-02, REVWD-03, REVWD-04, SIGNL-01]

# Metrics
duration: 10min
completed: 2026-06-17
---

# Phase 16 Plan 01: Sharper Review Column — Data Layer Summary

**Second `reviewed-by:@me draft:false` gh search riding the existing per-repo TTL cache, a `PRLists` combined runner with server-side top-precedence dedup, a widened `{prs, reviewed, state, stale, fetchedAt}` Result, and `/api/agents/status` entries carrying `prNumber`/`source` — plus the matching frontend wire types — with zero visual changes.**

## Performance

- **Duration:** 10 min
- **Started:** 2026-06-17T08:55:22Z
- **Completed:** 2026-06-17T09:05:56Z
- **Tasks:** 3 (2 TDD, 1 standard)
- **Files modified:** 9

## Accomplishments

- Added the `reviewed-by:@me draft:false` queue as a near-clone of the awaiting search, fetched in the SAME cached `Service` cycle as the existing call (D-12) — not a second poll. `listPRs(ctx, repo, repoDir, search)` is now the single gh-list primitive; `fetchLists` runs both searches and degrades both together on any failure.
- Server-side single-section dedup (`dedupeReviewed`, D-14): any PR number in the awaiting list is dropped from reviewed before the wire, so a re-requested PR returns to the top and never duplicates below. Top precedence, reviewed order preserved.
- Widened `github.Result` with a `reviewed []PRSummary` riding the same `state`/`stale`/`fetchedAt`; `repoEntry` caches both lists under one TTL/floor; `recordFailureLocked` drops both at maxFailures. Gate-ladder timing unchanged.
- `/api/agents/status` entries now carry `prNumber *int64` (null for `manual`) + `source` joined from the `tasks` row on BOTH the manager-derived and DB-derived passes — no migration (columns pre-exist from 00007). A post-restart PR-review session surfaces with its prNumber so the card's dot/border survive a restart.
- Extended the two frontend wire types (`PullRequestsResponse.reviewed`, `AgentStatusEntry.prNumber/source`) — the exact contracts the rendering plan (16-02) consumes.

## Task Commits

Each task was committed atomically:

1. **Task 1: reviewed-by:@me search + two-list cache + server-side dedup** — `2bb261f` (feat)
2. **Task 2: prNumber + source on /api/agents/status entries** — `87e86ca` (feat)
3. **Task 3: extend frontend wire types** — `043a719` (feat)

_Note: the two TDD tasks (1, 2) wrote the failing test first (RED), then implemented to GREEN; because Go test files compile-gate on the new production types, each TDD task's test + implementation landed in one buildable, atomic commit rather than separate test/feat commits._

## Files Created/Modified

- `internal/github/prlist.go` — `runGH` refactored into `listPRs(...,search)`; added `searchAwaiting`/`searchReviewed` consts, `PRLists` struct, `fetchLists` combined runner, `dedupeReviewed`.
- `internal/github/service.go` — `Result.Reviewed`; `Config.Runner`/`Service.runner` retyped to the `PRLists` signature; default runner is `fetchLists`; `repoEntry` holds two cached slices + `hasCache`; `recordFailureLocked`/`resultLocked`/`Get` thread both lists.
- `internal/github/prlist_test.go` — `TestDedupeReviewed` table (top-precedence drop, non-overlap kept, order preserved, empty cases).
- `internal/github/service_test.go` — `fakeRunner`/`newTestService` moved to `PRLists`; added `sampleReviewed`/`sampleLists`; gate-ladder assertions kept green with new `Reviewed` served-stale + degraded-no-cache checks.
- `internal/api/agents.go` — `agentStatusEntry.PRNumber`/`Source`; both task SELECTs widened to `pr_number, source`; `taskMeta` + Scans extended; `prNumberOf` helper.
- `internal/api/agents_test.go` — `TestAgentStatusPRLinkFields` (manual → null/"manual", github_pr/42 → 42/"github_pr") + `setTaskPRLink` helper.
- `internal/api/pullrequests_test.go` — `prFake.run` updated to the new `github.PRLists` runner signature.
- `web/src/api/pullRequests.ts` — `PullRequestsResponse.reviewed: PRSummary[] | null`.
- `web/src/api/agents.ts` — `AgentStatusEntry.prNumber: number | null` + `source: "manual" | "github_pr"`.

## Decisions Made

- **Refactor over duplicate:** the reviewed search reuses `listPRs` rather than copy-pasting `runGH`, so the field set / draft filter / reduceChecks / sort / classification live exactly once — both queues are guaranteed identical except the `--search` term.
- **Two-slice cache form** (`cachedAwaiting`/`cachedReviewed` + `hasCache`) over a single nilable `*PRLists`, keeping `resultLocked` explicit and the TTL serve gate a one-word change (`e.cached != nil` → `e.hasCache`).
- **Fail-fast in `fetchLists`:** the awaiting search runs first; if it (or the reviewed search) is non-ok, the whole cycle reports that degraded state with both lists empty — never a partial success (REVWD-04/D-12).
- **No migration:** `tasks.source`/`tasks.pr_number` already exist from migration 00007, so the agent-status change is a SELECT + struct widening only.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Updated `prFake.run` in pullrequests_test to the new PRLists runner signature**
- **Found during:** Task 2 (running the new agent-status test, which compiles the whole `internal/api` test package)
- **Issue:** Task 1 changed `github.Config.Runner` from `(…) ([]PRSummary, string, error)` to `(…) (PRLists, string, error)`. The pre-existing `internal/api/pullrequests_test.go` injects a fake runner via `github.New(github.Config{Runner: fake.run})`, so its `prFake.run` method no longer satisfied the field type and the api test package failed to compile.
- **Fix:** Retyped `prFake.run` to return `github.PRLists{Awaiting: …}` (the fake's single PR lands in `Awaiting`, keeping the existing `res.PRs[0].Number == 7` assertion valid).
- **Files modified:** internal/api/pullrequests_test.go
- **Verification:** `go test ./internal/api/` passes (full suite, 95s); the pullrequests gate-ladder/disabled/refresh tests stay green.
- **Committed in:** 87e86ca (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Mechanical signature-propagation fix in existing test infra, directly caused by the planned Task 1 runner-signature change (the plan's own acceptance criterion noted "compiles through every caller, including internal/api"). No scope creep.

## Issues Encountered

None. The two TDD cycles went RED → GREEN cleanly; the one cross-package compile break (pullrequests_test) was an expected consequence of the runner-signature widening and is documented as a blocking deviation above.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 16-02 (the rendering plan) has its exact wire contracts:
  - `PullRequestsResponse.reviewed: PRSummary[] | null` — the Recently-reviewed cards (reuse `PRSummary` verbatim).
  - `AgentStatusEntry.prNumber: number | null` + `source: "manual" | "github_pr"` — find a PR's review session via `data.find(e => e.projectId === projectId && e.prNumber === pr.number)`.
- The endpoint payload is `{state, stale, fetchedAt, prs, reviewed}`; `reviewed` is already deduped against `prs` server-side, so the rendering plan renders both lists directly with no client-side dedup.
- No live `gh` was required to verify the data shape — faked-runner tests cover the cache/dedup/degrade contract; the live-gh path is exercised by the 16-02 human-verify.

## Self-Check: PASSED

- All key files verified present on disk (prlist.go, service.go, agents.go, pullRequests.ts, agents.ts, SUMMARY.md).
- All 3 task commits found in git log (2bb261f, 87e86ca, 043a719).
- Gates green: `go build ./...`, `go vet ./internal/github/ ./internal/api/`, `go test ./...`, `tsc -b`, `vite build`.
