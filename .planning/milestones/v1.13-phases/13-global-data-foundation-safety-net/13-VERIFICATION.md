---
phase: 13-global-data-foundation-safety-net
verified: 2026-08-25T16:32:00Z
status: passed
score: 12/12 must-haves verified
behavior_unverified: 0
overrides_applied: 0
human_verification:

  - test: "Adjudicate CR-01 (13-REVIEW.md): agents delete handler can delete the current default agent → zero defaults → next boot fails permanently on idx_agents_name_nocase (pre-existing M001 defect chain, adjacent to this phase's guard)"
    expected: "Developer decision: fix now as a phase-13 gap-closure (add is_default guard → 409 'set another default agent first' + regression test), or explicitly schedule it before ship (no later phase in the roadmap covers it — Phase 17's SCs are global-flow E2E, not agents CRUD)"
    why_human: "Verifier judgment: CR-01 does NOT fail any phase-13 must-have (the referenced-agent delete IS refused — proven at handler and DB level) and is verifiably pre-existing, so it is not a phase gap. But the review labels it must-fix-before-ship with an unbootable-app outcome; accept/defer/fix-now is a scope decision only the developer can make"

  - test: "Adjudicate WR-01 (13-REVIEW.md): BackfillGlobalTask's INSERT..SELECT silently inserts zero rows (returns nil) when no is_default agent exists"
    expected: "Developer decision: add the RowsAffected()==1 fail-loud guard + zero-default test case (review's fix), or accept as-is. Note: through the real boot pipeline the state is unreachable (BackfillAgents runs first and re-seeds a default — verified wiring at serve.go:171→186→199); the gap only bites direct/future callers"
    why_human: "The specified must-have truth (idempotent no-op + re-insert after hand DELETE) is fully verified; the zero-default hardening is defense-in-depth beyond the phase contract — a scope decision"

  - test: "Adjudicate WR-02 (13-REVIEW.md, pre-existing M002): agents update engine allowlist rejects 'opencode' same-value round-trips, 400ing the whole PATCH"
    expected: "Developer decision: schedule the allowlist fix (accept current engine value) in a later phase or a dedicated task; no current UI path trips it (React dialog omits engine on PATCH)"
    why_human: "Entirely outside phase-13's diff and must-haves (update handler untouched); surfaced so it is not lost"

  - test: "Schedule investigation of the 4 pre-existing red tests (TestInput_Happy_WritesAndAppendsCR, TestInput_TrailingLF_TranslatedToCR, TestInput_EmptyMessage_WritesBareCR in internal/api; TestCustomEngineDoesNotGetHookEnv in internal/session)"
    expected: "A dedicated investigation task lands before Phase 15 (the milestone's risk center touches session paths); they are logged in deferred-items.md but no roadmap phase owns them"
    why_human: "Independently verified pre-existing at base cc9bb03 (throwaway worktree, per orchestrator context) and confirmed by this verifier's full-suite run (exact same 4 failures, every other package ok). Not phase regressions; scheduling is a developer call"
---

# Phase 13: Global data foundation & safety net — Verification Report

**Phase Goal:** The database and startup invariants that let task-free global sessions exist — a DB-enforced `global_task` singleton holding config + resume ids, a `tmux_sessions` shape that accepts task-less rows, and an orphan sweep that can never kill them — landed so that upgrading an existing install preserves everything byte-for-byte.
**Verified:** 2026-08-25T16:32:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

All 12 merged must-have truths (4 ROADMAP Success Criteria + 12 PLAN truths, deduplicated) are VERIFIED with behavioral evidence the verifier executed itself — every named test re-run green, plus an independent re-run of the real-install rehearsal against a fresh `.backup` copy of the live v1.12 database. `human_needed` is NOT about the phase goal: all automated gates pass. It escalates the code-review findings (CR-01/WR-01/WR-02) and the pre-existing red tests for developer adjudication — none fail a phase must-have, but none may be silently absorbed into a pass either.

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Staged v16 (v1.12 shape) DB with seeded custom labels + exact created_at literals upgrades through real Migrate() with every tmux row byte-for-byte identical (SC1, GDATA-02) | ✓ VERIFIED | `TestTmuxScopeRebuildMigration` PASS (verifier-run): exact-string `id\|task_id\|n\|name\|label\|created_at` snapshots compared row-for-row before/after; seeds 2 custom labels + 1 DEFAULT label + exact timestamps. Reinforced on real data by truth 12 |
| 2 | After upgrade global_task holds exactly one row: id=1, agent_id from `is_default=1` (never hardcoded), root_path='', github_repo/resume ids NULL (GDATA-01) | ✓ VERIFIED | `TestGlobalTaskMigration` PASS: asserts via flag query, never a literal id |
| 3 | INSERT id=2 rejected by CHECK(id=1); DELETE of referenced agent rejected by ON DELETE RESTRICT (GDATA-01) | ✓ VERIFIED | `TestGlobalTaskMigration` asserts both rejections; verifier's fresh-connection rehearsal probe: `CHECK constraint failed: id = 1` |
| 4 | tmux_sessions accepts task-less global row, rejects ambiguous AND neither; copied rows read scope='task' (SC3, GDATA-02, D-11 `global` naming) | ✓ VERIFIED | `TestTmuxScopeRebuildMigration` asserts both acceptance + both rejection directions (n=7 isolates the XOR from UNIQUE(task_id,n)); rehearsal copy: all 10 real rows scope='task' |
| 5 | PRAGMA foreign_keys == 1 on the migrated pooled handle | ✓ VERIFIED | Pragma probe in test + bogus task_id=999 insert fails on the same handle |
| 6 | Second Migrate(db) is a clean no-op | ✓ VERIFIED | Both tests assert re-run idempotence (singleton count, row count unchanged) |
| 7 | BackfillGlobalTask idempotent: no-op on healthy boot, re-inserts id=1 from is_default after hand DELETE (SC2 boot half, GDATA-01) | ✓ VERIFIED | `TestBackfillGlobalTask` PASS: all three phases asserted, seed-shape via SQL comparison |
| 8 | serve.go wires BackfillGlobalTask AFTER BackfillAgents/BackfillOpenCodeAgent; error refuses boot with ExitFailure | ✓ VERIFIED | serve.go:171 → :186 → :199 ordering (grep); `slog.Error` + `return subcommands.ExitFailure` present; `go build ./...` OK |
| 9 | DELETE /api/agents/{id} for referenced agent → 409 body `reassign the Scratchpad agent first`; after reassignment → 204 (SC2 FK half, GDATA-01, D-09) | ✓ VERIFIED | `TestAgentDeleteInUseByGlobal` PASS: exact-string 409 + 204-after-reassign; handler COUNT guard sits before DELETE exec, ahead of the 00017 RESTRICT backstop |
| 10 | Sweep never kills a live global tab; task-backed survives; true orphan dies (SC4, GDATA-03) | ✓ VERIFIED | `TestSweepOrphanTmuxScopeAware` PASS on this host (real tmux 3.4, per-test socket): global ALIVE, task-backed ALIVE, orphan DEAD (`INFO swept orphan tmux session name=kamacu-42-1` observed in run output). RED-first discipline confirmed: commit 8ad3b23 landed the test while serve.go still had the INNER JOIN; fix in 7debf89 |
| 11 | Known-set query is exactly the scope-aware form; everything else in the sweep unchanged | ✓ VERIFIED | serve.go:384 = `SELECT name FROM tmux_sessions WHERE scope = 'global' OR task_id IN (SELECT id FROM tasks)`; `git diff cc9bb03..HEAD -- serve.go` shows ONLY the backfill block + this query + its adjacent comment — 30s timeout, warn-only returns, kamacu- prefix filter untouched |
| 12 | Real v1.12 install copy booted once with the real binary upgrades to v18 byte-for-byte (SC1 rehearsal) | ✓ VERIFIED | **Verifier-executed rehearsal** (not SUMMARY trust): `.backup` of live install (goose 16, 147 tasks, 10 labeled rows, 4 agents) → built real binary → booted on 127.0.0.1:7395 → log shows `OK 00017`, `OK 00018`, `migrated to version: 18`, `listening`; before/after tmux diff EMPTY; tasks 147→147; global_task=1, agent_id=1 == is_default agent; 0 sweeps; temp artifacts cleaned |

**Score:** 12/12 truths verified (0 present-but-behavior-unverified — every behavior-dependent truth has a passing behavioral test the verifier ran)

### ROADMAP Success Criteria Rollup

| SC | Status | Covered by truths |
|----|--------|-------------------|
| SC1 — seeded v1.12 install upgrades byte-for-byte, rehearsed on a real-install copy | ✓ VERIFIED | 1, 12 |
| SC2 — singleton exists every boot, re-armed by backfill, referenced-agent delete refused | ✓ VERIFIED | 2, 3, 7, 8, 9 |
| SC3 — task-less rows accepted, ambiguous rejected, FK discipline unchanged | ✓ VERIFIED | 4, 5 |
| SC4 — live global tab survives sweep, task-orphan killing unchanged | ✓ VERIFIED | 10, 11 |

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/store/migrations/00017_global_task.sql` | singleton table + seed | ✓ VERIFIED | Exact spec shape: `CHECK (id = 1)`, `REFERENCES agents(id) ON DELETE RESTRICT`, seed `SELECT 1, id FROM agents WHERE is_default = 1`, plain `-- +goose Up`, Down present; storage contract comments (D-02/D-09/D-11) |
| `internal/store/migrations/00018_tmux_scope.sql` | scope rebuild | ✓ VERIFIED | `-- +goose NO TRANSACTION`, FK-off before BEGIN / FK-on after COMMIT, sqlite.org §8 ordering (create-new→copy→drop-old→rename-new), exact XOR spelling `(task_id IS NULL) = (scope = 'global')`, column-list-pinned copy, Down mirrors reverse discipline |
| `internal/store/global_task_migration_test.go` | staged-upgrade proofs | ✓ VERIFIED | 331 substantive lines: stageV116 helper (UpTo 16 + sqlite_master pre-checks), both test functions with all (a)-(h) assertions |
| `internal/api/global_backfill.go` | BackfillGlobalTask | ✓ VERIFIED | QueryRow fast path → `errors.Is(ErrNoRows)` discrimination → INSERT..SELECT from is_default; leaf posture (database/sql + errors only) |
| `internal/api/global_backfill_test.go` | backfill proof | ✓ VERIFIED | 3-phase idempotence test, seed-shape asserted via SQL comparison |
| `internal/api/agents_crud.go` (modified) | delete-guard COUNT | ✓ VERIFIED | COUNT guard + exact D-09 string before DELETE exec |
| `internal/api/agents_crud_test.go` (modified) | 409/204 proof | ✓ VERIFIED | TestAgentDeleteInUseByGlobal: exact-string 409 + 204 after reassign |
| `internal/api/agents_backfill_test.go` (modified) | wipe-sim repair | ✓ VERIFIED | Drops singleton before default agent under the 00017 FK (commit 1034a78); `TestBackfillAgents` PASS |
| `cmd/kamacu/serve.go` (modified) | wiring + sweep fix | ✓ VERIFIED | Diff scope exactly: backfill block + sweep query + comment |
| `cmd/kamacu/sweep_test.go` | sweep regression | ✓ VERIFIED | Host-gated per-test socket, KillServer cleanup before sessions, detached-session starter, poll-with-deadline liveness, 3-way assertion |

Migration-dir guard: `git diff cc9bb03..HEAD --name-status -- internal/store/migrations/` = exactly 2 `A` entries, zero existing migrations modified.

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| staged test | real migrations | goose.UpTo(16) → Migrate() on migrationsFS | ✓ WIRED | Test log shows the real runner applying 00001..00018 then "no migrations to run" |
| 00018 | pooled connection | trailing `PRAGMA foreign_keys = ON` after COMMIT | ✓ WIRED | Pragma probe reads 1 on the migrated handle; bogus-FK insert bites |
| serve.go boot | BackfillGlobalTask | call after BackfillOpenCodeAgent (:199) | ✓ WIRED | Ordering + ExitFailure error path verified; rehearsal boot exercised the real pipeline end-to-end |
| sweepOrphanTmux | scope column | known-set query (serve.go:384) | ✓ WIRED | Query only valid post-00018; live test proves behavior |
| agents delete handler | global_task.agent_id | COUNT guard ahead of FK RESTRICT | ✓ WIRED | 409-then-204 proven through the real HTTP handler (newTestServer) |

Note: `gsd-tools query verify.artifacts / verify.key-links` fell back to LLM-derived mode on both plans (shorthand string-list frontmatter instead of object form) — all artifact/key-link verification above was performed manually at existence/substance/wiring depth.

### Data-Flow Trace (Level 4)

Not applicable in the render sense — no UI artifacts this phase. Data-flow equivalents verified instead: the migrations move real seeded rows (verified byte-for-byte), the backfill writes real rows on the hand-DELETE path (verified), the sweep query reads real tmux_sessions rows against a live tmux server (verified 3-way), and the guard reads global_task against real HTTP requests (verified 409/204). No static/hollow paths found.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Singleton + rebuild migrations | `go test ./internal/store/ -run 'TestGlobalTaskMigration\|TestTmuxScopeRebuildMigration' -v -count=1` | both PASS | ✓ PASS |
| Backfill + delete guard + wipe-sim repair | `go test ./internal/api/ -run 'TestBackfillGlobalTask\|TestAgentDeleteInUseByGlobal\|TestBackfillAgents' -v -count=1` | all 3 PASS | ✓ PASS |
| Scope-aware sweep (live tmux 3.4) | `go test ./cmd/kamacu/ -run TestSweepOrphanTmuxScopeAware -v` | PASS (orphan kill logged, global spared) | ✓ PASS |
| Whole-repo build | `go build ./...` | exit 0 | ✓ PASS |
| Full suite (single run, saved) | `go test ./... -count=1` | exactly the 4 known pre-existing failures (TestInput_*, TestCustomEngineDoesNotGetHookEnv); every other package ok | ✓ PASS (no regressions) |

### Probe Execution

| Probe | Command | Result | Status |
|-------|---------|--------|--------|
| Real-install rehearsal (re-executed by verifier, not trusted from SUMMARY) | sqlite3 `.backup` → build real binary → boot on 127.0.0.1:7395 against copy → kill → diff + assertions | before/after diff EMPTY; goose 16→18 (00017 OK, 00018 OK); global_task=1 seeded agent_id=1=is_default; tasks 147→147; 10 rows all scope='task'; 0 sweeps; CHECK bites on fresh conn (`CHECK constraint failed: id = 1`) | PASS |

Safety posture: copy-only I/O (online-backup API on the live DB), spare port (real server untouched on 7333), no tmux server live on the kamacu socket during the probe, all /tmp/opencode artifacts removed afterward.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| GDATA-01 | 13-01, 13-02 | DB-enforced singleton, seeded at migration, guaranteed by idempotent boot backfill, agent FK RESTRICT + delete-guard | ✓ SATISFIED | Truths 2, 3, 7, 8, 9 |
| GDATA-02 | 13-01, 13-02 | tmux_sessions rebuilt for task-less rows with XOR CHECK; existing rows survive byte-for-byte on the goose upgrade path (real install rehearsed) | ✓ SATISFIED | Truths 1, 4, 5, 6, 12 |
| GDATA-03 | 13-02 | Scope-aware startup sweep — global tabs never killed (live regression test) | ✓ SATISFIED | Truths 10, 11 |

Orphaned requirements: none — REQUIREMENTS.md maps exactly GDATA-01/02/03 to Phase 13; the union of both plans' `requirements` covers all three.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (phase files) | — | TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER scan on all 10 files | — | CLEAN — zero matches |
| internal/api/agents_crud.go, agents_crud_test.go, cmd/kamacu/serve.go | various | gofmt drift (13-REVIEW IN-01; verified pre-existing at base) | ℹ️ Info | Whitespace-only; fix before any format gate lands |
| internal/store/migrations/00018_tmux_scope.sql | :33 | UNIQUE(task_id,n) doesn't constrain global rows (NULLs distinct) (13-REVIEW IN-02) | ℹ️ Info | By design; Phase 15 must serialize global `n` app-side (roadmap Phase 15 SC1 names the `kamacu-global-<n>` mint) — obligation recorded |

### Review Findings Judged Against Phase Must-Haves

| Finding | Severity claimed | Verifier judgment | Disposition |
|---------|-----------------|-------------------|-------------|
| CR-01 delete-default → unbootable-app chain | critical | **Pre-existing (M001)** — the delete path predates the phase; verified this phase's own guard truths all hold (truth 9). Does not fail any phase-13 must-have | Escalated — human decision (fix now vs schedule; no later phase covers it) |
| WR-01 BackfillGlobalTask silent zero-row insert | warning | In-phase robustness gap beyond the specified truth; unreachable through the real boot pipeline (BackfillAgents ordering guarantees a default exists first — verified at :171→:186→:199) | Escalated — human decision (fail-loud hardening) |
| WR-02 engine allowlist rejects 'opencode' round-trip | warning | Pre-existing M002, update handler untouched by phase | Escalated — schedule |
| 4 pre-existing red tests | — | Confirmed pre-existing at base cc9bb03 (independent throwaway-worktree verification) and by this verifier's full-suite run; phase surfaces untouched (Pitfall 7) | Escalated — schedule before Phase 15 |

### Human Verification Required

Developer adjudication items (Escalation Gate) — see frontmatter `human_verification` for the four decision items: (1) CR-01 fix-vs-schedule, (2) WR-01 fail-loud guard, (3) WR-02 allowlist fix, (4) pre-existing red-test investigation task. None block the phase goal; all must be decided so nothing critical is silently absorbed.

### Gaps Summary

No gaps. All 12 must-have truths, all 4 ROADMAP success criteria, and all 3 GDATA requirements are verified with verifier-executed behavioral evidence — including an independent re-run of the real-install rehearsal (byte-for-byte empty diff, goose 16→18, singleton seeded from the real default agent). The TDD RED-first discipline was confirmed in git history (8ad3b23 test before 7debf89 fix, old INNER JOIN still in place at RED). The status is `human_needed` solely to force decisions on the review findings and pre-existing red tests.

---

_Verified: 2026-08-25T16:32:00Z_
_Verifier: the agent (gsd-verifier)_
