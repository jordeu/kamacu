---
gsd_state_version: 1.0
milestone: v1.2
milestone_name: Quota & Resumable Shells
status: verifying
stopped_at: Completed 08-04-PLAN.md
last_updated: "2026-06-13T05:09:54.370Z"
last_activity: 2026-06-13
progress:
  total_phases: 3
  completed_phases: 2
  total_plans: 7
  completed_plans: 7
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-06-11)

**Core value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.
**Current focus:** Phase 08 — tmux-shells-spawn-detach-lifecycle

## Current Position

Phase: 08 (tmux-shells-spawn-detach-lifecycle) — EXECUTING
Plan: 4 of 4
Status: Phase complete — ready for verification
Last activity: 2026-06-13

Progress: [░░░░░░░░░░] 0%

## Performance Metrics

**Velocity (v1.1):**

- Plans completed: 4
- Total execution time: ~68 min agent time

| Phase | Plans | Notable |
|-------|-------|---------|
| 6 | 4/4 | P01 11 min / P02 24 min / P03 8 min / P04 25 min (incl. human gate) |

Historical per-plan timings for v1.0 are preserved in `.planning/milestones/` archives and git history.
| Phase 07-claude-quota-indicator P02 | 5 min | 3 tasks | 5 files |
| Phase 07 P01 | 14 min | 3 tasks | 5 files |
| Phase 07-claude-quota-indicator P03 | 1h 8m | 2 tasks | 1 files |
| Phase 08-tmux-shells-spawn-detach-lifecycle P01 | 6min | 3 tasks | 3 files |
| Phase 08-tmux-shells-spawn-detach-lifecycle P02 | 8 min | 2 tasks | 4 files |
| Phase 08-tmux-shells-spawn-detach-lifecycle P03 | 9 min | 3 tasks | 3 files |
| Phase 08-tmux-shells-spawn-detach-lifecycle P04 | 52min | 3 tasks | 4 files |

## Accumulated Context

### Decisions

Full decision log lives in PROJECT.md (Key Decisions) and the archived milestone files:

- `.planning/milestones/v1.0-ROADMAP.md` / `v1.0-REQUIREMENTS.md`
- `.planning/milestones/v1.1-ROADMAP.md` / `v1.1-REQUIREMENTS.md`

Notable standing decisions for future work:

- D-51 reversed in v1.1 (AGENT-02): `--dangerously-skip-permissions` is the default extra-param; removable per-settings. Documented side effect: amber waiting dot rarely fires while active.
- Settings are global-only, read-at-use, absent-row-=-code-default; per-project overrides deferred (SET-FUT-01); additional shells are future data, not code (SHELL-FUT-01).

v1.2 roadmap-time decisions (from research, treat as settled):

- tmux runs on a dedicated `-L kangent` socket with `-f /dev/null` (never the user's default server/config); one helper injects both on every invocation.
- Lifecycle strategy (detach vs kill) is an explicit per-session property set at spawn time, not stop-time `if isTmux` branches — the milestone's main regression-risk control.
- Kangent is a read-only passenger on `~/.claude/.credentials.json`: never refreshes or writes the OAuth token; 401 → "re-authenticate with claude".
- Quota indicator is strictly best-effort: lenient decode, degrade-don't-break; the upstream endpoint is undocumented.
- [Phase 07-claude-quota-indicator]: Quota trigger bar: width tracks the 5h window, color tracks max utilization across ALL windows (QUOTA-07/D-70)
- [Phase 07-claude-quota-indicator]: D-72 warning chip resolved as TriangleAlert in text-amber-400 replacing the bar — same footprint, motion-free
- [Phase 07]: quota.Config gained an optional Now func() time.Time test seam (research-sanctioned) so api-package handler tests control TTL/floor without sleeps
- [Phase 07]: 10s quota attempt floor binds ALL upstream attempts (not just ?refresh=1), protecting a failing upstream after the cache drops
- [Phase 07]: Quota token fingerprint recorded on every Get so pre-success failures accumulate toward the 3-failure drop instead of resetting
- [Phase 07]: Quota popup footer/reset countdowns tick on a 10s useNow interval mounted only while the popup is open — supersedes 07-02's no-extra-timer decision (live test showed 'Updated 0m ago' frozen against 60s TTL + 60s poll)
- [Phase 07]: Quota popup reset column: bare duration text, no label/icon, fixed w-14 tabular-nums so all row bars share an identical track width (checkpoint feedback)
- [Phase 08-tmux-shells-spawn-detach-lifecycle]: Generated -f config file (status off, mouse on, history-limit 50000) over post-create set-option sequence — the latter raced empirically
- [Phase 08-tmux-shells-spawn-detach-lifecycle]: tmux_sessions is identity-only (no status column); tmux itself is the status authority via has-session
- [Phase 08-tmux-shells-spawn-detach-lifecycle]: KillSession/KillServer treat exit 1 as idempotent success (already dead / no server)
- [Phase 08]: AllowedShells converted var->func with exec.LookPath('tmux') checked at call time — dropdown offering and save acceptance can never disagree, no restart needed (TMUX-01)
- [Phase 08-tmux-shells-spawn-detach-lifecycle]: Stop is killer-first: kill-session ends inner shell + tmux session atomically; killer error or undead client falls through to SIGTERM/grace/SIGKILL
- [Phase 08-tmux-shells-spawn-detach-lifecycle]: Manual detach recorded as detachedAlive with honest exited state in Phase 8 — no auto-reattach; Info() wire shape keeps no tmux marker (D-77)
- [Phase 08-tmux-shells-spawn-detach-lifecycle]: tmux session names come from tmux_sessions MAX(n)+1 reserved by an INSERT before Spawn (never the in-memory counter) — unique across restarts, UNIQUE(task_id,n) the backstop; spawn failure DELETEs the reserved row, success back-fills label warn-only (TMUX-02)
- [Phase 08-tmux-shells-spawn-detach-lifecycle]: Honest spawn errors are 409s rendered verbatim in the tab header (D-84 'tmux not found...', dev-route 'tmux shells need a task', 'task has no worktree'); 500s stay generic. Frontend stays 100% tmux-unaware (D-77) — only ApiError status===409 forwards the message
- [Phase 08-tmux-shells-spawn-detach-lifecycle]: TMUX-05 (restart reconcile + Resume) deferred to Phase 9: tmux_sessions is persisted but not read at startup and ListByTask is in-memory only, so a surviving tmux session loses its tab after a server restart — no schema change needed, mirrors Phase 5 claude --resume reconcile

### Pending Todos

- Quota poll while idle (no connected browsers) is acceptable for the first iteration with jitter + backoff; revisit before milestone close (research tech-debt note).

### Blockers/Concerns

- Plan-mode exit-plan approval → amber dot (v1.0 research OQ1): still unobserved — carry to v1.2 UAT.
- `api/oauth/usage` is undocumented/gray-area; handled by design (best-effort indicator, `available:false` degradation), but upstream changes could land mid-milestone.

## Session Continuity

Last session: 2026-06-13T05:09:42.370Z
Stopped at: Completed 08-04-PLAN.md
Resume file: None
Next: `/gsd:plan-phase 7`
