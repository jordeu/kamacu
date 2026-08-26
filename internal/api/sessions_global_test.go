package api

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kamacu/internal/session"
	"kamacu/internal/settings"
	"kamacu/internal/store"
	"kamacu/internal/tmux"
	"kamacu/internal/worktree"
)

// newGlobalSessionServerWithTmux wires the FULL surface the global-session
// suite needs over one DB + Manager: task Routes (so worktreeTask provisions
// real tasks for the cross-scope direction), SessionRoutes (the spawn path
// under test), and GlobalRoutes (the PUT /api/global seeding step) — the
// production shape Phase 15 spawns against. HOME-sandboxed like
// newGlobalTestServer so PUT /api/global path validation (ExpandHome + the
// D-27 ~/.kamacu block) resolves into the sandbox.
func newGlobalSessionServerWithTmux(t *testing.T, c tmux.Client) (*httptest.Server, *session.Manager, *sql.DB) {
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
	wtDir := t.TempDir()
	if err := settings.Set(db, settings.KeyWorktreeBase, wtDir); err != nil {
		t.Fatalf("seed worktree_base: %v", err)
	}
	wt := worktree.NewService(wtDir)
	mgr := session.NewManager()
	mgr.SetTmuxClient(c)
	mux := http.NewServeMux()
	Routes(mux, db, wt, mgr, c)
	SessionRoutes(mux, mgr, db, c)
	GlobalRoutes(mux, db, mgr, c)
	srv := httptest.NewServer(mux)
	t.Cleanup(func() {
		srv.Close()
		for _, info := range mgr.List() {
			if s, ok := mgr.Get(info.ID); ok {
				s.Stop()
			}
		}
		db.Close()
	})
	return srv, mgr, db
}

// newGlobalSessionServer is the CI shape: no tmux client wired, so tmux
// spawns fail at LookPath — exactly what the gate tests want (the gates fire
// before any binary resolves).
func newGlobalSessionServer(t *testing.T) (*httptest.Server, *session.Manager, *sql.DB) {
	t.Helper()
	return newGlobalSessionServerWithTmux(t, tmux.Client{})
}

// putGlobalFolderRoot seeds a folder root through the REAL PUT (validateRepoPath
// included, mirroring the Phase-14 flow the spawn gates sit behind) and returns
// the canonical stored root path.
func putGlobalFolderRoot(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	status, body := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"root_path": gitRepo(t)})
	if status != http.StatusOK {
		t.Fatalf("PUT /api/global folder root: status = %d; body=%v", status, body)
	}
	root, _ := body["root_path"].(string)
	if root == "" {
		t.Fatalf("PUT /api/global returned empty root_path: %v", body)
	}
	return root
}

// seedShellSetting writes the shell setting row directly (the settings.Set
// upsert without the AllowedShells gate) so the D-30 tests prove the GATE
// placement — the root gates fire before the shell is ever read — on hosts
// without the tmux binary.
func seedShellSetting(t *testing.T, db *sql.DB, shell string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO settings(key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET
		   value = excluded.value,
		   updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
		settings.KeyShell, shell); err != nil {
		t.Fatalf("seed shell setting: %v", err)
	}
}

// TestGlobalSessionUnconfiguredRoot409 (D-28/D-30): scope:"global" with no
// root configured is an honest 409 for BOTH the plain-bash and the tmux
// variant — never a silent home/cwd fallback.
func TestGlobalSessionUnconfiguredRoot409(t *testing.T) {
	srv, mgr, db := newGlobalSessionServer(t)

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "bash"})
	if status != http.StatusConflict {
		t.Fatalf("plain bash: status = %d, want 409; body=%v", status, body)
	}
	if body["error"] != "global root not configured" {
		t.Errorf("plain bash error = %q, want %q", body["error"], "global root not configured")
	}

	// The tmux variant hits the SAME gate (D-30): the root gates run before
	// the shell setting is ever read, so no tmux binary is needed here.
	seedShellSetting(t, db, "tmux")
	status, body = doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "bash"})
	if status != http.StatusConflict {
		t.Fatalf("tmux variant: status = %d, want 409; body=%v", status, body)
	}
	if body["error"] != "global root not configured" {
		t.Errorf("tmux variant error = %q, want %q", body["error"], "global root not configured")
	}

	// The 409 spawned nothing.
	if got := len(mgr.List()); got != 0 {
		t.Errorf("mgr.List() = %d sessions after a gated spawn, want 0", got)
	}
}

// TestGlobalSessionVanishedRoot409 (D-29/D-30): a root configured via PUT
// whose directory was deleted since is an honest 409 carrying the stored path
// VERBATIM — for bash AND the tmux variant.
func TestGlobalSessionVanishedRoot409(t *testing.T) {
	srv, mgr, db := newGlobalSessionServer(t)
	root := putGlobalFolderRoot(t, srv)
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("remove root: %v", err)
	}

	want := "global root no longer exists on disk: " + root
	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "bash"})
	if status != http.StatusConflict {
		t.Fatalf("plain bash: status = %d, want 409; body=%v", status, body)
	}
	if body["error"] != want {
		t.Errorf("plain bash error = %q, want %q (path verbatim)", body["error"], want)
	}

	seedShellSetting(t, db, "tmux")
	status, body = doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "bash"})
	if status != http.StatusConflict {
		t.Fatalf("tmux variant: status = %d, want 409; body=%v", status, body)
	}
	if body["error"] != want {
		t.Errorf("tmux variant error = %q, want %q (path verbatim)", body["error"], want)
	}
	if got := len(mgr.List()); got != 0 {
		t.Errorf("mgr.List() = %d sessions after a gated spawn, want 0", got)
	}
}

// TestGlobalSessionScopeValidation (T-15-05): scope is a closed set — "weird"
// is a 400 in the invalid-kind family; scope together with task_id is the
// mutual-exclusion 400 (the global.go repo/root_path family).
func TestGlobalSessionScopeValidation(t *testing.T) {
	srv, _, _ := newGlobalSessionServer(t)

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "weird"})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid scope: status = %d, want 400; body=%v", status, body)
	}
	if body["error"] != "invalid scope" {
		t.Errorf("error = %q, want %q", body["error"], "invalid scope")
	}

	status, body = doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "task_id": 5})
	if status != http.StatusBadRequest {
		t.Fatalf("scope+task_id: status = %d, want 400; body=%v", status, body)
	}
	if body["error"] != "supply either scope or task_id, not both" {
		t.Errorf("error = %q, want %q", body["error"], "supply either scope or task_id, not both")
	}
}

// TestGlobalSessionPlainBashSpawn (GVIEW-03): a configured root + plain bash
// spawns 201 with the global "Bash N" label family, global:true on the wire,
// and the shell really running in the global root — proven end-to-end by
// driving the real input endpoint to write pwd to a file (only the shell's
// own $PWD expansion can satisfy the assertion).
func TestGlobalSessionPlainBashSpawn(t *testing.T) {
	srv, mgr, _ := newGlobalSessionServer(t)
	root := putGlobalFolderRoot(t, srv)

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "bash"})
	if status != http.StatusCreated {
		t.Fatalf("spawn: status = %d, want 201; body=%v", status, body)
	}
	if body["label"] != "Bash 1" {
		t.Errorf("label = %q, want %q (global family, never the dev bash #N)", body["label"], "Bash 1")
	}
	if body["global"] != true {
		t.Errorf("global = %v, want true on the wire", body["global"])
	}
	sid, _ := body["id"].(string)
	if sid == "" {
		t.Fatalf("spawn returned empty id: %v", body)
	}
	if _, ok := mgr.Get(sid); !ok {
		t.Fatalf("session %q not in manager", sid)
	}

	// cwd proof: pwd written to a file via the real input endpoint. bash
	// reports the physical cwd, so compare against the symlink-resolved root.
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", root, err)
	}
	marker := filepath.Join(t.TempDir(), "pwd.txt")
	istatus, ibody := doJSON(t, "POST", srv.URL+"/api/sessions/"+sid+"/input", map[string]any{"message": "pwd > " + marker})
	if istatus != http.StatusOK {
		t.Fatalf("input: status = %d; body=%v", istatus, ibody)
	}
	var wrote string
	deadline := time.Now().Add(5 * time.Second)
	for {
		b, rerr := os.ReadFile(marker)
		if rerr == nil {
			wrote = strings.TrimSpace(string(b))
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("pwd file never appeared at %s (cwd proof failed)", marker)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if wrote != resolved {
		t.Errorf("shell cwd = %q, want the global root %q", wrote, resolved)
	}

	// Second global spawn: "Bash 2" from the same global family.
	status, body = doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "bash"})
	if status != http.StatusCreated {
		t.Fatalf("second spawn: status = %d, want 201; body=%v", status, body)
	}
	if body["label"] != "Bash 2" {
		t.Errorf("second label = %q, want %q", body["label"], "Bash 2")
	}
}

// TestGlobalSessionTmuxMint (D-11/D-02, tmux-guarded): shell=tmux + global
// scope mints kamacu-global-<n> from the scope-scoped counter with a
// (NULL task_id, scope 'global') row — the 00018 XOR CHECK shape — and a
// real tmux session on the dedicated socket.
func TestGlobalSessionTmuxMint(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	// Per-test socket + kill-server cleanup registered BEFORE the server
	// (LIFO: runs after the harness teardown).
	c := tmux.Client{Socket: fmt.Sprintf("ktest-gmint-%d", os.Getpid()), ConfPath: "/dev/null"}
	t.Cleanup(func() { _ = c.KillServer(context.Background()) })
	srv, _, db := newGlobalSessionServerWithTmux(t, c)
	putGlobalFolderRoot(t, srv)

	// Valid via Set because tmux IS on PATH (AllowedShells re-validates).
	if err := settings.Set(db, settings.KeyShell, "tmux"); err != nil {
		t.Fatalf("set shell=tmux: %v", err)
	}

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "bash"})
	if status != http.StatusCreated {
		t.Fatalf("spawn: status = %d, want 201; body=%v", status, body)
	}
	if body["label"] != "Bash 1" {
		t.Errorf("label = %q, want %q", body["label"], "Bash 1")
	}
	var (
		taskID sql.NullInt64
		scope  string
		n      int64
		name   string
		label  string
	)
	if err := db.QueryRow(`SELECT task_id, scope, n, name, label FROM tmux_sessions WHERE n = 1`).Scan(&taskID, &scope, &n, &name, &label); err != nil {
		t.Fatalf("read minted row: %v", err)
	}
	if taskID.Valid {
		t.Errorf("task_id = %v, want NULL (global rows are task-less)", taskID.Int64)
	}
	if scope != "global" {
		t.Errorf("scope = %q, want %q (the 00018 XOR CHECK shape)", scope, "global")
	}
	if name != "kamacu-global-1" {
		t.Errorf("name = %q, want %q", name, "kamacu-global-1")
	}
	if label != "Bash 1" {
		t.Errorf("label = %q, want %q (persisted at INSERT, D-04)", label, "Bash 1")
	}
	awaitHasSession(t, c, "kamacu-global-1", true)

	// Second mint: kamacu-global-2 / "Bash 2" (the scope-scoped counter).
	status, body = doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global"})
	if status != http.StatusCreated {
		t.Fatalf("second spawn: status = %d, want 201; body=%v", status, body)
	}
	if body["label"] != "Bash 2" {
		t.Errorf("second label = %q, want %q", body["label"], "Bash 2")
	}
	var cnt int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tmux_sessions WHERE name = 'kamacu-global-2'`).Scan(&cnt); err != nil {
		t.Fatalf("count kamacu-global-2: %v", err)
	}
	if cnt != 1 {
		t.Errorf("rows for kamacu-global-2 = %d, want 1", cnt)
	}
	awaitHasSession(t, c, "kamacu-global-2", true)
}

// TestGlobalSessionTmuxReattach (D-32 happy half, tmux-guarded): reattach by
// the minted name returns 201 KEEPING the persisted label and mints nothing —
// the global analog of the task survivor flow (persisted row + live detached
// tmux session + no in-memory session).
func TestGlobalSessionTmuxReattach(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	c := tmux.Client{Socket: fmt.Sprintf("ktest-greattach-%d", os.Getpid()), ConfPath: "/dev/null"}
	t.Cleanup(func() { _ = c.KillServer(context.Background()) })
	srv, mgr, db := newGlobalSessionServerWithTmux(t, c)
	root := putGlobalFolderRoot(t, srv)

	// Post-restart survivor shape: a persisted global row plus a live
	// detached tmux session under that exact name, NO in-memory session.
	const name = "kamacu-global-1"
	if _, err := db.Exec(`INSERT INTO tmux_sessions (scope, n, name, label) VALUES ('global', 1, ?, 'Bash 1')`, name); err != nil {
		t.Fatalf("seed global tmux_sessions row: %v", err)
	}
	detach := append(c.BaseArgs(), "new-session", "-d", "-s", name, "-c", root)
	if err := exec.Command("tmux", detach...).Run(); err != nil {
		t.Fatalf("seed surviving global tmux session: %v", err)
	}
	awaitHasSession(t, c, name, true)

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "reattach_tmux_name": name})
	if status != http.StatusCreated {
		t.Fatalf("reattach: status = %d, want 201; body=%v", status, body)
	}
	if body["label"] != "Bash 1" {
		t.Errorf("reattach label = %q, want the persisted %q", body["label"], "Bash 1")
	}
	sid, _ := body["id"].(string)
	if sid == "" {
		t.Fatalf("reattach returned empty id: %v", body)
	}
	if _, ok := mgr.Get(sid); !ok {
		t.Fatalf("reattach session %q not in manager — WS could not attach", sid)
	}

	// No duplicate row: still exactly one row for this name.
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tmux_sessions WHERE name = ?`, name).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Errorf("tmux_sessions rows for %q = %d, want 1 (no mint on reattach)", name, count)
	}

	// An unknown global name → the honest 404.
	status, body = doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "reattach_tmux_name": "kamacu-global-99"})
	if status != http.StatusNotFound {
		t.Fatalf("reattach unknown name: status = %d, want 404; body=%v", status, body)
	}
	if body["error"] != "no session to reattach" {
		t.Errorf("error = %q, want %q", body["error"], "no session to reattach")
	}

	// Clean up the in-memory session before the harness tears down.
	if s, ok := mgr.Get(sid); ok {
		s.Stop()
	}
	awaitHasSession(t, c, name, false)
}

// TestGlobalSessionCrossScopeReattach404 (D-32, CI): the scoped lookup makes
// a foreign-scope row invisible in BOTH directions — the same honest 404, no
// scope-mismatch oracle, no information leak. Pure SQL before Spawn, so no
// tmux binary is needed.
func TestGlobalSessionCrossScopeReattach404(t *testing.T) {
	srv, mgr, db := newGlobalSessionServer(t)
	putGlobalFolderRoot(t, srv)
	id, _ := worktreeTask(t, srv, "Cross Scope")

	// One row per scope: a task row and a global row.
	taskName := fmt.Sprintf("kamacu-%d-1", id)
	if _, err := db.Exec(`INSERT INTO tmux_sessions (task_id, scope, n, name, label) VALUES (?, 'task', 1, ?, 'Bash 1')`, id, taskName); err != nil {
		t.Fatalf("seed task row: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO tmux_sessions (scope, n, name, label) VALUES ('global', 1, 'kamacu-global-1', 'Bash 1')`); err != nil {
		t.Fatalf("seed global row: %v", err)
	}

	// Task scope cannot see the global row.
	status, body := doJSON(t, "POST", srv.URL+"/api/sessions",
		map[string]any{"task_id": id, "reattach_tmux_name": "kamacu-global-1"})
	if status != http.StatusNotFound {
		t.Fatalf("task→global: status = %d, want 404; body=%v", status, body)
	}
	if body["error"] != "no session to reattach" {
		t.Errorf("task→global error = %q, want %q", body["error"], "no session to reattach")
	}

	// Global scope cannot see the task row.
	status, body = doJSON(t, "POST", srv.URL+"/api/sessions",
		map[string]any{"scope": "global", "reattach_tmux_name": taskName})
	if status != http.StatusNotFound {
		t.Fatalf("global→task: status = %d, want 404; body=%v", status, body)
	}
	if body["error"] != "no session to reattach" {
		t.Errorf("global→task error = %q, want %q", body["error"], "no session to reattach")
	}

	// Neither direction spawned anything.
	if got := len(mgr.List()); got != 0 {
		t.Errorf("mgr.List() = %d sessions after cross-scope 404s, want 0", got)
	}
}
