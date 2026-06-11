package api

import (
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

	"github.com/google/uuid"

	"kangent/internal/session"
	"kangent/internal/store"
	"kangent/internal/worktree"
)

// recoveryHarness bundles everything a "process" needs to serve the recovery
// surface over the SHARED durable state (the SQLite file, the worktree.Service,
// and the injected transcript globRoot). A second harness over the SAME db /
// worktree service / globRoot — but a brand-new session.Manager — IS the
// simulated server restart: the manager (and its PTYs) is the only thing a
// restart actually loses, and RCVR-01 reconciliation is by architecture (the DB
// never records "running", so a fresh manager + DB-derived /api/agents/status
// entries is the whole story — no migration, no startup mutation pass).
type recoveryHarness struct {
	srv *httptest.Server
	mgr *session.Manager
}

// newRecoveryProcess wires one "process": the full production route surface
// (Routes + Session + Worktree + Hook + Agent + Diff + WS) over the supplied
// shared db / worktree.Service / globRoot, with a FRESH manager pointed at the
// committed fake-claude stub. Session/Agent handlers are constructed in-package
// so the test-only globRoot injection rides alongside the real route wiring
// (the production SessionRoutes/AgentRoutes signatures are unchanged).
// The caller passes the manager so the two distinct session.NewManager()
// instances (before vs after the restart) are visible at the call sites — a
// restart's ONLY real loss is the manager and its live PTYs.
func newRecoveryProcess(t *testing.T, mgr *session.Manager, db *sql.DB, wt *worktree.Service, globRoot string) recoveryHarness {
	t.Helper()
	mux := http.NewServeMux()
	Routes(mux, db, wt, mgr)

	// Session routes with the injected glob root (resume validation).
	sh := &sessionHandlers{mgr: mgr, db: db, globRoot: globRoot}
	mux.HandleFunc("GET /api/sessions", sh.list)
	mux.HandleFunc("POST /api/sessions", sh.create)
	mux.HandleFunc("POST /api/sessions/{id}/stop", sh.stop)
	mux.HandleFunc("DELETE /api/sessions/{id}", sh.delete)

	WorktreeRoutes(mux, db, wt, mgr)
	HookRoutes(mux, mgr, agentLifecycleToken)

	// Agent status route with the injected glob root (resumable derivation +
	// DB-derived post-restart entries).
	ah := &agentHandlers{mgr: mgr, db: db, globRoot: globRoot}
	mux.HandleFunc("GET /api/agents/status", ah.status)

	// The wired diff path (ADD DiffRoutes vs the Phase 4 harness).
	DiffRoutes(mux, db, wt)

	srv := httptest.NewServer(mux)
	mgr.SetAgentConfig(session.AgentConfig{
		BaseURL:   srv.URL,
		Token:     agentLifecycleToken,
		ClaudeBin: testdataFakeClaude(t),
	})
	t.Cleanup(func() {
		srv.Close()
		// Stop every session this manager owns so no fake-claude outlives the
		// test (a stray sleeper would hold a tmp dir / pollute later runs).
		for _, info := range mgr.List() {
			if s, ok := mgr.Get(info.ID); ok {
				s.Stop()
			}
		}
	})
	return recoveryHarness{srv: srv, mgr: mgr}
}

// recoveryStatusForTask returns the /api/agents/status entry for taskID on the
// given harness, or nil when none is listed.
func recoveryStatusForTask(t *testing.T, h recoveryHarness, taskID int64) map[string]any {
	t.Helper()
	return agentStatusForTask(t, h.srv, taskID)
}

// TestRecoveryLifecycle proves Phase 5 end to end over the real REST surface
// against the committed fake-claude stub (the real binary is NEVER spawned —
// auth + API cost): restart reconciliation by architecture (RCVR-01, D-58,
// D-65), the full resume lifecycle (success argv + same-id persistence, the
// D-67 one-per-task gate on resume, honest exit-1 failure D-56, and the
// unresumable 409s), and the wired diff path (REVW-01). One sequential
// lifecycle test under a wall budget < 30s.
func TestRecoveryLifecycle(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	// Shared durable state: the SQLite file + worktree service + transcript
	// glob root all survive the "restart"; only the manager is replaced.
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}
	wtDir := t.TempDir()
	seedWorktreeBase(t, db, wtDir)
	wt := worktree.NewService(wtDir)
	globRoot := t.TempDir()

	// Agents inherit the test process env (D-52); point the stub's argv
	// recorder at a temp file. t.Setenv restores it on cleanup.
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("FAKE_CLAUDE_ARGS_FILE", argsFile)

	var (
		pid    int64
		tid    int64
		uuidA  string
		wtPath string
	)

	// The repo must live on the OUTER t: a subtest's t.TempDir is removed when
	// that subtest ends, which would delete the repo (and its .git/worktrees
	// bookkeeping) out from under later stages that provision fresh worktrees.
	repo := gitRepoWithCommit(t)

	// proc1/proc2 are the "before" and "after restart" processes over the
	// shared db. Both are built on the OUTER t so their httptest servers (and
	// their managers' cleanup) live for the WHOLE lifecycle — a subtest's
	// t.Cleanup would close the server when that stage returns, before later
	// stages use it. proc1's manager is the one a restart abandons; its PTYs
	// are reaped in cleanup.
	mgr1 := session.NewManager()
	proc1 := newRecoveryProcess(t, mgr1, db, wt, globRoot)

	// proc2 is the post-restart process: a SECOND, fresh session.NewManager()
	// + mux over the SAME db / worktree / globRoot. Constructing it up front
	// (empty manager) is harmless — it observes the shared DB, and the restart
	// "happens" the moment stage 2 starts querying it after proc1 seeded
	// running sessions.
	mgr2 := session.NewManager()
	proc2 := newRecoveryProcess(t, mgr2, db, wt, globRoot)

	stage := func(name string, fn func(t *testing.T)) {
		t.Helper()
		if !t.Run(name, fn) {
			t.FailNow()
		}
	}

	stage("1_seed_running_agent_and_bash", func(t *testing.T) {
		pid = createProject(t, proc1.srv, repo)
		body := createTask(t, proc1.srv, pid, "Recover Me")
		tid = taskID(t, body)
		wtPath, _ = body["worktree_path"].(string)
		if wtPath == "" {
			t.Fatalf("task has no worktree_path: %v", body)
		}

		// Spawn the agent (fake-claude) — left RUNNING.
		status, sess := doJSON(t, "POST", proc1.srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "agent"})
		if status != http.StatusCreated {
			t.Fatalf("agent spawn = %d, want 201; body=%v", status, sess)
		}
		// One bash session — also left RUNNING (D-58: it must vanish on restart,
		// no stub left behind).
		status, bash := doJSON(t, "POST", proc1.srv.URL+"/api/sessions", map[string]any{"task_id": tid})
		if status != http.StatusCreated {
			t.Fatalf("bash spawn = %d, want 201; body=%v", status, bash)
		}

		// The persisted --resume key (uuid A).
		uuidA = taskClaudeSessionID(t, db, tid)
		if _, err := uuid.Parse(uuidA); err != nil {
			t.Fatalf("claude_session_id %q is not a uuid: %v", uuidA, err)
		}

		// Both are running in proc1 right now (the "live before crash" state).
		status, list := doJSONList(t, fmt.Sprintf("%s/api/sessions?task_id=%d", proc1.srv.URL, tid))
		if status != http.StatusOK || len(list) != 2 {
			t.Fatalf("pre-restart sessions = %v (status %d), want 2 running", list, status)
		}
	})

	stage("2_restart_reconciles_silently", func(t *testing.T) {
		// THE RESTART: proc2 is a brand-new manager + mux over the SAME db /
		// worktree / globRoot, built up front. proc1's manager is simply
		// abandoned (cleanup stops its PTYs). From here on we query proc2 only.

		// No ghost sessions, no bash stubs — the new manager is empty and the DB
		// has no row that can claim "running" (D-58 / RCVR-01 by architecture).
		status, list := doJSONList(t, fmt.Sprintf("%s/api/sessions?task_id=%d", proc2.srv.URL, tid))
		if status != http.StatusOK {
			t.Fatalf("post-restart sessions list = %d, want 200", status)
		}
		if len(list) != 0 {
			t.Fatalf("post-restart sessions = %v, want [] (no ghosts, no bash stubs)", list)
		}

		// No transcript fixture yet → a never-prompted session is NOT resumable
		// (Pitfall 4); the status endpoint lists nothing for the task.
		if got := recoveryStatusForTask(t, proc2, tid); got != nil {
			t.Fatalf("status (no transcript) = %v, want no entry (never-prompted is not resumable)", got)
		}

		// Create the transcript fixture for uuid A → exactly one DB-derived
		// entry: sessionId "", status "exited", exitCode null, stopRequested
		// false, resumable true (RCVR-01 reconciliation carrier).
		seedTranscript(t, globRoot, uuidA)
		entry := recoveryStatusForTask(t, proc2, tid)
		if entry == nil {
			t.Fatalf("status (with transcript) has no entry for task %d", tid)
		}
		if entry["sessionId"] != "" {
			t.Errorf("sessionId = %v, want \"\" (DB-derived, no live session)", entry["sessionId"])
		}
		if entry["status"] != "exited" {
			t.Errorf("status = %v, want %q", entry["status"], "exited")
		}
		if entry["exitCode"] != nil {
			t.Errorf("exitCode = %v, want null (no real exit observed)", entry["exitCode"])
		}
		if entry["stopRequested"] != false {
			t.Errorf("stopRequested = %v, want false", entry["stopRequested"])
		}
		if entry["resumable"] != true {
			t.Errorf("resumable = %v, want true (transcript exists)", entry["resumable"])
		}

		// Poll status twice more; sessions stays [] — NOTHING auto-resumes
		// (D-67 / anti-pattern: a session is only ever spawned by an explicit
		// POST). The status reads themselves must never spawn a PTY.
		for i := 0; i < 2; i++ {
			_ = recoveryStatusForTask(t, proc2, tid)
		}
		status, list = doJSONList(t, fmt.Sprintf("%s/api/sessions?task_id=%d", proc2.srv.URL, tid))
		if status != http.StatusOK || len(list) != 0 {
			t.Fatalf("sessions after status polls = %v (status %d), want still [] (no auto-resume)", list, status)
		}
	})

	stage("3_resume_uses_persisted_uuid", func(t *testing.T) {
		// The stub OVERWRITES the argv file, but stage 1's fresh agent spawn
		// already wrote it (--session-id). Clear it so readArgv blocks until the
		// resume process writes its own argv, not the stale fresh-spawn one.
		if err := os.Remove(argsFile); err != nil && !os.IsNotExist(err) {
			t.Fatalf("clear argv file: %v", err)
		}
		status, sess := doJSON(t, "POST", proc2.srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "agent", "resume": true})
		if status != http.StatusCreated {
			t.Fatalf("resume = %d, want 201; body=%v", status, sess)
		}

		// argv: line 1 == --resume, line 2 == uuid A, --settings present.
		args := readArgv(t, argsFile)
		if len(args) < 2 || args[0] != "--resume" {
			t.Fatalf("resume argv[0] = %v, want --resume; argv=%v", args, args)
		}
		if args[1] != uuidA {
			t.Errorf("resume argv[1] = %q, want the persisted uuid %q", args[1], uuidA)
		}
		hasSettings := false
		for _, a := range args {
			if a == "--settings" {
				hasSettings = true
			}
		}
		if !hasSettings {
			t.Errorf("resume argv missing --settings (hook overlay must apply on resume); argv=%v", args)
		}

		// Same-id persistence: the post-spawn UPDATE rewrote the SAME value.
		after := taskClaudeSessionID(t, db, tid)
		if after != uuidA {
			t.Errorf("claude_session_id = %q after resume, want unchanged %q", after, uuidA)
		}
	})

	stage("4_one_per_task_gate_on_resume", func(t *testing.T) {
		// A second resume while the resumed agent runs → 409 (D-67); the gate
		// fires BEFORE resume validation.
		status, body := doJSON(t, "POST", proc2.srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "agent", "resume": true})
		if status != http.StatusConflict {
			t.Fatalf("second resume = %d, want 409; body=%v", status, body)
		}
		if msg, _ := body["error"].(string); !strings.Contains(msg, "agent session already running") {
			t.Errorf("409 body = %q, want it to contain %q", msg, "agent session already running")
		}
	})

	stage("5_honest_failure_and_unresumable", func(t *testing.T) {
		// Stop the running resumed agent and wait for it to leave the slot.
		entry := recoveryStatusForTask(t, proc2, tid)
		if entry == nil {
			t.Fatalf("expected a running agent entry before stop")
		}
		sid, _ := entry["sessionId"].(string)
		if sid == "" {
			t.Fatalf("running agent has empty sessionId: %v", entry)
		}
		status, body := doJSON(t, "POST", proc2.srv.URL+"/api/sessions/"+sid+"/stop", nil)
		if status != http.StatusAccepted {
			t.Fatalf("stop = %d, want 202; body=%v", status, body)
		}
		waitAgentStatus(t, proc2.srv, tid, "exited", 7*time.Second)

		// Delete the transcript fixture → resume now 409 "no session to resume"
		// (server-side fresh validation against a stale client snapshot).
		if err := os.Remove(filepath.Join(globRoot, "-home-x-wt", uuidA+".jsonl")); err != nil {
			t.Fatalf("remove transcript fixture: %v", err)
		}
		status, body = doJSON(t, "POST", proc2.srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "agent", "resume": true})
		if status != http.StatusConflict {
			t.Fatalf("resume (no transcript) = %d, want 409; body=%v", status, body)
		}
		if body["error"] != "no session to resume" {
			t.Errorf("error = %q, want %q", body["error"], "no session to resume")
		}

		// Recreate the fixture + force the v2.1.173 missing-transcript failure:
		// resume 201s but the spawned process exits code 1 (honest failure,
		// D-56). The status entry shows the red-path exit, and resumable stays
		// true because the transcript still exists (only deleting it flips
		// resumable false).
		seedTranscript(t, globRoot, uuidA)
		t.Setenv("FAKE_CLAUDE_RESUME_FAIL", "1")
		status, sess := doJSON(t, "POST", proc2.srv.URL+"/api/sessions", map[string]any{"task_id": tid, "kind": "agent", "resume": true})
		if status != http.StatusCreated {
			t.Fatalf("resume (fail mode) = %d, want 201 (the spawn itself succeeds); body=%v", status, sess)
		}
		failed := waitAgentStatus(t, proc2.srv, tid, "exited", 5*time.Second)
		if failed["exitCode"] != float64(1) {
			t.Errorf("exitCode = %v, want 1 (honest --resume failure)", failed["exitCode"])
		}
		if failed["resumable"] != true {
			t.Errorf("resumable = %v, want true (a crashed-but-resumable session keeps its transcript)", failed["resumable"])
		}
		os.Unsetenv("FAKE_CLAUDE_RESUME_FAIL")
	})

	stage("6_no_session_to_resume_for_null_task", func(t *testing.T) {
		// A SECOND task WITH a worktree but a NULL claude_session_id → the
		// resume guard (after the worktree gate) returns "no session to resume".
		body := createTask(t, proc2.srv, pid, "Never Started")
		tid2 := taskID(t, body)
		if wtp, _ := body["worktree_path"].(string); wtp == "" {
			t.Fatalf("Never Started task has no worktree (worktree_error=%v) — needed to reach the resume guard", body["worktree_error"])
		}
		status, rbody := doJSON(t, "POST", proc2.srv.URL+"/api/sessions", map[string]any{"task_id": tid2, "kind": "agent", "resume": true})
		if status != http.StatusConflict {
			t.Fatalf("resume NULL-id task = %d, want 409; body=%v", status, rbody)
		}
		if rbody["error"] != "no session to resume" {
			t.Errorf("error = %q, want %q", rbody["error"], "no session to resume")
		}
	})

	stage("7_diff_endpoint_wired", func(t *testing.T) {
		// In the first task's worktree: one modified tracked file + one
		// untracked file → totals.files == 2, untracked entry status "new".
		if err := os.WriteFile(filepath.Join(wtPath, "file.txt"), []byte("hello\nmodified\n"), 0o644); err != nil {
			t.Fatalf("modify tracked file: %v", err)
		}
		if err := os.WriteFile(filepath.Join(wtPath, "newfile.txt"), []byte("brand new\n"), 0o644); err != nil {
			t.Fatalf("write untracked file: %v", err)
		}
		status, d := doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d/diff", proc2.srv.URL, tid), nil)
		if status != http.StatusOK {
			t.Fatalf("diff = %d, want 200; body=%v", status, d)
		}
		totals, _ := d["totals"].(map[string]any)
		if totals == nil || totals["files"] != float64(2) {
			t.Fatalf("totals.files = %v, want 2; totals=%v", totalsFiles(totals), totals)
		}
		files, _ := d["files"].([]any)
		var newEntry map[string]any
		for _, f := range files {
			fm, _ := f.(map[string]any)
			if fm["path"] == "newfile.txt" {
				newEntry = fm
			}
		}
		if newEntry == nil {
			t.Fatalf("untracked file newfile.txt missing from diff files: %v", files)
		}
		if newEntry["status"] != "new" {
			t.Errorf("newfile.txt status = %v, want %q", newEntry["status"], "new")
		}

		// A task with NO worktree → 409 "task has no worktree". Create one and
		// remove its worktree (force) to null worktree_path server-side.
		body := createTask(t, proc2.srv, pid, "No Worktree")
		tid3 := taskID(t, body)
		status, rm := doJSON(t, "DELETE", fmt.Sprintf("%s/api/tasks/%d/worktree", proc2.srv.URL, tid3), map[string]any{"force": true})
		if status != http.StatusNoContent {
			t.Fatalf("remove worktree = %d, want 204; body=%v", status, rm)
		}
		status, body = doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d/diff", proc2.srv.URL, tid3), nil)
		if status != http.StatusConflict {
			t.Fatalf("diff (no worktree) = %d, want 409; body=%v", status, body)
		}
		if body["error"] != "task has no worktree" {
			t.Errorf("error = %q, want %q", body["error"], "task has no worktree")
		}
	})
}

// totalsFiles safely extracts totals.files for an error message.
func totalsFiles(totals map[string]any) any {
	if totals == nil {
		return nil
	}
	return totals["files"]
}
