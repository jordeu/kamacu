---
phase: 17-hardening-e2e
plan: "04"
subsystem: testing
tags: [e2e, go-test, opencode, restart-resume, wrapper-agent, uat-walkthrough, d-52, d-59, d-60, d-61]

requires:
  - phase: 17-hardening-e2e/01
    provides: the real-binary lifecycle E2E harness (newE2EServer) this plan extends, the argv consecutive-pair idiom, the stop-202/root-re-PUT wire precedents
  - phase: 15-global-sessions-backend
    provides: the engine-branched opencode resume append (-s <ocsid>) and capture poller being proven through a real restart, TestGlobalOpencodeCaptureHost's out-of-band-turn posture
  - phase: 16-global-view-settings-bar-integration
    provides: the 16-UAT.md walkthrough-doc shape D-59 mirrors, the shipped UI the UAT observes
provides:
  - GSESS-02 closed for BOTH engines through real-binary restarts (claude: 17-01; opencode: TestE2EGlobalOpencodeRestartResume — capture, survive, resume-with-id, refuse-after-clear)
  - The realHomeEnv posture knob on the E2E harness — REAL HOME + inherited XDG for legs that must resolve real config/auth, with TMUX_TMPDIR/--db/root confinement intact
  - 17-UAT.md — the SC5 walkthrough doc (authored-pending): six locked flows, verbatim expected strings, the D-60 safety posture, the D-12 settlement procedure
affects: [17-VERIFICATION (executes the UAT + records the D-12 settlement), future E2E legs needing the real-HOME posture]

tech-stack:
  added: [] # stdlib + in-repo helpers (store.Open for the quiescent DB read) — zero new dependencies
  patterns:
    - "Wrapper-agent argv proof: PATCH the system agent's command to a record-then-exec script (absolute LookPath-resolved binary baked in); the engine-branched resume path runs verbatim while argv stays observable; restore PATCH rides t.Cleanup registered immediately after the mutation"
    - "Env-posture knob over env duplication: one bool on the shared harness (realHomeEnv) conditionally drops the sandbox HOME/XDG pair from both the fixed entries and the skip list — both server generations stay identical by construction"
    - "Quiescent-window DB read: the persisted opencode id is read between SIGTERM and server B (no writer), WAL + busy_timeout making the second connection safe — the HTTP resumable flip stays the primary observable"

key-files:
  created:
    - internal/api/e2e_opencode_resume_test.go
    - .planning/phases/17-hardening-e2e/17-UAT.md
  modified:
    - internal/api/e2e_global_restart_test.go

key-decisions:
  - "realHomeEnv added to the 17-01 harness (test-only, additive) — the plan's mandated REAL-HOME posture was structurally unreachable through childEnv() as shipped, and duplicating start() would violate the plan's own no-duplicated-spawn-logic acceptance criterion; XDG_CONFIG_HOME is inherited too because opencode resolves config via os.UserConfigDir (XDG first) — the honest consequence (the serve boot's idempotent InstallPlugin may refresh the user's real kamacu-managed plugin file, skip-on-match) is documented in-file"
  - "The D-54 edge re-PUTs the root between the clear and the resume refusal — D-33's root gates fire before resume validation, so a bare post-clear resume returns 'global root not configured' (17-01's step-10 precedent, applied to the opencode branch)"
  - "The captured ses_ id is read from the temp DB in the quiescent window between SIGTERM and server B (Pitfall 6's sanctioned belt-and-braces window) so the resume argv is asserted against the EXACT persisted id, not just a ses_ prefix; the HTTP resumable flip (opencode keys off the persisted id alone) remains the primary over-HTTP observable (T-17-13 cross-check)"
  - "Both the wrapper's exec line and the out-of-band first turn use the ONE LookPath-resolved absolute opencode path (T-17-12 — no PATH games); the out-of-band turn's model errors are tolerated — both live runs errored with an upstream UnknownError yet the session row landed and the capture poller persisted it, exactly the probe-verified 1.18.22 behavior"
  - "The UAT quotes 'No root configured yet.' inside flow 1's stop-everything-then-clear ending (SC1's post-clear row) rather than re-opening 16-UAT's Settings matrix; the preamble scopes out all 16-UAT-verified Phase-16 UX and quotes the two D-54 refusal strings as API-level reference (grep gates are line-based — the doc keeps every locked string unwrapped on one line)"
  - "TestGlobalSessionPlainBashSpawn's one-time 10.22s marker timeout (8s deadline, third site of the documented host-load flake class) is LOGGED to deferred-items item 4, not fixed — pre-existing, in-process, untouched by this plan's diff (scope boundary)"

patterns-established:
  - "Pattern: wrapper-agent proof — mutate a system agent's command, never its engine; the engine lock keeps the production branch verbatim while the test observes the argv"
  - "Pattern: authored-pending UAT — the walkthrough doc ships with empty result: lines and a decision slot; the human executes it at verify time and the doc becomes the record"

requirements-completed: [GSESS-02]

coverage:
  - id: D1
    description: "D-52 real-opencode restart-resume leg: wrapper-agent argv proof through a REAL restart — fresh spawn (no -s), out-of-band first turn, capture persisted (resumable flip over HTTP), SIGTERM + second process on the same --db, resume 201 carrying the consecutive pair -s <the SAME captured ses_ id>, D-54 refusal 409 'no global opencode session to resume' after the root clear"
    requirement: GSESS-02
    verification:
      - kind: e2e
        ref: "internal/api/e2e_opencode_resume_test.go#TestE2EGlobalOpencodeRestartResume"
        status: pass
    human_judgment: false
  - id: D2
    description: "D-59/D-60/D-61 walkthrough doc: six locked flows in the 16-UAT shape (SC5 lifecycle, both engines' D-53 resume, SC3 interlock, SC4 sentinel-leak, D-12 settlement with a decision slot), the mandatory safety-posture paragraph, verbatim locked strings throughout"
    verification:
      - kind: other
        ref: "structural gates: frontmatter keys, 6 flows, every required string grep-found on one line, safety concretes + disposable-clone suggestion, decision slot, Summary/Gaps blocks — all PASS"
        status: pass
    human_judgment: true
    rationale: "The walkthrough is authored-pending by design (the plan's output spec): the D-53 'it remembers' judgments, the visual sentinel-leak sanity and the D-12 keep-or-flip settlement are inherently human and execute at verify time"

duration: 30 min
completed: 2026-08-28T11:58:40Z
status: complete
---

# Phase 17 Plan 04: Opencode Restart-Resume E2E + UAT Walkthrough Summary

**One-liner:** GSESS-02's opencode half proven through a real serve restart with the real binary — a command-PATCHed wrapper agent records the argv while capture, the resumable flip and the `-s ses_<id>` resume append run verbatim — plus the authored-pending 17-UAT.md walkthrough (six locked flows, honest safety posture, D-12 settlement procedure).

## Accomplishments

- **`internal/api/e2e_opencode_resume_test.go`** (new): `TestE2EGlobalOpencodeRestartResume`, host-gated on opencode+tmux+git (D-50), extending the 17-01 harness with the ONE posture difference — `realHomeEnv` (REAL HOME + inherited XDG, Pitfall 3) so the spawned opencode, the capture poller's `opencode session list` and the test's out-of-band first turn all resolve the same real config/auth/session storage. The story: folder root + OpenCode default agent + tmux shell → fresh spawn through the wrapper (argv recorded, NO `-s` token) → out-of-band `opencode run` first turn (TestGlobalOpencodeCaptureHost's block, absolute binary, 120s budget, model errors tolerated) → stop (202) → the status feed's global entry flips `resumable:true` (60s poll — the capture persisted; opencode keys off the id alone) → SIGTERM → quiescent-window DB read of the captured `ses_…` id → second process on the same --db → exactly one resumable global entry (Scratchpad/Global, no live id) → resume 201 with the wrapper argv carrying the consecutive pair `-s <the SAME id>` → stop → root clear (D-16 wipe) → root re-PUT (D-33) → the honest 409 `no global opencode session to resume`. Live-run color: both full runs' `opencode run` errored with an upstream `UnknownError` — the session row landed anyway and the capture poller persisted `ses_fb7d…`, the exact probe-verified behavior the plan tolerates.
- **`internal/api/e2e_global_restart_test.go`** (modified, test-only): the additive `realHomeEnv` knob — a bool that conditionally drops the sandbox `HOME`/`XDG_CONFIG_HOME` pair from childEnv's fixed entries AND its skip list. Default false keeps every 17-01 leg byte-equivalent (all three `TestE2EGlobal*` legs pass together).
- **`.planning/phases/17-hardening-e2e/17-UAT.md`** (new): the D-59 walkthrough in the exact 16-UAT shape — frontmatter, `## Tests` with six `### N. Title (gate-ref)` flows each carrying a one-line `expected:` and an EMPTY `result:` line, the Summary counter block (6 pending) and Gaps. Preamble scopes out 16-UAT-verified Phase-16 UX and quotes the API-level refusal edges as reference strings. The D-60 safety posture is its own section naming the concretes. Flow 6 carries the D-12 `decision:` slot (default KEEP; a flip is recorded, never implemented).

## Verification Results

| Check | Result |
|---|---|
| `go test ./internal/api -run 'TestE2EGlobalOpencode' -count=1 -v -timeout 20m` (Task 1 gate) | PASS — 8.76s, first run |
| `go test ./internal/api -run 'TestE2EGlobal' -count=1 -timeout 20m` (plan verification block: 17-01 legs + this plan's leg) | PASS ×3 — all three legs (1.4–1.7s claude legs, 7.9–8.8s opencode leg) |
| `go vet ./internal/api` | PASS |
| gofmt on the two touched Go files | clean (the 8 gofmt-flagged files in the package are pre-existing debt, untouched — scope boundary) |
| Task 1 acceptance greps: 17-01 constructor call, LookPath skips ×3, realHomeEnv wiring, PATCH-before-Cleanup ordering, D-54 exact string | all PASS |
| Task 2 acceptance: frontmatter keys, `grep -c '^### '` = 6, every required string grep-found (`Agent session ended.`, `Resume session`, `Reset session`, `Global · Scratchpad`, `No root configured yet.`, `no global claude session to resume`, `no global opencode session to resume`, `Resuming…`, `Start agent`, `Bash <n>`, `` `Stop` ``, `global agent already running`), safety concretes + disposable clone, `decision:` slot, 6×`expected:`/6×empty `result:`, both engine names | all PASS |
| Plan verify command `test -f 17-UAT.md && [ $(grep -c '^### ' …) -ge 6 ]` | PASS |
| Full package `go test ./internal/api -count=1 -timeout 20m` | PASS ×2 (236.8s / run2) with TWO transient single-test failures across four runs — see Issues; all three `TestE2EGlobal*` legs passed in every run that reported them |
| Prohibition: `git diff 9451d51..HEAD -- web/` | EMPTY — zero changes under `web/src/components/` and `web/src/pages/` (D-12 settlement is doc-recorded only) |
| Isolation after runs: user's real kamacu socket | `no server running on /tmp/tmux-1000/kamacu` — never touched |
| Stray processes / leftover wrapper DBs after runs | none (temp DBs removed with t.TempDir; no test kamacu-serve or wrapper opencode processes remain) |

## Task Commits

1. **Task 1: real-opencode restart-resume leg (wrapper argv proof)** — `4adaa24` (test)
2. **Task 2: 17-UAT.md walkthrough doc** — `9ff6570` (docs)

## Files Created/Modified

- `internal/api/e2e_opencode_resume_test.go` — the opencode restart-resume leg + `e2eWaitOcArgv`/`e2eWaitGlobalResumable` local helpers
- `internal/api/e2e_global_restart_test.go` — the additive `realHomeEnv` posture knob (struct field + conditional in childEnv)
- `.planning/phases/17-hardening-e2e/17-UAT.md` — the D-59 walkthrough (authored-pending)

## Decisions Made

See frontmatter `key-decisions` — the load-bearing ones: the test-only harness knob over duplicating spawn logic; the D-33 root re-PUT making the D-54 refusal honest; the quiescent-window exact-id DB read behind the HTTP-first observability; the deferred-items logging (not fixing) of the third marker-flake site.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocker] `realHomeEnv` knob on the 17-01 harness file**
- **Found during:** Task 1 (before writing the leg)
- **Issue:** the plan's files_modified lists only the new test file, but the mandated env posture (REAL HOME, NOT HOME-sandboxed) is structurally unreachable through the shipped harness — `childEnv()` hardcodes `HOME=<sandbox>` and is called directly by `start()`; Go permits no cross-file method override, and re-implementing `start()` would violate the plan's own "no duplicated build/spawn/restart logic" acceptance criterion.
- **Fix:** a minimal additive bool on `e2eServer` (+ conditional in childEnv, both fully commented) that drops the sandbox HOME/XDG pair from the fixed entries and the skip list. Default (false) keeps every 17-01 leg byte-equivalent — all three legs verified green together. Test-only; zero production changes.
- **Files modified:** internal/api/e2e_global_restart_test.go
- **Verification:** `TestE2EGlobal*` ×3 PASS; full package green
- **Commit:** 4adaa24

**2. [Rule 1 - Plan-text vs wire] Stop endpoint asserted 202, not 200**
- **Found during:** Task 1 (story step 6)
- **Issue:** the plan text says `POST /api/sessions/{id}/stop → 200`; the shipped wire is 202 (sessions.go, locked by 17-01 deviation #4a).
- **Fix:** asserted 202 for both stops — resolved toward the locked wire contract.
- **Files modified:** internal/api/e2e_opencode_resume_test.go
- **Commit:** 4adaa24

**3. [Rule 1 - Plan-text vs wire ordering] Root re-PUT between clear and the D-54 resume refusal**
- **Found during:** Task 1 (story step 9)
- **Issue:** a bare post-clear resume returns the D-28 `global root not configured` 409 because D-33's root gates run before resume validation — the plan's literal step 9 would never observe the engine-naming refusal.
- **Fix:** stop the resumed agent, clear the root (200, wipes the ids per D-16), re-PUT the root (200), THEN the resume 409s `no global opencode session to resume` — 17-01's step-10 precedent applied to the opencode branch, making the refusal a true cleared-ocsid observable.
- **Files modified:** internal/api/e2e_opencode_resume_test.go
- **Commit:** 4adaa24

---

**Total deviations:** 3 auto-fixed (1 blocker, 2 plan-text vs wire alignments — both following 17-01's established precedents). **Impact:** the plan's own constraints hold — zero production changes, zero new dependencies, zero new exports; the harness extension is additive and default-off.

## Issues Encountered

- **Full-package transient (pre-existing, NOT caused by this plan):** across four consecutive full `./internal/api` runs: FAIL (run 1, test name not captured — output truncated) → PASS → FAIL `TestGlobalSessionPlainBashSpawn` at 10.22s (an 8s pwd-marker deadline, sessions_global_test.go:233) → PASS. This is the third site of the twice-documented host-load marker-wait flake class (17-03 gave the two recurred sites 10s headroom); this site had never recurred before. It is an in-process Phase-15 test untouched by this plan's diff (e2e_* files + docs only). Logged as deferred-items item 4 for a future quick task — not fixed here (scope boundary).

## Known Stubs

None — no stubs, placeholders, or unwired data paths. 17-UAT.md's empty `result:` lines are the authored-pending walkthrough contract (the deliverable IS the doc; execution happens at verify time), not a stub.

## Authentication Gates

None.

## UAT Status (per the plan's output spec)

**17-UAT.md is authored-pending.** The human walkthrough — including the D-12 bar-row settlement (decision slot, default KEEP) and both engines' D-53 same-conversation memory judgments — executes at verify time (`/gsd-verify-work`); the doc's `result:`/`decision:` lines and Summary counters are the record.

## Threat Register Closure

| Threat | Disposition |
|---|---|
| T-17-10 (shared tmux socket) | MITIGATED — TMUX_TMPDIR sandbox in both the server child env and harness probes for this leg too; the real socket verified untouched after every run |
| T-17-11 (leftover wrapper command) | MITIGATED — restore PATCH registered in t.Cleanup immediately after the mutation (runs before server teardown); dedicated temp --db contains even a missed restore (verified: no leftover temp DBs) |
| T-17-12 (PATH-spoofed opencode) | MITIGATED — LookPath-resolved ABSOLUTE path baked into the wrapper's exec line and used for the out-of-band turn |
| T-17-13 (argv evidence) | MITIGATED — consecutive-token-pair idiom cross-checked against the status-feed resumable flip AND the quiescent-window exact-id DB read |

## Next

Phase 17 complete (4/4 plans) — ready for `/gsd-verify-work 17` (executes 17-UAT.md live, records the D-12 settlement) and the phase verifier carrying 17-02's audit table + this plan's UAT record into 17-VERIFICATION.md.

## Self-Check: PASSED

- [x] internal/api/e2e_opencode_resume_test.go exists; TestE2EGlobalOpencodeRestartResume green (×5 runs)
- [x] .planning/phases/17-hardening-e2e/17-UAT.md exists; structural gates + all string greps PASS
- [x] Commits 4adaa24 / 9ff6570 present on task/add-global-session-301
- [x] Zero web/ diff; user's real tmux socket untouched; no stray processes or leftover wrapper DBs
