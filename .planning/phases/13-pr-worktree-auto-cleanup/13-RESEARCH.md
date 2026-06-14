# Phase 13: PR Worktree Auto-Cleanup - Research

**Researched:** 2026-06-14
**Domain:** Server-side gated worktree cleanup (reaper extension + shared helper refactor) over `gh`/`git` CLI, in an existing Go + React local app (Kangent v1.3)
**Confidence:** HIGH (every claim below is grounded in the actual source files this phase touches, with exact line refs; `gh pr view --json state` and all four git gates were verified live on this host against real repos and synthetic worktrees)

This is the FINAL v1.3 phase. The two risk centers are confirmed and both have concrete, plan-ready answers below:
1. The **regression-sensitive extraction** of the live gated-cleanup sequence out of `worktreeHandlers.remove` into a shared helper called by both the HTTP handler and the reaper.
2. The **conservative auto-removal gate** — verified live that `git status --porcelain` alone is NOT sufficient (a committed-but-unpushed fixup is invisible to it), and that `git stash list` is repo-global, not per-worktree.

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Detection & mechanism (GHCLN-01):**
- **D-01:** Detection is server-side and always-on — add a second pass to the existing reaper tick (`internal/reaper`), NOT a new goroutine and NOT the browser poll. The existing ~10-minute tick is fine.
- **D-02:** For each `source='github_pr'` task that still has a worktree, the reaper calls a `PRState`-style read: `gh pr view <n> -R <repo> --json state` → `OPEN | CLOSED | MERGED` (UPPERCASE; `merged` is derived from `state == "MERGED"`, not a separate field). Define a **local `PRStateGetter` interface in the reaper package** (exactly like the existing `SessionStopper` seam) so tests inject a spy and the real `*github.Service` satisfies it. Wire the github service into the reaper in `main.go`.
- **D-03:** Only `MERGED` or `CLOSED` triggers a cleanup attempt. `OPEN` (or any `gh` error / degraded state) → leave it untouched (degrade-don't-break). The PR pass is gated on `source='github_pr'` exactly as the Done pass is gated on `status='done'` — manual tasks' worktrees are NEVER auto-removed (D-87 preserved).

**Shared gated-cleanup helper:**
- **D-04:** Extract the inline gate logic from `worktreeHandlers.remove` (`internal/api/worktrees.go`) into a shared helper (e.g. `cleanupWorktreeGated(ctx, ..., taskID, force) (removed bool, reason string)`) used by BOTH the manual HTTP path and the reaper. The reaper always passes **`force=false`**. Guard the refactor with the existing worktree/cleanup tests.

**Auto-removal safety gate (GHCLN-02) — CONSERVATIVE:**
- **D-05:** Auto-removal happens **only when the worktree is pristine AND idle**. Skip (leave for manual) if ANY of:
  1. Uncommitted changes — `git status --porcelain` non-empty (`wt.DirtyCount`).
  2. Unpushed / local-only commits — `git rev-list <headRefOid>..HEAD` non-empty.
  3. A stash — `git stash list` non-empty.
  4. A running or detached session — the folded `cleanupSessionCount`.
  If any gate trips → skip, leave the worktree, log/record a reason. Never force past a gate in the reaper.
- **D-06:** Cleanup removes the *worktree* only; the git branch ref (the named `pr/<n>` or head branch from Phase 12) is ALWAYS kept. Accumulating `pr/<n>` refs is acceptable for now.

**PR task row fate after cleanup:**
- **D-07:** Auto-cleanup deletes the task row; manual cleanup keeps it (nulled).
  - Reaper auto-cleanup of a MERGED/CLOSED PR whose worktree was successfully removed → DELETE the `tasks` row entirely.
  - Manual cleanup (user-invoked) → keep the row, null `worktree_path`/`branch`/`worktree_error` (existing helper behavior).
  - The skipped (gated) case never deletes — the row + worktree both stay.

**Manual cleanup affordance (GHCLN-03):**
- **D-08:** Re-add the `⋯` menu to the PR review view (Phase 12 D-10 omitted it) with a single "Clean up worktree" item that opens the existing `CleanupWorktreeDialog`. No `Delete task` item for PR reviews.

**Merged/closed surfacing in the review view:**
- **D-09:** Show a banner in the review view when the PR is no longer OPEN. Plumb the PR `state` into the Phase 12 detail endpoint (`GET /api/projects/{id}/pull-requests/{n}` → add `state` to the wire/`PRDetailWire`). Banner when `state != "OPEN"`: worktree still present → "This PR was {merged|closed}." + a manual Clean up prompt. Clean auto-removed case degrades to "Task not found". Never a modal, never blocking — inline, degrade-don't-break.

### Claude's Discretion
- Exact helper signature/return shape for `cleanupWorktreeGated` and how the reaper signals "delete the row after auto-remove" (extra DELETE vs a helper flag).
- Whether `PRState` is a new `github.Service` method or `ViewPR` is extended with `state` (the detail endpoint needs `state` regardless for D-09 — a single source is fine).
- Exact banner copy and placement (inside the `shrink-0` header block, consistent with Phase 12's fix #6 height chain).
- Reaper batching/ordering of the two passes per tick; rate-limit friendliness of the per-PR `gh pr view` calls.
- Whether a snappier reconcile is triggered by the Review column's manual refresh (optional polish, not core).

### Deferred Ideas (OUT OF SCOPE)
- Stale `pr/<n>` branch-ref purge — branches are always kept this phase.
- Snappier reconcile triggered from the Review column's manual refresh — optional polish.
- Notifications of auto-cleanup events — out of scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| **GHCLN-01** | When a PR is merged or closed on GitHub, its review worktree is removed automatically. | `gh pr view <n> -R <repo> --json state` verified live returns UPPERCASE `OPEN`/`CLOSED`/`MERGED` (§A). Reaper second pass (`reconcilePRsOnce`) mirrors the existing `reapOnce` structure (§D); `PRStateGetter` mirrors `SessionStopper` (reaper.go:48-55). |
| **GHCLN-02** | Auto-removal is gated — a dirty worktree, uncommitted/unpushed work, or running sessions block silent removal; the branch is always kept. | Four gates verified live (§C): `DirtyCount` (existing), `git rev-list <headOID>..HEAD` (committed-but-unpushed work is INVISIBLE to porcelain — proven §C2), `git stash list` (repo-global gotcha — §C3), `cleanupSessionCount` (existing). Branch kept: `wt.Remove` never deletes branches (worktree.go:284). |
| **GHCLN-03** | User can clean up a PR review worktree manually from the review view, using the same gated cleanup flow tasks already use. | `CleanupWorktreeDialog` is already rendered in TaskPage.tsx:595 and works on any `task.id`; only an entry point (a PR-specific `⋯` menu) is missing (§F). The dialog drives `DELETE /api/tasks/{id}/worktree` → the shared helper's manual-semantics path. |
</phase_requirements>

## Summary

Phase 13 closes the v1.3 loop with two backend changes and a small frontend change, all riding existing seams.

**Backend:** (1) Extract the inline gate sequence in `worktreeHandlers.remove` (worktrees.go:210-293) into a shared `cleanupWorktreeGated` helper that both the HTTP handler and the reaper call (D-04). (2) Add a `reconcilePRsOnce` pass to the reaper (mirroring `reapOnce`), driven by a new local `PRStateGetter` interface (mirroring the existing `SessionStopper` at reaper.go:48-55), wiring the github service into the reaper in main.go:175. The reaper calls the helper with `force=false`, then on a successful auto-remove of a MERGED/CLOSED PR, deletes the `tasks` row (D-07).

**Frontend:** Re-add a PR-specific `⋯` menu with one "Clean up worktree" item (D-08) — the `CleanupWorktreeDialog` and its `cleanupOpen` state already exist in TaskPage.tsx (lines 111, 595-602), only the menu entry is gated out by `!isPR` (line 520). Add `state` to `PRDetailWire` + the GET detail endpoint to drive a merged/closed banner inside the `shrink-0` header block (D-09).

**Primary recommendation:** Add `state` as a new field on `github.PRDetail`/`ViewPR` (single source, satisfies BOTH the reaper's `PRState` need via a thin `PRStateGetter` wrapper AND the D-09 detail-endpoint banner) — `gh pr view --json` is one call either way and the existing `ViewPR` already does exactly this `gh pr view` shape. The gate logic's load-bearing subtlety: `git status --porcelain` does NOT catch a committed-but-unpushed fixup (verified live) — the `git rev-list` gate is genuinely necessary, and `git stash list` is repo-global (a stash from the main checkout shows up in the PR worktree).

## Standard Stack

No new dependencies — Go stdlib + the already-authenticated host `gh`/`git` CLIs, exactly as the rest of the app.

### Core (all already present)
| Technology | Version | Purpose | Why Standard |
|------------|---------|---------|--------------|
| `gh` CLI (host) | 2.82.0 (verified) | `gh pr view <n> -R <repo> --json state` → MERGED/CLOSED/OPEN | Already the read surface for `internal/github`; soft dependency, degrade-don't-break |
| `git` CLI (host) | 2.43+ (any modern) | `rev-list`, `stash list`, `status --porcelain`, `worktree remove` | The existing worktree-service mechanic; the gate commands are read-only |
| `internal/github` | n/a | `ViewPR` / new `state` field / `PRStateGetter` wrapper | Existing leaf package; `ViewPR` already runs the right `gh pr view` shape |
| `internal/reaper` | n/a | second pass `reconcilePRsOnce` | Existing background goroutine — extended, not replaced (D-01) |

**Installation:** None. No `go.mod` change, no npm change. (`gh --version` → `gh version 2.82.0 (2025-10-15)`, verified.)

## Architecture Patterns

### Recommended structure (files this phase touches)
```
internal/github/github.go        # add `State` to PRDetail; ViewPR requests it; (optional) PRState wrapper
internal/api/worktrees.go        # extract cleanupWorktreeGated; remove() refactored onto it
internal/reaper/reaper.go        # add PRStateGetter interface + reconcilePRsOnce + cleanup dep
internal/api/pullrequests.go     # add state to prWire/prWireFrom
cmd/kangent/main.go              # wire ghSvc + cleanup deps into reaper.New(...)
web/src/api/pullRequests.ts      # add `state` to PRDetailWire
web/src/pages/TaskPage.tsx       # PR-specific ⋯ menu + merged/closed banner
```

### Pattern 1: The shared `cleanupWorktreeGated` helper (D-04 — the regression-sensitive refactor)

**The exact inline sequence in `worktreeHandlers.remove` today** (worktrees.go:210-293), in order:

1. Parse `{stop_sessions, force}` from the request body (lines 215-223). **HTTP-only** — stays in the handler.
2. `loadTaskRepo(id)` → task + repo_path (line 224). Errors → 404 / 500. **HTTP semantics** — see split below.
3. `t.WorktreePath == nil` → 404 "task has no worktree" (line 233). **HTTP-only.**
4. **Gate 1 (sessions):** `cleanupSessionCount(ctx, id) > 0 && !req.StopSessions` → 409 "sessions running" (line 243).
5. **Gate 2 (dirty):** stat the path, `wt.DirtyCount`, `dirty > 0 && !req.Force` → 409 "worktree has uncommitted changes" (line 257). A missing dir counts clean (line 250 — `Remove` self-heals).
6. 60s timeout context (line 262).
7. `mgr.StopAllForTask(id)` — blocks through the SIGTERM grace BEFORE remove (line 267). Ordering is load-bearing (Pitfall 4: git removes trees under live cwds).
8. Kill every live tmux session of the task (`liveTmuxNames` → `KillSession`) before `wt.Remove` (lines 274-278). Warn-only on failure.
9. `wt.Remove(ctx, repo, path, dirty > 0)` — `--force` only when the dirty gate was passed (line 281). 500 on failure.
10. `UPDATE tasks SET branch=NULL, worktree_path=NULL, worktree_error=NULL WHERE id=?` (line 287) — manual semantics: row stays, columns nulled.
11. `204 No Content` (line 292).

**Recommended split** (keep HTTP semantics in the handler, move the gate+remove core into the helper):

```go
// cleanupWorktreeGated runs the gated worktree-removal core shared by the HTTP
// handler and the reaper. force=false (the reaper) NEVER bypasses a gate.
// On a clean pass it stops sessions, kills live tmux, removes the worktree, and
// nulls the task columns (manual semantics — the row survives). It returns
// removed=false + a reason when a gate trips (and does NOT mutate anything).
//
// stopSessions/force come from the user-confirmed HTTP request; the reaper
// passes both false (D-05 conservative). The unpushed-commit + stash gates
// (D-05 gates b/c) are ADDED here and apply to BOTH callers (a stricter manual
// path is harmless — the dialog already gates on dirty+sessions, and these
// extra gates only matter for the force=false reaper path; see "force" note).
func cleanupWorktreeGated(
    ctx context.Context, db *sql.DB, wt *worktree.Service,
    mgr *session.Manager, tmuxClient tmux.Client,
    taskID int64, repo, path string,
    stopSessions, force bool,
) (removed bool, reason string, err error)
```

**Decision the planner must make on gate scope (Discretion + a correctness note):**
- D-05's gates b (rev-list) and c (stash) are the *reaper's* conservative gate. The *manual* HTTP path today gates only on dirty (porcelain) + sessions and lets the user `force` past dirty. If the helper enforces rev-list/stash for everyone, a user could be blocked from manually cleaning a worktree with a committed fixup — UNLESS `force=true` bypasses them. **Recommendation:** make `force` bypass the dirty + rev-list + stash gates together (they are all "uncommitted/local-only/stashed work" gates), and keep the sessions gate bypassed by `stopSessions`. The reaper passes `force=false, stopSessions=false`, so all four gates bind for auto-removal; the manual dialog's force/stop-sessions flags preserve today's HTTP behavior exactly. This keeps ONE code path with the reaper's extra gates dormant on the (forced) manual path.
  - **Simpler alternative (also valid):** keep gates b/c ONLY in the reaper pass (compute them before calling the helper, skip the helper if they trip), and leave the helper's gate set identical to today's (dirty + sessions). This is the lowest-regression-risk option for the helper extraction — the helper stays byte-equivalent to the current handler, and the two new gates live entirely in `reconcilePRsOnce`. Given D-04 calls the extraction "regression-sensitive," **this alternative is the safer default**: the helper is a pure mechanical extraction (verifiable against the existing worktree tests unchanged), and the new conservative gates are net-new reaper code.

**What stays in the handler (`remove`):** body parse, `loadTaskRepo`, the 404s (no-task / no-worktree), mapping the helper's `(removed, reason)` back to HTTP (a tripped gate → the same 409 strings it returns today: "sessions running" / "worktree has uncommitted changes"; a remove failure → 500; success → 204). The handler keeps owning the 409 gate responses by inspecting the helper's return rather than the helper writing HTTP.

**Dependencies the helper needs and where they live:** `db *sql.DB`, `wt *worktree.Service`, `mgr *session.Manager`, `tmuxClient tmux.Client` — all four are already fields on `worktreeHandlers` (worktrees.go:36-41) and all four are already constructed in main.go (`db`, `wtSvc`, `mgr`, `tmuxClient` at main.go:60/109/111/119). The reaper currently holds only `db` + `mgr` (as `SessionStopper`), so it must gain `wt`, `tmuxClient`, and the github service (see Pattern 2 + Environment wiring).
  - **Helper placement:** put `cleanupWorktreeGated` in `internal/api` (it uses `*session.Manager`, `tmux.Client`, `*worktree.Service`, and the `liveTmuxNames`/`cleanupSessionCount` helpers that are methods on `worktreeHandlers`). The reaper then imports `internal/api`. **Watch for an import cycle:** `internal/api` does NOT currently import `internal/reaper` (verified — main.go wires them separately), so `reaper → api` is acyclic and safe. Alternatively, lift the helper into a new small `internal/cleanup` package that both `api` and `reaper` import — cleaner dependency graph, more files. Planner's discretion; `reaper → api` is the smaller change.

### Pattern 2: Reaper second pass `reconcilePRsOnce` (D-01/D-02/D-03)

The existing reaper (reaper.go) is a clean template. Mirror it exactly:

```go
// PRStateGetter is the slice of the github service the reaper needs. Defined
// LOCALLY (like SessionStopper at reaper.go:48) so a test injects a spy and the
// real *github.Service satisfies it. Returns "OPEN"|"CLOSED"|"MERGED" or an
// error (degrade: a gh failure is warn-and-skip, never a crash — D-03).
type PRStateGetter interface {
    PRState(ctx context.Context, repo string, n int) (string, error)
}
```

**`reconcilePRsOnce(ctx)` structure** (parallel to `reapOnce`, reaper.go:93-158):
1. `SELECT t.id, t.pr_number, p.github_repo FROM tasks t JOIN projects p ON p.id = t.project_id WHERE t.source='github_pr' AND t.worktree_path IS NOT NULL AND p.github_repo IS NOT NULL`. (The `source='github_pr'` gate is D-03's analogue of `reapOnce`'s `status='done'` — manual tasks are never touched.)
2. For each row: `state, err := pr.PRState(ctx, repo, n)`. On `err` → `slog.Warn` and `continue` (degrade-don't-break, D-03).
3. If `state == "OPEN"` → skip (leave it). Only `MERGED`/`CLOSED` proceed (D-03). (Defensive: any other/unknown value → skip.)
4. **The conservative gate** (D-05 — see §C for exact commands): compute the two new gates (rev-list unpushed, stash) and call `cleanupWorktreeGated(..., taskID, repo, path, stopSessions=false, force=false)`. If the helper returns `removed=false` (a gate tripped) → skip and `slog.Info` the reason (the user finds it via the D-09 banner).
5. If `removed == true` → DELETE the task row (D-07 — see §E for the FK ordering).

**Run wiring:** call `reconcilePRsOnce(ctx)` from `Run` alongside `reapOnce` — both at the immediate-start call (reaper.go:77) and inside the ticker loop (reaper.go:85). Discretion: run them sequentially per tick (negligible cost; a few `gh pr view` calls per 10-min tick is well within the 5000/hr core budget — STACK.md verified). The 10s `gh pr view` calls are off any request path (background goroutine), so latency is irrelevant.

### Pattern 3: `state` as a single source (Discretion → recommended)

`ViewPR` (github.go:129) already runs `gh pr view <n> -R <repo> --json number,title,body,author,url,headRefName,headRefOid,baseRefName,baseRefOid,isCrossRepository,commits`. Add `state` to that field list and to `PRDetail`:

```go
// in PRDetail (github.go:106):
State string `json:"state"` // "OPEN" | "CLOSED" | "MERGED" (UPPERCASE — verified)
```
Then the reaper's `PRState` can be a thin wrapper: `func (s *Service) PRState(ctx, repo, n) (string, error) { d, err := github.ViewPR(ctx, repo, n); return d.State, err }` — OR a dedicated minimal `gh pr view <n> --json state` method (one fewer field fetched per call). Either satisfies `PRStateGetter`. The D-09 banner needs `state` on the detail-endpoint wire regardless, so adding it to `PRDetail`/`ViewPR` is the single-source win. Then `prWire` (pullrequests.go:277) + `prWireFrom` (pullrequests.go:292) gain a `State string \`json:"state"\`` field, and `PRDetailWire` (pullRequests.ts:61) gains `state: "OPEN" | "CLOSED" | "MERGED"`.

### Anti-Patterns to Avoid
- **Forcing past a gate in the reaper.** The reaper MUST pass `force=false, stopSessions=false`. A live session BLOCKS auto-removal (it is NOT killed). (D-05, ARCHITECTURE §5 "what the reaper must NOT do".)
- **Deleting the branch.** `wt.Remove` never deletes branches (worktree.go:284) — do not add any `git branch -D`. The accumulating `pr/<n>` refs are accepted (D-06).
- **Relying on `git status --porcelain` alone for the unpushed gate.** Verified live: a committed-but-unpushed fixup shows porcelain count 0 (§C2). The rev-list gate is mandatory.
- **Requesting a `merged` boolean from `gh pr view --json`.** Verified live: `gh pr view --json merged` errors `Unknown JSON field: "merged"`. Use `state == "MERGED"`.
- **Reaching the row-delete before tmux_sessions rows are gone.** FKs are ON (store.go:16) with no CASCADE — `DELETE FROM tasks` fails if a `tmux_sessions` row references it (§E).
- **Detecting merges from the browser poll.** Server-side only (D-01, ARCHITECTURE Anti-Pattern 3).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Gated worktree removal | A second copy of the gate/kill/remove sequence in the reaper | Extract `cleanupWorktreeGated`, one path two callers (D-04) | The inline sequence (worktrees.go:243-291) has load-bearing ordering (stop-before-remove, kill-tmux-before-remove) + self-heal edge cases (`Remove` idempotency, missing-dir-is-clean) that are easy to get subtly wrong twice |
| Test seam for the github dependency | A real `*github.Service` in reaper tests | A local `PRStateGetter` interface + a spy (mirror `SessionStopper`, reaper.go:48 + reaper_test.go:18) | The reaper test pattern already exists and is hermetic (no `gh` spawn in unit tests) |
| Dirty detection | A custom `git status` parse | `wt.DirtyCount` (worktree.go:269) | Already uses `--porcelain=v2 --untracked-files=all -z` and counts untracked individually — matches exactly what `worktree remove` refuses on |
| Session + detached-tmux counting | A new count | `cleanupSessionCount` (worktrees.go:115) | Already folds in-memory running sessions + live detached tmux survivors into one honest number, with the `HasLiveTmux` double-count guard |
| Worktree removal mechanics | `os.RemoveAll` + bookkeeping | `wt.Remove` (worktree.go:301) | Handles submodule `--force` retry, missing-dir idempotency, and `worktree prune` |
| Manual cleanup UI | A new dialog | `CleanupWorktreeDialog` (already rendered, TaskPage.tsx:595) | Already gated (confirm/force/stop-sessions), works on any `task.id`, fetches FRESH server state on open |

**Key insight:** This phase is almost entirely *reuse + extract*. The only genuinely new logic is the two extra git gates (rev-list, stash) and the reaper's `reconcilePRsOnce` loop. Everything else is moving existing, tested code behind a shared helper.

## Common Pitfalls

### Pitfall 1: The unpushed-commit gate is invisible to `git status --porcelain` (verified live)
**What goes wrong:** A reviewer commits a local fixup in the PR worktree (`git commit`) but never pushes it. `git status --porcelain` reports a CLEAN tree (count 0). If the gate is only `DirtyCount`, the reaper auto-removes the worktree and the commit is silently lost (it was never on a remote).
**Verified (this host):** in a worktree with one local-only commit on top of the PR head, `git status --porcelain | wc -l` → `0` while `git rev-list <headOID>..HEAD | wc -l` → `1`.
**How to avoid:** D-05 gate b. The reaper must run `git rev-list <PR-head>..HEAD` and skip if non-empty. **The correct base to diff against:** the PR's head commit. But `headRefOid` is NOT stored in the DB (the `tasks` table has only `source`/`pr_number`/`pr_base_ref` — migration 00007; verified no `pr_head_oid` column anywhere). The worktree was checked out on a NAMED branch pinned to `headOID` at open time (CheckoutPR, worktree.go:249), so the recovery options are:
  - **(Recommended) Re-fetch the PR head and diff against it:** `git -C <repo> fetch origin refs/pull/<n>/head` then `git rev-list FETCH_HEAD..<wt-HEAD>`. Verified live end-to-end (§C2) — a clean worktree gives 0, a local fixup gives 1. This is exactly the fetch CheckoutPR already does, and works for forks (the pull ref lives on the base repo). It also correctly handles a force-pushed PR head (compares against the *current* remote head).
  - **(Alternative) Diff against the PR base merge-base:** `git rev-list origin/<pr_base_ref>..HEAD` after `FetchRef(base)` — but this counts the PR's own commits as "unpushed," which is wrong (the PR's commits ARE pushed). Do NOT use the base for this gate.
  - The reaper already calls `PRState` per PR; adding one `fetch refs/pull/<n>/head` per merged/closed PR is cheap and only fires for non-OPEN PRs.
**Warning signs:** A user reports "my review fixup vanished after the PR merged."

### Pitfall 2: `git stash list` is repo-global, not per-worktree (verified live)
**What goes wrong:** `git stash list` run inside the PR worktree returns ALL stashes in the whole repository, including stashes created in the main checkout or any other worktree. Gating the PR worktree's auto-removal on `git stash list` non-empty will therefore SKIP a perfectly clean PR worktree whenever the user happens to have an unrelated stash anywhere in the repo.
**Verified (this host):** a stash created in the main checkout appears in the PR worktree's `git stash list` (count 1 in both); a stash created in the PR worktree appears in the main checkout's list (count went 1→2 across both). Stashes are stored as `refs/stash` in the shared common git dir, so they are not worktree-scoped.
**How to avoid (planner decision — flag explicitly):**
  - **(Conservative, recommended)** Keep the `git stash list` non-empty gate as written in D-05. Consequence: a clean PR worktree won't auto-remove while ANY stash exists in the repo — it lingers until the next tick after the stash is cleared, and the user can always remove it manually via the D-08 menu. This is safe (never destroys work) and matches D-05's "conservative" intent, at the cost of some false-positive skips. **This is the safest reading of the locked decision and what D-05 literally says.**
  - **(Precise but harder)** There is no reliable, portable way to attribute a stash to a specific worktree — `git stash` does not record originating worktree in a queryable way (the stash commit's parentage points at the HEAD it was taken from, which *can* be compared, but it is fragile across git versions and not worth the complexity for a single-user local app).
  - **Recommendation:** implement the simple repo-global `git stash list` non-empty gate (honors D-05 verbatim), and document the false-positive in a code comment + the plan's acceptance notes. It only ever causes a *too-conservative* skip (never a destructive removal), which is the correct failure direction for this gate. The user's escape hatch is the manual D-08 cleanup.
**Warning signs:** Merged-PR worktrees never auto-remove on a machine where the user keeps long-lived stashes — explainable, not a bug, but document it.

### Pitfall 3: `DELETE FROM tasks` fails the FK constraint if tmux_sessions rows remain (verified)
**What goes wrong:** D-07's reaper row-delete (`DELETE FROM tasks WHERE id=?`) will fail with a foreign-key violation if any `tmux_sessions` row still references the task. `foreign_keys` PRAGMA is ON (store.go:16: `_pragma=foreign_keys(1)`), and `tmux_sessions.task_id` is `REFERENCES tasks(id)` with NO `ON DELETE CASCADE` (migration 00005:9).
**Why it happens:** `cleanupWorktreeGated` (extracted from `remove`) KILLS tmux sessions but does NOT delete `tmux_sessions` ROWS — manual cleanup keeps the task row, so the rows can stay (they're lazily GC'd / swept). For the reaper's auto-delete, the rows must go first.
**How to avoid:** The reaper's post-remove delete must mirror `taskHandlers.delete` (tasks.go:481-499), which does exactly this ordering: kill sessions → `DELETE FROM tmux_sessions WHERE task_id=?` → `DELETE FROM tasks WHERE id=?`. So the reaper, after a successful `cleanupWorktreeGated`, must `DELETE FROM tmux_sessions WHERE task_id=?` BEFORE `DELETE FROM tasks WHERE id=?`. (The sessions are already killed by the helper; this just clears the rows so the FK is satisfied.) **No other FK references `tasks(id)`** — `claude_session_id` is a column ON `tasks` (migration 00003), not a separate table, so it goes with the row. There are no `agent_sessions` rows to clean (agents are tracked in-memory + the `tasks.claude_session_id` column).
**Warning signs:** Reaper logs `FOREIGN KEY constraint failed` and the merged PR's task row never disappears.

### Pitfall 4: A live agent/bash/tmux session must BLOCK auto-removal, not be killed
**What goes wrong:** Copying the manual path's "stop sessions then remove" into the reaper would silently kill a running review agent.
**How to avoid:** The reaper passes `stopSessions=false`. With `cleanupSessionCount > 0` and `stopSessions=false`, the sessions gate trips → the helper returns `removed=false, reason="sessions running"` → reaper skips. The session is never touched (D-05 gate 4). This is automatic if the helper's sessions gate is `count > 0 && !stopSessions` (mirroring worktrees.go:243).

## Code Examples

### Verified: `gh pr view --json state` for all three states (this host, gh 2.82.0)
```bash
# Source: live execution against cli/cli, 2026-06-14
$ gh pr view 13642 -R cli/cli --json state
{"state":"OPEN"}
$ gh pr view 13632 -R cli/cli --json number,state,mergedAt
{"mergedAt":"2026-06-10T22:22:38Z","number":13632,"state":"MERGED"}   # MERGED → mergedAt populated
$ gh pr view 13656 -R cli/cli --json state
{"state":"CLOSED"}                                                      # closed-without-merge
$ gh pr view 13632 -R cli/cli --json merged
Unknown JSON field: "merged"                                           # no `merged` field — derive from state
```

### Verified: the unpushed-commit gate beyond porcelain (this host, synthetic worktree)
```bash
# Source: live test, 2026-06-14 — a worktree pinned to the PR head (CheckoutPR shape),
# reaper re-fetches refs/pull/<n>/head, then rev-lists local-only commits.
git -C <repo> fetch origin refs/pull/<n>/head           # the same fetch CheckoutPR uses; fork-safe
PR_HEAD=$(git -C <repo> rev-parse FETCH_HEAD)
git -C <wt> rev-list "$PR_HEAD"..HEAD | wc -l            # 0 on a clean wt → gate PASSES (safe to remove)
                                                         # >0 after a local commit → gate TRIPS (skip)
# Proof the rev-list gate is necessary:
git -C <wt> status --porcelain | wc -l                   # 0 even WITH a committed local fixup
```

### Verified: stash is repo-global (this host)
```bash
# Source: live test, 2026-06-14 — stash created in main checkout, listed from the PR worktree.
git -C <main-checkout> stash push -m x
git -C <pr-worktree> stash list | wc -l                  # 1 — the main checkout's stash IS visible here
```

### The existing inline gate sequence to extract (worktrees.go:243-291, abridged)
```go
// Source: internal/api/worktrees.go (the D-04 extraction target)
if h.cleanupSessionCount(r.Context(), id) > 0 && !req.StopSessions {
    writeError(w, http.StatusConflict, "sessions running"); return     // Gate 1
}
dirty := 0
if _, statErr := os.Stat(path); statErr == nil {
    dirty, err = h.wt.DirtyCount(r.Context(), path); /* 500 on err */
}
if dirty > 0 && !req.Force {
    writeError(w, http.StatusConflict, "worktree has uncommitted changes"); return  // Gate 2
}
ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second); defer cancel()
h.mgr.StopAllForTask(id)                                                // stop BEFORE remove
for _, name := range h.liveTmuxNames(ctx, id) { _ = h.tmuxClient.KillSession(ctx, name) }  // kill tmux BEFORE remove
if err := h.wt.Remove(ctx, repo, path, dirty > 0); err != nil { /* 500 */ }
h.db.Exec(`UPDATE tasks SET branch=NULL, worktree_path=NULL, worktree_error=NULL WHERE id=?`, id)  // manual semantics
```

### Frontend: the entry point that already exists (TaskPage.tsx)
```tsx
// Source: web/src/pages/TaskPage.tsx — the dialog + state are ALREADY present (lines 111, 595-602):
const [cleanupOpen, setCleanupOpen] = useState(false);          // line 111
// ... <CleanupWorktreeDialog open={cleanupOpen} onOpenChange={setCleanupOpen}
//       taskId={task.id} taskTitle={task.title} projectId={projectId} trigger="menu" />  // line 595
// The ⋯ menu is gated out for PRs at line 520 ({!isPR && (...)}). D-08 = a PR-specific
// menu (or relaxing the gate) with a single onSelect={() => setCleanupOpen(true)} item.
```

## State of the Art

| Old (Phase 12) | New (Phase 13) | Impact |
|----------------|----------------|--------|
| `⋯` menu omitted for PR reviews (D-10) | PR-specific `⋯` with one "Clean up worktree" item (D-08) | The dialog + state already exist; only the menu entry is added |
| `worktreeHandlers.remove` owns the gate sequence inline | Shared `cleanupWorktreeGated`, two callers (D-04) | Reaper reuses the exact gated path; no duplicated mechanics |
| Reaper has one pass (`reapOnce`, Done-TTL) | Two passes: `reapOnce` + `reconcilePRsOnce` (D-01) | Merged/closed PR worktrees self-clean server-side |
| `PRDetail`/`prWire`/`PRDetailWire` lack `state` | `state` added through the whole chain (D-09) | Drives both the reaper gate and the review banner |
| PR worktrees never auto-removed (Phase 12 deferred) | Pristine+idle merged/closed PR worktrees auto-removed, branch kept (GHCLN-01/02) | The v1.3 loop closes |

**Deprecated/outdated:** nothing — this phase is additive and reuse-heavy.

## Open Questions

1. **Helper gate scope (rev-list/stash in the helper vs reaper-only)?**
   - What we know: D-04 mandates the extraction; D-05 mandates the four gates for *auto*-removal. The manual HTTP path historically gates only on dirty+sessions with a `force` bypass.
   - What's unclear: whether the two new gates live in the shared helper (applying to both callers, with `force` bypassing them) or only in `reconcilePRsOnce` (helper stays a pure mechanical extraction).
   - Recommendation: keep the new gates in `reconcilePRsOnce` (reaper-only); the helper is a byte-equivalent extraction of today's dirty+sessions logic, verifiable against the unchanged worktree tests. This is the lowest-regression-risk reading of "regression-sensitive refactor." (Either is defensible — documented in Pattern 1.)

2. **Helper placement: `internal/api` (reaper imports api) vs a new `internal/cleanup` package?**
   - What we know: the helper needs `*session.Manager`, `tmux.Client`, `*worktree.Service`, `*sql.DB`, and the `liveTmuxNames`/`cleanupSessionCount` methods (currently on `worktreeHandlers`). `reaper → api` is acyclic today (verified). 
   - Recommendation: `internal/api` for the smaller change, unless the planner prefers a dedicated package for graph cleanliness. Either works.

3. **PRState as `ViewPR(+state)` wrapper vs a dedicated `gh pr view --json state` method?**
   - What we know: the D-09 banner needs `state` on the detail wire regardless. The reaper needs only the string.
   - Recommendation: add `state` to `PRDetail`/`ViewPR` (single source) and give the reaper a thin `PRState` wrapper, OR a minimal dedicated method that fetches only `state` (one fewer JSON field per reaper call). Negligible either way at this scale. Minor lean toward the dedicated minimal method so the reaper's per-PR call is the smallest possible.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| `gh` CLI | PRState read (GHCLN-01) | ✓ | 2.82.0 | `Available()` false → `PRState` errors → reaper warn-and-skips (D-03 degrade); no auto-cleanup, manual D-08 still works |
| `git` CLI | rev-list / stash / status / worktree remove gates | ✓ | (host git, modern) | None needed — git is the app's hard premise |
| GitHub auth (`gh`) | `gh pr view` resolving private PRs | ✓ (`✓ Logged in to github.com`) | — | Auth failure → `gh pr view` nonzero → `ViewPR`/`PRState` error → reaper skip (degrade) |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** if `gh` is absent/unauthenticated at runtime, the reaper's PR pass degrades to a no-op (warn-and-skip per tick); the Done-TTL pass and all manual flows are unaffected — exactly the degrade-don't-break posture (GHSET-03, D-03).

## Sources

### Primary (HIGH confidence)
- Live codebase inspection (HIGH) — `internal/reaper/reaper.go` (+`reaper_test.go`), `internal/api/{worktrees,pullrequests,tasks,diffs}.go`, `internal/github/{github,service}.go`, `internal/worktree/worktree.go`, `internal/store/migrations/{00005,00007}*.sql`, `internal/store/store.go`, `cmd/kangent/main.go`, `web/src/pages/TaskPage.tsx`, `web/src/components/task/CleanupWorktreeDialog.tsx`, `web/src/api/pullRequests.ts` — with exact line refs throughout.
- Live `gh` 2.82.0 on host (HIGH): `gh pr view <n> -R cli/cli --json state` returns UPPERCASE `OPEN`/`CLOSED`/`MERGED`; MERGED populates `mergedAt`/`closed:true`; `--json merged` errors `Unknown JSON field`. Verified 2026-06-14.
- Live `git` gate tests on host (HIGH, 2026-06-14): committed-but-unpushed fixup → `rev-list <head>..HEAD`=1 while `status --porcelain`=0; `git stash list` is repo-global (a main-checkout stash is visible from the PR worktree and vice versa); end-to-end reaper gate (re-fetch `refs/pull/<n>/head` + rev-list) gives 0 on clean / 1 on local commit.
- `foreign_keys` PRAGMA ON (store.go:16) + `tmux_sessions.task_id REFERENCES tasks(id)` no CASCADE (migration 00005:9) → `taskHandlers.delete` ordering (tasks.go:481-499) is the canonical row-delete model.

### Secondary (MEDIUM confidence)
- `.planning/research/{ARCHITECTURE,PITFALLS,STACK}.md` — the v1.3 milestone research (§5 auto-cleanup, the unpushed/stash gate flag, `gh pr view --json state`), corroborated by the live verification above.

### Tertiary (LOW confidence)
- None. All load-bearing claims verified live or against source.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — no new deps; all CLIs verified present and at the expected versions.
- Architecture (helper extraction + reaper pass): HIGH — exact inline sequence mapped line-by-line; reaper template + test seam already exist.
- Pitfalls (gates, FK, stash-global): HIGH — all three subtleties verified live, not recalled.

**Research date:** 2026-06-14
**Valid until:** 2026-07-14 (stable — host CLIs and the codebase seams are settled; re-verify only if `gh` or the schema changes)
