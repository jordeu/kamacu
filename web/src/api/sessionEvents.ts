import { useEffect } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";

/** One stream per app shell; EventSource reconnects automatically. */
export function subscribeSessionEvents(client: QueryClient): () => void {
  const events = new EventSource("/api/sessions/events");
  let closed = false;
  events.onmessage = () => {
    // invalidateQueries alone reuses an initial request when there is no cached
    // data yet. That response may predate the event, hiding it until the poll.
    const filters = ["sessions", "agent-statuses"].map((key) => ({
      queryKey: [key],
    }));
    void Promise.all(filters.map((filter) => client.cancelQueries(filter))).then(
      () => {
        if (closed) return;
        return Promise.all(
          filters.map((filter) => client.invalidateQueries(filter)),
        );
      },
    );
  };
  return () => {
    closed = true;
    events.close();
  };
}

export function useSessionEvents() {
  const client = useQueryClient();
  useEffect(() => subscribeSessionEvents(client), [client]);
}
