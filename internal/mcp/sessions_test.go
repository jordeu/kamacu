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

// TestBridge_ListSessions_NoArgs_HitsBareEndpoint (MCPSESS-01): when no
// arguments are supplied (or both project_id and task_id are omitted), the
// bridge calls GET /api/sessions with NO query string.
func TestBridge_ListSessions_NoArgs_HitsBareEndpoint(t *testing.T) {
	var (
		gotPath string
		gotRaw  string
		gotMeth string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotRaw = r.URL.RawQuery
		gotMeth = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	if _, err := b.listSessions(context.Background(), newCallToolRequest(nil)); err != nil {
		t.Fatalf("listSessions (no args): %v", err)
	}
	if gotPath != "/api/sessions" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/sessions", gotPath)
	}
	if gotRaw != "" {
		t.Errorf("request URL.RawQuery: want empty (no args → no query), got %q", gotRaw)
	}
	if gotMeth != http.MethodGet {
		t.Errorf("request method: want %q, got %q", http.MethodGet, gotMeth)
	}
}

// TestBridge_ListSessions_ProjectID_HitsFilteredEndpoint (MCPSESS-01): when
// project_id is supplied, the bridge calls GET /api/sessions?project_id=N.
func TestBridge_ListSessions_ProjectID_HitsFilteredEndpoint(t *testing.T) {
	var (
		gotPath string
		gotRaw  string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotRaw = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"project_id":5}`))
	if _, err := b.listSessions(context.Background(), req); err != nil {
		t.Fatalf("listSessions with project_id=5: %v", err)
	}
	if gotPath != "/api/sessions" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/sessions", gotPath)
	}
	if gotRaw != "project_id=5" {
		t.Errorf("request URL.RawQuery: want %q, got %q", "project_id=5", gotRaw)
	}
}

// TestBridge_ListSessions_TaskID_HitsFilteredEndpoint (MCPSESS-01): when only
// task_id is supplied, the bridge calls GET /api/sessions?task_id=N. Project_id
// takes precedence when both are supplied; this test confirms task_id works
// when project_id is absent.
func TestBridge_ListSessions_TaskID_HitsFilteredEndpoint(t *testing.T) {
	var gotRaw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRaw = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"task_id":9}`))
	if _, err := b.listSessions(context.Background(), req); err != nil {
		t.Fatalf("listSessions with task_id=9: %v", err)
	}
	if gotRaw != "task_id=9" {
		t.Errorf("request URL.RawQuery: want %q, got %q", "task_id=9", gotRaw)
	}
}

// TestBridge_ListSessions_FiltersOrphanedRows (D-13, MCPSESS-01): the Kamacu
// list still includes orphaned restored-tmux survivor rows (id="" +
// orphaned:true + tmuxName=...) for the SPA's reattach affordance. The bridge
// must filter them out so every id the agent sees is operable. This is the
// only behavior unique to list_sessions vs the Phase 07 listTasks template.
func TestBridge_ListSessions_FiltersOrphanedRows(t *testing.T) {
	// Kamacu returns two rows: an orphaned restored-tmux survivor (filtered)
	// and a live running session (kept).
	const kamacuBody = `[{"id":"","orphaned":true,"tmuxName":"kamacu-1-1","label":"bash"},{"id":"b","status":"running","label":"agent"}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(kamacuBody))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	res, err := b.listSessions(context.Background(), newCallToolRequest(nil))
	if err != nil {
		t.Fatalf("listSessions: %v", err)
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("result.Content[0]: want *TextContent, got %T", res.Content[0])
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &got); err != nil {
		t.Fatalf("decoded filtered JSON: %v\nraw=%q", err, tc.Text)
	}
	if len(got) != 1 {
		t.Fatalf("orphaned filter: want 1 surviving row, got %d (rows=%v)", len(got), got)
	}
	if got[0]["id"] != "b" {
		t.Errorf("surviving row id: want %q, got %v", "b", got[0]["id"])
	}
	if _, present := got[0]["orphaned"]; present {
		t.Errorf("surviving row must NOT carry orphaned field, got %v", got[0])
	}
}

// TestBridge_GetSession_Happy_PassesThroughJSON (MCPSESS-02): GET
// /api/sessions/{id} is called with the right path and method, and the canned
// response body is passed through verbatim in a single TextContent block.
func TestBridge_GetSession_Happy_PassesThroughJSON(t *testing.T) {
	const canned = `{"id":"xyz","status":"running","taskTitle":"t","projectName":"p","agentName":"a"}`
	var (
		gotPath string
		gotMeth string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMeth = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(canned))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"session_id":"xyz"}`))
	res, err := b.getSession(context.Background(), req)
	if err != nil {
		t.Fatalf("getSession: %v", err)
	}
	if gotPath != "/api/sessions/xyz" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/sessions/xyz", gotPath)
	}
	if gotMeth != http.MethodGet {
		t.Errorf("request method: want %q, got %q", http.MethodGet, gotMeth)
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("result.Content[0]: want *TextContent, got %T", res.Content[0])
	}
	if tc.Text != canned {
		t.Errorf("passthrough text: want %q, got %q", canned, tc.Text)
	}
}

// TestBridge_GetSession_NotFound404_ReturnsError (MCPSESS-02): Kamacu's 404
// with {"error":"session not found"} surfaces through bridge.call's non-2xx
// wrap as a handler error containing "HTTP 404" (Phase 07 D-05).
func TestBridge_GetSession_NotFound404_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"session not found"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"session_id":"nope"}`))
	_, err := b.getSession(context.Background(), req)
	if err == nil {
		t.Fatal("getSession: expected error for 404, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("getSession error message: want substring \"HTTP 404\", got %q", err.Error())
	}
}

// TestBridge_GetSessionOutput_WithBytes_HitsOutputEndpoint (MCPSESS-03): GET
// /api/sessions/{id}/output?bytes=N is called with the right path and query,
// and the D-08 base64 envelope is passed through verbatim. The Kamacu handler
// (Plan 01) does the clamping server-side.
func TestBridge_GetSessionOutput_WithBytes_HitsOutputEndpoint(t *testing.T) {
	const canned = `{"encoding":"base64","output":"AA==","bytes":1,"clamped":false}`
	var (
		gotPath string
		gotRaw  string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotRaw = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(canned))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"session_id":"xyz","bytes":8192}`))
	res, err := b.getSessionOutput(context.Background(), req)
	if err != nil {
		t.Fatalf("getSessionOutput: %v", err)
	}
	if gotPath != "/api/sessions/xyz/output" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/sessions/xyz/output", gotPath)
	}
	if gotRaw != "bytes=8192" {
		t.Errorf("request URL.RawQuery: want %q, got %q", "bytes=8192", gotRaw)
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("result.Content[0]: want *TextContent, got %T", res.Content[0])
	}
	if tc.Text != canned {
		t.Errorf("passthrough text: want %q, got %q", canned, tc.Text)
	}
}

// TestBridge_GetSessionOutput_NoBytes_HitsOutputEndpointNoQuery (MCPSESS-03):
// when bytes is omitted, the bridge sends GET /api/sessions/{id}/output with
// no query string — Kamacu's default (4096, clamped to 512KiB) applies.
func TestBridge_GetSessionOutput_NoBytes_HitsOutputEndpointNoQuery(t *testing.T) {
	var (
		gotPath string
		gotRaw  string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotRaw = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"encoding":"base64","output":"","bytes":0,"clamped":false}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"session_id":"abc"}`))
	if _, err := b.getSessionOutput(context.Background(), req); err != nil {
		t.Fatalf("getSessionOutput: %v", err)
	}
	if gotPath != "/api/sessions/abc/output" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/sessions/abc/output", gotPath)
	}
	if gotRaw != "" {
		t.Errorf("request URL.RawQuery: want empty (no bytes → no query), got %q", gotRaw)
	}
}

// TestWithRecover_ConvertsPanicToError closes Phase 06 Open Question 1 /
// Pitfall 2: the SDK's tools/call dispatch (server.go:753) calls st.handler
// directly with NO recover, so a panicking handler would unwind the stack and
// desync the stdio JSON-RPC stream. withRecover catches the panic and turns
// it into a returned non-nil error whose message contains "panic" — the SDK
// wraps that as JSON-RPC error.code = -32603 (the clean error path proven by
// TestSC2 in server_test.go).
//
// The unit contract: result MUST be nil and err MUST be non-nil with both
// "panic" and the recovered value ("boom") in the message.
func TestWithRecover_ConvertsPanicToError(t *testing.T) {
	called := false
	wrapped := withRecover("test_tool", func(_ context.Context, _ *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		called = true
		panic("boom")
	})
	result, err := wrapped(context.Background(), newCallToolRequest(nil))
	if !called {
		t.Fatal("withRecover: wrapped handler was never invoked")
	}
	if err == nil {
		t.Fatal("withRecover: expected non-nil error from panic, got nil")
	}
	if !strings.Contains(err.Error(), "panic") {
		t.Errorf("withRecover error message: want substring \"panic\", got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("withRecover error message: want substring \"boom\", got %q", err.Error())
	}
	if result != nil {
		t.Errorf("withRecover result: want nil, got %+v", result)
	}
}

// TestWithRecover_PassesThroughCleanResult proves withRecover is transparent
// for non-panicking handlers: a handler returning a normal CallToolResult +
// nil error is returned unchanged (no spurious mutation, no swallowed
// content). This guards against the wrapper accidentally mutating the happy
// path.
func TestWithRecover_PassesThroughCleanResult(t *testing.T) {
	want := &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "ok"}},
	}
	wrapped := withRecover("clean_tool", func(_ context.Context, _ *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		return want, nil
	})
	got, err := wrapped(context.Background(), newCallToolRequest(nil))
	if err != nil {
		t.Fatalf("withRecover clean: expected nil err, got %v", err)
	}
	if got != want {
		t.Errorf("withRecover clean: result pointer identity should be preserved, got %p want %p", got, want)
	}
	tc, ok := got.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("withRecover clean: result.Content[0]: want *TextContent, got %T", got.Content[0])
	}
	if tc.Text != "ok" {
		t.Errorf("withRecover clean: text content: want %q, got %q", "ok", tc.Text)
	}
}

// TestWithRecover_PassesThroughHandlerError proves a handler returning a
// non-nil error (not panic) is propagated unchanged — withRecover adds nothing
// to the error path. A Kamacu 404 surfaces through bridge.call's wrap and
// withRecover must not swallow or re-wrap it.
func TestWithRecover_PassesThroughHandlerError(t *testing.T) {
	wrapped := withRecover("err_tool", func(_ context.Context, _ *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		return nil, HTTP404Sentinel{}
	})
	_, err := wrapped(context.Background(), newCallToolRequest(nil))
	if err == nil {
		t.Fatal("withRecover err: expected non-nil err, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("withRecover err: want substring \"HTTP 404\", got %q", err.Error())
	}
}

// HTTP404Sentinel is a tiny error type used by TestWithRecover_PassesThroughHandlerError
// to prove withRecover propagates a returned error verbatim (does not re-wrap).
type HTTP404Sentinel struct{}

func (HTTP404Sentinel) Error() string { return "kamacu get /api/sessions/x: HTTP 404: not found" }
