# Stack Research

**Domain:** GitHub PR-review integration (read-only surfacing + worktree checkout) added to an existing single-binary Go + React local app
**Researched:** 2026-06-13
**Confidence:** HIGH (all `gh`/`git` commands, flags, and `--json` field lists below were run live against the host's `gh 2.82.0` and real GitHub repos, not recalled from training data)

## TL;DR for the roadmap author

- **No new Go dependency. No new npm dependency. No stored token.** Shell out to the already-authenticated host `gh` CLI exactly the way Kangent already shells out to `git` and `claude`. A new `internal/github` (or `internal/gh`) leaf package mirroring `internal/tmux` / `internal/quota` is the whole backend footprint.
- **List PRs:** `gh pr list -R <owner/repo> --search "review-requested:@me" --state open --json <fields>` — per-repo, rich card fields, drives the Review column.
- **Detect merge/close:** `gh pr view <n> -R <owner/repo> --json state,closed,closedAt,mergedAt,mergeCommit,headRefOid` — poll `state` (`OPEN`/`CLOSED`/`MERGED`).
- **Check out the PR branch into the pre-made worktree:** do NOT use `gh pr checkout` (it has no target-directory arg and is fork-remote-fragile). Instead resolve the ref yourself and use the existing git-worktree service:
  `git fetch origin refs/pull/<n>/head:refs/kangent/pr-<n>` then `git worktree add -b review/pr-<n> <dir> refs/kangent/pr-<n>`. Verified end-to-end; fork-agnostic.
- **Degrade:** `exec.LookPath("gh")` for presence; `gh auth status` (exit code **4** = needs auth, **1** = other auth issue) or `gh auth status --json hosts` for state. Treat like the quota indicator: best-effort, never break.
- **Rate limits:** `gh pr list`/`gh pr view` hit the **core** GraphQL/REST budget (5000/hr) — fine. `gh search prs` hits the **search** budget (**30/min**) — avoid it for polling.

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| `gh` CLI (host binary) | **2.82.0 floor** (host has exactly this) | All GitHub reads: list review-requested PRs, fetch single-PR state, auth detection | Already installed and authenticated on the host (`✓ Logged in to github.com`, scopes `repo, read:org, gist, project`). Mirrors Kangent's settled philosophy of shelling out to real CLIs (`git`, `claude`) instead of reimplementing them. No token ever touches Kangent's DB — `gh` owns credentials, refresh, and host config. This is the milestone's *settled* decision; research confirms it is also the technically correct one. |
| `os/exec` + system `git` (existing `worktree` service) | system git | Materialize the PR head into a worktree | Kangent already shells out to `git worktree add/remove/list --porcelain`. The PR-checkout flow is a *new variant of the existing worktree-create path*, not a new subsystem: fetch the universal pull ref, then `git worktree add` on it. `gh pr checkout` is deliberately **not** used (see "What NOT to Use"). |
| `internal/github` Go package (new) | n/a | Typed wrapper over `gh` invocations | One leaf package owning `exec.Command("gh", ...)`, JSON unmarshalling into Go structs, and the auth/degrade state machine — the same shape as the existing `internal/quota` (best-effort server proxy) and `internal/tmux` (CLI shell-out) packages. |

### Frontend

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| (existing) React 19 + TanStack Query + shadcn card/column | already in repo | Review column + PR cards | **Zero new npm deps.** The Review column reuses the existing board column/card components; the PR list is one more TanStack Query query (`useQuery(['pr-reviews', projectId])`) with the auto-poll-paused-when-hidden pattern already built for the quota indicator (Phase 7). PR cards are presentational variants of the existing task card. |

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `encoding/json` (stdlib) | stdlib | Unmarshal `gh --json` output into typed structs | Always — `gh` emits clean JSON; define a `PRSummary` struct matching the field list below. |
| `log/slog` (stdlib) | stdlib | Log `gh` failures at debug/warn without breaking the request | Already the project logger; degrade-don't-throw on every `gh` non-zero exit. |
| (existing) `goose` migration | v3.27.1 | Persist per-project linked-repo config + the global GitHub toggle + review-worktree metadata | One new migration: add `github_repo` (nullable) and `description` (nullable) to `projects`; a global setting row `github_integration_enabled` (default true); and a way to tag a worktree/task row as a PR review (store `pr_number`, `pr_repo`, `pr_head_oid` so the merge/close reaper knows what to poll). |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| `gh <cmd> --help` | Authoritative flag + `--json` field discovery | The JSON field lists below were copied from `gh pr list --help` / `gh pr view --help` on 2.82.0. Re-run `--help` when bumping gh; field names are stable but new ones are additive. |
| `gh help exit-codes` | Degradation logic | Documents the exit-code contract used in the auth-detection section. |

## The exact commands (verified live on gh 2.82.0)

### 1. List OPEN PRs awaiting your review, for ONE linked repo (drives the Review column)

**Use `gh pr list` with a `review-requested:@me` search qualifier — NOT `gh search prs`.**

```bash
gh pr list \
  --repo <owner>/<repo> \
  --search "review-requested:@me" \
  --state open \
  --limit 50 \
  --json number,title,author,headRefName,baseRefName,isDraft,reviewDecision,statusCheckRollup,additions,deletions,updatedAt,url,headRepositoryOwner,headRepository,isCrossRepository,headRefOid
```

Verified output (real PR, fields trimmed):
```json
{"number":1457,"title":"feat(metrics): ...","author":{"login":"alberto-miranda","is_bot":false},
 "headRefName":"COMP-1819/provider-request-metrics","baseRefName":"master","isDraft":false,
 "reviewDecision":"REVIEW_REQUIRED","additions":1993,"deletions":32,
 "headRefOid":"7b80ca5a514d68e57a88d8fa680da330d67c7624",
 "headRepositoryOwner":{"login":"seqeralabs"},"isCrossRepository":false,
 "updatedAt":"2026-06-12T17:58:09Z","url":"https://github.com/seqeralabs/fusion/pull/1457"}
```

Field-by-field (all present in `gh pr list --json` on 2.82.0; confirmed returning data):

| `--json` field | Card use |
|----------------|----------|
| `number` | PR id, also the arg to `gh pr view` / fetch |
| `title` | card title |
| `author` (object: `login`, `name`, `is_bot`, `id`) | "by @login" |
| `headRefName` | branch label; informs the local review branch name |
| `baseRefName` | "→ base" label |
| `isDraft` | draft badge / dim |
| `reviewDecision` | `REVIEW_REQUIRED` / `APPROVED` / `CHANGES_REQUESTED` / `""` — status pill |
| `statusCheckRollup` (array of CheckRun/StatusContext: `conclusion`, `status`, `name`, `workflowName`) | CI pass/fail/pending dot. Roll up in the backend to one of pass/fail/pending; do not ship the raw array to the browser. |
| `additions`, `deletions` | "+838 −2" diffstat |
| `updatedAt` | sort key + "updated Xh ago" |
| `url` | open-in-browser link (reuse `@xterm/addon-web-links` philosophy; just an `<a>`) |
| `headRepositoryOwner.login` | fork owner; with `isCrossRepository` distinguishes forks |
| `headRepository` (`name`; `nameWithOwner` is **empty for same-repo PRs**) | fork repo name when cross-repo |
| `isCrossRepository` | **the reliable fork flag** (`true` ⇒ head is a fork) |
| `headRefOid` | exact head commit SHA — pre-resolves the worktree checkout and lets the poller detect "PR got new commits" |

Why `gh pr list` and not `gh search prs`:
- `gh pr list` is scoped to one repo (the linked project repo) — exactly the Review column's scope.
- Its `--json` set is **rich** (includes `statusCheckRollup`, `additions/deletions`, `reviewDecision`, `headRefName`, `headRefOid`). `gh search prs --json` is **poor** by comparison: only `number, title, repository, state, isDraft, labels, author, url, createdAt, updatedAt, ...` — **no `headRefName`, no `statusCheckRollup`, no diffstat, no `reviewDecision`, no `headRefOid`** (verified from `gh search prs --help`). You'd then need a second `gh pr view` per card anyway.
- **Rate budget:** `gh pr list` consumes the **core** budget (verified `rate_limit.resources.core` = 5000/hr). `gh search prs` consumes the **search** budget = **30 requests/min** (verified `rate_limit.resources.search.limit` = 30) — far too tight for an auto-poller across multiple linked projects.

`review-requested:@me` is a GitHub server-side search qualifier (long predates the CLI). Verified it returns the authenticated user's review queue on a real repo. `@me` resolves server-side to the `gh`-authenticated user, so Kangent never needs to know the username.

### 2. Single-PR detail for the auto-cleanup-on-merge/close poll

```bash
gh pr view <number> \
  --repo <owner>/<repo> \
  --json number,state,closed,closedAt,mergedAt,mergeCommit,headRefOid,headRefName,baseRefName,isCrossRepository,headRepositoryOwner
```

Verified output:
```json
{"number":13642,"state":"OPEN","closed":false,"closedAt":null,"mergedAt":null,"mergeCommit":null,
 "headRefOid":"a74367d8...","headRefName":"...","baseRefName":"trunk","isCrossRepository":false}
```

**Decision logic for the reaper:** key off `state`, which is `OPEN` | `CLOSED` | `MERGED`.
- `state == "MERGED"` ⇒ merged (also `mergedAt`/`mergeCommit` populated).
- `state == "CLOSED"` ⇒ closed-without-merge (also `closed:true`, `closedAt` set, `mergedAt:null`).
- `state == "OPEN"` ⇒ still in review; keep the worktree.

**Caveat (verified):** `gh pr view --json` has a `merged`-related field? **No top-level boolean named `merged` is exposed** in the `--json` field list on 2.82.0 — the available fields are `state, closed, closedAt, mergedAt, mergeCommit, mergedBy` (no bare `merged`). Use `state` (or `mergedAt != null`) for the merged test; do **not** request a `merged` field (gh will error on an unknown field name). This corrects a common assumption.

The merge/close poll can ride the **same** auto-poll loop as the Review column list (paused-when-hidden), or run server-side on the existing Done-TTL-reaper-style background goroutine — either way, removal stays gated on the existing dirty-tree + running-session gates, and the branch is kept (consistent with the project's worktree-cleanup rules).

### 3. Check out the PR branch into the (already-created) worktree directory

**Recommendation: resolve the ref yourself and reuse the existing `git worktree` service. Do NOT use `gh pr checkout`.**

Why `gh pr checkout` is the wrong tool here (verified from `gh pr checkout --help`):
- Signature is `gh pr checkout [<number>|<url>|<branch>] [-b|--detach|--force|--recurse-submodules]`. **There is no target-directory argument.** It checks out *into the working directory of the repo it's run from* (`cmd.Dir`) by switching the current branch — it is built for the "I'm in my clone, put me on this PR" flow, not "materialize this PR into a separate, pre-created worktree dir."
- For **fork** PRs it adds the fork as a remote and fetches the contributor's branch — extra remote-management state in the user's real checkout, and failure modes (fork deleted, branch force-pushed) that you'd have to detect and recover from.
- It would mutate the *project's primary checkout*, which Kangent must never disturb.

**The recommended sequence (verified end-to-end against a real `cli/cli` PR, including the worktree-dir-must-be-created-by-us constraint):**

```bash
# Run with cmd.Dir = the project repo root (same as every other worktree op).
# <n>   = PR number from step 1
# <dir> = the worktree path Kangent assigns (under the configured worktree base)

# 1. Fetch the PR head into a Kangent-namespaced local ref.
#    refs/pull/<n>/head is a GitHub server-side ref that resolves to the PR's head
#    commit REGARDLESS of whether the head is a fork — no fork remote needed.
git fetch origin "refs/pull/<n>/head:refs/kangent/pr-<n>"

# 2. Create the worktree on that ref with a named local review branch.
#    (Use the existing worktree-create code path; this is just a different base ref
#     and a PR-derived branch name instead of task/<slug>-<id>.)
git worktree add -b "review/pr-<n>" "<dir>" "refs/kangent/pr-<n>"
```

Verified result: the new worktree's `HEAD` equals the PR's `headRefOid` from step 1 (`a74367d8...` matched exactly), it carries a clean named branch `review/pr-<n>`, and it appears normally in `git worktree list --porcelain` (the format the existing service already parses):
```
worktree /.../pr-worktree
HEAD a74367d8282228856f9edc6bf4d2631545cbb354
branch refs/heads/review/pr-13642
```

Notes / variants:
- **Detached vs named branch:** prefer `-b review/pr-<n>` (named) so the diff tab's merge-base logic and the worktree-list parser behave like a normal task; `--detach` worktrees show `detached HEAD` in porcelain and complicate the existing UI. (`git worktree add --detach <dir> <oid>` also works and is the pure-OID fallback if a branch-name collision occurs.)
- **Origin remote name:** Kangent should resolve the actual remote name rather than hard-coding `origin` (most clones use `origin`, but parse `git remote` or use `git rev-parse --abbrev-ref --symbolic-full-name @{u}` / `git remote get-url`). For GitHub repos the pull ref lives on whichever remote points at the PR's base repo.
- **Fork PRs need no special-casing** with this approach — that's the whole point of `refs/pull/<n>/head`. (Confirmed 7/20 sampled `cli/cli` open PRs were `isCrossRepository:true`; the pull-ref fetch is identical for them.)
- **Branch-name collision / re-open:** if `review/pr-<n>` already exists from a prior review, either reuse it (`git worktree add <dir> review/pr-<n>` without `-b`, then `git reset --hard refs/kangent/pr-<n>` if you want to fast-forward) or fall back to `--detach`. Keep it simple: the milestone says cleanup keeps the branch, so a re-open can reuse it.
- **Cleanup** uses the existing `git worktree remove` path (gated), keeping the `review/pr-<n>` branch and the `refs/kangent/pr-<n>` ref (or prune the ref — cheap either way).

### 4. Auth detection, presence, and rate-limit surfacing

**Presence:** `exec.LookPath("gh")` (same call-time pattern Kangent already uses for the tmux dropdown). Missing ⇒ hide all GitHub UI / report "gh not installed".

**Auth state — exit codes (verified via `gh help exit-codes` + `gh auth status --help`):**

```bash
gh auth status            # exit 0 = authed; exit 1 = an account has auth issues; exit 4 = requires authentication
gh auth status --active   # only the active account
gh auth status --json hosts   # ALWAYS exits 0 (unless fatal); inspect JSON for issues — better for programmatic use
```

- Exit **0** ⇒ authenticated; proceed.
- Exit **4** ⇒ "requires authentication" — show the degrade banner ("Run `gh auth login`").
- Exit **1** ⇒ an account has an auth problem (e.g. token scope/expiry) — degrade with the stderr message.
- `gh auth status --json hosts` is the cleaner programmatic probe: it exits 0 even on auth issues and returns a `hosts` object you can inspect, so you parse state rather than branch on exit codes. (Available on 2.82.0; introduced via cli/cli issue #8637.)

Verified on host: `gh auth status` ⇒ `✓ Logged in to github.com account ... (keyring)`, exit 0; scopes include `repo, read:org`.

**Rate-limit surfacing (optional, nice-to-have):**
```bash
gh api rate_limit --jq '.resources.core, .resources.search'
# core:   {"limit":5000,"remaining":...,"reset":<epoch>,"used":...}   <- pr list / pr view live here
# search: {"limit":30,  "remaining":...,"reset":<epoch>,"used":...}    <- gh search prs lives here (why we avoid it)
```
Every `gh api` (and the GraphQL calls behind `gh pr list/view`) also returns `X-RateLimit-Remaining` / `X-RateLimit-Reset` headers (verified via `gh api -i`); if you ever call `gh api` directly you can read them with `-i`. For the degrade UX, mirroring the quota indicator's "best-effort, show stale, back off on failure" model is sufficient — explicit rate-limit polling is optional.

## Installation

```bash
# Nothing to install. gh is a HOST dependency (soft), already present:
gh --version   # gh version 2.82.0 (2025-10-15)   <-- verified floor

# No Go module additions:
#   the new internal/github package uses only os/exec, encoding/json, log/slog (all stdlib)
#   and reuses the existing git-worktree service.

# No npm additions:
#   Review column + PR cards reuse existing shadcn card/column + TanStack Query.
```

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| Shell out to `gh` | `github.com/google/go-github` v74 (REST) or `shurcooL/githubv4` (GraphQL) + a Go OAuth/token flow | Only if Kangent ever needed to **store its own token**, run **without `gh` installed**, or do high-volume API work. None apply: it's single-user, local, `gh` is present and authed, and the whole project philosophy is "drive real CLIs." A Go GitHub lib would force Kangent to own credential storage/refresh — the exact thing the milestone explicitly rules out. |
| `gh pr list --search "review-requested:@me"` (per repo) | `gh search prs --review-requested=@me` (cross-repo) | If a future milestone wants a *global* "all my review requests across every repo" view independent of linked projects. For the per-linked-project Review column it's wrong: poorer JSON fields, and the 30/min search rate budget. |
| Self-resolve `refs/pull/<n>/head` + `git worktree add` | `gh pr checkout <n>` (with `cmd.Dir` = a fresh clone) | If you wanted gh to manage fork remotes for you AND you were operating in a normal single-checkout clone (not a worktree). Not applicable: Kangent pre-creates the worktree dir and must not touch the primary checkout. |
| Named review branch `review/pr-<n>` | `git worktree add --detach <dir> <headRefOid>` | If a branch name collides or you explicitly want a throwaway detached review with no local branch. Detached HEAD complicates the existing porcelain parser and diff/merge-base UX, so named branch is the default. |
| `gh auth status` exit codes | `gh auth status --json hosts` | Use the `--json` form when you want to *parse* state without exit-code branching (it always exits 0). Use plain exit codes for a quick "is it usable" gate. Both work on 2.82.0; pick one consistently. |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `go-github` / `githubv4` / any Go GitHub SDK | Forces Kangent to own a token (storage + refresh) — explicitly out of scope; contradicts the "shell out to real CLIs, no stored creds" philosophy; adds a dependency for zero benefit at single-user localhost scale. | `gh` CLI shell-out |
| Storing a PAT / OAuth token in SQLite or env | Out of scope per PROJECT.md; `gh` already holds creds in the OS keyring and handles refresh/SSO. | Let `gh` own auth |
| `gh pr checkout` for the worktree flow | No target-dir arg (checks out into `cmd.Dir`, mutating the primary checkout); adds fork remotes; harder to make idempotent for a pre-created worktree. | `git fetch origin refs/pull/<n>/head:<ref>` + existing `git worktree add` |
| `gh search prs` for the per-repo polling column | Sparse `--json` fields (no `headRefName`/`statusCheckRollup`/diffstat/`reviewDecision`/`headRefOid`) ⇒ needs a second call per card; consumes the **30/min** search rate budget. | `gh pr list -R <repo> --search "review-requested:@me" --json <rich set>` |
| Requesting a `merged` boolean field from `gh pr view --json` | No top-level `merged` field exists in 2.82.0's field list; gh errors on unknown field names. | `state` (`MERGED`/`CLOSED`/`OPEN`) or `mergedAt != null` |
| Hard-coding the remote as `origin` blindly | Most clones use `origin`, but Kangent points at user repos that may differ. | Resolve the remote (`git remote`, upstream of base branch) before the pull-ref fetch |
| Parsing human-readable `gh`/`git` output | Same trap the project already avoids for `git worktree list`. | Always `--json` (gh) / `--porcelain` (git) |
| New npm DnD/state libs for the Review column | The column is a read-only list, not draggable; PR cards never enter the kanban flow (settled decision). | Reuse existing card/column components + one TanStack Query query |

## Stack Patterns by Variant

**If the linked repo's PR head is a fork (`isCrossRepository: true`):**
- No special handling. `git fetch origin refs/pull/<n>/head` resolves the fork's head commit via the base repo's server-side pull ref. (Verified across 7 real fork PRs.) Use `headRepositoryOwner.login` only for display ("from @forkowner").

**If `gh` is missing or unauthenticated:**
- Degrade like the quota indicator: `LookPath` fails ⇒ hide GitHub UI; `gh auth status` non-zero ⇒ show a one-line "Connect GitHub via `gh auth login`" notice. Never block the board; the global toggle (default on) plus this runtime check are independent gates.

**If a project has no linked GitHub repo:**
- No Review column for that project. The column is driven entirely by the per-project `github_repo` config (new nullable column) AND the global `github_integration_enabled` setting.

**If the same PR is re-reviewed after a prior cleanup (branch kept):**
- The `review/pr-<n>` branch still exists; re-attach a worktree to it (skip `-b`) or `--detach` to the fresh `headRefOid`. Refetch `refs/pull/<n>/head` first so you get any new commits (compare `headRefOid` to the stored one).

## Version Compatibility

| Component | Compatible With | Notes |
|-----------|-----------------|-------|
| `gh` 2.82.0 (host floor) | `gh pr list --search --json {…,statusCheckRollup,headRefOid,reviewDecision,headRepositoryOwner,isCrossRepository}` | All fields/flags **verified returning data** on 2.82.0. Field set is additive across releases — re-check `gh pr list --help` only if you adopt newer fields. |
| `gh` 2.82.0 | `gh auth status --json hosts`, exit codes 0/1/4 | `--json hosts` present and working; exit-code contract per `gh help exit-codes`. |
| `gh pr checkout` (not used) | — | Documented for completeness; intentionally avoided. |
| `git worktree add -b <branch> <dir> <ref>` | any modern git (worktrees stable since git 2.5; `--porcelain` list parsing already in use) | The PR-checkout path is a parameter change to the existing worktree service, not a new git feature. |
| `refs/pull/<n>/head` fetch | GitHub.com (and GHES) | Server-side ref, fork-agnostic; verified fetch returns the PR's `headRefOid`. |
| New code (`internal/github`) | stdlib only (`os/exec`, `encoding/json`, `log/slog`) | No new go.mod entries; no `CGO`. |
| Frontend | existing React 19 / TanStack Query 5 / shadcn / Tailwind 4 | No new npm deps. |

## Integration points with existing code (for the roadmap author)

- **New leaf package** `internal/github` modeled on `internal/tmux` (CLI shell-out) + `internal/quota` (best-effort degrade): functions like `ListReviewRequested(ctx, repo) ([]PRSummary, error)`, `ViewPR(ctx, repo, n) (PRState, error)`, `AuthStatus(ctx) (AuthState, error)`, `Available() bool` (LookPath).
- **Worktree service reuse:** add a "checkout existing ref into a worktree" variant alongside the current `task/<slug>-<id>` create path — same `git worktree add` machinery, different base ref + branch name (`review/pr-<n>`), preceded by the pull-ref `git fetch`. Cleanup uses the existing gated `git worktree remove`.
- **DB (one goose migration):** `projects.github_repo` (nullable, `owner/repo`), `projects.description` (nullable); global setting `github_integration_enabled` (default `true`, served via the existing settings API); review-task rows tagged with `pr_number` + `pr_repo` + `pr_head_oid` so the merge/close poller knows what to check and can detect new commits.
- **Polling:** reuse the Phase-7 auto-poll-paused-when-hidden pattern for the Review column (frontend TanStack Query) and the Phase-9 background-goroutine pattern (Done-TTL reaper) for server-side merge/close cleanup — both already exist.
- **API surface:** ~2 new read endpoints (`GET /api/projects/{id}/pr-reviews`, optionally `GET /api/github/status`) + one action to open a PR as a review workspace (creates the worktree + a task-like view). No write endpoints (settled: no in-app GitHub writes).

## Sources

- Live execution on host `gh 2.82.0 (2025-10-15)` — `gh pr list --help`, `gh pr view --help`, `gh pr checkout --help`, `gh search prs --help`, `gh auth status --help`, `gh help exit-codes` (authoritative `--json` field lists + flags + exit-code contract) — **HIGH**
- Live `gh pr list -R seqeralabs/fusion --search "review-requested:@me" --json …` and `gh pr list -R cli/cli …` against real repos (confirmed every recommended field returns data; confirmed `isCrossRepository` fork detection on 7 real fork PRs) — **HIGH**
- Live `gh pr view 13642 -R cli/cli --json state,closed,closedAt,mergedAt,mergeCommit,headRefOid,…` (confirmed merge/close fields; confirmed no top-level `merged` field) — **HIGH**
- Live end-to-end worktree proof: `git fetch origin refs/pull/13642/head:refs/kangent/pr-13642` + `git worktree add -b review/pr-13642 <dir> refs/kangent/pr-13642` against a real `cli/cli` clone — resulting worktree HEAD matched the PR's `headRefOid` exactly; `git worktree list --porcelain` clean — **HIGH**
- Live `gh api rate_limit --jq '.resources.core, .resources.search'` + `gh api rate_limit -i` (core 5000/hr vs search 30/min; `X-RateLimit-*` headers) — **HIGH**
- [cli.github.com/manual/gh_pr_list](https://cli.github.com/manual/gh_pr_list), [gh_pr_view](https://cli.github.com/manual/gh_pr_view), [gh_search_prs](https://cli.github.com/manual/gh_search_prs), [gh_auth_status](https://cli.github.com/manual/gh_auth_status) — official manual corroboration — **MEDIUM**
- [cli/cli#8637](https://github.com/cli/cli/issues/8637) — `gh auth status --json` provenance — **MEDIUM**

---
*Stack research for: GitHub PR-review (`gh`-CLI) integration in a Go + React local single-binary app*
*Researched: 2026-06-13*
