package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kangent/internal/session"
	"kangent/internal/store"
	"kangent/internal/worktree"
)

// newSessionServer starts an httptest server with only the session routes
// registered, backed by a real Manager (bash spawns are cheap). All spawned
// sessions are stopped on cleanup.
func newSessionServer(t *testing.T) (*httptest.Server, *session.Manager) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	mgr := session.NewManager()
	mux := http.NewServeMux()
	SessionRoutes(mux, mgr, db)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		for _, info := range mgr.List() {
			if s, ok := mgr.Get(info.ID); ok {
				s.Stop()
			}
		}
	})
	return srv, mgr
}

// exitSession drives a session to exit and waits for it.
func exitSession(t *testing.T, sess *session.Session) {
	t.Helper()
	if err := sess.WriteInput([]byte("exit\n")); err != nil {
		t.Fatalf("write exit: %v", err)
	}
	select {
	case <-sess.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("session did not exit within 10s")
	}
}

// newTaskSessionServer wires task routes AND session routes over one DB +
// worktree service + manager, for the task-scoped session surface (TERM-04).
func newTaskSessionServer(t *testing.T) (*httptest.Server, *session.Manager) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}
	wt := worktree.NewService(t.TempDir())
	mgr := session.NewManager()
	mux := http.NewServeMux()
	Routes(mux, db, wt)
	SessionRoutes(mux, mgr, db)
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
	return srv, mgr
}

// worktreeTask creates a project on a healthy repo plus a provisioned task,
// returning (taskID, worktreePath).
func worktreeTask(t *testing.T, srv *httptest.Server, title string) (int64, string) {
	t.Helper()
	pid := createProject(t, srv, gitRepoWithCommit(t))
	body := createTask(t, srv, pid, title)
	id := taskID(t, body)
	wtPath, _ := body["worktree_path"].(string)
	if wtPath == "" {
		t.Fatalf("task provisioning failed: %v", body)
	}
	return id, wtPath
}

func TestSessionCreateEmptyBodyDevRouteUnchanged(t *testing.T) {
	srv, _ := newTaskSessionServer(t)

	// The /terminal dev route POSTs with NO body — must keep working.
	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", nil)
	if status != http.StatusCreated {
		t.Fatalf("empty-body create: status = %d, want 201; body=%v", status, body)
	}
	if body["label"] != "bash #1" {
		t.Errorf("label = %q, want %q (dev counter unchanged)", body["label"], "bash #1")
	}
	if _, present := body["taskId"]; present {
		t.Errorf("taskId present on dev session JSON: %v — must be omitted", body["taskId"])
	}
}

func TestSessionSpawnInTaskWorktree(t *testing.T) {
	srv, mgr := newTaskSessionServer(t)
	id, wtPath := worktreeTask(t, srv, "Tabbed Work")

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": id})
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%v", status, body)
	}
	if body["taskId"] != float64(id) {
		t.Errorf("taskId = %v, want %d", body["taskId"], id)
	}
	if body["label"] != "Bash 1" {
		t.Errorf("label = %q, want %q (per-task counter)", body["label"], "Bash 1")
	}

	// Behavioral cwd proof: the shell expands $PWD to the worktree path; the
	// echoed command text only ever contains the literal "$PWD", so a marker
	// match can't be satisfied by input echo.
	sid, _ := body["id"].(string)
	sess, ok := mgr.Get(sid)
	if !ok {
		t.Fatalf("session %q not in manager", sid)
	}
	if err := sess.WriteInput([]byte("echo \"mark:$PWD\"\n")); err != nil {
		t.Fatalf("write input: %v", err)
	}
	want := []byte("mark:" + wtPath)
	deadline := time.Now().Add(10 * time.Second)
	for {
		if bytes.Contains(sess.Snapshot(), want) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("shell cwd is not the worktree: %q never appeared in output:\n%s", want, sess.Snapshot())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestSessionSpawnTaskValidation(t *testing.T) {
	srv, _ := newTaskSessionServer(t)

	// Unknown task → 404.
	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": 424242})
	if status != http.StatusNotFound {
		t.Fatalf("unknown task: status = %d, want 404; body=%v", status, body)
	}
	if body["error"] != "task not found" {
		t.Errorf("error = %q, want %q", body["error"], "task not found")
	}

	// Task with NULL worktree_path → 409 (D-30 server side).
	pid := createProject(t, srv, gitRepo(t)) // unborn HEAD → no worktree
	id := taskID(t, createTask(t, srv, pid, "Treeless"))
	status, body = doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": id})
	if status != http.StatusConflict {
		t.Fatalf("no worktree: status = %d, want 409; body=%v", status, body)
	}
	if body["error"] != "task has no worktree" {
		t.Errorf("error = %q, want %q", body["error"], "task has no worktree")
	}
}

func TestSessionListTaskFilter(t *testing.T) {
	srv, mgr := newTaskSessionServer(t)
	id, _ := worktreeTask(t, srv, "Filtered")

	// One dev session + two task sessions.
	if status, body := doJSON(t, "POST", srv.URL+"/api/sessions", nil); status != http.StatusCreated {
		t.Fatalf("dev spawn: status = %d; body=%v", status, body)
	}
	var taskSessionIDs []string
	for i := 0; i < 2; i++ {
		status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": id})
		if status != http.StatusCreated {
			t.Fatalf("task spawn %d: status = %d; body=%v", i, status, body)
		}
		taskSessionIDs = append(taskSessionIDs, body["id"].(string))
	}

	// Filtered list → exactly the task's sessions.
	status, list := doJSONList(t, fmt.Sprintf("%s/api/sessions?task_id=%d", srv.URL, id))
	if status != http.StatusOK {
		t.Fatalf("filtered list: status = %d, want 200", status)
	}
	if len(list) != 2 {
		t.Fatalf("filtered len = %d, want 2: %v", len(list), list)
	}
	for _, item := range list {
		if item["taskId"] != float64(id) {
			t.Errorf("filtered item taskId = %v, want %d", item["taskId"], id)
		}
	}

	// Exited task sessions stay in the filtered list (exited-ghost handling
	// is client-side per D-28).
	sess, ok := mgr.Get(taskSessionIDs[0])
	if !ok {
		t.Fatalf("session %q not in manager", taskSessionIDs[0])
	}
	exitSession(t, sess)
	_, list = doJSONList(t, fmt.Sprintf("%s/api/sessions?task_id=%d", srv.URL, id))
	if len(list) != 2 {
		t.Errorf("filtered len after exit = %d, want 2 (exited included)", len(list))
	}

	// Unfiltered list → everything.
	status, list = doJSONList(t, srv.URL+"/api/sessions")
	if status != http.StatusOK {
		t.Fatalf("unfiltered list: status = %d, want 200", status)
	}
	if len(list) != 3 {
		t.Errorf("unfiltered len = %d, want 3", len(list))
	}

	// Non-integer task_id → 400.
	status, body := doJSON(t, "GET", srv.URL+"/api/sessions?task_id=abc", nil)
	if status != http.StatusBadRequest {
		t.Fatalf("bad task_id: status = %d, want 400; body=%v", status, body)
	}
	if body["error"] != "invalid task_id" {
		t.Errorf("error = %q, want %q", body["error"], "invalid task_id")
	}

	// Empty filtered result is JSON [] — never null.
	resp, err := http.Get(srv.URL + "/api/sessions?task_id=999999")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if got := strings.TrimSpace(string(raw)); got != "[]" {
		t.Errorf("empty filtered body = %q, want %q", got, "[]")
	}
}

func TestSessionListEmpty(t *testing.T) {
	srv, _ := newSessionServer(t)

	resp, err := http.Get(srv.URL + "/api/sessions")
	if err != nil {
		t.Fatalf("GET /api/sessions: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, raw)
	}
	// Empty list must be JSON [] — never null.
	if got := strings.TrimSpace(string(raw)); got != "[]" {
		t.Errorf("empty list body = %q, want %q", got, "[]")
	}
}

func TestSessionCreateAndList(t *testing.T) {
	srv, _ := newSessionServer(t)

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", nil)
	if status != http.StatusCreated {
		t.Fatalf("create: status = %d, want 201; body=%v", status, body)
	}
	if id, ok := body["id"].(string); !ok || id == "" {
		t.Errorf("id = %v, want non-empty string", body["id"])
	}
	if body["label"] != "bash #1" {
		t.Errorf("label = %q, want %q", body["label"], "bash #1")
	}
	if body["status"] != "running" {
		t.Errorf("status = %q, want %q", body["status"], "running")
	}
	if created, ok := body["createdAt"].(string); !ok || created == "" {
		t.Errorf("createdAt = %v, want non-empty string", body["createdAt"])
	}

	listStatus, list := doJSONList(t, srv.URL+"/api/sessions")
	if listStatus != http.StatusOK {
		t.Fatalf("list: status = %d, want 200", listStatus)
	}
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}
	if list[0]["id"] != body["id"] {
		t.Errorf("listed id = %v, want %v", list[0]["id"], body["id"])
	}
}

func TestSessionStop(t *testing.T) {
	srv, mgr := newSessionServer(t)

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", nil)
	if status != http.StatusCreated {
		t.Fatalf("create: status = %d; body=%v", status, body)
	}
	id := body["id"].(string)

	status, _ = doJSON(t, "POST", srv.URL+"/api/sessions/"+id+"/stop", nil)
	if status != http.StatusAccepted {
		t.Fatalf("stop: status = %d, want 202", status)
	}

	// Stop is async (202) — wait for the session to actually exit.
	sess, ok := mgr.Get(id)
	if !ok {
		t.Fatal("session vanished from manager")
	}
	select {
	case <-sess.Done():
	case <-time.After(15 * time.Second):
		t.Fatal("session did not exit after stop")
	}

	// Idempotent: stopping an already-exited session is still 202.
	status, _ = doJSON(t, "POST", srv.URL+"/api/sessions/"+id+"/stop", nil)
	if status != http.StatusAccepted {
		t.Fatalf("repeated stop: status = %d, want 202", status)
	}

	// Unknown id → 404 with the {"error"} contract.
	status, body = doJSON(t, "POST", srv.URL+"/api/sessions/nonexistent/stop", nil)
	if status != http.StatusNotFound {
		t.Fatalf("unknown stop: status = %d, want 404", status)
	}
	if body["error"] != "session not found" {
		t.Errorf("error = %q, want %q", body["error"], "session not found")
	}
}

func TestSessionDelete(t *testing.T) {
	srv, mgr := newSessionServer(t)

	// Running session → 409.
	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", nil)
	if status != http.StatusCreated {
		t.Fatalf("create: status = %d; body=%v", status, body)
	}
	runningID := body["id"].(string)
	status, body = doJSON(t, "DELETE", srv.URL+"/api/sessions/"+runningID, nil)
	if status != http.StatusConflict {
		t.Fatalf("delete running: status = %d, want 409; body=%v", status, body)
	}
	if body["error"] != "session is still running. Stop it first." {
		t.Errorf("error = %q, want %q", body["error"], "session is still running. Stop it first.")
	}

	// Exited session → 204, and it disappears from the list.
	sess, ok := mgr.Get(runningID)
	if !ok {
		t.Fatal("session vanished from manager")
	}
	exitSession(t, sess)
	status, _ = doJSON(t, "DELETE", srv.URL+"/api/sessions/"+runningID, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete exited: status = %d, want 204", status)
	}
	listStatus, list := doJSONList(t, srv.URL+"/api/sessions")
	if listStatus != http.StatusOK {
		t.Fatalf("list: status = %d, want 200", listStatus)
	}
	for _, item := range list {
		if item["id"] == runningID {
			t.Errorf("deleted session %s still listed", runningID)
		}
	}

	// Unknown id → 404.
	status, body = doJSON(t, "DELETE", srv.URL+"/api/sessions/nonexistent", nil)
	if status != http.StatusNotFound {
		t.Fatalf("delete unknown: status = %d, want 404; body=%v", status, body)
	}
	if body["error"] != "session not found" {
		t.Errorf("error = %q, want %q", body["error"], "session not found")
	}
}
