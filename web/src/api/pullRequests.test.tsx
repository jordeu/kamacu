import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { expect, it } from "vitest";
import { prDetailKey, usePullRequestDetail } from "./pullRequests";

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
