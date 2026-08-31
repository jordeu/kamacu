# Phase 13: Global data foundation & safety net - Pattern Map

**Mapped:** 2026-08-25
**Files analyzed:** 7 (5 new, 2 modified)
**Analogs found:** 7 / 7 (every file has a direct in-repo precedent; the one heavier piece — the full-table rebuild — has research-verified SQL to lift)

> Scope guard from CONTEXT.md: **No API routes, no frontend, no spawn paths this phase.** D-01..D-16 decisions about `repos/global/`, validators, gates, and naming are Phase 14–16 contracts — the planner must NOT create files for them here. Phase 13 = 2 migrations + backfill + sweep fix + delete-guard extension + tests.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/store/migrations/00017_global_task.sql` (NEW) | migration (schema + seed) | batch (one-shot schema transform) | `internal/store/migrations/00013_agents.sql` | role-match (seeded table + FK RESTRICT; but 00017 needs a PLAIN `-- +goose Up` transaction — do NOT copy 00013's NO TRANSACTION header) |
| `internal/store/migrations/00018_tmux_scope.sql` (NEW) | migration (table rebuild) | transform (CREATE-copy-drop-rename) | `internal/store/migrations/00012_workspaces.sql` + `00013_agents.sql` (NO TRANSACTION discipline) | role-match, one notch heavier (first full rebuild in repo; verified SQL lives in 13-RESEARCH.md Pattern 2 — lift verbatim) |
| `internal/api/global_backfill.go` (NEW) | service (boot backfill hook) | batch (idempotent startup backfill) | `internal/api/agents_backfill.go` | exact (QueryRow no-op fast path → ErrNoRows → literal INSERT) |
| `internal/store/global_task_migration_test.go` (NEW; may combine the tmux-scope test — planner's call per discretion) | test (staged-upgrade migration proof) | batch (upgrade-path verification) | `internal/store/agents_migration_test.go` | exact (`goose.UpTo` staging → seed → `Migrate` → assert) |
| `cmd/kamacu/sweep_test.go` (NEW) | test (host-gated integration) | event-driven (startup reconciliation, kill/no-kill) | `internal/tmux/tmux_test.go:16-36` (`newTestClient`/`newDetachedSession`) + `internal/api/sessions_test.go:672-679` | exact (skip-if-absent + per-test socket + KillServer cleanup) |
| `internal/api/agents_crud.go` (MOD) | controller (HTTP delete handler) | request-response | itself — `agents_crud.go:226-275` (the projects in-use guard being extended) | exact (one-COUNT addition inside the existing guard) |
| `cmd/kamacu/serve.go` (MOD) | config (boot bootstrap) | batch (boot pipeline) + event-driven (sweep) | itself — backfill wiring block `serve.go:150-189` + `sweepOrphanTmux` `serve.go:351-403` | exact (one wiring block + one query string) |

## Pattern Assignments

### `internal/store/migrations/00017_global_task.sql` (migration, batch)

**Analog:** `internal/store/migrations/00013_agents.sql` (seeded table + RESTRICT FK + header-comment discipline)
**Also consult:** 13-RESEARCH.md Pattern 1 — the exact DDL was rehearsal-verified on a copy of the real install (11/11). Lift it; do not redesign.

**Header/comment pattern** (00013_agents.sql lines 1-14 — keep this commenting discipline, but 00017 is a fresh CREATE so a plain transaction suffices):

```sql
-- +goose NO TRANSACTION        -- ← 00013's annotation; 00017 must NOT have it (fresh
                                --   CREATE TABLE, none of the ALTER restrictions apply)
-- +goose Up
-- <WHY header: which requirement, which invariant, why the seed is here>
```

**Seeded-table + RESTRICT pattern** (00013_agents.sql lines 17-37 — the shape 00017 mirrors):

```sql
CREATE TABLE agents (                      -- → global_task: id PK CHECK(id=1), root_path
  id         INTEGER PRIMARY KEY,          --   DEFAULT '', github_repo, agent_id FK RESTRICT,
  ...                                       --   claude_session_id, opencode_session_id, timestamps
  created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
-- seed comment explains WHY the id is deterministic and what the flags protect
INSERT INTO agents (name, command, engine, is_default, is_system)
  VALUES ('Claude Code', 'claude', 'claude', 1, 1);
```

**CRITICAL deviation from the analog (Pitfall 4, 13-RESEARCH.md):** 00013 hardcodes `DEFAULT 1` because its own seed created id 1 in the same migration. 00017 MUST NOT — the real install has 4 agents and `POST /api/agents/{id}/default` moves the flag. Seed via:

```sql
INSERT INTO global_task (id, agent_id)
  SELECT 1, id FROM agents WHERE is_default = 1;  -- NEVER hardcode 1 (13-RESEARCH Pitfall 4)
```

**Down pattern** (00013_agents.sql lines 49-58): `-- +goose Down` → `DROP TABLE global_task;` (plain transaction; no FK toggling needed since nothing references it).

---

### `internal/store/migrations/00018_tmux_scope.sql` (migration, transform)

**Analog:** `internal/store/migrations/00012_workspaces.sql` (NO TRANSACTION + FK-off/re-arm discipline — proven shape, one notch lighter)
**Source table:** `internal/store/migrations/00005_tmux_sessions.sql` (the shape being rebuilt — every column/constraint must survive byte-for-byte)
**Primary source:** 13-RESEARCH.md Pattern 2 — the complete 12-step SQL is drafted, probe-verified (15/15), and rehearsal-verified on real data. **Lift it verbatim.**

**NO TRANSACTION discipline** (00012_workspaces.sql lines 1-15 — copy this header discipline exactly):

```sql
-- +goose NO TRANSACTION
-- +goose Up
-- WHY NO TRANSACTION + PRAGMA foreign_keys=OFF:
--   foreign_keys can only be toggled OUTSIDE a transaction, and goose runs
--   migrations in a transaction by default -- hence NO TRANSACTION and an
--   explicit BEGIN/COMMIT. store.go opens with SetMaxOpenConns(1), so this same
--   pooled connection MUST be re-armed with foreign_keys=ON at the end.
PRAGMA foreign_keys = OFF;
BEGIN;
```

**The current table being rebuilt** (00005_tmux_sessions.sql lines 7-15 — the new table keeps every column; only `task_id` loses NOT NULL and `scope` is added):

```sql
CREATE TABLE tmux_sessions (
    id         INTEGER PRIMARY KEY,
    task_id    INTEGER NOT NULL REFERENCES tasks(id),
    n          INTEGER NOT NULL,
    name       TEXT    NOT NULL UNIQUE,
    label      TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE(task_id, n)
);
```

**Rebuild + re-arm skeleton** (from 13-RESEARCH.md Pattern 2; ordering create-new → copy → drop-old → rename-new is the sqlite.org-blessed order — renaming old first is the documented way to corrupt references):

```sql
PRAGMA foreign_keys = OFF;
BEGIN;
CREATE TABLE tmux_sessions_new ( ... task_id INTEGER REFERENCES tasks(id),      -- nullable now
    scope TEXT NOT NULL DEFAULT 'task' CHECK (scope IN ('task','global')),
    ... UNIQUE(task_id, n),
    CHECK ( (task_id IS NULL) = (scope = 'global') ) );  -- the XOR, verified spelling
INSERT INTO tmux_sessions_new (id, task_id, scope, n, name, label, created_at)
  SELECT id, task_id, 'task', n, name, label, created_at FROM tmux_sessions;
DROP TABLE tmux_sessions;
ALTER TABLE tmux_sessions_new RENAME TO tmux_sessions;
COMMIT;
PRAGMA foreign_key_check;      -- non-gating parity (goose ignores its rows) — 00012/13 posture
PRAGMA foreign_keys = ON;      -- ★ re-arm: pooled single connection stays FK-off otherwise
```

**Error-handling notes baked into the analogs:** the trailing `PRAGMA foreign_key_check` is deliberately non-gating (00012 lines 33-35 comment); the `PRAGMA foreign_keys = ON` re-arm is load-bearing because `store.Open` pins one pooled connection (`store.go:23` `SetMaxOpenConns(1)`). `-- +goose Down` mirrors 00012 lines 38-47 (same FK-toggle discipline around the reverse rebuild).

---

### `internal/api/global_backfill.go` (service, batch backfill)

**Analog:** `internal/api/agents_backfill.go` — `BackfillAgents` (lines 22-37). Mirror it exactly; a third-of-fourth instance of the house backfill shape.

**Imports pattern** (agents_backfill.go lines 1-6):

```go
package api

import (
	"database/sql"
	"errors"
)
```

**Core backfill pattern** (agents_backfill.go lines 22-37 — QueryRow no-op fast path → ErrNoRows discrimination → literal INSERT; all SQL parameterless/literal):

```go
func BackfillAgents(db *sql.DB) error {
	var id int64
	err := db.QueryRow(`SELECT id FROM agents WHERE is_default = 1`).Scan(&id)
	if err == nil {
		return nil // default already exists -> no-op (the healthy-boot path)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err // a real DB error is propagated, never swallowed
	}
	_, err = db.Exec(`INSERT INTO agents (name, command, engine, is_default, is_system) VALUES ('Claude Code', 'claude', 'claude', 1, 1)`)
	return err
}
```

**`BackfillGlobalTask` translation:** fast path = `SELECT id FROM global_task WHERE id = 1` → no-op; ErrNoRows → `INSERT INTO global_task (id, agent_id) SELECT 1, id FROM agents WHERE is_default = 1` (same never-hardcode-1 rule as the migration). Keep the doc-comment posture of the analog (lines 8-21): state the invariant, why it's idempotent, why ordering after `BackfillAgents` is load-bearing.

**Sibling references:** `internal/api/workspaces.go:29-41` (`BackfillWorkspaces`, the origin of the shape) and `agents_backfill.go:89-104` (`BackfillOpenCodeAgent`, the newest instance — shows the no-op-normalize variant). Error handling is uniform: propagate real DB errors, never swallow.

---

### `internal/store/global_task_migration_test.go` (test, batch verification)

**Analog:** `internal/store/agents_migration_test.go` — `TestAgentsMigration` (lines 20-213). Mirror the staging discipline exactly; only the seeded tables and assertions change.

**Imports + staging pattern** (agents_migration_test.go lines 1-36):

```go
package store

import (
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestAgentsMigration(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))  // real DSN discipline (store.Open)
	// ... error handling ...
	goose.SetBaseFS(migrationsFS)                            // the embedded REAL migrations
	if err := goose.SetDialect("sqlite3"); err != nil { ... }
	if err := goose.UpTo(db, "migrations", 12); err != nil { // stage at pre-migration schema
		t.Fatalf("UpTo(12): %v", err)
	}
```

Phase 13 stages at **16** (pre-00017 v1.12 schema), seeds the "existing install" (projects + tasks + tmux_sessions rows with custom labels + exact `created_at` strings — snapshot them before), then `Migrate(db)` and asserts.

**Assertion patterns to reuse** (agents_migration_test.go):
- Pre-check staging worked (lines 39-47): `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='...'` must be 0.
- Data survival (lines 108-115): count before/after must match.
- RESTRICT bites (lines 175-179): `DELETE FROM agents WHERE id = 1` must error while referenced — for 00017, `DELETE FROM agents WHERE id = (SELECT agent_id FROM global_task)` must fail.
- **FK re-armed** (lines 181-188 — Pitfall 2's canary, keep verbatim):

```go
var fk int
if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
	t.Fatalf("PRAGMA foreign_keys: %v", err)
}
if fk != 1 {
	t.Errorf("foreign_keys after migration = %d, want 1 (re-armed ON)", fk)
}
```

- Idempotency (lines 190-212): second `Migrate(db)` is a clean no-op (singleton still exactly one row).
- Phase-13-specific (from 13-RESEARCH Pattern 3): byte-for-byte tmux snapshot diff; `CHECK(id=1)` rejects id=2; XOR CHECK rejected in BOTH directions (ambiguous `task_id` set + `scope='global'`, AND neither NULL + `scope='task'`) and accepted in both; bogus `task_id=999` insert fails (FK re-armed bites); every copied row reads `scope='task'`.

If the planner splits tests per migration (discretion), `tmux_scope_migration_test.go` uses the identical staging block — see also `internal/store/workspaces_migration_test.go` for a second instance of the shape.

---

### `cmd/kamacu/sweep_test.go` (test, host-gated integration)

**Analog:** `internal/tmux/tmux_test.go:16-36` (`newTestClient` + `newDetachedSession`) — the exact host-gate + per-test-socket helpers to re-create in package main.
**Package-main conventions:** `cmd/kamacu/main_test.go` (plain `package main` tests calling unexported funcs — `sweepOrphanTmux` is package-private, so the test MUST live here; confirmed no existing sweep test).

**Host-gate + socket pattern** (tmux_test.go lines 16-25):

```go
func newTestClient(t *testing.T) Client {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	socket := fmt.Sprintf("ktest-%d-%s", os.Getpid(), t.Name())
	c := Client{Socket: socket, ConfPath: "/dev/null"}
	t.Cleanup(func() { _ = c.KillServer(context.Background()) })
	return c
}
```

**Detached-session pattern** (tmux_test.go lines 30-36 — tests have no tty; production's `new-session -A` would fail):

```go
func newDetachedSession(t *testing.T, c Client, name string) {
	t.Helper()
	args := append(c.BaseArgs(), "new-session", "-d", "-s", name, "-c", t.TempDir())
	if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
		t.Fatalf("create detached session %q: %v: %s", name, err, out)
	}
}
```

**DB setup:** `store.Open(filepath.Join(t.TempDir(), "test.db"))` + `store.Migrate(db)` (same as the migration test) — the sweep takes `(*sql.DB, tmux.Client)`. Seed per 13-RESEARCH Pattern 4: task 1 + row `kamacu-1-1` (task-backed), row `kamacu-global-1` (`scope='global'`, NULL task — inserted **directly via SQL**, no spawn path exists), NO row for `kamacu-42-1` (true orphan). Live sessions: all three on the per-test socket. Assert after `sweepOrphanTmux(...)`: `kamacu-global-1` ALIVE, `kamacu-1-1` ALIVE, `kamacu-42-1` DEAD. Liveness probe helper pattern: `sessions_test.go:650-665` (poll `HasSession` with deadline). `HasSession`/`KillSession` exact-match `"="+name` semantics: `internal/tmux/tmux.go:88-98, 104-114`.

---

### `internal/api/agents_crud.go` (MOD — controller, request-response)

**Analog:** itself — the delete handler's in-use guard (`agents_crud.go:226-275`). The change is ONE additional COUNT block inside the existing guard sequence.

**Existing guard being extended** (agents_crud.go lines 235-264 — keep the order: pathID → 404 → system guard 409 → in-use guard 409 → DELETE → 204):

```go
func (h *agentCRUDHandlers) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	...
	// System guard: the claude seed is non-deletable.
	if isSystem == 1 {
		writeError(w, http.StatusConflict, "the system agent can't be deleted")
		return
	}
	// In-use guard: explicit count -> clean message (block-until-unassigned).
	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM projects WHERE agent_id = ?`, id).Scan(&n); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n > 0 {
		writeError(w, http.StatusConflict, fmt.Sprintf("reassign its %d project(s) first", n))
		return
	}
	// ← INSERT the global_task COUNT here (13-RESEARCH "Code Examples"):
	// var g int
	// if err := h.db.QueryRow(`SELECT COUNT(*) FROM global_task WHERE agent_id = ?`, id).Scan(&g); err != nil { ...500... }
	// if g > 0 { writeError(w, http.StatusConflict, "reassign the global task's agent first"); return }
```

**Error-handling patterns:** `writeError(w, http.StatusConflict, ...)` for guards, `writeError(w, http.StatusInternalServerError, err.Error())` for DB failures, `errors.Is(err, sql.ErrNoRows)` → 404 — all already in the handler; the addition reuses them verbatim. Message wording is planner discretion (13-RESEARCH Open Q3) but must stay in the count-carrying grammar above. The FK RESTRICT from 00017 is the backstop, not the UX — the guard runs BEFORE the delete for the clean message (comment at agents_crud.go:230-233 explains this split).

---

### `cmd/kamacu/serve.go` (MOD — config/boot, batch + event-driven)

**Analog:** itself — two touch points, both with in-file precedent.

**Touch point 1: backfill wiring** (serve.go lines 165-189 — the one-shot block; insert `BackfillGlobalTask` after line 189/`BackfillOpenCodeAgent`, keeping one-shots grouped — Pitfall 6: MUST be after `BackfillAgents` at line 171 because the seed reads the default agent):

```go
	// One-shot idempotent agent guard (M001 AGENTDATA): guarantee a default
	// Claude Code agent row always exists, mirroring BackfillWorkspaces. Ordering
	// is load-bearing -- it MUST run after store.Migrate ... Cheap no-op on
	// healthy boots; re-creates the claude seed only if the default is gone.
	if err := api.BackfillAgents(db); err != nil {
		slog.Error("backfilling agents", "error", err)
		return subcommands.ExitFailure
	}
```

Copy the block shape: requirement-tagged comment → `if err := api.BackfillX(db); err != nil` → `slog.Error(...)` → `return subcommands.ExitFailure` (an error refuses boot — the established posture, serve.go:124-127 for migrations).

**Touch point 2: the sweep known-set query** (serve.go lines 365-373 — the exact line to replace is 369):

```go
	// Known = a tmux_sessions row whose task STILL exists. The JOIN drops rows
	// whose task was deleted while down, so those sessions get swept too.
	known := make(map[string]bool)
	rows, err := db.QueryContext(ctx,
		`SELECT ts.name FROM tmux_sessions ts JOIN tasks t ON t.id = ts.task_id`)
	if err != nil {
		slog.Warn("orphan sweep: loading known tmux sessions", "error", err)
		return // can't tell orphan from live -> never kill blindly (Pitfall 6)
	}
```

Replace the query (and update the comment) with the verified scope-aware form from 13-RESEARCH "Code Examples":

```go
	`SELECT name FROM tmux_sessions WHERE scope = 'global' OR task_id IN (SELECT id FROM tasks)`
```

(LINE JOIN alternative is equivalent — discretion.) **Preserve everything else in `sweepOrphanTmux` (serve.go:351-403) unchanged** — the 30s timeout, `ListSessions` degrade-don't-break warn-only return (355-360), the `kamacu-` prefix filter (391), warn-only kill failures (397-400). Do NOT touch `internal/api/sessions.go` or `internal/reaper/reaper.go` — verified safe-by-construction (13-RESEARCH Pitfall 7).

## Shared Patterns

### Boot backfill wiring (error refuses boot)
**Source:** `cmd/kamacu/serve.go:150-189` (four instances: Icons, Workspaces, Agents, OpenCode)
**Apply to:** the `BackfillGlobalTask` wiring.
Shape: requirement-tagged doc comment stating ordering constraints → `if err := api.BackfillX(db); err != nil { slog.Error("...", "error", err); return subcommands.ExitFailure }`. Cheap no-op on healthy boots is the stated contract in every instance.

### NO TRANSACTION + FK-off/re-arm migration discipline
**Source:** `internal/store/migrations/00012_workspaces.sql:1-15, 32-36` and `00013_agents.sql:1-16, 43-47`
**Apply to:** `00018_tmux_scope.sql` only (00017 is a plain transaction).
Shape: `-- +goose NO TRANSACTION` → WHY header explaining `SetMaxOpenConns(1)` re-arm necessity → `PRAGMA foreign_keys = OFF; BEGIN; ...body... COMMIT; PRAGMA foreign_key_check; PRAGMA foreign_keys = ON;` — the re-arm is load-bearing (`store.go:23` single pooled connection; the DSN pragma only applies at open).

### Staged-upgrade migration testing
**Source:** `internal/store/agents_migration_test.go:20-93` (also `workspaces_migration_test.go`)
**Apply to:** both new migration tests.
Shape: `Open(t.TempDir()...)` → `goose.SetBaseFS(migrationsFS)` + `SetDialect("sqlite3")` → `goose.UpTo(db, "migrations", 16)` → sqlite_master pre-check → seed pre-migration data → snapshot → `Migrate(db)` → assert survival + structural guarantees (RESTRICT, CHECK, FK pragma, second-Migrate idempotence).

### Host-gated real-tmux tests
**Source:** `internal/tmux/tmux_test.go:16-36`; also `internal/api/sessions_test.go:672-679`
**Apply to:** `cmd/kamacu/sweep_test.go`.
Shape: `exec.LookPath("tmux")` → `t.Skip("tmux not on PATH")` → unique socket `fmt.Sprintf("ktest-...%d...", os.Getpid(), ...)` + `ConfPath: "/dev/null"` → `t.Cleanup(KillServer)` registered BEFORE creating sessions → detached sessions via `-d` (never production's `-A`).

### Count-carrying 409 delete-guard
**Source:** `internal/api/agents_crud.go:255-263`
**Apply to:** the global_task guard addition.
Shape: explicit `SELECT COUNT(*)` ahead of the delete → `writeError(w, http.StatusConflict, fmt.Sprintf(...))` with a "reassign ... first" message → FK RESTRICT as the DB backstop. (D-15's `{error, reasons:[...]}` grammar is the Phase-14 gate's shape — NOT needed for this single-COUNT guard.)

### Degrade-don't-break sweep posture
**Source:** `cmd/kamacu/serve.go:355-372`
**Apply to:** any touch of the sweep (only the query string changes).
Shape: tmux missing/broken → warn + return; query failure → warn + return ("never kill blindly"); kill failure → warn + continue.

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| (none) | — | — | Every file has a direct analog. The single piece without an in-repo precedent at this exact shape — the full CREATE-copy-drop-rename table rebuild in `00018_tmux_scope.sql` — is fully specified and empirically verified in 13-RESEARCH.md Pattern 2 (driver probe 15/15, goose runner green, real-install rehearsal 11/11); the planner lifts that SQL rather than searching for an analog. |

## Metadata

**Analog search scope:** `internal/store` (migrations + tests), `internal/api` (backfills, agents_crud, workspaces, sessions_test), `cmd/kamacu` (serve.go, main_test.go), `internal/tmux` (tmux.go, tmux_test.go)
**Files scanned:** 12 read in full or targeted (00005/00012/00013 migrations, store.go, migrate.go, agents_backfill.go, workspaces.go:1-55, agents_crud.go, serve.go, tmux.go, tmux_test.go:1-60, sessions_test.go:650-729, main_test.go)
**Pattern extraction date:** 2026-08-25
**Key external reference:** 13-RESEARCH.md Patterns 1-4 + Code Examples (verified SQL and test designs — the planner transcribes, not designs)
