package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"kamacu/internal/pi"
	"kamacu/internal/session"
)

func TestPiModelDiscoveryResponse(t *testing.T) {
	input := `{"type":"agent_start"}
{"id":"another","type":"response","command":"get_available_models","success":true,"data":{"models":[]}}
{"id":"kamacu-models","type":"response","command":"get_available_models","success":true,"data":{"models":[{"provider":"one","id":"org/model"},{"provider":"two","id":"org/model"},{"provider":"one","id":"org/model"}]}}`
	models, err := parsePiDiscoveredModels([]byte(input))
	if err != nil || !reflect.DeepEqual(models, []string{"one/org/model", "two/org/model"}) {
		t.Fatalf("models = %v, %v", models, err)
	}
	empty := `{"id":"kamacu-models","type":"response","command":"get_available_models","success":true,"data":{"models":[]}}`
	if models, err := parsePiDiscoveredModels([]byte(empty)); err != nil || models == nil || len(models) != 0 {
		t.Fatalf("empty models = %v, %v", models, err)
	}
	for _, input := range []string{`{"type":"agent_start"}`, `{"id":"kamacu-models","type":"response","command":"get_available_models","success":false}`, "invalid json"} {
		if _, err := parsePiDiscoveredModels([]byte(input)); err == nil {
			t.Errorf("accepted invalid response: %s", input)
		}
	}
}

func TestPiLaunchResumeAndContext(t *testing.T) {
	srv, mgr, db := newAgentIntegrationServer(t)
	folder := t.TempDir()
	argsPath := filepath.Join(folder, "args")
	envPath := filepath.Join(folder, "env")
	contextDir := filepath.Join(folder, "test-context")
	t.Setenv("KAMACU_PROJECT_CONTEXT", "stale")
	t.Setenv("KAMACU_HOOK_TOKEN", "stale-token")
	t.Setenv("PI_TEST_ARGS", argsPath)
	t.Setenv("PI_TEST_ENV", envPath)
	binary := filepath.Join(folder, "pi")
	script := `#!/bin/sh
case "$1" in
--version) printf '1.6.1\n'; exit ;;
--mode)
	read -r request
	[ "$request" = '{"type":"get_available_models","id":"kamacu-models"}' ] || exit 1
	printf '%s\n' '{"id":"kamacu-models","type":"response","command":"get_available_models","success":true,"data":{"models":[{"provider":"fixture","id":"org/model"}]}}'
	exit ;;
esac
printf '%s\n' "$KAMACU_PROJECT_CONTEXT" "$KAMACU_TASK_ID" "$KAMACU_HOOK_TOKEN" > "$PI_TEST_ENV"
printf '%s\n' "$@" > "$PI_TEST_ARGS"
exec /bin/cat
`
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	var agentID int64
	if err := db.QueryRow(`UPDATE agents SET command = ?, models = '["fixture/org/model","fixture/removed"]' WHERE engine = 'pi' RETURNING id`, binary).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	result, catalog := doJSON(t, "POST", srv.URL+"/api/agents/discover-models", map[string]string{"engine": "pi", "command": binary})
	if result != http.StatusOK || !reflect.DeepEqual(catalog["models"], []any{"fixture/org/model"}) {
		t.Fatalf("discover = %d, %v", result, catalog)
	}
	pid := createProject(t, srv, gitRepoWithCommit(t))
	assignProjectToAgent(t, db, pid, agentID)
	task := createTask(t, srv, pid, "Pi task")
	taskID := taskID(t, task)
	cwd := task["worktree_path"].(string)
	mgr.SetAgentConfig(session.AgentConfig{
		Token:        "test-token",
		PiMCPEnabled: func() (bool, error) { return true, nil },
		ProjectContext: func(id int64, engine string) (string, error) {
			if id != pid || engine != "pi" {
				t.Errorf("context resolved for project %d, engine %q", id, engine)
			}
			return contextDir, nil
		},
	})
	status, body := doJSON(t, "PATCH", srv.URL+"/api/tasks/"+itoa(taskID), map[string]string{"model": "fixture/org/model"})
	if status != http.StatusOK {
		t.Fatalf("set model = %d, %v", status, body)
	}
	start := func(resume bool, want int) map[string]any {
		t.Helper()
		status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"kind": "agent", "task_id": taskID, "resume": resume})
		if status != want {
			t.Fatalf("start (resume=%v) = %d, %v; want %d", resume, status, body, want)
		}
		return body
	}
	body = start(false, http.StatusCreated)
	args := readArgv(t, argsPath)
	if len(args) != 6 || args[0] != "--session-dir" || args[2] != "--session-id" ||
		args[4] != "--model" || args[5] != "fixture/org/model" {
		t.Fatalf("fresh Pi argv = %q", args)
	}
	sessionID := strID(body)
	start(false, http.StatusConflict)
	env, err := os.ReadFile(envPath)
	if err != nil || string(env) != contextDir+"\n"+itoa(taskID)+"\ntest-token\n" {
		t.Fatalf("Pi runtime environment = %q, %v", env, err)
	}
	stopAndWaitExited(t, srv, taskID, sessionID)
	start(true, http.StatusConflict)
	// The same path is used by status and resume even after the live entry is removed.
	if err := mgr.Remove(sessionID); err != nil {
		t.Fatal(err)
	}
	id := args[3]
	transcript := filepath.Join(args[1], "2026-01-01T00-00-00.000Z_"+id+".jsonl")
	header, _ := json.Marshal(map[string]any{"type": "session", "version": 3, "id": id, "cwd": cwd})
	if err := os.WriteFile(transcript, append(header, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if entry := agentStatusForTask(t, srv, taskID); entry["resumable"] != true {
		t.Fatalf("saved Pi session isn't resumable: %v", entry)
	}
	if err := os.Remove(argsPath); err != nil {
		t.Fatal(err)
	}
	body = start(true, http.StatusCreated)
	want := []string{"--session-dir", args[1], "--session", id, "--model", "fixture/org/model"}
	if args := readArgv(t, argsPath); !reflect.DeepEqual(args, want) {
		t.Fatalf("resume Pi argv = %q, want %q", args, want)
	}
	stopAndWaitExited(t, srv, taskID, strID(body))
	status, body = doJSON(t, "PATCH", srv.URL+"/api/tasks/"+itoa(taskID), map[string]string{"model": "fixture/removed"})
	if status != http.StatusOK {
		t.Fatalf("set stale model = %d, %v", status, body)
	}
	if body := start(false, http.StatusConflict); !strings.Contains(body["error"].(string), "model is unavailable") {
		t.Fatalf("unexpected stale model error: %v", body)
	}
	var saved string
	if err := db.QueryRow(`SELECT pi_session_id FROM tasks WHERE id = ?`, taskID).Scan(&saved); err != nil || saved != id {
		t.Fatalf("failed reset changed the resume reference to %q: %v", saved, err)
	}
	root, err := pi.SessionRoot(db)
	if err != nil || pi.CanResume(root, taskID, id, t.TempDir()) {
		t.Fatalf("accepted resume in a different working directory: %v", err)
	}
	// A standalone Pi launch still gets task identity. Integration credentials
	// and project context are available only to opted-in sessions.
	mgr.SetAgentConfig(session.AgentConfig{
		Token:        "configured-token",
		PiMCPEnabled: func() (bool, error) { return false, nil },
	})
	status, body = doJSON(t, "PATCH", srv.URL+"/api/tasks/"+itoa(taskID), map[string]string{"model": ""})
	if status != http.StatusOK {
		t.Fatalf("clear model = %d, %v", status, body)
	}
	if err := os.Remove(argsPath); err != nil {
		t.Fatal(err)
	}
	body = start(false, http.StatusCreated)
	readArgv(t, argsPath)
	env, err = os.ReadFile(envPath)
	if err != nil || string(env) != "\n"+itoa(taskID)+"\n\n" {
		t.Fatalf("Pi runtime environment without integration = %q, %v", env, err)
	}
	stopAndWaitExited(t, srv, taskID, strID(body))
}

func TestPiScratchpadResumeAndRootChange(t *testing.T) {
	srv, mgr, db := newAgentIntegrationServer(t)
	binary := filepath.Join(t.TempDir(), "pi")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nif [ \"$1\" = --version ]; then echo 1.6.1; else exec cat; fi\n"), 0700); err != nil {
		t.Fatal(err)
	}
	var agentID int64
	if err := db.QueryRow(`UPDATE agents SET command = ? WHERE engine = 'pi' RETURNING id`, binary).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	mgr.SetAgentConfig(session.AgentConfig{ProjectContext: func(int64, string) (string, error) {
		t.Error("Scratchpad called the project context resolver")
		return "", nil
	}})
	root := gitRepo(t)
	status, body := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"agent_id": agentID, "root_path": root})
	if status != http.StatusOK {
		t.Fatalf("set Scratchpad = %d, %v", status, body)
	}
	start := func(resume bool, want int) {
		t.Helper()
		status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"kind": "agent", "scope": "global", "resume": resume})
		if status != want {
			t.Fatalf("start Scratchpad (resume=%v) = %d, %v; want %d", resume, status, body, want)
		}
		if status == http.StatusCreated {
			sess, ok := mgr.Get(strID(body))
			if !ok {
				t.Fatal("missing Scratchpad session")
			}
			sess.Stop()
			<-sess.Done()
		}
	}
	start(false, http.StatusCreated)
	var id string
	if err := db.QueryRow(`SELECT pi_session_id FROM global_task WHERE id = 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	sessionRoot, err := pi.SessionRoot(db)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(sessionRoot, "scratchpad", id)
	header, _ := json.Marshal(map[string]any{"type": "session", "version": 3, "id": id, "cwd": root})
	if err := os.WriteFile(filepath.Join(dir, "start_"+id+".jsonl"), append(header, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	start(true, http.StatusCreated)
	status, body = doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"root_path": gitRepo(t)})
	if status != http.StatusOK {
		t.Fatalf("change Scratchpad root = %d, %v", status, body)
	}
	start(true, http.StatusConflict)
	var cleared bool
	if err := db.QueryRow(`SELECT pi_session_id IS NULL FROM global_task WHERE id = 1`).Scan(&cleared); err != nil || !cleared {
		t.Fatalf("Scratchpad conversation reference wasn't cleared: %v, %v", cleared, err)
	}
}
