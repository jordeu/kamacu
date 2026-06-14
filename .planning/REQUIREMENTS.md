# Requirements: Kangent — v1.4 Repo-First Projects

**Defined:** 2026-06-14
**Core Value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.

## v1.4 Requirements

Requirements for the Repo-First Projects (Kangent-Managed Checkouts) milestone. Each maps to exactly one roadmap phase.

### Repo-First Project Creation

- [ ] **RPROJ-01**: When GitHub integration is on, the "Add project" flow defaults to repo-first — the user enters a GitHub repo (`owner/name`) instead of picking a local folder.
- [ ] **RPROJ-02**: Entering a valid repo auto-derives the project name (prefilled from the repo name, editable before create) and auto-fills the GitHub link and description.
- [ ] **RPROJ-03**: The user can still create a project from a local folder when GitHub is on — the folder path is the optional alternative, not the default.
- [ ] **RPROJ-04**: When GitHub integration is off, project creation is folder-only (exactly as before v1.4) and no repo-first UI appears.
- [ ] **RPROJ-05**: The repo is validated via `gh` before any clone is attempted; an invalid or inaccessible repo is rejected inline without creating a project.

### Managed Checkout Lifecycle

- [x] **CKOUT-01**: Creating a project from a repo `gh repo clone`s it into `~/.kangent/repos/<owner>/<name>` and checks out the repo's auto-detected default branch (main/master); that managed clone becomes the project's repo root for all worktree operations.
- [ ] **CKOUT-02**: Task and PR-review worktrees branch off the managed checkout; before creating a new task's worktree, Kangent fetches the latest default branch so new work starts from latest.
- [ ] **CKOUT-03**: Deleting a managed-checkout project removes the managed clone from disk, gated on the same conditions as worktree cleanup (uncommitted/unpushed work or running sessions block silent removal); folder-based (user-pointed) project directories are never removed.
- [ ] **CKOUT-04**: Provisioning degrades-don't-break — a clone failure (auth, network, or missing repo) surfaces inline and leaves no half-created project (no orphan DB row, no partial directory).
- [ ] **CKOUT-05**: Re-adding a repo whose managed directory already exists on disk reattaches to / reuses the existing checkout rather than failing or cloning over it.

## Future Requirements

Acknowledged but deferred — not in the v1.4 roadmap.

### Managed Checkout Maintenance

- **CKMNT-01**: On-demand "sync" of a managed checkout's default branch from the project view (beyond the per-new-task fetch).
- **CKMNT-02**: Choose a non-default base branch when creating a project from a repo.
- **CKMNT-03**: Shallow / partial clone option for large repositories.

### Provisioning UX

- **CKUX-01**: Live clone-progress streaming with the ability to cancel an in-progress clone.

## Out of Scope

Explicitly excluded from v1.4. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Non-GitHub forges / arbitrary git-URL clones | GitHub-only for the repo path (consistent with v1.3); the local-folder option is the non-GitHub escape hatch |
| Migrating existing folder-based projects into managed checkouts | Existing projects stay exactly as they are; v1.4 only changes the *new*-project path |
| Scheduled / background auto-pull of managed checkouts | Freshness is per-new-task-worktree only; continuous syncing is out of scope |
| Remote actions on project delete (deleting the GitHub repo, pushing, etc.) | Cleanup is local-directory-only; the app never mutates the remote |
| Multiple checkouts per repo / monorepo sub-path checkouts | One managed checkout per project; sub-path/monorepo handling is out of scope |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| RPROJ-01 | Phase 15 | Pending |
| RPROJ-02 | Phase 15 | Pending |
| RPROJ-03 | Phase 15 | Pending |
| RPROJ-04 | Phase 15 | Pending |
| RPROJ-05 | Phase 14 | Pending |
| CKOUT-01 | Phase 14 | Complete |
| CKOUT-02 | Phase 14 | Pending |
| CKOUT-03 | Phase 14 | Pending |
| CKOUT-04 | Phase 15 | Pending |
| CKOUT-05 | Phase 14 | Pending |

**Coverage:**
- v1.4 requirements: 10 total
- Mapped to phases: 10 (Phase 14: CKOUT-01, RPROJ-05, CKOUT-02, CKOUT-05, CKOUT-03 — Phase 15: RPROJ-01, RPROJ-02, RPROJ-03, RPROJ-04, CKOUT-04)
- Unmapped: 0

---
*Requirements defined: 2026-06-14*
*Roadmapped: 2026-06-14 — 2 phases (14–15), granularity: coarse*
