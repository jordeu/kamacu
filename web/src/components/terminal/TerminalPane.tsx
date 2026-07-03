import { useCallback, useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
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
  onNewTerminal?: () => void; // banner "New terminal" — parent spawns; the pane NEVER spawns
  headerActions?: ReactNode; // header right group, BEFORE Stop (D-36 slot)
  headerMenu?: ReactNode; // agent-only ⋯ menu (D-07): when set, SUPERSEDES the Stop button (its own Stop item lives in AgentTab). Bash panes pass nothing → inline Stop unchanged.
  exitedPrimaryLabel?: string; // exited-banner primary action label; default "New terminal"
  showExitedClose?: boolean; // default true; false hides the banner's ghost Close (D-38)
  exitedMessage?: string; // exited-banner copy override; default "Session exited (code {N})" (bash)
  exitedActions?: ReactNode; // replaces the default exited/not-found banner action group (agent resume pair); bash defaults unchanged when undefined
  dimWhenExited?: boolean; // default false (bash unchanged); true dims the terminal area once exited (agent)
  onReady?: (api: { paste: (text: string) => void } | null) => void; // imperative paste handle
  onConnect?: () => void; // fires when conn state becomes "connected" (D-45 hook)
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
  onClosed,
  onSessionExit,
  onNewTerminal,
  headerActions,
  headerMenu,
  exitedPrimaryLabel,
  showExitedClose,
  exitedMessage,
  exitedActions,
  dimWhenExited,
  onReady,
  onConnect,
}: TerminalPaneProps) {
  const viewportRef = useRef<HTMLDivElement | null>(null);
  const [term, setTerm] = useState<Terminal | null>(null);
  const [conn, setConn] = useState<ConnState>({ kind: "connecting" });
  const [showConnecting, setShowConnecting] = useState(false);
  const [stopping, setStopping] = useState(false);

  const onSessionExitRef = useRef(onSessionExit);
  onSessionExitRef.current = onSessionExit;
  // Ref-stabilized like onSessionExit so effect deps never change.
  const onReadyRef = useRef(onReady);
  onReadyRef.current = onReady;
  const onConnectRef = useRef(onConnect);
  onConnectRef.current = onConnect;

  // Set by the mount effect; lets the conn-state effect force a fit + resize
  // send after every (re)connect (the forced send triggers the server-side
  // SIGWINCH jiggle even when dimensions are unchanged).
  const fitAndSendRef = useRef<(force: boolean) => void>(() => {});

  const handleState = useCallback((s: ConnState) => {
    setConn(s);
    if (s.kind === "exited") onSessionExitRef.current?.();
  }, []);

  const { sendResize, retry } = useTerminalSocket({
    sessionId,
    term,
    onState: handleState,
  });

  // First-attach "Connecting…" only renders after a 150ms delay (no flash).
  useEffect(() => {
    if (conn.kind !== "connecting") {
      setShowConnecting(false);
      return;
    }
    const timer = window.setTimeout(() => setShowConnecting(true), 150);
    return () => window.clearTimeout(timer);
  }, [conn.kind]);

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
    onReadyRef.current?.({ paste: (t: string) => term.paste(t) });

    return () => {
      onReadyRef.current?.(null);
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
      onConnectRef.current?.(); // D-45 optimistic waiting-clear hook
    } else if (conn.kind !== "connecting") {
      // reconnecting / lost / exited / not-found: input off (the hook's
      // onData forwarding gates on OPEN+!exited too), scrollback stays usable.
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
  // Agent panes render the dead terminal visually disabled (checkpoint
  // revision); the REST status covers the pre-attach render of an already
  // exited session. Bash panes (dimWhenExited unset) are unaffected.
  const dimTerminal = Boolean(dimWhenExited) && (exited || status === "exited");

  // Header status text (12px muted) — only when not cleanly connected.
  const headerStatus =
    conn.kind === "connecting"
      ? "Connecting…"
      : conn.kind === "reconnecting"
        ? "Reconnecting…"
        : conn.kind === "lost"
          ? "Connection lost"
          : conn.kind === "exited"
            ? "Exited"
            : null;

  // One banner at a time, docked under the header and above the viewport
  // (zinc-900 surface, 1px zinc-800 bottom border). Bash terminals are NEVER
  // dimmed (D-15); agent panes opt into dimWhenExited (checkpoint revision) —
  // exited history stays readable/copyable in both cases.
  let banner: ReactNode = null;
  if (conn.kind === "reconnecting") {
    banner = (
      <div className="flex shrink-0 items-center border-b border-border bg-card px-3 py-2">
        <span className="text-sm text-muted-foreground">
          Connection lost. Reconnecting…
        </span>
      </div>
    );
  } else if (conn.kind === "lost") {
    banner = (
      <div className="flex shrink-0 items-center gap-2 border-b border-border bg-card px-3 py-2">
        <span className="text-sm font-medium">Couldn't reconnect.</span>
        <span className="text-sm text-muted-foreground">
          The session may still be running on the server.
        </span>
        <div className="ml-auto">
          <Button variant="ghost" size="sm" onClick={retry}>
            Retry connection
          </Button>
        </div>
      </div>
    );
  } else if (conn.kind === "not-found") {
    banner = (
      <div className="flex shrink-0 items-center gap-2 border-b border-border bg-card px-3 py-2">
        <span className="text-sm font-medium">Session not found.</span>
        <div className="ml-auto flex items-center gap-2">
          {exitedActions ?? (
            <>
              <Button size="sm" onClick={() => onNewTerminal?.()}>
                {exitedPrimaryLabel ?? "New terminal"}
              </Button>
              {showExitedClose !== false && (
                <Button variant="ghost" size="sm" onClick={() => onClosed?.()}>
                  Close
                </Button>
              )}
            </>
          )}
        </div>
      </div>
    );
  } else if (conn.kind === "exited") {
    banner = (
      <div className="flex shrink-0 items-center gap-2 border-b border-border bg-card px-3 py-2">
        {/* Bash default: render the server's code as-is (no signal special-casing).
            Agent panes override with code-free copy (checkpoint revision). */}
        <span className="text-sm font-medium">
          {exitedMessage ?? `Session exited (code ${conn.code})`}
        </span>
        <div className="ml-auto flex items-center gap-2">
          {exitedActions ?? (
            <>
              <Button size="sm" onClick={() => onNewTerminal?.()}>
                {exitedPrimaryLabel ?? "New terminal"}
              </Button>
              {showExitedClose !== false && (
                <Button variant="ghost" size="sm" onClick={() => onClosed?.()}>
                  Close
                </Button>
              )}
            </>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden rounded-lg border border-border has-[.xterm-helper-textarea:focus-visible]:ring-2 has-[.xterm-helper-textarea:focus-visible]:ring-blue-500">
      {/* Header bar — 36px: label left, status + Stop right, 1px separator below */}
      <div className="flex h-9 shrink-0 items-center gap-2 border-b border-border px-3">
        <span className="text-sm font-medium">{label}</span>
        <div className="ml-auto flex items-center gap-2">
          {headerStatus && (
            <span className="text-xs text-muted-foreground">
              {headerStatus}
            </span>
          )}
          {headerActions}
          {/* D-07: an agent-only headerMenu (⋯) SUPERSEDES the inline Stop
              button — its own Stop item lives in AgentTab. Bash panes pass no
              headerMenu, so their inline Stop stays exactly as-is. */}
          {headerMenu ? (
            headerMenu
          ) : (
            showStop && (
              <Button
                variant="ghost"
                size="sm"
                disabled={stopping}
                onClick={handleStop}
                className="text-red-500 hover:text-red-500"
              >
                {stopping ? "Stopping…" : "Stop"}
              </Button>
            )
          )}
        </div>
      </div>

      {banner}

      {/* Viewport — 8px inset painted in terminal background (never page
          background). Bash canvases are never dimmed (D-15); agent panes dim
          here when exited (banner stays full-contrast). Clicking anywhere in
          the viewport focuses xterm. */}
      <div
        className={`relative min-h-0 flex-1 bg-[#09090b] p-2${
          dimTerminal ? " opacity-50 brightness-75" : ""
        }`}
        onClick={() => term?.focus()}
      >
        <div ref={viewportRef} className="h-full w-full" />
        {conn.kind === "connecting" && showConnecting && (
          <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
            <span className="text-sm text-muted-foreground">Connecting…</span>
          </div>
        )}
      </div>
    </div>
  );
}
