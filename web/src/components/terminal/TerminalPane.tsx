import { useCallback, useEffect, useRef, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebglAddon } from "@xterm/addon-webgl";
import "@xterm/xterm/css/xterm.css";
import { Button } from "@/components/ui/button";
import { useStopSession } from "@/api/sessions";
import {
  zincTheme,
  terminalFontFamily,
  terminalFontSize,
} from "./xtermTheme";
import { useTerminalSocket, type ConnState } from "./useTerminalSocket";

export interface TerminalPaneProps {
  sessionId: string;
  label: string; // "bash #3" for the header
  status: "running" | "exited"; // initial REST status (drives Stop button visibility)
  exitCode?: number;
  onClosed?: () => void; // user clicked Close on a terminal banner
  onSessionExit?: () => void; // 'x' frame received — parent invalidates ["sessions"]
}

/**
 * Self-contained terminal pane: header + xterm viewport. Attach-only — the
 * pane NEVER spawns sessions (StrictMode double-mount would double-spawn).
 * No route coupling (D-12): Phase 3 re-mounts this inside TaskTabs unchanged.
 */
export function TerminalPane({
  sessionId,
  label,
  status,
  onSessionExit,
}: TerminalPaneProps) {
  const viewportRef = useRef<HTMLDivElement | null>(null);
  const [term, setTerm] = useState<Terminal | null>(null);
  const [conn, setConn] = useState<ConnState>({ kind: "connecting" });
  const [stopping, setStopping] = useState(false);

  const onSessionExitRef = useRef(onSessionExit);
  onSessionExitRef.current = onSessionExit;

  // Set by the mount effect; lets the conn-state effect force a fit + resize
  // send after every (re)connect (the forced send triggers the server-side
  // SIGWINCH jiggle even when dimensions are unchanged).
  const fitAndSendRef = useRef<(force: boolean) => void>(() => {});

  const handleState = useCallback((s: ConnState) => {
    setConn(s);
    if (s.kind === "exited") onSessionExitRef.current?.();
  }, []);

  const { sendResize } = useTerminalSocket({
    sessionId,
    term,
    onState: handleState,
  });

  // ONE effect keyed on sessionId creates everything; its cleanup destroys
  // everything. StrictMode setup→cleanup→setup leaks nothing.
  useEffect(() => {
    const el = viewportRef.current;
    if (!el) return;

    // Reset per-session UI state on (re)mount.
    setConn({ kind: "connecting" });
    setStopping(false);

    const term = new Terminal({
      fontFamily: terminalFontFamily, // literal stack — CSS var() does NOT resolve in the renderer
      fontSize: terminalFontSize, // 13 (D-19)
      fontWeight: 400,
      fontWeightBold: 700,
      scrollback: 10000, // D-18 (default is only 1000)
      cursorBlink: true,
      theme: zincTheme,
      // do NOT set ignoreBracketedPasteMode — default false lets xterm wrap pastes in
      // \x1b[200~..\x1b[201~ when the app enabled CSI ?2004h; onData carries it through (D-17)
    });
    term.open(el);
    const fit = new FitAddon();
    term.loadAddon(fit);
    try {
      const webgl = new WebglAddon();
      webgl.onContextLoss(() => webgl.dispose()); // dispose → built-in DOM renderer fallback
      term.loadAddon(webgl); // AFTER open()
    } catch {
      /* WebGL unavailable → DOM renderer; visual parity required (UI-SPEC) */
    }

    // Copy-on-select (D-17) — NO copyOnSelect option exists in xterm 6; manual.
    term.onSelectionChange(() => {
      if (term.hasSelection()) {
        void navigator.clipboard.writeText(term.getSelection());
      }
    });

    // Ctrl+Shift+C / Ctrl+Shift+V — best-effort (Pitfall 8); copy-on-select
    // and native Ctrl+V are the guaranteed paths.
    term.attachCustomKeyEventHandler((e) => {
      if (e.type !== "keydown") return true;
      if (e.ctrlKey && e.shiftKey && e.code === "KeyC" && term.hasSelection()) {
        void navigator.clipboard.writeText(term.getSelection());
        e.preventDefault();
        return false;
      }
      if (e.ctrlKey && e.shiftKey && e.code === "KeyV") {
        void navigator.clipboard.readText().then((t) => term.paste(t));
        e.preventDefault();
        return false;
      }
      return true; // everything else (incl. Esc, Ctrl+B) goes to the PTY
    });

    // Resize flow: ResizeObserver → debounce 75ms → hidden-tab guard → fit →
    // send only when cols/rows changed (force=true after (re)connect).
    let lastCols = -1;
    let lastRows = -1;
    let debounceTimer: number | null = null;
    const fitAndSend = (force: boolean) => {
      // Hidden-tab guard ships now for Phase 3 TaskTabs.
      if (el.clientWidth <= 0 || el.clientHeight <= 0) return;
      fit.fit();
      if (force || term.cols !== lastCols || term.rows !== lastRows) {
        lastCols = term.cols;
        lastRows = term.rows;
        sendResize(term.cols, term.rows);
      }
    };
    fitAndSendRef.current = fitAndSend;
    const ro = new ResizeObserver(() => {
      if (debounceTimer !== null) window.clearTimeout(debounceTimer);
      debounceTimer = window.setTimeout(() => fitAndSend(false), 75);
    });
    ro.observe(el);
    // Initial fit; the socket is not open yet so the send no-ops — the
    // post-connect forced send delivers the real first resize.
    fitAndSend(false);

    setTerm(term);

    return () => {
      setTerm(null);
      fitAndSendRef.current = () => {};
      if (debounceTimer !== null) window.clearTimeout(debounceTimer);
      ro.disconnect();
      term.dispose(); // disposes loaded addons (fit, webgl) with it
    };
  }, [sessionId, sendResize]);

  // Connection-state side effects on the terminal.
  useEffect(() => {
    if (!term) return;
    if (conn.kind === "connected") {
      term.options.disableStdin = false;
      fitAndSendRef.current(true); // re-fit + forced resize → server jiggle
    } else if (conn.kind !== "connecting") {
      // reconnecting / lost / exited / not-found: input off (the hook gates
      // too), scrollback stays usable.
      term.options.disableStdin = true;
    }
  }, [conn, term]);

  const stopSession = useStopSession();
  const handleStop = () => {
    setStopping(true); // stays disabled until exit arrives (D-14 grace window)
    stopSession.mutate(sessionId, { onError: () => setStopping(false) });
  };

  const exited = conn.kind === "exited";
  const showStop = status === "running" && !exited && conn.kind !== "not-found";

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden rounded-lg border border-border has-[.xterm-helper-textarea:focus-visible]:ring-2 has-[.xterm-helper-textarea:focus-visible]:ring-blue-500">
      {/* Header bar — 36px: label left, status + Stop right, 1px separator below */}
      <div className="flex h-9 shrink-0 items-center gap-2 border-b border-border px-3">
        <span className="text-sm font-medium">{label}</span>
        <div className="ml-auto flex items-center gap-2">
          {showStop && (
            <Button
              variant="ghost"
              size="sm"
              disabled={stopping}
              onClick={handleStop}
              className="text-red-500 hover:text-red-500"
            >
              {stopping ? "Stopping…" : "Stop"}
            </Button>
          )}
        </div>
      </div>

      {/* Viewport — 8px inset painted in terminal background (never page
          background); the canvas is NEVER dimmed (D-15). Clicking anywhere
          in the viewport focuses xterm. */}
      <div
        className="relative min-h-0 flex-1 bg-[#09090b] p-2"
        onClick={() => term?.focus()}
      >
        <div ref={viewportRef} className="h-full w-full" />
      </div>
    </div>
  );
}
