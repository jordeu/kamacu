package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"kamacu/internal/notifications"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestNotificationMCPFullCatalogAndRealBridge(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Kamacu-Token") != "test-token" {
			t.Error("missing token")
		}
		switch r.Method + " " + r.URL.Path {
		case "POST /api/sessions/correct/input":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["message"] != "Fix review feedback" {
				t.Errorf("body: %v %v", body, err)
			}
		case "POST /api/projects/2/tasks":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["title"] != "Fix notification" {
				t.Errorf("body: %v %v", body, err)
			}
		case "GET /api/sessions/correct/output":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"encoding": "base64", "output": base64.StdEncoding.EncodeToString([]byte("Review in progress")), "bytes": 18})
			return
		case "DELETE /api/tasks/1":
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
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
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
	}
	for _, name := range []string{"create_task", "delete_task", "move_task", "create_project", "delete_project", "create_workspace", "post_pr_review", "list_artifacts", "get_user_activity", "list_feedback", "send_session_message"} {
		if !names[name] {
			t.Errorf("missing registered tool %s", name)
		}
	}
	raw, err := client.Call(context.Background(), "send_session_message", json.RawMessage(`{"session_id":"correct","message":"Fix review feedback"}`))
	if err != nil || !strings.Contains(raw, "bytes_written") {
		t.Fatalf("call: %s %v", raw, err)
	}
	for _, call := range []struct{ name, args string }{
		{"create_task", `{"project_id":2,"title":"Fix notification"}`},
		{"delete_task", `{"task_id":1}`},
	} {
		if _, err = client.Call(context.Background(), call.name, json.RawMessage(call.args)); err != nil {
			t.Fatal(err)
		}
	}
	terminal, err := client.Call(context.Background(), "get_session_output", json.RawMessage(`{"session_id":"correct"}`))
	if err != nil || !strings.Contains(terminal, "Review in progress") {
		t.Fatalf("terminal: %s %v", terminal, err)
	}
	if _, err = client.Call(context.Background(), "unknown_tool", json.RawMessage(`{}`)); err == nil {
		t.Fatal("unknown tool accepted")
	}
	if _, err = client.client.CallTool(context.Background(), &sdk.CallToolParams{Name: "unknown_tool", Arguments: map[string]any{}}); err == nil {
		t.Fatal("server accepted unknown tool")
	}
	if calls != 4 {
		t.Fatalf("unexpected requests: %d", calls)
	}
}

func TestNotificationTerminalEvidenceIsReadable(t *testing.T) {
	for _, tc := range []struct{ name, output string }{
		{"snapshot", "Reviewing https://github.com/acme/repo/pull/7"},
		{"recent excerpt", strings.Repeat("x", 14000) + "Review still running"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{"encoding": "base64", "output": base64.StdEncoding.EncodeToString([]byte(tc.output)), "bytes": len(tc.output)})
			result, err := readableNotificationTerminal(string(raw))
			if err != nil {
				t.Fatal(err)
			}
			var envelope map[string]any
			if err := json.Unmarshal([]byte(result), &envelope); err != nil {
				t.Fatal(err)
			}
			want := tc.output
			if len(want) > 12000 {
				want = want[len(want)-12000:]
			}
			if envelope["encoding"] != "utf-8" || envelope["output"] != want {
				t.Fatalf("unexpected terminal evidence: %v", envelope)
			}
		})
	}
	if _, err := readableNotificationTerminal(`{"encoding":"base64","output":"invalid!"}`); err == nil {
		t.Fatal("accepted malformed terminal data")
	}
}

func TestNotificationTerminalBudgetPreservesLatestEvidence(t *testing.T) {
	const latest = "Review https://github.com/acme/repo/pull/7 is still active"
	for _, prefix := range []string{
		strings.Repeat("\x1b[32mx\x1b[0m", 1000),
		strings.Repeat("\x1b", 12000),
		strings.Repeat("\x1b[32m日本語\x1b[0m", 1000),
	} {
		raw, _ := json.Marshal(map[string]any{"encoding": "base64", "output": base64.StdEncoding.EncodeToString([]byte(prefix + latest))})
		result, err := readableNotificationTerminal(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		if len(result) > notifications.ToolResultLimit || !json.Valid([]byte(result)) {
			t.Fatalf("terminal envelope exceeds downstream budget: %d", len(result))
		}
		var envelope struct {
			Output    string `json:"output"`
			Truncated bool   `json:"notification_excerpt_truncated"`
		}
		if err := json.Unmarshal([]byte(result), &envelope); err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(envelope.Output, latest) || !utf8.ValidString(envelope.Output) || !envelope.Truncated {
			t.Fatalf("latest terminal evidence lost or invalid: %+v", envelope)
		}
	}
}
