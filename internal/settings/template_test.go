package settings_test

import (
	"strings"
	"testing"

	"kangent/internal/settings"
)

func TestExpandTemplate(t *testing.T) {
	tests := []struct {
		name  string
		tpl   string
		slug  string
		id    int64
		title string
		want  string
	}{
		{"default shape", "task/{slug}-{id}", "fix-login", 42, "Fix login", "task/fix-login-42"},
		{"id only", "{id}", "x", 7, "x", "7"},
		{"title token sanitized", "wip/{title}-{id}", "fix-login", 9, "Fix Login!", "wip/fix-login-9"},
		{"empty title falls back to task", "wip/{title}-{id}", "task", 3, "", "wip/task-3"},
		{"symbols-only title falls back to task", "{title}-{id}", "task", 4, "!!!", "task-4"},
		{"no tokens passes through", "static", "s", 1, "t", "static"},
		{"repeated tokens", "{id}/{id}", "s", 5, "t", "5/5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := settings.ExpandTemplate(tt.tpl, tt.slug, tt.id, tt.title)
			if got != tt.want {
				t.Errorf("ExpandTemplate(%q, %q, %d, %q) = %q, want %q", tt.tpl, tt.slug, tt.id, tt.title, got, tt.want)
			}
		})
	}
}

func TestExpandTemplateTitleCappedAt100(t *testing.T) {
	long := strings.Repeat("abcde ", 40) // sanitizes to far more than 100 chars
	got := settings.ExpandTemplate("{title}-{id}", "s", 1, long)
	title := strings.TrimSuffix(got, "-1")
	if len(title) > 100 {
		t.Errorf("expanded {title} length = %d, want <= 100", len(title))
	}
	if strings.HasSuffix(title, "-") {
		t.Errorf("capped title %q has trailing '-', want trimmed", title)
	}
}

func TestExpandTemplateNoRecursion(t *testing.T) {
	// A title that sanitizes near a token must not be re-expanded:
	// sanitizer output is [a-z0-9-] so braces can never appear, but a
	// literal brace in the template stays literal after one pass.
	got := settings.ExpandTemplate("{title}-{id}", "s", 8, "{slug}")
	if got != "slug-8" {
		t.Errorf("ExpandTemplate single-pass = %q, want %q", got, "slug-8")
	}
}
