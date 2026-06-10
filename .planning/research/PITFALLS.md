# Pitfalls Research

**Domain:** Local web app running Claude Code CLI in server-side PTYs (Go backend, xterm.js frontend, git worktree per task, SQLite)
**Researched:** 2026-06-10
**Confidence:** HIGH (PTY/xterm.js/worktree/SQLite verified against official docs and GitHub issues), MEDIUM (some Claude Code internals are version-dependent and move fast)

## Critical Pitfalls

### Pitfall 1: Zombie and orphaned processes from naive PTY lifecycle

**What goes wrong:**
Killing the Claude Code process (or a bash tab) leaves children behind. `cmd.Process.Kill()` only signals the direct child — bash spawns subprocesses (and Claude Code spawns node, MCP servers, tool subprocesses) that get reparented to PID 1 and keep running, holding open file handles in worktrees you're about to delete. Separately, never calling `cmd.Wait()` after the process exits leaves zombies that accumulate over the app's lifetime.

**Why it happens:**
Go's `os/exec` kills by PID, not by process group. PTY tutorials show `pty.Start()` but rarely show teardown.

**How to avoid:**
- Set `cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}` (creack/pty needs the child to be the session leader with the PTY as controlling terminal — `pty.Start` does this for you).
- To terminate: signal the *process group* with `syscall.Kill(-pgid, syscall.SIGTERM)`, wait with timeout, then SIGKILL the group. Because the child is a session leader, its PID is the PGID.
- Always run a goroutine that calls `cmd.Wait()` and reaps exit status; this is also your "session died" event source for updating the DB and notifying the browser.
- Prefer graceful first: closing the PTY master sends SIGHUP to the foreground process group — many TUIs (including Claude Code) shut down cleanly on SIGHUP, which lets it persist session state for `--resume`.

**Warning signs:**
`ps` shows `<defunct>` entries or stray `node`/`claude` processes after closing tasks; `git worktree remove` fails with "device or resource busy"; server FD count climbs over time.

**Phase to address:**
PTY session engine phase — make process-group teardown + `Wait()` reaping part of the very first spawn implementation, with a test that asserts zero surviving descendants after Stop.

---

### Pitfall 2: UTF-8 and escape-sequence splitting in the PTY → WebSocket → xterm.js pipeline

**What goes wrong:**
Terminal output renders mojibake (`�`) or glitches intermittently, especially with Claude Code's heavy use of box-drawing characters, spinners, and emoji. A `Read()` from the PTY master returns an arbitrary byte slice that can cut a multi-byte UTF-8 character — or an ANSI escape sequence — in half. If the server decodes chunks to Go strings, or sends WebSocket *text* frames, the split bytes become invalid and are replaced or dropped.

**Why it happens:**
PTY reads are byte streams with no message boundaries; developers treat each read as a complete string.

**How to avoid:**
- Treat PTY output as opaque bytes end to end: read `[]byte`, send **binary** WebSocket frames, feed `Uint8Array` directly to `term.write()` — xterm.js accepts `Uint8Array` and its parser handles partial UTF-8 and partial escape sequences across writes correctly.
- Never `string(buf)`-convert, never split/merge chunks on the server based on string operations, never log-then-forward through a string path.
- Same in reverse: keystrokes from xterm.js (`onData`) are strings; send them as-is and write raw to the PTY without transformation.

**Warning signs:**
Occasional `�` characters; spinners or borders corrupt only under fast output; bugs that disappear when output is slow.

**Phase to address:**
PTY session engine + terminal UI phase — define the wire protocol as binary-frames-for-output from day one; retrofitting is a protocol break.

---

### Pitfall 3: Reattach/replay strategy that ignores the alternate screen buffer

**What goes wrong:**
Claude Code is a full-screen TUI on the alternate screen buffer. Two naive replay strategies both fail:
1. **Full-stream replay** (store every byte since session start, replay on reattach): replays hours of animation frames and redraws — multi-MB replays, seconds of flicker, and xterm.js choking (it discards beyond a 50MB write buffer).
2. **Raw ring buffer** (last N KB): almost certainly starts mid-escape-sequence and mid-frame, producing a corrupted screen; and if the buffer boundary falls after the alt-screen-enter sequence was evicted, xterm.js never enters the alternate screen, so rendering is garbage.

**Why it happens:**
Replay strategies are designed against line-oriented output (build logs) and then meet a TUI.

**How to avoid:**
Pick one of two proven approaches:
- **Server-side terminal state (recommended):** run a headless VT100 emulator in Go (e.g., `hinshun/vt10x` or similar) that consumes the PTY stream and maintains the current screen grid + modes. On attach, serialize the current screen (or replay a compact snapshot) then stream live bytes. This is what tmux effectively does.
- **Resize-jiggle redraw (pragmatic v1):** keep a bounded ring buffer for the scrollback portion, and on reattach force the TUI to repaint by resizing the PTY (cols−1 then cols, i.e., two `pty.Setsize` calls → kernel delivers SIGWINCH → Claude Code redraws the full screen). Cheap and works because well-behaved alt-screen TUIs repaint on SIGWINCH. Caveat: known Claude Code bug where resize storms duplicate banner content into scrollback (anthropics/claude-code#49086) — debounce to a single jiggle.
- Either way, `term.reset()` on the client before replay/snapshot so stale state never mixes with the new stream.

**Warning signs:**
Reattach shows a blank screen until the next keypress; reattach shows interleaved garbage; reattach takes seconds on old sessions.

**Phase to address:**
This is the hardest design decision in the project — decide in the session-engine phase *before* building reattach UX; flag for deeper phase-specific research.

---

### Pitfall 4: No flow control — fast PTY output overwhelms WebSocket and browser

**What goes wrong:**
A command like `cat large.log` or a fast Claude Code tool-output burst produces output far faster than the browser renders. Without backpressure the server buffers unboundedly (memory blowup), the WebSocket queue grows (multi-second input latency — keystrokes feel dead), and xterm.js silently discards data past its 50MB internal buffer.

**Why it happens:**
WebSockets give no application-level flow control; reading the PTY in a tight loop and `ws.Write()`ing "works" in demos.

**How to avoid:**
- Implement the xterm.js-documented watermark protocol: client tracks bytes pending in `term.write(chunk, callback)`; when pending > high water (~128KB) send a `pause` control message; server stops reading the PTY (the kernel PTY buffer then blocks the child — natural backpressure); on low water (~16KB) send `resume`.
- Alternatively (simpler, weaker): client ACKs every N bytes processed; server caps unacknowledged bytes in flight.
- Batch client writes per animation frame to keep `term.write` call counts sane.
- Bound any server-side per-session output buffer; drop-oldest with an explicit marker rather than OOM.

**Warning signs:**
Typing latency during heavy output; server RSS spikes when a session prints a lot; output truncated or frozen terminal after big bursts.

**Phase to address:**
Terminal streaming phase — design the WebSocket protocol with control messages (resize, pause/resume, stdin, stdout) from the start, not as raw byte pipes.

---

### Pitfall 5: Resize handling — fit-addon loops, hidden tabs, and size authority

**What goes wrong:**
Several intertwined failures: (a) FitAddon on a hidden/`display:none` container computes 0/Infinity dimensions — known xterm.js issues where this crashes the tab or sets cols=1; (b) ResizeObserver + fit() + fractional pixel sizes create resize feedback loops; (c) the kanban task view has multiple terminal tabs — resizing every hidden terminal on every layout change wastes CPU and triggers Claude Code's redraw-duplication bug; (d) forgetting to propagate resize to the PTY (`pty.Setsize`) leaves the TUI rendering at the wrong size — Claude Code wraps/garbles lines.

**Why it happens:**
The DOM-side terminal size and PTY-side size are two systems that must be kept in sync, and React mount/unmount/tab-switch life cycles fight with them.

**How to avoid:**
- Client is the source of truth for size: on fit, send `{cols, rows}` over WS; server calls `pty.Setsize` (kernel delivers SIGWINCH automatically — no manual signaling needed).
- Only call `fit()` when the terminal container is visible and has nonzero dimensions; guard `if (width > 0 && height > 0)`. Fit once on tab activation, debounce ResizeObserver (~50–100ms).
- Keep one live xterm instance per session, but don't keep hidden ones in layout-affecting DOM; either unmount or freeze fitting while hidden, refit on show.
- Debounce server-side `Setsize` during drag-resizes to avoid Claude Code's per-frame redraw scrollback flooding.

**Warning signs:**
Browser tab freezes when switching kanban tasks; terminal shows 1-column output; duplicated Claude Code banners filling scrollback after window resize.

**Phase to address:**
Terminal UI phase; include "resize while hidden tab" and "drag-resize window" as explicit test cases.

---

### Pitfall 6: Spawned environment is wrong — TERM, PATH, HOME

**What goes wrong:**
The Go server inherits a minimal environment (especially if started from a launcher, systemd, or a non-login shell). Consequences: `claude` binary not found (it's often in `~/.local/bin`, a node version manager shim, or npm global bin not on PATH); `TERM` unset or `dumb`, so Claude Code's TUI renders without colors/box-drawing or refuses fullscreen mode; missing `HOME` breaks `~/.claude` session storage, killing `--resume`.

**Why it happens:**
Devs test by running the server from their interactive shell where everything is inherited; it breaks the moment the server is launched any other way.

**How to avoid:**
- Explicitly construct the child env: set `TERM=xterm-256color`, `COLORTERM=truecolor`, pass through `HOME`, `PATH`, `LANG`/`LC_*`, and Claude-specific vars (`ANTHROPIC_*`, `CLAUDE_*`).
- Resolve the `claude` binary at project/app config time (configurable path + `exec.LookPath` fallback) and surface a clear error in the UI if missing — don't fail silently inside the PTY.
- Consider launching bash tabs as `bash -l` or interactive (`-i`) deliberately, understanding that rc files will run.

**Warning signs:**
"command not found: claude" in the terminal; monochrome/ASCII-art-broken TUI; `--resume` finds no sessions.

**Phase to address:**
PTY session engine phase (env construction); app setup/onboarding phase (claude binary detection with user-visible diagnostics).

---

### Pitfall 7: Worktree creation/cleanup that can damage the user's repo state

**What goes wrong:**
The app operates on the user's real repository. Failure modes:
- **Branch collision:** `git worktree add -b <branch>` fails if the branch exists or is checked out in another worktree (including the user's main checkout). Task titles like `Fix bug #1: foo/bar` produce invalid ref names.
- **Destructive cleanup:** `git worktree remove` refuses dirty worktrees; reaching for `--force` or `rm -rf` deletes uncommitted agent work. Deleting the directory without `git worktree remove` leaves stale admin entries in `.git/worktrees/`, causing later "already checked out" / "already exists" errors.
- **Cleanup while sessions live:** removing a worktree whose PTY sessions still have it as cwd — processes end up in a deleted directory; subsequent git commands in those shells fail confusingly, and removal itself may fail with EBUSY.
- **Submodules:** new worktrees don't init submodules; agent builds fail mysteriously. (`git worktree remove` also refuses worktrees containing submodules.)
- **Worktree placement:** creating worktrees inside the repo tree pollutes `git status` for the user unless ignored.

**Why it happens:**
git worktree semantics are subtle and the app automates them against a repo it doesn't own.

**How to avoid:**
- Sanitize task titles into ref-safe slugs (`git check-ref-format --branch` rules); add a unique suffix (task ID) to guarantee no collision; verify branch nonexistence first and fail with a clear message.
- Place worktrees in an app-owned directory *outside* the repo (e.g., `~/.kangent/worktrees/<project>/<task>/` or a sibling dir), never inside the checkout.
- Cleanup flow: (1) stop all task sessions and wait for process-group exit, (2) check `git status --porcelain` in the worktree, (3) if dirty, show the user what's uncommitted and require explicit confirmation before `git worktree remove --force`, (4) keep the branch (per requirements). Run `git worktree prune` opportunistically on project open to self-heal stale entries.
- Detect submodules (`.gitmodules`) and run `git submodule update --init --recursive` after add — or at minimum warn.
- Treat every git invocation as fallible: parse exit codes/stderr and surface them in the UI; never assume success.

**Warning signs:**
"fatal: '<branch>' is already checked out at ..."; tasks whose worktree dir exists but git doesn't know about it (or vice versa); user reports lost uncommitted changes — this one is trust-destroying.

**Phase to address:**
Worktree management phase. The "dirty worktree confirmation" rule is non-negotiable and should be a phase success criterion.

---

### Pitfall 8: DB claims sessions are running after server restart (and lost Claude session identity)

**What goes wrong:**
PTY processes die with the server, but the DB still says `running`. On restart the UI shows live sessions that don't exist; clicking them errors or, worse, spawns duplicates. Additionally, if the app never captured Claude's session ID, `claude --resume` recovery requires the user to pick from an interactive picker inside a fresh PTY — fragile to automate, and resuming sessions is directory-scoped (sessions live under `~/.claude/projects/<encoded-cwd>/`), so resume must run with cwd = the same worktree.

**Why it happens:**
Process state is ephemeral; DB state is durable; nobody writes the reconciliation code until the first restart bites.

**How to avoid:**
- Startup reconciliation pass: mark every `running` session row as `exited(unclean)` before serving requests (the server owns all PTYs, so none can survive it). Optionally record server boot ID per session to detect this positively.
- Capture Claude session identity at spawn: launch with `claude --session-id <uuid-you-generate>` so the resume command is fully deterministic (`claude --resume <uuid>` in the same worktree cwd). Verify this flag against the installed Claude Code version at runtime; fall back to parsing `~/.claude/projects/<encoded-cwd>/*.jsonl` mtimes if needed (MEDIUM confidence — flag surface changes between versions).
- Never auto-resume the same Claude session into two PTYs concurrently — transcripts interleave (documented Claude behavior). Enforce one live PTY per task session row.
- UI: show "session ended (server restarted) — Resume?" rather than silently restarting.

**Warning signs:**
After restart, task views show connected terminals with no output; duplicate `claude` processes for one task; resume starting a blank conversation instead of the old one.

**Phase to address:**
Persistence/reconciliation phase — should land in the same phase as session spawn, not later; recovery UX (resume button) can follow in a later phase.

---

### Pitfall 9: SQLite concurrency misconfiguration in Go

**What goes wrong:**
Intermittent `database is locked` / `SQLITE_BUSY` errors under concurrent access — e.g., session-exit goroutines updating rows while HTTP handlers read. Worst with default journal mode, no busy_timeout, and Go's default connection pool opening many connections.

**Why it happens:**
`database/sql` pools connections; SQLite allows one writer; deferred transactions that upgrade read→write return SQLITE_BUSY immediately regardless of busy_timeout.

**How to avoid:**
- Open with `PRAGMA journal_mode=WAL`, `PRAGMA busy_timeout=5000`, `PRAGMA foreign_keys=ON`, `PRAGMA synchronous=NORMAL`.
- Either `db.SetMaxOpenConns(1)` (simplest, fine for this app's tiny write volume), or two pools: a single-connection write pool using `BEGIN IMMEDIATE` transactions + a multi-connection read pool.
- Prefer `modernc.org/sqlite` (pure Go, no CGO) for the single-binary goal unless you need a CGO extension; cross-compilation stays trivial.
- Keep transactions short; never hold a transaction across PTY I/O or git operations.

**Warning signs:**
Sporadic 500s mentioning "locked"; hangs when a task is moved while sessions are exiting.

**Phase to address:**
Foundation/data-layer phase — pragmas and pool settings are 10 lines if done first, a debugging week if done later.

---

### Pitfall 10: "It's localhost, so it's safe" — CSWSH and DNS rebinding give remote pages a terminal

**What goes wrong:**
This app's WebSocket is a shell. Any web page you visit can attempt `new WebSocket("ws://localhost:PORT/...")` — browsers do not enforce same-origin on WebSockets. Without Origin validation, a malicious page gets full keystroke access to a PTY running in your repos = arbitrary code execution. DNS rebinding similarly defeats "we only bind 127.0.0.1" for plain HTTP endpoints (Host header points at attacker domain resolving to 127.0.0.1). This exact class hit webpack-dev-server and Vite (CVE'd advisories).

**Why it happens:**
"Single user at localhost" reads like a non-security context. The PTY makes it the highest-stakes localhost app possible.

**How to avoid:**
- Bind to `127.0.0.1` only (not `0.0.0.0`).
- Strictly validate `Origin` on every WebSocket upgrade (exact match against `http://localhost:PORT` / `http://127.0.0.1:PORT`); reject missing/other origins. Note gorilla/websocket rejects cross-origin by default but `nhooyr.io/websocket` and some setups require explicit `CheckOrigin` — never set the permissive `return true` found in every tutorial.
- Validate `Host` header on all HTTP routes (blocks DNS rebinding).
- Add a per-instance bearer token: generated at startup, embedded in the served frontend, required on API + WS connect. Cheap, and makes both attacks moot even if a check regresses.

**Warning signs:**
`CheckOrigin: func(...) bool { return true }` anywhere in the codebase; WS connects succeeding from a page served on another port during testing.

**Phase to address:**
Must be in the first phase that exposes the WebSocket endpoint — not a hardening afterthought. Verification: an integration test that a cross-origin upgrade is rejected.

---

### Pitfall 11: Claude Code TUI-specific surprises

**What goes wrong:**
A cluster of behaviors that break assumptions:
- Alternate-screen TUI keeps only ~2000 lines of internal scrollback (not configurable); users expect browser-terminal scrollback of the whole conversation and won't get it.
- Interactive permission prompts and plan mode require real keystroke round-trips — anything that buffers, drops, or reorders stdin (e.g., during reattach) leaves the agent stuck waiting on an invisible prompt.
- Mouse reporting: Claude Code enables mouse mode; xterm.js forwards mouse events, which means browser text selection/copy behaves differently inside the TUI (users must use the TUI's own selection or you intercept with a modifier key).
- Bracketed paste: multi-line pastes into the prompt rely on bracketed paste mode passing through intact; mangling it submits each line as a separate message.
- `CLAUDE_CODE_DISABLE_ALTERNATE_SCREEN=1` exists as an escape hatch to line-oriented output — a legitimate v1 simplification lever for the replay problem, at the cost of the fullscreen UX (MEDIUM confidence: env var behavior is version-dependent; verify against installed version).

**Why it happens:**
Claude Code is a fast-moving TUI; its terminal contract is richer than a typical CLI.

**How to avoid:**
Test the real `claude` binary inside the app early (week 1 spike), specifically: permission prompt flow, paste of multi-line text, mouse selection, detach during a pending prompt, and reattach mid-conversation. Treat "bash in a PTY works" as proving ~60% of the problem.

**Warning signs:**
Agent appears hung after reattach (pending prompt not visible); pasted code arrives as multiple messages; users complain they can't copy text.

**Phase to address:**
A dedicated "Claude Code integration" spike/phase after the generic PTY engine works with plain bash.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Full raw-stream recording for replay (no headless emulator) | No VT-state code | Multi-MB replays, broken alt-screen reattach | Never for TUI sessions; fine for bash-tab logs |
| `CheckOrigin: return true` "to make it work in dev" | WS connects from Vite dev server | Remote code execution via CSWSH | Never — fix dev origin allowlist instead |
| `rm -rf` worktrees instead of `git worktree remove` | One less git invocation | Stale `.git/worktrees` metadata, "already checked out" errors | Never |
| Storing terminal output in SQLite per chunk | Durable scrollback | Write amplification, DB bloat, lock contention | Never per-chunk; periodic snapshot to file is fine |
| Skipping `--session-id` capture at spawn | Less version-coupling to claude flags | Resume after restart requires interactive picker automation | MVP only if resume is manual-in-terminal |
| Text WebSocket frames with JSON-base64 output | Easy debugging | ~33% bandwidth overhead, encode/decode CPU on hot path | Acceptable for v1 if binary path is awkward; keep protocol versioned |
| One global SQLite connection, no migrations framework | Fast start | Fine, honestly, at this scale | Acceptable for v1 (single user) |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| creack/pty | Manually plumbing SIGWINCH from somewhere | Client sends cols/rows over WS → `pty.Setsize(f, &pty.Winsize{...})`; kernel delivers SIGWINCH to the child automatically |
| creack/pty | Treating PTY master `Read` EOF as the only exit signal | Use `cmd.Wait()` goroutine as authoritative exit event; on some platforms read errors with EIO instead of EOF when child dies |
| xterm.js | `new Terminal()` defaults + DOM renderer | Use `@xterm/addon-webgl` (fallback canvas), `scrollback` sized deliberately, `term.write(Uint8Array)` |
| xterm.js FitAddon | `fit()` on hidden/zero-size container | Guard on visibility + nonzero dims; fit on tab activation; debounce ResizeObserver |
| Claude Code CLI | Spawning with server's bare env | Explicit env: TERM=xterm-256color, HOME, PATH, LANG; resolve binary path with LookPath + config override |
| Claude Code CLI | Assuming flags (`--resume`, `--session-id`) are stable | Version-check `claude --version` at startup; gate features on detected version |
| git worktree | Deriving branch directly from task title | Slug + task-ID suffix; validate with check-ref-format rules; handle "branch exists" as expected error |
| git worktree | Cleanup without checking dirty state / live processes | Stop sessions → porcelain status check → user confirmation → `worktree remove` → keep branch |
| SQLite (modernc/mattn) | Default pool + default journal mode | WAL + busy_timeout=5000 + MaxOpenConns(1) (or split read/write pools with BEGIN IMMEDIATE) |
| WebSocket (gorilla/nhooyr) | No ping/pong keepalive | Heartbeat both directions; detect dead clients to release flow-control pauses (a paused PTY with a dead client = hung session) |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| One `term.write()` per WS message | Jank during fast output | Coalesce into per-animation-frame flushes | Any `cat bigfile` / verbose agent tool output |
| Unbounded server output buffering per session | RSS climbs with detached sessions producing output | Bounded ring buffer per session; pause PTY reads when no client and buffer full | First long-running detached noisy session |
| DOM renderer instead of WebGL | High CPU at 100% during TUI animation (Claude Code spinners redraw constantly) | WebGL addon | Immediately, with any animated TUI |
| Mounting every task's terminal in the kanban DOM | Page slows as task count grows | Mount terminals lazily on task open; dispose on close (server keeps the session) | ~10+ tasks with sessions |
| Replaying full session history on each reattach | Reattach latency grows with session age | Snapshot/ring-buffer replay (see Pitfall 3) | Hours-old sessions |
| Per-output-chunk DB writes | SQLITE_BUSY storms, disk churn | Don't persist output to DB; in-memory ring + optional file snapshot | First chatty session |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| No Origin check on WS upgrade | Any website gets keystroke access to a shell (RCE) | Strict Origin allowlist; per-instance auth token |
| No Host header validation | DNS rebinding reaches HTTP API from remote pages | Reject Host ∉ {localhost:PORT, 127.0.0.1:PORT} |
| Binding 0.0.0.0 "to test from phone" | LAN-wide unauthenticated shell | Hard-default 127.0.0.1; require explicit flag + token to widen |
| Passing task title/description into shell commands | Command injection via task fields into git calls | Always `exec.Command` with arg arrays, never `sh -c` with interpolation; sanitize ref names separately |
| Serving the repo browser/API without path normalization | Path traversal from project "directory" field into arbitrary FS | Clean + absolutize paths; require the dir to contain `.git`; no symlink-following surprises |
| Auto-answering Claude permission prompts or defaulting `--dangerously-skip-permissions` | Agent executes destructive actions without the human gate the CLI was chosen for | Keep prompts interactive; if a yolo-mode toggle exists, make it explicit per-task UI |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| No visual distinction between "attached to live session" and "showing stale snapshot" | User types into a dead terminal | Connection state badge; disable input until attached; auto-reconnect with backoff |
| Reattach lands on a blank alt-screen until keypress | Looks broken | Resize-jiggle redraw or screen snapshot on attach (Pitfall 3) |
| Marking task Done silently force-removes a dirty worktree | Lost uncommitted agent work; trust destroyed | Show porcelain diff summary; require confirmation; default to keep |
| Killing sessions when browser tab closes (accidental coupling) | Work lost on tab close, violating core promise | Server lifecycle fully decoupled from WS lifecycle; only explicit Stop kills |
| Browser keybindings swallow terminal keys (Ctrl+W, Ctrl+T, Ctrl+R, Cmd+K) | Closes tab instead of sending to TUI | `attachCustomKeyEventHandler`; document unfixable ones (Ctrl+W can't be intercepted in most browsers) — consider alternative bindings note in UI |
| Copy/paste mismatch (TUI mouse mode owns the mouse) | "I can't select text" | Shift+drag for native selection (xterm.js default), visible hint; Ctrl/Cmd+V paste path tested with multi-line bracketed paste |
| Claude's ~2000-line internal scrollback vs. user expectation of full history | "Where did the conversation go?" | Document Ctrl+O transcript mode in-app; don't promise scrollback the TUI can't give |

## "Looks Done But Isn't" Checklist

- [ ] **Session spawn:** Works from your dev shell — verify it works when the server starts from a clean env (`env -i ./kangent`) with TERM/PATH/HOME constructed explicitly
- [ ] **Session stop:** Claude exits — verify zero descendant processes remain (`pgrep -g <pgid>`) including MCP servers and bash children
- [ ] **Reattach:** Works after 1 minute — verify after hours of output, after a pending permission prompt, and after server-side flow-control pause
- [ ] **Resize:** Works on window drag — verify with hidden tab → activate, browser zoom ≠ 100%, and rapid drag (no scrollback duplication storm)
- [ ] **Worktree create:** Works on toy repo — verify on a repo with submodules, with the task branch name already existing, and when the project dir is itself a worktree
- [ ] **Worktree cleanup:** Works on clean tree — verify dirty tree prompts, running session blocks/stops first, and `.git/worktrees` has no stale entries after
- [ ] **Restart recovery:** DB rows reconciled — verify UI offers resume and `claude --resume <id>` actually restores the conversation in the worktree cwd
- [ ] **Security:** WS connects from app — verify cross-origin upgrade is rejected (test from a page on another port) and Host-header spoof is rejected
- [ ] **Paste:** Single line works — verify 200-line paste arrives as one bracketed paste, not 200 submissions
- [ ] **Unicode:** ASCII output fine — verify emoji/CJK/box-drawing under fast output (stress: `yes 'こんにちは🎉│'`)

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Orphaned processes accumulated | LOW | Add pgroup kill + Wait reaping; one-off `pkill` cleanup; add descendant-count test |
| Stale worktree metadata | LOW | `git worktree prune` + re-add; add prune-on-project-open |
| DB says running after crash | LOW | Startup reconciliation migration; mark rows exited |
| Wrong replay architecture (raw stream) shipped | HIGH | Requires session-engine rework: introduce headless VT state or snapshot protocol; protocol version bump for clients |
| Text-frame/UTF-8 protocol shipped | MEDIUM | Add binary frame type behind protocol version; migrate client write path |
| Dirty worktree force-deleted user work | HIGH (trust) | `git reflog`/`git fsck --lost-found` may recover committed-but-unreferenced objects; uncommitted changes are gone — prevention is the only real strategy |
| CSWSH discovered post-ship | MEDIUM | Add Origin/Host checks + token; force-update; audit for abuse is impossible locally — assume compromise messaging |

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| Zombie/orphan processes (1) | PTY session engine | Test: zero descendants after Stop; no `<defunct>` after 50 spawn/stop cycles |
| UTF-8/escape splitting (2) | PTY engine + WS protocol design | Stress test with multi-byte output under load |
| Alt-screen replay (3) | Session engine design (flag: needs deeper research) | Reattach to hours-old Claude session renders correct screen <1s |
| Flow control (4) | WS streaming protocol | `cat` 500MB file: bounded memory, responsive input, no data loss markers wrong |
| Resize loops/authority (5) | Terminal UI | Hidden-tab activation, zoomed browser, drag-resize all stable |
| Env construction (6) | PTY engine + onboarding | App works launched via `env -i`; claude-not-found shows actionable UI error |
| Worktree safety (7) | Worktree management | Dirty-tree confirmation flow; submodule repo test; no stale metadata |
| Restart reconciliation (8) | Persistence layer (same phase as spawn) | Kill -9 server mid-session → restart → correct states + working resume |
| SQLite config (9) | Foundation/data layer | Concurrent write test passes without SQLITE_BUSY surfacing to handlers |
| CSWSH/DNS rebinding (10) | First phase exposing WS endpoint | Automated test: cross-origin upgrade rejected; bad Host rejected |
| Claude TUI specifics (11) | Dedicated Claude integration spike (flag: needs phase research) | Manual checklist: prompts, paste, mouse, detach-during-prompt |

## Sources

- xterm.js flow control guide (watermark pause/resume protocol, 50MB buffer): https://xtermjs.org/docs/guides/flowcontrol/ — HIGH
- xterm.js flow control discussion: https://github.com/xtermjs/xterm.js/issues/2077 — HIGH
- xterm.js FitAddon Infinity/zero-dimension crashes: https://github.com/xtermjs/xterm.js/issues/1416, https://github.com/xtermjs/xterm.js/issues/5320, https://github.com/xtermjs/xterm.js/issues/3584 — HIGH
- creack/pty docs (Setsize, StartWithSize, InheritSize): https://pkg.go.dev/github.com/creack/pty — HIGH
- Killing Go child process trees (Setpgid, negative PGID): https://medium.com/@felixge/killing-a-child-process-and-all-of-its-children-in-go-54079af94773, https://www.sobyte.net/post/2021-08/avoid-go-command-orphan-processes/ — HIGH
- Claude Code sessions docs (`--resume`, session IDs, interleaving warning, per-directory storage): https://code.claude.com/docs/en/sessions — HIGH
- Claude Code alt-screen scrollback issues: https://github.com/anthropics/claude-code/issues/42670, https://github.com/anthropics/claude-code/issues/38283, https://github.com/anthropics/claude-code/issues/42002 — HIGH
- Claude Code resize redraw-duplication bug: https://github.com/anthropics/claude-code/issues/49086 — HIGH
- Claude Code resume directory-scoping: https://github.com/anthropics/claude-code/issues/5768 — MEDIUM (older issue; current docs say names resolve across worktrees)
- git-worktree official docs (remove/prune/lock, checked-out refusal): https://git-scm.com/docs/git-worktree — HIGH
- Worktrees + submodules guide: https://gist.github.com/ashwch/946ad983977c9107db7ee9abafeb95bd — MEDIUM
- SQLITE_BUSY despite busy_timeout (deferred→write upgrade): https://berthub.eu/articles/posts/a-brief-post-on-sqlite3-database-locked-despite-timeout/ and https://sqlite.org/forum/info/a15478046be7db2a106ae66de00fb97cb9acdb73e5cb5a2c02fc45fa642e8f82 — HIGH
- Go+SQLite practices (WAL, MaxOpenConns(1), modernc vs mattn): https://oneuptime.com/blog/post/2026-02-02-sqlite-go/view, https://github.com/mattn/go-sqlite3/issues/274 — MEDIUM
- CSWSH explainer: https://portswigger.net/web-security/websockets/cross-site-websocket-hijacking — HIGH
- Localhost CORS/DNS rebinding dangers: https://github.blog/security/application-security/localhost-dangers-cors-and-dns-rebinding/ — HIGH
- Real-world dev-server precedents: webpack-dev-server advisory https://github.com/webpack/webpack-dev-server/security/advisories/GHSA-9jgg-88mc-972h, Vite advisory https://github.com/vitejs/vite/security/advisories/GHSA-vg6x-rcgg-rjx6 — HIGH

---
*Pitfalls research for: local PTY-backed Claude Code session manager (Go + React + SQLite)*
*Researched: 2026-06-10*
