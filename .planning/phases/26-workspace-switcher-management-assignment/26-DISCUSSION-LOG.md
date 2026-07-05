# Phase 26: Workspace Switcher, Management & Assignment - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-07-05
**Phase:** 26-workspace-switcher-management-assignment
**Areas discussed:** Switcher shape & placement, Create/rename/delete flows, Project transfer UX, Switch nav / empty state / reload

---

## Switcher shape & placement

### Trigger form
| Option | Description | Selected |
|--------|-------------|----------|
| Name + chevron button | Full-width button row (workspace name + chevron) styled as a section header, opens a dropdown-menu. Reuses shadcn dropdown-menu. | ✓ |
| shadcn Select control | Native-feeling Select; awkward host for management actions. | |
| Name text + caret icon | Plain heading + separate caret icon-button. Two visual targets. | |

### Where rename & delete live
| Option | Description | Selected |
|--------|-------------|----------|
| "Manage workspaces…" dialog | Dropdown does pick-active + quick "+ New workspace"; a "Manage workspaces…" entry opens a dedicated dialog with per-row rename/delete. | ✓ |
| Inline per-row in dropdown | Hover rename/delete affordances per row; cramped, no home for the block message. | |
| Settings page section | Workspaces section on /settings; more separation, more clicks. | |

**User's choice:** Name + chevron trigger + dedicated Manage dialog.
**Notes:** Approved the ASCII mockups of both the trigger row and the dropdown/Manage-dialog split.

---

## Create / rename / delete flows

### Create & rename style
| Option | Description | Selected |
|--------|-------------|----------|
| Small name dialog | Compact name-Input dialog mirroring RenameProjectDialog; inline unique-name error. | ✓ |
| Inline editable row | Edit-in-place inside the Manage dialog; no clean home for the error. | |
| You decide | Follow existing patterns. | |

### Non-empty delete guard
| Option | Description | Selected |
|--------|-------------|----------|
| Disable + hint proactively | Delete disabled with "Move or remove its N projects first" tooltip; count computed client-side. Backend still enforces. | ✓ |
| Click → refusal message | Always clickable; shows the block message on attempt. Dead-end. | |
| Backend 409 → toast | No client preemption; surface the 409. Most round-trips. | |

### Default "Personal" delete affordance
| Option | Description | Selected |
|--------|-------------|----------|
| Shown but disabled + tooltip | Visible-but-disabled with "The default workspace can't be deleted"; keyed off is_default. | ✓ |
| Hidden entirely | No delete action; the "why" is invisible. | |
| You decide | No preference. | |

### Confirm on empty delete
| Option | Description | Selected |
|--------|-------------|----------|
| Light AlertDialog confirm | Small "Delete workspace?" confirm, consistent with project delete. | ✓ |
| No confirm — immediate | Delete immediately; inconsistent, no accident guard. | |

**User's choice:** name dialog; proactively-disabled non-empty delete; Personal disabled+tooltip; light confirm.
**Notes:** Confirmed there is no toast library in the app, so all feedback is inline/tooltip/dialog. Approved all four mockups.

---

## Project transfer UX

### Target-workspace pick
| Option | Description | Selected |
|--------|-------------|----------|
| "Move to workspace" submenu | Submenu in the ⋯ menu; workspaces as radio items, current one checked/disabled. Reuses DropdownMenuSub + RadioGroup. | ✓ |
| "Move to workspace…" dialog | Menu item opens a dialog with a Select + confirm. Extra step. | |
| You decide | No preference. | |

### View behavior on transferring the currently-open project
| Option | Description | Selected |
|--------|-------------|----------|
| Follow the project | Active workspace flips to target; moved project stays open & selected; localStorage updates. | ✓ |
| Stay in current workspace | Keep active workspace; jump to its first remaining project (or empty state). | |
| You decide | No preference. | |

**User's choice:** Submenu pick + view follows the project.
**Notes:** Transferring a non-open project just removes it from the filtered list; active workspace unchanged.

---

## Switch navigation / empty state / reload

### Landing project on switch
| Option | Description | Selected |
|--------|-------------|----------|
| First project by name | First in sidebar order (backend sorts name COLLATE NOCASE). Matches WSNAV-03 literally. | ✓ |
| Remember last-viewed per workspace | Return to last-open project; needs per-workspace state. Future enhancement. | |
| You decide | No preference. | |

### Empty-workspace board state
| Option | Description | Selected |
|--------|-------------|----------|
| Workspace-scoped empty state | "No projects in {workspace} yet" + Add-project-into-this-workspace. Adapts App.tsx empty state. | ✓ |
| Reuse global empty state | Generic "No projects yet"; reads oddly when other workspaces have projects. | |
| You decide | No preference. | |

### Reload / deep-link precedence
| Option | Description | Selected |
|--------|-------------|----------|
| URL/project wins | Set active workspace to the URL project's workspace; update localStorage. Stale saved id → default fallback. | ✓ |
| Saved active wins | localStorage authoritative; redirect away if the URL project isn't in it. | |
| You decide | No preference. | |

**User's choice:** first-by-name landing; workspace-scoped empty state; URL/project wins on reload.
**Notes:** Stale/deleted saved workspace id (and first-ever load) falls back to the default (is_default) workspace.

---

## Claude's Discretion

- Active-workspace state-management shape (localStorage-backed hook vs. React context vs. zustand) and how it's shared across switcher / sidebar filter / index redirect / transfer-follow.
- Endpoint response shapes and error codes (409 vs 422 for non-empty-delete), exact dialog copy, ✓/disabled-radio rendering, and whether the Manage dialog's add control opens the create dialog or an inline row.

## Deferred Ideas

- WSFUT-01 (workspace icon/color identity), WSFUT-02 (reorder workspaces), WSFUT-03 (bulk transfer).
- Remember-last-viewed-project per workspace (considered for switch navigation, deferred — WSNAV-03 specifies "first project").
- Filtering the sessions bar by workspace — permanently out of scope (WSBAR-01 keeps it global).
