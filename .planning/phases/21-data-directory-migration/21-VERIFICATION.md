---
phase: 21-data-directory-migration
verified: 2026-07-01T16:20:02Z
status: gaps_found
score: 1/5 must-haves verified
source_gate: 21-05 Task 1 automated migration smoke (copied-HOME UAT)
---

# Phase 21: Data Directory Migration Verification Report

**Phase Goal:** An existing `~/.kangent` install upgrades cleanly to `~/.kamacu` — live worktrees, sessions, the SQLite DB, and browser state all survive — while a fresh or already-migrated install skips safely and any failure is recoverable.
**Verified:** 2026-07-01T16:20:02Z
**Status:** gaps_found

This report captures the outcome of the phase gate (Plan 21-05's Task 1 automated smoke run against a copy of the real 5.5 GB `~/.kangent` install). Plans 21-01…21-04 are complete and merged; the gate exposed a real, blocking migration defect plus an unsafe UAT recipe. The real install was briefly mutated by the test harness (Finding 2) and has been fully recovered — independently re-verified byte-intact (see "Real Install Safety").

## Goal Achievement

### Observable Truths

| # | Truth (must_have) | Status | Evidence |
|---|-------------------|--------|----------|
| 1 | First boot migrates the dir to `~/.kamacu` and the app serves against it (MIGRATE-01) | ✗ FAILED | Dir move + WAL-safe DB rename succeed (`migrated ~/.kangent -> ~/.kamacu` logged once; `.kamacu` present, `.kangent` gone; `kamacu.db` present). BUT `migrate.Complete` returns an error at `repairWorktrees` → `os.Exit(1)`; `/api/healthz` never comes up. App does **not** serve on a real install. |
| 2 | After migration a migrated worktree is a clean `git status`; DB paths rewritten (MIGRATE-02) | ✗ FAILED (partial) | DB rewrite ✓ — all 9 managed `repo_path` + 29 `tasks.worktree_path` under `.kamacu`, 0 under `.kangent` (hot-WAL fold preserved full row integrity). Worktree repair ✗ — aborts on stale/unregistered worktree dirs (Finding 1). |
| 3 | Second boot is a clean no-op — dir stays `~/.kamacu`, no re-migration logged (MIGRATE-05) | ✗ FAILED (partial) | Rename gate correct (no migrate line, dir stays `.kamacu`). BUT boot 2 (RollForward) re-runs `Complete` → same `repairWorktrees` error → refuses to serve. Not a clean no-op. |
| 4 | Browser `kamacu.*` localStorage keys populated from `kangent.*` (MIGRATE-04) | ? UNCERTAIN (needs human) | Shipped in 21-01 (idempotent copy, wired in `main.tsx`); requires the browser walkthrough (Task 2), which was not reached because the automated smoke failed first. |
| 5 | Board loads, task view opens, a dead agent resumes, a diff renders (MIGRATE-01/02/03) | ? UNCERTAIN (needs human) | Blocked — the migrated instance cannot serve (Finding 1), so the human walkthrough could not run. |

**Score:** 1/5 truths verified (only the WAL-safe DB path rewrite fully passed; the dir-move mechanics work but the app cannot complete startup)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/migrate/migrate.go` (Part-1) | Gate + preflight + atomic rename + WAL-safe DB rename + tmux retire | ✓ EXISTS + SUBSTANTIVE | 9 unit tests pass incl. hot-WAL guard; dir move + DB rename verified live |
| `internal/migrate/paths.go` (Part-2) | Path rewrite + worktree repair + tmux row delete | ✗ DEFECTIVE | `repairWorktrees` aborts on stale worktrees (Finding 1); `deleteOldTmuxRows` never reached |
| `cmd/kamacu/main.go` wiring | Part-1 before store.Open, Part-2 after store.Migrate | ✓ EXISTS + SUBSTANTIVE | Ordering line-asserted; refuse-to-boot path works (arguably too aggressively — see Finding 1) |
| `web/src/lib/migrateStorage.ts` | localStorage carryover | ✓ EXISTS + SUBSTANTIVE | Needs human confirmation (Task 2) |

## Requirements Coverage

| Requirement | Status | Blocking Issue |
|-------------|--------|----------------|
| MIGRATE-01: dir moves + app runs against `~/.kamacu` | ✗ BLOCKED | App refuses to boot (Finding 1) |
| MIGRATE-02: worktrees valid + DB paths rewritten | ✗ BLOCKED | DB rewrite works; worktree repair aborts on stale worktrees (Finding 1) |
| MIGRATE-03: tmux socket/prefix switch + `kangent-%` row cleanup | ✗ BLOCKED | Socket/prefix flip ✓ (21-04); row DELETE never runs — `Complete` aborts before it (Finding 1) |
| MIGRATE-04: localStorage carryover | ? NEEDS HUMAN | Deferred to Task 2 (not reached) |
| MIGRATE-05: safe gate / recoverable failure / no-op second boot | ✗ BLOCKED | Gate + preflight safety ✓, but boot 2 refuses to serve (Finding 1); anomaly refuse-to-boot untested |

**Coverage:** 0/5 fully satisfied (1 partial DB-rewrite pass)

## Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| internal/migrate/paths.go | `repairWorktrees` / `existingWorktreePaths` | Passes all `os.Stat`-existing DB worktree paths to one `git worktree repair`; treats any non-zero exit as fatal | 🛑 Blocker | Real `sched` repo's 4 stale (existing-but-unregistered) worktree dirs make repair exit non-zero → migration refuses to boot and keeps failing on every RollForward |
| 21-05-PLAN.md / 21-RESEARCH.md OQ3 | copied-HOME recipe | Rewrites copy's DB paths but not on-disk `.git`/`gitdir` link files (baked absolute `/home/jordi/.kangent`) | 🛑 Blocker (UAT) | `git worktree repair` follows absolute pointers into the REAL install; HOME override does not isolate git. Threat T-21-05-07 ("real install untouched") unmitigated for the repair step |

## Real Install Safety (independently re-verified by orchestrator)

- `~/.kangent` present; `~/.kamacu` absent ✓
- Real DB byte-sizes unchanged (`kangent.db`=4096, `-wal`=2381392, `-shm`=32768; real DB never opened) ✓
- Zero `/tmp/claude-1000` references anywhere under `~/.kangent` ✓
- All managed repos' `git worktree list` show only `/home/jordi` paths ✓
- Executor's before-vs-recovered manifest (153,802 files, name+size): IDENTICAL. Only residual: one benign git stat-cache index inode change (size identical).

## Gaps Summary

### Critical Gaps (Block Progress)

1. **`repairWorktrees` aborts the whole migration on a single stale/unregistered worktree**
   - Missing: tolerance for DB-referenced worktree dirs that exist on disk but are not registered in the repo's `.git/worktrees/`
   - Impact: Deterministic refuse-to-boot on the real install (4 stale `sched` dirs); also prevents `deleteOldTmuxRows` (MIGRATE-03) from ever running. Blocks MIGRATE-01/02/03/05.
   - Fix: Before calling `git worktree repair`, intersect DB worktree paths with `git worktree list --porcelain` (skip unregistered) — or run repair per-path and log+skip individual "not a valid worktree" failures. Add a regression test for an existing-but-unregistered worktree dir.

2. **Copied-HOME UAT recipe lets `git worktree repair` reach into the real install**
   - Missing: relocation of the copy's on-disk `.git` gitfiles and `.git/worktrees/*/gitdir` (baked `/home/jordi/.kangent` → `$TMPHOME/.kangent`) before boot
   - Impact: The verification harness itself mutated the real install (recovered). Re-verification cannot be safely rerun until fixed.
   - Fix: In the UAT recipe, rewrite the copy's DB **and** all on-disk git link files to `$TMPHOME/.kangent` before the first boot, so post-rename pointers dangle exactly like the real in-place case. Update the 21-05 threat register to note HOME override does not isolate `git`.

### Non-Critical Gaps (Can Defer)

None — both gaps are blocking.

## Recommended Fix Plans

### 21-06-PLAN.md: Harden worktree repair against stale worktrees (code)

**Objective:** Make `migrate.Complete` survive DB-referenced worktree dirs that git no longer tracks, so the real install boots.

**Tasks:**
1. In `internal/migrate/paths.go`, filter the repair set to genuinely registered worktrees (intersect with `git worktree list --porcelain`) or tolerate per-path repair failures (log + skip); ensure `deleteOldTmuxRows` still runs afterward.
2. Add a regression test: a repo with an existing-but-unregistered worktree dir migrates without error and still deletes `kangent-%` tmux rows.
3. Verify: `go test ./internal/migrate/...` green; `go build`/`vet`/`test ./...` green.

**Estimated scope:** Small

---

### 21-07-PLAN.md: Make the copied-HOME UAT recipe git-safe (plan/harness)

**Objective:** A re-runnable, provably-isolated end-to-end migration UAT that cannot touch the real install.

**Tasks:**
1. Update the 21-05 UAT recipe (and 21-RESEARCH OQ3) to rewrite the copy's on-disk `.git`/`gitdir` link files (and DB) from `/home/jordi/.kangent` → `$TMPHOME/.kangent` before any boot; update the threat register (HOME override ≠ git isolation).
2. Re-run the automated smoke against a fresh copy; assert boot serves, boot 2 is a clean no-op, and the real install manifest is byte-identical before/after.
3. Present the human-verify checkpoint (Task 2) once the automated smoke passes.

**Estimated scope:** Small/Medium

---

## Verification Metadata

**Verification approach:** Live end-to-end gate (Plan 21-05 Task 1 automated smoke against a copy of the real install)
**Must-haves source:** 21-05-PLAN.md frontmatter + ROADMAP.md phase goal
**Automated checks:** 16 passed, 4 failed (dir move / WAL-safe DB rename / DB path rewrite pass; worktree repair / tmux-row delete / serve / clean-no-op fail)
**Human checks required:** blocked by Finding 1 (migrated instance cannot serve)
**Real install:** verified byte-intact after harness recovery

---
*Verified: 2026-07-01T16:20:02Z*
*Verifier: Claude (orchestrator, from Plan 21-05 gate failure)*
