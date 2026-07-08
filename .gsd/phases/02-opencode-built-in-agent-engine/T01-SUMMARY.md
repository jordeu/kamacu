---
id: T01
parent: S03
milestone: M002
key_files:
  - internal/store/migrations/00016_opencode_session_id.sql
  - internal/store/opencode_session_id_test.go
key_decisions:
  - (none)
duration: 
verification_result: passed
completed_at: 2026-07-07T19:45:20.943Z
blocker_discovered: false
---

# T01: Added nullable tasks.opencode_session_id column (migration 00016) with upgrade + fresh-install migration tests

**Added nullable tasks.opencode_session_id column (migration 00016) with upgrade + fresh-install migration tests**

## What Happened

Added the strategy-invariant data-model foundation for opencode session resume: a nullable `tasks.opencode_session_id TEXT` column, the opencode analog of `tasks.claude_session_id` (00003). Unlike claude — where kamacu mints `--session-id <uuid>` and always knows it — opencode mints an opaque `ses_…` id internally, so kamacu must persist the id it later discovers (T03's capture path) for a restart to resume the conversation (T02's resume argv reads it).

## What Happened

1. Created `internal/store/migrations/00016_opencode_session_id.sql` mirroring `00003_agent_sessions.sql` exactly in shape: `-- +goose Up / ALTER TABLE tasks ADD COLUMN opencode_session_id TEXT;` and `-- +goose Down / ALTER TABLE tasks DROP COLUMN opencode_session_id;`. This is plain transactional DDL (a nullable column ADD, no data, no FK toggle), so it does NOT need the `-- +goose NO TRANSACTION` + PRAGMA dance that 00013 needed. The existing `//go:embed migrations/*.sql` in `internal/store/migrate.go` picks the new file up with no other change — confirmed by the green whole-module `go build ./...`.

2. Created `internal/store/opencode_session_id_test.go` mirroring the 00015 migration-test posture (`opencode_agent_migration_test.go`): a `tasksColumnCount` helper runs `PRAGMA table_info(tasks)` and returns how many columns match a name + the first match's full info row. Two tests:
   - `TestOpencodeSessionIdMigration` stages the DB at pre-00016 (`goose.UpTo(..., 15)`), asserts the column does NOT yet exist AND that `claude_session_id` (00003) is present (so 00016 runs against the real populated tasks schema), applies `Migrate`, then asserts: (a) `opencode_session_id` now exists exactly once as nullable TEXT (`notnull=0`, no default — no NOT NULL), (b) `PRAGMA foreign_keys == 1` (re-armed ON; 00016 is transactional and must not have left it off), (c) idempotency — a second `Migrate` is a clean no-op (column still exactly one), and the sibling `claude_session_id` column is unchanged.
   - `TestOpencodeSessionIdMigrationOnFreshDB` runs `Migrate` from scratch and asserts the column lands alongside `claude_session_id` in one pass (nullable TEXT).

3. Fixed two compile bugs in the test before it ran: added the missing `database/sql` import (needed for `sql.NullString` in the helper struct) and corrected the helper signature from `db DB` (no such type) to `db *sql.DB` (the actual `Open` return type).

4. Verified per MEM026: a column ADD — unlike a new agent seed row — changes no agent COUNT, so the per-engine/per-flag count assertions in the opencode/workspace migration tests stay green. The full store suite confirms no regression.

This task changes NO runtime behavior — it only lays the nullable column that T02 (resume argv) and T03 (capture path) depend on.

## Failure Modes (Q5)

The task's external dependencies are SQLite (modernc.org/sqlite, pure-Go transpiled, in-process — no network/CGO/external service) and goose (embedded migrations via `go:embed`). There are no APIs, network calls, or subprocesses.

- **Migration apply failure:** 00016 is plain transactional DDL (`ALTER TABLE ADD COLUMN` of a nullable TEXT). goose wraps it in a transaction; if the ALTER failed, goose rolls back and `Migrate` returns the error, which the app surfaces at startup. For a nullable ADD COLUMN the only realistic failure is a duplicate column, which goose's version table prevents (proven by the idempotent re-run in the test). The test exercises the real goose runner on a real SQLite handle — not a stub — so the transactional rollback path is the same path production takes.
- **Staging failure:** the test explicitly asserts that staging at 00015 leaves the column ABSENT before 00016 applies (`preCount != 0` → fatal "staging failed"), catching the failure mode where a prior migration didn't land.
- **FK left off:** the test asserts `PRAGMA foreign_keys == 1` post-migration; a 00016 that accidentally left FK disabled (as 00013's PRAGMA toggle could) would be caught here.

No silent error swallowing: goose errors propagate through `Migrate`'s return value and `t.Fatalf` in tests.

## Load Profile (Q6)

Omitted. 00016 is a one-time schema migration run at startup (`Migrate`) or once per test (`t.TempDir()`). The nullable column has no index and is never read/written in a per-request hot path until T03's capture path and T02's resume path are built (downstream tasks). There is no runtime load dimension to this task.

## Negative Tests (Q7)

`internal/store/opencode_session_id_test.go` covers these negative/boundary scenarios:

- **Pre-migration absence (boundary):** `TestOpencodeSessionIdMigration` stages at 00015 and asserts `opencode_session_id` count == 0 before applying 00016 — fails fast with "staging failed" if a prior migration erroneously created it.
- **Nullable, not NOT NULL (malformed-schema guard):** asserts `notnull == 0` and `dflt` is NULL/empty post-migration — catches an accidental `NOT NULL` that would break the "empty until T03 writes it" invariant on existing rows.
- **Type guard:** asserts column type == "TEXT" exactly — catches a typo'd DDL.
- **Exactly-once / no duplication (idempotency boundary):** asserts count == 1 after the first apply AND after a second `Migrate` no-op re-run — catches a non-idempotent migration that would error or duplicate.
- **Sibling-column preservation:** asserts `claude_session_id` count stays 1 before and after — catches a migration that accidentally dropped the existing resume key.
- **FK re-armed (state-leak guard):** asserts `PRAGMA foreign_keys == 1` post-migration — catches a migration that left FK disabled.
- **Fresh-install coexistence:** `TestOpencodeSessionIdMigrationOnFreshDB` asserts both `opencode_session_id` and `claude_session_id` land together from scratch — catches a migration-ordering bug that only manifests on fresh installs.

## Verification

Ran `go test ./internal/store/ -count=1` (the plan's specified verification command): exit 0, `ok kamacu/internal/store 0.306s` — both new tests (TestOpencodeSessionIdMigration, TestOpencodeSessionIdMigrationOnFreshDB) pass alongside all existing store tests, confirming the nullable column ADD breaks no existing total-migration/per-engine-count assertions (MEM026). Also ran `go build ./...` (exit 0) to confirm the new embedded migration compiles into the binary cleanly via the existing `//go:embed migrations/*.sql` directive with no other change.

## Verification Evidence

| # | Command | Exit Code | Verdict | Duration |
|---|---------|-----------|---------|----------|
| 1 | `go test ./internal/store/ -count=1` | 0 | ✅ pass | 735ms |
| 2 | `go build ./...` | 0 | ✅ pass | 816ms |

## Deviations

None.

## Known Issues

None.

## Files Created/Modified

- `internal/store/migrations/00016_opencode_session_id.sql`
- `internal/store/opencode_session_id_test.go`
