# Requirements

This file is the explicit capability and coverage contract for the project.

## Active

## Validated

### R001 — Kanban board of tasks per project with worktree-per-task isolation — each task gets its own git worktree (task/<slug>-<id>) under ~/.kamacu/worktrees, with drag-and-drop (dnd-kit) across four columns (To Do / In Progress / Review / Done) and task CRUD.
- Class: core-capability
- Status: validated
- Description: Kanban board of tasks per project with worktree-per-task isolation — each task gets its own git worktree (task/<slug>-<id>) under ~/.kamacu/worktrees, with drag-and-drop (dnd-kit) across four columns (To Do / In Progress / Review / Done) and task CRUD.
- Why it matters: This is the product's spine — the unit of agent work is a task, and isolation prevents tasks clobbering each other's working trees.
- Source: v1.0 (Phases 1, 3)
- Primary owning slice: internal/worktree, internal/store, internal/api
- Validation: Shipped v1.0; full Go test suite green across session/store/worktree packages; human-verified at v1.0 gate.

### R002 — Server-owned PTY terminal engine with binary WebSocket bridge, ring-buffer scrollback replay, and detach/reattach — sessions outlive their WS connections. xterm.js 6.0 (WebGL + DOM fallback) renders in the browser.
- Class: core-capability
- Status: validated
- Description: Server-owned PTY terminal engine with binary WebSocket bridge, ring-buffer scrollback replay, and detach/reattach — sessions outlive their WS connections. xterm.js 6.0 (WebGL + DOM fallback) renders in the browser.
- Why it matters: The terminal is how the user sees and drives every agent/bash session. Detach/reattach is the core value prop (open, leave, come back). This was the riskiest subsystem and was proven first.
- Source: v1.0 (Phase 2)
- Primary owning slice: internal/session, internal/ws, web/src/components/terminal
- Validation: Shipped v1.0 Phase 2; proven against plain bash before anything depended on it; internal/ws + internal/session tests green.

### R003 — Claude Code agent session orchestration — the real claude CLI spawned in a worktree PTY, with status hooks (working/waiting/idle/exited), dispatcher board status dots + sidebar waiting chips, and claude --resume recovery on a dead session. The app spawns claude; it never reimplements it.
- Class: core-capability
- Status: validated
- Description: Claude Code agent session orchestration — the real claude CLI spawned in a worktree PTY, with status hooks (working/waiting/idle/exited), dispatcher board status dots + sidebar waiting chips, and claude --resume recovery on a dead session. The app spawns claude; it never reimplements it.
- Why it matters: Claude is the agent doing the actual work; the app is the organizer around it. Spawning (not reimplementing) preserves plan mode, slash commands, and permission prompts.
- Source: v1.0 (Phase 4), v1.0 (Phase 5)
- Primary owning slice: internal/session, internal/api
- Validation: Shipped v1.0 Phases 4–5; fake-claude stub keeps tests off the real binary/API; --resume recovery human-verified.

### R004 — GitHub PR review integration (gh-gated): a per-project Review column (awaiting your review + recently-reviewed), open-a-review on the PR's real head branch (git fetch refs/pull/<n>/head + worktree add, never gh pr checkout), diff vs the PR base merge-base, and auto-cleanup of merged/closed PR worktrees. Degrades cleanly when gh is absent.
- Class: core-capability
- Status: validated
- Description: GitHub PR review integration (gh-gated): a per-project Review column (awaiting your review + recently-reviewed), open-a-review on the PR's real head branch (git fetch refs/pull/<n>/head + worktree add, never gh pr checkout), diff vs the PR base merge-base, and auto-cleanup of merged/closed PR worktrees. Degrades cleanly when gh is absent.
- Why it matters: Lets the user drive PR review in the same task/terminal/diff shell as manual work, without leaving the app. The gh pr checkout worktree-unawareness made the git fetch path a verified safety property.
- Source: v1.3 (Phases 10–13)
- Primary owning slice: internal/github, internal/reaper, internal/api
- Validation: Shipped v1.3; gh pr checkout worktree-unawareness spiked pre-planning (cli/cli#972); shared github.Service integration-audited across 6 seams.

### R005 — Project workspaces — group projects into named workspaces (e.g. Personal, Professional), switchable from an expanded-only sidebar switcher. One .filter scopes both sidebar surfaces to the active workspace; active workspace persists in localStorage; projects transferable between workspaces; the global Active Sessions bar stays cross-workspace.
- Class: core-capability
- Status: validated
- Description: Project workspaces — group projects into named workspaces (e.g. Personal, Professional), switchable from an expanded-only sidebar switcher. One .filter scopes both sidebar surfaces to the active workspace; active workspace persists in localStorage; projects transferable between workspaces; the global Active Sessions bar stays cross-workspace.
- Why it matters: At scale, a flat project list gets unwieldy; workspaces give the user lightweight grouping without changing the cross-project visibility that makes the app useful.
- Source: v1.9 (Phases 25–26)
- Primary owning slice: internal/api (workspaces.go), web/src/lib/useActiveWorkspace, web/src/components/sidebar
- Validation: Shipped v1.9 (12/12 requirements WSDATA/WSMGMT/WSNAV/WSPROJ/WSBAR); human-verified gate 23/23 must-haves; migration 00012 staged-upgrade tested.

### R006 — Local-only, single-user — the app runs as one process serving API + embedded SPA at localhost, with SQLite on local disk and loopback-only network binding. No multi-user, no external services, no remote deployment.
- Class: constraint
- Status: validated
- Description: Local-only, single-user — the app runs as one process serving API + embedded SPA at localhost, with SQLite on local disk and loopback-only network binding. No multi-user, no external services, no remote deployment.
- Why it matters: This is the product boundary: a personal tool for driving one's own agent work, not a hosted service. It shapes every security, scaling, and auth decision (none needed beyond loopback).
- Source: PROJECT.md constraints
- Validation: Held across all 10 milestones; never scoped to multi-user/remote.

## Deferred

### R007 — Workspace icon/color identity — a monogram + color (like projects have) for each workspace, shown in the workspace switcher and header. Workspaces are name-only in v1.9.
- Class: differentiator
- Status: deferred
- Description: Workspace icon/color identity — a monogram + color (like projects have) for each workspace, shown in the workspace switcher and header. Workspaces are name-only in v1.9.
- Why it matters: Visual identity helps distinguish workspaces at a glance once the list grows; mirrors the proven project-icon pattern from v1.7.
- Source: v1.9 future backlog (WSFUT-01)

### R008 — Reorder workspaces in the switcher (drag-to-reorder or manual order). Workspaces are currently name-sorted only.
- Class: differentiator
- Status: deferred
- Description: Reorder workspaces in the switcher (drag-to-reorder or manual order). Workspaces are currently name-sorted only.
- Why it matters: Lets the user put frequently-used workspaces first rather than living with alphabetical order.
- Source: v1.9 future backlog (WSFUT-02)

### R009 — Bulk / multi-select transfer of projects between workspaces. Currently transfer is one project at a time via the ⋯ "Move to" submenu.
- Class: differentiator
- Status: deferred
- Description: Bulk / multi-select transfer of projects between workspaces. Currently transfer is one project at a time via the ⋯ "Move to" submenu.
- Why it matters: When reorganizing many projects into a new workspace, one-at-a-time transfer is tedious.
- Source: v1.9 future backlog (WSFUT-03)

### R010 — Add-project (+) and Settings (gear) affordances in the collapsed icon rail. Currently the collapsed rail shows only project avatars; add/settings are reachable only when the sidebar is expanded.
- Class: differentiator
- Status: deferred
- Description: Add-project (+) and Settings (gear) affordances in the collapsed icon rail. Currently the collapsed rail shows only project avatars; add/settings are reachable only when the sidebar is expanded.
- Why it matters: In the collapsed-rail mode the user has no way to add a project or reach settings without expanding first; adding small affordances keeps the rail a full mini-mode.
- Source: v1.7 future backlog (ICON-FUT-05)

### R011 — Browser/OS notifications for waiting agent sessions. Currently alerting is in-bar only (pulsing-amber + count in the Active Sessions bar).
- Class: differentiator
- Status: deferred
- Description: Browser/OS notifications for waiting agent sessions. Currently alerting is in-bar only (pulsing-amber + count in the Active Sessions bar).
- Why it matters: When the app is in a background tab, the user can't see an agent is waiting for them; a desktop notification would surface it without watching the bar.
- Source: v1.6 future backlog (SBAR-FUT-03)

## Out of Scope

## Traceability

| ID | Class | Status | Primary owner | Supporting | Proof |
|---|---|---|---|---|---|
| R001 | core-capability | validated | internal/worktree, internal/store, internal/api | none | Shipped v1.0; full Go test suite green across session/store/worktree packages; human-verified at v1.0 gate. |
| R002 | core-capability | validated | internal/session, internal/ws, web/src/components/terminal | none | Shipped v1.0 Phase 2; proven against plain bash before anything depended on it; internal/ws + internal/session tests green. |
| R003 | core-capability | validated | internal/session, internal/api | none | Shipped v1.0 Phases 4–5; fake-claude stub keeps tests off the real binary/API; --resume recovery human-verified. |
| R004 | core-capability | validated | internal/github, internal/reaper, internal/api | none | Shipped v1.3; gh pr checkout worktree-unawareness spiked pre-planning (cli/cli#972); shared github.Service integration-audited across 6 seams. |
| R005 | core-capability | validated | internal/api (workspaces.go), web/src/lib/useActiveWorkspace, web/src/components/sidebar | none | Shipped v1.9 (12/12 requirements WSDATA/WSMGMT/WSNAV/WSPROJ/WSBAR); human-verified gate 23/23 must-haves; migration 00012 staged-upgrade tested. |
| R006 | constraint | validated | none | none | Held across all 10 milestones; never scoped to multi-user/remote. |
| R007 | differentiator | deferred | none | none | unmapped |
| R008 | differentiator | deferred | none | none | unmapped |
| R009 | differentiator | deferred | none | none | unmapped |
| R010 | differentiator | deferred | none | none | unmapped |
| R011 | differentiator | deferred | none | none | unmapped |

## Coverage Summary

- Active requirements: 0
- Mapped to slices: 0
- Validated: 6 (R001, R002, R003, R004, R005, R006)
- Unmapped active requirements: 0
