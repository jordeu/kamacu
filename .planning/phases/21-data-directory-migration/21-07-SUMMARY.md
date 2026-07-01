---
phase: 21-data-directory-migration
plan: 07
subsystem: infra
tags: [migration, git-worktree, sqlite, tmux, verification, uat]

# Dependency graph
requires:
  - phase: 21-06
    provides: stale/unregistered worktree tolerance in repairWorktrees (log+skip, not fatal)
  - phase: 21-05
    provides: copied-HOME UAT recipe + preserved human-verify checkpoint (this plan re-runs its gate)
provides:
  - Git-safe, provably-isolated end-to-end migration UAT re-run against a copy of the real ~/.kangent install
  - Live proof the 21-06 fix completes migration despite 4 stale sched worktree dirs (no abort)
  - Byte-identical real-install isolation proof (Gap 2 from 21-VERIFICATION closed)
  - Human-verified live migration (board, agent resume, kamacu-* shells, diff, clean worktree, carried localStorage)
  - Corrected copied-HOME UAT recipe recorded in RESEARCH OQ3 (git-link rewrite + isolation gate)
affects: [22, data-directory-migration, verification]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Copied-HOME UAT must rewrite on-disk git link files (not just DB) before boot — HOME override does not isolate git"
    - "Pre-boot isolation gate via REAL GNU grep -rl over the copy = zero files before booting"

key-files:
  created:
    - .planning/phases/21-data-directory-migration/21-07-SUMMARY.md
  modified:
    - .planning/phases/21-data-directory-migration/21-RESEARCH.md

key-decisions:
  - "No source files changed — this is a verification/gap-closure plan; the only repo change is the RESEARCH OQ3 note"
  - "Real GNU grep (not a wrapped grep/ugrep) is mandatory for the isolation gate — wrapped grep silently skipped .git dirs and gave a false-clean gate"
  - "Rewrite BOTH the copy's DB (with VACUUM) AND all on-disk git link files before boot so git repair stays inside the copy"

patterns-established:
  - "Git-safe copied-HOME UAT: rewrite DB + .git gitfiles + worktrees/*/gitdir (+commondir/config), then grep -rl isolation gate, then boot"
  - "HOME override != git isolation: git follows absolute gitdir/.git pointers regardless of $HOME"

requirements-completed: [MIGRATE-01, MIGRATE-02, MIGRATE-03, MIGRATE-04, MIGRATE-05]

# Metrics
duration: 8min
completed: 2026-07-01
---

# Phase 21 Plan 07: Git-Safe Migration Re-Verification Gate Summary

**Re-ran the MIGRATE-01..05 phase gate against a provably-isolated copy of the real 5.5 GB ~/.kangent install: the migration now completes and serves (21-06 stale-worktree fix proven live), the real install stayed byte-identical (Gap 2 closed), and the preserved human-verify checkpoint was approved.**

## Performance

- **Duration:** ~8 min (finalize step; Task 1 smoke ran earlier in the same plan)
- **Started:** 2026-07-01
- **Completed:** 2026-07-01
- **Tasks:** 2 (Task 1 auto PASS, Task 2 human-verify APPROVED)
- **Files modified:** 1 source-of-record (21-RESEARCH.md) + 1 created (this SUMMARY)

## Accomplishments

- **Git-safe copied-HOME smoke passed (Task 1, automated).** A throwaway `TMPHOME` received a `cp -a` copy of the real `~/.kangent` (5.5 GB, 120,359 files). Before any boot, the copy's DB paths were rewritten + `VACUUM`ed, and every on-disk git link file was rewritten from the real-home prefix to `$TMPHOME`: **27 repo-side `gitdir` link files** + **31 worktree `.git` gitfiles**, plus **51 prefix-carrying build-artifact/log/cache files removed**. The pre-boot isolation gate then passed with **real GNU grep AND the plan-literal `grep -rl` = 0 files**.
- **21-06 fix proven live (MIGRATE-02/03).** Boot 1 logged `"migrated ~/.kangent -> ~/.kamacu"` exactly once; the migration **completed** despite the real install's **4 stale sched worktree dirs** (`cluster-gc-...-28`, `add-nvidia-...-2`, `vmscalingservice-...-18`, `pr-561-22`), which were logged as WARN `"skipping unrepairable worktree (stale/unregistered)"` and skipped rather than aborting. `kamacu` served (`/api/healthz` → `{"status":"ok"}`); `.kamacu` present, `.kangent` gone, `kamacu.db` present.
- **DB + worktree state correct (MIGRATE-02/03).** 9/9 managed `repo_path` and 29/29 `worktree_path` rows under `$TMPHOME/.kamacu`, 0 under `.kangent`; `tmux_sessions LIKE 'kangent-%'` = 0 (`deleteOldTmuxRows` ran). One repo's `git worktree list` was clean and a migrated worktree (`fusion/pr-1461-5`) `git status` exited 0.
- **Idempotent no-op second boot (MIGRATE-05).** Boot 2 logged no migrate line, served, and left the dir at `.kamacu`.
- **Real-install isolation proven (Gap 2 closed, T-21-05-07 mitigated through the repair step).** Before/after name+size manifests of the real install (120,359 files) diffed **IDENTICAL**; `~/.kamacu` was absent throughout; the real `kangent.db` (4096) / `-wal` (2381392) / `-shm` (32768) sizes were unchanged; zero scratchpad refs leaked into the real install.
- **Human-verify approved (Task 2, MIGRATE-01..04).** The user drove the migrated instance in the browser and confirmed the board loads, an agent resumes via `claude --resume`, a fresh `kamacu-*` shell spawns, the diff renders, a migrated worktree's `git status` is clean, and `kamacu.*` localStorage keys carried over from `kangent.*`.
- **Corrected recipe recorded (RESEARCH OQ3).** Added the durable note: a safe copied-HOME UAT MUST rewrite the copy's on-disk git link files (`.git` gitfiles + `<repo>/.git/worktrees/*/gitdir`, plus `commondir`/`config`) AND the DB before boot, gated by a pre-boot `grep -rl "$HOME/.kangent" "$TMPHOME"` = zero-files assertion using REAL GNU grep — because HOME override does NOT isolate `git`.

## Task Commits

This is a verification/gap-closure plan — **no source files were changed** (by design). Task 1 was a smoke-only run against a scratchpad copy and made **no repo commits**; Task 2 was a human-verify checkpoint.

1. **Task 1: Git-safe copied-HOME smoke + real-install isolation proof** — no repo commit (scratchpad-only smoke; PASS)
2. **Task 2: Human-verify the live migration end-to-end** — no repo commit (human checkpoint; APPROVED)

**Plan metadata:** `docs(21-07): complete git-safe migration re-verification gate (human-approved)` — this SUMMARY + the RESEARCH OQ3 note.

## Files Created/Modified

- `.planning/phases/21-data-directory-migration/21-RESEARCH.md` — added the Gap-2 correction to Open Questions / OQ3: git-link-file rewrite requirement, the real-GNU-grep isolation gate, and the "HOME override != git isolation" caveat.
- `.planning/phases/21-data-directory-migration/21-07-SUMMARY.md` — this summary.

## Decisions Made

- **No source-file changes.** The 21-06 fix already lives on HEAD (`internal/migrate/paths.go` `slog.Warn("migrate: skipping unrepairable worktree (stale/unregistered)")`); this plan only re-verifies the gate and records the corrected recipe.
- **Real GNU grep is mandatory for the isolation gate.** A wrapped `grep` (routed to `ugrep`) silently excluded `.git` directories and produced a false-clean gate; the smoke caught and corrected this before booting, then re-asserted with real GNU grep.
- **Rewrite the copy's DB (with VACUUM) AND all git link files before boot.** This makes the copy fully self-contained so `git worktree repair` operates only within `$TMPHOME`, never reaching the real install.

## Deviations from Plan

None - plan executed exactly as written. Task 1's action already anticipated the wrapped-grep hazard implicitly via the isolation gate; catching and correcting the wrapped grep→ugrep within the harness is part of that gate, not a deviation, and is now recorded in RESEARCH OQ3 as the required real-GNU-grep caveat.

## Issues Encountered

- **Wrapped `grep` gave a false-clean isolation gate.** The environment's `grep` was routed to `ugrep`, which silently skipped `.git` directories — so the initial isolation assertion under-reported surviving real-home references. Resolved by re-running the gate with real GNU `grep` (and the plan-literal `grep -rl`), both returning 0 files before boot. Captured as a durable caveat in RESEARCH OQ3.

## User Setup Required

None - no external service configuration required. (Verification ran the built binary against a scratchpad copy of the real install; the real `~/.kangent` was read-only via `cp -a` and left untouched.)

## Next Phase Readiness

- The MIGRATE-01..05 phase gate that Plan 21-05 failed is now **passed live** (automated smoke + human sign-off) with the real install proven byte-identical — Gap 2 from 21-VERIFICATION is closed.
- No blockers. STATE.md / ROADMAP.md are intentionally left for the orchestrator to update on phase completion.

## Self-Check: PASSED

- `21-07-SUMMARY.md` created (verified on disk)
- `21-RESEARCH.md` OQ3 note present (git-link rewrite + real-GNU-grep isolation gate + "HOME override != git isolation")
- `internal/migrate/paths.go` on HEAD carries the 21-06 skip fix (`slog.Warn "migrate: skipping unrepairable worktree (stale/unregistered)"`)
- Task 1 made no repo commits (smoke-only, as designed)

---
*Phase: 21-data-directory-migration*
*Completed: 2026-07-01*
