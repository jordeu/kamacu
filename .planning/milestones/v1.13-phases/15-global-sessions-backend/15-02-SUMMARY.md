---
phase: 15-global-sessions-backend
plan: 02
subsystem: api
tags: [go, http, sqlite, pty, sessions, global-scope, agent-spawn, status-feed, opencode-capture]

requires:
  - phase: 15-global-sessions-backend (plan 01)
    provides: Info.Global / SpawnOpts.Global / Manager.ListGlobal(), the scope branch's root gates (D-28/D-29), putGlobalFolderRoot + newGlobalSessionServerWithTmux harness
  - phase: 14-global-config-api
    provides: PUT /api/global agent_id read-at-use surface (D-24) exercised by the tests
provides:
  - Global AGENT spawn: POST /api/sessions {scope:"global", kind:"agent"} behind D-33 gate order, D-34 one-agent 409, engine-branched D-31 resume, csid persist to the global_task singleton
  - captureOpencodeSessionAsync parametrized persist target (taskPersistTarget/globalPersistTarget closures — T-15-06, one poller, never forked)
  - /api/agents/status widened: ONE synthesized source:"global" entry in BOTH passes (manager-derived live + DB-derived post-restart), zero ids, locked Scratchpad/Global labels — the Phase-16 discriminator (SC3 closed)
  - globalResumeState — one singleton read per status call, engine-branched resumable derivation over root_path (never worktree_path)
  - Host-gated real-opencode capture e2e (LookPath skip guard) + CI stub re-target test
affects: [16-view-settings-bar, 17-hardening-e2e]

tech-stack:
  added: []  # zero new dependencies
  patterns:
    - "Persist-target closures: a wrong-table write is structurally inexpressible (no task-id sentinel can reach the singleton UPDATE)"
    - "Hoisted singleton read before an open rows loop — MaxOpenConns(1) makes a second concurrent query on the single SQLite connection a deadlock"
    - "Synthesized status entries collected OUTSIDE the task-keyed map (Pitfall 3) and gated on !hasGlobalAgent for at-most-one (Pitfall 1)"

key-files:
  created: []
  modified:
    - internal/api/sessions.go
    - internal/api/agents.go
    - internal/api/sessions_global_test.go
    - internal/api/agent_integration_test.go
    - internal/api/testdata/fake-claude

key-decisions:
  - "Pass 2b's singleton read is HOISTED before the task DB pass opens its rows: store.Open sets MaxOpenConns(1), so the plan's literal placement (read after the task loop, whose rows close via defer) would deadlock the single connection. One shared read per status call instead of a conditional one — behaviorally identical, structurally safe"
  - "Capture re-target implemented as opencodePersistTarget closures (taskPersistTarget/globalPersistTarget) over separate SQL — the T-15-06 shape; owner string replaces taskID in the poller's log lines"
  - "Host e2e drives the first turn out-of-band via `opencode run` in the root (the manual-terminal-session analog; the capture's most-recently-updated-wins rule owns shared directories). Probe-verified on opencode 1.18.22: the session row lands even when the model call errors — deterministic, no API dependency"
  - "The host e2e runs WITHOUT the HOME sandbox: the real binary must resolve its own config/auth from the real home (spike-verified posture); the row it writes is keyed by the temp root directory, which is exactly what the filter matches"
  - "fake-claude gained an additive pwd recorder (FAKE_CLAUDE_PWD_FILE) — the end-to-end cwd proof for the global agent spawn; argv recorder + TERM trap + bracketed-paste contract untouched"

patterns-established:
  - "Task-less exception to a task gate: relax with an explicit scope conjunct (req.Scope != \"global\"), never by weakening the 409 copy"
  - "Read-at-use proof pattern: PUT /api/global {agent_id} inside a test flips the singleton engine; the next spawn obeys with no restart (D-24 exercised end-to-end)"

requirements-completed: [GSESS-02, GSESS-04, GVIEW-02, GINT-03]

coverage:
  - id: D1
    description: "Global agent spawn: agent-kind root gates (D-28/D-30), 201 with label Agent + global:true + engine read-at-use, cwd == the global root (pwd proof), D-34 one-agent 409 concurrent-only (exited never blocks)"
    requirement: GVIEW-02
    verification:
      - kind: unit
        ref: internal/api/sessions_global_test.go#TestGlobalAgentUnconfiguredRoot409
        status: pass
      - kind: unit
        ref: internal/api/sessions_global_test.go#TestGlobalAgentSpawn
        status: pass
      - kind: unit
        ref: internal/api/sessions_global_test.go#TestGlobalAgentOneAgentGate
        status: pass
    human_judgment: false
  - id: D2
    description: "Engine-branched global resume (D-31): 409 \"no global claude/opencode session to resume\" with no persisted id (never a silent fresh spawn); seeded singleton csid + transcript -> argv carries --resume <csid>; opencode resumes via the -s append"
    requirement: GSESS-02
    verification:
      - kind: unit
        ref: internal/api/sessions_global_test.go#TestGlobalAgentResumeNoId409
        status: pass
      - kind: unit
        ref: internal/api/sessions_global_test.go#TestGlobalAgentResumeArgv
        status: pass
    human_judgment: false
  - id: D3
    description: "Singleton persist + capture re-target (T-15-06): global_task.claude_session_id non-empty after a fake-claude spawn; stubbed discovery -> global_task.opencode_session_id written while a REAL task row's column stays NULL"
    requirement: GSESS-02
    verification:
      - kind: unit
        ref: internal/api/sessions_global_test.go#TestGlobalAgentSpawn
        status: pass
      - kind: unit
        ref: internal/api/sessions_global_test.go#TestGlobalAgentOpencodeCapture
        status: pass
    human_judgment: false
  - id: D4
    description: "Host-gated e2e: the REAL opencode in a temp-git-repo global root -> first turn -> ses_… id captured into global_task (skips cleanly where opencode is absent)"
    requirement: GSESS-02
    verification:
      - kind: integration
        ref: internal/api/sessions_global_test.go#TestGlobalOpencodeCaptureHost
        status: pass
    human_judgment: false
  - id: D5
    description: "Status feed widening (SC3 both passes): exactly ONE source:\"global\" entry — live manager pass (field-by-field wire contract, zero ids, locked labels) AND post-restart DB pass (exited/resumable, engine-branched); at-most-one under live+resumable and exited-in-manager; task entries byte-stable"
    requirement: GSESS-02
    verification:
      - kind: unit
        ref: internal/api/sessions_global_test.go#TestGlobalStatusLiveEntry
        status: pass
      - kind: unit
        ref: internal/api/sessions_global_test.go#TestGlobalStatusPostRestart
        status: pass
      - kind: unit
        ref: internal/api/sessions_global_test.go#TestGlobalStatusNoDuplication
        status: pass
    human_judgment: false
  - id: D6
    description: "GINT-03/GSESS-04 hold by construction and are test-pinned: RUNNING global agent + bash absent from GET /api/activity while a done task appears; reaper.go untouched in the diff"
    requirement: GINT-03
    verification:
      - kind: unit
        ref: internal/api/sessions_global_test.go#TestGlobalActivityExclusion
        status: pass
    human_judgment: false

duration: 42min
completed: 2026-08-26
status: complete
---

# Phase 15 Plan 02: Global agent spawn + status feed widening Summary

**Global agent spawns behind the D-33/D-34/D-31 gates with singleton csid persist, a closure-parametrized opencode capture re-target, and /api/agents/status widened to ONE source:"global" entry in both passes — closing the SC3 co-phasing mandate with the task paths byte-for-byte clean.**

## Performance

- **Duration:** 42 min
- **Started:** 2026-08-26T13:47:09Z
- **Completed:** 2026-08-26T14:29:28Z
- **Tasks:** 2 (both TDD: RED → GREEN)
- **Files modified:** 5

## Accomplishments
- Global agent branch in `create`: the one task-less exception to the agent-requires-task gate, one-agent 409 over `ListGlobal()` ("global agent already running", RUNNING-only), engine-branched resume 409s off the singleton ids, and csid persisted with `UPDATE global_task ... WHERE id = 1` (warn-only)
- `captureOpencodeSessionAsync` re-targeted via persist closures — one poller, two owners; the CI stub test proves global_task written while a real task row stays NULL
- Status feed widened in both passes: manager-derived live entry + DB-derived post-restart entry, zero ids, locked Scratchpad/Global labels, `source:"global"` — the Phase-16 discriminator; at-most-one guaranteed by the `hasGlobalAgent` gate
- Host-gated e2e green on this host: real opencode 1.18.22 in a temp-git-repo root, first turn driven out-of-band, `ses_…` id captured into global_task
- Whole repo green (`go test ./internal/... -count=1`), zero frontend changes, reaper.go untouched

## Task Commits

Each task was committed atomically (TDD):

1. **Task 1: Global agent spawn — gates, resume, persist, capture re-target** — `5ffa48d` (test) + `7702962` (feat)
2. **Task 2: Status feed widening — both passes, source:"global"** — `fd5ceab` (test) + `57edf0a` (feat)

## Files Created/Modified
- `internal/api/sessions.go` — global agent branch (gate exception, D-34 gate, D-31 resume, scope-branched csid persist), `opencodePersistTarget` + `taskPersistTarget`/`globalPersistTarget`, re-signed poller
- `internal/api/agents.go` — `globalResumeState` + `loadGlobalResumeState` + engine-branched `resumable`, Pass 1b + Pass 2b (110-line append-only diff; task JOINs byte-identical)
- `internal/api/sessions_global_test.go` — `newGlobalAgentServer(Opt)` harness + 11 tests (agent suite, status suite, activity exclusion) + helpers
- `internal/api/agent_integration_test.go` — GlobalRoutes registered in `newAgentIntegrationServer` (serve.go-parity drift catch)
- `internal/api/testdata/fake-claude` — additive pwd recorder (cwd proof)

## Decisions Made
See key-decisions in the frontmatter — notably the MaxOpenConns(1) hoist (below), the closure-based re-target, and the out-of-band `opencode run` first-turn driver.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Pass 2b singleton read hoisted before the task DB pass**
- **Found during:** Task 2 (status feed implementation)
- **Issue:** The plan places Pass 2b's singleton read "after the task DB loop" — but the task DB-derived pass holds its rows open via `defer rows.Close()`, and `store.Open` sets `MaxOpenConns(1)`. A second query while those rows are open waits forever on the single SQLite connection: a guaranteed deadlock on every status call with a non-empty task DB pass.
- **Fix:** `loadGlobalResumeState()` runs ONCE immediately after the manager task pass (before the DB pass opens rows), shared by Pass 1b and Pass 2b. One query per status call instead of the plan's conditional one — strictly cheaper, behaviorally identical.
- **Files modified:** internal/api/agents.go
- **Verification:** `go test ./internal/... -count=1` green (the deadlock would hang every status test); TestGlobalStatusPostRestart proves the DB-pass entry still emits.
- **Committed in:** 57edf0a (Task 2 GREEN)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Structural safety fix required for the feature to function at all; no scope creep — the emitted entries are exactly the plan's wire contract.

## Issues Encountered
- The FIRST full-repo test run reported an unnamed failure in `internal/api` under parallel-compile load; three subsequent complete runs (one `-v`, two plain, including the whole `./internal/...` tree) are fully green. The failing test name was not captured (output truncated before the grep). No repro in any later run; if it resurfaces, suspect a deadline-sensitive ws/api test under load, not this plan's suites (all ran green in every repeat).

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Phase 16 can branch on `source === "global"` (wire value flows through unrecognized today, harmlessly); the interim ActiveSessionsBar dead-nav row is the accepted UI-SPEC artifact until then
- Phase 17's restart E2E (both engines + resume through the real curl story) builds on TestGlobalStatusPostRestart/TestGlobalOpencodeCaptureHost
- The whole-repo green gate and the append-only agents.go diff satisfy the task-path byte-for-byte risk center

---
*Phase: 15-global-sessions-backend*
*Completed: 2026-08-26*

## Self-Check: PASSED

- All 6 key-files exist on disk; all 5 plan commits verified in git log (5ffa48d, 7702962, fd5ceab, 57edf0a, 1962331)
- TDD gates: test→feat sequence present for both tasks
- Acceptance greps re-verified: 'global agent already running'=1, both 'no global <engine> session to resume'=2, one captureOpencodeSessionAsync, both global_task UPDATEs, GlobalRoutes in harness, '"global"'×2 + Scratchpad + "Global" in agents.go, zero worktree_path in global branches, reaper.go untouched, zero web/ files
- Final suites: go test ./internal/... -count=1 green; TestGlobalOpencodeCaptureHost green on this host
