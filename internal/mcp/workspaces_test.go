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

// TestBridge_ListWorkspaces_Happy_CallsGetPath (MCPPROJ-06): GET /api/workspaces
// is called with the right path and method, and the canned JSON array body
// (empty array when no workspaces exist — Kamacu never returns nil) is passed
// through verbatim in a single TextContent block.
func TestBridge_ListWorkspaces_Happy_CallsGetPath(t *testing.T) {
	const canned = `[]`
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
	// list_workspaces takes no args — pass nil to mirror the SDK zero value.
	res, err := b.listWorkspaces(context.Background(), newCallToolRequest(nil))
	if err != nil {
		t.Fatalf("listWorkspaces: %v", err)
	}
	if gotPath != "/api/workspaces" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/workspaces", gotPath)
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

// TestBridge_CreateWorkspace_Happy_SendsNameBody (MCPPROJ-06): POST
// /api/workspaces is called with the right path and method, and the body
// decodes to {"name":"Research"}.
func TestBridge_CreateWorkspace_Happy_SendsNameBody(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"id":1,"name":"Research"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"name":"Research"}`))
	if _, err := b.createWorkspace(context.Background(), req); err != nil {
		t.Fatalf("createWorkspace: %v", err)
	}
	if gotPath != "/api/workspaces" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/workspaces", gotPath)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("request method: want %q, got %q", http.MethodPost, gotMethod)
	}
	if gotBody["name"] != "Research" {
		t.Errorf("body name: want %q, got %v", "Research", gotBody["name"])
	}
}

// TestBridge_CreateWorkspace_DuplicateName_Kamacu409 (MCPPROJ-06): Kamacu's 409
// for a case-insensitive duplicate name surfaces as a wrapped error containing
// the "HTTP 409" status-code substring. The assertion matches on the status
// substring, NOT on a JSON key name (07-RESEARCH Gap 2 / Pitfall 2: the real
// Kamacu error body shape is keyed under "error", not under status/message keys).
func TestBridge_CreateWorkspace_DuplicateName_Kamacu409(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"a workspace with that name already exists"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"name":"Personal"}`))
	_, err := b.createWorkspace(context.Background(), req)
	if err == nil {
		t.Fatal("createWorkspace: expected error for 409, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 409") {
		t.Errorf("createWorkspace error: want substring \"HTTP 409\", got %q", err.Error())
	}
}

// TestBridge_UpdateWorkspace_Happy_SendsNameBody (MCPPROJ-06, D-03): when
// workspace_id and name are both supplied, PATCH /api/workspaces/{id} is
// called with the right path and method, and the body decodes to
// {"name":"Renamed"}.
func TestBridge_UpdateWorkspace_Happy_SendsNameBody(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"id":2,"name":"Renamed"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"workspace_id":2,"name":"Renamed"}`))
	if _, err := b.updateWorkspace(context.Background(), req); err != nil {
		t.Fatalf("updateWorkspace: %v", err)
	}
	if gotPath != "/api/workspaces/2" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/workspaces/2", gotPath)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("request method: want %q, got %q", http.MethodPatch, gotMethod)
	}
	if gotBody["name"] != "Renamed" {
		t.Errorf("body name: want %q, got %v", "Renamed", gotBody["name"])
	}
}

// TestBridge_UpdateWorkspace_NoName_EmptyBody_Kamacu400 (MCPPROJ-06, D-03):
// when name is OMITTED (nil pointer), the PATCH body is the empty object {} —
// no name key. Kamacu's PATCH handler rejects this with 400 "nothing to
// update". The bridge does NOT add a name key when the arg is absent (D-03
// pointer-field semantics).
func TestBridge_UpdateWorkspace_NoName_EmptyBody_Kamacu400(t *testing.T) {
	var (
		gotMethod string
		gotBody   map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"nothing to update"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	// name OMITTED — only workspace_id supplied.
	req := newCallToolRequest(json.RawMessage(`{"workspace_id":2}`))
	_, err := b.updateWorkspace(context.Background(), req)
	if err == nil {
		t.Fatal("updateWorkspace: expected error for 400, got nil")
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("request method: want %q, got %q", http.MethodPatch, gotMethod)
	}
	// Body MUST be {} — no name key when the pointer is nil.
	if _, ok := gotBody["name"]; ok {
		t.Errorf("body must NOT contain name key when name is omitted (D-03 nil pointer), got %v", gotBody)
	}
	if !strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("updateWorkspace error: want substring \"HTTP 400\", got %q", err.Error())
	}
}

// TestBridge_DeleteWorkspace_Happy_CallsDeletePath (MCPPROJ-06): DELETE
// /api/workspaces/{id} is called with the right path and method. Kamacu
// answers 204 on success (empty non-default workspace removal).
func TestBridge_DeleteWorkspace_Happy_CallsDeletePath(t *testing.T) {
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
	req := newCallToolRequest(json.RawMessage(`{"workspace_id":3}`))
	if _, err := b.deleteWorkspace(context.Background(), req); err != nil {
		t.Fatalf("deleteWorkspace: %v", err)
	}
	if gotPath != "/api/workspaces/3" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/workspaces/3", gotPath)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("request method: want %q, got %q", http.MethodDelete, gotMethod)
	}
}

// TestBridge_DeleteWorkspace_DefaultGuard_Kamacu409 (MCPPROJ-06, v1.9 guarded
// delete — is_default branch): Kamacu's 409 for the is_default workspace
// surfaces as a wrapped error containing the "HTTP 409" status-code substring
// AND the Kamacu message substring. The assertion matches on the Kamacu
// message text ("the default workspace") — NOT on a JSON key name (07-RESEARCH
// Gap 2: the real Kamacu error body shape is keyed under "error", not under
// status/message keys).
func TestBridge_DeleteWorkspace_DefaultGuard_Kamacu409(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		// Real Kamacu shape (internal/api/workspaces.go:210) — keyed on "error".
		_, _ = w.Write([]byte(`{"error":"the default workspace can't be deleted"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"workspace_id":1}`))
	_, err := b.deleteWorkspace(context.Background(), req)
	if err == nil {
		t.Fatal("deleteWorkspace: expected error for default-guard 409, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 409") {
		t.Errorf("deleteWorkspace error: want substring \"HTTP 409\", got %q", err.Error())
	}
	// Gap 2: assert on the Kamacu message substring, NOT on a JSON key name.
	// The raw body is inlined in the D-05 wrap, so the message text survives.
	if !strings.Contains(err.Error(), "the default workspace") {
		t.Errorf("deleteWorkspace error: want substring \"the default workspace\" (Kamacu message), got %q", err.Error())
	}
}

// TestBridge_DeleteWorkspace_NonEmptyGuard_Kamacu409 (MCPPROJ-06, v1.9 guarded
// delete — non-empty COUNT branch): Kamacu's 409 for a workspace still owning
// projects surfaces as a wrapped error containing the "HTTP 409" status-code
// substring AND the Kamacu message substring. Same Gap 2 assertion pattern as
// the default-guard case above.
func TestBridge_DeleteWorkspace_NonEmptyGuard_Kamacu409(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		// Real Kamacu shape (internal/api/workspaces.go:220) — keyed on "error"
		// with a count interpolated into the message.
		_, _ = w.Write([]byte(`{"error":"move or remove its 3 project(s) first"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"workspace_id":2}`))
	_, err := b.deleteWorkspace(context.Background(), req)
	if err == nil {
		t.Fatal("deleteWorkspace: expected error for non-empty-guard 409, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 409") {
		t.Errorf("deleteWorkspace error: want substring \"HTTP 409\", got %q", err.Error())
	}
	// Gap 2: assert on the Kamacu message substring, NOT on a JSON key name.
	if !strings.Contains(err.Error(), "project(s) first") {
		t.Errorf("deleteWorkspace error: want substring \"project(s) first\" (Kamacu message), got %q", err.Error())
	}
}

// TestBridge_MoveProjectToWorkspace_Happy_PATCHesProjectRoute (MCPPROJ-07):
// PATCH /api/projects/{id} is called (NOT a /api/workspaces route) with the
// right path and method, and the body decodes to {"workspace_id":2} with NO
// other keys. This proves the transfer tool reuses the v1.9 PATCH /api/projects
// workspace_id branch — the bridge does NOT call any /api/workspaces route.
func TestBridge_MoveProjectToWorkspace_Happy_PATCHesProjectRoute(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"id":5,"workspace_id":2}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"project_id":5,"workspace_id":2}`))
	if _, err := b.moveProjectToWorkspace(context.Background(), req); err != nil {
		t.Fatalf("moveProjectToWorkspace: %v", err)
	}
	// CRITICAL: the path is /api/projects/{id}, NOT /api/workspaces/...
	if gotPath != "/api/projects/5" {
		t.Errorf("request URL.Path: want %q (PATCH /api/projects/{id} — v1.9 transfer surface), got %q", "/api/projects/5", gotPath)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("request method: want %q, got %q", http.MethodPatch, gotMethod)
	}
	// Body MUST contain ONLY workspace_id — no name, no description, etc.
	v, ok := gotBody["workspace_id"]
	if !ok {
		t.Fatalf("body MUST contain workspace_id, got %v", gotBody)
	}
	f, ok := v.(float64) // JSON numbers decode as float64
	if !ok {
		t.Fatalf("body workspace_id: want numeric, got %T %v", v, v)
	}
	if f != 2.0 {
		t.Errorf("body workspace_id: want 2, got %v", f)
	}
	// No other keys allowed — this is the dedicated transfer tool, body
	// contains ONLY workspace_id (D-03 excludes workspace_id from
	// update_project specifically because this tool owns the transfer).
	for _, absent := range []string{"name", "description", "github_repo", "icon_letters", "icon_color", "agent_id", "repo", "repo_path"} {
		if _, ok := gotBody[absent]; ok {
			t.Errorf("body must NOT contain %q key (transfer tool body is {workspace_id}-only), got %v", absent, gotBody)
		}
	}
}

// TestBridge_MoveProjectToWorkspace_TargetMissing_Kamacu400 (MCPPROJ-07):
// Kamacu's 400 "workspace not found" (the v1.9 PATCH /api/projects/{id}
// WorkspaceID branch validates target existence via
// `SELECT 1 FROM workspaces WHERE id = ?` → 400 if missing) surfaces as a
// wrapped error containing the "HTTP 400" status-code substring AND the Kamacu
// message substring. Same Gap 2 assertion pattern.
func TestBridge_MoveProjectToWorkspace_TargetMissing_Kamacu400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		// Real Kamacu shape (internal/api/projects.go:606) — keyed on "error".
		_, _ = w.Write([]byte(`{"error":"workspace not found"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"project_id":5,"workspace_id":9999}`))
	_, err := b.moveProjectToWorkspace(context.Background(), req)
	if err == nil {
		t.Fatal("moveProjectToWorkspace: expected error for target-missing 400, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("moveProjectToWorkspace error: want substring \"HTTP 400\", got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "workspace not found") {
		t.Errorf("moveProjectToWorkspace error: want substring \"workspace not found\" (Kamacu message), got %q", err.Error())
	}
}
