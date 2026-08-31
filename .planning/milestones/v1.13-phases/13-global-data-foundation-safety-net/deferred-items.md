# Deferred Items — Phase 13

## Pre-existing test failures (out of scope, verified at 13-01 final commit `1d10016`)

Logged during 13-02 execution on 2026-08-25. All out-of-scope failures below reproduce
identically BEFORE any 13-02 change (verified in throwaway worktrees at `1d10016`), so they
are not 13-02 regressions. The phase's verification gate is "no regression to
sessions/reaper/agent surfaces" — these were already red.

- `internal/api` — `TestInput_Happy_WritesAndAppendsCR`, `TestInput_TrailingLF_TranslatedToCR`,
  `TestInput_EmptyMessage_WritesBareCR` (sessions_test.go): PTY input tests fail with
  `bytes_written = 36, want 34` and the marker never appearing in output (terminal shows
  `Command '~echo' not found`). Session-input surface is explicitly untouched by Phase 13
  (13-RESEARCH Pitfall 7); failure looks environment-dependent (bash/terminal behavior on
  this host). Needs a dedicated investigation task — not fixable in-scope here.

- `internal/session` — `TestCustomEngineDoesNotGetHookEnv` (opencode_engine_test.go): fails with
  KAMACU_SESSION_ID/HOOK_TOKEN/HOOK_BASE present in the custom-engine child env ("D014 injection
  must be opencode-gated"). Also verified failing identically at `1d10016`. The session engine is
  explicitly untouched by Phase 13 (Pitfall 7); needs a dedicated M002-follow-up investigation.

## Resolved during 13-02 (phase's own schema surface — in-plan Rule-1 fix)

- `internal/api` — `TestBackfillAgents` was failing at `1d10016`: migration 00017's
  `global_task.agent_id ON DELETE RESTRICT` refused the test's raw DELETE of the default agent.
  Fixed in commit `1034a78` — the wipe simulation now drops the singleton first (the honest
  hand-wiped-agents install shape; BackfillGlobalTask re-arms it next boot).
