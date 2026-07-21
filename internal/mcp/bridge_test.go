package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestBridge_ListProjects_PassesThroughKamacuJSON stands up a Kamacu-shaped
// HTTP test server returning a canned []Project JSON body, points a *bridge
// at it, and asserts listProjects returns the body verbatim in a single
// TextContent block. Also asserts the X-Kamacu-Token header is set on the
// request (D-06/D-07 wire contract).
func TestBridge_ListProjects_PassesThroughKamacuJSON(t *testing.T) {
	const canned = `[{"id":1,"name":"alpha","repo_path":"/a","description":"","github_repo":null,"managed":false,"icon_letters":"A","icon_color":"#1","workspace_id":1,"agent_id":1,"created_at":"","updated_at":""},{"id":2,"name":"beta","repo_path":"/b","description":"","github_repo":null,"managed":false,"icon_letters":"B","icon_color":"#2","workspace_id":1,"agent_id":1,"created_at":"","updated_at":""}]`

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
		_, _ = w.Write([]byte(canned))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "test-token", client: &http.Client{Timeout: 10 * time.Second}}
	res, err := b.listProjects(context.Background())
	if err != nil {
		t.Fatalf("listProjects: %v", err)
	}
	if len(res.Content) == 0 {
		t.Fatal("listProjects result.Content: empty")
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("listProjects result.Content[0]: want *TextContent, got %T", res.Content[0])
	}
	if tc.Text != canned {
		t.Errorf("listProjects passthrough:\n want: %s\n  got: %s", canned, tc.Text)
	}
	// Wire contract (D-06/D-07): X-Kamacu-Token sent on every bridge call.
	if gotHeader != "test-token" {
		t.Errorf("bridge request X-Kamacu-Token: want %q, got %q", "test-token", gotHeader)
	}
	if gotPath != "/api/projects" {
		t.Errorf("bridge request path: want %q, got %q", "/api/projects", gotPath)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("bridge request method: want %q, got %q", http.MethodGet, gotMethod)
	}
}

// TestBridge_ListProjects_KamacuReturnsNon200_ReturnsError proves a non-200
// Kamacu response surfaces as a handler error (which the SDK wraps as a
// JSON-RPC error.code = -32603 response). The error message carries the
// status code and body so the LLM can self-correct.
func TestBridge_ListProjects_KamacuReturnsNon200_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"db down"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	_, err := b.listProjects(context.Background())
	if err == nil {
		t.Fatal("listProjects: expected error for non-200, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("listProjects error message: want substring \"HTTP 500\", got %q", err.Error())
	}
}

// TestBridge_ListProjects_KamacuUnreachable_ReturnsError proves a transport
// error (Kamacu down, port refused) surfaces as a wrapped handler error. The
// bridge points at 127.0.0.1:1 (always-refused port) to keep the test
// deterministic without depending on a specific Kamacu instance being up.
func TestBridge_ListProjects_KamacuUnreachable_ReturnsError(t *testing.T) {
	b := &bridge{base: "http://127.0.0.1:1", token: "t", client: &http.Client{Timeout: 2 * time.Second}}
	_, err := b.listProjects(context.Background())
	if err == nil {
		t.Fatal("listProjects: expected transport error for unreachable Kamacu, got nil")
	}
	if !strings.Contains(err.Error(), "kamacu bridge:") {
		t.Errorf("listProjects error message: want substring \"kamacu bridge:\", got %q", err.Error())
	}
}

// TestNewBridgeFromEnv_DefaultsAndOverride proves MCPPROC-03 / SC4: when
// KAMACU_HOOK_BASE is unset, the bridge defaults to http://127.0.0.1:7333;
// when set, the bridge uses that URL. t.Setenv handles auto-cleanup.
func TestNewBridgeFromEnv_DefaultsAndOverride(t *testing.T) {
	// Default
	t.Setenv("KAMACU_HOOK_BASE", "")
	b, err := newBridgeFromEnv()
	if err != nil {
		t.Fatalf("newBridgeFromEnv default: %v", err)
	}
	if b.base != defaultBase {
		t.Errorf("default base: want %q, got %q", defaultBase, b.base)
	}

	// Override
	t.Setenv("KAMACU_HOOK_BASE", "http://example")
	b, err = newBridgeFromEnv()
	if err != nil {
		t.Fatalf("newBridgeFromEnv override: %v", err)
	}
	if b.base != "http://example" {
		t.Errorf("override base: want %q, got %q", "http://example", b.base)
	}

	// Trailing slash trimmed
	t.Setenv("KAMACU_HOOK_BASE", "http://example/")
	b, err = newBridgeFromEnv()
	if err != nil {
		t.Fatalf("newBridgeFromEnv trailing slash: %v", err)
	}
	if b.base != "http://example" {
		t.Errorf("trim trailing slash: want %q, got %q", "http://example", b.base)
	}
}
