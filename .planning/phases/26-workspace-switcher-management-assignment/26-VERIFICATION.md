---
phase: 26-workspace-switcher-management-assignment
verified: 2026-07-06T05:52:02Z
status: passed
score: 5/5 roadmap success criteria verified (23/23 plan must-have truths verified)
overrides_applied: 0
re_verification:
  previous_status: none
  previous_score: n/a
---

# Phase 26: Workspace Switcher, Management & Assignment — Verification Report

**Phase Goal:** Users can create, rename, and delete named workspaces, switch the active one from the sidebar, and assign projects to workspaces — while the global Active Sessions bar stays cross-workspace.
**Verified:** 2026-07-06T05:52:02Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths — ROADMAP Success Criteria (the contract)

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| SC1 | Workspace switcher at top of projects sidebar, **expanded-state only** (hidden in icon rail); opening lists all workspaces, picks active, reaches add/rename/delete | ✓ VERIFIED | `WorkspaceSwitcher.tsx:55` outer `<div className="px-2 group-data-[collapsible=icon]:hidden">`; dropdown maps `DropdownMenuCheckboxItem` per workspace + `New workspace` + `Manage workspaces…` (lines 73–90). Mounted at top of `SidebarContent` in `ProjectSidebar.tsx:74`. |
| SC2 | Sidebar list (expanded rows AND collapsed rail) shows only active workspace's projects; switching navigates to first project or clear empty state; active workspace survives reload (`localStorage`) | ✓ VERIFIED | Single `.filter((p) => p.workspace_id === activeWorkspaceId)` on the shared project `.map` (`ProjectSidebar.tsx:81-83`) feeds both surfaces. `handlePick` navigates to first project or `/` (`WorkspaceSwitcher.tsx:46-52`); `RedirectToFirstProject` renders "No projects in {workspaceName} yet" (`App.tsx:25-54`). Persisted via `ACTIVE_WORKSPACE_KEY="kamacu.workspace"` + `localStorage.setItem` (`useActiveWorkspace.tsx:15,70`). |
| SC3 | Create by name; rename any incl. Personal; delete non-empty blocked with "transfer or remove" message; default Personal never deletable (≥1 always exists) | ✓ VERIFIED | Backend `workspaces.go`: `create` 409-dedups (line 118), `update` renames without `is_default` gate (line 137-179), `delete` guards `is_default==1` (line 209) + `COUNT(*) FROM projects` >0 → 409 with count message (line 215-221). `BackfillWorkspaces` guarantees a default row (line 29-41). Client mirrors in `ManageWorkspacesDialog.tsx:104-108`. |
| SC4 | Transfer a project to another workspace from its `⋯` menu; newly created project lands in the active workspace | ✓ VERIFIED | Backend: `projects.go` `update` validates target then `SET workspace_id=?` (line 501-518); `resolveCreateWorkspaceID` (line 242-259). UI: `ProjectMenu.tsx` `DropdownMenuSub` radio submenu (line 102-120), `handleMove` (line 66-72). `AddProjectDialog.tsx:116,121` sends `workspace_id: activeWorkspaceId ?? undefined` in both create branches. |
| SC5 | Global Active Sessions bar keeps showing sessions across every workspace, unaffected by active-workspace filter (WSBAR-01 non-regression) | ✓ VERIFIED | `ActiveSessionsBar.tsx` has ZERO workspace references (grep clean); `agents.ts` has ZERO workspace references. `ProjectSidebar.tsx` `useAgentStatuses`/`waitingByProject` (line 32,39-47) left untouched. |

**Score:** 5/5 ROADMAP success criteria verified

### Supporting Truths — Plan-level must_haves (23 total)

| Plan | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 01 | GET /api/workspaces returns every workspace name-sorted | ✓ VERIFIED | `workspaces.go:78` `ORDER BY name COLLATE NOCASE`; returns `[]Workspace{}` never nil |
| 01 | POST creates by name; case-insensitive dup rejected 409 | ✓ VERIFIED | `workspaces.go:118` `SELECT 1 … COLLATE NOCASE` → 409 |
| 01 | PATCH renames any workspace incl. default Personal | ✓ VERIFIED | `workspaces.go:137-179` no `is_default` gate; test `TestWorkspaceRenamePersonal` green |
| 01 | DELETE refuses non-empty + is_default; deletes empty non-default | ✓ VERIFIED | `workspaces.go:209,219,232`; tests DeleteEmpty/NonEmpty/Default green |
| 02 | PATCH /api/projects/{id} with workspace_id transfers | ✓ VERIFIED | `projects.go:517-518`; `TestProjectTransferWorkspace` green |
| 02 | PATCH with non-existent workspace_id rejected without mutating | ✓ VERIFIED | `projects.go:508-512` 400 before append; test asserts DB row unchanged |
| 02 | POST with workspace_id creates in that workspace | ✓ VERIFIED | `resolveCreateWorkspaceID` (line 242); `TestProjectCreateInWorkspace` green |
| 02 | POST without workspace_id lands in default Personal | ✓ VERIFIED | resolver nil branch → `defaultWorkspaceID()` (line 254); test asserts id 1 |
| 03 | Project type carries workspace_id + Workspace type exists | ✓ VERIFIED | `types.ts:31,40` |
| 03 | useWorkspaces() reads GET /api/workspaces | ✓ VERIFIED | `queries.ts:12-16` queryKey `["workspaces"]` |
| 03 | CRUD + move mutations exist, invalidate right keys | ✓ VERIFIED | `mutations.ts:25-80`; delete invalidates both `["workspaces"]`+`["projects"]` |
| 03 | Active workspace single shared reactive source, localStorage, is_default fallback | ✓ VERIFIED | `useActiveWorkspace.tsx:53-83`; fallback `workspaces.find(w=>w.is_default)` line 65 |
| 04 | Create/rename dialog: name, inline dup error, Save disabled while pending/empty | ✓ VERIFIED | `WorkspaceNameDialog.tsx:100,110` |
| 04 | Manage dialog lists workspaces with rename + delete | ✓ VERIFIED | `ManageWorkspacesDialog.tsx:103-157` |
| 04 | Delete disabled with tooltip for default + non-empty | ✓ VERIFIED | `ManageWorkspacesDialog.tsx:105-108,126-144` (aria-disabled keeps tooltip) |
| 04 | Switcher dropdown: workspaces (✓ active), New, Manage; pick → active + navigate | ✓ VERIFIED | `WorkspaceSwitcher.tsx:73-90,46-52` |
| 05 | Index redirect to active workspace's first project or scoped empty state | ✓ VERIFIED | `App.tsx:25-54` |
| 05 | Deep-link flips active workspace to project's workspace (URL wins) | ✓ VERIFIED | `App.tsx:78-112` `BoardWorkspaceSync` (ref-gated, post-fix) |
| 05 | Add from empty state/sidebar creates in active workspace | ✓ VERIFIED | `AddProjectDialog.tsx:116,121` |
| 06 | Switcher renders at top of expanded sidebar, hidden in collapsed rail | ✓ VERIFIED | `ProjectSidebar.tsx:74` + switcher self-hide wrapper |
| 06 | Sidebar list (both surfaces) shows only active workspace's projects | ✓ VERIFIED | `ProjectSidebar.tsx:81-83` single filter |
| 06 | ⋯ menu Move-to submenu; moving open project makes view follow | ✓ VERIFIED | `ProjectMenu.tsx:66-72,102-120` |
| 06 | Global Active Sessions bar still shows sessions across every workspace | ✓ VERIFIED | `ActiveSessionsBar.tsx`/`agents.ts` no workspace refs |

**Supporting score:** 23/23 plan must-have truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `internal/api/workspaces.go` | workspaceHandlers CRUD + guards | ✓ VERIFIED | 234 lines; list/create/update/delete + scanWorkspace; guards key off `is_default` flag |
| `internal/api/routes.go` | 4 /api/workspaces routes | ✓ VERIFIED | Lines 21-25 register GET/POST/PATCH/DELETE |
| `internal/api/workspaces_test.go` | CRUD + guard tests | ✓ VERIFIED | 8 handler tests + BackfillWorkspaces; all green |
| `internal/api/projects.go` | workspace_id on transfer + create | ✓ VERIFIED | `WorkspaceID *int64` on update+create; resolver + transfer validation |
| `internal/api/projects_test.go` | transfer + create-in-workspace tests | ✓ VERIFIED | TestProjectTransferWorkspace / TestProjectCreateInWorkspace green |
| `web/src/api/types.ts` | Project.workspace_id + Workspace | ✓ VERIFIED | lines 31, 40 |
| `web/src/api/queries.ts` | useWorkspaces | ✓ VERIFIED | line 12 |
| `web/src/api/mutations.ts` | workspace CRUD + useMoveProject | ✓ VERIFIED | lines 25-80 |
| `web/src/lib/useActiveWorkspace.tsx` | provider + hook | ✓ VERIFIED | 104 lines; context, localStorage, is_default fallback |
| `web/src/components/layout/AppLayout.tsx` | provider mounted | ✓ VERIFIED | wraps sidebar+Outlet+bar (line 42-56) |
| `web/src/components/sidebar/WorkspaceNameDialog.tsx` | create/rename dialog | ✓ VERIFIED | 118 lines |
| `web/src/components/sidebar/ManageWorkspacesDialog.tsx` | rename/delete hub | ✓ VERIFIED | 203 lines |
| `web/src/components/sidebar/WorkspaceSwitcher.tsx` | expanded-only switcher | ✓ VERIFIED | 102 lines |
| `web/src/App.tsx` | workspace-aware redirect + URL-wins | ✓ VERIFIED | 141 lines |
| `web/src/components/sidebar/AddProjectDialog.tsx` | create-in-active | ✓ VERIFIED | workspace_id in both branches |
| `web/src/components/sidebar/ProjectSidebar.tsx` | mounted switcher + filter | ✓ VERIFIED | 197 lines |
| `web/src/components/sidebar/ProjectMenu.tsx` | Move-to submenu | ✓ VERIFIED | 160 lines |

### Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| routes.go | workspaceHandlers | mux.HandleFunc VERB /api/workspaces | ✓ WIRED | lines 22-25 |
| workspaceHandlers.delete | projects.workspace_id | COUNT(*) FROM projects | ✓ WIRED | `workspaces.go:215` |
| projectHandlers.update | workspaces table | SELECT 1 … then SET workspace_id=? | ✓ WIRED | `projects.go:508,517` |
| projectHandlers.create | workspaces table | resolveCreateWorkspaceID → defaultWorkspaceID | ✓ WIRED | `projects.go:242-259` |
| useActiveWorkspace | useWorkspaces + localStorage | resolve saved id, fall back to is_default | ✓ WIRED | `useActiveWorkspace.tsx:54,61,65,70` |
| AppLayout | ActiveWorkspaceProvider | wraps sidebar + Outlet + bar | ✓ WIRED | `AppLayout.tsx:42-56` |
| WorkspaceSwitcher | useActiveWorkspace + useNavigate | setActiveWorkspaceId + navigate first project | ✓ WIRED | `WorkspaceSwitcher.tsx:30,33,46-52` |
| ManageWorkspacesDialog | useProjects | per-workspace count for delete guard | ✓ WIRED | `ManageWorkspacesDialog.tsx:50,63-67` |
| App.tsx RedirectToFirstProject | useActiveWorkspace + useProjects | filter by workspace_id === activeWorkspaceId | ✓ WIRED | `App.tsx:25-27` |
| AddProjectDialog | useCreateProject | workspace_id: activeWorkspaceId | ✓ WIRED | `AddProjectDialog.tsx:116,121` |
| ProjectSidebar | useActiveWorkspace | filter p.workspace_id === activeWorkspaceId | ✓ WIRED | `ProjectSidebar.tsx:33,82` |
| ProjectMenu | useMoveProject + useActiveWorkspace | move; if open, setActiveWorkspaceId(target) | ✓ WIRED | `ProjectMenu.tsx:43,45,66-72` |

Note: `gsd-sdk query verify.key-links` reported "Source file not found" for all frontend links because the `from` fields are symbol names (e.g. `useActiveWorkspace`, `WorkspaceSwitcher`) rather than file paths — a tool-resolution limitation, not a wiring gap. All links verified manually by reading the source (evidence above).

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| --- | --- | --- | --- | --- |
| WorkspaceSwitcher | workspaces | `useWorkspaces()` → GET /api/workspaces → SQLite `workspaces` table | Yes | ✓ FLOWING |
| ProjectSidebar (filter) | projects filtered by activeWorkspaceId | `useProjects()` + `useActiveWorkspace()` context | Yes | ✓ FLOWING |
| ManageWorkspacesDialog | countByWorkspace | `useProjects()` derived Map keyed by workspace_id | Yes | ✓ FLOWING |
| RedirectToFirstProject | wsProjects | `useProjects().filter(workspace_id===active)` | Yes | ✓ FLOWING |
| ActiveSessionsBar | live sessions | `useAgentStatuses()` — deliberately UNfiltered (WSBAR-01) | Yes (cross-workspace) | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| Workspace CRUD + guard handlers | `go test ./internal/api/ -run 'Workspace' -count=1` | ok (0.632s, incl. transfer/create-in-workspace) | ✓ PASS |
| Project transfer + create-in-workspace | `go test ./internal/api/ -run 'ProjectTransfer|ProjectCreateInWorkspace|ProjectsCreateAssignsDefaultWorkspace'` | ok | ✓ PASS |
| Full build/vet/test (orchestrator) | `go build ./... && go vet && go test ./...` | PASSED (internal/api ok, internal/store ok) | ✓ PASS |
| Frontend typecheck+build (orchestrator) | `cd web && npm run build` (tsc -b + vite) | PASSED | ✓ PASS |

### Probe Execution

No probes declared or implied for this phase (not a migration/tooling phase; no `scripts/*/tests/probe-*.sh` referenced in PLAN/SUMMARY). Verification model per project convention is build + lint + human-verify. Step 7c: N/A.

### Requirements Coverage

| Requirement | Source Plan(s) | Description | Status | Evidence |
| --- | --- | --- | --- | --- |
| WSMGMT-01 | 01, 04 | Create workspace by name | ✓ SATISFIED | `workspaces.go:103`; `WorkspaceNameDialog` create mode |
| WSMGMT-02 | 01, 04 | Rename any incl. Personal | ✓ SATISFIED | `workspaces.go:137` (no is_default gate); rename control |
| WSMGMT-03 | 01, 04 | Delete only when empty; else clear message | ✓ SATISFIED | `workspaces.go:219-221`; tooltip guard |
| WSMGMT-04 | 01, 04 | Default never deletable; ≥1 always exists | ✓ SATISFIED | `workspaces.go:209`; `BackfillWorkspaces` |
| WSNAV-01 | 01, 04, 06 | Expanded-only switcher, add/rename/delete | ✓ SATISFIED | `WorkspaceSwitcher.tsx:55`; mounted `ProjectSidebar.tsx:74` |
| WSNAV-02 | 06 | Both surfaces show only active workspace's projects | ✓ SATISFIED | `ProjectSidebar.tsx:81-83` single filter |
| WSNAV-03 | 03, 05 | localStorage restore + switch nav + empty state | ✓ SATISFIED | `useActiveWorkspace.tsx`; `App.tsx` redirect/empty/URL-wins |
| WSPROJ-01 | 02, 06 | Transfer via ⋯ menu | ✓ SATISFIED | `projects.go:501-518`; `ProjectMenu.tsx:102-120` |
| WSPROJ-02 | 02, 05 | New project lands in active workspace | ✓ SATISFIED | `resolveCreateWorkspaceID`; `AddProjectDialog.tsx:116,121` |
| WSBAR-01 | 03, 06 | Sessions bar cross-workspace, unaffected | ✓ SATISFIED | zero workspace refs in `ActiveSessionsBar.tsx`/`agents.ts` |

**Coverage:** 10/10 requirement IDs claimed by plans, all present in REQUIREMENTS.md, no orphans. Matches roadmap traceability (Phase 26: 10).

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| --- | --- | --- | --- | --- |
| ManageWorkspacesDialog.tsx | 88-92 | `mutateAsync` delete with no try/catch; enable-guard trusts possibly-unloaded `useProjects()` | ⚠️ Warning (WR-01) | Failed delete silently swallowed; unresolved project list could briefly render an incorrect enabled delete. Server + FK RESTRICT prevent data loss. Non-blocking. |
| ProjectMenu.tsx | 66-72 | `handleMove` flips active workspace optimistically, no onError rollback | ⚠️ Warning (WR-02) | If PATCH fails (e.g. target deleted concurrently), sidebar filters to a workspace the project didn't move into until reload. Single-user local app; edge case. Non-blocking. |
| ProjectMenu.tsx | 50-59 | `handleDelete` swallows managed-project 409 structured reasons (predates Phase 26) | ⚠️ Warning (WR-03) | Managed-delete rejection reasons not surfaced. Pre-existing pattern. Non-blocking. |
| ProjectMenu.tsx | 104 | Submenu trigger visible label "Move to" vs UI-SPEC "Move to workspace" | ℹ️ Info | Documented, human-approved deviation (D-08): shortened to avoid 2-line wrap; `aria-label="Move to workspace"` preserves the accessible name and the must_have `contains` check. |

No debt markers (TBD/FIXME/XXX) in any modified file. No stubs — grep matches for TODO/PLACEHOLDER are SQL `?`-placeholder comments, the `"To Do"` kanban status enum, and legitimate HTML input `placeholder` attributes; `return null` matches are loading/guard states, not stub returns. The 3 warnings are the same client-side error-handling robustness items logged in 26-REVIEW.md (0 blockers, 3 warnings, 4 info) — none risk data loss because the server enforces every invariant, and none falsify a must-have truth.

### Human Verification Required

None outstanding. The phase's own blocking end-of-phase human-verify gate (26-06 PLAN Task 3) was executed and **PASSED** — all 10 UI checks confirmed by the user (per trusted orchestrator context and 26-06-SUMMARY.md), including the two post-gate fixes: the empty-workspace-switch revert bug (commit `8367783`) and the "Move to" label wrap (commit `f09cfba`), plus the deep-link URL-wins check. Re-surfacing the same visual/interaction checks would be a redundant loop; the single human-verify sink is already satisfied.

### Gaps Summary

No gaps. Every ROADMAP success criterion and every plan-level must-have truth is independently verified in the codebase with concrete evidence at all four levels (exists, substantive, wired, data-flowing). Backend guards are enforced server-side and keyed off the rename-proof `is_default` flag (never the string "Personal"), with green tests. The WSBAR-01 non-regression guardrail is confirmed clean by grep (zero workspace references reaching the Active Sessions bar or `useAgentStatuses`). All 10 requirement IDs are accounted for with no orphans. The 3 warnings from 26-REVIEW.md are non-blocking client-side error-handling robustness items on failure edge cases; they do not prevent goal achievement and are already logged for a future robustness pass. The end-of-phase human-verify gate passed with explicit user approval.

---

_Verified: 2026-07-06T05:52:02Z_
_Verifier: Claude (gsd-verifier)_
