# Roadmap: Kangent

## Milestones

- ✅ **v1.0 MVP** — Phases 1–5 (shipped 2026-06-11) — see [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md)
- ✅ **v1.1 Settings & Polish** — Phase 6 (shipped 2026-06-11) — see [milestones/v1.1-ROADMAP.md](milestones/v1.1-ROADMAP.md)
- ✅ **v1.2 Quota & Resumable Shells** — Phases 7–9 (shipped 2026-06-13) — see [milestones/v1.2-ROADMAP.md](milestones/v1.2-ROADMAP.md)
- 🚧 **v1.3 GitHub PR Review** — Phases 10–13 (active, started 2026-06-13)

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

### 🚧 v1.3 GitHub PR Review (Phases 10–13)

- [x] **Phase 10: GitHub Foundations** — Schema, global toggle, project link config, and the degrade-don't-break `internal/github` base. (3/3 plans executed; gaps found in UAT — gh-gated enablement + copy) (completed 2026-06-13)
- [x] **Phase 11: PR Review Column** — Read-only `gh`-backed Review column listing review-requested PRs with auto-poll, refresh, and PR cards. (completed 2026-06-14)
- [x] **Phase 12: Open-a-Review** — Click a PR → worktree on the PR branch + reused task view (agent/bash/PR-base diff), board-leak-safe. *(research-flagged)* (12-01..12-05 implemented; human-verify found 5 follow-ups → gap plans 12-06/12-07 pending; NOT verified) (completed 2026-06-14)
- [ ] **Phase 13: PR Worktree Auto-Cleanup** — Reaper reconciles PR state and gated-removes merged/closed review worktrees (branch kept). *(research-flagged)*

## Phase Details

> **Build order is forced by dependencies (A→B→C→D, converged across all four research tracks):** Phase 10's columns/toggle are read by everything; Phase 11's cache populates the `pr_base_ref` that Phase 12 captures; Phase 13 cleans up the worktrees Phase 12 creates and needs Phase 11's `PRState`. Do not reorder.

### Phase 10: GitHub Foundations
**Goal**: A user can turn GitHub integration on/off globally and link a project to a GitHub repo with an optional description — and when `gh` is missing or the toggle is off, the rest of Kangent is byte-for-byte unchanged.
**Depends on**: Phase 9 (v1.2 complete) — the first phase of this milestone; builds on existing settings KV, migration, and PATCH patterns.
**Requirements**: GHSET-01, GHSET-02, GHSET-03, GHPRJ-01, GHPRJ-02, GHPRJ-03
**Success Criteria** (what must be TRUE):
  1. User can toggle GitHub integration on/off from the global settings page, and it is ON by default (GHSET-01).
  2. With the toggle OFF, no GitHub UI appears anywhere — no per-project GitHub config, no Review column — and the board, tasks, sessions, and every existing flow behave exactly as they did before v1.3 (GHSET-02).
  3. User can open a per-project config section and set, edit, or clear a short description on a project (GHPRJ-01, GHPRJ-02).
  4. User can link a project to a GitHub repo (`owner/name`), and change or clear the link; the link is validated via `gh` when present and falls back to syntactic validation when `gh` is absent (GHPRJ-03).
  5. **Degrade test:** renaming/removing the `gh` binary (or having it unauthenticated) leaves the whole app fully usable — linking still accepts a syntactically valid repo, and nothing in core flows breaks (GHSET-03).
**Plans**: 5 plans (3 original + 2 gap closure)
- [x] 10-01-PLAN.md — Schema migration 00007 + github_integration settings key (on/off, default on)
- [x] 10-02-PLAN.md — internal/github link canonicalization + partial PATCH (description + repo) + origin auto-detect endpoint
- [x] 10-03-PLAN.md — shadcn Switch + Settings GitHub section + Project settings dialog with OFF cascade
- [x] 10-04-PLAN.md — (gap closure) GET /api/github/status gh-availability endpoint
- [x] 10-05-PLAN.md — (gap closure) gh-gate the integration toggle (default off + enable guard) + fix toggle copy
**UI hint**: yes

### Phase 11: PR Review Column
**Goal**: On a linked project's board, a user sees a live, collapsible "Review" column of the open PRs that need their review — with the column appearing/clearing as review state changes — without any of it ever blocking the board.
**Depends on**: Phase 10 (needs `github_repo` link + the `github_integration` toggle). Pure read path — no worktree or task mutation — so it stands up the whole `gh` integration at the lowest risk.
**Requirements**: GHCOL-01, GHCOL-02, GHCOL-03, GHCOL-04, GHCOL-05, GHCOL-06
**Success Criteria** (what must be TRUE):
  1. When a project is linked to a GitHub repo and integration is on, a collapsible "Review" column appears on the right of that project's board, and the user can collapse/expand it — the collapsed column shows a PR count and the collapse state is remembered (GHCOL-01, GHCOL-06).
  2. The column lists the open PRs in the linked repo where review is requested directly from the user (`user-review-requested:@me`), excluding drafts, and the list matches GitHub's own filtered "Review requested" page for that qualifier (GHCOL-02).
  3. Each PR shows as a card resembling a task card with its number, title, author, relative "updated X ago" time, and a pass/fail/pending CI-checks rollup pill (GHCOL-03).
  4. The column auto-refreshes on an interval that is paused when the browser tab is hidden, offers a manual refresh, and mirrors live GitHub state — a card disappears once the user submits its review and reappears on a re-request (GHCOL-04).
  5. The column surfaces clear loading, empty ("you're all caught up"), and degraded (`gh` missing / unauthenticated / error) states inline, never as a modal and never blocking the board (GHCOL-05).
**Plans**: 4 plans (4 waves — backend leaf → endpoint → frontend hook+card → column+board wiring)
- [x] 11-01-PLAN.md — internal/github ListReviewRequested (gh pr list + statusCheckRollup reduction) + best-effort per-repo Service (quota clone)
- [x] 11-02-PLAN.md — always-200 GET /api/projects/{id}/pull-requests endpoint (toggle+link gated) + main.go wiring
- [x] 11-03-PLAN.md — usePullRequests/useRefreshPullRequests hooks + shared formatAgo + PRCard
- [x] 11-04-PLAN.md — ReviewColumn (collapse/localStorage/inline states) + Board.tsx wiring + human-verify checkpoint
**UI hint**: yes

### Phase 12: Open-a-Review
**Goal**: Clicking a PR card opens it as a full task-like review workspace — a worktree on the PR's branch with the same agent, bash, and diff tabs — without ever disturbing the project's primary checkout or leaking PR reviews onto the kanban board.
**Depends on**: Phase 11 (the PR list/cache populates `pr_base_ref`) and Phase 10 (the `tasks.source`/`pr_number`/`pr_base_ref` columns). This is the milestone's headline and its highest-risk regression surface.
**Requirements**: GHREV-01, GHREV-02, GHREV-03, GHREV-04, GHREV-05
**Success Criteria** (what must be TRUE):
  1. Clicking a PR card opens a task-like review view backed by a worktree with the PR's branch checked out (by fetching `refs/pull/<n>/head`, never `gh pr checkout`, never a fresh task branch), reusing the existing agent, bash, and diff tabs (GHREV-01).
  2. Opening the same PR again reattaches to its existing review workspace instead of creating a duplicate (GHREV-02).
  3. The review view's diff is computed against the PR's own base branch merge-base, and Kangent's review diff matches GitHub's Files-changed for that PR (GHREV-03).
  4. PR review workspaces never appear as cards on the kanban board — they are not To Do / In Progress / In Review / Done tasks (GHREV-04).
  5. **Checkout-safety test:** the primary checkout's HEAD is unchanged after opening a fork PR whose branch name collides with a local branch — fork and same-name PRs open correctly without disturbing the main checkout (GHREV-05).
**Plans**: 7 plans (5 waves — original 12-01..05 + gap plans 12-06 backend / 12-07 frontend closing the 12-05 human-verify follow-ups)
- [x] 12-01-PLAN.md — worktree.CheckoutPR (detached refs/pull/<n>/head, pinned to headRefOid) + FetchRef + github.ViewPR/PRDetail
- [x] 12-02-PLAN.md — board-leak audit: source='manual' on 5 board/position queries + /move 409 guard + source/pr_number/pr_base_ref on the Task wire shape
- [x] 12-03-PLAN.md — diff base branched on source/pr_base_ref (fetch base + merge-base origin/<base>, reuse diff.Compute)
- [x] 12-04-PLAN.md — POST .../pull-requests/{n}/review open-or-reattach endpoint (ViewPR + CheckoutPR, find-by project+pr_number) + main.go wiring
- [x] 12-05-PLAN.md — frontend: PR card open trigger + useOpenReview + TaskPage source branches (read-only title/meta/Description) + seeded Start + human-verify checkpoint
- [x] 12-06-PLAN.md — gap (backend): named-branch PR checkout (headRefName, pr/<n> fallback; GHREV-05 safety preserved) + commits/head in the open response + pr_review_seed settings key
- [x] 12-07-PLAN.md — gap (frontend): seed once-per-session from the pr_review_seed setting + GitHub-style merge line + PR-review terminal height fix + Settings prompt field + human re-test
**UI hint**: yes
**Research flag**: yes — carried forward from research SUMMARY. The PR-branch worktree checkout is the milestone's risk center (`gh pr checkout` is not worktree-aware — cli/cli#972; fails on `/`-branches — cli/cli#3231; fork same-name fast-forward — cli/cli#8383). The `git fetch refs/pull/<n>/head` + `worktree add` path is verified end-to-end, but the named-branch-vs-`--detach` choice and the per-query `WHERE source='manual'` audit of EVERY `tasks` SELECT (the single highest-risk board-leak regression) warrant a focused spike. `/gsd:plan-phase` should decide on `/gsd:research-phase`. Carry these as success criteria: "primary checkout HEAD unchanged after opening a fork PR with a colliding branch name" (above, #5); "Kangent's review diff matches GitHub's Files-changed" (above, #3).

### Phase 13: PR Worktree Auto-Cleanup
**Goal**: When a PR merges or closes, its clean review worktree disappears on its own — while a dirty or busy one is never silently removed and the branch always survives.
**Depends on**: Phase 12 (PR tasks with worktrees to clean up) and Phase 11 (`internal/github.PRState`). Last because it operates on the artifacts the earlier phases create, and the shared-helper extraction is safest once the manual cleanup path is stable.
**Requirements**: GHCLN-01, GHCLN-02, GHCLN-03
**Success Criteria** (what must be TRUE):
  1. **Auto-cleanup test:** a merged PR's *clean* review worktree disappears on its own (server-side, detected by the always-on reaper via `gh pr view --json state`, never the browser poll) (GHCLN-01).
  2. **Safety test:** a *dirty* or *busy* (running-session / uncommitted-or-unpushed) merged/closed PR worktree is NOT silently removed — it is left for manual cleanup — and the branch is always kept (the user may not own it) (GHCLN-02).
  3. User can also clean up a PR review worktree manually from the review view using the same gated cleanup flow tasks already use (GHCLN-03).
**Plans**: 3 plans (2 waves — backend foundations → {reaper pass, frontend + human-verify})
- [ ] 13-01-PLAN.md — backend foundations: PR `state` through PRDetail/ViewPR + cheap github.Service.PRState; byte-equivalent `cleanupWorktreeGated` extraction (regression-guarded); worktree UnpushedCount/StashCount gate primitives; `state` on the detail wire
- [ ] 13-02-PLAN.md — reaper `reconcilePRsOnce` pass (PRStateGetter seam) + conservative gate (dirty/unpushed/stash/session) + D-07 FK-ordered row delete + main.go wiring + spy test matrix
- [ ] 13-03-PLAN.md — frontend: PR-review `⋯` "Clean up worktree" menu + merged/closed banner (D-08/D-09) + end-to-end human-verify checkpoint
**Research flag**: yes — carried forward from research SUMMARY. The gated-cleanup-helper extraction refactors live cleanup code (`cleanupWorktreeGated` extracted from `worktreeHandlers.remove`; one path, two callers — HTTP + reaper), and the unpushed-work gate beyond `git status --porcelain` (also check `git rev-list <headRefOid>..HEAD` and `git stash list`) deserves careful, regression-guarded planning. RESEARCH complete (13-RESEARCH.md): all gates verified live; the helper extraction is taken as a byte-equivalent mechanical refactor with the two new gates in the reaper only (lowest-regression reading).

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
| 10. GitHub Foundations | v1.3 | 4/5 | Complete    | 2026-06-13 |
| 11. PR Review Column | v1.3 | 4/4 | Complete    | 2026-06-14 |
| 12. Open-a-Review | v1.3 | 7/7 | Complete    | 2026-06-14 |
| 13. PR Worktree Auto-Cleanup | v1.3 | 0/3 | Planned | - |

---
*v1.0 shipped 2026-06-11 — 5 phases, 28 plans, 74 tasks*
*v1.1 shipped 2026-06-11 — 1 phase, 4 plans, 10 tasks*
*v1.2 shipped 2026-06-13 — 3 phases, 12 plans, 29 tasks*
*v1.3 roadmapped 2026-06-13 — 4 phases (10–13), 20 requirements mapped, granularity: coarse*
