# Requirements: Kangent — v1.7 Project Icons in Collapsed Sidebar

**Defined:** 2026-06-19
**Core Value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.

## v1.7 Requirements

Give every project a colored monogram avatar (two uppercase letters on a background color) so projects are identifiable and switchable directly from the collapsed sidebar — today the offcanvas sidebar slides fully off-screen, leaving no project reference. The avatar is shown both as the collapsed-rail icon and beside the name when expanded; letters and color are auto-derived at creation and editable in Project settings. Mostly a frontend milestone; the only backend work is two new `projects` columns + migration/backfill and the PATCH path to edit them.

### Project Icon Identity

- [x] **ICON-01**: Every project has a monogram avatar — two uppercase letters on a colored background — created automatically when the project is added, with no extra user action required.
- [x] **ICON-02**: A new project's two letters are derived from its name automatically — the first letters of the first two words for multi-word names ("My Cool App" → "MC"), or the first two letters for single-word names ("kangent" → "KA"), uppercased.
- [x] **ICON-03**: A new project's background color is assigned at random from a curated, dark-theme-friendly palette (legible against the avatar's text) and stays stable for that project across sessions.
- [x] **ICON-04**: A project's letters and color are stored in SQLite and persist across restarts; all projects that existed before this feature are backfilled with derived letters and an assigned palette color by the migration.

### Collapsed Sidebar Rail

- [ ] **ICON-05**: When the sidebar is collapsed, the user sees a vertical rail of project avatars instead of an empty / off-screen sidebar.
- [ ] **ICON-06**: User can click a project's avatar in the collapsed rail to switch to that project, without first expanding the sidebar.
- [ ] **ICON-07**: The currently-open project's avatar is visually marked as active (selected) in the collapsed rail.
- [ ] **ICON-08**: Hovering a collapsed avatar shows the full project name in a tooltip (since the text label is hidden when collapsed).
- [ ] **ICON-09**: A project with one or more agents waiting for input shows the amber waiting indicator as a badge overlaid on its collapsed avatar.

### Expanded Sidebar

- [ ] **ICON-10**: The same monogram avatar appears beside the project name in the expanded sidebar, with the existing name display and waiting-count chip behavior preserved.

### Editing in Project Settings

- [ ] **ICON-11**: User can edit a project's two letters in Project settings; input is normalized/validated to at most two uppercase characters.
- [ ] **ICON-12**: User can change a project's color in Project settings by picking from the curated palette swatches, with the current color clearly indicated.

## Future Requirements

Acknowledged but deferred — not in the v1.7 roadmap.

### Project Icons

- **ICON-FUT-01**: Upload an image/logo as the project icon instead of a letter monogram.
- **ICON-FUT-02**: Use an emoji as the project icon.
- **ICON-FUT-03**: Free-form/custom color (full hex picker) beyond the curated palette swatches.
- **ICON-FUT-04**: Drag-to-reorder projects in the collapsed rail (and sidebar).
- **ICON-FUT-05**: Make Add-project (+) and Settings (gear) reachable as icons in the collapsed rail so it is fully functional without expanding.

## Out of Scope

Explicitly excluded for v1.7, with reasoning (kept in Future where they may return).

| Feature | Reason |
|---------|--------|
| Free-form hex / arbitrary color picker | User chose a curated palette with preset swatches for visual consistency with the zinc dark theme and guaranteed text contrast. Parked as ICON-FUT-03. |
| Image / logo / emoji icons | Letter monograms only this milestone — simplest, no upload/storage surface. Parked as ICON-FUT-01/02. |
| More than two letters in the monogram | Fixed two-character monogram keeps the glyph legible at the small collapsed-rail size. |
| Add-project / Settings actions in the collapsed rail | User did not select them; both stay reachable by expanding the sidebar (the reopen trigger remains). Parked as ICON-FUT-05. |
| Reordering projects from the rail | Project order is unchanged this milestone. Parked as ICON-FUT-04. |
| Per-project theme/accent beyond the avatar (e.g. tinting the board) | Scope is the sidebar avatar identity only. |

## Traceability

Which phases cover which requirements. Populated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| ICON-01 | Phase 18 | Complete |
| ICON-02 | Phase 18 | Complete |
| ICON-03 | Phase 18 | Complete |
| ICON-04 | Phase 18 | Complete |
| ICON-05 | Phase 19 | Pending |
| ICON-06 | Phase 19 | Pending |
| ICON-07 | Phase 19 | Pending |
| ICON-08 | Phase 19 | Pending |
| ICON-09 | Phase 19 | Pending |
| ICON-10 | Phase 19 | Pending |
| ICON-11 | Phase 19 | Pending |
| ICON-12 | Phase 19 | Pending |

**Coverage:**
- v1.7 requirements: 12 total
- Mapped to phases: 12 ✓
- Unmapped: 0 ✓

---
*Requirements defined: 2026-06-19*
*Last updated: 2026-06-19 — roadmap created (Phases 18–19)*
