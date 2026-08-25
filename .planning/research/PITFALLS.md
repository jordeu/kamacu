# Pitfalls Research

**Domain:** Adding a global/ad-hoc session surface (v1.13 Global Task) to Kamacu — an existing local-only Go+React app whose entire data model is task-scoped, project-scoped, and worktree-isolated
**Researched:** 2026-08-25
**Confidence:** HIGH — every pitfall mechanic below was verified directly against the v1.12 codebase in this worktree (file:line references), not inferred from generic advice. Two findings are additionally cross-validated against community sources (tagged MEDIUM in Sources).

The milestone brief names the exact risk: sessions keyed by `task_id`, agent-status JOINs over `tasks+projects`, board queries filtering `source='manual'`, a Done-TTL reaper keyed on task status, MCP tools filtering by project/task, cleanup gates assuming worktrees. This document walks each of those seams, in the order you'd hit them, with the concrete code that breaks.

---

## Critical Pitfalls

### Pitfall 1: The invisible global session — every status and reconciliation pass filters `TaskID <= 0` and INNER-JOINS tasks→projects→agents

**What goes wrong:**
The global agent session spawns fine, the terminal works, but it never appears in the Active Sessions bar, never reports working/waiting/idle, and after a server restart offers no Resume. The feature looks broken end-to-end even though "the session runs."

**Why it happens:**
Three independent filters, any one of which alone hides the global session — all three fire together:

1. `internal/api/agents.go:50` — the manager-derived pass skips any agent session with `info.TaskID <= 0` before it ever reaches the entry list.
2. `internal/api/agents.go:99-103` — the metadata query is `FROM tasks JOIN projects ON … JOIN agents ON …` (INNER JOINs). A task-less session has no `tasks` row, so `metas[tid]` is missing and the loop's `continue` at agents.go:127-129 ("task deleted under a still-tracked session") silently drops it. This is the classic nullable-FK-meets-INNER-JOIN failure: rows don't error, they vanish (web-validated, see Sources).
3. `internal/api/agents.go:164-168` — the DB-derived post-restart pass requires `tasks.claude_session_id IS NOT NULL … AND tasks.worktree_path IS NOT NULL`. The global session has neither a tasks row nor a worktree, so post-restart resume entries can never surface. The in-pass `Resumable` derivation (agents.go:135-137) additionally requires `m.wtp.Valid` — resume is worktree-gated *by construction*.

The same triple filter exists in the sessions REST layer: `joinSessionContext` (internal/api/sessions.go:917-923) skips `info.TaskID <= 0`, so `GET /api/sessions` entries for the global surface carry empty `taskTitle`/`projectName`/`agentName`.

**How to avoid:**
Treat `/api/agents/status` as the milestone's risk center, exactly like v1.6 widened it and v1.5 widened it before that. The global entry must be emitted deliberately in BOTH passes: the manager pass needs a parallel branch for global-scoped agent sessions (synthesized `taskTitle`/`projectName` — see Pitfall 6 for the wire shape), and restart-resume needs a persisted global session id read from wherever Pitfall 7 puts it, with the resumable gate keyed off the *configured global root* (which plays the role the worktree plays for tasks: "the directory the agent runs in still exists"). Add a regression test that spawns a global agent against a fake agent binary and asserts an entry appears in `/api/agents/status`, both live and post-restart.

**Warning signs:**
- Global agent runs but the bar says "No active sessions."
- `/api/agents/status` returns entries whose `taskTitle`/`projectName` are empty strings (you found the JOIN gap but papered over it frontend-side).
- After `kamacu` restart, the global view shows no Resume affordance while task agents do.

**Phase to address:**
Phase 2 (session surface & status integration) — the status-feed widening must land in the same phase as the spawn path, or the phase cannot be human-verified.

---

### Pitfall 2: The sentinel-row hack — a fake project/task row leaks onto boards, listings, stats, and MCP tools

**What goes wrong:**
The team shortcuts "not linked to any workspace or project" by inserting a hidden sentinel project (or a tasks row with a magic project) and filtering it out of the board. Every month, another surface forgets the filter: the sentinel appears in the sidebar, in `GET /api/projects`, in a workspace's project count, in the Activity page's project scope selector, in the worktree-cleanup panel's per-project grouping, in `list_projects` for MCP agents — or the sentinel *task* shows up on every board via `GET /api/tasks` (which the v1.11 bridge consumes), in drag-position arithmetic (`MIN(position)-1` queries at tasks.go:288/432/538/554 all assume `source='manual'` rows belong to a project), or in v1.12 Activity stats.

**Why it happens:**
The schema makes the honest modeling painful, so the dishonest one looks cheap: `tasks.project_id INTEGER NOT NULL REFERENCES projects(id)` (migration 00001) and `tmux_sessions.task_id INTEGER NOT NULL REFERENCES tasks(id)` (migration 00005). A task-less row literally cannot exist without a migration. Meanwhile the codebase already has one successful sentinel-adjacent pattern — `source='github_pr'` rows are kept off boards by filtering `source = 'manual'` on all five board/position queries (tasks.go:194/233/288/432/538/554) plus a `/move` 409 — so "add `source='global'` and filter" *feels* familiar. But the PR-review precedent works because those rows still have a real project; a project-less row has no such anchor, and the number of places that enumerate projects/tasks unscoped keeps growing every milestone (v1.9 workspace transfer, v1.11 MCP `GET /api/tasks` + `list_projects`, v1.12 Activity scopes, the cleanup panel's cross-project union).

**How to avoid:**
- Do NOT create a sentinel project. `projects` rows are now load-bearing in too many enumerations (sidebar, workspace filters, `?workspace_id` scoping, Activity, cleanup panel, MCP).
- Prefer a dedicated singleton representation for the global task (its own single-row table or a settings-anchored config with the session ids), and keep `tasks` untouched. If a tasks row is nonetheless chosen (`source='global'`), then: (a) make `project_id` nullable via migration and grep-audit **every** `FROM tasks` and `FROM projects` query in `internal/` for INNER JOINs and unscoped enumerations (there are ~30; the audit is the phase's real cost), and (b) add a "sentinel leak" regression test asserting the global row appears in NONE of: board lists (both project-scoped and unscoped `GET /api/tasks`), Activity `tasks-done`, `GET /api/projects`, and the cleanup panel.
- Decide explicitly where the global session *does* appear: the Active Sessions bar (yes, it's live work), `GET /api/sessions` unfiltered (yes — MCP `list_sessions` with no filter lists all live sessions, and that is arguably correct), `GET /api/tasks` (no).

**Warning signs:**
- A migration adds a project row the user never created.
- Code review contains "and we filter it out here / here / here" more than twice.
- Any query in the diff adds `WHERE id != <magic>` or `WHERE title != 'Global'`.

**Phase to address:**
Phase 1 (data foundation) — the representation choice is the first thing the milestone builds; getting it wrong here rewrites every later phase.

---

### Pitfall 3: No worktree safety net — a skip-permissions agent mutating the user's real checkout, with no diff and no gates

**What goes wrong:**
Every task-level safety invariant Kamacu has built since v1.0 is worktree-shaped: isolated `task/<slug>-<id>` branch, diff vs merge-base (Phase 5/v1.8), dirty/unpushed/stash/session cleanup gates (v1.3/v1.4), worktree auto-provisioning at task create. The global scratchpad runs the agent directly in the configured root with NONE of that. If the user points the global root at their real working checkout of a daily-driver repo, a hallucinating agent can delete uncommitted work, switch/reset branches, or run destructive git commands — and per the milestone spec the global view has **no Diff tab**, so the user cannot even review what changed. There is no branch kept, no merge-base to diff against, and no gated cleanup semantics for a non-worktree directory.

**Why it happens:**
The default agent is the Claude seed whose `extra_params` code default is `--dangerously-skip-permissions` (internal/api/agents_backfill.go:114, AGENT-02) — the intended daily-driver posture inside a disposable worktree, but the worst possible default inside the user's only copy of a repo. The scratchpad's "no worktree, no branch" requirement is exactly what removes every safety net the app otherwise assumes. Community practice confirms the shape of the risk: multiple practitioner sources document agents silently overwriting work and deleting crucial files when pointed at a shared checkout, with worktrees as the standard isolation boundary (web-validated, Sources).

**How to avoid:**
- Make the **managed clone** (v1.4 pattern) the recommended default for the global root — a Kamacu-owned `gh repo clone` gives a disposable directory with origin to push against, making "no worktree" safe-ish by construction. Treat folder mode as the power-user escape hatch.
- In Settings, when a folder path is configured: validate it is a git repo (reuse the `reattachManaged`-style `git rev-parse` check), and surface a persistent, honest warning ("Sessions run directly in this folder — changes are not isolated") rather than silently accepting it.
- In the global view UI, since there is no Diff tab, consider surfacing a lightweight `git status --short` summary line so the user can see the scratchpad accumulating changes (cheap, no new machinery).
- Ship the milestone README/UAT script with an explicit statement of the threat model change: the global root is trusted-by-default workspace, unlike a task worktree.

**Warning signs:**
- The Settings save path accepts any existing directory with no git check and no warning copy.
- UAT only exercises a managed-clone global root, never a folder pointed at a real repo.
- Anyone proposes "reuse the Diff tab" late in the milestone — that way scope creep lies (the spec says no diff; the `git status` summary is the honest substitute).

**Phase to address:**
Phase 1 (root configuration: validation + warning copy) and Phase 4 (human-verify against a real folder checkout).

---

### Pitfall 4: `TaskID = 0` is already taken — reusing the "unscoped dev session" overload makes stop/list/label machinery into landmines

**What goes wrong:**
The quickest wiring for "no task" is `SpawnOpts{TaskID: 0}` — which already means "unscoped `/terminal` dev session" (internal/session/manager.go:56). The global surface then shares an ID-space with dev sessions: `Manager.StopAllForTask(0)` would kill every open dev terminal; `ListByTask(0)` returns dev sessions mixed with global bash tabs; the label counters collide (`bash #N` global counter vs per-task `Bash N` map, manager.go:338-344); and any future "stop all global sessions" affordance nukes unrelated terminals.

**Why it happens:**
Zero looks like a natural sentinel and the manager already tolerates it. But the value is *overloaded*, not *free*: three separate subsystems (labels, listing, stopping) branch on it with dev-session semantics. The same class of bug as the reaper's task-keyed queries — any "for task X" operation with X=0 fans out to everything unscoped.

**How to avoid:**
Introduce an explicit scope discriminator rather than reusing 0: either a dedicated sentinel that cannot collide with dev sessions (e.g. a `Scope string` on `SpawnOpts`/`Info` — `"task"`/`"global"`/`"dev"`), or a reserved pseudo-task-id constant used ONLY by the global handlers and never by `/terminal`. `StopAllForTask`-equivalent global stop should be a new targeted method (`StopAllForScope`), not a call with 0. Keep `TaskID <= 0` filters meaning "dev only" untouched so the v1.11 read-only endpoints' dev-session behavior stays byte-for-byte.

**Warning signs:**
- A diff passes literal `0` as a task id to `StopAllForTask`/`ListByTask` outside the dev route.
- The global bash tabs' labels render as `bash #N` instead of `Bash N`.

**Phase to address:**
Phase 2 (session surface) — the discriminator must exist before the first global spawn is wired.

---

### Pitfall 5: Restart amnesia — the tmux FK, the startup orphan sweep, and the worktree-gated resume path all erase the global session on restart

**What goes wrong:**
Everything works in a fresh process, then the user restarts `kamacu` and: (a) global bash tabs are **killed** at startup — `sweepOrphanTmux` (cmd/kamacu/serve.go:343-401) kills any live `kamacu-*` tmux session on the dedicated socket with no DB row, and global tmux tabs cannot have rows because `tmux_sessions.task_id INTEGER NOT NULL REFERENCES tasks(id)` (migration 00005); (b) the global agent's resume id has nowhere persisted to live (task sessions keep theirs in `tasks.claude_session_id`/`opencode_session_id`; the global has no row), so no Resume after restart — violating the milestone's "restart reconciliation/resume" requirement outright; (c) `GET /api/sessions?task_id=…`-driven reattach has no task_id to query for the global scope.

**Why it happens:**
The restart-durability machinery (v1.2 Phase 9, hardened in v1.8 Phase 24) is keyed end-to-end on `tmux_sessions.task_id` → `tasks.id`, and the resume machinery on `tasks.*_session_id`. The milestone spec says "same session semantics as tasks" without naming that ALL of those semantics are table-backed. This pitfall is sneaky because nothing fails in development until the process restarts — exactly the class of bug UAT catches only if the script remembers to restart mid-test.

**How to avoid:**
- Migration: make global tmux persistence possible — either a nullable `task_id` plus a scope/label column, or (if the global got its own singleton row per Pitfall 2's preferred design) a FK to that row. The tmux **name** needs its own mint scheme (`kamacu-global-<n>`), and note `defaultTmuxLabel` (sessions.go:615-622) parses the trailing `-<n>` — keep the scheme last-segment-numeric so label re-derivation keeps working.
- Beware the tmux prefix-matching gotcha documented at internal/tmux/tmux.go:78 (`has-session -t kamacu-1` matches `kamacu-1-10`): the global name scheme must not create a prefix of task names or vice versa — `kamacu-global-1` is safe; `kamacu-g-1` next to a hypothetical task id `g` would not be, and more importantly any exact-name probes must stay exact.
- Persist the global agent's session ids wherever the global config lives, and extend BOTH restart consumers: the orphan sweep's known-name set (so global tmux survivors are recognized, not killed) and the agents.go DB-derived pass (Pitfall 1).
- Regression tests: (1) global tmux tab survives a simulated restart (row exists → sweep skips it, reconcile surfaces an orphaned entry); (2) global agent exit → restart → Resume offered and works with the fake-claude harness.

**Warning signs:**
- The global spawn path mints a tmux name but the INSERT is skipped "because there's no task."
- Resume tests exist only for task agents.
- The sweep's known-names query still reads only `tmux_sessions` keyed by task.

**Phase to address:**
Phase 3 (persistence & restart reconciliation) — with the migration itself in Phase 1 since it's schema.

---

### Pitfall 6: The bar breaks on a project-less entry — non-nullable wire contract, hardcoded task route, `key={taskId}`

**What goes wrong:**
Once the backend does emit a global entry (Pitfall 1 fixed), the frontend chokes in four small ways: `AgentStatusEntry` types `projectId: number; taskTitle: string; projectName: string` as non-optional (web/src/api/agents.ts); the row click navigates to `` `/projects/${entry.projectId}/tasks/${entry.taskId}` `` (ActiveSessionsBar.tsx:133-135) — with zero/negative ids that's a dead route; the React list keys rows by `entry.taskId` (line 129) so a global row keyed `0` can collide with any other unscoped entry; and the row label renders `projectName · taskTitle` (lines 262-266) as an empty or "undefined · undefined" string. The `source` union is `"manual" | "github_pr"` (types + backend `agentStatusEntry.Source`) and PR-badge logic branches on `source === "github_pr"` — a third value needs handling at every switch.

**Why it happens:**
v1.6 built the bar when every agent session had a task and a project by definition; the contract was never nullable because nullity was unrepresentable. This is the UI face of Pitfall 1 — and the two must land together or the feature oscillates between "invisible" and "broken row."

**How to avoid:**
- Widen the wire deliberately: either nullable `projectId`/`taskTitle`/`projectName` + a `source: "global"` (or `scope`) discriminator, or — cleaner for TS consumers — keep the fields non-nullable and synthesize values server-side (`projectName: "Global"`, `taskTitle: <agent name or "Scratchpad">`). The synthesized approach touches zero TS types beyond the `source` union and keeps every existing consumer rendering.
- Give the global view its own route (`/global`, sibling of `/settings` and `/activity` in App.tsx) and branch the bar's click-through on the discriminator. Key rows by `sessionId` (stable, unique, already on the entry) instead of `taskId`.
- The current-row highlight compares `String(entry.taskId) === openTaskId` from `useParams()` — the global route has no taskId param, so add the global-route case explicitly rather than letting it never-highlight.

**Warning signs:**
- `tsc` passes but the bar renders a blank-labeled row that navigates nowhere.
- The `source` union grows without every `source ===` site being grepped (there are branch points in ActiveSessionsBar.tsx:268, TaskPage.tsx:105/278, types.ts:89).

**Phase to address:**
Phase 2 (bar + view UI), same phase as the status-feed widening.

---

### Pitfall 7: Global-root reconfiguration while sessions are live — spawn-time cwd, stale resume ids, and an orphaned old clone

**What goes wrong:**
Sessions capture their cwd at spawn (`exec.Cmd.Dir`, validated at spawn time — manager.go:137-143). Reconfiguring the global root in Settings changes only *future* spawns, producing three divergences: (a) the bar/settings entry describes the new root while a live session still runs in the old one — the user thinks they're talking to repo B, the agent is editing repo A; (b) the persisted agent resume id was minted in the old root — after the session exits and the server restarts, "Resume" reopens a conversation whose entire context is the previous repo, in a directory it never saw; (c) if the old root was a managed clone and the reconfigure flow removes it (or the user deletes it manually), a live PTY keeps running on unlinked inodes — writes "succeed," reads of new files fail, and the failure surfaces as confusing agent behavior rather than an error.

**Why it happens:**
For tasks, the analogous mutation is impossible by construction: the worktree path is per-task and immutable-ish, and every destructive path (worktree removal, task delete, project delete) runs the session gate first (`CleanupWorktreeGated` gate 1, cleanup.go:113-115). The global root is the first *mutable* cwd source in the app, and Settings' existing save semantics are immediate partial-PATCH with no lifecycle awareness. Also note the v1.10 precedent: agent config changes intentionally apply "at next spawn" (read-at-use) — root changes look like they should follow the same benign pattern, but a *directory* is not a flag.

**How to avoid:**
- Block root reconfiguration while any global session (agent or bash, live tmux included) is running: 409 with a reason list, exactly mirroring the gated-delete philosophy (`sessions running` → stop first). This is one small gate and it eliminates all three divergences.
- On a successful reconfigure: clear the persisted global resume ids (a conversation from another directory is not a resumable scratchpad) and decide the old managed clone's fate explicitly (Pitfall 8).
- Settings UI: show the live-session state on the global section (the same data the bar has) so the gate never surprises.

**Warning signs:**
- The Settings PUT handler for the global root has no session check.
- A reconfigure test changes the root while a fake agent is running and the response is 200.

**Phase to address:**
Phase 4 (settings & hardening) for the gate; Phase 1 decides the semantics so the config schema can hold what it must.

---

### Pitfall 8: Managed global clone lifecycle — path collision with a project's managed clone makes project-delete `RemoveAll` the global root

**What goes wrong:**
The global root's GitHub flavor "cloned like a managed project per the v1.4 pattern" naturally reuses `reposBase = "~/.kamacu/repos/"` and `createByRepo`'s dest math (`~/.kamacu/repos/<owner>/<name>`, projects.go:355-391). Two traps: (a) **collision** — if the user already has (or later adds) a *project* for the same `owner/name`, project creation's dedup (`SELECT 1 FROM projects WHERE repo_path = ?`, projects.go:406) keys on the projects table only; the global clone at the identical path is invisible to it, so either the create 409s confusingly ("already added" — no project shows it) or, worse, paths are shared and the **project's gated delete does `os.RemoveAll` on the directory the global sessions live in** (v1.4 all-or-nothing remove); (b) **orphaning** — if the global root is reconfigured away from repo A, nothing in the cleanup panel knows about A's clone (the panel enumerates project worktrees), so it accumulates forever with no gated removal.

**Why it happens:**
v1.4's ownership model is row-anchored: "managed" means *a projects row owns this directory*. The milestone brief says "cloned like a managed project," which copies the clone mechanics but not the ownership anchor — and nothing in the existing delete/cleanup surfaces references a directory that no row points at. The collision case is the severe one because the project-delete gates are thorough about *worktrees* and sessions but have no concept of "another surface's cwd is this clone root."

**How to avoid:**
- Namespace the global clone OUTSIDE the projects namespace: `~/.kamacu/repos/global/<owner>/<name>` or `~/.kamacu/global/repo` — structurally incapable of colliding with any project's `repo_path`, now or ever. Cheap, and it makes the collision class unrepresentable rather than merely tested-for.
- Define the global root's own lifecycle: reconfigure-from-managed → gated removal of the old clone (reuse the four-gate shape — dirty/unpushed/stash/running-session — via a sibling of `CleanupWorktreeGated` adapted for a non-worktree clone root; do NOT call the task-shaped helper with a fake taskID, its null-columns UPDATE branches on `taskID > 0`, cleanup.go:157-166), and surface the global clone in the Settings cleanup panel's candidate list when abandoned.
- The gh-gated, degrade-don't-break creation posture (validate → clone → record) transfers as-is; the reattach-on-existing-dir check (`reattachManaged`) should also transfer so a re-point to an existing dir never clobbers.

**Warning signs:**
- The global clone dest is computed with the same `filepath.Join(reposBase, canonical)` as projects.
- A test adds a project for a repo that is also the global root and both coexist silently.
- Reconfigure leaves the old clone on disk with no Settings affordance mentioning it.

**Phase to address:**
Phase 4 (managed-clone root), with the namespace decision made in Phase 1's config schema (the stored root path format bakes it in).

---

### Pitfall 9: No lifecycle owner — the Done-TTL reaper, stop paths, and cleanup gates are all task-keyed; global sessions live forever

**What goes wrong:**
After the feature ships, a global agent left `waiting` (or running with skip-permissions) never dies: the Done-TTL reaper selects `tasks WHERE status='done' AND done_at < cutoff` (reaper.go:165-167) — the global never has a status or a `done_at`; `StopAllForTask` is the only bulk-stop primitive and is task-keyed (manager.go:456-475); `CleanupWorktreeGated`'s stop-before-remove likewise. The scratchpad becomes the only surface in the app with unbounded session lifetime. Additionally, any "stop the global" button implemented via the wrong key (see Pitfall 4) takes dev sessions with it.

**Why it happens:**
Every lifecycle decision in the codebase routes through the tasks table because that was the only session-owning entity. The milestone spec is silent on global-session TTL, which reads as "don't build one" — but "same session semantics as tasks" cut both ways: tasks get reaped, the global doesn't, and nobody decided.

**How to avoid:**
- Make the non-reaping an explicit, documented decision (a scratchpad you deliberately keep alive is defensible — the user sees it in the bar and can stop it; write it into REQUIREMENTS as out-of-scope so the audit doesn't flag it as a gap).
- Give the global view a Stop affordance that uses a scope-targeted stop (Pitfall 4's `StopAllForScope`), wired like the task ⋯ menu's Stop.
- If a TTL is ever wanted, it keys off the global row's last-activity/exit timestamps — not the tasks machinery.

**Warning signs:**
- The phrase "we'll add a TTL later" without a REQUIREMENTS line saying it's out of scope for v1.13.
- A stop button on the global view whose handler passes a numeric task id.

**Phase to address:**
Phase 2 (stop affordance) + Phase 4 (documented out-of-scope decision in REQUIREMENTS).

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Sentinel project row for the global task (Pitfall 2) | No migration; task machinery reused verbatim | Every project enumeration becomes a filter-audit forever; each new milestone re-risks the leak | Never — the v1.12+ surface count makes it unmaintainable |
| `TaskID = 0` for global sessions (Pitfall 4) | Zero manager changes | Stop/list/label semantics shared with dev sessions; one bad call kills unrelated terminals | Never |
| Skipping global resume-id persistence "for now" | Phase 3 shrinks | Restart drops the scratchpad conversation — silently violates a stated milestone requirement | Never (it's a requirement, not a nicety) |
| Global clone inside `~/.kamacu/repos/<owner>/<name>` (Pitfall 8) | Reuses v1.4 dest math | Path collision class with project deletes; unownable orphans | Never — the namespaced path costs one line |
| Emitting global bar entries with empty `projectName` instead of a discriminator | No wire change | Every consumer renders blanks; TS union grows anyway later | MVP-only, and only if a `scope` field ships in the same milestone |
| Not gating root reconfigure on live sessions (Pitfall 7) | Settings stays a dumb PUT | Split-brain cwd + stale resume ids; support burden ("agent edited the wrong repo") | Acceptable only if reconfigure also force-clears resume ids AND the UI warns — the 409 gate is cheaper |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| `/api/agents/status` (v1.6 bar feed) | Widening only the manager pass, forgetting the DB-derived post-restart pass (agents.go:164+) | Both passes need a global branch; post-restart keys off persisted global ids + root-exists check |
| `GET /api/sessions` + MCP `list_sessions` | Assuming global sessions should be filtered OUT of the unfiltered list (agents legitimately use it to observe sibling terminals) | Keep them listed; ensure `joinSessionContext` fills global rows with synthesized context instead of skipping (sessions.go:921) |
| MCP `start_task_agent` / `send_session_message` | Blocking global sessions from the delegate surface, or special-casing them in the bridge | No bridge changes needed — they target session ids, not tasks; verify a global agent can be driven by MCP the same as a task agent (it inherits `KAMACU_*` env via the same spawn branch) |
| tmux (`-L kamacu` socket) | Global tmux names colliding with task names or the sweep's known-set (tmux.go:78 prefix matching; serve.go:343 sweep) | Own name scheme `kamacu-global-<n>`, last segment numeric for `defaultTmuxLabel`; sweep's known-set query extended |
| Done-TTL reaper | Expecting it to cover global sessions "because they're task-like" | It cannot see them by design (tasks-table-keyed); decide non-reaping explicitly (Pitfall 9) |
| v1.4 managed-clone flow (`createByRepo`) | Copy-pasting it for the global root including the dest path and the projects dedup | Reuse validate→clone→record ordering and `reattachManaged`; change the base dir and skip the projects-table dedup in favor of the global config's own single-slot semantics |
| opencode engine (`captureOpencodeSessionAsync`) | Forgetting the discovery poll writes `tasks.opencode_session_id` — a global opencode spawn has no row to write | Persist to the global config's id slot; the poll's `directory == dir` filter works unchanged since it keys off the spawn cwd |

## Performance Traps

Kamu is single-user localhost — most scale traps are irrelevant. The ones that aren't:

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Global resume-id capture poll outliving its purpose | `opencode session list` subprocess every 2s forever if the global session never exits (the hard cap is 60min, but the scratchpad *never exits* by design — every restart re-polls) | Acceptable: the existing activeWindow bounds churn; just don't spawn a NEW capture per reattach | N/A within v1.13; watch if multiple global sessions ever exist |
| `/api/agents/status` fan-out widening | Each new entry type adds JOIN branches to a 5s-polled endpoint | Keep the global branch a single-row lookup (it's a singleton — no query fan-out at all) | Never, if singleton |
| Unbounded global bash-tab count | Scratchpad accumulates tabs over weeks (no Done-TTL to prune rows) | The existing exited-session GC + `×`-to-delete posture is sufficient; just don't disable `Remove` for global tabs | Only with scope creep to N globals |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Folder-mode global root + default Claude agent (`--dangerously-skip-permissions`) in the user's only checkout (Pitfall 3) | Uncommitted work destroyed; branch state mutated; no recovery via merge-base diff | Managed-clone default; git-repo validation + persistent warning for folder mode; `git status` summary in the view |
| Global agent inherits the full MCP delegate surface (`start_task_agent`, `send_session_message`, PR writes) with the real checkout as cwd | Blast radius of a delegate mistake is the user's real repo instead of a task worktree | Same trust model as tasks is the *intended* design — document it; do not add extra write surface in this milestone |
| Reconfigure flow accepting arbitrary paths (e.g. `~/.ssh`, `/`) as the global root | Agent sessions with inherit-all env running in sensitive directories | Path validation at save: must exist, must be a directory, must be a git repo (warn if not), refuse obvious non-repos rather than warning |
| Skipping the root-reconfigure session gate (Pitfall 7) | Not a classic security issue, but enables "agent edits repo the user believes is inactive" confusion | 409 gate with live-session reasons |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Bar row with blank labels / dead click-through (Pitfall 6) | The flagship entry point ("reachable from the Active Sessions bar") renders broken | Synthesized `Global · Scratchpad` label + `/global` route + discriminator-branched navigation |
| "Global" naming collision with v1.12 Activity's "Global" scope | Users conflate the Activity scope selector with the global task | Consider "Scratchpad" as the UI label for the view while keeping "global task" as the internal concept — decide once, in Phase 1 |
| Reachability asymmetry (bar when live, Settings when idle) forgotten | Users can't find the scratchpad when the agent has exited but bash tabs live on | Define "live" as ANY live global session (agent OR bash/tmux), not agent-only; the bar filter is `agent sessions only`, so idle-exited-agent + live-bash needs its own presence signal — simplest: the Settings entry always opens the view; the bar shows agent-live only (documented) |
| Settings-only idle entry point buried under the gear | The scratchpad feels second-class; users create tasks "for a quick check" anyway (the exact behavior v1.13 exists to stop) | Sidebar entry (sibling of Activity/Settings) is the cheap fix if UAT shows friction — but it's scope creep; note it as a parked follow-up, don't build unprompted |
| No feedback that reconfigure is gated | User edits root, hits Save, gets a 409 they didn't expect | Inline "N sessions running — stop them first" copy in the settings section, mirroring the cleanup dialogs' reason lists |

## "Looks Done But Isn't" Checklist

- [ ] **Bar entry:** visible with a real label, click lands on `/global`, current-row highlight works while the global view is open — verify after BOTH passes of `/api/agents/status` were widened (restart the server with a live-then-exited global agent and re-check)
- [ ] **Restart resume:** global agent exits → `kamacu` restart → Resume offered and reconnects (fake-claude harness); global opencode spawn captures its `ses_…` id to the *global* slot, not nowhere
- [ ] **tmux survival:** global bash tabs survive a server restart (row-backed, sweep-recognized, reattach restores custom labels — the v1.8 GAP-01 regression replayed for the global scope)
- [ ] **Sentinel leak sweep:** with the feature configured, grep-verify the global row/clone appears in NONE of: board lists, unscoped `GET /api/tasks`, `GET /api/projects`, Activity lists/stats, cleanup panel project groups, MCP `list_tasks`/`list_projects`
- [ ] **Stop affordance:** stopping the global kills exactly its sessions — dev `/terminal` sessions and all task sessions untouched
- [ ] **Root reconfigure gate:** 409 with reasons while live; after stopping, reconfigure succeeds and the old managed clone's fate (gated removal / surfaced orphan) matches the chosen design
- [ ] **Path collision proof:** add a project for the same `owner/name` as the global root — both coexist; deleting the project never touches the global root
- [ ] **Shared-cwd honesty:** folder-mode root shows the not-isolated warning; `git status` summary (if built) reflects real state
- [ ] **MCP contract:** `list_sessions` shows global sessions with synthesized context; `start_task_agent` for a global correctly errors (it's task-only) rather than half-working

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Sentinel project leaked onto surfaces (P2) | MEDIUM | Add the missing filters one by one (whack-a-mole), or migrate the global off the sentinel (a real migration + backfill). The longer it lives, the more filters exist to find |
| Status feed misses global (P1) | LOW | Both passes are additive branches in one handler + tests; no schema involvement if ids are already persisted |
| No global resume persistence shipped (P5/P7) | MEDIUM | Retro-fit a Phase-3-style insertion phase (v1.12's 12.1 precedent): migration + sweep/reconcile widening + tests; live sessions unaffected |
| Path collision shipped (P8) | HIGH | Migrate the global clone to the namespaced dir while sessions are stopped (a miniature of v1.8's `~/.kamacu` migration: gated, idempotent, path rewrite in config); until then the collision is latent data-loss risk |
| TaskID=0 overload shipped (P4) | MEDIUM | Introduce the scope discriminator and migrate nothing (in-memory only) — but every handler that grew a `== 0` special case needs revisiting |
| Ungated reconfigure shipped (P7) | LOW | Add the 409 gate + resume-id clearing; users may have already hit the split-brain confusion once (support-level fix) |

## Pitfall-to-Phase Mapping

Phases are the natural structure the roadmap will likely take (provisional names; adjust when scoping):

| Pitfall | Prevention Phase | Verification |
|---------|------------------|---------------|
| P2 sentinel row | Phase 1 — Global data foundation | Sentinel-leak regression test enumerating every listing surface |
| P3 shared-cwd risk | Phase 1 (validation/warning) + Phase 4 (UAT) | Settings shows warning on folder roots; UAT script exercises a real folder |
| P7 reconfigure semantics | Phase 1 (decide) + Phase 4 (gate) | 409 test with a live fake session; resume-id clearing on success |
| P8 clone namespace | Phase 1 (path format) + Phase 4 (clone + orphan fate) | Collision test: project + global same repo coexist; delete project leaves global intact |
| P1 status-feed invisibility | Phase 2 — Session surface & status | `/api/agents/status` returns the global entry live AND post-restart |
| P4 TaskID=0 overload | Phase 2 | Stop-global kills only global sessions (dev + task untouched) |
| P6 bar/UI contract | Phase 2 | Bar row renders, navigates, highlights; `source` union grep clean |
| P9 lifecycle decision | Phase 2 (stop) + Phase 4 (out-of-scope doc) | REQUIREMENTS line; stop button works |
| P5 restart amnesia | Phase 3 — Persistence & restart | tmux survives restart; agent resumes after restart (both engines) |

Ordering rationale: the data-foundation phase carries the highest blast-radius decisions (representation, clone namespace, reconfigure semantics) because Pitfalls 2/7/8 get exponentially more expensive once later phases build on a wrong base — this mirrors v1.9/v1.10's migration-first, backfill-guarded pattern. Phase 2 is the risk center for visibility (P1/P4/P6 land or the feature is DOA in UAT). Phase 3 exists specifically because restart bugs are invisible until a restart (P5). Phase 4 closes the lifecycle/managed-clone/gate hardening (P3/P7/P8/P9) where the gated-delete philosophy already provides the templates.

**Research flags for phases:** Phase 2 needs a short spike on the `agentStatusEntry` wire widening (nullable-vs-synthesized choice ripples into 4 frontend consumers); Phase 4 needs the gated-clone-removal design reviewed against `CleanupWorktreeGated` before implementation (adapt, don't call with a fake taskID).

## Sources

- **Codebase (primary, HIGH — read directly in this worktree):**
  - `internal/api/agents.go:44-219` — the two-pass status handler: `TaskID <= 0` skip, INNER JOIN meta query, worktree-gated resumable, DB-derived pass
  - `internal/api/sessions.go:223-496` — spawn handler (agent requires task+worktree, tmux needs task 409, `kamacu-<task>-<n>` minting, resume validation); `:595-622` `defaultTmuxLabel`; `:693-858` opencode capture; `:917-964` `joinSessionContext`
  - `internal/session/manager.go:53-79` (`SpawnOpts.TaskID` contract), `:114-385` (spawn, cwd validation, label counters), `:421-423` (`ListByTask`), `:456-475` (`StopAllForTask`)
  - `internal/reaper/reaper.go:140-205` (Done-TTL keyed on `tasks.status='done'`), `:223-344` (PR pass gates)
  - `cmd/kamacu/serve.go:304-401` — startup orphan sweep kills `kamacu-*` with no DB row
  - `internal/tmux/tmux.go:78-103` — `has-session` prefix-matching gotcha
  - `internal/store/migrations/00001/00002/00003/00005` — `tasks.project_id NOT NULL REFERENCES projects`, `tmux_sessions.task_id NOT NULL REFERENCES tasks`
  - `internal/api/tasks.go:182-233, 288, 432, 538, 554` — `source='manual'` board/position/list filters; `internal/api/activity.go:135` — Activity manual filter
  - `internal/api/projects.go:355-478` — v1.4 managed-clone dest/dedup/reattach; gated delete (`os.RemoveAll` on the clone)
  - `internal/api/cleanup.go:103-168` — `CleanupWorktreeGated` gates + `taskID > 0` branch
  - `internal/api/agents_backfill.go:108-114` — `--dangerously-skip-permissions` code default
  - `internal/mcp/sessions.go:94-115, 477-514` — `list_sessions` filters + no-filter semantics
  - `web/src/api/agents.ts:4-18`, `web/src/components/layout/ActiveSessionsBar.tsx:127-136, 243-268`, `web/src/pages/TaskPage.tsx:105, 278`, `web/src/App.tsx:117-144` — wire contract, row rendering/navigation/keying, source branching, routes
  - `.planning/PROJECT.md` — v1.13 milestone brief + shipped-history context (v1.4 gated delete, v1.6 bar, v1.8 GAP-01 label regression, v1.9 workspace enumerations, v1.10 agent defaults, v1.11 MCP surface, v1.12 activity scopes)
- **Web (secondary, MEDIUM — cross-validation only; cached in research-store):**
  - INNER JOIN drops NULL-keyed rows; "expected rows missing" as the classic symptom — sqlitetutorial.net LEFT JOIN, sqlteam.com forum thread #8262, r/SQL thread tvn6er (retrieved 2026-08-25)
  - Agents in shared checkouts overwrite work / delete crucial files; worktrees as the standard isolation — nx.dev "Git Worktrees Changed My AI Agent Workflow", augmentcode.com multi-agent workspace guide (2026), "Stop letting AI agents fight over your codebase" (retrieved 2026-08-25)
- **Experience-based (project history, HIGH):** the v1.8 Phase 24 GAP-01 bug (renamed tmux tab reverted after restart because the reattach path dropped the persisted label) is the precedent for P5's "works until restart" class; the v1.9 Phase 26 empty-workspace `startTransition` bug is the precedent for P6's "route/param assumptions" class.

---
*Pitfalls research for: v1.13 Global Task — adding a global session surface to a task/worktree-centric app*
*Researched: 2026-08-25*
