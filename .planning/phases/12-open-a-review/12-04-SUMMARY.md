---
phase: 12-open-a-review
plan: 04
subsystem: api
tags: [go, github-pr, worktree, open-reattach, idempotent-endpoint, find-or-create]

# Dependency graph
requires:
  - phase: 12-open-a-review
    provides: "12-01 worktree.CheckoutPR (detached PR-head provisioning) + github.ViewPR/PRDetail (fresh gh pr view)"
  - phase: 12-open-a-review
    provides: "12-02 source/pr_number/pr_base_ref on the Task wire shape (scanTask + taskColumns) + the board-leak guard that keeps github_pr rows off the board"
provides:
  - "POST /api/projects/{id}/pull-requests/{n}/review — open-or-reattach: find-or-create a source='github_pr' task keyed by (project_id, pr_number), provision a detached PR-head worktree, return {task, pr}"
  - "the live PRDetail wire object (number/title/body/author/url/baseRefName) returned alongside the task so the review header/body never drift from GitHub"
  - "the viewPR package seam (defaults to github.ViewPR) — the hermetic test injection point for the live gh read"
affects: [12-05-frontend]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Open action gates with 409 (not the GET's degraded state field): the review can't proceed without a linked repo, so off/unlinked/unknown all short-circuit BEFORE any gh/worktree spawn"
    - "find-or-create idempotency by (project_id, pr_number, source='github_pr'): full row -> instant reattach (no CheckoutPR); worktree_path NULL -> re-provision in place; not found -> INSERT"
    - "package-var seam (viewPR = github.ViewPR) lets the create/reattach tests stub the live gh read while the git half (CheckoutPR) runs against a real local refs/pull/<n>/head repo — fully hermetic, no gh, no network"

key-files:
  created: []
  modified:
    - "internal/api/pullrequests.go - the open-or-reattach POST handler + prWire envelope + viewPR seam + recordWorktreeError helper, beside the existing GET list"
    - "internal/api/pullrequests_test.go - makePRRepo (real refs/pull/<n>/head) + viewPR stub + gate/degrade/create/reattach/re-provision tests"
    - "cmd/kangent/main.go - PullRequestRoutes now passed wtSvc"

key-decisions:
  - "Live gh pr view on EVERY open (create AND reattach), not just create — a renamed PR never shows a stale title/body (D-08/D-11, Pitfall 6); reattach pays one extra gh call for freshness"
  - "ViewPR fetched BEFORE the INSERT on the create path so a gh failure -> 502 with NO half-created row (nothing to clean up)"
  - "position=0 on the INSERT: the schema's NOT NULL position is meaningless for a PR review (the 12-02 board-leak filters exclude github_pr rows from every position query), so a fixed sentinel satisfies the column without touching board math"
  - "viewPR is a package seam, not a *github.Service method — github.ViewPR is already a package-level func; the seam keeps create/reattach tests hermetic without a gh binary"

patterns-established:
  - "Open-or-reattach idempotent endpoint: find-by-key, branch on worktree_path validity (full=reattach / NULL=re-provision / absent=create), synchronous provision, return the ready row + live external detail"

requirements-completed: [GHREV-01, GHREV-02]

# Metrics
duration: 12min
completed: 2026-06-14
---

# Phase 12 Plan 04: Open-or-Reattach PR Review Endpoint Summary

**`POST /api/projects/{id}/pull-requests/{n}/review` turns a PR-card click into a review workspace: it find-or-creates a `source='github_pr'` task keyed by `(project_id, pr_number)`, provisions a detached PR-head worktree via `worktree.CheckoutPR`, and returns `{task, pr}` where `pr` is a fresh `gh pr view` so the header/body never drift from GitHub — composing the 12-01 leaf verbs and the 12-02 source plumbing into one idempotent endpoint.**

## Performance

- **Duration:** ~12 min
- **Tasks:** 1 (TDD)
- **Files modified:** 3
- **Completed:** 2026-06-14

## Accomplishments

- Added the open-or-reattach POST endpoint beside the existing GET list in `pullrequests.go`. It mirrors the GET's two-gate ladder (toggle → link) but returns **409** instead of a degraded state field: the open action can't proceed without a linked repo, and both gates (plus unknown-project) short-circuit **before any gh/worktree spawn** (a `viewPR`-must-never-be-called guard in the gate tests proves this).
- **GHREV-02 reattach** keyed on `project_id + pr_number + source='github_pr'`: a fully provisioned review returns the **same task id** with NO `CheckoutPR` (instant reattach); a row whose `worktree_path` is NULL (a prior failed provision) **re-provisions in place** under the existing id; not-found **INSERTs** a fresh row. The create-then-reattach test asserts one row, same id, and an intact worktree dir across reopens.
- **GHREV-01 provisioning** via `wtSvc.CheckoutPR(ctx, repoPath, prWtPath, detail.HeadRefOid, n)` — detached, pinned to gh's `headRefOid`, NEVER `gh pr checkout`, NEVER `wtSvc.Create`, NEVER a named branch (all four negative guards grep-verified at 0).
- **No GitHub drift (Pitfall 6):** the endpoint does a fresh `gh pr view` on **every** open (create AND reattach) and returns the live `{number, title, body, author, url, baseRefName}` as a sibling `pr` object; the frontend (12-05) reads `task.source` to branch and `pr.*` for the read-only header / PR meta line / Description.
- **Degrade-don't-break:** a gh `ViewPR` failure → **502** with no half-created row (the fetch runs before the INSERT); a worktree provision failure → 502 with the error stamped in `worktree_error` for the Retry path, the row preserved.
- Introduced the `viewPR` package seam (defaults to `github.ViewPR`) so the create/reattach tests stay hermetic — the git half runs against a real local repo carrying a `refs/pull/<n>/head` ref (the fork-PR shape), the gh half is stubbed. No real gh, no network, no `t.Skip`.

## Task Commits

TDD (test → feat); no REFACTOR commit needed (gofmt + vet clean as written):

1. **Task 1: The open-or-reattach review endpoint**
   - `7a66d12` (test) — failing gate/degrade/create/reattach/re-provision tests + `makePRRepo` + `viewPR` stub
   - `e96d1ca` (feat) — the POST handler + `prWire` envelope + `viewPR` seam + `main.go` wiring

## Files Created/Modified

- `internal/api/pullrequests.go` — Added the `POST .../{n}/review` handler (find-or-create + CheckoutPR + live ViewPR), the `prWire` response struct, the `writeReview` envelope helper, the `recordWorktreeError` D-25 helper, and the `viewPR` seam var; extended `PullRequestRoutes` with `wtSvc *worktree.Service`.
- `internal/api/pullrequests_test.go` — Added `prPost`, `gitInit`, `makePRRepo` (real `refs/pull/<n>/head` + self-`origin`), `stubViewPR`, `newPRReviewEnv`, and the suite: toggle-off/unlinked/unknown → 409, bad PR number → 400, ViewPR-failure → 502 (no half-row), create-then-reattach (one row, same id, live detail), re-provision-when-NULL.
- `cmd/kangent/main.go` — `api.PullRequestRoutes(mux, db, ghSvc, wtSvc)`.

## Decisions Made

- **Fresh `gh pr view` on every open, including reattach** — the cost is one gh call per open (not per-poll); the payoff is a reattached review never showing a renamed PR's old title/body (D-08/D-11, Pitfall 6).
- **Fetch ViewPR before the INSERT** so the create path has nothing to roll back on a gh failure (clean 502).
- **`position = 0` on the INSERT** — the `position` column is `NOT NULL` with no default, but a PR review never participates in board positioning (the 12-02 `source='manual'` filters exclude it from all five board/position queries). A fixed sentinel satisfies the column without perturbing manual-task math. (Plan's INSERT omitted `position`; this is a Rule 3 blocking-issue fix surfaced by the NOT NULL constraint.)
- **`viewPR` as a package seam** (not a Service method) — `github.ViewPR` is already package-level; the seam is the lightest indirection that makes create/reattach hermetic, exactly the plan's preferred approach over `t.Skip`-gating on real gh.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] `tasks.position` NOT NULL constraint on the INSERT**
- **Found during:** Task 1 GREEN (first test run)
- **Issue:** The plan's verbatim INSERT (`INSERT INTO tasks (project_id, title, status, source, pr_number, pr_base_ref) VALUES (...)`) omitted `position`, which is `REAL NOT NULL` with no schema default — the create path failed with `NOT NULL constraint failed: tasks.position`.
- **Fix:** Added `position` to the INSERT column list with a fixed `0` value, documented inline as safe because the 12-02 board-leak filters exclude `github_pr` rows from every position query.
- **Files modified:** `internal/api/pullrequests.go` (handler INSERT), `internal/api/pullrequests_test.go` (the re-provision test's seed INSERT had the same omission).
- **Commit:** `e96d1ca`

No other deviations — the gate ladder, find-or-create branching, CheckoutPR-with-HeadRefOid, ViewPR-on-open, and the `{task, pr}` return shape all landed per the plan's verbatim shape.

## Issues Encountered

- The `position` NOT NULL constraint (above) was the only surprise; both the handler and the test seed needed it. Caught immediately by the first GREEN run and fixed within the same task.

## User Setup Required

None — no external service configuration. `gh` and `git` are already present on the host; the endpoint degrades to a 502 the UI surfaces when `gh` is absent/unauthenticated.

## Known Stubs

None — the endpoint is fully wired (real CheckoutPR provisioning, real live ViewPR, real DB find-or-create). The only frontend-facing consumer (the `useOpenReview` mutation + the `source==='github_pr'` TaskPage deltas) is plan 12-05's scope.

## Next Phase Readiness

- **12-05 (frontend):** the `{task, pr}` envelope is ready — `task.source==='github_pr'` is the discriminator, `pr.{title,body,author,url,baseRefName}` are the live header/meta/Description fields. The card's open mutation routes to `/projects/{id}/tasks/{task.id}`; reattach returns the same id (indistinguishable from opening a task). The in-flight guard (disable the card while pending) is the frontend's job per RESEARCH §5/§6.
- Full Go suite green (`go test ./...`), `go build ./...` clean, gofmt + vet clean. No blockers.

## Self-Check: PASSED

- `internal/api/pullrequests.go`, `internal/api/pullrequests_test.go`, `cmd/kangent/main.go` all exist on disk.
- Commits `7a66d12` (test) and `e96d1ca` (feat) present in git history.
- `POST /api/projects/{id}/pull-requests/{n}/review` registered; `PullRequestRoutes` signature includes `wtSvc *worktree.Service`.
- Reattach key `pr_number = ? AND source = 'github_pr'` present (1); `CheckoutPR(...detail.HeadRefOid...)` present; `viewPR(r.Context(), ghRepo...)` present; 3 × `http.StatusConflict` gates; `main.go` wires `wtSvc`.
- Negative guards: `gh pr checkout` = 0, `wtSvc.Create` = 0.
- `go build ./...` passes; `go test ./internal/api/ -run 'PullRequest|Review|PR' -count=1` passes; full `go test ./...` green.

---
*Phase: 12-open-a-review*
*Completed: 2026-06-14*
