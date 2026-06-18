# Requirements: Kangent — v1.6 Global Active Sessions Bar

**Defined:** 2026-06-18
**Core Value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.

## v1.6 Requirements

A persistent bottom status bar giving a single, app-wide view of every active Claude agent session across all projects — collapsed for stats, expanded to browse and jump in. Built on the existing `GET /api/agents/status` feed (agent sessions only); no new external surface.

### Global Sessions Bar

- [ ] **SBAR-01**: User sees a persistent status bar pinned to the bottom of the app on every screen (board, task view, settings), regardless of which project is open.
- [ ] **SBAR-02**: Collapsed by default, the bar shows global counts of active agent sessions broken down by state (working / waiting / idle) plus a total.
- [ ] **SBAR-03**: When one or more sessions are waiting for input, the bar highlights it prominently (pulsing-amber emphasis + count) so the user notices without expanding.
- [ ] **SBAR-04**: User can expand the bar to see every active agent session as a row showing its project name, task (or PR) title, and live state.
- [ ] **SBAR-05**: In the expanded list, sessions are ordered attention-first (waiting → working → idle) so the ones needing input are at the top.
- [ ] **SBAR-06**: User can click a session row to jump straight into that task's full-page agent view — including when the session belongs to a different project than the one currently open (cross-project navigation).
- [ ] **SBAR-07**: User can collapse and expand the bar, and that choice persists across page reloads (default collapsed).
- [ ] **SBAR-08**: The bar reflects session changes within ~5s using the existing agent-status poll — sessions appear when started and drop off when they exit.
- [ ] **SBAR-09**: When there are no active agent sessions, the bar stays present and shows a quiet zero/empty state (the affordance is always discoverable, never blocks the view).
- [x] **SBAR-10**: The agent-status feed carries each session's task title and project name so the bar renders every row's label from one existing request (no per-session fetch, no new DB migration).

## Future Requirements

Acknowledged but deferred — not in the v1.6 roadmap.

### Global Sessions Bar

- **SBAR-FUT-01**: Per-session quick actions from the bar (stop / resume / start) without navigating into the task.
- **SBAR-FUT-02**: Time-in-state / duration per session row (e.g. "waiting 4m") — needs a state-transition timestamp the feed doesn't expose today.
- **SBAR-FUT-03**: Browser/OS desktop notification when a session starts waiting (the long-deferred NOTF-01).
- **SBAR-FUT-04**: Tab title / favicon waiting badge — ambient out-of-app signal with no permission prompt.
- **SBAR-FUT-05**: Include bash/tmux terminal sessions in the bar (behind a filter/toggle).
- **SBAR-FUT-06**: Group or filter the expanded list by project or by state.

## Out of Scope

Explicitly excluded for v1.6, with reasoning (kept in Future where they may return).

| Feature | Reason |
|---------|--------|
| Desktop/browser notifications + tab-title/favicon changes | User chose in-bar-only alerting for v1.6 (pulsing highlight + count). Parked as SBAR-FUT-03/04. |
| bash/tmux terminal sessions in the bar | Agent sessions only this milestone; the existing `/api/agents/status` feed is agent-only (bash never drives board state, D-48). Parked as SBAR-FUT-05. |
| Replacing the per-project sidebar "N waiting" chips | User chose to keep both — the bar is additive, the sidebar chips stay as the per-project glance. |
| Exited / dead sessions in the bar | The bar shows only *active* (live-PTY) sessions; exited sessions still surface on cards and via Resume. Keeps "active sessions" honest. |
| New DB migration / schema change | Task title + project name come from existing columns via a JOIN onto the status query (SBAR-10); no schema work expected. |
| Per-session quick actions (stop/resume/start) from the bar | v1.6 navigates only; in-bar actions are SBAR-FUT-01. |

## Traceability

Which phases cover which requirements. Populated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| SBAR-01 | Phase 17 | Pending |
| SBAR-02 | Phase 17 | Pending |
| SBAR-03 | Phase 17 | Pending |
| SBAR-04 | Phase 17 | Pending |
| SBAR-05 | Phase 17 | Pending |
| SBAR-06 | Phase 17 | Pending |
| SBAR-07 | Phase 17 | Pending |
| SBAR-08 | Phase 17 | Pending |
| SBAR-09 | Phase 17 | Pending |
| SBAR-10 | Phase 17 | Complete |

**Coverage:**
- v1.6 requirements: 10 total
- Mapped to phases: 10 (all → Phase 17) ✓
- Unmapped: 0 ✓

---
*Requirements defined: 2026-06-18*
*Last updated: 2026-06-18 after roadmap creation (all SBAR-01..SBAR-10 → Phase 17)*
