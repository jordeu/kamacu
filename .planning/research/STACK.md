# Stack Research

**Domain:** A "Global Task" scratchpad added to an existing local-only Go + React app (Kamacu) — a task-like view with agent + bash tabs running directly in a configured repo/folder, not linked to any workspace/project/worktree.
**Researched:** 2026-08-25
**Confidence:** HIGH — the central claim ("zero new dependencies") was verified against the actual source: `go.mod`, `web/package.json`, every touched integration point read at file:line level in the working tree. External checks (pkg.go.dev / release mirrors via web search, 2026-08-25) confirmed no pinned dependency has drifted in a way this milestone cares about.

## TL;DR for the roadmap author

- **Zero new dependencies. Zero required version bumps.** Every capability the Global Task needs — PTY sessions with replay/detach-reattach, engine-branched agent spawn (claude/opencode/custom), restart-resume, managed `gh repo clone`, folder-path validation, settings UI, terminal rendering, the Active Sessions bar — already ships in the pinned stack (`go.mod`, `web/package.json`, read 2026-08-25). The milestone is a **composition milestone**: the work is schema design + API widening + frontend routing, not libraries.
- **One migration IS required (00017).** Two things cannot be done without touching the schema: (1) persistent restart-resume state and an FK-validated default agent need a home that isn't the `tasks` table, and (2) `tmux_sessions.task_id` is `NOT NULL REFERENCES tasks(id)` (migration 00005, read verbatim) — global bash tabs cannot persist restart-surviving rows until that FK admits a global scope. Recommended shape: a `global_task` singleton table + a `tmux_sessions` rebuild that makes `task_id` nullable with a `global` discriminator.
- **The session engine needs no fork — only an additive discriminator.** `session.SpawnOpts.Cwd` is already a free string and `TaskID` is just a scoping label the API layer sets (`internal/session/manager.go:54-80`); the engine never cares whether the cwd is a worktree. All task-scoping that must widen lives in `internal/api` (sessions.go create gate, agents.go status JOIN, tmux row minting) — exactly where v1.4/v1.10 widened it before.
- **The v1.4 managed-clone sequence is reusable verbatim** for the "GitHub repo as global root" path: `github.ParseRepoRef → github.ValidateRepo → github.Clone` into `~/.kamacu/repos/<owner>/<name>` with reattach-or-refuse (`internal/github/clone.go:38`, `internal/api/projects.go:357-449`).
- **Do NOT smuggle the global task through the `tasks` table** (hidden project row or `source='global'` task). Board queries, position math, v1.12 activity/statistics JOINs (`internal/api/activity.go:134`), and workspace scoping all assume real tasks; exclusion guards would leak into every future feature. A dedicated singleton is one table + one seed row.

---

## Recommended Stack

### The verdict: add nothing

| Layer | Pinned (read from manifest, 2026-08-25) | Role in THIS milestone | Change |
|-------|------------------------------------------|------------------------|--------|
| Go 1.26 + stdlib `net/http` ServeMux | `go.mod` | New `/api/global` routes, widened `POST /api/sessions` | none |
| `github.com/creack/pty` | v1.1.24 | PTY for global agent/bash sessions (cwd-agnostic) | none |
| `github.com/coder/websocket` | v1.8.14 | Browser terminal attach for global sessions — the WS handler is scope-blind (keyed by session id) | none |
| `modernc.org/sqlite` + `github.com/pressly/goose/v3` | v1.52.0 / v3.27.1 | Migration **00017** (the only schema work) | none |
| `github.com/google/uuid` | v1.6.0 | Claude `--session-id` minting for global agent spawns | none |
| `internal/session` (in-repo) | — | `SpawnOpts{Cwd, Kind, AgentEngine, AgentArgs, ResumeSessionID, TmuxName…}` already parameterizes everything a global spawn needs (`manager.go:54-80`) | +1 additive field (below) |
| `internal/github` (in-repo) | — | `ParseRepoRef` / `ValidateRepo` / `Clone` / `reattachManaged` — the v1.4 managed-root path, reused for the global root | none |
| `internal/settings` (in-repo) | — | `ExpandHome` + KV store — still used for shell/tmux read-at-use on global bash spawns | none |
| React 19 + react-router 7 + TanStack Query 5 | `web/package.json` | New `/global` route, `useGlobalConfig` query, Settings section, bar entry | none |
| `@xterm/xterm` 6 + fit/webgl addons | 6.0.0 / 0.11.0 / 0.19.0 | `TerminalPane` + `useTerminalSocket` are session-id-keyed — scope-blind | none |
| shadcn/ui + Tailwind 4 | CLI / 4.3.0 | Settings section + global page chrome reuse existing primitives | none |

**Nothing to install.** The "installation" step of the template is intentionally empty — that IS the finding, and it is verified rather than assumed: every primitive listed above was read in the working tree, not recalled from memory.

### The real stack work (what the roadmap must schedule)

The feature's cost lives in four composition seams. These are stack decisions (they constrain every plan), so they belong here even though they're not libraries.

#### 1. SQLite migration 00017 — `global_task` singleton + `tmux_sessions` scope

**Persistent state the Global Task needs and where it can live:**

| State | Why it must be a column, not a settings KV row |
|-------|------------------------------------------------|
| `agent_id` (default agent) | The v1.10 pattern is a real FK (`projects.agent_id … ON DELETE RESTRICT`, migration 00013) backing the block-until-unassigned delete guard (`agents_crud.go:255`). A KV string has no integrity, no RESTRICT, and the guard can't see it. |
| `claude_session_id` / `opencode_session_id` | Restart-resume keys written on EVERY spawn and read by the status endpoint's DB-derived pass (`agents.go:164-168`). These are hot entity state, not slowly-changing config; the settings table's contract is the opposite (`settings.go` package doc). |
| `repo_path` / `managed` / `github_repo` | Mirrors `projects` v1.4 columns so the reattach/dedup/gated-delete logic reads one shape. |

**Recommended migration shape** (adapted from the proven 00012/00013 seeding pattern):

```sql
-- 00017_global_task.sql (illustrative — exact DDL is plan-phase)
CREATE TABLE global_task (
  id                  INTEGER PRIMARY KEY CHECK (id = 1),  -- singleton: exactly one row, ever
  repo_path           TEXT,                                 -- NULL = not yet configured
  managed             INTEGER NOT NULL DEFAULT 0,
  github_repo         TEXT,
  agent_id            INTEGER NOT NULL REFERENCES agents(id) ON DELETE RESTRICT,
  claude_session_id   TEXT,
  opencode_session_id TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
INSERT INTO global_task (id, agent_id)
  SELECT 1, COALESCE((SELECT id FROM agents WHERE is_default = 1), 1);
```

No delete endpoint will ever exist for this row, so no backfill hook is strictly needed (the seed runs inside the migration, after 00013 has guaranteed a default agent). The `CHECK (id = 1)` makes the singleton a DB-level invariant — the v1.9 `is_default`-style "structurally can't be wrong" move.

**`tmux_sessions` must be rebuilt** (SQLite cannot `ALTER` a column to drop `NOT NULL`; `ADD COLUMN` can't relax it either). Current shape (migration 00005, verbatim): `task_id INTEGER NOT NULL REFERENCES tasks(id)` + `UNIQUE(task_id, n)`. Recommended new shape via the standard `PRAGMA foreign_keys=OFF` rebuild (the 00012/00013 `NO TRANSACTION` discipline, plus a copy step):

```sql
CREATE TABLE tmux_sessions (
  id         INTEGER PRIMARY KEY,
  task_id    INTEGER REFERENCES tasks(id),        -- nullable now
  global     INTEGER NOT NULL DEFAULT 0,           -- 1 = global-scratchpad tab
  n          INTEGER NOT NULL,
  name       TEXT    NOT NULL UNIQUE,
  label      TEXT    NOT NULL DEFAULT '',
  created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE(task_id, n),
  CHECK ( (task_id IS NULL) = (global = 1) )       -- task-scoped XOR global, never both/neither
);
```

Two caveats the plans must carry:
- **`UNIQUE(task_id, n)` goes soft for global rows** — SQLite treats NULLs as distinct, so global `n` monotonicity rests on the app-side `SELECT COALESCE(MAX(n),0)+1 … WHERE global = 1` reserve-before-spawn (the exact posture `sessions.go:414-431` already documents as acceptable for single-user localhost). `name UNIQUE` still hard-guards collisions via the `kamacu-global-<n>` mint.
- **Every existing `tmux_sessions` reader must be audited**: the reconcile pass (`sessions.go:145`), lazy GC (`sessions.go:208`), reaper tmux kills (`reaper.go:335,370`), and the startup orphan sweep. The rebuild copies all rows with `global=0`, so task behavior is preserved byte-for-byte; global rows must not break the `JOIN`-free queries (they don't JOIN tasks — they filter by `task_id = ?`, which global rows simply never match; the orphan sweep matches `kamacu-*` names against table rows, which now include global rows — correct by construction).

#### 2. Session-engine seam — one additive discriminator, no fork

`session.Info.TaskID` docs say `0 = unscoped dev session` (`manager.go:56`, `session.go:82`). Global sessions need to be distinguishable from dev sessions and from each other's scope in `mgr.List()` consumers (`/api/agents/status`, the bar, MCP `list_sessions`). Recommended: **`Global bool` on `SpawnOpts` and `Info`** (`json:"global,omitempty"`), zero value = current behavior — the same additive-field move `Kind`, `TmuxName`, and `AgentEngine` each made in prior milestones. Global sessions keep `TaskID = 0`; consumers filter on `info.Global` instead of keying off a sentinel int.

Rejected alternatives: a reserved `TaskID = -1` (sentinel ints leak into every `TaskID > 0` guard — there are many, e.g. `sessions.go:269`); a `Scope string` enum (more surface, same information).

#### 3. API surface — additive widening, no new transport

| Endpoint | Shape | Notes |
|----------|-------|-------|
| `GET /api/global` | singleton config + derived status | `{repoPath, managed, githubRepo, agentId, agentName, configured}` |
| `PUT /api/global` (or PATCH) | `{repo_path}` XOR `{github_repo}` + optional `agent_id` | Folder path: `validateRepoPath` (`projects.go:78`) reused. Repo path: the v1.4 eight-step sequence (`projects.go:357-449`) with `UPDATE global_task` instead of `INSERT projects` — same atomic clone-before-row ordering, same reattach-or-refuse. `agent_id` validated against the agents table (RESTRICT backstop). |
| `POST /api/sessions` widened | `{"scope":"global","kind":"agent"\|"bash","resume":bool}` | Spawns with `opts.Cwd = global root`, `opts.Global = true`. Unconfigured root → 409 `"global root not configured"` (mirrors the `"task has no worktree"` 409 posture, `sessions.go:269-272`). Agent resolve reads `global_task.agent_id` instead of the task JOIN. One-running-agent gate (D-38 analog) over global sessions. Claude id persisted to `global_task.claude_session_id` post-spawn; opencode async capture parameterized to write the singleton. |
| `GET /api/agents/status` widened | global entries in the response | Manager-derived pass admits `info.Global && Kind == agent` entries (currently skipped at `agents.go:50`); DB-derived singleton read mirrors the tasks pass for restart survivors; entry shape gains `scope`/`isGlobal` (additive, `omitempty`) so the bar can route to `/global` instead of `/projects/:id/tasks/:tid` (`ActiveSessionsBar.tsx:134`). |
| tmux mint (internal) | `kamacu-global-<n>` | Reserve via `MAX(n)+1 WHERE global = 1`; row inserts with `task_id NULL, global 1`. The `kamacu-` prefix keeps the row inside the startup orphan-sweep namespace for free. |

Route registration is one `mux.HandleFunc` block in `routes.go` — the stdlib ServeMux pattern match syntax (`GET /api/global`) is already in use everywhere.

#### 4. Frontend — three composition points, zero packages

- **Route `/global`** in `App.tsx` (react-router 7 `<Route>` tree, `App.tsx:117-144`) rendering a slimmed task view: `AgentTab` + bash tabs only. `AgentTab.tsx` filters `useAgentStatuses()` by `taskId === task.id` (`AgentTab.tsx:58,194`) — the global variant matches the `scope: "global"` entry instead. `TerminalPane`/`useTerminalSocket` are session-id-keyed and need nothing.
- **Settings section** on `SettingsPage.tsx` (gear route, SQLite-backed per-field commit pattern) — folder input + `owner/name` input + agent dropdown, reusing the v1.4 segmented "GitHub repo | Local folder" toggle shape and the v1.10 agent selector.
- **Active Sessions bar entry** when a global session is live (`ActiveSessionsBar.tsx`), navigating to `/global`; the idle entry point is the Settings section (per PROJECT.md — roadmap decides the exact idle affordance).

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| `global_task` singleton table | Settings-KV rows (`global_root`, `global_agent_id`, …) | Only if the milestone dropped restart-resume AND tmux persistence AND agent-FK integrity — i.e. never, given PROJECT.md demands "same session semantics as tasks". KV also can't fix the `tmux_sessions` FK, so a migration happens anyway; two storage homes for one feature is worse than one. |
| `tmux_sessions` rebuild (nullable `task_id` + `global`) | Separate `global_tmux_sessions` table | Never for this codebase: it duplicates spawn-reserve, reconcile, lazy-GC, label-rename, and orphan-sweep code paths — the persistence-layer twin of forking the session engine. A rebuild is a bigger single migration but zero new logic shapes. |
| Additive `Global bool` on SpawnOpts/Info | Reserved sentinel `TaskID = -1` | Never — sentinel ints silently misuse every existing `TaskID > 0` / `TaskID <= 0` guard (at least `sessions.go:50,262,269`, `agents.go:50`). |
| Dedicated singleton | Hidden "global" project + task row (`source='global'`) | Never — the tasks table leaks into board queries, positions, Done-TTL reaping, v1.12 activity/statistics JOINs (`activity.go:134`), workspace scoping; every future feature would need an exclusion guard. This is the single biggest trap in the milestone. |
| Reuse `~/.kamacu/repos/<owner>/<name>` as the managed global-root dest | A separate `~/.kamacu/global/` clone root | Separate root only if cross-collision handling (same repo as project AND global root) proves simpler that way — plan-phase decision. One namespace + a cross-entity dedup check (`repo_path` against both tables → 409) reuses `reattachManaged` verbatim. |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| Any new Go/npm dependency for this feature | No required capability is missing; 15 milestones of pinned stack covers it all. Each dep would be pure liability (audit surface, version drift) for zero function. | The inventory above. |
| Bumping `modernc.org/sqlite` (v1.52.0 → v1.56.0) or `coder/websocket` (v1.8.14 → v1.8.15) as part of this milestone | Both newer releases exist (checked 2026-08-25) but are routine drift with nothing this feature needs; coupling a behavior-free bump to a feature milestone muddies bisection. | Stay pinned; bump separately in maintenance if desired. |
| A second WebSocket/HTTP transport for the global view | `internal/ws` is scope-blind (session-id keyed); a parallel transport doubles the security surface (Origin/Host validation, loopback binding) for nothing. | Existing `/ws` attach path. |
| Persisting global state in `localStorage` only | Restart-resume and cross-tab liveness are server truths; the client already learned this in v1.0 Phase 5. | SQLite singleton. |
| `gh repo clone` alternatives (`git clone` direct, go-git) | Settled in v1.4: gh gives host auth (private/org repos); go-git can't do what's needed. Ruled out again here — the global root rides the identical path. | `internal/github.Clone`. |
| A `POST /api/global/session` side-channel | A second agent-spawn endpoint forks the engine-branched spawn logic (claude argv, opencode capture, resume validation) that `sessions.go` already owns. | Widen `POST /api/sessions` with a scope field. |

## Stack Patterns by Variant

**If the global root is a local folder:**
- `PUT /api/global {repo_path}` → `validateRepoPath` (absolute, existing, git repo — same rules as projects) → store with `managed=0`.
- Kamacu never touches the folder's contents (v1.4 folder-project contract: delete/cleanup never applies).

**If the global root is a GitHub repo:**
- `PUT /api/global {github_repo}` → `ParseRepoRef` → `ValidateRepo` (gh-gated, degrade-don't-break) → dest `~/.kamacu/repos/<owner>/<name>` → existing dir? `reattachManaged` (origin-match) or 409 : `github.Clone` (atomic, cleanup-on-fail) → store with `managed=1`.
- Cross-entity collision (same dest already a `projects.repo_path`) → 409 with distinct copy; plan-phase decides whether switching is offered.

**If unconfigured (`repo_path IS NULL`):**
- `GET /api/global` reports `configured:false`; spawn endpoints 409 `"global root not configured"`; Settings shows the entry point; the bar shows nothing. Never a silent fallback to `$HOME` — an honest 409, the D-56/D-84 house posture.

**If the configured root dir vanishes on disk:**
- Spawn's stat pre-check (the vanished-worktree path, `sessions.go:437`) surfaces the same honest failure; reconfigure via Settings.

## Version Compatibility

| Package A | Compatible With | Notes |
|-----------|-----------------|-------|
| Migration 00017 | goose v3.27.1, modernc v1.52.0, migrations 00001–00016 | Rebuild step needs `PRAGMA foreign_keys=OFF` outside a transaction (`-- +goose NO TRANSACTION`) — the 00012/00013 precedent. Runs after 00013 (agents) and 00016; ordering is implicit in the numbering. |
| `Global bool` on `session.Info` | Existing JSON consumers (frontend, MCP bridge) | Additive with `omitempty` — absent for every non-global session; v1.11 MCP tools stream `Info` verbatim and are unaffected (a global session merely appears in `list_sessions` with empty task fields — acceptable; MCP global tools are Out-of-Scope). |
| `/api/agents/status` new fields | `ActiveSessionsBar.tsx`, `AgentTab.tsx`, card dots | Additive fields; existing readers filter by `taskId`, which global entries lack — the bar's row renderer needs the scope branch, everything else ignores them. |
| `kamacu-global-<n>` names | Startup orphan sweep, `defaultTmuxLabel` | Same `kamacu-` prefix → swept-iff-no-row semantics unchanged; `defaultTmuxLabel("kamacu-global-3")` derives "Bash 3" for free (`sessions.go:615-622`). |
| React Router 7 `<Route path="/global">` | Existing `AppLayout` nesting | No change to index-redirect / workspace-sync logic (`/global` is workspace-free by design — mirror the `/settings` pattern, which already sits outside workspace routing). |

## Integration Inventory (evidence for the plans)

| Integration point | File:line (working tree, 2026-08-25) | Change |
|-------------------|---------------------------------------|--------|
| Agent spawn gate + cwd resolution | `internal/api/sessions.go:267-304` | Global branch: resolve root + agent from singleton |
| One-agent gate | `internal/api/sessions.go:306-315` | Global variant of D-38 |
| Resume validation (engine-branched) | `internal/api/sessions.go:323-339` | Read singleton's csid/ocsid |
| Claude id persist post-spawn | `internal/api/sessions.go:460-463` | `UPDATE global_task …` |
| opencode async capture | `internal/api/sessions.go:793-858` | Parameterize persist target |
| tmux mint + reserve | `internal/api/sessions.go:403-432` | `kamacu-global-<n>`, `WHERE global=1` |
| Status endpoint (both passes) | `internal/api/agents.go:44-219` | Global pass + singleton DB-derived pass |
| Managed-clone sequence | `internal/api/projects.go:357-449`, `internal/github/clone.go:38` | Reuse; swap INSERT for UPDATE |
| Folder validation | `internal/api/projects.go:74-78` (`validateRepoPath`) | Reuse verbatim |
| Agent delete in-use guard | `internal/api/agents_crud.go:255` | Count global singleton alongside projects |
| tmux reconcile / GC | `internal/api/sessions.go:144-214` | Global-row aware (NULL task_id) |
| Reaper tmux paths + orphan sweep | `internal/reaper/reaper.go:335,370,390` | Verify global rows survive sweep (they have rows) |
| Hooks receiver (live status) | `internal/api/hooks.go` (session-id keyed) | **No change** — global sessions get working/waiting/idle for free |
| Activity attribution (v1.12) | `internal/api/activity.go:134` (tasks JOIN) | **Flag**: global sessions' hook events need either exclusion or a "Global" attribution — plan-phase decision, not a blocker |
| Bar routing | `web/src/components/layout/ActiveSessionsBar.tsx:134` | `/global` branch |
| Route tree | `web/src/App.tsx:117-144` | `<Route path="/global">` |
| Settings page | `web/src/pages/SettingsPage.tsx` | Global section |

## Sources

- Working tree of `kamacu@add-global-session-301` (87e6a77) — `go.mod`, `web/package.json`, `internal/{session,api,github,settings,store,reaper}`, migrations 00001–00016 read directly (curated source, HIGH).
- `.planning/PROJECT.md` v1.13 milestone targets (curated, HIGH).
- pkg.go.dev / release mirrors via web search (2026-08-25) — `modernc.org/sqlite` latest v1.56.0 vs pinned v1.52.0; `coder/websocket` v1.8.15 vs pinned v1.8.14 (MEDIUM — routine drift, no action).
- Historical: original STACK research (2026-06-10, embedded in CLAUDE.md) for version-selection rationale still governing the pins (HIGH at time of writing; nothing in this milestone re-opens it).

---
*Stack research for: v1.13 Global Task (Kamacu)*
*Researched: 2026-08-25*
