# Milestone v1.0 — Project Summary

**Generated:** 2026-06-11
**Purpose:** Team onboarding and project review

---

## 1. Project Overview

**Kangent** is a local-only web app for organizing Claude Code agent sessions around projects and tasks. A left sidebar lists projects (each pointing at a local git repo); the main area is a kanban board. Clicking a task opens a full-page view with the agent session (the real `claude` CLI running in a server-side PTY, rendered in a browser terminal) plus optional bash tabs and a read-only diff tab.

**Core value:** one place to see and drive all agent work — every task gets its own isolated git worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.

**Shape:** single user, runs locally, accessed at `http://127.0.0.1:7333`. Not multi-user, no auth, no remote deployment, no external trackers (JIRA/Linear) — local SQLite only, by design. Inspired by SlayZone's card→terminal→worktree model and vibe-kanban's web-app form, but uniquely combining a *real* PTY (full TUI fidelity: plan mode, slash commands, permission prompts) with a localhost web app.

**Status:** v1.0 MVP **shipped 2026-06-11**. All 5 phases complete, all 25 requirements validated, every phase human-verified.

## 2. Architecture & Technical Decisions

A single Go binary serves a REST API + an embedded React SPA and owns all PTY processes. The browser is a detachable view.

**Backend (Go, ~10,400 LOC)**
- **HTTP:** stdlib `net/http` ServeMux (Go 1.22+ method/wildcard routing) — no framework. Routes under `/api/*`; everything else falls through to the embedded SPA with `index.html` fallback.
- **Storage:** `modernc.org/sqlite` (pure-Go, CGO-free) with WAL, `busy_timeout`, `foreign_keys`, `MaxOpenConns(1)`; goose embedded migrations (00001 schema → 00002 worktree columns → 00003 `claude_session_id`).
- **Sessions:** in-memory `SessionManager` owns PTY lifetimes (never the WebSocket handler). One read goroutine per session fans out to a 1 MiB ring buffer + attached clients. `creack/pty` v1 for the PTY; `coder/websocket` for the bridge (concurrent-write safe).
- **Terminal wire protocol:** ttyd-style binary frames, 1-byte type prefix (`0x30` io, `0x31` resize, `0x78` exit), close code 4404 = session-not-found.
- **Git:** shell out to the `git` CLI via `os/exec` with `--porcelain`/`-z` machine output (go-git can't do linked worktrees). `internal/worktree` and `internal/diff` packages.
- **Single binary:** `//go:embed all:dist` of the Vite build; `make build` runs `npm build` then `go build`.

**Frontend (React 19 + TypeScript, ~5,800 LOC)**
- Vite + Tailwind 4 (CSS-first, no config file) + shadcn/ui (zinc, **dark-only**).
- TanStack Query for all server state (5s polling for session/agent status); dnd-kit for the board; xterm.js 6 (WebGL renderer + DOM fallback) for terminals; react-router 7; react-markdown 10.

**Key decisions (with rationale and the phase that made them):**
- **Real `claude` CLI in a PTY, not an Agent-SDK chat UI** — full interactive fidelity with zero reimplementation; verified claude doesn't even use the alt-screen buffer, so reattach is clean. *(Phases 1, 4)*
- **Session manager owns PTY lifetime, WebSocket only attaches/detaches** — this single rule delivers the "close tab, reattach later" core value. *(Phase 2)*
- **Worktree + branch auto-created per task** (`task/<slug>-<id>` under `~/.kangent/worktrees/<project>/`), branched from the default-branch tip — isolates concurrent agents. *(Phase 3)*
- **UUID-first session identity** — Kangent mints the UUID and spawns `claude --session-id <uuid>`, stores it in SQLite, and resumes with `claude --resume <uuid>`; never parses claude output. *(Phases 4, 5)*
- **Invisible, additive hook injection** — agent sessions spawn with an inline `--settings` overlay (Notification→waiting, Stop→idle) that merges with the user's own hooks and leaves nothing on disk; sessions started outside Kangent are untouched. *(Phase 4)*
- **Loopback-only security** — server refuses non-loopback `--addr`; strict Origin/Host allowlist on every request and WS upgrade (blocks CSWSH + DNS rebinding). No token needed for single-user. *(Phase 2)*
- **The `TabDef[]` tab-strip seam** — Phase 1 shipped a data-driven tab strip with just Description; Agent, Bash, and Diff tabs plugged into the same array across later phases with zero rework. *(Phase 1)*
- **Reserved color budget** — neutral palette through Phases 1–3 so status dots (amber/green/gray, Phase 4) and diff coloring (green/red, Phase 5) own their meaning.
- **Manual git workflow** — the app only creates/cleans worktrees; merging/PRs are the user's job in a bash tab. *(Phase 3)*

## 3. Phases Delivered

| Phase | Name | Status | One-Liner |
|-------|------|--------|-----------|
| 1 | Foundation — Projects & Board | ✅ passed | Single Go binary + embedded React SPA + SQLite; projects sidebar, four-column kanban board with drag-and-drop, task CRUD, full-page task view with the extensible tab strip |
| 2 | Terminal Engine | ✅ passed | Server-owned bash PTY sessions with 1 MiB ring-buffer replay, binary WebSocket bridge, xterm.js terminal with detach/reattach, zero-orphan teardown, loopback-only security |
| 3 | Worktree Isolation & Bash Tabs | ✅ passed | Auto worktree+branch per task, bash tabs running inside the worktree, and a four-variant cleanup dialog with dirty-tree/running-session gates that always keeps the branch |
| 4 | Claude Code Agent Sessions | ✅ passed | Start button spawns real `claude` in the worktree PTY with invisible status hooks; live status dots on cards/tab/sidebar; dimmed exited state with Reset session |
| 5 | Recovery & Review | ✅ passed | Restart reconciliation (no ghosts, zero schema change), `claude --resume <uuid>` recovery in both placements, and a read-only merge-base diff tab |

Every phase ended in a blocking human-verify checkpoint against the running app; the Phase 5 checkpoint walked the full v1 experience end-to-end.

## 4. Requirements Coverage

All 25 v1 requirements validated (full archive: `.planning/milestones/v1.0-REQUIREMENTS.md`):

- ✅ **Projects (PROJ-01..03):** create/list/rename/delete projects on local git repos, path-validated
- ✅ **Tasks & Board (TASK-01..05):** title + markdown description + status, CRUD, four fixed columns, drag-and-drop with persisted manual order, full-page task view
- ✅ **Git Worktrees (GIT-01..03):** auto worktree+branch per task, confirmed cleanup keeping the branch, dirty/running-session gates
- ✅ **Terminal (TERM-01..07):** Start button → claude in worktree PTY, interactive browser terminal, resize/scrollback/copy-paste, bash tabs, server-side persistence + reattach, full-tree stop, Origin/Host validation
- ✅ **Status (STAT-01..02):** live status dots, hooks-at-spawn detection + bell fallback
- ✅ **Recovery (RCVR-01..02):** startup reconciliation, `claude --resume`
- ✅ **Review (REVW-01):** read-only diff tab
- ✅ **Storage (STOR-01..02):** SQLite persistence, single localhost binary

No milestone audit was run (the per-phase human checkpoints + the final end-to-end walkthrough covered the integration surface); proceeded by explicit user decision.

## 5. Key Decisions Log

67 numbered decisions (D-01..D-67) were captured across the phase CONTEXT.md files and referenced by ID in plan actions. Highlights beyond §2:

- **D-05/D-06:** full-page task route (not modal), single tab strip with Description as a tab — the seam for everything later
- **D-09:** manual drag ordering via server-computed fractional positions with renormalization
- **D-14:** stop = SIGTERM → 5s → SIGKILL on the whole process group (zero orphans)
- **D-22/D-23:** branch `task/<slug>-<id>`, worktrees centralized under `~/.kangent/worktrees/`
- **D-25/D-26:** worktree-creation failure never blocks task creation (warning state + Retry); pre-existing tasks get lazy creation
- **D-28/D-29:** tabs mirror live server sessions; closing a tab stops the session (tab = session)
- **D-33:** dirty-worktree cleanup requires type-to-confirm; branch always kept
- **D-43..D-47:** dot-only status badges; waiting pulses + card border; clears on attach; Stop hook → idle; hooks + output-activity detection
- **D-54..D-56:** Resume/Reset pair in both the exited banner and post-restart pre-start; honest resume-failure (claude's error shown, no silent fallback)
- **D-59/D-62:** diff = everything vs merge-base (committed + uncommitted + untracked-as-additions), >400-line files collapsed, binaries stat-only

Full per-phase decision records live in `.planning/phases/*/0X-CONTEXT.md`.

## 6. Tech Debt & Deferred Items

**Carried bug (non-blocking, tracked in PROJECT.md + STATE.md):**
- Whether plan-mode exit-plan approval triggers the amber "waiting" dot was never empirically confirmed at the final checkpoint (research OQ1). Fix if needed = widen the Notification hook matcher. Not a regression.

**Deferred to v1.1+ (parked, not built):**
- Browser notifications on waiting/finished (NOTF-01) — detection already built, so cheap to add
- One-click "Move to In Review?" suggestion on the Stop hook (NOTF-02) — deliberately never auto-moves cards
- Stale-worktree purge list (MAINT-01)
- MCP server so agents update their own board state (AGNT-01)
- Multiple agent CLIs (Codex, Gemini) — the PTY abstraction makes this plumbing, not architecture
- Per-instance auth token (only if shared-machine support ever matters)

**Process lessons (from RETROSPECTIVE.md):**
- Pure-HTTP smoke tests gave false confidence — a blank-page Tooltip crash passed the Phase 1 smoke test because it only checked HTTP responses. UI phases need a real browser check (now standard via headless-Chrome pre-checks).
- Stray verification servers twice blocked the user's port — checkpoint executors must tear down their own processes.
- Parallel-executor commit interleaving occasionally raced (an `--amend` collision, ambiguous `go.mod` ownership) — recovered each time; tighter `files_modified` ownership prevents it.

**Implementation notes:**
- Sessions are in-memory only; persistence is task-level (`claude_session_id` on tasks). RCVR-01 needs no migration — nothing durable records "running", so ghosts are architecturally impossible.
- `web/dist/index.html` is a committed placeholder; the real Vite build output is never committed (regenerated by `make build`).

## 7. Getting Started

**Run it:**
```bash
make build        # npm build → embed → go build → bin/kangent
./bin/kangent     # serves http://127.0.0.1:7333
```
Flags: `--addr` (default `127.0.0.1:7333`, refuses non-loopback), `--db` (default `~/.kangent/kangent.db`). Requires the `claude` CLI installed on the host (v2.1.17x verified) and `git`.

**Dev mode:** run the Go server + `cd web && npm run dev` (Vite proxies `/api` and WebSockets to the backend).

**Tests:**
```bash
go test ./internal/...        # 6 packages: api, diff, session, store, worktree, ws
cd web && npm run build       # tsc + vite (the frontend gate)
```
Agent-session tests use a fake-claude stub (`internal/api/testdata/fake-claude`) — no API quota, deterministic.

**Key directories:**
- `cmd/kangent/main.go` — entry point, mux wiring, loopback enforcement, SPA fallback
- `internal/store/` — SQLite + goose migrations
- `internal/session/` — the PTY session manager (the heart of the system) + agent spawn/hook overlay
- `internal/ws/` — WebSocket terminal bridge + wire protocol
- `internal/worktree/`, `internal/diff/` — git CLI services
- `internal/api/` — REST handlers (projects, tasks, sessions, agents, worktrees, diffs, hooks)
- `web/src/components/` — `board/`, `task/` (incl. `TaskTabs`, `AgentTab`, `DiffTab`), `terminal/` (`TerminalPane`, `useTerminalSocket`), `sidebar/`
- `web/src/api/` — typed client + TanStack hooks

**Where to look first:** `internal/session/manager.go` (PTY ownership + spawn) and `web/src/components/terminal/TerminalPane.tsx` (the attachable terminal) — together they are the architectural spine everything else hangs off. Then `.planning/PROJECT.md` for the why.

---

## Stats

- **Timeline:** 2026-06-10 → 2026-06-11 (2 days)
- **Phases:** 5 / 5 complete
- **Plans:** 28 | **Tasks:** 74
- **Commits:** 183 (tag `v1.0`)
- **Code:** ~10,400 LOC Go + ~5,800 LOC TypeScript/React (hand-authored)
- **Verification:** 5/5 phases passed; every phase human-verified against the running app
