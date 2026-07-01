---
phase: 21-data-directory-migration
plan: 04
subsystem: infra
tags: [migration, startup-wiring, tmux, settings, go, kamacu-rebrand]

# Dependency graph
requires:
  - phase: 21-data-directory-migration
    provides: "internal/migrate Part-1 Prepare (gate + atomic rename + WAL-safe DB rename + old-tmux retire) and Part-2 Complete (managed-path rewrite + git worktree repair + kangent-* tmux-row cleanup), plus the Config/Decision contract"
  - phase: 20-kamacu-rebrand-and-brand
    provides: "kamacu Go module + binary; runtime data paths (~/.kangent), -L kangent tmux socket, kangent-* session prefix, and DB filename deliberately left un-flipped for this phase"
provides:
  - "cmd/kamacu/main.go wiring: migrate.Prepare before os.MkdirAll/store.Open (Part 1) and migrate.Complete after store.Migrate for DoMigrate/RollForward (Part 2); refuse-to-boot on either error"
  - "flipped --db default (~/.kamacu/kamacu.db) and all remaining runtime kangent literals: -L kamacu socket, kamacu-<task>-<n> session prefix, kamacu-managed tmux config header + filename, ~/.kamacu/worktrees root, ~/.kamacu/repos managed-clone root"
affects: []

# Tech tracking
tech-stack:
  added: []  # pure wiring + literal edits — no new dependencies
  patterns:
    - "Ordering-load-bearing startup splice: Part 1 before store.Open (dst-absent gate), Part 2 after store.Migrate (needs schema)"
    - "Decision-gated Part-2 invocation (DoMigrate/RollForward only; SkipCustom/FreshInstall no-op; RefuseBoot already errored in Prepare)"
    - "Silent one-shot: one slog.Info only on a performed move; slog.Error + os.Exit(1) naming both dirs + data is safe on failure (D-10/D-11)"

key-files:
  created: []
  modified:
    - "cmd/kamacu/main.go"
    - "internal/tmux/tmux.go"
    - "internal/settings/settings.go"
    - "internal/api/projects.go"
    - "internal/api/sessions.go"

key-decisions:
  - "slog.Info fires only on decision == DoMigrate (the boot that performs the os.Rename move); FreshInstall and already-migrated RollForward boots stay silent (D-10)"
  - "Part 2 runs for DoMigrate OR RollForward and a Complete error refuses to serve (D-04) so the next boot's RollForward re-runs the idempotent steps"
  - "The =name exact-match session guard in HasSession/KillSession/KillServer is preserved untouched — only the string prefix flipped to kamacu-"
  - "Old kangent literals remain ONLY inside cmd/kamacu/main.go migration log/error/comment copy (D-10/D-11) and internal/migrate (performs the move) and *_test.go — asserted by grep"

patterns-established:
  - "Textual-ordering acceptance: migrate.Prepare precedes os.MkdirAll; migrate.Complete follows store.Migrate and precedes api.BackfillProjectIcons"

requirements-completed: [MIGRATE-01, MIGRATE-03, MIGRATE-05]

# Metrics
duration: 17min
completed: 2026-07-01
---

# Phase 21 Plan 04: main.go Migration Wiring + Runtime Literal Flip Summary

**Wires the two-part `internal/migrate` one-shot into `cmd/kamacu/main.go` (Prepare before `store.Open`, Complete after `store.Migrate`, refuse-to-boot on either error) and flips every remaining runtime `kangent` literal — `--db` default, `-L kamacu` socket, `kamacu-<task>-<n>` session prefix, tmux config header/filename, and the `~/.kamacu` worktree + repos roots — so the running app switches atomically to `~/.kamacu`.**

## Performance

- **Duration:** ~17 min
- **Started:** 2026-07-01T15:16:18Z
- **Completed:** 2026-07-01T15:33:25Z
- **Tasks:** 2
- **Files modified:** 5 plan-scoped + 3 convention-comment + 3 test files

## Accomplishments
- **Two-part migration wired at the exact load-bearing seams** — `migrate.Prepare(ctx, dbPath, "~/.kamacu/kamacu.db")` runs immediately after the `--db` path resolves and BEFORE `os.MkdirAll`/`store.Open` (creating `~/.kamacu` early would defeat the dst-absent gate and fail `os.Rename`, RESEARCH Pitfall 4); `migrate.Complete(ctx, db, cfg)` runs after `store.Migrate` (it needs the schema) and before `api.BackfillProjectIcons`, only for a `DoMigrate`/`RollForward` decision.
- **Refuse-to-boot / silent-success semantics (MIGRATE-05, D-10/D-11)** — a `Prepare` error or a `Complete` error both `slog.Error` a terminal-visible line naming both `~/.kangent` and `~/.kamacu` plus "your data is safe" and `os.Exit(1)`; a performed move emits exactly one `slog.Info("migrated ~/.kangent -> ~/.kamacu")`; fresh installs and already-migrated boots stay silent.
- **Every remaining runtime `kangent` literal flipped (MIGRATE-01/03, D-08/D-12)** — `--db` default `~/.kamacu/kamacu.db`; `tmux.DefaultSocket` `kamacu` + `kamacu-managed` config header; the tmux config filename `kamacu-tmux.conf`; new bash-tab session names `kamacu-<task>-<n>`; the orphan-sweep prefix guard `kamacu-`; `KeyWorktreeBase` default `~/.kamacu/worktrees/`; and `reposBase` `~/.kamacu/repos/`. The `=name` exact-match session guard was left untouched so `kamacu-1-1` still never matches `kamacu-1-10`.
- **Full suite green** — `go build ./...`, `go vet ./...`, `go test ./...` (13 packages) all exit 0; `gofmt` clean; the acceptance grep for live `kangent` socket/prefix/path literals (excluding `internal/migrate/`, the intentional main.go migration copy, `_test.go`, and the unrelated `icons.go` deriveLetters example) returns nothing.

## Task Commits

Each task was committed atomically:

1. **Task 1: Wire Part 1 + Part 2 into main.go and flip the --db default + main.go path literals** - `44e0dcf` (feat)
2. **Task 2: Flip the tmux socket/prefix + settings/projects runtime defaults** - `e7bce56` (feat)

## Files Created/Modified
- `cmd/kamacu/main.go` - added `kamacu/internal/migrate` import; flipped `--db` default; spliced `migrate.Prepare` (Part 1, above `os.MkdirAll`) with refuse-to-boot + one-line success log; spliced `migrate.Complete` (Part 2, after `store.Migrate`, DoMigrate/RollForward only) with refuse-to-serve; flipped `kamacu-tmux.conf`, `~/.kamacu/worktrees`, and the `kamacu-` orphan-sweep prefix guard (+ its comments).
- `internal/tmux/tmux.go` - `DefaultSocket = "kamacu"`; `kamacu-managed` config header; updated the two illustrative prefix-match comments to `kamacu-1`/`kamacu-1-10`; the `=name` exact-match guard untouched.
- `internal/api/sessions.go` - new bash-tab session names built as `kamacu-%d-%d`.
- `internal/settings/settings.go` - `KeyWorktreeBase` default `~/.kamacu/worktrees/` (fresh-install only; stored rows are migrated by Plan 21-03).
- `internal/api/projects.go` - `reposBase = "~/.kamacu/repos/"` (fresh managed-clone root; existing rows rewritten by Plan 21-03) + comment accuracy.
- `internal/session/session.go`, `internal/session/manager.go`, `internal/worktree/worktree.go` - convention comments updated to name the new `kamacu-<task>-<n>` session scheme and `~/.kamacu/worktrees` root (comment-only, directly describing the code this plan flipped).
- `internal/settings/settings_test.go`, `internal/api/projects_test.go`, `internal/api/sessions_test.go` - flipped the pre-flip expectations these encode to the new intended behavior (worktree-base default, managed-repo harness base path, and the spawn-happy-path `kamacu-<T>-<n>` name assertions).

## Decisions Made
- **`slog.Info` only on `DoMigrate`:** the plan's "when the decision performed the move" maps to the boot that runs the `os.Rename` (DoMigrate); a `RollForward` boot finishes idempotent steps but the dir already moved, so it stays silent — matching D-10's "silent success = one line" and keeping fresh installs quiet.
- **Test expectations updated, not left stale:** the plan's acceptance criteria permit `_test.go` to still reference `kangent`, but three tests asserted the OLD default/prefix as the EXPECTED value (`TestDefaultsValues`, the managed-repo harness `newRepoTestServer`, and `TestSessionTmuxSpawnHappyPath`). Those are now the WRONG expectation post-flip, so they were updated to the new intended behavior (Rule 1 — a broken test assertion is a bug relative to the intended change). Self-consistent seeded reattach names (`TestSessionTmuxReattach` etc.) that pass regardless of prefix were left as-is per the plan's explicit `_test.go` allowance.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Updated three test assertions that hardcoded the pre-flip default/prefix**
- **Found during:** Task 2 (runtime-default flip)
- **Issue:** `go test ./...` failed after the flips because `internal/settings/settings_test.go` (`TestDefaultsValues`), `internal/api/projects_test.go` (`newRepoTestServer` managed-repo harness base), and `internal/api/sessions_test.go` (`TestSessionTmuxSpawnHappyPath` name1/name2) asserted the OLD `~/.kangent` default / `.kangent/repos` path / `kangent-<T>-<n>` spawn name as the expected value — now incorrect against the intentionally flipped code.
- **Fix:** Flipped each expectation to the new intended value (`~/.kamacu/worktrees/`, `.kamacu/repos`, `kamacu-<T>-<n>`) plus the immediately adjacent describing comments; ran `gofmt -w` on the settings files (the shorter `~/.kamacu` string shifted the trailing-comment column).
- **Files modified:** internal/settings/settings_test.go, internal/api/projects_test.go, internal/api/sessions_test.go, internal/settings/settings.go (gofmt realign)
- **Verification:** `go test ./internal/settings/ ./internal/api/` and the full `go test ./...` (13 packages) exit 0; `gofmt -l` clean.
- **Committed in:** e7bce56 (Task 2 commit)

**2. [Rule 3 - Accuracy] Updated three source convention comments naming the flipped scheme**
- **Found during:** Task 2 (final live-literal grep)
- **Issue:** `internal/session/session.go`, `internal/session/manager.go`, and `internal/worktree/worktree.go` doc/field comments still named the `kangent-<task>-<n>` session scheme and `~/.kangent/worktrees` root that this plan flipped — stale but not live literals.
- **Fix:** Updated the three comments to `kamacu-<task>-<n>` / `~/.kamacu/worktrees` for accuracy (comment-only, no behavior change).
- **Files modified:** internal/session/session.go, internal/session/manager.go, internal/worktree/worktree.go
- **Verification:** `go build ./...` + `go vet ./...` exit 0; grep for live socket/prefix/path literals returns clean.
- **Committed in:** e7bce56 (Task 2 commit)

---

**Total deviations:** 2 auto-fixed (1 Rule-1 bug in test expectations, 1 Rule-3 comment-accuracy)
**Impact on plan:** Both necessary to keep the suite green and the codebase accurate after the intentional literal flip. No scope creep — the unrelated `icons.go`/`icons_test.go` `"kangent" -> "KA"` deriveLetters example (a brand leftover, not a socket/prefix/path literal, unaffected by this plan's changes) was deliberately left out of scope.

## Issues Encountered
None beyond the test-expectation updates documented above. Task 1's build/vet/test were green immediately (the existing session/tmux tests use per-test sockets and did not depend on the prefix).

## Threat Surface Scan
No new security surface. Both threat-register mitigations this plan owns are in place and asserted: the Part-1-before-store.Open ordering (T-21-01-01, verified textually), the Complete-error refuse-to-serve (T-21-05-06), the socket/prefix flip paired with the preserved `=name` exact-match guard (T-21-03-02), and the single-slog.Info/terminal-visible-slog.Error clarity (T-21-01-02). No new dependencies (T-21-04-SC).

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Phase 21's runtime cutover is complete: with `~/.kangent` present, the app migrates the data root, DB file, managed paths, git worktree links, and tmux rows, then serves against `~/.kamacu` on the `-L kamacu` socket with `kamacu-*` session names.
- Remaining Phase 21 item outside this plan: the client-side localStorage migration (MIGRATE-04), if not already covered by a sibling wave-3 plan.

---
*Phase: 21-data-directory-migration*
*Completed: 2026-07-01*
