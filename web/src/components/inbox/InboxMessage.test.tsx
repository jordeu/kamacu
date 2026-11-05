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

it("renders new-tab links throughout pending and answered message dialogs", () => {
  const question: FeedbackQuestion = {
    id: "links", task_id: 1, project_id: 1, task_title: "Task", project_name: "Project", session_id: "",
    abstract: "", question: "Review https://github.com/acme/title/pull/1",
    summary: "See https://github.com/acme/summary/issues/2",
    context: "[Context](https://github.com/acme/context/pull/3)",
    options: [{ id: "yes", label: "Approve https://github.com/acme/option/pull/4" }],
    status: "pending", reason: "", answer: "https://github.com/acme/answer/issues/5", answer_source: "", revision: 1,
    created_at: 1, updated_at: 1, acknowledged_at: null,
  };
  function render(status: FeedbackQuestion["status"]) {
    return renderToStaticMarkup(<QueryClientProvider client={new QueryClient()}><MemoryRouter>
      <InboxMessage question={{ ...question, status }} dialog onResult={() => {}} />
    </MemoryRouter></QueryClientProvider>);
  }
  const pending = render("pending");
  for (const label of ["title#1", "summary#2", "context#3", "option#4"]) {
    expect(pending).toContain(`>${label}</a>`);
  }
  for (const anchor of pending.matchAll(/<a\b[^>]*>/g)) {
    expect(anchor[0]).toContain('target="_blank"');
    expect(anchor[0]).toContain('rel="noopener noreferrer"');
  }
  for (const button of pending.matchAll(/<button\b[^>]*>[\s\S]*?<\/button>/g)) {
    expect(button[0]).not.toContain("<a ");
  }
  expect(render("answered")).toContain('>answer#5</a>');
});
