---
phase: 21-data-directory-migration
reviewed: 2026-07-01T00:00:00Z
depth: standard
files_reviewed: 15
files_reviewed_list:
  - cmd/kamacu/main.go
  - internal/api/projects.go
  - internal/api/sessions.go
  - internal/migrate/migrate.go
  - internal/migrate/migrate_test.go
  - internal/migrate/paths.go
  - internal/migrate/paths_test.go
  - internal/migrate/worktree_repair_test.go
  - internal/settings/settings.go
  - internal/tmux/tmux.go
  - web/src/components/board/ReviewColumn.tsx
  - web/src/components/layout/ActiveSessionsBar.tsx
  - web/src/components/layout/AppLayout.tsx
  - web/src/lib/migrateStorage.ts
  - web/src/main.tsx
findings:
  critical: 1
  warning: 3
  info: 2
  total: 6
status: issues_found
---

# Phase 21: Code Review Report

**Reviewed:** 2026-07-01
**Depth:** standard
**Files Reviewed:** 15
**Status:** issues_found

## Summary

Reviewed the one-time gated `~/.kangent` → `~/.kamacu` migration (Part 1 `migrate.Prepare`, Part 2 `migrate.Complete`), its wiring in `cmd/kamacu/main.go`, the supporting settings/tmux leaves, and the frontend localStorage one-shot.

The commit-point mechanics are strong: the preflight-before-rename ordering, EXDEV refusal, WAL-checkpoint-before-DB-rename, both-dirs RefuseBoot anomaly, and the DoMigrate/RollForward idempotency are all well-constructed and well-tested (the hot-WAL guard test in particular is excellent). The DB path rewrite correctly uses the `managed` flag as the single source of truth and a boundary-anchored `/%` LIKE gate, and it never does a global string replace.

However, the git worktree repair step has a **scope gap that risks worktree loss for folder-pointed (unmanaged) projects** — the highest-severity finding, detailed below. Three correctness/robustness warnings and two informational items round out the review.

## Critical Issues

### CR-01: Worktree repair skips folder-pointed (unmanaged) projects → their moved worktrees are left prunable and get deregistered by a later `git worktree prune`

**File:** `internal/migrate/paths.go:192-248` (and `:194`)
**Issue:**
`repairWorktrees` only iterates managed clones:

```go
rows, err := db.Query(
    `SELECT id, repo_path FROM projects WHERE managed=1 AND repo_path LIKE ?`,
    cfg.NewRoot+"/%")
```

But task worktrees are created under the global `worktree_base` (`~/.kangent/worktrees/…`) for **every** project regardless of `managed` — `provisionWorktree` (`internal/api/tasks.go:120`) always calls `worktree.PathUnder(baseDir, repoPath, …)`; the `managed` bool only gates a best-effort fetch, never the placement. So a folder-pointed (managed=0) project — whose main repo lives *outside* the data root and does **not** move — still has task worktrees under `~/.kangent/worktrees/` that the Part-1 `os.Rename` relocates to `~/.kamacu/worktrees/`.

After the move, that worktree's two-way link is half-broken:
- worktree → repo (`<worktree>/.git` → `<external repo>/.git/worktrees/<id>`) survives, because the external repo did not move; but
- repo → worktree (`<external repo>/.git/worktrees/<id>/gitdir` → `~/.kangent/worktrees/…/.git`) is now **stale** (points at the vanished old path).

Because `repairWorktrees` filters `managed=1`, `git -C <external repo> worktree repair <new path>` is never run for these projects, so the stale pointer is never fixed. This is not merely cosmetic: `git worktree prune` treats a worktree whose recorded `gitdir` path is missing as a removed working tree and **deregisters it**. And `worktree.Remove` runs prune on the repo after every task-worktree removal:

```go
// internal/worktree/worktree.go:378
_, _ = gitRun(ctx, repo, "worktree", "prune")
```

So the first time the user deletes *any* task in a folder-pointed project post-migration, git prunes the *other* (unrepaired) worktrees in that same repo, orphaning live checkouts that may hold uncommitted agent work. Note the DB side is already rewritten to the new path (`rewriteManagedPaths` correctly rewrites `tasks.worktree_path` for all tasks, not just managed), so the DB will point at a worktree git has just deregistered — a silent integrity break for a whole first-class project class, in the milestone's highest-risk phase. It is also untested: every repair test uses `managed=1` fixtures.

**Fix:** Repair worktrees for all projects that own moved worktrees, running repair from each project's own `repo_path` (which for unmanaged projects is the unmoved external checkout):

```go
// Select every project with tasks whose worktrees moved, not just managed ones.
rows, err := db.Query(`SELECT id, repo_path FROM projects`)
// ...collect (id, repoPath)...
for _, r := range repos {
    paths, err := existingWorktreePaths(db, r.id) // already-rewritten new paths, os.Stat-filtered
    if err != nil {
        return err
    }
    for _, p := range paths {
        cmd := exec.CommandContext(ctx, "git", "-C", r.repoPath, "worktree", "repair", p)
        // ...existing per-path warn-and-skip tolerance...
    }
}
```

`existingWorktreePaths` already scopes to the project's own tasks and os.Stat-filters, so a project with zero moved worktrees is a clean no-op. Add a repair test that uses a `managed=0` project whose worktree lived under the old root.

## Warnings

### WR-01: Path rewrite trusts an unescaped LIKE match and never re-checks the prefix in Go before UPDATE → a false LIKE match writes a corrupted path

**File:** `internal/migrate/paths.go:89-121` (and the `LIKE ?` gates at `:68`, `:74`, `:194`)
**Issue:**
The LIKE pattern is `cfg.OldRoot+"/%"`, where `OldRoot` is the home-expanded absolute path. SQLite `LIKE` treats `_` and `%` as metacharacters, and they are not escaped. A home directory containing `_` (legal in Unix usernames, e.g. `/home/john_doe/.kangent`) makes the `_` match any single character, so the pattern can match a row that is *not* actually prefixed by `OldRoot`. `rewritePrefixColumn` then does:

```go
newPath := filepath.Join(cfg.NewRoot, strings.TrimPrefix(oldPath, cfg.OldRoot))
```

If `oldPath` is not truly prefixed, `TrimPrefix` returns it unchanged and `filepath.Join(NewRoot, <absolute oldPath>)` nests the whole old path under `NewRoot` — a corrupted value that is then written back by the UPDATE. There is no Go-side `HasPrefix` guard to catch this; the code relies entirely on the LIKE. Real-world blast radius is small on a single-user host (a colliding sibling path must also exist in the DB), but the failure mode is silent path corruption in the exact code the phase brief flags as data-loss-sensitive.

**Fix:** Escape the LIKE pattern (add `ESCAPE '\'` and backslash-escape `%`/`_`/`\` in `OldRoot`) *and* add a defensive Go-side guard so a non-prefixed row is skipped rather than corrupted:

```go
for rows.Next() {
    // ...scan id, oldPath...
    if oldPath != cfg.OldRoot && !strings.HasPrefix(oldPath, cfg.OldRoot+string(os.PathSeparator)) {
        continue // LIKE over-matched (metachar); never rewrite a non-prefixed path
    }
    // ...
}
```

### WR-02: `rewriteWorktreeBaseSetting` prefix match has no separator boundary → sibling dirs like `~/.kangent-backup` are mis-rewritten

**File:** `internal/migrate/paths.go:139-147`
**Issue:**

```go
case strings.HasPrefix(value, oldDataDir): // oldDataDir == "~/.kangent"
    newValue = newDataDir + strings.TrimPrefix(value, oldDataDir)
case strings.HasPrefix(value, cfg.OldRoot): // e.g. "/home/u/.kangent"
    newValue = cfg.NewRoot + strings.TrimPrefix(value, cfg.OldRoot)
```

`HasPrefix` matches without a trailing-separator boundary, so a `worktree_base` of `~/.kangent-backup/worktrees/` (or the expanded `/home/u/.kangent-old/...`) satisfies `HasPrefix(value, "~/.kangent")` and is rewritten to `~/.kamacu-backup/worktrees/` — silently corrupting a setting that points *outside* the migrated root. The DB path rewrite got the boundary right (`OldRoot+"/%"`); this setting rewrite did not.

**Fix:** Require the prefix to be the whole value or be followed by a path separator:

```go
func hasRootPrefix(value, root string) bool {
    return value == root || strings.HasPrefix(value, root+"/")
}
switch {
case hasRootPrefix(value, oldDataDir):
    newValue = newDataDir + strings.TrimPrefix(value, oldDataDir)
case hasRootPrefix(value, cfg.OldRoot):
    newValue = cfg.NewRoot + strings.TrimPrefix(value, cfg.OldRoot)
default:
    return nil
}
```

### WR-03: `migrateStorage` inserts keys into `localStorage` while iterating it by index

**File:** `web/src/lib/migrateStorage.ts:28-38`
**Issue:** The loop walks `localStorage` by numeric index (`localStorage.key(i)` for `i < localStorage.length`) while calling `localStorage.setItem(newKey, …)` inside the same loop, mutating the collection being iterated. In engines that keep insertion order and append new keys at the end this happens to be safe (new `kamacu.*` keys land past the current cursor), but localStorage key ordering under concurrent mutation has historically been engine-inconsistent, and this is a **one-shot, guard-flagged** migration: any key skipped because indices shifted is copied *never* (the `kamacu.storage-migrated` guard makes every later boot a no-op), permanently dropping that UI preference across the rebrand. The trivial, doubt-free fix is to snapshot the key list first.

**Fix:**

```ts
const keys: string[] = [];
for (let i = 0; i < localStorage.length; i++) {
  const k = localStorage.key(i);
  if (k) keys.push(k);
}
for (const key of keys) {
  if (!key.startsWith("kangent")) continue;
  const newKey = "kamacu" + key.slice("kangent".length);
  if (localStorage.getItem(newKey) === null) {
    const val = localStorage.getItem(key);
    if (val !== null) localStorage.setItem(newKey, val);
  }
}
```

## Info

### IN-01: Migration git repair runs under `context.Background()` with no timeout

**File:** `cmd/kamacu/main.go:113` → `internal/migrate/paths.go:233`
**Issue:** `migrate.Complete` is called with `context.Background()`, and `repairWorktrees` builds each `exec.CommandContext(ctx, "git", …)` from it. `git worktree repair` is normally fast and local, but a git invocation that hangs (e.g. a filesystem/network stall on a submodule-laden repo, or a git credential prompt) would block startup indefinitely with no visible progress, since this runs before the server binds. Elsewhere the codebase bounds git/tmux execs (e.g. `tmux.execTimeout`, the 30s worktree provisioning ctx). Consider wrapping the migration completion in a bounded `context.WithTimeout` so a stuck repair fails loudly (refuse-to-serve, re-run next boot) instead of hanging the boot.

### IN-02: `sameFilesystem` follows symlinks while `os.Rename` acts on the link inode

**File:** `internal/migrate/migrate.go:273-288`
**Issue:** `sameFilesystem` stats `src` with `os.Stat` (which follows symlinks) and compares `st_dev` against the parent of `target`. If `~/.kangent` is itself a symlink to another filesystem, `os.Stat` reports the *target's* device (possibly a different fs → false EXDEV refusal), whereas `os.Rename` would operate on the symlink inode (on the home fs). The mismatch could either refuse a migration that would actually succeed, or (less likely) pass preflight and then move only the symlink, leaving the real data behind. A symlinked data root is unusual, but if robustness here matters, `os.Lstat(src)` would match `os.Rename`'s semantics. No action strictly required for the common case.

---

_Reviewed: 2026-07-01_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
