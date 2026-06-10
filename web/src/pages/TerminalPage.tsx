import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  useDeleteSession,
  useSessions,
  useSpawnSession,
  type TermSession,
} from "@/api/sessions";
import { TerminalPane } from "@/components/terminal/TerminalPane";

/**
 * Dev/debug surface for the terminal engine (D-12). Deliberately exercises
 * the FULL engine: multiple sessions, attach-by-click (D-13), detach/reattach
 * with replay, stop, close. Phase 3 promotes TerminalPane into TaskTabs; the
 * rail and page chrome here are route-level composition only.
 */
export default function TerminalPage() {
  const queryClient = useQueryClient();
  const { data: sessions, isLoading } = useSessions();
  const spawn = useSpawnSession();
  const deleteSession = useDeleteSession();
  const [selectedId, setSelectedId] = useState<string | null>(null);

  // Live sessions above exited, newest first within each group.
  const sorted = [...(sessions ?? [])].sort((a, b) => {
    if (a.status !== b.status) return a.status === "running" ? -1 : 1;
    return b.createdAt.localeCompare(a.createdAt);
  });

  // The freshly spawned session may not be in the list yet (invalidate is
  // in flight) — fall back to the mutation result so the pane attaches
  // immediately on spawn success.
  const selected: TermSession | undefined =
    sorted.find((s) => s.id === selectedId) ??
    (spawn.data?.id === selectedId ? spawn.data : undefined);

  // Spawning happens ONLY from click handlers — never from effects
  // (StrictMode double-mount would double-spawn).
  const handleSpawn = () => {
    spawn.mutate(undefined, {
      onSuccess: (s) => setSelectedId(s.id),
    });
  };

  const handleClose = () => {
    if (!selectedId) return;
    deleteSession.mutate(selectedId, {
      onSuccess: () => setSelectedId(null),
    });
  };

  const hasSessions = sorted.length > 0;

  return (
    <div className="flex h-full flex-col">
      {/* Page header — board page chrome (24px horizontal padding); the New
          terminal button is the ONLY inverted high-contrast element here. */}
      <header className="flex items-center justify-between px-6 py-4">
        <h1 className="text-base font-medium">Terminal</h1>
        <div className="flex items-center gap-3">
          {spawn.isError && (
            <span className="text-xs text-red-500">
              Couldn't start a session. Try again.
            </span>
          )}
          <Button onClick={handleSpawn} disabled={spawn.isPending}>
            New terminal
          </Button>
        </div>
      </header>

      {/* Body — session rail (fixed 220px) + terminal pane filling the rest;
          pane height fills the viewport below the header. */}
      <div className="flex min-h-0 flex-1 gap-4 px-6 pb-6">
        <div className="flex w-[220px] shrink-0 flex-col gap-px overflow-y-auto">
          {isLoading ? (
            <>
              <Skeleton className="h-12 w-full" />
              <Skeleton className="h-12 w-full" />
            </>
          ) : (
            sorted.map((s) => (
              <button
                key={s.id}
                type="button"
                onClick={() => setSelectedId(s.id)}
                className={`flex flex-col items-start rounded-md px-3 py-2 text-left ${
                  s.id === selectedId
                    ? "bg-[#27272a] text-zinc-50"
                    : "hover:bg-[#18181b]"
                }`}
              >
                <span
                  className={`text-sm font-medium ${
                    s.status === "exited" ? "text-muted-foreground" : ""
                  }`}
                >
                  {s.label}
                </span>
                <span className="text-xs text-muted-foreground">
                  {s.status === "running"
                    ? "running"
                    : `exited (code ${s.exitCode ?? 0})`}
                </span>
              </button>
            ))
          )}
        </div>

        <div className="min-h-0 min-w-0 flex-1">
          {selected ? (
            // key forces a clean remount per session — switching rows
            // detaches the previous WS and reattaches with replay (D-13).
            <TerminalPane
              key={selected.id}
              sessionId={selected.id}
              label={selected.label}
              status={selected.status}
              exitCode={selected.exitCode}
              onSessionExit={() =>
                queryClient.invalidateQueries({ queryKey: ["sessions"] })
              }
              onClosed={handleClose}
              onNewTerminal={handleSpawn}
            />
          ) : !isLoading && !hasSessions ? (
            <div className="flex h-full items-center justify-center">
              <div className="flex flex-col items-center gap-4 py-8 text-center">
                <div className="flex flex-col items-center gap-1">
                  <h2 className="text-xl font-medium">No sessions</h2>
                  <p className="text-sm text-muted-foreground">
                    Start a shell that keeps running even when you close this tab.
                  </p>
                </div>
                <Button onClick={handleSpawn} disabled={spawn.isPending}>
                  New terminal
                </Button>
              </div>
            </div>
          ) : !isLoading ? (
            <div className="flex h-full items-center justify-center py-8">
              <p className="text-sm text-muted-foreground">
                Select a session to attach.
              </p>
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}
