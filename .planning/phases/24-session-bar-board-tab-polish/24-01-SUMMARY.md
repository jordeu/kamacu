---
phase: 24-session-bar-board-tab-polish
plan: 01
subsystem: api
tags: [go, sessions, tmux, rename, sqlite, http-patch, testing]

# Dependency graph
requires:
  - phase: 08-tmux-shells
    provides: "tmux_sessions table (migration 00005) with the label column, kamacu-<task>-<n> naming, reconcileTmux survivor path"
  - phase: 09-restart-durability
    provides: "post-restart reconcile that surfaces surviving tmux rows as orphaned reattach entries"
provides:
  - "Session.SetLabel — mutex-guarded rename write path making the set-once label mutable"
  - "PATCH /api/sessions/{id} — renames the in-memory label and (tmux) the persisted tmux_sessions.label row, returns the updated Info"
  - "D-04 fix: spawn INSERT persists the default Bash N label + reconcile re-derives Bash N from the tmux name (the Bash-? sentinel is gone)"
  - "defaultTmuxLabel helper: deterministic Bash <n> from a kamacu-<task>-<n> name"
affects: [24-04-tab-rename-ui, tabs-frontend, session-bar-polish]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "PATCH partial-update handler (*string present-vs-absent decode, TrimSpace, writeError/writeJSON) mirrored from tasks.go onto the session surface"
    - "Mutex-guarded mutable field setter (SetLabel) mirroring SetWaiting"
    - "Machine-derived default label from the server-minted tmux name (never a user-facing sentinel)"

key-files:
  created: []
  modified:
    - internal/session/session.go
    - internal/api/sessions.go
    - internal/api/sessions_test.go

key-decisions:
  - "Label max length capped at 200 runes (T-24-01 DoS mitigation), applied after trim/reset"
  - "Empty-reset on a non-tmux (plain-bash) tab leaves the current in-memory label unchanged — no derivable ordinal is exposed on Session, and a plain-bash tab does not survive a restart so there is nothing to re-derive"
  - "D-04 fixed belt-and-braces: INSERT the default label (never transiently '') AND reconcile re-derives Bash N — not a duplicate write; the two writes agree (both Bash N), so the existing back-fill UPDATE stays idempotent"

patterns-established:
  - "Session-label rename endpoint reuses the tasks.go PATCH posture verbatim"
  - "defaultTmuxLabel() is the single re-derivation used by both the reconcile fallback and the empty-reset path"

requirements-completed: [TABS-01, TABS-02]

# Metrics
duration: ~20min
completed: 2026-07-03
---

# Phase 24 Plan 01: Renameable Bash/Tmux Tab Backend Summary

**PATCH /api/sessions/{id} renames a session label in memory and (for tmux tabs) persists it to tmux_sessions.label so it survives a restart, plus a belt-and-braces D-04 fix that guarantees a restarted survivor never shows the "Bash ?" sentinel.**

## Performance

- **Duration:** ~20 min
- **Started:** 2026-07-03T17:10:00Z
- **Completed:** 2026-07-03T17:30:00Z
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments
- `Session.SetLabel` — a mutex-guarded setter mirroring `SetWaiting`, making the set-once-at-Spawn label mutable; `Info()` already reads `s.label` under the same mutex, so the next snapshot returns the new label with no other change.
- `PATCH /api/sessions/{id}` rename handler: 404 on an unknown/foreign session id (T-24-02), `{label}` decode → trim → 200-rune cap (T-24-01) → `SetLabel`, and — for tmux tabs only — persist to `tmux_sessions.label` (warn-only, mirroring the spawn back-fill). An empty/whitespace commit resets a tmux tab to its re-derived `Bash N` default (D-05).
- D-04 fix (belt-and-braces): the spawn INSERT now writes the default `Bash N` label so the column is never transiently `''`, and `reconcileTmux` re-derives `Bash N` from the `kamacu-<task>-<n>` name instead of the `"Bash ?"` sentinel — the sentinel is gone from the codebase.
- Four hermetic tests (rename happy-path + persistence, unknown-id 404, empty-reset re-derives `Bash N`, empty-label survivor reconciles to `Bash N` not the sentinel); full backend suite green, no regressions.

## Task Commits

Each task was committed atomically:

1. **Task 1: Add mutex-guarded Session.SetLabel** - `492d164` (feat)
2. **Task 2: rename handler + PATCH route + harden D-04 label persistence** - `8a82da7` (feat)
3. **Task 3: Tests — rename endpoint, restart label survival, empty-reset** - `e308733` (test)

_Task 3 is a validation-of-existing-behavior TDD task: the behavior it exercises was already implemented in Task 2, so the tests passed on first run (GREEN) with no separate RED-then-GREEN split needed. The implementation genuinely precedes the tests here because the rename write path is a single small handler; the tests confirm the four named behaviors rather than driving new code._

## Files Created/Modified
- `internal/session/session.go` - Added `SetLabel(label string)` mutex-guarded rename write path after `SetWaiting`.
- `internal/api/sessions.go` - Added the `rename` handler + `PATCH /api/sessions/{id}` route, the `defaultTmuxLabel` helper + `maxLabelRunes` const; hardened the spawn INSERT to persist the default label; replaced the `reconcileTmux` `"Bash ?"` fallback with `defaultTmuxLabel(name)`; added the `strings` import.
- `internal/api/sessions_test.go` - Added `TestRenameTmuxSession`, `TestRenameUnknownSession`, `TestRenameEmptyResetsDefault`, `TestReconcileNeverShowsBashQuestion`, and the `spawnTmuxTab` helper.

## D-04 Verification Finding (required by the plan output spec)

The CONTEXT's D-04 premise — "`tmux_sessions.label` is never written" — is **outdated**. Tracing the three label paths against the current code:

- **(a) Fresh spawn:** the INSERT at `sessions.go:303` wrote only `(task_id, n, name)`, leaving `label = ''` (the column DEFAULT); the warn-only back-fill UPDATE at `sessions.go:343-347` then writes `sess.Info().Label` (= `Bash N`). **A failed back-fill UPDATE leaves the row at `''`** — this is the one path that can produce an empty label.
- **(b) Reattach:** reads `label` at `sessions.go:265`, never re-writes it — so an already-`''` row stays `''`.
- **(c) Pre-existing / seeded rows:** any row written before the back-fill existed (or a manually-seeded `''` row) is likewise empty.

So the residual bug is **narrower than D-04 describes**: only a fresh-spawn row whose back-fill UPDATE failed (or any `''` survivor) reaches the `reconcileTmux` fallback, which turned `''` into `"Bash ?"`.

**Why the fix is belt-and-braces, not a duplicate write:** the INSERT-default (`Bash N` at insert) closes path (a) at the source so the column is never transiently `''`; the reconcile re-derivation (`Bash N` from the machine-minted name) closes paths (b)/(c) and any future `''`. The existing back-fill UPDATE is left in place because it reconciles the *in-memory* label (authoritative) into the row — and it writes the **same** `Bash N` the INSERT does, so it is idempotent reconciliation, not a conflicting second write. No duplicate `UPDATE` was added to the spawn path.

## Decisions Made
- **Label cap = 200 runes** (T-24-01): applied after trim and after the empty-reset re-derivation, bounding the stored + rendered string. Chosen as generous for a human tab name while preventing row/strip bloat.
- **Empty-reset on a non-tmux tab:** leaves the current in-memory label unchanged. `Session` exposes no per-task ordinal (the `Bash N` counter lives in the manager, not on the session), and a plain-bash tab does not survive a restart, so there is nothing meaningful to re-derive. Documented as Claude's discretion per the plan.
- **`defaultTmuxLabel` malformed-name fallback:** returns a safe non-empty `"Bash"` (never the old sentinel) when the trailing `<n>` cannot be parsed.

## Deviations from Plan

None - plan executed exactly as written. The `"Bash ?"` mentions that remained were in explanatory comments after the code fix; they were reworded to keep the literal sentinel string out of the file entirely so the acceptance grep (`grep -v '^#' | grep -c '"Bash ?"'` = 0) is satisfied. This is a wording refinement within Task 2, not a scope change.

## Issues Encountered
- The plan's Task 3 text references "a fake tmux client reporting the session alive," but `tmux.Client` is a concrete struct that shells out to the real `tmux` binary — there is no injectable fake in the harness. Resolved by mirroring the existing `TestSessionTmuxReattach` scaffold: a real detached tmux session under the seeded name plus a skip-guard (`exec.LookPath("tmux")`), which is exactly how every other tmux-dependent test in the file behaves. The unknown-id test needs no tmux (the `mgr.Get` miss fires first) and runs unconditionally.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- The backend seam for Plan 04's frontend tab-rename editor is complete: `PATCH /api/sessions/{id}` accepts `{label}`, returns the updated `Info` (with the new `label`), and persists tmux tabs across restarts. Plan 04 is now a pure UI wiring task (add a `patch` client helper / rename mutation + the inline editor in `TaskTabs.tsx`).
- No blockers. The `label` field already rides `TermSession`/`Info` on the wire, so no type change is needed frontend-side.

---
*Phase: 24-session-bar-board-tab-polish*
*Completed: 2026-07-03*
