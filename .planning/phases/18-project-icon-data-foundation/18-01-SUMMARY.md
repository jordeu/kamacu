---
phase: 18-project-icon-data-foundation
plan: 01
subsystem: backend-data
tags: [icons, migration, sqlite, goose, backfill, validation]
requires: []
provides:
  - "projectPalette (9 locked Tailwind-600 hexes, Go source of truth for D-01/D-02)"
  - "deriveLetters(name string) string — monogram rule (D-04/D-05/D-06)"
  - "pickColor() string — pure-random palette member (D-03)"
  - "validateIconLetters(s) (string, error) (D-10)"
  - "validateIconColor(s) (string, error) (D-11)"
  - "BackfillProjectIcons(db *sql.DB) error — idempotent startup pass (D-08)"
  - "migration 00009: icon_letters + icon_color columns (D-07)"
affects:
  - "Plan 02 consumes all icons.go helpers in projects.go (struct/columns/scan/create/createByRepo/update) and wires BackfillProjectIcons into main.go startup"
  - "Plan 03 mirrors projectPalette in TS (web/src/api), no palette endpoint"
tech-stack:
  added: []
  patterns:
    - "goose ADD COLUMN Up / reverse-order DROP COLUMN Down (mirrors 00007/00008)"
    - "pure string helper with strings.Builder flush idiom (tokenize.go analog)"
    - "canonical-error validator returning (canonical, error) (ParseRepoRef analog)"
    - "collect-then-update batch under single-writer SetMaxOpenConns(1)"
    - "table-driven helper tests (TestParseRepoRef analog)"
key-files:
  created:
    - internal/store/migrations/00009_project_icons.sql
    - internal/api/icons.go
    - internal/api/icons_test.go
  modified: []
decisions:
  - "math/rand/v2 (imported as rand) for pickColor — non-crypto fine per D-03 (single-user, cosmetic)"
  - "deriveLetters tokenizes into alphanumeric-only tokens, skipping leading/embedded non-alphanumerics; multi-token -> first char of first two tokens, single-token -> first two chars"
  - "validator error copy as package consts (errIconLettersRequired / errIconColorOffPalette) so create + PATCH callers reuse verbatim (no drift)"
  - "BackfillProjectIcons collects rows then UPDATEs (single-writer deadlock avoidance)"
metrics:
  duration: "~6 min"
  completed: 2026-06-19
  tasks: 3
  files: 3
---

# Phase 18 Plan 01: Project Icon Data Foundation Summary

Backend data layer for project icons: goose migration `00009` adds two TEXT columns and a new same-package `internal/api/icons.go` is the single source of truth for the 9-hex palette, name-to-monogram derivation, random color picking, the two validators, and the idempotent startup backfill — all proven by table tests.

## What Was Built

- **`internal/store/migrations/00009_project_icons.sql`** (D-07): SQL-only. Up adds `icon_letters` and `icon_color` as `TEXT NOT NULL DEFAULT ''` (so SQLite `ADD COLUMN` is happy and pre-existing rows are non-null-but-blank); Down `DROP COLUMN`s both in reverse order (modernc/SQLite 3.53 supports direct DROP COLUMN, mirroring 00007/00008). No letter/color logic in SQL.
- **`internal/api/icons.go`** (package `api`):
  - `projectPalette` — the exactly-9 lowercase Tailwind-600 hexes in locked order (D-01/D-02). Documented as the source of truth that Plan 03 mirrors in TS; text color is fixed white (`#ffffff`). No palette endpoint.
  - `deriveLetters(name string) string` (D-04/D-05/D-06) — splits on whitespace plus `- _ .`, keeps alphanumeric runes per token, skips leading/embedded non-alphanumerics; multi-token → first char of first two tokens, single-token → first two chars; Unicode-uppercased; zero-alphanumeric names fall back to `"?"`. Bounded work (T-18-02).
  - `pickColor() string` (D-03) — `projectPalette[rand.IntN(len(projectPalette))]` via `math/rand/v2`.
  - `validateIconLetters(s) (string, error)` (D-10) — trim, uppercase, cap at 2 runes, require ≥1, alphanumerics allowed (`7E` valid).
  - `validateIconColor(s) (string, error)` (D-11) — case-insensitive (`strings.EqualFold`) membership against the palette; returns canonical lowercase hex or rejects off-palette.
  - `BackfillProjectIcons(db *sql.DB) error` (D-08) — idempotent (`WHERE icon_letters = '' OR icon_color = ''`), collect-then-update (single-writer safe), reuses `deriveLetters` + `pickColor`, parameterized `?` placeholders only (T-18-01).
- **`internal/api/icons_test.go`** — `TestDeriveLetters` (all 7 CONTEXT-locked cases), `TestValidateIconLetters`, `TestValidateIconColor`, `TestPickColor`. All green.

## Task Commits

| Task | Name | Commit |
| ---- | ---- | ------ |
| 1 | Migration 00009 + icons palette/derive/pick/validators | be46dc6 |
| 2 | Table tests for deriveLetters + validators | 0f45b23 |
| 3 | BackfillProjectIcons idempotent batch pass | 708ff0a |

## Verification Results

- `go build ./...` exits 0
- `go vet ./internal/api/` exits 0
- `go test ./internal/api/` exits 0 (full package suite, including the 4 icon tests; all 7 locked deriveLetters cases pass)
- `internal/store/migrations/00009_project_icons.sql` present with both ADD COLUMN (Up) and reverse-order DROP COLUMN (Down)
- Palette source-of-truth: all 9 D-01 hexes present in `icons.go`

## TDD Gate Compliance

Plan task types: Tasks 1 & 2 are `tdd="true"`. Task 1 produced the implementation (`feat` commit be46dc6); Task 2 added the table tests (`test` commit 0f45b23) which ran GREEN against Task 1. The plan deliberately authors the implementation before the locked-expectation tests (the CONTEXT `<specifics>` cases are the spec), so the `test(...)` commit follows the `feat(...)` commit here. All assertions pass; no expectation was altered to fit the code. No REFACTOR commit was needed.

## Deviations from Plan

None — plan executed exactly as written. All three tasks' `<action>` and `<acceptance_criteria>` met; no Rule 1-4 deviations required.

## Notes for Downstream Plans

- Plan 02: `deriveLetters`, `pickColor`, `validateIconLetters`, `validateIconColor`, `BackfillProjectIcons`, and `projectPalette` are all package `api` (same package as `projects.go`) — call directly, no import. Wire `BackfillProjectIcons(db)` into `cmd/kangent/main.go` right after `store.Migrate(db)` with the `slog.Error + os.Exit(1)` fatal idiom.
- Validator error copy lives in package consts `errIconLettersRequired` / `errIconColorOffPalette` for `writeError` reuse at both create and PATCH.
- Plan 03: mirror `projectPalette` (the 9 lowercase hexes in this order) in TS; there is no palette endpoint by design (D-02).

## Self-Check: PASSED

- FOUND: internal/store/migrations/00009_project_icons.sql
- FOUND: internal/api/icons.go
- FOUND: internal/api/icons_test.go
- FOUND commit: be46dc6 (Task 1)
- FOUND commit: 0f45b23 (Task 2)
- FOUND commit: 708ff0a (Task 3)
