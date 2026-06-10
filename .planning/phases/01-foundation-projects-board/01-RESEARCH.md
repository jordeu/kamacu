# Phase 1: Foundation — Projects & Board - Research

**Researched:** 2026-06-10
**Domain:** Single-binary Go web server (embedded React SPA) + SQLite-backed kanban CRUD
**Confidence:** HIGH (all versions re-verified against npm registry and Go module proxy on research date; library APIs verified against official docs)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Visual Direction**
- **D-01:** Dark-first theme. No light mode in v1 — terminals (coming in Phase 2) should feel native against the UI.
- **D-02:** Dense developer-tool aesthetic (Linear/Raycast-style): compact spacing, small text, information-dense, keyboard-friendly.
- **D-03:** Mostly neutral palette (greys/dark neutrals); color is reserved for status accents (session status badges arriving in Phase 4). Columns are NOT individually colored.
- **D-04:** Collapsible left sidebar — toggles to give the board/terminal more space.

**Task View Presentation**
- **D-05:** Clicking a task navigates to a full-page task route; the sidebar stays visible. Each task is URL-addressable (deep-linkable). Modal/drawer rejected — terminals in later phases need maximum space.
- **D-06:** Task view uses a single tab strip where Description is a tab. Future phases append Agent and Bash tabs to the same strip. Phase 1 ships the tab container with just the Description tab.
- **D-07:** Back to board via explicit back button + Esc shortcut; browser back also works (routing-based).

**Task Creation & Board Flow**
- **D-08:** Two creation paths: inline "+ New task" quick-add at the top of the To Do column (title only) AND a New Task button opening a full dialog (title + markdown description).
- **D-09:** Manual drag ordering within columns; position persists in DB. New tasks land at the top of their column.
- **D-10:** Markdown description uses an edit/preview toggle: plain textarea when editing, rendered markdown when viewing, explicit edit button. No side-by-side live preview.
- **D-11:** Task deletion is a hard delete behind a confirmation dialog. No archive/soft-delete in v1.

### Claude's Discretion
- Project setup UX: how the repo path is entered/validated, default project name (e.g., directory name), invalid-path error presentation.
- Server startup behavior: port choice, flags, whether to auto-open browser.
- Keyboard shortcuts beyond Esc (add where they're cheap and obvious; dense-tool aesthetic implies keyboard-friendliness).
- Exact component library usage (research recommends shadcn/ui + Tailwind 4 — follow research unless something conflicts).
- Empty states (no projects yet, empty column) — design something sensible and unobtrusive.

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope.

**Scope guard (from phase description):** No git operations beyond repo-path validation, no terminals, no sessions. Task creation does NOT create worktrees. Schema leaves room for later phases without building them.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| STOR-01 | Projects/tasks persist in local SQLite across restarts | modernc.org/sqlite DSN + pragmas + MaxOpenConns(1) (verified syntax below); goose embedded migrations |
| STOR-02 | Single local process serves API + frontend at localhost | `//go:embed all:dist` + SPA fallback handler; bind 127.0.0.1; Vite dev proxy for development |
| PROJ-01 | Create project pointing at local git repo, path validated | Plain-text path input (browsers cannot supply absolute paths); server-side `git rev-parse --git-dir` validation; default name = `filepath.Base` |
| PROJ-02 | Projects in left sidebar, click to switch board | shadcn/ui Sidebar component (collapsible, satisfies D-04); react-router route `/projects/{id}` |
| PROJ-03 | Edit project name, delete project (tasks removed, repo untouched) | PATCH/DELETE endpoints; `ON DELETE CASCADE` on tasks.project_id; AlertDialog confirm |
| TASK-01 | Create task with title + markdown description | POST endpoint; quick-add (title only) + dialog (title+description) per D-08 |
| TASK-02 | Kanban board with fixed columns To Do/In Progress/In Review/Done | dnd-kit multi-container pattern (code example below); status as TEXT enum |
| TASK-03 | Drag between columns, status persists | dnd-kit onDragOver/onDragEnd + `POST /api/tasks/{id}/move` with server-computed fractional position |
| TASK-04 | Edit title/description, delete task | PATCH/DELETE endpoints; markdown edit/preview toggle per D-10; confirm dialog per D-11 |
| TASK-05 | Click task → expanded task view with tab strip | Full-page route `/projects/{pid}/tasks/{tid}`; shadcn Tabs with Description tab only; react-markdown rendering |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **GSD workflow enforcement:** file changes go through GSD commands (`/gsd:execute-phase` for this work).
- **Stack is locked by project research embedded in CLAUDE.md:** Go 1.26 + stdlib ServeMux, modernc.org/sqlite, goose, Vite + React 19 + Tailwind 4 + shadcn/ui, dnd-kit, TanStack Query. Do not substitute.
- **User global rule:** never mention or co-author Claude (or happy-otter) on commits or PRs.
- Deployment constraint: single binary, localhost-only, SQLite, no external services.

## Summary

This phase is conventional CRUD on a well-trodden stack — the risk is not "can it be built" but "are the foundations laid correctly," because every later phase builds on the project layout, DB discipline, API shape, and frontend structure established here. Three things must be right from the first commit: (1) SQLite opened with WAL + busy_timeout + foreign_keys pragmas and `SetMaxOpenConns(1)` (PITFALLS.md Pitfall 9 — "10 lines if done first, a debugging week if done later"); (2) the single-binary embed pattern with a Vite dev proxy so dev and prod paths both work; (3) the task-view tab strip as a real container component, since it is the architectural seam Phases 2–5 extend.

All library versions were re-verified today against the npm registry and Go module proxy and match the project STACK.md (researched the same day). Two phase-specific design decisions were resolved: manual ordering uses a **REAL fractional position column with a server-computed move endpoint** (`POST /api/tasks/{id}/move` with `{status, after_id}`) — the server owns float math and renormalization, the client only expresses intent; and **repo-path entry is a plain text input validated server-side** — browsers deliberately cannot give absolute filesystem paths to web pages, so a native picker is impossible in a web app, and `git -C <path> rev-parse --git-dir` is the correct validation (handles `.git`-as-file cases that a naive `.git` directory check misses).

One environment note: the machine has Go 1.24.3, but `GOTOOLCHAIN=auto` (confirmed on this machine) means a `go 1.26` directive in go.mod auto-downloads the 1.26 toolchain on first build. Node 24.4.1 satisfies Vite 8's requirement. No blocking gaps.

**Primary recommendation:** Build backend-first (module + SQLite + migrations + projects/tasks handlers with the move endpoint), then the frontend shell (Vite + Tailwind 4 + shadcn + router + sidebar), then the board (dnd-kit + TanStack Query optimistic moves), then the task view (tabs + markdown). Wire embed + Makefile early so the single-binary success criterion is proven before UI polish.

## Standard Stack

All versions verified against npm registry / Go module proxy on 2026-06-10.

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go | 1.26 (go.mod directive) | Backend | Project decision (STACK.md). Local Go 1.24.3 auto-fetches the toolchain (GOTOOLCHAIN=auto verified) |
| `net/http` ServeMux | stdlib | Routing | Method + wildcard patterns (`GET /api/projects/{id}`) cover the whole API; zero deps |
| `modernc.org/sqlite` | v1.52.0 | SQLite driver | Pure Go, CGO-free single binary. Requires Go ≥ 1.25 (verified from its go.mod) |
| `github.com/pressly/goose/v3` | v3.27.1 | Migrations | Library mode + `embed.FS`; run `goose.Up` at startup. Requires Go ≥ 1.25.7 |
| `embed` + `http.FileServerFS` | stdlib | Ship SPA in binary | `//go:embed all:dist` + SPA fallback handler |
| React | 19.2.7 | UI | Project decision |
| Vite | 8.0.16 | Build/dev server | Needs Node 20.19+/22.12+ (local: 24.4.1 ✓) |
| `@vitejs/plugin-react` | 6.0.2 | Fast refresh | Standard companion |
| TypeScript | 6.0.3 | Types | Default |
| Tailwind CSS + `@tailwindcss/vite` | 4.3.0 | Styling | v4 CSS-first config — NO tailwind.config.js; use the Vite plugin, not PostCSS |
| shadcn/ui | CLI latest (`npx shadcn@latest`) | Components | Copied-in components; full Tailwind v4 + React 19 support |
| `@dnd-kit/core` / `@dnd-kit/sortable` / `@dnd-kit/utilities` | 6.3.1 / 10.0.0 / (latest) | Board drag-and-drop | The 2026 React DnD default; multi-container sortable pattern is exactly a kanban |
| `@tanstack/react-query` | 5.101.0 | Server state | Mutations + optimistic updates for drag persistence |
| `react-router` | 7.17.0 | Routing (D-05 URL-addressable tasks) | Use **declarative/library mode** (`BrowserRouter` + `Routes`) — no framework mode, no SSR. Single package `react-router` in v7 (not react-router-dom) |
| `react-markdown` | 10.1.0 | Markdown rendering (D-10) | Renders to React elements, no `dangerouslySetInnerHTML`; raw HTML in markdown is NOT rendered by default — safe without a sanitizer |
| `remark-gfm` | 4.0.1 | GFM (tables, task lists, strikethrough) | Standard companion to react-markdown |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `@tailwindcss/typography` | 0.5.20 | Markdown prose styling | Load via `@plugin "@tailwindcss/typography";` in CSS (v4 style); `prose prose-invert prose-sm` for dense dark markdown |
| `log/slog` | stdlib | Logging | No logging library |
| Makefile | — | `npm run build` → `go build` | One `make build` produces the binary |
| `air` or `wgo` | latest | Go hot reload in dev | Optional convenience |

**Explicitly NOT in this phase:** coder/websocket, creack/pty, xterm packages, zustand (no ephemeral state worth a store yet — sidebar collapse can be a `useState`/localStorage), sqlc (hand-written queries fine at 2 tables).

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| react-router 7 (declarative) | TanStack Router 1.x | Type-safe routes, but heavier setup/codegen for 3 routes; react-router declarative mode is ~zero ceremony. Not worth it here |
| Server-computed move endpoint | Client-computed fractional position in PATCH | Fewer endpoints, but float math + renormalization scattered into the client; server authority keeps ordering logic in one transaction |
| REAL fractional position | Integer positions with gap (1024-spacing) or full reindex per move | Integer reindex = O(n) row updates per drag; gapped integers still need renormalization logic but with more code. REAL midpoint + renormalize-on-exhaustion is the least code at this scale |
| react-markdown | marked / markdown-it + DOMPurify | String-HTML pipeline needs explicit sanitization (XSS risk if forgotten); react-markdown is safe by construction |

**Installation:**

```bash
# Backend (after go mod init github.com/<user>/kangent)
go get modernc.org/sqlite@v1.52.0
go get github.com/pressly/goose/v3@v3.27.1

# Frontend
npm create vite@latest web -- --template react-ts
cd web
npm install @tanstack/react-query @dnd-kit/core @dnd-kit/sortable @dnd-kit/utilities \
  react-router react-markdown remark-gfm
npm install -D tailwindcss @tailwindcss/vite @tailwindcss/typography
npx shadcn@latest init
npx shadcn@latest add button dialog alert-dialog input textarea tabs sidebar \
  dropdown-menu tooltip separator skeleton
```

(`shadcn init` requires the `@/*` path alias in tsconfig.json + tsconfig.app.json and `resolve.alias` in vite.config.ts — set those up first; the shadcn Vite guide documents the exact steps.)

## Architecture Patterns

### Recommended Project Structure (Phase 1 subset of ARCHITECTURE.md layout)

```
kangent/
├── go.mod                    # go 1.26
├── Makefile                  # build: npm build → go build; dev: vite + go run
├── cmd/kangent/
│   └── main.go               # flags, DB open, migrations, mux wiring, ListenAndServe
├── internal/
│   ├── api/                  # HTTP handlers: projects.go, tasks.go, respond.go
│   └── store/                # store.go (Open + pragmas), queries
│       └── migrations/       # 00001_init.sql (embedded)
└── web/
    ├── embed.go              # package web; //go:embed all:dist
    ├── dist/index.html       # COMMITTED placeholder so `go build` never fails
    ├── vite.config.ts        # @tailwindcss/vite plugin + /api proxy + @ alias
    └── src/
        ├── main.tsx          # QueryClientProvider + BrowserRouter
        ├── App.tsx           # routes
        ├── api/              # fetch client + TanStack Query hooks (queries.ts, mutations.ts)
        ├── components/ui/    # shadcn (generated)
        ├── components/sidebar/   # project list, create-project dialog
        ├── components/board/     # Board, Column, TaskCard, QuickAdd
        └── components/task/      # TaskView (tab strip), MarkdownEditor (edit/preview)
```

`internal/gitx/`, `internal/session/`, `internal/ws/` do NOT exist yet — created by Phases 2–3. Do not pre-create empty packages.

### Pattern 1: Embed + SPA fallback + dev proxy

**What:** Production serves the Vite build from the binary; development runs Vite's dev server proxying `/api` to the Go process. The Go binary needs no "dev mode."

```go
// web/embed.go
package web

import "embed"

// all: prefix required — plain //go:embed dist skips dotfiles and _-prefixed files
//go:embed all:dist
var DistFS embed.FS
```

```go
// cmd/kangent/main.go (essence) — SPA fallback, source: standard Go 1.22+ pattern
dist, _ := fs.Sub(web.DistFS, "dist")
fileServer := http.FileServerFS(dist)
mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
    // never fall back for API routes
    if strings.HasPrefix(r.URL.Path, "/api/") {
        http.NotFound(w, r)
        return
    }
    p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
    if p != "" {
        if f, err := dist.Open(p); err == nil {
            f.Close()
            fileServer.ServeHTTP(w, r) // real asset (hashed → cacheable)
            return
        }
    }
    // deep links (/projects/3/tasks/7) → index.html, never cached
    w.Header().Set("Cache-Control", "no-store")
    http.ServeFileFS(w, r, dist, "index.html")
})
```

```ts
// web/vite.config.ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(__dirname, "./src") } },
  server: { proxy: { "/api": "http://127.0.0.1:7333" } },
});
```

**Critical detail:** `go:embed` fails the build if `web/dist` is missing or empty. Commit a placeholder `web/dist/index.html` ("run make build") and have the Makefile always build the frontend before the Go binary.

**Server startup (Claude's discretion — recommendation):** bind `127.0.0.1:7333` by default (never 0.0.0.0 — PITFALLS Pitfall 10 starts now, not Phase 2); flags `--addr` and `--db`; DB defaults to `~/.kangent/kangent.db` (`os.MkdirAll` the dir; `~/.kangent/` becomes the app home that Phase 3 reuses for worktrees); print the URL on startup; auto-open browser only behind an `--open` flag (default off — restarting during dev would spam tabs).

### Pattern 2: SQLite open discipline (must be the first store code written)

```go
// internal/store/store.go — DSN syntax verified against pkg.go.dev/modernc.org/sqlite
import (
    "database/sql"
    _ "modernc.org/sqlite" // registered driver name is "sqlite", NOT "sqlite3"
)

func Open(dbPath string) (*sql.DB, error) {
    dsn := "file:" + dbPath +
        "?_pragma=journal_mode(WAL)" +
        "&_pragma=busy_timeout(5000)" +
        "&_pragma=foreign_keys(1)" +
        "&_pragma=synchronous(NORMAL)" +
        "&_txlock=immediate"
    db, err := sql.Open("sqlite", dsn)
    if err != nil {
        return nil, err
    }
    db.SetMaxOpenConns(1) // single-writer discipline: eliminates SQLITE_BUSY entirely
    return db, db.Ping()
}
```

Notes: `_pragma=name(value)` runs as a PRAGMA per new connection (so per-connection pragmas like `foreign_keys` and `busy_timeout` are always set); `journal_mode=WAL` is persistent in the file but harmless to re-apply. `_txlock=immediate` makes write transactions take the lock up front, avoiding deferred-upgrade `SQLITE_BUSY` (moot with one connection, but correct if the pool ever widens).

### Pattern 3: Goose embedded migrations at startup

```go
// internal/store/migrate.go — pattern verified against github.com/pressly/goose README
import (
    "embed"
    "github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func Migrate(db *sql.DB) error {
    goose.SetBaseFS(migrationsFS)
    if err := goose.SetDialect("sqlite3"); err != nil { // dialect string is "sqlite3"
        return err                                       // even though driver name is "sqlite"
    }
    return goose.Up(db, "migrations")
}
```

goose's README lists `modernc.org/sqlite` as a supported driver for the `sqlite3` dialect; goose operates on the `*sql.DB`, the dialect only controls version-table SQL.

**Phase 1 migration (`migrations/00001_init.sql`):**

```sql
-- +goose Up
CREATE TABLE projects (
  id          INTEGER PRIMARY KEY,
  name        TEXT NOT NULL,
  repo_path   TEXT NOT NULL UNIQUE,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE tasks (
  id          INTEGER PRIMARY KEY,
  project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  title       TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status      TEXT NOT NULL DEFAULT 'todo'
              CHECK (status IN ('todo','in_progress','in_review','done')),
  position    REAL NOT NULL,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX idx_tasks_board ON tasks(project_id, status, position);

-- +goose Down
DROP TABLE tasks;
DROP TABLE projects;
```

**Schema room for later phases (per scope guard):** do NOT add `branch_name`/`worktree_path` or a `sessions` table now. SQLite's `ALTER TABLE ADD COLUMN` + new goose migrations make Phase 3/4 additions trivial; `ON DELETE CASCADE` + goose-from-day-one IS the room. Status values match ARCHITECTURE.md exactly so later phases agree.

### Pattern 4: REST API shape

| Method & Path | Body | Returns | Notes |
|---------------|------|---------|-------|
| `GET /api/projects` | — | `[Project]` | Sidebar list |
| `POST /api/projects` | `{name?, repo_path}` | `201 Project` | Validates repo path (Pattern 5); empty name → `filepath.Base(repo_path)`; duplicate path → 409 |
| `PATCH /api/projects/{id}` | `{name}` | `Project` | Rename only; repo_path immutable in v1 |
| `DELETE /api/projects/{id}` | — | `204` | CASCADE removes tasks; repo on disk untouched |
| `GET /api/projects/{id}/tasks` | — | `[Task]` | `ORDER BY status, position ASC` — board fetch |
| `POST /api/projects/{id}/tasks` | `{title, description?}` | `201 Task` | status=todo, position = top of column (min−1.0) |
| `GET /api/tasks/{id}` | — | `Task` | Task view (deep link must work without board cache) |
| `PATCH /api/tasks/{id}` | `{title?, description?}` | `Task` | Content edits only |
| `POST /api/tasks/{id}/move` | `{status, after_id: number\|null}` | `Task` | `after_id:null` = top of target column |
| `DELETE /api/tasks/{id}` | — | `204` | Hard delete (D-11) |

Conventions: JSON everywhere; errors as `{"error": "human-readable message"}` with proper status (400 validation, 404, 409 conflict); Go 1.22+ ServeMux patterns (`mux.HandleFunc("POST /api/tasks/{id}/move", h.MoveTask)`, `r.PathValue("id")`). Position is returned in Task JSON so the client can sort, but the client never writes it.

### Pattern 5: Repo-path validation (Claude's discretion — resolved)

**Input UX:** plain text input. This is settled by browser security, not preference — `<input type="file" webkitdirectory>` and the File System Access API intentionally never expose absolute filesystem paths to a web page, so a real "picker" that yields a server-usable path is impossible in a web app. A monospace text input with placeholder `/home/you/code/my-repo`, validate-on-submit (optionally on blur via the same endpoint), inline error text under the field.

**Server validation (on POST /api/projects):**

```go
func validateRepoPath(p string) (string, error) {
    if !filepath.IsAbs(p) {
        return "", fmt.Errorf("path must be absolute")
    }
    abs := filepath.Clean(p)
    info, err := os.Stat(abs)
    if err != nil || !info.IsDir() {
        return "", fmt.Errorf("not a directory: %s", abs)
    }
    // handles .git-as-directory AND .git-as-file (worktrees/submodules)
    cmd := exec.Command("git", "-C", abs, "rev-parse", "--git-dir")
    if err := cmd.Run(); err != nil {
        return "", fmt.Errorf("not a git repository: %s", abs)
    }
    return abs, nil
}
```

Always `exec.Command` with arg arrays (never `sh -c` interpolation — PITFALLS security table). git presence is a project premise. Also expand a leading `~/` to the home dir server-side as a courtesy. This is the only git invocation in Phase 1.

### Pattern 6: Fractional ordering with server-computed positions

**Decision: REAL fractional position, server computes, client sends intent.** Rationale: client-side float math drifts and renormalization can't live in the client; integer reindexing rewrites O(column) rows per drag for no benefit at this scale.

Server logic for `move(taskID, status, afterID)` in ONE transaction:
1. `afterID == null` → `pos = (SELECT MIN(position) FROM tasks WHERE project_id=? AND status=?) - 1.0` (or `1.0` if column empty). This same rule implements D-09 (new tasks at top).
2. `afterID != null` → read `after.position` and the next task's position in that column; `pos = (after + next) / 2`, or `after + 1.0` if dropping at the bottom.
3. **Renormalization guard:** if `next - after < 1e-9` (float64 midpoint exhaustion — ~50 consecutive same-gap insertions), renumber the whole target column to 1.0, 2.0, 3.0… inside the same transaction, then recompute. Single-user scale: renumber is dozens of rows, instantaneous.
4. Update `status`, `position`, `updated_at`; return the task.

Validate that `after_id`, when given, belongs to the same project and target status → 400 otherwise.

### Pattern 7: dnd-kit kanban (multi-container sortable)

The canonical structure for a fixed-4-column board with dnd-kit 6.3.1/10.0.0 (API stable for years; React 19 compatible per STACK.md verification):

```tsx
// components/board/Board.tsx (essence)
// Pattern source: dnd-kit docs (sortable, multiple containers) + docs.dndkit.com
const sensors = useSensors(
  useSensor(PointerSensor, { activationConstraint: { distance: 5 } }), // CRITICAL: lets click-to-open still work
  useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
);

// Local mirror of board state during drag — dnd-kit needs synchronous reorders
const [columns, setColumns] = useState<Record<Status, Task[]>>(grouped);

<DndContext
  sensors={sensors}
  collisionDetection={closestCorners}
  onDragStart={({ active }) => setActiveTask(findTask(active.id))}
  onDragOver={handleDragOver}   // move card between columns in LOCAL state only
  onDragEnd={handleDragEnd}     // compute {status, after_id}, fire mutation
>
  {STATUSES.map((status) => (
    <Column key={status} status={status} tasks={columns[status]} />
  ))}
  <DragOverlay>{activeTask && <TaskCard task={activeTask} overlay />}</DragOverlay>
</DndContext>

// components/board/Column.tsx — column must be droppable so EMPTY columns accept drops
function Column({ status, tasks }) {
  const { setNodeRef } = useDroppable({ id: `column:${status}` }); // prefix avoids id collision with task ids
  return (
    <SortableContext items={tasks.map((t) => t.id)} strategy={verticalListSortingStrategy}>
      <div ref={setNodeRef}>{tasks.map((t) => <TaskCard key={t.id} task={t} />)}</div>
    </SortableContext>
  );
}

// TaskCard: useSortable({ id: task.id }) + CSS.Transform.toString(transform)
```

`handleDragOver`: if `over` resolves to a different container (a `column:*` id or a task in another column), splice the task into that column's local array. `handleDragEnd`: find the task's final index; `after_id` = id of the card above it (or `null` at index 0); call the move mutation; on settle, invalidate the board query (server position becomes truth).

**TanStack Query integration:** the move mutation uses optimistic update (`onMutate` → cancel queries, snapshot, write moved order into cache; `onError` → rollback snapshot; `onSettled` → invalidate `["tasks", projectId]`). The local `columns` state and query cache must reconcile — simplest correct approach: derive `columns` from the query data via `useEffect`/`useMemo`, suspend derivation while a drag is active.

### Pattern 8: Frontend shell — routing, theme, tabs

**Routes (react-router 7, declarative mode):**

```tsx
<BrowserRouter>
  <Routes>
    <Route element={<AppLayout />}>           {/* sidebar always visible (D-05) */}
      <Route index element={<RedirectToFirstProject />} />
      <Route path="/projects/:projectId" element={<BoardPage />} />
      <Route path="/projects/:projectId/tasks/:taskId" element={<TaskPage />} />
    </Route>
  </Routes>
</BrowserRouter>
```

Esc in TaskPage → `navigate(`/projects/${projectId}`)` (not `navigate(-1)` — deep-linked tabs have no history). Guard the keydown handler: ignore when `e.target` is an input/textarea/contenteditable.

**Dark-only theme (D-01..03):** set `class="dark"` on `<html>` in `index.html` and keep shadcn's generated dark token block — least friction with shadcn component updates; no theme toggle code. Dense aesthetic: shadcn defaults are roomier than Linear — globally nudge with smaller base font (`text-sm` on body), reduced card padding, `--radius: 0.375rem`-ish. Tailwind v4 config is CSS-only: `@import "tailwindcss";` + `@theme` / CSS variables in `src/index.css`; `@plugin "@tailwindcss/typography";` for markdown prose. There is no tailwind.config.js — do not create one.

**Tab strip (D-06, the architectural seam):** shadcn `Tabs` in TaskPage with a tabs array rendered from a list — Phase 1 list is `[{id:"description", label:"Description"}]`. Later phases append to that list; the container, not the tab, owns layout. Don't over-engineer: a typed array + map is the whole "extensibility."

**Markdown (D-10):**

```tsx
// view mode
<div className="prose prose-invert prose-sm max-w-none">
  <ReactMarkdown remarkPlugins={[remarkGfm]}>{task.description}</ReactMarkdown>
</div>
// edit mode: plain <Textarea>, explicit Save/Cancel; toggle via edit button
```

XSS posture: react-markdown does not render raw HTML by default (it's simply omitted) and sanitizes `javascript:` URLs via its default `urlTransform`. Do NOT add `rehype-raw`. No DOMPurify needed.

### Anti-Patterns to Avoid

- **tailwind.config.js with Tailwind 4:** legacy v3 config silently ignored unless `@config` is used; v4 is CSS-first (`@theme`). Tutorials older than 2025 will mislead.
- **`react-router-dom` package:** v7 consolidated into `react-router`; installing both causes duplicate-router bugs.
- **Driver name `"sqlite3"` with modernc:** registers as `"sqlite"`; `sql.Open("sqlite3", …)` fails at runtime with "unknown driver."
- **Client-computed positions:** float drift + renormalization in two places; server owns ordering (Pattern 6).
- **Drawer/modal task view:** explicitly rejected (D-05).
- **Per-keystroke description autosave:** explicit Save in edit/preview toggle (D-10) — simpler and matches the decision.
- **Pre-building Phase 2+ schema/packages:** no sessions table, no gitx package, no worktree columns. Goose migrations are the extension mechanism.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Drag-and-drop | Pointer-event math, drop zones, keyboard a11y | dnd-kit | Collision detection, sensors, accessibility, overlay rendering — months of edge cases |
| Server-state caching / optimistic updates | Hand-rolled fetch + useState + rollback | TanStack Query | Mutation rollback, request dedup, cache invalidation are subtle to get right |
| Markdown → React | Regex/string HTML pipeline | react-markdown + remark-gfm | XSS-safe by construction; GFM edge cases (tables, autolinks) are a swamp |
| Schema evolution | "CREATE TABLE IF NOT EXISTS" drift | goose embedded migrations | Versioned, transactional, already chosen project-wide |
| Dialog/tabs/dropdown primitives | Custom focus traps and portals | shadcn/ui (Radix under the hood) | Focus management, aria, Esc/overlay-click handling |
| Collapsible sidebar | Custom collapse state + layout | shadcn Sidebar component | Ships collapsible behavior, keyboard shortcut, mobile handling; matches D-04 exactly |
| Git repo detection edge cases | Stat-ing `.git` manually | `git -C <p> rev-parse --git-dir` | `.git` can be a FILE (worktrees, submodules); git itself is the authority |

**Key insight:** this phase has zero novel algorithms. Every hour spent here should go into wiring verified libraries correctly and establishing conventions, not reimplementing solved problems.

## Common Pitfalls

### Pitfall 1: SQLite configured "later"
**What goes wrong:** Default journal mode + default pool → intermittent `database is locked` once anything concurrent happens (Phase 4 session goroutines).
**Why it happens:** CRUD works fine in single-request testing; the misconfiguration is invisible until load.
**How to avoid:** Pattern 2 verbatim, in the first store commit. PITFALLS.md Pitfall 9 assigns this to exactly this phase.
**Warning signs:** any `sql.Open` call without `_pragma=` params or without `SetMaxOpenConns(1)`.

### Pitfall 2: embed breaks the build or serves stale assets
**What goes wrong:** `go build` fails with "pattern all:dist: no matching files" on fresh clone; or devs edit React, rebuild Go only, and see stale UI.
**Why it happens:** `go:embed` resolves at compile time; dist/ is gitignored build output.
**How to avoid:** commit placeholder `web/dist/index.html`; gitignore `web/dist/*` except the placeholder (or re-add after builds via Makefile); `make build` always runs `npm run build` first; dev workflow uses Vite dev server + proxy so Go rebuilds aren't needed for UI changes.
**Warning signs:** CI green only because dist was committed wholesale; "my change isn't showing" reports.

### Pitfall 3: SPA fallback eats API 404s (or vice versa)
**What goes wrong:** unknown `/api/...` paths return index.html (confusing JSON parse errors in the client), or deep links like `/projects/3/tasks/7` return 404 from the file server.
**How to avoid:** fallback handler explicitly excludes `/api/` prefix and serves index.html with `Cache-Control: no-store` for all other misses (Pattern 1). Test both: `curl localhost:7333/api/nope` → JSON 404; `curl localhost:7333/projects/1/tasks/2` → HTML.

### Pitfall 4: dnd-kit drag swallows clicks (task can't be opened)
**What goes wrong:** `useSortable` listeners capture pointerdown; every click on a card starts a 0-distance drag and the click-to-navigate (D-05/TASK-05) never fires.
**How to avoid:** `PointerSensor` with `activationConstraint: { distance: 5 }` (Pattern 7). Test: click opens task; press-and-move drags.
**Warning signs:** navigation works from keyboard but not mouse; drag starts on plain clicks.

### Pitfall 5: Empty columns reject drops / id collisions
**What goes wrong:** a column with no tasks has no sortable items, so there's nothing to drop "over" — cards can't be dragged into Done. Or a column id equal to a task id confuses `over` resolution.
**How to avoid:** each column is itself a `useDroppable` with a namespaced id (`column:todo`); `onDragOver` resolves `over.id` to either a task's column or the column id itself.
**Warning signs:** drops only work on non-empty columns.

### Pitfall 6: Optimistic update fights server truth
**What goes wrong:** card flickers back after drop, or order silently diverges from DB after rapid consecutive drags.
**Why it happens:** local drag state, query cache, and server positions are three copies of order; invalidation mid-drag overwrites local state.
**How to avoid:** single derivation path (query data → columns), frozen during active drag; optimistic cache write in `onMutate`; `invalidateQueries` only in `onSettled`. Accept last-writer-wins — single user, no concurrency.

### Pitfall 7: Tailwind v4 setup follows v3 instructions
**What goes wrong:** `npx tailwindcss init`, `content` arrays, PostCSS config — none apply; classes silently don't generate or the build errors.
**How to avoid:** v4 = `npm i tailwindcss @tailwindcss/vite` + plugin in vite.config.ts + `@import "tailwindcss";` in CSS. shadcn init detects v4 and writes CSS-variable themes. Plugins load via `@plugin` in CSS.

### Pitfall 8: goose dialect/driver name confusion
**What goes wrong:** `sql.Open("sqlite3", …)` → "unknown driver"; or `goose.SetDialect("sqlite")` → unsupported dialect.
**How to avoid:** driver name is `"sqlite"` (modernc), goose dialect is `"sqlite3"`. Yes, they differ. Pattern 2 + 3 have it right.

### Pitfall 9: Esc shortcut fires while typing
**What goes wrong:** user presses Esc to dismiss autocomplete or cancel an edit inside the markdown textarea and gets navigated to the board, losing the draft.
**How to avoid:** global keydown handler checks `document.activeElement` (input/textarea/contenteditable → ignore, or first Esc blurs); shadcn Dialog already handles its own Esc with stopPropagation — verify ordering with the dialog open.

### Pitfall 10: Go toolchain surprise on first build
**What goes wrong:** machine has Go 1.24.3; go.mod says `go 1.26`; first `go build` pauses to download the 1.26 toolchain (needs network) — confusing in offline/CI contexts.
**How to avoid:** expected behavior with `GOTOOLCHAIN=auto` (verified on this machine). Document in README; CI should pin/setup Go 1.26.x explicitly. modernc.org/sqlite requires Go ≥ 1.25.0, goose ≥ 1.25.7 (verified from module proxy), so staying on 1.24 is not an option.

## Code Examples

Beyond the patterns above, two glue examples the planner can reference:

### TanStack Query move mutation with optimistic update

```tsx
// api/mutations.ts — pattern source: tanstack.com/query optimistic updates guide
const moveTask = useMutation({
  mutationFn: ({ id, status, afterId }: MoveArgs) =>
    api.post(`/api/tasks/${id}/move`, { status, after_id: afterId }),
  onMutate: async (args) => {
    await queryClient.cancelQueries({ queryKey: ["tasks", projectId] });
    const prev = queryClient.getQueryData<Task[]>(["tasks", projectId]);
    queryClient.setQueryData(["tasks", projectId], (old) => applyMove(old, args));
    return { prev };
  },
  onError: (_e, _v, ctx) => queryClient.setQueryData(["tasks", projectId], ctx?.prev),
  onSettled: () => queryClient.invalidateQueries({ queryKey: ["tasks", projectId] }),
});
```

### Move endpoint core (Go, single transaction)

```go
// internal/api/tasks.go (essence)
tx, err := s.db.BeginTx(ctx, nil) // _txlock=immediate makes this a write lock
// 1. load task, verify exists
// 2. compute position:
//    afterID == nil  → SELECT COALESCE(MIN(position),2.0)-1.0 FROM tasks WHERE project_id=? AND status=?
//    afterID != nil  → SELECT position FROM tasks WHERE id=? (after)
//                      SELECT MIN(position) FROM tasks WHERE project_id=? AND status=? AND position>? (next)
//                      pos = next==nil ? after+1.0 : (after+next)/2
// 3. if next-after < 1e-9 → renumber column: UPDATE ... SET position=rowNum (loop), recompute
// 4. UPDATE tasks SET status=?, position=?, updated_at=? WHERE id=?
// 5. tx.Commit(); return updated task JSON
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| tailwind.config.js + PostCSS | CSS-first `@theme` + `@tailwindcss/vite` plugin | Tailwind 4.0 (Jan 2025) | No JS config file at all; plugins via `@plugin` in CSS |
| react-router-dom v6 | `react-router` v7 single package, declarative/data/framework modes | v7 (Nov 2024) | Install `react-router` only; declarative mode for this SPA |
| react-beautiful-dnd | dnd-kit (or pragmatic-drag-and-drop) | rbd archived 2024 | rbd incompatible with React 19 — never reach for it |
| `http.FileServer(http.FS(...))` | `http.FileServerFS` / `http.ServeFileFS` | Go 1.22 | Cleaner embed serving, no wrapper |
| `mattn/go-sqlite3` default choice | `modernc.org/sqlite` for CGO-free builds | ecosystem shift ~2023–2025 | Single static binary, trivial cross-compile |
| react-markdown v8/v9 APIs | v10: `urlTransform` (replaced `transformLinkUri`), no `linkTarget` prop | v9→v10 (2024–2025) | Old snippets using removed props won't compile |

**Deprecated/outdated:** `npx tailwindcss init` (gone in v4 workflow); `tailwindcss-animate` (shadcn now uses `tw-animate-css` with v4); unscoped `xterm` package (irrelevant this phase but in repo orbit).

## Open Questions

1. **shadcn Sidebar component fit for the dense aesthetic**
   - What we know: shadcn ships a full Sidebar primitive with collapsible behavior (verified supported in current CLI); it satisfies D-04 with near-zero code.
   - What's unclear: whether its default density matches the Linear-style target or needs token overrides.
   - Recommendation: adopt it, override spacing tokens in CSS; only hand-roll (a `<aside>` + width toggle is ~30 lines) if it fights the design. Low risk either way — Claude's discretion area.

2. **Renormalization threshold tuning**
   - What we know: float64 midpoints survive ~50 consecutive worst-case insertions at the same gap; 1e-9 threshold with whole-column renumber is safe.
   - What's unclear: nothing material — single-user scale makes any sane threshold fine.
   - Recommendation: implement as specified; add a unit test that inserts 200 tasks at top and 200 between the same pair and asserts strict ordering.

3. **`~` expansion and path UX niceties**
   - What we know: plain text input is forced by browser security; server validates.
   - What's unclear: whether to add a "recent paths" or server-side directory-listing autocomplete endpoint.
   - Recommendation: skip autocomplete in Phase 1 (scope discipline); `~/` expansion server-side is 3 lines and worth it.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | backend build | ✓ | 1.24.3 + GOTOOLCHAIN=auto (auto-fetches 1.26 per go.mod; verified `go env GOTOOLCHAIN` = auto) | none needed |
| Node.js | Vite 8 (needs 20.19+/22.12+) | ✓ | 24.4.1 | — |
| npm | frontend deps | ✓ | 11.6.0 | — |
| git CLI | repo-path validation (PROJ-01) | ✓ | 2.43.0 | — (hard project premise) |
| SQLite | storage | ✓ (embedded in modernc driver — no system sqlite needed) | driver tracks SQLite 3.53.2 | — |
| Network on first build | Go toolchain download + module/npm fetch | assumed ✓ | — | pre-install Go 1.26 manually if offline |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** none — environment is fully ready.

## Sources

### Primary (HIGH confidence)
- pkg.go.dev/modernc.org/sqlite — driver name `"sqlite"`, `_pragma=name(value)` DSN syntax, `_txlock`, `_time_format` (fetched 2026-06-10)
- github.com/pressly/goose README — `SetBaseFS`/`SetDialect("sqlite3")`/`Up` library pattern (fetched 2026-06-10)
- proxy.golang.org — modernc.org/sqlite v1.52.0 (requires go 1.25.0), goose v3.27.1 (requires go 1.25.7) (queried 2026-06-10)
- npm registry direct queries (2026-06-10) — react 19.2.7, vite 8.0.16, tailwindcss/@tailwindcss/vite 4.3.0, @tailwindcss/typography 0.5.20, react-markdown 10.1.0, remark-gfm 4.0.1, react-router 7.17.0, typescript 6.0.3, @vitejs/plugin-react 6.0.2
- Local environment probes (2026-06-10) — go 1.24.3, GOTOOLCHAIN=auto, node 24.4.1, npm 11.6.0, git 2.43.0
- `.planning/research/STACK.md` (researched same day, registry-verified) — dnd-kit 6.3.1/10.0.0 + React 19 pairing, shadcn Tailwind-v4 support, TanStack Query 5.101.0, embed/serving pattern
- `.planning/research/PITFALLS.md` — SQLite pragma discipline (Pitfall 9), localhost binding (Pitfall 10), path-traversal/command-injection security table

### Secondary (MEDIUM confidence)
- dnd-kit documentation patterns (multi-container sortable, PointerSensor activationConstraint, DragOverlay) — API stable since 2022, versions verified current via STACK.md; specific code shapes from training corroborated by the docs structure
- react-markdown v10 security defaults (raw HTML omitted, `urlTransform` sanitizes `javascript:`) — long-standing documented behavior across v9/v10; version verified, behavior from training + changelog knowledge
- File System Access API not exposing absolute paths (forces text-input UX) — well-established browser security model, unchanged for years

### Tertiary (LOW confidence)
- shadcn Sidebar component default density/ergonomics — flagged in Open Questions; verify during implementation

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — every version re-verified against registries today; matches same-day project STACK.md
- Architecture: HIGH — patterns are the project's own ARCHITECTURE.md narrowed to Phase 1, plus verified driver/migration syntax
- Pitfalls: HIGH for SQLite/embed/routing (official docs + project PITFALLS.md); MEDIUM for dnd-kit interaction details (stable API, training-sourced specifics)

**Research date:** 2026-06-10
**Valid until:** ~2026-07-10 (stable stack; re-check npm versions if planning slips a month)
