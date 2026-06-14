---
phase: 13-pr-worktree-auto-cleanup
verified: 2026-06-14T00:00:00Z
status: passed
score: 18/18 must-haves verified
gaps: []
---

# Phase 13: PR Worktree Auto-Cleanup Verification Report

**Phase Goal:** When a PR merges or closes, its clean review worktree disappears on its own — while a dirty or busy one is never silently removed and the branch always survives.
**Verified:** 2026-06-14
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| #  | Truth                                                                                                    | Status     | Evidence |
| -- | ------------------------------------------------------------------------------------------------------- | ---------- | -------- |
| 1  | GET `.../pull-requests/{n}` detail carries PR state (OPEN/CLOSED/MERGED) for the banner                  | ✓ VERIFIED | `prWire.State `json:"state"`` (pullrequests.go:286); `State: d.State` (303); `PRDetail.State` (github.go:122); ViewPR `--json ...,state` (141) |
| 2  | A cheap `github.Service.PRState(repo,n)` returns the UPPERCASE state for per-tick use                    | ✓ VERIFIED | `func (s *Service) PRState` (state.go:23), `--json state`, returns `resp.State`; degrades to `("",err)` on gh-absent/exit |
| 3  | HTTP DELETE worktree handler keeps its 409/500/204 responses unchanged                                   | ✓ VERIFIED | worktrees.go:245-259 delegates then maps `reason`→409, err→500, removed→204; 4 existing DELETE tests pass unchanged |
| 4  | A single `CleanupWorktreeGated` helper performs gate+stop+kill+remove+null for handler and reaper        | ✓ VERIFIED | cleanup.go:36; called from worktrees.go:245 and reaper.go:319 — one path, two callers |
| 5  | `worktree.Service` exposes UnpushedCount + StashCount gate primitives                                    | ✓ VERIFIED | worktree.go:288 (rev-list `base..HEAD`), :307 (stash list); hermetic tests pass |
| 6  | Each reaper tick runs a second pass over `source='github_pr'` tasks with a worktree, reading PR state    | ✓ VERIFIED | reconcilePRsOnce (reaper.go:223); SELECT WHERE `source='github_pr' AND worktree_path IS NOT NULL`; called in Run at start + tick (122-132) |
| 7  | A MERGED/CLOSED PR with a pristine, idle worktree is auto-removed (clean case)                           | ✓ VERIFIED | TestReconcileMergedCleanRemovesAndDeletes + TestReconcileClosedRemovesAndDeletes PASS (os.Stat NotExist + row count 0) |
| 8  | A MERGED/CLOSED dirty/unpushed/stash/session worktree is LEFT untouched (worktree + row remain)          | ✓ VERIFIED | TestReconcileMerged{Dirty,Unpushed,Stash}Skips PASS; 4-gate logic reaper.go:271-322 |
| 9  | An OPEN PR (or any gh error) is never touched                                                            | ✓ VERIFIED | reaper.go:267-268 (`state != MERGED && != CLOSED → continue`); err→warn+continue (263-266); TestReconcileOpenUntouched PASS |
| 10 | A `source='manual'` task is NEVER touched by the PR pass                                                 | ✓ VERIFIED | WHERE `source='github_pr'` excludes manual; TestReconcileManualNeverTouched PASS (zero PRState calls) |
| 11 | After a successful auto-remove the row is DELETED (tmux_sessions first, then tasks) — only reaper deletes | ✓ VERIFIED | reaper.go:335 then :338, guarded by `if !removed { continue }` (327); helper itself only nulls (cleanup.go:87) |
| 12 | The reaper never forces past a gate and never stops a live session (force=false, stopSessions=false)     | ✓ VERIFIED | reaper.go:322 `false /*stopSessions*/, false /*force*/` |
| 13 | PR review view shows a ⋯ menu with a single "Clean up worktree" item (no "Delete task")                  | ✓ VERIFIED | TaskPage.tsx:557-574 — `isPR && task.worktree_path` menu, one DropdownMenuItem, no Delete |
| 14 | Selecting it opens the existing CleanupWorktreeDialog (same gated flow)                                   | ✓ VERIFIED | `onSelect={() => setCleanupOpen(true)}` (569); dialog already rendered, reused unchanged |
| 15 | When state != OPEN, an inline merged/closed banner with a manual Clean-up prompt renders                 | ✓ VERIFIED | TaskPage.tsx:620-639 — `prDetail && prDetail.state !== "OPEN"`, "This PR was merged/closed.", Clean-up button |
| 16 | The banner lives inside the shrink-0 header block, never modal/blocking                                  | ✓ VERIFIED | banner is in the `<>` fragment of the `isPR ?` arm, inside the `w-full shrink-0` div (462→648); TaskTabs at 650 outside; `role="status"`, no modal |
| 17 | The git branch ref is never deleted (no `git branch -D` anywhere)                                        | ✓ VERIFIED | No `git branch -d/-D/--delete` in internal/ or cmd/; Remove docstring "It NEVER deletes branches" (worktree.go:319-336) |
| 18 | Auto-cleanup observed end-to-end via the reaper; dirty/busy left in place, branch kept (human-verify)    | ✓ VERIFIED | Human-verify APPROVED 2026-06-14 (13-03 Task 3): GHCLN-01/02/03 confirmed in running app |

**Score:** 18/18 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/github/state.go` | minimal PRState reader | ✓ VERIFIED | `func (s *Service) PRState` returns UPPERCASE state, degrades on gh failure |
| `internal/github/github.go` | State on PRDetail; ViewPR requests state | ✓ VERIFIED | `State string `json:"state"`` (122); json list ends `...,commits,state` (141) |
| `internal/api/cleanup.go` | exported CleanupWorktreeGated | ✓ VERIFIED | byte-equivalent gate+stop+kill+remove+null; sessions/dirty gates only |
| `internal/api/worktrees.go` | remove() delegates | ✓ VERIFIED | delegates at :245, maps reason→409/err→500/removed→204 |
| `internal/worktree/worktree.go` | UnpushedCount + StashCount | ✓ VERIFIED | both present (288, 307); hermetic unit tests pass |
| `internal/reaper/reaper.go` | PRStateGetter + reconcilePRsOnce + deps + NewWithPR | ✓ VERIFIED | all present; 4-gate logic; FK-ordered delete; Run calls both passes |
| `internal/reaper/pr_reconcile_test.go` | spy-driven matrix | ✓ VERIFIED | 7 tests covering clean/dirty/unpushed/stash/open/manual/closed — all PASS |
| `cmd/kangent/main.go` | NewWithPR wiring with ghSvc | ✓ VERIFIED | `reaper.NewWithPR(db, mgr, wtSvc, tmuxClient, ghSvc)` (182) |
| `internal/api/pullrequests.go` | state on detail wire | ✓ VERIFIED | prWire.State + prWireFrom `State: d.State` |
| `web/src/pages/TaskPage.tsx` | PR ⋯ menu + banner | ✓ VERIFIED | menu (557-574), banner (620-639), both inside shrink-0 header |
| `web/src/api/pullRequests.ts` | PRDetailWire.state | ✓ VERIFIED | `state: "OPEN" | "CLOSED" | "MERGED"` (73) |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| worktrees.go remove() | CleanupWorktreeGated | handler delegates | ✓ WIRED | call at :245, helper at cleanup.go:36 |
| pullrequests.go prWireFrom | PRDetail.State | state on wire | ✓ WIRED | `State: d.State` (303) |
| reconcilePRsOnce | github.Service.PRState | PRStateGetter seam | ✓ WIRED | `r.pr.PRState(ctx, t.repo, int(t.n))` (262); `*github.Service` satisfies it via main.go ghSvc |
| reconcilePRsOnce | CleanupWorktreeGated | shared helper, force=false stopSessions=false | ✓ WIRED | reaper.go:319-322 |
| reconcilePRsOnce | DELETE FROM tasks | tmux_sessions-before-tasks FK order | ✓ WIRED | :335 then :338, guarded by removed=true |
| TaskPage PR menu | setCleanupOpen(true) | opens existing dialog | ✓ WIRED | onSelect (569) and banner button (633) |
| TaskPage banner | prDetail.state | render when != OPEN | ✓ WIRED | `prDetail.state !== "OPEN"` (620) |
| main.go | reaper.NewWithPR | ghSvc as PRStateGetter | ✓ WIRED | (182) reuses existing ghSvc — no new construction |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| reconcilePRsOnce delete | task row removal | gated on `removed==true` from real `CleanupWorktreeGated` → `wt.Remove` | Yes — only on a verified git worktree removal | ✓ FLOWING |
| TaskPage banner | `prDetail.state` | `usePullRequestDetail` → GET wire → `prWireFrom(d.State)` → `gh pr view --json state` | Yes — live gh-sourced state | ✓ FLOWING |
| reaper conservative gate | dirty/unpushed/stash counts | real `DirtyCount`/`UnpushedCount` (rev-list after re-fetch)/`StashCount` on FRESH git state | Yes — actual git probes | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Merged/closed clean → removed + row deleted | `go test ./internal/reaper -run Reconcile -v` | TestReconcileMerged/ClosedClean PASS | ✓ PASS |
| Dirty/unpushed/stash merged → skipped | same | 3 Skip tests PASS | ✓ PASS |
| OPEN untouched; manual zero PRState calls | same | OpenUntouched + ManualNeverTouched PASS | ✓ PASS |
| UnpushedCount/StashCount primitives | `go test ./internal/worktree` | TestUnpushedCount + TestStashCount PASS | ✓ PASS |
| Existing worktree DELETE tests unchanged | `go test ./internal/api` | 4 DELETE/cleanup tests PASS | ✓ PASS |
| Full suite + vet | `go test ./... && go vet ./cmd/...` | all ok, exit 0 | ✓ PASS |
| Frontend typecheck + build | `npx tsc --noEmit`; web/dist | exit 0; dist freshly built 16:46 | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| GHCLN-01 | 13-01,02,03 | Merged/closed PR worktree removed automatically | ✓ SATISFIED | reconcilePRsOnce auto-removes + deletes row; clean/closed tests PASS; human-verified |
| GHCLN-02 | 13-01,02,03 | Auto-removal gated (dirty/unpushed/sessions block); branch always kept | ✓ SATISFIED | 4-gate skip logic; force=false/stopSessions=false; no branch delete anywhere; skip tests PASS; human-verified |
| GHCLN-03 | 13-01,02,03 | Manual cleanup from review view via same gated flow | ✓ SATISFIED | PR ⋯ menu + banner → CleanupWorktreeDialog; manual keeps row; human-verified |

All three requirements marked **Complete** in REQUIREMENTS.md traceability (lines 101-103) — consistent with plan frontmatter declarations. No orphaned requirements.

### Anti-Patterns Found

None. No TODO/FIXME/placeholder/not-implemented markers, no empty-return stubs, no hardcoded empty data in any of the 10 modified files. The one documented deviation (13-02 Rule 1: fetch IN THE WORKTREE for per-worktree FETCH_HEAD) is a correctness bug-fix present in the verified code (reaper.go:294) and covered by the passing reconcile tests, not a gap.

### Human Verification Required

None outstanding. The 13-03 Task 3 human-verify gate was APPROVED 2026-06-14 — the user confirmed end-to-end in the running app: GHCLN-01 (clean merged PR worktree auto-removed + row deleted), GHCLN-02 (dirty/busy worktree left in place, branch kept), GHCLN-03 (manual cleanup via the ⋯ menu/banner through the gated dialog). No NEW unverified must-haves were found during this verification.

### Gaps Summary

No gaps. All 18 must-haves across the three plans verify against the actual codebase. The safety contract holds in code: the reaper never touches `source='manual'`, never forces past a gate (force=false), never stops a live session (stopSessions=false), the conservative gate skips on ANY of dirty/unpushed(rev-list FETCH_HEAD..HEAD after a per-worktree re-fetch)/stash/sessions, the git branch ref is never deleted (no `git branch -D`; Remove documents the guarantee), the D-07 row delete runs only on a successful auto-remove with tmux_sessions-before-tasks FK order, and manual cleanup keeps the row with columns nulled. Full Go suite + `go vet`, the 7-case reconcile matrix, the hermetic gate-primitive tests, the 4 unchanged worktree DELETE tests, and the frontend typecheck/build all pass. This is the final v1.3 phase; the goal is achieved.

---

_Verified: 2026-06-14_
_Verifier: Claude (gsd-verifier)_
