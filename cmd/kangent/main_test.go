package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
