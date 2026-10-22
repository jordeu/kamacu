package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerArtifactTools(s *mcp.Server, b *bridge) {
	for _, spec := range []struct {
		name, description string
		fields            map[string]any
		required          []string
	}{
		{"list_artifacts", "List a task's published HTML artifacts, stable IDs, titles, and revisions.", map[string]any{}, []string{"task_id"}},
		{"begin_artifact_upload", "Create an HTML artifact upload. Accepts a self-contained .html or .tar.gz with root index.html. Returns an absolute short-lived upload_url, method and headers. HTTP PUT the raw file bytes from your machine, then call finalize_artifact_upload. No shared filesystem is needed. Limit 20 MB; all resources must be bundled. JavaScript is sandboxed.", map[string]any{"title": map[string]any{"type": "string"}, "filename": map[string]any{"type": "string"}, "size_bytes": map[string]any{"type": "integer"}}, []string{"task_id", "title", "filename", "size_bytes"}},
		{"begin_artifact_replacement", "Replace an existing artifact's content, preserving its ID and user-edited title. PUT raw file bytes to the returned upload_url using its headers, then finalize. A failed or conflicting upload leaves the current preview intact.", map[string]any{"artifact_id": map[string]any{"type": "string"}, "filename": map[string]any{"type": "string"}, "size_bytes": map[string]any{"type": "integer"}}, []string{"task_id", "artifact_id", "filename", "size_bytes"}},
		{"finalize_artifact_upload", "Validate and atomically publish an uploaded artifact. Call only after the HTTP PUT succeeds. Successful retries return the artifact; replacement conflicts require beginning a new upload.", map[string]any{"upload_id": map[string]any{"type": "string"}}, []string{"task_id", "upload_id"}},
	} {
		spec.fields["task_id"] = map[string]any{"type": "integer"}
		s.AddTool(&mcp.Tool{Name: spec.name, Description: spec.description, InputSchema: map[string]any{"type": "object", "properties": spec.fields, "required": spec.required, "additionalProperties": false}}, withRecover(spec.name, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.artifactCall(ctx, spec.name, req)
		}))
	}
}

func (b *bridge) artifactCall(ctx context.Context, name string, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input struct {
		TaskID     int64  `json:"task_id"`
		ArtifactID string `json:"artifact_id"`
		UploadID   string `json:"upload_id"`
		Title      string `json:"title"`
		Filename   string `json:"filename"`
		Size       int64  `json:"size_bytes"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/api/tasks/%d", input.TaskID)
	switch name {
	case "list_artifacts":
		return b.call(ctx, http.MethodGet, path+"/artifacts", nil)
	case "finalize_artifact_upload":
		return b.call(ctx, http.MethodPost, path+"/artifact-uploads/"+url.PathEscape(input.UploadID)+"/finalize", nil)
	case "begin_artifact_upload":
		path += "/artifact-uploads"
	case "begin_artifact_replacement":
		path += "/artifacts/" + url.PathEscape(input.ArtifactID) + "/uploads"
	}
	body, _ := json.Marshal(map[string]any{"title": input.Title, "filename": input.Filename, "size_bytes": input.Size})
	resp, err := b.do(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 201 {
		return nil, fmt.Errorf("kamacu upload: HTTP %d: %s", resp.StatusCode, raw)
	}
	var result map[string]any
	if err = json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	relative, ok := result["upload_url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing upload_url")
	}
	result["upload_url"] = b.base + relative
	raw, err = json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, nil
}
