# Requirements: Kangent — v1.13 Global Task

**Defined:** 2026-08-25
**Core Value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent agent session you can open, leave, and reattach to from the browser.

## v1.13 Requirements

A single global scratchpad — a task-like view with agent + bash tabs running directly in a configured repo/folder — for small checks unrelated to any project. NOT linked to any workspace or project; no worktree, no branch, no board. Research: `.planning/research/SUMMARY.md` (architecture: `global_task` singleton table + task-free "global scope" sessions — never a sentinel project/task row).

### Data Foundation

- [x] **GDATA-01**: A DB-enforced `global_task` singleton row (id=1) stores the configured root (path + managed/`github_repo` marker), the default `agent_id` (FK to agents, ON DELETE RESTRICT), and restart-resume session ids — seeded with the default agent at migration and guaranteed by an idempotent boot backfill
- [x] **GDATA-02**: `tmux_sessions` is rebuilt so global bash-tab rows can exist (nullable `task_id` / explicit scope discriminator with an XOR CHECK); existing rows, labels, and FK discipline survive byte-for-byte (goose upgrade-path tested on a seeded install)
- [x] **GDATA-03**: The startup tmux orphan sweep is scope-aware — global tmux tabs are never killed at startup (regression test: sweep-no-kill on a live global tab)

### Global Configuration

- [x] **GCONF-01**: User can configure the global root as a validated local folder path from a Global Settings section
- [x] **GCONF-02**: User can configure the global root as a GitHub repo (`owner/name`) — Kamacu gh-validates and clones it (v1.4 managed-clone pattern) into a separate global namespace that a project's gated delete can never touch; clone failures surface inline leaving no half-configured state
- [x] **GCONF-03**: User can pick the global task's default agent from the configured agents (v1.10 agents CRUD)
- [x] **GCONF-04**: Changing or clearing the root while global sessions are live is refused with a 409 + reasons list; on success the persisted resume ids are cleared
- [x] **GCONF-05**: Settings provides an "Open Global Task" affordance so the global view is reachable when idle (the Active Sessions bar covers the live case)

### Global View

- [ ] **GVIEW-01**: A `/global` route renders a task-like view with ONLY the Agent tab + bash tabs (no description tab, no diff tab, never on a kanban board) — the full TaskPage-derived shell with explicit Start and ⋯ Stop parity
- [x] **GVIEW-02**: The Agent tab starts the configured default agent in the global root cwd (no worktree, no branch); one agent per global task is enforced (409 on a second concurrent spawn)
- [x] **GVIEW-03**: Bash tabs spawn in the global root cwd with the same shell options as tasks (plain bash + invisible tmux)
- [ ] **GVIEW-04**: With no root configured, the global view shows an honest unconfigured state pointing to Settings — never a silent `$HOME` or cwd fallback

### Global Sessions

- [x] **GSESS-01**: Global sessions carry full task-parity semantics — persistent server-side PTYs, detach/reattach with ring-buffer replay, live working/waiting/idle status, scope-targeted Stop (never `StopAllForTask(0)`)
- [x] **GSESS-02**: After a server restart, global agent sessions reconcile and offer Resume (claude `--resume` / opencode `--resume`, keyed on the persisted singleton ids, engine-branched)
- [x] **GSESS-03**: Global tmux bash tabs survive a server restart and reattach invisibly (same semantics as task tabs — no Resume button)
- [x] **GSESS-04**: Global sessions are never auto-reaped (no Done-TTL analog, no PR reconcile pass) — immortal until explicitly stopped

### Integrations

- [ ] **GINT-01**: A live global agent session appears as a row in the global Active Sessions bar (server-synthesized label, non-nullable wire contract preserved) with click-through to `/global` and current-page highlight
- [x] **GINT-02**: MCP session tools (`list_sessions` / `get_session` / `get_session_output` / `subscribe_session_output`) handle task-less global sessions with honest labels — never 404 or garbled; the read-only terminal contract is unchanged
- [x] **GINT-03**: Global sessions are excluded from Activity stats and lists (the v1.12 task-keyed surface is unchanged by the global scope)

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Global Task follow-ups

- **GT-FUT-01**: Root-path legibility line in the global view header ("runs directly in \<root\>") — deselected at v1.13 scoping
- **GT-FUT-02**: Lightweight `git status --short` summary line in the view (P3 safety visibility) — deselected at v1.13 scoping
- **GT-FUT-03**: Promotion — turn scratch work into a real task (JetBrains F6 analog); needs its own design pass
- **GT-FUT-04**: Multiple global scratchpads (v1.13 is deliberately a singleton)
- **GT-FUT-05**: Per-workspace scratchpads
- **GT-FUT-06**: MCP write parity + `scope` filter for global sessions (parked with the banked v1.11 MCP follow-ups)
- **GT-FUT-07**: Sidebar entry for the global task (v1.13 reachability is bar + Settings only)

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Description / Diff tabs on the global view | It's a scratchpad — no task metadata, no base branch to diff against |
| Worktree/branch isolation for the global task | Deliberate: sessions run directly in the configured root; isolation is traded away for zero-ceremony |
| Kanban/board presence, workspace/project linkage | Not a task — that's the point |
| Auto-starting the agent on view open | Explicit Start only (standing app-wide decision) |
| TTL / auto-cleanup / reaping of global sessions | GSESS-04 guarantees immortality until explicit stop |
| Root-path header line + git-status summary | Deselected at scoping; parked as GT-FUT-01/02 |
| Notifications for global sessions | Rides the standing NOTF-01 deferral |
| Sentinel project/task row representation | Research P2: leaks into ~10 enumeration surfaces (boards, listings, stats, MCP tools); dedicated singleton instead |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| GDATA-01 | Phase 13 | Complete |
| GDATA-02 | Phase 13 | Complete |
| GDATA-03 | Phase 13 | Complete |
| GCONF-01 | Phase 14 | Complete |
| GCONF-02 | Phase 14 | Complete |
| GCONF-03 | Phase 14 | Complete |
| GCONF-04 | Phase 14 | Complete |
| GCONF-05 | Phase 16 | Complete |
| GVIEW-01 | Phase 16 | Pending |
| GVIEW-02 | Phase 15 | Complete |
| GVIEW-03 | Phase 15 | Complete |
| GVIEW-04 | Phase 16 | Pending |
| GSESS-01 | Phase 15 | Complete |
| GSESS-02 | Phase 15 | Complete |
| GSESS-03 | Phase 15 | Complete |
| GSESS-04 | Phase 15 | Complete |
| GINT-01 | Phase 16 | Pending |
| GINT-02 | Phase 15 | Complete |
| GINT-03 | Phase 15 | Complete |

**Coverage:**

- v1.13 requirements: 19 total
- Mapped to phases: 19
- Unmapped: 0 ✓

Phase 17 (Hardening & E2E) owns no requirements — it closes the end-to-end verification gates for GCONF-04, GSESS-02, GSESS-03, GINT-02, GINT-03, and the Phase-13 sentinel-leak invariant.

Mapping notes:

- GVIEW-02/GVIEW-03 map to Phase 15 (not 16) because their observable behaviors — agent runs in the global root cwd, 409 on a second concurrent spawn, bash tabs with the same shell options — are delivered and API-testable with the spawn path; the Agent/bash tab UI itself is covered by GVIEW-01 in Phase 16.
- GCONF-01..04 map to Phase 14 as API-level capabilities (curl-verifiable); their Settings-section UX is delivered in Phase 16 under GCONF-05.

---
*Requirements defined: 2026-08-25*
*Last updated: 2026-08-25 after roadmap creation (Phases 13–17)*
