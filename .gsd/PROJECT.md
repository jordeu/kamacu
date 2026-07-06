# Kangent → Kamacu

## What This Is

A local-only web app for organizing Claude Code agent sessions around projects and tasks. A left sidebar lists projects — grouped into switchable named workspaces — each pointing at a local git repo checkout; the main area is a kanban board of tasks. Clicking a task expands a view with the main agent session (Claude Code CLI running in a PTY, rendered in a browser terminal) plus optional tabs with bash sessions. Single user, runs locally, accessed from a browser at localhost.

**Core value:** One place to see and drive all agent work — every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.

## Milestone Status

**Between milestones.** v1.9 "Workspaces" shipped 2026-07-06. The next milestone is not yet scoped.

> **Format note (2026-07-06):** This project was migrated from the legacy `.planning/` (flat-markdown GSD v1.0) to the current `.gsd/` (SQLite-backed GSD v1.6.0) format. The full per-phase/per-task history of milestones v1.0–v1.9 lives archived under `.planning/` (committed to git, read-only). This `.gsd/` DB carries forward the **project context, durable decisions, knowledge, and the active requirements contract**; granular task checkbox history for the 26 shipped phases remains in the archive. New-format milestone IDs (M0XX) are independent of the legacy phase numbers (1–26); semantic versioning continues from v1.9.

## Constraints

- **Tech stack:** Go backend, React frontend (user's choice).
- **Deployment:** Single binary/process serving API + embedded static frontend at localhost — local-only by design.
- **Storage:** Local SQLite (`~/.kamacu/kamacu.db`), no external services.
- **Agent:** Claude Code CLI must be installed on the host; the app spawns it in a PTY and never reimplements it.

## Shipped Milestones (v1.0 → v1.9)

| Version | Name | Shipped | Phases · Plans · Tasks |
|---------|------|---------|------------------------|
| v1.0 | MVP | 2026-06-11 | 5 · 28 · 74 |
| v1.1 | Settings & Polish | 2026-06-11 | 1 · 4 · 10 |
| v1.2 | Quota & Resumable Shells | 2026-06-13 | 3 · 12 · 29 |
| v1.3 | GitHub PR Review | 2026-06-14 | 4 · 19 · 46 |
| v1.4 | Repo-First Projects | 2026-06-15 | 2 · 7 · 13 |
| v1.5 | Sharper Review Column | 2026-06-17 | 1 · 2 · 6 |
| v1.6 | Global Active Sessions Bar | 2026-06-18 | 1 · 2 · 5 |
| v1.7 | Project Icons in Collapsed Sidebar | 2026-06-19 | 2 · 6 · 12 |
| v1.8 | Kamacu Rebrand & UX Polish | 2026-07-04 | 5 · 26 · 53 |
| v1.9 | Workspaces | 2026-07-06 | 2 · 9 · 22 |

**Totals:** 26 phases · 115 plans · 270 tasks · 12 SQL migrations (00001–00012).

## Current Capabilities (Validated)

- **Projects & board** — four-column kanban (To Do / In Progress / Review / Done), task CRUD, SQLite persistence, drag-and-drop (dnd-kit).
- **Terminal engine** — server-owned PTY sessions with ring-buffer scrollback replay, binary WebSocket bridge (coder/websocket), xterm.js 6.0 (WebGL + DOM fallback), detach/reattach, loopback-only security.
- **Worktree isolation** — one git worktree per task (`task/<slug>-<id>`), bash tabs inside the worktree, conservative gated cleanup that never deletes the branch.
- **Claude Code agent sessions** — real `claude` CLI in the worktree PTY, status hooks (working/waiting/idle/exited), `claude --resume` recovery, dispatcher board with status dots + sidebar waiting chips.
- **Settings** — SQLite-backed KV store with absent-row-as-default; branch templates, extra-params tokenizer, shell select, claude extra-params applied at next spawn (managers stay DB-free).
- **Quota indicator** — DB-free proxy of the Claude OAuth usage endpoint, 60s TTL cache, six-state degradation.
- **Resumable tmux shells** — socket-isolated tmux (`-L kamacu`), ghost-tab reconcile on restart, Done-TTL reaper, tab renaming that survives restart.
- **GitHub PR review** — gh-gated integration, PR review column (awaiting + recently-reviewed), open-a-review on the PR's real head branch, auto-cleanup of merged/closed PR worktrees.
- **Repo-first projects** — managed `gh repo clone` checkout (`managed=1`), repo-first creation flow, atomic clone-then-insert, gated managed-delete.
- **Project icons** — colored monogram avatars (2 letters + muted palette), collapsed-sidebar rail + inline icon, editable in settings.
- **Global sessions bar** — persistent bottom bar showing active agent sessions across all projects, collapsed stats + expanded attention-first list.
- **GitHub-style diff review** — two-pane diff (file tree + collapsible per-file diffs), persistent per-file "Viewed" checkbox.
- **Worktree cleanup panel** — settings queue of orphan/stale/finished worktrees, per-item force-remove + bulk "clean eligible".
- **Workspaces** — `workspaces` table + `projects.workspace_id` FK, CRUD (`/api/workspaces`), expanded-only switcher, per-workspace sidebar filter (one `.filter`, both surfaces), workspace-aware routing, project transfer, cross-workspace sessions bar.

## Architecture Pillars (Load-Bearing Subsystems)

- **`internal/session`** — `SessionManager` (`map[sessionID]*Session` behind a mutex); each `Session` owns the `*exec.Cmd`, PTY `*os.File`, a fixed-size ring buffer for scrollback replay, and the set of attached WS clients. One goroutine per session reads the PTY and fans out to clients + buffer. Sessions outlive WS connections — that *is* detach/reattach.
- **`internal/ws`** — binary-frame WebSocket glue (never text frames — PTY output is arbitrary bytes; UTF-8 validation on text frames corrupts split multi-byte sequences). Control messages for resize.
- **`internal/worktree`** + **`internal/migrate`** — shells out to system git (`git worktree add/remove/list --porcelain`); never go-git (v5 can't create linked worktrees; v6 is alpha).
- **`internal/tmux`** — leaf package, every tmux verb in one place; `new-session -A`, `has-session` (only exit-vs-detach discriminator), `=name` exact-match.
- **`internal/github`** — single shared `github.Service` constructed once in `main.go`, injected into PR routes + reaper via a narrow `PRStateGetter` interface. Degrades silently when `gh` is absent.
- **`internal/store`** — SQLite via `modernc.org/sqlite` (pure Go, CGO-free), goose migrations embedded via `embed.FS`.
- **`internal/reaper`** — background goroutine: Done-TTL reaper for bash/tmux/agent sessions + a `reconcilePRsOnce` pass for merged/closed PR worktrees.
- **`web/`** — React 19 + Vite + TanStack Query + dnd-kit + Tailwind v4 + shadcn/ui; `//go:embed dist` ships the SPA in the binary.

## Stack

See the **Technology Stack** section in `CLAUDE.md` for the full versioned stack with rationale and alternatives. Headline versions: Go 1.26, React 19, Vite 8, TypeScript 6, `coder/websocket` v1.8.14, `creack/pty` v1.1.24 (import the v1 path — v2 is orphaned), `modernc.org/sqlite` v1.52.0, `@xterm/xterm` 6.0.0 (WebGL renderer; canvas removed in 6.0), `@dnd-kit/core` 6.3.1.

## Deferred / Future Backlog

Acknowledged-but-deferred items, candidates for a future milestone:
- **WSFUT-01:** Workspace icon/color identity (monogram + color like projects), shown in switcher and header.
- **WSFUT-02:** Reorder workspaces in the switcher (drag-to-reorder or manual order).
- **WSFUT-03:** Bulk / multi-select transfer of projects between workspaces.
- **ICON-FUT-05:** Add-project (+) and Settings (gear) affordances in the collapsed icon rail.
- **SBAR-FUT-03:** Browser/OS notifications for waiting agent sessions (currently in-bar alerting only).
- ~18–20 pre-existing `react-hooks/exhaustive-deps` eslint advisories carried as audited tech debt (gating build is `tsc -b && vite build`, which masks them).

## Verification Commands

Per `.gsd/PREFERENCES.md`:
- `go test ./...`
- `go vet ./...`
- `make test`
- Frontend gate: `cd web && tsc -b && vite build`
