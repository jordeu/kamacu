package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// createTask POSTs a task to a project and returns the decoded response body.
func createTask(t *testing.T, srv *httptest.Server, projectID int64, title string) map[string]any {
	t.Helper()
	status, body := doJSON(t, "POST", fmt.Sprintf("%s/api/projects/%d/tasks", srv.URL, projectID),
		map[string]any{"title": title})
	if status != http.StatusCreated {
		t.Fatalf("create task %q: status=%d body=%v", title, status, body)
	}
	return body
}

func taskID(t *testing.T, body map[string]any) int64 {
	t.Helper()
	id, ok := body["id"].(float64)
	if !ok {
		t.Fatalf("no numeric id in task body: %v", body)
	}
	return int64(id)
}

func TestTaskCreateDefaults(t *testing.T) {
	srv, _, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepo(t))

	body := createTask(t, srv, pid, "a")
	if body["status"] != "todo" {
		t.Errorf("status = %q, want %q", body["status"], "todo")
	}
	if body["description"] != "" {
		t.Errorf("description = %q, want empty", body["description"])
	}
	if body["title"] != "a" {
		t.Errorf("title = %q, want %q", body["title"], "a")
	}
	if _, ok := body["position"].(float64); !ok {
		t.Errorf("position missing or not a number: %v", body["position"])
	}
}

func TestTaskCreateLandsAtTop(t *testing.T) {
	srv, _, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepo(t))

	createTask(t, srv, pid, "first")
	createTask(t, srv, pid, "second")
	last := createTask(t, srv, pid, "third")

	status, list := doJSONList(t, fmt.Sprintf("%s/api/projects/%d/tasks", srv.URL, pid))
	if status != http.StatusOK {
		t.Fatalf("list status = %d, want 200", status)
	}
	if len(list) != 3 {
		t.Fatalf("len = %d, want 3", len(list))
	}
	lowest := list[0]
	if lowest["id"] != last["id"] {
		t.Errorf("first listed task id = %v, want last-created %v (new tasks at top)", lowest["id"], last["id"])
	}
	for i := 1; i < len(list); i++ {
		if list[i-1]["position"].(float64) >= list[i]["position"].(float64) {
			t.Errorf("positions not strictly ascending at index %d: %v", i, list)
		}
	}
}

func TestTaskCreateValidation(t *testing.T) {
	srv, _, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepo(t))

	status, body := doJSON(t, "POST", fmt.Sprintf("%s/api/projects/%d/tasks", srv.URL, pid),
		map[string]any{"title": "  "})
	if status != http.StatusBadRequest {
		t.Fatalf("whitespace title: status = %d, want 400; body=%v", status, body)
	}
	if body["error"] != "title is required" {
		t.Errorf("error = %q, want %q", body["error"], "title is required")
	}

	status, body = doJSON(t, "POST", srv.URL+"/api/projects/999/tasks", map[string]any{"title": "x"})
	if status != http.StatusNotFound {
		t.Fatalf("nonexistent parent: status = %d, want 404; body=%v", status, body)
	}
}

func TestTaskGet(t *testing.T) {
	srv, _, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepo(t))
	id := taskID(t, createTask(t, srv, pid, "deep link me"))

	status, body := doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d", srv.URL, id), nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", status, body)
	}
	if body["title"] != "deep link me" {
		t.Errorf("title = %q, want %q", body["title"], "deep link me")
	}
	if body["project_id"] != float64(pid) {
		t.Errorf("project_id = %v, want %v", body["project_id"], pid)
	}

	status, _ = doJSON(t, "GET", srv.URL+"/api/tasks/424242", nil)
	if status != http.StatusNotFound {
		t.Fatalf("unknown id: status = %d, want 404", status)
	}
}

func TestTaskUpdate(t *testing.T) {
	srv, _, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepo(t))
	id := taskID(t, createTask(t, srv, pid, "keep title"))

	status, body := doJSON(t, "PATCH", fmt.Sprintf("%s/api/tasks/%d", srv.URL, id),
		map[string]any{"description": "# md"})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", status, body)
	}
	if body["description"] != "# md" {
		t.Errorf("description = %q, want %q", body["description"], "# md")
	}
	if body["title"] != "keep title" {
		t.Errorf("title = %q, want unchanged %q", body["title"], "keep title")
	}

	status, body = doJSON(t, "PATCH", fmt.Sprintf("%s/api/tasks/%d", srv.URL, id),
		map[string]any{"title": ""})
	if status != http.StatusBadRequest {
		t.Fatalf("empty title: status = %d, want 400; body=%v", status, body)
	}
}

func TestTaskDelete(t *testing.T) {
	srv, _, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepo(t))
	id := taskID(t, createTask(t, srv, pid, "doomed"))

	req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/tasks/%d", srv.URL, id), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}

	status, _ := doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d", srv.URL, id), nil)
	if status != http.StatusNotFound {
		t.Fatalf("after hard delete: GET status = %d, want 404", status)
	}
}
