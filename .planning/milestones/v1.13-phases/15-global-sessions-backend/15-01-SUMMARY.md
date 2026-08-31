---
phase: 15-global-sessions-backend
plan: 01
subsystem: api
tags: [go, http, sqlite, tmux, pty, sessions, global-scope, gated-409]

requires:
  - phase: 14-global-config-api
    provides: GET/PUT /api/global singleton config + globalLiveBlockers/deleteBlocker grammar + newGlobalTestServer harness
  - phase: 13-global-data-foundation-safety-net
    provides: global_task singleton (00017), tmux_sessions scope discriminator (00018)
provides:
  - session.SpawnOpts.Global / Session.global / Info.Global (json global,omitempty) — the additive third scope
  - session.Manager.ListGlobal() + StopAllForScope() (scope-targeted stop, never the task-id zero fan-out)
  - POST /api/sessions {scope:"global"} — root gates (D-28/D-29), plain bash, tmux mint kamacu-global-<n>, scoped reattach (D-32)
  - GET /api/sessions?scope=global — engine list + scoped tmux reconcile with orphaned Global:true survivors
  - Honest synthesized labels Scratchpad/Global/agentName on scoped AND unscoped lists + getSession (GINT-02)
  - GET /api/global live.agent/live.bash derived from the engine (D-19 seams closed); GCONF-04 root-change 409 bites on live global PTYs
  - sessions_global_test.go + newGlobalSessionServerWithTmux harness reusable by plan 15-02
affects: [15-plan-02-agent-status, 16-view-settings-bar, 17-hardening-e2e]

tech-stack:
  added: []  # zero new dependencies
  patterns:
    - "Scope discriminator rides the Kind/Shell/TmuxName zero-value precedent: false = task/dev behavior byte-identical"
    - "One gate block before kind dispatch covers bash/tmux/agent uniformly (D-30) — gates at the singleton resolution point, exactly where the task worktree query feeds every kind"
    - "Scoped reattach = scoped SQL, not scope-mismatch branching: a foreign row simply does not resolve → same honest 404 both directions (D-32, no oracle)"

key-files:
  created:
    - internal/api/sessions_global_test.go
  modified:
    - internal/session/manager.go
    - internal/session/session.go
    - internal/session/session_test.go
    - internal/api/sessions.go
    - internal/api/global.go
    - internal/api/sessions_test.go  # deviation — see deviations

key-decisions:
  - "Global label arm sits AFTER the KindAgent arm, BEFORE the TaskID-zero dev arm (Pitfall 2 ordering) — global bash tabs get \"Bash 1\"/\"Bash 2\" from a dedicated globalCounter, never the dev 'bash #N' family"
  - "Global tmux mint counter is scope-scoped SQL (MAX(n)+1 WHERE scope='global'), INSERT with (NULL task_id, 'global') — the 00018 XOR shape; name UNIQUE is the real mint-race backstop (NULL task_ids are DISTINCT under UNIQUE(task_id,n))"
  - "deriveGlobalState Live.Agent/Live.Bash disjoint-by-name resolution of OQ1: KindAgent → agent; TmuxName==\"\" bash → bash; minted tabs → tmux only"
  - "globalLiveBlockers dedupes engine sessions against the tmux half by name — each live surface listed exactly once in the 409 reasons"
  - "Pitfall 3 structural guard: global labels synthesized per-entry in the sessionDetail loop + getSession gated on info.Global; joinSessionContext query text unchanged, ctxByTask never gains a 0-keyed entry (dev-session empty-label regression test)"
  - "Test stop-wait deadlines are 8s, not 5s: interactive bash ignores SIGTERM, so the D-14 grace must elapse before the SIGKILL lands (~5.3s observed)"

patterns-established:
  - "Scoped reconcile sibling: reconcileGlobalTmux mirrors reconcileTmux with the WHERE on scope='global' — survivor/lazy-GC/inconclusive-probe semantics identical"
  - "Per-entry context synthesis for scopeless resources: one singleton JOIN per request (globalSessionContext), degrade-to-empty on error, labels stay honest"

requirements-completed: [GSESS-01, GSESS-03, GVIEW-03, GINT-02]

coverage:
  - id: T1
    description: "Engine global scope: labels from the dedicated counter, ListGlobal filter, StopAllForScope isolation (dev+task survive), Info.Global omitempty serialization"
    requirement: GSESS-01
    verification:
      - kind: unit
        ref: internal/session/session_test.go#TestGlobalScopeLabels
        status: pass
      - kind: unit
        ref: internal/session/session_test.go#TestListGlobal
        status: pass
      - kind: unit
        ref: internal/session/session_test.go#TestStopAllForScope
        status: pass
    human_judgment: false
  - id: T2a
    description: "D-28/D-30: scope:global with unconfigured root is 409 \"global root not configured\" for bash AND tmux kinds; D-29 vanished root carries the path verbatim"
    requirement: GVIEW-03
    verification:
      - kind: unit
        ref: internal/api/sessions_global_test.go#TestGlobalSessionUnconfiguredRoot409
        status: pass
      - kind: unit
        ref: internal/api/sessions_global_test.go#TestGlobalSessionVanishedRoot409
        status: pass
    human_judgment: false
  - id: T2b
    description: "Configured root: plain bash spawns 201 with cwd == root (pwd-marker proof via the real input endpoint), global:true, \"Bash N\" family; tmux mints kamacu-global-<n> with (NULL,'global') rows; reattach keeps the persisted label; cross-scope 404 both directions (D-32)"
    requirement: GVIEW-03
    verification:
      - kind: unit
        ref: internal/api/sessions_global_test.go#TestGlobalSessionPlainBashSpawn
        status: pass
      - kind: unit
        ref: internal/api/sessions_global_test.go#TestGlobalSessionTmuxMint
        status: pass
      - kind: unit
        ref: internal/api/sessions_global_test.go#TestGlobalSessionTmuxReattach
        status: pass
      - kind: unit
        ref: internal/api/sessions_global_test.go#TestGlobalSessionCrossScopeReattach404
        status: pass
    human_judgment: false
  - id: T3a
    description: "?scope=global returns exactly the global sessions with Scratchpad/Global/agentName; unscoped list (MCP surface) carries the same labels; dev sessions keep EMPTY join fields (Pitfall 3)"
    requirement: GINT-02
    verification:
      - kind: unit
        ref: internal/api/sessions_global_test.go#TestGlobalSessionListScopedLabels
        status: pass
    human_judgment: false
  - id: T3b
    description: "GET /api/global live.bash ≥ 1 while a global bash runs (agent 0); PUT root change 409 with reasons while RUNNING, 200 after stop (GCONF-04 bite)"
    requirement: GSESS-01
    verification:
      - kind: unit
        ref: internal/api/sessions_global_test.go#TestGlobalSessionLiveCountsAndRootGate
        status: pass
    human_judgment: false
  - id: T3c
    description: "Restart-sim: fresh Manager over the same DB surfaces a surviving global tmux row as orphaned:true + global:true + label; counts only under live.tmux (GSESS-03)"
    requirement: GSESS-03
    verification:
      - kind: unit
        ref: internal/api/sessions_global_test.go#TestGlobalSessionRestartOrphan
        status: pass
    human_judgment: false

deviations:
  - id: DEV-1
    summary: "Fixed pre-existing wire bug in wrapInputForWrite (commit 67f39ef): bracketed-paste delimiters were ESC[2004~/ESC[2014~ (mode number confused with delimiter) — leaked a stray '~' into readline command lines, corrupting EVERY plain-bash submission via POST /api/sessions/{id}/input"
    reason: "The plan's behavior spec requires the cwd proof 'via POST /api/sessions/{id}/input writing pwd to a file' — unsatisfiable with the malformed delimiters; TestInput_Happy was also latently red on readline-bash hosts (bytes_written 36 vs 34)"
    scope: "internal/api/sessions.go (3 comment lines + constants), internal/api/sessions_test.go (TestWrapInputForWrite constants + TestInput_Happy comment arithmetic)"
    test_impact: "TestWrapInputForWrite and TestInput_Happy updated in lockstep; both green; spec-correct markers are the same 6-byte length the tests' arithmetic always assumed"
  - id: DEV-2
    summary: "Test stop-wait deadline 8s instead of 5s (D-14 termGrace)"
    reason: "Interactive bash ignores SIGTERM; exit lands after the 5s grace SIGKILL (~5.3s observed)"
    scope: internal/api/sessions_global_test.go
    test_impact: none

task-completion:
  - task: 1
    status: complete
    commits: [c34d748, 7a2be80]
  - task: 2
    status: complete
    commits: [57f7920, 67f39ef, a0f666f]
  - task: 3
    status: complete
    commits: [ed593eb, 4d94a96]

verification:
  - "go test ./internal/... -count=1 — ALL packages green (api 205s, session, ws, mcp, ...)"
  - "Acceptance greps: Get(\"scope\")=1; ListGlobal in global.go=5; zero-seam assignments gone; D-28/D-29 strings present; WHERE scope='global' AND name=1; kamacu-global-%d=1; StopAllForTask(0) zero code matches (comment reworded); label arm ordering KindAgent < opts.Global < dev arm"
  - "Curl story (plan verification) deferred to Phase 17 E2E per roadmap; API-level coverage via TestGlobalSession* suite incl. tmux-guarded mint/reattach/restart on this host (tmux present)"
