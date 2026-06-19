---
phase: 18-project-icon-data-foundation
verified: 2026-06-19T07:24:00Z
status: passed
score: 4/4 must-haves verified
overrides_applied: 0
---

# Phase 18: Project Icon Data Foundation Verification Report

**Phase Goal:** Every project — newly added or pre-existing — has two persisted properties (two uppercase letters + a curated-palette background color) that are sensible by default and editable over the API, so the frontend has stable identity data to render avatars from.
**Verified:** 2026-06-19T07:24:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (ROADMAP Success Criteria + PLAN must_haves)

| #   | Truth                                                                                                                                    | Status     | Evidence |
| --- | -------------------------------------------------------------------------------------------------------------------------------------- | ---------- | -------- |
| SC1 | Adding a new project (folder or managed-repo) auto-derives two uppercase letters ("My Cool App"→"MC", "kangent"→"KA") with no user action | ✓ VERIFIED | Both create paths call `deriveLetters(name), pickColor()` in their `INSERT … RETURNING` (projects.go:184-185 folder, 277-278 repo). End-to-end spot-check: POST "My Cool App" returned `"icon_letters":"MC"`. `TestDeriveLetters` passes all 7 locked cases. |
| SC2 | A new project gets a random color from a curated dark-theme palette, stable across restarts (ICON-03)                                    | ✓ VERIFIED | `pickColor()` returns `projectPalette[rand.IntN(9)]`; `projectPalette` holds exactly the 9 D-01 Tailwind-600 hexes (icons.go:19-22). Stored once in SQLite (TEXT NOT NULL) → stable. Spot-check: created color `#7c3aed` was a palette member; backfill color stable across two runs. |
| SC3 | After migration, every pre-existing project has non-empty letters + a palette color — none blank (ICON-04)                               | ✓ VERIFIED | Migration 00009 adds both columns `TEXT NOT NULL DEFAULT ''`; `BackfillProjectIcons` UPDATEs `WHERE icon_letters='' OR icon_color=''` using the same helpers; `main.go:77` invokes it after `store.Migrate`. End-to-end spot-check: pre-existing blank "nf-core" row → `letters="NC"`, palette color, after backfill. |
| SC4 | Letters/color survive restart (SQLite) and are readable+updatable via the project API; PATCH accepts icon_letters+icon_color (ICON-04)   | ✓ VERIFIED | `Project` struct + `projectColumns` + `scanProject` extended (projects.go:43-44, 90, 103); `list`/`RETURNING` echo both; PATCH `update` accepts both pointers with validation (projects.go:340-423). TS `Project` + `useUpdateProjectSettings` mirror the wire shape. Spot-check: valid PATCH persisted `ZZ`/`#2563eb`; off-palette `#000000` → 400 with row untouched. |

**Score:** 4/4 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/store/migrations/00009_project_icons.sql` | ADD both columns NOT NULL DEFAULT '' (Up); DROP both reverse-order (Down) | ✓ VERIFIED | Up adds `icon_letters` then `icon_color` (lines 9-10); Down drops `icon_color` then `icon_letters` (reverse, lines 16-17). SQL-only, no letter logic. Applies cleanly (goose migrated to v9 in spot-check). |
| `internal/api/icons.go` | palette, deriveLetters, pickColor, validateIconLetters, validateIconColor, BackfillProjectIcons | ✓ VERIFIED | 212 lines. All 6 symbols present and substantive. `projectPalette` = exactly 9 D-01 hexes. WR-01 fix present: `validateIconLetters` filters via `isAlnum` (lines 133-150). |
| `internal/api/icons_test.go` | table tests for deriveLetters + both validators + pickColor | ✓ VERIFIED | `TestDeriveLetters` (7 locked cases), `TestValidateIconLetters` (incl. WR-01 regressions: interspersed symbol→AB, emoji-only→err, symbol-only→err), `TestValidateIconColor`, `TestPickColor`. All pass. |
| `internal/api/projects.go` | extended struct/columns/scan, both create paths, PATCH validation | ✓ VERIFIED | Struct fields (43-44), projectColumns (90), scanProject (103), both INSERTs (184-185, 277-278), PATCH blocks (400-423), nothing-to-update guard (347-348). Column order consistent. |
| `cmd/kangent/main.go` | BackfillProjectIcons after store.Migrate | ✓ VERIFIED | `api.BackfillProjectIcons(db)` at line 77, immediately after `store.Migrate(db)` (line 67), with `slog.Error + os.Exit(1)` fatal idiom. |
| `web/src/api/types.ts` | Project interface with icon_letters + icon_color string | ✓ VERIFIED | Lines 25-26: `icon_letters: string; icon_color: string;` between `managed` and `created_at`. |
| `web/src/api/mutations.ts` | useUpdateProjectSettings carrying optional icon fields | ✓ VERIFIED | Args type adds `icon_letters?`/`icon_color?` (45-46); conditional body assembly sends each only when `!== undefined` (57-59). |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| projects.go create + createByRepo | deriveLetters + pickColor | INSERT args `deriveLetters(name), pickColor()` | ✓ WIRED | Appears 2× (lines 185, 278), one per create path. |
| projects.go update | validateIconLetters + validateIconColor | validate-then-append, reject 400 on error | ✓ WIRED | `validateIconLetters(*req.IconLetters)` (404), `validateIconColor(*req.IconColor)` (416); each errors → `writeError(…,400,…)` + early return. |
| icons.go BackfillProjectIcons | deriveLetters + pickColor | same create-path helpers (no divergence) | ✓ WIRED | Line 198: `fill{… letters: deriveLetters(name), color: pickColor()}`. |
| cmd/kangent/main.go | api.BackfillProjectIcons(db) | after store.Migrate(db) | ✓ WIRED | Line 77 follows line 67; line-order load-bearing (columns exist before query). |
| mutations.ts useUpdateProjectSettings | PATCH /api/projects/{id} | conditional payload (omitted = untouched) | ✓ WIRED | `patch<Project>(\`/api/projects/${id}\`, body)` with conditional field adds. |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| Project row (create) | icon_letters/icon_color | `deriveLetters(name)` + `pickColor()` at INSERT | Yes — derived from name + palette PRNG | ✓ FLOWING |
| Backfill | icon_letters/icon_color | SELECT blank rows → compute → UPDATE | Yes — real DB rows filled (spot-check: nf-core→NC) | ✓ FLOWING |
| PATCH | icon_letters/icon_color | validated request body → UPDATE … RETURNING | Yes — persisted + echoed (spot-check: ZZ/#2563eb) | ✓ FLOWING |
| TS mutation payload | icon_letters?/icon_color? | conditional body from caller args | Wire path only (no UI this phase, by design) | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Icon helper unit tests | `go test ./internal/api/ -run 'TestDeriveLetters\|TestValidateIconLetters\|TestValidateIconColor\|TestPickColor' -v` | All pass; 7 locked deriveLetters cases + WR-01 regressions green | ✓ PASS |
| Migration + backfill end-to-end | temp test: Migrate → insert blank "nf-core" → BackfillProjectIcons → assert | nf-core → letters="NC", palette color, idempotent across 2 runs | ✓ PASS |
| Create + PATCH at handler level | temp test: POST "My Cool App" → assert MC+palette; PATCH off-palette→400 untouched; valid→ZZ/#2563eb | All assertions pass | ✓ PASS |
| Backend build | `go build ./...` | exit 0 | ✓ PASS |
| Backend vet | `go vet ./internal/api/ ./cmd/kangent/` | exit 0 | ✓ PASS |
| Backend full api suite | `go test ./internal/api/` | ok | ✓ PASS |
| Frontend build | `cd web && npm run build` | exit 0 (tsc -b + vite build, 2216 modules) | ✓ PASS |

(Temporary verifier test files were created in-process, run, and removed; working tree restored clean.)

### Probe Execution

No probes declared in PLAN/SUMMARY and no conventional `scripts/*/tests/probe-*.sh` exist. Step 7c: SKIPPED (no probes for this phase).

### Requirements Coverage

| Requirement | Source Plan(s) | Description | Status | Evidence |
| ----------- | -------------- | ----------- | ------ | -------- |
| ICON-01 | 18-02 | Every project has a monogram avatar, auto-created, no user action | ✓ SATISFIED | Both create paths auto-assign letters+color at INSERT (data foundation for the avatar Phase 19 renders). |
| ICON-02 | 18-01, 18-02 | Two letters derived from name (multi-word → first letters of first two words; single → first two) | ✓ SATISFIED | `deriveLetters` + 7 locked tests; create paths use it. |
| ICON-03 | 18-01, 18-02 | Random color from curated dark-theme palette, stable across sessions | ✓ SATISFIED | 9-hex `projectPalette`, `pickColor()`, stored once in SQLite. |
| ICON-04 | 18-01, 18-02, 18-03 | Stored in SQLite, persist across restart; pre-existing rows backfilled by migration | ✓ SATISFIED | Migration 00009 + idempotent backfill + PATCH wire path + TS mirror. |

No orphaned requirements: REQUIREMENTS.md maps exactly ICON-01..04 to Phase 18, all claimed across the three plans' `requirements:` fields.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| — | — | None | — | No unreferenced TBD/FIXME/XXX debt markers; no empty-return stubs in icon code; no placeholder UI (no UI by design this phase). |

(False-positive scan hits: `web/src/api/types.ts` "todo" is the pre-existing task-status enum value, unrelated to icons; icons.go:178 "placeholders" is SQL terminology in a comment. Neither is a stub.)

### Code-Review Items (advisory, from 18-REVIEW.md)

- **WR-01** (validateIconLetters accepting non-alphanumeric runes) — FIXED in commit `aeff8e0`; verified in code (isAlnum filter, icons.go:137) and by tests (interspersed-symbol/emoji-only/symbol-only cases pass).
- **WR-02** (broader HTTP-path test coverage) — remains advisory; the verifier independently exercised the create + PATCH + backfill HTTP/DB paths via spot-checks (all green), so the behavior is proven even though no permanent committed test covers it. Not a goal blocker.
- **WR-03** (non-atomic per-row backfill) — documented and accepted; self-healing idempotent design. Not a goal blocker.

### Human Verification Required

None. All four success criteria are programmatically verifiable and were verified via unit tests + end-to-end DB/HTTP spot-checks. No visual/real-time/external-service behavior is in scope (no UI this phase — Phase 19 owns rendering and the editor). No `<verify><human-check>` blocks were deferred in the PLAN files.

### Gaps Summary

No gaps. Every must-have truth is VERIFIED against real source and proven behaviorally:

- Migration 00009 adds both columns (NOT NULL DEFAULT '') with a reverse-order DROP-COLUMN Down, and applies cleanly.
- icons.go holds the exact 9-hex palette, the WR-01-fixed validators, deterministic monogram derivation (7 locked cases), and an idempotent parameterized backfill that reuses the create-path helpers.
- projects.go extends struct/columns/scan in consistent order; both create paths assign letters+color; the partial PATCH validates both fields and leaves the row untouched on rejection.
- main.go invokes the backfill after migrations.
- The TS Project type and useUpdateProjectSettings carry the two fields on the wire for Phase 19.

Build, vet, backend tests, and frontend build all pass. Phase goal achieved.

---

_Verified: 2026-06-19T07:24:00Z_
_Verifier: Claude (gsd-verifier)_
