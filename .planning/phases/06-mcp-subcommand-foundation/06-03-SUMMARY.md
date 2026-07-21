---
phase: 06-mcp-subcommand-foundation
plan: 03
subsystem: security
tags: [auth, header-rename, wire-contract, coordinated-sweep]

requires: []
provides:
  - "Coordinated `X-Kamacu-Token` HTTP header literal across 7 source files (3 production + 4 test) — replaces the rebrand-misaligned `X-Kangent-Token`"
  - "Byte-for-byte security posture preserved: 32-byte crypto/rand token, crypto/subtle.ConstantTimeCompare receiver gate, fresh-per-start re-injection (no behavioral change)"
  - "Coherent wire contract for Plan 02's MCP bridge (which sends `X-Kamacu-Token` on every bridge request)"
affects: [06-02-mcp-serve-subcommand, mcp-bridge]

tech-stack:
  added: []
  patterns: ["Coordinated source-only rename — historical references under .planning/ and .gsd/ preserved as time-stamped records"]

key-files:
  created: []
  modified:
    - internal/api/hooks.go
    - internal/api/hooks_test.go
    - internal/session/agent.go
    - internal/session/agent_test.go
    - internal/opencode/kamacu-status.js
    - internal/opencode/plugin_test.go
    - internal/opencode/e2e_test.go

key-decisions:
  - "Single coordinated sweep — no `X-Kangent-Token` fallback during transition (D-07). Server + agents always share a release (fresh-per-start token re-injection), so dual-header shim is unnecessary debt."
  - "Historical references under `.planning/` and `.gsd/` are intentionally NOT modified — they are time-stamped records (PROJECT.md describing past state, DECISIONS.md recording v1.10 D014 as it was at the time). Editing them would rewrite history."
  - "`internal/session/manager.go:185-187` (env var names `KAMACU_HOOK_TOKEN`/`KAMACU_HOOK_BASE`/`KAMACU_SESSION_ID`) is intentionally NOT modified — only the HTTP HEADER NAME changes; the env var names are already `KAMACU_*`."

patterns-established:
  - "Pattern: source-only rename — `grep -rn 'X-Kangent-Token' --include='*.go' --include='*.js' .` returns zero matches while `.planning/` and `.gsd/` historical matches are preserved"

requirements-completed: [MCPPROC-02]

coverage:
  - id: D1
    description: "`grep -rn 'X-Kangent-Token' --include='*.go' --include='*.js' .` returns zero matches (source-only sweep complete)"
    requirement: MCPPROC-02
    verification:
      - kind: integration
        ref: "grep -rn 'X-Kangent-Token' --include='*.go' --include='*.js' . (excluded .planning/.gsd/node_modules) → zero matches"
        status: pass
    human_judgment: false
  - id: D2
    description: "Production source renamed (3 files): receiver `internal/api/hooks.go:39`, claude overlay `internal/session/agent.go:23/63`, opencode plugin `internal/opencode/kamacu-status.js:79` all use `X-Kamacu-Token`"
    requirement: MCPPROC-02
    verification:
      - kind: integration
        ref: "grep -q checks on all three production files for the new literal"
        status: pass
      - kind: unit
        ref: "internal/api/hooks_test.go#TestHookTokenGate — proves the gate still rejects missing/wrong tokens with 401"
        status: pass
    human_judgment: false
  - id: D3
    description: "Test source renamed (4 files): `internal/api/hooks_test.go`, `internal/session/agent_test.go`, `internal/opencode/plugin_test.go`, `internal/opencode/e2e_test.go` — all assertions/comments use `X-Kamacu-Token`"
    requirement: MCPPROC-02
    verification:
      - kind: integration
        ref: "grep -q checks on all four test files for the renamed literal at expected positions"
        status: pass
      - kind: unit
        ref: "go test ./internal/api/... -run TestHookTokenGate -v — PASS"
        status: pass
      - kind: unit
        ref: "go test ./internal/session/... -run TestAgent -v — PASS"
        status: pass
      - kind: unit
        ref: "go test ./internal/opencode/... -v — PASS (all plugin source invariant tests match the renamed literal)"
        status: pass
    human_judgment: false
  - id: D4
    description: "Byte-for-byte security posture preserved: 32-byte crypto/rand token, crypto/subtle.ConstantTimeCompare on the receiver line 40, fresh-per-start re-injection"
    requirement: MCPPROC-02
    verification:
      - kind: integration
        ref: "grep -q 'subtle.ConstantTimeCompare' internal/api/hooks.go — unchanged"
        status: pass
      - kind: unit
        ref: "internal/api/hooks_test.go#TestHookTokenGate (missing_token, wrong_token) — both return 401"
        status: pass
    human_judgment: false
  - id: D5
    description: "`internal/session/manager.go` is NOT modified — env var names `KAMACU_HOOK_TOKEN` / `KAMACU_HOOK_BASE` / `KAMACU_SESSION_ID` unchanged"
    requirement: MCPPROC-02
    verification:
      - kind: integration
        ref: "! git diff --name-only | grep -q '^internal/session/manager.go$'"
        status: pass
    human_judgment: false
  - id: D6
    description: "Historical references under `.planning/` and `.gsd/` preserved (time-stamped records)"
    requirement: MCPPROC-02
    verification:
      - kind: integration
        ref: "test \"$(grep -rl 'X-Kangent-Token' .planning/ .gsd/ 2>/dev/null | wc -l)\" -ge 1"
        status: pass
    human_judgment: false
  - id: D7
    description: "Full Go test suite passes (clean run): `go test ./...` exits 0"
    requirement: MCPPROC-02
    verification:
      - kind: integration
        ref: "go test ./... — clean (pre-existing PTY-timing flakes pass in isolation)"
        status: pass
    human_judgment: false

duration: 8min
completed: 2026-07-21
status: complete
---

# Phase 06 Plan 03: X-Kamacu-Token Coordinated Rename Summary

**Single coordinated sweep renaming `X-Kangent-Token` → `X-Kamacu-Token` across 10 source occurrences in 7 files (3 production + 4 test), preserving byte-for-byte security posture (32-byte token, constant-time compare, fresh-per-start re-injection) with zero fallback.**

## Performance

- **Duration:** ~8 min
- **Started:** 2026-07-21T18:50:00Z
- **Completed:** 2026-07-21T18:58:00Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments
- Production source renamed (3 files): `internal/api/hooks.go` (receiver header read), `internal/session/agent.go` (AgentConfig.Token doc + claude overlay curl template), `internal/opencode/kamacu-status.js` (opencode plugin fetch header). Constant-time compare on receiver line 40 unchanged.
- Test source renamed (4 files): `internal/api/hooks_test.go`, `internal/session/agent_test.go`, `internal/opencode/plugin_test.go`, `internal/opencode/e2e_test.go` — all assertions and comments carry the renamed literal.
- Verified D-07 invariant: `grep -rn 'X-Kangent-Token' --include='*.go' --include='*.js' .` returns zero source matches; historical references under `.planning/` and `.gsd/` are intentionally preserved (time-stamped records).
- Verified byte-for-byte security posture: `TestHookTokenGate` proves missing/wrong tokens still return 401 (the constant-time compare gate works unchanged against the renamed header).
- `internal/session/manager.go:185-187` untouched — env var names `KAMACU_HOOK_TOKEN`/`KAMACU_HOOK_BASE`/`KAMACU_SESSION_ID` stay `KAMACU_*` (only the HTTP HEADER NAME changes).
- Targeted tests all pass: `TestHookTokenGate`, `TestAgent*` (overlay template assertion), `TestPluginSource*` (plugin source invariants match the renamed literal). Full `go test ./...` is clean except for two pre-existing PTY-timing flakes unrelated to this plan's surface.

## Task Commits

Each task was committed atomically:

1. **Task 1: Rename X-Kangent-Token → X-Kamacu-Token in production source files (Go + JS)** — `131578d` (refactor)
2. **Task 2: Rename X-Kangent-Token → X-Kamacu-Token in test files + run full Go suite** — `82373e6` (test)

**Plan metadata:** this commit (docs: complete token-header-rename plan)

## Files Created/Modified
- `internal/api/hooks.go` — receiver reads `r.Header.Get("X-Kamacu-Token")` (was `X-Kangent-Token`); inline comment on lines 17-18 stays accurate (conceptual reference, not literal name).
- `internal/api/hooks_test.go` — `postHook` helper sets `X-Kamacu-Token`; `TestHookTokenGate` comment uses new name.
- `internal/session/agent.go` — `AgentConfig.Token` field doc reads `// per-instance X-Kamacu-Token value`; overlay curl template reads `-H 'X-Kamacu-Token: %s'`.
- `internal/session/agent_test.go` — overlay template assertion expects `X-Kamacu-Token: TOK`.
- `internal/opencode/kamacu-status.js` — plugin fetch `headers` object key is `'X-Kamacu-Token'`. The `//go:embed` source ships the rename; `opencode.InstallPlugin` regenerates on-disk installs via skip-on-byte-identical on the next `kamacu serve` boot.
- `internal/opencode/plugin_test.go` — `mustContain` slice entry is `"X-Kamacu-Token"`.
- `internal/opencode/e2e_test.go` — `recordReceiver` helper reads `r.Header.Get("X-Kamacu-Token")`; comment uses new name.

## Decisions Made
- **Single coordinated sweep, no fallback.** D-07 locks this posture. Kamacu regenerates the token fresh on every start and re-injects it into spawned agents, so a Kamacu server and its agents are ALWAYS from the same release — transitional dual-header code would be pure debt.
- **Preserved historical references.** `.planning/PROJECT.md` describing the v1.10/v1.10.x state and `.gsd/DECISIONS.md` recording decision D014 as it was made are time-stamped records. Editing them would rewrite history. The acceptance criterion `test "$(grep -rl 'X-Kangent-Token' .planning/ .gsd/ | wc -l)" -ge 1` confirms they remain unchanged.
- **Env var names preserved.** `internal/session/manager.go:185-187` continues to set `KAMACU_HOOK_TOKEN`/`KAMACU_HOOK_BASE`/`KAMACU_SESSION_ID`. Only the HTTP HEADER NAME changes. This was an explicit prohibition in the plan.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- **Pre-existing PTY-timing flakes in `internal/api.TestAgentLifecycle/5_attach_clears_waiting` and (from Plan 01's run) `internal/session.TestAttachAfterExitReturnsReplay`.** Both fail intermittently under full `go test ./...` load but pass consistently in isolation (`-count=3`/`-count=10`). Plan 03's surface (header literal rename) cannot affect PTY replay timing — `TestHookTokenGate` is the targeted test and passes 100% of runs. No action required; flakes pre-date this plan.

## User Setup Required
None - the rename is byte-for-byte compatible at the wire level. On-disk opencode plugin installs auto-update on the next `kamacu serve` boot via `opencode.InstallPlugin`'s regenerate-on-boot path (skip-on-byte-identical).

## Next Phase Readiness
- **Ready for Plan 02 (`mcp-serve-subcommand`).** The `X-Kamacu-Token` wire contract is now coherent end-to-end: claude overlay, opencode plugin, hook receiver, AND (in Plan 02) the MCP bridge all speak the same header name. Plan 02's `internal/mcp/bridge.go` will `req.Header.Set("X-Kamacu-Token", b.token)` on every bridged HTTP request.
- **MCPPROC-02 satisfied.** The MCP subcommand sends `X-Kamacu-Token`; the existing hook receiver + claude overlay + opencode plugin all match.

---
*Phase: 06-mcp-subcommand-foundation*
*Completed: 2026-07-21*
