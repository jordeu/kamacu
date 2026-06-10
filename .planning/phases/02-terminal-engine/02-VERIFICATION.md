---
phase: 02-terminal-engine
verified: 2026-06-10T00:00:00Z
status: passed
score: 5/5 must-haves verified
---

# Phase 2: Terminal Engine Verification Report

**Phase Goal:** User can run a real shell in the browser whose lifetime belongs to the server, not the browser tab
**Verified:** 2026-06-10
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (ROADMAP Success Criteria)

| #   | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| 1   | Interactive terminal running a real PTY-backed shell — typing, full TUI programs, control sequences | ✓ VERIFIED | `pty.StartWithSize` + explicit env (`internal/session/manager.go:55-65`); WS round-trip + integration test `internal/ws/integration_test.go:96-153`; uncached `go test ./internal/session ./internal/ws` passes (4.3s / 49.5s); human approved htop TUI |
| 2   | Terminal resizes to fit pane, scrollback, copy/paste incl. bracketed paste | ✓ VERIFIED | `scrollback: 10000`, `onSelectionChange` copy, `attachCustomKeyEventHandler`, `ResizeObserver` debounce + hidden-tab guard in `TerminalPane.tsx`; resize jiggle in `session.go:219-230`; bracketed paste pass-through (D-17, `ignoreBracketedPasteMode` never set); human approved |
| 3   | Close tab mid-command → session keeps running server-side; reopening reattaches with replay + clean redraw | ✓ VERIFIED | Detach never touches PTY (`handler.go` uses only Attach/Detach); 1 MiB ring (`circbuf.NewBuffer(1 << 20)`); replay-first attach; `term.reset()` on reconnect + forceRedraw jiggle on first resize; integration test asserts detach→list-still-running→reattach-replay (`integration_test.go:140-153`); human approved |
| 4   | Stop from UI terminates entire process tree — zero orphans | ✓ VERIFIED | `syscall.Kill(-pgid, SIGTERM)` → 5s grace → `syscall.Kill(-pgid, SIGKILL)` (`session.go:287-306`); zero-descendants test with `sleep 300 &` asserting ESRCH (`session_test.go:312-328`); signaled exits as 128+signal (`session.go:111`); orchestrator headless check + human approved |
| 5   | WS rejects wrong Origin/Host; server refuses non-loopback bind (token deferred per D-20) | ✓ VERIFIED | `ensureLoopback` called before serve (`main.go:33,109`); `hostCheck(mux)` wraps everything (`main.go:99`); `OriginPatterns` allowlist, no `InsecureSkipVerify` (`handler.go:39`); `TestIntegrationEvilOriginRejected` + table tests; automated checks during execution: 0.0.0.0 bind refused, evil Host → 403, evil Origin → 403 |

**Score:** 5/5 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/session/session.go` | Pump, ring, Attach/Detach, Resize+jiggle, Stop, exit watcher | ✓ VERIFIED | All patterns present; min_lines met; no api/ws/net-http imports (dependency direction enforced) |
| `internal/session/manager.go` | Manager: Spawn/Get/List/Remove | ✓ VERIFIED | `pty.StartWithSize`, explicit env, 1 MiB circbuf ring |
| `internal/session/session_test.go` | Table tests incl. zero-descendants | ✓ VERIFIED | `sleep 300 &` + ESRCH assertion present; suite green uncached |
| `web/src/components/terminal/xtermTheme.ts` | Zinc ITheme + ANSI 16 | ✓ VERIFIED | `selectionBackground: "#3f3f46"`; `selectionForeground` unset; 21 hex values |
| `web/src/api/sessions.ts` | TermSession + 4 hooks | ✓ VERIFIED | useSessions/useSpawnSession/useStopSession/useDeleteSession exported; `refetchInterval: 5000` |
| `web/vite.config.ts` | WS-capable dev proxy | ✓ VERIFIED | `ws: true` on `/api` proxy |
| `internal/ws/proto.go` | Frame constants + 4404 | ✓ VERIFIED | `CloseSessionNotFound = 4404` |
| `internal/ws/handler.go` | Accept, read limit, single writer | ✓ VERIFIED | `SetReadLimit(1 << 20)`, `MessageBinary` only (no MessageText anywhere in package) |
| `internal/api/sessions.go` | REST endpoints, {error} contract | ✓ VERIFIED | `SessionRoutes` with exactly 4 routes; `writeError` reused |
| `cmd/kangent/main.go` | ensureLoopback, hostCheck, --dev-origin, wiring | ✓ VERIFIED | All present incl. `api.SessionRoutes(mux, mgr)` and `GET /api/sessions/{id}/ws` route |
| `web/src/components/terminal/useTerminalSocket.ts` | Protocol + reconnect machine | ✓ VERIFIED | 0x30/0x31/0x78 handled; backoff `[500, 1000, 2000, 4000, 8000]`; `term.reset()`; `disposed` gate; 4404 → not-found, no retry |
| `web/src/components/terminal/TerminalPane.tsx` | Self-contained pane + banners | ✓ VERIFIED | WebGL `onContextLoss` fallback; all UI-SPEC banner copy verbatim; `disableStdin` toggling; no useParams/useNavigate/useSpawnSession (D-12 portability) |
| `web/src/pages/TerminalPage.tsx` | Dev route: rail + pane composition | ✓ VERIFIED | `w-[220px]` rail; all empty-state copy verbatim; spawn only from click handler |
| `web/src/App.tsx` | /terminal route | ✓ VERIFIED | `path="/terminal"` under AppLayout (line 47) |
| `internal/ws/integration_test.go` | E2E through real route table | ✓ VERIFIED | `TestIntegration` full lifecycle + `TestIntegrationEvilOriginRejected`; uses `api.SessionRoutes` (production route shape) |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | --- | --- | ------ | ------- |
| session.go | circbuf ring | pump writes every read | ✓ WIRED | `s.ring.Write(chunk)` at session.go:84 (tool regex false-negative; verified manually) |
| session.go | process group | Stop signals -pgid | ✓ WIRED | session.go:287,303 (SIGTERM + SIGKILL paths) |
| session.go | cmd.Wait() | single authoritative exit | ✓ WIRED | session.go:108 in waitExit goroutine |
| ws/handler.go | internal/session | Attach/Detach only, never the ring | ✓ WIRED | `sess.Attach(connID)` at handler.go:57; no direct ring/Snapshot access |
| main.go | ensureLoopback | called before ListenAndServe | ✓ WIRED | main.go:33 |
| ws/handler.go | websocket.AcceptOptions | OriginPatterns allowlist | ✓ WIRED | handler.go:39 |
| sessions.ts | /api/sessions | client helpers | ✓ WIRED | tool-verified |
| package.json | @xterm/xterm | pinned 6.0.0/0.11.0/0.19.0 | ✓ WIRED | exact pins, no unscoped xterm |
| useTerminalSocket.ts | /api/sessions/{id}/ws | binaryType arraybuffer | ✓ WIRED | tool-verified |
| TerminalPane.tsx | xtermTheme.ts | zincTheme import | ✓ WIRED | tool-verified |
| TerminalPane.tsx | term.onData | input gated when exited | ✓ WIRED | tool-verified |
| App.tsx | TerminalPage | /terminal route | ✓ WIRED | tool-verified |
| TerminalPage.tsx | TerminalPane | sessionId from rail selection | ✓ WIRED | tool-verified |
| TerminalPage.tsx | useSpawnSession | click handler only | ✓ WIRED | tool-verified |

Note: 5 key-links reported "failed" by `gsd-tools verify key-links` due to regex double-escaping in the tool ("Invalid regex pattern"); all 5 verified manually with grep and confirmed WIRED.

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| TerminalPage.tsx | sessions (useSessions) | GET /api/sessions → `mgr.List()` | Yes — live Manager state, `[]` only when truly empty | ✓ FLOWING |
| TerminalPane.tsx | props at call site | `selected.id/label/status/exitCode` from query data | Yes — no hardcoded props | ✓ FLOWING |
| TerminalPane viewport | terminal output | WS '0' frames ← PTY pump ← real bash | Yes — proven by integration test echo round-trip | ✓ FLOWING |
| api/sessions.go list | infos | `h.mgr.List()` | Yes — real manager query, not static return | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Backend builds | `go build ./...` | exit 0 | ✓ PASS |
| Full Go suite | `go test ./internal/... ./cmd/...` | all ok | ✓ PASS |
| Session + WS suites uncached (real PTYs, real WS dials) | `go test ./internal/session/ ./internal/ws/ -count=1` | ok 4.267s / ok 49.489s | ✓ PASS |
| Frontend production build | `npm run build` (web/) | exit 0 (chunk-size warning only) | ✓ PASS |
| Live UX (htop, detach/reattach, Stop, banners, security curls) | human checkpoint (plan 02-05 Task 3) | APPROVED by user | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan(s) | Description | Status | Evidence |
| ----------- | -------------- | ----------- | ------ | -------- |
| TERM-02 | 02-01, 02-03, 02-04, 02-05 | Real interactive terminal in the browser (full TUI) | ✓ SATISFIED | PTY spawn + WS byte path + xterm pane; human approved htop |
| TERM-03 | 02-02, 02-04, 02-05 | Resize, scrollback, copy/paste incl. bracketed paste | ✓ SATISFIED | fit/ResizeObserver, scrollback 10000, copy-on-select, D-17 paste pass-through; human approved |
| TERM-05 | 02-01, 02-03, 02-04, 02-05 | Sessions outlive the tab; reattach with replay + clean redraw | ✓ SATISFIED | Detach leaves PTY; ring replay-first; jiggle redraw; integration test + human approved |
| TERM-06 | 02-01, 02-05 | Stop fully terminates process tree, zero orphans | ✓ SATISFIED | -pgid SIGTERM→SIGKILL; ESRCH zero-descendants test; orchestrator headless check (backgrounded sleep → zero orphans) |
| TERM-07 | 02-03, 02-05 | Origin/Host validation (CSWSH protection) | ✓ SATISFIED | OriginPatterns + hostCheck + ensureLoopback; evil Origin/Host → 403 proven by tests and curls |

No orphaned requirements: REQUIREMENTS.md maps exactly TERM-02/03/05/06/07 to Phase 2 (all marked Complete); every ID appears in at least one plan's `requirements` field. Per locked decision D-20, no per-instance token is expected (deferred) — ROADMAP criterion 5 reflects this.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| — | — | none | — | No TODO/FIXME/placeholder/not-implemented markers in any phase file; no `MessageText`, no `InsecureSkipVerify`, no unscoped xterm, no gorilla/websocket |

### Human Verification Required

None outstanding. The blocking human-verify checkpoint in plan 02-05 was walked and APPROVED (htop TUI, resize/scrollback/clipboard/bracketed paste, detach/reattach with clean redraw, Stop with zero orphans, reconnect banner flow), and the security criteria were verified with live curls during execution.

### Gaps Summary

No gaps. All five ROADMAP success criteria are backed by substantive, wired, data-flowing implementations; the server-side lifecycle is machine-verified end-to-end through the production route shape (`TestIntegration`), and the user-facing experience was human-approved.

---

_Verified: 2026-06-10_
_Verifier: Claude (gsd-verifier)_
