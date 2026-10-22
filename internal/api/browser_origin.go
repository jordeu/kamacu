package api

import (
	"net/http"
	"net/url"
	"strings"
)

// BrowserOriginGuard prevents opaque-origin artifact documents from writing to
// Kamacu even if they navigate away from their original CSP-protected document.
// CLI/MCP requests carry no browser Origin and retain their existing behavior.
// Uploaded assets alone permit cross-origin reads for sandboxed module scripts.
func BrowserOriginGuard(next http.Handler, allowedHosts []string) http.Handler {
	allowed := make(map[string]bool, len(allowedHosts))
	for _, host := range allowedHosts {
		allowed[host] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || (strings.HasPrefix(r.URL.Path, "/api/artifact-content/") && (r.Method == "GET" || r.Method == "HEAD")) {
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		if origin == "" {
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				writeError(w, http.StatusForbidden, "cross-origin API request denied")
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Host != r.Host && !allowed[parsed.Host]) {
			writeError(w, http.StatusForbidden, "cross-origin API request denied")
			return
		}
		next.ServeHTTP(w, r)
	})
}
