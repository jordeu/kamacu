---
phase: 08-tmux-shells-spawn-detach-lifecycle
verified: 2026-06-13T07:20:00Z
status: passed
score: 6/6 success criteria verified (all 6 Phase 8 requirement IDs satisfied)
re_verification: false
---

# Phase 8: tmux Shells — Spawn & Detach Lifecycle Verification Report

**Phase Goal:** User can pick tmux as the bash-tab shell and get invisibly durable shells: tabs look and behave exactly like plain bash tabs (× kills), sessions survive leaving the task view — and (Phase 9) a server restart — and nothing about plain bash or agent sessions changes.

**Verified:** 2026-06-13T07:20:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (Success Criteria from ROADMAP.md)

| #   | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| 1   | With tmux on PATH, "tmux" appears in the shell dropdown; absent without tmux (TMUX-01) | ✓ VERIFIED | `settings.AllowedShells()` (validate.go:18-24) appends "tmux" only on `exec.LookPath("tmux")` success; consumed by both `Validate` (validate.go:41) and `entryFor` (api/settings.go:26) — one source of truth. Tests assert both PATH states at validation + HTTP layers. |
| 2   | tmux shell spawns attach-or-create `kangent-<task>-<n>` on `-L kangent` socket with worktree cwd (TMUX-02) | ✓ VERIFIED | `NewSessionArgs` (tmux.go:62) builds `new-session -A -s <name> -c <dir>` with `-L`/`-f` injected by `BaseArgs`; handler mints name via `COALESCE(MAX(n),0)+1` (sessions.go:172,176); manager spawns under creack/pty (manager.go:199). Live integration test `TestSessionTmuxSpawnHappyPath` confirms 201 + real `HasSession==true`. |
| 3   | Leaving + reopening task view reattaches cleanly; tab indistinguishable, no status bar/badge (TMUX-03, amended) | ✓ VERIFIED (code) / ? HUMAN (repaint feel) | TMUX-03 needs no new code: Detach never touches the PTY (TERM-05); config `set -g status off` (tmux.go:29); `Info` struct has zero tmux fields (session.go:44-57, D-77). Clean-repaint visual confirmed in 08-04 human checkpoint. |
| 4   | Closing a tmux tab (×) kills the tmux session — kill-on-close parity (TMUX-04, amended) | ✓ VERIFIED | Killer-first `Stop()` (session.go:429-444) invokes `KillSession` closure (manager.go:306) before any signal; `TestTmuxStopKillsSession` asserts `HasSession==false` after Stop; HTTP-layer `TestSessionTmuxSpawnHappyPath` doubles as ×-kill proof. |
| 5a  | Shell exit inside tmux shows existing exited state, not "resumable" (TMUX-06) | ✓ VERIFIED | `waitExit` (session.go:150-163) probes `has-session` between Wait and `markExited`; alive→`detachedAlive=true` but still falls through to exited banner. `TestTmuxInnerExit` + detach-alive test cover both branches. |
| 5b  | Plain-bash + agent sessions keep kill-on-stop unchanged, regression-guarded (TMUX-07) | ✓ VERIFIED | nil killer ⇒ `Stop` executes byte-identical signal path (session.go:445-450). Killer assigned ONLY when `TmuxName != ""` (manager.go:300-307). Pre-existing `session_test.go`/`manager_test.go` UNTOUCHED in Phase 8 (last changed phases 02/03/06). Lifecycle test asserts nil-killer for bash-default, bash-settings, agent. |

**Score:** 6/6 success criteria verified (truth 3's visual repaint confirmed by 08-04 human checkpoint; all automated layers pass).

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/tmux/tmux.go` | Leaf pkg: DefaultSocket, Config, WriteConfig, Client{BaseArgs,NewSessionArgs,HasSession,KillSession,KillServer} | ✓ VERIFIED | 129 lines; stdlib-only imports (leaf invariant holds — `grep kangent/internal` empty); `"="` exact-match on 4 targets; 5s bounded ctx; `-A` present, `-d` absent from NewSessionArgs. |
| `internal/tmux/tmux_test.go` | argv + real-tmux integration tests | ✓ VERIFIED | `go test ./internal/tmux -count=1` green (0.213s); prefix-collision (kangent-1-10), idempotent kill, exec-error discrimination, KillServer cleanup all present. |
| `internal/store/migrations/00005_tmux_sessions.sql` | identity-only table | ✓ VERIFIED | `CREATE TABLE tmux_sessions` with `name UNIQUE`, `UNIQUE(task_id,n)`, no status column (only in comment). Goose Up/Down present; store tests apply it on fresh DB. |
| `internal/settings/validate.go` | `AllowedShells()` call-time LookPath func | ✓ VERIFIED | var→func conversion complete (`var AllowedShells` absent); `exec.LookPath("tmux")`; `range AllowedShells()` in Validate; `"Unknown shell."` copy byte-identical. |
| `internal/api/settings.go` | `entryFor` calls `AllowedShells()` | ✓ VERIFIED | settings.go:26 `e.Options = settings.AllowedShells()`. |
| `internal/session/session.go` | tmuxName/tmuxClient/killer/detachedAlive; killer-first Stop; has-session waitExit | ✓ VERIFIED | All four fields present (lines 74-76, 100); Stop killer-first (429); waitExit probe before markExited (158); Info() wire shape unchanged (no tmux json). |
| `internal/session/manager.go` | SpawnOpts.TmuxName, SetTmuxClient, tmux branch, ErrTmuxNotFound | ✓ VERIFIED | ErrTmuxNotFound exported (29); TmuxName (68); SetTmuxClient (82); tmux branch LookPath-before-PTY (195-197); env allow-list scrubs TMUX/TMUX_PANE (205-213); bash-only guard (122); raw `shell=="tmux"` guard (224). |
| `internal/session/tmux_lifecycle_test.go` | TMUX-07 guard + kill/exit/detach/D-84 tests | ✓ VERIFIED | 218 lines; nil-killer regression (bash×2, agent), kill, inner-exit, detach-client, ErrTmuxNotFound, KillServer cleanup all present. |
| `cmd/kangent/main.go` | WriteConfig at startup + SetTmuxClient | ✓ VERIFIED | `kangent-tmux.conf` write (71-72); `mgr.SetTmuxClient(tmux.Client{Socket: tmux.DefaultSocket, ...})` (114). |
| `internal/api/sessions.go` | shell==tmux branch: name minting, D-84 mapping, row cleanup | ✓ VERIFIED | dev-route 409 (163); MAX(n)+1 mint (172,176); INSERT-before-spawn (177); DELETE on failure (191); ErrTmuxNotFound→409 (195-198); warn-only label back-fill (215). Response is `sess.Info()` (D-77). |
| `web/src/pages/TaskPage.tsx` | 409 ApiError verbatim, else generic | ✓ VERIFIED | `grep tmux == 0` (tmux-unaware, D-77); `ApiError` imported (5); `status === 409 ? message : "Couldn't start a session. Try again."` (375-377); tsc compiles clean. |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| tmux.go | session targets | `"="+name` exact match | ✓ WIRED | 4 occurrences on has-session + kill-session targets. |
| migrate.go | 00005 sql | embedded `migrations/*.sql` glob | ✓ WIRED | Auto-picked-up; store + api suites apply migration on fresh DB. |
| api/settings.go | validate.go | `AllowedShells()` shared fn | ✓ WIRED | Dropdown options + save validation share one LookPath truth. |
| session.go | internal/tmux | killer→KillSession; waitExit→HasSession | ✓ WIRED | Both present and invoked at the documented sites. |
| manager.go | internal/tmux | Spawn builds cmd from NewSessionArgs | ✓ WIRED | manager.go:199. |
| main.go | internal/tmux | WriteConfig + Client{DefaultSocket,conf} | ✓ WIRED | main.go:71-72,114. |
| sessions.go | tmux_sessions table | `COALESCE(MAX(n),0)+1` then INSERT | ✓ WIRED | sessions.go:172,176-177. |
| TaskPage.tsx | api/client.ts | ApiError status/message on spawn error | ✓ WIRED | TaskPage.tsx:5,375. |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| sessions.go tmux name | `n` | `SELECT COALESCE(MAX(n),0)+1 FROM tmux_sessions` (real DB query) | Yes | ✓ FLOWING |
| session killer | `KillSession` result | real `tmux kill-session` exec, verified by post-Stop `HasSession==false` in test | Yes | ✓ FLOWING |
| waitExit discriminator | `detachedAlive` | real `tmux has-session` probe, verified true/false in detach vs exit tests | Yes | ✓ FLOWING |
| TaskPage error span | `spawn.error.message` | server `writeError` 409 body, asserted verbatim in handler tests | Yes | ✓ FLOWING |
| settings dropdown | `AllowedShells()` | live `exec.LookPath("tmux")`, asserted both PATH states | Yes | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Full Go suite green | `go test ./...` | all packages ok | ✓ PASS |
| tmux pkg fresh | `go test ./internal/tmux -count=1` | ok 0.213s | ✓ PASS |
| session pkg fresh (real tmux lifecycle) | `go test ./internal/session -count=1` | ok 5.330s | ✓ PASS |
| api pkg fresh (real tmux spawn/kill/D-84) | `go test ./internal/api -count=1` | ok 81.388s | ✓ PASS |
| settings pkg fresh (PATH states) | `go test ./internal/settings -count=1` | ok 0.258s | ✓ PASS |
| Frontend type-checks | `npx tsc -b --noEmit` | exit 0 | ✓ PASS |
| No stop-time kind-branching | `grep -rn isTmux internal/session/` | none | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| TMUX-01 | 08-02 | tmux option offered only when on PATH | ✓ SATISFIED | AllowedShells() LookPath; both call sites share it; tests cover both states. |
| TMUX-02 | 08-01, 08-04 | Spawn `kangent-<task>-<n>` on `-L kangent`, worktree cwd | ✓ SATISFIED | NewSessionArgs + DB-minted name + creack/pty spawn; happy-path integration test. |
| TMUX-03 (amended) | 08-03, 08-04 | Implicit detach on leaving task view; reattach on reopen | ✓ SATISFIED | Zero-code by design (Detach never touches PTY); session survives within server lifetime; human-confirmed clean repaint. |
| TMUX-04 (amended) | 08-03, 08-04 | × kills the tmux session (kill-on-close parity) | ✓ SATISFIED | Killer-first Stop → kill-session; HasSession false after Stop (unit + HTTP tests). |
| TMUX-06 | 08-03 | Inner exit shows exited (not resumable); exit-vs-detach via has-session | ✓ SATISFIED | waitExit has-session probe; inner-exit + detach-alive tests. |
| TMUX-07 | 08-03 | Bash/agent keep kill-on-stop, regression-guarded | ✓ SATISFIED | nil killer ⇒ identical signal path; pre-existing test files untouched; nil-killer assertions for bash×2 + agent. |

All 6 Phase 8 requirement IDs accounted for and satisfied. No ORPHANED requirements: REQUIREMENTS.md maps exactly TMUX-01/02/03/04/06/07 to Phase 8 (TMUX-05 correctly mapped to Phase 9).

### Anti-Patterns Found

None blocking. Scan of the 10 phase files surfaced no stubs, placeholders, or hollow returns. The handler's `INSERT ... VALUES` and `SELECT COALESCE(MAX(n),0)+1` are real DB operations; the killer closure executes a real tmux exec; `return null`-style stubs are absent. The `web/dist/index.html` uncommitted change shows an empty diff stat (no-op rebuilt asset), not a regression.

### Scope Boundary Confirmation (NOT a gap)

The known limitation that a tmux session **survives a server restart but its tab disappears** is CORRECT and EXPECTED for Phase 8. `tmux_sessions` rows are persisted (INSERT on every spawn, label back-filled) but intentionally NOT read at startup, and `Manager.ListByTask` is in-memory only. This is TMUX-05, explicitly scoped to Phase 9, and documented in 08-04-SUMMARY.md's "Known Limitations / Phase 9 Seam" section. The persisted table is the seam Phase 9 builds on with no schema change. NOT flagged as a gap.

### Human Verification Required

None outstanding for sign-off. The 08-04 blocking human-verify checkpoint was completed and approved (08-04-SUMMARY.md: indistinguishable tab/no status bar, clean reattach repaint, mouse-wheel scrollback, ×-kill parity all confirmed). The one MEDIUM-confidence item (mouse-wheel scrollback reaching tmux history, D-80) was validated during that checkpoint.

### Gaps Summary

No gaps. Every Phase 8 success criterion is satisfied at the code level and proven green by the test suite (`go test ./...` all ok, including the 81s real-tmux api integration tests). All 6 requirement IDs (TMUX-01/02/03/04/06/07) are accounted for and satisfied. The amended TMUX-03 (implicit detach on leaving the task view, not tab-close) and amended TMUX-04 (× KILLS the tmux session) are both implemented per the amended 2026-06-12 wording. TMUX-07's regression contract is doubly proven: explicit nil-killer assertions AND the untouched pre-existing Stop test suite staying green. D-77 indistinguishability holds at both the API wire shape (`Info` struct has no tmux field) and the frontend (`grep tmux == 0` in TaskPage.tsx). The Phase 9 restart-reconcile seam is correctly out of scope.

---

_Verified: 2026-06-13T07:20:00Z_
_Verifier: Claude (gsd-verifier)_
