# Roadmap: Kangent

## Milestones

- ✅ **v1.0 MVP** — Phases 1–5 (shipped 2026-06-11) — see [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md)
- ✅ **v1.1 Settings & Polish** — Phase 6 (shipped 2026-06-11) — see [milestones/v1.1-ROADMAP.md](milestones/v1.1-ROADMAP.md)
- ✅ **v1.2 Quota & Resumable Shells** — Phases 7–9 (shipped 2026-06-13) — see [milestones/v1.2-ROADMAP.md](milestones/v1.2-ROADMAP.md)
- ✅ **v1.3 GitHub PR Review** — Phases 10–13 (shipped 2026-06-14) — see [milestones/v1.3-ROADMAP.md](milestones/v1.3-ROADMAP.md)
- ✅ **v1.4 Repo-First Projects** — Phases 14–15 (shipped 2026-06-15) — see [milestones/v1.4-ROADMAP.md](milestones/v1.4-ROADMAP.md)
- ✅ **v1.5 Sharper Review Column** — Phase 16 (shipped 2026-06-17) — see [milestones/v1.5-ROADMAP.md](milestones/v1.5-ROADMAP.md)
- 🚧 **v1.6 Global Active Sessions Bar** — Phase 17 (in progress)

## Phases

<details>
<summary>✅ v1.0 MVP (Phases 1–5) — SHIPPED 2026-06-11</summary>

- [x] Phase 1: Foundation — Projects & Board (7/7 plans) — completed 2026-06-10
- [x] Phase 2: Terminal Engine (5/5 plans) — completed 2026-06-10
- [x] Phase 3: Worktree Isolation & Bash Tabs (6/6 plans) — completed 2026-06-10
- [x] Phase 4: Claude Code Agent Sessions (5/5 plans) — completed 2026-06-11
- [x] Phase 5: Recovery & Review (5/5 plans) — completed 2026-06-11

Full details: [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md)

</details>

<details>
<summary>✅ v1.1 Settings & Polish (Phase 6) — SHIPPED 2026-06-11</summary>

- [x] Phase 6: Settings & Polish (4/4 plans) — completed 2026-06-11

Full details: [milestones/v1.1-ROADMAP.md](milestones/v1.1-ROADMAP.md)

</details>

<details>
<summary>✅ v1.2 Quota & Resumable Shells (Phases 7–9) — SHIPPED 2026-06-13</summary>

- [x] Phase 7: Claude Quota Indicator (3/3 plans) — completed 2026-06-12
- [x] Phase 8: tmux Shells — Spawn & Detach Lifecycle (4/4 plans) — completed 2026-06-13
- [x] Phase 9: tmux Restart Resume & Cleanup Integration (5/5 plans) — completed 2026-06-13

Full details: [milestones/v1.2-ROADMAP.md](milestones/v1.2-ROADMAP.md)

</details>

<details>
<summary>✅ v1.3 GitHub PR Review (Phases 10–13) — SHIPPED 2026-06-14</summary>

- [x] Phase 10: GitHub Foundations (5/5 plans) — completed 2026-06-13
- [x] Phase 11: PR Review Column (4/4 plans) — completed 2026-06-14
- [x] Phase 12: Open-a-Review (7/7 plans) — completed 2026-06-14
- [x] Phase 13: PR Worktree Auto-Cleanup (3/3 plans) — completed 2026-06-14

Full details: [milestones/v1.3-ROADMAP.md](milestones/v1.3-ROADMAP.md)

</details>

<details>
<summary>✅ v1.4 Repo-First Projects (Phases 14–15) — SHIPPED 2026-06-15</summary>

- [x] Phase 14: Managed Checkout Foundations (4/4 plans) — completed 2026-06-14
- [x] Phase 15: Repo-First Creation Flow (3/3 plans) — completed 2026-06-15

Full details: [milestones/v1.4-ROADMAP.md](milestones/v1.4-ROADMAP.md)

</details>

<details>
<summary>✅ v1.5 Sharper Review Column (Phase 16) — SHIPPED 2026-06-17</summary>

- [x] Phase 16: Sharper Review Column (2/2 plans) — completed 2026-06-17

Full details: [milestones/v1.5-ROADMAP.md](milestones/v1.5-ROADMAP.md)

</details>

### 🚧 v1.6 Global Active Sessions Bar (Phase 17) — IN PROGRESS

- [ ] **Phase 17: Global Active Sessions Bar** — a persistent bottom bar showing every active agent session across all projects: collapsed stats (per-state counts, waiting emphasized), expanded attention-sorted session list with cross-project click-through, all driven by the existing 5s agent-status poll extended with task title + project name.

## Phase Details

### Phase 17: Global Active Sessions Bar
**Goal**: Give the user a single, always-present, app-wide view of every live Claude agent session across all projects — collapsed for at-a-glance stats (with waiting emphasized), expanded to browse and jump straight into any session, including one in a project that isn't currently open.
**Depends on**: Phase 16 (uses the `/api/agents/status` feed already extended with `prNumber`/`source` in v1.5; this phase adds task title + project name to the same query)
**Requirements**: SBAR-01, SBAR-02, SBAR-03, SBAR-04, SBAR-05, SBAR-06, SBAR-07, SBAR-08, SBAR-09, SBAR-10
**Success Criteria** (what must be TRUE):
  1. A status bar is pinned to the bottom of every screen (board, task view, settings) regardless of which project is open; with no active sessions it stays present showing a quiet empty/zero state (SBAR-01, SBAR-09).
  2. Collapsed (the default), the bar shows global per-state counts (working / waiting / idle) plus a total, and when any session is waiting it draws attention with a pulsing-amber highlight + count without the user expanding (SBAR-02, SBAR-03).
  3. Expanding the bar lists every active agent session as a row (project name · task or PR title · live state), ordered attention-first (waiting → working → idle) (SBAR-04, SBAR-05).
  4. Clicking a session row navigates the user straight into that task's full-page agent view, including when the session belongs to a different project than the one currently open (SBAR-06).
  5. The collapse/expand choice persists across page reloads (default collapsed), and the bar reflects sessions appearing/exiting within ~5s off the existing agent-status poll — each row's project name and task/PR title coming from the one extended request, with no per-session fetch and no new DB migration (SBAR-07, SBAR-08, SBAR-10).
**Plans**: TBD
**UI hint**: yes

## Progress

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. Foundation — Projects & Board | v1.0 | 7/7 | Complete | 2026-06-10 |
| 2. Terminal Engine | v1.0 | 5/5 | Complete | 2026-06-10 |
| 3. Worktree Isolation & Bash Tabs | v1.0 | 6/6 | Complete | 2026-06-10 |
| 4. Claude Code Agent Sessions | v1.0 | 5/5 | Complete | 2026-06-11 |
| 5. Recovery & Review | v1.0 | 5/5 | Complete | 2026-06-11 |
| 6. Settings & Polish | v1.1 | 4/4 | Complete | 2026-06-11 |
| 7. Claude Quota Indicator | v1.2 | 3/3 | Complete | 2026-06-12 |
| 8. tmux Shells — Spawn & Detach Lifecycle | v1.2 | 4/4 | Complete | 2026-06-13 |
| 9. tmux Restart Resume & Cleanup Integration | v1.2 | 5/5 | Complete | 2026-06-13 |
| 10. GitHub Foundations | v1.3 | 5/5 | Complete | 2026-06-13 |
| 11. PR Review Column | v1.3 | 4/4 | Complete | 2026-06-14 |
| 12. Open-a-Review | v1.3 | 7/7 | Complete | 2026-06-14 |
| 13. PR Worktree Auto-Cleanup | v1.3 | 3/3 | Complete | 2026-06-14 |
| 14. Managed Checkout Foundations | v1.4 | 4/4 | Complete | 2026-06-14 |
| 15. Repo-First Creation Flow | v1.4 | 3/3 | Complete | 2026-06-15 |
| 16. Sharper Review Column | v1.5 | 2/2 | Complete | 2026-06-17 |
| 17. Global Active Sessions Bar | v1.6 | 0/0 | Not started | - |

---
*v1.0 shipped 2026-06-11 — 5 phases, 28 plans, 74 tasks*
*v1.1 shipped 2026-06-11 — 1 phase, 4 plans, 10 tasks*
*v1.2 shipped 2026-06-13 — 3 phases, 12 plans, 29 tasks*
*v1.3 shipped 2026-06-14 — 4 phases (10–13), 19 plans, 46 tasks, 20 requirements*
*v1.4 shipped 2026-06-15 — 2 phases (14–15), 7 plans, 13 tasks, 10 requirements*
*v1.5 shipped 2026-06-17 — 1 phase (16), 2 plans, 6 tasks, 9 requirements*
*v1.6 in progress — 1 phase (17), 10 requirements (SBAR-01..SBAR-10)*
