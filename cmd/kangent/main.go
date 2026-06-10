package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"kangent/internal/api"
	"kangent/internal/store"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:7333", "listen address (localhost-only by design)")
	dbFlag := flag.String("db", "~/.kangent/kangent.db", "path to SQLite database file")
	flag.Parse()

	dbPath, err := expandHome(*dbFlag)
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

	mux := http.NewServeMux()
	api.Routes(mux, db)
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	slog.Info("kangent listening", "url", "http://"+*addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// expandHome resolves a leading "~" or "~/" to the current user's home directory.
func expandHome(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, strings.TrimPrefix(p, "~")), nil
	}
	return p, nil
}
