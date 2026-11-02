package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kamacu/internal/integration"
)

func TestIntegrationRoutes(t *testing.T) {
	home := t.TempDir()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	manager := integration.New(home, filepath.Join(home, ".config"), "/usr/bin/kamacu")
	mux := http.NewServeMux()
	IntegrationRoutes(mux, manager)
	handler := BrowserOriginGuard(mux, nil)
	call := func(method, path, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if w := call("POST", "/api/integrations/claude/enable", "https://elsewhere.test"); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := call("POST", "/api/integrations/custom/enable", ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := call("POST", "/api/integrations/claude/unknown", ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := call("POST", "/api/integrations/claude/enable", ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	config := filepath.Join(home, ".claude.json")
	if _, err := os.Stat(config); err != nil {
		t.Fatal(err)
	}
	if w := call("GET", "/api/integrations", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"enabled":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if err := os.Remove(config); err != nil {
		t.Fatal(err)
	}
	if w := call("POST", "/api/integrations/claude/repair", ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call("POST", "/api/integrations/claude/remove", ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	contents, err := os.ReadFile(config)
	if err != nil || strings.Contains(string(contents), `"kamacu"`) {
		t.Fatal("MCP registration not removed", err)
	}
}
