# Requirements: Kangent — v1.3 GitHub PR Review

**Defined:** 2026-06-13
**Core Value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.

## v1.3 Requirements

Requirements for the GitHub PR Review milestone. Each maps to exactly one roadmap phase.

### GitHub Integration Settings

- [ ] **GHSET-01**: User can turn GitHub integration on/off from the global settings page; it is ON by default.
- [ ] **GHSET-02**: When GitHub integration is OFF, all GitHub UI disappears (no per-project GitHub config, no Review column) and the app behaves exactly as it did before v1.3.
- [ ] **GHSET-03**: GitHub features are best-effort — a missing or unauthenticated `gh` CLI, or any GitHub failure, never breaks the board or any existing feature (degrade, don't break). Access is via the host's already-authenticated `gh`; no credentials are stored.

### Project GitHub Config

- [ ] **GHPRJ-01**: User can open a per-project config section to view and edit that project's settings.
- [ ] **GHPRJ-02**: User can set, edit, and clear an optional short description on a project.
- [ ] **GHPRJ-03**: User can link a project to a GitHub repository (owner/name), and change or clear the link; the link is validated (via `gh` when available, syntactically otherwise).

### PR Review Column

- [ ] **GHCOL-01**: When a project is linked to a GitHub repo (and integration is on), a collapsible "Review" column appears on the right side of that project's kanban board.
- [ ] **GHCOL-02**: The Review column lists the open PRs in the linked repo where review is requested directly from the user (`user-review-requested:@me`), excluding drafts.
- [ ] **GHCOL-03**: Each PR is shown as a card resembling a task card, with its number, title, author, relative "updated X ago" time, and a CI/checks status indicator (pass/fail/pending).
- [ ] **GHCOL-04**: The column auto-refreshes on an interval (paused when the browser tab is hidden) and offers a manual refresh; the list mirrors current GitHub state, so cards appear and disappear as review state changes (e.g. the card leaves once the user submits a review).
- [ ] **GHCOL-05**: The column surfaces clear loading, empty ("you're all caught up"), and degraded (`gh` missing / unauthenticated / error) states inline, without blocking the board.
- [ ] **GHCOL-06**: User can collapse and expand the Review column; the collapsed column shows a PR count and the collapse state is remembered.

### PR Review Workspace

- [ ] **GHREV-01**: Clicking a PR card opens a task-like review view backed by a worktree that has the PR's branch checked out (by fetching the PR head ref, not creating a fresh task branch), reusing the existing agent, bash, and diff tabs.
- [ ] **GHREV-02**: Opening the same PR again reattaches to its existing review workspace instead of creating a duplicate.
- [ ] **GHREV-03**: The review view's diff is computed against the PR's own base branch merge-base, matching the changes GitHub shows for that PR.
- [ ] **GHREV-04**: PR review workspaces never appear as cards on the kanban board — they are not To Do / In Progress / In Review / Done tasks.
- [ ] **GHREV-05**: Fork PRs, and PRs whose branch name collides with an existing branch, open correctly without disturbing the project's primary checkout (HEAD of the main checkout is unchanged).

### PR Worktree Cleanup

- [ ] **GHCLN-01**: When a PR is merged or closed on GitHub, its review worktree is removed automatically.
- [ ] **GHCLN-02**: Auto-removal is gated — a dirty worktree, uncommitted/unpushed work, or running sessions block silent removal; the branch is always kept (the user may not own it).
- [ ] **GHCLN-03**: User can also clean up a PR review worktree manually from the review view, using the same gated cleanup flow tasks already use.

## Future Requirements

Acknowledged but deferred — not in the v1.3 roadmap.

### Richer PR Cards

- **GHCARD-01**: Show diff size (`+adds −dels`) on PR cards.
- **GHCARD-02**: Show a fork indicator pill on PR cards.
- **GHCARD-03**: Show the head → base branch line on PR cards.
- **GHCARD-04**: Show review-decision, labels, and comment-count on PR cards.

### Filter Options

- **GHFILT-01**: Option to also include team review requests (`review-requested:@me`).
- **GHFILT-02**: Option to show draft PRs.

### Broader Surfaces

- **GHWIDE-01**: Cross-project "all repos" aggregate review inbox.
- **GHWIDE-02**: Author-side view of PRs the user opened.

## Out of Scope

Explicitly excluded from v1.3. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| In-app GitHub write actions (approve / request-changes / comment / merge) | Preserves Kangent's manual-git philosophy; review here, act in the terminal with `gh` |
| Storing GitHub credentials / personal access tokens | Access is via the host's already-authenticated `gh` CLI only; the app never holds a credential |
| Non-GitHub forges (GitLab, Bitbucket, Gitea) | GitHub-only for v1.3; the `gh`-based design doesn't generalize without rework |
| Reimplementing GitHub's diff/review UI | Reuse the existing local Diff tab + a real checked-out worktree; no custom GitHub-style review UI |
| Auto-deleting the PR branch on cleanup | The user may not own the branch; the app keeps branches by long-standing decision (D-34) |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| GHSET-01 | — | Pending |
| GHSET-02 | — | Pending |
| GHSET-03 | — | Pending |
| GHPRJ-01 | — | Pending |
| GHPRJ-02 | — | Pending |
| GHPRJ-03 | — | Pending |
| GHCOL-01 | — | Pending |
| GHCOL-02 | — | Pending |
| GHCOL-03 | — | Pending |
| GHCOL-04 | — | Pending |
| GHCOL-05 | — | Pending |
| GHCOL-06 | — | Pending |
| GHREV-01 | — | Pending |
| GHREV-02 | — | Pending |
| GHREV-03 | — | Pending |
| GHREV-04 | — | Pending |
| GHREV-05 | — | Pending |
| GHCLN-01 | — | Pending |
| GHCLN-02 | — | Pending |
| GHCLN-03 | — | Pending |

**Coverage:**
- v1.3 requirements: 20 total
- Mapped to phases: 0 (set during roadmap)
- Unmapped: 20 ⚠️ (resolved by roadmap)

---
*Requirements defined: 2026-06-13*
*Last updated: 2026-06-13 after initial definition*
