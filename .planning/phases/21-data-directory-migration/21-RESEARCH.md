# Phase 21: Data Directory Migration - Research

**Researched:** 2026-07-01
**Domain:** One-time, gated, idempotent, failure-safe on-disk data migration (`~/.kangent` → `~/.kamacu`) at startup — Go stdlib file ops + git worktree repair + SQLite WAL handling + tmux socket switch + browser localStorage.
**Confidence:** HIGH — every mechanical claim below was verified empirically against the *actual* host tools (git 2.43.0, sqlite 3.45, go 1.26.0), the *actual* codebase seams, and the *live* `~/.kangent` install (9 managed repos, 29 task worktrees, 8 tmux rows, a hot 2.3 MB WAL).

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions (research the HOW, not the WHETHER)

- **D-01:** Physical move is `os.Rename(~/.kangent → ~/.kamacu)` — atomic, same-filesystem, no transient 2× disk. (Copy-then-swap rejected on disk-cost grounds — the live dir is 5.5 GB.)
- **D-02:** Preflight checks run BEFORE the rename (source exists, target absent, same filesystem, write perms/headroom). A preflight failure aborts before touching anything; `~/.kangent` left untouched with a clear error.
- **D-03:** The rename is the commit point. After it succeeds, remaining steps (`git worktree repair`, DB path rewrite, file renames) are idempotent.
- **D-04:** Roll forward, do not roll back. On a post-rename failure: leave dir at `~/.kamacu`, refuse to serve, log a clear error; next startup detects the migrated-but-incomplete state and re-runs the remaining idempotent steps. No rename-back.
- **D-05:** During migration, cleanly retire all live `kangent-*` sessions on the old `-L kangent` socket. No orphaned agent process left behind.
- **D-06:** Retired agent sessions come back via `claude --resume` on reopen (existing Phase 5 restart-recovery). Transcript persists.
- **D-07:** Retired bash/tmux shell sessions respawn fresh on the new socket; scrollback + foreground processes lost. Accepted (tmux cannot move a session between sockets).
- **D-08:** New sessions use `-L kamacu` + `kamacu-<task>-<n>`. Old `-L kangent` server is killed once its sessions are retired. DB rows reconciled the same way the existing startup orphan-sweep does.
- **D-09:** Silent server-side one-shot at startup, run before serving against the new paths — modeled on `BackfillProjectIcons` (runs once right after `store.Migrate`).
- **D-10:** Success is silent — a single `slog` info line (`migrated ~/.kangent → ~/.kamacu`), no UI toast/browser notification.
- **D-11:** Failure = refuse to boot + clear `slog` error (which dir is where, that data is safe, what to do). No degraded browser error page this phase.
- **D-12:** Rename DB file `kangent.db → kamacu.db` and tmux config `kangent-tmux.conf → kamacu-tmux.conf` (after the dir rename), and flip the `--db` default to `~/.kamacu/kamacu.db`.
- **D-13:** Trigger gate = default paths only. Migrate **iff** `~/.kangent` exists AND `~/.kamacu` absent AND user has not overridden `--db`. Custom `--db`/data path opts out entirely.
- **D-14:** One-time client-side localStorage migration on app boot: for each `kangent.*` key, if the corresponding `kamacu.*` key is absent, copy the value over; runs once, no-op afterward. Generic over the `kangent.*` prefix, not hard-coded to one key.
- **D-15:** Only DB paths **under the old data root** get rewritten: `tasks.worktree_path`, managed `projects.repo_path` (under `~/.kangent/repos/`), and the `worktree_base` setting. External/unmanaged repos left untouched. Distinguish via the `managed` flag / path-prefix check, not blind string replace.

### Claude's Discretion

- Exact preflight check set and error-message wording (D-02/D-11).
- Whether the migration lives in a new `internal/migrate` package vs an existing startup path — model on `BackfillProjectIcons` + startup-orphan-sweep hooks in `cmd/kamacu/main.go`.
- Mechanics of the file renames within the dir (order relative to `git worktree repair`), and how the old `-L kangent` server is killed (D-08).
- How the "migrated-but-incomplete" roll-forward state is detected on reboot (D-04).

### Deferred Ideas (OUT OF SCOPE)

- Degraded browser error page for migration failure — rejected this phase.
- Permanent dual-path support (reading both `~/.kangent` and `~/.kamacu`) — explicitly out of scope per REQUIREMENTS.md.
- Centralizing the data-dir path into one constant/config as a standalone refactor beyond what MIGRATE needs.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| MIGRATE-01 | One-time gated move of the data dir (`~/.kangent` → `~/.kamacu`) incl. managed repos, worktrees, SQLite DB | `os.Rename` verified atomic same-fs (Go 1.26); D-13 gate decision table (§Architecture Patterns Pattern 1); WAL-safe DB file rename (§Pitfall 1 — empirically proven) |
| MIGRATE-02 | Worktrees stay valid — `git worktree repair` + rewrite DB worktree/repo paths | **`git worktree repair` needs explicit NEW paths** (bare form is a no-op when both endpoints move — empirically proven, §Pitfall 2); DB path rewrite scope from live DB: 9 managed `repo_path` + 29 `worktree_path` rows, 0 unmanaged (§Runtime State Inventory) |
| MIGRATE-03 | tmux socket + prefix switch; live `kangent-*` sessions reconciled without orphaning an agent | **Agents never run under tmux** (bare PTY — die on restart; §Code Insight A); `tmux -L kangent kill-server` retires all old detached shells in one idempotent call (§Pattern 4); reconcile 8 `kangent-*` DB rows |
| MIGRATE-04 | Browser localStorage `kangent.*` → `kamacu.*` | **Keys use TWO separators (`kangent.` AND `kangent:`) plus a dynamic per-project key** — prefix-scan is REQUIRED, a hard-coded list is insufficient (§Pitfall 3, verified by grep) |
| MIGRATE-05 | Idempotent + failure-safe — skip when done/fresh, leave `~/.kangent` untouched on failure, clear error not half-state | Derived-consistency-check roll-forward (§Pattern 2) beats a marker file; preflight-before-rename (D-02) guarantees untouched-on-failure; refuse-to-boot on anomaly (§Pattern 1) |
</phase_requirements>

## Summary

This phase moves a live 5.5 GB data directory and re-wires four kinds of state that a filename grep will never surface: **git worktree two-way link files, an un-checkpointed SQLite WAL, detached tmux shell sessions on a dedicated socket, and browser localStorage keys.** Every one of these has a non-obvious failure mode that I reproduced and verified on this exact machine's toolchain. The good news: with `os.Rename` moving the whole tree atomically (D-01), the mechanics reduce to a small, precisely-ordered sequence of idempotent steps — no library is needed beyond the Go stdlib and the git/tmux CLIs the app already shells out to.

The three findings that most de-risk the plan: (1) **`git worktree repair` with NO arguments does nothing** when both the main repo *and* its linked worktrees move together (our exact case — both live under the renamed root); you must pass the *new* worktree paths explicitly, and `os.Stat`-filter them first (a stale path makes git exit 1). (2) **Renaming `kangent.db → kamacu.db` without carrying the `-wal`/`-shm` files — or checkpointing first — silently destroys all data** when the WAL is hot; the live DB has a 2.3 MB hot WAL right now (the process gets SIGKILL'd by Falcon per MEMORY.md, so a clean checkpoint-on-close is not guaranteed). (3) **The localStorage keys are NOT uniformly `kangent.*`** — there are `kangent.sidebar`, `kangent:sessions-bar-collapsed`, and a dynamic `kangent:review-collapsed:<projectId>`; D-14's literal wording ("generic over the `kangent.*` prefix") would miss the colon-separated and dynamic keys.

**Primary recommendation:** Implement as a two-part startup one-shot in `cmd/kamacu/main.go` modeled on `BackfillProjectIcons`. **Part 1 (before `store.Open`):** D-13 gate → preflight (incl. same-filesystem `Stat_t.Dev` check) → `os.Rename` dir → WAL-safe DB file rename (checkpoint-TRUNCATE then rename) → `tmux -L kangent kill-server`. **Part 2 (a hook after `store.Migrate`):** rewrite managed paths under the old root, `git worktree repair` each managed repo with `os.Stat`-filtered new paths, delete `kangent-*` tmux rows. Each step self-gates on observable state, so a mid-migration crash rolls forward on the next boot with no marker file. Refuse to boot (D-11) only on a preflight failure or the both-dirs-present anomaly.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Physical dir move + DB file rename | Backend startup (pre-DB) | Filesystem | Must run before `store.Open` so the DB opens at the final `~/.kamacu/kamacu.db` path |
| DB path-value rewrite (managed only) | Backend startup hook (post-Migrate) | Database | Needs an open+migrated DB; mirrors the `BackfillProjectIcons` idempotent-hook slot |
| `git worktree repair` | Backend startup hook (post-Migrate) | git CLI | Needs the rewritten new paths from the DB + filesystem; git is the only authority for the link files |
| tmux session retirement | Backend startup (pre/around DB) | tmux CLI | Old detached shells live in the tmux server, not the DB; `kill-server` on `-L kangent` is the authority |
| localStorage key migration | Browser / Client (app boot) | — | localStorage is per-origin browser state; only the client can read/rewrite it |
| Trigger gating / refuse-to-boot | Backend startup | — | Single-user local process; the terminal is the only UX surface (D-10/D-11) |

## Standard Stack

This phase adds **no new dependencies.** It uses the Go stdlib and the CLIs the app already drives.

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `os` / `path/filepath` (stdlib) | Go 1.26 | `os.Rename`, `os.Stat`, `os.MkdirAll`, `os.RemoveAll` for the dir + file moves | `os.Rename` is atomic on same-fs; already the app's idiom (see `projects.go` `os.RemoveAll`) `[VERIFIED: go run, Go 1.26.0]` |
| `syscall` (stdlib) | Go 1.26 | `Stat_t.Dev` for the same-filesystem preflight; `EXDEV` detection | Only portable way to detect cross-device before a rename `[VERIFIED: go run]` |
| `database/sql` + `modernc.org/sqlite` | v1.52.0 (in-binary) | WAL checkpoint + managed-path rewrite | Already the app's driver; `PRAGMA wal_checkpoint(TRUNCATE)` folds the WAL so the rename is safe `[VERIFIED: sqlite 3.45 repro]` |
| `internal/tmux` (existing) | — | `KillServer`, `ListSessions`, `HasSession` on the old `-L kangent` socket | `Client.KillServer` already treats "no server" (exit 1) as idempotent success (`tmux.go:119`) `[VERIFIED: tmux.go read]` |
| git CLI (system) | 2.43.0 | `git worktree repair <new-path>...` | The only tool that rewrites the worktree two-way link files `[VERIFIED: git worktree repair -h]` |
| `pressly/goose` (existing) | v3.27.1 | Schema only — **NOT** used for this data move | goose can't move files or repair worktrees; this is a runtime hook, not a migration (per CONTEXT §Established Patterns) |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `log/slog` (stdlib) | Go 1.26 | The single success line (D-10) + the refuse-to-boot error (D-11) | Already the app's logger |
| `internal/settings.ExpandHome` (existing) | — | Resolve `~/.kangent` / `~/.kamacu` / `~` to absolute paths | Already used at startup (`main.go:60,124`); the gate/preflight builds on it (`home.go`) |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `os.Rename` (D-01, locked) | copy-then-swap (`io/fs` walk + `os.RemoveAll`) | 2× transient disk (5.5 GB here) + non-atomic; rejected by D-01. Only relevant if the same-fs preflight fails (EXDEV) — then refuse to boot, don't fall back (copy is out of scope). |
| checkpoint-TRUNCATE then rename main | rename all three DB files (`kamacu.db` + `-wal` + `-shm`) | Both work; checkpoint-first is more roll-forward-safe (after it, the `-wal` is empty so even an interrupted rename can't strand data). See §Pitfall 1. |
| Derived-consistency roll-forward check | marker file (e.g. `~/.kamacu/.migrated`) | Marker adds its own failure mode (present-but-incomplete) and doesn't compose with "re-run remaining steps"; derived check is strictly better here. See §Pattern 2. |

**Installation:** None — no `go get` / `npm install`. Pure stdlib + existing internal packages + existing system git/tmux.

## Package Legitimacy Audit

**No external packages are installed by this phase.** All code uses the Go stdlib (`os`, `syscall`, `database/sql`, `path/filepath`, `log/slog`) and already-vetted in-tree dependencies (`modernc.org/sqlite` v1.52.0, `pressly/goose` v3.27.1, `internal/tmux`, `internal/settings`, `internal/worktree`) plus the system `git`/`tmux` CLIs. slopcheck / registry verification is **N/A** — nothing to audit. Confidence: HIGH.

## Architecture Patterns

### System Architecture Diagram

```
                       kamacu process starts (bin/kamacu)
                                    │
                                    ▼
                      flag.Parse()  (--db default = ~/.kamacu/kamacu.db)  [D-12 flip]
                                    │
                          ┌─────────▼──────────┐
                          │  D-13 GATE          │  inputs: customDb?, src=~/.kangent?, dst=~/.kamacu?
                          └─────────┬──────────┘
             ┌──────────────┬───────┼────────────┬─────────────────┐
        customDb=Y      src Y/dst N   src N/dst Y   src Y/dst Y   src N/dst N
        (opt out)       MIGRATE       roll-forward  ANOMALY       fresh install
             │              │           (finish)    (refuse boot) (nothing)
             │              ▼              │            │             │
             │      ┌───── PART 1 (pre-DB) ┴────────────┘             │
             │      │  preflight (src dir, dst absent, SAME-FS Dev,   │
             │      │            parent writable)  ──fail──▶ refuse boot (D-11)
             │      │  os.Rename(~/.kangent → ~/.kamacu)   ◀── COMMIT POINT (D-03)
             │      │  WAL-safe DB rename: open kangent.db, checkpoint(TRUNCATE),
             │      │       close, rename → kamacu.db  (+ kamacu-tmux.conf)
             │      │  tmux -L kangent kill-server  (idempotent)
             │      └──────────────┬──────────────────────────────────┘
             └──────────────┐      │                                   │
                            ▼      ▼                                   ▼
                       store.Open(~/.kamacu/kamacu.db)  ◀── DB opened at NEW path
                            │
                       store.Migrate (goose schema)
                            │
                     ┌──────▼─── PART 2 (post-Migrate hook, BackfillProjectIcons-style) ───┐
                     │  rewrite managed paths under old root → new root (D-15)              │
                     │     tasks.worktree_path, projects.repo_path(managed=1), worktree_base│
                     │  for each managed repo: git worktree repair <os.Stat-filtered new    │
                     │     worktree paths from DB>   (idempotent, §Pitfall 2)               │
                     │  DELETE FROM tmux_sessions WHERE name LIKE 'kangent-%'  (D-07/D-08)  │
                     └──────┬───────────────────────────────────────────────────────────────┘
                            │  slog.Info("migrated ~/.kangent → ~/.kamacu")   (D-10, once)
                            ▼
                   BackfillProjectIcons → tmux.WriteConfig(kamacu-tmux.conf) →
                   sweepOrphanTmux (on -L kamacu) → reaper → ListenAndServe

  Browser (separate tier, at app boot in web/src):
       for each localStorage key starting with "kangent": newKey = "kamacu"+rest;
       if localStorage[newKey] absent → copy value.  (D-14, prefix scan — §Pitfall 3)
```

### Recommended Project Structure

```
internal/migrate/            # NEW package (Claude's discretion — recommended over inlining in main.go)
├── migrate.go               # Gate decision table + Part 1 (dir rename, WAL-safe DB rename, tmux retire)
├── paths.go                 # Part 2: managed-path rewrite (D-15) + worktree repair driver
├── migrate_test.go          # gate table, idempotency, roll-forward, path-rewrite scope
└── worktree_repair_test.go  # git integration (skip-if-no-git), mirrors internal/worktree tests
cmd/kamacu/main.go           # wire Part 1 before store.Open; Part 2 hook after store.Migrate
web/src/lib/migrateStorage.ts  # NEW: one-shot prefix-scan localStorage migration (D-14)
web/src/main.tsx             # call migrateStorage() ONCE before createRoot (before any component reads a key)
```

### Pattern 1: Gate Decision Table (D-13 + D-04 + D-11)

**What:** A single pure function of three observable booleans decides the entire trigger + roll-forward + anomaly behavior. Keeps the "highest-risk" branch logic testable in isolation.

**When to use:** At startup, right after `flag.Parse`, before `store.Open`.

```go
// customDb: true when the user overrode --db to a non-default location (D-13 opt-out).
//   Detect by comparing the resolved --db path to ExpandHome(defaultDbFlag);
//   equal → default → eligible. Not-equal → custom → SKIP everything.
// src = DirExists(ExpandHome("~/.kangent")); dst = DirExists(ExpandHome("~/.kamacu"))
switch {
case customDb:            return SkipCustom          // D-13: never touch a bespoke layout
case src && !dst:         return DoMigrate           // fresh migration
case !src && dst:         return RollForward         // already-migrated OR finish remaining idempotent steps
case !src && !dst:        return FreshInstall        // store.Open creates ~/.kamacu fresh
case src && dst:          return RefuseBoot          // ANOMALY (D-11): both present on default paths — do not guess
}
```

- `RollForward` and `DoMigrate` both run the same idempotent Part-2 steps; `DoMigrate` additionally does the rename. This is what makes D-04 work with no marker file.
- `RefuseBoot`: `slog.Error` naming both dirs + "data is safe, resolve manually", then `os.Exit(1)`.

### Pattern 2: Derived-Consistency Roll-Forward (D-04) — recommended over a marker file

**What:** "Is migration complete?" is fully derivable from observable state, so each post-rename step self-gates and re-runs only its unfinished remainder. No `.migrated` marker file.

**When to use:** Every step in Part 2.

| Step | Self-gating predicate (re-run iff true) | Becomes a no-op when |
|------|------------------------------------------|----------------------|
| Dir rename | `src exists && dst absent` | `~/.kangent` gone |
| DB file rename | `~/.kamacu/kangent.db exists && kamacu.db absent` | only `kamacu.db` present |
| Managed-path rewrite | `EXISTS row WHERE (managed=1 AND repo_path LIKE old/%) OR worktree_path LIKE old/%` | 0 rows under old root |
| Worktree repair | (cheap idempotent no-op — just run it for every managed repo; clean when healthy) | always safe |
| tmux row cleanup | `EXISTS tmux_sessions WHERE name LIKE 'kangent-%'` | 0 old-named rows |

**Why better than a marker:** a marker can be present-but-work-incomplete (crash between marker-write and last step) or absent-but-work-done; the derived check can't disagree with reality. Verified empirically: `git worktree repair` on an already-healthy tree is a clean exit-0 no-op (§Pitfall 2 test E), and the SQL `LIKE` gates naturally stop matching once rewritten.

### Pattern 3: WAL-Safe DB File Rename (D-12) — checkpoint-first

**What:** Before renaming `kangent.db → kamacu.db`, fold the WAL into the main file so no committed data is stranded.

```go
// After os.Rename(dir) put kangent.db + kangent.db-wal + kangent.db-shm under ~/.kamacu.
db, _ := store.Open(filepath.Join(newRoot, "kangent.db"))   // WAL mode, as store.go does
db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")                   // fold + empty the WAL
db.Close()                                                  // SQLite removes -wal/-shm on clean close
os.Rename(newRoot+"/kangent.db", newRoot+"/kamacu.db")      // single file, now self-contained
os.Remove(newRoot+"/kangent.db-wal"); os.Remove(newRoot+"/kangent.db-shm") // defensive (already gone)
```

**Why:** proven below (§Pitfall 1) that a main-only rename with a hot WAL loses *everything*. The live DB has a 2.3 MB hot WAL right now.

### Pattern 4: tmux Retirement via `kill-server` (D-05/D-08)

**What:** Because **agents never run under tmux** (they're bare PTYs — §Code Insight A), the only live state on `-L kangent` is *detached bash/tmux shells left by the previous run*. A single `Client{Socket:"kangent"}.KillServer(ctx)` retires them all and kills the old server in one idempotent call — no per-session enumeration, so the `kangent-1-1`-vs-`kangent-1-10` exact-match guard is moot for retirement.

```go
old := tmux.Client{Socket: "kangent", ConfPath: "/dev/null"}
_ = old.KillServer(ctx)   // exit 1 (no server) is idempotent success (tmux.go:119)
// then in Part 2: DELETE FROM tmux_sessions WHERE name LIKE 'kangent-%'  (D-07: shells respawn fresh)
```

**Note:** on this machine the `-L kangent` server is already down (`no server running on /tmp/tmux-1000/kangent`), but 8 `kangent-*` rows persist — so the DB row cleanup matters even when `kill-server` is a no-op.

### Anti-Patterns to Avoid

- **Blind string replace of `~/.kangent` across the DB.** D-15: only `managed=1` repos and `worktree_path`/`worktree_base` *under the old root*. An unmanaged folder project could legitimately live at `/home/u/dev/kangent-something` — never rewrite it. Gate on the `managed` flag + prefix, exactly as `projects.go` gates managed deletes on the marker ("never derive it from the path — data-loss hazard", `projects.go:511`).
- **Bare `git worktree repair`.** No-op when both endpoints move (§Pitfall 2). Always pass new paths.
- **`os.MkdirAll(~/.kamacu)` / `store.Open(~/.kamacu/...)` before the rename.** Creating the target defeats the "dst absent" gate and makes `os.Rename` fail (target exists). The migration Part 1 must run *before* `main.go:65` MkdirAll and *before* `main.go:70` store.Open.
- **Renaming `kangent.db` alone with a hot WAL.** Destroys data (§Pitfall 1).
- **Renaming/keeping `kangent-*` tmux sessions as live.** They're killed, not moved; reattach must not think they exist.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Repair worktree link files | Manual rewrite of `.git` files + `.git/worktrees/*/gitdir` | `git worktree repair <new-paths>` | git owns the format; hand-editing both endpoints is fragile and version-specific |
| Fold the WAL | Manual `-wal` frame parsing / copy | `PRAGMA wal_checkpoint(TRUNCATE)` then rename, or move all three files | SQLite's crash-recovery invariant; parsing WAL frames is a data-loss generator |
| Atomic dir move | Recursive copy + verify + delete | `os.Rename` (D-01) | Atomic same-fs, no 2× disk; copy reintroduces the exact risk D-01 removed |
| Same-fs detection | Parse `/proc/mounts` | `syscall.Stat_t.Dev` compare | One stat call each; portable across the filesystems this app runs on |
| Retire detached shells | Track PIDs, `kill` process trees | `tmux -L kangent kill-server` | tmux already manages the process group; `kill-server` is atomic + idempotent |
| localStorage migration | Hard-coded key list | Prefix scan over `localStorage` keys | Dynamic per-project keys can't be enumerated ahead of time (§Pitfall 3) |

**Key insight:** every "moving part" in this phase already has a correct, idempotent primitive (git repair, WAL checkpoint, os.Rename, kill-server). The engineering is *ordering + gating*, not building.

## Runtime State Inventory

*(Rename/migration phase — all five categories answered explicitly. Grounded in the live `~/.kangent` install, read 2026-07-01.)*

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| **Stored data** | (1) **SQLite DB** `~/.kangent/kangent.db` + **hot 2.3 MB `-wal`** + 32 KB `-shm`. Contains: 9 `projects` all `managed=1`, `repo_path=/home/jordi/.kangent/repos/<owner>/<name>`; **29** `tasks.worktree_path=/home/jordi/.kangent/worktrees/<proj>/<slug>-<id>`; 8 `tmux_sessions` named `kangent-<task>-<n>`; settings rows `{github_integration, pr_review_seed, shell=tmux}` — **NO `worktree_base` row** (uses code default). (2) **git worktree link files** inside every managed repo (`.git/worktrees/<id>/gitdir` → absolute `~/.kangent/worktrees/...`) and each linked worktree's `.git` file (→ absolute `~/.kangent/repos/...`). (3) **Browser localStorage** keys under origin `localhost:7333` (see Live service config). | **Code edit + data migration.** WAL-safe DB file rename (Pattern 3). Rewrite 9 `repo_path` + 29 `worktree_path` (managed/under-root only, D-15). `git worktree repair` each of the 9 managed repos with new worktree paths. Paths are stored **expanded absolute** — rewrite the `<home>/.kangent` prefix; `worktree_base` (if ever present) is stored **raw `~/.kangent/...`** — handle that form too. |
| **Live service config** | tmux dedicated socket `-L kangent` at `/tmp/tmux-1000/kangent` (server currently **down**, but 8 `kangent-*` DB rows reference retired shells). Browser **localStorage** (3 key shapes): `kangent.sidebar`, `kangent:sessions-bar-collapsed`, `kangent:review-collapsed:<projectId>` (dynamic). No other external services — the app is local-only (no Datadog/n8n/Tailscale/Cloudflare). | `tmux -L kangent kill-server` (idempotent). Delete `kangent-*` tmux rows (D-07). Client-side prefix-scan localStorage migration (D-14, §Pitfall 3) **plus** flipping the 3 key literals in `AppLayout.tsx:8`, `ActiveSessionsBar.tsx:33`, `ReviewColumn.tsx:56` to `kamacu`. |
| **OS-registered state** | **None.** No systemd `--user` units, no user unit files, no crontab entries reference kangent/kamacu (verified). kamacu is launched manually from a terminal (`bin/kamacu`). The only OS-level artifact is the tmux socket file, retired by `kill-server`. | None — verified by `systemctl --user list-units`, `ls ~/.config/systemd/user`, `crontab -l`. |
| **Secrets / env vars** | **None reference the data path by name.** The per-instance hook token is `crypto/rand`-generated in memory each boot, never persisted (`main.go:107`). No `.env`/SOPS/CI var carries the old name. No shell env var references kangent (verified `env | grep`). | None. (The `X-Kangent-Token` HTTP header name in `agent.go` is a *code* string, Phase 20 territory / harmless — not a data-migration concern.) |
| **Build artifacts / installed pkgs** | Go module already `kamacu` (Phase 20). `Makefile` builds `bin/kamacu` (present). A **stale `./kangent` binary** (18 MB, Jun 14) sits at the repo root — a Phase 20 leftover, unrelated to the data dir. `kangent-tmux.conf` inside the data dir is regenerated every boot by `tmux.WriteConfig` (`main.go:96`). | Data-dir file: rename `kangent-tmux.conf → kamacu-tmux.conf` (D-12) — but note it's **regenerated at startup anyway**, so the rename is cosmetic; the code literal flip (`main.go:96`) is what matters, and the old file is harmless clutter. Stale `./kangent` binary: out of scope (optional `rm`). |

**The canonical question — after every file in the repo is updated, what runtime systems still have the old string cached, stored, or registered?** Answer: the SQLite DB (path values + tmux row names), the git worktree link files, and browser localStorage. All three are handled by the Part-2 hook + the client boot migration above. Nothing else.

## Common Pitfalls

### Pitfall 1: Renaming `kangent.db → kamacu.db` with a hot WAL destroys all data
**What goes wrong:** SQLite locates the WAL by appending `-wal` to the opened DB filename. Rename only the main file and the `-wal` is orphaned; SQLite opens a `kamacu.db` with no matching `kamacu.db-wal` and silently ignores every committed-but-uncheckpointed transaction.
**Empirical proof (this machine's sqlite 3.45, hot WAL forced via `os._exit` mid-transaction):**
- **Rename main only:** `sqlite3.OperationalError: no such table: t` — *the entire dataset was in the WAL and is lost.*
- **Rename all three (matching basenames) OR checkpoint-first:** 2/2 rows intact, incl. the hot-in-WAL row.
**Why it happens here specifically:** the live `~/.kangent/kangent.db-wal` is **2.3 MB and hot** right now — and per MEMORY.md the process is SIGKILL'd by Falcon EDR, so a clean checkpoint-on-close is not guaranteed.
**How to avoid:** Pattern 3 — `PRAGMA wal_checkpoint(TRUNCATE)` then rename the single self-contained file (roll-forward-safe: after truncate, an interrupted rename can't strand data). Alternative: rename all three (`kamacu.db`+`-wal`+`-shm`).
**Warning signs:** a test that only checks row counts after a *clean* close will pass (clean close checkpoints); you must force a hot WAL (kill mid-txn) to catch this. `[VERIFIED: sqlite 3.45 repro, scratchpad/waltest2]`

### Pitfall 2: Bare `git worktree repair` is a no-op when both endpoints move
**What goes wrong:** After `os.Rename(~/.kangent → ~/.kamacu)`, both the managed repo (under `repos/`) and its linked worktrees (under `worktrees/`) move together. Their two-way link files still point at the *old* absolute paths. `git worktree repair` with no args reads the stale main-side pointer, finds the old path gone, and can't guess the new location — the worktree stays `prunable` and the linked worktree is `fatal: not a git repository`.
**Empirical proof (git 2.43.0, faithful `repos/`+`worktrees/` layout, renamed dir):**
- **(C) bare `git worktree repair`:** worktree still `prunable`, linked tree still broken.
- **(D) `git worktree repair <NEW worktree path>`:** `repair: gitdir incorrect` + `repair: .git file broken` → both pointers fixed, `worktree list` clean, linked tree usable.
- **(E)** repairing an already-healthy tree: clean exit-0 no-op (idempotent).
- **(F)** multiple new paths in one invocation: all repaired.
- **(G)/(partial)** a **nonexistent** path → `error: not a valid path`, **exit 1**, but valid paths in the same call *are still repaired*.
**How to avoid:** For each managed repo, run `git worktree repair <p1> <p2> ...` passing the **new** (post-rewrite) `tasks.worktree_path` values, **`os.Stat`-filtered to existing dirs** (so git never sees a stale path and never exits 1 — matching the existing codebase idiom of stat-ing worktree dirs before operating, `projects.go:580`). The main repo must already be at its new path when you run this (it is — the dir moved). `[VERIFIED: git 2.43.0 repro, scratchpad/wtrepro]`

### Pitfall 3: localStorage keys are NOT uniformly `kangent.*`
**What goes wrong:** D-14 says "generic over the `kangent.*` prefix." But the actual keys (grep-verified) are:
- `kangent.sidebar` (`AppLayout.tsx:8`) — **dot**
- `kangent:sessions-bar-collapsed` (`ActiveSessionsBar.tsx:33`) — **colon**
- `kangent:review-collapsed:${projectId}` (`ReviewColumn.tsx:56`) — **colon + dynamic per-project suffix**

A migration keyed on the literal prefix `"kangent."` misses the last two; a hard-coded key *list* can't enumerate the dynamic review-collapsed keys.
**How to avoid:** iterate **all** localStorage keys and migrate any key whose name starts with `"kangent"` (separator-agnostic): `newKey = "kamacu" + key.slice("kangent".length)`, copy value iff `newKey` absent. Idempotent; optionally set a one-shot `kamacu.storage-migrated` flag. Also flip the 3 code literals to `kamacu` in the same phase (Phase 20 left them — they are runtime-compat state, deferred to MIGRATE-04). Run the migration in `main.tsx` **before** `createRoot` so components read the new keys. `[VERIFIED: grep web/src]`

### Pitfall 4: Migration Part 1 running after `store.Open`
**What goes wrong:** `main.go` currently does `os.MkdirAll(dir(dbPath))` then `store.Open(dbPath)`. With the flipped default (`~/.kamacu/kamacu.db`), doing this before the rename *creates* `~/.kamacu`, which (a) trips the "dst absent" gate on the next boot and (b) makes `os.Rename` fail (`ENOTEMPTY`/target exists).
**How to avoid:** Part 1 (gate + preflight + dir rename + DB file rename + tmux retire) must run *before* `main.go:65` (MkdirAll) and `main.go:70` (store.Open). Part 2 (DB rewrites + repair) runs *after* `store.Migrate` (needs the schema). `[VERIFIED: main.go read]`

### Pitfall 5: Cross-device rename (EXDEV) surfacing as a partial move
**What goes wrong:** If `~/.kangent` is a symlink/bind-mount to another filesystem, `os.Rename` returns `*os.LinkError{Err: syscall.EXDEV}` and moves nothing — but a naive impl might have already renamed the DB file or killed tmux.
**How to avoid:** preflight the same-filesystem check *before* any mutation: `Stat(src).Dev == Stat(filepath.Dir(dst)).Dev` (stat the *parent* of dst since dst doesn't exist yet). On mismatch, refuse to boot (copy-then-swap is out of scope, D-01). Detect the runtime error defensively with `errors.Is(err, syscall.EXDEV)`. `[VERIFIED: go run — Dev compare + errors.Is(EXDEV) both work]`

### Pitfall 6: Old kangent process still running during migration
**What goes wrong:** If the operator launches the new `kamacu` while the old `kangent` still holds `~/.kangent/kangent.db` open, the WAL is live and two processes diverge onto different paths.
**How to avoid:** the single-binary/single-user model means "upgrade = stop old, start new" (operational invariant). Document it in the refuse-to-boot copy. `os.Rename` on Linux will still succeed (inode-based) but the checkpoint could race — the operator must stop the old process first. Flag as an assumption (A1).

## Code Examples

### Same-filesystem preflight + EXDEV detection (D-02)
```go
// Source: verified with `go run` on Go 1.26.0 (scratchpad/fscheck)
import ("os"; "path/filepath"; "syscall"; "errors")

func sameFilesystem(src, target string) (bool, error) {
	si, err := os.Stat(src)               // ~/.kangent
	if err != nil { return false, err }
	ti, err := os.Stat(filepath.Dir(target)) // parent of ~/.kamacu (dst doesn't exist yet)
	if err != nil { return false, err }
	return si.Sys().(*syscall.Stat_t).Dev == ti.Sys().(*syscall.Stat_t).Dev, nil
}

// after os.Rename:  if errors.Is(err, syscall.EXDEV) { /* clean preflight-style refuse */ }
```

### WAL-safe DB rename (D-12, Pattern 3)
```go
// Source: verified with sqlite 3.45 hot-WAL repro (scratchpad/waltest2)
db, err := store.Open(filepath.Join(newRoot, "kangent.db")) // WAL mode per store.go
if err != nil { return err }
if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil { db.Close(); return err }
db.Close() // clean close removes -wal/-shm
if err := os.Rename(filepath.Join(newRoot,"kangent.db"), filepath.Join(newRoot,"kamacu.db")); err != nil { return err }
_ = os.Remove(filepath.Join(newRoot,"kangent.db-wal"))
_ = os.Remove(filepath.Join(newRoot,"kangent.db-shm"))
```

### Managed-path rewrite (D-15) — collect-then-update (single-writer discipline)
```go
// Mirror BackfillProjectIcons (icons.go:184): SetMaxOpenConns(1) means you MUST
// close the SELECT cursor before issuing UPDATEs on the same connection.
oldRoot := filepath.Join(home, ".kangent")  // expanded form, as stored
newRoot := filepath.Join(home, ".kamacu")
// projects: managed=1 AND repo_path under oldRoot  (D-15: never unmanaged)
//   UPDATE projects SET repo_path = newRoot||substr(repo_path, len(oldRoot)+1)
//     WHERE managed=1 AND repo_path LIKE oldRoot||'/%'
// tasks: worktree_path under oldRoot
//   UPDATE tasks SET worktree_path = newRoot||substr(...) WHERE worktree_path LIKE oldRoot||'/%'
// settings.worktree_base: stored RAW ('~/.kangent/...') — rewrite the '~/.kangent' prefix
//   (absent on this machine → no-op; handle defensively for other installs)
```

### git worktree repair driver (D-15 / MIGRATE-02)
```go
// For each managed repo, gather its tasks' NEW worktree_path values, os.Stat-filter,
// then one repair call (Pitfall 2). Treat exit 0 as success (worktree.gitRun idiom).
paths := existingWorktreePaths(db, repoID) // os.Stat filter — skip user-deleted dirs
if len(paths) > 0 {
	args := append([]string{"-C", newRepoPath, "worktree", "repair"}, paths...)
	// exec.CommandContext("git", args...).Run(); nonzero exit → real error (all paths pre-stat'd)
}
```

### localStorage prefix-scan migration (D-14 / Pitfall 3)
```typescript
// Source: verified key set via grep web/src. Run ONCE in main.tsx before createRoot.
export function migrateStorage() {
  if (localStorage.getItem("kamacu.storage-migrated") === "1") return;
  for (let i = 0; i < localStorage.length; i++) {
    const key = localStorage.key(i);
    if (!key || !key.startsWith("kangent")) continue;      // matches kangent. AND kangent: AND dynamic
    const newKey = "kamacu" + key.slice("kangent".length);
    if (localStorage.getItem(newKey) === null) {
      localStorage.setItem(newKey, localStorage.getItem(key)!);
    }
  }
  localStorage.setItem("kamacu.storage-migrated", "1");     // idempotent guard
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Hand-edit worktree `.git`/`gitdir` files | `git worktree repair [<path>...]` | git 2.30 (2021) | Available on host git 2.43.0; the supported way to fix moved worktrees |
| Rollback-on-failure migrations | Roll-forward + idempotent steps (D-04) | — (project decision) | No partial-undo logic to get wrong; matches the user's "simplest robust" bias |

**Deprecated/outdated:** none relevant — this phase uses long-stable primitives (`os.Rename`, `PRAGMA wal_checkpoint`, `git worktree repair`, `tmux kill-server`).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The old `kangent` process is stopped before the new `kamacu` runs the migration (single-user/single-binary upgrade invariant). | Pitfall 6 | A concurrently-running old process holding the WAL could race the checkpoint; document in the refuse-to-boot copy. LOW (operational norm). |
| A2 | `~/.kangent` and `~/.kamacu` share a filesystem (both under `~`). Preflight verifies this; assumption is only that the *common* case holds so migration isn't routinely refused. | Pattern 1 / Pitfall 5 | If `~/.kangent` is a mount to another device, migration refuses to boot (correct, but surprising). Preflight makes it explicit. LOW. |
| A3 | Deleting `kangent-*` `tmux_sessions` rows (vs leaving them as harmless dead refs) is the desired D-07 "respawn fresh" behavior, giving clean `Bash 1` numbering. | Pattern 4 | If wrong, reopened tasks number shells from the old `MAX(n)` (`Bash 4`) instead of `Bash 1` — cosmetic only. LOW. |
| A4 | Flipping the 3 localStorage key *code literals* to `kamacu` is in-scope for MIGRATE-04 (Phase 20 deliberately left runtime-compat strings). | Pitfall 3 | If out of scope, the copy migration populates `kamacu.*` keys that nothing reads. Confirm the literal-flip belongs here. LOW-MED. |

*These four are the only non-verified judgments; all mechanics above are `[VERIFIED]` by on-host repro.*

## Open Questions (RESOLVED)

1. **Should the DB-file rename carry all three files or checkpoint-first?** — **RESOLVED:** checkpoint-first (Pattern 3); adopted by 21-02 Task 2.
   - What we know: both prevent data loss (§Pitfall 1, both verified).
   - What's unclear: checkpoint-first needs an extra DB open/close before `store.Open`; rename-all-three doesn't but must order the three renames carefully for roll-forward.
   - Recommendation: **checkpoint-first** (Pattern 3) — after `TRUNCATE`, an interrupted rename cannot strand data, which is the cleanest fit for D-04 roll-forward.

2. **Where exactly do the Part-2 steps live — one post-Migrate hook, or split?** — **RESOLVED:** single post-`store.Migrate` hook; adopted by 21-03/21-04.
   - What we know: the DB rewrite needs schema (post-Migrate); repair needs the rewritten paths; tmux-row cleanup needs the DB.
   - Recommendation: one `migrate.CompletePaths(db)` hook called right after `store.Migrate` and before `BackfillProjectIcons`, mirroring the existing hook slot. Keeps ordering obvious and testable.

3. **Live E2E must not risk the real 5.5 GB install.** (STATE.md calls for a real-install verification.) — **RESOLVED:** copied-HOME UAT; adopted by 21-05.
   - Recommendation: because the D-13 gate keys on `ExpandHome("~")`, run the verification against a **copied HOME**: `cp -a ~/.kangent /tmp/kamacu-uat-home/.kangent` then `HOME=/tmp/kamacu-uat-home ./bin/kamacu`. This exercises the *real* default-path gate + real data (9 repos, 29 worktrees, tmux shells) with zero risk to `~/.kangent`. Verify: board loads, task view opens, an agent resumes, diff renders, `git status` in a migrated worktree is clean, a reopened bash tab spawns a fresh `kamacu-*` shell, and a **second boot is a clean no-op** (dir stays `~/.kamacu`, no re-migration). Then load the app and confirm `kamacu.*` localStorage keys populated from `kangent.*`.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| git CLI | `git worktree repair` (MIGRATE-02) | ✓ | 2.43.0 | — (repair unavailable pre-2.30; host is fine) |
| tmux | retire `-L kangent` sessions (MIGRATE-03) | ✓ | 3.4 (per code) | `KillServer` treats "no tmux/no server" as no-op |
| SQLite (modernc, in-binary) | WAL checkpoint + path rewrite | ✓ | 1.52.0 / SQLite 3.53 | — |
| Go toolchain | build | ✓ | 1.26.0 | — |
| Live `~/.kangent` install | E2E verification | ✓ | 5.5 GB, 9 repos / 29 worktrees / 8 tmux rows / hot 2.3 MB WAL | copy-HOME for safe UAT (OQ3) |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** none — all present.

## Security Domain

*(security_enforcement absent in config = enabled; this is a local, single-user, no-network migration — most ASVS categories are N/A.)*

### Applicable ASVS Categories
| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | Local-only single user; no auth surface added |
| V3 Session Management | no | tmux/agent sessions are OS processes, not auth sessions |
| V4 Access Control | no | No multi-tenant surface |
| V5 Input Validation | **yes** | Paths come from the DB, not user input; **prefix/`managed`-gated rewrite (D-15), never blind string replace**; all git/tmux calls use **arg arrays, never `sh -c`** (existing invariant in `worktree.go`/`tmux.go`) |
| V6 Cryptography | no | Hook token already uses `crypto/rand` (`main.go:107`); migration adds no crypto |

### Known Threat Patterns for a local data migration
| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Data loss via stranded WAL | Tampering / DoS (of user data) | Checkpoint-TRUNCATE before DB rename (Pitfall 1) |
| Partial/half migration | DoS (unbootable app) | Preflight-before-rename (D-02) + refuse-to-boot (D-11) + roll-forward (D-04) |
| Rewriting a user's external repo path | Tampering | `managed`-flag + old-root-prefix gate (D-15); unmanaged projects never touched |
| Command injection via task title / path in git args | Tampering | Arg-array exec only (existing `worktree.go`/`tmux.go` invariant) — no shell interpolation |

## Sources

### Primary (HIGH confidence — on-host empirical verification, 2026-07-01)
- **git 2.43.0 worktree-repair repro** (`scratchpad/wtrepro`) — proved bare repair no-op, path-arg repair fixes both endpoints, idempotency, multi-path, stale-path exit-1 with partial repair. Confirmed against the live layout (`~/.kangent/repos/seqeralabs/cloudinfo` + linked worktrees — absolute pointer files).
- **sqlite 3.45 hot-WAL repro** (`scratchpad/waltest2`) — proved main-only rename → total data loss; all-three / checkpoint-first → intact.
- **Go 1.26.0 preflight repro** (`scratchpad/fscheck`) — `Stat_t.Dev` same-fs check + `errors.Is(err, syscall.EXDEV)` + `os.Rename` atomic same-fs.
- **Live DB read** (safe copy of `~/.kangent/kangent.db*`) — 9 managed repos, 29 task worktrees, 8 `kangent-*` tmux rows, no `worktree_base` row, `shell=tmux`, all paths expanded-absolute.
- **Codebase seams read:** `cmd/kamacu/main.go` (startup sequence, `BackfillProjectIcons` slot, `sweepOrphanTmux`), `internal/tmux/tmux.go` (`KillServer`/exact-match/`ListSessions`), `internal/settings/{settings,home}.go` (`ExpandHome`, `worktree_base` default), `internal/api/{projects,icons}.go` (`reposBase`, `managed` gate, backfill model), `internal/store/{store,migrate}.go` (WAL pragmas, `SetMaxOpenConns(1)`), `internal/reaper/reaper.go` + `internal/session/{manager,agent}.go` (**agents are bare PTY, not tmux**), `internal/worktree/worktree.go`, migrations `00001/00002/00003/00005/00008`, `web/src/**` (localStorage keys via grep).
- **OS-state checks:** `systemctl --user list-units`, `~/.config/systemd/user`, `crontab -l`, `env` — no kangent/kamacu registration; `tmux -L kangent list-sessions` — no server running.

### Secondary (MEDIUM)
- SQLite WAL naming/recovery semantics (`-wal`/`-shm` derived from DB filename) — sqlite.org WAL doc, corroborated by the repro. `[CITED: sqlite.org/wal.html]`
- `git worktree repair` semantics for moved main+linked trees — git-worktree(1) man page, corroborated by the repro. `[CITED: git-scm.com/docs/git-worktree]`

### Tertiary (LOW)
- None — all load-bearing claims were reproduced on-host.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — no new deps; all primitives verified on-host.
- Architecture (gate table, two-part hook, roll-forward): HIGH — grounded in the real `main.go` sequence + `BackfillProjectIcons` precedent + derived-check proof.
- Pitfalls (WAL, worktree repair, localStorage, ordering, EXDEV): HIGH — each empirically reproduced.
- Assumptions (A1–A4): LOW-MED — operational/judgment calls flagged for confirmation.

**Research date:** 2026-07-01
**Valid until:** 2026-08-01 (stable primitives; re-verify only if git/sqlite/Go major versions change or the data-dir layout changes).
