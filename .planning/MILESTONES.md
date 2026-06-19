# Milestones

## v1.7 Project Icons in Collapsed Sidebar (Shipped: 2026-06-19)

**Phases completed:** 2 phases (18–19), 6 plans, 12 tasks

**Delivered:** Every project now has a colored monogram avatar (two uppercase letters on a curated-palette background) that makes projects identifiable and switchable directly from the collapsed sidebar — which previously slid fully off-screen, leaving no project reference. The avatar renders both as a clickable collapsed-rail icon (active highlight, name tooltip, amber waiting badge) and inline beside the project name when expanded; letters and color auto-derive at creation (name initials + random palette pick) and are editable from Project settings. Backend was minimal — two new `projects` columns with migration/backfill and the PATCH path to edit them — the rest was frontend.

**Key accomplishments:**

- Backend data foundation (Phase 18): new `internal/api/icons.go` as the single source of truth — `projectPalette`, `deriveLetters` (name → ≤2 uppercase monogram), `pickColor` (random palette member), and server-side `validateIconLetters`/`validateIconColor`; migration 00009 adds `icon_letters` + `icon_color` columns with an idempotent post-`Migrate` `BackfillProjectIcons` so every pre-existing project gets non-blank letters + a stable color; both create paths auto-assign at INSERT and the partial-PATCH handler validates+persists both (ICON-01..04).
- Shared `<ProjectAvatar>` monogram primitive (rail|inline sizes, rail-only active state + static amber waiting dot) plus the `PROJECT_PALETTE` TS const mirroring the Go `projectPalette` byte-for-byte — the Wave-1 dependency root every Phase 19 surface consumes.
- The collapsed sidebar is now a 3rem icon rail of per-project circular monogram avatars (filled-row active highlight, side=right name tooltip, static amber waiting badge); the same avatar appears inline beside the name when expanded; the floating re-expand trigger is retired (ICON-05..10).
- Project settings gained a live `<ProjectAvatar>` preview, an "Initials" input (client-normalized to ≤2 uppercase alphanumerics, mirroring the server rule), and a 9-swatch `PROJECT_PALETTE` color grid with the current color ring+check-marked — wired into the existing conditional PATCH so `icon_letters`/`icon_color` are sent only when changed, with saved edits propagating to the rail + expanded sidebar (ICON-11/12).

**UAT revisions shipped as the new contract:** circular avatars (reversed the planned `rounded-md` square, D-04); rail-row filled-highlight active state instead of a ring (reversed D-06); and a **muted desaturated palette** replacing the bright Tailwind-600 hues — changing both the Go source of truth and the TS mirror, plus a follow-up migration `00010_muted_palette.sql` that remapped existing rows by palette position.

**Known deferred items at close:** 4 completed quick tasks (`260613-osu`, `260613-ph5`, `260616-8l7`, `260618-mlu`) from earlier milestones (v1.3/v1.5/v1.6) lingered in `.planning/quick/` and were flagged by the pre-close artifact audit; each has a `SUMMARY.md` with a completion date — verified done, not gaps (audit metadata false-positive).

---

## v1.6 Global Active Sessions Bar (Shipped: 2026-06-18)

**Phases completed:** 1 phases, 2 plans, 5 tasks

**Key accomplishments:**

- The agent-status feed now carries `taskTitle` + `projectName` on every entry (both the manager-derived and post-restart passes) via a `JOIN projects`, and the TS `AgentStatusEntry` type exposes them — the sole backend change for the Global Active Sessions Bar (SBAR-10), with no new endpoint and no new migration.
- A persistent, collapsible bottom bar (`ActiveSessionsBar.tsx`) mounted globally in `AppLayout` shows every LIVE Claude agent session across all projects — collapsed it renders per-state counts (working/waiting/idle) + total with the waiting count amber-pulsing only when > 0; expanded it floats a panel UP over content (a fixed overlay that never reflows the xterm terminals) listing live sessions attention-first (waiting → working → idle), each row click-through to that task's agent view including cross-project, with collapse state persisted in localStorage and ~5s freshness off the existing poll. Covers SBAR-01..SBAR-09.

---

## v1.5 Sharper Review Column (Shipped: 2026-06-17)

**Phases completed:** 1 phases, 2 plans, 6 tasks

**Delivered:** The Review column now carries two independent at-a-glance signals per PR — your agent's session state (a colored left rail: green working / pulsing-amber waiting / blue idle / gray exited) and the PR's CI state (a bare glyph: green check / red cross / static amber circle; no-checks renders nothing) — split onto separate visual channels so they never confuse, plus a "Recently reviewed" section that keeps PRs you've reviewed (`reviewed-by:@me`, approve or request-changes) visible until they merge or close. Audit: 9/9 requirements, 4/4 integration seams, 3/3 E2E flows. Milestone-time redesign: the originally-planned agent dot was replaced by the left rail at the human-verify gate (a dot beside the CI glyph clashed).

**Key accomplishments:**

- Second `reviewed-by:@me draft:false` gh search riding the existing per-repo TTL cache, a `PRLists` combined runner with server-side top-precedence dedup, a widened `{prs, reviewed, state, stale, fetchedAt}` Result, and `/api/agents/status` entries carrying `prNumber`/`source` — plus the matching frontend wire types — with zero visual changes.
- The Review column's two at-a-glance signals split onto separate channels: your agent's state as a 3px colored LEFT RAIL (working green / waiting amber-pulse / idle blue / exited gray) and the PR's CI as a bare lucide glyph (green Check / red X / static amber Circle, `none` renders nothing) — plus a quiet-omitted "Recently reviewed" subsection reusing the same PRCard, all server-deduped against the awaiting list.

---

## v1.4 Repo-First Projects (Shipped: 2026-06-15)

**Phases completed:** 2 phases, 7 plans, 13 tasks

**Delivered:** When GitHub integration is on, add a project by naming a GitHub repo — Kangent `gh repo clone`s and manages the checkout under `~/.kangent/repos/<owner>/<name>` on the default branch — with the folder path as the optional fallback (folder-only when GitHub is off). Task worktrees branch off the managed checkout; the clone is gated-removed on project delete; clone failures degrade-don't-break with no half-created project; existing folder-based projects are untouched. Audit: 10/10 requirements, 6/6 integration seams, 5/5 E2E flows.

**Key accomplishments:**

- **Managed Checkout Foundations (Phase 14):** migration 00008 adds the `managed` marker (existing folder rows backfill to never-touch); `github.Clone` wraps `gh repo clone` (exit-0-only, remove-on-failure, faked-runner test seam); a second `POST /api/projects` path gh-validates `owner/name` (RPROJ-05), clones into `~/.kangent/repos/<owner>/<name>`, and INSERTs `managed=1` + `github_repo` ONLY after exit 0 (atomic — no orphan row/partial dir), with origin-matched reattach (refuse-without-clobber on mismatch).
- **Fresh worktrees off the managed clone (Phase 14):** managed task/PR-review worktrees best-effort `git fetch origin <default>` before `ResolveBase` so new work starts from the freshest tip — gated on the `managed` marker so folder projects keep their no-network guarantee, and a failed fetch is discarded so it never blocks task creation.
- **All-or-nothing gated managed delete (Phase 14):** a two-pass delete gates every task/PR worktree AND the clone root on dirty/unpushed (`origin/<default>..HEAD`, no fetch)/stash/running-session, refuses with a 409 `{reasons:[…]}` list removing nothing, and on all-clear tears down linked worktrees → `os.RemoveAll` the clone → deletes the rows; folder (`managed=0`) delete stays byte-for-byte unchanged (never touches the dir).
- **Repo description auto-capture (Phase 15):** repo-first create auto-captures the GitHub repo description via a best-effort `gh repo view --json description` read and persists it into `projects.description` (degrade-don't-break, never blocks; `ValidateRepo`'s shared signature untouched).
- **Repo-first Add-project UI (Phase 15):** integration-gated "GitHub repo | Local folder" segmented toggle (repo default, reusing `ui/tabs.tsx`); the `owner/name` input prefills the editable Name; submit drives the Phase-14 atomic create with a blocking "Cloning <owner/name>…" spinner; clone failures surface inline (dialog open, values preserved, no half-created project); folder mode byte-for-byte and the whole repo-first UI vanishes when integration is off. Human-verify gate approved end-to-end.

---

## v1.3 GitHub PR Review (Shipped: 2026-06-14)

**Phases completed:** 4 phases, 19 plans, 46 tasks

**Delivered:** Surface the GitHub PRs that need your review on a linked project's board and open each as a full task-like review workspace — a worktree on the PR's branch with the same agent/bash/diff tabs — then auto-clean the worktree when the PR merges/closes. All via the host's already-authenticated `gh` (no tokens stored), best-effort and degrade-don't-break throughout.

**Key accomplishments:**

- **GitHub Foundations (Phase 10):** migration 00007 lands all five v1.3 schema columns (`projects.description`/`github_repo`, `tasks.source`/`pr_number`/`pr_base_ref`) so Phases 11–13 need no further migration; the degrade-don't-break `internal/github` leaf (`ParseRepoRef`/`ValidateRepo`/`Available`); a gh-gated `/settings` integration toggle (OFF + un-enableable when `gh` is absent, via always-200 `GET /api/github/status`) with a full OFF cascade; and a Project settings dialog editing an origin-prefilled, soft-validated `owner/name` link + description.
- **PR Review Column (Phase 11):** a self-gating, collapsible per-project "Review" column listing `user-review-requested:@me draft:false` open PRs via ONE cached `gh pr list` call (server-side `statusCheckRollup` → pass/fail/pending/none, no N+1), behind an always-200 `GET /api/projects/{id}/pull-requests` endpoint; 60s visibility-paused auto-poll + manual refresh, inline loading/empty/degraded states, default-collapsed, appended outside the dnd machinery so PR cards never enter the kanban.
- **Open-a-Review (Phase 12) — the milestone headline:** clicking a PR card find-or-creates a `source='github_pr'` task rendered through the same TaskPage/agent/bash/diff shell, backed by a worktree on the PR's REAL head branch (`fetch refs/pull/<n>/head` + `worktree add -b`, `pr/<n>` collision fallback — never `gh pr checkout`); reopening reattaches (no duplicates); the diff computes against the PR's own base merge-base (matches GitHub Files-changed); a 5-query `source='manual'` board-leak guard + `/move` 409 keeps PR reviews off the board; fork/colliding-branch PRs open with the primary checkout HEAD provably unchanged (GHREV-05).
- **PR Worktree Auto-Cleanup (Phase 13):** the Phase-9 reaper gains a second `reconcilePRsOnce` pass that reads each PR's state (`gh pr view --json state`) and gated-removes a merged/closed worktree ONLY when pristine + idle (dirty / unpushed via `rev-list FETCH_HEAD..HEAD` / stash / running session each skip), always keeping the branch; the gated logic was extracted byte-equivalent into a shared `CleanupWorktreeGated` (HTTP DELETE + reaper, `force=false`); manual cleanup re-adds a PR `⋯` "Clean up worktree" item + a merged/closed banner.
- **Cross-phase integrity:** one shared `github.Service` flows through all three downstream phases (list cache, PR detail/checkout, reaper PRState); audit confirmed 20/20 requirements satisfied, 6/6 integration seams wired, 4/4 E2E flows complete. Every blocking human-verify checkpoint (11/12/13) was approved by the user.

---

## v1.2 Quota & Resumable Shells (Shipped: 2026-06-13)

**Phases completed:** 3 phases, 12 plans, 29 tasks

**Key accomplishments:**

- Demand-driven token-keyed quota cache (`internal/quota`) proxying Anthropic's OAuth usage endpoint at always-200 `GET /api/usage`, with the full six-state degradation matrix proven by 20 unit tests and zero new dependencies
- "Claude 5h" trigger with width=5h/color=max-of-all threshold bar, server-driven hover popup with reset countdowns and manual refresh, mounted in both page headers polling /api/usage every 60s while visible — zero new npm dependencies
- Phase 7 verified end-to-end — whole-repo green, token-leak audit clean, live /api/usage 200 — plus three user-feedback polish passes on the quota popup (live ticking footer, compact icon-free reset column, equal-width comparable bars), human-approved
- Socket-isolated tmux Client (new-session -A / has-session / kill-session with =name exact match, 5s exec timeouts, idempotent kill) plus the identity-only tmux_sessions table — all later tmux work calls through this one package
- settings.AllowedShells is now a call-time LookPath function: the shell dropdown offers "tmux" only while the binary resolves on PATH, and save-time validation tracks the exact same truth — zero frontend changes
- tmux threaded through the session package as a spawn-time lifecycle property: SpawnOpts.TmuxName spawns `tmux new-session -A` under the existing PTY pipeline, Stop is killer-first (kill-session, signal fallback), and waitExit discriminates exit-vs-detach via has-session — with bash/agent stop paths proven byte-identical
- End-to-end invisible tmux: POST /api/sessions with shell=tmux mints and persists `kangent-<task>-<n>` from the tmux_sessions table, attaches it under creack/pty on the dedicated `-L kangent` socket, and surfaces honest 409 spawn errors verbatim in the tab header — with zero tmux markers reaching the UI or the wire (D-77).
- `done_session_ttl` global setting (Go-style duration, default 24h; empty/0/never disable) with a shared `ParseDoneSessionTTL` helper that is the single source of truth for both save-time validation and the 09-05 reaper, plus a Cleanup field on the settings page.
- A background ticker goroutine that kills bash + tmux + agent sessions of tasks left in Done past `done_session_ttl` (clocked from `done_at`), keeping every agent resumable and never touching worktrees — the codebase's first background goroutine.

---

## v1.1 Settings & Polish (Shipped: 2026-06-11)

**Phases completed:** 1 phases, 4 plans, 10 tasks

**Key accomplishments:**

- SQLite-backed settings KV store (migration 00004) with code defaults, per-key validation carrying the UI-SPEC canonical error copy, branch-template expansion + git ref validation, quote-aware extra-params tokenizer, and the GET/PUT REST surface
- All four settings wired into their v1.0 call sites: tokenized claude extra-params append to every agent spawn (fresh + resume), bash tabs run the LookPath-resolved settings shell, and provisionWorktree builds template-driven branches under the settings worktree base with a create-time ref-format defense — all read-at-use, managers DB-free.
- Full /settings page with per-field commit (blur/Enter, Esc revert, 2s Saved flash, Reset to default, verbatim inline server errors), sidebar gear with active state, API-driven shell select, and the UI-01 full-width task header
- Phase 6 gate passed: release binary built clean with the settings page embedded, full Go suite green across 8 packages, 8/8 verbatim copy audit, restart-persistence smoke proven, and all seven v1.1 success criteria approved live by the user.

---

## v1.0 MVP (Shipped: 2026-06-11)

**Phases completed:** 5 phases, 28 plans, 74 tasks

**Key accomplishments:**

- SQLite store with WAL/foreign_keys/MaxOpenConns(1) discipline, embedded goose migrations creating projects/tasks schema, and a runnable server binary serving /api/healthz at 127.0.0.1:7333
- All 10 JSON endpoints (projects CRUD with git rev-parse path validation, tasks CRUD, move) with server-computed fractional ordering proven stable under a 200-insert renormalization stress test
- Vite + React 19 + Tailwind 4 SPA with dark-only zinc shadcn theme, full typed TanStack Query API layer (10 hooks incl. optimistic move), and the 3-route shell feature plans build against
- Collapsible projects sidebar with add/rename/delete flows: mono-path Add dialog mirroring server validation errors inline, AlertDialog delete with repo-untouched copy, and localStorage-persisted collapse
- dnd-kit kanban board with four fixed columns, optimistic drag persistence through the move endpoint, click-to-open cards, and dual task creation (quick-add row + 560px dialog with n shortcut)
- Deep-linkable full-page task view with the TabDef[]-driven tab strip seam, GFM markdown edit/preview description, inline title editing, and confirmed hard delete
- One binary serves the full kanban app: //go:embed'd Vite build with SPA-fallback routing, Makefile pipeline, and a scripted kill/restart persistence proof — human-approved end to end
- Server-side bash PTY session engine with 1 MiB ring-buffer replay, atomic attach/detach, and session-wide SIGTERM→5s→SIGKILL teardown proven to leave zero orphaned subprocesses
- xterm 6 set pinned exactly, Vite /api proxy made WebSocket-capable (ws: true), UI-SPEC zinc theme encoded as xtermTheme.ts, and typed TanStack Query session hooks (list/spawn/stop/delete) built against the fixed REST contract
- ttyd-style binary WebSocket bridge (input/output/replay/resize-jiggle/exit frames) over coder/websocket, session REST endpoints, and the full TERM-07 model: loopback-bind refusal, Host middleware, exact-origin allowlist with --dev-origin escape hatch — all proven headlessly with httptest + WS dials
- useTerminalSocket hook implementing the locked 0x30/0x31/0x78 wire protocol with 0.5–8s ×5 backoff and 4404 short-circuit, plus a self-contained TerminalPane with WebGL/DOM xterm rendering, debounced fit/resize, D-17 clipboard, and the full UI-SPEC banner contract
- /terminal dev route composing a 220px session rail with attach-by-click TerminalPane remounts, plus a Go integration test driving spawn→echo→detach→replay-reattach→stop→delete through the production mux — all five phase success criteria human-verified
- `internal/worktree` git-CLI service: ref-safe slugs, no-network default-branch resolution, leak-safe worktree+branch creation, -uall dirty counts, and branch-preserving removal — all tested against real git 2.43 repos in t.TempDir()
- Spawn(SpawnOpts{Cwd, TaskID}) with pre-PTY cwd validation, per-task monotonic "Bash N" labels, ListByTask filtering, and concurrent StopAllForTask — the engine half of TERM-04
- Migration 00002 + task-create worktree provisioning (201 always, D-25) + POST/GET/DELETE /api/tasks/{id}/worktree with server-enforced stop/force gates and branch-preserving cleanup + task-scoped session spawn/filter — the whole phase is now exercisable with curl
- Task view is now the working surface: typed worktree/session API hooks, the branch/failed/absent meta line with Retry/Create, and live bash tabs (spawn via +, mirror server sessions, x-to-stop with left-neighbor activation) mounted through a controlled TaskTabs — verified against the real server end-to-end at the API level
- The GIT-02/GIT-03 user surface: one CleanupWorktreeDialog composing four variants from state fetched at open (clean / N-sessions-stopped / dirty type-to-confirm / both), wired to fire after every persisted move into Done and from the task-view ellipsis menu — never a silent kill, branch always kept
- Six-stage lifecycle integration test over the real REST surface (provision → task-scoped session cwd proof → dirty/session state → composed 409 gates → branch-keeping forced cleanup → kept-branch reuse), full build gates green, and human approval of all four Phase 3 success criteria against the built binary
- Agent session kind spawning the real claude CLI (--session-id + inline --settings hook overlay, inherit-all env) plus the D-47 working/idle/waiting state machine with settle-gated activity and hooks-dead BEL fallback, all TDD-driven against a fake-claude stub
- Full agent backend over REST and WS: kind-discriminated agent spawn with one-per-task 409 and claude_session_id persistence, a constant-time token-gated hook receiver driving the D-47 state machine, the 5s-pollable GET /api/agents/status board source, delete-stops-sessions, and D-45 attach-clears-waiting — all TDD against fake-claude stubs
- Live agent status on the kanban: 5s-polled useAgentStatuses hook, shared StatusDot with the exact UI-SPEC palette (pulsing amber waiting, gray-not-red stop exits), D-44 waiting card border, and D-49 sidebar waiting-count chips
- Permanent first Agent tab with Start/Start-again fresh-spawn lifecycle, Insert-description bracketed paste through a TerminalPane onReady handle, kind-aware session API, and the tab-label StatusDot with D-45 optimistic waiting clear
- 8-stage agent-lifecycle integration test against a fake-claude stub plus human-approved live verification of real claude v2.1.170, closing Phase 4 with one checkpoint feedback cycle (dimmed exited terminal, code-free banner, Reset session)
- `claude --resume <persisted uuid>` as a body-flag variant of the existing agent spawn endpoint, plus a transcript-glob `resumable` flag and DB-derived post-restart exited entries on `/api/agents/status` — RCVR-01 reconciled by architecture with zero schema change.
- REVW-01 backend: GET /api/tasks/{id}/diff returns everything the task changed vs the merge-base of its base branch (committed + staged + unstaged + untracked-as-additions, base movement excluded) as structured per-file-hunk JSON, parsed from git's -z/unified machine output with rename/binary/no-newline/mode-only edges handled.
- The D-57 muted-gray post-restart dot and the D-54 Reset/Resume button pair in both placements (resumable pre-start after a restart, exited banner within a run), driven entirely by the server's `resumable` flag from plan 05-01 — pure wiring, zero new dependencies.
- REVW-01 frontend: a read-only Diff tab (third, after Agent/Description, D-64) that renders plan 05-02's structured per-file-hunk JSON as collapsible unified diffs with a scoped green/red content palette, a totals bar with manual-refresh (no polling), >400-line collapse + binary header-only handling, and verbatim empty/loading/error states — disabled with an explanation when the task has no worktree.
- One deterministic 7-stage integration test (TestRecoveryLifecycle) locks in restart reconciliation, the full resume lifecycle, and the wired diff path against fake-claude; full build gates pass; and a human verified the entire v1 experience end-to-end against real claude v2.1.173 — the v1 milestone gate.

---
