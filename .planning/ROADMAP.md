# Roadmap: Kangent

## Milestones

- ✅ **v1.0 MVP** — Phases 1–5 (shipped 2026-06-11) — see [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md)
- ✅ **v1.1 Settings & Polish** — Phase 6 (shipped 2026-06-11) — see [milestones/v1.1-ROADMAP.md](milestones/v1.1-ROADMAP.md)
- ✅ **v1.2 Quota & Resumable Shells** — Phases 7–9 (shipped 2026-06-13) — see [milestones/v1.2-ROADMAP.md](milestones/v1.2-ROADMAP.md)
- ✅ **v1.3 GitHub PR Review** — Phases 10–13 (shipped 2026-06-14) — see [milestones/v1.3-ROADMAP.md](milestones/v1.3-ROADMAP.md)
- ✅ **v1.4 Repo-First Projects** — Phases 14–15 (shipped 2026-06-15) — see [milestones/v1.4-ROADMAP.md](milestones/v1.4-ROADMAP.md)
- ✅ **v1.5 Sharper Review Column** — Phase 16 (shipped 2026-06-17) — see [milestones/v1.5-ROADMAP.md](milestones/v1.5-ROADMAP.md)
- ✅ **v1.6 Global Active Sessions Bar** — Phase 17 (shipped 2026-06-18) — see [milestones/v1.6-ROADMAP.md](milestones/v1.6-ROADMAP.md)
- 🚧 **v1.7 Project Icons in Collapsed Sidebar** — Phases 18–19 (in progress)

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

<details>
<summary>✅ v1.6 Global Active Sessions Bar (Phase 17) — SHIPPED 2026-06-18</summary>

- [x] Phase 17: Global Active Sessions Bar (2/2 plans) — completed 2026-06-18

Full details: [milestones/v1.6-ROADMAP.md](milestones/v1.6-ROADMAP.md)

</details>

### 🚧 v1.7 Project Icons in Collapsed Sidebar (Phases 18–19) — IN PROGRESS

- [ ] **Phase 18: Project Icon Data Foundation** - Add `icon_letters` + `icon_color` to projects (migration 00009 with backfill), derive default letters from the name, assign a stable random color from a curated palette at creation, and extend the project PATCH path to edit both.
- [ ] **Phase 19: Sidebar Avatars & Settings Editors** - One shared monogram avatar component shown both as a clickable collapsed-rail icon (active state, name tooltip, amber waiting badge) and beside the name when expanded, plus letters + swatch editors in Project settings.

## Phase Details

### Phase 18: Project Icon Data Foundation

**Goal**: Every project — newly added or pre-existing — has two persisted properties (two uppercase letters + a curated-palette background color) that are sensible by default and editable over the API, so the frontend has stable identity data to render avatars from.
**Depends on**: Nothing in v1.7 (builds on the existing `projects` table, `internal/api/projects.go` model, and the `useUpdateProjectSettings` PATCH path)
**Requirements**: ICON-01, ICON-02, ICON-03, ICON-04
**Success Criteria** (what must be TRUE):

  1. Adding a new project (folder or managed-repo) automatically gives it two uppercase letters derived from its name — first letters of the first two words for multi-word names ("My Cool App" → "MC"), first two letters for single-word names ("kangent" → "KA") — with no extra user action (ICON-01, ICON-02).
  2. A new project is assigned a background color picked at random from a curated, dark-theme-friendly palette (legible against the avatar text), and that color stays the same for the project across server restarts (ICON-03).
  3. After the migration runs, every project that existed before this feature has non-empty letters (derived from its name) and an assigned palette color — no project is left blank (ICON-04).
  4. The project's letters and color survive a server restart (stored in SQLite) and can be read and updated through the project API (the PATCH path now accepts `icon_letters` + `icon_color`), so the Phase 19 editors have a wire path (ICON-04).

**Plans**: 3 plans
Plans:
**Wave 1**

- [ ] 18-01-PLAN.md — Migration 00009 + icons.go helpers (palette, deriveLetters, pickColor, validators, idempotent backfill) with table tests
- [ ] 18-03-PLAN.md — Frontend wire path: add icon_letters + icon_color to the TS Project type and useUpdateProjectSettings PATCH payload

**Wave 2** *(blocked on Wave 1 completion)*

- [ ] 18-02-PLAN.md — Wire helpers into projects.go (struct/columns/scan, both create paths, PATCH validation) + invoke backfill at startup

### Phase 19: Sidebar Avatars & Settings Editors

**Goal**: The user can identify and switch between projects directly from the collapsed sidebar via colored monogram avatars (which also appear beside the name when expanded), and can edit a project's letters and color from Project settings.
**Depends on**: Phase 18 (renders and edits the `icon_letters` + `icon_color` data and PATCH path it establishes)
**Requirements**: ICON-05, ICON-06, ICON-07, ICON-08, ICON-09, ICON-10, ICON-11, ICON-12
**Success Criteria** (what must be TRUE):

  1. When the sidebar is collapsed, the user sees a vertical rail of project monogram avatars (instead of an empty, off-screen sidebar) and can click any avatar to switch to that project without first expanding the sidebar (ICON-05, ICON-06).
  2. In the collapsed rail, the currently-open project's avatar is visually marked active, hovering an avatar shows the full project name in a tooltip, and a project with one or more agents waiting shows the amber waiting indicator as a badge overlaid on its avatar (ICON-07, ICON-08, ICON-09).
  3. The same monogram avatar appears beside the project name in the expanded sidebar, with the existing name display and waiting-count chip behavior preserved (ICON-10).
  4. In Project settings the user can edit the two letters (input normalized/validated to at most two uppercase characters) and change the color by picking from the curated palette swatches, with the current color clearly indicated; saved changes are reflected in both the collapsed rail and the expanded sidebar (ICON-11, ICON-12).

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
| 17. Global Active Sessions Bar | v1.6 | 2/2 | Complete | 2026-06-18 |
| 18. Project Icon Data Foundation | v1.7 | 0/3 | Planned | - |
| 19. Sidebar Avatars & Settings Editors | v1.7 | 0/? | Not started | - |

---
*v1.0 shipped 2026-06-11 — 5 phases, 28 plans, 74 tasks*
*v1.1 shipped 2026-06-11 — 1 phase, 4 plans, 10 tasks*
*v1.2 shipped 2026-06-13 — 3 phases, 12 plans, 29 tasks*
*v1.3 shipped 2026-06-14 — 4 phases (10–13), 19 plans, 46 tasks, 20 requirements*
*v1.4 shipped 2026-06-15 — 2 phases (14–15), 7 plans, 13 tasks, 10 requirements*
*v1.5 shipped 2026-06-17 — 1 phase (16), 2 plans, 6 tasks, 9 requirements*
*v1.6 shipped 2026-06-18 — 1 phase (17), 2 plans, 5 tasks, 10 requirements (SBAR-01..SBAR-10)*
*v1.7 in progress — 2 phases (18–19), 12 requirements (ICON-01..ICON-12)*
