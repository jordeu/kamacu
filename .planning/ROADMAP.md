# Roadmap: Kamacu

## Milestones

- ✅ **v1.0 MVP** — Phases 1–5 (shipped 2026-06-11) — see [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md)
- ✅ **v1.1 Settings & Polish** — Phase 6 (shipped 2026-06-11) — see [milestones/v1.1-ROADMAP.md](milestones/v1.1-ROADMAP.md)
- ✅ **v1.2 Quota & Resumable Shells** — Phases 7–9 (shipped 2026-06-13) — see [milestones/v1.2-ROADMAP.md](milestones/v1.2-ROADMAP.md)
- ✅ **v1.3 GitHub PR Review** — Phases 10–13 (shipped 2026-06-14) — see [milestones/v1.3-ROADMAP.md](milestones/v1.3-ROADMAP.md)
- ✅ **v1.4 Repo-First Projects** — Phases 14–15 (shipped 2026-06-15) — see [milestones/v1.4-ROADMAP.md](milestones/v1.4-ROADMAP.md)
- ✅ **v1.5 Sharper Review Column** — Phase 16 (shipped 2026-06-17) — see [milestones/v1.5-ROADMAP.md](milestones/v1.5-ROADMAP.md)
- ✅ **v1.6 Global Active Sessions Bar** — Phase 17 (shipped 2026-06-18) — see [milestones/v1.6-ROADMAP.md](milestones/v1.6-ROADMAP.md)
- ✅ **v1.7 Project Icons in Collapsed Sidebar** — Phases 18–19 (shipped 2026-06-19) — see [milestones/v1.7-ROADMAP.md](milestones/v1.7-ROADMAP.md)
- ✅ **v1.8 Kamacu Rebrand & UX Polish** — Phases 20–24 (shipped 2026-07-04) — see [milestones/v1.8-ROADMAP.md](milestones/v1.8-ROADMAP.md)
- ✅ **v1.9 Workspaces** — Phases 25–26 (shipped 2026-07-06) — see [milestones/v1.9-ROADMAP.md](milestones/v1.9-ROADMAP.md)
- ✅ **v1.10 Configurable Agents** — Phases 01–05 (shipped 2026-07-11) — see [milestones/v1.10-ROADMAP.md](milestones/v1.10-ROADMAP.md)
- ✅ **v1.11 Kamacu MCP Server** — Phases 06–09 (shipped 2026-07-29) — see [milestones/v1.11-ROADMAP.md](milestones/v1.11-ROADMAP.md)
- ✅ **v1.12 Activity & Statistics** — Phases 10–12.1 (shipped 2026-08-25) — see [milestones/v1.12-ROADMAP.md](milestones/v1.12-ROADMAP.md)
- 🚧 **v1.13 Global Task** — Phases 13–17 (in planning)

## Phases

<details open>
<summary>🚧 v1.13 Global Task (Phases 13–17) — IN PLANNING</summary>

**Milestone goal:** A single global scratchpad — a task-like view with agent + bash tabs running directly in a configured repo/folder — for small checks unrelated to any project. Not linked to any workspace or project; no worktree, no branch, no board. Architecture (research-locked): a `global_task` singleton table + task-free "global scope" sessions — never a sentinel project/task row, never `TaskID = 0`.

**Phase Numbering:** continues from v1.12's last phase (12.1). v1.13 = Phases 13–17.

**Co-phasing mandates (from research — non-negotiable):**

- The `sweepOrphanTmux` scope-aware fix MUST ship in the same phase as the `tmux_sessions` migration (Phase 13) — shipping the migration without the sweep fix kills every live global tmux tab at the next startup.
- The `/api/agents/status` feed widening MUST ship in the same phase as the global spawn path (Phase 15) — otherwise the global session is invisible and the phase cannot be human-verified.

- [x] **Phase 13: Global data foundation & safety net** - The `global_task` singleton + task-less `tmux_sessions` schema + the scope-aware startup sweep — the base every later phase builds on, landed together with the one pre-existing killer bug it introduces (completed 2026-08-25)
- [ ] **Phase 14: Global config API** - `GET/PUT /api/global` — root as validated folder or gh-cloned managed repo (v1.4 pattern), default-agent picker, and the live-session 409 gate on root reconfiguration
- [ ] **Phase 15: Global sessions backend** - Session-engine scope + `scope:"global"` spawn (agent + bash + resume + tmux) + the widened status feed — the milestone's risk center
- [ ] **Phase 16: Global view, Settings & bar integration** - The `/global` route (TaskPage shell, agent + bash tabs only), the Settings Global section, and the Active Sessions bar row
- [ ] **Phase 17: Hardening & E2E** - Reconfigure-gate + restart-resume E2E, managed-root ↔ project-delete interlock, sentinel-leak sweep, folder-mode UAT — closing the gates invisible until a restart or a delete happens

## Phase Details

### Phase 13: Global data foundation & safety net

**Goal**: The database and startup invariants that let task-free global sessions exist — a DB-enforced `global_task` singleton holding config + resume ids, a `tmux_sessions` shape that accepts task-less rows, and an orphan sweep that can never kill them — landed so that upgrading an existing install preserves everything byte-for-byte.
**Depends on**: Nothing (first phase of v1.13; follows the v1.9/v1.10 migration-first pattern — migrations 00012/00013 are the column-add precedents, this phase adds the one heavier table rebuild)
**Requirements**: GDATA-01, GDATA-02, GDATA-03
**Success Criteria** (what must be TRUE):

  1. A seeded v1.12 install upgrades cleanly through the new migrations — every existing task row, tmux_sessions row, and custom tab label survives byte-for-byte (goose upgrade path rehearsed on a copy of a real install).
  2. On every boot the `global_task` singleton row (DB-enforced id=1) exists, seeded with the default agent; deleting the row is re-armed by the idempotent backfill; deleting an agent assigned to it is refused by the FK (ON DELETE RESTRICT delete-guard extension).
  3. `tmux_sessions` accepts a task-less global row (explicit scope discriminator + XOR CHECK) and rejects an ambiguous row (both task and global set); task-scoped FK discipline is unchanged for existing rows.
  4. A live `kamacu-global-*` tmux session survives the startup orphan sweep (regression test proves sweep-no-kill on a live global tab) while the pre-existing orphan-killing behavior for task sessions is unchanged.

**Plans**: 2 plans

Plans:
**Wave 1**

- [x] 13-01-PLAN.md — global_task singleton (00017) + tmux_sessions scope rebuild (00018) with the staged-upgrade byte-for-byte migration proof

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 13-02-PLAN.md — boot safety net: BackfillGlobalTask wiring, scope-aware orphan sweep (failing-regression-first), Scratchpad agent-delete 409 guard, real-install rehearsal

> **Research flag**: run `/gsd-plan-phase 13 --research-phase` — the `tmux_sessions` full-table rebuild (SQLite can't drop NOT NULL → CREATE-copy-drop-rename under `PRAGMA foreign_keys=OFF`) is the one piece without a direct in-repo precedent at this exact shape; plan a migration rehearsal on a copy of a real install. Phase-1 decisions that bake in here: managed-clone namespace (lean namespaced — structurally safe), folder-root validation strictness, reconfigure gate semantics, UI naming ("Global" vs "Scratchpad").

### Phase 14: Global config API

**Goal**: The global root and default agent are configurable over the API — a validated local folder or a gh-validated managed clone in a namespace no project can touch — with reconfiguration gated on live sessions and resume ids cleared on success. Independently testable with curl before any frontend exists.
**Depends on**: Phase 13 (the `global_task` singleton row the API reads/writes)
**Requirements**: GCONF-01, GCONF-02, GCONF-03, GCONF-04
**Success Criteria** (what must be TRUE):

  1. `GET /api/global` returns the current config plus derived state; `PUT` with a local folder path validates the directory and persists it as the global root.
  2. `PUT` with `owner/name` gh-validates and clones the repo into a separate global-managed namespace that no project's gated delete can ever touch; re-configuring the same repo reattaches instead of re-cloning; a failed clone surfaces its error leaving no half-configured row or directory (atomic, v1.4 createByRepo ordering with UPDATE swapped for INSERT).
  3. The global task's default agent is settable to any configured agent (v1.10 agents CRUD surface); an agent in use by the global task cannot be deleted.
  4. Changing or clearing the root while global sessions are live is refused with a 409 + reasons list; on success the persisted resume ids are cleared.

**Plans**: TBD

### Phase 15: Global sessions backend

**Goal**: The API-driven global scratchpad runs with full task parity — the session engine gains an additive global scope, `POST /api/sessions {scope:"global"}` spawns the agent and bash tabs in the global root, the status feed widens so the session is visible live and post-restart, and restart-resume works for both engines. The milestone's risk center: every task path must diff clean.
**Depends on**: Phase 14 (root + agent resolution reads the configured singleton)
**Requirements**: GSESS-01, GSESS-02, GSESS-03, GSESS-04, GVIEW-02, GVIEW-03, GINT-02, GINT-03
**Success Criteria** (what must be TRUE):

  1. `POST /api/sessions` with `scope:"global"` spawns the configured default agent with cwd = the global root (no worktree, no branch) and bash tabs with the same shell options as tasks (plain bash + invisible tmux, `kamacu-global-<n>` mint); a second concurrent global agent spawn is refused with 409.
  2. Global sessions carry full task-parity semantics — persistent server-side PTYs, detach/reattach with ring-buffer replay, live working/waiting/idle status — and Stop kills exactly the global scope (never `StopAllForTask(0)` / dev terminals).
  3. A live global agent session appears in `/api/agents/status` with a server-synthesized non-nullable label (projectName/taskTitle) in BOTH passes — the manager-derived live pass and the DB-derived post-restart pass.
  4. After a server restart, global agent sessions reconcile and offer engine-branched Resume (claude `--resume` / opencode `--resume`, keyed on the persisted singleton ids), and global tmux tabs reattach invisibly with no Resume button.
  5. Global sessions never auto-reap (no Done-TTL analog, no PR reconcile pass — immortal until explicitly stopped); MCP session tools list/describe them with honest labels (never 404 or garbled, read-only contract unchanged); Activity stats and lists exclude them.

**Plans**: TBD

> **Research flag**: run `/gsd-plan-phase 15 --research-phase` — two spikes: the `agentStatusEntry` wire widening (nullable-vs-synthesized choice ripples into 4+ frontend consumers; research recommends synthesized server-side) and the opencode session-capture re-targeting (the `PWD` pin + directory filter now matching the global root — worth one host-gated e2e). If the phase feels heavy at planning time, split into waves (engine+bash, then agent+status) — not separate phases.

### Phase 16: Global view, Settings & bar integration

**Goal**: The user-facing global scratchpad — a `/global` route rendering the TaskPage-derived shell with agent + bash tabs only, a Settings Global section to configure root and default agent, and a live row in the Active Sessions bar with click-through. Scratch is a first-class citizen built from the same machinery, not a degraded task.
**Depends on**: Phases 14–15 (the config API and the spawn/status backend the view drives)
**Requirements**: GCONF-05, GVIEW-01, GVIEW-04, GINT-01
**Success Criteria** (what must be TRUE):

  1. The `/global` route renders the full TaskPage-derived shell with ONLY the Agent tab + bash tabs (no description tab, no diff tab, never on a kanban board) with explicit Start and ⋯ Stop parity.
  2. The Agent tab starts the configured default agent in the global root and bash tabs spawn with the same shell options as tasks — all through the Phase-15 session surface (GVIEW-02/GVIEW-03 behaviors, now user-triggered).
  3. With no root configured, the global view shows an honest unconfigured state pointing to Settings — never a silent `$HOME` or cwd fallback.
  4. The Settings Global section configures the root (folder path or GitHub repo) and the default agent with server errors surfaced inline (clone failures leave the dialog open, no half-configured state), and provides an "Open Global Task" affordance so the view is reachable when idle.
  5. A live global agent session appears as a row in the global Active Sessions bar (synthesized label, non-nullable wire contract) with click-through to `/global` and current-page highlight.

**Plans**: TBD
**UI hint**: yes

### Phase 17: Hardening & E2E

**Goal**: The lifecycle gates proven end-to-end — the flows that are invisible until a restart or a delete happens: reconfigure-while-live, restart-resume for both engines, the managed-root ↔ project-delete interlock, and the sentinel-leak guarantee across every enumeration surface. Closes the verification halves of the earlier phases' requirements.
**Depends on**: Phase 16 (the full stack — config, sessions, view — exercised through real flows)
**Requirements**: none owned — closes the end-to-end verification gates for GCONF-04, GSESS-02, GSESS-03, GINT-02, GINT-03 and the Phase-13 architecture invariants (sentinel-leak)
**Success Criteria** (what must be TRUE):

  1. Reconfigure gate E2E: with a live global agent + tmux tab, changing or clearing the root from Settings is refused with 409 + reasons; after stopping them, the change succeeds and the persisted resume ids are cleared.
  2. Restart-resume E2E: after a server restart, a global claude session offers Resume and resumes the same conversation, a global opencode session likewise, and a global tmux tab reattaches invisibly (the v1.8 GAP-01 regression replayed for the global scope).
  3. Managed-root ↔ project-delete interlock proven in both directions: deleting a project (incl. its gated clone removal) never touches the global root, and clearing/reconfiguring the global root never touches any project clone.
  4. Sentinel-leak sweep: no global entity appears on any enumeration surface (kanban boards, project/task listings, workspace non-empty delete guard, Activity stats/lists, MCP task tools) — each surface enumerated and asserted.
  5. A UAT script exercises a folder-mode root against a real repo checkout end-to-end (configure → agent → bash → reattach → stop), documenting the no-worktree safety posture (skip-permissions agent in a real checkout).

**Plans**: TBD

</details>

<details>
<summary>✅ v1.12 Activity & Statistics (Phases 10–12.1) — SHIPPED 2026-08-25</summary>

- [x] Phase 10: Activity Data & API (3/3 plans) — completed 2026-07-30
- [x] Phase 11: Activity Page & Controls (3/3 plans) — completed 2026-07-31
- [x] Phase 12: Activity Chart & Stat Tooltips (2/2 plans) — completed 2026-08-01
- [x] Phase 12.1: Address Activity tech debt WR-01..03 (1/1 plan, INSERTED) — completed 2026-08-25

Full details: [milestones/v1.12-ROADMAP.md](milestones/v1.12-ROADMAP.md)

</details>

<details>
<summary>✅ v1.11 Kamacu MCP Server (Phases 06–09) — SHIPPED 2026-07-29</summary>

- [x] Phase 06: MCP Subcommand Foundation (3/3 plans) — completed 2026-07-21
- [x] Phase 07: Tasks, Projects & Workspaces Tools (4/4 plans) — completed 2026-07-22
- [x] Phase 08: Sessions & Terminal Read Access (3/3 plans) — completed 2026-07-23
- [x] Phase 09: PR Review Tools (1/1 plan) — completed 2026-07-28

Full details: [milestones/v1.11-ROADMAP.md](milestones/v1.11-ROADMAP.md)

</details>

<details>
<summary>✅ v1.10 Configurable Agents (Phases 01–05) — SHIPPED 2026-07-11</summary>

- [x] Phase 01: Agent data foundation and spawn engine (6/6 plans) — completed 2026-07-06
- [x] Phase 02: Agent management UI and per-project selection (5/5 plans) — completed 2026-07-06
- [x] Phase 03: opencode engine spawn and activity-based status (4/4 plans) — completed 2026-07-07
- [x] Phase 04: Gated status plugin unlocks waiting and idle (2/2 plans) — completed 2026-07-07
- [x] Phase 05: Session resume and argv regression hardening (3/3 plans) — completed 2026-07-08

Full details: [milestones/v1.10-ROADMAP.md](milestones/v1.10-ROADMAP.md)

</details>

<details>
<summary>✅ v1.9 Workspaces (Phases 25–26) — SHIPPED 2026-07-06</summary>

- [x] Phase 25: Workspace data foundation (3/3 plans) — completed 2026-07-05
- [x] Phase 26: Workspace switcher, management & assignment (6/6 plans) — completed 2026-07-06

Full details: [milestones/v1.9-ROADMAP.md](milestones/v1.9-ROADMAP.md)

</details>

<details>
<summary>✅ v1.8 Kamacu Rebrand & UX Polish (Phases 20–24) — SHIPPED 2026-07-04</summary>

- [x] Phase 20: Kamacu rebrand & brand (4/4 plans) — completed 2026-07-01
- [x] Phase 21: Data directory migration (8/8 plans) — completed 2026-07-01
- [x] Phase 22: GitHub-style diff review (5/5 plans) — completed 2026-07-02
- [x] Phase 23: Worktree cleanup panel (5/5 plans) — completed 2026-07-02
- [x] Phase 24: Session-bar, board & tab polish (4/4 plans) — completed 2026-07-04

Full details: [milestones/v1.8-ROADMAP.md](milestones/v1.8-ROADMAP.md)

</details>

<details>
<summary>✅ v1.0–v1.7 (Phases 1–19) — SHIPPED 2026-06-11 through 2026-06-19</summary>

- [x] v1.0 MVP — Phases 1–5 (28 plans) — [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md)
- [x] v1.1 Settings & Polish — Phase 6 (4 plans) — [milestones/v1.1-ROADMAP.md](milestones/v1.1-ROADMAP.md)
- [x] v1.2 Quota & Resumable Shells — Phases 7–9 (12 plans) — [milestones/v1.2-ROADMAP.md](milestones/v1.2-ROADMAP.md)
- [x] v1.3 GitHub PR Review — Phases 10–13 (19 plans) — [milestones/v1.3-ROADMAP.md](milestones/v1.3-ROADMAP.md)
- [x] v1.4 Repo-First Projects — Phases 14–15 (7 plans) — [milestones/v1.4-ROADMAP.md](milestones/v1.4-ROADMAP.md)
- [x] v1.5 Sharper Review Column — Phase 16 (2 plans) — [milestones/v1.5-ROADMAP.md](milestones/v1.5-ROADMAP.md)
- [x] v1.6 Global Active Sessions Bar — Phase 17 (2 plans) — [milestones/v1.6-ROADMAP.md](milestones/v1.6-ROADMAP.md)
- [x] v1.7 Project Icons in Collapsed Sidebar — Phases 18–19 (6 plans) — [milestones/v1.7-ROADMAP.md](milestones/v1.7-ROADMAP.md)

</details>

## Progress

**Execution Order:** Phases execute in numeric order: 06 → 07 → 08 → 09 (v1.11, shipped) → 10 → 11 → 12 → 12.1 (v1.12, shipped) → 13 → 14 → 15 → 16 → 17 (v1.13, current).

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 01. Agent data foundation and spawn engine | v1.10 | 6/6 | Complete | 2026-07-06 |
| 02. Agent management UI and per-project selection | v1.10 | 5/5 | Complete | 2026-07-06 |
| 03. opencode engine spawn and activity-based status | v1.10 | 4/4 | Complete | 2026-07-07 |
| 04. Gated status plugin unlocks waiting and idle | v1.10 | 2/2 | Complete | 2026-07-07 |
| 05. Session resume and argv regression hardening | v1.10 | 3/3 | Complete | 2026-07-08 |
| 06. MCP Subcommand Foundation | v1.11 | 3/3 | Complete | 2026-07-21 |
| 07. Tasks, Projects & Workspaces Tools | v1.11 | 4/4 | Complete | 2026-07-22 |
| 08. Sessions & Terminal Read Access | v1.11 | 3/3 | Complete | 2026-07-23 |
| 09. PR Review Tools | v1.11 | 1/1 | Complete | 2026-07-28 |
| 10. Activity Data & API | v1.12 | 3/3 | Complete | 2026-07-30 |
| 11. Activity Page & Controls | v1.12 | 3/3 | Complete | 2026-07-31 |
| 12. Activity Chart & Stat Tooltips | v1.12 | 2/2 | Complete | 2026-08-01 |
| 12.1. Address Activity tech debt (WR-01..03) | v1.12 | 1/1 | Complete | 2026-08-25 |
| 13. Global data foundation & safety net | v1.13 | 2/2 | Complete    | 2026-08-25 |
| 14. Global config API | v1.13 | 0/? | Not started | - |
| 15. Global sessions backend | v1.13 | 0/? | Not started | - |
| 16. Global view, Settings & bar integration | v1.13 | 0/? | Not started | - |
| 17. Hardening & E2E | v1.13 | 0/? | Not started | - |
