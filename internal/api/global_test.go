package api

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

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

// readGlobalRow reads the singleton's stored columns for before/after
// comparisons in the untouched-row asserts.
func readGlobalRow(t *testing.T, db *sql.DB) (string, sql.NullString, int64) {
	t.Helper()
	var rootPath string
	var repo sql.NullString
	var agentID int64
	err := db.QueryRow(`SELECT root_path, github_repo, agent_id FROM global_task WHERE id = 1`).
		Scan(&rootPath, &repo, &agentID)
	if err != nil {
		t.Fatalf("read global_task row: %v", err)
	}
	return rootPath, repo, agentID
}

// TestPutGlobalFolder (GCONF-01): a validated git dir persists as the root
// with github_repo NULL, root_exists true, and updated_at bumped.
func TestPutGlobalFolder(t *testing.T) {
	srv, _ := newGlobalTestServer(t)
	root := gitRepo(t)
	_, before := doJSON(t, "GET", srv.URL+"/api/global", nil)
	// updated_at has millisecond resolution — sleep past it so the bump is
	// deterministic, not a same-millisecond race.
	time.Sleep(10 * time.Millisecond)
	status, body := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"root_path": root})
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
	after, ok := body["updated_at"].(string)
	if !ok || after == "" {
		t.Fatalf("updated_at = %v, want non-empty string", body["updated_at"])
	}
	if before["updated_at"] == after {
		t.Errorf("updated_at not bumped: %v", after)
	}
}

// TestPutGlobalFootguns (D-07/D-26/D-27): the home dir (every spelling),
// /, ~/.kamacu itself and children, and a non-git dir are all rejected 400
// with the singleton row byte-identical before/after each reject. The
// footgun dirs are git-initialized so the D-07/D-27 branches themselves
// fire (a plain dir would stop at validateRepoPath's git check).
func TestPutGlobalFootguns(t *testing.T) {
	srv, db := newGlobalTestServer(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	for _, d := range []string{
		home,
		filepath.Join(home, ".kamacu"),
		filepath.Join(home, ".kamacu", "repos", "foo"),
	} {
		if out, err := exec.Command("git", "init", d).CombinedOutput(); err != nil {
			t.Fatalf("git init %s: %v\n%s", d, err, out)
		}
	}
	plain := t.TempDir() // exists, but not a git repo
	cases := []struct{ name, path string }{
		{"home tilde spelling", "~"},
		{"home tilde-slash spelling", "~/"},
		{"home absolute spelling", home},
		{"filesystem root", "/"},
		{"kamacu base", filepath.Join(home, ".kamacu")},
		{"kamacu child", filepath.Join(home, ".kamacu", "repos", "foo")},
		{"non-git dir", plain},
	}
	beforeRoot, beforeRepo, beforeAgent := readGlobalRow(t, db)
	for _, tc := range cases {
		status, body := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"root_path": tc.path})
		if status != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400; body=%v", tc.name, status, body)
		}
		afterRoot, afterRepo, afterAgent := readGlobalRow(t, db)
		if afterRoot != beforeRoot || afterRepo != beforeRepo || afterAgent != beforeAgent {
			t.Errorf("%s: singleton row mutated on a 400: root %q→%q repo %v→%v agent %d→%d",
				tc.name, beforeRoot, afterRoot, beforeRepo, afterRepo, beforeAgent, afterAgent)
		}
	}
}

// TestPutGlobalClear (D-21): explicit root_path:"" resets both root columns
// (root_path ” AND github_repo NULL) behind the same gate as any change.
func TestPutGlobalClear(t *testing.T) {
	srv, _ := newGlobalTestServer(t)
	root := gitRepo(t)
	if status, body := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"root_path": root}); status != http.StatusOK {
		t.Fatalf("seed folder root: status = %d; body=%v", status, body)
	}
	status, body := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"root_path": ""})
	if status != http.StatusOK {
		t.Fatalf("clear: status = %d, want 200; body=%v", status, body)
	}
	status, body = doJSON(t, "GET", srv.URL+"/api/global", nil)
	if status != http.StatusOK {
		t.Fatalf("GET after clear: status = %d", status)
	}
	if got := body["root_path"]; got != "" {
		t.Errorf("root_path = %v, want \"\" after clear", got)
	}
	if got, ok := body["github_repo"]; !ok || got != nil {
		t.Errorf("github_repo = %v (present=%v), want null after clear", got, ok)
	}
	if got, ok := body["root_exists"]; !ok || got != false {
		t.Errorf("root_exists = %v (present=%v), want false after clear", got, ok)
	}
}

// TestPutGlobalAgent (GCONF-03/D-24): agent_id sets the default agent (the
// response's agent summary reflects it immediately) and an unknown id is a
// 400 "agent not found" with no gate.
func TestPutGlobalAgent(t *testing.T) {
	srv, db := newGlobalTestServer(t)
	// 00015 already seeds the OpenCode system agent — insert a third,
	// distinctly-named agent so the default-agent switch is observable.
	res, err := db.Exec(`INSERT INTO agents (name, command, engine) VALUES ('Codex CLI', 'codex', 'codex')`)
	if err != nil {
		t.Fatalf("insert second agent: %v", err)
	}
	newID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	status, body := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"agent_id": newID})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", status, body)
	}
	agent, ok := body["agent"].(map[string]any)
	if !ok {
		t.Fatalf("agent missing or not an object: %v", body["agent"])
	}
	if got, ok := agent["id"].(float64); !ok || int64(got) != newID {
		t.Errorf("agent.id = %v, want %d", agent["id"], newID)
	}
	if got := agent["engine"]; got != "codex" {
		t.Errorf("agent.engine = %v, want codex", got)
	}
	status, body = doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"agent_id": 99999})
	if status != http.StatusBadRequest {
		t.Fatalf("unknown agent: status = %d, want 400; body=%v", status, body)
	}
	if got := body["error"]; got != "agent not found" {
		t.Errorf("error = %v, want %q", got, "agent not found")
	}
}

// TestPutGlobalEmptyBody: PUT {} is the workspaces-precedent 400.
func TestPutGlobalEmptyBody(t *testing.T) {
	srv, _ := newGlobalTestServer(t)
	status, body := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%v", status, body)
	}
	if got := body["error"]; got != "nothing to update" {
		t.Errorf("error = %v, want %q", got, "nothing to update")
	}
}

// TestPutGlobalRootChangeClearsResumeIDs (D-16/D-25): ANY successful root
// change clears both resume ids in the same UPDATE (no no-op-re-PUT
// exception); an agent-only PUT leaves them set — D-16 is the only
// clearing path.
func TestPutGlobalRootChangeClearsResumeIDs(t *testing.T) {
	srv, db := newGlobalTestServer(t)
	root := gitRepo(t)
	seedIDs := func() {
		if _, err := db.Exec(`UPDATE global_task SET claude_session_id = 'c1', opencode_session_id = 'o1'`); err != nil {
			t.Fatalf("seed resume ids: %v", err)
		}
	}
	readIDs := func() (claude, opencode sql.NullString) {
		if err := db.QueryRow(`SELECT claude_session_id, opencode_session_id FROM global_task WHERE id = 1`).Scan(&claude, &opencode); err != nil {
			t.Fatalf("read resume ids: %v", err)
		}
		return
	}
	seedIDs()
	if status, body := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"root_path": root}); status != http.StatusOK {
		t.Fatalf("root PUT: status = %d; body=%v", status, body)
	}
	c, o := readIDs()
	if c.Valid || o.Valid {
		t.Errorf("resume ids must be NULL after a root change, got claude=%v opencode=%v", c, o)
	}
	seedIDs()
	if status, body := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"agent_id": 1}); status != http.StatusOK {
		t.Fatalf("agent-only PUT: status = %d; body=%v", status, body)
	}
	c, o = readIDs()
	if !c.Valid || c.String != "c1" || !o.Valid || o.String != "o1" {
		t.Errorf("resume ids must STILL be set after an agent-only PUT (D-25), got claude=%v opencode=%v", c, o)
	}
}
