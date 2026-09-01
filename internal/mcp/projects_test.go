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

// TestBridge_GetProject_Happy_CallsCorrectPath (MCPPROJ-02): GET
// /api/projects/{id} is called with the right path and method, and the canned
// response body is passed through verbatim in a single TextContent block.
func TestBridge_GetProject_Happy_CallsCorrectPath(t *testing.T) {
	const canned = `{"id":3,"name":"alpha"}`
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
	req := newCallToolRequest(json.RawMessage(`{"project_id":3}`))
	res, err := b.getProject(context.Background(), req)
	if err != nil {
		t.Fatalf("getProject: %v", err)
	}
	if gotPath != "/api/projects/3" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/projects/3", gotPath)
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

// TestBridge_GetProject_NotFound404_ReturnsError (MCPPROJ-02): Kamacu's 404
// with {"error":"project not found"} surfaces as a wrapped error containing
// the "HTTP 404" status-code substring. The assertion matches on the status
// substring, NOT on a JSON key name (07-RESEARCH Gap 2 / Pitfall 2: the real
// Kamacu error body shape is keyed under "error", not under status/message keys).
func TestBridge_GetProject_NotFound404_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"project not found"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"project_id":9999}`))
	_, err := b.getProject(context.Background(), req)
	if err == nil {
		t.Fatal("getProject: expected error for 404, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("getProject error: want substring \"HTTP 404\", got %q", err.Error())
	}
}

// TestBridge_CreateProject_FolderPath_SendsAllFieldsAsIs (MCPPROJ-03, D-04):
// when repo_path is supplied and repo is empty, POST /api/projects is called
// with a body that decodes to a map containing the name, repo_path, AND repo
// keys (all four sent — even when some are empty strings). Kamacu's create
// handler dispatches on `strings.TrimSpace(req.Repo) != ""`; the bridge does
// ZERO type detection (no `if args.Repo != ""` branch).
func TestBridge_CreateProject_FolderPath_SendsAllFieldsAsIs(t *testing.T) {
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
	req := newCallToolRequest(json.RawMessage(`{"name":"foo","repo_path":"/abs/path"}`))
	if _, err := b.createProject(context.Background(), req); err != nil {
		t.Fatalf("createProject folder: %v", err)
	}
	if gotPath != "/api/projects" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/projects", gotPath)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("request method: want %q, got %q", http.MethodPost, gotMethod)
	}
	if gotBody["name"] != "foo" {
		t.Errorf("body name: want %q, got %v", "foo", gotBody["name"])
	}
	if gotBody["repo_path"] != "/abs/path" {
		t.Errorf("body repo_path: want %q, got %v", "/abs/path", gotBody["repo_path"])
	}
	// D-04: all four fields sent — even the empty `repo` string. Kamacu's
	// `strings.TrimSpace(req.Repo) != ""` check is what dispatches the
	// folder-vs-repo fork; the bridge sends `repo` as-is.
	if gotBody["repo"] != "" {
		t.Errorf("body repo: want empty string (sent as-is, Kamacu dispatches on it), got %v", gotBody["repo"])
	}
	// workspace_id key must be ABSENT when omitted (Kamacu falls back to
	// default Personal workspace via its *int64 decode).
	if _, ok := gotBody["workspace_id"]; ok {
		t.Errorf("body must NOT contain workspace_id when omitted (Kamacu falls back to default), got %v", gotBody)
	}
}

// TestBridge_CreateProject_ManagedRepo_SendsAllFieldsAsIs (MCPPROJ-03, D-04):
// when repo is supplied (owner/name) and repo_path is empty, the body still
// contains the repo field verbatim — the field Kamacu's create handler forks
// on. The bridge does NOT branch; it sends all fields as-is.
func TestBridge_CreateProject_ManagedRepo_SendsAllFieldsAsIs(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"name":"foo","repo":"owner/name"}`))
	if _, err := b.createProject(context.Background(), req); err != nil {
		t.Fatalf("createProject managed: %v", err)
	}
	if gotBody["repo"] != "owner/name" {
		t.Errorf("body repo: want %q (the field Kamacu dispatches on), got %v", "owner/name", gotBody["repo"])
	}
	// repo_path is still present (empty string) — all four fields sent.
	if gotBody["repo_path"] != "" {
		t.Errorf("body repo_path: want empty string (sent as-is), got %v", gotBody["repo_path"])
	}
	if gotBody["name"] != "foo" {
		t.Errorf("body name: want %q, got %v", "foo", gotBody["name"])
	}
}

// TestBridge_CreateProject_WorkspaceIDIncluded_WhenSupplied (MCPPROJ-03):
// when workspace_id is supplied, the body INCLUDES workspace_id with the
// supplied integer value. Kamacu validates the id against the workspaces
// table (a non-existent id → 400 "workspace not found", no row created).
func TestBridge_CreateProject_WorkspaceIDIncluded_WhenSupplied(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"name":"foo","repo_path":"/x","workspace_id":2}`))
	if _, err := b.createProject(context.Background(), req); err != nil {
		t.Fatalf("createProject with workspace_id: %v", err)
	}
	v, ok := gotBody["workspace_id"]
	if !ok {
		t.Fatalf("body MUST contain workspace_id when supplied, got %v", gotBody)
	}
	// JSON numbers decode as float64 — assert the value 2.0.
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("body workspace_id: want numeric, got %T %v", v, v)
	}
	if f != 2.0 {
		t.Errorf("body workspace_id: want 2, got %v", f)
	}
}

// TestBridge_CreateProject_Kamacu400_InvalidPath_ReturnsError (MCPPROJ-03):
// Kamacu's 400 with {"error":"path must be absolute"} surfaces as a wrapped
// error containing "HTTP 400". The bridge does NOT validate the path — Kamacu
// is the validator (SC4).
func TestBridge_CreateProject_Kamacu400_InvalidPath_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"path must be absolute"}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"name":"foo","repo_path":"relative/path"}`))
	_, err := b.createProject(context.Background(), req)
	if err == nil {
		t.Fatal("createProject: expected error for 400, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("createProject error: want substring \"HTTP 400\", got %q", err.Error())
	}
}

// TestBridge_UpdateProject_PartialPATCH_SendsOnlySuppliedFields (MCPPROJ-04,
// D-03): when only name is supplied (all other fields OMITTED), the PATCH body
// contains ONLY the name key — description/github_repo/icon_letters/icon_color
// are absent. Pointer fields distinguish nil (omitted) from "" (explicit clear).
func TestBridge_UpdateProject_PartialPATCH_SendsOnlySuppliedFields(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"id":5}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	// All other fields OMITTED — partial-PATCH must include only name.
	req := newCallToolRequest(json.RawMessage(`{"project_id":5,"name":"new"}`))
	if _, err := b.updateProject(context.Background(), req); err != nil {
		t.Fatalf("updateProject partial: %v", err)
	}
	if gotPath != "/api/projects/5" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/projects/5", gotPath)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("request method: want %q, got %q", http.MethodPatch, gotMethod)
	}
	if gotBody["name"] != "new" {
		t.Errorf("body name: want %q, got %v", "new", gotBody["name"])
	}
	for _, absent := range []string{"description", "github_repo", "icon_letters", "icon_color"} {
		if _, ok := gotBody[absent]; ok {
			t.Errorf("body must NOT contain %q key (D-03 partial-PATCH), got %v", absent, gotBody)
		}
	}
}

// TestBridge_UpdateProject_GithubRepoEmptyString_Unlinks (MCPPROJ-04, D-03):
// an explicit empty github_repo string is a distinct value from omitted —
// Kamacu unlinks (clears to NULL). The body map must contain github_repo with
// the empty-string value when the pointer is non-nil.
func TestBridge_UpdateProject_GithubRepoEmptyString_Unlinks(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":5}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"project_id":5,"github_repo":""}`))
	if _, err := b.updateProject(context.Background(), req); err != nil {
		t.Fatalf("updateProject github_repo empty: %v", err)
	}
	v, ok := gotBody["github_repo"]
	if !ok {
		t.Fatalf("body MUST contain github_repo key when supplied as empty (unlinks → NULL), got %v", gotBody)
	}
	if v != "" {
		t.Errorf("body github_repo: want empty string (unlink), got %v", v)
	}
}

// TestBridge_UpdateProject_BodyExcludes_WorkspaceID_And_AgentID (MCPPROJ-04,
// D-03 / 07-RESEARCH Pitfall 3): even when the agent supplies the excluded
// keys workspace_id and agent_id in the arguments, the bridge's PATCH body
// does NOT contain them. The updateProject args struct declares no such fields
// (json.Unmarshal silently drops unknown keys), AND the body-map construction
// only iterates declared fields. Workspace transfer has its own tool in a
// later plan; agent reassignment is Out of Scope (MCPMORE-01).
func TestBridge_UpdateProject_BodyExcludes_WorkspaceID_And_AgentID(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":5}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	// Agent supplies the EXCLUDED fields — they must be dropped by the bridge.
	req := newCallToolRequest(json.RawMessage(`{"project_id":5,"name":"new","workspace_id":2,"agent_id":3}`))
	if _, err := b.updateProject(context.Background(), req); err != nil {
		t.Fatalf("updateProject excludes: %v", err)
	}
	if _, ok := gotBody["workspace_id"]; ok {
		t.Errorf("body MUST NOT contain workspace_id (D-03 / Pitfall 3 — excluded field), got %v", gotBody)
	}
	if _, ok := gotBody["agent_id"]; ok {
		t.Errorf("body MUST NOT contain agent_id (D-03 / Pitfall 3 — excluded field), got %v", gotBody)
	}
	// The supplied name (a declared field) MUST still reach Kamacu.
	if gotBody["name"] != "new" {
		t.Errorf("body name: want %q, got %v", "new", gotBody["name"])
	}
}

// TestBridge_UpdateProject_Kamacu400_DescriptionTooLong_ReturnsError
// (MCPPROJ-04): Kamacu's 400 with {"error":"Description is too long."}
// surfaces as a wrapped error containing "HTTP 400". The bridge does NOT
// enforce the 280-char cap — Kamacu is the validator.
func TestBridge_UpdateProject_Kamacu400_DescriptionTooLong_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Description is too long."}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"project_id":5,"description":"over 280 chars..."}`))
	_, err := b.updateProject(context.Background(), req)
	if err == nil {
		t.Fatal("updateProject: expected error for 400, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("updateProject error: want substring \"HTTP 400\", got %q", err.Error())
	}
}

// TestBridge_DeleteProject_Happy_CallsDeletePath (MCPPROJ-05): DELETE
// /api/projects/{id} is called with the right path and method. Kamacu answers
// 204 on success (folder: hard delete; managed: gated removal all-clear).
func TestBridge_DeleteProject_Happy_CallsDeletePath(t *testing.T) {
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
	req := newCallToolRequest(json.RawMessage(`{"project_id":7}`))
	if _, err := b.deleteProject(context.Background(), req); err != nil {
		t.Fatalf("deleteProject: %v", err)
	}
	if gotPath != "/api/projects/7" {
		t.Errorf("request URL.Path: want %q, got %q", "/api/projects/7", gotPath)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("request method: want %q, got %q", http.MethodDelete, gotMethod)
	}
}

// TestBridge_DeleteProject_Managed409_PassesThroughStructuredBody (MCPPROJ-05,
// Gap 2): Kamacu's managed-delete 409 surfaces as a wrapped error containing
// the "HTTP 409" status-code substring AND the raw body. The real Kamacu
// managed-409 body shape is {"error":"the project can't be deleted yet",
// "reasons":[{"kind":"uncommitted","target":"task #3"}]} — keyed on "error",
// NOT "message" (07-RESEARCH Gap 2). The D-05 wrap passes the body as a raw
// string, so the structured shape survives. Assertions match on the
// "can't be deleted" message substring and the "reasons" key substring —
// NEVER on the "message" JSON key name (Pitfall 2).
func TestBridge_DeleteProject_Managed409_PassesThroughStructuredBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		// Real Kamacu shape (internal/api/projects.go:850-853) — keyed on
		// "error" with a structured "reasons" array of {kind, target} objects.
		_, _ = w.Write([]byte(`{"error":"the project can't be deleted yet","reasons":[{"kind":"uncommitted","target":"task #3"}]}`))
	}))
	t.Cleanup(srv.Close)

	b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
	req := newCallToolRequest(json.RawMessage(`{"project_id":7}`))
	_, err := b.deleteProject(context.Background(), req)
	if err == nil {
		t.Fatal("deleteProject: expected error for 409, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 409") {
		t.Errorf("deleteProject error: want substring \"HTTP 409\", got %q", err.Error())
	}
	// Gap 2: assert on Kamacu message substring, NOT on a JSON key name. The
	// raw body is inlined in the D-05 wrap, so the message text survives.
	if !strings.Contains(err.Error(), "can't be deleted") {
		t.Errorf("deleteProject error: want substring \"can't be deleted\" (Kamacu message), got %q", err.Error())
	}
	// The structured "reasons" array also survives in the raw body string.
	if !strings.Contains(err.Error(), "reasons") {
		t.Errorf("deleteProject error: want substring \"reasons\" (structured 409 body), got %q", err.Error())
	}
}
