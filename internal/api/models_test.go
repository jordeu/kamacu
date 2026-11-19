package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestModelArguments(t *testing.T) {
	for _, tc := range []struct {
		name, engine, template string
		args, want             []string
	}{
		{"option delimiter", "custom", "--model {{model}}", []string{"--verbose", "--", "prompt", "--model", "literal"}, []string{"--verbose", "--model", "chosen model;$(literal)", "--", "prompt", "--model", "literal"}},
		{"missing value before delimiter", "custom", "--model {{model}}", []string{"--model", "--", "literal"}, []string{"--model", "chosen model;$(literal)", "--", "literal"}},
		{"codex resume", "codex", "--model {{model}}", []string{"--model", "old", "resume", "session", "-m=other"}, []string{"resume", "session", "--model", "chosen model;$(literal)"}},
		{"custom config", "custom", "-c model={{model}}", []string{"-c", "other=keep", "-c", "model=old"}, []string{"-c", "other=keep", "-c", "model=chosen model;$(literal)"}},
		{"custom equals", "custom", "--llm={{model}}", []string{"--llm=old", "--verbose"}, []string{"--verbose", "--llm=chosen model;$(literal)"}},
		{"claude extras", "claude", "--model {{model}}", []string{"--model=old", "--verbose"}, []string{"--verbose", "--model", "chosen model;$(literal)"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := applyModelArgument(tc.args, tc.template, "chosen model;$(literal)", tc.engine); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("args = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestModelDiscoveryFormats(t *testing.T) {
	for _, tc := range []struct {
		name, output string
		want         []string
	}{
		{"codex", `{"models":[{"slug":"listed","visibility":"list"},{"slug":"hidden","visibility":"hide"}]}`, []string{"listed"}},
		{"claude", "{\"type\":\"control_response\",\"response\":{\"response\":{\"models\":[{\"value\":\"opus\"},{\"value\":\"sonnet\"}]}}}\n", []string{"opus", "sonnet"}},
		{"opencode", "anthropic/sonnet\nopenai/codex\n", []string{"anthropic/sonnet", "openai/codex"}},
		{"custom", `["one", "two", "one"]`, []string{"one", "two"}},
		{"context suffix", "sonnet[1m]\nprovider/model[1m]", []string{"sonnet[1m]", "provider/model[1m]"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseDiscoveredModels([]byte(tc.output))
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("models = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
	for _, output := range []string{"", "[]", "{bad", "[1,2]", "this is CLI help", "null", "true", "false"} {
		if models, err := parseDiscoveredModels([]byte(output)); err == nil {
			t.Fatalf("accepted invalid catalog %q: %v", output, models)
		}
	}
}

func TestCuratedModelConfigurationAndDiscovery(t *testing.T) {
	srv, db, _ := newTestServer(t)
	status, agent := doJSON(t, "POST", srv.URL+"/api/agents", map[string]any{
		"name": "Model agent", "command": "my-agent", "model_argument": "--llm={{model}}", "models": []string{" first ", "second", "first"},
	})
	if status != http.StatusCreated {
		t.Fatalf("create = %d: %v", status, agent)
	}
	id := int64(agent["id"].(float64))
	if got := agent["models"]; !reflect.DeepEqual(got, []any{"first", "second"}) {
		t.Fatalf("models = %v", got)
	}
	status, _ = doJSON(t, "PATCH", srv.URL+"/api/agents/"+itoa(id), map[string]any{"model_argument": "--llm"})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid argument status = %d", status)
	}
	status, agent = doJSON(t, "PATCH", srv.URL+"/api/agents/"+itoa(id), map[string]any{"models": []string{"second"}})
	if status != http.StatusOK || !reflect.DeepEqual(agent["models"], []any{"second"}) {
		t.Fatalf("remove = %d: %v", status, agent)
	}
	// A catalog process returns suggestions, without changing the curated row.
	command := filepath.Join(t.TempDir(), "discover")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nprintf '[\"suggested\",\"second\"]'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	status, result := doJSON(t, "POST", srv.URL+"/api/agents/discover-models", map[string]any{"engine": "custom", "command": "my-agent", "model_discovery_command": command})
	if status != http.StatusOK || !reflect.DeepEqual(result["models"], []any{"suggested", "second"}) {
		t.Fatalf("discover = %d: %v", status, result)
	}
	var stored string
	if err := db.QueryRow(`SELECT models FROM agents WHERE id=?`, id).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != `["second"]` {
		t.Fatalf("discovery changed configuration: %s", stored)
	}
	status, _ = doJSON(t, "PATCH", srv.URL+"/api/agents/"+itoa(id), map[string]any{"models": []string{}})
	if status != http.StatusOK {
		t.Fatalf("clear models status = %d", status)
	}
}

func TestTaskAgentModelOverrideLaunch(t *testing.T) {
	for _, engine := range []string{"claude", "codex", "opencode", "custom"} {
		t.Run(engine, func(t *testing.T) {
			srv, _, db, agentID, argsFile := newCodexResumeServer(t)
			t.Setenv("FAKE_CLAUDE_ARGS_FILE", argsFile)
			if _, err := db.Exec(`UPDATE agents SET engine=?, models='["chosen"]', model_argument='--model {{model}}', command=command || ' --model old' WHERE id=?`, engine, agentID); err != nil {
				t.Fatal(err)
			}
			pid := createProject(t, srv, gitRepoWithCommit(t)) // Project remains on Claude.
			status, task := doJSON(t, "POST", srv.URL+"/api/projects/"+itoa(pid)+"/tasks", map[string]any{"title": "Chosen model", "agent_id": agentID, "model": "chosen"})
			if status != http.StatusCreated || task["agent_id"] != float64(agentID) || task["model"] != "chosen" {
				t.Fatalf("create task = %d: %v", status, task)
			}
			tid := taskID(t, task)
			status, result := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "agent"})
			if status != http.StatusCreated {
				t.Fatalf("start = %d: %v", status, result)
			}
			args := readArgv(t, argsFile)
			if !strings.Contains(strings.Join(args, "|"), "--model|chosen") || strings.Contains(strings.Join(args, "|"), "|old") {
				t.Fatalf("spawn args = %q", args)
			}
			// Persisted task choices survive reads and do not revalidate against
			// today's list: removing a suggestion must not break an existing task.
			if _, err := db.Exec(`UPDATE agents SET models='[]' WHERE id=?`, agentID); err != nil {
				t.Fatal(err)
			}
			status, loaded := doJSON(t, "GET", srv.URL+"/api/tasks/"+itoa(tid), nil)
			if status != http.StatusOK || loaded["model"] != "chosen" {
				t.Fatalf("reload = %d: %v", status, loaded)
			}
			if engine == "codex" {
				stopAndWaitExited(t, srv, tid, result["id"].(string))
				setTaskCodexSession(t, db, tid, "12345678-1234-1234-1234-123456789012")
				if err := os.Remove(argsFile); err != nil {
					t.Fatal(err)
				}
				status, result = doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "agent", "resume": true})
				if status != http.StatusCreated {
					t.Fatalf("resume = %d: %v", status, result)
				}
				args = readArgv(t, argsFile)
				if !reflect.DeepEqual(args, []string{"--no-alt-screen", "resume", "12345678-1234-1234-1234-123456789012", "--model", "chosen"}) {
					t.Fatalf("resume args = %q", args)
				}
			}
		})
	}
}

func TestTaskModelValidationAndAgentDeletion(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	status, agent := doJSON(t, "POST", srv.URL+"/api/agents", map[string]any{"name": "Task only", "command": "my-agent", "model_argument": "-m {{model}}", "models": []string{"allowed"}})
	if status != http.StatusCreated {
		t.Fatal(status, agent)
	}
	id := int64(agent["id"].(float64))
	for _, body := range []map[string]any{
		{"title": "Unknown agent", "agent_id": 999999},
		{"title": "Unknown model", "agent_id": id, "model": "other"},
		{"title": "Wrong agent model", "model": "allowed"},
	} {
		status, result := doJSON(t, "POST", srv.URL+"/api/projects/"+itoa(pid)+"/tasks", body)
		if status != http.StatusBadRequest {
			t.Fatalf("invalid selection = %d: %v", status, result)
		}
	}
	status, task := doJSON(t, "POST", srv.URL+"/api/projects/"+itoa(pid)+"/tasks", map[string]any{"title": "Pinned", "agent_id": id, "model": "allowed"})
	if status != http.StatusCreated {
		t.Fatal(status, task)
	}
	status, result := doJSON(t, "DELETE", srv.URL+"/api/agents/"+itoa(id), nil)
	if status != http.StatusConflict {
		t.Fatalf("delete referenced agent = %d: %v", status, result)
	}
	// Older callers still inherit the project default.
	legacy := createTask(t, srv, pid, "Inherited")
	if legacy["agent_id"] != nil || legacy["model"] != "" {
		t.Fatalf("legacy task = %v", legacy)
	}
	if _, err := db.Exec(`UPDATE agents SET models='["project-model"]' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	status, task = doJSON(t, "POST", srv.URL+"/api/projects/"+itoa(pid)+"/tasks", map[string]any{"title": "Implicit agent", "model": "project-model"})
	if status != http.StatusCreated || task["agent_id"] != float64(1) {
		b, _ := json.Marshal(task)
		t.Fatalf("model should pin project agent: %d: %s", status, b)
	}
}

func TestBuiltinModelDiscovery(t *testing.T) {
	srv, _, _ := newTestServer(t)
	for _, engine := range []string{"claude", "codex", "opencode"} {
		t.Run(engine, func(t *testing.T) {
			dir := t.TempDir()
			script := filepath.Join(dir, "catalog")
			argv := filepath.Join(dir, "args")
			input := filepath.Join(dir, "input")
			payload := ""
			switch engine {
			case "claude":
				payload = `{"type":"control_response","response":{"response":{"models":[{"value":"chosen"}]}}}`
			case "codex":
				payload = `{"models":[{"slug":"chosen","visibility":"list"}]}`
			case "opencode":
				payload = "chosen"
			}
			source := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %q\ncat > %q\nprintf '%%s' '%s'\n", argv, input, payload)
			if err := os.WriteFile(script, []byte(source), 0700); err != nil {
				t.Fatal(err)
			}
			status, result := doJSON(t, "POST", srv.URL+"/api/agents/discover-models", map[string]any{"engine": engine, "command": script})
			if status != http.StatusOK || !reflect.DeepEqual(result["models"], []any{"chosen"}) {
				t.Fatalf("discover = %d: %v", status, result)
			}
			args, err := os.ReadFile(argv)
			if err != nil {
				t.Fatal(err)
			}
			if engine == "codex" && string(args) != "debug\nmodels\n" {
				t.Fatalf("args = %q", args)
			}
			if engine == "opencode" && string(args) != "models\n" {
				t.Fatalf("args = %q", args)
			}
			if engine == "claude" {
				data, err := os.ReadFile(input)
				if err != nil {
					t.Fatal(err)
				}
				var request map[string]any
				if err := json.Unmarshal(data, &request); err != nil {
					t.Fatal(err)
				}
				if request["type"] != "control_request" || !strings.Contains(string(data), `"subtype":"initialize"`) || strings.Contains(string(data), `"type":"user"`) {
					t.Fatalf("unexpected input: %s", data)
				}
			}
		})
	}
}

func TestDiscoveryRequiresExplicitOverrideForLaunchArguments(t *testing.T) {
	srv, _, _ := newTestServer(t)
	for _, engine := range []string{"claude", "codex", "opencode"} {
		status, result := doJSON(t, "POST", srv.URL+"/api/agents/discover-models", map[string]any{"engine": engine, "command": "my-agent --provider special"})
		if status != http.StatusBadRequest || !strings.Contains(result["error"].(string), "discovery command") {
			t.Fatalf("%s discovery = %d: %v", engine, status, result)
		}
	}
}

func TestDiscoveryDoesNotInheritHookCredentials(t *testing.T) {
	srv, _, _ := newTestServer(t)
	t.Setenv("KAMACU_HOOK_TOKEN", "private-parent-token")
	t.Setenv("CLAUDECODE", "1")
	script := filepath.Join(t.TempDir(), "catalog")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nif [ -n \"${KAMACU_HOOK_TOKEN+x}${CLAUDECODE+x}\" ]; then exit 1; fi\nprintf '[\"safe\"]'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	status, result := doJSON(t, "POST", srv.URL+"/api/agents/discover-models", map[string]any{"engine": "claude", "command": "claude --settings custom.json", "model_discovery_command": script})
	if status != http.StatusOK || !reflect.DeepEqual(result["models"], []any{"safe"}) {
		t.Fatalf("discovery = %d: %v", status, result)
	}
}

func TestDiscoveryCancellationStopsChildProcesses(t *testing.T) {
	dir := t.TempDir()
	script, ready, marker := filepath.Join(dir, "catalog"), filepath.Join(dir, "ready"), filepath.Join(dir, "orphaned")
	source := fmt.Sprintf("#!/bin/sh\n(sleep 0.3; touch %q) &\nprintf ready > %q\nwait\n", marker, ready)
	if err := os.WriteFile(script, []byte(source), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body, _ := json.Marshal(map[string]any{"engine": "custom", "model_discovery_command": script})
	request := httptest.NewRequest(http.MethodPost, "/api/agents/discover-models", strings.NewReader(string(body))).WithContext(ctx)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { (&agentCRUDHandlers{}).discoverModels(response, request); close(done) }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("discovery did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled discovery did not exit")
	}
	if response.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d", response.Code)
	}
	time.Sleep(400 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("cancelled discovery left a child process running")
	}
}

func TestTaskModelCorruptionIsServerError(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	if _, err := db.Exec(`UPDATE agents SET models='invalid json' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	status, result := doJSON(t, "POST", srv.URL+"/api/projects/"+itoa(pid)+"/tasks", map[string]any{"title": "Model selection", "model": "sonnet"})
	if status != http.StatusInternalServerError {
		t.Fatalf("corrupt configuration = %d: %v", status, result)
	}
}
