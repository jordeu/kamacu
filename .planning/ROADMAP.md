# Roadmap: Kangent

## Milestones

- ✅ **v1.0 MVP** — Phases 1–5 (shipped 2026-06-11) — see [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md)
- ✅ **v1.1 Settings & Polish** — Phase 6 (shipped 2026-06-11) — see [milestones/v1.1-ROADMAP.md](milestones/v1.1-ROADMAP.md)
- 🚧 **v1.2 Quota & Resumable Shells** — Phases 7–9 (in progress) — Claude quota at a glance before starting agents, plus tmux-backed bash tabs that survive tab closes and Kangent restarts

## Phases

<details>
<summary>✅ v1.0 MVP (Phases 1–5) — SHIPPED 2026-06-11</summary>

- [x] Phase 1: Foundation — Projects & Board (7/7 plans) — completed 2026-06-10
- [x] Phase 2: Terminal Engine (5/5 plans) — completed 2026-06-10
- [x] Phase 3: Worktree Isolation & Bash Tabs (6/6 plans) — completed 2026-06-10
- [x] Phase 4: Claude Code Agent Sessions (5/5 plans) — completed 2026-06-11
- [x] Phase 5: Recovery & Review (5/5 plans) — completed 2026-06-11

Full details: [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md)

</details>

<details>
<summary>✅ v1.1 Settings & Polish (Phase 6) — SHIPPED 2026-06-11</summary>

- [x] Phase 6: Settings & Polish (4/4 plans) — completed 2026-06-11

Full details: [milestones/v1.1-ROADMAP.md](milestones/v1.1-ROADMAP.md)

</details>

### 🚧 v1.2 Quota & Resumable Shells (in progress)

**Milestone Goal:** Make Kangent a better dispatcher cockpit: see Claude quota usage at a glance before starting agents, and make bash tabs durable via tmux-backed shells that detach on close and survive server restarts.

- [ ] **Phase 7: Claude Quota Indicator** - Compact threshold-colored quota bar in the board/task headers with an all-windows hover popup, backed by a cached, backoff-protected server proxy of the OAuth usage endpoint
- [ ] **Phase 8: tmux Shells — Spawn & Detach Lifecycle** - "tmux" shell option spawning attach-or-create sessions on a dedicated socket — invisible-tmux tabs with kill-on-close parity, exit-vs-detach discrimination, and a regression-guarded shared stop path
- [ ] **Phase 9: tmux Restart Resume & Cleanup Integration** - tmux sessions survive Kangent restarts with a Resume affordance, cleanup gates count and kill them so no shell is ever orphaned, and a Done-TTL reaper kills idle sessions on finished tasks

## Phase Details

### Phase 7: Claude Quota Indicator
**Goal**: User can see Claude quota usage at a glance — before starting agents — from anywhere in the app, with a trustworthy degrade-don't-break indicator
**Depends on**: Phase 6 (v1.1 complete). Fully independent of Phases 8–9 — touches zero session/PTY code
**Requirements**: QUOTA-01, QUOTA-02, QUOTA-03, QUOTA-04, QUOTA-05, QUOTA-06, QUOTA-07, QUOTA-08
**Success Criteria** (what must be TRUE):
  1. User sees a compact "Claude 5h" indicator with a threshold-colored usage bar (green <60 / yellow 60–84 / red ≥85) in the top-right of the main area, on both the board and the task view (QUOTA-01)
  2. Hovering the indicator opens a popup with one row per quota window the API returns (5h, 7d, per-model — server-driven, never hardcoded): colored bar, rounded percentage, and "Resets in Xh Ym" countdown; the footer shows "Updated Xm ago" with a manual refresh that bypasses the cache (QUOTA-02, QUOTA-03)
  3. Quota data auto-refreshes about every 60s while the browser tab is visible and never polls when hidden; the server enforces a 60s cache TTL, 10s hard floor, in-flight dedup, and 429 Retry-After backoff (min 30s), sending the load-bearing `claude-code/<version>` User-Agent — and never writes or refreshes the OAuth token (QUOTA-04, QUOTA-05)
  4. Failure states degrade gracefully: not-logged-in / 401 token-expired / 403 / 5xx / network each show a clear popup message with a warning on the trigger; stale cached data stays visible marked "error · Xm old" and is dropped after 3 consecutive failures; API-key-only users see a neutral/hidden indicator — never fabricated 0% bars (QUOTA-06)
  5. When any quota window is ≥85%, the compact trigger shows red-zone emphasis (the "stop starting agents" glance signal); switching Claude accounts never shows the previous account's data because the server cache is keyed by token (QUOTA-07, QUOTA-08)
**Plans**: 3 plans (2 waves)

Plans:
- [x] 07-01-PLAN.md — Backend: `internal/quota` service + `GET /api/usage` (six-state matrix, token-keyed cache, UA probe) [wave 1]
- [x] 07-02-PLAN.md — Frontend: QuotaIndicator trigger + hover popup, usage hooks, both header mounts [wave 1]
- [x] 07-03-PLAN.md — Integration verification: full-stack sweep, token-leak audit, live smoke, visual checkpoint [wave 2]

**UI hint**: yes

**Phase notes:**
- New DB-free `internal/quota` package behind `GET /api/usage`; the browser never talks to Anthropic directly. Frontend is one new `QuotaIndicator` component (TanStack Query `refetchInterval` visible-only) mounted in both page headers, plus shadcn `hover-card`/`progress` copy-ins. Zero new dependencies.
- Plan-level decisions to encode (from research): read-only credentials (`~/.claude/.credentials.json`), never refresh/write the token (rotation can log the user out of claude itself); lenient JSON decode (rotating experimental fields, nullable `resets_at`).
- Verification should include the six-state degradation matrix (file absent / key absent / expired / 429 / 5xx / malformed) and a grep audit for `sk-ant-oat` in logs and API payloads.

### Phase 8: tmux Shells — Spawn & Detach Lifecycle
**Goal**: User can pick tmux as the bash-tab shell and get invisibly durable shells: tabs look and behave exactly like plain bash tabs (× kills), sessions survive leaving the task view — and (Phase 9) a server restart — and nothing about plain bash or agent sessions changes
**Depends on**: Phase 6 (v1.1 `AllowedShells`/LookPath settings seam, SHELL-FUT-01); independent of Phase 7
**Requirements**: TMUX-01, TMUX-02, TMUX-03, TMUX-04, TMUX-06, TMUX-07
**Success Criteria** (what must be TRUE):
  1. With tmux on PATH, "tmux" appears in the global shell setting dropdown; without tmux installed, the option is absent (TMUX-01)
  2. With tmux selected, opening a new bash tab spawns an attach-or-create tmux session (deterministic `kangent-<task>-<n>` name, `[A-Za-z0-9_-]` only) on the dedicated `-L kangent` socket, with the task worktree as cwd (TMUX-02)
  3. With a process left running (e.g. `top`), leaving the task view and reopening it reattaches to the same still-running session with a clean single repaint (no replayed garbage); the tab is indistinguishable from a plain bash tab — no status bar, no badge (TMUX-03, amended 2026-06-12)
  4. Closing a tmux-backed tab (×) kills the tmux session — kill-on-close parity with plain bash tabs; no separate kill affordance (TMUX-04, amended 2026-06-12)
  5. When the shell exits inside tmux, the tab shows the existing exited state — not "resumable" (exit vs detach discriminated via `tmux has-session` after the attach PTY exits); plain-bash tabs and agent sessions keep kill-on-stop behavior unchanged, locked in by regression tests on the shared stop path (TMUX-06, TMUX-07)
**Plans**: 4 plans (3 waves)

Plans:
- [x] 08-01-PLAN.md — internal/tmux leaf package (socket/config/has-session/kill-session) + migration 00005 tmux_sessions [wave 1]
- [x] 08-02-PLAN.md — TMUX-01: AllowedShells var → call-time LookPath function (conditional "tmux" dropdown option) [wave 1]
- [x] 08-03-PLAN.md — Session lifecycle: SpawnOpts.TmuxName, killer-first Stop, has-session exit-vs-detach, TMUX-07 regression guard [wave 2]
- [ ] 08-04-PLAN.md — Wiring: main.go config + handler name minting + D-84 honest error + frontend 409 rendering + human checkpoint [wave 3]

**UI hint**: yes

**Phase notes:**
- **Highest regression risk in the milestone:** the tmux server daemonizes, so the existing /proc process-tree sweep can't reach it. Lifecycle strategy (detach vs kill) must be an explicit per-session property decided at spawn time — not `if isTmux` branches at stop time. Detach/kill/exit semantics must land together; shipping a subset produces "Stop doesn't stop" or "close kills".
- Decided by research (treat as settled): dedicated socket `-L kangent` with `-f /dev/null` via one helper injecting both on every invocation; `=name` exact-match targets; no new session `Kind` (tmux tabs are `KindBash` + `tmuxName`); name minted and persisted by the HTTP handler (new `tmux_sessions` table, migration 00005, identity only — never status); env scrubbing (`TMUX`/`TMUX_PANE` removed, `TERM` pinned to `xterm-256color`) at the single spawn seam; cross-generation ring-buffer replay skipped for tmux tabs (tmux repaints itself).
- Amended by Phase 8 discussion (2026-06-12, see 08-CONTEXT.md): **invisible tmux** — × kills (kill-on-close parity with bash, D-78), detach only via leaving the task view or server shutdown; Kangent-controlled socket config: `status off` (D-79) + `set -g mouse on` (D-80, scrollback via tmux history); Ctrl+B prefix left default (D-81); setting flips apply at next spawn (D-83); tmux gone from PATH → honest spawn error (D-84); externally-died sessions surface the existing exited banner (D-82).
- New leaf package `internal/tmux` (`HasSession`/`KillSession`/`DetachClient`/`NewSessionArgs`) imported by both `internal/session` and `internal/api`. PTY/WS/ring-buffer transport untouched — a tmux client is just another full-screen PTY child.
- Cheap planning-time task: 5-minute smoke test of `=` exact-match targets and `detach-client` flags on the host tmux (the only MEDIUM-confidence details).

### Phase 9: tmux Restart Resume & Cleanup Integration
**Goal**: tmux shells complete the durability promise — they survive Kangent restarts with a Resume affordance, task/worktree cleanup accounts for them so no shell is ever orphaned in a deleted directory, and a Done-TTL reaper keeps finished tasks from accumulating idle sessions
**Depends on**: Phase 8 (needs working spawn + detach/kill/exit semantics to reconcile against)
**Requirements**: TMUX-05, TMUX-08, REAP-01
**Success Criteria** (what must be TRUE):
  1. After a Kangent server restart, tabs whose tmux sessions still exist offer Resume, and clicking it reattaches to the still-running session (mirrors the v1.1 agent reconcile → `resumable` flag → Resume UX) (TMUX-05)
  2. If a tmux session died while Kangent was down, the tab shows the exited/cleaned state — Resume is never offered for a dead session (TMUX-05, TMUX-06 boundary)
  3. Task/worktree cleanup gates count live detached tmux sessions as running work and surface them in the cleanup dialog; confirmed cleanup or task deletion kills the task's tmux sessions before worktree removal, leaving zero `kangent-*` sessions behind (TMUX-08)
  4. Sessions of tasks in Done — bash, tmux, AND agent — are killed after a configurable TTL (global setting, default 24h, clocked from entering Done; leaving Done cancels; 0/never disables); worktrees are never auto-removed (REAP-01, added 2026-06-12)
**Plans**: TBD

**Phase notes:**
- Reuses the Phase 5 (v1.0) `resumable` pattern with `tmux has-session` swapped in as the liveness probe; DB-derived detached entries in the sessions list with lazy row GC when the session died. Reattach = the same `new-session -A` spawn with the persisted name.
- Kill-session runs in worktree-remove and task-delete paths *before* `wt.Remove`; add an orphan sweep (tmux-has-it + DB-doesn't → kill) and a README note on `tmux -L kangent kill-server`.
- Frontend is thin wiring on existing patterns: ghost tabs + Resume button (v1.1 D-54 precedent), cleanup-gate dialog additions.
- Done-TTL reaper (REAP-01, D-86/D-87 in 08-CONTEXT.md): periodic server-side check kills all sessions (bash, tmux, agent) of tasks that have been in Done longer than the configured TTL; new global setting (duration, default 24h, 0/never disables); timer anchored to the Done-entry timestamp and cancelled when the task leaves Done; never touches worktrees.

## Progress

**Execution Order:**
Phases execute in numeric order: 7 → 8 → 9 (Phase 7 is independent of 8–9 and may run in either order relative to them)

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. Foundation — Projects & Board | v1.0 | 7/7 | Complete | 2026-06-10 |
| 2. Terminal Engine | v1.0 | 5/5 | Complete | 2026-06-10 |
| 3. Worktree Isolation & Bash Tabs | v1.0 | 6/6 | Complete | 2026-06-10 |
| 4. Claude Code Agent Sessions | v1.0 | 5/5 | Complete | 2026-06-11 |
| 5. Recovery & Review | v1.0 | 5/5 | Complete | 2026-06-11 |
| 6. Settings & Polish | v1.1 | 4/4 | Complete | 2026-06-11 |
| 7. Claude Quota Indicator | v1.2 | 0/3 | Not started | - |
| 8. tmux Shells — Spawn & Detach Lifecycle | v1.2 | 0/4 | Not started | - |
| 9. tmux Restart Resume & Cleanup Integration | v1.2 | 0/? | Not started | - |

---
*v1.0 shipped 2026-06-11 — 5 phases, 28 plans, 74 tasks*
*v1.1 shipped 2026-06-11 — 1 phase, 4 plans, 10 tasks*
*v1.2 roadmap created 2026-06-12 — Phases 7–9, granularity coarse, 16/16 requirements mapped*
