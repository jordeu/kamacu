---
phase: 26-workspace-switcher-management-assignment
reviewed: 2026-07-06T00:00:00Z
depth: standard
files_reviewed: 17
files_reviewed_list:
  - internal/api/projects.go
  - internal/api/projects_test.go
  - internal/api/routes.go
  - internal/api/workspaces.go
  - internal/api/workspaces_test.go
  - web/src/App.tsx
  - web/src/api/mutations.ts
  - web/src/api/queries.ts
  - web/src/api/types.ts
  - web/src/components/layout/AppLayout.tsx
  - web/src/components/sidebar/AddProjectDialog.tsx
  - web/src/components/sidebar/ManageWorkspacesDialog.tsx
  - web/src/components/sidebar/ProjectMenu.tsx
  - web/src/components/sidebar/ProjectSidebar.tsx
  - web/src/components/sidebar/WorkspaceNameDialog.tsx
  - web/src/components/sidebar/WorkspaceSwitcher.tsx
  - web/src/lib/useActiveWorkspace.tsx
findings:
  critical: 0
  warning: 3
  info: 4
  total: 7
status: issues_found
---

# Phase 26: Code Review Report

**Reviewed:** 2026-07-06T00:00:00Z
**Depth:** standard
**Files Reviewed:** 17
**Status:** issues_found

## Summary

Reviewed the Phase 26 workspace switcher / management / project-assignment slice: the
`/api/workspaces` CRUD surface, the project create-in-workspace and transfer-via-PATCH
extensions, and the full sidebar switcher/management/filter UI plus the shared
active-workspace context.

**High-level assessment: the security- and data-integrity-critical paths are sound.**
I specifically verified and could NOT break:

- **SQL injection** — every workspace and project query uses `?` placeholders. The only
  string concatenation is the constant `workspaceColumns` / `projectColumns` and the
  `strings.Join(sets, ", ")` in `projects.update`, whose entries are all literal
  `column = ?` fragments; placeholder/arg counts stay aligned across every field
  combination (incl. the arg-less `github_repo = NULL` unlink and `updated_at` literal).
- **Command injection** — all git shell-outs use arg arrays, never `sh -c`.
- **Workspace delete guards** — the default guard keys off `is_default` (rename-proof,
  D-06) and the non-empty guard does an explicit `COUNT` before the delete, backstopped
  by `ON DELETE RESTRICT` on `projects.workspace_id` (migration 00012:31). Two
  independent layers — no data-loss path.
- **Forged/stale workspace_id** — create (`resolveCreateWorkspaceID`) and transfer
  (`update`) both validate the target against `workspaces` before touching a row and
  return a clean 400; the NOT NULL FK is the ultimate backstop.
- **The App.tsx BoardWorkspaceSync race** — traced the switcher-picks-empty-workspace,
  transfer-open-project, deep-link-reload, and back-button scenarios. The
  `lastSyncedProjectId` ref correctly gates reconciliation to genuine URL-project
  changes; the switcher wins while D-14 URL-wins deep-linking is preserved.
- **TanStack Query invalidation** — every mutation invalidates the correct key(s);
  no stale-cache path found.
- **WSBAR-01** — `ActiveSessionsBar` receives no workspace prop and contains no
  workspace references; it stays cross-workspace.

The findings below are all client-side robustness / error-handling gaps and cosmetic
items. None risk data loss (the server enforces every invariant), so none are BLOCKERs.

## Narrative Findings (AI reviewer)

## Warnings

### WR-01: Failed workspace delete is swallowed; delete guard trusts possibly-unloaded project data

**File:** `web/src/components/sidebar/ManageWorkspacesDialog.tsx:88-92` (and `104-108`, `188-190`)

**Issue:** `handleDelete` does `await deleteWorkspace.mutateAsync(deleteTarget.id)` with no
try/catch. `AlertDialogAction` closes the alert synchronously on click regardless of the
handler's returned promise, so if the mutation rejects (e.g. the server's non-empty
guard fires with a 409): the alert closes, `setDeleteTarget(null)` never runs, the
workspace is NOT deleted, and the user sees no error — plus an unhandled promise
rejection.

This is reachable because the client-side enable guard `deleteDisabled = ws.is_default
|| count > 0` derives `count` from `useProjects()`. While that query is still
loading/undefined, `countByWorkspace` is empty, so `count` is `0` for every workspace:
a non-default workspace that actually owns projects renders with the delete button
ENABLED and the confirmation copy hardcoded to `"Its projects are unaffected — it has
none."` (line 189) — both wrong. Clicking through then hits the server 409, which is
silently swallowed per the above. Data is safe (server + FK RESTRICT), but the UX
lies and then fails without feedback.

**Fix:** Wrap the mutation and surface failures; only close on success:
```tsx
async function handleDelete() {
  if (!deleteTarget) return;
  try {
    await deleteWorkspace.mutateAsync(deleteTarget.id);
    setDeleteTarget(null);
  } catch (err) {
    setDeleteError(
      err instanceof ApiError ? sentenceCase(err.message) : "Couldn't delete. Try again.",
    );
  }
}
```
Also gate the enabled/`"it has none"` state on `projects !== undefined` (treat an
unresolved project list as "count unknown → keep delete disabled") so the guard never
claims empty before it can see the projects.

### WR-02: ProjectMenu.handleMove flips the active workspace optimistically with no rollback on a failed transfer

**File:** `web/src/components/sidebar/ProjectMenu.tsx:66-72`

**Issue:** `handleMove` fires `moveProject.mutate(...)` (fire-and-forget, no `onError`)
and then immediately calls `setActiveWorkspaceId(targetWorkspaceId)` for the open
project. If the PATCH fails — e.g. the target workspace was deleted in another
surface/tab and the server returns 400 `"workspace not found"` — the active workspace
has already been switched to a workspace the project did NOT move into. Result: the
sidebar now filters to the target workspace (which does not contain this project), the
open project drops out of the sidebar entirely while its board is still showing, and no
error is surfaced. Because `BoardWorkspaceSync`'s `lastSyncedProjectId` ref already
equals this `projectId`, it will not reconcile the mismatch either — the inconsistent
state persists until a manual reload.

**Fix:** Drive the follow-the-project flip from mutation success, and surface errors:
```tsx
function handleMove(targetWorkspaceId: number) {
  if (targetWorkspaceId === project.workspace_id) return;
  const follow = projectId === String(project.id);
  moveProject.mutate(
    { id: project.id, workspace_id: targetWorkspaceId },
    {
      onSuccess: () => { if (follow) setActiveWorkspaceId(targetWorkspaceId); },
      onError: () => {/* toast / inline error */},
    },
  );
}
```

### WR-03: ProjectMenu.handleDelete swallows the gated managed-project 409 (structured reasons never surfaced)

**File:** `web/src/components/sidebar/ProjectMenu.tsx:50-59`

**Issue:** `handleDelete` awaits `deleteProject.mutateAsync(project.id)` with no
try/catch, in an `AlertDialogAction` that closes on click. For a **managed** project the
backend delete (`projects.go` `deleteManaged`) can legitimately return 409 with a
structured `{ error, reasons: [...] }` body (uncommitted / unpushed / stash / running
sessions). On that 409 the mutation rejects, the alert closes, and the carefully
structured reasons the backend went to trouble to produce are never shown — the user
just sees the project fail to disappear, with no explanation and an unhandled rejection.
(This error-handling shape predates Phase 26, but it sits in a reviewed file and the
managed-delete 409 path makes it user-visible.)

**Fix:** Catch the rejection and render the server reason(s) instead of silently
closing:
```tsx
try {
  await deleteProject.mutateAsync(project.id);
  if (isCurrent) { await queryClient.refetchQueries({ queryKey: ["projects"] }); navigate("/"); }
  setDeleteOpen(false);
} catch (err) {
  setDeleteError(err instanceof ApiError ? err.message : "Couldn't delete the project.");
  // keep the dialog open
}
```

## Info

### IN-01: Stale doc comment on `defaultWorkspaceID`

**File:** `internal/api/projects.go:223-227`

**Issue:** The comment states "Both create paths call it before their INSERT to set
workspace_id (D-09)." Since the `resolveCreateWorkspaceID` refactor, `defaultWorkspaceID`
is called from exactly one place (`resolveCreateWorkspaceID`, whose own comment correctly
says "This is the SINGLE direct defaultWorkspaceID call site"). The two comments
contradict each other.

**Fix:** Update the `defaultWorkspaceID` comment to note it is now reached only via
`resolveCreateWorkspaceID`, not directly from both create paths.

### IN-02: `workspace!.id` non-null assertion in WorkspaceNameDialog

**File:** `web/src/components/sidebar/WorkspaceNameDialog.tsx:68`

**Issue:** `renameWorkspace.mutateAsync({ id: workspace!.id, ... })` uses a non-null
assertion. It is safe by construction (rename mode is only opened with a target), but
the `workspace?` optional prop plus the `!` is a foot-gun: a future caller that opens
`mode="rename"` without a `workspace` would throw at runtime rather than being caught by
the type system.

**Fix:** Guard explicitly (`if (mode === "rename" && !workspace) return;`) or make the
prop required when `mode === "rename"` via a discriminated-union props type.

### IN-03: Duplicated client sort diverges from backend ordering

**File:** `web/src/components/sidebar/WorkspaceSwitcher.tsx:42-44`, `ManageWorkspacesDialog.tsx:72-74`, `ProjectMenu.tsx:76-78`

**Issue:** Three components independently re-sort workspaces with
`a.name.localeCompare(b.name)`. The backend already returns them `ORDER BY name COLLATE
NOCASE`. `localeCompare` (locale-dependent) and SQLite `COLLATE NOCASE` (ASCII
case-fold) can order some names differently, so the switcher/manage/move lists may not
match each other or the sidebar in edge cases. Minor, and the defensive re-sort is
reasonable, but the logic is copy-pasted three times.

**Fix:** Extract a single `sortByName` helper (or a `useSortedWorkspaces` selector) and
reuse it, so ordering is defined once.

### IN-04: Server vs client pluralization divergence in the non-empty-workspace refusal

**File:** `internal/api/workspaces.go:220` vs `web/src/components/sidebar/ManageWorkspacesDialog.tsx:108`

**Issue:** The server refusal reads `"move or remove its N project(s) first"` while the
client tooltip renders proper pluralization `"Move or remove its N project/projects
first."`. Both are correct, but the user can see two different phrasings for the same
condition depending on whether the message comes from the client guard or a server 409.
Cosmetic only.

**Fix:** Align the copy (either pluralize server-side or let the client always
re-derive), so the two surfaces read identically.

---

_Reviewed: 2026-07-06T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
