import { expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import type { FeedbackQuestion } from "@/api/inbox";
import { InboxMessage } from "./InboxMessage";

it("hides internal notification comments while preserving visible Markdown and HTML code examples", () => {
  const question: FeedbackQuestion = {
    id: "notification",
    task_id: 0,
    project_id: 0,
    task_title: "",
    project_name: "",
    session_id: "",
    abstract: "",
    summary: "Review project updates",
    question: "Review these updates?",
    options: [],
    context:
      '<!-- kamacu-triage-evidence:[{"scope":"internal-fingerprint"}] -->\n\nUseful **context**.\n\n<!-- kamacu-triage:review_request -->\n\n```html\n<div>example</div>\n```',
    status: "pending",
    reason: "",
    answer: "",
    answer_source: "",
    revision: 1,
    created_at: 1,
    updated_at: 1,
    acknowledged_at: null,
    origin: "notification",
    source_url: "https://github.com/acme/repo/pull/1",
  };
  const html = renderToStaticMarkup(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <InboxMessage question={question} expanded onResult={() => {}} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  expect(html).toContain("Improve rules");
  expect(html).not.toContain("kamacu-triage");
  expect(html).not.toContain("internal-fingerprint");
  expect(html).toContain("<strong>context</strong>");
  expect(html).toContain("&lt;div&gt;example&lt;/div&gt;");
});

it("renders new-tab links throughout pending and answered message dialogs", () => {
  const question: FeedbackQuestion = {
    id: "links",
    task_id: 1,
    project_id: 1,
    task_title: "Task",
    project_name: "Project",
    session_id: "",
    abstract: "",
    question: "Review https://github.com/acme/title/pull/1",
    summary: "See https://github.com/acme/summary/issues/2",
    context: "[Context](https://github.com/acme/context/pull/3)",
    options: [
      { id: "yes", label: "Approve https://github.com/acme/option/pull/4" },
    ],
    status: "pending",
    reason: "",
    answer: "https://github.com/acme/answer/issues/5",
    answer_source: "",
    revision: 1,
    created_at: 1,
    updated_at: 1,
    acknowledged_at: null,
  };
  function render(status: FeedbackQuestion["status"]) {
    return renderToStaticMarkup(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter>
          <InboxMessage
            question={{ ...question, status }}
            dialog
            onResult={() => {}}
          />
        </MemoryRouter>
      </QueryClientProvider>,
    );
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
  expect(render("answered")).toContain(">answer#5</a>");
});

it("keeps task navigation independent of notification actions in pending and archived cards", () => {
  const q: FeedbackQuestion = {
    id: "bound",
    task_id: 0,
    project_id: 0,
    task_title: "",
    project_name: "",
    session_id: "",
    abstract: "",
    summary: "New review feedback",
    context: "Source",
    question: "Investigate?",
    options: [{ id: "investigate", label: "Create another task" }],
    status: "pending",
    reason: "",
    answer: "",
    answer_source: "",
    revision: 1,
    created_at: 1,
    updated_at: 1,
    acknowledged_at: null,
    origin: "notification",
    source_url: "https://github.com/acme/repo/pull/7",
    linked_tasks: [
      {
        task_id: 1117,
        project_id: 2,
        title: "Existing implementation",
        status: "in_review",
        role: "implementation",
        source: "pr_link",
        evidence: "PR link",
      },
      {
        task_id: 1222,
        project_id: 2,
        title: "Existing review",
        status: "done",
        role: "review",
        source: "pr_link",
        evidence: "PR link",
      },
    ],
  };
  for (const status of ["pending", "answered", "unanswered"] as const) {
    const html = renderToStaticMarkup(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter>
          <InboxMessage
            question={{
              ...q,
              status,
              reason:
                status === "unanswered"
                  ? "notification_task_routing_changed"
                  : "",
            }}
            dialog
            onResult={() => {}}
          />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    expect(html).toContain('aria-label="Linked tasks"');
    expect(html).toContain('href="/projects/2/tasks/1117"');
    expect(html).toContain('href="/projects/2/tasks/1222"');
    expect(html).toContain("Open task 1117");
    expect(html).toContain("Open notification");
    if (status === "unanswered")
      expect(html).toContain("Requeued for task matching");
  }
});

it("shows saved direct actions and explains that free text is interpreted once", () => {
  const q: FeedbackQuestion = {
    id: "direct",
    attention_source_id: "original-run",
    task_id: 0,
    project_id: 0,
    task_title: "",
    project_name: "",
    session_id: "",
    abstract: "Review",
    summary: "Two review requests",
    context: "Requested by the author",
    question: "Start reviews?",
    options: [
      {
        id: "review",
        label: "Review both",
        action_summary:
          "Open two review workspaces and submit configured prompts",
        action: {
          version: 1,
          actions: [
            {
              id: "reviews",
              kind: "start_reviews",
              project_id: 1,
              pr_numbers: [7, 8],
            },
          ],
          guards: { review_seed: "Review PR <n>: <title>" },
        },
      },
    ],
    status: "pending",
    reason: "",
    answer: "",
    answer_source: "",
    revision: 1,
    created_at: 1,
    updated_at: 1,
    acknowledged_at: null,
    origin: "notification",
  };
  const html = renderToStaticMarkup(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <InboxMessage question={q} dialog onResult={() => {}} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  expect(html).toContain("Original execution steps");
  expect(html).toContain("Direct action · no extra model call");
  expect(html).toContain(
    "Open two review workspaces and submit configured prompts",
  );
  expect(html).toContain("Configured review prompt");
  expect(html).toContain("interpreted once before execution");
});

it("shows exhausted recovery as an operational issue without requesting a reply", () => {
  const question: FeedbackQuestion = {
    id: "operational",
    task_id: 0,
    project_id: 0,
    task_title: "",
    project_name: "",
    session_id: "",
    abstract: "Database recording failed",
    summary: "Automatic recovery stopped",
    question: "No reply is required.",
    context: "The database remained unavailable. Existing work is preserved.",
    options: [{ id: "ignore", label: "Dismiss this notice" }],
    status: "pending",
    reason: "operational_failure",
    answer: "",
    answer_source: "",
    revision: 1,
    created_at: 1,
    updated_at: 1,
    acknowledged_at: null,
    origin: "notification",
  };
  const html = renderToStaticMarkup(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <InboxMessage question={question} expanded onResult={() => {}} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  expect(html).toContain("Operational issue");
  expect(html).toContain("Failure details");
  expect(html).toContain("Dismiss this notice");
  expect(html).not.toContain("Needs reply");
  expect(html).not.toContain("Write your own answer");
  expect(html).not.toContain("Improve rules");
});
