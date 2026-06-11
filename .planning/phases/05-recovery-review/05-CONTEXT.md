# Phase 5: Recovery & Review - Context

**Gathered:** 2026-06-11
**Status:** Ready for planning

<domain>
## Phase Boundary

The final v1 phase. Work survives server restarts: startup reconciliation removes ghost "running" state silently, and any task whose Claude session ended — within-run or with the server — offers **Resume session** (`claude --resume <uuid>` with the backend-minted, task-persisted session ID from Phase 4). Plus the read-only **Diff tab** that makes In Review meaningful: everything the task changed vs the merge-base of its base branch. (RCVR-01, RCVR-02, REVW-01)

Out of scope: any diff interaction beyond reading (no staging/commit UI), session-history tables, notifications, merge/PR automation.

</domain>

<decisions>
## Implementation Decisions

### Resume (RCVR-02 + the Phase 4 checkpoint commitment)
- **D-54:** Resume is available in BOTH places: (a) the exited agent banner within a server run — **Resume session** (primary) + **Reset session** (ghost/secondary); (b) after a server restart, the Agent tab's pre-start layout with headline `Agent session ended.`, body explaining a previous conversation can be resumed, and the same button pair. One consistent pair everywhere a resumable session exists.
- **D-55:** Resume always runs `claude --resume <uuid>` in the task's worktree using the persisted `tasks.claude_session_id`. The UUID workflow is locked end-to-end: Kangent mints the UUID at spawn (`claude --session-id <uuid>`), stores it in SQLite next to the task, never parses claude output. Each fresh spawn/Reset overwrites with a new UUID (newest conversation wins); Resume targets the stored one.
- **D-56:** Resume failure (transcript gone, claude errors) is honest: the terminal shows claude's error output; the banner returns with Reset session. No silent fallback to a fresh session.
- **D-57:** After a restart, resumable tasks show the normal muted-gray exited dot — no new dot states; the Resume affordance lives in the task view.
- **D-58:** Bash sessions after a restart vanish quietly — tabs mirror live sessions (D-28) and none are live. No exited stubs.

### Diff Tab (REVW-01)
- **D-59:** Scope: everything the task changed vs the **merge-base** with the base branch (three-dot semantics) — commits on the task branch PLUS uncommitted changes (staged, unstaged) PLUS untracked files rendered as full additions. "What did this task change" in one view, even if the base branch moved on.
- **D-60:** Presentation: totals bar at top ("N files changed, +A −B vs <base>") with a refresh button; collapsible per-file sections with unified diffs, add/remove line coloring, per-file stats (+x −y) in headers.
- **D-61:** Refresh on tab open + manual refresh button. No auto-refresh polling.
- **D-62:** Large-diff handling: files over ~400 changed lines render collapsed (stats only, click to expand); binary files always stat-only ("Binary file changed").
- **D-63:** Empty state: "No changes yet. The worktree matches <base branch>." (quiet, consistent with existing empty states).
- **D-64:** Tab placement: **Agent, Description, Diff, Bash 1..N**. On tasks without a worktree the Diff tab is present but disabled with an explanation (same pattern as the bash `+`).

### Post-Restart / Reconciliation (RCVR-01)
- **D-65:** Reconciliation is silent: on startup the server reconciles task agent-state recorded in the DB (no ghost "running"), no banners or notices. The UI simply reflects reality (gray dots, Resume offered).
- **D-66:** Persistence stays task-level: the task's last agent session id + whether the agent ended cleanly (whatever minimal columns recovery needs). NO session-history table; bash sessions remain fully ephemeral.
- **D-67:** Never resume one session into two PTYs (roadmap criterion): one agent per task is already enforced server-side; resume goes through the same one-per-task 409 gate.

### Claude's Discretion
- Mechanics of detecting "agent was running at shutdown" vs "ended cleanly" (e.g. a status column updated on exit vs reconciled at startup) — keep it minimal per D-66
- Diff implementation: git plumbing choice (`git diff <merge-base>` + status for untracked), parsing into structured JSON vs raw patch text rendering, syntax coloring depth (add/remove only is fine)
- Whether the diff renders client-side from a unified patch or the server pre-structures per-file hunks
- Exact body copy for the resumable pre-start state (follow UI-SPEC patterns)
- The `/terminal` dev route's fate at v1 close (keep — costs nothing — unless it interferes)
- Carried UAT item: verify whether plan-mode exit-plan approval triggers the amber waiting dot (research OQ1) during this phase's human verification

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project planning
- `.planning/PROJECT.md` — core value; recovery via --resume is a founding context note
- `.planning/REQUIREMENTS.md` — Phase 5 owns RCVR-01, RCVR-02, REVW-01
- `.planning/ROADMAP.md` — Phase 5 success criteria (3)

### Prior phase contracts (this phase composes them)
- `.planning/phases/04-claude-code-agent-sessions/04-CONTEXT.md` — D-41 revision (Reset session, code-free banner, dimmed exited terminal) + the Deferred Ideas note this phase implements (Resume primary / Reset secondary)
- `.planning/phases/04-claude-code-agent-sessions/04-RESEARCH.md` — verified `--session-id`/`--resume` semantics on claude v2.1.170, hook overlay, status machine
- `.planning/phases/04-claude-code-agent-sessions/04-UI-SPEC.md` — agent tab states incl. checkpoint revisions
- `.planning/phases/03-worktree-isolation-bash-tabs/03-RESEARCH.md` — verified git behaviors (ResolveBase chain reusable for the diff base)
- `.planning/research/PITFALLS.md` — restart reconciliation must ship with session spawn (already partially true); resume directory-scoping notes

### Existing code (integration points)
- `internal/api/sessions.go` — agent spawn path; resume is a variant (`--resume <id>` instead of `--session-id <id>`)
- `internal/session/agent.go` — claude arg construction; `internal/session/manager.go`
- `internal/store/migrations/` — migration 00004 if any new column is needed (agent end-state)
- `internal/worktree/` — ResolveBase + repo paths for the diff service; new diff functions live near here or in a new `internal/diff`
- `web/src/components/task/AgentTab.tsx` — gains Resume/Reset pair and the resumable pre-start state
- `web/src/components/task/TaskTabs.tsx` + `web/src/pages/TaskPage.tsx` — Diff tab insertion (Agent, Description, Diff, Bash...)
- `.planning/phases/01-foundation-projects-board/01-UI-SPEC.md` — diff colors must fit the palette (green/red add/remove lines are content-semantic like ANSI, scoped to the diff surface)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `tasks.claude_session_id` already persisted per spawn (Phase 4, migration 00003) — Resume's key exists
- One-agent-per-task 409 gate already enforced server-side (D-67 rides it)
- `worktree.Service.ResolveBase` already resolves the base branch (diff base discovery)
- AgentTab already has pre-start/running/exited states — Resume adds a button + one new pre-start variant
- UI patterns: collapsible sections, mono text, empty states, totals — all established

### Established Patterns
- goose migrations, table-driven httptest, fake-claude stub for agent tests (no API quota), TanStack hooks, UI-SPEC inheritance chain (a 05-UI-SPEC for the diff surface + resume states is warranted)

### Integration Points
- Startup reconciliation hooks into `cmd/kangent/main.go` after `store.Migrate`
- Diff REST endpoint (e.g. `GET /api/tasks/{id}/diff`) following the worktree endpoint patterns
- Resume REST: spawn endpoint gains a `resume: true` variant or dedicated endpoint; same session manager path with different claude args

</code_context>

<specifics>
## Specific Ideas

- The UUID-first session workflow (mint → store → spawn → resume) was re-affirmed by the user in this discussion as the simplifying principle: never parse claude output for IDs
- Diff tab is read-only by design — reviewing happens in Kangent, acting on it happens in the terminal (manual git, locked since project init)

</specifics>

<deferred>
## Deferred Ideas

- Sessions audit table (no v1 consumer)
- Restart notice banner (silent reconciliation chosen; revisit only if silence proves confusing)
- Diff comments / feed-line-to-agent interactions (v2 territory)

</deferred>

---

*Phase: 05-recovery-review*
*Context gathered: 2026-06-11*
