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
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
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

// validateRunner is the overridable seam for ValidateRepo's gh-verification leg,
// mirroring clone.go's cloneRunner package var. Production points at ghValidate
// (real `gh repo view`); tests swap in a fake so the API-layer create-by-repo
// tests can inject a canonical/verified outcome WITHOUT a live gh or network
// (the seam recommended in 14-02-PLAN.md Task 2's validation-seam note). It is
// reached ONLY after ParseRepoRef succeeds, so it never sees syntactic garbage.
var validateRunner = ghValidate

// ValidateRepo is the soft-validate entry point used by the PATCH handler and
// the create-by-repo path. It returns the canonical owner/name, whether gh
// verified it, and an error ONLY when the ref is syntactically invalid (the
// lone blocking case):
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
	return validateRunner(ctx, parsed)
}

// ghValidate is the production validateRunner: it shells out to `gh repo view`
// with an arg array (NEVER `sh -c`) and returns the gh-canonicalized
// nameWithOwner on a verified hit, or (parsed, false, nil) on any gh failure
// (degrade-don't-break). parsed is the already-canonicalized owner/name.
func ghValidate(ctx context.Context, parsed string) (canonical string, verified bool, err error) {
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

// descriptionRunner is the overridable seam behind RepoDescription's
// gh-read leg, mirroring validateRunner = ghValidate. Production points at
// ghDescription (real `gh repo view … --json description`); tests swap in a
// fake so the create-by-repo path can inject a description WITHOUT a live gh or
// network. It is reached ONLY when Available() is true.
var descriptionRunner = ghDescription

// RepoDescription is a best-effort, error-free read of a verified repo's
// GitHub description (D-05 / RPROJ-02). It rides the existing `gh repo view`
// surface but is DELIBERATELY separate from ValidateRepo — its signature must
// never widen, since the PATCH update handler shares it. The public contract is
// degrade-don't-break: the caller (createByRepo) gets a plain string and NEVER
// has to handle a description error, so a missing/empty/failed read simply
// yields "" (the projects.description default) and never blocks create:
//
//   - gh absent (Available() false)        → ""  (runner never invoked)
//   - gh present, repo has a description   → the description string
//   - gh present, empty description        → ""  (the natural default)
//   - gh nonzero exit / parse failure      → ""  (degrade-don't-break)
func RepoDescription(ctx context.Context, canonical string) string {
	if !Available() {
		return ""
	}
	return descriptionRunner(ctx, canonical)
}

// ghDescription is the production descriptionRunner: it shells out to
// `gh repo view <canonical> --json description` with an arg array (NEVER
// `sh -c`) and returns the repo's description, mapping EVERY failure (gh
// nonzero exit, parse error) to "" so RepoDescription's public signature can
// stay error-free. An empty description reads back as "" naturally.
func ghDescription(ctx context.Context, canonical string) string {
	out, runErr := exec.CommandContext(ctx, "gh", "repo", "view", canonical, "--json", "description").Output()
	if runErr != nil {
		return ""
	}
	var resp struct {
		Description string `json:"description"`
	}
	if jsonErr := json.Unmarshal(out, &resp); jsonErr != nil {
		return ""
	}
	return resp.Description
}

// PRDetail is a fresh, full PR-metadata read for opening a review (RESEARCH
// §1). It is read on the OPEN click (not per-poll), so the Description tab
// gets `body` (the Phase 11 list omits it) and the freshest head/base OIDs.
type PRDetail struct {
	Number            int    `json:"number"`
	Title             string `json:"title"`
	Body              string `json:"body"`
	AuthorLogin       string `json:"-"` // flattened from author.login
	URL               string `json:"url"`
	HeadRefName       string `json:"headRefName"`
	HeadRefOid        string `json:"headRefOid"`
	BaseRefName       string `json:"baseRefName"`
	BaseRefOid        string `json:"baseRefOid"`
	IsCrossRepository bool   `json:"isCrossRepository"`
	// State is the PR's lifecycle state: "OPEN" | "CLOSED" | "MERGED"
	// (UPPERCASE — verified; gh has NO `merged` field, so merged is derived
	// from State == "MERGED"). Unlike Commits/AuthorLogin it decodes DIRECTLY
	// from the JSON, so a plain tag is correct. Drives the D-09 review banner
	// and is the single source the reaper's PRState read mirrors (13-01).
	State string `json:"state"`
	// Commits is the PR's commit count, DERIVED from the length of gh's
	// `commits` JSON array (not a scalar in the payload) — hence json:"-" so
	// the direct unmarshal never touches it; ViewPR sets it from len(raw.Commits).
	// GHREV-05/12-07: feeds the GitHub-style "<author> wants to merge <N>
	// commits into <base> from <head>" merge line.
	Commits int `json:"-"`
}

// ViewPR reads one PR's metadata via `gh pr view`. Read-only leaf surface
// (the package contract). Degrades to a typed error when gh is absent or the
// call/parse fails, so the open endpoint can surface the UI-SPEC §F
// provisioning error instead of crashing (GHSET-03, degrade-don't-break).
func ViewPR(ctx context.Context, repo string, n int) (PRDetail, error) {
	if !Available() {
		return PRDetail{}, errors.New("the GitHub CLI (gh) is not installed")
	}
	out, err := exec.CommandContext(ctx, "gh", "pr", "view", strconv.Itoa(n),
		"-R", repo, "--json",
		"number,title,body,author,url,headRefName,headRefOid,baseRefName,baseRefOid,isCrossRepository,commits,state",
	).Output()
	if err != nil {
		return PRDetail{}, fmt.Errorf("couldn't read PR #%d", n)
	}
	var raw struct {
		PRDetail
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
		// gh returns `commits` as an array of objects; we only need the count,
		// so decode each element as an empty struct (ignores all fields).
		Commits []struct{} `json:"commits"`
	}
	if jsonErr := json.Unmarshal(out, &raw); jsonErr != nil {
		return PRDetail{}, fmt.Errorf("couldn't parse PR #%d", n)
	}
	d := raw.PRDetail
	d.AuthorLogin = raw.Author.Login
	d.Commits = len(raw.Commits)
	return d, nil
}

// availableRunner is the overridable seam behind Available, mirroring
// cloneRunner/validateRunner. Production points at ghLookPath; tests in other
// packages swap it (via SetAvailableForTest) so the create-by-repo path can be
// exercised deterministically regardless of whether `gh` is installed on the
// CI/dev host.
var availableRunner = ghLookPath

// Available reports whether the `gh` CLI resolves on PATH. Checked at call time
// (microseconds on localhost) so install/uninstall is reflected without a
// restart — mirroring AllowedShells's tmux LookPath.
func Available() bool {
	return availableRunner()
}

// ghLookPath is the production availableRunner: a real PATH lookup for `gh`.
func ghLookPath() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}
