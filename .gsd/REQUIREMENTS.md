# Requirements

This file is the explicit capability and coverage contract for the project.

## Active

### R012 — An `agents` table (migration 00013) holds every runnable agent: name, command template, engine ('claude'|'custom'), is_default, is_system. Claude Code is pre-seeded as a non-deletable (is_system=1) row, marked the default (is_default=1). Exactly one row is_default=1 at all times (flag-keyed, rename-proof — mirrors workspaces D-002).
- Class: core-capability
- Status: active
- Description: An `agents` table (migration 00013) holds every runnable agent: name, command template, engine ('claude'|'custom'), is_default, is_system. Claude Code is pre-seeded as a non-deletable (is_system=1) row, marked the default (is_default=1). Exactly one row is_default=1 at all times (flag-keyed, rename-proof — mirrors workspaces D-002).
- Why it matters: Agents must be first-class configurable data, not hardcoded. The table is the single source of truth that both the spawn engine and the settings UI read. The claude seed guarantees the v1.9-and-prior default behavior survives unchanged for existing projects.
- Source: M001
- Primary owning slice: internal/store, internal/api

### R013 — projects.agent_id foreign key (NOT NULL ... REFERENCES agents(id) ON DELETE RESTRICT, migration 00013), backfilling every existing project to the default agent in-SQL, plus an idempotent BackfillAgents startup hook (mirroring BackfillWorkspaces / BackfillProjectIcons) guaranteeing a project always has an agent.
- Class: core-capability
- Status: active
- Description: projects.agent_id foreign key (NOT NULL ... REFERENCES agents(id) ON DELETE RESTRICT, migration 00013), backfilling every existing project to the default agent in-SQL, plus an idempotent BackfillAgents startup hook (mirroring BackfillWorkspaces / BackfillProjectIcons) guaranteeing a project always has an agent.
- Why it matters: Per-project agent selection is the headline feature. The FK + RESTRICT + backfill ensure a project can never be agent-less and an agent in use can't be orphaned — the same invariant shape that made v1.9 workspaces safe.
- Source: M001
- Primary owning slice: internal/store, internal/api

### R014 — The user can create a custom agent from global Settings by providing a name and a command template (e.g. "gemini", "aider --model sonnet"). The worktree is always the cwd; {{worktree}} and {{session_id}} placeholders are available for power users to wire custom flags.
- Class: primary-user-loop
- Status: active
- Description: The user can create a custom agent from global Settings by providing a name and a command template (e.g. "gemini", "aider --model sonnet"). The worktree is always the cwd; {{worktree}} and {{session_id}} placeholders are available for power users to wire custom flags.
- Why it matters: This is the primary user loop of the milestone — adding a non-Claude agent. The templated-command model (D-M001-1) lets users run arbitrary CLIs in the worktree and optionally wire resume/session flags for capable agents.
- Source: M001
- Primary owning slice: internal/api, web/src/components/settings

### R015 — The user can edit any agent's name and command. The claude seed's engine stays 'claude' (its hook/resume plumbing is internal, not user-editable), but its name and command (binary path) are editable. Custom agents are fully editable. The is_system seed is non-deletable.
- Class: primary-user-loop
- Status: active
- Description: The user can edit any agent's name and command. The claude seed's engine stays 'claude' (its hook/resume plumbing is internal, not user-editable), but its name and command (binary path) are editable. Custom agents are fully editable. The is_system seed is non-deletable.
- Why it matters: Users need to correct a mis-typed command or point the claude seed at a specific binary path. Locking the claude engine (not the whole row) keeps the internal hook machinery safe while allowing the legitimate binary-path edit.
- Source: M001
- Primary owning slice: internal/api, web/src/components/settings

### R016 — The claude engine spawn path is preserved byte-for-byte: session-id (fresh) / --resume (reattach), the --settings hook overlay (Notification/Stop/SessionStart), BEL fallback scanning, working/waiting/idle status, tasks.claude_session_id resume, and the Claude-OAuth quota indicator all continue to work with zero regression for any project whose agent is the claude seed.
- Class: core-capability
- Status: active
- Description: The claude engine spawn path is preserved byte-for-byte: session-id (fresh) / --resume (reattach), the --settings hook overlay (Notification/Stop/SessionStart), BEL fallback scanning, working/waiting/idle status, tasks.claude_session_id resume, and the Claude-OAuth quota indicator all continue to work with zero regression for any project whose agent is the claude seed.
- Why it matters: The existing Claude integration is the app's primary agent and is verified across 10 milestones. Generalizing the spawn path must not regress it. The fake-claude test stub must pass unchanged as regression proof.
- Source: M001
- Primary owning slice: internal/session

### R017 — The custom engine spawn path renders the agent's command template (shell-words split, cwd=worktree, {{worktree}}/{{session_id}} substitution) in the task's worktree PTY. Custom-agent sessions report running/exited status only (PTY process liveness); no hook overlay, no BEL scanning, no --resume. Terminal detach/reattach works mid-run; on process exit, "Reset session" starts a fresh spawn (same as a bash tab today).
- Class: core-capability
- Status: active
- Description: The custom engine spawn path renders the agent's command template (shell-words split, cwd=worktree, {{worktree}}/{{session_id}} substitution) in the task's worktree PTY. Custom-agent sessions report running/exited status only (PTY process liveness); no hook overlay, no BEL scanning, no --resume. Terminal detach/reattach works mid-run; on process exit, "Reset session" starts a fresh spawn (same as a bash tab today).
- Why it matters: This is the mechanism that makes non-Claude agents actually run. The generic-status choice (D-M001-2) keeps it honest — we report what we can reliably know (process liveness) rather than guessing TUI state.
- Source: M001
- Primary owning slice: internal/session, internal/api

### R018 — The user can delete a custom agent only when no project uses it (block-until-unassigned, never cascade-reassign) and never the claude seed (is_system=1 is non-deletable). The ON DELETE RESTRICT foreign key is the backstop; the count-guard gives a clear error message telling the user to reassign its projects first.
- Class: primary-user-loop
- Status: active
- Description: The user can delete a custom agent only when no project uses it (block-until-unassigned, never cascade-reassign) and never the claude seed (is_system=1 is non-deletable). The ON DELETE RESTRICT foreign key is the backstop; the count-guard gives a clear error message telling the user to reassign its projects first.
- Why it matters: Mirrors the v1.9 workspace-delete guard (WSMGMT-03/04, D006 destructive-defaults-leave-it). Never bulldoze a project's agent as a side effect of deleting another agent, and never let the claude seed disappear and orphan every legacy project.
- Source: M001
- Primary owning slice: internal/api, internal/store

### R019 — The user can set any agent as the global default; exactly one agent is_default=1 at all times. Setting a new default clears the previous flag in the same transaction. The default is rename-proof (keyed off the is_default flag, never the literal name "Claude" — mirrors workspaces D-002). Existing projects do NOT auto-follow a default change; their agent_id is stable.
- Class: core-capability
- Status: active
- Description: The user can set any agent as the global default; exactly one agent is_default=1 at all times. Setting a new default clears the previous flag in the same transaction. The default is rename-proof (keyed off the is_default flag, never the literal name "Claude" — mirrors workspaces D-002). Existing projects do NOT auto-follow a default change; their agent_id is stable.
- Why it matters: The default agent is what new projects and the claude-compatibility baseline depend on. The exactly-one invariant plus rename-proof flag-keying guarantee a permanent home and a stable default even if the user renames the claude seed.
- Source: M001
- Primary owning slice: internal/api, internal/store

### R020 — A project's agent resolves at spawn time from projects.agent_id (read fresh inside the spawn handler, never cached client-side). A newly created project is assigned the current global default agent. Changing a project's agent applies to the NEXT spawn only (read-at-use, no restart); running sessions are unaffected.
- Class: primary-user-loop
- Status: active
- Description: A project's agent resolves at spawn time from projects.agent_id (read fresh inside the spawn handler, never cached client-side). A newly created project is assigned the current global default agent. Changing a project's agent applies to the NEXT spawn only (read-at-use, no restart); running sessions are unaffected.
- Why it matters: Read-at-use is the established settings pattern (D011, v1.1 SET-03) and makes agent changes structural rather than requiring a restart. New-project-uses-default means a user who never configures agents gets Claude exactly as today.
- Source: M001
- Primary owning slice: internal/api, internal/session

### R021 — Global Settings gains an Agents section: an ordered list of agents (name + command + engine badge), add (name + command-template fields with inline help for {{worktree}}/{{session_id}} placeholders), edit, delete (disabled with a tooltip when is_system or in-use by a project), and set-default (radio/flag). The claude seed renders as a system agent with its name + command editable but no delete affordance.
- Class: primary-user-loop
- Status: active
- Description: Global Settings gains an Agents section: an ordered list of agents (name + command + engine badge), add (name + command-template fields with inline help for {{worktree}}/{{session_id}} placeholders), edit, delete (disabled with a tooltip when is_system or in-use by a project), and set-default (radio/flag). The claude seed renders as a system agent with its name + command editable but no delete affordance.
- Why it matters: This is the management surface for the headline feature. Reuses the v1.9 WorkspaceNameDialog + ManageWorkspacesDialog shape (per D008 reuse-before-invent) so the CRUD UI is low-risk and visually consistent.
- Source: M001
- Primary owning slice: web/src/components/settings, web/src/pages/SettingsPage

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
| R012 | core-capability | active | internal/store, internal/api | none | unmapped |
| R013 | core-capability | active | internal/store, internal/api | none | unmapped |
| R014 | primary-user-loop | active | internal/api, web/src/components/settings | none | unmapped |
| R015 | primary-user-loop | active | internal/api, web/src/components/settings | none | unmapped |
| R016 | core-capability | active | internal/session | none | unmapped |
| R017 | core-capability | active | internal/session, internal/api | none | unmapped |
| R018 | primary-user-loop | active | internal/api, internal/store | none | unmapped |
| R019 | core-capability | active | internal/api, internal/store | none | unmapped |
| R020 | primary-user-loop | active | internal/api, internal/session | none | unmapped |
| R021 | primary-user-loop | active | web/src/components/settings, web/src/pages/SettingsPage | none | unmapped |

## Coverage Summary

- Active requirements: 10
- Mapped to slices: 0
- Validated: 6 (R001, R002, R003, R004, R005, R006)
- Unmapped active requirements: 10
