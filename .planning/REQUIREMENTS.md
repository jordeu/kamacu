# Requirements: Kamacu — v1.9 Workspaces

**Defined:** 2026-07-05
**Core Value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.

## v1.9 Requirements

Requirements for the Workspaces milestone. Each maps to exactly one roadmap phase.

### Data & Migration

- [ ] **WSDATA-01**: Every project belongs to exactly one workspace — a `workspaces` table plus a `projects.workspace_id` foreign key (migration `00012`), enforced so a project can never be workspace-less.
- [ ] **WSDATA-02**: On first startup after upgrade, a one-time idempotent backfill creates a default **Personal** workspace and assigns every pre-existing project to it (mirrors the `BackfillProjectIcons` startup-hook pattern; safe to re-run).

### Workspace Management

- [ ] **WSMGMT-01**: User can create a new workspace by name.
- [ ] **WSMGMT-02**: User can rename any workspace, including the default Personal workspace.
- [ ] **WSMGMT-03**: User can delete a workspace only when it contains no projects; attempting to delete a non-empty workspace is blocked with a clear message telling the user to transfer or remove its projects first.
- [ ] **WSMGMT-04**: The default Personal workspace can never be deleted, and at least one workspace always exists — guaranteeing a permanent home for projects.

### Switcher & Navigation

- [ ] **WSNAV-01**: A workspace switcher sits at the top of the projects sidebar in its **expanded state only** (hidden in the collapsed icon rail); clicking it opens a dropdown to pick the active workspace and to reach add/rename/delete management.
- [ ] **WSNAV-02**: The sidebar project list — both the expanded rows and the collapsed icon rail — shows only the projects belonging to the active workspace.
- [ ] **WSNAV-03**: The active workspace is remembered in `localStorage` and restored on reload; switching workspaces navigates to that workspace's first project, and shows a clear empty state when the workspace has no projects.

### Project Assignment

- [ ] **WSPROJ-01**: User can transfer a project to a different workspace from the project's existing `⋯` menu.
- [ ] **WSPROJ-02**: A newly created project is placed in the currently active workspace.

### Global Sessions Bar

- [ ] **WSBAR-01**: The global Active Sessions bar continues to show agent sessions across all projects in every workspace, unaffected by the active-workspace filter (non-regression of the existing cross-project bar).

## Future Requirements

Acknowledged but deferred beyond v1.9. Tracked, not in the current roadmap.

### Workspace Identity

- **WSFUT-01**: Workspace icon/color identity (a monogram + color like projects have), shown in the switcher and header.

### Workspace UX

- **WSFUT-02**: Reorder workspaces in the switcher (drag-to-reorder or manual order).
- **WSFUT-03**: Bulk / multi-select transfer of projects between workspaces.

## Out of Scope

Explicitly excluded for v1.9. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Cascade-delete projects when deleting a workspace | Rejected in favor of block-until-empty — never bulldoze a project (and its worktrees/sessions) as a side effect of workspace deletion |
| Workspace icons/colors | Kept name-only for a thin slice; parked as WSFUT-01 |
| Drag-and-drop project between workspaces | Transfer is via the `⋯` menu only for v1.9 |
| Reordering workspaces | Deferred (WSFUT-02) |
| Nested/hierarchical workspaces or sub-groups | Flat single-level grouping only |
| Per-workspace settings overrides | Settings stay global; workspaces are a grouping construct only |
| Filtering the global sessions bar by workspace | The bar is deliberately global (WSBAR-01) |
| Workspace in the URL / bookmarkable workspace routes | Active workspace is localStorage-only; routing stays `/projects/:projectId` |
| Multi-user / shared workspaces | Single-user localhost app — out of scope project-wide |

## Traceability

Which phases cover which requirements. Populated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| WSDATA-01 | Phase 25 | Pending |
| WSDATA-02 | Phase 25 | Pending |
| WSMGMT-01 | Phase 26 | Pending |
| WSMGMT-02 | Phase 26 | Pending |
| WSMGMT-03 | Phase 26 | Pending |
| WSMGMT-04 | Phase 26 | Pending |
| WSNAV-01 | Phase 26 | Pending |
| WSNAV-02 | Phase 26 | Pending |
| WSNAV-03 | Phase 26 | Pending |
| WSPROJ-01 | Phase 26 | Pending |
| WSPROJ-02 | Phase 26 | Pending |
| WSBAR-01 | Phase 26 | Pending |

**Coverage:**
- v1.9 requirements: 12 total
- Mapped to phases: 12 ✓ (Phase 25: 2 · Phase 26: 10)
- Unmapped: 0

---
*Requirements defined: 2026-07-05*
*Last updated: 2026-07-05 — traceability populated by roadmapper (Phases 25–26, 12/12 mapped)*
