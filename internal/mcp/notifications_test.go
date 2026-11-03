package mcp

import (
	"context"
	"encoding/json"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNotificationMCPAllowlistAndRealBridge(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Kamacu-Token") != "test-token" {
			t.Error("missing token")
		}
		if r.Method != "POST" || r.URL.Path != "/api/sessions/correct/input" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["message"] != "Fix review feedback" {
			t.Errorf("body: %v %v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"bytes_written":20}`))
	}))
	defer srv.Close()
	client, err := NewNotificationClient(context.Background(), srv.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	tools, err := client.List(context.Background())
	if err != nil || len(tools) != 9 {
		t.Fatalf("tools: %d %v", len(tools), err)
	}
	raw, err := client.Call(context.Background(), "send_session_message", json.RawMessage(`{"session_id":"correct","message":"Fix review feedback"}`))
	if err != nil || !strings.Contains(raw, "bytes_written") {
		t.Fatalf("call: %s %v", raw, err)
	}
	if _, err = client.Call(context.Background(), "delete_task", json.RawMessage(`{"task_id":1}`)); err == nil {
		t.Fatal("delete exposed")
	}
	// Verify the private MCP server itself also rejects forbidden calls.
	if _, err = client.client.CallTool(context.Background(), &sdk.CallToolParams{Name: "delete_task", Arguments: map[string]any{"task_id": 1}}); err == nil {
		t.Fatal("server still exposes delete")
	}
	if calls != 1 {
		t.Fatalf("unexpected requests: %d", calls)
	}
}
