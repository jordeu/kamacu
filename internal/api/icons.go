package api

import (
	"database/sql"
	"errors"
	"math/rand/v2"
	"strings"
	"unicode"
)

// projectPalette is the curated dark-theme avatar palette (D-01): 9 muted,
// desaturated hues with fixed white (#ffffff) text. The hues are deliberately
// low-saturation/mid-dark so they sit subtly on the dark sidebar (a gentle
// difference between projects, never a bright highlight — Phase 19 UAT) while
// every hue still clears WCAG AA (>=4.5:1) against white at monogram sizes.
// This Go slice is the SINGLE SOURCE OF TRUTH (D-02): the backend random-picks
// from it at project creation and the Phase 19 swatch UI mirrors the identical
// lowercase list in TS — there is deliberately NO palette endpoint. Hexes are
// stored/compared lowercase. Order is locked (it is the order the TS mirror
// reproduces). Members, in order: red, orange, amber, green, teal, blue,
// indigo, violet, pink. Migration 00010 remaps pre-existing rows from the
// original bright palette to these muted hues by position.
var projectPalette = []string{
	"#9e5757", "#9c6b4b", "#8a7345", "#4e7a54", "#46776f",
	"#5e719c", "#6c6699", "#84689e", "#9c6188",
}

// errIconLettersRequired is the canonical reject copy for an empty/whitespace
// icon_letters value. An avatar must always render at least one glyph, so an
// empty monogram is the lone hard error (D-10). Reused verbatim by both the
// create and PATCH callers so the message can never drift.
const errIconLettersRequired = "icon letters must contain at least one character"

// errIconColorOffPalette is the canonical reject copy for an off-palette
// icon_color (D-11). Free-form hex is deferred (ICON-FUT-03); only the nine
// curated palette members are accepted. Reused verbatim by create and PATCH.
const errIconColorOffPalette = "icon color must be one of the curated palette colors"

// isAlnum reports whether r is a Unicode letter or digit — the "usable" class
// for monogram derivation (D-05: digit-derived monograms like 7E are valid).
func isAlnum(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// isLetterSep reports whether r is a token separator for deriveLetters: any
// whitespace (the tokenize.go separator) PLUS the connector runes '-' '_' '.'
// (D-04 — folder basenames and repo names are commonly hyphened/underscored
// with no spaces).
func isLetterSep(r rune) bool {
	return unicode.IsSpace(r) || r == '-' || r == '_' || r == '.'
}

// deriveLetters turns a project name into its 1-2 char uppercase monogram
// (D-04/D-05/D-06). It is pure (no I/O) and the SINGLE source of the rule —
// both create paths AND the startup backfill call it, so backfill behavior ===
// create-time behavior (no divergence). The rule, adapting tokenize.go's
// for-range + strings.Builder flush idiom (separators here are whitespace plus
// '-' '_' '.'):
//
//   - Split the name into tokens on the separator set, keeping only the
//     alphanumeric runes of each token (leading/embedded non-alphanumerics are
//     skipped, so a token like ".env" contributes "env").
//   - Multi-token name  -> the first alphanumeric of each of the first two
//     non-empty tokens ("My Cool App" -> "MC", "nf-core" -> "NC").
//   - Single-token name -> that token's first two alphanumerics
//     ("kangent" -> "KA"); a one-char token yields a one-char monogram
//     ("X" -> "X").
//   - Result is Unicode-uppercased.
//   - A name with ZERO alphanumerics (e.g. emoji-only "🎉") falls back to "?"
//     so the column is never empty.
//
// Work is bounded: at most the first two usable tokens/chars are read, so a
// hostile/huge name costs O(name) scan with O(1) output (T-18-02).
func deriveLetters(name string) string {
	// Collect the alphanumeric-only form of each token, in order.
	var tokens []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	for _, r := range name {
		switch {
		case isLetterSep(r):
			flush()
		case isAlnum(r):
			cur.WriteRune(r)
		default:
			// Non-alphanumeric, non-separator (e.g. a stray symbol or emoji
			// inside a token): treated as a no-op so it is skipped, not kept.
		}
	}
	flush()

	if len(tokens) == 0 {
		return "?" // zero alphanumerics anywhere -> never-empty fallback (D-05)
	}

	var b strings.Builder
	if len(tokens) == 1 {
		// Single token: take its first two runes.
		for i, r := range []rune(tokens[0]) {
			if i >= 2 {
				break
			}
			b.WriteRune(r)
		}
	} else {
		// Multi-token: first rune of each of the first two tokens.
		for i := 0; i < 2 && i < len(tokens); i++ {
			b.WriteRune([]rune(tokens[i])[0])
		}
	}
	return strings.ToUpper(b.String())
}

// pickColor returns a uniformly random member of projectPalette (D-03: pure
// random at creation, no round-robin / repeat-avoidance). math/rand/v2 is fine
// here — this is a single-user local app and the choice is cosmetic, not a
// security boundary, so a non-crypto PRNG is the right tool.
func pickColor() string {
	return projectPalette[rand.IntN(len(projectPalette))]
}

// validateIconLetters normalizes a user-supplied monogram for storage (D-10):
// trim surrounding whitespace, keep up to 2 runes of the SAME alphanumeric
// "usable" class deriveLetters uses (isAlnum: Unicode letter or digit), then
// Unicode-uppercase. At least one usable char is REQUIRED — an empty/whitespace-
// only OR all-non-alphanumeric value (e.g. "💀", "!!") is the lone hard error
// (an avatar must render). Alphanumerics are allowed so digit monograms like
// "7E" are valid; non-alphanumerics (symbols, emoji) are skipped, mirroring
// deriveLetters so create and PATCH cannot diverge. Returns the normalized
// letters or the canonical errIconLettersRequired error. Reused by both create
// and PATCH so the rule never diverges.
func validateIconLetters(s string) (string, error) {
	var b strings.Builder
	kept := 0
	for _, r := range strings.TrimSpace(s) {
		if !isAlnum(r) {
			continue
		}
		b.WriteRune(r)
		kept++
		if kept >= 2 {
			break
		}
	}
	if kept == 0 {
		return "", errors.New(errIconLettersRequired)
	}
	return strings.ToUpper(b.String()), nil
}

// validateIconColor canonicalizes a user-supplied color against the curated
// palette (D-11): trim, then case-insensitively (strings.EqualFold) match each
// palette member, returning the canonical lowercase palette hex on a hit. Any
// off-palette value (free-form hex, a color name, garbage) is rejected with the
// canonical errIconColorOffPalette error. Enforced at BOTH create and PATCH.
func validateIconColor(s string) (string, error) {
	trimmed := strings.TrimSpace(s)
	for _, hex := range projectPalette {
		if strings.EqualFold(trimmed, hex) {
			return hex, nil
		}
	}
	return "", errors.New(errIconColorOffPalette)
}

// BackfillProjectIcons fills icon_letters/icon_color for every project row left
// blank by migration 00009 (D-08). It runs ONCE right after store.Migrate(db) at
// startup and is IDEMPOTENT: the WHERE icon_letters = '' OR icon_color = ''
// clause makes it a cheap no-op on subsequent boots once every row is filled. It
// reuses the SAME deriveLetters + pickColor the create paths use, so backfill
// behavior === create-time behavior (D-06/D-08, no divergence).
//
// Collect-then-update is REQUIRED: the store opens with db.SetMaxOpenConns(1)
// (single-writer discipline, store.go), so holding the SELECT cursor open while
// issuing UPDATEs on the same connection would deadlock. We scan every row that
// needs filling into a slice (computing letters/color during the scan), close
// the cursor, then run the parameterized UPDATEs. All SQL uses '?' placeholders
// only — name/id are NEVER string-concatenated into the query (T-18-01).
func BackfillProjectIcons(db *sql.DB) error {
	rows, err := db.Query(`SELECT id, name FROM projects WHERE icon_letters = '' OR icon_color = ''`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type fill struct {
		id      int64
		letters string
		color   string
	}
	var fills []fill
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		fills = append(fills, fill{id: id, letters: deriveLetters(name), color: pickColor()})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, f := range fills {
		if _, err := db.Exec(
			`UPDATE projects SET icon_letters = ?, icon_color = ? WHERE id = ?`,
			f.letters, f.color, f.id,
		); err != nil {
			return err
		}
	}
	return nil
}
