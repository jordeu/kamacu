# Project Research Summary

**Project:** Kangent — v1.3 GitHub PR Review
**Domain:** `gh`-CLI GitHub PR-review surface (read-only list + PR-branch worktree checkout) added to an existing single-binary Go + React local agent-session app
**Researched:** 2026-06-13
**Confidence:** HIGH

## Executive Summary

v1.3 adds a *read-only* "PRs that need my review" surface to an already-built app, plus the ability to open any such PR as a full task-like review workspace (worktree on the PR branch + agent + bash + diff). All four research tracks converged hard on one shape: **this is overwhelmingly reuse, not new machinery, and it must shell out to the host's already-authenticated `gh` CLI** — no `go-github`, no stored token — exactly the way Kangent already shells out to `git` and `claude`. The entire new backend footprint is one `internal/github` leaf package (modeled on the existing `internal/quota` best-effort proxy), one goose migration adding columns to `projects` and `tasks`, a worktree "checkout a PR" verb, a poll-backed PR-list endpoint pair, and a frontend Review column + project-config UI. Zero new Go modules, zero new npm deps.

The single most important *modeling* decision — settled by all four agents — is that **a PR review is a `tasks` row with a `source` discriminator (`source='github_pr'`), excluded from the board via `WHERE source='manual'`, NOT a separate `pr_reviews` table.** The existing session manager, tmux table, diff/session/worktree handlers, reaper, and the entire task view are *all* keyed on `TaskID int64`; a second id-space would fork every one of them for zero gain. The two most important *correctness* facts: (1) **do not use `gh pr checkout`** — it mutates the current checkout, isn't worktree-aware, and breaks on fork PRs and `/`-branches; instead `git fetch <remote> refs/pull/<n>/head:<ref>` then `git worktree add` (fork-safe via the server-side pull ref); and (2) the **diff base for a PR task must come from the PR's own base branch (`pr_base_ref`), not the project default** — `diff.Compute(wt, base)` already takes base as a parameter, so only the handler branches.

The dominant *risk* is that `gh` failures, or the board-query `source` filter being forgotten on even one `SELECT`, could either break the core app or leak PR reviews onto the kanban. Both are mitigated by hard rules from the research: degrade-don't-break (always-200 endpoints, typed `no_gh`/`auth_required`/`disabled` states copied from quota), and an explicit per-query audit of every `tasks` SELECT during the open-a-review phase. There is exactly **one unresolved product decision that must be made before/during requirements**: the review qualifier — `review-requested:@me` (includes ALL team review requests; one user saw ~100 PRs) vs `user-review-requested:@me` (direct requests only). Recommended default: `user-review-requested:@me`.

## Key Findings

### Recommended Stack

The stack is "nothing new." `gh` (host floor **2.82.0**, already installed and authed on this host) is the only access mechanism — every command, flag, and `--json` field below was verified live, not recalled. List via `gh pr list -R <repo> --search "<qualifier>:@me" --state open --json <rich set>` (rich fields: `number,title,author,headRefName,baseRefName,isDraft,reviewDecision,statusCheckRollup,additions,deletions,updatedAt,url,headRepositoryOwner,headRepository,isCrossRepository,headRefOid`). Do **not** use `gh search prs` — its `--json` set is thin (no `reviewDecision`/`statusCheckRollup`/`headRefName`/diffstat/`headRefOid`) and it burns the **search** rate budget (30/min) vs `gh pr list`'s **core** budget (5000/hr). State via `gh pr view <n> -R <repo> --json state` → UPPERCASE `OPEN`/`CLOSED`/`MERGED` (there is **no top-level `merged` boolean** — requesting it errors; derive merged from `state`).

**Core technologies:**
- **`gh` CLI (host, 2.82.0 floor):** all GitHub reads (list review-requested PRs, single-PR state, repo-link validation, auth detect) — already authed, owns credentials in the OS keyring, mirrors the settled "shell out to real CLIs, store no tokens" philosophy.
- **`os/exec` + system `git` (existing worktree service):** materialize the PR head into a worktree via `git fetch <remote> refs/pull/<n>/head:<ref>` + `git worktree add` — a new variant of the existing worktree-create path, fork-agnostic, never touches the primary checkout.
- **`internal/github` Go package (new, stdlib only):** typed wrapper over `gh` invocations with the quota-style cache/TTL/floor/backoff + degraded-state machine. No go.mod additions.
- **Existing React 19 + TanStack Query + shadcn card/column:** the Review column is one more `useQuery` with the Phase-7 auto-poll-paused-when-hidden pattern; PR cards are presentational variants of the task card. Zero new npm deps.
- **Existing `goose` migrations:** one new migration (00007) adds `projects.description`, `projects.github_repo`, `tasks.source`/`pr_number`/`pr_base_ref`, and the `github_integration` settings key (code-default `on`).

See STACK.md for the full verified command set and rate-budget detail.

### Expected Features

The settled mental model matches the industry pattern (Graphite PR Inbox, gh-dash's "Needs My Review", GitHub's "Review requested" filter): a **queue, not a board** — a filtered list of open PRs awaiting your review, sorted by recency, that mirrors live GitHub state and auto-empties as you review. Kangent's differentiator is the third step list-tools lack: **drop the PR into a real local worktree with an agent + terminal + diff.** Critical implementation fact (resolves cleanly because the column is per-linked-project = one repo): `gh pr list --repo … --search` gives the review-requested filter *and* the rich card fields in one call.

**Must have (table stakes):**
- Global GitHub toggle (default on) + per-project link config (description + repo) — gates everything; cheapest; lands first.
- Collapsible right-side Review column on linked projects (`gh pr list … review-requested`), auto-poll (visibility-gated) + manual refresh, persisted collapse + count-on-collapsed badge.
- Loading / empty ("you're all caught up") / degraded (`gh` missing / unauthenticated) states — non-negotiable for a soft dependency; inline, never a modal, never block the board.
- PR card: number + title, author (+avatar), relative `updatedAt`, draft handling (recommend filtering drafts out by default).
- Click PR → task-like review view (worktree on PR branch) reusing Agent/bash/Diff tabs.
- Diff tab pointed at the PR's `baseRefName` merge-base (the correctness item).
- Auto-remove review worktree on merge/close, gated (dirty + sessions), branch kept.

**Should have (competitive, cheap differentiators — recommend at launch):**
- CI/checks rollup pill (`statusCheckRollup`, reduced server-side to pass/fail/pending/none) — strong triage accelerator.
- Additions/deletions (`+120 −30`) size hint; fork pill (`isCrossRepository`); head→base line (also feeds the diff base).
- Agent pre-pointed at the PR diff in a live checkout — the headline differentiator; falls out for free once the worktree is correct.

**Defer (v1.x / v2+):**
- `reviewDecision` pill, labels, comments-count (add if cards feel info-poor); stale-aging sort.
- Cross-project "all repos" aggregate inbox (forces `gh search prs` thin-field trap), author-side "PRs I opened", any in-app write actions (hard anti-feature).

### Architecture Approach

The integration map is "reuse everything `taskID`-keyed, add one discriminator." A PR review **is** a task in every subsystem that matters (it owns a worktree + agent + bash tabs + diff) — it just isn't a *kanban* task. So model it as a `tasks` row with `source='github_pr'` + `pr_number` + `pr_base_ref`, filter the board with `WHERE source='manual'`, and the session manager, tmux table, diff endpoint, task view, gated-cleanup dialog, and reaper all light up for free. The genuinely new code is small and isolated.

**Major components:**
1. **`internal/github` (NEW leaf package)** — shells out to `gh`; `ListReviewRequestedPRs`, `PRState`, `ValidateRepo`; per-repo cache with quota-style TTL/floor/backoff and typed degraded states (`ok`/`no_gh`/`auth_required`/`disabled`/`error`); always-200 endpoints.
2. **Migration 00007 (NEW columns)** — `projects.description`, `projects.github_repo` (nullable = not linked); `tasks.source` (default `'manual'`), `pr_number`, `pr_base_ref`; `github_integration` settings KV (code-default `on`).
3. **`internal/worktree.CheckoutPR` (NEW verb)** — `git fetch <remote> refs/pull/<n>/head:<localBranch>` + `git worktree add` (a scoped exception to the package's "never fetch" rule); fork-safe via the pull ref.
4. **diff handler (MODIFIED, one branch)** — PR tasks resolve base from `pr_base_ref`; manual tasks keep `ResolveBase`. `internal/diff.Compute(wt, base)` is unchanged (already parameterized).
5. **`internal/reaper` (EXTENDED)** — a second per-tick pass reconciles PR state via `PRState` and runs gated worktree auto-removal on merge/close (the always-on server-side detector — NOT the browser poll).
6. **Frontend Review column + project-config UI (NEW)** — clone of the QuotaIndicator poll pattern; the global toggle reads through the existing shared `useSettings()` query.

### Critical Pitfalls

1. **`gh pr checkout` mutates the CURRENT checkout, not a worktree** — run with `cmd.Dir = project root` it yanks the user's real checkout onto the PR branch. **Never use it.** Use `git fetch <remote> refs/pull/<n>/head:<ref>` + `git worktree add` so all HEAD mutation is confined to the worktree.
2. **Auto-removal on merge/close can destroy un-pushed review work** — merge is an external async signal that can fire mid-review. Gate on dirty-tree **and** running-sessions (research recommends also checking unpushed commits via `git rev-list <headRefOid>..HEAD` and `git stash list`); make removal **deferred + re-checked**, never immediate; **never delete the branch**.
3. **Branch-already-checked-out / fork same-name fast-forward** — a PR head that is `main` or an existing `task/...` branch collides; a fork PR sharing a base-branch name can fast-forward your local base (cli/cli#8383). The `refs/pull/<n>/head` fetch + a Kangent-namespaced local ref (or `--detach`) sidesteps both.
4. **`gh` failures take down the board** — a soft dependency must never break core flows. Isolate in `internal/github` with a typed state enum, always-200 endpoints, all calls off the render path with context timeouts. Don't trust `gh auth status` exit codes alone (cli/cli#8845).
5. **Wrong diff base / wrong review qualifier** — reusing the project merge-base shows a misleading PR diff (use `pr_base_ref`); `review-requested:@me` silently floods with team requests (use `user-review-requested:@me` by default). Also: GitHub auto-removes you from the review-requested set once you submit a review — the column self-empties for free; **don't fight that** by over-caching.

See PITFALLS.md for the full 8-pitfall list, the technical-debt table, and the "Looks Done But Isn't" checklist.

## Implications for Roadmap

Based on combined research, the suggested phase structure is the **A→B→C→D build order all four agents converged on** — each phase is independently shippable and builds only on already-landed columns/services, with the riskiest change (the board-query audit) landing where it is the focus, not buried.

### Phase A — Foundations: schema + global toggle + project link
**Rationale:** Every later phase reads these columns and the toggle; no dependency on the others; ships visible value (project config) immediately. This is also the "degrade-don't-break" foundation — the `internal/github` package and its typed states start here.
**Delivers:** Migration 00007 (`projects.description`/`github_repo`, `tasks.source`/`pr_number`/`pr_base_ref`); `github_integration` setting + SettingsPage field; extended `PATCH /api/projects/{id}` (description + repo) with `gh repo view` validation that degrades to syntactic when `gh` is absent; project-config frontend UI.
**Addresses:** Global toggle + per-project link (table stakes; gates everything).
**Avoids:** Pitfall 7 (`gh` failures break the board) — toggle + per-project link both short-circuit `gh` entirely. Lays the typed-state foundation.

### Phase B — `internal/github` package + read-only PR list + Review column
**Rationale:** Pure read path — no worktree/task mutation — so it's the lowest-risk way to stand up the whole `gh` integration and the Review column. Depends only on Phase A (needs `github_repo` + toggle).
**Delivers:** `internal/github.ListReviewRequestedPRs` + quota-shaped cache/poll/degrade `Service` wired in `main.go`; `GET /api/projects/{id}/pull-requests[?refresh=1]` (toggle- and link-gated, always 200); frontend `usePullRequests` hook + collapsible Review column + PR cards (number/title/author/updatedAt + cheap richness: checks rollup, +/−, fork pill, head→base).
**Uses:** `gh pr list -R <repo> --search "<qualifier>:@me" --state open --json <rich set>`; reuses the Phase-7 visibility-gated poll + manual-refresh pattern.
**Implements:** `internal/github` package + Review column components.
**Avoids:** Pitfall 5 (review-qualifier semantics — the column must match GitHub's own filtered page), Pitfall 8 (rate limits — one list call per project per poll, no `gh pr view` per card).

### Phase C — Open-a-review: PR task creation + PR worktree checkout + diff base
**Rationale:** The milestone's headline — the reused task view lights up for PRs. Depends on B (the PR list/cache populates `pr_base_ref`) and A (columns). This is where the **highest-risk regression** lives, so the per-query `source`-filter audit is the phase's focus.
**Delivers:** `worktree.CheckoutPR` verb (`git fetch refs/pull/n/head` + `worktree add`); `provisionWorktree` branch on `source`; `POST .../{n}/review` find-or-create task (+ `UNIQUE(project_id, pr_number)`); board query `WHERE source='manual'` filter **plus an audit of every `tasks` SELECT**; `/move` guard for PR tasks; diff handler base selection by `source`/`pr_base_ref`; PR card click → reused TaskPage.
**Addresses:** Click-PR → review view; diff at PR base (correctness).
**Avoids:** Pitfall 1 (`gh pr checkout` mutation — never use it), Pitfall 3 (branch-already-checked-out — namespaced ref / `--detach`), Pitfall 4 (fork same-name fast-forward — pull-ref fetch), Pitfall 6 (wrong diff base — use `pr_base_ref`), and the board-leak regression.

### Phase D — Auto-cleanup on merge/close (reaper extension)
**Rationale:** Operates on the artifacts the earlier phases create; the shared-helper extraction is safest once the manual cleanup path is stable and PR tasks exist. Depends on C (PR tasks with worktrees) and B (`PRState`).
**Delivers:** Extract `cleanupWorktreeGated` shared helper from `worktreeHandlers.remove` (regression-guarded; one path, two callers — HTTP + reaper); `internal/github.PRState` + reaper `PRStateGetter` interface + `reconcilePRsOnce` pass; `ghSvc` wired into the reaper.
**Avoids:** Pitfall 2 (auto-removal destroys un-pushed work) — gated on dirty + sessions (recommend also unpushed-commits/stash), deferred-and-re-checked, branch never deleted. Detect merges **server-side in the always-on reaper**, never from the browser poll.

### Phase Ordering Rationale

- **Dependencies force the order:** A's columns/toggle are read by everything; B's cache populates the `pr_base_ref` that C's task creation captures; D cleans up the worktrees C creates and needs B's `PRState`.
- **Architecture grouping:** A is schema/config, B is the read-only `gh` surface, C is the write-into-local-worktree surface, D is background reconciliation — each maps to one isolated change in the existing component map.
- **Risk placement:** the board-query `source`-filter audit (the milestone's single highest-risk regression) and the checkout mechanics (Pitfalls 1/3/4/6) both land in C where they are the explicit focus, after the low-risk read path (B) is proven.

### Research Flags

Phases likely needing deeper research during planning (`/gsd:research-phase`):
- **Phase C:** the PR-branch worktree checkout is the milestone's risk center — `gh pr checkout` is not worktree-aware (cli/cli#972) and fails on `/`-branches (cli/cli#3231); fork same-name fast-forward (cli/cli#8383). The `git fetch refs/pull/<n>/head` + `worktree add` path is verified end-to-end in research, but the named-branch-vs-`--detach` choice and the per-query audit warrant a focused spike. **Carry forward these success criteria:** "primary checkout HEAD unchanged after opening a fork PR with a colliding branch name"; "Kangent's review diff matches GitHub's Files-changed."
- **Phase D:** the gated-cleanup-helper extraction (refactoring live cleanup code) plus the unpushed-work gate beyond `git status --porcelain` deserve careful, regression-guarded planning.

Phases with standard / verified patterns (likely skip research-phase):
- **Phase A:** migrations, settings KV, PATCH extension — all follow established codebase patterns (migration 00006, KeyShell validation, `validateRepoPath`).
- **Phase B:** a near-verbatim clone of the proven quota proxy + auto-poll pattern; all `gh` commands and `--json` fields verified live on 2.82.0.

### Decisions needed (product) — resolve in requirements

1. **Review qualifier (PROMINENT):** `review-requested:@me` (includes ALL team review requests — flood risk, ~100 PRs seen) vs `user-review-requested:@me` (direct only). **Recommended default: `user-review-requested:@me`.** GitHub auto-removes you from the set on review submit (column self-empties) — don't fight it.
2. **Drafts:** show or hide draft PRs by default? (Research recommends filtering out by default.)
3. **`statusCheckRollup` reduction policy:** the exact pass/fail/pending/none roll-up rule (any failure → fail; any pending → pending; all pass → pass; none → none).
4. **Collapse/count persistence scope:** per-project vs global, and SQLite vs localStorage.
5. **Link-validation strictness:** strict (`gh repo view`) when `gh` is present + fall back to syntactic when absent (recommended), vs always-syntactic.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Every `gh`/`git` command, flag, and `--json` field run live against host `gh 2.82.0` + real repos (incl. 7 fork PRs); worktree checkout proven end-to-end (worktree HEAD matched the PR's `headRefOid`). |
| Features | HIGH | `gh` capabilities/field availability verified; competitor UX (gh-dash, Graphite, vibe-kanban) MEDIUM but corroborated across multiple sources; fork+worktree mechanics MEDIUM-HIGH (cli/cli#972/#3231). |
| Architecture | HIGH | Every integration point names the real v1.2 file/function/table/migration it touches; modeling decision forced by the `taskID`-keyed session manager (verified in source). |
| Pitfalls | HIGH | gh/git behaviors + GitHub search semantics verified against official docs + cli/cli issues (#8383, #8845, #972, #3231, community #137828); a few app-integration judgments MEDIUM and flagged inline. |

**Overall confidence:** HIGH

### Gaps to Address

- **Review qualifier is a genuine product gap, not a research gap** — the technical answer is clear (both qualifiers work; recommend `user-review-requested:@me`), but the intent call is the user's. Resolve in requirements and log as a decision; success criterion = "column matches GitHub's own filtered page for the chosen qualifier."
- **Named-branch vs detached-HEAD for the PR worktree** — STACK recommends a named `review/pr-<n>`/`kangent-pr/<n>` branch (cleaner diff/porcelain UX); PITFALLS recommends `--detach` (no branch collision, aligns with no-writes). Reconcile in Phase C planning: prefer a Kangent-namespaced local branch with collision pre-check, fall back to `--detach`.
- **Unpushed-work gate scope** — whether to extend the dirty gate beyond `git status --porcelain` to unpushed commits + stash (research recommends it) is a Phase-D scoping call.
- **Multi-host / multiple `gh` accounts** — run `gh` with `cmd.Dir` = the worktree/repo so repo context selects the right host/account; verify `GH_HOST`/enterprise doesn't silently fall back to github.com. Handle during Phase B implementation.

## Sources

### Primary (HIGH confidence)
- Live execution on host `gh 2.82.0 (2025-10-15)` — `gh pr list/view/checkout/search/auth status --help`, `gh help exit-codes`; live `gh pr list --search "review-requested:@me" --json …` against real repos (every field returns data; fork detection on 7 PRs); live `gh pr view --json state,…` (UPPERCASE state, no `merged` field); live end-to-end worktree proof (`git fetch refs/pull/<n>/head` + `git worktree add`, HEAD matched `headRefOid`); live `gh api rate_limit` (core 5000/hr vs search 30/min).
- Live codebase inspection of v1.2 — `internal/{worktree,quota,tmux,reaper,diff,session,settings}`, `internal/api/*`, migrations 0000{1,3,5,6}, `cmd/kangent/main.go`, frontend `TaskPage`/`Board`/`TaskTabs`/`api/{queries,usage,settings}`.
- GitHub Docs — review qualifiers (`review-requested` vs `user-review-requested` vs `team-review-requested`); "requested reviewers no longer listed after they review."
- cli/cli #8383 (fork same-name fast-forward), #8845 (`gh auth status` exit-code), discussion #5902 (`--json` fields), #13239 (`gh search prs` thin fields).
- git-worktree(1) — "already checked out" lock + `--detach`.
- `.planning/PROJECT.md` — v1.3 milestone scope, settled decisions, D-* decision log.

### Secondary (MEDIUM confidence)
- gh-dash PR-section docs ("Needs My Review" `is:open review-requested:@me`, columnar fields); Graphite PR-inbox guides ("empty = caught up", section model); vibe-kanban GitHub-integration / PrMonitorService polling + orphan-worktree cleanup (DeepWiki).
- cli/cli #972 (checkout-PR-as-worktree request) + superset #3231 (`gh pr checkout` fails on `/`-branches in a worktree).
- cli.github.com manuals (gh_pr_list/view/checkout/search/auth_status); community #137828 (team-flood confirmation), #156480 (rate-limit polling), #4221 (multi-account host selection).
- `refs/pull/<n>/head` convention; GitHub Changelog (clearer reviewer status; re-request review).

### Tertiary (LOW confidence)
- Blog/guide write-ups of the `refs/pull/N/head` + `git worktree add --detach` PR-review pattern (corroborate the verified-live mechanic; not load-bearing on their own).

---
*Research completed: 2026-06-13*
*Ready for roadmap: yes*
