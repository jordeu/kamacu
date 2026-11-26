package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"kamacu/internal/projectcontext"
	"kamacu/internal/session"
	"kamacu/internal/tmux"
	"kamacu/internal/worktree"
)

func TestProjectContextOnStartAndResume(t *testing.T) {
	srv, mgr, db, transcripts, record := newResumeServer(t)
	id, _ := worktreeTask(t, srv, "Live project instructions")
	var projectID int64
	if err := db.QueryRow("SELECT project_id FROM tasks WHERE id=?", id).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	dir := filepath.Join(base, fmt.Sprint(projectID), "context folder")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	stub := filepath.Join(base, "test-agent")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\ncat \"$KAMACU_PROJECT_CONTEXT/instructions.md\" > \"$FAKE_CLAUDE_ARGS_FILE\"\nexec sleep 60\n"), 0700); err != nil {
		t.Fatal(err)
	}
	mgr.SetAgentConfig(session.AgentConfig{ClaudeBin: stub, ProjectContext: func(id int64, engine string) (string, error) {
		if engine != "claude" {
			return "", fmt.Errorf("unexpected engine %s", engine)
		}
		return filepath.Join(base, fmt.Sprint(id), "context folder"), nil
	}})
	t.Setenv("KAMACU_PROJECT_CONTEXT", "wrong inherited directory")
	for i, want := range []string{"original", "edited"} {
		if err := os.WriteFile(filepath.Join(dir, "instructions.md"), []byte(want), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(record, nil, 0600); err != nil {
			t.Fatal(err)
		}
		status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": id, "kind": "agent", "resume": i > 0})
		if status != 201 {
			t.Fatal(status, body)
		}
		if got := readArgv(t, record); len(got) != 1 || got[0] != want {
			t.Fatalf("read %q, want %s", got, want)
		}
		mgr.StopAllForTask(id)
		seedTranscript(t, transcripts, claudeSessionIDFor(t, db, id).String)
	}
}

func TestProjectDeletionArchivesContextBeforeIDReuse(t *testing.T) {
	_, db, dbPath := newTestServer(t)
	root := t.TempDir()
	folders, err := projectcontext.New(root, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RoutesWithProjectContext(mux, db, worktree.NewService(t.TempDir()), session.NewManager(), tmux.Client{}, folders)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	repo := gitRepo(t)
	id := createProject(t, srv, repo)
	dir := folders.Directory(id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "instructions.md"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	// A failed archive must leave the project addressable and its files usable.
	instance := filepath.Dir(filepath.Dir(filepath.Dir(dir)))
	archiveRoot := filepath.Join(instance, "archive")
	if err := os.WriteFile(archiveRoot, []byte("blocked archive directory"), 0600); err != nil {
		t.Fatal(err)
	}
	url := fmt.Sprintf("%s/api/projects/%d", srv.URL, id)
	if status, _ := doJSON(t, "DELETE", url, nil); status != 500 {
		t.Fatal(status)
	}
	if status, _ := doJSON(t, "GET", url, nil); status != 200 {
		t.Fatal(status)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "instructions.md")); err != nil || string(got) != "original" {
		t.Fatal(string(got), err)
	}
	if err := os.Remove(archiveRoot); err != nil {
		t.Fatal(err)
	}
	// Failure after staging must restore the folder before the request returns.
	if _, err := db.Exec("CREATE TRIGGER reject_delete BEFORE DELETE ON projects BEGIN SELECT RAISE(ABORT,'test failure'); END"); err != nil {
		t.Fatal(err)
	}
	if status, _ := doJSON(t, "DELETE", url, nil); status != 500 {
		t.Fatal(status)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "instructions.md")); err != nil || string(got) != "original" {
		t.Fatalf("rollback lost instructions: %q %v", got, err)
	}
	if _, err := db.Exec("DROP TRIGGER reject_delete"); err != nil {
		t.Fatal(err)
	}
	status, body := doJSON(t, "DELETE", url, nil)
	if status != 204 {
		t.Fatal(status, body)
	}
	if next := createProject(t, srv, repo); next != id {
		t.Fatalf("expected reuse, got %d", next)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("inherited context", err)
	}
	archives, err := filepath.Glob(filepath.Join(root, "instances", "*", "archive", "projects", fmt.Sprint(id), "*", "context", "instructions.md"))
	if err != nil || len(archives) != 1 {
		t.Fatal(archives, err)
	}
}

func TestArchiveFailurePreservesManagedWorktreeAndAgent(t *testing.T) {
	_, db, dbPath := newTestServer(t)
	root := t.TempDir()
	folders, err := projectcontext.New(root, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	mgr := session.NewManager()
	mgr.SetAgentConfig(session.AgentConfig{ClaudeBin: testdataFakeClaude(t), ProjectContext: func(id int64, engine string) (string, error) { return folders.Directory(id), nil }})
	mux := http.NewServeMux()
	RoutesWithProjectContext(mux, db, worktree.NewService(t.TempDir()), mgr, tmux.Client{}, folders)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	id, _, clone := makeManagedProject(t, db)
	task, wt := addManagedTaskWorktree(t, srv, id, "preserve on archive error")
	agent, err := mgr.Spawn(session.SpawnOpts{Kind: session.KindAgent, ProjectID: id, TaskID: task, Cwd: wt, AgentEngine: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	defer agent.Stop()
	dir := folders.Directory(id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	archiveRoot := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(dir))), "archive")
	if err := os.WriteFile(archiveRoot, nil, 0600); err != nil {
		t.Fatal(err)
	}
	status, body := doJSON(t, "DELETE", fmt.Sprintf("%s/api/projects/%d", srv.URL, id), map[string]bool{"force": true})
	if status != 500 {
		t.Fatalf("status=%d body=%v", status, body)
	}
	for _, p := range []string{wt, clone, dir} {
		if _, err := os.Stat(p); err != nil {
			t.Fatal(p, err)
		}
	}
	if agent.Info().Status != session.StatusRunning {
		t.Fatal("agent stopped before archiving")
	}
}
