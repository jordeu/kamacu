package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
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

// ============================================================================
// subscribe_session_output (MCPSESS-04) — the SC3 streaming panic surface.
// Tests below cover the happy path, duration clamp, non-2xx error wrap,
// truncation cap, the central SC3 cancellation gate (in-memory SDK transport
// + fake streaming httptest.Server), the SC3 goroutine-leak gate across N
// cycles, and the D-01 strict-edge cancel-before-attach.
// ============================================================================

// subscribeEnvelope is the test-side mirror of internal subscribeEnvelope.
// It is re-declared here only for documentation; the production struct is
// unexported and these tests decode into a local anonymous struct instead.
// (This comment exists so a reader doesn't go looking for it.)

// decodeSubscribeEnvelope pulls the D-08 envelope JSON out of a CallToolResult's
// first TextContent block. Fatals on shape mismatch so each test can proceed
// with field assertions.
func decodeSubscribeEnvelope(t *testing.T, res *mcpsdk.CallToolResult) map[string]any {
	t.Helper()
	if res == nil {
		t.Fatal("decodeSubscribeEnvelope: nil result")
	}
	if len(res.Content) == 0 {
		t.Fatal("decodeSubscribeEnvelope: empty Content")
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("decodeSubscribeEnvelope: Content[0]: want *TextContent, got %T", res.Content[0])
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &env); err != nil {
		t.Fatalf("decodeSubscribeEnvelope: unmarshal %v\nraw=%q", err, tc.Text)
	}
	return env
}

// TestBridge_Subscribe_Happy_ReturnsEnvelope (MCPSESS-04): a short duration
// subscribe returns a D-08 envelope carrying the streamed bytes. The fake
// server writes "hello" then closes the stream; the follow-up GET reports
// status running. Asserts encoding/base64 shape, output decodes back to
// "hello", exited=false, truncated=false.
func TestBridge_Subscribe_Happy_ReturnsEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sessions/xyz/subscribe":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("hello"))
		case "/api/sessions/xyz":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"running"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"session_id":"xyz","duration_seconds":1}`))
	res, err := b.subscribeSessionOutput(context.Background(), req)
	if err != nil {
		t.Fatalf("subscribeSessionOutput: %v", err)
	}
	env := decodeSubscribeEnvelope(t, res)
	if got := env["encoding"]; got != "base64" {
		t.Errorf("encoding: want %q, got %v", "base64", got)
	}
	outputStr, _ := env["output"].(string)
	decoded, derr := base64.StdEncoding.DecodeString(outputStr)
	if derr != nil {
		t.Fatalf("base64 decode output: %v (raw=%q)", derr, outputStr)
	}
	if !bytes.Contains(decoded, []byte("hello")) {
		t.Errorf("decoded output: want substring %q, got %q", "hello", string(decoded))
	}
	if got := env["truncated"]; got != false {
		t.Errorf("truncated: want false, got %v", got)
	}
	if got := env["exited"]; got != false {
		t.Errorf("exited: want false, got %v", got)
	}
}

// TestBridge_Subscribe_DurationClamp_QueryReflects300 (T-08-15): a
// duration_seconds of 999 is clamped to 300 before the request is built —
// the recorded subscribe query string is duration_seconds=300.
func TestBridge_Subscribe_DurationClamp_QueryReflects300(t *testing.T) {
	var recordedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/sessions/xyz/subscribe" {
			recordedQuery = r.URL.RawQuery
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"running"}`))
		}
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"session_id":"xyz","duration_seconds":999}`))
	if _, err := b.subscribeSessionOutput(context.Background(), req); err != nil {
		t.Fatalf("subscribeSessionOutput: %v", err)
	}
	wantQuery := "duration_seconds=300&include_history=false"
	if recordedQuery != wantQuery {
		t.Errorf("subscribe query: want %q, got %q", wantQuery, recordedQuery)
	}
}

// TestBridge_Subscribe_Non200_ReturnsWrappedError (D-05 error wrap): a
// non-200 streaming response surfaces the Phase 07 D-05 wrap containing
// "HTTP 500".
func TestBridge_Subscribe_Non200_ReturnsWrappedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"session_id":"xyz","duration_seconds":1}`))
	_, err := b.subscribeSessionOutput(context.Background(), req)
	if err == nil {
		t.Fatal("subscribeSessionOutput: expected error for HTTP 500, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("error message: want substring %q, got %q", "HTTP 500", err.Error())
	}
}

// TestBridge_Subscribe_TruncationCap (D-04 / T-08-11): the fake server streams
// 1.5 MiB of 'x' then closes — exceeding subscribeTailCap (1 MiB). Asserts
// truncated==true AND bytes <= subscribeTailCap (the buffer kept the most
// recent 1 MiB, dropping the older half from the front).
func TestBridge_Subscribe_TruncationCap(t *testing.T) {
	// 1.5 MiB of 'x' — comfortably above the 1 MiB cap.
	oversized := bytes.Repeat([]byte("x"), subscribeTailCap+(1<<19))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sessions/big/subscribe":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(oversized)
		case "/api/sessions/big":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"running"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"session_id":"big","duration_seconds":1}`))
	res, err := b.subscribeSessionOutput(context.Background(), req)
	if err != nil {
		t.Fatalf("subscribeSessionOutput: %v", err)
	}
	env := decodeSubscribeEnvelope(t, res)
	if got := env["truncated"]; got != true {
		t.Errorf("truncated: want true (stream exceeded 1 MiB cap), got %v", got)
	}
	bytesN, _ := env["bytes"].(float64)
	if int(bytesN) > subscribeTailCap {
		t.Errorf("bytes: want <= %d (cap held), got %v", subscribeTailCap, bytesN)
	}
	if int(bytesN) != subscribeTailCap {
		t.Errorf("bytes: want exactly %d (trim-front keeps the most recent cap), got %v", subscribeTailCap, bytesN)
	}
}

// TestSubscribe_CancelledViaContext_ReturnsPartialAndDetaches is the central
// SC3 regression gate. It uses mcp.NewInMemoryTransports to drive the SDK
// end-to-end: client.CallTool(ctx, ...) → SDK sends notifications/cancelled
// on cancel → handler ctx cancelled → in-flight resp.Body.Read returns →
// handler returns the partial buffer.
//
// SDK cancellation surface (verified against go-sdk@v1.6.1's own
// Example_cancellation at mcp_example_test.go:108-164): when the CLIENT's
// ctx is cancelled, CallTool returns (nil, context.Canceled) — the client
// observes the cancellation as an error. The HANDLER on the server side
// still runs to completion and returns its partial result, but the client
// SDK does not surface it. SC3 ("returns the partial stream collected so
// far" — D-01) is a contract on the *handler's* return value, not on what
// CallTool surfaces to the client. So this test wraps
// b.subscribeSessionOutput in a recorder to observe the handler's actual
// return — proving the handler returned a partial result with no panic and
// no hang (the load-bearing SC3 assertion).
//
// The fake streaming httptest.Server writes one chunk ("partial") then blocks
// on r.Context().Done() so it holds the conn until the bridge closes the
// request body. It records that Done fired (proving the cancel propagated
// all the way to the Kamacu-side r.Context() — the MCP half of SC3; Plan 01's
// TestSubscribe_ClientCancel_DetachesPromptly is the Kamacu half).
//
// Asserts:
//   - The handler returns within a generous bound (no hang — T-08-12).
//   - The handler returns a non-nil result with nil err (partial buffer is
//     the SC3 result, not a transport error, not a panic-via-withRecover).
//     Or an emptySubscribeEnvelope if cancel beat the first Read — both are
//     valid SC3 results.
//   - The fake server's request-context Done fired within a bound (proves
//     the bridge closed the HTTP body on ctx cancel — the loopback
//     propagation that triggers Session.Detach in Plan 01).
func TestSubscribe_CancelledViaContext_ReturnsPartialAndDetaches(t *testing.T) {
	// Fake Kamacu streaming server: writes one chunk then blocks until the
	// client disconnects (r.Context().Done()).
	serverCtxDone := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sessions/xyz/subscribe":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("partial"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			// Hold the conn until the client disconnects. r.Context().Done()
			// fires when the bridge closes the request body.
			<-r.Context().Done()
			serverCtxDone <- struct{}{}
		case "/api/sessions/xyz":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"running"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	t1, t2 := mcpsdk.NewInMemoryTransports()

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	mcpSrv := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "kamacu-test", Version: "test"}, nil)

	// Wrap b.subscribeSessionOutput in a recorder so we can observe the
	// handler's actual return value. The client-side CallTool will return
	// (nil, context.Canceled) per the SDK's documented cancellation surface
	// — SC3 is about the handler, not the client. The recorder captures
	// what the handler returned so we can assert "partial result, no error,
	// no panic".
	type handlerReturn struct {
		res *mcpsdk.CallToolResult
		err error
	}
	handlerReturnCh := make(chan handlerReturn, 1)
	mcpSrv.AddTool(
		&mcpsdk.Tool{
			Name:        "subscribe_session_output",
			Description: "test wrapper",
			InputSchema: map[string]any{"type": "object"},
		},
		func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			res, err := b.subscribeSessionOutput(ctx, req)
			handlerReturnCh <- handlerReturn{res, err}
			return res, err
		},
	)
	serverSession, err := mcpSrv.Connect(ctx, t1, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "kamacu-test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	callCtx, cancel := context.WithCancel(ctx)
	go func() {
		// CallTool's ctx-cancel return is the SDK's documented behavior
		// (Example_cancellation). We do not assert on its return — the
		// load-bearing assertion is the handler-side channel below.
		_, _ = clientSession.CallTool(callCtx, &mcpsdk.CallToolParams{
			Name:      "subscribe_session_output",
			Arguments: map[string]any{"session_id": "xyz", "duration_seconds": 60},
		})
	}()

	// Give the stream time to attach + write "partial", then cancel (mimics
	// the agent CLI sending notifications/cancelled).
	time.Sleep(200 * time.Millisecond)
	cancel()

	// SC3 handler-side: the handler must return within a generous bound (no
	// hang — T-08-12). It must return a non-nil partial result with nil err
	// (the partial buffer is the result; not a transport error, not a panic-
	// via-withRecover which would surface as non-nil err).
	select {
	case r := <-handlerReturnCh:
		if r.err != nil {
			t.Fatalf("handler returned err=%v — want nil (partial result, not a transport error; not a panic)", r.err)
		}
		if r.res == nil {
			t.Fatal("handler returned nil result — want non-nil partial envelope")
		}
		// The envelope is either the partial buffer (output contains
		// "partial") OR emptySubscribeEnvelope (cancel beat the first Read).
		// Both are valid SC3 results. Decode and log which one we got.
		env := decodeSubscribeEnvelope(t, r.res)
		if got := env["encoding"]; got != "base64" {
			t.Errorf("envelope.encoding: want %q, got %v", "base64", got)
		}
		if outputStr, _ := env["output"].(string); outputStr != "" {
			decoded, _ := base64.StdEncoding.DecodeString(outputStr)
			t.Logf("handler returned partial envelope: %d bytes (decoded=%q)", int(env["bytes"].(float64)), string(decoded))
		} else {
			t.Logf("handler returned empty envelope (cancel arrived before first Read — still valid SC3)")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("handler did not return within 10s of cancel — handler hung (SC3 detach leak / T-08-12)")
	}

	// SC3 detach-proof: the fake server's r.Context().Done() must have fired
	// (the bridge closed the HTTP body when its ctx was cancelled — this is
	// the loopback propagation that triggers Session.Detach in Plan 01).
	select {
	case <-serverCtxDone:
		// pass — detach propagation confirmed
	case <-time.After(2 * time.Second):
		t.Fatal("fake server r.Context().Done() did not fire within 2s — bridge did not close the stream on ctx cancel (SC3 violation)")
	}
}

// TestSubscribe_GoroutineStabilityAcrossCycles is the SC3 leak gate. It runs
// N=5 subscribe/cancel cycles against a blocking fake streaming server and
// asserts runtime.NumGoroutine() does not grow proportional to N. Each cycle:
// fresh cancellable ctx, call b.subscribeSessionOutput in a goroutine, cancel
// after a short sleep, wait for return. A leak (e.g., a reader goroutine that
// survives ctx cancel) would surface as delta >= N. A small delta (+2) is
// allowed for GC/timer goroutines.
func TestSubscribe_GoroutineStabilityAcrossCycles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Streaming handler: write one byte then block on r.Context().Done()
		// (client disconnect). The follow-up GET returns running.
		if r.URL.Path == "/api/sessions/xyz/subscribe" {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("x"))
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"running"}`))
	}))
	t.Cleanup(srv.Close)

	// Force a GC baseline so the count reflects live goroutines, not garbage.
	runtime.GC()
	baseline := runtime.NumGoroutine()
	t.Logf("baseline NumGoroutine = %d", baseline)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	const N = 5
	for i := 0; i < N; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		var (
			res *mcpsdk.CallToolResult
			err error
			wg  sync.WaitGroup
		)
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := newCallToolRequest(json.RawMessage(`{"session_id":"xyz","duration_seconds":60}`))
			res, err = b.subscribeSessionOutput(ctx, req)
		}()
		// Let the stream attach, then cancel.
		time.Sleep(50 * time.Millisecond)
		cancel()
		wg.Wait()
		_ = res // not asserted — the contract is "returns promptly", not "returns a specific shape"
		_ = err
	}

	// Settle: allow any GC/timer goroutines to retire.
	time.Sleep(200 * time.Millisecond)
	runtime.GC()
	after := runtime.NumGoroutine()
	delta := after - baseline
	t.Logf("after %d cycles: NumGoroutine = %d (delta %d)", N, after, delta)
	if delta >= N {
		t.Errorf("goroutine leak: delta=%d (after=%d, baseline=%d, N=%d) — expected no growth proportional to N", delta, after, baseline, N)
	}
}

// TestSubscribe_CancelBeforeAttach_ReturnsEmptyEnvelopeNotError is the D-01
// strict-edge: when the handler ctx is cancelled BEFORE
// subscribeSessionOutput is called (subscribeClient.Do returns
// context.Canceled immediately — the stream never attached), the handler
// returns (emptySubscribeEnvelope, nil) — a valid zero-byte D-08 envelope,
// NOT a transport error. D-01's "returns ONE CallToolResult" holds even when
// zero bytes were streamed.
//
// The fake server is unreachable in practice (the request never succeeds
// because the ctx is already cancelled); we still stand it up so the test
// does not depend on connection-refused vs context-cancelled ordering.
func TestSubscribe_CancelBeforeAttach_ReturnsEmptyEnvelopeNotError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Unreachable in practice — the request ctx is already cancelled.
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel BEFORE calling subscribeSessionOutput

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"session_id":"xyz","duration_seconds":1}`))
	res, err := b.subscribeSessionOutput(ctx, req)

	// D-01 strict-edge: NOT a transport error.
	if err != nil {
		t.Fatalf("subscribeSessionOutput on pre-cancelled ctx: want nil err, got %v", err)
	}
	if res == nil {
		t.Fatal("subscribeSessionOutput on pre-cancelled ctx: want non-nil empty envelope, got nil")
	}
	env := decodeSubscribeEnvelope(t, res)
	if got := env["encoding"]; got != "base64" {
		t.Errorf("envelope.encoding: want %q, got %v", "base64", got)
	}
	if got := env["output"]; got != "" {
		t.Errorf("envelope.output: want empty (zero bytes streamed), got %v", got)
	}
	if got := env["bytes"]; got != float64(0) {
		t.Errorf("envelope.bytes: want 0, got %v", got)
	}
	if got := env["truncated"]; got != false {
		t.Errorf("envelope.truncated: want false, got %v", got)
	}
	if got := env["exited"]; got != false {
		t.Errorf("envelope.exited: want false, got %v", got)
	}
}

// Compile-time guard: the errors package must be referenced by the test file
// so go vet / unused-import checks pass even when only some of the seven
// tests reference it directly. (TestBridge_Subscribe_Non200_ReturnsWrappedError
// uses strings.Contains — kept separate.)
var _ = errors.Is

// ============================================================================
// start_task_agent (MCPSESS-05) — the MCP-only auto-start reversal. Tests
// below cover the happy D-05 passthrough (201 → verbatim body), resume flag
// routing into the POST body, and the dedup 409 wrap.
// ============================================================================

// TestBridge_StartTaskAgent_Happy_PostsAgentBody (MCPSESS-05 happy path,
// D-05 passthrough): POST /api/sessions is called with the right path and
// method, the body decodes to {"kind":"agent","task_id":N,"resume":false},
// and Kamacu's 201 session-row JSON is passed through verbatim in a single
// TextContent block.
func TestBridge_StartTaskAgent_Happy_PostsAgentBody(t *testing.T) {
	const canned = `{"id":"sess-9","taskId":7,"kind":"agent","status":"running"}`
	var (
		gotPath   string
		gotMethod string
		gotBody   map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(canned))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"task_id":7}`))
	res, err := b.startTaskAgent(context.Background(), req)
	if err != nil {
		t.Fatalf("startTaskAgent: %v", err)
	}
	if gotPath != "/api/sessions" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/sessions", gotPath)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("request method: want %q, got %q", http.MethodPost, gotMethod)
	}
	if gotBody["kind"] != "agent" {
		t.Errorf("body kind: want %q, got %v", "agent", gotBody["kind"])
	}
	if gotBody["task_id"] != float64(7) {
		t.Errorf("body task_id: want %v, got %v", float64(7), gotBody["task_id"])
	}
	if gotBody["resume"] != false {
		t.Errorf("body resume: want false, got %v", gotBody["resume"])
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("result.Content[0]: want *TextContent, got %T", res.Content[0])
	}
	if tc.Text != canned {
		t.Errorf("passthrough text: want %q, got %q", canned, tc.Text)
	}
}

// TestBridge_StartTaskAgent_ResumeTrue_SendsResumeBody (MCPSESS-05 resume
// routing): when resume:true is supplied in the tool args, the bool flag is
// marshalled into the POST body — proving the one novel wiring this tool adds
// beyond createTask's POST-with-body shape.
func TestBridge_StartTaskAgent_ResumeTrue_SendsResumeBody(t *testing.T) {
	var (
		gotBody map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"sess-9"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"task_id":7,"resume":true}`))
	if _, err := b.startTaskAgent(context.Background(), req); err != nil {
		t.Fatalf("startTaskAgent: %v", err)
	}
	if gotBody["resume"] != true {
		t.Errorf("body resume: want true, got %v", gotBody["resume"])
	}
}

// TestBridge_StartTaskAgent_AlreadyRunning_Kamacu409 (MCPSESS-05 dedup error
// path, D-05 wrap): Kamacu's 409 with {"error":"agent session already running"}
// (the one-running-agent-per-task dedup, sessions.go:310) surfaces through
// bridge.call's non-2xx wrap as a handler error containing both "HTTP 409"
// and the Kamacu message (Gap 2 — the message survives the wrap).
func TestBridge_StartTaskAgent_AlreadyRunning_Kamacu409(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		// Real Kamacu shape (sessions.go:310) — keyed on "error".
		_, _ = w.Write([]byte(`{"error":"agent session already running"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"task_id":7}`))
	_, err := b.startTaskAgent(context.Background(), req)
	if err == nil {
		t.Fatal("startTaskAgent: expected error for dedup 409, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 409") {
		t.Errorf("startTaskAgent error: want substring \"HTTP 409\", got %q", err.Error())
	}
	// Gap 2: assert on the Kamacu message substring, NOT on a JSON key name.
	// The raw body is inlined in the D-05 wrap, so the message text survives.
	if !strings.Contains(err.Error(), "agent session already running") {
		t.Errorf("startTaskAgent error: want substring \"agent session already running\" (Kamacu message survives D-05 wrap), got %q", err.Error())
	}
}

// TestBridge_SendSessionMessage_Happy_PostsInputPath (MCPSESS-06 happy path,
// D-05 passthrough): POST /api/sessions/{id}/input is called with the right
// path + method, the body decodes to {"message":"run tests"}, and Kamacu's
// 200 {"bytes_written":N} is passed through verbatim in a single TextContent.
func TestBridge_SendSessionMessage_Happy_PostsInputPath(t *testing.T) {
	const canned = `{"bytes_written":14}`
	var (
		gotPath   string
		gotMethod string
		gotBody   map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(canned))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"session_id":"s1","message":"run tests"}`))
	res, err := b.sendSessionMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("sendSessionMessage: %v", err)
	}
	if want := "/api/sessions/s1/input"; gotPath != want {
		t.Errorf("request URL.Path: want %q, got %q", want, gotPath)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("request method: want %q, got %q", http.MethodPost, gotMethod)
	}
	if gotBody["message"] != "run tests" {
		t.Errorf("body message: want %q, got %v", "run tests", gotBody["message"])
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("result.Content[0]: want *TextContent, got %T", res.Content[0])
	}
	if tc.Text != canned {
		t.Errorf("passthrough text: want %q, got %q", canned, tc.Text)
	}
}

// TestBridge_SendSessionMessage_UnknownID_Kamacu404 (MCPSESS-06 error path,
// D-05 wrap): Kamacu's 404 with {"error":"session not found"} surfaces through
// bridge.call's non-2xx wrap as a handler error containing both "HTTP 404" and
// the Kamacu message (Gap 2 — the message survives the wrap).
func TestBridge_SendSessionMessage_UnknownID_Kamacu404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"session not found"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"session_id":"gone","message":"hi"}`))
	_, err := b.sendSessionMessage(context.Background(), req)
	if err == nil {
		t.Fatal("sendSessionMessage: expected error for unknown id 404, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("sendSessionMessage error: want substring \"HTTP 404\", got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "session not found") {
		t.Errorf("sendSessionMessage error: want substring \"session not found\" (Kamacu message survives D-05 wrap), got %q", err.Error())
	}
}

// TestBridge_SendSessionMessage_EmptyMessage_StillPosted: an empty message is
// POSTed verbatim — the bridge does ZERO client-side validation; Kamacu's
// newline normalization handles it server-side. The body posted is
// {"message":""}, proving the empty message reaches Kamacu unchanged.
func TestBridge_SendSessionMessage_EmptyMessage_StillPosted(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"bytes_written":1}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"session_id":"s1","message":""}`))
	if _, err := b.sendSessionMessage(context.Background(), req); err != nil {
		t.Fatalf("sendSessionMessage empty message: %v", err)
	}
	if gotBody["message"] != "" {
		t.Errorf("body message: want \"\" (empty message posted verbatim — bridge does no validation), got %q", gotBody["message"])
	}
}
