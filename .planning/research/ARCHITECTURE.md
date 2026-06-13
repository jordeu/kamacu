# Architecture Research

**Domain:** GitHub PR review integration into an existing Go + React local app (Kangent v1.3)
**Researched:** 2026-06-13
**Confidence:** HIGH (grounded in the real v1.2 source: every integration point below names the actual file/function/table/migration it touches; `gh` 2.82.0 command shapes were verified live on this host)

---

## Executive recommendation up front

**The key modeling question is settled: model a PR review as a `tasks` row with a `kind`/`source` discriminator, NOT a separate `pr_reviews` table.** Rationale and the full reuse-vs-fork map are in [§1](#1-the-modeling-question-pr-review-vs-task). Every other section flows from that decision.

The integration is overwhelmingly **reuse, not new machinery**:
- The session manager is keyed on `TaskID int64` and nothing else — a PR review must be a task to get sessions for free.
- The diff endpoint, worktree gates, cleanup dialog, task view, and reaper are all `taskID`-addressed and become available the moment a PR review *is* a task.
- The genuinely new code is small: one `internal/github` leaf package, one migration adding columns to `projects` + `tasks`, a poll-backed PR-list endpoint pair, a worktree "checkout existing branch" verb, and a frontend Review column + project-config UI.

---

## Standard Architecture

### System Overview — where the new pieces sit

```
┌──────────────────────────────────────────────────────────────────────┐
│  FRONTEND (React + TanStack Query)                                     │
│  ┌────────────┐  ┌──────────────┐  ┌───────────────┐  ┌────────────┐ │
│  │ Project    │  │  Board        │  │ Review column │  │ TaskPage   │ │
│  │ config UI  │  │  (kanban)     │  │ (NEW, gated)  │  │ (REUSED    │ │
│  │ (NEW)      │  │               │  │ usePullReqs() │  │  for PRs)  │ │
│  └─────┬──────┘  └──────┬───────┘  └──────┬────────┘  └─────┬──────┘ │
│        │ desc/repo      │ tasks query     │ poll-paused-hidden        │
│        │                │                 │ + manual refresh          │
├────────┼────────────────┼─────────────────┼──────────────┼──────────┤
│  REST API (stdlib ServeMux)                                           │
│  PATCH /api/projects/{id}      GET  /api/projects/{id}/pull-requests  │
│  (desc + github_repo, NEW)     POST /api/projects/{id}/pull-requests/ │
│                                       {n}/review   (open a review)     │
│  GET /api/settings (github_integration toggle gates list endpoint)    │
├──────────────────────────────────────────────────────────────────────┤
│  SERVICES (leaf packages — same shape as worktree/quota/tmux)         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌───────────┐ │
│  │ internal/    │  │ internal/    │  │ internal/    │  │ internal/ │ │
│  │ github (NEW) │  │ worktree     │  │ diff         │  │ session   │ │
│  │ gh shell-out │  │ +CheckoutPR  │  │ (REUSED, PR  │  │ (REUSED,  │ │
│  │ +cache/poll  │  │  (NEW verb)  │  │  base ref)   │  │  taskID)  │ │
│  └──────┬───────┘  └──────┬───────┘  └──────────────┘  └───────────┘ │
│         │ gh CLI          │ git CLI                                    │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │ internal/reaper (REUSED + EXTENDED):  Done-TTL reap            │   │
│  │   + NEW: PR-state reconcile loop → gated worktree auto-remove  │   │
│  └──────────────────────────────────────────────────────────────┘   │
├──────────────────────────────────────────────────────────────────────┤
│  STORAGE (SQLite, goose migration 00007)                              │
│  projects: + description TEXT, + github_repo TEXT                     │
│  tasks:    + source TEXT NOT NULL DEFAULT 'manual'                    │
│            + pr_number INTEGER, + pr_base_ref TEXT (PR-only)          │
│  settings: github_integration KV key (code default 'on')             │
└──────────────────────────────────────────────────────────────────────┘
```

### Component Responsibilities

| Component | Responsibility | New / Reused / Extended |
|-----------|----------------|--------------------------|
| `internal/github` | Shell out to `gh`; list review-requested PRs; get a PR's state; cache + degrade like quota | **NEW** |
| `projects` table cols | Persist per-project description + linked `owner/name` | **NEW columns** (migration 00007) |
| `tasks` table cols | Discriminate manual vs PR-backed; hold PR number + base ref | **NEW columns** (migration 00007) |
| `internal/worktree` | Provision a worktree; add a `CheckoutPR` path (checkout existing branch, not `add -b`) | **EXTENDED** |
| `internal/diff` `Compute(wt, base)` | Unchanged — already takes an explicit base ref | **REUSED as-is** |
| `internal/session` `Manager` | Owns PTYs keyed on `taskID` | **REUSED as-is** (PR review = a task → free) |
| `internal/reaper` | Done-TTL session reaping; gains a PR-state reconcile + gated worktree auto-remove pass | **EXTENDED** |
| TaskPage / TaskTabs | Full task view with Agent/Description/Diff/bash tabs | **REUSED** (PR view IS a task view) |
| Review column (frontend) | Render review-requested PR cards, poll paused-when-hidden, manual refresh | **NEW** (clone of the QuotaIndicator poll pattern) |
| `github_integration` setting | Global on/off; hides all GitHub UI and gates the API | **NEW KV key** |

---

## 1. The modeling question: PR review vs task

### Recommendation: a `tasks` row with a `source` discriminator (option **b**)

Add to `tasks`:
- `source TEXT NOT NULL DEFAULT 'manual'` — values `'manual'` | `'github_pr'`
- `pr_number INTEGER` — NULL for manual tasks
- `pr_base_ref TEXT` — the PR's base branch (e.g. `main`), NULL for manual tasks

Board queries get a `WHERE source = 'manual'` filter so PR reviews never appear in any kanban column.

### Why this, not a separate `pr_reviews` table (option a)

The decision is forced by one hard fact in the existing code: **the session manager addresses everything by `TaskID int64` and nothing else.**

- `Manager.Spawn(SpawnOpts{TaskID, Cwd, ...})`, `ListByTask(taskID)`, `StopAllForTask(taskID)`, `HasLiveTmux(name)` — all keyed on a task id (`internal/session/manager.go`).
- `tmux_sessions.task_id` has an `FK REFERENCES tasks(id)` and `UNIQUE(task_id, n)` (`migration 00005`). tmux session names are minted as `kangent-<task>-<n>` (`internal/api/sessions.go`).
- `tasks.claude_session_id` (`migration 00003`) holds the agent's resumable session id.
- The diff endpoint reads `tasks.worktree_path` and the project's `repo_path` (`internal/api/diffs.go`).
- The reaper queries `tasks` and calls `StopAllForTask(id)` (`internal/reaper/reaper.go`).
- The whole frontend task view is `useTask(taskId)` / `useSessions(taskId)` (`web/src/pages/TaskPage.tsx`).

A `pr_reviews` table would require **forking every one of these** to accept a second id space: a second FK column on `tmux_sessions`, a `SpawnOpts.PRReviewID` branch in the manager, a parallel diff handler, a parallel session-list handler, a parallel cleanup dialog, and a parallel task view. That is a large, bug-prone surface for zero benefit — a PR review genuinely *is* a task (it owns a worktree + agent + bash tabs + diff); it just isn't a *kanban* task.

Option (b) makes a PR review a first-class task in every subsystem that matters and excludes it from exactly one place — the board — with a single `WHERE` clause. The discriminator is the smallest possible change that satisfies "owns everything a task owns, but is not on the board."

### What gets reused vs forked under option (b)

| Subsystem | Treatment | Detail |
|-----------|-----------|--------|
| `internal/session` Manager | **Reused untouched** | PR review has a `tasks.id`; `Spawn`/`ListByTask`/`StopAllForTask` work verbatim. |
| `tmux_sessions` table + naming | **Reused untouched** | `kangent-<task>-<n>` is already a task id; the FK already points at `tasks`. |
| `internal/diff` `Compute` | **Reused untouched** | Already `Compute(ctx, wt, base)` with an explicit base param. |
| Diff endpoint `GET /api/tasks/{id}/diff` | **Reused, one branch** | Base resolution forks (see [§4](#4-worktree-for-pr-and-the-correct-diff-base)): PR tasks use `tasks.pr_base_ref`, manual tasks keep `ResolveBase`. |
| TaskPage / TaskTabs / Agent/Bash/Diff tabs | **Reused untouched** | Open `/projects/{pid}/tasks/{taskId}` for the PR's task id; identical view. |
| Worktree gates (sessions, dirty), cleanup dialog | **Reused untouched** | `worktreeHandlers` works on any `tasks.id`. |
| Session/agent/worktree REST handlers | **Reused untouched** | All `{id}` = task id. |
| Worktree *creation* | **Forked verb** | Manual tasks `worktree add -b`; PR tasks check out the existing PR branch (see [§4](#4-worktree-for-pr-and-the-correct-diff-base)). |
| Board list query | **Filtered** | `GET /api/projects/{id}/tasks` adds `WHERE source = 'manual'`. |
| `move` endpoint | **Guarded** | PR tasks must never receive a `/move` (they aren't on the board); reject with 409 if `source != 'manual'` — defense in depth. |
| `provisionWorktree` | **Branched** | Detects `source='github_pr'` and routes to the checkout path. |

### One caveat to flag for the roadmap

Several existing queries (`listByProject`, board mutations) assume *every* row in `tasks` is a board card. **Audit every `SELECT ... FROM tasks` for the missing `source` filter** — a forgotten filter would leak PR reviews onto the board. This is the single highest-risk regression of the whole milestone; treat it as a checklist item per query. Known sites today: `listByProject` (`tasks.go`), and any future stats/cycle-time consumer of the `*_at` columns.

---

## 2. Per-project config storage

### Migration 00007 adds columns to `projects` (and `tasks`)

```sql
-- +goose Up
ALTER TABLE projects ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN github_repo TEXT;   -- nullable: NULL = not linked

ALTER TABLE tasks ADD COLUMN source      TEXT NOT NULL DEFAULT 'manual';
ALTER TABLE tasks ADD COLUMN pr_number   INTEGER;    -- NULL for manual tasks
ALTER TABLE tasks ADD COLUMN pr_base_ref TEXT;       -- NULL for manual tasks
```

- `description` follows the migration-00006 pattern exactly: nullable-or-defaulted ADD COLUMN, one statement each (SQLite requirement, already established).
- `github_repo` is **nullable** so "not linked" is `NULL`, distinct from "linked to empty string". This mirrors how the codebase treats `worktree_path`/`branch` nullability for state derivation (`tasks.go` D-26 comment).
- Reuse the `projectColumns` constant pattern (`projects.go`): extend it and `scanProject` together so every read includes the new fields.

### What "linked repo" stores: `owner/name`, validated via `gh repo view`

Store the canonical **`owner/name`** string (e.g. `cli/cli`), not a full URL.

- `gh` accepts `owner/name` directly via `-R/--repo` on every command, so storing the URL would force parsing on every call. Verified: `gh pr list -R cli/cli ...` and `gh repo view cli/cli` both work.
- Validation mirrors `validateRepoPath` (`projects.go`) — same posture, different tool: on PATCH, run `gh repo view <owner/name> --json nameWithOwner` (verified live). Exit 0 with a matching `nameWithOwner` → accept and **store the canonical `nameWithOwner` gh returns** (normalizes case / casing drift). Nonzero (verified failure: `GraphQL: Could not resolve to a Repository...`) → reject with a 400 carrying gh's trimmed stderr, exactly as `worktree.gitRun` shapes git errors.
- **Degrade-don't-break for validation:** if `gh` is absent/unauthenticated, do NOT hard-fail the PATCH — that would make linking impossible on a host where the integration is simply dormant. Accept a syntactically valid `owner/name` (regex `^[^/\s]+/[^/\s]+$`) and let the list endpoint degrade later. This matches the quota philosophy: the soft dependency never blocks core flows. (Decision to confirm with the roadmap author: strict-validate-when-gh-present vs. always-syntactic. Recommend: validate via `gh repo view` when `gh` is present, fall back to syntactic when it isn't.)

### Where it surfaces

- `PATCH /api/projects/{id}` already exists for rename (`projects.go`); extend its request struct with optional `description` and `github_repo` (pointer fields → "field omitted ≠ clear"). Accept clearing `github_repo` to `null` (unlink).
- Frontend: a project-config surface (a dialog opened from `ProjectMenu`, or a small project settings panel) using the existing `useSaveSetting`-style per-field mutation pattern. No new patterns required.

---

## 3. The `internal/github` leaf package

### A new leaf package mirroring `internal/quota` + `internal/tmux`

`internal/github/github.go` — stdlib + `os/exec` only, arg-array exec (never a shell), bounded context per call, exit-0-only success with trimmed stderr surfaced. This is the established leaf-package contract shared by `worktree.gitRun`, `tmux.run`, and `diff.run`.

### Surface

```go
// PullRequest is the /api/.../pull-requests contract; JSON tags ARE the
// frontend contract (mirrors quota.Window).
type PullRequest struct {
    Number     int    `json:"number"`
    Title      string `json:"title"`
    Author     string `json:"author"`      // author.login
    HeadRef    string `json:"headRef"`     // headRefName
    BaseRef    string `json:"baseRef"`     // baseRefName
    URL        string `json:"url"`
    UpdatedAt  string `json:"updatedAt"`
    IsDraft    bool   `json:"isDraft"`
    IsFork     bool   `json:"isFork"`      // isCrossRepository (see §4 caveat)
}

// ListReviewRequestedPRs runs:
//   gh pr list -R <repo> --search "is:open review-requested:@me"
//     --json number,title,author,headRefName,baseRefName,url,updatedAt,isDraft,isCrossRepository
// Verified live: returns a JSON array ([] when none). Parse into []PullRequest.
func (c *Client) ListReviewRequestedPRs(ctx context.Context, repo string) ([]PullRequest, error)

// PRState returns OPEN | CLOSED | MERGED for one PR. Runs:
//   gh pr view <n> -R <repo> --json state
// Verified: state is UPPERCASE OPEN/CLOSED/MERGED; `merged` is NOT a json
// field — derive merged from state == "MERGED".
func (c *Client) PRState(ctx context.Context, repo string, number int) (string, error)

// ValidateRepo confirms a repo is resolvable (PATCH-time link validation).
//   gh repo view <repo> --json nameWithOwner  -> returns canonical name
func (c *Client) ValidateRepo(ctx context.Context, repo string) (string, error)
```

**No `gh`-based checkout helper here.** PR checkout into a worktree is a *git* operation, not a `gh` one (see [§4](#4-worktree-for-pr-and-the-correct-diff-base) — `gh pr checkout` checks out into the repo's current branch, which is wrong for a worktree). The checkout verb belongs in `internal/worktree`, keeping `internal/github` purely a read-only `gh` query surface.

### Caching / poll / degradation — reuse the quota Service shape verbatim

Model the `Service` on `quota.Service` (`internal/quota/quota.go`), which is the codebase's proven best-effort proxy pattern:

- **Demand-driven cache, keyed by repo** (quota keys by token fingerprint; here key by `owner/name`). A `map[repo]cacheEntry` behind a mutex, each entry holding last-good `[]PullRequest`, `fetchedAt`, `lastAttempt`, `failures`, `lastErr`.
- **TTL + attempt floor + backoff**: copy quota's `cacheTTL` (60s, matches the auto-poll cadence), `attemptFloor` (10s hard floor even on `?refresh=1`), and consecutive-failure drop (`maxFailures=3`). `gh` has its own rate limits; the floor protects against refresh-spam.
- **Degradation states** mirror quota's `Result.State`: `"ok" | "no_gh" | "auth_required" | "error"`, plus `Stale bool`. Map:
  - `exec.LookPath("gh")` fails → `no_gh` (first-class state, never an error to surface).
  - `gh` exits with an auth message (stderr contains auth hints / exit on `gh auth status` probe) → `auth_required`.
  - Any other nonzero / parse error → `error`, keep last-good windows until `maxFailures`.
- **The endpoint is always 200** with the state in the body — identical to `UsageRoutes` (`internal/api/usage.go`). The browser never sees a 5xx for a dormant integration; the Review column just shows an empty/degraded state.
- **No ticker in the service.** Like quota, the browser's poll is the only trigger (auto-poll paused when hidden). The one exception is the PR-state reconcile, which runs in the reaper goroutine (see [§5](#5-auto-cleanup-on-mergeclose)) — that reads state directly, separate from the list cache.

### REST endpoints

```
GET  /api/projects/{id}/pull-requests            -> { state, stale, fetchedAt, pullRequests: [...] }
GET  /api/projects/{id}/pull-requests?refresh=1  -> bypass TTL (10s floor still applies)
POST /api/projects/{id}/pull-requests/{n}/review -> { task: Task }  (open/find the review task)
```

- The two GETs follow `UsageRoutes` exactly (always 200, state carries degradation). They first check the `github_integration` toggle ([§6](#6-global-toggle-plumbing)) and that the project has a `github_repo`; if either is off/absent, return `{state:"disabled"}` / `{state:"not_linked"}` — never an error.
- `POST .../{n}/review` is the "click a PR card" action. It is **idempotent / find-or-create**: if a `tasks` row already exists for `(project_id, pr_number)` return it (and re-provision its worktree if missing, reusing the existing `worktree.create` Retry path); otherwise INSERT a `source='github_pr'` task with `pr_number`, `pr_base_ref` (from the cached PR's `baseRef`), a generated title (e.g. `#<n> <pr title>`), then provision the PR worktree ([§4](#4-worktree-for-pr-and-the-correct-diff-base)). Add a `UNIQUE(project_id, pr_number)` index for PR tasks to make find-or-create race-safe at single-user scale.
- Reuse `writeJSON`/`writeError`/`pathID` (`respond.go`, `projects.go`) — no new HTTP helpers.

### Wiring (`cmd/kangent/main.go`)

Construct `ghSvc := github.New(github.Config{GHBin: ...})` next to `quotaSvc := quota.New(...)`, and register `api.PullRequestRoutes(mux, db, ghSvc, wtSvc, mgr)` next to `api.UsageRoutes`. Same shape as the existing service construction + route registration block.

---

## 4. Worktree for PR, and the correct diff base

### The checkout problem (verified `gh` behavior)

`gh pr checkout <n>` checks out the PR branch **into the current branch of the repository it runs in** — it fetches the PR head and switches HEAD. It does **not** create a worktree, and run inside the main repo it would hijack the user's checkout. So the existing `worktree add -b <branch> <path> <base>` path is wrong (it creates a *new* branch), and naive `gh pr checkout` is wrong (it mutates the main checkout). **The correct mechanic is git, in two steps**, and it belongs in `internal/worktree` as a new verb.

### New `worktree` verb: `CheckoutPR`

```go
// CheckoutPR provisions a worktree at `path` checked out on a PR head ref.
// Same mutex/MkdirAll/path-precheck discipline as Create. Two-step:
//   1. Fetch the PR head into a local ref (no network beyond this fetch — a
//      deliberate exception to worktree's "never fetch" D-24 rule, because a
//      PR review's whole point is fetching someone else's branch):
//        git -C <repo> fetch origin pull/<n>/head:<localBranch>
//      (works for fork PRs too — pull/<n>/head is the merged head ref on the
//      base repo, sidestepping the isCrossRepository fork-remote problem.)
//   2. Add a worktree on that local branch (NOT -b — the branch now exists):
//        git -C <repo> worktree add <path> <localBranch>
func (s *Service) CheckoutPR(ctx context.Context, repo, localBranch, path string, prNumber int) error
```

- Use `refs/pull/<n>/head` as the fetch source — this is the canonical GitHub PR-head ref reachable on the *base* repo, so it works uniformly for same-repo and fork PRs without configuring a fork remote. This is more robust than `gh pr checkout` for the worktree case and avoids `gh`'s current-checkout mutation entirely.
- `localBranch` should be a Kangent-namespaced ref (e.g. `kangent-pr/<n>`) so it never collides with a branch the user already has, and so the existing branch-reuse path in `Create` (already handles "branch exists, add without -b") composes cleanly on a re-review.
- This is a **deliberate, scoped exception to worktree's "never fetch" invariant (D-24)** — document it in the package doc comment, the same way the milestone scopes the "worktrees can be auto-removed" exception. The fetch is the one network call the package makes, gated to the PR path.

### `provisionWorktree` branches on `source`

`provisionWorktree` (`tasks.go`) is the single choke point for both creation paths today. Extend it: when the task's `source == 'github_pr'`, call `wt.CheckoutPR(...)` instead of the slug/template/`ResolveBase`/`Create` chain. Everything else (the D-25 failure-into-`worktree_error` handling, the 30s timeout, the `EnsureSubmodules` best-effort) is reused verbatim. The PR's base ref (`pr_base_ref`) is captured at task creation from the cached PR data, so no extra `gh` call is needed at provision time.

### Diff base: read `pr_base_ref`, do not `ResolveBase`

The diff endpoint (`diffs.go`) currently calls `wt.ResolveBase(repo)` to get the project default branch's tip, then `diff.Compute(wt, base)`. For a PR review the correct base is the **PR's base branch** (e.g. `main` the PR targets), not the project default — and `diff.Compute` already takes the base as a parameter, so **no change to `internal/diff` is needed.**

The only change is in the handler: select the base by `source`.

```go
// in diffHandlers.get, after loading the task (now also select source, pr_base_ref):
var base string
if source == "github_pr" && prBaseRef.Valid {
    // The diff is the PR's changes vs its own base branch's merge-base.
    base = prBaseRef.String        // e.g. "main" / "origin/main"
} else {
    base, err = h.wt.ResolveBase(r.Context(), repo)   // manual tasks unchanged
}
d, err := diff.Compute(r.Context(), path, base)
```

`diff.Compute` does `merge-base base HEAD` then three-dot semantics (`diffs.go` / `diff.go`), which is exactly right for "what this PR changed vs its base." Use `origin/<pr_base_ref>` if the local base ref may be stale; prefer the local ref when it exists (mirror `ResolveBase`'s local-then-remote chain). Recommend storing `pr_base_ref` as the plain branch name and resolving local-vs-remote at diff time, reusing `ResolveBase`'s logic shape.

---

## 5. Auto-cleanup on merge/close

### Who detects the state change: the reaper goroutine, a new pass

The codebase has exactly one background goroutine — the Done-TTL reaper (`internal/reaper`, started in `main.go` via `go reaper.New(db, mgr).Run(ctx)`). The milestone explicitly says to model PR-merge cleanup on "the gated-cleanup + the reaper background-goroutine pattern." So **add a second pass to the reaper's tick**, not a new goroutine.

The Review-column *list* poll (browser-driven, 60s, paused when hidden) is the wrong place to detect merges: it stops when the tab is hidden, and it only sees *open* review-requested PRs (a merged PR drops out of the list — the list can tell a PR *vanished*, but not authoritatively *why*, and only while the tab is open). Authoritative state detection must be server-side and always-on → the reaper.

### How the reconcile pass works

Extend `reaper.reapOnce` (or add `reconcilePRsOnce` called from the same `Run` ticker) to:

1. `SELECT id, project_id, pr_number FROM tasks WHERE source='github_pr' AND worktree_path IS NOT NULL` joined to `projects.github_repo`.
2. For each, call `ghSvc.PRState(ctx, repo, prNumber)` (verified: returns `OPEN`/`CLOSED`/`MERGED`). The reaper gains a dependency on a `PRStateGetter` interface — define it *locally in the reaper package* exactly like the existing `SessionStopper` interface, so tests inject a spy and the real `*github.Service` satisfies it. This keeps the reaper's clean test seam.
3. If state is `MERGED` or `CLOSED`, attempt **the same gated cleanup the manual path already enforces** (`worktreeHandlers.remove` logic): check dirty-tree (`wt.DirtyCount`) and running/detached sessions (the folded `cleanupSessionCount` logic), and **only auto-remove when both gates pass clean**. If dirty or sessions are live, **skip and leave it** — the user reviews and cleans up manually. The branch is always kept (D-34). This is the milestone's deliberate exception ("worktrees CAN be auto-removed on merge/close") reconciled with the existing safety gates: auto-removal is *opt-out-by-state* (dirty/busy worktrees are never bulldozed).

### Reuse the gated-cleanup logic — extract it once

Today the gate logic (sessions count → dirty count → `StopAllForTask` → kill detached tmux → `wt.Remove` → null the columns) lives inline in `worktreeHandlers.remove` (`worktrees.go`). To avoid duplicating it in the reaper, **extract a shared helper** — e.g. `worktree`-adjacent `cleanupWorktreeGated(ctx, db, wt, mgr, tmuxClient, taskID, force=false) (removed bool, reason string)` — and call it from both the HTTP handler and the reaper pass. The reaper passes `force=false` always (auto-removal never forces past a dirty gate). This is the cleanest reconciliation of "new auto-removal" with "existing gated cleanup": one code path, two callers.

### What the reaper must NOT do here

- Never delete the branch (D-34 — global invariant).
- Never force past the dirty gate or the sessions gate (auto-removal is conservative).
- Never touch manual (`source='manual'`) tasks' worktrees — the Done-TTL reaper already keeps "worktrees never auto-removed" for those (D-87). The PR pass is gated on `source='github_pr'` exactly the way the Done pass is gated on `status='done'`.
- Reconcile is best-effort: a `gh` failure (`no_gh`/`error`) on a given tick is a warn-and-skip, never a crash — same posture as `reapOnce` today.

### Tick cadence

The existing 10-minute reaper tick is fine for merge cleanup (a worktree lingering a few extra minutes after merge is harmless). Keep one ticker; run both passes per tick. If a snappier feel is wanted later, the Review column's manual refresh can additionally trigger a targeted reconcile — but that's optional polish, not core.

---

## 6. Global toggle plumbing

### Settings key → API gating → frontend hiding

A new KV setting, fully reusing the migration-free settings pattern (`internal/settings/settings.go`):

1. **`internal/settings`**: add `KeyGithubIntegration = "github_integration"` to the const block and `Defaults` map with default `"on"`. Add a `Validate` case accepting `on`/`off` (mirror the `KeyShell` enum check). No migration — absent row reads as the code default `"on"` (the established "defaults live in code" rule). Add a tiny `ParseBool`-style helper (or reuse the `on`/`off` literal) shared by API gating and validation, mirroring how `ParseDoneSessionTTL` / `AllowedShells` are single-source-of-truth helpers.

2. **API gating** (read-at-use, managers stay DB-free — the established posture): the PR-list and PR-review endpoints check `settings.Get(db, KeyGithubIntegration)` at the top of each handler. When `off`, the GETs return `{state:"disabled"}` (always 200, never an error) and `POST .../review` returns 409/403. This is the same read-at-use pattern `provisionWorktree` uses for `branch_template`/`worktree_base`.

3. **Frontend hiding**: the global `useSettings()` hook (`web/src/api/settings.ts`) already fetches all settings in one query. The Review column, the project-config "linked repo" field, and any PR affordance read `settings.github_integration.value === "on"` and render nothing when off. Because `useSettings` is one shared cached query, the toggle flips all GitHub UI atomically with no extra fetch. The SettingsPage gets one new `SettingsField` (an on/off select, reusing the existing `Options`-driven select the shell field already uses).

The toggle is purely additive: when off, the board, tasks, sessions, and existing flows are byte-for-byte unchanged (no GitHub queries fire, no columns render).

---

## 7. Data flow + build order

### PR-list data flow (end to end)

```
[Review column mounts / 60s poll tick (paused when tab hidden) / manual refresh]
        │  usePullRequests(projectId)  (refetchInterval 60s, refetchIntervalInBackground:false)
        ▼
GET /api/projects/{id}/pull-requests[?refresh=1]
        │  handler: check github_integration toggle + project.github_repo
        ▼
github.Service.List(ctx, repo, force)
        │  TTL/floor/backoff gate (quota shape, keyed by repo)
        │  miss → exec: gh pr list -R <repo> --search "is:open review-requested:@me" --json ...
        ▼
parse JSON array → []PullRequest → cache → Result{state, stale, fetchedAt, pullRequests}
        ▼
always-200 JSON  →  Review column renders PR cards (or disabled/no_gh/empty state)
        ▼
[user clicks a PR card]
        ▼
POST /api/projects/{id}/pull-requests/{n}/review
        │  find-or-create tasks row (source='github_pr', pr_number, pr_base_ref)
        │  provisionWorktree → wt.CheckoutPR (fetch refs/pull/n/head → worktree add)
        ▼
navigate to /projects/{id}/tasks/{taskId}  →  REUSED TaskPage (Agent/Description/Diff/bash)

[meanwhile, server-side, every reaper tick]
reconcilePRsOnce → ghSvc.PRState(repo, n) → MERGED/CLOSED?
        → gated cleanup (dirty + sessions clean) → wt.Remove (branch kept)
```

### Suggested build order (dependency-honoring phases)

**Phase A — Foundations: schema + settings toggle + project link (no PR fetching yet).**
- Migration 00007 (projects + tasks columns); extend `projectColumns`/`scanProject` and `taskColumns`/`scanTask`.
- `github_integration` setting + validation + SettingsPage field.
- `PATCH /api/projects/{id}` extended for description + github_repo; `internal/github.ValidateRepo`.
- Project-config frontend UI.
- *Why first:* every later phase reads these columns and the toggle. No dependency on the others. Ships visible value (project config) immediately.

**Phase B — `internal/github` package + read-only PR list + Review column.**
- `internal/github` leaf package: `ListReviewRequestedPRs`, the quota-shaped cache/poll/degrade `Service`, wiring in `main.go`.
- `GET /api/projects/{id}/pull-requests` (toggle- and link-gated, always 200).
- Frontend `usePullRequests` hook (quota poll pattern) + collapsible Review column + PR cards.
- *Depends on:* Phase A (needs `github_repo` + toggle). Pure read path; no worktree/task mutation yet — low risk, independently demoable.

**Phase C — Open-a-review: PR task creation + PR worktree checkout + diff base.**
- `worktree.CheckoutPR` verb (fetch `refs/pull/n/head` + `worktree add`).
- `provisionWorktree` branch on `source`.
- `POST .../{n}/review` find-or-create task + provision; `UNIQUE(project_id, pr_number)`.
- Board query `WHERE source='manual'` filter (+ audit all `tasks` selects); `/move` guard.
- Diff handler base selection by `source`/`pr_base_ref`.
- Frontend: PR card click → review task view (reused TaskPage).
- *Depends on:* Phase B (needs the PR list/cache to populate `pr_base_ref`) and Phase A (columns). This is the milestone's headline — the reused task view lights up for PRs.

**Phase D — Auto-cleanup on merge/close (reaper extension).**
- Extract shared `cleanupWorktreeGated` helper from `worktreeHandlers.remove`; refactor the handler to use it (regression-guarded).
- `internal/github.PRState` + reaper `PRStateGetter` interface + `reconcilePRsOnce` pass.
- Wire `ghSvc` into the reaper in `main.go`.
- *Depends on:* Phase C (needs PR tasks with worktrees to clean up) and Phase B (needs `PRState`). Last because it operates on the artifacts the earlier phases create, and the shared-helper extraction is safest once the manual cleanup path is stable and the PR path exists.

This order means each phase is independently shippable, every phase builds only on already-landed columns/services, and the riskiest change (the board-query `source` filter audit) lands in Phase C where it's the focus rather than buried.

---

## Anti-Patterns (specific to this integration)

### Anti-Pattern 1: A separate `pr_reviews` table
**What people do:** Model PR reviews in their own table for "cleanliness."
**Why it's wrong:** The session manager, tmux table, diff/session/worktree handlers, reaper, and the entire task view are all `taskID`-keyed. A second id space forks all of them for no gain — a PR review is a task in every way that matters except board membership.
**Do this instead:** One `source` discriminator column + a board-query filter.

### Anti-Pattern 2: `gh pr checkout` into a worktree
**What people do:** Reach for the obvious `gh pr checkout` verb.
**Why it's wrong:** `gh pr checkout` switches the *current repo's* HEAD; it doesn't make a worktree and would hijack the user's main checkout. (Verified: it has `-b`/`--detach`/`--force` but no worktree mode.)
**Do this instead:** `git fetch origin pull/<n>/head:<localBranch>` then `git worktree add <path> <localBranch>` — robust for forks via the `refs/pull/n/head` ref.

### Anti-Pattern 3: Detecting merges from the browser poll
**What people do:** Trigger cleanup when a PR drops out of the Review list.
**Why it's wrong:** The list poll pauses when the tab is hidden and only sees *open* review-requested PRs — it can't authoritatively tell merged from closed-without-merge, and it's off entirely when nobody's looking.
**Do this instead:** Server-side `PRState` reconcile in the always-on reaper goroutine.

### Anti-Pattern 4: Letting `gh` failures break core flows
**What people do:** Hard-fail link validation / list endpoints when `gh` is missing or unauthenticated.
**Why it's wrong:** `gh` is a soft dependency by milestone decision; a dormant integration must never block the board, tasks, or project linking.
**Do this instead:** First-class degraded states (`no_gh`/`auth_required`/`disabled`), always-200 endpoints — copy `quota.Result` exactly.

### Anti-Pattern 5: Auto-removing dirty/busy PR worktrees
**What people do:** Treat "PR merged" as unconditional permission to delete the worktree.
**Why it's wrong:** It would destroy uncommitted review notes / running agent work — violating the same invariants the manual cleanup gates protect.
**Do this instead:** Auto-remove only when the dirty gate AND the sessions gate pass clean; otherwise leave it for manual cleanup. Branch always kept.

---

## Integration Points

### External Services

| Service | Integration Pattern | Notes / gotchas |
|---------|---------------------|-----------------|
| `gh` CLI (2.82.0 verified) | `os/exec` arg-array, bounded ctx, exit-0-only | `gh pr list --json` returns `[]` when none; `gh pr view --json state` is UPPERCASE `OPEN/CLOSED/MERGED` (no `merged` field); `gh repo view --json nameWithOwner` validates + canonicalizes a link. Soft dependency — degrade like quota. |
| git CLI (PR fetch) | `git fetch origin pull/<n>/head:<branch>` + `worktree add` | The one network call `internal/worktree` makes (scoped D-24 exception); `refs/pull/n/head` handles forks. |
| GitHub auth | `gh` host auth (keyring) — no tokens stored | App never reads/stores tokens; `gh auth status` is the only auth signal, and only to pick the `auth_required` state. |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| `internal/github` ↔ API | `Service.List/PRState/ValidateRepo` | Always-200 result struct (quota shape); per-repo cache. |
| `internal/github` ↔ reaper | `PRStateGetter` interface (defined in reaper) | Local interface for the test-spy seam, mirroring `SessionStopper`. |
| `internal/worktree` ↔ `provisionWorktree` | `CheckoutPR` vs `Create` chosen by `tasks.source` | Single choke point already exists; add one branch. |
| diff handler ↔ `internal/diff` | base ref selected by `source`/`pr_base_ref` | `diff.Compute(wt, base)` unchanged — already parameterized. |
| reaper ↔ shared cleanup helper | `cleanupWorktreeGated` extracted from `worktrees.go` | One gated-cleanup path, two callers (HTTP + reaper). |
| settings KV ↔ API gating | `settings.Get(db, KeyGithubIntegration)` read-at-use | Managers stay DB-free; handlers read the toggle. |

---

## Sources

- Live codebase inspection (HIGH) — `internal/{worktree,quota,tmux,reaper,diff,session,settings}`, `internal/api/{routes,projects,tasks,worktrees,diffs,usage,settings,sessions}.go`, `internal/store/migrations/0000{1,3,5,6}_*.sql`, `cmd/kangent/main.go`, `web/src/{pages/TaskPage,components/board/Board,components/task/TaskTabs,api/{queries,usage,settings,agents}}.{tsx,ts}`
- `.planning/PROJECT.md` — v1.3 milestone scope, settled decisions, D-* decision log (HIGH)
- `gh` 2.82.0 verified live on host (HIGH): `gh pr list --search "is:open review-requested:@me" --json ...` (array, `[]` when empty); `gh pr view <n> --json state` (`OPEN`/`CLOSED`/`MERGED`, no `merged` field); `gh repo view <owner/name> --json nameWithOwner,defaultBranchRef` (validation + canonical name; `GraphQL: Could not resolve...` on bad repo); `gh pr checkout --help` (no worktree mode — confirms the git-fetch approach); `isCrossRepository`/`headRepositoryOwner` fields present for fork PRs
- git PR-head ref convention `refs/pull/<n>/head` (MEDIUM — standard GitHub mechanic, the robust fork-safe path vs configuring fork remotes)

---
*Architecture research for: GitHub PR review integration into Kangent v1.3*
*Researched: 2026-06-13*
