---
phase: 18-project-icon-data-foundation
reviewed: 2026-06-19T05:11:40Z
depth: standard
files_reviewed: 7
files_reviewed_list:
  - cmd/kangent/main.go
  - internal/api/icons.go
  - internal/api/icons_test.go
  - internal/api/projects.go
  - internal/store/migrations/00009_project_icons.sql
  - web/src/api/mutations.ts
  - web/src/api/types.ts
findings:
  critical: 0
  warning: 3
  info: 3
  total: 6
status: issues_found
---

# Phase 18: Code Review Report

**Reviewed:** 2026-06-19T05:11:40Z
**Depth:** standard
**Files Reviewed:** 7
**Status:** issues_found

## Summary

Phase 18 adds project-icon identity data: migration 00009 (`icon_letters`/`icon_color`, both `TEXT NOT NULL DEFAULT ''`), a new `internal/api/icons.go` (palette, `deriveLetters`, `pickColor`, `validateIconLetters`, `validateIconColor`, `BackfillProjectIcons`), wiring into `internal/api/projects.go` (struct/columns/scan + both create paths + partial PATCH), a startup backfill in `cmd/kangent/main.go`, and the TS wire-path additions.

The core mechanics are sound and were verified empirically:

- **SQL injection:** clean. Every query in the new code uses `?` placeholders; `name`/`id` are never concatenated. The PATCH builds `sets` from a fixed, code-controlled allowlist (no user keys reach the SQL string).
- **Single-writer backfill:** correct. `BackfillProjectIcons` collects rows into a slice and closes the cursor (`defer rows.Close()` + the loop drains `rows.Next()`) before issuing any `UPDATE`, so it never holds a read cursor open against the `SetMaxOpenConns(1)` connection during writes. It runs synchronously before `http.ListenAndServe`, so no concurrency race exists. It is crash-safe: each `UPDATE` is its own auto-commit txn and the idempotent `WHERE icon_letters = '' OR icon_color = ''` re-runs only unfilled rows on the next boot.
- **Column-order discipline:** consistent. The physical table order (id, name, repo_path, created_at, updated_at, description, github_repo, managed, icon_letters, icon_color) differs from `projectColumns`, but every read uses the explicit `projectColumns` list in both `SELECT`/`RETURNING` and `scanProject`'s `Scan`, so physical order is irrelevant. `scanProject`'s `Scan` order matches `projectColumns` exactly. The full project HTTP handler test suite passes, which exercises the create-path `INSERT ... RETURNING` and would crash on any mismatch.
- **Migration up/down:** safe. `DROP COLUMN` in reverse order is supported by the modernc/SQLite 3.53 line per project conventions; neither new column is indexed, so DROP cannot fail on that account.
- **Build + tests:** `go build ./...` clean, `go test ./internal/api/` passes, `tsc --noEmit` clean.

The findings below are an input-validation contract gap in the PATCH path (the one area the task flagged), a test-coverage gap on the new HTTP/backfill surface, plus minor quality items. No blockers.

## Warnings

### WR-01: PATCH `icon_letters` accepts arbitrary non-alphanumeric runes, breaking the monogram contract

**File:** `internal/api/icons.go:130-143` (used by `internal/api/projects.go:400-411`)
**Issue:** `validateIconLetters` enforces only *non-empty after trim* — it does **not** restrict runes to the alphanumeric "usable" class. Every other path that produces `icon_letters` (both create paths and `BackfillProjectIcons`) routes through `deriveLetters`, which filters via `isAlnum` (`unicode.IsLetter || unicode.IsDigit`) and can only emit letters/digits (or the `"?"` fallback). The PATCH validator is the lone writer that bypasses that rule, so a client can persist values the rest of the system can never generate. Verified empirically:

```
validateIconLetters("💀")   -> "💀"   err=<nil>
validateIconLetters("<>")    -> "<>"   err=<nil>
validateIconLetters("  !  ") -> "!"    err=<nil>
validateIconLetters("a💀")   -> "A💀"  err=<nil>
```

The function's own doc comment (lines 123-129) claims "Alphanumerics are allowed so digit monograms like 7E are valid" — implying alphanumerics are the accepted class — but the implementation accepts any glyph. This is a correctness/contract violation: stored data diverges from the documented and create-time invariant. It is *not* an XSS vector on its own (React escapes text content when the Phase 19 avatar renders it), so it stays a Warning, but the divergence will surface as inconsistent avatars and undermines D-05's "usable class" rule.

**Fix:** Filter to the same alnum class `deriveLetters` uses, and reject (or strip-then-recheck-empty) non-alnum input. Mirror the helper so create and PATCH cannot diverge:
```go
func validateIconLetters(s string) (string, error) {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		if isAlnum(r) {
			b.WriteRune(r)
			if utf8.RuneCountInString(b.String()) >= 2 {
				break
			}
		}
	}
	if b.Len() == 0 {
		return "", errors.New(errIconLettersRequired)
	}
	return strings.ToUpper(b.String()), nil
}
```
(Decide deliberately: reject-on-non-alnum vs. strip-and-keep. Either is fine, but the accepted class must match `deriveLetters`.)

### WR-02: No test coverage for `BackfillProjectIcons` or the icon create/PATCH HTTP paths

**File:** `internal/api/icons_test.go` (whole file); `internal/api/projects.go:184-185`, `277-278`, `400-423`
**Issue:** Tests cover only the four pure helpers (`deriveLetters`, `validateIconLetters`, `validateIconColor`, `pickColor`). The phase's stateful, load-bearing surface is untested:
- `BackfillProjectIcons` — the idempotency guarantee (`WHERE ... = '' OR ... = ''` is a no-op on the second run), the collect-then-update single-writer ordering, and "blank pre-existing rows get filled" are asserted by comment only, never by a test against a real DB.
- The create paths' `INSERT ... RETURNING projectColumns` returning populated `icon_letters`/`icon_color` — no HTTP-level assertion that a newly created project comes back with a derived monogram + palette color.
- The PATCH icon branches (valid update persists; off-palette color → 400 with row untouched; empty letters → 400 with row untouched) — entirely uncovered.

The existing project suite passes (so column order is implicitly validated), but the new behavior has no regression guard. WR-01 in particular would have been caught by a single PATCH test.

**Fix:** Add table-driven tests using the existing in-memory/temp-DB harness pattern (see `projects_test.go` / `integration_test.go`):
- `BackfillProjectIcons`: insert rows with blank icons, run, assert filled; run again, assert unchanged (idempotent); assert already-filled rows are not touched.
- Create (both branches): POST a project, assert the response carries non-empty `icon_letters` and a palette `icon_color`.
- PATCH: valid icon update persists; off-palette color and empty letters each 400 and leave the prior values intact.

### WR-03: `pickColor` chosen per-row inside the scan loop makes backfill output non-deterministic across partial-failure boots

**File:** `internal/api/icons.go:191`
**Issue:** `pickColor()` is called while scanning (`fills = append(..., color: pickColor())`). Combined with the per-row auto-commit `UPDATE`s (no wrapping transaction, lines 196-203), a crash partway through backfill leaves the already-updated rows with their colors and re-randomizes only the remainder on the next boot. That is acceptable behavior (each row still ends up with one valid palette color), but it means backfill results are not reproducible and a half-completed run is invisible to operators. This is the documented design (collect-then-update is required for the single writer), so it is a robustness note rather than a bug — flagged because the comment block (lines 167-172) explains *why no cursor is held* but not that the operation is non-atomic across rows.

**Fix:** Optional. If atomicity across the whole backfill is desired, wrap the `UPDATE` loop in a single `*sql.Tx` (`db.Begin()` → loop `tx.Exec` → `tx.Commit()`); this keeps the single-writer discipline (one connection, no open read cursor) while making the fill all-or-nothing. If non-atomic per-row fill is intended (it is cheap and self-healing), add a one-line comment saying so to prevent a future reader from "fixing" it incorrectly.

## Info

### IN-01: `validateIconLetters` cap-at-2 runs after `ToUpper`, while `deriveLetters` caps before — benign in Go today but fragile

**File:** `internal/api/icons.go:131-142` vs `icons.go:98-112`
**Issue:** `validateIconLetters` uppercases the full trimmed string, then takes the first 2 runes; `deriveLetters` takes runes first, then uppercases. For runes whose uppercase form expands to multiple runes (classic example `ß` → `SS`), these would disagree on the final glyph count. Verified that Go's `strings.ToUpper("ß")` returns `"ß"` (Go's simple case-mapping does **not** apply the special `ß`→`SS` rule), so there is no observable divergence today — hence Info, not Warning. It remains a latent inconsistency if the case-mapping behavior ever changes or a different expanding rune is involved.
**Fix:** For consistency, cap to 2 runes first, then uppercase, matching `deriveLetters` ordering. Folding both into one shared helper (see WR-01 fix) eliminates the question entirely.

### IN-02: `isAlnum` admits Unicode letter-modifiers/marks, yielding odd monograms

**File:** `internal/api/icons.go:37-39`
**Issue:** `isAlnum` uses `unicode.IsLetter`, which is true for letter-category runes that are not visually "letters" (e.g. U+02BC MODIFIER LETTER APOSTROPHE). A name like `"ʼquote"` derives to `"ʼQ"` (verified) — a monogram beginning with a modifier glyph that does not uppercase and renders poorly. The never-empty invariant still holds, and such names are rare, so this is a cosmetic edge case.
**Fix:** If tighter monograms are wanted, additionally require `unicode.IsLetter(r) && !unicode.Is(unicode.Lm, r) && !unicode.Is(unicode.M, r)` (or restrict to `unicode.In(r, unicode.L, unicode.Nd)` minus modifier categories). Low priority.

### IN-03: `deriveLetters` is missing the all-separator / whitespace-only test case

**File:** `internal/api/icons_test.go:8-29`
**Issue:** The 7 locked cases cover emoji-only → `"?"`, but not a name that is all separators/whitespace (`"---"`, `"   "`) — also a zero-token input that must fall back to `"?"`. Verified the code handles these correctly (both return `"?"`), but the fallback branch (`len(tokens) == 0`) is only partially exercised. Worth pinning so a future refactor of the tokenizer cannot silently regress the never-empty guarantee.
**Fix:** Add cases `{"all-separators", "---", "?"}` and `{"whitespace-only", "   ", "?"}` to `TestDeriveLetters`.

---

_Reviewed: 2026-06-19T05:11:40Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
