/** Persist the last visited project independently for each workspace. */
function storageKey(workspaceId: number): string {
  return `kamacu.workspace.${workspaceId}.last-project`;
}

export function rememberWorkspaceProject(
  workspaceId: number,
  projectId: number,
): void {
  try {
    localStorage.setItem(storageKey(workspaceId), String(projectId));
  } catch {
    // History is optional; storage restrictions must not interrupt navigation.
  }
}

/** Only restore projects that still belong to the workspace. */
export function getWorkspaceProject<T extends { id: number; workspace_id: number }>(
  workspaceId: number,
  projects: readonly T[],
): T | undefined {
  const candidates = projects.filter(
    (project) => project.workspace_id === workspaceId,
  );
  try {
    const savedId = Number(localStorage.getItem(storageKey(workspaceId)));
    return candidates.find((project) => project.id === savedId) ?? candidates[0];
  } catch {
    return candidates[0];
  }
}
