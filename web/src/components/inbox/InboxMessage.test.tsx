import { expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import type { FeedbackQuestion } from "@/api/inbox";
import { InboxMessage } from "./InboxMessage";

it("hides internal notification comments while preserving visible Markdown and HTML code examples", () => {
  const question: FeedbackQuestion = {
    id: "notification", task_id: 0, project_id: 0, task_title: "", project_name: "", session_id: "",
    abstract: "", summary: "Review project updates", question: "Review these updates?", options: [],
    context: '<!-- kamacu-triage-evidence:[{"scope":"internal-fingerprint"}] -->\n\nUseful **context**.\n\n<!-- kamacu-triage:review_request -->\n\n```html\n<div>example</div>\n```',
    status: "pending", reason: "", answer: "", answer_source: "", revision: 1, created_at: 1, updated_at: 1, acknowledged_at: null,
    origin: "notification", source_url: "https://github.com/acme/repo/pull/1",
  };
  const html = renderToStaticMarkup(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter><InboxMessage question={question} expanded onResult={() => {}} /></MemoryRouter>
    </QueryClientProvider>,
  );
  expect(html).not.toContain("kamacu-triage");
  expect(html).not.toContain("internal-fingerprint");
  expect(html).toContain("<strong>context</strong>");
  expect(html).toContain("&lt;div&gt;example&lt;/div&gt;");
});
