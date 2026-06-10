---
phase: 03-worktree-isolation-bash-tabs
plan: 06
subsystem: testing
tags: [go, httptest, integration-test, git-worktree, pty, rest]

# Dependency graph
requires:
  - phase: 03-worktree-isolation-bash-tabs (plan 03-01)
    provides: internal/worktree service — slug, base resolution, branch-keeping remove
  - phase: 03-worktree-isolation-bash-tabs (plan 03-02)
    provides: session.Manager task scoping — SpawnOpts{Cwd, TaskID}, per-task Bash N labels
  - phase: 03-worktree-isolation-bash-tabs (plan 03-03)
    provides: REST surface — task-create provisioning, GET/DELETE/POST /api/tasks/{id}/worktree with exact 409 bodies, task-scoped /api/sessions
  - phase: 03-worktree-isolation-bash-tabs (plan 03-04)
    provides: worktree meta line + bash tabs UI (human-verified here)
  - phase: 03-worktree-isolation-bash-tabs (plan 03-05)
    provides: cleanup dialog variants + Done/menu triggers (human-verified here)
provides:
  - internal/api/integration_test.go — one six-stage lifecycle test over the production route wiring (Routes + SessionRoutes + WorktreeRoutes on one mux)
  - Phase 3 verified — all four ROADMAP success criteria approved by human against the built binary
affects: [phase-04 agent sessions, phase-05 resume]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Integration harness mirrors cmd/kangent/main.go wiring order exactly (Routes, SessionRoutes, WorktreeRoutes) so production wiring drift breaks the test"
    - "stage() helper: t.Run + FailNow so dependent lifecycle subtests abort on the first failed stage instead of cascading"

key-files:
  created:
    - internal/api/integration_test.go
  modified: []

key-decisions:
  - "Test repo created on the OUTER t, not inside a subtest — a subtest's t.TempDir is removed when the subtest ends, which would delete the repo's .git/worktrees bookkeeping out from under later stages"
  - "cwd proven behaviorally with echo \"mark:$PWD\" — the echoed input only ever contains the literal $PWD, so PTY command echo can never satisfy the marker assertion"
  - "Lifecycle stages share state via outer-scope vars and abort on first failure (stage helper wraps t.Run + FailNow) — later stages depend on earlier server state"

patterns-established:
  - "Phase-goal verification plan shape: one composing integration test + full build gates + human checkpoint against the single built binary"

requirements-completed: [GIT-01, GIT-02, GIT-03, TERM-04]

# Metrics
duration: 27min
completed: 2026-06-10
---

# Phase 3 Plan 06: End-to-End Verification Summary

**Six-stage lifecycle integration test over the real REST surface (provision → task-scoped session cwd proof → dirty/session state → composed 409 gates → branch-keeping forced cleanup → kept-branch reuse), full build gates green, and human approval of all four Phase 3 success criteria against the built binary**

## Performance

- **Duration:** 27 min (including human-verify checkpoint wait)
- **Started:** 2026-06-10T15:29:17Z
- **Completed:** 2026-06-10T15:56:29Z
- **Tasks:** 3 (1 code, 1 verification-only, 1 human checkpoint)
- **Files modified:** 1

## Accomplishments

- `TestIntegrationWorktreeLifecycle` (263 lines) proves the slices compose: one mux wired exactly like production (Routes + SessionRoutes + WorktreeRoutes over one DB, one worktree.Service, one session.Manager), a real temp git repo with a commit on main
  - Stage 1 (GIT-01): POST task → 201 with `worktree_path` set, branch `task/fix-login-{id}` (collision-safe slug+id), dir on disk OUTSIDE the repo tree (filepath.Rel escape check), branch visible in `git branch --list`
  - Stage 2 (TERM-04): POST /api/sessions {task_id} → 201 label "Bash 1"; `echo "mark:$PWD"` marker proves the shell's cwd IS the worktree; filtered list shows exactly that session running
  - Stage 3: untracked file → GET /worktree reports dirty_files ≥ 1, running_sessions == 1
  - Stage 4 (GIT-03): bare DELETE → 409 "sessions running"; DELETE {stop_sessions} on the dirty tree → 409 "worktree has uncommitted changes" — gates compose, dirty re-checked server-side before anything is stopped; worktree intact after both refusals
  - Stage 5 (GIT-02/D-34): DELETE {stop_sessions, force} → 204; session exited (stop-before-remove ordering), dir gone, branch STILL listed, task lands in the D-26 absent state (all three fields null)
  - Stage 6: POST /worktree → 200 with `worktree_error` null and the SAME branch name — kept-branch reuse proven over REST
- Full build gates green: `go build ./...`, `go vet ./...`, `go test ./... -count=1`, `tsc -b --noEmit`, `npm run build`, `make build` (embedded single binary with the real dist)
- Human verification APPROVED for all four ROADMAP Phase 3 success criteria: lazy worktree creation (D-26 Create worktree), bash tab reattach after browser close, disabled `+` with "Bash sessions need a worktree" tooltip, Done-transition dialog with Keep worktree decline, and the dirty-tree type-to-confirm gate. Orchestrator pre-verified in headless Chrome + on disk: worktree/branch creation at correct paths, bash tab cwd ($PWD proof), sessions-running dialog variant with exact UI-SPEC copy, confirm → session stopped (exit 137) + worktree removed + branch kept

## Task Commits

1. **Task 1: End-to-end worktree lifecycle integration test** - `f468b64` (test) — single commit; the implementation under test already existed from plans 03-01..03-05, so the RED/GREEN split did not apply
2. **Task 2: Full build gates** - verification-only, no commit (all four gate commands exited 0)
3. **Task 3: Human verification checkpoint** - APPROVED (no commit)

## Files Created/Modified

- `internal/api/integration_test.go` — six-stage lifecycle test, production-shaped harness, behavioral cwd proof, gate-composition and branch-survival assertions

## Decisions Made

- Test repo created on the outer `t` rather than inside stage 1's subtest — subtest TempDir cleanup would otherwise delete the repo (and its `.git/worktrees` bookkeeping) before stages 4–6 run
- `stage()` helper (t.Run + FailNow) aborts the lifecycle on the first failed stage since later stages depend on earlier server state — no cascading noise
- cwd asserted behaviorally via `mark:$PWD` expansion so PTY command echo can never satisfy the output assertion (same trick family as Phase 2's `mar''ker`)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- `web/dist/index.html` again carries local real-build output after `make build` (pre-existing, logged in 03-03's deferred-items) — left uncommitted per the Phase 01 "real build output never committed" decision

## Authentication Gates

None.

## User Setup Required

None - no external service configuration required.

## Known Stubs

None — this plan added test code only; the surfaces it verifies are fully wired.

## Next Phase Readiness

- Phase 3 complete and human-verified: every task gets an isolated worktree + branch, bash tabs run inside the worktree with attach/detach/reattach, and Done offers gated cleanup that always keeps the branch
- GIT-01, GIT-02, GIT-03, TERM-04 demonstrably satisfied over both the REST surface (automated) and the browser UI (human)
- Ready for Phase 4 (agent sessions): worktree cwds and task-scoped session plumbing are the substrate the Claude Code tab will spawn into
- Carried concern for Phase 4 planning: alt-screen replay strategy and Claude Code hook/`--resume` semantics (already flagged in STATE.md blockers)

---
*Phase: 03-worktree-isolation-bash-tabs*
*Completed: 2026-06-10*

## Self-Check: PASSED

internal/api/integration_test.go (263 lines) and this SUMMARY exist on disk; task commit f468b64 present in git log; `go test ./internal/api/ -run TestIntegration -count=1` and all build gates passed during Task 2; human approval recorded for Task 3.
