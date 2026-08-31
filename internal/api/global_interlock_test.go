package api

// global_interlock_test.go — the managed-root ↔ project-delete interlock,
// both directions (D-58 / SC3 / 13-CTX D-01..D-03). The global managed root
// (~/.kamacu/repos/global/<owner>/<name>) and project clones
// (~/.kamacu/repos/<owner>/<name>) are SEPARATE namespaces by construction —
// no cross-entity guard exists because none is needed. These tests PROVE the
// design with deterministic filesystem assertions: no cross-entity delete is
// possible because no cross-entity path exists.
//
// Deterministic and offline: the github.Set*ForTest seams fake clone/validate/
// availability (no network, no real gh), HOME is sandboxed via the harness so
// the .kamacu/repos namespace lands in a temp dir, and the fake clone builds
// a REAL git directory whose remote-tracking refs satisfy the managed delete
// gates (origin/HEAD + a clean committed tree — see interlockFakeClone).

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"kamacu/internal/github"
	"kamacu/internal/tmux"
)

// interlockFakeClone materializes a real git repo at dest that (a) passes
// reattachManaged's origin check (remote origin → https://github.com/<ref>)
// and (b) passes the managed-delete git gates: a committed clean tree plus
// refs/remotes/origin/HEAD pointing at a remote-tracking main that equals
// HEAD, so DefaultBranch resolves and Dirty/Unpushed/Stash are all zero. The
// plain fakeGitClone (init + remote add, no commits) is enough for the PUT
// path but leaves origin/HEAD absent, which deleteManaged treats as a
// conservative blocker — this richer shape keeps Direction 1's delete at 204.
func interlockFakeClone(t *testing.T, ref, dest string) error {
	t.Helper()
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	run := func(args ...string) error {
		full := append([]string{"-C", dest, "-c", "user.name=test", "-c", "user.email=test@test"}, args...)
		cmd := exec.Command("git", full...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %v in %s: %v\n%s", args, dest, err, out)
		}
		return nil
	}
	if err := run("init", "-b", "main"); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dest, "file.txt"), []byte("v1\n"), 0o644); err != nil {
		return err
	}
	if err := run("add", "file.txt"); err != nil {
		return err
	}
	if err := run("commit", "-m", "c1"); err != nil {
		return err
	}
	if out, err := exec.Command("git", "-C", dest, "remote", "add", "origin",
		"https://github.com/"+ref+".git").CombinedOutput(); err != nil {
		return fmt.Errorf("git remote add: %v\n%s", err, out)
	}
	// Remote-tracking refs: origin/main == HEAD, origin/HEAD → origin/main.
	if err := run("update-ref", "refs/remotes/origin/main", "HEAD"); err != nil {
		return err
	}
	return run("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
}

// interlockSeams installs the four github seams with restores deferred to the
// TEST's lifetime via t.Cleanup (a `defer` here would restore them at helper
// exit — before any request runs). SetDescriptionRunnerForTest is the fourth
// seam: createByRepo captures a repo description when Available() is forced
// true — the fake keeps the whole fixture offline, never spawning a real gh.
func interlockSeams(t *testing.T) {
	t.Helper()
	t.Cleanup(github.SetCloneRunnerForTest(func(_ context.Context, ref, dest string) (string, error) {
		return "", interlockFakeClone(t, ref, dest)
	}))
	t.Cleanup(github.SetValidateRunnerForTest(func(_ context.Context, parsed string) (string, bool, error) {
		return "octo/widgets", true, nil // gh-canonicalized casing wins
	}))
	t.Cleanup(github.SetAvailableForTest(true))
	t.Cleanup(github.SetDescriptionRunnerForTest(func(context.Context, string) string {
		return ""
	}))
}

// interlockProjectDest computes the project-side managed namespace path for a
// canonical ref under the sandboxed HOME (reposBase: ~/.kamacu/repos/<owner>/<name>).
func interlockProjectDest(t *testing.T, canonical string) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	return filepath.Join(home, ".kamacu", "repos", filepath.FromSlash(canonical))
}

// TestGlobalInterlockProjectDeleteKeepsGlobalRoot (D-58 direction 1): the
// gated project delete removes ONLY the project clone directory — the global
// root directory stays on disk and the singleton still reports it configured.
func TestGlobalInterlockProjectDeleteKeepsGlobalRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	interlockSeams(t)
	srv, _, _ := newGlobalSessionServerWithTmux(t, tmux.Client{})

	// Project side: the clone lands at ~/.kamacu/repos/octo/widgets.
	status, body := doJSON(t, "POST", srv.URL+"/api/projects", map[string]any{"repo": "Octo/Widgets"})
	if status != http.StatusCreated {
		t.Fatalf("create managed project: status = %d, want 201; body=%v", status, body)
	}
	pid := int64(body["id"].(float64))
	projDest := interlockProjectDest(t, "octo/widgets")

	// Global side: the SAME ref clones into its own namespace at
	// ~/.kamacu/repos/global/octo/widgets (D-03: both succeed).
	status, gbody := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"repo": "Octo/Widgets"})
	if status != http.StatusOK {
		t.Fatalf("PUT /api/global managed root: status = %d, want 200; body=%v", status, gbody)
	}
	globDest := globalManagedDest(t, "octo/widgets")
	for _, dir := range []string{projDest, globDest} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("expected a cloned dir at %s (err=%v)", dir, err)
		}
	}

	// Direction 1: no tasks → no worktrees → gates clear → 204.
	status, dbody := deleteProject(t, srv, pid)
	if status != http.StatusNoContent {
		t.Fatalf("DELETE managed project: status = %d, want 204; body=%v", status, dbody)
	}
	if _, err := os.Stat(projDest); !os.IsNotExist(err) {
		t.Errorf("project clone %s still on disk after delete (err=%v) — removeManaged must RemoveAll it", projDest, err)
	}
	if info, err := os.Stat(globDest); err != nil || !info.IsDir() {
		t.Errorf("global root %s must survive the project delete untouched (err=%v)", globDest, err)
	}
	// The singleton still reports the root configured.
	status, g := doJSON(t, "GET", srv.URL+"/api/global", nil)
	if status != http.StatusOK {
		t.Fatalf("GET /api/global after project delete: status = %d; body=%v", status, g)
	}
	if got := g["root_path"]; got != globDest {
		t.Errorf("root_path = %v, want %s (the global root is not the project's to lose)", got, globDest)
	}
}

// TestGlobalInterlockClearKeepsProjectClone (D-58 direction 2): clearing the
// global root (PUT {"root_path":""}, the D-21 clear) clears the singleton row
// while the project clone directory and the project row remain intact.
func TestGlobalInterlockClearKeepsProjectClone(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	interlockSeams(t)
	srv, _, db := newGlobalSessionServerWithTmux(t, tmux.Client{})

	status, body := doJSON(t, "POST", srv.URL+"/api/projects", map[string]any{"repo": "Octo/Widgets"})
	if status != http.StatusCreated {
		t.Fatalf("create managed project: status = %d, want 201; body=%v", status, body)
	}
	pid := int64(body["id"].(float64))
	projDest := interlockProjectDest(t, "octo/widgets")

	status, gbody := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"repo": "Octo/Widgets"})
	if status != http.StatusOK {
		t.Fatalf("PUT /api/global managed root: status = %d, want 200; body=%v", status, gbody)
	}
	globDest := globalManagedDest(t, "octo/widgets")

	// Direction 2: the clear (no live global sessions — nothing spawned).
	status, cbody := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"root_path": ""})
	if status != http.StatusOK {
		t.Fatalf("clear global root: status = %d, want 200; body=%v", status, cbody)
	}
	// The global row is cleared…
	status, g := doJSON(t, "GET", srv.URL+"/api/global", nil)
	if status != http.StatusOK {
		t.Fatalf("GET /api/global after clear: status = %d; body=%v", status, g)
	}
	if got := g["root_path"]; got != "" {
		t.Errorf("root_path = %v, want \"\" after the clear", got)
	}
	// …the project clone directory STILL exists…
	if info, err := os.Stat(projDest); err != nil || !info.IsDir() {
		t.Errorf("project clone %s must survive the global clear untouched (err=%v)", projDest, err)
	}
	// …and the project row is intact.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM projects WHERE id = ?`, pid).Scan(&n); err != nil {
		t.Fatalf("count project row: %v", err)
	}
	if n != 1 {
		t.Errorf("project row gone after the global clear (count=%d, want 1)", n)
	}
	// The cleared global root directory also stays on disk (D-04:
	// re-configuring never deletes; clearing is a row op, not a disk op).
	if info, err := os.Stat(globDest); err != nil || !info.IsDir() {
		t.Errorf("global root dir %s must stay on disk after the clear (D-04; err=%v)", globDest, err)
	}
}

// TestGlobalInterlockFolderFolderDirections (D-58 cheap leg): with a folder
// global root and an unrelated folder project, deleting the project touches
// neither its own repository (folder deletes are row-only, PROJ-03/D-09) nor
// the global root — and the global stays configured.
func TestGlobalInterlockFolderFolderDirections(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, _, _ := newGlobalSessionServerWithTmux(t, tmux.Client{})

	projRepo := gitRepo(t)
	globRoot := gitRepo(t)
	status, body := doJSON(t, "POST", srv.URL+"/api/projects", map[string]any{"repo_path": projRepo})
	if status != http.StatusCreated {
		t.Fatalf("create folder project: status = %d, want 201; body=%v", status, body)
	}
	pid := int64(body["id"].(float64))
	status, gbody := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"root_path": globRoot})
	if status != http.StatusOK {
		t.Fatalf("PUT /api/global folder root: status = %d, want 200; body=%v", status, gbody)
	}

	status, dbody := deleteProject(t, srv, pid)
	if status != http.StatusNoContent {
		t.Fatalf("DELETE folder project: status = %d, want 204; body=%v", status, dbody)
	}
	// The folder project's own repo is NEVER touched (D-09)…
	if info, err := os.Stat(projRepo); err != nil || !info.IsDir() {
		t.Errorf("folder project repo %s must survive its own delete (err=%v)", projRepo, err)
	}
	// …and neither is the global root, which stays configured.
	if info, err := os.Stat(globRoot); err != nil || !info.IsDir() {
		t.Errorf("folder global root %s must survive the project delete (err=%v)", globRoot, err)
	}
	status, g := doJSON(t, "GET", srv.URL+"/api/global", nil)
	if status != http.StatusOK {
		t.Fatalf("GET /api/global after folder delete: status = %d; body=%v", status, g)
	}
	if got := g["root_path"]; got != globRoot {
		t.Errorf("root_path = %v, want %s", got, globRoot)
	}
}
