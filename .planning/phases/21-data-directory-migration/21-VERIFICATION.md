---
phase: 21-data-directory-migration
verified: 2026-07-01T19:30:00Z
status: passed
score: 5/5 must-haves verified
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 1/5
  gaps_closed:
    - "First boot migrates the dir to ~/.kamacu and the app serves against it (MIGRATE-01) — repairWorktrees no longer aborts on stale worktrees"
    - "After migration a migrated worktree is a clean git status; DB paths rewritten (MIGRATE-02)"
    - "Second boot is a clean no-op (MIGRATE-05) — Complete now survives stale stragglers"
    - "Copied-HOME UAT recipe is now git-safe; real install proven byte-identical (Gap 2)"
    - "Browser kamacu.* localStorage keys populated from kangent.* (MIGRATE-04) — human-confirmed via 21-07"
    - "Board loads, task view opens, dead agent resumes, diff renders (MIGRATE-01/02/03) — human-confirmed via 21-07"
  gaps_remaining: []
  regressions: []
---

# Phase 21: Data Directory Migration Verification Report

**Phase Goal:** An existing `~/.kangent` install upgrades cleanly to `~/.kamacu` — live worktrees, sessions, the SQLite DB, and browser state all survive — while a fresh or already-migrated install skips safely and any failure is recoverable.
**Verified:** 2026-07-01T19:30:00Z
**Status:** passed
**Re-verification:** Yes — after gap closure (21-06 code fix + 21-07 git-safe live re-run). The first live gate (21-05) failed with 2 blocking gaps; both are now closed and independently confirmed against HEAD.

## Goal Achievement

### Observable Truths

| # | Truth (must_have) | Status | Evidence |
|---|-------------------|--------|----------|
| 1 | First launch migrates the data dir `~/.kangent` → `~/.kamacu` and the app runs against the new location (MIGRATE-01) | ✓ VERIFIED | `migrate.Prepare` (migrate.go:142) gates + preflights + `os.Rename` commit point (line 180) + WAL-safe DB rename (`PRAGMA wal_checkpoint(TRUNCATE)`, line 252) + tmux retire, wired in `main.go:75` BEFORE `store.Open` (line 91). Gap-1 fix lets `Complete` finish so the app serves. 21-07 live run against a copy of the real 5.5 GB install: boot logged `migrated ~/.kangent -> ~/.kamacu` once, `/api/healthz` → `{"status":"ok"}`; `.kamacu` present, `.kangent` gone, `kamacu.db` present. Human confirmed board loads. |
| 2 | Every worktree stays valid — links repaired + DB paths rewritten to `~/.kamacu` (MIGRATE-02) | ✓ VERIFIED | `rewriteManagedPaths` (paths.go:65) prefix-swaps managed `repo_path`/`worktree_path`/`worktree_base` (managed=1, under old root only; `TrimPrefix`+`Join`, never a global replace). `repairWorktrees` (paths.go:192) runs `git worktree repair` PER PATH with arg-array exec. 21-07: 9/9 managed repos + 29/29 worktree paths rewritten under `.kamacu`, 0 under `.kangent`; migrated worktree `git status` exit 0. Human confirmed diff renders. Regression `TestCompleteRepairsMovedWorktrees` + `TestCompleteToleratesUnregisteredWorktree` pass. |
| 3 | tmux socket/prefix switched; live `kangent-*` sessions retired without orphaning an agent; new sessions use `-L kamacu` + `kamacu-<task>-<n>` (MIGRATE-03) | ✓ VERIFIED | `retireTmux` (migrate.go:125) kills the old `-L kangent` server (agents run as bare PTYs, never under tmux). `deleteOldTmuxRows` (paths.go:161) DELETEs `kangent-%` rows so reopened tasks respawn fresh. `tmux.DefaultSocket = "kamacu"` (tmux.go:22); `sessions.go:302` names `kamacu-%d-%d`; orphan sweep guards on the `kamacu-` prefix (main.go:307). 21-07: 0 `kangent-%` rows remain; human confirmed a fresh `kamacu-*` shell spawns and an agent resumes via `claude --resume`. |
| 4 | Browser `localStorage kamacu.*` keys populated from `kangent.*` (MIGRATE-04) | ✓ VERIFIED | `web/src/lib/migrateStorage.ts` one-shot separator-agnostic prefix scan copies every `kangent`-prefixed value to the `kamacu`-prefixed key when absent, guarded idempotent; invoked in `main.tsx:20` before `createRoot`. Components consume the new keys: `kamacu.sidebar` (AppLayout.tsx:8), `kamacu:sessions-bar-collapsed` (ActiveSessionsBar.tsx:33), `kamacu:review-collapsed:${projectId}` (ReviewColumn.tsx:56). Human confirmed carryover in the 21-07 browser walkthrough (recorded sign-off). |
| 5 | Migration is idempotent + safe: fresh/already-migrated skips; a failure leaves `~/.kangent` untouched with a clear error (MIGRATE-05) | ✓ VERIFIED | Pure 5-branch `Gate` (migrate.go:98): SkipCustom / DoMigrate / RollForward / FreshInstall / RefuseBoot. Preflight (line 212) runs before the `os.Rename` commit point; EXDEV and both-dirs anomalies refuse to boot with a clear `slog.Error` (main.go:77). Every Part-2 step self-gates on observable state (LIKE-gate, no-op repair, empty DELETE) so `Complete` is re-runnable on the RollForward path. 21-07: clean no-op second boot (no migrate line, dir stays `.kamacu`); real `~/.kangent` byte-identical before/after (120,359-file name+size manifest IDENTICAL). |

**Score:** 5/5 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/migrate/migrate.go` (Part 1) | Gate + preflight + atomic rename + WAL-safe DB rename + tmux retire | ✓ VERIFIED | 301 lines; substantive; 12 unit tests incl. hot-WAL guard, cross-device refusal, both-dirs RefuseBoot, Gate table. Wired in main.go:75. |
| `internal/migrate/paths.go` (Part 2) | Managed-path rewrite + per-path worktree repair + tmux-row delete | ✓ VERIFIED | 279 lines. Gap-1 fix present: `repairWorktrees` loops per path (line 232), on nonzero exit emits `slog.Warn` (line 241) + `continue` (line 244) — never returns the git error; `Complete` (line 42) still reaches `deleteOldTmuxRows`. Arg-array exec only (line 233), no `sh -c`. |
| `cmd/kamacu/main.go` wiring | Part 1 before store.Open, Part 2 after store.Migrate | ✓ VERIFIED | `migrate.Prepare` at line 75 (before `os.MkdirAll`/`store.Open`); `migrate.Complete` at line 113 (after `store.Migrate`, gated to DoMigrate/RollForward); refuse-to-boot on error (lines 77, 114). |
| `web/src/lib/migrateStorage.ts` | localStorage carryover | ✓ VERIFIED | Substantive one-shot prefix migration; wired in main.tsx:20 before render; consuming components read kamacu.* keys. Human-confirmed live (21-07). |
| `internal/migrate/worktree_repair_test.go` | Regression test for stale/unregistered worktree | ✓ VERIFIED | `TestCompleteToleratesUnregisteredWorktree` (line 187) faithfully reproduces the 4 real `sched` stragglers (real `git worktree add` then rm `.git/worktrees/<id>`); asserts Complete returns nil, valid worktree repaired, 0 `kangent-%` rows, second Complete no-op. PASS with the expected WARN skip lines. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| `main.go` | `migrate.Prepare` | before store.Open | ✓ WIRED | main.go:75, followed by store.Open at :91 |
| `main.go` | `migrate.Complete` | after store.Migrate | ✓ WIRED | main.go:113, after store.Migrate at :98 |
| `paths.go repairWorktrees` | `git worktree repair` (per path) | arg-array exec; nonzero → slog.Warn + continue | ✓ WIRED | paths.go:233 exec; :241 slog.Warn; :244 continue — Gap-1 fix |
| `paths.go Complete` | `deleteOldTmuxRows` | repair no longer returns fatal error | ✓ WIRED | paths.go:49 — DELETE always reached |
| `main.tsx` | `migrateStorage` | call before createRoot | ✓ WIRED | main.tsx:7 import, :20 invoke |
| `rewriteManagedPaths` | projects/tasks/settings rows | UPDATE WHERE managed=1 AND path LIKE oldRoot||'/%' | ✓ WIRED | paths.go:65-153, collect-then-update (SetMaxOpenConns(1) discipline) |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Full Go build | `go build ./...` | clean | ✓ PASS |
| Static analysis | `go vet ./...` | clean | ✓ PASS |
| Full test suite (13 pkgs) | `go test ./...` | all `ok` | ✓ PASS |
| Migrate package tests | `go test ./internal/migrate/... -count=1` | 17 `--- PASS` | ✓ PASS |
| Gap-1 regression | `go test -run TestCompleteToleratesUnregisteredWorktree -v` | PASS; WARN skip on ghost worktree; valid tree repaired; 0 kangent-% rows | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan(s) | Description | Status | Evidence |
|-------------|---------------|-------------|--------|----------|
| MIGRATE-01 | 21-02, 21-04, 21-05, 21-07 | One-time gated dir move + app runs against `~/.kamacu` | ✓ SATISFIED | Prepare wiring + Gap-1 fix; 21-07 live boot served |
| MIGRATE-02 | 21-03, 21-05, 21-06, 21-07 | Worktree links repaired + DB paths rewritten | ✓ SATISFIED | rewriteManagedPaths + per-path repair; 9/9 + 29/29 rewritten, clean git status |
| MIGRATE-03 | 21-03, 21-04, 21-05, 21-06, 21-07 | tmux socket/prefix switch + `kangent-*` reconciled | ✓ SATISFIED | retireTmux + deleteOldTmuxRows + DefaultSocket/sessions naming; 0 kangent-% rows |
| MIGRATE-04 | 21-01, 21-05, 21-07 | localStorage `kangent.*` → `kamacu.*` carryover | ✓ SATISFIED | migrateStorage.ts wired in main.tsx; human-confirmed live |
| MIGRATE-05 | 21-02, 21-04, 21-05, 21-07 | Idempotent + safe; fresh/migrated skip; failure recoverable | ✓ SATISFIED | 5-branch Gate + preflight-before-commit; clean no-op boot 2; real install byte-identical |

**Coverage:** 5/5 fully satisfied. All 5 MIGRATE requirement IDs appear in both plan frontmatter and REQUIREMENTS.md; no orphaned requirements.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| — | — | No `TBD`/`FIXME`/`XXX`/`HACK`/`PLACEHOLDER`/`TODO` in any migration or wiring file | — | None |

The Blocker anti-pattern from the prior report (`repairWorktrees` treating a batch non-zero exit as fatal) is RESOLVED: repair is now per-path with `slog.Warn` + skip.

### Gap Closure (from prior gaps_found report)

| Prior Gap | Status | Evidence on HEAD |
|-----------|--------|------------------|
| Gap 1 (code): `repairWorktrees` aborts the whole migration on a stale/unregistered worktree | ✓ CLOSED | paths.go:218-246 per-path loop; nonzero exit → `slog.Warn` + `continue`, never `return err`; commits `8875d6e` (RED) + `1bbd0e8` (GREEN) on HEAD, merged via `8295cdc`; `TestCompleteToleratesUnregisteredWorktree` passes. |
| Gap 2 (UAT harness): copied-HOME recipe let `git worktree repair` reach the real install | ✓ CLOSED | 21-07 git-safe recipe rewrites the copy's DB **and** all on-disk git link files before boot, gated by a real-GNU-grep zero-leak isolation assertion; RESEARCH OQ3 updated. Real install byte-identical (120,359-file manifest); `~/.kamacu` absent throughout. |

### Human Verification Required

None outstanding. The decisive end-to-end verification was a live, human-approved run (Plan 21-07) against a provably-isolated copy of the real install. The human confirmed: board loads, agent resumes via `claude --resume`, a fresh `kamacu-*` shell spawns, the diff renders, a migrated worktree's `git status` is clean, and `kamacu.*` localStorage keys carried over. These items are marked passed on the recorded 21-07 sign-off (2026-07-01); no new human run is demanded.

### Gaps Summary

No gaps. All five observable truths are VERIFIED and all five MIGRATE requirements are satisfied. Both blocking gaps from the prior 21-05 gate failure are closed on HEAD: the code fix (21-06) is present and covered by a passing regression test, and the harness fix (21-07) proved the migration completes live while leaving the real `~/.kangent` byte-identical. Plan 21-05 is closed as superseded-by-21-07. Build, vet, and the full 13-package test suite are green.

---
*Verified: 2026-07-01T19:30:00Z*
*Verifier: Claude (gsd-verifier)*
