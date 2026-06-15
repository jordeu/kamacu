---
phase: 15-repo-first-creation-flow
plan: 01
subsystem: api
tags: [github, gh-cli, projects, sqlite, react-query, typescript, tdd]

# Dependency graph
requires:
  - phase: 14-managed-checkout-foundations
    provides: "createByRepo (gh-validate → gh repo clone → atomic INSERT managed=1 + github_repo) and the internal/github gh-leaf seams (validateRunner/availableRunner + SetXForTest)"
provides:
  - "github.RepoDescription(ctx, canonical) string — best-effort, error-free read of a repo's GitHub description"
  - "descriptionRunner seam + SetDescriptionRunnerForTest for deterministic cross-package tests"
  - "createByRepo persists the auto-captured description into projects.description at create (best-effort, never blocks)"
  - "useCreateProject body widened to { name?; repo_path?; repo? } for the repo-first POST"
  - "Project wire type carries managed: boolean"
affects: [15-02 add-project-dialog, project-settings-description-edit]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Degrade-don't-break best-effort gh read with an error-free public signature (RepoDescription returns a bare string; all failures map to \"\")"
    - "Dedicated read separate from the shared ValidateRepo rather than widening a shared signature"

key-files:
  created: []
  modified:
    - internal/github/github.go
    - internal/github/testhooks.go
    - internal/github/github_test.go
    - internal/api/projects.go
    - internal/api/projects_test.go
    - web/src/api/mutations.ts
    - web/src/api/types.ts

key-decisions:
  - "RepoDescription is a SEPARATE best-effort read (mirroring ghValidate/validateRunner), NOT a widening of ValidateRepo — ValidateRepo is shared with the PATCH update handler and its signature stays (ctx, ref) (canonical, verified, err)"
  - "RepoDescription's public signature is a bare string (no error): the caller (createByRepo) must never have to handle a description error, and a missing/empty/failed read persists \"\" and never blocks create"
  - "Description is captured server-side at create and persisted into projects.description — it is NOT a field in the Add dialog (D-05); it surfaces editable in Project settings later"

patterns-established:
  - "Best-effort gh leaf with error-free public signature: RepoDescription(ctx, canonical) string maps gh-absent / empty / nonzero-exit / parse-fail all to \"\""
  - "Cross-package test seam: SetDescriptionRunnerForTest returns a restore func, mirroring SetValidateRunnerForTest"

requirements-completed: [RPROJ-02]

# Metrics
duration: 5 min
completed: 2026-06-15
---

# Phase 15 Plan 01: Repo-First Description Capture Summary

**Repo-first create now auto-captures the GitHub repo's description via a best-effort `gh repo view --json description` read and persists it into `projects.description` (degrade-don't-break, never blocks), plus the frontend `useCreateProject` body and `Project` wire type gain the repo-first `{ repo }` shape and the `managed` marker.**

## Performance

- **Duration:** 5 min
- **Started:** 2026-06-15T05:02:19Z
- **Completed:** 2026-06-15T05:07:43Z
- **Tasks:** 2 (both TDD)
- **Files modified:** 7

## Accomplishments

- `github.RepoDescription(ctx, canonical) string` — a best-effort, error-free read mirroring `ghValidate`/`validateRunner`: gh-absent / empty-description / nonzero-exit / parse-failure all yield `""`, so the caller never sees an error and create is never blocked.
- `createByRepo` captures the description right before its INSERT and persists it (the repo-first INSERT now includes the `description` column); the folder-create INSERT and the PATCH update handler are untouched (`ValidateRepo` signature unchanged).
- Frontend `useCreateProject` body widened to `{ name?; repo_path?; repo? }` (repo_path now optional) so the repo-first path can POST `{ repo }` with a correct type; `Project` gains `managed: boolean` (the backend already serializes `json:"managed"`).
- `go test ./internal/github/ ./internal/api/`, `go build ./...`, `go vet ./...`, and frontend `tsc --noEmit` all green.

## Task Commits

Each task ran the full RED → GREEN TDD cycle (no refactor needed — minimal code mirrored existing patterns):

1. **Task 1: github.RepoDescription best-effort read + test seam**
   - `0c50351` (test) — RED: failing tests for the four degrade-don't-break cases + fake-gh exec path
   - `fb10c6c` (feat) — GREEN: `RepoDescription` + `descriptionRunner` + `ghDescription` + `SetDescriptionRunnerForTest`
2. **Task 2: Persist captured description in createByRepo + widen frontend mutation/type**
   - `5a575b0` (test) — RED: `TestCreateRepoCapturesDescription` (fails) + `TestCreateRepoEmptyDescriptionStill201` (degrade-don't-break)
   - `7558fe4` (feat) — GREEN: createByRepo capture+INSERT, widened `useCreateProject` body, `Project.managed`

**Plan metadata:** _(docs commit below)_

## Files Created/Modified

- `internal/github/github.go` — added `descriptionRunner` seam, `RepoDescription(ctx, canonical) string`, and the production `ghDescription` runner (`gh repo view <canonical> --json description`, arg array, never `sh -c`). `ValidateRepo`/`ghValidate` untouched.
- `internal/github/testhooks.go` — added `SetDescriptionRunnerForTest` mirroring `SetValidateRunnerForTest`.
- `internal/github/github_test.go` — added `TestRepoDescription` (4 cases via the seams) + `TestRepoDescriptionFakeGH`/`...Empty` (production runner through a restricted-PATH fake `gh`).
- `internal/api/projects.go` — `createByRepo` step 7 now reads `desc := github.RepoDescription(r.Context(), canonical)` and adds `description` to the repo-first INSERT column list/values.
- `internal/api/projects_test.go` — added `repoSuccessSeams` helper, `TestCreateRepoCapturesDescription`, `TestCreateRepoEmptyDescriptionStill201`.
- `web/src/api/mutations.ts` — `useCreateProject` body widened to `{ name?; repo_path?; repo? }`.
- `web/src/api/types.ts` — `Project` gains `managed: boolean`.

## Decisions Made

- **Dedicated read, not a widened `ValidateRepo`:** `ValidateRepo` is shared with the PATCH update handler (`projects.go`), and the plan's acceptance asserts its signature `(ctx, ref) (canonical, verified, err)` stays unchanged. `RepoDescription` is a separate best-effort leaf that rides the same `gh repo view` surface independently.
- **Error-free public signature:** `RepoDescription` returns a bare `string`. Every failure mode (gh absent, empty description, gh nonzero exit, JSON parse failure) maps to `""` inside the package, so `createByRepo` never has to handle or propagate a description error — honoring D-05's "never blocks create."
- **Description is server-side-only at create:** persisted into `projects.description`, surfaced (editable) later in Project settings; intentionally NOT a field in the Add dialog (D-05). Plan 02's dialog drives the existing `createByRepo` path, which now also persists the description.

## Deviations from Plan

None - plan executed exactly as written.

**Total deviations:** 0
**Impact on plan:** None — both TDD tasks followed the plan's `<action>` and `<acceptance_criteria>` verbatim.

## Issues Encountered

None.

Note on one acceptance criterion: the plan's `grep -c "sh -c\|exec.Command(\"sh\"" internal/github/github.go` "returns 0" check matches 3 lines — all of them comments asserting "never `sh -c`" (the same situation the Phase-14 verifier explicitly accepted). There is no actual `sh -c` invocation or `exec.Command("sh"` in production code; `ghDescription` uses an `exec.CommandContext(ctx, "gh", "repo", "view", ...)` arg array. Intent (no shell interpolation of repo input) is fully satisfied.

## User Setup Required

None - no external service configuration required. (`gh` is already present + authenticated on this host; the description read is best-effort and degrades silently if not.)

## Next Phase Readiness

- RPROJ-02's backend half is complete: a repo-first create auto-captures and persists the GitHub description, best-effort, never blocking create.
- The frontend now has the typed `{ repo }` body (`useCreateProject`) and reads `managed` off `Project` — Plan 02 (the Add-project repo-first dialog) can build on these without further API or type changes.
- Zero regression to the folder-create path and the PATCH update handler (both INSERT-unchanged / `ValidateRepo`-unchanged).
- Ready for 15-02.

## Self-Check: PASSED

All 7 modified files + the SUMMARY exist on disk; all 4 task commits (`0c50351`, `fb10c6c`, `5a575b0`, `7558fe4`) are present in the git log.

---
*Phase: 15-repo-first-creation-flow*
*Completed: 2026-06-15*
