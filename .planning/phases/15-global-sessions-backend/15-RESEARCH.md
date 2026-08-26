# Phase 15: Global sessions backend - Research

**Researched:** 2026-08-26
**Domain:** Additive global session scope on the existing Go session engine + spawn/status API widening (Kamacu v1.13, composition milestone — zero new dependencies)
**Confidence:** HIGH (every claim verified at file:line in this worktree this session)

## Summary

Phase 15 is the milestone's risk center and a pure composition phase: no new libraries, no new migrations (00017/00018 landed in Phase 13; the resume-id columns this phase writes already exist), no frontend. The work is five additive seams in existing files: (1) a `Global` scope flag on `internal/session` (`SpawnOpts`/`Info` + a global `Bash N` counter + `ListGlobal()`/`StopAllForScope()`), (2) a `scope:"global"` branch in `POST /api/sessions` covering plain bash, tmux mint `kamacu-global-<n>`, reattach, agent spawn (root+agent resolution from the singleton, read-at-use), engine-branched resume, csid persist, and the opencode capture re-target, (3) the `/api/agents/status` widening — a synthesized global entry in BOTH passes (manager-derived live + DB-derived post-restart) with non-nullable labels `projectName:"Global"` / `taskTitle:"Scratchpad"` (D-09/D-10), (4) the `GET /api/global` count fields flipping from forward-wired zeros to `ListGlobal()`-derived truth **plus the GCONF-04 gate's manager half widening in the same phase** (a newly-live class of sessions the Phase-14 gate cannot see would silently break the root-change 409 contract), and (5) synthesized labels for global sessions in `joinSessionContext` (GINT-02 MCP honesty).

Both roadmap-flagged spikes are resolved from source. **Spike 1 (wire widening):** emit `source:"global"` (extend the existing union) with `taskId:0`/`projectId:0` zero values and synthesized labels — zero TS type changes beyond the `source` union, and every frontend consumer except the ActiveSessionsBar is safe-by-construction because its lookups key on real task/project ids which never equal 0. The one interim artifact: until Phase 16 branches bar navigation on `source`, a live global agent renders one bar row navigating to the dead route `/projects/0/tasks/0` — acceptable mid-milestone (unreleased), and the co-phasing mandate forbids deferring the widening to avoid it. **Spike 2 (opencode capture re-target):** mechanically identical to the task path — the `PWD=<dir>` pin (manager.go:199) and the `directory == dir` filter are cwd-parameterized already; only the persist target changes (`UPDATE global_task` instead of `UPDATE tasks`). One host-gated e2e IS warranted and IS runnable on this host (opencode 1.18.22 present); CI covers the re-target via the injectable `discoverOpenCodeSession` stub.

**Primary recommendation:** Split into two sequential waves — Wave 1 (engine scope flag + bash surface: plain bash, tmux mint/reattach, `?scope=global` list + global reconcile, `joinSessionContext` labels, `GET /api/global` counts + gate widening), Wave 2 (agent spawn + resume + opencode capture re-target + both status passes). Wave 2 depends on Wave 1's `Info.Global`/`ListGlobal()`; Wave 1 is independently curl-verifiable; the task path must diff byte-for-byte clean in both.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Carry-Forward Locked (research + Phases 13–14 — do not reopen)

- **Representation:** additive scope discriminator on the session engine — NEVER `TaskID = 0` (already the dev-session overload, three load-bearing checks), never a sentinel row (research P2/P4).
- **Status feed:** synthesized server-side entries with non-nullable labels (`projectName:"Global"`, `taskTitle:"Scratchpad"` — D-09/D-10; strings start emitting in THIS phase); bar rows key by `sessionId`, not taskId (P6). Co-phased with the spawn path.
- **tmux mint:** `kamacu-global-<n>` (D-11/D-02) — stays inside the sweep's `kamacu-` prefix and `defaultTmuxLabel`'s last-numeric-segment parsing.
- **One-agent gate:** a second CONCURRENT global agent spawn → 409 (GVIEW-02); an exited agent never blocks — fresh spawn replaces (task parity, agents.go:31).
- **No auto-reap:** no Done-TTL analog, no PR reconcile pass — immortal until explicitly stopped (GSESS-04); Stop is scope-targeted, NEVER `StopAllForTask(0)` (GSESS-01).
- **Agent resolution:** read-at-use — spawn reads the singleton's `agent_id` FK (D-24); a live session keeps the agent it was spawned with.
- **Resume ids:** internal to the server (D-18 — never on any wire); persisted engine-keyed in the `global_task` singleton (00017 columns); cleared only via D-16 (root change).
- **GET /api/global count fields become real** in this phase (D-19 forward-wired them as zeros; `ListGlobal()` lands the truth behind the Phase-14 409 gate).
- **Activity exclusion (GINT-03):** the v1.12 task-keyed surface is unchanged by construction — verify, don't add filters (research component 8: reaper/activity unchanged-by-design).
- **MCP (GINT-02):** session tools handle task-less global sessions with honest synthesized labels — never 404 or garbled; read-only contract unchanged.

#### Spawn-gate error posture (new — D-28..D-34)

- **D-28: Unconfigured root → 409.** `scope:"global"` spawn with `root_path=''` + `github_repo=NULL` returns 409 "global root not configured" — the task-gate family ("task has no worktree", sessions.go:269), state conflict not malformed request. Never a silent `$HOME`/cwd fallback (research pattern 4).
- **D-29: Vanished root → distinct 409 naming the path.** Root configured but directory gone since PUT → 409 "global root no longer exists on disk: \<path\>" — deliberately SHARPER than task parity (tasks fall through to Spawn's stat pre-check → generic 500). The user can tell misconfiguration (fix in Settings) from disk rot (fix on disk). No boot revalidation (D-08 unchanged).
- **D-30: Gate applies to ALL kinds uniformly.** Agent, plain bash, tmux bash, and `resume:true` all hit the unconfigured/vanished gate — one gate before kind dispatch (mirrors how the worktree query feeds every kind today). No kind can silently land in `$HOME`.
- **D-31: Resume with no persisted id → 409.** `scope:"global"` + `resume:true` with no singleton id for the requested engine → 409 "no global \<engine\> session to resume". Never a silent fresh spawn — the user asked for a conversation back, not a blank terminal.
- **D-32: Cross-scope tmux reattach → 404.** `scope:"global"` + `reattach_tmux_name` pointing at a task-scoped tab (or vice versa) → 404 not-found-in-scope. The lookup runs scoped (`WHERE scope='global' AND name=?`); a foreign row simply doesn't exist in this scope. No scope-mismatch 409 branch, no information leak.
- **D-33: Gate order — root gates first.** Unconfigured → vanished → one-agent limit. Config-level checks are cheapest and most actionable; mirrors the task path (worktree resolution precedes the one-agent gate).
- **D-34: One-agent 409 copy mirrors task-gate voice.** e.g. "global agent already running" — same voice as "task has no worktree" and the Scratchpad 409s from Phases 13–14. Only RUNNING blocks (SC1 "second concurrent"); exited never blocks, fresh spawn replaces.

### the agent's Discretion

- `SpawnOpts.Global`/`Info.Global` field spelling, `ListGlobal()`/`StopAllForScope` signatures, counter wiring (research locks the shape, not the names).
- `agentStatusEntry` wire mechanics for the global entry — the FLAGGED SPIKE for planning research (`--research-phase`): nullable-vs-synthesized is settled (synthesized), the TaskID/ProjectID/discriminator field spelling ripples into 4+ frontend consumers.
- opencode capture re-target mechanics (`discoverOpenCodeSession` PWD/directory filter matching the global root; UPDATE `global_task` instead of `tasks`) — flagged for the planning-research spike, worth one host-gated e2e.
- Bash tab label spelling (task-parity "Bash N" is the default; not user-locked).
- csid persist-failure posture (tasks do `slog.Warn` non-fatal — mirroring is the default).
- Wave split if the phase feels heavy at planning time (engine+bash, then agent+status — waves, NOT separate phases; roadmap research flag).

### Deferred Ideas (OUT OF SCOPE)

None — discussion stayed within phase scope. Not selected for discussion (left to planning/discretion): stop surface shape, bash tab label spelling, resume-id capture timing details. GT-FUT-01..07 remain parked in REQUIREMENTS.md.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| GSESS-01 | Full task-parity semantics — persistent PTYs, detach/reattach with replay, live status, scope-targeted Stop | Engine is cwd-driven (manager.go:114-143) — `Global` flag rides the `Kind`/`Shell`/`TmuxName` zero-value pattern; per-session `POST /api/sessions/{id}/stop` (sessions.go:502) is id-keyed and works verbatim; hooks.go status receiver is id-keyed (working/waiting/idle flows unchanged); `StopAllForScope` is the new engine primitive, never `StopAllForTask(0)` |
| GSESS-02 | Post-restart reconcile + engine-branched Resume keyed on singleton ids | DB-derived pass analog reads `global_task.claude_session_id`/`opencode_session_id` (00017 columns verified); claude `transcriptExists` is cwd-agnostic (resume.go:16); opencode keys on ocsid alone; resume spawn = `{scope:"global", kind:"agent", resume:true}` with D-31 409 |
| GSESS-03 | Global tmux tabs survive restart, reattach invisibly | 00018 scope rows exist; sweep already scope-aware (serve.go:389, Phase 13); global `reconcileTmux` variant scoped `WHERE scope='global'`; reattach rides `new-session -A` name-keyed verbatim; `defaultTmuxLabel("kamacu-global-3")` → "Bash 3" (sessions.go:615) |
| GSESS-04 | Never auto-reaped — immortal until explicitly stopped | Verified-by-construction: Done-TTL selects `FROM tasks WHERE status='done'` (reaper.go:166), PR pass selects `source='github_pr'` (reaper.go:230) — a task-less global session is invisible to both; verify, don't touch |
| GVIEW-02 | Agent runs in global root cwd; 409 on second concurrent spawn | `create` global branch: singleton SELECT + agents JOIN (mirrors sessions.go:285-290), one-agent gate via `mgr.ListGlobal()` RUNNING check (mirrors :308-315), D-33/D-34 copy |
| GVIEW-03 | Bash tabs in global root cwd, same shell options as tasks | Plain bash: settings shell read-at-use verbatim (:398); tmux: `kamacu-global-<n>` mint with scope-scoped `COALESCE(MAX(n),0)+1 WHERE scope='global'`; D-30 gate covers both |
| GINT-02 | MCP session tools handle task-less sessions with honest labels | `list_sessions` rides the unfiltered list already; `joinSessionContext` needs a global branch — **per-entry via `info.Global`, OFF the taskID-keyed map** (see Pitfall 3); get/output/subscribe are id-keyed, work verbatim |
| GINT-03 | Activity stats/lists exclude global sessions | Verified-by-construction: activity.go:134-135 queries `FROM tasks t JOIN projects ... WHERE t.source='manual'` — no task row means exclusion; verify, don't add filters |
</phase_requirements>

## Project Constraints (from CLAUDE.md / AGENTS.md)

No `AGENTS.md` exists in the repo. The repo `CLAUDE.md` carries GSD-managed blocks (PROJECT.md project description, STACK.md, conventions placeholder, workflow enforcement). Directives that apply:

- **Stack:** Go backend + React frontend, single binary, local-only, SQLite storage, agent = real CLI in a PTY (never reimplemented). This phase is backend-only (curl/API-testable; frontend is Phase 16).
- **GSD workflow enforcement:** file changes go through GSD commands; planning artifacts stay in sync.
- **User-level git conventions (from ~/.claude/CLAUDE.md):** never mention or co-author Claude on commits/PRs; PRs open as drafts; PR descriptions lead with a short abstract; review comments short with empty global body when inline comments exist. (Relevant to later ship phases; commit messages in this phase follow repo style: `type(scope): message`.)
- **Conventions:** `.planning/CONVENTIONS.md` is empty ("populate as patterns emerge") — the in-repo code IS the convention source; the patterns documented below are the canonical precedents.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Global spawn gating (D-28..D-34) | API handler (`sessions.go` create) | — | Gates are API-layer policy exactly like "task has no worktree" (:269); the engine only validates Cwd existence (manager.go:137-143) |
| Root + agent resolution | API handler (read-at-use SELECT) | Storage (`global_task` singleton) | The engine treats Cwd as opaque; the singleton JOIN mirrors the task worktree+agent JOIN (:285-290); read per-spawn so config changes need no restart (D-24) |
| Scope discriminator + label counter | Session engine (`manager.go`) | — | `TaskID` is opaque to the engine; the flag must live where labels/listing/stopping branch (manager.go:335-344, :421-423, :456) |
| PTY lifecycle, replay, attach | Session engine (UNCHANGED) | — | Ring buffer, Attach/Detach, Stop are id-keyged — zero changes needed |
| Status feed widening | API (`agents.go`) | — | Synthesized entries appended after existing passes; singleton read = no query fan-out |
| Restart durability (tmux) | Storage (`tmux_sessions` scope rows) + reconcile pass | — | Rows persist; `reconcileTmux` variant + sweep (already scope-aware) surface survivors |
| Restart resume (agent) | Storage (`global_task` resume ids) + DB-derived pass | — | Ids written at spawn/capture; DB-derived pass synthesizes the resumable entry |
| Label synthesis (3 consumers) | API | — | agents.go both passes, `joinSessionContext` (MCP), bar rides the feed — Pattern 3 (never LEFT-JOIN surgery) |
| Reaping / Activity exclusion | NOTHING (unchanged-by-design) | — | Task-keyed queries cannot see task-less sessions; GSESS-04/GINT-03 hold by construction |
| Frontend consumption | Browser (Phase 16) | — | Wire spelling decided HERE so Phase 16 is additive |

## Standard Stack

**Zero new dependencies.** [VERIFIED: codebase — go.mod, web/package.json] This is a composition milestone over the pinned stack (milestone research SUMMARY: "Add nothing"). No `npm install`, no `go get`. The "stack" for this phase is the in-repo module surface being widened:

### Core (in-repo seams to widen)

| Module | Current Anchor | What Widens | Why This Tier Owns It |
|---------|---------------|-------------|----------------------|
| `internal/session/manager.go` | `SpawnOpts` (:54), label switch (:335-344), `listWhere` (:427), `StopAllForTask` (:456) | `Global bool` on SpawnOpts/Info + `globalCounter` + `ListGlobal()` + `StopAllForScope()` | Scope is an engine-level property; labels/listing/stopping all branch here |
| `internal/api/sessions.go` | `create` (:223), `list` (:65), `reconcileTmux` (:144), `joinSessionContext` (:917), `captureOpencodeSessionAsync` (:793) | `scope` body field + global branch (all kinds + resume + reattach + mint), `?scope=global` list filter, global reconcile, global labels, capture persist re-target | Every task/worktree coupling lives HERE, not in the engine (architecture-locked finding) |
| `internal/api/agents.go` | `agentStatusEntry` (:26), `status` (:44 — manager pass `TaskID <= 0` filter at :50, DB pass :156-214) | Synthesized global entry in both passes; `source:"global"` | The bar feed is the visibility contract; widening only here keeps consumers additive |
| `internal/api/global.go` | `deriveGlobalState` (:223 — `Live.Agent/Bash = 0` seams), `globalLiveBlockers` (:123 — tmux-only) | Counts from `ListGlobal()`; blockers' manager half (GCONF-04 must bite once global PTYs exist) | D-19 forward-wired these exact seams for this phase |
| `internal/store` (migrations 00017/00018) | `global_task` singleton (resume-id columns), `tmux_sessions` scope + XOR CHECK | NOTHING — write targets only | Phase 13 landed the schema; this phase only reads/writes it |

### Supporting (consumed verbatim, verify-don't-touch)

| Module | Role | Verification |
|--------|------|--------------|
| `internal/reaper/reaper.go` | GSESS-04 non-reaping | Task-keyed queries (:166, :230) — global invisible by construction |
| `internal/api/activity.go` | GINT-03 exclusion | `FROM tasks ... source='manual'` (:134-135) — same |
| `internal/api/hooks.go` + `internal/ws` | Working/waiting/idle status, attach replay | Id-keyed; claude AND opencode hooks fire identically in a global PTY |
| `internal/api/resume.go` | `transcriptExists` | Glob is repo-agnostic (cwd-independent) — global resume needs no change |
| `internal/mcp/sessions.go` | GINT-02 | Id-keyed tools work verbatim; only `joinSessionContext` (server side) needs the label branch |
| `cmd/kamacu/serve.go` | Sweep | Already scope-aware (`WHERE scope = 'global' OR task_id IN (SELECT id FROM tasks)`, :389) — Phase 13 done |

**Installation:** none.

**Version verification:** N/A (no packages added). Engine versions pinned in `go.mod` unchanged: goose v3.27.1, modernc.org/sqlite v1.52.0. [VERIFIED: go.mod]

## Package Legitimacy Audit

**This phase installs zero external packages** — a zero-new-dependencies composition phase over the existing Go module and React workspace (verified against `go.mod` and milestone STACK research). No registry checks required; no `[ASSUMED]` package names appear in this research.

## Architecture Patterns

### System Architecture Diagram

```
                        ┌──────────────────────────────────────────────┐
                        │  Clients: curl (Phase 15) / Phase-16 UI      │
                        └──────┬───────────────────────┬───────────────┘
             POST /api/sessions {scope:"global"}   GET /api/agents/status
                               │                         │
┌──────────────────────────────▼─────────────────────────▼──────────────────┐
│ internal/api/sessions.go create()          internal/api/agents.go status() │
│                                                                            │
│  [GATE ORDER — D-33, before kind dispatch]     PASS 1 (tasks, manager-     │
│   1. root_path='' → 409 "global root            derived) — UNCHANGED      │
│      not configured"      [D-28]               PASS 1b (NEW): mgr.List()  │
│   2. dir gone → 409 naming path [D-29]            → Kind==agent && Global │
│   3. (agent only) RUNNING global                  → newest wins → ONE     │
│      agent → 409 "global agent                     synthesized entry      │
│      already running"          [D-34]          PASS 2 (tasks, DB-derived  │
│                                                    post-restart) — UNCH. │
│  [RESOLVE — read-at-use, singleton]             PASS 2b (NEW): global_task│
│   SELECT root_path, agent_id, csid,               csid/ocsid + engine-    │
│   ocsid FROM global_task JOIN agents                branched check → ONE │
│   WHERE id=1                                      resumable entry        │
│                                                                            │
│  [DISPATCH by kind]                                                        │
│   agent → Spawn{Cwd:root, Global:true,          Every global entry:       │
│             Engine, ExtraArgs|AgentArgs,        source:"global",          │
│             ResumeSessionID?} → UPDATE          taskId:0, projectId:0,    │
│             global_task.claude_session_id       taskTitle:"Scratchpad",   │
│             → (opencode) capture poll →         projectName:"Global",     │
│               UPDATE global_task.               prNumber:null             │
│   bash  → settings shell (verbatim) ──┐                                    │
│   tmux  → n = MAX(n)+1 WHERE          │                                  │
│            scope='global' → INSERT    │                                  │
│            (task_id NULL, 'global')   │                                  │
│            → Spawn{TmuxName:           │                                  │
│            "kamacu-global-<n>"}       │                                  │
│   resume → D-31: no engine id → 409   │                                  │
│   reattach → WHERE scope='global'     │                                  │
│            AND name=? → 404 if foreign│                                  │
└────────────┬──────────────────────────┼───────────────────────────────────┘
             │                          │
┌────────────▼──────────┐  ┌────────────▼─────────────────────────────────┐
│ internal/session      │  │ SQLite                                        │
│  Manager.Spawn        │  │  global_task (id=1): root_path, agent_id,     │
│   Global flag →       │  │    claude_session_id, opencode_session_id     │
│   label "Bash N" via  │  │  tmux_sessions: scope='global' rows           │
│   globalCounter;      │  │    (task_id NULL, XOR CHECK, name UNIQUE)     │
│   "Agent" for agents  │  │  tasks/projects — UNTOUCHED (byte-for-byte)   │
│  ListGlobal() /       │  └───────────────────────────────────────────────┘
│  StopAllForScope()    │  ┌───────────────────────────────────────────────┐
│  PTY/ring/attach:     │  │ Unchanged-by-design: reaper (never sees       │
│  ZERO changes         │  │ global), activity (task-JOIN), ws, hooks,     │
│                       │  │ sweep (already scope-aware, Phase 13)         │
└───────────────────────┘  └───────────────────────────────────────────────┘
```

Trace the primary use case: `PUT /api/global {"root_path":"/repo"}` (Phase 14) → `POST /api/sessions {"scope":"global","kind":"agent"}` → gates pass → singleton read → `mgr.Spawn` in the root cwd → csid persisted → `/api/agents/status` shows the live entry within the 5s poll → restart kills the PTY but the id survives → status DB-pass offers `resumable:true` → `POST {"scope":"global","kind":"agent","resume:true}` resumes.

### Recommended Change Shape

No new files are required (a `sessions_global_test.go` / test additions are the only new files). The changes are branches inside existing functions:

```
internal/session/manager.go   — Global flag, globalCounter, ListGlobal, StopAllForScope
internal/session/session.go   — global field on Session, Global on Info (wire: "global,omitempty")
internal/api/sessions.go      — create() scope branch, list() ?scope=global, global
                                 reconcileTmux, joinSessionContext global labels,
                                 captureOpencodeSessionAsync persist re-target
internal/api/agents.go        — both passes widened (synthesized global entries)
internal/api/global.go        — deriveGlobalState counts real, globalLiveBlockers
                                 manager half (GCONF-04 bite)
```

### Pattern 1: Explicit scope flag, never a sentinel id (carry-forward)
**What:** `SpawnOpts.Global bool` / `Info.Global bool` — zero value = current behavior, exactly like `Kind`, `Shell`, `TmuxName` before it. [VERIFIED: manager.go:54-79]
**When to use:** the label switch must check `opts.Global` BEFORE the `opts.TaskID == 0` dev arm (manager.go:338), or global bash tabs get `bash #N` dev labels.

### Pattern 2: Synthesized JOIN entries instead of LEFT-JOIN surgery (carry-forward)
**What:** the global status/context entries are appended by dedicated passes reading `global_task` — the tasks→projects→agents INNER JOINs (:99-103, :164-168, sessions.go:934-939) are never rewritten.
**When to use:** every place a row-less entity must appear on a JOIN-backed wire. Cost: the global entry's field set lives in one more place — contained, explicit, unit-testable.

### Pattern 3: One-gate-before-kind-dispatch with honest 409s (D-28..D-33)
**What:** the root gates run before kind dispatch, exactly where the worktree query feeds every kind today (sessions.go:282-304). The current handler's gate placement (`reattach` needs task :262, agent needs task :269, tmux needs task :408) interleaves gates with kinds; the global branch hoists ONE root gate that covers agent + bash + tmux + resume uniformly.

### Pattern 4: Read-at-use singleton resolution (D-24)
**What:** spawn reads `global_task` fresh per request (`SELECT ... FROM global_task g JOIN agents a ON a.id = g.agent_id WHERE g.id = 1`) — an agent change applies at the next Start, no restart. Mirrors `loadGlobalConfig` (global.go:198) and the task JOIN (:285-290).

### Pattern 5: Warn-only persistence degradation
**What:** csid persist and tmux label back-fill failures are `slog.Warn` non-fatal (:461-463, :490-493) — a failed write costs resume/label only, never the live session. The global variants mirror this (CONTEXT: mirroring is the default).

### Anti-Patterns to Avoid
- **Side-channel spawn endpoint:** a separate `/api/global/sessions` would fork the engine-branched spawn logic — widen `POST /api/sessions` with `scope` instead (CONTEXT integration point).
- **`StopAllForTask(0)`** — kills every dev terminal (manager.go:456 matches `taskID == 0`).
- **LEFT-JOIN surgery on the hot status queries** — append synthesized entries instead.
- **Forking the capture poller** — parametrize the persist step (owner enum or small func), never copy `captureOpencodeSessionAsync`.
- **Silent `$HOME` fallback** on an unconfigured root (Pattern 4 of milestone research; D-28 forbids).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Global session visibility | New status endpoint / bar feed | Widen `/api/agents/status` both passes | The 5s poll already fans to every consumer; a second feed forks the truth |
| Global spawn | New route + handler copy | `scope` field on the existing `create` | Engine-branched spawn logic (claude argv, opencode env, resume, extras) stays single-source |
| opencode id capture for global | A second poller | Parametrized persist target on the existing poller | Poll/timing/exit conditions are identical; only the UPDATE target differs |
| Scope-targeted bulk stop | TaskID-sentinel tricks | `StopAllForScope()` engine method | `StopAllForTask` fans out by task id; 0 = dev terminals |
| tmux liveness probes | Hand-rolled `has-session` parsing | `tmux.Client.HasSession` (exact-match `=name` embedded) | tmux 3.4 prefix-matching trap (tmux.go:78) — the client already handles it |
| Restart reconcile for global tmux | New sweep/reconcile machinery | Scoped variant of `reconcileTmux` | Same survivor/GC logic; only the WHERE clause changes |
| Labels for row-less sessions | Nullable wire fields + frontend null-guards | Server-side synthesis ("Global"/"Scratchpad") | Keeps TS non-nullable; zero consumer changes beyond the `source` union |

**Key insight:** every deceptively complex problem in this phase (engine-branched resume, opencode discovery, tmux durability, status fan-out) already has a task-scoped solution in the repo — the phase's entire risk lives in re-targeting those solutions without perturbing the task paths they serve today.

## Common Pitfalls

### Pitfall 1: The invisible global session (milestone P1)
**What goes wrong:** the global agent spawns and works but never appears in the status feed or post-restart Resume.
**Why it happens:** three independent filters — manager pass skips `TaskID <= 0` (agents.go:50); the INNER-JOIN metadata query drops task-less rows (agents.go:99-103 + :127-129 `continue`); the DB-derived pass requires a tasks row + worktree (agents.go:164-168). A fourth lives in `joinSessionContext` (sessions.go:921).
**How to avoid:** widen BOTH status passes AND the session-context labels in this phase (the co-phasing mandate — SC3 requires the entry visible live and post-restart). Regression test: fake-claude global spawn → entry appears in `/api/agents/status` (both passes).
**Warning signs:** "the session runs but the bar says No active sessions"; post-restart global view shows no Resume while task agents do.

### Pitfall 2: `TaskID = 0` is already taken (milestone P4)
**What goes wrong:** reusing 0 for "no task" makes global sessions share the dev-session ID-space — a scope stop nukes dev terminals; `ListByTask(0)` mixes scopes; labels collide.
**Why it happens:** three load-bearing `TaskID == 0 / <= 0` checks (manager.go:338, sessions.go:921, agents.go:50) mean "dev session".
**How to avoid:** explicit `Global` flag, checked BEFORE the zero arm; `ListGlobal()`/`StopAllForScope()` as new engine methods.
**Warning signs:** any diff passing literal `0` to `StopAllForTask`/`ListByTask` in the global path; global bash tabs labeled `bash #N`.

### Pitfall 3: `joinSessionContext`'s taskID-keyed map leaks Scratchpad labels onto dev sessions (NEW — found this session)
**What goes wrong:** if the global context is stored in the `out[info.TaskID]` map under key 0 (global sessions have TaskID 0), EVERY TaskID-0 session — including `/terminal` dev sessions — picks up `taskTitle:"Scratchpad"` in `list` (:127-131 does `ctxByTask[info.TaskID]`) and `getSession` (:976-977).
**Why it happens:** the map is keyed by the very field global sessions don't have; 0 is the natural (wrong) key.
**How to avoid:** synthesize the global context per-entry off the map, gated on `info.Global` — never insert a 0-keyed entry. Same discipline in the agents.go manager pass: collect the global newest OUTSIDE the `newest[TaskID]` map.
**Warning signs:** dev terminals showing "Scratchpad" in MCP `list_sessions`; a test asserting dev sessions keep empty JOIN fields.

### Pitfall 4: The GCONF-04 gate silently stops biting (NEW — found this session)
**What goes wrong:** Phase 14's `globalLiveBlockers` (global.go:123-129) only counts the tmux half — `Live.Agent = 0; Live.Bash = 0` are documented zero-seams. The moment this phase makes global agent/bash PTYs possible, a root change while a global agent runs would pass the gate that exists to block it (GCONF-04 regression, milestone P7's split-brain cwd).
**How to avoid:** widen `globalLiveBlockers`' manager half (via `ListGlobal()` — any RUNNING global PTY blocks) and the count fields in the SAME phase as the spawn path. This is the D-13 gate the CONTEXT mandates ("GET /api/global count fields become real").
**Warning signs:** the phase plan touches `deriveGlobalState` but not `globalLiveBlockers`; a test changing the root while a fake global agent runs returns 200.

### Pitfall 5: Interim bar dead-nav row (accept, don't fix)
**What goes wrong:** after this phase, a live global agent renders in the existing ActiveSessionsBar as a row labeled "Global · Scratchpad" navigating to `/projects/0/tasks/0` (dead route) until Phase 16 branches navigation on `source`.
**Why it happens:** the bar renders all live entries and hardcodes the task route (:133-135); `key={entry.taskId}` and the label work (labels are synthesized; the key 0 is unique — at most one global entry exists thanks to the one-agent gate + the DB pass skipping manager-covered ids).
**How to avoid:** can't and shouldn't within this phase — deferring the status widening violates the co-phasing mandate. Accept the interim artifact (unreleased milestone); Phase 16's bar work (source branch, `sessionId` key, `/global` route) is already planned (P6).
**Warning signs:** anyone proposing to defer the status widening to Phase 16 to keep the bar clean.

### Pitfall 6: `UNIQUE(task_id, n)` doesn't constrain global rows
**What goes wrong:** the mint race backstop changes — in SQLite, NULL `task_id` values are distinct under `UNIQUE(task_id,n)`, so two concurrent global mints with the same `n` do NOT violate that constraint.
**How to avoid:** rely on `name UNIQUE` (the real backstop — same read-then-insert race posture tasks accept today, sessions.go:414-431) and the scope-scoped counter `COALESCE(MAX(n),0)+1 WHERE scope='global'`. Single-user localhost makes the window acceptable, as today.
**Warning signs:** a plan claiming `UNIQUE(task_id,n)` protects the global mint.

### Pitfall 7: tmux name prefix matching
**What goes wrong:** `has-session -t kamacu-1` prefix-matches `kamacu-1-10` (tmux 3.4, tmux.go:78) — any hand-rolled probe without `=name` can hit the wrong session.
**How to avoid:** all probes go through `tmux.Client.HasSession`/`KillSession` (exact match embedded, verified :89/:104). The `kamacu-global-<n>` scheme is structurally collision-free: task names are `kamacu-<digits>-<digits>` and "global" is non-numeric, so no task name can prefix-match a global name or vice versa. `defaultTmuxLabel` parses the LAST `-` segment (:615-622) → "Bash N" works.
**Warning signs:** raw `tmux has-session` execs in new code.

### Pitfall 8: The capture poll writes the wrong table
**What goes wrong:** a global opencode spawn whose poll persists `UPDATE tasks SET opencode_session_id = ? WHERE id = ?` with a zero/absent id silently no-ops (or corrupts task 0 semantics) — restart-resume for global opencode never works (GSESS-02 half).
**How to avoid:** parametrize the persist target when the poller is launched (:476 is the only call site) — owner enum (`taskID int64` vs a global marker) or a persist-closure. Assert the write lands in `global_task` in a CI test with the injected `discoverOpenCodeSession` stub.
**Warning signs:** two copies of `captureOpencodeSessionAsync`; a `taskID` parameter holding a sentinel.

### Pitfall 9: Task-path regression (the milestone's stated risk center)
**What goes wrong:** widening `create`/`status`/`joinSessionContext` perturbs the task paths — spawn argv drift, status feed shape drift, JOIN label drift.
**How to avoid:** the fake-claude suite (byte-for-byte argv assertions, agent_integration_test.go) and the full sessions/agents tests stay green untouched; new global tests are additive. The engine's claude/custom arms (manager.go:152-243) are NOT modified — only the label switch and SpawnOpts gain fields.
**Warning signs:** any diff touching the argv construction; a task test needing an update (vs a new global test).

### Pitfall 10: Resumable derivation without a worktree
**What goes wrong:** copying the task resumable check verbatim requires `m.wtp.Valid` (agents.go:135) — the global has no worktree, so the check always fails and the global is never resumable.
**How to avoid:** the configured root plays the worktree's role ("the directory the agent runs in still exists" — milestone P1): global resumable = engine-branched id check (claude: csid + `transcriptExists`, which is cwd-agnostic — resume.go:16; opencode: ocsid alone) + root_path non-empty. The D-29 vanished-root check at spawn time covers the pathological case.
**Warning signs:** a global resumable branch referencing `worktree_path`.

## Code Examples

All examples are shape sketches derived from verified in-repo precedents (not copy-paste-ready code) — the planner maps them to tasks.

### Engine: label switch + scope surface (manager.go)
```go
// Source: internal/session/manager.go:335-344 (verified) + ARCHITECTURE Pattern 2
switch {
case kind == KindAgent:
    label = "Agent" // one agent per scope — no counter (API enforces)
case opts.Global: // NEW — MUST precede the TaskID==0 dev arm
    m.globalCounter++
    label = fmt.Sprintf("Bash %d", m.globalCounter)
case opts.TaskID == 0:
    m.counter++
    label = fmt.Sprintf("bash #%d", m.counter)
default:
    m.taskCounters[opts.TaskID]++
    label = fmt.Sprintf("Bash %d", m.taskCounters[opts.TaskID])
}

// ListGlobal: listWhere(s.global) — same comparator contract as ListByTask (:421-423)
// StopAllForScope: collect RUNNING sessions where s.global, concurrent Stop + Wait
//   — the StopAllForTask shape (:456-475) with the predicate swapped. NEVER taskID 0.
```

### API: the global gate + resolution block (create, before kind dispatch)
```go
// Source: sessions.go:267-304 task analog (verified) + D-28..D-33
if req.Scope == "global" {
    var rootPath string
    var agentID int64
    err := h.db.QueryRow(
        `SELECT g.root_path, g.agent_id FROM global_task g JOIN agents a ON a.id = g.agent_id WHERE g.id = 1`,
    ).Scan(&rootPath, &agentID) // read-at-use (D-24); ErrNoRows = corrupted invariant, 500
    // ... D-28: rootPath == "" → 409 "global root not configured"
    // ... D-29: os.Stat fails / not dir → 409 "global root no longer exists on disk: <path>"
    // ... one-agent gate (agent kind): mgr.ListGlobal() RUNNING check → 409 "global agent already running" (D-34)
    // ... D-31 (resume): engine-branched singleton-id check → 409 "no global <engine> session to resume"
    opts.Cwd, opts.Global = rootPath, true
}
```

### API: tmux mint (scope-scoped)
```go
// Source: sessions.go:417-432 task mint (verified) — n from DB, INSERT before Spawn
var n int64
h.db.QueryRow(`SELECT COALESCE(MAX(n),0)+1 FROM tmux_sessions WHERE scope = 'global'`).Scan(&n)
name := fmt.Sprintf("kamacu-global-%d", n)
label := fmt.Sprintf("Bash %d", n)
// INSERT INTO tmux_sessions (task_id, scope, n, name, label) VALUES (NULL, 'global', ?, ?, ?)
// XOR CHECK (00018): task_id NULL ⇔ scope='global' — both must agree or the INSERT fails.
// Real race backstop = name UNIQUE (NULL task_ids are DISTINCT under UNIQUE(task_id,n)).
```

### API: synthesized status entry (agents.go, both passes)
```go
// Source: agents.go:26-38 wire contract, :138-150 entry shape (verified) + D-10
// Manager pass: newest global agent session (collected OUTSIDE the newest[TaskID] map — Pitfall 3):
entries = append(entries, agentStatusEntry{
    TaskID: 0, ProjectID: 0,           // zero values — TS number types stay non-nullable
    SessionID: info.ID, Status: info.AgentStatus,
    Resumable: resumable,               // engine-branched; root_path plays worktree's role
    PRNumber: nil, Source: "global",   // the discriminator the frontend branches on
    TaskTitle: "Scratchpad", ProjectName: "Global", // D-09/D-10 locked strings
})
// DB-derived pass (post-restart, only when no manager entry): same shape with
// SessionID:"", Status:"exited", Resumable:true — reading global_task's csid/ocsid.
```

### API: capture persist re-target (sessions.go:476 call site)
```go
// Source: sessions.go:475-477 + :793-817 (verified). Parametrize, don't fork:
if agentEngine == "opencode" && !req.Resume {
    if req.Scope == "global" {
        go captureOpencodeSessionAsync(h.db, sess.Done(), globalOwner, opts.Cwd) // UPDATE global_task
    } else {
        go captureOpencodeSessionAsync(h.db, sess.Done(), taskOwner(req.TaskID), opts.Cwd) // UPDATE tasks
    }
}
// The PWD pin (manager.go:199 "PWD="+dir) and discoverOpenCodeSession's
// directory filter (e.Directory != dir, :766) are cwd-parameterized — unchanged.
```

## Spike Resolutions (the roadmap's two research flags)

### Spike 1: `agentStatusEntry` wire widening — RESOLVED: synthesized, `source:"global"`, zero ids

**The settled half (CONTEXT):** server-side synthesis, non-nullable labels. **The open half (field spelling):** resolved as follows.

**Recommendation:** extend the existing `Source` union with `"global"`; emit `TaskID:0` and `ProjectID:0` (not nullable, not a separate `scope` field).

**Why (verified consumer-by-consumer this session):**

| Consumer | Lookup key | Effect of `{source:"global", taskId:0, projectId:0}` |
|----------|-----------|------------------------------------------------------|
| `TaskPage.tsx:87` | `find(e => e.taskId === task.id)` | Never matches (task ids ≥ 1) — safe by construction |
| `AgentTab.tsx:58` | `find(e => e.taskId === task.id)` | Same — safe |
| `TaskCard.tsx: / PRCard.tsx` | task/pr linkage per card | Never matches — safe |
| `ProjectSidebar.tsx:41-44` | `waitingByProject.get(entry.projectId)` (build) → read by `project.id` | Gains a dead 0-key no project reads — safe |
| `StatusDot.tsx` | status/exitCode only | Renders fine |
| `ActiveSessionsBar.tsx:129-135` | renders ALL live entries; `key={taskId}`; nav `/projects/${projectId}/tasks/${taskId}` | **The one interim artifact:** a live global row navigates to dead `/projects/0/tasks/0` until Phase 16 branches on `source` (Pitfall 5). Key 0 is unique — the one-agent gate + the DB-pass skip guarantee at most one global entry. |

**Why zero ids and not nullable:** `AgentStatusEntry` types `taskId: number; projectId: number` non-optional (web/src/api/agents.ts:4-16) — zero values require zero TS changes in this phase (Phase 16 widens only the `source` union + the bar branch); SQLite `INTEGER PRIMARY KEY` rowids start at 1 [ASSUMED — standard SQLite behavior; also enforced by every lookup being an equality against real ids]. A separate `scope` field would add a SECOND discriminator alongside `source` at every existing `source ===` branch site (ActiveSessionsBar.tsx:268, TaskPage.tsx:105/278) — strictly worse than extending the union the code already branches on.

**String content (locked):** `projectName:"Global"`, `taskTitle:"Scratchpad"` (13-CONTEXT D-09/D-10) — the bar renders `Global · Scratchpad`.

### Spike 2: opencode capture re-target — RESOLVED: mechanical re-target + ONE host-gated e2e (warranted and runnable)

**Mechanics (verified):** the discovery pipeline is cwd-parameterized end-to-end — `PWD=<dir>` pin at Spawn (manager.go:199, inside the opencode-gated env block), `discoverOpenCodeSession` runs `opencode session list` with `cmd.Dir=dir` + `PWD=dir` (:726-735), and `parseOpenCodeSessionList` filters `e.Directory != dir` (:766) with most-recently-updated-wins (:771-774). When `<dir>` is the global root, the "project" opencode resolves from `$PWD` IS the global root — the filter matches exactly as it does for a worktree. **The only change is the persist target:** `UPDATE global_task SET opencode_session_id = ? WHERE id = 1` instead of the tasks UPDATE (:811).

**Edge analysis:** a shared-directory collision (global root = a directory opencode also uses for task/manual sessions) resolves the same way tasks already accept it — most-recently-updated wins (documented task behavior, sessions.go:769-771 comment). Task worktrees as folder roots are blocked by D-27 (worktrees live under `~/.kamacu/worktrees`, serve.go:251) unless the user moved `worktree_base`.

**E2E verdict: YES, one host-gated test.** The re-target touches a persistence path driven by an async poll with injected-stub coverage in CI — the real-binary path (`PWD` semantics against opencode's project resolution in a NON-worktree directory) has never been exercised. Host is provisioned: opencode 1.18.22, git 2.43, tmux 3.4 all on PATH. Shape: temp git repo as folder root → PUT /api/global → spawn global opencode agent → drive (or wait for) a first turn → assert `global_task.opencode_session_id` captured → restart-simulate → resume offered. Gate on `exec.LookPath("opencode")` per the house skip convention (tmux tests, sessions_test.go:673). The fuller restart E2E (both engines + tmux survivor) remains Phase 17's gate.

### Wave-split assessment — RESOLVED: split into two sequential waves (coarse granularity)

**Wave 1 — engine + bash surface** (independently curl-verifiable, no agent machinery):
- `SpawnOpts.Global`/`Info.Global` + `globalCounter` + `ListGlobal()` + `StopAllForScope()` (manager.go, session.go)
- `create()` global bash branches: plain bash (settings shell verbatim), tmux mint `kamacu-global-<n>` + scope-scoped INSERT, reattach (D-32 scoped lookup)
- Gates D-28/D-29/D-30 for every kind (agent gate ships with Wave 2's agent branch)
- `list()` `?scope=global` + global `reconcileTmux` variant
- `joinSessionContext` global labels (GINT-02 for bash tabs; per-entry off the map — Pitfall 3)
- `GET /api/global` counts real + `globalLiveBlockers` manager half (Pitfall 4)
- Tests: tmux-integration (host-guarded), dev-session label non-regression, D-32 cross-scope 404, task-path byte-for-byte

**Wave 2 — agent + status** (depends on Wave 1's `Info.Global`/`ListGlobal()`):
- Agent spawn: singleton root+agent resolution, D-33 gate order, D-34 one-agent 409, resume (D-31, engine-branched), csid persist to `global_task`
- `captureOpencodeSessionAsync` persist re-target + CI stub test + the host-gated e2e
- `agents.go` BOTH passes widened + `source:"global"` entries
- Tests: fake-claude global spawn/status (live + post-restart DB pass), one-agent 409, resume argv, opencode capture, GSESS-04/GINT-03 verify-by-construction assertions

**Why waves, not separate phases:** the roadmap mandates it (waves within the phase; the status widening ships with the spawn path). Why sequential, not parallel: both waves touch `sessions.go create()` — the same function body. With `parallelization: true` at coarse granularity, two sequential plans is the right shape; Wave 1 merges only if planning finds it small.

## Test Strategy

*(`workflow.nyquist_validation` is explicitly `false` in `.planning/config.json` — the templated Validation Architecture section is skipped. This section is practical guidance for the planner.)*

**Harness inventory (all verified present):**
- `internal/api/testdata/fake-claude` + `fake-opencode` — committed stub binaries; integration tests build a real `session.Manager` + httptest server against them (agent_integration_test.go:31-50, opencode_resume_test.go:31-47)
- `discoverOpenCodeSession` — injectable package var (sessions.go:726); CI stubs discovery, the host-gated e2e uses the real binary
- tmux tests skip-guard on `exec.LookPath("tmux")` with per-test sockets + KillServer cleanup (sessions_test.go:673, :681) — tmux 3.4 present here
- Existing suites that must stay green untouched: `sessions_test.go` (2330 lines — spawn/argv/tmux/reconcile/resume/subscribe), `agents_test.go` (both status passes), `opencode_resume_test.go`, `global_test.go` (Phase 14 wire + gates)

**New test coverage map (requirement → test):**

| Requirement | Test | Type |
|-------------|------|------|
| D-28/D-29 gates | unconfigured spawn → 409; PUT a root; delete the dir; spawn → 409 naming path | CI (no tmux needed) |
| D-30 uniform gating | bash + tmux + agent + resume all hit the root gates | CI + tmux-guarded |
| D-31 | resume w/o singleton id → 409 per engine | CI |
| D-32 | global reattach of a task row → 404; task reattach of a global row → 404 | CI |
| D-33/D-34 | second concurrent global agent → 409 "global agent already running"; exited → fresh spawn replaces | CI (fake-claude) |
| GSESS-01/SC3 | global entry in `/api/agents/status` LIVE (manager pass) AND post-restart (DB pass) with non-nullable labels | CI (fake-claude) |
| GSESS-02 | restart-sim: csid in global_task → resumable entry; resume spawn argv engine-branched | CI (fake-claude + stub) |
| GSESS-03 | global tmux tab survives restart-sim: row → reconcile surfaces orphaned entry; reattach keeps label | tmux-guarded |
| GSESS-04/GINT-03 | assert-by-construction: reaper/activity queries unchanged + a seeded global session absent from activity | CI |
| GINT-02 | MCP list/get on global sessions carry Scratchpad labels; dev sessions keep empty labels (Pitfall 3 regression) | CI |
| Capture re-target | stub discovery → `global_task.opencode_session_id` written (never tasks) | CI + one host-gated e2e |
| GCONF-04 bite | root PUT while fake global agent RUNNING → 409 reasons | CI |
| Task-path regression | full existing suites green, byte-for-byte | CI |

**Quick run:** `go test ./internal/... ` ; scoped: `go test ./internal/api -run 'TestGlobal|TestSession' -count=1`. Host-gated: `go test ./internal/api -run TestGlobalOpencodeCapture -tags host -count=1` (naming illustrative).

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `tmux_sessions.task_id NOT NULL` (00005) | nullable + `scope` XOR CHECK (00018) | Phase 13 | Global rows already representable; this phase only INSERTs them |
| Startup sweep INNER-JOIN known-set | `WHERE scope='global' OR task_id IN (...)` (serve.go:389) | Phase 13 | Global tmux tabs survive restarts — no sweep work in this phase |
| Single-scope session engine (task/dev via TaskID) | + explicit `Global` flag (this phase) | v1.13 | Third scope; zero-value back-compat keeps dev/task paths byte-identical |
| `GET /api/global` counts = 0 seams | `ListGlobal()`-derived (this phase) | v1.13 | D-19 wire contract stable; values become real without a wire change |

**Deprecated/outdated:** nothing in the pinned stack is being replaced this phase.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | SQLite `INTEGER PRIMARY KEY` rowids start at 1, so `taskId:0`/`projectId:0` never equal a real task/project id | Spike 1 | Low — even if violated, frontend lookups are equality-based and would simply never match; no crash path |
| A2 | The interim dead-nav bar row (Phase 15 shipped, Phase 16 not) is acceptable because v1.13 is unreleased mid-milestone | Pitfall 5 | Low — cosmetic, local-only, self-heals at Phase 16; if the milestone shipped between phases it would be a visible bug |
| A3 | `globalLive{Bash}` counts plain-bash RUNNING sessions with tmux counted separately (the `{agent, bash, tmux}` triple implies it) | Open Questions / Wave 1 | Low — wire shape is fixed (D-19); only the derivation rule needs one decision |

All other claims in this research were verified directly against the codebase this session (see Sources).

## Open Questions (RESOLVED)

1. **`globalLive{Bash}` derivation rule** — plain-bash only, or all non-agent non-tmux? The Phase-14 wire ships `{agent, bash, tmux}` as separate fields, implying bash excludes tmux (tmux has its own field). One-line decision for planning; recommend plain-bash RUNNING count with tmux separate (matches the field triple).
   - What we know: the wire shape (D-19) and the tmux derivation (already real, global.go:231).
   - What's unclear: whether "bash" was intended inclusive of tmux.
   - Recommendation: plain-bash-only; the fields are disjoint by name.
   - **RESOLVED:** plain-bash RUNNING count only, tmux disjoint — adopted in 15-01-PLAN.md Task 3.
2. **Bulk stop surface** — `StopAllForScope()` is a locked engine primitive with possibly ZERO Phase-15 call sites (the per-session `POST /api/sessions/{id}/stop` covers explicit-stop semantics; GCONF-04's gate counts but never stops). CONTEXT leaves "stop surface shape" to planning discretion. Recommend: ship the engine method now (locked carry-forward), let the API surface follow Phase 16's UI need — do not add an untested endpoint speculatively.
   - **RESOLVED:** engine primitive only, no speculative endpoint — adopted in 15-01-PLAN.md Task 1.
3. **`scope` + `task_id` mutual exclusion spelling** — `scope:"global"` with a non-zero `task_id` in one body is contradictory; 400 (the "supply either" family, global.go:288) is the obvious posture but wasn't D-numbered. Planner picks the copy.
   - **RESOLVED:** 400 with copy "supply either scope or task_id, not both" — adopted in 15-01-PLAN.md Task 2.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | build/tests | ✓ | 1.26.0 | — |
| tmux | tmux-integration tests (mint/reattach/reconcile) | ✓ | 3.4 | Tests skip-guard (house convention); CI without tmux still covers non-tmux paths |
| git | test fixtures (temp repos for folder roots) | ✓ | 2.43.0 | Tests skip-guard (opencode_resume_test.go:161) |
| opencode | host-gated capture e2e (Spike 2) | ✓ | 1.18.22 | CI stub (`discoverOpenCodeSession` injection) — the e2e skips where absent |
| claude | not required (fake-claude stub covers agent tests) | ✓ | 2.1.246 | — |
| SQLite/goose | runtime storage | ✓ | modernc v1.52.0 / goose v3.27.1 (go.mod) | — |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** none — all present on this host; skip-guards exist for CI portability.

## Security Domain

Local-only single-user app (loopback-enforced, serve.go:425 `ensureLoopback`); this phase adds no new network surface, no new external input channels beyond one request-body field, and no secrets handling.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | Local-only; no auth surface changed |
| V3 Session Management | no | PTY sessions, not auth sessions; unchanged |
| V4 Access Control | yes (marginally) | Loopback bind + origin allowlist (existing, serve.go:241-245); the `scope` field gates on server-side config state (the singleton), never client-supplied paths |
| V5 Input Validation | yes | `scope` body field validated against a closed set (reject unknown values 400 — the `invalid kind` family, sessions.go:244); tmux names remain server-minted (never client input); reattach names looked up scoped (D-32 — a foreign name simply doesn't resolve, no injection surface) |
| V6 Cryptography | no | Nothing cryptographic in scope |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Path traversal / arbitrary cwd via spawn body | Tampering | cwd comes ONLY from the server-side singleton (validated at PUT time by Phase 14: git-repo check, footgun blocks D-07/D-27) — the spawn body carries no path |
| tmux target injection | Tampering | Names are server-minted `kamacu-global-<n>`; client-supplied reattach names go through parameterized scoped SQL + the tmux client's exact-match `=name` probe |
| Scope-confused reattach (cross-tenant style) | Elevation | D-32: scoped lookup — a task tab is invisible from the global scope (404), no information leak |
| Spawn-gate bypass via race | Tampering | Same single-threaded-per-request posture as the task path; mint race backstopped by `name UNIQUE` (Pitfall 6) |

## Sources

### Primary (HIGH confidence)
- Live codebase, this worktree (`add-global-session-301`), read at file:line level THIS session:
  - `internal/session/manager.go` (SpawnOpts :54, Spawn :114, label switch :335, listWhere :427, StopAllForTask :456), `internal/session/session.go` (Info :44)
  - `internal/api/sessions.go` (create :223, gates :262-272, resolution :282-304, one-agent :308, resume :323, mint :417-432, csid :461, capture launch :475, stop :502, defaultTmuxLabel :615, discovery :726, capture :793, joinSessionContext :917)
  - `internal/api/agents.go` (agentStatusEntry :26, status :44, manager-pass filter :50, JOIN :99, DB pass :164)
  - `internal/api/global.go` (deriveGlobalState :223, globalLiveBlockers :123, liveGlobalTmuxNames :88, put gates :310), `internal/api/global_backfill.go`
  - `internal/store/migrations/00017_global_task.sql`, `00018_tmux_scope.sql`
  - `cmd/kamacu/serve.go` (backfills :160-202, sweep :361-423, loopback :425)
  - `internal/tmux/tmux.go` (exact-match :78-105), `internal/api/resume.go` (transcriptExists :16), `internal/reaper/reaper.go` (:166, :230), `internal/api/activity.go` (:134-135)
  - `internal/mcp/sessions.go` (tools :93-180, orphan filter :479-554)
  - Frontend consumers: `web/src/api/agents.ts` (:4-16), `ActiveSessionsBar.tsx` (:79-135, :268), `TaskPage.tsx` (:87, :105, :278), `AgentTab.tsx` (:58), `ProjectSidebar.tsx` (:41-44), `StatusDot.tsx`, `TaskCard.tsx`, `PRCard.tsx`
  - Test harnesses: `agent_integration_test.go`, `opencode_resume_test.go`, `sessions_test.go` (2330 lines), `agents_test.go`, `testdata/fake-claude`, `testdata/fake-opencode`
- `.planning/phases/15-global-sessions-backend/15-CONTEXT.md` — all D-decisions (locked inputs)
- `.planning/research/{SUMMARY,ARCHITECTURE,PITFALLS}.md` — v1.13 milestone research (architecture-locked; P1/P4/P5/P9 + patterns 1-4)
- `.planning/phases/13-.../13-CONTEXT.md`, `.planning/phases/14-.../14-CONTEXT.md` — carry-forward decisions exercised here

### Secondary (MEDIUM confidence)
- `.planning/REQUIREMENTS.md` + `.planning/ROADMAP.md` §Phase 15 — requirement texts, success criteria, co-phasing mandate, research flags

### Tertiary (LOW confidence)
- None — no external web sources were needed or used (zero-new-deps composition phase; research-plan seam consulted, digest cached under key `9b0b8e6f…`)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — zero-new-deps verified against go.mod/web package.json; every seam read in source
- Architecture: HIGH — all five seams mapped at file:line; both spikes resolved from verified code
- Pitfalls: HIGH — 8 of 10 pitfalls verified at file:line this session (P3/P4 in this doc are new, both discovered by reading the actual map/loop code); 2 carried from milestone research whose mechanics were re-verified

**Research date:** 2026-08-26
**Valid until:** 2026-09-25 (stable — in-repo composition; only drift risk is concurrent edits to sessions.go/agents.go/manager.go)
