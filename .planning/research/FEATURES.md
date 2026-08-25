# Feature Research

**Domain:** A single global scratchpad surface ("Global Task") in an existing local-only agent-orchestration app — a task-like view (agent + bash tabs only) whose sessions run directly in a configured repo/folder root with no worktree, no branch, and no project/task linkage, reachable from the global Active Sessions bar.
**Researched:** 2026-08-25
**Confidence:** MEDIUM overall — comparable-tool behavior surveyed from official docs/sites (fetched Aug 2026); every claim about existing Kamacu behavior is HIGH confidence, verified directly against the code in this worktree (`internal/api/{agents,sessions,settings}.go`, `internal/reaper`, `internal/session`, PROJECT.md).

---

## TL;DR for the roadmap author

- **Scratch must be a first-class citizen, not a degraded task.** Every comparable surface (JetBrains scratches are "fully functional, runnable, and debuggable"; VS Code terminals get full persistence; SlayZone cards without worktrees get identical terminals) treats ad-hoc work as feature-equal to structured work. The global task should reuse the *same* TaskPage/TaskTabs/TerminalPane machinery minus tabs — not a bespoke lighter terminal widget.
- **The pattern across tools is: configured context + low ceremony + explicit promotion.** ttyd/gotty = one configured command in one configured cwd, always there. JetBrains = IDE-global scratch, with an F6 "move into project" promotion path when scratch outgrows itself. vibe-kanban = "standalone workspaces for quick questions about a codebase or one-off tasks" (no issue required). Copilot Spaces = a named context bundle outside the PR flow for ad-hoc Q&A. Kamacu's global task is exactly this shape: one configured root + one configured agent + explicit Start.
- **Singleton is the right scope.** JetBrains caps scratch *buffers* at five rotating slots precisely because unbounded global scratch accumulates into clutter (their own support forum documents the pain: "35 console files across 10–15 projects"). Kamacu's single-instance scratchpad is the most aggressive — and simplest — bound on that failure mode. Don't build N scratchpads.
- **The codebase's real work is in four seams, not in the terminal UI.** (1) `create()` hard-409s agent spawns with `TaskID <= 0` ("task has no worktree") — the gate must branch to a global-root path; (2) engine/command resolution rides a `tasks→projects→agents` JOIN — global needs an agents-row lookup from settings; (3) `/api/agents/status` skips `TaskID <= 0` sessions — the Active Sessions bar is blind to a global session today; (4) restart-resume ids live in per-task columns (`tasks.claude_session_id` / `opencode_session_id`) — a task-less session needs its own persistence. The session *manager* already supports task-less sessions (`SpawnOpts.TaskID = 0`, "dev sessions").
- **No reaper will ever touch the global session — make Stop prominent.** The Done-TTL reaper is task-keyed (`tasks.done_at`); a task-less session is immortal until explicitly stopped. That's the correct scratch semantic (users trust a scratchpad not to vanish), but the ⋯ Stop affordance must be at least as reachable as a task's.

---

## Feature Landscape

### Table Stakes (Users Expect These)

Omitting any of these makes the scratchpad feel like a broken task view rather than a scratchpad.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| **Full session parity: persistent server-side PTY, leave/reattach with replay, restart reconciliation + Resume, live working/waiting/idle status** | JetBrains scratches are "fully functional, runnable, debuggable"; VS Code gives terminals process reconnection (reload) AND process revive (restart). Scratch ≠ toy. Kamacu's own spec: "same session semantics as tasks." | MEDIUM | All machinery exists (ring replay, `claude --resume`, opencode `ses_…` capture). New work: persisting resume ids for a task-less surface (settings KV row or singleton storage) + reconciliation that knows about the global session. |
| **Global Settings section: scratch root + default agent** | ttyd/gotty's whole model is "one configured command in one configured cwd"; Copilot Spaces = explicitly configured context for ad-hoc Q&A. A scratch surface without a configuration home is unusable for repo-rooted checks. | MEDIUM | Settings KV needs no migration (absent row = default — verified in `internal/settings`). New keys for root path (+ managed marker) and agent id. The gh-cloned root reuses the v1.4 `github.Clone` + `managed` pattern wholesale. Agent picker reuses the v1.10 agents CRUD list. |
| **Global root works with BOTH a local folder path AND a gh-cloned managed repo** | Kamacu's own v1.4 precedent: repo-first with folder fallback. Spec'd as a target feature. The folder is also the non-GitHub escape hatch. | MEDIUM | `github.Clone` (gh-authenticated, `~/.kamacu/repos/<owner>/<name>`, exit-0-only, atomic) is directly reusable; a `managed`-style marker distinguishes Kamacu-owned scratch roots from user folders (delete semantics differ). |
| **Live reachability from the Active Sessions bar + click-through** | SlayZone's "Needs attention" panel surfaces agent state globally across cards; Kamacu's bar is already the cross-workspace global surface (WSBAR-01). A live global session invisible to the bar is a ghost. | MEDIUM | `/api/agents/status` today skips `info.TaskID <= 0` and JOINs `tasks.title`/`projects.name` for row labels — a global pass with a synthetic label ("Scratch" / root name) is required. Bar rows navigate by task/project id; a global row needs its own route target. |
| **Idle reachability from Settings** | Spec'd. Mirrors JetBrains: scratches live in a global view (Scratches and Consoles), not on any board. | LOW | A Settings section row/button opening the scratch view; disabled/empty state when unconfigured. |
| **Agent tab + bash tabs ONLY — no description, no diff, no board** | The entire point: low ceremony. JetBrains buffers have no language assistance; vibe-kanban standalone workspaces skip the issue linkage; Copilot Spaces sessions produce no PR. | LOW | Reuse TaskPage/TaskTabs shell with a reduced tab set. Bash tabs reuse the existing spawn paths (`settings.shell`, tmux-backed) — but tmux naming (`kamacu-<task>-<n>`) and `tmux_sessions` rows are task-keyed; a global naming scheme + orphan-sweep awareness is needed. |
| **Singleton: exactly one global scratchpad** | Bounded scratch is the norm (JetBrains' 5 rotating buffers; ttyd's single shared process). Prevents the documented accumulation failure mode. | LOW | Enforce one view/route; a second "open" reattaches. The agent spawn path already has a one-agent-per-task gate (D-38) — a global analog (`ListByTask(0)`-equivalent or a dedicated list) enforces one agent. |
| **Honest "no isolation" legibility** | VS Code terminals run directly on the workspace root checkout — that's the norm users expect from a scratch terminal. But Kamacu's task model *promises* worktree isolation, so the global view must make the difference legible (root path in the header; copy like "runs directly in <root>"). | LOW | Mostly copy + showing the configured root (and, if cheap, its checked-out branch) in the view header. No gating logic. |
| **Path validation + missing-root degrade** | Kamacu validates project folder paths at create; a configured root that was deleted or never existed must degrade cleanly (clear error at Start, view shows an unconfigured/broken state, Settings shows the error) — never a half-spawned session. | LOW–MEDIUM | Folder-exists validation exists for projects; reuse. The v1.4 "degrade-don't-break" clone-failure pattern applies to the gh path. |
| **Explicit Stop with full-process-tree kill, ⋯ menu parity** | Scratch sessions are immortal otherwise (reaper is task-keyed — verified). JetBrains auto-deletes only *empty* scratches; nothing auto-kills live ones. | LOW | `Stop` already kills the process tree; the global surface just needs the affordance (⋯ menu shaped like the task view's). |

### Differentiators (Competitive Advantage)

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **Managed gh-cloned scratch root** | No comparable local tool offers an authenticated GitHub clone as the ad-hoc workspace. Copilot Spaces attaches repos cloud-side; ttyd/gotty run whatever you launched. Reusing v1.4 makes this nearly free. | MEDIUM | `github.Clone` + managed marker + gated delete-if-ever-needed. Best-effort default-branch fetch before agent starts is optional polish (tasks do it before worktree creation). |
| **Conversation-level restart-resume on a task-less surface** | VS Code revives the *process*; Kamacu resumes the *conversation* (`claude --resume <id>`, opencode `ses_…`). Extending that to scratch is beyond the comparables. | MEDIUM | Persist global claude/opencode session ids outside the tasks table (KV row is sufficient — single instance). Resume validation (transcript glob for claude; persisted id for opencode) is engine-branched already. |
| **Scratch sessions visible to the agent-delegate surface (MCP)** | An agent in any task can `list_sessions` / read-only-tail the user's scratch session — "watch what I'm trying in the scratchpad" — with the v1.11 type-level read-only contract intact. | LOW–MEDIUM | Falls out almost for free if `/api/sessions` + status JOINs carry a global label instead of empty taskTitle/projectName. Must verify MCP tools don't 404/garble on task-less sessions — a no-regression item, not new surface. |
| **Promotion: scratch → real task** (JetBrains F6 analog) | When scratch work turns real, promote it into a project task (which mints the worktree/branch and carries the conversation via the agent's own resume). JetBrains' scratch→project move is the canonical pattern; no kanban-agent tool has it. | HIGH | Deferred beyond v1.13 (see MVP). Hard part is semantics (which project? what happens to the running session?) — needs its own design pass. Park with an ID like GT-FUT. |
| **Scratch reset with preserved history** (rotation analog) | JetBrains rotates 5 buffers, clearing the oldest. A "Reset" that clears the live session but keeps the transcript reachable matches Kamacu's existing Reset-session semantics. | LOW | Reuse the task view's Reset flow (kill PTY, keep transcript, mint new id at next start). |

### Anti-Features (Commonly Requested, Often Problematic)

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| **Description / diff / kanban presence for the global task** | "It's basically a task, give it task things." | Spec-excluded for good reason: a diff has no base (no branch, no merge-base — the diff machinery keys on `worktree_path` + base ref), a description invites ceremony, and board presence destroys the cross-workspace/global nature (boards are per-project). | Promotion to a real task when work grows (differentiator, deferred). |
| **Worktree/branch isolation for scratch sessions** | "It edits a real checkout — unsafe!" | Defeats the scratchpad (a worktree mint per scratch check is exactly the ceremony the feature removes); vibe-kanban keeps worktrees even for standalone workspaces and pays the creation/cleanup cost on every one-off. Kamacu's isolation story stays: tasks = isolated, scratch = direct (and legibly labeled). | Honest no-isolation labeling + Stop; promote to task for isolation. |
| **Multiple scratchpads (per-workspace scratch, scratch grid)** | "One per workspace would be neat." | The documented global-scratch failure mode is accumulation (JetBrains forum: dozens of orphaned consoles across projects). N instances also multiply every seam (labels, resume ids, tmux names) for negligible value at single-user scale. | Single instance + Reset. Revisit only with evidence of real pain. |
| **Auto-starting the agent (at boot, or on opening the view)** | "It's a scratchpad — it should just be there running." | Violates Kamacu's explicit-start philosophy (existing Out-of-Scope: no auto-start on task creation); a boot-spawned agent burns quota invisibly; ttyd's always-there process is a *terminal*, not a metered agent. | Explicit Start button, same as tasks. |
| **TTL / auto-cleanup of the scratch session** | "Scratch should self-clean." | The reaper is task-keyed by design; surprise-killing a scratch session destroys the trust that makes a scratchpad usable (JetBrains deletes only *empty* scratches, never live ones). | Immortal-until-stopped + prominent Stop; Reset for a fresh slate. |
| **Browser/OS notifications for scratch state** | Parity with SlayZone's desktop alerts. | Kamacu deliberately scoped alerting to in-bar only (SBAR-FUT-03 precedent); scratch is low-stakes by definition. | The bar's existing pulsing-amber waiting treatment covers it. |

---

## Feature Dependencies

```
[Global Settings section]
    └──requires──> [settings KV new keys: root path (+managed marker), agent id]  (exists; no migration)
    └──requires──> [v1.10 agents CRUD]                      (exists — agent picker)
    └──requires──> [v1.4 github.Clone + managed pattern]    (exists — gh root option)

[Global task view (agent + bash tabs)]
    └──requires──> [TaskPage/TaskTabs/TerminalPane shell]   (exists — reduced tab set)
    └──requires──> [session.Manager task-less sessions]     (exists — SpawnOpts.TaskID = 0 "dev sessions")
    └──requires──> [relaxed agent spawn gate]               (NEW — create() 409s TaskID<=0 agents today)
    └──requires──> [global engine resolution]               (NEW — agents-row lookup by settings id,
                                                                replacing the tasks→projects→agents JOIN)
    └──requires──> [singleton agent gate]                   (NEW analog of D-38's ListByTask gate)

[Bash tabs in scratch]
    └──requires──> [tmux naming + rows for task-less tabs]  (NEW — kamacu-<task>-<n> + tmux_sessions
                                                                are task-keyed; reattach path requires TaskID>0)
    └──enhances──> [startup orphan sweep must recognize global rows]

[Active Sessions bar integration]
    └──requires──> [/api/agents/status global pass]         (NEW — skips TaskID<=0 + task-centric JOIN today)
    └──requires──> [bar row route for a non-task target]    (NEW — rows navigate by task id today)

[Restart resume for scratch]
    └──requires──> [global persistence for claude/opencode ids]  (NEW — tasks.*_session_id columns are task-only)
    └──requires──> [reconciliation pass for the global session]  (NEW sibling of the task pass)

[MCP no-regression] ──conflicts-if-ignored──> [status/list JOINs]
        (global sessions have empty taskTitle/projectName; MCP tools + Active Sessions bar
         must not 404/garble — label widening, not new tools)

[Reaper] ──deliberately-excludes──> [global sessions]
        (Done-TTL keys on tasks.done_at — task-less sessions are never reaped; correct, but
         makes explicit Stop the only death path)
```

### Dependency Notes

- **Spawn gate is the load-bearing change:** `internal/api/sessions.go` `create()` rejects agent spawns with `TaskID <= 0` (`409 "task has no worktree"`) and resolves engine/command/extra-params via `tasks → projects → agents`. The global path needs: read root from settings → resolve agent row by settings-stored agent id → set `opts.Cwd` to the root → skip the worktree-validity branch. Everything downstream (engine fork, hook env, status) keys off `SpawnOpts`, which already tolerates `TaskID = 0`.
- **The bar is blind today:** `/api/agents/status` filters `info.TaskID <= 0` and labels rows via `tasks.title`/`projects.name`. The v1.6 bar (and v1.11 MCP `list_sessions` label JOIN) need a global entry with a synthetic label. This is the same "widen the query, no new endpoint" move as SBAR-10.
- **Resume storage is the only schema-ish decision:** single-instance scratch means a settings-KV row per engine id is sufficient (no new table needed); a singleton-row table is the alternative if more metadata accrues (last-started-at, root provenance). Either way, absent-migration is achievable via KV.
- **tmux tabs are the sneaky one:** tmux session names, `tmux_sessions` rows, the reattach branch (`reattach requires TaskID > 0`), and the startup orphan sweep all key on tasks. Global bash tabs need a `kamacu-global-<n>`-style scheme + row strategy, or global bash tabs ship plain-bash-only (cheaper; loses restart survival for scratch bash tabs — an explicit tradeoff to surface in planning).
- **Reaper/restart interplay:** restart reconciliation for tasks reconstructs ghosts from DB rows; a global session with no row needs its own reconcile path (KV-stored ids + `has-session`/transcript checks) or it silently vanishes from the bar after a restart — exactly the "ghost" class v1.9's tmux work eliminated for tasks.

---

## MVP Definition

### Launch With (v1.13)

- [ ] Global Settings section — root (folder path with exists-validation, or `owner/name` gh-clone reusing v1.4 atomically) + default-agent picker (v1.10 list) — **why essential: nothing else is reachable without configuration**
- [ ] Scratch view — full TaskPage-derived shell, agent tab + bash tabs only, root path legible in the header, ⋯ Stop parity — **why essential: the surface itself**
- [ ] Full session semantics — persistent PTY, reattach+replay, restart reconcile + Resume (engine-branched), live status — **why essential: the parity principle every comparable upholds; a toy scratch erodes trust**
- [ ] Active Sessions bar integration — live global row with synthetic label + click-through — **why essential: spec'd; a live-but-invisible session is a ghost**
- [ ] Singleton enforcement + explicit Start — **why essential: bounded scratch + Kamacu's explicit-start philosophy**
- [ ] MCP/no-regression pass — `list_sessions`/`get_session`/status surfaces handle task-less sessions with a label, never 404/garble — **why essential: v1.11 contract must hold**

### Add After Validation (v1.x)

- [ ] Promotion scratch→task (JetBrains F6 analog) — trigger: users report "did real work in scratch" — needs its own design pass (project choice, session handoff)
- [ ] tmux-backed global bash tabs (restart-surviving scratch bash) — trigger: if plain-bash-only shipped and users miss restart survival
- [ ] Scratch Reset with reachable prior transcript — trigger: users accumulate one long-lived agent conversation

### Future Consideration (v2+)

- [ ] Multiple named scratch roots / per-workspace scratch — only with documented pain (JetBrains' accumulation precedent says: don't)
- [ ] Root dirty-state/branch indicator in the bar row — polish; only if "which branch is scratch on" confusion shows up

---

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Settings section (root + agent) | HIGH | MEDIUM | P1 |
| Scratch view (agent + bash tabs, reduced chrome) | HIGH | LOW–MEDIUM | P1 |
| Session parity incl. restart resume | HIGH | MEDIUM | P1 |
| Active Sessions bar integration | HIGH | MEDIUM | P1 |
| Singleton + explicit Start/Stop | MEDIUM | LOW | P1 |
| gh-cloned managed root | MEDIUM | MEDIUM (reuses v1.4) | P1 (spec'd) — P2 if cuts needed |
| MCP no-regression labels | MEDIUM | LOW | P1 (guardrail) |
| Global tmux bash tabs | MEDIUM | MEDIUM | P2 (plain-bash first is defensible) |
| Promotion scratch→task | HIGH (later) | HIGH | P2/P3 (post-v1.13) |
| Scratch history/rotation | LOW | LOW | P3 |

---

## Competitor Feature Analysis

| Feature | SlayZone | vibe-kanban | VS Code | JetBrains | ttyd/gotty | Copilot Spaces | Our Approach (v1.13) |
|---------|----------|-------------|---------|-----------|------------|----------------|----------------------|
| Ad-hoc surface outside task/PR structure | Not documented — every surface is a card, but cards may omit worktrees ("assign manually or auto") | Yes — "standalone workspaces" with no issue, for "quick questions / one-off tasks" | Terminals are window/workspace-scoped; agents' sessions live in the Agents window | Yes — IDE-global scratches/consoles, project-independent | The whole product: one global terminal | Yes — Spaces = configured context outside PR flow for Q&A | Single global task view, no project linkage |
| Where sessions run | Card folder or worktree (worktree optional per card) | Always an isolated worktree, even for standalone workspaces | Workspace root directly (default cwd) | IDE config dir (files); consoles run with module classpath | Fixed cwd at launch; `--wd` flag | Cloud-side over attached repos | Configured root directly — no worktree, honestly labeled |
| Session persistence / resume | PTY per card; attention-state machine | `spawn` + `spawn_follow_up` by persisted session id | Reconnect on reload; revive (content + relaunch) on restart | Files persist in config dir | Dies with process (client reconnect only) | Sessions in history | Full Kamacu parity: PTY + replay + `--resume`/`ses_…` — the strongest of the set for a scratch surface |
| Global visibility of ad-hoc work | "Needs attention" panel across tasks | Board + per-task status | Terminal badges / agents window status | Scratches view in Project tool window | Single surface (nothing to aggregate) | Spaces list | Active Sessions bar (cross-workspace) global row + click-through |
| Configured context for the ad-hoc surface | Per-card repo | Per-workspace repos | Workspace folders / profiles | `idea.scratch.path` property | CLI flags (command, cwd, credential) | Repos/files/PRs/issues/free-text per Space | Global Settings: folder or gh-managed root + default agent |
| Bounds on scratch accumulation | Cards are cheap (board clutter risk) | One per standalone workspace | Unlimited terminals | Buffers capped at 5, rotated; files unbounded (forum pain) | One | Spaces count-capped (community discussion) | Singleton — one scratchpad, Reset for a fresh slate |
| Promotion to structured work | Card → PR (in-card git) | Workspace → PR | — (no analog) | F6 "move scratch into project" | — | — | Deferred (GT-FUT): scratch → task (mints worktree) |

---

## Sources

- SlayZone official features page (slay.zone/features, fetched 2026-08-25) — card-scoped terminals, optional per-card worktrees, attention panel, task/project-scoped dev servers (MEDIUM; marketing page, no separate global surface documented)
- vibe-kanban official docs via zread (BloopAI/vibe-kanban, Overview + Workspace/Worktree Management + Database Models) — standalone workspaces without issues; executor spawn/spawn_follow_up; scratch KV table (MEDIUM; project sunsetting — pattern reference only)
- VS Code official docs, Terminal/Advanced (code.visualstudio.com, dated 2026-08-19) — persistent sessions (reconnection vs revive), terminal scoping, detach/attach (MEDIUM, official)
- JetBrains IntelliJ IDEA 2026.2 official docs, Scratch files (jetbrains.com/help/idea/scratches.html, dated 2026-08-17) + Stack Overflow (storage location) + JetBrains support forum (accumulation complaint) (MEDIUM, official + corroborated)
- ttyd (github.com/tsl0922/ttyd) + Arch man page + x-cmd/tldr mirrors + gotty README — cwd/command config, read-only default, writable flag, single shared process (MEDIUM, cross-verified)
- GitHub Copilot Spaces — docs.github.com/en/copilot/concepts/context/spaces + github.blog announcement (MEDIUM, official)
- Claude Code official sessions doc (code.claude.com/docs/en/sessions) — "a session is a saved conversation tied to a project directory" (MEDIUM, official)
- Kamacu codebase (this worktree): `internal/api/agents.go` (status task-keying + JOINs), `internal/api/sessions.go` (spawn gate, engine resolution, tmux reattach task requirement, one-agent gate), `internal/settings/settings.go` (KV no-migration defaults), `internal/reaper/reaper.go` (task-keyed Done-TTL), `internal/session/manager.go` (`TaskID=0` dev sessions), PROJECT.md v1.4/v1.6/v1.8/v1.10/v1.11 milestone records (HIGH — verified in source)

**Gaps / lower-confidence areas:** SlayZone internals beyond the marketing features page (docs site not crawled — its "no separate global surface" is a negative claim from official pages only); Cursor/Warp scratch-session specifics (searches yielded no primary sources — excluded rather than guessed); whether VS Code's Agents-window session model exposes a cwd-free session (partially documented, not load-bearing here).

---
*Feature research for: v1.13 Global Task (global scratchpad session surface)*
*Researched: 2026-08-25*
