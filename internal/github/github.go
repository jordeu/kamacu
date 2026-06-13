// Package github is a best-effort, read-only leaf over the host `gh` CLI
// (modeled on internal/quota). It holds NO credentials and NEVER reimplements
// gh — it shells out with an arg array (never `sh -c`) and treats `gh` as a
// soft dependency: every gh path degrades (syntactic save, no error) when gh
// is absent or can't verify, so linking a repo never hard-blocks on the CLI
// (GHSET-03, D-11). The single hard error is a syntactically invalid ref.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"regexp"
	"strings"
)

// errInvalidRef is the canonical copy surfaced verbatim by the dialog. Every
// reject path returns this exact message so the UI string can never drift.
var errInvalidRef = errors.New("Not a valid repository — use owner/name or a GitHub URL.")

// repoRefRe matches the canonicalized `owner/name` shape. Both segments allow
// [A-Za-z0-9_.-]; the leading-dash hazard is rejected separately so a ref like
// `-owner/name` (a git/argv foot-gun) never slips through.
var repoRefRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// ParseRepoRef canonicalizes any accepted reference form to `owner/name`
// (D-09). It is pure — no I/O. Accepted inputs:
//
//	owner/name
//	https://github.com/owner/name(.git)
//	http://github.com/owner/name(.git)
//	git@github.com:owner/name(.git)
//
// A leading/trailing whitespace is trimmed. The ONLY hard error is syntactic
// invalidity (empty, wrong segment count, illegal characters, a leading dash
// on either segment) — in which case the canonical errInvalidRef is returned.
func ParseRepoRef(ref string) (string, error) {
	s := strings.TrimSpace(ref)
	if s == "" {
		return "", errInvalidRef
	}
	// Strip a recognized host prefix.
	switch {
	case strings.HasPrefix(s, "https://github.com/"):
		s = strings.TrimPrefix(s, "https://github.com/")
	case strings.HasPrefix(s, "http://github.com/"):
		s = strings.TrimPrefix(s, "http://github.com/")
	case strings.HasPrefix(s, "git@github.com:"):
		s = strings.TrimPrefix(s, "git@github.com:")
	}
	// Strip a trailing .git and any surrounding slashes.
	s = strings.TrimSuffix(s, ".git")
	s = strings.Trim(s, "/")
	if !repoRefRe.MatchString(s) {
		return "", errInvalidRef
	}
	parts := strings.Split(s, "/")
	// repoRefRe guarantees exactly one '/', so len(parts) == 2.
	for _, seg := range parts {
		if strings.HasPrefix(seg, "-") {
			return "", errInvalidRef
		}
	}
	return s, nil
}

// ValidateRepo is the soft-validate entry point used by the PATCH handler. It
// returns the canonical owner/name, whether gh verified it, and an error ONLY
// when the ref is syntactically invalid (the lone blocking case):
//
//   - ParseRepoRef errors                 → ("", false, err)   [HARD block]
//   - gh absent (LookPath fails)          → (parsed, false, nil) [soft save]
//   - gh present, `gh repo view` exit 0   → (nameWithOwner, true, nil)
//   - gh present, any nonzero / parse err → (parsed, false, nil) [soft save]
//
// The gh-canonicalized casing wins on a verified hit. gh auth/error exit codes
// are unreliable (cli/cli#8845), so any gh failure degrades to a syntactic save
// rather than blocking the link (D-11).
func ValidateRepo(ctx context.Context, ref string) (canonical string, verified bool, err error) {
	parsed, err := ParseRepoRef(ref)
	if err != nil {
		return "", false, err
	}
	if !Available() {
		return parsed, false, nil
	}
	out, runErr := exec.CommandContext(ctx, "gh", "repo", "view", parsed, "--json", "nameWithOwner").Output()
	if runErr != nil {
		return parsed, false, nil
	}
	var resp struct {
		NameWithOwner string `json:"nameWithOwner"`
	}
	if jsonErr := json.Unmarshal(out, &resp); jsonErr != nil || resp.NameWithOwner == "" {
		return parsed, false, nil
	}
	return resp.NameWithOwner, true, nil
}

// Available reports whether the `gh` CLI resolves on PATH. Checked at call time
// (microseconds on localhost) so install/uninstall is reflected without a
// restart — mirroring AllowedShells's tmux LookPath.
func Available() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}
