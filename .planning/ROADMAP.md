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
- 🚧 **v1.12 Activity & Statistics** — Phases 10–11 (planning 2026-07-29)

## Phases

<details open>
<summary>🚧 v1.12 Activity & Statistics (Phases 10–11) — PLANNING 2026-07-29</summary>

**Milestone goal:** A new top-level Activity page lists tasks done and PR reviews completed (merged/closed) over a rolling Week (7d) / Month (30d) window — scoped globally, by workspace, or by project — alongside a statistics section with counts and min/max/median cycle (In-Progress→Done) + per-column dwell times. Tasks-only for time stats (GitHub's merge signal has no Kamacu in-progress/in-review timestamps).

**Phase Numbering:** continues from v1.11's last phase (09). v1.12 = Phases 10–11.

- [x] **Phase 10: Activity Data & API** - Scoped/windowed tasks-done, reviews-done (merged/closed reviewed-by:@me), and task cycle/dwell statistics behind a new activity endpoint set (completed 2026-07-30)
- [ ] **Phase 11: Activity Page & Controls** - Top-level Activity page (sidebar entry, scope selector, Week/Month toggle) rendering tasks-done + reviews-done lists and statistics

## Phase Details

### Phase 10: Activity Data & API

**Goal**: The backend answers "which tasks were done, which reviews were completed (merged/closed), and what are the cycle/dwell stats" for any Global / Workspace / Project scope and a Week (7d) / Month (30d) window — including the new merged/closed `reviewed-by:@me` `gh` search dimension and graceful degradation when `gh` is absent.
**Depends on**: Nothing (first phase of v1.12; the per-status timestamps powering the stats already exist from migration 00006 — `todo_at`/`in_progress_at`/`in_review_at`/`done_at` — and the `internal/github` package + per-repo cache from v1.3/v1.5 are reused).
**Requirements**: TASKS-01, REVIEWS-01, REVIEWS-04, STATS-01, STATS-02, STATS-03, STATS-04
**Success Criteria** (what must be TRUE):

  1. An activity endpoint returns every task completed (moved to Done) within the selected scope and window, each carrying its project name, title, and `done_at` timestamp.
  2. The activity endpoint returns PRs reviewed-by:@me that MERGED or CLOSED within the window, each with PR number, title, and merge/close date — backed by a NEW `is:merged`/`is:closed` + `reviewed-by:@me` `gh` search aggregated across the projects in scope (reusing the v1.3/v1.5 `internal/github` package + per-repo cache).
  3. When `gh` is unavailable, GitHub integration is off, or an individual repo's fetch fails, the reviews-done portion returns empty without breaking the tasks-done list or the task statistics (degrade-don't-break).
  4. The activity endpoint returns counts of tasks done and reviews done within the selected scope and window.
  5. The activity endpoint returns min / max / median cycle time (In-Progress → Done) and per-column dwell (time in In-Progress and time in In-Review) for tasks done in the window; reviews contribute a count, never time stats.

**Plans**:
**Wave 1**

- [x] 10-01-PLAN.md — GitHub `GetMergedClosed` extension (merged/closed reviewed-by:@me data source with its own 5min-TTL cache, decoupled from the review-column hot path)
- [x] 10-03-PLAN.md — Pure activity helpers & wire types (scope/window parsing, in-Go cycle/dwell/median stats, review-state rollup, mismatched-precision ISO-time parser) — wave-1 sibling parallel to 10-01

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 10-02-PLAN.md — `GET /api/activity` handler (wire types, cross-repo aggregation with reviews window-filter cutoff, GATE 1, always-200) consuming 10-01 + 10-03

### Phase 11: Activity Page & Controls

**Goal**: Users open a dedicated top-level Activity page, scope it (Global / a Workspace / a Project) and toggle Week / Month, and see the tasks-done + reviews-done lists grouped by project plus the task statistics — with the reviews section quietly emptying itself when GitHub integration is off.
**Depends on**: Phase 10 (the activity data + API).
**Requirements**: ACT-01, ACT-02, ACT-03, ACT-04, TASKS-02, TASKS-03, REVIEWS-02, REVIEWS-03
**Success Criteria** (what must be TRUE):

  1. User can open a dedicated top-level Activity page from a persistent sidebar entry (like Settings), accessible app-wide.
  2. User can change scope (Global / a specific workspace / a specific project) via a selector, and both the lists and the statistics update to that scope.
  3. User can toggle the time window between Week (last 7 days) and Month (last 30 days), rolling from now, and both the lists and the statistics update to the chosen window.
  4. Tasks-done and reviews-done entries are each grouped by project with their fields shown (task title + completion time for tasks; PR number + title + merge/close date for reviews); clicking a tasks-done entry navigates to that task's view, and clicking a reviews-done entry opens the PR (its review workspace if it exists, else the PR).
  5. When GitHub integration is off (or `gh` absent), the reviews section renders empty while tasks-done and task statistics still display normally.

**Plans**: TBD
**UI hint**: yes

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

**Execution Order:** Phases execute in numeric order: 06 → 07 → 08 → 09 (v1.11, shipped) → 10 → 11 (v1.12, active).

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 01. Agent data foundation and spawn engine | v1.10 | 6/6 | Complete | 2026-07-06 |
| 02. Agent management UI and per-project selection | v1.10 | 5/5 | Complete | 2026-07-06 |
| 03. opencode engine spawn and activity-based status | v1.10 | 4/4 | Complete | 2026-07-07 |
| 04. Gated status plugin unlocks waiting and idle | v1.10 | 2/2 | Complete | 2026-07-07 |
| 05. Session resume and argv regression hardening | v1.10 | 3/3 | Complete | 2026-07-08 |
| 06. MCP Subcommand Foundation | v1.11 | 3/3 | Complete    | 2026-07-21 |
| 07. Tasks, Projects & Workspaces Tools | v1.11 | 4/4 | Complete    | 2026-07-22 |
| 08. Sessions & Terminal Read Access | v1.11 | 3/3 | Complete    | 2026-07-23 |
| 09. PR Review Tools | v1.11 | 1/1 | Complete    | 2026-07-28 |
| 10. Activity Data & API | v1.12 | 3/3 | Complete    | 2026-07-30 |
| 11. Activity Page & Controls | v1.12 | 0/? | Not started | - |
