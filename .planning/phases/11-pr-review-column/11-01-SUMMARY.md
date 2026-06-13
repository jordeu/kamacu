---
phase: 11-pr-review-column
plan: 01
subsystem: api
tags: [go, gh-cli, github, exec, cache, statusCheckRollup, tdd]

# Dependency graph
requires:
  - phase: 10-github-foundations
    provides: "internal/github leaf (Available/ParseRepoRef/ValidateRepo), gh as a soft dependency, projects.github_repo + repo_path columns"
  - phase: 07-claude-quota-indicator
    provides: "internal/quota.Service state machine (TTL/floor/backoff/in-flight-dedup/drop-on-N) cloned here"
provides:
  - "github.PRSummary wire struct (fetch-rich, render-minimal; checks reduced server-side)"
  - "github.reduceChecks: statusCheckRollup → pass|fail|pending|none (both __typename variants, SKIPPED/NEUTRAL/STALE non-failing)"
  - "github.runGH: verified `gh pr list` arg array + no_gh|auth_required|error classification, never spawns when gh absent"
  - "github.Service.Get(ctx, repo, repoDir, force) → typed Result{State,Stale,FetchedAt,PRs}, per-repo cache (quota clone)"
affects: [11-02 (endpoint consumes Service.Get), 11-03 (PRSummary wire shape → TS interface), 12-open-a-review (HeadRef*/BaseRef* fetched here), 13-pr-worktree-auto-cleanup]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Best-effort per-repo cache Service (quota.Service clone, keyed map[owner/name]*repoEntry)"
    - "Server-side gh exit-code + stderr classification into typed degraded states"
    - "Two-timestamp cache gate: fetchedAt (60s TTL) vs lastAttempt (10s floor binding even force)"

key-files:
  created:
    - internal/github/prlist.go
    - internal/github/prlist_test.go
    - internal/github/service.go
    - internal/github/service_test.go
  modified: []

key-decisions:
  - "PRSummary fetches the rich --json set (HeadRef*/BaseRef*/IsCrossRepository) but renders minimal in Phase 11 — one gh call serves Phases 12/13 (D-01/D-02)"
  - "reduceChecks maps SKIPPED/NEUTRAL/STALE to non-failing (Pitfall 1) — only FAILURE/ERROR/TIMED_OUT/CANCELLED/ACTION_REQUIRED/STARTUP_FAILURE go red"
  - "auth classification combines exit-code-4 OR stderr substrings (gh auth login / 401 / Bad credentials) — exit code 4 alone is unreliable (cli/cli#9338)"
  - "Service drops backoffUntil entirely (no 429 field): gh rate-limit surfaces as 'error' with serve-stale per D-11, so a backoff scheme would be invented complexity"
  - "ISO-8601 lexical string compare used for the updatedAt-desc sort (D-14) — no time.Parse needed since RFC3339 strings sort chronologically"

patterns-established:
  - "github.Service is a structural clone of quota.Service retargeted from token-fingerprint keying to map[owner/name]*repoEntry"
  - "Config{Now, Runner} construction seam: tests inject a frozen clock + a call-counting fake runner (no real gh spawn in unit tests)"

requirements-completed: [GHCOL-02, GHCOL-03, GHCOL-05]

# Metrics
duration: 6min
completed: 2026-06-13
---

# Phase 11 Plan 01: PR Review Backend Leaf Summary

**`internal/github` gains a verified `gh pr list` shell-out with server-side statusCheckRollup→pass/fail/pending/none reduction and a per-repo cache `Service` (a quota.Service clone) that returns a typed `Result{State,Stale,FetchedAt,PRs}` and degrades to no_gh/auth_required/error without ever panicking.**

## Performance

- **Duration:** ~6 min
- **Started:** 2026-06-13T17:58:03Z
- **Completed:** 2026-06-13T18:03:30Z
- **Tasks:** 2 (both TDD)
- **Files modified:** 4 created, 0 modified (github.go untouched as required)

## Accomplishments
- `reduceChecks` collapses the live-verified two-variant statusCheckRollup array to one pill value, with SKIPPED/NEUTRAL/STALE pinned non-failing (the Pitfall-1 regression that would otherwise paint ~30% of real PRs red) — 23 table cases + a SKIPPED-heavy guard, all green.
- `runGH` spawns the exact verified `gh pr list -R … --search "user-review-requested:@me draft:false" --state open --json …` arg array with `cmd.Dir = repoDir` (multi-account/enterprise correctness, Pitfall 6), classifies failures into `no_gh`/`auth_required`/`error`, defensively skips drafts, and sorts most-recently-updated first.
- `github.Service` is a structural clone of `quota.Service` re-keyed to `map[owner/name]*repoEntry`: 60s success-TTL on `fetchedAt`, 10s hard floor on `lastAttempt` (binds even `force`), in-flight dedup, drop-cached-after-3-consecutive-failures — verified by 7 gate-ladder subtests via an injected call-counting fake runner.
- The whole package compiles, `go build ./...` and the full `go test ./...` suite are green; no `sh -c`, no stored credentials.

## Task Commits

Each task was committed atomically (TDD: test → feat):

1. **Task 1 (RED): failing reduceChecks tests** - `0df9f9e` (test)
2. **Task 1 (GREEN): prlist.go** - `8c50f6a` (feat)
3. **Task 2 (RED): failing Service tests** - `b7e7d9f` (test)
4. **Task 2 (GREEN): service.go + gofmt** - `4896799` (feat)

**Plan metadata:** _(this commit)_ (docs: complete plan)

## Files Created/Modified
- `internal/github/prlist.go` - PRSummary/checkEntry/prRaw shapes; `reduceChecks` (pill reduction); `runGH` (gh shell-out + classification + draft filter + updatedAt-desc sort)
- `internal/github/prlist_test.go` - `TestReduceChecks` (23 cases, both __typename variants + precedence) and `TestReduceChecksSkippedHeavy` (Pitfall-1 guard)
- `internal/github/service.go` - `Result`/`repoEntry`/`Config`/`Service`/`New`, the `Get` gate ladder, per-entry `recordFailureLocked`/`resultLocked`, the TTL/floor/maxFailures constant block
- `internal/github/service_test.go` - `TestServiceGet` (happy path, TTL-caches, force-bypasses-TTL, floor-binds-force, per-repo-isolation, no_gh-degrade, drop-after-3-failures) via injected fake runner + frozen clock

## Decisions Made
- **No 429/backoff field on the Service.** The plan permitted keeping a zero `backoffUntil`; per D-11 gh rate-limit surfaces as `error` with serve-stale, so the field was omitted entirely to avoid an unused invented backoff scheme. The gate ladder is otherwise identical to quota (the `now.Before(e.backoffUntil)` clause is simply absent).
- **Lexical ISO-8601 sort for D-14.** RFC3339 strings sort chronologically as strings, so `out[i].UpdatedAt > out[j].UpdatedAt` is correct without `time.Parse` — leaner and avoids a parse-error path.

## Deviations from Plan

None - plan executed exactly as written. (The two "Decisions Made" above are choices the plan explicitly left to Claude's discretion — the no-backoff option is sanctioned in the plan's constant-block note, and the sort approach is an internal detail of "sort by UpdatedAt descending using sort.Slice".)

## Issues Encountered
- An acceptance grep (`grep -q 'sh -c'` must return nothing) initially matched a doc comment that read "...NEVER uses `sh -c`...". Reworded the comment to "...NEVER shell-interpolates..." so the literal token is absent anywhere in the file; behavior unchanged (the command was always an arg array). Verified the grep now returns nothing and the build still passes.

## User Setup Required
None - no external service configuration required. (`gh` is a soft dependency: when absent the Service returns `no_gh` and never blocks.)

## Next Phase Readiness
- **Plan 11-02 (endpoint)** can wire `github.New(github.Config{})` in `main.go` and call `svc.Get(ctx, repo, repoDir, refresh=="1")` directly — the Result JSON shape (`state`/`stale`/`fetchedAt`/`prs`) is the always-200 endpoint contract.
- **Plan 11-03 (frontend)** can mirror `PRSummary`'s JSON tags into the TS `PRSummary` interface verbatim.
- **Phase 12** already has `HeadRefName`/`HeadRefOid`/`BaseRefName`/`IsCrossRepository` populated by this plan's single gh call — no re-fetch needed for the PR-branch checkout.

---
*Phase: 11-pr-review-column*
*Completed: 2026-06-13*

## Self-Check: PASSED

- All 4 created source files present (prlist.go, prlist_test.go, service.go, service_test.go).
- SUMMARY.md present.
- All 4 task commits present in git history (0df9f9e, 8c50f6a, b7e7d9f, 4896799).
