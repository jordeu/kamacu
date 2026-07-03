# Deferred Items — Phase 24

Out-of-scope discoveries logged during execution. Do NOT fix within the current plan.

## 24-02 — Pre-existing ESLint `react-hooks/set-state-in-effect` errors

- **Discovered during:** Plan 24-02 verification (`cd web && npm run lint`)
- **Scope:** Pre-existing, NOT caused by 24-02's changes (confirmed unchanged 21-error total before and after edits; none in the files 24-02 modified — `ActiveSessionsBar.tsx`, `Column.tsx`, `Board.tsx` line 201 change is clean).
- **Symptom:** `npm run lint` exits 1 with 21 errors flagging synchronous `setState()` calls inside `useEffect` bodies (rule `react-hooks/set-state-in-effect`, a newer React-hooks-plugin rule).
- **Affected files (16):** `hooks/use-mobile.ts`, `pages/TaskPage.tsx`, `components/board/Board.tsx` (line 73-75), `components/board/PRCard.tsx`, `components/board/ReviewColumn.tsx`, `components/settings/SettingsField.tsx`, `components/sidebar/RenameProjectDialog.tsx`, `components/StatusDot.tsx`, `components/task/CleanupWorktreeDialog.tsx`, `components/task/DiffTab.tsx`, `components/terminal/TerminalPane.tsx`, `components/terminal/useTerminalSocket.ts`, `components/ui/badge.tsx`, `components/ui/button.tsx`, `components/ui/sidebar.tsx`, `components/ui/tabs.tsx`.
- **Recommendation:** Address as a dedicated lint-cleanup pass (refactor the flagged effects per https://react.dev/learn/you-might-not-need-an-effect) — not appropriate to bundle into a UI-polish plan.
