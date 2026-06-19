---
gsd_state_version: 1.0
milestone: v1.7
milestone_name: Project Icons in Collapsed Sidebar
status: executing
stopped_at: Phase 19 plans complete (3/3), pending phase verification
last_updated: "2026-06-19T08:55:00.000Z"
last_activity: 2026-06-19 -- Phase 19 Plans 02 + 03 complete (rail + settings editors); UAT-approved after 3 visual-revision rounds
progress:
  total_phases: 2
  completed_phases: 2
  total_plans: 6
  completed_plans: 6
  percent: 100
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-06-19)

**Core value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.
**Current focus:** Phase 19 — sidebar-avatars-settings-editors

## Current Position

Phase: 19 (sidebar-avatars-settings-editors) — PLANS COMPLETE (3/3), pending phase verification
Plan: 3 of 3 complete
Status: All Phase 19 plans done + human-verify approved; next is phase verification (code-review + verify-phase gates)
Last activity: 2026-06-19 -- Phase 19 Plans 02 + 03 complete; UAT-approved

### Phase 19 UAT decisions (visual revisions, user-approved — treat as the new contract)

- **Avatar shape (reverses D-04):** `rounded-md` square → `rounded-full` circle.
- **Active indicator (reverses D-06):** dropped the rail-only ring on `<ProjectAvatar>` (removed the `active` prop); the selected/open project is highlighted by the rail `SidebarMenuButton`'s filled rounded-md `bg-sidebar-accent` (a 40px filled-row, the SAME treatment as the expanded selection). Dim `sidebar-ring` was invisible; a near-white ring was ugly.
- **Palette (amends D-01/D-13):** the 9 bright Tailwind-600 hues → 9 muted desaturated hues (still ≥4.5:1 white-text contrast, ~2.7–3.4:1 vs the dark rail). Changed BOTH the Go source of truth (`internal/api/icons.go projectPalette`, a Phase 18 artifact) and the TS mirror (`web/src/lib/palette.ts`); added migration `00010_muted_palette.sql` to remap existing rows by palette position. **Cross-phase note:** a Phase 18 file was modified during Phase 19 UAT.
- **Rail avatar size:** `size-8` → `size-6` (24px); `kangent` brand title hidden in icon mode (it was overflowing the rail).

## Performance Metrics

**Velocity (v1.6):**

- Plans completed: 2 across 1 phase (17); 5 tasks
- Headline: a persistent global bottom bar showing every live agent session across all projects (collapsed counts + expanded attention-sorted cross-project list)

**Velocity (v1.5):**

- Plans completed: 2 across 1 phase (16); 6 tasks
- Headline: two independent at-a-glance signals per PR card (agent left rail + CI glyph) + "Recently reviewed" section

**Velocity (v1.3):**

- Plans completed: 19 across 4 phases (10–13); 46 tasks
- Headline: clicking a PR opens it as a full task-like review workspace on the PR's real head branch; reaper auto-cleans merged/closed worktrees

**Velocity (v1.2):**

- Plans completed: 12 across 3 phases (7, 8, 9)
- Headline: tmux sessions survive a server restart and auto-reattach invisibly; first background goroutine (Done-TTL reaper)

| Phase | Plans | Notable |
|-------|-------|---------|
| 6 | 4/4 | P01 11 min / P02 24 min / P03 8 min / P04 25 min (incl. human gate) |

Historical per-plan timings preserved in `.planning/milestones/` archives and git history.
| Phase 07-claude-quota-indicator P02 | 5 min | 3 tasks | 5 files |
| Phase 07 P01 | 14 min | 3 tasks | 5 files |
| Phase 07-claude-quota-indicator P03 | 1h 8m | 2 tasks | 1 files |
| Phase 08-tmux-shells-spawn-detach-lifecycle P01 | 6min | 3 tasks | 3 files |
| Phase 08-tmux-shells-spawn-detach-lifecycle P02 | 8 min | 2 tasks | 4 files |
| Phase 08-tmux-shells-spawn-detach-lifecycle P03 | 9 min | 3 tasks | 3 files |
| Phase 08-tmux-shells-spawn-detach-lifecycle P04 | 52min | 3 tasks | 4 files |
| Phase 09 P01 | 3 min | 2 tasks | 3 files |
| Phase 09-tmux-restart-resume-cleanup-integration P02 | 4min | 2 tasks | 5 files |
| Phase 09-tmux-restart-resume-cleanup-integration P03 | 14 min | 3 tasks | 11 files |
| Phase 09-tmux-restart-resume-cleanup-integration P04 | 14 min | 3 tasks | 6 files |
| Phase 09-tmux-restart-resume-cleanup-integration P05 | 12 min | 3 tasks | 5 files |
| Phase 10-github-foundations P01 | 7 min | 2 tasks | 5 files |
| Phase 10-github-foundations P02 | 9 min | 3 tasks | 5 files |
| Phase 10-github-foundations P03 | 8 min | 3 tasks | 7 files |
| Phase 10-github-foundations P04 | 4 min | 1 tasks | 3 files |
| Phase 10-github-foundations P05 | 3 min | 3 tasks | 3 files |
| Phase 11-pr-review-column P01 | 6 min | 2 tasks | 4 files |
| Phase 11-pr-review-column P02 | 5min | 2 tasks | 3 files |
| Phase 11-pr-review-column P03 | 3min | 3 tasks | 4 files |
| Phase 11-pr-review-column P04 | ~10 min active (overnight human-verify gate) | 3 tasks | 2 files |
| Phase 12-open-a-review P01 | 6 min | 2 tasks | 4 files |
| Phase 12-open-a-review P02 | 7min | 2 tasks | 3 files |
| Phase 12-open-a-review P03 | 4min | 1 tasks | 2 files |
| Phase 12-open-a-review P04 | 12min | 1 tasks | 3 files |
| Phase 12-open-a-review P05 | 18min | 2 tasks | 7 files |
| Phase 12-open-a-review P06 | 7min | 3 tasks | 9 files |
| Phase 12-open-a-review P07 | ~3h (incl. 3 human-verify rounds) | 4 tasks | 7 files |
| Phase 13-pr-worktree-auto-cleanup P01 | 9 min | 3 tasks | 11 files |
| Phase 13-pr-worktree-auto-cleanup P02 | 9min | 3 tasks | 5 files |
| Phase 13-pr-worktree-auto-cleanup P03 | 2min active (+ human-verify gate) | 3 tasks | 3 files |
| Phase 14-managed-checkout-foundations P01 | 13 min | 3 tasks | 6 files |
| Phase 14-managed-checkout-foundations P02 | 9 min | 2 tasks | 4 files |
| Phase 14-managed-checkout-foundations P03 | 7 min | 2 tasks | 4 files |
| Phase 14-managed-checkout-foundations P04 | 7 min | 2 tasks | 2 files |
| Phase 15 P01 | 5 min | 2 tasks | 7 files |
| Phase 15-repo-first-creation-flow P02 | 3 min | 2 tasks | 2 files |
| Phase 16 P01 | 10 min | 3 tasks | 9 files |
| Phase 16-sharper-review-column P02 | ~40 min | 3 tasks | 4 files |
| Phase 17-global-active-sessions-bar P01 | 9 min | 2 tasks | 3 files |
| Phase 17-global-active-sessions-bar P02 | 9min active (+ human-verify gate) | 3 tasks | 2 files |
| Phase 19-sidebar-avatars-settings-editors P01 | 3min | 2 tasks | 2 files |

## Accumulated Context

### Decisions

Full decision log lives in PROJECT.md (Key Decisions) and the archived milestone files:

- `.planning/milestones/v1.0-ROADMAP.md` / `v1.0-REQUIREMENTS.md`
- `.planning/milestones/v1.1-ROADMAP.md` / `v1.1-REQUIREMENTS.md`
- `.planning/milestones/v1.2-ROADMAP.md` / `v1.2-REQUIREMENTS.md`
- `.planning/milestones/v1.3-ROADMAP.md` / `v1.3-REQUIREMENTS.md`

Notable standing decisions for future work:

- D-51 reversed in v1.1 (AGENT-02): `--dangerously-skip-permissions` is the default extra-param; removable per-settings. Documented side effect: amber waiting dot rarely fires while active.
- Settings are global-only, read-at-use, absent-row-=-code-default; per-project overrides deferred (SET-FUT-01); additional shells are future data, not code (SHELL-FUT-01).

v1.7 milestone-time decisions (settled with the user before roadmapping — treat as constraints going into planning):

- **Default letters (ICON-02):** word initials for multi-word names, else first two letters; uppercased; max 2 chars.
- **Default color (ICON-03/12):** random from a CURATED dark-theme-friendly palette (not arbitrary hex); editing = pick a preset swatch. Define the curated palette as ONE shared constant used by both the random-default assignment and the settings swatch picker.
- **Shared avatar component (ICON-05/10):** the monogram avatar is shown in BOTH the collapsed rail AND beside the name when expanded — implement ONE shared avatar component reused in both places.
- **Collapsed rail scope:** keeps active-project indication (ICON-07), full-name tooltip on hover (ICON-08), and the amber waiting badge overlay (ICON-09, reusing the existing per-project waiting count). It does NOT add Add/Settings icons (deferred, ICON-FUT-05).
- **Schema (ICON-04):** new `projects.icon_letters` + `projects.icon_color` columns via migration 00009 with a backfill for existing projects (derived letters + an assigned palette color). NOTE for planning: a per-row RANDOM palette color in the backfill may be easier done in Go at migration-run time (or via SQL) than as a pure goose `.sql` statement — flag derive-defaults-for-existing-rows explicitly in the Phase 18 plan.

v1.7 codebase grounding (orchestrator-verified — treat as fact):

- Projects sidebar is `web/src/components/sidebar/ProjectSidebar.tsx`, rendering `<Sidebar>` from `web/src/components/ui/sidebar.tsx` with NO `collapsible` prop → defaults to `collapsible="offcanvas"` (collapses to `w-0`, fully off-screen), which is why there is no project reference when collapsed. The collapsed-rail feature = make the collapsed state show a thin avatar rail: either switch to `collapsible="icon"` (3rem `--sidebar-width-icon` rail, already supported by the shadcn sidebar) and style the icon state, or a custom collapsed render.
- Open/collapsed state is controlled in `web/src/components/layout/AppLayout.tsx` via `useState` backed by `localStorage` key `kangent.sidebar` (D-04); the reopen trigger is `CollapsedSidebarTrigger` (top-left). `--sidebar-width` is `15rem`.
- Project settings are edited in `web/src/components/sidebar/ProjectSettingsDialog.tsx` (description + github_repo), opened from `web/src/components/sidebar/ProjectMenu.tsx` — the letters + color editors go HERE.
- Backend project model: `Project` struct + `projectColumns` + `scanProject` in `internal/api/projects.go`; the TS `Project` interface in `web/src/api/types.ts`. The partial-PATCH path for description/github_repo is `useUpdateProjectSettings` (frontend) → projects.go handler — extend it to ALSO accept `icon_letters` + `icon_color`.
- Migrations live in `internal/store/migrations/` (embedded goose, run at startup); latest is `00008_managed_checkout.sql`. New migration = `00009_project_icons.sql`.
- The amber waiting-count chip already exists per-project in `ProjectSidebar.tsx` (`waitingByProject` map from `useAgentStatuses()`); ICON-09 reuses that count as a badge overlay on the collapsed avatar.
- Stack: Go stdlib mux + modernc SQLite + goose migrations; React 19 + Vite + Tailwind 4 + shadcn + TanStack Query. NO frontend test framework (no vitest): frontend phases verify via `cd web && npm run build` (tsc -b + vite build) + `npm run lint` + a human-verify checkpoint. Backend verifies via `go test ./...` / `go build` / `go vet`.

(Earlier v1.0–v1.6 per-phase decisions are preserved in the archived milestone files and PROJECT.md Key Decisions.)

### Pending Todos

- Lint-cleanup pass: ~18–20 pre-existing react-hooks eslint errors + a possibly-remaining v1.3 `Date.now()`-in-render advisory in `ReviewColumn.tsx` — gating build green, but a dedicated pass is the right home.
- Phase 18 plan-time question: implement the existing-row backfill color assignment in Go at migration-run time vs as goose SQL — pick whichever keeps random-per-row assignment clean (goose supports Go migrations).
- Phase 19 plan-time question: switch `<Sidebar>` to `collapsible="icon"` and style the icon rail, vs a custom collapsed render — decide which gives the cleanest avatar rail with active/tooltip/badge.

### Blockers/Concerns

- Plan-mode exit-plan approval → amber waiting dot (v1.0 research OQ1): still unobserved — with `--dangerously-skip-permissions` on by default the waiting state rarely fires; if Phase 19 UAT needs to verify the amber waiting badge on a collapsed avatar (ICON-09), it may need a session run without the skip flag (or a manually-induced waiting state).

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260613-osu | Warn when a linked GitHub repo cannot be verified (surface verify_state) — completes GHPRJ-03 soft-save-with-warning | 2026-06-13 | 41f3d50 | [260613-osu-warn-when-a-linked-github-repo-cannot-be](./quick/260613-osu-warn-when-a-linked-github-repo-cannot-be/) |
| 260613-ph5 | Make GitHub repo-link validation MANDATORY (hard-block invalid repos with highlighted error) — supersedes 260613-osu's soft verify_state advisory; reverses D-11 for the repo-link UX per user decision | 2026-06-13 | 9ea0df6 | [260613-ph5-make-github-repo-link-validation-mandato](./quick/260613-ph5-make-github-repo-link-validation-mandato/) |
| 260616-8l7 | Fix Review-column refresh showing stale RED dots: `reduceChecks` now dedupes superseded check runs (keeps the latest run per check name, matching GitHub's rollup state) so a re-run/concurrency-cancelled FAILURE no longer paints a green PR red. Initial cache attempt-floor diagnosis was wrong and discarded (service.go unchanged). Code + tests done; Task 3 human-verify pending (user verifies against live instance). | 2026-06-16 | ccdb2d5 | [260616-8l7-the-refresh-button-at-review-column-seem](./quick/260616-8l7-the-refresh-button-at-review-column-seem/) |
| 260618-mlu | Auto-collapse the Active Sessions bottom bar when a session row (task/PR) is opened from the expanded list: added a `collapse()` helper that sets collapsed + persists "1", and the `SessionRow` `onOpen` now navigates AND collapses (covers click + Enter/Space via the existing handler). One-file change to `ActiveSessionsBar.tsx`; build+lint green. Human-verify passed (user approved runtime click-through). | 2026-06-18 | d3f7795 | [260618-mlu-when-the-status-bottom-bar-is-expanded-a](./quick/260618-mlu-when-the-status-bottom-bar-is-expanded-a/) |

## Session Continuity

Last session: 2026-06-19T06:43:19.669Z
Stopped at: Phase 19 UI-SPEC approved
Resume file: None
Next: `/gsd:plan-phase 18` — Project Icon Data Foundation (ICON-01..04): migration 00009 adding `icon_letters` + `icon_color` with backfill, default-letter derivation, curated-palette random color assignment, and the project PATCH path to edit both. Then `/gsd:plan-phase 19` for the sidebar avatars + settings editors (ICON-05..12).
