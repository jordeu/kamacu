package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kamacu/internal/integration"
)

func TestIntegrationRoutes(t *testing.T) {
	for _, tc := range []struct {
		engine, config, instructions string
	}{
		{"claude", ".claude.json", ".claude/CLAUDE.md"},
		{"pi", ".pi/agent/mcp.json", ".pi/agent/AGENTS.md"},
	} {
		t.Run(tc.engine, func(t *testing.T) {
			testIntegrationRoutes(t, tc.engine, tc.config, tc.instructions)
		})
	}
}

func testIntegrationRoutes(t *testing.T, engine, config, instructions string) {
	t.Helper()
	home := t.TempDir()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, engine), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	manager := integration.New(home, filepath.Join(home, ".config"), "/usr/bin/kamacu")
	mux := http.NewServeMux()
	IntegrationRoutes(mux, manager)
	handler := BrowserOriginGuard(mux, nil, nil)
	call := func(method, path, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	prefix := "/api/integrations/" + engine
	const origin = "http://example.com" // Host used by httptest.NewRequest for relative targets.
	assertStatus := func(w *httptest.ResponseRecorder, enabled bool, mcp string) {
		t.Helper()
		var result integration.Status
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &result) != nil ||
			result.Engine != engine || result.Enabled != enabled || result.MCP != mcp {
			t.Fatalf("%s integration status = %d %s", engine, w.Code, w.Body.String())
		}
	}
	if w := call("POST", prefix+"/enable", "https://elsewhere.test"); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := call("POST", "/api/integrations/custom/enable", ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := call("POST", prefix+"/unknown", ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	assertStatus(call("POST", prefix+"/enable", origin), true, "registered")
	config = filepath.Join(home, config)
	if _, err := os.Stat(config); err != nil {
		t.Fatal(err)
	}
	instructions = filepath.Join(home, instructions)
	if contents, err := os.ReadFile(instructions); err != nil || !strings.Contains(string(contents), "KAMACU_PROJECT_CONTEXT") {
		t.Fatalf("project context instructions not installed: %v", err)
	}
	w := call("GET", "/api/integrations", origin)
	var statuses []integration.Status
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &statuses) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	found := false
	for _, item := range statuses {
		if item.Engine == engine && item.Enabled && item.MCP == "registered" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("%s missing from enabled integrations: %s", engine, w.Body.String())
	}
	if err := os.Remove(config); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(instructions); err != nil {
		t.Fatal(err)
	}
	assertStatus(call("POST", prefix+"/repair", origin), true, "registered")
	if contents, err := os.ReadFile(instructions); err != nil || !strings.Contains(string(contents), "KAMACU_PROJECT_CONTEXT") {
		t.Fatalf("project context instructions not restored: %v", err)
	}
	assertStatus(call("POST", prefix+"/remove", origin), false, "missing")
	contents, err := os.ReadFile(config)
	if err != nil || strings.Contains(string(contents), `"kamacu"`) {
		t.Fatal("MCP registration not removed", err)
	}
	if _, err := os.Stat(instructions); !os.IsNotExist(err) {
		t.Fatalf("managed project context instructions not removed: %v", err)
	}
}
