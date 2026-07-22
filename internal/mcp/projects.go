package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerProjectTools registers the project resource's five MCP tools (D-07
// per-resource split):
//
//   - list_projects (MCPPROJ-01, Plan 01): GET /api/projects with optional
//     ?workspace_id=N.
//   - get_project (MCPPROJ-02, Plan 03): GET /api/projects/{id}.
//   - create_project (MCPPROJ-03, Plan 03, D-04 two-arg fork): POST /api/projects
//     with {name, repo_path, repo, workspace_id?} sent as-is — Kamacu's
//     create handler dispatches on `repo != ""` (managed checkout) vs folder
//     path; the bridge does ZERO type detection.
//   - update_project (MCPPROJ-04, Plan 03, D-03 partial-PATCH): PATCH
//     /api/projects/{id} with the supplied fields only. D-03 EXCLUDES
//     workspace_id and agent_id from the InputSchema and from the args struct
//     — workspace transfer is its own tool in a later plan, agent reassignment
//     is Out of Scope (MCPMORE-01). The body map only carries declared fields,
//     so even if an agent supplies excluded keys the SDK's json.Unmarshal
//     silently drops them.
//   - delete_project (MCPPROJ-05, Plan 03): DELETE /api/projects/{id}. The v1.4
//     two-pass gated removal (folder: hard delete never touching the dir;
//     managed: all-or-nothing with structured 409 reasons) runs unchanged
//     through the bridge.
//
// Each tool is a thin HTTP-bridge handler that builds a path and optional body,
// then delegates the response-handling half to bridge.call (Plan 01's shared
// helper). The bridge performs NO validation — Kamacu's existing handlers
// validate (empty name → 400 "name is required"; description >280 → 400
// "Description is too long."; off-palette icon_color → 400; missing project →
// 404 "project not found"; dirty managed checkout → 409 with structured
// reasons). The bridge sends fields as-is and lets the 400/404/409 surface
// verbatim through the D-05 wrap (SC4 satisfied by NOT duplicating validation
// in the bridge).
//
// The low-level Server.AddTool is used with an explicit map[string]any
// InputSchema — NOT the typed generic mcp.AddTool[In, Out] helper (Phase 06
// locked this; SDK v1.6.1's Tool.InputSchema field is `any`).
func registerProjectTools(s *mcp.Server, b *bridge) {
	// 1. list_projects (MCPPROJ-01, Plan 01).
	s.AddTool(
		&mcp.Tool{
			Name:        "list_projects",
			Description: "List all Kamacu projects, optionally scoped to a workspace. Returns the raw JSON array from GET /api/projects (with ?workspace_id=N when workspace_id is supplied).",
			// InputSchema MUST be a JSON Schema with type:"object" (Pitfall 2:
			// AddTool panics otherwise). A plain map[string]any keeps the file
			// readable without pulling in jsonschema-go's typed API. The stale
			// project_id no-op arg from Phase 06 is REMOVED (07-RESEARCH
			// Pitfall 6); the real workspace_id? arg is the only property.
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace_id": map[string]any{
						"type":        "integer",
						"description": "Optional workspace id. When supplied, only projects in that workspace are returned ( Kamacu appends ?workspace_id=N to GET /api/projects ).",
					},
				},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.listProjects(ctx, req)
		},
	)

	// 2. get_project (MCPPROJ-02).
	s.AddTool(
		&mcp.Tool{
			Name:        "get_project",
			Description: "Fetch a single project by id. Returns the raw JSON object from GET /api/projects/{id}. A missing project surfaces Kamacu's 404 body verbatim.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{
						"type":        "integer",
						"description": "The project id.",
					},
				},
				"required": []string{"project_id"},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.getProject(ctx, req)
		},
	)

	// 3. create_project (MCPPROJ-03, D-04 two-arg fork).
	s.AddTool(
		&mcp.Tool{
			Name:        "create_project",
			Description: "Create a new Kamacu project. Two creation modes share this tool (Kamacu's fork dispatches on whether `repo` is non-empty): supply `repo_path` (absolute path to a local git checkout) for a folder project Kamacu never touches on delete; OR supply `repo` (GitHub owner/name) for a managed checkout Kamacu gh-validates, clones into ~/.kamacu/repos/<owner>/<name>, and gated-removes on delete. Returns the raw JSON object from POST /api/projects (201 on success). Clone failures (auth/network/bad repo) surface inline without leaving a half-created project.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "Project name. When omitted on the folder path, Kamacu defaults to filepath.Base(repo_path); on the managed path, defaults to the repo name.",
					},
					"repo_path": map[string]any{
						"type":        "string",
						"description": "Absolute path to a local git checkout. Creates a folder project (Kamacu never touches the dir on delete).",
					},
					"repo": map[string]any{
						"type":        "string",
						"description": "GitHub owner/name (e.g. 'owner/repo'). Creates a managed checkout — Kamacu gh-validates, clones into ~/.kamacu/repos/<owner>/<name>, and owns the dir (gated remove on delete).",
					},
					"workspace_id": map[string]any{
						"type":        "integer",
						"description": "Optional target workspace id. Omitted → the default Personal workspace.",
					},
				},
				"required": []string{"name"},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.createProject(ctx, req)
		},
	)

	// 4. update_project (MCPPROJ-04, D-03 partial-PATCH).
	// D-03 / 07-RESEARCH Pitfall 3: workspace_id and agent_id are EXCLUDED
	// from the properties map AND from the args struct below. Workspace
	// transfer has its own tool in a later plan; agent reassignment is Out of
	// Scope (MCPMORE-01). Even if an agent supplies excluded keys, the SDK's
	// json.Unmarshal silently drops them (unknown properties are ignored) AND
	// the body-map construction only iterates the declared fields.
	s.AddTool(
		&mcp.Tool{
			Name:        "update_project",
			Description: "Partial-PATCH a project's editable fields. Only supplied fields are written; omitted fields are left untouched. An explicit empty github_repo string unlinks the repo (clears to NULL). Returns the raw JSON object from PATCH /api/projects/{id}. Workspace transfer and agent reassignment are NOT exposed here — they have their own dedicated tools.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{
						"type":        "integer",
						"description": "The project id.",
					},
					"name": map[string]any{
						"type":        "string",
						"description": "Optional new name (trimmed, must be non-empty when supplied).",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "Optional new description (≤280 chars).",
					},
					"github_repo": map[string]any{
						"type":        "string",
						"description": "Optional new GitHub repo link (owner/name). Empty string unlinks (clears to NULL); a non-empty value is gh-validated + canonicalized.",
					},
					"icon_letters": map[string]any{
						"type":        "string",
						"description": "Optional new icon letters (normalized to ≤2 uppercase alphanumerics).",
					},
					"icon_color": map[string]any{
						"type":        "string",
						"description": "Optional new icon color (must be a curated-palette member).",
					},
				},
				"required": []string{"project_id"},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.updateProject(ctx, req)
		},
	)

	// 5. delete_project (MCPPROJ-05).
	s.AddTool(
		&mcp.Tool{
			Name:        "delete_project",
			Description: "Hard-delete a project. Folder projects: Kamacu deletes the row (CASCADE removes tasks) and NEVER touches the directory. Managed projects: Kamacu runs a two-pass all-or-nothing gated removal — the gate phase computes dirty/unpushed/stash/sessions across every task worktree AND the clone root; if ANY gate trips, returns 409 with a structured {error, reasons} body and removes NOTHING. Returns 204 on success, 404 if the project id is missing, 409 (with reasons) if a managed project is dirty.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{
						"type":        "integer",
						"description": "The project id. Folder projects: hard delete, directory never touched. Managed projects: two-pass gated removal — 409 with structured reasons if dirty/unpushed/stash/sessions.",
					},
				},
				"required": []string{"project_id"},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.deleteProject(ctx, req)
		},
	)
}

// listProjects is the body of the list_projects tool handler. It calls
// GET /api/projects, optionally with ?workspace_id=N when the caller supplies
// the workspace_id arg (D-02 / MCPPROJ-01). The handler delegates the
// response-handling half to bridge.call — transport / non-200 / 200
// passthrough behavior is identical to every other Phase 07 tool.
//
// Backward-compat note: when workspace_id is omitted (or args is empty), the
// request path is the bare /api/projects — same as Phase 06's listProjects,
// so existing callers (the SPA, Phase 06's tool invocation before this
// rename) see byte-for-byte identical responses.
func (b *bridge) listProjects(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		WorkspaceID *int64 `json:"workspace_id"`
	}
	// Only unmarshal when there are arguments — an empty/nil Arguments slice
	// would error on json.Unmarshal, but the MCP SDK may deliver a request
	// with no arguments at all (e.g. tests, the SDK zero value).
	if len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, fmt.Errorf("list_projects: invalid arguments: %w", err)
		}
	}
	path := "/api/projects"
	if args.WorkspaceID != nil {
		path = fmt.Sprintf("/api/projects?workspace_id=%d", *args.WorkspaceID)
	}
	return b.call(ctx, http.MethodGet, path, nil)
}

// getProject is the body of the get_project tool handler (MCPPROJ-02). It
// calls GET /api/projects/{id} (the Gap 1 endpoint added in Plan 01). A
// missing/invalid project_id surfaces Kamacu's 404 body verbatim through the
// D-05 wrap.
func (b *bridge) getProject(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		ProjectID int64 `json:"project_id"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("get_project: invalid arguments: %w", err)
	}
	return b.call(ctx, http.MethodGet, fmt.Sprintf("/api/projects/%d", args.ProjectID), nil)
}

// createProject is the body of the create_project tool handler (MCPPROJ-03,
// D-04 two-arg fork). It POSTs {name, repo_path, repo, workspace_id?} to
// /api/projects. ALL four fields are sent as-is — Kamacu's create handler
// dispatches on `strings.TrimSpace(req.Repo) != ""` (managed checkout via gh
// clone) vs the folder path (validateRepoPath + INSERT). The bridge does ZERO
// type detection (D-04): no `if args.Repo != ""` branch, no validation of
// repo_path syntax, no canonicalization of the repo ref. Kamacu is the
// validator (invalid path → 400; clone failure → 400/500 with no row created).
//
// WorkspaceID is a *int64 so omitted = nil (Kamacu falls back to the default
// Personal workspace); when supplied it is included in the body so Kamacu
// resolves + validates it (a non-existent workspace id → 400 "workspace not
// found", no row created).
//
// Note: the Kamacu handler also accepts an agent_id field; the MCP tool does
// NOT expose it (the new project lands on the global default agent — Claude
// seed on a fresh install). agent_id is therefore absent from the body.
func (b *bridge) createProject(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Name        string `json:"name"`
		RepoPath    string `json:"repo_path"`
		Repo        string `json:"repo"`
		WorkspaceID *int64 `json:"workspace_id"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("create_project: invalid arguments: %w", err)
	}
	// D-04: send all four fields as-is. Kamacu's create handler dispatches on
	// whether `repo` is non-empty; the bridge does not branch. workspace_id is
	// the lone optional — only included when supplied (matches Kamacu's *int64
	// decode where omitted = default fallback).
	body := map[string]any{
		"name":      args.Name,
		"repo_path": args.RepoPath,
		"repo":      args.Repo,
	}
	if args.WorkspaceID != nil {
		body["workspace_id"] = *args.WorkspaceID
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("create_project: marshal body: %w", err)
	}
	return b.call(ctx, http.MethodPost, "/api/projects", bytes.NewReader(jsonBody))
}

// updateProject is the body of the update_project tool handler (MCPPROJ-04,
// D-03 partial-PATCH). It PATCHes only the supplied fields to
// /api/projects/{id} — pointer fields distinguish nil (omitted) from ""
// (explicit clear). Kamacu's validation gates (name required, description
// ≤280, github_repo gh-canonicalized, icon_letters validateIconLetters,
// icon_color validateIconColor palette member, empty body → 400 "nothing to
// update") apply unchanged through the bridge — the bridge does NOT
// re-implement validation.
//
// D-03 / 07-RESEARCH Pitfall 3: the args struct has NO WorkspaceID and NO
// AgentID fields. Even if an agent supplies them in the arguments, the SDK's
// json.Unmarshal silently drops them (unknown properties are ignored) AND the
// body-map construction below only iterates the declared fields — they never
// reach Kamacu's PATCH. Workspace transfer has its own tool in a later plan;
// agent reassignment is Out of Scope (MCPMORE-01).
//
// Note on github_repo: an explicit empty string IS a valid value (Kamacu
// unlinks → NULL); it is distinct from omitted (left untouched). The body map
// therefore includes github_repo when the pointer is non-nil, regardless of
// whether the dereferenced string is empty.
func (b *bridge) updateProject(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		ProjectID   int64   `json:"project_id"`
		Name        *string `json:"name"`
		Description *string `json:"description"`
		GithubRepo  *string `json:"github_repo"`
		IconLetters *string `json:"icon_letters"`
		IconColor   *string `json:"icon_color"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("update_project: invalid arguments: %w", err)
	}
	// D-03: build the body map conditionally — ONLY non-nil fields are sent.
	// Omitted keys never reach Kamacu's partial-PATCH; an explicit "" for
	// github_repo is preserved (unlinks → NULL).
	body := map[string]any{}
	if args.Name != nil {
		body["name"] = *args.Name
	}
	if args.Description != nil {
		body["description"] = *args.Description
	}
	if args.GithubRepo != nil {
		body["github_repo"] = *args.GithubRepo
	}
	if args.IconLetters != nil {
		body["icon_letters"] = *args.IconLetters
	}
	if args.IconColor != nil {
		body["icon_color"] = *args.IconColor
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("update_project: marshal body: %w", err)
	}
	return b.call(ctx, http.MethodPatch, fmt.Sprintf("/api/projects/%d", args.ProjectID), bytes.NewReader(jsonBody))
}

// deleteProject is the body of the delete_project tool handler (MCPPROJ-05).
// It DELETEs /api/projects/{id}. The v1.4 two-pass gated removal (folder
// projects: hard delete, directory never touched; managed projects: gate
// phase computes dirty/unpushed/stash/sessions, REMOVE phase tears down only
// on all-clear, 409 with structured reasons otherwise) runs unchanged through
// the bridge. A missing project surfaces Kamacu's 404 body verbatim.
func (b *bridge) deleteProject(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		ProjectID int64 `json:"project_id"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("delete_project: invalid arguments: %w", err)
	}
	return b.call(ctx, http.MethodDelete, fmt.Sprintf("/api/projects/%d", args.ProjectID), nil)
}
