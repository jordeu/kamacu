---
phase: 14-global-config-api
verified: 2026-08-26T05:13:42Z
status: passed
score: 9/9 must-haves verified
behavior_unverified: 0
overrides_applied: 0
deferred:
  - truth: "Live.Agent / Live.Bash counts and the manager half of globalLiveBlockers are zero (no global PTY can exist until the spawn path lands)"
    addressed_in: "Phase 15"
    evidence: "Phase 15 goal: 'the session engine gains an additive global scope, POST /api/sessions {scope:\"global\"} spawns the agent and bash tabs in the global root' — globalLiveBlockers' mgr seam (global.go:117-122) is widened in place by Phase 15's ListGlobal() per D-19; the tmux half is REAL and proven now"
---

# Phase 14: Global config API Verification Report

**Phase Goal:** The global root and default agent are configurable over the API — a validated local folder or a gh-validated managed clone in a namespace no project can touch — with reconfiguration gated on live sessions and resume ids cleared on success. Independently testable with curl before any frontend exists.
**Verified:** 2026-08-26T05:13:42Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

Merged from ROADMAP Success Criteria 1–4 plus PLAN 14-01/14-02 frontmatter truths (deduplicated; roadmap wording kept).

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | SC1: GET /api/global returns config + derived state; PUT with a local folder path validates the directory and persists it as the global root; resume-id columns never serialize (D-17/D-18) | ✓ VERIFIED | `internal/api/global.go:198-255` (JOIN loader + os.Stat derivation); `globalConfig` struct (global.go:48-56) has no resume-id fields; TestGetGlobalUnconfigured (asserts key absence), TestGetGlobalConfiguredRoot — both PASS in verifier's own run; real-binary curl step 1 shows the full 7-key shape with zero resume-id keys |
| 2 | SC1/GCONF-01: folder variant validates via validateRepoPath; $HOME (any spelling), /, and anything inside ~/.kamacu rejected 400 with row untouched | ✓ VERIFIED | global.go:344-378 (validateRepoPath call, D-07 equality on post-expansion abs, D-27 equality+prefix block); TestPutGlobalFolder, TestPutGlobalFootguns (7-case table incl. `~`, `~/`, `$HOME`, `/`, `~/.kamacu`, child, non-git — each asserts row byte-identical) PASS |
| 3 | SC2/GCONF-02: PUT with repo gh-validates and clones into ~/.kamacu/repos/global/<owner>/<name>; same-repo re-PUT reattaches without re-cloning; failed clone leaves prior config and no directory (atomic) | ✓ VERIFIED | global.go:138-191 (putManagedRoot: ParseRepoRef → ValidateRepo degrade copy → 3-segment dest `filepath.Join(base, "global", canonical)` → reattachManaged verbatim → Clone + RemoveAll on error); gate hoisted before all gh work (put:310 precedes putManagedRoot call at put:329); TestPutGlobalManagedClone, ...ValidateFail, ...CloneFailureAtomic (asserts prior row + no dest residue), ...Reattach (failCloneNeverCalled seam), ...ReattachMismatch — all PASS |
| 4 | SC3/GCONF-03: default agent settable to any configured agent (unknown id 400); agent in use by global task cannot be deleted; agent-only PUTs never gate and never clear resume ids | ✓ VERIFIED | global.go:385-396 (existence check, "agent not found"); agent branch outside the rootSupplied gate (put:302,310); TestPutGlobalAgent, TestPutGlobalRootChangeClearsResumeIDs PASS; delete guard at agents_crud.go:274 (`SELECT COUNT(*) FROM global_task WHERE agent_id = ?`) — TestAgentDeleteInUseByGlobal PASS in verifier's run; curl step 3 shows agent summary swap to OpenCode |
| 5 | SC4/GCONF-04: root change or clear under a live global session refused 409 {error, reasons:[{kind,target}]}, row and disk untouched | ✓ VERIFIED | global.go:310-318 (409 write reusing deleteBlocker grammar), global.go:123-129 (globalLiveBlockers over scope='global' rows + exact-match HasSession, alive only on (true,nil) at global.go:104); **TestPutGlobalRootBlockedByLiveTmux PASS in verifier's own run (0.11s, real detached tmux session, not skipped)** — asserts structured reasons {kind:"sessions", target:"kamacu-global-1"}, row untouched, and gate releases after KillServer (liveness, not row existence) |
| 6 | SC4/D-16: every successful root change — including the clear and the same-repo reattach re-PUT — clears claude_session_id/opencode_session_id in the same single UPDATE; agent-only PUTs preserve them | ✓ VERIFIED | global.go:404-408 (id clears inside the single SET list, unconditional on rootSupplied); TestPutGlobalRootChangeClearsResumeIDs and TestPutGlobalConfigHistory (id-clearing asserted on managed, folder, clear AND reattach steps; ids survive the agent interlude) PASS |
| 7 | Dispatch grammar: both repo+root_path → 400; explicit-empty repo → 400 guidance; invalid ref → 400; empty body → 400 "nothing to update"; PUT returns the GET shape | ✓ VERIFIED | global.go:279-297 (branch-first dispatch), put responds via the shared loadGlobalConfig+deriveGlobalState path (D-23, global.go:421-431); TestPutGlobalRepoDispatch, TestPutGlobalEmptyBody PASS; real-binary curl steps 5–6 reproduce both 400s verbatim |
| 8 | Config-history (managed → folder → clear → managed) never leaks a stale github_repo marker and always bumps updated_at | ✓ VERIFIED | Folder/clear variants always set github_repo (global.go:339,378); TestPutGlobalConfigHistory asserts github_repo null after the folder transition, no clone-runner invocation on reattach (failCloneNeverCalled), and strictly increasing updated_at across every step — PASS |
| 9 | Phase goal: the real production binary serves the complete /api/global contract over curl before any frontend exists | ✓ VERIFIED | **Verifier's own smoke** (not the SUMMARY's): built `cmd/kamacu`, isolated HOME + temp --db, healthz-gated start → GET 200 (7-key shape), PUT folder 200 (root_exists true), PUT agent_id 2 200 (summary swap), PUT clear 200, PUT {} 400 "nothing to update", PUT both 400 "supply either repo or root_path, not both", zero resume-id keys on the wire, clean SIGTERM stop. Matches the 14-02-SUMMARY transcript exactly |

**Score:** 9/9 truths verified (0 present, behavior-unverified)

### Deferred Items

| # | Item | Addressed In | Evidence |
|---|------|-------------|----------|
| 1 | Live.Agent/Live.Bash counts and the manager half of globalLiveBlockers are zero-count seams | Phase 15 | Phase 15 goal: "the session engine gains an additive global scope, POST /api/sessions {scope:'global'} spawns the agent and bash tabs" — D-19 by design: no global PTY can exist until the spawn path lands; the tmux half is real and gate-proven now |

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/api/global.go` | GET/PUT handlers, wire types, gate, managed flow | ✓ VERIFIED | 432 lines, substantive (no debt markers), all acceptance-criteria greps hold (GlobalRoutes signature, JSON tags, "WHERE scope = 'global'", "global task row missing", "claude_session_id = NULL", "nothing to update", filepath.Join(base, "global", canonical), reattachManaged call) |
| `internal/api/global_test.go` | harness + 16 tests | ✓ VERIFIED | 891 lines; all 16 phase tests enumerated via `-list` and PASS in verifier's run; harness reuses doJSON/gitRepo from projects_test.go (same package) |
| `cmd/kamacu/serve.go` | one api.GlobalRoutes registration line | ✓ VERIFIED | Line 293, in the registry block alongside SessionRoutes/WorktreeRoutes; routes.go untouched (verified) |

### Key Link Verification

| From | To | Via | Status |
|------|----|----|--------|
| serve.go registry | api.GlobalRoutes | `api.GlobalRoutes(mux, db, mgr, tmuxClient)` at serve.go:293 | ✓ WIRED (real-binary smoke proves routes serve) |
| global.go folder branch | validateRepoPath + deleteBlocker grammar | global.go:344; deleteBlocker at global.go:126 | ✓ WIRED |
| global.go managed branch | github.ParseRepoRef/ValidateRepo/Clone + reattachManaged + reposBase | global.go:141/149/185/176/166 | ✓ WIRED |
| globalLiveBlockers | tmux_sessions scope='global' + HasSession exact-match | global.go:89, 104 (alive only on (true, nil)) | ✓ WIRED |
| TestPutGlobalRootBlockedByLiveTmux | real tmux + seeded row + WithTmux harness | global_test.go:565-673 | ✓ WIRED (test PASSES against real detached session) |
| Curl smoke | cmd/kamacu serve → GlobalRoutes | verifier's own round-trip | ✓ WIRED |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|--------------------|--------|
| global.go get/put | globalConfig | `SELECT ... FROM global_task g JOIN agents a ON a.id = g.agent_id WHERE g.id = 1` | Yes (real DB query; singleton seeded by migration 00017) | ✓ FLOWING |
| liveGlobalTmuxNames | names | `SELECT name FROM tmux_sessions WHERE scope = 'global'` + live probe | Yes (real query + tmux probe; empty on fresh DB is the honest state, exercised by tests) | ✓ FLOWING |
| put | sets/args | single `UPDATE global_task SET ... WHERE id = 1` | Yes | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| All global tests | `go test ./internal/api/ -run 'TestGlobal\|TestGetGlobal\|TestPutGlobal' -count=1 -v` | 17/17 PASS incl. TestPutGlobalRootBlockedByLiveTmux (0.11s, real tmux — not skipped) and TestPutGlobalConfigHistory (0.16s) | ✓ PASS |
| Agent delete guard (SC3 half) | `go test ./internal/api/ -run 'TestAgentDeleteInUseByGlobal' -count=1 -v` | PASS (0.07s) | ✓ PASS |
| Real-binary curl contract | isolated HOME + temp db smoke (6 requests) | 200/200/200/200/400/400 with exact bodies; zero resume-id keys; clean shutdown | ✓ PASS |
| go vet | `go vet ./internal/api/ ./cmd/kamacu/` | exit 0 | ✓ PASS |
| Full-package regression | `go test ./internal/api/ -count=1` (single run) | FAIL on exactly the 3 documented pre-existing TestInput_* failures (sessions_test.go PTY surface, verified pre-existing at baseline by Phase 13's verifier and at ddd00a4 per deferred-items.md); zero new failures | ✓ PASS (no regressions) |

### Probe Execution

Step 7c: SKIPPED — no `scripts/*/tests/probe-*.sh` probes declared by PLAN/SUMMARY for this phase; the phase's runnable checks are the Go test suite and the real-binary curl smoke (both executed above).

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| GCONF-01 | 14-01 | Configure the global root as a validated local folder path | ✓ SATISFIED | Truths 1–2; folder PUT proven on real binary |
| GCONF-02 | 14-01, 14-02 | Configure root as GitHub repo — gh-validate, clone into separate global namespace, atomic failure | ✓ SATISFIED | Truth 3 (seam-covered managed tests + config history; live gh deliberately not curled — network-flaky, documented, wire contract proven via the other variants) |
| GCONF-03 | 14-01 | Pick the global task's default agent from configured agents | ✓ SATISFIED | Truth 4 (settable half + Phase-13 delete guard both pass) |
| GCONF-04 | 14-01, 14-02 | Root change/clear refused 409 + reasons while live; resume ids cleared on success | ✓ SATISFIED | Truths 5–6 (real-tmux gate trip + id clearing, no exceptions) |

Orphaned requirements: none — REQUIREMENTS.md maps exactly GCONF-01..04 to Phase 14 (all marked Complete); GCONF-05 (Settings UX) is explicitly Phase 16's per the mapping note.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | — | No TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER markers, no placeholder returns, no empty implementations in any phase file | — | — |

**Info observations (not gaps):**
1. **Pre-existing dirty tree:** 114 uncommitted deletions of `.planning/phases/01..13/*` docs + 1 untracked `13-PATTERNS.md` in the working tree. Verified NOT caused by this phase (`git log ddd00a4..HEAD --diff-filter=D -- .planning` is empty; all six phase commits touch only global.go/global_test.go/serve.go/phase-14 docs). Orchestrator may want to address separately.
2. **Namespace mirror edge (research-dispositioned):** a project whose GitHub owner is literally named "global" clones to `repos/global/<name>` — inside the global subtree. 14-RESEARCH (line 267) verified exact-path collision is unrepresentable (2 vs 3 segments) and dispositioned "no guard needed". The mirror case (a global clone nested inside such a project's dir, exposed to the project's gated RemoveAll) is theoretically reachable but requires a pathological repo name and project-first creation order. Suggest a future hardening phase reject owner=="global" on the project side; no must-have truth fails today.
3. **TestPutGlobalRootBlockedByLiveTmux** exercises the agent-only escape with the CURRENT agent id (a sanctioned no-op agent change) rather than a different agent — still proves agent PUTs never gate.
4. PUT with invalid JSON → 400 "invalid JSON body" (global.go:272-274) has no named test — trivial shared idiom, info only.
5. gofmt drift in serve.go struct (pre-existing, logged in deferred-items.md, untouched by this phase's one-line addition).

### Human Verification Required

None. This phase is backend-only and fully curl-verifiable — every behavior-dependent truth (the 409 gate against real tmux liveness, resume-id state transitions, no-reclone reattach ordering, atomic failure) is exercised by a passing behavioral test run in this verification, and the phase-goal curl contract was reproduced by the verifier against the real binary. No UI, visual, or external-service surface exists in scope.

### Gaps Summary

No gaps. All four GCONF requirements are satisfied with behavioral evidence; all artifacts exist, are substantive, and are wired; the phase goal's curl-verifiable contract was independently reproduced (not just read from the SUMMARY). The manager-half of the live gate ships as a documented zero-count seam by design (D-19) and is Phase 15's explicit scope. The only red in the tree is the pre-existing, thrice-documented TestInput_* trio (plus one internal/session failure outside this package) — verified not regressions.

---

_Verified: 2026-08-26T05:13:42Z_
_Verifier: the agent (gsd-verifier)_
