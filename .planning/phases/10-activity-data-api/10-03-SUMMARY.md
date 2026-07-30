---
phase: 10-activity-data-api
plan: 03
subsystem: api
tags: [go, backend, statistics, parsing, validation, pure-helpers, tdd]

# Dependency graph
requires:
  - phase: v1.8 (per-status timestamps)
    provides: migration 00006 *_at columns (in_progress_at/in_review_at/done_at) — the raw strings buildDurationSlices parses; the reaper's ms-ISO storage format parseActivityTime mirrors
provides:
  - "api.parseActivityTime(s string) (time.Time, bool) — the shared mismatched-precision ISO parser (ms .000Z then bare-Z); consumed by buildDurationSlices here AND by plan 10-02's aggregateReviews reviews-window cutoff"
  - "api.parseScope(raw string) (kind string, id int64, err error) — global/workspace:N/project:N validation gate the handler maps to HTTP 400"
  - "api.parseWindow(raw string) (time.Duration, error) — week(default)/month validation gate the handler maps to HTTP 400"
  - "api.computeTimeStat(durations []time.Duration) timeStat — min/max/median (int64 seconds, even-N parity correct)"
  - "api.buildDurationSlices(tasks []activityTask) (cycle, dwellInProgress, dwellInReview []time.Duration) — Open Q3 dwellInProgress semantic, NULL/unparseable *_at skipped per-slice"
  - "api.rollupReviewState(states []string) string — ok/partial/degraded rollup with priority error>auth_required>no_gh"
  - "api.activityTask / api.timeStat / api.statsBlock pure wire shapes plan 10-02's handler composes into the activityResponse"
affects:
  - "10-02-PLAN.md (GET /api/activity handler imports these helpers + composes statsBlock from the duration slices)"
  - "Phase 11 Activity Page (renders statsBlock.cycle/dwellInProgress/dwellInReview as human-friendly seconds)"

# Tech tracking
tech-stack:
  added: []  # zero new dependencies — pure Go standard library only
  patterns:
    - "Pure-helper extraction for testability — parse/validate/compute/median logic decoupled from the handler's DB+gh+aggregation wiring so the pure surface ships + verifies independently with its own unit-test gate (sibling to plan 10-01's classifyGhListError/parseCompletedReviews precedent)"
    - "Dual-layout time.Parse loop for mismatched-precision ISO timestamps — try the ms layout (.000Z, requires the fraction) first, then the bare-Z second-precision layout; the ordered fallback is what makes one time-comparison correct across reaper ms vs gh second-precision closedAt"
    - "Per-slice NULL/unparseable skip (Pitfall 7) — a row with a missing/invalid *_at is excluded from THAT stat slice only and never panics; cycle.n / dwell*.n may be < taskCount (D-09 defensive exclude)"

key-files:
  created:
    - "internal/api/activity_helpers.go — activityTask/timeStat/statsBlock wire types + parseActivityTime/parseScope/parseWindow/computeTimeStat/buildDurationSlices/rollupReviewState (stdlib-only: fmt, sort, strconv, strings, time)"
    - "internal/api/activity_helpers_test.go — TestParseActivityTime/TestParseScope/TestParseWindow/TestComputeTimeStat/TestBuildDurationSlices/TestRollupReviewState (full coverage of every documented branch incl. even-N median parity, Open Q3 fallback, NULL/unparseable per-slice skip)"
  modified: []

key-decisions:
  - "parseActivityTime is the SHARED parse primitive — a dual-layout time.Parse loop (ms '2006-01-02T15:04:05.000Z' then bare '2006-01-02T15:04:05Z'). The .000Z layout REQUIRES the fraction (verified: it fails on bare-Z inputs), so the ordered fallback is load-bearing — it is what lets buildDurationSlices (reaper ms timestamps) and plan 10-02's reviews-window cutoff (gh second-precision closedAt) share one correct comparison. Never string-compare timestamps across precisions."
  - "buildDurationSlices implements the Open Q3 dwellInProgress semantic (USER-CONFIRMED during revision): in_review_at set -> dwellInProgress = inReview - inProgress (time in In Progress before entering In Review); in_review_at absent -> fallback done - inProgress. cycle is always done - inProgress; dwellInReview is always done - inReview. The fallback makes the skipped-In-Review path still surface an In-Progress dwell number."
  - "activity_helpers.go is stdlib-only by contract — no *sql.DB, no internal/github. The negative-grep gate (grep internal/github == 0 AND grep database/sql == 0) is enforced as an acceptance criterion; all gh/DB coupling lives in plan 10-02's handler. This purity is what let the plan run as a wave-1 sibling parallel to plan 10-01 (which only touches internal/github/*)."
  - "computeTimeStat copies the input slice before sorting (never mutates the caller's slice) and surfaces Min/Max/Median as int64 seconds via int64(d.Seconds()) truncation. Even-N median = (sorted[n/2-1]+sorted[n/2])/2 as time.Duration ns-integer-division then truncated to seconds — the standard textbook, asserted by an explicit even-N parity test (Pitfall 6)."

patterns-established:
  - "Pattern: pure-helper extraction ahead of the handler — when a handler mixes DB/gh/external I/O with parse/validate/compute logic, extract the pure logic into its own stdlib-only file with its own unit tests so it ships + verifies independently and is reusable by sibling plans."
  - "Pattern: dual-layout time.Parse for mismatched-precision ISO — when two data sources store the same conceptual timestamp at different precisions (ms vs second), parse both layouts in order and return the first success; this is the foundation for any cross-source time comparison."

requirements-completed: [STATS-02, STATS-03]

# Coverage metadata (#1602) — one entry per shipped deliverable.
coverage:
  - id: D1
    description: "parseActivityTime — shared mismatched-precision ISO parser (ms .000Z then bare-Z); returns (time.Time, bool), false on empty/junk, never panics"
    requirement: STATS-02
    verification:
      - kind: unit
        ref: "internal/api/activity_helpers_test.go#TestParseActivityTime (ms precision instant, second precision instant, .000Z==bare-Z same instant, empty->false/zero, junk->false/zero)"
        status: pass
    human_judgment: false

  - id: D2
    description: "parseScope + parseWindow — the validation gate (global/workspace:N/project:N; week/month) whose errors the handler maps to HTTP 400"
    requirement: STATS-02
    verification:
      - kind: unit
        ref: "internal/api/activity_helpers_test.go#TestParseScope (7 cases: global default, empty->global, workspace:N, project:N, missing N, non-numeric N, unknown prefix)"
        status: pass
      - kind: unit
        ref: "internal/api/activity_helpers_test.go#TestParseWindow (week default, empty->week, month, junk->error)"
        status: pass
    human_judgment: false

  - id: D3
    description: "computeTimeStat — min/max/median over a duration slice as int64 seconds, with correct even-N median parity (Pitfall 6) and no caller-slice mutation"
    requirement: STATS-02
    verification:
      - kind: unit
        ref: "internal/api/activity_helpers_test.go#TestComputeTimeStat (empty->zero, odd-N middle, even-N two-middle average=9000s, truncation-to-int64-seconds)"
        status: pass
    human_judgment: false

  - id: D4
    description: "buildDurationSlices — cycle/dwellInProgress/dwellInReview with the Open Q3 dwellInProgress semantic (USER-CONFIRMED) and per-slice NULL/unparseable skip (Pitfall 7)"
    requirement: STATS-03
    verification:
      - kind: unit
        ref: "internal/api/activity_helpers_test.go#TestBuildDurationSlices (full In Progress->In Review->Done, skipped-In-Review fallback, NULL in_review_at excludes dwellInReview only, NULL in_progress_at excludes cycle+dwellInProgress, NULL done_at excludes all, unparseable skipped not panicked)"
        status: pass
    human_judgment: false

  - id: D5
    description: "rollupReviewState — ok/partial/degraded aggregate with stable priority error>auth_required>no_gh (the D-08 contract the handler rolls per-repo states into)"
    requirement: STATS-03
    verification:
      - kind: unit
        ref: "internal/api/activity_helpers_test.go#TestRollupReviewState (empty->ok, all-ok->ok, mixed->partial, ok+error->partial, error+auth_required+no_gh->error, no_gh+auth_required->auth_required, only-no_gh->no_gh)"
        status: pass
    human_judgment: false

  - id: D6
    description: "stdlib-only purity contract — activity_helpers.go imports neither internal/github nor database/sql (all gh/DB coupling deferred to plan 10-02's handler)"
    verification:
      - kind: automated_procedural
        ref: "grep -c 'internal/github' internal/api/activity_helpers.go == 0 AND grep -c 'database/sql' internal/api/activity_helpers.go == 0; import block is {fmt, sort, strconv, strings, time}"
        status: pass
    human_judgment: false

# Metrics
duration: 15 min
completed: 2026-07-30
status: complete
---

# Phase 10 Plan 03: Activity Pure Helpers Summary

**Stdlib-only pure helpers (parseActivityTime/parseScope/parseWindow/computeTimeStat/buildDurationSlices/rollupReviewState) + activityTask/timeStat/statsBlock wire types extracted from the future `GET /api/activity` handler, fully unit-tested and ready for plan 10-02 to import — the shared mismatched-precision ISO parser + in-Go cycle/dwell/median compute + review-state rollup.**

## Performance

- **Duration:** 15 min (focused execution; wall-clock 12:02–13:27 UTC included upfront 10-RESEARCH/CONTEXT reading and two hung full-`internal/api`-suite runs — see Issues Encountered)
- **Started:** 2026-07-30T12:02:38Z
- **Completed:** 2026-07-30T13:27:29Z
- **Tasks:** 1 (TDD: RED → GREEN)
- **Files created:** 2

## Accomplishments

- **Six pure helpers + three wire types** in a single stdlib-only file (`fmt, sort, strconv, strings, time` — no `database/sql`, no `internal/github`), so the plan ran as a wave-1 sibling parallel to 10-01 and lands with its own unit-test gate independent of the handler's DB/gh wiring.
- **`parseActivityTime` — the shared mismatched-precision ISO parser.** Dual-layout `time.Parse` loop: ms layout `2006-01-02T15:04:05.000Z` first (it requires the fraction — verified it fails on bare-`Z` inputs), then the bare `2006-01-02T15:04:05Z` layout. This ordered fallback is the foundation that lets `buildDurationSlices` (reaper ms `*_at`) and plan 10-02's reviews-window cutoff (gh second-precision `closedAt`) share one correct time comparison — never string-compare timestamps across precisions.
- **`parseScope` / `parseWindow` — the validation gate.** `parseScope` accepts exactly `global` (default on empty) / `workspace:N` / `project:N`, rejecting missing-N, non-numeric-N, and negative-N (defensive) with non-nil errors the handler maps to HTTP 400 (T-10-10). `parseWindow` accepts `week` (7·24h, default on empty) / `month` (30·24h), erroring on anything else.
- **`computeTimeStat` — min/max/median as int64 seconds.** Copies the input slice before sorting (never mutates the caller's), computes odd-N median = `sorted[n/2]` and even-N median = `(sorted[n/2-1]+sorted[n/2])/2` (ns-integer division, then truncated to seconds). The even-N parity test asserts median=9000s for `[1h,2h,3h,4h]` — catches the off-by-one "upper-middle only" / "wrong pair" bugs (Pitfall 6).
- **`buildDurationSlices` — the Open Q3 dwellInProgress semantic (USER-CONFIRMED).** For each task: no `done_at` → skip all slices (D-09); otherwise `cycle = done − inProgress`, `dwellInReview = done − inReview`, and `dwellInProgress = inReview − inProgress` when `in_review_at` is set (time in In Progress before entering In Review) OR `done − inProgress` when `in_review_at` is absent (the skipped-In-Review fallback). A NULL/unparseable `*_at` excludes the row from THAT slice only and never panics (Pitfall 7) — so `cycle.n` / `dwell*.n` may be `< taskCount`.
- **`rollupReviewState` — the D-08 aggregate.** Empty → `ok`; all-ok → `ok`; mix of ok + non-ok → `partial`; none-ok → worst present in stable priority `error > auth_required > no_gh` (unrecognized-and-none-ok defensively falls back to `error`, never falsely `ok`).
- **Full TDD discipline** — RED (stub panics + full test suite, every test fails) → GREEN (real bodies, every test passes). Gate sequence verified in git log.

## Task Commits

Each task was committed atomically via TDD RED → GREEN:

1. **Task 1 RED: failing tests for all six pure helpers** — `2986110` (test)
2. **Task 1 GREEN: implement the six pure helpers + wire types** — `67861b5` (feat)

**Plan metadata:** `docs(10-03): complete activity-pure-helpers plan` (this commit)

## Files Created/Modified

- `internal/api/activity_helpers.go` — `activityTask` / `timeStat` / `statsBlock` wire types (JSON-tagged; `timeStat` numeric fields are int64 seconds) + `parseActivityTime` / `parseScope` / `parseWindow` / `computeTimeStat` / `buildDurationSlices` / `rollupReviewState`. Import block is stdlib-only (`fmt, sort, strconv, strings, time`).
- `internal/api/activity_helpers_test.go` — six test functions covering every documented branch: ms/second/equal/empty/junk parse cases; the seven scope cases; week/month/junk window cases; empty/odd-N/even-N/truncation stat cases; the six buildDurationSlices paths (full / skipped-In-Review / NULL in_review / NULL in_progress / NULL done / unparseable); the seven rollup cases.

## Decisions Made

- **Dual-layout parse order is load-bearing.** Probed `time.Parse` before implementing: the `.000Z` layout parses `.123Z` and `.000Z` (with fraction) but FAILS on bare `Z` (no fraction); the bare-`Z` layout catches that case (and is lenient enough to also accept fractions, but the ms layout is tried first so precision is preserved). The plan's literal two-layout spec is correct as written — no deviation needed.
- **`computeTimeStat` copies before sorting.** The plan's `<action>` said "copy into a local slice and sort" — honored, so the caller's duration slice is never mutated (a correctness requirement the tests don't directly assert but the spec mandates).
- **Negative-N scope rejected defensively.** `strconv.ParseInt` accepts `-5`; the spec said "reject negative/non-numeric", so an explicit `id < 0 → error` guard was added. Not exercised by the documented test cases but is the spec-mandated validation (Rule 2: input validation at the trust boundary).
- **Unrecognized-and-none-ok rollup falls back to `"error"`.** The spec defines the priority only over `{error, auth_required, no_gh}`; an input with none of those and no `ok` is impossible from the gh classifier but defensively returns `"error"` (never falsely `ok`).

## Deviations from Plan

None — plan executed exactly as written. The two small defensive choices above (negative-N rejection, unrecognized-rollup fallback) are explicit spec mandates ("reject negative", "return the worst present"), not unplanned work.

## Issues Encountered

- **Full `internal/api` test suite hangs (pre-existing, out of scope).** Running `go test ./internal/api/...` (the whole package) exceeded the 120s timeout twice. The package carries heavy integration tests (real gh/PTY/process spawns) unrelated to this plan. Per the scope-boundary rule, this is a pre-existing environmental characteristic, NOT caused by this plan's changes — this plan only ADDED two isolated pure-helper files and modified no existing file, so existing tests cannot be affected. The plan's actual verification (`go test ./internal/api/ -run '<the six tests>'`, `go vet ./internal/api/`, the purity greps) all pass cleanly and quickly (<1s). No fix attempted — out of scope.

## User Setup Required

None — no external service configuration required. Pure Go standard-library helpers; no `gh`, no DB, no network.

## Next Phase Readiness

- **Ready for Plan 10-02** (`GET /api/activity` handler): all six helpers + the three wire types are importable from `package api`. The handler will `parseScope`/`parseWindow` the query params (mapping errors to 400), SQL-scan the in-window done rows into `activityTask`, call `buildDurationSlices` + `computeTimeStat` to fill `statsBlock`, and `rollupReviewState` over the per-repo `MergedClosedResult.State`s (from plan 10-01's `GetMergedClosed`) into the reviews sub-object's `state`. `parseActivityTime` is also the primitive plan 10-02's `aggregateReviews` reviews-window cutoff uses to compare the gh `closedAt` (second-precision) against the window start.
- **No blockers.** The function signatures/names match the plan's `artifacts_this_phase_produces` contract verbatim (the plan is the contract for 10-02). The stdlib-only purity gate is green.

---
*Phase: 10-activity-data-api*
*Completed: 2026-07-30*

## Self-Check: PASSED

- SUMMARY.md exists at `.planning/phases/10-activity-data-api/10-03-SUMMARY.md` ✓
- Task commits found in git log: `2986110` (test RED), `67861b5` (feat GREEN), in order (RED before GREEN — TDD gate clean) ✓
- `go build ./...` passes ✓
- `go test ./internal/api/ -run 'TestParseActivityTime|TestParseScope|TestParseWindow|TestComputeTimeStat|TestBuildDurationSlices|TestRollupReviewState' -count=1` passes ✓
- `go vet ./internal/api/` clean ✓
- Stdlib-only purity verified by grep: `internal/github` count == 0, `database/sql` count == 0; import block is `{fmt, sort, strconv, strings, time}` ✓
- Acceptance criteria: all six helpers + three wire types declared; parseActivityTime accepts both precisions; even-N median parity asserted (9000s); Open Q3 dwellInProgress branch + skipped-In-Review fallback both asserted; rollupReviewState returns "partial" exactly on ok/non-ok mix ✓
