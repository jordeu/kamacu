# Codebase Map

Generated: 2026-07-06T16:12:09Z | Files: 216 | Described: 0/216
<!-- gsd:codebase-meta {"generatedAt":"2026-07-06T16:12:09Z","fingerprint":"5cc56bb7eb09348ee78307e48d92cd53decaf97e","fileCount":216,"truncated":false} -->

### (root)/
- `.gitignore`
- `CLAUDE.md`
- `go.mod`
- `go.sum`
- `Makefile`
- `README.md`
- `skills-lock.json`

### cmd/kamacu/
- `cmd/kamacu/main_test.go`
- `cmd/kamacu/main.go`

### internal/api/
- *(43 files: 43 .go)*

### internal/api/testdata/
- `internal/api/testdata/fake-claude`

### internal/diff/
- `internal/diff/diff_test.go`
- `internal/diff/diff.go`
- `internal/diff/parse_test.go`
- `internal/diff/parse.go`

### internal/github/
- `internal/github/clone_test.go`
- `internal/github/clone.go`
- `internal/github/github_test.go`
- `internal/github/github.go`
- `internal/github/prlist_test.go`
- `internal/github/prlist.go`
- `internal/github/service_test.go`
- `internal/github/service.go`
- `internal/github/state_test.go`
- `internal/github/state.go`
- `internal/github/testhooks.go`

### internal/migrate/
- `internal/migrate/migrate_test.go`
- `internal/migrate/migrate.go`
- `internal/migrate/paths_test.go`
- `internal/migrate/paths.go`
- `internal/migrate/worktree_repair_test.go`

### internal/quota/
- `internal/quota/quota_test.go`
- `internal/quota/quota.go`

### internal/reaper/
- `internal/reaper/pr_reconcile_test.go`
- `internal/reaper/reaper_test.go`
- `internal/reaper/reaper.go`

### internal/session/
- `internal/session/agent_engine_test.go`
- `internal/session/agent_test.go`
- `internal/session/agent.go`
- `internal/session/manager.go`
- `internal/session/session_test.go`
- `internal/session/session.go`
- `internal/session/tmux_lifecycle_test.go`

### internal/settings/
- `internal/settings/home.go`
- `internal/settings/settings_test.go`
- `internal/settings/settings.go`
- `internal/settings/template_test.go`
- `internal/settings/template.go`
- `internal/settings/tokenize_test.go`
- `internal/settings/tokenize.go`
- `internal/settings/validate_test.go`
- `internal/settings/validate.go`

### internal/store/
- `internal/store/agents_migration_test.go`
- `internal/store/migrate.go`
- `internal/store/store_test.go`
- `internal/store/store.go`
- `internal/store/workspaces_migration_test.go`

### internal/store/migrations/
- `internal/store/migrations/00001_init.sql`
- `internal/store/migrations/00002_worktrees.sql`
- `internal/store/migrations/00003_agent_sessions.sql`
- `internal/store/migrations/00004_settings.sql`
- `internal/store/migrations/00005_tmux_sessions.sql`
- `internal/store/migrations/00006_task_status_timestamps.sql`
- `internal/store/migrations/00007_github_foundations.sql`
- `internal/store/migrations/00008_managed_checkout.sql`
- `internal/store/migrations/00009_project_icons.sql`
- `internal/store/migrations/00010_muted_palette.sql`
- `internal/store/migrations/00011_diff_viewed.sql`
- `internal/store/migrations/00012_workspaces.sql`
- `internal/store/migrations/00013_agents.sql`

### internal/tmux/
- `internal/tmux/tmux_test.go`
- `internal/tmux/tmux.go`

### internal/worktree/
- `internal/worktree/cleanup_gates_test.go`
- `internal/worktree/worktree_test.go`
- `internal/worktree/worktree.go`

### internal/ws/
- `internal/ws/handler_test.go`
- `internal/ws/handler.go`
- `internal/ws/integration_test.go`
- `internal/ws/proto.go`

### scripts/
- `scripts/smoke.sh`

### web/
- `web/.gitignore`
- `web/components.json`
- `web/embed.go`
- `web/eslint.config.js`
- `web/index.html`
- `web/package-lock.json`
- `web/package.json`
- `web/README.md`
- `web/tsconfig.app.json`
- `web/tsconfig.json`
- `web/tsconfig.node.json`
- `web/vite.config.ts`

### web/src/
- `web/src/App.tsx`
- `web/src/index.css`
- `web/src/main.tsx`

### web/src/api/
- `web/src/api/agents.ts`
- `web/src/api/client.ts`
- `web/src/api/diffs.ts`
- `web/src/api/mutations.ts`
- `web/src/api/pullRequests.ts`
- `web/src/api/queries.ts`
- `web/src/api/sessions.ts`
- `web/src/api/settings.ts`
- `web/src/api/types.ts`
- `web/src/api/usage.ts`
- `web/src/api/worktreeCleanup.ts`
- `web/src/api/worktrees.ts`

### web/src/components/
- `web/src/components/StatusDot.tsx`

### web/src/components/board/
- `web/src/components/board/Board.tsx`
- `web/src/components/board/Column.tsx`
- `web/src/components/board/NewTaskDialog.tsx`
- `web/src/components/board/PRCard.tsx`
- `web/src/components/board/ReviewColumn.tsx`
- `web/src/components/board/TaskCard.tsx`

### web/src/components/brand/
- `web/src/components/brand/KamacuMark.tsx`

### web/src/components/layout/
- `web/src/components/layout/ActiveSessionsBar.tsx`
- `web/src/components/layout/AppLayout.tsx`

### web/src/components/quota/
- `web/src/components/quota/QuotaIndicator.tsx`

### web/src/components/settings/
- `web/src/components/settings/CleanEligibleDialog.tsx`
- `web/src/components/settings/ForceRemoveDialog.tsx`
- `web/src/components/settings/SettingsField.tsx`
- `web/src/components/settings/WorktreeCleanupSection.tsx`
- `web/src/components/settings/WorktreeRow.tsx`

### web/src/components/sidebar/
- `web/src/components/sidebar/AddProjectDialog.tsx`
- `web/src/components/sidebar/ManageWorkspacesDialog.tsx`
- `web/src/components/sidebar/ProjectMenu.tsx`
- `web/src/components/sidebar/ProjectSettingsDialog.tsx`
- `web/src/components/sidebar/ProjectSidebar.tsx`
- `web/src/components/sidebar/RenameProjectDialog.tsx`
- `web/src/components/sidebar/WorkspaceNameDialog.tsx`
- `web/src/components/sidebar/WorkspaceSwitcher.tsx`

### web/src/components/task/
- `web/src/components/task/AgentTab.tsx`
- `web/src/components/task/CleanupWorktreeDialog.tsx`
- `web/src/components/task/DeleteTaskDialog.tsx`
- `web/src/components/task/DescriptionTab.tsx`
- `web/src/components/task/DiffFileSection.tsx`
- `web/src/components/task/DiffTab.tsx`
- `web/src/components/task/FileTree.tsx`
- `web/src/components/task/TaskTabs.tsx`
- `web/src/components/task/WorktreeMetaLine.tsx`

### web/src/components/terminal/
- `web/src/components/terminal/TerminalPane.tsx`
- `web/src/components/terminal/useTerminalSocket.ts`
- `web/src/components/terminal/xtermTheme.ts`

### web/src/components/ui/
- `web/src/components/ui/alert-dialog.tsx`
- `web/src/components/ui/badge.tsx`
- `web/src/components/ui/button.tsx`
- `web/src/components/ui/checkbox.tsx`
- `web/src/components/ui/collapsible.tsx`
- `web/src/components/ui/dialog.tsx`
- `web/src/components/ui/dropdown-menu.tsx`
- `web/src/components/ui/hover-card.tsx`
- `web/src/components/ui/input.tsx`
- `web/src/components/ui/label.tsx`
- `web/src/components/ui/ProjectAvatar.tsx`
- `web/src/components/ui/select.tsx`
- `web/src/components/ui/separator.tsx`
- `web/src/components/ui/sheet.tsx`
- `web/src/components/ui/sidebar.tsx`
- `web/src/components/ui/skeleton.tsx`
- `web/src/components/ui/switch.tsx`
- `web/src/components/ui/tabs.tsx`
- `web/src/components/ui/textarea.tsx`
- `web/src/components/ui/tooltip.tsx`

### web/src/hooks/
- `web/src/hooks/use-mobile.ts`
- `web/src/hooks/use-scroll-spy.ts`

### web/src/lib/
- `web/src/lib/migrateStorage.ts`
- `web/src/lib/palette.ts`
- `web/src/lib/time.ts`
- `web/src/lib/useActiveWorkspace.tsx`
- `web/src/lib/utils.ts`

### web/src/pages/
- `web/src/pages/BoardPage.tsx`
- `web/src/pages/SettingsPage.tsx`
- `web/src/pages/TaskPage.tsx`
- `web/src/pages/TerminalPage.tsx`
