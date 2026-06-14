# Phase 14: Managed Checkout Foundations - Research

**Researched:** 2026-06-14
**Domain:** Go backend — `gh repo clone` provisioning, SQLite migration, git worktree lifecycle, gated directory removal
**Confidence:** HIGH (all `gh`/git behaviors empirically verified on this host: `gh` 2.82.0, git 2.43.0)

## Summary

Phase 14 adds a **second project-creation path** alongside the existing folder path: validate a GitHub `owner/name` with the already-built `github.ValidateRepo`, `gh repo clone` it into `~/.kangent/repos/<owner>/<name>`, and record the resulting clone as the project's `repo_path` with a new schema marker that flags it Kangent-managed. Everything downstream that reads `repo_path` (worktree provisioning, diff, github-origin) works unchanged because the managed clone *is* a normal git checkout — the only behavioral deltas are a best-effort pre-task default-branch fetch (managed-only) and a gated, all-or-nothing directory removal on project delete.

The single highest-risk requirement is **CKOUT-04's "no half-created project"** (clone-then-create atomicity). Empirically, `gh repo clone` / `git clone` already give us most of this for free: a repo-not-found leaves **no directory** (verified, exit 1), a SIGINT mid-clone **removes the partial directory git created** (verified), and a non-empty destination **fails before writing anything** (verified, exit 1). The plan still wraps every failure path in an `os.RemoveAll(dest)` belt-and-braces and creates the DB row **only after** the clone returns exit 0.

**Important correction to the phase brief:** `gh` is **present and authenticated** on this host (`gh version 2.82.0`, `gh auth status` → logged in as `jordeu`, git protocol ssh). The brief asserted `gh` was absent; that is wrong for this machine, so every `gh repo clone` contract below is **host-verified**, not documented-only. The gh-absent degrade path is still designed (it is a real runtime possibility on other machines), but it did not need to be reasoned from docs.

**Primary recommendation:** Add a `github.Clone(ctx, ref, dest)` verb (host-verified `gh repo clone` wrapper, exit-0-only success, `os.RemoveAll` on failure) and a small `internal/api` orchestration in `projects.create`/`projects.delete`; add migration `00008` with a single `managed INTEGER NOT NULL DEFAULT 0` column on `projects`; insert one best-effort `git fetch origin <default>` in `provisionWorktree` gated on `managed`; reuse `CleanupWorktreeGated` per task worktree + an explicit clone-root gate for the gated delete.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| **CKOUT-01** | `gh repo clone` into `~/.kangent/repos/<owner>/<name>`, default branch auto-checked-out, becomes the project repo root, recorded with the managed marker | §"gh repo clone — Host-Verified Contract", §"Clone-then-Create Ordering", §"Schema Marker (migration 00008)" |
| **RPROJ-05** | gh-validate the repo BEFORE any clone; reject invalid/inaccessible with no clone, no project | Reuse existing `github.ValidateRepo` (projects.go:193 precedent); §"Validate-before-clone" |
| **CKOUT-02** | Task/PR-review worktrees branch off the managed clone; best-effort fetch latest default branch first; folder projects unchanged | §"Pre-task Fetch Insertion (provisionWorktree)" |
| **CKOUT-05** | Re-adding an existing managed dir reattaches/reuses rather than failing or cloning over | §"Reattach on Existing Dir" |
| **CKOUT-03** | Gated, all-or-nothing delete of the managed clone + its task worktrees; folder dirs never touched | §"Gated Delete of a Managed Clone" |
</phase_requirements>

## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01** synchronous clone-then-create — atomic; row created **only after** clone succeeds; failed clone → no row, no dir (satisfies CKOUT-04).
- **D-02** clone via `gh repo clone <owner>/<name> <dest>` into `~/.kangent/repos/<owner>/<name>`; full clone; default branch auto-checked-out; no separate default-branch detection for the initial checkout.
- **D-03** gh-validate (reuse `github.ValidateRepo` + `github.ParseRepoRef`) **before** any clone (RPROJ-05); invalid/inaccessible rejected, no clone, no row.
- **D-04** before each new task/PR-review worktree on a **managed** checkout, fetch latest default branch from origin.
- **D-05** fetch is **best-effort** — on failure proceed from local base (degrade-don't-break); managed-only relaxation of `ResolveBase`'s D-24 local-only rule; folder projects keep D-24 exactly.
- **D-06** new schema marker via migration **00008** distinguishing managed checkouts from folders; column shape is planner's call (contract: app can reliably tell "I cloned and own this dir" from "user pointed me at theirs").
- **D-07** managed-checkout delete is gated, all-or-nothing — clean up task worktrees AND the clone; if ANY (worktree or clone) is dirty/unpushed/has-stash/has-session, refuse with a reason list, remove nothing. Reuses `CleanupWorktreeGated` gate set.
- **D-08** all-or-nothing rationale: removing the clone dir discards local branches → the **unpushed gate is the safety net**; D-34 "never `git branch -D`" still holds for worktree removal; wholesale clone-dir removal only after all gates pass.
- **D-09** folder-based (user-pointed) project delete keeps today's behavior exactly — directory on disk never touched (current PROJ-03). Gated-removal applies only to managed checkouts.
- **D-10** existing managed dir on disk: if its `origin` canonicalizes (via `ParseRepoRef`) to the **same** `owner/name` → reattach/reuse (no re-clone); if it does NOT match → **error**, never clobber.
- **D-11** reattach never forces a re-clone or destructive reset; a freshness fetch on reattach is acceptable under the same best-effort rule as D-05 (never block on a failed fetch).

### Claude's Discretion
- **Repos base location:** hardcode `~/.kangent/repos/` for v1.4 (no new user setting). Planner MAY mirror the `worktree_base` settings pattern with a `repos_base` setting only if trivial and consistent — not required, not user-facing.
- **Schema marker column shape** (D-06) — planner chooses the concrete representation.
- **Where the clone primitive lives** — natural home `internal/github` for the `gh repo clone` verb and/or `internal/worktree`/a small provisioning helper for repo-root + reattach + gated-delete orchestration; planner decides the split.
- **`github_repo` link on a managed project** — a managed project's `github_repo` is the cloned `owner/name`; planner decides whether Phase 14's primitive sets it or Phase 15 does.

### Deferred Ideas (OUT OF SCOPE)
- On-demand "sync" of a managed checkout's default branch from the project view — **CKMNT-01**.
- Non-default base branch at project creation — **CKMNT-02**.
- Shallow/partial clone for large repos — **CKMNT-03**.
- Live clone-progress streaming + cancel (the async-provisioning alternative rejected in D-01) — **CKUX-01**.
- A `repos_base` user setting (unless trivially consistent with `worktree_base`).
- The repo-first Add-project UI, name/link/description auto-fill, inline clone-failure surfacing — **Phase 15** (next phase, not deferred).

## Project Constraints (from CLAUDE.md)

- **Tech stack:** Go backend (Go 1.26 line), `net/http` stdlib ServeMux, `modernc.org/sqlite`, `goose` migrations via `embed.FS`. No new deps needed for this phase.
- **Shell out to git/gh with arg arrays, `cmd.Dir`/`-C <repo>`, never `sh -c`** — established invariant (projects.go:55, worktree.go:33, github.go:90). The clone verb MUST follow this (task titles / owner-name flow near these commands — injection surface).
- **`gh` is a soft dependency** — every gh path degrades (no hard crash) when gh is absent/unauthenticated (github.go package doc). The repo-first path must fail gracefully; the folder path (RPROJ-04) must stay fully usable.
- **GSD workflow enforcement** — file edits go through a GSD command; this is research only.
- **modernc/SQLite ALTER TABLE pattern** — one `ADD COLUMN` per statement; NOT NULL needs a DEFAULT (migration 00007 precedent). modernc tracks SQLite 3.53 → `DROP COLUMN` works directly in the Down (no table rebuild).

## Standard Stack

No new libraries. Everything is stdlib + existing internal packages.

| Component | Source | Purpose | Reuse point (file:line) |
|-----------|--------|---------|--------------------------|
| `gh repo clone` | host `gh` 2.82.0 | clone into namespaced dest | NEW `github.Clone` verb |
| `github.ValidateRepo` | `internal/github/github.go:82` | gh-validate before clone (RPROJ-05/D-03) | call as-is |
| `github.ParseRepoRef` | `internal/github/github.go:40` | canonicalize ref + reattach origin match (D-03/D-10) | call as-is |
| `github.Available` | `internal/github/github.go:167` | gh-present check for degrade path | call as-is |
| `CleanupWorktreeGated` | `internal/api/cleanup.go:36` | per-task-worktree gated removal (CKOUT-03/D-07) | call per worktree |
| `worktree.DirtyCount/UnpushedCount/StashCount` | `internal/worktree/worktree.go:269/288/307` | clone-root gates (D-07/D-08) | call on clone root |
| `worktree.Remove` | `internal/worktree/worktree.go:337` | remove each linked worktree before clone-dir rm | call per linked wt |
| `worktree.ResolveBase` | `internal/worktree/worktree.go:122` | branch base (unchanged for both project kinds) | call as-is |
| `settings.ExpandHome` | `internal/settings` | `~/.kangent/repos/` → abs path | call as-is |
| `goose` + `embed.FS` | existing migration runner | migration 00008 | `internal/store/migrations/` |

**Version verification (host-verified 2026-06-14):**
```
gh version 2.82.0 (2025-10-15)   # gh PRESENT + authenticated (account jordeu, ssh protocol)
git version 2.43.0               # matches the version every worktree behavior was verified against
```

## gh repo clone — Host-Verified Contract

All of the following were run against `gh` 2.82.0 / git 2.43.0 on this host (CKOUT-01, CKOUT-04, CKOUT-05 all rest on these):

| Scenario | Command | Result | Exit | Implication |
|----------|---------|--------|------|-------------|
| Clone into a NEW nested dest (parent missing) | `gh repo clone octocat/Hello-World <tmp>/sub1/Hello-World` | Cloned; **parent dirs auto-created**; default branch `master` checked out; `origin/HEAD` → `refs/remotes/origin/master`; `.git` + working files present | 0 | No pre-`MkdirAll` of the parent strictly required, but do it anyway for the gh-absent `git clone` fallback. |
| Dest is an EXISTING NON-EMPTY dir | `gh repo clone ... <nonempty>` | `fatal: destination path '...' already exists and is not an empty directory.` `failed to run git: exit status 128` | 1 | **Clean refusal, nothing written** — supports atomicity. |
| Dest is an EXISTING EMPTY dir | `gh repo clone ... <emptydir>` | Clones successfully into it | 0 | An empty pre-created dir is fine. |
| Repo NOT FOUND | `gh repo clone octocat/this-repo-does-not-exist-xyz123 <dest>` | `GraphQL: Could not resolve to a Repository ...` | 1 | **No directory left on disk** (verified `ls` → No such file). |
| Parent path is a FILE | `gh repo clone ... <afile>/repo` | `fatal: could not create leading directories ... Not a directory` | 1 | Can't happen with our `<owner>/<name>` layout, but error is clean. |
| Extra git flags | `gh repo clone owner/name <dest> -- --no-tags` | Passthrough works; everything after `--` goes to `git clone` | 0 | Not needed for v1.4 (shallow is deferred CKMNT-03), but the seam exists. |
| SIGINT mid-clone | `timeout -s INT 0.4 git clone https://github.com/torvalds/linux.git <dest>` | clone aborts; **dest dir REMOVED by git** | (124) | git cleans up the partial dir it created. |

**Documented exit codes** (`gh help exit-codes`): 0 success · 1 generic failure · 2 cancelled · **4 authentication required**. So the gh-absent vs unauthenticated vs not-found cases can be distinguished if needed — but the plan does NOT need to branch on these: RPROJ-05 already runs `ValidateRepo` first, so by the time `Clone` runs the repo is known-valid-and-accessible; any clone failure is then network/disk/transient and surfaces as one inline error.

**Default-branch auto-checkout (CKOUT-01) — CONFIRMED:** the clone checks out the remote's default branch (`master` for Hello-World) and sets `refs/remotes/origin/HEAD`. **No separate default-branch detection or checkout step is needed** for the initial provision — exactly as D-02 states. `worktree.ResolveBase` then works on the clone with zero changes (it reads `origin/HEAD` first).

**origin URL form (matters for reattach, D-10):** with this host's `git_protocol=ssh`, `gh repo clone` sets `origin` to `git@github.com:octocat/Hello-World.git`. `github.ParseRepoRef` (github.go:51) explicitly strips the `git@github.com:` prefix and `.git` suffix → `octocat/Hello-World`. So reattach's origin-match works for both ssh and https clones. **Verified against the ParseRepoRef regex/code, not assumed.**

### gh-absent degrade path (CKOUT-04 / RPROJ-04 safety)
If `github.Available()` is false at create-by-repo time, the repo-first path cannot run. Options, in order of preference:
1. **Reject the repo-first create with the existing `msgGHUnavailable` copy** (projects.go:139) — RPROJ-05 already validates with gh, so a no-gh machine simply can't create-by-repo; the **folder path stays fully usable** (RPROJ-04). This is the cleanest and matches the existing PATCH degrade behavior.
2. (Not recommended) a raw `git clone` fallback — loses host gh auth for private/org repos (the whole reason D-02 chose `gh repo clone`), so private repos would silently fail. Skip it.

Recommendation: **option 1**. The clone verb may still internally prefer `gh repo clone` and is only ever reached after a successful gh validation, so an absent-gh create-by-repo is rejected at the validate step, never mid-clone.

## Clone-then-Create Ordering (CKOUT-01 / CKOUT-04 / D-01)

The exact sequence for `projects.create`'s new repo-first branch (sibling to the existing folder branch at projects.go:99):

```
1. ParseRepoRef(input) → owner/name        (HARD-reject syntactic garbage; github.go:40)
2. ValidateRepo(ctx, owner/name)           (RPROJ-05/D-03; github.go:82)
     - syntactic err  → 400 (errInvalidRef)
     - !verified      → 400 msgRepoNotFound / msgGHUnavailable (projects.go:138)
     - verified       → use the gh-canonical owner/name
3. dest = ExpandHome("~/.kangent/repos/") / owner / name
4. If dest already exists on disk → REATTACH path (CKOUT-05, see below). Else:
5. repo_path UNIQUE pre-check on dest       (mirror projects.go:115; 409 "already added")
6. Clone:  github.Clone(ctx, owner/name, dest)
     - on error: os.RemoveAll(dest)         (belt-and-braces; git usually already cleaned up)
                 → surface ONE inline error, NO row created  (CKOUT-04)
7. INSERT ... RETURNING with managed=1 (and github_repo=owner/name if Phase 14 sets it)
     - mirror projects.go:126 INSERT-RETURNING + scanProject
8. 201 Created with the Project
```

**Why this ordering is atomic (D-01):**
- The row is written in step 7, *after* the clone returns exit 0. A clone failure short-circuits at step 6 → no row.
- The directory is removed in step 6's error branch. git already removes its own partial dir on most failures (verified), and `os.RemoveAll(dest)` covers the residual cases (e.g. a non-empty pre-existing dir is left untouched by git but step 4/5 already guard that).
- **Do NOT use a temp-dir-then-rename.** Cloning directly into the final `<owner>/<name>` dest is simpler, and the verified failure modes (no dir on not-found, dir removed on abort) make a staging dir unnecessary. A rename across the `~/.kangent/repos/` tree would also break git's internal absolute paths in `.git/worktrees/*` if any worktree were created before the rename — another reason to clone in place.
- **Concurrency:** the app is single-user; `projects.create` is not heavily concurrent. The existing code already pre-checks `repo_path` uniqueness with a plain SELECT (projects.go:115) and relies on the `repo_path UNIQUE` constraint (00001_init.sql:5) as the real backstop. Two simultaneous create-by-repo for the same owner/name: the first clones+inserts; the second's INSERT hits the UNIQUE constraint and 409s (or its dest-exists check routes it to reattach). No new locking needed.

### Suggested `github.Clone` signature
```go
// Clone runs `gh repo clone <ref> <dest>` (host gh auth → private/org repos).
// Success is exit 0 ONLY (gh/git exit codes are otherwise unreliable). On any
// failure it best-effort removes dest (git usually already did) and returns the
// trimmed stderr. dest must NOT pre-exist non-empty (caller guards via the
// reattach/dedup checks). ref is the gh-canonical owner/name from ValidateRepo.
func Clone(ctx context.Context, ref, dest string) error
```
Placement: `internal/github` (the gh leaf, alongside `ValidateRepo`). Follows the package's arg-array, `exec.CommandContext`, exit-0-only conventions. Add a `Runner`-style seam (mirror `service.go:43` `Config.Runner`) so tests can inject a fake clone for the gh-absent / API-layer tests, while the real-git integration tests use a `file://` bare-origin clone (see Test Strategy).

## Schema Marker (migration 00008, D-06)

**Recommendation: a single boolean-as-INTEGER column `managed`.**

```sql
-- +goose Up
-- v1.4 managed-checkout marker (D-06). Distinguishes Kangent-managed clones
-- (the app cloned and OWNS the directory under ~/.kangent/repos/) from
-- user-pointed folder projects (the directory must NEVER be removed, D-09).
-- INTEGER 0/1 is SQLite's boolean idiom (as 00007's tasks.source default
-- pattern). NOT NULL + DEFAULT 0 is REQUIRED by SQLite ADD COLUMN and is the
-- correct backfill: every EXISTING project predates v1.4 and is folder-based,
-- so 0 (= not managed, never touch its dir) is the safe default for all rows.
ALTER TABLE projects ADD COLUMN managed INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- modernc.org/sqlite tracks SQLite 3.53, which supports DROP COLUMN directly
-- (no table rebuild) — same as 00007's Down.
ALTER TABLE projects DROP COLUMN managed;
```

**Why a boolean, not a `source`/`origin` enum:** the only branch anyone needs is binary — "do I own this dir (gated-remove on delete, pre-task fetch) or not (never touch, D-24 local-only)". An enum (`'managed'|'folder'`) adds a CHECK constraint and string handling for no extra behavior in v1.4. If a future milestone needs more origin kinds (e.g. arbitrary git URLs — explicitly Out of Scope here), it can widen then. Keep it lean.

**Why NOT a path-prefix derivation (`strings.HasPrefix(repo_path, reposBase)`):** fragile and a data-loss hazard. It breaks if the repos base ever changes, if a user symlinks, or if a folder project happens to live under `~/.kangent/repos/`. The Blockers note in STATE.md is explicit: "the schema marker is the single source of truth for 'Kangent owns this dir.' A missing/incorrect marker check is a data-loss risk." A real column is the safe choice.

**Backfill correctness (the highest-risk migration detail):** `ADD COLUMN ... NOT NULL DEFAULT 0` sets **every existing row to 0**. All current projects are folder-based (managed checkouts don't exist until this phase ships), so 0 is exactly right — existing user folders are flagged not-managed and their dirs are never removed (D-09). Verified that this is the only viable shape: SQLite forbids `ADD COLUMN NOT NULL` without a default on a table with rows.

### Wire/scan changes (mirror the 00007 `github_repo` rollout)
- `Project` struct (projects.go:21): add `Managed bool \`json:"managed"\``.
- `projectColumns` (projects.go:62): append `, managed`.
- `scanProject` (projects.go:64): scan into the new field. SQLite returns 0/1; scan into a Go `bool` works via `database/sql` (modernc maps INTEGER 0/1 → bool). If any scan-type friction appears, scan into an `int`/`sql.NullInt64` and set `p.Managed = n != 0` — mirror how `github_repo` used `sql.NullString` at projects.go:65–70.
- The folder `create` path (projects.go:126) keeps inserting with `managed` defaulted to 0 (omit the column → DEFAULT 0). The new repo-first path inserts `managed` = 1 explicitly.

## Pre-task Fetch Insertion (provisionWorktree, CKOUT-02 / D-04 / D-05)

`provisionWorktree` (tasks.go:82) is the single choke point for **both** the task-create and the POST `/worktree` Retry paths, and PR-review worktrees go through the same `repo_path`. The fetch hooks in **immediately before `wt.ResolveBase`** (tasks.go:126):

```
... existing: branch/base settings resolved, CheckRefFormat passes ...
IF project is managed:                         // requires knowing managed-ness here
    defaultBranch := <resolve from the clone>  // see below
    _ = bestEffortFetch(wctx, repoPath, defaultBranch)   // ignore error (D-05)
base, err = wt.ResolveBase(wctx, repoPath)     // unchanged; now sees fresher refs
err = wt.Create(wctx, repoPath, branch, path, base)
```

**Getting `managed` into `provisionWorktree`:** today it receives only `repoPath` (a string). The cleanest change is to extend `loadTaskRepo` (worktrees.go:44) and the task-create caller to also select `projects.managed`, and pass a `managed bool` into `provisionWorktree`. (Alternative: pass the whole project, but a single bool is minimal.)

**What to fetch (verified):**
- `git -C <clone> fetch origin <defaultBranch>` — explicit branch. **Verified:** `fetch origin main` on a `file://` clone → exit 0, updates `origin/main`. This is the precise, cheap shape.
- The default-branch name comes from the clone's `origin/HEAD`: `git -C <clone> symbolic-ref --short refs/remotes/origin/HEAD` → `origin/main` (strip the `origin/`). This is exactly what `ResolveBase` already reads (worktree.go:123), so a small helper or a reuse of that first leg gives the name. **Verified** `origin/HEAD` is set by `gh repo clone` and by `file://` clone.
- Simpler alternative that avoids resolving the name: `git -C <clone> fetch origin` (fetch all). **Verified** exit 0. It updates `origin/<default>` along with everything else. Slightly more network but no name-resolution step. **Recommendation:** prefer the explicit `fetch origin <defaultBranch>` (matches D-04's "the latest default branch" intent and is cheaper), but `fetch origin` is an acceptable fallback if resolving the name is inconvenient.

**Why this is the right seam and why it does NOT regress folder projects (D-24):**
- The fetch runs in the **main clone dir** (`repoPath`), not in a worktree — and only updates remote-tracking refs; `ResolveBase` then prefers the **local** branch tip but falls back to `origin/<name>` as a commit-ish (worktree.go:123–129), so a managed checkout naturally starts off the freshest base while folder projects (no fetch) keep their local-only base.
- The `IF managed` guard means folder projects **never** hit the network — `ResolveBase`'s D-24 "no network, ever" guarantee is preserved byte-for-byte for them. This is the managed-only relaxation D-05 calls for.
- **Best-effort (D-05):** the fetch error is discarded (`_ =`). On offline/auth failure, `ResolveBase` proceeds from the local default-branch tip. Task creation never blocks on a failed fetch — degrade-don't-break.

**Add a scoped-fetch exception note** to `worktree`/`provisionWorktree` like the existing CheckoutPR/FetchRef carve-outs (worktree.go:11–14): the per-new-task default-branch fetch is the **second** deliberate, scoped exception to the never-fetch invariant (the first was PR-head fetch). Worktree.go already exposes `FetchRef(ctx, repo, ref)` (worktree.go:257) — `FetchRef(wctx, repoPath, defaultBranch)` is exactly the primitive; the managed guard + best-effort handling live in `provisionWorktree`, not in `worktree`.

## Gated Delete of a Managed Clone (CKOUT-03 / D-07 / D-08 / D-09)

**Verified facts that drive the sequence:**
1. `git worktree remove` **refuses the main worktree** (clone root): `fatal: '<clone>' is a main working tree`, exit 128. So the clone dir itself is removed with **`os.RemoveAll`, not `git worktree remove`**.
2. `git worktree list --porcelain` from the clone root lists the **main worktree (clone root) first, then each linked worktree** — clean enumeration of everything to gate/remove. (But the app already tracks task worktrees via `tasks.worktree_path`, so it can enumerate from the DB; the porcelain list is a cross-check.)
3. If you `rm -rf` the clone **without** removing its linked worktrees first, the worktrees are **orphaned** — their `.git` file points at the now-deleted common dir (`fatal: not a git repository: <clone>/.git/worktrees/<wt>`). **Therefore: remove linked worktrees FIRST, then the clone dir.**
4. The clone root behaves like any worktree for the gates: `DirtyCount`, `UnpushedCount(origin/<default>..HEAD)`, `StashCount` all work on it (verified). A local-only commit on the clone's default branch gives `origin/main..HEAD == 1` — the unpushed gate catches it.

**Recommended sequence for the managed branch of `projects.delete` (projects.go:269):**

```
1. Load the project; read `managed`.
2. IF NOT managed → today's behavior exactly (DELETE FROM projects; CASCADE tasks;
     dir untouched — PROJ-03/D-09). Done. (No change to the folder path.)
3. Managed → GATE PHASE (compute fresh, mutate NOTHING):
   a. Enumerate the project's task worktrees:
        SELECT id, worktree_path, branch FROM tasks
        WHERE project_id = ? AND worktree_path IS NOT NULL
   b. For EACH worktree path: DirtyCount, UnpushedCount, StashCount, session count
        (reuse the worktreeHandlers gate primitives; for unpushed use the SAME
         FETCH_HEAD pattern the reaper uses for PR worktrees, or origin/<base>
         for task worktrees — see note below).
   c. For the CLONE ROOT itself: DirtyCount(clone), StashCount(clone),
        UnpushedCount(clone, "origin/<default>") — local-only commits on the
        clone's default branch are the D-08 safety net.
   d. Sessions: cleanupSessionCount across the project's tasks (in-memory + live tmux).
   IF ANY gate trips on ANY worktree OR the clone → 409 with a reason LIST
     (mirror the worktree DELETE 409 strings; aggregate "task #N has uncommitted
      changes", "clone has unpushed commits", "sessions running", etc.).
     REMOVE NOTHING. (all-or-nothing, D-07)
4. ALL CLEAR → REMOVE PHASE (order is load-bearing):
   a. For each linked worktree: worktree.Remove(ctx, repoPath=clone, wt, force=false)
        (worktree.go:337 — also kill its sessions/tmux first, exactly as
         CleanupWorktreeGated does; reuse it per task).
   b. os.RemoveAll(clone)   // the main worktree + .git; git worktree remove REFUSES this
   c. DELETE FROM projects WHERE id = ?  (CASCADE removes task rows;
        delete tmux_sessions rows first per FK order if FKs are on — mirror
        reaper.go:335 and tasks.go:543)
5. 204 No Content.
```

**Reuse vs. new code:** the cleanest reuse is to call `CleanupWorktreeGated` (cleanup.go:36) **once per task worktree** — it already does the gate-check + stop-sessions + kill-tmux + `worktree.Remove` + null-columns in the exact proven order. BUT note its current gate scope: cleanup.go only gates **sessions + dirty** inline; the **unpushed + stash gates live in the caller** (the reaper computes them before calling — reaper.go:271–312, and the cleanup.go:32 comment is explicit about this split). So the project gated-delete must **replicate the reaper's pre-gate** (dirty → fetch → unpushed → stash) per worktree before calling `CleanupWorktreeGated`, **plus** add the clone-root gate. This is the same four-gate ladder, applied N+1 times (N worktrees + the clone) with all-or-nothing aggregation.

**Two-pass requirement (all-or-nothing, D-07):** because the delete must remove **nothing** if **anything** is blocked, the gate phase (step 3) and the remove phase (step 4) must be **separate passes** — you cannot interleave gate-and-remove per worktree (that would remove the first clean worktrees before discovering the third is dirty). The reaper interleaves because it's per-PR-independent; the project delete is atomic across the whole project, so gate-all-then-remove-all.

**Unpushed base for task worktrees (`UnpushedCount`):** task worktrees branch off the default branch via `ResolveBase`. The conservative, network-free base for the unpushed gate is `origin/<default>` (the remote-tracking ref) — `rev-list --count origin/<default>..HEAD`. **Verified** this counts local-only commits. (The reaper uses `FETCH_HEAD` after a PR-head fetch because PR worktrees track a PR ref, not the default branch; task worktrees can use `origin/<default>` directly, or do a best-effort `FetchRef` first for freshness — but a fetch is network and may fail; `origin/<default>` already reflects the last-known remote state and is the safe conservative gate. Recommendation: gate on `origin/<default>..HEAD` without a fetch, matching D-24's conservatism for the gate read.)

**HTTP contract:** 409 Conflict with a body listing all blocking reasons (extend the existing single-string 409 to a list/aggregated message). The frontend (Phase 15 / a future cleanup dialog) can render the list; STATE.md's Pending Todo notes the intent to "mirror the worktree CleanupWorktreeDialog variants." Phase 14 only needs the backend contract — a clear, enumerated reason payload.

## Reattach on Existing Dir (CKOUT-05 / D-10 / D-11)

When `dest = ~/.kangent/repos/<owner>/<name>` already exists at create-by-repo time (step 4 of the ordering):

```
1. Is dest a git repo? → git -C dest rev-parse --git-dir  (the validateRepoPath
     check, projects.go:55). If NOT a git repo → ERROR (don't clobber a stray dir).
2. Read origin:  git -C dest remote get-url origin   (the githubOrigin pattern,
     projects.go:254). No origin → ERROR (can't confirm ownership).
3. Canonicalize: github.ParseRepoRef(originURL)  (github.go:40). Handles ssh+https.
4. IF canonical == requested owner/name:
      - REATTACH: treat dest as the managed clone (D-10). No re-clone, no reset (D-11).
      - Best-effort fetch (D-11/D-05): _ = FetchRef(ctx, dest, defaultBranch). Ignore error.
      - Then proceed to the INSERT (managed=1) — UNLESS a project row already
        references this repo_path (the UNIQUE pre-check, projects.go:115):
          → 409 "this repository is already added" (the existing copy).
   ELSE (origin mismatch):
      - ERROR, do NOT clobber the directory (D-10). Surface a clear message
        ("a different repository is already checked out at <dest>").
```

**Edge cases (all covered above):**
- **dest exists but not a git repo** → error (step 1). Never `rm` it (it might be user data).
- **dest exists, right origin, but a project row already references it** → 409 "already added" (dedup; step 4). This is the re-add-the-same-managed-project case.
- **dest exists, wrong origin** → error, no clobber (step 4 else). This is the collision-safety guarantee — the `<owner>/<name>` namespacing makes a true collision rare, but a manually-placed wrong checkout must not be destroyed.

The reattach path **never** sets `managed=0` and never removes anything; it only adopts an existing correct clone. Pair it with the `managed=1` marker so a later delete gates it correctly.

## Service / Package Placement

| Concern | Home | Rationale |
|---------|------|-----------|
| `Clone(ctx, ref, dest) error` | **`internal/github`** | The gh leaf, beside `ValidateRepo`. Follows the package's exit-0-only, arg-array, soft-dependency conventions. Add a `Runner` test seam (mirror service.go:43). |
| Validate→clone→insert orchestration | **`internal/api` projects.go `create`** | Sibling to the existing folder branch (projects.go:99). Reuses `ValidateRepo`/`ParseRepoRef`/dedup/INSERT-RETURNING already there. |
| Pre-task fetch | **`internal/api` tasks.go `provisionWorktree`** | The single choke point (tasks.go:82); gate on `managed`, call `wt.FetchRef`. |
| Gated delete orchestration | **`internal/api` projects.go `delete`** | Sibling to today's delete (projects.go:269); reuse `CleanupWorktreeGated` per worktree + clone-root gate + `os.RemoveAll`. |
| Reattach detection | **`internal/api` projects.go** (small helper) | Reuses `rev-parse --git-dir`, `remote get-url origin`, `ParseRepoRef` — all already used in projects.go. |
| Default-branch name reader (for fetch) | reuse the `origin/HEAD` read in **`worktree.ResolveBase`** leg, or a tiny `worktree` helper | Avoids duplicating the symbolic-ref logic. |

**Wiring in `cmd/kangent/main.go`:** minimal. `Routes(mux, db, wtSvc, mgr, tmuxClient)` (main.go:123) already passes `wt`, `mgr`, `tmuxClient`. Today `projectHandlers` only takes `db` (routes.go:19, projects.go:31). The change: give `projectHandlers` the `wt *worktree.Service`, `mgr *session.Manager`, `tmuxClient tmux.Client` fields (it needs them for the gated delete) and construct it with them in `Routes` (routes.go:19) — exactly how `taskHandlers` is already built one line below (routes.go:20). **No new top-level service construction in main.go is required.** The `github.Clone` verb is a package function (like `ValidateRepo`), so it needs no service instance either.

## Test Strategy

The codebase's proven pattern: **real git, no mocks** (worktree_test.go:3), with a `file://` bare-origin + clone fixture, and a `Runner`/`Config` seam for the gh-dependent surface.

| What to test | How | Existing pattern to copy |
|--------------|-----|--------------------------|
| `github.Clone` success/failure | Inject a fake `Runner` that returns canned (success/err); for a real-git variant, clone a `file://` bare origin | service.go:43 `Config.Runner`; worktree_test.go:73 `cloneRepo` |
| clone-then-create atomicity (no row on clone fail) | API test: stub `Clone` to fail → assert 0 rows + no dir | projects_test.go newTestServer + a `Clone` seam |
| migration 00008 backfill | open DB, migrate, assert existing folder projects have `managed=0`; new repo project `managed=1` | pullrequests_test.go store.Open/Migrate |
| pre-task fetch managed-only | real `file://` clone as the project repo_path, set managed=1, push a new commit to the bare origin, create a task → assert the worktree base reflects the fetched tip; folder project (managed=0) does NOT fetch | worktree_test.go fixtures + provisionWorktree |
| gated delete — each gate trips | per-worktree dirty / unpushed (`origin/main..HEAD`) / stash / session, plus clone-root unpushed → assert 409 + nothing removed | reaper/pr_reconcile_test.go gate tests (the exact mirror) |
| gated delete — all clean | assert linked worktrees removed, clone dir gone (`os.RemoveAll`), rows deleted | reaper.go:319 + os.Stat assertions |
| reattach — origin match / mismatch / non-git dir / already-added | pre-create dest with matching/mismatching origin → assert reuse / error / 409 | projects_test.go githubOrigin + ParseRepoRef |

**gh-absent / unauthenticated tests** use the `Runner` seam (no live gh needed). The **integration** confidence comes from the host-verified `gh repo clone` evidence in this doc; CI need not have gh installed because the `Clone` seam is faked in API tests and the git mechanics are tested via `file://` clones.

## Common Pitfalls

### Pitfall 1: Removing the clone dir while linked worktrees still exist (data-corruption)
**What goes wrong:** `rm -rf <clone>` while task worktrees are still registered orphans them — their `.git` file points at the deleted common dir, breaking them silently.
**Why it happens:** the clone root and its worktrees share one `.git` common dir.
**How to avoid:** in the gated-delete REMOVE phase, `worktree.Remove` **every linked worktree first**, then `os.RemoveAll(clone)`. **Verified** ordering above. Owner: `projects.delete` managed branch.
**Warning signs:** a leftover `~/.kangent/worktrees/<repo>/<slug>-<id>` whose `git status` errors with "not a git repository".

### Pitfall 2: Trying to `git worktree remove` the clone root
**What goes wrong:** exit 128 `is a main working tree` — the gated delete would error out instead of removing the clone.
**How to avoid:** remove the clone **directory** with `os.RemoveAll`, never `git worktree remove`. `worktree.Remove` is only for the LINKED worktrees. Owner: `projects.delete`.

### Pitfall 3: Marker backfill wrong → deleting a user's folder (data loss — STATE.md's highest-severity item)
**What goes wrong:** if the marker defaults to "managed" or is path-derived, the gated delete could `os.RemoveAll` a user's working checkout.
**How to avoid:** `managed INTEGER NOT NULL DEFAULT 0` — every pre-v1.4 row backfills to **not-managed** (folder, never touch). The `managed` column is the single source of truth; never derive ownership from the path prefix. Owner: migration 00008 + every code path that decides to remove a dir.

### Pitfall 4: Half-created project on clone failure (CKOUT-04)
**What goes wrong:** a row written before the clone, or a partial dir left after a failure.
**How to avoid:** INSERT only after `Clone` returns exit 0; `os.RemoveAll(dest)` on the failure branch. git already removes its own partial dir (verified) and refuses a non-empty dest (verified), but the explicit cleanup covers the residue. Owner: `projects.create` repo-first branch.

### Pitfall 5: Pre-task fetch regressing folder projects' D-24 "no network" guarantee
**What goes wrong:** an unconditional fetch in `provisionWorktree` would make folder projects hit the network — violating the locked D-24 invariant and slowing/breaking offline folder work.
**How to avoid:** guard the fetch with `if managed`. Folder projects take the exact same `ResolveBase`-only path as today. Owner: `provisionWorktree`.

### Pitfall 6: Blocking the create request on a large clone (sync clone UX)
**What goes wrong:** D-01 is synchronous; a huge repo blocks the POST `/api/projects` request for the full clone duration.
**Why it's accepted for Phase 14:** D-01 explicitly defers async/progress (CKUX-01) and full clone (shallow is CKMNT-03). Phase 14's contract is the blocking primitive.
**How to handle:** use a generous (or no) timeout on the clone `exec.CommandContext` — unlike worktree ops (30s, tasks.go:117), a clone of a large repo legitimately takes minutes. Do NOT impose the 30s worktree timeout here. Surface a single inline error if the underlying network truly fails. Note for Phase 15: this is where live progress (CKUX-01) would eventually attach. Owner: `github.Clone` / `projects.create`.

### Pitfall 7: ssh-vs-https origin form breaking reattach
**What goes wrong:** assuming `origin` is always `https://github.com/...` when this host clones via ssh (`git@github.com:...`).
**How to avoid:** always canonicalize with `github.ParseRepoRef` (it handles both forms — github.go:46–53, verified). Compare canonical owner/name, never raw URLs. Owner: reattach path.

### Pitfall 8: All-or-nothing violated by interleaving gate+remove
**What goes wrong:** removing the first clean worktrees before discovering a later one is dirty leaves the project half-deleted.
**How to avoid:** two passes — gate ALL (worktrees + clone), then remove ALL only if every gate is clean. Owner: `projects.delete`.

## Runtime State Inventory

> This is a brownfield phase that ADDS a managed-checkout capability; it does not rename/migrate existing runtime state. The relevant "state" inventory:

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | `projects` table — gains one `managed` column (migration 00008). Existing rows backfill to `managed=0` (folder). No existing data is reinterpreted; no `repo_path` values change. | Migration 00008 (code + schema). No data migration of existing rows beyond the DEFAULT 0 backfill. |
| Live service config | None — the app spawns `gh`/`git` per-call; no daemon config holds the marker. New managed clones live under `~/.kangent/repos/<owner>/<name>` (a new on-disk location, sibling of the existing `~/.kangent/worktrees/`). | None. |
| OS-registered state | None — no scheduler/launchd/systemd registrations reference any renamed string. | None. |
| Secrets/env vars | None changed. `gh` reads host auth (keyring) — unchanged. No new env vars. | None. |
| Build artifacts | None — pure Go source + one new migration file embedded via the existing `embed.FS`. No package rename, no egg-info/binary-name change. | None. |

**Canonical question answered:** after the code ships, the only persisted new state is the `managed` column on `projects` and the on-disk `~/.kangent/repos/` clones. Existing folder projects are untouched (managed=0). Nothing else caches or registers a renamed string.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|-------------|-----------|---------|----------|
| `gh` CLI | RPROJ-05 validate + CKOUT-01 `gh repo clone` | ✓ (this host) | 2.82.0 | Reject repo-first create with `msgGHUnavailable`; folder path stays usable (RPROJ-04). Soft dependency per github.go. |
| `gh` auth | private/org clone | ✓ (account jordeu, ssh) | — | `ValidateRepo` already surfaces not-verified → 400 before clone. |
| `git` CLI | clone/worktree/gates | ✓ | 2.43.0 | None — git is the app's premise; guaranteed present (CLAUDE.md). |

**Missing dependencies with no fallback:** none on this host.
**Missing dependencies with fallback:** `gh` absence on *other* machines — handled by the degrade path (validate fails → repo-first create rejected → folder path unaffected). This is a real cross-machine concern even though `gh` is present here.

## State of the Art

| Old (pre-Phase-14) | New (Phase 14) | Impact |
|--------------------|----------------|--------|
| Projects are folder-only; `repo_path` always user-pointed | Two kinds: folder (managed=0) and managed clone (managed=1) | Delete/fetch branch on the marker; folder path byte-for-byte unchanged |
| `projects.delete` never touches disk (PROJ-03) | Folder delete still never touches disk; managed delete gated-removes the clone + worktrees | The gated path is NEW; folder path preserved (D-09) |
| `provisionWorktree` never fetches (D-24) | Managed projects fetch latest default branch best-effort before worktree create | Second scoped fetch exception; folder projects keep D-24 |
| `projectHandlers struct{ db *sql.DB }` | gains `wt`, `mgr`, `tmuxClient` (for gated delete) | Construction mirrors `taskHandlers` (routes.go:20) |

**Deprecated/outdated:** none. The phase brief's claim that `gh` is absent on this host is **incorrect** — `gh` 2.82.0 is installed and authenticated; treat all `gh repo clone` contracts in this doc as host-verified.

## Open Questions

1. **Does Phase 14's primitive set `github_repo` on the managed project, or Phase 15?**
   - What we know: D-discretion says a managed project's `github_repo` is the cloned `owner/name`, and Phase 15's form relies on it.
   - Recommendation: Phase 14 sets `github_repo = <canonical owner/name>` at create-by-repo INSERT time (it already has the canonical ref from `ValidateRepo`, and the column + verified-update path exist from 00007). One fewer thing for Phase 15 to wire. Low risk — it's just persisting a value the primitive already holds.

2. **Unpushed gate base for *task* worktrees during gated delete: `origin/<default>` (no fetch) vs. fetch-then-`FETCH_HEAD`?**
   - What we know: the reaper uses `FETCH_HEAD` for *PR* worktrees (they track a PR ref); task worktrees branch off the default branch.
   - What's unclear: whether to fetch before the gate (freshest, but network + can fail) or read `origin/<default>` (network-free, conservative).
   - Recommendation: gate on `origin/<default>..HEAD` **without** a fetch — conservative (may over-count if origin moved, which only makes the gate *safer*), network-free (consistent with the delete being a local operation), and verified to catch local-only commits. Let the planner confirm.

3. **409 reason payload shape — single aggregated string vs. structured list?**
   - What we know: the worktree DELETE returns a single 409 string; STATE.md wants to mirror the CleanupWorktreeDialog variants in a future UI.
   - Recommendation: return a structured list of `{kind, target}` reasons (or at minimum a multi-line aggregated string) so Phase 15's UI can enumerate blockers. Backend-only for Phase 14; the exact JSON shape is a small planner decision.

## Sources

### Primary (HIGH confidence — host-verified)
- `gh repo clone --help` + live clones on this host (gh 2.82.0) — destination handling, parent auto-create, non-empty-dest refusal, repo-not-found (no dir), default-branch auto-checkout, `origin/HEAD` set, ssh origin form, `--` passthrough, exit codes.
- `gh help exit-codes` — 0/1/2/4 semantics.
- Live git 2.43.0 experiments — `git worktree remove` refuses main worktree (128), `rm -rf` clone orphans linked worktrees, `worktree list --porcelain` enumeration, `DirtyCount`/`UnpushedCount(origin/main..HEAD)`/`StashCount` on a clone root, `fetch origin <branch>`/`fetch origin`, SIGINT-mid-clone removes partial dir.
- Codebase reads (file:line cited throughout): `internal/api/projects.go`, `internal/github/github.go`, `internal/github/service.go`, `internal/worktree/worktree.go`, `internal/api/worktrees.go`, `internal/api/cleanup.go`, `internal/api/tasks.go`, `internal/reaper/reaper.go`, `internal/store/migrations/00001_init.sql`, `internal/store/migrations/00007_github_foundations.sql`, `internal/settings/settings.go`, `internal/api/routes.go`, `cmd/kangent/main.go`, `internal/worktree/worktree_test.go`, `internal/api/projects_test.go`.

### Planning docs (HIGH confidence)
- `.planning/phases/14-managed-checkout-foundations/14-CONTEXT.md` — D-01..D-11, discretion, deferred.
- `.planning/REQUIREMENTS.md` — CKOUT-01/02/03/04/05, RPROJ-04/05, Future, Out of Scope.
- `.planning/ROADMAP.md` §Phase 14 — goal, success criteria, forced 14→15 order.
- `.planning/STATE.md` — v1.4 standing decisions, Blockers (data-loss risk on marker), Pending Todos (schema-marker shape, reattach origin check, gated-delete UI).
- `./CLAUDE.md` — stack, shell-out conventions, modernc/SQLite migration style.

## Metadata

**Confidence breakdown:**
- gh repo clone contract: **HIGH** — host-verified, not documented-only (gh present, contradicting the brief).
- Clone-then-create atomicity: **HIGH** — failure modes empirically verified (no dir on not-found, dir removed on abort, clean refusal on non-empty dest).
- Migration 00008 shape: **HIGH** — mirrors 00007's verified pattern; SQLite ADD COLUMN NOT NULL DEFAULT semantics confirmed.
- Gated delete sequence: **HIGH** — worktree-remove-refuses-main, orphan-on-rm, gate primitives on clone root all empirically verified; reuses the reaper's proven 4-gate ladder.
- Pre-task fetch: **HIGH** — fetch shapes verified; insertion point is the single existing choke point.
- Reattach: **HIGH** — reuses verified `ParseRepoRef` (ssh+https) and existing origin-read pattern.

**Research date:** 2026-06-14
**Valid until:** ~2026-07-14 (stable — stdlib + host CLIs; re-verify only if `gh`/git are upgraded materially).
