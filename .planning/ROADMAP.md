# Roadmap: Kangent

## Milestones

- ✅ **v1.0 MVP** — Phases 1–5 (shipped 2026-06-11) — see [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md)
- ✅ **v1.1 Settings & Polish** — Phase 6 (shipped 2026-06-11) — see [milestones/v1.1-ROADMAP.md](milestones/v1.1-ROADMAP.md)
- ✅ **v1.2 Quota & Resumable Shells** — Phases 7–9 (shipped 2026-06-13) — see [milestones/v1.2-ROADMAP.md](milestones/v1.2-ROADMAP.md)
- ✅ **v1.3 GitHub PR Review** — Phases 10–13 (shipped 2026-06-14) — see [milestones/v1.3-ROADMAP.md](milestones/v1.3-ROADMAP.md)
- ✅ **v1.4 Repo-First Projects** — Phases 14–15 (shipped 2026-06-15) — see [milestones/v1.4-ROADMAP.md](milestones/v1.4-ROADMAP.md)
- 🚧 **v1.5 Sharper Review Column** — Phase 16 (active, started 2026-06-17)

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

### 🚧 v1.5 Sharper Review Column (Phase 16) — ACTIVE

- [ ] **Phase 16: Sharper Review Column** — Agent-state dot + open-session border on PR cards, CI status as icons, and a "Recently Reviewed" bottom section that holds reviewed PRs until merged/closed

## Phase Details

### Phase 16: Sharper Review Column
**Goal**: The Review column shows two independent at-a-glance signals per PR — *your* agent's state and the PR's CI state — without confusing the two, and stops losing PRs from view after you review them.
**Depends on**: Phase 13 (PR Review column + cards), Phase 12 (`source='github_pr'` task↔PR link + agent sessions). All prerequisite machinery already shipped in v1.3.
**Requirements**: SIGNL-01, SIGNL-02, SIGNL-03, CHECK-01, CHECK-02, REVWD-01, REVWD-02, REVWD-03, REVWD-04
**Success Criteria** (what must be TRUE):
  1. When a PR has an open review session, its card shows the same working/waiting/idle/exited agent dot used on task cards (reused `StatusDot`), and the card gets the colored left border used for waiting tasks (D-44) — in both the "awaiting your review" and "Recently Reviewed" sections. (SIGNL-01/02/03)
  2. A PR's CI status renders as an icon — green check (pass), red cross (fail), orange circle (running/pending) — instead of the old colored CI dot, freeing the colored-dot vocabulary for agent state; a PR with no checks renders no CI indicator at all (no icon, no reserved gutter, no layout shift). (CHECK-01/02)
  3. A "Recently Reviewed" section appears at the bottom of the column, listing open PRs the user has reviewed (approve OR request-changes) via `reviewed-by:@me state:open`, rendered as the same clickable PR cards; a reviewed PR stays there until its PR merges or closes, then drops off. (REVWD-01/02)
  4. A PR appears in exactly one section: the top "awaiting your review" section takes precedence, so a re-requested PR returns to the top and is never duplicated below. (REVWD-03)
  5. The Recently Reviewed section shares the column's manual refresh + visibility-paused auto-poll and the same loading/degraded states, and is quietly omitted (no blank section, no fabricated count) when it holds no PRs. (REVWD-04)
**Scope notes**:
  - All-internal, reuse-heavy. Reuses: the `internal/github` `gh pr list` + reducer + per-repo TTL cache `Service` (a second `reviewed-by:@me state:open` search clones the existing `user-review-requested:@me` path in `prlist.go`); the `StatusDot` component + `GET /api/agents/status`; the `source='github_pr'` task↔PR link (a PR's open-session/agent state is the agent status of its linked `source='github_pr'` task); and the reaper's existing merge/close detection (bounds Recently Reviewed by open state — no new time window). No new external surface beyond the second `gh pr list` search.
  - Natural plan-level seam (NOT a phase boundary): a data plan (second query + cache wiring + two-list top-precedence dedup + exposing/joining open-session agent state per PR) and a rendering plan (CI icons replacing the dot, agent dot + open-session border on cards, the Recently Reviewed section with its reuse of the column's poll/refresh/loading/degraded/empty behavior). Cohesive enough to live in one phase; keep the seam as plan structure.
  - Touched files concentrate in `web/src/components/board/PRCard.tsx` and `web/src/components/board/ReviewColumn.tsx` (plus the `internal/github` list path and the `usePullRequests` hook). The two carried `Date.now()`-in-render lint advisories live in exactly these two files — clearing them here is a natural, non-blocking moment (build gate is already green; do not let it block the phase).
**Plans**: 2 plans
Plans:
- [ ] 16-01-PLAN.md — Data layer: second reviewed-by:@me search + one-cycle cache + server-side dedup (D-12/13/14), pr_number/source on agent status (D-15), extended web/src/api types
- [ ] 16-02-PLAN.md — Rendering: CI icons (CHECK-01/02), agent dot + blue/amber session border on PRCard (SIGNL-01/02/03), Recently reviewed section in ReviewColumn (REVWD-01/03/04)
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
| 16. Sharper Review Column | v1.5 | 0/? | Not started | - |

---
*v1.0 shipped 2026-06-11 — 5 phases, 28 plans, 74 tasks*
*v1.1 shipped 2026-06-11 — 1 phase, 4 plans, 10 tasks*
*v1.2 shipped 2026-06-13 — 3 phases, 12 plans, 29 tasks*
*v1.3 shipped 2026-06-14 — 4 phases (10–13), 19 plans, 46 tasks, 20 requirements*
*v1.4 shipped 2026-06-15 — 2 phases (14–15), 7 plans, 13 tasks, 10 requirements*
*v1.5 active (started 2026-06-17) — 1 phase (16), 9 requirements*
