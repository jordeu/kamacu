package api

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"kamacu/internal/session"
	"kamacu/internal/settings"
	"kamacu/internal/store"
	"kamacu/internal/tmux"
	"kamacu/internal/worktree"
)

// This file is the CI-testable core of M002/S03/T02 — the slice's namesake
// deliverable. It locks the exact fresh-vs-resume opencode spawn argv behind a
// committed fake-opencode stub (mirroring testdata/fake-claude) and proves the
// engine-gated resumable flag surfaces a resumable:true entry for an EXITED
// opencode task (previously NEVER, because transcriptExists is a claude-only
// ~/.claude/projects jsonl glob). No real opencode is invoked in CI: the
// opencode-engine agent row points its command field at testdata/fake-opencode,
// and spawn is driven through the custom/opencode AgentArgs arm exactly as
// production does.

// testdataFakeOpencode returns the absolute path to the committed fake-opencode
// stub and re-applies the executable bit defensively (git permission bits can
// drop on some checkouts). Mirrors testdataFakeClaude.
func testdataFakeOpencode(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", "fake-opencode"))
	if err != nil {
		t.Fatalf("resolve testdata/fake-opencode: %v", err)
	}
	if err := os.Chmod(abs, 0o755); err != nil {
		t.Fatalf("chmod testdata/fake-opencode: %v", err)
	}
	return abs
}

// newOpencodeResumeServer wires the route surface over a fresh DB seeded with
// an opencode-engine agent whose command points at the committed fake-opencode
// stub. Modeled on newResumeServer: SessionRoutes + AgentRoutes are constructed
// in-package with the injected glob root so the resumable derivation is testable
// without touching the real ~/.claude/projects. Returns the server, manager, db,
// the seeded opencode agent's ID, and the FAKE_OPENCODE_ARGS_FILE path.
func newOpencodeResumeServer(t *testing.T) (*httptest.Server, *session.Manager, *sql.DB, int64, string) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
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
	mgr.SetAgentConfig(session.AgentConfig{
		BaseURL:   "http://127.0.0.1:7333",
		Token:     testHookToken,
		ClaudeBin: testdataFakeClaude(t), // unused by the opencode arm; set for parity
	})
	globRoot := t.TempDir()
	argsFile := filepath.Join(t.TempDir(), "args")
	// The opencode stub inherits the test process env (D-52) — point its argv
	// recorder at our file. t.Setenv restores it on cleanup.
	t.Setenv("FAKE_OPENCODE_ARGS_FILE", argsFile)

	// Seed an opencode-engine agent row pointing at the committed fake-opencode
	// stub. Spawn resolves engine+command from the agents row via the
	// projects→agents JOIN, so the task's project must reference this agent.
	var agentID int64
	if err := db.QueryRow(
		`INSERT INTO agents (name, command, engine, is_default, is_system) VALUES ('opencode-test', ?, 'opencode', 0, 0) RETURNING id`,
		testdataFakeOpencode(t),
	).Scan(&agentID); err != nil {
		t.Fatalf("seed opencode agent row: %v", err)
	}

	mux := http.NewServeMux()
	Routes(mux, db, wt, mgr, tmux.Client{}) // includes SettingsRoutes — spawn reads shell
	// Session routes with the injected glob root (resume validation is
	// engine-branched; the opencode path skips transcriptExists, so the glob
	// root is irrelevant for opencode, but the claude parity path needs it).
	s := &sessionHandlers{mgr: mgr, db: db, globRoot: globRoot}
	mux.HandleFunc("GET /api/sessions", s.list)
	mux.HandleFunc("POST /api/sessions", s.create)
	mux.HandleFunc("POST /api/sessions/{id}/stop", s.stop)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.delete)
	// Agent status route with the injected glob root (resumable derivation +
	// DB-derived post-restart entries).
	ah := &agentHandlers{mgr: mgr, db: db, globRoot: globRoot}
	mux.HandleFunc("GET /api/agents/status", ah.status)

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
	return srv, mgr, db, agentID, argsFile
}

// assignProjectToAgent re-points a project at the given agent id. createProject
// assigns the default (claude) agent; the opencode spawn arm is reached only
// when the project's agent row carries engine='opencode'.
func assignProjectToAgent(t *testing.T, db *sql.DB, projectID, agentID int64) {
	t.Helper()
	if _, err := db.Exec(`UPDATE projects SET agent_id = ? WHERE id = ?`, agentID, projectID); err != nil {
		t.Fatalf("assign project %d to agent %d: %v", projectID, agentID, err)
	}
}

// setTaskOpencodeSession stamps tasks.opencode_session_id for a task — the
// T01 column that is the restart-resume key (the opencode analog of
// claude_session_id). Mirrors setTaskClaudeSession.
func setTaskOpencodeSession(t *testing.T, db *sql.DB, taskID int64, ocsid string) {
	t.Helper()
	if _, err := db.Exec(`UPDATE tasks SET opencode_session_id = ? WHERE id = ?`, ocsid, taskID); err != nil {
		t.Fatalf("set opencode_session_id: %v", err)
	}
}

// stopAndWaitExited stops an agent session and polls /api/agents/status until
// the task's entry reports "exited", so the next spawn isn't blocked by the
// one-agent-per-task gate (D-38: an EXITED agent never blocks; a RUNNING one
// does).
func stopAndWaitExited(t *testing.T, srv *httptest.Server, taskID int64, sessionID string) {
	t.Helper()
	if sessionID == "" {
		t.Fatalf("stopAndWaitExited: empty sessionID for task %d", taskID)
	}
	status, body := doJSON(t, "POST", srv.URL+"/api/sessions/"+sessionID+"/stop", nil)
	if status != http.StatusAccepted {
		t.Fatalf("stop session %s = %d, want 202; body=%v", sessionID, status, body)
	}
	waitAgentStatus(t, srv, taskID, "exited", 7*time.Second)
}

// TestOpencodeFreshSpawnArgvHasNoResumeFlag (assertion a): a fresh opencode
// spawn (resume:false) records an argv with NO "-s" token — exactly the bare
// [fake-opencode] command, never accidentally carrying a stale resume flag.
// This is the regression guard that the fresh spawn path can never drift into
// emitting -s.
func TestOpencodeFreshSpawnArgvHasNoResumeFlag(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, _, db, agentID, argsFile := newOpencodeResumeServer(t)

	tid, _ := opencodeWorktreeTask(t, srv, db, agentID, "Fresh Opencode")

	// Clear any stale argv so readArgv blocks until THIS spawn writes its own.
	if err := os.Remove(argsFile); err != nil && !os.IsNotExist(err) {
		t.Fatalf("clear argv file: %v", err)
	}
	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "agent"})
	if status != http.StatusCreated {
		t.Fatalf("fresh opencode spawn = %d, want 201; body=%v", status, body)
	}
	args := readArgv(t, argsFile)
	for _, a := range args {
		if a == "-s" {
			t.Errorf("fresh opencode argv leaked a -s flag: %v (must be the bare [fake-opencode] command)", args)
		}
	}
	// Stop so the one-per-task gate stays clear for sibling subtests.
	stopAndWaitExited(t, srv, tid, strID(body))
}

// TestOpencodeResumeSpawnAppendsSessionFlag (assertion b): an opencode task
// with a persisted opencode_session_id spawns `opencode -s <id>` on Resume —
// NOT claude's --resume, NOT a fresh opencode. The recorded argv is exactly
// [fake-opencode, "-s", "<stored id>"]. This locks the resume argv so it can
// never drift.
func TestOpencodeResumeSpawnAppendsSessionFlag(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, _, db, agentID, argsFile := newOpencodeResumeServer(t)

	tid, _ := opencodeWorktreeTask(t, srv, db, agentID, "Resume Opencode")
	const stored = "ses_test123"
	setTaskOpencodeSession(t, db, tid, stored)

	if err := os.Remove(argsFile); err != nil && !os.IsNotExist(err) {
		t.Fatalf("clear argv file: %v", err)
	}
	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "agent", "resume": true})
	if status != http.StatusCreated {
		t.Fatalf("resume opencode spawn = %d, want 201; body=%v", status, body)
	}
	args := readArgv(t, argsFile)
	want := []string{"-s", stored}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("resume opencode argv = %v, want exactly %v (the opencode -s <id> resume flag, NOT claude --resume and NOT a fresh spawn)", args, want)
	}
	stopAndWaitExited(t, srv, tid, strID(body))
}

// TestOpencodeExitedTaskIsResumable (assertion c): with the agent session
// EXITED + a stored opencode_session_id + worktree present, GET
// /api/agents/status reports resumable:true for the opencode task. This is the
// engine-gate proof: previously an exited opencode task was NEVER resumable
// because the resumable derivation gated on transcriptExists (a claude-only
// ~/.claude/projects jsonl glob). The opencode path now keys off the persisted
// opencode_session_id alone.
func TestOpencodeExitedTaskIsResumable(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, _, db, agentID, argsFile := newOpencodeResumeServer(t)

	tid, _ := opencodeWorktreeTask(t, srv, db, agentID, "Resumable Opencode")
	const stored = "ses_test456"
	setTaskOpencodeSession(t, db, tid, stored)

	// Spawn the opencode agent (fresh), then stop it so it transitions to
	// exited while staying tracked in the manager — the state a card's Resume
	// button appears for.
	if err := os.Remove(argsFile); err != nil && !os.IsNotExist(err) {
		t.Fatalf("clear argv file: %v", err)
	}
	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "agent"})
	if status != http.StatusCreated {
		t.Fatalf("opencode spawn = %d, want 201; body=%v", status, body)
	}
	stopAndWaitExited(t, srv, tid, strID(body))

	entry := waitAgentStatus(t, srv, tid, "exited", 7*time.Second)
	if entry["status"] != "exited" {
		t.Fatalf("status = %v, want \"exited\"", entry["status"])
	}
	if entry["resumable"] != true {
		t.Errorf("resumable = %v, want true (an exited opencode task with a persisted opencode_session_id + worktree must be resumable — the engine-gate that previously gated on the claude-only transcriptExists glob is broken)", entry["resumable"])
	}
}

// TestOpencodeResumeWithoutStoredIdIs409 (negative test): a Resume request on
// an opencode task with a NULL opencode_session_id is an honest 409 "no session
// to resume" — the engine-gated resume validation refuses to silently fork a
// fresh session. Mirrors claude's NULL-csid posture.
func TestOpencodeResumeWithoutStoredIdIs409(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, _, db, agentID, _ := newOpencodeResumeServer(t)

	tid, _ := opencodeWorktreeTask(t, srv, db, agentID, "No Id Opencode")
	// opencode_session_id left NULL.

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "agent", "resume": true})
	if status != http.StatusConflict {
		t.Fatalf("resume NULL-opencode-id task = %d, want 409; body=%v", status, body)
	}
	if body["error"] != "no session to resume" {
		t.Errorf("error = %q, want %q", body["error"], "no session to resume")
	}
}

// opencodeWorktreeTask creates a project (initially on the default claude
// agent), reassigns it to the opencode agent, and provisions a task with a
// worktree. Returns (taskID, worktreePath). The repo lives on the test's t so
// provisioning succeeds.
func opencodeWorktreeTask(t *testing.T, srv *httptest.Server, db *sql.DB, agentID int64, title string) (int64, string) {
	t.Helper()
	pid := createProject(t, srv, gitRepoWithCommit(t))
	assignProjectToAgent(t, db, pid, agentID)
	body := createTask(t, srv, pid, title)
	id := taskID(t, body)
	wtPath, _ := body["worktree_path"].(string)
	if wtPath == "" {
		t.Fatalf("opencode task provisioning failed: %v", body)
	}
	return id, wtPath
}

// strID extracts the "id" string from a JSON object reply.
func strID(body map[string]any) string {
	id, _ := body["id"].(string)
	return id
}
