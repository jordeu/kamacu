---
gsd_state_version: 1.0
milestone: v1.2
milestone_name: Quota & Resumable Shells
status: verifying
stopped_at: Completed 09-05-PLAN.md
last_updated: "2026-06-13T07:20:29.095Z"
last_activity: 2026-06-13
progress:
  total_phases: 3
  completed_phases: 3
  total_plans: 12
  completed_plans: 12
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-06-11)

**Core value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.
**Current focus:** Phase 09 — tmux-restart-resume-cleanup-integration

## Current Position

Phase: 09
Plan: Not started
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
| Phase 09 P01 | 3 min | 2 tasks | 3 files |
| Phase 09-tmux-restart-resume-cleanup-integration P02 | 4min | 2 tasks | 5 files |
| Phase 09-tmux-restart-resume-cleanup-integration P03 | 14 min | 3 tasks | 11 files |
| Phase 09-tmux-restart-resume-cleanup-integration P04 | 14 min | 3 tasks | 6 files |
| Phase 09-tmux-restart-resume-cleanup-integration P05 | 12 min | 3 tasks | 5 files |

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
- [Phase 09]: ListSessions captures stdout via exec.Output() (not the error-only run helper); tmux exit 1 (no server running) is the empty case nil,nil, never an error (D-94)
- [Phase 09]: Migration 00006 adds 4 nullable per-status timestamp cols on tasks; only done_at is backfilled (from updated_at) so existing Done rows are reapable, the other three are banked stats with no current consumer (D-90)
- [Phase 09-tmux-restart-resume-cleanup-integration]: [Phase 09-02]: ParseDoneSessionTTL is the single source of truth for done_session_ttl disable semantics — called by both Validate (save-time) and the 09-05 reaper (read-at-use), so they can never disagree (mirrors AllowedShells shared by Validate + GET options)
- [Phase 09-tmux-restart-resume-cleanup-integration]: [Phase 09-02]: a done_session_ttl duration <= 0 (e.g. 0s, -5m) is treated as disabled, not an error and not reap-everything — it can never expire (REAP-01/D-91)
- [Phase 09-tmux-restart-resume-cleanup-integration]: TMUX-05 restart reattach is AUTO + INVISIBLE (D-88): a surviving tmux row surfaces as an orphaned bash entry in GET /api/sessions; the frontend fires one reattach spawn (new-session -A) per name — no Resume button, no banner (diverges from agent explicit-Resume)
- [Phase 09-tmux-restart-resume-cleanup-integration]: Survivor detection uses Manager.HasLiveTmux(name) so the wire format is unchanged for live sessions; TmuxName is carried on the wire ONLY for orphaned entries (omitempty), keeping live tabs tmux-unaware (D-77)
- [Phase 09-tmux-restart-resume-cleanup-integration]: Dead-on-restart rows are lazily DELETEd ONLY on a conclusive has-session exit-1 (D-89); an inconclusive probe (tmux binary broken/hung) never GCs — Pitfall 6 honesty. Reattach spawn failure never deletes the row (pre-existing survivor, not a reserved n)
- [Phase 09-tmux-restart-resume-cleanup-integration]: [Phase 09-04]: Cleanup dialog running_sessions folds in live DETACHED tmux via has-session, de-duped by Manager.HasLiveTmux — one honest number, no tmux-specific field (D-92/D-77); a conclusive-only probe (alive && err==nil) keeps a broken tmux from inflating or killing (Pitfall 6)
- [Phase 09-tmux-restart-resume-cleanup-integration]: [Phase 09-04]: TMUX-08 kill-before-remove on BOTH worktree-remove and task-delete (StopAllForTask reaches only in-memory sessions; detached survivors killed by tmux_sessions row), plus a synchronous once-at-startup DB-driven orphan sweep (JOIN tmux_sessions vs tasks) before ListenAndServe — branch always kept (D-34), worktrees never touched (D-87), never periodic (D-99); task-delete removes tmux_sessions rows explicitly (no FK cascade, migration 00005)
- [Phase 09-tmux-restart-resume-cleanup-integration]: [Phase 09-05]: REAP-01 reaper is the codebase's first background goroutine — a 10-min ticker (reap-once-at-start) launched after the orphan sweep, before ListenAndServe, on context.Background() (no graceful shutdown; process death is the stop)
- [Phase 09-tmux-restart-resume-cleanup-integration]: [Phase 09-05]: the reaper has zero DB writes — one SELECT gated on status='done' AND done_at IS NOT NULL AND done_at < lexical-ISO-cutoff, then StopAllForTask; claude_session_id/transcript (D-96) and worktrees (D-87) are never touched, automatically
- [Phase 09-tmux-restart-resume-cleanup-integration]: [Phase 09-05]: move stamps only the entered status's *_at column via a fixed status->column map (never raw req.Status), last-entry-wins; leaving Done never clears done_at — the status='done' gate cancels reaping, not done_at (D-90)

### Pending Todos

- Quota poll while idle (no connected browsers) is acceptable for the first iteration with jitter + backoff; revisit before milestone close (research tech-debt note).

### Blockers/Concerns

- Plan-mode exit-plan approval → amber dot (v1.0 research OQ1): still unobserved — carry to v1.2 UAT.
- `api/oauth/usage` is undocumented/gray-area; handled by design (best-effort indicator, `available:false` degradation), but upstream changes could land mid-milestone.

## Session Continuity

Last session: 2026-06-13T07:10:53.180Z
Stopped at: Completed 09-05-PLAN.md
Resume file: None
Next: `/gsd:plan-phase 7`
