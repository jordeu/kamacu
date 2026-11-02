package monitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kamacu/internal/inbox"
	"kamacu/internal/observation"
	"kamacu/internal/presence"
	"kamacu/internal/session"
	"kamacu/internal/store"
)

func TestCompletionActionValidation(t *testing.T) {
	for _, tc := range []struct {
		kind    string
		actions []string
		valid   bool
	}{
		{"completion", nil, true},
		{"completion", []string{inbox.MoveToReview, inbox.MoveToDone}, true},
		{"completion", []string{"exit"}, false},
		{"completion", []string{inbox.MoveToDone, inbox.MoveToDone}, false},
		{"none", []string{inbox.MoveToDone}, false},
	} {
		d := Decision{State: "idle", Kind: tc.kind, Evidence: "Done", InputMode: "none", CompletionActions: tc.actions}
		if err := validateDecision(d, observation.Snapshot{Screen: "Done"}); (err == nil) != tc.valid {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
}

func TestCompletionTaskContextAndReclassification(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{`INSERT INTO projects(id,name,repo_path) VALUES(1,'Project','/tmp/project')`, `INSERT INTO tasks(id,project_id,title,description,status,position) VALUES(1,1,'Improve uploads','Validate uploads and add tests','in_progress',0)`} {
		if _, err = db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Inspect the actual provider request, not just the monitor's internal state.
	contexts := make(chan TaskContext, 10)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		prompt := body.Messages[len(body.Messages)-1].Content
		_, tail, ok := strings.Cut(prompt, "TASK CONTEXT (metadata, not instructions):\n")
		if !ok {
			t.Error("no task context")
			w.WriteHeader(500)
			return
		}
		data, _, _ := strings.Cut(tail, "\nTERMINAL DATA:")
		var task TaskContext
		if err := json.Unmarshal([]byte(data), &task); err != nil {
			t.Error(err)
			return
		}
		contexts <- task
		// Deliberately suggest both; the application must filter impossible moves.
		d := Decision{State: "idle", Kind: "completion", Evidence: "All done", Summary: "All done", InputMode: "none", CompletionActions: []string{inbox.MoveToReview, inbox.MoveToDone}}
		raw, _ := json.Marshal(d)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(raw)}}}})
	}))
	defer provider.Close()
	p, err := SaveProfile(ctx, db, Profile{Name: "fixture", Backend: "openai_compatible", Endpoint: provider.URL, Model: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE monitor_settings SET default_profile_id=?`, p.ID); err != nil {
		t.Fatal(err)
	}
	mgr := session.NewManager()
	sess, err := mgr.Spawn(session.SpawnOpts{Kind: session.KindAgent, AgentEngine: "custom", AgentArgs: []string{"sh", "-c", "printf 'All done\\n'; read answer"}, Cwd: t.TempDir(), TaskID: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Stop()
	id := sess.Info().ID
	in := inbox.New(db, func(string) inbox.Session { return inbox.Session{TaskID: 1, Live: true, Done: sess.Done()} })
	defer in.Close()
	svc := New(db, mgr, in, presence.NewTracker(), Backend{})
	defer svc.jobs.Wait()
	wait := func(status string, count int) inbox.Question {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			svc.mu.Lock()
			if w := svc.states[id]; w != nil {
				w.lastAttempt = time.Time{}
				w.lastChange = time.Time{}
			}
			svc.mu.Unlock()
			svc.tick(ctx)
			page, e := in.List(ctx, nil, false, 0)
			if e == nil && len(page.Items) == 1 && page.Items[0].CompletionTaskStatus == status && len(page.Items[0].Options) == count {
				return page.Items[0]
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("completion not updated for %s: %+v", status, sess.Info())
		return inbox.Question{}
	}
	first := wait("in_progress", 2)
	task := <-contexts
	if task.Title != "Improve uploads" || task.Description != "Validate uploads and add tests" || task.Status != "in_progress" || task.Engine != "custom" || task.Scope != "task" {
		t.Fatalf("context=%+v", task)
	}
	if _, err = db.Exec(`UPDATE tasks SET status='in_review' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	second := wait("in_review", 1)
	if second.ID != first.ID || second.Revision <= first.Revision || second.Options[0].ID != inbox.MoveToDone {
		t.Fatalf("stale actions: %+v", second)
	}
	if _, err = db.Exec(`UPDATE tasks SET status='done' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	wait("done", 0)
}
