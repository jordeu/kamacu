import { QueryClient, QueryClientProvider, QueryObserver } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, expect, it, vi } from "vitest";
import { prDetailKey, usePullRequestDetail, useRefreshPullRequests, useSetPRHidden, type PullRequestsResponse } from "./pullRequests";
import type { ProjectReviewQueue } from "./workspaceOverview";

afterEach(() => vi.unstubAllGlobals());

function Details({
  projectId,
  prNumber,
}: {
  projectId: number;
  prNumber: number;
}) {
  const { data } = usePullRequestDetail(projectId, prNumber, 1288, true);
  return <span>{data?.url ?? "Loading correct PR"}</span>;
}

it("does not reuse PR details cached under another project for the same task", () => {
  const client = new QueryClient();
  client.setQueryData(prDetailKey(1288, 6, 1939), {
    url: "https://github.com/seqeralabs/mcp/pull/1939",
  });
  const html = renderToStaticMarkup(
    <QueryClientProvider client={client}>
      <Details projectId={1} prNumber={1939} />
    </QueryClientProvider>,
  );
  expect(html).toContain("Loading correct PR");
  expect(html).not.toContain("seqeralabs/mcp");
  client.clear();
});

type VisibilityMutations = {
  visibility: ReturnType<typeof useSetPRHidden>;
  refresh: ReturnType<typeof useRefreshPullRequests>;
};

function MutationHarness({ onRender }: { onRender: (value: VisibilityMutations) => void }) {
  const visibility = useSetPRHidden(1);
  const refresh = useRefreshPullRequests(1);
  onRender({ visibility, refresh });
  return null;
}

function visibilityEnv() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const data = {
    state: "ok", stale: false, fetchedAt: "2026-11-25T08:00:00Z",
    prs: [{ number: 7 }, { number: 8 }], reviewed: [{ number: 7 }],
  } as PullRequestsResponse;
  client.setQueryData(["pull-requests", 1], data);
  client.setQueryData(["pull-requests", 2], data);
  client.setQueryData(["workspace-reviews", 1], [
    { projectId: 1, repository: "org/repo", data, failed: false },
    { projectId: 2, repository: "org/repo", data, failed: false },
  ]);
  let mutations!: VisibilityMutations;
  renderToStaticMarkup(<QueryClientProvider client={client}>
    <MutationHarness onRender={(value) => { mutations = value; }} />
  </QueryClientProvider>);
  return { client, ...mutations, data };
}

it("keeps saved visibility when list refetch fails and removes hidden PRs from overview fallback data", async () => {
  const { client, visibility, data } = visibilityEnv();
  const refetch = vi.fn(async () => { throw new Error("offline"); });
  const observer = new QueryObserver(client, {
    queryKey: ["pull-requests", 1], queryFn: refetch, staleTime: Infinity,
  });
  const unsubscribe = observer.subscribe(() => {});
  vi.stubGlobal("fetch", vi.fn(async (_path, init) => Response.json(JSON.parse(init.body))));
  try {
    await visibility.mutateAsync({ number: 7, hidden: true });
    expect(refetch).toHaveBeenCalledOnce();
    const cached = client.getQueryData<PullRequestsResponse>(["pull-requests", 1])!;
    expect(cached.prs?.[0].hidden).toBe(true);
    expect(cached.reviewed?.[0].hidden).toBe(true);
    expect(cached.prs?.[1].hidden).toBeUndefined();
    expect(client.getQueryData(["pull-requests", 2])).toEqual(data);
    const overview = client.getQueryData<ProjectReviewQueue[]>(["workspace-reviews", 1])!;
    expect(overview[0].data?.prs?.map((pr) => pr.number)).toEqual([8]);
    expect(overview[0].data?.reviewed).toEqual([]);
    expect(overview[1].data).toEqual(data);
    await visibility.mutateAsync({ number: 7, hidden: false });
    expect(client.getQueryData<PullRequestsResponse>(["pull-requests", 1])?.prs?.[0].hidden).toBe(false);
  } finally {
    unsubscribe();
    client.clear();
  }
});

it("cancels an older manual refresh so it cannot undo a successful hide", async () => {
  const { client, visibility, refresh, data } = visibilityEnv();
  let completeRefresh!: (response: Response) => void;
  let refreshSignal: AbortSignal | undefined;
  vi.stubGlobal("fetch", vi.fn(async (_path, init) => {
    if (init.method === "PUT") return Response.json({ hidden: true });
    refreshSignal = init.signal;
    // Deliberately complete even after abort to verify query cancellation
    // protects the cache from a late response too.
    return new Promise<Response>((resolve) => { completeRefresh = resolve; });
  }));
  try {
    const pending = refresh.mutateAsync().catch((error: unknown) => error);
    await vi.waitFor(() => expect(completeRefresh).toBeTypeOf("function"));
    await visibility.mutateAsync({ number: 7, hidden: true });
    expect(refreshSignal?.aborted).toBe(true);
    completeRefresh(Response.json(data));
    await pending;
    expect(client.getQueryData<PullRequestsResponse>(["pull-requests", 1])?.prs?.[0].hidden).toBe(true);
  } finally {
    client.clear();
  }
});

it("leaves cached visibility unchanged when saving fails", async () => {
  const { client, visibility, data } = visibilityEnv();
  vi.stubGlobal("fetch", vi.fn(async () => Response.json({ error: "save failed" }, { status: 500 })));
  try {
    await expect(visibility.mutateAsync({ number: 7, hidden: true })).rejects.toThrow("save failed");
    expect(client.getQueryData(["pull-requests", 1])).toEqual(data);
    expect(client.getQueryData<ProjectReviewQueue[]>(["workspace-reviews", 1])?.[0].data).toEqual(data);
  } finally {
    client.clear();
  }
});

it("hides from overview without a cached project board even when overview refresh fails", async () => {
  const { client, visibility, data } = visibilityEnv();
  client.removeQueries({ queryKey: ["pull-requests", 1] });
  const refetch = vi.fn(async () => { throw new Error("offline"); });
  const observer = new QueryObserver(client, {
    queryKey: ["workspace-reviews", 1], queryFn: refetch, staleTime: Infinity,
  });
  const unsubscribe = observer.subscribe(() => {});
  const save = vi.fn(async () => Response.json({ hidden: true }));
  vi.stubGlobal("fetch", save);
  try {
    await visibility.mutateAsync({ number: 7, hidden: true });
    expect(save).toHaveBeenCalledWith("/api/projects/1/pull-requests/7/visibility", expect.objectContaining({
      method: "PUT", body: JSON.stringify({ hidden: true }),
    }));
    expect(refetch).toHaveBeenCalledOnce();
    expect(client.getQueryData(["pull-requests", 1])).toBeUndefined();
    const overview = client.getQueryData<ProjectReviewQueue[]>(["workspace-reviews", 1])!;
    expect(overview[0].data?.prs?.map((pr) => pr.number)).toEqual([8]);
    expect(overview[1].data).toEqual(data);
  } finally {
    unsubscribe();
    client.clear();
  }
});

it("isolates different PR numbers while retaining the matching open-time cache", () => {
  const client = new QueryClient();
  const url = "https://github.com/seqeralabs/sched/pull/1939";
  client.setQueryData(prDetailKey(1288, 1, 1939), { url });
  const render = (prNumber: number) =>
    renderToStaticMarkup(
      <QueryClientProvider client={client}>
        <Details projectId={1} prNumber={prNumber} />
      </QueryClientProvider>,
    );
  expect(render(1939)).toContain(url);
  expect(render(1940)).toContain("Loading correct PR");
  expect(render(1940)).not.toContain(url);
  client.clear();
});
