// Package ws bridges the session engine to the network: a ttyd-style binary
// WebSocket protocol with a 1-byte type prefix on every frame. The browser
// client (plan 02-04) mirrors these bytes exactly — keep them stable.
//
// Binary frames ONLY: PTY output is arbitrary bytes, and text frames' UTF-8
// validation corrupts multi-byte sequences split across frames.
package ws

import "github.com/coder/websocket"

// Frame type prefixes (first byte of every binary frame).
const (
	// FrameData carries raw bytes in both directions: client→server stdin
	// (including bracketed-paste wrappers untouched) and server→client PTY
	// output (replay and live stream use the same frame).
	FrameData byte = '0' // 0x30

	// FrameResize is client→server only: JSON {"cols":N,"rows":N}.
	FrameResize byte = '1' // 0x31

	// FrameExit is server→client only: JSON {"code":N}, after which the
	// server closes with StatusNormalClosure (1000).
	FrameExit byte = 'x' // 0x78

	// Bytes '2'/'3' are reserved (ttyd PAUSE/RESUME flow control) and NOT
	// implemented this phase.
)

// CloseSessionNotFound is the application close code sent when the session
// id in the URL is unknown or already closed. 4000-4999 is the RFC 6455
// private-use range; the client sees CloseEvent.code === 4404 and skips its
// reconnect cycle.
const CloseSessionNotFound websocket.StatusCode = 4404
