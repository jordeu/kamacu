import { useCallback, useEffect, useRef } from "react";
import type { Terminal } from "@xterm/xterm";

/**
 * Connection state machine for a terminal WebSocket (UI-SPEC contract).
 */
export type ConnState =
  | { kind: "connecting" }
  | { kind: "connected" }
  | { kind: "reconnecting"; attempt: number }
  | { kind: "lost" } // retries exhausted
  | { kind: "exited"; code: number } // 'x' frame received
  | { kind: "not-found" }; // close code 4404

// Wire protocol (server side fixed in 02-03 — bytes are locked):
//   client→server: 0x30 + raw stdin bytes | 0x31 + JSON {"cols":N,"rows":N}
//   server→client: 0x30 + PTY bytes (replay and live identical)
//                  0x78 + JSON {"code":N}, then clean close 1000
//   close code 4404 = session not found → skip retries
const FRAME_STDIN = 0x30;
const FRAME_RESIZE = 0x31;
const FRAME_OUTPUT = 0x30;
const FRAME_EXIT = 0x78;

// Exponential backoff for transient drops: 5 attempts (UI-SPEC, locked).
const BACKOFF_MS = [500, 1000, 2000, 4000, 8000];

const encoder = new TextEncoder();
const decoder = new TextDecoder();

/**
 * Owns the SOCKET lifecycle only. The Terminal instance is owned by
 * TerminalPane and passed in — it must survive reconnects so scrollback
 * stays usable.
 */
export function useTerminalSocket(opts: {
  sessionId: string;
  term: Terminal | null; // attach IO once non-null
  onState: (s: ConnState) => void;
}): { sendResize: (cols: number, rows: number) => void; retry: () => void } {
  const { sessionId, term } = opts;

  // Latest callback without re-running the effect.
  const onStateRef = useRef(opts.onState);
  onStateRef.current = opts.onState;

  // Imperative escape hatches into the current effect instance.
  const sendResizeRef = useRef<(cols: number, rows: number) => void>(() => {});
  const retryRef = useRef<() => void>(() => {});

  useEffect(() => {
    if (!term) return;

    // Per-effect lifecycle flags. StrictMode runs setup→cleanup→setup;
    // exhaustive cleanup makes the double-mount invisible to the server
    // (it just sees attach→detach→attach).
    let disposed = false;
    let exited = false;
    let hasConnected = false; // distinguishes first attach from re-connects
    let attempt = 0;
    let ws: WebSocket | null = null;
    let reconnectTimer: number | null = null;

    const emit = (s: ConnState) => {
      if (!disposed) onStateRef.current(s);
    };

    const scheduleReconnect = () => {
      if (attempt >= BACKOFF_MS.length) {
        emit({ kind: "lost" }); // 5 failures → retries exhausted
        return;
      }
      const delay = BACKOFF_MS[attempt];
      attempt += 1;
      emit({ kind: "reconnecting", attempt });
      reconnectTimer = window.setTimeout(connect, delay);
    };

    const connect = () => {
      if (disposed) return;
      const proto = location.protocol === "https:" ? "wss" : "ws";
      ws = new WebSocket(
        `${proto}://${location.host}/api/sessions/${sessionId}/ws`,
      );
      ws.binaryType = "arraybuffer";

      ws.onopen = () => {
        if (disposed) return;
        if (hasConnected) {
          // RE-connect: wipe the stale screen BEFORE any replay frame is
          // processed; replay then arrives as ordinary 0x30 frames.
          term.reset();
        }
        hasConnected = true;
        attempt = 0;
        emit({ kind: "connected" });
        // The pane re-fits and sends a resize frame on `connected`, which
        // triggers the server-side SIGWINCH jiggle for TUI repaint.
      };

      ws.onmessage = (ev) => {
        const data = new Uint8Array(ev.data as ArrayBuffer);
        if (data[0] === FRAME_OUTPUT) {
          term.write(data.subarray(1));
        } else if (data[0] === FRAME_EXIT) {
          const { code } = JSON.parse(
            decoder.decode(data.subarray(1)),
          ) as { code: number };
          exited = true;
          emit({ kind: "exited", code });
          // Server follows with a clean close 1000 — onclose stays exited.
        }
      };

      ws.onclose = (ev) => {
        if (disposed) return; // unmount/session switch → do nothing
        if (exited) return; // post-'x' close 1000 → stay exited
        if (ev.code === 4404) {
          emit({ kind: "not-found" }); // session gone — NO retries
          return;
        }
        scheduleReconnect(); // transient drop
      };
    };

    // stdin: 0x30-framed; gated off while not OPEN or after exit.
    // Bracketed-paste wrappers (\x1b[200~..\x1b[201~) arrive inside onData
    // strings and pass through UNTOUCHED (D-17).
    const dataDisposable = term.onData((s) => {
      if (!ws || ws.readyState !== WebSocket.OPEN || exited) return;
      const bytes = encoder.encode(s);
      const frame = new Uint8Array(bytes.length + 1);
      frame[0] = FRAME_STDIN;
      frame.set(bytes, 1);
      ws.send(frame);
    });

    sendResizeRef.current = (cols: number, rows: number) => {
      if (!ws || ws.readyState !== WebSocket.OPEN) return;
      const bytes = encoder.encode(JSON.stringify({ cols, rows }));
      const frame = new Uint8Array(bytes.length + 1);
      frame[0] = FRAME_RESIZE;
      frame.set(bytes, 1);
      ws.send(frame);
    };

    retryRef.current = () => {
      // UI-SPEC "Retry connection": restart the cycle from attempt 0.
      if (disposed || exited) return;
      if (reconnectTimer !== null) {
        window.clearTimeout(reconnectTimer);
        reconnectTimer = null;
      }
      attempt = 0;
      emit({ kind: "reconnecting", attempt: 0 });
      connect();
    };

    // First connection: emit `connecting` immediately; the >150ms display
    // delay is the pane's concern, not the hook's.
    emit({ kind: "connecting" });
    connect();

    return () => {
      disposed = true;
      if (reconnectTimer !== null) window.clearTimeout(reconnectTimer);
      dataDisposable.dispose();
      ws?.close(); // safe even in CONNECTING state
      sendResizeRef.current = () => {};
      retryRef.current = () => {};
    };
  }, [sessionId, term]);

  const sendResize = useCallback((cols: number, rows: number) => {
    sendResizeRef.current(cols, rows);
  }, []);
  const retry = useCallback(() => {
    retryRef.current();
  }, []);

  return { sendResize, retry };
}
