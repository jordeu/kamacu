---
phase: 14-managed-checkout-foundations
plan: 03
subsystem: api
tags: [go, git, worktree, fetch, managed-checkout, sqlite]

# Dependency graph
requires:
  - phase: 14-managed-checkout-foundations
    provides: "migration 00008 projects.managed marker (plan 01) — the boolean the fetch is gated on"
  - phase: 03-worktrees
    provides: "worktree.ResolveBase / FetchRef / Create — the base resolver and the scoped-fetch primitive reused"
provides:
  - "provisionWorktree gains a managed bool: a best-effort, error-discarded default-branch fetch runs immediately before ResolveBase, gated on managed"
  - "worktree.DefaultBranch(ctx, repo) — reads origin/HEAD (the same ref ResolveBase reads) and strips the origin/ prefix"
  - "loadTaskRepo returns the project's managed marker, threading it to the Retry/PR-review worktree-creation path"
  - "managed-only freshness for every worktree-creation surface (task create + POST /worktree Retry + PR-review) through the single provisionWorktree choke point"
affects: [14-04-gated-delete, 15-repo-first-creation-flow]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "managed-gated, error-discarded best-effort fetch (D-05) before ResolveBase — a freshness convenience, never a provisioning gate (not routed through D-25 worktree_error)"
    - "second deliberate, scoped exception to worktree's never-fetch invariant (the first was PR-head/base fetch), documented in the package doc"
    - "real-git file:// origin+clone fixtures with per-command GIT_CONFIG isolation (api package has no TestMain) to prove fetch / no-fetch / best-effort-no-block"

key-files:
  created: []
  modified:
    - "internal/api/tasks.go"
    - "internal/api/worktrees.go"
    - "internal/worktree/worktree.go"
    - "internal/api/tasks_test.go"

key-decisions:
  - "The default-branch git read lives in a new worktree.DefaultBranch method (where the arg-array/never-fetch invariants live); tasks.go's defaultBranchName helper delegates to it rather than shelling out from the api layer."
  - "On a failed default-branch-name resolve, SKIP the fetch (no fallback to an all-refs `fetch origin`) — D-04 wants the targeted default branch; ResolveBase still works on the local base."
  - "The pre-task-fetch tests insert the managed/folder project row directly (not via the repo-first create endpoint, which is plan 02's surface) to keep them independent of plan 02."

patterns-established:
  - "managed gate: every network touch in worktree provisioning is behind `if managed`, so folder projects keep ResolveBase's D-24 no-network guarantee byte-for-byte."
  - "best-effort fetch: `_ = wt.FetchRef(...)` with the error deliberately discarded — degrade-don't-break (D-05)."

requirements-completed: [CKOUT-02]

# Metrics
duration: 7 min
completed: 2026-06-14
---

# Phase 14 Plan 03: Pre-Task Default-Branch Fetch Summary

**Managed-checkout task/PR-review worktrees now `git fetch origin <defaultBranch>` best-effort before `ResolveBase`, so new work starts from the freshest tip — gated on the `managed` marker so folder projects keep their no-network guarantee, and the fetch failure is discarded so it never blocks task creation.**

## Performance

- **Duration:** ~7 min
- **Started:** 2026-06-14T17:26:43Z
- **Completed:** 2026-06-14T17:33:28Z
- **Tasks:** 2
- **Files modified:** 4 (0 created, 4 modified)

## Accomplishments

- **`provisionWorktree` gains a `managed bool`** (CKOUT-02/D-04): immediately before `wt.ResolveBase`, a managed checkout best-effort `FetchRef`es its default branch so the new worktree branches off the freshest origin tip. The fetch error is discarded (`_ =`) — it is a freshness convenience (D-05), not a provisioning gate, and is deliberately NOT routed through the D-25 `worktree_error`/fail path.
- **Folder projects (`managed=false`) never fetch** — they take the exact same `ResolveBase`-only path as before, preserving D-24's "no network, ever" byte-for-byte (proven by `TestFolderTaskWorktreeNoFetch`).
- **`worktree.DefaultBranch(ctx, repo)`** reads the same `refs/remotes/origin/HEAD` symbolic ref `ResolveBase` reads and strips the `origin/` prefix, giving the precise, cheap fetch target. On absence it errors → `provisionWorktree` skips the fetch (no all-refs fallback).
- **`loadTaskRepo` now returns `managed`**, threading it to the POST `/worktree` Retry and PR-review provisioning path — the single `provisionWorktree` choke point covers every worktree-creation surface in one change.
- **Second scoped-fetch exception documented** in the `worktree` package doc (mirroring the CheckoutPR/FetchRef carve-out): the per-new-task default-branch fetch joins PR-head/base fetch as a deliberate, gated relaxation of the never-fetch invariant.

## Task Commits

Each task committed atomically:

1. **Task 1: Thread `managed` + managed-only best-effort fetch before ResolveBase** - `fb02e40` (feat)
2. **Task 2: loadTaskRepo returns managed; file:// fetch / no-fetch / best-effort tests** - `d32a52e` (test)

_Task 1's commit also carried the `loadTaskRepo` signature change (its create-caller needed the `managed` value); Task 2 added the test coverage._

## Files Created/Modified

- `internal/api/tasks.go` - `provisionWorktree` takes `managed bool`; managed-gated best-effort fetch before `ResolveBase`; `defaultBranchName` helper; create() caller selects `managed` and passes it
- `internal/api/worktrees.go` - `loadTaskRepo` selects+returns `managed` (second scalar subquery); all three call sites updated; Retry path passes `managed` to `provisionWorktree`
- `internal/worktree/worktree.go` - new `DefaultBranch` method (origin/HEAD reader); package-doc note of the second scoped-fetch exception
- `internal/api/tasks_test.go` - `gitOut` + `makeOriginAndClone`/`advanceOrigin`/`cloneOriginMainSHA`/`insertProjectRow` helpers; the three managed/folder/best-effort tests

## Decisions Made

- **The default-branch git read lives in `worktree.DefaultBranch`, not in the api layer.** The `worktree` package owns the arg-array, `-C <repo>`, exit-0-only, never-fetch invariants; reading `origin/HEAD` is a worktree concern. `tasks.go`'s `defaultBranchName(ctx, wt, repo)` delegates to it — keeping the api layer free of direct git shell-outs and reusing the exact ref `ResolveBase` reads.
- **A failed default-branch-name resolve SKIPS the fetch** rather than falling back to an all-refs `fetch origin`. D-04 wants the targeted default branch (cheap), and a skipped fetch is harmless — `ResolveBase` still resolves the local base.
- **The pre-task-fetch tests insert the project row directly** with `managed=0/1` instead of going through the repo-first create endpoint (plan 02's surface). This keeps plan 03's coverage independent of plan 02 and lets a single test toggle the marker.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] `gitIn` test helper name already taken**
- **Found during:** Task 2 (writing the file:// fixtures)
- **Issue:** The plan's read-firsts pointed at `worktree_test.go`'s `gitCmd`/`cloneRepo`, but the api package already has its own `gitIn` (in `worktrees_test.go`) that returns nothing — my output-returning helper collided with it (`gitIn redeclared`).
- **Fix:** Named the output-returning variant `gitOut` (used for `rev-parse` reads) and reused the existing no-output `gitIn` for the fixture-build commands.
- **Files modified:** internal/api/tasks_test.go
- **Verification:** `go test ./internal/api/...` compiles and the three new tests pass.
- **Committed in:** d32a52e (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** A test-helper naming collision only; no behavior change. The plan's "create tasks_test.go if it doesn't exist" assumed a fresh file, but `tasks_test.go` already exists with a full harness — the new tests/helpers were appended to it. No scope creep.

## Issues Encountered

None — both tasks executed essentially as written.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- CKOUT-02 is complete: managed checkouts start every new task/PR-review worktree from the freshest default branch (best-effort), folder projects are byte-for-byte unchanged, and a failed fetch never blocks creation.
- The remaining wave-2 plan 14-04 (gated managed-clone delete) is disjoint from this plan's files (it touches `projects.go`); the `managed` marker it gates on (plan 01) and the worktree gate primitives are already in place.
- `go build ./...` and `go test ./internal/...` are green (full suite, not just the targeted tests).

---
*Phase: 14-managed-checkout-foundations*
*Completed: 2026-06-14*

## Self-Check: PASSED

- All modified files exist on disk (tasks.go, worktrees.go, worktree.go, tasks_test.go, 14-03-SUMMARY.md).
- Both task commits present in git history (fb02e40, d32a52e).
- `go build ./...` and `go test ./internal/...` green (full suite).
