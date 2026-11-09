import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { get } from "@/api/client";

type Step = {
  key: string;
  operation_id: string;
  kind: string;
  state: string;
  result: string;
  task_id: number;
  session_id: string;
  arguments: unknown;
};
type Run = { state: string; action: unknown; steps: Step[] };
export function ActionRunDetails({
  id,
  label = "Execution steps",
}: {
  id: string;
  label?: string;
}) {
  const [open, setOpen] = useState(false);
  return (
    <div className="mt-3 text-xs">
      <button className="underline" onClick={() => setOpen(!open)}>
        {label}
      </button>
      {open && <Details id={id} />}
    </div>
  );
}
function Details({ id }: { id: string }) {
  const q = useQuery({
    queryKey: ["notification-action", id],
    queryFn: () =>
      get<Run>(`/api/notifications/actions/${encodeURIComponent(id)}`),
    refetchInterval: (q) =>
      ["queued", "running"].includes(q.state.data?.state ?? "") ? 1500 : false,
  });
  if (q.isPending) return <p>Loading execution steps…</p>;
  if (q.error) return <p role="alert">{q.error.message}</p>;
  return (
    <section
      className="mt-2 space-y-2 rounded border p-3"
      aria-label="Saved Inbox action progress"
    >
      <p>
        Action status: {q.data.state}. Each step is recorded before execution;
        uncertain steps are never automatically resent.
      </p>
      {q.data.steps.map((s) => (
        <details key={s.key} className="rounded bg-muted p-2">
          <summary className="cursor-pointer">
            {s.operation_id} · {s.kind} · {s.state}
            {s.task_id > 0 ? ` · task ${s.task_id}` : ""}
          </summary>
          <pre className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-words">
            {JSON.stringify(
              {
                arguments: s.arguments,
                result: s.result,
                session_id: s.session_id,
              },
              null,
              2,
            )}
          </pre>
        </details>
      ))}
      {q.data.steps.length === 0 && <p>No execution steps recorded.</p>}
      <details>
        <summary className="cursor-pointer">Saved action payload</summary>
        <pre className="mt-2 max-h-80 overflow-auto whitespace-pre-wrap break-words">
          {JSON.stringify(q.data.action, null, 2)}
        </pre>
      </details>
    </section>
  );
}
