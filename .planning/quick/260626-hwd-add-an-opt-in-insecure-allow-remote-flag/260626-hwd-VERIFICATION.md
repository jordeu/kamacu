---
phase: quick-260626-hwd
verified: 2026-06-26T13:30:00Z
status: passed
score: 6/6 must-haves verified
overrides_applied: 0
---

# Quick Task 260626-hwd: Opt-in `--insecure-allow-remote` flag Verification Report

**Task Goal:** Add an opt-in `--insecure-allow-remote` flag that lets kangent bind to a non-loopback address, bypassing all three loopback-enforcement layers (ensureLoopback bind gate, hostCheck Host-header middleware, WS Origin allowlist), printing a loud no-auth startup warning, and normalizing the agent-status hook BaseURL for wildcard binds — with the SAFE-BY-DEFAULT invariant that, with the flag absent, behavior is byte-for-byte unchanged.

**Verified:** 2026-06-26T13:30:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Default path byte-for-byte unchanged: WITHOUT the flag, ensureLoopback rejects non-loopback `--addr` (os.Exit(1)), hostCheck 403s non-loopback Host, WS Origin allowlist stays loopback-only | ✓ VERIFIED | `main.go:49-53` gates `ensureLoopback` behind `!*insecureAllowRemote` with `os.Exit(1)`. md5 of `ensureLoopback` body identical pre/post (`ad55aead...`); `hostCheck` body identical pre/post (`9c03837d...`). `main.go:212-214` selects `hostCheck(mux)` by default. `handler.go:50` sets `InsecureSkipVerify: h.insecureAnyOrigin` (false → allowlist active). `TestEnsureLoopback`, `TestHostCheck`, `TestIntegrationEvilOriginRejected` PASS. The only deletions in the diff are wiring call-sites, never enforcement logic. |
| 2 | WITH the flag, a non-loopback `--addr` is permitted (ensureLoopback bypassed, process starts) | ✓ VERIFIED | `main.go:49` `if !*insecureAllowRemote { ensureLoopback... }` — the gate is skipped entirely when the flag is set; no error/exit branch on the insecure path. |
| 3 | WITH the flag, a non-loopback Host header is served (hostCheck pass-through, no 403) | ✓ VERIFIED | `main.go:212-215`: `var handler http.Handler = hostCheck(mux); if *insecureAllowRemote { handler = mux }` then `ListenAndServe(*addr, handler)`. `TestInsecureAllowRemoteRelaxesHostCheck` PASS: flag=false → 403, flag=true → 200 for Host `192.168.1.5:7333`. |
| 4 | WITH the flag, a cross-origin WS upgrade is accepted (Origin verification skipped via InsecureSkipVerify) | ✓ VERIFIED | `main.go:156` passes `*insecureAllowRemote` to `ws.NewHandler`; `handler.go:50` sets `InsecureSkipVerify: h.insecureAnyOrigin` (the field, not a literal). `TestIntegrationInsecureAnyOriginAccepted` PASS (cross-origin `http://evil.example` reaches 101). |
| 5 | WITH the flag, a LOUD slog.Warn security banner prints at startup naming the exposure | ✓ VERIFIED | `main.go:57`: single `slog.Warn("SECURITY: kangent is listening with NO authentication via --insecure-allow-remote — anyone who can reach this address gets a shell on this host", "addr", *addr)` in the `else` branch of the bind gate (insecure path only). |
| 6 | Hook BaseURL loopback-correct: wildcard host (0.0.0.0 / :: / empty) → 127.0.0.1:<port>; specific non-loopback IP left as-is | ✓ VERIFIED | `main.go:312-323` `hookBaseURL` switches on host: `"", "0.0.0.0", "::", "[::]"` → `http://127.0.0.1:<port>`, default → `http://`+addr, SplitHostPort error → `http://`+addr fallback. `main.go:138` `BaseURL: hookBaseURL(*addr)`. `TestHookBaseURL` PASS for all 6 cases incl. `192.168.1.5:7333` unchanged. |

**Score:** 6/6 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `cmd/kangent/main.go` | flag + conditional ensureLoopback/hostCheck + warn banner + hook BaseURL normalization | ✓ VERIFIED | `insecure-allow-remote` flag at :42; conditional gate :49-58; banner :57; handler selection :212-214; `hookBaseURL` :312-323 wired at :138; NewHandler call :156. |
| `internal/ws/handler.go` | NewHandler accepts insecureAnyOrigin; sets InsecureSkipVerify when true | ✓ VERIFIED | Struct field `insecureAnyOrigin bool` :21; param :35; `InsecureSkipVerify: h.insecureAnyOrigin` :50. |
| `cmd/kangent/main_test.go` | Retained default-path assertions + new flag coverage | ✓ VERIFIED | `TestEnsureLoopback` + `TestHostCheck` unchanged (md5-confirmed bodies); new `TestHookBaseURL` + `TestInsecureAllowRemoteRelaxesHostCheck`. All PASS. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| main.go `--insecure-allow-remote` flag | ensureLoopback gate, hostCheck wrap, ws.NewHandler | single boolean `*insecureAllowRemote` guarding all three layers + banner | ✓ WIRED | `*insecureAllowRemote` referenced at :49 (gate), :156 (NewHandler), :213 (hostCheck wrap), :54/:57 (banner). Single source flag. |
| main.go hook BaseURL | session.AgentConfig.BaseURL | hookBaseURL normalizing wildcard host | ✓ WIRED | `BaseURL: hookBaseURL(*addr)` at :138 inside `SetAgentConfig`. |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Compiles | `go build ./...` | clean | ✓ PASS |
| Static analysis | `go vet ./...` | clean | ✓ PASS |
| Default-path + new tests (changed pkgs) | `go test ./cmd/kangent/ ./internal/ws/ -count=1` | ok / ok | ✓ PASS |
| WS origin must-haves | `go test ./internal/ws/ -run 'TestIntegrationInsecureAnyOriginAccepted\|TestIntegrationEvilOriginRejected'` | both PASS | ✓ PASS |
| Safe-by-default invariant (function-body immutability) | md5 of `ensureLoopback`/`hostCheck` bodies, pre vs post | identical (`ad55aead...` / `9c03837d...`) | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| INSECURE-REMOTE-01 | 260626-hwd-PLAN.md | Opt-in non-loopback bind bypassing all three loopback layers, loud warn, hook BaseURL normalization, safe-by-default | ✓ SATISFIED | All 6 truths verified; full gate green. (REQUIREMENTS.md was removed in commit 4b35f3a for the v1.7 milestone archive — requirement traced from PLAN frontmatter.) |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | — | No TBD/FIXME/XXX/HACK/PLACEHOLDER/"not yet implemented" in any of the 6 modified files | — | None |

### Test Suite Note (transient flake — not a defect)

The full `go test ./... -count=1` run reported failures in `kangent/internal/session` (`TestAttachAfterExitReturnsReplay`, `TestExitCapturesCodeAndNotifies`, `TestRemoveSemantics`, `TestStopZeroDescendants`, `TestStopOnExitedSessionIsIdempotent`, `TestStopEscalatesToSigkill`, `TestSpawnCwd`) — all PTY/process-group timeout patterns ("session did not exit", "timed out after 5s waiting for…", "Done() not closed within 5s"). This is the documented load-induced flake in the heavy PTY/tmux suite under full-suite parallelism.

Confirmed transient by isolated re-run:
- `go test ./internal/session/ -count=1` → **ok 22.265s** (clean, all pass).
- `internal/api` (the package the brief/STATE flagged) **passed** in the same full-suite run (176s).

This change is confined to `cmd/kangent` + `internal/ws`; `internal/session` does not depend on either and cannot be regressed by it. The isolated pass is definitive evidence that the suite is green and the flake is environmental load, not a logic defect.

### Human Verification Required

None. All must-haves are verifiable programmatically (flag wiring, function-body immutability via md5, conditional enforcement via table tests, cross-origin WS upgrade via integration test). No visual/UX/real-time behavior in scope.

### Gaps Summary

No gaps. All 6 observable truths are VERIFIED, all 3 artifacts exist/substantive/wired, both key links are WIRED, build+vet+targeted-tests are green, the SAFE-BY-DEFAULT invariant is proven by byte-identical `ensureLoopback`/`hostCheck` function bodies (md5-confirmed) with only invocation sites changed, the loud warn banner is emitted on the insecure path, `InsecureSkipVerify` is wired to the handler field (not a literal), and `hookBaseURL` normalizes only wildcard hosts. The single full-suite `internal/session` failure is the documented PTY/tmux load flake and passes cleanly in isolation.

---

_Verified: 2026-06-26T13:30:00Z_
_Verifier: Claude (gsd-verifier)_
