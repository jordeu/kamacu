import { expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { RuleImprovement } from "./RuleImprovement";
it("offers explicit feedback without saving or interpreting rules on mount", () => {
  const html = renderToStaticMarkup(<RuleImprovement questionID="message" />);
  expect(html).toContain("Improve rules");
  expect(html).not.toContain("Save suggested rules");
  expect(html).not.toContain("Prepare");
});
