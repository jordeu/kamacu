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
vi.mock("@/components/settings/ScratchpadSection", () => ({ ScratchpadSection: () => <div>Scratchpad controls</div> }));
vi.mock("@/components/settings/MonitoringSection", () => ({ MonitoringSection: () => <div>Monitor controls</div> }));
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
      workspace: ["Worktree base directory", "Shell", "Branch name template"],
      agents: ["Agent controls", "Scratchpad controls"],
      monitoring: ["Monitor controls"],
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
    for (const name of ["agents", "monitoring", "github", "cleanup"]) {
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

  it("falls back to workspace for an unknown category", () => {
    expect(panel(render("?section=missing"), "workspace")).toContain('data-state="active"');
  });
});
