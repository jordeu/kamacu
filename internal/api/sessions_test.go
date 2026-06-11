package api

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

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
	Routes(mux, db, wt, mgr)
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

// newResumeServer wires the session routes (with an injected globRoot) +
// task/worktree routes over one DB and a manager whose ClaudeBin points at the
// committed testdata/fake-claude stub. FAKE_CLAUDE_ARGS_FILE is set so each
// agent spawn records its argv (D-52 inherit-all env), and globRoot is a temp
// dir the caller seeds with transcript fixtures. Returns the server, manager,
// db, the injected globRoot, and the argv file path.
func newResumeServer(t *testing.T) (*httptest.Server, *session.Manager, *sql.DB, string, string) {
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
		ClaudeBin: testdataFakeClaude(t),
	})
	globRoot := t.TempDir()
	argsFile := filepath.Join(t.TempDir(), "args")
	// Agents inherit the test process env (D-52) — point the stub's recorder
	// at our file. t.Setenv restores it on cleanup.
	t.Setenv("FAKE_CLAUDE_ARGS_FILE", argsFile)

	mux := http.NewServeMux()
	Routes(mux, db, wt, mgr) // includes SettingsRoutes — spawn tests PUT settings
	// Session routes with the injected glob root (SessionRoutes signature is
	// unchanged in production; tests construct the handler directly).
	s := &sessionHandlers{mgr: mgr, db: db, globRoot: globRoot}
	mux.HandleFunc("GET /api/sessions", s.list)
	mux.HandleFunc("POST /api/sessions", s.create)
	mux.HandleFunc("POST /api/sessions/{id}/stop", s.stop)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.delete)
	AgentRoutes(mux, mgr, db)
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
	return srv, mgr, db, globRoot, argsFile
}

// seedTranscript writes globRoot/-home-x-wt/<csid>.jsonl so transcriptExists
// hits.
func seedTranscript(t *testing.T, globRoot, csid string) {
	t.Helper()
	dir := filepath.Join(globRoot, "-home-x-wt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir transcript dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, csid+".jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write transcript fixture: %v", err)
	}
}

// readArgv polls the argv file (≤3s) and returns the recorded args.
func readArgv(t *testing.T, argsFile string) []string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if b, err := os.ReadFile(argsFile); err == nil && len(b) > 0 {
			return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
		}
		if time.Now().After(deadline) {
			t.Fatalf("stub never recorded argv at %s", argsFile)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestSessionResumeArgvAndPersistence: a resume request on a task with a
// worktree + stored claude_session_id + transcript spawns claude --resume
// <stored id> and leaves tasks.claude_session_id unchanged (the UPDATE
// rewrites the same value).
func TestSessionResumeArgvAndPersistence(t *testing.T) {
	srv, _, db, globRoot, argsFile := newResumeServer(t)
	id, _ := worktreeTask(t, srv, "Resume Me")

	stored := uuid.NewString()
	setTaskClaudeSession(t, db, id, stored)
	seedTranscript(t, globRoot, stored)

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": id, "kind": "agent", "resume": true})
	if status != http.StatusCreated {
		t.Fatalf("resume: status = %d, want 201; body=%v", status, body)
	}

	args := readArgv(t, argsFile)
	if len(args) < 2 || args[0] != "--resume" {
		t.Fatalf("argv[0] = %v, want --resume; argv=%v", args, args)
	}
	if args[1] != stored {
		t.Errorf("argv[1] = %q, want the STORED uuid %q", args[1], stored)
	}

	after := claudeSessionIDFor(t, db, id)
	if !after.Valid || after.String != stored {
		t.Errorf("claude_session_id = %v after resume, want unchanged %q", after, stored)
	}
}

// TestSessionResumeNoSession: resume with a NULL claude_session_id, or a
// stored id whose transcript is missing, both return 409 "no session to
// resume".
func TestSessionResumeNoSession(t *testing.T) {
	srv, _, db, globRoot, _ := newResumeServer(t)

	// (a) NULL claude_session_id.
	id, _ := worktreeTask(t, srv, "No Stored Id")
	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": id, "kind": "agent", "resume": true})
	if status != http.StatusConflict {
		t.Fatalf("resume NULL id: status = %d, want 409; body=%v", status, body)
	}
	if body["error"] != "no session to resume" {
		t.Errorf("error = %q, want %q", body["error"], "no session to resume")
	}

	// (b) Stored id but NO transcript fixture.
	id2, _ := worktreeTask(t, srv, "No Transcript")
	setTaskClaudeSession(t, db, id2, uuid.NewString())
	_ = globRoot // intentionally not seeded
	status, body = doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": id2, "kind": "agent", "resume": true})
	if status != http.StatusConflict {
		t.Fatalf("resume no transcript: status = %d, want 409; body=%v", status, body)
	}
	if body["error"] != "no session to resume" {
		t.Errorf("error = %q, want %q", body["error"], "no session to resume")
	}
}

// TestSessionResumeGoesThroughOnePerTaskGate: resume while an agent already
// runs for the task hits the existing one-agent-per-task 409 BEFORE resume
// validation (D-67 never-two-PTYs).
func TestSessionResumeGoesThroughOnePerTaskGate(t *testing.T) {
	srv, _, db, globRoot, _ := newResumeServer(t)
	id, _ := worktreeTask(t, srv, "Already Running")

	// Fresh spawn occupies the one-agent slot and persists a stored id.
	if status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": id, "kind": "agent"}); status != http.StatusCreated {
		t.Fatalf("first spawn: status = %d; body=%v", status, body)
	}
	stored := claudeSessionIDFor(t, db, id)
	if !stored.Valid {
		t.Fatal("claude_session_id NULL after first spawn")
	}
	seedTranscript(t, globRoot, stored.String) // resume would otherwise be valid — prove the gate fires first

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": id, "kind": "agent", "resume": true})
	if status != http.StatusConflict {
		t.Fatalf("resume while running: status = %d, want 409; body=%v", status, body)
	}
	if body["error"] != "agent session already running" {
		t.Errorf("error = %q, want %q (gate before resume validation)", body["error"], "agent session already running")
	}
}

// TestSessionResumeRequiresAgentKind: {"resume":true} with kind "" or "bash"
// is a 400 "resume requires kind agent".
func TestSessionResumeRequiresAgentKind(t *testing.T) {
	srv, _, _, _, _ := newResumeServer(t)
	id, _ := worktreeTask(t, srv, "Wrong Kind")

	for _, kind := range []any{nil, "bash"} {
		req := map[string]any{"task_id": id, "resume": true}
		if kind != nil {
			req["kind"] = kind
		}
		status, body := doJSON(t, "POST", srv.URL+"/api/sessions", req)
		if status != http.StatusBadRequest {
			t.Fatalf("resume kind=%v: status = %d, want 400; body=%v", kind, status, body)
		}
		if body["error"] != "resume requires kind agent" {
			t.Errorf("kind=%v error = %q, want %q", kind, body["error"], "resume requires kind agent")
		}
	}
}

// TestSessionFreshSpawnUnchangedByResumeField: a fresh agent spawn (resume
// absent) still 201s with a NEW uuid persisted — the regression guard.
func TestSessionFreshSpawnUnchangedByResumeField(t *testing.T) {
	srv, _, db, _, argsFile := newResumeServer(t)
	id, _ := worktreeTask(t, srv, "Fresh Spawn")

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": id, "kind": "agent"})
	if status != http.StatusCreated {
		t.Fatalf("fresh spawn: status = %d, want 201; body=%v", status, body)
	}
	args := readArgv(t, argsFile)
	if len(args) < 2 || args[0] != "--session-id" {
		t.Fatalf("fresh argv[0] = %v, want --session-id; argv=%v", args, args)
	}
	if _, err := uuid.Parse(args[1]); err != nil {
		t.Errorf("fresh argv[1] = %q is not a uuid: %v", args[1], err)
	}
	persisted := claudeSessionIDFor(t, db, id)
	if !persisted.Valid || persisted.String != args[1] {
		t.Errorf("persisted claude_session_id = %v, want the minted uuid %q", persisted, args[1])
	}
}

// TestSessionAgentDefaultExtraParamsInArgv: with NO settings rows stored, an
// agent spawn carries the default --dangerously-skip-permissions AFTER the
// fixed flags (AGENT-01/02 default-on — the deliberate D-51 reversal).
func TestSessionAgentDefaultExtraParamsInArgv(t *testing.T) {
	srv, _, _, _, argsFile := newResumeServer(t)
	id, _ := worktreeTask(t, srv, "Default Extras")

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": id, "kind": "agent"})
	if status != http.StatusCreated {
		t.Fatalf("agent spawn: status = %d, want 201; body=%v", status, body)
	}

	args := readArgv(t, argsFile)
	if len(args) != 5 {
		t.Fatalf("argv = %q, want 5 args (--session-id <uuid> --settings <json> --dangerously-skip-permissions)", args)
	}
	if args[2] != "--settings" {
		t.Errorf("argv[2] = %q, want --settings", args[2])
	}
	if args[4] != "--dangerously-skip-permissions" {
		t.Errorf("argv[4] = %q, want the default extra param after the fixed flags", args[4])
	}
}

// TestSessionAgentExtraParamsRemovable: PUT "" for agent_extra_params, then
// spawn — the NEXT spawn has zero extras with no restart (AGENT-02
// removability + SET-03 next-spawn semantics; Pitfall 1: stored "" is a real
// value, never re-defaulted).
func TestSessionAgentExtraParamsRemovable(t *testing.T) {
	srv, _, _, _, argsFile := newResumeServer(t)
	id, _ := worktreeTask(t, srv, "No Extras")

	status, body := doJSON(t, "PUT", srv.URL+"/api/settings/agent_extra_params", map[string]any{"value": ""})
	if status != http.StatusOK {
		t.Fatalf("PUT agent_extra_params \"\": status = %d, want 200; body=%v", status, body)
	}

	status, body = doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": id, "kind": "agent"})
	if status != http.StatusCreated {
		t.Fatalf("agent spawn: status = %d, want 201; body=%v", status, body)
	}

	args := readArgv(t, argsFile)
	if len(args) != 4 {
		t.Fatalf("argv = %q, want exactly 4 args — stored \"\" must yield zero extras", args)
	}
	for _, a := range args {
		if a == "--dangerously-skip-permissions" {
			t.Errorf("argv still carries --dangerously-skip-permissions after the flag was removed: %q", args)
		}
	}
}

// TestSessionAgentResumeCarriesExtraParams: a resume spawn appends the same
// settings extras — AGENT-01 covers EVERY claude spawn, not just fresh ones.
func TestSessionAgentResumeCarriesExtraParams(t *testing.T) {
	srv, _, db, globRoot, argsFile := newResumeServer(t)
	id, _ := worktreeTask(t, srv, "Resume Extras")

	stored := uuid.NewString()
	setTaskClaudeSession(t, db, id, stored)
	seedTranscript(t, globRoot, stored)

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": id, "kind": "agent", "resume": true})
	if status != http.StatusCreated {
		t.Fatalf("resume spawn: status = %d, want 201; body=%v", status, body)
	}

	args := readArgv(t, argsFile)
	if len(args) != 5 {
		t.Fatalf("argv = %q, want 5 args (--resume <uuid> --settings <json> --dangerously-skip-permissions)", args)
	}
	if args[0] != "--resume" || args[1] != stored {
		t.Errorf("argv[0:2] = %q, want [--resume %s]", args[:2], stored)
	}
	if args[4] != "--dangerously-skip-permissions" {
		t.Errorf("argv[4] = %q, want the extra param on the resume spawn too", args[4])
	}
}

// TestSessionBashShellReadAtUse: the handler reads the shell setting from the
// DB at every spawn (SET-03). A hand-corrupted row (raw INSERT bypassing Set's
// validation) fails the spawn cleanly; fixing the row makes the very next
// spawn succeed — no restart, no caching.
func TestSessionBashShellReadAtUse(t *testing.T) {
	srv, _, db, _, _ := newResumeServer(t)

	if _, err := db.Exec(`INSERT INTO settings(key, value) VALUES('shell', 'no-such-shell-xyz')`); err != nil {
		t.Fatalf("raw settings insert: %v", err)
	}
	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", nil)
	if status != http.StatusInternalServerError {
		t.Fatalf("bash spawn with broken shell: status = %d, want 500; body=%v", status, body)
	}
	if body["error"] != "couldn't start a session" {
		t.Errorf("error = %q, want %q", body["error"], "couldn't start a session")
	}

	if _, err := db.Exec(`UPDATE settings SET value = 'bash' WHERE key = 'shell'`); err != nil {
		t.Fatalf("settings update: %v", err)
	}
	status, body = doJSON(t, "POST", srv.URL+"/api/sessions", nil)
	if status != http.StatusCreated {
		t.Fatalf("bash spawn after fixing shell: status = %d, want 201 (read-at-use, no restart); body=%v", status, body)
	}
}

// claudeSessionIDFor reads tasks.claude_session_id for a task.
func claudeSessionIDFor(t *testing.T, db *sql.DB, taskID int64) sql.NullString {
	t.Helper()
	var csid sql.NullString
	if err := db.QueryRow(`SELECT claude_session_id FROM tasks WHERE id = ?`, taskID).Scan(&csid); err != nil {
		t.Fatalf("read claude_session_id: %v", err)
	}
	return csid
}

func TestSessionAgentSpawn(t *testing.T) {
	srv, _, db := newAgentServer(t)
	id, _ := worktreeTask(t, srv, "Agent Work")

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": id, "kind": "agent"})
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%v", status, body)
	}
	if body["label"] != "Agent" {
		t.Errorf("label = %q, want %q", body["label"], "Agent")
	}
	if body["kind"] != "agent" {
		t.Errorf("kind = %q, want %q", body["kind"], "agent")
	}
	if body["agentStatus"] != "working" {
		t.Errorf("agentStatus = %q, want %q (spawn -> working)", body["agentStatus"], "working")
	}

	// The claude session id is persisted on the task row BEFORE the reply —
	// the Phase 5 --resume key.
	csid := claudeSessionIDFor(t, db, id)
	if !csid.Valid {
		t.Fatal("tasks.claude_session_id is NULL after agent spawn")
	}
	if _, err := uuid.Parse(csid.String); err != nil {
		t.Errorf("claude_session_id %q does not parse as a uuid: %v", csid.String, err)
	}
}

func TestSessionAgentOnePerTask(t *testing.T) {
	srv, _, _ := newAgentServer(t)
	id, _ := worktreeTask(t, srv, "One Agent")

	if status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": id, "kind": "agent"}); status != http.StatusCreated {
		t.Fatalf("first spawn: status = %d; body=%v", status, body)
	}
	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": id, "kind": "agent"})
	if status != http.StatusConflict {
		t.Fatalf("second spawn: status = %d, want 409; body=%v", status, body)
	}
	if body["error"] != "agent session already running" {
		t.Errorf("error = %q, want %q", body["error"], "agent session already running")
	}
}

func TestSessionAgentRestartAfterExit(t *testing.T) {
	srv, mgr, db := newAgentServer(t)
	id, _ := worktreeTask(t, srv, "Start Again")

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": id, "kind": "agent"})
	if status != http.StatusCreated {
		t.Fatalf("first spawn: status = %d; body=%v", status, body)
	}
	first := claudeSessionIDFor(t, db, id)
	if !first.Valid {
		t.Fatal("claude_session_id NULL after first spawn")
	}
	stopAndWait(t, mgr, body["id"].(string))

	// "Reset session" (D-41, revised at checkpoint): a FRESH session after the first exited — 201, and
	// the persisted claude_session_id is overwritten (latest wins).
	status, body = doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": id, "kind": "agent"})
	if status != http.StatusCreated {
		t.Fatalf("respawn after exit: status = %d, want 201; body=%v", status, body)
	}
	second := claudeSessionIDFor(t, db, id)
	if !second.Valid {
		t.Fatal("claude_session_id NULL after respawn")
	}
	if _, err := uuid.Parse(second.String); err != nil {
		t.Errorf("respawned claude_session_id %q does not parse as a uuid: %v", second.String, err)
	}
	if second.String == first.String {
		t.Errorf("claude_session_id not overwritten on respawn: still %q", first.String)
	}
}

func TestSessionAgentValidation(t *testing.T) {
	srv, _, _ := newAgentServer(t)

	// kind=agent without task_id: agents always need a worktree — same gate
	// as a worktree-less task (D-30 wording contract).
	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"kind": "agent"})
	if status != http.StatusConflict {
		t.Fatalf("agent without task_id: status = %d, want 409; body=%v", status, body)
	}
	if body["error"] != "task has no worktree" {
		t.Errorf("error = %q, want %q", body["error"], "task has no worktree")
	}

	// kind=agent on a task with NULL worktree_path (unborn HEAD repo).
	pid := createProject(t, srv, gitRepo(t))
	id := taskID(t, createTask(t, srv, pid, "Treeless Agent"))
	status, body = doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": id, "kind": "agent"})
	if status != http.StatusConflict {
		t.Fatalf("agent on worktree-less task: status = %d, want 409; body=%v", status, body)
	}
	if body["error"] != "task has no worktree" {
		t.Errorf("error = %q, want %q", body["error"], "task has no worktree")
	}

	// Unknown kind value -> 400.
	status, body = doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"kind": "bogus"})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid kind: status = %d, want 400; body=%v", status, body)
	}
	if body["error"] != "invalid kind" {
		t.Errorf("error = %q, want %q", body["error"], "invalid kind")
	}

	// Explicit kind=bash stays the Phase 3 task-scoped bash path.
	tid, _ := worktreeTask(t, srv, "Explicit Bash")
	status, body = doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "bash"})
	if status != http.StatusCreated {
		t.Fatalf("explicit bash: status = %d, want 201; body=%v", status, body)
	}
	if body["label"] != "Bash 1" {
		t.Errorf("explicit bash label = %q, want %q", body["label"], "Bash 1")
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
