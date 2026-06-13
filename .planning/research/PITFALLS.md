# Pitfalls Research

**Domain:** Adding `gh`-CLI GitHub PR review + PR-branch worktree checkout to an existing local Go+React agent-session app (Kangent v1.3)
**Researched:** 2026-06-13
**Confidence:** HIGH (gh/git behaviors and GitHub search semantics verified against official docs + cli/cli issues; a few app-integration judgments are MEDIUM and flagged inline)

> Scope note: these pitfalls are specific to wiring `gh pr` + PR-branch worktrees into THIS app's existing primitives (git-CLI worktree-per-task, server-owned PTY sessions, merge-base diff tab, best-effort quota proxy, Done-TTL reaper). Generic "use GitHub responsibly" advice is omitted. Phase references use placeholders (PR-1..PR-6) that the roadmap should map onto the six Active v1.3 requirements; suggested ownership is given per pitfall.

---

## Critical Pitfalls

### Pitfall 1: `gh pr checkout` mutates the CURRENT checkout, not a worktree

**What goes wrong:**
`gh pr checkout <n>` is designed to run in a repo and **switch the current working tree's HEAD/branch** to the PR branch (or detached HEAD with `--detach`). If Kangent runs it with `cmd.Dir = <project repo root>` (the user's real checkout, which the app's premise says they actively work in), it will yank that checkout onto the PR branch — clobbering whatever the user had checked out, possibly stashing/failing on dirty state. This is the single most dangerous default in the milestone because the settled design says "clicking a PR creates a worktree that checks out the PR branch," but `gh pr checkout` by itself does NOT create a worktree.

**Why it happens:**
The mental model "`gh pr checkout` = get the PR branch" hides that it operates on the cwd's index/HEAD. The fork case is worse: for cross-repo PRs it fetches `refs/pull/<n>/head` and creates/switches a local branch in the current tree.

**How to avoid:**
Two safe patterns; pick one and make it the only path:
- **(Preferred) Create the worktree first, then checkout inside it.** `git worktree add --detach <wt-path> <base-or-placeholder>` (or a fresh worktree on a throwaway ref), then run `gh pr checkout <n>` (or `gh pr checkout <n> -b <local-branch>`) with `cmd.Dir = <wt-path>`. Now all branch/HEAD mutation is confined to the worktree.
- **(Most robust) Don't use `gh pr checkout` for the git mechanics at all.** Use `gh` only to resolve metadata (`gh pr view <n> --json headRefName,headRefOid,headRepositoryOwner,isCrossRepository,baseRefName,baseRefOid`), then drive git yourself: `git fetch <project-remote> "pull/<n>/head"` and `git worktree add --detach <wt-path> FETCH_HEAD` (or `<headRefOid>`). `--detach` guarantees no branch is occupied (see Pitfall 3) and nothing in the user's main checkout moves. This mirrors the app's existing "shell out to git, parse `--porcelain`, never touch the user's checkout" philosophy.

Never set `cmd.Dir` to the project root for any state-mutating `gh pr`/git command in this feature.

**Warning signs:**
After opening a PR for review, the user's primary checkout is suddenly on a different branch; `git status` in the main repo shows unexpected changes; a "your local changes would be overwritten" error surfaces from a click in the UI.

**Phase to address:** PR-5 (PR review view / worktree checkout). Make "main checkout HEAD is never modified" an explicit success criterion.

---

### Pitfall 2: Auto-removing a review worktree destroys un-pushed review work (the invariant break)

**What goes wrong:**
v1.3 deliberately breaks the prior "worktrees are never auto-removed" invariant: a review worktree is auto-removed when its PR merges/closes. A reviewer commonly accumulates local-only artifacts in that worktree — scratch notes, a WIP fixup commit, `git notes`, stash entries, uncommitted experiments the agent produced while reasoning about the PR. If auto-removal fires while that exists (especially the "merged" path, which can happen at any moment from GitHub's side, mid-review), the user silently loses work that was never on a remote.

**Why it happens:**
Merge/close is an *external, asynchronous* signal — it can arrive seconds after the user started poking at the PR. The reaper precedent (Done-TTL) trained the codebase to treat background destruction as safe because "killing a PTY is reversible." Worktree removal is NOT reversible, and the PR's merge is outside the user's control.

**How to avoid:**
- Reuse the EXISTING gated-cleanup machinery from Phase 3 verbatim: dirty-tree gate (`git status --porcelain` non-empty → refuse) AND running-session gate (any live agent/bash/tmux session in this worktree → refuse). The milestone already requires both; treat them as hard preconditions, not advisories.
- Extend the dirty check beyond `git status --porcelain`: also refuse if there are local commits not present on the PR head (`git rev-list <headRefOid>..HEAD` non-empty) and if `git stash list` is non-empty for that worktree. Plain `--porcelain` will not catch a committed-but-unpushed fixup.
- Make auto-removal a **deferred, re-checked** action, not an immediate one: on detecting merge/close, mark the card "merged — safe to remove" and only remove after the gates pass; if gated, leave the worktree and surface a manual "Remove" affordance. Never block or retry-loop on a gated worktree.
- **Never delete the branch.** The branch may belong to a fork / another contributor; even for same-repo PRs the branch is GitHub's to manage. Removal touches the worktree directory only (`git worktree remove`), matching the existing "branch always kept" rule.

**Warning signs:**
A user reports "my notes/commit on a PR vanished after it merged"; cleanup logs show removal succeeding on a worktree that had `git status` output; the removal path calls `git branch -D` anywhere.

**Phase to address:** PR-6 (auto-cleanup on merge/close). Flag the dirty+session+unpushed-commit gate and "branch never deleted" as explicit success criteria. This is the highest-severity new behavior in the milestone.

---

### Pitfall 3: Checking out a PR branch that's already checked out in another worktree

**What goes wrong:**
git refuses to check out the same branch in two worktrees: `fatal: '<branch>' is already checked out at '<path>'`. This happens readily here: (a) a same-repo PR whose head branch is a `task/...` branch the user already has a Kangent worktree for; (b) the user opens the same PR for review twice; (c) the PR head branch name collides with an existing local branch that's checked out elsewhere; (d) the PR head equals the project's default branch (the user's main checkout occupies it).

**Why it happens:**
The naive implementation does `git worktree add -b <headRefName>` or `gh pr checkout` (which wants a named branch). Named-branch checkout is exactly what triggers the lock.

**How to avoid:**
Check out the PR head **as a detached worktree** keyed on the commit, not the branch name: `git worktree add --detach <wt-path> <headRefOid>` (after fetching `pull/<n>/head`). A detached HEAD occupies no branch, so it never collides and never blocks the branch elsewhere. This is the documented PR-review pattern and is ideal because v1.3 forbids in-app GitHub writes — the user reviews/comments in the terminal and doesn't need a local branch ref. If a named branch is ever wanted, generate a unique, namespaced local name (e.g. `pr/<n>`) and detect collision first via `git worktree list --porcelain` + `git show-ref`.

**Warning signs:**
Opening a PR fails with "already checked out at"; opening the same PR twice errors; PRs whose head is `main`/`master` or a `task/...` branch fail to open.

**Phase to address:** PR-5. Verify by opening a PR whose head branch is also an existing task worktree's branch — it must succeed via detached HEAD.

---

### Pitfall 4: Fork (cross-repository) PRs — wrong remote, same-name-branch corruption, read-only head

**What goes wrong:**
For PRs from forks (`isCrossRepository: true`): (a) the head branch lives in a *different* repo, so `git fetch origin <branch>` fetches the wrong thing or nothing; (b) the notorious cli/cli #8383 case — when the fork's head branch shares a name with the base branch (e.g. both `main`/`master`), `gh pr checkout` can **fast-forward the current branch** and silently add the fork's commits to your local base branch (and a later push would push to your repo); (c) the head ref is on someone else's repo, so any "save back" assumption is invalid (read-only).

**Why it happens:**
The implementation treats every PR like a same-repo branch. Fork PRs need the `refs/pull/<n>/head` ref (which GitHub maintains regardless of fork) or an explicit fork remote.

**How to avoid:**
- Always fetch via the PR ref, never the branch name: `git fetch <project-remote> "pull/<n>/head"` then `git worktree add --detach <wt-path> FETCH_HEAD`. `pull/<n>/head` resolves the fork head from the *base* repo, sidestepping fork-remote management and the #8383 same-name fast-forward entirely.
- Branch detection from `gh pr view --json isCrossRepository,headRepositoryOwner,headRefName`: surface fork PRs as such in the card (e.g. `owner:branch`) so the user isn't surprised the worktree is detached/read-only-ish.
- Never assume the head is writable by the user; v1.3 already forbids in-app writes, so this aligns — just don't build any "push from worktree" affordance.

**Warning signs:**
Fork PRs open onto the wrong code or fail to fetch; the project's local base branch unexpectedly gains the PR's commits; head-branch name collisions appear only for forks.

**Phase to address:** PR-5. Verify by opening a fork PR whose head branch is named `main`/`master` (the #8383 trap) — base branch must remain untouched.

---

### Pitfall 5: "Review requested" semantics — `review-requested:@me` silently includes TEAM requests

**What goes wrong:**
The milestone says "open PRs where review is requested from the authenticated user." There are two different GitHub qualifiers, and they are NOT the same:
- `review-requested:@me` → **direct requests AND every team you belong to** (verified: community discussion #137828 — one user saw ~100 PRs because all team requests are included).
- `user-review-requested:@me` → **direct requests to you only** (the `user-` prefix is the direct-only filter).

Picking `review-requested:@me` floods the Review column with team PRs the user may have no intent to review; picking `user-review-requested:@me` hides PRs assigned via a team (which the user may genuinely owe). Either is "wrong" depending on intent — the spec is ambiguous and must be resolved as a decision, not guessed.

**Why it happens:**
The two qualifiers look interchangeable; the team-inclusion behavior of the unprefixed form is undocumented in the primary help text and only surfaces in practice/discussion.

**How to avoid:**
- Make the qualifier an explicit, logged decision. **Recommended default: `user-review-requested:@me`** (direct, matches "requested from you" literally, avoids the 100-PR flood), with team-inclusion as a possible later toggle. (MEDIUM-confidence recommendation; the precise UX is the user's call.)
- Drive the list with `gh search prs` or `gh pr list --search` using the chosen qualifier plus `is:open` and `archived:false`; scope per-linked-project with `repo:<owner>/<name>` so each board column only shows that project's PRs.
- Critical correctness fact: **GitHub removes a user from the review-requested set the moment they submit a review.** So the column auto-empties when the user reviews — good, this is the desired "card disappears as review state changes" behavior, and it comes for free; don't reimplement it.
- Re-request-after-changes correctly **re-adds** the user (it's a fresh request), so the card reappears — also desired. Don't dedupe it away.
- Do NOT additionally filter on `reviewDecision`/`review:changes_requested` to hide PRs — those reflect the PR's aggregate state, not "is a request pending from me," and will wrongly hide re-requested PRs.

**Warning signs:**
The Review column shows dozens of PRs the user doesn't recognize (team flood) → wrong qualifier; PRs the user knows they were assigned via a team never appear → over-restrictive; a PR the user already reviewed lingers → you're not trusting GitHub's auto-removal and are caching stale state.

**Phase to address:** PR-3 (PR review column). The exact qualifier should be a documented decision (D-xx) and an explicit success criterion ("column matches GitHub's own 'Review requested' page for the same filter").

---

### Pitfall 6: PR diff base ≠ project base-branch merge-base

**What goes wrong:**
The existing diff tab (Phase 5) computes changes vs the *project's* configured base branch merge-base. A PR can target a DIFFERENT base (`baseRefName` may be a release branch, a stacked-PR parent, `develop`, etc.). Reusing the project-base merge-base for a PR worktree shows a diff that mixes in unrelated commits from the real base — a misleading review surface, the worst possible failure for a review feature.

**Why it happens:**
The diff tab already exists and "just works" for tasks (whose base IS the project base), so it's tempting to reuse it unchanged for PR worktrees.

**How to avoid:**
For PR worktrees, compute the diff base from the PR, not the project: fetch `gh pr view <n> --json baseRefName,baseRefOid`, fetch that base ref (`git fetch <remote> <baseRefName>`), and diff against `git merge-base <baseRefOid> HEAD` (the PR's own merge-base). Plumb a per-worktree "diff base ref" so the existing diff renderer is reused but parameterized, rather than forked. Re-resolve the base on refresh (a PR's base can be retargeted).

**Warning signs:**
A PR's diff shows far more files/commits than the PR's "Files changed" tab on GitHub; the diff includes commits the PR author didn't make; PRs targeting non-default branches look enormous.

**Phase to address:** PR-5 (review view diff), building on the Phase 5 diff tab. Success criterion: PR diff file/line counts match GitHub's Files-changed for the same PR.

---

### Pitfall 7: `gh` availability/auth failures take down the board (violating degrade-don't-break)

**What goes wrong:**
`gh` is a *soft* dependency, but a naive integration lets its failures bleed into the core app: a blocking `gh` call on board render hangs the whole board; an `exec` error (gh not installed) bubbles a 500; an auth-expired error mid-session makes the Review column throw instead of degrading. The settled design explicitly says this must mirror the quota indicator (best-effort, degrade-don't-break).

**Why it happens:**
The quota proxy already established the right pattern, but the PR feature touches more surfaces (column list, card sync, worktree creation), so it's easy to let one of them be a hard call. Also: `gh auth status` has a known exit-code bug (cli/cli #8845 — historically returned 0 even when unauthenticated; `--json` always exits 0), so naive exit-code checks misreport auth.

**How to avoid:**
- Treat the GitHub integration as an isolated `internal/github` package (mirroring `internal/quota`) with a single typed result that always includes an availability/state enum: `{ok, gh_missing, not_authenticated, host_mismatch, rate_limited, error}`. Every consumer renders a graceful state; nothing throws.
- Detect availability robustly: `exec.LookPath("gh")` for presence; for auth, prefer parsing `gh auth status` output (or detecting `hosts.yml`) over trusting the exit code; better still, treat the first real API failure (e.g. `gh pr list` returning an auth error) as the source of truth and surface `not_authenticated`.
- All `gh` calls run with a context timeout and OFF the request-render path (background poller, like the quota auto-poll). The board, projects, tasks, terminals, and diff for non-PR work must function identically with `gh` absent.
- The global GitHub toggle (default on) and per-project "no linked repo" both short-circuit `gh` entirely — no calls, no column.

**Warning signs:**
Board render latency spikes when GitHub/network is slow; uninstalling `gh` breaks unrelated pages; a 500 originates from a `gh` exec; auth state shows "OK" while every PR call fails.

**Phase to address:** PR-1 (global toggle) and PR-3 (column). Make "uninstall `gh` → entire rest of app unaffected" an explicit success criterion (a literal test: rename the gh binary, app still fully usable).

---

### Pitfall 8: Auto-polling many linked projects trips primary/secondary rate limits

**What goes wrong:**
Auto-poll per linked project, multiplied by manual refreshes and re-polling on every board render/tab focus, can burst enough `gh` (GitHub API) calls to hit the primary rate limit or — more likely for bursts — the **secondary/abuse** limit (403, `Retry-After`). Each card-detail enrichment (diff base, fork detection via `gh pr view`) multiplies calls per PR. The result: the Review column starts failing intermittently for ALL projects.

**Why it happens:**
"Refresh on render" + "one `gh pr view` per card" + N projects compounds quickly. `gh` adds auth but does not magically debounce your invocation pattern.

**How to avoid:**
- Reuse the quota indicator's caching + backoff shape: a single background poller per *enabled* state, not per render; cache results with a short TTL; pause polling when the tab is hidden (already a milestone requirement — honor it strictly).
- One list call per project per poll (`gh pr list --search ...`), and lazily enrich a PR's `baseRefName`/fork status only when the user actually opens it (or batch via a single GraphQL `gh api graphql` query) — do NOT `gh pr view` every card on every poll.
- Coalesce manual refresh with the poll loop (debounce; a manual refresh resets the timer rather than adding a call).
- On 403/secondary-limit, honor `Retry-After`, apply exponential backoff, surface `rate_limited` state, and STOP — never tight-retry. (`gh` already retries some transient cases; don't stack your own aggressive retry on top.)
- Default poll interval generous (e.g. 60s, matching the quota poller) and ideally jittered across projects so they don't all fire on the same tick.

**Warning signs:**
Intermittent empty/error columns under several linked projects; 403 with `Retry-After` in logs; call volume scales with board re-renders rather than with time.

**Phase to address:** PR-3 (column + polling). Success criterion: with N linked projects open, steady-state call rate is bounded and independent of UI re-renders.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Use `gh pr checkout` directly with `cmd.Dir` = project root | One command, "it works" in a demo repo | Clobbers the user's real checkout; data loss; the #8383 fork fast-forward corruption | **Never** — always worktree-first / detached |
| Reuse the task diff tab's project-base merge-base for PR worktrees | Zero new diff code | Misleading diffs for PRs targeting non-default bases — defeats the feature's purpose | Only if you verify every linked project's PRs target the default branch (you can't) — effectively never |
| `review-requested:@me` because it's the obvious qualifier | Matches the most-Googled snippet | Silent team-PR flood; mismatch with user intent; hard to change once users build habits | Only if the product explicitly wants team requests included AND it's a logged decision |
| Trust `gh auth status` exit code for auth detection | Simple boolean | Misreports auth (cli/cli #8845); column shows OK while failing | Never alone — parse output or use first real call's result |
| Immediate worktree removal the instant merge/close is detected | Simple reaper logic | Races mid-review; destroys unpushed work; breaks the new invariant unsafely | Never — must be gated + deferred + re-checked |
| `gh pr view` per card on every poll for diff-base/fork info | Cards are "fully populated" | N×PRs API calls per tick → rate limits | Only lazily on open, or via one batched GraphQL call |
| Delete the PR branch on cleanup ("tidy up") | Cleaner branch list | Deletes refs the user/forks don't own; breaks "branch always kept" | **Never** — worktree dir only |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| `gh pr checkout` | Assuming it creates a worktree / runs anywhere | It mutates the cwd's HEAD; run it *inside* a pre-created worktree, or skip it and use `git fetch pull/<n>/head` + `git worktree add --detach` |
| Fork PRs | `git fetch origin <headRefName>` | `git fetch <remote> "pull/<n>/head"` (base repo serves fork heads); detect via `isCrossRepository` |
| Review-requested filter | `review-requested:@me` = "requested from me" | That includes ALL team requests; use `user-review-requested:@me` for direct-only — decide explicitly |
| Already-reviewed PRs | Caching them as "still needs review" | GitHub drops you from the set on submit; trust the live query, don't over-cache |
| Re-requested review | Deduping it as "already seen" | A re-request is a NEW request; let the card reappear |
| Multiple gh accounts / enterprise | Assuming one default host/account | `gh` defaults to github.com without repo context; respect `GH_HOST`/active account; run `gh` with `cmd.Dir` = the worktree so repo context picks the right host/account |
| `gh auth status` | Relying on exit code | Parse output / treat first failing API call as truth (#8845) |
| Secondary rate limit | Tight-retry on 403 | Honor `Retry-After`, exponential backoff, surface `rate_limited`, stop |
| Diff base | Reusing project merge-base | Use PR's `baseRefName`/`baseRefOid` merge-base, re-resolved on refresh |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Poll/enrich on every board render | Call volume tracks UI activity, not time | Single background poller; cache + TTL; pause when tab hidden | A few linked projects + active UI |
| `gh pr view` per card per tick | API calls = N projects × M PRs × ticks | Lazy enrich on open, or one batched `gh api graphql` per project | ~handful of projects with several PRs each |
| Synchronous `gh` on the request path | Board/terminal latency spikes when network slow | All `gh` off-render, context-timeouts | Any slow/offline network |
| Synchronous merge/close detection per render | Repeated `gh pr view --json state` storms | Fold merge/close detection into the existing poll result | Many open review worktrees |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Logging `gh` output verbatim | PR titles/bodies/URLs and possibly tokens land in logs | Log structured metadata (PR number, state) at info; gate full output behind debug; never log env |
| Inheriting full env into `gh` (`GH_TOKEN`, etc.) without thought | App becomes a token-laundering surface; wrong-host/account leakage | App stores NO tokens (settled). Let `gh` use its own config; pass a minimal env; set `cmd.Dir` so repo context selects host/account; never echo `GH_TOKEN`/`GITHUB_TOKEN` |
| Private-repo PR code/diffs persisted under `~/.kangent/worktrees` | Private source on disk beyond the user's own checkout | Same trust model as existing task worktrees (local, single-user); ensure auto-cleanup actually reclaims them; don't copy diffs into SQLite — recompute |
| Surfacing `gh` stderr (which can include auth URLs/tokens on some flows) to the browser | Sensitive strings rendered in UI | Map errors to the typed state enum; never pass raw stderr to the client |
| Assuming localhost = no exposure for GitHub data | The WS/API is already Origin/Host-validated; new endpoints might not be | Apply the existing loopback-bind + Origin/Host validation to all new `/api/github/*` routes |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Review column floods with team PRs | Column "basically useless" (the #137828 complaint) | Default to `user-review-requested:@me`; make team-inclusion an opt-in |
| Card vanishes mid-review (user submitted review, or PR merged) | Confusion / lost place | Expected for the list, BUT keep the *open review view/worktree* alive until gated-cleanup; don't yank the panel out from under the user |
| Worktree silently removed on merge | "Where did my review go?" | Gated + deferred removal; show a "merged — worktree kept (dirty)" state when gates block; provide manual remove |
| Fork/detached HEAD surprises the user in the terminal | "Why am I on a detached HEAD / can't push?" | Label fork PRs and detached state in the card/header; it's intentional (no in-app writes) |
| PR diff looks huge (wrong base) | User distrusts the whole feature | PR-specific base; match GitHub's Files-changed |
| Hard error when `gh` missing/unauth | Whole board feels broken | Degrade to a quiet "GitHub unavailable" state in the column only |

## "Looks Done But Isn't" Checklist

- [ ] **PR worktree checkout:** Often missing the worktree-first/detached path — verify the user's MAIN checkout HEAD is never moved when opening a PR (test with a dirty main checkout).
- [ ] **Branch-already-checked-out:** Often missing — verify opening a PR whose head is `main` or an existing task branch succeeds (via `--detach`).
- [ ] **Fork PR with same-name branch:** Often missing — verify a fork PR named `main`/`master` does NOT add commits to the local base (the #8383 trap).
- [ ] **Review-requested filter:** Often missing the team-vs-direct decision — verify the column matches GitHub's own filtered page for the chosen qualifier; verify a PR drops after you review it and reappears after a re-request.
- [ ] **Diff base:** Often missing PR-specific base — verify file/line counts match GitHub's Files-changed for a PR targeting a non-default branch.
- [ ] **Auto-cleanup gates:** Often missing the unpushed-commit/stash check — verify a merged PR with a local fixup commit and with a running session is NOT removed.
- [ ] **Branch preservation:** Verify cleanup NEVER runs `git branch -D` (grep the codebase).
- [ ] **Degrade-don't-break:** Verify renaming the `gh` binary leaves every non-GitHub feature fully functional and the column shows a quiet unavailable state.
- [ ] **Rate-limit behavior:** Verify steady-state `gh` call rate is bounded and independent of UI re-renders; verify 403 → backoff, not tight-retry.
- [ ] **Unlink/delete races:** Verify unlinking a repo or deleting a project with live PR-review worktrees uses the existing gated cleanup (kill sessions → remove worktree, keep branch).
- [ ] **Multi-host/account:** Verify `gh` commands run with worktree `cmd.Dir` so the correct host/account is selected; verify `GH_HOST`/enterprise doesn't silently fall back to github.com.

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| `gh pr checkout` clobbered the main checkout | MEDIUM | `git reflog` in the main repo → `git checkout <prior-branch>`; restore stash if one was made; then patch to worktree-first path |
| Fork same-name fast-forward added commits to local base | MEDIUM | `git reset --hard <pre-checkout-base-oid>` on the local base branch (recoverable via reflog); switch to `pull/<n>/head` fetch |
| Auto-removal destroyed unpushed review work | HIGH | Often unrecoverable (the whole point of the gates) — `git fsck --lost-found` on the common dir may recover dangling commits if not GC'd; primary fix is preventing it via gates |
| Review column flooded (wrong qualifier) | LOW | Switch qualifier to `user-review-requested:@me`; invalidate cache |
| Rate-limited / 403 storm | LOW | Back off, wait out `Retry-After`; raise poll interval; switch to batched GraphQL enrichment |
| Wrong diff base shipped | LOW | Parameterize diff base from `baseRefOid`; recompute (diffs aren't persisted) |
| Branch deleted by overzealous cleanup | MEDIUM | If same-repo and recently pushed, `git fetch` from origin or `git branch <name> <oid>` from reflog; fork branches unrecoverable locally — fix code to never delete |

## Pitfall-to-Phase Mapping

> Phase IDs PR-1..PR-6 map to the six Active v1.3 requirements in PROJECT.md order. Roadmap should rename to match its ROADMAP.md.

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| 1. `gh pr checkout` mutates current checkout | PR-5 (review view/worktree) | Main checkout HEAD unchanged after opening a PR (dirty main checkout test) — **success criterion** |
| 2. Auto-removal destroys unpushed work (invariant break) | PR-6 (auto-cleanup) | Merged PR with local commit + running session is NOT removed; `git branch -D` never called — **success criterion** |
| 3. Branch already checked out elsewhere | PR-5 | Open PR whose head is `main`/an existing task branch succeeds (detached) |
| 4. Fork PR remote/same-name/read-only | PR-5 | Fork PR named `main` doesn't touch local base (#8383) — **success criterion** |
| 5. "Review requested" team-vs-direct semantics | PR-3 (column) | Column matches GitHub's filtered page; drops after review, reappears on re-request — **success criterion** + logged decision |
| 6. PR diff base ≠ project merge-base | PR-5 | Diff file/line counts match GitHub Files-changed for a non-default-base PR — **success criterion** |
| 7. `gh` failures break the board | PR-1 (toggle) + PR-3 (column) | Rename `gh` binary → rest of app fully functional — **success criterion** |
| 8. Rate limits under auto-poll | PR-3 (polling) | Bounded steady-state call rate independent of re-renders; 403 → backoff |
| Security (no token logging, env, private data) | PR-1/PR-3 (foundation) | No `GH_TOKEN`/stderr in logs or UI; new routes loopback+Origin validated |
| Stale/race (unlink/delete with live worktree) | PR-6 | Unlink/delete uses gated cleanup; head force-push re-resolves OID on next poll |

## Sources

- [GitHub Docs — Filtering and searching issues and PRs](https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/filtering-and-searching-issues-and-pull-requests) — review:* qualifiers; "requested reviewers no longer listed after they review" (HIGH)
- [GitHub Docs — Searching issues and PRs](https://docs.github.com/en/search-github/searching-on-github/searching-issues-and-pull-requests) — review-requested vs user-review-requested vs team-review-requested (HIGH)
- [GitHub Blog — Easily filter review requests by team](https://github.blog/news-insights/product-news/easily-filter-review-requests-by-team/) — `user-` prefix = direct-only; unprefixed includes teams (HIGH)
- [community discussion #137828 — "Review requested shows also requests from the team"](https://github.com/orgs/community/discussions/137828) — confirms team flood for `review-requested:@me` (HIGH)
- [GitHub Changelog — Re-request review on a pull request](https://github.blog/changelog/2019-02-21-re-request-review-on-a-pull-request/) + [community #17875](https://github.com/orgs/community/discussions/17875) — re-request creates a fresh pending request (MEDIUM/HIGH)
- [cli/cli #8383 — gh pr checkout adds commits to current branch](https://github.com/cli/cli/issues/8383) — fork same-name fast-forward corruption (HIGH)
- [cli/cli manual — gh pr checkout](https://cli.github.com/manual/gh_pr_checkout) — flags (`--detach`, `-b`, `-f`), mutates cwd HEAD, fork uses `refs/pull/<n>/head` (HIGH)
- [cli/cli #282 / PR #1889 — PR commands on detached HEAD](https://github.com/cli/cli/issues/282) — detached-HEAD handling history (MEDIUM)
- [cli/cli discussion #5902 — gh --json fields](https://github.com/cli/cli/discussions/5902) + [gh pr view manual](https://cli.github.com/manual/gh_pr_view) — available JSON fields incl. baseRefName, baseRefOid, isCrossRepository, headRepositoryOwner, reviewRequests, reviewDecision (HIGH)
- [git-worktree(1) docs](https://git-scm.com/docs/git-worktree) + [kernel.org git-worktree](https://www.kernel.org/pub/software/scm/git/docs/git-worktree.html) — "is already checked out" lock; `--detach` to avoid (HIGH)
- [DevToolbox — git worktree "already checked out" fix](https://devtoolbox.dedyn.io/blog/git-worktree-already-checked-out-fix-guide) — detached worktree for PR review pattern (MEDIUM)
- [GitWorktree.org — GitHub worktrees / checkout](https://www.gitworktree.org/guides/github-worktrees) — `refs/pull/N/head` + `git worktree add --detach` pattern (MEDIUM)
- [GitHub community #156480 — handling rate limits for frequent polling](https://github.com/orgs/community/discussions/156480) — caching/ETags/backoff/Retry-After (MEDIUM)
- [GitHub Docs (Enterprise) — Best practices for API rate limits](https://docs.github.com/en/enterprise-server@3.20/admin/configuring-settings/configuring-user-applications-for-your-enterprise/best-practices-for-configuring-api-rate-limits) — secondary limits, 1s spacing for writes (HIGH)
- [GitHub Docs — Using the GitHub CLI across platforms / multiple accounts](https://docs.github.com/en/github-cli/github-cli/using-multiple-accounts) + [cli/cli discussion #4221](https://github.com/cli/cli/discussions/4221) — GH_HOST/GH_TOKEN, repo-context account selection, defaults to github.com without context (HIGH)
- [cli/cli #8845 — gh auth status wrong exit code](https://github.com/cli/cli/issues/8845) — exit-code unreliable; `--json` always 0 (HIGH)

---
*Pitfalls research for: gh-CLI PR review + PR-branch worktree checkout integration (Kangent v1.3)*
*Researched: 2026-06-13*
