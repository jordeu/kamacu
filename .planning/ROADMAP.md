# Roadmap: Kangent

## Milestones

- ✅ **v1.0 MVP** — Phases 1–5 (shipped 2026-06-11) — see [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md)
- ✅ **v1.1 Settings & Polish** — Phase 6 (shipped 2026-06-11) — see [milestones/v1.1-ROADMAP.md](milestones/v1.1-ROADMAP.md)
- ✅ **v1.2 Quota & Resumable Shells** — Phases 7–9 (shipped 2026-06-13) — see [milestones/v1.2-ROADMAP.md](milestones/v1.2-ROADMAP.md)
- ✅ **v1.3 GitHub PR Review** — Phases 10–13 (shipped 2026-06-14) — see [milestones/v1.3-ROADMAP.md](milestones/v1.3-ROADMAP.md)
- 🚧 **v1.4 Repo-First Projects** — Phases 14–15 (active, started 2026-06-14)

## Phases

<details>
<summary>✅ v1.0 MVP (Phases 1–5) — SHIPPED 2026-06-11</summary>

- [x] Phase 1: Foundation — Projects & Board (7/7 plans) — completed 2026-06-10
- [x] Phase 2: Terminal Engine (5/5 plans) — completed 2026-06-10
- [x] Phase 3: Worktree Isolation & Bash Tabs (6/6 plans) — completed 2026-06-10
- [x] Phase 4: Claude Code Agent Sessions (5/5 plans) — completed 2026-06-11
- [x] Phase 5: Recovery & Review (5/5 plans) — completed 2026-06-11

Full details: [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md)

</details>

<details>
<summary>✅ v1.1 Settings & Polish (Phase 6) — SHIPPED 2026-06-11</summary>

- [x] Phase 6: Settings & Polish (4/4 plans) — completed 2026-06-11

Full details: [milestones/v1.1-ROADMAP.md](milestones/v1.1-ROADMAP.md)

</details>

<details>
<summary>✅ v1.2 Quota & Resumable Shells (Phases 7–9) — SHIPPED 2026-06-13</summary>

- [x] Phase 7: Claude Quota Indicator (3/3 plans) — completed 2026-06-12
- [x] Phase 8: tmux Shells — Spawn & Detach Lifecycle (4/4 plans) — completed 2026-06-13
- [x] Phase 9: tmux Restart Resume & Cleanup Integration (5/5 plans) — completed 2026-06-13

Full details: [milestones/v1.2-ROADMAP.md](milestones/v1.2-ROADMAP.md)

</details>

<details>
<summary>✅ v1.3 GitHub PR Review (Phases 10–13) — SHIPPED 2026-06-14</summary>

- [x] Phase 10: GitHub Foundations (5/5 plans) — completed 2026-06-13
- [x] Phase 11: PR Review Column (4/4 plans) — completed 2026-06-14
- [x] Phase 12: Open-a-Review (7/7 plans) — completed 2026-06-14
- [x] Phase 13: PR Worktree Auto-Cleanup (3/3 plans) — completed 2026-06-14

Full details: [milestones/v1.3-ROADMAP.md](milestones/v1.3-ROADMAP.md)

</details>

### 🚧 v1.4 Repo-First Projects (Phases 14–15)

- [x] **Phase 14: Managed Checkout Foundations** — The `gh`-validated clone/provisioning primitive, a schema marker distinguishing Kangent-managed checkouts from user-pointed folders, branch-fresh task worktrees off the managed clone, reattach-on-existing, and gated removal on project delete. (4 plans, planned 2026-06-14) (completed 2026-06-14)
- [ ] **Phase 15: Repo-First Creation Flow** — The "Add project" surface defaults to entering a GitHub `owner/name` when integration is on (folder optional; folder-only when off), auto-derives name/link/description, and surfaces clone failures inline with no half-created project.

## Phase Details

> **Build order is forced by dependencies (14 → 15).** Phase 14 builds the whole managed-checkout *capability* in `internal/...` + the persistence/cleanup paths (the layer that task/PR-review worktrees and project-delete already operate through), reusing v1.3's `internal/github` (gh auth, `ParseRepoRef`/`ValidateRepo`, `Available`, the project `github_repo` link) and the existing `internal/worktree` + `CleanupWorktreeGated` gating (dirty / unpushed / stash / running-session). Phase 15 puts the repo-first **creation flow** on top of that capability — it can only surface inline clone failures and reattach behavior once the provisioning primitive (CKOUT-01/04/05) exists. Do not reorder. A schema marker requires a migration (next number after v1.3's 00007 → **00008**), landed in Phase 14 so Phase 15 needs no further migration.

### Phase 14: Managed Checkout Foundations
**Goal**: Kangent can clone a `gh`-validated GitHub repo into a managed, owner/name-namespaced checkout that it knows it owns, use that clone as the repo root for fresh-off-default-branch task worktrees, reattach to an already-cloned directory, and gated-remove the clone on project delete — while never touching user-pointed folder projects.
**Depends on**: Phase 13 (v1.3 complete) — builds on `internal/github` (`ParseRepoRef`/`ValidateRepo`/`Available`, the `github_repo` link), `internal/worktree` provisioning, and the shared `CleanupWorktreeGated` gates. The first phase of this milestone.
**Requirements**: CKOUT-01, RPROJ-05, CKOUT-02, CKOUT-05, CKOUT-03
**Success Criteria** (what must be TRUE):
  1. Given a valid GitHub `owner/name`, Kangent validates it via `gh` and clones it into `~/.kangent/repos/<owner>/<name>` checked out on the repo's auto-detected default branch (main/master); an invalid or inaccessible repo is rejected before any clone is attempted, and the managed clone is recorded as the project's repo root with a schema marker that flags it as Kangent-managed (CKOUT-01, RPROJ-05).
  2. A task (or PR-review) worktree created for a managed-checkout project branches off that managed clone, and Kangent fetches the latest default branch first so new work starts from latest — existing folder-based projects' worktree behavior is unchanged (CKOUT-02).
  3. Re-provisioning a managed checkout whose directory already exists on disk reattaches to / reuses the existing clone rather than failing or cloning over it (CKOUT-05).
  4. Deleting a managed-checkout project removes the managed clone from disk, gated on the same conditions as worktree cleanup (uncommitted/unpushed work or a running session blocks silent removal); deleting a folder-based (user-pointed) project never removes its directory (CKOUT-03).
**Plans**: 4 plans
Plans:
- [x] 14-01-PLAN.md — Migration 00008 managed marker, Project wire/scan, github.Clone verb, projectHandlers wiring
- [x] 14-02-PLAN.md — Repo-first create: validate→clone→insert atomicity + reattach-on-existing
- [x] 14-03-PLAN.md — Best-effort default-branch fetch before managed task worktrees (folder projects unchanged)
- [x] 14-04-PLAN.md — Gated all-or-nothing managed-project delete (worktrees + clone); folder delete untouched

### Phase 15: Repo-First Creation Flow
**Goal**: When GitHub integration is on, a user adds a project by naming a GitHub repo — name/link/description auto-fill from the repo and a folder path is the optional alternative — and a failed clone surfaces inline leaving no half-created project; when integration is off the Add-project flow is folder-only, exactly as before v1.4.
**Depends on**: Phase 14 — consumes the validated clone/provisioning primitive (CKOUT-01), its inline-failure / no-orphan guarantees (CKOUT-04), and reattach-on-existing (CKOUT-05) to drive the create flow. Backward compatibility with folder-only creation (RPROJ-04) is verified against the unchanged folder path.
**Requirements**: RPROJ-01, RPROJ-02, RPROJ-03, RPROJ-04, CKOUT-04
**Success Criteria** (what must be TRUE):
  1. With GitHub integration on, the "Add project" flow defaults to entering a GitHub repo (`owner/name`) rather than picking a local folder (RPROJ-01).
  2. Entering a valid repo prefills the project name from the repo name (editable before create) and auto-fills the GitHub link and description (RPROJ-02).
  3. With GitHub integration on, the user can still create a project from a local folder — the folder path is the optional alternative, not the default — and with integration off, project creation is folder-only with no repo-first UI appearing (RPROJ-03, RPROJ-04).
  4. A clone failure (auth, network, or missing repo) surfaces inline in the Add-project flow and leaves no half-created project — no orphan DB row and no partial directory on disk (CKOUT-04).
**Plans**: TBD
**UI hint**: yes

## Progress

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. Foundation — Projects & Board | v1.0 | 7/7 | Complete | 2026-06-10 |
| 2. Terminal Engine | v1.0 | 5/5 | Complete | 2026-06-10 |
| 3. Worktree Isolation & Bash Tabs | v1.0 | 6/6 | Complete | 2026-06-10 |
| 4. Claude Code Agent Sessions | v1.0 | 5/5 | Complete | 2026-06-11 |
| 5. Recovery & Review | v1.0 | 5/5 | Complete | 2026-06-11 |
| 6. Settings & Polish | v1.1 | 4/4 | Complete | 2026-06-11 |
| 7. Claude Quota Indicator | v1.2 | 3/3 | Complete | 2026-06-12 |
| 8. tmux Shells — Spawn & Detach Lifecycle | v1.2 | 4/4 | Complete | 2026-06-13 |
| 9. tmux Restart Resume & Cleanup Integration | v1.2 | 5/5 | Complete | 2026-06-13 |
| 10. GitHub Foundations | v1.3 | 5/5 | Complete | 2026-06-13 |
| 11. PR Review Column | v1.3 | 4/4 | Complete | 2026-06-14 |
| 12. Open-a-Review | v1.3 | 7/7 | Complete | 2026-06-14 |
| 13. PR Worktree Auto-Cleanup | v1.3 | 3/3 | Complete | 2026-06-14 |
| 14. Managed Checkout Foundations | v1.4 | 4/4 | Complete   | 2026-06-14 |
| 15. Repo-First Creation Flow | v1.4 | 0/? | Not started | - |

---
*v1.0 shipped 2026-06-11 — 5 phases, 28 plans, 74 tasks*
*v1.1 shipped 2026-06-11 — 1 phase, 4 plans, 10 tasks*
*v1.2 shipped 2026-06-13 — 3 phases, 12 plans, 29 tasks*
*v1.3 shipped 2026-06-14 — 4 phases (10–13), 19 plans, 46 tasks, 20 requirements*
*v1.4 roadmapped 2026-06-14 — 2 phases (14–15), 10 requirements mapped, granularity: coarse*
