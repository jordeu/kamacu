---
phase: 18-project-icon-data-foundation
plan: 02
subsystem: backend-api
tags: [projects, icons, validation, startup, sqlite]
requires:
  - "18-01: icons.go helpers (deriveLetters, pickColor, validateIconLetters, validateIconColor, BackfillProjectIcons) + migration 00009 (icon_letters, icon_color)"
provides:
  - "Project struct + projectColumns + scanProject extended with icon_letters/icon_color (column-order consistent, non-null on the wire)"
  - "Both create paths (folder + repo-first) auto-assign derived letters + a random palette color"
  - "PATCH /api/projects/{id} accepts + server-validates icon_letters/icon_color (partial-pointer pattern, the wire path Phase 19 edits through)"
  - "Startup icon backfill wired after store.Migrate (idempotent)"
affects:
  - "internal/api/projects.go"
  - "cmd/kangent/main.go"
tech-stack:
  added: []
  patterns:
    - "Column-order discipline: struct field order == projectColumns == scanProject Scan order == INSERT column order"
    - "Partial-pointer PATCH (omitted key = untouched) with validate-then-append + early-return-on-400"
    - "Startup fatal idiom (slog.Error + os.Exit(1)) for the one-shot idempotent backfill"
    - "Parameterized ? placeholders only (no string-concatenated user values)"
key-files:
  created: []
  modified:
    - "internal/api/projects.go"
    - "cmd/kangent/main.go"
decisions:
  - "D-12: IconLetters/IconColor are plain non-null strings placed between Managed and CreatedAt; TEXT columns scan straight into string (no sql.NullString)"
  - "D-06: both create paths source the two fields from the SAME shared helpers (deriveLetters(name), pickColor()) so create behavior === backfill behavior"
  - "D-09/D-10/D-11: PATCH server-validates BOTH fields at route entry; off-palette/empty rejected 400 with the row untouched"
  - "D-08: backfill runs after store.Migrate (load-bearing order — columns must exist first); migrate.go embed stays SQL-only (backfill lives in package api)"
metrics:
  duration: "~7 min"
  completed: "2026-06-19"
  tasks: 3
  files_changed: 2
requirements: [ICON-01, ICON-02, ICON-03, ICON-04]
---

# Phase 18 Plan 02: Wire Icon Fields Into Project Paths Summary

Wired the Plan 01 icon helpers end-to-end: the `Project` model, both create paths, the PATCH `update` handler, and a startup backfill — new projects now get derived monogram letters + a random palette color with no extra user action, the two fields round-trip on list/GET and persist across restart, PATCH server-validates both fields, and pre-existing rows are backfilled at boot.

## What Was Built

### Task 1 — Model + both create paths (`internal/api/projects.go`, commit e3569db)
- Added `IconLetters string` and `IconColor string` (json `icon_letters`/`icon_color`) to the `Project` struct, between `Managed` and `CreatedAt` (D-12, never null on the wire).
- Extended `projectColumns` to `... managed, icon_letters, icon_color, created_at, updated_at`.
- Added `&p.IconLetters, &p.IconColor` to the `scanProject` Scan in projectColumns order (TEXT → string, no `sql.NullString`).
- `create` (folder path) INSERT now includes `icon_letters, icon_color` sourced from `deriveLetters(name), pickColor()`.
- `createByRepo` (repo-first path) INSERT now includes the same two columns/args.
- Column order is consistent across struct / columns / scan / both INSERTs.

### Task 2 — PATCH validation (`internal/api/projects.go`, commit 41da709)
- Added `IconLetters *string` / `IconColor *string` to the partial-PATCH req struct and to the "nothing to update" OR-guard.
- Added an `if req.IconLetters != nil` block calling `validateIconLetters(*req.IconLetters)` (D-10): on error `writeError(w, 400, ...)` + early return (row untouched); on success append `icon_letters = ?` + the normalized value.
- Added an `if req.IconColor != nil` block calling `validateIconColor(*req.IconColor)` (D-11): off-palette rejected 400 + early return; on success append `icon_color = ?` + the canonical hex.
- The existing `updated_at` append + `UPDATE ... RETURNING projectColumns` tail is unchanged and echoes the updated row via `scanProject`. Only `?` placeholders used.

### Task 3 — Startup backfill (`cmd/kangent/main.go`, commit 4558270)
- Inserted `api.BackfillProjectIcons(db)` immediately after the `store.Migrate(db)` success check, using the `slog.Error("backfilling project icons", ...) + os.Exit(1)` fatal idiom.
- No new import (the `api` package was already imported). `internal/store/migrate.go` is unchanged — the goose embed stays SQL-only; the backfill lives in package `api`.

## Verification

- `go build ./...` — exit 0
- `go vet ./internal/api/ ./cmd/kangent/` — exit 0
- `go test ./internal/api/` — ok (existing project tests + Plan 01 icon tests green)
- Column-order consistency confirmed by compile + grep (`icon_letters, icon_color, created_at` present; `&p.IconLetters, &p.IconColor` in scan).
- Create-time assignment: `deriveLetters(name), pickColor()` appears 2x (both create paths).
- PATCH validation: `validateIconLetters(*req.IconLetters)` and `validateIconColor(*req.IconColor)` present; guard includes `req.IconLetters == nil`.
- Backfill ordering: `awk` line-order check confirms `api.BackfillProjectIcons(db)` follows `store.Migrate(db)` in main.go.

## Requirements Satisfied

- ICON-01/02/03: new folder + managed-repo projects auto-assign derived letters + a random palette color (both create paths wired to the shared helpers).
- ICON-04: icon_letters + icon_color round-trip on list/GET and persist (SQLite); PATCH is the server-validated wire path Phase 19 edits through; startup backfill fills pre-existing blank rows idempotently.

## Threat Mitigations Applied

- T-18-05 (PATCH icon_color tampering): `validateIconColor` enforces curated-palette membership at route entry; off-palette → 400 before any SQL.
- T-18-06 (PATCH icon_letters tampering): `validateIconLetters` trims/uppercases/caps≤2/requires≥1; empty → 400, row untouched.
- T-18-07 (SQLi): all icon values bound via `?` placeholders in INSERT/UPDATE; never string-concatenated.
- T-18-SC (package installs): no new packages — all stdlib + the Plan 01 helpers.

## Deviations from Plan

None — plan executed exactly as written. No bugs, missing functionality, or blocking issues encountered; no architectural decisions required.

## Known Stubs

None — all changes wire real data through existing, tested helpers (no placeholder/empty-value flows introduced).

## Self-Check: PASSED

- Files: internal/api/projects.go, cmd/kangent/main.go, 18-02-SUMMARY.md — all present.
- Commits: e3569db, 41da709, 4558270, 826133f — all in the git log.
- Working tree: clean.
