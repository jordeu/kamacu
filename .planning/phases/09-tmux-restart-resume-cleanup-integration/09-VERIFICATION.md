---
phase: 09-tmux-restart-resume-cleanup-integration
verified: 2026-06-13T09:30:00Z
status: passed
score: 16/16 must-haves verified
re_verification: null
---

# Phase 9: tmux Restart Resume & Cleanup Integration Verification Report

**Phase Goal:** tmux shells complete the durability promise — they survive Kangent restarts and reattach automatically and INVISIBLY when the task is reopened (no Resume button — D-88), task/worktree cleanup accounts for them so no shell is ever orphaned in a deleted directory, and a Done-TTL reaper keeps finished tasks from accumulating idle sessions.
**Verified:** 2026-06-13T09:30:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| #   | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| 1   | Orphan sweep/cleanup can enumerate live `kangent-*` tmux sessions by name | ✓ VERIFIED | `internal/tmux/tmux.go:138` `ListSessions` runs `list-sessions -F '#{session_name}'`, trims/drops empties, treats exit-1 as empty (nil,nil). Tested in `internal/tmux/tmux_test.go` (passing). |
| 2   | Every task carries per-status timestamp columns (`done_at` + 3 banked) | ✓ VERIFIED | Migration `00006` adds `todo_at/in_progress_at/in_review_at/done_at` (all nullable). Applies clean up+down; store tests pass. |
| 3   | Existing Done tasks immediately reapable (done_at backfilled) | ✓ VERIFIED | `UPDATE tasks SET done_at = updated_at WHERE status = 'done';` in 00006. |
| 4   | `done_session_ttl` setting exists, default 24h, read-at-use | ✓ VERIFIED | `settings.go:20,34` KeyDoneSessionTTL + Defaults "24h". Reaper reads via `settings.Get`. |
| 5   | Invalid TTL rejected at save; empty/0/never accepted (disable) | ✓ VERIFIED | `validate.go:48-54` case + `ParseDoneSessionTTL` (empty/"0"/"never"/non-positive → disabled). Canonical error copy. Tests pass. |
| 6   | TTL renders as a settings-page field | ✓ VERIFIED | `SettingsPage.tsx:125-134` Cleanup section, `settingKey="done_session_ttl"`, DONE_TTL_HELP. tsc + build pass. |
| 7   | After restart, reopening a task with a surviving tmux session shows its tab and reconnects | ✓ VERIFIED | End-to-end: `sessions.go:75` reconcileTmux emits orphaned entries (has-session gated) → frontend auto-fires reattach → `create` reattach branch runs `new-session -A` → real in-memory session WS can attach. `TestSessionTmuxReattach` asserts `mgr.Get(sid)` succeeds. |
| 8   | A tmux session that died while down produces no tab; row lazily DELETEd | ✓ VERIFIED | `sessions.go:131-137` conclusive-dead (exit 1) → `DELETE FROM tmux_sessions WHERE name = ?`, no append. Inconclusive probe leaves row (Pitfall 6 honesty). |
| 9   | Restored tabs visually identical to bash — no Resume, no badge | ✓ VERIFIED | Orphaned entry Kind="bash"; frontend filters orphaned ghosts out of tabs, auto-reattaches; reattach test asserts kind=="bash". D-88 audit below. |
| 10  | Cleanup dialog count includes live detached tmux | ✓ VERIFIED | `worktrees.go:114` cleanupSessionCount = runningSessions + live-tmux-not-in-memory (HasLiveTmux de-dup). Single `running_sessions` key, no tmux JSON field. |
| 11  | Confirmed cleanup kills tmux before wt.Remove — zero leftovers | ✓ VERIFIED | `worktrees.go:266-277` StopAllForTask → KillSession loop → wt.Remove. Correct ordering. |
| 12  | Deleting a task kills tmux before row delete | ✓ VERIFIED | `tasks.go:507-529` StopAllForTask → KillSession → DELETE tmux_sessions → DELETE tasks. |
| 13  | Startup sweep kills DB-less / task-gone `kangent-*` sessions | ✓ VERIFIED | `main.go:190` sweepOrphanTmux: ListSessions vs `JOIN tasks` known-set, prefix-guard `kangent-`, logs each, synchronous before ListenAndServe (line 162 < 176). |
| 14  | Move stamps the entered status's `*_at` column (last-entry-wins) | ✓ VERIFIED | `tasks.go:401-410` fixed-map statusAtCol concatenated into UPDATE; req.Status validated at :336. |
| 15  | Background reaper kills bash+tmux+agent of expired Done tasks; never/0/empty disables; leaving Done cancels | ✓ VERIFIED | `reaper.go:118-156` query `status='done' AND done_at IS NOT NULL AND done_at < cutoff` → StopAllForTask. ParseDoneSessionTTL disable + status gate. 6 behavioral tests + mixed-scenario test pass. |
| 16  | Reaping silent + logs each reap; agent stays resumable (PTY killed, claude_session_id/transcript kept) | ✓ VERIFIED | `reaper.go:156` logs each reap, no UI. StopAllForTask does ZERO DB work (manager.go:387 only calls s.Stop()); only claude_session_id write in codebase is the agent spawn path (sessions.go:336 UPDATE = write, never delete). D-96 confirmed. |

**Score:** 16/16 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/tmux/tmux.go` | ListSessions verb | ✓ VERIFIED | Substantive impl, exit-1→empty, tested |
| `internal/store/migrations/00006_task_status_timestamps.sql` | 4 columns + backfill | ✓ VERIFIED | All 4 ADD COLUMN + done_at backfill + Down drops |
| `internal/settings/settings.go` | KeyDoneSessionTTL + default | ✓ VERIFIED | const + Defaults "24h" |
| `internal/settings/validate.go` | ParseDoneSessionTTL + validation | ✓ VERIFIED | Shared parse helper, canonical error |
| `web/src/pages/SettingsPage.tsx` | TTL field | ✓ VERIFIED | Cleanup section, wired |
| `internal/api/sessions.go` | reconcile + reattach | ✓ VERIFIED | reconcileTmux + reattach branch, both substantive |
| `internal/session/manager.go` | HasLiveTmux | ✓ VERIFIED | Used by sessions.go + worktrees.go |
| `internal/session/session.go` | Orphaned + TmuxName fields | ✓ VERIFIED | Both present, omitempty |
| `web/src/api/sessions.ts` | orphaned/tmuxName + useReattachTmux | ✓ VERIFIED | Mutation + cache write |
| `web/src/pages/TaskPage.tsx` | auto-reattach, no Resume | ✓ VERIFIED | useEffect one-shot reattach w/ ref guard; orphaned filtered from tabs |
| `internal/api/worktrees.go` | count fold-in + kill-before-remove | ✓ VERIFIED | cleanupSessionCount + KillSession loop |
| `internal/api/tasks.go` | kill-before-delete + move stamping | ✓ VERIFIED | Delete kill ordering + statusAtCol |
| `internal/reaper/reaper.go` | ticker + reapOnce | ✓ VERIFIED | New/Run/reapOnce, gated query, now() seam |
| `cmd/kangent/main.go` | sweep + reaper launch | ✓ VERIFIED | sweepOrphanTmux (sync) + go reaper.New().Run() |

### Key Link Verification

| From | To | Via | Status |
| ---- | -- | --- | ------ |
| sessions.go list | surviving tmux rows | ListByTask + HasSession diff, lazy DELETE | ✓ WIRED |
| TaskPage.tsx | POST reattach | spawn with persisted name on open | ✓ WIRED |
| ListSessions | tmux list-sessions | exec.CommandContext on -L kangent | ✓ WIRED |
| validate.go | KeyDoneSessionTTL case | time.ParseDuration via ParseDoneSessionTTL | ✓ WIRED |
| worktrees.go remove | KillSession per row | has-session → kill before wt.Remove | ✓ WIRED |
| main.go startup | ListSessions diff vs rows | kill kangent-* w/o DB row, sync before serve | ✓ WIRED |
| tasks.go move | {status}_at column | fixed-map set in UPDATE | ✓ WIRED |
| reaper.go | StopAllForTask per expired Done | tick → query → StopAllForTask | ✓ WIRED |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Real Data | Status |
| -------- | ------------- | ------ | --------- | ------ |
| TaskPage orphaned ghosts | `sessions` | GET /api/sessions reconcileTmux (live DB rows + has-session probe) | Yes | ✓ FLOWING |
| reaper expired set | SELECT id FROM tasks | real DB query gated on status+done_at | Yes | ✓ FLOWING |
| cleanup count | running_sessions | runningSessions + liveTmuxNames (real has-session probes) | Yes | ✓ FLOWING |
| SettingsPage field | settings.done_session_ttl | GET /api/settings (data-driven Defaults) | Yes | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| ListSessions | `go test ./internal/tmux/ -run ListSessions` | ok | ✓ PASS |
| Reaper 6 cases + mixed | `go test ./internal/reaper/` | ok | ✓ PASS |
| TTL parse + validate | `go test ./internal/settings/ -run DoneSessionTTL\|Validate` | ok | ✓ PASS |
| Migration + reattach + delete | `go test ./internal/store/ ./internal/api/` | ok | ✓ PASS |
| Full suite | `go test ./...` | all ok | ✓ PASS |
| Frontend typecheck + build | `npx tsc --noEmit && npm run build` | exit 0 | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| TMUX-05 | 09-03 | Restart auto-reattach, invisible, no Resume button | ✓ SATISFIED | End-to-end chain verified + TestSessionTmuxReattach proves mgr-attachable session; D-88 no-Resume audit passes |
| TMUX-08 | 09-01, 09-04 | Cleanup counts + kills tmux; no orphaned shells | ✓ SATISFIED | Count fold-in, kill-before-remove, kill-before-delete, startup sweep all wired |
| REAP-01 | 09-01, 09-02, 09-05 | Done-TTL reaper for bash+tmux+agent; configurable; resumable agent | ✓ SATISFIED | Reaper gated query + StopAllForTask + ParseDoneSessionTTL disable + D-96 keep-resumable confirmed |

No orphaned requirements: all three IDs appear in plan frontmatter and REQUIREMENTS.md maps exactly these three to Phase 9.

### Critical Decision Verification (D-88..D-96)

| Decision | Status | Evidence |
| -------- | ------ | -------- |
| D-88 auto+invisible, NO tmux Resume | ✓ HONORED | Only "Resume" in TaskPage.tsx is a comment saying there is none; agent Resume lives solely in AgentTab.tsx (pre-existing, allowed) |
| D-89 dead-on-restart → no tab, lazy GC | ✓ HONORED | sessions.go:131-137 DELETE on conclusive-dead, no stub emitted |
| D-90 migration 00006 + move stamps + reaper keyed on done_at | ✓ HONORED | All present; reaper gated status='done' |
| D-91 done_session_ttl default 24h, disable semantics, validated | ✓ HONORED | settings + ParseDoneSessionTTL + validate case |
| D-92 single count, no double-count, no tmux dialog wording | ✓ HONORED | HasLiveTmux de-dup; no tmux JSON key |
| D-93 kill-before-remove on BOTH paths + startup sweep, branch kept | ✓ HONORED | worktrees + tasks kill ordering + sweepOrphanTmux |
| D-94 ListSessions in internal/tmux | ✓ HONORED | tmux.go:138 |
| D-95 silent reaper, logs each reap | ✓ HONORED | reaper.go:156 log, no UI |
| D-96 reaper kills PTY, NEVER deletes claude_session_id/transcript | ✓ HONORED | StopAllForTask does zero DB work; only claude_session_id SQL is a WRITE in the agent spawn path, never in the reaper |
| D-77 frontend stays tmux-unaware (no tmux-specific UI) | ✓ HONORED | Only tmuxName wire field + reattach plumbing; restored tabs render as plain bash |

### Anti-Patterns Found

None. No TODO/FIXME/PLACEHOLDER/stub markers in any modified Go or frontend file.

### Human Verification Required

None blocking. The reattach chain is proven at the HTTP layer by `TestSessionTmuxReattach` (real tmux session → 201 → mgr.Get succeeds). A full manual end-to-end (restart the running server, reopen a task, watch the terminal repaint) would confirm the live UX feel but is not required for goal achievement — every link is independently verified and tested.

### Gaps Summary

No gaps. All 16 must-haves across the five plans are verified at existence, substantive, wired, and data-flow levels. The headline requirement TMUX-05 is genuinely wired end-to-end: surviving tmux row → orphaned entry in GET /api/sessions → frontend auto-reattach → `new-session -A` reconnect → real in-memory session the WS layer attaches to, with no Resume button anywhere (D-88). Cleanup (TMUX-08) closes every orphan path — count, online kill on remove/delete, and offline startup sweep. The Done-TTL reaper (REAP-01) is the codebase's first background goroutine, silent, configurable, and keeps agents fully resumable (D-96 verified: zero claude_session_id deletions in the reap path). `go build ./...`, `go test ./...`, frontend `tsc --noEmit`, and `npm run build` all pass green.

---

_Verified: 2026-06-13T09:30:00Z_
_Verifier: Claude (gsd-verifier)_
