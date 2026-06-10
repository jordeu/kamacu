---
phase: 03-worktree-isolation-bash-tabs
plan: 01
subsystem: git
tags: [git, worktree, os-exec, tdd, go]

# Dependency graph
requires:
  - phase: 01-foundation-projects-board
    provides: project repo_path validation pattern (arg-array git exec), table-driven test conventions
provides:
  - internal/worktree package: Slug, Service{Root}, NewService, PathFor, ResolveBase, Create, DirtyCount, Remove, EnsureSubmodules
  - Collision-safe worktree+branch creation with leak-safe branch reuse (GIT-01)
  - Dirty detection (-uall record count) and branch-preserving removal with submodule/idempotency fallbacks (GIT-03 mechanics)
affects: [03-03 worktree API endpoints, 03-06 cleanup flow, 04 agent sessions in worktrees]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "gitRun: exec.CommandContext arg arrays, exit-0-only success, stderr trimmed + fatal-prefix-stripped for verbatim UI display"
    - "Real-git tests in t.TempDir() with TestMain-level config isolation (controlled temp GIT_CONFIG_GLOBAL)"
    - "git check-ref-format --branch as slug-validity oracle"

key-files:
  created:
    - internal/worktree/worktree.go
    - internal/worktree/worktree_test.go
  modified: []

key-decisions:
  - "ResolveBase verifies the current-branch ref resolves before returning it — symbolic-ref --short HEAD succeeds even on unborn HEAD, so the RESEARCH sketch alone would have returned an invalid base instead of the D-25 error"
  - "Test binary's isolated global gitconfig is a controlled temp file with protocol.file.allow=always (not /dev/null) — submodule clone subprocesses read global/system config, not the superproject's repo-local config"
  - "Slug capped by bytes (charset is ASCII by construction) with post-cap trailing-dash re-trim"

patterns-established:
  - "Worktree mutations serialized behind one coarse Service mutex (single-user app)"
  - "Remove never touches branches (D-34) and never double-forces (manual locks must surface)"

requirements-completed: [GIT-01, GIT-03]

# Metrics
duration: 13min
completed: 2026-06-10
---

# Phase 3 Plan 01: Worktree Service Summary

**`internal/worktree` git-CLI service: ref-safe slugs, no-network default-branch resolution, leak-safe worktree+branch creation, -uall dirty counts, and branch-preserving removal — all tested against real git 2.43 repos in t.TempDir()**

## Performance

- **Duration:** 13 min
- **Started:** 2026-06-10T14:18:05Z
- **Completed:** 2026-06-10T14:31:18Z
- **Tasks:** 2 (both TDD)
- **Files modified:** 2

## Accomplishments

- Full `Service` surface implemented to the exact RESEARCH-verified git 2.43 semantics: `Slug`, `PathFor`, `ResolveBase` (4-leg local-only chain), `Create` (leaf pre-check + branch reuse), `DirtyCount` (porcelain=v2 -uall), `Remove` (submodule fallback, idempotency, trailing prune), `EnsureSubmodules`
- 18 tests against real throwaway git repos — zero mocks, zero new dependencies; every RESEARCH pitfall edge (branch leak, untracked-only refusal, submodule refusal, missing-dir idempotency, no-origin/HEAD repos, unborn HEAD) has explicit coverage
- Every `Remove` test asserts the branch survives (D-34 invariant); `git check-ref-format --branch` used as the slug oracle

## Task Commits

Each TDD cycle was committed atomically:

1. **Task 1 RED: Slug/ResolveBase/Create tests** - `d142f61` (test)
2. **Task 1 GREEN: gitRun, Slug, Service, ResolveBase, Create** - `61d3032` (feat)
3. **Task 1 follow-up: comment reword for acceptance grep** - `1d75a2a` (docs)
4. **Task 2 RED: DirtyCount/Remove/EnsureSubmodules tests** - `6fc510b` (test)
5. **Task 2 GREEN: DirtyCount, Remove, EnsureSubmodules** - `77a8e77` (feat)

## Files Created/Modified

- `internal/worktree/worktree.go` (241 lines) - gitRun exec helper + the full service surface; doc comments encode the verified pitfalls (branch leak, exit-code inconsistency, no-EBUSY removal, double-force prohibition)
- `internal/worktree/worktree_test.go` (504 lines) - TestMain config isolation, real-repo helpers (makeRepoOn, cloneRepo, addSubmodule), 18 table/leg tests

## Decisions Made

- **Unborn-HEAD detection added to ResolveBase:** `symbolic-ref --short HEAD` exits 0 on a fresh `git init` with no commits, so the chain now verifies `refs/heads/<name>` resolves before returning the current branch; otherwise it falls through to `rev-parse HEAD`, whose failure is the D-25 error
- **Submodule test transport:** the isolated "global" gitconfig is a temp file containing `protocol.file.allow=always` rather than `/dev/null`, because the clone subprocess spawned by `submodule update --init` cannot see superproject repo-local config
- **PathFor uses byte-exact `filepath.Base(repoPath)`** per the interface contract; task IDs are globally unique so leaf dirs never collide across projects

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] ResolveBase returned an invalid base on unborn HEAD**
- **Found during:** Task 1 (GREEN — TestResolveBaseUnbornHead failed)
- **Issue:** The RESEARCH §Code Examples sketch returns `symbolic-ref --short HEAD` output directly, but that command succeeds on an unborn HEAD (prints the branch name with no commit behind it), so the plan's required error (D-25 failed state) never fired
- **Fix:** Verify `refs/heads/<name>` with `show-ref --verify --quiet` before returning; fall through to `rev-parse HEAD` whose failure carries git's own error
- **Files modified:** internal/worktree/worktree.go
- **Verification:** TestResolveBaseUnbornHead passes; all other legs unchanged
- **Committed in:** 61d3032 (Task 1 commit)

**2. [Rule 3 - Blocking] file-protocol submodule clones blocked under config isolation**
- **Found during:** Task 2 (GREEN — submodule tests failed with "transport 'file' not allowed")
- **Issue:** git ≥2.38 blocks file-protocol submodule clones by default; the repo-local `protocol.file.allow=always` workaround is invisible to the clone subprocess spawned by the package's `submodule update --init`
- **Fix:** TestMain points GIT_CONFIG_GLOBAL at a controlled temp config file containing `[protocol "file"] allow = always` (host isolation preserved; package code unchanged)
- **Files modified:** internal/worktree/worktree_test.go
- **Verification:** TestEnsureSubmodulesInitializes and TestRemoveCleanWithInitializedSubmodules pass
- **Committed in:** 77a8e77 (Task 2 commit)

**3. [Rule 1 - Bug] Doc comment tripped the no-network-command acceptance grep**
- **Found during:** Task 1 acceptance verification
- **Issue:** A ResolveBase comment named the forbidden `set-head` command, failing the literal `grep -c 'set-head\|remote show' = 0` criterion
- **Fix:** Reworded the comment to describe the forbidden commands without naming them
- **Files modified:** internal/worktree/worktree.go
- **Verification:** grep count 0; tests unchanged
- **Committed in:** 1d75a2a

---

**Total deviations:** 3 auto-fixed (2 bugs, 1 blocking test-infra)
**Impact on plan:** Deviation 1 is a real correctness fix the plan's behavior table demanded; 2 and 3 are test/verification plumbing. No scope creep — no file outside internal/worktree/ touched.

## Issues Encountered

- **Parallel-executor amend collision:** a `git commit --amend` intended for my Task 1 commit landed on the parallel 03-02 agent's commit that had slipped in between, folding my one-file edit into their commit and rewriting their hash. Repaired immediately via `git reset --soft` to their original commit (`91acbc6`, hash restored on the branch) plus a separate commit (`1d75a2a`) for my edit. Lesson recorded: never `--amend` while parallel executors share a branch.
- **Transient `go build ./...` failure in `internal/api`:** caused by the parallel 03-02 agent's in-flight `Manager.Spawn` signature change — out of this plan's scope; resolved when they committed their call-site update (`a8bf064`). Final verification ran green across the whole module.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Plans 03-03 (worktree REST endpoints) and 03-06 (cleanup flow) can now provision with `svc.ResolveBase` → `svc.Create(repo, "task/"+Slug(title)+"-"+id, svc.PathFor(...), base)` and tear down with `DirtyCount` + `Remove` — branch survives every path
- The running-sessions gate (D-32) is intentionally NOT in this package (Pitfall 4: git gives no EBUSY protection); the API layer must enforce it before calling Remove

---
*Phase: 03-worktree-isolation-bash-tabs*
*Completed: 2026-06-10*

## Self-Check: PASSED

- All created files exist on disk
- All 5 task commits present in git history
- go build ./... && go vet ./internal/worktree/ && go test ./internal/worktree/ green
