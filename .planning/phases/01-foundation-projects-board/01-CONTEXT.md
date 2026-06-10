# Phase 1: Foundation — Projects & Board - Context

**Gathered:** 2026-06-10
**Status:** Ready for planning

<domain>
## Phase Boundary

Single Go binary serving an embedded React SPA at localhost. Projects sidebar (create/rename/delete, repo-path validation), kanban board with four fixed columns (To Do / In Progress / In Review / Done), task CRUD with markdown descriptions, drag-and-drop with persisted order, all backed by SQLite. No git operations, no terminals, no sessions — those are Phases 2–4. Task creation in this phase does NOT create worktrees.

</domain>

<decisions>
## Implementation Decisions

### Visual Direction
- **D-01:** Dark-first theme. No light mode in v1 — terminals (coming in Phase 2) should feel native against the UI.
- **D-02:** Dense developer-tool aesthetic (Linear/Raycast-style): compact spacing, small text, information-dense, keyboard-friendly.
- **D-03:** Mostly neutral palette (greys/dark neutrals); color is reserved for status accents (session status badges arriving in Phase 4). Columns are NOT individually colored.
- **D-04:** Collapsible left sidebar — toggles to give the board/terminal more space.

### Task View Presentation
- **D-05:** Clicking a task navigates to a full-page task route; the sidebar stays visible. Each task is URL-addressable (deep-linkable). Modal/drawer rejected — terminals in later phases need maximum space.
- **D-06:** Task view uses a single tab strip where Description is a tab. Future phases append Agent and Bash tabs to the same strip. Phase 1 ships the tab container with just the Description tab.
- **D-07:** Back to board via explicit back button + Esc shortcut; browser back also works (routing-based).

### Task Creation & Board Flow
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

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project planning
- `.planning/PROJECT.md` — vision, core value, key decisions (PTY-not-SDK, worktree-per-task, Go+React)
- `.planning/REQUIREMENTS.md` — Phase 1 owns STOR-01, STOR-02, PROJ-01..03, TASK-01..05
- `.planning/ROADMAP.md` — phase boundaries and success criteria

### Research (stack & architecture for this phase)
- `.planning/research/STACK.md` — prescriptive library choices with versions (Go 1.26 stdlib mux, modernc.org/sqlite, goose, Vite, React 19, Tailwind 4, shadcn/ui, dnd-kit, TanStack Query); single-binary embed.FS pattern
- `.planning/research/ARCHITECTURE.md` — component boundaries, data model (projects/tasks tables), build order; Phase 1 = its "Skeleton" stage
- `.planning/research/PITFALLS.md` — SQLite pragmas (WAL, busy_timeout, MaxOpenConns(1)) must land in this phase, before anything writes
- `.planning/research/SUMMARY.md` — synthesis and phase-mapping of all research

No external specs beyond these — requirements fully captured in decisions above.

</canonical_refs>

<code_context>
## Existing Code Insights

Greenfield — repository contains only `.planning/` and `CLAUDE.md`. No existing code, patterns, or integration points. This phase establishes the conventions every later phase builds on (project layout, API shape, DB access, frontend structure).

### Phase-1-relevant research guidance
- Single Go process: stdlib `net/http` mux serving REST API + embedded SPA via `embed.FS`
- SQLite via `modernc.org/sqlite` (pure Go, CGO-free single binary) with WAL mode, busy_timeout, MaxOpenConns(1) configured at startup; goose for migrations
- Frontend: Vite + React 19 + Tailwind 4 + shadcn/ui components, TanStack Query for data fetching, dnd-kit for board drag-and-drop
- Data model anticipates later phases: tasks will gain worktree/branch fields (Phase 3) and session metadata (Phases 4–5) — schema design should leave room without building those now

</code_context>

<specifics>
## Specific Ideas

- Look-and-feel reference: SlayZone's layout (left projects rail + board + expandable task workspace) translated to a dense, dark web UI; vibe-kanban as the web-app precedent
- The task view's tab strip is the architectural seam for the whole product: Description tab now, Agent + Bash tabs (Phases 2–4) and Diff tab (Phase 5) join the same strip later

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 01-foundation-projects-board*
*Context gathered: 2026-06-10*
