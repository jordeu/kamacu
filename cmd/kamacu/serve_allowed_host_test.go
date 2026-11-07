package main

import (
	"context"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/coder/websocket"

	"kamacu/internal/api"
	"kamacu/internal/session"
	"kamacu/internal/ws"
)

func TestAllowedHostFlag(t *testing.T) {
	for _, value := range []string{"", "*", "*.example.com", "http://x", "x/path", "user@x", "x:443", "x?query", "x#fragment", "x[ab]", "x\\y", " x", "0.0.0.0", "::", "[::]", "-x", "x..example"} {
		t.Run(value, func(t *testing.T) {
			var cmd serveCmd
			flags := flag.NewFlagSet("serve", flag.ContinueOnError)
			flags.SetOutput(io.Discard)
			cmd.SetFlags(flags)
			if err := flags.Parse([]string{"--allowed-host=" + value}); err == nil {
				t.Fatalf("invalid host %q accepted", value)
			}
		})
	}
	var cmd serveCmd
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	cmd.SetFlags(flags)
	if err := flags.Parse([]string{"--allowed-host=GM0Z662Y.tail18d2f.ts.net", "--allowed-host=other.example"}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"gm0z662y.tail18d2f.ts.net", "other.example"}; !reflect.DeepEqual(cmd.allowedHosts, want) {
		t.Fatalf("allowedHosts = %v, want %v", cmd.allowedHosts, want)
	}
}

func TestHostCheckAllowedHosts(t *testing.T) {
	for _, host := range []string{"trusted.example", "trusted.example:443", "TRUSTED.EXAMPLE:8443", "localhost", "LOCALHOST:7333", "127.0.0.1:7333", "[::1]:7333", "evil.example", "trusted.example.evil", "sub.trusted.example"} {
		t.Run(host, func(t *testing.T) {
			h := hostCheck(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}), []string{"trusted.example"})
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Host = host
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			want := http.StatusNoContent
			if host == "evil.example" || host == "trusted.example.evil" || host == "sub.trusted.example" {
				want = http.StatusForbidden
			}
			if rec.Code != want {
				t.Fatalf("status = %d, want %d", rec.Code, want)
			}
		})
	}
}

// Exercise the actual origin patterns through both guards and the terminal
// handler. The proxy may preserve the public Host or rewrite it to loopback.
func TestAllowedHostProxyRequests(t *testing.T) {
	var cmd serveCmd
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	cmd.SetFlags(flags)
	if err := flags.Parse([]string{"--allowed-host=trusted.example"}); err != nil {
		t.Fatal(err)
	}
	patterns := cmd.originPatterns()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("POST /api/tasks", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.Handle("GET /api/sessions/{id}/ws", ws.NewHandler(session.NewManager(), patterns, false, nil))
	guarded := api.BrowserOriginGuard(mux, patterns, cmd.allowedHosts)
	// The direct route isolates websocket Origin verification from the API guard.
	directMux := http.NewServeMux()
	directMux.Handle("GET /direct/sessions/{id}/ws", ws.NewHandler(session.NewManager(), patterns, false, nil))
	directMux.Handle("/", guarded)
	srv := httptest.NewServer(hostCheck(directMux, cmd.allowedHosts))
	defer srv.Close()

	for _, host := range []string{"trusted.example", "127.0.0.1:7333"} {
		for _, origin := range []string{"https://trusted.example", "https://trusted.example:443", "https://TRUSTED.EXAMPLE:8443", "https://evil.example", "https://trusted.example.evil", "https://sub.trusted.example"} {
			t.Run(host+"/"+origin, func(t *testing.T) {
				allowed := origin == "https://trusted.example" || origin == "https://trusted.example:443" || origin == "https://TRUSTED.EXAMPLE:8443"
				for _, route := range []struct{ method, path string }{{"GET", "/"}, {"POST", "/api/tasks"}} {
					req, err := http.NewRequest(route.method, srv.URL+route.path, nil)
					if err != nil {
						t.Fatal(err)
					}
					req.Host = host
					req.Header.Set("Origin", origin)
					resp, err := srv.Client().Do(req)
					if err != nil {
						t.Fatal(err)
					}
					resp.Body.Close()
					want := http.StatusOK
					if route.method == "POST" {
						want = http.StatusForbidden
						if allowed {
							want = http.StatusNoContent
						}
					}
					if resp.StatusCode != want {
						t.Fatalf("%s status = %d, want %d", route.method, resp.StatusCode, want)
					}
				}

				for _, socketPath := range []string{"/api/sessions/missing/ws", "/direct/sessions/missing/ws"} {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					conn, resp, err := websocket.Dial(ctx, srv.URL+socketPath, &websocket.DialOptions{
						Host:       host,
						HTTPHeader: http.Header{"Origin": []string{origin}},
					})
					if allowed {
						if err != nil {
							t.Fatalf("upgrade rejected: %v", err)
						}
						defer conn.CloseNow()
						// A missing session closes after a successful upgrade, without
						// needing a real terminal process for this origin test.
						_, _, err = conn.Read(ctx)
						if websocket.CloseStatus(err) != ws.CloseSessionNotFound {
							t.Fatalf("close = %v, want session not found", err)
						}
					} else {
						if err == nil {
							conn.CloseNow()
							t.Fatal("unknown origin upgraded")
						}
						if resp == nil || resp.StatusCode != http.StatusForbidden {
							t.Fatalf("response = %v, want 403", resp)
						}
					}
				}
			})
		}
	}
}

func TestDefaultProxyOriginRules(t *testing.T) {
	cmd := serveCmd{addr: "127.0.0.1:7333"}
	guard := api.BrowserOriginGuard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), cmd.originPatterns(), cmd.allowedHosts)
	for _, tc := range []struct {
		origin string
		want   int
	}{
		{"http://localhost:7333", http.StatusNoContent},
		{"http://127.0.0.1:7333", http.StatusNoContent},
		{"http://localhost:5173", http.StatusForbidden},
		{"https://trusted.example", http.StatusForbidden},
	} {
		t.Run(tc.origin, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7333/api/tasks", nil)
			req.Header.Set("Origin", tc.origin)
			rec := httptest.NewRecorder()
			guard.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
