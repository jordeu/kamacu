---
phase: 19-sidebar-avatars-settings-editors
verified: 2026-06-19T00:00:00Z
status: passed
score: 9/9 must-haves verified
overrides_applied: 0
re_verification:
  previous_status: none
  note: initial verification
---

# Phase 19: Sidebar Avatars & Settings Editors Verification Report

**Phase Goal:** The user can identify and switch between projects directly from the collapsed sidebar via colored monogram avatars (which also appear beside the name when expanded), and can edit a project's letters and color from Project settings.

**Verified:** 2026-06-19
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

The phase goal decomposes into one shared `<ProjectAvatar>` primitive consumed at three call sites (collapsed rail, expanded inline, settings preview) plus letters + swatch editors in Project settings. All four ROADMAP success criteria are observably enabled in the AS-BUILT code. Verification was performed against the post-UAT code (the three user-approved visual-revision rounds that amended the original UI-SPEC: avatar shape `rounded-md`→`rounded-full`; active state moved from an avatar ring to the rail `SidebarMenuButton`'s filled `bg-sidebar-accent` row, with the `active` prop removed from `ProjectAvatar`; palette re-muted in both `internal/api/icons.go` and `web/src/lib/palette.ts` with migration `00010` remapping existing rows).

### Observable Truths

| #   | Truth (merged ROADMAP SC + PLAN must_haves) | Status | Evidence |
| --- | ------------------------------------------- | ------ | -------- |
| 1 | Collapsed sidebar shows a vertical rail of project monogram avatars instead of an empty/off-screen sidebar (ICON-05, D-01) | ✓ VERIFIED | `ProjectSidebar.tsx:46` `<Sidebar collapsible="icon">`; rail `<ProjectAvatar size="rail">` at L96-102 shown via `hidden group-data-[collapsible=icon]:flex` (L95). Replaces the prior `offcanvas`/`w-0` behavior. |
| 2 | Clicking a rail avatar switches to that project without expanding the sidebar (ICON-06) | ✓ VERIFIED | Rail avatar is wrapped in `<Link to={`/projects/${project.id}`}>` (L81-82) inside `SidebarMenuButton asChild`; navigation is a route change, not a sidebar-open toggle. Human-verify (Plan 02 Task 3) approved live. |
| 3 | The currently-open project's rail avatar is visually marked active (ICON-07, UAT-amended D-06) | ✓ VERIFIED | `isActive = projectId === String(project.id)` (L63) → `SidebarMenuButton isActive` (L78) → `data-active={isActive}` (`ui/sidebar.tsx:511`) → `data-active:bg-sidebar-accent` filled-row style (`ui/sidebar.tsx:469`). The icon-mode `size-10! rounded-md` override (L79) makes it a 40px filled row. `active` prop removed from `ProjectAvatar` per UAT (grep count 0). Satisfies "visually marked active". |
| 4 | Hovering a rail avatar shows the full project name in a `side="right"` tooltip (ICON-08) | ✓ VERIFIED | Rail avatar wrapped in `Tooltip`/`TooltipTrigger asChild`/`TooltipContent side="right"` (L93-108) with content `{project.name}`. `TooltipProvider delayDuration={0}` set once in `AppLayout.tsx:24`. |
| 5 | A project with ≥1 waiting agent shows a static amber badge on its collapsed avatar; the expanded count chip is unchanged (ICON-09, D-07/D-08) | ✓ VERIFIED | `waiting={count > 0}` (L100) → static `size-2 rounded-full bg-amber-400` overlay dot (`ProjectAvatar.tsx:65-69`, `aria-hidden`, never `animate-pulse`). Expanded amber count chip preserved at L123-130, hidden in icon mode; no dot added to the inline avatar (D-08). |
| 6 | The same monogram avatar appears inline beside the name when expanded, with name + count chip preserved (ICON-10) | ✓ VERIFIED | `<ProjectAvatar size="inline">` at L112-117 before the truncated name span (L118-120); `className="group-data-[collapsible=icon]:hidden"` so inline shows only when expanded. Name span and count chip unchanged. |
| 7 | User can edit two letters, normalized client-side to ≤2 uppercase alphanumerics mirroring the server rule (ICON-11, D-10) | ✓ VERIFIED | `normalizeIconLetters` (`ProjectSettingsDialog.tsx:54-57`): `/[\p{L}\p{Nd}]/gu` match → `slice(0,2)` → `toUpperCase()`, mirroring Go `validateIconLetters` (`icons.go:137-154`). `<Input maxLength={2}>` (L178), label "Initials" (L174), help copy (L183-185). Save disabled when `letters.trim() === ""` (L292). |
| 8 | User can pick from the 9 curated palette swatches; current color clearly indicated by ring + check (ICON-12, D-11) | ✓ VERIFIED | `PROJECT_PALETTE.map` swatch grid (L191-211): `size-7 rounded-md`, inline `backgroundColor`, `aria-label`, `focus-visible:ring-2`, `onClick → setColor(hex)`. Selected swatch (`hex.toLowerCase() === color.toLowerCase()`) gets `ring-2 ring-sidebar-ring` + centered white `<Check>` + `aria-pressed`. No hardcoded hex (grep 0); no free-form input (ICON-FUT-03 deferred). |
| 9 | A live `<ProjectAvatar>` preview reflects in-progress letters + color; saved changes propagate via the existing conditional PATCH (D-09, D-12) | ✓ VERIFIED | `<ProjectAvatar size="rail" letters={letters} color={color}>` driven by local draft state (L166). `handleSubmit` sends `icon_letters`/`icon_color` only when changed (L128-129, L135-136) through `useUpdateProjectSettings` (which drops undefined keys, `mutations.ts:58-59`). 2xx closes (L139); 400 surfaces inline (L141-149). Rail/expanded read the same `project.*` fields → save propagates. Human-verify (Plan 03 Task 3) approved live. |

**Score:** 9/9 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `web/src/lib/palette.ts` | `PROJECT_PALETTE` = 9 hexes mirroring Go verbatim | ✓ VERIFIED | 9 lowercase hexes `as const`; byte-for-byte equal to `icons.go projectPalette` (automated diff: MATCH true, count 9/9). Provenance doc comment present. |
| `web/src/components/ui/ProjectAvatar.tsx` | Shared monogram primitive (rail\|inline, waiting dot) | ✓ VERIFIED | Exports `ProjectAvatar`; inline `backgroundColor` (never bg-* class); `size-6`/`size-5` sizes; static amber dot; `text-white font-medium`; no `animate-pulse`. `active` prop removed per UAT. Wired at 3 call sites. |
| `web/src/components/sidebar/ProjectSidebar.tsx` | collapsible=icon rail + inline avatars; footer hidden | ✓ VERIFIED | All grep gates pass: `collapsible="icon"`, `size="rail"`, `size="inline"`, `group-data-[collapsible=icon]:hidden` (×5), `waitingByProject` (×4), `side="right"` (×3). |
| `web/src/components/layout/AppLayout.tsx` | Floating trigger removed, pl-9 dropped, persistence kept | ✓ VERIFIED | `CollapsedSidebarTrigger` 0, `pl-9` 0; `kangent.sidebar` localStorage + `delayDuration={0}` preserved; `<main className="relative flex-1 overflow-hidden">`. No dangling imports (build green). |
| `web/src/components/sidebar/ProjectSettingsDialog.tsx` | Initials input + 9-swatch grid + live preview + conditional PATCH | ✓ VERIFIED | All grep gates pass; iterates `PROJECT_PALETTE` from `@/lib/palette`; no hardcoded hex. |
| `internal/api/icons.go` | Muted `projectPalette` source of truth | ✓ VERIFIED | 9 muted hexes; `go build`/`vet`/`test ./internal/api` all green. |
| `internal/store/migrations/00010_muted_palette.sql` | Positional remap of existing rows + safe Down | ✓ VERIFIED | 9 `UPDATE` statements remap old→new in palette order (automated check: matches palette order). Down is documented intentional NO-OP (`SELECT 1;`) — CR-01 fix in `ecc233e`. |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | --- | --- | ------ | ------- |
| `ProjectAvatar.tsx` | `project.icon_color` | inline `style={{ backgroundColor: color }}` | ✓ WIRED | L55; never a Tailwind class. |
| `palette.ts` | `icons.go projectPalette` | verbatim TS mirror | ✓ WIRED | Byte-for-byte equal (automated). |
| `ProjectSidebar.tsx` | `ProjectAvatar` | import + rail/inline call sites | ✓ WIRED | Import L23; used ×3 (rail, inline). |
| `ProjectSidebar.tsx` | `waitingByProject` map | one `useAgentStatuses`-derived count → rail dot AND chip | ✓ WIRED | Single map L35-43; reused for `waiting` (L100) and chip (L123); no second query. |
| `AppLayout.tsx` | `localStorage kangent.sidebar` | controlled open state kept as-is | ✓ WIRED | L8, L14-21 unchanged. |
| `ProjectSettingsDialog.tsx` | `useUpdateProjectSettings` | conditional PATCH (icon_letters/icon_color only when changed) | ✓ WIRED | L128-137; mutation drops undefined keys. |
| `ProjectSettingsDialog.tsx` | `PROJECT_PALETTE` | swatch grid maps 9 hexes | ✓ WIRED | L20 import; L191 map. |
| `ProjectSettingsDialog.tsx` | `ProjectAvatar` | live preview driven by local draft | ✓ WIRED | L166 driven by `letters`/`color` state. |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| Rail/inline avatar | `project.icon_letters`, `project.icon_color` | `useProjects()` query → Go API (server-validated, Phase 18) | Yes — real per-project fields, not hardcoded | ✓ FLOWING |
| Rail waiting dot | `count` from `waitingByProject` | `useAgentStatuses()` query, derived map | Yes — live agent status; dot conditional on `count > 0` | ✓ FLOWING |
| Settings live preview | `letters`/`color` local draft | `useState(project.*)`, re-seeded on open | Yes — reflects edits in real time | ✓ FLOWING |
| Settings save | conditional PATCH body | `useUpdateProjectSettings.mutateAsync` | Yes — sends changed fields to validated PATCH | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Palette TS↔Go byte-for-byte sync | node hex-diff palette.ts vs icons.go | MATCH true, 9/9 | ✓ PASS |
| Migration remaps in palette order | node hex-extract 00010_muted_palette.sql | matches palette order true | ✓ PASS |
| Go API package builds + tests | `go build ./...` / `go test ./internal/api/...` | build exit 0, test ok | ✓ PASS |
| No hardcoded palette hex in settings | grep muted hexes in dialog | 0 matches | ✓ PASS |
| Frontend build (orchestrator) | `cd web && npm run build` | exit 0 | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| ICON-05 | 19-02 | Collapsed rail of avatars instead of empty sidebar | ✓ SATISFIED | Truth 1 |
| ICON-06 | 19-02 | Click rail avatar to switch without expanding | ✓ SATISFIED | Truth 2 |
| ICON-07 | 19-01, 19-02 | Active project visually marked in rail | ✓ SATISFIED | Truth 3 (UAT-amended: filled-row accent) |
| ICON-08 | 19-02 | Hover shows full name in tooltip | ✓ SATISFIED | Truth 4 |
| ICON-09 | 19-01, 19-02 | Waiting badge overlaid on collapsed avatar | ✓ SATISFIED | Truth 5 |
| ICON-10 | 19-01, 19-02 | Same avatar inline beside name when expanded | ✓ SATISFIED | Truth 6 |
| ICON-11 | 19-03 | Edit two letters, normalized to ≤2 uppercase | ✓ SATISFIED | Truth 7 |
| ICON-12 | 19-01, 19-03 | Pick color from curated swatches, current indicated | ✓ SATISFIED | Truth 8 |

All 8 phase requirement IDs (ICON-05..12) are claimed by at least one plan and mapped to Phase 19 in REQUIREMENTS.md. No orphaned requirements — every ID in REQUIREMENTS.md's Phase 19 set appears in a plan's `requirements` frontmatter.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| — | — | None found | — | All 7 phase files clean of TBD/FIXME/XXX/HACK/PLACEHOLDER/stub-prose. No empty-data stubs; the migration Down `SELECT 1;` is a documented intentional NO-OP (CR-01 reviewed/accepted), not a stub. |

### Human Verification Required

None outstanding. The three blocking `checkpoint:human-verify` gates (Plan 02 Task 3 rail appearance, Plan 03 Task 3 editor flow + edit→rail propagation) were RUN and APPROVED live by the user, including three UAT visual-revision rounds that amended the UI-SPEC. Per the verification context, the runtime/visual dimension is satisfied and code evidence for each amended decision is present in the AS-BUILT code.

### Gaps Summary

No gaps. The phase goal is achieved in the codebase: one shared `<ProjectAvatar>` primitive is consumed at all three call sites (collapsed rail with filled-row active highlight + side=right name tooltip + static amber waiting dot, expanded inline avatar beside the unchanged name + count chip, and the settings live preview); the Project-settings dialog has a working Initials input (client normalization mirroring the Go server rule) and a 9-swatch `PROJECT_PALETTE` color grid (current swatch ring + check), both wired into the existing conditional PATCH so edits propagate to every avatar surface. The palette is byte-for-byte in sync across Go source, TS mirror, and the remap migration. Build, vet, test, and lint are green; the one critical code-review finding (CR-01 migration Down) was fixed in `ecc233e`.

---

_Verified: 2026-06-19_
_Verifier: Claude (gsd-verifier)_
