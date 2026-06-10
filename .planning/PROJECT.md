# Kangent

## What This Is

A local-only web app for organizing Claude Code agent sessions around projects and tasks. A left sidebar lists projects (each pointing at a local git repo checkout); the main area is a kanban board of tasks. Clicking a task expands a view with the main agent session (Claude Code CLI running in a PTY, rendered in a browser terminal) plus optional tabs with bash sessions. Single user, runs locally, accessed from a browser at localhost.

## Core Value

One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] Projects sidebar — create/list projects, each pointing at a local directory containing a git repo checkout
- [ ] Kanban board per project with fixed columns: To Do / In Progress / In Review / Done
- [ ] Tasks have title, markdown description, and status (column); tasks can be created, edited, moved between columns, and deleted
- [ ] Creating a task automatically creates a new git worktree and branch for that task in the project repo
- [ ] Task view with an explicit Start button that spawns a Claude Code CLI session in a PTY, working directory set to the task's worktree
- [ ] Agent session rendered as an interactive terminal in the browser (full interactive CLI experience)
- [ ] Optional bash session tabs in the task view, running in the task's worktree
- [ ] Sessions keep running on the server when the browser tab closes; reopening the task reattaches to the live session
- [ ] All projects/tasks stored in a local database (SQLite or similar) — no external services
- [ ] Marking a task done offers to clean up its worktree (branch is kept)

### Out of Scope

- External task services (JIRA, Linear, etc.) — explicitly excluded; local database only
- Multi-user support, auth, remote deployment — single user at localhost only
- Desktop app packaging (Electron/Tauri) — this is a web app by design
- Multiple agent CLIs (Codex, Gemini, etc.) — Claude Code only for v1
- Merge/PR automation from the app — git integration beyond worktree create/cleanup is manual, done by the user in the terminal
- Custom kanban columns, labels, priorities — fixed columns and lean task cards for v1
- Auto-starting agents on task creation — sessions start only via explicit Start button

## Context

- Inspiration: layout and basic features of SlayZone (https://github.com/debuglebowski/SlayZone), but as a web app instead of a desktop app, in the spirit of vibe-kanban (https://github.com/BloopAI/vibe-kanban)
- Vibe-kanban-style worktree-per-task isolation: each task works on its own branch in its own worktree, so multiple agent sessions never collide in one checkout
- The agent is the real `claude` CLI in a pseudo-terminal — not an SDK-driven chat UI — so the full interactive experience (plan mode, slash commands, permission prompts) works as-is
- Server-side session persistence means the backend owns PTYs; the browser is just an attached view (xterm.js or similar). A server restart loses live processes; resuming via `claude --resume` is the recovery path

## Constraints

- **Tech stack**: Go backend, React frontend — user's choice
- **Deployment**: Single binary/process serving API + static frontend at localhost — local-only by design
- **Storage**: Local database (e.g., SQLite), no external services
- **Agent**: Claude Code CLI must be installed on the host; the app spawns it, never reimplements it

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Claude Code CLI in a PTY (not Agent SDK chat UI) | Full interactive CLI experience with zero reimplementation | — Pending |
| Worktree + branch auto-created per task | Isolates concurrent agent sessions; vibe-kanban-proven model | — Pending |
| Sessions persist server-side | Browser is a detachable view; work survives tab closes | — Pending |
| Explicit Start button for agent sessions | Most control, least surprise; no accidental agent runs | — Pending |
| Go backend + React frontend | User preference | — Pending |
| Manual git workflow, app only cleans up worktrees | Keeps v1 scope lean; user merges/PRs in terminal | — Pending |
| Fixed columns: To Do / In Progress / In Review / Done | In Review holds agent-finished work awaiting human check | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd:transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd:complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-06-10 after initialization*
