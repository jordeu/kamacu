package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func response(body, headers string) []byte {
	return []byte("HTTP/2.0 200 OK\r\nContent-Type: application/json\r\n" + headers + "\r\n" + body)
}
func TestGitHubPaginationScopePollingAndAccount(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	calls := []string{}
	item := func(id, repo, updated string) string {
		return fmt.Sprintf(`{"id":%q,"updated_at":%q,"reason":"review_requested","repository":{"full_name":%q},"subject":{"title":"Review","type":"PullRequest","url":"https://api.github.com/repos/%s/pulls/7","latest_comment_url":""}}`, id, updated, repo, repo)
	}
	g := GitHub{Now: func() time.Time { return now }, Run: func(_ context.Context, args ...string) ([]byte, error) {
		path := args[len(args)-1]
		calls = append(calls, path)
		if args[0] != "api" || args[2] != "github.com" || args[4] != "GET" {
			t.Fatalf("unexpected argv: %v", args)
		}
		switch {
		case path == "user":
			return response(`{"login":"alice"}`, ""), nil
		case strings.Contains(path, "page=2"):
			return response("["+item("new-2", "ACME/REPO", "2026-11-02T11:59:59Z")+"]", ""), nil
		default:
			return response("["+strings.Join([]string{item("new-1", "acme/repo", "2026-11-02T11:59:58Z"), item("old", "acme/repo", "2026-11-01T00:00:00Z"), item("foreign", "other/repo", "2026-11-02T11:59:58Z")}, ",")+"]", `X-Poll-Interval: 120`+"\r\n"+`Link: <https://api.github.com/notifications?page=2>; rel="next"`+"\r\n"), nil
		}
	}}
	source := Source{Config: json.RawMessage(`{}`), Account: "alice", Checkpoint: "2026-11-02T11:00:00Z", CreatedAt: now.Add(-time.Hour).UnixMilli()}
	batch, err := g.Collect(context.Background(), source, map[string]bool{"acme/repo": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Events) != 2 || batch.PollAfter != 2*time.Minute || batch.Checkpoint != "2026-11-02T12:00:00Z" {
		t.Fatalf("batch: %+v", batch)
	}
	if !strings.Contains(calls[1], "all=true") || batch.Events[0].URL != "https://github.com/acme/repo/pull/7" {
		t.Fatalf("bad request/url: %v %+v", calls, batch)
	}
	source.Account = "bob"
	if _, err = g.Collect(context.Background(), source, map[string]bool{}); err == nil {
		t.Fatal("silently switched account")
	}
}
func TestGitHubRejectsUntrustedURLsAndPartialPages(t *testing.T) {
	for _, u := range []string{"https://evil.test/repos/acme/repo/pulls/1", "http://api.github.com/repos/acme/repo/pulls/1", "https://user@api.github.com/repos/acme/repo/pulls/1", "https://api.github.com:443/repos/acme/repo/pulls/1"} {
		if _, err := githubPath("github.com", u); err == nil {
			t.Fatalf("accepted %s", u)
		}
	}
	g := GitHub{Run: func(_ context.Context, args ...string) ([]byte, error) {
		switch args[len(args)-1] {
		case "user":
			return response(`{"login":"alice"}`, ""), nil
		case "notifications?page=2":
			return nil, errors.New("offline")
		default:
			return response(`[]`, `Link: <https://api.github.com/notifications?page=2>; rel="next"`+"\r\n"), nil
		}
	}}
	if _, err := g.Collect(context.Background(), Source{Config: json.RawMessage(`{}`), Checkpoint: "2026-11-02T11:00:00Z"}, map[string]bool{}); err == nil {
		t.Fatal("accepted partial scan")
	}
}
func TestGitHubEnrichIncludesInlineComments(t *testing.T) {
	paths := []string{}
	g := GitHub{Run: func(_ context.Context, args ...string) ([]byte, error) {
		path := args[len(args)-1]
		paths = append(paths, path)
		return response(`{"body":"Please fix the race","user":{"login":"reviewer"},"irrelevant":"omit"}`, ""), nil
	}}
	e := event()
	e.SubjectURL = "https://api.github.com/repos/acme/repo/pulls/7"
	e.LatestCommentURL = "https://api.github.com/repos/acme/repo/issues/comments/99"
	got, err := g.Enrich(context.Background(), Source{Config: json.RawMessage(`{}`)}, e)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 4 || !strings.Contains(paths[2], "/pulls/7/comments?") || !strings.Contains(got.Context, "reviewer") || strings.Contains(got.Context, "irrelevant") {
		t.Fatalf("enrichment: %v %s", paths, got.Context)
	}
}

func TestGitHubCheckIdentityUsesCurrentCredentials(t *testing.T) {
	login := "alice"
	calls := 0
	g := GitHub{Run: func(_ context.Context, args ...string) ([]byte, error) {
		calls++
		if args[len(args)-1] != "user" {
			t.Fatal("expected identity request", args)
		}
		return response(fmt.Sprintf(`{"login":%q}`, login), ""), nil
	}}
	source := Source{Account: "alice", Config: json.RawMessage(`{}`)}
	if err := g.CheckIdentity(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	login = "bob"
	if err := g.CheckIdentity(context.Background(), source); err == nil {
		t.Fatal("accepted changed GitHub account")
	}
	if calls != 2 {
		t.Fatalf("cached identity instead of rechecking: %d", calls)
	}
	source.Account = ""
	if err := g.CheckIdentity(context.Background(), source); err == nil {
		t.Fatal("accepted unbound source")
	}
}

func TestGitHubEnrichmentRetainsResolvedStateOutsideLargeExcerpts(t *testing.T) {
	g := GitHub{Run: func(_ context.Context, args ...string) ([]byte, error) {
		path := args[len(args)-1]
		if strings.Contains(path, "?") {
			return response(`[]`, ""), nil
		}
		body, _ := json.Marshal(map[string]any{"state": "closed", "merged": true, "body": strings.Repeat("Long description. ", 1000)})
		return response(string(body), ""), nil
	}}
	e := event()
	e.SubjectURL = "https://api.github.com/repos/acme/repo/pulls/7"
	enriched, err := g.Enrich(context.Background(), Source{Config: json.RawMessage(`{}`)}, e)
	if err != nil {
		t.Fatal(err)
	}
	if enriched.SubjectState != "merged" {
		t.Fatalf("lost resolved subject state: %+v", enriched)
	}
}

func errorResponse(status int) []byte {
	return []byte(fmt.Sprintf("HTTP/2.0 %d %s\r\nContent-Type: application/json\r\n\r\n{\"message\":\"resource request failed\"}", status, http.StatusText(status)))
}
func TestGitHubHTTPFailuresKeepStatusDespiteCLIExit(t *testing.T) {
	for _, status := range []int{401, 403, 404, 410, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			g := GitHub{Run: func(context.Context, ...string) ([]byte, error) {
				return errorResponse(status), errors.New("exit status 1")
			}}
			_, _, err := g.api(context.Background(), "github.com", "repos/acme/repo/pulls/7")
			var httpErr *GitHubHTTPError
			if !errors.As(err, &httpErr) || httpErr.StatusCode != status {
				t.Fatalf("lost HTTP status: %v", err)
			}
			missing := status == 404 || status == 410
			if errors.Is(err, ErrContextUnavailable) != missing {
				t.Fatalf("wrong missing-context classification: %v", err)
			}
		})
	}
}
func TestGitHubDeletedOptionalCommentPreservesOtherEvidence(t *testing.T) {
	paths := []string{}
	g := GitHub{Run: func(_ context.Context, args ...string) ([]byte, error) {
		path := args[len(args)-1]
		paths = append(paths, path)
		switch {
		case strings.Contains(path, "/issues/comments/"):
			return errorResponse(404), errors.New("exit status 1")
		case strings.Contains(path, "?"):
			return response(`[]`, ""), nil
		default:
			return response(`{"state":"closed","merged":true,"body":"Verified current subject"}`, ""), nil
		}
	}}
	e := event()
	e.SubjectURL = "https://api.github.com/repos/acme/repo/pulls/7"
	e.LatestCommentURL = "https://api.github.com/repos/acme/repo/issues/comments/99"
	fresh, err := g.Enrich(context.Background(), Source{Config: json.RawMessage(`{}`)}, e)
	if err != nil || fresh.SubjectState != "merged" || !strings.Contains(fresh.Context, "Verified current subject") || len(fresh.ContextWarnings) != 1 {
		t.Fatalf("lost partial evidence: %+v %v", fresh, err)
	}
	if len(paths) != 4 {
		t.Fatalf("stopped fetching other evidence after missing comment: %v", paths)
	}
}
func TestGitHubUnavailableSubjectDoesNotReuseStaleEvidence(t *testing.T) {
	for _, status := range []int{404, 410} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			g := GitHub{Run: func(context.Context, ...string) ([]byte, error) {
				calls++
				return errorResponse(status), errors.New("exit status 1")
			}}
			e := event()
			e.SubjectURL = "https://api.github.com/repos/acme/repo/pulls/7"
			e.Context = "Stale body"
			e.SubjectState = "merged"
			fresh, err := g.Enrich(context.Background(), Source{Config: json.RawMessage(`{}`)}, e)
			if err != nil || fresh.Context != "" || fresh.SubjectState != "" || len(fresh.ContextWarnings) != 1 || calls != 1 {
				t.Fatalf("missing subject was treated as resolved: %+v %v", fresh, err)
			}
		})
	}
}
func TestGitHubOptionalAuthorizationFailuresAreNotHidden(t *testing.T) {
	for _, status := range []int{401, 403, 429} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			g := GitHub{Run: func(_ context.Context, args ...string) ([]byte, error) {
				if strings.Contains(args[len(args)-1], "/issues/comments/") {
					return errorResponse(status), errors.New("exit status 1")
				}
				return response(`{"state":"open"}`, ""), nil
			}}
			e := event()
			e.SubjectURL = "https://api.github.com/repos/acme/repo/pulls/7"
			e.LatestCommentURL = "https://api.github.com/repos/acme/repo/issues/comments/99"
			if _, err := g.Enrich(context.Background(), Source{Config: json.RawMessage(`{}`)}, e); err == nil || errors.Is(err, ErrContextUnavailable) {
				t.Fatalf("masked authorization/rate failure: %v", err)
			}
		})
	}
}
func TestGitHubMissingIdentityIsStillFatal(t *testing.T) {
	g := GitHub{Run: func(context.Context, ...string) ([]byte, error) {
		return errorResponse(404), errors.New("exit status 1")
	}}
	if err := g.CheckIdentity(context.Background(), Source{Account: "alice", Config: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("missing identity was accepted")
	}
}
