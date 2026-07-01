# Plan 21-05 Summary — SUPERSEDED by 21-07

**Status:** Closed — superseded by gap-closure plan 21-07 (human-approved 2026-07-01)
**Tasks:** Task 1 (auto smoke) RAN and FAILED by design; Task 2 (human-verify) not reached
**Requirements:** MIGRATE-01, MIGRATE-02, MIGRATE-03, MIGRATE-04, MIGRATE-05 — satisfied live via 21-07

## What happened

21-05 was the milestone's live end-to-end migration gate. Its Task 1 automated smoke ran against a copy of the real 5.5 GB `~/.kangent` install and **correctly failed**, surfacing two blocking defects (recorded in `21-VERIFICATION.md`, status `gaps_found`):

1. **Gap 1 (real code bug):** `migrate.repairWorktrees` aborted the whole migration on the first stale/unregistered worktree (the real `sched` repo's 4 stragglers), so the app refused to boot and never reached `deleteOldTmuxRows`. Deterministic on the real install.
2. **Gap 2 (unsafe UAT recipe):** the copied-HOME recipe rewrote the copy's DB but not its on-disk git link files, so `git worktree repair` followed absolute pointers into the real install. The executor detected this, recovered the real install, and verified it byte-identical.

The real `~/.kangent` install was left byte-for-byte intact (independently re-verified).

## Resolution

Both gaps were closed by gap-closure plans:

- **[[21-06]]** — per-path `git worktree repair` that logs (`slog.Warn`) and skips unregistrable worktrees instead of aborting; regression test reproduces the 4 real `sched` stragglers.
- **[[21-07]]** — git-safe copied-HOME UAT recipe (rewrites copy DB **and** on-disk git link files before boot, gated by a zero-leak isolation assertion), re-ran this exact gate, and drove the preserved Task 2 human-verify walkthrough.

## Outcome (via 21-07)

21-07's re-run demonstrated all five requirements live against a copy of the real install:
- Migration completed and **served** despite the 4 stale `sched` worktrees (21-06 fix proven live) — MIGRATE-01
- 9/9 managed repo paths + 29/29 worktree paths rewritten under `~/.kamacu`; migrated worktree `git status` clean — MIGRATE-02
- `-L kamacu` socket + `kamacu-*` prefix in effect; 0 `kangent-%` tmux rows remain — MIGRATE-03
- `kamacu.*` localStorage keys carried over (human-confirmed) — MIGRATE-04
- Clean no-op second boot; real install untouched throughout — MIGRATE-05
- **Human sign-off received** on the browser walkthrough (board, agent resume, fresh `kamacu-*` shell, diff, clean worktree, localStorage).

This plan's objective is therefore satisfied. No source files were produced by 21-05 itself; see `21-07-SUMMARY.md` for the passing gate evidence and `21-VERIFICATION.md` for the gap analysis.

## Self-Check: PASSED (superseded — objective met via 21-07)
