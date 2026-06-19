# Phase 19: Sidebar Avatars & Settings Editors - Pattern Map

**Mapped:** 2026-06-19
**Files analyzed:** 6 (2 new, 4 modified)
**Analogs found:** 6 / 6

> Frontend-only phase (React 19 + Vite + Tailwind 4 + shadcn + TanStack Query). No frontend test framework — verification is `cd web && npm run build` + `cd web && npm run lint` + human visual check. Backend is unchanged: Phase 18 shipped the `icon_letters`/`icon_color` columns, the validated PATCH path, and the Go `projectPalette` source-of-truth.

---

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| NEW `web/src/components/ui/ProjectAvatar.tsx` | component (presentational primitive) | transform (props → styled DOM) | `web/src/components/ui/skeleton.tsx` (shape) + `web/src/components/ui/input.tsx` (cn-merge prop API) | role-match (small presentational primitive; no avatar exists) |
| NEW `web/src/lib/palette.ts` | utility (const data) | transform (const lookup) | `web/src/lib/time.ts` (single-export lib const/helper) | role-match |
| MODIFY `web/src/components/sidebar/ProjectSidebar.tsx` | component (sidebar render) | request-response (TanStack reads) | self (in-place extension) | exact (this file IS the pattern) |
| MODIFY `web/src/components/layout/AppLayout.tsx` | component (layout shell) | event-driven (open state + localStorage) | self (in-place edit) | exact |
| MODIFY `web/src/components/ui/sidebar.tsx` | component (shadcn primitive) | n/a (style reference) | self (read-only reference; likely NO change) | exact |
| MODIFY `web/src/components/sidebar/ProjectSettingsDialog.tsx` | component (form dialog) | CRUD (conditional PATCH) | self (extend the description/github_repo save block) | exact |

**Verified data-layer prerequisites (all already in place, Phase 18):**
- `web/src/api/types.ts` L24-26 — `Project.icon_letters: string` and `Project.icon_color: string` are present (non-null on the wire). ✓
- `web/src/api/mutations.ts` L32-66 — `useUpdateProjectSettings` already accepts optional `icon_letters` / `icon_color` and only adds them to the PATCH body when defined. ✓
- `web/src/api/agents.ts` L20-26 — `useAgentStatuses()` is the waiting source. ✓
- `internal/api/icons.go` L19-22 — `projectPalette` holds the 9 hexes the TS mirror must reproduce verbatim. ✓

---

## Pattern Assignments

### NEW `web/src/components/ui/ProjectAvatar.tsx` (component, transform)

**Analog A — primitive file shape & `cn()` prop-merge API:** `web/src/components/ui/skeleton.tsx` (whole file) + `web/src/components/ui/input.tsx`

The avatar is a leaf presentational primitive. Copy the established shadcn primitive skeleton: import `cn`, accept `className` + props, merge with `cn(base, className)`, single named export. The `rounded-md` + `bg-*` shape is exactly what `skeleton.tsx` already does.

`web/src/components/ui/skeleton.tsx` (lines 1-13) — the canonical primitive shape to copy:
```tsx
import { cn } from "@/lib/utils"

function Skeleton({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="skeleton"
      className={cn("animate-pulse rounded-md bg-muted", className)}
      {...props}
    />
  )
}

export { Skeleton }
```

`web/src/components/ui/input.tsx` (lines 5-17) — the `cn(longBaseString, className)` + spread-props convention (apply the same `data-slot` + `cn` merge pattern for the avatar):
```tsx
function Input({ className, type, ...props }: React.ComponentProps<"input">) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn("h-8 w-full ... ", className)}
      {...props}
    />
  )
}
```

**Construction (from 19-UI-SPEC.md "Component & Interaction Contract" + Typography table):**
- Shape: `rounded-md`, `flex items-center justify-center`, `text-white font-medium leading-none`, `shrink-0`.
- Fill via **inline style** (never a Tailwind class — color is a runtime hex): `style={{ backgroundColor: project.icon_color }}`. The 9 palette hues are reserved exclusively for this background (UI-SPEC Color § "Accent reserved for").
- Two sizes via a `size: "rail" | "inline"` prop: `rail` → `size-8 text-sm`; `inline` → `size-5 text-[10px]` (UI-SPEC Spacing + Typography tables; D-05).
- Letters: render `project.icon_letters` (server-guaranteed non-empty, Phase 18 D-10 `?` fallback — render whatever the field holds, never blank).
- Letter color is ALWAYS `text-white`; never derive text color from background (UI-SPEC Typography § "Letter color is ALWAYS").
- Active ring (rail only, driven by a caller-passed `isActive` / `active` boolean): `ring-2 ring-sidebar-ring ring-offset-2 ring-offset-sidebar` (D-06).
- Optional waiting dot (rail only): `size-2 rounded-full bg-amber-400 absolute -top-0.5 -right-0.5`; this requires the avatar root to be `relative` when the dot is shown. **Static — never `animate-pulse`** (D-07; matches the existing chip which never pulses). Note: `skeleton.tsx` uses `animate-pulse` — the avatar must NOT; copy its shape, not its animation.

**Size-token source of truth** (19-UI-SPEC.md Spacing table): rail box `size-8` (32px, matches `sidebar.tsx` icon-mode `size-8!`), inline box `size-5` (20px), preview box `size-8`, waiting dot `size-2` (8px).

---

### NEW `web/src/lib/palette.ts` (utility, transform)

**Analog:** `web/src/lib/time.ts` (whole file) — the established `web/src/lib/*.ts` single-purpose export with a doc comment.

`web/src/lib/time.ts` (lines 1-11) — the file convention (top doc comment explaining provenance + a single named export, zero deps):
```ts
/** Age of an ISO timestamp relative to `now` (ms): "12s", "7m", "1h 5m".
 *  Lifted from QuotaIndicator so the quota footer and PR cards share one
 *  implementation (no date library — UI-SPEC: zero new npm deps). */
export function formatAgo(iso: string | null, now: number): string {
  ...
}
```

**Source of truth to mirror — `internal/api/icons.go` (lines 19-22), VERBATIM, same order, lowercase:**
```go
var projectPalette = []string{
	"#dc2626", "#ea580c", "#d97706", "#16a34a", "#0d9488",
	"#2563eb", "#4f46e5", "#7c3aed", "#db2777",
}
```

**Write as (D-13; UI-SPEC Color table):**
```ts
/** PROJECT_PALETTE mirrors internal/api/icons.go projectPalette VERBATIM
 *  (same 9 hexes, same order, lowercase). No palette endpoint (Phase 18 D-02):
 *  Go is the source of truth, this is the TS mirror — keep byte-for-byte in
 *  sync. Source AND only legal background for <ProjectAvatar>. White text fixed. */
export const PROJECT_PALETTE = [
  "#dc2626", "#ea580c", "#d97706", "#16a34a", "#0d9488",
  "#2563eb", "#4f46e5", "#7c3aed", "#db2777",
] as const;
```
Use `as const` so the swatch grid gets a tuple of literal hex strings. Never hardcode a palette hex anywhere else (UI-SPEC: "never hardcode a hex elsewhere").

---

### MODIFY `web/src/components/sidebar/ProjectSidebar.tsx` (component, request-response)

**Analog:** self — the file already has every pattern the phase extends. The waiting map, the `isActive` check, the row `Link`, the amber chip, and the `side="right"` tooltip all live here.

**Switch to icon-collapse (D-01):** change `<Sidebar>` (line 45) to `<Sidebar collapsible="icon">`. The 3rem rail + `group-data-[collapsible=icon]` states are built into `ui/sidebar.tsx` (see that file's section below). Do NOT hand-roll a collapsed render.

**Waiting map — reuse as-is for BOTH the expanded chip and the collapsed dot** (lines 33-42):
```tsx
// D-49: count agents (not tasks-with-sessions) waiting for input, per project.
const waitingByProject = new Map<number, number>();
for (const entry of agentStatuses ?? []) {
  if (entry.status === "waiting") {
    waitingByProject.set(
      entry.projectId,
      (waitingByProject.get(entry.projectId) ?? 0) + 1,
    );
  }
}
```

**Active check (D-06, ICON-07)** (line 65) — already present; reuse the same boolean to drive the avatar ring in the rail and the inline avatar in the expanded row:
```tsx
isActive={projectId === String(project.id)}
```

**Expanded row — preserve the existing name span + amber count chip UNCHANGED (D-08, ICON-10)** (lines 68-86). Add `<ProjectAvatar size="inline" .../>` BEFORE the name span; do NOT add the dot here (would double-signal):
```tsx
<Link to={`/projects/${project.id}`}>
  <span className="min-w-0 flex-1 truncate">
    {project.name}
  </span>
  {count > 0 && (
    <span
      aria-label={
        count === 1
          ? "1 agent waiting for input"
          : `${count} agents waiting for input`
      }
      className="ml-auto inline-flex min-w-[18px] shrink-0 items-center justify-center rounded-full bg-amber-400/10 px-1 text-xs font-medium text-amber-400 tabular-nums"
    >
      {count}
    </span>
  )}
</Link>
```
**Reuse the exact `aria-label` copy** (`"1 agent waiting for input"` / `"${count} agents waiting for input"`) on the collapsed rail avatar/link too (D-07; UI-SPEC Copywriting).

**Tooltip — already imported and used here with `side="right"` (ICON-08)** (lines 18-22 import; lines 48-53 usage):
```tsx
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
// ...
<Tooltip>
  <TooltipTrigger asChild>
    <SidebarTrigger aria-label="Toggle sidebar" />
  </TooltipTrigger>
  <TooltipContent side="right">Toggle sidebar (Ctrl+B)</TooltipContent>
</Tooltip>
```
Wrap each collapsed rail avatar in this same `Tooltip` / `TooltipTrigger asChild` / `TooltipContent side="right"` structure, content = `{project.name}` (full, untruncated). `TooltipProvider delayDuration={0}` is already set in `AppLayout`.

**Re-expand toggle at top of rail (D-02):** the `SidebarHeader` already contains the `SidebarTrigger` in a `side="right"` tooltip (lines 46-54) — this header is what shows at the top of the rail. Confirm the trigger remains visible in icon mode (it sits in `SidebarHeader`, which is not `group-data-[collapsible=icon]:hidden`). The footer Add/Settings should be hidden in icon mode (D-03) — the existing `SidebarFooter` (lines 95-125) is the block to gate with `group-data-[collapsible=icon]:hidden`.

**Note:** `cn` is already imported (line 7) for conditional class composition.

---

### MODIFY `web/src/components/layout/AppLayout.tsx` (component, event-driven)

**Analog:** self.

**Retire `CollapsedSidebarTrigger` (D-02)** — delete the whole helper (lines 23-37) and its usage inside `<main>` (line 67). The 3rem icon rail now occupies layout space and `ProjectSidebar`'s header trigger is the re-expand entry point.

**Drop `<main>`'s collapsed `pl-9` (D-02)** (lines 60-66) — once the floating trigger is gone and the rail occupies space, `<main>` no longer needs collapsed padding:
```tsx
<main
  className={
    open
      ? "relative flex-1 overflow-hidden"
      : "relative flex-1 overflow-hidden pl-9"   // <- remove the pl-9 branch
  }
>
```

**KEEP exactly as-is (D-02, D-04 from v1.0):** the controlled `open` state + `localStorage["kangent.sidebar"]` (lines 17, 39-50), the `SidebarProvider open/onOpenChange` wiring (lines 54-58), the `--sidebar-width: 15rem` style, `TooltipProvider delayDuration={0}` (line 53), and the `<ActiveSessionsBar />` mount (line 74):
```tsx
const SIDEBAR_STORAGE_KEY = "kangent.sidebar";
// ...
const [open, setOpen] = useState(
  () => localStorage.getItem(SIDEBAR_STORAGE_KEY) !== "false",
);
function handleOpenChange(next: boolean) {
  setOpen(next);
  localStorage.setItem(SIDEBAR_STORAGE_KEY, String(next));
}
```
After removing `CollapsedSidebarTrigger`, prune any now-unused imports (`useSidebar`, `SidebarTrigger`, the `Tooltip*` set) if no longer referenced — the lint/build step will catch dangling imports.

---

### MODIFY `web/src/components/ui/sidebar.tsx` (shadcn primitive — style REFERENCE, likely no change)

**Analog:** self. This is the read-only contract the rail styles against. The collapsible="icon" machinery already exists; expected change is minimal/none.

**Collapsible prop + rail width (already supports `"icon"`):**
- L154 default `collapsible = "offcanvas"`; L162 type `collapsible?: "offcanvas" | "icon" | "none"`. Passing `collapsible="icon"` from `ProjectSidebar` is all that is needed.
- L135 sets `--sidebar-width-icon` (SIDEBAR_WIDTH_ICON = 3rem); L224-225 / L235-236 collapse the rail to `--sidebar-width-icon`.

**The icon-mode hit target your rail avatar must match — `sidebarMenuButtonVariants` base (L469):**
```
... group-data-[collapsible=icon]:size-8! group-data-[collapsible=icon]:p-2! ...
... data-active:bg-sidebar-accent data-active:font-medium data-active:text-sidebar-accent-foreground ...
```
The collapsed `SidebarMenuButton` becomes `size-8` (32px) with `p-2` — which is exactly the `rail` avatar box size (`size-8`, D-05/UI-SPEC). Render `<ProjectAvatar size="rail">` inside the existing `SidebarMenuButton asChild` `<Link>` so it fills the rail hit target. `data-active:` styling is the EXPANDED active treatment (kept, D-06); the rail active marker is the avatar's own `ring-2 ring-sidebar-ring` (added in `ProjectAvatar`).

**Hide-in-icon-mode utility** to reuse for the footer (D-03) and any expanded-only chrome — the primitive uses this class throughout (e.g. L575 chip, L627 submenu, L669):
```
group-data-[collapsible=icon]:hidden
```

**`SidebarContent` keeps rail overflow scrolling (UI-SPEC rail-overflow discretion)** (L373) — already `overflow-auto` + `no-scrollbar`; no change:
```
no-scrollbar flex min-h-0 flex-1 flex-col gap-0 overflow-auto group-data-[collapsible=icon]:overflow-hidden
```
Note `group-data-[collapsible=icon]:overflow-hidden` on `SidebarContent` — if the rail needs to scroll with many projects, the scroll container is the inner `SidebarMenu`/wrapper, not `SidebarContent`. Verify during implementation; this may be the ONE place a primitive tweak is needed, otherwise treat the file as read-only.

---

### MODIFY `web/src/components/sidebar/ProjectSettingsDialog.tsx` (component, CRUD)

**Analog:** self — the description/github_repo conditional-save block is the EXACT template for the icon fields (D-12).

**Conditional PATCH save (D-12) — copy the `repoChanged` pattern for `icon_letters`/`icon_color`** (lines 91-119):
```tsx
async function handleSubmit(event: FormEvent<HTMLFormElement>) {
  event.preventDefault();
  setError(null);
  const repoChanged =
    integrationOn && repo.trim() !== (project.github_repo ?? "");
  try {
    await updateSettings.mutateAsync({
      id: project.id,
      description,
      github_repo: repoChanged ? repo.trim() : undefined,
      // NEW: same omitted-=-untouched shape (mutations.ts only sends defined keys)
      // icon_letters: lettersChanged ? letters : undefined,
      // icon_color:   colorChanged   ? color   : undefined,
    });
    onOpenChange(false);            // 2xx always closes
  } catch (err) {
    if (err instanceof ApiError && err.status < 500) {
      setError(sentenceCase(err.message));   // 400 -> inline destructive alert
    } else {
      setError(`Couldn't save. Try again.`); // network/5xx
    }
  }
}
```
The mutation already wires both icon fields (`web/src/api/mutations.ts` L41-59) — sending `undefined` omits the key, sending a value PATCHes it.

**Local-state + render-time reset on open — copy the "adjust state on prop change" pattern** (lines 46-89) so the dialog drafts `letters`/`color` and resets them each open without `useEffect`:
```tsx
const [description, setDescription] = useState(project.description);
const [repo, setRepo] = useState(project.github_repo ?? "");
const [prevOpen, setPrevOpen] = useState(open);
if (open !== prevOpen) {
  setPrevOpen(open);
  if (open) {
    setDescription(project.description);   // reset + prefill each open
    setRepo(project.github_repo ?? "");
    setRepoEdited(false);
    setError(null);
  }
}
```
Add `const [letters, setLetters] = useState(project.icon_letters)` and `const [color, setColor] = useState(project.icon_color)` and reset them in the same `if (open)` block.

**Inline 400 error styling — reuse verbatim (D-12; empty letters → 400)** (lines 181-184):
```tsx
{error !== null ? (
  <p className="rounded-md border border-destructive/50 bg-destructive/10 px-3 py-2 text-xs text-destructive">
    {error}
  </p>
) : (
  <p className="text-xs text-muted-foreground">{repoHelp}</p>
)}
```
Server message is sentence-cased via the existing `sentenceCase()` helper (lines 33-36).

**Field label + help layout — copy the existing field block** (lines 131-159): `<div className="flex flex-col gap-2">` wrapping a `<Label className="text-xs font-medium">` + control + `<p className="text-xs text-muted-foreground">` help. Form root is `flex flex-col gap-4` (line 130). Reuse `Label` + `Input` (already imported, lines 15-16).

**Letters editor (D-10, ICON-11):** an `<Input maxLength={2}>` with live client normalization mirroring the server (`internal/api/icons.go` `validateIconLetters`, L133-150): `.toUpperCase()`, strip non-alphanumerics, keep ≤2. Server stays the enforcer. Copy text: label `"Initials"`, help `"Two letters shown on the project avatar."` (UI-SPEC Copywriting). Empty allowed in field but Save disabled / surfaces the 400 (≥1 required). The Save button already disables on pending (line 206: `disabled={updateSettings.isPending}`) — extend the condition with the empty-letters guard.

**Color editor (D-11, ICON-12):** a grid mapping `PROJECT_PALETTE` to `<button type="button">` swatches: `size-7 rounded-md`, `style={{ backgroundColor: hex }}`, `aria-label` = hex/hue, current selection = `ring-2 ring-sidebar-ring` + a centered lucide `<Check>` (white), `aria-pressed` on the selected one, `focus-visible:ring-2 ring-sidebar-ring`. Layout: `flex flex-wrap gap-2` (single row of 9) OR `grid grid-cols-9 gap-1` (UI-SPEC swatch-grid discretion). Import `Check` from `lucide-react` (named import — same convention as `Loader2` in `AddProjectDialog.tsx`, `MoreHorizontal` in `ProjectMenu.tsx`).

**Live preview (D-09):** `<ProjectAvatar size="rail">` at the top of the dialog driven by local `letters`/`color` (not the saved project), so it updates as the user types/picks.

---

## Shared Patterns

### Avatar component reuse (the ICON-10 integration point)
**Source:** NEW `web/src/components/ui/ProjectAvatar.tsx`
**Apply to:** `ProjectSidebar.tsx` (rail `size="rail"` + inline `size="inline"`), `ProjectSettingsDialog.tsx` (preview `size="rail"`).
ONE component, three call sites. Single `size` prop switches box + glyph size. The `ProjectAvatar` is the only place that ever renders a palette hex as a background.

### Palette const (single source, mirrored)
**Source:** NEW `web/src/lib/palette.ts` ← mirrors `internal/api/icons.go` L19-22.
**Apply to:** the swatch grid in `ProjectSettingsDialog.tsx` (iterate `PROJECT_PALETTE`). The avatar background comes from `project.icon_color` (already a palette member, server-validated) — the grid is the only consumer of the full list. Keep byte-for-byte in sync with Go; no endpoint.

### Waiting source (one query, two renderings)
**Source:** `web/src/api/agents.ts` `useAgentStatuses()` (L20-26) → `waitingByProject` map (`ProjectSidebar.tsx` L33-42).
**Apply to:** expanded count chip (kept, D-08) AND collapsed amber dot (new, D-07). Same map, two visual treatments. Reuse the exact a11y copy string in both.

### `cn()` class merge
**Source:** `web/src/lib/utils.ts` (`clsx` + `tailwind-merge`).
**Apply to:** every new/edited component for conditional + override-safe classes (`ProjectAvatar` size/active branches, swatch selected state). Already imported in `ProjectSidebar.tsx` (L7).

### `side="right"` Tooltip
**Source:** `web/src/components/ui/tooltip.tsx` (primitive) + usage in `ProjectSidebar.tsx` L48-53.
**Apply to:** collapsed rail avatars (ICON-08). `TooltipProvider delayDuration={0}` is set once in `AppLayout` (L53) — do not re-wrap.

### Conditional PATCH (omitted = untouched)
**Source:** `web/src/api/mutations.ts` `useUpdateProjectSettings` L32-66 + the `repoChanged` guard in `ProjectSettingsDialog.tsx` L98-105.
**Apply to:** the new `icon_letters` / `icon_color` save. 2xx → `onOpenChange(false)`; 400 → `sentenceCase(err.message)` inline; 5xx/network → `"Couldn't save. Try again."`.

### shadcn primitive shape (for the new avatar)
**Source:** `web/src/components/ui/skeleton.tsx` + `web/src/components/ui/input.tsx`.
**Apply to:** `ProjectAvatar.tsx` — `data-slot`, `cn(base, className)`, spread props, single named export. Copy the shape, NOT skeleton's `animate-pulse` (the avatar and its dot are static).

---

## No Analog Found

None. Every file in scope has a strong analog (the four modified files are their own pattern; the two new files map cleanly to existing `ui/*` primitives and `lib/*` consts).

---

## Metadata

**Analog search scope:** `web/src/components/ui/`, `web/src/components/sidebar/`, `web/src/components/layout/`, `web/src/lib/`, `web/src/api/`, `internal/api/`
**Files scanned (read in full or targeted):** `ProjectSidebar.tsx`, `AppLayout.tsx`, `ProjectSettingsDialog.tsx`, `ui/sidebar.tsx` (targeted: L300-370, L460-530 + grep), `ui/skeleton.tsx`, `ui/input.tsx`, `ui/label.tsx`, `ui/tooltip.tsx`, `api/types.ts`, `api/mutations.ts`, `api/agents.ts`, `lib/utils.ts`, `lib/time.ts`, `internal/api/icons.go`
**Stack confirmations:** lucide-react named imports in use (`Loader2`, `MoreHorizontal`); `Check` follows the same convention. `cn` = `clsx`+`tailwind-merge`. No new shadcn installs (UI-SPEC Registry Safety).
**Pattern extraction date:** 2026-06-19
