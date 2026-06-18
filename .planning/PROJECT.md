# Kangent

## What This Is

A local-only web app for organizing Claude Code agent sessions around projects and tasks. A left sidebar lists projects (each pointing at a local git repo checkout); the main area is a kanban board of tasks. Clicking a task expands a view with the main agent session (Claude Code CLI running in a PTY, rendered in a browser terminal) plus optional tabs with bash sessions. Single user, runs locally, accessed from a browser at localhost.

## Core Value

One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.

## Current Milestone: v1.6 Global Active Sessions Bar

**Goal:** A persistent bottom status bar that shows every active Claude agent session across all projects at a glance — collapsed for global stats, expanded to browse and jump straight into any session.

**Target features:**
- Persistent **bottom status bar** in the app shell, present app-wide regardless of the current project/view.
- Tracks **agent sessions only** (working / waiting / idle; PR-review agent sessions included) across all projects, driven by an extended `/api/agents/status` feed (adds task title + project name).
- **Collapsed (default):** global stats — counts per state with waiting emphasized (pulsing amber) + a total.
- **Expanded:** a list of all active sessions (project · task/PR title · live state), attention-sorted (waiting first); clicking a row jumps into that task's full-page agent view (cross-project).
- **In-bar alerting only** for sessions needing input (pulsing-amber highlight + count badge) — no browser/OS notifications, no tab-title/favicon changes this milestone.
- Additive: the per-project sidebar "N waiting" chips stay; collapse/expand state persists (localStorage), default collapsed.

_v1.5 Sharper Review Column shipped 2026-06-17 (audited, archived to `milestones/v1.5-*`). Shipped-milestone history is in the collapsible blocks below._

<details>
<summary>Shipped milestone targets — v1.5 Sharper Review Column (2026-06-17)</summary>

**Goal:** Make the Review column convey two independent signals at a glance — your agent's state and the PR's CI state — and stop losing sight of PRs after you review them.

**Delivered features:**
- **Agent state on PR cards** — a colored **left rail** (green working / pulsing-amber waiting / blue idle / gray exited) marks any PR with an open review session; redesigned from a `StatusDot` dot to a rail at the human-verify gate so it never clashes with the CI mark.
- **CI status as icons** — bare glyphs: green check (passing) / red cross (failing) / static amber circle (running); no-checks (`none`) renders nothing.
- **"Recently reviewed" section** — open PRs you've reviewed (`reviewed-by:@me`, approve OR request-changes) stay visible until merged/closed; the top section remains "awaiting your review"; a PR appears in only one section (top wins; server-side deduped); quietly omitted when empty.

**Settled decisions (milestone-time):**
- Two searches drive the two sections (`user-review-requested:@me draft:false` + `reviewed-by:@me`) from ONE cached `gh` cycle, server-side top-precedence deduped; `--state open` bounds the reviewed list (drops on merge/close) — no separate time window.
- Agent state is a **left rail, not a dot** (user-directed redesign at the human-verify gate): the two signals split onto separate visual channels (left edge = your agent, right glyph = the PR's CI).
- All-internal milestone: reuses `internal/github` query/cache, the `source='github_pr'` task↔PR link + `/api/agents/status`, and the reaper's merge/close detection. No new external surface beyond the second `gh pr list` search.

</details>

**v1.4 Repo-First Projects — shipped 2026-06-15.** See the Validated requirements and Current State below for what v1.4 delivered.

<details>
<summary>Shipped milestone targets — v1.4 Repo-First Projects (2026-06-15)</summary>

**Goal:** When GitHub integration is on, add a project by naming a GitHub repo — Kangent clones and manages the checkout under `~/.kangent` on the default branch — with the folder path as the optional fallback.

**Delivered features:**
- Repo-first "Add project" when GitHub is on: enter `owner/name` → Kangent `gh repo clone`s it into `~/.kangent/repos/<owner>/<name>` on the repo's auto-detected default branch (main/master).
- Project name auto-derived from the repo (prefilled, editable); GitHub link auto-set + description auto-captured.
- Folder path becomes the optional alternative when GitHub is on; folder-only when GitHub is off — existing folder-based projects untouched (backward compatible).
- Task (and PR-review) worktrees branch off the managed checkout; best-effort fetch of the latest default branch before creating each new task worktree.
- Deleting a managed-checkout project does an all-or-nothing gated removal of the clone (dirty / unpushed / stash / running-session gates, like worktree cleanup); folder-based project dirs are never touched.
- Degrade-don't-break: clone failures (auth/network/bad repo) surface inline without leaving a half-created project; re-adding an existing managed dir reattaches instead of failing.

**Settled decisions (milestone-time):**
- Repo cloning uses `gh repo clone` (host auth, so private/org repos work) into an `owner/name`-namespaced dir under `~/.kangent/repos/` — collision-safe.
- The managed clone IS the repo root that task/PR-review worktrees branch off (the user just never picks a folder); the `managed` schema marker (migration 00008) distinguishes Kangent-managed checkouts from user-pointed folders so delete knows what it owns.
- GitHub-only for the repo path (consistent with v1.3); arbitrary git URLs / other forges stay out of scope — the folder is the non-GitHub escape hatch.
- Built on v1.3's `internal/github` (gh auth, `ParseRepoRef`/`ValidateRepo`, repo link) and the existing worktree/cleanup gating.

</details>

<details>
<summary>Shipped milestone targets — v1.3 GitHub PR Review (2026-06-14)</summary>

**Goal:** Surface the GitHub pull requests that need your review on a linked project's board, and open each as a full task-like review workspace — a worktree checked out on the PR's branch, with the same agent session, bash tabs, and diff as a normal task.

**Delivered features:**
- Global GitHub integration toggle in settings (gh-gated; default on when `gh` present, OFF + un-enableable when absent); when off, all GitHub UI disappears
- Per-project config section: optional short description + a linked GitHub repository
- Collapsible "Review" column on the right of a linked project's board, listing open PRs where review is requested from you, fetched via `gh`, auto-polled (paused when tab hidden) + manual refresh
- PR cards rendered like task cards; the list syncs live from GitHub (cards appear/disappear as review state changes)
- Clicking a PR opens a task-like view backed by a worktree on the PR's real head branch (fetch `refs/pull/<n>/head`, named branch with `pr/<n>` collision fallback — **not** `gh pr checkout`, which the research ruled out as not worktree-aware)
- A review's worktree is auto-removed when its PR merges/closes (gated on dirty/unpushed/stash/running-session; branch kept)

**Settled decisions (milestone-time):**
- GitHub access is via the `gh` CLI (already authenticated on host) — no tokens stored; mirrors how Kangent shells out to git/claude. `gh` is a soft dependency: the integration is best-effort (like the quota indicator) and degrades gracefully when `gh` is absent or unauthenticated.
- PR reviews are a GitHub-synced list, not kanban tasks — they never enter To Do/In Progress/Done.
- No in-app GitHub write actions (approve/request-changes/comment/merge) — done in the terminal, preserving the manual-git philosophy.
- Worktrees CAN be auto-removed on PR merge/close — a deliberate, gated exception to the prior "worktrees never auto-removed" rule.

</details>

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
- ✓ Collapsible per-project "Review" column listing `user-review-requested:@me draft:false` open PRs via one cached `gh pr list` call (server-side `statusCheckRollup` → pass/fail/pending/none), 60s visibility-paused auto-poll + manual refresh, inline loading/empty/degraded states, default-collapsed, never blocking the board — Phase 11 (GHCOL-01..06)
- ✓ Open-a-Review: clicking a PR card opens a task-like review workspace (`source='github_pr'` task reusing TaskPage/agent/bash/diff) backed by a worktree on the PR's real head branch (fetch `refs/pull/<n>/head`, `pr/<n>` collision fallback, primary checkout HEAD provably unchanged); open-or-reattach (no duplicates); diff vs the PR's own base merge-base matching GitHub; PR reviews never leak onto the kanban board — Phase 12 (GHREV-01..05)
- ✓ PR worktree auto-cleanup: the reaper reconciles PR state (`gh pr view --json state`) and gated-removes a merged/closed worktree only when pristine + idle (dirty/unpushed/stash/session all skip), always keeping the branch; manual cleanup from the review view via the same gated `CleanupWorktreeGated` flow + a merged/closed banner — Phase 13 (GHCLN-01/02/03)
- ✓ Managed-checkout backend: `github.Clone` (`gh repo clone`, exit-0-only) provisions a gh-validated repo into `~/.kangent/repos/<owner>/<name>` recorded as the project repo root with a `managed` marker (migration 00008, existing folder rows backfill to never-touch); atomic clone-then-create (failed clone → no row, no dir); reattach-on-origin-match / error-on-mismatch — Phase 14 (CKOUT-01, RPROJ-05, CKOUT-05)
- ✓ Managed-checkout worktree freshness + gated delete: managed task/PR-review worktrees best-effort `fetch origin <default>` before `ResolveBase` (folder projects keep the D-24 no-network path); deleting a managed project is all-or-nothing gated (dirty/unpushed via `origin/<default>..HEAD`/stash/session across every worktree + the clone root → 409 reason list, removes nothing) then unlinks worktrees → `os.RemoveAll` clone → deletes row; folder-project delete byte-for-byte unchanged — Phase 14 (CKOUT-02, CKOUT-03)
- ✓ Repo-first Add-project flow: when GitHub is on, the "Add project" dialog shows a segmented "GitHub repo | Local folder" toggle (repo default, reusing `ui/tabs.tsx`); the `owner/name` input prefills the editable project name, submit drives the Phase-14 atomic create with a blocking "Cloning…" spinner, clone failures surface inline (dialog stays open, values preserved, no half-created project), and the GitHub repo description is auto-captured into the project. When GitHub is off, the dialog is byte-for-byte the pre-v1.4 folder-only form — Phase 15 (RPROJ-01/02/03/04, CKOUT-04)

- ✓ Sharper Review column — agent session state and CI state as two independent at-a-glance signals per PR: agent state is an always-on colored **left rail** on the card (green working / pulsing-amber waiting / blue idle / gray exited — redesigned from a `StatusDot` to a rail at the human-verify gate so it never clashes with the CI mark), CI state is a bare glyph (green `Check` / red `X` / static amber `Circle`; no-checks renders nothing). Two separate channels — left edge = your agent, right glyph = the PR's CI — Phase 16 (SIGNL-01/02/03, CHECK-01/02)
- ✓ "Recently reviewed" section in the Review column — open PRs you've reviewed (`reviewed-by:@me draft:false`, approve OR request-changes) stay visible until merged/closed, riding one cached `gh` cycle, server-side deduped against the awaiting-review list (top precedence), quietly omitted when empty — Phase 16 (REVWD-01/02/03/04)

- ✓ Global Active Sessions Bar — a persistent bottom bar (`ActiveSessionsBar`, mounted in `AppLayout` outside `<Outlet/>`, every route) showing every LIVE agent session across all projects. Collapsed (default) shows per-state colored counts (working/waiting/idle via reused `dotMeta()`) + total with the waiting count amber + pulsing; expanded floats a panel UP over content (overlay — never reflows the xterm terminals) listing sessions attention-first (waiting→working→idle), each row `project · task/PR title` (PR rows get a `#n` badge), the current task's row highlighted, click → that task's agent view (cross-project). Live-only (exited drops off); quiet "No active sessions" zero state; collapse persists in `localStorage` (default collapsed). Driven by the existing 5s `/api/agents/status` poll, widened with `taskTitle`+`projectName` via a JOIN on both passes (no new endpoint, no migration); per-project sidebar waiting chips kept (additive) — Phase 17 (SBAR-01..SBAR-10)

### Active

_No milestone currently active. v1.6 Global Active Sessions Bar shipped 2026-06-18 (Phase 17) — verified (11/11 must-haves, human-verify approved). Run `/gsd:complete-milestone` to archive, or `/gsd:new-milestone` to scope the next cycle._

Carried-forward candidates from earlier milestones live in **Deferred** below and in the archived milestones' Future Requirements.

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

**v1.3 in progress — Phase 10 (GitHub Foundations) complete (2026-06-13)** — 5 plans (3 original + 2 gap-closure). Schema migration 00007 lands all five v1.3 columns + the `github_integration` toggle KV; `internal/github` is the degrade-don't-break leaf (`ParseRepoRef`/`ValidateRepo`/`Available`, `gh` a soft dependency); projects gained partial-PATCH description + repo link with origin auto-detect; the `/settings` GitHub Switch and Project settings dialog ship the OFF cascade. UAT-driven refinement (`gh`-gated enablement + copy) closed via `GET /api/github/status` and a gh-aware toggle. Verified passed (full Go suite + web build green; gh-absent degrade exercised live).

**Phase 11 (PR Review Column) complete (2026-06-14)** — 4 plans, strict sequential chain. `internal/github` gained `ListReviewRequested` (the verified `gh pr list --search "user-review-requested:@me draft:false"` arg array with `cmd.Dir` = repo path, server-side `statusCheckRollup` → pass/fail/pending/none reduction, and exit-code+stderr degraded classification) plus a per-repo TTL cache `Service` cloned from `quota.Service` — no N+1 `gh pr checks`. Always-200 `GET /api/projects/{id}/pull-requests[?refresh=1]`, two-gate ladder (toggle → link) short-circuiting before any `gh` spawn. Frontend: `usePullRequests` (60s visibility-paused poll + manual refresh), `formatAgo` lifted to `lib/time.ts`, `PRCard`, and a self-gating collapsible `ReviewColumn` pinned right of Done outside the dnd machinery. Per-project localStorage collapse persistence, now **default collapsed** (user override of D-05/D-06 during the human-verify checkpoint). 17/17 must-haves verified; human-verify checkpoint approved.

**Phase 12 (Open-a-Review) complete (2026-06-14)** — 7 plans (5 original + 2 gap-closure after the human-verify checkpoint). The milestone headline: clicking a PR card opens it as a task-like review workspace — a `tasks` row with `source='github_pr'` rendered through the same `TaskPage`/`TaskTabs` shell. `worktree.CheckoutPR` fetches `refs/pull/<n>/head` and `git worktree add -b <branch> <path> <headOID>` on the PR's **real head branch** with a `pr/<n>` collision fallback (the named-branch path *supersedes* the original detached D-01 after human-verify, and preserves GHREV-05 — an existing local ref is never reused/moved, main checkout HEAD provably unchanged). Open-or-reattach `POST .../pull-requests/{n}/review` (find by project+pr_number) + read-only `GET .../pull-requests/{n}` (live `gh pr view` re-hydration on F5). Diff branches on `pr_base_ref` (fetch + merge-base `origin/<base>`, renderer reused). Board-leak guard: `source='manual'` on all 5 board/position queries + `/move` 409. Frontend: read-only single-line clickable GitHub-style header (`#<n> @<author> wants to merge <N> commits into <base> from <head>`), read-only PR-body Description, Agent-tab-first, seed prefilled-once-per-session from the configurable `pr_review_seed` Settings field. 5/5 success criteria verified; human-verify approved end-to-end.

**Phase 13 (PR Worktree Auto-Cleanup) complete (2026-06-14) — v1.3 fully implemented** — 3 plans. The reaper (the Phase 9 Done-TTL goroutine) gains a second `reconcilePRsOnce` pass: per tick, each `source='github_pr'` task with a worktree is checked via `github.PRState` (`gh pr view --json state` → OPEN/CLOSED/MERGED); a MERGED/CLOSED PR's worktree is auto-removed **only when pristine and idle** (conservative gate: uncommitted / unpushed via `rev-list FETCH_HEAD..HEAD` re-fetched in the worktree / stash / running session — skip on any), and on success the task row is deleted (FK-ordered `tmux_sessions` before `tasks`). The gated-cleanup logic was extracted byte-equivalent into `CleanupWorktreeGated` (shared by the HTTP handler + reaper; reaper passes `force=false`, never stops sessions); the branch ref is never deleted. Manual cleanup (GHCLN-03) re-adds a PR `⋯` "Clean up worktree" item + a merged/closed banner in the review view. 18/18 must-haves verified; human-verify approved end-to-end. **v1.3 (GitHub PR Review) shipped 2026-06-14** — milestone audit passed (20/20 requirements, 6/6 integration seams, 4/4 E2E flows), archived to `milestones/v1.3-*`.

**v1.4 in progress — Phase 14 (Managed Checkout Foundations) complete (2026-06-14)** — 4 plans, the backend capability for Kangent-managed checkouts. Migration 00008 adds the `managed` marker (existing folder projects backfill to never-touch). `github.Clone` wraps `gh repo clone` (exit-0-only, test-seam) into `~/.kangent/repos/<owner>/<name>`; `createByRepo` does gh-validate → clone → INSERT atomically (failed clone leaves no row, no dir) with reattach-on-origin-match. Managed task/PR worktrees best-effort fetch the default branch before `ResolveBase` (folder projects keep the D-24 no-network path); managed-project delete is all-or-nothing gated (dirty/unpushed/stash/session across every worktree + clone root) then unlinks worktrees → removes the clone dir → deletes the row, while folder-project delete stays byte-for-byte unchanged. 5/5 requirements verified (CKOUT-01/02/03/05, RPROJ-05); `go build`/`vet`/`test ./...` all green.

**v1.4 fully implemented — Phase 15 (Repo-First Creation Flow) complete (2026-06-15)** — 3 plans. The repo-first Add-project UI on top of Phase 14's primitive: `AddProjectDialog` gains an integration-gated "GitHub repo | Local folder" segmented toggle (repo default, reusing `ui/tabs.tsx`); the `owner/name` input prefills the editable name, submit drives the atomic Phase-14 create with a blocking "Cloning…" spinner, and clone failures surface inline (dialog open, values preserved, no half-created project — Phase-14 atomicity). A dedicated best-effort `github.RepoDescription` (`gh repo view --json description`) captures the repo description into `projects.description` at create (visible/editable later in Project settings), leaving the shared `ValidateRepo` signature untouched. Integration-off renders the byte-for-byte pre-v1.4 folder-only form. 5/5 requirements verified (RPROJ-01..04, CKOUT-04); human-verify gate approved end-to-end; `go build`/`vet`/`test ./...` + `tsc` all green; embedded SPA rebuilt. **v1.4 (Repo-First Projects) shipped 2026-06-15** — milestone audit passed (10/10 requirements, 6/6 integration seams, 5/5 E2E flows), archived to `milestones/v1.4-*`.

**v1.5 Sharper Review Column — Phase 16 complete (2026-06-17), milestone fully implemented** — 2 plans, 2 waves (data → rendering). The Review column now carries two independent at-a-glance signals per PR. Backend (`internal/github` + `internal/api`): a second `reviewed-by:@me draft:false` search rides ONE cache cycle alongside the existing `user-review-requested:@me` call (`fetchLists`, degrade-together), server-side top-precedence `dedupeReviewed`, the `{prs, reviewed}` endpoint response, and `pr_number`+`source` joined onto `/api/agents/status` entries (no migration — columns pre-exist in 00007). Frontend (`PRCard`/`ReviewColumn`): CI status as a bare lucide glyph (green `Check` / red `X` / static amber `Circle`; `none` renders nothing), agent state as an always-on colored **left rail** (green working / pulsing-amber waiting / blue idle / gray exited) — **redesigned from a `StatusDot` to a rail at the human-verify gate** per user feedback (a dot beside the line-art glyph clashed), so the two signals split cleanly by channel (left edge = your agent, right glyph = the PR's CI); and a quietly-omitted "Recently reviewed" subsection holding reviewed-still-open PRs until merge/close. 10/10 must-haves verified; human-verify approved end-to-end (incl. the redesign); `go test ./...` (12 pkgs) + `tsc -b && vite build` green; embedded SPA rebuilt. **v1.5 (Sharper Review Column) shipped 2026-06-17** — milestone audit passed (9/9 requirements, 4/4 integration seams, 3/3 E2E flows), archived to `milestones/v1.5-*`.

**v1.6 Global Active Sessions Bar — Phase 17 complete (2026-06-18), milestone fully implemented** — 2 plans, 2 waves (data → UI). The app now has a persistent global view of all live agent work. Backend (`internal/api/agents.go`): the existing `/api/agents/status` query was widened with `tasks.title` + `projects.name` via a JOIN on BOTH passes (manager-derived + DB-derived/post-restart), the `agentStatusEntry` struct + the TS `AgentStatusEntry` gained `taskTitle`/`projectName` — no new endpoint, no migration (the columns pre-exist; mirrors v1.5's `prNumber`/`source` widening). Frontend: `ActiveSessionsBar` (new, mounted in `AppLayout` outside `<Outlet/>`, present on every route) reuses the 5s `useAgentStatuses()` poll as a 4th consumer and `dotMeta()` for status colors — collapsed per-state counts + total with amber-pulsing waiting emphasis, an overlay panel that floats UP over content (chosen so the xterm terminals never reflow, D-03) listing live sessions attention-first with cross-project click-through and current-row highlight, `localStorage` default-collapsed persistence, and live-only filtering (exited drops off). The per-project sidebar waiting chips were kept (additive). 11/11 must-haves verified; human-verify gate approved end-to-end (incl. the critical no-terminal-reflow check); `go test ./...` (11 pkgs) + `tsc -b` + `vite build` green; embedded SPA rebuilt. **v1.6 fully implemented 2026-06-18** — run `/gsd:complete-milestone` to audit and archive.

## Next Milestone

**v1.6 Global Active Sessions Bar is now active** (scoped 2026-06-18 — see Current Milestone above). Candidates NOT pulled into v1.6 remain parked below for a future cycle.

Candidates carried forward live in **Deferred** below. Banked forward investments worth a future milestone:
- The v1.4 managed-checkout follow-ups, already scoped in `milestones/v1.4-REQUIREMENTS.md` "Future Requirements": on-demand checkout sync (CKMNT-01), non-default base branch at create (CKMNT-02), shallow/partial clone for large repos (CKMNT-03), and live clone-progress streaming + cancel (CKUX-01). The frontend `Project.managed` wire field is already in place to hang a managed-delete cleanup affordance on.
- Per-status task timestamps (`todo_at`/`in_progress_at`/`in_review_at`/`done_at`, migration 00006) ready to power board cycle-time / dwell-time stats.
- The background-goroutine reaper pattern (Done-TTL + PR reconcile passes) that future periodic maintenance (e.g. MAINT-01 stale-worktree purge) can model on.
- Deferred v1.3 GitHub follow-ups already scoped in the archived `milestones/v1.3-REQUIREMENTS.md` "Future Requirements": richer PR cards (diff size, fork pill, head→base line, review-decision/labels — GHCARD-01..04), filter options (team review requests, draft PRs — GHFILT-01/02), and broader surfaces (cross-project review inbox, author-side PRs — GHWIDE-01/02).

**Known tech debt (from the v1.3 audit, non-blocking):** ~18–20 pre-existing `react-hooks` eslint errors plus two v1.3-introduced lint advisories (`Date.now()` in render in `PRCard.tsx`/`ReviewColumn.tsx`) — gating build (`tsc -b && vite build`) is green; a dedicated lint-cleanup pass is the right home.

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
| PR Review column reads `user-review-requested:@me draft:false` via the `gh` list call, with `statusCheckRollup` riding the same call (no N+1) and degraded states classified by exit-code+stderr (v1.3, Phase 11) | Pure read path stands up the whole `gh` integration at lowest risk; degrade-don't-break on every failure mode; rate-limit-bounded by one cached call | ✓ Good — always-200 endpoint, per-repo cache, never blocks the board |
| Review column **defaults to collapsed** per-project (v1.3, Phase 11, supersedes D-05/D-06) | User-directed change during the human-verify checkpoint — the review queue is secondary to the task board, so it stays out of the way until opened | ✓ Intentional — `localStorage` absent key reads as collapsed; explicit expand persists |
| PR review worktree checks out the PR's **real head branch** (named, `pr/<n>` fallback on collision), not detached (v1.3, Phase 12, supersedes D-01) | User-directed during the Phase 12 human-verify — a detached HEAD reads as "not checked out"; a named branch matches GHREV-01's wording. Collision fallback keeps GHREV-05 safety (never reuse/move an existing ref; main checkout HEAD unchanged) | ✓ Good — `worktree add -b`, collision-checked; fork same-name `master` lands on `pr/<n>` |
| Configurable PR-review seed prompt via a global Settings field (v1.3, Phase 12) | User-requested during human-verify — was deferred at discuss-time, pulled in once the feature was live | ✓ Good — `pr_review_seed` KV key (default template, `<n>`/`<title>` placeholders), prefilled once per agent session, never auto-sent |
| PR worktrees auto-removed on merge/close, but only when pristine+idle; branch always kept (v1.3, Phase 13, GHCLN-01/02) | Closes the loop without ever bulldozing work — the reaper detects merge/close server-side; any dirty/unpushed/stashed/busy worktree is left for manual cleanup | ✓ Good — second reaper pass + shared `CleanupWorktreeGated` (force=false); auto deletes the row, manual keeps it nulled |
| Agent state on PR cards is a colored **left rail**, not a `StatusDot` dot (v1.5, Phase 16, supersedes the planned SIGNL-01/02 dot) | User-directed at the human-verify gate: a filled dot beside the line-art CI glyph clashed (different styles, misaligned). The rail carries the full state by color and splits the two signals onto separate channels (left edge = your agent, right glyph = the PR's CI) | ✓ Good — `agentRail()` 3px span; green working / pulsing-amber waiting / blue idle / gray exited; no dot, zero layout shift |
| Two PR lists from one cached `gh` cycle: `user-review-requested:@me` (awaiting) + `reviewed-by:@me` (recently reviewed), server-side top-precedence deduped (v1.5, Phase 16, REVWD-01/03/04) | Keeps reviewed-but-unmerged PRs in view without a second poll or N+1; `--state open` bounds the list (drops on merge/close) so no retention bookkeeping; one classified state/stale degrades both together | ✓ Good — `fetchLists`/`dedupeReviewed` in the existing `Service`; both lists ride one `repoEntry` |

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
*Last updated: 2026-06-18 after completing Phase 17 — v1.6 Global Active Sessions Bar fully implemented*
