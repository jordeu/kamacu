package settings

import (
	"strconv"
	"strings"
)

// ExpandTemplate replaces the three branch-template tokens with task values:
// {slug} → slug (callers produce it via worktree.Slug), {id} → decimal id,
// {title} → sanitizeTitle(title). Plain single-pass replacement, no
// recursion — expanded values are [a-z0-9-] and can never contain '{'.
func ExpandTemplate(tpl, slug string, id int64, title string) string {
	r := strings.NewReplacer(
		"{slug}", slug,
		"{id}", strconv.FormatInt(id, 10),
		"{title}", sanitizeTitle(title),
	)
	return r.Replace(tpl)
}

// sanitizeTitle applies the same constructive-charset algorithm as
// worktree.Slug (lowercase, collapse non-[a-z0-9] runs to '-', trim '-',
// fallback "task") but capped at 100 chars instead of 40 — "full title"
// semantics while keeping refs filesystem-sane (research OQ3: cap = 100).
// Reimplemented rather than imported: the settings package stays free of
// the worktree package (the {slug} ARGUMENT is the caller's job).
func sanitizeTitle(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 100 {
		s = strings.TrimRight(s[:100], "-") // charset is ASCII; byte slice is safe
	}
	if s == "" {
		return "task"
	}
	return s
}
