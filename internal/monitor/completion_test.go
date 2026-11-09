package monitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kamacu/internal/diagnostics"
	"kamacu/internal/inbox"
	"kamacu/internal/observation"
	"kamacu/internal/presence"
	"kamacu/internal/session"
	"kamacu/internal/store"
)

// Synthetic counterparts of the missed production cases. Opt-in because these
// exercise actual model judgment, which mocked classifier responses cannot test.
func TestInstalledLoginCompletionRecommendations(t *testing.T) {
	if os.Getenv("KAMACU_MONITOR_CORPUS") != "1" {
		t.Skip("opt-in synthetic live completion corpus")
	}
	for _, backend := range []string{"claude_cli", "codex_cli"} {
		t.Run(backend, func(t *testing.T) {
			t.Parallel()
			for _, tc := range []struct {
				name, source, description, screen string
				done                              bool
			}{
				{"published PR review", "github_pr", "", "I approved the PR on GitHub with two nonblocking inline comments. The review is published and complete.", false},
				{"manual review optional followup", "manual", "Review the PR and publish the requested approval.", "I published the approval and inline comments as requested. I can also investigate the optional performance concern if you want.", true},
				{"Renovate batch merged", "manual", "Review and merge this repository's Renovate batch after green CI.", "I approved and merged all four Renovate PRs. Each had green CI before merging. No Renovate PRs remain open in acme/project.", true},
				{"informational master CI", "manual", "Review and merge this repository's Renovate batch after green CI on each PR.", "All four Renovate PRs are merged with green pre-merge CI. No Renovate PRs remain. Master CI is still queued; no release is needed.", true},
				{"required master CI", "manual", "Merge the batch and verify that post-merge master CI passes.", "All four PRs are merged. Master CI is queued; the required post-merge validation is not yet complete.", false},
				{"extra requested review", "manual", "Merge the Renovate batch and also report findings on the separate Guava security PR. Do not merge that extra PR.", "All four Renovate PRs are merged. I have not yet reviewed or reported findings on the separately requested Guava PR.", false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
					defer cancel()
					task := TaskContext{ID: 1, Title: "Repository maintenance", Description: tc.description, Source: tc.source, Status: "in_progress", Scope: "task", ProjectName: "Project", Repository: "acme/project"}
					d, err := (Backend{}).Classify(ctx, Profile{Backend: backend}, observation.Snapshot{Screen: tc.screen + "\n> ", At: time.Now()}, task)
					if err != nil {
						t.Fatal(err)
					}
					if d.Kind != "completion" || slices.Contains(d.CompletionActions, inbox.MoveToDone) != tc.done {
						t.Fatalf("expected completion with Done recommendation=%v, got %+v", tc.done, d)
					}
					if tc.source == "github_pr" && len(d.CompletionActions) != 0 {
						t.Fatalf("PR review recommended a board move: %+v", d)
					}
				})
			}
		})
	}
}

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
	for _, query := range []string{`INSERT INTO projects(id,name,repo_path,github_repo) VALUES(1,'Project','/tmp/project','acme/project')`, `INSERT INTO tasks(id,project_id,title,description,status,position) VALUES(1,1,'Improve uploads','Validate uploads and add tests','in_progress',0)`} {
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
		// Omit recommendations initially; controls must remain available.
		d := Decision{State: "idle", Kind: "completion", Evidence: "All done", Summary: "All done", InputMode: "none"}
		if task.Status == "in_review" {
			d.CompletionActions = []string{inbox.MoveToDone}
		}
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
	captureDir := t.TempDir()
	recorder, err := diagnostics.NewRecorder(captureDir)
	if err != nil {
		t.Fatal(err)
	}
	svc := New(db, mgr, in, presence.NewTracker(), Backend{Transcripts: recorder})
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
	first := wait("in_progress", 3)
	for _, o := range first.Options {
		if o.Recommended {
			t.Fatalf("invented recommendation: %+v", o)
		}
	}
	task := <-contexts
	if task.Title != "Improve uploads" || task.Description != "Validate uploads and add tests" || task.Status != "in_progress" || task.Engine != "custom" || task.Scope != "task" || task.ProjectName != "Project" || task.Repository != "acme/project" {
		t.Fatalf("context=%+v", task)
	}
	if _, err = db.Exec(`UPDATE tasks SET status='in_review' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	second := wait("in_review", 2)
	if second.ID != first.ID || second.Revision <= first.Revision || second.Options[0].ID != inbox.MoveToDone || !second.Options[0].Recommended {
		t.Fatalf("stale actions: %+v", second)
	}
	if _, err = db.Exec(`UPDATE tasks SET status='done' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	wait("done", 1)
	daily := filepath.Join(captureDir, "llm", "monitoring", "session-"+id, time.Now().UTC().Format("2006-01-02"))
	data, err := os.ReadFile(filepath.Join(daily, "group.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct{ Group diagnostics.Group }
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Group.SessionID != id || manifest.Group.TaskTitle != "Improve uploads" || manifest.Group.ProjectName != "Project" {
		t.Fatalf("group: %+v", manifest.Group)
	}
	entries, err := os.ReadDir(daily)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			count++
		}
	}
	if count < 3 {
		t.Fatalf("missing reclassification transcripts: %d", count)
	}
}
