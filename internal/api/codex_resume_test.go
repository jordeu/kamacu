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

func TestCodexSessionIDFromFooter(t *testing.T) {
	const id = "0195abcd-1234-5678-9abc-123456789abc"
	for _, tc := range []struct {
		name, output, want string
	}{
		{"plain", "To continue this session, run codex resume " + id + "\r\n", id},
		{"color and wrap", "output\r\nTo continue this session, run\r\n\x1b[32mcodex resume " + id + "\x1b[0m\r\n", id},
		{"ordinary command", "codex resume " + id, ""},
		{"not final", "To continue this session, run codex resume " + id + "\nmore output", ""},
		{"invalid id", "To continue this session, run codex resume invalid", ""},
		{"empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := codexSessionIDFromFooter([]byte(tc.output)); got != tc.want {
				t.Fatalf("captured %q, want %q", got, tc.want)
			}
		})
	}
}

// No hooks fire: the CLI prints its footer and exits immediately. This also
// exercises the race between process exit, terminal draining, and persistence.
func TestCodexExitWithoutHooksCanResume(t *testing.T) {
	for _, global := range []bool{false, true} {
		name := "task"
		if global {
			name = "global"
		}
		t.Run(name, func(t *testing.T) {
			srv, mgr, db, agentID, argsFile := newCodexResumeServer(t)
			const id = "0195abcd-1234-5678-9abc-123456789abc"
			t.Setenv("FAKE_CODEX_EXIT_ID", id)
			req := map[string]any{"kind": "agent"}
			var tid int64
			query := "SELECT codex_session_id FROM global_task WHERE id = 1"
			var queryArgs []any
			if global {
				if _, err := db.Exec("UPDATE global_task SET root_path = ?, agent_id = ? WHERE id = 1", t.TempDir(), agentID); err != nil {
					t.Fatal(err)
				}
				req["scope"] = "global"
			} else {
				tid, _ = codexWorktreeTask(t, srv, db, agentID, "No hooks")
				req["task_id"] = tid
				query = "SELECT codex_session_id FROM tasks WHERE id = ?"
				queryArgs = []any{tid}
			}
			status, body := doJSON(t, "POST", srv.URL+"/api/sessions", req)
			if status != http.StatusCreated {
				t.Fatalf("spawn = %d: %v", status, body)
			}
			sess, ok := mgr.Get(strID(body))
			if !ok {
				t.Fatal("spawned session missing")
			}
			select {
			case <-sess.Done():
			case <-time.After(5 * time.Second):
				t.Fatal("session did not exit")
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				var stored sql.NullString
				if err := db.QueryRow(query, queryArgs...).Scan(&stored); err != nil {
					t.Fatal(err)
				}
				if stored.String == id {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("footer ID not persisted: %q; output: %q", stored.String, sess.Snapshot())
				}
				time.Sleep(10 * time.Millisecond)
			}
			entry := waitAgentStatus(t, srv, tid, "exited", 5*time.Second)
			if entry["resumable"] != true {
				t.Fatalf("exited session is not resumable: %v", entry)
			}
			t.Setenv("FAKE_CODEX_EXIT_ID", "")
			if err := os.Remove(argsFile); err != nil {
				t.Fatal(err)
			}
			req["resume"] = true
			status, body = doJSON(t, "POST", srv.URL+"/api/sessions", req)
			if status != http.StatusCreated {
				t.Fatalf("resume = %d: %v", status, body)
			}
			args := readArgv(t, argsFile)
			if !reflect.DeepEqual(args, []string{"-p", "kamacu", "resume", id}) {
				t.Fatalf("resume argv = %v", args)
			}
		})
	}
}

// This file locks the codex-engine fresh-vs-resume spawn argv, the
// engine-gated resumable derivation, the honest no-id 409, and the
// hook-driven codex session-id capture — the CI-testable core of codex
// restart-resume, mirroring opencode_resume_test.go. No real codex is invoked
// in CI: the codex-engine agent row points its command field at
// testdata/fake-codex, and spawn is driven through the custom/codex AgentArgs
// arm (with the session layer's engine-gated `-p kamacu` prepend) exactly as
// production does.

// testdataFakeCodex returns the absolute path to the committed fake-codex
// stub and re-applies the executable bit defensively. Mirrors
// testdataFakeOpencode.
func testdataFakeCodex(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", "fake-codex"))
	if err != nil {
		t.Fatalf("resolve testdata/fake-codex: %v", err)
	}
	if err := os.Chmod(abs, 0o755); err != nil {
		t.Fatalf("chmod testdata/fake-codex: %v", err)
	}
	return abs
}

// newCodexResumeServer wires the route surface over a fresh DB seeded with a
// codex-engine agent whose command points at the committed fake-codex stub.
// Modeed on newOpencodeResumeServer; the hook receiver is ALSO registered so
// the capture test can drive the exact production loop (SessionStart payload
// carrying codex's own session uuid). Returns the server, manager, db, the
// seeded codex agent's ID, and the FAKE_CODEX_ARGS_FILE path.
func newCodexResumeServer(t *testing.T) (*httptest.Server, *session.Manager, *sql.DB, int64, string) {
	t.Helper()
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
		ClaudeBin: testdataFakeClaude(t), // unused by the codex arm; set for parity
	})
	globRoot := t.TempDir()
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("FAKE_CODEX_ARGS_FILE", argsFile)

	var agentID int64
	if err := db.QueryRow(
		`INSERT INTO agents (name, command, engine, is_default, is_system) VALUES ('codex-test', ?, 'codex', 0, 0) RETURNING id`,
		testdataFakeCodex(t),
	).Scan(&agentID); err != nil {
		t.Fatalf("seed codex agent row: %v", err)
	}

	mux := http.NewServeMux()
	Routes(mux, db, wt, mgr, tmux.Client{})
	s := &sessionHandlers{mgr: mgr, db: db, globRoot: globRoot}
	mux.HandleFunc("GET /api/sessions", s.list)
	mux.HandleFunc("POST /api/sessions", s.create)
	mux.HandleFunc("POST /api/sessions/{id}/stop", s.stop)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.delete)
	ah := &agentHandlers{mgr: mgr, db: db, globRoot: globRoot}
	mux.HandleFunc("GET /api/agents/status", ah.status)
	HookRoutes(mux, mgr, testHookToken)

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

// setTaskCodexSession stamps tasks.codex_session_id — the codex restart-resume
// key. Mirrors setTaskOpencodeSession.
func setTaskCodexSession(t *testing.T, db *sql.DB, taskID int64, cxid string) {
	t.Helper()
	if _, err := db.Exec(`UPDATE tasks SET codex_session_id = ? WHERE id = ?`, cxid, taskID); err != nil {
		t.Fatalf("set codex_session_id: %v", err)
	}
}

// codexWorktreeTask creates a project (initially on the default claude
// agent), reassigns it to the codex agent, and provisions a task with a
// worktree. Mirrors opencodeWorktreeTask.
func codexWorktreeTask(t *testing.T, srv *httptest.Server, db *sql.DB, agentID int64, title string) (int64, string) {
	t.Helper()
	pid := createProject(t, srv, gitRepoWithCommit(t))
	assignProjectToAgent(t, db, pid, agentID)
	body := createTask(t, srv, pid, title)
	id := taskID(t, body)
	wtPath, _ := body["worktree_path"].(string)
	if wtPath == "" {
		t.Fatalf("codex task provisioning failed: %v", body)
	}
	return id, wtPath
}

// TestCodexFreshSpawnArgvHasProfileFlag: a fresh codex spawn (resume:false)
// records EXACTLY ["-p", "kamacu"] — the engine-gated profile selector the
// session layer prepends, and nothing else (no resume subcommand, no -s, no
// claude flags).
func TestCodexFreshSpawnArgvHasProfileFlag(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, _, db, agentID, argsFile := newCodexResumeServer(t)

	tid, _ := codexWorktreeTask(t, srv, db, agentID, "Fresh Codex")

	if err := os.Remove(argsFile); err != nil && !os.IsNotExist(err) {
		t.Fatalf("clear argv file: %v", err)
	}
	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "agent"})
	if status != http.StatusCreated {
		t.Fatalf("fresh codex spawn = %d, want 201; body=%v", status, body)
	}
	args := readArgv(t, argsFile)
	want := []string{"-p", "kamacu"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("fresh codex argv = %v, want exactly %v (the profile selector only)", args, want)
	}
	stopAndWaitExited(t, srv, tid, strID(body))
}

// TestCodexResumeSpawnAppendsResumeSubcommand: a codex task with a persisted
// codex_session_id spawns `codex -p kamacu resume <uuid>` — NOT claude's
// --resume, NOT a fresh codex. Locks the resume argv exactly.
func TestCodexResumeSpawnAppendsResumeSubcommand(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, _, db, agentID, argsFile := newCodexResumeServer(t)

	tid, _ := codexWorktreeTask(t, srv, db, agentID, "Resume Codex")
	const stored = "0d1a2b3c-5555-6666-7777-88889999aaaa"
	setTaskCodexSession(t, db, tid, stored)

	if err := os.Remove(argsFile); err != nil && !os.IsNotExist(err) {
		t.Fatalf("clear argv file: %v", err)
	}
	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "agent", "resume": true})
	if status != http.StatusCreated {
		t.Fatalf("resume codex spawn = %d, want 201; body=%v", status, body)
	}
	args := readArgv(t, argsFile)
	want := []string{"-p", "kamacu", "resume", stored}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("resume codex argv = %v, want exactly %v (the codex resume subcommand, NOT claude --resume and NOT a fresh spawn)", args, want)
	}
	stopAndWaitExited(t, srv, tid, strID(body))
}

// TestCodexExitedTaskIsResumable: with the agent session EXITED + a stored
// codex_session_id + worktree present, GET /api/agents/status reports
// resumable:true for the codex task — keyed off the persisted id alone (codex
// has no kamacu-side transcript glob; its sessions live under ~/.codex).
func TestCodexExitedTaskIsResumable(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, _, db, agentID, argsFile := newCodexResumeServer(t)

	tid, _ := codexWorktreeTask(t, srv, db, agentID, "Resumable Codex")
	const stored = "0d1a2b3c-abcd-ef01-2345-6789abcdef01"
	setTaskCodexSession(t, db, tid, stored)

	if err := os.Remove(argsFile); err != nil && !os.IsNotExist(err) {
		t.Fatalf("clear argv file: %v", err)
	}
	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "agent"})
	if status != http.StatusCreated {
		t.Fatalf("codex spawn = %d, want 201; body=%v", status, body)
	}
	stopAndWaitExited(t, srv, tid, strID(body))

	entry := waitAgentStatus(t, srv, tid, "exited", 7*time.Second)
	if entry["resumable"] != true {
		t.Errorf("resumable = %v, want true (an exited codex task with a persisted codex_session_id + worktree must be resumable)", entry["resumable"])
	}
}

// TestCodexResumeWithoutStoredIdIs409 (negative test): a Resume request on a
// codex task with a NULL codex_session_id is an honest 409 "no session to
// resume" — refusing to silently fork a fresh session. Mirrors the opencode
// posture.
func TestCodexResumeWithoutStoredIdIs409(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, _, db, agentID, _ := newCodexResumeServer(t)

	tid, _ := codexWorktreeTask(t, srv, db, agentID, "No Id Codex")
	// codex_session_id left NULL.

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "agent", "resume": true})
	if status != http.StatusConflict {
		t.Fatalf("resume NULL-codex-id task = %d, want 409; body=%v", status, body)
	}
	if body["error"] != "no session to resume" {
		t.Errorf("error = %q, want %q", body["error"], "no session to resume")
	}
}

// TestCodexCapturePersistsSessionID proves the capture loop end-to-end at the
// API layer: a FRESH codex spawn + a SessionStart hook POST (the exact
// production payload shape, carrying codex's own uuid) results in
// tasks.codex_session_id being persisted — the restart-resume key. The
// capture poller reads the session's recorded id (set by the receiver) and
// persists it without any subprocess discovery (codex's id rides the hook
// payload, unlike opencode's `session list` scan).
func TestCodexCapturePersistsSessionID(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, _, db, agentID, argsFile := newCodexResumeServer(t)

	tid, _ := codexWorktreeTask(t, srv, db, agentID, "Capture Codex")

	if err := os.Remove(argsFile); err != nil && !os.IsNotExist(err) {
		t.Fatalf("clear argv file: %v", err)
	}
	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "agent"})
	if status != http.StatusCreated {
		t.Fatalf("codex spawn = %d, want 201; body=%v", status, body)
	}
	sessID := strID(body)

	// Drive the exact production loop: the profile's SessionStart hook POSTs
	// the codex-owned session uuid to the receiver.
	const codexID = "0d1a2b3c-capture-0001-0002-000000000003"
	hookBody := `{"hook_event_name":"SessionStart","source":"startup","session_id":"` + codexID + `"}`
	if got := postHook(t, srv.URL+"/api/hooks/sessions/"+sessID, testHookToken, hookBody); got != http.StatusNoContent {
		t.Fatalf("SessionStart hook = %d, want 204", got)
	}

	// The capture poller persists the id (500ms poll; generous window).
	deadline := time.Now().Add(10 * time.Second)
	for {
		var got sql.NullString
		if err := db.QueryRow(`SELECT codex_session_id FROM tasks WHERE id = ?`, tid).Scan(&got); err != nil {
			t.Fatalf("select codex_session_id: %v", err)
		}
		if got.Valid && got.String == codexID {
			break // captured + persisted — the restart-resume key
		}
		if time.Now().After(deadline) {
			t.Fatalf("codex_session_id not persisted within 10s (last value: %q)", got.String)
		}
		time.Sleep(100 * time.Millisecond)
	}

	stopAndWaitExited(t, srv, tid, sessID)
}
