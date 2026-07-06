---
id: T03
parent: S02
milestone: M001
key_files:
  - web/src/components/settings/AgentNameDialog.tsx
  - web/src/components/settings/AgentsSection.tsx
  - web/src/pages/SettingsPage.tsx
key_decisions:
  - (none)
duration: 
verification_result: passed
completed_at: 2026-07-06T16:20:25.490Z
blocker_discovered: false
---

# T03: Built global Settings Agents section (AgentNameDialog create/edit + AgentsSection list with set-default/edit/guarded-delete) and wired into SettingsPage; tsc + vite green.

**Built global Settings Agents section (AgentNameDialog create/edit + AgentsSection list with set-default/edit/guarded-delete) and wired into SettingsPage; tsc + vite green.**

## What Happened

Built the M001 agents management UI as a Settings page section. Two new components: (1) AgentNameDialog.tsx — a two-field create/edit dialog (Name + Command, the command mono with placeholder help for {{worktree}}/{{session_id}}), cloned from WorkspaceNameDialog's structure; (2) AgentsSection.tsx — the management hub: ordered list (useAgents, default-first per backend sort), each row shows name + an engine badge (Claude/Custom via Badge variant=secondary), a set-default control (star glyph, amber when default, calls useSetDefaultAgent), an Edit button (opens AgentNameDialog in edit mode), and a guarded Delete (disabled-with-tooltip when is_system OR count-by-agent > 0, mirroring ManageWorkspacesDialog's countByWorkspace), plus a "New agent" entry. Delete confirm via AlertDialog. Wired AgentsSection into SettingsPage right after the existing Agent extra-params section.

## Verification

cd web && npx tsc -b (exit 0) && npx vite build (exit 0). Removed an unused Dialog import flagged by tsc. Badge variant=secondary confirmed available for the engine badge.

## Verification Evidence

| # | Command | Exit Code | Verdict | Duration |
|---|---------|-----------|---------|----------|
| 1 | `cd web && npx tsc -b` | 0 | ✅ pass | 8000ms |
| 2 | `cd web && npx vite build` | 0 | ✅ pass | 5000ms |

## Deviations

Used a star glyph (★/☆) for the set-default control instead of a native radio — a button styled to read as default-or-not, which fits the row's icon-button vocabulary better than a radio and matches the amber accent the v1.9 default workspace uses. The AlertDialog description says "Its projects are unaffected — it has none" (mirrors ManageWorkspacesDialog) since the delete is already guarded to in-use agents. Removed an unused Dialog import caught by tsc.

## Known Issues

None.

## Files Created/Modified

- `web/src/components/settings/AgentNameDialog.tsx`
- `web/src/components/settings/AgentsSection.tsx`
- `web/src/pages/SettingsPage.tsx`
