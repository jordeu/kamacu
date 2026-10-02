package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"kamacu/internal/github"
	"kamacu/internal/settings"
)

func TestIssueImportLifecycle(t *testing.T) {
	srv, db, _ := newTestServer(t)
	repo := gitRepoWithCommit(t)
	pid := createProject(t, srv, repo)
	if _, err := db.Exec("UPDATE projects SET github_repo='owner/repo' WHERE id=?", pid); err != nil {
		t.Fatal(err)
	}
	oldView, oldSearch := viewIssue, searchIssues
	t.Cleanup(func() { viewIssue, searchIssues = oldView, oldSearch })
	reads := 0
	viewIssue = func(_ context.Context, ghRepo, dir string, n int) (github.Issue, error) {
		reads++
		if ghRepo != "owner/repo" || dir != repo || n != 42 {
			t.Errorf("scope: %s %s %d", ghRepo, dir, n)
		}
		return github.Issue{Number: n, Title: "Fix issue", Body: "**Markdown**", URL: "https://github.com/owner/repo/issues/42", State: "open"}, nil
	}
	endpoint := fmt.Sprintf("%s/api/projects/%d/issues", srv.URL, pid)
	status, body := doJSON(t, "POST", endpoint+"/42/import", nil)
	if status != 201 {
		t.Fatalf("status=%d body=%v", status, body)
	}
	task := body["task"].(map[string]any)
	id := task["id"]
	if task["title"] != "Fix issue" || task["status"] != "todo" || task["source"] != "manual" || task["worktree_path"] == nil || task["issue_number"] != float64(42) || !strings.Contains(task["description"].(string), "**Markdown**\n\nImported from GitHub:") {
		t.Fatalf("task=%v", task)
	}
	status, body = doJSON(t, "POST", endpoint+"/42/import", nil)
	if status != 200 || body["already_imported"] != true || body["task"].(map[string]any)["id"] != id || reads != 1 {
		t.Fatalf("duplicate=%v reads=%d", body, reads)
	}
	searchIssues = func(_ context.Context, ghRepo, dir, q string, closed, assigned bool, page int) (github.IssuePage, error) {
		if ghRepo != "owner/repo" || dir != repo || q != "hello" || !closed || !assigned || page != 2 {
			t.Error("incorrect search parameters")
		}
		return github.IssuePage{Issues: []github.Issue{{Number: 42}}}, nil
	}
	status, body = doJSON(t, "GET", endpoint+"?q=hello&include_closed=true&assigned_to_me=true&page=2", nil)
	if status != 200 || body["issues"].([]any)[0].(map[string]any)["task_id"] != id {
		t.Fatalf("list=%v", body)
	}
	// The regular board must include the imported task.
	resp, err := http.Get(fmt.Sprintf("%s/api/projects/%d/tasks", srv.URL, pid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var tasks []Task
	if err = json.NewDecoder(resp.Body).Decode(&tasks); err != nil || len(tasks) != 1 || tasks[0].IssueRepo == nil {
		t.Fatalf("board=%+v err=%v", tasks, err)
	}
	// Repository casing changes cannot bypass duplicate prevention.
	if _, err = db.Exec("UPDATE projects SET github_repo='OWNER/REPO' WHERE id=?", pid); err != nil {
		t.Fatal(err)
	}
	status, body = doJSON(t, "POST", endpoint+"/42/import", nil)
	if status != 200 || body["already_imported"] != true {
		t.Fatalf("case duplicate=%v", body)
	}
}

func TestIssueImportGatesAndFailures(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepo(t)) // unborn repository: provisioning will fail
	endpoint := fmt.Sprintf("%s/api/projects/%d/issues", srv.URL, pid)
	oldView, oldSearch := viewIssue, searchIssues
	t.Cleanup(func() { viewIssue, searchIssues = oldView, oldSearch })
	calls := 0
	viewIssue = func(context.Context, string, string, int) (github.Issue, error) {
		calls++
		return github.Issue{}, errors.New("GitHub unavailable")
	}
	searchIssues = func(context.Context, string, string, string, bool, bool, int) (github.IssuePage, error) {
		calls++
		return github.IssuePage{}, errors.New("GitHub unavailable")
	}
	for _, tc := range []struct{ method, path string }{{"GET", ""}, {"GET", "/42"}, {"POST", "/42/import"}} {
		status, _ := doJSON(t, tc.method, endpoint+tc.path, nil)
		if status != 400 {
			t.Errorf("unlinked status=%d", status)
		}
	}
	if _, err := db.Exec("UPDATE projects SET github_repo='owner/repo' WHERE id=?", pid); err != nil {
		t.Fatal(err)
	}
	if err := settings.Set(db, settings.KeyGithubIntegration, "off"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ method, path string }{{"GET", ""}, {"GET", "/42"}, {"POST", "/42/import"}} {
		status, _ := doJSON(t, tc.method, endpoint+tc.path, nil)
		if status != 403 {
			t.Errorf("disabled status=%d", status)
		}
	}
	if calls != 0 {
		t.Fatalf("gates called GitHub %d times", calls)
	}
	if err := settings.Set(db, settings.KeyGithubIntegration, "on"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/0/import", "/-1/import", "/abc/import"} {
		status, _ := doJSON(t, "POST", endpoint+path, nil)
		if status != 400 {
			t.Errorf("invalid number status=%d", status)
		}
	}
	status, _ := doJSON(t, "GET", endpoint+"?page=34", nil)
	if status != 400 {
		t.Errorf("invalid page status=%d", status)
	}
	status, _ = doJSON(t, "POST", endpoint+"/42/import", nil)
	if status != 502 {
		t.Errorf("upstream status=%d", status)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed import inserted task: %d %v", count, err)
	}
	viewIssue = func(context.Context, string, string, int) (github.Issue, error) {
		return github.Issue{Number: 42, Title: "Task survives", URL: "https://github.com/owner/repo/issues/42"}, nil
	}
	status, body := doJSON(t, "POST", endpoint+"/42/import", nil)
	if status != 201 || body["task"].(map[string]any)["worktree_error"] == nil {
		t.Fatalf("provision failure=%d %v", status, body)
	}
}

func TestConcurrentIssueImport(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	if _, err := db.Exec("UPDATE projects SET github_repo='owner/repo' WHERE id=?", pid); err != nil {
		t.Fatal(err)
	}
	old := viewIssue
	t.Cleanup(func() { viewIssue = old })
	var ready sync.WaitGroup
	ready.Add(2)
	viewIssue = func(context.Context, string, string, int) (github.Issue, error) {
		ready.Done()
		ready.Wait()
		return github.Issue{Number: 42, Title: "Concurrent", URL: "https://github.com/owner/repo/issues/42"}, nil
	}
	results := make(chan int, 2)
	for range 2 {
		go func() {
			resp, err := http.Post(fmt.Sprintf("%s/api/projects/%d/issues/42/import", srv.URL, pid), "application/json", nil)
			if err != nil {
				results <- 0
				return
			}
			defer resp.Body.Close()
			results <- resp.StatusCode
		}()
	}
	a, b := <-results, <-results
	if !(a == 200 && b == 201 || a == 201 && b == 200) {
		t.Fatalf("statuses %d %d", a, b)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&count); err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}
