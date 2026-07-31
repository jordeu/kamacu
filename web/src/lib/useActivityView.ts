import { useEffect, useState } from "react";
import { useActiveWorkspace } from "@/lib/useActiveWorkspace";

/**
 * Phase 11 — the Activity page's persisted scope+window view state.
 *
 * CONTEXT D-05/D-06. This is a PLAIN React hook, NOT a context/provider:
 * scope and window have exactly ONE consumer (ActivityPage). Contrast
 * useActiveWorkspace.tsx, which IS a context because workspace state is read
 * across the sidebar, the board, and the redirect — a context prevents those
 * consumers from desyncing. Activity scope/window has no such cross-tree
 * audience, so a plain hook is the right tool (Phase 11 RESEARCH Assumption A4).
 *
 * The persisted scope is validated against the EXACT parseScope grammar
 * (activity_helpers.go:51-80: `global | workspace:N | project:N`) on read so a
 * tampered/stale localStorage value falls back to default — mirrors
 * useActiveWorkspace.tsx:60-65's stale-fallback pattern.
 */
export const ACTIVITY_SCOPE_KEY = "kamacu.activity.scope";
export const ACTIVITY_WINDOW_KEY = "kamacu.activity.window";

export type ActivityScope = "global" | `workspace:${number}` | `project:${number}`;
export type ActivityWindow = "week" | "month";

/**
 * Read and validate the saved scope against the parseScope grammar.
 * Stale/missing/tampered values fall back to the caller-provided default.
 */
function readScope(fallback: ActivityScope): ActivityScope {
  const raw = localStorage.getItem(ACTIVITY_SCOPE_KEY);
  if (raw === null) {
    return fallback;
  }
  if (
    raw === "global" ||
    /^workspace:\d+$/.test(raw) ||
    /^project:\d+$/.test(raw)
  ) {
    return raw as ActivityScope;
  }
  return fallback;
}

/**
 * Read the saved window. Defaults to "week" (CONTEXT D-06) when absent/invalid.
 */
function readWindow(): ActivityWindow {
  const raw = localStorage.getItem(ACTIVITY_WINDOW_KEY);
  return raw === "month" ? "month" : "week";
}

/**
 * Persisted Activity scope + window view state.
 *
 * Default-scope resolution (CONTEXT D-05): the initial fallback is `"global"`
 * while `useActiveWorkspace()` is still loading. Once the active workspace
 * resolves AND no scope has been persisted yet (first open), the hook adopts
 * `workspace:<id>` as the default — WITHOUT overwriting a user's saved scope
 * (Phase 11 RESEARCH Pitfall 6: check the key is null, not just that
 * activeWorkspaceId resolved).
 */
export function useActivityView() {
  const { activeWorkspaceId } = useActiveWorkspace();
  const [scope, setScopeState] = useState<ActivityScope>(() =>
    readScope("global"),
  );
  const [window, setWindowState] = useState<ActivityWindow>(readWindow);

  // Adopt the active workspace as the default scope on first open (D-05).
  // The `localStorage.getItem(...) === null` guard ensures we never overwrite a
  // user's explicit saved scope (Pitfall 6).
  useEffect(() => {
    if (
      activeWorkspaceId !== null &&
      localStorage.getItem(ACTIVITY_SCOPE_KEY) === null
    ) {
      setScopeState(`workspace:${activeWorkspaceId}`);
    }
  }, [activeWorkspaceId]);

  const setScope = (s: ActivityScope) => {
    setScopeState(s);
    localStorage.setItem(ACTIVITY_SCOPE_KEY, s);
  };

  const setWindow = (w: ActivityWindow) => {
    setWindowState(w);
    localStorage.setItem(ACTIVITY_WINDOW_KEY, w);
  };

  return { scope, setScope, window, setWindow };
}
