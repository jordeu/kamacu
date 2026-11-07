package api

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestSnoozeLifecycle(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	for _, origin := range []string{"in_progress", "in_review"} {
		t.Run(origin, func(t *testing.T) {
			id := taskID(t, createTask(t, srv, pid, "Snoozable"))
			recurrentMove(t, srv, id, origin)
			setRecurrent(t, srv, id, true)
			until := time.Now().Add(time.Hour).UTC().Truncate(time.Millisecond)
			url := fmt.Sprintf("%s/api/tasks/%d", srv.URL, id)
			code, body := doJSON(t, "POST", url+"/snooze", map[string]any{"until": until.Format(time.RFC3339Nano)})
			if code != http.StatusOK || body["status"] != "todo" || body["snoozed_from"] != origin || body["snoozed_until"] != until.Format(snoozeTimeFormat) || body["recurrent"] != true {
				t.Fatalf("snooze: %d %v", code, body)
			}
			// A fresh handler simulates restarting with only persisted state available.
			h := &taskHandlers{db: db}
			if err := h.reactivateDue(context.Background(), until.Add(-time.Millisecond)); err != nil {
				t.Fatal(err)
			}
			current, err := scanTask(db.QueryRow(`SELECT `+taskColumns+` FROM tasks WHERE id=?`, id))
			if err != nil || current.Status != "todo" {
				t.Fatalf("woke early: %+v %v", current, err)
			}
			if err := h.reactivateDue(context.Background(), until); err != nil {
				t.Fatal(err)
			}
			current, err = scanTask(db.QueryRow(`SELECT `+taskColumns+` FROM tasks WHERE id=?`, id))
			if err != nil || current.Status != origin || current.SnoozedUntil != nil || current.SnoozedFrom != nil {
				t.Fatalf("wake: %+v %v", current, err)
			}
			// Resnooze and reactivate manually; a second reactivation must be harmless.
			code, body = doJSON(t, "POST", url+"/snooze", map[string]any{"until": until.Format(time.RFC3339Nano)})
			if code != http.StatusOK {
				t.Fatalf("resnooze: %d %v", code, body)
			}
			for range 2 {
				code, body = doJSON(t, "POST", url+"/reactivate", nil)
				if code != http.StatusOK || body["status"] != origin || body["snoozed_until"] != nil || body["snoozed_from"] != nil {
					t.Fatalf("reactivate: %d %v", code, body)
				}
			}
		})
	}
	taskCount(t, db, pid, 2) // Snoozing a recurring task never creates a successor.
}

func TestSnoozeValidation(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	id := taskID(t, createTask(t, srv, pid, "Validation"))
	url := fmt.Sprintf("%s/api/tasks/%d/snooze", srv.URL, id)
	future := map[string]any{"until": time.Now().Add(time.Hour).Format(time.RFC3339Nano)}
	for _, status := range []string{"todo", "done"} {
		recurrentMove(t, srv, id, status)
		code, body := doJSON(t, "POST", url, future)
		if code != http.StatusConflict {
			t.Fatalf("snoozed %s: %d %v", status, code, body)
		}
	}
	recurrentMove(t, srv, id, "in_progress")
	for _, input := range []map[string]any{{}, {"until": "invalid"}, {"until": time.Now().Add(-time.Minute).Format(time.RFC3339Nano)}} {
		code, body := doJSON(t, "POST", url, input)
		if code != http.StatusBadRequest {
			t.Fatalf("accepted invalid time: %d %v", code, body)
		}
	}
	if _, err := db.Exec(`UPDATE tasks SET source='github_pr' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	code, body := doJSON(t, "POST", url, future)
	if code != http.StatusConflict {
		t.Fatalf("snoozed PR workspace: %d %v", code, body)
	}
	code, _ = doJSON(t, "POST", srv.URL+"/api/tasks/999999/snooze", future)
	if code != http.StatusNotFound {
		t.Fatalf("missing task: %d", code)
	}
}

func TestSnoozeCompetingMoves(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	id := taskID(t, createTask(t, srv, pid, "Competing moves"))
	h := &taskHandlers{db: db}
	url := fmt.Sprintf("%s/api/tasks/%d", srv.URL, id)
	until := time.Now().Add(time.Hour).UTC().Truncate(time.Millisecond)
	snooze := func(deadline time.Time) {
		t.Helper()
		code, body := doJSON(t, "POST", url+"/snooze", map[string]any{"until": deadline.Format(time.RFC3339Nano)})
		if code != http.StatusOK {
			t.Fatalf("snooze: %d %v", code, body)
		}
	}
	for _, target := range []string{"in_progress", "in_review", "done"} {
		recurrentMove(t, srv, id, "in_progress")
		snooze(until)
		recurrentMove(t, srv, id, "todo") // TODO reordering preserves the snooze.
		current, err := scanTask(db.QueryRow(`SELECT `+taskColumns+` FROM tasks WHERE id=?`, id))
		if err != nil || current.SnoozedUntil == nil {
			t.Fatalf("TODO reorder lost snooze: %+v %v", current, err)
		}
		recurrentMove(t, srv, id, target)
		// Simulate a scheduler that selected the task just before the move committed.
		current, err = h.reactivateTask(context.Background(), id, &until)
		if err != nil || current.Status != target || current.SnoozedUntil != nil || current.SnoozedFrom != nil {
			t.Fatalf("stale wake overwrote move: %+v %v", current, err)
		}
	}
	recurrentMove(t, srv, id, "in_review")
	snooze(until.Add(time.Hour))
	current, err := h.reactivateTask(context.Background(), id, &until)
	if err != nil || current.Status != "todo" {
		t.Fatalf("stale wake overwrote new deadline: %+v %v", current, err)
	}
	// Deleting a snoozed task leaves no timer to resurrect it.
	code, _ := doJSON(t, "DELETE", url, nil)
	if code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if err := h.reactivateDue(context.Background(), until.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	taskCount(t, db, pid, 0)
}
