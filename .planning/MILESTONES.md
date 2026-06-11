# Milestones

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
