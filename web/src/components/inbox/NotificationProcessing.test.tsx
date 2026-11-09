import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { useNotificationStatus, useProcessNotifications, type NotificationStatus } from "@/api/notifications";
import { TooltipProvider } from "@/components/ui/tooltip";
import { NotificationProcessing } from "./NotificationProcessing";

vi.mock("@/api/notifications", () => ({ useNotificationStatus: vi.fn(), useProcessNotifications: vi.fn() }));
function show(data: NotificationStatus, collapsed = false) {
 vi.mocked(useNotificationStatus).mockReturnValue({ data, isError: false } as ReturnType<typeof useNotificationStatus>);
 return renderToStaticMarkup(<MemoryRouter><TooltipProvider><NotificationProcessing collapsed={collapsed} /></TooltipProvider></MemoryRouter>);
}
beforeEach(() => {
 vi.resetAllMocks();
 vi.mocked(useProcessNotifications).mockReturnValue({ mutate: vi.fn(), isPending: false, isError: false } as unknown as ReturnType<typeof useProcessNotifications>);
});
describe("manual notification processing control", () => {
 it("shows a raw counter and an explicit processing button in both sidebar layouts", () => {
  for (const collapsed of [false, true]) {
   const html = show({ pending_count: 7, processing: null }, collapsed);
   expect(html).toContain("7 pending notifications");
   expect(html).toContain('href="/notifications"');
   expect(html).toContain('aria-label="Process pending notifications"');
   expect(html).not.toMatch(/<button[^>]*\sdisabled(?:=|\s|>)/);
  }
 });
 it("disables duplicate processing while a snapshot is queued or running", () => {
  for (const status of ["queued", "running"] as const) {
   const html = show({ pending_count: 8, processing: { id: "run", status, notification_count: 5, action_count: 0, created_at: 0, updated_at: 0, error: "" } });
   expect(html).toContain("8 pending notifications");
   expect(html).toContain("Processing 5 notifications");
   expect(html).toMatch(/<button[^>]*\sdisabled(?:=|\s|>)/);
  }
 });
 it("keeps failed runs retryable and explains that raw items were preserved", () => {
  const html = show({ pending_count: 3, processing: { id: "run", status: "failed", notification_count: 3, action_count: 0, created_at: 0, updated_at: 0, error: "Model unavailable." } });
  expect(html).toContain("Model unavailable.");
  expect(html).toContain("Pending notifications were kept.");
  expect(html).not.toMatch(/<button[^>]*\sdisabled(?:=|\s|>)/);
 });
 it("reports created actions separately from the remaining raw count", () => {
  const html = show({ pending_count: 0, processing: { id: "run", status: "completed", notification_count: 9, action_count: 2, created_at: 0, updated_at: 0, error: "" } });
  expect(html).toContain("0 pending notifications");
  expect(html).toContain("9 notifications processed");
  expect(html).toContain("2 inbox actions created");
  expect(html).toMatch(/<button[^>]*\sdisabled(?:=|\s|>)/);
 });
});

it("requires an explicit notification model without blocking collection",()=>{const html=show({pending_count:23,model_configured:false,processing:null});expect(html).toContain("23 pending notifications");expect(html).toContain("Choose a notification model in Settings");expect(html).toMatch(/<button[^>]*\sdisabled(?:=|\s|>)/);});
