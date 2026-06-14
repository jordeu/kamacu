---
phase: 12-open-a-review
plan: 01
subsystem: api
tags: [git, worktree, gh, github, pull-request, go]

# Dependency graph
requires:
  - phase: 10-github-foundations
    provides: "internal/github read-only gh leaf (Available/ValidateRepo, lenient author decode) + migration 00007 tasks.pr_number/pr_base_ref/source columns"
  - phase: 11-pr-review-column
    provides: "internal/github prlist.go (PRSummary carrying head/base refs, fetched-but-unrendered for Phase 12) + the gh arg-array + lenient-decode posture"
provides:
  - "worktree.CheckoutPR(ctx, repo, path, headOID, prNumber) — fetch refs/pull/<n>/head + worktree add --detach <headOID>, leaves the source repo HEAD provably unchanged (GHREV-01/05)"
  - "worktree.FetchRef(ctx, repo, ref) — best-effort origin fetch of a named ref so origin/<ref> exists for a later merge-base (reused by 12-03 diff base)"
  - "github.ViewPR(ctx, repo, n) (PRDetail, error) — fresh gh pr view --json read with body + freshest head/base OIDs, author.login flattened, gh-absence degrades to a typed error (GHREV-01)"
  - "github.PRDetail typed struct (number/title/body/authorLogin/url/head+base refName+Oid/isCrossRepository)"
affects: [12-02, 12-03, open-review-endpoint, diff-base, frontend-review-view]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Scoped fetch exception: worktree's never-fetch invariant (D-24) is breached deliberately only inside CheckoutPR/FetchRef, documented in the package header"
    - "Detached PR-head worktree: worktree add --detach <headOID> (NOT FETCH_HEAD, NOT a named branch, NOT gh pr checkout) — the GHREV-05 primary-checkout safety property"
    - "Fake-gh test stub: a shebang script using only the echo shell builtin under a temp-dir-only PATH exercises the full exec+decode path with no real gh / network"

key-files:
  created: []
  modified:
    - "internal/worktree/worktree.go - CheckoutPR + FetchRef beside Create; package header documents the scoped fetch exception"
    - "internal/worktree/worktree_test.go - makePRRemote helper (publishes refs/pull/<n>/head, drops the branch for a fork-PR shape) + 6 real-git tests"
    - "internal/github/github.go - PRDetail struct + ViewPR gh pr view reader"
    - "internal/github/github_test.go - gh-absent degrade, lenient-decode fixture, fake-gh end-to-end"

key-decisions:
  - "Pin the detached worktree to gh's headRefOid, never FETCH_HEAD — FETCH_HEAD is clobbered by 12-03's later base fetch (RESEARCH Pitfall 1)"
  - "Hard-code remote origin for v1.3 (every linked repo is a GitHub clone); remote-resolution left as future hardening (RESEARCH OQ3)"
  - "ViewPR does a FRESH gh pr view rather than reusing the Phase 11 PRSummary — the list omits body and reattach can fire with no list mounted"

patterns-established:
  - "Scoped fetch exception inside an otherwise never-fetch package, documented at the header"
  - "GHREV-05 proof as a unit test: capture source symbolic-ref + rev-parse before and after, assert byte-for-byte identical"
  - "Fork-PR test fidelity: publish refs/pull/<n>/head then delete the source branch so ONLY the pull ref resolves the head"

requirements-completed: [GHREV-01, GHREV-05]

# Metrics
duration: 6min
completed: 2026-06-14
---

# Phase 12 Plan 01: PR-Head Worktree + PR-Metadata Leaf Primitives Summary

**Detached PR-head worktree checkout (`worktree.CheckoutPR`, pinned to `headRefOid`, source HEAD provably unchanged) plus a fresh `gh pr view` reader (`github.ViewPR` → typed `PRDetail`) and a `worktree.FetchRef` helper — the two leaf verbs the rest of Phase 12 stands on.**

## Performance

- **Duration:** ~6 min
- **Started:** 2026-06-14T05:28:33Z
- **Completed:** 2026-06-14T05:33:53Z
- **Tasks:** 2 (both TDD)
- **Files modified:** 4

## Accomplishments

- `worktree.CheckoutPR` fetches `refs/pull/<n>/head` then `worktree add --detach <headOID>`, with the GHREV-05 safety property proven at the unit level: the source repo's branch and HEAD sha are byte-for-byte identical before and after, and no branch leaks. Fork PRs and same-repo PRs take the identical path (the pull ref resolves fork heads from the base repo).
- `worktree.FetchRef` ships as the one-line best-effort origin-fetch the 12-03 diff-base plan will call before its `merge-base`.
- `github.ViewPR` reads a fresh `gh pr view --json` payload into a typed `PRDetail` (carries `body`, the freshest head/base OIDs, author flattened from the `author.login` object), and degrades to a typed error — not a panic — when `gh` is absent.
- The `internal/worktree` package header now documents the deliberate, scoped fetch exception to its never-fetch invariant (D-24), so the breach is intentional and traceable.

## Task Commits

Each task was committed atomically (TDD: test → feat):

1. **Task 1: worktree.CheckoutPR + worktree.FetchRef**
   - `c2355ab` (test) — failing real-git tests
   - `3c8d4b8` (feat) — CheckoutPR + FetchRef + header update
2. **Task 2: github.ViewPR + PRDetail**
   - `609f2d1` (test) — failing gh-absent / lenient-decode / fake-gh tests
   - `1edd9fe` (feat) — PRDetail struct + ViewPR reader

_TDD tasks have two commits each (test → feat); no refactor commit was needed — both implementations were minimal and gofmt-clean as written._

## Files Created/Modified

- `internal/worktree/worktree.go` — Added `CheckoutPR` (detached PR-head provisioning) and `FetchRef` (best-effort origin fetch) beside `Create`; updated the package header to note the scoped fetch exception.
- `internal/worktree/worktree_test.go` — Added the `makePRRemote` helper and 6 real-git tests: detached-HEAD-equals-headOID, source-HEAD-unchanged (GHREV-05), no-branch-leak, path-collision pre-check, FetchRef happy path, FetchRef error.
- `internal/github/github.go` — Added the `PRDetail` struct and `ViewPR` (`gh pr view --json`), mirroring `ValidateRepo`'s arg-array exec + lenient `json.Unmarshal` posture.
- `internal/github/github_test.go` — Added gh-absent degrade, an inline lenient-decode fixture (author flatten + empty body), and a fake-gh end-to-end exec test.

## Decisions Made

- **Pin to `headRefOid`, never `FETCH_HEAD`** — FETCH_HEAD is overwritten by the 12-03 base fetch; the OID from `gh pr view` is unconditionally safe (RESEARCH Pitfall 1).
- **Hard-code `origin`** for v1.3 (matches the verified path; remote-resolution is noted future hardening).
- **Fresh `gh pr view` on open** rather than reusing the Phase 11 `PRSummary` — the list lacks `body` and a reattach can occur with no list mounted.
- Followed the plan's verbatim code for both verbs as specified.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- The fake-`gh` test stub initially used a `cat <<EOF` heredoc; under the test's temp-dir-only PATH, the shebang script couldn't resolve `cat` (exit 127). Resolved by rewriting the stub to use only the `echo` shell builtin with a JSON-compacted, quote-escaped single line — no external tools needed. This was a test-harness fix within Task 2, not a deviation in shipped code.

## User Setup Required

None - no external service configuration required. (`gh` and `git` are already present on the host; the new code degrades gracefully when `gh` is absent.)

## Next Phase Readiness

- `worktree.CheckoutPR`, `worktree.FetchRef`, and `github.ViewPR`/`PRDetail` are pure backend leaf functions with no callers yet — they ship independently, fully tested, ready for:
  - **12-02** (open-or-reattach endpoint): calls `ViewPR` then `CheckoutPR`.
  - **12-03** (diff base): calls `FetchRef` before the PR-base `merge-base`.
- Full Go suite green (`go test ./...`), `go build ./...` clean, `gofmt` clean. No blockers.

## Self-Check: PASSED

All created/modified files exist on disk; all four task commits (`c2355ab`, `3c8d4b8`, `609f2d1`, `1edd9fe`) are present in git history.

---
*Phase: 12-open-a-review*
*Completed: 2026-06-14*
