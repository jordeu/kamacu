# Phase 11: Activity Page & Controls - Pattern Map

**Mapped:** 2026-07-31
**Files analyzed:** 7 (5 new, 2 modified) + 5 discretionary sub-components
**Analogs found:** 7 / 7 (every file has an exact or strong in-repo analog)

> This is a **frontend-only** phase. Phase 10 already shipped `GET /api/activity`; Phase 11 renders it. Every file below is `web/src/*` TypeScript/React. No Go, no SQL, no migration, **zero new npm packages** (verified — `react`, `react-router`, `@tanstack/react-query`, `lucide-react`, Tailwind, and the copied-in shadcn/radix primitives are all already installed and exercised by existing pages).

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `web/src/pages/ActivityPage.tsx` (NEW) | page/component | request-response (render) | `web/src/pages/SettingsPage.tsx` | **exact** (page shell) |
| `web/src/App.tsx` (MODIFY +1 route) | route | request-response | `web/src/App.tsx:135` (`/settings`) | **exact** (1-line sibling) |
| `web/src/api/queries.ts` (MODIFY +1 query) | hook (data) | request-response | `web/src/api/queries.ts` (`useProjects`/`useTasks`) + `web/src/api/pullRequests.ts` (`usePullRequests`) | **exact** |
| `web/src/api/types.ts` (MODIFY +wire types) | model | request-response | `web/src/api/types.ts` + `web/src/api/pullRequests.ts:24` (`PullRequestsResponse.state`) | **exact** |
| `web/src/lib/useActivityView.ts` (NEW) | hook (view state) | event-driven (localStorage) | `web/src/lib/useActiveWorkspace.tsx` | **role-match** (single-page hook, NOT a context) |
| `web/src/lib/time.ts` (MODIFY +`formatDuration`) | utility | transform | `web/src/lib/time.ts:4` (`formatAgo`) | **exact** (sibling formatter) |
| `web/src/components/sidebar/ProjectSidebar.tsx` (MODIFY +entry) | component | event-driven (nav) | `web/src/components/sidebar/ProjectSidebar.tsx:162-192` (footer) + `:117-142` (rail dual-surface) | **exact** |
| `web/src/components/activity/ScopeSelector.tsx` (NEW, discretionary) | component | event-driven | `web/src/components/sidebar/WorkspaceSwitcher.tsx` | **exact** |
| `web/src/components/activity/WindowToggle.tsx` (NEW, discretionary) | component | event-driven | `web/src/components/ui/tabs.tsx` (default-variant `Tabs`) | **role-match** |
| `web/src/components/activity/StatsStrip.tsx` (NEW, discretionary) | component | transform | `web/src/components/board/ReviewColumn.tsx:161` (`ReviewStates` count/empty pattern) | **role-match** |
| `web/src/components/activity/ActivityList.tsx` (NEW, discretionary) | component | transform (group) | `web/src/components/board/ReviewColumn.tsx` (`.map` over `prs`) | **role-match** |
| `web/src/components/activity/ReviewsList.tsx` (NEW, discretionary) | component | transform + degrade | `web/src/components/board/ReviewColumn.tsx:161-269` (`ReviewStates` state-branch) | **exact** (degrade idiom) |

> **Discretionary split:** CONTEXT §"the agent's Discretion" leaves open whether the five `components/activity/*` files are split out or inlined into `ActivityPage.tsx`. `SettingsPage.tsx` inlines its sections; the board splits into `components/board/*`. **Either is consistent** — pick one and stay uniform. The pattern assignments below apply regardless of file boundary.

---

## Pattern Assignments

### `web/src/pages/ActivityPage.tsx` (page, request-response render)

**Analog:** `web/src/pages/SettingsPage.tsx` (the top-level-page template — verbatim shell)

**Imports pattern** (mirror lines 1-10):
```typescript
// Source: web/src/pages/SettingsPage.tsx:1-10
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
// ActivityPage additionally imports:
//   useActivity (new query), useActivityView (new hook), formatDuration, Link/navigate
```

**Page shell pattern** (lines 140-148) — `h-full overflow-y-auto p-4` + bounded `max-w` column. Activity follows the same shell with `max-w-[640px]` (or a wider `max-w-2xl` for the stats strip + two lists — agent's discretion per CONTEXT):
```tsx
// Source: web/src/pages/SettingsPage.tsx:140-148
return (
  <div className="h-full overflow-y-auto p-4">
    <div className="max-w-[640px]">
      <h1 className="text-base font-medium">{`Activity`}</h1>
      {/* …sections… */}
    </div>
  </div>
);
```

**Error + retry pattern** (lines 128-138) — the project's load-failure idiom (board + settings share it). Activity reuses verbatim with "Activity" copy:
```tsx
// Source: web/src/pages/SettingsPage.tsx:128-138
if (isError) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3">
      <p className="text-muted-foreground">{`Couldn't load activity.`}</p>
      <Button variant="outline" onClick={() => refetch()}>
        {`Retry loading`}
      </Button>
    </div>
  );
}
```

**Loading skeleton pattern** (lines 148-156) — quiet `Skeleton`s, no spinner, no overlay (matches `ReviewStates` at `ReviewColumn.tsx:174-180`):
```tsx
// Source: web/src/pages/SettingsPage.tsx:148-156
{isLoading || !data ? (
  <div className="mt-6 flex flex-col gap-6">
    {[0, 1, 2, 3].map((i) => (
      <div key={i} className="flex flex-col gap-2">
        <Skeleton className="h-3 w-24" />
        <Skeleton className="h-8 w-full" />
      </div>
    ))}
  </div>
) : (
  /* …real content… */
)}
```

**Section heading pattern** (lines 79, 161, 171, 183, 197) — the project's universal section heading style. Reuse verbatim for "Tasks" / "Reviews" / stats group headings:
```tsx
// Source: web/src/pages/SettingsPage.tsx:79
<h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{`GitHub`}</h2>
```

**Page composition** (CONTEXT D-08 — stats strip on top, then tasks-done, then reviews-done, stacked single column):
```tsx
<div className="mt-6 flex flex-col gap-6">
  {/* control bar: ScopeSelector + WindowToggle (D-07) */}
  {/* StatsStrip (counts + cycle/dwell rows) */}
  {/* ActivityList (tasks-done grouped by project) */}
  {/* ReviewsList (reviews-done grouped, degrade-aware) */}
</div>
```

---

### `web/src/App.tsx` (route, request-response)

**Analog:** `web/src/App.tsx:135` (`/settings`) — a bare top-level route under `<AppLayout />`

**Route registration pattern** (lines 114-141) — `/activity` is a sibling of `/settings`, **not** under `/projects/:projectId` (so no `:projectId` in the URL):
```tsx
// Source: web/src/App.tsx:114-141
import ActivityPage from "@/pages/ActivityPage";  // ADD this import

export default function App() {
  return (
    <Routes>
      <Route element={<AppLayout />}>
        <Route index element={<RedirectToFirstProject />} />
        <Route path="/projects/:projectId" element={…} />
        <Route path="/projects/:projectId/tasks/:taskId" element={…} />
        <Route path="/settings" element={<SettingsPage />} />
        {/* ADD: bare /activity route — no :projectId, no BoardWorkspaceSync wrapper */}
        <Route path="/activity" element={<ActivityPage />} />
        <Route path="/terminal" element={<TerminalPage />} />
      </Route>
    </Routes>
  );
}
```

> **Do NOT wrap `<ActivityPage />` in `<BoardWorkspaceSync>`** (App.tsx:78-112) — that helper reconciles the URL's `:projectId` against the active workspace; Activity has no `:projectId` in the URL (CONTEXT D-06 — scope/window live in localStorage, never the URL).

---

### `web/src/api/queries.ts` + `web/src/api/types.ts` (data hook + wire types, request-response)

**Analog (hook):** `web/src/api/queries.ts:29-35` (`useTasks`) for the param-keyed query shape; `web/src/api/pullRequests.ts:37-45` (`usePullRequests`) for the always-200 degrade-contract sibling.
**Analog (types):** `web/src/api/pullRequests.ts:24-30` (`PullRequestsResponse.state` — the **same** degrade enum surface) + `web/src/api/types.ts` for the type declaration style.

**Wire types** — add to `web/src/api/types.ts` (mirroring the `Project`/`Workspace`/`Task` declaration style at types.ts:12-93). The exact shape is locked by Phase 10 (RESEARCH §"Code Examples" — verified against `internal/api/activity.go` + `activity_helpers.go` + `internal/github/prlist.go:324-332`):
```typescript
// Source shape: web/src/api/pullRequests.ts:24-30 (the degrade enum sibling)
// Wire contract: internal/api/activity_helpers.go:11-34 + prlist.go:324-332

export type ActivityReviewState =
  | "ok" | "partial" | "error" | "auth_required" | "no_gh" | "disabled";

export interface ActivityTask {
  id: number;
  title: string;
  doneAt: string;        // ms-ISO (reaper format 2006-01-02T15:04:05.000Z)
  projectId: number;
  projectName: string;
  // NOTE: inProgressAt / inReviewAt are json:"-" server-side — NOT shipped.
  // Do NOT recompute cycle/dwell on the client (RESEARCH Anti-Pattern).
}

export interface ReviewDoneSummary {
  number: number;
  title: string;
  completedAt: string;   // second-precision ISO (gh closedAt)
  url: string;           // https GitHub PR URL — the D-14 click target
  projectName: string;
  projectId: number;
  repo: string;          // owner/name
}

export interface TimeStat {
  n: number;             // count of rows this stat was computed over
  min: number;           // int64 SECONDS (truncated) — NOT ms
  max: number;
  median: number;
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
    state: ActivityReviewState;
    stale: boolean;
    fetchedAt: string | null;
    prs: ReviewDoneSummary[];
  };
  stats: StatsBlock;
}
```

**Query** — add `useActivity` to `queries.ts` (mirroring the `useTasks` param-keyed shape; the `get` primitive comes from `client.ts:37`). One endpoint → **one query, one loading state** (Phase 10 D-01):
```typescript
// Source pattern: web/src/api/queries.ts:29-35 (useTasks) + web/src/api/pullRequests.ts:37-45
import { get } from "./client";
import type { ActivityResponse } from "./types";

export function useActivity(
  scope: "global" | `workspace:${number}` | `project:${number}`,
  window: "week" | "month",
  enabled: boolean = true,
) {
  return useQuery({
    queryKey: ["activity", scope, window],
    queryFn: () =>
      get<ActivityResponse>(`/api/activity?scope=${scope}&window=${window}`),
    // No refetchInterval — Activity is an occasional view, not a live dashboard
    // (contrast pullRequests.ts:42 which polls every 60s).
    // `enabled` lets the page defer the fetch until the persisted scope resolves
    // (RESEARCH Pitfall 6 — don't fire with a null-derived scope).
    enabled,
  });
}
```

> **`get` + `ApiError`** come from `web/src/api/client.ts:1-39`. The endpoint is always HTTP 200 (Phase 10 D-02), so errors are transport-level only — surface them via the SettingsPage-style error+retry, never a stack trace (RESEARCH §Security V7).

> **Alternative (agent's discretion, RESEARCH §Alternatives):** a dedicated `web/src/api/activity.ts` (mirroring `pullRequests.ts` as a one-feature file) is equally valid if the wire types feel large co-located in `types.ts`. `queries.ts` already holds `useProjects`/`useWorkspaces`/`useGithubStatus` — co-locating is consistent. Pick one.

---

### `web/src/lib/useActivityView.ts` (hook, localStorage view state)

**Analog:** `web/src/lib/useActiveWorkspace.tsx` — the `kamacu.*` localStorage hook template. **BUT scope/window are consumed by ONE page only → a plain hook, NOT a context** (RESEARCH Assumption A4; contrast useActiveWorkspace.tsx:42-52 which explicitly justifies the context because the workspace state is read across sidebar + board + redirect).

**Key + grammar pattern** (lines 15, 31-37) — the `kamacu.*` prefix convention (see `migrateStorage.ts`). Validate the saved scope against the EXACT `parseScope` grammar (`global | workspace:N | project:N`, activity_helpers.go:51) so a tampered key falls back to default (mirrors useActiveWorkspace's stale-fallback at lines 60-65):
```typescript
// Source shape: web/src/lib/useActiveWorkspace.tsx:15,31-37
// Grammar anchor: internal/api/activity_helpers.go:51-80 (parseScope/parseWindow)

export const ACTIVITY_SCOPE_KEY = "kamacu.activity.scope";   // dot style (matches kamacu.workspace)
export const ACTIVITY_WINDOW_KEY = "kamacu.activity.window";

export type ActivityScope = "global" | `workspace:${number}` | `project:${number}`;
export type ActivityWindow = "week" | "month";

function readScope(fallback: ActivityScope): ActivityScope {
  const raw = localStorage.getItem(ACTIVITY_SCOPE_KEY);
  if (raw === "global" || /^workspace:\d+$/.test(raw) || /^project:\d+$/.test(raw)) {
    return raw;
  }
  return fallback;  // stale/missing/invalid → default (D-06)
}

function readWindow(): ActivityWindow {
  const raw = localStorage.getItem(ACTIVITY_WINDOW_KEY);
  return raw === "month" ? "month" : "week";  // default week (D-06)
}
```

> **Key separator note:** the codebase uses both `.` (`kamacu.workspace`) and `:` (`kamacu:review-collapsed:${projectId}` at `ReviewColumn.tsx:56`). `migrateStorage.ts:14-20` documents this — the migration does a separator-agnostic bare-word scan. Either separator is fine for a new key; the dot style matches the closest analog (`kamacu.workspace`).

**Plain hook shape** (no context, no provider — single consumer). The default-scope resolution against `useActiveWorkspace()` (D-05) happens HERE, mirroring useActiveWorkspace's resolve-against-live-data pattern at lines 57-66:
```typescript
// Source resolve-against-live-data pattern: web/src/lib/useActiveWorkspace.tsx:57-66
import { useState, useEffect } from "react";
import { useActiveWorkspace } from "@/lib/useActiveWorkspace";

export function useActivityView() {
  const { activeWorkspaceId } = useActiveWorkspace();
  // Lazily adopt the active workspace as default once it resolves AND no saved
  // scope exists (RESEARCH Pitfall 6). Initial fallback = "global" (D-05).
  const [scope, setScopeState] = useState<ActivityScope>(() => readScope("global"));
  const [window, setWindowState] = useState<ActivityWindow>(readWindow);

  // When the active workspace resolves and the user has no persisted scope,
  // adopt workspace:<id> as the default (D-05).
  useEffect(() => {
    if (activeWorkspaceId !== null && localStorage.getItem(ACTIVITY_SCOPE_KEY) === null) {
      setScopeState(`workspace:${activeWorkspaceId}`);
    }
  }, [activeWorkspaceId]);

  const setScope = (s: ActivityScope) => {
    setScopeState(s);
    localStorage.setItem(ACTIVITY_SCOPE_KEY, s);
  };
  const setWindow = (w: ActivityWindow) => {
    setWindowState(w);
    localStorage.setItem(ACTIVITY_WINDOW_KEY, w);
  };

  return { scope, setScope, window, setWindow };
}
```

---

### `web/src/lib/time.ts` (utility, transform — add `formatDuration`)

**Analog:** `web/src/lib/time.ts:4-10` (`formatAgo`) — the zero-dep adaptive-duration formatter to copy as a sibling.

**Existing `formatAgo`** (the style template — no date library, adaptive largest-units, UI-SPEC: zero new npm deps):
```typescript
// Source: web/src/lib/time.ts:4-10
export function formatAgo(iso: string | null, now: number): string {
  if (iso === null) return "0s";
  const secs = Math.max(0, Math.floor((now - Date.parse(iso)) / 1000));
  if (secs < 60) return `${secs}s`;
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `${mins}m`;
  return `${Math.floor(mins / 60)}h ${mins % 60}m`;
}
```

**New `formatDuration`** (CONTEXT D-10 — adaptive largest-1-2-units d/h/m/s; input is SECONDS from `timeStat.{min,max,median}`). Add as a sibling export. The `n===0` em-dash guard (D-11) is the CALLER's responsibility (see StatsStrip below) — `formatDuration` itself formats a finite non-negative number:
```typescript
/** Adaptive largest-1-2-units duration: "1d 3h", "4h 20m", "45m", "30s".
 *  Sibling of formatAgo; input is integer SECONDS (the timeStat wire contract
 *  — int64 seconds truncated server-side, activity_helpers.go:100-102). No date
 *  library (UI-SPEC: zero new npm deps). CONTEXT D-10. */
export function formatDuration(totalSeconds: number): string {
  if (!Number.isFinite(totalSeconds) || totalSeconds < 0) return "—";
  const s = Math.floor(totalSeconds);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  const remM = m % 60;
  if (h < 24) return remM > 0 ? `${h}h ${remM}m` : `${h}h`;
  const d = Math.floor(h / 24);
  const remH = h % 24;
  return remH > 0 ? `${d}d ${remH}h` : `${d}d`;
}
```

> **Unit labels (`m`/`h`/`d` vs `min`/`hr`):** CONTEXT D-10 leaves this to the agent. `m`/`h`/`d` matches `formatAgo`'s existing style (`12s`/`7m`/`1h 5m`) — stay consistent with the sibling formatter.

---

### `web/src/components/sidebar/ProjectSidebar.tsx` (component, nav — add Activity entry)

**Analog (mount point + active state):** `ProjectSidebar.tsx:174-191` (the Settings gear).
**Analog (collapsed-rail dual-surface):** `ProjectSidebar.tsx:117-142` (the project row's icon-rail + expanded-row pair).

**The wrinkle (CONTEXT D-02 — locked):** the current `SidebarFooter` (line 162) carries `group-data-[collapsible=icon]:hidden`, which hides the WHOLE footer (Add project + Settings gear) in the collapsed rail. **Activity must NOT be swallowed by that hide** — it stays reachable in both states. Two valid structures (agent's pick):

**Structure A (recommended) — dual-surface button INSIDE the footer**, applying the project-row's rail/expanded pair classes (lines 117, 140):
```tsx
// Source pattern: web/src/components/sidebar/ProjectSidebar.tsx:117-142 (rail/expanded pair)
// Active-state idiom: ProjectSidebar.tsx:180-183 (Settings gear)
<Tooltip>
  <TooltipTrigger asChild>
    <Button
      asChild
      variant="ghost"
      size="icon-sm"
      className={cn(
        // rail icon: visible ONLY when collapsed (overrides the footer's hide)
        // expanded icon: visible only when expanded (the footer's hide handles it)
        location.pathname === "/activity" &&
          "bg-sidebar-accent text-sidebar-accent-foreground",
      )}
    >
      <Link to="/activity" aria-label="Activity">
        <Activity className="size-4" />
      </Link>
    </Button>
  </TooltipTrigger>
  <TooltipContent side="right">Activity</TooltipContent>
</Tooltip>
```

> The cleanest fix is usually to **remove `group-data-[collapsible=icon]:hidden` from the `SidebarFooter` className** (line 162) and instead apply it per-child: keep it on "Add project" + the Settings gear (their current behavior — they hide in the rail), and leave the Activity button without it so it survives the collapse. This mirrors how the project rows (lines 117-142) manage their own rail-vs-expanded visibility rather than relying on a parent hide.

**Icon import** (D-03 — `lucide-react` `Activity`, verified present at runtime):
```typescript
// Source: web/src/components/sidebar/ProjectSidebar.tsx:3
import { Plus, Settings, Activity } from "lucide-react";  // ADD Activity
```

**Active-state pattern** (lines 180-183 — the Settings gear idiom; CONTEXT D-01):
```tsx
className={cn(
  location.pathname === "/activity" &&
    "bg-sidebar-accent text-sidebar-accent-foreground",
)}
```

> **Anti-pattern (RESEARCH §Pitfall 1):** dropping the Activity button inside the footer WITHOUT addressing the `group-data-[collapsible=icon]:hidden` makes it vanish when the sidebar is collapsed — violating D-02's "always reachable" lock. Test with Ctrl+B.

---

### `web/src/components/activity/ScopeSelector.tsx` (component, event-driven — DISCRETIONARY)

**Analog:** `web/src/components/sidebar/WorkspaceSwitcher.tsx` — the sectioned-dropdown template (CONTEXT D-04).

**Imports + trigger pattern** (lines 1-13, 56-71) — the `DropdownMenu` + `ChevronsUpDown` trigger:
```tsx
// Source: web/src/components/sidebar/WorkspaceSwitcher.tsx:1-13,29-71
import { ChevronsUpDown } from "lucide-react";
import { useProjects, useWorkspaces } from "@/api/queries";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,   // selected-✓ idiom (lines 73-81)
  DropdownMenuContent,
  DropdownMenuLabel,          // section headers (CONTEXT D-04)
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
```

**Sectioned menu + selected-✓ pattern** (lines 72-91) — Activity's dropdown lists Global + every workspace + each project, with the active scope checked:
```tsx
// Source: web/src/components/sidebar/WorkspaceSwitcher.tsx:72-91
<DropdownMenuContent side="bottom" align="start">
  <DropdownMenuLabel>Global</DropdownMenuLabel>
  <DropdownMenuCheckboxItem
    checked={scope === "global"}
    onSelect={() => setScope("global")}
  >
    All activity
  </DropdownMenuCheckboxItem>

  <DropdownMenuSeparator />
  <DropdownMenuLabel>Workspaces</DropdownMenuLabel>
  {sortedWorkspaces.map((ws) => (
    <DropdownMenuCheckboxItem
      key={ws.id}
      checked={scope === `workspace:${ws.id}`}
      onSelect={() => setScope(`workspace:${ws.id}`)}
    >
      {ws.name}
    </DropdownMenuCheckboxItem>
  ))}

  <DropdownMenuSeparator />
  <DropdownMenuLabel>Projects</DropdownMenuLabel>
  {sortedProjects.map((p) => (
    <DropdownMenuCheckboxItem
      key={p.id}
      checked={scope === `project:${p.id}`}
      onSelect={() => setScope(`project:${p.id}`)}
    >
      {p.name}
    </DropdownMenuCheckboxItem>
  ))}
</DropdownMenuContent>
```

> **Name-sort the lists** (WorkspaceSwitcher.tsx:42-44 sorts workspaces by name; projects arrive name-sorted from the backend — `useProjects` returns them `ORDER BY name COLLATE NOCASE`, see App.tsx:30-31). This keeps dropdown order stable regardless of fetch order.

> **`DropdownMenuSub` (nesting) alternative:** RESEARCH §Open Question 2 leaves flat-with-labels vs nested-workspace→projects to the agent. Flat (above) matches WorkspaceSwitcher's simplicity; `DropdownMenuSub`/`DropdownMenuSubTrigger`/`DropdownMenuSubContent` are all already exported (`dropdown-menu.tsx:253-268`) if nesting is preferred for many projects.

---

### `web/src/components/activity/WindowToggle.tsx` (component, event-driven — DISCRETIONARY)

**Analog:** `web/src/components/ui/tabs.tsx` — default-variant `Tabs` IS visually a segmented control (`bg-muted` list, active `bg-background` pill — see `tabsListVariants` at tabs.tsx:25-38, `data-active:bg-background` at tabs.tsx:66). CONTEXT D-07 + RESEARCH §"Don't Hand-Roll" both endorse this reuse.

**Segmented control via `Tabs`** (zero new CSS — `value`/`onValueChange` bind directly to the window state):
```tsx
// Source primitives: web/src/components/ui/tabs.tsx (default variant)
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

export function WindowToggle({
  window,
  onWindowChange,
}: {
  window: "week" | "month";
  onWindowChange: (w: "week" | "month") => void;
}) {
  return (
    <Tabs value={window} onValueChange={(v) => onWindowChange(v as "week" | "month")}>
      <TabsList>
        <TabsTrigger value="week">Week</TabsTrigger>
        <TabsTrigger value="month">Month</TabsTrigger>
      </TabsList>
    </Tabs>
  );
}
```

> **Alternative (agent's discretion):** two `Button`s with active styling is equally valid (D-07). `Tabs` is zero-new-code reuse; pick it unless the visual differs from the design.

---

### `web/src/components/activity/StatsStrip.tsx` (component, transform — DISCRETIONARY)

**Analog:** `web/src/components/board/ReviewColumn.tsx:161-269` (`ReviewStates`) — for the count-renders-as-number + empty-state pattern. The `min · median · max` row shape is NEW (no existing analog) but trivial.

**Time-stat row with em-dash guard** (CONTEXT D-09 + D-11 — `n===0` → em-dash for the whole row, counts always render literal):
```tsx
// Em-dash guard BEFORE formatting (RESEARCH §Pitfall 4).
function TimeStatRow({ label, stat }: { label: string; stat: TimeStat }) {
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

**Counts as lead numbers** (D-09) — render `stats.taskCount` / `stats.reviewCount` as the prominent readouts (mirror `ReviewColumn.tsx:130-132` count style):
```tsx
<div className="flex items-baseline gap-2">
  <span className="text-2xl font-semibold tabular-nums">{stats.taskCount}</span>
  <span className="text-xs text-muted-foreground">tasks done</span>
</div>
<div className="flex items-baseline gap-2">
  <span className="text-2xl font-semibold tabular-nums">{stats.reviewCount}</span>
  <span className="text-xs text-muted-foreground">reviews done</span>
</div>
```

> **No cards grid, no charts** (D-09 / Out of Scope ACTFUT-02). The strip is a compact horizontal/vertical row of readouts, not a dashboard.

---

### `web/src/components/activity/ActivityList.tsx` (component, transform/group — DISCRETIONARY)

**Analog:** `web/src/components/board/ReviewColumn.tsx:209-211` (`.map` over the PR list) — the list-render idiom. Grouping by project is a client-side bucket (no existing analog; RESEARCH §"Don't Hand-Roll" endorses `Array.reduce` into a `Map`).

**Client-side grouping by project** (CONTEXT D-13). Tasks arrive `doneAt DESC` from the server (verified — `internal/api/activity_test.go:180-257`), so no client sort needed:
```typescript
// Source idiom: web/src/components/board/ReviewColumn.tsx:209-211 (.map over list)
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
  // Group ordering (by name vs by recency) is agent's discretion (CONTEXT).
  return [...map.values()];
}
```

**Entry click → navigate** (CONTEXT D-15 — `activityTask` carries `id` + `projectId`):
```tsx
// Source Link idiom: web/src/components/sidebar/ProjectSidebar.tsx:103-107
<Link to={`/projects/${task.projectId}/tasks/${task.id}`}>
  <span className="flex-1 truncate text-sm">{task.title}</span>
  <span className="text-xs text-muted-foreground">{formatAgo(task.doneAt, now)}</span>
</Link>
```

> **Reuse `formatAgo`** (lib/time.ts:4) for the relative completion time — it already handles ISO timestamps and is the established idiom (PRCard.tsx:180, ReviewColumn.tsx:189).

---

### `web/src/components/activity/ReviewsList.tsx` (component, transform + degrade — DISCRETIONARY)

**Analog (degrade-branch structure):** `web/src/components/board/ReviewColumn.tsx:161-269` (`ReviewStates`) — the **exact** `data.state === "no_gh" | "auth_required" | "error"` branching idiom.

> **CRITICAL CONTRAST (CONTEXT D-12 + RESEARCH §Pitfall 2):** ReviewColumn renders an **amber hint** when degraded (lines 238-249). **Activity does the OPPOSITE** — it renders **QUIETLY EMPTY (return null)**, no error, no hint, no heading. Copy the *branching structure*, NOT the *result*. The hard-degrade state set must include `auth_required` (which CONTEXT D-12 omits but the backend emits — RESEARCH §Pitfall 2).

**Degrade-branch pattern** — the set-based check mirrors ReviewColumn.tsx:218-221, the result is `null` instead of an amber note:
```tsx
// Source branch structure: web/src/components/board/ReviewColumn.tsx:218-221
// Source degrade enum: web/src/api/pullRequests.ts:25 (same 5 states + "partial")

const HARD_DEGRADE: ReadonlySet<ActivityReviewState> = new Set([
  "disabled",
  "no_gh",
  "auth_required",   // NOT in CONTEXT D-12 — backend emits it (RESEARCH Pitfall 2)
  "error",
]);

function ReviewsList({ reviews }: { reviews: ActivityResponse["reviews"] }) {
  // D-12: quietly empty on hard-degrade — return null (NO amber hint, NO heading).
  // ReviewColumn.tsx:222 renders a hint here; Activity must NOT.
  if (HARD_DEGRADE.has(reviews.state) && reviews.prs.length === 0) {
    return null;
  }
  // ok / partial / error-with-cached-prs: render the grouped list.
  // (partial carries real PRs — never whitelist {ok} alone; RESEARCH Pitfall 2.)
  const groups = groupByProject([...reviews.prs].sort(byCompletedDesc));
  if (groups.length === 0) return null;  // quiet empty even on ok+empty
  // …render groups with project subheadings + entry <a>…
}
```

**Reviews client-side sort** (RESEARCH §Pitfall 5 — reviews are NOT pre-sorted across repos; the server concatenates per-repo lists unsorted. Tasks, by contrast, arrive pre-sorted. Sort reviews by `completedAt DESC` before/within grouping):
```typescript
// completedAt is uniform second-precision ISO (gh closedAt) → lexical DESC sort is chronological.
export const byCompletedDesc = (a: ReviewDoneSummary, b: ReviewDoneSummary) =>
  b.completedAt.localeCompare(a.completedAt);
```

**Entry click → external PR URL** (CONTEXT D-14 — universal PR-URL fallback, NOT an in-app workspace lookup). Use a plain `<a>`, **never** a router `<Link>` (it's an external URL):
```tsx
// Source: web/src/components/board/PRCard.tsx:166-176 (external-link idiom)
// Source: web/src/pages/TaskPage.tsx:608-616 (the same idiom, #num as the link)
<a
  href={review.url}
  target="_blank"
  rel="noreferrer"
  aria-label={`Open PR #${review.number} on GitHub`}
>
  #{review.number} {review.title}
</a>
```

> **Security (RESEARCH §V5):** `url` is server-sourced from `gh pr list --json url` (always https GitHub). React auto-escapes `{title}`/`{projectName}`. Optional cheap belt-and-suspenders: `url.startsWith("https://")` guard before rendering as `href` (mitigates a hypothetical server compromise). Never use `dangerouslySetInnerHTML`.

---

## Shared Patterns

### Page Shell (loading / error / scroll)
**Source:** `web/src/pages/SettingsPage.tsx:128-156`
**Apply to:** `ActivityPage.tsx` (verbatim — the top-level-page template)
```tsx
// Error + retry
if (isError) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3">
      <p className="text-muted-foreground">{`Couldn't load activity.`}</p>
      <Button variant="outline" onClick={() => refetch()}>{`Retry loading`}</Button>
    </div>
  );
}
// Shell
<div className="h-full overflow-y-auto p-4">
  <div className="max-w-[640px]"> … </div>
</div>
// Loading (quiet Skeletons)
{isLoading || !data ? (<div className="mt-6 flex flex-col gap-6">{[0,1,2,3].map(…)}</div>) : (…)}
```

### TanStack Query (param-keyed, always-200 degrade contract)
**Source:** `web/src/api/queries.ts:29-35` (`useTasks`) + `web/src/api/pullRequests.ts:37-45` (`usePullRequests`)
**Apply to:** `useActivity(scope, window)` in `queries.ts` (or a dedicated `api/activity.ts`)
```typescript
return useQuery({
  queryKey: ["activity", scope, window],
  queryFn: () => get<ActivityResponse>(`/api/activity?scope=${scope}&window=${window}`),
  enabled,  // defer until scope resolves (RESEARCH Pitfall 6)
});
// No refetchInterval (Activity is occasional, not a live poll — contrast pullRequests.ts:42)
```

### localStorage View-State (kamacu.* prefix, validate-on-read)
**Source:** `web/src/lib/useActiveWorkspace.tsx:15,31-37` + `web/src/lib/migrateStorage.ts`
**Apply to:** `useActivityView.ts` (the scope+window hook)
- Prefix every key with `kamacu.` (dot, matching `kamacu.workspace`) or `kamacu:` (colon, matching `kamacu:review-collapsed:N`) — both are valid; pick dot for consistency with the closest analog.
- **Validate the saved value against the live grammar on read** — a tampered `kamacu.activity.scope` must fall back to default, never produce a malformed query (mirrors useActiveWorkspace.tsx:60-65 stale-fallback; `parseScope` rejects malformed values with HTTP 400).
- **Plain hook, NOT a context** — scope/window have one consumer (ActivityPage); a context (like useActiveWorkspace) is overkill (RESEARCH A4).

### Sidebar Active State + Tooltip
**Source:** `web/src/components/sidebar/ProjectSidebar.tsx:174-191` (Settings gear) + `:115-130` (project row Tooltip)
**Apply to:** the Activity entry in `ProjectSidebar.tsx` SidebarFooter
```tsx
className={cn(location.pathname === "/activity" && "bg-sidebar-accent text-sidebar-accent-foreground")}
// Wrap in <Tooltip><TooltipTrigger asChild>…</TooltipTrigger><TooltipContent side="right">Activity</TooltipContent></Tooltip>
```

### Collapsed-Rail Dual-Surface (the D-02 wrinkle)
**Source:** `web/src/components/sidebar/ProjectSidebar.tsx:117-142` (project row rail-icon + expanded-label pair)
**Apply to:** the Activity entry — it must stay clickable when the sidebar is collapsed
```tsx
// Rail icon (visible only when collapsed): hidden group-data-[collapsible=icon]:flex
// Expanded label (visible only when expanded): group-data-[collapsible=icon]:hidden
```
> The footer's existing `group-data-[collapsible=icon]:hidden` (line 162) hides the WHOLE footer. Either lift Activity out of that wrapper, or remove the class from the footer and re-apply it per-child (Add project + Settings gear keep hiding; Activity does not). RESEARCH §Pitfall 1.

### External Link (reviews click-through, D-14)
**Source:** `web/src/components/board/PRCard.tsx:166-176` + `web/src/pages/TaskPage.tsx:608-616`
**Apply to:** every reviews-done entry in `ReviewsList.tsx`
```tsx
<a href={review.url} target="_blank" rel="noreferrer" aria-label={`Open PR #${review.number} on GitHub`}>
  …
</a>
// NEVER a router <Link> (external URL). NEVER window.open (popup-blocked — RESEARCH Pitfall 7).
```

### Degrade Branch on `reviews.state` (always-200 contract)
**Source:** `web/src/components/board/ReviewColumn.tsx:161-269` (`ReviewStates`) + `web/src/api/pullRequests.ts:24-30` (the state enum)
**Apply to:** `ReviewsList.tsx` (the reviews-done section)
- Branch on the state set `{ disabled, no_gh, auth_required, error }` — **include `auth_required`** (RESEARCH §Pitfall 2 — CONTEXT D-12 omits it but the backend emits it).
- **Result differs from the analog:** ReviewColumn renders an amber hint; **Activity returns `null`** (D-12 — quietly empty, no hint, no heading).
- **Never whitelist `{ok}` alone** — `partial` carries real PRs and must render.

### Section Heading Typography
**Source:** `web/src/pages/SettingsPage.tsx:79,161,171,183,197` (the universal `h2` style)
**Apply to:** every section/group heading in ActivityPage
```tsx
<h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{`…`}</h2>
```

### No Toasts (project convention)
**Source:** Phase 26 convention (restated in CONTEXT canonical_refs); `ReviewColumn.tsx:238-249` shows the inline-feedback alternative (which Activity does NOT use — it goes further to `null`).
**Apply to:** all feedback in ActivityPage — degrade states, errors, empty states are inline/muted/`null`, **never** a toast/sonner. The project has no toast library installed and Phase 26 confirmed the convention.

---

## No Analog Found

**None.** Every file in this phase has at least a role-match analog in the codebase:

| File | Closest Analog | Notes |
|------|----------------|-------|
| (all) | see table above | The phase is structurally a clone-and-adapt of SettingsPage (shell) + WorkspaceSwitcher (dropdown) + ProjectSidebar footer (entry) + ReviewColumn/ReviewStates (degrade + list render) + useActiveWorkspace (localStorage hook) + time.ts/formatAgo (duration formatter). The only genuinely NEW logic is the ~15-line `formatDuration` and the degrade-branch result-flip (null vs hint) — both trivial. |

If the planner needs a pattern not covered here, fall back to RESEARCH.md §"Code Examples" (which carries the verified wire contract + the `formatDuration` + `TimeStatRow` + `groupByProject` + `ReviewsSection` reference implementations).

---

## Metadata

**Analog search scope:**
- `web/src/pages/` (4 files — SettingsPage, BoardPage, TaskPage, TerminalPage)
- `web/src/components/sidebar/` (8 files — ProjectSidebar, WorkspaceSwitcher, + dialogs)
- `web/src/components/board/` (6 files — PRCard, ReviewColumn, Column, Board, TaskCard, NewTaskDialog)
- `web/src/components/ui/` (20 files — dropdown-menu, tabs, skeleton, button, tooltip, sidebar, …)
- `web/src/lib/` (5 files — time, useActiveWorkspace, migrateStorage, palette, utils)
- `web/src/api/` (12 files — queries, client, types, pullRequests, …)
- `web/src/App.tsx`

**Files scanned:** ~55 source files (all `web/src/**/*.{ts,tsx}` relevant to the phase)
**Pattern extraction date:** 2026-07-31
**Confidence:** HIGH — every analog cited is a named in-repo file at a named line, cross-checked against the Phase 10 wire contract in RESEARCH.md.
