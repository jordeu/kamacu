---
phase: 14-managed-checkout-foundations
verified: 2026-06-14T00:00:00Z
status: passed
score: 5/5 requirements verified (20/20 plan truths)
gaps: []
human_verification: []
---

# Phase 14: Managed Checkout Foundations Verification Report

**Phase Goal:** Kangent can clone a `gh`-validated GitHub repo into a managed, owner/name-namespaced checkout that it knows it owns, use that clone as the repo root for fresh-off-default-branch task worktrees, reattach to an already-cloned directory, and gated-remove the clone on project delete — while never touching user-pointed folder projects.
**Verified:** 2026-06-14
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

This phase is backend-only — there is no UI to human-verify. The full goal is substantiated by source + 24 passing Phase-14 tests run with `-count=1` (forced, not cached).

### Observable Truths (per plan must_haves)

| #  | Truth | Status | Evidence |
| -- | ----- | ------ | -------- |
| 1  | Folder project (pre-v1.4) reads back managed=false after migration | ✓ VERIFIED | `00008_managed_checkout.sql:9` `ADD COLUMN managed INTEGER NOT NULL DEFAULT 0`; `TestProjectManagedDefaultsZero` PASS |
| 2  | Row with managed=1 reads back managed=true through Project JSON | ✓ VERIFIED | `projects.go:92-97` scan int→bool; `Managed bool json:"managed"` at `projects.go:37` |
| 3  | github.Clone runs `gh repo clone` and reports success ONLY on exit 0 | ✓ VERIFIED | `clone.go:67-74` arg-array `gh repo clone`, exit-0-only; `TestCloneSuccess` PASS |
| 4  | github.Clone removes dest on failure, returns trimmed stderr | ✓ VERIFIED | `clone.go:50-57` `os.RemoveAll(dest)` + stderr trim; `TestCloneFailureRemovesDest` PASS |
| 5  | projectHandlers carries wt/mgr/tmuxClient for gated delete | ✓ VERIFIED | `projects.go:47-52`; `routes.go:19` constructs all four fields |
| 6  | Valid owner/name → gh-validated, cloned to ~/.kangent/repos/<owner>/<name>, row managed=1 + github_repo=canonical | ✓ VERIFIED | `projects.go:190-268` ordered sequence; `TestCreateRepoSuccess` PASS |
| 7  | Invalid/inaccessible repo → 400, NO clone, NO row (RPROJ-05) | ✓ VERIFIED | `projects.go:191-211` validate-before-clone; `TestCreateRepoValidateBeforeClone` PASS (clone runner fails test if invoked) |
| 8  | Clone failure → NO row AND NO dir (atomic, D-01) | ✓ VERIFIED | `projects.go:245-251`; `TestCreateRepoCloneAtomicity` asserts row count unchanged + `os.Stat(dest)` IsNotExist |
| 9  | Reattach on matching origin (no re-clone), insert managed=1 (CKOUT-05) | ✓ VERIFIED | `projects.go:224-231,275-297` `reattachManaged`; `TestReattachOriginMatch` PASS |
| 10 | Reattach mismatch → error, dir NOT clobbered | ✓ VERIFIED | `projects.go:290-293`; `reattachManaged` contains NO os.Remove/RemoveAll; `TestReattachOriginMismatch` asserts 409 + dir intact |
| 11 | gh unavailable → msgGHUnavailable, folder path still usable | ✓ VERIFIED | `projects.go:204-211`; `TestCreateRepoGHUnavailable` PASS |
| 12 | Managed task worktree fetches latest default branch before ResolveBase (CKOUT-02) | ✓ VERIFIED | `tasks.go:130-148` `if managed { FetchRef } ; ResolveBase`; `TestManagedTaskWorktreeFetchesLatest` asserts origin/main advanced |
| 13 | Folder (managed=0) worktree performs NO fetch — D-24 byte-for-byte | ✓ VERIFIED | `tasks.go:130` gated on managed; `TestFolderTaskWorktreeNoFetch` asserts origin/main did NOT move |
| 14 | Failed fetch does NOT block task creation (best-effort, D-05) | ✓ VERIFIED | `tasks.go:144` `_ = wt.FetchRef(...)` discarded; `TestManagedFetchBestEffortDoesNotBlock` asserts worktree still provisions |
| 15 | PR-review worktrees go through the same managed-fetch path | ✓ VERIFIED | single choke point `provisionWorktree`; `worktrees.go:156` Retry caller passes managed; `loadTaskRepo` returns managed (no source filter) |
| 16 | Managed delete clean+idle → remove worktrees, RemoveAll clone, delete row (204) | ✓ VERIFIED | `projects.go:638-682`; `TestManagedDeleteAllClean` asserts clone gone + wt gone + rows cascaded |
| 17 | Any dirty/unpushed/stash/session blocker → 409 reason list, nothing removed | ✓ VERIFIED | `projects.go:521-605` two-pass gate; `TestManagedDeleteGate{Dirty,Unpushed,Stash,CloneUnpushed}` via `assertBlocked` (409 + reasons + clone/wt/row all intact) |
| 18 | Folder delete unchanged — directory NEVER touched (D-09) | ✓ VERIFIED | `projects.go:481-494` managed==0 plain DELETE, no disk ops; `TestFolderDeleteNeverTouchesDir` asserts dir survives |
| 19 | Clone root removed with os.RemoveAll, never `git worktree remove`; worktrees removed FIRST | ✓ VERIFIED | `projects.go:639-667` loop then `os.RemoveAll(clone)`; no `.Remove(` on projects.go; `TestManagedDeleteOrdering` PASS |
| 20 | Unpushed gate uses origin/<default>..HEAD with NO fetch | ✓ VERIFIED | `projects.go:509-510,554,583` `UnpushedCount(ctx, path, "origin/"+defBranch)`; gate phase has no FetchRef |

**Score:** 20/20 plan truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/store/migrations/00008_managed_checkout.sql` | ADD COLUMN managed + DROP down | ✓ VERIFIED | Exact SQL present, lines 9 & 14 |
| `internal/github/clone.go` | Clone(ctx,ref,dest) + overridable runner seam | ✓ VERIFIED | `Clone` exported; `var cloneRunner = realClone` seam line 19 |
| `internal/github/clone_test.go` | file:// fixtures + runner override | ✓ VERIFIED | `TestCloneSuccess`/`TestCloneFailureRemovesDest` PASS |
| `internal/api/projects.go` | Managed field, createByRepo, reattachManaged, deleteManaged | ✓ VERIFIED | All present and substantive (759 lines) |
| `internal/api/tasks.go` | managed-gated best-effort fetch in provisionWorktree | ✓ VERIFIED | `tasks.go:87,130-148,178-180` |
| `internal/api/worktrees.go` | loadTaskRepo selects managed, threads to callers | ✓ VERIFIED | `worktrees.go:47-59,156` |
| `internal/api/routes.go` | projectHandlers{db,wt,mgr,tmuxClient} | ✓ VERIFIED | `routes.go:19` |

All artifacts pass via `gsd-tools verify artifacts` (14-01..14-04: all_passed=true).

### Key Link Verification

(gsd-tools `verify key-links` reported "Source file not found" — a tool path-resolution artifact: the `from` field embeds extra text after the path. All links verified manually via grep.)

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| projects.go scanProject | projects.managed column | int scan → bool | ✓ WIRED | `projects.go:92-97` |
| routes.go Routes | projectHandlers{...} | struct construction | ✓ WIRED | `routes.go:19` |
| projects.go createByRepo | ParseRepoRef/ValidateRepo/Clone | validate-then-clone-then-insert | ✓ WIRED | `projects.go:192,199,246` in order |
| projects.go createByRepo | row managed=1 | INSERT ... managed VALUES ... 1 | ✓ WIRED | `projects.go:260` |
| tasks.go provisionWorktree | wt.FetchRef gated managed | error ignored, before ResolveBase | ✓ WIRED | `tasks.go:144` `_ = wt.FetchRef` before `tasks.go:148` ResolveBase |
| worktrees.go loadTaskRepo | projects.managed | scalar subquery | ✓ WIRED | `worktrees.go:54` |
| projects.go deleteManaged | Dirty/Unpushed/StashCount + sessions | gate-all before removal | ✓ WIRED | `projects.go:544,554,561,567` |
| projects.go removeManaged | os.RemoveAll(clone) after wt removal | ordered remove phase | ✓ WIRED | `projects.go:645` loop then `:664` RemoveAll |

### Data-Flow Trace (Level 4)

Backend-only phase; no dynamic-data-rendering components. Data flow verified via the test fixtures: real `file://` clones with real commits prove FetchRef advances origin/main (truth #12), DefaultBranch reads real symbolic-ref (`worktree.go:163-168`), UnpushedCount runs real `rev-list` (`worktree.go:307-317`). All gate primitives shell real git with arg arrays.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Build compiles | `go build ./...` | exit 0 | ✓ PASS |
| Vet clean | `go vet ./...` | exit 0 | ✓ PASS |
| Full suite | `go test ./...` | all ok | ✓ PASS |
| Phase-14 api+github tests (forced) | `go test -run '...' -count=1` | 24/24 PASS | ✓ PASS |

Phase-14 tests (all PASS): TestCloneSuccess, TestCloneFailureRemovesDest, TestProjectManagedDefaultsZero, TestCreateRepoSuccess, TestCreateRepoValidateBeforeClone, TestCreateRepoGHUnavailable, TestCreateRepoCloneAtomicity, TestReattachOriginMatch, TestReattachOriginMismatch, TestReattachNonGitDir, TestReattachAlreadyAdded, TestManagedTaskWorktreeFetchesLatest, TestFolderTaskWorktreeNoFetch, TestManagedFetchBestEffortDoesNotBlock, TestManagedDeleteGateDirty, TestManagedDeleteGateUnpushed, TestManagedDeleteGateStash, TestManagedDeleteGateCloneUnpushed, TestManagedDeleteAllClean, TestManagedDeleteOrdering, TestFolderDeleteNeverTouchesDir, TestManagedDeleteUnknownID, plus PR-review reattach + session reattach regressions.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| CKOUT-01 | 14-01, 14-02 | `gh repo clone` into ~/.kangent/repos/<owner>/<name>, managed root | ✓ SATISFIED (REQUIREMENTS.md:63 Complete) | clone.go + createByRepo + INSERT managed=1; TestCreateRepoSuccess |
| RPROJ-05 | 14-02 | gh-validate before any clone; invalid → no clone, no row | ✓ SATISFIED (REQUIREMENTS.md:62 Complete) | projects.go:191-211; TestCreateRepoValidateBeforeClone |
| CKOUT-02 | 14-03 | Worktrees branch off managed checkout after fetching latest default | ✓ SATISFIED (REQUIREMENTS.md:64 Complete) | tasks.go fetch gate; TestManagedTaskWorktreeFetchesLatest/FolderTaskWorktreeNoFetch |
| CKOUT-05 | 14-02 | Re-add reattaches to existing managed dir, never clobbers | ✓ SATISFIED (REQUIREMENTS.md:67 Complete) | reattachManaged; TestReattachOriginMatch/Mismatch/NonGitDir |
| CKOUT-03 | 14-04 | Gated removal of managed clone; folder dirs never removed | ✓ SATISFIED (REQUIREMENTS.md:65 Complete) | deleteManaged two-pass; 4 gate tests + AllClean + FolderDelete |

No orphaned requirements — all five Phase-14 IDs (REQUIREMENTS.md:62-67) appear in plan frontmatter AND are marked Complete.

### Anti-Patterns Found

None. Scan of all nine Phase-14 source/test files:

| Concern | Result |
| ------- | ------ |
| TODO/FIXME/XXX/HACK/PLACEHOLDER | None (all `"todo"` matches are the kanban status value, not debt markers) |
| `sh -c` / exec sh / bash | None (only comments asserting "never sh -c" — arg arrays used throughout) |
| `git branch -D` / `branch -d` | None new (only `"branch"` JSON keys, `Branch` field, and a read-only `git branch --list` in tests) |
| Stub returns (return null/[]/{} to render) | None — all returns carry real data or real errors |

Security checks confirmed: `clone.go:68` and `reattachManaged` (`projects.go:278,283`) use `exec.CommandContext` arg arrays; no shell interpolation of the attacker-adjacent `ref`/origin input.

### Regression Safety (folder projects)

| Surface | Status | Evidence |
| ------- | ------ | -------- |
| Folder create INSERT unchanged | ✓ | `projects.go:174` `INSERT INTO projects (name, repo_path)` — no managed/github_repo cols; managed defaults to 0 |
| Folder delete never touches dir | ✓ | `projects.go:482-494` managed==0 → plain `DELETE FROM projects`, no git/disk; TestFolderDeleteNeverTouchesDir asserts dir survives |
| Folder worktree no-fetch (D-24) | ✓ | `tasks.go:130` fetch gated on managed; TestFolderTaskWorktreeNoFetch asserts origin unchanged |
| Clone root never `git worktree remove`d | ✓ | No `.Remove(` in projects.go; clone passed only as common-dir `repo` arg to CleanupWorktreeGated (cleanup.go:80 removes `path`, the linked worktree, not the clone) |

### Human Verification Required

None. Backend-only phase; all guarantees are substantiated by source inspection and 24 passing real-git tests.

### Gaps Summary

No gaps. Every locked decision D-01..D-11 is honored:
- D-01 atomicity: row inserted only after clone exit 0; failure → no row + os.RemoveAll (truth #8).
- D-02/CKOUT-01: clone into ~/.kangent/repos/<owner>/<name>, managed=1 (truths #6).
- D-03/RPROJ-05: ParseRepoRef→ValidateRepo before any clone (truth #7).
- D-04/D-05/CKOUT-02: managed-only best-effort fetch, failure never blocks (truths #12,#14).
- D-06: migration 00008 marker column, backfill 0 (truth #1).
- D-07/D-08: two-pass all-or-nothing gated delete, origin/<default>..HEAD no-fetch unpushed safety net (truths #17,#20).
- D-09: folder dir never touched on delete (truth #18).
- D-10/D-11/CKOUT-05: reattach on origin-match, never clobbers (truths #9,#10).

Minor note (not a gap): plan 14-04's frontmatter listed `internal/api/projects_test.go` for its tests, but they were implemented in a sibling file `internal/api/projects_delete_test.go`. The tests exist, are named as planned, and pass — the file split is cosmetic and does not affect coverage.

---

_Verified: 2026-06-14_
_Verifier: Claude (gsd-verifier)_
