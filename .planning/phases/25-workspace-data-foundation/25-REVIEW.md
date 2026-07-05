---
phase: 25-workspace-data-foundation
reviewed: 2026-07-05T16:40:17Z
depth: standard
files_reviewed: 7
files_reviewed_list:
  - cmd/kamacu/main.go
  - internal/api/projects.go
  - internal/api/projects_test.go
  - internal/api/workspaces.go
  - internal/api/workspaces_test.go
  - internal/store/migrations/00012_workspaces.sql
  - internal/store/workspaces_migration_test.go
findings:
  critical: 0
  warning: 4
  info: 1
  total: 5
status: issues_found
---

# Phase 25: Code Review Report

**Reviewed:** 2026-07-05T16:40:17Z
**Depth:** standard
**Files Reviewed:** 7
**Status:** issues_found

## Summary

Phase 25 adds a `workspaces` table (migration 00012), a `NOT NULL` `workspace_id` FK
on `projects`, wires the column through `scanProject` / `projectColumns` and both
create paths, adds `BackfillWorkspaces` as a startup safety-net, and calls it from
`main`. The column plumbing (JSON tag, `projectColumns`, scan order, both INSERT
placeholder/arg alignments) is correct and well-tested; the migration test exercises
the real goose runner and proves data survival, `NOT NULL`, `RESTRICT`, FK re-arm, and
idempotency. No SQL injection: every query is parameterized or literal, and `update`'s
dynamic `SET` list is built from a fixed column whitelist.

The defects below cluster around two seams the phase introduced: (1) the `is_default`
invariant is enforced only by convention, and its recovery path can itself brick the
boot; (2) the newly-added `defaultWorkspaceID()` call sits *after* the clone in the
repo-first create path, breaking the documented clone atomicity contract; and (3)
migration 00012 is the codebase's first `NO TRANSACTION` migration, which forfeits the
re-run safety every prior migration enjoys. None are reachable through the Phase-25 API
during a single healthy run, so all are latent — but each is a real correctness gap in
a "data foundation" phase whose whole job is to make later phases safe.

## Warnings

### WR-01: Repo-first create orphans the freshly-cloned directory when the default workspace is missing

**File:** `internal/api/projects.go:305-320` (introduced at 309-313)
**Issue:** `createByRepo` resolves the default workspace at step 7 — *after* the clone
at step 6. The clone-failure path (step 6) carefully `os.RemoveAll(dest)`s to honor the
documented atomicity invariant ("a failed clone leaves no row and no dir",
CKOUT-04/D-01). The new `defaultWorkspaceID()` call is inserted between the successful
clone and the INSERT, and its error path just `writeError(500)` + `return` with **no
`os.RemoveAll(dest)`**. So when there is no `is_default = 1` row, a repo-first create
performs a real network+disk clone into `~/.kamacu/repos/<owner>/<name>` and then leaves
that directory orphaned with no project row — exactly the half-created state the ordering
was designed to prevent. (The pre-existing INSERT-failure path shares this gap, but this
phase widens it with a new, earlier failure point that has zero external side effects and
therefore has no reason to run after the clone.)
**Fix:** Resolve the workspace before any disk side effect — it is a cheap, side-effect-free
DB read that belongs in the validate-before-clone prefix. Move it above step 6:

```go
// Resolve the default workspace BEFORE cloning so a missing default fails
// fast and never orphans a clone (preserves CKOUT-04/D-01 atomicity).
wsID, err := h.defaultWorkspaceID()
if err != nil {
    writeError(w, http.StatusInternalServerError, err.Error())
    return
}
// ... step 6 clone (only now touch disk) ...
// ... step 7 INSERT using wsID ...
```

Alternatively, keep the position but `_ = os.RemoveAll(dest)` in the error branch when
`!reattached` (mirroring the clone-failure cleanup).

### WR-02: `BackfillWorkspaces` recovery INSERT can crash the boot via the case-insensitive name UNIQUE constraint

**File:** `internal/api/workspaces.go:34-36`
**Issue:** When no `is_default = 1` row exists, the safety-net unconditionally runs
`INSERT INTO workspaces (name, is_default) VALUES ('Personal', 1)`. If a workspace named
`Personal` (any case) still exists but is not flagged default, this INSERT violates
`idx_workspaces_name_nocase` (`UNIQUE ... name COLLATE NOCASE`). The error propagates to
`main.go:134-137`, which calls `os.Exit(1)` — the app refuses to boot. This is a
recoverable state (a default just needs re-flagging), yet the guard whose stated purpose
is "guarantee a default workspace row always exists" bricks startup instead of recovering.
The state is reachable once Phase 26 can move/clear the default flag while a `Personal`
row lingers. The existing test only deletes the row entirely (`workspaces_test.go:65`,
leaving the table empty so the INSERT succeeds) and never exercises this collision, giving
false confidence.
**Fix:** Make recovery idempotent against an existing name — re-flag an existing workspace
as default rather than blindly inserting. For example, prefer an existing `Personal`, else
promote any workspace, else insert:

```go
// Promote an existing workspace to default if one exists (case-insensitive
// 'Personal' preferred), so recovery never collides with the UNIQUE name index.
if _, err := db.Exec(
    `UPDATE workspaces SET is_default = 1
       WHERE id = (SELECT id FROM workspaces
                   ORDER BY (name = 'Personal' COLLATE NOCASE) DESC, id LIMIT 1)`); err != nil {
    return err
}
var n int
if err := db.QueryRow(`SELECT COUNT(*) FROM workspaces WHERE is_default = 1`).Scan(&n); err != nil {
    return err
}
if n == 0 { // table was empty — safe to create Personal
    _, err := db.Exec(`INSERT INTO workspaces (name, is_default) VALUES ('Personal', 1)`)
    return err
}
return nil
```

Add a test that seeds a non-default `Personal` row and asserts the backfill re-flags it
rather than erroring.

### WR-03: Nothing enforces "at most one default workspace"; `defaultWorkspaceID` silently picks an arbitrary row

**File:** `internal/store/migrations/00012_workspaces.sql:16-24`; `internal/api/projects.go:217-221`
**Issue:** The schema enforces uniqueness only on `name`, not on `is_default`. `is_default`
is a plain `INTEGER NOT NULL DEFAULT 0` with no partial unique index. Both consumers of the
flag — `defaultWorkspaceID()` (`projects.go:219`) and `BackfillWorkspaces`
(`workspaces.go:27`) — use `QueryRow(... WHERE is_default = 1)`, which returns whichever
row SQLite yields first and silently ignores extras. If two rows ever carry `is_default = 1`
(a bug Phase 26's default-switching could introduce), new projects would be assigned to a
non-deterministic "default" workspace with no error surfaced. For a "data foundation" phase
this is exactly the invariant that should be defended at the DB layer, not left to
application discipline.
**Fix:** Add a partial unique index so the DB rejects a second default:

```sql
CREATE UNIQUE INDEX idx_workspaces_single_default
  ON workspaces (is_default) WHERE is_default = 1;
```

(Requires care with the toggle sequence Phase 26 uses to move the default — clear then set,
or `UPDATE ... SET is_default = CASE ...` in one statement.) At minimum, have
`defaultWorkspaceID` and the backfill order by `id` for determinism.

### WR-04: Migration 00012 is `NO TRANSACTION` — a crash after COMMIT but before goose records the version permanently bricks boot on re-run

**File:** `internal/store/migrations/00012_workspaces.sql:1,16`
**Issue:** 00012 is the first and only `-- +goose NO TRANSACTION` migration
(00007–00011 run inside goose's default transaction, so the schema change *and* goose's
version-record commit atomically — a re-run is always safe). Here, goose records the
`goose_db_version` row in a separate operation *after* the manual `COMMIT` (line 32). If
the process dies (or the version INSERT fails) in that window, the `workspaces` table and
`workspace_id` column are already committed but 00012 is not marked applied. The next boot
re-runs 00012, and `CREATE TABLE workspaces` (no `IF NOT EXISTS`, line 16) fails with
"table workspaces already exists" → `store.Migrate` errors → `main.go:98-101` `os.Exit(1)`,
and every subsequent boot repeats it. Recovery requires manual DB surgery. The window is
small, but the impact (permanent boot failure) is severe and the phase newly forfeits the
atomicity every other migration has.
**Fix:** Make the DDL re-runnable so a partial application self-heals on the next boot:

```sql
CREATE TABLE IF NOT EXISTS workspaces ( ... );
CREATE UNIQUE INDEX IF NOT EXISTS idx_workspaces_name_nocase ON workspaces (name COLLATE NOCASE);
INSERT INTO workspaces (name, is_default)
  SELECT 'Personal', 1 WHERE NOT EXISTS (SELECT 1 FROM workspaces);
-- ADD COLUMN cannot use IF NOT EXISTS; guard by catching/ignoring the duplicate-column
-- case, or keep it last so a re-run reaches it only after the guarded statements pass.
```

And mirror with `DROP ... IF EXISTS` in the Down block (lines 43-45).

## Info

### IN-01: Missing-default create returns the raw `sql: no rows in result set` string to the client

**File:** `internal/api/projects.go:197-201, 309-313, 217-221`
**Issue:** When `defaultWorkspaceID()` hits `sql.ErrNoRows`, both create paths do
`writeError(w, 500, err.Error())`, surfacing the opaque driver message
`"sql: no rows in result set"` to the API consumer (asserted indirectly by
`TestProjectsCreateRejectsWhenNoDefaultWorkspace`, which only checks the 500 status).
Low impact on a single-user localhost app, but it is an uninformative error for a
condition the code specifically anticipates.
**Fix:** Return a purposeful message, e.g.:

```go
wsID, werr := h.defaultWorkspaceID()
if werr != nil {
    writeError(w, http.StatusInternalServerError, "no default workspace configured")
    return
}
```

---

_Reviewed: 2026-07-05T16:40:17Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
