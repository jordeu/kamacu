package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kamacu/internal/artifact"
	"kamacu/internal/inbox"
	"kamacu/internal/integration"
	"kamacu/internal/store"
)

func TestInboxHTTPIdentityAndBrowserResolution(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`INSERT INTO projects(id,name,repo_path) VALUES(1,'P','/tmp/p')`, `INSERT INTO tasks(id,project_id,title,position) VALUES(1,1,'Task',0)`} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	svc := inbox.New(db, func(id string) inbox.Session {
		if id == "scratch" {
			return inbox.Session{Global: true, Live: true}
		}
		if id == "live" {
			return inbox.Session{TaskID: 1, Live: true}
		}
		return inbox.Session{}
	})
	defer svc.Close()
	mux := http.NewServeMux()
	InboxRoutes(mux, svc, "token")
	handler := BrowserOriginGuard(mux, nil)
	call := func(path, token, task, session string, body any) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		r.Header.Set("X-Kamacu-Token", token)
		r.Header.Set("X-Kamacu-Task-ID", task)
		r.Header.Set("X-Kamacu-Session-ID", session)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	body := map[string]any{"request_key": "key", "abstract": "Compact message", "summary": "Summary", "context": "Enough context", "question": "Proceed?", "options": []inbox.Option{{ID: "yes", Label: "Yes"}}}
	for _, tc := range []struct {
		token, task, session string
		want                 int
	}{{"", "1", "live", 401}, {"token", "", "live", 403}, {"token", "2", "live", 403}, {"token", "1", "unknown", 403}, {"token", "", "scratch", 403}, {"token", "garbage", "scratch", 403}, {"token", "1", "scratch", 403}, {"token", "0", "live", 403}} {
		if w := call("/api/agent-feedback/ask", tc.token, tc.task, tc.session, body); w.Code != tc.want {
			t.Fatalf("identity %v: %d %s", tc, w.Code, w.Body.String())
		}
	}
	if w := call("/api/agent-feedback/ask", "token", "0", "scratch", body); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w := call("/api/agent-feedback/ask", "token", "1", "live", body)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var created inbox.Result
	if err = json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	readMessage := func(id string, want int) inbox.Question {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/inbox/"+id, nil))
		if w.Code != want {
			t.Fatalf("read message: %d %s", w.Code, w.Body.String())
		}
		var q inbox.Question
		if want == http.StatusOK {
			if err := json.Unmarshal(w.Body.Bytes(), &q); err != nil {
				t.Fatal(err)
			}
		}
		return q
	}
	if q := readMessage(created.Question.ID, http.StatusOK); q.ID != created.Question.ID || q.Status != "pending" {
		t.Fatalf("pending message: %+v", q)
	}
	readMessage("missing", http.StatusNotFound)
	path := "/api/inbox/" + created.Question.ID + "/answer"
	req := httptest.NewRequest("POST", path, bytes.NewBufferString(`{"revision":1,"option_id":"yes"}`))
	req.Header.Set("Origin", "https://evil.invalid")
	blocked := httptest.NewRecorder()
	handler.ServeHTTP(blocked, req)
	if blocked.Code != 403 {
		t.Fatal("cross-origin answer permitted")
	}
	w = call(path, "", "", "", map[string]any{"revision": 1, "option_id": "yes"})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = call("/api/agent-feedback/resolve", "token", "1", "live", map[string]any{"question_id": created.Question.ID, "revision": 1, "answer": "No"})
	var resolved inbox.Result
	json.Unmarshal(w.Body.Bytes(), &resolved)
	if w.Code != 200 || resolved.Outcome != "already_resolved" || resolved.Question.Answer != "Yes" {
		t.Fatalf("conflict: %s", w.Body.String())
	}
	if q := readMessage(created.Question.ID, http.StatusOK); q.Status != "answered" || q.Answer != "Yes" {
		t.Fatalf("archived message: %+v", q)
	}
	page, err := svc.List(context.Background(), nil, true, 0)
	if err != nil || page.PendingCount != 1 || len(page.Items) != 1 {
		t.Fatalf("archive: %+v %v", page, err)
	}
}

// Opt-in browser acceptance against the built UI and a real disposable inbox
// API. Other application endpoints are mocked by the browser script.
func TestInboxBrowser(t *testing.T) {
	if os.Getenv("KAMACU_BROWSER_TEST") != "1" {
		t.Skip("set KAMACU_BROWSER_TEST=1 and KAMACU_PLAYWRIGHT_PACKAGE")
	}
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`INSERT INTO projects(id,name,repo_path) VALUES(1,'Kamacu','/tmp/p')`, `INSERT INTO tasks(id,project_id,title,position) VALUES(1,1,'Improve task navigation',0)`} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	svc := inbox.New(db, func(id string) inbox.Session {
		if id == "scratch-session" {
			return inbox.Session{Global: true, Live: true}
		}
		if id == "browser-session" {
			return inbox.Session{TaskID: 1, Live: true}
		}
		return inbox.Session{}
	})
	defer svc.Close()
	mux := http.NewServeMux()
	InboxRoutes(mux, svc, "test-token")
	artifacts, err := artifact.New(db, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ArtifactRoutes(mux, artifacts, "test-token")
	integrationHome := t.TempDir()
	fakeBin := t.TempDir()
	if err := os.WriteFile(filepath.Join(fakeBin, "claude"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	IntegrationRoutes(mux, integration.New(integrationHome, filepath.Join(integrationHome, ".config"), "/usr/bin/kamacu"))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		root := "../../web/dist"
		if _, err := os.Stat(filepath.Join(root, r.URL.Path)); err == nil && r.URL.Path != "/" {
			http.FileServer(http.Dir(root)).ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
	})
	srv := httptest.NewServer(BrowserOriginGuard(mux, nil))
	defer srv.Close()
	cmd := exec.Command("node", "../../scripts/inbox-browser-check.cjs", srv.URL)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("browser acceptance: %v\n%s", err, output)
	} else {
		t.Log(string(output))
	}
}

func TestCompletionExitRequiresWinningOption(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "completion.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	svc := inbox.New(db, func(id string) inbox.Session { return inbox.Session{Global: true, Live: id == "scratch"} })
	defer svc.Close()
	stopped := ""
	mux := http.NewServeMux()
	InboxRoutes(mux, svc, "token", func(id string) { stopped = id })
	for _, tc := range []struct {
		key, answer, option string
		exit                bool
	}{
		{"text", inbox.CompletionExit, "", false},
		{"exit", "", "exit", true},
	} {
		r, err := svc.Complete(context.Background(), inbox.Identity{TaskID: 0, SessionID: "scratch"}, inbox.Create{Key: tc.key, Abstract: "Compact message", Summary: "Finished", Context: "Verified"})
		if err != nil {
			t.Fatal(err)
		}
		call := func(answer, option string) {
			raw, _ := json.Marshal(map[string]any{"revision": r.Question.Revision, "answer": answer, "option_id": option})
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/inbox/"+r.Question.ID+"/answer", bytes.NewReader(raw)))
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
		}
		call(tc.answer, tc.option)
		if (stopped != "") != tc.exit {
			t.Fatalf("%s: stopped %q", tc.key, stopped)
		}
		// A losing exit submission cannot override text or Seen.
		call("", "exit")
		if (stopped != "") != tc.exit {
			t.Fatalf("%s: losing answer stopped session", tc.key)
		}
	}
}

func TestInboxTerminalDelivery(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint("fail=", fail), func(t *testing.T) {
			db, err := store.Open(filepath.Join(t.TempDir(), "inbox.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err := store.Migrate(db); err != nil {
				t.Fatal(err)
			}
			svc := inbox.New(db, func(string) inbox.Session { return inbox.Session{Global: true, Live: true} })
			defer svc.Close()
			who := inbox.Identity{SessionID: "scratch"}
			calls := 0
			stopped := 0
			mux := http.NewServeMux()
			inboxRoutes(mux, svc, "token", func(q inbox.Question) error {
				calls++
				if q.SessionID != who.SessionID || q.Answer == "" {
					t.Fatalf("wrong recipient/payload: %+v", q)
				}
				if strings.Contains(feedbackMessage(q), "\x1b") {
					t.Fatal("unescaped terminal control")
				}
				if fail {
					return fmt.Errorf("PTY closed")
				}
				return nil
			}, func(string) { stopped++ })
			for _, action := range []string{"text", "exit"} {
				created, err := svc.Complete(context.Background(), who, inbox.Create{Key: action, Abstract: "Compact message", Summary: "Done", Context: "Tests passed"})
				if err != nil {
					t.Fatal(err)
				}
				body := inboxAnswer{Revision: created.Question.Revision, OptionID: action}
				if action == "text" {
					body.OptionID = ""
					body.Answer = "line one\n\x1b[201~line two"
				}
				raw, _ := json.Marshal(body)
				before := calls
				for attempt := 0; attempt < 2; attempt++ {
					w := httptest.NewRecorder()
					mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/inbox/"+created.Question.ID+"/answer", bytes.NewReader(raw)))
					want := 200
					if fail && action != "exit" && attempt == 0 {
						want = 502
					}
					if w.Code != want {
						t.Fatalf("%s attempt %d: %d %s", action, attempt, w.Code, w.Body.String())
					}
				}
				expected := before + 1
				if action == "exit" {
					expected = before
				}
				if calls != expected {
					t.Fatalf("%s: delivery calls=%d want=%d", action, calls, expected)
				}
				page, err := svc.List(context.Background(), &who, false, 0)
				if err != nil || page.Items[0].Status != "answered" {
					t.Fatalf("answer not durable: %+v %v", page, err)
				}
			}
			if stopped == 0 {
				t.Fatal("exit did not stop session")
			}
		})
	}
}
