# Phase 25: Workspace Data Foundation - Pattern Map

**Mapped:** 2026-07-05
**Files analyzed:** 4 (2 new, 2 modified)
**Analogs found:** 4 / 4 (all exact in-repo analogs — this phase is a near-mechanical clone of the v1.7 icon-column precedent)

This is a **backend-only data-layer** phase. Every new/modified file has a strong existing analog in the same package or migrations directory; the planner should copy structure directly rather than invent. All excerpts below are quoted from the real repo with verified line numbers.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/store/migrations/00012_workspaces.sql` (NEW) | migration | schema-change / batch DDL | `00009_project_icons.sql` (ADD COLUMN NOT NULL DEFAULT + DROP COLUMN Down) + `00001_init.sql` (table + strftime defaults + `REFERENCES … ON DELETE`) | exact (composite of two migrations) |
| `internal/api/workspaces.go` (NEW) | utility / startup hook | batch / one-shot idempotent | `internal/api/icons.go` (`BackfillProjectIcons`) | exact (same package, same call slot) |
| `internal/api/projects.go` (MODIFIED) | controller/handler | request-response / CRUD | itself — the v1.7 `icon_letters`/`icon_color` threading | exact (self-precedent) |
| `cmd/kamacu/main.go` (MODIFIED) | config / wiring | startup sequence | itself — the existing `api.BackfillProjectIcons(db)` call site (`main.go:124`) | exact (self-precedent) |

## Pattern Assignments

### `internal/store/migrations/00012_workspaces.sql` (migration, schema-change DDL)

**Analogs:** `internal/store/migrations/00009_project_icons.sql` (column-add + Down), `internal/store/migrations/00001_init.sql` (table body, `strftime` defaults, `REFERENCES … ON DELETE`).

**Note:** RESEARCH.md §1 supplies the full copy-ready `00012` SQL (Approach A: `-- +goose NO TRANSACTION` + `PRAGMA foreign_keys=OFF` → `BEGIN` → CREATE/INSERT/ADD COLUMN → `COMMIT` → `PRAGMA foreign_keys=ON`). The excerpts below are the *repo idioms* that SQL reuses, so the migration stays consistent with the existing 11 migrations.

**Table + `strftime` timestamp-default idiom to mirror for `workspaces`** — from `00001_init.sql` lines 2-8 (the `projects` table that `workspaces` mirrors; D-01 says copy these exact defaults):
```sql
CREATE TABLE projects (
  id          INTEGER PRIMARY KEY,
  name        TEXT NOT NULL,
  repo_path   TEXT NOT NULL UNIQUE,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
```

**`REFERENCES … ON DELETE` FK syntax to copy** (D-04 wants `ON DELETE RESTRICT`, not CASCADE) — from `00001_init.sql` line 12 (the `tasks.project_id` FK; note it uses `CASCADE` — the new column deliberately differs by using `RESTRICT`):
```sql
project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
```

**ADD COLUMN NOT NULL DEFAULT idiom** — from `00009_project_icons.sql` lines 9-10 (the shape `00012`'s `ADD COLUMN workspace_id … NOT NULL DEFAULT 1 REFERENCES …` follows; the difference is the `REFERENCES` clause forces FK-OFF + NO TRANSACTION per RESEARCH.md Pitfall 1):
```sql
ALTER TABLE projects ADD COLUMN icon_letters TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN icon_color TEXT NOT NULL DEFAULT '';
```

**Down: `DROP COLUMN` directly, reverse order** — from `00009_project_icons.sql` lines 12-17 (D-08: modernc/SQLite 3.53 supports `DROP COLUMN`; `00012`'s Down adds a `DROP INDEX` + `DROP TABLE workspaces` and the same FK-toggle discipline):
```sql
-- +goose Down
-- modernc.org/sqlite tracks SQLite 3.53, which supports DROP COLUMN directly
-- (no table rebuild) — same as 00007's/00008's Down. One DROP per statement,
-- reverse order of the adds.
ALTER TABLE projects DROP COLUMN icon_color;
ALTER TABLE projects DROP COLUMN icon_letters;
```

**Migration file naming + embed pickup** — from `internal/store/migrate.go` lines 10-11 & 15-21 (the glob `migrations/*.sql` auto-picks up `00012`; goose version-tracking makes re-runs a no-op — the idempotency backbone for D-06):
```go
//go:embed migrations/*.sql
var migrationsFS embed.FS
// ...
func Migrate(db *sql.DB) error {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil { // dialect string is "sqlite3"
		return err // even though driver name is "sqlite"
	}
	return goose.Up(db, "migrations")
}
```
Latest existing migration is `00011_diff_viewed.sql`; the new file is therefore `00012_workspaces.sql`.

---

### `internal/api/workspaces.go` (utility / startup hook, one-shot idempotent)

**Analog:** `internal/api/icons.go` — `BackfillProjectIcons` (D-07 says "mirror this exactly").

**Imports pattern** — from `icons.go` lines 1-9 (the workspace hook needs a subset: `database/sql`, `errors`):
```go
package api

import (
	"database/sql"
	"errors"
	"math/rand/v2"
	"strings"
	"unicode"
)
```

**Idempotent one-shot backfill pattern (the whole function to model on)** — from `icons.go` lines 184-216. The key parts to carry over: the function signature `func BackfillWorkspaces(db *sql.DB) error`, the no-op guard, and error-return-not-panic:
```go
func BackfillProjectIcons(db *sql.DB) error {
	rows, err := db.Query(`SELECT id, name FROM projects WHERE icon_letters = '' OR icon_color = ''`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type fill struct {
		id      int64
		letters string
		color   string
	}
	var fills []fill
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		fills = append(fills, fill{id: id, letters: deriveLetters(name), color: pickColor()})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, f := range fills {
		if _, err := db.Exec(
			`UPDATE projects SET icon_letters = ?, icon_color = ? WHERE id = ?`,
			f.letters, f.color, f.id,
		); err != nil {
			return err
		}
	}
	return nil
}
```

**Collect-then-update single-writer discipline (the load-bearing doc-comment rule D-07 cites)** — from `icons.go` lines 178-183:
```go
// Collect-then-update is REQUIRED: the store opens with db.SetMaxOpenConns(1)
// (single-writer discipline, store.go), so holding the SELECT cursor open while
// issuing UPDATEs on the same connection would deadlock. We scan every row that
// needs filling into a slice (computing letters/color during the scan), close
// the cursor, then run the parameterized UPDATEs. All SQL uses '?' placeholders
// only — name/id are NEVER string-concatenated into the query (T-18-01).
```

**IMPORTANT divergence from the icon analog** (RESEARCH.md §3 + Open Q1): under Approach A the migration's `DEFAULT 1` already assigns every project, so `BackfillWorkspaces` does **not** run a SELECT→UPDATE loop over projects — it collapses to a thin "ensure the default workspace row exists" guard. The copy-ready form (RESEARCH.md §3) is:
```go
func BackfillWorkspaces(db *sql.DB) error {
	var id int64
	err := db.QueryRow(`SELECT id FROM workspaces WHERE is_default = 1`).Scan(&id)
	if err == nil {
		return nil // default already exists -> no-op (the healthy-boot path)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = db.Exec(`INSERT INTO workspaces (name, is_default) VALUES ('Personal', 1)`)
	return err
}
```
The `errors.Is(err, sql.ErrNoRows)` guard idiom is already used throughout `projects.go` (e.g. lines 175, 249, 429) — same package convention.

---

### `internal/api/projects.go` (controller/handler, request-response + CRUD)

**Analog:** itself — the v1.7 `icon_letters`/`icon_color` columns were threaded through this exact file the same way (D-10). Four coordinated edit sites; the excerpts below are the current state to insert `workspace_id` alongside.

**1. `Project` struct — add `WorkspaceID int64` after `IconColor`, before `CreatedAt`** — current struct at lines 26-47 (relevant tail):
```go
	IconLetters string `json:"icon_letters"`
	IconColor   string `json:"icon_color"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}
```
The struct doc-comment at lines 38-42 already states the column-order contract: *"Column ORDER here MUST match projectColumns and the scanProject Scan order (they sit between managed and the timestamps)."* — extend that invariant to `workspace_id`.

**2. `projectColumns` const — insert `workspace_id` in the same slot** — current const at line 90:
```go
const projectColumns = `id, name, repo_path, description, github_repo, managed, icon_letters, icon_color, created_at, updated_at`
```
Becomes `… icon_letters, icon_color, workspace_id, created_at, updated_at`.

**3. `scanProject` Scan order — add `&p.WorkspaceID` in the same slot** — current scan at lines 92-109 (the load-bearing `Scan(...)` call is line 103):
```go
func scanProject(row interface{ Scan(...any) error }) (Project, error) {
	var p Project
	var repo sql.NullString
	var managedInt int
	err := row.Scan(&p.ID, &p.Name, &p.RepoPath, &p.Description, &repo, &managedInt, &p.IconLetters, &p.IconColor, &p.CreatedAt, &p.UpdatedAt)
	if repo.Valid {
		p.GithubRepo = &repo.String
	}
	p.Managed = managedInt != 0
	return p, err
}
```
`workspace_id` is `INTEGER NOT NULL` → scan straight into `int64` (`&p.WorkspaceID`), no `sql.NullString`/`sql.NullInt64` — same as `icon_letters`/`icon_color` (never null on the wire, D-10). Position: after `&p.IconColor`, before `&p.CreatedAt`. Pitfall 4 (RESEARCH.md): the const slot and Scan slot MUST line up or `go build` throws a scan-arg error.

**4a. `create` folder-path INSERT — resolve default + set `workspace_id`** — current INSERT at lines 183-185:
```go
	p, err := scanProject(h.db.QueryRow(
		`INSERT INTO projects (name, repo_path, icon_letters, icon_color) VALUES (?, ?, ?, ?) RETURNING `+projectColumns,
		name, abs, deriveLetters(name), pickColor()))
```
Per D-09 / RESEARCH.md §2b: add a `SELECT id FROM workspaces WHERE is_default = 1` resolve before the INSERT, add the `workspace_id` column + `?` placeholder, and append `wsID` to the args. The `RETURNING `+projectColumns already carries the new column back onto the wire automatically (no extra work — that is why the const change propagates through both create paths and the PATCH `update`).

**4b. `createByRepo` INSERT — same resolve + set** — current INSERT at lines 276-278:
```go
	p, err := scanProject(h.db.QueryRow(
		`INSERT INTO projects (name, repo_path, github_repo, managed, description, icon_letters, icon_color) VALUES (?, ?, ?, 1, ?, ?, ?) RETURNING `+projectColumns,
		name, dest, canonical, desc, deriveLetters(name), pickColor()))
```
Add the `workspace_id` column + placeholder + `wsID` arg the same way (mind the literal `1` for `managed` — it is a hardcoded value in the VALUES list, so count placeholders carefully when inserting the new one).

**Error-handling idiom to reuse for the `is_default` resolve** — the file's standard reject shape (e.g. lines 186-189): `writeError(w, http.StatusInternalServerError, err.Error())` then `return`. The default-workspace resolve failing is a 500 (RESEARCH.md §2b).

---

### `cmd/kamacu/main.go` (config / wiring, startup sequence)

**Analog:** itself — the existing `api.BackfillProjectIcons(db)` call site.

**Call-site pattern to clone** — from `main.go` lines 119-127 (D-07: wire `api.BackfillWorkspaces(db)` immediately after this block; ordering is load-bearing — it must run after `store.Migrate` so the `workspaces` table exists):
```go
	// One-shot idempotent icon backfill (D-08): migration 00009 adds icon_letters
	// + icon_color (NOT NULL DEFAULT ''); this fills every pre-existing blank row
	// with derived letters + a palette color, reusing the SAME helpers the create
	// paths use. Ordering is load-bearing — it MUST run after Migrate (the columns
	// must exist). Cheap no-op on later boots once all rows are filled.
	if err := api.BackfillProjectIcons(db); err != nil {
		slog.Error("backfilling project icons", "error", err)
		os.Exit(1)
	}
```
The new hook mirrors this exactly (RESEARCH.md §3 — verified insert point is right after line 127):
```go
	if err := api.BackfillWorkspaces(db); err != nil {
		slog.Error("backfilling workspaces", "error", err)
		os.Exit(1)
	}
```
No new imports needed in `main.go` — `api`, `slog`, and `os` are already imported for the surrounding backfill/error blocks.

## Shared Patterns

### Single-writer store discipline (foreign_keys ON + SetMaxOpenConns(1))
**Source:** `internal/store/store.go` lines 12-25
**Apply to:** the migration (RESTRICT actually bites; the NO-TRANSACTION FK-toggle must re-arm `foreign_keys=ON` on the one pooled connection) AND `workspaces.go` (any SELECT→UPDATE must collect-then-update).
```go
func Open(dbPath string) (*sql.DB, error) {
	dsn := "file:" + dbPath +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // single-writer discipline: eliminates SQLITE_BUSY
	return db, db.Ping()
}
```
Two consequences the planner must honor: (1) `_pragma=foreign_keys(1)` means `ON DELETE RESTRICT` is enforced (D-04 is real, not decorative); (2) `SetMaxOpenConns(1)` means the migration leaving `foreign_keys=OFF` would disable FK enforcement app-wide for the whole process (RESEARCH.md Pitfall 3) — the trailing `PRAGMA foreign_keys=ON` is mandatory.

### Parameterized SQL only (`?` placeholders)
**Source:** `internal/api/icons.go` line 209, `internal/api/projects.go` throughout (e.g. lines 172, 452, 515)
**Apply to:** `workspaces.go` and both `projects.go` create INSERTs. Never string-concatenate names/ids. The migration is the one exception — it inserts the literal `'Personal'` (no user input), which is safe.

### Idempotent startup one-shots after `store.Migrate`
**Source:** `cmd/kamacu/main.go` — `migrate.Complete` (lines 112-117), `BackfillProjectIcons` (lines 124-127)
**Apply to:** `BackfillWorkspaces` joins this ordering-sensitive line-up. Each is a cheap no-op on healthy boots and `os.Exit(1)`s on error. The new hook slots in right after `BackfillProjectIcons`.

## No Analog Found

None. Every file in this phase has an exact in-repo analog (this is a deliberately mechanical clone of the v1.7 icon-column work). The planner should prefer these repo excerpts over RESEARCH.md's generic examples where they overlap — though note RESEARCH.md §1/§2/§2b/§3 already contain the copy-ready, empirically-verified final SQL/Go, which supersede the analog *shape* excerpts here for the exact bytes to write.

## Metadata

**Analog search scope:** `internal/store/migrations/` (all 11 existing migrations enumerated), `internal/api/` (`icons.go`, `projects.go`), `internal/store/` (`store.go`, `migrate.go`), `cmd/kamacu/main.go`.
**Files scanned:** 7 (5 read in full: `icons.go`, `projects.go`, `store.go`, `migrate.go`, `00009`/`00001` migrations; `main.go` targeted read lines 90-139).
**Pattern extraction date:** 2026-07-05
