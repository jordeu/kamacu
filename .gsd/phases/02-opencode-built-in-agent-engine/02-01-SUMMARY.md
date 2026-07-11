---
id: S01
parent: M002
milestone: M002
provides:
  - selectable opencode built-in agent engine (system seed, non-deletable)
  - KAMACU_SESSION_ID/HOOK_TOKEN/HOOK_BASE env contract (D014) injected at opencode spawn
  - env-gated on-disk opencode status plugin + idempotent warn-only installer
  - opencode as a full-heuristics engine in the status machine (working/waiting/idle)
  - reusable fake-opencode stub + httptest hook-driver integration test harness
requires:
  - slice: M001
    provides: agents table + projects.agent_id FK (R012/R013), manager.Spawn engine switch, engine-agnostic hook receiver, session.go status machine + agentStatusLocked, AgentConfig BaseURL/Token
affects:
  - S02 (gated status plugin unlocks waiting/idle — builds on this plugin + hooksAlive canary)
  - S03 (session resume + argv regression hardening — reuses spawn env contract + fake-opencode stub pattern)
key_files:
  - internal/store/migrations/00015_opencode_agent.sql
  - internal/api/agents_backfill.go
  - cmd/kamacu/main.go
  - internal/store/opencode_agent_migration_test.go
  - internal/session/manager.go
  - internal/session/session.go
  - internal/session/opencode_engine_test.go
  - internal/opencode/kamacu-status.js
  - internal/opencode/plugin.go
  - internal/opencode/plugin_test.go
  - internal/api/opencode_hook_status_test.go
key_decisions:
  - opencode reuses the custom command-render arm verbatim and differs only by D014 env injection (no separate opencode spawn branch)
  - opencode status hooks ship as an env-gated on-disk FILE plugin (opencode can't take a per-instance hook command via argv); no-ops for non-Kamacu runs
  - opencode seed is is_default=0 so claude stays the sole default (R019); is_system=1 makes it non-deletable (R018)
  - plugin install is warn-only at startup (degrade-don't-break); opencode uses singular plugin/ dir (MEM028)
  - agentStatusLocked custom-collapse gate excludes opencode so it reaches full working/waiting/idle heuristics (D013)
patterns_established:
  - second system-agent seed: migration + in-process Backfill mirror of BackfillAgents, with NOT EXISTS guard for partial-apply recovery
  - engine-gated env injection reusing the custom command-render arm (no new spawn branch)
  - env-gated managed plugin-on-disk pattern: embed + idempotent skip-on-match installer, warn-only at startup, no-pollution (write only when config dir exists)
  - fake-opencode stub + httptest hook-driver integration harness reusable by S02/S03
observability_surfaces:
  - GET /api/sessions .status (opencode-engine now working/waiting/idle, not running)
  - hooksAlive canary on session info (flips true on SessionStart hook POST; the plugin->receiver loop diagnostic)
  - SELECT name,engine,command,is_default,is_system FROM agents WHERE engine='opencode'
  - managed plugin file ~/.config/opencode/plugin/kamacu-status.js (kamacu-managed header)
  - slog WARN from opencode.InstallPlugin() at startup (warn-only, never fatal)
drill_down_paths:
  - .gsd/phases/02-opencode-built-in-agent-engine/T01-SUMMARY.md
  - .gsd/phases/02-opencode-built-in-agent-engine/T02-SUMMARY.md
  - .gsd/phases/02-opencode-built-in-agent-engine/T03-SUMMARY.md
  - .gsd/phases/02-opencode-built-in-agent-engine/T04-SUMMARY.md
duration: ""
verification_result: passed
completed_at: 2026-07-07T18:43:24.255Z
blocker_discovered: false
---

# S01: opencode engine spawn and activity-based status

**Added opencode as a third built-in agent engine that spawns in a task worktree PTY and reports working/waiting/idle status by reusing the existing claude hook receiver unchanged via an env-gated on-disk plugin; claude and custom spawn paths are byte-for-byte unchanged.**

## What Happened

## What the slice delivered

S01 adds **opencode** as a first-class built-in agent engine alongside claude and custom. An opencode task spawns a real opencode process in the task worktree PTY (same shell-out path as custom) and reports **working/waiting/idle** status — not the custom "running" collapse — driven by an env-gated on-disk opencode plugin that curls the **unchanged** claude hook receiver with claude-compatible event names. The opencode seed appears in the existing M001 agent dropdowns with no UI change.

Four tasks, all green:

- **T01 — opencode system-agent seed (migration 00015 + BackfillOpenCodeAgent).** Seeded a non-deletable (`is_system=1`), opt-in (`is_default=0`) opencode row, mirroring the claude seed (00013 + BackfillAgents). R019 (claude sole default) and R018 (non-deletable) are asserted directly. A NOT EXISTS guard gives partial-apply recovery on top of the goose version table. Wired into startup right after `BackfillAgentExtraParams`. Regression-fixed 3 shared tests that assumed a single-system-agent baseline (now 2) — all intent-preserving per-engine/per-flag assertions.

- **T02 — spawn env injection + status gate (the only two Go runtime changes).** `manager.Spawn` opencode branch injects `KAMACU_SESSION_ID=<id>`, `KAMACU_HOOK_TOKEN=<cfg.Token>`, `KAMACU_HOOK_BASE=<cfg.BaseURL>` (the D014 contract), gated on `AgentEngine=="opencode"`; the custom and claude arms are byte-for-byte unchanged (verified by `TestCustomEngineDoesNotGetHookEnv`). `agentStatusLocked` gate widened by one token (`&& s.engine != "opencode"`) so opencode reaches the full working/waiting/idle heuristic path instead of collapsing to "running". Status machine itself untouched.

- **T03 — env-gated on-disk plugin (internal/opencode leaf).** `kamacu-status.js` is a complete no-op unless Kamacu spawned the process (reads KAMACU_*; returns {} if any missing) — invisible to non-Kamacu opencode users. When Kamacu-spawned, it maps opencode events onto the exact hook_event_name values the unchanged receiver switches on: `session.created` (non-child)→SessionStart, `permission.ask status=ask`→Notification, `session.idle`/`session.error`/`session.status idle`→Stop. Child/subagent sessions are suppressed (parentID check) so a tool-spawned subagent can't flip the root task idle early. `InstallPlugin()` embeds the .js, writes it to `${XDG_CONFIG_HOME:-~/.config}/opencode/plugin/kamacu-status.js` **only when the opencode config dir already exists** (no-pollution), skip-on-match idempotent, overwrite-stale on upgrade, **warn-only** at startup (degrade-don't-break). Empirically verified opencode uses the **singular** `plugin/` dir (MEM028), correcting the CONTEXT prose.

- **T04 — hook-status integration proof.** `internal/api/opencode_hook_status_test.go` spawns real opencode-engine sessions and drives the **unchanged** `HookRoutes` over httptest — hooks.go is neither imported nor modified, proving engine-agnosticism by construction. Five cases cover the heuristic-state contract, Notification→waiting, Stop→idle, the hooksAlive canary (SessionStart disables the BEL fallback), and the full one-turn loop.

## Integration closure

- **Upstream consumed (M001):** the `agents` table + `projects.agent_id` FK (R012/R013); `manager.Spawn` engine switch; the engine-agnostic hook receiver (`POST /api/hooks/sessions/{id}`); `session.go` status machine + `agentStatusLocked`; `AgentConfig` (BaseURL/Token).
- **New wiring:** migration 00015 + `BackfillOpenCodeAgent()`; `internal/opencode.InstallPlugin()` at startup; opencode env injection (engine-gated); `agentStatusLocked` opencode exclusion.
- **Regression:** fake-claude suite (R016) and custom-engine suite (R017) pass **unchanged** — the negative test `TestCustomEngineDoesNotGetHookEnv` is the byte-for-byte guard.
- **Deferred to milestone UAT:** a LIVE opencode UAT (real opencode binary in a worktree → plugin loads → curls hook → status-dot transitions in the browser) needs opencode installed and is **not** automatable in CI; it is the milestone's UAT gate, not this slice's. The Go-side status contract is fully proven here.

## Key decisions

- opencode reuses the custom command-render arm verbatim and differs only by env injection (no separate opencode spawn branch).
- opencode status hooks come from an on-disk FILE plugin (opencode can't take a per-instance hook command via argv), env-gated to no-op for non-Kamacu runs.
- Plugin install is warn-only (degrade-don't-break); a missing/unreachable plugin leaves the session on activity-based status with the BEL fallback armed.
- opencode seed is `is_default=0` — claude remains the sole default (R019), so existing/new projects are never silently switched.

## Operational Readiness (Q8)

**Health signal (proves the slice works):**
- `GET /api/sessions` `.status` for an opencode-engine task reflects hook-driven **working/waiting/idle** (previously collapsed to `running` under the custom gate).
- `hooksAlive` canary: flips true only when a `SessionStart` hook POST reaches the receiver — the definitive diagnostic that the plugin → env → hook-receiver loop is wired. If it never flips after spawn, the plugin/env path is broken.
- `SELECT name,engine,command,is_default,is_system FROM agents WHERE engine='opencode'` shows the seeded row (idempotent across restarts).
- The managed plugin file `~/.config/opencode/plugin/kamacu-status.js` exists and carries the `kamacu-managed` header (skip-on-match keeps mtime stable on healthy boots).

**Failure signal (triggers attention):**
- opencode session **stuck on `working`** with `hooksAlive=false` (BEL fallback armed) → plugin missing/unreachable OR `KAMACU_SESSION_ID` unset OR `~/.config/opencode` absent (no-pollution gate means the plugin is only written when that dir exists).
- Startup `slog` WARN from `opencode.InstallPlugin()` (warn-only, never `os.Exit`) → plugin could not be written (permissions / read-only home).
- A typo'd engine name routes to custom: no env injection, collapses to `running` — safe-by-default but silently inert (caught by the engine-gate regression test).

**Recovery procedure:**
1. Confirm the opencode CLI is on PATH (`command -v opencode`).
2. Confirm `~/.config/opencode/` exists; if not, run `opencode` once to create it, then **restart kamacu** — `InstallPlugin()` re-runs (skip-on-match or overwrite-stale) and `BackfillOpenCodeAgent()` re-seeds if dropped.
3. Verify the plugin file matches the embedded source (`kamacu-managed` header present).
4. For an opencode task stuck on `working`: check `hooksAlive` in the session info and whether status ever reaches `idle`; if `hooksAlive=false`, the plugin/env loop is the fault locus.
5. `KAMACU_HOOK_TOKEN` is injected via env and posted only to loopback; never logged (slog) — same trust boundary as claude's `--settings` overlay.

**Monitoring gaps:**
- No success-rate metric/counters are emitted for hook POSTs (a future enhancement would surface plugin→receiver reliability). Today, `hooksAlive` is the binary canary.
- The LIVE opencode binary path is unverifiable in CI (no opencode in the test env); it is the milestone UAT's responsibility.

## Provides / Affects

- **Provides to S02/S03:** a selectable opencode engine; the KAMACU_* env contract (D014); the on-disk plugin + installer; opencode as a full-heuristics engine in the status machine; the integration test harness (fake-opencode stub + httptest hook driver) reusable by S02/S03.
- **Affects:** S02 (gated status plugin unlocks waiting/idle — builds directly on this plugin + the hooksAlive canary); S03 (session resume + argv regression hardening — reuses the spawn env contract and the fake-opencode stub pattern).

## Verification

Slice-level integration verification run through gsd_exec (verification lane), all green:

1. **Build + vet + focused suites** (evidence `f9843b07`): `go build ./...` → BUILD_OK; `go vet ./...` → VET_OK; `go test ./internal/store/ ./internal/session/ ./internal/opencode/ ./internal/api/ -run 'Opencode|OpenCode|Hook|Custom|Claude' -count=1` → all four packages `ok`.

2. **Must-have artifact inspection** (evidence `c7e50508`): confirmed in source — migration 00015 seeds `engine='opencode'`, `is_default=0`, `is_system=1` with a NOT EXISTS guard; `BackfillOpenCodeAgent` (agents_backfill.go:87) + `opencode.InstallPlugin()` wired in main.go (L161, L177); manager.go opencode env gate (L182) injects KAMACU_SESSION_ID/TOKEN/BASE; session.go agentStatusLocked gate widened to exclude opencode (L324); plugin.go uses singular `plugin/` + `//go:embed`; kamacu-status.js env-gated with claude-compatible event names + child suppression; **hooks.go has 0 opencode refs** (engine-agnostic receiver unchanged).

3. **Per-task verify evidence** (T01–T04 VERIFY.json, all `passed:true, exit 0`): store migration/backfill (T01), session opencode/custom/claude suites (T02), opencode plugin (T03), api opencode-hook integration (T04).

Regression guards confirmed: fake-claude suite (R016) and custom-engine suite (R017) pass unchanged; `TestCustomEngineDoesNotGetHookEnv` proves the custom arm is byte-for-byte unchanged.

No source edits were made in this closeout unit.

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

None at the slice level. (T01 regression-fixed 3 shared test files that assumed a single-system-agent baseline — an unavoidable, intent-preserving consequence of seeding a second system agent, captured as MEM026. T03 corrected the CONTEXT-prose `plugins/` → singular `plugin/` per empirical verification, captured as MEM028.)

## Known Limitations

The LIVE opencode UAT (real binary in a worktree → plugin loads → status-dot transitions in the browser) needs opencode installed and is NOT automatable in CI; deferred to the milestone M002 UAT gate. No success-rate metrics are emitted for hook POSTs today — hooksAlive is the binary canary. A dedicated 'opencode' engine badge label in the UI is cosmetic and may be polished later.

## Follow-ups

Milestone UAT: run the deferred LIVE opencode UAT (real binary → browser status-dot transitions) once opencode is installed in the UAT environment. Optional future polish: a dedicated 'opencode' engine badge label; hook-POST success-rate metrics.

## Files Created/Modified

None.
