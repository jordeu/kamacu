---
gsd_state_version: 1.0
milestone: v1.11
milestone_name: Kamacu MCP Server
current_phase: 08
current_phase_name: sessions-terminal-read-access
status: executing
stopped_at: Completed 08-02-mcp-session-tools-PLAN.md
last_updated: "2026-07-23T06:24:57.200Z"
last_activity: 2026-07-23
last_activity_desc: Phase 08 execution started
progress:
  total_phases: 4
  completed_phases: 2
  total_plans: 10
  completed_plans: 9
  percent: 50
---

# Project State

**Current focus:** Phase 08 — sessions-terminal-read-access

See: .planning/PROJECT.md (updated 2026-07-21)

**Core value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent agent session you can open, leave, and reattach to from the browser.

## Deferred Items

Items acknowledged and deferred at milestone close on 2026-07-11 (carried over from v1.10 — none are v1.11 work):

| Category | Item | Status |
|----------|------|--------|
| quick_task | 260613-osu-warn-when-a-linked-github-repo-cannot-be | unknown |
| quick_task | 260613-ph5-make-github-repo-link-validation-mandato | unknown |
| quick_task | 260616-8l7-the-refresh-button-at-review-column-seem | awaiting-human-verify |
| quick_task | 260618-mlu-when-the-status-bottom-bar-is-expanded-a | unknown |
| quick_task | 260625-9db-the-bottom-status-bar-is-hidding-the-bot | unknown |
| quick_task | 260626-hwd-add-an-opt-in-insecure-allow-remote-flag | unknown |

Known verification overrides: 6 (all prior-milestone quick tasks, none v1.11)

## Current Position

Phase: 08 (sessions-terminal-read-access) — EXECUTING
Plan: 3 of 3
Status: Ready to execute
Last activity: 2026-07-23 — Phase 08 execution started

Progress: [░░░░░░░░░░] 0% (v1.11 milestone-scoped)

## Session

**Last session:** 2026-07-23T06:24:57.187Z
**Stopped at:** Completed 08-02-mcp-session-tools-PLAN.md
**Resume file:** None

## Performance Metrics

| Phase | Plan | Duration | Notes |
|-------|------|----------|-------|
| Phase 06 P01 | 12 min | 2 tasks | 7 files |
| Phase 06 P03 | 8 min | 2 tasks | 7 files |
| Phase 06 P02 | 22 min | 2 tasks | 8 files |
| Phase Phase 07 P01 | 14 min | 2 tasks | 11 files |
| Phase 07 P02 | 7 min | 2 tasks | 3 files |
| Phase Phase 07 P03 | 7 min | 2 tasks tasks | 2 files files |
| Phase 07 P04 | 7min | 2 tasks | 2 files |
| Phase 08 P01 | 21 min | 2 tasks | 2 files |
| Phase 08 P02 | 10 min | 2 tasks | 3 files |

## Decisions

- [Phase ?]: Phase 07 / Plan 01: Extracted bridge.call as the shared response-handling helper (chosen YES over inline — halves per-tool line count for the 13 Phase 07 tools; structurally enforces the every-tool-is-the-same-shape invariant)
- [Phase ?]: Phase 07 / Plan 01: list_projects InputSchema declared workspace_id (integer, optional) as the ONLY property; stale project_id no-op arg REMOVED per 07-RESEARCH Pitfall 6
- [Phase ?]: Phase 07 / Plan 02: Widened bridge.call from HTTP 200-only to full 2xx range — Kamacu POST returns 201 (Created), DELETE returns 204 (No Content); 200-only would have surfaced every successful create_task / delete_task as an error to the agent
- [Phase ?]: Phase 07 / Plan 02: move_task InputSchema exposes ONLY task_id + status (D-06); handler marshals {status, after_id: nil} unconditionally so Kamacu's MIN(position)-1.0 top-of-column path is the only one taken
- [Phase ?]: Phase 07 / Plan 02: update_task uses *string pointer fields (Title, Description) per D-03 — nil = omitted (leave untouched), "" = explicit clear (legal for description); body map built conditionally so omitted keys never reach Kamacu's partial-PATCH
- [Phase ?]: Phase 07 / Plan 03: create_project body marshals {name, repo_path, repo} unconditionally and workspace_id only when non-nil (D-04) - Kamacu's strings.TrimSpace(req.Repo) != empty check is the canonical dispatch; the bridge sends every field as-is with ZERO type detection
- [Phase ?]: Phase 07 / Plan 03: update_project args struct has NO WorkspaceID and NO AgentID fields (D-03 / 07-RESEARCH Pitfall 3) - workspace transfer has its own tool in a later plan; agent reassignment is Out of Scope (MCPMORE-01). Double-lock ensures excluded fields never reach Kamacu
- [Phase 07]: Phase 07 / Plan 04: registerWorkspaceTools owns move_project_to_workspace even though the handler PATCHes /api/projects/{id} (NOT a /api/workspaces route). D-07 per-resource ownership tracks the resource being acted on (transferring a project INTO a workspace), not the route being called. The InputSchema exposes {project_id, workspace_id}; the body contains ONLY workspace_id (D-03 excludes workspace_id from update_project specifically because this tool owns the transfer). — Cross-route resource ownership — the D-07 split tracks the resource being acted on. move_project_to_workspace is conceptually a workspace-management action even though it touches the project route.
- [Phase 07]: Phase 07 / Plan 04: update_workspace uses *string pointer for the lone Name field (D-03 single-field rename). InputSchema declares name as optional (only workspace_id is required). When name is omitted the body is {} and Kamacu returns 400 "nothing to update" verbatim; when supplied (incl. explicit "") the rename triggers. The bridge performs NO validation — Kamacu's empty-trim/dup/default-renamable gates apply unchanged. — D-03 single-field rename pattern — pointer field distinguishes omitted (nil) from supplied (incl. ""), matching Kamacu's PATCH *string decode.
- [Phase ?]: [Phase 08 / Plan 01]: sessionDetail embeds session.Info so every existing JSON tag flows through unchanged; taskTitle/projectName/agentName ride as additional top-level fields (omitempty for dev sessions) — D-10 JOIN wired into list + get_session
- [Phase ?]: [Phase 08 / Plan 01]: subscribe drains the Attach replay (D-02) when include_history is absent — one <-q read atomically removes the replay; streams application/octet-stream (not text); duration clamped to [1s, 300s] server-side (T-08-03); defer Detach on every return path (SC3)
- [Phase ?]: [Phase 08 / Plan 01]: Type-level read-only contract (D-14) enforced — handlers consume ONLY Info/Snapshot/Attach/Detach/Done; scoped grep gate proves zero references to the PTY-write primitive and zero references to WS FrameData
- [Phase ?]: [Phase 08 / Plan 02]: withRecover(name, h) wraps every session tool handler closure so a panic becomes a returned non-nil error rather than unwinding through the SDK's tools/call dispatch (server.go:753 has no recover — Phase 06 Open Q1 / Pitfall 2 closed). Named returns (result, err) are REQUIRED so the deferred recover can overwrite them. — Top-level helper (not a method) so Plan 03 subscribe_session_output reuses it unchanged
- [Phase ?]: [Phase 08 / Plan 02]: listSessions D-13 orphan filter degrades to passthrough on ANY shape surprise (not a JSON array, orphaned not bool, marshal failure) — never errors. The orphan filter is best-effort shape cleanup; a malformed Kamacu body still reaches the agent verbatim rather than breaking the tool. — Preserves Kamacu's SPA behavior intact while guaranteeing the MCP contract (every listed id is operable)
- [Phase ?]: [Phase 08 / Plan 02]: Type-level read-only contract (D-14) enforced — internal/mcp/sessions.go imports ONLY stdlib + SDK; performs only HTTP GETs; scoped grep gate (grep -c 'internal/session' sessions.go == 0) is green; no PTY-write primitive and no WS FrameData type are in scope. — The bridge speaks HTTP only; the session engine package never appears in the import graph
