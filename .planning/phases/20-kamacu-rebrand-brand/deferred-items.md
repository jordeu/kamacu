# Deferred Items — Phase 20

Out-of-scope discoveries logged during execution (NOT fixed — unrelated to the current task's changes).

## Pre-existing gofmt formatting drift (not caused by the rename)

These 8 files were already `gofmt`-dirty at the phase base commit (`0672e3b`), verified via `git show HEAD:<file> | gofmt -l`. The `"kangent/` → `"kamacu/` import rewrite did not shift any import ordering, so it did not cause or worsen these. `go build`/`vet`/`test ./...` are unaffected (they do not require gofmt). Left untouched to avoid out-of-scope churn; a dedicated `gofmt -w ./...` pass is the right home.

- `cmd/kamacu/main_test.go` (was `cmd/kangent/main_test.go` at HEAD)
- `internal/api/icons.go`
- `internal/github/prlist.go`
- `internal/github/service.go`
- `internal/reaper/reaper_test.go`
- `internal/settings/settings_test.go`
- `internal/ws/handler_test.go`
- `internal/ws/integration_test.go`

## Pre-existing frontend ESLint errors (not caused by the rebrand)

`cd web && npm run lint` reports 20 errors (exit 1) at the phase base — all in files NOT touched by plan 20-03. The six files this plan edits (`ProjectSidebar.tsx`, `App.tsx`, `ProjectMenu.tsx`, `SettingsPage.tsx`, `api/types.ts`, `api/sessions.ts`) lint clean (`npx eslint <those files>` exits 0). These are pre-existing `react-hooks/set-state-in-effect` and related violations, out of scope per the executor SCOPE BOUNDARY. Left untouched; a dedicated react-hooks cleanup pass is the right home. Affected files:

- `web/src/components/board/Board.tsx`
- `web/src/components/board/PRCard.tsx`
- `web/src/components/board/ReviewColumn.tsx`
- `web/src/components/settings/SettingsField.tsx`
- `web/src/components/sidebar/RenameProjectDialog.tsx`
- `web/src/components/StatusDot.tsx`
- `web/src/components/task/CleanupWorktreeDialog.tsx`
- `web/src/components/task/DiffTab.tsx`
- `web/src/components/terminal/TerminalPane.tsx`
- `web/src/components/terminal/useTerminalSocket.ts`
- `web/src/components/ui/button.tsx`
- `web/src/components/ui/sidebar.tsx`
- `web/src/components/ui/tabs.tsx`
- `web/src/hooks/use-mobile.ts`
- `web/src/pages/TaskPage.tsx`
