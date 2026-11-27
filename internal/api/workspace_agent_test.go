package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"kamacu/internal/session"
	"kamacu/internal/settings"
	"kamacu/internal/store"
	"kamacu/internal/tmux"
	"kamacu/internal/worktree"
)

func TestWorkspaceDefaultAgentForNewTasks(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	worktreeBase := t.TempDir()
	if err := settings.Set(db, settings.KeyWorktreeBase, worktreeBase); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Routes(mux, db, worktree.NewService(worktreeBase), session.NewManager(), tmux.Client{})

	request := func(method, path string, body map[string]any, want int) map[string]any {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, bytes.NewReader(raw)))
		if rec.Code != want {
			t.Fatalf("%s %s: got %d, want %d: %s", method, path, rec.Code, want, rec.Body)
		}
		if want == http.StatusNoContent {
			return nil
		}
		var result map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	id := func(result map[string]any) int64 { return int64(result["id"].(float64)) }
	assertTaskAgent := func(task map[string]any, want int64) {
		t.Helper()
		var got int64
		if err := db.QueryRow(`SELECT agent_id FROM task_agents WHERE task_id = ?`, id(task)).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("task %q: agent = %d, want %d", task["title"], got, want)
		}
	}

	claude := int64(1)
	first := id(request("POST", "/api/agents", map[string]any{"name": "First", "command": "first"}, 201))
	second := id(request("POST", "/api/agents", map[string]any{"name": "Second", "command": "second"}, 201))
	workspace := "/api/workspaces/1"
	project := request("POST", "/api/projects", map[string]any{"repo_path": gitRepo(t)}, 201)
	projectPath := "/api/projects/" + itoa(id(project))
	newTask := func(title string, override *int64) map[string]any {
		t.Helper()
		return request("POST", projectPath+"/tasks", map[string]any{"title": title, "agent_id": override}, 201)
	}

	old := newTask("Before opting in", nil)
	request("PATCH", workspace, map[string]any{"default_agent_id": first}, 200)
	request("PATCH", workspace, map[string]any{"default_agent_id": 999}, 400)
	request("DELETE", "/api/agents/"+itoa(first), nil, 409)
	assertTaskAgent(newTask("Project pin", nil), claude)
	inherited := request("PATCH", projectPath, map[string]any{"agent_id": nil}, 200)
	if inherited["agent_id"] != nil || inherited["workspace_agent_id"] != float64(first) {
		t.Fatalf("project agent settings = %v", inherited)
	}
	earlier := newTask("First workspace default", nil)
	if earlier["agent_id"] != nil || earlier["workspace_agent_id"] != float64(first) {
		t.Fatalf("inherited task agent settings = %v", earlier)
	}

	request("PATCH", workspace, map[string]any{"default_agent_id": second}, 200)
	assertTaskAgent(old, claude)
	assertTaskAgent(earlier, first)
	assertTaskAgent(newTask("Second workspace default", nil), second)
	pinned := newTask("Task pin", &first)
	assertTaskAgent(pinned, first)
	taskPath := "/api/tasks/" + itoa(id(earlier))
	request("PATCH", taskPath, map[string]any{"agent_id": second}, 200)
	assertTaskAgent(earlier, second)
	request("PATCH", taskPath, map[string]any{"agent_id": nil}, 200)
	assertTaskAgent(earlier, first)

	// An active snapshot protects its agent. Once every task is reassigned,
	// a fallback masked by a pin can be removed with the agent.
	request("DELETE", "/api/agents/"+itoa(first), nil, 409)
	request("PATCH", taskPath, map[string]any{"agent_id": second}, 200)
	request("PATCH", "/api/tasks/"+itoa(id(pinned)), map[string]any{"agent_id": second}, 200)
	request("DELETE", "/api/agents/"+itoa(first), nil, 204)
	unpinned := request("PATCH", taskPath, map[string]any{"agent_id": nil}, 200)
	if unpinned["workspace_agent_id"] != float64(second) {
		t.Fatalf("unpin did not capture the available workspace default: %v", unpinned)
	}
	request("PATCH", workspace, map[string]any{"default_agent_id": nil}, 200)
	assertTaskAgent(earlier, second)
	assertTaskAgent(newTask("Global fallback", nil), claude)

	request("PATCH", projectPath, map[string]any{"agent_id": claude}, 200)
	assertTaskAgent(newTask("Project pin restored", nil), claude)
	assertTaskAgent(pinned, second)
	request("PATCH", projectPath, map[string]any{"agent_id": nil}, 200)
	assertTaskAgent(earlier, claude)
	assertTaskAgent(pinned, second)
}
