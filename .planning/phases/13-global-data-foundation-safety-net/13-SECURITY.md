---
phase: 13
slug: global-data-foundation-safety-net
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate)
threats_open: 0
asvs_level: 1
created: 2026-08-25
---

# Phase 13 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| DB schema ← future writers | Every future Go writer (Phase 14 config API, Phase 15 spawn path) inserts through these constraints; the DB is the enforcement point, not caller discipline | global_task row, tmux_sessions rows (integrity) |
| Existing install data → migration | The upgrade path transforms a live v1.12 DB in place; pre-existing rows are the asset under protection | 147 tasks, 10 custom-labeled tmux rows (user data) |
| Startup sweep ← live tmux server | The sweep holds kill power over live user shells on the dedicated kamacu socket; a wrong known-set is a denial-of-service on user sessions | kill commands against kamacu-* sessions (availability) |
| Real install DB ← rehearsal binary | The rehearsal boots the real binary against a COPY; the copy path and port choice must never touch the live install or its port | full DB copy (all user data) |
| agents delete handler ← API clients | The COUNT guard is the friendly gate ahead of the FK RESTRICT backstop for any caller | agent id path param |

---

## Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation | Status |
|-----------|----------|-----------|----------|-------------|------------|--------|
| T-13-01 | Tampering/Destruction | migrations 00017+00018 against live-install data | high | mitigate | sqlite.org 12-step rebuild ordering + staged-upgrade test with byte-for-byte snapshot diff (TestTmuxScopeRebuildMigration PASS) + goose version-table idempotence (second-Migrate no-op asserted) + real-install rehearsal (empty diff, goose 16→18) | closed |
| T-13-02 | Tampering | tmux_sessions shape (future buggy writers) | medium | mitigate | Table-level XOR CHECK `(task_id IS NULL) = (scope = 'global')` + scope CHECK constraint — both rejection directions asserted in TestTmuxScopeRebuildMigration | closed |
| T-13-03 | Tampering | global_task.agent_id reference | medium | mitigate | CHECK(id=1) makes a second singleton row unrepresentable; FK ON DELETE RESTRICT refuses deleting the referenced agent (asserted in TestGlobalTaskMigration + handler 409) | closed |
| T-13-04 | Tampering | FK enforcement after NO TRANSACTION rebuild | high | mitigate | Trailing PRAGMA foreign_keys = ON re-arms the pooled single connection; pragma probe + bogus-FK insert assert the re-arm on the migrated handle | closed |
| T-13-05 | Denial of Service | sweepOrphanTmux vs live global tabs | high | mitigate | Scope-aware known-set query `WHERE scope = 'global' OR task_id IN (SELECT id FROM tasks)` in serve.go + host-gated kill/no-kill regression test (RED proved the killer bug first); degrade-don't-break posture preserved | closed |
| T-13-06 | Tampering/Destruction | rehearsal against the live install | high | mitigate | sqlite3 .backup online-backup API only (never cp on a live WAL); all rehearsal I/O against the copy; spare loopback port (7391/7395) avoids the real server's bind | closed |
| T-13-07 | Tampering | agents delete path vs global_task reference | low | mitigate | Handler COUNT 409 "reassign the Scratchpad agent first" ahead of the 00017 RESTRICT backstop; TestAgentDeleteInUseByGlobal proves block + unblock | closed |
| T-13-08 | Tampering | singleton dropped between boots | medium | mitigate | BackfillGlobalTask re-arms id=1 every boot, seeded from is_default (never hardcoded); error refuses boot (ExitFailure) so a broken backfill is loud | closed |
| T-13-SC | Tampering | package installs | low | accept | Zero installs this phase — verified: go.mod/go.sum/web/package.json untouched across the phase diff (cc9bb03..HEAD) | closed |

*Status: open · closed · open — below high threshold (non-blocking)*
*Severity: critical > high > medium > low — only open threats at or above workflow.security_block_on count toward threats_open*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-13-01 | T-13-SC | Zero package installs this phase; no new supply-surface introduced | developer | 2026-08-25 |

*Accepted risks do not resurface in future audit runs.*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-08-25 | 9 | 9 | 0 | orchestrator (L1 grep-depth, short-circuit: threats_open 0 + plan-time register + ASVS 1) |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-08-25
