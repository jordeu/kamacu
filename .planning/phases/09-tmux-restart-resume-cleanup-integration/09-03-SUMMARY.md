---
phase: 09-tmux-restart-resume-cleanup-integration
plan: 03
subsystem: sessions
tags: [tmux, restart-resume, reconcile, reattach, frontend]
requires:
  - "tmux_sessions table (migration 00005, Phase 8)"
  - "tmux.Client.HasSession liveness probe (Phase 8)"
  - "Manager.SetTmuxClient + tmuxName/killer per-session lifecycle (Phase 8)"
  - "agents.go two-pass reconcile pattern (Phase 5, the D-88 model)"
provides:
  - "GET /api/sessions task-scoped reconcile: surviving tmux rows surface as orphaned bash entries; dead rows lazily GC'd"
  - "POST /api/sessions reattach variant (reattach_tmux_name) reconnecting a survivor by name via new-session -A"
  - "session.Info.Orphaned + session.Info.TmuxName wire fields (omitempty, survivor-only)"
  - "Manager.HasLiveTmux(name) liveness check"
  - "frontend useReattachTmux mutation + TaskPage auto-reattach effect"
affects:
  - "internal/api/sessions.go (list reconcile + create reattach branch)"
  - "internal/session (Info shape, Manager method)"
  - "web/src/pages/TaskPage.tsx (orphaned auto-reattach, ghost filtered from tabs)"
tech-stack:
  added: []
  patterns:
    - "DB-derived survivor pass gated by a liveness probe (mirrors agents.go transcriptExists, swapping in tmux has-session)"
    - "attach-or-create reattach: new-session -A against a persisted name needs no Manager change"
    - "ref-Set one-shot guard for poll/StrictMode-safe auto-reattach"
key-files:
  created: []
  modified:
    - internal/api/sessions.go
    - internal/session/manager.go
    - internal/session/session.go
    - cmd/kangent/main.go
    - internal/api/sessions_test.go
    - internal/api/agents_test.go
    - internal/api/agent_integration_test.go
    - internal/api/integration_test.go
    - internal/ws/integration_test.go
    - web/src/api/sessions.ts
    - web/src/pages/TaskPage.tsx
decisions:
  - "Survivor detection authoritative via Manager.HasLiveTmux(name) — keeps the wire format unchanged (no TmuxName exposed on live sessions) while the reconcile distinguishes covered-vs-survivor rows"
  - "Inconclusive has-session probe (tmux binary broken/hung) never GCs the row (Pitfall 6 honesty) — only a conclusive exit-1 dead session triggers the D-89 lazy DELETE"
  - "Reattach spawn failure NEVER deletes the row (it is a pre-existing survivor, not a freshly reserved n) — diverges from the fresh-spawn release path"
  - "TmuxName carried on the wire ONLY for orphaned entries (omitempty); the frontend keys reattach on it. Live sessions stay tmux-unaware (D-77)"
metrics:
  duration: "14 min"
  completed: "2026-06-13"
  tasks: 3
  files: 11
---

# Phase 9 Plan 3: tmux Restart Auto-Reattach Summary

Restart-surviving tmux sessions reattach automatically and invisibly (TMUX-05, D-88): `GET /api/sessions` reports a survivor row as an `orphaned` bash ghost, the frontend fires one reattach spawn against the persisted name (`new-session -A` reconnects losslessly), and the resulting real tab is indistinguishable from any other bash tab — no Resume button, no banner. Dead-on-restart rows are lazily GC'd and never surface (D-89).

## What Was Built

**Task 1 — reconcile branch (commit 9ba3150).** `GET /api/sessions?task_id=N` now runs a two-pass reconcile mirroring `agents.go`: the manager-derived live sessions, plus a DB-derived pass over `tmux_sessions` rows for the task. A row with NO live in-memory session (`Manager.HasLiveTmux(name)` is false) is probed with `tmux has-session`. Alive ⇒ appended as `session.Info{ID:"", Kind:"bash", Orphaned:true, TmuxName:name}`. Conclusively dead (exit 1) ⇒ `DELETE FROM tmux_sessions` (D-89 lazy GC). Inconclusive probe (tmux binary broken/hung) ⇒ left untouched, surfaces nothing (Pitfall 6). Scoped to task-scoped requests only; unscoped dev lists are untouched. `SessionRoutes` gained a `tmux.Client` parameter; `main.go` hoists `tmuxClient` and threads it into both `mgr.SetTmuxClient` and `SessionRoutes`.

**Task 2 — reattach spawn variant (commit 8c3e1b3).** `POST /api/sessions` accepts `reattach_tmux_name`. It validates bash kind (400 "reattach requires a bash session" for agents) and task presence, verifies the row belongs to the task via `SELECT label FROM tmux_sessions WHERE task_id = ? AND name = ?` (404 "no session to reattach" otherwise), reuses the persisted name, and SKIPs the mint/INSERT path. `Manager.Spawn` already runs `new-session -A` (attach-or-create) for `opts.TmuxName`, so reattach needed no Manager change. A transient spawn failure on a reattach never deletes the row.

**Task 3 — frontend auto-reattach (commit 04b1dad).** `TermSession` gained `orphaned` + `tmuxName`. `useReattachTmux(taskId)` POSTs the reattach and writes the real session into the scoped cache. `TaskPage` fires one reattach spawn per orphaned bash ghost via an effect guarded by a `useRef<Set<string>>` (poll- and StrictMode-safe). Orphaned ghosts are filtered out of `visibleSessions` so they never render as their own tab — the real tab appears on the next poll, connecting via the normal WS path.

## Reasoning Chain (verified)

surviving tmux row → orphaned entry in `GET /api/sessions` → frontend reattach spawn (one per name) → `new-session -A` attaches to the live session → real in-memory session registered → WS `mgr.Get(id)` succeeds → tab connects. Dead-on-restart rows are DELETEd and produce no tab (D-89). No tmux Resume button exists (D-88 / D-77 invisibility).

## Deviations from Plan

None — plan executed as written. Task 1's action was pre-amended by Task 3's note to carry `TmuxName` on the wire for orphaned entries; both the `Orphaned` and `TmuxName` fields were added to `session.Info` in Task 1's commit as the plan specified, so the field landed once, in order.

**Out-of-plan test-harness updates (mechanical, required by the signature change):** `SessionRoutes` gained a fourth parameter, so five additional test call sites were updated to pass `tmux.Client{}` (zero value — those harnesses have no tmux rows, so reconcile is a no-op): `internal/api/agents_test.go`, `internal/api/agent_integration_test.go`, `internal/api/integration_test.go`, `internal/ws/integration_test.go`, and the `newTmuxSessionServer` harness (which passes its real per-test client `c` so the reattach test exercises a live socket). This is a Rule 3 blocking-issue fix (the signature change would not compile otherwise); no behavior change to those tests.

## Verification

- `go build ./...` — exits 0.
- `go test ./internal/api/ ./internal/session/ ./internal/ws/` — all pass (api ~90s, session ~10s, ws ~59s).
- `cd web && npx tsc --noEmit && npm run build` — both exit 0.
- New tests: `TestSessionTmuxReattach` (seeds a row + a detached survivor tmux session, POSTs reattach, asserts 201 + `mgr.Get` finds the session + no duplicate row + 404 on unknown name), `TestSessionTmuxReattachRejectsAgent` (400 bash-only guard). Both skip-guarded on tmux availability.
- D-88 / D-77 audit: `grep -rln "Resume" web/src --include="*.tsx"` → only `AgentTab.tsx` (pre-existing agent Resume) and a TaskPage.tsx comment explicitly stating NO Resume button. No tmux/bash Resume affordance.

## Known Stubs

None. The orphaned `Label` falls back to "Bash ?" only when the persisted label column is empty (a degraded-but-honest case from a Phase 8 back-fill warn-only failure); it is a real survivor entry, not a stub, and resolves to the correct label once the live session replaces the ghost.

## Notes for Downstream

- `web/dist/index.html` (the only git-tracked dist file; `dist/assets/` is gitignored) was regenerated by the Task 3 `npm run build` and was already dirty at plan start. It was deliberately left unstaged per the parallel-execution rule (stage only plan files); the orchestrator rebuilds and commits dist once after all agents complete.
- The reconcile and reattach are the two halves of TMUX-05; cleanup-gate integration (TMUX-08) and the Done-TTL reaper (REAP-01) remain in sibling plans 09-04 / 09-05.

## Self-Check: PASSED

All claimed files exist (SUMMARY + 4 modified source files spot-checked) and all three task commits are present (9ba3150, 8c3e1b3, 04b1dad).
