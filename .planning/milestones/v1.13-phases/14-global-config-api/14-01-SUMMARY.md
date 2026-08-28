---
phase: 14-global-config-api
plan: 01
subsystem: api
tags: [go, http, sqlite, gh, tmux, global-config, gated-409]

requires:
  - phase: 13-global-data-foundation-safety-net
    provides: global_task singleton (00017), scope-aware tmux_sessions (00018), BackfillGlobalTask boot hook, agents delete-guard
provides:
  - GET /api/global — config + derived state (root_exists, live counts, agent summary) wire shape (D-17)
  - PUT /api/global — single partial endpoint: folder root (GCONF-01), managed clone into ~/.kamacu/repos/global/<owner>/<name> (GCONF-02), clear (D-21), default agent (GCONF-03)
  - Forward-wired live-session 409 gate (globalLiveBlockers) + resume-id clearing on root change (GCONF-04/D-16)
  - Test harness newGlobalTestServer/newGlobalTestServerWithTmux reusable by plan 14-02
affects: [15-sessions-backend, 16-view-settings-bar, 17-hardening-e2e]

tech-stack:
  added: []  # zero new dependencies — locked
  patterns:
    - "XxxRoutes per-resource constructor (GlobalRoutes) registered from serve.go, routes.go untouched"
    - "Gate-hoisted managed-clone ordering: 409 -> parse -> gh-validate -> dest -> reattach-or-clone -> single UPDATE -> GET-shape response"
    - "deleteBlocker {kind,target} grammar reused verbatim for the config-change 409 (D-15 locked reading)"

key-files:
  created:
    - internal/api/global.go
    - internal/api/global_test.go
  modified:
    - cmd/kamacu/serve.go

key-decisions:
  - "Footgun tests git-init the trap dirs (home, ~/.kamacu, child) so the D-07/D-27 branches themselves fire instead of stopping at validateRepoPath's git check"
  - "Clone-failure inline error surfaces github.Clone's trimmed-stderr contract ('fatal: ' stripped) — same as createByRepo, not err.Error()"
  - "TestPutGlobalAgent inserts a third 'Codex CLI' agent — 00015 already seeds the OpenCode system agent, so the name collides"

patterns-established:
  - "Singleton-resource handler: GET/PUT pair over a guaranteed row, ErrNoRows fails loud 500 'global task row missing'"
  - "Branch-first partial-PUT dispatch: all-nil 400 -> both-400 -> repo-empty guidance -> gate -> variant resolution -> single UPDATE"

requirements-completed: [GCONF-01, GCONF-02, GCONF-03, GCONF-04]

coverage:
  - id: D1
    description: "GET /api/global serves the D-17 wire shape — stored config + root_exists + live counts + agent summary — with resume ids never serializable (D-18)"
    requirement: GCONF-01
    verification:
      - kind: unit
        ref: internal/api/global_test.go#TestGetGlobalUnconfigured
        status: pass
      - kind: unit
        ref: internal/api/global_test.go#TestGetGlobalConfiguredRoot
        status: pass
    human_judgment: false
  - id: D2
    description: "PUT folder variant: validateRepoPath git-repo requirement + D-07 home//- footgun equality + D-27 ~/.kamacu equality-and-prefix block, each 400 leaving the row untouched"
    requirement: GCONF-01
    verification:
      - kind: unit
        ref: internal/api/global_test.go#TestPutGlobalFolder
        status: pass
      - kind: unit
        ref: internal/api/global_test.go#TestPutGlobalFootguns
        status: pass
    human_judgment: false
  - id: D3
    description: "PUT clear + agent + id lifecycle: root_path:\"\" clears both root columns; agent_id existence-validated 400 with no gate; resume ids cleared only on root changes (D-16/D-25)"
    requirement: GCONF-03
    verification:
      - kind: unit
        ref: internal/api/global_test.go#TestPutGlobalClear
        status: pass
      - kind: unit
        ref: internal/api/global_test.go#TestPutGlobalAgent
        status: pass
      - kind: unit
        ref: internal/api/global_test.go#TestPutGlobalEmptyBody
        status: pass
      - kind: unit
        ref: internal/api/global_test.go#TestPutGlobalRootChangeClearsResumeIDs
        status: pass
    human_judgment: false
  - id: D4
    description: "PUT managed variant: gh-validated clone into ~/.kamacu/repos/global/<owner>/<name>, atomic failure (prior config survives, no residue), same-repo reattach without re-clone, full repo/root_path dispatch grammar"
    requirement: GCONF-02
    verification:
      - kind: unit
        ref: internal/api/global_test.go#TestPutGlobalManagedClone
        status: pass
      - kind: unit
        ref: internal/api/global_test.go#TestPutGlobalManagedValidateFail
        status: pass
      - kind: unit
        ref: internal/api/global_test.go#TestPutGlobalManagedCloneFailureAtomic
        status: pass
      - kind: unit
        ref: internal/api/global_test.go#TestPutGlobalManagedReattach
        status: pass
      - kind: unit
        ref: internal/api/global_test.go#TestPutGlobalManagedReattachMismatch
        status: pass
      - kind: unit
        ref: internal/api/global_test.go#TestPutGlobalRepoDispatch
        status: pass
    human_judgment: false
  - id: D5
    description: "Forward-wired live-session 409 gate on root change/clear (globalLiveBlockers over scope='global' tmux rows + exact-match HasSession probe; manager half a zero-count seam until Phase 15)"
    requirement: GCONF-04
    verification: []
    human_judgment: true
    rationale: "The live-tmux 409-trip needs a real detached tmux session — that heavyweight test (TestPutGlobalRootBlockedByLiveTmux) is plan 14-02's scope per this plan's task notes; 14-01 exercises the gate only trivially (zero rows)."
  - id: D6
    description: "Production wiring: api.GlobalRoutes(mux, db, mgr, tmuxClient) registered in serve.go's route registry; routes.go untouched"
    verification:
      - kind: other
        ref: "grep 'api.GlobalRoutes(mux, db, mgr, tmuxClient)' cmd/kamacu/serve.go"
        status: pass
      - kind: other
        ref: "go build ./... && go vet ./internal/api/ ./cmd/kamacu/"
        status: pass
    human_judgment: false

duration: 23 min
completed: 2026-08-25
status: complete
---

# Phase 14 Plan 01: Global config API Summary

**GET/PUT /api/global over the Phase-13 singleton: validated folder root, gh-validated managed clone into ~/.kamacu/repos/global/&lt;owner&gt;/&lt;name&gt; with reattach and atomic failure, settable default agent, and the forward-wired live-session 409 gate with resume-id clearing — zero new dependencies**

## Performance

- **Duration:** 23 min
- **Started:** 2026-08-25T17:34:44Z
- **Completed:** 2026-08-25T17:57:58Z
- **Tasks:** 3
- **Files modified:** 3 source (+1 docs: deferred-items.md)

## Accomplishments

- GET /api/global serves the full D-17 wire shape (root_path, github_repo nullable marker, agent_id, updated_at, root_exists via os.Stat, live{agent,bash,tmux} counts, agent{id,name,engine} JOIN summary) from a migrated fresh DB — resume ids structurally never serialize (D-18, key-absence asserted)
- PUT /api/global implements the complete D-20 dispatch grammar: folder variant (validateRepoPath + D-07 home//- equality set + D-27 ~/.kamacu equality-and-prefix block with the WHY copy), the root_path:"" clear (D-21), agent_id with existence-validated 400 and no gate (D-24/D-25), and the managed variant (ParseRepoRef → ValidateRepo degrade copy → three-segment global namespace → reattachManaged verbatim → Clone with RemoveAll belt-and-braces)
- Every root change — including the clear — clears claude_session_id/opencode_session_id in the SAME single UPDATE (D-16); agent-only PUTs never touch them (D-25)
- globalLiveBlockers 409 gate reuses the deleteBlocker {kind,target} grammar verbatim, gates before any gh/clone work, and ships the manager half as a documented Phase-15 zero-count seam

## Task Commits

Each task was committed atomically:

1. **Task 1: Wire types, GlobalRoutes, GET handler, serve.go wiring, harness, GET tests** - `9f5959d` (feat)
2. **Task 2: 409 gate + PUT dispatch grammar, folder variant, clear, agent, single UPDATE** - `a8e8be8` (feat)
3. **Task 3: Managed-clone variant — repo dispatch, atomic ordering, reattach** - `6df4471` (feat)

**Plan metadata:** (see final docs commit below)

## Files Created/Modified

- `internal/api/global.go` - NEW: globalConfig/globalLive/globalAgent wire types, globalHandlers, GlobalRoutes, liveGlobalTmuxNames probe, globalLiveBlockers gate, loadGlobalConfig/deriveGlobalState shared loader, get + put handlers, putManagedRoot managed flow
- `internal/api/global_test.go` - NEW: newGlobalTestServer/newGlobalTestServerWithTmux harness + 14 tests (2 GET, 6 PUT root/agent, 6 managed/dispatch)
- `cmd/kamacu/serve.go` - one api.GlobalRoutes registration line in the registry block
- `.planning/phases/14-global-config-api/deferred-items.md` - pre-existing issue log (see Deviations)

## Decisions Made

- Footgun tests git-init the trap dirs (home, ~/.kamacu, ~/.kamacu/repos/foo) so the D-07/D-27 branches themselves fire — a plain dir would stop at validateRepoPath's git check and never reach the dedicated footgun code
- Clone-failure inline error surfaces github.Clone's trimmed-stderr contract ("fatal: " stripped), mirroring createByRepo — the fake's err.Error() is only the empty-stderr fallback
- TestPutGlobalAgent inserts a third "Codex CLI" agent — 00015 already seeds the OpenCode system agent, so the obvious name collides on the NOCASE unique index
- updated_at-bump test sleeps 10ms past SQLite's millisecond timestamp resolution so the bump is deterministic, not a same-millisecond race

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Test asserted the wrong clone-failure message string**
- **Found during:** Task 3 (TestPutGlobalManagedCloneFailureAtomic)
- **Issue:** Test expected `err.Error()` ("clone failed") but the handler surfaces github.Clone's returned error, whose message is the trimmed stderr with "fatal: " stripped ("repository not found") per Clone's documented contract
- **Fix:** Asserted the stderr-derived message (the real user-facing string); the handler code was already correct
- **Files modified:** internal/api/global_test.go
- **Verification:** test passes; message matches Clone's contract in internal/github/clone.go
- **Committed in:** 6df4471 (Task 3 commit)

**2. [Rule 1 - Bug] Second-agent test INSERT collided with the 00015 seed**
- **Found during:** Task 2 (TestPutGlobalAgent)
- **Issue:** INSERT of an 'Opencode' agent failed the NOCASE unique index — migration 00015 already seeds the OpenCode system agent on every fresh DB
- **Fix:** Insert a distinctly-named 'Codex CLI' agent and assert engine 'codex'
- **Files modified:** internal/api/global_test.go
- **Verification:** test passes
- **Committed in:** a8e8be8 (Task 2 commit)

---

**Total deviations:** 2 auto-fixed (2 bug — both test-side corrections, production code unchanged from plan)
**Impact on plan:** None — both were test-expectation fixes discovered by running the plan's own verification commands.

## Issues Encountered

- Three pre-existing session-input test failures (TestInput_Happy_WritesAndAppendsCR, TestInput_TrailingLF_TranslatedToCR, TestInput_EmptyMessage_WritesBareCR — "bytes_written = 15, want 13") verified to fail identically at the pre-phase baseline commit ddd00a4 via a throwaway worktree: NOT caused by this plan. Logged to deferred-items.md with the pre-existing gofmt drift in serve.go and the ~190s full-suite runtime note. The plan's targeted verification (`go test ./internal/api/ -run 'TestGetGlobal|TestPutGlobal' -count=1`) is fully green.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Route pair GET/PUT /api/global is live on the real server (serve.go wiring); curl-complete per the research smoke script
- Plan 14-02 (verification plan) builds TestPutGlobalRootBlockedByLiveTmux and TestPutGlobalConfigHistory on the newGlobalTestServerWithTmux harness shipped here
- Phase 15's spawn path consumes the singleton root/agent via the same load path and widens globalLiveBlockers' manager half with ListGlobal()
- No blockers

## Self-Check: PASSED

- internal/api/global.go, internal/api/global_test.go exist; cmd/kamacu/serve.go modified
- Commits 9f5959d, a8e8be8, 6df4471 present on task/add-global-session-301
- `go test ./internal/api/ -run 'TestGetGlobal|TestPutGlobal' -count=1` green (16 tests); `go build ./...` and `go vet ./internal/api/ ./cmd/kamacu/` clean
- No file deletions in any task commit; acceptance criteria greps for all three tasks re-verified

---
*Phase: 14-global-config-api*
*Completed: 2026-08-25*
