package api

// Managed-project gated-delete tests (14-04, CKOUT-03 / D-07 / D-08 / D-09).
//
// These run against REAL git file:// clones + worktrees (no mocks), matching
// the worktree/reaper test hygiene. A managed project's repo_path is a clone
// with origin/HEAD set (makeOriginAndClone); its task worktrees branch off it.
// The delete is two-pass all-or-nothing: gate every task worktree AND the
// clone root on dirty/unpushed/stash/session; on any blocker → 409 + a
// structured reasons list + NOTHING removed; on all-clear → linked worktrees
// removed first, then os.RemoveAll(clone), then the rows (204). Folder
// (managed=0) delete is byte-for-byte unchanged — its dir is NEVER touched.

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// makeManagedProject creates a managed (managed=1) project whose repo_path is a
// real file:// clone (origin/HEAD set). Returns (projectID, origin, clone).
func makeManagedProject(t *testing.T, db *sql.DB) (int64, string, string) {
	t.Helper()
	origin, clone := makeOriginAndClone(t)
	var id int64
	err := db.QueryRow(
		`INSERT INTO projects (name, repo_path, github_repo, managed) VALUES (?, ?, ?, 1) RETURNING id`,
		filepath.Base(clone), clone, "owner/name").Scan(&id)
	if err != nil {
		t.Fatalf("insert managed project: %v", err)
	}
	return id, origin, clone
}

// addManagedTaskWorktree creates a task on the managed project and provisions a
// real worktree off the clone (a task/<slug>-<id> branch), returning
// (taskID, worktreePath). It drives the same provisioning the HTTP create path
// uses so the worktree is a genuine linked worktree of the clone.
func addManagedTaskWorktree(t *testing.T, srv *httptest.Server, projectID int64, title string) (int64, string) {
	t.Helper()
	body := createTask(t, srv, projectID, title)
	id := taskID(t, body)
	wtPath, _ := body["worktree_path"].(string)
	if wtPath == "" {
		t.Fatalf("task %q did not provision a worktree: %v", title, body)
	}
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("provisioned worktree dir missing: %v", err)
	}
	return id, wtPath
}

// projectExists reports whether a project row with id still exists.
func projectExists(t *testing.T, db *sql.DB, id int64) bool {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM projects WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("count project %d: %v", id, err)
	}
	return n > 0
}

// deleteProject issues DELETE /api/projects/{id} and returns (status, body).
func deleteProject(t *testing.T, srv *httptest.Server, id int64) (int, map[string]any) {
	t.Helper()
	return doJSON(t, "DELETE", fmt.Sprintf("%s/api/projects/%d", srv.URL, id), nil)
}

// assertBlocked asserts a managed-delete refusal: 409, a non-empty structured
// reasons list, the clone dir + worktree dirs still present, and the row intact.
func assertBlocked(t *testing.T, srv *httptest.Server, db *sql.DB, id int64, clone string, worktrees ...string) {
	t.Helper()
	status, body := deleteProject(t, srv, id)
	if status != http.StatusConflict {
		t.Fatalf("blocked delete: status = %d, want 409; body=%v", status, body)
	}
	reasons, ok := body["reasons"].([]any)
	if !ok || len(reasons) == 0 {
		t.Fatalf("blocked delete: body missing non-empty 'reasons' list; body=%v", body)
	}
	if _, err := os.Stat(clone); err != nil {
		t.Fatalf("clone removed on a blocked delete (err=%v); must remove NOTHING", err)
	}
	for _, wt := range worktrees {
		if _, err := os.Stat(wt); err != nil {
			t.Fatalf("worktree %s removed on a blocked delete (err=%v); must remove NOTHING", wt, err)
		}
	}
	if !projectExists(t, db, id) {
		t.Fatalf("project row deleted on a blocked delete; must remove NOTHING")
	}
}

// TestManagedDeleteGateDirty: a task worktree with an uncommitted change blocks
// the managed delete → 409 + reasons, nothing removed.
func TestManagedDeleteGateDirty(t *testing.T) {
	srv, env := newWorktreeServer(t)
	id, _, clone := makeManagedProject(t, env.db)
	taskID, wt := addManagedTaskWorktree(t, srv, id, "Dirty Task")
	_ = taskID
	// Make the worktree dirty (an untracked file is dirt per DirtyCount).
	if err := os.WriteFile(filepath.Join(wt, "scratch.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatalf("write dirt: %v", err)
	}
	assertBlocked(t, srv, env.db, id, clone, wt)
}

// TestManagedDeleteGateUnpushed: a task worktree with a local-only commit
// (origin/<default>..HEAD > 0) blocks the delete → 409, nothing removed.
func TestManagedDeleteGateUnpushed(t *testing.T) {
	srv, env := newWorktreeServer(t)
	id, _, clone := makeManagedProject(t, env.db)
	_, wt := addManagedTaskWorktree(t, srv, id, "Unpushed Task")
	// Commit locally in the worktree — never pushed → unpushed gate trips.
	if err := os.WriteFile(filepath.Join(wt, "local.txt"), []byte("local\n"), 0o644); err != nil {
		t.Fatalf("write local file: %v", err)
	}
	gitIn(t, wt, "add", "local.txt")
	gitIn(t, wt, "commit", "-m", "local-only")
	assertBlocked(t, srv, env.db, id, clone, wt)
}

// TestManagedDeleteGateStash: a stash (repo-global) blocks the delete → 409,
// nothing removed.
func TestManagedDeleteGateStash(t *testing.T) {
	srv, env := newWorktreeServer(t)
	id, _, clone := makeManagedProject(t, env.db)
	_, wt := addManagedTaskWorktree(t, srv, id, "Stash Task")
	// Modify a TRACKED file then stash it — leaves a refs/stash entry.
	if err := os.WriteFile(filepath.Join(wt, "file.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("modify tracked file: %v", err)
	}
	gitIn(t, wt, "stash")
	assertBlocked(t, srv, env.db, id, clone, wt)
}

// TestManagedDeleteGateCloneUnpushed (D-08 safety net): a local-only commit on
// the CLONE ROOT's default branch (origin/<default>..HEAD > 0) blocks the
// delete → 409, nothing removed. No task worktree needed.
func TestManagedDeleteGateCloneUnpushed(t *testing.T) {
	srv, env := newWorktreeServer(t)
	id, _, clone := makeManagedProject(t, env.db)
	// Commit directly on the clone's default branch — never pushed.
	if err := os.WriteFile(filepath.Join(clone, "cloneonly.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write clone file: %v", err)
	}
	gitIn(t, clone, "add", "cloneonly.txt")
	gitIn(t, clone, "commit", "-m", "clone-local-only")
	assertBlocked(t, srv, env.db, id, clone)
}

// TestManagedDeleteAllClean (CKOUT-03 all-clear): a managed project with a clean
// clone + one clean task worktree → DELETE → 204; the clone dir is gone
// (os.RemoveAll), the worktree dir is gone, and the project + task rows are
// deleted.
func TestManagedDeleteAllClean(t *testing.T) {
	srv, env := newWorktreeServer(t)
	id, _, clone := makeManagedProject(t, env.db)
	taskID, wt := addManagedTaskWorktree(t, srv, id, "Clean Task")

	status, body := deleteProject(t, srv, id)
	if status != http.StatusNoContent {
		t.Fatalf("all-clean delete: status = %d, want 204; body=%v", status, body)
	}
	if _, err := os.Stat(clone); !os.IsNotExist(err) {
		t.Fatalf("clone dir still present after clean delete (err=%v); want removed", err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("worktree dir still present after clean delete (err=%v); want removed", err)
	}
	if projectExists(t, env.db, id) {
		t.Fatalf("project row still present after clean delete; want deleted")
	}
	var nTasks int
	if err := env.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE id = ?`, taskID).Scan(&nTasks); err != nil {
		t.Fatalf("count task row: %v", err)
	}
	if nTasks != 0 {
		t.Fatalf("task row still present after clean delete = %d, want 0 (cascade)", nTasks)
	}
}

// TestManagedDeleteOrdering: with a linked worktree, after a clean delete the
// clone dir is FULLY gone (not a partial tree). The remove phase tears the
// linked worktree down BEFORE os.RemoveAll(clone), so no orphaned worktree with
// a dangling .git is left behind (Pitfall 1).
func TestManagedDeleteOrdering(t *testing.T) {
	srv, env := newWorktreeServer(t)
	id, _, clone := makeManagedProject(t, env.db)
	_, wt := addManagedTaskWorktree(t, srv, id, "Ordering Task")

	// Sanity: the worktree is a genuine LINKED worktree of the clone.
	if got := gitOut(t, clone, "worktree", "list", "--porcelain"); !containsPath(got, wt) {
		t.Fatalf("precondition: %s is not a linked worktree of the clone:\n%s", wt, got)
	}

	status, body := deleteProject(t, srv, id)
	if status != http.StatusNoContent {
		t.Fatalf("ordering delete: status = %d, want 204; body=%v", status, body)
	}
	// The whole clone tree is gone — no partial dir, no orphaned worktree.
	if _, err := os.Stat(clone); !os.IsNotExist(err) {
		t.Fatalf("clone dir not fully removed (err=%v)", err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("linked worktree dir not removed (err=%v)", err)
	}
}

// TestFolderDeleteNeverTouchesDir (D-09 / PROJ-03): a folder (managed=0) project
// pointing at a real dir → DELETE → 204 AND the directory still exists on disk.
// The managed disk-removal path must never run for a folder project.
func TestFolderDeleteNeverTouchesDir(t *testing.T) {
	srv, env := newWorktreeServer(t)
	repo := gitRepoWithCommit(t)
	id := createProject(t, srv, repo) // folder path → managed=0

	status, body := deleteProject(t, srv, id)
	if status != http.StatusNoContent {
		t.Fatalf("folder delete: status = %d, want 204; body=%v", status, body)
	}
	if _, err := os.Stat(repo); err != nil {
		t.Fatalf("folder dir removed on delete (err=%v); must NEVER be touched (D-09)", err)
	}
	if projectExists(t, env.db, id) {
		t.Fatalf("folder project row still present after delete; want deleted")
	}
}

// TestManagedDeleteUnknownID: deleting a non-existent project → 404 (the marker
// load short-circuits before any git/disk work).
func TestManagedDeleteUnknownID(t *testing.T) {
	srv, _ := newWorktreeServer(t)
	status, body := deleteProject(t, srv, 999999)
	if status != http.StatusNotFound {
		t.Fatalf("unknown id delete: status = %d, want 404; body=%v", status, body)
	}
}

// containsPath reports whether porcelain worktree-list output references path
// (worktree paths are emitted as "worktree <abs>" lines).
func containsPath(porcelain, path string) bool {
	for _, line := range splitLines(porcelain) {
		if line == "worktree "+path {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
