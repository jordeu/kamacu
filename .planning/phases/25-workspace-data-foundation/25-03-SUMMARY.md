---
phase: 25-workspace-data-foundation
plan: 03
subsystem: database
tags: [go, sqlite, startup-hook, backfill, workspaces, idempotent]

# Dependency graph
requires:
  - phase: 25-01
    provides: "migration 00012 (workspaces table + projects.workspace_id NOT NULL DEFAULT 1 FK; seeds the default Personal workspace)"
provides:
  - "api.BackfillWorkspaces(db) — idempotent startup guard guaranteeing a default (Personal) workspace row always exists"
  - "startup wiring in cmd/kamacu/main.go running the guard after store.Migrate + BackfillProjectIcons"
affects: [phase-26-workspace-switcher, workspace-crud]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Lean idempotent startup invariant-guard (SELECT default -> no-op if present, re-INSERT if missing) mirroring BackfillProjectIcons but without a SELECT->UPDATE loop"

key-files:
  created:
    - internal/api/workspaces.go
    - internal/api/workspaces_test.go
  modified:
    - cmd/kamacu/main.go

key-decisions:
  - "D-07 reinterpreted as the LEAN guard (user-confirmed 2026-07-05): BackfillWorkspaces only guarantees a default workspace row exists; NO project-reassignment loop because migration 00012's NOT NULL DEFAULT 1 FK already assigns every project."
  - "Hook is a startup-only backstop, NOT an HTTP endpoint (all workspace CRUD/filtering/transfer deferred to Phase 26, honoring D-11)."

patterns-established:
  - "Idempotent boot-time invariant guard: QueryRow for the invariant, errors.Is(err, sql.ErrNoRows) branch to re-establish it, propagate all other errors — parameterless/literal SQL only."

requirements-completed: [WSDATA-02]

# Metrics
duration: 12min
completed: 2026-07-05
---

# Phase 25 Plan 03: Workspace Startup Backfill Guard Summary

**Every boot now guarantees a default Personal workspace exists via the idempotent `BackfillWorkspaces` startup hook, wired right after `BackfillProjectIcons` — the thin invariant-guard backstop for WSDATA-02.**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-07-05T10:49Z
- **Completed:** 2026-07-05T10:55Z
- **Tasks:** 2 completed
- **Files modified:** 3 (2 created, 1 modified)

## Accomplishments
- Implemented `BackfillWorkspaces(db *sql.DB) error` — a lean, idempotent startup guard that no-ops when a default workspace exists, re-creates Personal (is_default=1) when it is missing, and propagates real DB errors (only `sql.ErrNoRows` triggers re-create).
- Wired the guard into `cmd/kamacu/main.go` immediately after `api.BackfillProjectIcons(db)` (after `store.Migrate`), guarded with `slog.Error` + `os.Exit(1)`, matching the icon-backfill block.
- Proved all three behaviors with a migrated-DB TDD test (no-op on healthy boot, re-create on missing default, single Personal after a double run).

## Task Commits

Each task was committed atomically:

1. **Task 1 (RED): failing test for BackfillWorkspaces** - `dc52a0e` (test)
2. **Task 1 (GREEN): implement BackfillWorkspaces idempotent guard** - `d2093e3` (feat)
3. **Task 2: wire BackfillWorkspaces into main.go startup** - `396c31f` (feat)

_TDD task 1 produced test -> feat commits; no refactor commit was needed (the minimal implementation was already clean)._

## Files Created/Modified
- `internal/api/workspaces.go` (created) - `BackfillWorkspaces` idempotent startup guard; doc-comment records the D-07 lean reinterpretation and the T-25-01 parameterless-SQL rationale.
- `internal/api/workspaces_test.go` (created) - `TestBackfillWorkspaces` on a `store.Open`+`store.Migrate` harness: no-op on healthy boot, re-create on missing default, idempotent double run.
- `cmd/kamacu/main.go` (modified) - added the `api.BackfillWorkspaces(db)` block after `BackfillProjectIcons`, with a load-bearing-ordering comment.

## Deviations from Plan

None - plan executed exactly as written (lean hook per the user-confirmed D-07 reinterpretation).

## Decisions Made
- **Lean guard, not a reassign loop (D-07, user-confirmed 2026-07-05):** migration 00012's `NOT NULL DEFAULT 1` FK already assigns every project to Personal, so a SELECT->UPDATE loop over projects would be provably empty. The hook only guarantees the default workspace *row* exists (invariant 1); no-project-is-workspace-less (invariant 2) is enforced structurally by the schema. This reinterprets D-07's mechanism while fully meeting its stated job — not an open concern.
- **Startup hook only:** `BackfillWorkspaces` is a boot-time backstop, not an HTTP endpoint; all workspace CRUD/filtering/transfer stays deferred to Phase 26 (D-11).

## Verification
- `go build ./...` exits 0.
- `go vet ./internal/api/... ./cmd/kamacu/` exits 0.
- `go test ./internal/api/ -run TestBackfillWorkspaces -v` PASSES (no-op, re-create, idempotent).
- Full `go test ./internal/api/...` green (85.7s; the known-flaky TestSessionTmuxReattach passed under full load this run).
- `grep -n 'api.BackfillWorkspaces(db)' cmd/kamacu/main.go` -> line 134, positioned after the `BackfillProjectIcons` block.

## Threat Surface
Threat register dispositions honored: T-25-01 (SQL injection) mitigated — only a parameterless SELECT and a literal `'Personal'` INSERT, no string-concatenated input. T-25-05 (missing-default invariant) mitigated — the guard re-creates Personal when absent, proven by the test. No new security surface introduced.

## Known Stubs
None.

## Self-Check: PASSED
- Files verified present: `internal/api/workspaces.go`, `internal/api/workspaces_test.go`, `cmd/kamacu/main.go`, `.planning/phases/25-workspace-data-foundation/25-03-SUMMARY.md`.
- Commits verified present: `dc52a0e` (test), `d2093e3` (feat), `396c31f` (feat), `b965004` (docs).
- Working tree clean (stray root `kamacu` build artifact from `go build ./cmd/kamacu/` verification removed; not part of this plan).
