---
phase: 10-activity-data-api
plan: 01
subsystem: api
tags: [gh, github, reviews, caching, go, backend]

# Dependency graph
requires:
  - phase: v1.3-v1.5 (github foundations)
    provides: internal/github package with Service.Get per-repo cache + listPRs gh runner + Available()/SetAvailableForTest seams
provides:
  - "(*github.Service).GetMergedClosed(ctx, repo, repoDir, force) — merged/closed reviewed-by:@me list with its own 5min-TTL per-repo cache"
  - "github.ReviewDoneSummary wire type (number/title/completedAt/url + project provenance fields for Plan 02 annotation)"
  - "github.listCompletedReviews gh runner (verified is:closed search + --limit 100 + closedAt→completedAt mapping)"
  - "github.classifyGhListError shared helper (auth_required vs error classification, used by BOTH listPRs and listCompletedReviews)"
  - "github.MergedClosedResult envelope (state/stale/fetchedAt/prs — mirrors github.Result for the activity endpoint's reviews sub-object)"
  - "github.Config.CompletedRunner test/production seam (defaults to listCompletedReviews)"
affects:
  - "10-02-PLAN.md (GET /api/activity handler consumes GetMergedClosed per in-scope repo + rolls up state)"
  - "Phase 11 Activity Page (renders ReviewDoneSummary lists grouped by project)"

# Tech tracking
tech-stack:
  added: []  # no new dependencies — pure Go against existing module + gh CLI
  patterns:
    - "Demand-driven per-repo cache with a SEPARATE TTL tier (mergedClosedEntry/mergedClosedTTL) decoupled from the review-column hot path (D-03) — the same Get gate-ladder shape (in-flight dedup + attemptFloor + drop-after-N) retargeted to a 5min success-TTL"
    - "Shared gh-failure classification helper (classifyGhListError) — one pure function called from BOTH listPRs and listCompletedReviews so the two fetchers never drift (Pattern 3)"
    - "Pure-helper decomposition for testability — parseCompletedReviews + classifyGhListError are pure functions unit-tested directly, mirroring the reduceChecks precedent"

key-files:
  created: []  # all changes are extensions to existing files
  modified:
    - "internal/github/prlist.go — ReviewDoneSummary type, searchCompletedReviews const, completedRaw decode struct, classifyGhListError + parseCompletedReviews pure helpers, listCompletedReviews runner; listPRs refactored to call classifyGhListError"
    - "internal/github/service.go — mergedClosedTTL const, mergedClosedEntry struct, MergedClosedResult envelope, Config.CompletedRunner seam, Service.completed map + completedRunner field, entryMergedClosed accessor, mergedClosedEntry.recordFailureLocked + resultMergedClosedLocked helpers, GetMergedClosed method"
    - "internal/github/prlist_test.go — TestClassifyGhListError, TestParseCompletedReviews, TestListCompletedReviews (+ exitErrOf sh-spawn helper for real *exec.ExitError)"
    - "internal/github/service_test.go — TestGetMergedClosed (9 sub-tests), fakeCompletedRunner + newTestServiceWithCompleted helpers, sampleCompletedPRs fixture"

key-decisions:
  - "5min TTL (mergedClosedTTL) for the merged/closed cache, fully decoupled from the 60s review-column cacheTTL — D-03/D-04. The activity page is an occasional view; coupling onto the 5s-poll hot path would 3x the gh load for data the column never renders."
  - "is:closed SEARCH QUALIFIER is the authoritative merged+closed filter (GitHub docs #5599); --state closed is kept only as belt-and-suspenders since cli/cli #8102 is a filed unfixed bug that a future gh could 'fix' to exclude merged (Pitfall 2)."
  - "--limit 100 on the gh search to avoid the default-30 truncation that would silently drop a weeks/months review history (Pitfall 1 — the existing listPRs omits --limit because the open review queue fits in 30)."
  - "classifyGhListError extracted from listPRs's inlined exit-code-4 + stderr-substring sniff as a PURE helper (no I/O, no logging) so listPRs and listCompletedReviews share one classification path with no drift; callers slog.Debug the returned state only (T-10-03: never log the stderr body)."
  - "Config.CompletedRunner is a NEW Service-level seam (defaults to listCompletedReviews), separate from Config.Runner (review column) — mirrors the existing Config.Runner/fetchLists pattern. No package-var exec seam on listCompletedReviews itself (consistent with listPRs being untested at the unit-spawn level)."
  - "parseCompletedReviews extracted as a pure helper so the ok-path transformation (closedAt→CompletedAt mapping + desc sort) is unit-testable with canned fixtures without coupling to live gh — mirrors the reduceChecks precedent."

patterns-established:
  - "Pattern: separate cache tier per access pattern — when a new consumer has a different access frequency than an existing cache, add a new map+TTL+entry-type rather than coupling onto the hot path. The gate-ladder shape (in-flight dedup + attemptFloor + drop-after-N) is reused verbatim; only the TTL and value type differ."
  - "Pattern: pure-helper decomposition for gh-shell-out functions — extract the JSON-parse/map/sort transformation AND the error classification into pure helpers, so they're unit-testable without a live gh seam. The shell-out wrapper stays thin (Available gate + spawn + delegate)."
  - "Pattern: shared classification helper across sibling gh fetchers — when two functions shell out to gh pr list with different searches, extract the failure classification so both call one path. Prevents drift on the subtle exit-code-4 + stderr-substring sniff (cli/cli#9338)."

requirements-completed: [REVIEWS-01, REVIEWS-04]  # from PLAN.md frontmatter — this plan delivers the gh data source + degrade foundation; Plan 10-02 wires the endpoint that surfaces them to users

# Coverage metadata (#1602) — one entry per shipped deliverable.
coverage:
  - id: D1
    description: "listCompletedReviews gh runner — fetches merged+closed reviewed-by:@me PRs via the verified is:closed + --limit 100 + closedAt→completedAt search"
    requirement: REVIEWS-01
    verification:
      - kind: unit
        ref: "internal/github/prlist_test.go#TestParseCompletedReviews (closedAt→CompletedAt mapping, desc sort, empty, malformed)"
        status: pass
      - kind: unit
        ref: "internal/github/prlist_test.go#TestListCompletedReviews/no_gh_returns_empty_+_no_spawn (Available()=false short-circuits before any spawn)"
        status: pass
      - kind: automated_procedural
        ref: "grep -c 'is:closed' prlist.go == 1; grep -c '\"--limit\", \"100\"' == 1; grep -c 'number,title,closedAt,url' == 1"
        status: pass
    human_judgment: true
    rationale: "The ok/auth_required/error spawn paths of listCompletedReviews itself are not unit-tested with live gh (consistent with listPRs); classification coverage comes from TestClassifyGhListError and ok-transformation from TestParseCompletedReviews. A live gh integration test would be the only way to prove the real spawn+parse end-to-end — deferred until Plan 10-02's handler can be exercised via the endpoint."

  - id: D2
    description: "classifyGhListError shared helper — auth_required vs error classification used by BOTH listPRs and listCompletedReviews (no drift)"
    requirement: REVIEWS-04
    verification:
      - kind: unit
        ref: "internal/github/prlist_test.go#TestClassifyGhListError (exit-4, three stderr substrings, no-exit-info, generic exits)"
        status: pass
      - kind: automated_procedural
        ref: "grep -c 'classifyGhListError' prlist.go == 3 (1 decl + 2 call sites)"
        status: pass
    human_judgment: false

  - id: D3
    description: "GetMergedClosed method — 5min-TTL per-repo cache with the Get gate-ladder shape (in-flight dedup + attemptFloor + drop-after-N), decoupled from the review-column entries map (D-03)"
    requirement: REVIEWS-01
    verification:
      - kind: unit
        ref: "internal/github/service_test.go#TestGetMergedClosed (happy path, TTL hit at 4min, TTL expire at 5min, force bypass, attemptFloor bind, maxFailures drop, D-03 cache independence, no_gh degrade, never-disabled)"
        status: pass
      - kind: automated_procedural
        ref: "awk GetMergedClosed body grep 's.entries' == 0; awk Get body grep 's.completed' == 0 (D-03 separation)"
        status: pass
    human_judgment: false

  - id: D4
    description: "ReviewDoneSummary wire type — number/title/completedAt/url + provenance fields (projectName/projectId/repo) left zero-valued for Plan 02's aggregation loop to annotate (D-06)"
    requirement: REVIEWS-01
    verification:
      - kind: unit
        ref: "internal/github/prlist_test.go#TestParseCompletedReviews/maps_closedAt_to_CompletedAt (asserts provenance fields stay zero)"
        status: pass
    human_judgment: false

  - id: D5
    description: "Per-repo degrade classification (no_gh/auth_required/error) shared via classifyGhListError — REVIEWS-04 foundation (a failing repo never blocks the rest)"
    requirement: REVIEWS-04
    verification:
      - kind: unit
        ref: "internal/github/service_test.go#TestGetMergedClosed/degrade_no_gh_never_panics + /drop_cached_after_maxFailures"
        status: pass
    human_judgment: true
    rationale: "The full REVIEWS-04 contract (a failing repo never blocks the aggregate) is exercised at the Service level here, but the cross-repo errgroup rollup that turns per-repo degrades into a partial/ok aggregate state lives in Plan 10-02's handler — verified there."

# Metrics
duration: 12 min
completed: 2026-07-30
status: complete
---

# Phase 10 Plan 01: GitHub GetMergedClosed Extension Summary

**New `GetMergedClosed` method on `*github.Service` with a decoupled 5min-TTL per-repo cache, backed by `listCompletedReviews` (verified `is:closed` + `--limit 100` gh search) and the shared `classifyGhListError` helper — the reviews-done data source for Plan 10-02's `GET /api/activity` handler.**

## Performance

- **Duration:** 12 min
- **Started:** 2026-07-30T11:45:03Z
- **Completed:** 2026-07-30T11:57:06Z
- **Tasks:** 2 (both TDD: RED → GREEN)
- **Files modified:** 4

## Accomplishments

- **`listCompletedReviews` gh runner** — fetches merged+closed `reviewed-by:@me` PRs via the verified search (`is:closed` qualifier is the authoritative merged+closed filter per GitHub docs #5599; `--state closed` is belt-and-suspenders only since cli/cli #8102 is a filed unfixed bug). Passes `--limit 100` to avoid the default-30 truncation (Pitfall 1). Maps `closedAt → CompletedAt` (a merge IS a close, so `closedAt` is set for both). Sorts desc by CompletedAt (ISO-8601 lexical == chrono).
- **`classifyGhListError` shared helper** — extracted from `listPRs`'s inlined exit-code-4 + stderr-substring sniff (cli/cli#9338: exit code 4 alone is unreliable). Pure function (no I/O, no logging); called from BOTH `listPRs` and `listCompletedReviews` so the two fetchers never drift. Callers `slog.Debug` the classified state only — never the stderr body (T-10-03 information-disclosure control).
- **`GetMergedClosed` method + 5min-TTL decoupled cache** — mirrors `Service.Get`'s gate ladder (in-flight dedup + `attemptFloor` + drop-after-`maxFailures`) but keyed on a SEPARATE `mergedClosedEntry` map with `mergedClosedTTL = 5min` standing in for `cacheTTL = 60s`. D-03 separation is enforced at the method-body level: `GetMergedClosed` never touches `s.entries`, `Get` never touches `s.completed` (verified by grep + the D-03 independence test case). Never emits state `"disabled"` (endpoint-only, Plan 02 GATE 1).
- **`ReviewDoneSummary` wire type** — carries number/title/completedAt/url plus provenance fields (projectName/projectId/repo) left zero-valued for Plan 02's aggregation loop to annotate (D-06). `PRSummary` (the review-column wire type) is intentionally untouched.
- **Full TDD discipline** — both tasks shipped RED (failing tests + stubs) → GREEN (real impl) commits. RED gates prove the tests fail for the right reason; GREEN gates prove the impl satisfies them.

## Task Commits

Each task was committed atomically via TDD RED → GREEN:

1. **Task 1 RED: failing tests for listCompletedReviews + classifyGhListError** — `67152db` (test)
2. **Task 1 GREEN: implement listCompletedReviews + classifyGhListError** — `38f8705` (feat)
3. **Task 2 RED: failing tests for GetMergedClosed + merged/closed cache** — `02aef05` (test)
4. **Task 2 GREEN: implement GetMergedClosed with 5min-TTL decoupled cache** — `36baf38` (feat)

**Plan metadata:** `docs(10-01): complete getmergedclosed-extension plan` (this commit)

## Files Created/Modified

- `internal/github/prlist.go` — `ReviewDoneSummary` type, `searchCompletedReviews` const, `completedRaw` decode struct, `classifyGhListError` + `parseCompletedReviews` pure helpers, `listCompletedReviews` runner; `listPRs` refactored to call `classifyGhListError` (byte-identical behavior).
- `internal/github/service.go` — `mergedClosedTTL` const, `mergedClosedEntry` struct, `MergedClosedResult` envelope, `Config.CompletedRunner` seam, `Service.completed` map + `completedRunner` field, `entryMergedClosed` accessor, `mergedClosedEntry.recordFailureLocked` + `resultMergedClosedLocked` helpers, `GetMergedClosed` method.
- `internal/github/prlist_test.go` — `TestClassifyGhListError` (10 cases: exit-4, three stderr substrings, generic exits, no-exit-info), `TestParseCompletedReviews` (closedAt→CompletedAt mapping, desc sort, empty, malformed), `TestListCompletedReviews` (no_gh path via `SetAvailableForTest(false)`); `exitErrOf` helper spawns real `sh -c "exit N"` to construct `*exec.ExitError` (os.ProcessState is not publicly constructible).
- `internal/github/service_test.go` — `TestGetMergedClosed` (9 sub-tests: happy path, TTL hit at 4min, TTL expire at 5min+1s, force bypass, attemptFloor bind even under force, drop-after-maxFailures, D-03 cache independence, no_gh degrade, never-disabled); `fakeCompletedRunner` + `newTestServiceWithCompleted` helpers; `sampleCompletedPRs` fixture.

## Decisions Made

- **Pure-helper decomposition for testability.** `parseCompletedReviews` (JSON decode + closedAt→CompletedAt map + desc sort) and `classifyGhListError` (exit-code + stderr sniff) are extracted as pure helpers so they're unit-testable with canned fixtures, mirroring the `reduceChecks` precedent. `listCompletedReviews` stays a thin shell-out wrapper (Available gate + spawn + delegate). This is how `listPRs` should have been decomposed; the new fetch does it right.
- **No package-var exec seam on `listCompletedReviews`.** Symmetric with `listPRs` (which has no package-var seam — only `Config.Runner` at the Service level). The testable surface for the spawn+classify path is `Config.CompletedRunner` (Task 2's Service-level seam). The `postReviewRunner` package-var pattern was considered and rejected to avoid overlapping seams.
- **Real `sh -c "exit N"` for exit-error test fixtures.** `*os.ProcessState` is not publicly constructible, so `exitErrOf(t, code)` spawns a real `sh` to produce an `*exec.ExitError` with the desired code. Tests run on Linux/macOS where `sh` is present; `t.Skip` if absent.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Reworded trailing comment to satisfy the strict AC6 grep**
- **Found during:** Task 2 GREEN (acceptance criteria verification)
- **Issue:** AC6 (`grep -v '^//'` within the `GetMergedClosed` body shows no `"disabled"` write) flagged a trailing comment on the `default:` case that contained the literal string `"disabled"` — even though no code path assigns `"disabled"` to State. The AC's grep filters only lines starting with `//`, not trailing comments.
- **Fix:** Reworded the comment from `// no_gh / auth_required / error — never "disabled" (endpoint-only)` to `// no_gh / auth_required / error — the disabled state is endpoint-only (Plan 02 GATE 1)`. The word "disabled" remains (for readability) but the literal quoted `"disabled"` does not.
- **Files modified:** `internal/github/service.go`
- **Verification:** `awk GetMergedClosed body | grep -v '^//' | grep -c '"disabled"'` returns 0 (was 1).
- **Committed in:** `36baf38` (Task 2 GREEN commit)

---

**Total deviations:** 1 auto-fixed (1 bug — comment wording for AC compliance)
**Impact on plan:** Cosmetic only — no behavior change. The AC grep is strict about the literal string; the comment reword keeps the intent readable while satisfying the gate.

## Issues Encountered

None — both tasks landed cleanly on the first GREEN attempt. The pre-existing `TestServiceGet`, `TestReduceChecks`, `TestReduceChecksSupersededRuns`, `TestReduceChecksSkippedHeavy`, and `TestDedupeReviewed` continued to pass unchanged through the `listPRs` → `classifyGhListError` refactor (byte-identical classification behavior).

## User Setup Required

None — no external service configuration required. This plan uses only the existing Go module + the already-installed `gh` CLI (a soft dependency via `Available()`).

## Next Phase Readiness

- **Ready for Plan 10-02** (`GET /api/activity` handler): `GetMergedClosed(ctx, repo, repoDir, force)` is callable per in-scope repo; `MergedClosedResult` carries the `{state, stale, fetchedAt, prs}` envelope mirroring `github.Result`; `ReviewDoneSummary` carries provenance fields ready for the aggregation loop to annotate; the per-repo degrade classification is shared with the review column via `classifyGhListError`.
- **Ready for Plan 10-03** (pure helpers sibling): no shared surface — 10-01 and 10-03 are wave-1 siblings; 10-02 consumes both.
- **No blockers.** The D-03 cache separation is load-bearing and verified; the verified gh search tokens (`is:closed`, `--state closed`, `--limit 100`, `number,title,closedAt,url`) are pinned by grep and by the `searchCompletedReviews` const.

---
*Phase: 10-activity-data-api*
*Completed: 2026-07-30*

## Self-Check: PASSED

- SUMMARY.md exists at `.planning/phases/10-activity-data-api/10-01-SUMMARY.md` ✓
- Task commits found in git log: `67152db` (test RED T1), `38f8705` (feat GREEN T1), `02aef05` (test RED T2), `36baf38` (feat GREEN T2) ✓
- `go build ./...` passes ✓
- `go test ./internal/github/ -count=1` passes (all pre-existing + new tests) ✓
- `go vet ./internal/github/` clean ✓
- D-03 separation verified by grep: GetMergedClosed body has 0 `s.entries` refs; Get body has 0 `s.completed` refs ✓
- Verified gh search tokens present: `is:closed` (1), `--state closed` (1), `--limit 100` (1), `number,title,closedAt,url` (1) ✓
