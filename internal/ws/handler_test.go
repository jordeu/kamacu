package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"kamacu/internal/session"
)

// newWSServer starts an httptest server routing GET /api/sessions/{id}/ws to
// a Handler over a fresh Manager. originPatterns is nil: coder/websocket
// always authorizes the request's own Host, so same-host test dials succeed
// while a forged evil Origin is rejected without extra patterns.
func newWSServer(t *testing.T) (*httptest.Server, *session.Manager) {
	t.Helper()
	mgr := session.NewManager()
	mux := http.NewServeMux()
	mux.Handle("GET /api/sessions/{id}/ws", NewHandler(mgr, nil, false))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, mgr
}

// spawn starts a bash session and guarantees teardown.
func spawn(t *testing.T, mgr *session.Manager) *session.Session {
	t.Helper()
	sess, err := mgr.Spawn(session.SpawnOpts{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	t.Cleanup(sess.Stop)
	return sess
}

func dial(t *testing.T, ctx context.Context, srv *httptest.Server, id string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.Dial(ctx, srv.URL+"/api/sessions/"+id+"/ws", nil)
	if err != nil {
		t.Fatalf("dial session %s: %v", id, err)
	}
	return conn
}

// sendFrame writes a binary frame with the given 1-byte type prefix.
func sendFrame(t *testing.T, ctx context.Context, conn *websocket.Conn, typ byte, payload string) {
	t.Helper()
	frame := append([]byte{typ}, payload...)
	if err := conn.Write(ctx, websocket.MessageBinary, frame); err != nil {
		t.Fatalf("write %q frame: %v", typ, err)
	}
}

// collectUntil reads data frames, accumulating '0' payloads, until the
// accumulated output contains substr. Markers in tests use the shell
// quote-splitting trick (echo ws-MAR''KER) so the terminal echo of the typed
// command never matches — only real command output does.
func collectUntil(t *testing.T, ctx context.Context, conn *websocket.Conn, substr string) string {
	t.Helper()
	var out strings.Builder
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read while waiting for %q (have %q): %v", substr, out.String(), err)
		}
		if typ != websocket.MessageBinary || len(data) == 0 {
			continue
		}
		if data[0] != FrameData {
			t.Fatalf("unexpected frame type %q while waiting for %q", data[0], substr)
		}
		out.Write(data[1:])
		if strings.Contains(out.String(), substr) {
			return out.String()
		}
	}
}

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// writeFakeClaude writes an executable claude stand-in (spawned via
// AgentConfig.ClaudeBin — WS tests never touch the real binary): bracketed
// paste enable, TERM trap -> exit 143, then idle until stopped.
func writeFakeClaude(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-claude")
	script := `#!/usr/bin/env bash
printf '\x1b[?2004h'
echo "fake claude ready"
trap 'exit 143' TERM
sleep 300 & wait
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude stub: %v", err)
	}
	return path
}

// TestAttachClearsWaitingAgent proves D-45 server-side: attaching a client to
// a WAITING agent session clears it to idle — opening the agent tab
// acknowledges the prompt authoritatively, not just in the client cache.
func TestAttachClearsWaitingAgent(t *testing.T) {
	srv, mgr := newWSServer(t)
	mgr.SetAgentConfig(session.AgentConfig{
		BaseURL:   "http://127.0.0.1:7333",
		Token:     "TOK",
		ClaudeBin: writeFakeClaude(t),
	})
	sess, err := mgr.Spawn(session.SpawnOpts{Kind: session.KindAgent, Cwd: t.TempDir(), TaskID: 1})
	if err != nil {
		t.Fatalf("spawn agent: %v", err)
	}
	t.Cleanup(sess.Stop)

	sess.SetWaiting()
	if got := sess.Info().AgentStatus; got != "waiting" {
		t.Fatalf("pre-attach AgentStatus = %q, want waiting", got)
	}

	ctx := testCtx(t)
	conn := dial(t, ctx, srv, sess.Info().ID)
	defer conn.Close(websocket.StatusNormalClosure, "")

	// The handler clears waiting right after Attach; the dial returning only
	// guarantees the upgrade, so poll briefly.
	deadline := time.Now().Add(10 * time.Second)
	for {
		if got := sess.Info().AgentStatus; got == "idle" {
			break
		} else if time.Now().After(deadline) {
			t.Fatalf("AgentStatus = %q after WS attach, want idle (D-45)", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestAttachBashNoAgentStatus: the attach-clears-waiting hook is a strict
// no-op for bash sessions — they never carry an agent status.
func TestAttachBashNoAgentStatus(t *testing.T) {
	srv, mgr := newWSServer(t)
	sess := spawn(t, mgr)
	ctx := testCtx(t)

	conn := dial(t, ctx, srv, sess.Info().ID)
	defer conn.Close(websocket.StatusNormalClosure, "")

	// A full round trip proves the server-side attach path has completed.
	sendFrame(t, ctx, conn, FrameData, "echo attach-MAR''KER\n")
	collectUntil(t, ctx, conn, "attach-MARKER")

	if got := sess.Info().AgentStatus; got != "" {
		t.Errorf("bash AgentStatus = %q after attach, want empty", got)
	}
}

// TestRoundTrip proves the full byte path: a '0' input frame reaches bash
// and the command's output comes back as '0' frames.
func TestRoundTrip(t *testing.T) {
	srv, mgr := newWSServer(t)
	sess := spawn(t, mgr)
	ctx := testCtx(t)

	conn := dial(t, ctx, srv, sess.Info().ID)
	defer conn.Close(websocket.StatusNormalClosure, "")

	sendFrame(t, ctx, conn, FrameData, "echo ws-MAR''KER\n")
	collectUntil(t, ctx, conn, "ws-MARKER")
}

// TestReplayOnReattach proves TERM-05 over the wire: after disconnecting, a
// new connection's FIRST data frame carries the prior output (the ring
// replay), and the live stream continues after it.
func TestReplayOnReattach(t *testing.T) {
	srv, mgr := newWSServer(t)
	sess := spawn(t, mgr)
	ctx := testCtx(t)
	id := sess.Info().ID

	conn1 := dial(t, ctx, srv, id)
	sendFrame(t, ctx, conn1, FrameData, "echo replay-MAR''KER\n")
	collectUntil(t, ctx, conn1, "replay-MARKER")
	conn1.Close(websocket.StatusNormalClosure, "")

	conn2 := dial(t, ctx, srv, id)
	defer conn2.Close(websocket.StatusNormalClosure, "")

	// The replay is queued as the connection's first item, so the very first
	// data frame must already contain the prior output.
	typ, data, err := conn2.Read(ctx)
	if err != nil {
		t.Fatalf("read first frame on reattach: %v", err)
	}
	if typ != websocket.MessageBinary || len(data) == 0 || data[0] != FrameData {
		t.Fatalf("first frame on reattach: type=%v prefix=%q, want binary '0' frame", typ, data)
	}
	if !strings.Contains(string(data[1:]), "replay-MARKER") {
		t.Fatalf("first data frame does not contain replay; got %q", data[1:])
	}

	// Live stream continues after the replay.
	sendFrame(t, ctx, conn2, FrameData, "echo live-MAR''KER\n")
	collectUntil(t, ctx, conn2, "live-MARKER")
}

// TestResizeChangesPTYSize proves a '1' frame reaches pty.Setsize: bash's
// stty reports the new dimensions.
func TestResizeChangesPTYSize(t *testing.T) {
	srv, mgr := newWSServer(t)
	sess := spawn(t, mgr)
	ctx := testCtx(t)

	conn := dial(t, ctx, srv, sess.Info().ID)
	defer conn.Close(websocket.StatusNormalClosure, "")

	sendFrame(t, ctx, conn, FrameResize, `{"cols":120,"rows":40}`)
	sendFrame(t, ctx, conn, FrameData, "stty size\n")
	out := collectUntil(t, ctx, conn, "40 120")
	if !strings.Contains(out, "40 120") {
		t.Fatalf("stty size did not report 40 120; got %q", out)
	}
}

// TestResizeJiggleOnceOnEqualSize proves the once-per-attach forceRedraw
// contract behaviorally:
//   - the FIRST resize frame after attach with a size equal to the PTY's
//     current size must still deliver SIGWINCH (the server-side jiggle), and
//   - a LATER equal-size resize must NOT (debounce — resize storms must not
//     re-trigger redraws).
//
// SIGWINCH delivery is observed via a WINCH trap inside a foreground
// NON-interactive `bash -c` child: interactive bash reserves SIGWINCH for
// its own line-editing machinery and does not run user WINCH traps promptly,
// but a non-interactive foreground child (the recipient of the PTY's
// SIGWINCH) runs them reliably (verified empirically).
func TestResizeJiggleOnceOnEqualSize(t *testing.T) {
	srv, mgr := newWSServer(t)
	sess := spawn(t, mgr) // PTY starts at 80x24
	ctx := testCtx(t)

	conn := dial(t, ctx, srv, sess.Info().ID)
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Phase 1: first resize frame, equal size → jiggle → SIGWINCH → trap
	// fires. The READY marker is printed AFTER the trap is installed, so once
	// it is observed the SIGWINCH cannot be lost no matter when it lands.
	sendFrame(t, ctx, conn, FrameData, "bash -c 'trap \"echo WIN-MARKER-\"CH WINCH; echo READY-\"1\"; read -t 10; echo DONE-\"1\"'\n")
	collectUntil(t, ctx, conn, "READY-1")
	sendFrame(t, ctx, conn, FrameResize, `{"cols":80,"rows":24}`)
	collectUntil(t, ctx, conn, "WIN-MARKER-CH")
	// Wait for the child to pass its read before typing anything else —
	// otherwise the next input line is swallowed as the read's input.
	collectUntil(t, ctx, conn, "DONE-1")

	// Phase 2: re-arm with a NEW trap message (so stragglers from phase 1
	// can't confuse the assertion), then a SECOND equal-size resize. No
	// SIGWINCH may arrive before the read times out.
	sendFrame(t, ctx, conn, FrameData, "bash -c 'trap \"echo AGAIN-MARKER-\"XX WINCH; echo READY-\"2\"; read -t 2; echo PHASE2-\"EN\"D'\n")
	collectUntil(t, ctx, conn, "READY-2")
	sendFrame(t, ctx, conn, FrameResize, `{"cols":80,"rows":24}`)
	out := collectUntil(t, ctx, conn, "PHASE2-END")
	if strings.Contains(out, "AGAIN-MARKER-XX") {
		t.Fatalf("second equal-size resize delivered SIGWINCH (jiggle not debounced); output: %q", out)
	}
}

// TestExitFrame proves session exit fans out an 'x' frame with the exit
// code, followed by a 1000 close.
func TestExitFrame(t *testing.T) {
	srv, mgr := newWSServer(t)
	sess := spawn(t, mgr)
	ctx := testCtx(t)

	conn := dial(t, ctx, srv, sess.Info().ID)
	defer conn.Close(websocket.StatusNormalClosure, "")

	sendFrame(t, ctx, conn, FrameData, "exit\n")

	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read before exit frame: %v", err)
		}
		if typ != websocket.MessageBinary || len(data) == 0 || data[0] == FrameData {
			continue
		}
		if data[0] != FrameExit {
			t.Fatalf("unexpected frame type %q, want 'x'", data[0])
		}
		var p struct {
			Code int `json:"code"`
		}
		if err := json.Unmarshal(data[1:], &p); err != nil {
			t.Fatalf("decode exit payload %q: %v", data[1:], err)
		}
		if p.Code != 0 {
			t.Fatalf("exit code = %d, want 0", p.Code)
		}
		break
	}

	_, _, err := conn.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
		t.Fatalf("close status = %v (err %v), want 1000", websocket.CloseStatus(err), err)
	}
}

// TestSessionNotFound proves an unknown session id completes the upgrade and
// then closes with the application code 4404.
func TestSessionNotFound(t *testing.T) {
	srv, _ := newWSServer(t)
	ctx := testCtx(t)

	conn := dial(t, ctx, srv, "nonexistent")
	defer conn.Close(websocket.StatusNormalClosure, "")

	_, _, err := conn.Read(ctx)
	if websocket.CloseStatus(err) != CloseSessionNotFound {
		t.Fatalf("close status = %v (err %v), want 4404", websocket.CloseStatus(err), err)
	}
}

// TestEvilOriginRejected proves CSWSH protection: a cross-origin browser
// upgrade never reaches 101.
func TestEvilOriginRejected(t *testing.T) {
	srv, mgr := newWSServer(t)
	sess := spawn(t, mgr)
	ctx := testCtx(t)

	url := fmt.Sprintf("%s/api/sessions/%s/ws", srv.URL, sess.Info().ID)
	conn, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"http://evil.example"}},
	})
	if err == nil {
		conn.Close(websocket.StatusNormalClosure, "")
		t.Fatal("upgrade succeeded with Origin http://evil.example")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("handshake response = %+v, want HTTP 403", resp)
	}
}
