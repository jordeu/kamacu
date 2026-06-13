# Architecture Research

**Domain:** v1.2 integration — Claude quota indicator + tmux-backed resumable shell tabs into the existing Kangent Go+React codebase
**Researched:** 2026-06-11
**Confidence:** HIGH for codebase integration points (every claim verified against the actual source at line level); MEDIUM for tmux behavioral specifics (stable, man-page-documented behavior, from training data); quota fetch mechanism deliberately abstracted behind a seam (parallel research owns it)

## Standard Architecture

### System Overview

Both features slot into existing seams. Nothing restructures; the architecture question is *which side of each existing boundary* the new logic lives on.

```
┌────────────────────────── Browser (React) ───────────────────────────────┐
│  BoardPage header ──┐                      TaskPage header ──┐            │
│                     ├─ QuotaIndicator (NEW, shared component)┘            │
│                     │    └─ useQuota() 60s poll + manual refresh          │
│  TaskPage tab strip (TabDef seam, EXISTS)                                 │
│    └─ tmux tabs: × → detach (not stop); detached ghosts; Resume          │
└───────────┬──────────────────────────────────┬───────────────────────────┘
            │ GET /api/usage (NEW)             │ /api/sessions (EXTENDED)
┌───────────▼──────────────────────────────────▼───────────────────────────┐
│                         Go server (internal/api)                          │
│  usage.go (NEW)          sessions.go (MOD)        worktrees.go/tasks.go   │
│   └─ proxies + caches     ├─ shell=tmux → mint     (MOD: kill-session     │
│      quota snapshot       │  name, persist row,     on cleanup/delete)    │
│                           │  SpawnOpts.TmuxName                           │
│                           └─ DB-derived detached-shell entries            │
│                              (Phase 5 reconciliation pattern)             │
├───────────────┬──────────────────────┬───────────────────────────────────┤
│ internal/quota│  internal/session    │  internal/tmux (NEW, exec wrappers)│
│ (NEW, DB-free │  (MOD: tmux command  │  HasSession / KillSession /        │
│  fetch+cache) │  construction only;  │  DetachClient / NewSessionArgs     │
│               │  stays DB-free)      │                                    │
├───────────────┴──────────────────────┴───────────────────────────────────┤
│  SQLite: migration 00005 tmux_sessions table (name persistence ONLY —    │
│  never status); settings KV unchanged (tmux is a new AllowedShells value)│
└───────────────────────────────────────────────────────────────────────────┘
        │                                      │
   ~/.claude credentials                  tmux server (daemon, own sid —
   → Anthropic usage endpoint             survives Kangent restarts and
   (mechanism: parallel research)         Session.Stop's /proc sweep)
```

### Component Responsibilities

| Component | Status | Responsibility | Implementation |
|-----------|--------|----------------|----------------|
| `internal/quota` | NEW | Read local claude credentials, fetch quota snapshot from Anthropic, cache with TTL + last-good fallback | DB-free service struct, `Fetch` behind an interface (parallel research plugs in the mechanism); mutex-guarded cache like `session.Manager` |
| `internal/api/usage.go` | NEW | `GET /api/usage` — serve cached snapshot, `?force=1` bypasses cache | Thin handler over `quota.Service`, registered in `cmd/kangent/main.go` |
| `internal/tmux` | NEW | Pure exec wrappers: `HasSession`, `KillSession`, `DetachClient`, `NewSessionArgs` | `os/exec` + arg arrays, `=name` exact-match targets; imported by BOTH `internal/session` and `internal/api` (no cycle: it imports neither) |
| `internal/session` (manager.go) | MOD | tmux command construction in the existing `KindBash` Spawn branch; `SpawnOpts.TmuxName`/`Label`; `Session.tmuxName` in `Info` | `tmux new-session -A -s <name> -c <dir>` instead of plain shell exec; manager stays DB-free |
| `internal/api/sessions.go` | MOD | Name minting + `tmux_sessions` row persistence at spawn (the `claude_session_id` precedent); reattach variant; detach endpoint; DB-derived detached entries in list | Handler owns ALL DB writes, exactly like the existing claude-session-id persist at sessions.go:166-170 |
| `internal/settings` | MOD | `"tmux"` in `AllowedShells` gated by `LookPath`; save-time validation | `AvailableShells()` filtering `AllowedShells`; `Validate` rejects tmux when absent |
| `internal/store` migration 00005 | NEW | `tmux_sessions(task_id, tmux_name UNIQUE, label, seq)` | Names/labels only — the DB NEVER records running/detached status (Phase 5 invariant) |
| `web/src/components/quota/QuotaIndicator.tsx` | NEW | Compact label + 5h bar; HoverCard popup with all quotas, reset times, "Updated Xm ago", refresh button | Self-contained, owns its `useQuota()` call (the `useAgentStatuses` pattern — no prop drilling) |
| `web/src/components/ui/hover-card.tsx` | NEW | shadcn primitive (NOT currently in `web/src/components/ui/` — only tooltip/dropdown/dialog/select/etc. exist) | `npx shadcn@latest add hover-card` (Radix HoverCard keeps content open while hovered, so the refresh button works) |
| `web/src/pages/TaskPage.tsx` | MOD | tmux-aware tab close (detach vs stop), detached ghost tabs, Resume affordance | Extends `visibleSessions` / `tabIds` / `handleCloseTab` |

## Recommended Project Structure

New and modified files only (everything else untouched):

```
internal/
├── quota/                       # NEW package — DB-free, mirrors session.Manager's
│   ├── quota.go                 #   "service struct + mutex + seam interface" shape
│   └── quota_test.go            #   (fake Fetcher; cache TTL + last-good tests)
├── tmux/                        # NEW package — exec wrappers only, zero state
│   ├── tmux.go                  #   HasSession/KillSession/DetachClient/NewSessionArgs
│   └── tmux_test.go
├── session/
│   ├── manager.go               # MOD: SpawnOpts{TmuxName, Label}; tmux cmd in KindBash branch
│   └── session.go               # MOD: Session.tmuxName field; Info.TmuxName JSON
├── api/
│   ├── usage.go                 # NEW: GET /api/usage
│   ├── sessions.go              # MOD: spawn mint/persist; detach handler; list merge
│   ├── settings.go              # MOD: entryFor uses settings.AvailableShells()
│   ├── worktrees.go             # MOD: kill-session in remove; detached count in get
│   └── tasks.go                 # MOD: kill-session in delete
├── settings/
│   └── validate.go              # MOD: AllowedShells += "tmux"; LookPath gate
└── store/migrations/
    └── 00005_tmux_sessions.sql  # NEW
web/src/
├── api/
│   ├── usage.ts                 # NEW: useQuota + useRefreshQuota
│   └── sessions.ts              # MOD: TermSession.tmuxName; useDetachSession; reattach spawn
├── components/
│   ├── quota/QuotaIndicator.tsx # NEW (indicator + popup in one file)
│   └── ui/hover-card.tsx        # NEW via shadcn CLI
└── pages/
    ├── BoardPage.tsx            # MOD: <QuotaIndicator/> in header right group
    └── TaskPage.tsx             # MOD: header mount + tmux tab semantics
```

### Structure Rationale

- **`internal/quota` as its own package:** keeps the Anthropic-facing code out of `internal/api` so the parallel-research mechanism (token refresh, endpoint shape) lands in one place; `usage.go` stays a thin HTTP adapter. DB-free by design — it needs nothing from SQLite.
- **`internal/tmux` as its own package:** both `internal/session` (spawn command) and `internal/api` (kill/probe at cleanup, reconciliation) need tmux execs. `session` must not import `api` (enforced dependency direction, session.go:6-7), and duplicating exec details in both would drift. A leaf package imported by both resolves it.
- **No new frontend route/page:** both features ride existing pages; the only new UI tree is the indicator component.

## Architectural Patterns

### Pattern 1: Server-side quota proxy with TTL cache + client poll (feature a)

**What:** The Go server fetches quota from Anthropic using the local `~/.claude` credentials and serves it at `GET /api/usage`; the frontend polls that endpoint every 60s with TanStack Query and renders server-reported staleness.

**Why server-side is forced, not chosen:**
1. Credentials live on the host (`~/.claude/.credentials.json` on Linux) — the browser cannot read them.
2. Anthropic's endpoints don't serve CORS for `localhost:7333`; a browser fetch dies preflight.
3. Precedent: the server already owns `~/.claude` access (`transcriptExists` globbing `~/.claude/projects`, internal/api/resume.go:16-25).

**Endpoint shape (design for the parallel research's "server fetches quota JSON using local claude credentials"):**

```
GET /api/usage          → 200 always (degrade, never error the UI)
GET /api/usage?force=1  → bypass server cache (manual refresh)

{
  "available": true,            // false ⇒ indicator renders muted "—" state
  "reason": "",                 // "no credentials" / "fetch failed" when unavailable
  "fetchedAt": "2026-06-11T..", // SERVER fetch time — drives "Updated Xm ago"
  "quotas": [
    { "id": "5h",      "label": "Current session", "utilization": 34, "resetsAt": "..." },
    { "id": "7d",      "label": "Weekly",          "utilization": 12, "resetsAt": "..." },
    { "id": "7d_opus", "label": "Weekly (Opus)",   "utilization": 5,  "resetsAt": "..." }
  ]
}
```

`internal/quota.Service` seam:

```go
// Fetcher is the parallel-research seam: whatever mechanism that research
// lands (OAuth usage endpoint, token refresh) implements this.
type Fetcher interface {
    Fetch(ctx context.Context) (*Snapshot, error)
}

type Service struct {
    mu      sync.Mutex
    fetcher Fetcher
    cached  *Snapshot     // last-good; served with stale fetchedAt on errors
    ttl     time.Duration // ~30s — halves the 60s poll, absorbs multi-tab fan-in
}
func (s *Service) Get(ctx context.Context, force bool) (*Snapshot, error)
```

**Caching split (server vs TanStack):** cache on the SERVER (TTL ~30s + last-good-on-error), poll on the CLIENT (plain `refetchInterval: 60_000`). Rationale: (1) multiple browser tabs each poll — server cache collapses them to one upstream call; (2) "Updated Xm ago" must be fetch-truth, not render-truth, so `fetchedAt` has to come from the server anyway; (3) rate behavior of the upstream endpoint is unverified — a server TTL is the safety valve. Manual refresh = mutation hitting `?force=1`, then `setQueryData` with the response (same shape as `useSpawnSession`'s write-then-invalidate, web/src/api/sessions.ts:38-48).

**Trade-offs:** a second cache layer to reason about; acceptable because both layers are trivially small and the failure mode (stale-but-labeled data) is exactly what the UI spec wants.

### Pattern 2: Handler-owns-persistence, manager-stays-DB-free (feature b — the `claude_session_id` precedent)

**What:** The tmux session name is minted and persisted by the HTTP handler; the session manager only receives it via `SpawnOpts` and constructs the command. This is byte-for-byte the Phase 4/5 pattern for `claude_session_id`: handler reads task row → spawns → `UPDATE tasks SET claude_session_id = ?` after spawn (internal/api/sessions.go:96-113, 164-170), while `Manager` (internal/session/manager.go:23-33) never touches `*sql.DB`.

**Spawn flow (shell setting == "tmux"):**

```go
// sessions.go create, in the existing `else` settings branch (sessions.go:148-157):
sh, _ := settings.Get(h.db, settings.KeyShell)   // read-at-use, EVERY spawn (SET-03)
if sh == "tmux" {
    // mint: seq = MAX(seq)+1 for task; name = fmt.Sprintf("kangent-task-%d-%d", taskID, seq)
    // INSERT INTO tmux_sessions(task_id, tmux_name, label, seq) BEFORE Spawn
    opts.TmuxName, opts.Label = name, fmt.Sprintf("Bash %d", seq)
} else {
    opts.Shell = sh                               // existing path, unchanged
}
```

```go
// manager.go Spawn, KindBash branch (manager.go:158-186):
if opts.TmuxName != "" {
    bin, err := exec.LookPath("tmux")            // early-fail posture, mirrors opts.Shell
    if err != nil { return nil, fmt.Errorf("tmux not found") }
    cmd = exec.Command(bin, tmux.NewSessionArgs(opts.TmuxName, dir)...)
    // args: "new-session", "-A", "-s", name, "-c", dir   (-A = attach-or-create,
    // which makes fresh-spawn and reattach the SAME command)
    // env: the existing minimal bash env (manager.go:177-185) — critically it
    // omits TMUX, so Kangent-inside-tmux can't trip the nesting guard, and
    // SHELL stays the $SHELL fallback (tmux's default-shell derives from it).
}
```

**Why the name is minted from a persisted per-task seq, not the in-memory `taskCounters`:** `Manager.taskCounters` (manager.go:30) is memory-only and resets on restart — a post-restart "Bash 1" would collide with a surviving `kangent-task-5-1` tmux session. The DB `MAX(seq)+1` is the restart-stable counter; the label is stored alongside so reattached tabs keep their name. (`SpawnOpts.Label` is a new field; the manager's label-minting switch at manager.go:203-214 uses it when non-empty.)

**Trade-offs:** one more `SpawnOpts` field and a label override; the alternative (manager mints + persists) breaks the DB-free invariant the codebase explicitly documents (manager.go:23-25).

### Pattern 3: DB-derived reconciliation with a liveness probe (feature b — the Phase 5 `resumable` pattern, probe swapped)

**What:** Detached/surviving tmux sessions are discovered exactly the way post-restart resumable agents are: rows in the DB that have NO manager entry, filtered through an existence probe. For agents the probe is `transcriptExists` (glob `~/.claude/projects/*/<uuid>.jsonl`, resume.go:16-25); for tmux tabs it is `tmux has-session -t =<name>` (exit 0 ⇒ alive). The two-pass shape of `agents.go status` (manager-derived pass at agents.go:66-113, then DB-derived pass at agents.go:121-152) is the template.

**Where it runs:** extend `sessionHandlers.list` (`GET /api/sessions?task_id=N`, sessions.go:36-52) — the query TaskPage already polls every 5s — to append synthetic entries after the manager snapshot:

```go
// For each tmux_sessions row of the task with no RUNNING manager session
// carrying that tmuxName:
//   tmux has-session -t =name  →  alive: append {id: "tmux:"+name, label,
//                                  status: "detached", tmuxName, taskId}
//                              →  dead:  DELETE the row (lazy self-heal — the
//                                  inner shell exited; mirrors how a missing
//                                  transcript silently drops resumable)
```

The DB never records "running" or "detached" — status is ALWAYS derived at read time (the migration-00001-through-00004 invariant called out at agents.go:117-121). Restart reconciliation therefore costs nothing: empty manager + rows + probe IS the whole story, no startup mutation pass.

**Exit-vs-detach disambiguation (the subtle bit):** the tmux *client* process exits with status 0 in BOTH cases — user detaches (client prints `[detached]`) and inner shell exits (session destroyed, client prints `[exited]`). The kangent `Session` (which wraps the client PTY) cannot tell them apart from the exit code. `has-session` after exit is the only reliable discriminator, and the lazy probe in the list path handles it with zero new lifecycle machinery: detach ⇒ probe true ⇒ "detached" entry appears; inner exit ⇒ probe false ⇒ row deleted, the exited manager session renders the normal bash exit banner until closed.

**Probe cost:** one `tmux has-session` exec per row per 5s poll, single-user localhost — negligible (same order as the transcript glob).

### Pattern 4: Close = detach, Stop = kill (feature b — semantics mapping onto existing affordances)

**What:** Today's two destruction affordances get distinct tmux meanings:

- **Tab × (routine close)** — today: `handleCloseTab` (TaskPage.tsx:247-250) → `useStopSession` → `POST /api/sessions/{id}/stop` (sessions.go:178-186) → `Session.Stop()` SIGTERM/SIGKILL sweep (session.go:382-416). For tmux tabs: NEW `POST /api/sessions/{id}/detach` → `tmux detach-client -s =<name>` → the client exits 0 → `waitExit` reaps normally → existing closing-tab machinery (`closingIds` → `visibleSessions` filter → `removeTab` + `DELETE /api/sessions/{id}`) runs unchanged. Next 5s poll shows the "detached" entry. The frontend branches on `session.tmuxName` (now in `Info` JSON) — it has the data; no server-side guessing about client intent.
- **Header Stop button (red, destructive, TerminalPane.tsx:313-323)** — keeps meaning "kill": the stop handler, when the session has a `tmuxName`, runs `tmux kill-session -t =<name>` first (destroys server-side session; client exits as a consequence), then deletes the `tmux_sessions` row. Without this, "Stop" would secretly leave the inner shell running forever.

**A safety footnote that makes this design honest:** plain `Session.Stop()` cannot kill a tmux-backed shell anyway — the tmux *server* daemonizes (own `setsid`), so it is invisible to the `/proc` session sweep (`sessionPGIDs` scans for `sess == sid` of the *client*, session.go:443-484, and the documented escape hatch at session.go:378-381 names exactly this case). Every kill path for tmux tabs MUST go through `tmux kill-session`; relying on signals is silently wrong.

**Reattach:** `POST /api/sessions` gains `{"task_id": N, "tmux_name": "kangent-task-5-1"}` — handler validates the row exists + `has-session` true + no RUNNING kangent session already holds that name (mirror of the one-agent-per-task 409 gate, sessions.go:117-124; two attached clients would fight over window size), then spawns with `SpawnOpts{TmuxName: name, Label: row.label, Cwd: worktree}` — `-A` attaches. Restart-resume is the same call; no separate endpoint.

## Data Flow

### Quota flow

```
[60s useQuota poll / manual refresh click]
    → GET /api/usage(?force=1)
    → quota.Service.Get: cache fresh? serve : Fetcher.Fetch (creds from ~/.claude)
         ├─ ok    → cache = snapshot, serve
         └─ error → serve last-good (stale fetchedAt) or available:false
    → QuotaIndicator: compact "Claude 5h ▓▓▓░" bar (board + task headers)
    → HoverCard popup: all quotas, reset times, "Updated Xm ago" from fetchedAt,
      refresh button → useRefreshQuota mutation → ?force=1 → setQueryData
```

### tmux tab lifecycle

```
SPAWN  (shell setting = "tmux", read-at-use in handler)
  POST /api/sessions {task_id} → mint seq/name → INSERT tmux_sessions row
  → Spawn{TmuxName} → tmux new-session -A -s =name -c <worktree> in PTY
  → tab renders via existing useSessions/TerminalPane, byte-identical WS path

CLOSE (×)                              INNER SHELL EXITS (`exit`)
  POST /sessions/{id}/detach             tmux destroys session; client exits 0
  → detach-client → client exits 0       → waitExit → exited banner (unchanged)
  → tab removed (existing closing flow)  → next list poll: has-session FALSE
  → next list poll: has-session TRUE       → row deleted (lazy self-heal)
    → "detached" entry → ghost tab/Resume

REOPEN / POST-RESTART RESUME           STOP (red button) & CLEANUP
  POST /sessions {task_id, tmux_name}    stop handler / DELETE worktree /
  → gate: row + has-session + no         DELETE task: tmux kill-session -t =name
    running holder → Spawn -A attaches   BEFORE wt.Remove / row delete
  → replay ring + resize jiggle           (StopAllForTask CANNOT reach the
    repaints (existing reattach path)      daemonized tmux server — see Pattern 4)
```

### Key data flows

1. **Quota:** server is the only Anthropic client; browser only ever talks to `/api/usage`. Staleness is server-truth.
2. **tmux identity:** name minted in handler → persisted in `tmux_sessions` → carried in `SpawnOpts`/`Info` → probed by `has-session` at every read. The kangent session uuid stays ephemeral; the tmux name is the durable key (exactly the `claude_session_id` split: ephemeral session vs durable resume key).
3. **Status:** never stored. Manager snapshot (running/exited) + probe (detached/gone) at read time.

## Scaling Considerations

Single-user localhost — scaling is not a concern. The only quantities that grow: one `has-session` exec per detached row per 5s poll (trivial), and one upstream quota fetch per 30s TTL window regardless of open tabs (the point of the server cache).

## Anti-Patterns

### Anti-Pattern 1: Fetching quota from the browser

**What people do:** call the Anthropic usage endpoint directly from React with a token read from "somewhere".
**Why it's wrong:** CORS blocks it; the browser can't read `~/.claude`; shipping credentials into JS widens the trust boundary the loopback-only design (main.go:38-41, 145-162) deliberately keeps narrow.
**Do this instead:** server proxy (`internal/quota` + `GET /api/usage`), Pattern 1.

### Anti-Pattern 2: Making the session Manager database-aware for tmux names

**What people do:** mint/persist the tmux name inside `Manager.Spawn` "since the counter lives there".
**Why it's wrong:** breaks the explicitly documented invariant (manager.go:23-25, session.go:6-7) that the manager owns PTYs and nothing else; every existing persistence concern (`claude_session_id`) lives in handlers.
**Do this instead:** handler mints + persists, `SpawnOpts.TmuxName` carries it in (Pattern 2).

### Anti-Pattern 3: A new `Kind` for tmux sessions

**What people do:** add `KindTmux` alongside `KindBash`/`KindAgent`.
**Why it's wrong:** every Kind branch in the codebase (one-agent gate sessions.go:117-124, agent status filter agents.go:46, frontend `s.kind !== "agent"` TaskPage.tsx:115, label minting, env policy) would need auditing; tmux tabs ARE bash-family in every one of those decisions — they differ only in command construction and close semantics.
**Do this instead:** `KindBash` + `tmuxName` field; behavior branches on `tmuxName != ""`.

### Anti-Pattern 4: Trusting signals to kill tmux-backed shells

**What people do:** assume `StopAllForTask` before worktree removal covers tmux tabs like it covers bash tabs.
**Why it's wrong:** the tmux server daemonizes into its own session — the `/proc` sweep can't see it (the documented setsid escape, session.go:378-381). Worktree removal would orphan live shells cwd'd inside a deleted directory — exactly the Pitfall-4 class the D-32 gate exists to prevent (worktrees.go:139-145, 192-199).
**Do this instead:** `tmux kill-session -t =<name>` for every task row in BOTH cleanup paths (worktrees.go `remove`, tasks.go `delete` at tasks.go:487-503), before `wt.Remove`/row delete; surface detached shells in the `GET /worktree` dialog payload (worktrees.go:130-135) so Gate 1 stays honest.

### Anti-Pattern 5: Storing tmux/quota status in SQLite

**What people do:** add a `status` column to `tmux_sessions`, or persist quota snapshots.
**Why it's wrong:** the DB-never-records-"running" invariant is what makes restart reconciliation a pure read (agents.go:117-121 spells it out). Stored status WILL go stale (kill -9, host reboot wiping tmux); the probe never lies.
**Do this instead:** rows store identity (name/label/seq) only; `has-session` is status. Quota stays in-memory (it's a cache of remote truth).

### Anti-Pattern 6: Prefix-ambiguous tmux targets

**What people do:** `tmux kill-session -t kangent-task-5-1`.
**Why it's wrong:** tmux target-session resolution falls back to prefix matching — `-t kangent-task-5-1` can match `kangent-task-5-12`. (MEDIUM confidence; man-page-documented `=` exact-match prefix.)
**Do this instead:** always `-t =<name>` in `internal/tmux`; one wrapper package means one fix point.

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| Anthropic usage endpoint | `internal/quota.Fetcher` interface; creds from `~/.claude` | Mechanism (URL, auth, refresh) owned by parallel research — design ONLY against the interface. Degrade to `available:false`, never 5xx the indicator |
| tmux binary | `internal/tmux` exec wrappers; `exec.LookPath` gates the settings option | Mirror the v1.1 LookPath posture (manager.go:159-174). Session names use only `[a-z0-9-]` (no `.`/`:` — tmux target separators) |

### Internal Boundaries (file-level integration map)

| Boundary / file | Change | Detail |
|-----------------|--------|--------|
| `cmd/kangent/main.go:103-115` | MOD | construct `quota.Service`, register `api.UsageRoutes(mux, qs)` next to `AgentRoutes` |
| `internal/api/usage.go` | NEW | `GET /api/usage` handler |
| `internal/settings/validate.go:16` | MOD | `AllowedShells = ["bash","tmux"]`; new `AvailableShells()` (LookPath filter); `Validate(KeyShell)` checks availability for tmux |
| `internal/api/settings.go:22-29` | MOD | `entryFor` options ← `AvailableShells()` — the SHELL-FUT-01 seam means ZERO frontend dropdown changes (SettingsPage.tsx:98-106 already renders server options) |
| `internal/store/migrations/00005_tmux_sessions.sql` | NEW | `task_id FK ON DELETE CASCADE, tmux_name TEXT UNIQUE, label, seq` |
| `internal/session/manager.go:44-55, 158-186, 203-214` | MOD | `SpawnOpts.TmuxName`, `SpawnOpts.Label`; tmux command construction; label override |
| `internal/session/session.go:65-96, 163-187` | MOD | `tmuxName` field; `Info.TmuxName` JSON |
| `internal/api/sessions.go:61-172` | MOD | create: tmux mint/persist + reattach variant (`tmux_name` body field + gates); list: DB-derived detached entries + lazy row GC; stop: kill-session for tmux; NEW `POST /api/sessions/{id}/detach` |
| `internal/api/worktrees.go:97-136, 143-212` | MOD | get: `detached_shells` count in dialog payload; remove: kill-session per row before `wt.Remove` |
| `internal/api/tasks.go:487-503` | MOD | delete: kill-session per row before row delete (CASCADE removes rows) |
| `web/src/api/usage.ts` | NEW | `useQuota` (refetchInterval 60s), `useRefreshQuota` |
| `web/src/api/sessions.ts:5-15` | MOD | `TermSession.tmuxName?`, `status` union += `"detached"`; `useDetachSession`; spawn mutation accepts `tmux_name` |
| `web/src/components/quota/QuotaIndicator.tsx` | NEW | indicator + HoverCard popup |
| `web/src/components/ui/hover-card.tsx` | NEW | shadcn CLI (`ui/` currently has NO popover/hover-card — verified) |
| `web/src/pages/BoardPage.tsx:46-50` | MOD | header right group: `<QuotaIndicator/>` beside New task |
| `web/src/pages/TaskPage.tsx:383-450, 109-126, 247-250, 264-341` | MOD | header mount (beside ellipsis); detached entries into `visibleSessions`/`tabIds`/TabDef ghosts; × branches detach-vs-stop on `tmuxName`; click/Resume → reattach spawn |

**Quota indicator mount decision:** there is NO shared app header — `AppLayout` (web/src/components/layout/AppLayout.tsx:51-71) renders only the sidebar + a bare `<main><Outlet/></main>`; each page builds its own header (BoardPage's title/New-task row, TaskPage's title/ellipsis row). An AppLayout absolute overlay (the `CollapsedSidebarTrigger` pattern, AppLayout.tsx:22-36) would collide with BoardPage's right-aligned New-task button. Recommendation: ONE shared `QuotaIndicator` component mounted in BOTH page headers' right groups (matches PROJECT.md's "board and task view" scope exactly); TanStack Query dedupes the poll across mounts — the established `useAgentStatuses` multi-consumer pattern (web/src/api/agents.ts:14-22).

**Detached-tab UX (flag for roadmap/UX decision, two viable shapes):** (a) detached sessions render as muted ghost tabs in the strip (TabDef already supports `muted`, TaskTabs.tsx:27), click reattaches — most discoverable; or (b) the `+` button becomes a split control listing detached sessions + "New". Either way the data flow is identical (detached entries in the sessions list); recommend (a) for continuity with the existing muted-exited-tab idiom.

### Suggested Build Order

The two features are fully independent — they can be separate phases executed in either order or parallel.

**Quota (blocked only by the parallel quota-mechanism research):**
1. `internal/quota` service with fake-Fetcher tests + real Fetcher per research → `internal/api/usage.go` → `main.go` wiring.
2. `shadcn add hover-card` → `web/src/api/usage.ts` → `QuotaIndicator` → mount in BoardPage + TaskPage.

**tmux (strictly ordered by dependency):**
1. Settings seam: `AllowedShells`/`AvailableShells` + validation + `entryFor` (smallest slice; dropdown shows tmux immediately, server-data-only).
2. `internal/tmux` wrappers + migration 00005 (no behavior change yet).
3. Spawn path: `SpawnOpts.TmuxName`/`Label` + manager command construction + `Info.TmuxName` (a tmux tab now works end-to-end through the untouched WS/terminal stack — verifiable milestone).
4. Lifecycle: handler mint/persist, detach endpoint, tmux-aware stop, list-merge reconciliation + lazy GC, reattach spawn variant.
5. Cleanup integration: worktree remove + task delete kill-sessions, dialog count.
6. Frontend: types, ×-branching, ghost tabs/Resume, restart-resume verification.

### What v1.2 Must NOT Touch

- **`internal/ws`** — detach and inner-exit both ride the existing `Done()`/`'x'`-frame machinery (handler.go:78-130); zero protocol changes.
- **Manager/DB separation** — no `*sql.DB` anywhere in `internal/session`; no settings reads in the manager (settings are read-at-use in handlers, sessions.go:138-157, and stay there).
- **Agent paths** — `agent.go`, hooks, `agents.go` status, resume gates: tmux is bash-family only; the agent status model is untouched.
- **Existing migrations / settings rows** — 00005 is append-only; `shell` stays the same KV key, "tmux" is just a new value (absent row still defaults to "bash").
- **`TerminalPane` / `useTerminalSocket`** — a tmux client is just another full-screen PTY program; the replay ring + first-resize jiggle (session.go:339-359) already handles repaint-after-reattach.

## Sources

- Primary: the Kangent codebase at `/home/jordi/workspace/github/kangent` (all file:line citations above verified by direct read, 2026-06-11) — HIGH
- `.planning/PROJECT.md` + `.planning/milestones/v1.1-ROADMAP.md` — milestone scope, v1.1 settings-wiring precedent — HIGH
- tmux behavior (server daemonization/own sid; `new-session -A`; `detach-client`; `has-session`; client exit 0 on both detach and session-destroy; `=` exact-match target prefix; no `.`/`:` in session names; TMUX nesting guard): tmux(1) man page semantics, stable across 2.x–3.x — MEDIUM (training data; flag for a 5-minute empirical smoke test against the host's tmux during phase planning)
- Quota fetch mechanism: deliberately NOT researched here — parallel research owns it; this document designs only the `quota.Fetcher` seam and HTTP contract around it

---
*Architecture research for: Kangent v1.2 — quota indicator + tmux-backed resumable shells*
*Researched: 2026-06-11*
