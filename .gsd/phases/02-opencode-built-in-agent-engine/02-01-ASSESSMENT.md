---
sliceId: S01
uatType: runtime-executable
verdict: PASS
attempt: 1
runId: uat:M002:S01:attempt-1
worktreeRoot: /home/jordi/workspace/github/kangent/.gsd-worktrees/M002
date: 2026-07-07T18:50:07.932Z
---

# UAT Result - S01

## Checks

| Check | Mode | Result | Evidence | Notes |
|-------|------|--------|----------|-------|
| Precondition: module kamacu builds and vets clean (go build ./..., go vet ./...). | runtime | PASS | gsd_uat_exec:db4a8c3b-9978-490e-b87f-91116c859b04 | go build ./... exit 0; go vet ./... exit 0. Supporting precondition for all runtime checks. |
| opencode is a non-deletable system agent seed: exactly one opencode row engine='opencode', command='opencode', is_default=0, is_system=1; claude remains sole is_default=1 (R019); idempotent; is_system blocks deletion (R018). | runtime | PASS | gsd_uat_exec:4326ce39-35ec-4b8a-bdce-fae7c356bbd6<br>gsd_uat_exec:d8558b8a-d69a-4780-9b86-fb1da0266762 | Store tests assert engine='opencode', command='opencode', is_system=1, is_default=0, claude sole is_default=1, idempotent re-run (NOT EXISTS guard), FK re-armed ON, plus fresh-install + upgrade paths. BackfillOpenCodeAgent (agents_backfill.go:87) wired at main.go:161 re-creates the seed if dropped (edge case: drop seed -> reboot re-creates it). |
| opencode spawn injects KAMACU_SESSION_ID==session.ID, KAMACU_HOOK_TOKEN, KAMACU_HOOK_BASE for opencode-engine child; all three ABSENT for custom-engine child (R017 byte-for-byte guard); status in {working,idle,waiting} never running. | runtime | PASS | gsd_uat_exec:2ed1675d-0167-4021-9668-6b0935a6f2c6<br>gsd_uat_exec:0a35f441-d361-4b82-9244-8617771b6cd7 | Opencode -run pattern matched 2 tests; TestCustomEngineDoesNotGetHookEnv (R017 guard) runs under the Custom filter and passes. KAMACU_SESSION_ID equals s.Info().ID; TOKEN/BASE present for opencode, ABSENT for custom; heuristic states exclude 'running'. |
| Env-gated plugin installs idempotently with no-pollution and correct content: 7 cases; content carries kamacu-managed header + 3 env vars + SessionStart/Notification/Stop + X-Kangent-Token + singular plugin/ path + singleton guard + parentID child-suppression. | runtime | PASS | gsd_uat_exec:81daa853-1d8e-4ad1-98be-1c0c3a7f6a73<br>gsd_uat_exec:db847781-59db-4a2b-9409-612b0dc5d0d2 | 7 cases pass: NoopWhenOpencodeAbsent (no-pollution), NoopWhenTargetIsFile, WritesWhenOpencodePresent, IdempotentSkip, OverwritesStale, ConfigDirError, SourceMatchesFile. Content grep: kamacu-managed=1, KAMACU_SESSION_ID/HOOK_TOKEN/HOOK_BASE=2 each, SessionStart/Notification=2 each, Stop=12, X-Kangent-Token=1, plugin/=2, parentID=2, plugins/(plural)=0. Singleton guard implemented as byte-identical skip-on-match (plugin.go:87-89), proven by TestInstallPluginIdempotentSkip (literal word 'singleton' not used but behavior present). |
| UNCHANGED hook receiver drives opencode working/waiting/idle + hooksAlive canary: 5 cases; never running; Notification->waiting; Stop->idle; SessionStart->hooksAlive (BEL fallback disabled, hooks-dead control); full one-turn loop. hooks.go unmodified. | runtime | PASS | gsd_uat_exec:2dd0b4ac-51d8-42ae-84c0-aa232d85fa98 | KeepsHeuristicStates (never running), NotificationSetsWaiting, StopSetsIdle, SessionStartMarksHooksAlive (BEL fallback disabled w/ hooks-dead control), FullTurnLoop all PASS. Receiver exercised is the unchanged claude hook receiver reused for opencode. |
| Regression: claude and custom engines unchanged. fake-claude suite (R016) and custom-engine suite (R017) pass unchanged; engine-agnostic hook boundary suite passes. | runtime | PASS | gsd_uat_exec:6617d4e0-7439-4dc2-8f7c-a2bb7b8d4295 | go test ./internal/session/ -run 'Custom\|Claude' exit 0; go test ./internal/api/ -run Hook exit 0. No regressions in the claude/custom spawn or hook paths. |
| Integration sanity: full (unfiltered) test run across all four touched packages is green; no other regressions. | runtime | PASS | gsd_uat_exec:ffe90bbc-4b5b-4f69-9c1e-1ebca7469600<br>gsd_uat_exec:b8fe971d-68ee-4605-95a0-e5e8f9b1fd69 | store ok (0.370s), session ok (5.620s), opencode ok (0.013s), api ok (87.845s) — full-package green; api is slow due to pre-existing PTY/session integration tests, not an M002 regression. |

## Overall Verdict

PASS - All five automatable runtime-contract checks (UAT-01..UAT-05) PASS, plus build/vet clean (UAT-BUILD) and full-package green across store/session/opencode/api (UAT-FULL-PKG; api 87.8s slow-but-green from pre-existing PTY tests, no M002 regression). The browser-visible LIVE opencode status-dot check is explicitly scoped OUT of this slice to the M002 milestone UAT gate as human-follow-up per the slice's own spec ("This slice's UAT scope is the runtime contract"); its precondition is now SATISFIED (opencode v1.17.15 at ~/.opencode/bin/opencode, ~/.config/opencode present), so the milestone gate can run it. Recorded in evidence 9ae2c4c3.

## Tool Presentation

```json
{
  "blockedTools": [
    {
      "name": "edit",
      "reason": "forbidden during run-uat"
    },
    {
      "name": "write",
      "reason": "forbidden during run-uat"
    },
    {
      "name": "gsd_exec",
      "reason": "forbidden during run-uat"
    },
    {
      "name": "gsd_summary_save",
      "reason": "forbidden during run-uat"
    },
    {
      "name": "gsd_save_gate_result",
      "reason": "forbidden during run-uat"
    },
    {
      "name": "search-the-web",
      "reason": "forbidden during run-uat"
    },
    {
      "name": "WebSearch",
      "reason": "forbidden during run-uat"
    },
    {
      "name": "Bash",
      "reason": "forbidden during run-uat"
    },
    {
      "name": "Write",
      "reason": "forbidden during run-uat"
    },
    {
      "name": "Edit",
      "reason": "forbidden during run-uat"
    }
  ],
  "presentedTools": [
    "gsd_uat_exec",
    "gsd_uat_result_save",
    "gsd_resume",
    "gsd_milestone_status",
    "gsd_journal_query",
    "find",
    "glob",
    "grep",
    "ls",
    "read"
  ],
  "surface": "mcp",
  "toolPresentationPlanId": "run-uat/default-v1"
}
```

## Gate

Aggregate UAT gate saved as pass.
