---
phase: 17-hardening-e2e
plan: "01"
subsystem: testing
tags: [e2e, go-test, process-lifecycle, tmux, sigterm-restart, fake-claude]

requires:
  - phase: 15-global-sessions-backend
    provides: global spawn/status/resume wire surface, fake-claude recorder conventions, restart-sim semantics
  - phase: 14-global-config-api
    provides: PUT /api/global 409-gate grammar {error, reasons[{kind,target}]}, resume-id clearing (D-16)
provides:
  - The repo's first real-binary lifecycle E2E harness (newE2EServer) — in-test go build, sandboxed spawn, healthz poll, SIGTERM+Wait restart loop
  - SC1 permanent regression coverage (GCONF-04 E2E half): reconfigure-while-live gate cycle over real HTTP
  - SC2 permanent regression coverage (GSESS-02 claude half + GSESS-03 API half): real SIGTERM → second process on the same DB → resume offer with the persisted csid on the argv → tmux row survival + reattach
  - D-54 claude refusal edges at API level (no persisted id via SC1's clear; transcript vanished via SC2's fixture delete)
affects: [17-03 (D-63 env hygiene absorbs the TMUX_TMPDIR scrub seam), 17-04 (opencode leg reuses this harness), 17-VERIFICATION]

tech-stack:
  added: [] # stdlib + in-repo testdata only — zero new dependencies
  patterns:
    - "Real-binary restart loop: spawn → poll /api/healthz → SIGTERM → Wait-with-deadline → re-spawn identical args/env against the same --db"
    - "Socket-dir isolation via a sandbox tmux PATH wrapper (production's tmux spawn env allow-list drops TMUX_TMPDIR — the wrapper re-arms it for every server-side invocation)"
    - "File-backed server logs in exec'd-process harnesses (pipe/buffer outputs deadlock cmd.Wait when a daemonizing grandchild — the tmux server — inherits the pipe)"

key-files:
  created:
    - internal/api/e2e_global_restart_test.go
  modified:
    - internal/api/testdata/fake-claude

key-decisions:
  - "Harness home is internal/api (17-RESEARCH OQ1): one new file, unexported helpers, in-package reuse of doJSON/doJSONList/gitRepo/testdataFakeClaude/argvHasPair — no refactor of the Phase-15 suite"
  - "Sandbox tmux PATH wrapper instead of a production env change: manager.go's tmux spawn allow-list (the TMUX/TMUX_PANE leak scrub) drops TMUX_TMPDIR, so tabs would land on the user's REAL socket while every probe honors the sandbox dir; the wrapper is the minimal harness-side fix, and the production inconsistency is logged to deferred-items for 17-03's D-63 env-hygiene work"
  - "fake-claude gained a --version fast path (before the recorders, so a boot probe never clobbers the argv file): serve's boot-time quota.DetectVersion probe would otherwise hang ~300s on the stub's orphaned `sleep 300` grandchild holding the probe's stdout pipe — no existing test passes --version, so zero behavioral delta for the in-process suites"
  - "File-backed (never buffer/pipe) server output capture: the daemonized tmux server inherits the serve process's stdio and holds the pipe open forever, deadlocking cmd.Wait on an already-dead process (found as a 15-minute test hang)"
  - "engine 'claude' asserted via GET /api/global agent.engine — the /api/agents/status wire has NO engine field (agentStatusEntry), and the singleton agent summary is the same engine read that keys the resume branch"
  - "SC1's id-clearing proof re-PUTs the root before the resume-refusal 409 (D-33: the unconfigured-root 409 fires before resume validation, so a bare post-clear resume returns 'global root not configured') and seeds a transcript first so an UNCLEARED csid would have resumed 201 — making the 409 a true D-16 proof"
  - "Live tmux tabs carry no tmuxName on the engine wire (Info() omits it by design; only orphaned/reconciled rows have it) — the sandbox-socket ListSessions probe is the authoritative live-name source (waitGlobalTmuxName)"

patterns-established:
  - "Pattern: real-binary SUT harness — host-gate (LookPath tmux+git, skip; build failure fatal), t.TempDir sandbox (HOME/TMUX_TMPDIR/XDG_CONFIG_HOME child env + KAMACU_* scrub), net.Listen 127.0.0.1:0 port allocation, LIFO t.Cleanup (stop server → KillServer sandbox socket → TempDir removal)"
  - "Pattern: over-HTTP lifecycle polling — e2eWaitGlobalSessionStatus observes API-stop exits via the scoped list (no manager handle into the real process); e2eWaitArgvPair distinguishes a new spawn's recorder content from the previous spawn's (the file is overwritten per spawn)"

requirements-completed: [GCONF-04, GSESS-02, GSESS-03]

coverage:
  - id: D1
    description: "Real-binary E2E harness + SC1 reconfigure-gate leg: configure → agent+tmux spawns → concurrent 409 → root change/clear 409s with reasons → stop → clear 200 → id-clearing proof via resume-refusal 409"
    requirement: GCONF-04
    verification:
      - kind: e2e
        ref: "internal/api/e2e_global_restart_test.go#TestE2EGlobalReconfigureGate"
        status: pass
    human_judgment: false
  - id: D2
    description: "SC2 restart leg: real SIGTERM + second process on the same DB → exactly one resumable global status entry → tmux row survives (orphaned, label byte-identical, alive on the sandbox socket) → reattach 201 label-preserved → resume 201 with --resume <same csid> → D-54 transcript-vanished 409"
    requirement: GSESS-02
    verification:
      - kind: e2e
        ref: "internal/api/e2e_global_restart_test.go#TestE2EGlobalRestartResume"
        status: pass
    human_judgment: false
  - id: D3
    description: "tmux survival + invisible reattach at API level (GSESS-03 half owned by this plan): post-restart row with orphaned/global flags, byte-identical label, harness has-session probe alive, reattach_tmux_name POST 201"
    requirement: GSESS-03
    verification:
      - kind: e2e
        ref: "internal/api/e2e_global_restart_test.go#TestE2EGlobalRestartResume"
        status: pass
    human_judgment: false

duration: 91 min
completed: 2026-08-28T07:21:30Z
status: complete
---

# Phase 17 Plan 01: Real-binary Global Restart E2E Summary

**One-liner:** The repo's first real-binary lifecycle E2E — builds cmd/kamacu in-test, spawns it in a fully isolated sandbox, drives the SC1 reconfigure-gate cycle and the SC2 SIGTERM→restart→resume narrative over real HTTP, and proves tmux tab survival with byte-identical labels.

## Accomplishments

- **`internal/api/e2e_global_restart_test.go`** (new): the `TestE2EGlobal*` family — host-gated (tmux+git LookPath skips, build failure fatal), inline in `go test ./...` (D-50), zero make targets.
  - **Harness (`newE2EServer`)**: in-test `go build -o <sandbox>/kamacu ./cmd/kamacu`; child env with `HOME`/`TMUX_TMPDIR`/`XDG_CONFIG_HOME` inside the sandbox, `FAKE_CLAUDE_ARGS_FILE`/`FAKE_CLAUDE_PWD_FILE` recorders, the `KAMACU_SESSION_ID`/`KAMACU_HOOK_TOKEN`/`KAMACU_HOOK_BASE` trio filtered out (secret/nesting hygiene, T-17-04), allocated `127.0.0.1:<port>`, `--claude-bin` = absolute testdata path; start/stop with healthz polling (5s/100ms) and SIGTERM+Wait (10s deadline, Kill backstop); LIFO cleanup (stop server → KillServer the sandbox tmux socket).
  - **`TestE2EGlobalReconfigureGate`** (SC1, D-48/GCONF-04): configure → shell=tmux → agent spawn (csid from the `--session-id` argv pair) → tmux tab → D-34 concurrent 409 (`global agent already running`) → root change 409 + root clear 409, both with the `{error, reasons[{kind,target}]}` grammar → API stops + tab delete → clear 200 → **id-clearing proof (D-16)**: root re-PUT + transcript seeded (an uncleared csid would resume 201) → `no global claude session to resume` 409.
  - **`TestE2EGlobalRestartResume`** (SC2, D-49/D-51/D-54/GSESS-02+03): fresh csid + transcript fixture (Pitfall 5) → tab minted, label+name recorded → **real SIGTERM** → second real process on the same `--db` → exactly ONE global status entry (source global, exited, resumable, Scratchpad/Global labels, engine claude via `GET /api/global` agent summary) → tmux row survived (orphaned+global, label byte-identical — the GAP-01 replay for the global scope) → harness has-session probe alive → reattach 201 label-preserved → resume 201 with `--resume <SAME csid>` (never forked) → transcript deleted → the D-54 transcript-vanished 409.
- **`internal/api/testdata/fake-claude`** (modified): additive `--version` fast path — see Deviations.

## Verification Results

| Check | Result |
|---|---|
| `go vet ./internal/api` | PASS |
| `go test ./internal/api -run 'TestE2EGlobal' -count=1 -v -timeout 15m` | PASS — both legs (~4.3s) |
| `go test ./internal/api -run 'TestE2EGlobalRestartResume' -count=1 -timeout 15m` (Task 2 gate) | PASS |
| Full-package regression `go test ./internal/api -count=1 -timeout 15m` | PASS 216.3s (baseline at pre-plan commit 9819b82: 215.8s — no cross-suite regression from the fake-claude edit) |
| Isolation sanity: user's real kamacu socket after runs | `no server running on /tmp/tmux-1000/kamacu` — never touched |
| Stray processes after runs (pgrep kamacu serve / sleep 300 / test binaries) | none |
| Acceptance criteria (Tasks 1+2, all grep/test gates) | all PASS — LookPath gates present, `TMUX_TMPDIR` ×11 with zero underscore-spelling occurrences, KAMACU_ trio in the env-filter switch, exact 409 strings asserted, serve flags `--db`/`--addr`/`--claude-bin` |

SC1/SC2 assertions map 1:1 to ROADMAP success criteria 1 and 2 (claude/tmux halves).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocker] fake-claude `--version` fast path (test fixture, not production)**
- **Found during:** Task 1 (first spawn never listened)
- **Issue:** serve's boot runs `quota.DetectVersion` → `<claude-bin> --version` once per process (serve.go:279, quota.go:410) with a 5s context kill — but the kill only reaps the stub's bash, leaving its `sleep 300` grandchild holding the probe's stdout pipe, so `Output()` blocks ~300s and the server never listens.
- **Fix:** 4-line additive `--version` branch in `internal/api/testdata/fake-claude`, placed BEFORE the recorders so a boot probe never clobbers the argv file. No existing test passes `--version` (verified by grep) — zero behavioral delta for the in-process suites. `--claude-bin` stays the absolute testdata path per the acceptance criterion.
- **Files modified:** internal/api/testdata/fake-claude
- **Commit:** 00d4258

**2. [Rule 3 - Blocker + T-17-01] Sandbox tmux PATH wrapper in the harness**
- **Found during:** Task 1 (spawned tabs landed on the user's REAL socket)
- **Issue:** `internal/session/manager.go`'s tmux spawn arm carries an explicit env allow-list (the TMUX/TMUX_PANE leak scrub) that **drops TMUX_TMPDIR** — the attach client resolves the socket at the default directory (the user's real tmux server) while every server-side probe (which inherits the full env) honors the sandbox dir. The plan's T-17-01 mitigation (TMUX_TMPDIR in the server env + probes) is insufficient against this seam; two test tabs briefly polluted the real socket during diagnosis (immediately killed; socket restored to its pre-test no-server state).
- **Fix:** the harness writes `<sandbox>/bin/tmux` — `TMUX_TMPDIR=<sandbox> exec <real-tmux> "$@"` — and prepends that dir to the child PATH, so every tmux invocation from inside the server (spawn arm AND probes, both PATH-resolve) agrees on the sandbox socket. No production change. The production-side inconsistency (probes honor TMUX_TMPDIR, spawns scrub it — only observable for users who set that var) is logged to `deferred-items.md` for plan 17-03's D-63 manager.go env work.
- **Files modified:** internal/api/e2e_global_restart_test.go (harness only)
- **Commit:** 00d4258

**3. [Rule 1 - Bug] File-backed server logs (pipe-EOF deadlock)**
- **Found during:** Task 1 (first green-leg attempt hung the whole test binary for the 15m timeout)
- **Issue:** with `cmd.Stdout = &bytes.Buffer{}`, os/exec pipes the output through copying goroutines that wait for EOF; the daemonized tmux server inherits the pipe write-end and never closes it, so `cmd.Wait()` never returns even after SIGTERM kills the serve process — cleanup hung forever.
- **Fix:** per-generation log FILES in the sandbox (`serve-N.log`), diagnostics read via `dumpLogs()`. Documented in the harness for future real-binary tests.
- **Files modified:** internal/api/e2e_global_restart_test.go
- **Commit:** 00d4258

**4. [Rule 1 - Plan-text vs wire] Stop endpoint is 202, not 200; engine asserted via GET /api/global; live tabs carry no tmuxName; SC1 resume proof re-PUTs the root**
- **Found during:** Tasks 1–2
- **Issue:** four plan-text/wire mismatches, each resolved toward the locked wire contract: (a) `POST /api/sessions/{id}/stop` replies **202** (sessions.go:759; the plan text said 200) — asserted 202; (b) `/api/agents/status` has NO `engine` field (agentStatusEntry) — engine "claude" asserted via `GET /api/global` → `agent.engine`, the same singleton read that keys the resume branch; (c) `Info()` deliberately omits `tmuxName` for LIVE sessions (only orphaned rows carry it) — the live tab's name is recorded from the sandbox socket itself (`waitGlobalTmuxName`); (d) a bare post-clear resume returns the D-28 "root not configured" 409 because D-33's root gates run before resume validation — the id-clearing proof re-PUTs the root (200) and seeds a transcript first, so the subsequent `no global claude session to resume` 409 is a true D-16 observable (an uncleared csid would have resumed 201).
- **Files modified:** internal/api/e2e_global_restart_test.go
- **Commits:** 00d4258, 5c7e3b7

**Total deviations:** 4 auto-fixed (2 blockers, 2 plan-vs-wire alignments). **Impact:** the harness now genuinely isolates in both socket directions, boots in seconds, and never hangs; all assertions target the shipped wire contracts.

## Full-Suite Flake Note (pre-existing, not caused by this plan)

Two consecutive full-package runs during development failed `TestInput_Happy_WritesAndAppendsCR` (marker timeout at ~7.4s) while ~12 unrelated PTY tests ran ~40% slow (suite 299s vs canonical 215s) — transient host load. Evidence it is NOT this plan's changes: the test passes in isolation on this branch; the full suite **passes on this branch** at the canonical 216.3s when the machine is quiet; and the pre-plan commit (9819b82) full suite passes at 215.8s under the same conditions. The 6s marker deadline under load is the flake's surface — belongs to the D-63 baseline investigation (plan 17-03), logged in deferred-items.md.

## Known Stubs

None — no stubs, placeholders, or unwired data paths.

## Authentication Gates

None.

## Next

Ready for 17-02 (per the phase directory's plan order).

## Self-Check: PASSED
