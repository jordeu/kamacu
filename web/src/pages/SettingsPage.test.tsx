import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import SettingsPage from "./SettingsPage";

vi.mock("@/api/settings", () => ({
  useSettings: () => ({ data: { github_integration: { value: "off" } } }),
  useSaveSetting: () => ({}),
}));
vi.mock("@/api/queries", () => ({ useGithubStatus: () => ({ data: { gh_available: true } }) }));
vi.mock("@/components/settings/SettingsField", () => ({ SettingsField: ({ label }: { label: string }) => <div>{label}</div> }));
vi.mock("@/components/settings/AgentsSection", () => ({ AgentsSection: () => <div>Agent controls</div> }));
vi.mock("@/components/settings/AgentIntegrations", () => ({ AgentIntegrations: () => <div>Integration controls</div> }));
vi.mock("@/components/settings/ScratchpadSection", () => ({ ScratchpadSection: () => <div>Scratchpad controls</div> }));
vi.mock("@/components/settings/MonitoringSection", () => ({ MonitoringSection: ({ view }: { view: string }) => <div>{view === "profiles" ? "Profile controls" : "Monitor controls"}</div> }));
vi.mock("@/components/settings/InboxAgentSection", () => ({ InboxAgentSection: () => <div>Inbox agent controls</div> }));
vi.mock("@/components/settings/NotificationModel", () => ({ NotificationModel: () => <div>Notification model controls</div> }));
vi.mock("@/components/settings/NotificationRules", () => ({ NotificationRules: () => <div>Notification rules controls</div> }));
vi.mock("@/components/inbox/NotificationSchedule", () => ({ NotificationSchedule: () => <div>Notification schedule controls</div> }));
vi.mock("@/components/settings/NotificationsSection", () => ({ NotificationsSection: () => <div>Notification controls</div> }));
vi.mock("@/components/settings/WorktreeCleanupSection", () => ({ WorktreeCleanupSection: () => <div>Cleanup controls</div> }));

function render(section = "") {
  return renderToStaticMarkup(<MemoryRouter initialEntries={[`/settings${section}`]}><SettingsPage /></MemoryRouter>);
}

function panel(html: string, name: string) {
  return html.match(new RegExp(`<div[^>]*data-state="[^"]*"[^>]*id="[^"]*-content-${name}"[^>]*>`))?.[0];
}

describe("settings categories", () => {
  it("keeps each control in its expected category with labelled panels", () => {
    const html = render();
    const expected = {
      workspace: ["Worktree base directory", "Shell", "Branch name template", "Browser notifications"],
      agents: ["Agent controls", "Integration controls", "Profile controls", "Scratchpad controls", "Monitor controls", "Notification model controls", "Notification rules controls", "Inbox agent controls"],
      prompts: ["Custom prompts", "Add prompt"],
      github: ["GitHub integration", "Notification controls"],
      cleanup: ["Done session TTL", "Cleanup controls"],
    };
    const panels = html.split('role="tabpanel"').slice(1);
    expect(panels).toHaveLength(5);
    Object.entries(expected).forEach(([name, controls], index) => {
      expect(panels[index]).toContain(`-trigger-${name}`);
      controls.forEach(control => expect(panels[index]).toContain(control));
      Object.entries(expected).filter(([other]) => other !== name).forEach(([, otherControls]) => {
        otherControls.forEach(control => expect(panels[index]).not.toContain(`>${control}<`));
      });
    });
  });

  it("starts with workspace and keeps other panels mounted but hidden", () => {
    const html = render();
    expect(panel(html, "workspace")).toContain('data-state="active"');
    for (const name of ["agents", "prompts", "github", "cleanup"]) {
      expect(panel(html, name)).toContain('hidden=""');
    }
    expect(html).toContain("Agent controls");
    expect(html).toContain("Cleanup controls");
  });

  it("opens the category in the URL, including GitHub when integration is off", () => {
    const html = render("?section=github");
    expect(panel(html, "github")).toContain('data-state="active"');
    expect(panel(html, "github")).not.toContain('hidden=""');
    expect(panel(html, "workspace")).toContain('hidden=""');
    expect(html).toContain("GitHub integration");
    expect(html).toContain("Notification controls");
  });

  it.each([
    ["?section=agents", "Coding agents"],
    ["?section=agents&agent=unknown", "Coding agents"],
    ["?section=agents&agent=profiles", "Model profiles"],
    ["?section=monitoring", "Terminal monitoring"],
  ])("selects the agent category for %s and keeps all category controls mounted", (url, selected) => {
    const html = render(url);
    expect(panel(html, "agents")).toContain('data-state="active"');
    expect(html).toMatch(new RegExp(`<a[^>]*aria-current="page"[^>]*>${selected}</a>`));
    const sections = [...html.matchAll(/<section[^>]*aria-labelledby="([^"]+)-(coding|integrations|monitoring|notifications|inbox|profiles|scratchpad)"[^>]*>/g)];
    expect(sections).toHaveLength(7);
    expect(sections.filter(([tag]) => !tag.includes('hidden=""'))).toHaveLength(1);
    expect(html).toContain("Profile controls");
  });

  it("falls back to workspace for an unknown category", () => {
    expect(panel(render("?section=missing"), "workspace")).toContain('data-state="active"');
  });
});
