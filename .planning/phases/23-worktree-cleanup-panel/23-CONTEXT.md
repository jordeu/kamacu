# Phase 23: Worktree Cleanup Panel - Context

**Gathered:** 2026-07-02
**Status:** Ready for planning

<domain>
## Phase Boundary

A **Settings worktree-management panel** that gives the user a full accounting of
every worktree and the controls to reclaim them. Delivers WTREE-01..04:

- **WTREE-01** — list every worktree with its task/PR association,
  referenced-vs-orphaned status, and dirty / unpushed / stash flags.
- **WTREE-02** — per-item **force-remove**, overriding the dirty/unpushed/stash
  gates, behind a confirmation.
- **WTREE-03** — a bulk **"clean eligible"** action that removes all
  safely-removable worktrees in one action.
- **WTREE-04** — detect, list, and remove **orphaned** worktrees (present in
  `git worktree list` / on disk but with no matching DB task) — reconciling the
  accumulation the reaper currently skips.

Scope anchor: this is a **manual, global** panel. NOT in scope — scheduled/automatic
purge (WTREE-FUT-01), per-project cleanup settings, or app-driven privileged deletes.
</domain>

<decisions>
## Implementation Decisions

### Removal Safety & Permission-Blocked Worktrees
- **D-01 (blocked removal — the load-bearing gotcha):** Force-remove attempts a
  normal removal first; on a **permission error** caused by a path inside the
  worktree owned by another uid (e.g. a container-written `./.db` Postgres data dir,
  uid 70, mode 700), the app does **NOT** escalate privileges and does **NOT**
  partially deregister. It leaves the worktree in place, keeps its git registration
  and DB row intact, and marks it **"Blocked — needs manual removal"**, surfacing the
  exact offending path and a copyable `sudo rm -rf <path>` hint. (The app stays an
  unprivileged localhost process — no sudo/pkexec from the app.)
- **D-02 (branch always kept):** Removal reclaims the **worktree only**, never the
  branch — including for orphaned worktrees. Consistent with the existing D-34
  invariant. Orphan branches are not deleted.
- **D-03 (force = the single per-item override):** "Force-remove" passes BOTH the
  dirty gate (`force=true`) AND stops live sessions (`stopSessions=true`) — it is the
  "override every gate" action, always behind an explicit confirm that names what will
  be destroyed (uncommitted / unpushed / stash counts, running sessions).

### "Clean Eligible" Bulk Action
- **D-04 (eligible set):** Bulk "Clean eligible" removes **(a)** orphaned worktrees
  (no DB task) AND **(b)** referenced worktrees that pass **every** gate — no live
  session, not dirty, no unpushed commits, no stash — whose task is in the **Done**
  column OR whose **PR is merged/closed**. It **NEVER forces**: any worktree with a
  tripped gate, or that is blocked/undeletable (D-01), is skipped and left for a
  deliberate per-item force.
- **D-05 (never destructive in bulk):** Because bulk clean only touches
  provably-safe worktrees, one click is never destructive. The action shows a
  **preview/confirm** listing exactly which worktrees it will remove before running.

### Enumeration & Orphan Detection
- **D-06 (union scan, all repos):** The list is the **UNION** of
  `git worktree list --porcelain` (run at every project's repo root — folder repos
  AND managed clones) with the DB's task/PR worktree rows. **Grouped by project.**
- **D-07 (classification + reverse-orphans):** git-listed with no matching DB task =
  **ORPHAN** (WTREE-04). A DB row whose `worktree_path` is gone from disk / absent
  from git's list = **STALE POINTER** — surfaced with an action to clear the pointer
  (null the task's worktree columns, keep the row), reconciling DB↔disk drift.
- **D-08 (status flags):** Per-worktree **dirty / unpushed / stash** flags are
  computed for each worktree, reusing the existing `DirtyCount` / `UnpushedCount(wt, base)`
  / `StashCount` helpers. "Unpushed" resolves its base ref the way existing cleanup
  does (task branch base vs PR base) — exact ref resolution left to research/planning.

### Panel Placement, Loading & Scope
- **D-09 (placement):** A **new `<section>` in the existing full-page Settings route**
  (`SettingsPage.tsx`), consistent with the other section blocks — not a separate
  route. **Global** behavior (per-project cleanup settings are out of scope for v1.8).
- **D-10 (loading model):** One **GET endpoint returns the fully server-annotated
  worktree list** (association, referenced/orphaned/stale, dirty/unpushed/stash,
  blocked). The frontend **fetches on section open** with a **manual Refresh** button
  and **no polling** — mirrors the Diff tab's fetch-on-mount + manual-refresh (D-61)
  pattern. (Flags cost N×git calls; computing them server-side per GET keeps the
  client simple and makes refresh explicit.)

### Claude's Discretion
Left to research/planning: exact API shape/paths and endpoint naming; the
`git worktree list --porcelain` parser; precise base-ref resolution for "unpushed";
confirm-dialog copy and severity styling; empty-state text; whether the annotated data
is one GET or a list + per-row detail; and how the new "blocked" outcome is modeled
distinctly from a generic error.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase scope & requirements
- `.planning/ROADMAP.md` § "Phase 23: Worktree Cleanup Panel" — goal + the 4 success criteria (WTREE-01..04)
- `.planning/REQUIREMENTS.md` § "Worktree Cleanup (Settings panel)" — WTREE-01/02/03/04, WTREE-FUT-01, and the "Per-project diff-view or cleanup settings" out-of-scope row
- `.planning/PROJECT.md` § "Key Decisions" — the gated-cleanup invariants (sessions/dirty gates, branch-always-kept D-34, worktrees-never-auto-removed) this panel must honor

### Existing cleanup implementation (MUST read — the panel extends this, does not reinvent it)
- `internal/api/cleanup.go` — `CleanupWorktreeGated`: the shared gated-removal core (sessions + dirty gates, stop-sessions, kill-tmux, `wt.Remove`, null-columns). `force`/`stopSessions` semantics. The panel's per-item remove should call this path (a third caller after handler + reaper); it currently returns `err` on a `Remove` failure and does NOT model the D-01 "blocked" outcome.
- `internal/worktree/worktree.go` — `Remove(ctx, repo, wt, force)` (+ clean-but-submodules `--force` fallback), `DirtyCount`, `UnpushedCount(wt, base)` (`rev-list --count base..HEAD`), `StashCount`. No `git worktree list` parser exists yet — that enumeration is new.
- `internal/api/worktrees.go` — `worktreeHandlers` (POST/GET/DELETE `/api/tasks/{id}/worktree`), the `remove` handler (inline gated sequence CleanupWorktreeGated was extracted from), `liveTmuxNames`, `cleanupSessionCount`.
- `internal/reaper/reaper.go` — Done-TTL + PR reconcile passes; `liveTmuxNames` / session-count helpers; the general-orphan accumulation it currently skips (exactly WTREE-04's target).
- `internal/api/projects.go` § `removeManaged` — all-or-nothing gated managed-project delete across every worktree + clone root: a precedent for a multi-worktree gated bulk action.

### Frontend settings surface
- `web/src/pages/SettingsPage.tsx` — the `<section>` block pattern + page layout the panel slots into.
- `web/src/api/settings.ts` — `useSettings` / `useSaveSetting` (note: this panel is action-driven, not a KV setting — it likely needs its own query + mutation hooks, not `useSaveSetting`).
- `web/src/components/settings/SettingsField.tsx` — field/row primitive.
- `web/src/components/task/DiffTab.tsx` — the fetch-on-mount + spinning manual-Refresh, no-poll pattern (D-61) to mirror for the list.

### Real-world gotcha (auto-memory, not a repo doc)
- Memory `worktree-removal-blocked-by-container-owned-dirs` — the concrete uid-70 `./.db` case that D-01 is designed around (the `sched` project's dockerized Postgres bind-mount).
- Memory `falcon-may-kill-kangent` — unrelated to the feature, but relevant when live-testing the panel against a real install (CrowdStrike Falcon may SIGKILL `./bin/kamacu`).
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`CleanupWorktreeGated`** (`internal/api/cleanup.go`) — the exact per-worktree gated removal path; the panel's per-item remove calls it with `force`/`stopSessions` from the confirm. Extend only for (a) orphans (no `taskID`/no row to null) and (b) the D-01 "blocked" outcome.
- **Worktree gate helpers** (`DirtyCount` / `UnpushedCount` / `StashCount`) — surface as the row flags AND drive the "eligible" predicate (D-04).
- **`reaper.liveTmuxNames` / session-count helpers** — how to count live sessions per task (the session-gate input); the same probing the panel needs.
- **`projects.go removeManaged`** — precedent for an all-or-nothing gated bulk action across multiple worktrees.

### Established Patterns
- **One shared cleanup path, two callers** (HTTP handler + reaper) → the panel becomes a **third caller**; keep the single path (D-04 milestone convention).
- **Branch always kept; manual removal nulls task columns but keeps the row; reaper deletes the row** — the panel's manual removal follows the manual (keep-row) semantics.
- **Settings = full-page route of `<section>` blocks; server-computed data via a GET + manual refresh** (Diff tab) — no new UI paradigm.
- **git ops shell out with `--porcelain` machine-readable parsing** (never human output) — the new `git worktree list --porcelain` parser follows this rule.

### Integration Points
- **New endpoints**: an annotated **GET (list all worktrees)** + **per-item force-remove** + **bulk clean-eligible**, registered alongside `worktreeHandlers` in `routes.go`.
- **Cross-project enumeration**: orphan detection must enumerate across **all** projects' repo roots + managed clones (join the `projects` table for repo paths), not just one task's repo.
- **New "blocked" outcome**: `CleanupWorktreeGated` returns `err` on a `Remove` failure today; D-01 needs a distinct **blocked** classification (permission-denied on a foreign-uid path) rather than a generic 500, so the row can render "needs manual removal" with the path + hint.
</code_context>

<specifics>
## Specific Ideas

- **The concrete failure to design D-01 around:** the `sched` project's dockerized
  Postgres bind-mounts `./.db` into each worktree → a uid-70, mode-700 data dir →
  `git worktree remove` / `rm` gets `Permission denied`; only `sudo rm -rf` clears it
  (discovered 2026-07-01 cleaning stale worktree leftovers). The panel must degrade to
  "Blocked — needs manual removal" with the path + copyable `sudo rm -rf` hint, never
  a silent partial failure or a raw 500.
- **Mirror the Diff tab** (`DiffTab.tsx`): fetch on open, spinning manual Refresh
  button, no polling.
</specifics>

<deferred>
## Deferred Ideas

- **WTREE-FUT-01** — scheduled / automatic stale-worktree purge (the deferred
  MAINT-01), modeled on the reaper goroutine. This phase is a **manual** panel only.
- **App-driven elevated removal** (sudo/pkexec) — rejected for D-01; parked in case
  the manual-instruction path proves insufficient in practice.
- **Per-project cleanup settings** — out of scope for v1.8 (global behavior only, per
  REQUIREMENTS.md out-of-scope).

None of these are acted on in Phase 23.
</deferred>

---

*Phase: 23-worktree-cleanup-panel*
*Context gathered: 2026-07-02*
