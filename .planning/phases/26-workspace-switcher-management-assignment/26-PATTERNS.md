# Phase 26: Workspace Switcher, Management & Assignment - Pattern Map

**Mapped:** 2026-07-05
**Files analyzed:** 14 (5 new, 9 modified)
**Analogs found:** 14 / 14 (every file has a concrete in-repo analog — this is an established codebase; almost nothing is greenfield)

> Everything below is a real excerpt from the current tree with real file paths + line numbers. `workspace_id` already flows on the projects wire (Phase 25) — the backend scan/columns are done; this phase adds the *workspaces* CRUD surface, the transfer PATCH, and all the UI.

---

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/api/workspaces.go` (MOD) | controller/handler | CRUD / request-response | `internal/api/settings.go` (struct shape) + `internal/api/projects.go` (scan/columns/RETURNING) | role-match (mix of two) |
| `internal/api/routes.go` (MOD) | route | request-response | itself (existing project route block) | exact (self) |
| `internal/api/projects.go` (MOD) | controller/handler | CRUD | itself (`update()` partial-PATCH + the two INSERTs) | exact (self) |
| `internal/api/workspaces_test.go` (MOD) | test | CRUD | `internal/api/projects_test.go` (`newTestServer`/`doJSON`/`doJSONList`) | role-match |
| `web/src/api/types.ts` (MOD) | model/type | — | itself (`Project` interface) | exact (self) |
| `web/src/api/queries.ts` (MOD) | store/query hook | request-response | `useProjects()` (same file) | exact |
| `web/src/api/mutations.ts` (MOD) | store/mutation hook | CRUD | `useCreateProject`/`useRenameProject`/`useDeleteProject`/`useUpdateProjectSettings` (same file) | exact |
| `web/src/components/sidebar/WorkspaceNameDialog.tsx` (NEW) | component | request-response | `web/src/components/sidebar/RenameProjectDialog.tsx` | exact (near-verbatim clone) |
| `web/src/components/sidebar/WorkspaceSwitcher.tsx` (NEW) | component | event-driven | `web/src/components/sidebar/ProjectMenu.tsx` (DropdownMenu) + `ProjectSidebar.tsx` (expanded-only idiom) | role-match |
| `web/src/components/sidebar/ManageWorkspacesDialog.tsx` (NEW) | component | CRUD | `RenameProjectDialog.tsx` (Dialog shell) + `ProjectMenu.tsx` (AlertDialog delete) | role-match |
| `web/src/components/sidebar/ProjectMenu.tsx` (MOD) | component | event-driven | itself (existing `DropdownMenu`) | exact (self) |
| `web/src/components/sidebar/ProjectSidebar.tsx` (MOD) | component | request-response | itself (project-row map + `waitingByProject` derivation) | exact (self) |
| `web/src/App.tsx` (MOD) | route/provider | request-response | itself (`RedirectToFirstProject`) | exact (self) |
| `web/src/lib/useActiveWorkspace.ts` (NEW, Claude's Discretion) | hook | — | `web/src/components/layout/AppLayout.tsx` (localStorage-backed state) + `web/src/lib/migrateStorage.ts` (`kamacu.*` key convention) | role-match |

---

## Pattern Assignments

### `internal/api/workspaces.go` (handler, CRUD) — ADD the CRUD handlers (D-16)

The file already exists (holds `BackfillWorkspaces`). Add a `workspaceHandlers` struct + `list`/`create`/`update`(rename)/`delete`. Two analogs combine:

**(a) Handler-struct + route-registration shape — from `internal/api/settings.go:31-38`** (the simplest db-only handler; workspaces need none of `projects.go`'s worktree/session deps):
```go
// SettingsRoutes registers the global settings endpoints.
func SettingsRoutes(mux *http.ServeMux, db *sql.DB) {
	h := &settingsHandlers{db: db}
	mux.HandleFunc("GET /api/settings", h.getAll)
	mux.HandleFunc("PUT /api/settings/{key}", h.put)
}

type settingsHandlers struct{ db *sql.DB }
```
→ `type workspaceHandlers struct{ db *sql.DB }`. (Per D-16 the routes register in the main `Routes()` in `routes.go`, not a separate `WorkspaceRoutes` — but this is the struct shape to mirror.)

**(b) `columns` const + `scanX` + list loop — from `internal/api/projects.go:97-142`:**
```go
const projectColumns = `id, name, repo_path, description, github_repo, managed, icon_letters, icon_color, workspace_id, created_at, updated_at`

func scanProject(row interface{ Scan(...any) error }) (Project, error) {
	var p Project
	// ...
	err := row.Scan(&p.ID, &p.Name, /* ... */ &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func (h *projectHandlers) list(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`SELECT ` + projectColumns + ` FROM projects ORDER BY name COLLATE NOCASE`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	projects := []Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }
	writeJSON(w, http.StatusOK, projects)
}
```
→ `const workspaceColumns = ` + "`id, name, is_default, created_at, updated_at`" + `; scanWorkspace(...)` scanning `is_default` as `int` → `bool` (mirror the `managedInt` bool-mapping at `projects.go:106,116`, since modernc returns INTEGER 0/1). `Workspace` JSON struct mirrors `Project` at `projects.go:26-54` (`json:"is_default"`). List `ORDER BY name COLLATE NOCASE` (WSNAV-03 "first project by name" depends on projects being name-sorted, already true at `projects.go:122`).

**Create + RETURNING (rename target too) — from `projects.go:202-209`:**
```go
p, err := scanProject(h.db.QueryRow(
	`INSERT INTO projects (name, repo_path, icon_letters, icon_color, workspace_id) VALUES (?, ?, ?, ?, ?) RETURNING `+projectColumns,
	name, abs, deriveLetters(name), pickColor(), wsID))
if err != nil {
	writeError(w, http.StatusInternalServerError, err.Error())
	return
}
writeJSON(w, http.StatusCreated, p)
```
→ workspace create = `INSERT INTO workspaces (name) VALUES (?) RETURNING ` + workspaceColumns. The `name UNIQUE COLLATE NOCASE` index (migration `00012:24`) makes a duplicate INSERT fail; catch that DB error and surface it as the `{"error": msg}` the frontend renders inline (see Shared Pattern "Error surface"). Decide 409 vs 422 (Claude's Discretion, D per CONTEXT.md) — the existing folder-create duplicate uses **409** (`projects.go:182`).

**Rename = partial-ish PATCH with RETURNING — from `projects.go:369-476`** (the `update` handler): decode into a struct, `strings.TrimSpace`, `writeError(400, "name is required")` on empty (`projects.go:393-401`), then `UPDATE ... SET name = ?, updated_at = strftime(...) WHERE id = ? RETURNING` + columns, mapping `sql.ErrNoRows` → 404 (`projects.go:467-470`). Rename must work for the default/Personal workspace too (WSMGMT-02) — do NOT gate rename on `is_default`.

**Guarded delete (the phase's core new logic, D-05/D-06) — pattern base from `projects.go:543-576`:**
```go
func (h *projectHandlers) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok { return }
	// Load a marker column first, then branch on it (never derive from path).
	var managed int
	var clone string
	err := h.db.QueryRow(`SELECT managed, repo_path FROM projects WHERE id = ?`, id).Scan(&managed, &clone)
	if errors.Is(err, sql.ErrNoRows) { writeError(w, http.StatusNotFound, "project not found"); return }
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }
	// ... branch on the marker ...
	res, err := h.db.Exec(`DELETE FROM projects WHERE id = ?`, id)
	if n, _ := res.RowsAffected(); n == 0 { writeError(w, http.StatusNotFound, "project not found"); return }
	w.WriteHeader(http.StatusNoContent)
}
```
For workspace delete, the two guards mirror this "load the marker column, then branch" idiom:
1. **`is_default` guard (D-06/WSMGMT-04):** `SELECT is_default FROM workspaces WHERE id = ?`; if `1` → refuse. (Never key off the name "Personal" — rename-proof, Phase 25 D-02.)
2. **Non-empty guard (D-05):** `SELECT COUNT(*) FROM projects WHERE workspace_id = ?`; if `> 0` → refuse. The DB `ON DELETE RESTRICT` (migration `00012:31`) is the ultimate backstop but a plain `DELETE` would surface a raw FK error — do the explicit count first so the refusal carries a clean message. On all-clear: `DELETE FROM workspaces WHERE id = ?` → `204` via `w.WriteHeader(http.StatusNoContent)`.

Use `pathID(w, r)` (`projects.go:833-840`) verbatim for `{id}` parsing.

---

### `internal/api/projects.go` (handler, CRUD) — extend for transfer + active-workspace-at-create

**Transfer via optional `workspace_id` on the existing PATCH (D-17)** — extend the partial-PATCH `update` at `projects.go:369-476`. The exact pointer-field + conditional-`sets` idiom to copy (lines 374-410, 462-475):
```go
var req struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	GithubRepo  *string `json:"github_repo"`
	IconLetters *string `json:"icon_letters"`
	IconColor   *string `json:"icon_color"`
}
// ... decode; reject "nothing to update" if all nil ...
var sets []string
var args []any
if req.Name != nil {
	name := strings.TrimSpace(*req.Name)
	if name == "" { writeError(w, http.StatusBadRequest, "name is required"); return }
	sets = append(sets, "name = ?")
	args = append(args, name)
}
// ... more fields ...
sets = append(sets, "updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')")
args = append(args, id)
query := `UPDATE projects SET ` + strings.Join(sets, ", ") + ` WHERE id = ? RETURNING ` + projectColumns
p, err := scanProject(h.db.QueryRow(query, args...))
if errors.Is(err, sql.ErrNoRows) { writeError(w, http.StatusNotFound, "project not found"); return }
```
→ add `WorkspaceID *int64 `+"`json:\"workspace_id\"`"+`` to `req`; when non-nil, validate the target exists (`SELECT 1 FROM workspaces WHERE id = ?` → `writeError(400, ...)` if absent, mirroring the `github_repo` validate-then-append reject at `projects.go:410-436`), then `sets = append(sets, "workspace_id = ?")`. Add it to the all-nil "nothing to update" check at `projects.go:385-388`.

**Active-workspace-at-create (D-10/WSPROJ-02)** — refine BOTH INSERT sites. Today they call `h.defaultWorkspaceID()`:
- folder path: `projects.go:197-204`
- repo-first path: `projects.go:309-316`
- helper: `projects.go:217-221`:
```go
func (h *projectHandlers) defaultWorkspaceID() (int64, error) {
	var id int64
	err := h.db.QueryRow(`SELECT id FROM workspaces WHERE is_default = 1`).Scan(&id)
	return id, err
}
```
→ the create `req` struct (`projects.go:158-162`) gains an optional `WorkspaceID *int64 `+"`json:\"workspace_id\"`"+``; when the client sends it, validate it exists and use it; when omitted/nil, fall back to `defaultWorkspaceID()` (keep it exactly as the fallback — D-14 baseline: missing → default). Both paths must set the same resolved `wsID`.

---

### `internal/api/routes.go` (route) — register the 4 workspace endpoints (D-16)

Add alongside the project routes (`routes.go:19-26`):
```go
p := &projectHandlers{db: db, wt: wt, mgr: mgr, tmuxClient: tmuxClient}
mux.HandleFunc("GET /api/projects", p.list)
mux.HandleFunc("POST /api/projects", p.create)
mux.HandleFunc("PATCH /api/projects/{id}", p.update)
mux.HandleFunc("DELETE /api/projects/{id}", p.delete)
```
→ construct `wh := &workspaceHandlers{db: db}` and register:
```
GET    /api/workspaces
POST   /api/workspaces
PATCH  /api/workspaces/{id}
DELETE /api/workspaces/{id}
```
Method+wildcard ServeMux patterns, same `{id}` wildcard as the project routes. No change to `BackfillWorkspaces` wiring (already called in `cmd/kamacu/main.go:134`).

---

### `internal/api/workspaces_test.go` (test, CRUD) — add handler tests

The file exists (`TestBackfillWorkspaces`, a `store.Open`+`store.Migrate` harness). For the HTTP handlers, use the fuller harness from `projects_test.go`:

**`newTestServer` (`projects_test.go:430-452`)** — spins a real `httptest.Server` over `Routes(...)` on a migrated temp DB (already seeds Personal via migration 00012):
```go
func newTestServer(t *testing.T) (*httptest.Server, *sql.DB, string) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, _ := store.Open(dbPath)
	store.Migrate(db)
	// ...
	mux := http.NewServeMux()
	Routes(mux, db, worktree.NewService(wtDir), session.NewManager(), tmux.Client{})
	srv := httptest.NewServer(mux)
	t.Cleanup(func() { srv.Close(); db.Close() })
	return srv, db, dbPath
}
```
**`doJSON` (`projects_test.go:467-497`)** and `doJSONList` (line 499) are the request helpers: `status, body := doJSON(t, "POST", srv.URL+"/api/workspaces", map[string]any{"name": "Work"})`. Test the guards: create-duplicate → the conflict status; delete non-empty → refuse (seed a project via `gitRepo(t)` at `projects_test.go:454-463`); delete `is_default` → refuse; rename Personal → OK.

---

### `web/src/api/types.ts` (type) — add `workspace_id` + `Workspace`

`Project` is at `types.ts:12-29` and is **missing `workspace_id`** even though the backend already serializes it. Add the field (mirror the existing `icon_letters`/`icon_color` non-null comment style at `types.ts:23-26`):
```ts
export interface Project {
  id: number;
  name: string;
  // ...
  icon_letters: string;
  icon_color: string;
  workspace_id: number; // ADD — v1.8 FK (migration 00012), always present on the wire
  created_at: string;
  updated_at: string;
}
```
Add a new interface mirroring the backend `Workspace` struct:
```ts
export interface Workspace {
  id: number;
  name: string;
  is_default: boolean;
  created_at: string;
  updated_at: string;
}
```

---

### `web/src/api/queries.ts` (query hook) — add `useWorkspaces()`

Verbatim shape from `useProjects()` (`queries.ts:5-10`):
```ts
export function useProjects() {
  return useQuery({
    queryKey: ["projects"],
    queryFn: () => get<Project[]>("/api/projects"),
  });
}
```
→
```ts
export function useWorkspaces() {
  return useQuery({
    queryKey: ["workspaces"],
    queryFn: () => get<Workspace[]>("/api/workspaces"),
  });
}
```

---

### `web/src/api/mutations.ts` (mutation hooks) — add workspace CRUD + transfer

Copy the exact `useMutation` + `invalidateQueries` shape from the project mutations (`mutations.ts:7-76`):
```ts
export function useCreateProject() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { name?: string; repo_path?: string; repo?: string }) =>
      post<Project>("/api/projects", body),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["projects"] }); },
  });
}
export function useRenameProject() {
  // patch<Project>(`/api/projects/${id}`, { name }) → invalidate ["projects"]
}
export function useDeleteProject() {
  // del(`/api/projects/${id}`) → invalidate ["projects"]
}
```
→ `useCreateWorkspace` / `useRenameWorkspace` / `useDeleteWorkspace` all invalidate `["workspaces"]`. **`useMoveProject` (transfer)** = a PATCH to the *project* endpoint that must invalidate **`["projects"]`** (the sidebar list) — model the conditional-body idiom on `useUpdateProjectSettings` (`mutations.ts:32-66`):
```ts
mutationFn: ({ id, workspace_id }: { id: number; workspace_id: number }) =>
  patch<Project>(`/api/projects/${id}`, { workspace_id }),
onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["projects"] }); },
```
`del`/`patch`/`post` come from `@/api/client` (`client.ts:41-67`); `ApiError` (`client.ts:1-9`) is what the dialogs catch.

---

### `web/src/components/sidebar/WorkspaceNameDialog.tsx` (component) — create/rename dialog (D-04)

**Near-verbatim clone of `RenameProjectDialog.tsx` (whole file, 99 lines).** Copy its structure exactly — controlled `name` state, prefill-on-open, `ApiError` → `sentenceCase` inline error, disabled-while-pending Save:
```tsx
function sentenceCase(message: string): string {
  return message.charAt(0).toUpperCase() + message.slice(1);
}
// prefill each open:
useEffect(() => { if (open) { setName(project.name); setError(null); } }, [open, project.name]);
// submit:
try {
  await renameProject.mutateAsync({ id: project.id, name: name.trim() });
  onOpenChange(false);
} catch (err) {
  setError(err instanceof ApiError ? sentenceCase(err.message) : "Couldn't save. Try again.");
}
// markup:
<Dialog open={open} onOpenChange={onOpenChange}>
  <DialogContent aria-describedby={undefined}>
    <DialogHeader><DialogTitle>Rename project</DialogTitle></DialogHeader>
    <form onSubmit={handleSubmit} className="flex flex-col gap-4">
      <div className="flex flex-col gap-1.5">
        <label className="text-xs font-medium">Name</label>
        <Input value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        {error && <p className="text-xs text-destructive">{error}</p>}
      </div>
      <DialogFooter>
        <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>Cancel</Button>
        <Button type="submit" disabled={renameProject.isPending || name.trim() === ""}>Save</Button>
      </DialogFooter>
    </form>
  </DialogContent>
</Dialog>
```
Differences: mode prop (create=empty field + title "New workspace" + button "Create"; rename=prefilled + "Rename workspace" + "Save" — copy from UI-SPEC Copywriting Contract); calls `useCreateWorkspace`/`useRenameWorkspace`. The `name UNIQUE COLLATE NOCASE` server reject arrives as `ApiError.message` and renders in the same `text-xs text-destructive` line (UI-SPEC: "A workspace with that name already exists.").

---

### `web/src/components/sidebar/WorkspaceSwitcher.tsx` (component) — the dropdown trigger (D-01/D-02)

Two analogs:

**(a) DropdownMenu composition — from `ProjectMenu.tsx:53-76`:**
```tsx
<DropdownMenu>
  <DropdownMenuTrigger asChild>
    <SidebarMenuAction showOnHover aria-label={`Project menu: ${project.name}`}>
      <MoreHorizontal className="size-4" />
    </SidebarMenuAction>
  </DropdownMenuTrigger>
  <DropdownMenuContent side="bottom" align="start">
    <DropdownMenuItem onSelect={() => setRenameOpen(true)}>Rename</DropdownMenuItem>
    <DropdownMenuItem variant="destructive" onSelect={() => setDeleteOpen(true)}>Delete project</DropdownMenuItem>
  </DropdownMenuContent>
</DropdownMenu>
```
→ trigger is a full-width `min-h-7 px-3 text-sm font-medium` name+chevron button (UI-SPEC Interaction table), content lists workspaces as `DropdownMenuCheckboxItem` (the ✓ active marker, D-02), a `DropdownMenuSeparator`, then `DropdownMenuItem`s "New workspace" (lucide `Plus`) + "Manage workspaces…" (lucide `Settings`). All of `DropdownMenuCheckboxItem`, `DropdownMenuSeparator`, `DropdownMenuSub*`, `DropdownMenuRadio*` are already exported (`web/src/components/ui/dropdown-menu.tsx:253-269`) — no new primitive.

**(b) Expanded-only rendering — the brand-lockup idiom from `ProjectSidebar.tsx:53-58`:**
```tsx
<span className="flex items-center gap-2 px-1 group-data-[collapsible=icon]:hidden">
  <KamacuMark className="size-5 shrink-0" />
  <span className="text-sm font-medium group-data-[collapsible=icon]:hidden">Kamacu</span>
</span>
```
→ wrap the whole switcher in `group-data-[collapsible=icon]:hidden` so it vanishes in the icon rail (WSNAV-01). Active/hover fill = `bg-sidebar-accent` (UI-SPEC Color — same treatment as the selected project row).

---

### `web/src/components/sidebar/ManageWorkspacesDialog.tsx` (component) — rename/delete hub (D-03/D-07)

Compose the `Dialog` shell (from `RenameProjectDialog.tsx:59-97`) with one row per workspace. Each row's **delete** reuses the AlertDialog confirm from `ProjectMenu.tsx:90-105`:
```tsx
<AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
  <AlertDialogContent>
    <AlertDialogHeader>
      <AlertDialogTitle>Delete project?</AlertDialogTitle>
      <AlertDialogDescription>
        {`"${project.name}" and all its tasks will be removed...`}
      </AlertDialogDescription>
    </AlertDialogHeader>
    <AlertDialogFooter>
      <AlertDialogCancel>Cancel</AlertDialogCancel>
      <AlertDialogAction variant="destructive" onClick={handleDelete}>Delete project</AlertDialogAction>
    </AlertDialogFooter>
  </AlertDialogContent>
</AlertDialog>
```
→ title "Delete workspace?", action "Delete workspace" (UI-SPEC copy). The **disabled-delete guards** (D-05 non-empty, D-06 default) compute the per-workspace project count **client-side** from `useProjects()` filtered by `workspace_id` (mirror the in-memory `waitingByProject` derivation at `ProjectSidebar.tsx:36-44`). Disabled control + `Tooltip` (`text-muted-foreground`, never `text-destructive`) — UI-SPEC accessibility note: keep it focusable-with-tooltip, don't use raw `disabled` that swallows the tooltip.

---

### `web/src/components/sidebar/ProjectMenu.tsx` (component, MOD) — "Move to workspace" submenu (D-08)

Extend the existing `DropdownMenuContent` (`ProjectMenu.tsx:62-75`) with a submenu using the already-exported primitives:
```tsx
<DropdownMenuSub>
  <DropdownMenuSubTrigger>Move to workspace</DropdownMenuSubTrigger>
  <DropdownMenuSubContent>
    <DropdownMenuRadioGroup value={String(project.workspace_id)}>
      {workspaces.map((ws) => (
        <DropdownMenuRadioItem
          key={ws.id}
          value={String(ws.id)}
          disabled={ws.id === project.workspace_id}
          onSelect={() => moveProject.mutate({ id: project.id, workspace_id: ws.id })}
        >
          {ws.name}
        </DropdownMenuRadioItem>
      ))}
    </DropdownMenuRadioGroup>
  </DropdownMenuSubContent>
</DropdownMenuSub>
```
The current workspace = checked + disabled (D-08). **Follow-the-project (D-09):** this file already imports `useNavigate`/`useParams` (`ProjectMenu.tsx:2`) and knows `projectId` — when the moved project is the currently-open one, set active workspace to the target after mutate (mirror the `isCurrent`/`refetchQueries`/`navigate("/")` delete flow at `ProjectMenu.tsx:40-49`). Calls `useMoveProject` + `useWorkspaces`.

---

### `web/src/components/sidebar/ProjectSidebar.tsx` (component, MOD) — mount switcher + filter list (D-12)

- **Mount** `<WorkspaceSwitcher />` at the top of `SidebarContent` (or `SidebarHeader`), expanded-only via the `group-data-[collapsible=icon]:hidden` idiom already in this file (`ProjectSidebar.tsx:53`).
- **Filter** the project map (`ProjectSidebar.tsx:69` `(projects ?? []).map(...)`) to the active workspace: `(projects ?? []).filter((p) => p.workspace_id === activeWorkspaceId).map(...)`. This filters BOTH surfaces at once — the expanded rows and the collapsed `size-10` rail avatars are the same `.map` (WSNAV-02). Client-side, same in-memory approach as `waitingByProject` (`ProjectSidebar.tsx:36-44`). The active id comes from the `useActiveWorkspace` hook.

---

### `web/src/App.tsx` (route, MOD) — workspace-aware redirect + empty state (D-11/D-13/D-14)

`RedirectToFirstProject` (`App.tsx:12-35`) currently redirects to `projects[0]` globally:
```tsx
function RedirectToFirstProject() {
  const { data: projects, isLoading } = useProjects();
  if (isLoading) return null;
  if (projects && projects.length > 0) {
    return <Navigate to={`/projects/${projects[0].id}`} replace />;
  }
  return (/* "No projects yet" empty state + AddProjectDialog */);
}
```
→ filter `projects` to the active workspace before picking `[0]` (D-11: first by name, and the list is already name-sorted backend-side). Empty state becomes workspace-scoped: "No projects in {workspace} yet" + "Add project" that creates into the active workspace (D-13) — reuse the exact empty-state layout at `App.tsx:21-33`. **D-14 URL-wins** lives on the board route: when a `/projects/:projectId` deep-link resolves to a project whose `workspace_id ≠` the saved active id, set active to that project's workspace (+ update localStorage). Keep routes `/projects/:projectId` unchanged — workspace stays out of the URL.

---

### `web/src/lib/useActiveWorkspace.ts` (hook, NEW — Claude's Discretion)

State shape is Claude's Discretion (localStorage-backed hook vs context vs zustand). The **localStorage-backed `useState` idiom** from `AppLayout.tsx:8-21` is the template:
```ts
const SIDEBAR_STORAGE_KEY = "kamacu.sidebar";
const [open, setOpen] = useState(
  () => localStorage.getItem(SIDEBAR_STORAGE_KEY) !== "false",
);
function handleOpenChange(next: boolean) {
  setOpen(next);
  localStorage.setItem(SIDEBAR_STORAGE_KEY, String(next));
}
```
→ key `kamacu.workspace` (the `kamacu.*` convention documented in `migrateStorage.ts:22-41`; existing keys: `kamacu.sidebar`, `kamacu:sessions-bar-collapsed`, `kamacu:review-collapsed:${projectId}`). Baseline (D-14): a stale/missing saved id falls back to the `is_default` workspace — so the hook needs `useWorkspaces()` to validate the saved id against live rows and pick `is_default` when absent. This hook is consumed by the switcher, the sidebar filter (`ProjectSidebar`), `App.tsx`'s redirect, and the transfer follow logic (`ProjectMenu`) — a shared module (or a context provider mounted in `AppLayout`) keeps them in sync.

---

## Shared Patterns

### Error surface (backend → inline frontend text)
**Source:** `internal/api/respond.go:9-18` + `web/src/api/client.ts:19-28`
The backend writes `{"error": msg}`; the frontend `api()` extracts `body.error` into `ApiError.message`; dialogs render it `sentenceCase`'d in `<p className="text-xs text-destructive">`.
```go
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
```
```ts
const body = (await res.json()) as { error?: string };
if (body.error) message = body.error;
throw new ApiError(message, res.status);
```
**Apply to:** all workspace handlers (duplicate-name, non-empty-delete, default-delete, not-found) + `WorkspaceNameDialog` + `ManageWorkspacesDialog`. No toast library exists — feedback is inline/tooltip/dialog only (UI-SPEC).

### Single-writer + parameterized SQL
**Source:** `internal/store/store.go` (`SetMaxOpenConns(1)`, `foreign_keys(1)`) + every query in `projects.go`
All new workspace SQL uses `?` placeholders (never string-concatenated input); if a `SELECT` cursor is open, collect-then-write (never write while iterating `rows.Next()`). The `ON DELETE RESTRICT` FK (`00012:31`) bites because `foreign_keys` is ON.

### TanStack Query invalidation on mutation
**Source:** `web/src/api/mutations.ts:15-18`
```ts
onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["projects"] }); },
```
**Apply to:** workspace create/rename/delete → invalidate `["workspaces"]`; transfer + create-in-active → invalidate `["projects"]` (the sidebar list is keyed `["projects"]`, `queries.ts:7`).

### Partial-PATCH contract (omitted key = untouched)
**Source:** `internal/api/projects.go:369-476` (server) + `web/src/api/mutations.ts:51-60` (client builds body conditionally)
**Apply to:** the transfer `workspace_id` on `PATCH /api/projects/{id}` (D-17) and the optional `workspace_id` at create (D-10).

### Expanded-only sidebar rendering
**Source:** `web/src/components/sidebar/ProjectSidebar.tsx:53` (`group-data-[collapsible=icon]:hidden`)
**Apply to:** `WorkspaceSwitcher` (WSNAV-01 — hidden in the icon rail). NOT applied to the project-list filter, which runs in both states.

### `is_default`-keyed protection (never name-keyed)
**Source:** `internal/api/projects.go:217-221` (`WHERE is_default = 1`) + `internal/api/workspaces.go:27`
**Apply to:** the default-workspace delete guard (D-06) and the create/redirect fallback (D-14). Rename-proof — never compare to the string "Personal".

---

## No Analog Found

None. Every file in this phase has a concrete in-repo analog (this is an established phase-26 codebase). The single "greenfield-ish" file — `useActiveWorkspace.ts` — still has a direct idiom analog (`AppLayout.tsx` localStorage-backed state) and a naming-convention analog (`migrateStorage.ts`); only its precise shape is Claude's Discretion per CONTEXT.md.

### Guardrail (non-regression, WSBAR-01 / D-15) — DO NOT touch
The global Active Sessions bar reads its own independent poll and must stay cross-workspace:
- `web/src/api/agents.ts:20-26` — `useAgentStatuses()` (`queryKey: ["agent-statuses"]`, `GET /api/agents/status`, 5s poll). Do NOT thread a `workspace_id` filter into this query.
- `web/src/components/layout/ActiveSessionsBar.tsx` — mounted globally in `AppLayout.tsx:48`, outside `<Outlet/>`. Leave it unfiltered. The board's `useProjects()`/active-workspace filter must not reach it.

---

## Metadata

**Analog search scope:** `internal/api/` (handlers, routes, respond, tests, migrations), `internal/store/migrations/`, `web/src/api/`, `web/src/components/sidebar/`, `web/src/components/layout/`, `web/src/lib/`, `web/src/App.tsx`.
**Files scanned/read:** ~20 (projects.go, workspaces.go, routes.go, settings.go, respond.go, projects_test.go, workspaces_test.go, migration 00012; RenameProjectDialog, ProjectMenu, ProjectSidebar, AddProjectDialog, App, AppLayout, agents.ts, queries.ts, mutations.ts, client.ts, types.ts, migrateStorage.ts, dropdown-menu exports).
**Pattern extraction date:** 2026-07-05
