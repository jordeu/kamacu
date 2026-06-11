package settings

import (
	"strings"
	"unicode"
)

// Tokenize splits s into argv tokens: runs of non-space, with '...' and "..."
// preserving spaces. No escapes, no expansion, no globbing — tokens pass to
// exec.Command verbatim. An unclosed quote consumes the rest of the string as
// one token (LENIENT: the field is pass-through per REQUIREMENTS Out of
// Scope, and no UI-SPEC error copy exists for it).
//
// Quirks worth knowing:
//   - Quote handling is mid-token (`a"b c"d` → `ab cd`), matching POSIX-ish
//     expectations.
//   - Known sharp edge (documented, not policed): a user-supplied --settings
//     here would override Kangent's hook overlay (commander last-value-wins)
//     and silently kill status hooks. The Phase 4 SessionStart canary detects
//     that (hooks-dead → bell fallback + log warning); policing the field is
//     explicitly out of scope.
func Tokenize(s string) []string {
	var tokens []string
	var cur strings.Builder
	var quote rune // 0 = unquoted
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case unicode.IsSpace(r):
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return tokens
}
