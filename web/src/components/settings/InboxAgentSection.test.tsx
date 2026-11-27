import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { InboxAgentSection } from "./InboxAgentSection";

vi.mock("@tanstack/react-query", () => ({ useQuery: () => ({ data: { profiles: [{ id: 7, name: "Inbox model", model: "chosen", backend: "codex_cli" }] } }) }));
vi.mock("@/api/inboxAgent", () => ({
  useInboxAgentConfig: () => ({ data: { profile_id: 7, revision: 2, context_bytes: 1500000, output_tokens: 4000, timeout_seconds: 300, rules: [{ id: "I-review", text: "Start a rereview only after a previous approval.", enabled: true }] } }),
  useSaveInboxAgentConfig: () => ({}), useTestInboxAgent: () => ({ reset: vi.fn() }),
}));

describe("inbox agent settings", () => {
  it("offers an independent model and explicit publication rules without manual or periodic processing", () => {
    const html = renderToStaticMarkup(<InboxAgentSection />);
    for (const text of ["Inbox model", "Start a rereview only after a previous approval.", "Save inbox agent", "Add inbox rule", "Test inbox model", "Successful replies go directly to Archived", "Unclear cases and failures remain in Pending"]) expect(html).toContain(text);
    expect(html).not.toContain("interval_seconds");
    expect(html).not.toContain("Process pending");
    expect(html).not.toContain("Automatic processing interval");
  });
});
