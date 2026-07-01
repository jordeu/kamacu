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

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"kamacu/internal/session"
	"kamacu/internal/settings"
	"kamacu/internal/store"
	"kamacu/internal/tmux"
	"kamacu/internal/worktree"
	"kamacu/internal/ws"
)

// agentLifecycleToken is the per-instance hook token the lifecycle harness is
// configured with (production generates one with crypto/rand at startup).
const agentLifecycleToken = "test-token"

// testdataFakeClaude returns the absolute path to the committed fake-claude
// stub (spawned via AgentConfig.ClaudeBin — this suite NEVER invokes the real
// binary: auth + API cost). The executable bit is re-applied defensively
// because git permission bits can drop on some checkouts.
func testdataFakeClaude(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", "fake-claude"))
	if err != nil {
		t.Fatalf("resolve testdata/fake-claude: %v", err)
	}
	if err := os.Chmod(abs, 0o755); err != nil {
		t.Fatalf("chmod testdata/fake-claude: %v", err)
	}
	return abs
}

// newAgentIntegrationServer wires EVERY route family the production binary
// registers — Routes + SessionRoutes + WorktreeRoutes + HookRoutes +
// AgentRoutes + the WS attach handler — over one DB, one worktree.Service,
// and one session.Manager whose AgentConfig points at the fake-claude stub.
// If cmd/kangent/main.go wiring drifts, this harness catches it.
func newAgentIntegrationServer(t *testing.T) (*httptest.Server, *session.Manager, *sql.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}
	wtDir := t.TempDir()
	// Phase 6: provisioning reads worktree_base at use — seed the temp dir.
	if err := settings.Set(db, settings.KeyWorktreeBase, wtDir); err != nil {
		t.Fatalf("seed worktree_base: %v", err)
	}
	wt := worktree.NewService(wtDir)
	mgr := session.NewManager()
	mux := http.NewServeMux()
	Routes(mux, db, wt, mgr, tmux.Client{})
	SessionRoutes(mux, mgr, db, tmux.Client{})
	WorktreeRoutes(mux, db, wt, mgr, tmux.Client{})
	HookRoutes(mux, mgr, agentLifecycleToken)
	AgentRoutes(mux, mgr, db)
	mux.Handle("GET /api/sessions/{id}/ws", ws.NewHandler(mgr, nil, false))
	srv := httptest.NewServer(mux)
	mgr.SetAgentConfig(session.AgentConfig{
		BaseURL:   srv.URL, // overlay hook URLs point back at this harness
		Token:     agentLifecycleToken,
		ClaudeBin: testdataFakeClaude(t),
	})
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

// taskClaudeSessionID reads tasks.claude_session_id directly from the DB —
// the Phase 5 --resume key persisted at agent spawn.
func taskClaudeSessionID(t *testing.T, db *sql.DB, taskID int64) string {
	t.Helper()
	var s sql.NullString
	if err := db.QueryRow(`SELECT claude_session_id FROM tasks WHERE id = ?`, taskID).Scan(&s); err != nil {
		t.Fatalf("query claude_session_id for task %d: %v", taskID, err)
	}
	if !s.Valid {
		t.Fatalf("tasks.claude_session_id is NULL for task %d, want a uuid", taskID)
	}
	return s.String
}

// agentStatusForTask returns the GET /api/agents/status entry for taskID, or
// nil when the endpoint lists no entry for it.
func agentStatusForTask(t *testing.T, srv *httptest.Server, taskID int64) map[string]any {
	t.Helper()
	status, list := doJSONList(t, srv.URL+"/api/agents/status")
	if status != http.StatusOK {
		t.Fatalf("GET /api/agents/status = %d, want 200", status)
	}
	for _, e := range list {
		if e["taskId"] == float64(taskID) {
			return e
		}
	}
	return nil
}

// waitAgentStatus polls /api/agents/status until the task's entry reports
// want, failing after timeout. Returns the matching entry.
func waitAgentStatus(t *testing.T, srv *httptest.Server, taskID int64, want string, timeout time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		entry := agentStatusForTask(t, srv, taskID)
		if entry != nil && entry["status"] == want {
			return entry
		}
		if time.Now().After(deadline) {
			t.Fatalf("agents status for task %d never reached %q within %s; last entry: %v", taskID, want, timeout, entry)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestAgentLifecycle proves Phase 4 end to end over the real REST/WS surface
// against the committed fake-claude stub: spawn → hook POSTs → status
// transitions → attach-clears-waiting (D-45) → stop → exited(143,
// stopRequested) → fresh restart, plus both security/leak gap fixes (hook
// token gate, task-delete stops sessions). TERM-01 / STAT-01 / STAT-02.
func TestAgentLifecycle(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	srv, _, db := newAgentIntegrationServer(t)

	// Repo on the OUTER t: subtest TempDirs are removed when the subtest
	// ends, which would delete .git/worktrees bookkeeping under later stages.
	repo := gitRepoWithCommit(t)

	var (
		pid            int64
		tid            int64
		agentSessionID string
		firstClaudeID  string
	)

	// stage aborts the lifecycle on failure — later stages depend on state.
	stage := func(name string, fn func(t *testing.T)) {
		t.Helper()
		if !t.Run(name, fn) {
			t.FailNow()
		}
	}

	stage("1_spawn_agent_in_worktree", func(t *testing.T) {
		pid = createProject(t, srv, repo)
		body := createTask(t, srv, pid, "Agent Work")
		tid = taskID(t, body)
		if wt, _ := body["worktree_path"].(string); wt == "" {
			t.Fatalf("task has no worktree_path — agent spawn needs one: %v", body)
		}

		status, sess := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "agent"})
		if status != http.StatusCreated {
			t.Fatalf("agent spawn status = %d, want 201; body=%v", status, sess)
		}
		if sess["label"] != "Agent" {
			t.Errorf("label = %q, want %q", sess["label"], "Agent")
		}
		if sess["kind"] != "agent" {
			t.Errorf("kind = %q, want %q", sess["kind"], "agent")
		}
		if sess["agentStatus"] != "working" {
			t.Errorf("agentStatus = %q, want %q (spawn -> working)", sess["agentStatus"], "working")
		}
		agentSessionID, _ = sess["id"].(string)
		if agentSessionID == "" {
			t.Fatalf("session id missing: %v", sess)
		}

		// The Phase 5 --resume key is persisted before the 201 reply.
		firstClaudeID = taskClaudeSessionID(t, db, tid)
		if _, err := uuid.Parse(firstClaudeID); err != nil {
			t.Fatalf("tasks.claude_session_id %q is not a parseable uuid: %v", firstClaudeID, err)
		}
	})

	stage("2_one_agent_per_task", func(t *testing.T) {
		status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "agent"})
		if status != http.StatusConflict {
			t.Fatalf("second agent spawn status = %d, want 409; body=%v", status, body)
		}
		if msg, _ := body["error"].(string); !strings.Contains(msg, "agent session already running") {
			t.Errorf("409 body = %q, want it to contain %q", msg, "agent session already running")
		}
	})

	stage("3_hook_token_gate", func(t *testing.T) {
		url := srv.URL + "/api/hooks/sessions/" + agentSessionID
		body := fmt.Sprintf(`{"hook_event_name":"SessionStart","session_id":%q}`, firstClaudeID)

		// Gap #2 (localhost hook spoofing): no token and a wrong token are
		// both 401 — any webpage can fire a no-CORS POST at localhost.
		if got := postHook(t, url, "", body); got != http.StatusUnauthorized {
			t.Errorf("no token: status = %d, want 401", got)
		}
		if got := postHook(t, url, "wrong-token", body); got != http.StatusUnauthorized {
			t.Errorf("wrong token: status = %d, want 401", got)
		}
		if got := postHook(t, url, agentLifecycleToken, body); got != http.StatusNoContent {
			t.Fatalf("correct token SessionStart: status = %d, want 204", got)
		}
	})

	stage("4_notification_hook_sets_waiting", func(t *testing.T) {
		url := srv.URL + "/api/hooks/sessions/" + agentSessionID
		body := `{"hook_event_name":"Notification","notification_type":"permission_prompt"}`
		if got := postHook(t, url, agentLifecycleToken, body); got != http.StatusNoContent {
			t.Fatalf("Notification hook: status = %d, want 204", got)
		}

		entry := agentStatusForTask(t, srv, tid)
		if entry == nil {
			t.Fatalf("GET /api/agents/status has no entry for task %d", tid)
		}
		if entry["status"] != "waiting" {
			t.Errorf("status = %q, want %q (sticky until answered/attached)", entry["status"], "waiting")
		}
		if entry["sessionId"] != agentSessionID {
			t.Errorf("sessionId = %v, want %q", entry["sessionId"], agentSessionID)
		}
		if entry["projectId"] != float64(pid) {
			t.Errorf("projectId = %v, want %d", entry["projectId"], pid)
		}
	})

	stage("5_attach_clears_waiting", func(t *testing.T) {
		ctx := testWSCtx(t)
		conn, _, err := websocket.Dial(ctx, srv.URL+"/api/sessions/"+agentSessionID+"/ws", nil)
		if err != nil {
			t.Fatalf("dial agent WS: %v", err)
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		// D-45: the handler clears waiting right after Attach; the dial
		// returning only guarantees the upgrade, so poll briefly.
		waitAgentStatus(t, srv, tid, "idle", 2*time.Second)
	})

	stage("6_stop_exits_143_stop_requested", func(t *testing.T) {
		status, body := doJSON(t, "POST", srv.URL+"/api/sessions/"+agentSessionID+"/stop", nil)
		if status != http.StatusAccepted {
			t.Fatalf("stop status = %d, want 202; body=%v", status, body)
		}

		// Stop blocks up to the 5s SIGTERM grace; fake-claude traps TERM and
		// exits ~instantly, but budget the full grace plus slack.
		entry := waitAgentStatus(t, srv, tid, "exited", 7*time.Second)
		if entry["exitCode"] != float64(143) {
			t.Errorf("exitCode = %v, want 143 (SIGTERM trap)", entry["exitCode"])
		}
		// Gap #3 (gray-vs-red semantics): a server-initiated stop reports
		// stopRequested true — the client maps exited+stopRequested to the
		// muted-gray dot, never red.
		if entry["stopRequested"] != true {
			t.Errorf("stopRequested = %v, want true (server-initiated stop renders gray)", entry["stopRequested"])
		}
	})

	stage("7_fresh_restart_overwrites_resume_key", func(t *testing.T) {
		// An exited agent no longer blocks the one-per-task gate (D-41).
		status, sess := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "agent"})
		if status != http.StatusCreated {
			t.Fatalf("restart spawn status = %d, want 201; body=%v", status, sess)
		}
		newSessionID, _ := sess["id"].(string)
		if newSessionID == "" || newSessionID == agentSessionID {
			t.Fatalf("restart session id = %q, want a fresh id (old %q)", newSessionID, agentSessionID)
		}

		secondClaudeID := taskClaudeSessionID(t, db, tid)
		if _, err := uuid.Parse(secondClaudeID); err != nil {
			t.Fatalf("restart claude_session_id %q is not a parseable uuid: %v", secondClaudeID, err)
		}
		if secondClaudeID == firstClaudeID {
			t.Errorf("claude_session_id not overwritten on restart: still %q (latest spawn must win)", firstClaudeID)
		}

		// Newest-wins: the status endpoint now reports the NEW session.
		entry := waitAgentStatus(t, srv, tid, "working", 2*time.Second)
		if entry["sessionId"] != newSessionID {
			t.Errorf("agents status sessionId = %v, want the new session %q", entry["sessionId"], newSessionID)
		}
	})

	stage("8_task_delete_stops_sessions", func(t *testing.T) {
		// Gap #1 (deleted task leaks a headless claude): with the restarted
		// agent AND a bash session both running, DELETE /api/tasks/{id} must
		// stop everything before the row goes.
		status, bash := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": tid})
		if status != http.StatusCreated {
			t.Fatalf("bash spawn status = %d, want 201; body=%v", status, bash)
		}

		status, body := doJSON(t, "DELETE", fmt.Sprintf("%s/api/tasks/%d", srv.URL, tid), nil)
		if status != http.StatusNoContent {
			t.Fatalf("task delete status = %d, want 204; body=%v", status, body)
		}

		status, list := doJSONList(t, fmt.Sprintf("%s/api/sessions?task_id=%d", srv.URL, tid))
		if status != http.StatusOK {
			t.Fatalf("sessions list status = %d, want 200", status)
		}
		for _, s := range list {
			if s["status"] == string(session.StatusRunning) {
				t.Errorf("session %v still running after task delete — leaked", s["id"])
			}
		}
	})
}

// testWSCtx returns a context bounded well under the test timeout.
func testWSCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}
