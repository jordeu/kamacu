import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getWorkspaceProject, rememberWorkspaceProject } from "./workspaceProject";

const projects = [
  { id: 1, workspace_id: 10 },
  { id: 2, workspace_id: 10 },
  { id: 3, workspace_id: 20 },
  { id: 4, workspace_id: 20 },
];

beforeEach(() => {
  const values = new Map<string, string>();
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => values.set(key, value),
  });
});

afterEach(() => vi.unstubAllGlobals());

describe("workspace project history", () => {
  it("restores each workspace's last project independently on repeated switches", () => {
    rememberWorkspaceProject(10, 2);
    rememberWorkspaceProject(20, 4);
    expect(getWorkspaceProject(10, projects)?.id).toBe(2);
    expect(getWorkspaceProject(20, projects)?.id).toBe(4);
    rememberWorkspaceProject(10, 1);
    expect(getWorkspaceProject(10, projects)?.id).toBe(1);
    expect(getWorkspaceProject(20, projects)?.id).toBe(4);
  });

  it("uses the first project for a workspace with no history", () => {
    expect(getWorkspaceProject(10, projects)?.id).toBe(1);
  });

  it("falls back when the saved project was deleted or moved", () => {
    rememberWorkspaceProject(10, 2);
    expect(getWorkspaceProject(10, projects.filter((p) => p.id !== 2))?.id).toBe(1);
    const moved = projects.map((p) =>
      p.id === 2 ? { ...p, workspace_id: 20 } : p,
    );
    expect(getWorkspaceProject(10, moved)?.id).toBe(1);
  });

  it("handles empty workspaces and invalid saved values", () => {
    rememberWorkspaceProject(10, 2);
    expect(getWorkspaceProject(10, [])).toBeUndefined();
    localStorage.setItem("kamacu.workspace.10.last-project", "invalid");
    expect(getWorkspaceProject(10, projects)?.id).toBe(1);
  });

  it("keeps navigation usable when storage reads or writes fail", () => {
    vi.stubGlobal("localStorage", {
      getItem: () => { throw new Error("Storage blocked"); },
      setItem: () => { throw new Error("Storage quota exceeded"); },
    });
    expect(() => rememberWorkspaceProject(10, 2)).not.toThrow();
    expect(getWorkspaceProject(10, projects)?.id).toBe(1);
    expect(getWorkspaceProject(30, projects)).toBeUndefined();
  });

});
