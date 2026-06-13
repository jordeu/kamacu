# Feature Research

**Domain:** GitHub "PRs that need my review" surface added to a local kanban + agent app (Kangent v1.3)
**Researched:** 2026-06-13
**Confidence:** HIGH on `gh` CLI capabilities and field availability (verified against cli.github.com manuals + cli/cli issues); MEDIUM on comparable-tool UX patterns (gh-dash, Graphite, vibe-kanban docs); MEDIUM-HIGH on fork-PR + worktree mechanics (verified against cli/cli#972 and #3231).

> Scope note: v1.3 adds a *read-only PR-review surface* to an already-built app. Everything below is scoped to the NEW surface. Existing Kangent capabilities (task view, worktree service, server-owned PTY sessions, diff tab, auto-poll-when-visible pattern, gated cleanup) are treated as **dependencies to reuse**, never re-proposed.

---

## How the "PRs that need my review" workflow actually works

The settled mental model for this milestone matches the dominant industry pattern (Graphite PR Inbox, GitHub's own "Review requested" filter, gh-dash's "Needs My Review" section):

1. **A queue, not a board.** Reviewers want a *filtered list* of "open PRs where my review is requested," ordered by recency/staleness. It is explicitly **not** a kanban they drag through — it mirrors live GitHub state. Kangent's settled decision (sync'd column, PRs never enter To Do/Done) is exactly right and matches Graphite ("Needs your review" section is read-from-GitHub) and gh-dash (`is:open review-requested:@me` section).
2. **Triage → open → review locally.** The list answers "what needs me?"; clicking one answers "what changed and is it OK?" Kangent's differentiator is the third step most tools lack: **drop the PR into a real local worktree with an agent and a terminal**, so review is hands-on (run it, read it, ask the agent), not just read-a-web-diff.
3. **Cards vanish as state changes.** Once you approve/the author re-requests/the PR merges, it leaves your queue. This appear/disappear-on-sync behavior is core to the workflow's value and must feel calm, not jarring.

**Key CLI grounding that shapes the whole feature** (verified):
- The natural global query is `gh search prs --review-requested=@me --state=open`, but `gh search prs --json` exposes only a **thin** field set: `assignees, author, authorAssociation, body, closedAt, commentsCount, createdAt, id, isDraft, isLocked, isPullRequest, labels, number, repository, state, title, updatedAt, url`. It has **no `reviewDecision`, no `statusCheckRollup`, no `headRefName`/`baseRefName`, no additions/deletions** (cli/cli#13239).
- The **rich** fields (`reviewDecision`, `statusCheckRollup`, `additions`, `deletions`, `headRefName`, `baseRefName`, `isCrossRepository`, `reviewRequests`, `comments`, `labels`, `updatedAt`) come from `gh pr list --json …` / `gh pr view`, which are **repo-scoped**.
- **This resolves cleanly for Kangent because the column is per-linked-project (one repo).** Use `gh pr list --repo OWNER/REPO --search "review-requested:@me" --state open --json <rich fields>`. One repo-scoped call gets the review-requested filter *and* the rich card fields in a single command — no cross-repo aggregation, no thin-field problem. This is the single most important implementation fact for the requirements author.

---

## Feature Landscape

### Table Stakes (Users Expect These)

Features the review surface must have to not feel broken.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| **PR card: number + title** | The minimum identity of a PR; every comparable tool leads with `#123 Title`. | LOW | From `number`, `title`. Title likely truncated/clamped to 2 lines like a task card. |
| **PR card: author (login + avatar)** | "Who is asking me" is the first triage signal; gh-dash and GitHub both show author prominently. | LOW–MEDIUM | `author.login` is free; avatar = one `<img>` to `avatars.githubusercontent.com/u/…` or `author.avatarUrl`. Avatar is a small polish lift, no GitHub write. |
| **PR card: draft badge** | Draft PRs usually shouldn't be in a "needs my review" queue at all; if shown, must be visually distinct. | LOW | `isDraft`. Recommend **filtering drafts out by default** (GitHub rarely requests review on drafts); if shown, dim + "Draft" pill. |
| **PR card: relative updatedAt** | Staleness is the #1 ordering signal in every reviewer-queue tool (Graphite, gh-dash sort by updated). | LOW | `updatedAt` → "2h ago". Reuse whatever relative-time helper the quota "Updated Xs ago" footer already uses. |
| **List filter = review-requested-from-me, open only** | This *is* the feature. Anything else is a different list. | LOW | `gh pr list --repo … --search "review-requested:@me" --state open`. Note: GitHub drops a PR from `review-requested:@me` once you submit a review, which gives the desired auto-disappear for free. |
| **Auto-poll, paused when tab hidden + manual refresh** | Users expect the queue to be live but not to hammer the API in a background tab. Kangent already established this exact contract with the quota indicator. | LOW (reuse) | Reuse the quota auto-poll pattern (visibility-gated interval + cache-bypassing manual refresh). Poll interval should be slower than quota (PR state changes on the order of minutes, not seconds) — 60–120s is plenty. |
| **Empty state: "No PRs need your review"** | A queue that's empty is the *success* state; it must read as "you're caught up," not "something's broken." | LOW | Distinct copy from the error/degraded states below. |
| **Loading state (first fetch)** | First paint before `gh` returns; skeleton or spinner so the column isn't a flash of "empty = caught up." | LOW | Distinguish "loading" from "empty" so the user never misreads in-flight as done. |
| **Degraded state: `gh` missing / unauthenticated** | `gh` is a soft dependency (settled). The column must degrade gracefully exactly like the quota indicator does, not error the board. | MEDIUM | Detect `gh` not on PATH vs `gh auth status` failing vs repo not found / no GitHub remote. Show an inline, non-blocking notice ("GitHub CLI not found" / "Run `gh auth login`") — never a modal, never block the kanban. |
| **Collapse/expand the column, persisted** | A right-side column that can't be collapsed steals board width permanently; persistence is expected so the choice sticks across reloads. | LOW | Persist per-project (or global) in SQLite/localStorage. Default expanded for linked projects. |
| **Click PR → task-like review view (worktree on PR branch)** | The headline of the milestone; without it this is just a read-only list. Users expect "open it" to mean "I can work on it locally." | HIGH | Depends on worktree service + `gh pr checkout` semantics — see Review-View section and Dependencies. |
| **Review view reuses Agent / bash / Diff tabs** | Once open, a PR should behave like a task (settled). Users expect parity, not a stripped-down view. | MEDIUM (reuse) | Reuse the existing tab strip + session manager + diff tab. New work is plumbing the worktree + correct diff base, not new tab UI. |
| **Diff tab base = PR's base branch merge-base (not project default)** | A PR diff against the wrong base is *wrong data* — silently misleading during review. | MEDIUM | Existing diff tab computes "vs base branch merge-base." For a PR it must use the **PR's `baseRefName`** (e.g. `develop`), not the project's default branch. This is a real behavioral change to the diff tab's base resolution, not just config. |
| **Auto-remove review worktree on PR merge/close (gated)** | Settled. Reviewers expect closed PRs to clean up after themselves; leaving stale worktrees is the failure mode vibe-kanban explicitly built cleanup for. | MEDIUM | Reuse existing gated-cleanup (dirty-tree + running-sessions gates, branch kept). Trigger = poll observes the PR's `state` became `MERGED`/`CLOSED`. |

### Differentiators (Competitive Advantage)

Features that make Kangent's PR review better than a web tab or a TUI — align with Core Value ("one place to drive all agent work").

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **Agent pre-pointed at the PR diff in a real checkout** | The whole reason to review *in Kangent*: ask `claude` "summarize this PR / does this introduce a regression?" against a live worktree, not a pasted diff. No comparable list-tool (gh-dash, Graphite inbox) does this. | LOW (reuse) | Falls out of "worktree on PR branch + existing agent session." The differentiator is mostly free once the worktree is correct. |
| **CI / checks rollup status on the card** | Lets you skip PRs whose CI is red/pending before opening them — a real triage accelerator. GitHub and gh-dash both surface this. | MEDIUM | `statusCheckRollup` from `gh pr list --json`. Reduce to a single dot/pill: success / failure / pending / none. Rolling up the array correctly (any failure → fail; any pending → pending; all pass → pass) is the work; the data is one field. |
| **Review-decision pill (approved / changes-requested / review-required)** | Shows whether *others* have already weighed in — useful context, and lets the card distinguish "fresh" from "already-approved-by-others." | MEDIUM | `reviewDecision`. Note: for the `review-requested:@me` list the PR is by definition still wanting review, so the most useful states are "REVIEW_REQUIRED" (default) vs "CHANGES_REQUESTED" (others already pushed back). Modest value; treat as P2. |
| **head→base branch line on the card** | Disambiguates PRs targeting non-default bases (e.g. `feature/x → develop`) and is needed anyway to compute the diff base. | LOW | `headRefName`, `baseRefName`. Cheap to show; doubles as the data the diff base needs. Show compactly, possibly only when base ≠ default branch. |
| **Additions/deletions (+/−) size hint** | "Is this a 5-line typo fix or a 2,000-line refactor" is a strong triage/ordering signal; reviewers batch by size. | LOW | `additions`, `deletions`. Render as `+120 −30` in green/red. Pure display. |
| **Fork PR indicator** | A heads-up that this PR comes from a fork (different checkout mechanics, can't push back trivially, security caution on running untrusted code). | LOW | `isCrossRepository` / `headRepositoryOwner`. A small "fork" pill. Pairs with the fork checkout handling below. |
| **Labels on the card** | Quick context (`bug`, `security`, `dependencies`); reviewers use labels to prioritize. | LOW | `labels[].name` (+ color). Cap to N labels with "+k more" to avoid card bloat. Differentiator, not table-stakes for a single-user tool. |
| **Comments count** | Signals discussion volume / contentiousness before opening. | LOW | `comments` count. Low value alone; fine as a tiny icon+count. P3. |
| **Stale/needs-attention sort or subtle aging cue** | Surfaces the PR that's been waiting longest on *you* — the reviewer-queue superpower (Graphite leans on this). | LOW–MEDIUM | Sort by `updatedAt` asc/desc; optionally a faint "waiting N days" tint. Cheap, high triage value. |

### Anti-Features (Commonly Requested, Often Problematic)

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| **In-app approve / request-changes / comment / merge** | "I'm already looking at the PR, let me act." | Explicitly out of scope (settled): breaks the manual-git philosophy, needs write scopes, and reimplements GitHub review UI poorly. Review comments especially are a deep feature (line anchors, suggestions, threads). | User acts in the terminal — `gh pr review`, `gh pr comment`, `gh pr merge` — in the worktree's bash/agent tab. Kangent already gives them a terminal in the right directory. |
| **Drag a PR into a kanban column / convert PR ⇄ task** | "It's on a board, let me move it." | PRs mirror live GitHub state; making them mutable kanban items creates a dual source of truth and sync conflicts (which wins, GitHub or the board?). Settled: PRs never enter columns. | Keep the PR list strictly read-synced and visually distinct (separate collapsible column on the right, not a draggable column). |
| **Aggregate "all repos" review inbox** | "Show me every PR across all my projects needing review" (Graphite/gh-dash do this globally). | Cross-repo means `gh search prs`, which **loses the rich fields** (no reviewDecision/checks/branches/diffstat — cli/cli#13239) and is on the heavily rate-limited search API. It also breaks the per-project model and the worktree story (which repo's worktree?). | Per-linked-project column using `gh pr list --repo … --search review-requested:@me` (rich fields, per-repo rate limit). Aggregation is a possible far-future feature, not v1.3. |
| **Render the PR diff in a custom in-app GitHub-style review UI** | "Make it look like github.com." | Reimplements GitHub's diff/review surface; huge surface area; Kangent already has a perfectly good local diff tab against the real checkout. | Reuse the existing Diff tab (pointed at the PR base merge-base) + the agent. The local checkout *is* the review surface. |
| **Store a GitHub token / PAT in the app** | "Faster API calls / works without `gh`." | Explicitly out of scope (settled): credential storage is a security liability and a maintenance burden; mirrors the quota indicator's "never write credentials" stance. | Shell out to the host's already-authenticated `gh`. Degrade gracefully when absent. |
| **Auto-delete the PR branch on merge** | "Clean up everything." | Settled rule is *branch always kept*; deleting branches is irreversible and can surprise the user mid-review. | Remove the *worktree* (gated), keep the branch — consistent with task cleanup. |
| **Real-time push of PR changes (webhooks/WS to GitHub)** | "Instant updates." | Requires a public endpoint / webhook plumbing — impossible for a localhost-only app and overkill for single-user. | Polling (settled), visibility-gated, like the quota indicator. Minutes-fresh is fine for review triage. |
| **Auto-start the agent when a PR view opens** | "Save me a click." | Conflicts with the established "explicit Start button, no accidental agent runs" decision — and running an agent unprompted on **fork** code (untrusted) is a security smell. | Keep the explicit Start button. The worktree is checked out; starting the agent stays a deliberate action. |
| **Auto-create a worktree for every PR in the list on sync** | "Have them ready." | Would spawn dozens of worktrees/checkouts for PRs you may never open; expensive `gh pr checkout`/fetch per card; cleanup nightmare. | Create the worktree lazily — only when the user clicks the PR (open = checkout), mirroring how a task's worktree is created. |

---

## Fork-PR behavior & diff-base semantics (called out for the requirements author)

**Diff base (must-get-right):** A PR's diff is `head` vs the **merge-base of `head` and the PR's `baseRefName`**, where `baseRefName` is the branch the PR targets on the *upstream* repo (often `main`, sometimes `develop`/a release branch). Kangent's existing diff tab computes "vs base merge-base" using the project's default branch; for a PR it must substitute the PR's `baseRefName`. Getting this wrong silently shows the wrong changeset.

**Checkout mechanics — and a real gotcha:** `gh pr checkout <n>` handles fork vs same-repo uniformly: it fetches the PR head ref and creates/updates a local tracking branch (`--branch` to rename, `--detach` for detached HEAD, `--force` to reset, `-R/--repo` to target a repo). **However, `gh pr checkout` is not worktree-aware and has known failures when run from inside a linked worktree, especially for branch names containing `/`** (cli/cli#972 is an open request to add worktree support; cli/cli#3231 documents `gh pr checkout` failing for `/`-containing PR branch names inside a worktree — git can't resolve `origin/user/branch` as a tracking ref in a fresh detached worktree). Implications:
- **Don't assume `gh pr checkout` "just works" in the worktree Kangent creates.** Plan for the manual-git path Kangent already prefers (CLAUDE.md: "shell out, parse `--porcelain`"): `git fetch origin pull/<n>/head:<local-ref>` (works for forks too, since `pull/<n>/head` is on the *base* repo's refs) then `git worktree add <path> <local-ref>`. This sidesteps the worktree-unaware `gh pr checkout` entirely and is robust to fork PRs and `/`-in-branch-name.
- Either way, **flag this as the highest-risk integration point of the milestone** — it deserves a focused feasibility/spike in the relevant phase (the requirements author should mark the checkout phase "needs deeper research").

**Fork specifics to handle:**
- A fork PR's head lives in another owner's repo; `pull/<n>/head` on the base repo is the reliable, auth-free-of-fork-remote way to fetch it. Surface a **fork pill** (`isCrossRepository`) so the user knows.
- Pushing back to a fork PR branch is not generally possible from Kangent's checkout (it's the contributor's branch; `maintainerCanModify` governs upstream pushes) — fine, since v1.3 does **no writes**. The worktree is for *reading/running/asking-the-agent*, not pushing.
- Security: fork PR code is untrusted. This reinforces the anti-feature "don't auto-start the agent" — keep Start explicit, especially for forks.

---

## Column behaviors (states, sync feel)

| State | Expected behavior | Source/Notes |
|-------|-------------------|--------------|
| **Loading (first fetch)** | Skeleton/spinner in the column; never read as "empty/caught up." | LOW |
| **Empty (query returns 0)** | "You're all caught up — no PRs need your review." Calm success copy. | gh-dash/Graphite both treat empty queue as a positive. |
| **Populated** | Cards sorted by `updatedAt` (configurable later); newest-or-stalest first. | Match reviewer-queue norm. |
| **Refreshing (background poll)** | Subtle, non-jarring; keep existing cards, diff in/out changes. Don't blank the column on each poll. | Mirror quota "Updated Xs ago" + manual refresh affordance. |
| **Manual refresh** | A refresh control (icon) that bypasses cache, like the quota popup's refresh. | Reuse pattern. |
| **`gh` missing** | Inline degraded notice: "GitHub CLI (`gh`) not found." No board breakage. | Detect via PATH lookup (mirror the tmux `LookPath` pattern). |
| **`gh` unauthenticated** | Inline: "Run `gh auth login` to see review requests." | `gh auth status` non-zero. |
| **Repo not on GitHub / not linked** | Column simply doesn't render for unlinked projects (GitHub link is per-project, optional). | Settled: column only on linked projects. |
| **Global GitHub toggle off** | All GitHub UI disappears (settled). Column, link config, pills — gone. | Settings flag, default on. |
| **Card appears (new review request)** | Animate in gently; don't steal focus. | Sync feel matters; avoid flashes. |
| **Card disappears (reviewed/merged/closed/re-requested elsewhere)** | Animate out gently. If the PR's review view is currently open, **don't yank it** — keep the worktree/session until gated cleanup runs (merge/close) or the user closes it. | Important interaction with the open review view. |
| **Collapse/expand** | Persisted; collapsing reclaims board width. Show a count badge on the collapsed column ("Review · 3"). | Count-on-collapsed is a small, high-value touch. |

---

## Feature Dependencies

```
PR review view (worktree on PR branch)
    └──requires──> Worktree service (existing, Phase 3)
    └──requires──> gh pr checkout / git fetch pull/N/head + worktree add  [HIGH-RISK, fork+worktree gotcha]
    └──requires──> Server-owned PTY session manager (existing, Phases 2/4/5)
    └──requires──> Diff tab with PR-base resolution (existing diff tab, MODIFIED base)
    └──requires──> Agent/bash/tmux tabs (existing, Phases 4/8/9)

PR review column (synced list)
    └──requires──> Per-project GitHub link config (new this milestone)
    └──requires──> gh availability detection (mirror tmux LookPath + quota degrade pattern)
    └──requires──> Auto-poll-when-visible + manual refresh (existing quota pattern, reused)
    └──enhanced-by──> CI rollup / reviewDecision / diffstat / labels (extra --json fields)

Auto-cleanup of review worktree on merge/close
    └──requires──> Poll observing PR.state == MERGED/CLOSED
    └──requires──> Gated cleanup (existing: dirty-tree + running-sessions gates, branch kept)

Global GitHub toggle (settings)
    └──gates──> everything above (off ⇒ all GitHub UI hidden)

Per-project GitHub link
    └──gates──> the review column for that project

[Cross-repo "all-repos inbox"] ──conflicts──> [rich card fields + per-project worktree model]
    (gh search prs loses reviewDecision/checks/branches — cli/cli#13239)
```

### Dependency Notes

- **Review view requires the worktree-checkout path, which is the milestone's risk center.** `gh pr checkout` is not worktree-aware (cli/cli#972) and fails on `/`-branches inside worktrees (cli/cli#3231); prefer the manual `git fetch origin pull/<n>/head:<ref>` + `git worktree add` path consistent with Kangent's shell-out philosophy. Phase that does this needs a spike.
- **Diff tab base must switch from project-default to PR `baseRefName`.** This is a behavioral modification to an existing feature, not pure reuse — call it out so it isn't underscoped.
- **Card richness depends on staying repo-scoped.** `gh pr list --repo … --search review-requested:@me --json …` gives the filter *and* the rich fields in one call; going cross-repo (`gh search prs`) silently strips them. The per-project column model is what makes the rich card cheap.
- **Auto-cleanup reuses the exact existing gated-cleanup contract** (dirty-tree + running-session gates, branch kept). The only new part is the *trigger* (poll sees MERGED/CLOSED) — and the deliberate, settled exception that worktrees may be auto-removed here.
- **Everything is gated by two new config switches** (global toggle, per-project link) — these are the cheapest, earliest dependencies and should land first.

---

## MVP Definition

### Launch With (v1.3)

- [ ] Global GitHub toggle (default on) + per-project link config (optional description + repo) — gates everything; cheap; land first.
- [ ] Collapsible right-side **Review column** on linked projects, `gh pr list --repo … --search "review-requested:@me" --state open`, auto-poll (visibility-gated) + manual refresh — the core list.
- [ ] **Loading / empty / degraded (`gh` missing/unauth) states** — non-negotiable for a soft-dependency surface.
- [ ] **PR card (table-stakes set):** number + title, author + avatar, relative `updatedAt`, draft handling (filter drafts by default). — minimum recognizable card.
- [ ] **PR card (cheap differentiators, recommend including at launch):** CI/checks rollup pill (`statusCheckRollup`), additions/deletions, fork pill (`isCrossRepository`), head→base when base ≠ default. — all one `--json` field each, high triage value, low cost.
- [ ] **Click PR → task-like review view** with worktree checked out on the PR branch (via the robust fetch+worktree-add path), reusing Agent/bash/Diff tabs.
- [ ] **Diff tab pointed at the PR's `baseRefName` merge-base** (the correctness item).
- [ ] **Auto-remove review worktree on merge/close**, gated (dirty + sessions), branch kept.
- [ ] Collapse/expand persistence + count-on-collapsed badge.

### Add After Validation (v1.x)

- [ ] `reviewDecision` pill (changes-requested vs review-required) — add if users want "has someone already pushed back?" context.
- [ ] Labels + comments-count on cards — add if cards feel information-poor; cap with "+k more."
- [ ] Stale-aging visual cue / configurable sort — add if the queue gets long enough to need prioritization.

### Future Consideration (v2+)

- [ ] Cross-project "all repos needing review" aggregate inbox — only worth it if/when the per-project model feels limiting; accept the thin-field tradeoff or do N per-repo calls.
- [ ] Author-side PRs ("PRs I opened" status) — different list, different value; out of this milestone.
- [ ] Any in-app write actions — currently a hard anti-feature; revisit only if the manual-git philosophy is ever relaxed.

---

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Global toggle + per-project GitHub link | HIGH (gates all) | LOW | P1 |
| Review column (review-requested list, repo-scoped) | HIGH | MEDIUM | P1 |
| Loading / empty / `gh`-degraded states | HIGH | LOW–MEDIUM | P1 |
| Auto-poll (visibility-gated) + manual refresh | HIGH | LOW (reuse quota pattern) | P1 |
| Card: number, title, author+avatar, updatedAt | HIGH | LOW | P1 |
| Click → review view (worktree on PR branch) | HIGH | HIGH (checkout risk) | P1 |
| Diff tab at PR `baseRefName` merge-base | HIGH (correctness) | MEDIUM | P1 |
| Auto-cleanup worktree on merge/close (gated) | MEDIUM–HIGH | MEDIUM (reuse cleanup) | P1 |
| Collapse/expand persistence + count badge | MEDIUM | LOW | P1 |
| Card: CI/checks rollup pill | HIGH (triage) | MEDIUM (rollup logic) | P1–P2 |
| Card: additions/deletions | MEDIUM–HIGH | LOW | P1–P2 |
| Card: fork pill | MEDIUM | LOW | P2 |
| Card: head→base branch line | MEDIUM | LOW | P2 |
| Card: reviewDecision pill | MEDIUM | MEDIUM | P2 |
| Card: labels | MEDIUM | LOW | P2 |
| Card: comments count | LOW | LOW | P3 |
| Stale-aging cue / sort options | MEDIUM | LOW–MEDIUM | P3 |
| In-app review/merge actions | — | — | Anti-feature |
| Cross-repo aggregate inbox | LOW (now) | MEDIUM | Anti-feature (v1.3) |

---

## Competitor Feature Analysis

| Aspect | gh-dash (TUI) | Graphite PR Inbox | vibe-kanban | GitHub "Review requested" filter | Kangent's Approach |
|--------|---------------|-------------------|-------------|----------------------------------|--------------------|
| The list | "Needs My Review" section, filter `is:open review-requested:@me`; configurable columns (width/visibility) | "Needs your review" section, email-client-like, customizable/shareable sections | N/A (its flow is task→PR, polls PR status to flip task state) | A saved search/filter (`review-requested:@me is:open`) | Per-project synced right-side collapsible column, repo-scoped `gh pr list --search review-requested:@me` |
| Card fields | number, title, author, repo, review status, CI, +/− lines, comments, updated (columnar) | author, title, status pills, age, review state | task card maps to PR after creation | number, title, author, labels, checks, review state | number, title, author+avatar, updatedAt + (checks, +/−, fork, base) as cheap richness |
| Open/act | Opens PR on github.com or in browser; no local checkout | In-app GitHub-style review (web) | Opens its own diff/agent view on the task's worktree | github.com web review | **Local worktree + agent + terminal + local diff** (the differentiator) |
| Writes | Some shortcuts (comment/approve via gh) | Full in-app review actions | Creates PRs, can merge | Full web actions | **None** — user acts in terminal (settled) |
| Refresh | Manual + interval | Live | Background PrMonitorService polling | Page reload | Visibility-gated poll + manual refresh (reuse quota pattern) |
| Anti-feature avoided | (TUI: no local checkout) | (Heavy web review UI we deliberately skip) | (Sunsetting; cleanup of orphan/expired worktrees worth copying) | (No local workspace) | Lazy per-click checkout; no cross-repo thin-field list; no in-app writes |

**Patterns worth copying:**
- gh-dash's exact filter (`is:open review-requested:@me`) and columnar card field set — it's the proven minimal-yet-useful set.
- Graphite's "empty = you're caught up" framing and section-based, calm read-only queue.
- vibe-kanban's **PR-status polling that drives lifecycle actions** (it flips task state on merge; Kangent flips to worktree-cleanup on merge/close) and its **orphan/expired worktree cleanup** discipline.

**Anti-features to avoid (seen in the wild):**
- Graphite-style full in-app review/approve/merge UI (out of scope; huge surface).
- Cross-repo aggregation when it costs you the rich fields (the `gh search prs` thin-field trap — cli/cli#13239).
- Any flow that assumes `gh pr checkout` works inside a worktree without verification (cli/cli#972, #3231).

---

## Sources

- [gh-dash PR section docs](https://www.gh-dash.dev/configuration/pr-section/) and [examples](https://www.gh-dash.dev/configuration/examples/) — default sections incl. "Needs My Review" `is:open review-requested:@me`, configurable columns (MEDIUM)
- [gh pr checkout manual](https://cli.github.com/manual/gh_pr_checkout) — fork vs same-repo handling, `--branch/--detach/--force/--recurse-submodules/-R` (HIGH)
- [cli/cli#972 — checkout PR by creating a new worktree](https://github.com/cli/cli/issues/972) — `gh pr checkout` is not worktree-aware; manual `git fetch` + `git worktree add` workaround (HIGH)
- [superset-sh/superset#3231](https://github.com/superset-sh/superset/issues/3231) — `gh pr checkout` fails for `/`-containing PR branch names inside a worktree (MEDIUM-HIGH)
- [cli/cli#13239 — gh search prs missing reviewDecision/mergeStateStatus](https://github.com/cli/cli/issues/13239) — thin field set of `gh search prs` vs `gh pr list` (HIGH)
- [gh search prs manual](https://cli.github.com/manual/gh_search_prs) + [discussion #6801](https://github.com/cli/cli/discussions/6801) — `--review-requested=@me`, available JSON fields, search API rate-limiting (HIGH)
- [gh pr list manual](https://cli.github.com/manual/gh_pr_list) + [discussion #5902](https://github.com/cli/cli/discussions/5902) — full `--json` field list incl. statusCheckRollup, reviewDecision, additions/deletions, head/baseRefName, isCrossRepository (HIGH)
- [Graphite PR inbox / review-requests guides](https://graphite.com/guides/github-review-requests-guide) and [review-pull-requests docs](https://graphite.com/docs/review-pull-requests) — "Needs your review" reviewer-queue UX, section model (MEDIUM)
- [vibe-kanban GitHub integration & PR workflow (DeepWiki)](https://deepwiki.com/BloopAI/vibe-kanban/2.4-github-integration-and-pr-workflow) and [git+GitHub integration](https://deepwiki.com/BloopAI/vibe-kanban/6-git-and-github-integration) — worktree-per-attempt, PrMonitorService polling drives lifecycle, orphan worktree cleanup (MEDIUM)
- [GitHub Changelog: clearer PR reviewer status (2025-08)](https://github.blog/changelog/2025-08-14-clearer-pull-request-reviewer-status-and-enhanced-email-filtering/) — reviewer status / review-decision semantics (MEDIUM)
- [GitHub Code Review feature page](https://github.com/features/code-review) — canonical PR components (title/description/diff/metadata: labels, reviewers, CI status) (MEDIUM)

---
*Feature research for: GitHub "PRs needing my review" surface in a local kanban + agent app*
*Researched: 2026-06-13*
