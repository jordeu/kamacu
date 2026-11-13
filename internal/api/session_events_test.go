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

// Inbox actions run without an attached terminal. After an idle interval they
// must still reach every browser over the original stream, without waiting for
// the heartbeat/reconnect or the five-second status poll.
func TestSessionEventsInboxLifecycleAfterIdle(t *testing.T) {
	mgr := session.NewManager()
	h := &sessionHandlers{mgr: mgr}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(h.events))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/sessions/events", nil)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.ProtoMajor != 2 {
		t.Fatalf("expected HTTP/2 stream, got %s", res.Proto)
	}
	reader := bufio.NewReader(res.Body)
	readEvent := func() {
		t.Helper()
		for _, want := range []string{"data: changed\n", "\n"} {
			got, err := reader.ReadString('\n')
			if err != nil || got != want {
				t.Fatalf("event after idle = %q, %v; want %q", got, err, want)
			}
		}
	}
	readEvent()
	// Cross the former write deadline while staying before the heartbeat.
	timer := time.NewTimer(11 * time.Second)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	sess, err := mgr.Spawn(session.SpawnOpts{
		Kind: session.KindAgent, TaskID: 123, Cwd: t.TempDir(),
		AgentEngine: "custom", AgentArgs: []string{"/bin/bash", "-c", "read -r line"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Stop()
	readEvent()
	runtime := &inboxActionRuntime{mgr: mgr}
	expected, err := runtime.Session(ctx, sess.Info().ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Stop(ctx, expected); err != nil {
		t.Fatal(err)
	}
	readEvent()
	if sess.Info().Status != session.StatusExited {
		t.Fatal("inbox stop event preceded session exit")
	}
}
