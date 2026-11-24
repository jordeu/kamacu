import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { FeedbackQuestion } from "@/api/inbox";
import { AutomaticReply } from "./AutomaticReply";

const question = {
  automation: {
    status: "answered", config_revision: 3,
    rules: [{ id: "I-release", text: "Approve verified releases for acme/release." }],
    reason: "The author and project match the saved rule.", answer: "Yes, proceed.", option_id: "yes",
  },
} as FeedbackQuestion;

describe("automatic inbox responses", () => {
  it("shows the saved response and immutable triggering rule", () => {
    const html = renderToStaticMarkup(<AutomaticReply question={question} />);
    for (const text of ["Automatically answered by Inbox agent", "Yes, proceed.", "Triggering rule", "revision 3", "I-release", "Approve verified releases for acme/release."]) expect(html).toContain(text);
  });
  it("shows a delivery failure without offering a resend", () => {
    const q = { ...question, delivery_status: "uncertain", automation: { ...question.automation!, status: "failed", error: "Could not confirm terminal delivery." } };
    const html = renderToStaticMarkup(<AutomaticReply question={q} />);
    for (const text of ["Automatic response needs attention", "Attempted response", "Could not confirm terminal delivery.", "will not be sent again automatically"]) expect(html).toContain(text);
    expect(html).not.toContain("<button");
  });
  it("does not decorate ordinary pending messages", () => {
    expect(renderToStaticMarkup(<AutomaticReply question={{} as FeedbackQuestion} />)).toBe("");
  });
});
