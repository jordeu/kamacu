# Phase 13: Global data foundation & safety net - Research

**Researched:** 2026-08-25
**Domain:** SQLite schema migration (singleton table + full-table rebuild), goose migration discipline, startup invariants (idempotent backfill, scope-aware tmux orphan sweep), FK delete-guard extension
**Confidence:** HIGH — every claim verified at one of three levels: in-repo file:line reads, official SQLite docs, or **empirical probes run against the exact pinned dependency versions** (modernc.org/sqlite v1.52.0 = SQLite 3.53.2, goose v3.27.1), including a full upgrade rehearsal on a WAL-safe copy of the **real v1.12 install** (11/11 checks green).

## Summary

Phase 13 is a pure composition phase over proven in-repo patterns: the `global_task` singleton mirrors the 00012/00013 seeded-table + idempotent-backfill precedent, the `tmux_sessions` scope rebuild is the 00012/00013 `NO TRANSACTION` + `PRAGMA foreign_keys=OFF` discipline taken one notch heavier (CREATE-copy-drop-rename, the exact procedure sqlite.org §"Making Other Kinds Of Table Schema Changes" blesses), and the sweep fix is a one-query change in `cmd/kamacu/serve.go`. Zero new dependencies, zero API routes, zero frontend, zero spawn-path code.

The roadmap's flagged unknown — "the `tmux_sessions` full-table rebuild is the one piece without a direct in-repo precedent at this exact shape" — is now **closed by research**: this session drafted the exact 00017 + 00018 SQL, ran it through **goose v3.27.1 itself** against the pinned driver (15/15 driver-level checks + full goose run green), then ran the **real-install rehearsal** the roadmap asked for — a `.backup` copy of the live `~/.kamacu/kamacu.db` (goose v16, 147 tasks, 10 custom-labeled tmux rows, 4 agents) upgraded cleanly with every tmux row byte-for-byte identical, singleton/CHECK/RESTRICT/XOR/FK-re-arm all asserted on real data (11/11). The migration SQL, the XOR CHECK spelling, the sweep rewrite, and the rehearsal recipe in this document are all pre-verified — the planner lifts them; implementation is largely transcription plus the surrounding test/wiring work.

The one behavioral landmine (already known from milestone research, re-confirmed empirically here): the sweep's known-set query at `serve.go:369` is an INNER JOIN that drops NULL-task rows — on the real install's data it would classify a live `kamacu-global-*` session as orphaned and kill it at next startup. The verified fix (`WHERE scope='global' OR task_id IN (SELECT id FROM tasks)`) preserves task-orphan killing exactly while never killing globals. Per the co-phasing mandate it ships in the same phase as the migration — the regression test seeds a global row + a live tmux session directly (no spawn path exists until Phase 15).

**Primary recommendation:** Two migrations — 00017 `global_task` (plain transaction; fresh CREATE TABLE needs no FK-off discipline) + 00018 tmux rebuild (`NO TRANSACTION` + the verified 12-step shape) — plus `BackfillGlobalTask` wired after `BackfillAgents`, the one-query sweep fix, and a one-COUNT extension of the agents delete guard. Stage the migration test at `goose.UpTo(db, "migrations", 16)` (the exact `TestAgentsMigration` pattern) and repeat the real-install rehearsal as a scripted verification step.

<user_constraints>

## User Constraints (from CONTEXT.md)

### Locked Decisions

**Carry-Forward Locked (research-locked — do not reopen)**

- **Representation:** `global_task` singleton table + task-free "global scope" sessions — NEVER a sentinel project/task row, NEVER `TaskID = 0` (REQUIREMENTS Out-of-Scope; research P2/P4).
- **Co-phasing mandate:** the sweep fix ships WITH the tmux migration in this phase — migration alone kills every live global tmux tab at next startup.
- **Storage:** singleton table only — settings-KV rejected (loses FK integrity, mixes config with state).
- **tmux rebuild technique:** CREATE-copy-drop-rename under `PRAGMA foreign_keys=OFF` / `NO TRANSACTION` (00012/00013 discipline, one notch heavier). Whether this is one migration or two (e.g., 00017+00018) is a PLAN-PHASE detail — explicitly deferred, do not re-ask the user.
- **Zero new dependencies** (verified against `go.mod` / `web/package.json`).

**Managed-root namespace (bakes into the stored path format now; clone itself is Phase 14)**

- **D-01: Separate namespace.** The gh-cloned global root lives in its own namespace — structurally unrepresentable collision with a project's gated `os.RemoveAll` delete (research P8). No cross-entity dedup 409 guards anywhere.
- **D-02: Path format `~/.kamacu/repos/global/<owner>/<name>`.** Sits inside the existing `repos/` tree; `global` reads as a reserved pseudo-owner. Minimal new surface — no second top-level data dir.
- **D-03: Same repo as project + global is allowed.** The same `owner/name` may exist as a managed project clone AND the global root simultaneously — separate entities by design, zero extra guard code, disk cost is the user's explicit choice.
- **D-04: Reconfiguring never deletes the old clone.** When the root moves away from a managed clone, the clone stays on disk — so re-configuring the same repo later reattaches for free (Phase-14 SC2). No gated-removal code in the global path.

**Folder-root validation (validator lands in Phase 14; strictness locked now)**

- **D-05: Git repo required.** Folder-root validation reuses `validateRepoPath` semantics (current project behavior) — a git repo is the recovery story (checkout/reset) for a skip-permissions agent with no worktree isolation. Matches STACK + PITFALLS leans.
- **D-06: Persistent un-isolation banner.** When the root is a folder (vs managed clone), the global view shows a one-line persistent notice ("agent runs directly in \<root\> — no worktree isolation"). This is the P3 safety element — deliberately DISTINCT from the deselected GT-FUT-01 (root-path legibility line) and GT-FUT-02 (git-status summary).
- **D-07: Block obvious footguns.** `$HOME` / `~` / `/` (and equivalents) are rejected with a clear 400 at configure time — one equality check against the P3 worst case.
- **D-08: Spawn-time honesty for a vanished root.** Config validates once at PUT; if the dir disappears later, spawn fails with an honest error (mirrors project folder-path behavior — validated at creation, failures surface at use). No boot-time revalidation.

**UI naming (label strings start emitting in Phase 15; decided once, early)**

- **D-09: User-facing name is "Scratchpad".** Avoids the collision with v1.12 Activity's "Global" scope-selector label. All user-facing copy (Settings section, view title, "Open Scratchpad" affordance) says Scratchpad.
- **D-10: Synthesized label strings: `projectName:"Global"`, `taskTitle:"Scratchpad"`.** The bar/status row reads "Global · Scratchpad" — scope + name, no duplication; keeps the TS wire contract non-nullable.
- **D-11: Internal naming stays `global` everywhere.** `scope:"global"`, `/global` route, `global_task` table, `kamacu-global-*` tmux mint, `repos/global/` namespace — the label is presentation-only; zero churn against the research architecture.
- **D-12: Bar-row visual treatment settles at UAT (Phase 17).** Research says cosmetic; strings are locked now, visuals (globe badge vs text-only) deferred.

**"Live" definition for the reconfigure gate (gate enforces in Phase 14; semantics locked now)**

- **D-13: Any live global PTY blocks.** Agent + plain-bash + tmux tabs all count as live for the GCONF-04 409 gate (one `ListGlobal()`-nonempty check). A cwd change under a running shell is a lie waiting to happen.
- **D-14: Exited sessions never block.** Exited PTYs and persisted resume ids do not gate — only live processes. On success the resume ids are cleared (they point at conversations in the OLD root).
- **D-15: 409 shape is a reasons list.** `{error, reasons:["agent session running", "2 bash tabs running"]}` — mirrors the existing gated-delete 409 grammar (managed project delete, worktree cleanup) app-wide.
- **D-16: Resume ids clear on ANY successful root change.** Folder↔repo, or clearing — uniform GCONF-04 contract; no no-op-re-PUT exception branch.

### the agent's Discretion

- Migration split shape (one combined migration vs 00017+00018) — explicitly a plan-phase detail per research.
- The exact scope-discriminator column shape (`scope TEXT` vs `global INTEGER`) and column names/granularity for resume ids in the singleton — GDATA-01/02 lock the behavior, the researcher/planner picks the schema spelling.
- The sweep known-set query rewrite (LEFT JOIN vs UNION of global rows) — implementation detail of GDATA-03.

### Deferred Ideas (OUT OF SCOPE)

- Bar-row visual treatment (globe badge vs text-only) — settles at UAT in Phase 17 (D-12).
- GT-FUT-01..07 remain parked in REQUIREMENTS.md (promotion, multiple scratchpads, per-workspace scratchpads, MCP write parity, sidebar entry, root-path line, git-status line).

</user_constraints>

<phase_requirements>

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| GDATA-01 | A DB-enforced `global_task` singleton row (id=1) stores the configured root (path + managed/`github_repo` marker), the default `agent_id` (FK to agents, ON DELETE RESTRICT), and restart-resume session ids — seeded with the default agent at migration and guaranteed by an idempotent boot backfill | Verified singleton DDL (CHECK(id=1) rejects second row; RESTRICT refuses agent delete — proven on real install copy); the seed must `SELECT ... WHERE is_default = 1` (never hardcode 1 — real install has 4 agents and the default flag is movable); backfill mirrors `BackfillAgents` (agents_backfill.go:22) and wires after it in serve.go |
| GDATA-02 | `tmux_sessions` is rebuilt so global bash-tab rows can exist (nullable `task_id` / explicit scope discriminator with an XOR CHECK); existing rows, labels, and FK discipline survive byte-for-byte (goose upgrade-path tested on a seeded install) | The exact rebuild SQL is drafted and triple-verified (driver probe 15/15, goose runner green, real-install rehearsal 11/11 with byte-for-byte tmux row comparison); `scope TEXT NOT NULL DEFAULT 'task'` + XOR CHECK `(task_id IS NULL) = (scope = 'global')` keeps every existing INSERT/writer valid unchanged; test staging pattern = `goose.UpTo(db, "migrations", 16)` per `TestAgentsMigration` |
| GDATA-03 | The startup tmux orphan sweep is scope-aware — global tmux tabs are never killed at startup (regression test: sweep-no-kill on a live global tab) | The bug demonstrated empirically (INNER JOIN drops NULL-task rows: 10 of 11 known on real data); verified fix query `WHERE scope='global' OR task_id IN (SELECT id FROM tasks)`; host-gated regression test design (seed a `scope='global'` row via SQL + a live `kamacu-global-1` on a per-test tmux socket + an orphan that MUST still die) — tmux 3.4 present on this host |

</phase_requirements>

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Singleton schema + seed (GDATA-01) | Database (migration 00017) | Boot backfill (serve.go wiring) | The invariant is DB-enforced (`CHECK(id=1)`, FK RESTRICT); the backfill re-arms it every boot because a migration runs exactly once — the v1.9/v1.10 split of labor |
| tmux_sessions scope rebuild (GDATA-02) | Database (migration 00018) | — | Schema shape is purely a DB concern; every existing Go reader is safe by construction (verified audit below) — no API/engine changes in this phase |
| Scope-aware orphan sweep (GDATA-03) | Go server startup (cmd/kamacu serve.go) | Database (the known-set query) | The sweep is startup reconciliation logic in package main; only its known-set SQL changes |
| Agents delete-guard extension | API layer (agents_crud.go) | Database (FK RESTRICT backstop) | The handler owns the friendly 409 with a clean message; the FK is the invariant of last resort — the exact projects.agent_id pattern (agents_crud.go:226-264) |
| Resume-id storage columns | Database (global_task) | Phase-15 spawn path (writer) | This phase only provides the columns; the newest-wins writes land with the spawn path |
| `~/.kamacu/repos/global/` namespace | Filesystem convention (locked D-02) | Phase-14 clone code | No code this phase — the path FORMAT is baked into the singleton's storage contract now |

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `modernc.org/sqlite` | v1.52.0 (pinned, go.mod) | SQLite driver — executes the rebuild | **Empirically verified this session**: reports `sqlite_version() = 3.53.2`; runs the full 12-step rebuild, XOR CHECK, FK re-arm correctly on the pooled `SetMaxOpenConns(1)` handle [VERIFIED: probe on pinned driver] |
| `github.com/pressly/goose/v3` | v3.27.1 (pinned, go.mod) | Migration runner — embedded FS + version table | **Empirically verified this session**: its statement splitter and NO TRANSACTION mode execute the exact rebuild file cleanly; second `Up` is a no-op [VERIFIED: goose-runner probe] |
| `internal/store` (in-repo) | — | `Open` (WAL + `foreign_keys(1)` DSN + `SetMaxOpenConns(1)`), `Migrate` (goose.Up over embedded FS), migration-test staging pattern (`goose.UpTo(db, "migrations", N)`) | The single pooled connection is why the migration must re-arm `PRAGMA foreign_keys=ON` itself — documented hazard in 00012's header comment (store.go:23, migrations 00012/13) |
| `internal/api/agents_backfill.go` (in-repo) | — | The idempotent boot-backfill template `BackfillGlobalTask` mirrors | `BackfillAgents` (line 22): QueryRow no-op fast path → `ErrNoRows` check → literal INSERT; wired once in serve.go (line 171), error refuses boot |
| `internal/tmux` (in-repo) | — | Real-tmux host-gated tests: `Client{Socket, ConfPath}` per-test sockets, `t.Skip("tmux not on PATH")` convention | The sweep regression test needs a LIVE tmux session; the house pattern is per-test sockets + skip-if-absent (tmux_test.go:19, sessions_test.go:674) |

### Supporting

None. **Zero new dependencies** — locked decision, verified against `go.mod` and `web/package.json` by milestone STACK research and re-confirmed here (backend-only phase; no frontend changes).

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| CREATE-copy-drop-rename rebuild (LOCKED) | `ALTER TABLE tmux_sessions ALTER COLUMN task_id DROP NOT NULL` + `ADD COLUMN scope` | **Verified to work on SQLite 3.53.2** (probe C1/C2) — new 3.53.0 syntax, zero in-repo precedent, and it still can't express the table-level XOR CHECK without a rebuild anyway. **LOCKED OUT by CONTEXT.md carry-forward — do not use** [VERIFIED: capability exists; ASSUMED: nothing — the lock is a decision, not a fact] |
| `scope TEXT` discriminator (recommended) | `global INTEGER NOT NULL DEFAULT 0` (STACK.md's original shape) | Same information; TEXT is self-documenting in queries, extensible to future scopes, and pairs with the exact XOR CHECK verified in the probe. Either satisfies GDATA-02; planner picks once |
| Two migrations 00017+00018 (recommended) | One combined 00017 | Combined forces the plain-CREATE global_task into a NO TRANSACTION file for no benefit; split keeps the heavy discipline scoped to the one migration that needs it, and the split is what the rehearsal validated |

**Installation:**

```bash
# nothing — zero new dependencies (locked decision)
```

**Version verification:** go.mod read 2026-08-25: `modernc.org/sqlite v1.52.0`, `github.com/pressly/goose/v3 v3.27.1` — both exercised directly by this session's probes.

## Package Legitimacy Audit

> No packages are installed by this phase (zero-new-deps locked decision; `go.mod`/`web/package.json` untouched). Audit not applicable — table kept explicit for the record.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| (none) | — | — | — | — | — | No installs this phase |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

Phase 13 changes the **boot-time pipeline** and the schema it starts from. Runtime request paths are untouched.

```
kamacu serve
  │
  ├─ store.Open(dbPath)                      DSN: WAL, busy_timeout, foreign_keys(1), txlock=immediate
  │                                          SetMaxOpenConns(1) — ONE pooled connection (store.go:23)
  │
  ├─ store.Migrate(db) ──► goose.Up(embedded FS)
  │     ├─ 00001..00016   (unchanged, v1.12 install is here)
  │     ├─ 00017  NEW     global_task singleton  [plain transaction — fresh CREATE TABLE]
  │     │         CREATE TABLE global_task (id PK CHECK(id=1), root_path DEFAULT '',
  │     │           github_repo, agent_id FK→agents ON DELETE RESTRICT,
  │     │           claude_session_id, opencode_session_id, timestamps)
  │     │         INSERT id=1, agent_id=(SELECT id FROM agents WHERE is_default=1)
  │     └─ 00018  NEW     tmux_sessions rebuild  [-- +goose NO TRANSACTION]
  │            PRAGMA foreign_keys=OFF → BEGIN → CREATE tmux_sessions_new
  │            (task_id nullable, scope TEXT DEFAULT 'task', XOR CHECK)
  │            → INSERT..SELECT copy (scope='task') → DROP old → RENAME → COMMIT
  │            → PRAGMA foreign_key_check (non-gating) → PRAGMA foreign_keys=ON  ★re-arm
  │
  ├─ migrate.Complete(...)                   (v1.8 dir migration — unchanged)
  ├─ BackfillProjectIcons / BackfillWorkspaces / BackfillAgents /
  │  BackfillAgentExtraParams / BackfillOpenCodeAgent            (unchanged one-shots)
  ├─ BackfillGlobalTask(db)      NEW         re-insert singleton if a hand-DELETE
  │                                          dropped it — MUST run after BackfillAgents
  │                                          (the seed reads the default agent)
  │
  ├─ sweepOrphanTmux(ctx, db, tmuxClient)    MOD — known-set query becomes scope-aware:
  │     OLD: SELECT ts.name FROM tmux_sessions ts JOIN tasks t ON t.id = ts.task_id
  │          (INNER JOIN — DROPS NULL-task rows → would kill every live global tab)
  │     NEW: SELECT name FROM tmux_sessions
  │          WHERE scope='global' OR task_id IN (SELECT id FROM tasks)
  │
  ├─ reaper.NewWithPR(...).Run(...)          (unchanged — task-keyed queries never
  │                                          match NULL-task rows by construction)
  └─ ListenAndServe                           (no new routes this phase)

Readers/writers of tmux_sessions (all verified safe-by-construction, NO changes):
  sessions.go:145  reconcile    WHERE task_id = ?      NULL never matches → global rows invisible ✓
  sessions.go:208  lazy GC      DELETE WHERE name = ?  name-keyed ✓
  sessions.go:418  n reserve    WHERE task_id = ?      ✓ (global n-reserve lands Phase 15)
  sessions.go:428  INSERT       omits scope            → DEFAULT 'task' → XOR holds ✓ zero change
  reaper.go:335/370 DELETE/SELECT WHERE task_id = ?    ✓
```

### Recommended Project Structure

```
internal/store/migrations/
├── 00017_global_task.sql          # NEW — singleton (plain transaction)
├── 00018_tmux_scope.sql           # NEW — rebuild (NO TRANSACTION + FK-off discipline)
internal/store/
├── global_task_migration_test.go  # NEW — staged-upgrade test (UpTo(16) + seed + Migrate)
└── tmux_scope_migration_test.go   # NEW — (or combined file; planner's call)
internal/api/
├── global_backfill.go             # NEW — BackfillGlobalTask (mirrors agents_backfill.go)
└── agents_crud.go                 # MOD — delete guard: +1 COUNT against global_task
cmd/kamacu/
├── serve.go                       # MOD — backfill wiring; sweepOrphanTmux known-set query
└── sweep_test.go                  # NEW — host-gated sweep-no-kill / still-kills regression
```

### Pattern 1: Singleton row + seeded migration + idempotent boot backfill (GDATA-01)

**What:** migration creates the table and seeds row id=1; a boot-time hook re-creates the row if it ever goes missing. The migration runs once; the backfill runs every boot — together the invariant is unconditional.

**When to use:** every DB-enforced singleton (third use after workspaces' Personal and agents' Claude seed).

**Example** (verified on real-install copy — the exact shape the rehearsal ran):

```sql
-- Source: milestone ARCHITECTURE.md 00017 sketch + this session's rehearsal (11/11 green)
-- +goose Up
CREATE TABLE global_task (
  id                  INTEGER PRIMARY KEY CHECK (id = 1),
  root_path           TEXT NOT NULL DEFAULT '',             -- '' = unconfigured
  github_repo         TEXT,                                  -- NOT NULL ⇒ managed clone
  agent_id            INTEGER NOT NULL REFERENCES agents(id) ON DELETE RESTRICT,
  claude_session_id   TEXT,
  opencode_session_id TEXT,
  created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
INSERT INTO global_task (id, agent_id)
  SELECT 1, id FROM agents WHERE is_default = 1;  -- NEVER hardcode 1: the flag is movable
```

Notes (verified): `CHECK (id = 1)` rejects a second row (probe + real-install rehearsal); the FK RESTRICT refuses deleting the referenced agent; a plain `-- +goose Up` transaction is correct here (fresh CREATE — none of the ALTER-restrictions apply). The backfill mirrors `BackfillAgents` exactly: `SELECT` fast-path no-op → literal `INSERT INTO global_task (id, agent_id) SELECT 1, id FROM agents WHERE is_default = 1`. Because it seeds from the default agent, **wiring order is load-bearing: after `BackfillAgents`** (serve.go:171) — by then a default always exists.

### Pattern 2: The 12-step table rebuild under goose NO TRANSACTION (GDATA-02)

**What:** SQLite cannot drop a column's NOT NULL via the older ALTER surface this codebase targets, so the table is rebuilt: create-with-new-shape → copy → drop old → rename, with FKs disabled for the duration. The ordering (create-new FIRST, drop-then-rename) is the exact procedure sqlite.org blesses; the rename-old-first variant is documented to corrupt references.

**When to use:** any change SQLite's ALTER TABLE can't express (this is the codebase's first — 00012/00013 only ADD COLUMN).

**Example** (verified end-to-end by this session's goose-runner probe AND the real-install rehearsal — lift verbatim):

```sql
-- Source: sqlite.org/lang_altertable.html §8 + probe-verified on modernc v1.52.0 / goose v3.27.1
-- +goose NO TRANSACTION
-- +goose Up
PRAGMA foreign_keys = OFF;               -- must be OUTSIDE any transaction to take effect
BEGIN;
CREATE TABLE tmux_sessions_new (
    id         INTEGER PRIMARY KEY,
    task_id    INTEGER REFERENCES tasks(id),   -- nullable now: global rows have no task
    scope      TEXT NOT NULL DEFAULT 'task' CHECK (scope IN ('task','global')),
    n          INTEGER NOT NULL,
    name       TEXT    NOT NULL UNIQUE,
    label      TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE(task_id, n),
    CHECK ( (task_id IS NULL) = (scope = 'global') )   -- the XOR, verified spelling
);
INSERT INTO tmux_sessions_new (id, task_id, scope, n, name, label, created_at)
  SELECT id, task_id, 'task', n, name, label, created_at FROM tmux_sessions;
DROP TABLE tmux_sessions;
ALTER TABLE tmux_sessions_new RENAME TO tmux_sessions;
COMMIT;
PRAGMA foreign_key_check;                -- non-gating parity (goose ignores its rows) — 00012/13 posture
PRAGMA foreign_keys = ON;                -- ★ re-arm: the pooled single connection stays FK-off otherwise
```

Verified properties of this exact SQL (driver probe A1–A11 + goose probe + real-install rehearsal):
- Copy fidelity: ids, task_ids, n, names, labels (incl. custom), created_at survive byte-for-byte (exact string compare on the real install's 10 rows).
- XOR CHECK: ambiguous (task_id set + scope='global') REJECTED; neither (NULL + scope='task') REJECTED; task rows and global rows accepted. Error message embeds the expression text (`CHECK constraint failed: (task_id IS NULL) = (scope = 'global')`, rc 275) — useful for test assertions.
- `PRAGMA foreign_keys = ON` inside a transaction is a **no-op** (probe B2) — hence NO TRANSACTION + toggling outside BEGIN/COMMIT.
- Re-armed FK bites immediately on the same pooled handle (bogus `task_id=999` insert fails, rc 787).
- `UNIQUE(task_id, n)` goes **soft for global rows** (NULLs distinct — sqlite.org lang_createtable §3.6): two global rows may share n; `name UNIQUE` is the real collision guard (probe A7/A8). Global-n monotonicity is therefore app-side (Phase 15's `MAX(n)+1 WHERE scope='global'` reserve) — same accepted read-then-insert posture as today (sessions.go:414-431).
- Nothing else in the schema references `tmux_sessions` (verified against the real install's sqlite_master: only the table itself) → the RENAME rewrites no foreign REFERENCES; triggers/views/indexes: none exist.

### Pattern 3: Staged-upgrade migration test (the goose upgrade-path proof)

**What:** exercise the REAL goose runner over the REAL migration files against a DB staged at the pre-migration schema with seeded "existing install" data.

**When to use:** every migration phase — the repo's proven shape (`TestAgentsMigration`, `TestWorkspacesMigration`).

**Example:**

```go
// Source: internal/store/agents_migration_test.go (proven in-repo pattern, read verbatim)
func TestGlobalTaskMigration(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))  // real DSN discipline
	// ... error handling ...
	goose.SetBaseFS(migrationsFS)                            // the embedded REAL migrations
	goose.SetDialect("sqlite3")
	if err := goose.UpTo(db, "migrations", 16); err != nil { // stage at v1.12 (pre-00017)
		t.Fatalf("UpTo(16): %v", err)
	}
	// seed the "existing install": projects + tasks + tmux_sessions rows WITH custom
	// labels + exact created_at strings — the byte-for-byte survival data (SC1)
	// ... INSERTs ...
	// snapshot: SELECT id,task_id,n,name,label,created_at FROM tmux_sessions ORDER BY id
	if err := Migrate(db); err != nil {                      // apply 00017+00018 for real
		t.Fatalf("Migrate: %v", err)
	}
	// assert: snapshot identical; scope='task' on every row; singleton exists + seeded
	// from is_default; CHECK(id=1) rejects id=2; RESTRICT refuses agent delete;
	// global row insertable; ambiguous rejected; bogus task_id rejected (FK re-armed);
	// PRAGMA foreign_keys == 1; second Migrate no-op (idempotence)
}
```

### Pattern 4: Host-gated real-tmux sweep regression (GDATA-03)

**What:** the sweep takes a concrete `tmux.Client` struct (serve.go:351 — not an interface), so the regression test uses the real tmux binary on a per-test socket, skipping when absent.

**When to use:** any test that must observe actual kill/no-kill behavior.

**Example:**

```go
// Source: house pattern — tmux_test.go:19, sessions_test.go:674 (`t.Skip("tmux not on PATH")`)
// + Client struct doc (tmux.go:47: "per-test sockets in tests")
func TestSweepOrphanTmuxScopeAware(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	db := store.Open(t.TempDir()/...) ; store.Migrate(db)
	// seed: task 1 + row kamacu-1-1 (task-backed); row kamacu-global-1 (scope='global');
	// NO row for kamacu-42-1, and task 42 does not exist (a true task-orphan)
	// live sessions on a unique test socket: kamacu-global-1, kamacu-1-1, kamacu-42-1
	client := tmux.Client{Socket: "kamacu-test-<uniq>", ConfPath: "/dev/null"}
	sweepOrphanTmux(context.Background(), db, client)
	// assert: kamacu-global-1 ALIVE (sweep-no-kill on a live global tab — SC4)
	//         kamacu-1-1    ALIVE (task-backed, unchanged behavior)
	//         kamacu-42-1  DEAD  (pre-existing orphan-killing preserved — SC4 second half)
}
```

Design note: no live global spawn path exists until Phase 15, so the test seeds the `scope='global'` row **directly via SQL** and starts the tmux session by hand (`tmux -L <socket> new-session -d -s kamacu-global-1`) — the row is the sweep's only input.

### Anti-Patterns to Avoid

- **Inverted/soft XOR CHECK spellings:** `(task_id IS NOT NULL) = (scope='global')` is backwards (rejects every task row at copy time — fails loudly, but only if you test the copy), and `task_id IS NULL OR scope='global'` passes everything silently. Assert BOTH rejection directions in tests (ambiguous AND neither). [VERIFIED: probe A5/A6 + first-draft inversion demonstrated]
- **Rename-old-first rebuild ordering:** sqlite.org's caution box — renaming `tmux_sessions` away before creating the new table is the documented way to corrupt references. Create-new → copy → drop-old → rename-new. [CITED: sqlite.org/lang_altertable.html §8]
- **Hardcoding the seed agent id:** the real install has 4 agents and `POST /api/agents/{id}/default` moves the flag; seed via `SELECT id FROM agents WHERE is_default = 1`. [VERIFIED: real install agent table read]
- **`cp` on the live DB for the rehearsal:** the real install carries a 4MB WAL — a raw `cp` while the server runs can tear. Use `sqlite3 ~/.kamacu/kamacu.db ".backup <dest>"` (online-backup API, safe while live). [VERIFIED: 4,124,152-byte WAL observed 2026-08-25]
- **Migration-only seeding with no backfill:** goose runs the migration exactly once; a hand-deleted singleton would stay missing forever. SC2 explicitly requires the backfill re-arm. [VERIFIED: pattern established at agents_backfill.go:22]

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Table rebuild procedure | Ad-hoc rename/drop sequences | The sqlite.org 12-step (drafted above) | Reference procedure; the blessed ordering avoids reference corruption; already probe-verified on the pinned driver |
| Singleton re-arm | Migration-time checks / triggers | Boot backfill hook (BackfillAgents shape) | Migrations run once; the invariants must hold every boot; three in-repo precedents |
| Friendly delete-guard | Relying on the raw FK error surfacing | Handler COUNT + clean 409 ahead of RESTRICT | The projects.agent_id pattern (agents_crud.go:255-263): RESTRICT is the backstop, not the UX |
| Orphan detection | New sweep machinery | Fix the known-set query only | The sweep's kill loop, degrade-don't-break posture, and `kamacu-` prefix filter are all correct; only the known-set is wrong |

**Key insight:** this phase's risk is concentrated in ~90 lines of SQL and one query — everything around them (backfill wiring, delete-guard shape, test staging, host-gated tmux tests) has a direct, green in-repo precedent. The research has already de-risked the SQL itself; the planner's job is sequencing and test coverage, not design exploration.

## Runtime State Inventory

> Included because this is a migration phase (schema migration of live runtime state). The canonical question: after the repo is updated, what runtime systems still hold pre-migration state?

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | **Real install DB exists**: `~/.kamacu/kamacu.db` at goose v16 — 147 tasks, 10 tmux_sessions rows (ALL with custom labels: "Sched", "Tunnel", "Nextflow", …), 4 agents (default=Claude id=1), plus workspaces/settings/agent_sessions state | The migrations transform it in place; verified safe by rehearsal on a `.backup` copy (11/11). Re-run the scripted rehearsal as a phase verification step |
| Live service config | tmux server on the dedicated `-L kamacu` socket may hold live `kamacu-*` sessions while the binary is upgraded (server currently running: -wal modified today) | None this phase — no live global tabs can exist yet (no spawn path). The sweep fix protects the FUTURE class; the regression test simulates it |
| OS-registered state | None — kamacu is a foreground process, no scheduler/launchd registrations | None — verified by design (PROJECT.md: local-only single process) |
| Secrets/env vars | None — the singleton stores no secrets; no env var names change | None |
| Build artifacts | None — pure-Go single binary; no compiled artifacts carry schema state | None |

## Common Pitfalls

### Pitfall 1: Shipping the migration without the sweep fix (the co-phasing mandate)

**What goes wrong:** every live `kamacu-global-*` tmux session is killed at the next startup.
**Why it happens:** the known-set query (`serve.go:369`) INNER-JOINS tasks — NULL-task rows drop out, so live global sessions look orphaned. **Demonstrated on real data this session: 11 known rows → 10 under the INNER JOIN.**
**How to avoid:** the one-query fix ships in the same phase (locked decision): `SELECT name FROM tmux_sessions WHERE scope='global' OR task_id IN (SELECT id FROM tasks)` — verified to keep all 11.
**Warning signs:** a migration PR whose sweep diff is empty; a sweep test that only seeds task rows.

### Pitfall 2: Forgetting the FK re-arm (or toggling inside a transaction)

**What goes wrong:** after 00018, the pooled connection silently keeps `foreign_keys=OFF` — inserts with dangling task_id succeed until the pool ever reopens (the DSN would re-arm ON), making enforcement nondeterministic.
**Why it happens:** `PRAGMA foreign_keys` is a **no-op inside a transaction** (verified, probe B2) and `store.Open` pins one connection (`SetMaxOpenConns(1)`); goose's default transaction would swallow the toggle.
**How to avoid:** `-- +goose NO TRANSACTION` + explicit BEGIN/COMMIT + trailing `PRAGMA foreign_keys = ON` — exactly the 00012/00013 discipline; the migration test asserts `PRAGMA foreign_keys == 1` on the migrated handle (agents_migration_test.go:181-188 precedent).
**Warning signs:** a rebuild migration without the NO TRANSACTION annotation; a test that never probes the pragma.

### Pitfall 3: Wrong XOR CHECK spelling

**What goes wrong:** the inverted spelling rejects all task rows at copy time (migration aborts — loud), or an OR-spelling accepts everything (silent — the CHECK protects nothing).
**Why it happens:** `(task_id IS NULL) = (scope = 'global')` is only one of several plausible-looking spellings; truth-table discipline is required.
**How to avoid:** use the verified spelling verbatim; assert both rejection directions (ambiguous + neither) AND both acceptance directions in the migration test.
**Warning signs:** tests that only assert the happy path.

### Pitfall 4: Seeding the singleton from a hardcoded agent id

**What goes wrong:** installs where the user moved the default flag (real install: 4 agents, flag movable via POST /default) seed a non-default agent, or the INSERT..SELECT finds zero rows and silently seeds nothing.
**Why it happens:** copy-pasting `agent_id = 1` from 00013's DEFAULT (which was safe there because the seed itself created id 1 in the same migration).
**How to avoid:** `SELECT 1, id FROM agents WHERE is_default = 1` in both migration and backfill; backfill wiring AFTER `BackfillAgents` guarantees a default exists.
**Warning signs:** a literal `1` anywhere near the singleton seed.

### Pitfall 5: Torn rehearsal copy (WAL)

**What goes wrong:** `cp ~/.kamacu/kamacu.db /tmp/rehearsal.db` while the server is live copies the main file without the 4MB WAL — the copy is a stale/partial snapshot and the rehearsal proves nothing.
**Why it happens:** WAL mode keeps recent commits in `kamacu.db-wal` until checkpointed.
**How to avoid:** `sqlite3 ~/.kamacu/kamacu.db ".backup /tmp/rehearsal.db"` (online-backup; safe while live) — the recipe this session used successfully.
**Warning signs:** a rehearsal whose "before" snapshot shows fewer rows than the live DB.

### Pitfall 6: Backfill ordering

**What goes wrong:** `BackfillGlobalTask` wired before `BackfillAgents` on an install whose agents table was wiped — the singleton seed finds no default and errors (or silently no-ops), refusing/garbling boot.
**Why it happens:** the seed depends on the agents invariant.
**How to avoid:** wire it after `BackfillAgents` (and after `BackfillOpenCodeAgent` to keep the one-shots grouped — serve.go:186 is the insertion point).
**Warning signs:** serve.go diff placing the call before line 171.

### Pitfall 7: Over-touching the existing tmux writers "while we're here"

**What goes wrong:** scope creep — changing `sessions.go` INSERT/reconcile/reserve in this phase adds diff surface to hot paths with zero requirement coverage (global spawn is Phase 15).
**Why it happens:** the new column feels like it demands writer updates.
**How to avoid:** none are needed — the existing INSERT omits `scope` → DEFAULT 'task' → XOR holds (verified); every `WHERE task_id = ?` reader is NULL-safe by construction (verified audit). Phase 13's Go diff is: sweep query + backfill + delete-guard COUNT + wiring.
**Warning signs:** a Phase-13 diff touching `internal/api/sessions.go`.

## Code Examples

### The verified upgrade rehearsal script (SC1 — lift into the plan's verification task)

```bash
# Source: executed end-to-end this session (11/11 green) — the SC1 recipe
# 1. WAL-safe copy of the live install (server may stay up)
sqlite3 ~/.kamacu/kamacu.db ".backup /tmp/opencode/rehearsal-v112.db"
# 2. Snapshot the survival surface BEFORE
sqlite3 /tmp/opencode/rehearsal-v112.db \
  "SELECT id,task_id,n,name,label,created_at FROM tmux_sessions ORDER BY id" > before.txt
sqlite3 /tmp/opencode/rehearsal-v112.db "SELECT COUNT(*) FROM tasks;"              # 147
# 3. Apply the migrations via the real runner (tiny go run using store.Open+Migrate,
#    OR boot the real binary against the copy: kamacu serve --db /tmp/opencode/rehearsal-v112.db
#    — the binary path ALSO exercises BackfillGlobalTask wiring, proving SC2's boot half)
# 4. Snapshot AFTER + diff (modulo the new scope column) — must be byte-identical
```

### The sweep fix (GDATA-03 — one query at serve.go:369)

```go
// Source: cmd/kamacu/serve.go:365-369 (current) → verified replacement
// Known = a tmux_sessions row whose task STILL exists, OR any global-scoped row
// (task-less by design — GDATA-03). The JOIN drops rows whose task was deleted
// while down, so those sessions get swept too; globals must never be swept.
rows, err := db.QueryContext(ctx,
	`SELECT name FROM tmux_sessions WHERE scope = 'global' OR task_id IN (SELECT id FROM tasks)`)
// Discretion alternative (equivalent, LEFT JOIN shape):
// `SELECT ts.name FROM tmux_sessions ts LEFT JOIN tasks t ON t.id = ts.task_id
//  WHERE ts.scope = 'global' OR t.id IS NOT NULL`
```

Both shapes verified equivalent on real data (fixed-known = 11 = total, vs INNER JOIN's 10).

### The delete-guard extension (SC2's FK half — agents_crud.go)

```go
// Source: agents_crud.go:255-263 (projects guard, read verbatim) — add the second COUNT
var n int
if err := h.db.QueryRow(`SELECT COUNT(*) FROM projects WHERE agent_id = ?`, id).Scan(&n); err != nil { ... }
if n > 0 {
	writeError(w, http.StatusConflict, fmt.Sprintf("reassign its %d project(s) first", n))
	return
}
// NEW: the global task reference — friendly 409 ahead of the 00017 RESTRICT backstop
var g int
if err := h.db.QueryRow(`SELECT COUNT(*) FROM global_task WHERE agent_id = ?`, id).Scan(&g); err != nil { ... }
if g > 0 {
	writeError(w, http.StatusConflict, "reassign the global task's agent first")
	return
}
```

(Message string is planner's choice; keep it in the existing count-carrying grammar. The DB-level RESTRICT — verified on the real-install copy — is the backstop either way.)

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Table rebuild required for ANY NOT NULL change | `ALTER TABLE ... ALTER COLUMN ... SET/DROP NOT NULL` exists | SQLite 3.53.0 (2026-04-09) — **works on the pinned modernc v1.52.0 (3.53.2), probe-verified** | Could simplify this specific migration — but **LOCKED OUT by CONTEXT.md** (rebuild technique is a carry-forward decision) and it still cannot add the table-level XOR CHECK without a rebuild anyway. Documented for future phases only |
| goose migrations default-transactional | `-- +goose NO TRANSACTION` + explicit BEGIN/COMMIT for pragma-dependent migrations | In-repo since 00012 (v1.9) | The established discipline this phase reuses |
| `cp` for DB copies | online-backup (`.backup`) for live WAL databases | Always true; matters now (4MB WAL) | The rehearsal recipe |

**Deprecated/outdated:**
- `gorilla/websocket`-era patterns, unscoped `xterm` — irrelevant this phase (no frontend).
- Nothing in the pinned stack is deprecated for this phase's surface.

## Assumptions Log

> All claims in this research were verified or cited — with one deliberate exception flagged below.

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The `global_task` column names suggested (`root_path`, `github_repo`, `claude_session_id`, `opencode_session_id`) are the planner's to finalize — the rehearsal validated THIS spelling, so deviating from it re-runs the (documented) verification | Standard Stack / Pattern 1 | Low — any spelling works; the verified one is merely de-risked |

**No other [ASSUMED] claims** — schema mechanics, driver behavior, goose behavior, real-install state, and in-repo patterns are all [VERIFIED] via direct probe, official docs, or file:line reads.

## Open Questions

1. **Migration split: 00017+00018 (recommended) vs one combined file**
   - What we know: both work; the split keeps NO TRANSACTION scoped and is exactly what the rehearsal validated; CONTEXT defers this to plan-phase.
   - What's unclear: nothing material — planner preference only.
   - Recommendation: split (00017 plain-transaction singleton, 00018 NO TRANSACTION rebuild).

2. **Scope discriminator spelling: `scope TEXT` (recommended) vs `global INTEGER`**
   - What we know: both satisfy GDATA-02; the XOR CHECK was verified with `scope TEXT`; STACK.md originally sketched `global INTEGER` with `CHECK ( (task_id IS NULL) = (global = 1) )` (same truth table).
   - Recommendation: `scope TEXT` — self-documenting, extensible, verified.

3. **Delete-guard 409 message wording** for the global reference
   - Planner's call; existing grammar is `fmt.Sprintf("reassign its %d project(s) first", n)`.

4. **Where the sweep regression test lives** — `cmd/kamacu/sweep_test.go` (package main, next to the func) is the only option since `sweepOrphanTmux` is package-private; confirmed no existing sweep test file.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | all builds/tests | ✓ | go1.26.0 linux/amd64 | — |
| tmux (real binary) | GDATA-03 host-gated sweep regression | ✓ | 3.4 | `t.Skip("tmux not on PATH")` (house convention) |
| sqlite3 CLI | rehearsal copy (`.backup`) + snapshot tooling | ✓ | present on PATH | Go-side backup via modernc also possible |
| Real v1.12 install (`~/.kamacu/kamacu.db`) | SC1 rehearsal on a real-install copy | ✓ | goose v16; 147 tasks; 10 labeled tmux rows; 4 agents | Seeded-fresh staging (weaker but sufficient) — the automated test uses this |
| goose / modernc (module deps) | everything | ✓ | v3.27.1 / v1.52.0 (go.mod, module cache warm) | — |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** none.

## Security Domain

> `security_enforcement` is absent from `.planning/config.json` → enabled by default. This phase adds no external input surface (no new routes, no user-controlled input reaches new code paths) — its security contribution is *integrity by construction*.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | unchanged (local-only, loopback-enforced at serve.go:409) |
| V3 Session Management | no | unchanged (no session-surface changes this phase) |
| V4 Access Control | no | no new routes; singleton has no write surface until Phase 14 |
| V5 Input Validation | yes (DB-level) | The XOR CHECK + `CHECK (id = 1)` + FK RESTRICT are exactly DB-enforced input validation for every future writer — bad rows are unrepresentable regardless of caller |
| V6 Cryptography | no | no secrets stored (resume ids are opaque engine session ids, not credentials) |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Corrupt/ambiguous scope rows via a buggy future writer | Tampering | XOR CHECK rejects ambiguous + neither rows at the DB (verified, rc 275) — the constraint, not the caller, is the guarantee |
| Dangling agent reference on the singleton | Tampering | `ON DELETE RESTRICT` (verified) + handler COUNT 409 |
| Sweep killing legitimate live sessions (DoS on user shells) | Denial of Service | known-set includes all global rows by construction (verified query); degrade-don't-break posture unchanged (query failure → never kill blindly, serve.go:370-373) |
| Migration data loss on upgrade | Tampering/Destruction | 12-step procedure + byte-for-byte staged test + real-install rehearsal + goose version-table idempotence; boot refuses on migration error (serve.go:124-127) |

## Sources

### Primary (HIGH confidence)

- **Empirical probes on the pinned versions (this session, 2026-08-25)** — modernc.org/sqlite v1.52.0 (`sqlite_version()` = 3.53.2) + goose v3.27.1, scratch modules in /tmp/opencode/rebuild-probe and /tmp/opencode/rehearsal-run: 15/15 driver-level checks (rebuild mechanics, XOR CHECK both directions, NULL-distinct UNIQUE, FK re-arm, PRAGMA-in-tx no-op, ALTER COLUMN capability), full goose-runner migration green, **real-install rehearsal 11/11** (byte-for-byte tmux survival on `.backup` of the live DB).
- **Live codebase (this worktree, task/add-global-session-301 @ 28beaa2)** — read at file:line: migrations 00001/00005/00012/00013/00015/00016; `internal/store/{store,migrate}.go`; `internal/store/agents_migration_test.go`; `internal/api/agents_backfill.go`; `internal/api/workspaces.go:29`; `internal/api/agents_crud.go:226-275`; `cmd/kamacu/serve.go` (backfill wiring 117-204, sweep 343-403); `internal/api/sessions.go` (reconcile 136-214, mint/reserve 395-457, defaultTmuxLabel 610-622); `internal/reaper/reaper.go` (tmux queries 335/370); `internal/tmux/tmux.go`; `go.mod`.
- **Real install** `~/.kamacu/kamacu.db` (read-only): goose v16; 147 tasks; 10 tmux rows all custom-labeled; 4 agents, default=Claude(id=1); no views/triggers/other references to tmux_sessions; live 4MB WAL.
- **sqlite.org official docs** — lang_altertable.html (12-step procedure §8, RENAME semantics §2, ADD/ALTER COLUMN restrictions §4/§6, fetched 2026-08-25), lang_createtable.html (UNIQUE NULL-distinct §3.6, CHECK evaluation §3.7).

### Secondary (MEDIUM confidence)

- `.planning/research/{SUMMARY,ARCHITECTURE,PITFALLS,STACK}.md` — the v1.13 milestone research whose architecture this phase implements (all its file:line claims spot-verified during this session — consistent throughout).

### Tertiary (LOW confidence)

- None — no claim in this document rests on an unverified source.

## Project Constraints (from CLAUDE.md)

The repo-root CLAUDE.md is GSD-managed project context (no AGENTS.md exists). Directives relevant to the planner:

- **Tech stack constraints:** Go backend, React frontend, single binary serving API + static frontend at localhost, SQLite local storage, the app spawns the agent CLI (never reimplements). Phase 13 touches only the Go/SQLite side.
- **GSD workflow enforcement:** "Do not make direct repo edits outside a GSD workflow" — all phase work flows through `/gsd-execute-phase` on the generated PLAN.md files.
- **Stack pins** (CLAUDE.md technology tables): modernc.org/sqlite v1.52.0 with the "do not independently bump libc" caveat (not touched this phase — zero dep changes); goose v3.27.1.

## Metadata

**Confidence breakdown:**
- Migration mechanics: HIGH — empirically verified on the pinned driver AND goose AND a real-install copy; the SQL in this document ran green.
- Sweep fix: HIGH — bug demonstrated and fix verified against real data; test design follows green in-repo host-gated patterns.
- Backfill/delete-guard/wiring: HIGH — direct mirrors of read-at-file:line in-repo precedents.
- Open items: only planner-preference choices (split shape, spelling) — no unknowns.

**Research date:** 2026-08-25
**Valid until:** 2026-09-24 (stable domain — pinned deps, local schema; re-verify only if go.mod moves or SQLite/migrations change)
