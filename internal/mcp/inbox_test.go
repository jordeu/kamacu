package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestFeedbackToolIdentityAndTransport(t *testing.T) {
	paths := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Kamacu-Token") != "secret" || (r.Header.Get("X-Kamacu-Task-ID") != "42" && r.Header.Get("X-Kamacu-Task-ID") != "0") || r.Header.Get("X-Kamacu-Session-ID") != "session" {
			t.Error("environment identity not forwarded")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("invalid request body: %v", err)
		}
		paths <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"outcome":"pending"}`))
	}))
	defer server.Close()
	b := &bridge{base: server.URL, token: "secret", taskID: 42, sessionID: "session", client: &http.Client{Timeout: time.Second}}
	ctx := context.Background()
	a, z := sdk.NewInMemoryTransports()
	srv := sdk.NewServer(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	registerInboxTools(srv, b)
	ss, err := srv.Connect(ctx, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(ctx, z, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	list, err := cs.ListTools(ctx, nil)
	if err != nil || len(list.Tools) != 6 {
		t.Fatalf("tools: %+v %v", list, err)
	}
	for _, tool := range list.Tools {
		if tool.Name == "wait_for_feedback" {
			t.Fatal("polling tool still advertised")
		}
	}
	r, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "acknowledge_feedback", Arguments: map[string]any{"question_id": "q", "revision": 1}})
	if err != nil || r.IsError {
		t.Fatalf("wait: %+v %v", r, err)
	}
	if path := <-paths; path != "/api/agent-feedback/acknowledge" {
		t.Fatal(path)
	}
	badResult, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "ask_feedback", Arguments: map[string]any{"task_id": 99, "request_key": "k", "summary": "s", "context": "c", "question": "q"}})
	if err == nil && (badResult == nil || !badResult.IsError) {
		t.Fatal("model supplied identity was accepted")
	}
	r, err = cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_feedback"})
	if err != nil || r.IsError {
		t.Fatalf("empty list args: %+v %v", r, err)
	}
	if path := <-paths; path != "/api/agent-feedback/list" {
		t.Fatal(path)
	}
	b.taskID = 0
	r, err = cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_feedback"})
	if err != nil || r.IsError {
		t.Fatalf("Scratchpad: %+v %v", r, err)
	}
	if path := <-paths; path != "/api/agent-feedback/list" {
		t.Fatal(path)
	}
	b.taskID = -1
	_, err = cs.CallTool(ctx, &sdk.CallToolParams{Name: "list_feedback", Arguments: map[string]any{}})
	if err == nil || !strings.Contains(err.Error(), "KAMACU_TASK_ID") {
		t.Fatalf("missing identity: %v", err)
	}
}
func TestFeedbackIdentityEnvironment(t *testing.T) {
	t.Setenv("KAMACU_TASK_ID", "123")
	t.Setenv("KAMACU_SESSION_ID", "s")
	b, err := newBridgeFromEnv()
	if err != nil || b.taskID != 123 || b.sessionID != "s" {
		t.Fatalf("%+v %v", b, err)
	}
}

func TestScratchpadFeedbackIdentityEnvironment(t *testing.T) {
	for _, raw := range []string{"0", "", "invalid", "-1"} {
		t.Setenv("KAMACU_TASK_ID", raw)
		t.Setenv("KAMACU_SESSION_ID", "scratch")
		id, session := feedbackIdentityFromEnv()
		if session != "scratch" || (raw == "0" && id != 0) || (raw != "0" && id >= 0) {
			t.Fatalf("%q: %d %s", raw, id, session)
		}
	}
}
