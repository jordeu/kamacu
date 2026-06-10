# Feature Research

**Domain:** Local agent-session orchestration ("vibe kanban" style) — kanban board + browser terminal + worktree-per-task for Claude Code
**Researched:** 2026-06-10
**Confidence:** MEDIUM-HIGH (product feature pages + docs + GitHub issues; some UX details from marketing pages are LOW confidence)

## Comparable Products Surveyed

| Product | Form Factor | Core Model | Status |
|---------|-------------|------------|--------|
| vibe-kanban (BloopAI) | Web app (Rust + TS, local) | Kanban + structured agent log view, 10+ agent CLIs, ephemeral worktrees | Active |
| SlayZone | Electron desktop | Kanban cards each hiding real PTY terminals, worktrees, diff/git UI | Active — closest analog to Kangent |
| claude-squad | TUI (terminal) | tmux-backed sessions, worktree per session, preview/diff tabs | Active |
| Conductor | Mac desktop app | Parallel Claude Code workspaces, diff/merge, notifications, archive | Active |
| Crystal | Desktop app | Parallel sessions in worktrees | Deprecated (Feb 2026, → Nimbalyst) |
| Omnara | Mobile/web dashboard + CLI wrapper | Remote monitoring, push notifications when agent needs input | Active |

**Convergent pattern:** every product in this space has (1) worktree-per-task isolation, (2) some form of automatic session status detection (working/idle/waiting), (3) diff viewing, (4) a way to be told when an agent needs attention. These four are the genre-defining features.

## Feature Landscape

### Table Stakes (Users Expect These)

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Projects sidebar (CRUD, point at local repo) | Entry point in every comparable product | LOW | Validate path is a git repo at creation |
| Kanban board with drag-and-drop columns | The "kanban" in vibe kanban; all products have it | MEDIUM | Fixed columns (To Do / In Progress / In Review / Done) keeps it lean; dnd-kit or similar |
| Task CRUD (title, markdown description, status) | Baseline for any board | LOW | Markdown render in task view; description is what user pastes into Claude |
| Worktree + branch auto-created per task | Genre-defining; vibe-kanban, SlayZone, claude-squad, Conductor, Crystal all do it | MEDIUM | Branch naming convention (e.g., `task/<slug>`); handle dirty repo / name collisions |
| Real interactive PTY terminal in browser | Kangent's core value; SlayZone calls this out as its differentiator vs "chat widgets" | HIGH | xterm.js + WebSocket + server-side PTY (creack/pty in Go); the foundation everything else plugs into |
| Server-side session persistence + reattach | "Open, leave, reattach" is the core value prop; claude-squad gets it via tmux, SlayZone via persistent PTY model | HIGH | Server owns PTYs; ring buffer (~1–2 MB/session) replayed on attach; flow control (high/low water marks) to avoid drowning xterm.js |
| Terminal resize handling | Broken resize makes Claude Code's TUI unusable (garbled panes) | MEDIUM | xterm.js FitAddon → send rows/cols → PTY resize (SIGWINCH). Single-viewer assumption simplifies (last attach wins) |
| Terminal scrollback | Users scroll to read agent reasoning | LOW-MEDIUM | xterm.js scrollback (default 1000, set ~10k lines); on reattach only replay-buffer history exists — accept this, don't persist full history |
| Copy/paste in terminal | Daily-driver requirement | LOW | xterm.js handles paste natively; add copy-on-select option; bracketed paste passes through to Claude Code for multiline prompts |
| Bash session tabs in worktree | All comparables offer extra shells per task (SlayZone: "agents, dev servers, ad-hoc shell work, verification") | LOW-MEDIUM | Cheap once PTY infra exists — same machinery, different command |
| Session status indicator (running / idle / exited) | SlayZone, Conductor, claude-squad, vibe-kanban all show per-task agent state; board is useless for parallel work without it | MEDIUM | Output-activity heuristic: bytes flowing = working, quiet N seconds = idle, process gone = exited. Show badge on kanban card, not just in task view |
| Waiting-for-input detection | The #1 reason these tools exist — SlayZone's "attention panel," Omnara's entire product, Conductor's notifications. Without it users babysit terminals | MEDIUM-HIGH | Heuristics on raw PTY output are fragile. Better: Claude Code hooks — inject per-session `Notification` hook (fires when Claude needs permission/input) and `Stop` hook (fires when it finishes) that POST to the local server. Terminal bell (BEL) as fallback signal |
| Worktree cleanup on task done (with confirmation) | All products handle teardown; vibe-kanban's auto-cleanup caused real bugs (issues #1571, #1764) | MEDIUM | Explicit offer-to-clean (already a PROJECT.md decision) is the safer pattern. Guard: refuse/warn if worktree has uncommitted changes; keep branch |
| Local SQLite persistence | vibe-kanban (SQLx) and SlayZone (SQLite) both; survives restarts | LOW | Projects, tasks, session metadata (incl. Claude session IDs for resume) |

### Differentiators (Competitive Advantage)

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Full-fidelity Claude Code CLI (not a reimplemented chat view) | vibe-kanban renders structured logs and must reimplement approvals/follow-ups per agent; a real PTY gets plan mode, slash commands, permission prompts, `/compact`, themes for free and never lags Claude Code releases | (already core) | This is Kangent's positioning vs vibe-kanban; SlayZone proves the model but is a desktop app |
| Web app at localhost (no Electron) | Open from any browser/tab, lighter than SlayZone/Conductor/Crystal desktop apps; vibe-kanban-style ergonomics with SlayZone-style terminals | (already core) | The combination (real PTY + web) is the niche — none of the surveyed products occupy it |
| Attention badges on kanban cards + needs-attention count | SlayZone's attention panel is its hero feature; surfacing "⚠ waiting 4m" on the card turns the board into a real dispatcher | LOW (once detection exists) | Sort/visually elevate cards needing attention |
| Browser notifications when agent needs input or finishes | Omnara is an entire company built on this; Conductor and vibe-kanban (v0.1.41 "jump back from desktop notifications") both added it | LOW-MEDIUM | Web Notifications API; click → focus task. Needs waiting-detection first |
| `claude --resume` recovery after server restart | No comparable web product handles process death gracefully; persist Claude session ID per task and offer "Resume session" instead of a dead terminal | MEDIUM | Capture session ID via hooks payload or `claude --session-id <uuid>` at spawn; show "exited — resume?" state |
| Read-only diff view per task | Every single comparable has diff viewing; it's what makes the In Review column meaningful | MEDIUM | v1 workaround: `git diff main...` in a bash tab. v1.x: diff tab in task view (server runs git diff, render with a diff component). Read-only — no staging/commit UI |
| Per-task PTY tab layout remembered | Reattach restores not just sessions but which tabs were open | LOW | Falls out of server-owned session model |

### Anti-Features (Commonly Requested, Often Problematic)

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Structured agent log/chat UI (vibe-kanban style) | Prettier than a terminal; clickable approve/deny rows | Requires reimplementing Claude Code's protocol per release; loses plan mode, slash commands, interactive prompts — the exact thing PTY-first avoids | Real PTY (core decision) |
| Automatic kanban column transitions (agent done → In Review) | SlayZone/vibe-kanban auto-track status; feels magical | Heuristic misfires move cards wrongly and erode trust; vibe-kanban needed multiple fixes around execution-state vs board-state drift | Status badge on card + one-click "Move to In Review" suggestion when Stop hook fires; user confirms |
| Automatic/timed worktree cleanup | Disk fills up with stale worktrees (vibe-kanban issue #765 requests it) | vibe-kanban's auto-cleanup produced ghost "running" executions on missing worktrees (#1571) and surprised users by deleting work (#1764, discussion #2335) | Explicit cleanup offer at Done + a "stale worktrees" list the user can purge manually |
| Merge/PR automation, commit UI, branch graph | SlayZone and vibe-kanban have full git UIs and PR lifecycle | Large surface area, high blast radius for bugs, duplicates what user does fine in the bash tab | Manual git in bash tab (already a PROJECT.md decision); keep app's git scope to worktree create/cleanup |
| Multiple agent CLIs (Codex, Gemini, Cursor...) | Every competitor advertises 5–10 agents | Each CLI has different prompts, session resume semantics, hook systems; multiplies status-detection work | Claude Code only for v1; PTY abstraction means adding others later is plumbing, not architecture |
| Embedded browser / dev-server preview panels | vibe-kanban and SlayZone both ship it | iframe/embedded-browser security headaches in a web app; big scope for marginal v1 value | User opens localhost dev server in another browser tab |
| External tracker sync (JIRA/Linear/GitHub Issues) | SlayZone and vibe-kanban offer two-way sync | Auth, webhooks, conflict resolution — an entire product domain | Local SQLite only (PROJECT.md decision) |
| Token usage / cost analytics | SlayZone shows burn-rate meters | Requires parsing agent output or API hooks; cosmetic for single user | `/cost` inside the Claude session |
| Full terminal history persistence across restarts | "I want to scroll back to yesterday" | Serializing/restoring unbounded scrollback is complex (xterm.js #595); replay buffers are capped for good reason | Bounded ring-buffer replay on reattach; rely on `claude --resume` for conversation history, which Claude Code persists itself |
| MCP server so agents update the board | SlayZone's MCP board access is genuinely cool (agent moves its own card, ticks subtasks) | Adds an API surface + agent-config injection in v1; not needed to validate core loop | Strong v2 candidate — noted, not built |

## Feature Dependencies

```
PTY infrastructure (server-side spawn, WS bridge, ring buffer)
    └──requires──> nothing (foundation)

Browser terminal (xterm.js render, resize, copy/paste, scrollback)
    └──requires──> PTY infrastructure

Claude Code session (Start button, cwd = worktree)
    └──requires──> PTY infrastructure
    └──requires──> Worktree creation (task must have a worktree first)

Bash tabs ──requires──> PTY infrastructure (same machinery)

Session persistence / reattach
    └──requires──> PTY infrastructure (server owns process + replay buffer)

Status detection (working/idle/exited)
    └──requires──> PTY infrastructure (output activity)

Waiting-for-input detection
    └──requires──> Claude Code session (hooks injected at spawn)

Attention badges on cards ──requires──> Status + waiting detection
Browser notifications ──requires──> Waiting detection
"Move to In Review" suggestion ──requires──> Stop-hook detection

Worktree creation ──requires──> Project (repo path)
Worktree cleanup ──requires──> Worktree creation; ──enhanced by──> dirty-state check
`claude --resume` recovery ──requires──> Session ID capture at spawn

Diff view (v1.x) ──requires──> Worktree creation
Diff view ──enhances──> In Review column

Auto column transitions ──conflicts──> heuristic status detection (trust erosion)
```

### Dependency Notes

- **Everything sits on PTY infrastructure:** spawn/attach/replay/resize is the single hardest subsystem and should be the first thing built and proven (with plain bash before Claude Code).
- **Waiting detection requires hook injection at spawn:** the Start button should launch `claude` with a per-session settings overlay (`Notification` + `Stop` hooks POSTing to localhost). Retrofitting later means redesigning spawn.
- **Notifications are cheap once detection exists** — don't build detection without immediately surfacing it on cards.
- **Worktree before session:** task creation creates the worktree; Start spawns into it. Cleanup must check for live sessions and uncommitted changes before removal.

## MVP Definition

### Launch With (v1)

- [ ] Projects sidebar (create/list, repo path validation) — entry point
- [ ] Kanban board, fixed 4 columns, drag-and-drop, task CRUD with markdown — the organizing surface
- [ ] Worktree + branch auto-created on task creation — genre-defining isolation
- [ ] Start button → Claude Code in PTY, cwd = worktree — the product's reason to exist
- [ ] xterm.js terminal: resize, scrollback, copy/paste — unusable without these
- [ ] Bash tabs per task — near-free once PTY infra exists, high daily value
- [ ] Server-side persistence + reattach with replay buffer — core value prop
- [ ] Status badge per task card (working / idle / waiting / exited) — board is a dashboard, not just a list
- [ ] Waiting-for-input detection via Claude Code hooks (+ bell fallback) — the feature that lets users actually walk away
- [ ] Done → offer worktree cleanup with dirty-check, keep branch — completes the lifecycle
- [ ] SQLite storage — restarts must not lose the board

### Add After Validation (v1.x)

- [ ] Browser notifications (click → jump to task) — trigger: users report missing waiting-state while in other tabs
- [ ] Read-only diff tab (worktree vs base branch) — trigger: users tire of typing `git diff` in bash tabs during In Review
- [ ] `claude --resume` recovery UI after server restart — trigger: first server restart with live sessions
- [ ] "Move to In Review?" suggestion on Stop hook — trigger: users want lighter board upkeep
- [ ] Stale-worktree list with manual purge — trigger: disk usage complaints

### Future Consideration (v2+)

- [ ] MCP server for agent board access (agent updates own task) — defer: API surface + agent config injection
- [ ] Multiple agent CLIs — defer: per-CLI session/hook semantics multiply detection work
- [ ] Split panes / terminal layouts — defer: tabs suffice for v1
- [ ] Task templates / prompt snippets — defer: validate raw flow first

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| PTY infra + browser terminal | HIGH | HIGH | P1 |
| Session persistence/reattach | HIGH | HIGH | P1 |
| Kanban + task CRUD | HIGH | MEDIUM | P1 |
| Worktree per task + cleanup | HIGH | MEDIUM | P1 |
| Status badges (working/idle/exited) | HIGH | MEDIUM | P1 |
| Waiting-for-input via hooks | HIGH | MEDIUM | P1 |
| Bash tabs | MEDIUM | LOW | P1 |
| Browser notifications | MEDIUM | LOW | P2 |
| Read-only diff tab | MEDIUM | MEDIUM | P2 |
| `--resume` recovery UI | MEDIUM | MEDIUM | P2 |
| Move-to-review suggestion | LOW | LOW | P2 |
| MCP board access | MEDIUM | HIGH | P3 |
| Multi-agent support | LOW (for this user) | HIGH | P3 |

## Competitor Feature Analysis

| Feature | vibe-kanban | SlayZone | claude-squad | Conductor | Kangent (our approach) |
|---------|-------------|----------|--------------|-----------|------------------------|
| Agent UI | Structured log stream, approve/deny rows, follow-up messages | Real PTY (xterm.js + node-pty), split panes | tmux attach in TUI | Workspace view w/ terminal | Real PTY in browser (xterm.js + Go PTY) |
| Status detection | Process states + execution timeline | Terminal state machine: idle/working/attention | Per-session status in list | Status indicators | Output-activity heuristic + Claude Code Notification/Stop hooks |
| Needs-attention UX | Desktop notifications (v0.1.41) | Attention panel: queue, ⚠ badges, wait timers, desktop alerts | Visible on attach | Notifications | Card badges + attention count; browser notifications v1.x |
| Worktree lifecycle | Auto-create per attempt; auto-cleanup (Done +1h) — caused ghost-run/lost-work issues | Manual or auto assign; merge UI from card | Worktree per session; pause = checkout | Isolated workspaces; archive/cleanup | Auto-create on task creation; explicit confirmed cleanup at Done, branch kept |
| Diff viewing | Inline diff + comments fed back to agent | Full diff viewer + staging + commit UI + commit graph | Diff tab w/ scroll | Diff + merge review | None in v1 (bash tab); read-only diff tab v1.x |
| Git automation | PR creation, AI descriptions, merge | Squash/rebase/auto-merge from card, PR lifecycle | Commit+push keybind | Merge capabilities | None — manual in bash tab |
| Persistence | SQLx DB; sessions across restarts | SQLite; persistent PTY sessions | tmux survives detach | Desktop app state | Go-owned PTYs + SQLite; replay buffer; `--resume` for restarts |
| Agents | 10+ CLIs | Claude/Codex/Gemini/OpenCode/Cursor | Any CLI via profiles | Claude/Codex/Cursor | Claude Code only |

## Sources

- [vibe-kanban GitHub](https://github.com/BloopAI/vibe-kanban) — features, stack (MEDIUM)
- [vibe-kanban docs: Monitoring Task Execution](https://vibekanban.com/docs/core-features/monitoring-task-execution) — status/approval/follow-up UX (HIGH, official docs)
- [vibe-kanban issue #1571 (ghost runs)](https://github.com/BloopAI/vibe-kanban/issues/1571), [#765 (configurable cleanup)](https://github.com/BloopAI/vibe-kanban/issues/765), [#1764 (worktree not cleaned after merge)](https://github.com/BloopAI/vibe-kanban/issues/1764), [discussion #2335 (cleanup workflow)](https://github.com/BloopAI/vibe-kanban/discussions/2335) — worktree-cleanup pitfalls (HIGH, primary sources)
- [Vibe Kanban Tool Review — Eleanor Berger](https://elite-ai-assisted-coding.dev/p/vibe-kanban-tool-review) — task lifecycle, review flow (MEDIUM)
- [SlayZone GitHub](https://github.com/debuglebowski/SlayZone), [slay.zone/features](https://slay.zone/features), [attention panel](https://slay.zone/features/attention-panel), [terminals](https://slay.zone/features/terminals) — state machine, attention queue, PTY model (MEDIUM, official pages)
- [claude-squad GitHub](https://github.com/smtg-ai/claude-squad) — tmux sessions, diff tab, pause/resume (MEDIUM)
- [Conductor](https://conductor.build/) — parallel workspaces, notifications, archive (LOW-MEDIUM, marketing page)
- [Crystal GitHub](https://github.com/stravu/crystal) — deprecated Feb 2026 → Nimbalyst (MEDIUM)
- [Omnara GitHub](https://github.com/omnara-ai/omnara) — needs-input notifications as a product (MEDIUM)
- xterm.js persistence patterns: [Zed persistent-terminal RFC](https://github.com/zed-industries/zed/discussions/50584), [copilot-cli scroll fix #1805](https://github.com/github/copilot-cli/issues/1805), [xterm.js #595 state save/restore](https://github.com/xtermjs/xterm.js/issues/595), [xterm.js #518 scrollback](https://github.com/xtermjs/xterm.js/issues/518) — replay buffers, flow-control water marks (MEDIUM)
- Claude Code hooks (`Notification`, `Stop` events) for attention detection — training knowledge of official Anthropic docs; verify exact hook payload/config during phase research (MEDIUM)

---
*Feature research for: local Claude Code agent-session orchestration (kanban + browser PTY + worktrees)*
*Researched: 2026-06-10*
