package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBrowserOriginGuard(t *testing.T) {
	for _, tc := range []struct {
		name, origin, site, path string
		want                     int
	}{
		{"CLI", "", "", "/api/tasks", 204},
		{"UI", "http://localhost:7333", "same-origin", "/api/tasks", 204},
		{"dev proxy", "http://localhost:5173", "same-site", "/api/tasks", 204},
		{"opaque artifact", "null", "cross-site", "/api/tasks", 403},
		{"external page", "https://example.com", "cross-site", "/api/tasks", 403},
		{"cross-site without origin", "", "cross-site", "/api/tasks", 403},
		{"opaque local assets", "null", "cross-site", "/api/artifact-content/id/rev/index.html", 204},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := BrowserOriginGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), []string{"localhost:5173"})
			method := "POST"
			if tc.name == "opaque local assets" {
				method = "GET"
			}
			req := httptest.NewRequest(method, "http://localhost:7333"+tc.path, nil)
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Sec-Fetch-Site", tc.site)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != tc.want {
				t.Fatalf("status %d, want %d", response.Code, tc.want)
			}
		})
	}
}
