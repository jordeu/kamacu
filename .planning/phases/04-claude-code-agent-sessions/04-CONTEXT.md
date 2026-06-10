# Phase 4: Claude Code Agent Sessions - Context

**Gathered:** 2026-06-10
**Status:** Ready for planning

<domain>
## Phase Boundary

The product's payoff: an explicit Start button spawns the real `claude` CLI in the task's worktree PTY with full TUI fidelity (plan mode, slash commands, permission prompts); live session-status dots on kanban cards (working / idle / waiting / exited) driven by hooks injected at spawn plus output-activity heuristics; clean alt-screen reattach for the Claude TUI (the hard replay problem deferred from Phase 2 behind the `Snapshot()` seam). (TERM-01, STAT-01, STAT-02)

Out of this phase: `--resume`/`--continue` recovery after server restarts and startup reconciliation (Phase 5), diff tab (Phase 5), browser notifications and move-to-review suggestions (v2), multiple agent CLIs (out of scope).

Installed Claude Code on this host: v2.1.170 at `~/.local/bin/claude` — verify all CLI/hook behaviors against this binary.

</domain>

<decisions>
## Implementation Decisions

### Start & Initial Prompt
- **D-35:** Start launches plain interactive `claude` — the task description is NOT auto-sent. An "Insert task description" action types the description into the prompt (bracketed paste, does NOT submit), editable before sending. Repeatable.
- **D-36:** The insert action lives in the agent pane header (next to Stop), available whenever the session is running.
- **D-37:** Tasks with no description: nothing special — clean start, insert action hidden or disabled.

### Agent Tab & Lifecycle
- **D-38:** The Agent tab is a permanent first tab. Tab order: **Agent, Description, Bash 1..N**. It cannot be closed — only stopped. Exactly one agent session per task.
- **D-39:** Opening a task always lands on the first tab (Agent). No smart tab selection.
- **D-40:** Pre-start state: the Agent tab shows a Start button (plus context: branch name, short explanation). Start is disabled with the same pattern as bash `+` when the task has no worktree (message points to Create worktree).
- **D-41:** Stop = the standard Phase 2 teardown (SIGTERM→5s→SIGKILL on the session). After exit/stop the tab shows the exited banner with **"Start again"** which launches a FRESH `claude` session (no --continue/--resume in this phase — that's Phase 5 recovery scope).
- **D-42:** The agent session counts in the cleanup gate exactly like bash sessions: it appears in "N sessions running" and "Stop sessions and clean up" stops it too. One consistent rule.

### Status Badges & Attention
- **D-43:** Card badge is a **dot only** (no text): green = working, amber = waiting for input, gray = idle, muted gray = exited(0), red = exited(non-zero). Tooltip gives the status word.
- **D-44:** Waiting is loud: the amber dot pulses AND the card gets a subtle highlighted border. Everything else is calm.
- **D-45:** Waiting clears **on attach** (opening the task's agent tab); it re-fires if Claude prompts again.
- **D-46:** Claude finishing a turn (Stop hook) while the session stays open = **idle** (gray). No separate "done" state — the board only gets loud when input is NEEDED.
- **D-47:** Status detection = hooks for precise transitions (Notification → waiting; Stop → idle) + PTY output-activity heuristic for working-vs-idle between hook events (output flowing = working; quiet ~10s = idle).
- **D-48:** Card badges reflect the AGENT session only. Bash sessions never drive board state.
- **D-49:** Sidebar shows an amber waiting-count chip next to project names with ≥1 waiting agent (e.g. "Fusion ·2·"); hidden when zero. Cross-project dispatching at a glance.
- **D-50:** The same status dot renders on the Agent tab label inside the task view (visible while on Description/bash tabs).

### Claude Spawn Config
- **D-51:** Default interactive permission posture — plain `claude`, normal permission prompts in the terminal (waiting-detection surfaces them on the board). No mode picker, no --dangerously-skip-permissions.
- **D-52:** The spawned claude inherits EVERYTHING the user's terminal would have: user settings (~/.claude), the worktree's CLAUDE.md, MCP servers, env. Run it as the user would run it.
- **D-53:** Status hooks are injected invisibly and additively: they must not interfere with or replace the user's existing hooks, must not leave files in the user's repos or global config, and `claude` sessions started outside Kangent are untouched. Mechanism (e.g. `--settings` overlay file in an app dir) is research's job to verify against v2.1.170.

### Claude's Discretion
- Alt-screen reattach strategy for the Claude TUI (the deferred Phase 2 problem — `Snapshot()` seam, resize-jiggle, or headless VT state; pick what's verifiably clean on reattach with the real claude binary)
- Hook payload parsing, session-ID capture mechanics (persist what Phase 5 will need for --resume, schema-wise, even if unused this phase)
- Exact dot styling within UI-SPEC color budget (amber is new — needs UI-SPEC treatment; this is THE badge moment the palette was reserved for)
- How status flows to the board (polling the sessions list vs SSE/WS push — pick pragmatically; 5s polling already exists in useSessions)
- Working/idle quiet-threshold tuning
- Whether agent sessions get a distinct label ("Agent") in the session manager vs "Bash N" numbering

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project planning
- `.planning/PROJECT.md` — core value; "explicit Start button" is a founding key decision
- `.planning/REQUIREMENTS.md` — Phase 4 owns TERM-01, STAT-01, STAT-02
- `.planning/ROADMAP.md` — Phase 4 success criteria (4)

### Research
- `.planning/research/FEATURES.md` — waiting-detection is the make-or-break feature; hooks-at-spawn design constraint; anti-feature: automatic column transitions (do NOT auto-move cards on Stop hook)
- `.planning/research/PITFALLS.md` — alt-screen replay problem (raw ring corrupts mid-escape; resize-jiggle with known Claude Code debounce caveat); resume determinism via `claude --session-id` at spawn; hook payload verification flagged version-dependent
- `.planning/research/ARCHITECTURE.md` — session manager seams; status flow
- `.planning/phases/02-terminal-engine/02-RESEARCH.md` — the Snapshot() seam left for this phase; SIGWINCH jiggle semantics already verified

### Existing code (integration points)
- `internal/session/` — manager with SpawnOpts{Cwd, TaskID}, labels, StopAllForTask; the agent session rides this
- `internal/api/sessions.go`, `internal/api/worktrees.go`, `internal/api/tasks.go` — REST surface to extend (agent spawn endpoint or kind field; status in task/session payloads)
- `internal/ws/handler.go` — attach path (waiting-clears-on-attach hook point, D-45)
- `web/src/pages/TaskPage.tsx` + `web/src/components/task/TaskTabs.tsx` — tab strip to gain the permanent Agent tab
- `web/src/components/terminal/TerminalPane.tsx` — the pane the agent tab mounts
- `web/src/components/board/TaskCard.tsx` — where dots land
- `web/src/components/sidebar/ProjectSidebar.tsx` — waiting-count chips (D-49)
- `.planning/phases/01-foundation-projects-board/01-UI-SPEC.md` — the reserved color budget this phase finally spends

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- The complete terminal stack: agent session = `SpawnOpts{Cwd: worktree, TaskID}` with command `claude` instead of bash + a `kind` discriminator
- Exited banner + "Start again" maps onto the Phase 2 exited UX with one new action
- `useSessions` polling (5s) already exists — status dots can ride it or be upgraded
- Cleanup gate (D-42) needs zero new logic if the agent session is just another session under StopAllForTask

### Established Patterns
- Hooks/REST/test conventions from Phases 1-3; UI-SPEC inheritance chain (this phase needs a 04-UI-SPEC for dots/amber/pulse before planning UI tasks)

### Integration Points
- Session manager: `kind: agent|bash` (one agent per task enforced server-side), session-ID capture for Phase 5
- Hook receiver endpoint on the server (localhost POST target for injected hooks) — must validate origin/host like everything else
- Status aggregation: task → agent session status, exposed in tasks list payload (board) + sessions payload (tabs)

</code_context>

<specifics>
## Specific Ideas

- "Run claude exactly as I would in my terminal, plus invisible status wiring" is the spirit of D-51..D-53
- The board-as-dispatcher: green/amber/gray dots scanning across columns, amber pulses pull you in, sidebar counts route you across projects
- Verify everything against the installed claude v2.1.170 — hooks config shape, Notification/Stop payloads, alt-screen behavior, bracketed-paste insert

</specifics>

<deferred>
## Deferred Ideas

- Browser notifications on waiting/finished (v2 NOTF-01 — detection built this phase makes it cheap later)
- "Move to In Review?" suggestion on Stop hook (v2 NOTF-02 — explicitly do NOT auto-move cards)
- `--continue`/`--resume` recovery (Phase 5 RCVR-02 — but capture session IDs now)
- Per-session permission-mode picker (only if default posture proves annoying)

</deferred>

---

*Phase: 04-claude-code-agent-sessions*
*Context gathered: 2026-06-10*
