---
phase: 14-managed-checkout-foundations
plan: 02
subsystem: api
tags: [github, gh, clone, projects, reattach, atomicity, go, sqlite]

# Dependency graph
requires:
  - phase: 14-managed-checkout-foundations
    provides: "github.Clone (cloneRunner seam), migration 00008 managed column, Project.Managed wired through projectColumns/scanProject, projectHandlers struct"
  - phase: 10-github-foundations
    provides: "github.ParseRepoRef / ValidateRepo / Available, settings.ExpandHome, project github_repo column"
provides:
  - "repo-first POST /api/projects branch: ParseRepoRef→ValidateRepo (RPROJ-05) BEFORE clone; INSERT(managed=1, github_repo=canonical) only AFTER github.Clone exit 0 (atomic, D-01/CKOUT-04)"
  - "reattachManaged helper (CKOUT-05/D-10): reuse on origin match, refuse without clobbering on mismatch/non-git/no-origin"
  - "github.validateRunner + availableRunner package seams + exported SetCloneRunnerForTest/SetValidateRunnerForTest/SetAvailableForTest for cross-package deterministic create-by-repo tests"
affects: [14-03-pre-task-fetch, 14-04-gated-delete, 15-repo-first-creation-flow]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Cross-package exec seam exposure: exported SetXForTest setters returning a restore func, so internal/api tests can fake internal/github's gh/clone exec without a live gh or network"
    - "Clone-then-create atomicity: validate-before-clone, clone-before-insert; INSERT only after exit 0; os.RemoveAll(dest) belt-and-braces on failure → no orphan row, no partial dir"
    - "Reattach-not-clobber: an existing dest is reused on canonicalized origin match and refused (never removed/reset) on mismatch/non-git"

key-files:
  created: []
  modified:
    - "internal/api/projects.go"
    - "internal/api/projects_test.go"
    - "internal/github/github.go"
    - "internal/github/testhooks.go"

key-decisions:
  - "Phase 14 SETS github_repo=canonical at create-by-repo (research Open Question 1, recommended) so Phase 15's form relies on it — one fewer thing to wire later."
  - "Validation seam chosen per Task 2 NOTE option (b): a package-level validateRunner var + exported setter, plus an availableRunner seam, so create-by-repo tests are deterministic regardless of whether gh is installed on CI."
  - "Reattach/non-git/mismatch all surface as 409 Conflict (the dir is in the way), consistent with the folder dedup 409; mismatch and non-git NEVER os.Remove the dir (D-11)."

patterns-established:
  - "SetXForTest(restore func()): exported test hooks let a sibling package swap an unexported exec seam and defer-restore production behavior."
  - "Repo-first create ordering is the load-bearing atomicity contract — keep validate→(reattach|clone)→dedup→insert in that exact order."

requirements-completed: [CKOUT-01, RPROJ-05, CKOUT-05]

# Metrics
duration: 9 min
completed: 2026-06-14
---

# Phase 14 Plan 02: Repo-First Project Create + Reattach Summary

**A second POST /api/projects path that gh-validates an `owner/name` (RPROJ-05), `gh repo clone`s it into `~/.kangent/repos/<owner>/<name>`, and writes the row with managed=1 + github_repo only after the clone returns exit 0 (atomic, no orphan row/partial dir) — with origin-matched reattach on an existing dir and refuse-without-clobber on mismatch.**

## Performance

- **Duration:** ~9 min
- **Started:** 2026-06-14T17:14Z (approx)
- **Completed:** 2026-06-14T17:23:19Z
- **Tasks:** 2 (both TDD)
- **Files modified:** 4

## Accomplishments

- **Repo-first create branch (CKOUT-01/RPROJ-05/D-01).** `POST /api/projects` now decodes a `repo` field; when non-empty it runs `createByRepo`: `ParseRepoRef` (hard-reject syntactic garbage) → `ValidateRepo` (gh-verify; not-verified → 400 `msgRepoNotFound`, gh-absent → 400 `msgGHUnavailable`) → compute `dest = ExpandHome("~/.kangent/repos/")/owner/name` → reattach-or-clone → dedup pre-check → `INSERT (name, repo_path, github_repo, managed=1) RETURNING` → 201. The validation runs **before** any clone and the INSERT runs **only after** `github.Clone` returns exit 0, so a rejected/failed clone leaves **no row and no dir** (`os.RemoveAll(dest)` belt-and-braces on the clone-failure path).
- **Reattach-on-existing-dir (CKOUT-05/D-10/D-11).** `reattachManaged(ctx, dest, canonical)` confirms dest is a git repo (`rev-parse --git-dir`), reads `origin` (`remote get-url origin`), canonicalizes it via `github.ParseRepoRef` (handles ssh + https), and case-insensitively compares to the requested owner/name: match → reuse (skip clone, fall through to dedup + INSERT managed=1); mismatch / non-git / no-origin → a clear error surfaced as 409, and the directory is **never** removed or reset. An already-tracked matching dir 409s "this repository is already added".
- **Deterministic cross-package test seams.** Added `validateRunner` and `availableRunner` package vars in `internal/github` (mirroring the existing `cloneRunner`) plus exported `SetCloneRunnerForTest` / `SetValidateRunnerForTest` / `SetAvailableForTest` (each returns a restore func) so the `internal/api` create-by-repo tests inject canonical/verified/available outcomes without a live gh — making CI green regardless of host gh.
- **Folder path untouched (RPROJ-04 safety).** The original `repo_path` branch (validateRepoPath → dedup → `INSERT (name, repo_path)`) is byte-for-byte unchanged; `TestCreateRepoGHUnavailable` proves the folder create still works on the same endpoint when gh is absent.

## Task Commits

Each task committed atomically (TDD: test → feat):

1. **Task 1: Repo-first branch (validate→clone→insert, atomic)** (TDD)
   - `6ca48fa` (test/RED) — failing create-by-repo tests + the github seams
   - `8030fcc` (feat/GREEN) — `createByRepo` + `reattachManaged` + folder-branch split
2. **Task 2: Reattach helper coverage** (TDD)
   - `aa46a08` (test) — origin match / mismatch / non-git / already-added tests

_No REFACTOR commits — GREEN implementations were minimal and clean._

## Files Created/Modified

- `internal/api/projects.go` — `create` decodes `repo` and branches to `createByRepo`; new `createByRepo` (8-step atomic ordering), `reattachManaged` helper, `reposBase` const; added `context`/`settings` imports
- `internal/api/projects_test.go` — `newRepoTestServer` (isolated HOME → managed base), `countProjects`, `failCloneNeverCalled`, `preCreateManagedDir`; 8 new tests (validate-before-clone, gh-unavailable, success, clone atomicity, reattach match/mismatch/non-git/already-added)
- `internal/github/github.go` — `validateRunner` + `availableRunner` package seams; `ValidateRepo`/`Available` now route through them; extracted `ghValidate`/`ghLookPath`
- `internal/github/testhooks.go` (new) — exported `SetCloneRunnerForTest`/`SetValidateRunnerForTest`/`SetAvailableForTest` for sibling-package tests

## Decisions Made

- **Phase 14 sets `github_repo = canonical`** at create-by-repo INSERT time (research Open Question 1, recommended): the primitive already holds the canonical ref from `ValidateRepo`, and Phase 15's form relies on it — one fewer wiring step downstream.
- **Validation seam = package var + exported setter (Task 2 NOTE option b).** Rather than gating tests behind a real-gh availability check (flaky CI), I added `validateRunner`/`availableRunner` seams mirroring `cloneRunner`, exposed via `SetXForTest`. This keeps the create-by-repo tests deterministic and host-independent — the clone atomicity test was mandatory to seam regardless, and the verified/available paths reuse the same mechanism.
- **409 for reattach mismatch/non-git/no-origin.** The dest dir is "in the way", consistent with the folder dedup 409; the alternative (500) would mislead. The mismatch/non-git branches never touch the dir (D-11).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] reattachManaged implemented in Task 1 (GREEN) rather than purely in Task 2**
- **Found during:** Task 1 (GREEN)
- **Issue:** `createByRepo` references `reattachManaged` on its dest-exists path, so the function must exist for Task 1's branch to compile. A pure RED-first for Task 2 (tests before any helper code) would have left Task 1 non-compiling.
- **Fix:** Added the full `reattachManaged` implementation (its real D-10 logic) in the Task 1 GREEN commit (`8030fcc`); Task 2 then added the dedicated reattach **tests** (`aa46a08`) that rigorously prove match/mismatch/non-git/already-added behavior. The helper is a structural dependency of Task 1's branch, so this ordering is correct.
- **Files modified:** internal/api/projects.go (Task 1), internal/api/projects_test.go (Task 2)
- **Verification:** `TestReattachOriginMatch/Mismatch/NonGitDir/AlreadyAdded` all pass; clone seam asserts not-called on every reattach path
- **Committed in:** `8030fcc` (helper), `aa46a08` (tests)

---

**Total deviations:** 1 (TDD sequencing for a shared helper — Rule 3 blocking).
**Impact on plan:** None on behavior or scope. The atomicity, validate-before-clone, and reattach contracts are all proven by test; the only change is which commit the helper body landed in.

## Issues Encountered

None. Build, vet, and the full `./internal/...` suite are green.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- The repo-first provisioning primitive is in place for **14-03** (pre-task fetch on managed checkouts — it reads `Project.Managed` and the `~/.kangent/repos/` clone) and **14-04** (gated managed-clone delete).
- **15** (repo-first Add-project UI) can drive `POST /api/projects {"repo":"owner/name"}` directly; `github_repo` is already persisted by this primitive for the form's link/name auto-fill.
- The `SetXForTest` seams in `internal/github` are available for any future API-layer test needing a faked gh.
- `go build ./...`, `go vet`, and `go test ./internal/...` are all green.

---
*Phase: 14-managed-checkout-foundations*
*Completed: 2026-06-14*

## Self-Check: PASSED

- All modified/created files exist on disk (projects.go, projects_test.go, github.go, testhooks.go, 14-02-SUMMARY.md).
- All task commits present in git history (6ca48fa test/RED, 8030fcc feat/GREEN, aa46a08 test).
- `go build ./...`, `go vet ./internal/api/... ./internal/github/...`, and `go test ./internal/...` all green.
