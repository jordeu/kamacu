# Phase 3: Worktree Isolation & Bash Tabs - Context

**Gathered:** 2026-06-10
**Status:** Ready for planning

<domain>
## Phase Boundary

Every task gets its own git worktree and branch with collision-safe naming, created automatically at task creation; bash session tabs in the task view run inside that worktree (Phase 2's terminal engine mounted into the task UI via the `TabDef[]` seam); marking a task Done offers confirmed worktree cleanup that keeps the branch, warns on uncommitted changes, and never silently kills running sessions (GIT-01, GIT-02, GIT-03, TERM-04).

Out of this phase: Claude Code agent sessions and the Start button (Phase 4), status badges (Phase 4), diff tab (Phase 5), merge/PR automation (out of scope entirely).

</domain>

<decisions>
## Implementation Decisions

### Branch & Worktree Naming
- **D-22:** Branch name: `task/<slug>-<id>` (e.g. `task/fix-login-42`). Slug from task title, task id guarantees uniqueness.
- **D-23:** Worktrees live centrally at `~/.kangent/worktrees/<project>/<task-slug-id>/` — outside every repo tree, easy to inspect and purge.

### Base Branch & Failure Handling
- **D-24:** Task branches are created from the repo's **default branch tip** (resolve via `origin/HEAD` → main/master; fall back to current HEAD if unresolvable). No fetch before branching — local tip is the base.
- **D-25:** Worktree creation failure does NOT block task creation: the task lands on the board with a visible "no worktree" warning state and a Retry action in the task view. Git problems never block idea capture.
- **D-26:** Pre-existing tasks (Phase 1 era) get lazy worktree creation: opening such a task shows the no-worktree state with a "Create worktree" button; bash tabs unlock once it exists. No startup backfill/migration.

### Bash Tabs UX
- **D-27:** A `+` button at the end of the task view's tab strip spawns a new bash session (cwd = task worktree) and appends a tab labeled `Bash 1`, `Bash 2`, ... Explicit spawn only — consistent with the Start-button philosophy.
- **D-28:** Tabs mirror live server sessions: reopening a task shows a tab per still-running session for that task, reattached via the Phase 2 replay path. Server is the source of truth for which tabs exist.
- **D-29:** Closing a tab (× on the tab) STOPS the session (Phase 2 SIGTERM→5s→SIGKILL teardown) and removes the tab. One concept: tab = session. Exited sessions can also be closed from the exited banner.
- **D-30:** Bash tabs require a worktree — disabled with explanation when the task has none (ties to D-25/D-26).

### Done & Cleanup Flow
- **D-31:** Moving a task to Done (drag or status change) triggers the cleanup offer dialog. Declining keeps the worktree. Cleanup is ALSO always available as an action in the task view (i.e. "Both" behavior: prompt on Done + menu action).
- **D-32:** If sessions are running when cleanup is requested, the dialog shows "N sessions running" and offers "Stop sessions and clean up" — explicit, single flow. Cleanup never silently kills sessions (satisfies GIT-03's refuse-while-running as an explicit gate).
- **D-33:** If the worktree has uncommitted changes, the dialog warns with the changed-file count and requires type-to-confirm (typing the task slug) before deletion. The branch is ALWAYS kept regardless — only uncommitted work is ever at risk.
- **D-34:** Cleanup = `git worktree remove` + prune bookkeeping; never `branch -D`.

### Claude's Discretion
- Slug generation rules (length cap, charset, dedup)
- Exact dialog copy (follow UI-SPEC copywriting patterns: specific, verb+noun CTAs, destructive confirmations)
- DB schema for worktree/branch fields on tasks and session→task association
- How "tabs mirror sessions" is implemented (extend Phase 2 session manager with task association; list endpoint filter)
- Whether the `/terminal` dev route stays or goes this phase (deferred decision from Phase 2 — keep if it costs nothing)
- Submodule handling (research Phase pitfalls doc flags it; do something sensible or document the limitation)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project planning
- `.planning/PROJECT.md` — worktree-per-task is a core key decision; manual git workflow locked
- `.planning/REQUIREMENTS.md` — Phase 3 owns GIT-01, GIT-02, GIT-03, TERM-04
- `.planning/ROADMAP.md` — Phase 3 success criteria (4)

### Research
- `.planning/research/PITFALLS.md` — worktree section: dirty-tree destruction hazard, branch collisions, submodules, sessions-running-in-tree cwd hazard; use git CLI not go-git
- `.planning/research/STACK.md` — git CLI via os/exec with `--porcelain` parsing (go-git non-viable for linked worktrees, verified)
- `.planning/research/FEATURES.md` — vibe-kanban cleanup bug history (issues #1571/#1764) motivating D-31..D-33

### Existing code (integration points)
- `internal/session/` — Phase 2 session manager: Spawn (needs cwd + task association now), Stop, List, Attach
- `internal/api/sessions.go` + `internal/ws/handler.go` — session REST/WS surface to extend with task scoping
- `internal/store/migrations/` — goose migrations; tasks table gains worktree/branch columns
- `web/src/components/task/TaskTabs.tsx` — the `TabDef[]` seam bash tabs plug into
- `web/src/components/terminal/TerminalPane.tsx` + `useTerminalSocket.ts` — route-agnostic pane to mount in tabs
- `web/src/api/sessions.ts` — session hooks to extend with task filtering
- `internal/api/tasks.go` — task creation (worktree hook) and status change (Done → cleanup offer signal)
- `.planning/phases/01-foundation-projects-board/01-UI-SPEC.md` + `.planning/phases/02-terminal-engine/02-UI-SPEC.md` — design system + terminal component contracts

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Phase 2 terminal stack is complete and route-agnostic: spawning a bash tab is "POST spawn with cwd + task_id, mount TerminalPane with session id"
- Session manager already supports labels (`bash #N`), teardown, replay; needs cwd parameter and task association
- AlertDialog/Dialog patterns + destructive copy conventions established in Phase 1 (delete task/project dialogs)
- `useMoveTask` optimistic mutation is where the Done-transition hook fires client-side

### Established Patterns
- goose migrations with WAL SQLite; REST under `/api/*`; table-driven httptest Go tests
- TanStack Query hooks with `["sessions"]` / `["tasks", projectId]` keys; optimistic updates on move

### Integration Points
- Task creation handler (`internal/api/tasks.go` createTask) calls a new worktree service after insert; failure records the no-worktree state instead of erroring the request (D-25)
- Status-change to done → response includes cleanup-offer info, or client checks worktree state and shows dialog (D-31)
- New `internal/git` (or `internal/worktree`) package shelling out to `git -C`; parse `--porcelain` output
- Sessions gain `task_id` + cwd; `/api/tasks/{id}/sessions` or query param filter feeds the tab strip (D-28)

</code_context>

<specifics>
## Specific Ideas

- Tab = session is the mental model: the strip is a live view of what's running in the worktree
- Cleanup dialog should feel like the project-delete dialog ("The branch task/fix-login-42 is kept.") — reassuring about what is NOT deleted
- vibe-kanban's ghost-run/lost-work issues are the cautionary tale: no automatic cleanup, no force-removal without typed confirmation

</specifics>

<deferred>
## Deferred Ideas

- Stale-worktree list with manual purge (v2 — MAINT-01)
- Worktree status indicator on kanban cards (could ride along with Phase 4 status badges)
- Per-project base-branch setting (only if default-branch resolution proves annoying)

</deferred>

---

*Phase: 03-worktree-isolation-bash-tabs*
*Context gathered: 2026-06-10*
