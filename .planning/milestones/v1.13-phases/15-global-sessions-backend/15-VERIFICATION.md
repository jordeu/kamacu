---
phase: 15-global-sessions-backend
verified: 2026-08-26T14:57:15Z
status: passed
score: 17/17 must-haves verified
behavior_unverified: 0
overrides_applied: 0
deferred:
  - truth: "Live-server curl story (PUT root → spawn global agent → status entry within poll → real server restart → exited+resumable entry → resume 201 same conversation)"
    addressed_in: "Phase 17"
    evidence: "Phase 17 goal: 'restart-resume for both engines… Closes the verification halves of the earlier phases' requirements' — SC2 'Restart-resume E2E' names the global claude/opencode resume and tmux invisible reattach explicitly. API-level equivalents are test-proven this phase (TestGlobalStatusPostRestart, TestGlobalAgentResumeArgv, TestGlobalOpencodeCaptureHost)."
  - truth: "Frontend consumption of the source:\"global\" status discriminator (bar row click-through, /global view)"
    addressed_in: "Phase 16"
    evidence: "Phase 16 SC5: 'A live global agent session appears as a row in the global Active Sessions bar… with click-through to /global'. Phase 15 mandates ZERO frontend changes (prohibition verified: no web/ files in diff)."
  - truth: "REQUIREMENTS.md checkbox closure for GVIEW-03/GSESS-01/GSESS-03/GINT-02 (marked Pending pending E2E confirmation)"
    addressed_in: "Phase 17"
    evidence: "REQUIREMENTS.md line 105: 'Phase 17 (Hardening & E2E) owns no requirements — it closes the end-to-end verification gates for GCONF-04, GSESS-02, GSESS-03, GINT-02, GINT-03, and the Phase-13 sentinel-leak invariant.' Phase-level implementation evidence for all four exists and is test-proven (see Requirements Coverage)."
---

# Phase 15: Global Sessions Backend — Verification Report

**Phase Goal:** The API-driven global scratchpad runs with full task parity — the session engine gains an additive global scope, `POST /api/sessions {scope:"global"}` spawns the agent and bash tabs in the global root, the status feed widens so the session is visible live and post-restart, and restart-resume works for both engines. The milestone's risk center: every task path must diff clean.
**Verified:** 2026-08-26T14:57:15Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

Must-haves merged from ROADMAP SC1–SC5 plus both PLAN frontmatters (9 truths in 15-01, 8 in 15-02; no roadmap SC was subtracted).

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | `POST /api/sessions {scope:"global"}` with no root → 409 "global root not configured" for bash AND tmux kinds — never a silent home/cwd fallback (D-28, D-30) | ✓ VERIFIED | sessions.go:462-464 (gate before kind dispatch); `TestGlobalSessionUnconfiguredRoot409` asserts both kinds + `mgr.List()` stays 0 — PASS |
| 2 | Configured root deleted since PUT → 409 "global root no longer exists on disk: \<path\>" verbatim (D-29) | ✓ VERIFIED | sessions.go:466-468; `TestGlobalSessionVanishedRoot409` asserts the path verbatim for both kinds — PASS |
| 3 | Global plain-bash + tmux bash spawn 201 with cwd = global root, same shell options as tasks (GVIEW-03); tmux mints `kamacu-global-<n>` from a scope-scoped counter (D-11/D-02) | ✓ VERIFIED | `TestGlobalSessionPlainBashSpawn` — real cwd proof via `pwd > marker` through the real input endpoint; `TestGlobalSessionTmuxMint` — DB row (NULL task_id, scope 'global', kamacu-global-1/2) + live tmux sessions probed; settings shell read shared with task path (sessions.go:609) |
| 4 | Cross-scope tmux reattach 404s both directions (D-32 scoped lookup) | ✓ VERIFIED | sessions.go:591-592 (`WHERE scope = 'global' AND name = ?`); `TestGlobalSessionCrossScopeReattach404` — PASS |
| 5 | `GET /api/sessions?scope=global` returns exactly global sessions with taskTitle "Scratchpad"/projectName "Global"; dev sessions keep EMPTY join fields — no 0-keyed leak (Pitfall 3) | ✓ VERIFIED | sessions.go:70, 141-152 (per-entry synthesis gated on `info.Global`; `joinSessionContext` skips `TaskID <= 0` so no 0-key ever exists); `TestGlobalSessionListScopedLabels` asserts scoped list + unscoped MCP-surface labels + dev-empty-fields regression — PASS |
| 6 | `GET /api/global` live.agent/live.bash count RUNNING global PTYs (D-19); PUT root change while a global session runs → 409 with reasons (GCONF-04) | ✓ VERIFIED | global.go:238-263 `deriveGlobalState` consumes `ListGlobal()` (zero-seams gone: 0 matches for `Live.Agent = 0`/`Live.Bash = 0`); globalLiveBlockers dedupes by tmux name (global.go:123-149); `TestGlobalSessionLiveCountsAndRootGate` asserts live.bash ≥ 1 → 409 with reasons → stop → 200 — PASS |
| 7 | Surviving global tmux row surfaces post-restart-sim as orphaned entry with Global:true via scoped reconcile (GSESS-03) | ✓ VERIFIED | `reconcileGlobalTmux` (sessions.go:159-224) synthesizes `Orphaned:true, Global:true` with defaultTmuxLabel fallback; `TestGlobalSessionRestartOrphan` (tmux-guarded, ran on this host) — PASS |
| 8 | Scope stop kills exactly global RUNNING sessions; dev + task survive; ListGlobal lists only global (GSESS-01 scope half) | ✓ VERIFIED | manager.go:503-522 `StopAllForScope` (predicate `s.global`); `TestStopAllForScope`/`TestListGlobal`/`TestGlobalScopedLabels` engine tests — PASS |
| 9 | Every existing task/dev test passes — full suites green (task-path regression gate) | ✓ VERIFIED (see Deviation note) | Full repo `go test ./internal/... -count=1 -p 1` — 14/14 packages ok (api 215s, ws 69s, session 8.5s, mcp, reaper, …). One documented deviation (DEV-1) modified `sessions_test.go` in lockstep with a spec-correct bug fix — see Anti-Patterns/Notes |
| 10 | `{scope:"global", kind:"agent"}` spawns configured default agent with cwd = global root, label "Agent", engine read-at-use (D-24, GVIEW-02) | ✓ VERIFIED | `TestGlobalAgentSpawn` — fake-claude pwd-file cwd proof, label/engine/global flags, csid persisted to singleton, tasks table stays empty; read-at-use proven by `TestGlobalAgentResumeNoId409` flipping the singleton agent via PUT with no restart |
| 11 | Second CONCURRENT global agent spawn → 409 "global agent already running" (D-34); exited never blocks | ✓ VERIFIED | sessions.go:478-484 (RUNNING-only over `ListGlobal()`); `TestGlobalAgentOneAgentGate` — concurrent 409, stop, fresh 201 replacement — PASS |
| 12 | resume:true with no persisted engine id → 409 "no global \<engine\> session to resume" — never silent fresh spawn (D-31); valid claude csid → argv carries `--resume <csid>` (GSESS-02) | ✓ VERIFIED | sessions.go:502-524 engine-branched; `TestGlobalAgentResumeNoId409` (both engines, nothing spawned), `TestGlobalAgentResumeArgv` (--resume pair + id stability) — PASS |
| 13 | csid persisted to global_task (warn-only); opencode capture writes global_task.opencode_session_id — never the tasks table (re-target, not a forked poller) | ✓ VERIFIED | sessions.go:700-727, 1035-1085 — exactly ONE `captureOpencodeSessionAsync`, parametrized `taskPersistTarget`/`globalPersistTarget` closures over separate SQL; `TestGlobalAgentOpencodeCapture` — singleton written, real task row stays NULL (wrong-table detector), poll dir = global root; `TestGlobalOpencodeCaptureHost` — REAL opencode 1.18.x e2e, ses_ id captured — PASS on this host |
| 14 | Live global agent appears in /api/agents/status with source "global", taskId 0, projectId 0, projectName "Global", taskTitle "Scratchpad", non-empty sessionId (SC3 live pass) | ✓ VERIFIED | agents.go:173-196 (Pass 1b, collected OUTSIDE the task-keyed map — Pitfall 3); `TestGlobalStatusLiveEntry` asserts every field + task-entry byte-stability peer — PASS |
| 15 | Post-restart, exactly ONE global entry with status "exited"/resumable true, engine-branched — no duplication (SC3 DB pass, GSESS-02, Pitfall 1) | ✓ VERIFIED | agents.go:270-284 (Pass 2b gated on `!hasGlobalAgent`), `globalResumeState.resumable` over root_path (never worktree_path — 0 matches in the global funcs); `TestGlobalStatusPostRestart` (claude+opencode+2 negatives), `TestGlobalStatusNoDuplication` (live+resumable and exited-in-manager) — PASS |
| 16 | Global sessions never appear in Activity stats/lists (GINT-03) and no reaping pass was added (GSESS-04) | ✓ VERIFIED | `internal/reaper/reaper.go` NOT in the phase diff; activity queries task-keyed and untouched; `TestGlobalActivityExclusion` — RUNNING global agent+bash absent while a done task appears — PASS |
| 17 | Every existing task/dev suite passes — whole repo green | ✓ VERIFIED (see Deviation note) | Same full-repo run as truth 9; agents.go task JOINs byte-identical (diff shows only the additive global_task JOIN); task passes preserved in `else` branches (read at sessions.go:485-539, agents.go:94-153, 198-262) |

**Score:** 17/17 truths verified (0 present-but-behavior-unverified)

### Roadmap Success Criteria Cross-Check

| SC | Criterion | Status | Evidence |
|----|-----------|--------|----------|
| 1 | Global agent spawn in root cwd + bash tabs task-parity + concurrent 409 | ✓ VERIFIED | Truths 3, 10, 11 |
| 2 | Full task-parity semantics (PTYs, replay, status) + scope-targeted Stop, never `StopAllForTask(0)` | ✓ VERIFIED | Truths 3, 8; `grep -rn 'StopAllForTask(0)'` → zero matches; global sessions are ordinary manager sessions (ring/attach/ws keyed by session id, no task branching) |
| 3 | Status feed: synthesized non-nullable label in BOTH passes | ✓ VERIFIED | Truths 14, 15 |
| 4 | Restart reconcile + engine-branched resume; tmux invisible reattach | ✓ VERIFIED | Truths 7, 12, 13 (restart-sim level; live-server E2E = Phase 17, deferred) |
| 5 | Never auto-reap; MCP honest labels; Activity exclusion | ✓ VERIFIED | Truths 5, 16; MCP bridge is a verbatim passthrough of GET /api/sessions[/{id}] (internal/mcp/sessions.go:516, 579) — the test-proven API labels flow through unchanged; mcp package suite green |

### Deferred Items

| # | Item | Addressed In | Evidence |
|---|------|-------------|----------|
| 1 | Live-server curl story (real restart, resume same conversation) | Phase 17 | Phase 17 SC2 "Restart-resume E2E" names exactly this flow for both engines + tmux |
| 2 | Frontend consumption of source:"global" | Phase 16 | Phase 16 SC5 (bar row + /global click-through); Phase 15 prohibition: zero web/ changes — verified held |
| 3 | REQUIREMENTS.md E2E-half closure for GVIEW-03/GSESS-01/GSESS-03/GINT-02 | Phase 17 | REQUIREMENTS.md:105 assigns the E2E verification gates to Phase 17 |

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/api/sessions_global_test.go` | New test suite covering all phase behaviors | ✓ VERIFIED | 1486 lines, 21 test functions, substantive assertions (cwd proofs, DB row shapes, wrong-table detectors, byte-stability peers); all PASS |
| `session.SpawnOpts.Global` | Additive scope flag w/ zero-value back-compat contract | ✓ VERIFIED | manager.go:81-86 with contract comment; set at :365; opaque to argv/PTY machinery |
| `session.Info.Global` (json `global,omitempty`) | Wire discriminator | ✓ VERIFIED | session.go:69-73, populated at :219; `global:true` asserted on the wire in 5 tests; omitempty keeps task/dev payloads byte-identical |
| `session.Manager.ListGlobal()` / `StopAllForScope()` | Scope-scoped listing + scope-targeted stop | ✓ VERIFIED | manager.go:442-444, 503-522; consumed by sessions.go, global.go, agents.go (5 references in global.go alone) |
| `captureOpencodeSessionAsync` persist-target parameter | Parametrized poller, never forked | ✓ VERIFIED | Exactly 1 definition (sessions.go:1087); `opencodePersistTarget` closures (:1043-1072) |

### Key Link Verification

| From | To | Via | Status |
|------|----|----|--------|
| create() global branch | Manager.Spawn | singleton SELECT (global_task JOIN agents, id=1) → D-28/D-29 gates → `opts.Cwd` + `opts.Global` | ✓ WIRED (sessions.go:438-471; spawned sessions verified in-manager by tests) |
| mgr.ListGlobal() | list ?scope=global / deriveGlobalState / globalLiveBlockers / one-agent gate | list branch (sessions.go:70), derive (global.go:252), blockers (global.go:130), D-34 gate (sessions.go:479) | ✓ WIRED (4 distinct consumers, all exercised by passing tests) |
| info.Global | per-entry synthesized labels in sessionDetail loop + getSession | sessions.go:141-152 + getSession global branch | ✓ WIRED (TestGlobalSessionListScopedLabels covers both surfaces' shared shape) |
| create() global agent branch | global_task persists (csid UPDATE + capture re-target) | sessions.go:700-727 | ✓ WIRED (TestGlobalAgentSpawn, TestGlobalAgentOpencodeCapture, TestGlobalOpencodeCaptureHost) |
| mgr.List() global scan + singleton read | ONE synthesized agentStatusEntry per pass | agents.go:161-196 (Pass 1b) + 270-284 (Pass 2b) | ✓ WIRED (both passes field-asserted) |
| agentStatusEntry.Source "global" | Phase-16 discriminator | wire value flows today; frontend is Phase 16 (prohibition: no premature TS changes — held) | ✓ WIRED (backend half complete) |
| MCP session tools | GET /api/sessions passthrough | internal/mcp/sessions.go:516, 579 — verbatim JSON relay, no scope logic to drift | ✓ WIRED (labels ride the test-proven API response) |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|-------------------|--------|
| ?scope=global list | `out []sessionDetail` | `mgr.ListGlobal()` + `reconcileGlobalTmux` + `globalSessionContext()` singleton JOIN | Yes — real engine sessions + real DB rows + real agent name ("Claude Code" seed asserted) | ✓ FLOWING |
| /api/agents/status global entries | `entries []agentStatusEntry` | Pass 1b: live `mgr.List()` scan; Pass 2b: `loadGlobalResumeState()` singleton read | Yes — sessionId/status from live PTY sessions; csid/ocsid/root from singleton | ✓ FLOWING |
| GET /api/global live block | `g.Live.{Agent,Bash,Tmux}` | `mgr.ListGlobal()` + `liveGlobalTmuxNames` probe | Yes — asserted ≥1/0 transitions with real session lifecycle | ✓ FLOWING |

### Behavioral Spot-Checks

Single named-suite runs plus ONE full-suite run (`-p 1` serialization, env stripped per the Kamacu hook-env constraint):

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Engine scope (labels/ListGlobal/StopAllForScope) | `go test ./internal/session -run 'TestGlobalScopedLabels\|TestListGlobal\|TestStopAllForScope' -count=1 -v` | 3/3 PASS (0.70s) | ✓ PASS |
| Global API suite (spawn/gates/labels/counts/restart/agent/resume/capture/status/activity) | `go test ./internal/api -run 'TestGlobalSession\|TestGlobalAgent\|TestGlobalStatus\|TestGlobalActivity\|TestGlobalOpencodeCaptureHost' -count=1 -v` | 21/21 PASS incl. host-gated real-opencode e2e (36.8s) | ✓ PASS |
| Task-path regression gate (milestone risk center) | `go test ./internal/... -count=1 -p 1` | 14/14 packages ok (api 215.0s, ws 69.7s, session 8.6s, mcp 0.7s, reaper 1.2s, …) | ✓ PASS |

### Probe Execution

Not applicable — no `scripts/*/tests/probe-*.sh` declared in PLAN/SUMMARY and no probe-based verification claimed; the plans' verification contract is the Go test suites (executed above).

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| GSESS-01 | 15-01 | Engine scope, task-parity semantics, scope-targeted Stop | ✓ SATISFIED | Truths 3, 8; `StopAllForTask(0)` grep = 0; engine tests PASS |
| GSESS-02 | 15-02 | Restart reconcile + engine-branched resume keyed on singleton ids | ✓ SATISFIED | Truths 12, 13, 15 (both engines; host e2e for capture) |
| GSESS-03 | 15-01 | Global tmux tabs survive restart, reattach invisibly | ✓ SATISFIED | Truths 4, 7 (restart-sim + reattach tests; live-server E2E = Phase 17) |
| GSESS-04 | 15-02 | Never auto-reaped — immortal until explicit stop | ✓ SATISFIED | Truth 16 (reaper.go untouched, no reap pass added, exclusion pinned) |
| GVIEW-02 | 15-02 | Agent tab spawns default agent in global root; 409 on second concurrent | ✓ SATISFIED | Truths 10, 11 (pwd cwd proof; concurrent-only 409) |
| GVIEW-03 | 15-01 | Bash tabs in global root with same shell options as tasks | ✓ SATISFIED | Truth 3 (settings shell shared; cwd proof; kamacu-global-\<n\> mint) |
| GINT-02 | 15-01 | MCP session tools handle global sessions with honest labels, read-only contract unchanged | ✓ SATISFIED | Truth 5 + MCP passthrough wiring (Key Links); mcp suite green |
| GINT-03 | 15-02 | Activity stats/lists exclude global sessions | ✓ SATISFIED | Truth 16 (TestGlobalActivityExclusion) |

Orphaned requirements: none — the roadmap's 8 requirement IDs for Phase 15 exactly equal the union of the two plans' `requirements` fields.

### Prohibition Compliance (all 12 verified)

| Prohibition | Evidence |
|-------------|----------|
| No side-channel global spawn endpoint | routes wiring not in phase diff; `scope` rides `POST /api/sessions` body only |
| Never `StopAllForTask(0)` | repo grep → zero matches; scope stop uses `StopAllForScope` |
| No LEFT-JOIN surgery on hot list/status queries | `joinSessionContext` and agents.go task JOINs byte-identical in diff; global labels synthesized per-entry |
| No silent home/cwd fallback | D-28/D-29 gates before kind dispatch; 409 tests for bash+tmux+agent kinds |
| No 0-keyed ctxByTask entry | `joinSessionContext` skips `TaskID <= 0`; list loop branches on `info.Global` before map lookup; dev-empty-fields regression test |
| No raw tmux has-session execs | zero `exec.Command` tmux matches in the touched api files; probes via `tmux.Client.HasSession` |
| One capture poller, parametrized | exactly 1 `func captureOpencodeSessionAsync`; closure targets |
| No worktree_path in global branches | 0 matches in `globalResumeState`/`resumable`/`loadGlobalResumeState`; root_path plays the role |
| agents.go append-only | diff: task passes untouched, only additive global branches + the singleton JOIN |
| Global entries outside newest[TaskID] map | separate `globalAgent`/`hasGlobalAgent` variables; at-most-one test |
| No auto-reap / PR reconcile analog | reaper.go not in diff; activity exclusion test |
| Zero frontend changes | no `web/` files in the phase diff |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| internal/api/sessions_test.go | 2112-2130, 2246-2330 | Existing guard test modified (not additive) — DEV-1 lockstep fix of pre-existing bracketed-paste delimiter bug (ESC[2004~ → ESC[200~; xterm spec: 2004 is the MODE, 200~/201~ are the DELIMITERS) | ⚠️ Warning (documented deviation, spec-correct) | None negative: fix is empirically required by this phase's own cwd-proof spec (plain-bash input was broken for every consumer); test constants corrected to spec, arithmetic updated, assertions STRENGTHENED. Isolated commit 67f39ef with full rationale. Violates the letter of "byte-for-byte untouched" for one test file — see Override suggestion below |
| internal/api/sessions_global_test.go | 233, 590 | 8s stop-wait deadlines (DEV-2) | ℹ️ Info | D-14 grace reality (interactive bash ignores SIGTERM); documented, no impact |

**Override suggestion (developer acceptance recommended, not required for phase pass):** the "byte-for-byte untouched" letter of truths 9/17 is broken once by DEV-1. The behavior it guards (no masked task-path regressions) holds — full suites green, task JOINs byte-identical, task logic preserved in `else` branches. To formalize acceptance, add to this file's frontmatter:

```yaml
overrides:
  - must_have: "Every existing task/dev test passes byte-for-byte untouched"
    reason: "DEV-1: sessions_test.go guard-test constants corrected in lockstep with the spec-correct bracketed-paste delimiter fix (67f39ef) — required by this phase's cwd-proof spec; assertions strengthened, not weakened"
    accepted_by: "{name}"
    accepted_at: "{ISO timestamp}"
```

### Coverage Seams (informational — no action required)

- `deriveGlobalState`'s `Live.Agent` arm has no direct ≥1 assertion (bash and tmux arms are test-proven; the agent arm is the same switch in the same loop, and live.agent==0 is asserted when no agent runs).
- Global opencode resume argv (`-s` append) is covered by composition: the append lives in the SHARED custom-agent arm (sessions.go:572-574) locked by the task-path argv test, and the global singleton-id feed into that branch is proven by the engine-branched 409 test. The dedicated global `-s` argv assertion arrives with Phase 17's both-engine resume E2E.
- REQUIREMENTS.md checkbox states (GVIEW-03/GSESS-01/GSESS-03/GINT-02 "Pending") track the Phase-17 E2E halves, not missing implementation — phase-level evidence exists for all 8 (table above).

### Human Verification Required

None. This is a backend-only phase (zero frontend changes mandated and verified); every behavior-dependent truth is exercised by a passing behavioral test (gates via HTTP, cwd proofs via the real input endpoint, stop isolation at the engine, status entries field-by-field, capture against the real opencode binary on this host). The live-server curl story is explicitly Phase 17 scope (deferred table).

### Gaps Summary

No gaps. All 17 truths verified with behavioral test evidence; all artifacts exist, are substantive, wired, and flowing; all 12 prohibitions hold with deterministic grep/diff/test evidence; all 8 requirement IDs accounted for with no orphans; the milestone risk center (task paths diff clean) holds — full repo green under `-p 1`, task JOINs byte-identical, the single task-path code change being a documented spec-correct bug fix whose guard-test update strengthened assertions.

---

_Verified: 2026-08-26T14:57:15Z_
_Verifier: the agent (gsd-verifier)_
