package ws

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kamacu/internal/session"
)

func TestTerminalReplayRestoresBracketedPasteAfterWrap(t *testing.T) {
	for _, tc := range []struct {
		name, beforeOutput, afterOutput string
		wantPrefix                      bool
	}{
		{name: "evicted enable", beforeOutput: "\\033[?2004h", wantPrefix: true},
		{name: "evicted disable", beforeOutput: "\\033[?2004h\\033[?2004l"},
		{name: "retained enable", afterOutput: "\\033[?2004h"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, mgr := newWSServer(t)
			script := "#!/bin/sh\nprintf '" + tc.beforeOutput + "'\n" +
				"head -c 1100000 /dev/zero | tr '\\000' A\n" +
				"printf '" + tc.afterOutput + "\\nREPLAY-READY'\nread -r line\n"
			shell := filepath.Join(t.TempDir(), "terminal")
			if err := os.WriteFile(shell, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			sess, err := mgr.Spawn(session.SpawnOpts{Shell: shell})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(sess.Stop)
			ctx := testCtx(t)
			const enable = "\x1b[?2004h"
			var snapshot []byte
			for {
				snapshot = sess.Snapshot()
				if bytes.Contains(snapshot, []byte("REPLAY-READY")) {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal("terminal replay never filled")
				case <-time.After(10 * time.Millisecond):
				}
			}
			if len(snapshot) != 1<<20 || bytes.Contains(snapshot, []byte(enable)) != (tc.afterOutput != "") {
				t.Fatal("unexpected mode sequence in raw replay")
			}
			raw := sess.Attach("raw-output")
			if replay := <-raw; !bytes.Equal(replay, snapshot) {
				t.Fatal("raw output subscriber received a modified replay")
			}
			sess.Detach("raw-output")

			conn := dial(t, ctx, srv, sess.Info().ID)
			defer conn.CloseNow()
			conn.SetReadLimit(2 << 20)
			replay := collectUntil(t, ctx, conn, "REPLAY-READY")
			want := string(snapshot)
			if tc.wantPrefix {
				want = enable + want
			}
			if replay != want {
				t.Fatalf("interactive replay differs from expected output (got %d bytes, want %d)", len(replay), len(want))
			}
		})
	}
}
