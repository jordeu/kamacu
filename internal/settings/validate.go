package settings

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// AllowedShells returns the curated shell set (SHELL-01). "tmux" is offered
// only while the binary resolves on PATH (TMUX-01) — checked at call time so
// install/uninstall is reflected without a restart. LookPath on localhost is
// microseconds; both call sites (save-time validation, GET options) share it,
// so the dropdown offering and the save acceptance can never disagree.
func AllowedShells() []string {
	shells := []string{"bash"}
	if _, err := exec.LookPath("tmux"); err == nil {
		shells = append(shells, "tmux")
	}
	return shells
}

// tokenRe matches any {...} group in a branch template. Braces are GIT-LEGAL
// (verified: `git check-ref-format refs/heads/{a}` passes), so unknown-token
// detection MUST be an explicit app check — git would happily accept a
// literal "{slugg}" in every branch name (Pitfall 2).
var tokenRe = regexp.MustCompile(`\{[^}]*\}`)

// Validate checks value for key, returning the canonical UI-SPEC error copy
// verbatim on failure — the frontend mirrors these strings exactly.
func Validate(key, value string) error {
	switch key {
	case KeyBranchTemplate:
		return validateBranchTemplate(value)
	case KeyWorktreeBase:
		return validateWorktreeBase(value)
	case KeyShell:
		for _, s := range AllowedShells() {
			if value == s {
				return nil
			}
		}
		return errors.New("Unknown shell.")
	case KeyDoneSessionTTL:
		// REAP-01/D-91: reject garbage, accept durations and the disable
		// sentinels. ParseDoneSessionTTL is the single source of truth shared
		// with the reaper (09-05), so validator and reaper can never disagree
		// about what "disabled" means — same shape as AllowedShells above.
		if _, _, err := ParseDoneSessionTTL(value); err != nil {
			return errors.New("Enter a duration like 24h, 90m, or 'never' to disable.")
		}
		return nil
	case KeyAgentExtraParams:
		// Pass-through field: validating individual claude flags is explicitly
		// out of scope (REQUIREMENTS Out of Scope).
		return nil
	}
	return nil
}

// ParseDoneSessionTTL parses the done_session_ttl setting (REAP-01/D-91) into
// a reaper window. It is the single source of truth for the disable semantics,
// shared by Validate (save-time) and the reaper (09-05, read-at-use):
//
//   - empty, "0", or "never" → disabled (reaping off)
//   - a Go-style duration <= 0 → disabled (it can never expire)
//   - a valid positive duration → that ttl, not disabled
//   - anything else → a non-nil parse error (rejected at save)
func ParseDoneSessionTTL(value string) (ttl time.Duration, disabled bool, err error) {
	v := strings.TrimSpace(value)
	if v == "" || v == "0" || v == "never" {
		return 0, true, nil
	}
	d, perr := time.ParseDuration(v)
	if perr != nil {
		return 0, false, perr
	}
	if d <= 0 {
		// A negative or zero duration can never expire — treat as disabled
		// rather than reaping everything in Done immediately.
		return 0, true, nil
	}
	return d, false, nil
}

// validateBranchTemplate applies the MANDATORY check order:
// empty → unknown token → missing {id} → git ref legality.
func validateBranchTemplate(tpl string) error {
	if strings.TrimSpace(tpl) == "" {
		return errors.New("Template can't be empty.")
	}
	for _, m := range tokenRe.FindAllString(tpl, -1) {
		tok := m[1 : len(m)-1]
		switch tok {
		case "slug", "id", "title":
		default:
			return fmt.Errorf("Unknown token {%s}. Use {slug}, {id}, or {title}.", tok)
		}
	}
	if !strings.Contains(tpl, "{id}") {
		return errors.New("Template must include {id} so branch names never collide.")
	}
	// Sample-expansion is sound by the invariance argument (06-RESEARCH.md
	// Pattern 5): every token expands to a non-empty string over [a-z0-9]
	// with internal '-' only, so any git-illegal construct can only come
	// from the template's literal characters and token boundaries — which
	// are identical for every real task. Legal with samples ⇒ legal always.
	sample := ExpandTemplate(tpl, "fix-login", 42, "fix-login")
	return CheckRefFormat(context.Background(), sample)
}

func validateWorktreeBase(value string) error {
	v := strings.TrimSpace(value)
	if v != "~" && !strings.HasPrefix(v, "~/") && !strings.HasPrefix(v, "/") {
		return errors.New("Path must be absolute or start with ~/")
	}
	// Expansion happens only here (exists-check) and at use; the RAW value
	// is what gets stored and compared (Pitfall 6: keeps the user's "~"
	// display form and Reset-to-default visibility intact).
	abs, err := ExpandHome(v)
	if err != nil {
		return err
	}
	if fi, err := os.Stat(abs); err == nil && !fi.IsDir() {
		return fmt.Errorf("Not a directory: %s", v)
	}
	// Nonexistent is fine: Create's MkdirAll makes the base on first use.
	return nil
}

// CheckRefFormat reports whether name is a usable branch name. It rejects a
// leading '-' first (git-legal but an argv hazard for `git worktree add -b`,
// Pitfall 3), then defers to `git check-ref-format refs/heads/<name>` —
// the positional form, which has no shorthand-expansion semantics. NEVER
// use the branch-shorthand flag (it can rewrite candidates like @{-1}
// instead of validating them). Arg array exec, never sh -c. Repo-less.
func CheckRefFormat(ctx context.Context, name string) error {
	if strings.HasPrefix(name, "-") {
		return errors.New("Not a valid git branch name.")
	}
	if err := exec.CommandContext(ctx, "git", "check-ref-format", "refs/heads/"+name).Run(); err != nil {
		return errors.New("Not a valid git branch name.")
	}
	return nil
}
