# Phase 12: Open-a-Review - Research

**Researched:** 2026-06-14
**Domain:** `gh` PR-metadata reads + detached PR-head worktree checkout + PR-base diff + board-leak `source` filter, layered onto Kangent's existing task view (Go + React)
**Confidence:** HIGH (every `gh`/`git` mechanic re-verified live against `gh 2.82.0` and real `cli/cli` PRs on this host; every code-location and signature read from the actual v1.3 source)

<user_constraints>
## User Constraints (from 12-CONTEXT.md)

> Locked decisions D-01..D-13. Research HOW, do not re-litigate.

### Locked Decisions

- **D-01 (worktree mechanic):** Open via git, never `gh pr checkout`. Resolve PR metadata with `gh pr view <n> --json headRefName,headRefOid,baseRefName,baseRefOid,isCrossRepository,title,body,author,url`, then `git fetch <project-remote> "pull/<n>/head"` and `git worktree add --detach <wt-path> FETCH_HEAD` (or `headRefOid`). **Detached HEAD, no named branch** (guarantees GHREV-05). The checkout verb lives in `internal/worktree` as a new `CheckoutPR`-style function, NOT in `internal/github` (which stays a read-only `gh` query surface).
- **D-02 (open affordance):** Clicking the **PR card body** opens the review. The ↗ external-link stays the GitHub link, with `stopPropagation`. The checks dot stays non-interactive (click falls through to open).
- **D-03 (reattach, GHREV-02):** Reopening a PR that already has a review reattaches to the existing `source='github_pr'` row. Identity key = `project_id` + `pr_number`.
- **D-04 (default tab):** A PR review opens on the **Agent tab** (carry D-39 forward — NO source-based override). Diff tab enabled immediately.
- **D-05 (explicit Start):** Opening a review does NOT auto-spawn `claude`.
- **D-06 (review-seeded):** On Start the seed prompt is **prefilled into the input, NOT auto-sent**.
- **D-07 (seed text):** Fixed Kangent template interpolating PR number + title, e.g. `Review PR #<n> "<title>". Summarize the changes, then flag bugs, risky changes, and missing tests.` Exact wording is planner's discretion; not configurable this phase.
- **D-08 (read-only identity):** Header shows the **PR title, non-editable**. Reuse the `TaskPage` header shell but replace the editable-title button with a static title for `source='github_pr'`.
- **D-09 (PR meta line):** Replace/augment `WorktreeMetaLine` with a PR meta line: **`#<number>` · `@<author>` · base branch · ↗ open-on-GitHub**.
- **D-10 (⋯ menu):** For a PR review, **omit the ⋯ menu** this phase (no Delete task; cleanup is Phase 13).
- **D-11 (Description tab):** Renders the **PR body (the `body` field), read-only, as markdown**. Empty body → quiet "No description." state.
- **D-12 (diff base):** The review's diff is computed against the **PR's own base** (`pr_base_ref`), not the project's configured base. Fetch the base ref, diff against `git merge-base <base> HEAD` (three-dot), reusing `internal/diff` parameterized — do NOT fork the renderer, do NOT reuse the project-base merge-base. Re-resolve the base on refresh.
- **D-13 (board-leak guard):** PR reviews must NEVER appear as kanban cards. Add `WHERE source='manual'` to the board list query **and audit EVERY `tasks` SELECT**. Add a `/move` 409 guard for `source != 'manual'`. Single highest-risk regression — first-class task with explicit per-query review.

### Claude's Discretion

- PR worktree on-disk location/naming (reuse `~/.kangent/worktrees/` dir; `pr-<n>` or `<project>-pr-<n>`).
- Route shape (reuse `/projects/{id}/tasks/{taskId}` vs a distinct `/.../reviews/{prNumber}` path). Reattach is by project+pr_number regardless.
- Whether worktree meta line is shown alongside the PR meta line.
- Exact seed wording (D-07), empty-PR-body copy (D-11), single "Open on GitHub" ⋯ item (D-10).
- Loading/spinner UX while the worktree provisions on first open.

### Deferred Ideas (OUT OF SCOPE)

- **Auto-cleanup of PR worktrees** on merge/close and **gated manual cleanup** from the review view — GHCLN-01/02/03 → Phase 13.
- **In-app GitHub writes** (reviews/comments/approvals) — not in v1.3.
- **Configurable / richer review-seed prompts** — Phase 12 ships one fixed template.

> The 12-UI-SPEC.md visual contract is ALSO locked (approved). Its Component Contract §A–§F decides every undecided visual: card hover `hover:bg-[#27272a]` + `cursor-pointer` + `role="button"`; read-only title box `min-w-0 flex-1 truncate text-base font-medium px-1 py-0.5 text-left` (a `<span>`/`<h1>`, no hover, no onClick); PR meta line `flex min-w-0 items-center gap-2 text-xs text-muted-foreground` with `·` separators and `font-mono` base; loading `Setting up the review worktree…` after a 150ms delay; error `Couldn't open this review.` + `Couldn't set up the worktree: {error}` + neutral `Retry`. No new tokens, colors, or npm deps.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| GHREV-01 | Click a PR card → task-like review view backed by a worktree with the PR head checked out (fetch the PR head ref, not a fresh task branch), reusing agent/bash/diff tabs | §2 worktree checkout sequence (verified live); §5 open endpoint; §6 frontend wiring. PR head fetch + `worktree add --detach FETCH_HEAD` verified to leave main HEAD untouched. |
| GHREV-02 | Reopening the same PR reattaches to its existing review workspace (no duplicate) | §5 find-or-create by `project_id + pr_number AND source='github_pr'`; needs `UNIQUE(project_id, pr_number)` for race-safety. |
| GHREV-03 | The review diff is the PR's own base-branch merge-base, matching GitHub's Files-changed | §3 diff base plumbing — branch `diffs.go` on `source`/`pr_base_ref`; fetch base before merge-base; `diff.Compute` already three-dot, unchanged. |
| GHREV-04 | PR review workspaces never appear as kanban cards | §4 board-leak audit — per-query verdict table; `WHERE source='manual'` on board list + position queries; `/move` 409 guard. |
| GHREV-05 | Fork PRs and branch-name-colliding PRs open without disturbing the primary checkout (main HEAD unchanged) | §2 — `--detach` + `refs/pull/<n>/head` from the BASE repo; verified live on a fork PR (`isCrossRepository:true`) that main `trunk` HEAD is unchanged and no fork remote is needed. |
</phase_requirements>

## Summary

Phase 12 is overwhelmingly **plumbing onto proven primitives**, not new machinery. A PR review is a `tasks` row with `source='github_pr'`; the migration columns (`source`, `pr_number`, `pr_base_ref`) already exist (00007). The session manager, diff renderer, worktree gates, and the entire `TaskPage`/`TaskTabs` view are all `taskID`-keyed and light up for free the moment a PR review *is* a task. The genuinely new code is: one `worktree.CheckoutPR` verb, one `provisionWorktree` branch, one PR-metadata fetch (`gh pr view`), one open-or-reattach endpoint, one diff-base branch in `diffs.go`, the board-leak `source` filter + `/move` guard, and the frontend `source==='github_pr'` deltas the UI-SPEC already specified.

The two non-negotiables were both **verified live**: (a) `git fetch origin refs/pull/<n>/head` + `git worktree add --detach <path> <oid>` leaves the project's primary checkout's HEAD byte-for-byte unchanged for **same-repo AND fork PRs** (tested against `cli/cli` PR #1 same-repo and PR #13655 fork — main `trunk@57b9b207d900` unchanged both times, no fork remote configured); and (b) the board-leak surface is a **finite, enumerated set of `tasks` SELECTs** (16 sites found, classified in §4). The `refs/pull/<n>/head` ref resolves fork heads from the base repo, which is exactly why `--detach` on it sidesteps the cli/cli#8383 same-name fast-forward trap entirely.

One sharp gotcha surfaced during live verification: **do not chain two `git fetch` calls and rely on `FETCH_HEAD` across both** — the second fetch (the base branch) overwrites `FETCH_HEAD`. Pin the worktree to the PR's `headRefOid` (captured from `gh pr view`) and resolve the diff base via the stable `origin/<base>` remote-tracking ref, never `FETCH_HEAD`.

**Primary recommendation:** Add `worktree.CheckoutPR(ctx, repo, path, headOID, prNumber)` (fetch `refs/pull/<n>/head`, then `worktree add --detach <path> <headOID>`); add `POST /api/projects/{id}/pull-requests/{n}/review` that `gh pr view`s the PR, find-or-creates the `source='github_pr'` task, and provisions; branch `diffs.go` and `provisionWorktree` on `source`; add `WHERE source='manual'` to the four board/position queries + a `/move` 409 guard; and apply the UI-SPEC's locked `source==='github_pr'` deltas to `TaskPage` and the three tabs. No new Go or npm dependency.

## Standard Stack

### Core
| Tool | Version | Purpose | Why Standard |
|------|---------|---------|--------------|
| `gh` CLI (host binary) | 2.82.0 (verified live on host) | PR metadata read (`gh pr view <n> -R <repo> --json …`) | Already installed/authed; the same shell-out posture Phase 10/11 use. No token stored. |
| system `git` | host git (worktree stable since 2.5) | fetch PR head + detached worktree | Phase 12 is a parameter variant of the existing `internal/worktree` create path, not a new git feature. |
| `internal/github` (existing) | — | read-only `gh` query surface | Already exists (Phase 10/11). **Extend** with a per-PR `gh pr view` fetch (`GetPR`/`ViewPR`). Stays read-only; the git mechanics go in `internal/worktree`. |
| `internal/worktree` (existing) | — | worktree provisioning | Already shells out with `-C <repo>`, arg-arrays, exit-0-only, `--porcelain`. **Add** `CheckoutPR`. |
| `internal/diff` (existing) | — | diff compute | `Compute(ctx, wt, base)` is **already parameterized by base** — reused unchanged. |

### Supporting (frontend — all already in repo)
| Library | Purpose | When to Use |
|---------|---------|-------------|
| React 19 + TanStack Query 5 | open/reattach mutation, route to review | New `useOpenReview(projectId)` mutation hook (mirrors `useCreateWorktree`'s shape). |
| `react-router` `useNavigate` | route to `/projects/{id}/tasks/{taskId}` | Same nav `TaskCard` already uses (`navigate(\`/projects/${pid}/tasks/${id}\`)`). |
| `react-markdown` + `remark-gfm` | render PR body read-only | Already in `DescriptionTab`; reuse the `prose prose-invert prose-sm` block, drop the Edit/Textarea. |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `worktree add --detach FETCH_HEAD` | named branch `kangent-pr/<n>` (STACK.md preferred) | CONTEXT D-01 **locks `--detach`**; named branch reintroduces the "branch already checked out" lock (PITFALL 3) and the #8383 fork trap. Use `--detach`, pinned to `headRefOid`. |
| Pin worktree to `headRefOid` | pin to `FETCH_HEAD` | `FETCH_HEAD` is overwritten by the subsequent base-branch fetch (verified failure). Use the OID from `gh pr view` for determinism + no race. |
| Per-PR `gh pr view` on open | reuse the Phase 11 list's `PRSummary` (already carries `baseRefName`, `headRefOid`) | The list **lacks `body`** (the Description tab needs it) and may be stale/absent (reattach can happen with no list mounted). Do a fresh `gh pr view` on open for `body` + freshest OID/base; see §1. |

**Installation:** None. No new Go module, no new npm dependency, no stored token (verified: `gh` owns auth in the keyring; `gh --version` → 2.82.0).

## Architecture Patterns

### Where the new pieces sit (all on existing seams)
```
internal/worktree/worktree.go   + CheckoutPR(ctx, repo, path, headOID, prNumber)   [NEW verb beside Create]
internal/github/github.go       + ViewPR(ctx, repo, n) (PRDetail, error)           [NEW gh pr view fetch]
internal/api/tasks.go             provisionWorktree branches on source              [BRANCH]
                                  listByProject + position queries get source filter [GUARD — §4]
                                  move handler gets a source!='manual' 409 guard     [GUARD — §4]
internal/api/diffs.go             base selected by source/pr_base_ref               [BRANCH — §3]
internal/api/pullrequests.go    + POST .../pull-requests/{n}/review                 [NEW endpoint — §5]
web/src/components/board/PRCard.tsx  body becomes open trigger (UI-SPEC §A)         [WIRE — §6]
web/src/pages/TaskPage.tsx        source==='github_pr' header/meta/tab deltas        [BRANCH — §6]
web/src/api/pullRequests.ts     + useOpenReview mutation                            [NEW hook — §6]
```

---

### 1. PR metadata query (locked verb: `gh pr view`)

**The exact command (verified live against `cli/cli` PR #1, all fields returned):**
```bash
gh pr view <n> -R <owner/name> \
  --json number,title,body,author,url,headRefName,headRefOid,baseRefName,baseRefOid,isCrossRepository
```
Verified output shape (real, fields trimmed):
```json
{"number":1,"title":"interactive pr list","body":"this PR implements…",
 "author":{"id":"…","is_bot":false,"login":"vilmibm","name":"Nate Smith"},
 "url":"https://github.com/cli/cli/pull/1",
 "headRefName":"gh-pr","headRefOid":"e9a3253762e768badaa1d4a5b3d267416d1e42f4",
 "baseRefName":"prototype","baseRefOid":"8ebaf1d3…","isCrossRepository":false}
```

- **Author shape:** `author` is an **object** — flatten `.author.login` (the existing Phase 11 `prRaw` already does exactly this; mirror that lenient decode struct). `gh pr view` (singular) returns `author` as an object identically to `gh pr list`.
- **`body`** is a plain Markdown string (may be `""` for an empty PR body — D-11 empty state). **This is the field the Phase 11 list does NOT fetch**, so a fresh `gh pr view` is required on open.
- **`baseRefName`** (e.g. `"prototype"`, `"main"`, `"trunk"`) is the **plain branch name** to store in `pr_base_ref` (D-12 "store the plain branch name").
- **`headRefOid`** is the exact head SHA — pin the worktree to it (avoids the FETCH_HEAD race; §2).
- **`isCrossRepository`** → fork detection (display only; the fetch mechanic is identical for forks).

**Does the Phase 11 PR list already carry enough?** Partially. `PRSummary` (`internal/github/prlist.go`) already fetches `headRefName, headRefOid, baseRefName, isCrossRepository` (the fields were deliberately fetched-but-unrendered "for Phase 12/13"). **But it lacks `body` and `title`-is-list-only-and-may-be-stale, and reattach (D-03) can fire with no list mounted.** Decision: **open does a fresh `gh pr view`** to get `body` + the freshest `headRefOid`/`baseRefName`. This is one extra `gh` call only on the open click (not per-poll) — well within the core 5000/hr budget (PITFALL 8 only warns against per-card-per-poll calls).

**Where the verb belongs:** extend `internal/github` with a `ViewPR(ctx, repo, n)` returning a typed `PRDetail{Number, Title, Body, AuthorLogin, URL, HeadRefName, HeadRefOid, BaseRefName, BaseRefOid, IsCrossRepository}`. It is a **read-only `gh` query** — exactly the package's contract (the package doc says "NEVER reimplements gh… read-only leaf"). The git mechanics stay in `internal/worktree` (D-01). Use the same `exec.CommandContext("gh", "pr", "view", …).Output()` + lenient `json.Unmarshal` posture as `ValidateRepo`/`runGH`. `gh` absence/auth failure degrades to a typed error the open endpoint surfaces as the §F provisioning error (degrade-don't-break).

---

### 2. The worktree checkout sequence (THE risk center — GHREV-01/05) — VERIFIED LIVE

**The exact, verified git sequence (run with `cmd.Dir`/`-C` = the project repo root, like every other worktree op):**
```bash
# 1. Fetch the PR head ref from the project remote. refs/pull/<n>/head is a
#    GitHub server-side ref on the BASE repo that resolves the PR head EVEN FOR
#    FORKS — no fork remote needed.
git -C <repo> fetch origin "refs/pull/<n>/head"

# 2. Add a DETACHED worktree pinned to the PR head OID (from gh pr view).
#    --detach => occupies NO branch => never collides, never moves main HEAD.
git -C <repo> worktree add --detach <wt-path> <headRefOid>
```

**Live verification results (this host, `gh 2.82.0`, real `cli/cli`):**

| Test | Result |
|------|--------|
| (a) **Same-repo PR** (#1, `isCrossRepository:false`) | `fetch refs/pull/1/head` → `worktree add --detach <path> e9a3253` → wt HEAD = `e9a3253762e7` ✓. **Main checkout stayed `trunk@57b9b207d900` (unchanged).** |
| (b) **Fork PR** (#13655, `isCrossRepository:true`, author `KirtiRamchandani`, head `fix/merge-queue-without-auto-merge`, base `trunk`) | `fetch origin refs/pull/13655/head` resolved the fork head `bfd7df8d2463` from the **base repo, no fork remote** → detached wt HEAD = `bfd7df8d2463` ✓. **Main checkout stayed `trunk@57b9b207d900` (unchanged).** |
| (c) **`--detach` avoids the branch lock** | A detached worktree occupies no branch ref, so it cannot trip `fatal: '<branch>' is already checked out at …` (PITFALL 3) regardless of head name (incl. `main`/`master`/an existing task branch). |
| (d) **`git worktree remove` on a detached worktree** (for Phase 13 — noted only) | `git worktree remove <detached-wt>` succeeded cleanly; `git branch` afterward showed only `* trunk` (no kangent branch was ever created). Existing `worktree.Remove` works unchanged for PR worktrees. |

> **Critical gotcha (verified failure):** Do **not** rely on `FETCH_HEAD` after fetching the base branch in step 3 of the diff path (§3) — the base fetch overwrites `FETCH_HEAD`. Pin the worktree to the **`headRefOid` from `gh pr view`** in step 2, not `FETCH_HEAD`. (Pinning to FETCH_HEAD in step 2 only works if no other fetch intervenes; the OID is unconditionally safe.)

**Where `CheckoutPR` belongs — the real `internal/worktree` shape to mirror:**

The existing `Create` (worktree.go:160) is the template:
```go
func (s *Service) Create(ctx context.Context, repo, branch, path, base string) error {
    s.mu.Lock(); defer s.mu.Unlock()
    if _, err := os.Stat(path); err == nil { return fmt.Errorf("worktree path already exists: %s", path) }
    if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { return err }
    // … branch-exists reuse, then:
    _, err := gitRun(ctx, repo, "worktree", "add", "-b", branch, path, base)
    return err
}
```
`CheckoutPR` is the same discipline (same mutex, same `os.Stat` path pre-check, same `MkdirAll`, same `gitRun`) but **detached + a fetch first**:
```go
// CheckoutPR provisions a DETACHED worktree at `path` on the PR head (D-01,
// GHREV-01/05). headOID comes from gh pr view (NOT FETCH_HEAD — that is
// clobbered by the later base fetch). The fetch is a DELIBERATE, SCOPED
// EXCEPTION to the package's "never fetch" invariant (D-24): a PR review's
// whole point is fetching someone else's branch. refs/pull/<n>/head resolves
// fork heads from the base repo, so forks need no special-casing. Detached
// HEAD occupies no branch => never moves main HEAD, never collides (PITFALL 3/5).
func (s *Service) CheckoutPR(ctx context.Context, repo, path, headOID string, prNumber int) error {
    s.mu.Lock(); defer s.mu.Unlock()
    if _, err := os.Stat(path); err == nil { return fmt.Errorf("worktree path already exists: %s", path) }
    if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { return err }
    if _, err := gitRun(ctx, repo, "fetch", "origin", fmt.Sprintf("refs/pull/%d/head", prNumber)); err != nil {
        return err
    }
    _, err := gitRun(ctx, repo, "worktree", "add", "--detach", path, headOID)
    return err
}
```
- **Update the package doc comment:** the file header asserts "never fetches (D-24)". Add the scoped exception note (the milestone already sanctions this in ARCHITECTURE §4 / STATE.md "deliberate, scoped exception to worktree's 'never fetch' invariant").
- **Remote name:** ARCHITECTURE/STACK note "don't hard-code `origin` blindly." For v1.3, `origin` is acceptable (every linked repo is a GitHub clone with an `origin`); if hardening is wanted, resolve via `git -C <repo> remote` first. **Recommend: hard-code `origin` for this phase** (simplest, matches the verified path) and leave remote-resolution as a noted future hardening — keep scope tight.
- **Named-branch vs `--detach`: DEFINITIVELY `--detach`** (D-01 locks it; STATE.md flagged the STACK-vs-PITFALLS disagreement and CONTEXT resolved it to detached). Do not create a `kangent-pr/<n>` branch.

---

### 3. Diff base plumbing (GHREV-03)

**Current path (manual tasks):** `diffs.go:get` (diffs.go:72) calls `base, err := h.wt.ResolveBase(r.Context(), repo)` (the project default-branch chain), then `diff.Compute(ctx, path, base)` (diffs.go:78), then `d.Base = base`. `diff.Compute` (diff.go:85) does `merge-base base HEAD` then three-dot numstat/patch — **already correct three-dot semantics for "what changed vs base."**

**The change is entirely in the handler — `internal/diff` is NOT touched (D-12):**

1. **Extend the diffs.go SELECT** to also read `source` and `pr_base_ref` (currently it selects only `worktree_path` + the project's `repo_path`, diffs.go:42–44). New query:
   ```go
   var wtPath sql.NullString
   var repo string
   var source string
   var prBaseRef sql.NullString
   err := h.db.QueryRow(
       `SELECT worktree_path, source, pr_base_ref,
          (SELECT repo_path FROM projects WHERE projects.id = tasks.project_id)
        FROM tasks WHERE id = ?`, id).Scan(&wtPath, &source, &prBaseRef, &repo)
   ```
   (`source` is NOT NULL DEFAULT 'manual'; `pr_base_ref` is nullable.)

2. **Branch the base selection** (replacing the single `ResolveBase` call at diffs.go:72):
   ```go
   var base string
   if source == "github_pr" && prBaseRef.Valid {
       // D-12: the PR's OWN base, not the project default. Fetch it first so the
       // remote-tracking ref exists, then prefer local-then-remote (ResolveBase's
       // chain shape). Use origin/<base> remote-tracking — STABLE, never FETCH_HEAD.
       baseName := prBaseRef.String
       // best-effort fetch of the base branch (a retarget may have moved it);
       // ignore fetch error and fall through — merge-base will surface a clear
       // error if the ref truly can't resolve.
       _, _ = h.wt.FetchRef(r.Context(), repo, baseName)   // see note
       base = resolvePRBase(r.Context(), h.wt, repo, baseName) // local refs/heads/<base> else origin/<base>
   } else {
       base, err = h.wt.ResolveBase(r.Context(), repo)
       if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }
   }
   d, err := diff.Compute(r.Context(), path, base)
   ```

3. **Fetch-before-merge-base (verified necessary):** `git merge-base origin/<base> HEAD` fails with `fatal: Not a valid object name origin/<base>` if the base ref was never fetched into the worktree's repo (verified — PR #1's `prototype` base was absent in a fresh clone). So the handler **must `git -C <repo> fetch origin <baseRefName>` before computing the base**, then use the **`origin/<base>` remote-tracking ref** (stable) for `merge-base`. Add a small `worktree.FetchRef(ctx, repo, ref)` helper (a one-line `gitRun(ctx, repo, "fetch", "origin", ref)` wrapper) so the fetch stays inside the worktree package's git surface, consistent with `CheckoutPR`'s fetch. Prefer a **local `refs/heads/<base>`** if it exists (same local-then-remote pattern `ResolveBase` already uses, worktree.go:120–126), else `origin/<base>`.

4. **Re-resolve on every diff request** (D-12 "re-resolve the base on refresh"): the diff endpoint is already on-demand (no caching, diffs.go header) and re-resolves the base per request — so a retargeted PR's base is picked up for free on the next Diff-tab open. The `pr_base_ref` column is the stored base **name**; resolution local-vs-remote happens at diff time.

5. **Three-dot correctness — verified.** `diff.Compute` does `merge-base base HEAD` then `diff <mb>`, which is "the PR's changes since it diverged from its base" = GitHub's Files-changed semantics. Live check (cli/cli PR #13632, base `trunk`): `merge-base origin/trunk <pr-head>` resolved; the numstat pipeline is the same one Kangent ships. (A precise file-count match requires the worktree's repo to have the base fully fetched — hence step 3 — and the head not already merged into the local base; the **mechanic** is confirmed identical to the existing diff tab.)

> **Storage note:** `pr_base_ref` stores `baseRefName` (plain name) — set at task creation from `gh pr view` (§5). The diff handler never re-calls `gh`; it works purely from git refs.

---

### 4. Board-leak audit (GHREV-04 — single highest-risk regression)

**Every `tasks` SELECT/INSERT/UPDATE/DELETE in non-test Go, with a per-query verdict.** A PR review is a `tasks` row, so any query that treats *all* tasks as board cards leaks it. Verdicts:

| # | File:Line | Query (purpose) | Verdict | Action |
|---|-----------|-----------------|---------|--------|
| 1 | `tasks.go:157` | `SELECT … FROM tasks WHERE project_id = ? ORDER BY status, position` (**board list**, `listByProject`) | **MUST FILTER** | Add `AND source = 'manual'`. THE primary board-leak point (GHREV-04). |
| 2 | `tasks.go:211` | `SELECT COALESCE(MIN(position),2.0)-1.0 FROM tasks WHERE project_id=? AND status='todo'` (**create: top-of-ToDo position**) | **MUST FILTER** | Add `AND source = 'manual'`. PR rows have no status/position role; excluding them keeps positioning math pure. (PR rows get `status` default `'todo'` unless set otherwise — see §5 note — so without a filter a PR row could perturb the MIN.) |
| 3 | `tasks.go:346` | `SELECT COALESCE(MIN(position),2.0)-1.0 FROM tasks WHERE project_id=? AND status=? AND id!=?` (**move: drop-at-top**) | **MUST FILTER** | Add `AND source = 'manual'`. Same positioning-purity reason. (Also gated by the §move guard below, but defense-in-depth: filter anyway.) |
| 4 | `tasks.go:452` | `SELECT MIN(position) FROM tasks WHERE project_id=? AND status=? AND position>? AND id!=?` (**move: nextPosition**) | **MUST FILTER** | Add `AND source = 'manual'`. |
| 5 | `tasks.go:469` | `SELECT id FROM tasks WHERE project_id=? AND status=? ORDER BY position,id` (**move: renumberColumn**) | **MUST FILTER** | Add `AND source = 'manual'`. A PR row must never be renumbered into the board sequence. |
| 6 | `tasks.go:328` | `SELECT project_id FROM tasks WHERE id = ?` (**move: load mover**) | **GUARD** (by-id) | Add the §move 409 guard: also select `source`; if `source != 'manual'` → 409. (See "the /move guard" below.) |
| 7 | `tasks.go:433` | `SELECT project_id, status, position FROM tasks WHERE id = ?` (**move: afterPosition anchor**) | **OK as-is** (defended by #6) | By-id anchor lookup; #6's guard already rejects a PR mover. No filter needed but harmless to add a `source='manual'` AND to be belt-and-suspenders. |
| 8 | `tasks.go:237` | `SELECT … FROM tasks WHERE id = ?` (**get by id**, `taskHandlers.get`) | **OK as-is** | Deep-link by id; the review view legitimately fetches its own PR row here. Do NOT filter. |
| 9 | `tasks.go:529` | `DELETE FROM tasks WHERE id = ?` (**delete by id**) | **OK as-is** | By-id; UI omits Delete for PR reviews (D-10) but the endpoint is harmless if ever hit. No filter. |
| 10 | `tasks.go:288` | `UPDATE tasks SET … WHERE id = ? RETURNING …` (**update title/description**) | **OK as-is** | By-id; the review view never PATCHes (read-only title/body D-08/D-11), but no leak risk. |
| 11 | `tasks.go:408` | `UPDATE tasks SET status=?, position=?, …_at=… WHERE id = ?` (**move: persist**) | **OK** (defended by #6) | Only reached after #6's guard passes (manual only). |
| 12 | `worktrees.go:51` | `SELECT taskColumns, (repo_path) FROM tasks WHERE id = ?` (`loadTaskRepo`) | **OK as-is** | By-id; the review view uses worktree gates legitimately. No filter. |
| 13 | `worktrees.go:150` | `SELECT taskColumns FROM tasks WHERE id = ?` (re-read after create) | **OK as-is** | By-id. |
| 14 | `diffs.go:42` | `SELECT worktree_path, (repo_path) FROM tasks WHERE id = ?` (diff) | **OK** but **EXTEND** | By-id (no leak), but §3 requires adding `source, pr_base_ref` to the SELECT to branch the base. Not a leak fix — a feature change. |
| 15 | `sessions.go:207` | `SELECT worktree_path, claude_session_id FROM tasks WHERE id = ?` (spawn cwd) | **OK as-is** | By-id; PR review agent/bash sessions are intended. No filter. |
| 16 | `agents.go:74` | `SELECT id, project_id, claude_session_id, worktree_path FROM tasks WHERE id IN (…)` (agent status, manager-derived) | **OK as-is** | The IN-list is built from live agent sessions keyed by task id (agents.go:43–54). A PR review legitimately HAS an agent session, and its dot is shown **on the review view**, not the board. The board card dot is keyed per-card; since PR rows are filtered out of the board list (#1), no PR dot can render as a card. No filter needed. |
| 17 | `agents.go:123` | `SELECT id, project_id, claude_session_id, worktree_path FROM tasks WHERE claude_session_id IS NOT NULL AND worktree_path IS NOT NULL` (agent status, DB-derived post-restart) | **OK as-is** (see note) | Surfaces resumable-ghost dots. A PR review with a stored `claude_session_id` would appear in this list with its `projectId`. **This is fine for GHREV-04**: the board renders dots only for tasks that are *also board cards* (the board card list is filtered by #1), so a PR entry here has no card to attach to. **Verify** the frontend board dot consumer keys strictly by board-card task id (it does — `useAgentStatuses().find(e => e.taskId === card.id)`), so a stray PR entry is inert. No backend filter required; **note for the planner to confirm** the frontend never iterates the agent-status list to *create* board rows. |
| 18 | `reaper.go:119` | `SELECT id FROM tasks WHERE status='done' AND done_at IS NOT NULL AND done_at < ?` (Done-TTL reaper) | **OK as-is for Phase 12** | Gated on `status='done'`. A PR review never enters Done via the board (it's not on the board; `/move` is guarded). If a PR row's `status` defaults to `'todo'` it is never `'done'`, so the reaper never touches it. (Phase 13 adds the separate PR-state reconcile pass — out of scope here.) **No change.** |
| 19 | `main.go:211` | `SELECT ts.name FROM tmux_sessions ts JOIN tasks t …` (orphan sweep) | **OK as-is** | Joins tmux rows to tasks by FK; a PR review's tmux sessions are legitimately swept like any task's. No leak. |

**The `/move` guard (defense-in-depth, D-13):** in `taskHandlers.move` (tasks.go:304), change the mover-load query (#6, tasks.go:328) to also select `source`, and reject non-manual movers **before** any position computation:
```go
var projectID int64
var source string
if err := tx.QueryRowContext(ctx,
    `SELECT project_id, source FROM tasks WHERE id = ?`, id).Scan(&projectID, &source); err != nil { … }
if source != "manual" {
    writeError(w, http.StatusConflict, "PR reviews are not board tasks")  // 409, defense in depth
    return
}
```
This guarantees a PR row can never receive a board position even if a client constructs the request directly. Pair it with the `source='manual'` filters on the position queries (#2–#5) so the math also never *sees* a PR row.

**Recommended test coverage for the planner** (this is the milestone's highest-severity regression — make it a first-class task): insert a `source='github_pr'` task and assert it does **not** appear in `GET /api/projects/{id}/tasks`; assert `POST /api/tasks/{prId}/move` returns 409; assert a manual task's positioning is unaffected by a co-existing PR row.

---

### 5. PR-task creation + reattach endpoint (GHREV-01/02)

**New endpoint** (mirrors ARCHITECTURE §3's planned `POST .../{n}/review`, registered beside `PullRequestRoutes` in `pullrequests.go` / wired in `main.go` next to the existing `api.PullRequestRoutes(mux, db, ghSvc)`):
```
POST /api/projects/{id}/pull-requests/{n}/review  ->  200 { …Task }  (the source='github_pr' row to route to)
```
Signature needs the `*github.Service` (for `ViewPR`) and the `*worktree.Service` — so either fold this handler into `PullRequestRoutes(mux, db, svc, wtSvc)` (add `wtSvc *worktree.Service` param) or add a sibling registration. Reuse `pathID` for `{id}` and a second numeric parse for `{n}` (mirror `pathID`'s `strconv.ParseInt`).

**Handler logic (find-or-create, idempotent — D-03):**
```
1. pid = pathID({id}); n = parse {n}  (400 on non-numeric)
2. GATE: settings.Get(KeyGithubIntegration) != "on" -> 409/disabled  (mirror the GET gate, pullrequests.go:35)
3. SELECT github_repo, repo_path FROM projects WHERE id = ?   (repo for gh -R; repo_path for cmd.Dir/CheckoutPR)
     - not linked / unknown -> 409 (the review can't open without a linked repo)
4. FIND EXISTING:
     SELECT <taskColumns> FROM tasks
       WHERE project_id = ? AND pr_number = ? AND source = 'github_pr'
     - found AND worktree_path IS NOT NULL -> return it (200) — instant reattach (D-03), NO gh, NO provision.
     - found AND worktree_path IS NULL (a prior provision failed) -> re-provision (reuse the worktree_error/Retry path) then return.
5. NOT FOUND -> provision a new review:
     a. detail := ghSvc.ViewPR(ctx, repo, n)   (gh pr view --json …; degrade -> 502-ish error the UI shows as §F)
     b. INSERT INTO tasks (project_id, title, source, pr_number, pr_base_ref [, status])
          VALUES (?, ?, 'github_pr', ?, ?, 'todo')
          RETURNING <taskColumns>
        - title = a PR-derived title (e.g. "#<n> <pr title>" — used only as the row's title;
          the read-only header shows detail.Title verbatim, D-08).
        - pr_base_ref = detail.BaseRefName (plain name, §3).
        - status: set to 'todo' (the column is NOT NULL in practice via the create default) OR
          add a dedicated value — BUT the board filter (#1) excludes it regardless of status.
          Recommend status='todo' (no schema/CHECK change; the WHERE source='manual' filter is the gate, not status).
     c. provision the worktree: wt.CheckoutPR(ctx, repo_path, prWorktreePath, detail.HeadRefOid, n)
          - prWorktreePath = PathUnder(worktree_base, repo_path, "pr-"+n, taskID)  (reuse PathUnder; "pr-<n>" slug)
          - on success: UPDATE tasks SET worktree_path = ? WHERE id = ?  (branch stays NULL — detached, no branch)
          - on failure: UPDATE tasks SET worktree_error = ? (the §F error; the row persists for Retry)
     d. return the (re-read) task row (200).
```

**Concurrency / double-click (D-03):** add `CREATE UNIQUE INDEX … ON tasks (project_id, pr_number) WHERE source = 'github_pr'` (a partial unique index — SQLite supports it; manual tasks have NULL `pr_number` so they're excluded). This makes find-or-create race-safe: a double-click's second INSERT fails the unique constraint, and the handler falls back to the find path. **This needs a migration** — but CONTEXT says "NO new migration this phase." Reconcile: **the index is optional for single-user localhost** (the existing pattern note: "single-user localhost makes the read-then-insert race window acceptable, with the constraint as backstop" — sessions.go:296). **Recommend: skip the partial index this phase** (no migration) and rely on the find-first ordering + single-user reality + a frontend in-flight guard (disable the card while the open mutation is pending, mirroring `useCreateWorktree`). Document this as the deliberate single-user trade (consistent with the tmux `n` precedent). If the planner wants the constraint, it would be a new migration 00008 — flag it as a scope decision.

**Sync vs async provisioning (UI-SPEC §F "Setting up the review worktree…"):** the existing task-create path provisions **synchronously** under a 30s cap and always returns the task (tasks.go:218–228). **Mirror that: provision synchronously and return the task with `worktree_path` set (or `worktree_error`).** The UI-SPEC's 150ms-delayed "Setting up…" state covers the request latency (the fetch + `worktree add` of a real PR head is a few seconds); the mutation resolves with the ready task, then the frontend routes to it. This is simpler than async (no polling, no intermediate state) and matches the established `provisionWorktree` posture. The §F error state is the `worktree_error`/`ApiError` branch.

**Return shape:** the full `Task` row (same `taskColumns` JSON the frontend already consumes via `useTask`), so the frontend routes to `/projects/{id}/tasks/{task.id}` and `useTask` renders it. No new response type needed.

> **`pr_number`/`pr_base_ref` are not in `taskColumns`** (tasks.go:54) or the `Task` JSON struct (tasks.go:26–38). The frontend review view needs `source`, `pr_number`, `pr_base_ref` (and the live PR `title`/`body`/`author`/`url`/`base` for the header). **Two options:** (a) add `source, pr_number, pr_base_ref` to `taskColumns` + `Task` struct + `scanTask` + the frontend `Task` type, and have the open endpoint **also return the live `gh pr view` detail** (title/body/author/url) in a sibling object the frontend caches; or (b) the open endpoint returns `{task, pr: PRDetail}` and the frontend stashes the `PRDetail` (title/body/author/url/base) keyed by task id. **Recommend (a) for the row discriminator (`source`) — TaskPage must branch on it — plus returning the live `PRDetail` for the GitHub-sourced display fields** (title/body must come from GitHub, not the stored row, to "not drift" per D-08/D-11). The planner should decide the exact transport; the key constraint is: **`source` must reach the frontend `Task`** (so `TaskPage` branches), and **title/body/author/url/base must come from a live `gh pr view`** (so they mirror GitHub).

---

### 6. Frontend wiring

**PRCard body → open (UI-SPEC §A, D-02):** `PRCard.tsx` currently renders an inert `<div className="rounded-md border border-border bg-card px-3 py-2">`. Make the root a clickable, keyboard-reachable element per the UI-SPEC:
- Add `hover:bg-[#27272a] cursor-pointer`, `role="button"`, `tabIndex={0}`, `aria-label={\`Open review for PR #${pr.number}\`}`, `focus-visible:ring-2 focus-visible:ring-blue-500`, and `onClick` + `onKeyDown` (Enter/Space) → `openReview.mutate(pr.number)` then `navigate(...)`.
- The ↗ `<a>` already exists (PRCard.tsx:57–65); add `onClick={e => e.stopPropagation()}` so it opens GitHub only (D-02). The checks dot (PRCard.tsx:43–53) stays non-interactive — no stopPropagation (click falls through to open).
- `PRCard` needs `projectId` (to call the open endpoint) — `ReviewColumn`/`ReviewStates` pass it down (ReviewColumn already has `projectId`; `ReviewStates` currently maps `prs.map(pr => <PRCard pr={pr} />)` at ReviewColumn.tsx:231 — add `projectId`).

**New mutation hook** in `web/src/api/pullRequests.ts` (mirror `useCreateWorktree`'s shape, worktrees.ts:33):
```ts
export function useOpenReview(projectId: number) {
  return useMutation<Task /* or {task, pr} */, ApiError, number /* prNumber */>({
    mutationFn: (n) => post(`/api/projects/${projectId}/pull-requests/${n}/review`),
  });
}
```
The card's onClick: `openReview.mutate(pr.number, { onSuccess: (t) => navigate(\`/projects/${projectId}/tasks/${t.id}\`) })`. Disable the card while `openReview.isPending` (in-flight guard for the double-click concurrency note in §5). The §F loading state ("Setting up the review worktree…") can be shown either on the card (during the pending mutation) or on the routed-to review view; UI-SPEC §F renders the review shell immediately with the loading area below — but since open is synchronous (§5), the mutation resolves with a ready task, so the loading state primarily covers the mutation's in-flight period. Planner's call on placement (UI-SPEC allows either; the simplest is the card showing pending then routing to a ready view).

**TaskPage branch on `source === 'github_pr'`** (UI-SPEC §B–§E). All deltas are gated on the (new) `task.source` field:
| Delta | Current code | PR change |
|-------|--------------|-----------|
| Read-only title (D-08, §B) | editable `<button … onClick={() => setTitleDraft(...)}>` (TaskPage.tsx:432–457) | when `source==='github_pr'`, render a `<span>`/`<h1>` with the SAME box `min-w-0 flex-1 truncate px-1 py-0.5 text-base font-medium text-left`, **minus** `hover:bg-muted/50`, `onClick`, `title="Edit title"`. `title={prTitle}`. Use the **live PR title** (from the open endpoint's `PRDetail`), not `task.title`. |
| ⋯ menu omitted (D-10, §B) | `<DropdownMenu>…</DropdownMenu>` (TaskPage.tsx:461–485) | when `source==='github_pr'`, render nothing in the menu slot. |
| PR meta line (D-09, §C) | `<WorktreeMetaLine task=… />` (TaskPage.tsx:488–492) | when `source==='github_pr'`, render a PR meta line (`flex min-w-0 items-center gap-2 text-xs text-muted-foreground`, `·`-separated: `#{n}` · `@{author}` · `font-mono {base}` · ↗ `<a href={pr.url}>`). Optionally keep the `WorktreeMetaLine` active-row beneath it (Claude's discretion, UI-SPEC §C "subordinate"). Hide its Create/Retry affordances (the worktree always exists by render time). |
| Description tab read-only (D-11, §D) | `<DescriptionTab task=… />` (editable, DescriptionTab.tsx) | when `source==='github_pr'`, render the **PR `body`** through the existing `prose prose-invert prose-sm max-w-none prose-a:text-blue-500` block (DescriptionTab.tsx:62), **no Edit button, no Textarea, no Save/Cancel**. Empty body → `No description.` in `text-muted-foreground`, **no** Edit button. Parameterize `DescriptionTab` with an optional read-only markdown source, or branch in TaskPage and pass the PR body. |
| Default tab = Agent (D-04, §D) | `useState("agent")` (TaskPage.tsx:82) | **unchanged** — no source override. |
| Diff tab enabled (D-04, §D) | `"diff"` in `tabIds` iff `task.worktree_path` (TaskPage.tsx:162–170) | **unchanged** — the PR worktree exists by render time, so `worktree_path` is set and `"diff"` is included automatically. The base ref change is server-side (§3) — zero Diff-tab UI change. |

**Seed prefill on Start (D-05/D-06/D-07, §E) — the `pasteApiRef` path EXISTS and is confirmed:** `AgentTab.tsx` holds `const pasteApiRef = useRef<{ paste: (t: string) => void } | null>(null)` (AgentTab.tsx:38), wires it via `onReady={(api) => { pasteApiRef.current = api }}` (AgentTab.tsx:201–203), and already uses `pasteApiRef.current?.paste(task.description)` for "Insert description" (AgentTab.tsx:133). xterm wraps the paste in bracketed-paste `\x1b[200~..\x1b[201~` (AgentTab.tsx:124 comment, verified design) so claude receives it un-submitted.

**To seed on Start for a PR review:** after the agent session spawns and the terminal connects (the `onReady`/`onConnect` callback fires), call `pasteApiRef.current?.paste(seed)` once, where `seed = \`Review PR #${prNumber} "${prTitle}". Summarize the changes, then flag bugs, risky changes, and missing tests.\`` (D-07). The cleanest hook: in `AgentTab`, when `source==='github_pr'` and a session transitions to running/ready for the first time this mount, fire one paste of the seed (guard with a ref so it fires exactly once, like `reattachedRef` in TaskPage). `AgentTab` will need the PR `number`/`title` (and `source`) — pass them in (TaskPage already passes `task` and `projectId`; add the PR detail). The Start button itself is **unchanged** (`Start agent` / `Starting…`); the only delta is the one-shot paste after connect (UI-SPEC §E: "invisible until the agent is running and the prompt is populated"). It is **never auto-submitted** — same as Insert description.

**Routing:** reuse `/projects/{id}/tasks/{taskId}` (the PR review IS a task row). The App route `/projects/:projectId/tasks/:taskId` (App.tsx:44) already matches. Reattach (D-03) routes to the same path with the existing task id — indistinguishable from opening a task.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Check out a PR head into a worktree | `gh pr checkout` / a custom fork-remote dance / parsing `git worktree list` text | `git fetch origin refs/pull/<n>/head` + `git worktree add --detach <path> <headOID>` (in `worktree.CheckoutPR`) | `gh pr checkout` mutates the current checkout (PITFALL 1); the pull ref handles forks with no remote (PITFALL 4); `--detach` avoids the branch lock (PITFALL 3). All verified live. |
| PR diff vs base | A new diff renderer / reuse the project-base merge-base | `diff.Compute(ctx, wt, base)` parameterized with the **PR base** | The renderer already does correct three-dot merge-base; only the base ref changes (D-12). Reusing the project base shows a misleading mega-diff (PITFALL 6). |
| Detect "PR head changed" / fork specifics | Bespoke ref math | `headRefOid` / `isCrossRepository` from `gh pr view --json` | gh resolves it correctly and server-side; the pull ref is fork-agnostic. |
| Keep PR reviews off the board | A separate `pr_reviews` table | `source='github_pr'` discriminator + `WHERE source='manual'` on board/position queries + `/move` 409 | The session manager/diff/worktree/view are all `taskID`-keyed; a second id space forks all of them for zero gain (ARCHITECTURE §1). |
| Seed the agent prompt | A new "paste" mechanism | The existing `pasteApiRef.current.paste()` → `term.paste()` bracketed-paste path | Already shipped for "Insert description"; xterm's bracketed paste delivers it un-submitted (D-06). |
| Render the PR body | A custom markdown component | The existing `react-markdown` + `remark-gfm` `prose prose-invert prose-sm` block | Already in `DescriptionTab`; reuse read-only. Zero new deps. |

**Key insight:** Phase 12 introduces **one** new git verb and **one** new gh read; everything else is a `source`-discriminated branch on existing, tested code. Resist building parallel structures — the discriminator + reuse is the whole architecture.

## Common Pitfalls

### Pitfall 1: FETCH_HEAD clobbered between the two fetches
**What goes wrong:** Fetching the PR head, then fetching the base branch, then `worktree add … FETCH_HEAD` checks out the **base**, not the PR head.
**Why it happens:** `FETCH_HEAD` is rewritten by every `git fetch`. The diff path (§3) fetches the base after the head.
**How to avoid:** Pin the worktree to the PR's **`headRefOid`** (from `gh pr view`), not `FETCH_HEAD`. Resolve the diff base via the stable **`origin/<base>`** remote-tracking ref, never `FETCH_HEAD`.
**Warning signs:** A freshly opened review's worktree shows the base branch's tree, not the PR's changes; the diff is empty.
**(Verified live — the second fetch overwrote FETCH_HEAD in the probe.)**

### Pitfall 2: `gh pr checkout` mutating the primary checkout (PITFALL 1)
**What goes wrong:** `gh pr checkout` switches the *current repo's* HEAD — run with `cmd.Dir`=project root, it yanks the user's real checkout onto the PR branch.
**How to avoid:** Never use `gh pr checkout` for the git mechanics (D-01). Use the fetch + detached-worktree sequence. Verified: main `trunk` HEAD unchanged for both same-repo and fork PRs.
**Warning signs:** After opening a PR, the user's primary checkout is on a different branch / shows unexpected changes.

### Pitfall 3: Diff base not fetched → `merge-base` fails
**What goes wrong:** `git merge-base origin/<base> HEAD` errors `fatal: Not a valid object name origin/<base>` if the base ref was never fetched into the repo.
**Why it happens:** A fresh clone / a base branch the user never checked out has no `origin/<base>` ref.
**How to avoid:** `git -C <repo> fetch origin <baseRefName>` **before** computing the base (§3 step 3), then use `origin/<base>` (or local `refs/heads/<base>` if present). Best-effort: if the fetch fails, let `merge-base` surface the clear error into the UI error card (Pitfall 7 posture, already how `diffs.go` relays git stderr).
**Warning signs:** The Diff tab errors immediately for a PR whose base the user doesn't track locally.
**(Verified live — `merge-base origin/prototype HEAD` failed when `prototype` wasn't fetched.)**

### Pitfall 4: A forgotten `source='manual'` filter leaks a PR onto the board (GHREV-04)
**What goes wrong:** Any board/position query without the `source` filter renders a PR review as a kanban card or perturbs positioning math.
**How to avoid:** Apply the §4 per-query table verbatim: filter queries #1–#5, guard #6 (`/move` 409). Add the regression test (insert a `github_pr` task; assert absent from the board list; assert `/move` → 409).
**Warning signs:** A PR card appears in To Do / a manual task's drop position jumps when a PR review exists in the project.

### Pitfall 5: Double-click opens two reviews / two worktrees (GHREV-02)
**What goes wrong:** Two rapid card clicks each miss the find step and both INSERT + provision, yielding duplicate `github_pr` rows / worktrees.
**How to avoid:** Find-first ordering in the handler (§5), a frontend in-flight guard (disable the card while `openReview.isPending`), and the single-user reality (accepted, per the tmux-`n` precedent). The partial unique index is optional and would require a migration (CONTEXT says none) — recommend skipping it this phase and documenting the single-user trade.
**Warning signs:** Two PR rows with the same `(project_id, pr_number)`; two `pr-<n>-*` worktrees.

### Pitfall 6: Title/body drift from GitHub
**What goes wrong:** Showing the stored `task.title`/`task.description` for a PR review lets the review identity drift from GitHub.
**How to avoid:** The header title (D-08), Description body (D-11), author, and base (D-09) must come from a **live `gh pr view`** (returned by the open endpoint), not the stored row. The stored row's `title` is incidental.
**Warning signs:** A renamed PR shows the old title in the review header.

## Code Examples

### gh pr view metadata (verified live, `gh 2.82.0`)
```bash
# Source: live exec against cli/cli PR #1, all fields returned.
gh pr view 1 -R cli/cli --json number,title,body,author,url,headRefName,headRefOid,baseRefName,baseRefOid,isCrossRepository
# -> author is an OBJECT: flatten .author.login
```

### Detached PR worktree (verified: main HEAD unchanged, same-repo + fork)
```bash
# Source: live exec. headOID from gh pr view (NOT FETCH_HEAD).
git -C <repo> fetch origin "refs/pull/<n>/head"
git -C <repo> worktree add --detach <wt-path> <headRefOid>
# main checkout HEAD verified byte-for-byte unchanged for #1 (same-repo) and #13655 (fork)
```

### Diff base for a PR (verified mechanic)
```bash
# Source: live exec. Fetch base FIRST, use origin/<base> (stable), not FETCH_HEAD.
git -C <repo> fetch origin <baseRefName>
# inside the PR worktree (diff.Compute does exactly this):
git -C <wt> merge-base origin/<baseRefName> HEAD   # the PR's own merge-base (three-dot)
```

### The existing paste-seed handle (confirmed in AgentTab.tsx)
```ts
// Source: web/src/components/task/AgentTab.tsx:38,133,201-203 (already shipped)
const pasteApiRef = useRef<{ paste: (t: string) => void } | null>(null);
// ... onReady={(api) => { pasteApiRef.current = api; }}
pasteApiRef.current?.paste(seed); // xterm wraps in bracketed-paste -> un-submitted
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `gh pr checkout` to get a PR branch | `git fetch refs/pull/<n>/head` + `worktree add --detach` | Settled in v1.3 research (PITFALL 1/3/5), locked D-01 | Never touches the primary checkout; fork-safe. |
| Project-default merge-base for all diffs | PR-base merge-base for `github_pr` tasks | D-12 | Review diff matches GitHub Files-changed. |
| Every `tasks` row is a board card | `source` discriminator + `WHERE source='manual'` | ARCHITECTURE §1 / D-13 | PR reviews own a worktree+agent+diff but are off the board. |

**Deprecated/outdated for this phase:** named-branch PR worktree (`kangent-pr/<n>` / `review/pr-<n>`) — STACK.md preferred it, but CONTEXT D-01 **overrides to `--detach`**. Do not create a branch.

## Open Questions

1. **Transport for the live PR detail (title/body/author/url/base) to the frontend.**
   - What we know: `source` must reach the frontend `Task` (TaskPage branches on it); title/body must come from a live `gh pr view` to avoid drift (D-08/D-11).
   - What's unclear: whether to (a) extend `taskColumns`/`Task` with `source`/`pr_number`/`pr_base_ref` AND return a sibling `PRDetail`, or (b) return `{task, pr}` and cache the `pr` keyed by task id, with a re-fetch path on reattach.
   - Recommendation: add `source` (and likely `pr_number`/`pr_base_ref`) to `Task` for the discriminator; return the live `PRDetail` (title/body/author/url/base) from the open endpoint; on a deep-link/reattach where no `PRDetail` is cached, the planner decides whether to re-`gh pr view` (freshest, one call) — recommend re-fetching so a reattached review never shows stale GitHub data.

2. **Partial `UNIQUE(project_id, pr_number) WHERE source='github_pr'` index.**
   - What we know: it makes find-or-create race-safe; it needs a migration; CONTEXT says no new migration this phase.
   - What's unclear: whether the single-user in-flight-guard trade is acceptable (it is, by the tmux-`n` precedent) or whether the planner wants migration 00008 for the constraint.
   - Recommendation: skip the index this phase; rely on find-first + frontend in-flight guard + single-user reality. Document the trade.

3. **Remote name assumption (`origin`).**
   - What we know: every linked GitHub repo has an `origin`; the verified path hard-codes `origin`.
   - What's unclear: whether to resolve the remote generically now.
   - Recommendation: hard-code `origin` for v1.3 (matches the verified path); note remote-resolution as future hardening to keep scope tight.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| `gh` CLI | PR metadata (`gh pr view`), open endpoint | ✓ | 2.82.0 (2025-10-15) | Degrade: open endpoint returns a typed error → UI §F "Couldn't open this review." (degrade-don't-break). gh is a soft dependency. |
| system `git` | fetch PR head + detached worktree + base fetch | ✓ | host git (worktree stable since 2.5) | None needed — git is guaranteed present (the app's premise). |
| GitHub auth (keyring) | `gh` reads | ✓ | `✓ Logged in to github.com` (scopes repo, read:org, …) | auth_required degrade (existing Phase 11 classification) → §F error. |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** none missing (all present on this host).

## Sources

### Primary (HIGH confidence)
- **Live `gh 2.82.0` / `git` execution on host (2026-06-14):**
  - `gh pr view 1 -R cli/cli --json number,title,body,author,url,headRefName,headRefOid,baseRefName,baseRefOid,isCrossRepository` — all fields returned; `author` is an object.
  - Same-repo PR #1: `fetch refs/pull/1/head` + `worktree add --detach <path> <oid>` → wt HEAD = PR head; **main `trunk@57b9b207d900` unchanged.**
  - Fork PR #13655 (`isCrossRepository:true`): base-repo pull ref fetched the fork head with no fork remote; **main unchanged.**
  - `git worktree remove` on a detached worktree succeeded; no branch created.
  - Diff base: `merge-base origin/<base> HEAD` requires the base fetched first (verified failure without it); three-dot numstat is `diff.Compute`'s pipeline.
- **Live codebase read (HIGH):** `internal/worktree/worktree.go`, `internal/diff/diff.go`, `internal/api/{tasks,worktrees,diffs,sessions,agents,pullrequests}.go`, `internal/reaper/reaper.go`, `internal/github/{github,prlist,service}.go`, `internal/store/migrations/00007_github_foundations.sql`, `cmd/kangent/main.go`, `web/src/pages/TaskPage.tsx`, `web/src/components/task/{AgentTab,DescriptionTab,WorktreeMetaLine}.tsx`, `web/src/components/board/{PRCard,ReviewColumn}.tsx`, `web/src/api/{pullRequests,worktrees,queries,mutations,types}.ts`, `web/src/App.tsx`.
- **Phase research (HIGH):** `.planning/research/{PITFALLS,ARCHITECTURE,STACK}.md` (all gh/git mechanics independently re-verified above).
- `.planning/phases/12-open-a-review/{12-CONTEXT.md, 12-UI-SPEC.md}` (locked decisions + visual contract).

### Secondary (MEDIUM confidence)
- GitHub `refs/pull/<n>/head` convention (standard server-side ref; the fork-safe path) — corroborated by the live fork-PR verification.

### Tertiary (LOW confidence)
- None — every claim is backed by a live probe or a direct source read.

## Metadata

**Confidence breakdown:**
- Worktree checkout (GHREV-01/05): HIGH — verified live for same-repo and fork PRs; main HEAD unchanged both times.
- Diff base (GHREV-03): HIGH — fetch-before-merge-base requirement and three-dot mechanic verified live; renderer reused unchanged.
- Board-leak audit (GHREV-04): HIGH — exhaustive grep of `tasks` queries (16 sites, all classified); finite, enumerated surface.
- Open/reattach endpoint (GHREV-01/02): HIGH on mechanics (mirrors existing create/provision paths); MEDIUM on the transport/index trade (two reasonable options, flagged as Open Questions).
- Frontend wiring: HIGH — every prop/handle/route confirmed in source (`pasteApiRef`, App route, `useNavigate`, `prose` block).

**Research date:** 2026-06-14
**Valid until:** ~30 days (stack is stable; re-probe `gh pr view --json` field set only if `gh` is bumped past 2.82.0 — fields are additive).
