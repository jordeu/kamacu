# Project Research Summary

**Project:** Kangent — local Claude Code agent orchestration web app
**Domain:** Local-only kanban + browser-PTY agent session manager (Go backend, React frontend, worktree-per-task)
**Researched:** 2026-06-10
**Confidence:** HIGH

## Executive Summary

Kangent sits at the intersection of two well-trodden architectures: the "kanban of agents" model (vibe-kanban, SlayZone, claude-squad, Conductor) and the web terminal model (ttyd, gotty, Coder's reconnecting PTY). Every successful product in this space converges on the same four genre-defining features: worktree-per-task isolation, automatic session status detection, diff viewing, and a way to be told when an agent needs attention. Kangent's specific niche — a real, unmodified Claude Code CLI in a browser PTY, served from a single Go binary at localhost — is currently unoccupied: SlayZone proves the real-PTY model but is Electron; vibe-kanban is web-based but reimplements agent chat UIs (and is sunsetting).

The recommended approach is conservative and verified: Go 1.26 with stdlib ServeMux, `coder/websocket`, `creack/pty` (v1), `modernc.org/sqlite`, and shelling out to the system git CLI for worktrees (go-git cannot manage linked worktrees); React 19 + Vite + `@xterm/xterm` 6 + TanStack Query + dnd-kit on the frontend, embedded into the binary via `go:embed`. The single most important architectural rule, proven by all reference systems: **the PTY's lifetime is owned by a session manager, never by the WebSocket connection** — the browser is a detachable view. No off-the-shelf "persistent PTY session" library exists; the session manager (~200–400 lines: registry, ring buffer, fan-out, attach/detach) is the heart of the system and the highest-risk component, so it should be built and proven first against plain bash before Claude Code enters the picture.

The key risks are: (1) reattach/replay against Claude Code's alternate-screen TUI — raw ring-buffer replay alone produces corrupted screens; mitigate with a post-replay resize nudge and flag this for deeper phase research; (2) process lifecycle — naive kills orphan Claude's node/MCP children and block worktree removal; kill by process group and always reap with `cmd.Wait()`; (3) worktree safety — destructive cleanup of dirty worktrees is trust-destroying; dirty-check + explicit confirmation is non-negotiable; and (4) localhost security — an unguarded WebSocket here is a remote shell via cross-site WebSocket hijacking; Origin/Host validation and a per-instance token must land in the same phase that exposes the WS endpoint, not as hardening later.

## Key Findings

### Recommended Stack

Backend is deliberately minimal: Go stdlib routing (Go 1.22+ ServeMux handles method+path matching for ~15 REST endpoints), pure-Go SQLite for trivial single-binary builds, and zero framework lock-in. Frontend is the 2026 default React SPA toolchain. All versions verified against registries on 2026-06-10. Full details in `STACK.md`.

**Core technologies:**
- Go 1.26 + stdlib `net/http` ServeMux: backend + routing — zero deps, sufficient for the endpoint surface
- `github.com/coder/websocket` v1.8.14: terminal WS bridge — concurrent-write safe (gorilla panics in fan-out), context-aware
- `github.com/creack/pty` v1.1.24: PTY allocation — de-facto standard; import v1, the v2 module is orphaned
- `modernc.org/sqlite` v1.52.0: CGO-free SQLite — keeps `CGO_ENABLED=0` single-binary builds trivial
- `os/exec` + system git: worktree ops — go-git stable cannot create linked worktrees; parse `--porcelain` output only
- React 19 + Vite 8 + TypeScript: SPA, embedded via `go:embed` + `http.FileServerFS`
- `@xterm/xterm` 6.0.0 (+fit, +webgl addons): browser terminal — scoped packages only; canvas renderer is gone in 6.0
- TanStack Query 5 + dnd-kit: server state and kanban drag-and-drop — react-beautiful-dnd is dead
- Tailwind 4 + shadcn/ui: styling and primitives
- `goose` v3.27.1: embedded SQL migrations run at startup

### Expected Features

The genre has a clear convergent feature set; Kangent's differentiation is the combination (real PTY + web app), not novel features. Full landscape in `FEATURES.md`.

**Must have (table stakes):**
- Projects sidebar (repo-path validation) and kanban board with fixed 4 columns, drag-and-drop, task CRUD with markdown
- Worktree + branch auto-created per task; explicit confirmed cleanup at Done (keep branch, dirty-check)
- Real interactive PTY terminal (xterm.js): resize, scrollback, copy/paste — the product's reason to exist
- Server-side session persistence + reattach with replay buffer — the core value prop ("open, leave, reattach")
- Bash session tabs per task — near-free once PTY infra exists
- Status badge per card (working / idle / waiting / exited) via output-activity heuristic
- Waiting-for-input detection via Claude Code `Notification`/`Stop` hooks injected at spawn (+ BEL fallback)
- SQLite persistence of board + session metadata

**Should have (competitive, v1.x):**
- Browser notifications on needs-attention (cheap once detection exists)
- Read-only diff tab vs base branch (makes In Review meaningful)
- `claude --resume`/`--continue` recovery UI after server restart
- "Move to In Review?" suggestion on Stop hook (suggest, never auto-move)

**Defer (v2+):**
- MCP server for agent board access; multiple agent CLIs; split panes; task templates
- Anti-features to actively avoid: structured chat UI (loses plan mode/slash commands), automatic column transitions (trust erosion), auto worktree cleanup (vibe-kanban lost user work — issues #1571/#1764), git/PR automation, external tracker sync

### Architecture Approach

One Go process serves the REST API, per-terminal WebSockets, and the embedded SPA. Three state domains with three owners: workflow state in SQLite, code state in git/filesystem, live terminal state in process memory (ring buffers, never persisted to DB). Wire protocol is ttyd-style: binary WS frames with a 1-byte type prefix (input/output/resize/exit) — `@xterm/addon-attach` cannot be used; the replacement glue is ~30 lines. Reattach follows Coder's reconnecting-PTY pattern: replay ring buffer, then subscribe; force a TUI repaint via resize nudge after replay. One WS per terminal tab; no multiplexing. Full details and data model in `ARCHITECTURE.md`.

**Major components:**
1. Session manager (`internal/session/`) — owns all PTYs in a registry; spawn/kill/attach/detach, ring buffer, exit reaping, startup sweep; must not import api/ws
2. WS terminal bridge (`internal/ws/`) — deliberately thin: upgrade, framing, fan-out; the only consumer of PTY I/O
3. Git/worktree service (`internal/gitx/`) — worktree add/remove/prune via exec git; path conventions outside the repo tree
4. SQLite store — projects, tasks, session *metadata* only; embedded migrations
5. HTTP API — thin JSON handlers wiring store + services; serves embedded React build
6. React SPA — sidebar, board (dnd-kit), task detail with xterm tabs; TanStack Query for REST state

### Critical Pitfalls

Top 5 of 11 documented in `PITFALLS.md`:

1. **PTY lifetime tied to WebSocket** — one tab close kills an hour of agent work; session manager owns PTYs, WS handlers only attach/detach
2. **Alt-screen replay corruption** — raw ring-buffer replay of a TUI starts mid-escape-sequence; `term.reset()` + replay + debounced resize-jiggle redraw; flag for deeper research before building reattach UX
3. **Zombie/orphan processes** — kill the process *group* (child is session leader), always `cmd.Wait()` in a goroutine; test zero descendants after Stop
4. **Worktree safety** — ref-safe slugs + task-ID suffix for branches; worktrees outside the repo tree; cleanup = stop sessions → porcelain dirty check → explicit confirmation → `git worktree remove`; never `rm -rf`
5. **Localhost is not a security boundary** — browsers don't enforce same-origin on WebSockets; strict Origin + Host validation and a per-instance token from the first phase that exposes the WS endpoint

Also critical: binary frames end-to-end (UTF-8 splitting corrupts output), flow-control watermarks (pause/resume) in the WS protocol from day one, explicit child env construction (TERM/PATH/HOME), startup reconciliation of `running` session rows, and SQLite WAL + busy_timeout + MaxOpenConns(1).

## Implications for Roadmap

Based on research, suggested phase structure (mirrors the build order validated in ARCHITECTURE.md, with pitfall prevention assigned per phase):

### Phase 1: Foundation — single binary, store, projects
**Rationale:** Proves the deployment model (Vite build embedded in Go binary) end to end and lands SQLite configuration before anything writes to it.
**Delivers:** Go server serving embedded SPA; SQLite open + goose migrations; projects CRUD + sidebar with repo-path validation.
**Addresses:** Projects sidebar, SQLite persistence (FEATURES table stakes).
**Avoids:** Pitfall 9 (SQLite WAL/busy_timeout/MaxOpenConns — 10 lines now, a debugging week later); Host-header validation groundwork.

### Phase 2: Terminal vertical slice (de-risk spike)
**Rationale:** The session manager + WS bridge + xterm pane is the highest-risk subsystem and has zero dependency on tasks or git — prove it against plain `bash` in a fixed directory. Everything else in the app is conventional CRUD.
**Delivers:** Session manager (spawn, ring buffer, fan-out, attach/detach, exit reaping, process-group kill), binary framed WS protocol with resize + flow control, xterm.js pane with fit/WebGL/resize. Test: open, type, close tab, reopen, see replayed scrollback.
**Uses:** creack/pty, coder/websocket, @xterm/xterm 6 + fit + webgl.
**Implements:** Session manager + WS bridge (the architectural core).
**Avoids:** Pitfalls 1 (zombies), 2 (UTF-8/binary frames), 4 (flow control), 5 (resize), 6 (env construction), 10 (Origin checks + token — MUST land here, this phase exposes the WS endpoint).

### Phase 3: Tasks + kanban board
**Rationale:** Independent of Phase 2 (could parallelize); pure CRUD + UI with well-documented patterns.
**Delivers:** Tasks CRUD with markdown, fixed 4-column board, dnd-kit drag-and-drop with optimistic updates, task detail view shell.
**Addresses:** Kanban board, task CRUD (table stakes).

### Phase 4: Git worktree integration
**Rationale:** Depends on tasks existing; isolates all git semantics in one service before sessions need worktree cwds.
**Delivers:** Worktree + branch on task create (slug + ID suffix, sibling-dir placement); cleanup offer on Done with dirty-check confirmation, branch kept; `git worktree prune` self-healing.
**Uses:** os/exec + git CLI, `--porcelain` parsing.
**Avoids:** Pitfall 7 (dirty-worktree confirmation is a non-negotiable phase success criterion; submodule detection).

### Phase 5: Claude Code agent sessions
**Rationale:** Depends on Phases 2+4; this is where the generic PTY engine meets Claude Code's specific TUI behavior — treat as a dedicated integration phase, not "just another command."
**Delivers:** Start button spawning `claude` with cwd = worktree and hook injection (`Notification`/`Stop` POSTing to localhost); bash tabs; one-agent-per-task rule; status badges (working/idle/waiting/exited) on cards; reattach UX with alt-screen handling.
**Addresses:** Claude session, status detection, waiting-for-input detection (the feature that lets users walk away).
**Avoids:** Pitfall 3 (alt-screen replay), Pitfall 11 (permission prompts, bracketed paste, mouse mode, ~2000-line internal scrollback).

### Phase 6: Restart semantics + polish
**Rationale:** Completes the lifecycle; the startup sweep and `sessions.status` model must already exist from Phase 2/5 — this phase adds the recovery UX.
**Delivers:** Dead-session UI with `claude --continue`/`--resume` resume in the worktree cwd; worktree cleanup edge cases; optional SSE status events; browser notifications; read-only diff tab (v1.x items as capacity allows).
**Avoids:** Pitfall 8 (DB claims running after restart; never auto-resume one Claude session into two PTYs).

### Phase Ordering Rationale

- The terminal vertical slice is pulled forward because it is the only genuinely hard subsystem and has no upstream dependencies — de-risk first, then everything else is conventional.
- Phases 2 and 3 are independent and can swap or parallelize; 4 depends on 3; 5 depends on 2+4; 6 depends on 5.
- Hook injection at spawn (Phase 5) cannot be retrofitted cheaply — the Start-button spawn design must accommodate per-session settings overlays from the start.
- Security (Origin/Host/token) is bound to Phase 2, not deferred: the WS endpoint is a shell.
- Restart reconciliation data model (`sessions.status`, startup sweep) is baked in at Phase 2/5; only the resume *UX* is deferred to Phase 6.

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 5 (Claude Code integration):** alt-screen replay strategy (headless VT emulator vs resize-jiggle), exact Claude Code hook payloads/config for `Notification`/`Stop`, `--session-id`/`--resume`/`--continue` flag semantics on the installed version, `CLAUDE_CODE_DISABLE_ALTERNATE_SCREEN` behavior — all version-dependent and flagged MEDIUM confidence in research.
- **Phase 6 (restart recovery):** `--continue` cwd-keyed resume semantics (MEDIUM confidence; verify against current docs).

Phases with standard patterns (skip research-phase):
- **Phase 1 (foundation):** go:embed SPA + SQLite setup is thoroughly documented.
- **Phase 3 (kanban):** dnd-kit multi-container sortable is the documented canonical example.
- **Phase 4 (worktrees):** git worktree CLI semantics fully covered by PITFALLS.md + official docs.
- **Phase 2 (terminal slice):** patterns are HIGH-confidence (ttyd/gotty/Coder source verified), though it remains the highest *implementation* risk — research is done, execution care is what's needed.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All versions verified against npm/pkg.go.dev/GitHub releases on 2026-06-10; reference implementations confirmed active |
| Features | MEDIUM-HIGH | Product docs + primary GitHub issues; some competitor UX details from marketing pages are LOW |
| Architecture | HIGH | Terminal bridge/persistence patterns verified against ttyd, gotty, and Coder source code |
| Pitfalls | HIGH | PTY/xterm.js/worktree/SQLite verified against official docs and issues; Claude Code internals MEDIUM (fast-moving) |

**Overall confidence:** HIGH

### Gaps to Address

- **Claude Code hook payloads and flag surface** (`Notification`/`Stop` hooks, `--session-id`, `--resume`, `--continue`, alt-screen env var): version-dependent; verify against the installed `claude --version` during Phase 5 planning, and gate features on detected version at runtime.
- **Alt-screen replay strategy decision** (headless VT state vs ring buffer + resize-jiggle): the hardest design decision in the project; must be made in the session-engine phase *before* building reattach UX — shipping the wrong replay architecture is a HIGH-cost rework.
- **go-git v6 worktree status:** not needed (CLI route is safe regardless), but noted if a pure-Go path is ever revisited.
- **Waiting-detection fidelity:** hooks are the primary signal, BEL is the fallback; real-world reliability needs validation with actual Claude Code sessions early in Phase 5.

## Sources

### Primary (HIGH confidence)
- pkg.go.dev / GitHub releases / npm registry (2026-06-10) — all stack versions: coder/websocket v1.8.14, creack/pty v1.1.24, modernc.org/sqlite v1.52.0, xterm.js 6.0, Vite 8, goose v3.27.1
- ttyd `src/protocol.c`, gotty `webtty/webtty.go`, Coder `agent/reconnectingpty/buffered.go` — wire protocol and reconnecting-PTY patterns
- git-scm.com/docs/git-worktree; go-git releases + issue #1956 — worktree semantics; go-git linked-worktree limitations
- xterm.js flow-control guide + FitAddon issues (#1416, #5320, #3584) — watermark protocol, hidden-container crashes
- anthropics/claude-code issues #49086, #42670, #38283 + sessions docs — resize redraw bug, alt-screen scrollback, resume semantics
- vibe-kanban issues #1571, #765, #1764, discussion #2335 — worktree auto-cleanup failure modes
- PortSwigger CSWSH explainer; GitHub blog on localhost CORS/DNS rebinding; webpack-dev-server and Vite CVE advisories — localhost attack surface

### Secondary (MEDIUM confidence)
- vibe-kanban / SlayZone / claude-squad / Conductor / Omnara product pages and repos — competitor feature analysis
- DeepWiki + community analyses of vibe-kanban architecture — workflow/code state split
- Go+SQLite community practices (WAL, MaxOpenConns) — concurrency configuration

### Tertiary (LOW confidence)
- Conductor marketing pages — feature claims, needs validation
- Claude Code hook payload details from training knowledge — verify exact config during Phase 5 research

---
*Research completed: 2026-06-10*
*Ready for roadmap: yes*
