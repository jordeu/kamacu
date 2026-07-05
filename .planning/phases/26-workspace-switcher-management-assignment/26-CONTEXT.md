# Phase 26: Workspace Switcher, Management & Assignment - Context

**Gathered:** 2026-07-05
**Status:** Ready for planning

<domain>
## Phase Boundary

The **user-visible workspace layer** on top of Phase 25's data foundation. Nothing here changes the schema (migration `00012` already landed); this phase is the UI + the workspace CRUD/transfer endpoints that drive it.

Delivers:
1. A **workspace switcher** at the top of the projects sidebar — **expanded state only** (hidden in the collapsed icon rail): a trigger that opens a dropdown to pick the active workspace and reach add / rename / delete.
2. **Workspace CRUD**: create by name, rename any workspace (including Personal), delete only when empty — with the default (`is_default`) workspace permanently undeletable and at least one workspace always present.
3. **Per-workspace filtering** of the sidebar project list (both expanded rows AND the collapsed icon rail) to the active workspace's projects.
4. **Switch navigation** to the workspace's first project (or a workspace-scoped empty state), with the active workspace **persisted in `localStorage`** across reloads.
5. **Project transfer** to another workspace via the project's existing `⋯` menu, and **create-in-active-workspace** for new projects (refines Phase 25's interim default-to-Personal).
6. The global **Active Sessions bar stays cross-workspace** — unaffected by the active-workspace filter (non-regression of the existing cross-project bar).

Requirements covered: **WSMGMT-01/02/03/04**, **WSNAV-01/02/03**, **WSPROJ-01/02**, **WSBAR-01**.

**Out of this phase (per REQUIREMENTS.md Out of Scope):** cascade-delete of projects, workspace icons/colors (WSFUT-01), drag-and-drop transfer, reordering workspaces (WSFUT-02), bulk/multi-select transfer (WSFUT-03), nested/hierarchical workspaces, per-workspace settings overrides, filtering the sessions bar by workspace, workspace in the URL / bookmarkable workspace routes (active workspace is `localStorage`-only; routing stays `/projects/:projectId`), remember-last-viewed-project-per-workspace.
</domain>

<decisions>
## Implementation Decisions

### Switcher shape & placement (WSNAV-01)
- **D-01:** The switcher trigger is a **full-width "name + chevron" button** at the top of the expanded sidebar, styled like a section header, opening a **dropdown-menu** on click. Reuses the existing shadcn `dropdown-menu` primitive (`web/src/components/ui/dropdown-menu.tsx`) and the sidebar visual language. **Expanded-only** — hidden in the collapsed icon rail via the same `group-data-[collapsible=icon]:hidden` idiom the KamacuMark brand lockup already uses in `ProjectSidebar.tsx`.
- **D-02:** The dropdown content is: the workspace list for **pick-active** (active row marked with a **✓**, the standard selected-item pattern), a separator, a quick **"+ New workspace"** entry, and a **"⚙ Manage workspaces…"** entry. Picking a workspace sets it active.
- **D-03:** **Rename and delete live in a dedicated "Manage workspaces" dialog** (opened from the dropdown's "⚙ Manage workspaces…" entry), listing every workspace with per-row rename/delete affordances plus an add control. This keeps the dropdown clean and gives the delete-guard messages room. The quick "+ New workspace" in the dropdown is a fast path to the same create flow.

### Workspace create / rename / delete (WSMGMT-01/02/03/04)
- **D-04:** Create and rename both use a **compact name-Input dialog** mirroring `web/src/components/sidebar/RenameProjectDialog.tsx` — create = empty field, rename = prefilled current name (works for Personal too, WSMGMT-02). The DB `name UNIQUE COLLATE NOCASE` rejection (from migration `00012`) is caught and rendered as **inline error text**, exactly as `RenameProjectDialog` already does via `ApiError` + `sentenceCase`.
- **D-05:** **Non-empty delete is proactively disabled** with a tooltip/hint like *"Move or remove its N projects first."* The per-workspace project count is computed **client-side** from the existing `useProjects()` data (each `Project` now carries `workspace_id`) — no dedicated count endpoint needed. The backend still enforces the block (return an error; DB `ON DELETE RESTRICT` is the ultimate backstop, D-04 of Phase 25) so the frontend disable is a UX affordance, not the security boundary.
- **D-06:** The **default (`is_default = 1`) workspace shows a disabled delete** with a tooltip like *"The default workspace can't be deleted"* (WSMGMT-04). Keyed off the **`is_default` flag**, never the name "Personal" — rename-proof, per Phase 25 D-02. This also upholds the "at least one workspace always exists" invariant.
- **D-07:** Deleting an **empty** workspace goes through a **light `AlertDialog` confirm** ("Delete workspace?"), consistent with the existing project-delete confirm in `ProjectMenu.tsx`.

### Project transfer & create-in-active (WSPROJ-01/02)
- **D-08:** Transfer is a **"Move to workspace" submenu** added to the project's existing `⋯` menu (`ProjectMenu.tsx`), listing all workspaces as **radio items** with the project's **current workspace checked/disabled**. Reuses `DropdownMenuSub` + `DropdownMenuRadioGroup`/`DropdownMenuRadioItem`, already exported from `dropdown-menu.tsx` — no new primitive. One hover + click, no dialog.
- **D-09:** When the user transfers the **currently-open project** out of the active workspace, **the view follows the project**: the active workspace flips to the target so the moved project stays on screen and selected, and `localStorage` active-workspace updates to match. Transferring **any other** (not-currently-open) project just removes it from the filtered list; the active workspace is unchanged. (Coherent with the D-14 reload rule.)
- **D-10:** A **newly created project lands in the currently active workspace** (WSPROJ-02), refining Phase 25's interim default-to-Personal. The create paths in `internal/api/projects.go` (`create` folder path + `createByRepo` repo-first path) must set `workspace_id` from the active workspace the client sends, falling back to the default when unspecified.

### Switch navigation, empty state & reload (WSNAV-02/03)
- **D-11:** Switching lands on the workspace's **first project by name** — which is the sidebar order (backend `list` sorts `ORDER BY name COLLATE NOCASE`). Predictable, matches WSNAV-03's literal "first project," no per-workspace last-project state (that would be scope creep — see Deferred).
- **D-12:** The **sidebar project list is filtered to the active workspace** in BOTH surfaces — the expanded rows and the collapsed icon rail (WSNAV-02). Filtering is client-side over `useProjects()` by `workspace_id`.
- **D-13:** When the active workspace has **no projects**, the board area shows a **workspace-scoped empty state** — e.g. *"No projects in {workspace} yet"* + an "Add project" button that creates into this workspace (D-10). Adapt the existing `App.tsx` `RedirectToFirstProject` "No projects yet" empty state to be workspace-aware (don't show a global "no projects" message when other workspaces still have projects).
- **D-14:** **On reload / deep-link, the URL's project wins.** If the project in the URL belongs to a different workspace than the one saved in `localStorage`, set the active workspace to **that project's workspace** (and update `localStorage`), so the viewed project is always visible in its sidebar. Baseline: a **stale/deleted saved workspace id falls back to the default** (`is_default`) workspace; first-ever load with no saved value also uses the default.

### Filtering boundary & sessions-bar guardrail (WSBAR-01)
- **D-15:** The active-workspace **filter lives ONLY in the sidebar project list + the index/redirect logic**. The global Active Sessions bar reads its own independent `useAgentStatuses()` poll (`GET /api/agents/status`, `web/src/api/agents.ts`), NOT `useProjects()` — so it is naturally unaffected by the filter. **Do not** thread a workspace filter into the sessions bar or its query. WSBAR-01 is a non-regression: keep the bar cross-workspace.

### Backend surface (new endpoints)
- **D-16:** New workspace REST endpoints, registered in `internal/api/routes.go` alongside the project routes and implemented in `internal/api/workspaces.go` (the file already exists for the Phase 25 `BackfillWorkspaces` hook): `GET /api/workspaces` (list), `POST /api/workspaces` (create by name), `PATCH /api/workspaces/{id}` (rename), `DELETE /api/workspaces/{id}` (delete, guarded: refuse if it owns projects, refuse if `is_default`). Mirror the `projectHandlers` shape (struct + `sql.DB`, `scanX`/`columns` const, `writeError`/respond helpers) and the single-writer / parameterized-`?` discipline from Phase 25.
- **D-17:** Project **transfer is a `workspace_id` PATCH on the existing project endpoint** — extend `PATCH /api/projects/{id}` (`internal/api/projects.go` `update`, already a partial-PATCH per D-13 of that file) to accept an optional `workspace_id`, validated against an existing workspace. No separate transfer route.

### Claude's Discretion
- **Active-workspace state management shape** — where the active-workspace id lives (a small `localStorage`-backed hook, React context, or a `zustand` store per the stack's "ephemeral client state" guidance) and how it's shared between the switcher, the sidebar filter, the index redirect, and the transfer "follow" logic. Follow existing patterns (the app already uses raw `localStorage` for sidebar/session-bar/review-column collapse state; `zustand` is available but unused so far). Researcher/planner decide.
- Exact endpoint response shapes, error codes (e.g. 409 vs 422 for the non-empty-delete block), the precise dialog copy, the `✓`/disabled-radio rendering details, and whether the Manage dialog's add control opens the same create dialog or an inline row — follow existing patterns; no user preference expressed beyond the decisions above.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & roadmap (scope anchors)
- `.planning/ROADMAP.md` § "Phase 26: Workspace Switcher, Management & Assignment" — goal + the 5 success criteria.
- `.planning/REQUIREMENTS.md` — **WSMGMT-01..04, WSNAV-01..03, WSPROJ-01/02, WSBAR-01** (the locked requirements), the **Out of Scope** table (no cascade-delete, transfer via `⋯` only, no reorder, flat, localStorage-only routing, sessions bar stays global), and **WSFUT-01/02/03** (deferred).

### Phase 25 foundation this builds on (READ FIRST)
- `.planning/phases/25-workspace-data-foundation/25-CONTEXT.md` — the data-layer decisions: `is_default` flag identity (D-02, rename-proof — the delete/protection guards key off it), `name UNIQUE COLLATE NOCASE` (D-03, the create/rename reject), `ON DELETE RESTRICT` (D-04, the DB backstop for block-until-empty), `workspace_id` on the projects wire (D-10).
- `internal/store/migrations/00012_workspaces.sql` — the shipped schema: `workspaces` table (`id`, `name`, `is_default`, timestamps), the case-insensitive unique index, and `projects.workspace_id NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT`.

### Switcher / sidebar assets
- `web/src/components/sidebar/ProjectSidebar.tsx` — where the switcher mounts (top of `SidebarHeader`/`SidebarContent`); the `group-data-[collapsible=icon]:hidden` expanded-only idiom (brand lockup) and the collapsed-rail avatar rendering to filter (D-12).
- `web/src/components/ui/dropdown-menu.tsx` — the switcher dropdown + the transfer submenu. Exports present: `DropdownMenuSub`, `DropdownMenuSubTrigger`, `DropdownMenuSubContent`, `DropdownMenuRadioGroup`, `DropdownMenuRadioItem`, `DropdownMenuCheckboxItem`.
- `web/src/components/ui/dialog.tsx` + `web/src/components/ui/alert-dialog.tsx` — the create/rename dialog (D-04) and the empty-delete confirm (D-07).
- `web/src/components/sidebar/RenameProjectDialog.tsx` — **the create/rename dialog template** (D-04): controlled name Input, prefill-on-open, `ApiError` → inline `sentenceCase` error, disabled-while-pending Save.

### Project menu / transfer
- `web/src/components/sidebar/ProjectMenu.tsx` — the `⋯` `DropdownMenu` that gains the "Move to workspace" submenu (D-08); also the AlertDialog + `useNavigate` delete pattern to mirror for workspace delete.

### Routing, filtering & localStorage persistence
- `web/src/App.tsx` — `RedirectToFirstProject` (index redirect to `projects[0]`) → must become active-workspace-aware (D-11/D-13/D-14); the routes stay `/projects/:projectId`.
- `web/src/components/layout/AppLayout.tsx` — the `localStorage`-backed sidebar-open pattern (`SIDEBAR_STORAGE_KEY`, `!== "false"` idiom) — the template for active-workspace persistence (D-14).
- `web/src/components/layout/ActiveSessionsBar.tsx` + `web/src/api/agents.ts` — the **independent** `useAgentStatuses()` poll the bar consumes; DO NOT filter it by workspace (D-15, WSBAR-01).
- `web/src/lib/migrateStorage.ts` — the `kamacu.*` localStorage prefix convention for any new key.

### Frontend data layer
- `web/src/api/types.ts` — `Project` interface **missing `workspace_id`** — add `workspace_id: number` (it's already on the backend wire from Phase 25).
- `web/src/api/queries.ts` (`useProjects`) + `web/src/api/mutations.ts` (`useCreateProject`, `useRenameProject`, `useDeleteProject`, invalidation pattern) — add workspace queries/mutations here (list/create/rename/delete + transfer) following the exact `queryKey`/`invalidateQueries(["projects"])` shape.
- `web/src/api/client.ts` — `get`/`post`/`patch`/`del` + `ApiError`.

### Backend endpoints & patterns
- `internal/api/routes.go` — register the new workspace routes here (alongside the project routes).
- `internal/api/workspaces.go` — already holds `BackfillWorkspaces`; add the workspace handlers here.
- `internal/api/projects.go` — `Project` struct (`workspace_id` at `scanProject`/`projectColumns` order, already wired), the `update` partial-PATCH (extend for transfer, D-17), and the two create INSERT sites (set active `workspace_id`, D-10).
- `internal/store/store.go` — pragma discipline: `foreign_keys(1)` (RESTRICT bites) and `SetMaxOpenConns(1)` (single-writer → collect-then-update; never hold a SELECT cursor while issuing writes).

No external ADRs/specs — requirements are fully captured in the decisions above plus these in-repo references.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`RenameProjectDialog.tsx`** — near-verbatim template for the workspace create/rename dialog (controlled Input, prefill-on-open, inline `ApiError` handling).
- **`ProjectMenu.tsx`** — the `⋯` dropdown to extend with the transfer submenu; also its AlertDialog + navigate-on-delete pattern for the workspace-delete confirm.
- **`dropdown-menu.tsx`** — full submenu + radio-group + checkbox-item exports already present; the switcher dropdown and transfer submenu need no new UI primitives.
- **`AppLayout.tsx` localStorage idiom** — the model for persisting the active workspace (`kamacu.*` prefixed key, `getItem/setItem`, sensible default).
- **`useProjects()` + `projectColumns`/`scanProject`** — `workspace_id` already flows on the wire (Phase 25); the frontend just needs the type field and to filter/count on it.
- **`projectHandlers` in `projects.go`** — the handler struct + `scanX`/`columns` + `writeError`/respond shape to mirror for `workspaceHandlers`.

### Established Patterns
- **Expanded-only rendering** via `group-data-[collapsible=icon]:hidden` (brand lockup, expanded project rows) — the switcher uses the same to stay hidden in the icon rail (WSNAV-01).
- **Client-side derivation over the projects list** (e.g. `waitingByProject` map in `ProjectSidebar.tsx`) — the per-workspace project count for the non-empty-delete guard (D-05) and the sidebar filter (D-12) follow this same in-memory approach.
- **TanStack Query invalidation on mutation** — `invalidateQueries(["projects"])` after create/rename/delete/move; workspace mutations invalidate `["workspaces"]` and, for transfer/create, `["projects"]`.
- **Partial PATCH contract** — `PATCH /api/projects/{id}` treats an omitted key as untouched (`update` in `projects.go`, `useUpdateProjectSettings` builds the body conditionally); transfer adds an optional `workspace_id` (D-17).
- **Single-writer + parameterized SQL** — `SetMaxOpenConns(1)`, `foreign_keys(1)`; all new workspace SQL uses `?` placeholders and collect-then-update if a cursor is involved.
- **Go REST route registration** — `mux.HandleFunc("VERB /api/...", handler)` in `routes.go`, method+wildcard ServeMux patterns.

### Integration Points
- New switcher component mounted at the top of `ProjectSidebar.tsx` (expanded-only).
- New "Manage workspaces" dialog + workspace create/rename dialogs (new files under `web/src/components/sidebar/` following the project-dialog naming).
- "Move to workspace" submenu inside `ProjectMenu.tsx`.
- Active-workspace state (hook/store — Claude's discretion) consumed by: the switcher, the sidebar filter, `App.tsx` `RedirectToFirstProject`, the empty-state board, and the transfer "follow" logic.
- New workspace endpoints in `internal/api/workspaces.go` + `routes.go`; `workspace_id` transfer on `PATCH /api/projects/{id}`; active `workspace_id` on the two create paths in `projects.go`.
- `web/src/api/types.ts` `Project.workspace_id`, plus workspace queries/mutations in `queries.ts`/`mutations.ts`.
</code_context>

<specifics>
## Specific Ideas

- Preview mockups the user approved during discussion (in `26-DISCUSSION-LOG.md`): the sidebar switcher trigger row, the dropdown vs. Manage-dialog split, the disabled-delete tooltips ("Move or remove its N projects first" / "The default workspace can't be deleted"), the empty-delete confirm, the "Move to workspace" radio submenu, the follow-the-project transfer behavior, the workspace-scoped empty board, and the URL-wins reload rule. Treat these as the intended UX.
- No toast/sonner library exists in the app — all feedback (guard blocks, errors) stays **inline / tooltip / dialog**, never a toast.
</specifics>

<deferred>
## Deferred Ideas

- **WSFUT-01** — workspace icon/color identity (monogram + palette). Workspaces stay name-only; a future `00013` migration adds columns.
- **WSFUT-02** — reorder workspaces in the switcher (no `position` column exists yet, Phase 25 D-01).
- **WSFUT-03** — bulk / multi-select project transfer.
- **Remember-last-viewed-project per workspace** — considered for switch navigation (D-11) and deferred: WSNAV-03 specifies "first project," and per-workspace last-project state is a future enhancement, not this phase.
- Filtering the sessions bar by workspace — permanently out of scope (WSBAR-01 keeps it global).

None of these were pulled into Phase 26 — discussion stayed within the switcher/management/assignment boundary.
</deferred>

---

*Phase: 26-workspace-switcher-management-assignment*
*Context gathered: 2026-07-05*
