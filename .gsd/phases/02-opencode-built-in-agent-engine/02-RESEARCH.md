# S02 — Gated status plugin unlocks waiting and idle — Research

**Date:** 2026-07-07
**Risk:** high (real-binary end-to-end verification + one shipped defect found)

## Summary

S01 shipped the on-disk opencode status plugin (`internal/opencode/kamacu-status.js`) and proved the **receiver** half end-to-end: a fake hook driver POSTing to the unchanged `hooks.go` flips an opencode-engine session through working/waiting/idle. What S01 did **not** prove — and what this high-risk slice exists to retire — is the **plugin half**: that the shipped .js actually loads in real opencode, that its event-shape assumptions match opencode 1.17.15's real emissions, and that the POST reliably reaches the receiver so `hooksAlive` flips and waiting/idle *unlock*. S01 explicitly deferred the LIVE opencode loop to "the milestone M002 UAT gate."

Research against the **real opencode 1.17.15 binary installed on this host** (`/home/jordi/.opencode/bin/opencode`, `opencode debug paths` → config `~/.config/opencode`) plus the official opencode source types confirms the plugin's event-mapping contract is **correct**: `session.status` carries `properties.status.type ∈ {idle, retry, busy}`; `session.created` carries `properties.info.parentID`; `session.idle` is deprecated but still emitted; the `permission.ask` **named hook** (distinct from the `permission.asked` *event*) fires with `output.status = 'ask'` at prompt time. The reference production plugin `~/.config/opencode/plugin/slayzone-notify.js` validates the exact API shape the kamacu plugin mirrors. The plugin directory question is also settled: the opencode binary embeds the literal string `` `.opencode/plugin/` or `.opencode/plugins/` `` — it scans **both** — so the installer's singular `plugin/` path (MEM028) is valid.

One **real defect** surfaced: the shipped `notify()` uses `await $\`curl -s -m 3 -H … --data-binary … ${url}\`` (Bun `$` shell), which **depends on `curl` being on PATH** inside the opencode process. This contradicts both D014's rationale ("`fetch()` avoids a curl-on-PATH dependency; opencode plugins run in bun with global `fetch`") and T03-PLAN step (iii), which specified `fetch()` + a 3s `AbortController`. T03-SUMMARY's "Deviations: None" is inaccurate. On any host without `curl`, the plugin silently no-ops → `hooksAlive` never flips → waiting/idle never unlock. This is the load-bearing fix for S02.

## Recommendation

1. **Switch the plugin's `notify()` from `curl` (Bun `$`) to `fetch()` + `AbortController(3s)`**, exactly as D014/T03-PLAN specified. Bun provides global `fetch`; this removes the silent-no-op-on-curl-missing failure mode, honors the documented decision (no D014 amendment needed — switching *to* fetch makes the doc accurate), and is lighter than spawning a process per event. Update `plugin_test.go`'s `assertPluginContent` to pin `fetch` and assert the **absence** of `curl`.
2. **Add the first real-opencode end-to-end proof.** opencode 1.17.15 is installed here and a model provider is configured (`~/.config/opencode/opencode.json`), so S02 can retire S01's deferred-LIVE-UAT risk with a build-tagged `//go:build opencode_e2e` integration test (or a `gsd_uat_exec` UAT script) that spawns real `opencode run` with `KAMACU_*` env + the managed plugin installed, pointed at an `httptest` receiver, and asserts `SessionStart`/`Notification`/`Stop` arrive from a real turn. Keep it **out of default CI** (needs opencode + a live provider) via the build tag.
3. **No change to `hooks.go` or `session.go`** — the receiver and status machine are already correct and proven (S01 UAT-04). S02's work is entirely in `internal/opencode/` (plugin source + test) plus the new e2e harness.

The browser-visible LIVE status-dot transition remains the milestone UAT gate (human-follow-up), but the e2e harness shrinks the unverified gap from "the whole plugin loop" to "the browser rendering."

## Implementation Landscape

### Key Files

- `internal/opencode/kamacu-status.js` — the plugin source. **Only `notify()` changes** (curl → fetch + AbortController). Everything else (env gate, child suppression, event mapping, singleton guard) is confirmed correct against opencode 1.17.15 and stays. Optional: add a comment that `status.type === 'retry'` is intentionally ignored (transient; session still active).
- `internal/opencode/plugin_test.go` — `assertPluginContent` (line ~183) currently pins event names + headers but does **not** pin the wire mechanism. Add a `fetch` substring assertion and a negative assertion that `curl` is absent, so the fix can't silently regress.
- `internal/opencode/` (new file, e.g. `opencode_e2e_test.go`) — `//go:build opencode_e2e` integration test: install the plugin into a temp `XDG_CONFIG_HOME`, start an `httptest` receiver, spawn `opencode run "ping"` with `KAMACU_SESSION_ID/HOOK_TOKEN/HOOK_BASE`, assert the receiver observes `SessionStart` (→ `hooksAlive`) and at least `Stop` (turn end → idle); assert nothing fires when `KAMACU_SESSION_ID` is unset (env-gate no-op proof against the real binary). Skip cleanly if `opencode` is not on PATH (`exec.LookPath`).
- `internal/session/manager.go:182-187` — **reference only, no change**: the opencode env-injection site (`KAMACU_SESSION_ID/TOKEN/BASE`), proven by S01 `TestOpencodeSpawnInjectsHookEnv`. The e2e test replicates this env set.
- `internal/api/hooks.go` + `internal/api/opencode_hook_status_test.go` — **reference only, no change**: the unchanged receiver and the fake-driver regression suite. Re-run as regression.
- `.gsd/DECISIONS.md` (D014) — **verify, likely no edit**: once the plugin uses `fetch()`, D014's rationale ("fetch avoids curl-on-PATH dependency") is accurate as-written. Capture a memory noting the T03 curl→fetch correction so the historical record is honest.

### Build Order

1. **First: the `fetch()` fix in `kamacu-status.js` + `plugin_test.go` assertions.** This is the highest-risk/highest-unblock item — it's the difference between the plugin silently no-opping (curl missing) and reliably unlocking waiting/idle. Small, surgical, fully unit-testable without opencode. Unblocks the e2e harness (which would otherwise prove a curl-dependent loop).
2. **Second: the real-opencode e2e harness (build-tagged).** Retires the core S02 risk and S01's deferred-LIVE-UAT. Depends on (1) being merged so the loop under test is the real one. Probe `opencode` on PATH; skip if absent.
3. **Third (optional polish):** capture the empirical event-shape + dual-plugin-dir confirmations as memories; confirm D014 wording matches the now-fetch-based reality.

### Verification Approach

- `go test ./internal/opencode/ -count=1` → existing 7 install tests + updated content assertions (fetch pinned, curl absent) pass.
- `go test ./internal/api/ -run OpencodeHook -count=1` → receiver regression green (unchanged).
- `go test ./internal/session/ -run 'Opencode|Custom|Claude' -count=1` → env-injection + engine-gate regression green.
- `go test -tags opencode_e2e ./internal/opencode/ -run E2E -count=1` (NEW; skips if opencode absent) → real turn drives `SessionStart`/`Stop` (and `Notification` if a permission fires) into the `httptest` receiver; env-gate no-op variant asserts zero POSTs when `KAMACU_SESSION_ID` unset.
- LIVE browser UAT (human, milestone gate): opencode task → status-dot transitions working→waiting→idle; `hooksAlive=true` after first SessionStart.

## Constraints

- **Plugin runs in opencode's Bun runtime** (`/home/jordi/.opencode/bin/opencode`), which provides global `fetch` and the `$` template shell. `fetch` is available; `curl` is an external dep — prefer `fetch`.
- **No third-party Go deps.** The e2e harness uses stdlib `net/http/httptest` + `os/exec` only.
- **Receiver is token-gated** (`hooks.go`: empty token rejects all; constant-time compare). The e2e harness must inject the matching `X-Kangent-Token` via `KAMACU_HOOK_TOKEN`.
- **opencode `--pure` flag = "run without external plugins".** Kamacu does **not** pass `--pure` (custom command-render arm), so the local file plugin loads. Gotcha if anyone later adds `--pure` for hygiene.
- **Provider/network dependency** for a real model turn: the e2e test needs a working provider (`opencode.json` zai config) and network, or it must be content with session-lifecycle events that fire before/without a completed model response. The test must tolerate a turn that errors and still assert `session.created`/`session.status` reached the receiver.

## Common Pitfalls

- **Silent plugin no-op when `curl` missing** — the current defect. The plugin swallows all hook failures, so `hooksAlive` simply never flips and the task looks "stuck working." Switching to `fetch` removes the external dep; `assertPluginContent` must lock it in.
- **`session.busy` is dead in modern opencode** — the modern taxonomy emits `session.status` with `status.type==='busy'`; the top-level `session.busy` event the plugin also handles is legacy/forward-compat and won't fire on 1.17.15. Harmless (the `session.status` busy branch covers it) but don't rely on it. `session.idle` is likewise deprecated but still emitted — keep the branch as a fallback.
- **`status.type === 'retry'` is unhandled** — intentional (retry = still active, not idle). Add a comment so a future reader doesn't "fix" it by emitting Stop.
- **Child/subagent false-idle** — opencode tools spawn child sessions; their idle must not flip the root task idle. The plugin's `parentID`/`client.session.list()` suppression (mirrored from slayzone) handles this — preserve it verbatim in any rewrite.
- **`permission.ask` is a named HOOK, not the `permission.asked` EVENT.** The plugin correctly uses the named hook (gets `output.status`). Don't confuse the two; the event variant would only notify post-hoc.

## Open Risks

- **e2e flakiness / environment coupling**: a real-opencode test depends on the installed binary version, a live provider, and network. Mitigation: build-tag it out of default CI; skip on `exec.LookPath("opencode")` miss; assert lifecycle events (which fire regardless of model success) rather than a completed response.
- **opencode minor-version event drift**: 1.17.15's `session.status`/`session.idle` shapes are confirmed now, but opencode releases frequently. The plugin's dual-shape handling (modern `session.status` + legacy `session.idle`/`session.error`) is the forward-compat hedge; the e2e test pins the current contract.

## Sources

- opencode 1.17.15 installed binary + `opencode debug paths` (config dir = `~/.config/opencode`), `opencode plugin --help` (npm-module plugin install is a separate mechanism from local file plugins).
- opencode source `packages/opencode/src/session/status.ts` — `session.status` event schema `{ sessionID, status: { type: 'idle'|'retry'|'busy' } }`; `session.idle` marked deprecated. (github.com/anomalyco/opencode)
- opencode source `packages/opencode/src/session/session.ts` — `Session.Info` includes optional `parentID` (child-session detection). (github.com/anomalyco/opencode)
- opencode source `packages/plugin/src/index.ts` — `Hooks` interface: `event?`, `"permission.ask"?: (input, output: {status:'ask'|'deny'|'allow'}) => Promise<void>`. (github.com/anomalyco/opencode)
- opencode docs `plugins.mdx` — full event taxonomy (Permission: `permission.asked`/`permission.replied`; Session: `created`/`idle`/`status`/`error`/…); local plugins load from `~/.config/opencode/plugins/` (docs say plural; binary string `` `.opencode/plugin/` or `.opencode/plugins/` `` confirms BOTH are scanned).
- Reference production plugin `~/.config/opencode/plugin/slayzone-notify.js` (singular dir) — validates the plugin factory shape `async ({ $, client }) => ({ event, 'permission.ask' })`, `client.session.list()` → `{data:[...]}` with `.parentID`, singleton guard, child suppression.
- Shipped `internal/opencode/kamacu-status.js` line 71 — confirmed `await $\`curl ...\`` contradicts D014/T03-PLAN `fetch()`+AbortController.
