---
phase: 12-open-a-review
plan: 03
subsystem: api
tags: [go, git, diff, merge-base, github-pr, three-dot, source-discriminator]

# Dependency graph
requires:
  - phase: 12-open-a-review
    provides: "12-01 worktree.FetchRef (best-effort origin fetch of a named ref so origin/<ref> exists for a later merge-base) — called before the PR-base merge-base"
  - phase: 12-open-a-review
    provides: "12-02 source/pr_base_ref on the Task wire shape + the board-leak guard (this plan reads source/pr_base_ref straight from the row in diffs.go's own SELECT)"
  - phase: 10-github-foundations
    provides: "migration 00007 tasks.source (CHECK manual/github_pr, default manual) + tasks.pr_base_ref (nullable)"
provides:
  - "source-branched diff base in internal/api/diffs.go: a source='github_pr' task diffs against its OWN base (pr_base_ref) via FetchRef + local-then-remote resolvePRBase, matching GitHub Files-changed (GHREV-03)"
  - "resolvePRBase(ctx, wt, baseName) — prefers local refs/heads/<base> via show-ref, else origin/<base>; never FETCH_HEAD (RESEARCH Pitfall 1)"
affects: [12-04-open-endpoint, 12-05-frontend, phase-13-cleanup]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Diff-base discriminator: diffs.go's SELECT carries source + pr_base_ref so the handler picks the base; internal/diff (diff.Compute) is reused VERBATIM, never forked (D-12)"
    - "Fetch-before-merge-base: FetchRef(base) runs before resolvePRBase so origin/<base> exists (RESEARCH Pitfall 3); best-effort — the merge-base relays a clear error if the ref still can't resolve"
    - "local-then-remote PR-base resolution mirroring worktree.ResolveBase: show-ref --verify refs/heads/<base> in the worktree dir (worktrees share the common git dir), else origin/<base>"

key-files:
  created: []
  modified:
    - "internal/api/diffs.go - SELECT reads source/pr_base_ref; source branch picks the PR base (FetchRef + resolvePRBase) vs ResolveBase for manual; new resolvePRBase helper"
    - "internal/api/diffs_test.go - gitRepoForPRBase 3-branch fixture, newDiffServerDB/gitC/insertPRReviewRow harness, TestDiffPRReviewUsesPRBase + TestDiffManualTaskUnchangedAlongsidePR"

key-decisions:
  - "resolvePRBase prefers a LOCAL refs/heads/<base> (show-ref --verify --quiet) over origin/<base>, mirroring worktree.ResolveBase's local-then-remote chain — not the plan's fallback-only 'default to origin/<base>'. Reason: the correct three-dot merge-base needs a ref that actually resolves; a local base branch resolves with no network, and the test fixture (no origin remote) only resolves locally. RESEARCH §3 explicitly prefers this; the plan sanctioned it as the preferred-if-feasible shape."
  - "resolvePRBase runs show-ref in the WORKTREE dir (path), not the repo root — worktrees share the common git dir so refs/heads and refs/remotes resolve identically, and it matches where diff.Compute runs the merge-base"
  - "diff base discriminated entirely in diffs.go; internal/diff untouched (D-12) — diff.Compute is already correct three-dot (merge-base base HEAD), so only the base ref selection branches"

patterns-established:
  - "Three-branch discriminator fixture for diff-base correctness: main → feature-base (+basechange.txt) → pr-head (+prchange.txt); basechange.txt is the discriminator — ABSENT in the correct feature-base diff, PRESENT in a wrong main-base diff"
  - "Hand-mint a github_pr review row + detached PR-head worktree in tests (insertPRReviewRow + git worktree add --detach) so the diff-base path is testable before the open endpoint (12-04) exists"

requirements-completed: [GHREV-03]

# Metrics
duration: 4min
completed: 2026-06-14
---

# Phase 12 Plan 03: PR-Base Diff Selection Summary

**A `source='github_pr'` task's Diff tab now computes against the PR's OWN base (`pr_base_ref`) — `FetchRef(base)` first so `origin/<base>` exists, then a local-then-remote `resolvePRBase` feeds `diff.Compute`'s unchanged three-dot merge-base — so a PR targeting a non-default branch matches GitHub's Files-changed instead of showing a misleading mega-diff against `main`; the manual path stays byte-for-byte `ResolveBase`.**

## Performance

- **Duration:** ~4 min
- **Started:** 2026-06-14T05:46:31Z
- **Completed:** 2026-06-14T05:51:26Z
- **Tasks:** 1 (TDD)
- **Files modified:** 2

## Accomplishments

- `internal/api/diffs.go`'s SELECT now reads `source` and `pr_base_ref` alongside `worktree_path` + the project `repo_path`, and the handler branches the diff base on `source`.
- For a `github_pr` task with a non-empty `pr_base_ref`, the base is the PR's own base: a best-effort `h.wt.FetchRef(repo, baseName)` runs first (so `origin/<base>` exists, RESEARCH Pitfall 3), then `resolvePRBase` prefers a local `refs/heads/<base>` (via `show-ref --verify --quiet`) else `origin/<base>` — never `FETCH_HEAD` (Pitfall 1). `diff.Compute` is reused unchanged.
- The manual path (and a defensive `github_pr` row with NULL/blank `pr_base_ref`) keeps the existing `h.wt.ResolveBase(repo)` call verbatim.
- `internal/diff/diff.go` is **NOT touched** — the renderer's correct three-dot (`merge-base base HEAD`) semantics are reused; only the base ref selection branches (D-12).
- Locked the behavior behind a discriminating test: a three-branch fixture (`main` → `feature-base` → `pr-head`) where `basechange.txt` is the discriminator — proven RED (the old code diffed against `main` and included `basechange.txt` + reported base `main`), GREEN after the branch (base `feature-base`, `basechange.txt` absent, `prchange.txt` present). A sibling test proves a manual task in the SAME multi-branch repo still resolves via `ResolveBase` to `main`.

## Task Commits

The single TDD task was committed atomically (test → feat):

1. **Task 1: Branch the diff base on source/pr_base_ref**
   - `a227116` (test) — failing PR-base diff test + 3-branch fixture + harness helpers
   - `468079a` (feat) — diffs.go SELECT extension, source branch, resolvePRBase helper

_No REFACTOR commit — the implementation was minimal and gofmt-clean as written._

## Files Created/Modified

- `internal/api/diffs.go` — SELECT reads `worktree_path, source, pr_base_ref` + the project `repo_path`; the base selection branches on `source == "github_pr"` (FetchRef + resolvePRBase) vs the unchanged `ResolveBase` else-branch; new unexported `resolvePRBase(ctx, wt, baseName)` helper (local-then-remote, `show-ref` in the worktree dir).
- `internal/api/diffs_test.go` — `gitRepoForPRBase` three-branch fixture, `newDiffServerDB`/`gitC`/`insertPRReviewRow` harness helpers, `TestDiffPRReviewUsesPRBase` (the load-bearing discriminator), `TestDiffManualTaskUnchangedAlongsidePR` (negative control).

## Decisions Made

- **`resolvePRBase` prefers a LOCAL `refs/heads/<base>` over `origin/<base>`**, mirroring `worktree.ResolveBase`'s local-then-remote chain — not the plan's fallback-only "default to `origin/<base>`". The plan explicitly noted RESEARCH prefers the local-head check; the load-bearing constraint (a ref that actually resolves for `merge-base`) is satisfied for both a network-connected clone (`origin/<base>` after FetchRef) and a local base branch (the test fixture has no `origin` remote, so only the local ref resolves). The implementation is deterministic and grep-verifiable (`show-ref`).
- **`show-ref` runs in the worktree dir (`path`), not the repo root** — linked worktrees share the common git dir, so `refs/heads`/`refs/remotes` resolve identically, and it matches where `diff.Compute` runs the merge-base.
- **Diff base discriminated entirely in `diffs.go`; `internal/diff` untouched (D-12)** — `diff.Compute` is already correct three-dot, so only the base ref selection branches; the renderer is reused, not forked.

## Deviations from Plan

None - plan executed as written. The one nuance (resolvePRBase preferring the local head rather than the plan's fallback-only `origin/<base>`) is the plan's own stated preference ("RESEARCH prefers checking for a local head first"), documented above as a decision, not a deviation. No code logic deviated from the plan's verbatim handler branch.

## Issues Encountered

None. The RED phase failed for exactly the predicted reasons (base `main`, `basechange.txt` leaked into the PR diff); the GREEN phase passed on the first implementation pass. `go build ./...`, `go vet ./internal/api/`, and `go test ./internal/api/ ./internal/diff/ -count=1` all green; `gofmt -l` clean. Negative guards verified: the only `ResolveBase` call is in the manual else-branch (the github_pr branch never calls it); `FETCH_HEAD` appears only in NEVER-USE comments; `internal/diff/diff.go` is not in this plan's diff.

## User Setup Required

None - no external service configuration required. (`git` is already present on the host; `FetchRef` is best-effort and degrades to the merge-base error card if a base ref can't resolve.)

## Next Phase Readiness

- **12-04 (open endpoint):** when it inserts a `github_pr` row with `pr_base_ref = detail.BaseRefName`, the Diff tab is correct for free — no further diff wiring. The handler re-reads + re-resolves the base on every request, so a retargeted PR picks up its new base on the next Diff-tab open (D-12 "re-resolve on refresh").
- **12-05 (frontend):** zero Diff-tab UI change — the base ref change is entirely server-side; the existing `"diff"` tab lights up the moment `worktree_path` is set.
- No blockers. The diff-base path is the last Wave-2 backend leaf; both Wave-2 plans (12-02 board-leak/wire-shape, 12-03 diff-base) are now complete and independent of each other (12-03 reads `source`/`pr_base_ref` via its own SELECT, not via `taskColumns`).

## Self-Check: PASSED

All claimed files exist on disk; both task commits (`a227116`, `468079a`) are present in git history.

---
*Phase: 12-open-a-review*
*Completed: 2026-06-14*
