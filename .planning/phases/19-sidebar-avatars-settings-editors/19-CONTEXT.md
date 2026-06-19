# Phase 19: Sidebar Avatars & Settings Editors - Context

**Gathered:** 2026-06-19
**Status:** Ready for planning

<domain>
## Phase Boundary

Frontend only. Build one shared monogram avatar component and use it in two
places — (a) as a clickable icon in a new collapsed-sidebar rail (active state,
name tooltip, amber waiting badge) and (b) inline beside the project name when
the sidebar is expanded — plus add letters + color-swatch editors to the
existing Project settings dialog.

**In scope (ICON-05..12):** switch the sidebar's collapsed render from the
off-screen offcanvas to an avatar rail (ICON-05), click-to-switch-project from
the rail (ICON-06), active-project marking in the rail (ICON-07), full-name
tooltip on collapsed avatars (ICON-08), amber waiting badge on collapsed avatars
(ICON-09), the same avatar beside the name when expanded with the existing
name + waiting-count chip preserved (ICON-10), a letters editor
(normalized ≤2 uppercase) and a curated-swatch color editor (current color
clearly indicated) in Project settings (ICON-11/12).

**Out of scope:** any backend work (Phase 18 shipped the `icon_letters` /
`icon_color` columns, derivation/backfill, and the validated PATCH path);
Add(+)/Settings(gear) actions inside the collapsed rail (deferred ICON-FUT-05);
image/emoji icons, free-form hex, >2 letters, drag-reorder (all deferred).

</domain>

<decisions>
## Implementation Decisions

### Collapsed Rail Mechanism (ICON-05/06)
- **D-01:** Switch `<Sidebar>` (in `ProjectSidebar.tsx`) to shadcn's
  **`collapsible="icon"`** and restyle the icon (collapsed) state to render the
  project avatars in the built-in 3rem rail (`--sidebar-width-icon`). Do NOT
  hand-roll a custom collapsed render — shadcn already provides the rail width
  and the `group-data-[collapsible=icon]` collapsed states.
- **D-02:** Re-expand from the collapsed rail via a **toggle pinned at the top of
  the rail** (a `SidebarTrigger`/icon button). This **retires the floating
  `CollapsedSidebarTrigger`** currently rendered over `<main>` in
  `AppLayout.tsx`; since the 3rem rail now occupies layout space, `<main>` drops
  its `pl-9` collapsed padding. Keep AppLayout's controlled `open` +
  `localStorage["kangent.sidebar"]` (D-04 from v1.0) exactly as-is.
- **D-03:** When collapsed, the rail shows **project avatars only** — the footer
  Add(+) and Settings(gear) are hidden in the icon state (ICON-FUT-05 deferred).
  They remain reachable by expanding the sidebar (the re-expand toggle is the
  entry point).

### Shared Avatar Component (ICON-07/10)
- **D-04:** New component `<ProjectAvatar>` (no avatar primitive exists today).
  **Rounded-square** (`rounded-md`) monogram — reads as a project/workspace icon
  (GitHub/Slack style), not a person. Background = `project.icon_color`, text =
  fixed white, letters = `project.icon_letters`, **bold**.
- **D-05:** **Two sizes** via a `size` prop: collapsed rail ≈ `size-8` (~32px,
  fits the 3rem rail); inline-beside-the-expanded-name ≈ `size-5` (~20px).
  Letter glyph auto-scales with size (e.g. `text-sm` rail / `text-xs` inline).
  One component, used in both places (ICON-10).
- **D-06:** Active-project marking in the rail (ICON-07) = a **ring** around the
  avatar (`ring-2 ring-sidebar-ring`), driven by the existing
  `isActive`/`projectId` check. In the expanded sidebar the existing row
  active treatment (`bg-sidebar-accent`) is unchanged.

### Waiting Badge (ICON-09)
- **D-07:** On a **collapsed** avatar, a project with ≥1 waiting agent shows a
  **small static amber dot** overlaid at the top-right corner (presence-only, no
  number — clean at 32px). Static (not pulsing) to match the existing expanded
  count chip (which "never pulses"). The count is exposed via `aria-label`
  ("N agents waiting for input") for a11y. Source = the existing
  `waitingByProject` map (`useAgentStatuses()`, `status === "waiting"`).
- **D-08:** The **expanded** sidebar keeps ONLY its existing amber count chip
  (the current `ProjectSidebar.tsx` chip) — ICON-10 says preserve it. Do NOT add
  the dot badge to the inline expanded avatar (would double-signal).

### Settings Editors (ICON-11/12)
- **D-09:** Editors live in the existing `ProjectSettingsDialog.tsx`. At the top
  of the dialog, a **live `<ProjectAvatar>` preview** reflecting the in-progress
  letters + color.
- **D-10:** Letters editor = a text `<input>` with `maxLength={2}`, **live
  client-side uppercasing** + alphanumeric normalization mirroring the server
  rule (Phase 18 D-10: trim/uppercase/≤2 alphanumerics, ≥1 required). The server
  remains the enforcer; the client mirror is UX.
- **D-11:** Color editor = a **row/grid of the 9 palette swatches**; the current
  color is clearly indicated with a **ring + check** mark. Selecting a swatch sets
  `icon_color`. No free-form hex (ICON-FUT-03 deferred).
- **D-12:** Wire `icon_letters` / `icon_color` into the **existing conditional
  PATCH** in `ProjectSettingsDialog` → `useUpdateProjectSettings` (already accepts
  both from Phase 18 18-03): send each field **only when changed** (the same
  omitted-=-untouched pattern used for description/github_repo). A 2xx closes the
  dialog; a 400 (e.g. empty letters) surfaces inline like the repo error.

### Palette Mirror (carry-forward, Phase 18 D-02)
- **D-13:** Add the curated palette as a **TS const mirroring the Go
  `projectPalette` verbatim** — the 9 hexes in order:
  `#dc2626 #ea580c #d97706 #16a34a #0d9488 #2563eb #4f46e5 #7c3aed #db2777`
  (fixed white text). This is the source for the swatch grid (D-11). No palette
  endpoint (Phase 18 D-02 chose Go-const-of-truth + TS mirror). Keep the two
  lists in sync.

### Claude's Discretion
- Exact px/Tailwind size tokens, ring colors, dot size/offset, and swatch grid
  columns — tune to the zinc theme.
- New file placement for `<ProjectAvatar>` (likely `web/src/components/ui/` or
  `web/src/components/sidebar/`) and the palette const (`web/src/lib/` or beside
  the avatar).
- Tooltip implementation for ICON-08 (reuse the existing `Tooltip` primitive
  already imported in `ProjectSidebar.tsx`, `side="right"`).
- Letters-empty / reset-to-derived behavior in the editor beyond the ≥1 rule
  (server rejects empty; client can disable Save or show inline error).
- Rail scroll behavior when many projects exceed the viewport height.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & Roadmap
- `.planning/REQUIREMENTS.md` — ICON-05..12 (this phase) + Out-of-Scope table +
  Future ICON-FUT-01..05 (esp. ICON-FUT-05 = Add/Settings in the rail, deferred).
- `.planning/ROADMAP.md` § "Phase 19: Sidebar Avatars & Settings Editors" — goal
  + 4 success criteria.
- `.planning/phases/18-project-icon-data-foundation/18-CONTEXT.md` — Phase 18
  locked decisions this phase builds on (esp. D-01 palette hexes, D-02 Go-const
  source-of-truth + TS mirror, D-10/D-11 validation the editors mirror).

### Frontend code to modify / create
- `web/src/components/sidebar/ProjectSidebar.tsx` — the sidebar render; rows +
  `waitingByProject` count chip live here; add avatars + switch to
  `collapsible="icon"`.
- `web/src/components/layout/AppLayout.tsx` — `<Sidebar>` open state +
  `localStorage["kangent.sidebar"]` + `CollapsedSidebarTrigger` (to retire) +
  `--sidebar-width` + `<main>` `pl-9`.
- `web/src/components/ui/sidebar.tsx` — shadcn primitive; `collapsible` prop
  (`offcanvas|icon|none`), `--sidebar-width-icon: 3rem`, the
  `group-data-[collapsible=icon]` collapsed-state classes to restyle against.
- `web/src/components/sidebar/ProjectSettingsDialog.tsx` — add the letters +
  swatch editors + live preview; reuses `useUpdateProjectSettings`.
- `web/src/components/sidebar/ProjectMenu.tsx` — opens the settings dialog (no
  change expected; reference only).
- NEW: `<ProjectAvatar>` component + the TS palette const.

### Data / API (already in place from Phase 18)
- `web/src/api/types.ts` — `Project.icon_letters` / `icon_color` (present).
- `web/src/api/mutations.ts` — `useUpdateProjectSettings` accepts both (present).
- `web/src/api/agents.ts` — `useAgentStatuses()` (the waiting source).
- `internal/api/icons.go` — the Go `projectPalette` the TS const must mirror
  verbatim (the 9 hexes).

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `ProjectSidebar.tsx` already computes `waitingByProject` from
  `useAgentStatuses()` and renders project rows with `isActive` + a static amber
  count chip — extend it; reuse the same waiting map for the collapsed dot.
- `Tooltip`/`TooltipTrigger`/`TooltipContent` already imported and used in
  `ProjectSidebar.tsx` (`side="right"`) — reuse for ICON-08 collapsed tooltips.
- `useUpdateProjectSettings` (mutations.ts) already carries `icon_letters` /
  `icon_color` as optional, only-sent-when-defined PATCH fields (Phase 18 18-03).
- shadcn `<Sidebar>` already supports `collapsible="icon"` with a 3rem rail and
  `group-data-[collapsible=icon]` state classes — no primitive surgery needed.
- `ProjectSettingsDialog`'s conditional-save pattern (send a field only when
  changed; 400 → inline error, 2xx → close) is the exact template for the icon
  fields.

### Established Patterns
- AppLayout owns sidebar `open` via `useState` + `localStorage` (the generated
  shadcn provider only writes a cookie) — keep this; only the collapsed VISUAL
  changes (icon rail vs offcanvas).
- Sidebar active state via `projectId === String(project.id)` →
  `SidebarMenuButton isActive`.
- Waiting count = agents with `status === "waiting"` grouped by `projectId`.

### Integration Points
- `collapsible="icon"` interacts with AppLayout's `<main>` `pl-9` (added for the
  floating trigger) — removing the floating trigger + adding the rail means
  `<main>` no longer needs the collapsed padding.
- The avatar appears in both the rail (collapsed) and the expanded rows — the
  single `<ProjectAvatar size>` component is the integration point for ICON-10.

</code_context>

<specifics>
## Specific Ideas

- Avatar = rounded-square, white bold letters on `icon_color`; rail ~32px,
  inline ~20px; active rail = ring.
- Collapsed waiting = static amber dot top-right (presence-only); expanded keeps
  the existing count chip.
- Settings: live avatar preview + 2-char uppercase input + 9-swatch grid with the
  current swatch ring+check-marked.
- TS palette mirror = the 9 Go hexes verbatim, in order.

</specifics>

<deferred>
## Deferred Ideas

- Add(+) / Settings(gear) actions inside the collapsed rail — ICON-FUT-05.
- Free-form hex color picker — ICON-FUT-03 (swatches only this phase).
- Image / emoji project icons — ICON-FUT-01/02.
- Drag-to-reorder avatars in the rail — ICON-FUT-04.
- A count (vs presence dot) on the collapsed waiting badge — considered, rejected
  for rail cleanliness; the number is available when expanded.

None of these are Phase 19 work — captured so they aren't lost.

</deferred>

---

*Phase: 19-sidebar-avatars-settings-editors*
*Context gathered: 2026-06-19*
