# Phase 19: Sidebar Avatars & Settings Editors - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-19
**Phase:** 19-sidebar-avatars-settings-editors
**Areas discussed:** Collapsed rail mechanism, Avatar component design, Waiting badge on avatar, Settings editors layout

---

## Collapsed Rail Mechanism

| Option | Description | Selected |
|--------|-------------|----------|
| shadcn collapsible="icon" | Set <Sidebar collapsible="icon">, restyle the 3rem icon-state rail to show avatars; re-expand toggle at top of rail (retire floating CollapsedSidebarTrigger, main drops pl-9); Add/Settings hidden when collapsed | ✓ |
| Custom collapsed render | Hand-built thin rail when !open; full control but reimplements what collapsible="icon" gives free | |

**User's choice:** shadcn collapsible="icon" (Recommended)
**Notes:** Keeps AppLayout's controlled open + localStorage["kangent.sidebar"]. Add(+)/Settings(gear) deferred from the rail (ICON-FUT-05), reachable by expanding.

---

## Avatar Component Design

| Option | Description | Selected |
|--------|-------------|----------|
| Rounded-square, two sizes | rounded-md monogram (project/workspace-icon style); rail ~size-8 (32px), inline ~size-5 (20px); bold white auto-scaled letters; active rail = ring-2 | ✓ |
| Circle, two sizes | rounded-full; softer/avatar-like, can read as a user avatar | |
| Rounded-square, single size | one fixed size in both places; simpler but rail wants a bigger glyph than inline | |

**User's choice:** Rounded-square, two sizes (Recommended)
**Notes:** New <ProjectAvatar> (no primitive exists). One component, size prop, used in collapsed rail + inline beside expanded name (ICON-10). Active rail item ringed (ICON-07).

---

## Waiting Badge on Avatar

| Option | Description | Selected |
|--------|-------------|----------|
| Static amber dot, presence-only | small static amber dot top-right of collapsed avatar; no number; matches the static expanded chip; count in aria-label | ✓ |
| Amber count badge | the actual number in a tiny amber pill on the corner; more info but cramped at rail size | |
| Pulsing amber dot | presence dot that pulses (matches ActiveSessionsBar); breaks consistency with the static expanded chip | |

**User's choice:** Static amber dot, presence-only (Recommended)
**Notes:** Source = existing waitingByProject (useAgentStatuses, status==="waiting"). Expanded sidebar keeps ONLY its existing count chip (ICON-10) — no dot on the inline avatar.

---

## Settings Editors Layout

| Option | Description | Selected |
|--------|-------------|----------|
| Live preview + swatch grid | top-of-dialog live <ProjectAvatar> preview; letters input (maxLength 2, live-uppercase, matches server ≤2-alnum rule); 9-swatch grid with current ring+check; wired into existing conditional PATCH | ✓ |
| No live preview | letters input + swatch row only, no preview avatar; less code, result only visible after save | |

**User's choice:** Live preview + swatch grid (Recommended)
**Notes:** Editors in existing ProjectSettingsDialog.tsx; reuses useUpdateProjectSettings (Phase 18 18-03 already accepts both fields, only-sent-when-changed). 400 (e.g. empty letters) surfaces inline like the repo error.

---

## Claude's Discretion

- Exact px/Tailwind size tokens, ring colors, dot size/offset, swatch grid columns.
- New file placement for <ProjectAvatar> + the TS palette const.
- Tooltip implementation for ICON-08 (reuse existing Tooltip primitive, side="right").
- Letters-empty / reset-to-derived handling beyond the ≥1 rule.
- Rail scroll behavior when many projects exceed viewport height.

## Deferred Ideas

- Add(+)/Settings(gear) in the collapsed rail — ICON-FUT-05.
- Free-form hex picker — ICON-FUT-03.
- Image/emoji icons — ICON-FUT-01/02.
- Drag-to-reorder avatars — ICON-FUT-04.
- Count (vs dot) on the collapsed waiting badge — considered, rejected for rail cleanliness.
