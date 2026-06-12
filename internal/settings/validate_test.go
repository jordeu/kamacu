package settings_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"kangent/internal/settings"
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
		{"-42", false},  // leading dash rejected app-side
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
