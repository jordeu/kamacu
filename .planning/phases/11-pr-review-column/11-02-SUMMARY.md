---
phase: 11-pr-review-column
plan: 02
subsystem: api
tags: [go, http, servemux, github, settings-gate, always-200, tdd]

# Dependency graph
requires:
  - phase: 11-pr-review-column (Plan 01)
    provides: "github.Service.Get(ctx, repo, repoDir, force) → Result{State,Stale,FetchedAt,PRs}; github.New(Config{}) production-default constructor"
  - phase: 10-github-foundations
    provides: "settings.KeyGithubIntegration KV (code default 'on'); projects.github_repo + repo_path columns; pathID/writeJSON API helpers"
  - phase: 07-claude-quota-indicator
    provides: "UsageRoutes always-200 one-HandleFunc pattern cloned here"
provides:
  - "GET /api/projects/{id}/pull-requests[?refresh=1] — always-200, toggle-gated (GHSET-02) + link-gated endpoint proxying github.Service.Get"
  - "api.PullRequestRoutes(mux, db, *github.Service) sibling registration (not inside Routes())"
  - "main.go wiring: github.New(github.Config{}) + PullRequestRoutes, mirroring quotaSvc/UsageRoutes"
affects:
  - "11-03 (frontend usePullRequests hook consumes this endpoint's {state,stale,fetchedAt,prs} contract)"
  - "12-open-a-review (the same endpoint surfaces the PR list the review-open flow reads)"

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Always-200 endpoint with a two-gate ladder (toggle gate then link gate) that short-circuits to state=disabled BEFORE any gh spawn (GHSET-02 backend enforcement)"
    - "Degrade-don't-break on project-lookup miss: unknown id returns 200+disabled, never 404"
    - "Service registered as a main.go sibling (like UsageRoutes), keeping api.Routes()/newTestServer untouched and letting the endpoint test inject a fake-runner Service"

key-files:
  created:
    - internal/api/pullrequests.go
    - internal/api/pullrequests_test.go
  modified:
    - cmd/kangent/main.go

key-decisions:
  - "Backend toggle read via settings.Get(db, settings.KeyGithubIntegration) with val != \"on\" comparison — absent row = code default 'on', any non-'on' disables (RESEARCH Open Q1)"
  - "SELECT fetches BOTH github_repo and repo_path; repo_path becomes cmd.Dir so gh resolves the right host/account (Pitfall 6 / D-00b)"
  - "Unknown project id returns 200+disabled (NOT 404) — the column never blocks the board on a lookup miss (degrade-don't-break)"
  - "settings.Get / db query errors map to state=error (200), never a 5xx — the only non-200 is the 400 from pathID on a non-numeric id"

patterns-established:
  - "Two-gate always-200 handler: pathID → settings toggle gate → project-link gate → svc.Get; every branch writes http.StatusOK with a typed State"
  - "Endpoint contract tests register PullRequestRoutes on their own mux with a call-counting fake-runner Service (no real gh), asserting calls.Load()==0 proves the gates short-circuit before gh"

requirements-completed: [GHCOL-02, GHCOL-04, GHCOL-05]

# Metrics
duration: 5min
completed: 2026-06-13
---

# Phase 11 Plan 02: Always-200 PR Review Endpoint Summary

**`GET /api/projects/{id}/pull-requests[?refresh=1]` — a thin always-200 consumer of the Plan 01 `github.Service`, gated server-side by the `github_integration` toggle (GHSET-02) AND the project's `github_repo` link, returning `{state, stale, fetchedAt, prs}` with degradation riding the `state` field; wired in `main.go` exactly like `quotaSvc`/`UsageRoutes`.**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-06-13T18:06:15Z
- **Completed:** 2026-06-13T18:11:04Z
- **Tasks:** 2 (Task 1 TDD)
- **Files modified:** 2 created, 1 modified

## Accomplishments
- A two-gate always-200 handler: GATE 1 reads `settings.KeyGithubIntegration` and returns `state=disabled` (no gh) when it is not `"on"` (GHSET-02 backend enforcement — independent of the frontend); GATE 2 returns `state=disabled` (no gh) when the project is unlinked OR unknown. A linked+on project proxies `github.Service.Get(repo, repo_path, refresh=="1")`.
- The endpoint NEVER returns a non-200 for a valid id — degradation rides `state` (GHSET-03/GHCOL-05); the only non-200 is the 400 from `pathID` on a non-numeric id, asserted by a dedicated test.
- `gh` runs with `cmd.Dir = repo_path` because the handler's `SELECT github_repo, repo_path` threads both through to the Service (Pitfall 6 / D-00b) — verified by a test asserting the fake runner saw `repo == github_repo` and `repoDir == repo_path`.
- 7 contract tests (toggle-off, unlinked, unknown-project, ok-when-linked, refresh-forces-fetch, bad-id-400, never-non-200) all green; the disabled paths assert `calls.Load()==0` proving the gates short-circuit before any gh spawn. Full `go build ./...` + full `go test ./...` suite green.

## Task Commits

Each task was committed atomically (Task 1 TDD: test → feat):

1. **Task 1 (RED): failing PR endpoint contract tests** - `df8dab5` (test)
2. **Task 1 (GREEN): pullrequests.go endpoint** - `4b4f51d` (feat)
3. **Task 2: main.go wiring** - `93fc0b7` (feat)

**Plan metadata:** _(this commit)_ (docs: complete plan)

_No REFACTOR commit — the GREEN implementation was already gofmt-clean and minimal._

## Files Created/Modified
- `internal/api/pullrequests.go` - `PullRequestRoutes(mux, db, *github.Service)`: the two-gate always-200 handler for `GET /api/projects/{id}/pull-requests[?refresh=1]`
- `internal/api/pullrequests_test.go` - 7 contract tests via a call-counting fake-runner Service on a private mux: toggle-off / unlinked / unknown / ok / refresh-forces-fetch / bad-id / never-non-200
- `cmd/kangent/main.go` - added `"kangent/internal/github"` import; `ghSvc := github.New(github.Config{})` + `api.PullRequestRoutes(mux, db, ghSvc)` right after the quota wiring

## Decisions Made
- **Toggle read = `settings.Get(db, settings.KeyGithubIntegration)` with `val != "on"`** — reuses the Phase 10 read-at-use path (absent row = code default `"on"`), and the `!= "on"` form disables on any non-`"on"` value including a future explicit `"off"` (RESEARCH Open Q1, resolved as the plan prescribed).
- **Unknown project → 200+disabled, not 404** — the plan baked this in deliberately: the review column must never block the board on a project-lookup miss. Added an explicit `TestPullRequestsUnknownProjectDisabled` to lock the contract.
- **`settings.Get` / DB-query errors → `state=error` (200)** — never a 5xx, so the always-200 invariant holds even on an infrastructure fault; the frontend still branches on `state`.

## Deviations from Plan

None - plan executed exactly as written. (The plan's prescribed handler code was used verbatim; the only additions are extra defensive tests — `TestPullRequestsUnknownProjectDisabled` and `TestPullRequestsNeverNon200ForValidId` — both within the plan's stated `<behavior>` of "the test asserts the status is NEVER non-200 for a valid id".)

## Issues Encountered
- First draft of the test file introduced an over-abstracted `sqlHandle` wrapper to avoid importing `database/sql`. This was unnecessary (the existing `projects_test.go`/`usage_test.go` import `database/sql` freely) and would not have compiled cleanly. Rewrote the test to import `database/sql` directly with a simple `prFake` struct + `insertProject` helper before the RED commit — no wasted commit.

## User Setup Required
None - no external service configuration required. (`gh` remains a soft dependency: when the toggle is on and a repo is linked, the Service degrades to `no_gh`/`auth_required`/`error` without blocking; when off or unlinked the endpoint returns `disabled` without ever touching gh.)

## Next Phase Readiness
- **Plan 11-03 (frontend)** can build `usePullRequests(projectId)` / `useRefreshPullRequests(projectId)` against `GET /api/projects/${projectId}/pull-requests[?refresh=1]` — the `{state, stale, fetchedAt, prs}` JSON contract is live and always-200, so the hook branches on `state`/`stale` (quota precedent), never on HTTP status. `?refresh=1` is the manual-refresh write-back path.
- The `PRSummary` wire shape (number/title/author/updatedAt/url/checks + the fetched-unrendered HeadRef*/BaseRef* fields) from Plan 01 is what this endpoint serializes — Plan 11-03 mirrors it verbatim into the TS interface.

---
*Phase: 11-pr-review-column*
*Completed: 2026-06-13*

## Self-Check: PASSED

- All created/modified files present (internal/api/pullrequests.go, internal/api/pullrequests_test.go, cmd/kangent/main.go, 11-02-SUMMARY.md).
- All 3 task commits present in git history (df8dab5, 4b4f51d, 93fc0b7).
- `go build ./...` + full `go test ./...` suite green; all 7 TestPullRequests subtests pass.
