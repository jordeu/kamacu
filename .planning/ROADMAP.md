# Roadmap: Kangent

## Overview

Kangent goes from empty repo to a working agent orchestrator in five phases. Phase 1 lands the deployment model (single Go binary serving an embedded React SPA) plus all conventional CRUD: projects sidebar and the kanban board, persisted in SQLite. Phase 2 is the de-risk spike: the session manager + WebSocket terminal bridge, proven against plain bash — the only genuinely hard subsystem, built before anything depends on it. Phase 3 wires git worktree-per-task isolation and uses the terminal engine to put bash tabs in every task. Phase 4 brings Claude Code itself into the PTY with hook-based status detection. Phase 5 closes the lifecycle: restart recovery via `claude --resume` and the read-only diff tab that makes In Review meaningful.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: Foundation — Projects & Board** - Single binary, SQLite, projects sidebar, kanban board with task CRUD (completed 2026-06-10)
- [x] **Phase 2: Terminal Engine** - Session manager + WebSocket bridge + browser terminal, proven against bash with detach/reattach (completed 2026-06-10)
- [ ] **Phase 3: Worktree Isolation & Bash Tabs** - Worktree + branch per task, safe confirmed cleanup, bash session tabs in the task view
- [ ] **Phase 4: Claude Code Agent Sessions** - Start button spawns `claude` in the worktree; live status badges via injected hooks
- [ ] **Phase 5: Recovery & Review** - Restart reconciliation, `claude --resume` recovery, read-only diff tab

## Phase Details

### Phase 1: Foundation — Projects & Board
**Goal**: User can organize projects and tasks on a persistent kanban board served by one local binary
**Depends on**: Nothing (first phase)
**Requirements**: STOR-01, STOR-02, PROJ-01, PROJ-02, PROJ-03, TASK-01, TASK-02, TASK-03, TASK-04, TASK-05
**Success Criteria** (what must be TRUE):
  1. User can run a single local process and open the app (React SPA embedded in the Go binary) in a browser at localhost
  2. User can create a project by pointing at a local git repo (invalid paths rejected), see it in the sidebar, switch boards by clicking, and rename or delete it (repo on disk untouched)
  3. User can create a task with title and markdown description, edit it, delete it, and open its expanded task view
  4. User can drag tasks between the four fixed columns (To Do / In Progress / In Review / Done) and the new status persists
  5. All projects and tasks survive a server restart (SQLite persistence)
**Plans**: 7 plans
**UI hint**: yes

Plans:
- [x] 01-01-PLAN.md — Backend foundation: Go module, SQLite store (pragma discipline), goose migrations, server entrypoint
- [x] 01-02-PLAN.md — REST API: projects CRUD with git repo-path validation, tasks CRUD, move endpoint with fractional ordering
- [x] 01-03-PLAN.md — Frontend scaffold: Vite + Tailwind 4 + shadcn (zinc dark), UI-SPEC tokens, API contract layer, router shell
- [x] 01-04-PLAN.md — Projects sidebar: collapsible list, add-project dialog with inline validation, rename, delete-with-confirm
- [x] 01-05-PLAN.md — Kanban board: dnd-kit four-column board, optimistic drag persistence, quick-add + New task dialog
- [x] 01-06-PLAN.md — Task view: full-page route, tab-strip seam, markdown edit/preview, delete confirmation
- [x] 01-07-PLAN.md — Single binary: embed + SPA fallback, Makefile, restart-persistence smoke test, human verification

### Phase 2: Terminal Engine
**Goal**: User can run a real shell in the browser whose lifetime belongs to the server, not the browser tab
**Depends on**: Phase 1
**Requirements**: TERM-02, TERM-03, TERM-05, TERM-06, TERM-07
**Success Criteria** (what must be TRUE):
  1. User can open an interactive terminal component in the browser running a real PTY-backed shell — typing, full TUI programs, and control sequences all work
  2. Terminal resizes to fit its pane, has scrollback, and supports copy/paste including bracketed paste
  3. User can close the browser tab while a command runs; the session keeps running server-side, and reopening reattaches with recent output replayed and a clean redraw
  4. User can stop a session from the UI and the entire process tree terminates — zero orphaned subprocesses
  5. WebSocket endpoints reject connections with wrong Origin/Host (strict loopback allowlist) and the server refuses to bind non-loopback addresses, so other websites cannot reach the shell (per-instance token deferred — D-20)
**Plans**: 5 plans
**UI hint**: yes

Plans:
- [x] 02-01-PLAN.md — Session engine: internal/session manager, PTY spawn/pump/ring, attach replay, process-group Stop (zero orphans)
- [x] 02-02-PLAN.md — Frontend foundation: xterm 6 deps, Vite WS proxy fix, zinc xterm theme, typed session REST hooks
- [x] 02-03-PLAN.md — WS bridge + REST + security: binary protocol, session endpoints, loopback/Host/Origin enforcement
- [x] 02-04-PLAN.md — TerminalPane + socket hook: xterm config, resize/clipboard, reconnect backoff, exited/connection banners
- [x] 02-05-PLAN.md — /terminal dev route, end-to-end Go integration test, human verification of all five success criteria

### Phase 3: Worktree Isolation & Bash Tabs
**Goal**: Every task gets its own isolated worktree and branch, with bash terminals working inside it and safe, confirmed cleanup
**Depends on**: Phase 1, Phase 2
**Requirements**: GIT-01, GIT-02, GIT-03, TERM-04
**Success Criteria** (what must be TRUE):
  1. Creating a task automatically creates a git worktree and branch with collision-safe naming (slug + task ID), placed outside the repo tree
  2. User can open one or more bash session tabs in the task view, each with cwd set to the task's worktree
  3. Marking a task Done offers to delete its worktree while keeping the branch; nothing is removed without explicit confirmation
  4. Cleanup warns and requires confirmation when the worktree has uncommitted changes, and refuses entirely while sessions are running in it
**Plans**: TBD
**UI hint**: yes

### Phase 4: Claude Code Agent Sessions
**Goal**: User can start, drive, leave, and reattach to a real Claude Code session per task, and see at a glance which agents need attention
**Depends on**: Phase 2, Phase 3
**Requirements**: TERM-01, STAT-01, STAT-02
**Success Criteria** (what must be TRUE):
  1. User can press an explicit Start button in the task view and get an interactive `claude` CLI session in a PTY with cwd set to the task's worktree — plan mode, slash commands, and permission prompts all work as in a native terminal
  2. Each task card shows a live status badge: working / idle / waiting for input / exited
  3. App detects when Claude Code needs input or finishes via per-session hooks injected at spawn, with terminal bell as fallback
  4. User can leave a running Claude session, reopen the task later, and get a correctly redrawn TUI (alt-screen replay handled, not corrupted)
**Plans**: TBD
**UI hint**: yes

### Phase 5: Recovery & Review
**Goal**: Work survives server restarts and finished agent work can be reviewed without leaving the app
**Depends on**: Phase 4
**Requirements**: RCVR-01, RCVR-02, REVW-01
**Success Criteria** (what must be TRUE):
  1. After a server restart, sessions recorded as running but no longer alive are reconciled to exited — no ghost sessions, and no session is ever auto-resumed into two PTYs
  2. For a task whose Claude session died with the server, user is offered "Resume session" which relaunches via `claude --resume` (persisted session ID) in the task's worktree
  3. User can open a read-only diff tab in the task view showing the worktree's changes vs the base branch
**Plans**: TBD
**UI hint**: yes

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4 → 5

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Foundation — Projects & Board | 0/7 | Complete    | 2026-06-10 |
| 2. Terminal Engine | 0/5 | Complete    | 2026-06-10 |
| 3. Worktree Isolation & Bash Tabs | 0/TBD | Not started | - |
| 4. Claude Code Agent Sessions | 0/TBD | Not started | - |
| 5. Recovery & Review | 0/TBD | Not started | - |

---
*Roadmap created: 2026-06-10*
*Granularity: coarse (5 phases)*
*Coverage: 25/25 v1 requirements mapped*
