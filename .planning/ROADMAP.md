# Roadmap: Kangent → Kamacu

## Milestones

- ✅ **v1.0 MVP** — Phases 1–5 (shipped 2026-06-11) — see [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md)
- ✅ **v1.1 Settings & Polish** — Phase 6 (shipped 2026-06-11) — see [milestones/v1.1-ROADMAP.md](milestones/v1.1-ROADMAP.md)
- ✅ **v1.2 Quota & Resumable Shells** — Phases 7–9 (shipped 2026-06-13) — see [milestones/v1.2-ROADMAP.md](milestones/v1.2-ROADMAP.md)
- ✅ **v1.3 GitHub PR Review** — Phases 10–13 (shipped 2026-06-14) — see [milestones/v1.3-ROADMAP.md](milestones/v1.3-ROADMAP.md)
- ✅ **v1.4 Repo-First Projects** — Phases 14–15 (shipped 2026-06-15) — see [milestones/v1.4-ROADMAP.md](milestones/v1.4-ROADMAP.md)
- ✅ **v1.5 Sharper Review Column** — Phase 16 (shipped 2026-06-17) — see [milestones/v1.5-ROADMAP.md](milestones/v1.5-ROADMAP.md)
- ✅ **v1.6 Global Active Sessions Bar** — Phase 17 (shipped 2026-06-18) — see [milestones/v1.6-ROADMAP.md](milestones/v1.6-ROADMAP.md)
- ✅ **v1.7 Project Icons in Collapsed Sidebar** — Phases 18–19 (shipped 2026-06-19) — see [milestones/v1.7-ROADMAP.md](milestones/v1.7-ROADMAP.md)
- 🚧 **v1.8 Kamacu Rebrand & UX Polish** — Phases 20–24 (in progress, started 2026-07-01)

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

<details>
<summary>✅ v1.7 Project Icons in Collapsed Sidebar (Phases 18–19) — SHIPPED 2026-06-19</summary>

- [x] Phase 18: Project Icon Data Foundation (3/3 plans) — completed 2026-06-19
- [x] Phase 19: Sidebar Avatars & Settings Editors (3/3 plans) — completed 2026-06-19

Full details: [milestones/v1.7-ROADMAP.md](milestones/v1.7-ROADMAP.md)

</details>

### 🚧 v1.8 Kamacu Rebrand & UX Polish (In Progress)

**Milestone Goal:** Rebrand kangent → Kamacu (full code/binary/module rename plus a one-time, gated on-disk migration of `~/.kangent` → `~/.kamacu` that keeps live worktrees, sessions, and DB intact) and sharpen the daily-driver UX — a GitHub-style "Files changed" diff view, a Settings worktree-cleanup panel, renameable bash/tmux tabs, a task-view actions menu that replaces the auto-inserted review prompt, and session-bar / board refinements.

- [ ] **Phase 20: Kamacu Rebrand & Brand** — Rename to Kamacu across code/binary/module/UI and ship the new logo, favicon, and README (no data-path change yet)
- [ ] **Phase 21: Data Directory Migration** — One-time gated `~/.kangent` → `~/.kamacu` move that repairs worktrees, rewrites DB paths, switches the tmux socket/prefix, and migrates localStorage
- [ ] **Phase 22: GitHub-Style Diff Review** — File-tree diff view with per-file collapse and sticky "Viewed" state that resets only when a file changes
- [ ] **Phase 23: Worktree Cleanup Panel** — Settings panel listing every worktree (incl. orphans) with per-item force-remove and a bulk "clean eligible" action
- [ ] **Phase 24: Session-Bar, Board & Tab Polish** — Renameable tabs, a task-view three-dots menu (no auto review-prompt), and session-bar / board cleanups

## Phase Details

### Phase 20: Kamacu Rebrand & Brand

**Goal**: The product presents itself as Kamacu everywhere — name, binary, Go module, logo, favicon, and a README — with runtime data paths/socket deliberately left unchanged (that flip belongs to Phase 21's gated migration).
**Depends on**: Nothing (first phase of v1.8)
**Requirements**: REBRAND-01, REBRAND-02, REBRAND-03, BRAND-01, BRAND-02, BRAND-03
**Success Criteria** (what must be TRUE):

  1. The browser tab title, sidebar brand title, page/section headers, and settings/about copy read "Kamacu" — no user-facing "kangent" remains (REBRAND-01, REBRAND-03)
  2. `make build` produces a `kamacu` binary; the Go module is renamed and all imports updated; `go build`, `go vet`, and `go test ./...` are green (REBRAND-02)
  3. The UI brand area renders the new Kamacu logo (SVG) and the browser tab shows the Kamacu favicon (BRAND-01, BRAND-02)
  4. The repo root has a `README.md` describing what Kamacu is, how to build and run it, and the project → task → agent → review workflow (BRAND-03)
  5. The rebrand touches only code identity — the `~/.kangent` data dir and `-L kangent` tmux socket are still used at runtime (compatibility deferred to Phase 21) so an existing install keeps working after this phase (REBRAND-03)

**Plans**: 4 plans
Plans:
**Wave 1**

- [ ] 20-01-PLAN.md — Rename Go module/binary/imports/cmd dir + Makefile to kamacu; sweep cosmetic prose (REBRAND-02/03)
- [ ] 20-02-PLAN.md — KamacuMark ember-spark SVG component + favicon + browser-tab title/link (BRAND-01/02, REBRAND-01)

**Wave 2** *(blocked on Wave 1 completion)*

- [ ] 20-03-PLAN.md — Sidebar brand lockup (mark + wordmark) + user-facing copy renames (REBRAND-01/03, BRAND-01)

**Wave 3** *(blocked on Wave 2 completion)*

- [ ] 20-04-PLAN.md — Repo-root README + end-of-phase branding human-verify & screenshot capture (BRAND-03)

**UI hint**: yes

### Phase 21: Data Directory Migration

**Goal**: An existing `~/.kangent` install upgrades cleanly to `~/.kamacu` — live worktrees, sessions, the SQLite DB, and browser state all survive — while a fresh or already-migrated install skips safely and any failure is recoverable.
**Depends on**: Phase 20
**Requirements**: MIGRATE-01, MIGRATE-02, MIGRATE-03, MIGRATE-04, MIGRATE-05
**Success Criteria** (what must be TRUE):

  1. On first launch after upgrade, the data directory — managed repos, worktrees, and the SQLite DB — is moved from `~/.kangent` to `~/.kamacu` and the app runs against the new location (MIGRATE-01)
  2. Every existing task/PR worktree stays valid after the move: worktree links are repaired (`git worktree repair`) and the DB's stored worktree/repo paths are rewritten to `~/.kamacu`, so the task view, sessions, and diff still open (MIGRATE-02)
  3. No running agent is orphaned by the socket switch: pre-existing live `kangent-*` tmux sessions are reattached or cleanly retired, and new sessions use the `-L kamacu` socket with a `kamacu-<task>-<n>` prefix (MIGRATE-03)
  4. Sidebar state, collapse preferences, and panel settings carry over — browser `localStorage` keys under `kangent.*` are migrated to `kamacu.*` (MIGRATE-04)
  5. The migration is idempotent and safe: a fresh or already-migrated install starts without attempting it, and a mid-migration failure leaves the original `~/.kangent` untouched and surfaces a clear error instead of a half-migrated state (MIGRATE-05)

**Plans**: TBD

### Phase 22: GitHub-Style Diff Review

**Goal**: The diff tab reviews changes the way GitHub's "Files changed" does — a left file tree, per-file collapse/expand, and a sticky per-file "Viewed" state that only resets when the file actually changes again.
**Depends on**: Phase 21
**Requirements**: DIFF-01, DIFF-02, DIFF-03, DIFF-04
**Success Criteria** (what must be TRUE):

  1. The diff view shows a left-hand tree of all changed files; selecting a file focuses/scrolls to its diff (DIFF-01)
  2. Each file's diff can be individually collapsed and expanded (DIFF-02)
  3. Marking a file "Viewed" collapses that file, and the viewed state persists across reopening the diff view and across a server restart (DIFF-03)
  4. When a previously-viewed file changes again it is automatically reset to un-viewed and re-expanded on the next open, while unchanged viewed files stay collapsed and viewed (DIFF-04)

**Plans**: TBD
**UI hint**: yes

### Phase 23: Worktree Cleanup Panel

**Goal**: Settings gives the user a full accounting of every worktree and the controls to reclaim them — including the orphaned worktrees the reaper currently leaves behind.
**Depends on**: Phase 21
**Requirements**: WTREE-01, WTREE-02, WTREE-03, WTREE-04
**Success Criteria** (what must be TRUE):

  1. Settings has a worktree-management section listing every worktree with its task/PR association, referenced-vs-orphaned status, and dirty / unpushed / stash flags (WTREE-01)
  2. Orphaned worktrees — present on disk / in `git worktree list` but with no matching DB task — are detected and shown in the list (WTREE-04)
  3. The user can force-remove an individual worktree from the list, overriding the dirty/unpushed/stash gates, behind a confirmation (WTREE-02)
  4. A bulk "clean eligible" action removes all safely-removable worktrees (done/merged-and-pristine, or orphaned) in one action (WTREE-03)

**Plans**: TBD
**UI hint**: yes

### Phase 24: Session-Bar, Board & Tab Polish

**Goal**: Sharpen the daily-driver surfaces — renameable bash/tmux tabs, a task-view actions menu that replaces the auto-inserted review prompt, and a cleaner active-sessions bar and board.
**Depends on**: Phase 21
**Requirements**: TABS-01, TABS-02, REVMENU-01, REVMENU-02, POLISH-01, POLISH-02, POLISH-03
**Success Criteria** (what must be TRUE):

  1. The user can rename a bash/tmux tab to a custom label that persists across reopening the task and a server restart; new tabs still default to Bash 1, Bash 2, … until renamed (TABS-01, TABS-02)
  2. The agent view's top-right Stop button is replaced by a three-dots menu offering "Stop" and "Insert review prompt" (REVMENU-01)
  3. Opening a PR review no longer auto-inserts the review prompt — it appears in the prompt only when the user chooses "Insert review prompt" (REVMENU-02)
  4. The bottom active-sessions bar drops the total-sessions count (per-state colored counts remain) and auto-collapses when the user clicks outside it (POLISH-01, POLISH-02)
  5. The To Do column no longer shows the inline "+ New task" shortcut (POLISH-03)

**Plans**: TBD
**UI hint**: yes

## Progress

**Execution Order:**
Phases execute in numeric order: 20 → 21 → 22 → 23 → 24

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
| 18. Project Icon Data Foundation | v1.7 | 3/3 | Complete | 2026-06-19 |
| 19. Sidebar Avatars & Settings Editors | v1.7 | 3/3 | Complete | 2026-06-19 |
| 20. Kamacu Rebrand & Brand | v1.8 | 0/4 | Planned | - |
| 21. Data Directory Migration | v1.8 | 0/TBD | Not started | - |
| 22. GitHub-Style Diff Review | v1.8 | 0/TBD | Not started | - |
| 23. Worktree Cleanup Panel | v1.8 | 0/TBD | Not started | - |
| 24. Session-Bar, Board & Tab Polish | v1.8 | 0/TBD | Not started | - |

---
*v1.0 shipped 2026-06-11 — 5 phases, 28 plans, 74 tasks*
*v1.1 shipped 2026-06-11 — 1 phase, 4 plans, 10 tasks*
*v1.2 shipped 2026-06-13 — 3 phases, 12 plans, 29 tasks*
*v1.3 shipped 2026-06-14 — 4 phases (10–13), 19 plans, 46 tasks, 20 requirements*
*v1.4 shipped 2026-06-15 — 2 phases (14–15), 7 plans, 13 tasks, 10 requirements*
*v1.5 shipped 2026-06-17 — 1 phase (16), 2 plans, 6 tasks, 9 requirements*
*v1.6 shipped 2026-06-18 — 1 phase (17), 2 plans, 5 tasks, 10 requirements (SBAR-01..SBAR-10)*
*v1.7 shipped 2026-06-19 — 2 phases (18–19), 6 plans, 12 tasks, 12 requirements (ICON-01..ICON-12)*
*v1.8 in progress (started 2026-07-01) — 5 phases (20–24), 26 requirements (REBRAND/MIGRATE/BRAND/DIFF/WTREE/TABS/REVMENU/POLISH)*
