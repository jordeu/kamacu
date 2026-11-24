package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"kamacu/internal/session"
)

func TestTaskAgentModelEditing(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	status, agent := doJSON(t, "POST", srv.URL+"/api/agents", map[string]any{"name": "Other", "command": "other", "models": []string{"small", "large"}, "model_argument": "--model {{model}}"})
	if status != http.StatusCreated {
		t.Fatal(status, agent)
	}
	agentID := int64(agent["id"].(float64))
	if _, err := db.Exec(`UPDATE agents SET models='["sonnet","opus"]' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	status, task := doJSON(t, "POST", srv.URL+"/api/projects/"+itoa(pid)+"/tasks", map[string]any{"title": "Editable", "agent_id": agentID, "model": "small"})
	if status != http.StatusCreated {
		t.Fatal(status, task)
	}
	tid := taskID(t, task)
	url := srv.URL + "/api/tasks/" + itoa(tid)
	if _, err := db.Exec(`UPDATE tasks SET claude_session_id='claude-history', codex_session_id='codex-history', opencode_session_id='opencode-history' WHERE id=?`, tid); err != nil {
		t.Fatal(err)
	}
	patch := func(body map[string]any, expectedAgent any, expectedModel string) {
		t.Helper()
		status, result := doJSON(t, "PATCH", url, body)
		if status != http.StatusOK || result["agent_id"] != expectedAgent || result["model"] != expectedModel {
			t.Fatalf("patch %v = %d: %v", body, status, result)
		}
	}
	patch(map[string]any{"model": "large"}, float64(agentID), "large")
	if _, err := db.Exec(`UPDATE agents SET models='[]', model_argument='' WHERE id=?`, agentID); err != nil {
		t.Fatal(err)
	}
	patch(map[string]any{"model": ""}, float64(agentID), "")
	if _, err := db.Exec(`UPDATE agents SET models='["small","large"]', model_argument='--model {{model}}' WHERE id=?`, agentID); err != nil {
		t.Fatal(err)
	}
	patch(map[string]any{"model": "large"}, float64(agentID), "large")
	patch(map[string]any{"description": "Only the description"}, float64(agentID), "large")
	patch(map[string]any{"agent_id": 1}, float64(1), "")
	patch(map[string]any{"model": "sonnet"}, float64(1), "sonnet")
	// Keeping a saved model is legal even after it leaves the curated list.
	if _, err := db.Exec(`UPDATE agents SET models='["opus"]' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	patch(map[string]any{"agent_id": 1, "model": "sonnet"}, float64(1), "sonnet")
	patch(map[string]any{"agent_id": agentID, "model": "small"}, float64(agentID), "small")
	patch(map[string]any{"agent_id": nil}, nil, "")
	patch(map[string]any{"model": "opus"}, float64(1), "opus")
	patch(map[string]any{"agent_id": nil, "model": ""}, nil, "")
	// PR descriptions remain read-only in the UI, but their agent is editable.
	if _, err := db.Exec(`UPDATE tasks SET source='github_pr' WHERE id=?`, tid); err != nil {
		t.Fatal(err)
	}
	patch(map[string]any{"agent_id": agentID, "model": "large"}, float64(agentID), "large")
	var claude, codex, opencode string
	if err := db.QueryRow(`SELECT claude_session_id, codex_session_id, opencode_session_id FROM tasks WHERE id=?`, tid).Scan(&claude, &codex, &opencode); err != nil {
		t.Fatal(err)
	}
	if claude != "claude-history" || codex != "codex-history" || opencode != "opencode-history" {
		t.Fatal("editing configuration changed conversation history", claude, codex, opencode)
	}
}

func TestTaskAgentEditingRejectsInvalidSelectionsAtomically(t *testing.T) {
	srv, _, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	task := createTask(t, srv, pid, "Original")
	url := srv.URL + "/api/tasks/" + itoa(taskID(t, task))
	for _, body := range []map[string]any{
		{"agent_id": 999999, "title": "Should not save"},
		{"agent_id": "invalid"},
		{"agent_id": 1.5},
		{"model": "not configured", "description": "Should not save"},
	} {
		status, result := doJSON(t, "PATCH", url, body)
		if status != http.StatusBadRequest {
			t.Fatalf("invalid patch %v = %d: %v", body, status, result)
		}
	}
	status, loaded := doJSON(t, "GET", url, nil)
	if status != http.StatusOK || loaded["title"] != "Original" || loaded["description"] != "" || loaded["agent_id"] != nil || loaded["model"] != "" {
		t.Fatalf("invalid patch modified the task: %v", loaded)
	}
	status, _ = doJSON(t, "PATCH", srv.URL+"/api/tasks/999999", map[string]any{"model": ""})
	if status != http.StatusNotFound {
		t.Fatalf("unknown task = %d", status)
	}
}

func TestTaskAgentEditingRequiresStoppedAgentAndAppliesOnResume(t *testing.T) {
	srv, _, db, agentID, argsFile := newCodexResumeServer(t)
	if _, err := db.Exec(`UPDATE agents SET models='["first","second"]', model_argument='--model {{model}}' WHERE id=?`, agentID); err != nil {
		t.Fatal(err)
	}
	pid := createProject(t, srv, gitRepoWithCommit(t))
	status, task := doJSON(t, "POST", srv.URL+"/api/projects/"+itoa(pid)+"/tasks", map[string]any{"title": "Change model", "agent_id": agentID, "model": "first"})
	if status != http.StatusCreated {
		t.Fatal(status, task)
	}
	tid := taskID(t, task)
	url := srv.URL + "/api/tasks/" + itoa(tid)
	status, started := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "agent"})
	if status != http.StatusCreated {
		t.Fatal(status, started)
	}
	readArgv(t, argsFile)
	for _, body := range []map[string]any{{"model": "second", "title": "Should not save"}, {"agent_id": 1}} {
		status, result := doJSON(t, "PATCH", url, body)
		if status != http.StatusConflict {
			t.Fatalf("live configuration change = %d: %v", status, result)
		}
	}
	status, result := doJSON(t, "PATCH", url, map[string]any{"description": "Can edit text while running", "agent_id": agentID, "model": "first"})
	if status != http.StatusOK || result["title"] != "Change model" || result["model"] != "first" {
		t.Fatalf("idempotent configuration/text edit = %d: %v", status, result)
	}
	stopAndWaitExited(t, srv, tid, started["id"].(string))
	status, result = doJSON(t, "PATCH", url, map[string]any{"model": "second"})
	if status != http.StatusOK || result["model"] != "second" {
		t.Fatal(status, result)
	}
	conversation := "12345678-1234-1234-1234-123456789012"
	setTaskCodexSession(t, db, tid, conversation)
	if err := os.Remove(argsFile); err != nil {
		t.Fatal(err)
	}
	status, resumed := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "agent", "resume": true})
	if status != http.StatusCreated {
		t.Fatal(status, resumed)
	}
	if args := readArgv(t, argsFile); !reflect.DeepEqual(args, []string{"--no-alt-screen", "resume", conversation, "--model", "second"}) {
		t.Fatalf("resume args = %q", args)
	}
	stopAndWaitExited(t, srv, tid, resumed["id"].(string))
	status, result = doJSON(t, "PATCH", url, map[string]any{"agent_id": 1})
	if status != http.StatusOK || result["agent_id"] != float64(1) || result["model"] != "" {
		t.Fatal(status, result)
	}
	t.Setenv("FAKE_CLAUDE_ARGS_FILE", argsFile)
	if err := os.Remove(argsFile); err != nil {
		t.Fatal(err)
	}
	status, started = doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "agent"})
	if status != http.StatusCreated {
		t.Fatal(status, started)
	}
	if args := readArgv(t, argsFile); len(args) < 2 || args[0] != "--session-id" || strings.Contains(strings.Join(args, "|"), "--model") {
		t.Fatalf("new agent args = %q", args)
	}
}

func TestTaskAgentEditingSharesSpawnLifecycleLock(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	task := createTask(t, srv, pid, "Serialized")
	tid := taskID(t, task)
	mgr := session.NewManager()
	h := &taskHandlers{db: db, mgr: mgr}
	unlock := mgr.LockTaskLifecycle(tid)
	var once sync.Once
	release := func() { once.Do(unlock) }
	defer release()
	request := httptest.NewRequest(http.MethodPatch, "/api/tasks/"+itoa(tid), strings.NewReader(`{"agent_id":1,"model":""}`))
	request.SetPathValue("id", itoa(tid))
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { h.update(response, request); close(done) }()
	select {
	case <-done:
		t.Fatal("configuration edit did not wait for the spawn lifecycle lock")
	case <-time.After(50 * time.Millisecond):
	}
	// Text editing remains independent of agent launch/configuration changes.
	text := httptest.NewRequest(http.MethodPatch, "/api/tasks/"+itoa(tid), strings.NewReader(`{"description":"Text edited during launch"}`))
	text.SetPathValue("id", itoa(tid))
	textResponse := httptest.NewRecorder()
	h.update(textResponse, text)
	if textResponse.Code != http.StatusOK {
		t.Fatalf("text edit = %d", textResponse.Code)
	}
	release()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("configuration edit did not finish after releasing the lifecycle lock")
	}
	if response.Code != http.StatusOK {
		t.Fatalf("configuration edit = %d: %s", response.Code, response.Body.String())
	}
}

func TestTaskAgentEditingAllowsRunningBashTabs(t *testing.T) {
	srv, mgr, _, agentID, _ := newCodexResumeServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	task := createTask(t, srv, pid, "Bash still open")
	tid := taskID(t, task)
	_, err := mgr.Spawn(session.SpawnOpts{Kind: session.KindBash, TaskID: tid, Cwd: task["worktree_path"].(string), Shell: "/bin/sh"})
	if err != nil {
		t.Fatal(err)
	}
	status, result := doJSON(t, "PATCH", srv.URL+"/api/tasks/"+itoa(tid), map[string]any{"agent_id": agentID})
	if status != http.StatusOK || result["agent_id"] != float64(agentID) {
		t.Fatalf("editing with a bash tab = %d: %v", status, result)
	}
}

func TestTaskAgentEditingRejectsStaleDrafts(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	if _, err := db.Exec(`UPDATE agents SET models='["sonnet","opus"]' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	task := createTask(t, srv, pid, "Concurrent edits")
	url := srv.URL + "/api/tasks/" + itoa(taskID(t, task))
	status, result := doJSON(t, "PATCH", url, map[string]any{"agent_id": 1, "model": "sonnet", "expected_agent_id": nil, "expected_model": ""})
	if status != http.StatusOK {
		t.Fatal(status, result)
	}
	for _, stale := range []map[string]any{
		{"agent_id": nil, "model": "", "expected_agent_id": nil, "expected_model": "sonnet"},
		{"model": "opus", "expected_agent_id": 1, "expected_model": ""},
	} {
		stale["description"] = "Must not overwrite"
		status, result = doJSON(t, "PATCH", url, stale)
		if status != http.StatusConflict {
			t.Fatalf("stale update = %d: %v", status, result)
		}
	}
	status, result = doJSON(t, "PATCH", url, map[string]any{"description": "Separate text edit"})
	if status != http.StatusOK {
		t.Fatal(status, result)
	}
	status, result = doJSON(t, "PATCH", url, map[string]any{"model": "opus", "expected_agent_id": 1, "expected_model": "sonnet"})
	if status != http.StatusOK || result["description"] != "Separate text edit" {
		t.Fatalf("independent text edit should not conflict = %d: %v", status, result)
	}
	status, result = doJSON(t, "PATCH", url, map[string]any{"model": "", "expected_agent_id": "invalid"})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid precondition = %d: %v", status, result)
	}
}

func TestTaskAgentEditingConcurrentDraftsCannotOverwriteEachOther(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	if _, err := db.Exec(`UPDATE agents SET models='["sonnet","opus"]' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	task := createTask(t, srv, pid, "Simultaneous settings edits")
	url := srv.URL + "/api/tasks/" + itoa(taskID(t, task))
	type outcome struct {
		status int
		model  string
		err    error
	}
	results := make(chan outcome, 2)
	start := make(chan struct{})
	for _, model := range []string{"sonnet", "opus"} {
		go func(model string) {
			<-start
			payload, _ := json.Marshal(map[string]any{"agent_id": 1, "model": model, "expected_agent_id": nil, "expected_model": ""})
			req, err := http.NewRequest(http.MethodPatch, url, strings.NewReader(string(payload)))
			if err != nil {
				results <- outcome{err: err}
				return
			}
			req.Header.Set("Content-Type", "application/json")
			response, err := srv.Client().Do(req)
			if err != nil {
				results <- outcome{err: err}
				return
			}
			response.Body.Close()
			results <- outcome{status: response.StatusCode, model: model}
		}(model)
	}
	close(start)
	winner := ""
	conflicts := 0
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		switch result.status {
		case http.StatusOK:
			if winner != "" {
				t.Fatal("both stale drafts were saved")
			}
			winner = result.model
		case http.StatusConflict:
			conflicts++
		default:
			t.Fatalf("unexpected update status %d", result.status)
		}
	}
	status, saved := doJSON(t, "GET", url, nil)
	if status != http.StatusOK || conflicts != 1 || winner == "" || saved["model"] != winner {
		t.Fatalf("concurrent updates lost the winning choice: conflicts=%d winner=%q saved=%v", conflicts, winner, saved)
	}
}
