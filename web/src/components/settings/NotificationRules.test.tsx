import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { NotificationRules } from "./NotificationRules";
import { useNotificationRules, useRuleMutation, useRulePreview, type NotificationRuleSet } from "@/api/notificationRules";

vi.mock("@/api/notificationRules", () => ({ useNotificationRules: vi.fn(), useRuleMutation: vi.fn(), useRulePreview: vi.fn() }));
const rule = { compiler_version: "1", id: "R001", revision: 1, enabled: true, text: "Ignore platform unless I am directly mentioned", clauses: [{ id: "R001.1", category: "filtering", effect: "suppress", argument: "", priority: 0, on_unknown: "hold", when: { op: "all", args: [{ op: "eq", fact: "repository", value: "seqeralabs/platform" }, { op: "not", args: [{ op: "eq", fact: "viewer.mentioned", value: true }] }] } }] };
function show(status: NotificationRuleSet["status"], diagnostics: string[] = []) {
 vi.mocked(useNotificationRules).mockReturnValue({ data: [{ id: "set", revision: 2, status, rules: [rule], diagnostics }], isPending: false, isError: false } as unknown as ReturnType<typeof useNotificationRules>);
 return renderToStaticMarkup(<NotificationRules />);
}
beforeEach(() => {
 vi.resetAllMocks();
 vi.mocked(useRuleMutation).mockReturnValue({ mutate: vi.fn(), isPending: false, isError: false } as unknown as ReturnType<typeof useRuleMutation>);
 vi.mocked(useRulePreview).mockReturnValue({ data: undefined } as ReturnType<typeof useRulePreview>);
});
describe("notification rule lifecycle", () => {
 it("does not offer activation for an uncompiled draft", () => { const html = show("draft"); expect(html).toContain("Prepare rules"); expect(html).not.toContain("Activate revision"); expect(html).toContain("R001"); expect(html).toContain(rule.text); });
 it("shows the interpreted condition and explicit unknown behavior before activation", () => { const html = show("ready"); expect(html).toContain(">Compiled</p>"); expect(html).toContain("Compiled interpretation"); expect(html).toContain("repository equals"); expect(html).toContain("viewer.mentioned equals true"); expect(html).toContain("Missing evidence: hold"); expect(html).toContain("Activate revision 2"); expect(html).toContain("Test on recent notifications"); });
 it("exposes compilation errors and keeps the invalid draft inactive", () => { const html = show("invalid", ["Conflicting action rules R001 and R002"]); expect(html).toContain("Conflicting action rules"); expect(html).not.toContain("Activate revision"); });
 it("keeps compilation observable and disables editing while it runs", () => { const html = show("compiling"); expect(html).toContain("Preparing rules"); expect(html).toMatch(/<textarea[^>]*disabled/); expect(html).not.toContain("Activate revision"); });
 it("identifies the active revision separately from drafts", () => { const html = show("active"); expect(html).toContain("Active rule set: revision 2"); expect(html).not.toContain("No active rule set"); });
});

it("shows the per-rule fallback explanation and alternative rules", () => {
 const fallback = { ...rule, mode: "llm" as const, clauses: [], diagnostic: "The PR scope is ambiguous.", alternatives: ["Group review requests on PRs authored by seqeralabs-renovate by project."] };
 vi.mocked(useNotificationRules).mockReturnValue({ data: [{ id: "set", revision: 2, status: "ready", rules: [fallback], diagnostics: [] }], isPending: false, isError: false } as unknown as ReturnType<typeof useNotificationRules>);
 const html=renderToStaticMarkup(<NotificationRules />);
 expect(html).toContain("LLM evaluated · not compiled");
 expect(html).toContain("The PR scope is ambiguous.");
 expect(html).toContain("Suggested alternative rules");
 expect(html).toContain(fallback.alternatives[0]);
 expect(html).toContain("Activate revision 2");
 expect(html).not.toContain("Compiled interpretation");
});
