# Phase 2: Terminal Engine - Research

**Researched:** 2026-06-10
**Domain:** Server-owned PTY sessions over WebSocket, rendered with xterm.js (Go + creack/pty + coder/websocket + @xterm/xterm 6)
**Confidence:** HIGH (all load-bearing APIs verified against pkg.go.dev, xterm.js master typings, official READMEs, and man pages on research date; project research docs were verified same-day)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Surface (where the terminal lives in Phase 2)
- **D-12:** Phase 2 ships a minimal dev route `/terminal` hosting the engine — NOT wired into the task view. Phase 3 promotes the terminal component into the task tab strip (the `TabDef[]` seam) and the dev route can then be dropped or kept as a debug surface.
- **D-13:** The dev route has a "New terminal" action; each spawn gets a session id. The surface lists live/exited sessions and supports attach (click to open), and stop. This deliberately exercises the full engine: multiple sessions, detach/reattach, replay, termination.

#### Session Lifecycle
- **D-14:** Stop = SIGTERM to the entire process group, wait ~5 seconds, then SIGKILL anything remaining. One button; no orphaned subprocesses (TERM-06).
- **D-15:** Exited sessions freeze their final output with an overlay banner: "Session exited (code N)" plus actions (New terminal / Close). History stays scrollable/copyable until closed.
- **D-16:** No idle timeouts — sessions live until explicitly stopped or the server process dies. "Leave and reattach" is the core value; single user means resource pressure is acceptable.

#### Terminal UX
- **D-17:** Copy-on-select enabled; Ctrl+Shift+C / Ctrl+Shift+V also work; bracketed paste passes through to the PTY (needed for Claude Code multiline prompts in Phase 4).
- **D-18:** Scrollback ~10,000 lines client-side.
- **D-19:** Font: system monospace stack (ui-monospace, ...), 13–14px — consistent with the UI-SPEC no-webfont rule. Terminal theme colors derive from the zinc dark palette.

#### Security
- **D-20:** WebSocket + API security is strict Origin/Host validation only — allowlist of loopback origins (127.0.0.1 / localhost with the exact serving port). No per-instance token in v1 (single-user machine assumption; revisit if shared-machine support ever matters).
- **D-21:** The server refuses to start if `--addr` is not a loopback address. Localhost-only is enforced, not aspirational.

### Claude's Discretion
- Ring buffer size for replay-on-reattach and the replay strategy (raw replay + resize nudge vs other approaches — research flagged this as the hard problem; pick what works against bash now without painting Phase 4's alt-screen TUI into a corner)
- WebSocket wire protocol details (binary frames, control message format — follow project research: ttyd-style 1-byte type prefix)
- WS reconnect behavior on transient drops (auto-reconnect with backoff is fine)
- Session naming/labels on the dev route
- Where sessions live in the DB vs memory for this phase (sessions table can wait for Phase 4 if memory-only is cleaner here — but startup reconciliation lands in Phase 5 either way)

### Deferred Ideas (OUT OF SCOPE)
- Per-instance auth token (Jupyter-style) — revisit only if shared/multi-user machines become a target (D-20 keeps the door open)
- Keeping `/terminal` as a permanent debug surface after Phase 3 — decide when Phase 3 lands
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| TERM-02 | Interact with a real terminal in the browser (full interactive TUI, control sequences) | Standard Stack (creack/pty + coder/websocket + @xterm/xterm 6); Pattern 1 (wire protocol); Pattern 2 (session manager); env construction (TERM=xterm-256color) |
| TERM-03 | Resize (fit to pane), scrollback, copy/paste incl. bracketed paste | Pattern 6 (xterm config: FitAddon+ResizeObserver, scrollback 10000, copy-on-select via onSelectionChange, attachCustomKeyEventHandler, bracketed paste pass-through verified against typings) |
| TERM-05 | Sessions survive tab close; reattach replays recent output with clean redraw | Pattern 2 (manager owns PTY) + Pattern 3 (ring buffer replay + SIGWINCH jiggle — verified that unchanged TIOCSWINSZ does NOT signal) |
| TERM-06 | Stop terminates the entire process tree, zero orphans | Pattern 4 (pty.Start makes child session leader → kill(-pid); SIGTERM → 5s → SIGKILL escalation; Wait() reaping) |
| TERM-07 | WS endpoints validate Origin/Host (CSWSH protection) | Pattern 5 (coder/websocket OriginPatterns semantics verified; loopback Host middleware; loopback-bind enforcement; dev-proxy wrinkle resolved) |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Locked stack (do not substitute):** `github.com/creack/pty` **v1 import path** (v2 is orphaned), `github.com/coder/websocket` (NOT gorilla), `@xterm/xterm` 6 scoped packages (NOT unscoped `xterm`), binary WS frames only (never text frames for PTY bytes), no `@xterm/addon-attach` (hand-roll ~30 lines), no `@xterm/addon-canvas` (removed in 6.0 — WebGL + DOM fallback).
- REST endpoints under `/api/*` registered via `api.Routes(mux, db)`; JSON respond helpers; table-driven Go tests with httptest.
- Frontend data flows through typed TanStack Query hooks; components own their files (wave-parallel discipline); zinc token set in `web/src/index.css`.
- GSD workflow enforcement: file changes happen through GSD commands (`/gsd:execute-phase` for this work).
- User global instruction: never mention or co-author Claude (or happy-otter) on commits/PRs.

## Summary

This phase builds the highest-risk subsystem: a session manager inside the existing Go binary that owns bash PTYs independent of any browser connection, a binary WebSocket bridge with a ttyd-style 1-byte type prefix, and a self-contained React `TerminalPane` on a `/terminal` dev route. Every library choice is already locked by project research (verified 2026-06-10, same day as this research); this document resolves the four discretion areas, verifies the exact API surfaces against current docs, and nails down the sequencing details the planner needs — especially the replay-then-nudge ordering and the dev-proxy security wrinkle.

The hard problem (replay strategy) resolves to: **1 MiB raw ring buffer per session, replayed atomically on attach, followed by a server-side resize jiggle**. The critical verified fact: the Linux kernel sends SIGWINCH **only when the winsize actually changes** (man7 TIOCSWINSZ), so a "nudge" with the same dimensions is a no-op — the server must jiggle (rows−1, then rows) when the client's requested size equals the PTY's current size. The Phase 4 escape hatch is structural, not algorithmic: the WS handler consumes an opaque `Snapshot() []byte` on the session, so Phase 4 can swap the ring for a headless-VT-emulator snapshot without touching the protocol.

Two environment-specific gotchas surfaced from the codebase: `web/src/main.tsx` renders inside `<StrictMode>` (effects run setup→cleanup→setup in dev — the terminal effect must fully tear down WS + xterm in cleanup, and session spawn must never live in an effect), and `web/vite.config.ts` uses the string-shorthand proxy which does NOT proxy WebSockets (`ws: true` required), and the Vite dev origin (port 5173) must be admissible by the Origin allowlist via a `--dev-origin` flag.

**Primary recommendation:** Build `internal/session` (manager + ring + teardown) first with table-driven tests asserting zero surviving descendants, then `internal/ws` (framing + Origin checks), then the React pane. Memory-only session records; no DB migration this phase.

## Standard Stack

### Core (new dependencies this phase)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/creack/pty` | v1.1.24 | PTY allocation, resize | Project-locked. `pty.Start(cmd)` "starts the process in a new session and sets the controlling terminal" (verified pkg.go.dev) — child is session leader, so PGID == child PID. v1.1.24 confirmed latest tag (GitHub tags API, 2026-06-10); v2 remains orphaned |
| `github.com/coder/websocket` | v1.8.14 | WS accept, binary frames, close codes | Project-locked. v1.8.14 confirmed latest release (GitHub releases API, 2026-06-10). Concurrent writes safe; only `Read`/`Reader` must be serialized (verified pkg.go.dev) |
| `github.com/armon/circbuf` | v0.0.0-20190214190532 | Per-session output ring buffer | What Coder's reconnectingpty uses in production. `Bytes()` returns chronological contents after wrap (verified). Frozen since 2019 but ~100 lines, zero deps, battle-proven; acceptable |
| `@xterm/xterm` | 6.0.0 | Browser terminal | Project-locked; addon set pinned together per STACK.md |
| `@xterm/addon-fit` | 0.11.0 | Fit terminal to pane | Required for resize flow |
| `@xterm/addon-webgl` | 0.19.0 | GPU renderer | Default renderer since canvas removal; `onContextLoss(() => addon.dispose())` falls back to the built-in DOM renderer (verified addon README) |

### Already present (verified in repo)

| Asset | Where | Reuse |
|-------|-------|-------|
| stdlib ServeMux + `api.Routes(mux, db)` | `cmd/kangent/main.go`, `internal/api/routes.go` | Register session REST + WS routes on the same mux; `/api/` prefix already excluded from SPA fallback |
| `--addr` flag, default `127.0.0.1:7333` | `cmd/kangent/main.go:19` | D-21 loopback check lands right after `flag.Parse()` |
| TanStack Query client + `get<T>` fetch layer | `web/src/api/` | Session list/spawn/stop hooks mirror `useProjects`/`useTasks` |
| shadcn `button, tooltip, separator, skeleton`; zinc tokens; `--font-mono` | `web/src/index.css:12`, `web/src/components/ui/` | No new shadcn components needed (UI-SPEC) |
| React Router routes under `AppLayout` | `web/src/App.tsx` | Add `<Route path="/terminal" ...>` inside the `AppLayout` route |

### Installation

```bash
# Backend (from repo root)
go get github.com/creack/pty@v1.1.24
go get github.com/coder/websocket@v1.8.14
go get github.com/armon/circbuf

# Frontend (from web/)
npm install @xterm/xterm@6.0.0 @xterm/addon-fit@0.11.0 @xterm/addon-webgl@0.19.0
```

xterm packages are NOT yet in `web/package.json` — this install is a real task, not a no-op.

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| armon/circbuf | Hand-rolled ~40-line ring | Equivalent; circbuf wins on parity with the Coder reference and existing tests. Either is acceptable — do not bikeshed |
| Server-side resize jiggle | Client-side `term.reset()` + re-fit tricks | Client cannot make the *child process* repaint; only SIGWINCH (a real size change) does. Jiggle must be server-side |
| Memory-only sessions | `sessions` table now | Table now forces reconciliation logic now (Phase 5 scope) for throwaway dev-route bash sessions. Memory-only is cleaner; see Discretion Resolutions |

## Claude's Discretion — Resolved Recommendations

### 1. Ring buffer size + replay strategy (the hard problem)

**Use a 1 MiB `circbuf.Buffer` per session. Replay = raw ring bytes; redraw = server-side SIGWINCH jiggle.**

- **Size rationale:** D-18 fixes client scrollback at 10,000 lines. Typical shell output runs ~80–120 bytes/line including SGR sequences, so ~1 MiB fills the client's entire scrollback budget on reattach. Coder's 64 KiB default is tuned for fleet memory pressure that doesn't exist here; 10 concurrent sessions cost 10 MiB. Don't go bigger: xterm discards what scrollback can't hold anyway, and replay time grows linearly.
- **Replay sequencing (exact order matters):**
  1. Client opens WS → server `Attach`: under the session mutex, snapshot `ring.Bytes()` AND register the connection's write queue in one critical section (queue the snapshot as the queue's first item). This guarantees no output gap or duplication between replay and live stream.
  2. Client writes replay frames into a **fresh** Terminal instance (no `reset()` needed on first attach; on in-place reconnect, call `term.reset()` before the new replay arrives).
  3. Client runs `fit()` and sends its first resize frame.
  4. Server, on the **first resize after an attach**: if requested `{cols,rows}` differs from current PTY size → single `pty.Setsize` (the change itself delivers SIGWINCH). If it is **equal** → jiggle: `Setsize(rows−1)` then `Setsize(rows)` (two real changes → SIGWINCH → full-screen programs repaint). One jiggle per attach, never more (debounce — Claude Code has a known resize-storm duplication bug, anthropics/claude-code#49086).
- **Verified load-bearing fact:** TIOCSWINSZ sends SIGWINCH to the foreground process group **only when the window size changes** (man7.org TIOCSWINSZ.2const, HIGH). A same-size "nudge" does nothing — this is why the jiggle exists.
- **Phase 4 seam (don't paint into a corner):** the WS layer must consume `session.Snapshot() []byte` — an opaque method — never reach into the ring directly. A bounded raw ring inherently evicts the alt-screen-enter sequence (`CSI ?1049h`, emitted once at TUI start) on any long-lived session; no ring size fixes that. Phase 4's likely answer is a headless VT emulator producing a screen snapshot behind the same `Snapshot()` method (already flagged in STATE.md blockers). The wire protocol is agnostic: replay bytes are just output frames. Nothing in Phase 2 needs to change shape later.

### 2. Exact wire protocol

Binary WS frames only, ttyd-style 1-byte ASCII type prefix:

| Direction | Byte | Payload | Meaning |
|-----------|------|---------|---------|
| client→server | `'0'` (0x30) | raw UTF-8 bytes from `term.onData` | stdin (includes bracketed-paste wrappers untouched) |
| client→server | `'1'` (0x31) | JSON `{"cols":N,"rows":N}` | resize → `pty.Setsize` |
| server→client | `'0'` (0x30) | raw PTY bytes | output (replay and live use the same frame) |
| server→client | `'x'` (0x78) | JSON `{"code":N}` | session exited; server then closes with 1000 |

Close codes: `1000` normal (after `'x'` delivered, or clean detach); **`4404`** application code = "session not found / already closed" — client sees `CloseEvent.code === 4404`, skips the retry cycle, shows the "Session not found." banner (UI-SPEC). 4000–4999 is the RFC 6455 private-use range (verified). ttyd's PAUSE/RESUME bytes (`'2'`/`'3'`) are deliberately reserved-not-implemented this phase (see flow-control note in Pitfalls).

Server read loop: `conn.Read(ctx)` in a per-connection goroutine (only reads must be serialized — verified). Call `conn.SetReadLimit(1 << 20)`: the library default is **32 KiB** and a large bracketed paste can exceed it, which would kill the connection with StatusMessageTooBig.

### 3. WS reconnect implementation

The UI-SPEC already fixes the contract: exponential backoff 0.5s → 1s → 2s → 4s → 8s, 5 attempts, reconnecting banner, input disabled but scrollback usable; exhausted → "Couldn't reconnect." + Retry; close code 4404 → skip retries → "Session not found."

Implementation shape: keep the Terminal instance alive across reconnect attempts (scrollback must stay usable). Separate the terminal's lifecycle (one per `sessionId` per mount) from the socket's lifecycle (a `connect()` function with an attempt counter, re-invoked by a timer). On reconnect success: `term.reset()` → replay arrives → fit + resize send (which triggers the server-side nudge). Suppress reconnect when the close was clean-and-expected: user clicked Close, component unmounting (cleanup sets a `disposed` flag checked in `onclose`), or code 1000-after-`'x'` (transition to exited banner instead).

### 4. Memory-only vs DB session records

**Memory-only this phase.** Rationale: these are throwaway dev-route bash sessions; a DB table now would immediately demand the startup-reconciliation sweep (Phase 5 scope, RCVR-01) to avoid ghost rows, for zero Phase 2 value. The schema already designed in ARCHITECTURE.md lands in Phase 4 when sessions gain task identity.

**The seam to build now:** session IDs are opaque strings at every boundary (REST JSON, WS URL, React props) — generate `uuid.NewString()` (google/uuid is already an indirect dep; promote it) and keep a separate monotonic spawn counter for the `bash #N` label (UI-SPEC: server-assigned spawn order, never reused, per server lifetime). The manager's lifecycle transitions (spawned / exited / closed) should each be a single method so Phase 4 can add "write DB row" inside them without restructuring.

REST surface (mirrors existing `internal/api` patterns):

| Endpoint | Action |
|----------|--------|
| `GET /api/sessions` | list `{id, label, status, exitCode?, createdAt}` |
| `POST /api/sessions` | spawn bash → 201 `{id, label, ...}` |
| `POST /api/sessions/{id}/stop` | D-14 teardown; 202; idempotent (stopping/stopped → no-op) |
| `DELETE /api/sessions/{id}` | remove an **exited** session (frees ring); 409 if still running |
| `GET /api/sessions/{id}/ws` | WS upgrade (coder/websocket Accept) |

Spawn cwd: the user's home directory (`os.UserHomeDir()`); worktree cwds are Phase 3. Shell: `$SHELL` fallback `/bin/bash`.

## Architecture Patterns

### Recommended Structure (additions)

```
internal/
├── session/            # Manager, Session, ring, spawn/stop, attach/detach, exit watcher
│   ├── manager.go      #   no imports of api/ or ws/ — enforces "PTY outlives socket"
│   ├── session.go
│   └── session_test.go #   zero-descendants test lives here
├── ws/                 # WS handler: Accept opts, framing, read/write pumps
│   ├── handler.go
│   └── proto.go        #   frame type bytes (shared constants)
internal/api/
│   └── sessions.go     # REST handlers calling session.Manager
cmd/kangent/main.go     # loopback-bind check, Host middleware, --dev-origin flag
web/src/
├── components/terminal/
│   ├── TerminalPane.tsx       # self-contained: props = sessionId only (Phase 3 portability)
│   ├── useTerminalSocket.ts   # WS + protocol + reconnect hook
│   └── xtermTheme.ts          # zinc theme object (UI-SPEC values, verbatim)
├── pages/TerminalPage.tsx     # dev route: header + rail + pane composition
└── api/sessions.ts            # TanStack Query hooks
```

### Pattern 1: Session manager + fan-out pump (Coder reconnectingpty shape)

**What:** Manager owns `map[string]*Session`. Each session: one pump goroutine reading the PTY master, writing to the ring and to every attached connection's buffered queue; one `Wait()` goroutine as the authoritative exit event.

```go
// internal/session/session.go (essence)
type Session struct {
    ID, Label string
    cmd       *exec.Cmd
    ptmx      *os.File
    ring      *circbuf.Buffer            // 1 MiB
    mu        sync.Mutex
    conns     map[string]chan []byte     // per-conn output queues (buffered, e.g. 256)
    status    Status                     // running | exited
    exitCode  int
    done      chan struct{}              // closed when Wait() returns
}

// pump goroutine:
//   buf := make([]byte, 4096)
//   n, err := s.ptmx.Read(buf)  // err == EIO on Linux after child exit — treat ANY error as EOF
//   s.mu.Lock(); s.ring.Write(buf[:n]); for _, q := range s.conns { select { case q <- chunk: default: /* drop conn: close its queue */ } }; s.mu.Unlock()

// Attach: atomically snapshot + subscribe (no gap, no duplication)
func (s *Session) Attach(connID string) (q chan []byte) {
    s.mu.Lock(); defer s.mu.Unlock()
    q = make(chan []byte, 256)
    q <- append([]byte(nil), s.ring.Bytes()...) // replay is the first queued item
    s.conns[connID] = q
    return q
}
// Detach: delete from map, close queue. PTY untouched — that IS TERM-05.
```

Per-connection: ONE writer goroutine draining the queue and calling `conn.Write(ctx, websocket.MessageBinary, frame)`. coder/websocket permits concurrent writes (verified), but a single writer per conn is still required to **guarantee output ordering**. If a queue is full (stalled browser), drop the connection — it auto-reconnects and gets a fresh replay; this keeps the pump non-blocking (the cheap localhost answer to backpressure).

**When to use:** This is the phase. Non-negotiable shape.

### Pattern 2: Process-group teardown (D-14, TERM-06)

`pty.Start` already starts the child "in a new session" (verified) → the child is session leader → **PGID == child PID**. No manual `SysProcAttr` needed for spawn.

```go
// Spawn
cmd := exec.Command(shell)            // $SHELL fallback /bin/bash
cmd.Dir = home
cmd.Env = []string{                   // explicit env — never inherit blindly (PITFALLS #6)
    "TERM=xterm-256color", "COLORTERM=truecolor",
    "HOME=" + home, "PATH=" + os.Getenv("PATH"),
    "LANG=" + os.Getenv("LANG"), "USER=" + os.Getenv("USER"), "SHELL=" + shell,
}
ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})

// Reap (authoritative exit event — PTY read EIO is NOT the exit signal)
go func() {
    _ = cmd.Wait()
    code := cmd.ProcessState.ExitCode()
    if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
        code = 128 + int(ws.Signal())  // shell convention; UI renders it as-is (UI-SPEC)
    }
    s.markExited(code)   // status, fan out 'x' frame, close conns with 1000
    _ = ptmx.Close()     // after Wait; pump's Read already returned EIO
    close(s.done)
}()

// Stop (D-14): SIGTERM pgroup → 5s → SIGKILL pgroup
func (s *Session) Stop() {
    pgid := s.cmd.Process.Pid
    _ = syscall.Kill(-pgid, syscall.SIGTERM)   // ESRCH ignored
    select {
    case <-s.done:
    case <-time.After(5 * time.Second):
        _ = syscall.Kill(-pgid, syscall.SIGKILL)
        <-s.done
    }
}
```

`syscall.Kill(-pgid, ...)` signals every remaining member of the group even after the leader died (ESRCH only when the group is empty). Verification test: after `Stop()`, `syscall.Kill(-pgid, 0)` returns ESRCH (or `pgrep -g <pgid>` is empty) — including a test case where bash spawned a background `sleep 300`.

Known limit (acceptable, same as every terminal multiplexer): a child that itself calls `setsid()` escapes the group and can't be caught.

### Pattern 3: Security — loopback bind, Host, Origin (D-20/D-21, TERM-07)

**Loopback bind enforcement (D-21)** — in `main.go` right after `flag.Parse()`:

```go
func ensureLoopback(addr string) error {
    host, _, err := net.SplitHostPort(addr)
    if err != nil { return fmt.Errorf("invalid --addr %q: %w", addr, err) }
    if host == "localhost" { return nil }
    ip := net.ParseIP(host)
    if ip == nil || !ip.IsLoopback() {
        return fmt.Errorf("--addr %q is not a loopback address; kangent serves a shell and must stay local", addr)
    }
    return nil
}
```

Empty host (`:7333` → all interfaces) yields `host == ""` → `ParseIP` nil → refused. Non-"localhost" hostnames are refused outright (don't resolve them — `/etc/hosts` games shouldn't widen the bind).

**Host validation (DNS-rebinding defense)** — middleware wrapping the entire mux: strip the port from `r.Host` and require the hostname ∈ {`localhost`, `127.0.0.1`, `::1`/`[::1]`}. **Hostname-only, port-agnostic** — rebinding attacks present an attacker-controlled hostname, never a loopback name; port-agnosticism is what keeps the Vite dev proxy working (it forwards `Host: 127.0.0.1:5173`).

**Origin validation (CSWSH defense)** — exact-origin allowlist per D-20, built at startup:

```go
allowed := []string{"127.0.0.1:" + port, "localhost:" + port} // host patterns for OriginPatterns
allowed = append(allowed, devOriginHosts...)                   // from --dev-origin flag
conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
    OriginPatterns: allowed, // matched case-insensitively against the Origin's host (verified)
    // InsecureSkipVerify stays false (default)
})
```

Verified semantics (pkg.go.dev): with `OriginPatterns` set, patterns match the Origin **host** (include `://` in a pattern to also match scheme); the request's own Host is always authorized; requests with **no** Origin header are allowed (non-browser clients — fine, the threat is browsers, which always send Origin on WS handshakes per RFC 6455).

**The dev-proxy wrinkle (must be planned for):** in dev the browser's origin is the Vite server (`http://localhost:5173`), so the Go server sees `Origin: http://localhost:5173` on proxied WS upgrades. Without action, dev terminal connections are rejected. Resolution: a repeatable `--dev-origin` flag (e.g., `--dev-origin localhost:5173 --dev-origin 127.0.0.1:5173`) appended to the allowlist; production runs never pass it, keeping D-20's exact-port letter intact. Additionally `web/vite.config.ts` must change from string shorthand to `"/api": { target: "http://127.0.0.1:7333", ws: true }` — **the current config does not proxy WebSockets at all**.

Verification (TERM-07 test): httptest server + WS dial with `Origin: http://evil.example` → upgrade rejected; `Host: evil.example` on a REST request → rejected; correct loopback origin → accepted.

### Pattern 4: React TerminalPane lifecycle (StrictMode-safe)

`web/src/main.tsx` renders in `<StrictMode>` — dev effects run setup → cleanup → setup. Rules:

- **One effect keyed on `sessionId`** creates everything (Terminal, FitAddon, WebGL addon, ResizeObserver, WS) and the cleanup destroys everything (`ws.close()` — safe even in CONNECTING state; `ro.disconnect()`; `term.dispose()`; set a `disposed` ref so the `onclose` handler doesn't schedule a reconnect). A complete cleanup makes the double-mount invisible: the server just sees attach→detach→attach, which the manager treats as routine.
- **Never spawn a session in an effect** — double-mount would double-spawn. Spawn only from the "New terminal" click via a TanStack Query mutation (which then selects the new session → pane attaches by prop change).
- Session rail: `useQuery({queryKey: ["sessions"], queryFn: ...})` + invalidate on spawn/stop/close mutations and on receipt of an `'x'` frame in the attached pane; add `refetchInterval: 5000` so non-attached rows' status stays honest. Plain TanStack Query — no zustand needed this phase.
- Phase 3 portability (UI-SPEC layout contract): `TerminalPane` props = `sessionId` only; rail and page header are route-level composition in `TerminalPage.tsx`.

### Pattern 5: xterm.js 6 configuration (D-17/18/19)

Verified against xterm.js master typings (2026-06-10): **there is no `copyOnSelect` option** — implement it manually; `attachCustomKeyEventHandler`, `paste(data: string)`, `reset()`, `write(string | Uint8Array)`, `onSelectionChange`, `getSelection/hasSelection` all exist as expected; theme keys `selectionBackground`/`selectionForeground` and the 16 ANSI names match UI-SPEC's table.

```ts
const term = new Terminal({
  fontFamily: 'ui-monospace, "SF Mono", "Cascadia Mono", Menlo, Consolas, monospace',
  // literal stack — a CSS var() does NOT resolve inside the renderer
  fontSize: 13,            // D-19 / UI-SPEC
  fontWeight: 400, fontWeightBold: 700,
  scrollback: 10000,       // D-18 (default is only 1000)
  cursorBlink: true,
  theme: zincTheme,        // exact values from 02-UI-SPEC.md "xterm Theme" table — copy verbatim
  // do NOT set ignoreBracketedPasteMode — default false means xterm wraps pastes in
  // \x1b[200~ ... \x1b[201~ whenever the app enabled CSI ?2004h; onData carries it through (D-17)
});
term.open(el);
const fit = new FitAddon(); term.loadAddon(fit);
try {
  const webgl = new WebglAddon();
  webgl.onContextLoss(() => webgl.dispose());   // dispose → falls back to DOM renderer
  term.loadAddon(webgl);                        // after open()
} catch { /* WebGL unavailable → DOM renderer, visual parity required (UI-SPEC) */ }

// Copy-on-select (D-17) — manual, no option exists:
term.onSelectionChange(() => {
  if (term.hasSelection()) navigator.clipboard.writeText(term.getSelection());
});
// 127.0.0.1/localhost are secure contexts, so navigator.clipboard is available.

// Ctrl+Shift+C / Ctrl+Shift+V (D-17):
term.attachCustomKeyEventHandler((e) => {
  if (e.type !== "keydown") return true;
  if (e.ctrlKey && e.shiftKey && e.code === "KeyC" && term.hasSelection()) {
    navigator.clipboard.writeText(term.getSelection()); e.preventDefault(); return false;
  }
  if (e.ctrlKey && e.shiftKey && e.code === "KeyV") {
    navigator.clipboard.readText().then((t) => term.paste(t)); e.preventDefault(); return false;
  }
  return true; // everything else (incl. Esc, Ctrl+B) goes to the PTY
});
```

Resize flow: `ResizeObserver` on the pane element → debounce ~75ms → guard `clientWidth > 0 && clientHeight > 0` → `fit.fit()` → send `'1'` frame only when cols/rows actually changed since last send. The `/terminal` pane is always visible (no hidden-tab hazard this phase), but the guard ships anyway because Phase 3 mounts the pane inside `TaskTabs` where hidden tabs are real.

Exited state (D-15): on `'x'` frame, stop forwarding `onData`, set `term.options.disableStdin = true`, show the docked banner; the Terminal instance stays alive so scrollback remains scrollable/copyable, and the canvas is never dimmed (UI-SPEC).

### Anti-Patterns to Avoid

- **PTY lifetime tied to the WS handler** (`defer kill` on socket close) — silently violates TERM-05. Only `Stop()` and process exit end a session.
- **Text WS frames / string round-trips for PTY bytes** — UTF-8 splitting corrupts output. Binary frames, `Uint8Array` straight into `term.write`.
- **`@xterm/addon-attach`** — no framing/control channel; hand-roll the ~30-line glue.
- **Same-size Setsize as the "redraw nudge"** — verified no-op; jiggle through a real size change.
- **Spawning sessions in useEffect** — StrictMode double-spawns; spawn only on explicit user action.
- **Relying on coder/websocket's default origin check alone** — it authorizes Origin==Host, which a DNS-rebinding page satisfies (`Origin: http://attacker.com:7333` + `Host: attacker.com:7333`). The loopback Host middleware + explicit OriginPatterns close this.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| PTY allocation / controlling terminal | forkpty ioctl plumbing | `creack/pty` `Start`/`StartWithSize`/`Setsize` | TIOCSCTTY/session-leader setup is platform-fiddly; the library does it correctly (verified docs) |
| WS handshake, close codes, concurrency | raw `net/http` hijack + frame codec | `coder/websocket` | RFC 6455 framing, masked reads, close handshake, ping/pong handled; concurrent-write safe |
| Ring buffer | — (or accept a 40-line hand-roll) | `armon/circbuf` | Proven in Coder's reconnectingpty; chronological `Bytes()` verified |
| Terminal emulation, scrollback, selection, bracketed paste | anything | `@xterm/xterm` 6 | Its parser correctly handles partial UTF-8/escape sequences across `write()` calls |
| cols/rows measurement | font metrics math | `@xterm/addon-fit` | Cell-size measurement is renderer-dependent |
| WS↔xterm glue | `@xterm/addon-attach` | **DO hand-roll** (~30 lines) | Inverse case: the addon can't carry the typed protocol (resize/exit frames, replay) |

**Key insight:** everything byte-handling is solved by the locked stack; the genuinely custom code is small and structural — the session manager (~250 lines), the framing handler (~150 lines), and the React hook (~150 lines).

## Common Pitfalls

### Pitfall 1: Same-size resize delivers no SIGWINCH
**What goes wrong:** Reattach replay looks stale/garbled; the "resize nudge" does nothing because the client's fitted size equals the PTY's existing size.
**Why:** Kernel sends SIGWINCH only when winsize *changes* (man7 TIOCSWINSZ, verified).
**Avoid:** Server-side jiggle on first post-attach resize when sizes are equal (rows−1 → rows). Exactly once per attach.
**Warning signs:** Reattach to a mid-`htop` session shows frozen output until the window is manually resized.

### Pitfall 2: coder/websocket 32 KiB default read limit kills big pastes
**What goes wrong:** A multi-hundred-line paste closes the WS with StatusMessageTooBig; the UI sees a phantom "connection lost".
**Avoid:** `conn.SetReadLimit(1 << 20)` after Accept.
**Warning signs:** Reconnect banner appearing precisely on large pastes.

### Pitfall 3: StrictMode double-mount churn
**What goes wrong:** In dev, the terminal effect runs twice; incomplete cleanup leaks a WS (server holds a ghost conn) or a Terminal (duplicate canvases stacked in the DOM).
**Avoid:** Single effect, exhaustive cleanup, `disposed` ref gating reconnect scheduling. Never spawn in effects.
**Warning signs:** Two cursors / doubled output in dev only; server logs two attaches per pane open.

### Pitfall 4: Vite proxy doesn't forward WebSockets
**What goes wrong:** Terminal works in the embedded production build but the WS 404s/hangs under `npm run dev`.
**Why:** Current `vite.config.ts` uses string shorthand — no `ws: true`.
**Avoid:** `proxy: { "/api": { target: "http://127.0.0.1:7333", ws: true } }`, and run the Go server with `--dev-origin localhost:5173 --dev-origin 127.0.0.1:5173`.

### Pitfall 5: Treating PTY read EOF/EIO as the exit event
**What goes wrong:** On Linux, the master read fails with EIO (not EOF) when the child exits; using the read error as the exit signal races `Wait()` and loses the exit code.
**Avoid:** Pump treats *any* read error as end-of-stream and just returns; the `Wait()` goroutine is the single authoritative exit event (status, exit code, `'x'` fan-out). Close `ptmx` after `Wait()` returns.

### Pitfall 6: Replay/live-stream gap on attach
**What goes wrong:** Output emitted between "snapshot taken" and "connection subscribed" is lost (or duplicated if subscribed first).
**Avoid:** Snapshot + subscribe in one mutex-held critical section, with the snapshot queued as the connection's first write (Pattern 1 code).

### Pitfall 7: Fan-out blocked by one slow connection
**What goes wrong:** A stalled browser tab blocks the pump goroutine; *every* viewer of that session freezes, and PTY reads stop.
**Avoid:** Per-conn buffered channel + non-blocking send; on overflow, drop that conn (it auto-reconnects with fresh replay). Full ttyd-style PAUSE/RESUME flow control is deliberately deferred — the protocol reserves `'2'`/`'3'` if Phase 4's output volume demands it.

### Pitfall 8: Clipboard API edge cases
**What goes wrong:** Ctrl+Shift+C collides with Chrome's "inspect element" shortcut (may open DevTools regardless of `preventDefault`); Firefox gates `clipboard.readText()` behind a paste prompt/user-activation.
**Avoid:** Treat copy-on-select as the primary copy path (D-17 mandates both anyway) and native Ctrl+V as the always-works paste path (xterm's textarea paste event needs no clipboard API). Ctrl+Shift+C/V are best-effort enhancements; do not block phase verification on browser-shortcut conflicts.

## Code Examples

Key verified signatures (sources: pkg.go.dev, xterm.js master typings — 2026-06-10):

```go
// creack/pty v1.1.24
func pty.Start(cmd *exec.Cmd) (*os.File, error)                       // new session + controlling tty
func pty.StartWithSize(cmd *exec.Cmd, ws *pty.Winsize) (*os.File, error)
func pty.Setsize(t *os.File, ws *pty.Winsize) error                   // Winsize{Rows, Cols, X, Y uint16}

// coder/websocket v1.8.14
func websocket.Accept(w http.ResponseWriter, r *http.Request, opts *websocket.AcceptOptions) (*websocket.Conn, error)
// AcceptOptions{ OriginPatterns []string, InsecureSkipVerify bool, ... }
func (c *Conn) Read(ctx context.Context) (MessageType, []byte, error)  // serialize reads
func (c *Conn) Write(ctx context.Context, typ MessageType, p []byte) error // concurrent-safe
func (c *Conn) Close(code StatusCode, reason string) error
func (c *Conn) SetReadLimit(n int64)                                   // default 32768

// armon/circbuf
func circbuf.NewBuffer(size int64) (*circbuf.Buffer, error)
func (b *Buffer) Write(p []byte) (int, error)   // overwrites oldest
func (b *Buffer) Bytes() []byte                 // chronological; do not mutate
```

```ts
// Client framing glue (~30 lines, replaces addon-attach)
ws.binaryType = "arraybuffer";
ws.onmessage = (ev) => {
  const data = new Uint8Array(ev.data as ArrayBuffer);
  if (data[0] === 0x30) term.write(data.subarray(1));                       // output
  else if (data[0] === 0x78) onExit(JSON.parse(dec.decode(data.subarray(1))).code);
};
term.onData((s) => {
  if (ws.readyState !== WebSocket.OPEN || exited) return;                   // input gating
  const b = enc.encode(s); const f = new Uint8Array(b.length + 1);
  f[0] = 0x30; f.set(b, 1); ws.send(f);
});
const sendResize = (cols: number, rows: number) => {
  const b = enc.encode(JSON.stringify({ cols, rows }));
  const f = new Uint8Array(b.length + 1); f[0] = 0x31; f.set(b, 1); ws.send(f);
};
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| gorilla/websocket + manual write mutex | coder/websocket (concurrent-write safe, context-first) | ~2023 (nhooyr → coder transfer) | Fan-out code needs no write lock; per-conn ordering still needs a single writer goroutine |
| `xterm` unscoped package, canvas renderer | `@xterm/xterm` 6, WebGL + DOM fallback | Dec 2024 (6.0) | Canvas addon gone; `onContextLoss → dispose()` is the documented fallback |
| gotty's base64-in-text-frames | ttyd binary frames + 1-byte prefix | long-standing | Project-locked protocol shape |
| `selection` theme key | `selectionBackground`/`selectionForeground` | xterm 5.x | UI-SPEC already uses current names (verified against typings) |

**Deprecated/outdated:** `creack/pty/v2` (orphaned major — pkg.go.dev still advertises it; do not follow the "latest version" banner), `@xterm/addon-canvas`, `@xterm/addon-attach` for this protocol.

## Open Questions

1. **Ctrl+Shift+C vs Chrome DevTools shortcut**
   - What we know: `attachCustomKeyEventHandler` receives the keydown; whether `preventDefault` suppresses Chrome's inspect-element binding is browser-version-dependent.
   - What's unclear: exact current Chrome behavior.
   - Recommendation: implement it, verify manually during execution; copy-on-select is the guaranteed path. Not a phase blocker.
2. **Firefox `clipboard.readText()` UX for Ctrl+Shift+V**
   - What we know: Firefox shows a paste-confirmation prompt; native Ctrl+V works without any clipboard API.
   - Recommendation: best-effort; document native Ctrl+V as the universal paste.
3. **Alt-screen replay fidelity for Phase 4 (Claude Code TUI)**
   - What we know: a bounded raw ring will eventually evict `CSI ?1049h`; bash-now works with raw replay + jiggle; the seam is `Snapshot()`.
   - Recommendation: already flagged in STATE.md blockers — Phase 4 research evaluates a headless VT emulator behind the same method. Nothing in Phase 2 changes either way.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | backend build | ✓ | 1.26.0 (linux/amd64) | — |
| Node | Vite 8 (needs 20.19+/22.12+) | ✓ | 24.4.1 | — |
| npm | frontend deps | ✓ | 11.6.0 | — |
| /bin/bash | spawned shell | ✓ | 5.2.21 | `$SHELL` env is /bin/bash anyway |
| Linux PTY/pgroup semantics | teardown + EIO handling | ✓ | kernel 6.8 | — (macOS differences noted but not the target) |

**Missing dependencies with no fallback:** none.

## Sources

### Primary (HIGH confidence)
- pkg.go.dev/github.com/coder/websocket — Accept/AcceptOptions/OriginPatterns semantics, concurrency model, 32 KiB read limit, close-code ranges (fetched 2026-06-10)
- pkg.go.dev/github.com/creack/pty — Start/StartWithSize/Setsize/Winsize, "new session + controlling terminal" (fetched 2026-06-10)
- pkg.go.dev/github.com/armon/circbuf — NewBuffer/Write/Bytes chronological semantics (fetched 2026-06-10)
- xterm.js master `typings/xterm.d.ts` — full ITerminalOptions list (no copyOnSelect), ITheme keys, method signatures, `ignoreBracketedPasteMode` (fetched 2026-06-10)
- xterm.js `addons/addon-webgl/README.md` — load-after-open, `onContextLoss → dispose()` (fetched 2026-06-10)
- man7.org TIOCSWINSZ(2const) — SIGWINCH sent only "when the window size changes" (fetched 2026-06-10)
- GitHub releases/tags API — coder/websocket latest = v1.8.14; creack/pty latest = v1.1.24 (2026-06-10)
- Repo inspection — `cmd/kangent/main.go`, `web/src/main.tsx` (StrictMode), `web/vite.config.ts` (no `ws:true`), `web/package.json` (xterm not installed), `internal/api/routes.go`
- `.planning/research/{STACK,ARCHITECTURE,PITFALLS}.md` — all verified 2026-06-10 (same day); treated as current

### Secondary (MEDIUM confidence)
- RFC 6455 origin requirement for browser WS clients (spec knowledge, consistent with coder/websocket docs)
- Chrome/Firefox clipboard-shortcut behaviors (community-reported, flagged in Open Questions)
- anthropics/claude-code#49086 resize-storm bug (via PITFALLS.md) — motivates single-jiggle debounce

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — versions re-verified against registries/releases today; matches same-day project research
- Architecture (manager/protocol/teardown): HIGH — Coder/ttyd patterns + all API signatures verified
- Replay + nudge: HIGH on mechanism (SIGWINCH-on-change verified via man page); MEDIUM on subjective redraw quality for arbitrary TUIs — bash/htop acceptance is the phase bar
- Security model: HIGH — OriginPatterns semantics verified; dev-proxy wrinkle confirmed against actual repo config
- Clipboard shortcuts: MEDIUM — browser-dependent, flagged

**Research date:** 2026-06-10
**Valid until:** ~2026-07-10 (stable Go libs; xterm 6 addon set pinned)
