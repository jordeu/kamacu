# Phase 23: Worktree Cleanup Panel - Research

**Researched:** 2026-07-02
**Domain:** Go worktree enumeration + gated removal (extending existing cleanup core); React 19 + TanStack Query settings panel
**Confidence:** HIGH — this phase extends code that already exists in-repo; the two genuinely new pieces (`git worktree list --porcelain` parsing, D-01 permission-blocked detection) were verified empirically against git 2.43.0 and the Go stdlib on this machine.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Removal Safety & Permission-Blocked Worktrees**
- **D-01 (blocked removal — the load-bearing gotcha):** Force-remove attempts a normal removal first; on a **permission error** caused by a path inside the worktree owned by another uid (e.g. a container-written `./.db` Postgres data dir, uid 70, mode 700), the app does **NOT** escalate privileges and does **NOT** partially deregister. It leaves the worktree in place, keeps its git registration and DB row intact, and marks it **"Blocked — needs manual removal"**, surfacing the exact offending path and a copyable `sudo rm -rf <path>` hint. (The app stays an unprivileged localhost process — no sudo/pkexec from the app.)
- **D-02 (branch always kept):** Removal reclaims the **worktree only**, never the branch — including for orphaned worktrees. Consistent with the existing D-34 invariant. Orphan branches are not deleted.
- **D-03 (force = the single per-item override):** "Force-remove" passes BOTH the dirty gate (`force=true`) AND stops live sessions (`stopSessions=true`) — it is the "override every gate" action, always behind an explicit confirm that names what will be destroyed (uncommitted / unpushed / stash counts, running sessions).

**"Clean Eligible" Bulk Action**
- **D-04 (eligible set):** Bulk "Clean eligible" removes **(a)** orphaned worktrees (no DB task) AND **(b)** referenced worktrees that pass **every** gate — no live session, not dirty, no unpushed commits, no stash — whose task is in the **Done** column OR whose **PR is merged/closed**. It **NEVER forces**: any worktree with a tripped gate, or that is blocked/undeletable (D-01), is skipped and left for a deliberate per-item force.
- **D-05 (never destructive in bulk):** Because bulk clean only touches provably-safe worktrees, one click is never destructive. The action shows a **preview/confirm** listing exactly which worktrees it will remove before running.

**Enumeration & Orphan Detection**
- **D-06 (union scan, all repos):** The list is the **UNION** of `git worktree list --porcelain` (run at every project's repo root — folder repos AND managed clones) with the DB's task/PR worktree rows. **Grouped by project.**
- **D-07 (classification + reverse-orphans):** git-listed with no matching DB task = **ORPHAN** (WTREE-04). A DB row whose `worktree_path` is gone from disk / absent from git's list = **STALE POINTER** — surfaced with an action to clear the pointer (null the task's worktree columns, keep the row), reconciling DB↔disk drift.
- **D-08 (status flags):** Per-worktree **dirty / unpushed / stash** flags are computed for each worktree, reusing the existing `DirtyCount` / `UnpushedCount(wt, base)` / `StashCount` helpers. "Unpushed" resolves its base ref the way existing cleanup does (task branch base vs PR base) — exact ref resolution left to research/planning.

**Panel Placement, Loading & Scope**
- **D-09 (placement):** A **new `<section>` in the existing full-page Settings route** (`SettingsPage.tsx`), consistent with the other section blocks — not a separate route. **Global** behavior (per-project cleanup settings are out of scope for v1.8).
- **D-10 (loading model):** One **GET endpoint returns the fully server-annotated worktree list** (association, referenced/orphaned/stale, dirty/unpushed/stash, blocked). The frontend **fetches on section open** with a **manual Refresh** button and **no polling** — mirrors the Diff tab's fetch-on-mount + manual-refresh (D-61) pattern.

### Claude's Discretion
Left to research/planning: exact API shape/paths and endpoint naming; the `git worktree list --porcelain` parser; precise base-ref resolution for "unpushed"; confirm-dialog copy and severity styling; empty-state text; whether the annotated data is one GET or a list + per-row detail; and how the new "blocked" outcome is modeled distinctly from a generic error.

### Deferred Ideas (OUT OF SCOPE)
- **WTREE-FUT-01** — scheduled / automatic stale-worktree purge (the deferred MAINT-01), modeled on the reaper goroutine. This phase is a **manual** panel only.
- **App-driven elevated removal** (sudo/pkexec) — rejected for D-01; parked in case the manual-instruction path proves insufficient in practice.
- **Per-project cleanup settings** — out of scope for v1.8 (global behavior only, per REQUIREMENTS.md out-of-scope).
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| WTREE-01 | Settings lists every worktree with task/PR association, referenced-vs-orphaned status, and dirty/unpushed/stash flags | New `git worktree list --porcelain -z` parser (§Pattern 1) UNION'd with the DB task rows (§Pattern 2); flags reuse `DirtyCount`/`UnpushedCount`/`StashCount` (`worktree.go:288/307/326`) with base-ref resolution mirroring `diffs.go` (§Pattern 3); one annotated GET (§API Shape) |
| WTREE-02 | Force-remove an individual worktree, overriding dirty/unpushed/stash gates, behind a confirmation | Per-item POST calling `CleanupWorktreeGated(..., stopSessions=true, force=true)` (`cleanup.go:36`) — a third caller; confirm dialog mirrors `CleanupWorktreeDialog.tsx`; D-01 blocked outcome (§Pattern 4) modeled distinctly from a 500 |
| WTREE-03 | Bulk "clean eligible" removes all safely-removable worktrees in one action | Server-side eligibility predicate (§Pattern 5: orphans + fully-gate-passing done/merged), NEVER forces; two-pass preview→execute mirroring `projects.go removeManaged` (`projects.go:682`) |
| WTREE-04 | Orphaned worktrees (on disk / in `git worktree list`, no DB task) detected, listed, removable | Union scan surfaces git-listed-with-no-DB-task rows (§Pattern 2); orphan removal calls `CleanupWorktreeGated` with `taskID=0`/no-null path — the helper must be extended to skip the null-columns UPDATE when there is no task (§Don't Hand-Roll / §Open Q) |
</phase_requirements>

## Summary

Phase 23 is an **extension phase, not a greenfield one**. Every removal mechanism it needs already exists and is battle-tested: the gated core `CleanupWorktreeGated` (`internal/api/cleanup.go`), the git primitives `Remove`/`DirtyCount`/`UnpushedCount`/`StashCount` (`internal/worktree/worktree.go`), the session-count/live-tmux probing (`worktreeHandlers` in `internal/api/worktrees.go`), and the multi-worktree gated-bulk precedent (`projectHandlers.removeManaged` in `internal/api/projects.go`). The panel becomes the **third caller** of the single shared cleanup path, exactly as the milestone convention intends (handler → reaper → this panel).

Only **two genuinely new pieces of engineering** exist, and both were verified empirically on this machine against git 2.43.0:

1. **Cross-project worktree enumeration** — a new `git worktree list --porcelain -z` parser (no such parser exists in the repo today), run at every project's `repo_path`, then UNION'd with `SELECT id, worktree_path, source, pr_number, status FROM tasks WHERE worktree_path IS NOT NULL` to classify each row as REFERENCED / ORPHAN / STALE-POINTER.
2. **D-01 permission-blocked detection** — the load-bearing gotcha. I reproduced the exact uid-70 `./.db` failure: `git worktree remove` (plain) exits 255 with `warning: could not open directory '.db/': Permission denied` + `error: failed to delete '<path>': Directory not empty`, and **crucially it has already deregistered the worktree from git metadata** so the second `--force` attempt exits 128 with the *misleading* `fatal: '<path>' is not a working tree` while the directory survives as an undeletable shell. This means D-01 detection must catch the permission signal on the FIRST removal attempt (via `errors.Is(err, fs.ErrPermission)` after an `os.RemoveAll` probe, OR by matching git's "Permission denied" / "Directory not empty" stderr) and refuse to proceed to `--force`.

**Primary recommendation:** Add an `internal/worktree` porcelain-list parser + a `Prune`/`ClearPointer` helper; extend `CleanupWorktreeGated` with a `taskID==0` orphan mode (skip the null-columns UPDATE) and a distinct `blocked`/`ErrBlocked` return that carries the offending path; add a new `internal/api/cleanuppanel.go` with three endpoints (`GET /api/worktrees`, `POST /api/worktrees/remove`, `POST /api/worktrees/clean-eligible`) registered in `main.go` alongside `WorktreeRoutes`; add a `WorktreeCleanupSection` to `SettingsPage.tsx` mirroring the DiffTab fetch-on-mount + manual-Refresh model and the `CleanupWorktreeDialog` confirm pattern.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Enumerate every worktree across all repos | API / Backend | Database (project repo paths) | Requires N× `git worktree list` shell-outs + DB join; N git calls must not run in the browser (D-10) |
| Classify REFERENCED / ORPHAN / STALE-POINTER | API / Backend | Database | Set arithmetic (git list △ DB rows) needs both the git enumeration and the `tasks` table — only the backend has both |
| Compute dirty/unpushed/stash flags | API / Backend | — | Each flag is a git shell-out (`status`, `rev-list`, `stash list`); server-computed per GET keeps the client dumb (D-10) |
| Base-ref resolution for "unpushed" | API / Backend | — | Reuses `worktree.ResolveBase` / `resolvePRBase` git logic already in the backend (`diffs.go`) |
| Gated force-remove of one worktree | API / Backend | — | `CleanupWorktreeGated` owns the sessions/dirty gate + stop + kill-tmux + `wt.Remove`; the browser never removes files |
| D-01 permission-blocked detection | API / Backend | — | An `os.RemoveAll`/git `EACCES` signal is only observable server-side |
| Bulk "clean eligible" (compute + execute) | API / Backend | Database | Eligibility predicate reads task status + PR state from the DB; execution loops `CleanupWorktreeGated` |
| Panel UI (list, per-row remove, bulk preview) | Frontend Server (SPA) | Browser | React section rendered from one annotated GET; TanStack Query cache + mutations |
| Confirm dialogs / copyable sudo hint | Browser | — | Pure client interaction; `navigator.clipboard` for the `sudo rm -rf` copy |

## Standard Stack

No new external dependencies. The phase is built entirely on the project's already-pinned stack (see `CLAUDE.md` for the full version matrix). The relevant already-present pieces:

### Core (already in the repo — reuse, do not add)
| Library / Primitive | Where | Purpose | Why Standard |
|---------|-------|---------|--------------|
| `os/exec` + system `git` | `internal/worktree/worktree.go` `gitRun` | Shell out `git worktree list --porcelain -z`, `worktree remove`, `worktree prune` | Project invariant: git ops shell out with `--porcelain`, never parse human output (`CLAUDE.md`) [CITED: CLAUDE.md] |
| `net/http` stdlib ServeMux | `internal/api/routes.go` | Register `GET /api/worktrees` etc. with method+path matching | Go 1.22+ ServeMux; the app's whole routing layer [VERIFIED: go.mod `go 1.26`] |
| `modernc.org/sqlite` via `database/sql` | throughout `internal/api` | Query `tasks`/`projects` for the DB side of the union | Existing driver [CITED: CLAUDE.md] |
| `io/fs` + `syscall` (stdlib) | new — D-01 detection | `errors.Is(err, fs.ErrPermission)` / `*fs.PathError` to catch EACCES and extract the offending path | Stdlib; verified below [VERIFIED: local test + pkg.go.dev/io/fs] |
| `@tanstack/react-query` | `web/src/api/*` | Panel query + remove/clean mutations | The app's server-state layer [CITED: CLAUDE.md] |
| shadcn `alert-dialog` | `web/src/components/ui/alert-dialog.tsx` | Force-remove + bulk-preview confirmations | Already used by `CleanupWorktreeDialog.tsx` [VERIFIED: codebase] |
| `lucide-react` icons | `web/src/components/**` | `RefreshCw` (spin on refetch), `TriangleAlert` (blocked/dirty), `Copy` | Already used by `DiffTab.tsx`/`CleanupWorktreeDialog.tsx` [VERIFIED: codebase] |

### Supporting (new small in-repo helpers — not packages)
| Helper | Location | Purpose |
|--------|----------|---------|
| `List(ctx, repo) ([]Entry, error)` | new in `internal/worktree/worktree.go` | Parse `git worktree list --porcelain -z` into `{Path, HEAD, Branch, Bare, Detached, Locked, Prunable}` records |
| `Prune(ctx, repo) error` (or reuse existing prune-in-Remove) | `internal/worktree` | `git worktree prune` to clear a STALE-POINTER git registration whose dir is gone |
| orphan mode in `CleanupWorktreeGated` | `internal/api/cleanup.go` | `taskID==0` ⇒ skip the null-columns UPDATE; return a distinct `blocked` outcome on EACCES |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| New `List` parser in `internal/worktree` | `git for-each-ref` / go-git worktree API | go-git can't enumerate linked worktrees reliably (`CLAUDE.md` "What NOT to Use"); `--porcelain` is the documented stable machine format. Stay with the shell-out convention. |
| Extend `CleanupWorktreeGated` for orphans | A separate `removeOrphan` function | A separate function would duplicate the stop-sessions/kill-tmux/`wt.Remove` sequence and re-introduce the two-code-path bug the D-04 milestone deliberately collapsed. Extend the one path. |
| One annotated GET | GET list + per-row detail fetch | The list GET already pays N git calls; a per-row detail fetch would double them and complicate refresh. One GET is D-10's explicit intent. |

**Installation:** None — no `npm install` / `go get`. This section intentionally lists no external packages, so the Package Legitimacy Audit below is a no-op.

**Version verification:** `go.mod` declares `go 1.26` [VERIFIED: go.mod line 3]. System git is `2.43.0` [VERIFIED: `git --version`], the same version every worktree behavior in `internal/worktree` was validated against (per the package doc comment).

## Package Legitimacy Audit

> No external packages are installed by this phase. All code builds on the stdlib and already-present, already-audited project dependencies.

| Package | Registry | Age | Downloads | Source Repo | slopcheck | Disposition |
|---------|----------|-----|-----------|-------------|-----------|-------------|
| *(none)* | — | — | — | — | — | No new packages |

**Packages removed due to slopcheck [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```
                     ┌─────────────────────────────────────────────┐
  Settings page      │  GET /api/worktrees   (fetch on section open │
  section opens ────►│                         + manual Refresh)     │
  (React)            └───────────────┬─────────────────────────────┘
                                     │
                     ┌───────────────▼───────────────────────────────┐
                     │  cleanuppanel.go : list()                       │
                     │                                                 │
                     │  1. SELECT id, name, repo_path, managed         │
                     │       FROM projects            ── DB            │
                     │  2. for each project repo_path:                 │
                     │       git worktree list --porcelain -z  ── git  │
                     │       └► worktree.List() parser                 │
                     │  3. SELECT id, worktree_path, source, pr_number,│
                     │       status, pr_base_ref, branch               │
                     │       FROM tasks WHERE worktree_path NOT NULL   │
                     │  4. UNION + classify:                           │
                     │       git-listed ∩ DB task  → REFERENCED        │
                     │       git-listed \ DB task  → ORPHAN            │
                     │       DB task \ git-listed  → STALE POINTER     │
                     │  5. per REFERENCED/ORPHAN worktree that exists: │
                     │       DirtyCount / UnpushedCount / StashCount   │
                     │       + cleanupSessionCount     ── git + mgr    │
                     │       (base ref resolved per row: PR vs task)   │
                     └───────────────┬────────────────────────────────┘
                                     │  annotated []WorktreeRow JSON
                                     ▼
                     ┌───────────────────────────────────────────────┐
                     │  Panel renders rows grouped by project.         │
                     │  Each row: association, class, flags, blocked?  │
                     └───┬───────────────────────────────┬───────────┘
     per-item force      │                               │  bulk "clean eligible"
     (confirm dialog)    ▼                               ▼  (preview → confirm)
   POST /api/worktrees/remove              POST /api/worktrees/clean-eligible
     {repo, path, task_id?, force,           (server recomputes eligible set,
      stop_sessions}                          NEVER forces, returns per-item result)
             │                                           │
             ▼                                           ▼
   ┌─────────────────────────────────────────────────────────────────┐
   │  CleanupWorktreeGated(...)   ← the ONE shared path (3rd caller)   │
   │    gate sessions ▸ gate dirty ▸ StopAllForTask ▸ kill tmux ▸      │
   │    wt.Remove ▸ (taskID>0) null-columns  ▸ [NEW] EACCES→blocked    │
   └─────────────────────────────────────────────────────────────────┘
```

### Recommended Project Structure

```
internal/
├── worktree/
│   └── worktree.go        # + List(ctx, repo) parser; maybe + Prune()
├── api/
│   ├── cleanup.go         # extend CleanupWorktreeGated: orphan mode + blocked outcome
│   ├── cleanuppanel.go    # NEW: list/remove/clean-eligible handlers + WorktreeCleanupRoutes
│   └── cleanuppanel_test.go
web/src/
├── api/
│   └── worktreeCleanup.ts # NEW: useWorktreeList, useRemoveWorktree, useCleanEligible
├── components/settings/
│   ├── WorktreeCleanupSection.tsx      # NEW: the <section> for SettingsPage
│   ├── WorktreeRow.tsx                 # NEW: one row (flags, blocked, remove btn)
│   ├── ForceRemoveDialog.tsx           # NEW: mirrors CleanupWorktreeDialog
│   └── CleanEligibleDialog.tsx         # NEW: preview→confirm bulk
└── pages/
    └── SettingsPage.tsx   # + <WorktreeCleanupSection/>
```

### Pattern 1: `git worktree list --porcelain -z` parser (NEW)

**What:** Enumerate every worktree registered in a repo, robustly.
**When to use:** Once per project `repo_path` in the list handler.
**Verified behavior (git 2.43.0, this machine):** Records are separated by a blank line; attributes within a record are one-per-line. The `-z` variant NUL-separates attributes (`\x00`) and double-NUL-separates records (`\x00\x00`), which makes paths containing spaces/newlines safe to parse — **use `-z`**. A worktree whose directory was manually deleted appears with a `prunable gitdir file points to non-existent location` attribute (and loses its `branch`, showing `detached`) — this is git's own reverse-orphan signal.

Empirical `-z` bytes captured:
```
"worktree /repo\x00HEAD <sha>\x00branch refs/heads/main\x00\x00worktree /wt\x00HEAD <sha>\x00detached\x00prunable gitdir file points to non-existent location\x00\x00"
```

```go
// Source: verified against git 2.43.0; format documented stable per
// https://git-scm.com/docs/git-worktree ("porcelain ... stable ... regardless of config")
type Entry struct {
    Path     string
    HEAD     string
    Branch   string // "" if detached/bare
    Bare     bool
    Detached bool
    Locked   bool
    LockedReason   string
    Prunable bool
    PrunableReason string
}

func (s *Service) List(ctx context.Context, repo string) ([]Entry, error) {
    out, err := gitRun(ctx, repo, "worktree", "list", "--porcelain", "-z")
    if err != nil {
        return nil, err
    }
    var entries []Entry
    // Records are "\x00\x00"-separated; attributes within a record "\x00".
    for _, rec := range strings.Split(strings.TrimRight(out, "\x00"), "\x00\x00") {
        if rec == "" {
            continue
        }
        var e Entry
        for _, line := range strings.Split(rec, "\x00") {
            switch {
            case strings.HasPrefix(line, "worktree "):
                e.Path = strings.TrimPrefix(line, "worktree ")
            case strings.HasPrefix(line, "HEAD "):
                e.HEAD = strings.TrimPrefix(line, "HEAD ")
            case strings.HasPrefix(line, "branch "):
                e.Branch = strings.TrimPrefix(line, "branch ")
            case line == "bare":
                e.Bare = true
            case line == "detached":
                e.Detached = true
            case line == "locked" || strings.HasPrefix(line, "locked "):
                e.Locked = true
                e.LockedReason = strings.TrimSpace(strings.TrimPrefix(line, "locked"))
            case line == "prunable" || strings.HasPrefix(line, "prunable "):
                e.Prunable = true
                e.PrunableReason = strings.TrimSpace(strings.TrimPrefix(line, "prunable"))
            }
        }
        entries = append(entries, e)
    }
    return entries, nil
}
```

**Note:** the FIRST entry of every repo's list is the **main worktree** (the repo checkout itself). The panel must **exclude the main worktree from removable rows** — it is not a task worktree and `git worktree remove` refuses the main worktree anyway (see `projects.go removeManaged`, which uses `os.RemoveAll` for the clone root precisely because git refuses the main worktree). Filter it by matching `Entry.Path == project.repo_path` (compare `filepath.Clean` on both).

### Pattern 2: Union + classification (NEW, pure Go set arithmetic)

**What:** Join the git enumeration with the DB task rows to classify each worktree.
**When to use:** In the list handler after gathering both sides.

```go
// gitPaths: filepath.Clean(Entry.Path) for every NON-main linked worktree,
//           across every project's repo (map path -> {Entry, projectID}).
// dbRows:   SELECT id, worktree_path, source, pr_number, status, pr_base_ref, branch
//             FROM tasks WHERE worktree_path IS NOT NULL   (map cleanPath -> taskRow)
//
// REFERENCED   = path present in BOTH git list and a DB task row.
// ORPHAN       = path in git list, NO DB task row      (WTREE-04).
// STALE POINTER= DB task row whose path is NOT in the git list
//                (dir manually deleted / never registered). D-07.
```

DB columns available for the DB side (verified in schema + `tasks.go`):
- `tasks`: `id, project_id, title, status, branch, worktree_path, worktree_error, source, pr_number, pr_base_ref` (`taskColumns` @ `tasks.go:61`; migrations `00002_worktrees.sql`, `00007_github_foundations.sql`).
- `status ∈ ('todo','in_progress','in_review','done')` (`00001_init.sql:16`). **"task is Done"** = `status = 'done'` for the D-04 predicate.
- `source ∈ ('manual','github_pr')`; a PR-review task has `source='github_pr'`, `pr_number`, `pr_base_ref` (`00007_github_foundations.sql:11-13`).
- `projects`: `id, name, repo_path, managed` (join on `tasks.project_id`) — `repo_path` is the git-list root for every project (folder AND managed clone). `managed` distinguishes them but the scan runs at `repo_path` regardless (D-06). Both branches were confirmed to have `origin/HEAD` set for managed clones (`worktree.go:DefaultBranch` doc).

**"PR merged/closed"** for the D-04 predicate is NOT stored in the DB — it requires a live `gh` call (`PRStateGetter.PRState` → `"OPEN"|"CLOSED"|"MERGED"`, see `reaper.go:65`). See §Open Questions Q1 for the recommended handling (mirror the reaper: warn-and-skip on a gh hiccup; a PR row whose state can't be read is simply not eligible, never forced).

### Pattern 3: Per-row base ref resolution for "unpushed" (D-08)

**What:** Resolve the commit-ish the "unpushed" count is measured against, identically to how existing cleanup does it — and it differs by worktree kind.
**Verified from the two existing callers:**

- **Managed-delete path** (`projects.go:553`, `deleteManaged`): resolves ONCE via `wt.DefaultBranch(clone)` → `unpushedBase = "origin/" + defBranch`, then `UnpushedCount(wt.path, unpushedBase)` with **NO fetch** (network-free, conservative). A base-resolution failure is treated as a blocker, not a skip.
- **Reaper PR path** (`reaper.go:294`): for a `github_pr` task it `FetchRef(t.wtPath, refs/pull/<n>/head)` then `UnpushedCount(t.wtPath, "FETCH_HEAD")`. The fetch MUST run in the worktree dir (FETCH_HEAD is per-worktree — see the load-bearing comment at `reaper.go:286-293`).
- **Diff base** (`diffs.go:78-103`): `github_pr` + non-empty `pr_base_ref` ⇒ fetch `pr_base_ref`, then prefer local `refs/heads/<base>` else `origin/<base>` (`resolvePRBase`, `diffs.go:157`); otherwise `wt.ResolveBase(repo)` (the 4-step default-branch chain, `worktree.go:125`).

**Recommendation for the panel (per row):**
- **Referenced task worktree, `source='manual'`:** resolve base = `wt.ResolveBase(ctx, project.repo_path)` (same as the diff/creation base). Network-free.
- **Referenced PR worktree, `source='github_pr'` with `pr_base_ref`:** resolve base the diff way — `resolvePRBase` against `origin/<pr_base_ref>` (best-effort `FetchRef` first, or skip the fetch for a network-free conservative read as `deleteManaged` does). **The panel is a read-heavy annotate step; prefer the network-free `origin/<default>` / `origin/<base>` reads (like `deleteManaged`) to keep the GET fast and offline-safe.** A resolution failure ⇒ show "unpushed: unknown" and treat as a gate for D-04 eligibility (conservative), never as removable.
- **Orphan worktree (no DB task):** no branch base known; the "unpushed" flag is best-effort. Recommend resolving `origin/HEAD` of the containing repo (`ResolveBase`) if a branch is checked out, else mark "unpushed: n/a". Orphans are still eligible for D-04 removal regardless (they have no task work to protect) — but D-01/dirty still apply; do NOT force in bulk.

Consistency requirement: whatever base logic a REFERENCED row uses in the annotate GET, the **same** base must be used if that worktree is later force-removed — but note force (`force=true`) bypasses the unpushed consideration entirely (the unpushed count is advisory in the confirm dialog, not a hard gate inside `CleanupWorktreeGated`, which only gates on sessions + dirty). Unpushed/stash are advisory display flags + D-04 eligibility inputs; they are NOT gates inside the shared helper (see `cleanup.go:30-35` GATE SCOPE note — the two extra conservative gates live in the reaper, not the helper).

### Pattern 4: D-01 permission-blocked outcome (NEW, load-bearing)

**What goes wrong (reproduced on this machine):** A worktree containing a foreign-uid, mode-700 subdir (the `sched` `./.db` case) can't be deleted by the unprivileged app.

**Verified failure sequence:**
1. `git worktree remove <path>` (plain) → exit **255**, stderr:
   `warning: could not open directory '.db/': Permission denied`
   `error: failed to delete '<path>': Directory not empty`
   — and git **has already removed the worktree's registration** (its `.git/worktrees/<name>` admin dir), so the tree is now an orphaned directory.
2. A follow-up `git worktree remove --force <path>` → exit **128**, stderr:
   `fatal: '<path>' is not a working tree` — because the registration is already gone. The directory **still exists** on disk (partial failure / undeletable shell).

**Implication for `wt.Remove`:** its current submodule fallback retries `--force` on any error containing "submodules"; a permission failure does NOT contain "submodules", so it returns the plain-remove error verbatim — which is the RIGHT error to inspect (the "Permission denied" / "Directory not empty" text), NOT the misleading `--force` one. **Do not add a blanket `--force` retry** for the permission case — that would produce the confusing "is not a working tree" message and still leave the shell.

**Recommended detection (two viable, combine for robustness):**

Option A (stderr-string match on git's own message — cheapest, no extra syscalls):
```go
func isPermissionBlocked(err error) bool {
    if err == nil { return false }
    m := err.Error()
    return strings.Contains(m, "Permission denied") ||
        (strings.Contains(m, "Directory not empty") && strings.Contains(m, "failed to delete"))
}
```

Option B (a pre-remove `os.RemoveAll`-free probe using EACCES — precise offending path):
```go
// Verified on this machine: os.RemoveAll on a dir containing a mode-000 subtree
// returns an error where errors.Is(err, fs.ErrPermission) == true AND
// errors.As(err, &*fs.PathError{}) exposes .Path = the exact blocked directory.
var pe *fs.PathError
if errors.As(err, &pe) && errors.Is(err, fs.ErrPermission) {
    // pe.Path is the offending path for the "sudo rm -rf <pe.Path>" hint;
    // errors.Is(err, syscall.EACCES) is also true.
}
```
`errors.Is(err, fs.ErrPermission)` and `errors.Is(err, syscall.EACCES)` both returned `true`; `*fs.PathError.Path` gave the exact blocked dir. [VERIFIED: local test on this machine + pkg.go.dev/io/fs]

**Recommended model in `CleanupWorktreeGated`:** return a distinct signal rather than a raw `err`. Two clean shapes (planner picks):
- A sentinel + typed error: `var ErrRemoveBlocked = errors.New("blocked")` wrapped as `type BlockedError struct{ Path string }`; the handler maps a `BlockedError` to a **200 with `{ "outcome": "blocked", "path": ... }`** (NOT a 500) so the row renders "Blocked — needs manual removal" with the copyable hint.
- Or widen the return to `(outcome string, path string, err error)` where `outcome ∈ {"removed","gated","blocked"}`.

The offending path for the hint: from Option B, `pe.Path`; from Option A, parse git's `could not open directory '<dir>'` (the `.db/` in the message). Prefer Option B's `pe.Path` — it's exact and needs no string parsing. **The app never runs `sudo`/`pkexec` itself (D-01)** — it only renders the hint string for the user to copy.

### Pattern 5: "Clean eligible" predicate + two-pass bulk (D-04/D-05)

**What:** Server computes the provably-safe set, previews it, then removes it — NEVER forcing.
**Precedent:** `projectHandlers.removeManaged` (`projects.go:682`) is the all-or-nothing multi-worktree gated bulk. The panel's bulk differs: it is **best-effort per item** (skip a tripped/blocked one, remove the rest) rather than all-or-nothing.

Eligibility per worktree (ALL must hold; never force):
```
eligible(wt) :=
  (orphan(wt)  OR  (referenced(wt) AND (task.status == 'done' OR prMergedOrClosed(wt))))
  AND cleanupSessionCount == 0
  AND DirtyCount == 0
  AND UnpushedCount(base) == 0
  AND StashCount == 0
  AND NOT blocked(wt)          // a known D-01 shell is never bulk-touched
```
- `task.status == 'done'` ⇐ `SELECT status FROM tasks` (`done`).
- `prMergedOrClosed` ⇐ `PRState(repo, pr_number) ∈ {"MERGED","CLOSED"}` via the same `github.Service` the reaper/PR routes use (§Open Q1).
- Orphans skip the task predicate entirely (no task to be Done) but STILL must pass the four gates + not-blocked.

**Two-pass shape (mirrors `deleteManaged`'s gate-then-remove, but non-atomic):**
1. **Preview** (`POST /api/worktrees/clean-eligible?dry_run=1` or a `GET` recompute): return the exact list the action would remove. D-05 requires the confirm to list these before running.
2. **Execute**: for each eligible item call `CleanupWorktreeGated(..., stopSessions=false, force=false)`. Because every gate already passed, none should trip; a race that re-trips a gate ⇒ skip that item (like the reaper), continue the rest. Return a per-item result array so the panel can refresh and show what remained.

### Anti-Patterns to Avoid
- **Blanket `--force` retry on any remove error.** Verified to produce the misleading "is not a working tree" message on the permission case and leave an undeletable shell. Only the existing "submodules" retry is safe (`worktree.go:365`).
- **Removing the main worktree via `git worktree remove`.** git refuses it; `removeManaged` uses `os.RemoveAll` for the clone root (`projects.go:708`). The panel simply excludes it from removable rows.
- **Parsing human `git worktree list` output.** Project invariant: `--porcelain` only (`CLAUDE.md`). Use `-z` for path safety.
- **Bulk clean that forces.** D-04/D-05 forbid it. Only the per-item action forces, and only behind the confirm.
- **Deleting branches.** D-02/D-34: worktree only, ever. `CleanupWorktreeGated` already never touches branches.
- **Polling the panel.** D-10: fetch on open + manual Refresh, no `refetchInterval` (mirror `useTaskDiff`).
- **Trusting the client's snapshot at remove time.** Re-check gates server-side at remove time (Pitfall 8 across the codebase — `worktrees.go:211`, `diffs.go`).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Gated worktree removal (sessions gate, dirty gate, stop-before-remove, kill-tmux-before-remove, null-columns) | A fresh removal routine in the panel | `CleanupWorktreeGated` (`cleanup.go:36`) as the **third caller** | The ordering (stop through SIGTERM grace, kill detached tmux, THEN remove) is load-bearing and already correct; a second copy re-opens the orphan-shell hole (Pitfall 4/5). |
| Counting live sessions incl. detached tmux survivors | Re-derive from the session Manager | `cleanupSessionCount` + `liveTmuxNames` (already on `worktreeHandlers` AND `projectHandlers` AND the reaper) | The detached-tmux fold (D-92) is subtle; three copies already agree — the panel's handler struct should carry the same helpers (they're ~30 lines, replicated deliberately per the package-boundary note at `projects.go:741`). |
| Base-ref resolution for a PR vs task worktree | New base logic | `wt.ResolveBase` / `resolvePRBase` / `wt.DefaultBranch` | Four legs of default-branch detection + PR-base handling are already verified against git edge cases (`worktree.go:125`, `diffs.go:157`). |
| Multi-worktree gated bulk over a project | A bespoke loop | The `deleteManaged`/`removeManaged` gate-then-remove structure (`projects.go:546/682`) | It's the proven precedent for iterating worktrees through the shared helper and mapping gate reasons to a structured response. |
| Detecting a stale/orphaned registration whose dir is gone | Manual stat + heuristics | git's own `prunable` attribute in the porcelain list + `git worktree prune` | git already reports `prunable gitdir file points to non-existent location` and `prune` deregisters it (verified). |

**Key insight:** The single most valuable thing this phase can do is *resist the urge to reimplement removal*. Everything destructive already flows through one audited path; the panel's job is enumeration, classification, a UI, and two small extensions to that one path (orphan mode + blocked outcome).

## Runtime State Inventory

> This phase is a feature addition, not a rename/migration, so most categories are N/A. But because it operates on live git/tmux/session runtime state, the relevant "state that isn't in the repo" is inventoried here.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | `tasks.worktree_path`/`branch`/`worktree_error` columns (nulled on manual remove, per `CleanupWorktreeGated`); `tmux_sessions` rows keyed by `task_id` | Panel remove nulls task columns for REFERENCED (taskID>0); for ORPHAN there is no row to null (helper extension). STALE-POINTER action nulls the task columns and keeps the row (D-07). |
| Live service config | git worktree registrations in each repo's `.git/worktrees/*` (NOT in the DB, NOT in git-tracked files) — this is exactly the disk↔DB drift the panel reconciles | Enumerate via `git worktree list --porcelain -z`; `git worktree prune` clears stale registrations. |
| OS-registered state | Live tmux sessions on the dedicated `-L kamacu` socket (detached survivors) | `CleanupWorktreeGated` already kills the task's live tmux before `wt.Remove` (`cleanup.go:75`). Orphans have no `task_id`, so no tmux rows to probe — verify: an orphan worktree has no associated `tmux_sessions` rows (they're keyed by `task_id`). None to kill. |
| Secrets/env vars | None — panel touches no secrets/env. | None. |
| Build artifacts | Foreign-uid data dirs inside worktrees (the `./.db` Postgres dir, uid 70) — the D-01 blocker. Not a build artifact per se but the concrete undeletable-state case. | Detect + surface (D-01); never auto-remove with privilege. |

**Prior-phase note (verified):** Phase 21's migration (`internal/migrate/paths.go:185-214`) **deliberately left stale/unregistered worktree dirs for "Phase-23 cleanup's job"** — a per-path `git worktree repair` skips a straggler and logs a warning. Those exact stragglers (the real `sched` repo's ~4-6 stale dirs) are STALE-POINTER / ORPHAN rows this panel is designed to surface and clear. This is direct upstream confirmation of the union-scan's value.

## Common Pitfalls

### Pitfall 1: Permission-blocked partial removal (D-01)
**What goes wrong:** The first `git worktree remove` deregisters the worktree but fails to delete the dir; a naive `--force` retry then reports the misleading "is not a working tree" and the dir survives as an undeletable shell.
**Why it happens:** A foreign-uid mode-700 subdir (container `./.db`, uid 70) blocks the unprivileged `rm`; git's deregistration happens before the filesystem delete completes.
**How to avoid:** Detect `errors.Is(err, fs.ErrPermission)` / "Permission denied" on the FIRST attempt; do NOT proceed to `--force`; return the `blocked` outcome with `pe.Path`; render the copyable `sudo rm -rf <path>` hint. Keep the git registration + DB row intact (D-01).
**Warning signs:** exit 255 with "Directory not empty"; a worktree that reappears as `prunable` after a "successful"-looking remove.

### Pitfall 2: Removing the main worktree
**What goes wrong:** `git worktree remove <repo>` on the repo checkout itself fails.
**How to avoid:** The first porcelain entry per repo is the main worktree; exclude any entry whose `filepath.Clean(Path)` equals the project's `filepath.Clean(repo_path)` from removable rows.
**Warning signs:** git error "is a main working tree".

### Pitfall 3: FETCH_HEAD is per-worktree (unpushed base)
**What goes wrong:** Fetching a PR base in the main repo dir then `rev-list FETCH_HEAD..HEAD` in the linked worktree fails because FETCH_HEAD is per-worktree (documented at `reaper.go:286-293`).
**How to avoid:** For the panel's read-only annotate, prefer network-free `origin/<base>` reads (like `deleteManaged`) instead of FETCH_HEAD, so no per-worktree fetch is needed and the GET stays offline-safe.
**Warning signs:** every PR row showing "unpushed: unknown" / conservative skips.

### Pitfall 4: Stale client snapshot at remove time
**What goes wrong:** A worktree that was clean/idle when the panel loaded gains a session or dirt before the user clicks remove.
**How to avoid:** Re-check gates server-side inside `CleanupWorktreeGated` at remove time (it already does). The panel refetches the list after any mutation (invalidate the query).
**Warning signs:** 409 "sessions running"/"uncommitted changes" surfacing at remove time — expected; surface it and refresh.

### Pitfall 5: N git calls scaling
**What goes wrong:** Many projects × many worktrees × 3 flag calls each = a slow GET.
**Why it's acceptable:** Single-user localhost; the existing diff/cleanup paths already pay per-request git costs. D-10 explicitly chose server-side per-GET computation with manual refresh (no polling) precisely so this cost is paid on demand only.
**How to avoid problems:** Don't poll; keep the manual-Refresh model; consider a bounded `context.WithTimeout` around the whole enumeration (the codebase uses 60s in `CleanupWorktreeGated`).

## Code Examples

### Registering the new routes (mirror existing style)
```go
// Source: internal/api/routes.go / cmd/kamacu/main.go wiring pattern (verified)
// In a new internal/api/cleanuppanel.go:
func WorktreeCleanupRoutes(mux *http.ServeMux, db *sql.DB, wt *worktree.Service,
    mgr *session.Manager, tmuxClient tmux.Client, pr PRStateGetter) {
    h := &cleanupPanelHandlers{db: db, wt: wt, mgr: mgr, tmuxClient: tmuxClient, pr: pr}
    mux.HandleFunc("GET /api/worktrees", h.list)
    mux.HandleFunc("POST /api/worktrees/remove", h.remove)
    mux.HandleFunc("POST /api/worktrees/clean-eligible", h.cleanEligible)
}
// Wire in cmd/kamacu/main.go next to api.WorktreeRoutes(...) — needs ghSvc for PRState
// (the same *github.Service passed to reaper.NewWithPR and api.PullRequestRoutes).
```

### Calling the shared helper for a REFERENCED force-remove
```go
// Source: internal/api/worktrees.go:250 (the existing 2nd caller), verified
count := h.cleanupSessionCount(ctx, taskID)
live := h.liveTmuxNames(ctx, taskID)
removed, reason, err := CleanupWorktreeGated(
    ctx, h.db, h.wt, h.mgr, h.tmuxClient, live,
    taskID, repo, path, count, true /*stopSessions*/, true /*force*/)
// D-03: force-remove passes stopSessions=true AND force=true.
```

### Frontend: fetch-on-open + manual refresh (mirror DiffTab / useTaskDiff)
```ts
// Source: web/src/api/diffs.ts:52 pattern (verified) — no refetchInterval (no poll)
export function useWorktreeList() {
  return useQuery<WorktreeListResponse, ApiError>({
    queryKey: ["worktrees"],
    queryFn: () => get<WorktreeListResponse>("/api/worktrees"),
    // section mounts on open; staleTime 0 default = fetch on mount; refetch() = manual button
  });
}
```

### Frontend: copy the sudo hint (blocked row)
```ts
// D-01: app renders the hint; the USER runs sudo. Never spawn a privileged process.
await navigator.clipboard.writeText(`sudo rm -rf ${blockedPath}`);
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Two cleanup code paths (handler + reaper) | One shared `CleanupWorktreeGated`, N callers | Phase 13 (D-04 milestone) | The panel MUST be the 3rd caller, not a 4th path. |
| Reaper leaves general orphans to accumulate | This panel surfaces + clears them (WTREE-04) | Phase 23 (now) | The reaper only auto-removes merged/closed PR worktrees; manual/general orphans were intentionally never auto-removed (D-87). |
| Migration left stale worktree dirs behind | Panel reconciles disk↔DB drift | Phase 21 → 23 | `paths.go:213` explicitly names "Phase-23 cleanup's job". |

**Deprecated/outdated:** none relevant — no library churn touches this phase.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The recommended endpoint paths (`GET /api/worktrees`, `POST /api/worktrees/remove`, `POST /api/worktrees/clean-eligible`) — CONTEXT.md leaves naming to discretion. | §API Shape | Low; naming is cosmetic and easily changed in planning. |
| A2 | For the panel's annotate GET, network-free `origin/<base>` unpushed reads (like `deleteManaged`) are preferable to per-worktree `FETCH_HEAD` (like the reaper), to keep the GET fast/offline. Both are valid existing patterns; the choice is a tradeoff (freshness vs speed/offline-safety) the planner/user may revisit. | §Pattern 3 | Medium; if fresh PR-head unpushed detection is required, the reaper's FETCH_HEAD-in-worktree approach must be used instead — slower, needs network. |
| A3 | D-04's "PR merged/closed" is resolved via a live `gh` `PRState` call at list/clean time (no stored PR state exists in the schema). A gh hiccup ⇒ that PR row is simply not eligible (warn-and-skip, like the reaper) rather than blocking the whole action. | §Pattern 2 / §Pattern 5 / §Open Q1 | Medium; if the user wants the panel to work fully offline, PR-merged eligibility can't be computed and only Done-status + orphan rows would be bulk-eligible. Needs confirmation. |
| A4 | The `blocked` outcome should be surfaced as HTTP 200 with an `{outcome:"blocked"}` body (so a row renders inline) rather than a 4xx/5xx. CONTEXT.md says "modeled distinctly from a generic error" but not the exact HTTP shape. | §Pattern 4 | Low; a 200-with-outcome vs a 422 is an easy planning decision; both let the client distinguish blocked from a real error. |
| A5 | Orphan worktrees have no `tmux_sessions` rows (those are keyed by `task_id`), so orphan removal needs no tmux-kill probing. | §Runtime State Inventory | Low; if a truly orphaned worktree somehow had a stale tmux session, the startup `sweepOrphanTmux` already reaps kamacu-* sessions with no known name. |
| A6 | Bulk clean is best-effort per-item (skip tripped/blocked, remove the rest), unlike `removeManaged`'s all-or-nothing. Inferred from D-04 ("any … tripped … is skipped"). | §Pattern 5 | Low; D-04 wording strongly implies per-item skipping, not all-or-nothing. |

## Open Questions

1. **How does the panel obtain PR merged/closed state (D-04 eligibility + PR association display)?**
   - What we know: the reaper uses `github.Service.PRState(ctx, repo, n) → "OPEN"|"CLOSED"|"MERGED"` (`reaper.go:65`); the same `*github.Service` is already constructed once in `main.go` and shared with the reaper and PR routes.
   - What's unclear: whether the panel should call `gh` per PR row on every list GET (adds N gh calls + latency + a network dependency to the GET), or only at clean-eligible time, or cache it. GitHub integration can also be toggled off entirely (`settings.github_integration`), in which case PR state is unavailable.
   - Recommendation: pass the shared `ghSvc` into `WorktreeCleanupRoutes`; call `PRState` only for `source='github_pr'` rows; warn-and-skip on any gh error (mirror the reaper's degrade-don't-break). When GitHub integration is off or gh is unavailable, PR-merged eligibility is simply unavailable (Done-status + orphan rows remain bulk-eligible). Confirm with the user whether per-GET PR-state calls are acceptable latency, or whether PR eligibility should be computed only in the clean-eligible preview.

2. **Exact shape of `CleanupWorktreeGated`'s extension.** Two clean options (typed `BlockedError` + sentinel, or widening to `(outcome, path, err)`). Both preserve the byte-equivalence guarantee for the existing two callers if the orphan/blocked branches are additive. The planner should pick one and keep the existing DELETE tests green (the D-04 regression guard, `cleanup.go:34`). Note: the existing callers pass `taskID>0` and never hit EACCES in their tests, so adding a `taskID==0` branch + an EACCES branch is additive.

3. **Should a STALE-POINTER row also run `git worktree prune`?** A DB task whose dir is gone may still have a `prunable` git registration in the repo. Clearing the pointer (null task columns) handles the DB side; `git worktree prune` handles the git side. Recommendation: the STALE-POINTER "clear" action should do BOTH (null columns + `git worktree prune` at the repo) so a subsequent scan is clean. Confirm the exact affordance copy in planning.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| system `git` | worktree enumeration + removal | ✓ | 2.43.0 | none — the app's core premise (guaranteed present per `CLAUDE.md`) |
| `gh` CLI | D-04 PR merged/closed eligibility | ✓ (assumed on this host; app guards absence) | — | GitHub integration off / gh absent ⇒ PR eligibility unavailable; Done-status + orphan rows still work (§Open Q1) |
| tmux (`-L kamacu` socket) | kill detached survivors during remove | ✓ (already wired) | — | inconclusive probe is never read as alive (`liveTmuxNames`) |
| Go toolchain | build | ✓ | go 1.26 (go.mod) | none |

**Missing dependencies with no fallback:** none identified on this host.
**Missing dependencies with fallback:** `gh` — its absence degrades D-04 PR eligibility only (the app already degrades GitHub features gracefully when `gh` is unavailable, per `github.Available()`).

## Security Domain

> `security_enforcement` is absent from `.planning/config.json` (treated as enabled). This phase is a local-only, single-user, unprivileged localhost app (`CLAUDE.md` core constraint), so the threat surface is narrow but real — it executes git and deletes filesystem paths derived from DB/git data.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | Single-user localhost; no auth by design (out-of-scope per `CLAUDE.md`). |
| V3 Session Management | no | No web sessions/cookies. |
| V4 Access Control | no | No multi-user model. |
| V5 Input Validation / Injection | **yes** | **Never `sh -c`** — every git call uses `exec.CommandContext` with an arg array (the `internal/worktree.gitRun` / `internal/api` invariant, e.g. `worktree.go:36`, `projects.go:83`). Worktree paths flow from the DB/git list into `git` args only, never a shell. The `sudo rm -rf <path>` hint is rendered as TEXT for the user to copy — the app NEVER executes it. |
| V6 Cryptography | no | No crypto in this phase (the diff-hash sha256 is unrelated). |

### Known Threat Patterns for {Go worktree removal on localhost}

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Command injection via a task title / worktree path reaching a shell | Tampering / Elevation | Arg-array `exec` only; no `sh -c` anywhere near these paths (established invariant). |
| Privilege escalation via app-run `sudo`/`pkexec` | Elevation | **Rejected by D-01** — the app stays unprivileged; the sudo hint is user-copied text, never executed by the app. |
| Destructive removal of the wrong path (main worktree, or a path outside the managed tree) | Tampering / DoS | Remove only via `wt.Remove` on git-registered linked worktrees; exclude the main worktree; the shared helper's gates (sessions/dirty) prevent silent destruction; force is confirm-gated. |
| Partial-removal shell left after a permission failure | DoS (undeletable state) | D-01 detection stops at the first EACCES, keeps registration + row, surfaces manual instructions — never a silent partial deregister. |

## Sources

### Primary (HIGH confidence)
- Repository code (read this session): `internal/api/cleanup.go`, `internal/worktree/worktree.go`, `internal/api/worktrees.go`, `internal/reaper/reaper.go`, `internal/api/projects.go`, `internal/api/diffs.go`, `internal/diff/diff.go`, `internal/api/routes.go`, `internal/api/settings.go`, `cmd/kamacu/main.go`, `internal/migrate/paths.go`, `internal/store/migrations/00001/00002/00007`, `internal/api/tasks.go`, `web/src/pages/SettingsPage.tsx`, `web/src/components/task/DiffTab.tsx`, `web/src/components/task/CleanupWorktreeDialog.tsx`, `web/src/api/{settings,diffs,worktrees,client}.ts` — the authoritative source for every "extend/reuse" claim.
- Empirical git 2.43.0 behavior verified on this machine (this session): `git worktree list --porcelain -z` byte format; `prunable`/`detached`/`locked` attributes; the two-step permission-blocked removal failure (exit 255 → deregistered → exit 128 "is not a working tree", dir survives).
- Empirical Go stdlib behavior verified on this machine (this session): `os.RemoveAll` on a mode-000 subtree ⇒ `errors.Is(err, fs.ErrPermission)==true`, `errors.Is(err, syscall.EACCES)==true`, `*fs.PathError.Path` = offending dir.
- [git-scm.com/docs/git-worktree](https://git-scm.com/docs/git-worktree) — porcelain format is documented stable across versions/config; attribute lines; `-z` semantics; `locked`/`prunable` reasons. [CITED]
- [pkg.go.dev/io/fs](https://pkg.go.dev/io/fs) — `fs.ErrPermission`, `*fs.PathError{Op,Path,Err}`, `errors.Is` mechanism. [CITED]

### Secondary (MEDIUM confidence)
- `CLAUDE.md` project stack + invariants (git shells out with `--porcelain`; no go-git worktrees; arg-array exec).
- Auto-memory `worktree-removal-blocked-by-container-owned-dirs` — the concrete uid-70 `./.db` case D-01 is designed around.

### Tertiary (LOW confidence)
- None — all load-bearing claims were verified in-repo or empirically.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — no new packages; everything reused is present and read this session.
- Architecture / reuse points: HIGH — every "call X" / "extend Y" cites a file:line read this session.
- `git worktree list --porcelain` parser: HIGH — format verified empirically AND against official docs (stable format).
- D-01 permission-blocked detection: HIGH — reproduced the exact failure and verified the Go error-matching on this machine.
- Base-ref resolution: HIGH — three existing callers traced (`deleteManaged`, reaper, diff handler).
- PR-merged eligibility path (D-04): MEDIUM — mechanism is known (`PRState`) but the per-GET vs per-clean call policy is an open decision (Open Q1).

**Research date:** 2026-07-02
**Valid until:** 2026-08-01 (stable — extends in-repo code + git behavior; the only external facts are the git porcelain format and Go stdlib, both stable).
