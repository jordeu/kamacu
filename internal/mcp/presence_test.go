package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// kamacuPresenceSnapshot is GET /api/presence verbatim for a user viewing a
// task while typing into its agent session (the presence.Snapshot wire
// shape — snake_case, omitempty zero fields).
const kamacuPresenceSnapshot = `{"active":true,"away_for_seconds":0,"last_active_at":"2026-09-25T05:30:00Z","viewing":{"client_id":"tab-1","route":"/projects/3/tasks/7","project_id":3,"task_id":7,"session_id":"5f0d9c2e-64b3-4d0a-9a1e-8b2c7d3e9a01"},"clients":[{"client_id":"tab-1","route":"/projects/3/tasks/7","project_id":3,"task_id":7,"session_id":"5f0d9c2e-64b3-4d0a-9a1e-8b2c7d3e9a01","visible":true,"last_seen":"2026-09-25T05:30:00Z"}],"typing_in":["5f0d9c2e-64b3-4d0a-9a1e-8b2c7d3e9a01"]}`

// TestBridge_GetUserActivity_PassesThroughSnapshot: the bridge leg is a
// verbatim GET /api/presence passthrough carrying the token — the parity
// pattern every resource file locks.
func TestBridge_GetUserActivity_PassesThroughSnapshot(t *testing.T) {
	var (
		gotHeader string
		gotPath   string
		gotMethod string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Kamacu-Token")
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(kamacuPresenceSnapshot))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	res, err := b.getUserActivity(context.Background(), newCallToolRequest(nil))
	if err != nil {
		t.Fatalf("getUserActivity: %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/presence" {
		t.Errorf("request = %s %s, want GET /api/presence", gotMethod, gotPath)
	}
	if gotHeader != "t" {
		t.Errorf("X-Kamacu-Token = %q, want %q (sent on every bridge call)", gotHeader, "t")
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("result.Content[0]: want *TextContent, got %T", res.Content[0])
	}
	if strings.TrimSpace(tc.Text) != kamacuPresenceSnapshot {
		t.Errorf("snapshot not verbatim:\n got %s\nwant %s", tc.Text, kamacuPresenceSnapshot)
	}
	var snap map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &snap); err != nil {
		t.Fatalf("decoded snapshot JSON: %v", err)
	}
	if snap["active"] != true {
		t.Errorf("active = %v, want true", snap["active"])
	}
}

// TestGetUserActivity_RegisteredCallable: registerTools exposes
// get_user_activity by name and an end-to-end CallTool roundtrip (in-memory
// transport, real SDK client) returns the snapshot — locks the registration
// wiring, not just the bridge method.
func TestGetUserActivity_RegisteredCallable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(kamacuPresenceSnapshot))
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	t1, t2 := mcpsdk.NewInMemoryTransports()
	srvr := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "kamacu-test", Version: "test"}, nil)
	registerTools(srvr, &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}})
	serverSession, err := srvr.Connect(ctx, t1, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close() //nolint:errcheck

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "kamacu-test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close() //nolint:errcheck

	res, callErr := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_user_activity",
		Arguments: map[string]any{},
	})
	if callErr != nil {
		t.Fatalf("CallTool get_user_activity: %v", callErr)
	}
	if res.IsError {
		t.Fatalf("CallTool get_user_activity: IsError=true; content=%v", res.Content)
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("result.Content[0]: want *TextContent, got %T", res.Content[0])
	}
	if !strings.Contains(tc.Text, `"typing_in"`) {
		t.Errorf("result missing typing_in:\n%s", tc.Text)
	}
}
