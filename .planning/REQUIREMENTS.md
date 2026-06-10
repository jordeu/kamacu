# Requirements: Kangent

**Defined:** 2026-06-10
**Core Value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.

## v1 Requirements

### Projects

- [ ] **PROJ-01**: User can create a project by pointing at a local directory containing a git repo (path validated at creation)
- [ ] **PROJ-02**: User can see all projects in a left sidebar and switch the board by clicking one
- [ ] **PROJ-03**: User can edit a project's name and delete a project (tasks removed from DB; repo on disk untouched)

### Tasks & Board

- [ ] **TASK-01**: User can create a task with title and markdown description in a project
- [ ] **TASK-02**: User can view tasks on a kanban board with fixed columns: To Do / In Progress / In Review / Done
- [ ] **TASK-03**: User can drag tasks between columns and the new status persists
- [ ] **TASK-04**: User can edit a task's title/description and delete a task
- [ ] **TASK-05**: User can click a task to open an expanded task view (description + session tabs)

### Git Worktrees

- [ ] **GIT-01**: Creating a task automatically creates a new git worktree and branch for that task (collision-safe branch naming, e.g. slug + task ID)
- [ ] **GIT-02**: When a task is marked Done, the app offers to delete its worktree, keeping the branch
- [ ] **GIT-03**: Worktree cleanup warns and requires confirmation if the worktree has uncommitted changes, and refuses while sessions are running in it

### Terminal Sessions

- [ ] **TERM-01**: User can start the task's agent session via an explicit Start button that spawns the `claude` CLI in a PTY with cwd set to the task's worktree
- [ ] **TERM-02**: User can interact with the session as a real terminal in the browser (full interactive TUI: plan mode, slash commands, permission prompts)
- [ ] **TERM-03**: Terminal supports resize (fit to pane), scrollback, and copy/paste including bracketed paste
- [ ] **TERM-04**: User can open additional bash session tabs running in the task's worktree
- [ ] **TERM-05**: Sessions keep running on the server when the browser tab closes; reopening the task reattaches with recent output replayed and a clean TUI redraw
- [ ] **TERM-06**: User can stop a running session from the task view; process tree is fully terminated (no orphaned subprocesses)
- [ ] **TERM-07**: WebSocket terminal endpoints validate Origin/Host so other websites cannot reach the shell (CSWSH protection)

### Status & Attention

- [ ] **STAT-01**: Each task card shows a live session status badge: working / idle / waiting for input / exited
- [ ] **STAT-02**: App detects when Claude Code needs input or finishes via per-session hooks injected at spawn (terminal bell as fallback)

### Recovery

- [ ] **RCVR-01**: On server startup, sessions recorded as running but no longer alive are reconciled to an exited state (no ghost sessions)
- [ ] **RCVR-02**: App persists each task's Claude session ID and, after a server restart, offers "Resume session" which relaunches via `claude --resume` in the worktree

### Review

- [ ] **REVW-01**: User can open a read-only diff tab in the task view showing the worktree's changes vs the base branch

### Storage

- [x] **STOR-01**: All projects, tasks, and session metadata persist in a local SQLite database across restarts
- [ ] **STOR-02**: App runs as a single local process serving the API and frontend at localhost (single user, no auth beyond local binding + per-instance token)

## v2 Requirements

### Notifications & Board Automation

- **NOTF-01**: Browser notification when an agent needs input or finishes; click jumps to the task
- **NOTF-02**: One-click "Move to In Review?" suggestion when the agent's Stop hook fires

### Maintenance

- **MAINT-01**: Stale-worktree list with manual purge

### Agent Integration

- **AGNT-01**: MCP server so agents can update their own task/board state

## Out of Scope

| Feature | Reason |
|---------|--------|
| External tracker sync (JIRA, Linear, GitHub Issues) | Explicit user exclusion; local SQLite only |
| Multi-user, auth, remote deployment | Single user at localhost by design |
| Desktop app packaging (Electron/Tauri) | Web app by design |
| Multiple agent CLIs (Codex, Gemini, ...) | Claude Code only for v1; per-CLI semantics multiply detection work |
| Structured agent chat UI (vibe-kanban style) | Real PTY is the core decision; reimplementing Claude's protocol loses fidelity |
| Merge/PR automation, commit UI, branch graph | User handles git manually in bash tabs; app's git scope is worktree create/cleanup |
| Automatic kanban column transitions | Heuristic misfires erode trust (research); badges + manual moves in v1 |
| Automatic/timed worktree cleanup | Caused ghost-run/lost-work bugs in vibe-kanban; explicit confirmed cleanup only |
| Full terminal history persistence across restarts | Unbounded scrollback serialization is complex; bounded replay + `claude --resume` covers it |
| Custom columns, labels, priorities | Lean v1: fixed columns, title + description + status |
| Embedded browser / dev-server preview panels | Security and scope; user opens dev servers in another tab |
| Token usage / cost analytics | `/cost` inside the Claude session covers it |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| PROJ-01 | Phase 1 | Pending |
| PROJ-02 | Phase 1 | Pending |
| PROJ-03 | Phase 1 | Pending |
| TASK-01 | Phase 1 | Pending |
| TASK-02 | Phase 1 | Pending |
| TASK-03 | Phase 1 | Pending |
| TASK-04 | Phase 1 | Pending |
| TASK-05 | Phase 1 | Pending |
| STOR-01 | Phase 1 | Complete |
| STOR-02 | Phase 1 | Pending |
| TERM-02 | Phase 2 | Pending |
| TERM-03 | Phase 2 | Pending |
| TERM-05 | Phase 2 | Pending |
| TERM-06 | Phase 2 | Pending |
| TERM-07 | Phase 2 | Pending |
| GIT-01 | Phase 3 | Pending |
| GIT-02 | Phase 3 | Pending |
| GIT-03 | Phase 3 | Pending |
| TERM-04 | Phase 3 | Pending |
| TERM-01 | Phase 4 | Pending |
| STAT-01 | Phase 4 | Pending |
| STAT-02 | Phase 4 | Pending |
| RCVR-01 | Phase 5 | Pending |
| RCVR-02 | Phase 5 | Pending |
| REVW-01 | Phase 5 | Pending |

**Coverage:**
- v1 requirements: 25 total
- Mapped to phases: 25
- Unmapped: 0 ✓

---
*Requirements defined: 2026-06-10*
*Last updated: 2026-06-10 after roadmap creation (traceability mapped)*
