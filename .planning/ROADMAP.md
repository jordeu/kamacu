# Roadmap: Kamacu

## Milestones

- ✅ **v1.0 MVP** — Phases 1–5 (shipped 2026-06-11) — see [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md)
- ✅ **v1.1 Settings & Polish** — Phase 6 (shipped 2026-06-11) — see [milestones/v1.1-ROADMAP.md](milestones/v1.1-ROADMAP.md)
- ✅ **v1.2 Quota & Resumable Shells** — Phases 7–9 (shipped 2026-06-13) — see [milestones/v1.2-ROADMAP.md](milestones/v1.2-ROADMAP.md)
- ✅ **v1.3 GitHub PR Review** — Phases 10–13 (shipped 2026-06-14) — see [milestones/v1.3-ROADMAP.md](milestones/v1.3-ROADMAP.md)
- ✅ **v1.4 Repo-First Projects** — Phases 14–15 (shipped 2026-06-15) — see [milestones/v1.4-ROADMAP.md](milestones/v1.4-ROADMAP.md)
- ✅ **v1.5 Sharper Review Column** — Phase 16 (shipped 2026-06-17) — see [milestones/v1.5-ROADMAP.md](milestones/v1.5-ROADMAP.md)
- ✅ **v1.6 Global Active Sessions Bar** — Phase 17 (shipped 2026-06-18) — see [milestones/v1.6-ROADMAP.md](milestones/v1.6-ROADMAP.md)
- ✅ **v1.7 Project Icons in Collapsed Sidebar** — Phases 18–19 (shipped 2026-06-19) — see [milestones/v1.7-ROADMAP.md](milestones/v1.7-ROADMAP.md)
- ✅ **v1.8 Kamacu Rebrand & UX Polish** — Phases 20–24 (shipped 2026-07-04) — see [milestones/v1.8-ROADMAP.md](milestones/v1.8-ROADMAP.md)
- ✅ **v1.9 Workspaces** — Phases 25–26 (shipped 2026-07-06) — see [milestones/v1.9-ROADMAP.md](milestones/v1.9-ROADMAP.md)
- ✅ **v1.10 Configurable Agents** — Phases 01–05 (shipped 2026-07-11) — see [milestones/v1.10-ROADMAP.md](milestones/v1.10-ROADMAP.md)
- 🚧 **v1.11 Kamacu MCP Server** — Phases 06–09 (started 2026-07-21) — see Phase Details below

## Phases

<details>
<summary>✅ v1.10 Configurable Agents (Phases 01–05) — SHIPPED 2026-07-11</summary>

- [x] Phase 01: Agent data foundation and spawn engine (6/6 plans) — completed 2026-07-06
- [x] Phase 02: Agent management UI and per-project selection (5/5 plans) — completed 2026-07-06
- [x] Phase 03: opencode engine spawn and activity-based status (4/4 plans) — completed 2026-07-07
- [x] Phase 04: Gated status plugin unlocks waiting and idle (2/2 plans) — completed 2026-07-07
- [x] Phase 05: Session resume and argv regression hardening (3/3 plans) — completed 2026-07-08

Full details: [milestones/v1.10-ROADMAP.md](milestones/v1.10-ROADMAP.md)

</details>

<details>
<summary>✅ v1.9 Workspaces (Phases 25–26) — SHIPPED 2026-07-06</summary>

- [x] Phase 25: Workspace data foundation (3/3 plans) — completed 2026-07-05
- [x] Phase 26: Workspace switcher, management & assignment (6/6 plans) — completed 2026-07-06

Full details: [milestones/v1.9-ROADMAP.md](milestones/v1.9-ROADMAP.md)

</details>

<details>
<summary>✅ v1.8 Kamacu Rebrand & UX Polish (Phases 20–24) — SHIPPED 2026-07-04</summary>

- [x] Phase 20: Kamacu rebrand & brand (4/4 plans) — completed 2026-07-01
- [x] Phase 21: Data directory migration (8/8 plans) — completed 2026-07-01
- [x] Phase 22: GitHub-style diff review (5/5 plans) — completed 2026-07-02
- [x] Phase 23: Worktree cleanup panel (5/5 plans) — completed 2026-07-02
- [x] Phase 24: Session-bar, board & tab polish (4/4 plans) — completed 2026-07-04

Full details: [milestones/v1.8-ROADMAP.md](milestones/v1.8-ROADMAP.md)

</details>

<details>
<summary>✅ v1.0–v1.7 (Phases 1–19) — SHIPPED 2026-06-11 through 2026-06-19</summary>

- [x] v1.0 MVP — Phases 1–5 (28 plans) — [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md)
- [x] v1.1 Settings & Polish — Phase 6 (4 plans) — [milestones/v1.1-ROADMAP.md](milestones/v1.1-ROADMAP.md)
- [x] v1.2 Quota & Resumable Shells — Phases 7–9 (12 plans) — [milestones/v1.2-ROADMAP.md](milestones/v1.2-ROADMAP.md)
- [x] v1.3 GitHub PR Review — Phases 10–13 (19 plans) — [milestones/v1.3-ROADMAP.md](milestones/v1.3-ROADMAP.md)
- [x] v1.4 Repo-First Projects — Phases 14–15 (7 plans) — [milestones/v1.4-ROADMAP.md](milestones/v1.4-ROADMAP.md)
- [x] v1.5 Sharper Review Column — Phase 16 (2 plans) — [milestones/v1.5-ROADMAP.md](milestones/v1.5-ROADMAP.md)
- [x] v1.6 Global Active Sessions Bar — Phase 17 (2 plans) — [milestones/v1.6-ROADMAP.md](milestones/v1.6-ROADMAP.md)
- [x] v1.7 Project Icons in Collapsed Sidebar — Phases 18–19 (6 plans) — [milestones/v1.7-ROADMAP.md](milestones/v1.7-ROADMAP.md)

</details>

---

### 🚧 v1.11 Kamacu MCP Server (In Progress)

**Milestone goal:** Expose Kamacu's task / session / project / workspace / review surface as MCP tools so an agent running inside a Kamacu task PTY can drive the entire app as the user's delegate. Read + subscribe only on terminals — no PTY keystroke injection (write stays browser-only).

**Architecture summary (additive):** One new `kamacu mcp serve` stdio subcommand spawned by the agent CLI, bridging tool calls to the running Kamacu HTTP API at `127.0.0.1:7333`. Auth rides on the existing `KAMACU_HOOK_TOKEN` envelope (already injected into spawned agents in v1.10). No DB schema changes, no migrations, no frontend changes, no new long-lived goroutines inside the Kamacu binary. Backend-only milestone.

**Phase Numbering:** continues from v1.10's last phase (05). v1.11 = Phases 06–09.

- [x] **Phase 06: MCP Subcommand Foundation** — `kamacu mcp serve` stdio subcommand bridging to the Kamacu HTTP API, proven end-to-end through one exercised tool (completed 2026-07-21)
- [ ] **Phase 07: Tasks, Projects & Workspaces Tools** — full CRUD tool surface (13 tools) mirroring every action the SPA exposes
- [ ] **Phase 08: Sessions & Terminal Read Access** — list/get sessions + snapshot + bounded live-tail (the milestone's risk center)
- [ ] **Phase 09: PR Review Tools** — list pending/recently-reviewed + open_review, a thin layer over the v1.3 gh integration

## Phase Details

### Phase 06: MCP Subcommand Foundation

**Goal**: A working `kamacu mcp serve` stdio subcommand that speaks the MCP protocol over stdio and bridges to the running Kamacu HTTP API — proving the architecture end-to-end via one exercised tool so every subsequent phase can repeat the bridge pattern mechanically.
**Depends on**: Nothing (first phase of v1.11; builds on the shipped v1.10 spawn-engine env injection of `KAMACU_SESSION_ID` / `KAMACU_HOOK_TOKEN` / `KAMACU_HOOK_BASE`).
**Requirements**: MCPPROC-01, MCPPROC-02, MCPPROC-03
**Success Criteria** (what must be TRUE):

  1. Running `kamacu mcp serve` from a shell starts a long-lived MCP server (using `github.com/modelcontextprotocol/go-sdk` v1.6.1) that speaks JSON-RPC over stdio and connects to the Kamacu HTTP API.
  2. An MCP client connecting to the subcommand's stdio can call `tools/list` and receive a valid JSON-RPC response over stdout — stdout contains ONLY valid MCP messages (a malformed-tool regression test confirms a stray print becomes a clean JSON-RPC error on stdout, never a stream desync).
  3. A tool call routed through the bridge returns real Kamacu data using `KAMACU_HOOK_TOKEN` for auth — proving the bridge end-to-end via ONE read-only tool (e.g. `list_tasks` or `list_projects`). The chosen tool's production completion is credited in Phase 07; here it is the architecture proof.
  4. When `KAMACU_HOOK_BASE` is set, the subcommand connects to that URL; when unset, it defaults to `127.0.0.1:7333` and works without further configuration.

**Plans**: 3/3 plans complete
Plans:
**Wave 1**

- [x] 06-01-cli-dispatcher-refactor-PLAN.md — Adopt google/subcommands; refactor cmd/kamacu/main.go into a thin dispatcher, move today's main body into serveCmd, break clean on `kamacu serve` (D-01..D-05). Wave 1.
- [x] 06-03-token-header-rename-PLAN.md — Coordinated X-Kangent-Token → X-Kamacu-Token rename across 7 files / 11 occurrences; no fallback (D-07). Wave 1 (parallel with 06-01).

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 06-02-mcp-serve-subcommand-PLAN.md — Build internal/mcp/ package (server.go + bridge.go), wire kamacu mcp serve via Pattern 2 nested Commander, register list_projects tool, ship SC2 malformed-tool unit test (D-08..D-12). Delivers SC1-SC4 + MCPPROC-01/02/03. Wave 2 (depends on 06-01).

### Phase 07: Tasks, Projects & Workspaces Tools

**Goal**: An agent inside a Kamacu task PTY can fully drive Kamacu's task, project, and workspace surface — every CRUD action the browser-attached user can do via the SPA, the agent can do via a tool. The mechanical bulk of the milestone: 13 thin HTTP-bridge tools repeating Phase 06's pattern over existing endpoints.
**Depends on**: Phase 06 (the subcommand + bridge + scope plumbing).
**Requirements**: MCPTASK-01, MCPTASK-02, MCPTASK-03, MCPTASK-04, MCPTASK-05, MCPTASK-06, MCPPROJ-01, MCPPROJ-02, MCPPROJ-03, MCPPROJ-04, MCPPROJ-05, MCPPROJ-06, MCPPROJ-07
**Success Criteria** (what must be TRUE):

  1. An agent can call the six task tools (`list_tasks` / `get_task` / `create_task` / `update_task` / `move_task` / `delete_task`) and the corresponding change appears on the board the browser-attached user sees — including the worktree+branch auto-provisioning on create and the gated cleanup on delete (no agent auto-start; auto-start remains Out of Scope per PROJECT.md).
  2. An agent can call the five project tools (`list_projects` / `get_project` / `create_project` / `update_project` / `delete_project`) and the project sidebar reflects the change — including the v1.4 managed-checkout atomic create + all-or-nothing gated delete (folder-project delete byte-for-byte unchanged).
  3. An agent can call the four workspace tools (`list_workspaces` / `create_workspace` / `update_workspace` / `delete_workspace`) and `move_project_to_workspace`, and the workspace switcher reflects the change — including the v1.9 guarded delete keyed off `is_default` + non-empty `COUNT`.
  4. Tool calls with invalid input (bad project_id, illegal workspace name, missing required fields) surface actionable MCP errors and produce no Kamacu state change — every existing validation gate applies unchanged through the bridge.

**Plans**: 1/4 plans executed

- [x] 07-01-endpoints-and-mcp-scaffold-PLAN.md
- [ ] 07-02-tasks-tools-PLAN.md
- [ ] 07-03-projects-tools-PLAN.md
- [ ] 07-04-workspaces-tools-PLAN.md

### Phase 08: Sessions & Terminal Read Access

**Goal**: An agent can enumerate sessions across the board and read terminal output — snapshots immediately, live tails bounded — without ever injecting bytes into a PTY. The milestone's only genuinely new capability (everything else is translation) and its risk center.
**Depends on**: Phase 06 (subcommand + bridge); two new read-only HTTP endpoints on the Kamacu binary exposing the existing Session ring buffer.
**Requirements**: MCPSESS-01, MCPSESS-02, MCPSESS-03, MCPSESS-04
**Success Criteria** (what must be TRUE):

  1. An agent can call `list_sessions(project_id? or task_id?)` and `get_session(session_id)` and receive current session state — status (working / waiting / idle / exited / running), task, project, agent, started_at — identical to what the SPA's Active Sessions bar shows.
  2. An agent can call `get_session_output(session_id, bytes?)` and receive a snapshot of the session's PTY ring buffer (last N bytes, default 4 KB) as a read-only result — the snapshot is taken from the existing v1.0 Phase 2 ring buffer with no new long-lived state.
  3. An agent can call `subscribe_session_output(session_id, duration_seconds?)` to tail live PTY output for a bounded duration (default 30s, hard cap 300s); on cancellation (the agent CLI sends `notifications/cancelled`) the tool returns the partial stream collected so far and detaches from the session within 100ms; goroutine count is stable across N subscribe/cancel cycles (no leak).
  4. No tool in this phase exposes any capability to send keystrokes / PTY input — the read-only contract is enforced at the type level (a wrapper with no public Write method), and no `FrameData` PTY-input frame is ever sent.

**Plans**: TBD

### Phase 09: PR Review Tools

**Goal**: An agent can triage the user's PR review load and open review workspaces by reusing Kamacu's existing v1.3 / v1.5 `internal/github` surface — no new gh calls, no bypassing of the existing gates. Last because it depends on the v1.3 gh integration being healthy.
**Depends on**: Phase 06 (subcommand + bridge); the v1.3 / v1.5 gh integration surfaced through `GET /api/projects/{id}/pull-requests` and `POST .../pull-requests/{n}/review`.
**Requirements**: MCPREV-01, MCPREV-02, MCPREV-03
**Success Criteria** (what must be TRUE):

  1. An agent can call `list_pending_reviews(project_id?)` and `list_recently_reviewed(project_id?)` and receive the same PR lists the browser Review column shows — `user-review-requested:@me draft:false` and `reviewed-by:@me draft:false` respectively, riding the existing per-repo TTL cache (no N+1 `gh` calls).
  2. An agent can call `open_review(project_id, pr_number)` to open a PR as a review workspace — reusing the existing v1.3 Phase 12 find-or-create `source='github_pr'` task path with worktree on the PR's real head branch — and the review opens identically to clicking the PR card in the browser (open-or-reattach, no duplicates, board-leak guard intact).
  3. When GitHub integration is off, the project has no linked repo, or `gh` is absent, the review tools degrade gracefully — returning an actionable MCP error and never bypassing the v1.3 two-gate ladder (toggle → link).

**Plans**: TBD

## Progress

**Execution Order:** Phases execute in numeric order: 06 → 07 → 08 → 09.

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 01. Agent data foundation and spawn engine | v1.10 | 6/6 | Complete | 2026-07-06 |
| 02. Agent management UI and per-project selection | v1.10 | 5/5 | Complete | 2026-07-06 |
| 03. opencode engine spawn and activity-based status | v1.10 | 4/4 | Complete | 2026-07-07 |
| 04. Gated status plugin unlocks waiting and idle | v1.10 | 2/2 | Complete | 2026-07-07 |
| 05. Session resume and argv regression hardening | v1.10 | 3/3 | Complete | 2026-07-08 |
| 06. MCP Subcommand Foundation | v1.11 | 3/3 | Complete    | 2026-07-21 |
| 07. Tasks, Projects & Workspaces Tools | v1.11 | 1/4 | In Progress|  |
| 08. Sessions & Terminal Read Access | v1.11 | 0/TBD | Not started | — |
| 09. PR Review Tools | v1.11 | 0/TBD | Not started | — |
