# Phase 14: Managed Checkout Foundations - Context

**Gathered:** 2026-06-14
**Status:** Ready for planning

<domain>
## Phase Boundary

The **backend capability** for Kangent-managed checkouts: given a `gh`-validated GitHub `owner/name`, clone it into a Kangent-owned `~/.kangent/repos/<owner>/<name>` checkout on the repo's default branch, mark the project as managed (so the app knows it owns the directory), use that clone as the repo root for task/PR-review worktrees with a best-effort latest-default-branch fetch first, reattach to an already-cloned directory, and gated-remove the clone on project delete — while never touching user-pointed folder projects.

Covers **CKOUT-01, CKOUT-02, CKOUT-03, CKOUT-05, RPROJ-05**.

**NOT in this phase:** the repo-first "Add project" UI / form, name/link/description auto-fill, and the inline clone-failure surfacing — that is Phase 15 (Repo-First Creation Flow). Phase 14 exposes the primitive(s) and persistence/cleanup paths Phase 15 drives.
</domain>

<decisions>
## Implementation Decisions

### Provisioning model
- **D-01:** Cloning is **synchronous clone-then-create** — the clone runs first and the project row is created only after the clone succeeds. This is atomic: a failed clone leaves **no project row and no directory**, which directly satisfies CKOUT-04 (no half-created project). The create call blocks until the checkout is ready. (Async/background provisioning with a "provisioning" state and live progress is explicitly deferred — see CKUX-01 in REQUIREMENTS.md Future.)
- **D-02:** Clone via `gh repo clone <owner>/<name> <dest>` (host gh auth → private/org repos work) into `~/.kangent/repos/<owner>/<name>`. `gh repo clone` / git checks out the remote's default branch (main/master) automatically — no separate default-branch detection is needed for the initial checkout. Full clone (shallow/partial is deferred CKMNT-03).
- **D-03:** Repo input is gh-validated **before** any clone (RPROJ-05) by reusing the existing `github.ValidateRepo` (gh-verified) + `github.ParseRepoRef` canonicalization. An invalid/inaccessible repo is rejected with no clone attempted and no project created.

### Freshness (CKOUT-02)
- **D-04:** Before creating each new task (and PR-review) worktree on a **managed** checkout, fetch the latest default branch from origin so new work starts from latest.
- **D-05:** **Fetch is best-effort.** If the fetch fails (offline/auth), **proceed from the local default-branch base** rather than blocking task creation — degrade-don't-break. The fetch is a freshness convenience, not a hard gate. (This is the managed-checkout-only relaxation of `worktree.ResolveBase`'s D-24 "local reads only, no network ever", which stays exactly as-is for folder-based projects.)

### Managed vs folder marker
- **D-06:** A new schema marker (migration **00008**, next after v1.3's 00007) distinguishes Kangent-managed checkouts from user-pointed folders, so delete/cleanup knows which directories the app owns. The exact column shape (e.g. a boolean `managed` flag vs a typed `source`/`origin` column) is the planner's call — the contract is: the app can reliably tell "I cloned and own this dir" from "the user pointed me at their existing checkout."

### Gated project delete (CKOUT-03)
- **D-07:** Deleting a managed-checkout project is **gated, all-or-nothing**. It must clean up the project's task worktrees AND the managed clone. If **anything** is blocked — any task worktree or the clone is dirty (uncommitted), has unpushed commits, has a stash, or has a running session — the delete is **refused with a clear reason listing what's blocking**, and **nothing is removed** until everything is clean + idle. Reuses the existing `CleanupWorktreeGated` gate set (dirty / unpushed / stash / session).
- **D-08:** Rationale for all-or-nothing: removing the managed clone directory discards its local branches, so the **unpushed gate is the safety net** that prevents losing local-only commits. (D-34 "never run `git branch -D`" still holds for worktree removal; wholesale clone-dir removal only happens once the unpushed/dirty/stash/session gates all pass.)
- **D-09:** Deleting a **folder-based (user-pointed)** project keeps today's behavior exactly — the directory on disk is never touched (current `projects.delete` "PROJ-03"). The gated-removal path applies only to managed checkouts.

### Reattach on existing dir (CKOUT-05)
- **D-10:** When adding a repo whose managed dir (`~/.kangent/repos/<owner>/<name>`) already exists on disk: if it is a git repo whose `origin` canonicalizes (via `github.ParseRepoRef`) to the **same** `owner/name`, **reattach/reuse** it (no re-clone). If `origin` does **not** match, **error** rather than clobber the directory.
- **D-11:** Reattach does NOT force a re-clone or destructive reset; a freshness fetch on reattach is acceptable but the same best-effort rule as D-05 applies (never block on a failed fetch).

### Claude's Discretion
- **Repos base location:** hardcode `~/.kangent/repos/` for v1.4 (no new user setting), keeping the surface lean. The planner MAY mirror the existing `worktree_base` settings pattern with a `repos_base` setting if it's trivial and consistent — but it is not required, and is not a user-facing requirement for this milestone.
- **Schema marker column shape** (D-06) — planner chooses the concrete representation.
- **Where the clone primitive lives** — natural home is `internal/github` (the gh leaf) for the `gh repo clone` verb and/or `internal/worktree`/a small provisioning helper for the repo-root + reattach + gated-delete orchestration; planner decides the package split.
- **`github_repo` link on a managed project** — a managed project's `github_repo` is the cloned `owner/name`; setting it as part of create-by-repo is natural (the Phase 15 form will rely on it), planner decides whether Phase 14's primitive sets it or Phase 15 does.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Milestone scope & locked decisions
- `.planning/PROJECT.md` — "Current Milestone: v1.4 Repo-First Projects" (target features + Settled decisions) and the v1.4 Key Decisions rows
- `.planning/REQUIREMENTS.md` — RPROJ-05 + CKOUT-01/02/03/05 (this phase's requirements), plus Future (CKMNT-01/02/03, CKUX-01) and Out of Scope rows
- `.planning/ROADMAP.md` §"Phase 14: Managed Checkout Foundations" — goal, depends-on, success criteria, the forced 14→15 build-order note
- `.planning/STATE.md` §"Accumulated Context → Decisions" — the v1.4 milestone-time decisions captured before roadmapping

### Existing code to reuse / extend (verified during scout)
- `internal/api/projects.go` — `create` (name + repo_path, `validateRepoPath`, dedup on `repo_path`, INSERT), `delete` (today never touches disk — "PROJ-03"), `Project` wire shape, `update`'s use of `github.ValidateRepo`/`Available`/`msgRepoNotFound`/`msgGHUnavailable`
- `internal/github/github.go` — `ParseRepoRef`, `ValidateRepo(ctx, ref) (canonical, verified, err)`, `Available()` (no clone/default-branch reader exists yet — Phase 14 adds the clone verb)
- `internal/worktree/worktree.go` — `ResolveBase` (D-24 local-only base resolution, origin/HEAD chain), `Create` (worktree add off a base), `PathUnder` (worktree placement), `DirtyCount`/`UnpushedCount`/`StashCount` gate primitives, `Remove` (worktree remove + prune, never deletes branches)
- `internal/api/cleanup.go` — `CleanupWorktreeGated` (the shared gated helper to reuse for delete; dirty/unpushed/stash/session gates, force=false)
- `internal/api/worktrees.go` — `loadTaskRepo` (joins task → project `repo_path`), `provisionWorktree` (where the pre-task fetch (D-04/D-05) hooks in)
- `internal/store/migrations/00007_github_foundations.sql` — the migration immediately before 00008; mirror its up/down + modernc/SQLite column-add style
- `internal/settings/settings.go` — `KeyWorktreeBase` = `~/.kangent/worktrees/` (the `~`-expanded-at-use settings pattern to mirror if a `repos_base` setting is added)

No external specs/ADRs beyond the `.planning/` docs above — requirements and decisions are fully captured here and in REQUIREMENTS/ROADMAP/PROJECT.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`github.ValidateRepo` / `ParseRepoRef` / `Available`** — RPROJ-05 validation and origin-matching for reattach (D-03, D-10) reuse these directly; no new gh-verification logic.
- **`CleanupWorktreeGated` + `DirtyCount`/`UnpushedCount`/`StashCount`** — CKOUT-03's gated delete (D-07) reuses the exact gate set already proven in v1.3.
- **`worktree.ResolveBase` / `Create` / `PathUnder`** — managed-checkout worktrees branch off the clone exactly as folder projects do today; only a pre-fetch (D-04) is added in front.
- **`projects.create` / `validateRepoPath` / `Project` wire shape** — the create-by-repo path is a sibling to the existing create-by-folder path; the `Project` struct gains the managed marker.
- **migration 00007** — template for the 00008 managed-marker migration (up/down, NOT NULL DEFAULT pattern).

### Established Patterns
- **Shell out to git/gh with arg arrays, `cmd.Dir`/`-C <repo>`, never `sh -c`** (projects.go, worktree.go) — the clone verb follows this.
- **`~`-expanded-at-use settings** (`worktree_base`) — any `repos_base` mirrors it (stored raw, expanded at use).
- **Degrade-don't-break / always-usable** — clone failure (D-01), fetch failure (D-05), and gh-absence all surface without breaking the app.
- **`repo_path` is the single repo-root field** consumed everywhere (worktrees, diff, github-origin) — a managed checkout simply sets `repo_path` to its `~/.kangent/repos/<owner>/<name>` clone, so all downstream worktree/diff code works unchanged.

### Integration Points
- `projects.create` (new repo-first branch + clone) and `projects.delete` (new managed gated-removal branch).
- `provisionWorktree` in `internal/api/worktrees.go` (pre-task fetch insertion for managed checkouts).
- New migration `00008_*` in `internal/store/migrations/`.
- New clone verb in `internal/github` (and/or a small provisioning helper) — planner decides the split.
</code_context>

<specifics>
## Specific Ideas

- Clone destination layout is fixed by the user: `~/.kangent/repos/<owner>/<name>` (owner/name-namespaced, collision-safe), a sibling of the existing `~/.kangent/worktrees/` base.
- "Reuse if origin matches" means: the existing dir's `origin` remote, canonicalized via `github.ParseRepoRef`, equals the requested `owner/name` (D-10).
- The managed clone IS the project's `repo_path` — there is no separate "primary checkout" concept; everything that reads `repo_path` today (worktree provisioning, diff base, github-origin prefill) keeps working.
</specifics>

<deferred>
## Deferred Ideas

- **On-demand "sync"** of a managed checkout's default branch from the project view (beyond the per-task fetch) — CKMNT-01 (REQUIREMENTS.md Future).
- **Non-default base branch** at project creation — CKMNT-02.
- **Shallow / partial clone** for large repos — CKMNT-03.
- **Live clone-progress streaming + cancel** — CKUX-01 (the async-provisioning alternative rejected in D-01).
- **A `repos_base` user setting** — only if trivially consistent with `worktree_base`; otherwise hardcoded `~/.kangent/repos/` (Claude's discretion above).
- The **repo-first Add-project UI**, name/link/description auto-fill, and inline clone-failure surfacing — Phase 15, not deferred (next phase).

None of the above were in-scope for Phase 14; discussion stayed within the managed-checkout backend boundary.
</deferred>

---

*Phase: 14-managed-checkout-foundations*
*Context gathered: 2026-06-14*
