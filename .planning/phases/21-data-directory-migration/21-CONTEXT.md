# Phase 21: Data Directory Migration - Context

**Gathered:** 2026-07-01
**Status:** Ready for planning

<domain>
## Phase Boundary

A **one-time, gated, idempotent, failure-safe** migration that moves the on-disk
data directory `~/.kangent` → `~/.kamacu` on first launch after the Phase 20
rebrand, keeping live worktrees, agent sessions, the SQLite DB, and browser
`localStorage` intact. A fresh or already-migrated install detects this and skips
safely.

Covers requirements MIGRATE-01..05:
- MIGRATE-01: move the data dir (managed repos, worktrees, SQLite DB)
- MIGRATE-02: keep worktrees valid — `git worktree repair` + rewrite DB paths
- MIGRATE-03: switch the tmux socket/prefix, reconcile live `kangent-*` sessions
- MIGRATE-04: migrate browser `localStorage` `kangent.*` → `kamacu.*`
- MIGRATE-05: idempotent + failure-safe (skip when done; clear error, no half-state)

**Not in this phase (already handled / deferred):** the code/binary/module/UI
rename (Phase 20, done); permanent dual-path support (explicitly out of scope —
this is a one-time move, not a permanent dual-mount); renaming the GitHub
repo/remote.
</domain>

<decisions>
## Implementation Decisions

### Failure-safety model (how the bytes move)
- **D-01:** Physical move is `os.Rename(~/.kangent → ~/.kamacu)` — atomic and
  instant on the same filesystem, **no transient 2× disk** (managed `repos/` +
  `worktrees/` can be many GB of real git checkouts, so copy-then-swap was
  rejected on disk-cost grounds).
- **D-02:** **Preflight checks run BEFORE the rename** — at minimum: source
  exists, target absent, same filesystem (rename is not cross-device), write
  permissions / enough headroom for the subsequent in-place work. A preflight
  failure aborts before touching anything, so `~/.kangent` is left untouched with
  a clear error (satisfies MIGRATE-05's "original untouched on failure").
- **D-03:** The **rename is the commit point.** After it succeeds, the remaining
  steps (`git worktree repair`, DB path rewrite, file renames) are **idempotent**.
- **D-04:** **Roll forward, do not roll back.** If a post-rename step fails, leave
  the dir at `~/.kamacu`, refuse to serve, and log a clear error; the next
  startup detects the migrated-but-incomplete state and re-runs the remaining
  idempotent steps to completion. No rename-back (which could itself fail and
  would need partial-DB-rewrite undo).

### Live tmux session reconciliation (MIGRATE-03)
- **D-05:** During migration, **cleanly retire** all live `kangent-*` sessions on
  the old `-L kangent` socket. Their worktree CWDs are about to move, so keeping
  them running in place is not viable. No orphaned agent process is left behind.
- **D-06:** Retired **agent** sessions come back via `claude --resume` when the
  user reopens the task — the existing Phase 5 restart-recovery path. The
  transcript persists across the retire, so no conversation is lost.
- **D-07:** Retired **bash/tmux shell** sessions respawn fresh on the new socket;
  their scrollback and any foreground processes are lost. **Accepted** as an
  unavoidable consequence of the socket switch (tmux cannot move a session
  between sockets).
- **D-08:** New sessions use the `-L kamacu` socket and a `kamacu-<task>-<n>`
  prefix. The old `-L kangent` tmux server should be cleaned up (killed) once its
  sessions are retired so it does not linger. DB rows for retired sessions are
  reconciled the same way the existing startup orphan-sweep does (marked
  dead / reconciled so reopen triggers resume).

### Trigger & error UX
- **D-09:** Migration is a **silent server-side one-shot at startup**, run before
  the app serves against the new paths — modeled on the existing
  `BackfillProjectIcons` idempotent startup hook (runs once right after
  `store.Migrate`).
- **D-10:** **Success is silent** — a single `slog` info line
  (`migrated ~/.kangent → ~/.kamacu`), **no UI toast / no browser notification.**
- **D-11:** **Failure = refuse to boot + clear `slog` error** (which dir is where,
  that data is safe, what to do). kamacu is launched from a terminal
  (`bin/kamacu`), so the line is visible where the user started it. No degraded
  browser error page for this phase (rejected as more work than warranted for a
  single-user local tool).

### DB / socket naming & trigger gate
- **D-12:** Rename the DB file `kangent.db → kamacu.db` and the tmux config
  `kangent-tmux.conf → kamacu-tmux.conf` as part of the migration (after the dir
  rename), and **flip the `--db` default** to `~/.kamacu/kamacu.db`. Full brand
  consistency down to the filenames.
- **D-13:** **Trigger gate = default paths only.** Migrate **iff** `~/.kangent`
  exists AND `~/.kamacu` is absent AND the user has not overridden `--db` to a
  non-default location. A custom `--db` / custom data path **opts out entirely** —
  a bespoke or side-by-side layout is never touched. This is the primary safety
  gate alongside D-02.

### localStorage migration (MIGRATE-04)
- **D-14:** One-time **client-side** migration on app boot: for each `kangent.*`
  key, if the corresponding `kamacu.*` key is absent, copy the value over; guarded
  so it runs once and is a no-op afterward (mirrors the server-side idempotent
  gate). Known key today: `kangent.sidebar` (in `AppLayout.tsx`); the migration
  should be generic over the `kangent.*` prefix, not hard-coded to one key.

### Path-rewrite scope (MIGRATE-02)
- **D-15:** Only DB paths **under the old data root** get rewritten from
  `~/.kangent` → `~/.kamacu`: `tasks.worktree_path`, managed `projects.repo_path`
  (those under `~/.kangent/repos/`), and the stored `worktree_base` setting
  (`~/.kangent/worktrees/`). **External / unmanaged repo checkouts that live
  outside `~/.kangent` are left untouched** — the user's own repos elsewhere on
  disk must not be rewritten. Distinguish via the `managed` flag / path-prefix
  check, not by blind string replace.

### Claude's Discretion
- Exact preflight check set and error-message wording (D-02/D-11).
- Whether the migration lives in a new `internal/migrate` package vs an existing
  startup path — planner/researcher's call; model it on the
  `BackfillProjectIcons` + startup-orphan-sweep hooks in `cmd/kamacu/main.go`.
- Mechanics of the file renames within the dir (order relative to `git worktree
  repair`), and how the old `-L kangent` server is killed (D-08).
- How the "migrated-but-incomplete" roll-forward state is detected on reboot
  (e.g., `~/.kamacu` present but a marker/consistency check fails) — D-04.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & roadmap
- `.planning/REQUIREMENTS.md` — MIGRATE-01..05 (the 5 requirements this phase
  delivers) and the milestone Out-of-Scope list (no permanent dual-path support).
- `.planning/ROADMAP.md` §"Phase 21: Data Directory Migration" — goal + the 5
  success criteria.
- `.planning/STATE.md` §"Migration seams (Phase 21)" and §"Planning grounding
  for v1.8" — the pre-computed seam inventory; treat as fact.

### Data-dir seams (all currently `~/.kangent` string literals — no single constant)
- `cmd/kamacu/main.go` — `--db` default `~/.kangent/kangent.db` (line ~35);
  `~/.kangent/worktrees` (line ~124); `kangent-tmux.conf` filename (line ~96);
  `BackfillProjectIcons` one-shot hook (line ~87) — the idempotent-startup model;
  startup orphan sweep (line ~184) — the tmux reconciliation model.
- `internal/settings/settings.go` — `KeyWorktreeBase` default `~/.kangent/worktrees/`
  (line ~31; stored raw in the settings table, expanded at use).
- `internal/api/projects.go` — `reposBase = "~/.kangent/repos/"` (line ~195);
  managed-repo ownership + `managed` flag semantics.
- `internal/store/migrations/00001_init.sql` — `projects.repo_path TEXT NOT NULL
  UNIQUE`; `00002_worktrees.sql` — `tasks.worktree_path TEXT`. These are the
  absolute-path columns to rewrite (managed only, per D-15).

### tmux seams
- `internal/tmux/tmux.go` — `DefaultSocket = "kangent"` (line ~22) → `-L kangent`;
  `Config` template `kangent-managed tmux config` (line ~29); session naming
  `kangent-<task>-<n>` and the exact-match `has-session` guard (lines ~78, ~103).
- `internal/reaper/reaper.go` — how tmux rows are reconciled via `HasSession`
  (mirror for D-08 retirement).

### localStorage seam
- `web/src/.../AppLayout.tsx` — `kangent.sidebar` key (and any other `kangent.*`
  keys) — the client-side migration target (D-14).

### Recovery precedent
- Phase 5 (v1.0) recovery model — `claude --resume` on reopen for dead agent
  sessions (D-06). See `.planning/milestones/v1.0-ROADMAP.md`.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`api.BackfillProjectIcons(db)`** (`internal/api/icons.go`, called
  `cmd/kamacu/main.go:87`): the template for a one-shot, idempotent startup hook
  that runs once after `store.Migrate` and is a cheap no-op on later boots. The
  migration hook should follow this exact shape and ordering discipline.
- **Startup orphan sweep** (`cmd/kamacu/main.go:184+`): reconciles tmux sessions
  vs DB rows via `HasSession` — reuse this pattern for retiring live `kangent-*`
  sessions (D-05/D-08).
- **`settings.ExpandHome`**: already used to resolve `~/...` paths at startup;
  the migration's path detection/gating builds on it.
- **`claude --resume` recovery** (Phase 5): the agent-session comeback path (D-06).

### Established Patterns
- **`~/.kangent` is NOT centralized** — it is duplicated string literals across
  `main.go`, `settings.go`, `projects.go`, and `tmux.go`. The rebrand-safe move
  is to introduce a single data-dir/socket source of truth (or migrate + flip all
  literals atomically); the planner must ensure no literal is missed.
- **Goose migrations** (`internal/store/migrate.go`, `migrations/*.sql`) handle
  SCHEMA changes; this on-disk data move is a **runtime startup hook**, NOT a
  goose migration (goose can't move files or repair worktrees). Path-value
  rewrites happen in Go against the opened DB, not in SQL DDL.
- **tmux exact-match guard** (`kangent-1-1` must not match `kangent-1-10`) — any
  session enumeration during retirement must preserve this guard.

### Integration Points
- Startup sequence in `cmd/kamacu/main.go`: the migration one-shot slots in around
  `store.Open` → **migrate data dir + flip paths** → `store.Migrate` →
  `BackfillProjectIcons` → tmux config write → orphan sweep. Exact ordering vs
  `store.Open`/`store.Migrate` matters (the DB must be opened at the NEW path
  after the dir rename) — a key planner decision.
- Frontend boot (`AppLayout.tsx` / app entry) for the `localStorage` copy (D-14).
</code_context>

<specifics>
## Specific Ideas

- The user consistently chose the **simplest robust** option at every fork
  (rename over copy, roll-forward over rollback, refuse-to-boot over degraded
  page, silent success). Bias planning toward minimal, terminal-visible,
  single-user-local ergonomics — not enterprise-grade migration UI.
- STATE.md flags this as **the highest-risk phase of v1.8** and calls for a **live
  end-to-end restart/reattach verification against a real `~/.kangent` install**
  (not just unit tests): migrate a populated dir, confirm the board/task view,
  sessions (resume), and diff still open, and confirm a second boot is a clean
  no-op.
</specifics>

<deferred>
## Deferred Ideas

- **Degraded browser error page** for migration failure — considered, rejected for
  this phase (refuse-to-boot + terminal log is enough for a single-user local
  tool). Could revisit if kamacu ever grows a browser-only launch story.
- **Permanent dual-path support** (reading both `~/.kangent` and `~/.kamacu`) —
  explicitly out of scope per REQUIREMENTS.md; this is a one-time move.
- **Centralizing the data-dir path into one constant/config as a standalone
  refactor** — the migration will effectively force this, but a broader
  path/config cleanup beyond what MIGRATE needs is not this phase's job.

None of these should be acted on in Phase 21.
</deferred>

---

*Phase: 21-data-directory-migration*
*Context gathered: 2026-07-01*
