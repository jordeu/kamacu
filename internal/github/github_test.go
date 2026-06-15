package github

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

// TestRepoDescription exercises the four degrade-don't-break cases of the
// best-effort RepoDescription read (D-05): a verified repo's description rides
// through, while an empty description, an absent gh, and a failing runner all
// yield "" with NO error in the public signature — the caller (createByRepo)
// must never have to handle a description error or let one block create.
func TestRepoDescription(t *testing.T) {
	tests := []struct {
		name      string
		available bool
		runner    func(ctx context.Context, canonical string) string
		want      string
	}{
		{
			name:      "verified repo with a description",
			available: true,
			runner:    func(context.Context, string) string { return "hello world" },
			want:      "hello world",
		},
		{
			name:      "repo with an empty description",
			available: true,
			runner:    func(context.Context, string) string { return "" },
			want:      "",
		},
		{
			name:      "gh absent never invokes the runner",
			available: false,
			// If gh is reported absent, RepoDescription must short-circuit and
			// never call the runner — fail loudly if it does.
			runner: func(context.Context, string) string {
				t.Fatal("descriptionRunner called while gh is absent")
				return "should not happen"
			},
			want: "",
		},
		{
			name:      "runner failure degrades to empty",
			available: true,
			// A production ghDescription maps any gh/parse failure to "" itself,
			// so the runner-level contract is already error-free; this case
			// asserts RepoDescription faithfully returns whatever (possibly "")
			// the runner yields on a degraded read.
			runner: func(context.Context, string) string { return "" },
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer SetAvailableForTest(tt.available)()
			defer SetDescriptionRunnerForTest(tt.runner)()

			got := RepoDescription(context.Background(), "cli/cli")
			if got != tt.want {
				t.Errorf("RepoDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRepoDescriptionFakeGH stubs `gh` with a tiny script that echoes a
// `--json description` payload, exercising the production ghDescription runner
// (exec + decode) end-to-end without the real gh CLI or network. Mirrors
// TestViewPRFakeGH's restricted-PATH technique.
func TestRepoDescriptionFakeGH(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "gh")
	const payload = `{"description":"a managed checkout"}`
	script := "#!/bin/sh\necho '" + payload + "'\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir)

	got := RepoDescription(context.Background(), "cli/cli")
	if got != "a managed checkout" {
		t.Errorf("RepoDescription() = %q, want %q", got, "a managed checkout")
	}
}

// TestRepoDescriptionFakeGHEmpty asserts a repo with no description (gh emits
// `{"description":""}`) reads back as "" with no error — the natural default
// that lands in projects.description.
func TestRepoDescriptionFakeGHEmpty(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "gh")
	script := "#!/bin/sh\necho '{\"description\":\"\"}'\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir)

	if got := RepoDescription(context.Background(), "cli/cli"); got != "" {
		t.Errorf("RepoDescription() = %q, want empty", got)
	}
}

// TestViewPRNoGH forces gh to be unresolvable (an empty PATH) and asserts
// ViewPR degrades to a typed error + an empty PRDetail (degrade-don't-break) —
// it must NEVER panic and never return partial data.
func TestViewPRNoGH(t *testing.T) {
	// An empty dir on PATH so exec.LookPath("gh") fails deterministically,
	// regardless of whether gh is installed on the host.
	t.Setenv("PATH", t.TempDir())

	d, err := ViewPR(context.Background(), "cli/cli", 1)
	if err == nil {
		t.Fatal("ViewPR with gh absent returned nil error, want a typed error")
	}
	if d != (PRDetail{}) {
		t.Errorf("ViewPR with gh absent returned %+v, want a zero PRDetail", d)
	}
}

// viewPRRaw mirrors the lenient decode shape ViewPR uses internally: the
// PRDetail fields plus a nested author OBJECT that is flattened to AuthorLogin.
// The test exercises the decode contract (author.login flattening + empty-body
// tolerance) against a captured gh pr view fixture (RESEARCH §1).
type viewPRRaw struct {
	PRDetail
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	// commits decodes as an array of empty structs — we only need its length
	// for the PR commit count (the GitHub-style merge line, 12-07).
	Commits []struct{} `json:"commits"`
}

// ghViewFixture is a captured `gh pr view … --json …` payload (RESEARCH §1
// shape): author is an object, and body is an empty string (the empty-PR-body
// case D-11 must tolerate).
var ghViewFixture = []byte(`{
  "number": 1,
  "title": "interactive pr list",
  "body": "",
  "author": {"id": "x", "is_bot": false, "login": "vilmibm", "name": "Nate Smith"},
  "url": "https://github.com/cli/cli/pull/1",
  "headRefName": "gh-pr",
  "headRefOid": "e9a3253762e768badaa1d4a5b3d267416d1e42f4",
  "baseRefName": "prototype",
  "baseRefOid": "8ebaf1d3aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "isCrossRepository": false,
  "commits": [{"oid": "a"}, {"oid": "b"}, {"oid": "c"}],
  "state": "OPEN"
}`)

func TestViewPRDecodeFlattensAuthorAndEmptyBody(t *testing.T) {
	var raw viewPRRaw
	if err := json.Unmarshal(ghViewFixture, &raw); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	d := raw.PRDetail
	d.AuthorLogin = raw.Author.Login
	d.Commits = len(raw.Commits)

	if d.AuthorLogin != "vilmibm" {
		t.Errorf("AuthorLogin = %q, want %q (flattened from author.login)", d.AuthorLogin, "vilmibm")
	}
	if d.Body != "" {
		t.Errorf("Body = %q, want empty (empty-body tolerance, D-11)", d.Body)
	}
	if d.Number != 1 {
		t.Errorf("Number = %d, want 1", d.Number)
	}
	if d.Title != "interactive pr list" {
		t.Errorf("Title = %q, want %q", d.Title, "interactive pr list")
	}
	if d.HeadRefOid != "e9a3253762e768badaa1d4a5b3d267416d1e42f4" {
		t.Errorf("HeadRefOid = %q, want the captured head OID", d.HeadRefOid)
	}
	if d.HeadRefName != "gh-pr" {
		t.Errorf("HeadRefName = %q, want %q (head branch surfacing, GHREV-01)", d.HeadRefName, "gh-pr")
	}
	if d.Commits != 3 {
		t.Errorf("Commits = %d, want 3 (count of the commits array, merge line)", d.Commits)
	}
	if d.BaseRefName != "prototype" {
		t.Errorf("BaseRefName = %q, want %q", d.BaseRefName, "prototype")
	}
	if d.URL != "https://github.com/cli/cli/pull/1" {
		t.Errorf("URL = %q, want the captured url", d.URL)
	}
	if d.IsCrossRepository {
		t.Error("IsCrossRepository = true, want false")
	}
	if d.State != "OPEN" {
		t.Errorf("State = %q, want %q (plain json tag, decoded directly — D-09 banner + reaper source)", d.State, "OPEN")
	}
}

// TestViewPRFakeGH stubs `gh` with a tiny script that echoes the fixture, so
// the full ViewPR path (exec + decode + flatten) is exercised end-to-end
// without the real gh CLI or network. The script uses only the `echo` shell
// builtin (no external cat/printf) so it works under the restricted PATH this
// test sets — only the temp dir is on PATH, so no /usr/bin tools resolve.
func TestViewPRFakeGH(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "gh")
	// Compact the fixture to a single line so `echo` reproduces it faithfully.
	var compact bytes.Buffer
	if err := json.Compact(&compact, ghViewFixture); err != nil {
		t.Fatalf("compact fixture: %v", err)
	}
	// Escape for safe embedding inside double-quoted echo: the JSON has only
	// `"` to escape (no `$`, backtick, or backslash in this fixture).
	escaped := strings.ReplaceAll(compact.String(), `"`, `\"`)
	script := "#!/bin/sh\necho \"" + escaped + "\"\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir)

	d, err := ViewPR(context.Background(), "cli/cli", 1)
	if err != nil {
		t.Fatalf("ViewPR with fake gh: %v", err)
	}
	if d.AuthorLogin != "vilmibm" {
		t.Errorf("AuthorLogin = %q, want %q", d.AuthorLogin, "vilmibm")
	}
	if d.Number != 1 || d.Title != "interactive pr list" {
		t.Errorf("ViewPR returned %+v, want number 1 / interactive pr list", d)
	}
	if d.Body != "" {
		t.Errorf("Body = %q, want empty", d.Body)
	}
}
