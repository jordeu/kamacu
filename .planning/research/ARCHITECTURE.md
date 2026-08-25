# Architecture Research

**Domain:** A "Global Task" singleton scratchpad added to Kamacu (existing local-only Go+React app) — a task-like agent+bash view with no project, no worktree, no board, running directly in a configured repo/folder.
**Researched:** 2026-08-25
**Confidence:** HIGH — every claim below is grounded in the live Kamacu codebase at v1.12 (tag v1.12, worktree `add-global-session-301`): every file/function/line reference in the integration map was read in the actual repo. Schema claims come from `internal/store/migrations/00001–00016`. No external sources were needed — this is an integration-architecture question about our own code.

---

## TL;DR for the roadmap author

- **Do NOT use a reserved/sentinel `tasks` row.** `tasks.project_id` is `NOT NULL REFERENCES projects(id)` (migration 00001, DB-enforced, SQLite cannot ALTER it away without a table rebuild), and the sentinel escape hatch — a phantom "Global" project — leaks into ~10 surfaces that would all need hiding (sidebar, `GET /api/projects`, workspace non-empty COUNT/delete guard, Activity scopes, MCP `list_projects`/`get_project`, index redirect, agents in-use guard…). The task CRUD/MCP surface (`get_task`/`update_task`/`delete_task` reach any id with no source guard) would additionally need global-aware 409 guards. High blast radius, pure downside.
- **Recommended: a task-free "global scope" on the session engine + a singleton `global_task` config/state table.** The session engine (`internal/session`) is already **cwd-driven, not worktree-driven** — `Manager.Spawn` requires only a working directory for agents (manager.go:125); every task/worktree coupling lives in the API handler (`sessions.go:269-302`). `TaskID` is an opaque tag to the engine. Adding `SpawnOpts.Global bool` / `Info.Global bool` is small and additive.
- **Two migrations**: 00017 creates the `global_task` singleton table (root path + repo ref + agent FK + the two resume ids — config and resume state co-located, following the v1.9/v1.10 singleton-row + idempotent-backfill precedent); 00018 rebuilds `tmux_sessions` with a nullable `task_id` + `scope` column so global bash tabs keep restart durability. The rebuild is the codebase's proven NO-TRANSACTION/PRAGMA-off pattern (00012/00013), one notch heavier (CREATE-copy-drop-rename).
- **One pre-existing killer bug to fix as part of the migration phase**: `sweepOrphanTmux` (serve.go:351-401) computes "known" sessions via `INNER JOIN tmux_sessions × tasks` — once NULL-task global rows exist, every live global tmux tab looks orphaned and gets **killed at startup**. Must become scope-aware in the same phase as the migration.
- **Almost everything else is additive**: the WS attach path, hooks receiver, TerminalPane, reaper (zero changes — no task row means no Done-TTL and no PR reconcile), and MCP session tools are all session-id-keyed or task-filtered in ways that already behave correctly for a project-less session.
- **Build order is dictated by three dependencies**: (1) the data layer (table + tmux scope + sweep fix) must exist before any spawn path; (2) the spawn/status backend must exist before the frontend view; (3) the config UI (Settings section) is only useful once `/api/global` exists — but the folder-variant config API and the managed-clone variant can be split into two waves. Minimal vertical slice: `global_task` row + `POST /api/sessions {scope:"global", kind:"agent"}` resolving root+agent + `/api/agents/status` global entry.

---

## The Singleton Decision (the central question)

### Options considered

| Criterion | A. Sentinel project + `tasks` row (`source='global'`) | B. Reserved tasks row, `project_id` made nullable | **C. Task-free sessions + `global_task` singleton table (RECOMMENDED)** |
|---|---|---|---|
| Schema cost | None (phantom rows) | **Rebuild of `tasks`** (SQLite can't drop NOT NULL via ALTER) — the most-JOINed table in the app | 2 contained migrations (new table + `tmux_sessions` rebuild) |
| Task-scoped JOINs keep working | Yes, byte-for-byte (that's the whole appeal) | Only after rewriting both `agents.go` passes + `joinSessionContext` to LEFT JOINs | No — they're *replaced* by one synthesized global entry each (smaller, explicit) |
| Hidden-entity leaks | **~10 surfaces** (see below) | Few (board queries already filter `source='manual'`) | None — nothing pretends to be a project/task |
| Task CRUD/MCP guards needed | Yes: `PATCH`/`DELETE`/`get`/MCP `update_task`/`delete_task` must 409 the global row; `create` position subqueries must not count it | Same | None — there is no row to guard |
| Reaper interaction | Must exclude the global row from Done-TTL/PR passes | Same | **Zero changes** (no task row → both passes skip by construction) |
| tmux restart durability | Free (real task_id) | Free | Needs the 00018 rebuild |
| Resume-state storage | Free (`tasks.claude_session_id` etc.) | Free | `global_task.claude_session_id` / `opencode_session_id` — same semantics, one row |
| Agent resolution | `projects.agent_id` (sentinel project) | needs settings/LEFT JOIN anyway | `global_task.agent_id` FK (same shape as `projects.agent_id`) |

### Why the sentinel row loses — the concrete evidence

1. **`tasks.project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE`** (00001). SQLite has no `ALTER COLUMN`; Option B is a full rebuild of the table behind agents status (agents.go:99, 164), session context JOIN (sessions.go:935), activity (activity.go:135), the board's 5 position queries, the reaper, and PR find-or-create — the hottest paths in the app, all wrapped in regression suites we'd have to re-verify for zero feature gain. Option A dodges the rebuild but only by minting a phantom project — and `projects` is even more surfaced than `tasks`: sidebar rows, collapsed avatar rail, `GET /api/projects[?workspace_id=]`, the v1.9 **workspace non-empty COUNT guard** (a hidden sentinel project makes "Personal" permanently non-empty and undeletable), Activity project scopes, MCP `list_projects`/`get_project`, `RedirectToFirstProject`, project settings, and the agents in-use delete guard.
2. **The task surface is not source-guarded on the id paths.** The board queries filter `source='manual'` (tasks.go:194, 233, 288, 432, 538, 554) and `/move` 409s non-manual — but `GET/PATCH/DELETE /api/tasks/{id}` and the MCP `get_task`/`update_task`/`delete_task` tools operate on any id. A sentinel row is writable/deletable by the user *and by an MCP agent* unless every one of those grows a guard.
3. **The engine doesn't need it.** `Manager.Spawn` validates only `opts.Cwd != ""` for agents (manager.go:125-127) — "task has no worktree" is an API-layer gate (sessions.go:269-272), not an engine invariant. `StopAllForTask`/`ListByTask` treat the id opaquely. The DB JOINs are the *only* consumers that truly require a task row, and for the global task they must produce sentinel labels ("Global") anyway.

### What Option C costs (honestly)

- One bounded `tmux_sessions` rebuild (nullable `task_id` + `scope TEXT NOT NULL DEFAULT 'task'`) — required because global tmux tabs must persist rows and `task_id INTEGER NOT NULL REFERENCES tasks(id)` (00005) rejects both NULL and a fake 0.
- Every `tmux_sessions` touchpoint gains scope awareness: spawn INSERT, `reconcileTmux` (+ a global variant), the startup sweep fix, and `defaultTmuxLabel` (already fine — it parses the last `-` segment, so `kamacu-global-3` → "Bash 3").
- The agents-status and session-context JOINs each get one synthesized global entry instead of riding the JOIN.

That is strictly less risk than either sentinel variant, and it leaves **zero phantom entities** to hide.

### Rejected sub-variant: resume/config state in the settings KV

Storing `global_root`/`global_agent_id`/`global_claude_session_id` as settings KV rows would need zero migrations, but: (a) `agent_id` as a string loses FK integrity — the agents delete handler's in-use guard (agents_crud.go) counts only `projects`, so a referenced agent becomes deletable and the setting dangles; (b) it mixes per-instance *state* (latest claude session id, discovered opencode id — newest-wins written on every spawn) with user *config* (root, agent), which the settings page's "reset to default" semantics would actively corrupt; (c) `settings.Set` validation would grow root-path/git/agent-id rules foreign to that package. A dedicated table gives us the `REFERENCES agents(id) ON DELETE RESTRICT` backstop for free — the exact shape `projects.agent_id` already has.

---

## Recommended Architecture

### System overview

```
┌──────────────────────────────────────────────────────────────────────────┐
│ Browser (React SPA)                                                      │
│                                                                          │
│  ActiveSessionsBar ── global row ("Global · Global Task") ──► /global    │
│  SettingsPage ── new "Global Task" section (root | repo, agent, Open)    │
│  GlobalTaskPage (/global) ── AgentTab + Bash N tabs (TaskTabs shell)     │
│        │  TanStack Query: useGlobalConfig, useGlobalSessions,            │
│        │                   useSpawnGlobalAgent / useResumeGlobalAgent     │
└────────┼─────────────────────────────────────────────────────────────────┘
         │ HTTP/WS (unchanged client.ts / useTerminalSocket — id-keyed)
┌────────▼─────────────────────────────────────────────────────────────────┐
│ Go server                                                                │
│                                                                          │
│  NEW  POST/GET/PUT /api/global ── internal/api/global.go                 │
│         folder variant: validate dir      ─┐                             │
│         repo variant:  gh-validate ────────┼── internal/github (Clone,   │
│                        github.Clone ───────┘   ValidateRepo, reattach)    │
│                                                                          │
│  MOD  POST /api/sessions {scope:"global"} ── sessions.go global branch   │
│         cwd = global_task.root_path (NO worktree, NO branch)             │
│         agent = JOIN agents ON global_task.agent_id                      │
│         tmux name = kamacu-global-<n> (tmux_sessions.scope='global')     │
│                                                                          │
│  MOD  GET /api/sessions?scope=global ── list + global reconcileTmux      │
│  MOD  GET /api/agents/status ── third pass: synthesized global entry     │
│         (manager-derived + DB-derived post-restart resumable)            │
│                                                                          │
│  UNCHANGED  internal/session engine (adds Global flag only),             │
│             internal/ws (id-keyed), hooks.go (id-keyed), reaper (no-op)  │
└────────┼─────────────────────────────────────────────────────────────────┘
┌────────▼─────────────────────────────────────────────────────────────────┐
│ SQLite                                                                   │
│   NEW  global_task (singleton id=1: root_path, github_repo, agent_id FK, │
│        claude_session_id, opencode_session_id)      ── migration 00017   │
│   MOD  tmux_sessions (task_id nullable, scope 'task'|'global')           │
│                                                     ── migration 00018   │
│   UNCHANGED  projects / tasks / agents / workspaces / settings / diff_*  │
└──────────────────────────────────────────────────────────────────────────┘
```

### Data model

**Migration 00017 — `global_task` singleton** (new table + idempotent backfill hook `BackfillGlobalTask`, wired in serve.go after `BackfillOpenCodeAgent` — the v1.9/v1.10 pattern):

```sql
-- +goose Up
CREATE TABLE global_task (
  id                  INTEGER PRIMARY KEY CHECK (id = 1),   -- singleton, DB-enforced
  root_path           TEXT NOT NULL DEFAULT '',              -- effective cwd ("" = unconfigured)
  github_repo         TEXT,                                  -- non-NULL ⇒ managed clone (v1.4 pattern)
  agent_id            INTEGER NOT NULL REFERENCES agents(id) ON DELETE RESTRICT,
  claude_session_id   TEXT,                                  -- newest-wins resume key (tasks.claude_session_id semantics)
  opencode_session_id TEXT,                                  -- discovered ses_… (tasks.opencode_session_id semantics)
  created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
INSERT INTO global_task (id, agent_id)
  SELECT 1, id FROM agents WHERE is_default = 1;   -- fresh install lands on the Claude seed
```

Notes:
- `agent_id NOT NULL … RESTRICT` mirrors `projects.agent_id` (00013) exactly; the agents delete handler's friendly in-use guard grows one more COUNT (see Integration Points). The backfill (not the migration) owns re-creating the row if missing — migration runs once, backfill runs every boot.
- `root_path` always stores the **effective directory** (for the managed variant: `~/.kamacu/repos/<owner>/<name>` written at save). `github_repo IS NOT NULL` is the "Kamacu owns this dir" marker — the same single-source-of-truth rule the `managed` column plays for projects.
- Resume ids on the singleton behave exactly like their `tasks` counterparts: written on spawn (claude — kamacu mints it), discovered async (opencode), never cleared on stop (resumable ghost, D-96 posture).

**Migration 00018 — `tmux_sessions` scope rebuild** (the NO-TRANSACTION + `PRAGMA foreign_keys=OFF` discipline from 00012/00013, one step heavier — CREATE-copy-DROP-rename):

```sql
-- +goose NO TRANSACTION Up
PRAGMA foreign_keys = OFF;
BEGIN;
CREATE TABLE tmux_sessions_new (
    id         INTEGER PRIMARY KEY,
    task_id    INTEGER REFERENCES tasks(id),        -- now nullable: global rows have no task
    scope      TEXT NOT NULL DEFAULT 'task' CHECK (scope IN ('task','global')),
    n          INTEGER NOT NULL,
    name       TEXT    NOT NULL UNIQUE,
    label      TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE(task_id, n)
);
INSERT INTO tmux_sessions_new (id, task_id, scope, n, name, label, created_at)
  SELECT id, task_id, 'task', n, name, label, created_at FROM tmux_sessions;
DROP TABLE tmux_sessions;
ALTER TABLE tmux_sessions_new RENAME TO tmux_sessions;
COMMIT;
PRAGMA foreign_key_check;
PRAGMA foreign_keys = ON;
```

Consequences to carry through the code (all small, all listed in Integration Points):
- `n` sequencing for global tabs: `SELECT COALESCE(MAX(n),0)+1 WHERE scope='global'` (in SQLite, NULL `task_id` rows are distinct under `UNIQUE(task_id,n)`, so the pre-existing `name UNIQUE` constraint is the real collision guard — same as today, where the read-then-insert race is accepted with a constraint backstop).
- Names: `kamacu-global-<n>` — still under the `kamacu-` prefix the orphan sweep filters on, and `defaultTmuxLabel` already derives `Bash <n>` from the last `-` segment.
- Every existing `WHERE task_id = ?` query keeps working unchanged (NULL rows never match). The **startup sweep is the exception** — see Pitfall 1.

### Component responsibilities

| Component | Status | Responsibility |
|---|---|---|
| `internal/api/global.go` | NEW | `GET /api/global` (config + derived state: configured, managed, resolved root, live-session presence), `PUT /api/global` (folder variant: validate; repo variant: gh-validate → `github.Clone` → reattach-or-409 → atomic row update). Owns the v1.4 8-step atomicity ordering adapted to an UPDATE-not-INSERT. |
| `internal/session` (manager.go, session.go) | MOD (additive) | `SpawnOpts.Global bool`, `Info.Global bool`, a dedicated global bash-label counter (`Bash N`), `Manager.ListGlobal()` + a `StopAllGlobal()` used by the config-switch/root-clear gates. Zero changes to the PTY/ring/attach machinery. |
| `internal/api/sessions.go` | MOD | `create`: a `scope:"global"` branch (resolve root + agent from `global_task`, one-agent-per-global gate, tmux `kamacu-global-<n>` INSERT with `scope='global'`, resume variants persisting to `global_task`). `list`: `?scope=global` filter + a global `reconcileTmux` variant. |
| `internal/api/agents.go` | MOD | A third, synthesized pass appended to `status`: manager-derived global entry (newest global agent session) + DB-derived post-restart resumable entry from `global_task`. Emits `source:"global"`, `projectId:null`, `taskTitle:"Global Task"`, `projectName:"Global"`. |
| `cmd/kamacu/serve.go` | MOD | Wire `BackfillGlobalTask`; register `/api/global`; fix `sweepOrphanTmux` known-set query. |
| `internal/api/agents_crud.go` | MOD | In-use delete guard counts the `global_task.agent_id` reference (friendly 409 ahead of the RESTRICT backstop). |
| `internal/api/projects.go` | MOD (guard) | Managed-project delete gate: when `global_task.github_repo`/`root_path` points at this project's clone, a running global session (or any global config tie) is a `sessions` blocker — otherwise `removeManaged`'s `os.RemoveAll(clone)` (projects.go:914) deletes the live global root out from under a running agent. |
| `internal/reaper` | UNCHANGED | Done-TTL selects `tasks.status='done'` (no global row → skipped); PR reconcile selects `source='github_pr'` (skipped). This is a *feature* of Option C. |
| `internal/ws`, `internal/api/hooks.go`, `internal/api/resume.go` | UNCHANGED | All id-keyed (`mgr.Get(id)`) or glob-based (`transcriptExists` is repo-agnostic — claude transcript lookup is global by session uuid, cwd-independent). |
| `internal/mcp` | UNCHANGED (v1.13) | Global sessions already appear in `list_sessions` (the unfiltered `mgr.List()` pass) with empty JOIN fields; `get_session`/`output`/`subscribe`/`send_session_message`/`start_task_agent` are id/task-scoped and simply never address the global scope. An optional `scope` filter on `list_sessions` is a cheap follow-up, not a requirement. |
| `web/src/api/global.ts` | NEW | `GlobalConfig` type + `useGlobalConfig`, `useSaveGlobalConfig` (folder), `useCloneGlobalRoot` (repo, blocking "Cloning…" state), mirroring the worktreeCleanup/agents client shapes. |
| `web/src/api/sessions.ts` | MOD | `useGlobalSessions()` (`?scope=global`), `useSpawnGlobalAgent`/`useResumeGlobalAgent`/`useSpawnGlobalBash`/`useReattachGlobalTmux` — the same five hooks with `scope:"global"` bodies and a `["sessions","global"]` cache key. |
| `web/src/pages/GlobalTaskPage.tsx` | NEW | The trimmed task view: Agent tab + bash tabs ONLY (no Description, no Diff, no title editor, no ⋯ delete/cleanup menu, no WorktreeMetaLine). Reuses `TaskTabs`, `TerminalPane`, and a loosened `AgentTab`. |
| `web/src/components/task/AgentTab.tsx` | MOD (props loosening) | Currently takes `task: Task` and calls `useSpawnAgent(task.id)`; refactor to take explicit spawn/resume mutation callbacks (or a minimal `{id, worktree_path, description}`-shaped prop) so the global page reuses it without a fake Task object. |
| `web/src/components/layout/ActiveSessionsBar.tsx` | MOD | `AgentStatusEntry.projectId` → `number \| null`, `source` gains `"global"`; row navigation branches (`source==="global"` → `/global`, else the project/task route); row label renders `Global · Global Task` (a Globe glyph or badge substitutes the project-name span). |
| `web/src/pages/SettingsPage.tsx` + `components/settings/GlobalSection.tsx` | NEW section | The Settings "Global Task" block: segmented "Local folder | GitHub repo" (reuse the AddProjectDialog toggle pattern), agent selector (reuse the per-project selector), inline validation, and an "Open Global Task" affordance (the idle reachability path). |

### Where sessions run

- **cwd = `global_task.root_path`**, verified at spawn by the engine's existing pre-PTY `os.Stat` check (manager.go:137-143) — a root deleted out from under us produces the clean "couldn't start a session" path.
- **No worktree, no branch, no fetch** — `internal/worktree` is never in the global spawn path. This is the "true scratchpad" contract from PROJECT.md.
- **Folder-variant validation** (decision point, recommendation below): absolute path to an existing **directory** — *not* necessarily a git repo. Tasks need git because worktrees need it; the global scratchpad never creates a worktree, and both bash and agent CLIs run fine in a plain folder. Reuse `validateRepoPath`'s `~` expansion + `os.Stat` skeleton but drop the `git rev-parse` requirement (a new `validateDirPath`). The managed variant is git by construction (`gh repo clone`).
- **Spawn env**: identical to task agents — the claude path (hook overlay, `--session-id`, extras) and the opencode path (`KAMACU_*` env + the load-bearing `PWD=<dir>` pin, manager.go:199, which keeps `opencode session list` directory-matching working when `<dir>` is the global root instead of a worktree) are cwd-parameterized already.

### How the global agent is resolved

- `global_task.agent_id` → `JOIN agents a ON a.id = g.agent_id` supplies `engine`/`command`/`extra_params` — the exact same row shape the task spawn reads via `projects.agent_id` (sessions.go:285-290), so the engine fork (claude / custom / opencode), `renderAgentCommand` placeholders (`{{worktree}}` → the root path), extras tokenization, and the quota-indicator gating all carry over unchanged.
- Read-at-use (fresh query per spawn, the SET-03 rule): an agent edit applies at the next Start with no restart.
- Delete guard: `DELETE /api/agents/{id}` grows one COUNT against `global_task` (the RESTRICT FK is the DB backstop) — the same block-until-unassigned contract projects have.

### Restart reconciliation & resume (no task row)

The two-pass pattern in `agents.go` extends by one synthesized pass:

1. **Manager-derived (live)**: newest global agent session from `mgr.List()` (`Kind==agent && Global`), meta from `global_task` (engine, csid, ocsid, root). Resumable derivation is the same engine-branched check with `root_path` standing in for `wtp` (claude: csid + `transcriptExists`; opencode: ocsid alone).
2. **DB-derived (post-restart)**: when the manager has no global agent entry and `global_task` carries a csid/ocsid that passes the same resumability check, append the `exited`/`resumable:true`/`sessionId:""` entry — the global analogue of agents.go:164-214.
3. **Resume spawn**: `POST /api/sessions {scope:"global", kind:"agent", resume:true}` — claude reuses `--resume <csid>` (transcript glob is repo-agnostic); opencode appends `-s <ocsid>`. Fresh spawn persists the new csid to `global_task.claude_session_id` (the same newest-wins UPDATE, different table); fresh opencode launches `captureOpencodeSessionAsync` with a persist target of `global_task` — parametrize the persist step (a small func or an owner enum) rather than forking the poller.
4. **tmux bash tabs**: `GET /api/sessions?scope=global` runs a global `reconcileTmux` (same survivor/GC logic, `WHERE scope='global'`); the frontend's invisible reattach (`reattach_tmux_name`, `new-session -A`) works verbatim because the name is the only key.

### Managed-clone lifecycle for the global root

Reuse the v1.4 primitives; the differences are all *simplifications* (no worktrees hang off it):

- **Save (repo variant)** — the createByRepo 8-step adapted to UPDATE: ParseRepoRef → `ValidateRepo` (gh-gated; 400 with the canonical copy on failure, row untouched) → dest = `~/.kamacu/repos/<owner>/<name>` → existing dir ⇒ `reattachManaged` semantics (origin-match reuses; mismatch/non-git 409s, never clobbers) → `github.Clone` (atomicity: on failure the row keeps its previous value — no half-switched config) → UPDATE `root_path`+`github_repo` on exit 0.
- **Dedup against projects**: `projects.repo_path` is UNIQUE but the global root is *not* a project row — a managed project and the global root can point at the same clone dir. Recommended guard (decision point): refuse the repo variant with a 409 "this repository is already added as a project" when `projects.repo_path = dest` matches, and conversely extend the managed-project delete gate (above) — otherwise `removeManaged`'s `os.RemoveAll(clone)` orphans the global config and can kill a live global session's cwd. Both directions need exactly one check each.
- **Switching/clearing the root**: leaving a managed root (folder switch, repo switch, clear) should run the four gates against the old clone (dirty / unpushed vs `origin/<default>` / stash / live global sessions) and 409 with the structured `reasons` list on any trip — `deleteManaged`'s gate phase minus the worktree loop, plus a `StopAllGlobal()` in the remove phase. Folder roots are never touched (the projects D-09 rule).
- **Freshness**: no per-task fetch equivalent needed — the agent runs on whatever the clone holds; a manual `git pull` in a global bash tab is the scratchpad-native answer.

---

## Architectural Patterns

### Pattern 1: Singleton row + idempotent startup backfill

**What:** one guaranteed row (`global_task` id=1) created by migration, re-armed by `BackfillGlobalTask` every boot.
**When to use:** any future singleton config/state (this is the third use after workspaces' Personal and agents' Claude seed).
**Trade-offs:** the `CHECK (id = 1)` makes the singleton DB-enforced; the backfill makes "row missing" unreachable. Cost: one more boot step in serve.go — negligible.

### Pattern 2: Explicit scope flag, never a sentinel id

**What:** `SpawnOpts.Global bool` / `Info.Global bool` rather than encoding "global" into `TaskID`.
**When to use:** whenever a new session family appears.
**Trade-offs:** a sentinel id (e.g. `-1`) silently interacts with every existing `TaskID <= 0` check (agents.go:50, joinSessionContext, the manager's dev-session label branch) — three places that would treat `-1` as a dev session and one (taskCounters) that would happily allocate it. An explicit flag fails loudly anywhere it isn't threaded. Negative ids are the classic invisible-corruption trap; the codebase already proves the flag approach (`Orphaned bool`, `Kind`).

```go
// manager.go — label branch grows one arm; nothing else in the engine changes
switch {
case kind == KindAgent:
    label = "Agent"
case opts.Global:                     // NEW — before the TaskID==0 dev arm
    m.globalCounter++
    label = fmt.Sprintf("Bash %d", m.globalCounter)
case opts.TaskID == 0:
    ...
}
```

### Pattern 3: Synthesized JOIN entries instead of LEFT-JOIN surgery

**What:** the global status/context entries are appended by a dedicated pass that reads `global_task`, not produced by rewriting the tasks→projects→agents INNER JOINs.
**When to use:** whenever a legitimately row-less entity must appear on a JOIN-backed wire.
**Trade-offs:** two small passes instead of nullable-safe rewrites of the hottest queries in the app; the cost is that the global entry's field set must be maintained in one more place — contained, explicit, and testable in isolation.

### Pattern 4: Degrade-don't-break on the unconfigured state

**What:** an empty `root_path` is a normal state, not an error: `/api/agents/status` simply has no global entry; the bar shows nothing; `POST /api/sessions {scope:"global"}` returns a crisp 409 ("global root not configured") exactly like "task has no worktree" (sessions.go:300); the Settings section and the `/global` route render a configure-first empty state that links to Settings.
**When to use:** every optional-feature surface (the github integration toggle is the house precedent).

---

## Data Flow

### Global agent spawn

```
GlobalTaskPage "Start agent"
  → POST /api/sessions {scope:"global", kind:"agent"}
    → read global_task row (root_path, agent_id, csid/ocsid)     [read-at-use]
    → root empty? 409 "global root not configured"
    → one-agent-per-global gate (mgr.List(), Kind==agent && Global && running → 409)
    → JOIN agents (engine, command, extra_params)
    → mgr.Spawn{Cwd: root, Kind: agent, Global: true, AgentEngine, ExtraArgs|AgentArgs}
    → UPDATE global_task.claude_session_id                       [warn-only]
    → [opencode] go captureOpencodeSessionAsync(..., persist-to: global_task)
  ← 201 Info{global:true, label:"Agent"}
  → invalidate ["sessions","global"] + ["agent-statuses"]
```

### Post-restart resume discovery

```
server restart (PTYs die; global_task row survives; tmux server survives)
  → GET /api/agents/status
    → pass 1/2 (tasks, unchanged)
    → pass 3 (NEW): global_task.csid/ocsid + engine-branched check
        ↳ claude: transcriptExists(~/.claude/projects/*/csid.jsonl)  [cwd-agnostic]
        ↳ opencode: ocsid valid
    → append {source:"global", projectId:null, status:"exited", resumable:true}
  → ActiveSessionsBar: exited is filtered (live-only) — no row
  → Settings "Open Global Task" → /global → AgentTab pre-start resumable state
      → "Resume session" → POST {scope:"global", kind:"agent", resume:true}
```

### Reachability

- **Live**: the bar's global row (`Global · Global Task` + state dot) → click → `/global` → cross-surface behavior identical to a task row (collapse-on-open, current-row highlight keyed on route instead of taskId).
- **Idle**: Settings → Global Task section → "Open Global Task" (rendered only when `root_path` is set); `/global` itself is always routable and shows the configure-first empty state when unconfigured.

---

## Anti-Patterns (do NOT do these)

### Anti-Pattern 1: shipping the tmux migration without the sweep fix

**What people do:** add nullable-task rows, forget that `sweepOrphanTmux`'s known-set is `SELECT ts.name FROM tmux_sessions ts JOIN tasks t ON t.id = ts.task_id` (serve.go:369) — an INNER JOIN.
**Why it's wrong:** every global row (task_id NULL) drops out of the known set, so the sweep classifies every live `kamacu-global-*` tmux session as an orphan and **kills it at next startup** — silently destroying the exact restart durability the feature promises.
**Do this instead:** same phase as migration 00018, change the known-set to keep scope-aware rows (`ts.task_id IS NULL OR` join-exists — a LEFT JOIN with a NULL check), with a regression test that seeds a `scope='global'` row + a fake live session and asserts no kill.

### Anti-Pattern 2: a phantom "Global" project (or any hidden sentinel entity)

**What people do:** create the sentinel project so the task JOINs keep working, then filter it out of the UI "later".
**Why it's wrong:** the filter list is long and grows (sidebar expanded+rail, `GET /api/projects` ± workspace filter, workspace non-empty delete guard, Activity scopes, MCP project tools, index redirect, agents in-use guard, quota surface). One missed surface ships a user-visible ghost; the workspace-delete guard ships a *functional* bug (Personal becomes undeletable).
**Do this instead:** no project, no task — a synthesized global entry at the three consumers that need labels.

### Anti-Pattern 3: encoding "global" into TaskID

**What people do:** `task_id = -1` (or 0 with a convention) to avoid touching `SpawnOpts`.
**Why it's wrong:** `TaskID <= 0` already means "dev session" in three load-bearing checks (agents.go:50, joinSessionContext, the label switch); dev-session semantics (unscoped list, `bash #N` labels, `/terminal` spawns) would collide with global semantics.
**Do this instead:** an explicit `Global` flag (Pattern 2).

### Anti-Pattern 4: letting two owners race over one managed clone

**What people do:** allow the global repo root to point at a dir a managed project already owns, "because reattach says it's fine".
**Why it's wrong:** `removeManaged` does `os.RemoveAll(clone)` (projects.go:914) with gates that count only *task* sessions — a live global agent in that dir is invisible to the gate; delete succeeds; the global session's cwd vanishes mid-run and `global_task.root_path` dangles.
**Do this instead:** refuse the overlap at config save (409 when `projects.repo_path = dest`), and make the project-delete gate treat a global-root match as a blocker.

### Anti-Pattern 5: a second, parallel "global settings" mechanism

**What people do:** put `global_root` in the settings KV because it's already surfaced via `GET /api/settings`.
**Why it's wrong:** resume state would land in a config table (reset-to-default semantics become hazardous), and the agent reference loses its FK/in-use-guard integrity (see the singleton-decision section).
**Do this instead:** one resource (`/api/global`) over one table; the Settings *section* is UI placement, not storage placement.

---

## Integration Points

### Modified — backend

| File | Change | Risk |
|---|---|---|
| `internal/store/migrations/00017_global_task.sql` (NEW) + `internal/api/global_backfill.go` (NEW) | singleton table + `BackfillGlobalTask` wired in serve.go after `BackfillOpenCodeAgent` | Low (proven pattern; goose upgrade-path test) |
| `internal/store/migrations/00018_tmux_scope.sql` (NEW) | tmux_sessions rebuild (nullable task_id + scope) | **Medium** — full table rebuild under FK-off discipline; test copy fidelity (row count, UNIQUE name, n-sequence) + FK re-arm |
| `cmd/kamacu/serve.go` | backfill wiring, `/api/global` registration, `sweepOrphanTmux` scope-aware known-set | **High if skipped** (Pitfall 1) |
| `internal/api/sessions.go` | `create` global branch (root+agent resolution, one-agent gate, tmux global INSERT, resume, csid persist, opencode capture target); `list` `?scope=global` + global `reconcileTmux` | Medium — the milestone's risk center; fake-claude regression must stay byte-for-byte green on the task path |
| `internal/api/agents.go` | third synthesized pass (manager + DB-derived) with `source:"global"` / `projectId:null` | Low-Medium — append-only after the existing passes |
| `internal/api/global.go` (NEW) | GET/PUT config + managed-clone lifecycle + gated root switch/clear | Medium (repo variant reuses createByRepo's ordering) |
| `internal/api/agents_crud.go` | delete guard counts `global_task.agent_id` | Low |
| `internal/api/projects.go` | managed-delete gate: global-root tie ⇒ `sessions` blocker | Low (one conditional in `deleteManaged`) |
| `internal/session/{manager,session}.go` | `Global` flag on SpawnOpts/Info, global label counter, `ListGlobal`/`StopAllGlobal` | Low (additive; no PTY machinery changes) |
| `internal/reaper/reaper.go` | **none** | — |

### Modified — frontend

| File | Change |
|---|---|
| `web/src/api/sessions.ts` | global hook set (`useGlobalSessions`, spawn/resume/reattach variants) with `["sessions","global"]` cache key; `TermSession.global?: boolean` |
| `web/src/api/agents.ts` | `projectId: number \| null`, `source: "manual" \| "github_pr" \| "global"` |
| `web/src/api/global.ts` (NEW) | config client (get/save/clone-root) |
| `web/src/App.tsx` | `/global` route (sibling of `/settings`/`/activity`; NOT wrapped in BoardWorkspaceSync — no project to sync) |
| `web/src/pages/GlobalTaskPage.tsx` (NEW) | trimmed TaskPage: Agent + bash tabs, global agent-entry lookup (`e.source === "global"`), Esc → back to previous surface (no board), quota indicator gated on the global agent's engine |
| `web/src/components/task/AgentTab.tsx` | props loosened from `task: Task` to explicit callbacks/fields for task-free reuse |
| `web/src/components/layout/ActiveSessionsBar.tsx` | global row rendering + `/global` navigation + current-row highlight |
| `web/src/pages/SettingsPage.tsx` + `components/settings/GlobalSection.tsx` (NEW) | config section + Open affordance |

### Unchanged-by-design (verify, don't touch)

`internal/ws` (id-keyed attach), `internal/api/hooks.go` (id-keyed status receiver — claude AND opencode activity work in the global PTY exactly as in a worktree), `internal/api/resume.go` (`transcriptExists` is cwd-agnostic), `internal/worktree`, `internal/diff`, `internal/quota`, `internal/migrate` (note: its LIKE-gated path rewrite predates `global_task`; a managed global root stored *after* v1.8 needs no legacy rewrite — only note it if the table ever stores user-typed paths that could predate a migration), `internal/mcp` (task/project/workspace/review tools; global sessions already ride the unfiltered session list).

---

## Suggested Build Order (dependency-driven)

1. **Phase 1 — Data foundation & safety net.** Migration 00017 (`global_task` + backfill), migration 00018 (tmux scope rebuild), `sweepOrphanTmux` fix, agents delete-guard extension. Everything else blocks on this; the sweep fix MUST land with 00018 (Pitfall 1). Tests: goose upgrade path on a seeded install, backfill idempotence, sweep-no-kill regression, FK re-arm.
2. **Phase 2 — Global config API.** `internal/api/global.go`: GET + PUT folder variant (`validateDirPath`) + repo variant (createByRepo ordering, reattach, clone-atomicity, project-overlap 409) + gated switch/clear. Independently testable with curl; no frontend yet.
3. **Phase 3 — Session engine scope + spawn/status backend.** `SpawnOpts.Global`/`Info.Global` + counters + `ListGlobal`; sessions.go global branch (bash + agent + resume + tmux + reconcile + opencode capture); agents.go third pass. The fake-claude/fake-opencode suites extend with a global case; the task paths must diff clean. *This is the milestone's risk center — consider it two waves (bash+engine, then agent+status).*
4. **Phase 4 — Frontend view + reachability.** `/global` route + GlobalTaskPage + global session hooks + AgentTab loosening; bar integration (types, row, navigation); Settings Global section + config client. Depends on 2 (config UI) and 3 (everything else).
5. **Phase 5 — Hardening & E2E.** Managed-root/project-delete interlock (if not already in 2), restart-resume E2E (claude + opencode + tmux survivor), MCP parity spot-check (global session visible in `list_sessions`; read-only contract untouched), UAT.

**Research flags:** Phase 1's table rebuild is the one piece without a direct in-repo precedent at this exact shape (00012/00013 add columns; this rebuilds) — plan a migration rehearsal on a copy of a real install. Phase 3's opencode capture re-targeting is mechanical but subtle (the `PWD` pin and directory filter now match the global root — worth one host-gated e2e). Everything else follows established patterns and is unlikely to need phase-specific research.

## Open decision points (for the roadmap author)

- **Folder root: git repo or plain directory?** Recommended: plain directory (scratchpad semantics; nothing in the global path runs git). If UAT disagrees, restoring the git requirement is a one-line validator change.
- **Managed-root/project overlap policy.** Recommended: refuse at save + block at project delete (Anti-Pattern 4). The permissive alternative (shared dir, both gates counting each other's sessions) is more code for a rare case.
- **Bar row label.** `Global · Global Task` reads consistently with `project · task` rows; a Globe badge (mirroring the PR `#n` badge) is the lighter alternative. Cosmetic — safe to settle at UAT.
- **MCP `scope` filter on `list_sessions`** — defer to the banked v1.11 follow-up list unless trivially cheap during Phase 3.

## Sources

- Live codebase at v1.12 (this worktree): `internal/api/{sessions,agents,settings,projects,tasks,routes}.go`, `internal/session/{manager,session,agent}.go`, `internal/reaper/reaper.go`, `internal/store/migrations/00001–00016`, `cmd/kamacu/serve.go`, `internal/mcp/{sessions,tasks,projects,reviews}.go`, `web/src/{App.tsx,api/{sessions,agents,settings}.ts,pages/{TaskPage,SettingsPage}.tsx,components/{task/AgentTab,layout/ActiveSessionsBar}.tsx}` — all read in full or in the cited ranges (HIGH).
- `.planning/PROJECT.md` v1.13 milestone scope + v1.4/v1.9/v1.10 milestone histories (settled-decision precedents: managed-clone atomicity, singleton-row backfills, agent FK shape) (HIGH).

---
*Architecture research for: v1.13 Global Task (Kamacu)*
*Researched: 2026-08-25*
