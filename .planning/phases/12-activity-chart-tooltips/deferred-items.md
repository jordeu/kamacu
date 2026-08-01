# Phase 12 — Deferred Items (out-of-scope discoveries)

Out-of-scope issues discovered during Phase 12 execution. Logged per the executor
SCOPE BOUNDARY rule: "Only auto-fix issues DIRECTLY caused by the current task's
changes. Pre-existing warnings, linting errors, or failures in unrelated files
are out of scope."

## Pre-existing lint errors (discovered during Task 1 verify)

**Discovered:** 2026-08-01, running `npm run lint` in `web/` after Task 1
(`shadcn add chart`).

**Symptom:** `npm run lint` reports 30 problems (29 errors, 1 warning). The
errors are concentrated in files this phase did NOT touch — primarily
`web/src/pages/TaskPage.tsx:136` (`react-hooks/set-state-in-effect`) and other
pre-existing `react-hooks` rule violations.

**Verification that this is pre-existing (not caused by Phase 12):**
- `web/src/pages/TaskPage.tsx` is NOT in this phase's `files_modified` list.
- `git status --short -- src/pages/TaskPage.tsx` returns empty (file untouched).
- Linting ONLY the file this task added (`npx eslint src/components/ui/chart.tsx`)
  exits 0 — the generated shadcn primitive is clean.

**Disposition:** Out of scope for Phase 12. The plan's Task 1 acceptance
criterion is "no NEW lint errors introduced by the generated primitive" —
satisfied (chart.tsx lints clean). The pre-existing `react-hooks` errors predate
this phase and should be addressed in a dedicated cleanup task/phase, not
bolted onto a chart-tooltips phase.

**Repro:** `cd web && npm run lint`
