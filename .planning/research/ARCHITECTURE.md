# Architecture Research

**Domain:** Local-only, single-user web app orchestrating Claude Code agent sessions (Go backend, React frontend, PTY-over-WebSocket terminals, git worktree per task, SQLite)
**Researched:** 2026-06-10
**Confidence:** HIGH (terminal bridge / persistence patterns verified against ttyd, gotty, and Coder source; vibe-kanban structure verified via multiple sources)

## Standard Architecture

This product is the intersection of two well-trodden architectures:

1. **Kanban-of-agents** (vibe-kanban): REST API + SQLite for workflow state, git worktrees for code state, spawned child processes for agents, streaming channel for live updates.
2. **Web terminal** (ttyd/gotty/Coder): long-lived server owns PTYs; a thin WebSocket bridge with single-byte message-type framing connects each PTY to xterm.js; a ring buffer provides scrollback replay on reattach.

The single most important architectural rule, proven by all reference systems: **the PTY's lifetime is owned by the session manager, never by the WebSocket connection.** The browser is a detachable view.

### System Overview

```
┌────────────────────────────── Browser (React SPA) ──────────────────────────────┐
│  ┌──────────────┐  ┌─────────────────┐  ┌──────────────────────────────────┐    │
│  │ Project      │  │ Kanban Board    │  │ Task Detail View                 │    │
│  │ Sidebar      │  │ (4 fixed cols)  │  │  ┌─────────┐ ┌──────┐ ┌──────┐   │    │
│  └──────┬───────┘  └────────┬────────┘  │  │ Agent   │ │ bash │ │ bash │   │    │
│         │                   │           │  │ term    │ │ term │ │ term │   │    │
│         │   TanStack Query  │           │  └────┬────┘ └──┬───┘ └──┬───┘   │    │
│         │   (REST, JSON)    │           │   xterm.js per tab               │    │
└─────────┼───────────────────┼───────────┴───────┼──────────┼──────┼────────┘    │
          │                   │                   │ WS (binary, 1 per terminal)
══════════╪═══════════════════╪═══════════════════╪══════════╪══════╪═════ localhost
┌─────────▼───────────────────▼─────────┐ ┌───────▼──────────▼──────▼────────────┐
│  HTTP API (REST)                      │ │  WebSocket Terminal Bridge           │
│  /api/projects /api/tasks             │ │  /api/sessions/{id}/ws               │
│  /api/tasks/{id}/sessions             │ │  framing: input/output/resize        │
└───────┬───────────────┬───────────────┘ └──────────────────┬───────────────────┘
        │               │                                    │ attach/detach
┌───────▼───────┐ ┌─────▼──────────┐ ┌──────────────────────▼───────────────────┐
│ Git/Worktree  │ │ SQLite Store   │ │ Session Manager (in-memory registry)     │
│ Service       │ │ projects/tasks │ │  per session:                            │
│ (exec git CLI)│ │ /sessions meta │ │   PTY master ── ring buffer (scrollback) │
└───────┬───────┘ └────────────────┘ │            └── activeConns fan-out       │
        │                            └──────────────────────┬───────────────────┘
        │  git worktree add/remove                          │ spawn in worktree cwd
┌───────▼────────────────────────────────────────────────── ▼ ──────────────────┐
│ Host filesystem: project repos + .../worktrees/<task>/   claude / bash (PTYs) │
└────────────────────────────────────────────────────────────────────────────────┘
```

One Go process serves everything: the REST API, the WebSocket endpoints, and the embedded React build (`go:embed`). Single binary, matching the deployment constraint.

### Component Responsibilities

| Component | Responsibility | Typical Implementation |
|-----------|----------------|------------------------|
| HTTP API | CRUD for projects/tasks; session start/stop/list endpoints; serves embedded SPA | Go 1.22+ `net/http` mux (or chi); JSON handlers calling store + services |
| SQLite store | Durable workflow state only: projects, tasks, session *metadata*. Never terminal output | `modernc.org/sqlite` (CGO-free) or `mattn/go-sqlite3`; embedded migrations |
| Git/worktree service | `worktree add` + branch on task create; `worktree remove` on cleanup; path conventions | Shell out to `git` CLI via `os/exec` (see Anti-Patterns — don't use go-git for this) |
| Session manager | Owns all live PTYs. Registry `sessionID → *Session`. Spawn, kill, detach-safe lifecycle, exit reaping, startup sweep of stale DB rows | `creack/pty` + goroutine per session pumping PTY→{ring buffer, active conns} (Coder's pattern) |
| Ring buffer (per session) | Fixed-size scrollback of raw output bytes; cloned and replayed to every newly attached connection | `armon/circbuf` (what Coder uses, 64 KiB default; 256 KiB–1 MiB is fine locally) |
| WS terminal bridge | Upgrade, auth-free (localhost), framing (input/output/resize), fan-out writes, backpressure | `coder/websocket` or `gorilla/websocket`; binary frames; ttyd-style 1-byte type prefix |
| Event channel (optional) | Push session status changes (started/exited) and task moves to the board | SSE endpoint or one lightweight WS; can be deferred — single user can refetch on mutation |
| React SPA | Sidebar, board, task detail with terminal tabs | Vite + React; TanStack Query for REST state; xterm.js (`@xterm/xterm` + fit addon) per tab |

## Recommended Project Structure

```
kangent/
├── cmd/kangent/
│   └── main.go             # flag parsing, wiring, embedded FS, ListenAndServe
├── internal/
│   ├── api/                # HTTP handlers (projects.go, tasks.go, sessions.go)
│   ├── store/              # SQLite open/migrate + typed queries
│   │   └── migrations/     # embedded .sql files
│   ├── gitx/               # worktree/branch ops via exec git; path layout
│   ├── session/            # Manager, Session, ring buffer, PTY spawn/kill,
│   │   │                   #   attach/detach, exit watcher, startup sweep
│   │   └── proto.go        # WS message-type bytes shared with bridge
│   ├── ws/                 # WebSocket handler: upgrade, read/write pumps
│   └── events/             # (optional) SSE hub for status updates
├── web/                    # React app (Vite)
│   ├── src/
│   │   ├── api/            # fetch client + TanStack Query hooks
│   │   ├── components/
│   │   │   ├── sidebar/    # project list, create project
│   │   │   ├── board/      # columns, cards, drag-and-drop
│   │   │   ├── task/       # detail view, Start button, tab bar
│   │   │   └── terminal/   # XtermPane: xterm.js + WS protocol hook
│   │   └── App.tsx
│   └── dist/               # build output, embedded via go:embed
└── Makefile                # build web → embed → go build (single binary)
```

### Structure Rationale

- **`internal/session/` is the heart of the system** and must not import `api/` or `ws/` — it exposes `Attach(sessionID) (replay []byte, conn io.ReadWriteCloser, err)`-style methods that the bridge consumes. This keeps the "PTY outlives the socket" invariant enforceable in one place.
- **`internal/ws/` is deliberately thin**: framing translation only. gotty structures this identically (`webtty` package bridges a "master" — the WebSocket — and a "slave" — the PTY) and it stays under a few hundred lines.
- **`gitx/` isolated** so worktree path conventions live in one file (e.g., worktrees under `<repo>/.kangent/worktrees/<task-slug>/` or a sibling dir — sibling dir avoids polluting the repo and accidental commits).
- **Store holds metadata only.** Vibe-kanban's framing applies directly: *code state is managed by git; workflow state is managed by SQLite; live terminal state lives in process memory.* Three state domains, three owners.

## Architectural Patterns

### Pattern 1: ttyd-style framed WebSocket protocol (binary frames, 1-byte type prefix)

**What:** Every WS message is a binary frame whose first byte is a command, rest is payload. Verified protocols:
- **ttyd** — client→server: `'0'` INPUT, `'1'` RESIZE (JSON `{"columns":N,"rows":N}`), `'2'` PAUSE, `'3'` RESUME, `'{'` JSON init; server→client: `'0'` OUTPUT, `'1'` SET_WINDOW_TITLE, `'2'` SET_PREFERENCES.
- **gotty** — same shape (Input/Ping/ResizeTerminal vs Output/Pong/SetWindowTitle/...), but base64-encodes output by default (legacy of text frames — don't copy that).

**Recommended for kangent:** client→server `'0'+bytes` input, `'1'+JSON` resize; server→client `'0'+bytes` output, `'x'+JSON` exit notice (code). That's the whole protocol.

**When to use:** Always for this app. Resize *must* travel on the same socket, which is why a typed protocol beats raw piping.

**Trade-offs:** You cannot use `@xterm/addon-attach` (it pipes raw socket data with no framing and has no resize support). The replacement is ~30 lines of glue — universally what real projects do:

```typescript
// terminal/useTerminalSocket.ts (essence)
ws.binaryType = "arraybuffer";
ws.onmessage = (ev) => {
  const data = new Uint8Array(ev.data);
  if (data[0] === 0x30 /* '0' */) term.write(data.subarray(1));   // output
};
term.onData((s) => ws.send(concat([0x30], encoder.encode(s))));   // input
const sendResize = () =>
  ws.send(concat([0x31], encoder.encode(JSON.stringify(
    { cols: term.cols, rows: term.rows }))));
fitAddon.fit(); sendResize();           // on mount and on ResizeObserver
```

Go side: on resize message call `pty.Setsize(f, &pty.Winsize{Rows, Cols})` (kernel delivers SIGWINCH to the child — Claude Code's TUI redraws itself).

### Pattern 2: Reconnecting PTY with ring-buffer replay (Coder's pattern)

**What:** The session manager, not the WS handler, owns the PTY. One goroutine reads PTY output in chunks and multiplexes it to (a) a fixed-size circular buffer and (b) every connection in an `activeConns` map. On attach: clone the ring buffer, write it to the new connection first, then add the connection to the map. On socket close: just remove from the map — the process keeps running.

Verified against Coder's `agent/reconnectingpty/buffered.go`: `armon/circbuf` (64 KiB default), 1024-byte read chunks, replay-then-subscribe on attach, `activeConns` map keyed by connection ID.

**When to use:** This is the core requirement ("sessions keep running when the tab closes; reopening reattaches"). Non-negotiable.

**Trade-offs:** Replaying raw bytes into a *fresh* xterm instance can leave a TUI app (like Claude Code) visually stale or mid-escape-sequence. Two standard mitigations: (1) make the buffer comfortably larger than one screen of TUI redraw (256 KiB+ locally costs nothing); (2) after replay, force a repaint by sending a resize — the kernel's SIGWINCH makes full-screen TUIs redraw cleanly. Do both.

```go
// session/session.go (essence)
type Session struct {
    ptmx    *os.File              // PTY master (creack/pty)
    cmd     *exec.Cmd             // claude or bash, Dir = worktree path
    ring    *circbuf.Buffer       // scrollback
    mu      sync.Mutex
    conns   map[string]net.Conn   // active WS-backed conns
}
// pump goroutine: read ptmx → ring.Write(chunk) + write to each conn
// Attach(): mu.Lock; replay := ring.Bytes(); conns[id] = c; mu.Unlock; return replay
// Detach(): delete(conns, id)  // PTY untouched
```

### Pattern 3: Three state domains with explicit restart semantics

**What:** Be explicit about what survives what:

| State | Owner | Survives tab close | Survives server restart |
|-------|-------|--------------------|-------------------------|
| Projects, tasks, board, session metadata | SQLite | yes | yes |
| Branches, worktrees, uncommitted files | git / filesystem | yes | yes |
| Live processes (claude, bash), PTYs, scrollback ring | server process memory | **yes** (the point of the design) | **no** |

**Restart protocol:** PTY children are killed when the server dies (master side of the PTY closes → SIGHUP). On boot, the session manager runs a **startup sweep**: every DB session row in status `running` is marked `dead`. The UI shows dead agent sessions with a "Resume" affordance that starts a fresh PTY running `claude --continue` with cwd = the task's worktree. Because each task has a unique worktree directory and Claude Code keys conversation history by cwd, `--continue` resumes that task's most recent conversation without the app ever having to capture a session ID from the PTY stream. (Capturing `--resume <id>` IDs from terminal output is brittle; cwd-keyed `--continue` is the clean recovery path. Confidence: MEDIUM on `--continue` cwd semantics — verify against current Claude Code docs during the relevant phase.)

**When to use:** Bake into the data model from day one (`sessions.status`, startup sweep) — retrofitting restart semantics is painful.

### Pattern 4: Worktree-per-task via the git CLI

**What:** On task create: `git -C <repo> worktree add -b kangent/<task-slug> <worktrees-dir>/<task-slug>` (branch name + worktree path stored on the task row). On "mark done → clean up": `git -C <repo> worktree remove <path>` (add `--force` only with explicit user confirmation if the tree is dirty); branch is kept per requirements. This is exactly vibe-kanban's isolation model (one worktree per task attempt, automatic cleanup, `DISABLE_WORKTREE_CLEANUP` escape hatch for debugging — copy that flag idea).

**Trade-offs:** Shelling out means parsing exit codes/stderr rather than typed errors — acceptable, and far more reliable than reimplementing worktree semantics (see Anti-Patterns).

## Data Model

```sql
projects (
  id          INTEGER PRIMARY KEY,
  name        TEXT NOT NULL,
  repo_path   TEXT NOT NULL UNIQUE,     -- absolute path to checkout
  created_at  TEXT NOT NULL
);

tasks (
  id            INTEGER PRIMARY KEY,
  project_id    INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  title         TEXT NOT NULL,
  description   TEXT NOT NULL DEFAULT '',          -- markdown
  status        TEXT NOT NULL DEFAULT 'todo',      -- todo|in_progress|in_review|done
  position      REAL NOT NULL,                     -- ordering within column
  branch_name   TEXT,                              -- kangent/<slug>
  worktree_path TEXT,                              -- NULL after cleanup
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);

sessions (                                         -- metadata only; PTYs live in memory
  id          INTEGER PRIMARY KEY,
  task_id     INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  kind        TEXT NOT NULL,                       -- 'agent' | 'bash'
  status      TEXT NOT NULL,                       -- running|exited|dead (dead = lost to restart)
  pid         INTEGER,
  exit_code   INTEGER,
  started_at  TEXT NOT NULL,
  ended_at    TEXT
);
```

Cardinality: task 1..N sessions — at most one `running` session of kind `agent` per task (enforce in the session manager, not just the DB), plus any number of `bash` sessions. Worktree/branch metadata lives on the task (not the session) because the worktree's lifetime is the task's, not any one process's.

## Data Flow

### Request Flow (CRUD + side effects)

```
Create task:
  Board UI → POST /api/tasks → store.InsertTask (tx)
                             → gitx.CreateWorktree(repo, branch, path)
                             → update task row with branch/worktree → 201
                             → TanStack Query invalidates board

Start agent session:
  Task view → POST /api/tasks/{id}/sessions {kind:"agent"}
            → session.Manager.Start(taskID, kind, cwd=worktree)
            → spawns `claude` in PTY, inserts sessions row → returns {sessionID}
  Task view → opens WS /api/sessions/{sessionID}/ws
            → bridge: Attach → replay ring buffer → live binary stream
```

### Terminal Flow (per open terminal tab)

```
keystroke → xterm.onData → WS frame '0'+bytes → bridge → ptmx.Write
ptmx.Read → pump goroutine → ring buffer  AND  every active conn → WS '0'+bytes → term.write
container resize → fit addon → WS '1'+{cols,rows} → pty.Setsize → SIGWINCH → TUI redraws
process exit → pump sees EOF → manager marks sessions row exited → WS 'x'+{code} → UI badge
tab close → WS close → Detach (conn removed; PTY and ring keep running)
```

### Key Data Flows

1. **REST for everything durable** (projects, tasks, session start/stop/list). Plain JSON; TanStack Query with invalidate-on-mutate. Single user at localhost means optimistic concurrency is unnecessary.
2. **One WebSocket per terminal tab.** N tabs open = N sockets. Do not multiplex (see Anti-Patterns).
3. **Board/status push is optional.** Vibe-kanban streams DB-change events over WS because multiple agents mutate state autonomously; in kangent the only async state change is *session exit* (and the exit already arrives on that session's own WS). A small SSE endpoint for session-status events is a nice phase-late addition, not a foundation.

## Suggested Build Order

Dependencies drive this order; the terminal bridge is pulled early because it is the highest-risk component and has no hard dependency on tasks/git (it can be proven against a plain `bash` in any directory).

1. **Skeleton + foundation** — Go binary serving embedded Vite build; SQLite open + migrations; projects CRUD + sidebar. *Proves: single-binary deployment model end to end.*
2. **Terminal vertical slice (de-risk)** — session manager (spawn bash PTY in a fixed dir), ring buffer, WS bridge with framing, xterm.js pane with fit + resize. Test: open, type, close tab, reopen, see replayed scrollback. *Everything else is conventional CRUD; this is the part worth spiking first.*
3. **Tasks + kanban board** — tasks CRUD, fixed columns, drag between columns, detail view shell. No git yet.
4. **Git/worktree integration** — worktree+branch on create; wire cleanup offer on done; task row gains branch/worktree metadata.
5. **Agent sessions + tabs** — Start button spawns `claude` in the worktree; bash tabs; one-agent-per-task rule; exit handling; reattach UX.
6. **Restart semantics + polish** — startup sweep, dead-session UI with `claude --continue` resume, worktree cleanup edge cases (dirty tree confirmation), SSE status events if wanted.

Phases 2 and 3 are independent and could swap or parallelize; 4 depends on 3; 5 depends on 2+4; 6 depends on 5.

## Scaling Considerations

| Scale | Architecture Adjustments |
|-------|--------------------------|
| 1 user, ≤10 live sessions (the design point) | Everything above. ~2 goroutines per session, KBs of RAM each — no tuning needed |
| 1 user, dozens of sessions | Add an idle-timeout reaper (Coder's heartbeat/timeout pattern) so forgotten bash sessions don't accumulate |
| Multi-user / remote (explicitly out of scope) | Would require auth, origin checks beyond localhost, per-user PTY isolation, and flow control (ttyd's PAUSE/RESUME) — none of which should leak into v1 |

### Scaling Priorities

1. **First real bottleneck: slow/stalled WS writer.** A blocked connection write in the fan-out loop stalls output for all viewers of that session. Fix cheaply: per-conn buffered write channel, drop the conn if it falls behind (irrelevant-in-practice at localhost, but it keeps the pump loop non-blocking and is ~20 lines).
2. **Second: ring buffer sizing vs TUI replay quality.** If reattached Claude Code screens look mangled, increase the buffer and always send the post-replay resize nudge before debugging anything else.

## Anti-Patterns

### Anti-Pattern 1: Tying PTY lifetime to the WebSocket connection

**What people do:** Spawn the process in the WS handler; `defer cmd.Process.Kill()` on socket close (every basic xterm.js tutorial does this — ttyd and gotty also behave this way by design).
**Why it's wrong:** It silently violates the product's core promise. One accidental tab close kills an hour of agent work.
**Do this instead:** Session manager owns PTYs in a registry; WS handlers only `Attach`/`Detach` (Coder's reconnectingpty model, Pattern 2).

### Anti-Pattern 2: Using go-git (or any library) for worktree operations

**What people do:** Reach for `go-git` to keep things "pure Go."
**Why it's wrong:** go-git does not implement linked-worktree management (`git worktree add/remove`); partial reimplementations corrupt `.git/worktrees` metadata. (Confidence: MEDIUM — verify go-git's current state in the git phase, but the CLI route is safe regardless.)
**Do this instead:** `os/exec` the real `git` CLI. It's already a host requirement (the repos exist), and vibe-kanban's equivalent service is similarly a thin wrapper over real git operations.

### Anti-Pattern 3: Text frames + base64 output (gotty 1.x legacy)

**What people do:** Send terminal output as base64 inside text/JSON WS frames.
**Why it's wrong:** 33% size overhead, encode/decode hops, and UTF-8 boundary bugs with partial escape sequences. gotty only did this for pre-binary-frame compatibility.
**Do this instead:** Binary frames with a 1-byte type prefix (ttyd's protocol, Pattern 1).

### Anti-Pattern 4: Multiplexing all terminals over one WebSocket

**What people do:** One "channel" socket carrying every terminal plus board events, with session-ID routing in every frame.
**Why it's wrong:** Reinvents framing, complicates backpressure (one slow terminal stalls all), and buys nothing at localhost where sockets are free.
**Do this instead:** One WS per terminal tab; URL carries the session ID. Optional separate SSE stream for status events.

### Anti-Pattern 5: Persisting terminal output to SQLite

**What people do:** Log every output chunk to the DB so scrollback "survives restarts."
**Why it's wrong:** High write volume for low value — the processes themselves don't survive restarts, so a byte-perfect replay of a dead TUI is mostly garbage on screen. Claude Code already persists conversation history; `--continue` is the durable record.
**Do this instead:** In-memory ring buffer for live reattach; DB stores session metadata only; `claude --continue` for post-restart recovery.

### Anti-Pattern 6: Replacing the PTY with the Agent SDK / `--output-format stream-json`

**What people do:** Parse Claude Code's structured output and build a chat UI.
**Why it's wrong (for this project):** Loses plan mode, permission prompts, slash commands, and every future CLI feature — exactly what PROJECT.md decided against. Vibe-kanban takes the structured-output route (its `StandardCodingAgentExecutor` normalizes agent streams) because it supports 10+ agents and a review-centric UI; kangent's value is the *unmodified* interactive CLI, which mandates the ttyd-style PTY architecture instead.
**Do this instead:** Real `claude` in a real PTY; the browser renders bytes.

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| `git` CLI | `os/exec`, `-C <repo>` flag, parse exit code + stderr | Validate repo on project create (`git rev-parse --git-dir`); worktree dirs outside the repo tree |
| `claude` CLI | Spawn in PTY via `creack/pty`, `cmd.Dir = worktree` | Check presence on startup (`exec.LookPath`); pass through user env (auth lives in `~/.claude`) |
| `bash`/`$SHELL` | Same PTY path, `kind=bash` | Use `$SHELL` fallback bash; set `TERM=xterm-256color` |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| api ↔ store | Direct function calls, typed query layer | Handlers stay thin; transactions for task-create + worktree metadata |
| api ↔ gitx | Direct calls; worktree create/remove invoked from task handlers | Failure on `worktree add` must roll back the task insert (or mark task degraded) |
| api ↔ session manager | Direct calls: Start/Stop/List | API never touches PTY file handles |
| ws ↔ session manager | `Attach(id)` returns replay bytes + read/write handles; `Detach(id, connID)` | The only consumer of PTY I/O; framing lives entirely in `ws/` |
| session manager ↔ store | Manager writes session lifecycle rows (started/exited/dead) | One-way: store never reaches into the manager |
| frontend ↔ backend | REST (TanStack Query) + per-terminal WS + optional SSE | No state shared between the WS protocol and REST payloads except session IDs |

## Sources

- vibe-kanban repository and architecture — [github.com/BloopAI/vibe-kanban](https://github.com/BloopAI/vibe-kanban), [DeepWiki: BloopAI/vibe-kanban](https://deepwiki.com/BloopAI/vibe-kanban) (Axum + SQLx/SQLite, EventService WS streaming, executor processes, worktree lifecycle) — MEDIUM-HIGH
- vibe-kanban analyses — [virtuslab.com/blog/ai/vibe-kanban](https://virtuslab.com/blog/ai/vibe-kanban), [starlog.is article](https://starlog.is/articles/developer-tools/bloopai-vibe-kanban/) (dual-process Rust/React, SQLite-for-workflow/git-for-code split) — MEDIUM
- ttyd protocol — [github.com/tsl0922/ttyd](https://github.com/tsl0922/ttyd) `src/protocol.c` (1-byte prefixes, JSON resize, PAUSE/RESUME flow control) — HIGH
- gotty webtty bridge — [gotty webtty/webtty.go](https://github.com/sorenisanerd/gotty/blob/master/webtty/webtty.go) (master/slave bridge abstraction, message bytes, base64 legacy) — HIGH
- Coder reconnecting PTY — [coder/coder agent/reconnectingpty/buffered.go](https://github.com/coder/coder/blob/main/agent/reconnectingpty/buffered.go) (armon/circbuf 64 KiB, replay-then-subscribe, activeConns fan-out, timeout lifecycle) — HIGH
- xterm.js attach addon limitations — [xtermjs/xterm.js addons/addon-attach](https://github.com/xtermjs/xterm.js/tree/master/addons/addon-attach), [xterm-addon-attach (npm)](https://www.npmjs.com/package/xterm-addon-attach) — HIGH
- Claude Code `--continue` cwd-keyed resume semantics — training data — MEDIUM (flag for verification in the agent-session phase)

---
*Architecture research for: kangent — local agent-session kanban (Go + React + PTY/WS)*
*Researched: 2026-06-10*
