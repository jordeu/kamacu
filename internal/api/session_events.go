package api

import (
	"errors"
	"io"
	"net/http"
	"time"
)

// events pushes invalidations to all browsers, even when the changed session's
// terminal is not attached. Polling remains a fallback for disconnected clients.
func (h *sessionHandlers) events(w http.ResponseWriter, r *http.Request) {
	_, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	send := func(message string) error {
		// A stalled browser must not keep a handler blocked in Write/Flush.
		if err := controller.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return err
		}
		if _, err := io.WriteString(w, message); err != nil {
			return err
		}
		return controller.Flush()
	}
	changed := h.mgr.Changes()
	// Also refresh on reconnect: transitions while disconnected are coalesced.
	if err := send("data: changed\n\n"); err != nil {
		return
	}
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-changed:
			// Subscribe before sending, so another transition cannot be missed.
			changed = h.mgr.Changes()
			if err := send("data: changed\n\n"); err != nil {
				return
			}
		case <-heartbeat.C:
			if err := send(": heartbeat\n\n"); err != nil {
				return
			}
		}
	}
}
