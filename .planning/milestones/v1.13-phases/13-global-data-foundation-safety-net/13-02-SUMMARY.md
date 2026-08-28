---
phase: 13-global-data-foundation-safety-net
plan: 2
subsystem: infra
tags: [sqlite, backfill, boot-safety, tmux, orphan-sweep, fk-guard, go]

requires:
  - phase: 13-global-data-foundation-safety-net
    provides: migrations 00017 (global_task singleton + agent FK RESTRICT) and 00018 (tmux_sessions scope rebuild) that this plan's runtime halves wrap
provides:
  - BackfillGlobalTask — idempotent boot re-arm of the global_task singleton, seeded from the is_default agent (GDATA-01 runtime half)
  - Scope-aware startup tmux orphan sweep — a live global tab can never be killed at boot (GDATA-03, the co-phasing mandate closed)
  - Agents delete-guard extension — 409 with the locked D-09 string ahead of the 00017 RESTRICT backstop
  - Real-install upgrade rehearsal proof — v1.12 copy upgrades to goose v18 byte-for-byte with the singleton seeded from the real default agent (GDATA-02 real-data half)
affects: [14-global-config-api, 15-sessions-backend-scope-spawn, 17-hardening-e2e]

tech-stack:
  added: []  # zero new dependencies (locked decision; go.mod/web/package.json untouched)
  patterns:
    - "Host-gated real-tmux regression in package main: locally re-created per-test-socket helpers (the analogs are unexported test-file funcs in internal/tmux), KillServer cleanup registered before any session exists"
    - "Failing-test-first for a one-query production fix: the RED commit proves the killer bug empirically (live global tab dies under the old known-set) before the fix lands"

key-files:
  created:
    - internal/api/global_backfill.go
    - internal/api/global_backfill_test.go
    - cmd/kamacu/sweep_test.go
  modified:
    - cmd/kamacu/serve.go
    - internal/api/agents_crud.go
    - internal/api/agents_crud_test.go
    - internal/api/agents_backfill_test.go

key-decisions:
  - "Sweep fix is the research-verified subquery form — SELECT name FROM tmux_sessions WHERE scope='global' OR task_id IN (SELECT id FROM tasks) — not the LEFT-JOIN equivalent (planner resolution)"
  - "The 409 message is count-free ('reassign the Scratchpad agent first') because the singleton references exactly one agent, always — D-09, UI-SPEC Copywriting row 1"
  - "TestBackfillAgents' wipe simulation drops the singleton before the default agent — the honest hand-wiped-install shape under the 00017 FK"

patterns-established:
  - "Scope-aware known-set semantics: known = row whose task STILL exists OR any global-scoped row (task-less by design); globals are known WITHOUT a task JOIN"
  - "Package-private function testing: when the unit under test is package-private in package main, the test file (and its helpers) live beside it — helpers are re-created locally rather than exporting for testability"

requirements-completed: [GDATA-01, GDATA-02, GDATA-03]

coverage:
  - id: D1
    description: "BackfillGlobalTask + serve.go boot wiring — idempotent singleton re-arm seeded from the is_default agent; error refuses boot (GDATA-01 boot half, SC2)"
    requirement: GDATA-01
    verification:
      - kind: unit
        ref: internal/api/global_backfill_test.go#TestBackfillGlobalTask
        status: pass
      - kind: other
        ref: "grep: exactly one api.BackfillGlobalTask call in cmd/kamacu/serve.go, positioned after BackfillOpenCodeAgent with slog.Error + ExitFailure"
        status: pass
    human_judgment: false
  - id: D2
    description: "Agents delete-guard COUNT — 409 with the exact D-09 string while global_task references the agent, 204 after reassignment (GDATA-01 FK half, SC2)"
    requirement: GDATA-01
    verification:
      - kind: unit
        ref: internal/api/agents_crud_test.go#TestAgentDeleteInUseByGlobal
        status: pass
    human_judgment: false
  - id: D3
    description: "Scope-aware startup orphan sweep — a live kamacu-global-1 tmux tab survives the boot sweep, task-backed tabs survive, true task-orphans still die (GDATA-03, SC4; co-phasing mandate closed)"
    requirement: GDATA-03
    verification:
      - kind: integration
        ref: cmd/kamacu/sweep_test.go#TestSweepOrphanTmuxScopeAware
        status: pass
    human_judgment: false
  - id: D4
    description: "Real-install rehearsal — .backup copy of the live v1.12 DB (goose v16, 147 tasks, 10 labeled tmux rows, 4 agents) booted once with the real binary upgrades to goose v18 byte-for-byte, singleton seeded from the install's default agent (GDATA-02 real-data half, SC1)"
    requirement: GDATA-02
    verification:
      - kind: manual_procedural
        ref: "rehearsal: diff before/after EMPTY; goose MAX(version_id)=18; COUNT(global_task)=1; agent_id == is_default agent; tasks 147->147; boot log 00017 OK + 00018 OK, zero sweeps, zero errors"
        status: pass
    human_judgment: false

duration: 24 min
completed: 2026-08-25
status: complete
---

# Phase 13 Plan 2: Global runtime safety net Summary

**BackfillGlobalTask (idempotent boot re-arm of the singleton), the scope-aware one-query orphan-sweep fix (RED-proven to kill a live global tab before the fix, GREEN after), the D-09 Scratchpad delete-guard 409, and a byte-for-byte real-install rehearsal of the real binary against a .backup copy of the live v1.12 database**

## Performance

- **Duration:** 24 min
- **Started:** 2026-08-25T13:28:26Z
- **Completed:** 2026-08-25T13:52:23Z
- **Tasks:** 3 (2 TDD: RED→GREEN each)
- **Files modified:** 10 (7 source/test + 3 planning docs incl. this summary)

## Accomplishments
- `BackfillGlobalTask` mirrors `BackfillAgents` exactly (QueryRow no-op fast path → `errors.Is(ErrNoRows)` discrimination → literal INSERT..SELECT seeded from `WHERE is_default = 1`), wired in serve.go AFTER BackfillOpenCodeAgent — the load-bearing ordering (Pitfall 6) — with an error refusing boot
- The startup orphan sweep's known-set is now scope-aware (`WHERE scope = 'global' OR task_id IN (SELECT id FROM tasks)`); the regression test first PROVED the killer bug on this host (the live kamacu-global-1 session was killed under the old INNER JOIN — RED commit `8ad3b23`) and then proved the fix (global ALIVE, task-backed ALIVE, true orphan DEAD)
- `DELETE /api/agents/{id}` for the agent referenced by global_task returns 409 with the phase's one user-facing string `reassign the Scratchpad agent first` (count-free, D-09), 204 after reassignment — ahead of the 00017 RESTRICT backstop
- Real-install rehearsal (Task 3, all gates PASS): `sqlite3 .backup` copy (WAL-safe) → boot the real binary once on 127.0.0.1:7391 → goose 16→18 (00017 OK, 00018 OK), tmux survival diff EMPTY (10 custom-labeled rows byte-identical), `global_task` = exactly 1 row with agent_id=1 (= the install's is_default Claude agent), tasks 147→147, zero sweeps, zero errors, and the singleton CHECK constraint bites on a fresh connection

## Task Commits

Each task was committed atomically (TDD: RED test commit → GREEN implementation commit):

1. **Task 1: BackfillGlobalTask + boot wiring + Scratchpad delete-guard** — RED `ada0660` (test), GREEN `f2e3519` (feat)
2. **Task 2: Scope-aware orphan sweep** — RED `8ad3b23` (test, failing regression), GREEN `7debf89` (fix)
3. **Task 3: Real-install rehearsal** — verification-only, no code artifacts (proof recorded here)

**Plan metadata:** (see final docs commit below)

## Files Created/Modified
- `internal/api/global_backfill.go` — BackfillGlobalTask (leaf posture: database/sql + errors only)
- `internal/api/global_backfill_test.go` — TestBackfillGlobalTask (healthy-boot no-op / hand-DELETE re-arm / second-call idempotence)
- `cmd/kamacu/serve.go` — backfill wiring block after BackfillOpenCodeAgent; sweepOrphanTmux known-set query replaced with the scope-aware form (diff = exactly the query + its comment)
- `cmd/kamacu/sweep_test.go` — TestSweepOrphanTmuxScopeAware + locally re-created helpers (host-gate, per-test socket, detached sessions, poll-with-deadline liveness)
- `internal/api/agents_crud.go` — global_task COUNT guard inside the delete handler (+ doc comment now lists three guards)
- `internal/api/agents_crud_test.go` — TestAgentDeleteInUseByGlobal (409 with exact D-09 string; 204 after reassignment)
- `internal/api/agents_backfill_test.go` — wipe simulation drops the singleton first (deviation fix, see below)

## Decisions Made
- Followed the planner resolutions verbatim: subquery-form sweep fix (not LEFT JOIN), count-free 409 string, wiring point after BackfillOpenCodeAgent — all lifted from the research's verified forms, no redesign
- Rehearsal pragma nuance: a bare `PRAGMA foreign_keys` readout on a fresh sqlite3 CLI connection is 0 by SQLite per-connection design (the DSN arms the app's own pooled connections; the in-repo migration test asserts the pooled handle). The rehearsal therefore proves FK/CHECK durability via the enforcement probe — `INSERT INTO global_task (id, agent_id) VALUES (2, 999)` on a fresh FK-on connection fails `CHECK constraint failed: id = 1` — recorded in output-equality form exactly as the plan's step 6 mandates

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] TestBackfillAgents wipe simulation broke under the 00017 FK**
- **Found during:** Task 1 (full-package verification run)
- **Issue:** The pre-existing test's "missing default" phase hand-deletes the default agent; migration 00017's `global_task.agent_id ON DELETE RESTRICT` now (correctly) refuses that DELETE — `constraint failed: FOREIGN KEY constraint failed (1811)`. Verified pre-existing at 13-01's final commit `1d10016` (throwaway worktree), i.e. a 13-01 schema interaction this plan's verification gate ("full suite green") had to close
- **Fix:** The simulation now runs `DELETE FROM global_task` first — the honest hand-wiped-agents install shape under the phase's schema; BackfillGlobalTask re-arms the singleton next boot (proven separately by TestBackfillGlobalTask)
- **Files modified:** internal/api/agents_backfill_test.go
- **Verification:** `go test ./internal/api/ -run 'TestBackfillAgents|TestBackfillGlobalTask|TestAgentDeleteInUseByGlobal|TestBackfillOpenCodeAgent'` — all PASS
- **Committed in:** `1034a78`

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** The fix is the phase's own schema surface (the 00017 FK behaving as designed); no scope creep — one test's seeding updated to the new invariant.

## Issues Encountered
- Four pre-existing test failures in the repo, ALL verified failing identically at `1d10016` (before any 13-02 change, throwaway-worktree checks): `TestInput_Happy_WritesAndAppendsCR`, `TestInput_TrailingLF_TranslatedToCR`, `TestInput_EmptyMessage_WritesBareCR` (internal/api, PTY input surface) and `TestCustomEngineDoesNotGetHookEnv` (internal/session, opencode hook-env gating). None touch this plan's diff (session engine + input surface are explicitly out of scope per Pitfall 7). Logged to `deferred-items.md`; the phase's no-regression gate holds — every other package is green, including `cmd/kamacu`, `internal/store`, `internal/reaper`, `internal/tmux`

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Phase 13 is COMPLETE: all three GDATA requirements closed (schema halves in 13-01, runtime + real-data halves here), the co-phasing mandate satisfied (migration + sweep fix in the same phase, RED-proven)
- Phase 14 (global config API) can build on: the singleton's storage contract (root_path/github_repo/resume-id columns, seeded and boot-re-armed), the D-09 "Scratchpad" naming locked into the one shipped string, and the delete-guard precedent for its own gates
- Pre-existing red tests in internal/api (TestInput_*) and internal/session (TestCustomEngineDoesNotGetHookEnv) need a dedicated investigation task before Phase 15's risk-center work — see deferred-items.md

---
*Phase: 13-global-data-foundation-safety-net*
*Completed: 2026-08-25*

## Self-Check: PASSED

All key-files exist on disk; all 5 task commits verified in git log (ada0660, f2e3519, 1034a78, 8ad3b23, 7debf89); both targeted suites re-run green after SUMMARY creation (`go test ./internal/api/ -run 'TestBackfillGlobalTask|TestAgentDeleteInUseByGlobal'` ok, `go test ./cmd/kamacu/ -run TestSweepOrphanTmuxScopeAware` ok); `roadmap update-plan-progress 13` reports plan_count=2, summary_count=2, status=Complete.
