---
phase: 14-global-config-api
plan: "02"
subsystem: api
tags: [curl, smoke-test, tmux, global-config, go-test, verification]

# Dependency graph
requires:
  - phase: 14-global-config-api plan 01
    provides: "GET/PUT /api/global production surface (global.go, GlobalRoutes, serve.go wiring) + github/tmux test seams the verification builds on"
provides:
  - "Proof the 409 live-session gate trips against a REAL detached global tmux session and releases on stop (GCONF-04/D-13/D-15)"
  - "Proof config-history (managed -> folder -> clear -> managed) never leaks a stale github_repo marker and always clears resume ids — reattach re-PUT included (Pitfall 8, D-04/D-16)"
  - "Real-binary curl smoke transcript: the full PUT grammar served end-to-end by cmd/kamacu serve on a fresh isolated install"
  - "Full internal/api regression green (modulo the four pre-existing Phase-13-logged failures) + go vet clean"
affects: [15-sessions-backend, 16-view-settings-bar, 17-hardening-e2e]

# Tech tracking
tech-stack:
  added: []  # verification-only plan — zero new dependencies, zero production code changes
  patterns:
    - "Real-binary curl smoke: isolated HOME sandbox + temp --db + free 127.0.0.1 port, healthz-gated startup, SIGTERM shutdown"
    - "Per-test tmux socket (ktest-globalgate-*) with scope='global' row + real detached session — the gate-trip recipe for Phase 15/17 tests"
key-files:
  created:
    - .planning/phases/14-global-config-api/14-02-SUMMARY.md
  modified:
    - internal/api/global_test.go

key-decisions:
  - "Smoke ran the REAL binary with an isolated HOME so every ~/.kamacu resolution (worktrees root, tmux conf sibling, opencode plugin probe, migration gates) landed in the sandbox — production loopback surface exercised without touching user state"
  - "The managed/clone PUT variant is deliberately NOT curled: seam-covered by 14-01 unit tests (validate/clone/reattach fakes); a live gh clone in a smoke run would be network-flaky"
  - "SC3's settable half demonstrated with agent_id=2 (the opencode seed) so the agent summary observably changes engine — stronger than a same-id no-op PUT"

patterns-established:
  - "Curl smoke transcript format: labeled step + observed HTTP status + raw body, recorded in the SUMMARY as the acceptance artifact"

requirements-completed: [GCONF-02, GCONF-04]

coverage:
  - id: D1
    description: "The 409 root-change gate trips against a REAL live global tmux session (structured reasons, row untouched) and releases once the session stops; agent-only PUTs pass under the same live session"
    requirement: GCONF-04
    verification:
      - kind: unit
        ref: internal/api/global_test.go#TestPutGlobalRootBlockedByLiveTmux
        status: pass
    human_judgment: false
  - id: D2
    description: "Config-history matrix: managed -> folder -> clear -> managed never leaves a stale github_repo marker, bumps updated_at every step, reattaches without re-cloning, and clears resume ids on every root PUT (reattach included) while agent-only PUTs preserve them"
    requirement: GCONF-02
    verification:
      - kind: unit
        ref: internal/api/global_test.go#TestPutGlobalConfigHistory
        status: pass
    human_judgment: false
  - id: D3
    description: "Real-binary curl smoke: fresh serve install honors the full /api/global wire contract (unconfigured GET shape with 7 keys and no resume-id keys, folder configure, agent set, clear, both 400 grammar branches) over HTTP on 127.0.0.1"
    verification:
      - kind: other
        ref: "curl round-trip transcript below (observed statuses 200/200/200/200/400/400; T-14-09 key assertion: required keys present, resume-id keys absent)"
        status: pass
    human_judgment: false

# Metrics
duration: 8 min (Task 2 resume run; Task 1 executed in the prior run)
completed: 2026-08-26
status: complete
---

# Phase 14 Plan 02: Global config verification Summary

**Live-tmux 409 gate + config-history invariants proven in-package, and the real `kamacu serve` binary driven through the complete /api/global curl contract on a fresh isolated install**

## Performance

- **Duration:** 8 min (Task 2 resume run — Task 1 ran in the prior execution session)
- **Started:** 2026-08-26T04:49:48Z (Task 2 resume)
- **Completed:** 2026-08-26T04:58:10Z
- **Tasks:** 2 (Task 1 in prior run, Task 2 in this resume run)
- **Files modified:** 2 (internal/api/global_test.go; this SUMMARY)

## Accomplishments
- The GCONF-04 gate is proven against a REAL detached global tmux session (not just zero-session fakes): 409 with structured `{kind:"sessions", target:"kamacu-global-1"}` reasons, singleton row byte-untouched, agent-only PUT 200 under the same live session, and the gate releases after KillServer — liveness, not row existence, blocks (Task 1, prior run).
- The Pitfall-8 lifecycle matrix holds: managed -> folder -> clear -> managed never leaks a stale github_repo, strictly increases updated_at, reattaches the same repo WITHOUT invoking the clone runner, and clears resume ids on every root PUT — the identical-value reattach re-PUT included, proving D-16 has no no-op exception (Task 1, prior run).
- The real production binary serves the phase's curl-verifiable goal end-to-end: unconfigured GET (honest zeros, non-null agent summary, zero resume-id keys), folder configure (root_exists true, github_repo null), agent set (summary reflects the opencode engine), clear back to unconfigured, and both 400 grammar branches (Task 2, transcript below).
- Full internal/api regression: every global test green (17/17), only the pre-existing Phase-13-logged failures remain; go vet clean on both touched packages.

## Task Commits

Each task was committed atomically:

1. **Task 1: Live-tmux 409 gate test + config-history staleness matrix + reattach no-reclone/no-no-op assertions** - `5e14fa6` (test — prior run, 306 insertions in internal/api/global_test.go)
2. **Task 2: Real-binary curl smoke round-trip + full-package regression (this SUMMARY)** - see `docs(14-02)` commit below (docs)

**Plan metadata:** (see final docs commit below)

## Curl Smoke Transcript (Task 2 — real binary)

Environment: `go build -o /tmp/opencode/kamacu-smoke ./cmd/kamacu` (plain build — `web/dist` present, embed satisfied; same shape as `make backend`). Sandbox under `/tmp/opencode/kamacu-smoke-14-02/`: isolated `HOME` (so every `~/.kamacu` resolution — worktrees root, tmux conf sibling, opencode plugin probe, migration gates — lands in the sandbox), temp `--db` (fresh goose migrate through 00018 on boot), `git init` fixture dir for the folder variant, OS-assigned free port on 127.0.0.1. Startup gated on `GET /api/healthz -> 200`. Server log clean (18 migrations OK, reaper started, listening).

**Key-shape assertion on the observed unconfigured GET body (T-14-09):** required keys `root_path, github_repo, agent_id, updated_at, root_exists, live, agent` — missing: NONE; resume-id keys (`claude_session_id`, `opencode_session_id`) — present: NONE.

```text
=== 1. GET /api/global (unconfigured) -> HTTP 200
{"root_path":"","github_repo":null,"agent_id":1,"updated_at":"2026-08-26T04:50:08.566Z","root_exists":false,"live":{"agent":0,"bash":0,"tmux":0},"agent":{"id":1,"name":"Claude Code","engine":"claude"}}

=== 2. PUT folder root {"root_path":"/tmp/opencode/kamacu-smoke-14-02/fixture-root"} -> HTTP 200
{"root_path":"/tmp/opencode/kamacu-smoke-14-02/fixture-root","github_repo":null,"agent_id":1,"updated_at":"2026-08-26T04:50:32.054Z","root_exists":true,"live":{"agent":0,"bash":0,"tmux":0},"agent":{"id":1,"name":"Claude Code","engine":"claude"}}

=== 3. PUT agent {"agent_id":2} (opencode — SC3 settable half) -> HTTP 200
{"root_path":"/tmp/opencode/kamacu-smoke-14-02/fixture-root","github_repo":null,"agent_id":2,"updated_at":"2026-08-26T04:50:32.074Z","root_exists":true,"live":{"agent":0,"bash":0,"tmux":0},"agent":{"id":2,"name":"OpenCode","engine":"opencode"}}

=== 4. GET /api/global (reflects both) -> HTTP 200
{"root_path":"/tmp/opencode/kamacu-smoke-14-02/fixture-root","github_repo":null,"agent_id":2,"updated_at":"2026-08-26T04:50:32.074Z","root_exists":true,"live":{"agent":0,"bash":0,"tmux":0},"agent":{"id":2,"name":"OpenCode","engine":"opencode"}}

=== 5. PUT clear {"root_path":""} -> HTTP 200
{"root_path":"","github_repo":null,"agent_id":2,"updated_at":"2026-08-26T04:50:32.121Z","root_exists":false,"live":{"agent":0,"bash":0,"tmux":0},"agent":{"id":2,"name":"OpenCode","engine":"opencode"}}

=== 6. GET /api/global (unconfigured again) -> HTTP 200
{"root_path":"","github_repo":null,"agent_id":2,"updated_at":"2026-08-26T04:50:32.121Z","root_exists":false,"live":{"agent":0,"bash":0,"tmux":0},"agent":{"id":2,"name":"OpenCode","engine":"opencode"}}

=== 7. PUT {} (empty body) -> HTTP 400
{"error":"nothing to update"}

=== 8. PUT {"repo":"o/w","root_path":"/x"} (both supplied) -> HTTP 400
{"error":"supply either repo or root_path, not both"}
```

Observations against the SC1–SC4 shape:

- **SC1 (folder)**: step 2 → 200, `root_exists:true`, `github_repo:null` — the folder variant never writes a managed marker.
- **SC3 (agent settable half)**: step 3 → 200 with the agent summary observably changed to `{"id":2,"name":"OpenCode","engine":"opencode"}` — a real engine swap, not a same-id no-op. (The delete-guard half shipped in Phase 13.)
- **SC4 (clear half)**: steps 5–6 → 200 and the honest unconfigured shape returns. The 409 half of SC4 is Task 1's live-tmux proof (`TestPutGlobalRootBlockedByLiveTmux`) — nothing in Phase 14 can create a live global session on a real server, so the gate half is proven in-package against a real detached session instead.
- **400 grammar**: steps 7–8 cover both reject branches with the locked copy.
- **updated_at** strictly increases across every successful PUT (`...08.566 -> ...32.054 -> ...32.074 -> ...32.121`).
- **Managed/clone path (SC2) deliberately NOT curled**: the gh-dependent variant (validate → clone → reattach) is seam-covered by 14-01's unit tests (`TestPutGlobalManagedClone`, `TestPutGlobalManagedReattach`, `TestPutGlobalManagedValidateFail`, `TestPutGlobalManagedCloneFailureAtomic` with validate/clone fakes); a live `gh repo clone` inside a smoke run would be network-flaky and add no wire-contract coverage. The reattach/no-reclone invariant IS additionally proven in-package by Task 1's `TestPutGlobalConfigHistory`.

**Shutdown:** SIGTERM → process exited, port refuses connections (post-shutdown healthz probe 000), no error lines appended to the server log — clean stop.

## Regression Results (Task 2)

- `go test ./internal/api/ -count=1` — FAIL, exactly the three pre-existing failures logged in Phase 13's deferred-items (`TestInput_Happy_WritesAndAppendsCR`, `TestInput_TrailingLF_TranslatedToCR`, `TestInput_EmptyMessage_WritesBareCR` — PTY-input surface untouched since Phase 13, environment-dependent; the fourth logged failure, `TestCustomEngineDoesNotGetHookEnv`, lives in `internal/session`, outside this package run). **Zero new failures; every global test passes.**
- `go test ./internal/api/ -count=1 -run 'TestGlobal|TestGetGlobal|TestPutGlobal' -v` — **17/17 PASS**, including `TestPutGlobalRootBlockedByLiveTmux` (0.17s) and `TestPutGlobalConfigHistory` (0.17s).
- `go vet ./internal/api/ ./cmd/kamacu/` — exit 0.
- Repo clean of stray artifacts (all smoke files under `/tmp/opencode/` only).

## Files Created/Modified
- `internal/api/global_test.go` - +306 lines (Task 1, commit 5e14fa6): `TestPutGlobalRootBlockedByLiveTmux`, `TestPutGlobalConfigHistory`
- `.planning/phases/14-global-config-api/14-02-SUMMARY.md` - this file, including the real-binary curl smoke transcript

## Decisions Made
- Smoke isolation via env (`HOME=<sandbox>` + temp `--db`), not chroot/docker: the binary's every `~`-relative resolution follows HOME, which is the exact production contract on a fresh install — cheapest faithful sandbox.
- Managed variant excluded from the curl smoke (seam-covered in 14-01; live clone = network flake) — the plan's own scoping, restated here because the transcript is the phase's acceptance artifact.
- Agent-PUT demonstrated with the opencode seed id so the response's agent summary changes engine — proves the JOIN reflects the write, which a same-id PUT could not.

## Deviations from Plan

None - plan executed exactly as written. (Task 1 completed in the prior run at `5e14fa6`; this resume run executed Task 2 only, per the resume context.)

## Issues Encountered
- The smoke fixture's optional empty `git commit` failed on this host (global `commit.gpgsign=true`, no secret key). Harmless: `validateRepoPath` only requires `git -C <dir> rev-parse --git-dir` to succeed, which plain `git init` satisfies — step 2's 200 proves it. No repo config was touched.
- `wait $SRV_PID` reported 127 because the shutdown probe ran in a different shell than the spawning one (not a child); the authoritative clean-exit evidence is the reaped PID, the refused port, and the log.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Phase 14 complete (both plans): the /api/global wire contract and gate semantics are proven against the real binary — Phase 15 (sessions backend) can build the spawn path on `global_task` + `ListGlobal()` widening `globalLiveBlockers`' manager seam in place.
- Pre-existing deferred failures (3x PTY-input in `internal/api`, 1x opencode hook-env in `internal/session`) remain the only red in the tree — logged in Phase 13's deferred-items for a dedicated investigation task.

## Self-Check: PASSED

- Commit `5e14fa6` (Task 1) exists on HEAD: `git log --oneline -1 5e14fa6` → `test(14-02): live-tmux 409 gate trip + config-history staleness matrix` (306 insertions, `internal/api/global_test.go` only).
- `internal/api/global_test.go` contains `func TestPutGlobalRootBlockedByLiveTmux` and `func TestPutGlobalConfigHistory` (both PASS in the -v run above).
- `test -f .planning/phases/14-global-config-api/14-02-SUMMARY.md` and `grep -q 'curl'` → present (this file).
- Task `<verify>` chain: `go test ./internal/api/ -count=1` non-zero ONLY due to the pre-existing logged failures (accepted exception, acceptance criterion 4); `go vet` exit 0.
- Plan-level `<verification>`: global test command green (17/17), transcript recorded above, `git status` clean of stray artifacts (smoke files under `/tmp/opencode` only; unrelated pre-existing dirty tree left untouched).

---
*Phase: 14-global-config-api*
*Completed: 2026-08-26*
