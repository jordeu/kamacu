# Kangent

## What This Is

A local-only web app for organizing Claude Code agent sessions around projects and tasks. A left sidebar lists projects (each pointing at a local git repo checkout); the main area is a kanban board of tasks. Clicking a task expands a view with the main agent session (Claude Code CLI running in a PTY, rendered in a browser terminal) plus optional tabs with bash sessions. Single user, runs locally, accessed from a browser at localhost.

## Core Value

One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.

## Current Milestone: v1.3 GitHub PR Review

**Goal:** Surface the GitHub pull requests that need your review on a linked project's board, and open each as a full task-like review workspace — a worktree checked out on the PR's branch, with the same agent session, bash tabs, and diff as a normal task.

**Target features:**
- Global GitHub integration toggle in settings (enabled by default); when off, all GitHub UI disappears
- Per-project config section: optional short description + a linked GitHub repository
- Collapsible "Review" column on the right of a linked project's board, listing open PRs where review is requested from you, fetched via `gh`, auto-polled (paused when tab hidden) + manual refresh
- PR cards rendered like task cards; the list syncs live from GitHub (cards appear/disappear as review state changes)
- Clicking a PR opens a task-like view backed by a worktree that checks out the PR branch (`gh pr checkout`) instead of creating a new branch
- A review's worktree is auto-removed when its PR merges/closes (gated on dirty-tree + running sessions; branch kept)

**Settled decisions (milestone-time):**
- GitHub access is via the `gh` CLI (already authenticated on host) — no tokens stored; mirrors how Kangent shells out to git/claude. `gh` is a soft dependency: the integration is best-effort (like the quota indicator) and degrades gracefully when `gh` is absent or unauthenticated.
- PR reviews are a GitHub-synced list, not kanban tasks — they never enter To Do/In Progress/Done.
- No in-app GitHub write actions (approve/request-changes/comment/merge) — done in the terminal, preserving the manual-git philosophy.
- Worktrees CAN be auto-removed on PR merge/close — a deliberate, gated exception to the prior "worktrees never auto-removed" rule.

## Requirements

### Validated

- ✓ Projects sidebar — create/list/rename/delete projects pointing at local git repos, with path validation — Phase 1
- ✓ Kanban board per project with fixed columns (To Do / In Progress / In Review / Done), drag-and-drop with persisted manual order — Phase 1
- ✓ Tasks with title, markdown description, status; full CRUD and full-page task view with extensible tab strip — Phase 1
- ✓ All projects/tasks stored in local SQLite; single Go binary serves embedded SPA at localhost — Phase 1
- ✓ Real PTY-backed terminal in the browser: full interactive TUI, resize/scrollback/copy-paste, server-owned sessions that survive tab closes with replay on reattach, full-process-tree stop, Origin/Host-validated WebSocket with loopback-only binding — Phase 2
- ✓ Worktree-per-task isolation: task creation auto-creates `task/<slug>-<id>` branch + worktree under `~/.kangent/worktrees/`; bash session tabs run inside the worktree; confirmed cleanup with dirty-tree and running-session gates, branch always kept — Phase 3
- ✓ Claude Code agent sessions: explicit Start button spawns real `claude` in the worktree PTY (full TUI, inherits user settings, invisible additive status hooks, deterministic session IDs); live status dots (working/waiting/idle/exited) on cards, tab, and sidebar waiting chips; dimmed exited state with Reset session — Phase 4
- ✓ Server-side session persistence: sessions keep running when the browser tab closes and reattach with replay (Phase 2); after a server restart, silent reconciliation leaves no ghosts and the agent offers Resume session via `claude --resume <uuid>` from the persisted session ID — Phase 5
- ✓ Read-only diff tab: review everything a task changed vs the base branch merge-base (committed + uncommitted + untracked), collapsible unified diffs — Phase 5
- ✓ Global settings page (sidebar gear → full-page route, SQLite-backed, served over the API) with per-field commit, reset-to-default, and inline validation — Phase 6
- ✓ Configurable claude extra-params, pre-filled with `--dangerously-skip-permissions` (removable; applies at next spawn, running sessions unaffected) — Phase 6
- ✓ Configurable worktree base location (new worktrees only; existing stay put) — Phase 6
- ✓ Configurable bash-tab shell (dropdown, bash-only for now; bash de-hardcoded) — Phase 6
- ✓ Configurable branch-name token template (default `task/{slug}-{id}`, validated to a legal ref at save and create time) — Phase 6
- ✓ Task-view header layout spans full page width (title row + three-dots actions menu) — Phase 6
- ✓ Claude quota indicator: compact "Claude 5h" trigger + threshold-colored usage bar (zinc/amber/red, D-69) in the top-right of board and task views — Phase 7
- ✓ Quota hover popup with one row per server-reported window (5h, 7d, per-model): equal-width bars, rounded %, compact ticking reset countdowns, "Updated Xs ago" footer with cache-bypassing manual refresh — Phase 7
- ✓ Background quota auto-poll (60s, paused when tab hidden) backed by a cached, backoff-protected, token-keyed server proxy of the OAuth usage endpoint that never writes credentials and degrades without breaking — Phase 7
- ✓ "tmux" offered in the global shell setting dropdown only when `tmux` resolves on PATH (call-time LookPath, one truth shared by validation + options) — Phase 8
- ✓ Invisible tmux-backed bash tabs: spawn attach-or-create `kangent-<task>-<n>` sessions on a dedicated `-L kangent` socket (status off, mouse on), indistinguishable from plain bash tabs; × kills (kill-on-close parity), sessions survive leaving the task view and reattach on reopen; exit-vs-detach discriminated via `tmux has-session`; plain-bash and agent stop paths untouched (regression-guarded) — Phase 8
- ✓ tmux sessions survive a Kangent server restart and reattach automatically and invisibly when the task is reopened — no Resume button (DB-derived ghost entry gated by `has-session` → `new-session -A`); dead-on-restart sessions vanish quietly with lazy row GC — Phase 9
- ✓ Cleanup/delete account for tmux: live sessions fold into the single "N sessions running" count and are killed before worktree removal on both cleanup and task-delete; a once-at-startup orphan sweep kills any `kangent-*` session with no DB row (reconciles offline deletes) — Phase 9
- ✓ Done-TTL reaper: a background ticker kills sessions (bash, tmux, AND agent) of tasks in Done past a configurable TTL (`done_session_ttl`, default 24h from entering Done, `0`/`never` disables); the agent stays resumable (PTY killed, transcript kept); worktrees never auto-removed; new per-status timestamps (`todo_at`/`in_progress_at`/`in_review_at`/`done_at`) bank future cycle-time stats — Phase 9
- ✓ Global GitHub integration toggle (`github_integration` settings KV) with a full OFF cascade hiding all GitHub UI app-wide; **gh-gated**: when `gh` is absent the toggle defaults OFF and refuses to enable, surfacing "Install the GitHub CLI (gh) before enabling GitHub integration." (`GET /api/github/status` → `gh_available`); when `gh` is present it defaults on — Phase 10 (GHSET-01/02/03)
- ✓ Per-project config: optional short description (280 cap) + linked GitHub repo via Project settings dialog (⋯ menu), origin-prefilled and soft-validated through the degrade-don't-break `internal/github` leaf package (`gh` canonicalization with syntactic fallback) — Phase 10 (GHPRJ-01/02/03)
- ✓ Schema foundation for the whole milestone: migration 00007 adds `projects.description`/`github_repo` and `tasks.source`/`pr_number`/`pr_base_ref` (so Phases 11–13 need no further migration) — Phase 10

### Active

**v1.3 GitHub PR Review** (REQ-IDs in REQUIREMENTS.md):
- [ ] Collapsible PR review column listing review-requested-from-me open PRs via `gh`, auto-poll (paused when hidden) + manual refresh
- [ ] PR cards rendered like task cards, list synced live from GitHub
- [ ] PR review view: worktree on the PR branch (`gh pr checkout`) + agent/bash/diff like a normal task
- [ ] Auto-cleanup of the review worktree on PR merge/close (gated on dirty-tree + running sessions; branch kept)

### Out of Scope

- External task services (JIRA, Linear, etc.) — explicitly excluded; local database only
- Multi-user support, auth, remote deployment — single user at localhost only
- Desktop app packaging (Electron/Tauri) — this is a web app by design
- Multiple agent CLIs (Codex, Gemini, etc.) — Claude Code only for v1
- Merge/PR *write* automation from the app (approve/request-changes/comment/merge) — done by the user in the terminal; v1.3 adds read-only PR surfacing + worktree checkout for review, but no GitHub writes
- Storing GitHub credentials/tokens — access is via the host's already-authenticated `gh` CLI only (v1.3)
- Non-GitHub forges (GitLab, Bitbucket, Gitea) — GitHub-only for v1.3
- Custom kanban columns, labels, priorities — fixed columns and lean task cards for v1
- Auto-starting agents on task creation — sessions start only via explicit Start button

## Current State

**Shipped: v1.0 MVP (2026-06-11)** — 5 phases, 28 plans, 74 tasks. ~10,400 LOC Go + ~5,800 LOC TS/React. Single `make build` binary serving the embedded SPA at `127.0.0.1:7333`.

Kangent v1 does the whole loop: create a project on a local git repo → add a task (worktree + `task/<slug>-<id>` branch auto-created under `~/.kangent/worktrees/`) → Start a real `claude` session in the worktree PTY → watch the board as a dispatcher (status dots, amber when an agent needs you) → leave and reattach across tab closes and server restarts (`claude --resume`) → review the diff vs merge-base → mark Done with gated worktree cleanup. Stack: Go stdlib mux + modernc SQLite + creack/pty + coder/websocket; React 19 + Vite + Tailwind 4 + shadcn + xterm.js 6 + dnd-kit + TanStack Query.

**v1.2 complete (2026-06-13)** — all 3 phases done. Phase 7 — Claude quota indicator: `internal/quota` server proxy (`GET /api/usage`) + QuotaIndicator in both headers. Phase 8 — invisible tmux shells: `internal/tmux` leaf package, migration 00005 `tmux_sessions`, call-time shell dropdown, killer-first `Stop()`, end-to-end spawn wiring with honest 409 errors. Phase 9 — restart durability + cleanup: `GET /api/sessions` reconciles surviving tmux rows as auto-reattaching ghost tabs (TMUX-05, the milestone's headline — sessions survive a server restart and reattach invisibly), cleanup/delete kill tmux before worktree removal + a startup orphan sweep (TMUX-08), and the codebase's first background goroutine — a Done-TTL reaper keyed on new per-status timestamps (migration 00006) that kills bash/tmux/agent sessions of long-Done tasks while keeping the agent resumable (REAP-01). All phases verified green (full Go suite + frontend build).

**v1.3 in progress — Phase 10 (GitHub Foundations) complete (2026-06-13)** — 5 plans (3 original + 2 gap-closure). Schema migration 00007 lands all five v1.3 columns + the `github_integration` toggle KV; `internal/github` is the degrade-don't-break leaf (`ParseRepoRef`/`ValidateRepo`/`Available`, `gh` a soft dependency); projects gained partial-PATCH description + repo link with origin auto-detect; the `/settings` GitHub Switch and Project settings dialog ship the OFF cascade. UAT-driven refinement (`gh`-gated enablement + copy) closed via `GET /api/github/status` and a gh-aware toggle. Verified passed (full Go suite + web build green; gh-absent degrade exercised live). Next: Phase 11 — PR Review Column.

## Next Milestone

**v1.3 GitHub PR Review is now active** (started 2026-06-13). See "Current Milestone" above plus REQUIREMENTS.md / ROADMAP.md for scope and phasing.

Open candidates carried forward live in **Deferred** below. The v1.2 work also banked two forward investments worth a future milestone: per-status task timestamps (`todo_at`/`in_progress_at`/`in_review_at`/`done_at`, migration 00006) ready to power board cycle-time / dwell-time stats, and the first background-goroutine pattern (the Done-TTL reaper) that future periodic maintenance (e.g. MAINT-01 stale-worktree purge) can model on.

## Deferred (post-v1.1)

Parked candidates: browser notifications on waiting/finished (NOTF-01), one-click "Move to In Review?" on the Stop hook (NOTF-02), stale-worktree purge list (MAINT-01), MCP server for agent board access (AGNT-01), multiple agent CLIs, per-project settings overrides. One carried bug to confirm: whether plan-mode exit-plan approval triggers the amber waiting dot (research OQ1) — still unobserved as of Phase 6's gate (user approved without reporting it).

## Context

- Inspiration: layout and basic features of SlayZone (https://github.com/debuglebowski/SlayZone), but as a web app instead of a desktop app, in the spirit of vibe-kanban (https://github.com/BloopAI/vibe-kanban)
- Vibe-kanban-style worktree-per-task isolation: each task works on its own branch in its own worktree, so multiple agent sessions never collide in one checkout
- The agent is the real `claude` CLI in a pseudo-terminal — not an SDK-driven chat UI — so the full interactive experience (plan mode, slash commands, permission prompts) works as-is
- Server-side session persistence means the backend owns PTYs; the browser is just an attached view (xterm.js or similar). A server restart loses live processes; resuming via `claude --resume` is the recovery path

## Constraints

- **Tech stack**: Go backend, React frontend — user's choice
- **Deployment**: Single binary/process serving API + static frontend at localhost — local-only by design
- **Storage**: Local database (e.g., SQLite), no external services
- **Agent**: Claude Code CLI must be installed on the host; the app spawns it, never reimplements it

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Claude Code CLI in a PTY (not Agent SDK chat UI) | Full interactive CLI experience with zero reimplementation | ✓ Good — full TUI/plan-mode/hooks work unmodified (Phase 4) |
| Worktree + branch auto-created per task | Isolates concurrent agent sessions; vibe-kanban-proven model | ✓ Good (Phase 3) |
| Sessions persist server-side | Browser is a detachable view; work survives tab closes | ✓ Good — detach/reattach + restart resume (Phases 2, 5) |
| Explicit Start button for agent sessions | Most control, least surprise; no accidental agent runs | ✓ Good (Phase 4) |
| Go backend + React frontend | User preference | ✓ Good — single embedded binary |
| Manual git workflow, app only cleans up worktrees | Keeps v1 scope lean; user merges/PRs in terminal | ✓ Good (Phase 3) |
| Fixed columns: To Do / In Progress / In Review / Done | In Review holds agent-finished work awaiting human check | ✓ Good (Phase 1) |
| D-51 reversed: `--dangerously-skip-permissions` on by default (Phase 6, AGENT-02) | Dispatcher workflow favors unattended agents; flag is a removable settings default, so interactive prompts are one edit away | ✓ Intentional — documented side effect: amber waiting dot rarely fires while the flag is active |
| Quota via server-side proxy of the OAuth usage endpoint, never writing/refreshing the token (v1.2, Phase 7) | Browser never holds credentials; token rotation stays Claude Code's job; degrade-don't-break on every failure mode | ✓ Good — `internal/quota`, six-state matrix, token-keyed cache |
| Invisible tmux: × kills, leaving the task view detaches, sessions survive a server restart and auto-reattach with NO Resume button (v1.2, Phases 8–9, D-77/D-78/D-88) | Durability is the point; the user shouldn't have to know it's tmux. Reattach is cheap/lossless so it diverges from the agent's explicit Resume | ✓ Good — restart-survival verified end-to-end (the milestone's headline) |
| Done-TTL reaper kills idle bash/tmux/agent sessions of long-Done tasks; agent stays resumable; worktrees never auto-removed (v1.2, Phase 9, REAP-01/D-87/D-96) | Sessions shouldn't pile up on finished work; killing a PTY is reversible (transcript kept) but deleting a worktree is not | ✓ Good — codebase's first background goroutine, keyed on `done_at` |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd:transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd:complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-06-13 — Phase 10 (GitHub Foundations) complete; v1.3 in progress*
