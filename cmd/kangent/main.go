package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"kangent/internal/api"
	"kangent/internal/session"
	"kangent/internal/settings"
	"kangent/internal/store"
	"kangent/internal/worktree"
	"kangent/internal/ws"
	"kangent/web"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:7333", "listen address (localhost-only by design)")
	dbFlag := flag.String("db", "~/.kangent/kangent.db", "path to SQLite database file")
	claudeBin := flag.String("claude-bin", "", "path to the claude binary (default: resolve \"claude\" on PATH at spawn time)")
	var devOrigins []string
	flag.Func("dev-origin", "additional allowed Origin host:port for the Vite dev server (repeatable, e.g. localhost:5173)", func(v string) error {
		devOrigins = append(devOrigins, v)
		return nil
	})
	flag.Parse()

	// D-21: localhost-only is enforced, not aspirational.
	if err := ensureLoopback(*addr); err != nil {
		slog.Error("refusing to start", "error", err)
		os.Exit(1)
	}

	dbPath, err := settings.ExpandHome(*dbFlag)
	if err != nil {
		slog.Error("resolving db path", "path", *dbFlag, "error", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		slog.Error("creating db directory", "dir", filepath.Dir(dbPath), "error", err)
		os.Exit(1)
	}

	db, err := store.Open(dbPath)
	if err != nil {
		slog.Error("opening database", "path", dbPath, "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := store.Migrate(db); err != nil {
		slog.Error("running migrations", "error", err)
		os.Exit(1)
	}

	// Per-instance hook token (STAT-02 / research gap #2): generated fresh on
	// every start, held in memory only — never logged, never persisted. It is
	// embedded solely in the per-spawn settings overlay and checked by the
	// hook receiver, so webpages firing no-CORS POSTs at localhost can never
	// spoof agent status.
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		slog.Error("generating hook token", "error", err)
		os.Exit(1)
	}
	hookToken := hex.EncodeToString(tokenBytes)

	// Origin allowlist (D-20): exact loopback origins with the serving port,
	// plus any --dev-origin entries (dev runs need the Vite server's origin:
	// go run ./cmd/kangent --dev-origin localhost:5173 --dev-origin 127.0.0.1:5173).
	_, port, _ := net.SplitHostPort(*addr)
	originPatterns := append([]string{"127.0.0.1:" + port, "localhost:" + port}, devOrigins...)

	// D-23: worktrees live centrally outside every repo tree.
	wtRoot, err := settings.ExpandHome("~/.kangent/worktrees")
	if err != nil {
		slog.Error("resolving worktree root", "error", err)
		os.Exit(1)
	}
	wtSvc := worktree.NewService(wtRoot)

	mgr := session.NewManager()
	mgr.SetAgentConfig(session.AgentConfig{
		// Hooks curl localhost; addr is loopback-enforced above, and a
		// "localhost" host also passes the port-agnostic hostCheck.
		BaseURL:   "http://" + *addr,
		Token:     hookToken,
		ClaudeBin: *claudeBin,
	})

	mux := http.NewServeMux()
	api.Routes(mux, db, wtSvc, mgr)
	api.SessionRoutes(mux, mgr, db)
	api.WorktreeRoutes(mux, db, wtSvc, mgr)
	api.DiffRoutes(mux, db, wtSvc)
	api.HookRoutes(mux, mgr, hookToken)
	api.AgentRoutes(mux, mgr, db)
	mux.Handle("GET /api/sessions/{id}/ws", ws.NewHandler(mgr, originPatterns))
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// SPA fallback: serve the embedded Vite build; deep links get index.html,
	// unknown /api/* paths 404 and never serve HTML.
	dist, _ := fs.Sub(web.DistFS, "dist")
	fileServer := http.FileServerFS(dist)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p != "" {
			if f, err := dist.Open(p); err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r) // hashed asset → cacheable
				return
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFileFS(w, r, dist, "index.html") // deep links → SPA
	})

	slog.Info("kangent listening", "url", "http://"+*addr)
	if err := http.ListenAndServe(*addr, hostCheck(mux)); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// ensureLoopback rejects any listen address that is not loopback (D-21).
// Empty host (":7333" = all interfaces) is refused, and non-"localhost"
// hostnames are refused outright — never resolved — so /etc/hosts games
// cannot widen the bind.
func ensureLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid --addr %q: %w", addr, err)
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("--addr %q is not a loopback address; kangent serves a shell and must stay local", addr)
	}
	return nil
}

// hostCheck is middleware wrapping the entire mux, rejecting non-loopback
// Host headers (DNS-rebinding defense). Hostname-only and PORT-AGNOSTIC by
// design: rebinding attacks present an attacker hostname, never a loopback
// name, and port-agnosticism keeps the Vite dev proxy (which forwards
// Host: 127.0.0.1:5173) working.
func hostCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h // strips the port, including the [::1]:7333 bracket form
		}
		switch host {
		case "localhost", "127.0.0.1", "::1", "[::1]":
			next.ServeHTTP(w, r)
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"forbidden host"}`))
		}
	})
}
