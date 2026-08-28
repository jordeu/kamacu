---
phase: 17-hardening-e2e
plan: "03"
subsystem: testing
tags: [go-test, env-hygiene, secret-propagation, d-63, d-62, type-safety, suite-baseline]

requires:
  - phase: 15-global-sessions-backend
    provides: the D014 KAMACU_* hook-env contract and its opencode gate (manager.go), the TestInput_* family healed by the delimiter fix
  - phase: 16-global-view-settings-bar-integration
    provides: 16-01 Pitfall 2 (the stale Agent.engine union flag), the /global locked claude-only predicate D-62 aligns with
  - phase: 17-hardening-e2e/01
    provides: the deferred-items register (tmux PATH wrapper seam, first flake evidence) this plan owns
  - phase: 17-hardening-e2e/02
    provides: the flake recurrence evidence (deferred item 3) feeding the baseline
provides:
  - The suite-green substrate for the rest of Phase 17 — go test ./... exits 0 BOTH with and without the KAMACU_* trio exported (the hardening phase's own bar)
  - stripKamacuEnv — the production env-strip in the custom/opencode spawn arm closing the HOOK_TOKEN secret-propagation hole under nested Kamacu (T-17-08)
  - TestCustomEngineDoesNotGetHookEnv made environment-independent (self-exports the trio; green in every shell)
  - Deadline headroom (2s→10s) on both documented PTY marker-wait flake sites — deferred items 2/3 closed
  - The D-62 frontend pair — honest Agent.engine union + claude-only quota-chip predicate on task pages
affects: [17-04 (its E2E/UAT assertions assume the green substrate), 17-VERIFICATION (baseline evidence rows)]

tech-stack:
  added: [] # stdlib only (strings.Cut) — zero new dependencies
  patterns:
    - "Strip-before-reinject env idiom: filter owned names out of the inherited os.Environ() base, then append fresh values last (append-last-value-wins) so the owner's values always win and non-recipients see nothing"
    - "Self-exporting env-context tests: a test that asserts child-env ABSENCE must os.Setenv the ambient names itself (t.Cleanup restores) — otherwise it only proves anything in shells that happen to carry them"

key-files:
  created: []
  modified:
    - internal/session/manager.go
    - internal/session/opencode_engine_test.go
    - internal/session/session_test.go
    - internal/api/sessions_test.go
    - web/src/api/types.ts
    - web/src/pages/TaskPage.tsx

key-decisions:
  - "D-63 fixed in PRODUCTION (stripKamacuEnv in the custom arm), not test-side scrubbing — research OQ3's locked recommendation; closes the secret-propagation hole, honors D014 in all contexts, keeps the append-last-value-wins idiom and the claude arm untouched"
  - "The baseline's surprise red (TestSpawnPumpFillsRing) was root-caused as the documented cross-package host-load flake class, not a Kamacu-env effect — it passes in isolation with AND without the trio; absorbed via deadline headroom (10s) on both recurred sites per the investigate-and-fix mandate and deferred items 2/3"
  - "eslint 'zero errors' acceptance read as 'no posture regression' — TaskPage.tsx carries one pre-existing react-hooks/set-state-in-effect error (Phase-12 documented lint debt, STATE.md-scoped out); before/after posture proven identical (same single error, same rule, same untouched effect), types.ts clean"
  - "BoardPage.tsx:21-22 carries the identical stale projectEngine !== \"custom\" predicate — recorded as the known consistency site (out of contract per the UI-SPEC Continuity Lock; future scope)"
  - "Deferred item 1 (production TMUX_TMPDIR seam in the tmux allow-list) NOT absorbed — 17-03's plan scoped D-63 to the KAMACU_* trio only; re-homed to a future quick task in deferred-items.md"

patterns-established:
  - "Pattern: environment-posture proof for env-asserting tests — run the gate twice, once with the ambient names exported and once scrubbed (env -u), before claiming green"

requirements-completed: [] # Phase 17 owns no REQ-IDs; this plan closes carried-forward debts D-62/D-63

coverage:
  - id: D1
    description: "D-63 production fix: the custom/opencode spawn arm strips inherited KAMACU_SESSION_ID/KAMACU_HOOK_TOKEN/KAMACU_HOOK_BASE before the opencode gate re-injects its own values — parent hook token never reaches custom-agent children; suite green both with and without the trio exported"
    verification:
      - kind: unit
        ref: "internal/session/opencode_engine_test.go#TestCustomEngineDoesNotGetHookEnv"
        status: pass
      - kind: unit
        ref: "internal/session/opencode_engine_test.go#TestOpencodeSpawnInjectsHookEnv"
        status: pass
      - kind: other
        ref: "go test ./... -count=1 -timeout 20m — exit 0 with trio exported AND scrubbed (env -u)"
        status: pass
    human_judgment: false
  - id: D2
    description: "Flake-class absorption: PTY marker-wait deadlines raised 2s→10s on both documented recurred sites (TestSpawnPumpFillsRing, TestInput_Happy_WritesAndAppendsCR) — deferred items 2/3 closed; only the failure path gets headroom"
    verification:
      - kind: other
        ref: "baseline run (red: TestSpawnPumpFillsRing under cross-package load) vs post-fix full-suite runs (green ×2) — /tmp evidence recorded in Verification Results"
        status: pass
    human_judgment: false
  - id: D3
    description: "D-62 frontend pair: Agent.engine union widened to claude|custom|opencode (Go wire + 00015 seed parity) and TaskPage isClaudeAgent flipped to strict === 'claude' — quota chip claude-only and hidden while loading; exactly the two locked edit lines"
    verification:
      - kind: other
        ref: "cd web && npm run build (tsc -b && vite build) — exit 0"
        status: pass
      - kind: other
        ref: "npx eslint posture proof: types.ts clean; TaskPage.tsx identical single pre-existing error before/after (no regression)"
        status: pass
      - kind: other
        ref: "grep: engine: \"claude\" | \"custom\" | \"opencode\" (types.ts:66); const isClaudeAgent = projectEngine === \"claude\" (TaskPage.tsx:69); zero diff under web/src/components, GlobalTaskPage.tsx, BoardPage.tsx, package.json"
        status: pass
    human_judgment: false

duration: 22 min
completed: 2026-08-28T11:12:18Z
status: complete
---

# Phase 17 Plan 03: D-63 Env Hygiene + D-62 Type Debt Summary

**One-liner:** Full-suite green restored in both environment postures by stripping inherited `KAMACU_*` (incl. the hook-token secret) from the custom-agent spawn env, absorbing the documented PTY marker flake via 10s deadlines, and making the frontend honest about the opencode engine (union widening + claude-only quota chip).

## D-63 Baseline Evidence (mandated by the plan's output spec)

**Baseline run** (`go test ./... -count=1 -timeout 20m`, pre-fix, executed from inside a live Kamacu session — all three `KAMACU_*` vars exported):

| Package | Result | Wall-time |
|---|---|---|
| kamacu/internal/api | ok | **239.277s** |
| kamacu/internal/session | **FAIL** | 18.051s |
| kamacu/internal/ws | ok | 63.835s |
| all other packages | ok | ≤7s each |

**Actual red set found (2):**

1. `TestCustomEngineDoesNotGetHookEnv` — the expected deterministic red: all three `KAMACU_SESSION_ID`/`KAMACU_HOOK_TOKEN`/`KAMACU_HOOK_BASE` present in the custom child env (D-63 root cause confirmed; research Pitfall 2 reproduced exactly). Green when the trio is scrubbed.
2. `TestSpawnPumpFillsRing` — the surprise red ("timed out after 2s waiting for: snapshot to contain kangent-marker"). **Investigated:** passes in isolation with AND without the trio; failed only under `go test ./...` cross-package parallel load (api 239s + ws 64s of PTY-heavy tests). Same class as the twice-documented `TestInput_Happy_WritesAndAppendsCR` flake (deferred items #2/#3). Absorbed via deadline headroom — see Deviations #1.

**Measured internal/api package runtime (the -timeout 20m calibration):** 239.277s baseline / 241.250s post-fix (239–249s across all runs this session, consistent with 17-02's 226–249s). Comfortably inside the 20m per-package budget; the flag stays mandatory on `./...` invocations per D-50's inline posture.

**Post-fix full-suite gates (both green, exit 0):**
- WITH trio exported (under-Kamacu): all 15 packages ok — api 241.250s, session 17.280s, ws 64.549s
- WITHOUT trio (`env -u KAMACU_SESSION_ID -u KAMACU_HOOK_TOKEN -u KAMACU_HOOK_BASE`): all packages ok

## Performance

- **Duration:** 22 min
- **Started:** 2026-08-28T10:50:12Z
- **Completed:** 2026-08-28T11:12:18Z
- **Tasks:** 2
- **Files modified:** 6 (4 declared + 2 flake-absorption test files)

## Accomplishments

- **D-63 production fix (`internal/session/manager.go`)**: new unexported `stripKamacuEnv` helper + `kamacuEnvNames` set filter `KAMACU_SESSION_ID`/`KAMACU_HOOK_TOKEN`/`KAMACU_HOOK_BASE` out of the inherited `os.Environ()` base in the custom/opencode spawn arm, BEFORE the opencode gate's `envExtra` re-injection (append-last-value-wins preserved — injected values still win when present). Closes T-17-08: a parent Kamacu terminal's HOOK_TOKEN (secret) no longer propagates into arbitrary custom-agent children. Claude arm byte-identical (diff hunks confined to imports/helper/custom branch).
- **D-63 test companion (`internal/session/opencode_engine_test.go`)**: `TestCustomEngineDoesNotGetHookEnv` now self-exports the trio (`os.Setenv` + `t.Cleanup` restore, with prior-value preservation) and documents the under-Kamacu context — the D014 absence assertion is proven in every environment, and the test fails if the strip is removed (regression coverage for the secret hole).
- **Flake absorption** (`internal/session/session_test.go`, `internal/api/sessions_test.go`): the two documented PTY marker-wait sites raised 2s→10s with in-code rationale; deferred items #2/#3 closed in `deferred-items.md`.
- **D-62 (`web/src/api/types.ts` + `web/src/pages/TaskPage.tsx`)**: engine union widened to `"claude" | "custom" | "opencode"` (provenance comment mirroring the `Task.source` precedent); `isClaudeAgent` flipped to strict `projectEngine === "claude"` (aligned with /global's locked rule). The two accepted user-visible deltas per 17-UI-SPEC: chip hidden for opencode-engine projects (the bug fix) and hidden while agents load (the cosmetic flip; no loading machinery added).

## Verification Results

| Check | Result |
|---|---|
| RED gate: `TestCustomEngineDoesNotGetHookEnv` in a clean shell (`env -u` trio), pre-fix | FAIL (as required — test now self-reproduces the under-Kamacu state) |
| GREEN gate: both env tests, trio exported AND scrubbed, post-fix | PASS ×4 |
| `go vet ./internal/session ./internal/api` | PASS |
| `KAMACU_SESSION_ID=t KAMACU_HOOK_TOKEN=t KAMACU_HOOK_BASE=t go test ./internal/session -run 'TestCustomEngineDoesNotGetHookEnv' -count=1` | PASS (exit 0) |
| `go test ./internal/session -run 'TestOpencodeSpawnInjectsHookEnv' -count=1` | PASS (D014 injection intact) |
| Full package `go test ./internal/session -count=1` (post-fix) | PASS (10.5–10.8s) |
| Full suite `go test ./... -count=1 -timeout 20m` WITH trio | PASS — exit 0, api 241.250s |
| Full suite `go test ./... -count=1 -timeout 20m` WITHOUT trio | PASS — exit 0 |
| `cd web && npm run build` (tsc -b && vite build) | PASS |
| `npx eslint src/api/types.ts` | PASS (0 errors) |
| `npx eslint src/pages/TaskPage.tsx` posture | IDENTICAL pre/post edit — 1 pre-existing `react-hooks/set-state-in-effect` error on the untouched keepExitedIds effect (Phase-12 documented lint debt); 0 new violations |
| Diff scope `4deb8ff..HEAD` | exactly 6 files (4 declared + 2 absorbed); zero lines under BoardPage.tsx / GlobalTaskPage.tsx / QuotaIndicator.tsx / web/src/components / web/package.json / web/dist |

## Task Commits

Each task was committed atomically (Task 1 followed TDD RED→GREEN):

1. **Task 1 RED — test self-exports the KAMACU_ trio** — `affedcf` (test)
2. **Task 1 GREEN — stripKamacuEnv production fix** — `de3e906` (feat)
3. **Task 1 absorption — marker-wait deadline headroom** — `8ed08ad` (test)
4. **Task 2 — D-62 union widening + predicate flip** — `1d584a8` (fix)

## Files Created/Modified

- `internal/session/manager.go` — stripKamacuEnv + kamacuEnvNames + the filtered `cmd.Env` construction in the custom arm (claude arm untouched)
- `internal/session/opencode_engine_test.go` — TestCustomEngineDoesNotGetHookEnv self-export + context comment
- `internal/session/session_test.go` — TestSpawnPumpFillsRing marker deadline 2s→10s + rationale
- `internal/api/sessions_test.go` — TestInput_Happy_WritesAndAppendsCR round-trip deadline 2s→10s + rationale
- `web/src/api/types.ts` — Agent.engine union widened with provenance comment
- `web/src/pages/TaskPage.tsx` — isClaudeAgent strict claude-only predicate + comment

## Decisions Made

See frontmatter `key-decisions` — the load-bearing ones: production-side (not test-side) D-63 fix; the surprise red root-caused as the documented flake class and absorbed via headroom; the eslint zero-errors criterion satisfied as no-posture-regression (pre-existing Phase-12 debt documented, not silently skipped).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Absorbed surprise red] PTY marker-wait deadline headroom (2s→10s)**
- **Found during:** Task 1 Step 1 (the baseline run)
- **Issue:** the baseline surfaced a second red test, `TestSpawnPumpFillsRing` — outside the plan's expected set. Investigation: passes in isolation (both env postures); fails only under `go test ./...` cross-package load. It is the third recurrence of the class deferred items #2/#3 explicitly parked to "plan 17-03's D-63 Wave-0 baseline (budget for deadline headroom or a load-tolerant marker poll)".
- **Fix:** raised the marker-wait deadline to 10s on both recurred sites (TestSpawnPumpFillsRing + TestInput_Happy_WritesAndAppendsCR) with in-code rationale. Polls return on condition — quiet-host runtime and suite duration are unchanged (session package 10.8s post-fix vs 18.1s failing baseline); only the failure path gets headroom.
- **Files modified:** internal/session/session_test.go, internal/api/sessions_test.go
- **Verification:** both full-suite runs green; the two sites pass in isolation
- **Commit:** 8ed08ad

**2. [Rule 1 - Plan-text vs verification reality] eslint "zero errors" on TaskPage.tsx**
- **Found during:** Task 2 verification gate
- **Issue:** the acceptance criterion reads "npx eslint … exits with zero errors (no posture regression on the edited files)", but TaskPage.tsx carries one pre-existing `react-hooks/set-state-in-effect` error (line 141, the keepExitedIds effect) — the Phase-12 documented lint debt, explicitly scoped OUT of phases in STATE.md and parked for a dedicated cleanup task. Fixing it here would violate the UI-SPEC Continuity Lock (edits beyond the two locked lines) and the executor scope boundary.
- **Fix:** satisfied the criterion's operative intent — proved NO posture regression: identical lint output before/after the edit (same single error, same rule, same untouched effect; verified by running eslint against the pre-edit file restored in place). types.ts lints clean.
- **Files modified:** none (verification-only resolution)
- **Verification:** before/after eslint counts identical (2 error-pattern lines each = the same 1 error)
- **Commit:** n/a

**3. [Rule 1 - Execution order] TDD RED committed before the production fix**
- **Found during:** Task 1
- **Issue:** the plan's `<action>` lists the production fix as Step 2 and the test edit as Step 3, but the task is `tdd="true"` — the RED gate requires the failing test to exist (and be observed failing) before the fix lands.
- **Fix:** executed Step 3's test edit as the RED phase first (commit affedcf), then Step 2's production fix as GREEN (de3e906). Content identical to the plan's specification; only the commit order differs.
- **Files modified:** n/a (ordering only)
- **Verification:** RED observed failing in a clean shell; GREEN green in both postures
- **Commits:** affedcf, de3e906

---

**Total deviations:** 3 auto-fixed (1 absorbed surprise red, 1 criterion-intent resolution, 1 TDD ordering). **Impact:** all three keep the plan's own contracts intact (four declared files + the mandate-sanctioned absorption; Continuity Lock honored — the frontend diff is exactly the two locked lines plus comments). No scope creep.

## Issues Encountered

None beyond the absorbed baseline surprise (documented above and in deferred-items.md).

## Known Stubs

None — no stubs, placeholders, or unwired data paths.

## Known Consistency Site (future scope, per the plan's own instruction)

**`web/src/pages/BoardPage.tsx:21-22`** carries the identical stale predicate (`const projectEngine = …?.engine; const isClaudeAgent = projectEngine !== "custom";`) — out of contract this phase per the UI-SPEC Continuity Lock (allowed frontend diffs = exactly types.ts + TaskPage.tsx). On opencode-engine projects the board-level quota chip will still show until a future task applies the same strict-equality fix there. Recorded for future scoping.

## Authentication Gates

None.

## Next Phase Readiness

- Ready for 17-04 (the opencode real-resume E2E leg + UAT) — its assertions now sit on the suite-green substrate this plan established.
- Deferred item #1 (production TMUX_TMPDIR seam in manager.go's tmux allow-list) remains open and re-homed to a future quick task — 17-03's plan scoped D-63 to the KAMACU_* trio only; see deferred-items.md.
- Threat register: T-17-08 mitigated (strip + environment-independent regression test); T-17-09 mitigated (union + strict predicate).

## Self-Check: PASSED

- [x] All 6 modified files exist on disk
- [x] All 4 task commits present on task/add-global-session-301 (affedcf / de3e906 / 8ed08ad / 1d584a8)
- [x] TDD gates: test commit (RED) precedes feat commit (GREEN) in history
- [x] Full suite green in both env postures; all task acceptance criteria executed
