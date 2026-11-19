package api

import (
	"fmt"
	"net/http"
	"testing"
)

func TestTaskListWorkspaceOverviewFilters(t *testing.T) {
	srv, db, _ := newTestServer(t)
	first := createProject(t, srv, gitRepo(t))
	second := createProject(t, srv, gitRepo(t))
	var otherWorkspace int64
	if err := db.QueryRow(`INSERT INTO workspaces (name) VALUES ('Other') RETURNING id`).Scan(&otherWorkspace); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE projects SET workspace_id=? WHERE id=?`, otherWorkspace, second); err != nil {
		t.Fatal(err)
	}
	var workspace int64
	if err := db.QueryRow(`SELECT workspace_id FROM projects WHERE id=?`, first).Scan(&workspace); err != nil {
		t.Fatal(err)
	}
	for _, seed := range []struct {
		project               int64
		title, status, source string
	}{
		{first, "Work in progress", "in_progress", "manual"}, {first, "Ready for review", "in_review", "manual"},
		{first, "Backlog", "todo", "manual"}, {first, "Finished", "done", "manual"},
		{first, "Review workspace", "in_progress", "github_pr"}, {second, "Other workspace", "in_review", "manual"},
	} {
		if _, err := db.Exec(`INSERT INTO tasks (project_id,title,status,source,position) VALUES (?,?,?,?,0)`, seed.project, seed.title, seed.status, seed.source); err != nil {
			t.Fatal(err)
		}
	}
	status, rows := doJSONList(t, fmt.Sprintf("%s/api/tasks?workspace_id=%d&status=in_progress&status=in_review", srv.URL, workspace))
	if status != http.StatusOK || len(rows) != 2 {
		t.Fatalf("status=%d rows=%v", status, rows)
	}
	for i, title := range []string{"Work in progress", "Ready for review"} {
		if rows[i]["title"] != title {
			t.Errorf("row %d = %v, want %s", i, rows[i], title)
		}
	}
	status, rows = doJSONList(t, fmt.Sprintf("%s/api/tasks?workspace_id=%d&status=in_progress", srv.URL, otherWorkspace))
	if status != http.StatusOK || len(rows) != 0 {
		t.Fatalf("empty queue status=%d rows=%v", status, rows)
	}
	for _, query := range []string{"workspace_id=-1", "workspace_id=oops", "workspace_id=1%20OR%201=1", "status=unknown", "status=in_review&status="} {
		res, err := http.Get(srv.URL + "/api/tasks?" + query)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("%s status=%d, want 400", query, res.StatusCode)
		}
	}
}
