# Phase 17 Deferred Items

## From 17-01 (real-binary E2E harness)

| # | Item | Discovered | Suggested Home | Evidence |
|---|------|-----------|----------------|----------|
| 1 | **Production TMUX_TMPDIR inconsistency in the tmux spawn arm**: `internal/session/manager.go`'s tmux spawn env allow-list (the TMUX/TMUX_PANE leak scrub, ~:255-275) drops `TMUX_TMPDIR` while every server-side probe (tmux.Client execs inherit the full env) honors it. For users who set `TMUX_TMPDIR`, spawned tabs land in the default socket dir while probes/gates/sweeps look in `TMUX_TMPDIR`-dir — inconsistent. Invisible in default installs (nobody sets the var) and NOT fixed by 17-01 (harness-side PATH wrapper compensates). **Status update (17-03, 2026-08-28): NOT absorbed — 17-03's plan scoped D-63 to the KAMACU_\* trio strip only (the tmux allow-list was never in its contract). Needs re-homing: a future quick task / Phase 17 wrap-up.** | 2026-08-28, during 17-01 Task 1 (two test tabs briefly landed on the real socket during diagnosis; killed immediately, socket restored) | RE-HOME: quick task (one conditional `TMUX_TMPDIR=<val>` append beside the existing allow-list, preserving the TMUX/TMUX_PANE scrub intent) | e2e_global_restart_test.go file-doc comment; manual repro captured in the session |
| 2 | ~~**Full-suite flake: `TestInput_Happy_WritesAndAppendsCR` marker timeout under host load**~~ **RESOLVED by 17-03 (commit 8ed08ad)**: the marker round-trip deadline was raised 2s → 10s in both recurred sites (`internal/api/sessions_test.go` TestInput_Happy + `internal/session/session_test.go` TestSpawnPumpFillsRing, which recurred in 17-03's baseline). Polls return on condition, so only the failure path gets headroom. Full suite green ×2 (with AND without the KAMACU_\* trio) post-fix. | 2026-08-28, during 17-01 full-package regression runs | closed | 17-03-SUMMARY "D-63 Baseline Evidence" |

## From 17-02 (sentinel-leak / interlock / MCP parity tests)

| # | Item | Discovered | Suggested Home | Evidence |
|---|------|-----------|----------------|----------|
| 3 | ~~**Recurrence of deferred item 2 during 17-02**~~ **RESOLVED by 17-03 (commit 8ed08ad)** — see item 2 above; the same class also fired once on `TestSpawnPumpFillsRing` in 17-03's own baseline run, confirming the diagnosis before the fix landed. | 2026-08-28, during 17-02 full-package regression runs | closed | 17-03-SUMMARY "D-63 Baseline Evidence" |

## From 17-04 (opencode restart-resume E2E + UAT)

| # | Item | Discovered | Suggested Home | Evidence |
|---|------|-----------|----------------|----------|
| 4 | ~~**Third site of the PTY marker-wait flake class: `TestGlobalSessionPlainBashSpawn`**~~ **RESOLVED during phase-17 post-merge gate (2026-08-28)** — root cause went deeper than the suspected deadline: the pwd-marker poll broke on file *existence*, but `pwd > marker` creates the file empty before writing, so under host load the read caught `""` (failure at 10.24s with a 20s deadline proved the deadline was never the issue). Fixes: (a) non-empty-content guard in the marker poll (same guard `readStubFile` already had), (b) 8ed08ad-class deadline headroom across the fixed marker/exit-wait waits in `sessions_global_test.go` (8s→20s at the two D-14 sites + `waitGlobalSessionExited`, 3s→10s `readStubFile`, 5s→15s opencode-csid poll), (c) `awaitHasSession` (sessions_test.go) 10s→20s — the D-14 SIGTERM→5s-grace→SIGKILL→tmux-reap path exceeded 10s once (`TestSessionTmuxReattachPreservesCustomLabel`). Polls return on condition — only failure paths get headroom. Evidence: gate FAIL(run1) → PASS → FAIL → PASS → FAIL(10.24s, empty-marker) → FAIL(awaitHasSession) → **3 consecutive full-package PASSes post-fix** (~253s each). | 2026-08-28, during 17-04 full-package regressions; re-diagnosed at the post-merge gate | closed | this commit; gate transcript + /tmp run logs api-run{4..10} in the phase-17 execution session |
