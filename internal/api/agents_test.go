package api

import (
	"database/sql"
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

// newAgentServer wires task routes, session routes, and the agent status
// endpoint over one DB + worktree service + manager, with the manager's agent
// config pointing at the fake-claude stub (agent tests never touch the real
// binary).
func newAgentServer(t *testing.T) (*httptest.Server, *session.Manager, *sql.DB) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}
	wt := worktree.NewService(t.TempDir())
	mgr := session.NewManager()
	mgr.SetAgentConfig(session.AgentConfig{
		BaseURL:   "http://127.0.0.1:7333",
		Token:     testHookToken,
		ClaudeBin: writeFakeClaude(t),
	})
	mux := http.NewServeMux()
	Routes(mux, db, wt)
	SessionRoutes(mux, mgr, db)
	AgentRoutes(mux, mgr, db)
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

// stopAndWait stops a session via the manager and blocks until it has exited.
func stopAndWait(t *testing.T, mgr *session.Manager, id string) {
	t.Helper()
	sess, ok := mgr.Get(id)
	if !ok {
		t.Fatalf("session %q not in manager", id)
	}
	sess.Stop()
	select {
	case <-sess.Done():
	case <-time.After(15 * time.Second):
		t.Fatalf("session %q did not exit after Stop", id)
	}
}

// spawnAgentFor spawns an agent session for a task over REST and returns its
// session id.
func spawnAgentFor(t *testing.T, srv *httptest.Server, taskID int64) string {
	t.Helper()
	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": taskID, "kind": "agent"})
	if status != http.StatusCreated {
		t.Fatalf("agent spawn: status = %d, want 201; body=%v", status, body)
	}
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("agent spawn: no session id in %v", body)
	}
	return id
}

// TestAgentStatusEmptyList: zero agents marshal as JSON [] — never null
// (the sessions list convention; the 04-03 frontend polls this every 5s).
func TestAgentStatusEmptyList(t *testing.T) {
	srv, _, _ := newAgentServer(t)

	resp, err := http.Get(srv.URL + "/api/agents/status")
	if err != nil {
		t.Fatalf("GET /api/agents/status: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, raw)
	}
	if got := strings.TrimSpace(string(raw)); got != "[]" {
		t.Errorf("empty status body = %q, want %q", got, "[]")
	}
}

// TestAgentStatusSingleEntry: one running agent yields exactly one entry with
// the exact 04-03 frontend contract keys; bash sessions never appear (D-48).
func TestAgentStatusSingleEntry(t *testing.T) {
	srv, _, _ := newAgentServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	body := createTask(t, srv, pid, "Status Me")
	id := taskID(t, body)
	if wtPath, _ := body["worktree_path"].(string); wtPath == "" {
		t.Fatalf("task provisioning failed: %v", body)
	}

	// A bash session on the same task must NOT drive board state (D-48).
	if status, b := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": id}); status != http.StatusCreated {
		t.Fatalf("bash spawn: status = %d; body=%v", status, b)
	}
	sid := spawnAgentFor(t, srv, id)

	status, list := doJSONList(t, srv.URL+"/api/agents/status")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(list) != 1 {
		t.Fatalf("entries = %d, want 1 (bash excluded): %v", len(list), list)
	}
	entry := list[0]
	if entry["taskId"] != float64(id) {
		t.Errorf("taskId = %v, want %d", entry["taskId"], id)
	}
	if entry["projectId"] != float64(pid) {
		t.Errorf("projectId = %v, want %d", entry["projectId"], pid)
	}
	if entry["sessionId"] != sid {
		t.Errorf("sessionId = %v, want %q", entry["sessionId"], sid)
	}
	if entry["status"] != "working" {
		t.Errorf("status = %v, want %q", entry["status"], "working")
	}
	if v, present := entry["exitCode"]; !present || v != nil {
		t.Errorf("exitCode = %v (present=%v), want explicit null while running", v, present)
	}
	if entry["stopRequested"] != false {
		t.Errorf("stopRequested = %v, want false", entry["stopRequested"])
	}
}

// TestAgentStatusNewestWins: an exited agent stays listed (UI-SPEC: the
// exited dot persists) until a newer agent for the same task replaces it —
// then exactly ONE entry per task remains.
func TestAgentStatusNewestWins(t *testing.T) {
	srv, mgr, _ := newAgentServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	body := createTask(t, srv, pid, "Replace Me")
	id := taskID(t, body)
	if wtPath, _ := body["worktree_path"].(string); wtPath == "" {
		t.Fatalf("task provisioning failed: %v", body)
	}

	first := spawnAgentFor(t, srv, id)
	stopAndWait(t, mgr, first)

	// Exited agent still listed: server-initiated stop -> exit 143 with
	// stopRequested true (the 04-03 gray-not-red dot discriminator).
	status, list := doJSONList(t, srv.URL+"/api/agents/status")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(list) != 1 {
		t.Fatalf("entries after exit = %d, want 1: %v", len(list), list)
	}
	if list[0]["status"] != "exited" {
		t.Errorf("status = %v, want %q", list[0]["status"], "exited")
	}
	if list[0]["exitCode"] != float64(143) {
		t.Errorf("exitCode = %v, want 143 (SIGTERM exit)", list[0]["exitCode"])
	}
	if list[0]["stopRequested"] != true {
		t.Errorf("stopRequested = %v, want true after server Stop", list[0]["stopRequested"])
	}

	// A fresh agent replaces the exited predecessor: ONE entry, the new one.
	second := spawnAgentFor(t, srv, id)
	_, list = doJSONList(t, srv.URL+"/api/agents/status")
	if len(list) != 1 {
		t.Fatalf("entries after respawn = %d, want exactly 1 per task: %v", len(list), list)
	}
	if list[0]["sessionId"] != second {
		t.Errorf("sessionId = %v, want newest %q", list[0]["sessionId"], second)
	}
	if list[0]["status"] != "working" {
		t.Errorf("status = %v, want %q", list[0]["status"], "working")
	}
}
