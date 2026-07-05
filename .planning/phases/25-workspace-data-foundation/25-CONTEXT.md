# Phase 25: Workspace Data Foundation - Context

**Gathered:** 2026-07-05
**Status:** Ready for planning

<domain>
## Phase Boundary

The pure **data layer** for workspaces — nothing user-visible ships in this phase.

Delivers:
1. Migration `00012`: a `workspaces` table + a `projects.workspace_id` NOT NULL foreign key.
2. A one-time idempotent bootstrap that creates a protected default **Personal** workspace and assigns every pre-existing project to it.
3. `workspace_id` on the projects API **read** wire, ready for the Phase 26 switcher to filter on.
4. New-project creation defaults to the default workspace so the NOT NULL FK always holds.

**Out of this phase (all Phase 26):** the workspace switcher UI, workspace create/rename/delete endpoints, active-workspace filtering + navigation, project transfer via `⋯`, create-in-active-workspace, and the cross-workspace sessions-bar guardrail. Also permanently out of scope (per REQUIREMENTS.md): cascade-delete of projects, workspace icons/colors, drag-and-drop transfer, reordering, nested workspaces, per-workspace settings.

Requirements covered: **WSDATA-01**, **WSDATA-02**.
</domain>

<decisions>
## Implementation Decisions

### Schema — `workspaces` table
- **D-01:** New `workspaces` table, **minimal columns only**: `id INTEGER PRIMARY KEY`, `name TEXT NOT NULL`, `is_default INTEGER NOT NULL DEFAULT 0`, `created_at`/`updated_at` TEXT (mirror the `projects` `strftime('%Y-%m-%dT%H:%M:%fZ','now')` defaults from `00001_init.sql`). No `position` column and no icon columns — reordering (WSFUT-02) and identity (WSFUT-01) are deferred; a future `00013` is cheap. Matches the thin-slice approach.
- **D-02:** The protected default workspace is identified by the **`is_default` flag** (exactly one row = 1), **not** by matching the name "Personal". This is **rename-proof**: WSMGMT-02 (Phase 26) lets the user rename Personal, which would break any name-based protection. Downstream resolves the default via `SELECT id FROM workspaces WHERE is_default = 1`. Phase 26's WSMGMT-04 "can never be deleted" guard keys off this flag.
- **D-03:** `workspaces.name` is **UNIQUE, case-insensitive** — a unique index on `name COLLATE NOCASE` (so "Personal" and "personal" collide). A workspace is a short pick-list label; duplicates would make the Phase 26 switcher/transfer ambiguous. Gives Phase 26 a clean DB-level reject for create/rename (WSMGMT-01/02).

### Schema — `projects` foreign key
- **D-04:** `projects.workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT`. **RESTRICT** is a DB-level backstop that hard-refuses deleting a workspace that still owns projects — mirroring the Phase 26 app-level block-until-empty (WSMGMT-03) and honoring the project-wide "never bulldoze a project" value (Out of Scope: no cascade-delete). Never `ON DELETE CASCADE`. FKs are actually enforced (`store.go` opens with `_pragma=foreign_keys(1)`), so RESTRICT bites.
- **D-05:** NOT NULL is **DB-enforced from migration `00012` onward** (success criterion 1), not merely app-enforced — a project can never be workspace-less at the SQL layer.

### Migration + bootstrap (the phase's core mechanic)
- **D-06:** Migration `00012` performs the whole bootstrap **in SQL** so NOT NULL holds at migration time: create the `workspaces` table + the case-insensitive unique index, `INSERT` the Personal row (`is_default=1`), add `projects.workspace_id`, and assign every existing project to Personal. The **exact SQLite mechanism** for adding a NOT NULL FK column to a populated table is a **research item** (see Specifics → Research flag): candidates are `ADD COLUMN ... NOT NULL DEFAULT <Personal id>` (relies on Personal deterministically being the first row / a known id), an add-nullable → `UPDATE` → 12-step table-rebuild to tighten to NOT NULL, or a full table rebuild. Pick the approach that keeps DB-level NOT NULL and stays within modernc/SQLite 3.53 capabilities.
- **D-07:** The **Go startup hook** mirrors `api.BackfillProjectIcons` exactly: idempotent, runs **once right after `store.Migrate(db)`** in `cmd/kamacu/main.go` (alongside the existing `BackfillProjectIcons` call), a cheap no-op on healthy boots. Its job is the **idempotent safety net** WSDATA-02 mandates — guarantee a default workspace exists and no project is workspace-less — never creating a second Personal on restart (success criterion 3). **Collect-then-update is REQUIRED**: the store runs `db.SetMaxOpenConns(1)` (single-writer), so any SELECT cursor must be scanned into a slice and closed before issuing UPDATEs on the same connection (see the deadlock note in `BackfillProjectIcons`). All SQL parameterized with `?` — never string-concatenate names/ids.
- **D-08:** The migration `Down` reverses cleanly (drop the FK/column, drop the unique index + table) — model on `00009_project_icons.sql`'s Down, which uses `DROP COLUMN` directly (modernc tracks SQLite 3.53, which supports it). If a table rebuild is chosen for Up, the Down mirrors it.

### Write path & API surface (Phase 25 ↔ 26 boundary)
- **D-09:** Both create INSERTs in `internal/api/projects.go` — the folder path in `create` (~line 183) and the repo-first path in `createByRepo` (~line 276) — resolve the default workspace (`WHERE is_default = 1`) and set `workspace_id`, so creation keeps working under the NOT NULL FK. This is the **only** write-path change in Phase 25.
- **D-10:** The `Project` struct gains `WorkspaceID int64 \`json:"workspace_id"\``, added to the `projectColumns` const and the `scanProject` Scan order — **watch column order** (thread it consistently across the const and the `Scan(...)` call, exactly how `icon_letters`/`icon_color` were added in v1.7). Ships on the projects **read** wire (`GET /api/projects`, `POST`, `PATCH` RETURNING) — success criterion 4.
- **D-11:** **No new workspace endpoints in Phase 25.** All workspace CRUD (list/create/rename/delete), active-workspace filtering, project transfer, and create-in-active-workspace are **Phase 26** (WSMGMT/WSNAV/WSPROJ/WSBAR). WSPROJ-02 refines new-project placement to the *active* workspace then; Phase 25's default-to-Personal is the interim behavior.

### Claude's Discretion
- Migration filename (`00012_workspaces.sql`), the precise `workspace_id` column position within `projectColumns`/`scanProject`, the exact idempotency-guard SQL in the Go hook, and whether the hook lives in a new `internal/api/workspaces.go` or beside `icons.go` — follow existing patterns; no user preference expressed.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & roadmap (scope anchors)
- `.planning/ROADMAP.md` § "Phase 25: Workspace Data Foundation" — goal + the 4 success criteria (NOT NULL FK, Personal backfill, idempotency, wire).
- `.planning/REQUIREMENTS.md` — **WSDATA-01 / WSDATA-02** (locked), the **Out of Scope** table (no cascade-delete, name-only, flat, localStorage-only), and **WSFUT-01/02/03** (deferred — the reason `position`/icons are excluded now).

### Backfill / startup-hook pattern — MIRROR THIS EXACTLY (D-07)
- `internal/api/icons.go` — `BackfillProjectIcons`: the idempotent one-shot template — `WHERE ... = ''` no-op guard, **collect-then-update under single-writer**, parameterized `?` SQL. The workspace backfill hook is this pattern applied to workspaces.
- `cmd/kamacu/main.go` (~line 98–127) — `store.Migrate(db)` then the `api.BackfillProjectIcons(db)` call; the new hook wires in right here, same ordering rationale ("columns/table must exist first").

### Migration pattern & runner
- `internal/store/migrations/00009_project_icons.sql` — `ADD COLUMN ... NOT NULL DEFAULT` + `Down` `DROP COLUMN` template (the D-06/D-08 model).
- `internal/store/migrations/00001_init.sql` — the `projects` table definition (FK target; `strftime` timestamp-default idiom to reuse for `workspaces`).
- `internal/store/migrations/` — embedded goose migrations, run at startup; latest is `00011_diff_viewed.sql`, so the new file is `00012`.
- `internal/store/migrate.go` — the goose embed + `Migrate` runner (confirm the embed glob picks up `00012`).

### Projects schema / API / store discipline
- `internal/api/projects.go` — `Project` struct, `projectColumns`, `scanProject`, and the two INSERT sites (`create`, `createByRepo`) touched by D-09/D-10.
- `internal/store/store.go` — the pragma discipline: `foreign_keys(1)` (RESTRICT is enforced) and `SetMaxOpenConns(1)` (single-writer → collect-then-update is mandatory).

No external ADRs/specs — requirements are fully captured in the decisions above plus these in-repo references.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`api.BackfillProjectIcons` (`internal/api/icons.go`)** — near line-for-line template for the workspace backfill hook: idempotent guard, single-writer collect-then-update, param SQL.
- **`00009_project_icons.sql`** — template for the column-add migration and its `Down`.
- **`Project` / `projectColumns` / `scanProject` (`internal/api/projects.go`)** — the single, well-documented place to thread a new column onto the wire (v1.7 added two columns here the same way).
- **`INSERT ... RETURNING \`+projectColumns` idiom** — both create paths already return the full row; adding `workspace_id` to the const carries it through automatically.

### Established Patterns
- **Startup one-shots wired in `main.go` after `store.Migrate`** — `migrate.Complete`, `BackfillProjectIcons`, `sweepOrphanTmux` are all idempotent, ordering-sensitive hooks; the workspace backfill joins this line-up.
- **Single-writer discipline** — `db.SetMaxOpenConns(1)`: never hold a SELECT cursor open while issuing UPDATEs on the same connection (documented in `BackfillProjectIcons`).
- **FKs enforced** — `_pragma=foreign_keys(1)`: `ON DELETE RESTRICT` (D-04) actually prevents the delete; not decorative.
- **Numbered embedded goose migrations** in `internal/store/migrations/`, run once at startup; version-tracked so re-runs don't re-execute (supports idempotency of the Personal insert if done in-migration — D-06).

### Integration Points
- New file `internal/store/migrations/00012_workspaces.sql` (table + unique index + FK column + in-SQL Personal bootstrap/assign).
- New backfill function in package `api` (new `internal/api/workspaces.go` or beside `icons.go`), wired in `cmd/kamacu/main.go` right after `api.BackfillProjectIcons(db)`.
- `internal/api/projects.go`: `Project` struct + `projectColumns` + `scanProject` (wire), and both create INSERTs (default-to-Personal).
</code_context>

<specifics>
## Specific Ideas

**Research flag (hand to `gsd-phase-researcher`):** The one genuinely open technical question is the exact SQLite sequence to add a `NOT NULL` foreign-key column to the already-populated `projects` table while keeping NOT NULL DB-enforced from migration time (D-05/D-06). modernc/SQLite is 3.53. Evaluate: (a) `INSERT` Personal first (deterministic id) then `ADD COLUMN workspace_id INTEGER NOT NULL DEFAULT <that id>` + a follow-up to drop/normalize the literal default; (b) add nullable → `UPDATE` all rows → 12-step table rebuild to tighten to NOT NULL + FK; (c) full `projects` table rebuild with the FK inline. Constraint: the result must be a real DB-level NOT NULL FK with `ON DELETE RESTRICT`, and the Go hook (D-07) stays a valid idempotent backstop on top of whichever path is chosen. This is the single riskiest detail in the phase.

The rest is fully specified by the decisions and the mirror-`BackfillProjectIcons` pattern.
</specifics>

<deferred>
## Deferred Ideas

- **WSFUT-01** — workspace icon/color identity (monogram + palette like projects). Reason `workspaces` stays name-only and skips icon columns now.
- **WSFUT-02** — reorder workspaces in the switcher. Reason we did **not** add a `position` column (D-01); a `00013` adds it when the feature lands.
- **WSFUT-03** — bulk / multi-select project transfer between workspaces.
- **Phase 26 surface** (not deferred-forever, just next phase): the switcher UI, workspace CRUD endpoints, active-workspace filtering + navigation + localStorage persistence, `⋯`-menu transfer, create-in-active-workspace, and the cross-workspace sessions-bar guardrail (WSMGMT/WSNAV/WSPROJ/WSBAR).

None of these were pulled into Phase 25 — discussion stayed within the data-foundation boundary.
</deferred>

---

*Phase: 25-workspace-data-foundation*
*Context gathered: 2026-07-05*
