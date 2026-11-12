import { useState } from "react";
import {
  useNotificationRepairPreview,
  useRepairNotifications,
} from "@/api/notifications";
import { Button } from "@/components/ui/button";

export function NotificationRepair() {
  const preview = useNotificationRepairPreview();
  const repair = useRepairNotifications();
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [queued, setQueued] = useState<number | null>(null);
  return (
    <section
      className="mt-4 rounded border border-border p-3 text-sm"
      aria-label="Recover missed notifications"
    >
      <Button
        size="sm"
        variant="outline"
        disabled={preview.isPending || repair.isPending}
        onClick={() => {
          setSelected(new Set());
          setQueued(null);
          repair.reset();
          preview.mutate();
        }}
      >
        {preview.isPending
          ? "Checking history…"
          : "Check for missed notifications"}
      </Button>
      <p className="mt-2 text-xs text-muted-foreground">
        Find notifications ignored with incomplete evidence in the last 7 days.
        Checking does not process notifications or start work.
      </p>
      {preview.isError && (
        <p role="alert" className="mt-2 text-destructive">
          {preview.error.message}
        </p>
      )}
      {preview.data && queued === null && (
        <div className="mt-3 space-y-2">
          {preview.data.length === 0 ? (
            <p>No notifications need recovery.</p>
          ) : (
            <>
              {preview.data.map((item) => (
                <label
                  key={item.event_id}
                  className="flex items-start gap-2 rounded bg-muted p-2"
                >
                  <input
                    type="checkbox"
                    disabled={repair.isPending}
                    checked={selected.has(item.event_id)}
                    onChange={(event) =>
                      setSelected((old) => {
                        const next = new Set(old);
                        if (event.target.checked) next.add(item.event_id);
                        else next.delete(item.event_id);
                        return next;
                      })
                    }
                    aria-label={`Recover ${item.title}`}
                  />
                  <span>
                    <a
                      href={item.url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="hover:underline"
                    >
                      {item.title}
                    </a>
                    <span className="block text-xs text-muted-foreground">
                      {item.reason}
                    </span>
                  </span>
                </label>
              ))}
              <Button
                size="sm"
                disabled={selected.size === 0 || repair.isPending}
                onClick={() =>
                  repair.mutate(
                    preview.data.filter((item) => selected.has(item.event_id)),
                    {
                      onSuccess: (result) => {
                        setQueued(result.queued);
                        setSelected(new Set());
                      },
                    },
                  )
                }
              >
                {repair.isPending
                  ? "Moving…"
                  : `Move selected to pending (${selected.size})`}
              </Button>
            </>
          )}
        </div>
      )}
      {repair.isError && (
        <p role="alert" className="mt-2 text-destructive">
          {repair.error.message}
        </p>
      )}
      {queued !== null && (
        <p role="status" className="mt-2">
          Moved {queued} notification(s) to pending. Choose Process pending to
          reassess them with the current discussion and existing Inbox history.
          No work has been started.
        </p>
      )}
    </section>
  );
}
