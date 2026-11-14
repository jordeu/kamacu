package inbox

import (
	"fmt"
	"reflect"
	"testing"
)

func insertCatchUpMessage(t *testing.T, s *Service, id string, task any, origin, status string, created int, hidden bool) {
	t.Helper()
	_, err := s.db.Exec(`INSERT INTO inbox_questions(id,task_id,session_id,created_session_id,request_key,summary,context,question,options,status,created_at,updated_at,heartbeat_at,origin,hidden,queue_position) VALUES(?,?,?,'one',?,'Summary','Context','Question','[]',?,?,?,?,?,?,?)`, id, task, id, id, status, created, created, created, origin, hidden, created)
	if err != nil {
		t.Fatal(err)
	}
}
func TestCatchUpCapturesWholePendingQueueWithoutMutation(t *testing.T) {
	s, _ := fixture(t)
	s.lookup = func(string) Session { return Session{TaskID: 1, Live: true} }
	for i := 0; i < 55; i++ {
		insertCatchUpMessage(t, s, fmt.Sprintf("pending-%02d", i), 1, "agent", "pending", 100+i, false)
	}
	insertCatchUpMessage(t, s, "hidden", 1, "agent", "pending", 1, true)
	insertCatchUpMessage(t, s, "archived", 1, "agent", "answered", 2, false)
	pass, err := s.CatchUp(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(pass) != 55 || pass[0].ID != "pending-00" || pass[54].ID != "pending-54" {
		t.Fatal(len(pass), pass)
	}
	insertCatchUpMessage(t, s, "arrival", 1, "agent", "pending", 200, false)
	if _, err = s.db.Exec(`UPDATE inbox_questions SET status='answered' WHERE id='pending-01'`); err != nil {
		t.Fatal(err)
	}
	next, err := s.CatchUp(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(pass) != 55 || pass[1].Status != "pending" || next[len(next)-1].ID != "arrival" {
		t.Fatal("snapshot changed", pass, next)
	}
}
func TestCatchUpHistoryExplicitLinksChronologyAndPagination(t *testing.T) {
	s, _ := fixture(t)
	for i := 0; i < 52; i++ {
		insertCatchUpMessage(t, s, fmt.Sprintf("old-%02d", i), 1, "agent", "answered", 100+i, false)
	}
	insertCatchUpMessage(t, s, "linked-notification", nil, "notification", "answered", 200, false)
	insertCatchUpMessage(t, s, "unrelated", 2, "agent", "answered", 201, false)
	insertCatchUpMessage(t, s, "hidden", 1, "agent", "answered", 202, true)
	insertCatchUpMessage(t, s, "equal-before", 1, "agent", "answered", 300, false)
	insertCatchUpMessage(t, s, "selected", nil, "notification", "answered", 300, false)
	insertCatchUpMessage(t, s, "equal-after", 1, "agent", "answered", 300, false)
	insertCatchUpMessage(t, s, "future", 1, "agent", "answered", 301, false)
	for _, id := range []string{"linked-notification", "selected"} {
		if _, err := s.db.Exec(`INSERT INTO notification_task_links(question_id,task_id,role,source) VALUES(?,1,'implementation','test')`, id); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.History(t.Context(), "selected", 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.History(t.Context(), "selected", 50)
	if err != nil {
		t.Fatal(err)
	}
	if !first.HasMore || second.HasMore || len(first.Items) != 50 || len(second.Items) != 4 {
		t.Fatal(first, second)
	}
	got := []string{}
	for _, q := range append(first.Items, second.Items...) {
		got = append(got, q.ID)
	}
	want := []string{"equal-before", "linked-notification"}
	for i := 51; i >= 0; i-- {
		want = append(want, fmt.Sprintf("old-%02d", i))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got, want)
	}
	if _, err = s.History(t.Context(), "missing", 0); err != ErrNotFound {
		t.Fatal(err)
	}
	if _, err = s.History(t.Context(), "selected", -1); err != ErrInvalid {
		t.Fatal(err)
	}
}
func TestCatchUpScratchpadHistoryExcludesUnrelatedNotifications(t *testing.T) {
	s, _ := fixture(t)
	insertCatchUpMessage(t, s, "scratch", nil, "agent", "answered", 100, false)
	insertCatchUpMessage(t, s, "notification", nil, "notification", "answered", 101, false)
	insertCatchUpMessage(t, s, "selected", nil, "observer", "answered", 102, false)
	page, err := s.History(t.Context(), "selected", 0)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "scratch" {
		t.Fatal(page, err)
	}
}
