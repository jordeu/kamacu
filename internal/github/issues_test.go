package github

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSearchIssuesScopeAndPagination(t *testing.T) {
	old := issueRunner
	t.Cleanup(func() { issueRunner = old })
	issueRunner = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if dir != "/project" {
			t.Errorf("dir=%s", dir)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Error("missing deadline")
		}
		joined := strings.Join(args, " ")
		for _, want := range []string{`repo:owner/repo is:issue is:open "repo:other/repo" "bug"`, "page=2", "per_page=30", "--hostname github.com"} {
			if !strings.Contains(joined, want) {
				t.Errorf("args %q missing %q", joined, want)
			}
		}
		return []byte(`{"total_count":70,"items":[{"number":1,"title":"Good","state":"open","repository_url":"https://api.github.com/repos/Owner/Repo","user":{"login":"alice"}},{"number":2,"repository_url":"https://api.github.com/repos/other/repo"},{"number":3,"repository_url":"https://api.github.com/repos/owner/repo","pull_request":{"url":"pr"}}]}`), nil
	}
	page, err := SearchIssues(context.Background(), "owner/repo", "/project", "repo:other/repo bug", false, false, 2)
	if err != nil || len(page.Issues) != 1 || !page.HasMore || page.Issues[0].Author != "alice" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}

func TestIssueNumberAndPRExclusion(t *testing.T) {
	old := issueRunner
	t.Cleanup(func() { issueRunner = old })
	payload := `{"number":42,"title":"Closed issue","state":"closed","body":null}`
	issueRunner = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[len(args)-1] != "repos/owner/repo/issues/42" {
			t.Errorf("unexpected args %v", args)
		}
		return []byte(payload), nil
	}
	for _, query := range []string{"42", "#42"} {
		page, err := SearchIssues(context.Background(), "owner/repo", "", query, false, false, 1)
		if err != nil || len(page.Issues) != 0 {
			t.Fatalf("open only: %+v %v", page, err)
		}
		page, err = SearchIssues(context.Background(), "owner/repo", "", query, true, false, 1)
		if err != nil || len(page.Issues) != 1 {
			t.Fatalf("include closed: %+v %v", page, err)
		}
	}
	payload = `{"number":42,"title":"A PR","pull_request":{"url":"pr"}}`
	if _, err := ViewIssue(context.Background(), "owner/repo", "", 42); err == nil {
		t.Fatal("accepted PR")
	}
	payload = `broken`
	if _, err := ViewIssue(context.Background(), "owner/repo", "", 42); err == nil {
		t.Fatal("accepted invalid JSON")
	}
	issueRunner = func(context.Context, string, ...string) ([]byte, error) { return nil, errors.New("offline") }
	if _, err := SearchIssues(context.Background(), "owner/repo", "", "", true, false, 1); err == nil {
		t.Fatal("swallowed failure")
	}
}

func TestSearchIssuesAssignedToMe(t *testing.T) {
	old := issueRunner
	t.Cleanup(func() { issueRunner = old })
	for _, assigned := range []bool{false, true} {
		issueRunner = func(_ context.Context, _ string, args ...string) ([]byte, error) {
			hasFilter := strings.Contains(strings.Join(args, " "), "assignee:@me")
			if hasFilter != assigned {
				t.Errorf("assignment filter = %v, want %v", hasFilter, assigned)
			}
			return []byte(`{"items":[],"total_count":0}`), nil
		}
		if _, err := SearchIssues(context.Background(), "owner/repo", "", "bug", true, assigned, 1); err != nil {
			t.Fatal(err)
		}
	}
	// Exact-number lookups must obey assignment filters too, including multiple assignees.
	for _, assigned := range []bool{false, true} {
		issueRunner = func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if args[len(args)-1] == "user" {
				return []byte(`{"login":"ALICE"}`), nil
			}
			assignees := `[{"login":"bob"}]`
			if assigned {
				assignees = `[{"login":"bob"},{"login":"alice"}]`
			}
			return []byte(`{"number":42,"title":"An issue","state":"open","assignees":` + assignees + `}`), nil
		}
		page, err := SearchIssues(context.Background(), "owner/repo", "", "#42", false, true, 1)
		if err != nil {
			t.Fatal(err)
		}
		if (len(page.Issues) == 1) != assigned {
			t.Fatalf("assigned=%v results=%+v", assigned, page)
		}
	}
	issueRunner = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[len(args)-1] == "user" {
			return nil, errors.New("authentication required")
		}
		return []byte(`{"number":42,"title":"An issue","state":"open"}`), nil
	}
	if _, err := SearchIssues(context.Background(), "owner/repo", "", "42", false, true, 1); err == nil {
		t.Fatal("account lookup failure was hidden")
	}
}
