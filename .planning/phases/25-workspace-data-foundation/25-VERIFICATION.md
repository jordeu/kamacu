---
phase: 25-workspace-data-foundation
verified: 2026-07-05T18:45:00Z
status: passed
score: 13/13 must-haves verified
overrides_applied: 0
re_verification:
  # No previous VERIFICATION.md existed — initial verification.
---

# Phase 25: Workspace Data Foundation Verification Report

**Phase Goal:** Every project belongs to exactly one workspace, with all existing projects migrated into a protected default **Personal** workspace — the data layer the switcher UI (Phase 26) builds on.
**Verified:** 2026-07-05T18:45:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Roadmap Success Criteria

| # | Success Criterion | Status | Evidence |
| --- | --- | --- | --- |
| SC1 | A `workspaces` table and a `projects.workspace_id` FK exist (migration 00012), enforced NOT NULL so a project can never be workspace-less | ✓ VERIFIED | `00012_workspaces.sql:16-31` — `CREATE TABLE workspaces` + `ALTER TABLE projects ADD COLUMN workspace_id INTEGER NOT NULL DEFAULT 1 REFERENCES workspaces(id) ON DELETE RESTRICT`. `TestWorkspacesMigration` asserts an explicit `workspace_id = NULL` insert errors (NOT NULL DB-enforced). Migration applied by real goose runner (version 12). |
| SC2 | On first startup after upgrade, a default **Personal** workspace is created and every pre-existing project is assigned to it — all projects remain intact and accessible | ✓ VERIFIED | `TestWorkspacesMigration` stages the DB at 00011, seeds 2 projects + 2 tasks, applies 00012, then asserts every pre-seeded project has `workspace_id == 1`, task count unchanged (no cascade wipe), exactly one `is_default=1` row with `id==1`, and idempotent re-run leaves one workspace. Test PASSES. |

### Observable Truths (merged: roadmap SCs + PLAN must_haves)

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | `workspaces` table exists with an `is_default` flag; exactly one row = 1 after migration | ✓ VERIFIED | `00012_workspaces.sql:16-27`; test asserts `COUNT(*) WHERE is_default=1 == 1` |
| 2 | `projects.workspace_id` is a NOT NULL FK → workspaces(id) ON DELETE RESTRICT, DB-enforced from 00012 | ✓ VERIFIED | `00012_workspaces.sql:30-31`; test (7) NULL insert errors, test (8) `DELETE workspaces WHERE id=1` errors (RESTRICT) |
| 3 | After migration, Personal has is_default=1 and id=1 | ✓ VERIFIED | `INSERT INTO workspaces (name, is_default) VALUES ('Personal', 1)` first into empty PK table; test asserts `id==1` |
| 4 | Every pre-existing project assigned workspace_id=1; all pre-existing tasks survive intact | ✓ VERIFIED | `TestWorkspacesMigration` (a)+(b): every seeded project `workspace_id==1`, `COUNT(tasks)` unchanged |
| 5 | `foreign_keys` re-armed ON after migration (not left disabled on pooled connection) | ✓ VERIFIED | `PRAGMA foreign_keys = ON` at end of Up (line 36); test (9) asserts `PRAGMA foreign_keys == 1` on same handle |
| 6 | Re-running the migration is a clean no-op (no second Personal) | ✓ VERIFIED | test (10) second `Migrate` → `COUNT(workspaces)==1`; goose log "no migrations to run" |
| 7 | GET /api/projects returns `workspace_id` on every project | ✓ VERIFIED | `projects.go:97` const, `:112` scanProject, `:51` struct field `json:"workspace_id"`; `TestProjectsWorkspaceWire` PASSES |
| 8 | POST create (folder path) resolves default workspace; created project carries workspace_id=1 | ✓ VERIFIED | `projects.go:197-204` `defaultWorkspaceID()` + INSERT with `wsID`; `TestProjectsCreateAssignsDefaultWorkspace/folder` PASSES |
| 9 | POST create-by-repo path resolves default workspace; created project carries workspace_id=1 | ✓ VERIFIED | `projects.go:309-316`; `TestProjectsCreateAssignsDefaultWorkspace/repo` PASSES |
| 10 | NOT NULL workspace_id invariant holds for newly-created projects (never workspace-less; 500 on missing default) | ✓ VERIFIED | `defaultWorkspaceID()` error → `writeError(500)`; `TestProjectsCreateRejectsWhenNoDefaultWorkspace` PASSES |
| 11 | Startup `BackfillWorkspaces` guarantees a default workspace row exists, wired after Migrate + BackfillProjectIcons | ✓ VERIFIED | `workspaces.go:25-37`; `main.go:134-137` calls `api.BackfillWorkspaces(db)` after `BackfillProjectIcons` (line 124) which is after `store.Migrate` (line 98) |
| 12 | `BackfillWorkspaces` idempotent — no-op on healthy boot, never a second Personal | ✓ VERIFIED | `TestBackfillWorkspaces` (1)+(3): healthy no-op and double-run keep `COUNT==1` |
| 13 | If default missing, `BackfillWorkspaces` re-creates Personal (is_default=1) | ✓ VERIFIED | `workspaces.go:31-36` errors.Is(sql.ErrNoRows) → INSERT; `TestBackfillWorkspaces` (2) re-create verified |

**Score:** 13/13 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `internal/store/migrations/00012_workspaces.sql` | workspaces table + NOCASE unique index + Personal row + workspace_id NOT NULL RESTRICT FK under NO TRANSACTION + FK-toggle | ✓ VERIFIED | 47 lines; `NO TRANSACTION`×1, `foreign_keys = ON`×2 (Up+Down), `COLLATE NOCASE`×1, `ON DELETE RESTRICT`, `ON DELETE CASCADE`×0, `DROP TABLE projects`×0. Applied by goose (version 12). |
| `internal/store/workspaces_migration_test.go` | staged-upgrade invariant test | ✓ VERIFIED | `TestWorkspacesMigration` — real goose `UpTo(11)` → seed → `Migrate` → 10 staged/structural assertions. PASSES. |
| `internal/api/projects.go` | WorkspaceID threaded (struct/const/scan) + default-resolve+set in both INSERTs | ✓ VERIFIED | `WorkspaceID int64` (line 51), const slot `icon_color, workspace_id, created_at` (line 97), Scan `&p.WorkspaceID` (line 112), `defaultWorkspaceID()` (line 217) called at 197 & 309 |
| `internal/api/projects_test.go` | wire + default-assign tests | ✓ VERIFIED | `TestProjectsWorkspaceWire`, `TestProjectsCreateAssignsDefaultWorkspace`, `TestProjectsCreateRejectsWhenNoDefaultWorkspace` all PASS |
| `internal/api/workspaces.go` | `BackfillWorkspaces(db *sql.DB) error` idempotent guard | ✓ VERIFIED | Lean guard, no SELECT→UPDATE loop, parameterless/literal SQL |
| `internal/api/workspaces_test.go` | no-op / re-create / idempotent test | ✓ VERIFIED | `TestBackfillWorkspaces` PASSES |
| `cmd/kamacu/main.go` | `api.BackfillWorkspaces(db)` wired after BackfillProjectIcons | ✓ VERIFIED | Line 134, after line 124 (icons) which is after line 98 (Migrate); slog.Error + os.Exit(1) guard |

### Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| `internal/store/migrate.go` | `migrations/00012_workspaces.sql` | `//go:embed migrations/*.sql` + `goose.Up` | ✓ WIRED | `migrate.go:10-20`; goose log shows `OK 00012_workspaces.sql`, version 12 |
| `00012` ADD COLUMN DEFAULT 1 | workspaces row 1 (Personal) | Personal inserted before ADD COLUMN so DEFAULT 1 targets a valid parent | ✓ WIRED | `INSERT` line 27 precedes `ALTER TABLE` line 30; FK check passes |
| `projectColumns` const | `scanProject` Scan | workspace_id in same slot (after icon_color, before created_at) in both | ✓ WIRED | const line 97 + Scan line 112 aligned; `go build` (arg-count check) exit 0 |
| `projects.go` create()/createByRepo() | workspaces is_default=1 row | `SELECT id FROM workspaces WHERE is_default = 1` before each INSERT | ✓ WIRED | `defaultWorkspaceID()` line 219, called at 197 & 309 |
| `cmd/kamacu/main.go` startup | `api.BackfillWorkspaces(db)` | call after Migrate + BackfillProjectIcons | ✓ WIRED | main.go:134 |
| `BackfillWorkspaces` | workspaces is_default=1 row | SELECT default → INSERT Personal if missing | ✓ WIRED | workspaces.go:27-36 |

### Behavioral Spot-Checks / Probe Execution

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| Build compiles (scan-arg alignment) | `go build ./...` | exit 0 | ✓ PASS |
| Static analysis clean | `go vet ./internal/store/... ./internal/api/... ./cmd/kamacu/` | exit 0 | ✓ PASS |
| **Upgrade path** (existing install → every project under Personal, tasks intact, idempotent, NOT NULL/RESTRICT bite, FK re-armed) | `go test ./internal/store/ -run TestWorkspacesMigration -v` | PASS (real goose 11→12→no-op) | ✓ PASS |
| Store package regression | `go test ./internal/store/...` | ok | ✓ PASS |
| Read wire carries workspace_id | `go test ./internal/api/ -run 'TestProjectList\|TestProjectsWorkspaceWire' -v` | PASS | ✓ PASS |
| Both create paths assign default; 500 on missing default | `go test ./internal/api/ -run 'TestProjectsCreateAssignsDefaultWorkspace\|TestProjectsCreateRejectsWhenNoDefaultWorkspace' -v` | PASS (folder + repo subtests) | ✓ PASS |
| Startup backfill idempotent guard | `go test ./internal/api/ -run TestBackfillWorkspaces -v` | PASS | ✓ PASS |

The upgrade-path check requested in the task note is fully covered by `TestWorkspacesMigration`, which exercises the real "existing install → upgrade" sequence against the pinned goose runner: it proves every pre-existing project lands under Personal (workspace_id=1), all tasks survive (no cascade wipe), the re-run is a clean no-op, no project is left workspace-less (NOT NULL enforced), and FK enforcement is re-armed on the shared pooled connection.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| --- | --- | --- | --- | --- |
| WSDATA-01 | 25-01, 25-02 | Every project belongs to exactly one workspace — `workspaces` table + `projects.workspace_id` FK (migration 00012), enforced never workspace-less | ✓ SATISFIED | Truths 1,2,7-10; NOT NULL FK DB-enforced + surfaced on read wire + set on both create paths |
| WSDATA-02 | 25-01, 25-03 | First-startup idempotent backfill creates default Personal + assigns every pre-existing project (mirrors BackfillProjectIcons; safe to re-run) | ✓ SATISFIED | Truths 3,4,6,11-13; in-SQL assignment via ADD COLUMN DEFAULT 1 + idempotent BackfillWorkspaces startup guard wired in main.go |

Both phase requirement IDs are accounted for across the plans; no orphaned requirements (REQUIREMENTS.md line 87 confirms Phase 25 owns exactly 2). Note: the REQUIREMENTS.md checkboxes and traceability table still read "Pending"/unchecked — a documentation status update, not an implementation gap.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| --- | --- | --- | --- | --- |
| (phase-modified files) | — | TBD/FIXME/XXX debt markers | ℹ️ none | Scan clean across all 6 modified files |
| (phase-modified files) | — | TODO/HACK/PLACEHOLDER/stub returns | ℹ️ none | Scan clean; no stub implementations |

### Code Review Findings (from 25-REVIEW.md — factored into goal-risk)

The standard code review reported 0 critical, 4 warnings, 1 info. Each was assessed against the Phase 25 goal:

| ID | Finding | Reachable in Phase 25? | Goal impact |
| --- | --- | --- | --- |
| WR-01 | Repo-first create orphans cloned dir when default workspace missing (resolve happens after clone, no RemoveAll on error) | No — Phase 25 has no API to remove/clear the default; the resolve only fails in a state Phase 26 can create | ℹ️ Latent cleanliness gap; does not break "every project belongs to a workspace" |
| WR-02 | `BackfillWorkspaces` recovery INSERT can crash boot via NOCASE UNIQUE if a non-default `Personal` lingers | No — requires Phase 26 default-flag movement to reach a non-default `Personal` row | ⚠️ Latent boot-robustness gap for Phase 26; not reachable now |
| WR-03 | No DB enforcement of "at most one default" (`is_default` has no partial unique index) | No — migration creates exactly one is_default=1; Phase 25 cannot create a second | ⚠️ Hardening suggestion for Phase 26's default-switching |
| WR-04 | Migration 00012 is `NO TRANSACTION`; crash between COMMIT and goose version-record + no `IF NOT EXISTS` could brick re-run | Edge — narrow crash window; NO TRANSACTION is architecturally required (SQLite can't ADD a non-NULL REFERENCES column with FK on inside a txn) | ⚠️ Latent robustness gap; the two success criteria still hold and are tested |
| IN-01 | Missing-default create returns raw `sql: no rows` string to client | Yes (only via the no-default 500 path) | ℹ️ Cosmetic; localhost single-user |

None of these break the two roadmap success criteria or any of the 13 must-have truths — all are latent states not reachable through the Phase 25 surface, and all are naturally in scope for Phase 26 (workspace management/switching) where the default flag first becomes mutable. They are surfaced here as WARNING/INFO for the developer's awareness so Phase 26 planning can absorb WR-02/WR-03/WR-04 as hardening tasks. They do NOT block phase progression.

### Human Verification Required

None. This is backend-only work (SQL migration + Go startup hook + API wiring) with comprehensive automated coverage. The one behavior that would normally warrant human testing — the real existing-install upgrade path — is fully exercised programmatically by `TestWorkspacesMigration` against the actual goose runner (stage at 00011 → seed → apply 00012 → assert survival/assignment/idempotency). No visual, real-time, or external-service behavior is involved.

### Gaps Summary

No gaps. Both roadmap success criteria and all 13 merged must-have truths are verified against the codebase with passing automated tests. The migration creates the `workspaces` table and the DB-enforced `NOT NULL ... ON DELETE RESTRICT` `workspace_id` FK, assigns every pre-existing project to Personal (id=1) in-SQL with all tasks intact, re-arms foreign keys, and is idempotent on re-run. The projects API surfaces `workspace_id` on the read wire and both create paths resolve+set the default. The `BackfillWorkspaces` startup guard is wired after `Migrate`/`BackfillProjectIcons` and is idempotent. Requirement IDs WSDATA-01 and WSDATA-02 are both satisfied. The four code-review warnings are latent, not reachable in Phase 25, and appropriately deferred to Phase 26 hardening.

---

_Verified: 2026-07-05T18:45:00Z_
_Verifier: Claude (gsd-verifier)_
