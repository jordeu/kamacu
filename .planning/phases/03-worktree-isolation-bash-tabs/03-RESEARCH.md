# Phase 3: Worktree Isolation & Bash Tabs - Research

**Researched:** 2026-06-10
**Domain:** git worktree lifecycle automation (git CLI via os/exec), PTY session task-scoping, React tab strip + confirmation dialog flows
**Confidence:** HIGH — every git behavior below was verified empirically against the installed git 2.43.0 on this machine (not docs, not training data)

## Summary

This phase has three workstreams: (1) a small Go worktree service shelling out to the system git (`internal/worktree` or `internal/git`), (2) an extension of the Phase 2 session manager to accept a cwd and task association, and (3) frontend work mounting bash tabs into the existing `TabDef[]` seam plus the cleanup dialog family. No new Go or npm dependencies are needed — everything builds on stdlib `os/exec`, the existing session manager, and already-installed shadcn components.

The decisive research input is a battery of empirical experiments against git 2.43.0 (the installed version). Several behaviors differ from common assumptions and directly shape the plan: **a failed `worktree add -b` due to path collision leaves the branch created** (rollback/reuse needed on Retry); **untracked-only worktrees refuse plain `remove`** (force is needed for any dirt, not just modifications); **worktrees with initialized submodules refuse plain `remove` even when clean**; **removal succeeds while a live process is cwd'd inside** (so the stop-sessions-first gate of D-32 is UX policy, not a technical necessity — but still mandatory); and **`git worktree remove` on a manually-deleted worktree dir succeeds and self-heals the bookkeeping** in 2.43.

**Primary recommendation:** Build a ~200-line `internal/worktree` package with five operations (ResolveBase, Create, Status, Remove, Prune), each a single `exec.Command("git", "-C", repo, ...)` call with arg arrays, exit-code-based success detection, and stderr captured for UI display. Extend `Manager.Spawn` with an options struct (`Cwd`, `TaskID`) and per-task label counters. Keep sessions memory-only this phase (no sessions table); add three nullable columns to tasks in migration 00002.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Branch & Worktree Naming
- **D-22:** Branch name: `task/<slug>-<id>` (e.g. `task/fix-login-42`). Slug from task title, task id guarantees uniqueness.
- **D-23:** Worktrees live centrally at `~/.kangent/worktrees/<project>/<task-slug-id>/` — outside every repo tree, easy to inspect and purge.

#### Base Branch & Failure Handling
- **D-24:** Task branches are created from the repo's **default branch tip** (resolve via `origin/HEAD` → main/master; fall back to current HEAD if unresolvable). No fetch before branching — local tip is the base.
- **D-25:** Worktree creation failure does NOT block task creation: the task lands on the board with a visible "no worktree" warning state and a Retry action in the task view. Git problems never block idea capture.
- **D-26:** Pre-existing tasks (Phase 1 era) get lazy worktree creation: opening such a task shows the no-worktree state with a "Create worktree" button; bash tabs unlock once it exists. No startup backfill/migration.

#### Bash Tabs UX
- **D-27:** A `+` button at the end of the task view's tab strip spawns a new bash session (cwd = task worktree) and appends a tab labeled `Bash 1`, `Bash 2`, ... Explicit spawn only — consistent with the Start-button philosophy.
- **D-28:** Tabs mirror live server sessions: reopening a task shows a tab per still-running session for that task, reattached via the Phase 2 replay path. Server is the source of truth for which tabs exist.
- **D-29:** Closing a tab (× on the tab) STOPS the session (Phase 2 SIGTERM→5s→SIGKILL teardown) and removes the tab. One concept: tab = session. Exited sessions can also be closed from the exited banner.
- **D-30:** Bash tabs require a worktree — disabled with explanation when the task has none (ties to D-25/D-26).

#### Done & Cleanup Flow
- **D-31:** Moving a task to Done (drag or status change) triggers the cleanup offer dialog. Declining keeps the worktree. Cleanup is ALSO always available as an action in the task view (i.e. "Both" behavior: prompt on Done + menu action).
- **D-32:** If sessions are running when cleanup is requested, the dialog shows "N sessions running" and offers "Stop sessions and clean up" — explicit, single flow. Cleanup never silently kills sessions (satisfies GIT-03's refuse-while-running as an explicit gate).
- **D-33:** If the worktree has uncommitted changes, the dialog warns with the changed-file count and requires type-to-confirm (typing the task slug) before deletion. The branch is ALWAYS kept regardless — only uncommitted work is ever at risk.
- **D-34:** Cleanup = `git worktree remove` + prune bookkeeping; never `branch -D`.

### Claude's Discretion
- Slug generation rules (length cap, charset, dedup)
- Exact dialog copy (follow UI-SPEC copywriting patterns: specific, verb+noun CTAs, destructive confirmations)
- DB schema for worktree/branch fields on tasks and session→task association
- How "tabs mirror sessions" is implemented (extend Phase 2 session manager with task association; list endpoint filter)
- Whether the `/terminal` dev route stays or goes this phase (deferred decision from Phase 2 — keep if it costs nothing)
- Submodule handling (research Phase pitfalls doc flags it; do something sensible or document the limitation)

### Deferred Ideas (OUT OF SCOPE)
- Stale-worktree list with manual purge (v2 — MAINT-01)
- Worktree status indicator on kanban cards (could ride along with Phase 4 status badges)
- Per-project base-branch setting (only if default-branch resolution proves annoying)
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| GIT-01 | Creating a task automatically creates a new git worktree and branch for that task (collision-safe branch naming, e.g. slug + task ID) | Empirical `worktree add -b` semantics (§Empirical Git Findings 2), default-branch resolution chain (§1), slug rules (§Architecture Pattern 4), branch-leak rollback (§Pitfall 1), creation hook in `tasks.go create` (§Pattern 2) |
| GIT-02 | When a task is marked Done, the app offers to delete its worktree, keeping the branch | Verified branch survives `remove`/`remove --force` (§4); Done-transition hook location in `Board.tsx handleDragEnd` after persist (§Pattern 6); cleanup endpoint design (§Pattern 3) |
| GIT-03 | Worktree cleanup warns and requires confirmation if the worktree has uncommitted changes, and refuses while sessions are running in it | `status --porcelain=v2 -uall` count semantics (§3); force-required-for-untracked finding (§4); server-side gates on the DELETE endpoint mirroring D-32/D-33 (§Pattern 3) |
| TERM-04 | User can open additional bash session tabs running in the task's worktree | `Manager.Spawn` extension with Cwd/TaskID (§Pattern 5), tab strip construction from `useSessions(taskId)` (§Pattern 7), controlled `TaskTabs` changes (§Pattern 7) |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Locked stack:** Go 1.26 + stdlib ServeMux, `modernc.org/sqlite`, goose migrations, `creack/pty` v1, `coder/websocket`; React 19 + Vite + TanStack Query 5 + shadcn/Tailwind 4. Worktree management is explicitly locked to **`os/exec` + system git with `--porcelain` parsing; go-git is forbidden** for worktrees.
- **Never `sh -c` with interpolation** — always `exec.Command` with arg arrays (task titles flow near git commands; injection surface).
- **Single binary, local-only**: no new external services; everything in-process.
- **GSD workflow enforcement**: implementation happens via `/gsd:execute-phase`; this document feeds the planner.
- **User's global git rule:** never mention or co-author Claude (or happy-otter) on commits or PRs.

## Standard Stack

### Core

No new dependencies. The phase is built entirely from what is already in `go.mod` / `package.json`:

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `os/exec` (stdlib) | Go 1.26 | All git invocations | Locked by STACK.md; go-git non-viable for linked worktrees |
| system `git` | 2.43.0 (verified installed) | worktree add/remove/list/prune, status, ref resolution | All behaviors below verified against this exact version |
| existing `internal/session` | — | Spawn/Stop/List/Attach engine | Extended, not replaced: SpawnOpts{Cwd, TaskID}, per-task labels |
| `github.com/pressly/goose/v3` | v3.27.1 (in go.mod) | Migration 00002 | Established pattern: `internal/store/migrations/*.sql` embedded |
| shadcn `alert-dialog, input, tooltip, dropdown-menu, tabs` | already installed | Cleanup dialog family, type-to-confirm, tab strip | UI-SPEC: "no new shadcn components needed this phase" |
| lucide-react `plus, x, git-branch, triangle-alert` | already installed | Tab/dialog icons | UI-SPEC icon contract |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| sync worktree creation inside POST task handler | async goroutine + 'creating' status | Sync is far simpler (no status polling, no goroutine lifecycle) and fine for typical repos; `worktree add` materializes a full checkout, so huge repos (100k+ files) could make task creation take seconds. Recommend **sync** for v1 with a context timeout (~30s); on timeout the task still lands (D-25) with the error recorded. |
| `GET /api/sessions?task_id=N` filter | `GET /api/tasks/{id}/sessions` route | Query param keeps one list endpoint and the existing `useSessions` hook shape; the `/terminal` dev route keeps using the unfiltered list. Recommend **query param**. |
| in-memory session→task association | sessions DB table now | Phase 2 sessions are memory-only by design; persistence is Phase 5's RCVR scope. A table now would be dead weight (rows can't survive restarts anyway). Recommend **in-memory `TaskID` field on Session**, no sessions table this phase. |

**Installation:** nothing to install.

## Empirical Git Findings (git 2.43.0, verified 2026-06-10)

All of the following were tested directly on this machine with throwaway repos. These are the load-bearing facts for the plan.

### 1. Default branch resolution (D-24)

| Probe | Behavior (verified) |
|-------|---------------------|
| `git -C <repo> symbolic-ref refs/remotes/origin/HEAD` | Healthy clone → prints `refs/remotes/origin/main`, exit 0. Strip the `refs/remotes/origin/` prefix to get the branch name. |
| Same, when origin/HEAD unset | exit 128, `fatal: ref refs/remotes/origin/HEAD is not a symbolic ref`. **This is common**: it is only set by `git clone` of a non-empty repo or explicit `git remote set-head`. Repos created via `git init` + `git remote add`, and clones of then-empty repos, have NO origin/HEAD. |
| No remote at all | Same exit-128 failure. |
| `git remote set-head origin --auto` | NOT usable — performs a network ls-remote, violating D-24's no-fetch rule. Never call it. |
| `git show-ref --verify --quiet refs/heads/main` | exit 0 if local `main` exists, 1 otherwise. Same for `master`. |
| `git symbolic-ref --short HEAD` | Prints current branch, exit 0. **Detached HEAD → exit 128** (`fatal: ref HEAD is not a symbolic ref`); fall through to `git rev-parse HEAD` which still works detached. |
| Empty repo (unborn HEAD) | `git rev-parse HEAD` and `worktree add ... HEAD` both fail (`fatal: invalid reference: HEAD`). Lands cleanly in the D-25 failed state with that error string. |

**Recommended chain** (each step local-only, no network):
1. `symbolic-ref refs/remotes/origin/HEAD` → name N. If local `refs/heads/N` exists, base = `N` (local tip per D-24); else base = `origin/N` (the remote-tracking ref is a valid commit-ish).
2. Else `refs/heads/main` if it exists, else `refs/heads/master`.
3. Else `symbolic-ref --short HEAD` (current branch); if detached, `rev-parse HEAD` (raw sha is a valid base).
4. If all fail (unborn HEAD) → creation fails into the D-25 warn state with git's stderr.

Note: when the base passed to `worktree add -b` is a remote-tracking ref (`origin/main`), git auto-sets upstream tracking on the new branch (`branch 'task/x' set up to track 'origin/main'.` on stderr). Verified harmless; do not parse it as an error.

### 2. `git worktree add -b <branch> <abs-path> <base>` semantics

| Case | Verified behavior |
|------|-------------------|
| Happy path | exit 0. `Preparing worktree (new branch 'X')` goes to **stderr**; `HEAD is now at <sha> <subject>` goes to stdout. Success ≠ empty stderr — **use exit code only**; keep stderr for error display. `-q` suppresses chatter if preferred. |
| Branch already exists | **exit 255** (not 128!), `fatal: a branch named 'X' already exists`. Path is NOT created. |
| Path exists, non-empty | exit 128, `fatal: '<path>' already exists`. **CRITICAL: the branch IS created before the path check fails and leaks** (verified: failed add left `task/other-2` in `git branch --list`). See Pitfall 1. |
| Path exists, empty dir | exit 0 — empty dirs are fine (`MkdirAll` of parents is safe; just never pre-create the leaf… though even that works). |
| Branch checked out in another worktree (add without `-b`) | exit 128, `fatal: 'X' is already used by worktree at '<path>'`. |
| Base = raw commit sha | Works; new branch points at the sha. |

**Exit codes are inconsistent (255 vs 128) — never switch on them.** Treat any nonzero as failure and relay trimmed stderr (strip `fatal: ` prefix) to the UI per the UI-SPEC `Couldn't create a worktree: {git error}` copy.

### 3. Dirty detection — `git -C <wt> status --porcelain=v2`

Verified output for a worktree with one modified tracked file, one staged file, one untracked dir containing a file:

```
1 .M N... 100644 100644 100644 <sha> <sha> file.txt
1 A. N... 000000 100644 100644 0000000000000000000000000000000000000000 <sha> untracked.txt
? newdir/
```

- Line prefixes: `1 ` ordinary change, `2 ` rename/copy, `u ` unmerged, `? ` untracked. Default untracked mode **collapses directories** (`? newdir/` = 1 line for any number of files). With `--untracked-files=all` each file gets its own `?` line.
- **Changed-file count for the D-33 dialog** = line count of `git status --porcelain=v2 --untracked-files=all`. Use `-z` (NUL-terminated, verified working) if paths are ever parsed; for a pure count, counting records is enough.
- Clean tree (including a clean initialized submodule) → zero lines. Dirty check: `len(lines) > 0`.

### 4. `git worktree remove` semantics

| Case | Verified behavior |
|------|-------------------|
| Clean worktree | plain `remove` → exit 0. Branch survives (verified in `git branch --list` after). |
| Modified files | exit 128, `fatal: '<path>' contains modified or untracked files, use --force to delete it`. |
| **Untracked files only** | **Same refusal** — force is required for ANY dirt, not just modifications. |
| `remove --force` on dirty | exit 0; directory gone; **branch kept** (verified). |
| Locked worktree (`git worktree lock`) | plain and single `--force` both fail exit 128 (`use 'remove -f -f' to override or unlock first`); only `--force --force` removes. The app never locks; if a user locked it manually, surface the error — do NOT auto-double-force. |
| **Live process cwd'd inside** | **remove succeeds (exit 0)** on Linux; the directory disappears and the process keeps running with an ENOENT cwd. No EBUSY. D-32's stop-first gate is therefore pure policy — git will not protect running sessions. The server MUST enforce the gate itself. |
| Worktree dir manually `rm -rf`'d | `git worktree list --porcelain` shows the entry with `prunable gitdir file points to non-existent location`. **`git worktree remove <missing-path>` succeeds (exit 0) and cleans the bookkeeping** in 2.43. `git worktree prune` also clears it. |
| Path never registered as a worktree | exit 128, `fatal: '<path>' is not a working tree`. For idempotency: if the dir is also absent, treat as already-removed success. |
| **Initialized submodule present** | **plain `remove` refuses even when CLEAN**: `fatal: working trees containing submodules cannot be moved or removed`, exit 128. Single `--force` removes it. Worktrees with *uninitialized* submodules remove fine without force. |

**Removal strategy for the cleanup endpoint:** run our own porcelain check first; if clean → try plain `remove`; if that fails with the submodule refusal and our check said clean → retry with `--force` (safe: we verified cleanliness ourselves). If dirty and the user passed the type-to-confirm gate → `remove --force` directly. Always follow with `git worktree prune` (D-34's "prune bookkeeping"; verified harmless no-op when nothing to prune).

### 5. Supporting commands

- `git worktree list --porcelain -z` — stable machine format; per-entry fields `worktree <path>`, `HEAD <sha>`, `branch <ref>`, plus `prunable <reason>` / `locked <reason>` lines when applicable. Useful for the Retry path (does any worktree already use this branch?) and self-healing.
- `git check-ref-format --branch <name>` — exit 0/1 validity oracle. Verified: `task/fix-login-42` OK; spaces and `..` FAIL; unicode (`task/трюк-1`) is accepted by git, but the recommended slug charset excludes it anyway.
- `git -C <wt> rev-parse --git-common-dir` from inside a linked worktree → main repo's `.git` (useful if the project dir itself is ever a worktree).
- Submodule init inside a worktree works: `git -C <wt> submodule update --init --recursive` (verified, clones into the worktree).

## Architecture Patterns

### Recommended Structure (delta)

```
internal/
├── worktree/            # NEW: git CLI service (worktree.go, worktree_test.go)
├── session/             # EXTEND: SpawnOpts{Cwd, TaskID}, per-task labels, ListByTask, StopAllForTask
├── api/
│   ├── tasks.go         # EXTEND: create() calls worktree service; task JSON gains worktree fields
│   ├── worktrees.go     # NEW: POST/GET/DELETE /api/tasks/{id}/worktree
│   └── sessions.go      # EXTEND: spawn body {task_id}, list ?task_id= filter
└── store/migrations/
    └── 00002_worktrees.sql  # NEW: ALTER TABLE tasks ADD COLUMN ×3

web/src/
├── api/sessions.ts      # EXTEND: useSessions(taskId?), useSpawnSession(taskId)
├── api/worktrees.ts     # NEW: useWorktree(taskId), useCreateWorktree, useCleanupWorktree
├── components/task/
│   ├── TaskTabs.tsx     # EXTEND: controlled value, closable tabs, trailing + button slot
│   ├── WorktreeMetaLine.tsx  # NEW: branch line / failed / absent states
│   └── CleanupWorktreeDialog.tsx  # NEW: single AlertDialog, 4 variants (a–d)
└── pages/
    ├── TaskPage.tsx     # EXTEND: full-width layout, meta line, bash tabs, menu item
    └── BoardPage.tsx    # EXTEND: cleanup dialog trigger after move-to-done persists
```

### Pattern 1: Worktree service — one exec call per operation

```go
// internal/worktree/worktree.go — every call: arg arrays, no shell, ctx timeout.
func gitRun(ctx context.Context, repo string, args ...string) (stdout string, err error) {
    cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
    var out, errb bytes.Buffer
    cmd.Stdout, cmd.Stderr = &out, &errb
    if err := cmd.Run(); err != nil {
        msg := strings.TrimSpace(errb.String())
        msg = strings.TrimPrefix(msg, "fatal: ") // UI shows "{git error}" per UI-SPEC
        return "", fmt.Errorf("%s", msg)
    }
    return out.String(), nil
}
```

Service surface: `ResolveBase(repo)`, `Create(repo, branch, path, base)`, `DirtyCount(wtPath)`, `Remove(wtPath, force bool)`, `Prune(repo)`. Each maps to one or two gitRun calls per the empirical findings above. A per-task mutex (or a single coarse mutex — single-user app) serializes create/remove/retry so Retry spam can't race.

### Pattern 2: Task-creation hook (D-25)

In `tasks.go create()`: INSERT the task first (capture id) → build slug/branch/path → attempt creation synchronously with a ~30s ctx timeout → on success `UPDATE tasks SET branch=?, worktree_path=?` → on failure `UPDATE tasks SET worktree_error=?` → **return 201 with the task either way**. The request never errors because of git.

Schema (migration `00002_worktrees.sql` — SQLite `ALTER TABLE ADD COLUMN`, no table rebuild needed):

```sql
-- +goose Up
ALTER TABLE tasks ADD COLUMN branch TEXT;
ALTER TABLE tasks ADD COLUMN worktree_path TEXT;
ALTER TABLE tasks ADD COLUMN worktree_error TEXT;
-- +goose Down
ALTER TABLE tasks DROP COLUMN worktree_error;
ALTER TABLE tasks DROP COLUMN worktree_path;
ALTER TABLE tasks DROP COLUMN branch;
```

UI state is derived, no status enum needed: `worktree_path != NULL` → active; `worktree_error != NULL` → failed (Retry); both NULL → absent (D-26 "Create worktree"). A separate worktrees table is over-modeling for a strict 1:1.

### Pattern 3: Worktree REST surface

| Endpoint | Purpose |
|----------|---------|
| `POST /api/tasks/{id}/worktree` | Create or Retry (D-25/D-26). Reuses the same service path as task-create. Returns updated task or `{error}` recorded in `worktree_error`. |
| `GET /api/tasks/{id}/worktree` | Fresh dialog state: `{branch, path, dirty_files (porcelain -uall count), running_sessions (manager count for task)}` — fetched when the dialog opens per UI-SPEC ("not from stale cache"). |
| `DELETE /api/tasks/{id}/worktree` body `{stop_sessions: bool, force: bool}` | Server-enforced gates mirroring the dialog: 409 if sessions running and `!stop_sessions`; 409 if dirty and `!force`. On confirm: StopAllForTask (blocks through the 5s grace) → Remove(force per dirty) → Prune → NULL out branch?/path columns (keep `branch`? — see below) → 204. |

After cleanup the task should land in the D-26 absent state (`No worktree yet.` + `Create worktree`). Recommendation: NULL `worktree_path` and `worktree_error`, and also NULL `branch` — the branch still exists in git (D-34) but the meta line slot shows the absent state per UI-SPEC; a later re-create will hit "branch already exists" and must reuse it (Pitfall 1's reuse path handles exactly this).

The Stop-then-remove sequence runs in the request handler; total worst case ≈ N×5s grace. Acceptable for a local app (the dialog shows `Cleaning up…`); the planner may cap it or stop sessions in parallel (each `Stop()` is already safe to run concurrently).

### Pattern 4: Slug generation (Claude's discretion — recommended rules)

Lowercase the title → map every run of characters outside `[a-z0-9]` to a single `-` → trim leading/trailing `-` → cap at 40 chars (re-trim trailing `-`) → if empty, use `task`. Result charset `[a-z0-9-]` is ref-safe by construction; branch = `task/<slug>-<id>`, worktree dir = `<slug>-<id>` (this dir name is the D-33 type-to-confirm target, e.g. `fix-login-42`). Validate in tests with `git check-ref-format --branch` as the oracle. Task IDs are globally unique (single AUTOINCREMENT-style table), so `<slug>-<id>` dirs are unique even across projects — the `<project>` path component (repo basename per the UI-SPEC tooltip example `~/.kangent/worktrees/my-repo/fix-login-42`) needs no de-dup. Expand `~` in Go (`os.UserHomeDir()`); git never expands it.

### Pattern 5: Session manager extension

```go
type SpawnOpts struct {
    Cwd    string // "" → home (preserves /terminal dev route behavior)
    TaskID int64  // 0 → unscoped dev session, label "bash #N" (global counter)
}
func (m *Manager) Spawn(opts SpawnOpts) (*Session, error)
```

- Validate `opts.Cwd` exists before spawn (a deleted worktree must produce the clean `Couldn't start a session` error, not a confusing shell error).
- Task-scoped labels: `map[int64]int` per-task counters → `Bash 1`, `Bash 2`… monotonic per task, never reused (UI-SPEC). Global counter stays for task-less sessions (`bash #N`).
- `Info` gains `TaskID int64` (omit/zero for dev sessions) so the REST list filter and the FE can scope.
- `List()` keeps returning everything; add filtering in the handler (`?task_id=`) or a `ListByTask(id)` helper.
- `StopAllForTask(taskID)` — collect running sessions for the task, `Stop()` each (concurrently is fine; Stop is idempotent and blocks until done).
- `/terminal` dev route: **keep** — with `Cwd:""`/`TaskID:0` defaults it costs nothing (the D-area recommendation).

### Pattern 6: Done-transition hook (frontend)

Drag is currently the only path into Done (`Board.tsx handleDragEnd` → `moveTask.mutate`); TaskPage has no status control. Per UI-SPEC the dialog fires AFTER the move persists and never blocks/rolls back the move. Recommendation: pass an `onSuccess` callback at the **call site** in `Board.tsx` (not inside the shared `useMoveTask` hook): if `args.status === "done"` and the moved task has `worktree_path`, set `cleanupTaskId` state on BoardPage → renders `<CleanupWorktreeDialog taskId=... trigger="done" />`. The same dialog component is used by the TaskPage ellipsis menu with `trigger="menu"` (cancel label `Cancel` vs `Keep worktree`, per UI-SPEC). The task list JSON must therefore include the worktree fields (extend the `Task` struct + `taskColumns`).

### Pattern 7: Tabs mirror sessions (D-28)

- `useSessions(taskId)` → `queryKey: ["sessions", taskId]`, `GET /api/sessions?task_id=N`, keep `refetchInterval: 5000`. TerminalPage keeps `useSessions()` (all).
- Tab construction in TaskPage: `tabs = [description, ...sessions.filter(s => s.status === "running" || locallyExited(s)).map(...)]`. Per UI-SPEC: a session that exits while the tab is open keeps its tab (muted) until banner-Close; on a later revisit only running sessions produce tabs. Practical approach: derive tabs from running sessions + a local "keep showing these exited ids" set, cleared on unmount.
- `TaskTabs` must become **controlled** (`value`/`onValueChange` lifted to TaskPage) — spawn activates the new tab; closing the active tab activates the left neighbor else Description. Radix Tabs with a `value` pointing at a removed tab renders empty content — reassign before/with removal.
- Extend `TabDef` with optional `onClose` (renders the × inside the trigger) and add a `trailing` slot (the `+` button + inline spawn error) to `TaskTabs`. Description tab gets no ×.
- Spawn flow: `useSpawnSession(taskId)` POSTs `{task_id}`; on success, write the returned session into the `["sessions", taskId]` cache via `setQueryData` (Phase 2's spawn-select race fix pattern) and activate its tab immediately — don't wait for invalidation.
- Close flow (D-29): × → `useStopSession` fires immediately → local "closing" state (muted label, × disabled) → session disappears from the running list (poll or `onSessionExit`) → remove tab. `TerminalPane` mounts unchanged inside `TabsContent` (D-12); wire `onNewTerminal` to the task-scoped spawn and `onClosed` to tab removal.
- Hidden-tab fitting is already guarded in TerminalPane (Phase 2) — keep tabs mounted (Radix default keeps content mounted but hidden; verify `forceMount` behavior vs the pane's fit guard during implementation).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Dirty detection | FS walking / mtime comparison | `git status --porcelain=v2 -uall` line count | git knows about ignores, index state, renames; verified stable format |
| Worktree deletion | `os.RemoveAll` on the worktree dir | `git worktree remove [--force]` + `prune` | rm -rf leaves stale `.git/worktrees` metadata → "already used by worktree" errors forever (PITFALLS.md; locked decision D-34) |
| Branch-name safety | regex guesswork over titles | constructive slug charset `[a-z0-9-]` + `git check-ref-format --branch` as test oracle | git's ref rules have ~12 edge cases (`..`, `@{`, `.lock`, trailing `/`…); a closed charset sidesteps all of them |
| Git error classification | parsing exit codes (255 vs 128 observed!) or localized message matching | pre-checks (path exists? branch exists via `show-ref`?) + relay stderr verbatim for display | exit codes are inconsistent across failure modes; messages are for humans |
| Default-branch detection | `git remote show origin` parsing | `symbolic-ref refs/remotes/origin/HEAD` + local fallbacks | `remote show` hits the network (violates D-24); symbolic-ref is a local read |
| Session→tab sync | client-side tab registry in localStorage/zustand | server sessions list as source of truth (D-28) + existing 5s poll | the server already owns session lifecycle; duplicating it client-side is the vibe-kanban ghost-run bug class |

## Common Pitfalls

### Pitfall 1: Branch leaks from a failed `worktree add -b` (verified)
**What goes wrong:** `git worktree add -b task/x <path> <base>` creates the branch BEFORE validating the path; if the path exists, the command fails (exit 128) but the branch remains. The user clicks Retry, the path issue is fixed, and now the add fails with `a branch named 'task/x' already exists` (exit 255) — permanently stuck.
**How to avoid:** (1) Pre-check the leaf path with `os.Stat` before invoking git (the app owns `~/.kangent/worktrees`, so collisions are bugs, not user data). (2) On Create/Retry, if the branch already exists AND `git worktree list --porcelain` shows no worktree using it, **reuse it**: `git worktree add <path> task/x` (no `-b`). This also covers re-creating a worktree after cleanup (branch was deliberately kept per D-34). Never delete the branch to "fix" this — reuse is strictly better and respects D-34's spirit.
**Warning signs:** Retry fails with "branch already exists" after a path-collision failure.

### Pitfall 2: Untracked files block plain remove
**What goes wrong:** Devs assume only *modified* files need `--force`. Verified: a worktree with ONLY untracked files refuses plain `remove` with the same fatal. If the cleanup flow computes "dirty" from tracked changes only, variant (a) "clean" cleanups will fail.
**How to avoid:** The dirty check MUST count untracked files (`-uall`), matching what `remove` itself checks. Clean per our check → plain remove succeeds (verified).

### Pitfall 3: Clean worktrees with initialized submodules refuse removal
**What goes wrong:** `fatal: working trees containing submodules cannot be moved or removed` on a perfectly clean tree; the cleanup dialog shows the failure inline and the user is stuck.
**How to avoid:** If plain remove fails AND our porcelain check said clean, retry with `--force` (verified sufficient; `-f -f` is only for locks). Combined with submodule handling (below) this is the whole story.

### Pitfall 4: git will happily delete a worktree with live sessions inside
**What goes wrong:** Assuming `git worktree remove` fails with EBUSY while shells are cwd'd in the tree, and using that as the GIT-03 "refuses while running" enforcement. Verified: removal succeeds; processes survive with ENOENT cwd and every subsequent command in those shells fails confusingly.
**How to avoid:** The DELETE handler must enforce the running-sessions gate itself (count from the session manager) and only proceed after `StopAllForTask` completes. There is no git-level safety net.

### Pitfall 5: Treating stderr output or specific exit codes as signal
**What goes wrong:** Successful `worktree add` prints `Preparing worktree…` (and possibly `set up to track…`) on stderr; failures use exit 255 OR 128 depending on the mode. Code that flags "stderr non-empty" or matches exit codes misclassifies.
**How to avoid:** Success = exit 0, full stop. stderr is display-only material for the `{git error}` UI copy.

### Pitfall 6: origin/HEAD absent on perfectly normal repos
**What goes wrong:** Testing only against fresh clones (where origin/HEAD exists) and shipping a resolver that breaks on `git init`-born repos, empty-clone repos, or no-remote repos — all exit-128 on the symbolic-ref probe.
**How to avoid:** Implement the full 4-step chain (§Empirical 1); test each leg: no origin/HEAD, no main/master, detached HEAD, unborn HEAD.

### Pitfall 7: Radix Tabs value pointing at a removed tab
**What goes wrong:** Closing the active bash tab removes its TabDef while `value` still references it — the strip renders with nothing selected and the content area goes blank.
**How to avoid:** Make TaskTabs controlled; compute the next active tab (left neighbor else `description`) in the same state update that removes the tab (UI-SPEC interaction contract).

### Pitfall 8: Cleanup dialog acting on stale state
**What goes wrong:** Dirty count / session count captured when the task page loaded, not when the dialog opened — the user confirms variant (a) while an agent wrote files in between (the vibe-kanban lost-work class).
**How to avoid:** `GET /api/tasks/{id}/worktree` on dialog open (UI-SPEC: "fetched when the dialog opens"); additionally the server re-checks dirty/running at DELETE time and 409s if the client's `force`/`stop_sessions` flags don't cover reality — the dialog reopens-or-errors instead of deleting work.

### Pitfall 9: Worktree creation inside a DB transaction or request-blocking lock
**What goes wrong:** `worktree add` materializes a full checkout (seconds on big repos). Holding a SQLite write tx or a global mutex across it stalls the whole app (Phase-1 SQLite discipline: never hold a tx across git operations).
**How to avoid:** INSERT/UPDATE around the git call, never spanning it; per-task (not global) locking for worktree ops.

## Submodule Handling (Claude's discretion — recommendation)

Verified: `worktree add` does NOT populate submodules (dir exists empty, `submodule status` shows `-` prefix). Recommendation — cheap and sensible:
1. After successful create, check for `.gitmodules` at the worktree root; if present, run `git -C <wt> submodule update --init --recursive`. Verified working inside linked worktrees. If it fails (e.g. needs network), log + continue — the worktree is still usable; do not fail creation (D-25 spirit). Optionally record nothing: bash tabs let the user run it manually.
2. On cleanup, the clean-but-refused→`--force` retry (Pitfall 3) covers initialized submodules.
This avoids both documented limitations with ~10 lines.

## Code Examples

### Base resolution chain (Go sketch)

```go
// ResolveBase returns a commit-ish to branch from, per D-24. No network.
func ResolveBase(ctx context.Context, repo string) (string, error) {
    if out, err := gitRun(ctx, repo, "symbolic-ref", "refs/remotes/origin/HEAD"); err == nil {
        name := strings.TrimPrefix(strings.TrimSpace(out), "refs/remotes/origin/")
        if _, err := gitRun(ctx, repo, "show-ref", "--verify", "--quiet", "refs/heads/"+name); err == nil {
            return name, nil // local tip of the default branch
        }
        return "origin/" + name, nil // remote-tracking ref as commit-ish
    }
    for _, b := range []string{"main", "master"} {
        if _, err := gitRun(ctx, repo, "show-ref", "--verify", "--quiet", "refs/heads/"+b); err == nil {
            return b, nil
        }
    }
    if out, err := gitRun(ctx, repo, "symbolic-ref", "--short", "HEAD"); err == nil {
        return strings.TrimSpace(out), nil
    }
    out, err := gitRun(ctx, repo, "rev-parse", "HEAD") // detached HEAD
    if err != nil {
        return "", err // unborn HEAD → D-25 failed state
    }
    return strings.TrimSpace(out), nil
}
```

### Create with leak-safe retry (verified semantics)

```go
func Create(ctx context.Context, repo, branch, path, base string) error {
    if _, err := os.Stat(path); err == nil {
        return fmt.Errorf("worktree path already exists: %s", path)
    }
    if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
        return err
    }
    // Branch may already exist (kept after cleanup per D-34, or leaked by a
    // previous failed add). If no worktree uses it, reuse instead of -b.
    if _, err := gitRun(ctx, repo, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
        _, err := gitRun(ctx, repo, "worktree", "add", path, branch)
        return err
    }
    _, err := gitRun(ctx, repo, "worktree", "add", "-b", branch, path, base)
    return err
}
```

### Dirty count (D-33 dialog)

```go
func DirtyCount(ctx context.Context, wt string) (int, error) {
    out, err := gitRun(ctx, wt, "status", "--porcelain=v2", "--untracked-files=all", "-z")
    if err != nil {
        return 0, err
    }
    n := 0
    for _, rec := range strings.Split(out, "\x00") {
        if strings.TrimSpace(rec) != "" {
            n++
        }
    }
    return n, nil
}
```

### Remove with submodule fallback (verified behaviors)

```go
func Remove(ctx context.Context, repo, wt string, force bool) error {
    args := []string{"worktree", "remove"}
    if force {
        args = append(args, "--force")
    }
    if _, err := gitRun(ctx, repo, append(args, wt)...); err != nil {
        if !force && strings.Contains(err.Error(), "submodules") {
            // Clean per our check but initialized submodules present (verified refusal).
            _, err = gitRun(ctx, repo, "worktree", "remove", "--force", wt)
        }
        if err != nil {
            // Idempotency: never-registered path + dir absent = already clean.
            if strings.Contains(err.Error(), "is not a working tree") {
                if _, statErr := os.Stat(wt); os.IsNotExist(statErr) {
                    err = nil
                }
            }
        }
        if err != nil {
            return err
        }
    }
    _, _ = gitRun(ctx, repo, "worktree", "prune") // D-34 bookkeeping; harmless no-op
    return nil
}
```

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| git | entire phase | ✓ | 2.43.0 (all behaviors verified against this exact binary) | — |
| Go | backend | ✓ | 1.26.0 | — |
| Node | frontend build | ✓ | v24.4.1 (meets Vite 8 req) | — |

**Missing dependencies:** none. Note: minimum git version for the features used is ancient (worktree remove/prune since 2.17, porcelain=v2 since 2.11); the self-healing `remove`-on-missing-dir behavior was verified on 2.43 specifically — keep `prune` in the flow as the portable belt-and-braces.

## Open Questions

1. **Sync vs async worktree creation in POST task**
   - What we know: sync is simplest and matches the UI-SPEC state machine (failed/absent/active — no "creating during initial create" state exists in the spec); `worktree add` is a full checkout and can take seconds on very large repos.
   - What's unclear: whether target repos are large enough to make the create-task request feel slow.
   - Recommendation: sync with a 30s context timeout; timeout/error → `worktree_error` recorded, task created anyway (D-25). Revisit only if it hurts.

2. **Keeping exited-tab ghosts within a visit (D-28/D-29 interplay)**
   - What we know: UI-SPEC: exited session's tab stays (muted) until banner-Close, but produces no tab on a later revisit; the server list is truth and includes exited sessions until DELETEd.
   - What's unclear: exact mechanism — filter list to running + local keep-alive set, vs. filtering by "exited before page mount".
   - Recommendation: local component state (set of session ids seen running this mount); planner picks the simplest implementation that satisfies both UI-SPEC lines.

3. **Stop-sessions duration in the cleanup request**
   - What we know: each Stop blocks up to 5s grace; N sessions could make DELETE take ~5s+ (UI shows `Cleaning up…`).
   - Recommendation: stop concurrently (Stop is already concurrent-safe); accept the worst-case single 5s grace window.

## Sources

### Primary (HIGH confidence)
- **Empirical verification on installed git 2.43.0** (2026-06-10, /tmp scratch repos): default-branch resolution probes (clone/no-remote/detached/unborn), `worktree add -b` failure matrix incl. branch-leak-on-path-collision, `status --porcelain=v2` output incl. `-uall` and `-z`, `worktree remove` matrix (clean/modified/untracked-only/locked `-f -f`/live-process-cwd/manual-rm/submodule-initialized/never-registered), `worktree list --porcelain` prunable annotation, `check-ref-format --branch` cases, submodule init-in-worktree, upstream auto-tracking from remote-tracking base — this is the authoritative source for every claim marked "verified"
- Existing codebase (read 2026-06-10): `internal/session/{manager,session}.go`, `internal/api/{tasks,sessions,routes}.go`, `internal/store/migrations/00001_init.sql` + `migrate.go`, `web/src/components/task/TaskTabs.tsx`, `web/src/pages/TaskPage.tsx`, `web/src/api/{sessions,mutations}.ts`, `web/src/components/terminal/TerminalPane.tsx`, `Board.tsx` seams
- `.planning/phases/03-worktree-isolation-bash-tabs/03-UI-SPEC.md` (approved contract — dialog variants, tab strip, copy)

### Secondary (MEDIUM confidence)
- `.planning/research/PITFALLS.md` worktree section (Pitfall 7) — corroborated and refined by the empirical runs (the EBUSY claim there is corrected: removal does NOT fail with live processes on Linux)
- `.planning/research/STACK.md` — git-CLI-over-go-git decision (locked)

### Tertiary (LOW confidence)
- None — no unverified web claims are load-bearing in this document.

## Metadata

**Confidence breakdown:**
- Git behaviors: HIGH — every claim executed against the exact installed binary
- Architecture/schema/API shape: HIGH — derived from locked decisions + read code; choices in discretion areas flagged as recommendations
- Frontend tab mechanics: MEDIUM-HIGH — patterns follow Phase 2 precedents read from code; Radix Tabs mounted/hidden-content nuance flagged for implementation-time verification

**Research date:** 2026-06-10
**Valid until:** ~2026-12 (git CLI semantics are extremely stable; revisit only if the host git is upgraded across a major version)
