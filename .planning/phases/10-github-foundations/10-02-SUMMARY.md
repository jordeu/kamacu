---
phase: 10-github-foundations
plan: 02
subsystem: api
tags: [go, github, gh-cli, projects, rest, partial-patch, git-remote]

# Dependency graph
requires:
  - phase: 10-github-foundations (Plan 01)
    provides: "migration 00007 (projects.description, projects.github_repo) + github_integration settings KV"
provides:
  - "internal/github leaf package: ParseRepoRef (owner/name + URL/ssh canonicalization), ValidateRepo (soft gh repo view), Available (LookPath)"
  - "Project JSON/columns/scan grown by description (NOT NULL) + github_repo (*string, JSON null when unlinked)"
  - "PATCH /api/projects/{id} as a partial update accepting name, description, github_repo (soft-validated, canonicalized, unlinkable)"
  - "GET /api/projects/{id}/github-origin: on-open origin auto-detect canonicalized to owner/name"
affects: [10-03-frontend, 11-pr-review-column, 12-pr-tasks, 13-pr-cleanup]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "internal/github best-effort leaf: stdlib + os/exec only, arg-array exec (never sh -c), gh as a soft dependency that degrades (syntactic save, no error) on absence or unverifiable refs"
    - "Partial PATCH (D-13): pointer-typed JSON fields so an OMITTED key is untouched while an explicit \"\" clears/unlinks; dynamic UPDATE built from present fields with RETURNING projectColumns"
    - "Dedicated on-open read endpoint for an expensive shell-out (git remote) so the project list never pays for it (D-08)"

key-files:
  created:
    - internal/github/github.go
    - internal/github/github_test.go
  modified:
    - internal/api/projects.go
    - internal/api/projects_test.go
    - internal/api/routes.go

key-decisions:
  - "ParseRepoRef is pure (no I/O) and is the single canonicalization truth reused by both the PATCH link path and the origin-suggestion path; the canonical error copy 'Not a valid repository — use owner/name or a GitHub URL.' is the one hard-block message"
  - "ValidateRepo never hard-blocks on gh (D-11): gh absent OR any nonzero/parse failure → soft-save the syntactic owner/name with no error; only ParseRepoRef syntactic invalidity blocks (GHSET-03)"
  - "github_repo \"\" unlinks (stored NULL via 'github_repo = NULL' literal, no arg); description \"\" clears; both distinct from an omitted key"
  - "origin endpoint returns 200 {\"suggestion\":\"\"} (never an error) for a missing OR non-GitHub origin — detection is a convenience (D-08)"

patterns-established:
  - "Soft-dependency leaf package modeled on internal/quota/internal/tmux: call-time LookPath, exit-0-only success, degrade-don't-break on every gh failure mode"
  - "Partial-update PATCH via pointer fields + dynamically composed UPDATE — the template for future per-resource partial edits"

requirements-completed: [GHPRJ-01, GHPRJ-02, GHPRJ-03, GHSET-03]

# Metrics
duration: 9min
completed: 2026-06-13
---

# Phase 10 Plan 02: Project ↔ GitHub Link Backend Summary

**The `internal/github` leaf canonicalizes any owner/name/URL/ssh repo ref and soft-validates it via `gh` (degrade-don't-break), the `PATCH /api/projects/{id}` endpoint becomes a partial update for description + github_repo, and a dedicated `GET .../github-origin` endpoint prefills the link from the repo's git origin — only a syntactically malformed ref ever 4xx-blocks.**

## Performance

- **Duration:** ~9 min
- **Started:** 2026-06-13T14:18:32Z
- **Completed:** 2026-06-13T14:27:52Z
- **Tasks:** 3 (all TDD)
- **Files modified:** 5 (2 created, 3 modified)

## Accomplishments
- New `internal/github` leaf package (stdlib only): `ParseRepoRef` canonicalizes `owner/name`, `https://github.com/owner/name(.git)`, `http://...`, and `git@github.com:owner/name(.git)` all to `owner/name` (D-09); `ValidateRepo` soft-validates via `gh repo view --json nameWithOwner`; `Available` wraps `exec.LookPath("gh")`. Syntactic invalidity is the only hard error.
- `Project` shape grown by `description` (NOT NULL, "") and `github_repo` (`*string` → JSON `null` when unlinked); `projectColumns` and `scanProject` carry both (github_repo scanned through `sql.NullString`).
- `PATCH /api/projects/{id}` rewritten from rename-only into a partial update: pointer fields mean omitted ≠ clear; description set/clear with a 280-char cap; github_repo canonicalized + soft-validated, `""` unlinks to NULL; empty body 400s `nothing to update`.
- `GET /api/projects/{id}/github-origin`: shells `git -C <repo> remote get-url origin` (arg array) only on dialog open, returns the canonicalized owner/name or an empty suggestion (never an error) for a missing/non-GitHub origin; unknown id 404s.

## Task Commits

Each task was committed atomically (TDD: test → feat):

1. **Task 1 (RED): failing test for internal/github** - `8f792df` (test)
2. **Task 1 (GREEN): internal/github leaf package** - `c0196d7` (feat)
3. **Task 2 (RED): failing partial-PATCH test** - `31f6175` (test)
4. **Task 2 (GREEN): grow Project shape + partial PATCH** - `ec87761` (feat)
5. **Task 3 (RED): failing origin-suggestion test** - `88cf693` (test)
6. **Task 3 (GREEN): origin auto-detect endpoint** - `0206187` (feat)

**Plan metadata:** committed separately (docs: complete plan)

_No REFACTOR commits were needed — each GREEN implementation was clean (gofmt + go vet)._

## Files Created/Modified
- `internal/github/github.go` - Created. `ParseRepoRef`/`ValidateRepo`/`Available`; stdlib + os/exec only; arg-array exec; canonical hard-error copy; gh degrade paths.
- `internal/github/github_test.go` - Created. Table-driven `TestParseRepoRef` (all accept/reject forms), `TestParseRepoRefCanonicalError` (pins the dialog copy), `TestAvailable` (no-panic LookPath wrapper).
- `internal/api/projects.go` - Modified. Project struct + projectColumns + scanProject grown by 2 fields; `update` turned into a partial PATCH; new `githubOrigin` handler; added `kangent/internal/github` import.
- `internal/api/projects_test.go` - Modified. `TestUpdateProjectPartial` (set/clear/canonicalize/unlink/invalid-400-row-unchanged/empty-400/404) + `assertDBProject`; `TestGithubOriginSuggestion` (ssh/https canonicalize, no-origin → "", non-GitHub → "", bogus id → 404).
- `internal/api/routes.go` - Modified. Registered `GET /api/projects/{id}/github-origin` next to the other project routes.

## Decisions Made
- Followed the plan exactly. ParseRepoRef stays pure and is the single canonicalization truth shared by the PATCH link path and the origin endpoint, so the accepted-form set and the canonical reject copy can never diverge.
- `github_repo = NULL` is emitted as a literal SQL fragment (no bound arg) for the unlink path, keeping the dynamic UPDATE's arg list aligned with its `?` placeholders.
- The origin endpoint swallows ParseRepoRef errors into an empty suggestion rather than surfacing them — origin detection is a convenience prefill (D-08), not a validation gate; the user still types/confirms the link, which goes through the PATCH soft-validation.

## Deviations from Plan

None - plan executed exactly as written.

The plan's `<action>` for Task 2 included a self-correcting note about the "nothing to update" requirement (the plan text first mused `name is required` then settled on `nothing to update`); the implemented behavior matches the plan's final stated rule: at least one of name/description/github_repo must be present, else 400 `nothing to update`. This is the plan as written, not a deviation.

## Issues Encountered
None. Each RED step failed for the expected reason (undefined symbols for Task 1; missing fields / rename-only handler for Task 2; 404 missing route for Task 3) and each GREEN step passed on the first run. `gh` 2.82.0 is present on this host, but the github_repo and origin tests deliberately use syntactic refs (`https://github.com/cli/cli.git` → `cli/cli`, `git@github.com:owner/name.git` → `owner/name`) that exercise ParseRepoRef canonicalization without depending on gh network verification, so the suite is deterministic on any runner (GHSET-03 degrade path).

## User Setup Required
None - no external service configuration required. `gh` is a soft dependency: linking works whether or not it is installed or authenticated.

## Next Phase Readiness
- Plan 03 (frontend Project settings dialog) can consume the full server contract: the `PATCH /api/projects/{id}` partial-update shape (name/description/github_repo) and `GET /api/projects/{id}/github-origin` for on-open prefill, plus the canonical invalid-ref error string to mirror in the UI.
- `internal/github.ParseRepoRef`/`Available` are reusable by Phase 11+ (PR review column) for repo canonicalization and gh-presence gating.
- The full Go suite (`go test ./...`) and `go build ./...` are green. No board-leak surface touched here (this plan only reads/writes `projects`, never `tasks`); the Phase 12 `source='manual'` SELECT audit reminder still stands for later.

## Self-Check: PASSED

All created/modified files verified present on disk; all six task commits verified in git history (see below).

---
*Phase: 10-github-foundations*
*Completed: 2026-06-13*
