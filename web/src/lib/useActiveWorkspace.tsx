import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { useWorkspaces } from "@/api/queries";

/**
 * localStorage key for the active workspace (WSNAV-03). Follows the `kamacu.*`
 * convention (see lib/migrateStorage.ts) alongside `kamacu.sidebar` etc.
 */
export const ACTIVE_WORKSPACE_KEY = "kamacu.workspace";

interface ActiveWorkspaceContextValue {
  /**
   * The RESOLVED active workspace id: the saved id when it still matches a live
   * workspace row, otherwise the `is_default` workspace's id. `null` only while
   * `useWorkspaces()` is still loading (no rows to resolve against yet).
   */
  activeWorkspaceId: number | null;
  /** Persist and switch the active workspace; re-renders every consumer. */
  setActiveWorkspaceId: (id: number) => void;
}

const ActiveWorkspaceContext =
  createContext<ActiveWorkspaceContextValue | null>(null);

/** Parse the saved id from localStorage to a number, or null when absent/invalid. */
function readSavedId(): number | null {
  const raw = localStorage.getItem(ACTIVE_WORKSPACE_KEY);
  if (raw === null || raw === "") return null;
  const n = Number(raw);
  return Number.isFinite(n) ? n : null;
}

/**
 * Single shared, reactive source of the active workspace (Phase 26 data layer).
 *
 * It MUST be a context — not a per-component localStorage `useState` — so that
 * picking a workspace in the switcher re-renders the sidebar project filter, the
 * board redirect, and every other consumer in one shot. Independent copies would
 * desync (the switcher and the filtered list would disagree).
 *
 * The active id is resolved against the live `useWorkspaces()` rows: a stale or
 * forged saved id (T-26-07) that no longer matches any workspace falls back to
 * the `is_default` workspace — keyed off the `is_default` flag, never the name
 * "Personal", so it survives a rename (D-14). Since this is a single-user local
 * app, the worst a tampered id can do is surface the default's projects.
 */
export function ActiveWorkspaceProvider({ children }: { children: ReactNode }) {
  const { data: workspaces } = useWorkspaces();
  const [savedId, setSavedId] = useState<number | null>(readSavedId);

  const activeWorkspaceId = useMemo<number | null>(() => {
    // Still loading — nothing to resolve the saved id against yet.
    if (!workspaces) return null;
    // Saved id still points at a live workspace → honor it.
    if (savedId !== null && workspaces.some((w) => w.id === savedId)) {
      return savedId;
    }
    // Stale/missing id → fall back to the protected default workspace.
    return workspaces.find((w) => w.is_default)?.id ?? null;
  }, [workspaces, savedId]);

  const setActiveWorkspaceId = useCallback((id: number) => {
    setSavedId(id);
    localStorage.setItem(ACTIVE_WORKSPACE_KEY, String(id));
  }, []);

  const value = useMemo<ActiveWorkspaceContextValue>(
    () => ({ activeWorkspaceId, setActiveWorkspaceId }),
    [activeWorkspaceId, setActiveWorkspaceId],
  );

  return (
    <ActiveWorkspaceContext.Provider value={value}>
      {children}
    </ActiveWorkspaceContext.Provider>
  );
}

/**
 * Read the shared active-workspace state. Throws when used outside
 * <ActiveWorkspaceProvider> so a missing provider surfaces loudly.
 *
 * Co-located with the provider (the same one-file provider+hook convention as
 * ui/sidebar.tsx's SidebarProvider/useSidebar) — the plan requires both exports
 * from this module. The fast-refresh lint rule only cares about HMR granularity,
 * not correctness, so the hook export is suppressed here.
 */
// eslint-disable-next-line react-refresh/only-export-components
export function useActiveWorkspace(): ActiveWorkspaceContextValue {
  const ctx = useContext(ActiveWorkspaceContext);
  if (ctx === null) {
    throw new Error(
      "useActiveWorkspace must be used within an ActiveWorkspaceProvider.",
    );
  }
  return ctx;
}
