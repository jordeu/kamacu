# Deferred Items — Phase 26

Out-of-scope discoveries logged during execution. NOT fixed here (see scope boundary).

## Pre-existing frontend lint errors (unrelated to workspace routing)

Discovered during 26-05 verification (`cd web && npm run lint`): 22 pre-existing
`react-hooks/*` and `react-refresh/only-export-components` errors in files this
plan does not touch. These match the STATE.md "Pending Todos" note
("~18–20 pre-existing react-hooks eslint errors … gating lint, a dedicated pass
is the right home"). The build (`tsc -b && vite build`) is green; the files
modified by 26-05 (`web/src/App.tsx`, `web/src/components/sidebar/AddProjectDialog.tsx`)
lint clean (`npx eslint <those files>` exits 0).

Affected files (all untouched by 26-05):
- src/components/StatusDot.tsx
- src/components/board/Board.tsx
- src/components/board/PRCard.tsx
- src/components/board/ReviewColumn.tsx
- src/components/settings/SettingsField.tsx
- src/components/terminal/useTerminalSocket.ts
- src/components/ui/badge.tsx, button.tsx, sidebar.tsx, tabs.tsx
- src/hooks/use-mobile.ts
- src/pages/TaskPage.tsx
- (plus additional react-hooks/immutability + refs advisories)

Recommendation: a dedicated lint-cleanup pass (already tracked in STATE.md
Pending Todos), not folded into a feature plan.
