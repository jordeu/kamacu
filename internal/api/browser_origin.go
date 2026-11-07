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
// allowedOrigins contains exact host[:port] entries (e.g. --dev-origin).
// allowedHosts contains trusted proxy hostnames and allows any port.
func BrowserOriginGuard(next http.Handler, allowedOrigins, allowedHosts []string) http.Handler {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[strings.ToLower(origin)] = true
	}
	hosts := make(map[string]bool, len(allowedHosts))
	for _, host := range allowedHosts {
		hosts[strings.ToLower(host)] = true
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
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			writeError(w, http.StatusForbidden, "cross-origin API request denied")
			return
		}
		if !strings.EqualFold(parsed.Host, r.Host) && !allowed[strings.ToLower(parsed.Host)] && !hosts[strings.ToLower(parsed.Hostname())] {
			writeError(w, http.StatusForbidden, "cross-origin API request denied")
			return
		}
		next.ServeHTTP(w, r)
	})
}
