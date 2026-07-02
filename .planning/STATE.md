---
gsd_state_version: 1.0
milestone: v1.8
milestone_name: Kamacu Rebrand & UX Polish
status: executing
stopped_at: Phase 22 context gathered
last_updated: "2026-07-02T04:44:26.115Z"
last_activity: 2026-07-02 -- Phase 22 execution started
progress:
  total_phases: 5
  completed_phases: 2
  total_plans: 17
  completed_plans: 12
  percent: 40
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-07-01)

**Core value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.
**Current focus:** Phase 22 — github-style-diff-review

## Current Position

Phase: 22 (github-style-diff-review) — EXECUTING
Plan: 1 of 5
Status: Executing Phase 22
Last activity: 2026-07-02 -- Phase 22 execution started

### v1.8 Roadmap (Phases 20–24, continues numbering from v1.7's Phase 19)

Coarse granularity, yolo mode. 26 requirements → 5 phases, 100% mapped.

- **Phase 20 — Kamacu Rebrand & Brand** (REBRAND-01/02/03, BRAND-01/02/03): code/binary/module/UI rename to Kamacu + new logo, favicon, README. Deliberately does NOT flip runtime data paths or the tmux socket — that is Phase 21's gated migration. First phase; depends on nothing.
- **Phase 21 — Data Directory Migration** (MIGRATE-01..05): the highest-risk phase. One-time gated `~/.kangent` → `~/.kamacu` move (repos + worktrees + DB), `git worktree repair`, DB worktree/repo path rewrite, tmux `-L kangent` → `-L kamacu` socket + `kangent-*` → `kamacu-*` prefix switch with live-session reconciliation (no orphaned agent), localStorage `kangent.*` → `kamacu.*`; idempotent + failure-safe (leaves `~/.kangent` untouched on error). Depends on Phase 20.
- **Phase 22 — GitHub-Style Diff Review** (DIFF-01..04): file-tree diff view, per-file collapse/expand, sticky per-file "Viewed" state (persists across reopen + restart) that resets + re-expands only when a file changes. Depends on Phase 21.
- **Phase 23 — Worktree Cleanup Panel** (WTREE-01..04): Settings section listing every worktree (referenced/orphaned, dirty/unpushed/stash flags), per-item force-remove (confirmed), bulk "clean eligible", orphan detection (reconciles the reaper's skipped accumulation). Depends on Phase 21.
- **Phase 24 — Session-Bar, Board & Tab Polish** (TABS-01/02, REVMENU-01/02, POLISH-01/02/03): renameable bash/tmux tabs (persist across restart; Bash N defaults kept), task-view three-dots menu (Stop + Insert review prompt) replacing the Stop button with NO auto-inserted PR review prompt, session-bar drops total count + auto-collapses on outside click, To Do "+ New task" shortcut removed. Depends on Phase 21.

Execution order: 20 → 21 → 22 → 23 → 24. Phases 22/23/24 are independent feature work that only need the rebrand+migration foundation (Phase 21) landed first so they build against the final `~/.kamacu` paths.

### Planning grounding for v1.8 (from PROJECT.md / prior STATE — treat as fact)

- **Rebrand seams (Phase 20):** Go module rename touches every import + `go.mod`; `Makefile` build target → `kamacu`; UI brand title lives in the sidebar (`web/src/components/sidebar/ProjectSidebar.tsx`, currently hides the `kangent` brand title in icon mode); browser tab title in `web/index.html`; log lines via `log/slog`. REBRAND-03 must NOT touch the `~/.kangent` path constant or the `-L kangent` tmux socket constant — those are Phase 21's to flip atomically with the data move.
- **Migration seams (Phase 21):** data dir is `~/.kangent/` (managed repos under `repos/`, worktrees under `worktrees/`, plus the SQLite DB); worktree paths + repo roots are stored in the DB (need rewrite after move); tmux uses a dedicated `-L kangent` socket with `kangent-<task>-<n>` session names (`internal/tmux`); the reaper reconciles tmux rows via `has-session`. localStorage keys are `kangent.*` (e.g. `kangent.sidebar` in `AppLayout.tsx`). Migration must be gated (detect fresh/already-migrated), idempotent, and failure-safe. Consider modeling the startup one-shot on the existing startup orphan-sweep / `BackfillProjectIcons` idempotent-startup-hook pattern.
- **Diff seams (Phase 22):** the current diff is a read-only unified-diff tab (`GET /api/tasks/{id}/diff` returns per-file-hunk JSON; frontend renders collapsible unified diffs). DIFF-03/04 need persistent per-file "Viewed" state keyed by file + content hash — new persistence (likely a DB table or a per-worktree store) so it survives restart and resets on content change.
- **Worktree cleanup seams (Phase 23):** reuse `git worktree list --porcelain` + the existing gated `CleanupWorktreeGated` (`force` param already exists — HTTP handler passes force per gate, reaper passes force=false). Orphan = on disk / in `git worktree list` with no matching DB `tasks` row. The reaper currently SKIPS dirty/unpushed worktrees (the accumulation WTREE-04 addresses). New Settings section is a full-page `/settings` route addition.
- **Tabs seams (Phase 24):** bash/tmux tabs get auto labels "Bash 1", "Bash 2", …; TABS-01 needs a persisted custom label (likely a `label`/`name` column on the sessions/tmux_sessions row or a sidecar) surviving restart, defaulting to the auto name (TABS-02).
- **Review-menu / polish seams (Phase 24):** the agent view has a Stop button + a PR-review seed prompt currently prefilled-once-per-session (`pr_review_seed` KV, seeded in TaskPage/agent view) — REVMENU-02 removes the auto-insert, REVMENU-01 moves Stop + a manual "Insert review prompt" into a three-dots menu. The bottom bar is `web/src/components/.../ActiveSessionsBar.tsx` (POLISH-01 total-count removal, POLISH-02 outside-click auto-collapse — a `collapse()` helper already exists from quick task 260618-mlu). POLISH-03 removes the To Do "+ New task" quick-add row.
- **Verification model:** backend via `go test ./...` / `go build` / `go vet`; frontend via `cd web && npm run build` (tsc -b + vite build) + `npm run lint` + a human-verify checkpoint (NO frontend test framework). Phase 21 (migration) warrants especially careful gating + a live end-to-end restart/reattach verification against a real `~/.kangent` install.

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

## Session Continuity

Last session: 2026-07-02T03:48:51.193Z
Stopped at: Phase 22 context gathered
Resume file: .planning/phases/22-github-style-diff-review/22-CONTEXT.md
Next: `/gsd:plan-phase 20` to plan the first v1.8 phase (Kamacu Rebrand & Brand). Execution order 20 → 21 → 22 → 23 → 24. Phase 21 (data migration) is the highest-risk phase — plan its gating/idempotency/rollback carefully.

## Operator Next Steps

- Plan the first v1.8 phase with `/gsd:plan-phase 20` (Kamacu Rebrand & Brand).

</content>
