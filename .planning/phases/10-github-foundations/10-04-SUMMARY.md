---
phase: 10-github-foundations
plan: 04
subsystem: api
tags: [github, gh-cli, http, soft-dependency, degrade-dont-break]

# Dependency graph
requires:
  - phase: 10-github-foundations (Plan 10-02)
    provides: internal/github leaf package with Available() call-time LookPath signal
provides:
  - GET /api/github/status — always-200 endpoint reporting {"gh_available": bool} from github.Available()
  - Backend signal for the frontend to gate the GitHub integration toggle's default/enable behavior
affects: [10-05, frontend-settings, github-pr-review]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Package-level stateless handler (no DB) registered via mux.HandleFunc, distinct from the projectHandlers methods"
    - "Always-200 degrade endpoint modeled on internal/quota: gh is a soft dependency, the read never errors or blocks"
    - "Contract test compares the endpoint to its source of truth (github.Available()) instead of a hardcoded bool, staying green on any host"

key-files:
  created:
    - internal/api/github.go
    - internal/api/github_test.go
  modified:
    - internal/api/routes.go

key-decisions:
  - "gh_available is the exact snake_case JSON field name (Plan 10-05 reads it); the handler is a package-level func (no DB needed), not a projectHandlers method"
  - "Endpoint reuses github.Available() (call-time LookPath) so install/uninstall reflects without a server restart — no new gh logic, no new dependency"
  - "Contract test pins status=200 + boolean gh_available = github.Available() in-process, so it is deterministic whether gh is present or absent on the runner (gh is absent on this host)"

patterns-established:
  - "Backend gh-availability surface: always-200 GET /api/github/status, the frontend's single read for is-gh-installed"

requirements-completed: [GHSET-01, GHSET-03]

# Metrics
duration: 4min
completed: 2026-06-13
---

# Phase 10 Plan 04: GitHub Status Endpoint Summary

**Always-200 `GET /api/github/status` returning `{"gh_available": bool}` from `github.Available()`, giving the frontend a call-time read of whether the host `gh` CLI is installed — the backend half of Gap 1.**

## Performance

- **Duration:** ~4 min
- **Started:** 2026-06-13T17:23:00Z
- **Completed:** 2026-06-13T17:26:00Z
- **Tasks:** 1
- **Files modified:** 3 (2 created, 1 modified)

## Accomplishments
- New `githubStatus` handler in `internal/api/github.go` returns `{"gh_available": <bool>}` with HTTP 200, reading `github.Available()` at call time (modeled on the `internal/quota` degrade pattern — never errors, never blocks).
- Route `GET /api/github/status` registered in `Routes()`, beside the existing `GET /api/projects/{id}/github-origin`.
- TDD contract test `TestGithubStatus` pins the contract (status 200, boolean field named exactly `gh_available`, value = `github.Available()`) and is green on any host.

## Task Commits

Each task was committed atomically (TDD: test → feat):

1. **Task 1: Add GET /api/github/status endpoint (TDD) — RED** - `066a5d9` (test)
2. **Task 1: Add GET /api/github/status endpoint (TDD) — GREEN** - `275de67` (feat)

**Plan metadata:** see final docs commit.

_No REFACTOR step: the handler is a single minimal `writeJSON` line, nothing to clean up._

## Files Created/Modified
- `internal/api/github.go` - `githubStatus` handler returning `{"gh_available": github.Available()}`, always 200
- `internal/api/github_test.go` - `TestGithubStatus` contract test (200, boolean `gh_available` = `github.Available()`)
- `internal/api/routes.go` - registered `mux.HandleFunc("GET /api/github/status", githubStatus)`

## Decisions Made
- **`gh_available` snake_case field, package-level func:** the JSON field name is exactly `gh_available` (matching the project's snake_case convention; Plan 10-05 reads it). The handler takes no DB, so it is a package-level function rather than a `projectHandlers` method.
- **Reuse `github.Available()` (call-time LookPath):** install/uninstall of `gh` is reflected without a server restart, mirroring the existing `internal/github` and `AllowedShells`/tmux LookPath patterns. No new gh logic, no new dependency.
- **Contract test, not a hardcoded bool:** the test compares the endpoint's answer to `github.Available()` evaluated in the test process, so it stays green whether or not `gh` is installed on the runner (`gh` is absent on this host — `github.Available()` returns `false`, and the endpoint matched).

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None. RED failed for the correct reason (route 404), GREEN passed, the full `internal/api` suite stayed green, `go build ./...` succeeded, and `gofmt -l` printed nothing.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Backend half of Gap 1 is closed: the frontend now has an always-200 read of `gh` availability.
- Plan 10-05 (Wave 2) consumes `GET /api/github/status` (`gh_available`) to gate the `/settings` GitHub Switch (default off + block enable when `gh` is missing, with install-`gh` guidance) and to drop the over-claiming toggle copy (Gap 2). No frontend changes were made in this plan, as specified.

## Self-Check: PASSED

- FOUND: internal/api/github.go
- FOUND: internal/api/github_test.go
- FOUND: internal/api/routes.go
- FOUND: .planning/phases/10-github-foundations/10-04-SUMMARY.md
- FOUND commit: 066a5d9 (test)
- FOUND commit: 275de67 (feat)

---
*Phase: 10-github-foundations*
*Completed: 2026-06-13*
