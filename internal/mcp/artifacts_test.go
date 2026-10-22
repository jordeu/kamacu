package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestArtifactBridgeUploadURLsAndRoutes(t *testing.T) {
	for _, tc := range []struct{ name, path string }{
		{"begin_artifact_upload", "/api/tasks/7/artifact-uploads"},
		{"begin_artifact_replacement", "/api/tasks/7/artifacts/artifact-1/uploads"},
		{"list_artifacts", "/api/tasks/7/artifacts"},
		{"finalize_artifact_upload", "/api/tasks/7/artifact-uploads/upload-1/finalize"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Errorf("path: %s", r.URL.Path)
				}
				if r.Header.Get("X-Kamacu-Token") != "token" {
					t.Error("missing bridge token")
				}
				if tc.name == "list_artifacts" {
					if r.Method != "GET" {
						t.Error(r.Method)
					}
					fmt.Fprint(w, `[]`)
					return
				}
				if r.Method != "POST" {
					t.Error(r.Method)
				}
				if tc.name == "finalize_artifact_upload" {
					fmt.Fprint(w, `{"id":"artifact-1"}`)
					return
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["filename"] != "preview.html" || body["size_bytes"] != float64(10) {
					t.Error(body)
				}
				w.WriteHeader(201)
				fmt.Fprint(w, `{"upload_id":"upload-1","upload_url":"/api/artifact-uploads/upload-1","headers":{"Authorization":"Bearer scoped"}}`)
			}))
			defer server.Close()
			b := &bridge{base: server.URL, token: "token", client: server.Client()}
			result, err := b.artifactCall(context.Background(), tc.name, newCallToolRequest(json.RawMessage(`{"task_id":7,"artifact_id":"artifact-1","upload_id":"upload-1","title":"Preview","filename":"preview.html","size_bytes":10}`)))
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "begin_artifact_upload" || tc.name == "begin_artifact_replacement" {
				var body map[string]any
				if err = json.Unmarshal([]byte(result.Content[0].(*sdk.TextContent).Text), &body); err != nil {
					t.Fatal(err)
				}
				if body["upload_url"] != server.URL+"/api/artifact-uploads/upload-1" {
					t.Fatal(body)
				}
			}
		})
	}
}
