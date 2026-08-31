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
	"testing"
	"time"

	"kamacu/internal/github"
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

// fakeGitClone is the clone-seam fake that materializes a real git repo at
// dest with the origin remote a reattach check expects (git init + git
// remote add) — so reattach tests build directly on a successful "clone".
func fakeGitClone(t *testing.T, ref, dest string) error {
	t.Helper()
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	if out, err := exec.Command("git", "init", dest).CombinedOutput(); err != nil {
		return fmt.Errorf("git init: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", dest, "remote", "add", "origin",
		"https://github.com/"+ref+".git").CombinedOutput(); err != nil {
		return fmt.Errorf("git remote add: %v\n%s", err, out)
	}
	return nil
}

// globalManagedDest computes the expected managed namespace path for a
// canonical ref under the test HOME sandbox (D-02).
func globalManagedDest(t *testing.T, canonical string) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	return filepath.Join(home, ".kamacu", "repos", "global", filepath.FromSlash(canonical))
}

// TestPutGlobalManagedClone (GCONF-02/D-01/D-02): a verified repo clones
// into ~/.kamacu/repos/global/<owner>/<name> and the PUT response (and a
// follow-up GET) show the dest as root_path with the canonical
// github_repo.
func TestPutGlobalManagedClone(t *testing.T) {
	srv, _ := newGlobalTestServer(t)
	var gotDest string
	defer github.SetCloneRunnerForTest(func(_ context.Context, ref, dest string) (string, error) {
		gotDest = dest
		return "", fakeGitClone(t, ref, dest)
	})()
	defer github.SetAvailableForTest(true)()
	defer github.SetValidateRunnerForTest(func(_ context.Context, parsed string) (string, bool, error) {
		return "octo/widgets", true, nil // gh-canonicalized casing wins
	})()

	status, body := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"repo": "Octo/Widgets"})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", status, body)
	}
	wantDest := globalManagedDest(t, "octo/widgets")
	if gotDest != wantDest {
		t.Errorf("clone dest = %q, want %q", gotDest, wantDest)
	}
	if got := body["root_path"]; got != wantDest {
		t.Errorf("root_path = %v, want %s", got, wantDest)
	}
	if got := body["github_repo"]; got != "octo/widgets" {
		t.Errorf("github_repo = %v, want octo/widgets", got)
	}
	if got, ok := body["root_exists"]; !ok || got != true {
		t.Errorf("root_exists = %v (present=%v), want true (the fake created the dir)", got, ok)
	}
	// Persisted: a follow-up GET shows the same managed config.
	status, body = doJSON(t, "GET", srv.URL+"/api/global", nil)
	if status != http.StatusOK {
		t.Fatalf("GET: status = %d", status)
	}
	if got := body["root_path"]; got != wantDest {
		t.Errorf("GET root_path = %v, want %s", got, wantDest)
	}
	if got := body["github_repo"]; got != "octo/widgets" {
		t.Errorf("GET github_repo = %v, want octo/widgets", got)
	}
}

// TestPutGlobalManagedValidateFail (degrade-don't-break): a gh-unverified
// ref 400s with msgRepoNotFound; with gh absent the copy flips to
// msgGHUnavailable. No clone on either path.
func TestPutGlobalManagedValidateFail(t *testing.T) {
	srv, _ := newGlobalTestServer(t)
	defer failCloneNeverCalled(t)()

	defer github.SetAvailableForTest(true)()
	defer github.SetValidateRunnerForTest(func(_ context.Context, parsed string) (string, bool, error) {
		return parsed, false, nil // never verified
	})()
	status, body := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"repo": "octo/widgets"})
	if status != http.StatusBadRequest {
		t.Fatalf("not-verified: status = %d, want 400; body=%v", status, body)
	}
	if got := body["error"]; got != msgRepoNotFound {
		t.Errorf("not-verified error = %v, want %q", got, msgRepoNotFound)
	}

	defer github.SetAvailableForTest(false)()
	status, body = doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"repo": "octo/widgets"})
	if status != http.StatusBadRequest {
		t.Fatalf("gh-absent: status = %d, want 400; body=%v", status, body)
	}
	if got := body["error"]; got != msgGHUnavailable {
		t.Errorf("gh-absent error = %v, want %q", got, msgGHUnavailable)
	}
}

// TestPutGlobalManagedCloneFailureAtomic (SC2): a failed clone returns ONE
// inline 500, leaves the PRIOR config byte-identical, and no directory at
// the dest.
func TestPutGlobalManagedCloneFailureAtomic(t *testing.T) {
	srv, db := newGlobalTestServer(t)
	// Prior working config: a folder root.
	prior := gitRepo(t)
	if status, body := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"root_path": prior}); status != http.StatusOK {
		t.Fatalf("seed folder root: status = %d; body=%v", status, body)
	}
	defer github.SetAvailableForTest(true)()
	defer github.SetValidateRunnerForTest(func(_ context.Context, parsed string) (string, bool, error) {
		return parsed, true, nil
	})()
	defer github.SetCloneRunnerForTest(func(_ context.Context, ref, dest string) (string, error) {
		return "fatal: repository not found", fmt.Errorf("clone failed")
	})()

	dest := globalManagedDest(t, "octo/widgets")
	status, body := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"repo": "octo/widgets"})
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%v", status, body)
	}
	// Clone's contract surfaces the trimmed stderr ("fatal: " stripped).
	if got := body["error"]; got != "repository not found" {
		t.Errorf("error = %v, want the inline clone stderr", got)
	}
	// The singleton keeps its PRIOR value.
	rootPath, repo, _ := readGlobalRow(t, db)
	if rootPath != prior {
		t.Errorf("root_path = %q, want the prior %q", rootPath, prior)
	}
	if repo.Valid {
		t.Errorf("github_repo = %v, want NULL (prior was a folder root)", repo)
	}
	// No residue at the dest.
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("dest %s must not exist after a failed clone (stat err=%v)", dest, err)
	}
}

// TestPutGlobalManagedReattach (D-04): a same-repo re-PUT reattaches the
// existing dir without re-cloning (the clone seam fails the test if
// invoked).
func TestPutGlobalManagedReattach(t *testing.T) {
	srv, _ := newGlobalTestServer(t)
	defer github.SetAvailableForTest(true)()
	defer github.SetValidateRunnerForTest(func(_ context.Context, parsed string) (string, bool, error) {
		return parsed, true, nil
	})()
	// First PUT: a successful fake clone materializes the dest.
	defer github.SetCloneRunnerForTest(func(_ context.Context, ref, dest string) (string, error) {
		return "", fakeGitClone(t, ref, dest)
	})()
	status, body := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"repo": "octo/widgets"})
	if status != http.StatusOK {
		t.Fatalf("first PUT: status = %d; body=%v", status, body)
	}
	firstRoot := body["root_path"]

	// Second PUT with the same repo: clone must NOT run.
	defer failCloneNeverCalled(t)()
	status, body = doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"repo": "octo/widgets"})
	if status != http.StatusOK {
		t.Fatalf("reattach PUT: status = %d, want 200; body=%v", status, body)
	}
	if got := body["root_path"]; got != firstRoot {
		t.Errorf("reattach root_path = %v, want the same %v", got, firstRoot)
	}
	if got := body["github_repo"]; got != "octo/widgets" {
		t.Errorf("reattach github_repo = %v, want octo/widgets", got)
	}
}

// TestPutGlobalManagedReattachMismatch (D-04): an existing dest whose
// origin points at a different repo is refused 409 — never clobbered.
func TestPutGlobalManagedReattachMismatch(t *testing.T) {
	srv, _ := newGlobalTestServer(t)
	defer failCloneNeverCalled(t)()
	defer github.SetAvailableForTest(true)()
	defer github.SetValidateRunnerForTest(func(_ context.Context, parsed string) (string, bool, error) {
		return parsed, true, nil
	})()
	// Hand-materialize the gadgets dest with the WRONG origin (widgets).
	dest := globalManagedDest(t, "octo/gadgets")
	if err := fakeGitClone(t, "octo/widgets", dest); err != nil {
		t.Fatalf("seed mismatched dest: %v", err)
	}
	status, body := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"repo": "octo/gadgets"})
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%v", status, body)
	}
	want := fmt.Sprintf("a different repository is already checked out at %s", dest)
	if got := body["error"]; got != want {
		t.Errorf("error = %v, want %q", got, want)
	}
}

// TestPutGlobalRootBlockedByLiveTmux (GCONF-04 / D-13/D-15, 14-RESEARCH
// Pitfall 7): the 409 gate tripped against a REAL detached global tmux
// session — not a zero-rows unit fake. The worktrees_test.go:397-440 recipe
// adapted to the global scope: LookPath skip-guard, per-test socket Client,
// KillServer cleanup registered BEFORE the server is built (LIFO — it runs
// after the harness cleanup), a seeded scope='global' tmux_sessions row
// (task_id NULL satisfies the 00018 XOR CHECK), and a real new-session -d.
// Proves: every root-field shape gates, the row stays untouched, agent-only
// PUTs pass (D-24), and liveness — not row existence — is what blocks
// (D-14): stop the session and the same PUT succeeds.
func TestPutGlobalRootBlockedByLiveTmux(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	// Per-test socket + kill-server cleanup registered BEFORE the server
	// (LIFO: runs after the harness teardown).
	c := tmux.Client{Socket: fmt.Sprintf("ktest-globalgate-%d", os.Getpid()), ConfPath: "/dev/null"}
	t.Cleanup(func() { _ = c.KillServer(context.Background()) })
	srv, db := newGlobalTestServerWithTmux(t, c)

	// A non-empty prior root so "row untouched" asserts against something.
	prior := gitRepo(t)
	if status, body := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"root_path": prior}); status != http.StatusOK {
		t.Fatalf("seed folder root: status = %d; body=%v", status, body)
	}

	// Persisted global row + a REAL detached tmux session under that name —
	// exactly the restart-survivor shape the gate must catch.
	name := "kamacu-global-1"
	if _, err := db.Exec(
		`INSERT INTO tmux_sessions (scope, n, name, label) VALUES ('global', 1, ?, 'Bash 1')`, name); err != nil {
		t.Fatalf("seed tmux_sessions row: %v", err)
	}
	detach := append(c.BaseArgs(), "new-session", "-d", "-s", name, "-c", t.TempDir())
	if err := exec.Command("tmux", detach...).Run(); err != nil {
		t.Fatalf("seed live global tmux session: %v", err)
	}
	awaitHasSession(t, c, name, true)

	// The CLEAR shape (root_path:"") gates: 409 with the structured D-15
	// grammar — non-empty error + non-empty reasons[] of {kind,target} maps.
	status, body := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"root_path": ""})
	if status != http.StatusConflict {
		t.Fatalf("clear under live session: status = %d, want 409; body=%v", status, body)
	}
	if got, _ := body["error"].(string); got == "" {
		t.Errorf("409 error = %v, want non-empty", body["error"])
	}
	reasons, ok := body["reasons"].([]any)
	if !ok || len(reasons) == 0 {
		t.Fatalf("409 reasons = %v, want a non-empty array", body["reasons"])
	}
	first, ok := reasons[0].(map[string]any)
	if !ok {
		t.Fatalf("reasons[0] = %v, want a {kind,target} map", reasons[0])
	}
	if got := first["kind"]; got != "sessions" {
		t.Errorf("reasons[0].kind = %v, want %q", got, "sessions")
	}
	if got := first["target"]; got != name {
		t.Errorf("reasons[0].target = %v, want %q", got, name)
	}

	// A non-empty folder root gates too — every root-field shape.
	other := gitRepo(t)
	status, body = doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"root_path": other})
	if status != http.StatusConflict {
		t.Fatalf("folder change under live session: status = %d, want 409; body=%v", status, body)
	}

	// The singleton row is untouched by both 409s.
	rootPath, repo, agentID := readGlobalRow(t, db)
	if rootPath != prior {
		t.Errorf("root_path = %q, want the prior %q after a 409", rootPath, prior)
	}
	if repo.Valid {
		t.Errorf("github_repo = %v, want NULL (prior was a folder root)", repo)
	}
	status, gbody := doJSON(t, "GET", srv.URL+"/api/global", nil)
	if status != http.StatusOK {
		t.Fatalf("GET after 409s: status = %d", status)
	}
	if got := gbody["root_path"]; got != prior {
		t.Errorf("GET root_path = %v, want the prior %s", got, prior)
	}
	if got, ok := gbody["github_repo"]; !ok || got != nil {
		t.Errorf("GET github_repo = %v (present=%v), want null after a 409", got, ok)
	}

	// D-24: under the SAME live session an agent-only PUT succeeds — the
	// gate scopes to root changes only. Re-PUT the current agent (a no-op
	// agent change is a sanctioned success) so the row comparison above
	// stays meaningful for the post-kill clear below.
	status, body = doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"agent_id": agentID})
	if status != http.StatusOK {
		t.Fatalf("agent-only PUT under live session: status = %d, want 200; body=%v", status, body)
	}

	// D-13/D-14: LIVENESS blocks, not row existence. Stop the session (the
	// row stays) and the identical clear succeeds.
	if err := c.KillServer(context.Background()); err != nil {
		t.Fatalf("KillServer: %v", err)
	}
	awaitHasSession(t, c, name, false)
	status, body = doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"root_path": ""})
	if status != http.StatusOK {
		t.Fatalf("clear after session stopped: status = %d, want 200; body=%v", status, body)
	}
	if got := body["root_path"]; got != "" {
		t.Errorf("root_path = %v, want \"\" after the clear", got)
	}
	if got, ok := body["github_repo"]; !ok || got != nil {
		t.Errorf("github_repo = %v (present=%v), want null after the clear", got, ok)
	}
}

// TestPutGlobalConfigHistory (Pitfall 8 + D-04/D-16): the managed → folder →
// clear → managed lifecycle over ONE HOME-sandboxed server. The stale-marker
// invariant (github_repo NULL the moment the root leaves the managed
// variant), the no-reclone reattach (a must-not-be-called clone fake), and
// the D-16 id-clearing woven through EVERY root PUT — including the final
// same-repo reattach re-PUT (identical values, no no-op exception) — while
// an agent-only interlude leaves the ids set. updated_at strictly increases
// across each successful PUT (millisecond strftime ⇒ lexical compare).
func TestPutGlobalConfigHistory(t *testing.T) {
	srv, db := newGlobalTestServer(t)
	// Managed-variant seams: verified-by-fake + a clone fake that git-inits
	// dest with the canonical origin (reattach later builds on it).
	defer github.SetAvailableForTest(true)()
	defer github.SetValidateRunnerForTest(func(_ context.Context, parsed string) (string, bool, error) {
		return parsed, true, nil
	})()
	defer github.SetCloneRunnerForTest(func(_ context.Context, ref, dest string) (string, error) {
		return "", fakeGitClone(t, ref, dest)
	})()

	put := func(payload map[string]any) (int, map[string]any) {
		// Millisecond-resolution updated_at: sleep past the tick so each
		// PUT's bump is strictly greater, deterministically.
		time.Sleep(15 * time.Millisecond)
		return doJSON(t, "PUT", srv.URL+"/api/global", payload)
	}
	updated := func() string {
		var s string
		if err := db.QueryRow(`SELECT updated_at FROM global_task WHERE id = 1`).Scan(&s); err != nil {
			t.Fatalf("read updated_at: %v", err)
		}
		return s
	}
	seedIDs := func(c, o string) {
		if _, err := db.Exec(`UPDATE global_task SET claude_session_id = ?, opencode_session_id = ?`, c, o); err != nil {
			t.Fatalf("seed resume ids: %v", err)
		}
	}
	readIDs := func() (claude, opencode sql.NullString) {
		if err := db.QueryRow(`SELECT claude_session_id, opencode_session_id FROM global_task WHERE id = 1`).Scan(&claude, &opencode); err != nil {
			t.Fatalf("read resume ids: %v", err)
		}
		return
	}
	// assertCleared: both ids NULL after a root PUT (D-16, no exceptions).
	assertCleared := func(step string) {
		c, o := readIDs()
		if c.Valid || o.Valid {
			t.Errorf("%s: resume ids must be NULL after a root PUT (D-16), got claude=%v opencode=%v", step, c, o)
		}
	}

	wantDest := globalManagedDest(t, "octo/widgets")
	prevUpdated := updated()

	// 1. MANAGED: clone into the global namespace; GET agrees.
	seedIDs("c1", "o1")
	status, body := put(map[string]any{"repo": "octo/widgets"})
	if status != http.StatusOK {
		t.Fatalf("managed PUT: status = %d; body=%v", status, body)
	}
	if got := body["root_path"]; got != wantDest {
		t.Errorf("managed root_path = %v, want %s", got, wantDest)
	}
	if got := body["github_repo"]; got != "octo/widgets" {
		t.Errorf("managed github_repo = %v, want octo/widgets", got)
	}
	assertCleared("managed PUT")
	if now := updated(); now <= prevUpdated {
		t.Errorf("updated_at not bumped by the managed PUT: %q then %q", prevUpdated, now)
	} else {
		prevUpdated = now
	}
	status, body = doJSON(t, "GET", srv.URL+"/api/global", nil)
	if status != http.StatusOK {
		t.Fatalf("GET managed: status = %d", status)
	}
	if got := body["root_path"]; got != wantDest {
		t.Errorf("GET managed root_path = %v, want %s", got, wantDest)
	}
	if got := body["github_repo"]; got != "octo/widgets" {
		t.Errorf("GET managed github_repo = %v, want octo/widgets", got)
	}

	// 2. FOLDER transition: the stale-marker assertion — github_repo goes
	// JSON null in the SAME UPDATE that sets the folder root.
	seedIDs("c2", "o2")
	folder := gitRepo(t)
	status, body = put(map[string]any{"root_path": folder})
	if status != http.StatusOK {
		t.Fatalf("folder PUT: status = %d; body=%v", status, body)
	}
	if got := body["github_repo"]; got != nil {
		t.Errorf("folder PUT github_repo = %v, want nil — a stale managed marker must never survive the transition", got)
	}
	assertCleared("folder PUT")
	if now := updated(); now <= prevUpdated {
		t.Errorf("updated_at not bumped by the folder PUT: %q then %q", prevUpdated, now)
	} else {
		prevUpdated = now
	}
	status, body = doJSON(t, "GET", srv.URL+"/api/global", nil)
	if status != http.StatusOK {
		t.Fatalf("GET folder: status = %d", status)
	}
	if got, ok := body["github_repo"]; !ok || got != nil {
		t.Errorf("GET folder github_repo = %v (present=%v), want null — no stale marker", got, ok)
	}
	if got := body["root_path"]; got != folder {
		t.Errorf("GET folder root_path = %v, want %s", got, folder)
	}

	// 3. CLEAR: both root columns empty.
	seedIDs("c3", "o3")
	status, body = put(map[string]any{"root_path": ""})
	if status != http.StatusOK {
		t.Fatalf("clear PUT: status = %d; body=%v", status, body)
	}
	assertCleared("clear PUT")
	if now := updated(); now <= prevUpdated {
		t.Errorf("updated_at not bumped by the clear PUT: %q then %q", prevUpdated, now)
	} else {
		prevUpdated = now
	}
	status, body = doJSON(t, "GET", srv.URL+"/api/global", nil)
	if status != http.StatusOK {
		t.Fatalf("GET cleared: status = %d", status)
	}
	if got := body["root_path"]; got != "" {
		t.Errorf("GET cleared root_path = %v, want \"\"", got)
	}
	if got, ok := body["github_repo"]; !ok || got != nil {
		t.Errorf("GET cleared github_repo = %v (present=%v), want null", got, ok)
	}

	// 4. AGENT-ONLY interlude: the ids survive (D-25 — D-16 is the only
	// clearing path), updated_at still bumps.
	seedIDs("c4", "o4")
	var agentID int64
	if err := db.QueryRow(`SELECT agent_id FROM global_task WHERE id = 1`).Scan(&agentID); err != nil {
		t.Fatalf("read agent_id: %v", err)
	}
	status, body = put(map[string]any{"agent_id": agentID})
	if status != http.StatusOK {
		t.Fatalf("agent interlude PUT: status = %d; body=%v", status, body)
	}
	if c, o := readIDs(); !c.Valid || c.String != "c4" || !o.Valid || o.String != "o4" {
		t.Errorf("agent-only PUT must leave resume ids set (D-25), got claude=%v opencode=%v", c, o)
	}
	if now := updated(); now <= prevUpdated {
		t.Errorf("updated_at not bumped by the agent PUT: %q then %q", prevUpdated, now)
	} else {
		prevUpdated = now
	}

	// 5. SAME-REPO REATTACH: the dest from step 1 is still on disk, so the
	// re-PUT must succeed WITHOUT invoking the clone runner (D-04) and —
	// identical values or not — STILL clear the resume ids (D-16's
	// no-no-op-re-PUT exception ban, on the exact path most likely to skip
	// it). failCloneNeverCalled installs the must-not-be-called seam NOW
	// (the deferred call restores it at test end).
	defer failCloneNeverCalled(t)()
	seedIDs("c5", "o5")
	status, body = put(map[string]any{"repo": "octo/widgets"})
	if status != http.StatusOK {
		t.Fatalf("reattach PUT: status = %d, want 200; body=%v", status, body)
	}
	if got := body["root_path"]; got != wantDest {
		t.Errorf("reattach root_path = %v, want the same %s", got, wantDest)
	}
	if got := body["github_repo"]; got != "octo/widgets" {
		t.Errorf("reattach github_repo = %v, want octo/widgets", got)
	}
	assertCleared("reattach PUT")
	if now := updated(); now <= prevUpdated {
		t.Errorf("updated_at not bumped by the reattach PUT: %q then %q", prevUpdated, now)
	}
	// Managed config fully restored.
	status, body = doJSON(t, "GET", srv.URL+"/api/global", nil)
	if status != http.StatusOK {
		t.Fatalf("GET after reattach: status = %d", status)
	}
	if got := body["root_path"]; got != wantDest {
		t.Errorf("GET after reattach root_path = %v, want %s", got, wantDest)
	}
	if got := body["github_repo"]; got != "octo/widgets" {
		t.Errorf("GET after reattach github_repo = %v, want octo/widgets", got)
	}
}

// TestPutGlobalRepoDispatch (D-20/D-22 + Open Question 2): the dispatch
// grammar edges — both fields supplied, explicit-but-empty repo, and a
// syntactically invalid ref.
func TestPutGlobalRepoDispatch(t *testing.T) {
	srv, _ := newGlobalTestServer(t)
	defer failCloneNeverCalled(t)()

	status, body := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"repo": "o/w", "root_path": "/x"})
	if status != http.StatusBadRequest {
		t.Fatalf("both supplied: status = %d, want 400; body=%v", status, body)
	}
	if got := body["error"]; got != "supply either repo or root_path, not both" {
		t.Errorf("both supplied error = %v, want the not-both copy", got)
	}

	status, body = doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"repo": ""})
	if status != http.StatusBadRequest {
		t.Fatalf("empty repo: status = %d, want 400; body=%v", status, body)
	}
	if got := body["error"]; got != `repo must be owner/name; to clear the root send root_path ""` {
		t.Errorf("empty repo error = %v, want the guidance copy", got)
	}

	status, body = doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"repo": "notaref"})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid ref: status = %d, want 400; body=%v", status, body)
	}
	if got := body["error"]; got != "Not a valid repository — use owner/name or a GitHub URL." {
		t.Errorf("invalid ref error = %v, want the ParseRepoRef copy", got)
	}
}
