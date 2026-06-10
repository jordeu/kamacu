---
phase: 01-foundation-projects-board
verified: 2026-06-10T07:45:08Z
status: passed
score: 5/5 must-haves verified
---

# Phase 1: Foundation — Projects & Board Verification Report

**Phase Goal:** User can organize projects and tasks on a persistent kanban board served by one local binary
**Verified:** 2026-06-10T07:45:08Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (ROADMAP Success Criteria)

| #   | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| 1   | User can run a single local process and open the app (React SPA embedded in the Go binary) in a browser at localhost | ✓ VERIFIED | `cmd/kangent/main.go` embeds `web.DistFS` via `fs.Sub` + `http.FileServerFS` with SPA fallback (lines 55-72); `bin/kangent` exists; `scripts/smoke.sh` run live: `GET /` returns HTML, deep link `/projects/{pid}/tasks/{tid}` returns SPA index, `GET /api/nope` returns 404 — SMOKE OK |
| 2   | User can create a project by pointing at a local git repo (invalid paths rejected), see it in sidebar, switch boards, rename or delete it (repo on disk untouched) | ✓ VERIFIED | `internal/api/projects.go`: `exec.Command("git", "-C", abs, "rev-parse", "--git-dir")` validation (line 49), duplicate → 409 (line 106); smoke test: valid repo → 201 with defaulted name, `/nonexistent-zzz` → 400; `ProjectSidebar.tsx` renders `<Link to={`/projects/${project.id}`}>`; `ProjectMenu.tsx` wires `useDeleteProject` behind AlertDialog; zero `os.Remove`/`os.RemoveAll` calls anywhere in `internal/` or `cmd/` — delete is DB-only; UI walkthrough human-approved |
| 3   | User can create a task with title and markdown description, edit it, delete it, and open its expanded task view | ✓ VERIFIED | Smoke test: POST task → 201, PATCH description → 200; `TaskPage.tsx` fetches via `useTask(taskId)` (deep-linkable, line 41); `DescriptionTab.tsx` renders `<ReactMarkdown remarkPlugins={[remarkGfm]}>` with explicit Edit/Save toggle wired to `useUpdateTask`; `DeleteTaskDialog.tsx` confirms then navigates back; Esc handling guarded for inputs (TaskPage lines 54, 145) |
| 4   | User can drag tasks between the four fixed columns (To Do / In Progress / In Review / Done) and the new status persists | ✓ VERIFIED | `types.ts` defines exactly the 4 statuses with labels To Do/In Progress/In Review/Done; `Board.tsx` DndContext with `activationConstraint`, derives columns from `useTasks` data, `handleDragEnd` → `useMoveTask`; server move endpoint uses `BeginTx` with fractional positioning (`tasks.go` line 209); smoke test: move → 200, status `in_review` persists; ordering stress-tested in `tasks_test.go` (hundreds of moves, renormalization) — tests pass; drag feel human-approved |
| 5   | All projects and tasks survive a server restart (SQLite persistence) | ✓ VERIFIED | `store.Open` uses modernc.org/sqlite (`sql.Open("sqlite", ...)`) with WAL/busy_timeout/foreign_keys pragmas and `SetMaxOpenConns(1)`; goose migrations run at startup; smoke test run live: kill server → restart on same DB → project present, task status `in_review`, description `updated` — all asserted over HTTP; restart persistence also human-approved in checkpoint |

**Score:** 5/5 truths verified

### Required Artifacts

All 25 artifacts across 7 plans pass `gsd-tools verify artifacts` (exists + substantive + contains/exports checks). Wiring and data flow verified per plan:

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/store/store.go` | Open() with pragmas, SetMaxOpenConns(1) | ✓ VERIFIED | Full pragma discipline present; wired from main.go line 33 |
| `internal/store/migrations/00001_init.sql` | projects/tasks tables, CHECK enum, position REAL, ON DELETE CASCADE | ✓ VERIFIED | All present; index on (project_id, status, position) |
| `cmd/kangent/main.go` | flags, DB open, migrate, mux, ListenAndServe | ✓ VERIFIED | 127.0.0.1:7333 default; ~ expansion; SPA fallback with /api exclusion |
| `internal/api/projects.go` | CRUD + git rev-parse validation | ✓ VERIFIED | Real DB queries (Query/QueryRow/Exec); 400/409 paths; no disk mutation |
| `internal/api/tasks.go` | CRUD + move with fractional position | ✓ VERIFIED | BeginTx move, server-computed position, renormalization; after_id handling |
| `internal/api/routes.go` | Routes(mux, db), 10 endpoints | ✓ VERIFIED | All 10 endpoints with Go 1.22 method patterns; wired from main.go line 46 |
| `web/src/api/{types,client,queries,mutations}.ts` | Contract layer | ✓ VERIFIED | fetch wrapper throws ApiError; useProjects/useTasks/useTask; 7 mutation hooks incl. optimistic useMoveTask (onMutate) |
| `web/src/App.tsx` + `main.tsx` | Router + providers | ✓ VERIFIED | QueryClientProvider + BrowserRouter; board and task routes under AppLayout |
| `web/src/components/sidebar/*` | Project list, add dialog, menu, rename | ✓ VERIFIED | Link per project; useCreateProject with inline ApiError; useDeleteProject behind confirm; collapse state in localStorage |
| `web/src/components/board/*` | dnd-kit board, columns, quick-add, dialog | ✓ VERIFIED | DndContext + DragOverlay; columns derived from useTasks, frozen during drag; QuickAdd + NewTaskDialog both wired |
| `web/src/components/task/*` | Tabs seam, markdown edit, delete dialog | ✓ VERIFIED | Typed TabDef[] array (Phase 2+ seam); remarkGfm; useUpdateTask; AlertDialog |
| `web/embed.go` | //go:embed all:dist as DistFS | ✓ VERIFIED | Exact pattern present; placeholder `web/dist/index.html` committed (.gitignore exception `!web/dist/index.html`) so fresh clone compiles |
| `Makefile` | npm build → go build → bin/kangent | ✓ VERIFIED | Used live by smoke.sh; binary at bin/kangent |
| `scripts/smoke.sh` | restart-persistence proof over HTTP | ✓ VERIFIED | Executed live: SMOKE OK |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | --- | --- | ------ | ------- |
| cmd/kangent/main.go | internal/store | store.Open + store.Migrate before serve | ✓ WIRED | main.go:33,40 (tool regex false negative; confirmed by grep) |
| internal/store/store.go | modernc.org/sqlite | driver name "sqlite" | ✓ WIRED | sql.Open("sqlite", dsn) |
| cmd/kangent/main.go | internal/api | api.Routes(mux, db) | ✓ WIRED | main.go:46 |
| internal/api/tasks.go | tasks table | BeginTx move transaction | ✓ WIRED | tasks.go:209 |
| internal/api/projects.go | git CLI | exec.Command("git", ...) arg array | ✓ WIRED | projects.go:49 |
| web/src/main.tsx | QueryClientProvider + BrowserRouter | providers wrapping App | ✓ WIRED | tool-verified |
| web/src/api/client.ts | /api/* | fetch wrapper + ApiError | ✓ WIRED | client.ts:17,27 (tool regex error; confirmed by grep) |
| web/index.html | dark theme | class="dark" on html | ✓ WIRED | index.html:2 |
| ProjectSidebar.tsx | /projects/:projectId | Link per row | ✓ WIRED | line 51 |
| AddProjectDialog.tsx | useCreateProject | onSubmit, inline error | ✓ WIRED | tool-verified |
| ProjectMenu.tsx | useDeleteProject | AlertDialog action | ✓ WIRED | tool-verified |
| Board.tsx | useMoveTask | handleDragEnd → {status, after_id} | ✓ WIRED | tool-verified |
| TaskCard.tsx | task route | click nav gated by 5px activation | ✓ WIRED | tool-verified |
| Board.tsx | useTasks data | columns derived, frozen during drag | ✓ WIRED | tool-verified |
| TaskPage.tsx | useTask(taskId) | fetch by id (deep links) | ✓ WIRED | TaskPage.tsx:41 (tool regex error; confirmed by grep) |
| DescriptionTab.tsx | useUpdateTask | Save → PATCH {description} | ✓ WIRED | tool-verified |
| TaskPage.tsx | board route | back button + guarded Esc navigate | ✓ WIRED | TaskPage.tsx:64,117 |
| cmd/kangent/main.go | web.DistFS | fs.Sub + FileServerFS + SPA fallback | ✓ WIRED | main.go:55 (tool regex false negative; confirmed by grep) |
| scripts/smoke.sh | binary lifecycle | create → kill → restart → assert | ✓ WIRED | executed live, SMOKE OK |

19/19 key links wired. All 7 gsd-tools "not found" results were regex escaping artifacts in the tool ("Invalid regex pattern" / double-escaped patterns); every one was confirmed wired by direct grep.

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| Board.tsx | `columns` state | `useTasks(projectId)` → groupTasks → setColumns (lines 58-70) → `<Column tasks={columns[status]}>` | Yes — GET /api/projects/{id}/tasks does real SELECT | ✓ FLOWING |
| ProjectSidebar.tsx | project list | `useProjects()` → GET /api/projects → `h.db.Query(SELECT ... FROM projects)` | Yes | ✓ FLOWING |
| TaskPage.tsx | `task` | `useTask(taskId)` → GET /api/tasks/{id} → QueryRow SELECT | Yes — deep links work without board cache | ✓ FLOWING |
| API handlers | all responses | Real SQL (Query/QueryRow/Exec/BeginTx) in every handler | Yes — zero static/empty json returns found | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Backend compiles | `go build ./...` | BUILD OK | ✓ PASS |
| API + store test suites | `go test -count=1 ./internal/...` | ok kangent/internal/api 0.907s; ok kangent/internal/store 0.166s | ✓ PASS |
| End-to-end restart persistence + SPA serving | `bash scripts/smoke.sh` | SMOKE OK (build, create project 201, invalid path 400, create task, move, patch, kill+restart, data asserted, deep link HTML, /api/nope 404) | ✓ PASS |
| Frontend build embedded | `web/dist/index.html` + assets present; rebuilt by smoke run | exists | ✓ PASS |
| Documented fix commits exist | `git log` contains 7b600c1, 7a020d2 | both present | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan(s) | Description | Status | Evidence |
| ----------- | -------------- | ----------- | ------ | -------- |
| STOR-01 | 01-01, 01-07 | Projects/tasks persist in SQLite across restarts | ✓ SATISFIED | smoke.sh restart assertions pass; WAL SQLite + goose migrations |
| STOR-02 | 01-03, 01-07 | Single local process serves API + frontend at localhost | ✓ SATISFIED | one binary `bin/kangent` binds 127.0.0.1:7333, embeds SPA, serves /api |
| PROJ-01 | 01-02, 01-04 | Create project from validated git repo path | ✓ SATISFIED | git rev-parse validation; 400/409; AddProjectDialog inline errors |
| PROJ-02 | 01-04 | Sidebar lists projects; click switches board | ✓ SATISFIED | ProjectSidebar Link → /projects/:id route |
| PROJ-03 | 01-02, 01-04 | Rename/delete project; repo on disk untouched | ✓ SATISFIED | RenameProjectDialog + ProjectMenu; DELETE is DB-only (CASCADE tasks); no disk removal calls |
| TASK-01 | 01-02, 01-05 | Create task with title + markdown description | ✓ SATISFIED | QuickAdd + NewTaskDialog → POST, lands top of To Do |
| TASK-02 | 01-05 | Kanban board with 4 fixed columns | ✓ SATISFIED | STATUSES const + Board renders all four; labels match spec |
| TASK-03 | 01-02, 01-05 | Drag between columns, status persists | ✓ SATISFIED | dnd-kit → useMoveTask → POST /move (BeginTx); smoke + ordering tests |
| TASK-04 | 01-02, 01-06 | Edit title/description, delete task | ✓ SATISFIED | PATCH/DELETE endpoints; DescriptionTab edit, DeleteTaskDialog |
| TASK-05 | 01-06 | Expanded task view (description + session tabs seam) | ✓ SATISFIED | full-page TaskPage route, typed TabDef[] strip with Description tab |

10/10 requirements satisfied. No orphaned requirements — REQUIREMENTS.md maps exactly these 10 IDs to Phase 1 and every ID is claimed by at least one plan.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| — | — | none | — | — |

Scan of `internal/`, `cmd/`, `web/src/`, `scripts/`, `Makefile` found no TODO/FIXME/placeholder comments, no console.log, no empty handlers, no static empty API returns. All grep matches were legitimate: `todo` status literal, HTML input `placeholder=` attributes, and guarded conditional `return null` (loading states, empty tab arrays).

### Human Verification Required

None outstanding. The plan 01-07 human-verify checkpoint (full UI walkthrough: project create/rename/delete, board drag, task view, restart persistence) was walked and APPROVED by the user. Two bugs found during the checkpoint were fixed and verified in commits 7b600c1 (TooltipProvider crash) and 7a020d2 (collapsed-sidebar gutter overlap), both recorded in 01-07-SUMMARY.md.

### Gaps Summary

No gaps. All 5 success criteria verified against the actual codebase, all 25 artifacts substantive and wired, all 19 key links connected, data flows end-to-end from SQLite through REST to rendered React state, the full test suite passes, and the live smoke test proves single-binary serving and restart persistence over real HTTP.

---

_Verified: 2026-06-10T07:45:08Z_
_Verifier: Claude (gsd-verifier)_
