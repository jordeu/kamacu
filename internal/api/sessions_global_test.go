package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

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
	deadline := time.Now().Add(8 * time.Second) // D-14: interactive bash ignores SIGTERM; exit lands after the 5s grace SIGKILL
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

// TestGlobalSessionListScopedLabels (GINT-02, D-09/D-10): ?scope=global
// returns exactly the global sessions with the honest synthesized labels —
// taskTitle "Scratchpad", projectName "Global", agentName from the singleton
// agent JOIN. The unscoped list (what MCP list_sessions rides) includes the
// same global entry with the same labels, while a TaskID-0 DEV session in
// that same list keeps EMPTY join fields — the Scratchpad labels never leak
// onto dev sessions (Pitfall 3: no 0-keyed ctxByTask entry).
func TestGlobalSessionListScopedLabels(t *testing.T) {
	srv, mgr, _ := newGlobalSessionServer(t)
	putGlobalFolderRoot(t, srv)

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "bash"})
	if status != http.StatusCreated {
		t.Fatalf("global spawn: status = %d; body=%v", status, body)
	}
	gsid, _ := body["id"].(string)

	// A dev session in the same registry: TaskID 0, NOT global.
	status, body = doJSON(t, "POST", srv.URL+"/api/sessions", nil)
	if status != http.StatusCreated {
		t.Fatalf("dev spawn: status = %d; body=%v", status, body)
	}
	dsid, _ := body["id"].(string)
	t.Cleanup(func() {
		for _, id := range []string{gsid, dsid} {
			if s, ok := mgr.Get(id); ok {
				s.Stop()
			}
		}
	})

	// Scoped list: exactly the global session, labeled Scratchpad/Global +
	// the default agent's name (the 00013 seed: "claude").
	status, raw := getJSON(t, srv.URL+"/api/sessions?scope=global")
	if status != http.StatusOK {
		t.Fatalf("scoped list: status = %d; body=%s", status, raw)
	}
	var scoped []map[string]any
	if err := json.Unmarshal(raw, &scoped); err != nil {
		t.Fatalf("decode scoped list: %v", err)
	}
	if len(scoped) != 1 {
		t.Fatalf("scoped list has %d entries, want exactly 1 (the global session): %s", len(scoped), raw)
	}
	if scoped[0]["id"] != gsid {
		t.Errorf("scoped entry id = %v, want the global session %q", scoped[0]["id"], gsid)
	}
	if scoped[0]["taskTitle"] != "Scratchpad" {
		t.Errorf("taskTitle = %v, want %q", scoped[0]["taskTitle"], "Scratchpad")
	}
	if scoped[0]["projectName"] != "Global" {
		t.Errorf("projectName = %v, want %q", scoped[0]["projectName"], "Global")
	}
	if scoped[0]["agentName"] != "Claude Code" {
		t.Errorf("agentName = %v, want the singleton agent's name %q", scoped[0]["agentName"], "Claude Code")
	}
	if scoped[0]["global"] != true {
		t.Errorf("global = %v, want true", scoped[0]["global"])
	}

	// Unscoped list (MCP rides this): the global entry keeps its honest
	// labels; the dev session keeps EMPTY join fields (Pitfall 3).
	status, raw = getJSON(t, srv.URL+"/api/sessions")
	if status != http.StatusOK {
		t.Fatalf("unscoped list: status = %d; body=%s", status, raw)
	}
	var all []map[string]any
	if err := json.Unmarshal(raw, &all); err != nil {
		t.Fatalf("decode unscoped list: %v", err)
	}
	var gEntry, dEntry map[string]any
	for _, e := range all {
		switch e["id"] {
		case gsid:
			gEntry = e
		case dsid:
			dEntry = e
		}
	}
	if gEntry == nil || dEntry == nil {
		t.Fatalf("unscoped list must contain both sessions; got %s", raw)
	}
	if gEntry["taskTitle"] != "Scratchpad" || gEntry["projectName"] != "Global" {
		t.Errorf("unscoped global labels = %v/%v, want Scratchpad/Global", gEntry["taskTitle"], gEntry["projectName"])
	}
	for _, key := range []string{"taskTitle", "projectName", "agentName"} {
		if v, ok := dEntry[key]; ok && v != "" {
			t.Errorf("dev session %s = %v, want EMPTY (no 0-keyed label leak, Pitfall 3)", key, v)
		}
	}
}

// TestGlobalSessionLiveCountsAndRootGate (D-19, GCONF-04 bite, Pitfall 4):
// after a global plain-bash spawn, GET /api/global reports live.bash ≥ 1 and
// live.agent 0 (fields disjoint by name — a tmux tab counts only under
// live.tmux); a root-change PUT while it runs is a 409 whose reasons include
// the live session; after stopping it the PUT succeeds.
func TestGlobalSessionLiveCountsAndRootGate(t *testing.T) {
	srv, mgr, _ := newGlobalSessionServer(t)
	putGlobalFolderRoot(t, srv)

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "bash"})
	if status != http.StatusCreated {
		t.Fatalf("spawn: status = %d; body=%v", status, body)
	}
	sid, _ := body["id"].(string)
	t.Cleanup(func() {
		if s, ok := mgr.Get(sid); ok {
			s.Stop()
		}
	})

	live := func() map[string]any {
		t.Helper()
		status, raw := getJSON(t, srv.URL+"/api/global")
		if status != http.StatusOK {
			t.Fatalf("GET /api/global: status = %d; body=%s", status, raw)
		}
		var g map[string]any
		if err := json.Unmarshal(raw, &g); err != nil {
			t.Fatalf("decode global: %v", err)
		}
		l, _ := g["live"].(map[string]any)
		if l == nil {
			t.Fatalf("global response has no live block: %s", raw)
		}
		return l
	}

	l := live()
	if l["bash"].(float64) < 1 {
		t.Errorf("live.bash = %v, want ≥ 1 while the global bash session runs", l["bash"])
	}
	if l["agent"].(float64) != 0 {
		t.Errorf("live.agent = %v, want 0 (no global agent spawned)", l["agent"])
	}

	// Root change while RUNNING: 409 with reasons including the session.
	newRoot := gitRepo(t)
	status, body = doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"root_path": newRoot})
	if status != http.StatusConflict {
		t.Fatalf("root change while running: status = %d, want 409; body=%v", status, body)
	}
	reasons, _ := body["reasons"].([]any)
	if len(reasons) == 0 {
		t.Fatalf("409 carries no reasons: %v", body)
	}

	// Stop, wait for exited, then the PUT succeeds.
	sstatus, _ := doJSON(t, "POST", srv.URL+"/api/sessions/"+sid+"/stop", nil)
	if sstatus != http.StatusAccepted && sstatus != http.StatusOK {
		t.Fatalf("stop: status = %d", sstatus)
	}
	deadline := time.Now().Add(8 * time.Second) // D-14: interactive bash ignores SIGTERM; exit lands after the 5s grace SIGKILL
	for {
		if s, ok := mgr.Get(sid); ok && s.Info().Status == session.StatusExited {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("session %q never exited after stop", sid)
		}
		time.Sleep(50 * time.Millisecond)
	}
	status, body = doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"root_path": newRoot})
	if status != http.StatusOK {
		t.Fatalf("root change after stop: status = %d, want 200; body=%v", status, body)
	}
	if body["root_path"] != newRoot {
		t.Errorf("root_path = %v, want %q", body["root_path"], newRoot)
	}
}

// TestGlobalSessionRestartOrphan (GSESS-03, tmux-guarded): restart-sim — a
// FRESH Manager over the same DB, a persisted global tmux row with a live
// detached tmux session and no in-memory session. ?scope=global surfaces it
// as an orphaned entry with orphaned:true + global:true + the re-derived
// label, and GET /api/global counts it under live.tmux only.
func TestGlobalSessionRestartOrphan(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	c := tmux.Client{Socket: fmt.Sprintf("ktest-grst-%d", os.Getpid()), ConfPath: "/dev/null"}
	t.Cleanup(func() { _ = c.KillServer(context.Background()) })
	srv, _, db := newGlobalSessionServerWithTmux(t, c)
	root := putGlobalFolderRoot(t, srv)

	const name = "kamacu-global-7"
	if _, err := db.Exec(`INSERT INTO tmux_sessions (scope, n, name, label) VALUES ('global', 7, ?, 'Bash 7')`, name); err != nil {
		t.Fatalf("seed global row: %v", err)
	}
	detach := append(c.BaseArgs(), "new-session", "-d", "-s", name, "-c", root)
	if err := exec.Command("tmux", detach...).Run(); err != nil {
		t.Fatalf("seed surviving tmux session: %v", err)
	}
	awaitHasSession(t, c, name, true)

	// The restart: a fresh Manager + fresh routes over the SAME DB.
	mgr2 := session.NewManager()
	mgr2.SetTmuxClient(c)
	mux2 := http.NewServeMux()
	SessionRoutes(mux2, mgr2, db, c)
	GlobalRoutes(mux2, db, mgr2, c)
	srv2 := httptest.NewServer(mux2)
	t.Cleanup(srv2.Close)

	status, raw := getJSON(t, srv2.URL+"/api/sessions?scope=global")
	if status != http.StatusOK {
		t.Fatalf("scoped list: status = %d; body=%s", status, raw)
	}
	var entries []map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("restart list has %d entries, want 1 (the survivor): %s", len(entries), raw)
	}
	e := entries[0]
	if e["orphaned"] != true {
		t.Errorf("orphaned = %v, want true", e["orphaned"])
	}
	if e["global"] != true {
		t.Errorf("global = %v, want true", e["global"])
	}
	if e["label"] != "Bash 7" {
		t.Errorf("label = %v, want the persisted %q", e["label"], "Bash 7")
	}
	if e["tmuxName"] != name {
		t.Errorf("tmuxName = %v, want %q", e["tmuxName"], name)
	}
	if e["taskTitle"] != "Scratchpad" || e["projectName"] != "Global" {
		t.Errorf("labels = %v/%v, want Scratchpad/Global on the orphaned entry", e["taskTitle"], e["projectName"])
	}

	// The survivor counts under live.tmux (disjoint-by-name, OQ1) — and
	// nowhere else.
	status, raw = getJSON(t, srv2.URL+"/api/global")
	if status != http.StatusOK {
		t.Fatalf("GET /api/global: %d %s", status, raw)
	}
	var g struct {
		Live struct {
			Agent int `json:"agent"`
			Bash  int `json:"bash"`
			Tmux  int `json:"tmux"`
		} `json:"live"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatalf("decode global: %v", err)
	}
	if g.Live.Tmux != 1 {
		t.Errorf("live.tmux = %d, want 1 (the survivor)", g.Live.Tmux)
	}
	if g.Live.Agent != 0 || g.Live.Bash != 0 {
		t.Errorf("live.agent/bash = %d/%d, want 0/0 (tmux counts only under tmux)", g.Live.Agent, g.Live.Bash)
	}
}

// ---------------------------------------------------------------------------
// Phase 15 Plan 02: global AGENT spawn — one-agent gate (D-34), engine-
// branched resume (D-31), csid persist to the singleton, and the opencode
// capture re-target (T-15-06). The status-feed widening tests (Task 2) live
// below them.
// ---------------------------------------------------------------------------

// newGlobalAgentServerOpt wires the surface the global-agent suite needs over
// one DB + Manager: task Routes (worktreeTask fixtures), SessionRoutes with an
// INJECTED globRoot (resume validation + transcript fixtures never touch the
// real ~/.claude/projects), AgentRoutes sharing that globRoot (the status
// feed's resumable derivation), and GlobalRoutes (the PUT /api/global seeding
// step + the singleton the spawn branch reads read-at-use). The manager's
// ClaudeBin points at testdata/fake-claude; the stub's argv/pwd recorders are
// aimed at temp files via t.Setenv. sandboxHome toggles the HOME sandbox: CI
// tests sandbox it (PUT /api/global path validation — ExpandHome + the D-27
// ~/.kamacu block — resolves into the sandbox); the host-gated opencode e2e
// does NOT (the real binary must resolve its own config/auth from the real
// home — the spike-verified posture).
func newGlobalAgentServerOpt(t *testing.T, sandboxHome bool) (*httptest.Server, *session.Manager, *sql.DB, string, string, string) {
	t.Helper()
	if sandboxHome {
		t.Setenv("HOME", t.TempDir())
	}
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
	mgr.SetAgentConfig(session.AgentConfig{
		BaseURL:   "http://127.0.0.1:7333",
		Token:     testHookToken,
		ClaudeBin: testdataFakeClaude(t),
	})
	globRoot := t.TempDir()
	argsFile := filepath.Join(t.TempDir(), "args")
	pwdFile := filepath.Join(t.TempDir(), "pwd")
	t.Setenv("FAKE_CLAUDE_ARGS_FILE", argsFile)
	t.Setenv("FAKE_CLAUDE_PWD_FILE", pwdFile)

	mux := http.NewServeMux()
	Routes(mux, db, wt, mgr, tmux.Client{})
	s := &sessionHandlers{mgr: mgr, db: db, globRoot: globRoot}
	mux.HandleFunc("GET /api/sessions", s.list)
	mux.HandleFunc("POST /api/sessions", s.create)
	mux.HandleFunc("POST /api/sessions/{id}/stop", s.stop)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.delete)
	ah := &agentHandlers{mgr: mgr, db: db, globRoot: globRoot}
	mux.HandleFunc("GET /api/agents/status", ah.status)
	GlobalRoutes(mux, db, mgr, tmux.Client{})
	srv := httptest.NewServer(mux)
	t.Cleanup(func() {
		srv.Close()
		for _, info := range mgr.List() {
			if sess, ok := mgr.Get(info.ID); ok {
				sess.Stop()
			}
		}
		db.Close()
	})
	return srv, mgr, db, globRoot, argsFile, pwdFile
}

// newGlobalAgentServer is the CI shape: HOME-sandboxed.
func newGlobalAgentServer(t *testing.T) (*httptest.Server, *session.Manager, *sql.DB, string, string, string) {
	t.Helper()
	return newGlobalAgentServerOpt(t, true)
}

// globalClaudeSessionID reads the singleton's persisted claude session id —
// the Phase 15 global --resume key.
func globalClaudeSessionID(t *testing.T, db *sql.DB) string {
	t.Helper()
	var s sql.NullString
	if err := db.QueryRow(`SELECT claude_session_id FROM global_task WHERE id = 1`).Scan(&s); err != nil {
		t.Fatalf("query global_task.claude_session_id: %v", err)
	}
	if !s.Valid {
		t.Fatalf("global_task.claude_session_id is NULL, want a uuid")
	}
	return s.String
}

// setGlobalClaudeSession stamps the singleton's claude session id (restart-
// sim seeding for the resume path).
func setGlobalClaudeSession(t *testing.T, db *sql.DB, csid string) {
	t.Helper()
	if _, err := db.Exec(`UPDATE global_task SET claude_session_id = ? WHERE id = 1`, csid); err != nil {
		t.Fatalf("set global_task.claude_session_id: %v", err)
	}
}

// globalOpencodeAgentID returns the seeded system opencode agent (00015) —
// engine 'opencode', command 'opencode'. Used where no spawn happens (409
// gates) or where the REAL binary is wanted (host-gated e2e).
func globalOpencodeAgentID(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`SELECT id FROM agents WHERE engine = 'opencode' ORDER BY id LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("find opencode agent seed: %v", err)
	}
	return id
}

// waitGlobalSessionExited polls the manager until the session reports exited
// (fake-claude traps TERM and exits fast; 8s budgets the D-14 grace).
func waitGlobalSessionExited(t *testing.T, mgr *session.Manager, sid string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for {
		if s, ok := mgr.Get(sid); ok && s.Info().Status == session.StatusExited {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("global session %q never exited after stop", sid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// readStubFile polls a stub recorder file (≤3s) and returns its trimmed
// contents — same contract as readArgv, for the pwd recorder.
func readStubFile(t *testing.T, path string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
			return strings.TrimSpace(string(b))
		}
		if time.Now().After(deadline) {
			t.Fatalf("stub never recorded at %s", path)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// argvHasPair reports whether args contains the given consecutive tokens.
func argvHasPair(args []string, pair ...string) bool {
	for i := 0; i+len(pair) <= len(args); i++ {
		match := true
		for j, p := range pair {
			if args[i+j] != p {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// TestGlobalAgentUnconfiguredRoot409 (D-28/D-30, agent kind): the root gates
// cover the agent kind uniformly — an unconfigured root is the honest 409 and
// nothing spawns.
func TestGlobalAgentUnconfiguredRoot409(t *testing.T) {
	srv, mgr, _, _, _, _ := newGlobalAgentServer(t)

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "agent"})
	if status != http.StatusConflict {
		t.Fatalf("global agent spawn, unconfigured root: status = %d, want 409; body=%v", status, body)
	}
	if body["error"] != "global root not configured" {
		t.Errorf("error = %q, want %q", body["error"], "global root not configured")
	}
	if got := len(mgr.List()); got != 0 {
		t.Errorf("mgr.List() = %d sessions after the gated spawn, want 0", got)
	}
}

// TestGlobalAgentSpawn (GVIEW-02): a configured root spawns the singleton's
// agent 201 — label "Agent", global:true, engine read-at-use from the agent
// row — with the fake-claude really running in the global root (pwd proof)
// and the minted csid persisted to the singleton (never the tasks table).
func TestGlobalAgentSpawn(t *testing.T) {
	srv, mgr, db, _, _, pwdFile := newGlobalAgentServer(t)
	root := putGlobalFolderRoot(t, srv)

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "agent"})
	if status != http.StatusCreated {
		t.Fatalf("global agent spawn: status = %d, want 201; body=%v", status, body)
	}
	if body["label"] != "Agent" {
		t.Errorf("label = %q, want %q", body["label"], "Agent")
	}
	if body["kind"] != "agent" {
		t.Errorf("kind = %q, want %q", body["kind"], "agent")
	}
	if body["global"] != true {
		t.Errorf("global = %v, want true on the wire", body["global"])
	}
	if body["engine"] != "claude" {
		t.Errorf("engine = %v, want %q (read-at-use from the singleton's default agent)", body["engine"], "claude")
	}
	if body["agentStatus"] != "working" {
		t.Errorf("agentStatus = %q, want %q (spawn -> working)", body["agentStatus"], "working")
	}
	sid, _ := body["id"].(string)
	if sid == "" {
		t.Fatalf("spawn returned empty id: %v", body)
	}

	// cwd proof: fake-claude recorded its working directory — the PTY reports
	// the physical path, so compare against the symlink-resolved root.
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", root, err)
	}
	if got := readStubFile(t, pwdFile); got != resolved {
		t.Errorf("agent cwd = %q, want the global root %q", got, resolved)
	}

	// csid persist: the singleton carries the minted uuid (warn-only target).
	csid := globalClaudeSessionID(t, db)
	if _, err := uuid.Parse(csid); err != nil {
		t.Errorf("global_task.claude_session_id %q is not a parseable uuid: %v", csid, err)
	}

	// Exactly one agent session, registered under the global scope.
	agentCount := 0
	for _, info := range mgr.ListGlobal() {
		if info.Kind == session.KindAgent {
			agentCount++
		}
	}
	if agentCount != 1 {
		t.Errorf("ListGlobal() agent sessions = %d, want 1", agentCount)
	}
	// The persist never touched the tasks table: no task rows exist.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&n); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if n != 0 {
		t.Errorf("tasks rows = %d, want 0 (the csid persist targets global_task)", n)
	}
}

// TestGlobalAgentOneAgentGate (D-34/D-33): a second CONCURRENT global agent
// spawn is the honest 409 in the task-gate copy voice; an EXITED agent never
// blocks — a fresh spawn replaces it.
func TestGlobalAgentOneAgentGate(t *testing.T) {
	srv, mgr, _, _, _, _ := newGlobalAgentServer(t)
	putGlobalFolderRoot(t, srv)

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "agent"})
	if status != http.StatusCreated {
		t.Fatalf("first spawn: status = %d, want 201; body=%v", status, body)
	}
	sid1, _ := body["id"].(string)

	status, body = doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "agent"})
	if status != http.StatusConflict {
		t.Fatalf("concurrent spawn: status = %d, want 409; body=%v", status, body)
	}
	if body["error"] != "global agent already running" {
		t.Errorf("error = %q, want %q (D-34 copy voice)", body["error"], "global agent already running")
	}

	// Stop; an exited agent never blocks — fresh spawn replaces.
	if sstatus, _ := doJSON(t, "POST", srv.URL+"/api/sessions/"+sid1+"/stop", nil); sstatus != http.StatusAccepted {
		t.Fatalf("stop: status = %d, want 202", sstatus)
	}
	waitGlobalSessionExited(t, mgr, sid1)

	status, body = doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "agent"})
	if status != http.StatusCreated {
		t.Fatalf("spawn after exit: status = %d, want 201 (exited never blocks); body=%v", status, body)
	}
	sid2, _ := body["id"].(string)
	if sid2 == "" || sid2 == sid1 {
		t.Fatalf("replacement session id = %q, want a fresh id (old %q)", sid2, sid1)
	}
}

// TestGlobalAgentResumeNoId409 (D-31): resume with no persisted engine id is
// an honest 409 naming the ENGINE — never a silent fresh spawn. Both engine
// branches; the opencode half doubles as the read-at-use proof (the agent PUT
// changes the next spawn's engine with no restart).
func TestGlobalAgentResumeNoId409(t *testing.T) {
	srv, mgr, db, _, _, _ := newGlobalAgentServer(t)
	putGlobalFolderRoot(t, srv)

	// claude engine (the default agent): no singleton csid.
	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "agent", "resume": true})
	if status != http.StatusConflict {
		t.Fatalf("claude resume, no id: status = %d, want 409; body=%v", status, body)
	}
	if body["error"] != "no global claude session to resume" {
		t.Errorf("error = %q, want %q", body["error"], "no global claude session to resume")
	}

	// opencode engine (read-at-use agent switch): no singleton ocsid.
	status, body = doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"agent_id": globalOpencodeAgentID(t, db)})
	if status != http.StatusOK {
		t.Fatalf("PUT agent_id: status = %d; body=%v", status, body)
	}
	status, body = doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "agent", "resume": true})
	if status != http.StatusConflict {
		t.Fatalf("opencode resume, no id: status = %d, want 409; body=%v", status, body)
	}
	if body["error"] != "no global opencode session to resume" {
		t.Errorf("error = %q, want %q", body["error"], "no global opencode session to resume")
	}

	// Neither 409 spawned anything.
	if got := len(mgr.List()); got != 0 {
		t.Errorf("mgr.List() = %d sessions after resume 409s, want 0 (never a silent fresh spawn)", got)
	}
}

// TestGlobalAgentResumeArgv (GSESS-02, claude half): a seeded singleton csid
// + transcript makes the resume spawn carry --resume <csid>, and the
// latest-spawn-wins persist rewrites the SAME id.
func TestGlobalAgentResumeArgv(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, mgr, db, globRoot, argsFile, _ := newGlobalAgentServer(t)
	putGlobalFolderRoot(t, srv)

	stored := uuid.NewString()
	setGlobalClaudeSession(t, db, stored)
	seedTranscript(t, globRoot, stored) // sessions_test.go helper — cwd-agnostic glob

	if err := os.Remove(argsFile); err != nil && !os.IsNotExist(err) {
		t.Fatalf("clear argv file: %v", err)
	}
	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "agent", "resume": true})
	if status != http.StatusCreated {
		t.Fatalf("global claude resume: status = %d, want 201; body=%v", status, body)
	}
	args := readArgv(t, argsFile)
	if !argvHasPair(args, "--resume", stored) {
		t.Errorf("resume argv = %v, want it to carry --resume %q", args, stored)
	}

	// Resume keeps the id: the persist rewrites the same value.
	if got := globalClaudeSessionID(t, db); got != stored {
		t.Errorf("global_task.claude_session_id = %q after resume, want the SAME %q (resume never forks)", got, stored)
	}

	if sid, _ := body["id"].(string); sid != "" {
		doJSON(t, "POST", srv.URL+"/api/sessions/"+sid+"/stop", nil)
		waitGlobalSessionExited(t, mgr, sid)
	}
}

// TestGlobalAgentOpencodeCapture (CI, stubbed discovery — T-15-06): the
// capture poller is RE-TARGETED, not forked. A global opencode spawn
// persists the discovered id to global_task; a REAL task row's
// opencode_session_id stays NULL (the wrong-table write detector).
func TestGlobalAgentOpencodeCapture(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, _, db, _, _, _ := newGlobalAgentServer(t)
	root := putGlobalFolderRoot(t, srv)

	// A real task row: if the poller ever wrote the tasks table, THIS row's
	// opencode_session_id would flip non-NULL.
	tid, _ := worktreeTask(t, srv, "Wrong Table Detector")

	// Singleton agent -> opencode engine, command -> the committed
	// fake-opencode stub (the real binary is never invoked in CI).
	var ocAgent int64
	if err := db.QueryRow(
		`INSERT INTO agents (name, command, engine, is_default, is_system) VALUES ('global-oc-stub', ?, 'opencode', 0, 0) RETURNING id`,
		testdataFakeOpencode(t),
	).Scan(&ocAgent); err != nil {
		t.Fatalf("seed opencode stub agent: %v", err)
	}
	status, body := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"agent_id": ocAgent})
	if status != http.StatusOK {
		t.Fatalf("PUT agent_id: status = %d; body=%v", status, body)
	}

	// Stub discovery: a fixed id + the dir it was polled with (the PWD-pin /
	// cwd-parameterization proof — the poller must run against the root).
	orig := discoverOpenCodeSession
	var dirs []string
	discoverOpenCodeSession = func(ctx context.Context, dir string) (string, error) {
		dirs = append(dirs, dir)
		return "ses_global_stub_1", nil
	}
	t.Cleanup(func() { discoverOpenCodeSession = orig })

	status, body = doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "agent"})
	if status != http.StatusCreated {
		t.Fatalf("global opencode spawn: status = %d, want 201; body=%v", status, body)
	}

	// The poller's immediate first attempt hits the stub; poll the singleton
	// write (the goroutine races the 201 reply).
	deadline := time.Now().Add(5 * time.Second)
	var ocsid sql.NullString
	for {
		if err := db.QueryRow(`SELECT opencode_session_id FROM global_task WHERE id = 1`).Scan(&ocsid); err != nil {
			t.Fatalf("query global_task.opencode_session_id: %v", err)
		}
		if ocsid.Valid {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("global_task.opencode_session_id never captured (stub should return instantly)")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if ocsid.String != "ses_global_stub_1" {
		t.Errorf("global_task.opencode_session_id = %q, want the stub id %q", ocsid.String, "ses_global_stub_1")
	}
	if len(dirs) == 0 || dirs[0] != root {
		t.Errorf("discovery dirs = %v, want the first poll against the global root %q", dirs, root)
	}

	// The re-target proof: the tasks table gained NO opencode id.
	var taskOc sql.NullString
	if err := db.QueryRow(`SELECT opencode_session_id FROM tasks WHERE id = ?`, tid).Scan(&taskOc); err != nil {
		t.Fatalf("query tasks.opencode_session_id: %v", err)
	}
	if taskOc.Valid {
		t.Errorf("tasks.opencode_session_id = %q for task %d — the capture poller persisted to the WRONG table", taskOc.String, tid)
	}
}

// TestGlobalOpencodeCaptureHost (host-gated e2e, Spike 2): the REAL opencode
// binary in a temp-git-repo global root — after a first turn the capture
// poller persists the ses_… id to global_task. Skips where opencode is
// absent. NOT HOME-sandboxed: the real binary resolves its own config/auth
// from the real home; the session row it writes is keyed by the temp root
// directory, which is exactly what the capture filter matches on.
func TestGlobalOpencodeCaptureHost(t *testing.T) {
	if _, err := exec.LookPath("opencode"); err != nil {
		t.Skip("opencode not on PATH")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, mgr, db, _, _, _ := newGlobalAgentServerOpt(t, false)
	root := putGlobalFolderRoot(t, srv)

	// Singleton agent -> the seeded system opencode agent (command
	// 'opencode' — the real binary), via the real D-24 PUT surface.
	status, body := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"agent_id": globalOpencodeAgentID(t, db)})
	if status != http.StatusOK {
		t.Fatalf("PUT agent_id: status = %d; body=%v", status, body)
	}

	status, body = doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "agent"})
	if status != http.StatusCreated {
		t.Fatalf("global opencode spawn: status = %d, want 201; body=%v", status, body)
	}
	sid, _ := body["id"].(string)

	// Drive the first turn out-of-band: `opencode run` in the root is the
	// manual-terminal-session analog (the row is keyed by directory; the
	// capture's most-recently-updated-wins rule owns shared directories).
	// The model call may error — the session row is created regardless
	// (probe-verified on opencode 1.18.22).
	runCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "opencode", "run", "Reply with just: ok")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PWD="+root)
	out, rerr := cmd.CombinedOutput()
	t.Logf("opencode run: err=%v output=%s", rerr, out)

	// The capture poller (2s interval) picks the row up.
	deadline := time.Now().Add(60 * time.Second)
	var ocsid sql.NullString
	for {
		if err := db.QueryRow(`SELECT opencode_session_id FROM global_task WHERE id = 1`).Scan(&ocsid); err != nil {
			t.Fatalf("query global_task.opencode_session_id: %v", err)
		}
		if ocsid.Valid {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("global_task.opencode_session_id never captured; opencode run err=%v out=%s", rerr, out)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !strings.HasPrefix(ocsid.String, "ses_") {
		t.Errorf("captured id = %q, want a ses_… id", ocsid.String)
	}
	t.Logf("captured global opencode session id: %s", ocsid.String)

	if s, ok := mgr.Get(sid); ok {
		s.Stop()
	}
}
