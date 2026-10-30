import { useEffect, useRef } from "react";
import { useLocation } from "react-router";
import { getActiveSession } from "./activeSessionStore";
import { startPresenceBeacon, type Beacon, type HeartbeatBody } from "./beacon";

// The React/DOM glue over the beacon core: mounted once in AppLayout (every
// route, outside <Outlet/>), it beats the current route + focused session +
// visibility to POST /api/presence/heartbeat. Route changes and visibility
// flips poke an immediate beat; the 5s interval covers the steady state.
// No indicator, no API reads — the surface is passive by design.

// A duplicated tab inherits sessionStorage; using it for identity would let
// that tab overwrite the original tab's presence. Each document owns an ID.
let documentClientId: string | undefined;
function ensureClientId(): string {
  return (documentClientId ??= crypto.randomUUID());
}

/** Fire-and-forget transport; keepalive so the final hide beat survives unload. */
function sendBeat(beat: HeartbeatBody): void {
  void fetch("/api/presence/heartbeat", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(beat),
    keepalive: true,
  }).catch(() => {
    // A dropped beat just means the server's 15s prune marks the user
    // away until the next one lands — never worth surfacing.
  });
}

export function usePresenceBeacon(): void {
  const location = useLocation();
  const routeRef = useRef(location.pathname);
  const beaconRef = useRef<Beacon | null>(null);

  // Route change → immediate beat (routeRef is read fresh at beat time).
  useEffect(() => {
    routeRef.current = location.pathname;
    beaconRef.current?.poke();
  }, [location.pathname]);

  useEffect(() => {
    const clientId = ensureClientId();
    const beacon = startPresenceBeacon({
      getClientId: () => clientId,
      getRoute: () => routeRef.current,
      getSessionId: () => getActiveSession() ?? undefined,
      getVisible: () =>
        typeof document === "undefined" ||
        (document.visibilityState === "visible" && document.hasFocus()),
      send: sendBeat,
    });
    beaconRef.current = beacon;

    // Hide → the final visible:false beat (user away instantly);
    // return → an immediate visible beat.
    const onVisibility = () => beacon.poke();
    document.addEventListener("visibilitychange", onVisibility);
    window.addEventListener("pagehide", beacon.leave);
    window.addEventListener("pageshow", onVisibility);
    window.addEventListener("focus", onVisibility);
    window.addEventListener("blur", onVisibility);
    window.addEventListener("kamacu-terminal-view", onVisibility);
    return () => {
      document.removeEventListener("visibilitychange", onVisibility);
      window.removeEventListener("pagehide", beacon.leave);
      window.removeEventListener("pageshow", onVisibility);
      window.removeEventListener("focus", onVisibility);
      window.removeEventListener("blur", onVisibility);
      window.removeEventListener("kamacu-terminal-view", onVisibility);
      beacon.leave();
      beacon.stop();
      beaconRef.current = null;
    };
  }, []);
}
