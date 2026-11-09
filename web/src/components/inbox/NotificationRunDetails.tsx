import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { get } from "@/api/client";
import { Button } from "@/components/ui/button";
export function NotificationRunDetails({ id }: { id: string }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="mt-3 text-xs">
      <button className="underline" onClick={() => setOpen(!open)}>
        Processing details
      </button>
      {open && <Details id={id} />}
    </div>
  );
}
function Details({ id }: { id: string }) {
  const [showInput, setShowInput] = useState(false);
  const q = useQuery({
    queryKey: ["notification-run", id],
    queryFn: () =>
      get<{
        status: string;
        config: {
          profile?: { name: string; model: string };
          settings?: { revision: number };
        };
        snapshot: unknown;
        result: unknown;
        reads: unknown[];
      }>(`/api/notifications/batches/${id}`),
    refetchInterval: (q) =>
      ["queued", "running"].includes(q.state.data?.status ?? "") ? 5000 : false,
  });
  const input = useQuery({
    queryKey: ["notification-run-input", id],
    queryFn: () =>
      get<{ snapshot: unknown }>(`/api/notifications/batches/${id}?input=1`),
    enabled: showInput,
  });
  if (q.isPending) return <p>Loading processing details…</p>;
  if (q.error) return <p role="alert">{q.error.message}</p>;
  return (
    <div className="mt-2 space-y-2 rounded border p-3">
      <p>
        Model: {q.data.config.profile?.name ?? "Historical run"}{" "}
        {q.data.config.profile?.model}
      </p>
      <p>
        {q.data.reads.length} read-only lookups. Rule IDs and source references
        are recorded with each decision.
      </p>
      <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-words">
        {JSON.stringify(q.data.result, null, 2)}
      </pre>
      <details>
        <summary>Read-only lookup trace</summary>
        <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-words">
          {JSON.stringify(q.data.reads, null, 2)}
        </pre>
      </details>
      <Button
        size="sm"
        variant="outline"
        onClick={() => setShowInput(!showInput)}
      >
        {showInput ? "Hide" : "Show"} frozen input
      </Button>
      {showInput && input.isPending && <p>Loading frozen input…</p>}
      {input.error && <p role="alert">{input.error.message}</p>}
      {showInput && input.data && (
        <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-words">
          {JSON.stringify(input.data.snapshot, null, 2)}
        </pre>
      )}
    </div>
  );
}
