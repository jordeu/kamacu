# Phase 25: Workspace Data Foundation - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-07-05
**Phase:** 25-workspace-data-foundation
**Areas discussed:** Default-workspace identity, FK delete guard, Workspace name uniqueness, Schema build-ahead vs. minimal, NOT-NULL bootstrap mechanic, Phase 25 write-path scope

---

## Default-workspace identity & protection

| Option | Description | Selected |
|--------|-------------|----------|
| `is_default` flag column | Boolean column, exactly one row = 1; rename-proof (WSMGMT-02 allows renaming Personal); explicit queryable source of truth | ✓ |
| Match by name "Personal" | Simpler (no column) but breaks the instant the user renames Personal | |
| Lowest id / id = 1 | Implicit, brittle; relies on insertion order and survives no reseed | |

**User's choice:** `is_default` flag column
**Notes:** Decisive factor — WSMGMT-02 (Phase 26) explicitly allows renaming Personal, so name-based protection would silently break on rename. The flag survives renames.

---

## FK delete guard (`projects.workspace_id`)

| Option | Description | Selected |
|--------|-------------|----------|
| `ON DELETE RESTRICT` | DB hard-refuses deleting a workspace that still owns projects; backstop for block-until-empty (WSMGMT-03) | ✓ |
| `NO ACTION` (SQLite default) | Also prevents the delete, but RESTRICT states intent explicitly and fails immediately | |
| App-level block only | No DB action; rely solely on Phase 26 API — loses defense-in-depth | |

**User's choice:** `ON DELETE RESTRICT`
**Notes:** Honors the project-wide "never bulldoze a project" value; even a future bug can't orphan/delete projects via a workspace delete. FKs are enforced (`foreign_keys(1)`).

---

## Workspace name uniqueness

| Option | Description | Selected |
|--------|-------------|----------|
| UNIQUE, case-insensitive | Unique index on `name COLLATE NOCASE`; "Personal"/"personal" collide; clean DB reject for Phase 26 | ✓ |
| UNIQUE, case-sensitive | Exact-match only; near-duplicates coexist | |
| Allow duplicates | No constraint; pushes disambiguation onto the Phase 26 UI | |

**User's choice:** UNIQUE, case-insensitive
**Notes:** A workspace is a short pick-list label; duplicates would make the switcher/transfer ambiguous.

---

## Schema build-ahead vs. minimal

| Option | Description | Selected |
|--------|-------------|----------|
| Minimal | `id, name, is_default, created_at, updated_at` only; reorder (WSFUT-02) and icons (WSFUT-01) deferred | ✓ |
| Add a `position` column now | Pre-add `position REAL` so future drag-reorder needs no migration | |

**User's choice:** Minimal
**Notes:** Thin slice; the project runs migrations routinely (00001–00011), so a future 00013 is cheap. No speculative columns for deferred features.

---

## NOT-NULL bootstrap mechanic

| Option | Description | Selected |
|--------|-------------|----------|
| Migration bootstraps + Go hook backstops | Migration 00012 creates table, inserts Personal, assigns all projects in SQL → DB-enforced NOT NULL from migration time (criterion 1); Go hook = idempotent safety net | ✓ |
| Go hook does creation + assignment | Migration adds nullable column; hook creates+assigns — but DB NOT NULL can't be added afterward in SQLite without a rebuild, so NOT NULL ends up app-only (conflicts with criterion 1) | |

**User's choice:** Migration bootstraps + Go hook backstops
**Notes:** Success criterion 1 requires DB-enforced NOT NULL. Exact SQLite mechanism (DEFAULT-constant vs. table-rebuild) flagged as a research item in CONTEXT.md.

---

## Phase 25 write-path scope (25 ↔ 26 boundary)

| Option | Description | Selected |
|--------|-------------|----------|
| Default new projects to Personal; no new endpoints | Both create INSERTs resolve `is_default=1` and set `workspace_id`; `workspace_id` on the projects read wire; all workspace endpoints → Phase 26 | ✓ |
| Also ship read-only `GET /api/workspaces` | Same, plus a workspaces list endpoint now — testable wire, but adds surface Phase 26 owns | |

**User's choice:** Default new projects to Personal; no new endpoints
**Notes:** Keeps creation working under NOT NULL with the minimum surface. WSPROJ-02 refines new-project placement to the *active* workspace in Phase 26.

---

## Claude's Discretion

- Migration filename (`00012_workspaces.sql`), `workspace_id` column position within `projectColumns`/`scanProject`, the idempotency-guard SQL in the Go hook, and whether the hook lives in a new `workspaces.go` or beside `icons.go`.

## Deferred Ideas

- WSFUT-01 (workspace icon/color identity), WSFUT-02 (reorder — why `position` was skipped), WSFUT-03 (bulk transfer).
- Phase 26 surface: switcher UI, workspace CRUD endpoints, active-workspace filtering/navigation/localStorage, `⋯`-menu transfer, create-in-active-workspace, cross-workspace sessions-bar guardrail.
