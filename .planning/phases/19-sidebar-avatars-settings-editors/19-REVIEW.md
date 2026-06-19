---
phase: 19-sidebar-avatars-settings-editors
reviewed: 2026-06-19T00:00:00Z
depth: standard
files_reviewed: 8
files_reviewed_list:
  - internal/api/icons.go
  - internal/api/icons_test.go
  - internal/store/migrations/00010_muted_palette.sql
  - web/src/components/layout/AppLayout.tsx
  - web/src/components/sidebar/ProjectSettingsDialog.tsx
  - web/src/components/sidebar/ProjectSidebar.tsx
  - web/src/components/ui/ProjectAvatar.tsx
  - web/src/lib/palette.ts
findings:
  critical: 1
  warning: 4
  info: 3
  total: 8
status: issues_found
---

# Phase 19: Code Review Report

**Reviewed:** 2026-06-19T00:00:00Z
**Depth:** standard
**Files Reviewed:** 8
**Status:** issues_found

## Summary

Reviewed the Phase 19 sidebar-avatar / project-settings-editor work plus the
UAT-driven muted-palette change. The backend palette (`icons.go`), its tests,
the TS mirror (`palette.ts`), and the `ProjectAvatar` primitive are well
constructed: I independently verified all 9 muted hexes clear WCAG AA
(>=4.5:1) against white (range 4.55–5.31, with `#9c6b4b` and `#8a7345` sitting
right on the 4.55 edge), the Up-migration remap source/target sets are disjoint
(no chaining hazard), and the `deriveLetters`/`validateIcon*` helpers are pure,
parameterized, and SQL-injection-free. The `AppLayout` controlled-sidebar +
localStorage persistence is correct, including the Ctrl+B keyboard path.

The serious defect is in the **Down** side of migration 00010: it is not safely
reversible. Any project created *after* 00010 is born on a muted hex via
`pickColor()` — it was never bright — yet the Down migration unconditionally
rewrites those muted hexes back to bright ones, corrupting the color of
natively-muted rows. A rollback of 00010 therefore mangles data it never
created.

Secondary concerns cluster around client/server divergence in the letter
normalization rule (`\p{N}` vs `unicode.IsDigit`, and JS full-case-folding of
`ß`), which the code claims to "mirror" but does not, producing wrong avatar
previews and confusing 400s for certain inputs.

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: Migration 00010 Down corrupts natively-muted rows (non-reversible)

**File:** `internal/store/migrations/00010_muted_palette.sql:19-29`

**Issue:** The Down migration assumes every muted hex in the `projects` table
got there via the Up remap (bright→muted) and reverses it
(`#9e5757`→`#dc2626`, etc.). That assumption is false. After 00010 is applied,
every newly created project picks its color from the *new* muted
`projectPalette` via `pickColor()` (`icons.go:124`), and `BackfillProjectIcons`
(run after `Migrate`, `main.go:77`) also fills blank rows from the muted
palette. A project created on, say, `#9e5757` was **never bright**. Running
`goose down` on 00010 will rewrite that row to `#dc2626` (bright) — corrupting a
color the Up never touched and the user may have explicitly chosen via the
settings swatch grid. The Down is positionally indistinguishable between
"remapped-from-bright" and "natively-muted," so it cannot be correct.

This is a data-integrity defect: a rollback silently changes user-visible
project identity colors to values that are no longer even members of the
shipped palette (`PROJECT_PALETTE` / `projectPalette` now hold only the muted
set), so the post-rollback colors fail `validateIconColor` and can't be
re-selected in the editor.

**Fix:** A positional color remap is inherently irreversible once the new
palette becomes the creation source. Either make the Down a deliberate no-op
(document that the muted palette is forward-only and rollback does not restore
bright hues), or gate the Down so it only touches rows that can be proven to
predate 00010. The simplest correct option:

```sql
-- +goose Down
-- The muted palette is forward-only: after 00010, new/backfilled projects are
-- born on muted hexes, so a positional reverse-remap would corrupt rows that
-- were never bright. Intentionally a no-op — rolling back the schema does not
-- un-mute existing colors.
SELECT 1;
```

If a true reversal is genuinely required, 00009/00010 must record provenance
(e.g. an `icon_color_origin` marker) so the Down can target only remapped rows —
but for a single-user local app, a documented no-op Down is the pragmatic,
correct choice.

## Warnings

### WR-01: Client letter rule diverges from server — `\p{N}` accepts non-`Nd` numbers the server rejects

**File:** `web/src/components/sidebar/ProjectSettingsDialog.tsx:49-52`

**Issue:** `normalizeIconLetters` uses `/[\p{L}\p{N}]/gu`. The Unicode `\p{N}`
class includes `Nl` (letter-numbers like `Ⅻ` U+216B) and `No` (other numbers
like `½`, `²`, `𝟙`). The server's `isAlnum` uses `unicode.IsDigit`, which is
`Nd` (decimal digit) **only** (`icons.go:42`). So the client preview accepts and
displays `Ⅻ`, `½`, `²`, `𝟙𝟚` as valid initials, but on save the server's
`validateIconLetters` strips every one of them, sees zero usable chars, and
returns the `errIconLettersRequired` 400 (`icons.go:150-152`). The doc comment
at lines 42-48 explicitly claims this function "mirrors the server's
validateIconLetters rule" — it does not. Result: a user types a glyph that
previews fine, then gets a confusing "must contain at least one character"
rejection.

**Fix:** Restrict the client regex to decimal digits to match Go's
`unicode.IsDigit`:

```ts
function normalizeIconLetters(raw: string): string {
  // \p{Nd} (decimal digits) matches Go's unicode.IsDigit; \p{N} (Nl/No) does not.
  const alnum = raw.match(/[\p{L}\p{Nd}]/gu) ?? [];
  return alnum.slice(0, 2).join("").toUpperCase();
}
```

### WR-02: `toUpperCase()` full-case-folds `ß`→`SS`; server keeps `ß` — preview disagrees with stored value

**File:** `web/src/components/sidebar/ProjectSettingsDialog.tsx:51`

**Issue:** JS `String.prototype.toUpperCase()` performs *full* Unicode case
mapping, so `"ß".toUpperCase()` → `"SS"`. Go's `strings.ToUpper` performs
*simple* (1:1) case mapping, so `strings.ToUpper("ß")` → `"ß"` (verified). A
user typing `ß` sees the preview avatar render `SS`, but the server stores and
returns `ß`. After the mutation invalidates the `projects` query, the avatar
flips from the previewed `SS` to the persisted `ß` — a visible
preview-vs-reality mismatch. (Same class for any rune whose JS uppercase
expands but Go's does not, e.g. `ŉ`.)

**Fix:** This is hard to fully reconcile without porting Go's simple-mapping
table to the client. At minimum, document that the preview is approximate, or
normalize on the client using a locale/simple-case approximation. A pragmatic
mitigation is to not over-promise: the comment at lines 42-48 should stop
claiming byte-for-byte mirroring, and the preview should be understood as
best-effort. If exactness matters, derive the displayed preview from the
server's echoed value after save rather than from the local draft.

### WR-03: `Ⅻ` and similar `Nl`/`No` runes fall through `deriveLetters` to "?" fallback

**File:** `internal/api/icons.go:41-42, 97-99`

**Issue:** `isAlnum` is `unicode.IsLetter(r) || unicode.IsDigit(r)`. Unicode
letter-numbers (`Nl`, e.g. Roman numerals `Ⅻ`) and other-numbers (`No`, e.g.
`½`, superscripts) are neither `IsLetter` nor `IsDigit`, so a project named
purely from such characters derives to `"?"` (verified: `deriveLetters("Ⅻ")` →
`"?"`). This is arguably acceptable (the never-empty fallback fires), but it is
inconsistent with intuition and with the client regex (WR-01), which *does*
treat these as usable. Worth noting because it compounds the create-vs-edit
divergence: a project auto-named with such glyphs gets `?`, but the user can't
re-derive the "expected" initials in the editor either.

**Fix:** Decide on one definition of "usable" and apply it on both sides. If
`Nl`/`No` should count, broaden `isAlnum` to use `unicode.IsNumber(r)` (covers
`Nd`+`Nl`+`No`) on the Go side and keep `\p{N}` on the client. If they should
not, narrow the client to `\p{Nd}` (WR-01). Either way, make the two
definitions identical so create-time, backfill, and edit-time all agree.

### WR-04: Off-palette / pre-migration color leaves swatch grid with no selected state and no recovery path

**File:** `web/src/components/sidebar/ProjectSettingsDialog.tsx:186-207`

**Issue:** The swatch grid marks a button selected only when
`hex.toLowerCase() === color.toLowerCase()` (line 187). If a project's stored
`icon_color` is *not* a current palette member — e.g. migration 00010 was
rolled back (see CR-01), or a legacy bright hex slipped through — then no swatch
is highlighted, the live-preview avatar shows the off-palette color, and there
is no visual cue that the current color is unselectable. The user can pick a new
one, but cannot tell the current state is "off palette." Combined with CR-01,
a rolled-back DB produces exactly this dead state for every project.

**Fix:** When `color` matches no palette member, surface it explicitly — e.g.
render the current color as a disabled "current (legacy)" chip outside the grid,
or auto-normalize `color` to the nearest/first palette member on open so the
preview and grid never disagree. At minimum, this dependency on CR-01 being
fixed should be noted.

## Info

### IN-01: `colorChanged` comparison is case-sensitive against a guaranteed-lowercase source

**File:** `web/src/components/sidebar/ProjectSettingsDialog.tsx:124`

**Issue:** `const colorChanged = color !== project.icon_color;` is a
case-sensitive string compare. In practice both sides are always lowercase
palette hexes (swatch buttons set lowercase values; server stores lowercase),
so this never false-positives today. But it is fragile — if a server response or
future code path ever yields an uppercase hex, this would send a redundant PATCH
that the server would canonicalize anyway. Low impact.

**Fix:** Compare case-insensitively for robustness:
`color.toLowerCase() !== project.icon_color.toLowerCase()`.

### IN-02: Duplicated destructive-error markup block

**File:** `web/src/components/sidebar/ProjectSettingsDialog.tsx:260-263, 271-275`

**Issue:** The destructive error `<p className="rounded-md border
border-destructive/50 bg-destructive/10 px-3 py-2 text-xs text-destructive">`
markup is duplicated verbatim for the integration-on (under repo field) and
integration-off (form-level) cases. The two blocks must be kept in sync by hand.

**Fix:** Extract a small `ErrorBanner` subcomponent (or a shared className
constant) and render it in both branches to keep the styling DRY.

### IN-03: `linkLabel` includes waiting copy but a sibling Tooltip + count chip re-announce it — minor redundancy

**File:** `web/src/components/sidebar/ProjectSidebar.tsx:71-72, 100-101, 124-126`

**Issue:** The expanded-row `Link` carries
`aria-label="{name}, {count} agents waiting for input"`, while the rail
`ProjectAvatar` also gets `aria-label={waitingLabel}` and the count chip carries
`aria-label={waitingLabel}` too. In the expanded state a screen reader may
encounter the waiting count up to twice within one row (link label + chip
label). Not a defect (no incorrect output), but slightly chatty.

**Fix:** Consider `aria-hidden` on the count chip in the expanded row since the
parent `Link`'s `aria-label` already conveys the count, leaving a single
announcement.

---

_Reviewed: 2026-06-19T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
