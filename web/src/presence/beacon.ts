// The presence beacon core: the UI's periodic "what am I looking at" beat
// to POST /api/presence/heartbeat. Framework-free on purpose — the timing
// and visibility semantics are the contract, so they live here, injectable
// and testable, while usePresenceBeacon.ts supplies the React/DOM glue.
//
// Semantics:
//   - one beat immediately on start
//   - one beat per intervalMs tick, ONLY while visible (hidden tabs stay
//     silent; the server's 15s prune covers the away transition)
//   - poke() always sends — the glue calls it on route changes and on
//     visibility flips, where a flip to hidden is the final visible:false
//     beat that makes the user away instantly

/** One heartbeat payload — the server's presence.Beat wire shape. */
export interface HeartbeatBody {
  client_id: string;
  route: string;
  project_id?: number;
  task_id?: number;
  session_id?: string;
  visible: boolean;
}

export interface BeaconDeps {
  /** Per-tab identity (sessionStorage-backed in the glue). */
  getClientId: () => string;
  /** Current SPA path, read fresh at every beat. */
  getRoute: () => string;
  /** Focused terminal session, read fresh at every beat. */
  getSessionId: () => string | undefined;
  /** document.visibilityState === "visible", read fresh at every beat. */
  getVisible: () => boolean;
  /** Transport (fire-and-forget fetch in the glue). */
  send: (beat: HeartbeatBody) => void;
  intervalMs?: number;
}

const DEFAULT_INTERVAL_MS = 5000;

export interface Beacon {
  stop: () => void;
  /** Send one beat now, regardless of visibility. */
  poke: () => void;
}

export function startPresenceBeacon(deps: BeaconDeps): Beacon {
  const intervalMs = deps.intervalMs ?? DEFAULT_INTERVAL_MS;

  const build = (): HeartbeatBody => {
    const route = deps.getRoute();
    const ids = parseRouteIds(route);
    const body: HeartbeatBody = {
      client_id: deps.getClientId(),
      route,
      visible: deps.getVisible(),
    };
    if (ids.projectId !== undefined) body.project_id = ids.projectId;
    if (ids.taskId !== undefined) body.task_id = ids.taskId;
    const session = deps.getSessionId();
    if (session) body.session_id = session;
    return body;
  };

  const poke = () => deps.send(build());

  poke();

  const timer = setInterval(() => {
    if (deps.getVisible()) poke();
  }, intervalMs);

  return {
    stop: () => clearInterval(timer),
    poke,
  };
}

/**
 * Parse the SPA path into the route params the snapshot cares about.
 * "/projects/3/tasks/7" -> {projectId: 3, taskId: 7}; "/projects/3" ->
 * {projectId: 3}; anything else (or non-numeric ids) -> {}.
 */
export function parseRouteIds(path: string): {
  projectId?: number;
  taskId?: number;
} {
  const m = /^\/projects\/(\d+)(?:\/tasks\/(\d+))?/.exec(path);
  if (!m) return {};
  const projectId = Number(m[1]);
  if (!Number.isFinite(projectId)) return {};
  const ids: { projectId?: number; taskId?: number } = { projectId };
  if (m[2] !== undefined) {
    const taskId = Number(m[2]);
    if (!Number.isFinite(taskId)) return {};
    ids.taskId = taskId;
  }
  return ids;
}
