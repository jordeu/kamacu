package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPrintVersion proves the --version flag path: the binary reports the
// build-injected version string (defaults to "dev" for plain `go build`;
// goreleaser injects the git tag via -ldflags -X main.version=...).
func TestPrintVersion(t *testing.T) {
	var buf bytes.Buffer
	printVersion(&buf)
	if got := buf.String(); got != version+"\n" {
		t.Errorf("printVersion wrote %q, want %q", got, version+"\n")
	}
}

// TestEnsureLoopback proves D-21: the server refuses any --addr that is not
// a loopback address. Non-localhost hostnames are refused outright — they
// are never resolved, so /etc/hosts games cannot widen the bind.
func TestEnsureLoopback(t *testing.T) {
	tests := []struct {
		addr string
		ok   bool
	}{
		{"127.0.0.1:7333", true},
		{"localhost:7333", true},
		{"[::1]:7333", true},
		{"127.0.0.2:7333", true}, // any 127/8 address is loopback
		{":7333", false},         // empty host = all interfaces
		{"0.0.0.0:7333", false},
		{"192.168.1.5:7333", false},
		{"[::]:7333", false},
		{"myhost:7333", false}, // non-localhost hostname: refused, never resolved
		{"garbage", false},     // SplitHostPort error
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			err := ensureLoopback(tt.addr)
			if tt.ok && err != nil {
				t.Errorf("ensureLoopback(%q) = %v, want nil", tt.addr, err)
			}
			if !tt.ok && err == nil {
				t.Errorf("ensureLoopback(%q) = nil, want error", tt.addr)
			}
		})
	}
}

// TestHostCheck proves the DNS-rebinding defense: only loopback hostnames
// pass, port-agnostically (the Vite dev proxy forwards Host: 127.0.0.1:5173,
// which must keep working).
func TestHostCheck(t *testing.T) {
	tests := []struct {
		host string
		ok   bool
	}{
		{"localhost:7333", true},
		{"localhost", true},
		{"127.0.0.1:5173", true}, // port-agnostic by design (dev proxy)
		{"127.0.0.1", true},
		{"[::1]:7333", true},
		{"[::1]", true},
		{"evil.example:7333", false},
		{"attacker.com", false},
		{"192.168.1.5:7333", false},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			handlerRan := false
			h := hostCheck(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerRan = true
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest("GET", "/api/sessions", nil)
			req.Host = tt.host
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if tt.ok {
				if !handlerRan || rec.Code != http.StatusOK {
					t.Errorf("Host %q: handler ran=%v code=%d, want pass-through 200", tt.host, handlerRan, rec.Code)
				}
				return
			}
			if handlerRan {
				t.Errorf("Host %q: inner handler ran, want blocked before it", tt.host)
			}
			if rec.Code != http.StatusForbidden {
				t.Errorf("Host %q: code = %d, want 403", tt.host, rec.Code)
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Host %q: Content-Type = %q, want application/json", tt.host, got)
			}
			if body := rec.Body.String(); !strings.Contains(body, "forbidden host") {
				t.Errorf("Host %q: body = %q, want {\"error\":\"forbidden host\"}", tt.host, body)
			}
		})
	}
}

// TestHookBaseURL proves T-hwd-03: a wildcard --addr host (0.0.0.0 / :: /
// empty) is normalized to loopback for the agent-status hook URL — local
// hooks must never curl a wildcard/remote-looking address — while a SPECIFIC
// host (loopback IP, hostname, or a non-loopback IP under
// --insecure-allow-remote) is left unchanged.
func TestHookBaseURL(t *testing.T) {
	tests := []struct {
		addr string
		want string
	}{
		{"0.0.0.0:7333", "http://127.0.0.1:7333"},   // IPv4 wildcard -> loopback
		{"[::]:7333", "http://127.0.0.1:7333"},       // IPv6 wildcard -> loopback
		{":7333", "http://127.0.0.1:7333"},           // empty host (all ifaces) -> loopback
		{"192.168.1.5:7333", "http://192.168.1.5:7333"}, // specific reachable IP: unchanged
		{"127.0.0.1:7333", "http://127.0.0.1:7333"},  // already loopback: unchanged
		{"localhost:7333", "http://localhost:7333"},  // hostname: unchanged
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			if got := hookBaseURL(tt.addr); got != tt.want {
				t.Errorf("hookBaseURL(%q) = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}

// TestInsecureAllowRemoteRelaxesHostCheck proves the flag's effect on the
// hostCheck wrap WITHOUT booting main() or binding a socket: it replicates
// main's handler-selection logic (hostCheck(mux) when the flag is false, the
// bare mux when true) and asserts that a non-loopback Host header is 403'd in
// the default case but reaches the inner handler (200) when the flag is set.
func TestInsecureAllowRemoteRelaxesHostCheck(t *testing.T) {
	tests := []struct {
		name                string
		insecureAllowRemote bool
		host                string
		wantInnerRan        bool
		wantCode            int
	}{
		{"default blocks non-loopback Host", false, "192.168.1.5:7333", false, http.StatusForbidden},
		{"flag serves non-loopback Host", true, "192.168.1.5:7333", true, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			innerRan := false
			mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				innerRan = true
				w.WriteHeader(http.StatusOK)
			})

			// Mirror main()'s selection exactly.
			var handler http.Handler = hostCheck(mux)
			if tt.insecureAllowRemote {
				handler = mux
			}

			req := httptest.NewRequest("GET", "/api/sessions", nil)
			req.Host = tt.host
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if innerRan != tt.wantInnerRan {
				t.Errorf("inner handler ran=%v, want %v", innerRan, tt.wantInnerRan)
			}
			if rec.Code != tt.wantCode {
				t.Errorf("code = %d, want %d", rec.Code, tt.wantCode)
			}
		})
	}
}
