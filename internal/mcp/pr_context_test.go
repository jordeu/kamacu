package mcp

import (
	"context"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPRContextToolsRoundTrip(t *testing.T) {
	requests := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"github":{"viewer":"alice","viewer_ever_approved":true},"links":[]}`))
	}))
	defer srv.Close()
	ctx := context.Background()
	a, b := mcpsdk.NewInMemoryTransports()
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test", Version: "test"}, nil)
	registerPRContextTools(server, &bridge{base: srv.URL, client: srv.Client()})
	ss, err := server.Connect(ctx, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "test"}, nil)
	cs, err := client.Connect(ctx, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	for _, tc := range []struct {
		name string
		args map[string]any
		path string
	}{
		{"get_pr_context", map[string]any{"repository": "https://github.com/acme/repo/pull/7"}, "GET /api/pr-context?"},
		{"get_task_pr_context", map[string]any{"task_id": 1}, "GET /api/tasks/1/pr-context"},
		{"set_task_pr_link", map[string]any{"task_id": 1, "repository": "acme/repo", "number": 7, "role": "incidental", "evidence": "Wrong PR"}, "PUT /api/tasks/1/pr-links"},
	} {
		res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: tc.name, Arguments: tc.args})
		if err != nil || res.IsError {
			t.Fatalf("%s: %v %+v", tc.name, err, res)
		}
		if !strings.Contains(res.Content[0].(*mcpsdk.TextContent).Text, `"viewer_ever_approved":true`) {
			t.Fatal("lost structured facts")
		}
		if !strings.HasPrefix(requests[len(requests)-1], tc.path) {
			t.Fatal(requests)
		}
	}
}
