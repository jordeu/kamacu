# Phase 9: tmux Restart Resume & Cleanup Integration - Context

**Gathered:** 2026-06-13
**Status:** Ready for planning

<domain>
## Phase Boundary

Complete the tmux durability promise. Three threads:
1. **TMUX-05** — a tmux session that survived a Kangent restart reattaches when the user reopens the task (the headline value: "the session survives a server restart").
2. **TMUX-08** — worktree cleanup and task deletion account for live tmux sessions (count them, kill them before removal) so no shell is ever orphaned in a deleted directory.
3. **REAP-01** — a Done-TTL reaper kills idle sessions (bash + tmux + agent) on tasks that have sat in Done past a configurable TTL, so sessions don't accumulate forever.

This phase reuses the Phase 5 restart-reconcile pattern (DB-derived ghost entries + a liveness probe) with `tmux has-session` as the probe, and rides existing seams (settings, cleanup dialog, session list).

</domain>

<decisions>
## Implementation Decisions

### Restart reattach (TMUX-05)
- **D-88:** **Auto-reattach, invisible.** Reopening a task whose tmux session survived a restart → the tab silently reappears and reconnects (one clean tmux repaint), with whatever was running still there. No "Resume" button, no banner. This is a deliberate divergence from the agent's explicit-Resume pattern (Phase 5 D-54): agent Resume is explicit because it spawns a new `claude` process and costs tokens; tmux reattach is cheap and lossless (the session is just sitting there), so it stays invisible per D-77/D-78. Mechanism mirrors Phase 5 reconcile: `GET /api/sessions` gains DB-derived entries for `tmux_sessions` rows whose `tmux has-session` is live but have no in-memory session; reattach goes through the existing WS attach → `new-session -A` with the persisted name.
- **D-89:** **Dead-on-restart → tab silently absent.** If the tmux session did NOT survive (full machine reboot kills the tmux server, not just the Kangent process), the reopened task shows no tab for it and its `tmux_sessions` row is lazily GC'd. No exited stub. Matches Phase 5 D-58 (bash tabs vanish quietly after restart) and the invisible-tmux model — the work is gone either way, no tombstone UI.

### Done-TTL reaper (REAP-01)
- **D-90:** **Per-status timestamps.** Add `todo_at`, `in_progress_at`, `in_review_at`, `done_at` columns (new migration 00006), each set when the task enters that status (last-entry-wins; a re-entry overwrites). The reaper is keyed on `done_at` (pure time-in-Done — editing a Done task's title/description does NOT reset it). The other three are banked now for future cycle-time / dwell-time stats. Reaper cancellation is gated by `status = 'done'` (leaving Done stops reaping regardless of `done_at`). Existing rows backfilled (approximate from `updated_at`) so current Done tasks are reapable — see Discretion.
- **D-91:** **TTL setting = duration string, default `"24h"`.** A new global setting (e.g. `done_session_ttl`) holding a Go-style duration; empty / `"0"` / `"never"` disables reaping. Validated at save like the other settings (Phase 6 pattern); rendered as a new field on the settings page.
- **D-95:** **Reaper UX = silent; reflect reality on open.** No notification, no card badge. The next time the user opens a reaped Done task, its sessions are simply gone (bash/tmux tabs absent per D-89; agent shows its normal resumable ghost per D-96). The server logs each reap. Mirrors the Phase 5 D-65 silent-reconcile philosophy.
- **D-96:** **Agent reap = kill PTY, keep resumable.** The reaper stops the live agent process (frees the PTY) but leaves `tasks.claude_session_id` + the transcript intact, so the Done task's agent shows its normal resumable ghost — one click brings it back via `claude --resume`. Reaping reclaims resources without destroying conversations; fully reversible. (Confirms D-86's "include agent sessions" with the safety that nothing is lost.)

### Cleanup & orphan handling (TMUX-08)
- **D-92:** **Cleanup dialog counts live tmux sessions into one number.** The "N sessions running" gate's count includes live tmux sessions (probe each `tmux_sessions` row via `has-session`), folded into the single existing figure — no tmux-specific line, stays invisible (D-77). "Stop sessions and clean up" kills them via `kill-session` **before** `wt.Remove`.
- **D-93:** **Kill by DB rows + startup orphan sweep.** On worktree-remove AND task-delete: kill every `tmux_sessions` row's session for that task (`has-session` → `kill-session`) before removal. PLUS a once-at-startup, DB-driven orphan sweep: enumerate live `kangent-*` sessions on the socket and kill any whose name has no matching `tmux_sessions` row (or whose task no longer exists). This reconciles deletes that happened while the server was down. Branch is always kept (D-34 unchanged).
- **D-94:** **Add `ListSessions` to `internal/tmux`.** Extend the Phase 8 leaf package with `ListSessions` (`list-sessions -F '#{session_name}'` on the kangent socket) so the orphan sweep can diff live sessions against DB rows. Keeps every tmux verb in the one leaf package (Phase 8 pattern).
- **Orphan sweep timing (D-93 detail):** runs once at startup, synchronously, before serving. Not periodic — tmux sessions only become orphaned through paths Kangent already controls, so steady-state drift is negligible.

### Claude's Discretion
- Reaper tick interval (the background goroutine's period — e.g. every 5–15 min; this is the codebase's first background goroutine).
- Existing-row backfill for the per-status timestamps: approximate from `updated_at` (Done rows get `done_at = updated_at` so current Done tasks are reapable; other `*_at` from `updated_at`/`created_at` or NULL).
- Tab labels/counter on restore: reuse the stored `tmux_sessions.label`; new spawns must not collide with restored `n` (the existing `MAX(n)+1` minting already handles this since rows persist across restart).
- Whether non-active restored tabs eagerly connect or connect on tab-activate — fine either way as long as reopening the task *feels* automatic (D-88).
- Exact cleanup-dialog copy, settings field label/help text, log message wording.
- README note on `tmux -L kangent kill-server` as the manual escape hatch.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & roadmap
- `.planning/ROADMAP.md` — Phase 9 section: settled approach (reuse Phase 5 `resumable` with `has-session` probe, kill-before-remove, orphan sweep, README note) and the Done-TTL reaper note
- `.planning/REQUIREMENTS.md` — TMUX-05, TMUX-08, REAP-01 (REAP-01 added 2026-06-12)

### Patterns this phase mirrors (read for the established shape)
- `.planning/phases/05-recovery-review/05-CONTEXT.md` — D-54/D-57 (Resume affordance; tmux diverges per D-88), D-58 (bash vanish quietly → D-89), D-65 (silent reconcile → D-95), D-66/D-67 (task-level persistence, one-per-task)
- `.planning/phases/08-tmux-shells-spawn-detach-lifecycle/08-CONTEXT.md` — D-77/D-78 (invisible tmux, × kills), D-86/D-87 (REAP-01 semantics decided here)
- `.planning/phases/03-worktree-isolation-bash-tabs/03-CONTEXT.md` — D-31/D-32/D-33/D-34 (cleanup gates, branch always kept)

No external specs — tmux behavior verified empirically in Phase 8 (`08-RESEARCH.md`).

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets / Patterns to mirror
- `internal/api/agents.go:36-158` — the **two-pass reconcile** (manager-derived + DB-derived post-restart survivors) and `resumable` derivation (`:102`). The tmux equivalent: DB-derived `tmux_sessions` entries gated by `has-session` instead of `transcriptExists`. This is the model for D-88.
- `internal/api/resume.go` `transcriptExists()` — analog of the `has-session` liveness probe.
- `web/src/components/task/AgentTab.tsx:40-79` — the agent Resume UI. **NOT replicated for tmux** (D-88 auto-reattaches); read it to understand the pattern being deliberately diverged from.
- `internal/session/manager.go:370-389` `StopAllForTask` — concurrent stop through the 5s grace; the cleanup/delete kill path extends this for tmux (`kill-session` before `wt.Remove`).
- `internal/settings/settings.go:14-47` (`Keys`, `Defaults`, `Get/Set`) + `validate.go` + `internal/api/settings.go:32-82` (GET all / PUT one) — the seam for the new TTL setting (D-91), Phase 6 pattern.

### Integration Points (where new code connects)
- `GET /api/sessions` (`internal/api/sessions.go`, `mgr.ListByTask`) → add DB-derived live tmux entries so tabs reappear post-restart (D-88). `web/src/pages/TaskPage.tsx:63-142` consumes it.
- `internal/api/worktrees.go:49-58` `runningSessions` + `:138-212` cleanup handler → fold live-tmux count in (D-92), kill before `wt.Remove`.
- `internal/api/tasks.go:481-499` `delete` → add tmux kill before DELETE (D-93); `:302-405` `move` (status change) → set the per-status `*_at` timestamps (D-90).
- `internal/store/migrations/` → new `00006` for per-status timestamps (D-90). `00001_init.sql` has the tasks table (status enum, created_at, updated_at, no status_at today).
- `internal/tmux/tmux.go` → add `ListSessions` (D-94); existing `HasSession`/`KillSession`/`KillServer`/`NewSessionArgs`/`WriteConfig` reused.
- `cmd/kangent/main.go:28-158` → **no background goroutine exists today.** Add (a) the reaper ticker (D-90/D-91/D-95/D-96) and (b) the synchronous startup orphan sweep before serving (D-93).

</code_context>

<specifics>
## Specific Ideas

- The user's headline framing throughout this milestone: "the main goal of adding tmux was that sessions survive a server restart." D-88 is the payoff — and it must stay invisible (no Resume button on a tab that looks like plain bash).
- "Add a done_at timestamp but also todo_at, in_progress_at, in_review_at for consistency and it will be useful in the future to show some stats." — the user explicitly wants the full per-status timestamp set (D-90), not just `done_at`, as a forward investment in board analytics.

</specifics>

<deferred>
## Deferred Ideas

- **Board cycle-time / dwell-time stats** built on the new per-status timestamps (D-90) — the timestamps are banked now; surfacing stats is a future capability, not this phase.
- **Subtle board indicator for reaped tasks** — considered (D-95 alternative), declined for invisibility; could revisit if users miss the silent reap.
- **Auto-removing worktrees on the Done TTL** — already declined in Phase 8 (D-87); REAP-01 kills sessions only.
- **Periodic orphan sweep** — considered (D-93 alternative), declined as redundant; startup-only is sufficient.

</deferred>

---

*Phase: 09-tmux-restart-resume-cleanup-integration*
*Context gathered: 2026-06-13*
