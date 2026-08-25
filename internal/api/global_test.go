package api

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"kamacu/internal/session"
	"kamacu/internal/store"
	"kamacu/internal/tmux"
)

// newGlobalTestServer builds a HOME-sandboxed server with the global routes
// registered (the newRepoTestServer posture: HOME points at a temp dir so
// settings.ExpandHome and the D-27 ~/.kamacu block resolve into the sandbox,
// and the managed namespace ~/.kamacu/repos/global/... lands under it).
// Migration 00013 seeds the default agent and 00017 seeds the singleton
// in-migration, so a fresh migrated DB needs no backfill calls.
func newGlobalTestServer(t *testing.T) (*httptest.Server, *sql.DB) {
	t.Helper()
	return newGlobalTestServerWithTmux(t, tmux.Client{})
}

// newGlobalTestServerWithTmux is newGlobalTestServer with an explicit tmux
// client, so the 409-gate tests (plan 14-02) can wire a real per-test
// socket (mirrors newWorktreeServerWithTmux in worktrees_test.go).
func newGlobalTestServerWithTmux(t *testing.T, c tmux.Client) (*httptest.Server, *sql.DB) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home) // ExpandHome resolves ~ via os.UserHomeDir → HOME
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}
	mgr := session.NewManager()
	mgr.SetTmuxClient(c)
	mux := http.NewServeMux()
	GlobalRoutes(mux, db, mgr, c)
	srv := httptest.NewServer(mux)
	t.Cleanup(func() {
		srv.Close()
		db.Close()
	})
	return srv, db
}

// TestGetGlobalUnconfigured (D-17/D-18/D-19): a fresh migrated DB serves
// the honest unconfigured state — empty root, null github_repo, root_exists
// false, live counts zero, agent summary from the is_default seed — and the
// resume ids NEVER appear on the wire.
func TestGetGlobalUnconfigured(t *testing.T) {
	srv, db := newGlobalTestServer(t)
	// The agent the singleton was seeded from (00017 reads the is_default
	// flag — never a literal id, so resolve it the same way).
	var defAgentID int64
	if err := db.QueryRow(`SELECT id FROM agents WHERE is_default = 1`).Scan(&defAgentID); err != nil {
		t.Fatalf("read default agent: %v", err)
	}
	status, body := doJSON(t, "GET", srv.URL+"/api/global", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", status, body)
	}
	if got := body["root_path"]; got != "" {
		t.Errorf("root_path = %v, want \"\"", got)
	}
	if got, ok := body["github_repo"]; !ok || got != nil {
		t.Errorf("github_repo = %v (present=%v), want JSON null", got, ok)
	}
	if got, ok := body["root_exists"]; !ok || got != false {
		t.Errorf("root_exists = %v (present=%v), want false", got, ok)
	}
	live, ok := body["live"].(map[string]any)
	if !ok {
		t.Fatalf("live missing or not an object: %v", body["live"])
	}
	for _, k := range []string{"agent", "bash", "tmux"} {
		if v, ok := live[k].(float64); !ok || v != 0 {
			t.Errorf("live.%s = %v, want 0", k, live[k])
		}
	}
	agent, ok := body["agent"].(map[string]any)
	if !ok {
		t.Fatalf("agent missing or not an object: %v", body["agent"])
	}
	if got, ok := agent["id"].(float64); !ok || int64(got) != defAgentID {
		t.Errorf("agent.id = %v, want %d", agent["id"], defAgentID)
	}
	if got := agent["engine"]; got != "claude" {
		t.Errorf("agent.engine = %v, want claude", got)
	}
	if got, ok := body["updated_at"].(string); !ok || got == "" {
		t.Errorf("updated_at = %v, want non-empty string", body["updated_at"])
	}
	// D-18 wire assertion: the resume ids are spawn-path internals — they
	// must never serialize on any response of this endpoint.
	for _, key := range []string{"claude_session_id", "opencode_session_id"} {
		if _, present := body[key]; present {
			t.Errorf("response must not contain %q (D-18), got %v", key, body[key])
		}
	}
}

// TestGetGlobalConfiguredRoot (D-17 + D-08 complement): a stored folder root
// reports root_exists true with github_repo still null, and a vanished root
// is reported honestly (root_exists false) with no revalidation.
func TestGetGlobalConfiguredRoot(t *testing.T) {
	srv, db := newGlobalTestServer(t)
	root := gitRepo(t)
	if _, err := db.Exec(`UPDATE global_task SET root_path = ?`, root); err != nil {
		t.Fatalf("seed root_path: %v", err)
	}
	status, body := doJSON(t, "GET", srv.URL+"/api/global", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", status, body)
	}
	if got := body["root_path"]; got != root {
		t.Errorf("root_path = %v, want %s", got, root)
	}
	if got, ok := body["github_repo"]; !ok || got != nil {
		t.Errorf("github_repo = %v (present=%v), want null for a folder root", got, ok)
	}
	if got, ok := body["root_exists"]; !ok || got != true {
		t.Errorf("root_exists = %v (present=%v), want true", got, ok)
	}
	// A vanished root fails honestly at read time — os.Stat, no repair.
	gone := filepath.Join(t.TempDir(), "vanished")
	if _, err := db.Exec(`UPDATE global_task SET root_path = ?`, gone); err != nil {
		t.Fatalf("seed vanished root_path: %v", err)
	}
	status, body = doJSON(t, "GET", srv.URL+"/api/global", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if got, ok := body["root_exists"]; !ok || got != false {
		t.Errorf("root_exists = %v (present=%v), want false for a vanished root", got, ok)
	}
}
