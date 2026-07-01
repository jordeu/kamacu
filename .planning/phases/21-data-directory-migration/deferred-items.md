# Deferred Items — Phase 21

Out-of-scope discoveries during execution (NOT fixed; logged per the scope-boundary rule).

## Pre-existing lint errors (not introduced by this phase)

`cd web && npm run lint` exits 1 with 20 pre-existing `react-hooks` errors in files
untouched by plan 21-01. These are the known tech debt already documented in
STATE.md ("~18–20 pre-existing react-hooks eslint errors ... gating build is green;
a dedicated lint-cleanup pass is the right home") and PROJECT.md "Known tech debt".

Affected files (none touched by 21-01):
- web/src/components/board/Board.tsx
- web/src/components/board/PRCard.tsx
- web/src/components/board/ReviewColumn.tsx (21-01 Task 2 flips a string literal only; the pre-existing Date.now()-in-render advisory is unrelated)
- web/src/components/settings/SettingsField.tsx
- web/src/components/sidebar/RenameProjectDialog.tsx
- web/src/components/task/CleanupWorktreeDialog.tsx
- web/src/components/task/DiffTab.tsx
- web/src/components/terminal/TerminalPane.tsx
- web/src/components/terminal/useTerminalSocket.ts
- web/src/hooks/use-mobile.ts
- web/src/pages/TaskPage.tsx

The files created/modified by 21-01 (`web/src/lib/migrateStorage.ts`, `web/src/main.tsx`,
plus the three key-literal flips) are lint-clean: `npx eslint` scoped to them exits 0.
The gating `tsc -b && vite build` is green for the whole tree.
