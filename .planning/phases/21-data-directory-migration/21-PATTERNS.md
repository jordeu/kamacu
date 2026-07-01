# Phase 21: Data Directory Migration - Pattern Map

**Mapped:** 2026-07-01
**Files analyzed:** 13 (5 new, 8 modified)
**Analogs found:** 13 / 13 (every file grounds in an in-tree analog; only the localStorage one-shot leans on RESEARCH for the migration shape)

This phase is a **rename/migration**, not a greenfield feature. Most modified files are
**literal flips** of `~/.kangent` / `kangent` → `~/.kamacu` / `kamacu` and are their own
best analog (copy the existing line, change the string). The genuinely new code — the
two-part startup one-shot and the client localStorage migration — copies its shape from
`api.BackfillProjectIcons`, the `sweepOrphanTmux` startup sweep, and `worktree.gitRun`.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/migrate/migrate.go` (NEW) | service (startup one-shot) | file-I/O + batch | `internal/api/icons.go` `BackfillProjectIcons` + `cmd/kamacu/main.go:230` `sweepOrphanTmux` + `internal/tmux/tmux.go` `KillServer` | role-match (exact for one-shot shape) |
| `internal/migrate/paths.go` (NEW) | service (DB rewrite + git driver) | CRUD/transform | `internal/api/icons.go:184` collect-then-update + `internal/worktree/worktree.go:36` `gitRun` | role-match |
| `internal/migrate/migrate_test.go` (NEW) | test | — | `internal/store/store_test.go` (`t.TempDir`+`Open`+`Migrate`) + `internal/api/icons_test.go` | role-match |
| `internal/migrate/worktree_repair_test.go` (NEW) | test (git integration) | — | `internal/worktree/worktree_test.go` (skip-if-no-git git integration) | role-match |
| `web/src/lib/migrateStorage.ts` (NEW) | utility (client boot) | transform | `web/src/components/layout/AppLayout.tsx:14` localStorage idiom + RESEARCH §Code Examples | partial (no existing LS migration) |
| `cmd/kamacu/main.go` (MOD) | config/entrypoint | request-response (startup orchestration) | self (startup sequence lines 33-206) | exact |
| `internal/tmux/tmux.go` (MOD) | utility (tmux client) | event-driven | self (const `DefaultSocket`:22, `Config`:29) | exact |
| `internal/settings/settings.go` (MOD) | config/model | CRUD | self (`Defaults` map:31) | exact |
| `internal/api/projects.go` (MOD) | controller/service | CRUD | self (`reposBase`:195) | exact |
| `web/src/main.tsx` (MOD) | entrypoint | transform | self (`createRoot`:17) | exact |
| `web/src/components/layout/AppLayout.tsx` (MOD) | component | transform | self (`kangent.sidebar`:8) | exact |
| `web/src/components/layout/ActiveSessionsBar.tsx` (MOD) | component | transform | self (`kangent:sessions-bar-collapsed`:33) | exact |
| `web/src/components/board/ReviewColumn.tsx` (MOD) | component | transform | self (`kangent:review-collapsed`:56) | exact |

---

## Pattern Assignments

### `internal/migrate/migrate.go` (NEW — service, one-shot startup)

Holds the D-13 gate decision table (RESEARCH Pattern 1) and Part 1 (dir rename,
WAL-safe DB rename, tmux retire). Three analogs combine here.

**Analog 1 — one-shot idempotent hook shape:** `internal/api/icons.go:184-216` (`BackfillProjectIcons`)

The canonical "runs once, cheap no-op afterward, self-gates on observable state" shape.
Copy the function-level contract: a single exported entry point, an idempotency gate up
front, no error returned on the already-done path. RESEARCH Pattern 2 (derived-consistency
roll-forward) is exactly this gate discipline applied per step.

```go
// icons.go:184 — the shape to mirror: one exported func, self-gating query, no-op when done.
func BackfillProjectIcons(db *sql.DB) error {
	rows, err := db.Query(`SELECT id, name FROM projects WHERE icon_letters = '' OR icon_color = ''`)
	if err != nil {
		return err
	}
	defer rows.Close()
	// ... collect into a slice, close cursor, THEN issue UPDATEs (single-writer discipline) ...
}
```

**Analog 2 — tmux retirement (D-05/D-08):** `internal/tmux/tmux.go:119-129` (`KillServer`) — already idempotent (exit 1 = "no server" → `nil`). RESEARCH Pattern 4 calls it directly:

```go
// tmux.go:119 — exit 1 (no server) is idempotent success; call it on the OLD socket.
func (c Client) KillServer(ctx context.Context) error {
	err := c.run(ctx, append(c.BaseArgs(), "kill-server")...)
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return nil
	}
	return err
}
```
Usage per RESEARCH: `old := tmux.Client{Socket: "kangent", ConfPath: "/dev/null"}; _ = old.KillServer(ctx)`.

**Analog 3 — path resolution + gate inputs:** `internal/settings/home.go:10-19` (`ExpandHome`). The gate resolves `~/.kangent` / `~/.kamacu` and the `--db` default through this exact helper (already used at `main.go:60,124`):

```go
// home.go:10 — the only ~ expander; gate uses it for src/dst + custom-db detection.
func ExpandHome(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, strings.TrimPrefix(p, "~")), nil
	}
	return p, nil
}
```

**WAL-safe DB rename (D-12, RESEARCH Pattern 3):** reopen with `store.Open` (WAL DSN), checkpoint-TRUNCATE, close, rename. The `store.Open` DSN + `SetMaxOpenConns(1)` are load-bearing context:

```go
// store.go:12 — reuse this exact opener for the pre-rename checkpoint pass.
func Open(dbPath string) (*sql.DB, error) {
	dsn := "file:" + dbPath +
		"?_pragma=journal_mode(WAL)" + "&_pragma=busy_timeout(5000)" +
		"&_pragma=foreign_keys(1)" + "&_pragma=synchronous(NORMAL)" + "&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // single-writer discipline
	return db, db.Ping()
}
```

**Refuse-to-boot (D-11):** copy the `slog.Error` + `os.Exit(1)` idiom already used all over `main.go` startup (e.g. `main.go:51,62,72`): `slog.Error("refusing to start", "error", err); os.Exit(1)`.

---

### `internal/migrate/paths.go` (NEW — service, Part 2 DB rewrite + worktree repair)

Runs post-`store.Migrate`. Rewrites managed paths under the old root (D-15), drives
`git worktree repair`, deletes `kangent-*` tmux rows.

**Analog 1 — collect-then-update DB discipline:** `internal/api/icons.go:184-216` again. Because `store.go:23` sets `SetMaxOpenConns(1)`, you MUST close the SELECT cursor before issuing UPDATEs on the same connection. Same slice-then-UPDATE structure:

```go
// icons.go:196 — scan into a slice while the cursor is open ...
for rows.Next() {
	var id int64; var name string
	if err := rows.Scan(&id, &name); err != nil { return err }
	fills = append(fills, fill{id: id, letters: deriveLetters(name), color: pickColor()})
}
if err := rows.Err(); err != nil { return err }
// icons.go:207 — ... then UPDATE after the cursor is closed, '?' placeholders only.
for _, f := range fills {
	if _, err := db.Exec(`UPDATE projects SET icon_letters = ?, icon_color = ? WHERE id = ?`,
		f.letters, f.color, f.id); err != nil { return err }
}
```

For the managed-path rewrite, the self-gating SQL (RESEARCH Pattern 2 table) is a
`LIKE oldRoot||'/%'` predicate that naturally stops matching once rewritten — the same
"WHERE still-needs-work" gate as `icons.go:185`'s `WHERE icon_letters = '' OR icon_color = ''`.

**Analog 2 — the `managed` gate, NOT a blind string replace (D-15):** `internal/api/projects.go:511-515` establishes that `managed` is the single source of truth — "never derive it from the path (data-loss hazard)". The rewrite must gate on `managed=1` (+ old-root prefix), exactly as delete gates on the marker:

```go
// projects.go:511 — the marker is authoritative; the migration mirrors this gate.
var managed int
var clone string
err := h.db.QueryRow(`SELECT managed, repo_path FROM projects WHERE id = ?`, id).Scan(&managed, &clone)
// ...
if managed == 0 { /* never touch the directory */ }
```

`reposBase` (`projects.go:195`, `= "~/.kangent/repos/"`) defines what "under the old root"
means for managed clones; `settings.KeyWorktreeBase` default (`settings.go:31`) is the
`worktree_base` seam. Both are also literal-flip targets (see below).

**Analog 3 — git worktree repair driver:** `internal/worktree/worktree.go:36-49` (`gitRun`) is the arg-array git exec idiom (never a shell). The repair driver copies its `exec.CommandContext("git", "-C", repo, ...)` shape:

```go
// worktree.go:36 — the git exec idiom to copy for `git -C <repo> worktree repair <paths...>`.
func gitRun(ctx context.Context, repo string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		msg = strings.TrimPrefix(msg, "fatal: ")
		if msg == "" { msg = err.Error() }
		return "", fmt.Errorf("%s", msg)
	}
	return out.String(), nil
}
```

**`os.Stat`-filter before repair (RESEARCH Pitfall 2 / G):** a stale path makes `git worktree repair` exit 1. Filter new worktree paths through `os.Stat` first — the codebase already stats worktree dirs before operating on them:

```go
// projects.go:580 — the existing "stat before you git" idiom to reuse.
if _, statErr := os.Stat(wt.path); statErr != nil {
	// dir gone -> skip git gates for it (here: skip it from the repair arg list)
	continue
}
```

**Analog 4 — tmux-row cleanup (D-07/D-08):** `internal/reaper/reaper.go:335` is the exact `DELETE FROM tmux_sessions` idiom (FK order: tmux rows are child rows; delete them directly):

```go
// reaper.go:335 — copy this for `DELETE FROM tmux_sessions WHERE name LIKE 'kangent-%'`.
if _, derr := r.db.ExecContext(ctx, `DELETE FROM tmux_sessions WHERE task_id = ?`, t.id); derr != nil {
	slog.Warn("reaper: deleting tmux_sessions rows", "task", t.id, "error", derr)
}
```

---

### `cmd/kamacu/main.go` (MOD — entrypoint wiring + literal flips)

The startup sequence IS the integration point. Ordering is load-bearing (RESEARCH Pitfall 4):
**Part 1 must run BEFORE `os.MkdirAll` (line 65) and `store.Open` (line 70)**; **Part 2 runs
AFTER `store.Migrate` (line 77), in the `BackfillProjectIcons` slot (line 87).**

Current sequence to splice into (verified line numbers):

```go
// main.go:35  — FLIP the --db default (D-12).
dbFlag := flag.String("db", "~/.kangent/kangent.db", "path to SQLite database file")
//                                ^^^^^^^^^^^^^^^^^^^^  → "~/.kamacu/kamacu.db"

// main.go:60  — ExpandHome already resolves the db path (gate reuses this).
dbPath, err := settings.ExpandHome(*dbFlag)
// main.go:65  — >>> Part 1 (gate + preflight + dir rename + WAL-safe DB rename + tmux retire)
//               MUST run BEFORE this MkdirAll (creating ~/.kamacu defeats the "dst absent" gate).
if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil { /* ... */ }
// main.go:70  — DB opens at the NEW path only after Part 1's rename.
db, err := store.Open(dbPath)
// main.go:77
if err := store.Migrate(db); err != nil { /* ... */ }
// main.go:87  — <<< Part 2 hook slots HERE, right beside BackfillProjectIcons.
if err := api.BackfillProjectIcons(db); err != nil { /* ... */ }
```

Additional literal flips in this file:
- `main.go:96` — `"kangent-tmux.conf"` → `"kamacu-tmux.conf"` (D-12; note the file is regenerated every boot by `tmux.WriteConfig`, so this is the load-bearing flip, not the on-disk rename).
- `main.go:124` — `settings.ExpandHome("~/.kangent/worktrees")` → `"~/.kamacu/worktrees"`.
- `main.go:270` — the orphan-sweep prefix guard `strings.HasPrefix(name, "kangent-")` → `"kamacu-"` (after the socket flip, only `kamacu-*` sessions live on `-L kamacu`).

**The `sweepOrphanTmux` reconciliation (main.go:230-282) is the direct model for Part 1's
tmux retirement** — it already shows the "list live sessions, reconcile against DB rows,
kill the ones that shouldn't be there" flow, best-effort with warn-only failures:

```go
// main.go:234 — ListSessions → known-set from DB → kill unknown. Part-1 retirement is the
// coarser `KillServer` version of this (kill the whole old -L kangent server at once).
names, err := tmuxClient.ListSessions(ctx)
// ... build `known` from `SELECT ts.name FROM tmux_sessions ts JOIN tasks t ON t.id = ts.task_id` ...
for _, name := range names {
	if !strings.HasPrefix(name, "kangent-") { continue }
	if known[name] { continue }
	if err := tmuxClient.KillSession(ctx, name); err != nil { /* warn, continue */ }
}
```

---

### `internal/tmux/tmux.go` (MOD — const flips)

```go
// tmux.go:22 — FLIP the socket name (D-08: new sessions use -L kamacu).
const DefaultSocket = "kangent"   // → "kamacu"

// tmux.go:29 — FLIP the config header comment (cosmetic, brand consistency).
const Config = `# kangent-managed tmux config (regenerated at startup -- do not edit)
                  ^^^^^^^  → kamacu-managed
```
Session naming is `kangent-<task>-<n>` (built elsewhere from the socket/prefix — grep
`kangent-` in `internal/session/`). The exact-match `has-session` guard (`tmux.go:77-89`,
`"="+name`) MUST be preserved (`kamacu-1-1` must not match `kamacu-1-10`) — do not touch it,
only the string prefix changes.

---

### `internal/settings/settings.go` (MOD — default flip)

```go
// settings.go:31 — FLIP the worktree_base default (stored raw, expanded at use).
KeyWorktreeBase: "~/.kangent/worktrees/", // → "~/.kamacu/worktrees/"
```
Note (D-15): the *stored* `worktree_base` row (if present) is migrated by the Part-2
path rewrite in its **raw `~/.kangent/...` form**; this line only changes the code default
for fresh installs. `settings.Get` (`settings.go:48`) is the read-at-use path.

---

### `internal/api/projects.go` (MOD — managed-clone root flip)

```go
// projects.go:195 — FLIP the managed-clone root (D-02/D-15 old-root prefix).
const reposBase = "~/.kangent/repos/"  // → "~/.kamacu/repos/"
```
This is both a literal flip AND the definition of "under the old root" for the Part-2
managed `repo_path` rewrite. The `managed` gate semantics (`projects.go:511-515`) are the
authority for which rows get rewritten — never a blind string replace.

---

### `web/src/lib/migrateStorage.ts` (NEW — client one-shot) + `web/src/main.tsx` (MOD — call site)

**No existing localStorage-migration analog** — this is the one file that leans on RESEARCH
(§Code Examples, Pitfall 3) rather than an in-tree pattern. The keys are NOT uniformly
`kangent.*`: three shapes exist (`kangent.sidebar` dot, `kangent:sessions-bar-collapsed`
colon, `kangent:review-collapsed:<projectId>` dynamic), so a **prefix scan over all keys**
is required, keyed on the separator-agnostic `"kangent"` prefix.

```typescript
// RESEARCH §Code Examples — prefix scan, idempotent guard, run ONCE before createRoot.
export function migrateStorage() {
  if (localStorage.getItem("kamacu.storage-migrated") === "1") return;
  for (let i = 0; i < localStorage.length; i++) {
    const key = localStorage.key(i);
    if (!key || !key.startsWith("kangent")) continue;      // dot AND colon AND dynamic
    const newKey = "kamacu" + key.slice("kangent".length);
    if (localStorage.getItem(newKey) === null) {
      localStorage.setItem(newKey, localStorage.getItem(key)!);
    }
  }
  localStorage.setItem("kamacu.storage-migrated", "1");
}
```

**Call site (`main.tsx`):** invoke ONCE before `createRoot` (line 17) so components read the
new keys on first render:

```tsx
// main.tsx:17 — call migrateStorage() immediately above this line.
createRoot(document.getElementById("root")!).render(/* ... */);
```

The localStorage read/write idiom to match is `AppLayout.tsx:14-21` (the existing consumer):

```tsx
// AppLayout.tsx:8,14 — the getItem/setItem shape the migration and the flipped keys share.
const SIDEBAR_STORAGE_KEY = "kangent.sidebar"; // → "kamacu.sidebar"
const [open, setOpen] = useState(
  () => localStorage.getItem(SIDEBAR_STORAGE_KEY) !== "false",
);
```

---

### `web/src/components/layout/AppLayout.tsx` / `ActiveSessionsBar.tsx` / `ReviewColumn.tsx` (MOD — key-literal flips, D-14 / RESEARCH A4)

Flip the three code literals to `kamacu` (the copy migration populates the new keys; these
flips make the components READ them). Self-analog — change only the string:

```tsx
// AppLayout.tsx:8            "kangent.sidebar"                     → "kamacu.sidebar"
// ActiveSessionsBar.tsx:33   "kangent:sessions-bar-collapsed"     → "kamacu:sessions-bar-collapsed"
// ReviewColumn.tsx:56        `kangent:review-collapsed:${projectId}` → `kamacu:review-collapsed:${projectId}`
```

---

### `internal/migrate/*_test.go` (NEW — tests)

**Analog — temp-DB test harness:** `internal/store/store_test.go:8-9` is the idiom for a
unit test that opens + migrates a throwaway DB (`t.TempDir` + `store.Open` + `store.Migrate`):

```go
// store_test.go:9 — the temp-DB open idiom the migrate tests copy.
db, err := Open(filepath.Join(t.TempDir(), "test.db"))
```

- `migrate_test.go` — gate decision table (pure-function, mirror `icons_test.go`'s table-driven `TestDeriveLetters`), idempotency (run twice → second is a no-op, mirror the `WHERE ... = ''` gate), path-rewrite scope (managed vs unmanaged rows — assert unmanaged untouched, D-15), roll-forward (partial state → re-run completes).
- `worktree_repair_test.go` — git integration, skip-if-no-git, mirroring `internal/worktree/worktree_test.go` (build a `repos/`+`worktrees/` layout, rename, assert `worktree list` clean after repair). RESEARCH `scratchpad/wtrepro` already proved the mechanics.

---

## Shared Patterns

### One-shot idempotent startup hook (derived-consistency, no marker file)
**Source:** `internal/api/icons.go:184-216` (`BackfillProjectIcons`)
**Apply to:** `internal/migrate/migrate.go`, `internal/migrate/paths.go`
Every migration step self-gates on observable state (`LIKE oldRoot||'/%'`, `dst absent`,
`kangent-*` rows exist) so a mid-migration crash rolls forward on the next boot. This is
the exact `WHERE icon_letters = '' OR icon_color = ''` no-op-when-done discipline.

### Single-writer DB discipline (collect-then-update)
**Source:** `internal/store/store.go:23` (`SetMaxOpenConns(1)`) + `internal/api/icons.go:196-214`
**Apply to:** all DB rewrites in `internal/migrate/paths.go`
Close the SELECT cursor before issuing UPDATEs on the same connection. All SQL uses `?`
placeholders — never string-concatenate a path into the query.

### Arg-array exec, never a shell
**Source:** `internal/worktree/worktree.go:36-49` (`gitRun`) + `internal/tmux/tmux.go:69-73` (`run`)
**Apply to:** the `git worktree repair` driver and all tmux calls in `internal/migrate`
`exec.CommandContext("git"/"tmux", args...)` — no `sh -c`, no interpolation (security invariant, V5).

### Idempotent tmux verbs (exit 1 = success)
**Source:** `internal/tmux/tmux.go:119-129` (`KillServer`), `104-114` (`KillSession`)
**Apply to:** Part-1 tmux retirement in `internal/migrate/migrate.go`
"No server running" (exit 1) is treated as success, so retirement is safe to re-run.

### `~` expansion at the boundary
**Source:** `internal/settings/home.go:10-19` (`ExpandHome`)
**Apply to:** the D-13 gate (src/dst/custom-db detection) in `internal/migrate/migrate.go`
Values are stored raw with `~`; expand at use. The custom-`--db` opt-out compares the
resolved `--db` against `ExpandHome(defaultDbFlag)`.

### Refuse-to-boot on unrecoverable error
**Source:** `cmd/kamacu/main.go:51,62,72,79` (`slog.Error(...); os.Exit(1)`)
**Apply to:** D-11 preflight failure + D-13 both-dirs-present anomaly
Terminal-visible `slog.Error` naming both dirs + "data is safe", then `os.Exit(1)`.

---

## No Analog Found

No file is fully without an analog. The one partial case:

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `web/src/lib/migrateStorage.ts` | utility | transform | No existing localStorage-migration code in the repo. The localStorage read/write idiom (`AppLayout.tsx:14`) is the closest analog; the one-shot prefix-scan migration shape comes from RESEARCH §Code Examples / Pitfall 3. |

---

## Metadata

**Analog search scope:** `cmd/kamacu/`, `internal/{api,tmux,settings,store,reaper,worktree,session}/`, `internal/store/migrations/`, `web/src/{components,lib}/`, `web/src/main.tsx`
**Files scanned:** 12 Go source files + 5 web source files read; full `kangent` literal grep across `web/src` and Go seams
**Key facts grounding the map:**
- `~/.kangent` is NOT centralized — literals live in `main.go:35,96,124`, `settings.go:31`, `projects.go:195`, `tmux.go:22,29`, and 3 web files. No single constant to flip.
- Startup ordering is fixed: Part 1 before `main.go:65` MkdirAll / `:70` Open; Part 2 in the `:87` BackfillProjectIcons slot.
- localStorage keys use two separators + a dynamic suffix — prefix-scan mandatory.
**Pattern extraction date:** 2026-07-01
