# Phase 18: Project Icon Data Foundation - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-19
**Phase:** 18-project-icon-data-foundation
**Areas discussed:** Curated palette, Backfill mechanism, PATCH validation, Letter derivation edge cases

---

## Curated Palette

| Option | Description | Selected |
|--------|-------------|----------|
| 9 hues @600 | red/orange/amber/green/teal/blue/indigo/violet/pink at Tailwind-600, white text, Go-const source + TS mirror, pure-random pick | ✓ |
| Tighter 6 hues | red/amber/green/blue/violet/pink, same rules | |
| Larger ~12 set | adds cyan/lime/rose/slate, same rules | |

**User's choice:** 9 hues @600 (Recommended)
**Notes:** Fixed white text for AA contrast on the zinc-900 dark theme. Source of truth is a Go const slice the backend random-picks from; Phase 19 mirrors the same list in TS — no palette endpoint. Color picked pure-random at creation, stored once → stable.

---

## Backfill Mechanism

| Option | Description | Selected |
|--------|-------------|----------|
| SQL adds cols + Go startup backfill | 00009 SQL ADDs columns; idempotent Go pass after Migrate() fills blanks via shared deriveLetters()+pickColor(); one source of truth, testable | ✓ |
| Pure-SQL migration | random color via CASE on abs(random())%9, letters via substr/instr/upper — self-contained but duplicates letter logic in SQL (divergence risk) | |
| Go-registered goose migration | goose.AddMigrationContext runs Go fns in the migration timeline — reuses Go logic but changes the SQL-only embed/registration setup | |

**User's choice:** SQL adds cols + Go startup backfill (Recommended)
**Notes:** Migration only ADDs columns (empty defaults). Idempotent Go pass right after `store.Migrate(db)` fills `WHERE icon_letters='' OR icon_color=''` using the same functions the create paths use. Guarantees backfill === create-time behavior.

---

## PATCH Validation

### icon_letters

| Option | Description | Selected |
|--------|-------------|----------|
| Uppercase, 1–2 alphanumerics, reject empty | trim/uppercase, keep ≤2 chars, require ≥1, allow letters AND digits; server is enforcer | ✓ |
| Uppercase letters only, exactly 2 | reject anything not exactly two A–Z; rejects digit-derived/single-char | |
| Lenient: any 1–2 chars | cap at 2, no case/charset restriction | |

### icon_color

| Option | Description | Selected |
|--------|-------------|----------|
| Must be in the curated palette | reject any color not in the 9-hue Go const (case-insensitive); enforced create + PATCH | ✓ |
| Accept any non-empty hex string | validate #rrggbb syntax only, don't restrict to palette | |

**User's choice:** Letters → "Uppercase, 1–2 alphanumerics, reject empty"; Color → "Must be in the curated palette"
**Notes:** Both fields wired into the existing partial-pointer PATCH pattern (omitted = untouched). Palette membership enforced at create AND PATCH; consistent with swatch-only editing and the deferred ICON-FUT-03 (free-form hex).

---

## Letter Derivation Edge Cases

### Word boundary

| Option | Description | Selected |
|--------|-------------|----------|
| Whitespace + - _ . | split on spaces AND separators: nf-core→NC, my_cool_app→MC, My Cool App→MC, kangent→KA | ✓ |
| Whitespace only (literal spec) | only spaces split: nf-core→NF, kangent→KA — matches REQUIREMENTS verbatim, weaker on hyphenated names | |

### Fallbacks

| Option | Description | Selected |
|--------|-------------|----------|
| Sensible defaults, your call | skip leading non-alphanumerics, ≤2 alphanumeric chars, Unicode upper, 1-char→1 letter, 0 alphanumerics→`?` | ✓ |
| Let me specify the fallbacks | user dictates explicitly | |

**User's choice:** Word split → "Whitespace + - _ ."; Fallbacks → "Sensible defaults, your call"
**Notes:** Folder/repo names are commonly hyphenated with no spaces, so separator-splitting yields better monograms while still honoring the spec's space examples. Fallbacks documented in CONTEXT for the planner.

---

## Claude's Discretion

- Go file placement of `deriveLetters` / `pickColor` / palette const and the backfill call site.
- RNG choice for `pickColor` (math/rand/v2 fine — single-user, non-crypto).
- Exact validation error copy / status codes (reuse the `update()` 400 pattern).
- Letter-derivation fallback specifics (D-05).

## Deferred Ideas

- Free-form / hex color picker beyond curated swatches — ICON-FUT-03.
- Image / logo / emoji project icons — ICON-FUT-01/02.
- >2-letter monograms — out of scope.
- Palette endpoint / dynamic palette — not built (Go const + TS mirror chosen).
