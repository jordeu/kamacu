# Phase 11: Activity Page & Controls - Research

**Researched:** 2026-07-31
**Domain:** Frontend-only React UI — a new top-level Activity page consuming the Phase 10 `GET /api/activity` endpoint
**Confidence:** HIGH

## Summary

Phase 11 is a **frontend-only** phase. Phase 10 already shipped the combined `GET /api/activity?scope=…&window=…` endpoint (always HTTP 200, one response carrying `{tasks, reviews, stats}`), so this phase adds **zero backend, zero SQL, zero migration** — it renders that response. Every byte of the wire contract was verified by reading the Phase 10 Go source (`internal/api/activity.go`, `activity_helpers.go`, `internal/github/prlist.go`) and its tests directly, not assumed.

The work is structurally a clone-and-adapt of three existing surfaces: (1) `SettingsPage.tsx` is the page-shell template (scroll container, bounded column, loading skeletons, error+retry); (2) `WorkspaceSwitcher.tsx` is the dropdown template for the scope selector; (3) the `ProjectSidebar.tsx` `SidebarFooter` (next to the Settings gear) is the entry mount point. The one genuine structural wrinkle is **collapsed-rail reachability** (CONTEXT D-02): the footer self-hides wholesale via `group-data-[collapsible=icon]:hidden`, so the Activity entry must be lifted out of that wrapper or use the dual-surface (icon-rail + expanded) class idiom the project rows already demonstrate.

**Primary recommendation:** Build `ActivityPage.tsx` on the `SettingsPage` shell; add one `useActivity(scope, window)` TanStack query keyed `["activity", scope, window]`; mirror `useActiveWorkspace` for a `localStorage`-backed scope+window hook under the `kamacu.*` prefix; render stats as `min · median · max` rows via a new adaptive `formatDuration(seconds)` util; and branch the reviews section on `reviews.state` so it empties **quietly** when GitHub integration is off. No new npm packages.

<user_constraints>

## User Constraints (from CONTEXT.md)

> Copied verbatim from `.planning/phases/11-activity-page-controls/11-CONTEXT.md`. These are LOCKED — the planner honors them as-is.

### Locked Decisions

- **D-01:** Activity entry lives in the **sidebar footer, next to the Settings gear** (icon-button `Link` to `/activity`, active state from `location.pathname === "/activity"`).
- **D-02:** Activity renders as an icon in the **collapsed icon rail** — always reachable, NOT hidden by the footer's `group-data-[collapsible=icon]:hidden`. (Exact DOM structure is the agent's call; the *behavior* is locked.)
- **D-03:** Icon is **lucide `Activity`** (pulse/heartbeat line).
- **D-04:** Scope is a **single dropdown** shaped like `WorkspaceSwitcher` — lists Global + every workspace + each project; maps 1:1 to `scope=global|workspace:N|project:N`.
- **D-05:** **Default scope on open = the active workspace** (`useActiveWorkspace()`); fall back to Global if unresolved.
- **D-06:** **Scope + window persist in `localStorage` (no URL).** Use the `kamacu.*` prefix. Route is a bare `/activity`, never deep-linkable. Stale/missing saved value → default scope (active workspace) / `week`.
- **D-07:** Week / Month is a **segmented control** (Week | Month), next to the scope dropdown in a page-level control bar; maps to `window=week|month`.
- **D-08:** **Stats strip on top, lists below (stacked, single column).** Compact stats strip → tasks-done section → reviews-done section. Follow the `SettingsPage` single-column-scroll shell.
- **D-09:** Cycle/dwell render as compact rows: **`min · median · max`** with "over N tasks" beside each, from each `timeStat {n, min, max, median}`. Counts are the lead numbers. **No cards grid, no charts.**
- **D-10:** Durations format **adaptively to the largest 1–2 non-zero units** (d/h/m): `1d 3h`, `4h 20m`, `45m`, `30s`. Wire values are **seconds**.
- **D-11:** When `n === 0`, cycle/dwell show an **em-dash `—`** (not `0m`/`0h`); counts show `0`.
- **D-12:** When GitHub integration is off / `gh` absent (`reviews.state === "disabled"` / `"no_gh"`), the reviews section renders **QUIETLY EMPTY — no error, no hint.** Tasks/stats display normally. (`"partial"`/`"error"`: successful repos' PRs render, otherwise quiet.)
- **D-13:** Both lists **grouped by project** under a project-name subheading (client-side bucketing). Tasks already arrive `doneAt DESC`; reviews ordered `completedAt DESC` (client-side).
- **D-14 (deliberate simplification — flag at UAT):** Clicking a reviews-done entry **opens the PR on GitHub (`ReviewDoneSummary.url`) universally** — the `else the PR` fallback, NOT a cross-project in-app review-workspace lookup.
- **D-15:** Clicking a tasks-done entry **navigates to that task's view** (`/projects/:projectId/tasks/:taskId`).

### the agent's Discretion

- Route registration: `/activity` as a new top-level route under `<AppLayout />` in `App.tsx`, sibling of `/settings` (no `:projectId` in path).
- New `ActivityPage.tsx` under `web/src/pages/` mirroring `SettingsPage.tsx`.
- New `useActivity(scope, window)` TanStack query in `web/src/api/queries.ts` (or a dedicated `web/src/api/activity.ts`); `queryKey: ["activity", scope, window]`; activity wire types in `web/src/api/types.ts`.
- The scope/window UI state hook — where the persisted scope + window live (small `localStorage`-backed hook like `useActiveWorkspace`). App favors raw `localStorage` hooks over zustand for view state.
- The collapsed-rail icon structure (D-02) — one button outside the footer hide-wrapper vs. two surfaces; agent picks the cleaner structure.
- Exact copy (section headings, "over N tasks" phrasing, tooltip text), the `max-w` column width, whether the control bar is sticky.
- The duration formatter's exact unit labels (`m` vs `min`, `h` vs `hr`) and rounding.
- Grouping ordering — projects ordered by name (matches sidebar) or by most-recent-activity.

### Deferred Ideas (OUT OF SCOPE)

- Preferring the in-app review workspace over the PR URL on click (D-14 simplification).
- Custom date ranges / "All time" preset (ACTFUT-01).
- Throughput trend charts / bar graphs (ACTFUT-02).
- CSV/JSON export of activity data (ACTFUT-03).
- Per-agent or per-workspace breakdown statistics (ACTFUT-04).
- Activity for events beyond done/merged (ACTFUT-05).
- Deep-linkable scope/window via URL (D-06 chose localStorage-only).
- Time stats for reviews (STATS-04 — permanently out of scope).

</user_constraints>

<phase_requirements>

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| ACT-01 | Dedicated top-level Activity page from a persistent sidebar entry, app-wide | New `/activity` route under `<AppLayout />` (App.tsx:135 sibling pattern); new Activity icon-button in `ProjectSidebar.tsx` footer, collapsed-rail-reachable (D-01/D-02/D-03). lucide `Activity` icon verified present. |
| ACT-02 | Scope to Global / a Workspace / a Project via a selector; lists + stats update | Single dropdown mirroring `WorkspaceSwitcher.tsx` (`DropdownMenu` + `DropdownMenuCheckboxItem`/`DropdownMenuLabel`); maps to `scope=global\|workspace:N\|project:N` — the exact param strings `parseScope` accepts (activity_helpers.go:51). One query, key changes on scope. |
| ACT-03 | Toggle Week/Month (rolling from now); toggle drives lists + stats | Segmented control (`Tabs` default-variant is the natural reuse) bound to `window=week\|month` — the exact values `parseWindow` accepts (activity_helpers.go:73). Both controls feed the same `useActivity(scope, window)` query key. |
| ACT-04 | Reviews section quietly empties when GitHub integration off / `gh` absent; tasks + task stats still work | Branch on `reviews.state` (`disabled`/`no_gh`/`auth_required` → empty); the endpoint is always 200 and tasks/stats always populate regardless of gh state (activity.go:234-273, GATE 1). |
| TASKS-02 | Tasks-done grouped by project, showing title + completion time | Client-side bucket `activityTask[]` by `projectId`/`projectName` (both on the wire); `doneAt` is ms-ISO (renderable via `Date`). |
| TASKS-03 | Clicking a tasks-done entry navigates to that task's view | `activityTask` carries `id` + `projectId` → plain `Link`/`navigate` to `/projects/:projectId/tasks/:taskId` (D-15). |
| REVIEWS-02 | Reviews-done grouped by project, showing PR number + title + merge/close date | Client-side bucket `reviews.prs` (`ReviewDoneSummary`) by `projectId`/`projectName`; show `#number title` + `completedAt`. Sort by `completedAt DESC` client-side (server concatenates per-repo unsorted). |
| REVIEWS-03 | Clicking a reviews-done entry opens the PR (its review workspace if it exists, else the PR) | Implemented as the **universal PR-URL fallback** (D-14): `<a href={ReviewDoneSummary.url} target="_blank" rel="noreferrer">`. Conscious omission of the workspace preference — flag at UAT. |

</phase_requirements>

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Activity page route + shell | Browser / Client | — | Pure SPA route under `<AppLayout />`; no SSR (single-binary Go serves static `dist/`). |
| Sidebar entry (expanded + collapsed rail) | Browser / Client | — | `ProjectSidebar.tsx` is the only owner; active state from `location.pathname`. |
| Scope selector (Global/Workspace/Project) | Browser / Client | — | Lists built from already-fetched `useProjects`/`useWorkspaces`; no new fetch. |
| Week/Month toggle | Browser / Client | — | Ephemeral + localStorage-persisted view state; drives query key. |
| Activity data fetch | API / Backend (Phase 10, EXISTING) | Browser / Client (consumer) | `GET /api/activity` already shipped; Phase 11 only consumes. **Do NOT add backend code.** |
| Scope/window persistence | Browser / Client | — | `localStorage` under `kamacu.*` (D-06); never URL, never server. |
| Duration formatting (stats) | Browser / Client | — | `timeStat` arrives as seconds; formatting is purely client-side. |
| Reviews degrade branching | Browser / Client | API / Backend (signal) | Backend signals via `reviews.state`; the page decides visibility. |

**Why this matters:** The temptation is to push scope/window resolution or grouping into a backend tweak — **resist it.** Phase 10's endpoint is locked and tested; Phase 11's entire job is client-side rendering + view state. The only "backend" touch is reading the already-live endpoint.

## Standard Stack

**No new packages are installed in this phase.** Every dependency below is already in `web/package.json` and already exercised by existing pages. Versions verified from the installed `package.json`.

### Core (all existing — VERIFIED)

| Library | Version (installed) | Purpose | Why Standard / Already Used |
|---------|---------------------|---------|------------------------------|
| `react` | `^19.2.6` | UI framework | Project stack (CLAUDE.md). `[VERIFIED: package.json]` |
| `react-router` | `^7.17.0` | Routing — new `/activity` route + `Link`/`useNavigate` | Already drives every route (App.tsx). `[VERIFIED: package.json]` |
| `@tanstack/react-query` | `^5.101.0` | `useActivity(scope, window)` server state | Established for all data (queries.ts). `[VERIFIED: package.json]` |
| `lucide-react` | `^1.17.0` | `Activity` icon for the sidebar entry | **`Activity` export verified present** (`typeof Activity === "object"`); app already imports `Settings`/`Plus`/`ChevronsUpDown`/`ExternalLink`. `[VERIFIED: codebase — node -e require check]` |
| Tailwind CSS | `^4.3.0` + `@tailwindcss/vite ^4.3.0` | Styling (incl. `group-data-[collapsible=icon]:*`) | All classes used below already work in the sidebar. `[VERIFIED: package.json]` |
| shadcn/ui (radix) primitives | copied-in | `DropdownMenu*`, `Tabs`, `Skeleton`, `Button`, `Tooltip` | All already in `web/src/components/ui/`. `[VERIFIED: glob]` |
| TypeScript | `~6.0.2` | Type safety for new wire types | Project stack. `[VERIFIED: package.json]` |

### Supporting (existing — used by this phase)

| Library | Purpose | When to Use |
|---------|---------|-------------|
| `DropdownMenu` + `DropdownMenuCheckboxItem` + `DropdownMenuLabel` + `DropdownMenuSeparator` | Scope selector | Mirror `WorkspaceSwitcher.tsx`. Submenu primitives (`DropdownMenuSub*`) also exported if nesting projects under workspaces. `[VERIFIED: dropdown-menu.tsx exports]` |
| `Tabs` / `TabsList` / `TabsTrigger` (default variant) | Week/Month segmented control | Default-variant `Tabs` visually IS a segmented control (`bg-muted` list, active `bg-background` pill). Agent's discretion (D-07) — two `Button`s also fine. `[VERIFIED: tabs.tsx]` |
| `Skeleton`, `Button`, `Tooltip`, `Link` | Page shell + entry | Reused from `SettingsPage.tsx` / `ProjectSidebar.tsx`. `[VERIFIED: codebase]` |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `Tabs` (default variant) for Week/Month | Two-toggle `Button` group or bespoke segmented control | `Tabs` is zero-new-code reuse and visually matches; a bespoke control is more markup for no gain. Pick `Tabs`. |
| Flat scope dropdown with `DropdownMenuLabel` section headers | `DropdownMenuSub` (workspace → projects nesting) | Flat is simpler and matches "section/label idioms"; nesting is nicer with many projects. Agent's discretion (CONTEXT D-04). |
| Dedicated `web/src/api/activity.ts` | Add `useActivity` to `queries.ts` | `queries.ts` already holds `useProjects`/`useWorkspaces`/`useGithubStatus` — co-locating is consistent; a dedicated file is fine if the wire types are large. Agent's discretion. |

**Installation:** None required. `npm install` is a no-op for this phase.

**Version verification:** Ran `node -e require('./package.json')` — versions above are the installed values, not training-data guesses.

## Package Legitimacy Audit

> This phase installs **zero** external packages. The gate is therefore trivially satisfied; the table records that the dependencies this phase *uses* are already vetted by prior phases (10, 25, 26).

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| _(no new packages)_ | — | — | — | — | — | N/A — phase adds none |

**Existing deps this phase relies on** (all already approved in prior phases): `react`, `react-router`, `@tanstack/react-query`, `lucide-react`, `tailwindcss`, `@tailwindcss/vite`, `vite`, `typescript`, and the copied-in shadcn/radix primitives. **No `[SUS]` or `[SLOP]` packages. No `checkpoint:human-verify` needed.**

*No packages were discovered via WebSearch or training data for this phase — every recommendation is an existing in-repo dependency.*

## Architecture Patterns

### System Architecture Diagram

Data flow for the primary use case (open Activity → pick scope/window → see grouped lists + stats):

```
 ┌─────────────────────────── BROWSER (SPA) ───────────────────────────┐
 │                                                                     │
 │  ProjectSidebar.tsx (SidebarFooter)                                 │
 │    └─ Activity icon-button ──(always reachable: expanded + rail)──┐ │
 │                                                                   ▼ │
 │  react-router  /activity  (App.tsx, sibling of /settings) ──► ActivityPage.tsx
 │                                                                   │
 │  ActivityPage mounts:                                             │
 │    useActivityView() ──► { scope, setScope, window, setWindow }   │
 │      │  (localStorage: kamacu.activity.scope / .window)           │
 │      │  default scope = useActiveWorkspace() → workspace:N        │
 │      │                                       (fallback global)    │
 │      ▼                                                            │
 │    ┌─ Scope dropdown (DropdownMenu)  ──► setScope("workspace:N")  │
 │    └─ Week|Month segmented (Tabs)   ──► setWindow("week"|"month") │
 │            │                                                       │
 │            ▼                                                       │
 │    useActivity(scope, window)  queryKey ["activity",scope,window] │
 │            │  get(`/api/activity?scope=…&window=…`)                │
 │            ▼                                                       │
 └────────────┬────────────────────────────────────────────────────────┘
              │ HTTPS (always 200)
              ▼
 ┌──────────────── BACKEND (Phase 10 — EXISTING, DO NOT TOUCH) ─────────┐
 │  GET /api/activity?scope=global|workspace:N|project:N&window=week|month│
 │   → { tasks:[…], reviews:{state,stale,fetchedAt,prs:[…]}, stats:{…} } │
 └──────────────────────────────────────────────────────────────────────┘
              │
              ▼ (one response, one loading state)
 ┌──────────────── RENDERING (client-side) ────────────────┐
 │  Stats strip:  taskCount · reviewCount                  │
 │                cycle / dwellInProgress / dwellInReview  │
 │                  each = `min · median · max`  (formatDuration)
 │                  n===0 → em-dash                         │
 │                                                          │
 │  Tasks-done section:  bucket tasks by projectId          │
 │    per group: project-name subheading                    │
 │      entry → Link /projects/:projectId/tasks/:taskId     │
 │                                                          │
 │  Reviews-done section:  branch on reviews.state          │
 │    disabled/no_gh/auth_required → render NOTHING (quiet) │
 │    ok/partial/error → bucket prs by projectId            │
 │      entry → <a href={url} target=_blank rel=noreferrer> │
 └──────────────────────────────────────────────────────────┘
```

A reader can trace "user opens Activity → sees scoped stats + two grouped lists" end-to-end by following the arrows. File-to-component mapping is in the Component Responsibilities table below.

### Recommended Project Structure

```
web/src/
├── App.tsx                          # +1 line: <Route path="/activity" ...>
├── pages/
│   └── ActivityPage.tsx             # NEW — page shell + 3 sections (mirrors SettingsPage)
├── api/
│   ├── queries.ts  (or activity.ts) # NEW useActivity(scope, window)
│   └── types.ts                     # NEW: ActivityTask, ReviewDoneSummary, TimeStat, StatsBlock, ActivityResponse, ReviewState
├── lib/
│   ├── time.ts                      # +formatDuration(seconds) alongside formatAgo
│   └── useActivityView.ts           # NEW — localStorage scope+window hook (mirrors useActiveWorkspace)
└── components/
    ├── sidebar/ProjectSidebar.tsx   # +Activity entry in footer (rail-reachable per D-02)
    └── activity/                    # NEW (optional): ScopeSelector, WindowToggle, StatsStrip, ActivityList, ReviewsList
        ├── ScopeSelector.tsx
        ├── WindowToggle.tsx
        ├── StatsStrip.tsx
        ├── ActivityList.tsx         # tasks-done, grouped
        └── ReviewsList.tsx          # reviews-done, grouped, degrade-aware
```

> Whether the five `components/activity/*` files are split or inlined into `ActivityPage.tsx` is the agent's discretion — `SettingsPage.tsx` inlines its sections; the board splits into `components/board/*`. Either is consistent with the codebase.

### Component Responsibilities

| Component | Owns | Consumes |
|-----------|------|----------|
| `ActivityPage.tsx` | Page shell, loading/error states, section composition | `useActivity`, `useActivityView` |
| `ScopeSelector` | Dropdown trigger + menu (Global/workspaces/projects), writes scope | `useProjects`, `useWorkspaces`, `useActivityView().setScope` |
| `WindowToggle` | Week/Month segmented control, writes window | `useActivityView().setWindow` |
| `StatsStrip` | Counts + 3 time-stat rows (min·median·max) | `data.stats`, `formatDuration` |
| `ActivityList` | Bucket `tasks` by project, render entries, click→navigate | `data.tasks` |
| `ReviewsList` | Branch on `reviews.state`; bucket `prs` by project; click→external | `data.reviews` |
| `useActivityView` (lib) | Persist scope+window in `localStorage` (`kamacu.*`); default scope from `useActiveWorkspace()` | `useActiveWorkspace` |
| `useActivity` (api) | TanStack query, key `["activity", scope, window]` | `get`, wire types |
| `ProjectSidebar.tsx` | Mount the Activity entry (footer, rail-reachable) | `useLocation`, lucide `Activity` |

### Pattern 1: Top-level page route (mirror `/settings`)

**What:** A bare route under `<AppLayout />`, no path params.
**When to use:** App-level destinations (Settings, Activity) — not workspace/project-scoped.
**Example:**
```tsx
// Source: web/src/App.tsx:135 (the existing /settings registration)
<Route path="/settings" element={<SettingsPage />} />
// Phase 11 adds, as a sibling:
<Route path="/activity" element={<ActivityPage />} />
```
`[VERIFIED: codebase — App.tsx:135]`

### Pattern 2: Sidebar active state via `location.pathname`

**What:** The entry's active styling derives from the current path — no router-voodoo.
**Example:**
```tsx
// Source: web/src/components/sidebar/ProjectSidebar.tsx:180-183 (the Settings gear idiom)
className={cn(
  location.pathname === "/activity" &&
    "bg-sidebar-accent text-sidebar-accent-foreground",
)}
```
`[VERIFIED: codebase — ProjectSidebar.tsx:181]`

### Pattern 3: localStorage-backed view-state hook (mirror `useActiveWorkspace`)

**What:** Persist view state in `localStorage` under `kamacu.*`; resolve against live data on read.
**When to use:** View state that must survive reload but must NOT enter the URL (D-06).
**Example (shape — scope+window is simpler, likely needs NO context):**
```ts
// Source shape: web/src/lib/useActiveWorkspace.tsx (ACTIVE_WORKSPACE_KEY = "kamacu.workspace")
// For Activity, scope+window are consumed by ONE page only → a plain hook, not a context.
export const ACTIVITY_SCOPE_KEY = "kamacu.activity.scope";
export const ACTIVITY_WINDOW_KEY = "kamacu.activity.window";

export type ActivityScope = "global" | `workspace:${number}` | `project:${number}`;
export type ActivityWindow = "week" | "month";

// default scope = active workspace (D-05); fall back to "global" while unresolved
function readScope(fallback: ActivityScope): ActivityScope {
  const raw = localStorage.getItem(ACTIVITY_SCOPE_KEY);
  // validate against the exact parseScope grammar: global | workspace:N | project:N
  if (raw === "global" || /^workspace:\d+$/.test(raw) || /^project:\d+$/.test(raw)) return raw;
  return fallback;
}
```
`[VERIFIED: codebase — useActiveWorkspace.tsx:15,31-37]` (grammar against `parseScope`, activity_helpers.go:51)

### Pattern 4: One endpoint → one query, one loading state

**What:** The combined response means a single `useQuery` — switching scope/window just changes the key.
**Example:**
```ts
// Source pattern: web/src/api/queries.ts (useProjects/useWorkspaces) + Phase 10 D-01
export function useActivity(scope: ActivityScope, window: ActivityWindow) {
  return useQuery({
    queryKey: ["activity", scope, window],
    queryFn: () =>
      get<ActivityResponse>(`/api/activity?scope=${scope}&window=${window}`),
    // No refetchInterval — Activity is an occasional view, not a live dashboard.
  });
}
```
`[VERIFIED: codebase — queries.ts; wire path — activity.go:212]`

### Pattern 5: External link (reviews click-through, D-14)

**What:** Open the GitHub PR in a new tab. Established convention is `target="_blank" rel="noreferrer"`.
**Example:**
```tsx
// Source: web/src/components/board/PRCard.tsx:166-176 (the existing PR external-link idiom)
<a
  href={review.url}
  target="_blank"
  rel="noreferrer"
  aria-label={`Open PR #${review.number} on GitHub`}
>
  #{review.number} {review.title}
</a>
```
`[VERIFIED: codebase — PRCard.tsx:166, TaskPage.tsx:608]`

### Pattern 6: Collapsed-rail dual-surface rendering

**What:** Render one logical entry as two surfaces — a rail icon (hidden when expanded) + an expanded row (hidden in icon mode) — using `group-data-[collapsible=icon]:*`.
**Example (the project-row idiom to copy for the Activity entry):**
```tsx
// Source shape: web/src/components/sidebar/ProjectSidebar.tsx:117-142
{/* Rail icon: visible only when collapsed */}
<span className="hidden group-data-[collapsible=icon]:flex">
  <Activity className="size-5" />
</span>
{/* Expanded label: visible only when expanded */}
<span className="group-data-[collapsible=icon]:hidden">Activity</span>
```
`[VERIFIED: codebase — ProjectSidebar.tsx:117,140]`

### Anti-Patterns to Avoid

- **Don't put the Activity entry inside the footer's `group-data-[collapsible=icon]:hidden` wrapper.** That hides it in the collapsed rail (violates D-02). Lift it out or apply the dual-surface classes above. `[VERIFIED: codebase — ProjectSidebar.tsx:162]`
- **Don't recompute cycle/dwell on the client.** `activityTask.inProgressAt`/`inReviewAt` are `json:"-"` (NOT shipped) — only `doneAt` is. Use the precomputed `stats` block. `[VERIFIED: codebase — activity_helpers.go:15-16]`
- **Don't whitelist "good" review states.** Enumerate degrade states (`disabled`/`no_gh`/`auth_required`/`error` with empty prs) OR branch on `prs.length`. CONTEXT D-12 omits `auth_required`, but the backend emits it. `[VERIFIED: codebase — rollupReviewState, activity_helpers.go:129]`
- **Don't sort reviews by string-comparing timestamps.** `completedAt` is second-precision ISO, `doneAt` is ms-ISO — parse via `Date.parse` then compare, or just rely on the fact that ISO-8601 sorts lexically within one precision. `[VERIFIED: codebase — activity.go:88-92, prlist.go:343-353]`
- **Don't add a toast for degrade states.** Project convention: inline/muted/dialog only (Phase 26). `[CITED: CONTEXT.md canonical_refs]`
- **Don't touch the backend.** Phase 10 is verified complete (UAT 16/16); the endpoint is locked. `[VERIFIED: STATE.md]`

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Scope/window persistence | A new context + reducer | `localStorage` read/write in a small hook (mirror `useActiveWorkspace`) | Only `ActivityPage` consumes it — a context (like workspace) is overkill; raw localStorage is the app's view-state idiom. |
| Segmented control (Week/Month) | Custom button-group + active styling | `Tabs` (default variant) | Already visually a segmented control; zero new CSS. (Agent's discretion per D-07.) |
| Dropdown menu primitives | Custom popover/menu | `DropdownMenu*` from `ui/dropdown-menu.tsx` | Already exports Label/CheckboxItem/Separator/Sub — exactly what a sectioned scope selector needs. |
| Duration formatting (d/h/m) | A date library (`date-fns`/`dayjs`) | A ~15-line `formatDuration(seconds)` util | Adaptive largest-1-2-units is trivial arithmetic; `lib/time.ts` already has `formatAgo` with zero deps (UI-SPEC: zero new npm deps). |
| Active state for sidebar entry | A router context probe | `location.pathname === "/activity"` | The Settings gear idiom — one line. |
| Grouping by project | A grouping library | `Array.reduce` into `Map<projectId, entries>` | N is tiny (a week/month of done work); bucketing is 5 lines. |
| Loading/error states | Per-section spinners | `SettingsPage.tsx` skeleton + error/retry shell | The page-shell template already solves this. |

**Key insight:** This phase's entire value is composition of existing primitives against an existing endpoint. The only genuinely new logic is the ~15-line duration formatter and the degrade-branch on `reviews.state`. Everything else is clone-and-adapt.

## Common Pitfalls

### Pitfall 1: Collapsed-rail footer hide swallows the Activity entry

**What goes wrong:** Dropping the Activity button inside the existing `SidebarFooter` (which carries `group-data-[collapsible=icon]:hidden`) makes it vanish when the sidebar is collapsed — violating D-02's "always reachable" lock.
**Why it happens:** The footer class hides the WHOLE footer in icon mode (today this hides "Add project" + the Settings gear too — deliberately, for them).
**How to avoid:** Either (a) render the Activity entry OUTSIDE the hide wrapper (a separate `SidebarMenu`/footer element), or (b) keep it in the footer but apply the dual-surface classes (`hidden group-data-[collapsible=icon]:flex` rail icon + `group-data-[collapsible=icon]:hidden` label) so it survives the collapse.
**Warning signs:** With the sidebar collapsed (Ctrl+B), the Activity icon is absent from the rail.

### Pitfall 2: `auth_required` review state is not in CONTEXT's D-12

**What goes wrong:** CONTEXT D-12 names only `disabled`/`no_gh` for "quietly empty" and `partial`/`error` for "successful repos render." The backend ALSO emits `auth_required` (gh installed but not logged in) — see `rollupReviewState` and `classifyGhListError`.
**Why it happens:** The discuss-phase enumerated the common cases; the rarer `auth_required` (a valid degrade) was not spelled out.
**How to avoid:** Branch on a degrade-STATE SET `{ disabled, no_gh, auth_required, error }` (render reviews only when `state === "ok" || state === "partial"`, OR simply render whatever `prs` contains and let empty be empty). Never whitelist `{ ok }` alone — `partial` carries real PRs.
**Warning signs:** With gh installed but unauthenticated, the reviews section errors or shows a "no data" hint instead of being quietly empty.

### Pitfall 3: `timeStat` values are SECONDS (truncated), not ms or Duration

**What goes wrong:** Formatting `min/max/median` as milliseconds, or expecting sub-second precision, yields `45000m` or stale-looking values.
**Why it happens:** Go's `int64(duration.Seconds())` truncates to whole seconds server-side (activity_helpers.go:100-102).
**How to avoid:** Treat every `timeStat` field as integer seconds. `formatDuration(seconds)` does `d = floor(s/86400)`, etc.
**Warning signs:** Cycle times render in the thousands of "minutes."

### Pitfall 4: `n === 0` must render em-dash, not `0m`

**What goes wrong:** A default "no data" formatter prints `0m`/`0s` for an empty stat, implying a measured zero.
**Why it happens:** The stat object is `{n:0,min:0,max:0,median:0}` when no in-window rows had timestamps.
**How to avoid:** Guard `if (stat.n === 0) return "—";` BEFORE formatting (D-11). Counts still render literal `0`.
**Warning signs:** An empty week shows `0m · 0m · 0m` instead of `— · — · —`.

### Pitfall 5: Reviews are NOT pre-sorted across repos

**What goes wrong:** Rendering `reviews.prs` in arrival order yields jumbled dates.
**Why it happens:** `aggregateReviews` concatenates per-repo lists (each repo's list is gh-sorted internally, but the cross-repo merge is unsorted). Tasks, by contrast, arrive `doneAt DESC` from the SQL `ORDER BY`.
**How to avoid:** Sort the bucketed (or pre-bucket) `prs` by `completedAt DESC` client-side. ISO-8601 sorts lexically within the same precision (all `completedAt` are second-precision gh closedAt), so a locale compare is safe here.
**Warning signs:** Within a project group, review entries are not in reverse-chronological order.

### Pitfall 6: Default scope depends on `useActiveWorkspace()` still resolving

**What goes wrong:** Firing `useActivity` before the active workspace resolves issues a query with a null-derived scope, or the dropdown shows the wrong default.
**Why it happens:** `useActiveWorkspace().activeWorkspaceId` is `null` while `useWorkspaces()` loads.
**How to avoid:** Resolve the default scope lazily — initialize the persisted-scope hook's default to `"global"`, and when `activeWorkspaceId` becomes non-null AND no saved scope exists, adopt `workspace:<id>`. Gate the query `enabled` until scope is a valid non-null grammar string.
**Warning signs:** The first paint shows Global when the user's last scope was a workspace, or a console 400 from a malformed scope.

### Pitfall 7: Treat `ReviewDoneSummary.url` as the click target, not `window.open`

**What goes wrong:** Using `window.open(url)` with a default `_blank` can be blocked as a popup; programmatic navigation loses the new-tab intent.
**How to avoid:** Use a plain `<a href={url} target="_blank" rel="noreferrer">` (the established PRCard/TaskPage idiom). A `<Link>` would be wrong — it's an external URL, not a route.
**Warning signs:** Clicking a review entry opens in the same tab (replacing the app) or is popup-blocked.

## Code Examples

Verified patterns from the codebase + Phase 10 source.

### The full wire contract (the single source of truth)

```ts
// Source: internal/api/activity.go:22-36 + activity_helpers.go:11-34 + prlist.go:324-332
// VERIFIED by reading the Go source and the activity_test.go assertions.

export type ReviewState =
  | "ok" | "partial" | "error" | "auth_required" | "no_gh" | "disabled";

export interface ActivityTask {
  id: number;
  title: string;
  doneAt: string;        // ms-ISO (reaper format 2006-01-02T15:04:05.000Z)
  projectId: number;
  projectName: string;
  // NOTE: inProgressAt / inReviewAt are json:"-" — NOT shipped. Do not recompute cycle/dwell.
}

export interface ReviewDoneSummary {
  number: number;
  title: string;
  completedAt: string;   // second-precision ISO (gh closedAt, set for BOTH merged+closed)
  url: string;           // https GitHub PR URL
  projectName: string;   // annotated by the aggregation loop
  projectId: number;
  repo: string;          // owner/name
}

export interface TimeStat {
  n: number;             // count of rows this stat was computed over (may be < taskCount)
  min: number;           // int64 SECONDS (truncated)
  max: number;           // int64 SECONDS
  median: number;        // int64 SECONDS
}

export interface StatsBlock {
  taskCount: number;
  reviewCount: number;
  cycle: TimeStat;
  dwellInProgress: TimeStat;
  dwellInReview: TimeStat;
}

export interface ActivityResponse {
  tasks: ActivityTask[];
  reviews: {
    state: ReviewState;
    stale: boolean;
    fetchedAt: string | null;
    prs: ReviewDoneSummary[];
  };
  stats: StatsBlock;
}
```
`[VERIFIED: codebase — activity.go, activity_helpers.go, prlist.go]`

### Adaptive duration formatter (the one new util)

```ts
// Source: NEW — adaptive largest-1-2-units per CONTEXT D-10. Mirrors the zero-dep
// style of lib/time.ts:formatAgo. Input is SECONDS (the wire contract).
export function formatDuration(totalSeconds: number): string {
  if (!Number.isFinite(totalSeconds) || totalSeconds < 0) return "—";
  const s = Math.floor(totalSeconds);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m`;            // D-10 examples: "45m", "30s"
  const h = Math.floor(m / 60);
  const remM = m % 60;
  if (h < 24) return remM > 0 ? `${h}h ${remM}m` : `${h}h`;   // "4h 20m"
  const d = Math.floor(h / 24);
  const remH = h % 24;
  return remH > 0 ? `${d}d ${remH}h` : `${d}d`;               // "1d 3h"
}
```
`[CITED: CONTEXT.md D-10]` (unit labels `m`/`h`/`d` are the agent's discretion — D-10 lists `min`/`hr` as an alternative; pick one and stay consistent.)

### Stats row with em-dash guard (D-09 / D-11)

```tsx
// n===0 → em-dash for the whole row (D-11); counts always render literal.
function TimeStatRow({ label, stat, totalCount }: { label: string; stat: TimeStat; totalCount: number }) {
  return (
    <div className="flex items-baseline gap-2 text-sm">
      <span className="text-muted-foreground">{label}</span>
      {stat.n === 0 ? (
        <span className="font-mono text-muted-foreground">—</span>
      ) : (
        <span className="font-mono">
          {formatDuration(stat.min)} · {formatDuration(stat.median)} · {formatDuration(stat.max)}
        </span>
      )}
      <span className="text-xs text-muted-foreground">over {stat.n} tasks</span>
    </div>
  );
}
```
`[CITED: CONTEXT.md D-09, D-11]`

### Client-side grouping by project

```ts
// Both lists bucket the same way. Tasks already doneAt DESC; reviews need a client sort.
function groupByProject<T extends { projectId: number; projectName: string }>(
  rows: T[],
): { projectId: number; projectName: string; entries: T[] }[] {
  const map = new Map<number, { projectId: number; projectName: string; entries: T[] }>();
  for (const r of rows) {
    let g = map.get(r.projectId);
    if (!g) {
      g = { projectId: r.projectId, projectName: r.projectName, entries: [] };
      map.set(r.projectId, g);
    }
    g.entries.push(r);
  }
  // Group ordering is the agent's discretion (name vs recency) — CONTEXT discretion list.
  return [...map.values()];
}

// Reviews: sort the whole list by completedAt DESC BEFORE grouping (or sort within groups).
// completedAt is uniform second-precision ISO → lexical descending sort is chronological.
export const byCompletedDesc = (a: ReviewDoneSummary, b: ReviewDoneSummary) =>
  b.completedAt.localeCompare(a.completedAt);
```
`[CITED: CONTEXT.md D-13; VERIFIED sort safety — prlist.go:343-353 (completedAt is uniform gh closedAt)]`

### Reviews-section degrade branch (D-12 + the auth_required gap)

```tsx
// The cleanest rule: render the reviews body from prs; hide the whole section on hard-degrade.
const HARD_DEGRADE: ReadonlySet<ReviewState> = new Set([
  "disabled", "no_gh", "auth_required",  // auth_required NOT in CONTEXT D-12 — see Pitfall 2
]);

function ReviewsSection({ reviews }: { reviews: ActivityResponse["reviews"] }) {
  if (HARD_DEGRADE.has(reviews.state) && reviews.prs.length === 0) {
    return null; // D-12: quietly empty — no error, no hint
  }
  // ok / partial / error (or a degrade that nonetheless has prs): render grouped.
  const groups = groupByProject([...reviews.prs].sort(byCompletedDesc));
  return (
    <section className="flex flex-col gap-2">
      <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">Reviews</h2>
      {groups.length === 0 ? null : groups.map(/* …project subheading + entry <a>… */)}
    </section>
  );
}
```
`[CITED: CONTEXT.md D-12; VERIFIED state set — activity_helpers.go:129 rollupReviewState]`

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `react-beautiful-dnd` | `@dnd-kit/*` | Atlassian archived rbd (2023) | Not directly relevant here, but confirms the codebase is on current React 19-era primitives. |
| xterm unscoped `xterm` | `@xterm/*` scoped | xterm 6.0 (Dec 2024) | Not relevant to this phase (terminal-only). |
| Per-component `localStorage` `useState` | Shared context for cross-tree state (Phase 26) | Workspace work | Activity scope/window is single-page → plain hook, NOT a context (discretion). |

**Deprecated/outdated (none apply to this phase):** No deprecation risks — the phase composes only current primitives.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `lucide-react` `Activity` icon is the intended pulse-line glyph (not a different "Activity" export). | Standard Stack / D-03 | LOW — verified `typeof Activity === "object"` at runtime; visual confirm is trivial at impl. |
| A2 | `Tabs` default variant is an acceptable "segmented control" for Week/Month (D-07 says segmented; agent's discretion). | Architecture Patterns / D-07 | NONE — D-07 explicitly leaves the control choice to the agent; `Tabs` is one valid pick. |
| A3 | The duration unit labels `d`/`h`/`m`/`s` are acceptable (D-10 lists `min`/`hr` as an alternative). | Code Examples / D-10 | NONE — D-10 leaves labels to the agent. |
| A4 | Activity scope/window need NOT be a context (only `ActivityPage` consumes them). | Architecture Patterns | LOW — if a second consumer appears, lift into a context like `useActiveWorkspace`. |
| A5 | The reviews section heading may render even when `state` is `ok` but `prs` is empty. | Code Examples | LOW — CONTEXT D-12 says "quietly empty" with "no hint"; rendering just a heading is consistent. Agent can suppress the heading too. |

**No `[ASSUMED]` package-name claims** — this phase adds no packages.

## Open Questions

1. **Should the reviews section heading render at all when degraded?**
   - What we know: D-12 says "quietly empty — no error, no hint" for `disabled`/`no_gh`.
   - What's unclear: Whether "no hint" includes suppressing the "Reviews" heading, or just the body.
   - Recommendation: Suppress the entire section (heading + body) on hard-degrade states; render heading + body otherwise. This is the most literal "quietly empty." Agent's call (CONTEXT lists copy as discretion).

2. **Scope dropdown structure — flat with section labels, or nested workspace→projects?**
   - What we know: D-04 wants one dropdown listing Global + workspaces + projects, mirroring the switcher's "section/label idioms."
   - What's unclear: With many projects, a flat list gets long; nesting (`DropdownMenuSub`) under each workspace is cleaner but more markup.
   - Recommendation: Flat with `DropdownMenuLabel` section headers ("Global" implied, "Workspaces", "Projects") for v1.12 — matches the switcher's simplicity. Revisit nesting if project count grows. Agent's discretion.

3. **Should the stats strip / lists refetch when the user returns to the tab?**
   - What we know: `QueryClient` default is `refetchOnWindowFocus: false`, no `staleTime` (so refetch on mount/re-invalidHome). Activity has no `refetchInterval`.
   - What's unclear: Whether returning to `/activity` should always refetch (staleTime 0) or cache the last view for the session.
   - Recommendation: Keep defaults (refetch on mount) — Activity is occasional; a fresh fetch on each open is correct and cheap (5min gh cache on the backend absorbs repeats).

## Environment Availability

> This phase has no NEW external dependencies. It is pure frontend consuming an existing endpoint. The table records the one runtime precondition.

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Node.js (20.19+ / 22.12+) | Vite 8 build/dev | ✓ (Vite already builds) | — | — |
| Phase 10 `GET /api/activity` endpoint | `useActivity` query | ✓ | shipped, UAT 16/16 | — |
| `claude`/`gh` (host) | Reviews data (transitive, via Phase 10) | optional | — | Reviews section quietly empties (ACT-04) — by design |

**Missing dependencies with no fallback:** None.
**Missing dependencies with fallback:** None needed — `gh` absence is a *designed* degrade, not a missing dependency (the page handles it via `reviews.state`).

## Security Domain

> `security_enforcement` is absent in `.planning/config.json` → treated as enabled. This is a frontend-only phase rendering server-provided data; the surface is small. ASVS categories assessed below.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | Single-user localhost; no auth in this phase. |
| V3 Session Management | no | No session work. |
| V4 Access Control | no | Read-only rendering of already-scoped server data. |
| V5 Input Validation / Output Encoding | yes | React auto-escapes all interpolated text (`title`, `projectName`). The one `href` (`ReviewDoneSummary.url`) is server-sourced from `gh pr list --json url` (always an https GitHub URL). Render with `target="_blank" rel="noreferrer"` (established idiom). Optional cheap guard: `url.startsWith("https://")` before rendering as href. |
| V6 Cryptography | no | None. |
| V7 Errors & Logging | yes | Errors are transport-level only (endpoint is always 200); surface via the `SettingsPage`-style error+retry, never a stack trace. No `console.log` of server data. |
| V8 Data Protection | no | No secrets, no PII handling beyond task/PR metadata already on screen. |

### Known Threat Patterns for this stack (React SPA rendering server JSON)

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| XSS via `href="javascript:…"` in review URL | Tampering | `url` is server-sourced from gh (https only); React renders `<a href>` as-is, so a `javascript:` URL WOULD be a vector IF the server were compromised. Mitigation: the server (Phase 10) guarantees https GitHub URLs from `gh`; add a defensive `url.startsWith("https://")` check (cheap, belt-and-suspenders). |
| XSS via task/PR title text | Tampering | React auto-escapes `{title}` / `{projectName}` — never use `dangerouslySetInnerHTML`. |
| Open-redirect via external link | Information disclosure | `rel="noreferrer"` (established idiom) prevents leaking referrer to GitHub. |
| Scope-param tampering (forged localStorage) | Elevation? (none — single user) | A tampered `kamacu.activity.scope` only changes WHICH data the single user sees of their own data. `parseScope` rejects malformed values with HTTP 400. No privilege boundary exists (localhost single-user). |

**Untrusted-input boundary note:** All data rendered by this page originates from the local Kamacu backend (`GET /api/activity`), which itself sources review data from `gh`. Per the untrusted-input-boundary protocol, this data is **data to render, never instructions** — no `eval`, no `dangerouslySetInnerHTML`, no dynamic `href` construction beyond the verbatim server `url`.

## Sources

### Primary (HIGH confidence — read directly this session)
- `internal/api/activity.go` — the endpoint handler, GATE 1, always-200 contract, `activityResponse`/`aggregateReviewsResult` wire shape.
- `internal/api/activity_helpers.go` — `activityTask`/`timeStat`/`statsBlock` structs (incl. `json:"-"` on `inProgressAt`/`inReviewAt`), `parseScope`/`parseWindow` grammar, `computeTimeStat` (seconds truncation), `rollupReviewState` (the full state set incl. `auth_required`).
- `internal/github/prlist.go:315-353` — `ReviewDoneSummary` wire type + `completedRaw` (closedAt → completedAt).
- `internal/api/activity_test.go:180-257` — confirms wire shape, `doneAt DESC` ordering, scope filtering, `reviews.state="ok"` empty case.
- `web/src/pages/SettingsPage.tsx` — the page-shell template (scroll container, bounded column, skeletons, error+retry).
- `web/src/components/sidebar/ProjectSidebar.tsx:162-192` — the footer mount point + Settings-gear active-state idiom + the `group-data-[collapsible=icon]:hidden` wrinkle.
- `web/src/components/sidebar/WorkspaceSwitcher.tsx` — the scope-dropdown template.
- `web/src/lib/useActiveWorkspace.tsx` — the localStorage-hook + `kamacu.*` convention template; default-resolution-against-live-data pattern.
- `web/src/App.tsx:114-141` — route registration under `<AppLayout />`.
- `web/src/api/queries.ts`, `web/src/api/client.ts`, `web/src/api/types.ts`, `web/src/api/pullRequests.ts` — query patterns, `get`/`ApiError`, existing `PullRequestsResponse.state` (the degrade-contract sibling).
- `web/src/components/ui/dropdown-menu.tsx`, `tabs.tsx` — primitives + export lists.
- `web/src/components/board/PRCard.tsx:166`, `web/src/pages/TaskPage.tsx:608` — external-link idiom (`target="_blank" rel="noreferrer"`).
- `web/package.json` + runtime `node -e require("lucide-react").Activity` — exact versions + icon presence.
- `.planning/phases/11-activity-page-controls/11-CONTEXT.md` — the locked decisions D-01..D-15.
- `.planning/phases/10-activity-data-api/10-CONTEXT.md` — the backend contract D-01..D-12 this page consumes.
- `.planning/STATE.md` — Phase 10 verified complete (UAT 16/16); no backend changes expected.

### Secondary (MEDIUM confidence)
- `.planning/config.json` — `nyquist_validation: false` (skip Validation Architecture), `ui_phase: true` (UI safety gate applies), `security_enforcement` absent (enabled).

### Tertiary (LOW confidence)
- None. No external web/docs research was performed — the phase is fully grounded in the in-repo CONTEXT.md + Phase 10 source + the existing frontend code. No `[ASSUMED]` package or API claims.

## Project Constraints (from CLAUDE.md / AGENTS.md)

> No `AGENTS.md` exists; project instructions come from `CLAUDE.md` (which embeds PROJECT.md + STACK.md). Relevant directives for this phase:

- **GSD workflow enforcement:** Make changes through a GSD command (`/gsd:execute-phase`), not direct edits. *(Process — applies to execution, not research.)*
- **Tech stack lock:** React frontend, Go backend, single binary, local-only. This phase is React-only (no Go) — consistent.
- **No toasts / sonner** (Phase 26 convention, restated in CONTEXT canonical_refs): all feedback inline/muted/dialog. The reviews degrade MUST NOT toast.
- **`kamacu.*` localStorage prefix** (migrateStorage.ts): new keys `kamacu.activity.scope` / `kamacu.activity.window` follow this.
- **UI-SPEC: zero new npm deps** (time.ts comment) — this phase adds none; the duration formatter is hand-rolled in `lib/time.ts` style.
- **No co-authoring / PR conventions** (from `~/.claude/CLAUDE.md`): PRs open as draft with an abstract first section; review comments kept short. *(Applies at `/gsd-ship`, not research.)*

## Metadata

**Confidence breakdown:**
- Standard stack: **HIGH** — all deps already in `package.json`, verified by reading it + runtime icon check.
- Wire contract: **HIGH** — read the Go source + tests directly, not assumed.
- Architecture: **HIGH** — every pattern cited is cloned from a named in-repo file at a named line.
- Pitfalls: **HIGH** — each derived from a concrete code fact (footer hide class, `json:"-"`, `rollupReviewState`, seconds truncation, unsorted cross-repo merge).
- Security: **HIGH** — small surface; the one `href` consideration is mitigated by the server's gh sourcing + the established `rel="noreferrer"` idiom.

**Research date:** 2026-07-31
**Valid until:** 2026-08-30 (stable — the contract is locked by the verified Phase 10 backend; only the frontend layer this phase owns could drift, and it's greenfield).
