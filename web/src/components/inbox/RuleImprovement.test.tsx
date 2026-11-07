import { expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { RuleImprovement, RuleHistoryComparison } from "./RuleImprovement";

it("blocks suggestions while settings contain unsaved rule edits", () => {
 const html = renderToStaticMarkup(<RuleImprovement recurring disabled disabledReason="Save your rule edits before requesting suggestions." />);
 expect(html).toContain("Review recurring problems");
 expect(html).toMatch(/<button[^>]*disabled/);
 expect(html).toContain("Save your rule edits before requesting suggestions.");
 expect(html).not.toContain("Diagnose and propose draft");
});
it("opens notification feedback separately from starting task work", () => {
 const html = renderToStaticMarkup(<RuleImprovement questionID="message" />);
 expect(html).toContain("Improve rules");
 expect(html).not.toMatch(/<button[^>]*\sdisabled(?:=|\s|>)/);
 expect(html).not.toContain("Start task");
});
it("offers an explicit historical comparison without running it on mount", () => {
 expect(renderToStaticMarkup(<RuleHistoryComparison setID="draft" />)).toContain("Compare against processed history");
});
