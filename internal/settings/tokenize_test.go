package settings_test

import (
	"reflect"
	"testing"

	"kangent/internal/settings"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"single flag", "--dangerously-skip-permissions", []string{"--dangerously-skip-permissions"}},
		{"double-quoted span", `--append-system-prompt "be terse"`, []string{"--append-system-prompt", "be terse"}},
		{"single-quoted span", `--append-system-prompt 'be terse'`, []string{"--append-system-prompt", "be terse"}},
		{"mid-token quotes", `a"b c"d`, []string{"ab cd"}},
		{"unclosed quote is lenient", `"unclosed span`, []string{"unclosed span"}},
		{"empty", "", nil},
		{"whitespace only", "   ", nil},
		{"multiple flags", "--a --b --c", []string{"--a", "--b", "--c"}},
		{"tabs and newlines split", "--a\t--b\n--c", []string{"--a", "--b", "--c"}},
		{"empty quotes produce no token", `""`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := settings.Tokenize(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Tokenize(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}
