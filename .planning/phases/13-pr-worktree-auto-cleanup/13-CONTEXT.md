# Phase 13: PR Worktree Auto-Cleanup - Context

**Gathered:** 2026-06-14
**Status:** Ready for planning

<domain>
## Phase Boundary

Close the v1.3 loop: when a PR review's PR **merges or closes** on GitHub, its **clean, idle** review worktree is removed on its own by the always-on reaper — while a **dirty or busy** one is never silently removed (left for manual cleanup), and **the branch always survives**. Plus a **manual cleanup** affordance in the review view using the same gated flow tasks already use.

This is the last v1.3 phase. It operates on the artifacts Phase 12 created (PR tasks = `tasks` rows with `source='github_pr'` that own a worktree) and extends the Phase 9 reaper goroutine.

**Out of scope:** any new background goroutine (extend the existing reaper); cleaning up the local `pr/<n>` branch refs (branches are always kept — GHCLN-02); bulk/stale-worktree purge UIs; in-app GitHub writes.

</domain>

<decisions>
## Implementation Decisions

### Detection & mechanism (GHCLN-01) — locked by research
- **D-01 (reaper pass, locked by research §5):** Detection is **server-side and always-on** — add a second pass to the existing reaper tick (`internal/reaper`, the Phase 9 Done-TTL goroutine), NOT a new goroutine and NOT the browser poll (the poll pauses when hidden and only sees *open* review-requested PRs). The existing ~10-minute tick is fine (a worktree lingering a few minutes post-merge is harmless).
- **D-02 (PR state, locked by research):** For each `source='github_pr'` task that still has a worktree, the reaper calls a `PRState`-style read: `gh pr view <n> -R <repo> --json state` → `OPEN | CLOSED | MERGED` (UPPERCASE; `merged` is derived from `state == "MERGED"`, not a separate field). Define a **local `PRStateGetter` interface in the reaper package** (exactly like the existing `SessionStopper` seam) so tests inject a spy and the real `*github.Service` satisfies it. The reaper gains a dependency on the github service — wire it in `main.go`.
- **D-03:** Only `MERGED` or `CLOSED` triggers a cleanup attempt. `OPEN` (or any `gh` error / degraded state) → leave it untouched (degrade-don't-break). The PR pass is gated on `source='github_pr'` exactly as the Done pass is gated on `status='done'` — manual tasks' worktrees are NEVER auto-removed (D-87 preserved).

### Shared gated-cleanup helper — locked by research
- **D-04 (extract once, two callers, locked by research §5):** Extract the inline gate logic from `worktreeHandlers.remove` (`internal/api/worktrees.go`) into a shared helper (e.g. `cleanupWorktreeGated(ctx, ..., taskID, force) (removed bool, reason string)`) used by BOTH the manual HTTP path and the reaper. The reaper always passes **`force=false`** (auto-removal never forces past a gate). Refactor the HTTP handler to call the helper too — this is a regression-sensitive refactor of live cleanup code; guard it with the existing worktree/cleanup tests.

### Auto-removal safety gate (GHCLN-02) — CONSERVATIVE
- **D-05:** Auto-removal happens **only when the worktree is pristine AND idle**. Skip (leave for manual) if ANY of:
  1. **Uncommitted changes** — `git status --porcelain` non-empty (`wt.DirtyCount`).
  2. **Unpushed / local-only commits** — `git rev-list <headRefOid>..HEAD` non-empty (commits in the worktree not in the PR head ref). [research-flagged gate beyond porcelain]
  3. **A stash** — `git stash list` non-empty.
  4. **A running or detached session** — the folded `cleanupSessionCount` (in-memory sessions + live detached tmux survivors), including a live agent session.
  If any gate trips → **skip, leave the worktree**, log/record a reason; the user cleans up manually. Never force past a gate in the reaper.
- **D-06 (branch always kept, GHCLN-02 / D-34):** Cleanup removes the *worktree* only; the git branch ref (the named `pr/<n>` or head branch from Phase 12) is ALWAYS kept — the user may not own it (fork PRs) and we never destroy refs on their behalf. (Accumulating `pr/<n>` refs is acceptable for now; a stale-branch purge is a deferred idea.)

### PR task row fate after cleanup
- **D-07:** **Auto-cleanup deletes the task row; manual cleanup keeps it (nulled).**
  - **Reaper auto-cleanup** of a `MERGED`/`CLOSED` PR whose worktree was successfully removed → **DELETE the `tasks` row entirely** (the review is over; avoids invisible orphan rows accumulating — a merged PR leaves the Review column, so a kept row would be unreachable).
  - **Manual cleanup** (user-invoked, PR may still be open) → **keep the row, null `worktree_path`/`branch`/`worktree_error`** (the existing helper behavior) so reopening the PR re-provisions a fresh worktree.
  - Implementation note: the shared helper nulls the columns (manual semantics); the reaper performs the extra `DELETE` after a successful auto-remove of a merged/closed PR. The skipped (gated) case never deletes — the row + worktree both stay.

### Manual cleanup affordance (GHCLN-03)
- **D-08:** **Re-add the `⋯` menu to the PR review view** (Phase 12 D-10 omitted it) with a single **"Clean up worktree"** item that opens the **existing `CleanupWorktreeDialog`** (it already works on any `tasks.id` and enforces the gated flow with confirm/force/stop-sessions). No `Delete task` item for PR reviews. This reuses the manual cleanup path (D-07 manual semantics: keep row, null worktree).

### Merged/closed surfacing in the review view
- **D-09:** **Show a banner in the review view when the PR is no longer OPEN.** Plumb the PR `state` (OPEN/CLOSED/MERGED) into the Phase 12 read-only detail endpoint (`GET /api/projects/{id}/pull-requests/{n}` → add `state` to the wire/`PRDetailWire`). The review view shows an inline banner when `state != "OPEN"`:
  - worktree still present (skipped by a gate, or not yet reaped) → "This PR was {merged|closed}." + a manual **Clean up** prompt (reusing D-08).
  - This is the discoverability path for **skipped (dirty/busy) leftovers** — a merged PR with uncommitted work shows "merged — clean up manually" instead of silently lingering.
  - Clean auto-removed case: the reaper has deleted the row (D-07), so an open tab degrades to the existing "Task not found" path on next fetch — acceptable (the review is done). The banner primarily serves the skipped / not-yet-reaped window.
  Never a modal, never blocking — inline, degrade-don't-break.

### Claude's Discretion
- Exact helper signature/return shape for `cleanupWorktreeGated` and how the reaper signals "delete the row after auto-remove" (extra DELETE vs a helper flag).
- Whether `PRState` is a new `github.Service` method or `ViewPR` is extended with `state` (the detail endpoint needs `state` regardless for D-09 — a single source is fine).
- Exact banner copy and placement (inside the `shrink-0` header block, consistent with Phase 12's fix #6 height chain).
- Reaper batching/ordering of the two passes per tick; rate-limit friendliness of the per-PR `gh pr view` calls (cache/reuse the github service; a few calls per 10-min tick is negligible).
- Whether a snappier reconcile is triggered by the Review column's manual refresh (optional polish, not core).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Milestone research (v1.3) — the cleanup design is settled here
- `.planning/research/ARCHITECTURE.md` §5 "Auto-cleanup on merge/close" — the reaper second pass, `PRState` (`gh pr view --json state` → OPEN/CLOSED/MERGED), the `cleanupWorktreeGated` extraction (one path, two callers, `force=false`), and "what the reaper must NOT do" (never force a gate, never touch `source='manual'`, branch always kept). Also the `internal/reaper` EXTENDED note and the `PRStateGetter` local-interface seam.
- `.planning/research/PITFALLS.md` — cleanup gating, the unpushed-work gate beyond `git status --porcelain` (rev-list / stash), branch-kept rationale, degrade-don't-break.
- `.planning/research/STACK.md` — exact `gh pr view --json state` invocation.

### Requirements & roadmap
- `.planning/REQUIREMENTS.md` — GHCLN-01/02/03 (this phase).
- `.planning/ROADMAP.md` § "Phase 13: PR Worktree Auto-Cleanup" — goal + 3 success criteria (auto-cleanup test, safety test, manual cleanup) + the research flag.

### Prior-phase decisions to honor
- `.planning/phases/09-tmux-restart-resume-cleanup-integration/09-CONTEXT.md` — the Done-TTL reaper (REAP-01): background goroutine pattern, per-status timestamps, `SessionStopper` interface seam — the model this phase extends.
- `.planning/phases/12-open-a-review/12-CONTEXT.md` — PR review = `tasks` row `source='github_pr'`; the named `pr/<n>`/head branch (D-06 there); the `⋯` menu omitted for PR reviews (D-10 — re-added here); the `GET .../pull-requests/{n}` detail endpoint (extended with `state` here).

### Code to read before implementing
- `internal/reaper/reaper.go` — `Run` ticker + `reapOnce`; the `SessionStopper` interface seam (mirror it for `PRStateGetter`); how it queries `tasks` and calls `StopAllForTask`.
- `internal/api/worktrees.go` — `worktreeHandlers.remove` (the gate logic to extract: `cleanupSessionCount` folding detached tmux, `wt.DirtyCount`, `StopAllForTask`, `liveTmuxNames` + `KillSession`, `wt.Remove`, the `UPDATE tasks SET branch=NULL, worktree_path=NULL, worktree_error=NULL`).
- `internal/github/github.go` — `ViewPR`/`PRDetail` (add `state` / a `PRState` read).
- `internal/api/pullrequests.go` — the `GET .../pull-requests/{n}` detail handler (add `state` to the wire).
- `cmd/kangent/main.go` — reaper construction (`go reaper.New(db, mgr).Run(ctx)`) — wire the github service in.
- `web/src/pages/TaskPage.tsx` — re-add the `⋯` menu for PR reviews (Clean up worktree) + the merged/closed banner.
- `web/src/components/task/CleanupWorktreeDialog.tsx` — reused as-is for manual cleanup.
- `web/src/api/pullRequests.ts` — `PRDetailWire` (+`state`) / `usePullRequestDetail`.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/reaper` — the only background goroutine; one `Run` ticker, `reapOnce` per tick, a `SessionStopper` interface for test injection. Add a `reconcilePRsOnce` pass + a `PRStateGetter` interface alongside it.
- `worktreeHandlers.remove` (`internal/api/worktrees.go`) — the complete gated-cleanup sequence (sessions gate → dirty gate → StopAllForTask → kill detached tmux → `wt.Remove` → null columns). Extract it into `cleanupWorktreeGated` and call from both the handler and the reaper.
- `CleanupWorktreeDialog.tsx` — already gated (confirm/force/stop-sessions), works on any `tasks.id` — the GHCLN-03 manual UI, just needs an entry point.
- The Phase 12 `GET .../pull-requests/{n}` detail endpoint + `usePullRequestDetail` — extend with `state` to drive the banner.

### Established Patterns
- Reaper: per-status gating (`status='done'` → PR pass uses `source='github_pr'`), local interface seams for tests, warn-only on tmux failures, never block on best-effort steps.
- Cleanup: gates enforced at request time from fresh git state (Pitfall 8); `wt.Remove` self-heals a missing dir; branch ref never deleted.
- Shell-out: arg-array `exec.Command`, never `sh -c`; `gh ... --json` parsed as machine output.

### Integration Points
- `internal/reaper`: new PR-state reconcile pass per tick (needs the github service + the shared cleanup helper).
- `internal/api/worktrees.go`: extract `cleanupWorktreeGated`; HTTP handler refactored onto it.
- `internal/github` + `internal/api/pullrequests.go`: `PRState` / `state` on the detail endpoint.
- `cmd/kangent/main.go`: wire the github service into the reaper.
- `web` review view: `⋯` menu (Clean up) + merged/closed banner.

</code_context>

<specifics>
## Specific Ideas

- "Disappears on its own" should be literal for the clean case: a merged PR's pristine, idle review worktree is gone after the next reaper tick, and its task row with it — no UI action, no leftover.
- Safety is paramount and conservative: ANY uncommitted change, unpushed commit, stash, or live session means "don't touch it" — the user finds it via the merged/closed banner and cleans up by hand.
- The branch is sacred: never delete a branch ref on the user's behalf (fork PRs especially).
- Reuse, don't fork: one gated-cleanup code path for manual + auto; the existing dialog for manual; the existing reaper goroutine for detection.

</specifics>

<deferred>
## Deferred Ideas

- **Stale `pr/<n>` branch-ref purge** — branches are always kept this phase; a future maintenance pass could prune merged PR branches (ties to the parked MAINT-01 stale-worktree purge idea).
- **Snappier reconcile** — triggering a targeted PR-state reconcile from the Review column's manual refresh (optional polish; the 10-min reaper tick is the core mechanism).
- **Notifications** of auto-cleanup events — out of scope; the banner + silent removal are sufficient.

None of the discussion was scope creep — all four areas clarified HOW to implement the fixed Phase 13 scope.

</deferred>

---

*Phase: 13-pr-worktree-auto-cleanup*
*Context gathered: 2026-06-14*
