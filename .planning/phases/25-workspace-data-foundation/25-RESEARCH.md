# Phase 25: Workspace Data Foundation - Research

**Researched:** 2026-07-05
**Domain:** SQLite schema migration (modernc.org/sqlite 3.53.2) + goose + Go startup backfill
**Confidence:** HIGH (the core migration mechanic was verified empirically against the project's own pinned driver — see Code Examples)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Schema — `workspaces` table**
- **D-01:** New `workspaces` table, **minimal columns only**: `id INTEGER PRIMARY KEY`, `name TEXT NOT NULL`, `is_default INTEGER NOT NULL DEFAULT 0`, `created_at`/`updated_at` TEXT (mirror the `projects` `strftime('%Y-%m-%dT%H:%M:%fZ','now')` defaults). No `position`, no icon columns — deferred (WSFUT-01/02); a future `00013` is cheap.
- **D-02:** The protected default workspace is identified by the **`is_default` flag** (exactly one row = 1), **not** by name "Personal" — rename-proof (Phase 26 lets the user rename Personal). Downstream resolves default via `SELECT id FROM workspaces WHERE is_default = 1`.
- **D-03:** `workspaces.name` is **UNIQUE, case-insensitive** — a unique index on `name COLLATE NOCASE` ("Personal" and "personal" collide).

**Schema — `projects` foreign key**
- **D-04:** `projects.workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT`. **RESTRICT** hard-refuses deleting a workspace that still owns projects. Never `ON DELETE CASCADE`. FKs are enforced (`_pragma=foreign_keys(1)`).
- **D-05:** NOT NULL is **DB-enforced from migration `00012` onward** (success criterion 1), not merely app-enforced.

**Migration + bootstrap**
- **D-06:** Migration `00012` performs the whole bootstrap **in SQL** so NOT NULL holds at migration time. The **exact SQLite mechanism** for adding the NOT NULL FK column to the populated table is the research item (resolved below).
- **D-07:** The **Go startup hook** mirrors `api.BackfillProjectIcons`: idempotent, runs **once right after `store.Migrate(db)`** in `cmd/kamacu/main.go`, a cheap no-op on healthy boots. Job = idempotent safety net: guarantee a default workspace exists and no project is workspace-less; never create a second Personal. **Collect-then-update REQUIRED** (single-writer `SetMaxOpenConns(1)`); all SQL parameterized with `?`. *(See Open Question 1 — under the recommended migration the reassignment loop is vestigial.)*
- **D-08:** Migration `Down` reverses cleanly (drop FK/column, drop unique index + table) — model on `00009`'s `DROP COLUMN` Down (modernc 3.53 supports DROP COLUMN).

**Write path & API surface**
- **D-09:** Both create INSERTs in `internal/api/projects.go` (`create` ~line 183, `createByRepo` ~line 276) resolve the default workspace (`WHERE is_default = 1`) and set `workspace_id`. Only write-path change in Phase 25.
- **D-10:** `Project` struct gains `WorkspaceID int64 \`json:"workspace_id"\``, added to `projectColumns` const and `scanProject` Scan order — **watch column order**. Ships on the projects read wire (GET/POST/PATCH RETURNING).
- **D-11:** **No new workspace endpoints in Phase 25.** All workspace CRUD, filtering, transfer, create-in-active-workspace are Phase 26.

### Claude's Discretion
- Migration filename (`00012_workspaces.sql`), the precise `workspace_id` position within `projectColumns`/`scanProject`, the exact idempotency-guard SQL in the Go hook, and whether the hook lives in a new `internal/api/workspaces.go` or beside `icons.go` — follow existing patterns; no user preference expressed.

### Deferred Ideas (OUT OF SCOPE)
- **WSFUT-01** — workspace icon/color identity (reason `workspaces` stays name-only, no icon columns now).
- **WSFUT-02** — reorder workspaces (reason no `position` column; a `00013` adds it when the feature lands).
- **WSFUT-03** — bulk/multi-select project transfer.
- **All Phase 26 surface** — switcher UI, workspace CRUD endpoints, active-workspace filtering/navigation/localStorage, `⋯`-menu transfer, create-in-active-workspace, cross-workspace sessions-bar guardrail. **Do NOT propose any of these.**
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| WSDATA-01 | Every project belongs to exactly one workspace — `workspaces` table + `projects.workspace_id` FK (migration `00012`), enforced so a project can never be workspace-less. | Recommended migration (Approach A) creates the table + a **DB-enforced `NOT NULL … REFERENCES … ON DELETE RESTRICT`** column, verified against modernc 3.53.2: NOT NULL, FK, and RESTRICT all bite (Code Examples §1, probe Q3a-c). |
| WSDATA-02 | On first startup after upgrade, a one-time idempotent backfill creates a default **Personal** workspace and assigns every pre-existing project to it (mirrors `BackfillProjectIcons`; safe to re-run). | Migration itself creates Personal (`is_default=1`) and assigns all existing projects via the `DEFAULT 1` FK column (verified: 2/2 projects → workspace 1, 3/3 tasks intact). The Go hook (`BackfillWorkspaces`) is the idempotent boot-time backstop guaranteeing the default exists. Migration re-run is a clean no-op (goose version-tracking; verified). |
</phase_requirements>

## Summary

This is a **backend/data-layer** phase: one goose migration (`00012_workspaces.sql`), one idempotent Go startup hook, and a small wire/write-path change in `internal/api/projects.go`. Nothing user-visible ships. The entire recommended approach was **verified empirically** by running throwaway probes — including the full proposed migration through the real goose runner — against the project's exact pinned stack (modernc.org/sqlite v1.52.0, SQLite **3.53.2**, goose v3.27.1).

**The one riskiest detail — how to add a `NOT NULL` FK column to the already-populated `projects` table with NOT NULL enforced at the DB from migration time — is resolved.** SQLite **forbids** `ALTER TABLE … ADD COLUMN … REFERENCES … NOT NULL DEFAULT <x>` while `foreign_keys` is ON (verified error: *"Cannot add a REFERENCES column with non-NULL default value"*). And `foreign_keys` can only be toggled **outside** a transaction, while goose runs migrations inside one by default. So the migration must be `-- +goose NO TRANSACTION`, toggle `PRAGMA foreign_keys OFF → work → ON`, and add the column with `NOT NULL DEFAULT 1 REFERENCES workspaces(id) ON DELETE RESTRICT`. The `DEFAULT 1` (Personal is deterministically id 1) makes the ADD COLUMN itself assign every existing project to Personal — no separate UPDATE needed. **A full table rebuild (Approach C) is explicitly rejected**: with FK enforced, `DROP TABLE projects` performs an implicit cascade that **wipes every `tasks` row** (verified: 3 tasks → 0), so the rebuild is strictly more dangerous for zero benefit here.

**Primary recommendation:** Approach A — `-- +goose NO TRANSACTION` migration with `PRAGMA foreign_keys=OFF` + explicit `BEGIN/COMMIT` + `ADD COLUMN workspace_id INTEGER NOT NULL DEFAULT 1 REFERENCES workspaces(id) ON DELETE RESTRICT` + `PRAGMA foreign_keys=ON`. Exact SQL for Up and Down is in Code Examples §1.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| `workspaces` table + `workspace_id` FK + assign-existing | Database / Storage (migration `00012`) | — | Schema and DB-enforced NOT NULL/FK belong in the migration so the invariant holds from `00012` time (D-05/D-06). |
| Boot-time "default workspace exists" invariant | API / Backend (Go startup hook) | Database | Idempotent safety net mirrors `BackfillProjectIcons`, wired after `store.Migrate` (D-07). |
| Default-to-Personal on project create | API / Backend (`projects.go` create paths) | Database | Both INSERTs resolve `WHERE is_default=1` and set `workspace_id` so creation holds under the NOT NULL FK (D-09). |
| `workspace_id` on the projects wire | API / Backend (`Project` struct / `projectColumns` / `scanProject`) | — | Read-side field for the Phase 26 switcher; column threaded through the existing serialization path (D-10). |
| Switcher UI, workspace filtering, transfer | *(Phase 26 — OUT OF SCOPE)* | — | Explicitly deferred (D-11). No browser/frontend tier work in Phase 25. |

## Standard Stack

No new packages are introduced. Everything needed is already vendored and in `go.mod`.

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `modernc.org/sqlite` | v1.52.0 (SQLite 3.53.2) | Pure-Go SQLite driver | Already the project driver; `sqlite_version()` verified = **3.53.2** [VERIFIED: probe Q9]. Supports `DROP COLUMN` and the FK/ALTER semantics this phase relies on. |
| `github.com/pressly/goose/v3` | v3.27.1 | Embedded SQL migrations run at startup | Already the migration runner (`internal/store/migrate.go`). Default = one transaction per migration; `-- +goose NO TRANSACTION` opts out [VERIFIED: goose source `internal/sqlparser/parser.go`, README]. |
| `modernc.org/libc` | v1.72.3 (indirect) | modernc runtime | Do not bump independently (CLAUDE.md: `go mod tidy` resolves it). No change needed this phase. |

**Installation:** none — `go build ./...` already resolves these. [VERIFIED: `go build ./...` clean, `go1.26.0`.]

**Version verification (run at plan time, optional):**
```bash
grep -E 'modernc.org/sqlite|pressly/goose' go.mod   # v1.52.0 / v3.27.1 (confirmed 2026-07-05)
```

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| goose default transaction | `-- +goose NO TRANSACTION` | **Required here** — `PRAGMA foreign_keys` is a no-op inside a transaction, and the ADD COLUMN needs FK OFF. Not optional. |

## Package Legitimacy Audit

**No external packages are installed in this phase.** It uses only already-vendored dependencies (`modernc.org/sqlite` v1.52.0, `github.com/pressly/goose/v3` v3.27.1), both present in `go.mod` and confirmed via the module cache and a clean `go build ./...`. slopcheck / registry verification is **not applicable** — nothing is added to `go.mod`.

## Architecture Patterns

### System Architecture Diagram

```
                         ┌─────────────────────────────────────────────┐
  kamacu binary boot     │  cmd/kamacu/main.go  (startup sequence)      │
  (single process)       └─────────────────────────────────────────────┘
                                            │
        store.Open(dbPath)  ── DSN: _pragma=foreign_keys(1), SetMaxOpenConns(1)
                                            │
                                            ▼
        store.Migrate(db) ──► goose.Up ──► 00012_workspaces.sql  [NO TRANSACTION]
                                            │   PRAGMA foreign_keys=OFF
                                            │   BEGIN
                                            │   CREATE workspaces (+ NOCASE unique idx)
                                            │   INSERT Personal (is_default=1) → id 1
                                            │   ALTER projects ADD workspace_id
                                            │        NOT NULL DEFAULT 1 REFERENCES … RESTRICT
                                            │        └─ assigns EVERY existing project → 1
                                            │   COMMIT ; PRAGMA foreign_keys=ON
                                            ▼
        migrate.Complete(...)  (existing ~/.kangent→~/.kamacu step; unchanged)
                                            │
                                            ▼
        api.BackfillProjectIcons(db)   (existing; unchanged)
                                            │
                                            ▼
        api.BackfillWorkspaces(db)  ◄── NEW hook, idempotent no-op guard
                                            │   SELECT id FROM workspaces WHERE is_default=1
                                            │   (missing → INSERT Personal ; else return nil)
                                            ▼
                        ── HTTP serving ──
   GET  /api/projects  ─► list()        ─► SELECT projectColumns (now incl. workspace_id)
   POST /api/projects  ─► create()      ─► resolve is_default=1 → INSERT … workspace_id
                       └► createByRepo() ─► resolve is_default=1 → INSERT … workspace_id
   PATCH/api/projects/{id} ─► update()  ─► RETURNING projectColumns (workspace_id on wire)
```

### Recommended Project Structure (files touched)
```
internal/store/migrations/00012_workspaces.sql   # NEW — the migration (Approach A)
internal/api/workspaces.go                        # NEW — BackfillWorkspaces hook (or add beside icons.go)
internal/api/projects.go                          # EDIT — Project struct, projectColumns, scanProject, 2 INSERTs
cmd/kamacu/main.go                                # EDIT — one line: api.BackfillWorkspaces(db) after BackfillProjectIcons
```

### Pattern 1: FK-column-add requires FK OFF + NO TRANSACTION
**What:** Adding a `NOT NULL … REFERENCES` column to a populated table.
**When to use:** This phase (D-05/D-06). It is the ONLY way to get DB-enforced NOT NULL + FK on an existing populated table without a table rebuild.
**Rule (verified):** SQLite rejects `ADD COLUMN … REFERENCES … NOT NULL DEFAULT` when `foreign_keys` is ON. `foreign_keys` can only be toggled outside a transaction. Therefore: `-- +goose NO TRANSACTION` + `PRAGMA foreign_keys=OFF` before the ADD, `PRAGMA foreign_keys=ON` after. See Code Examples §1.

### Pattern 2: Existing-row assignment via the column DEFAULT
**What:** `ADD COLUMN workspace_id INTEGER NOT NULL DEFAULT 1 …` sets `workspace_id = 1` for **every existing row** as part of the ALTER — no separate `UPDATE projects SET workspace_id=1` needed.
**Why it works:** Personal is inserted first into an empty `INTEGER PRIMARY KEY` table, so its id is deterministically `1`; the literal `DEFAULT 1` therefore always points at Personal. [VERIFIED: probe Q2 — 2/2 existing projects assigned to workspace 1 by the ADD COLUMN alone.]

### Pattern 3: Idempotent startup hook (mirror `BackfillProjectIcons`)
**What:** `BackfillWorkspaces(db)` wired in `main.go` right after `api.BackfillProjectIcons(db)`. No-op guard + parameterized SQL. See Code Examples §3.

### Anti-Patterns to Avoid
- **Full `projects` table rebuild while FK is enforced (Approach C):** `DROP TABLE projects` with `foreign_keys=ON` fires the `tasks … ON DELETE CASCADE` and **deletes all tasks** [VERIFIED: probe Q7 — 3 tasks → 0]. Only safe with FK OFF, and even then it must faithfully reproduce every `projects` column + the `repo_path TEXT NOT NULL UNIQUE` constraint. Strictly more risk than Approach A for no benefit here. Do not use unless a "no column default" schema is a hard requirement.
- **Trying to add the FK column inside goose's default transaction:** fails — `PRAGMA foreign_keys=OFF` is a no-op in a transaction, so the ADD COLUMN hits the "non-NULL default REFERENCES" rejection.
- **Leaving `foreign_keys` OFF at the end of the migration:** because `SetMaxOpenConns(1)` reuses ONE pooled connection, a migration that forgets the final `PRAGMA foreign_keys = ON` leaves FK enforcement **disabled app-wide** for the rest of the process. Always re-arm. [VERIFIED: probe Q5 — FK state = 1 after the migration body.]
- **`ON DELETE CASCADE` on `workspace_id`:** forbidden by D-04. Use `ON DELETE RESTRICT`.
- **String-concatenating names/ids into SQL:** all workspace/project SQL uses `?` placeholders (D-07, matches existing code).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Migration versioning / "run once" | A custom "has this migration run?" flag | goose version table (`goose.Up`) | Already in place; makes the migration a no-op on re-run (verified — no second Personal on re-run). |
| Existing-row assignment to Personal | A Go loop `UPDATE`-ing every project | The column `DEFAULT 1` on ADD COLUMN | SQLite populates all rows atomically during the ALTER (verified). |
| Case-insensitive uniqueness | Lowercasing names in Go before insert | `CREATE UNIQUE INDEX … (name COLLATE NOCASE)` | DB-level reject; "Personal"/"personal" collide (verified probe Q8). |
| Deadlock-free SELECT→UPDATE under single-writer | Ad-hoc connection juggling | Collect-then-update (scan into slice, close cursor, then UPDATE) | Documented in `BackfillProjectIcons`; the single-writer (`SetMaxOpenConns(1)`) rule. *(Vestigial here — see Open Q1.)* |

**Key insight:** The migration + goose version-tracking + the column DEFAULT together do almost all the work. The Go hook is a thin invariant-guard, not a data-filler (unlike the icon backfill).

## Runtime State Inventory

This is a **schema migration** phase. The canonical question — *after every file in the repo is updated, what runtime systems still have old state?* — answered per category:

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| **Stored data** | The live SQLite DB (`~/.kamacu/kamacu.db`): existing `projects` rows gain `workspace_id=1`; a new `workspaces` row (Personal, id 1); a new `goose_db_version` row (version 12). All applied **by the migration itself** on first upgraded boot. | None beyond shipping `00012` — the migration is the data migration. No external datastore holds project/workspace state. |
| **Live service config** | None — Kamacu is a single local process with SQLite only; no n8n/Datadog/Tailscale/Cloudflare-style external config. | None. |
| **OS-registered state** | None — no Task Scheduler / pm2 / systemd / launchd registrations embed workspace identifiers. (tmux sockets/sessions are per-task, unrelated to workspaces.) | None. |
| **Secrets / env vars** | None — no secret key or env var references a workspace. | None. |
| **Build artifacts / embed** | The migrations are compiled into the binary via `//go:embed migrations/*.sql` (verified glob). The new `00012_workspaces.sql` is picked up automatically at build time. A **rebuilt binary is required** for the migration to ship; a running pre-Phase-25 binary will not have `00012` until rebuilt+restarted. | Rebuild the binary (`make build` / `go build`) — standard for this Go+embed app. No stale egg-info/compiled-artifact concerns. |

**Verified:** migrations embed glob is `//go:embed migrations/*.sql` (`internal/store/migrate.go:10`); `*.sql` matches `00012_workspaces.sql`.

## Common Pitfalls

### Pitfall 1: FK enforced → "Cannot add a REFERENCES column with non-NULL default value"
**What goes wrong:** Writing the migration in goose's default (transactional) mode, or without `PRAGMA foreign_keys=OFF`, makes the ADD COLUMN fail with `SQL logic error: Cannot add a REFERENCES column with non-NULL default value (1)`.
**Why:** SQLite rule — a `REFERENCES` column added via ADD COLUMN must default to NULL **when FK is enabled**; but NOT NULL needs a non-NULL default. The two requirements only coexist with FK OFF.
**How to avoid:** `-- +goose NO TRANSACTION` + `PRAGMA foreign_keys=OFF` before the ADD COLUMN. [VERIFIED: probe Q1 (fails FK-on) vs Q2 (succeeds FK-off).]
**Warning sign:** migration error mentioning "REFERENCES column with non-NULL default."

### Pitfall 2: Table rebuild wipes `tasks` via cascade
**What goes wrong:** Choosing the "clean rebuild" (CREATE new table, copy, DROP old, RENAME) with FK enforced silently **deletes every task** — `DROP TABLE projects` performs an implicit DELETE that fires `tasks.project_id … ON DELETE CASCADE`.
**Why:** With `foreign_keys=ON`, DROP TABLE removes rows before dropping the table, triggering FK actions.
**How to avoid:** Don't rebuild (use Approach A). If a rebuild is ever unavoidable, it MUST run with FK OFF and end with `PRAGMA foreign_key_check`. [VERIFIED: probe Q7 — 3 tasks → 0 on `DROP TABLE projects` with FK ON.]
**Warning sign:** post-migration task count drops to zero.

### Pitfall 3: Leaving the pooled connection with FK disabled
**What goes wrong:** A `NO TRANSACTION` migration that omits the trailing `PRAGMA foreign_keys=ON` leaves FK enforcement OFF for the rest of the process (one shared connection under `SetMaxOpenConns(1)`), silently disabling all RESTRICT/CASCADE guards.
**How to avoid:** Always end both Up and Down with `PRAGMA foreign_keys = ON`. [VERIFIED: probe Q5 restores FK=1.]
**Warning sign:** FK constraints not firing after boot (e.g., a workspace delete that should RESTRICT succeeds).

### Pitfall 4: Wire column-order mismatch (D-10)
**What goes wrong:** Adding `workspace_id` to `projectColumns` but not to `scanProject` (or in a different position) → `Scan` maps the wrong column into the wrong field, or a "expected N destination arguments" error.
**How to avoid:** Insert `workspace_id` at the **same position** in the `projectColumns` const AND the `scanProject` `Scan(...)` call — recommend right after `icon_color`, before `created_at` (exactly how `icon_letters`/`icon_color` sit before the timestamps). See Code Examples §2.
**Warning sign:** `go build` scan-arg errors, or `workspace_id` serializing a timestamp value.

### Pitfall 5: `PRAGMA foreign_key_check` via goose is non-gating
**What goes wrong:** Assuming `PRAGMA foreign_key_check;` in the migration will abort on a violation. goose runs statements via `Exec`, which ignores returned rows — `foreign_key_check` reports violations as **result rows, not an error**, so it cannot fail the migration.
**How to avoid:** Don't rely on it as a gate. The real safety is inserting Personal (id 1) **before** the ADD COLUMN so every row references a valid parent. Keep `foreign_key_check` only as documentation/parity with the official 12-step; it's harmless but non-gating here.

## Code Examples

### §1 — RECOMMENDED migration `internal/store/migrations/00012_workspaces.sql` (Approach A)

Verified end-to-end through the real goose runner against a seeded projects/tasks DB (2 projects, 3 tasks): after Up → FK=ON, both projects assigned to Personal, all 3 tasks intact, one workspace, `is_default` id=1; re-run Up → clean no-op (no second Personal); Down → column gone, table gone, FK restored, tasks intact. [VERIFIED: goose probe.]

```sql
-- +goose NO TRANSACTION
-- +goose Up
-- Phase 25 (WSDATA-01/02): add the workspaces table + a DB-enforced NOT NULL
-- workspace_id FK on projects, and assign every existing project to the default
-- Personal workspace -- all in one migration so NOT NULL holds from 00012 on.
--
-- WHY NO TRANSACTION + PRAGMA foreign_keys=OFF:
--   SQLite rejects `ALTER TABLE ADD COLUMN ... REFERENCES ... NOT NULL DEFAULT`
--   while foreign_keys is ON ("Cannot add a REFERENCES column with non-NULL
--   default value"). foreign_keys can only be toggled OUTSIDE a transaction, and
--   goose runs migrations in a transaction by default -- hence NO TRANSACTION and
--   an explicit BEGIN/COMMIT. store.go opens with SetMaxOpenConns(1), so this same
--   pooled connection MUST be re-armed with foreign_keys=ON at the end.
PRAGMA foreign_keys = OFF;
BEGIN;
CREATE TABLE workspaces (
  id         INTEGER PRIMARY KEY,
  name       TEXT    NOT NULL,
  is_default INTEGER NOT NULL DEFAULT 0,
  created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
-- D-03: case-insensitive unique name ('Personal' and 'personal' collide).
CREATE UNIQUE INDEX idx_workspaces_name_nocase ON workspaces (name COLLATE NOCASE);
-- D-02: the protected default. First row into an empty INTEGER PRIMARY KEY table
-- => id = 1 (deterministic), which the DEFAULT below points at.
INSERT INTO workspaces (name, is_default) VALUES ('Personal', 1);
-- D-04/D-05: NOT NULL FK. DEFAULT 1 assigns EVERY existing project to Personal as
-- part of the ADD COLUMN (no separate UPDATE needed). ON DELETE RESTRICT.
ALTER TABLE projects
  ADD COLUMN workspace_id INTEGER NOT NULL DEFAULT 1 REFERENCES workspaces(id) ON DELETE RESTRICT;
COMMIT;
-- Non-gating parity with the official 12-step (goose Exec ignores its rows); the
-- real guarantee is that Personal(1) was inserted before the ADD COLUMN.
PRAGMA foreign_key_check;
PRAGMA foreign_keys = ON;

-- +goose Down
-- NO TRANSACTION applies to Down too; same FK-toggle discipline. modernc 3.53
-- supports DROP COLUMN directly (as 00007/00008/00009).
PRAGMA foreign_keys = OFF;
BEGIN;
ALTER TABLE projects DROP COLUMN workspace_id;
DROP INDEX idx_workspaces_name_nocase;
DROP TABLE workspaces;
COMMIT;
PRAGMA foreign_keys = ON;
```

**Notes for the planner:**
- `-- +goose NO TRANSACTION` MUST be at the top of the file; it governs **both** Up and Down [CITED: goose v3.27.1 README].
- Statements are semicolon-delimited; no `-- +goose StatementBegin/End` is needed (no embedded semicolons) — consistent with every existing migration.
- The explicit `BEGIN/COMMIT` gives all-or-nothing DDL: on any statement failure the transaction rolls back, goose does not record version 12, and the next boot re-runs cleanly (the app `os.Exit(1)`s on migration error, and the next `store.Open` gives a fresh FK-on connection).
- **Optional hardening** (if extra re-run resilience is wanted without the manual transaction): drop `BEGIN/COMMIT` and use `CREATE TABLE IF NOT EXISTS` / `CREATE UNIQUE INDEX IF NOT EXISTS` / `INSERT … WHERE NOT EXISTS (SELECT 1 FROM workspaces WHERE is_default=1)`, keeping the ADD COLUMN last. The recommended (transactional) form was the one verified end-to-end.

### §2 — Wire change in `internal/api/projects.go` (D-10)

Insert `workspace_id` at the **same position** in the const and the Scan (after `icon_color`, before the timestamps):

```go
// struct: add after IconColor, before CreatedAt
type Project struct {
	// ... existing fields ...
	IconLetters string `json:"icon_letters"`
	IconColor   string `json:"icon_color"`
	WorkspaceID int64  `json:"workspace_id"` // NEW (D-10): NOT NULL FK, always present
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// const: add workspace_id in the same slot
const projectColumns = `id, name, repo_path, description, github_repo, managed, icon_letters, icon_color, workspace_id, created_at, updated_at`

// scanProject: add &p.WorkspaceID in the same slot
err := row.Scan(&p.ID, &p.Name, &p.RepoPath, &p.Description, &repo, &managedInt,
	&p.IconLetters, &p.IconColor, &p.WorkspaceID, &p.CreatedAt, &p.UpdatedAt)
```
`workspace_id` is `INTEGER NOT NULL` → scan straight into `int64` (no `sql.NullInt64`; never null on the wire). Column order in `projectColumns` is a curated logical list (independent of physical column order — the ADD COLUMN appends `workspace_id` physically last, which does not matter for an explicit column list).

### §2b — Create-path change in `internal/api/projects.go` (D-09)

Both INSERT sites resolve the default and set `workspace_id`. The column `DEFAULT 1` is a backstop, but D-09 requires the explicit resolve+set:

```go
// Resolve the default workspace once, before each INSERT (D-02/D-09).
var wsID int64
if err := h.db.QueryRow(`SELECT id FROM workspaces WHERE is_default = 1`).Scan(&wsID); err != nil {
	writeError(w, http.StatusInternalServerError, err.Error())
	return
}
// create() folder path (~line 183): add workspace_id column + value
//   INSERT INTO projects (name, repo_path, icon_letters, icon_color, workspace_id)
//   VALUES (?, ?, ?, ?, ?) RETURNING `+projectColumns
//   args: name, abs, deriveLetters(name), pickColor(), wsID
//
// createByRepo() (~line 276): add workspace_id column + value
//   INSERT INTO projects (name, repo_path, github_repo, managed, description, icon_letters, icon_color, workspace_id)
//   VALUES (?, ?, ?, 1, ?, ?, ?, ?) RETURNING `+projectColumns
//   args: name, dest, canonical, desc, deriveLetters(name), pickColor(), wsID
```

### §3 — Idempotent startup hook `internal/api/workspaces.go` (D-07)

```go
// BackfillWorkspaces guarantees the WSDATA-02 invariants at startup, mirroring
// api.BackfillProjectIcons: it runs ONCE right after store.Migrate(db) and is
// IDEMPOTENT -- a cheap no-op on healthy boots. Unlike the icon backfill (which
// FILLS blank columns), migration 00012 already creates Personal AND assigns
// every project to it via the NOT NULL DEFAULT 1 FK column; so this hook's active
// job is only the safety net: guarantee a default workspace row exists (WSMGMT-04:
// at least one workspace always exists). Under NOT NULL + FK + DEFAULT no project
// can be workspace-less, so there is nothing to reassign -- see Open Question 1.
// All SQL is parameterized.
func BackfillWorkspaces(db *sql.DB) error {
	var id int64
	err := db.QueryRow(`SELECT id FROM workspaces WHERE is_default = 1`).Scan(&id)
	if err == nil {
		return nil // default already exists -> no-op (the healthy-boot path)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	// No default workspace: recreate Personal (defensive; 00012 normally created it).
	_, err = db.Exec(`INSERT INTO workspaces (name, is_default) VALUES ('Personal', 1)`)
	return err
}
```

Wire it in `cmd/kamacu/main.go` immediately after the existing icon backfill (verified location: `main.go:124`, `api.BackfillProjectIcons(db)`):

```go
if err := api.BackfillProjectIcons(db); err != nil {   // existing (main.go ~124)
	slog.Error("backfilling project icons", "error", err)
	os.Exit(1)
}
// NEW: guarantee a default workspace exists (WSDATA-02 / D-07). Ordering is
// load-bearing -- MUST run after store.Migrate (the workspaces table must exist).
if err := api.BackfillWorkspaces(db); err != nil {
	slog.Error("backfilling workspaces", "error", err)
	os.Exit(1)
}
```

### §4 — The single-writer collect-then-update discipline (quoted, for reference)

From `internal/api/icons.go` (`BackfillProjectIcons` doc comment) — the rule D-07 references:

> *"Collect-then-update is REQUIRED: the store opens with db.SetMaxOpenConns(1) (single-writer discipline, store.go), so holding the SELECT cursor open while issuing UPDATEs on the same connection would deadlock. We scan every row that needs filling into a slice ... close the cursor, then run the parameterized UPDATEs. All SQL uses '?' placeholders only."*

**Applicability note:** this deadlock rule matters whenever a hook does SELECT-then-UPDATE on the projects table. The recommended `BackfillWorkspaces` does not update projects at all (the migration's `DEFAULT 1` already assigned them), so the deadlock cannot arise. If the team chooses to add a defensive reassignment loop to literally honor D-07's "collect-then-update," it must follow this exact scan-into-slice-then-update pattern — but that loop is provably empty under Approach A (see Open Question 1).

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| 12-step table rebuild for "add NOT NULL FK" | `ADD COLUMN … NOT NULL DEFAULT … REFERENCES` under FK OFF | SQLite ≥ 3.25 stabilized ALTER; `DROP COLUMN` added in 3.35 | The rebuild is only needed for constraints ADD COLUMN can't express; adding a NOT NULL FK column with a constant default does NOT require a rebuild. |
| Backfill via Go loop (like icons) | Backfill via column `DEFAULT` in the ALTER | This phase | Existing rows are assigned atomically by the migration; the Go hook shrinks to an invariant-guard. |

**Deprecated/outdated:** none relevant. `DROP COLUMN` is fully supported by modernc 3.53.2 (used by `00007`/`00008`/`00009` and re-verified here).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Keeping the permanent `DEFAULT 1` on `projects.workspace_id` is acceptable (it cannot be dropped without a rebuild; it's a NOT-NULL backstop pointing at the never-deletable Personal=1). | Approach A / Code Examples §1 | If the team requires a "no column default" schema, use Approach C (rebuild) and accept its cascade risk + full-schema-reproduction cost. Low risk — the default only fires when `workspace_id` is omitted, which app code never does after D-09. |
| A2 | Under Approach A the Go hook's "collect-then-update project reassignment" (D-07) is vestigial and can be omitted; the hook only ensures the default workspace exists. | §3 / Open Q1 | If a reviewer insists on literally honoring D-07's collect-then-update, add a provably-empty defensive loop. Behaviorally identical; no data risk. **Flag for discuss-phase.** |
| A3 | Personal is deterministically `id = 1` (first INSERT into an empty `INTEGER PRIMARY KEY` table) and stays 1 (WSMGMT-04: never deletable), so `DEFAULT 1` is correct forever. | Pattern 2 | If a future migration deletes+recreates Personal with a different id, the literal default would drift. Guarded by WSMGMT-04. Very low risk. |

## Open Questions

1. **D-07 says "collect-then-update is REQUIRED"; under Approach A there is nothing to update.**
   - What we know: WSDATA-02 / D-07 model the hook on `BackfillProjectIcons`, which *fills* blank columns via SELECT→UPDATE. But Approach A satisfies NOT NULL *at migration time* by assigning every existing project via the column `DEFAULT 1`. With `NOT NULL + FK + DEFAULT`, no project can ever be workspace-less, so the reassignment loop finds zero rows.
   - What's unclear: whether the planner should (a) implement the lean "ensure default exists" hook (recommended, Code Examples §3) and treat the reassignment loop as vestigial, or (b) include a defensive-but-provably-empty collect-then-update loop for literal symmetry with D-07.
   - Recommendation: (a). The hook's *stated job* in D-07 ("guarantee a default workspace exists and no project is workspace-less") is fully met — invariant 1 by the hook, invariant 2 structurally by the schema. Note this reinterpretation of D-07 explicitly in the plan; optionally confirm in discuss-phase. This is the only place the recommended approach nuances a locked decision.

2. **`PRAGMA foreign_key_check` is non-gating under goose.** Keep it for parity/documentation or drop it? Recommendation: keep it (harmless, signals intent), but the plan/verification must not treat it as a safety gate — safety comes from inserting Personal before the ADD COLUMN.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | build | ✓ | go1.26.0 | — |
| `modernc.org/sqlite` | migration + driver | ✓ | v1.52.0 (SQLite 3.53.2) | — |
| `github.com/pressly/goose/v3` | migration runner | ✓ | v3.27.1 | — |
| `go build ./...` | binary | ✓ | clean | — |

**Missing dependencies with no fallback:** none. This is a code + embedded-SQL phase with no external services, ports, or CLIs (SQLite is in-process; no `sqlite3` CLI needed).

## Security Domain

> `security_enforcement` is absent from `.planning/config.json` (treated as enabled). This is a **local, single-user, loopback-only** data-layer phase; most ASVS categories are structurally N/A.

### Applicable ASVS Categories
| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V5 Input Validation / Injection | yes | Parameterized `?` SQL everywhere (D-07 + existing `projects.go` pattern). Workspace name/ids are never string-concatenated. The migration inserts a literal `'Personal'` (no user input). |
| V6 Cryptography | no | No secrets/crypto in this phase. |
| V2/V3/V4 AuthN/Session/Access Control | no | Single-user localhost app; no auth surface added. Phase 25 adds no endpoints (D-11). |

### Known Threat Patterns for this stack
| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| SQL injection via project/workspace name | Tampering | Parameterized queries (`?`) — enforced by D-07 and existing code; the migration uses only literals. |
| FK-enforcement silently disabled after a NO TRANSACTION migration | Tampering / repudiation | Mandatory trailing `PRAGMA foreign_keys = ON` on both Up and Down (Pitfall 3; verified). |

## Sources

### Primary (HIGH confidence — empirically verified this session)
- **Live probes against the project's pinned driver** (modernc.org/sqlite v1.52.0, SQLite **3.53.2**), run and removed 2026-07-05:
  - Q1: `ADD COLUMN … REFERENCES … NOT NULL DEFAULT` **fails** with FK ON (*"Cannot add a REFERENCES column with non-NULL default value"*).
  - Q2/Q5: succeeds with FK OFF in an explicit tx; assigns all existing rows to workspace 1; FK re-armed to ON after.
  - Q3a-d: `ON DELETE RESTRICT` bites (err 1811), NOT NULL rejects NULL (1299), FK rejects nonexistent parent (787), omitted column defaults to 1.
  - Q6: `DROP COLUMN` + `DROP TABLE` Down succeeds under 3.53.2.
  - Q7: `DROP TABLE projects` with FK ON **cascades and wipes all `tasks` rows** (3 → 0).
  - Q8: `UNIQUE INDEX … COLLATE NOCASE` rejects `'personal'` vs `'Personal'` (2067).
  - **goose end-to-end probe:** the exact proposed `00012` runs through `goose.Up`/`DownTo`; post-Up FK=1, 2/2 projects→Personal, 3/3 tasks intact, re-run no-op (no second Personal), Down reverses clean.
- In-repo verified facts: `internal/store/migrate.go` (embed glob `migrations/*.sql`, `goose.Up`), `internal/store/store.go` (`_pragma=foreign_keys(1)`, `SetMaxOpenConns(1)`), `internal/api/icons.go` (`BackfillProjectIcons` collect-then-update template), `cmd/kamacu/main.go:98/124` (`store.Migrate` then `BackfillProjectIcons`), `internal/api/projects.go` (`Project`/`projectColumns`/`scanProject`, create INSERTs ~183/~276), migrations `00001`/`00006`/`00007`/`00008`/`00009`/`00010`/`00011` (conventions; latest = `00011_diff_viewed.sql` → new file `00012`).
- goose v3.27.1 source (`internal/sqlparser/parser.go` `useTx` default true, `NO TRANSACTION` sets false; `migration_sql.go` runSQLMigration tx vs no-tx paths) + README (`NO TRANSACTION` at top of file, governs Up+Down; semicolon delimiting).

### Secondary (MEDIUM)
- SQLite `ALTER TABLE` documented rules (ADD COLUMN REFERENCES/default restrictions; DROP TABLE implicit-DELETE cascade; `PRAGMA foreign_keys` no-op inside a transaction) — corroborated by the primary probes above.

### Tertiary (LOW)
- none required.

## Metadata

**Confidence breakdown:**
- Migration mechanic (Approach A): **HIGH** — verified end-to-end through goose against the exact pinned driver.
- Wire/write-path changes (D-09/D-10): **HIGH** — mirrors the verified `icon_letters`/`icon_color` precedent in the same file.
- Startup hook (D-07): **HIGH** on mechanics; **MEDIUM** on the collect-then-update reinterpretation (see Open Q1 — flagged for planner/discuss).

**Research date:** 2026-07-05
**Valid until:** ~2026-08-05 (stable stack; re-verify only if `modernc.org/sqlite` or `goose` is bumped).
