---
id: S03
parent: M002
milestone: M002
provides:
  - Nullable tasks.opencode_session_id column (migration 00016) — the restart-resume key for opencode
  - opencode resume argv (opencode -s <id>) + engine-gated resumable flag
  - Async subprocess-discovery capture path that persists the self-minted ses_id for a live opencode session
  - Permanent fake-opencode argv regression stub (testdata/fake-opencode)
  - PWD=worktree invariant pinned in opencode spawn + discovery so directory==worktree matching works in production
requires:
  - slice: S01
    provides: opencode engine seed, D014 KAMACU_SESSION_ID/HOOK_TOKEN/HOOK_BASE spawn env, on-disk env-gated status plugin, agentStatusLocked opencode full-heuristics gate
  - slice: S02
    provides: fetch-based status plugin, //go:build opencode_e2e real-binary harness pattern, proven plugin->receiver loop, httptest receiver mirroring the production X-Kangent-Token contract
affects:
  []
key_files:
  - internal/store/migrations/00016_opencode_session_id.sql
  - internal/store/opencode_session_id_test.go
  - internal/api/sessions.go
  - internal/api/agents.go
  - internal/api/testdata/fake-opencode
  - internal/api/opencode_resume_test.go
  - internal/api/opencode_capture_test.go
  - internal/opencode/e2e_test.go
  - internal/session/manager.go
  - internal/opencode/kamacu-status.js
  - internal/opencode/plugin_test.go
key_decisions:
  - Strategy B (subprocess discovery via `opencode session list --format json`) over Strategy A (plugin-capture) — host-gated spike proved opencode 1.17.15 does not fire session.created for a TUI spawn (MEM034)
  - Engine-branched resume-validation + resumable derivation on agentEngine=="opencode" keying off persisted opencode_session_id alone — NO claude transcriptExists glob, which is always false for opencode (MEM033)
  - opencode resume argv is `opencode -s <id>` appended to AgentArgs in the custom spawn arm — NOT claude's --resume (MEM027 forbids mixing); opts.ResumeSessionID stays empty for opencode
  - Committed fake-opencode argv stub mirrors fake-claude's FAKE_OPENCODE_ARGS_FILE diagnostic, locking the exact fresh-vs-resume argv so it can never drift
  - PWD=worktree injection in BOTH the opencode spawn (manager.go) and the discovery subprocess — opencode resolves session directory from $PWD not getcwd() (MEM035/MEM036)
  - Capture is fresh-spawn-only bounded async poll; resume reuses the stored id and never re-discovers (MEM037); all capture failures warn-only (best-effort, never costs the live session)
patterns_established:
  - Per-engine resume/resumable branching: there is no generic 'resumable' notion — each engine owns its persisted resume key + its resume argv; any future engine must add its own branch to both the resume-validation block (sessions.go) and the resumable derivation (agents.go)
  - Injectable package var (discoverOpenCodeSession) for external-binary discovery so CI can stub it and run no real opencode, while host-gated //go:build e2e tests exercise the real binary
  - Committed fake-<engine> argv-recording stubs (fake-claude, fake-opencode) as permanent fresh-vs-resume argv regression guards
observability_surfaces:
  - resumable:true flag in GET /api/agents/status for exited opencode tasks with a stored id + worktree (engine-gated, both live manager-derived and post-restart survivor passes) — the restart-resume health signal
  - warn-level slog lines from captureOpencodeSessionAsync (internal/api/sessions.go) for capture-loop failures (opencode absent, subprocess error, PWD mismatch, JSON parser drift)
  - hooksAlive canary (from S02) remains the plugin->receiver health signal
drill_down_paths:
  - .gsd/phases/02-opencode-built-in-agent-engine/T01-SUMMARY.md
  - .gsd/phases/02-opencode-built-in-agent-engine/T02-SUMMARY.md
  - .gsd/phases/02-opencode-built-in-agent-engine/T03-SUMMARY.md
duration: ""
verification_result: passed
completed_at: 2026-07-08T03:38:13.600Z
blocker_discovered: false
---

# S03: Session resume and argv regression hardening

**opencode tasks now persist their self-minted session id (tasks.opencode_session_id) and resume via `opencode -s <id>` after a kamacu restart, with the exact fresh-vs-resume spawn argv locked behind a committed fake-opencode regression stub.**

## What Happened

S03 closes the restart-resume loop for opencode tasks and locks the spawn argv behind a permanent regression guard. Three tasks, all CI-green plus a host-gated real-binary e2e.

**T01 — data model (internal/store):** Added nullable `tasks.opencode_session_id TEXT` (migration 00016), the opencode analog of `tasks.claude_session_id`. Unlike claude — where kamacu mints `--session-id <uuid>` and always knows it — opencode mints an opaque `ses_…` internally, so kamacu must DISCOVER and PERSIST that id for a restart to resume. 00016 is plain transactional DDL (nullable ADD COLUMN); tests pin idempotency on both the upgrade (stage at 00015) and fresh-install paths, plus nullable-not-NOT-NULL and FK-re-armed guards. No runtime behavior change.

**T02 — resume argv + resumable flag (internal/api):** Engine-branched the resume-validation block (sessions.go) and the resumable derivation (agents.go) on `agentEngine=="opencode"`. Opencode keys off the persisted `opencode_session_id` alone — NO claude `transcriptExists` glob (opencode has no ~/.claude/projects jsonl files, so that glob is always false for it). On resume, the custom spawn arm appends `["-s", ocsid]` -> `opencode -s <id>`; a fresh spawn stays `["opencode"]` (no -s). `opts.ResumeSessionID` stays "" for opencode — it does NOT route through claude's `--resume` (MEM027). The post-restart survivor pass widened to `(claude_session_id IS NOT NULL OR opencode_session_id IS NOT NULL) AND worktree_path IS NOT NULL`, engine-gated per row. A committed `testdata/fake-opencode` stub (mirroring fake-claude) records the exact argv via `FAKE_OPENCODE_ARGS_FILE` so the fresh-vs-resume argv can never drift.

**T03 — capture path (internal/api + internal/session + internal/opencode):** Spike proved opencode 1.17.15 does NOT fire `session.created` for a TUI spawn (MEM034, extends MEM029's run-mode finding), ruling out Strategy A (plugin-capture). Implemented Strategy B (subprocess discovery): a background goroutine (`captureOpencodeSessionAsync`) launched for a FRESH opencode spawn only, polling `opencode session list --format json` (filtered `directory==worktree`, most-recently-updated) until the ses_id appears, then UPDATEing `tasks.opencode_session_id`. The opencode session row only appears AFTER the user's first turn (TUI boot writes nothing), so capture is a bounded poll (2s interval, 10min active window, 60min hard cap) with a final best-effort attempt on exit — never a spawn-time read. The host-gated e2e surfaced a load-bearing correctness fix (MEM035/MEM036): opencode resolves a session's `directory` from `$PWD`, not getcwd(), and Go's `exec.Cmd.Dir` does NOT update `$PWD`. Both the opencode spawn (internal/session/manager.go) and the discovery subprocess now inject `PWD=<worktree>` so the `directory==worktree` filter matches in production.

**Regression posture:** The claude and custom spawn/status paths are byte-for-byte unchanged — TestRecoveryLifecycle (fake-claude R016), TestCustomEngineDoesNotGetHookEnv (R017), and the S01 opencode hook-status integration all pass. No frontend change: `useResumeAgent` already sends `{task_id, kind:"agent", resume:true}` engine-agnostically and the Resume button shows whenever resumable is true.

## Operational Readiness (Q8)

**Health signal (proves the slice works):** After the user's first turn in an opencode task, the capture loop writes `tasks.opencode_session_id`; the observable success signal is `GET /api/agents/status` reporting `resumable: true` for an EXITED opencode task with a stored id + worktree (both the live manager-derived pass and the post-restart survivor pass). After a kamacu restart, the opencode task's Resume button appears and clicking it spawns `opencode -s <ses_id>`.

**Failure signal:** Capture is best-effort and warn-only — if it fails (opencode not on PATH, session-list subprocess error, PWD mismatch, JSON parser drift), the ses_id stays NULL and the task silently becomes non-resumable after restart. The failure signal is warn-level `slog` lines from `captureOpencodeSessionAsync` in internal/api/sessions.go. A persisted stale/invalid ses_id does NOT fail silently: `opencode -s <bad-id>` surfaces "Session not found" in the terminal (proven by TestE2E_ResumeStaleIdErrors) — honest error, never a silent fresh fork.

**Recovery procedure:** A non-resumable opencode task is recovered by re-running the task (fresh spawn re-triggers capture; resume never re-discovers) or starting a fresh opencode session via the existing "Reset session" affordance. A stale-id resume error is dismissed and a fresh session started.

**Monitoring gaps:** Capture success/failure is only visible in server logs and only verifiable post-exit via the resumable flag — there is no live UI badge showing "session captured" vs "capture pending/failed" for an in-flight opencode task. This is a deliberate best-effort trade-off (capture never costs the live session) and a known limitation. Capture goroutines are bounded per fresh spawn by the session lifecycle + 60min hard cap.

## Verification

Ran fresh slice-level verification (all green). Build/vet: `go build ./...` and `go vet ./...` clean. T01: `go test ./internal/store/ -count=1` PASS (00016 idempotent on upgrade + fresh). T02/T03 CI core: `go test ./internal/api/ -run Opencode -count=1` PASS — 13 tests (resume argv `[fake-opencode, -s, ses_test123]`; fresh argv `[fake-opencode]` no -s; resumable:true for exited opencode task; 409 on NULL id; capture persist/poll-until-appears/no-match-stays-null/resume-no-rediscover; 5 S01 hook-status). T03 plugin: `go test ./internal/opencode/ -count=1` PASS. T03 real-binary: `go build -tags opencode_e2e ./internal/opencode/` clean + `go test -tags opencode_e2e ./internal/opencode/ -run E2E -count=1` PASS (4 tests vs opencode 1.17.15: TestE2E_DiscoverAndResumeById positive resume + TestE2E_ResumeStaleIdErrors stale-id "Session not found"). Regression guards: `go test ./internal/session/ -run 'Opencode|Custom|Claude'` PASS; `go test ./internal/api/ -run 'Custom|Resume'` PASS (R016/R017); `go test ./internal/api/ -run 'OpencodeHook'` PASS (S01); named `TestCustomEngineDoesNotGetHookEnv` (R017) PASS and `TestRecoveryLifecycle` (fake-claude R016) PASS. All must-haves met; no -s leaks into a fresh opencode spawn; opencode resume never routes through claude's --resume; capture is async (ses_id arrives via SessionStart/discovery AFTER boot, not at spawn return).

## Requirements Advanced

None.

## Requirements Validated

- R016 — TestRecoveryLifecycle (fake-claude) + api Custom/Resume suite stay green — the claude spawn/resume path is byte-for-byte unchanged after the opencode engine-branching
- R017 — TestCustomEngineDoesNotGetHookEnv + api Custom suite stay green — the custom spawn path is unchanged; the PWD=worktree env addition is opencode-only and did not perturb the custom-env guard

## New Requirements Surfaced

None.

## Requirements Invalidated or Re-scoped

None.

## Operational Readiness

None.

## Deviations

1. T03 chose Strategy B (subprocess discovery) over the plan's Strategy A (plugin-capture) — the host-gated spike proved opencode 1.17.15 does not fire session.created for a TUI spawn (MEM034). The column (T01) and resume argv (T02) were strategy-invariant, so the CI-green argv hardening landed regardless of the spike outcome. 2. T03's capture poll is longer than the plan's "short bounded retry/poll" — 10min active window + 60min hard cap plus an on-exit final attempt, because the spike proved the opencode session row only appears AFTER the user's first turn (a TUI boot writes nothing). Consistent with the plan's async-capture-timing pitfall note. 3. T03 added a necessary correctness fix not anticipated by the plan: PWD=worktree injection in BOTH manager.Spawn and the discovery subprocess (MEM035/MEM036) — without it opencode records kamacu's launch dir as the session directory and the production directory==worktree filter never matches. Discovered via the host-gated e2e (its exact purpose). 4. T03 touched internal/session/manager.go (in the plan's Strategy-A file list) only for the one-line PWD addition — not the SetAgentSessionID/manager-hook wiring that was Strategy A's design; the session package stays DB-free, all persistence is in the api layer.

## Known Limitations

1. Capture is best-effort/warn-only; there is no live UI badge showing capture success/failure for an in-flight opencode task — the only post-hoc signal is the resumable flag after exit. 2. The "prior conversation visible in the resumed TUI" assertion (strongest possible resume proof) is deferred to the human milestone UAT (restart kamacu, click Resume, confirm prior conversation in the browser terminal) — it requires a working provider + interactive TUI; the host-gated e2e proves the deterministic id-resolution signal (real id resolves, stale id errors) which is provider-independent. 3. TestE2E_DiscoverAndResumeById (positive proof) can t.Skip if a flaky provider errors before opencode creates a session row on a given host; the deterministic negative stale-id proof (TestE2E_ResumeStaleIdErrors) is the always-reliable DONE-WHEN backstop — both passed on this host.

## Follow-ups

1. Live browser restart-resume (restart kamacu, click Resume, confirm prior conversation in the browser terminal) remains the human milestone UAT gate — not automatable in CI. 2. Optional: add a live UI indicator (badge) for opencode capture status (pending/succeeded/failed) so users can see resume-readiness before exit; currently only the post-exit resumable flag reveals capture outcome.

## Files Created/Modified

- `internal/store/migrations/00016_opencode_session_id.sql` — New: nullable ALTER TABLE tasks ADD COLUMN opencode_session_id TEXT (Up/Down)
- `internal/store/opencode_session_id_test.go` — New: migration idempotency tests (upgrade stage-at-00015 + fresh install, nullable/FK-re-armed guards)
- `internal/api/sessions.go` — Engine-branched resume validation (opencode keys off opencode_session_id, no transcript glob); opencode resume argv [-s <id>] in custom spawn arm; async subprocess-discovery capture (captureOpencodeSessionAsync, discoverOpenCodeSession, parseOpenCodeSessionList)
- `internal/api/agents.go` — Engine-gated resumable derivation (opencode: exited+ocsid+worktree) + widened post-restart survivor pass WHERE (claude OR opencode id) AND worktree
- `internal/api/testdata/fake-opencode` — New committed argv-recording stub mirroring fake-claude (FAKE_OPENCODE_ARGS_FILE)
- `internal/api/opencode_resume_test.go` — New: fresh-vs-resume argv, resumable flag, 409-on-NULL-id tests
- `internal/api/opencode_capture_test.go` — New: capture persist/poll/no-match/resume-no-rediscover + parser unit tests (stubbed discovery)
- `internal/opencode/e2e_test.go` — New host-gated //go:build opencode_e2e: real-binary discover+resume-by-id + stale-id error proofs
- `internal/session/manager.go` — PWD=worktree env injection in opencode spawn arm (load-bearing for session directory matching)
- `internal/opencode/kamacu-status.js` — Status plugin (carry-over from S01/S02; rootSessionID/session.created present but unused for capture per MEM034)
