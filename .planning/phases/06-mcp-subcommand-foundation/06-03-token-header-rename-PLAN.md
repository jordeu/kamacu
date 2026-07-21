---
phase: 06-mcp-subcommand-foundation
plan: 03
slug: token-header-rename
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/api/hooks.go
  - internal/api/hooks_test.go
  - internal/session/agent.go
  - internal/session/agent_test.go
  - internal/opencode/kamacu-status.js
  - internal/opencode/plugin_test.go
  - internal/opencode/e2e_test.go
autonomous: true
requirements: [MCPPROC-02]
must_haves:
  truths:
    # D-07 — coordinated rename, break clean
    - "Every source occurrence of the literal `X-Kangent-Token` in Go and JS source files is renamed to `X-Kamacu-Token` — `grep -rn 'X-Kangent-Token' --include='*.go' --include='*.js' .` returns zero results after this plan"
    - "The hook receiver in `internal/api/hooks.go:39` reads the header via `r.Header.Get(\"X-Kamacu-Token\")` (D-07). The constant-time compare (`crypto/subtle.ConstantTimeCompare`) on the next line is byte-for-byte preserved — the rename is purely cosmetic on the header name; the security posture is unchanged."
    - "The claude settings overlay curl template in `internal/session/agent.go:63` emits the header as `-H 'X-Kamacu-Token: %s'` (D-07) — every spawned claude task's hooks POST with the renamed header."
    - "The `AgentConfig.Token` field doc comment in `internal/session/agent.go:23` reads `// per-instance X-Kamacu-Token value` (D-07)."
    - "The opencode status plugin in `internal/opencode/kamacu-status.js:79` sends the header as `'X-Kamacu-Token': token` in the fetch call (D-07). Existing on-disk installs auto-update on the next `kamacu serve` boot via `opencode.InstallPlugin`'s regenerate-on-boot path (skip-on-byte-identical)."
    - "All inline comments referencing the header name are updated to read `X-Kamacu-Token`: `internal/api/hooks.go` lines 17-18 (the security note about the token-required gate), `internal/api/hooks_test.go:103` (the TestHookTokenGate comment), `internal/opencode/e2e_test.go:136` (the recordReceiver contract comment)."
    - "All test assertions referencing the header literal are updated: `internal/api/hooks_test.go:87` (`req.Header.Set`), `internal/session/agent_test.go:94` (`strings.Contains(h.Command, \"X-Kamacu-Token: TOK\")`), `internal/opencode/plugin_test.go:195` (the mustContain slice entry), `internal/opencode/e2e_test.go:144` (`r.Header.Get`)."
    - "After the rename, the full Go test suite passes — `go test ./internal/api/... ./internal/session/... ./internal/opencode/...` exits 0. `TestHookTokenGate` proves the gate still rejects missing/wrong tokens with 401 (preserved byte-for-byte)."
  artifacts:
    - internal/api/hooks.go (header literal + comments)
    - internal/api/hooks_test.go (header literal + comment)
    - internal/session/agent.go (Token field doc + curl template literal)
    - internal/session/agent_test.go (assertion literal)
    - internal/opencode/kamacu-status.js (fetch header literal)
    - internal/opencode/plugin_test.go (mustContain slice literal)
    - internal/opencode/e2e_test.go (recordReceiver literal + comment)
  key_links:
    - "internal/session/agent.go overlay emits `X-Kamacu-Token` → internal/api/hooks.go receiver reads `X-Kamacu-Token` (claude engine status hooks)"
    - "internal/opencode/kamacu-status.js fetch sends `X-Kamacu-Token` → internal/api/hooks.go receiver reads `X-Kamacu-Token` (opencode engine status hooks)"
    - "internal/mcp/bridge.go (added in Plan 02) sends `X-Kamacu-Token` → wire contract matches across all three senders (claude overlay, opencode plugin, MCP bridge)"
  prohibitions:
    - statement: "NO `X-Kangent-Token` fallback during transition (D-07) — the rename is ONE coordinated sweep with NO dual-header shim. The receiver reads exactly ONE header name: `X-Kamacu-Token`."
      status: resolved
      verification: "`grep -c 'X-Kangent-Token' internal/api/hooks.go` returns 0; the file has no `or r.Header.Get(\"X-Kangent-Token\")` fallback logic, no `or` chain"
    - statement: "DO NOT touch `.planning/` or `.gsd/` historical references to `X-Kangent-Token`. Per 06-RESEARCH.md § 'Token-receiver coordinated rename': these are time-stamped historical prose (e.g. `.planning/PROJECT.md:227` describing a past state, `.gsd/DECISIONS.md:22` recording the v1.10 D014 decision as it was at the time). Editing them rewrites history."
      status: resolved
      verification: "After this plan, `grep -r 'X-Kangent-Token' .planning/ .gsd/` STILL returns the historical matches (unchanged); only `grep -rn 'X-Kangent-Token' --include='*.go' --include='*.js' .` (source files only) returns zero"
    - statement: "DO NOT add any new middleware, route, or handler. This is a pure mechanical rename preserving byte-for-byte security posture (constant-time compare, 32-byte crypto/rand token, fresh-per-start re-injection into spawned agents)."
      status: resolved
      verification: "`git diff internal/api/hooks.go` shows ONLY the literal header name change on line 39 + the inline comment update on lines 17-18; no structural changes, no new imports, no new functions"
    - statement: "DO NOT change the env var names `KAMACU_HOOK_TOKEN` / `KAMACU_HOOK_BASE` / `KAMACU_SESSION_ID`. Only the HTTP HEADER NAME changes (`X-Kangent-Token` → `X-Kamacu-Token`). The env vars stay `KAMACU_*` verbatim — `internal/session/manager.go:185-187` is NOT modified by this plan."
      status: resolved
      verification: "`git diff --name-only` for this plan does NOT include `internal/session/manager.go`"
    - statement: "DO NOT touch the historical decision IDs (e.g. `D014` from v1.10) that reference the old header name. Those are time-stamped records."
      status: resolved
      verification: "`grep -rn 'D014' .gsd/ .planning/` continues to return the historical matches unchanged"
---

# Plan 03: Coordinated `X-Kangent-Token` → `X-Kamacu-Token` rename

<objective>
Rename the per-instance hook token HTTP header from `X-Kangent-Token` to `X-Kamacu-Token` in ONE coordinated sweep across all 11 source occurrences in 7 files (Go production + Go tests + JS plugin + JS plugin test), preserving byte-for-byte security posture (constant-time compare, 32-byte crypto/rand token, fresh-per-start re-injection). No fallback during transition (D-07). This plan is parallel-safe with Plan 01 (no file overlap) and lands the wire contract that Plan 02's MCP bridge also uses.

Purpose: The rebrand-aligned header name. The MCP subcommand (Plan 02) sends `X-Kamacu-Token` on every bridge request; the existing hook receiver + claude overlay + opencode plugin must all speak the same header name to keep status hooks working. The break-clean posture is intentional and safe: Kamacu regenerates the token fresh on every start and re-injects it into spawned agents, so a Kamacu server and its agents are ALWAYS from the same release — transitional dual-header code is unnecessary debt.

Output: 7 files mechanically edited (3 production + 4 test). The full Go test suite passes. The opencode plugin regeneration path (`opencode.InstallPlugin`) picks up the JS rename on the next `kamacu serve` boot — existing on-disk installs auto-update.
</objective>

<execution_context>
@/home/jordi/.config/opencode/gsd-core/workflows/execute-plan.md
@/home/jordi/.config/opencode/gsd-core/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/phases/06-mcp-subcommand-foundation/06-CONTEXT.md
@.planning/phases/06-mcp-subcommand-foundation/06-RESEARCH.md
@internal/api/hooks.go
@internal/session/agent.go
@internal/opencode/kamacu-status.js
</context>

<tasks>

<task type="auto">
  <name>Task 1: Rename X-Kangent-Token → X-Kamacu-Token in production source files (Go + JS)</name>
  <files>internal/api/hooks.go, internal/session/agent.go, internal/opencode/kamacu-status.js</files>
  <read_first>
    - `.planning/phases/06-mcp-subcommand-foundation/06-RESEARCH.md` § "Token-receiver coordinated rename (the D-07 surface)" — the exhaustive surface inventory enumerating every Go + JS occurrence with line numbers, plus the explicit list of NON-source occurrences (`.planning/`, `.gsd/`) that MUST NOT be touched (history-only).
    - `internal/api/hooks.go` (full file, 77 lines) — the receiver. Line 39 `got := r.Header.Get("X-Kangent-Token")` is the rename target; lines 17-18 are the inline comment block (`Security note (verified middleware interplay)... the token is therefore REQUIRED, never optional`) that references the header in prose.
    - `internal/session/agent.go` (full file, 113 lines) — the overlay builder. Line 23 `Token string // per-instance X-Kangent-Token value` (the `AgentConfig.Token` field doc comment); line 63 `"curl -s -m 3 -H 'X-Kangent-Token: %s' --data-binary @- %s/api/hooks/sessions/%s"` (the curl template inside `buildOverlayJSON`).
    - `internal/opencode/kamacu-status.js` (full file, 160 lines) — the opencode plugin source. Line 79 `'X-Kangent-Token': token,` (the fetch headers literal in the `notify` async function). This file is `//go:embed`-ed and shipped; the rename lands in source and propagates to on-disk installs via `opencode.InstallPlugin` regenerate-on-boot.
    - `.planning/phases/06-mcp-subcommand-foundation/06-CONTEXT.md` D-07 — locks the header name as `X-Kamacu-Token`, locks the no-fallback-during-transition posture, enumerates the coordinated rename surface.
  </read_first>
  <action>
    Replace every literal occurrence of `X-Kangent-Token` with `X-Kamacu-Token` in three production files. No structural changes — pure cosmetic rename preserving all logic.

    1. `internal/api/hooks.go`:
       - Line 39: `got := r.Header.Get("X-Kangent-Token")` → `got := r.Header.Get("X-Kamacu-Token")`.
       - Lines 17-18 inline comment: update any prose that names the header (e.g. "the token is therefore REQUIRED, never optional" stays, but if the comment block names the literal `X-Kangent-Token`, rename it to `X-Kamacu-Token`). Read the actual lines first; the docstring may reference the header only conceptually in which case no edit is needed beyond line 39.
       - DO NOT touch line 40 (`subtle.ConstantTimeCompare([]byte(got), []byte(h.token))`) — that compare is byte-for-byte preserved.

    2. `internal/session/agent.go`:
       - Line 23 (the `AgentConfig.Token` field doc comment): `Token string // per-instance X-Kangent-Token value` → `Token string // per-instance X-Kamacu-Token value`.
       - Line 63 (the curl template inside `buildOverlayJSON`'s `overlayHook.Command` literal): the entire `"curl -s -m 3 -H 'X-Kangent-Token: %s' --data-binary @- %s/api/hooks/sessions/%s"` becomes `"curl -s -m 3 -H 'X-Kamacu-Token: %s' --data-binary @- %s/api/hooks/sessions/%s"`. Only the header literal changes; the `fmt.Sprintf` format verbs (`%s`, `%s`, `%s`) are unchanged.

    3. `internal/opencode/kamacu-status.js`:
       - Line 79 (inside the `notify` async function's `fetch` call `headers` object): `'X-Kangent-Token': token,` → `'X-Kamacu-Token': token,`. The surrounding `method`, `body`, `signal` keys are unchanged.

    4. Verify NO production source occurrence of `X-Kangent-Token` remains: `grep -rn 'X-Kangent-Token' --include='*.go' --include='*.js' internal/` returns zero matches (test files are intentionally NOT yet renamed — Task 2 owns those).

    5. DO NOT touch `internal/session/manager.go:185-187` — the env var names (`KAMACU_HOOK_TOKEN`, `KAMACU_HOOK_BASE`, `KAMACU_SESSION_ID`) stay `KAMACU_*` verbatim. Only the HTTP header name changes.

    6. DO NOT touch any `.planning/` or `.gsd/` historical reference — those are time-stamped records.
  </action>
  <verify>
    <automated>
      set -e
      # Zero production-source occurrences of the old literal
      ! grep -rn 'X-Kangent-Token' --include='*.go' --include='*.js' internal/api/hooks.go internal/session/agent.go internal/opencode/kamacu-status.js
      # Receiver reads the new header
      grep -q 'r.Header.Get("X-Kamacu-Token")' internal/api/hooks.go
      # Constant-time compare preserved (line 40 area — unchanged)
      grep -q 'subtle.ConstantTimeCompare' internal/api/hooks.go
      # Overlay curl template uses new header
      grep -q "curl -s -m 3 -H 'X-Kamacu-Token: %s'" internal/session/agent.go
      # Token field doc updated
      grep -q 'per-instance X-Kamacu-Token value' internal/session/agent.go
      # opencode plugin fetch uses new header
      grep -q "'X-Kamacu-Token': token," internal/opencode/kamacu-status.js
      # manager.go untouched (env var names preserved)
      ! git diff --name-only | grep -q '^internal/session/manager.go$'
      # Package still compiles (the production rename is internally consistent)
      go build ./internal/api/... ./internal/session/... ./internal/opencode/...
    </automated>
  </verify>
  <done>
    - `grep -c 'X-Kangent-Token' internal/api/hooks.go internal/session/agent.go internal/opencode/kamacu-status.js` returns 0 for each file
    - `internal/api/hooks.go:39` reads `r.Header.Get("X-Kamacu-Token")`; line 40 `subtle.ConstantTimeCompare` is unchanged
    - `internal/session/agent.go:23` doc comment reads `// per-instance X-Kamacu-Token value`; line 63 curl template reads `-H 'X-Kamacu-Token: %s'`
    - `internal/opencode/kamacu-status.js:79` reads `'X-Kamacu-Token': token,`
    - `internal/session/manager.go` is NOT modified (env var names preserved)
    - `go build ./internal/api/... ./internal/session/... ./internal/opencode/...` succeeds
  </done>
</task>

<task type="auto">
  <name>Task 2: Rename X-Kangent-Token → X-Kamacu-Token in test files + run full Go suite</name>
  <files>internal/api/hooks_test.go, internal/session/agent_test.go, internal/opencode/plugin_test.go, internal/opencode/e2e_test.go</files>
  <read_first>
    - `internal/api/hooks_test.go` around lines 80-105 — the `postHook` helper at line 87 (`req.Header.Set("X-Kangent-Token", token)`) and the `TestHookTokenGate` comment at line 103 (`// X-Kangent-Token and never mutates session state`).
    - `internal/session/agent_test.go` around line 94 — `if !strings.Contains(h.Command, "X-Kangent-Token: TOK")` (asserts the overlay curl template carries the token header literal — must match the renamed template from Task 1).
    - `internal/opencode/plugin_test.go` around lines 185-205 — the `mustContain` slice includes `"X-Kangent-Token",` at line 195 as a required token in the plugin source (now matches the renamed literal from Task 1).
    - `internal/opencode/e2e_test.go` around lines 135-145 — `recordReceiver` helper: line 136 comment `// receiver contract (X-Kangent-Token constant-time compare, decode`, line 144 `got := r.Header.Get("X-Kangent-Token")` (mirrors the production receiver; must match Task 1's rename).
    - The corresponding production files post-Task-1 — to confirm the test literals match the production literals exactly (a mismatch would fail tests).
  </read_first>
  <action>
    Replace every literal occurrence of `X-Kangent-Token` with `X-Kamacu-Token` in four test files. Pure cosmetic rename so the test assertions match the renamed production literals from Task 1.

    1. `internal/api/hooks_test.go`:
       - Line 87: `req.Header.Set("X-Kangent-Token", token)` → `req.Header.Set("X-Kamacu-Token", token)`.
       - Line 103: the comment `// X-Kangent-Token and never mutates session state` → `// X-Kamacu-Token and never mutates session state`. (Read the actual line first — the comment may span more text; rename only the literal header name within it.)

    2. `internal/session/agent_test.go`:
       - Line 94: `if !strings.Contains(h.Command, "X-Kangent-Token: TOK")` → `if !strings.Contains(h.Command, "X-Kamacu-Token: TOK")`. The `" TOK"` suffix matches the test fixture token — unchanged.

    3. `internal/opencode/plugin_test.go`:
       - Line 195 (inside the `mustContain` slice): `"X-Kangent-Token",` → `"X-Kamacu-Token",`. The trailing comment `// same header as claude's overlay` is still accurate — leave it.

    4. `internal/opencode/e2e_test.go`:
       - Line 136 (the `recordReceiver` comment): `// receiver contract (X-Kangent-Token constant-time compare, decode` → `// receiver contract (X-Kamacu-Token constant-time compare, decode`.
       - Line 144 (inside `recordReceiver`'s handler): `got := r.Header.Get("X-Kangent-Token")` → `got := r.Header.Get("X-Kamacu-Token")`.

    5. After all edits, verify zero source-wide occurrences remain: `grep -rn 'X-Kangent-Token' --include='*.go' --include='*.js' .` returns zero matches. Historical references under `.planning/` and `.gsd/` are intentionally NOT modified — confirm those still match (they should — Task 2 doesn't touch them).

    6. Run the full Go test suite for the three affected packages: `go test ./internal/api/... ./internal/session/... ./internal/opencode/...`. All tests must pass.
       - Specifically `go test ./internal/api/... -run TestHookTokenGate -v` proves the constant-time-compare gate still rejects missing/wrong tokens with 401 and accepts the right token with 204 (preserved byte-for-byte by the rename).
       - Specifically `go test ./internal/opencode/... -run TestPluginSource -v` (or whatever the plugin source invariant test is named) proves the `mustContain` slice now matches the renamed literal in `kamacu-status.js`.

    7. Finally, run `go test ./...` for full project regression safety.
  </action>
  <verify>
    <automated>
      set -e
      # Zero source-wide occurrences of the old literal (Go + JS only)
      ! grep -rn 'X-Kangent-Token' --include='*.go' --include='*.js' .
      # Historical references under .planning/ and .gsd/ are PRESERVED (not edited)
      test "$(grep -rl 'X-Kangent-Token' .planning/ .gsd/ 2>/dev/null | wc -l)" -ge 1
      # All four test files renamed
      grep -q 'req.Header.Set("X-Kamacu-Token", token)' internal/api/hooks_test.go
      grep -q 'X-Kamacu-Token and never mutates' internal/api/hooks_test.go
      grep -q 'X-Kamacu-Token: TOK' internal/session/agent_test.go
      grep -q '"X-Kamacu-Token",' internal/opencode/plugin_test.go
      grep -q 'X-Kamacu-Token constant-time compare' internal/opencode/e2e_test.go
      grep -q 'r.Header.Get("X-Kamacu-Token")' internal/opencode/e2e_test.go
      # Targeted tests pass
      go test ./internal/api/... -run TestHookTokenGate -v
      go test ./internal/session/... -run TestAgent -v
      go test ./internal/opencode/... -v
      # Full regression
      go test ./...
    </automated>
  </verify>
  <done>
    - `grep -rn 'X-Kangent-Token' --include='*.go' --include='*.js' .` returns zero matches
    - Historical references under `.planning/` and `.gsd/` remain unchanged (still match `X-Kangent-Token` as time-stamped records)
    - All four test files have the renamed literal in the expected positions
    - `go test ./internal/api/... -run TestHookTokenGate -v` passes (gate behavior preserved)
    - `go test ./internal/opencode/... -v` passes (plugin source invariants match renamed literal)
    - `go test ./...` exits 0 with zero failures
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| browser → Kamacu HTTP API | SPA fetches + malicious-webpage no-CORS POSTs at localhost. UNCHANGED by this plan — the same boundary that exists today. The hook receiver on `/api/hooks/sessions/{id}` is the one place Kamacu reads the per-instance token; the rename preserves the gate byte-for-byte. |
| claude / opencode task → Kamacu hook receiver | The spawned agent's hooks POST to `/api/hooks/sessions/{id}` with the per-instance token in the header. UNCHANGED auth posture; only the header NAME changes. |
| agent CLI spawned task → Kamacu HTTP API (loopback) | Same as today. The rename does NOT add token validation to `/api/*` routes (D-06). |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-06-09 | Spoofing | malicious webpage no-CORS POST at localhost:7333 with guessed hook token | medium | mitigate | UNCHANGED posture: 32-byte `crypto/rand` token, `crypto/subtle.ConstantTimeCompare` on the receiver (line 40 of `internal/api/hooks.go` — byte-for-byte preserved), fresh-per-start, regenerated into spawned agents every boot. Renaming the header does NOT weaken the gate; the test `TestHookTokenGate` proves missing/wrong tokens still return 401. |
| T-06-10 | Tampering | on-disk opencode plugin carries stale `X-Kangent-Token` literal until next `kamacu serve` boot | low | accept | 06-RESEARCH.md Pitfall 6 documents the two-layer protection: (a) the rename lands in `internal/opencode/kamacu-status.js` (the `//go:embed` source) AND (b) the rename lands in the receiver. After the next `kamacu serve`, `opencode.InstallPlugin` regenerates the on-disk file via skip-on-byte-identical. The D-07 invariant (server+agents always same-release, fresh-per-start token) means any mismatch is one-time during the rename. Acceptable for a single-user local app. |
| T-06-11 | Information Disclosure | header literal rename visible in network traffic | low | accept | The header NAME was never a secret — only the token VALUE is. Renaming the header reveals nothing an attacker didn't already know. The token value remains 32-byte random and is never logged. |
| T-06-12 | Tampering | subtle rename typo (e.g. `X-Kamacu-token` lowercase, `X_Kamacu_Token` underscore) desyncs senders from receiver | medium | mitigate | The test suite is the safety net: `TestHookTokenGate` exercises the receiver with the renamed header; `TestAgent*` (or whatever asserts the overlay template) exercises the claude sender; `TestPluginSource` exercises the opencode plugin source. A typo in any of the three senders fails one of these tests. The acceptance criterion `grep -c 'X-Kamacu-Token' --include='*.go' --include='*.js' .` returning the expected count (≥7 source occurrences after rename) is also a structural check. |

</threat_model>

<verification>
- `grep -rn 'X-Kangent-Token' --include='*.go' --include='*.js' .` returns zero matches
- Historical references under `.planning/` and `.gsd/` remain unchanged (still match the old literal as time-stamped records)
- `internal/api/hooks.go:39` reads `r.Header.Get("X-Kamacu-Token")`; line 40 `subtle.ConstantTimeCompare` unchanged
- `internal/session/agent.go:23` doc comment + `:63` curl template use `X-Kamacu-Token`
- `internal/opencode/kamacu-status.js:79` fetch header literal uses `X-Kamacu-Token`
- All four test files (`hooks_test.go`, `agent_test.go`, `plugin_test.go`, `e2e_test.go`) have the renamed literals in the expected positions
- `internal/session/manager.go` is NOT modified (env var names `KAMACU_*` preserved)
- `go test ./...` passes with zero failures
</verification>

<success_criteria>
Phase 06 D-07 fully delivered by this plan:
- Header name `X-Kamacu-Token` everywhere in Go + JS source (11 occurrences across 7 files renamed)
- No `X-Kangent-Token` fallback during transition (single coordinated sweep, no dual-header shim)
- Byte-for-byte security posture preserved (32-byte crypto/rand token, constant-time compare, fresh-per-start re-injection)
- Historical references under `.planning/` and `.gsd/` untouched
- Env var names `KAMACU_HOOK_TOKEN` / `KAMACU_HOOK_BASE` / `KAMACU_SESSION_ID` unchanged
- Full Go test suite passes
- Phase 06 MCPPROC-02 wire contract (MCP bridge sends `X-Kamacu-Token`; existing hook receiver + claude overlay + opencode plugin all match) is coherent end-to-end
</success_criteria>

<output>
Create `.planning/phases/06-mcp-subcommand-foundation/06-03-SUMMARY.md` when done
</output>

## Artifacts this phase produces

This plan creates NO new symbols, files, struct fields, or CLI flags. It is a pure mechanical rename of an existing HTTP header literal across 7 existing files (3 production + 4 test).

For plan-review-convergence source-grounding: the only "new" surface is the renamed literal `X-Kamacu-Token` replacing the prior `X-Kangent-Token` in:
- `internal/api/hooks.go:39` (receiver header read) + `:17-18` (inline comment)
- `internal/api/hooks_test.go:87` (test helper header set) + `:103` (test comment)
- `internal/session/agent.go:23` (AgentConfig.Token field doc) + `:63` (overlay curl template)
- `internal/session/agent_test.go:94` (assertion literal)
- `internal/opencode/kamacu-status.js:79` (fetch header literal)
- `internal/opencode/plugin_test.go:195` (mustContain slice literal)
- `internal/opencode/e2e_test.go:136` (recordReceiver comment) + `:144` (recordReceiver header read)

No new file paths. No new exports. No new struct fields. No new dependencies in `go.mod`. The wire-contract literal `X-Kamacu-Token` becomes the canonical name Plan 02's `internal/mcp/bridge.go` also uses.
