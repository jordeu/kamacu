# Project Research Summary

**Project:** Kamacu — v1.13 "Global Task"
**Domain:** Local-only agent-orchestration app (Go + React); adding a singleton global scratchpad surface to a task/worktree-centric data model
**Researched:** 2026-08-25
**Confidence:** HIGH

## Executive Summary

v1.13 is a **composition milestone, not a library milestone**. Every capability the Global Task needs — PTY sessions with replay/detach-reattach, engine-branched agent spawn (claude/opencode/custom), restart-resume, managed `gh repo clone`, folder validation, settings UI, terminal rendering, the Active Sessions bar — already ships in the pinned stack, verified file-by-file against `go.mod` and `web/package.json`. The real work lives in four composition seams: one (or two) SQLite migrations, an additive scope discriminator on the session engine, widened spawn/status API branches, and a slimmed frontend route. Zero new dependencies, zero version bumps.

The recommended architecture is a **task-free "global scope": a `global_task` singleton table + `SpawnOpts.Global`/`Info.Global` flags + synthesized status entries** — never a sentinel project/task row (which leaks into ~10 surfaces: sidebar, project listings, workspace delete guard, Activity scopes, MCP tools, board position math) and never `TaskID = 0` (already overloaded as "dev session" in three load-bearing checks). The v1.4 managed-clone sequence (ParseRepoRef → ValidateRepo → Clone → reattach-or-409) and the v1.10 agent-FK shape (`agent_id REFERENCES agents(id) ON DELETE RESTRICT`) are reusable nearly verbatim. The session engine itself is already cwd-driven, not worktree-driven — every task/worktree coupling lives in the API layer, exactly where v1.4/v1.10 widened it before.

The three risks that can sink the milestone: (1) **invisibility** — `/api/agents/status` triple-filters `TaskID <= 0` and INNER-JOINS tasks→projects→agents, so a global session runs but never appears in the bar and never offers post-restart Resume; both passes must be widened deliberately in the same phase as the spawn path. (2) **restart amnesia** — `tmux_sessions.task_id` is NOT NULL (blocking global bash-tab rows) and the startup orphan sweep's known-set is an INNER JOIN that would **kill every live global tmux tab at next startup**; the sweep fix must land in the same phase as the migration. (3) **no worktree safety net** — the default Claude agent runs `--dangerously-skip-permissions` and the global root can be the user's only checkout; mitigate with managed-clone-as-default, honest folder-mode warnings, and a 409 gate on root reconfiguration while sessions are live.

## Key Findings

### Recommended Stack

**Add nothing.** STACK.md verified against the actual manifests that the pinned stack (Go 1.26 stdlib, `creack/pty`, `coder/websocket`, `modernc.org/sqlite` + goose, React 19 + react-router 7 + TanStack Query 5, `@xterm/xterm` 6, shadcn/ui) covers 100% of the milestone. New dependencies would be pure liability. Do not couple routine version bumps (modernc v1.56.0, websocket v1.8.15 exist) to this milestone.

The stack work that the roadmap *must* schedule (composition seams, not libraries):

- **Migration — `global_task` singleton**: `CHECK (id = 1)` DB-enforced singleton holding `root_path`/`repo_path`, `managed`/`github_repo`, `agent_id` FK (RESTRICT), and the two restart-resume ids. Seeds from the default agent inside the migration; an idempotent boot backfill re-arms it (the v1.9/v1.10 pattern). Settings-KV storage was considered and rejected by two of three researchers: it loses agent-FK integrity (the delete guard can't see it) and mixes hot entity state with slowly-changing config.
- **Migration — `tmux_sessions` scope rebuild**: SQLite can't drop NOT NULL, so a CREATE-copy-drop-rename rebuild under `PRAGMA foreign_keys=OFF` / `NO TRANSACTION` (the proven 00012/00013 discipline, one notch heavier) makes `task_id` nullable and adds a `scope`/`global` discriminator with an XOR CHECK. Whether this ships as migration 00017+00018 or one combined migration is a plan-phase detail.
- **Session engine**: one additive `Global bool` on `SpawnOpts`/`Info` (zero value = current behavior) + a global bash-label counter + `ListGlobal()`/`StopAllForScope()`. No fork, no PTY machinery changes.
- **API surface**: `GET/PUT /api/global` (config + managed-clone lifecycle), `POST /api/sessions` widened with `scope:"global"` (no side-channel spawn endpoint — it would fork the engine-branched spawn logic), `/api/agents/status` third synthesized pass, `kamacu-global-<n>` tmux mint (stays inside the sweep's `kamacu-` prefix and `defaultTmuxLabel`'s last-numeric-segment parsing).
- **Frontend**: `/global` route (sibling of `/settings`, outside workspace sync), `GlobalTaskPage` (trimmed TaskPage: agent + bash tabs only), loosened `AgentTab` props, bar row with `source:"global"` branch, Settings Global section. Zero new packages.

### Expected Features

FEATURES.md's parity principle is the load-bearing conclusion: **scratch must be a first-class citizen, not a degraded task**. Every comparable (JetBrains scratches are "fully functional, runnable, debuggable"; VS Code terminals get full persistence; SlayZone worktree-less cards get identical terminals) treats ad-hoc work as feature-equal. Build the global view from the same TaskPage/TaskTabs/TerminalPane machinery minus tabs — not a bespoke lighter widget.

**Must have (table stakes, v1.13):**
- Global Settings section — root (folder or gh-cloned managed repo) + default-agent picker; nothing else is reachable without it
- Scratch view — full TaskPage-derived shell, agent + bash tabs only, root path legible in header ("runs directly in \<root\>"), ⋯ Stop parity
- Full session semantics — persistent PTY, reattach+replay, restart reconcile + Resume (engine-branched), live working/waiting/idle status
- Active Sessions bar integration — live global row with synthesized label + click-through to `/global`
- Singleton enforcement + explicit Start (one-agent-per-global gate, the D-38 analog)
- MCP no-regression — `list_sessions`/status surfaces handle task-less sessions with labels, never 404/garble

**Should have (differentiators):**
- Managed gh-cloned scratch root — authenticated clone as ad-hoc workspace; nearly free via v1.4 reuse
- Conversation-level restart-resume on a task-less surface — beyond all comparables (VS Code revives processes; Kamacu resumes conversations)
- Scratch sessions visible to the MCP delegate surface — "watch what I'm trying in the scratchpad"

**Defer (post-v1.13 / v2+):**
- Promotion scratch→task (JetBrains F6 analog) — HIGH complexity, needs its own design pass (park as GT-FUT)
- Scratch Reset with preserved transcript; per-workspace scratchpads; branch indicator polish
- Anti-features confirmed: no description/diff/board presence, no worktree isolation for scratch, no auto-start, no TTL/auto-cleanup (the reaper is task-keyed by design — make non-reaping an explicit documented decision), no notifications

### Architecture Approach

ARCHITECTURE.md evaluated three representations and decisively recommends **Option C: task-free sessions + `global_task` singleton**. Option A (sentinel project) leaks into ~10 enumeration surfaces including the workspace non-empty delete guard (would make "Personal" permanently undeletable); Option B (nullable `tasks.project_id`) is a full rebuild of the most-JOINed table in the app behind the hottest paths. Option C's honest costs: the `tmux_sessions` rebuild, scope-awareness at every tmux touchpoint, and one synthesized entry at each of the three label consumers (status passes, `joinSessionContext`, bar) — strictly less risk than either sentinel, zero phantom entities. Four patterns to follow: singleton row + idempotent backfill; explicit scope flag never a sentinel id; synthesized JOIN entries instead of LEFT-JOIN surgery on hot queries; degrade-don't-break on the unconfigured state (honest 409 "global root not configured", never a silent `$HOME` fallback).

**Major components:**
1. `internal/api/global.go` (NEW) — config GET/PUT, folder validation, managed-clone lifecycle (createByRepo's 8-step ordering adapted to UPDATE), gated root switch/clear
2. `internal/store/migrations` — `global_task` singleton + `tmux_sessions` scope rebuild, plus `BackfillGlobalTask` boot wiring
3. `internal/session` (additive only) — `Global` flag, label counter, `ListGlobal`/`StopAllForScope`
4. `internal/api/sessions.go` — `scope:"global"` spawn branch (root+agent resolution, one-agent gate, tmux mint, resume, csid persist, opencode capture re-target)
5. `internal/api/agents.go` — third synthesized status pass (manager-derived + DB-derived post-restart)
6. `cmd/kamacu/serve.go` — backfill wiring, route registration, **sweepOrphanTmux scope-aware fix**
7. Frontend: `GlobalTaskPage`, `web/src/api/global.ts`, loosened `AgentTab`, bar + Settings sections
8. Unchanged-by-design (verify, don't touch): `internal/ws`, `hooks.go`, `internal/reaper`, `internal/worktree`, `internal/diff`, `internal/mcp`

### Critical Pitfalls

1. **The invisible global session (P1)** — three independent filters (manager pass skips `TaskID <= 0`; INNER-JOIN metadata drops task-less rows silently; DB-derived restart pass requires task row + worktree) all hide the global session. Widen BOTH status passes in the same phase as the spawn path, with a fake-agent regression test asserting the entry appears live AND post-restart.
2. **The sentinel-row hack (P2)** — a fake project/task row leaks onto boards, listings, stats, MCP tools forever; each new milestone re-risks the filter. Dedicated singleton instead; add a sentinel-leak regression test enumerating every listing surface.
3. **No worktree safety net (P3)** — skip-permissions default agent in the user's real checkout with no diff and no gates. Managed clone as recommended default; git validation + persistent warning for folder mode; lightweight `git status --short` summary line in the view; explicit threat-model statement in the README/UAT.
4. **Restart amnesia (P5)** — the tmux FK blocks global rows, the startup sweep INNER JOIN would kill live global tabs, and resume ids have no home. Migration + sweep fix + persisted ids land together; regression-test tmux survival and both-engine resume across a simulated restart.
5. **`TaskID = 0` is already taken (P4) + the bar contract (P6)** — reusing the dev-session overload turns stop/list/label into landmines (`StopAllForTask(0)` nukes dev terminals); the non-nullable bar wire contract + hardcoded task route + `key={taskId}` breaks on a project-less entry. Explicit `Global`/`scope` discriminator; server-side synthesized labels (`projectName:"Global"`, `taskTitle:"Global Task"` — keeps TS types non-nullable); key bar rows by `sessionId`; add the global-route highlight case.

Also carried: **P7** (gate root reconfiguration on live sessions — 409 with reasons, clear resume ids on success; a mutable cwd is a new class in this app), **P8** (managed-clone path collision — see Gaps), **P9** (document non-reaping as an explicit out-of-scope REQUIREMENTS line; scope-targeted Stop).

## Implications for Roadmap

Both ARCHITECTURE.md (5-phase build order) and PITFALLS.md (pitfall-to-phase mapping over 4 phases) converge on the same dependency-driven structure. Suggested phases:

### Phase 1: Global data foundation & safety net
**Rationale:** Everything blocks on the data layer, and the highest blast-radius decisions (representation, clone namespace, reconfigure semantics) get exponentially more expensive once later phases build on a wrong base — the v1.9/v1.10 migration-first pattern.
**Delivers:** `global_task` singleton migration + backfill; `tmux_sessions` scope rebuild; **the `sweepOrphanTmux` scope-aware fix in the same phase** (shipping the migration without it kills every live global tmux tab at next startup); agents delete-guard extension; decisions locked: representation, clone namespace, reconfigure gate semantics, folder-root validation strictness, UI naming.
**Avoids:** P2 (sentinel), P5's schema half, P8's path format, P7's semantics.
**Tests:** goose upgrade path on a seeded install, backfill idempotence, sweep-no-kill regression, FK re-arm, copy fidelity.

### Phase 2: Global config API
**Rationale:** Independently testable with curl before any frontend; the managed-clone variant is the v1.4 sequence with INSERT swapped for UPDATE.
**Delivers:** `internal/api/global.go` — GET (config + derived state) + PUT folder variant (validated dir path) + repo variant (gh-validate → clone → reattach-or-409, atomic row update, project-overlap guard) + gated root switch/clear.
**Avoids:** P7's gate, P8's overlap direction (save-side), P3's validation/warning copy.
**Note:** Can be merged into Phase 1 (if small) or Phase 4's backend half (if the roadmap wants 4 phases) — but its managed-clone interlock decisions belong early.

### Phase 3: Session engine scope + spawn/status backend
**Rationale:** The milestone's risk center — the spawn gate, engine resolution, status feed, tmux mint, and resume persistence all widen here, and the task paths must diff clean.
**Delivers:** `SpawnOpts.Global`/`Info.Global` + counters + `ListGlobal`/`StopAllForScope`; `POST /api/sessions {scope:"global"}` (agent + bash + resume + tmux + reconcile + opencode capture re-target); `/api/agents/status` third synthesized pass (both live and DB-derived post-restart); scope-targeted Stop.
**Avoids:** P1 (invisibility), P4 (TaskID overload), P5's runtime half, P9's stop affordance.
**Note:** Consider two waves (engine+bash, then agent+status) if the phase feels heavy. Extend the fake-claude/fake-opencode suites with global cases; task-path regression must stay byte-for-byte green.

### Phase 4: Frontend view + reachability
**Rationale:** Depends on Phases 2–3; all standard in-repo patterns (v1.6 bar widening, TaskPage shell reuse, Settings sections).
**Delivers:** `/global` route + `GlobalTaskPage` (agent + bash tabs, no-isolation legibility); `AgentTab` props loosening; global session hooks; bar integration (types, row, navigation, highlight); Settings Global section + config client + "Open Global Task" idle affordance.
**Avoids:** P6 (bar contract), P3's UAT exposure (real folder checkout), the reachability asymmetry (define "live" as any live global session; bar shows agent-live only — documented).

### Phase 5: Hardening & E2E
**Rationale:** Closes the lifecycle/managed-clone gates where the gated-delete philosophy already provides templates; restart bugs are invisible until a restart, so E2E is the only catcher.
**Delivers:** Managed-root/project-delete interlock (both directions); reconfigure gate E2E (409 while live, resume-id clearing on success); restart-resume E2E (claude + opencode + tmux survivor, the v1.8 GAP-01 regression replayed for the global scope); MCP parity spot-check (`list_sessions` labels, read-only contract, `start_task_agent` correctly errors for global); sentinel-leak sweep; REQUIREMENTS line documenting non-reaping; UAT script exercising a folder-mode root against a real repo.
**Avoids:** P3/P7/P8/P9 verification halves.

### Phase Ordering Rationale
- Data layer before spawn path before frontend — dictated by three hard dependencies (schema must exist before spawns; spawn/status backend before the view; config API before Settings UI is useful).
- The sweep fix MUST be co-phased with the tmux migration — the one pre-existing killer bug the research found.
- Status widening MUST be co-phased with the spawn path — otherwise the phase can't be human-verified (invisible session).
- Pitfalls 2/7/8 decisions are Phase 1 because they bake into schema/path formats; their gates and verification land in later phases.

### Research Flags
Phases likely needing deeper research during planning (`/gsd-plan-phase --research-phase`):
- **Phase 1:** the `tmux_sessions` full-table rebuild is the one piece without a direct in-repo precedent at this exact shape (00012/00013 add columns; this rebuilds) — plan a migration rehearsal on a copy of a real install.
- **Phase 3:** short spike on the `agentStatusEntry` wire widening (nullable-vs-synthesized choice ripples into 4+ frontend consumers; research recommends synthesized server-side) and the opencode capture re-targeting (the `PWD` pin + directory filter now matching the global root — worth one host-gated e2e).

Phases with standard patterns (skip research-phase):
- **Phase 2:** v1.4's createByRepo ordering, reattach, and atomicity are documented in-repo at file:line.
- **Phase 4:** TaskPage/TaskTabs/TerminalPane shell reuse, v1.6 bar widening, Settings section patterns — all established.
- **Phase 5:** gated-delete templates exist (`CleanupWorktreeGated` — adapt, don't call with a fake taskID); fake-claude/fake-opencode harnesses already exist.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | "Zero new dependencies" verified against actual `go.mod`/`web/package.json` and every integration point read at file:line level, not recalled. |
| Features | MEDIUM | Comparable-tool behavior from official docs/sites (some negative claims, e.g. SlayZone's lack of a global surface, are marketing-page-only); every claim about existing Kamacu behavior is HIGH, verified in source. |
| Architecture | HIGH | All claims grounded in the live v1.12 codebase; schema claims read from migrations 00001–00016; no external sources needed. |
| Pitfalls | HIGH | Every pitfall mechanic verified at file:line in this worktree; two findings additionally cross-validated against community sources (MEDIUM). |

**Overall confidence:** HIGH — this is integration research about our own codebase, and the central recommendations (singleton table, scope flag, synthesized entries, co-phased sweep fix) are convergent across all four documents.

### Gaps to Address

- **Managed global clone namespace (genuine researcher disagreement):** STACK recommends reusing `~/.kamacu/repos/<owner>/<name>` + a cross-entity dedup 409; PITFALLS P8 recommends a separate namespace (`~/.kamacu/repos/global/<owner>/<name>` or `~/.kamacu/global/repo`) to make the collision class *unrepresentable* rather than tested-for (a project's gated delete does `os.RemoveAll` on its clone). **Lean namespaced** — structurally safe beats guarded — but decide explicitly in Phase 1 planning; the stored path format bakes it in.
- **Folder-root validation strictness:** ARCHITECTURE recommends plain directory (nothing in the global path runs git); PITFALLS recommends requiring a git repo + persistent warning (safety posture for skip-permissions agents); STACK says reuse `validateRepoPath` (git required). Decide in Phase 1/2 — one validator's worth of difference either way.
- **Resume/config storage:** FEATURES floated a settings-KV option; STACK and ARCHITECTURE both reject it (FK integrity, config/state separation). **Resolved: singleton table** — recorded here so planning doesn't reopen it without new evidence.
- **Activity attribution for global sessions** (v1.12 JOIN keys on tasks): exclude vs "Global" attribution — plan-phase decision, not a blocker.
- **Naming collision:** the UI label "Global" collides with v1.12 Activity's "Global" scope; consider "Scratchpad" as the user-facing label. Decide once, early.
- **Bar row shape** (`Global · Global Task` vs Globe badge) and idle affordance prominence (Settings-only vs sidebar entry): cosmetic, safe to settle at UAT; sidebar entry is parked scope creep.
- **MCP `scope` filter on `list_sessions`:** defer to the banked v1.11 follow-up list unless trivially cheap during Phase 3.

## Sources

### Primary (HIGH confidence)
- Live codebase at v1.12 (this worktree, 87e6a77) — `go.mod`, `web/package.json`, `internal/{session,api,github,settings,store,reaper,ws,mcp,tmux}`, `cmd/kamacu/serve.go`, migrations 00001–00016, and the cited frontend files — read directly at file:line level for all four research docs.
- `.planning/PROJECT.md` — v1.13 milestone scope + v1.4/v1.8/v1.9/v1.10/v1.11/v1.12 milestone histories (settled-decision precedents).

### Secondary (MEDIUM confidence)
- Comparable surfaces: JetBrains IntelliJ 2026.2 scratches docs (fully-functional scratches, 5-buffer rotation, F6 promotion, accumulation complaints); VS Code terminal persistence docs; vibe-kanban standalone-workspace docs (sunsetting — pattern reference only); ttyd/gotty READMEs (configured-cwd model); Copilot Spaces docs; SlayZone features page (marketing-page depth).
- Community cross-validation: INNER JOIN NULL-row drops (sqlitetutorial/sqlteam/r-SQL); agents-mutating-shared-checkouts incidents with worktrees as the standard boundary (nx.dev, augmentcode.com, 2026).
- Release drift checks (pkg.go.dev, 2026-08-25): modernc v1.56.0 and coder/websocket v1.8.15 exist — routine, nothing this milestone needs.

### Tertiary (LOW confidence)
- SlayZone internals beyond the marketing page; Cursor/Warp scratch-session specifics (no primary sources found — excluded rather than guessed).

---
*Research completed: 2026-08-25*
*Ready for roadmap: yes*
