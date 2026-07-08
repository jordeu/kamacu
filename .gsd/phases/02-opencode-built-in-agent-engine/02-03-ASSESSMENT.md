---
sliceId: S03
uatType: browser-executable
verdict: PASS
attempt: 1
runId: uat:M002:S03:attempt-1
worktreeRoot: /home/jordi/workspace/github/kangent/.gsd-worktrees/M002
date: 2026-07-08T04:03:26.294Z
---

# UAT Result - S03

## Checks

| Check | Mode | Result | Evidence | Notes |
|-------|------|--------|----------|-------|
| Migration 00016 is idempotent (upgrade stage-at-00015 + fresh install); store tests green | runtime | PASS | gsd_uat_exec:541b9b13-60b9-410a-a56e-4f0831bf2a36 | Fresh-install + upgrade-stage both reach version 16 with 00016_opencode_session_id.sql applied exactly once. |
| 13 Opencode API tests: resume argv [fake-opencode,-s,ses_test123], fresh argv [fake-opencode] (no -s), resumable:true for exited opencode task, 409 on NULL id, capture persist/poll-until-appears/no-match-stays-null/resume-no-rediscover, 5 S01 hook-status tests | runtime | PASS | gsd_uat_exec:e8b46e27-01ff-409a-a015-44b9b97ae294 | Specifically PASSed: TestOpencodeFreshSpawnArgvHasNoResumeFlag (fresh argv has no -s), TestOpencodeResumeSpawnAppendsSessionFlag (resume appends -s), TestOpencodeExitedTaskIsResumable, TestOpencodeResumeWithoutStoredIdIs409. |
| Real opencode 1.17.15 e2e: TestE2E_DiscoverAndResumeById resolves, TestE2E_ResumeStaleIdErrors -> 'Session not found' | runtime | PASS | gsd_uat_exec:297a1d0d-2833-438b-8e40-11c324be7567 | opencode 1.17.15 on PATH at /home/jordi/.opencode/bin/opencode (env check ref 0fde8b36). Real id resolves; stale id surfaces 'Session not found' honestly, no silent fresh fork. |
| Regression guards: TestRecoveryLifecycle (fake-claude R016), TestCustomEngineDoesNotGetHookEnv (R017), OpencodeHook suite (S01) all green | runtime | PASS | gsd_uat_exec:12f3e8eb-ec1f-4058-ba09-e5c12f35cdc7 | Claude resume path (--resume) unchanged and green; custom non-claude/non-opencode agent gets no hook env; S01 OpencodeHook suite intact. |
| kamacu single binary boots and serves SPA + API at localhost; on a clean DB migration 00015 seeds the engine='opencode' system agent | browser | PASS | gsd_uat_exec:5c0c9cf2-97d9-469e-8368-a9a3a1a43a61<br>gsd_uat_exec:224a4c8c-dd7d-4f92-903f-8d99401be37b<br>browser:.artifacts/browser/2026-07-08T04-01-19-402Z-s03-uat-fresh-boot | Single binary boots and serves SPA+API at localhost; clean install seeds the engine='opencode' system agent via migration 00015 (resolving an earlier scratch-DB observation where only a user-created engine='custom' OpenCode agent existed). |
| Live interactive restart-resume proof: with an engine='opencode' agent, create task, send a real turn, exit, confirm resumable:true, restart kamacu, confirm still resumable:true, click Resume (opencode -s <id>) and confirm prior conversation visible/restored in the browser terminal | human-follow-up | NEEDS-HUMAN | gsd_uat_exec:297a1d0d-2833-438b-8e40-11c324be7567 | HUMAN FOLLOW-UP: (1) point a project at the engine='opencode' system agent; (2) create task, send a real opencode turn, exit; (3) confirm resumable:true via GET /api/agents/status; (4) kill+relaunch kamacu; (5) confirm task still resumable:true after restart; (6) click Resume and confirm prior conversation visible in the browser terminal. Not automatable in CI; e2e covers the deterministic id-resolution contract. |

## Overall Verdict

PASS - All automatable checks pass: 4 runtime groups (migration 00016 idempotent, 13/13 Opencode API argv+resumable+409 tests, real-opencode 1.17.15 e2e resolving+stale-id, claude/custom regression suites) plus a live browser check proving the single binary boots/serves SPA+API and a clean DB seeds the engine='opencode' system agent via migration 00015. The interactive restart-resume with prior-conversation-visible is the explicitly non-automatable human milestone gate (NEEDS-HUMAN); its provider-independent deterministic signal is proven by the e2e.

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
    "read",
    "browser_navigate",
    "browser_click",
    "browser_type",
    "browser_fill_form",
    "browser_click_ref",
    "browser_fill_ref",
    "browser_wait_for",
    "browser_assert",
    "browser_verify",
    "browser_screenshot",
    "browser_snapshot_refs",
    "browser_find",
    "browser_get_console_logs",
    "browser_get_network_logs",
    "browser_evaluate",
    "browser_reload",
    "browser_batch",
    "browser_act"
  ],
  "surface": "hybrid",
  "toolPresentationPlanId": "run-uat/default-v1"
}
```

## Gate

Aggregate UAT gate saved as pass.

## Manual Validation

One or more checks are marked `NEEDS-HUMAN` and require a person to validate:

- Validate the work here: /home/jordi/workspace/github/kangent/.gsd-worktrees/M002
- This milestone runs in a git worktree, so the code lives under the GSD worktrees directory. Open it with: cd "/home/jordi/workspace/github/kangent/.gsd-worktrees/M002"
- Follow the UAT checklist at: .gsd/phases/02-opencode-built-in-agent-engine/02-03-UAT.md
