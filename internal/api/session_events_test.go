package api

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"kamacu/internal/session"
)

func TestSessionEventsStartExitAndReconnect(t *testing.T) {
	srv, mgr := newSessionServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connect := func() (*http.Response, *bufio.Reader) {
		t.Helper()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/sessions/events", nil)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { res.Body.Close() })
		if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/event-stream" {
			t.Fatalf("unexpected stream response: %s %v", res.Status, res.Header)
		}
		return res, bufio.NewReader(res.Body)
	}
	read := func(reader *bufio.Reader) {
		t.Helper()
		for _, want := range []string{"data: changed\n", "\n"} {
			got, err := reader.ReadString('\n')
			if err != nil || got != want {
				t.Fatalf("event = %q, %v; want %q", got, err, want)
			}
		}
	}
	res, reader := connect()
	read(reader) // initial synchronization
	s, err := mgr.Spawn(session.SpawnOpts{})
	if err != nil {
		t.Fatal(err)
	}
	read(reader)
	exitSession(t, s)
	read(reader)
	res.Body.Close()
	_, reader = connect()
	read(reader) // a reconnect always refreshes missed state
}

type failedEventStream struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (w *failedEventStream) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	return nil
}

func (w *failedEventStream) FlushError() error { return io.ErrClosedPipe }

func TestSessionEventsReturnsOnFlushFailure(t *testing.T) {
	h := &sessionHandlers{mgr: session.NewManager()}
	w := &failedEventStream{ResponseRecorder: httptest.NewRecorder()}
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.events(w, httptest.NewRequest(http.MethodGet, "/api/sessions/events", nil))
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream handler ignored flush failure")
	}
	if w.deadline.IsZero() {
		t.Fatal("stream writes need a deadline")
	}
}
