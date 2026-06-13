### Phase 9: tmux Restart Resume & Cleanup Integration
**Goal**: tmux shells complete the durability promise — they survive Kangent restarts and reattach automatically and invisibly when the task is reopened (no Resume button — D-88 diverges from agent Resume because tmux reattach is cheap/lossless), task/worktree cleanup accounts for them so no shell is ever orphaned in a deleted directory, and a Done-TTL reaper keeps finished tasks from accumulating idle sessions
**Depends on**: Phase 8 (needs working spawn + detach/kill/exit semantics to reconcile against)
**Requirements**: TMUX-05, TMUX-08, REAP-01
**Success Criteria** (what must be TRUE):
  1. After a Kangent server restart, reopening a task whose tmux session survived auto-reattaches to the still-running session — invisibly, no Resume button, no banner (D-88: reattach via DB-derived live-tmux entry gated by `has-session` → `new-session -A` with the persisted name) (TMUX-05)
  2. If a tmux session died while Kangent was down, the reopened task shows no tab for it and its `tmux_sessions` row is lazily GC'd — no exited stub (D-89) (TMUX-05, TMUX-06 boundary)
  3. Task/worktree cleanup gates count live detached tmux sessions into the single "N sessions running" number (no tmux wording) and surface them in the cleanup dialog; confirmed cleanup or task deletion kills the task's tmux sessions before worktree removal, leaving zero `kangent-*` sessions behind; a once-at-startup orphan sweep reconciles offline deletes (TMUX-08)
  4. Sessions of tasks in Done — bash, tmux, AND agent — are killed after a configurable TTL (global setting, default 24h, clocked from entering Done; leaving Done cancels; 0/never disables); the agent stays resumable (PTY killed, transcript kept); worktrees are never auto-removed (REAP-01, added 2026-06-12)
**Plans**: 5 plans (2 waves)

Plans:
- [ ] 09-01-PLAN.md — tmux ListSessions verb (D-94) + migration 00006 per-status timestamps with done_at backfill (D-90) [wave 1]
- [ ] 09-02-PLAN.md — done_session_ttl setting + ParseDoneSessionTTL helper + settings-page field (D-91) [wave 1]
- [ ] 09-03-PLAN.md — TMUX-05 restart reattach: DB-derived live-tmux session-list entries + reattach spawn variant + invisible frontend auto-reattach (D-88/D-89) [wave 1]
- [ ] 09-04-PLAN.md — TMUX-08 cleanup: fold live-tmux into the count + kill before remove/delete + startup orphan sweep (D-92/D-93) [wave 2]
- [ ] 09-05-PLAN.md — REAP-01 Done-TTL reaper: move status-timestamp stamping + reaper ticker goroutine + startup wiring (D-90/D-95/D-96) [wave 2]

**Phase notes:**
- TMUX-05 reattach is AUTO and INVISIBLE (D-88) — diverges from the Phase 5 agent Resume because tmux reattach is cheap/lossless. The WS handler requires a LIVE in-memory session (`mgr.Get(id)`), so a DB-derived ghost cannot WS-attach directly: `GET /api/sessions` reports surviving rows as `orphaned` entries and the frontend fires one reattach spawn (`new-session -A` attach-or-create) per orphan. Dead-on-restart rows are lazily GC'd (D-89). NO Resume button, NO badge (D-77 invisibility).
- Kill-session runs in worktree-remove and task-delete paths *before* `wt.Remove`/row delete (D-93); a once-at-startup DB-driven orphan sweep (ListSessions diff vs tmux_sessions rows) reconciles offline deletes; README note on `tmux -L kangent kill-server`.
- Done-TTL reaper (REAP-01, D-86/D-90/D-95/D-96): the codebase's first background goroutine — a ticker keyed on `done_at` (set by `move`) gated by `status='done'` kills all sessions (bash/tmux/agent) of long-Done tasks via `StopAllForTask`; agent stays resumable (PTY killed, claude_session_id+transcript kept, D-96); silent with server logs (D-95); never touches worktrees (D-87); `done_session_ttl` setting (duration, default 24h, 0/never disables).

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
| 9. tmux Restart Resume & Cleanup Integration | v1.2 | 0/5 | Not started | - |

---
*v1.0 shipped 2026-06-11 — 5 phases, 28 plans, 74 tasks*
*v1.1 shipped 2026-06-11 — 1 phase, 4 plans, 10 tasks*
*v1.2 roadmap created 2026-06-12 — Phases 7–9, granularity coarse, 16/16 requirements mapped*
