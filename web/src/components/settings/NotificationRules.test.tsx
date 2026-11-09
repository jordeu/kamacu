import { beforeEach, expect, it, vi } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { NotificationRules } from "./NotificationRules";
import {
  useNotificationRules,
  useSaveNotificationRules,
  useStartRulePreview,
  useRulePreview,
} from "@/api/notificationRules";
vi.mock("@/api/notificationRules", () => ({
  useNotificationRules: vi.fn(),
  useSaveNotificationRules: vi.fn(),
  useStartRulePreview: vi.fn(),
  useRulePreview: vi.fn(),
}));
beforeEach(() => {
  vi.mocked(useNotificationRules).mockReturnValue({
    data: [
      {
        id: "rules",
        revision: 3,
        status: "active",
        rules: [
          {
            id: "R003",
            revision: 1,
            text: "Group Renovate reviews by project and use the recurring task",
            enabled: true,
          },
        ],
      },
    ],
    isPending: false,
    isError: false,
  } as unknown as ReturnType<typeof useNotificationRules>);
  for (const hook of [useSaveNotificationRules, useStartRulePreview])
    vi.mocked(hook).mockReturnValue({
      isPending: false,
      mutate: vi.fn(),
    } as never);
  vi.mocked(useRulePreview).mockReturnValue({ data: undefined } as ReturnType<
    typeof useRulePreview
  >);
});
it("edits plain instructions with stable IDs and direct save", () => {
  const html = renderToStaticMarkup(<NotificationRules />);
  expect(html).toContain("R003");
  expect(html).toContain("Group Renovate reviews");
  expect(html).toContain("Save rules");
  expect(html).not.toContain("Prepare rules");
  expect(html).not.toContain("Activate");
  expect(html).not.toContain("Execution mode");
});
it("offers an agent preview without compilation", () => {
  expect(renderToStaticMarkup(<NotificationRules />)).toContain(
    "Preview recent notifications",
  );
});
it("preserves a historical unsaved list for explicit review", () => {
  vi.mocked(useNotificationRules).mockReturnValue({
    data: [{ id: "draft", revision: 4, status: "draft", rules: [] }],
    isPending: false,
    isError: false,
  } as unknown as ReturnType<typeof useNotificationRules>);
  expect(renderToStaticMarkup(<NotificationRules />)).toContain(
    "preserved unsaved revision",
  );
});
