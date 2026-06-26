---
phase: quick-260626-hwd
plan: 01
subsystem: backend (cmd/kangent + internal/ws)
tags: [security, networking, websocket, flag, opt-in]
requires:
  - existing ensureLoopback / hostCheck loopback enforcement (D-21)
  - ws.NewHandler Origin allowlist
  - coder/websocket AcceptOptions.InsecureSkipVerify
provides:
  - "--insecure-allow-remote flag gating all three loopback-enforcement layers + warn banner"
  - "ws.NewHandler third param insecureAnyOrigin (InsecureSkipVerify wiring)"
  - "hookBaseURL wildcard-host normalization for the agent-status hook URL"
affects:
  - cmd/kangent/main.go
  - internal/ws/handler.go
tech-stack:
  added: []
  patterns:
    - "coder/websocket InsecureSkipVerify (not '*' pattern) for any-origin"
    - "single boolean guarding bind gate + hostCheck wrap + WS Origin + banner"
key-files:
  created: []
  modified:
    - cmd/kangent/main.go
    - cmd/kangent/main_test.go
    - internal/ws/handler.go
    - internal/ws/integration_test.go
    - internal/ws/handler_test.go
    - internal/api/agent_integration_test.go
decisions:
  - "Use InsecureSkipVerify:true for any-origin (coder/websocket-blessed), NOT a '*' OriginPattern (accept.go line 51 warns against it)."
  - "SAFE BY DEFAULT: ensureLoopback and hostCheck bodies are untouched; only whether they are invoked is gated. TestEnsureLoopback/TestHostCheck unchanged."
  - "hookBaseURL normalizes ONLY wildcard hosts (0.0.0.0 / :: / empty) to 127.0.0.1:<port>; a specific IP (incl. a reachable non-loopback IP) is left as-is."
  - "--insecure-allow-remote with a still-loopback --addr is a harmless no-op (no error, no second branch)."
metrics:
  duration: 12 min
  completed: 2026-06-26
---

# Quick Task 260626-hwd: Opt-in `--insecure-allow-remote` flag Summary

Added an opt-in `--insecure-allow-remote` boolean flag (default false) that lets kangent bind a non-loopback `--addr` by bypassing all three loopback-enforcement layers (bind gate, hostCheck middleware, WS Origin allowlist), printing one loud `slog.Warn` security banner at startup, and normalizing the agent-status hook BaseURL for wildcard binds — with the default path left byte-for-byte unchanged.

## What Was Built

**Task 1 — `ws.NewHandler` insecureAnyOrigin (commit `040f15d`)**
- `NewHandler` gained a third parameter `insecureAnyOrigin bool`, stored on the `Handler` struct.
- `ServeHTTP` now sets `InsecureSkipVerify: h.insecureAnyOrigin` on the `websocket.AcceptOptions` literal alongside `OriginPatterns`. When true, Origin verification is disabled entirely (the coder/websocket-documented any-origin path — not the `"*"` pattern).
- All three non-production call sites updated to pass `false`: `internal/ws/integration_test.go` (via a parameterized `newTestServer`), `internal/ws/handler_test.go`, `internal/api/agent_integration_test.go`.
- New test `TestIntegrationInsecureAnyOriginAccepted` mirrors `TestIntegrationEvilOriginRejected`: builds the server with `insecureAnyOrigin=true`, dials with `Origin: http://evil.example`, and asserts the upgrade SUCCEEDS. `TestIntegrationEvilOriginRejected` still 403s with the flag false.

**Task 2 — flag, conditional enforcement, banner, hook BaseURL (commit `ce03d84`)**
- Registered `--insecure-allow-remote` bool flag (default false) next to the other flags.
- `ensureLoopback(*addr)` bind gate now runs only when `!*insecureAllowRemote`; the function body is unchanged.
- When the flag is set, a single loud `slog.Warn` banner names the exposure (no auth → remote shell), with `addr` as a structured field.
- The hostCheck wrap is selected at serve time: `hostCheck(mux)` by default, bare `mux` when the flag is set; the `hostCheck` body is unchanged.
- `ws.NewHandler(mgr, originPatterns, *insecureAllowRemote)` threads the flag to the WS Origin layer.
- New `hookBaseURL(addr string) string` helper: normalizes wildcard host (`""`, `0.0.0.0`, `::`/`[::]`) to `127.0.0.1:<port>`, leaves a specific host unchanged, and falls back to `"http://" + addr` on a `SplitHostPort` error. `BaseURL` now uses it.
- Refreshed the (previously slightly stale) comment at the `AgentConfig.BaseURL` site to reflect that `--addr` is no longer unconditionally loopback-enforced.
- Tests: `TestEnsureLoopback` and `TestHostCheck` kept exactly as-is (default-path regression guards); added `TestHookBaseURL` (wildcard-normalization table) and `TestInsecureAllowRemoteRelaxesHostCheck` (replicates main's handler selection: 403 by default, 200 with the flag, no socket bind / no `main()` boot).

## Deviations from Plan

### Auto-fixed / discretionary adjustments

**1. [Optional, plan-sanctioned] Refreshed the stale `AgentConfig.BaseURL` comment**
- **Found during:** Task 2
- **Issue:** The comment at the hook BaseURL site said "addr is loopback-enforced above", which is no longer always true once the flag exists. The plan-checker flagged this as a non-blocking, optional, cosmetic refresh.
- **Fix:** Updated the comment to describe the wildcard-normalization rationale instead.
- **Files modified:** `cmd/kangent/main.go`
- **Commit:** `ce03d84`

No other deviations — the plan executed as written. No Rule 1–4 triggers.

## Verification

- `go build ./... && go vet ./... && go test ./... -count=1` — GREEN (final gate).
- Default-path regression: `TestEnsureLoopback`, `TestHostCheck`, `TestIntegrationEvilOriginRejected` pass UNCHANGED.
- New coverage: `TestHookBaseURL`, `TestInsecureAllowRemoteRelaxesHostCheck`, `TestIntegrationInsecureAnyOriginAccepted` pass.

### Test-flake note (not a defect)

The FIRST full `go test ./...` run reported a single FAIL in `kangent/internal/api` after 92s. Two subsequent runs of that package passed (150s and 123s in isolation/under the final gate). The failure is a load-induced timeout in a heavy PTY/tmux integration suite under full-suite parallelism, NOT caused by this change (no `internal/api` source was modified; only the Task 1 `false`-arg test edit, which compiled and passed in Task 1's scoped run). The final gate run is green.

## Threat Model Outcome

- **T-hwd-01 / T-hwd-02 (accept, only when flag set):** Default path defenses fully intact and regression-guarded; the open-network surface is the user's explicit opt-in, announced by the warn banner.
- **T-hwd-03 (mitigate):** `hookBaseURL` normalizes wildcard binds to loopback so local hooks never curl a wildcard/remote-looking address — covered by `TestHookBaseURL`.
- **T-hwd-SC:** No new dependencies added (InsecureSkipVerify and `net` stdlib already in use).

No new security surface introduced beyond the deliberate, documented escape hatch.

## Known Stubs

None.

## Self-Check: PASSED

- SUMMARY.md present.
- Commits `040f15d` (Task 1) and `ce03d84` (Task 2) exist in git history.
- All six modified files are tracked.
