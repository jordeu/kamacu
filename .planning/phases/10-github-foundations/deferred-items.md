# Deferred Items — Phase 10 (github-foundations)

Out-of-scope discoveries logged during plan execution. NOT fixed (SCOPE BOUNDARY:
only auto-fix issues directly caused by the current task's changes).

## Pre-existing lint errors (discovered during 10-03 execution)

`cd web && npm run lint` reports 18 pre-existing errors across files this plan
does NOT touch. None are in `SettingsPage.tsx`, `switch.tsx`,
`ProjectSettingsDialog.tsx`, or `ProjectMenu.tsx` (verified: zero lint errors in
the files changed by 10-03). The gating build (`tsc -b && vite build`) is green.

Rule breakdown:
- 8× `react-hooks/set-state-in-effect` — Board.tsx, SettingsField.tsx,
  RenameProjectDialog.tsx, CleanupWorktreeDialog.tsx, DiffTab.tsx,
  TerminalPane.tsx, TaskPage.tsx, use-mobile.ts
- 4× `react-hooks/refs` — TerminalPane.tsx
- 4× `react-refresh/only-export-components` — ui/button.tsx, ui/sidebar.tsx,
  ui/tabs.tsx, StatusDot.tsx (shadcn-generated + a const-export utility)
- 2× `react-hooks/immutability` — TerminalPane.tsx, useTerminalSocket.ts

These appear to be the result of a stricter `react-hooks` lint ruleset than the
code was written against (the affected files predate this phase). The 10-03 plan
treated `npm run lint` as expected-clean (its verify only inspects the last 10
lines); the failures are pre-existing and unrelated to the GitHub frontend.

Recommendation: a dedicated lint-cleanup pass (or relaxing the rule set to match
the established codebase patterns, e.g. the per-field commit `useEffect` in
`SettingsField`/`RenameProjectDialog`).
