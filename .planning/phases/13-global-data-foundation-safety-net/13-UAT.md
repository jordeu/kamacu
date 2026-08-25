---
status: testing
phase: 13-global-data-foundation-safety-net
source: [13-VERIFICATION.md]
started: 2026-08-25T16:35:00Z
updated: 2026-08-25T16:35:00Z
---

## Current Test

number: 1
name: Adjudicate CR-01 — delete-default → unbootable-app chain (agents_crud.go)
expected: |
  Developer decision: fix now as a phase-13 gap-closure (add is_default guard → 409
  "set another default agent first" + regression test), or explicitly schedule before
  ship (no later roadmap phase covers it — Phase 17's SCs are global-flow E2E, not
  agents CRUD). Pre-existing M001 defect chain, adjacent to this phase's guard.
awaiting: user response

## Tests

### 1. Adjudicate CR-01 — delete-default → unbootable-app chain (agents_crud.go)
expected: Fix now as phase-13 gap-closure (is_default guard 409 + regression test) or explicitly schedule before ship; verifier judges it NOT a phase-13 gap (pre-existing, referenced-agent delete IS refused)
result: [pending]

### 2. Adjudicate WR-01 — BackfillGlobalTask silent zero-row insert when no default exists
expected: Add RowsAffected()==1 fail-loud guard + zero-default test, or accept as-is (unreachable through real boot pipeline — BackfillAgents re-seeds a default first; bites only direct/future callers)
result: [pending]

### 3. Adjudicate WR-02 — engine allowlist rejects 'opencode' same-value round-trips (pre-existing M002)
expected: Schedule the allowlist fix (accept current engine value) in a later phase or dedicated task; no current UI path trips it
result: [pending]

### 4. Schedule investigation of the 4 pre-existing red tests before Phase 15
expected: TestInput_Happy_WritesAndAppendsCR, TestInput_TrailingLF_TranslatedToCR, TestInput_EmptyMessage_WritesBareCR (internal/api), TestCustomEngineDoesNotGetHookEnv (internal/session) — dedicated investigation task before Phase 15 (risk center); verified pre-existing at base cc9bb03, logged in deferred-items.md, no roadmap phase owns them
result: [pending]

## Summary

total: 4
passed: 0
issues: 0
pending: 4
skipped: 0
blocked: 0

## Gaps
