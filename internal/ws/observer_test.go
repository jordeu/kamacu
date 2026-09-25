package ws

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/coder/websocket"

	"kamacu/internal/session"
)

// fakeInputObserver records NoteUserInput calls for assertion.
type fakeInputObserver struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeInputObserver) NoteUserInput(sessionID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, sessionID)
}

func (f *fakeInputObserver) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// newObserverWSServer is newWSServer with an InputObserver wired in.
func newObserverWSServer(t *testing.T, obs InputObserver) (*httptest.Server, *session.Manager) {
	t.Helper()
	mgr := session.NewManager()
	mux := http.NewServeMux()
	mux.Handle("GET /api/sessions/{id}/ws", NewHandler(mgr, nil, false, obs))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, mgr
}

// TestFrameDataNotifiesObserver: every '0' (stdin) frame a browser sends is
// reported to the InputObserver with the session's id — the presence
// typing signal. '1' (resize) frames must NOT be reported.
func TestFrameDataNotifiesObserver(t *testing.T) {
	obs := &fakeInputObserver{}
	srv, mgr := newObserverWSServer(t, obs)
	sess := spawn(t, mgr)
	ctx := testCtx(t)

	conn := dial(t, ctx, srv, sess.Info().ID)
	defer conn.Close(websocket.StatusNormalClosure, "")

	sendFrame(t, ctx, conn, FrameData, "echo obs-MAR''KER\n")
	collectUntil(t, ctx, conn, "obs-MARKER")
	sendFrame(t, ctx, conn, FrameResize, `{"cols":100,"rows":30}`)
	// Round-trip proof the resize was processed before asserting.
	sendFrame(t, ctx, conn, FrameData, "echo after-MAR''KER\n")
	collectUntil(t, ctx, conn, "after-MARKER")

	got := obs.snapshot()
	if len(got) != 2 {
		t.Fatalf("observer calls = %v, want exactly the 2 stdin frames (resize excluded)", got)
	}
	for _, id := range got {
		if id != sess.Info().ID {
			t.Errorf("observer session id = %q, want %q", id, sess.Info().ID)
		}
	}
}

// TestNilObserverNoop: a nil observer (routes wired without presence) must
// never panic — the observer is an optional seam.
func TestNilObserverNoop(t *testing.T) {
	srv, mgr := newObserverWSServer(t, nil)
	sess := spawn(t, mgr)
	ctx := testCtx(t)

	conn := dial(t, ctx, srv, sess.Info().ID)
	defer conn.Close(websocket.StatusNormalClosure, "")

	sendFrame(t, ctx, conn, FrameData, "echo nil-MAR''KER\n")
	collectUntil(t, ctx, conn, "nil-MARKER")
}
