# Phase 17: Hardening & E2E - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-27
**Phase:** 17-Hardening & E2E
**Areas discussed:** Restart E2E mechanics, Resume-proof realism, Leak/interlock proof form, UAT shape + leftovers

---

## Restart E2E mechanics

| Option | Description | Selected |
|--------|-------------|----------|
| Real-binary Go test | Host-gated test spawns real `bin/kamacu serve` (Phase-14 curl-smoke env isolation: sandbox HOME, temp --db, free port), drives spawn via HTTP, SIGTERMs, restarts against same DB, asserts resume offers + tmux ghost rows | ✓ |
| Shell script | A smoke.sh-style bash script doing the spawn→kill→restart→curl cycle; human/CI runs it | |
| Sims + UAT only | Extend in-process restart-sims only; real-restart proof rides the manual UAT | |

**User's choice:** Real-binary Go test
**Notes:** —

| Option | Description | Selected |
|--------|-------------|----------|
| Fold into restart E2E | SC1's full cycle (live → 409 with reasons → stop → change succeeds → resume ids cleared) rides the same harness as one curl story | ✓ |
| API-level + UAT replay | SC1 stays at the existing API-integration tests + Phase-17 UAT replays through Settings; the restart E2E covers SC2 only | |

**User's choice:** Fold into restart E2E
**Notes:** Phase-16 UAT already surfaced the 409 dialog flow live; no need to re-verify Settings UX.

| Option | Description | Selected |
|--------|-------------|----------|
| API-level assertions | After restart, global tmux row survives (GET /api/sessions scope=global) with label preserved + tmux has-session alive — v1.8 GAP-01 replay at API level; browser visual in UAT | ✓ |
| API + pane content | Additionally `tmux capture-pane` content check proving the shell process survived (stronger, more brittle) | |

**User's choice:** API-level assertions
**Notes:** —

| Option | Description | Selected |
|--------|-------------|----------|
| Inline, host-gated | Part of `go test ./...`, LookPath skips when prerequisites absent (house convention) | ✓ |
| Separate make target | A `make e2e` target kept out of the normal suite | |

**User's choice:** Inline, host-gated
**Notes:** —

## Resume-proof realism

| Option | Description | Selected |
|--------|-------------|----------|
| Fake argv + UAT | fake-claude in the real-binary harness proves resume argv carries persisted csid + transcript-gate 409; the real same-conversation judgment is UAT's job | ✓ |
| Real claude host-gated | Additionally gate a real claude binary test (needs auth + quota; content assertions are judgment calls) | |

**User's choice:** Fake argv + UAT
**Notes:** —

| Option | Description | Selected |
|--------|-------------|----------|
| Real opencode host-gated | Extend the existing harness (TestGlobalOpencodeCaptureHost prior art) through capture → real restart → resume argv → process back | ✓ |
| Stub + UAT parity | opencode matches claude's treatment: stub/fake argv proof automated + real resume in UAT | |

**User's choice:** Real opencode host-gated
**Notes:** opencode provisioned on this host (1.18.22); runs locally without quota anxiety.

| Option | Description | Selected |
|--------|-------------|----------|
| Both engines in UAT | One UAT flow drives both engines' real resume: identifiable question → stop/restart → Resume → confirm it remembers | ✓ |
| claude only in UAT | UAT verifies claude's real conversation resume; opencode fully covered by host-gated automation | |

**User's choice:** Both engines in UAT
**Notes:** —

| Option | Description | Selected |
|--------|-------------|----------|
| Include 409 edges | Harness also replays the D-31 family: resume with no persisted id, claude transcript vanished, opencode stale ocsid | ✓ |
| Happy paths only | Phase-15 API tests already cover the edges; E2E sticks to happy resume cycles | |

**User's choice:** Include 409 edges
**Notes:** At API level where not already test-covered.

## Leak/interlock proof form

| Option | Description | Selected |
|--------|-------------|----------|
| Tests + audit table | Permanent regression tests for every surface AND a documented enumeration table in VERIFICATION (surface → query/guard → assertion) | ✓ |
| Tests only | Regression tests, no separate audit document | |
| Audit only | One-time documented sweep, no permanent tests | |

**User's choice:** Tests + audit table
**Notes:** —

| Option | Description | Selected |
|--------|-------------|----------|
| One consolidated test | A single TestGlobalNoLeak* file walking every enumerated surface with a configured+live global session — legible, extensible | ✓ |
| Spread per-surface | Individual assertions in each surface's existing test files | |

**User's choice:** One consolidated test
**Notes:** —

| Option | Description | Selected |
|--------|-------------|----------|
| Both directions via bridge | Real MCP bridge (in-memory transport): task tools never surface anything global AND session tools list/describe the live global session with honest labels, never 404 (GINT-02) | ✓ |
| Leake direction only | Task tools clean; GINT-02's honest-labels half already covered by Phase-15 tests | |

**User's choice:** Both directions via bridge
**Notes:** —

| Option | Description | Selected |
|--------|-------------|----------|
| Automated both dirs | Managed project clone + managed global root (same repo allowed): gated project delete removes only the project clone; global clear/reconfigure touches nothing project-side; folder trivial direction asserted cheaply | ✓ |
| Managed only | Managed-to-managed direction only; folder stays reasoned in VERIFICATION | |

**User's choice:** Automated both dirs
**Notes:** —

## UAT shape + leftovers

| Option | Description | Selected |
|--------|-------------|----------|
| Walkthrough doc | The 16-UAT.md pattern: numbered flows with expected results, human drives the browser, results in 17-UAT.md | ✓ |
| Doc + setup script | Walkthrough plus a helper bash script automating curl-able setup steps | |

**User's choice:** Walkthrough doc
**Notes:** —

| Option | Description | Selected |
|--------|-------------|----------|
| Real repo, your pick | A real repo picked at UAT time (e.g. the kamacu checkout — dogfooding); honest no-worktree posture with a skip-permissions agent in a real checkout | ✓ |
| Temp fixture repo | A dedicated throwaway git repo — zero blast radius, slightly less real | |

**User's choice:** Real repo, your pick
**Notes:** —

| Option | Description | Selected |
|--------|-------------|----------|
| Decide at UAT | Keep text-only unless the walkthrough reveals a real confusion need; decide with the live bar in front of you (as D-12 locked) | ✓ |
| Keep text-only now | Pre-decide: no badge, D-12 closes without visual change | |
| Add globe badge now | Pre-decide: add the badge in this phase | |

**User's choice:** Decide at UAT
**Notes:** —

| Option | Description | Selected |
|--------|-------------|----------|
| Fold in | types.ts Agent.engine union widening + TaskPage QuotaIndicator predicate cleanup as a small type-only task | ✓ |
| Leave deferred | Post-milestone cleanup; Phase 17 stays verification-only | |

**User's choice:** Fold in
**Notes:** —

| Option | Description | Selected |
|--------|-------------|----------|
| Investigate + fix | Bounded debug-and-fix task for the TestInput_* family AND TestCustomEngineDoesNotGetHookEnv — a hardening phase shouldn't ship with known-red tests | ✓ |
| TestInput_* only | Session-input surface only; the opencode-engine env test stays deferred | |
| Stay deferred | Both stay deferred with documented rationale; verification gates exclude them | |

**User's choice:** Investigate + fix
**Notes:** Root-cause fix preferred over expectation-adjustment; the investigation decides.

## the agent's Discretion

- Harness file placement, binary-build strategy in tests, timeout budgets, sandbox teardown
- Test function naming and curl-story narrative splitting
- Audit table mechanics (query enumeration depth)
- Helper sharing/extraction with existing global tests
- Red-test fix approach (root cause vs expectation correction — investigation decides)
- UAT doc exact flow numbering and copy

## Deferred Ideas

None — discussion stayed within phase scope. GT-FUT-01..07 remain parked; D-12 settles at this phase's UAT; red tests and stale-union debt folded in.
