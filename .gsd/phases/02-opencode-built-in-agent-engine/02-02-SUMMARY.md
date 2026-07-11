---
id: S02
parent: M002
milestone: M002
provides:
  - fetch-based opencode status plugin (no curl-on-PATH dependency) with a permanent curl-absent regression guard
  - first real-binary opencode 1.17.15 end-to-end proof of the plugin→receiver loop (Stop→idle) and the env-gate no-op
  - reusable host-gated real-binary e2e harness pattern for S03 (build tag + httptest receiver mirroring the production X-Kangent-Token contract + temp XDG_CONFIG_HOME + os.Environ KAMACU_* scrub)
requires:
  - slice: S01
    provides: embedded opencode status plugin + PluginSource() + idempotent warn-only installer; KAMACU_SESSION_ID/HOOK_TOKEN/HOOK_BASE env contract (D014) injected at the opencode spawn branch (manager.go); the engine-agnostic hook receiver (hooks.go) and the fake-opencode-stub/httptest-driver integration harness shape
affects:
  - S03 (session resume + argv regression hardening — reuses the build-tagged real-binary e2e harness pattern and the now-proven plugin→receiver loop)
key_files:
  - internal/opencode/kamacu-status.js
  - internal/opencode/plugin.go
  - internal/opencode/plugin_test.go
  - internal/opencode/e2e_test.go
key_decisions:
  - notify() uses global fetch() + AbortController(3s), removing the curl-on-PATH silent-no-op failure mode (D014 now accurate as-written)
  - claude's curl hook overlay in internal/session/agent.go is correct by design (PTY context) and explicitly untouched — the fix is opencode-plugin-only
  - the real-binary e2e is host-gated (//go:build opencode_e2e + t.Skip on absent opencode/provider) so it never affects default CI but passes on a host with opencode installed
patterns_established:
  - host-gated real-binary e2e harness: //go:build <tag> + t.Skip on missing binary/config + httptest receiver mirroring the production X-Kangent-Token contract + temp XDG_CONFIG_HOME + os.Environ KAMACU_* scrub for deterministic env-gate testing
  - negative-content regression guard: assert a forbidden substring (curl) is ABSENT in //go:embed bytes, permanently protecting a fixed silent-no-op defect
observability_surfaces:
  - hooksAlive canary now reliably flips on hosts WITHOUT curl on PATH (the load-bearing fix — previously the plugin silently no-oped there, pinning opencode tasks on working)
  - go test -tags opencode_e2e ./internal/opencode/ -run E2E — a reusable real-binary diagnostic for the plugin→receiver loop on any host with opencode installed
  - grep -c curl internal/opencode/kamacu-status.js == 0 — the regression guard for the fixed silent-no-op defect
drill_down_paths:
  - .gsd/phases/02-opencode-built-in-agent-engine/T01-SUMMARY.md
  - .gsd/phases/02-opencode-built-in-agent-engine/T02-SUMMARY.md
duration: ""
verification_result: passed
completed_at: 2026-07-07T19:20:24.240Z
blocker_discovered: false
---

# S02: Gated status plugin unlocks waiting and idle

**Switched the opencode status plugin from a curl shell-out (silent no-op on curl-less hosts, leaving tasks stuck on working) to fetch()+AbortController(3s) and added the first real-binary opencode 1.17.15 e2e harness proving the shipped plugin loads, its POSTs reach the unchanged receiver (Stop observed → idle unlocks), and the env-gate no-ops against the real binary.**

## What Happened

## What the slice delivered

S02 retires the plugin-half risk S01 explicitly deferred to "the milestone M002 UAT gate": it makes the shipped opencode status plugin actually work end-to-end and proves that loop against the real opencode binary. Two tasks, both green.

- **T01 — notify() curl→fetch fix (the load-bearing defect).** S01 shipped `notify()` in `internal/opencode/kamacu-status.js` as `await $\`curl -s -m 3 -H ${header} --data-binary ${payload} ${url}\`` (Bun `$` shell). On any host without `curl` on PATH the surrounding `catch {}` swallowed the spawn failure and the plugin SILENTLY no-oped, so `hooksAlive` never flipped and opencode tasks stayed pinned on `working` — waiting/idle never unlocked. This directly contradicted D014's "fetch() avoids a curl-on-PATH dependency" rationale and T03-PLAN step (iii). The fix rewrote `notify()` to POST via the global `fetch()` (method POST, `X-Kangent-Token` + `Content-Type` headers, `body=payload`) guarded by a 3s `AbortController` (`clearTimeout` in `finally`), keeping the outer try/catch that swallows all errors (a down/aborting receiver must never crash the opencode TUI). Every `curl` substring was removed from the plugin source and a **negative regression assertion** was added to `assertPluginContent` (`if contains(s,"curl") { t.Errorf(...) }` against the `//go:embed` bytes) so the silent-no-op defect cannot return. `fetch` + `AbortController` were added to `mustContain`. claude's curl hook overlay in `internal/session/agent.go:63` was correctly left untouched — claude runs in a PTY where curl is expected; the fix is opencode-plugin-only by design. Captured MEM030 (T03-SUMMARY's "Deviations: None" was inaccurate; D014 is now accurate as-written).

- **T02 — first real-binary opencode end-to-end proof.** S01 proved only the RECEIVER half (a fake hook driver POSTing the unchanged `hooks.go` flips an opencode session through working/waiting/idle). T02 proves the PLUGIN half against real opencode 1.17.15: `internal/opencode/e2e_test.go` (`//go:build opencode_e2e`, stdlib-only, `t.Skip` when `opencode` or a provider config is absent) installs the REAL shipped `PluginSource()` bytes (post-T01) into an isolated temp `XDG_CONFIG_HOME`, copies the user's real `opencode.json` with `mcp` stripped, and points `KAMACU_*` env at a self-contained httptest receiver that mirrors the production contract (constant-time `X-Kangent-Token` compare, `hook_event_name` decode, 204). `TestE2E_RealTurnPostsLifecycle` drives a real `opencode run --format json say hi` turn and asserts the receiver saw ≥1 POST AND `counts["Stop"] ≥ 1` (idle unlocked — the load-bearing assertion, not softened). `TestE2E_EnvGateNoopWhenSessionIDUnset` omits `KAMACU_SESSION_ID` and asserts the receiver recorded ZERO POSTs (D014 gate proven against the real binary). A `envWithoutKamacu()` helper scrubs pre-existing `KAMACU_*` from `os.Environ()` so the env-gate test is deterministic even when the test process itself runs inside a Kamacu session. Empirically (opencode 1.17.15 + zai provider) the loop-live receiver saw `map[SessionStart:1 Stop:1]` even though the turn's JSON output was a provider `UnknownError` — the session lifecycle still fired idle, so both assertions held. Captured MEM031 (a lifecycle-completing provider error still fires idle; only session-SETUP-class failures — auth/connection-refused — may emit only a bare unmapped `error`).

## Integration closure

- **Upstream consumed (S01):** the embedded opencode status plugin + `PluginSource()` + the idempotent warn-only installer; the D014 `KAMACU_SESSION_ID/HOOK_TOKEN/HOOK_BASE` env contract injected at the opencode spawn branch (`manager.go`); the engine-agnostic hook receiver (`hooks.go`) and S01's fake-opencode-stub/httptest-driver harness shape.
- **Delivered entirely within `internal/opencode/`:** a fetch-based plugin source and the first real-opencode lifecycle e2e. No changes to `hooks.go`, `session.go`, `manager.go`, or `agent.go` — confirmed green (claude's curl overlay in `agent.go` is intact, 1 reference).
- **Provides to S03:** a proven plugin→receiver loop and a reusable host-gated real-binary e2e harness pattern (build tag + httptest receiver mirroring the production token contract + temp `XDG_CONFIG_HOME` + `os.Environ` scrub).

## Verification

Slice-level verification run fresh through gsd_exec (verification lane), all green.

**Fast chain (evidence `2f660746`):**
1. **Content guards** — `TestPluginSourceMatchesFile` PASS. Direct source grep of `kamacu-status.js`: `fetch`=3, `AbortController`=2, `curl`=0. The curl-absent regression guard holds against the shipped `//go:embed` bytes.
2. **Default opencode suite** — `go test ./internal/opencode/ -count=1` → `ok kamacu/internal/opencode 0.011s`.
3. **Build + vet with e2e tag** — `go build -tags opencode_e2e ./internal/opencode/` → ok; `go vet -tags opencode_e2e ./internal/opencode/` → clean.
4. **Regression — api receiver** — `go test ./internal/api/ -run OpencodeHook -count=1` → `ok 0.161s` (hooks.go unchanged).
5. **Regression — session env-injection** — `go test ./internal/session/ -run Opencode -count=1` → `ok 0.089s` (manager.go/session.go unchanged).
6. **Production-code untouched guard** — `grep -c curl internal/session/agent.go` = 1 (claude's overlay correctly intact).

**Real-binary e2e (evidence `4d95698f`, opencode 1.17.15):**
7. `go test -tags opencode_e2e ./internal/opencode/ -run E2E -count=1 -v -timeout 240s` → BOTH PASS (16.3s): `TestE2E_RealTurnPostsLifecycle` PASS (9.37s, loop-live + Stop→idle observed) and `TestE2E_EnvGateNoopWhenSessionIDUnset` PASS (6.95s, zero POSTs — gate proven).

Per-task VERIFY.json both `passed:true, exit 0` (T01 `go test ./internal/opencode/`; T02 the full chain + real-binary e2e). No source edits were made in this closeout unit.

## Requirements Advanced

None.

## Requirements Validated

None.

## New Requirements Surfaced

None.

## Requirements Invalidated or Re-scoped

None.

## Operational Readiness

None.

## Deviations

None at the slice level for production code. Two task-level notes: (1) T02 added envWithoutKamacu() to scrub pre-existing KAMACU_* from os.Environ() before appending test-controlled env — a robustness hardening so the env-gate negative test is deterministic if the test host has KAMACU_* set; no change to assertions, the wire contract, or production code. (2) T01's fetch fix made D014's rationale accurate as-written (T03-SUMMARY's "Deviations: None" was inaccurate — captured as MEM030). hooks.go, session.go, manager.go, and agent.go are unchanged.

## Known Limitations

The LIVE browser status-dot transition (working→waiting→idle for a real opencode task) remains the human milestone M002 UAT gate — not automatable in CI. The e2e Stop/idle assertion depends on a lifecycle-completing turn: a session-SETUP-class provider failure (auth/connection-refused) may emit only a bare unmapped `error` and not fire Stop, so the test can fail on a transient setup error even though the plugin loop is healthy (documented accepted tradeoff for a host-gated test — MEM031; re-run before weakening the assertion). In opencode `run` mode `session.created` did not appear in logs, so SessionStart/hooksAlive is best-effort in the e2e (its receiver half is already proven by S01's api tests). hooksAlive remains the binary canary — no per-POST success-rate metric is emitted today.

## Follow-ups

Milestone UAT: run the deferred LIVE opencode UAT (browser status-dot transitions working→waiting→idle) once opencode is in the UAT environment — this slice's fetch fix is what makes it pass on curl-less hosts. S03 should reuse the build-tagged real-binary e2e harness pattern (e2e_test.go) for its session-resume proof and argv-regression hardening.

## Files Created/Modified

- `internal/opencode/kamacu-status.js` — notify() switched from curl shell-out to global fetch() + AbortController(3s); all curl substrings removed
- `internal/opencode/plugin.go` — package doc comment updated (curls → POSTs) so D014 is accurate as-written
- `internal/opencode/plugin_test.go` — assertPluginContent gains fetch+AbortController mustContain and a curl-absent negative regression assertion
- `internal/opencode/e2e_test.go` — new //go:build opencode_e2e real-binary harness: real-turn Stop→idle proof + env-gate no-op proof
