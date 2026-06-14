# Phase 12: Open-a-Review - Context

**Gathered:** 2026-06-14
**Status:** Ready for planning

<domain>
## Phase Boundary

Clicking a PR card (Phase 11's Review column) opens that PR as a **task-like review workspace**: a git worktree with the PR's head checked out, reusing the existing task view wholesale — the same Agent, Bash, and Diff tabs (`TaskPage.tsx` / `TaskTabs`). A PR review is persisted as a `tasks` row with `source='github_pr'` (the migration-00007 columns `source` / `pr_number` / `pr_base_ref` already exist from Phase 10).

This delivers GHREV-01..05: open-as-worktree (GHREV-01), reattach-on-reopen (GHREV-02), PR-base-correct diff (GHREV-03), never-on-the-board (GHREV-04), and primary-checkout safety for fork/same-name PRs (GHREV-05). It is the milestone's headline **and its highest-risk regression surface** (the board-leak `source` filter and the worktree checkout mechanics).

**Out of scope (later phases):** auto-cleanup when a PR merges/closes and gated manual cleanup of a PR worktree (GHCLN-01/02/03 → Phase 13). Any in-app GitHub writes (reviews/comments/approvals) — v1.3 is read + local-review only; the user comments in the terminal.

</domain>

<decisions>
## Implementation Decisions

### Opening a review (GHREV-01) — locked by research, confirmed here
- **D-01 (worktree mechanic, locked by research PITFALLS §1/§3/§5):** Open via git, never `gh pr checkout`. Resolve PR metadata with `gh pr view <n> --json headRefName,headRefOid,baseRefName,baseRefOid,isCrossRepository,title,body,author,url`, then `git fetch <project-remote> "pull/<n>/head"` and `git worktree add --detach <wt-path> FETCH_HEAD` (or `headRefOid`). **Detached HEAD, no named branch** — this is what guarantees GHREV-05 (fork PRs and PRs whose branch name collides with a local branch never move the primary checkout's HEAD). The checkout verb lives in `internal/worktree` as a new `CheckoutPR`-style function, NOT in `internal/github` (which stays a read-only `gh` query surface). `internal/github` is NOT used for the git mechanics.
- **D-02 (open affordance):** Clicking the **PR card body** opens the review (the Phase 11 PRCard body was deliberately left inert — "whole-card click reserved for Phase 12"). The ↗ external-link stays the GitHub link (Phase 11 D-09), with `stopPropagation` so it does not also open the review. The checks dot stays non-interactive.
- **D-03 (reattach, GHREV-02):** Reopening a PR that already has a review workspace reattaches to the existing `source='github_pr'` row instead of creating a duplicate. Identity key = `project_id` + `pr_number` (look up an existing PR task before provisioning a new worktree; if found, route to it).

### Default tab on open
- **D-04:** A PR review opens on the **Agent tab**, same as tasks (carry the task default D-39 forward — NO source-based override). The user chose consistency over diff-first. The Diff tab is still available immediately (the worktree exists from the moment the review opens, so the Diff tab is enabled, not the disabled/no-worktree state).

### Agent session in a review
- **D-05:** **Explicit Start**, same as tasks (Phase 4 / `AgentTab` Start banner) — opening a review does NOT auto-spawn `claude`. No wasted runs; least surprise.
- **D-06 (review-seeded):** When the user clicks Start for a PR review, the session spawns and the **seed prompt is prefilled into the input, NOT auto-sent** — the user reviews/edits it and presses Enter to send. (Distinct from a normal task Start, which spawns a blank session.)
- **D-07 (seed text):** The seed is a **fixed Kangent template referencing the PR**, e.g. `Review PR #<n> "<title>". Summarize the changes, then flag bugs, risky changes, and missing tests.` Exact wording is the planner's discretion, but it MUST interpolate the PR number and title and stay a short, single instruction. Not configurable in this phase.

### Review header & identity
- **D-08:** **Read-only PR identity** (not the editable task title). The header shows the **PR title, non-editable** (it mirrors GitHub and must not drift). Reuse the `TaskPage` header shell (back arrow → board, quota indicator) but replace the editable-title button with a static title for `source='github_pr'`.
- **D-09 (PR meta line):** Replace/augment `WorktreeMetaLine` for reviews with a PR meta line surfacing **`#<number>` · `@<author>` · base branch · ↗ open-on-GitHub**. (The worktree path may still be shown as today; planner's discretion on whether to keep the worktree meta alongside the PR meta.)
- **D-10 (⋯ menu):** For a PR review the ⋯ menu has **no "Delete task"** (a PR review is not a board task and is not "deleted"). For Phase 12 there is no applicable cleanup action yet (manual cleanup = GHCLN-03 = Phase 13), so the ⋯ menu is effectively empty → **omit the ⋯ menu for PR reviews in this phase**; Phase 13 adds "Clean up worktree" back. (Planner's discretion if a single "Open on GitHub" item is preferred over omitting, but ↗ already lives in the meta line.)

### Description tab content
- **D-11:** For a PR review the Description tab renders the **PR body from GitHub (the `body` field), read-only, as markdown** — not the editable task description. It is review context and must not drift from GitHub. If the PR body is empty, show a quiet "No description." empty state (planner's discretion on exact copy).

### Diff (GHREV-03) — locked by research
- **D-12 (diff base, locked by research ARCHITECTURE §4 / PITFALLS §6):** The review's diff is computed against the **PR's own base**, not the project's configured base. Store `pr_base_ref` (plain branch name) on the PR task; at diff time fetch the base ref and diff against `git merge-base <baseRefOid|origin/base> HEAD` (three-dot semantics), reusing `internal/diff` parameterized by the PR base — do NOT fork the diff renderer, and do NOT reuse the project-base merge-base. Re-resolve the base on refresh (a PR can be retargeted). Acceptance: Kangent's review diff matches GitHub's Files-changed for the PR (including a PR targeting a non-default base).

### Board-leak guard (GHREV-04) — locked by research, highest-risk change
- **D-13 (locked by research ARCHITECTURE board-source-filter):** PR reviews must NEVER appear as kanban cards. Add `WHERE source='manual'` to the board list query (`internal/api/tasks.go` list-by-project) **and audit EVERY `tasks` SELECT** to ensure PR rows can't leak into board/positioning/agent-status surfaces. Add a **defense-in-depth guard**: reject `/move` (and any board-position mutation) for `source != 'manual'` with a 409. This audit is the single highest-risk regression in the milestone — treat it as a first-class task with explicit per-query review, not an afterthought.

### Claude's Discretion
- PR worktree on-disk location/naming (reuse the existing `~/.kangent/worktrees/` dir with a PR-distinct name, e.g. `pr-<n>` or `<project>-pr-<n>`) — research says same dir, detached; exact naming is the planner's call.
- The route shape for a review (reuse `/projects/{id}/tasks/{taskId}` since it IS a task row, vs a distinct `/.../reviews/{prNumber}` path) — planner's discretion; reattach (D-03) is by project+pr_number regardless.
- Whether the worktree meta line is shown alongside the PR meta line (D-09).
- Exact seed wording (D-07), empty-PR-body copy (D-11), and whether to keep a single "Open on GitHub" ⋯ item (D-10).
- Loading/spinner UX while the worktree is being provisioned on first open (fetch + worktree add can take a moment).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Milestone research (v1.3) — the risk center for this phase
- `.planning/research/PITFALLS.md` — **Pitfall 1** (`gh pr checkout` mutates the current checkout — never use it), **Pitfall 3** (named-branch worktree lock → use `--detach`), **Pitfall 5** (fork / same-name fast-forward, cli/cli#8383 → fetch `refs/pull/<n>/head`), **Pitfall 6** (PR diff base ≠ project merge-base). Includes the verification checklist and the two success-criteria tests (main-checkout-HEAD-unchanged; diff matches GitHub Files-changed).
- `.planning/research/ARCHITECTURE.md` — §4 worktree-for-PR + correct diff base (`worktree.CheckoutPR`, `refs/pull/n/head`, reuse `diff.Compute`/`ResolveBase` parameterized by `pr_base_ref`); the board `WHERE source='manual'` filter + `/move` guard + audit-all-selects; build order "Phase C — Open-a-review" with dependencies.
- `.planning/research/STACK.md` — exact `gh pr view --json` field set and `git fetch`/`git worktree add --detach` invocations.

### Schema & prior-phase decisions
- `.planning/phases/10-github-foundations/10-CONTEXT.md` — **D-12** (migration 00007 already landed `tasks.source` CHECK `('manual','github_pr')`, `tasks.pr_number`, `tasks.pr_base_ref` — no new migration needed this phase).
- `.planning/phases/11-pr-review-column/11-CONTEXT.md` — PR card (D-08 inert body now becomes the open trigger, D-09 ↗ link) and the `PRSummary` shape the card carries.

### Requirements & roadmap
- `.planning/REQUIREMENTS.md` — GHREV-01..05 (this phase), GHCLN-01/02/03 (Phase 13, out of scope).
- `.planning/ROADMAP.md` § "Phase 12: Open-a-Review" — goal + 5 success criteria (incl. the checkout-safety and diff-matches-GitHub tests).

### Code to reuse (read before implementing)
- `web/src/pages/TaskPage.tsx` — the task view shell to branch on `source='github_pr'` (header/title, default tab D-39, ⋯ menu, meta line, tabs assembly).
- `web/src/components/task/TaskTabs.tsx` — the `TabDef[]` tab strip (the architectural seam — add nothing new, just feed PR-shaped tabs).
- `web/src/components/task/DescriptionTab.tsx` / `DiffTab.tsx` / `AgentTab.tsx` — the three tabs to parameterize (read-only PR body, PR-base diff, seeded Start).
- `web/src/components/board/PRCard.tsx` / `ReviewColumn.tsx` — wire the card-body open affordance (D-02).
- `internal/worktree/worktree.go` — where `CheckoutPR` belongs.
- `internal/diff/diff.go` + `internal/api/diffs.go` — the diff renderer to parameterize by PR base.
- `internal/api/tasks.go` — the board list query + every `tasks` SELECT to audit (D-13); `internal/store/store_test.go`, `internal/api/agents.go`, `internal/api/sessions.go`, `internal/api/diffs.go`, `internal/api/worktrees.go`, `internal/reaper/reaper.go` are the other `tasks`-SELECT sites to review.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `TaskPage.tsx` + `TaskTabs.tsx` ARE the review view — a PR review is a `task` row with `source='github_pr'`, so the whole Agent/Bash/Diff/header machinery is reused, branched on `source`.
- `internal/worktree` already shells out to git with `--porcelain`/`-C <dir>` and never touches the user's checkout — `CheckoutPR` extends this exact philosophy (add `--detach` + `refs/pull/<n>/head`).
- `internal/diff` (`diff.Compute`, `ResolveBase`'s local-then-remote chain) already does `merge-base base HEAD` three-dot — parameterize the base with `pr_base_ref` instead of forking it.
- Migration 00007's `tasks.source/pr_number/pr_base_ref` columns already exist (Phase 10 D-12) — no schema migration this phase.

### Established Patterns
- Shell-out: arg-array `exec.Command`, never `sh -c`; git/`gh` parsed as machine output.
- Explicit Start for agent sessions (Phase 4); Esc/back returns to the board (`TaskPage` D-07); server owns sessions, browser is a view.
- Worktrees are never auto-removed for manual tasks (Done-TTL reaper, D-87) — the PR cleanup pass (Phase 13) will gate on `source='github_pr'`.

### Integration Points
- PRCard body click (Phase 11) → open/reattach review (new endpoint that provisions-or-finds the PR task + worktree).
- New `internal/worktree` `CheckoutPR` verb (fetch `refs/pull/<n>/head` + `worktree add --detach`).
- Board list query + `/move` guard + all-`tasks`-SELECT audit (`internal/api/tasks.go` and siblings).
- `DiffTab`/`internal/diff` parameterized by the PR's base ref.
- `DescriptionTab` renders read-only PR body for `source='github_pr'`.

</code_context>

<specifics>
## Specific Ideas

- The review view should feel like opening a task — same tabs, same shell — but be unmistakably a PR review: read-only PR title + a `#num · @author · base · ↗ GitHub` meta line.
- Opening a review lands on the Agent tab (consistency), and Start prefills (but does not auto-send) a canned `Review PR #<n> "<title>"…` prompt so the user can tweak before sending.
- The Description tab is the PR's GitHub body, read-only — review context, never an editable scratchpad.
- Safety first: the detached `refs/pull/<n>/head` worktree and the `source='manual'` board-query audit are the two non-negotiables — a fork PR must never move the main checkout's HEAD, and a PR must never show up as a kanban card.

</specifics>

<deferred>
## Deferred Ideas

- **Auto-cleanup of PR worktrees** when the PR merges/closes, and **gated manual cleanup** from the review view — GHCLN-01/02/03 → Phase 13 (explicitly out of scope here).
- **In-app GitHub writes** (submitting reviews/comments/approvals from Kangent) — not in v1.3; the user reviews in the terminal.
- **Configurable / richer review-seed prompts** (e.g., per-project templates, base-aware seeds) — Phase 12 ships one fixed template; a settings-driven seed could be a later polish.

None of the discussion was scope creep — all areas clarified HOW to implement the fixed Phase 12 scope.

</deferred>

---

*Phase: 12-open-a-review*
*Context gathered: 2026-06-14*
