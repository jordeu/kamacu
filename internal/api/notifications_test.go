package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"kamacu/internal/inbox"
	"kamacu/internal/notifications"
	"kamacu/internal/store"
)

func TestNotificationSourceRoutesAndDurableAnswer(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	box := inbox.New(db, func(string) inbox.Session { return inbox.Session{} })
	defer box.Close()
	svc := notifications.New(db, box, nil, nil)
	mux := http.NewServeMux()
	NotificationRoutes(mux, svc)
	delivered := 0
	inboxRoutes(mux, box, "token", func(inbox.Question) error { delivered++; return nil })
	call := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	w := call("POST", "/api/notifications/sources", `{"provider":"github","name":"GitHub","config":{}}`)
	if w.Code != 201 {
		t.Fatalf("add: %d %s", w.Code, w.Body.String())
	}
	var source notifications.Source
	if err = json.Unmarshal(w.Body.Bytes(), &source); err != nil {
		t.Fatal(err)
	}
	if w = call("POST", "/api/notifications/sources", `{"provider":"github","name":"Duplicate","config":{}}`); w.Code != 400 {
		t.Fatal("duplicate source accepted")
	}
	if w = call("POST", "/api/notifications/sources", `{"provider":"unknown","name":"Other"}`); w.Code != 400 {
		t.Fatal("unknown provider accepted")
	}
	result, err := db.Exec(`INSERT INTO notification_events(source_id,external_id,version,payload,created_at) VALUES(?,'thread','v1','{}',0)`, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	eventID, _ := result.LastInsertId()
	q, err := box.PublishNotification(context.Background(), eventID, "GitHub", "https://github.com/acme/repo/pull/7", inbox.Create{Abstract: "Review requested", Summary: "Review requested", Context: "A new pull request needs review.", Question: "Start review?"})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		w = call("POST", "/api/inbox/"+q.ID+"/answer", `{"revision":1,"answer":"Start review"}`)
		if w.Code != 200 {
			t.Fatalf("answer: %d %s", w.Code, w.Body.String())
		}
	}
	if delivered != 0 {
		t.Fatal("notification answer delivered to terminal")
	}
	var runs int
	db.QueryRow(`SELECT count(*) FROM notification_runs`).Scan(&runs)
	if runs != 1 {
		t.Fatalf("runs: %d", runs)
	}
	if w = call("PATCH", "/api/notifications/sources/"+source.ID, `{"enabled":false}`); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w = call("GET", "/api/notifications/sources", ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w = call("DELETE", "/api/notifications/sources/"+source.ID, ""); w.Code != 204 {
		t.Fatal(w.Body.String())
	}
	if w = call("POST", "/api/notifications/sources", `{"provider":"github","name":"Reconnected","config":{}}`); w.Code != 201 {
		t.Fatalf("reconnect: %s", w.Body.String())
	}
	if _, err = box.Get(context.Background(), q.ID); err != nil {
		t.Fatal("disconnect lost history", err)
	}
}

func TestRawNotificationRoutesAndManualProcessingRequest(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	box := inbox.New(db, func(string) inbox.Session { return inbox.Session{} })
	defer box.Close()
	service := notifications.New(db, box, nil, nil)
	mux := http.NewServeMux()
	NotificationRoutes(mux, service)
	source, err := service.AddSource(context.Background(), "github", "GitHub", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"external_id":"raw-thread","version":"v1","title":"Raw notification","repository":"acme/repo","url":"https://github.com/acme/repo/pull/7"}`
	result, err := db.Exec(`INSERT INTO notification_events(source_id,external_id,version,payload,created_at) VALUES(?,'raw-thread','v1',?,0)`, source.ID, payload)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	call := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	w := call("GET", "/api/notifications", "")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var page notifications.RawPage
	if err = json.Unmarshal(w.Body.Bytes(), &page); err != nil || page.PendingCount != 1 || len(page.Items) != 1 || page.Items[0].Event.Title != "Raw notification" {
		t.Fatalf("raw page: %+v %v", page, err)
	}
	path := fmt.Sprintf("/api/notifications/%d", id)
	for _, status := range []string{"archived", "pending"} {
		w = call("PATCH", path, fmt.Sprintf(`{"status":%q}`, status))
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	if w = call("GET", "/api/notifications?status=bad", ""); w.Code != 400 {
		t.Fatal("accepted invalid raw view")
	}
	if w = call("GET", "/api/notifications?offset=-1", ""); w.Code != 400 {
		t.Fatal("accepted invalid offset")
	}
	if w = call("PATCH", path, `{"status":"processed"}`); w.Code != 400 {
		t.Fatal("accepted third raw state")
	}
	w = call("POST", "/api/notifications/process", "")
	if w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	var first notifications.Processing
	json.Unmarshal(w.Body.Bytes(), &first)
	w = call("POST", "/api/notifications/process", "")
	var second notifications.Processing
	json.Unmarshal(w.Body.Bytes(), &second)
	if w.Code != 202 || first.ID != second.ID || first.NotificationCount != 1 {
		t.Fatal("duplicate manual request")
	}
	if w = call("PATCH", path, `{"status":"archived"}`); w.Code != 409 {
		t.Fatal("changed active processing snapshot")
	}
	w = call("GET", "/api/notifications/status", "")
	var status notifications.Status
	json.Unmarshal(w.Body.Bytes(), &status)
	if w.Code != 200 || status.PendingCount != 1 || status.Processing.Status != "queued" {
		t.Fatalf("status: %+v", status)
	}
	// Enqueuing and browsing never need a configured model or create inbox cards.
	cards, err := box.ListWithOrder(context.Background(), nil, false, 0, true)
	if err != nil || len(cards.Items) != 0 {
		t.Fatal("manual request performed inference synchronously")
	}
}
