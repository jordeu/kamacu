---
phase: 02-terminal-engine
plan: 03
subsystem: terminal
tags: [go, websocket, coder-websocket, rest, security, cswsh, dns-rebinding, tdd]

# Dependency graph
requires:
  - phase: 02-terminal-engine (plan 02-01)
    provides: internal/session Manager + Session public surface (Spawn/Get/List/Remove, Attach/Detach/WriteInput/Resize/Stop/Done/Info, ErrNotFound/ErrStillRunning)
  - phase: 01-foundation
    provides: internal/api respond helpers ({"error"} contract), stdlib ServeMux routing, cmd/kangent/main.go server skeleton
provides:
  - internal/ws — ttyd-style binary WS protocol ('0' data, '1' resize, 'x' exit, close 4404) with Accept-first upgrade, 1 MiB read limit, single-writer-per-conn ordering
  - internal/api/sessions.go — SessionRoutes(mux, mgr): GET/POST /api/sessions, POST stop (202 async, idempotent), DELETE (409 while running)
  - cmd/kangent hardening — ensureLoopback startup refusal (D-21), hostCheck middleware on the whole mux (DNS-rebinding), --dev-origin flag extending the exact-port Origin allowlist (D-20)
affects: [02-04, 02-05, phase-03, phase-04-claude-sessions]

# Tech tracking
tech-stack:
  added: [github.com/coder/websocket v1.8.14]
  patterns: [Accept-first-then-app-close-code for WS error signaling, single writer goroutine draining the attach queue, once-per-attach forceRedraw flag in the read loop, behavioral SIGWINCH tests via WINCH trap in a foreground non-interactive bash child]

key-files:
  created:
    - internal/ws/proto.go
    - internal/ws/handler.go
    - internal/ws/handler_test.go
    - internal/api/sessions.go
    - internal/api/sessions_test.go
    - cmd/kangent/main_test.go
  modified:
    - cmd/kangent/main.go
    - go.mod
    - go.sum

key-decisions:
  - "Closed attach queue is the writer's exit signal: session exited → 'x' frame + 1000; otherwise it's a slow-consumer drop → close 1013 so the client reconnects with fresh replay"
  - "Jiggle/SIGWINCH behavior tested behaviorally (not via test hooks): interactive bash defers WINCH traps, so tests trap inside a foreground non-interactive bash -c child with READY/DONE sync markers"
  - "hostCheck allows hostnames {localhost, 127.0.0.1, ::1, [::1]} port-agnostically; ensureLoopback accepts any IsLoopback IP (incl. 127.0.0.2) but refuses all non-localhost hostnames unresolved"

patterns-established:
  - "WS frame bytes are shared constants in internal/ws/proto.go — plan 02-04's client mirrors FrameData/FrameResize/FrameExit/4404 exactly"
  - "Session endpoints register via api.SessionRoutes(mux, mgr) — separate from api.Routes(mux, db); Phase 4 adds task identity inside the same handlers"
  - "Dev workflow: go run ./cmd/kangent --dev-origin localhost:5173 --dev-origin 127.0.0.1:5173 (browser Origin is the Vite server under npm run dev)"

requirements-completed: [TERM-02, TERM-05, TERM-07]

# Metrics
duration: 20min
completed: 2026-06-10
---

# Phase 2 Plan 03: WS Bridge, Session REST, and Security Hardening Summary

**ttyd-style binary WebSocket bridge (input/output/replay/resize-jiggle/exit frames) over coder/websocket, session REST endpoints, and the full TERM-07 model: loopback-bind refusal, Host middleware, exact-origin allowlist with --dev-origin escape hatch — all proven headlessly with httptest + WS dials**

## Performance

- **Duration:** 20 min
- **Started:** 2026-06-10T10:32:53Z
- **Completed:** 2026-06-10T10:53:01Z
- **Tasks:** 3 (all TDD: red + green commits each)
- **Files modified:** 9

## Accomplishments

- Full byte path proven headlessly: '0' input reaches bash, output returns as '0' frames; reattach delivers the ring replay as the FIRST data frame, then the live stream continues
- Once-per-attach jiggle contract proven behaviorally: the first equal-size resize after attach delivers SIGWINCH (observed via a WINCH trap), and a second equal-size resize provably does NOT (debounce against resize storms)
- Session exit fans out the 'x' frame with {"code":N} then a clean 1000 close; unknown session ids complete the upgrade and close with 4404
- CSWSH and DNS-rebinding closed by tests: Origin http://evil.example never reaches 101 (403), Host evil.example gets 403 before any handler, and ":7333"/"0.0.0.0"/LAN addrs refuse startup
- REST surface matches the research contract exactly: list ([] when empty), create 201, stop 202 idempotent-async, delete 404/409/204 — all error bodies on the Phase 1 {"error"} contract
- 13 new tests (7 ws + 4 api + 2 table-driven main) pass under -race; routes.go untouched

## Task Commits

Each task was committed atomically (TDD: test → feat):

1. **Task 1: internal/ws protocol constants and connection handler**
   - RED: `6b11455` (test) — round trip, replay, resize, jiggle debounce, exit, 4404, evil-origin
   - GREEN: `cc7ecca` (feat) — Accept-first handler, writer goroutine, read loop, jiggle flag
2. **Task 2: Session REST endpoints**
   - RED: `6a8e59b` (test) — list/create/stop/delete contract tests
   - GREEN: `1d26c52` (feat) — SessionRoutes + handlers on the {"error"} contract
3. **Task 3: main.go hardening**
   - RED: `66f5c19` (test) — ensureLoopback + hostCheck table tests
   - GREEN: `bc4fe8d` (feat) — loopback refusal, Host middleware, --dev-origin, wiring

## Files Created/Modified

- `internal/ws/proto.go` — shared frame-type constants ('0'/'1'/'x') and close code 4404; the 02-04 client mirrors these bytes
- `internal/ws/handler.go` — Accept-first upgrade with OriginPatterns, SetReadLimit(1<<20), single writer goroutine draining the attach queue, exit-frame fan-out, read loop with once-per-attach forceRedraw
- `internal/ws/handler_test.go` — 7 behavioral tests over httptest + websocket.Dial against real bash sessions
- `internal/api/sessions.go` — SessionRoutes(mux, mgr) with list/create/stop/delete handlers
- `internal/api/sessions_test.go` — 4 tests incl. exited-session delete via WriteInput("exit\n") + Done()
- `cmd/kangent/main.go` — ensureLoopback (called immediately after flag.Parse), hostCheck wrapping the whole mux, --dev-origin flag.Func, manager + routes + WS wiring
- `cmd/kangent/main_test.go` — table tests for ensureLoopback (10 cases) and hostCheck (9 cases)
- `go.mod` / `go.sum` — coder/websocket v1.8.14 pinned (re-added as 02-01's summary anticipated)

## Decisions Made

- **Writer exit signaling via the closed attach queue** — when the queue closes, the writer checks session status: exited → wait Done(), send 'x' + 1000; still running → it was a slow-consumer drop by the pump → close 1013 (try again later) so the client reconnects with a fresh replay
- **Behavioral SIGWINCH tests instead of test hooks** — the plan offered a fake/recorded-session fallback; instead the jiggle is proven end-to-end by observing actual SIGWINCH delivery (see Issues for the bash subtlety)
- **127.0.0.2 accepted by ensureLoopback** — any IsLoopback() IP passes, per the research Pattern 3 code; the hostname allowlist in hostCheck stays exact

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- **Interactive bash never ran the WINCH trap** in the first jiggle-test design: interactive bash reserves SIGWINCH for its own line-editing/checkwinsize machinery, so `trap ... WINCH` at the prompt (or around a `read`) never echoed the marker even though the jiggle's SIGWINCH demonstrably interrupted the read. Fix: install the trap inside a foreground NON-interactive `bash -c` child (the recipient of the PTY's SIGWINCH), verified empirically against a raw PTY before adopting.
- **Inter-phase input swallowing in the two-phase jiggle test**: the trap echo arrives before the child's `read` aborts, so input sent immediately after seeing the marker became the `read`'s stdin and the next command never executed. Fix: READY markers printed after trap installation (race-free — once the trap is installed a SIGWINCH can never be lost) and a DONE marker after the read, synced on before phase 2.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Plan 02-04 (React TerminalPane) can mirror internal/ws/proto.go bytes exactly: '0' 0x30 data both ways, '1' 0x31 resize JSON {"cols","rows"}, 'x' 0x78 exit JSON {"code"}, close 4404 = skip reconnect
- Dev runs need `go run ./cmd/kangent --dev-origin localhost:5173 --dev-origin 127.0.0.1:5173` — the browser's Origin under npm run dev is the Vite server (and vite.config.ts still needs `ws: true` on the /api proxy, owned by the frontend plan)
- REST endpoints live at /api/sessions (list/create), /api/sessions/{id}/stop, DELETE /api/sessions/{id}, WS at GET /api/sessions/{id}/ws
- '2'/'3' frame bytes remain reserved (ttyd PAUSE/RESUME) if Phase 4 output volume demands flow control

---
*Phase: 02-terminal-engine*
*Completed: 2026-06-10*

## Self-Check: PASSED

- All 6 created files exist on disk
- All 6 task commits (6b11455, cc7ecca, 6a8e59b, 1d26c52, 66f5c19, bc4fe8d) present in git log
- `go test ./... -timeout 180s` passes repo-wide; `go vet ./...` clean; no gorilla/websocket imports; no stub patterns
