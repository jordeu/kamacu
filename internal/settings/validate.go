package settings

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// AllowedShells is the curated shell set (SHELL-01). The SAME slice serves
// save-time validation here and the GET /api/settings "options" array —
// one source of truth, so future shells are data, not code (SHELL-02 seam).
var AllowedShells = []string{"bash"}

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
		for _, s := range AllowedShells {
			if value == s {
				return nil
			}
		}
		return errors.New("Unknown shell.")
	case KeyAgentExtraParams:
		// Pass-through field: validating individual claude flags is explicitly
		// out of scope (REQUIREMENTS Out of Scope).
		return nil
	}
	return nil
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
