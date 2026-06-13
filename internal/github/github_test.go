package github

import "testing"

func TestParseRepoRef(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"shorthand", "cli/cli", "cli/cli", false},
		{"https", "https://github.com/cli/cli", "cli/cli", false},
		{"https dot-git", "https://github.com/cli/cli.git", "cli/cli", false},
		{"ssh dot-git", "git@github.com:cli/cli.git", "cli/cli", false},
		{"ssh no dot-git", "git@github.com:cli/cli", "cli/cli", false},
		{"http", "http://github.com/cli/cli", "cli/cli", false},
		{"surrounding whitespace", "  cli/cli  ", "cli/cli", false},
		{"dots and dashes in segments", "owner-name/repo.name_v2", "owner-name/repo.name_v2", false},
		{"not a repo", "not-a-repo", "", true},
		{"too many segments", "a/b/c", "", true},
		{"empty", "", "", true},
		{"whitespace only", "   ", "", true},
		{"leading dash owner", "-owner/name", "", true},
		{"leading dash name", "owner/-name", "", true},
		{"single segment via url", "https://github.com/onlyone", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRepoRef(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseRepoRef(%q) = (%q, nil), want non-nil error", tt.in, got)
				}
				if got != "" {
					t.Errorf("ParseRepoRef(%q) returned canonical %q on error, want empty", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRepoRef(%q) returned error %v, want nil", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseRepoRef(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestParseRepoRefCanonicalError asserts the EXACT canonical error copy the
// dialog surfaces verbatim, so the message can never drift unnoticed.
func TestParseRepoRefCanonicalError(t *testing.T) {
	_, err := ParseRepoRef("not-a-repo")
	if err == nil {
		t.Fatal("expected an error for an invalid ref")
	}
	const want = "Not a valid repository — use owner/name or a GitHub URL."
	if err.Error() != want {
		t.Errorf("error = %q, want exactly %q", err.Error(), want)
	}
}

// TestAvailable only asserts Available() returns without panicking; the value
// depends on whether gh resolves on the host PATH.
func TestAvailable(t *testing.T) {
	_ = Available()
}
