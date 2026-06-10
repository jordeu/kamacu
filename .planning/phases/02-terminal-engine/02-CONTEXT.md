# Phase 2: Terminal Engine - Context

**Gathered:** 2026-06-10
**Status:** Ready for planning

<domain>
## Phase Boundary

A real PTY-backed shell running server-side inside the existing Go binary, rendered as an interactive terminal in the browser. Covers: session manager owning PTY lifetimes (server-side, independent of WebSocket connections), binary WebSocket bridge, xterm.js terminal component, resize/scrollback/copy-paste, detach/reattach with output replay, full process-tree termination, and WebSocket security (TERM-02, TERM-03, TERM-05, TERM-06, TERM-07).

Proven against plain **bash**. Out of this phase: Claude Code sessions (Phase 4), bash tabs inside the task view (Phase 3, TERM-04), worktree cwds (Phase 3), status detection (Phase 4).

</domain>

<decisions>
## Implementation Decisions

### Surface (where the terminal lives in Phase 2)
- **D-12:** Phase 2 ships a minimal dev route `/terminal` hosting the engine — NOT wired into the task view. Phase 3 promotes the terminal component into the task tab strip (the `TabDef[]` seam) and the dev route can then be dropped or kept as a debug surface.
- **D-13:** The dev route has a "New terminal" action; each spawn gets a session id. The surface lists live/exited sessions and supports attach (click to open), and stop. This deliberately exercises the full engine: multiple sessions, detach/reattach, replay, termination.

### Session Lifecycle
- **D-14:** Stop = SIGTERM to the entire process group, wait ~5 seconds, then SIGKILL anything remaining. One button; no orphaned subprocesses (TERM-06).
- **D-15:** Exited sessions freeze their final output with an overlay banner: "Session exited (code N)" plus actions (New terminal / Close). History stays scrollable/copyable until closed.
- **D-16:** No idle timeouts — sessions live until explicitly stopped or the server process dies. "Leave and reattach" is the core value; single user means resource pressure is acceptable.

### Terminal UX
- **D-17:** Copy-on-select enabled; Ctrl+Shift+C / Ctrl+Shift+V also work; bracketed paste passes through to the PTY (needed for Claude Code multiline prompts in Phase 4).
- **D-18:** Scrollback ~10,000 lines client-side.
- **D-19:** Font: system monospace stack (ui-monospace, ...), 13–14px — consistent with the UI-SPEC no-webfont rule. Terminal theme colors derive from the zinc dark palette.

### Security
- **D-20:** WebSocket + API security is strict Origin/Host validation only — allowlist of loopback origins (127.0.0.1 / localhost with the exact serving port). No per-instance token in v1 (single-user machine assumption; revisit if shared-machine support ever matters).
- **D-21:** The server refuses to start if `--addr` is not a loopback address. Localhost-only is enforced, not aspirational.

### Claude's Discretion
- Ring buffer size for replay-on-reattach and the replay strategy (raw replay + resize nudge vs other approaches — research flagged this as the hard problem; pick what works against bash now without painting Phase 4's alt-screen TUI into a corner)
- WebSocket wire protocol details (binary frames, control message format — follow project research: ttyd-style 1-byte type prefix)
- WS reconnect behavior on transient drops (auto-reconnect with backoff is fine)
- Session naming/labels on the dev route
- Where sessions live in the DB vs memory for this phase (sessions table can wait for Phase 4 if memory-only is cleaner here — but startup reconciliation lands in Phase 5 either way)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project planning
- `.planning/PROJECT.md` — core value: persistent sessions you can open, leave, reattach
- `.planning/REQUIREMENTS.md` — Phase 2 owns TERM-02, TERM-03, TERM-05, TERM-06, TERM-07
- `.planning/ROADMAP.md` — Phase 2 success criteria (5)

### Research (the load-bearing documents for this phase)
- `.planning/research/ARCHITECTURE.md` — session manager owns PTY lifetimes (never the WS handler); Coder reconnectingpty pattern (ring buffer, replay-then-subscribe, activeConns fan-out, post-replay resize nudge); ttyd wire protocol (binary frames, 1-byte type prefix, JSON resize)
- `.planning/research/STACK.md` — creack/pty v1 (NOT v2), coder/websocket (NOT gorilla — concurrent-write safety), @xterm/xterm 6 with WebGL addon + DOM fallback (canvas renderer removed in v6), skip @xterm/addon-attach (hand-roll ~30 lines)
- `.planning/research/PITFALLS.md` — process-group teardown (Setsid + kill(-pgid)), PTY read goroutine + Wait() reaping, explicit env (TERM/PATH/HOME), UTF-8/escape-sequence splitting (binary frames mandatory), CSWSH details, write backpressure
- `.planning/research/SUMMARY.md` — phase mapping and confidence notes

### Existing code (integration points)
- `cmd/kangent/main.go` — mux setup, addr flag (D-21 loopback enforcement lands here)
- `web/src/App.tsx` — router where the `/terminal` dev route mounts
- `web/src/components/task/TaskTabs.tsx` — the `TabDef[]` seam Phase 3 will use; the terminal component built now must be embeddable there later
- `.planning/phases/01-foundation-projects-board/01-UI-SPEC.md` — design tokens the terminal chrome must follow

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Go: `internal/store` (SQLite open discipline), `internal/api` (routes/respond patterns to mirror for any session endpoints), stdlib mux in `cmd/kangent/main.go`
- Frontend: TanStack Query client + api fetch layer (`web/src/api/`), shadcn components, zinc token set in `web/src/index.css`, `TabDef[]` tab seam

### Established Patterns
- REST endpoints under `/api/*` registered via `api.Routes(mux, db)`; JSON respond helpers; table-driven Go tests with httptest
- Frontend data flows through typed hooks; components own their files (wave-parallel discipline)

### Integration Points
- New WS endpoint(s) mount on the same mux (e.g. `/api/sessions/{id}/ws`) — must be excluded from SPA fallback only via `/api/` prefix (already handled)
- `/terminal` dev route added to `web/src/App.tsx` routes under AppLayout
- Terminal React component must be self-contained enough to re-mount inside TaskTabs in Phase 3 (props: session id / ws url)

</code_context>

<specifics>
## Specific Ideas

- The reattach experience to aim for: close the tab mid-`htop`, reopen, and the screen redraws correctly after replay + resize nudge
- Exited-session banner mirrors the UI-SPEC error/empty-state copy style (specific, with a next action)

</specifics>

<deferred>
## Deferred Ideas

- Per-instance auth token (Jupyter-style) — revisit only if shared/multi-user machines become a target (D-20 keeps the door open)
- Keeping `/terminal` as a permanent debug surface after Phase 3 — decide when Phase 3 lands

</deferred>

---

*Phase: 02-terminal-engine*
*Context gathered: 2026-06-10*
