package settings_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kamacu/internal/settings"
)

func TestValidateBranchTemplate(t *testing.T) {
	tests := []struct {
		tpl     string
		wantErr string // "" means valid
	}{
		// Order matters: empty → unknown token → missing {id} → ref format.
		{"", "Template can't be empty."},
		{"   ", "Template can't be empty."},
		{"task/{slugg}-{id}", "Unknown token {slugg}. Use {slug}, {id}, or {title}."},
		{"task/{slug}", "Template must include {id} so branch names never collide."},
		{"task/{slug}-{id}.", "Not a valid git branch name."},
		{"-{id}", "Not a valid git branch name."}, // leading dash: git-legal, argv hazard (Pitfall 3)
		{"a..b/{id}", "Not a valid git branch name."},
		{"{id}.lock", "Not a valid git branch name."},
		{"task//{id}", "Not a valid git branch name."},
		{"task/{slug}-{id}", ""},
		{"{id}", ""},
		{"wip/{title}-{id}", ""},
	}
	for _, tt := range tests {
		t.Run(tt.tpl, func(t *testing.T) {
			err := settings.Validate(settings.KeyBranchTemplate, tt.tpl)
			checkErrString(t, err, tt.wantErr)
		})
	}
}

func TestValidateWorktreeBase(t *testing.T) {
	// An existing FILE (not directory) for the Not-a-directory case.
	filePath := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tests := []struct {
		value   string
		wantErr string
	}{
		{"relative/path", "Path must be absolute or start with ~/"},
		{filePath, fmt.Sprintf("Not a directory: %s", filePath)}, // raw entered value in message
		{filepath.Join(t.TempDir(), "does-not-exist-yet"), ""},   // nonexistent dir is fine (MkdirAll at use)
		{"~", ""},
		{"~/x", ""},
		{"/abs", ""},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			err := settings.Validate(settings.KeyWorktreeBase, tt.value)
			checkErrString(t, err, tt.wantErr)
		})
	}
}

func TestValidateShell(t *testing.T) {
	if err := settings.Validate(settings.KeyShell, "bash"); err != nil {
		t.Errorf("Validate(shell, bash) = %v, want nil", err)
	}
	for _, v := range []string{"zsh", "", "fish", "/bin/bash"} {
		err := settings.Validate(settings.KeyShell, v)
		checkErrString(t, err, "Unknown shell.")
	}
	// AllowedShells is the single source of truth shared with the API options;
	// "bash" is always present and first regardless of PATH state.
	if shells := settings.AllowedShells(); len(shells) == 0 || shells[0] != "bash" {
		t.Errorf("AllowedShells() = %v, want bash first", shells)
	}
}

// TestAllowedShellsWithTmuxOnPath covers the PATH-present half of TMUX-01:
// when tmux resolves, it is both offered and accepted. Skipped on hosts
// without tmux so CI stays honest on tmux-less machines.
func TestAllowedShellsWithTmuxOnPath(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH; PATH-present assertions not testable on this host")
	}
	if got := settings.AllowedShells(); !slices.Equal(got, []string{"bash", "tmux"}) {
		t.Errorf("AllowedShells() = %v, want [bash tmux]", got)
	}
	if err := settings.Validate(settings.KeyShell, "tmux"); err != nil {
		t.Errorf("Validate(shell, tmux) = %v, want nil (tmux on PATH)", err)
	}
	if err := settings.Validate(settings.KeyShell, "bash"); err != nil {
		t.Errorf("Validate(shell, bash) = %v, want nil", err)
	}
	checkErrString(t, settings.Validate(settings.KeyShell, "zsh"), "Unknown shell.")
}

// TestAllowedShellsWithPathScrubbed covers the PATH-absent half of TMUX-01:
// the option vanishes AND re-saving "tmux" is rejected — both track the same
// call-time LookPath truth. t.Setenv auto-restores PATH (no t.Parallel here).
func TestAllowedShellsWithPathScrubbed(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if got := settings.AllowedShells(); !slices.Equal(got, []string{"bash"}) {
		t.Errorf("AllowedShells() with scrubbed PATH = %v, want [bash]", got)
	}
	checkErrString(t, settings.Validate(settings.KeyShell, "tmux"), "Unknown shell.")
	if err := settings.Validate(settings.KeyShell, "bash"); err != nil {
		t.Errorf("Validate(shell, bash) with scrubbed PATH = %v, want nil", err)
	}
	checkErrString(t, settings.Validate(settings.KeyShell, "zsh"), "Unknown shell.")
}

// TestParseDoneSessionTTL covers the shared parse+disable helper (D-91): the
// single source of truth the validator and the reaper (09-05) both call, so
// they can never disagree about what "disabled" means.
func TestParseDoneSessionTTL(t *testing.T) {
	tests := []struct {
		value        string
		wantTTL      time.Duration
		wantDisabled bool
		wantErr      bool
	}{
		{"24h", 24 * time.Hour, false, false},
		{"90m", 90 * time.Minute, false, false},
		{"", 0, true, false},      // empty disables reaping
		{"0", 0, true, false},     // "0" disables reaping
		{"never", 0, true, false}, // "never" disables reaping
		{"  never  ", 0, true, false},
		{"0s", 0, true, false},  // zero duration can never expire → disabled
		{"-5m", 0, true, false}, // negative duration can never expire → disabled
		{"banana", 0, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			ttl, disabled, err := settings.ParseDoneSessionTTL(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseDoneSessionTTL(%q) err = nil, want non-nil", tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseDoneSessionTTL(%q) err = %v, want nil", tt.value, err)
			}
			if disabled != tt.wantDisabled {
				t.Errorf("ParseDoneSessionTTL(%q) disabled = %v, want %v", tt.value, disabled, tt.wantDisabled)
			}
			if ttl != tt.wantTTL {
				t.Errorf("ParseDoneSessionTTL(%q) ttl = %v, want %v", tt.value, ttl, tt.wantTTL)
			}
		})
	}
}

func TestValidateDoneSessionTTL(t *testing.T) {
	const canonical = "Enter a duration like 24h, 90m, or 'never' to disable."
	tests := []struct {
		value   string
		wantErr string // "" means valid
	}{
		{"24h", ""},
		{"90m", ""},
		{"never", ""},
		{"", ""},
		{"0", ""},
		{"banana", canonical},
		{"24", canonical}, // missing unit is a parse error
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			err := settings.Validate(settings.KeyDoneSessionTTL, tt.value)
			checkErrString(t, err, tt.wantErr)
		})
	}
}

// TestValidateGithubIntegration covers the GHSET-01/D-01 global toggle:
// only the lowercase literals "on"/"off" validate; anything else returns the
// canonical UI-SPEC copy "Choose on or off." (case-sensitive, like the other
// validator errors).
func TestValidateGithubIntegration(t *testing.T) {
	const canonical = "Choose on or off."
	tests := []struct {
		value   string
		wantErr string // "" means valid
	}{
		{"on", ""},
		{"off", ""},
		{"yes", canonical},
		{"", canonical},
		{"ON", canonical}, // case-sensitive: only lowercase literals accepted
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			err := settings.Validate(settings.KeyGithubIntegration, tt.value)
			checkErrString(t, err, tt.wantErr)
		})
	}
}

// TestValidatePRReviewSeed covers the GHREV/12-07 configurable seed prompt:
// free-text accepted (including empty, which means "no injection" downstream),
// but a pathological 2001-char paste is rejected with the canonical copy.
func TestValidatePRReviewSeed(t *testing.T) {
	if err := settings.Validate(settings.KeyPRReviewSeed, "anything"); err != nil {
		t.Errorf("Validate(pr_review_seed, \"anything\") = %v, want nil (free-text)", err)
	}
	if err := settings.Validate(settings.KeyPRReviewSeed, ""); err != nil {
		t.Errorf("Validate(pr_review_seed, \"\") = %v, want nil (empty allowed)", err)
	}
	tooLong := strings.Repeat("x", 2001)
	checkErrString(t, settings.Validate(settings.KeyPRReviewSeed, tooLong), "Prompt is too long.")
}

func TestValidateAgentExtraParamsIsPassThrough(t *testing.T) {
	for _, v := range []string{"", "--dangerously-skip-permissions", "anything at all \"even unclosed", "--settings x"} {
		if err := settings.Validate(settings.KeyAgentExtraParams, v); err != nil {
			t.Errorf("Validate(agent_extra_params, %q) = %v, want nil (pass-through)", v, err)
		}
	}
}

func TestSetNeverStoresInvalidValue(t *testing.T) {
	// BRANCH-02: validation failure means nothing is written.
	db := testDB(t)
	if err := settings.Set(db, settings.KeyBranchTemplate, "task/{slugg}"); err == nil {
		t.Fatal("Set with invalid template succeeded, want error")
	}
	got, err := settings.Get(db, settings.KeyBranchTemplate)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != settings.Defaults[settings.KeyBranchTemplate] {
		t.Errorf("Get after rejected Set = %q, want untouched default %q", got, settings.Defaults[settings.KeyBranchTemplate])
	}
}

func TestCheckRefFormat(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		ok   bool
	}{
		{"task/fix-login-42", true},
		{"-42", false}, // leading dash rejected app-side
		{"a..b", false},
		{"a.lock", false},
		{"a b", false},
		{"task//x", false},
		{"wip/x", true},
	}
	for _, tt := range tests {
		err := settings.CheckRefFormat(ctx, tt.name)
		if tt.ok && err != nil {
			t.Errorf("CheckRefFormat(%q) = %v, want nil", tt.name, err)
		}
		if !tt.ok {
			checkErrString(t, err, "Not a valid git branch name.")
		}
	}
}

func checkErrString(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Errorf("got error %q, want nil", err.Error())
		}
		return
	}
	if err == nil {
		t.Errorf("got nil error, want %q", want)
		return
	}
	if err.Error() != want {
		t.Errorf("error = %q, want %q (canonical copy must match UI-SPEC verbatim)", err.Error(), want)
	}
}
