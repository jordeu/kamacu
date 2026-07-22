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

// TestBridge_ListTasks_NoProjectID_CallsUnscopedEndpoint (MCPTASK-01): when
// no arguments are supplied (or project_id is omitted), the bridge calls
// GET /api/tasks — the D-01 unscoped manual-only endpoint added in Plan 01.
func TestBridge_ListTasks_NoProjectID_CallsUnscopedEndpoint(t *testing.T) {
	var (
		gotPath   string
		gotMethod string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	if _, err := b.listTasks(context.Background(), newCallToolRequest(nil)); err != nil {
		t.Fatalf("listTasks (no args): %v", err)
	}
	if gotPath != "/api/tasks" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/tasks", gotPath)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("request method: want %q, got %q", http.MethodGet, gotMethod)
	}
}

// TestBridge_ListTasks_WithProjectID_CallsScopedEndpoint (MCPTASK-01): when
// project_id is supplied, the bridge calls GET /api/projects/{id}/tasks —
// the existing board fetch route.
func TestBridge_ListTasks_WithProjectID_CallsScopedEndpoint(t *testing.T) {
	var (
		gotPath   string
		gotMethod string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"project_id":5}`))
	if _, err := b.listTasks(context.Background(), req); err != nil {
		t.Fatalf("listTasks with project_id=5: %v", err)
	}
	if gotPath != "/api/projects/5/tasks" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/projects/5/tasks", gotPath)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("request method: want %q, got %q", http.MethodGet, gotMethod)
	}
}

// TestBridge_ListTasks_Kamacu500_ReturnsError: a non-2xx Kamacu response
// surfaces as a wrapped error with the status code and body inlined (D-05).
// The assertion matches on the "HTTP 500" status-code substring, NOT on a
// JSON key name (07-RESEARCH Gap 2 / Pitfall 2: the real Kamacu error body
// shape is keyed under "error", not under status/message keys).
func TestBridge_ListTasks_Kamacu500_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"db down"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	_, err := b.listTasks(context.Background(), newCallToolRequest(nil))
	if err == nil {
		t.Fatal("listTasks: expected error for non-200, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("listTasks error message: want substring \"HTTP 500\", got %q", err.Error())
	}
}

// TestBridge_GetTask_Happy_CallsCorrectPath (MCPTASK-02): GET /api/tasks/{id}
// is called with the right path and method, and the canned response body is
// passed through verbatim in a single TextContent block.
func TestBridge_GetTask_Happy_CallsCorrectPath(t *testing.T) {
	const canned = `{"id":7,"title":"do thing"}`
	var (
		gotPath   string
		gotMethod string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(canned))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"task_id":7}`))
	res, err := b.getTask(context.Background(), req)
	if err != nil {
		t.Fatalf("getTask: %v", err)
	}
	if gotPath != "/api/tasks/7" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/tasks/7", gotPath)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("request method: want %q, got %q", http.MethodGet, gotMethod)
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("result.Content[0]: want *TextContent, got %T", res.Content[0])
	}
	if tc.Text != canned {
		t.Errorf("passthrough text: want %q, got %q", canned, tc.Text)
	}
}

// TestBridge_GetTask_NotFound404_ReturnsError (MCPTASK-02): Kamacu's 404 with
// {"error":"task not found"} surfaces as a wrapped error containing "HTTP 404".
func TestBridge_GetTask_NotFound404_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"task not found"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"task_id":9999}`))
	_, err := b.getTask(context.Background(), req)
	if err == nil {
		t.Fatal("getTask: expected error for 404, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("getTask error message: want substring \"HTTP 404\", got %q", err.Error())
	}
}

// TestBridge_CreateTask_Happy_SendsTitleDescriptionBody (MCPTASK-03): POST
// /api/projects/{id}/tasks is called with the right path, method, and a body
// decoding to {"title":..., "description":...}.
func TestBridge_CreateTask_Happy_SendsTitleDescriptionBody(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"project_id":3,"title":"foo","description":"bar"}`))
	if _, err := b.createTask(context.Background(), req); err != nil {
		t.Fatalf("createTask: %v", err)
	}
	if gotPath != "/api/projects/3/tasks" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/projects/3/tasks", gotPath)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("request method: want %q, got %q", http.MethodPost, gotMethod)
	}
	if gotBody["title"] != "foo" {
		t.Errorf("body title: want %q, got %v", "foo", gotBody["title"])
	}
	if gotBody["description"] != "bar" {
		t.Errorf("body description: want %q, got %v", "bar", gotBody["description"])
	}
}

// TestBridge_CreateTask_Kamacu400_TitleRequired_ReturnsError (MCPTASK-03):
// Kamacu's 400 with {"error":"title is required"} surfaces as a wrapped error
// containing "HTTP 400".
func TestBridge_CreateTask_Kamacu400_TitleRequired_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"title is required"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"project_id":3,"title":"","description":""}`))
	_, err := b.createTask(context.Background(), req)
	if err == nil {
		t.Fatal("createTask: expected error for 400, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("createTask error message: want substring \"HTTP 400\", got %q", err.Error())
	}
}

// TestBridge_UpdateTask_PartialPATCH_SendsOnlySuppliedFields (MCPTASK-04,
// D-03): when only title is supplied (description omitted), the PATCH body
// contains ONLY the title key — description is absent. Pointer fields
// distinguish nil (omitted) from "" (clear).
func TestBridge_UpdateTask_PartialPATCH_SendsOnlySuppliedFields(t *testing.T) {
	var (
		gotMethod string
		gotBody   map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":9}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	// description OMITTED — partial-PATCH must not include it
	req := newCallToolRequest(json.RawMessage(`{"task_id":9,"title":"new"}`))
	if _, err := b.updateTask(context.Background(), req); err != nil {
		t.Fatalf("updateTask partial: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("request method: want %q, got %q", http.MethodPatch, gotMethod)
	}
	if gotBody["title"] != "new" {
		t.Errorf("body title: want %q, got %v", "new", gotBody["title"])
	}
	if _, ok := gotBody["description"]; ok {
		t.Errorf("body must NOT contain description key (D-03 partial-PATCH), got %v", gotBody)
	}
}

// TestBridge_UpdateTask_NoFields_KamacuReturnsCurrentRow (MCPTASK-04): when
// neither title nor description is supplied, the PATCH body is {} and Kamacu
// returns the current row (the underlying handler short-circuits to get).
// The canned 200 response is passed through verbatim.
func TestBridge_UpdateTask_NoFields_KamacuReturnsCurrentRow(t *testing.T) {
	var (
		gotMethod string
		gotBody   map[string]any
	)
	const canned = `{"id":9,"title":"current"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(canned))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"task_id":9}`))
	res, err := b.updateTask(context.Background(), req)
	if err != nil {
		t.Fatalf("updateTask no-fields: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("request method: want %q, got %q", http.MethodPatch, gotMethod)
	}
	if len(gotBody) != 0 {
		t.Errorf("body: want empty {}, got %v", gotBody)
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("result.Content[0]: want *TextContent, got %T", res.Content[0])
	}
	if tc.Text != canned {
		t.Errorf("passthrough text: want %q, got %q", canned, tc.Text)
	}
}

// TestBridge_MoveTask_HardcodedAfterIDNull (MCPTASK-05, D-06): the move_task
// handler hardcodes after_id: nil in the body (the MCP tool exposes NO
// after_id arg). The request goes to POST /api/tasks/{id}/move with a body
// decoding to {"status":..., "after_id":null}.
func TestBridge_MoveTask_HardcodedAfterIDNull(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"id":4,"status":"done"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"task_id":4,"status":"done"}`))
	if _, err := b.moveTask(context.Background(), req); err != nil {
		t.Fatalf("moveTask: %v", err)
	}
	if gotPath != "/api/tasks/4/move" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/tasks/4/move", gotPath)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("request method: want %q, got %q", http.MethodPost, gotMethod)
	}
	if gotBody["status"] != "done" {
		t.Errorf("body status: want %q, got %v", "done", gotBody["status"])
	}
	// D-06: after_id must be present and explicitly null (NOT omitted — the
	// handler must marshal {"after_id": nil} so Kamacu's null-after_id
	// top-of-column path is exercised).
	v, ok := gotBody["after_id"]
	if !ok {
		t.Fatalf("body MUST contain after_id key (D-06 hardcodes it to nil), got %v", gotBody)
	}
	if v != nil {
		t.Errorf("body after_id: want nil, got %v", v)
	}
}

// TestBridge_MoveTask_InvalidStatus_Kamacu400 (MCPTASK-05): Kamacu's 400 with
// {"error":"invalid status"} surfaces as a wrapped error containing "HTTP 400".
func TestBridge_MoveTask_InvalidStatus_Kamacu400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid status"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"task_id":4,"status":"bogus"}`))
	_, err := b.moveTask(context.Background(), req)
	if err == nil {
		t.Fatal("moveTask: expected error for 400, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("moveTask error message: want substring \"HTTP 400\", got %q", err.Error())
	}
}

// TestBridge_DeleteTask_Happy_CallsDeletePath (MCPTASK-06): DELETE
// /api/tasks/{id} is called with the right path and method.
func TestBridge_DeleteTask_Happy_CallsDeletePath(t *testing.T) {
	var (
		gotPath   string
		gotMethod string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"task_id":2}`))
	if _, err := b.deleteTask(context.Background(), req); err != nil {
		t.Fatalf("deleteTask: %v", err)
	}
	if gotPath != "/api/tasks/2" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/tasks/2", gotPath)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("request method: want %q, got %q", http.MethodDelete, gotMethod)
	}
}

// TestBridge_DeleteTask_NotFound404_ReturnsError (MCPTASK-06): Kamacu's 404
// with {"error":"task not found"} surfaces as a wrapped error containing
// "HTTP 404".
func TestBridge_DeleteTask_NotFound404_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"task not found"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"task_id":9999}`))
	_, err := b.deleteTask(context.Background(), req)
	if err == nil {
		t.Fatal("deleteTask: expected error for 404, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("deleteTask error message: want substring \"HTTP 404\", got %q", err.Error())
	}
}
