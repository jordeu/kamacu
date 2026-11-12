import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, expect, it, vi } from "vitest";
import { NotificationRepair } from "./NotificationRepair";

const state = vi.hoisted(() => ({
  preview: {
    data: undefined as
      | undefined
      | { event_id: number; url: string; title: string; reason: string }[],
    isPending: false,
    isError: false,
    mutate: vi.fn(),
  },
  repair: { isPending: false, isError: false, reset: vi.fn(), mutate: vi.fn() },
}));
vi.mock("@/api/notifications", () => ({
  useNotificationRepairPreview: () => state.preview,
  useRepairNotifications: () => state.repair,
}));
beforeEach(() => {
  state.preview.data = undefined;
  state.preview.mutate.mockClear();
  state.repair.mutate.mockClear();
});
it("offers an explicit preview without automatically retrieving or reprocessing history", () => {
  const html = renderToStaticMarkup(<NotificationRepair />);
  expect(html).toContain("Check for missed notifications");
  expect(html).toContain(
    "Checking does not process notifications or start work",
  );
  expect(state.preview.mutate).not.toHaveBeenCalled();
  expect(state.repair.mutate).not.toHaveBeenCalled();
});
it("shows affected PRs with no preselected repair and a disabled queue action", () => {
  state.preview.data = [
    {
      event_id: 1,
      url: "https://github.com/acme/repo/pull/7",
      title: "Missed human review",
      reason: "Incomplete evidence",
    },
  ];
  const html = renderToStaticMarkup(<NotificationRepair />);
  expect(html).toContain('href="https://github.com/acme/repo/pull/7"');
  expect(html).toContain("Move selected to pending (0)");
  expect(html).toContain('type="checkbox"');
  expect(html).not.toContain("checked=");
  expect(state.repair.mutate).not.toHaveBeenCalled();
});
