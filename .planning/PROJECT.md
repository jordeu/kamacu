# Kangent

## What This Is

A local-only web app for organizing Claude Code agent sessions around projects and tasks. A left sidebar lists projects (each pointing at a local git repo checkout); the main area is a kanban board of tasks. Clicking a task expands a view with the main agent session (Claude Code CLI running in a PTY, rendered in a browser terminal) plus optional tabs with bash sessions. Single user, runs locally, accessed from a browser at localhost.

## Core Value

One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.

## Requirements

### Validated

- ✓ Projects sidebar — create/list/rename/delete projects pointing at local git repos, with path validation — Phase 1
- ✓ Kanban board per project with fixed columns (To Do / In Progress / In Review / Done), drag-and-drop with persisted manual order — Phase 1
- ✓ Tasks with title, markdown description, status; full CRUD and full-page task view with extensible tab strip — Phase 1
- ✓ All projects/tasks stored in local SQLite; single Go binary serves embedded SPA at localhost — Phase 1
- ✓ Real PTY-backed terminal in the browser: full interactive TUI, resize/scrollback/copy-paste, server-owned sessions that survive tab closes with replay on reattach, full-process-tree stop, Origin/Host-validated WebSocket with loopback-only binding — Phase 2
- ✓ Worktree-per-task isolation: task creation auto-creates `task/<slug>-<id>` branch + worktree under `~/.kangent/worktrees/`; bash session tabs run inside the worktree; confirmed cleanup with dirty-tree and running-session gates, branch always kept — Phase 3
- ✓ Claude Code agent sessions: explicit Start button spawns real `claude` in the worktree PTY (full TUI, inherits user settings, invisible additive status hooks, deterministic session IDs); live status dots (working/waiting/idle/exited) on cards, tab, and sidebar waiting chips; dimmed exited state with Reset session — Phase 4
- ✓ Server-side session persistence: sessions keep running when the browser tab closes and reattach with replay (Phase 2); after a server restart, silent reconciliation leaves no ghosts and the agent offers Resume session via `claude --resume <uuid>` from the persisted session ID — Phase 5
- ✓ Read-only diff tab: review everything a task changed vs the base branch merge-base (committed + uncommitted + untracked), collapsible unified diffs — Phase 5
- ✓ Global settings page (sidebar gear → full-page route, SQLite-backed, served over the API) with per-field commit, reset-to-default, and inline validation — Phase 6
- ✓ Configurable claude extra-params, pre-filled with `--dangerously-skip-permissions` (removable; applies at next spawn, running sessions unaffected) — Phase 6
- ✓ Configurable worktree base location (new worktrees only; existing stay put) — Phase 6
- ✓ Configurable bash-tab shell (dropdown, bash-only for now; bash de-hardcoded) — Phase 6
- ✓ Configurable branch-name token template (default `task/{slug}-{id}`, validated to a legal ref at save and create time) — Phase 6
- ✓ Task-view header layout spans full page width (title row + three-dots actions menu) — Phase 6

### Active

**Milestone v1.2: Quota & Resumable Shells**

- [ ] Claude quota indicator in the top-right of the main area (compact label + 5h usage bar, visible on board and task view)
- [ ] Quota hover popup with all quotas (5h, 7d, model-specific): percentage bars, reset times, "Updated Xm ago", manual refresh
- [ ] Background quota auto-poll (~60s) plus manual refresh
- [ ] "tmux" added to the global shell setting dropdown (existing API-driven seam, SHELL-FUT-01)
- [ ] tmux-backed tabs detach on close instead of dying; reopening reattaches
- [ ] tmux-backed tabs survive a Kangent server restart with a Resume affordance that reattaches

### Out of Scope

- External task services (JIRA, Linear, etc.) — explicitly excluded; local database only
- Multi-user support, auth, remote deployment — single user at localhost only
- Desktop app packaging (Electron/Tauri) — this is a web app by design
- Multiple agent CLIs (Codex, Gemini, etc.) — Claude Code only for v1
- Merge/PR automation from the app — git integration beyond worktree create/cleanup is manual, done by the user in the terminal
- Custom kanban columns, labels, priorities — fixed columns and lean task cards for v1
- Auto-starting agents on task creation — sessions start only via explicit Start button

## Current State

**Shipped: v1.0 MVP (2026-06-11)** — 5 phases, 28 plans, 74 tasks. ~10,400 LOC Go + ~5,800 LOC TS/React. Single `make build` binary serving the embedded SPA at `127.0.0.1:7333`.

Kangent v1 does the whole loop: create a project on a local git repo → add a task (worktree + `task/<slug>-<id>` branch auto-created under `~/.kangent/worktrees/`) → Start a real `claude` session in the worktree PTY → watch the board as a dispatcher (status dots, amber when an agent needs you) → leave and reattach across tab closes and server restarts (`claude --resume`) → review the diff vs merge-base → mark Done with gated worktree cleanup. Stack: Go stdlib mux + modernc SQLite + creack/pty + coder/websocket; React 19 + Vite + Tailwind 4 + shadcn + xterm.js 6 + dnd-kit + TanStack Query.

## Current Milestone: v1.2 Quota & Resumable Shells

**Goal:** Make Kangent a better dispatcher cockpit: see Claude quota usage at a glance before starting agents, and make bash tabs durable via tmux-backed shells that survive tab closes and server restarts.

**Target features:**
- Claude quota indicator — compact "Claude 5h" label + usage bar in the top-right of the main area (board and task view); hover popup shows all quotas (5h, 7d, model-specific) with percentage bars, reset times, "Updated Xm ago", and manual refresh (SlayZone-style)
- Background quota auto-poll (~60s) plus manual refresh
- "tmux" shell type in the existing global shell setting dropdown (the API-driven seam built in v1.1 for SHELL-FUT-01)
- tmux-backed tabs detach on close (not kill) and reattach on reopen; after a Kangent restart they offer Resume, reattaching to the still-running tmux session

**Key context:**
- Quota data source needs empirical research: how SlayZone / `claude /usage` obtain quota data (likely the OAuth usage endpoint), auth, and rate behavior — verify against the installed claude version before planning
- tmux availability must be detected gracefully (option valid only when `tmux` resolves), mirroring v1.1's LookPath shell validation
- Both features ride existing seams: data-driven shell dropdown, v1.1 session lifecycle/resume UX patterns

## Deferred (post-v1.1)

Parked candidates: browser notifications on waiting/finished (NOTF-01), one-click "Move to In Review?" on the Stop hook (NOTF-02), stale-worktree purge list (MAINT-01), MCP server for agent board access (AGNT-01), multiple agent CLIs, per-project settings overrides. One carried bug to confirm: whether plan-mode exit-plan approval triggers the amber waiting dot (research OQ1) — still unobserved as of Phase 6's gate (user approved without reporting it).

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
| Claude Code CLI in a PTY (not Agent SDK chat UI) | Full interactive CLI experience with zero reimplementation | ✓ Good — full TUI/plan-mode/hooks work unmodified (Phase 4) |
| Worktree + branch auto-created per task | Isolates concurrent agent sessions; vibe-kanban-proven model | ✓ Good (Phase 3) |
| Sessions persist server-side | Browser is a detachable view; work survives tab closes | ✓ Good — detach/reattach + restart resume (Phases 2, 5) |
| Explicit Start button for agent sessions | Most control, least surprise; no accidental agent runs | ✓ Good (Phase 4) |
| Go backend + React frontend | User preference | ✓ Good — single embedded binary |
| Manual git workflow, app only cleans up worktrees | Keeps v1 scope lean; user merges/PRs in terminal | ✓ Good (Phase 3) |
| Fixed columns: To Do / In Progress / In Review / Done | In Review holds agent-finished work awaiting human check | ✓ Good (Phase 1) |
| D-51 reversed: `--dangerously-skip-permissions` on by default (Phase 6, AGENT-02) | Dispatcher workflow favors unattended agents; flag is a removable settings default, so interactive prompts are one edit away | ✓ Intentional — documented side effect: amber waiting dot rarely fires while the flag is active |

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
*Last updated: 2026-06-11 — milestone v1.2 Quota & Resumable Shells started*
