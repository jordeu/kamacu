---
phase: 13-global-data-foundation-safety-net
reviewed: 2026-08-25T16:30:00Z
depth: standard
files_reviewed: 10
files_reviewed_list:
  - cmd/kamacu/serve.go
  - cmd/kamacu/sweep_test.go
  - internal/api/agents_backfill_test.go
  - internal/api/agents_crud.go
  - internal/api/agents_crud_test.go
  - internal/api/global_backfill.go
  - internal/api/global_backfill_test.go
  - internal/store/global_task_migration_test.go
  - internal/store/migrations/00017_global_task.sql
  - internal/store/migrations/00018_tmux_scope.sql
findings:
  critical: 1
  warning: 2
  info: 2
  total: 5
status: issues_found
---

# Phase 13: Code Review Report

**Reviewed:** 2026-08-25T16:30:00Z
**Depth:** standard
**Files Reviewed:** 10
**Status:** issues_found

## Summary

Reviewed the global_task singleton migration (00017), the tmux_sessions scope rebuild (00018), the `BackfillGlobalTask` boot hook + its serve.go wiring, the scope-aware orphan-sweep query fix, the agents delete-guard 409, and the five test files covering them.

The core phase work is solid and empirically verified: the sweep query correctly spares `scope='global'` rows while still killing true orphans (regression-tested live against a real tmux server); migration 00018's rebuild preserves every row byte-for-byte, its XOR CHECK rejects both ambiguity directions, the FK is re-armed on the pooled handle (canary-verified), and the FK shape (`REFERENCES tasks(id)`, no ON DELETE clause) is exact parity with the 00005 original; migration 00017's `CHECK(id=1)` + RESTRICT + flag-sourced seed are all proven by the staged-upgrade test; all new tests pass and `go vet` is clean.

The adversarial pass found one severe defect chain adjacent to the code this phase touched: the agents `delete` handler can delete the **current default** agent, which both breaks the exactly-one-default invariant and — via `BackfillAgents`' literal re-seed colliding with the `idx_agents_name_nocase` unique index — leaves the app **permanently unbootable** on the next start. Two warnings cover `BackfillGlobalTask` silently inserting zero rows when no default agent exists, and the `update` engine allowlist rejecting the shipped `'opencode'` engine value. Two info items cover pre-existing gofmt violations in three in-scope files and a UNIQUE-constraint NULL-semantics note for Phase 15.

Per phase context: the 4 known pre-existing failures (TestInput_*, TestCustomEngineDoesNotGetHookEnv) were verified pre-existing at base and are excluded.

## Critical Issues

### CR-01: `delete` can remove the current default agent → zero defaults → next boot fails permanently on the name-unique index

**File:** `internal/api/agents_crud.go:238-292`
**Issue:** The delete handler's three guards (is_system, projects count, global_task reference) never check `is_default`. Traceable through fully supported API/UI operations today:

1. `POST /api/agents` → custom agent X (`create` forces `is_default=0`).
2. `POST /api/agents/{X}/default` (`setDefault`, shipped UI mutation `useSetDefaultAgent`) → X is_default=1, the Claude seed drops to is_default=0. `global_task.agent_id` still points at the Claude seed (nothing in shipped code reassigns it until Phase 14).
3. `DELETE /api/agents/{X}` → X is not system, referenced by 0 projects, and not the global_task agent → **204, delete succeeds. The DB now has zero `is_default=1` agents.**

Consequence on next boot: `BackfillAgents` (`internal/api/agents_backfill.go:24-36`) finds no default and re-inserts the literal seed `('Claude Code', 'claude', 'claude', 1, 1)`. The still-present Claude seed row already owns the name `Claude Code` under `idx_agents_name_nocase` (00013, UNIQUE COLLATE NOCASE), so the INSERT violates the unique index → `BackfillAgents` errors → `serve.go` refuses to boot ("backfilling agents") → **every subsequent boot fails until the user hand-edits the DB**. (If the seed was renamed, the boot instead succeeds with a *duplicate* system Claude agent — softer but still invariant-breaking.) This also degrades `BackfillGlobalTask`, whose seed reads the default agent (see WR-01).

The defect predates this phase (the delete path shipped in M001), but this phase extended this exact guard block (added the global_task 409) without closing the adjacent hole, and v1.13 adds a new boot-time consumer of the exactly-one-default invariant. With an unbootable-app outcome from a supported UI sequence, this must be fixed before ship.

**Fix:** Refuse to delete the current default unless another default would remain — mirror the existing guard pattern (clean 409 ahead of the invariant break):

```go
// after loading the row, extend the SELECT: SELECT is_system, is_default ...
if isDefault == 1 {
	writeError(w, http.StatusConflict, "set another default agent first")
	return
}
```

Alternatively (also acceptable): on delete of the default, atomically re-default the Claude seed in one transaction. The 409 is simpler and matches the handler's block-until-fixed posture. Add a regression test: create → set-default → delete → assert 409.

## Warnings

### WR-01: `BackfillGlobalTask` silently succeeds while inserting zero rows when no default agent exists

**File:** `internal/api/global_backfill.go:38-39`
**Issue:** `INSERT INTO global_task (id, agent_id) SELECT 1, id FROM agents WHERE is_default = 1` is a no-op (0 rows, `nil` error) when zero agents carry `is_default=1` — exactly the state produced by CR-01's renamed-seed variant, or by any hand-tampered install. The function's contract is "guarantee the singleton exists on every boot," yet it returns success with the singleton still missing; the failure then surfaces downstream as a confusing 500 in Phase 14's config GET or a spawn-path failure in Phase 15 — precisely the "later phases read it unconditionally" scenario the hook exists to prevent. The serve.go ordering comment correctly notes it must run after `BackfillAgents`, but the function itself does not defend its own postcondition. (Migration 00017's seed has the same zero-row shape, but the boot backfill covers it — provided the backfill actually verifies its insert.)

**Fix:** Fail loud when nothing was inserted, matching the boot posture of every other step in `serve.go`:

```go
res, err := db.Exec(`INSERT INTO global_task (id, agent_id) SELECT 1, id FROM agents WHERE is_default = 1`)
if err != nil {
	return err
}
if n, _ := res.RowsAffected(); n != 1 {
	return fmt.Errorf("global_task backfill inserted %d rows, want 1 (no is_default agent?)", n)
}
return nil
```

Extend `TestBackfillGlobalTask` with a zero-default case asserting the error.

### WR-02: `update` engine allowlist rejects `'opencode'` — a value the schema itself ships — 400ing the whole PATCH

**File:** `internal/api/agents_crud.go:168-174`
**Issue:** The validation `if e != "claude" && e != "custom"` predates M002's `engine='opencode'` system seed (migration 00015 / `BackfillOpenCodeAgent`). Two concrete misbehaviors: (a) a client that round-trips the engine field — PATCHing the OpenCode system agent with `{"name": "OC", "engine": "opencode"}`, where `engine` is the row's *current unchanged value* — is rejected 400, and because the check runs before any field is applied, the legitimate `name` change is blocked with it. (The Claude seed's equivalent same-value PATCH `{"engine": "claude"}` succeeds — inconsistent.) (b) The error message "engine must be 'claude' or 'custom'" misstates the actual engine domain. The shipped React dialog omits `engine` on PATCH (`web/src/components/settings/AgentNameDialog.tsx:78-85`), so no current UI path trips this, but any API consumer following the documented field set does. Keeping custom agents off the opencode engine is fine; rejecting the existing value on the system row is not.

**Fix:** Allow the row's current value through the allowlist (keeps opencode system-only while accepting same-value round-trips):

```go
if req.Engine != nil {
	e := strings.TrimSpace(*req.Engine)
	if e != "claude" && e != "custom" && e != curEngine {
		writeError(w, http.StatusBadRequest, "engine must be 'claude' or 'custom'")
		return
	}
}
```

Add a case to `TestAgentUpdateSystemEngineLocked` (or a sibling) PATCHing the OpenCode seed with same-value engine + a name change, asserting 200.

## Info

### IN-01: Three in-scope files fail `gofmt -l` (pre-existing at base, still unremediated)

**File:** `internal/api/agents_crud.go:22,27`, `internal/api/agents_crud_test.go:328-330`, `cmd/kamacu/serve.go:46`
**Issue:** `gofmt -l` flags all three (struct-field alignment drift in the `Agent` struct and `serveCmd`, table-literal alignment in `TestRenderAgentCommand`, plus one doc-comment list needing a trailing `//`). Verified via `git show cc9bb03:<file> | gofmt -d` that the misformatting predates this phase — but the phase added lines to all three files without fixing it, so any format gate added later will flag this diff too.
**Fix:** `gofmt -w internal/api/agents_crud.go internal/api/agents_crud_test.go cmd/kamacu/serve.go` (whitespace-only changes; no behavior impact).

### IN-02: 00018's `UNIQUE(task_id, n)` no longer constrains global rows (SQLite NULLs are distinct)

**File:** `internal/store/migrations/00018_tmux_scope.sql:33`
**Issue:** For `scope='global'` rows (`task_id NULL`), SQLite's UNIQUE semantics treat each NULL as distinct, so two global rows with `n=1` are both representable — per-task n monotonicity, which the task path gets from this constraint, is not DB-enforced for globals; only `name UNIQUE` guards them. The migration test's acceptance insert (c) already relies on this NULL-distinct behavior, so it's clearly load-bearing as designed — but Phase 15's global-session allocator must serialize `n` app-side (the `kamacu-global-<n>` naming scheme must not rely on the constraint to prevent collisions).
**Fix:** No schema change needed. Record the obligation in Phase 15's plan: allocate global `n` under the same reserve-before-spawn discipline, guarded by the `name` UNIQUE constraint.

---

_Reviewed: 2026-08-25T16:30:00Z_
_Reviewer: the agent (gsd-code-reviewer)_
_Depth: standard_
