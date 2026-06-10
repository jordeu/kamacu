# Phase 1: Foundation — Projects & Board - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-10
**Phase:** 1-foundation-projects-board
**Areas discussed:** Visual direction, Task view presentation, Task creation & board flow

---

## Area Selection

| Option | Description | Selected |
|--------|-------------|----------|
| Visual direction | Theme, density, styling approach | ✓ |
| Task view presentation | Modal vs slide-over vs full page; future terminal host | ✓ |
| Task creation & board flow | Quick-add vs dialog, ordering, empty states | ✓ |
| Project setup UX | Path input, validation, naming | (left to Claude's discretion) |

---

## Visual Direction

| Option | Description | Selected |
|--------|-------------|----------|
| Dark-first (Recommended) | Dark UI matching terminal-centric purpose | ✓ |
| Light-first | Light UI; terminals as dark islands | |
| Both from day one | Theme toggle with system detection | |

| Option | Description | Selected |
|--------|-------------|----------|
| Dense developer tool | Linear/Raycast-style: compact, keyboard-friendly | ✓ |
| Relaxed kanban | Trello-style: roomier, friendlier | |
| You decide | Claude picks | |

| Option | Description | Selected |
|--------|-------------|----------|
| Mostly neutral + status accents (Recommended) | Color reserved for status badges | ✓ |
| Colored columns | Per-column accent colors | |
| You decide | Claude picks | |

| Option | Description | Selected |
|--------|-------------|----------|
| Fixed, always visible | Persistent left rail | |
| Collapsible | Toggle for more board/terminal space | ✓ |
| You decide | Claude picks | |

**User's choices:** Dark-first, dense developer tool, neutral + status accents, collapsible sidebar.

---

## Task View Presentation

| Option | Description | Selected |
|--------|-------------|----------|
| Full page (Recommended) | Task route takes over content area; URL-addressable | ✓ |
| Slide-over panel | Wide drawer over board | |
| Modal dialog | Centered overlay | |

| Option | Description | Selected |
|--------|-------------|----------|
| Tabs incl. description (Recommended) | One tab strip: Description \| Agent \| Bash... | ✓ |
| Description above, tabs below | Persistent header + session-only tabs | |
| You decide | Claude picks later | |

| Option | Description | Selected |
|--------|-------------|----------|
| Back button + Esc (Recommended) | Explicit back + keyboard + browser back | ✓ |
| Breadcrumbs | Project > Task trail | |
| You decide | Claude picks | |

**User's choices:** Full page, single tab strip with Description as a tab, back button + Esc.

---

## Task Creation & Board Flow

| Option | Description | Selected |
|--------|-------------|----------|
| Quick-add + full dialog (Recommended) | Inline title-only capture + full form button | ✓ |
| Dialog only | Single New Task form | |
| You decide | Claude picks | |

| Option | Description | Selected |
|--------|-------------|----------|
| Manual drag order (Recommended) | Drag to reorder; new tasks on top | ✓ |
| Newest first, fixed | Creation-date sort only | |
| You decide | Claude picks | |

| Option | Description | Selected |
|--------|-------------|----------|
| Edit/preview toggle (Recommended) | Textarea edit, rendered view | ✓ |
| Side-by-side editor | Live preview while typing | |
| You decide | Claude picks | |

| Option | Description | Selected |
|--------|-------------|----------|
| Confirm dialog (Recommended) | Hard delete behind confirmation | ✓ |
| Soft delete/archive | Archive view added to scope | |
| You decide | Claude picks | |

**User's choices:** Quick-add + dialog, manual drag order, edit/preview toggle, confirm-dialog deletion.

---

## Claude's Discretion

- Project setup UX (path entry, validation feedback, default naming)
- Server startup behavior (port, flags, auto-open browser)
- Keyboard shortcuts beyond Esc
- Component library specifics (follow research: shadcn/ui + Tailwind 4)
- Empty states

## Deferred Ideas

None — discussion stayed within phase scope.
