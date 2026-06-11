package api

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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
	Routes(mux, db, wt, mgr)
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

// writeTranscriptFixture creates root/<projdir>/<csid>.jsonl so transcriptExists
// hits for csid against globRoot=root.
func writeTranscriptFixture(t *testing.T, root, csid string) {
	t.Helper()
	dir := filepath.Join(root, "-home-x-wt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir transcript dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, csid+".jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write transcript fixture: %v", err)
	}
}

// setTaskClaudeSession stamps tasks.claude_session_id for a task (the
// post-restart DB state recovery reads).
func setTaskClaudeSession(t *testing.T, db *sql.DB, taskID int64, csid string) {
	t.Helper()
	if _, err := db.Exec(`UPDATE tasks SET claude_session_id = ? WHERE id = ?`, csid, taskID); err != nil {
		t.Fatalf("set claude_session_id: %v", err)
	}
}

// statusEntriesDirect invokes the status handler in-package with an injected
// globRoot, returning the decoded entries. It simulates a fresh process by
// taking an explicit (possibly empty) manager.
func statusEntriesDirect(t *testing.T, mgr *session.Manager, db *sql.DB, globRoot string) []map[string]any {
	t.Helper()
	a := &agentHandlers{mgr: mgr, db: db, globRoot: globRoot}
	req := httptest.NewRequest("GET", "/api/agents/status", nil)
	rec := httptest.NewRecorder()
	a.status(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status handler: code = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var entries []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode status body %q: %v", rec.Body.String(), err)
	}
	return entries
}

// TestAgentStatusPostRestartResumable: an EMPTY manager (post-restart) plus a
// task row carrying claude_session_id + worktree_path + a transcript fixture
// yields exactly one DB-derived entry with the D-57 gray-dot shape and
// resumable:true.
func TestAgentStatusPostRestartResumable(t *testing.T) {
	srv, _, db := newAgentServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	body := createTask(t, srv, pid, "Recovered")
	id := taskID(t, body)
	if wtPath, _ := body["worktree_path"].(string); wtPath == "" {
		t.Fatalf("task provisioning failed: %v", body)
	}

	csid := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeee0a01"
	setTaskClaudeSession(t, db, id, csid)
	globRoot := t.TempDir()
	writeTranscriptFixture(t, globRoot, csid)

	// Fresh manager == post-restart: nothing spawned.
	entries := statusEntriesDirect(t, session.NewManager(), db, globRoot)
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1 DB-derived resumable entry: %v", len(entries), entries)
	}
	e := entries[0]
	if e["taskId"] != float64(id) {
		t.Errorf("taskId = %v, want %d", e["taskId"], id)
	}
	if e["projectId"] != float64(pid) {
		t.Errorf("projectId = %v, want %d", e["projectId"], pid)
	}
	if e["sessionId"] != "" {
		t.Errorf("sessionId = %v, want \"\" (no live session post-restart)", e["sessionId"])
	}
	if e["status"] != "exited" {
		t.Errorf("status = %v, want %q", e["status"], "exited")
	}
	if v, present := e["exitCode"]; !present || v != nil {
		t.Errorf("exitCode = %v (present=%v), want explicit null (D-57 gray dot)", v, present)
	}
	if e["stopRequested"] != false {
		t.Errorf("stopRequested = %v, want false", e["stopRequested"])
	}
	if e["resumable"] != true {
		t.Errorf("resumable = %v, want true", e["resumable"])
	}
}

// TestAgentStatusPostRestartNoTranscript: same post-restart shape but with NO
// transcript fixture → empty array. Non-resumable past sessions get no dot
// (D-57 only constrains resumable tasks).
func TestAgentStatusPostRestartNoTranscript(t *testing.T) {
	srv, _, db := newAgentServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	body := createTask(t, srv, pid, "Never Prompted")
	id := taskID(t, body)
	if wtPath, _ := body["worktree_path"].(string); wtPath == "" {
		t.Fatalf("task provisioning failed: %v", body)
	}

	setTaskClaudeSession(t, db, id, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeee0a02")
	globRoot := t.TempDir() // empty — no transcript

	entries := statusEntriesDirect(t, session.NewManager(), db, globRoot)
	if len(entries) != 0 {
		t.Fatalf("entries = %d, want 0 (no transcript -> not resumable -> no dot): %v", len(entries), entries)
	}
}

// TestAgentStatusRunningNotResumable: a RUNNING agent reports resumable:false
// and produces no duplicate DB-derived entry for the same task.
func TestAgentStatusRunningNotResumable(t *testing.T) {
	srv, mgr, db := newAgentServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	body := createTask(t, srv, pid, "Live Work")
	id := taskID(t, body)
	if wtPath, _ := body["worktree_path"].(string); wtPath == "" {
		t.Fatalf("task provisioning failed: %v", body)
	}
	_ = pid

	sid := spawnAgentFor(t, srv, id)
	csid := claudeSessionIDFor(t, db, id)
	if !csid.Valid {
		t.Fatal("claude_session_id NULL after spawn")
	}
	globRoot := t.TempDir()
	writeTranscriptFixture(t, globRoot, csid.String) // even WITH a transcript, a running agent is never resumable

	entries := statusEntriesDirect(t, mgr, db, globRoot)
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want exactly 1 (no DB-derived duplicate): %v", len(entries), entries)
	}
	e := entries[0]
	if e["sessionId"] != sid {
		t.Errorf("sessionId = %v, want the live session %q", e["sessionId"], sid)
	}
	if e["status"] != "working" {
		t.Errorf("status = %v, want %q", e["status"], "working")
	}
	if e["resumable"] != false {
		t.Errorf("resumable = %v, want false for a running agent", e["resumable"])
	}
}

// TestAgentStatusExitedResumable: an EXITED agent still in the manager whose
// task carries a stored id + transcript reports resumable:true with its real
// sessionId/exitCode (the within-run banner needs this for the D-54a pair).
func TestAgentStatusExitedResumable(t *testing.T) {
	srv, mgr, db := newAgentServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	body := createTask(t, srv, pid, "Exited Work")
	id := taskID(t, body)
	if wtPath, _ := body["worktree_path"].(string); wtPath == "" {
		t.Fatalf("task provisioning failed: %v", body)
	}
	_ = pid

	sid := spawnAgentFor(t, srv, id)
	csid := claudeSessionIDFor(t, db, id)
	if !csid.Valid {
		t.Fatal("claude_session_id NULL after spawn")
	}
	stopAndWait(t, mgr, sid)

	globRoot := t.TempDir()
	writeTranscriptFixture(t, globRoot, csid.String)

	entries := statusEntriesDirect(t, mgr, db, globRoot)
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1 (manager-derived exited entry): %v", len(entries), entries)
	}
	e := entries[0]
	if e["sessionId"] != sid {
		t.Errorf("sessionId = %v, want the exited session %q (real id)", e["sessionId"], sid)
	}
	if e["status"] != "exited" {
		t.Errorf("status = %v, want %q", e["status"], "exited")
	}
	if e["exitCode"] != float64(143) {
		t.Errorf("exitCode = %v, want 143 (real exit code, not null)", e["exitCode"])
	}
	if e["resumable"] != true {
		t.Errorf("resumable = %v, want true for an exited agent with a transcript", e["resumable"])
	}
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
