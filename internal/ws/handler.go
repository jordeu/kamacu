package ws

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"kangent/internal/session"
)

// Handler upgrades GET /api/sessions/{id}/ws requests and bridges the
// connection to a session. It consumes ONLY the session package's public
// surface (Attach/Detach/WriteInput/Resize/Done/Info) — never the ring
// directly — which is the Phase 4 seam.
type Handler struct {
	mgr            *session.Manager
	originPatterns []string
}

// NewHandler returns a Handler serving sessions from mgr. originPatterns is
// the Origin allowlist passed to websocket.Accept (the request's own Host is
// always authorized, and requests without an Origin header — non-browser
// clients — are allowed; browsers always send Origin on WS handshakes).
func NewHandler(mgr *session.Manager, originPatterns []string) *Handler {
	return &Handler{mgr: mgr, originPatterns: originPatterns}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, found := h.mgr.Get(id)

	// Accept FIRST in all cases: a WS close code (4404) can only be
	// delivered after the upgrade completes. Origin verification stays ON
	// (the library default) — Origin is matched against originPatterns.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: h.originPatterns,
	})
	if err != nil {
		return // Accept already wrote the HTTP error (e.g. 403 on bad Origin)
	}
	if !found {
		_ = conn.Close(CloseSessionNotFound, "session not found")
		return
	}

	// The library default read limit is 32 KiB; a large bracketed paste
	// would kill the conn with StatusMessageTooBig (Pitfall 2).
	conn.SetReadLimit(1 << 20)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	connID := uuid.NewString()
	q := sess.Attach(connID)
	defer sess.Detach(connID) // Detach NEVER touches the PTY — that IS TERM-05

	writeDone := make(chan struct{})
	go h.writeLoop(ctx, conn, sess, q, writeDone)

	h.readLoop(ctx, conn, sess)

	// Reader is done (client closed, or we closed after exit). Stop the
	// writer and wait for it so its final exit-frame write is never cut off
	// by the request context ending.
	cancel()
	<-writeDone
}

// writeLoop is the ONLY writer goroutine for this conn — that guarantees
// output ordering (coder/websocket permits concurrent writes, but ordering
// would be lost). It drains the attach queue into '0' frames; a closed queue
// means either session exit (deliver 'x' + 1000) or a slow-consumer drop by
// the pump (close so the client reconnects with a fresh replay).
func (h *Handler) writeLoop(ctx context.Context, conn *websocket.Conn, sess *session.Session, q <-chan []byte, done chan<- struct{}) {
	defer close(done)
	for {
		select {
		case chunk, ok := <-q:
			if !ok {
				h.finish(ctx, conn, sess)
				return
			}
			frame := append([]byte{FrameData}, chunk...)
			if err := conn.Write(ctx, websocket.MessageBinary, frame); err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// finish handles a closed attach queue. Buffered output was already drained
// by the channel before it reported closed, so the 'x' frame always follows
// the session's final output.
func (h *Handler) finish(ctx context.Context, conn *websocket.Conn, sess *session.Session) {
	if sess.Info().Status != session.StatusExited {
		// The pump dropped this conn as a slow consumer. Close so the client
		// reconnects and gets a fresh replay (the cheap localhost answer to
		// backpressure, Pitfall 7).
		_ = conn.Close(websocket.StatusTryAgainLater, "output overflow")
		return
	}
	// Session exited. ExitCode is documented final once Done() fires; the
	// queue closes in markExited just before that, so this wait is short.
	select {
	case <-sess.Done():
	case <-ctx.Done():
		return
	}
	code := 0
	if c := sess.Info().ExitCode; c != nil {
		code = *c
	}
	payload, err := json.Marshal(struct {
		Code int `json:"code"`
	}{code})
	if err != nil {
		return
	}
	frame := append([]byte{FrameExit}, payload...)
	if err := conn.Write(ctx, websocket.MessageBinary, frame); err != nil {
		return
	}
	_ = conn.Close(websocket.StatusNormalClosure, "")
}

// readLoop serializes all reads for the conn (coder/websocket requires
// serialized reads) and dispatches on the 1-byte type prefix.
//
// forceRedraw is passed to Resize exactly ONCE per attach — on the first
// resize frame — so a reattaching client whose fitted size equals the PTY's
// current size still triggers the SIGWINCH jiggle, while resize storms never
// re-trigger redraws (claude-code resize-storm duplication bug).
func (h *Handler) readLoop(ctx context.Context, conn *websocket.Conn, sess *session.Session) {
	firstResize := true
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if typ != websocket.MessageBinary || len(data) == 0 {
			continue
		}
		switch data[0] {
		case FrameData:
			// Errors only mean the session exited; the writer delivers the
			// 'x' frame and close — nothing to do here.
			_ = sess.WriteInput(data[1:])
		case FrameResize:
			var sz struct {
				Cols uint16 `json:"cols"`
				Rows uint16 `json:"rows"`
			}
			if err := json.Unmarshal(data[1:], &sz); err != nil {
				continue // malformed resize — ignore
			}
			_ = sess.Resize(sz.Cols, sz.Rows, firstResize)
			firstResize = false
		default:
			// '2'/'3' reserved (ttyd PAUSE/RESUME) — ignore unknown types.
		}
	}
}
