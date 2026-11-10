import type { QueryClient } from "@tanstack/react-query";

export interface PendingItem {
  task_id: number;
  project_id: number;
  title: string;
  project: string;
  state: string;
  detail?: string;
}

export interface PendingJob { id: string; state: string; items: PendingItem[] }

// A progress request may have started while a mutation was in flight. Cancel
// that request before publishing the accepted result, so an old run cannot
// overwrite the new run or restore a cancellation's previous state.
export async function publishPendingJob(client: QueryClient, job: PendingJob) {
  await client.cancelQueries({ queryKey: ["pending-check"] });
  client.setQueryData(["pending-check"], job);
}
