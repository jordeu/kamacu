package api

import "testing"

// TestDeriveLetters locks the 7 derivation cases from CONTEXT <specifics>
// (D-04/D-05, ICON-02). These expectations are CONTEXT-locked: if a case fails,
// the bug is in deriveLetters (icons.go), never these wants.
func TestDeriveLetters(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"multi-word", "My Cool App", "MC"},
		{"single", "kangent", "KA"},
		{"hyphen", "nf-core", "NC"},
		{"underscore", "my_cool_app", "MC"},
		{"digit", "7 Eleven", "7E"},
		{"single-char", "X", "X"},
		{"emoji-only", "🎉", "?"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveLetters(tt.in); got != tt.want {
				t.Errorf("deriveLetters(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestValidateIconLetters covers normalization (trim/upper/cap-2, alphanumeric-
// only filtering matching deriveLetters) and the lone hard error (empty,
// whitespace-only, or all-non-alphanumeric) per D-10.
func TestValidateIconLetters(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"two chars", "ab", "AB", false},
		{"trim and upper", "  a  ", "A", false},
		{"cap at two", "abc", "AB", false},
		{"digit monogram", "7e", "7E", false},
		{"empty", "", "", true},
		{"whitespace only", "   ", "", true},
		// WR-01: non-alphanumeric runes are skipped (mirrors deriveLetters),
		// not persisted, so PATCH can never store glyphs create-time can't emit.
		{"interspersed symbol", "A!B", "AB", false},
		{"emoji only", "💀", "", true},
		{"symbol only", "!!", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateIconLetters(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateIconLetters(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("validateIconLetters(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestValidateIconColor covers case-insensitive palette canonicalization and
// off-palette rejection per D-11.
func TestValidateIconColor(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"lowercase member", "#dc2626", "#dc2626", false},
		{"uppercase member canonicalized", "#DC2626", "#dc2626", false},
		{"trimmed member", "  #2563eb  ", "#2563eb", false},
		{"off-palette hex", "#000000", "", true},
		{"color name", "red", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateIconColor(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateIconColor(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("validateIconColor(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestPickColor asserts every pickColor result is a palette member (D-03).
func TestPickColor(t *testing.T) {
	members := make(map[string]bool, len(projectPalette))
	for _, hex := range projectPalette {
		members[hex] = true
	}
	for i := 0; i < 20; i++ {
		if got := pickColor(); !members[got] {
			t.Errorf("pickColor() = %q, not a member of projectPalette", got)
		}
	}
}
