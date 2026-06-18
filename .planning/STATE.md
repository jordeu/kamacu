---
gsd_state_version: 1.0
milestone: v1.6
milestone_name: Global Active Sessions Bar
status: executing
stopped_at: Paused at 17-02-PLAN.md Task 3 (human-verify checkpoint)
last_updated: "2026-06-18T06:29:56.537Z"
last_activity: 2026-06-18
progress:
  total_phases: 1
  completed_phases: 0
  total_plans: 2
  completed_plans: 1
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-06-18)

**Core value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.
**Current focus:** Phase 17 — global-active-sessions-bar

## Current Position

Milestone: v1.6 Global Active Sessions Bar
Phase: 17 (global-active-sessions-bar) — EXECUTING
Plan: 2 of 2
Status: Ready to execute
Last activity: 2026-06-18

**v1.6 goal:** A persistent **bottom status bar** showing every active Claude agent session across all projects — collapsed for global stats, expanded to browse and click into any session. Scoping decisions (settled with the user 2026-06-18, treat as constraints): **agent sessions only** (working/waiting/idle; PR-review agent sessions included), **in-bar alerting only** (pulsing-amber highlight + count; NO browser/OS notifications or tab/favicon changes this milestone), the per-project sidebar "N waiting" chips are **retained** (bar is additive). Built on the existing `/api/agents/status` 5s poll, extended with task title + project name (columns exist — no new migration expected).

**Roadmap shape:** One phase, Phase 17, covers all ten requirements. Coarse granularity + scope notes: this is a small, cohesive, mostly-frontend milestone built on existing infra. The single backend touch (SBAR-10 — JOIN task title + project name onto the existing `/api/agents/status` query) is a small first wave; the rest is the new bottom bar in the app shell (`web/src/components/layout/AppLayout.tsx`) reusing `useAgentStatuses()` (`web/src/api/agents.ts`, already a 5s poll). No split — the backend feed extension and its sole frontend consumer are tightly coupled.

**Next:** `/gsd:plan-phase 17` — Global Active Sessions Bar (SBAR-01..SBAR-10). Likely waves: (1) extend `/api/agents/status` with `taskTitle` + `projectName` via JOIN (SBAR-10), then (2) the persistent collapsible bottom bar — collapsed stats with waiting emphasis (SBAR-01/02/03/09), expanded attention-sorted list with cross-project click-through (SBAR-04/05/06), localStorage collapse persistence + 5s freshness (SBAR-07/08).

**Carried tech debt (non-blocking, build green):** ~18–20 pre-existing react-hooks eslint advisories; the v1.3 `Date.now()`-in-render advisory in `PRCard.tsx` was cleared in v1.5 (the `ReviewColumn.tsx` one may remain); plus a cosmetic stale "dot/border" comment in `ReviewColumn.tsx` / the `agentRail` JSDoc. A dedicated lint/comment-sweep is the right home.

## Performance Metrics

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

v1.6 milestone-time decisions (settled with the user before roadmapping — treat as constraints going into planning):

- **Agent sessions only** in the bar (working/waiting/idle; PR-review agent sessions included) — the bar reads the existing `/api/agents/status` feed, which is agent-only. bash/tmux sessions stay out (SBAR-FUT-05).
- **In-bar alerting only**: a pulsing-amber highlight + waiting count. NO browser/OS notifications, NO tab-title/favicon changes this milestone (SBAR-FUT-03/04).
- **Active (live-PTY) sessions only**: exited sessions drop off the bar (they still surface on cards + via Resume). Keeps "active sessions" honest.
- **Additive, not a replacement**: the per-project sidebar "N waiting" chips are kept; the bar is a new app-wide surface alongside them.
- **Default collapsed, persisted in localStorage** (mirrors the sidebar/Review-column collapse pattern; absent key reads as collapsed).
- **No new DB migration**: SBAR-10's task title + project name come from a JOIN onto the existing status query — the columns already exist. The `/api/agents/status` SELECT was already widened in v1.5 (Phase 16) to carry `prNumber`/`source`; this milestone adds `taskTitle` + `projectName` the same way.
- **No new endpoint**: the bar reuses `useAgentStatuses()` (`web/src/api/agents.ts`, 5s poll); the only backend change is the JOIN onto the existing handler.

Standing decisions still relevant to v1.6:

- The `/api/agents/status` feed is the single source of truth for card dots, the Agent-tab dot, and sidebar waiting chips (research Pattern 4) — one query, no per-project fan-out. The bar becomes a fourth consumer of the same query.
- `source='manual'` vs `source='github_pr'` distinguishes board tasks from PR-review workspaces; PR-review agent sessions ARE in scope for the bar (their title is the PR title).
- The app shell is `web/src/components/layout/AppLayout.tsx` (sidebar + `<Outlet/>` today); collapse-state-in-localStorage is the established pattern there (`kangent.sidebar`).

(Earlier v1.0–v1.5 per-phase decisions are preserved in the archived milestone files and PROJECT.md Key Decisions.)

- [Phase 17-global-active-sessions-bar]: SBAR-10: taskTitle + projectName added to /api/agents/status via a JOIN onto the existing query (both manager-derived and DB-derived passes) — no new endpoint, no new migration; TS AgentStatusEntry extended to match

### Pending Todos

- Quota poll while idle (no connected browsers) is acceptable for the first iteration with jitter + backoff; revisit before milestone close (research tech-debt note).
- Lint-cleanup pass: ~18–20 pre-existing react-hooks eslint errors + a possibly-remaining v1.3 `Date.now()`-in-render advisory in `ReviewColumn.tsx` — gating build green, but a dedicated pass is the right home.
- v1.6 plan-time questions: where exactly the JOIN lives (handler SQL vs a store method); how the bar's fixed bottom strip coexists with the existing full-height `<main>` overflow layout (reserve bottom space vs overlay); whether the expanded list scrolls with a cap or grows unbounded; cross-project navigation route shape (the task agent route already exists — confirm it accepts a project switch).

### Blockers/Concerns

- Plan-mode exit-plan approval → amber waiting dot (v1.0 research OQ1): still unobserved — note that with `--dangerously-skip-permissions` on by default, the waiting state (the bar's headline alert) rarely fires; confirm the amber-emphasis path is exercisable during UAT (may need a session run without the skip flag).
- The bar must never block the view (SBAR-09) — a fixed bottom strip changes the available height for every page; verify the board, task view (xterm fit/resize), and settings all still fit and the terminal still resizes correctly.

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260613-osu | Warn when a linked GitHub repo cannot be verified (surface verify_state) — completes GHPRJ-03 soft-save-with-warning | 2026-06-13 | 41f3d50 | [260613-osu-warn-when-a-linked-github-repo-cannot-be](./quick/260613-osu-warn-when-a-linked-github-repo-cannot-be/) |
| 260613-ph5 | Make GitHub repo-link validation MANDATORY (hard-block invalid repos with highlighted error) — supersedes 260613-osu's soft verify_state advisory; reverses D-11 for the repo-link UX per user decision | 2026-06-13 | 9ea0df6 | [260613-ph5-make-github-repo-link-validation-mandato](./quick/260613-ph5-make-github-repo-link-validation-mandato/) |
| 260616-8l7 | Fix Review-column refresh showing stale RED dots: `reduceChecks` now dedupes superseded check runs (keeps the latest run per check name, matching GitHub's rollup state) so a re-run/concurrency-cancelled FAILURE no longer paints a green PR red. Initial cache attempt-floor diagnosis was wrong and discarded (service.go unchanged). Code + tests done; Task 3 human-verify pending (user verifies against live instance). | 2026-06-16 | ccdb2d5 | [260616-8l7-the-refresh-button-at-review-column-seem](./quick/260616-8l7-the-refresh-button-at-review-column-seem/) |

## Session Continuity

Last session: 2026-06-18T06:29:56.534Z
Stopped at: Paused at 17-02-PLAN.md Task 3 (human-verify checkpoint)
Resume file: None
Next: `/gsd:plan-phase 17` — Global Active Sessions Bar (SBAR-01..SBAR-10); start with the SBAR-10 backend JOIN (task title + project name onto `/api/agents/status`), then the persistent collapsible bottom bar in `AppLayout.tsx` reusing `useAgentStatuses()`
