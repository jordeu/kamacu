---
phase: 23-worktree-cleanup-panel
plan: 02
subsystem: api
tags: [go, git-worktree, cleanup, http-handlers, tdd, union-scan, degrade-dont-break]

# Dependency graph
requires:
  - phase: 23-worktree-cleanup-panel (plan 23-01, Wave 1)
    provides: worktree.List/Entry parser + CleanupWorktreeGated orphan mode + D-01 BlockedError outcome
  - phase: 13-pr-worktree-auto-cleanup
    provides: CleanupWorktreeGated shared gated-removal core (handler + reaper callers)
  - phase: 03-worktree-per-task
    provides: worktree.Service gitRun idiom + DirtyCount/UnpushedCount/StashCount/ResolveBase/Remove
provides:
  - "GET /api/worktrees: cross-project annotated union scan grouped by project (WTREE-01/04, D-06/D-07)"
  - "POST /api/worktrees/remove: force-remove via CleanupWorktreeGated; D-01 block → 200 {outcome:blocked,path} (WTREE-02)"
  - "POST /api/worktrees/clean-eligible: server-recomputed eligible set, dry_run preview + best-effort execute, NEVER forces (WTREE-03)"
  - "POST /api/worktrees/clear-pointer: null a stale task's worktree columns + prune the stale git registration (D-07)"
  - "worktree.RefResolves (network-free base check) + worktree.Prune (stale-registration cleanup)"
  - "WorktreeCleanupRoutes wired in main.go with the shared ghSvc (PRStateGetter)"
affects: [23-03 (Wave 3 UI: section/rows/dialogs render these responses), worktree-cleanup-panel]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Shared enumerate() spine: list() and cleanEligible() both call one union-scan builder so the annotated list and the eligible set never drift"
    - "Panel is the 3rd caller of CleanupWorktreeGated (handler → reaper → panel) — no 4th removal path"
    - "Network-free per-row base resolution (Pattern 3): origin/<pr_base_ref> verified via RefResolves, else ResolveBase; unresolvable ⇒ unpushed:null + conservatively ineligible"
    - "degrade-don't-break on gh: PR-state display + PR-merged/closed eligibility call PRState only for github_pr rows; any gh error ⇒ null display / not-eligible, never a 500"
    - "DISPLAY-vs-ELIGIBILITY casing split: pr_state display lowercased (23-04 contract), eligibility compares the RAW uppercase MERGED/CLOSED"

key-files:
  created:
    - internal/api/cleanuppanel.go
    - internal/api/cleanuppanel_test.go
  modified:
    - internal/worktree/worktree.go
    - cmd/kamacu/main.go

key-decisions:
  - "enumerate() is the single union-scan spine both list and cleanEligible use (no drift); a per-project git-list failure is logged and skipped (its stale rows still surface), never a whole-GET 500"
  - "unpushedBase resolves origin/<pr_base_ref> ONLY when it exists locally (RefResolves, network-free), else falls back to ResolveBase; a detached orphan or a resolution failure yields unpushed:null and (for bulk) conservative ineligibility"
  - "buildRow emits blocked=false for every list row (blocked is only known after a remove attempt returns the blocked outcome — Wave 3 renders the banner from the remove response); the field is still emitted so the type stays stable"
  - "clearPointer prunes best-effort — a prune failure logs a warning but does not fail the DB-side pointer null (the load-bearing reconciliation)"

patterns-established:
  - "Cross-project worktree union scan (git worktree list ∪ task rows) classified referenced/orphan/stale, main worktree excluded by filepath.Clean match"
  - "Two-endpoint bulk (dry_run preview vs best-effort execute) recomputed server-side, never forcing, per-item skip on a raced gate"

requirements-completed: [WTREE-01, WTREE-02, WTREE-03, WTREE-04]

# Metrics
duration: 14min
completed: 2026-07-02
---

# Phase 23 Plan 02: Worktree Cleanup Panel Backend Summary

**The panel's HTTP contract: one annotated cross-project GET (`/api/worktrees`) enumerating + classifying every non-main worktree with per-row dirty/unpushed/stash/session flags and PR-state display, plus force-remove / bulk clean-eligible / clear-pointer actions — the 3rd caller of the shared `CleanupWorktreeGated`, wired in main.go with the shared `ghSvc`.**

## Performance

- **Duration:** ~14 min
- **Started:** 2026-07-02T14:15:42Z
- **Completed:** 2026-07-02T14:29:18Z
- **Tasks:** 2 (both TDD: RED → GREEN)
- **Files modified:** 4 (2 created: cleanuppanel.go + its test; 2 modified: worktree.go, main.go)

## Accomplishments
- `GET /api/worktrees` — the annotated union scan (D-06/D-07): every project's `git worktree list` (main + bare excluded, Pitfall 2) UNION'd with the DB's non-NULL-worktree_path task rows, classified `referenced` / `orphan` / `stale`, grouped by project, with `counts.{total,orphaned}`. Each referenced/orphan row that exists on disk carries `dirty` / `stash` / `sessions` and a network-free `unpushed` (null when the base can't resolve). `github_pr` rows carry a `pr_state` display suffix (lowercased from the raw `MERGED`/`CLOSED`/`OPEN`), degrading to `null` when gh is absent — never a 500.
- `POST /api/worktrees/remove` — per-item force-remove through the ONE shared `CleanupWorktreeGated` (D-03: force + stop_sessions honored from the body); a D-01 permission block maps to `200 {outcome:"blocked", path}` (not a 500), a raced gate to `409`, else `204`/`500`.
- `POST /api/worktrees/clean-eligible` — the provably-safe bulk (D-04/D-05): a server-recomputed eligible set (orphan OR referenced-done/pr-merged-or-closed, passing ALL four gates), previewed on `?dry_run=1` with `reason ∈ {orphaned,done,pr_merged,pr_closed}`, otherwise removed best-effort per item (`{removed, skipped}`). It **NEVER forces** (`stopSessions=false, force=false`; verified `grep` of the cleanEligible path = 0 force-true calls).
- `POST /api/worktrees/clear-pointer` — the D-07 reconcile: nulls a stale task's `branch`/`worktree_path`/`worktree_error` (keeps the row + branch) and best-effort `git worktree prune`s the stale registration; `204`.
- `worktree.RefResolves` (network-free `rev-parse --verify` base check) + `worktree.Prune` helpers; `WorktreeCleanupRoutes` wired in `cmd/kamacu/main.go` with the SAME `ghSvc` the reaper + PR routes use (no new construction).

## Task Commits

Each task was committed atomically (TDD: test → feat):

1. **Task 1 (RED): failing list-handler classification tests** - `0f7c52b` (test)
2. **Task 1 (GREEN): list handler + union scan + RefResolves** - `d55b854` (feat)
3. **Task 2 (RED): failing remove/clean-eligible/clear-pointer tests** - `893b767` (test)
4. **Task 2 (GREEN): action handlers + Prune + main.go wiring** - `f472248` (feat)

No REFACTOR commits — the GREEN implementations were clean up front (the shared `enumerate`/`unpushedBase`/`buildRow` helpers were factored during GREEN, not after).

## Files Created/Modified
- `internal/api/cleanuppanel.go` - NEW: `cleanupPanelHandlers` + `PRStateGetter` interface + the 3 replicated session-count helpers + `enumerate` (shared union-scan spine) + `list`/`remove`/`cleanEligible`/`clearPointer` handlers + `WorktreeCleanupRoutes`.
- `internal/api/cleanuppanel_test.go` - NEW: `stubPRState` spy + panel test harness; 6 list tests (classify, stale, dirty flag, unpushed-null degrade, PR-state lowercased, gh-absent degrade) + 5 action tests (remove 204, blocked→200, dry-run eligibility exclusion, execute best-effort, clear-pointer keeps row).
- `internal/worktree/worktree.go` - Added `RefResolves` (network-free `rev-parse --verify <ref>^{commit}`) and `Prune` (`git worktree prune`, mutex-serialized, branch-safe).
- `cmd/kamacu/main.go` - Registered `api.WorktreeCleanupRoutes(mux, db, wtSvc, mgr, tmuxClient, ghSvc)` next to the PR routes (shared `ghSvc` as `PRStateGetter`).

## Decisions Made
- **Single `enumerate()` spine** for both `list` and `cleanEligible` so the annotated list and the eligible set can never disagree — exactly the drift the plan's "factor a shared internal enumerate helper" directive calls for.
- **`unpushedBase` prefers `origin/<pr_base_ref>` only when it exists locally** (verified via the new network-free `RefResolves`), falling back to `ResolveBase`; a detached orphan or any resolution failure yields `unpushed:null` (row still renders) and, for bulk, conservative ineligibility. This is the network-free read-heavy annotate posture (Pattern 3 / Pitfall 3), not the reaper's `FETCH_HEAD` path.
- **DISPLAY-vs-ELIGIBILITY casing split honored:** the `pr_state` display field is `strings.ToLower(...)` of the raw `PRState` (23-04 wants `"merged"`), while `eligibilityReason` compares the RAW uppercase `"MERGED"`/`"CLOSED"` — as the plan explicitly requires.
- **`clearPointer` prunes best-effort:** the DB pointer-null is the load-bearing reconciliation; a `git worktree prune` failure logs a warning but does not fail the 204 (the stale dir is already gone).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added a network-free ref-resolution primitive (`worktree.RefResolves`)**
- **Found during:** Task 1 (implementing Pattern 3's network-free `origin/<pr_base_ref>` base check)
- **Issue:** The plan requires the panel to prefer `origin/<pr_base_ref>` "if that ref resolves" and fall back to `ResolveBase` — but no network-free "does this ref resolve locally?" primitive existed on `worktree.Service` (`resolvePRBase` in diffs.go shells out inline; `ResolveBase` always returns a base). Without one, the panel could not distinguish "PR base fetched" from "not fetched" without a git call scattered in the handler.
- **Fix:** Added `Service.RefResolves(ctx, wt, ref)` — a `rev-parse --verify --quiet <ref>^{commit}` wrapper (the `^{commit}` peel rejects non-commit refs), returning a boolean. Used only for the network-free base decision.
- **Files modified:** internal/worktree/worktree.go
- **Verification:** `TestWorktreeCleanupListDirtyFlag` (base resolvable → unpushed is a number) + `TestWorktreeCleanupListUnpushedNullDegrade` (detached orphan → null) exercise both branches; `go test ./internal/worktree/` green.
- **Committed in:** `d55b854` (Task 1 GREEN commit)

**2. [Rule 3 - Blocking] Added `worktree.Prune` for the stale-pointer reconcile**
- **Found during:** Task 2 (implementing `clearPointer`, D-07)
- **Issue:** The plan's `clearPointer` needs to prune the stale git registration after nulling the DB columns, but `worktree.Service` had no standalone prune method (`Remove` prunes only at its tail, and there is nothing to `Remove` for a stale pointer — the dir is already gone). The plan explicitly permitted "a small `wt.Prune(ctx, repo)` if Plan 01 added one, else `gitRun`-equivalent through an existing method"; Plan 01 did not add one.
- **Fix:** Added `Service.Prune(ctx, repo)` — a mutex-serialized `git worktree prune` (verified harmless no-op when nothing to prune; touches no branches, deletes no files).
- **Files modified:** internal/worktree/worktree.go
- **Verification:** `TestWorktreeCleanupClearPointer` asserts the row is kept with `worktree_path` NULL after the call; full suite green.
- **Committed in:** `f472248` (Task 2 GREEN commit)

---

**Total deviations:** 2 auto-fixed (both Rule 3 blocking — small worktree-service primitives the plan anticipated needing). No scope creep: both are minimal, tested, branch-safe git helpers the plan's own text called for.
**Impact on plan:** The implementation matches the plan verbatim; the two helpers are the mechanical enablers the plan explicitly left to the executor ("a small `wt.Prune` if Plan 01 added one, else …"; Pattern 3's network-free resolve check).

## Issues Encountered
- **The first `unpushed-null` test premise was wrong and was corrected.** My initial test seeded a `github_pr` row with an unresolvable `pr_base_ref` and expected `unpushed:null` — but per Pattern 3 the panel *correctly* falls back to `ResolveBase` (local `main`) and computes a real count. The true null-degrade case is when NO base resolves at all, so the test was rewritten to use a **detached orphan** worktree (`git worktree add --detach`), which has no branch base and legitimately yields `unpushed:null`. This validated the fallback behavior rather than masking it.
- **A stray `./kamacu` build binary** appeared after a verification `go build ./cmd/kamacu/` (which writes to cwd, not the gitignored `bin/`). Removed it before finishing — never staged.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Wave 3 (23-03, the Settings UI) can now build directly against a live backend: the JSON shapes match the 23-04 TypeScript contract byte-for-byte (`WorktreeRow` fields, `{projects, counts}`, `{outcome:"blocked", path}`, `{items:[EligibleItem]}`, `{removed, skipped}`).
- The panel is the 3rd (and final) caller of `CleanupWorktreeGated` — no 4th removal path exists, and the D-04 regression tests (`internal/api`, `internal/reaper`) stay green.
- No blockers. Threat register honored: every git call is arg-array `gitRun`/service-method (T-23-08, no shell); the server removes only paths it enumerated from `git worktree list` at DB-known `repo_path`s (T-23-07); bulk never forces (T-23-11); the D-01 block returns 200 and never `--force`-retries (T-23-10).

## Threat Flags
None — no new security surface beyond the threat register's already-mitigated items. All four endpoints operate on git-enumerated worktree paths at DB-known repo roots via the shared gated-removal core; no new auth path, no schema change, no `os.RemoveAll` on a client path.

---
*Phase: 23-worktree-cleanup-panel*
*Completed: 2026-07-02*
