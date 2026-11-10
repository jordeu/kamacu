import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, it, vi } from "vitest";
import type { PendingJob } from "@/api/pending";
import { PendingDialog } from "./PendingDialog";

vi.mock("@/api/queries", () => ({ useProjects: () => ({ data: [] }), useWorkspaces: () => ({ data: [] }) }));
vi.mock("@/lib/useActiveWorkspace", () => ({ useActiveWorkspace: () => ({ activeWorkspaceId: 1 }) }));

function minimized(job: PendingJob) {
  const client = new QueryClient();
  client.setQueryData(["pending-check"], job);
  try {
    return renderToStaticMarkup(<QueryClientProvider client={client}><MemoryRouter><PendingDialog /></MemoryRouter></QueryClientProvider>);
  } finally {
    client.clear();
  }
}

it("shows live progress with the dialog minimized", () => {
  const html = minimized({ id: "run", state: "running", items: [
    { task_id: 1, project_id: 1, title: "Done", project: "One", state: "done" },
    { task_id: 2, project_id: 1, title: "Queued", project: "One", state: "queued" },
  ] });
  expect(html).toContain("Checking · 1/2");
  expect(html).toContain("width:50%");
  expect(html).toContain('aria-haspopup="dialog"');
  expect(html).toContain('aria-expanded="false"');
  expect(html).not.toContain('role="dialog"');
});

it("explains when all cards became ineligible before starting instead of hiding the run", () => {
  const html = minimized({ id: "empty", state: "completed", items: [] });
  expect(html).toContain("No matching cards");
  expect(html).toContain("width:100%");
  expect(html).not.toContain("NaN");
});
