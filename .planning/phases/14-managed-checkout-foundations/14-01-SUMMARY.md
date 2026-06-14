---
phase: 14-managed-checkout-foundations
plan: 01
subsystem: database
tags: [sqlite, goose, migration, github, gh, clone, go, api]

# Dependency graph
requires:
  - phase: 10-github-foundations
    provides: "internal/github package (ParseRepoRef/ValidateRepo/Available, github_repo column via migration 00007)"
  - phase: 13-pr-worktree-auto-cleanup
    provides: "CleanupWorktreeGated + worktree/session/tmux gate primitives the gated delete reuses"
provides:
  - "migration 00008: projects.managed INTEGER NOT NULL DEFAULT 0 (the Kangent-owns-this-dir marker, D-06)"
  - "Project.Managed wired end-to-end (struct -> projectColumns -> scanProject -> JSON)"
  - "github.Clone(ctx, ref, dest) — gh repo clone wrapper, exit-0-only, remove-on-failure, package-level cloneRunner test seam"
  - "projectHandlers carries wt/mgr/tmuxClient (wired in Routes) for the wave-2 gated managed-clone delete"
affects: [14-02-create-by-repo, 14-03-pre-task-fetch, 14-04-gated-delete, 15-repo-first-creation-flow]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "boolean-as-INTEGER 0/1 marker column (SQLite idiom), scanned via int -> bool to avoid modernc bool-scan friction"
    - "package-level overridable runner var (var cloneRunner = realClone) as a unit-test seam — mirrors service.go Config.Runner but for a package function"
    - "exit-0-only success + os.RemoveAll-on-failure + trimmed-stderr (fatal: stripped) for a shell-out verb"

key-files:
  created:
    - "internal/store/migrations/00008_managed_checkout.sql"
    - "internal/github/clone.go"
    - "internal/github/clone_test.go"
  modified:
    - "internal/api/projects.go"
    - "internal/api/routes.go"
    - "internal/api/projects_test.go"

key-decisions:
  - "Schema marker is a single boolean-as-INTEGER `managed` column (D-06), not an enum or path-prefix derivation — the column is the single source of truth for ownership; path derivation is a data-loss hazard (STATE.md Blocker)."
  - "Clone seam is a package-level `var cloneRunner` (not a struct field) because Clone is a package function like ValidateRepo — tests swap it without a live gh."
  - "scanProject reads managed into an int and maps `!= 0` to bool, sidestepping modernc INTEGER->bool scan friction."

patterns-established:
  - "Marker column: managed INTEGER NOT NULL DEFAULT 0 backfills every pre-v1.4 row to not-managed (folder, never-touch), the safe default."
  - "Shell-out verb seam: a package-level runner var returning (stderr, err) lets Clone own its success/cleanup logic while tests inject canned outcomes."

requirements-completed: [CKOUT-01]

# Metrics
duration: 13 min
completed: 2026-06-14
---

# Phase 14 Plan 01: Managed Checkout Foundations Summary

**Migration 00008 `managed` marker column wired through Project JSON, a `github.Clone` verb wrapping `gh repo clone` (exit-0-only, remove-on-failure, faked-runner test seam), and projectHandlers gaining wt/mgr/tmuxClient for the wave-2 gated delete.**

## Performance

- **Duration:** ~13 min
- **Started:** 2026-06-14T16:59Z (approx)
- **Completed:** 2026-06-14T17:12:07Z
- **Tasks:** 3 (one TDD)
- **Files modified:** 6 (3 created, 3 modified)

## Accomplishments

- **Migration 00008** adds `managed INTEGER NOT NULL DEFAULT 0` to `projects` (DROP COLUMN in Down), backfilling every existing folder project to managed=0 — the single source of truth for "Kangent owns this dir" (D-06/D-09). Applies cleanly through the existing migrate-on-open harness.
- **`github.Clone(ctx, ref, dest)`** wraps `gh repo clone` via an arg array (never `sh -c`): success is exit 0 only; on any failure it `os.RemoveAll(dest)` and returns the trimmed stderr (leading `fatal: ` stripped). No short timeout (large clones take minutes). A package-level `cloneRunner` seam lets unit tests fake the clone without a live gh — verified with real `file://` fixtures for success and a canned-error fake for the remove-on-failure path.
- **`Project.Managed`** flows end-to-end: struct field (`json:"managed"`) → `projectColumns` → `scanProject` (INTEGER 0/1 → bool) → JSON. A folder-created project serializes `"managed": false`.
- **`projectHandlers`** now carries `wt`/`mgr`/`tmuxClient` (constructed in `Routes`, mirroring `taskHandlers`) so plan 04 can wire the gated managed-clone delete with no further plumbing.

## Task Commits

Each task committed atomically:

1. **Task 1: Migration 00008 — managed marker column** - `05c39c0` (feat)
2. **Task 2: github.Clone verb + test seam** (TDD) - `058f274` (test/RED) → `0deadc7` (feat/GREEN)
3. **Task 3: Project.Managed wire/scan + projectHandlers struct & Routes wiring** - `8a97ace` (feat)

_No REFACTOR commit for Task 2 — the GREEN implementation was already minimal and clean._

## Files Created/Modified

- `internal/store/migrations/00008_managed_checkout.sql` - adds/drops the `managed` boolean-as-INTEGER column on `projects`
- `internal/github/clone.go` - `Clone` verb + `cloneRunner`/`realClone` seam (exit-0-only, remove-on-failure)
- `internal/github/clone_test.go` - real-git `file://` success test + faked-runner failure/remove test (mirrors worktree_test.go fixtures)
- `internal/api/projects.go` - `Project.Managed` field, `managed` in `projectColumns`/`scanProject`, `projectHandlers` gains wt/mgr/tmuxClient + imports
- `internal/api/routes.go` - constructs `projectHandlers` with wt/mgr/tmuxClient (args already in `Routes` signature)
- `internal/api/projects_test.go` - `TestProjectManagedDefaultsZero` proves folder create serializes `managed=false`

## Decisions Made

- **Boolean-as-INTEGER `managed` column, not an enum or path-prefix derivation.** The only branch anyone needs is binary (own-the-dir vs never-touch); an enum adds a CHECK + string handling for no behavior. A path-prefix derivation is a data-loss hazard (STATE.md's highest-severity Blocker). A real column is the single source of truth.
- **Package-level `var cloneRunner` seam** (not a struct field) — `Clone` is a package function like `ValidateRepo`, so the test seam mirrors `service.go`'s `Config.Runner` indirection at package scope.
- **`scanProject` reads managed via an `int` (`managedInt != 0`)** to sidestep any modernc INTEGER→bool scan friction, mirroring how `github_repo` used `sql.NullString`.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- The shared contracts the wave-2 plans build against are in place: the `managed` marker (read by create-by-repo, pre-task fetch, gated delete), `github.Clone` (called by create-by-repo, plan 02, with a faked-runner path for atomicity tests), and the `projectHandlers` deps (gated delete, plan 04).
- No change to create/delete behavior yet — plans 02 (create-by-repo + reattach) and 04 (gated delete) own those, exactly as scoped.
- `go build ./...` and `go test ./internal/...` are green.

---
*Phase: 14-managed-checkout-foundations*
*Completed: 2026-06-14*

## Self-Check: PASSED

- All created files exist on disk (migration 00008, clone.go, clone_test.go, SUMMARY.md).
- All task commits present in git history (05c39c0, 058f274, 0deadc7, 8a97ace).
- `go build ./...` and `go test ./internal/...` green.
