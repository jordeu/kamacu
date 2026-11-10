import { QueryClient, QueryObserver } from "@tanstack/react-query";
import { expect, it } from "vitest";
import { publishPendingJob, type PendingJob } from "./pending";

it.each([
  { previous: { id: "old", state: "completed", items: [] }, accepted: { id: "new", state: "running", items: [] } },
  { previous: { id: "run", state: "running", items: [] }, accepted: { id: "run", state: "cancelling", items: [] } },
])("keeps the accepted $accepted.state result when an earlier progress poll returns late", async ({ previous, accepted }) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  let resolvePoll!: (job: PendingJob) => void;
  const pendingPoll = new Promise<PendingJob>((resolve) => { resolvePoll = resolve; });
  const observer = new QueryObserver(client, { queryKey: ["pending-check"], queryFn: () => pendingPoll });
  const stop = observer.subscribe(() => {});
  try {
    expect(observer.getCurrentResult().isFetching).toBe(true);
    // This request can start during the POST, after onMutate cancelled the
    // previous poll. Publishing must also invalidate this later request.
    await publishPendingJob(client, accepted);
    resolvePoll(previous);
    await pendingPoll;
    expect(client.getQueryData(["pending-check"])).toEqual(accepted);
  } finally {
    stop();
    client.clear();
  }
});
