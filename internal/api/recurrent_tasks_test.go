package api

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"kamacu/internal/session"
)

func recurrentMove(t *testing.T, srv *httptest.Server, id int64, status string) {
	t.Helper()
	code, body := doJSON(t, "POST", fmt.Sprintf("%s/api/tasks/%d/move", srv.URL, id), map[string]any{"status": status})
	if code != http.StatusOK {
		t.Fatalf("move %d to %s: %d %v", id, status, code, body)
	}
}

func setRecurrent(t *testing.T, srv *httptest.Server, id int64, enabled bool) {
	t.Helper()
	code, body := doJSON(t, "PATCH", fmt.Sprintf("%s/api/tasks/%d", srv.URL, id), map[string]any{"recurrent": enabled})
	if code != http.StatusOK || body["recurrent"] != enabled {
		t.Fatalf("toggle recurrence: %d %v", code, body)
	}
}

func taskCount(t *testing.T, db *sql.DB, pid int64, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE project_id = ?`, pid).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("task count = %d, want %d", got, want)
	}
}

func TestRecurrentTaskLifecycle(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	original := createTask(t, srv, pid, "Routine")
	id := taskID(t, original)
	if original["recurrent"] != false {
		t.Fatal("new tasks must not repeat by default")
	}
	setRecurrent(t, srv, id, true)
	if _, err := db.Exec(`UPDATE tasks SET title = 'Latest title', description = 'Latest description',
 claude_session_id = 'claude-conversation', opencode_session_id = 'opencode-conversation', codex_session_id = 'codex-conversation',
 issue_repo = 'owner/repo', issue_number = 123, issue_url = 'https://example.com/123', in_progress_at = 'old' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	recurrentMove(t, srv, id, "done")
	taskCount(t, db, pid, 2)
	next, err := scanTask(db.QueryRow(`SELECT `+taskColumns+` FROM tasks WHERE project_id = ? AND id != ?`, pid, id))
	if err != nil {
		t.Fatal(err)
	}
	if next.Title != "Latest title" || next.Description != "Latest description" || next.Status != "todo" || !next.Recurrent {
		t.Fatalf("unexpected successor: %+v", next)
	}
	if next.WorktreePath == nil || *next.WorktreePath == original["worktree_path"] || next.Branch == nil || *next.Branch == original["branch"] {
		t.Fatalf("successor needs an independent worktree: %+v", next)
	}
	if next.IssueRepo != nil || next.IssueNumber != nil || next.IssueURL != nil {
		t.Fatal("issue provenance must not be inherited")
	}
	var claude, opencode, codex string
	var progress, done sql.NullString
	var spawned bool
	if err := db.QueryRow(`SELECT claude_session_id, opencode_session_id, codex_session_id, in_progress_at, done_at, recurrence_spawned FROM tasks WHERE id = ?`, next.ID).Scan(&claude, &opencode, &codex, &progress, &done, &spawned); err != nil {
		t.Fatal(err)
	}
	if claude != "claude-conversation" || opencode != "opencode-conversation" || codex != "codex-conversation" || progress.Valid || done.Valid || spawned {
		t.Fatal("successor must inherit only conversation IDs and content")
	}
	// Retries, reordering within Done, and reopening never create duplicates.
	recurrentMove(t, srv, id, "done")
	recurrentMove(t, srv, id, "todo")
	recurrentMove(t, srv, id, "done")
	taskCount(t, db, pid, 2)
	// Edits and resets affect only this instance and future successors.
	if _, err := db.Exec(`UPDATE tasks SET title = 'Next iteration', codex_session_id = 'reset-conversation' WHERE id = ?`, next.ID); err != nil {
		t.Fatal(err)
	}
	recurrentMove(t, srv, next.ID, "done")
	taskCount(t, db, pid, 3)
	var title, conversation string
	if err := db.QueryRow(`SELECT title, codex_session_id FROM tasks WHERE status = 'todo' AND project_id = ?`, pid).Scan(&title, &conversation); err != nil {
		t.Fatal(err)
	}
	if title != "Next iteration" || conversation != "reset-conversation" {
		t.Fatal("latest content/session did not carry forward")
	}
	if err := db.QueryRow(`SELECT title, codex_session_id FROM tasks WHERE id = ?`, id).Scan(&title, &conversation); err != nil {
		t.Fatal(err)
	}
	if title != "Latest title" || conversation != "codex-conversation" {
		t.Fatal("prior instance changed")
	}
}

func TestRecurrentToggleAndDeletion(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	id := taskID(t, createTask(t, srv, pid, "No session"))
	recurrentMove(t, srv, id, "done")
	setRecurrent(t, srv, id, true)
	recurrentMove(t, srv, id, "done")
	taskCount(t, db, pid, 1)
	recurrentMove(t, srv, id, "todo")
	recurrentMove(t, srv, id, "done")
	var nextID int64
	var claude, opencode, codex sql.NullString
	if err := db.QueryRow(`SELECT id, claude_session_id, opencode_session_id, codex_session_id FROM tasks WHERE id != ? AND project_id = ?`, id, pid).Scan(&nextID, &claude, &opencode, &codex); err != nil {
		t.Fatal(err)
	}
	if claude.Valid || opencode.Valid || codex.Valid {
		t.Fatal("no session should be invented")
	}
	setRecurrent(t, srv, nextID, false)
	recurrentMove(t, srv, nextID, "done")
	taskCount(t, db, pid, 2)
	code, _ := doJSON(t, "DELETE", fmt.Sprintf("%s/api/tasks/%d", srv.URL, nextID), nil)
	if code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	recurrentMove(t, srv, id, "todo")
	recurrentMove(t, srv, id, "done")
	taskCount(t, db, pid, 1)
}

func TestRecurrentCompletionRollbackAndRetry(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	id := taskID(t, createTask(t, srv, pid, "Atomic"))
	setRecurrent(t, srv, id, true)
	if _, err := db.Exec(`CREATE TRIGGER reject_successor BEFORE INSERT ON tasks BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	code, _ := doJSON(t, "POST", fmt.Sprintf("%s/api/tasks/%d/move", srv.URL, id), map[string]any{"status": "done"})
	if code != http.StatusInternalServerError {
		t.Fatalf("failed completion: %d", code)
	}
	var status string
	var spawned bool
	if err := db.QueryRow(`SELECT status, recurrence_spawned FROM tasks WHERE id = ?`, id).Scan(&status, &spawned); err != nil {
		t.Fatal(err)
	}
	if status != "todo" || spawned {
		t.Fatal("failed completion was not rolled back")
	}
	taskCount(t, db, pid, 1)
	if _, err := db.Exec(`DROP TRIGGER reject_successor`); err != nil {
		t.Fatal(err)
	}
	recurrentMove(t, srv, id, "done")
	taskCount(t, db, pid, 2)
}

func TestRecurrentConcurrentCompletion(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	id := taskID(t, createTask(t, srv, pid, "Concurrent"))
	setRecurrent(t, srv, id, true)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Post(fmt.Sprintf("%s/api/tasks/%d/move", srv.URL, id), "application/json", strings.NewReader(`{"status":"done"}`))
			if err != nil {
				t.Error(err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("concurrent completion: %d", resp.StatusCode)
			}
		}()
	}
	wg.Wait()
	taskCount(t, db, pid, 2)
}

func TestRecurrentConversationExclusiveResume(t *testing.T) {
	srv, mgr, db := newAgentServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	id := taskID(t, createTask(t, srv, pid, "Shared conversation"))
	running := spawnAgentFor(t, srv, id)
	setRecurrent(t, srv, id, true)
	recurrentMove(t, srv, id, "done")
	var nextID int64
	var conversation string
	if err := db.QueryRow(`SELECT id, claude_session_id FROM tasks WHERE project_id = ? AND status = 'todo'`, pid).Scan(&nextID, &conversation); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeTranscriptFixture(t, root, conversation)
	h := &sessionHandlers{mgr: mgr, db: db, globRoot: root}
	resume := func(taskID int64) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/sessions", strings.NewReader(fmt.Sprintf(`{"task_id":%d,"kind":"agent","resume":true}`, taskID)))
		w := httptest.NewRecorder()
		h.create(w, r)
		return w
	}
	w := resume(nextID)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "another task") {
		t.Fatalf("expected shared-conversation conflict: %d %s", w.Code, w.Body.String())
	}
	stopAndWait(t, mgr, running)
	w = resume(nextID)
	if w.Code != http.StatusCreated {
		t.Fatalf("resume after stop: %d %s", w.Code, w.Body.String())
	}
	var nextSession *session.Session
	for _, info := range mgr.ListByTask(nextID) {
		if info.Kind == session.KindAgent {
			nextSession, _ = mgr.Get(info.ID)
		}
	}
	if nextSession == nil || nextSession.ClaudeSessionID() != conversation {
		t.Fatal("successor did not resume the same conversation")
	}
	w = resume(id)
	if w.Code != http.StatusConflict {
		t.Fatalf("original must not concurrently resume: %d", w.Code)
	}
}

func TestRecurrentWorktreeFailureKeepsSuccessor(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	id := taskID(t, createTask(t, srv, pid, "Retry worktree"))
	setRecurrent(t, srv, id, true)
	if _, err := db.Exec(`UPDATE projects SET repo_path = ? WHERE id = ?`, t.TempDir()+"/missing", pid); err != nil {
		t.Fatal(err)
	}
	recurrentMove(t, srv, id, "done")
	next, err := scanTask(db.QueryRow(`SELECT `+taskColumns+` FROM tasks WHERE project_id = ? AND status = 'todo'`, pid))
	if err != nil {
		t.Fatal(err)
	}
	if next.WorktreeError == nil || next.WorktreePath != nil {
		t.Fatal("worktree failure should leave a retryable successor")
	}
	recurrentMove(t, srv, id, "done")
	taskCount(t, db, pid, 2)
}

func TestRecurrentOtherEngineExclusiveResume(t *testing.T) {
	for _, engine := range []string{"codex", "opencode"} {
		t.Run(engine, func(t *testing.T) {
			var srv *httptest.Server
			var mgr *session.Manager
			var db *sql.DB
			var agentID int64
			if engine == "codex" {
				srv, mgr, db, agentID, _ = newCodexResumeServer(t)
			} else {
				srv, mgr, db, agentID, _ = newOpencodeResumeServer(t)
			}
			pid := createProject(t, srv, gitRepoWithCommit(t))
			assignProjectToAgent(t, db, pid, agentID)
			id := taskID(t, createTask(t, srv, pid, "Engine recurrence"))
			column := engine + "_session_id"
			if _, err := db.Exec(`UPDATE tasks SET `+column+` = 'shared-conversation' WHERE id = ?`, id); err != nil {
				t.Fatal(err)
			}
			setRecurrent(t, srv, id, true)
			recurrentMove(t, srv, id, "done")
			var nextID int64
			if err := db.QueryRow(`SELECT id FROM tasks WHERE project_id = ? AND status = 'todo'`, pid).Scan(&nextID); err != nil {
				t.Fatal(err)
			}
			// Two simultaneous resumes must serialize their check and spawn: only one wins.
			var wg sync.WaitGroup
			codes := make(chan int, 2)
			for _, tid := range []int64{id, nextID} {
				wg.Add(1)
				go func(tid int64) {
					defer wg.Done()
					resp, err := http.Post(srv.URL+"/api/sessions", "application/json", strings.NewReader(fmt.Sprintf(`{"kind":"agent","task_id":%d,"resume":true}`, tid)))
					if err != nil {
						t.Error(err)
						return
					}
					defer resp.Body.Close()
					codes <- resp.StatusCode
				}(tid)
			}
			wg.Wait()
			close(codes)
			counts := map[int]int{}
			for code := range codes {
				counts[code]++
			}
			if counts[http.StatusCreated] != 1 || counts[http.StatusConflict] != 1 {
				t.Fatalf("simultaneous resume codes: %v", counts)
			}
			for _, info := range mgr.List() {
				if info.Status == session.StatusRunning {
					stopAndWait(t, mgr, info.ID)
				}
			}
			code, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"kind": "agent", "task_id": nextID, "resume": true})
			if code != http.StatusCreated {
				t.Fatalf("resume after stop: %d %v", code, body)
			}
		})
	}
}
