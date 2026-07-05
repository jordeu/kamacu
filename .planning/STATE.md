---
gsd_state_version: 1.0
milestone: v1.9
milestone_name: Workspaces
status: planning
stopped_at: Phase 25 context gathered
last_updated: "2026-07-05T06:24:07.016Z"
last_activity: 2026-07-05 — v1.9 roadmap created (2 phases, 12/12 requirements mapped)
progress:
  total_phases: 2
  completed_phases: 0
  total_plans: 0
  completed_plans: 0
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-07-05)

**Core value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.
**Current focus:** v1.9 Workspaces — Phase 25 (Workspace Data Foundation) ready to plan

## Current Position

Phase: 25 — Workspace Data Foundation (not started)
Plan: —
Status: Roadmap created — ready to plan Phase 25
Last activity: 2026-07-05 — v1.9 roadmap created (2 phases, 12/12 requirements mapped)

### v1.9 Roadmap (Phases 25–26, continues numbering from v1.8's Phase 24)

Coarse granularity, yolo mode, parallelization on, ui_phase on. 12 requirements → 2 phases, 100% mapped (Phase 25: 2 · Phase 26: 10).

- **Phase 25 — Workspace Data Foundation** (WSDATA-01, WSDATA-02): the backend/data layer the switcher UI builds on. Migration `00012` adds a `workspaces` table + `projects.workspace_id` FK (NOT NULL, so a project can never be workspace-less); a one-time idempotent startup backfill creates the protected default **Personal** workspace and assigns every pre-existing project to it (model on the existing `BackfillProjectIcons` startup hook). `workspace_id` is surfaced on the projects wire for Phase 26. First phase; depends on nothing new (extends the existing schema).
- **Phase 26 — Workspace Switcher, Management & Assignment** (WSMGMT-01/02/03/04, WSNAV-01/02/03, WSPROJ-01/02, WSBAR-01): the full user-facing feature. A workspaces CRUD API (create / rename / delete-block-when-empty / Personal-protected) + the sidebar workspace switcher (expanded-state only, hidden in the collapsed rail) with add/rename/delete; the sidebar project list (expanded rows AND collapsed icon rail) filters to the active workspace; the active workspace persists in `localStorage` and switching navigates to that workspace's first project (empty state when none); project transfer via the existing per-project `⋯` menu; new projects land in the active workspace; and a WSBAR-01 non-regression guardrail keeping the global Active Sessions bar cross-workspace. Depends on Phase 25.

Execution order: 25 → 26. Phase 26 is one cohesive full-stack feature phase (CRUD API + all UI) that needs only the Phase 25 data foundation landed first; plan-phase will decompose it into waves.

### Planning grounding for v1.9 (from a fresh code map — treat as fact)

- **Scoping decisions locked (do not re-open):** delete is block-until-empty (never cascade); Personal is renamable but NEVER deletable and at least one workspace always exists; the switcher is expanded-sidebar only (hidden in the collapsed rail) while the project list filters in BOTH sidebar states; active workspace lives in `localStorage`, restored on reload, and switching navigates to that workspace's first project (empty state when none); new projects land in the active workspace; project transfer is via the existing per-project `⋯` menu (NOT drag-and-drop); the global Active Sessions bar stays cross-workspace (WSBAR-01 is a non-regression guardrail, likely folded into the UI phase, not its own phase); workspaces are name-only (no icons/colors) for v1.9.
- **Migrations (Phase 25):** `internal/store/migrations/NNNNN_snake.sql`, goose Up/Down, embedded + run at startup (`internal/store/migrate.go`). Latest is `00011_diff_viewed.sql`; the workspaces migration is `00012`. The idempotent startup backfill models on the existing `BackfillProjectIcons` hook (`internal/api/icons.go`) run after `Migrate`.
- **Projects CRUD (Phases 25–26):** `internal/api/projects.go` (`projectHandlers`: `list()`, `create()`, `createByRepo()`, `update()` partial-PATCH, `delete()`); `projectColumns` const; routes in `internal/api/routes.go` (GET/POST `/api/projects`, PATCH/DELETE `/api/projects/{id}`). `workspace_id` joins these — carried on the projects wire (Phase 25), set at create to the active workspace (WSPROJ-02), and transferable via partial-PATCH (WSPROJ-01). Workspaces get their own CRUD surface (e.g. `/api/workspaces`) in Phase 26.
- **Sidebar (Phase 26):** `web/src/components/sidebar/ProjectSidebar.tsx` renders both expanded rows and the collapsed `collapsible="icon"` rail from `useProjects()` (`web/src/api/queries.ts` → `GET /api/projects`, queryKey `["projects"]`). Sidebar header is brand lockup + `SidebarTrigger` only — the switcher is NEW, expanded-only. Per-project `⋯` menu is `web/src/components/sidebar/ProjectMenu.tsx` (Rename / Project settings / Delete via shadcn `DropdownMenu`) — WSPROJ-01 adds a "Move to workspace…" item here. Add-project button + `AddProjectDialog` in the `SidebarFooter`.
- **Current-project selection is URL-only (Phase 26):** routes `/projects/:projectId` and `/projects/:projectId/tasks/:taskId` (`web/src/App.tsx`); index `/` redirects to the first project (`RedirectToFirstProject`). No zustand — TanStack Query + URL + localStorage only. WSNAV-03 navigation reuses this; workspace stays OUT of the URL (localStorage-only, per Out of Scope).
- **Active sessions bar (Phase 26, WSBAR-01 guardrail):** `web/src/components/layout/ActiveSessionsBar.tsx`, mounted globally in `AppLayout.tsx` outside `<Outlet/>`; data from `GET /api/agents/status` (already cross-project, JOINs projects, no filter). WSBAR-01 = keep it global; do NOT add a workspace filter.
- **localStorage keys (Phase 26, WSNAV-03):** `kamacu.sidebar`, `kamacu:sessions-bar-collapsed`, `kamacu:review-collapsed:${projectId}`; boot migration shim `web/src/lib/migrateStorage.ts`. The active-workspace key follows this `kamacu.*` convention (e.g. `kamacu.workspace`).
- **Verification model:** backend via `go test ./...` / `go build` / `go vet`; frontend via `cd web && npm run build` (tsc -b + vite build) + `npm run lint` + a human-verify checkpoint (NO frontend test framework). Phase 25's migration + backfill warrant an upgrade-path check (existing install → every project lands under Personal, idempotent on re-run; no project left workspace-less).

### Phase 19 UAT decisions (visual revisions, user-approved — treat as the new contract)

- **Avatar shape (reverses D-04):** `rounded-md` square → `rounded-full` circle.
- **Active indicator (reverses D-06):** dropped the rail-only ring on `<ProjectAvatar>` (removed the `active` prop); the selected/open project is highlighted by the rail `SidebarMenuButton`'s filled rounded-md `bg-sidebar-accent` (a 40px filled-row, the SAME treatment as the expanded selection). Dim `sidebar-ring` was invisible; a near-white ring was ugly.
- **Palette (amends D-01/D-13):** the 9 bright Tailwind-600 hues → 9 muted desaturated hues (still ≥4.5:1 white-text contrast, ~2.7–3.4:1 vs the dark rail). Changed BOTH the Go source of truth (`internal/api/icons.go projectPalette`, a Phase 18 artifact) and the TS mirror (`web/src/lib/palette.ts`); added migration `00010_muted_palette.sql` to remap existing rows by palette position. **Cross-phase note:** a Phase 18 file was modified during Phase 19 UAT.
- **Rail avatar size:** `size-8` → `size-6` (24px); `kangent` brand title hidden in icon mode (it was overflowing the rail). NOTE for Phase 20: this brand title is one of the REBRAND-01 rename sites.

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

v1.8 roadmap-time decisions (settled at roadmap creation — treat as planning constraints):

- **REBRAND vs MIGRATE split (Phase 20 vs 21):** the kangent→kamacu change is split by risk. Phase 20 does code-identity only (module/binary/UI/logo/README) and explicitly does NOT touch the `~/.kangent` path constant or the `-L kangent` tmux socket constant. Phase 21 owns the runtime-path flip AND the on-disk data move together, atomically gated — so the socket/path constants change only alongside the live-data reconciliation that keeps existing sessions/worktrees from breaking. A kamacu-named binary that still reads `~/.kangent` between Phase 20 and Phase 21 is an accepted intermediate state within the milestone.
- **BRAND rides Phase 20:** logo/favicon/README are low-risk and grouped with the code rebrand rather than given a standalone phase (coarse granularity, avoid over-fragmenting).
- **Small UX items grouped (Phase 24):** TABS + REVMENU + POLISH are all daily-driver UX refinements to the session bar / board / task view and are grouped into one polish phase rather than three tiny phases.
- **Out of scope (do not scope-creep in planning):** renaming the GitHub repo/remote; permanent dual-path `~/.kangent` + `~/.kamacu` support; auto-inserting any agent prompt; per-project diff/cleanup settings. See REQUIREMENTS.md "Out of Scope".

v1.7 milestone-time decisions (settled with the user before roadmapping — treat as constraints going into planning):

- **Default letters (ICON-02):** word initials for multi-word names, else first two letters; uppercased; max 2 chars.
- **Default color (ICON-03/12):** random from a CURATED dark-theme-friendly palette (not arbitrary hex); editing = pick a preset swatch. Define the curated palette as ONE shared constant used by both the random-default assignment and the settings swatch picker.
- **Shared avatar component (ICON-05/10):** the monogram avatar is shown in BOTH the collapsed rail AND beside the name when expanded — implement ONE shared avatar component reused in both places.
- **Collapsed rail scope:** keeps active-project indication (ICON-07), full-name tooltip on hover (ICON-08), and the amber waiting badge overlay (ICON-09, reusing the existing per-project waiting count). It does NOT add Add/Settings icons (deferred, ICON-FUT-05).
- **Schema (ICON-04):** new `projects.icon_letters` + `projects.icon_color` columns via migration 00009 with a backfill for existing projects (derived letters + an assigned palette color). Backfill's random-per-row color was done as a Go startup hook (`BackfillProjectIcons`), a pattern Phase 21's migration one-shot can model on.

v1.7 codebase grounding (orchestrator-verified — treat as fact):

- Projects sidebar is `web/src/components/sidebar/ProjectSidebar.tsx`, rendering `<Sidebar>` from `web/src/components/ui/sidebar.tsx` with `collapsible="icon"` (3rem rail of avatars). The `kangent` brand title is hidden in icon mode — it is a REBRAND-01 rename site (Phase 20).
- Open/collapsed state is controlled in `web/src/components/layout/AppLayout.tsx` via `useState` backed by `localStorage` key `kangent.sidebar` — a MIGRATE-04 rename site (Phase 21: `kangent.sidebar` → `kamacu.sidebar`).
- Project settings are edited in `web/src/components/sidebar/ProjectSettingsDialog.tsx`, opened from `web/src/components/sidebar/ProjectMenu.tsx`. Settings page is a full-page `/settings` route — the WTREE-01 worktree-management section (Phase 23) is a new section there.
- Migrations live in `internal/store/migrations/` (embedded goose, run at startup); latest is `00010_muted_palette.sql`. New migrations for v1.8 (e.g. a per-file diff "Viewed" table for DIFF-03/04, or a tab-label column for TABS-01) continue at `00011_*`.
- Stack: Go stdlib mux + modernc SQLite + goose migrations; React 19 + Vite + Tailwind 4 + shadcn + TanStack Query. NO frontend test framework (no vitest): frontend phases verify via `cd web && npm run build` (tsc -b + vite build) + `npm run lint` + a human-verify checkpoint. Backend verifies via `go test ./...` / `go build` / `go vet`.

(Earlier v1.0–v1.6 per-phase decisions are preserved in the archived milestone files and PROJECT.md Key Decisions.)

### Pending Todos

- Lint-cleanup pass: ~18–20 pre-existing react-hooks eslint errors + a possibly-remaining v1.3 `Date.now()`-in-render advisory in `ReviewColumn.tsx` — gating build green, but a dedicated pass is the right home.
- Phase 21 plan-time question: implement the `~/.kangent` → `~/.kamacu` migration as a startup one-shot (model on the idempotent `BackfillProjectIcons` / orphan-sweep pattern) — decide gating detection (marker file? presence of `~/.kamacu`? a DB migration-version flag?) and failure rollback strategy (move-then-verify vs copy-then-swap; the requirement mandates leaving `~/.kangent` untouched on failure).
- Phase 22 plan-time question: where to persist per-file "Viewed" state (DIFF-03/04) — a new DB table keyed by task + file path + content hash so it survives restart AND auto-resets when the file's content changes.
- Phase 24 plan-time question: where to persist a custom tab label (TABS-01) — a `label` column on the sessions/tmux_sessions row vs a sidecar — so it survives a server restart while defaulting to the auto "Bash N" name.

### Blockers/Concerns

- Plan-mode exit-plan approval → amber waiting dot (v1.0 research OQ1): still unobserved — with `--dangerously-skip-permissions` on by default the waiting state rarely fires.
- **Phase 21 (migration) is the milestone's highest-risk work** — it moves a live data dir, repairs real worktrees, rewrites DB paths, and reconciles running tmux/agent sessions. It warrants especially careful gating, idempotency, and a live end-to-end verification (real `~/.kangent` install → upgrade → restart → reattach) at the human-verify checkpoint. A failure must be non-destructive (original `~/.kangent` intact).

### Quick Tasks Completed

| # | Description | Date | Commit | Status | Directory |
|---|-------------|------|--------|--------|-----------|
| 260613-osu | Warn when a linked GitHub repo cannot be verified (surface verify_state) — completes GHPRJ-03 soft-save-with-warning | 2026-06-13 | 41f3d50 |  | [260613-osu-warn-when-a-linked-github-repo-cannot-be](./quick/260613-osu-warn-when-a-linked-github-repo-cannot-be/) |
| 260613-ph5 | Make GitHub repo-link validation MANDATORY (hard-block invalid repos with highlighted error) — supersedes 260613-osu's soft verify_state advisory; reverses D-11 for the repo-link UX per user decision | 2026-06-13 | 9ea0df6 |  | [260613-ph5-make-github-repo-link-validation-mandato](./quick/260613-ph5-make-github-repo-link-validation-mandato/) |
| 260616-8l7 | Fix Review-column refresh showing stale RED dots: `reduceChecks` now dedupes superseded check runs (keeps the latest run per check name, matching GitHub's rollup state) so a re-run/concurrency-cancelled FAILURE no longer paints a green PR red. Initial cache attempt-floor diagnosis was wrong and discarded (service.go unchanged). Code + tests done; Task 3 human-verify pending (user verifies against live instance). | 2026-06-16 | ccdb2d5 |  | [260616-8l7-the-refresh-button-at-review-column-seem](./quick/260616-8l7-the-refresh-button-at-review-column-seem/) |
| 260618-mlu | Auto-collapse the Active Sessions bottom bar when a session row (task/PR) is opened from the expanded list: added a `collapse()` helper that sets collapsed + persists "1", and the `SessionRow` `onOpen` now navigates AND collapses (covers click + Enter/Space via the existing handler). One-file change to `ActiveSessionsBar.tsx`; build+lint green. Human-verify passed (user approved runtime click-through). NOTE: POLISH-02 (Phase 24) generalizes this to any outside-click. | 2026-06-18 | d3f7795 |  | [260618-mlu-when-the-status-bottom-bar-is-expanded-a](./quick/260618-mlu-when-the-status-bottom-bar-is-expanded-a/) |
| 260625-9db | Fix the fixed bottom status bar (`ActiveSessionsBar`, `h-9`/36px overlay) covering the sidebar footer (Add-project button unreachable) and clipping the bottom of the main task agent/shell view: reserve 36px (`pb-9`) under BOTH `<main>` (AppLayout.tsx) and the sidebar container (`<Sidebar className="pb-9">`, forwarded to the `fixed h-svh` container — TerminalPane's ResizeObserver re-fits xterm automatically). Bar overlay/expand-up behavior unchanged; no edits to the generated shadcn `sidebar.tsx` or `ActiveSessionsBar.tsx`. Build+lint green; human-verify passed (user verified runtime). | 2026-06-25 | 77fe135 |  | [260625-9db-the-bottom-status-bar-is-hidding-the-bot](./quick/260625-9db-the-bottom-status-bar-is-hidding-the-bot/) |
| 260626-hwd | Add opt-in `--insecure-allow-remote` flag to bind kangent to a non-loopback `--addr` (e.g. `0.0.0.0:7333`). A single boolean gates all THREE loopback-enforcement layers — skips `ensureLoopback`, serves the mux without the `hostCheck` wrap, sets `InsecureSkipVerify` on the WS upgrade — plus a loud `slog.Warn` no-auth banner. Added `hookBaseURL()` to normalize a wildcard bind host (`0.0.0.0`/`::`/empty) to `127.0.0.1:<port>` for the local agent-status hook (specific IPs left as-is). SAFE BY DEFAULT: `ensureLoopback`/`hostCheck` bodies byte-for-byte unchanged (verifier md5-confirmed), only conditionally invoked; `TestEnsureLoopback`/`TestHostCheck`/`TestIntegrationEvilOriginRejected` retained & green. NO auth, NO TLS (explicit user-accepted tradeoff). `go build && vet && test ./...` green. | 2026-06-26 | ce03d84 | Verified | [260626-hwd-add-an-opt-in-insecure-allow-remote-flag](./quick/260626-hwd-add-an-opt-in-insecure-allow-remote-flag/) |
| 260704-9yn | Fix Review-column cards overlapping when many are present. PRCards render in a `flex flex-col overflow-y-auto` list (`ReviewColumn.tsx`); flex children default to `flex-shrink:1`, so an overflowing column compressed each card below its natural height and PRCard's `overflow-hidden` then clipped the 2nd title line / spilled the meta row into the next card. Added `shrink-0` to the PRCard root so cards keep content height and the container scrolls. Scoped to ReviewColumn (PRCard's only consumer — awaiting + Recently Reviewed lists); kanban `TaskCard` shares the pattern but has no `overflow-hidden` so it was unaffected. `npm run build` (tsc -b + vite) green; human visually verified all cards render correctly. | 2026-07-04 | f40fe8b | Verified | [260704-9yn-fix-review-column-card-overlap-when-many](./quick/260704-9yn-fix-review-column-card-overlap-when-many/) |
| 260704-a6q | Fix board drag-and-drop overshooting to the next-next column (e.g. In Progress → In Review landed in Done). Root cause: `collisionDetection={closestCorners}` in `Board.tsx` ranks droppables by the dragged card's rectangle-corner distance — a full column-width card dragged sideways had corners reaching into the far column, so it selected the next-next column; `handleDragOver`'s live reflow oscillated the corner-closest target, making the adjacent column unreachable. Fix: pointer-first `boardCollisionDetection` — `pointerWithin` (droppable the cursor is inside; horizontally stable across the vertical reflow), preferring a card over its column so within-column insert index stays precise, with a `closestCorners` fallback for gap-hover + keyboard drag. `npm run build` (tsc -b + vite) green; human visually verified drag-and-drop lands on the correct adjacent column. | 2026-07-04 | cf841e2 | Verified | [260704-a6q-fix-board-drag-and-drop-overshooting-to-](./quick/260704-a6q-fix-board-drag-and-drop-overshooting-to-/) |

## Session Continuity

Last session: 2026-07-05T06:24:07.004Z
Stopped at: Phase 25 context gathered
Resume file: .planning/phases/25-workspace-data-foundation/25-CONTEXT.md
Next: `/gsd:plan-phase 25` to plan the Workspace Data Foundation (migration 00012 `workspaces` table + `projects.workspace_id` FK + idempotent Personal backfill). `/clear` first for a fresh context window.

## Operator Next Steps

- Plan Phase 25 (Workspace Data Foundation) with `/gsd:plan-phase 25`.
